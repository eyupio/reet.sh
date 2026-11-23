// Package api exposes the JSON API and the bookmarklet endpoint.
package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eyupio/reet.sh/internal/agenthealth"
	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/agentupdate"
	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/backup"
	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/mail"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/notify"
	"github.com/eyupio/reet.sh/internal/oidc"
	"github.com/eyupio/reet.sh/internal/projectrun"
	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/softwaremirror"
	"github.com/eyupio/reet.sh/internal/softwarerun"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/syncproof"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
)

type server struct {
	importMu sync.Mutex
	st       *store.Store
	f        meta.Fetcher
	token    string
	version  string
	commit   string
	// mcpAPI is the API handler reet's own tools are run against, for Gaffer.
	mcpAPI         http.Handler
	debug          bool
	topics         bool
	refreshJob     bool
	maintenanceJob bool
	authMode       AuthMode
	signup         bool
	supportDesk    bool
	contactEmail   string
	push           pushState
	limiter        *auth.Limiter
	addrLimiter    *auth.Limiter // failed password sign-ins per address seen; generous, as a proxy is one address
	dummyHash      string
	oauthLimiter   *auth.Limiter
	// trustedProxies are the peers whose X-Forwarded-For names the client.
	trustedProxies []netip.Prefix
	publicURL      string
	defaultUser    store.User
	queue          chan ref.Ref
	stop           chan struct{}
	stopOnce       sync.Once
	ctx            context.Context
	cancel         context.CancelFunc
	workers        sync.WaitGroup
	backups        *backup.Service
	mail           *mail.Mailer
	notify         *notify.Service
	releases       *agentupdate.Checker

	// skillFetcher gets public skills for the reet copy; skillCopies waits for
	// the background runs (tests only) and the rest keeps them one per account.
	skillFetcher SkillFetcher
	// refreshHosted checks a skill's public page follows its head after a
	// content save; tests replace it to make it fail.
	refreshHosted    func(uid int64, item string) error
	skillCopyMu      sync.Mutex
	skillCopyRunning map[int64]bool
	skillCopyAgain   map[int64]bool
	skillCopies      sync.WaitGroup
	original         originalCache
	originalMu       sync.Mutex
	originalRunning  map[int64]int // originals each account is downloading now
	originalTTL      time.Duration
	skillPollKicks   pollKicks
	// skillClock is when the Skills page's changes are made (tests only).
	skillClock func() time.Time

	sso                *oidc.Provider
	ssoSaveMu          sync.Mutex // one change to the single sign-on settings at a time
	afterSSOStore      func()     // tests only: runs after a save is stored and before it takes effect
	ssoLimiter         *auth.Limiter
	logoutLimiter      *auth.Limiter
	afterSessionsEnded func(ids []string) // a test seam: which sessions were just ended

	privateSealer     *sealed.Sealer
	gafferSealer      *sealed.Sealer
	gafferProviders   *gaffer.Service
	privateRelayURL   string
	private           privateTransport
	healthHub         *agenthealth.Hub
	terminals         *terminalbroker.Broker
	runs              *projectrun.Orchestrator
	softwareRuns      *softwarerun.Orchestrator
	mirror            *softwaremirror.Mirror
	mirrorServes      chan struct{}
	resumeDone        chan struct{} // closed when ResumeProjectRuns' watcher ends
	remoteTerminals   bool
	terminalOverride  atomic.Pointer[bool]
	publicURLOverride atomic.Pointer[string]
	terminalControlMu sync.Mutex
	// Folder browse and branch listing (terminal_browse.go).
	browseOnce   sync.Once
	browseLimits *browseLimits
	browseAudit  *browseAudit
	// Person-pressed model refreshes per account and machine (models.go).
	modelsOnce   sync.Once
	modelsLimits *modelLimits
	// accountsLimits bounds person-pressed reads of a machine's agent sign-ins.
	accountsOnce   sync.Once
	accountsLimits *modelLimits
	// Image uploads per account (terminal_upload.go).
	uploadLimits uploadLimits
	// Folder-project writes per account (folder_projects.go).
	folderWritesMu sync.Mutex
	folderWrites   *auth.Limiter
	// Clones of GitHub repositories that become projects (folder_projects_github.go).
	clones repoClones
	// One lock per draft serializes start/recovery of one backlog request.
	backlogStarts keyedLocks
	// Session cleanup after a confirmed close (session_isolation.go).
	sessionCleanupMu sync.Mutex
	sessionCleanup   map[string]bool
}

// Server is the application handler. Close releases an active private
// listener; the process still needs its normal shutdown for everything else.
type Server struct {
	s *server
	h http.Handler
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.h.ServeHTTP(w, r) }

// SignedIn reports whether the request carries a valid account session, so the
// SPA's static landing fallback can be left out of the served HTML.
func (s *Server) SignedIn(r *http.Request) bool { return s.s.SignedIn(r) }

// SetRemoteTerminals changes controller availability, never host shell lifetime.
// Closing control generations fences existing tickets/streams; agents reconnect
// and reconcile their surviving tmux sessions with fresh authorization.
func (s *Server) SetRemoteTerminals(enabled bool) {
	s.s.terminalControlMu.Lock()
	defer s.s.terminalControlMu.Unlock()
	s.s.terminalOverride.Store(&enabled)
	s.s.terminals.ResetConnections()
}
func (s *Server) RemoteTerminalsAvailable() bool { return s.s.terminalEnabled() }
func (s *Server) ConfigureTerminalOrigin(origin string) error {
	if s.s.publicOrigin() == origin {
		return nil
	}
	if !validTerminalOrigin(origin) {
		return errors.New("a valid HTTPS public URL is required (loopback HTTP is allowed)")
	}
	s.s.publicURLOverride.Store(&origin)
	return nil
}
func (s *Server) RemoteTerminalsReason() string   { return s.s.terminalUnavailableReason() }
func (s *Server) RemoteTerminalPublicURL() string { return s.s.publicOrigin() }
func (s *Server) RemoteTerminalAuthMode() string  { return string(s.s.authMode) }
func (s *server) publicOrigin() string {
	if origin := s.publicURLOverride.Load(); origin != nil {
		return *origin
	}
	return s.publicURL
}

// Close stops the private listener, the background workers and any of its
// active streams. It does not close the store.
func (s *Server) Close() error {
	if s == nil || s.s == nil {
		return nil
	}
	if s.s.stop != nil {
		s.s.stopOnce.Do(func() {
			close(s.s.stop)
			if s.s.cancel != nil {
				// Under the lock a kick takes, so a kick either sees the
				// cancelled context or has added to workers before Wait.
				s.s.skillCopyMu.Lock()
				s.s.cancel()
				s.s.skillCopyMu.Unlock()
			}
		})
	}
	s.s.workers.Wait()
	s.s.clones.close()
	if s.s.gafferProviders != nil {
		s.s.gafferProviders.Close()
	}
	if s.s.healthHub != nil {
		s.s.healthHub.Close()
	}
	if s.s.runs != nil {
		s.s.runs.Close()
	}
	if s.s.softwareRuns != nil {
		s.s.softwareRuns.Close()
	}
	if s.s.terminals != nil {
		s.s.terminals.Close()
	}
	if s.s.private == nil {
		return nil
	}
	return s.s.private.Close()
}

// New returns the API handler. An empty token disables authentication.
func New(st *store.Store, f meta.Fetcher, token string, opts ...Option) *Server {
	s := newServer(st, f, token, opts...)
	s.workers.Add(1)
	go func() { defer s.workers.Done(); s.fenceWatcher() }()
	return &Server{s: s, h: s.logRequests(s.fenceGate(s.auth(s.routes())))}
}

func newServer(st *store.Store, f meta.Fetcher, token string, opts ...Option) *server {
	dummy, _ := auth.HashPassword("not-a-real-password")
	ctx, cancel := context.WithCancel(context.Background())
	s := &server{
		st: st, f: f, token: token, version: "dev", commit: "unknown",
		authMode: AuthToken, limiter: auth.NewLimiter(5, time.Minute), addrLimiter: auth.NewLimiter(60, time.Minute), oauthLimiter: auth.NewLimiter(30, time.Minute), dummyHash: dummy,
		ssoLimiter: auth.NewLimiter(60, time.Minute), logoutLimiter: auth.NewLimiter(60, time.Minute),
		folderWrites: auth.NewLimiter(folderProjectWriteLimit, time.Minute),
		queue:        make(chan ref.Ref, 256),
		stop:         make(chan struct{}),
		ctx:          ctx,
		cancel:       cancel,

		skillCopyRunning: map[int64]bool{}, skillCopyAgain: map[int64]bool{},
		originalTTL: skillOriginalTTL,
	}
	s.refreshHosted = st.RefreshHostedSkillFor
	for _, opt := range opts {
		opt(s)
	}
	if s.skillFetcher == nil {
		// Nothing reaches the network unless the caller passes WithSkillFetcher;
		// cmd/reet passes skillfetch.Fetcher{}, whose default client guards its
		// addresses and honours proxies.
		s.skillFetcher = noSkillFetcher{}
	}
	if s.sso != nil {
		// The environment seeds the owner's single sign-on settings once; after
		// that the stored ones are the only ones used.
		s.sso.UseStoredBehaviour(st, log.Printf)
	}
	s.gafferProviders = gaffer.New(st, s.gafferSealer)
	if s.authMode == AuthToken || s.authMode == AuthNone {
		if id, err := st.DefaultOwner(); err == nil {
			if u, ok, err := st.UserByID(id); err == nil && ok {
				s.defaultUser = u
			}
		}
	}
	s.workers.Add(1)
	go func() { defer s.workers.Done(); s.worker() }()
	if s.topics {
		s.workers.Add(1)
		go func() { defer s.workers.Done(); s.topicWorker() }()
	}
	if s.refreshJob {
		s.workers.Add(1)
		go func() { defer s.workers.Done(); s.refreshWorker() }()
	}
	if s.maintenanceJob {
		s.workers.Add(1)
		go func() { defer s.workers.Done(); s.maintenanceWorker() }()
	}
	s.healthHub = agenthealth.NewHub(agenthealth.HubConfig{Store: st, Logf: func(format string, args ...any) {
		log.Printf("machine health: "+format, args...)
	}})
	s.terminals = terminalbroker.New(st)
	s.terminals.SetWorkerMessagingPolicy(s.terminalEnabled)
	s.gafferProviders.SetWorker(s.terminals, s.terminalEnabled)
	s.gafferProviders.SetSessionStarter(outcomeStarter{s})
	// What Gaffer and the controller do by themselves is in the activity log too, under its own source.
	s.gafferProviders.SetToolHost(gafferToolHost{s})
	s.gafferProviders.SetAudit(func(uid int64, action string, detail map[string]any) {
		s.event(uid, "gaffer", action, "", detail)
	})
	s.gafferProviders.StartGoalSupervisor()
	s.terminals.OnLateWorktreeRemoval(s.auditLateWorktreeRemoval)
	// Temporary projects (a repository cloned for one start) go, with their
	// clone, once nothing uses them and nothing would be lost.
	s.workers.Add(1)
	go func() { defer s.workers.Done(); s.temporaryProjectWorker() }()
	// Devices that asked to hear when a session needs their person.
	s.workers.Add(1)
	go func() { defer s.workers.Done(); s.pushWorker() }()
	s.terminals.OnSessionClosed(s.onSessionClosed)
	s.terminals.OnPeerReady(s.onPeerReady)
	s.terminals.OnSessionBase(s.recordSessionBase)
	s.runs = s.newProjectRuns()
	s.softwareRuns = s.newSoftwareRuns()
	if dir := st.Dir(); dir != "" {
		s.mirror = softwaremirror.New(filepath.Join(dir, "software", "artifacts"), agentsoftware.OpenCodeLinuxX64)
		_ = s.mirror.GC()
	}
	s.mirrorServes = make(chan struct{}, 4)
	s.configurePrivate()
	return s
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	s.registerSyncControl(mux)
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/version", s.buildInfo)
	mux.HandleFunc("GET /api/skills", s.list)
	mux.HandleFunc("POST /api/skills", s.requireWriter(s.add))
	mux.HandleFunc("PATCH /api/skill", s.requireWriter(s.patch))
	mux.HandleFunc("DELETE /api/skill", s.requireWriter(s.remove))
	mux.HandleFunc("POST /api/skill/refresh", s.requireWriter(s.refresh))
	mux.HandleFunc("POST /api/skill/resolve", s.requireWriter(s.resolve))
	mux.HandleFunc("POST /api/skill/tags", s.requireWriter(s.skillTags))
	mux.HandleFunc("PATCH /api/skills", s.requireWriter(s.patchMany))
	mux.HandleFunc("POST /api/skills/tags", s.requireWriter(s.tagMany))
	mux.HandleFunc("POST /api/skills/remove", s.requireWriter(s.removeMany))
	mux.HandleFunc("POST /api/skill/install-copy", s.requireWriter(s.installCopy))
	mux.HandleFunc("POST /api/import", s.requireWriter(s.importSkills))
	mux.HandleFunc("GET /api/environments", s.environments)
	mux.HandleFunc("GET /api/v2/setup/profiles", s.setupProfileList)
	mux.HandleFunc("PUT /api/v2/setup/profiles/{name}", s.requireWriter(s.saveSetupProfile))
	mux.HandleFunc("PATCH /api/v2/setup/profiles/{name}", s.requireWriter(s.renameSetupProfile))
	mux.HandleFunc("GET /api/v2/setup/profiles/{name}", s.getSetupProfile)
	mux.HandleFunc("DELETE /api/v2/setup/profiles/{name}", s.requireWriter(s.deleteSetupProfile))
	mux.HandleFunc("GET /api/v2/sync", s.syncState)
	mux.HandleFunc("POST /api/v2/machines/merge", s.requireWriter(s.mergeMachines))
	mux.HandleFunc("DELETE /api/v2/machines", s.requireWriter(s.forgetMachine))
	mux.HandleFunc("DELETE /api/v2/environments", s.requireWriter(s.forgetEnvironment))
	mux.HandleFunc("POST /api/v2/environments/delete", s.requireWriter(s.deleteEnvironments))
	mux.HandleFunc("POST /api/v2/environments/merge", s.requireWriter(s.mergeEnvironments))
	mux.HandleFunc("POST /api/v2/skills/bulk", s.requireWriter(s.bulkUpdateSkills))
	mux.HandleFunc("GET /api/v2/skills/duplicates", s.findDuplicates)
	s.registerSkillCopies(mux)
	s.registerSkillsPage(mux)
	s.registerSkillDetail(mux)
	mux.HandleFunc("GET /api/tags", s.tags)
	mux.HandleFunc("POST /api/v2/tags", s.requireWriter(s.createTag))
	mux.HandleFunc("PATCH /api/v2/tags/{id}", s.requireWriter(s.renameTagScoped))
	mux.HandleFunc("POST /api/v2/tags/{id}/delete", s.requireWriter(s.deleteTag))
	mux.HandleFunc("PATCH /api/tag", s.requireAdmin(s.renameTag))
	mux.HandleFunc("POST /api/tags/merge", s.requireAdmin(s.mergeTags))
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("GET /api/admin/status", s.requireAdmin(s.adminStatus))
	mux.HandleFunc("GET /api/admin/rollup", s.requireAdmin(s.rollup))
	mux.HandleFunc("GET /api/v2/auth/status", s.authStatus)
	mux.HandleFunc("POST /api/v2/auth/setup", s.setup)
	mux.HandleFunc("POST /api/v2/auth/signup", s.signUp)
	mux.HandleFunc("POST /api/v2/auth/sign-in", s.signIn)
	mux.HandleFunc("POST /api/v2/auth/sign-out", s.signOut)
	mux.HandleFunc("GET /api/v2/auth/me", s.me)
	mux.HandleFunc("GET /api/v2/auth/invite", s.inviteInfo)
	mux.HandleFunc("POST /api/v2/auth/join", s.join)
	s.registerSSORoutes(mux)
	mux.HandleFunc("GET /api/v2/tokens", s.requireWriter(s.listTokens))
	mux.HandleFunc("POST /api/v2/tokens", s.requireWriter(s.createToken))
	mux.HandleFunc("DELETE /api/v2/tokens", s.requireWriter(s.revokeToken))
	mux.HandleFunc("GET /api/v2/admin/users", s.requireAdmin(s.adminUsers))
	mux.HandleFunc("PATCH /api/v2/admin/users", s.requireAdmin(s.adminUserUpdate))
	mux.HandleFunc("DELETE /api/v2/admin/users", s.requireOwner(s.adminUserDelete))
	mux.HandleFunc("POST /api/v2/admin/users/reset", s.requireAdmin(s.adminUserReset))
	mux.HandleFunc("POST /api/v2/admin/users/revoke", s.requireAdmin(s.adminUserRevoke))
	mux.HandleFunc("POST /api/v2/admin/users/transfer", s.requireOwner(s.adminUserTransfer))
	mux.HandleFunc("POST /api/v2/admin/invites", s.requireAdmin(s.adminInvite))
	mux.HandleFunc("GET /api/v2/lists", s.listLists)
	mux.HandleFunc("POST /api/v2/lists", s.requireWriter(s.createList))
	mux.HandleFunc("GET /api/v2/lists/{id}", s.listDetail)
	mux.HandleFunc("PATCH /api/v2/lists/{id}", s.updateList)
	mux.HandleFunc("DELETE /api/v2/lists/{id}", s.deleteList)
	mux.HandleFunc("POST /api/v2/lists/{id}/skills", s.addListSkill)
	mux.HandleFunc("POST /api/v2/lists/{id}/skills/bulk", s.addListSkills)
	mux.HandleFunc("PATCH /api/v2/lists/{id}/skills", s.updateListSkill)
	mux.HandleFunc("DELETE /api/v2/lists/{id}/skills", s.removeListSkill)
	mux.HandleFunc("POST /api/v2/lists/{id}/delete", s.deleteListConfirmed)
	mux.HandleFunc("POST /api/v2/lists/{id}/members", s.addListMember)
	mux.HandleFunc("PATCH /api/v2/lists/{id}/members", s.updateListMember)
	mux.HandleFunc("DELETE /api/v2/lists/{id}/members", s.removeListMember)
	mux.HandleFunc("POST /api/v2/lists/{id}/transfer", s.transferList)
	mux.HandleFunc("POST /api/v2/lists/{id}/add-to-shelf", s.requireWriter(s.addListToShelf))
	mux.HandleFunc("GET /api/v2/public/lists/{slug}", s.publicList)
	mux.HandleFunc("GET /api/community", s.community)
	mux.HandleFunc("GET /api/v2/groups", s.groups)
	mux.HandleFunc("POST /api/v2/groups", s.requireWriter(s.createGroup))
	mux.HandleFunc("PATCH /api/v2/groups/{id}", s.requireWriter(s.updateGroup))
	mux.HandleFunc("POST /api/v2/groups/{id}/members", s.requireWriter(s.groupMember))
	mux.HandleFunc("PATCH /api/admin/settings", s.requireOwner(s.adminSettings))
	mux.HandleFunc("GET /api/v2/settings/yolo", s.requireWriter(s.yoloSettingGet))
	mux.HandleFunc("PUT /api/v2/settings/yolo", s.requireAdmin(s.yoloSettingPut))
	mux.HandleFunc("GET /api/v2/maintenance", s.requireWriter(s.maintenanceGet))
	mux.HandleFunc("PUT /api/v2/maintenance/worktrees", s.requireAdmin(s.maintenancePut))
	mux.HandleFunc("POST /api/v2/maintenance/worktrees/scan", s.terminal(s.maintenanceWorktreeScan))
	mux.HandleFunc("POST /api/v2/maintenance/worktrees/clean", s.terminal(s.maintenanceWorktreeClean))
	mux.HandleFunc("GET /api/v2/settings/branch-prune", s.requireWriter(s.branchPruneSettingGet))
	mux.HandleFunc("PUT /api/v2/settings/branch-prune", s.requireAdmin(s.branchPruneSettingPut))
	mux.HandleFunc("GET /api/export", s.requireWriter(s.export))
	mux.HandleFunc("POST /api/restore", s.requireWriter(s.restore))
	s.registerBackupRoutes(mux)
	s.registerFenceRoutes(mux)
	s.registerProfileRoutes(mux)
	s.registerGafferRoutes(mux)
	s.registerGafferOutcomeRoutes(mux)
	s.registerGafferTemplateRoutes(mux)
	s.registerGafferToolRoutes(mux)
	s.registerGafferChatRoutes(mux)
	s.registerModelFavouriteRoutes(mux)
	s.registerMCPRoutes(mux)
	s.registerHostedRoutes(mux)
	s.registerPrivateSkillRoutes(mux)
	s.registerMailRoutes(mux)
	s.registerTicketRoutes(mux)
	s.registerNotificationRoutes(mux)
	s.registerPushRoutes(mux)
	s.registerHealthRoutes(mux)
	s.registerTerminalRoutes(mux)
	s.registerBrowseRoutes(mux)
	s.registerModelRoutes(mux)
	s.registerAccountRoutes(mux)
	s.registerUsageRoutes(mux)
	s.registerSkillBuddy(mux)
	s.registerSoftwareRoutes(mux)
	s.registerProjectRoutes(mux)
	s.registerFolderProjectRoutes(mux)
	s.registerDeskRoutes(mux)
	s.registerSessionBacklogRoutes(mux)
	s.registerProjectRunRoutes(mux)
	mux.HandleFunc("GET /quick", s.requireWriter(s.quickConfirm))
	mux.HandleFunc("POST /quick", s.requireWriter(s.quick))
	return compressJSON(mux)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// logRequests records one line per write, never the query string (which can carry a token).
func (s *server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readMethod := r.Method == http.MethodGet || r.Method == http.MethodHead
		if readMethod && r.URL.Path != "/quick" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		source := "ui"
		switch {
		case r.URL.Path == "/quick":
			source = "quick"
		case r.Header.Get("X-Reet-Source") == "scan":
			source = "scan"
		}
		log.Printf("request method=%s path=%q status=%d duration=%s source=%s",
			r.Method, r.URL.Path, status, time.Since(start).Round(time.Millisecond), source)
	})
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	n, err := s.st.Health(ctx)
	if err != nil {
		log.Printf("health: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "version": s.version, "skills": n})
}

func (s *server) buildInfo(w http.ResponseWriter, r *http.Request) {
	doc := map[string]any{"version": s.version, "commit": s.commit}
	if f := featureList(); f != nil {
		doc["features"] = f
	}
	writeJSON(w, 200, doc)
}

func guardedPath(path string) bool {
	return strings.HasPrefix(path, "/api/") || path == "/quick"
}

func openAuthPath(path string) bool {
	switch path {
	case "/api/version", syncproof.Path, fenceLeftPath, "/api/v2/auth/status", "/api/v2/auth/setup", "/api/v2/auth/signup", "/api/v2/auth/sign-in", "/api/v2/auth/sign-out", "/api/v2/auth/invite", "/api/v2/auth/join", "/api/v2/auth/forgot", "/api/v2/auth/reset",
		"/api/v2/auth/oidc/start", "/api/v2/auth/oidc/callback", ssoLogoutPath:
		return true
	}
	return path == "/api/community" || strings.HasPrefix(path, "/api/v2/public/lists/")
}

func bootstrapPath(path string) bool {
	return path == "/api/import"
}

// requestToken reads the token from a header. Only /quick, which a
// bookmarklet opens as a plain link, may carry it as ?t= instead: a token in
// any other address would end up in logs, history and Referer headers.
func requestToken(r *http.Request) string {
	got := r.Header.Get("X-Reet-Token")
	if b, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		got = b
	}
	if got == "" && r.URL.Path == "/quick" {
		got = r.URL.Query().Get("t")
	}
	return got
}

// knownHost reports whether a Host header names this instance: an IP
// address, a name only a local network can give (one word such as
// "homeserver", or one ending .local, .lan, .internal, .home.arpa or
// .localhost), or the host of REET_PUBLIC_URL. Any other name may be a
// stranger's domain pointed at this machine (DNS rebinding).
func (s *server) knownHost(host string) bool {
	name := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		name = h
	}
	name = strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(name, "["), "]"), "."))
	if name == "" {
		return false
	}
	if net.ParseIP(name) != nil || !strings.Contains(name, ".") {
		return true
	}
	for _, suffix := range []string{".local", ".lan", ".internal", ".home.arpa", ".localhost"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	u, err := url.Parse(s.publicOrigin())
	return err == nil && u.Hostname() != "" && strings.EqualFold(u.Hostname(), name)
}

func (s *server) hasBootstrap(r *http.Request) bool {
	return s.token != "" && subtle.ConstantTimeCompare([]byte(requestToken(r)), []byte(s.token)) == 1
}

// importHeader marks an import upload. A form can send multipart/form-data
// from any website without asking first, but it cannot add a header of its
// own, so requiring this one keeps the upload from being forged cross-site.
const importHeader = "X-Reet-Import"

// backupImportUpload is the multipart form an owner sends to stage a sealed
// copy. It is not JSON, so it is exempt from the JSON check, on this path
// and with the marker header.
func backupImportUpload(r *http.Request) bool {
	return r.Method == http.MethodPost && r.URL.Path == "/api/v2/admin/backups/import" &&
		strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") &&
		r.Header.Get(importHeader) != ""
}

func (s *server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiPath := strings.HasPrefix(r.URL.Path, "/api/")
		writeMethod := r.Method != http.MethodGet && r.Method != http.MethodHead
		if apiPath && writeMethod {
			// Stops other websites from writing to a token-less instance through the visitor's browser.
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				writeErr(w, http.StatusForbidden, "cross-site requests are not allowed")
				return
			}
			// An identity provider's logout call is a form post, by its standard;
			// it is exempt on its exact path and nowhere else. An image upload is
			// the one other non-JSON write: its own handler requires an exact
			// image Content-Type and the terminal wrapper checks the origin.
			writeMethod := r.Method == http.MethodPost || r.Method == http.MethodPatch
			jsonType := strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json")
			exempt := r.URL.Path == ssoLogoutPath || terminalUploadPath(r.URL.Path) || backupImportUpload(r)
			needsJSON := writeMethod && !jsonType
			if needsJSON && !exempt {
				writeErr(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
				return
			}
		}
		// With no sign-in at all, the Host header is the only thing that tells
		// this instance from a website whose name was pointed at it.
		tokenless := s.authMode == AuthNone || (s.authMode == AuthToken && s.token == "")
		if tokenless && guardedPath(r.URL.Path) && !s.knownHost(r.Host) {
			writeErr(w, http.StatusForbidden, "this address is not recognised; set REET_PUBLIC_URL to the address you open reet at")
			return
		}
		switch s.authMode {
		case AuthNone:
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, s.defaultUser)))
			return
		case AuthToken:
			requiresToken := s.token != "" && !openAuthPath(r.URL.Path)
			guarded := guardedPath(r.URL.Path) && !s.hasBootstrap(r)
			if requiresToken && guarded {
				writeErr(w, http.StatusUnauthorized, "missing or wrong token")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, s.defaultUser)))
			return
		case AuthAccounts:
			if tok := requestToken(r); guardedPath(r.URL.Path) && strings.HasPrefix(tok, "reet_mcp_") {
				if r.Header.Get("Authorization") != "Bearer "+tok {
					writeErr(w, 401, "MCP credentials require a bearer authorization header")
					return
				}
				_, ok := s.mcpCredential(tok, s.oauthBase(r)+"/mcp")
				if !ok {
					writeErr(w, 401, "MCP credential is paused, expired, revoked or invalid")
					return
				}
				writeErr(w, 403, "MCP credentials are valid only at the /mcp endpoint; use a browser session to maintain access")
				return
			}
			if openAuthPath(r.URL.Path) || !guardedPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			if tok := requestToken(r); tok != "" {
				if u, ok, _ := s.st.APITokenUser(auth.HashToken(tok), time.Now().UnixMilli()); ok {
					next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, u)))
					return
				}
			}
			if s.hasBootstrap(r) && bootstrapPath(r.URL.Path) {
				users, err := s.st.UserCount()
				if err != nil || users == 0 {
					writeErr(w, http.StatusUnauthorized, "setup required")
					return
				}
				owner, ok, err := s.st.OwnerUser()
				if err != nil || !ok {
					writeErr(w, http.StatusUnauthorized, "the owner account is missing")
					return
				}
				if active, _ := s.st.HasActiveAPITokens(owner.ID); active {
					writeErr(w, http.StatusUnauthorized, "bootstrap token disabled; create a personal API token")
					return
				}
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, owner)))
				return
			}
			if u, ok := s.sessionUser(r); ok {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, u)))
				return
			}
			users, err := s.st.UserCount()
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "could not check accounts")
				return
			}
			if users == 0 {
				writeErr(w, http.StatusUnauthorized, "setup required")
				return
			}
			writeErr(w, http.StatusUnauthorized, "sign in required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
