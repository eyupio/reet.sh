// Package api exposes the JSON API and the bookmarklet endpoint.
package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/backup"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
)

type server struct {
	importMu     sync.Mutex
	st           *store.Store
	f            meta.Fetcher
	token        string
	version      string
	commit       string
	debug        bool
	topics       bool
	refreshJob   bool
	authMode     AuthMode
	limiter      *auth.Limiter
	dummyHash    string
	oauthLimiter *auth.Limiter
	publicURL    string
	defaultUser  store.User
	queue        chan ref.Ref
	backups      *backup.Service
}

// AuthMode selects how the instance authenticates requests.
type AuthMode string

const (
	AuthToken    AuthMode = "token"
	AuthAccounts AuthMode = "accounts"
	AuthNone     AuthMode = "none"
)

const (
	sessionCookie = "reet_session"
	sessionTTL    = 30 * 24 * time.Hour
)

type userContextKey struct{}

// Option configures the server.
type Option func(*server)

// WithPublicURL sets the canonical origin used by MCP OAuth discovery behind a proxy.
func WithPublicURL(origin string) Option {
	return func(s *server) { s.publicURL = strings.TrimRight(origin, "/") }
}

// WithVersion sets the build version and commit reported by /api/version and /healthz.
func WithVersion(version, commit string) Option {
	return func(s *server) {
		if version != "" {
			s.version = version
		}
		if commit != "" {
			s.commit = commit
		}
	}
}

// WithDebug enables per-fetch debug logging.
func WithDebug(debug bool) Option {
	return func(s *server) { s.debug = debug }
}

// WithTopics enables the daily skills.sh topic seed.
func WithTopics(enabled bool) Option {
	return func(s *server) { s.topics = enabled }
}

// WithRefresh enables the six-hourly metadata refresh.
func WithRefresh(enabled bool) Option {
	return func(s *server) { s.refreshJob = enabled }
}

// WithAuthMode selects token, accounts or no authentication.
func WithAuthMode(mode AuthMode) Option {
	return func(s *server) { s.authMode = mode }
}

// WithBackups gives the API the backup service. Without one the backup pages
// answer that no key is configured.
func WithBackups(service *backup.Service) Option {
	return func(s *server) { s.backups = service }
}

// New returns the API handler. An empty token disables authentication.
func New(st *store.Store, f meta.Fetcher, token string, opts ...Option) http.Handler {
	dummy, _ := auth.HashPassword("not-a-real-password")
	s := &server{
		st: st, f: f, token: token, version: "dev", commit: "unknown",
		authMode: AuthToken, limiter: auth.NewLimiter(5, time.Minute), oauthLimiter: auth.NewLimiter(30, time.Minute), dummyHash: dummy,
		queue: make(chan ref.Ref, 256),
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.authMode == AuthToken || s.authMode == AuthNone {
		if id, err := st.DefaultOwner(); err == nil {
			if u, ok, err := st.UserByID(id); err == nil && ok {
				s.defaultUser = u
			}
		}
	}
	go s.worker()
	if s.topics {
		go s.topicWorker()
	}
	if s.refreshJob {
		go s.refreshWorker()
	}
	mux := http.NewServeMux()
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
	mux.HandleFunc("DELETE /api/skill/ignore", s.requireWriter(s.unignore))
	mux.HandleFunc("POST /api/import", s.requireWriter(s.importSkills))
	mux.HandleFunc("GET /api/environments", s.environments)
	mux.HandleFunc("GET /api/v2/sync", s.syncState)
	mux.HandleFunc("POST /api/v2/machines/merge", s.requireWriter(s.mergeMachines))
	mux.HandleFunc("DELETE /api/v2/machines", s.requireWriter(s.forgetMachine))
	mux.HandleFunc("PUT /api/v2/sync", s.requireWriter(s.saveSync))
	mux.HandleFunc("DELETE /api/v2/sync", s.requireWriter(s.deleteSync))
	mux.HandleFunc("DELETE /api/v2/environments", s.requireWriter(s.forgetEnvironment))
	mux.HandleFunc("GET /api/tags", s.tags)
	mux.HandleFunc("PATCH /api/tag", s.requireAdmin(s.renameTag))
	mux.HandleFunc("POST /api/tags/merge", s.requireAdmin(s.mergeTags))
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("GET /api/ignored", s.ignored)
	mux.HandleFunc("GET /api/admin/status", s.requireAdmin(s.adminStatus))
	mux.HandleFunc("GET /api/admin/rollup", s.requireAdmin(s.rollup))
	mux.HandleFunc("GET /api/v2/auth/status", s.authStatus)
	mux.HandleFunc("POST /api/v2/auth/setup", s.setup)
	mux.HandleFunc("POST /api/v2/auth/sign-in", s.signIn)
	mux.HandleFunc("POST /api/v2/auth/sign-out", s.signOut)
	mux.HandleFunc("GET /api/v2/auth/me", s.me)
	mux.HandleFunc("GET /api/v2/auth/invite", s.inviteInfo)
	mux.HandleFunc("POST /api/v2/auth/join", s.join)
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
	mux.HandleFunc("PATCH /api/v2/lists/{id}/skills", s.updateListSkill)
	mux.HandleFunc("DELETE /api/v2/lists/{id}/skills", s.removeListSkill)
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
	mux.HandleFunc("GET /api/export", s.requireWriter(s.export))
	mux.HandleFunc("POST /api/restore", s.requireWriter(s.restore))
	s.registerBackupRoutes(mux)
	s.registerProfileRoutes(mux)
	s.registerMCPRoutes(mux)
	s.registerHostedRoutes(mux)
	mux.HandleFunc("GET /quick", s.requireWriter(s.quick))
	return s.logRequests(s.auth(mux))
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

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// logRequests records one line per write, never the query string (which can carry a token).
func (s *server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && r.URL.Path != "/quick" {
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
	writeJSON(w, 200, map[string]string{"version": s.version, "commit": s.commit})
}

func guardedPath(path string) bool {
	return strings.HasPrefix(path, "/api/") || path == "/quick"
}

func openAuthPath(path string) bool {
	switch path {
	case "/api/version", "/api/v2/auth/status", "/api/v2/auth/setup", "/api/v2/auth/sign-in", "/api/v2/auth/sign-out", "/api/v2/auth/invite", "/api/v2/auth/join":
		return true
	}
	return path == "/api/community" || strings.HasPrefix(path, "/api/v2/public/lists/")
}

func bootstrapPath(path string) bool {
	return path == "/api/import"
}

func requestToken(r *http.Request) string {
	got := r.Header.Get("X-Reet-Token")
	if b, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		got = b
	}
	if got == "" {
		got = r.URL.Query().Get("t")
	}
	return got
}

func (s *server) hasBootstrap(r *http.Request) bool {
	return s.token != "" && subtle.ConstantTimeCompare([]byte(requestToken(r)), []byte(s.token)) == 1
}

func (s *server) sessionUser(r *http.Request) (store.User, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return store.User{}, false
	}
	u, ok, err := s.st.SessionUser(auth.HashToken(c.Value), time.Now().UnixMilli())
	if err != nil {
		return store.User{}, false
	}
	return u, ok
}

func userFrom(r *http.Request) (store.User, bool) {
	u, ok := r.Context().Value(userContextKey{}).(store.User)
	return u, ok
}

func (s *server) currentUser(r *http.Request) store.User {
	if u, ok := userFrom(r); ok {
		return u
	}
	return s.defaultUser
}

func (s *server) uid(r *http.Request) int64 { return s.currentUser(r).ID }

func (s *server) role(r *http.Request) string {
	u := s.currentUser(r)
	if u.Role == "" {
		return "owner"
	}
	return u.Role
}

func (s *server) requireWriter(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.role(r) == "viewer" {
			writeErr(w, http.StatusForbidden, "this account is read-only")
			return
		}
		next(w, r)
	}
}

func (s *server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if role := s.role(r); role != "owner" && role != "admin" {
			writeErr(w, http.StatusForbidden, "admins only")
			return
		}
		next(w, r)
	}
}

func (s *server) requireOwner(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.role(r) != "owner" {
			writeErr(w, http.StatusForbidden, "the owner only")
			return
		}
		next(w, r)
	}
}

func (s *server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != http.MethodGet && r.Method != http.MethodHead {
			// Stops other websites from writing to a token-less instance through the visitor's browser.
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				writeErr(w, http.StatusForbidden, "cross-site requests are not allowed")
				return
			}
			if (r.Method == http.MethodPost || r.Method == http.MethodPatch) &&
				!strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
				writeErr(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
				return
			}
		}
		switch s.authMode {
		case AuthNone:
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, s.defaultUser)))
			return
		case AuthToken:
			if s.token != "" && !openAuthPath(r.URL.Path) && guardedPath(r.URL.Path) && !s.hasBootstrap(r) {
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

func (s *server) setSession(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds()),
	})
}

func (s *server) clearSession(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

func (s *server) newSession(w http.ResponseWriter, r *http.Request, user store.User) (store.User, error) {
	token, err := auth.NewToken()
	if err != nil {
		return user, err
	}
	expires := time.Now().Add(sessionTTL)
	if err := s.st.CreateSession(auth.HashToken(token), user.ID, expires.UnixMilli(), r.UserAgent()); err != nil {
		return user, err
	}
	s.setSession(w, r, token, expires)
	return user, nil
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *server) worker() {
	for r := range s.queue {
		s.fetchInto(r)
	}
}

func (s *server) fetchInto(r ref.Ref) bool {
	if r.SourceType == "hosted" {
		_, username, _ := strings.Cut(r.Source, "/")
		sk, err := s.st.PublishedHostedSkill(username, r.Slug)
		return err == nil && s.st.SetMeta(r.ID, sk.Slug, sk.Description, 0, 0, nil) == nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m := s.f.Fetch(ctx, r)
	if m.Empty() {
		_ = s.st.SetSetting("meta_last_error", "metadata unavailable")
		if s.debug {
			log.Printf("meta fetch id=%s result=empty", r.ID)
		}
		return false
	}
	if err := s.st.SetMeta(r.ID, m.Name, m.Description, m.Installs, m.InstallsWeek, m.Trend); err != nil {
		_ = s.st.SetSetting("meta_last_error", err.Error())
		if s.debug {
			log.Printf("meta store id=%s error=%v", r.ID, err)
		}
		return false
	}
	_ = s.st.SetSetting("meta_last_ok_at", strconv.FormatInt(time.Now().UnixMilli(), 10))
	_ = s.st.SetSetting("meta_last_error", "")
	if s.debug {
		log.Printf("meta fetch id=%s name=%q installs=%d", r.ID, m.Name, m.Installs)
	}
	return true
}

func (s *server) enqueue(r ref.Ref) {
	select {
	case s.queue <- r:
	default:
	}
}

func (s *server) topicWorker() {
	s.syncTopics()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		s.syncTopics()
	}
}

func (s *server) syncTopics() {
	if v, _ := s.st.GetSetting("topics_enabled"); v == "0" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	topics, err := s.f.Topics(ctx)
	if err != nil {
		_ = s.st.SetSetting("topics_last_error", err.Error())
		if s.debug {
			log.Printf("topics fetch error=%v", err)
		}
		return
	}
	converted := make([]store.Topic, len(topics))
	for i, t := range topics {
		converted[i] = store.Topic{Slug: t.Slug, Name: t.Name, Skills: t.Skills}
	}
	if err := s.st.ReplaceTopicsForAll(converted); err != nil {
		_ = s.st.SetSetting("topics_last_error", err.Error())
		if s.debug {
			log.Printf("topics sync error=%v", err)
		}
		return
	}
	if s.debug {
		log.Printf("topics synced count=%d", len(converted))
	}
}

func (s *server) refreshWorker() {
	time.Sleep(10 * time.Second)
	s.refreshBatch()
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		s.refreshBatch()
	}
}

func (s *server) refreshBatch() {
	if v, _ := s.st.GetSetting("refresh_enabled"); v == "0" {
		return
	}
	ids, err := s.st.RefreshQueue(50)
	if err != nil {
		if s.debug {
			log.Printf("refresh queue error=%v", err)
		}
		return
	}
	failures := 0
	for _, id := range ids {
		rf, err := ref.Parse(id)
		if err != nil {
			continue
		}
		if s.fetchInto(rf) {
			failures = 0
		} else {
			_ = s.st.TouchFetched(id)
			failures++
			if failures >= 3 {
				if s.debug {
					log.Printf("refresh backing off after %d failures", failures)
				}
				time.Sleep(time.Hour)
				failures = 0
			}
		}
		time.Sleep(2 * time.Second)
	}
}

func (s *server) event(userID int64, source, action, skillID string, detail any) {
	if err := s.st.AddEventFor(userID, source, action, skillID, detail); err != nil && s.debug {
		log.Printf("event store action=%s error=%v", action, err)
	}
}

func stageLabel(sk store.Skill) string {
	if sk.Used {
		return "In use"
	}
	if sk.Bookmarked {
		return "To test"
	}
	return "On the shelf"
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func filterFromRequest(r *http.Request) store.Filter {
	q := r.URL.Query()
	f := store.Filter{
		Tab:     q.Get("tab"),
		Query:   q.Get("q"),
		Sort:    q.Get("sort"),
		Env:     q.Get("env"),
		Machine: q.Get("machine"),
		Only:    q.Get("only"),
		Stage:   q.Get("stage"),
	}
	for _, raw := range q["tag"] {
		for _, part := range strings.Split(raw, ",") {
			if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil {
				f.Tags = append(f.Tags, id)
			}
		}
	}
	return f
}

func (s *server) list(w http.ResponseWriter, r *http.Request) {
	skills, counts, err := s.st.ListFor(s.uid(r), filterFromRequest(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"skills": skills, "counts": counts})
}

func (s *server) add(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Input string `json:"input"`
		store.Patch
	}
	if !decode(w, r, &in) {
		return
	}
	rf, err := s.parseSkillInput(r, in.Input)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	_, existed, err := s.st.GetFor(s.uid(r), rf.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = s.st.UnignoreFor(s.uid(r), rf.ID)
	sk, err := s.st.UpsertFor(s.uid(r), rf, in.Patch)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.enqueue(rf)
	s.event(s.uid(r), "ui", "add", sk.ID, map[string]any{"name": sk.Name, "created": !existed})
	writeJSON(w, 200, struct {
		store.Skill
		Created bool `json:"created"`
	}{sk, !existed})
}

func (s *server) patch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
		store.Patch
	}
	if !decode(w, r, &in) {
		return
	}
	old, _, _ := s.st.GetFor(s.uid(r), in.ID)
	sk, err := s.st.PatchFor(s.uid(r), in.ID, in.Patch)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "skill not found")
		return
	} else if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	action, detail := patchEvent(old, sk, in.Patch)
	s.event(s.uid(r), "ui", action, sk.ID, detail)
	writeJSON(w, 200, sk)
}

func patchEvent(old, updated store.Skill, p store.Patch) (string, map[string]any) {
	detail := map[string]any{"name": updated.Name}
	switch {
	case p.Notes != nil && p.Liked == nil && p.Bookmarked == nil && p.Used == nil:
		return "notes", detail
	case p.Liked != nil && p.Bookmarked == nil && p.Used == nil:
		if *p.Liked {
			return "like", detail
		}
		return "unlike", detail
	case p.Bookmarked != nil || p.Used != nil:
		detail["from"] = stageLabel(old)
		detail["to"] = stageLabel(updated)
		return "stage", detail
	}
	return "patch", detail
}

func (s *server) remove(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeErr(w, 400, "id required")
		return
	}
	sk, _, _ := s.st.GetFor(s.uid(r), id)
	if r.URL.Query().Get("ignore") == "1" {
		if err := s.st.IgnoreFor(s.uid(r), id, sk.Name, "removed"); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		s.event(s.uid(r), "ui", "ignore", id, map[string]any{"name": sk.Name})
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := s.st.RemoveFor(s.uid(r), id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.event(s.uid(r), "ui", "remove", id, map[string]any{"name": sk.Name})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) refresh(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &in) {
		return
	}
	before, _, _ := s.st.GetFor(s.uid(r), in.ID)
	rf, err := ref.Parse(in.ID)
	if before.SourceType == "hosted" {
		rf, err = ref.ParseAt(before.URL, r.Host)
	}
	if err != nil {
		writeErr(w, 400, "skill has no skills.sh source to refresh from")
		return
	}
	refreshed := s.fetchInto(rf)
	sk, ok, err := s.st.GetFor(s.uid(r), in.ID)
	if err != nil || !ok {
		writeErr(w, 404, "skill not found")
		return
	}
	changed := refreshed && metaChanged(before, sk)
	s.event(s.uid(r), "ui", "refresh", sk.ID, map[string]any{"name": sk.Name, "changed": changed, "ok": refreshed})
	writeJSON(w, 200, struct {
		store.Skill
		Changed   bool `json:"changed"`
		Refreshed bool `json:"refreshed"`
	}{sk, changed, refreshed})
}

func metaChanged(a, b store.Skill) bool {
	if a.Name != b.Name || a.Description != b.Description || a.Installs != b.Installs || a.InstallsWeek != b.InstallsWeek {
		return true
	}
	if len(a.Trend) != len(b.Trend) {
		return true
	}
	for i := range a.Trend {
		if a.Trend[i] != b.Trend[i] {
			return true
		}
	}
	return false
}

func (s *server) resolve(w http.ResponseWriter, r *http.Request) {
	var in struct{ ID, Input string }
	if !decode(w, r, &in) {
		return
	}
	rf, err := s.parseSkillInput(r, in.Input)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	sk, err := s.st.ResolveFor(s.uid(r), in.ID, rf)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "skill not found")
		return
	} else if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.enqueue(rf)
	s.event(s.uid(r), "ui", "link", sk.ID, map[string]any{"name": sk.Name, "from": in.ID})
	writeJSON(w, 200, sk)
}

type importItem struct {
	Name    string `json:"name"`
	Source  string `json:"source"`
	Host    string `json:"host"`
	Agent   string `json:"agent"`
	Project string `json:"project"`
	Path    string `json:"path"`
}

// importEntry marks one installed skill as used, keeping existing flags and notes.
func (s *server) importEntry(userID int64, item importItem, request *http.Request) (string, bool, bool, error) {
	name, source := strings.TrimSpace(item.Name), strings.TrimSpace(item.Source)
	if name == "" {
		return "", false, false, errors.New("empty name")
	}
	var rf ref.Ref
	local := false
	if source != "" {
		source = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(source, "https://github.com/"), "http://github.com/"), ".git")
		if r, err := ref.Parse(source + "/" + name); err == nil {
			rf = r
		}
	}
	if rf.ID == "" {
		if r, err := s.parseSkillInput(request, name); err == nil {
			rf = r
		} else if strings.ContainsAny(name, "/\\ \t") {
			return "", false, false, fmt.Errorf("can't tell what %q is", name)
		} else {
			local = true
		}
	}
	id := rf.ID
	if local {
		id = "local/" + name
	}
	if ignored, err := s.st.IsIgnoredFor(userID, id); err != nil || ignored {
		return id, false, ignored, err
	}
	_, existed, err := s.st.GetFor(userID, id)
	if err != nil {
		return id, false, false, err
	}
	used := true
	flags := store.Patch{Used: &used}
	var sk store.Skill
	if local {
		sk, err = s.st.UpsertLocalFor(userID, name, flags)
	} else {
		sk, err = s.st.UpsertFor(userID, rf, flags)
	}
	if err != nil {
		return id, false, false, err
	}
	if !local {
		s.enqueue(rf)
	}
	if item.Host != "" && item.Agent != "" {
		sourcePath := item.Path
		if sourcePath != "" {
			sourcePath = strings.TrimRight(sourcePath, "/") + "/" + name
		}
		if err := s.st.RecordEnvironmentFor(userID, sk.ID, item.Host, item.Agent, item.Project, item.Path, sourcePath); err != nil {
			return id, false, false, err
		}
	}
	return sk.ID, !existed, false, nil
}

func (s *server) importSkills(w http.ResponseWriter, r *http.Request) {
	s.importMu.Lock()
	defer s.importMu.Unlock()
	var in struct {
		Machine      store.MachineInfo `json:"machine"`
		Items        []importItem      `json:"items"`
		Text         string            `json:"text"`
		Prune        bool              `json:"prune"`
		Environments []struct {
			Host    string   `json:"host"`
			Agent   string   `json:"agent"`
			Project string   `json:"project"`
			Path    string   `json:"path"`
			Skills  []string `json:"skills"`
		} `json:"environments"`
	}
	if !decode(w, r, &in) {
		return
	}
	// Resolve legacy names once before IDs, pruning and inventories are computed.
	aliases, err := s.st.MachineAliasesFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	canonicalize := func(host string) string {
		host = strings.TrimSpace(host)
		if canonical, ok := aliases[strings.ToLower(host)]; ok {
			return canonical
		}
		return host
	}
	in.Machine.Host = canonicalize(in.Machine.Host)
	for i := range in.Items {
		in.Items[i].Host = canonicalize(in.Items[i].Host)
	}
	for i := range in.Environments {
		in.Environments[i].Host = canonicalize(in.Environments[i].Host)
	}
	imported, created, ignoredCount := 0, 0, 0
	errs := []string{}
	// A reference snapshot must never capture a scan midway through its writes.
	for _, env := range in.Environments {
		if err := s.st.TouchEnvironmentFor(s.uid(r), env.Host, env.Agent, env.Project, env.Path); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if err := s.st.RecordInventoryFor(s.uid(r), store.EnvironmentIDFor(s.uid(r), env.Host, env.Agent, env.Project), false, in.Machine); err != nil {
			errs = append(errs, err.Error())
		}
	}

	for _, item := range in.Items {
		if item.Host != "" && item.Agent != "" {
			if err := s.st.TouchEnvironmentFor(s.uid(r), item.Host, item.Agent, item.Project, item.Path); err != nil {
				errs = append(errs, err.Error())
				continue
			}
			if err := s.st.RecordInventoryFor(s.uid(r), store.EnvironmentIDFor(s.uid(r), item.Host, item.Agent, item.Project), false, in.Machine); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	seen := map[string]bool{}
	itemIDs := map[string]string{}
	handle := func(item importItem) {
		id, wasCreated, wasIgnored, err := s.importEntry(s.uid(r), item, r)
		switch {
		case err != nil:
			errs = append(errs, err.Error())
		case wasIgnored:
			ignoredCount++
		default:
			if !seen[id] {
				seen[id] = true
				imported++
				if wasCreated {
					created++
				}
			}
		}
		if id != "" && err == nil && !wasIgnored {
			key := store.EnvironmentIDFor(s.uid(r), item.Host, item.Agent, item.Project) + "\x00" + item.Name
			itemIDs[key] = id
		}
	}
	for _, item := range in.Items {
		handle(item)
	}
	for _, line := range strings.Split(in.Text, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimLeft(line, "-*"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		handle(importItem{Name: line})
	}
	pruned := 0
	if in.Prune && len(errs) == 0 {
		for _, env := range in.Environments {
			if err := s.st.TouchEnvironmentFor(s.uid(r), env.Host, env.Agent, env.Project, env.Path); err != nil {
				errs = append(errs, err.Error())
				continue
			}
			envID := store.EnvironmentIDFor(s.uid(r), env.Host, env.Agent, env.Project)
			keep := []string{}
			for _, name := range env.Skills {
				if id, ok := itemIDs[envID+"\x00"+name]; ok {
					keep = append(keep, id)
				}
			}
			n, err := s.st.PruneEnvironmentFor(s.uid(r), envID, keep)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			pruned += int(n)
		}
	}
	if in.Machine.Host != "" {
		if err := s.st.RegisterMachineFor(s.uid(r), in.Machine); err != nil {
			errs = append(errs, err.Error())
		}
	}
	for _, env := range in.Environments {
		if err := s.st.TouchEnvironmentFor(s.uid(r), env.Host, env.Agent, env.Project, env.Path); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if err := s.st.RecordInventoryFor(s.uid(r), store.EnvironmentIDFor(s.uid(r), env.Host, env.Agent, env.Project), in.Prune && len(errs) == 0, in.Machine); err != nil {
			errs = append(errs, err.Error())
		}
	}
	source, action := "import", "import"
	if r.Header.Get("X-Reet-Source") == "scan" {
		source, action = "scan", "scan"
	}
	s.event(s.uid(r), source, action, "", map[string]any{"imported": imported, "created": created, "ignored": ignoredCount, "pruned": pruned})
	writeJSON(w, 200, map[string]any{"imported": imported, "created": created, "ignored": ignoredCount, "pruned": pruned, "errors": errs, "user": s.currentUser(r).Name})
}

func (s *server) environments(w http.ResponseWriter, r *http.Request) {
	envs, err := s.st.SyncEnvironmentsFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	machines, err := s.st.GroupMachinesFor(s.uid(r), envs)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"environments": envs, "machines": machines})
}

func (s *server) tags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.st.ListTagsFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"tags": tags})
}

func (s *server) skillTags(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID     string   `json:"id"`
		Add    []string `json:"add"`
		Remove []int64  `json:"remove"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.ID == "" {
		writeErr(w, 400, "id required")
		return
	}
	if len(in.Add) > 0 {
		if err := s.st.AddSkillTagsFor(s.uid(r), in.ID, in.Add); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	for _, id := range in.Remove {
		if err := s.st.RemoveSkillTagFor(s.uid(r), in.ID, id); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	sk, ok, err := s.st.GetFor(s.uid(r), in.ID)
	if err != nil || !ok {
		writeErr(w, 404, "skill not found")
		return
	}
	s.event(s.uid(r), "ui", "tag", sk.ID, map[string]any{"name": sk.Name, "added": in.Add, "removed": in.Remove})
	writeJSON(w, 200, sk)
}

func (s *server) installCopy(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &in) {
		return
	}
	sk, ok, err := s.st.GetFor(s.uid(r), in.ID)
	if err != nil || !ok {
		writeErr(w, 404, "skill not found")
		return
	}
	s.event(s.uid(r), "ui", "install-copy", sk.ID, map[string]any{"name": sk.Name})
	w.WriteHeader(http.StatusNoContent)
}

func validIDs(ids []string) bool {
	if len(ids) == 0 || len(ids) > 500 {
		return false
	}
	for _, id := range ids {
		if id == "" {
			return false
		}
	}
	return true
}

func bulkAction(p store.Patch) (string, map[string]any) {
	detail := map[string]any{}
	switch {
	case p.Notes != nil:
		return "bulk-patch", detail
	case p.Liked != nil && p.Bookmarked == nil && p.Used == nil:
		detail["liked"] = *p.Liked
		return "bulk-like", detail
	case p.Bookmarked != nil || p.Used != nil:
		to := "On the shelf"
		if p.Used != nil && *p.Used {
			to = "In use"
		} else if p.Bookmarked != nil && *p.Bookmarked {
			to = "To test"
		}
		detail["to"] = to
		return "bulk-stage", detail
	}
	return "bulk-patch", detail
}

func (s *server) patchMany(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string `json:"ids"`
		store.Patch
	}
	if !decode(w, r, &in) {
		return
	}
	if !validIDs(in.IDs) {
		writeErr(w, 400, "choose between 1 and 500 skills")
		return
	}
	n, err := s.st.PatchManyFor(s.uid(r), in.IDs, in.Patch)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	action, detail := bulkAction(in.Patch)
	detail["count"] = n
	s.event(s.uid(r), "ui", action, "", detail)
	writeJSON(w, 200, map[string]int{"updated": n})
}

func (s *server) tagMany(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs    []string `json:"ids"`
		Add    []string `json:"add"`
		Remove []int64  `json:"remove"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !validIDs(in.IDs) {
		writeErr(w, 400, "choose between 1 and 500 skills")
		return
	}
	n, err := s.st.TagManyFor(s.uid(r), in.IDs, in.Add, in.Remove)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.event(s.uid(r), "ui", "bulk-tag", "", map[string]any{"count": n, "added": in.Add, "removed": in.Remove})
	writeJSON(w, 200, map[string]int{"tagged": n})
}

func (s *server) removeMany(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs    []string `json:"ids"`
		Ignore bool     `json:"ignore"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !validIDs(in.IDs) {
		writeErr(w, 400, "choose between 1 and 500 skills")
		return
	}
	n, err := s.st.RemoveManyFor(s.uid(r), in.IDs, in.Ignore)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	action := "bulk-remove"
	if in.Ignore {
		action = "bulk-ignore"
	}
	s.event(s.uid(r), "ui", action, "", map[string]any{"count": n})
	writeJSON(w, 200, map[string]int{"removed": n})
}

func (s *server) renameTag(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.RenameTag(in.ID, in.Name); errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "tag not found")
		return
	} else if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, 409, "that tag already exists")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	s.event(s.uid(r), "ui", "tag-rename", "", map[string]any{"name": in.Name})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *server) mergeTags(w http.ResponseWriter, r *http.Request) {
	var in struct {
		From int64 `json:"from"`
		Into int64 `json:"into"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.MergeTags(in.From, in.Into); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.event(s.uid(r), "ui", "tag-merge", "", map[string]any{"from": in.From, "into": in.Into})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *server) events(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 20
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 100 {
		limit = 100
	}
	events, err := s.st.ListEventsFor(s.uid(r), limit, q.Get("skill"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"events": events})
}

func (s *server) ignored(w http.ResponseWriter, r *http.Request) {
	items, err := s.st.ListIgnoredFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ignored": items})
}

func (s *server) unignore(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeErr(w, 400, "id required")
		return
	}
	name := id
	if items, err := s.st.ListIgnoredFor(s.uid(r)); err == nil {
		for _, item := range items {
			if item.ID == id {
				name = item.Name
				break
			}
		}
	}
	if err := s.st.UnignoreFor(s.uid(r), id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.event(s.uid(r), "ui", "unignore", id, map[string]any{"name": name})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) authStatus(w http.ResponseWriter, r *http.Request) {
	mode := string(s.authMode)
	setupRequired, bootstrapRequired := false, false
	var user *store.User
	switch s.authMode {
	case AuthToken:
		if s.token == "" {
			mode = string(AuthNone)
		}
	case AuthAccounts:
		n, err := s.st.UserCount()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "could not check accounts")
			return
		}
		setupRequired = n == 0
		bootstrapRequired = setupRequired && s.token != ""
		if u, ok := s.sessionUser(r); ok {
			user = &u
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":              mode,
		"setupRequired":     setupRequired,
		"bootstrapRequired": bootstrapRequired,
		"user":              user,
	})
}

func (s *server) setup(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	n, err := s.st.UserCount()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not check accounts")
		return
	}
	if n > 0 {
		writeErr(w, http.StatusConflict, "this instance is already set up")
		return
	}
	if s.token != "" && !s.hasBootstrap(r) {
		writeErr(w, http.StatusUnauthorized, "the bootstrap token is required")
		return
	}
	var in struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !strings.Contains(email, "@") || len(email) > 254 {
		writeErr(w, http.StatusBadRequest, "enter a valid email address")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = strings.Split(email, "@")[0]
	}
	if len(name) > 100 {
		writeErr(w, http.StatusBadRequest, "name is too long")
		return
	}
	if len(in.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if len(in.Password) > 1024 {
		writeErr(w, http.StatusBadRequest, "password is too long")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create the account")
		return
	}
	user, created, err := s.st.CreateFirstOwner(email, name, "owner", hash)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !created {
		writeErr(w, http.StatusConflict, "this instance is already set up")
		return
	}
	if err := s.st.MigrateToOwner(user.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not migrate the shelf")
		return
	}
	if _, err := s.newSession(w, r, user); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not start a session")
		return
	}
	s.event(user.ID, "ui", "setup", "", map[string]any{"name": user.Name, "email": user.Email})
	writeJSON(w, http.StatusOK, user)
}

func (s *server) signIn(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	keys := []string{clientIP(r), email}
	if !s.limiter.Allow(keys...) {
		w.Header().Set("Retry-After", "60")
		writeErr(w, http.StatusTooManyRequests, "too many attempts, try again in a minute")
		return
	}
	s.limiter.Hit(keys...)
	user, ok, err := s.st.UserByEmail(email)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		_ = auth.VerifyPassword(s.dummyHash, in.Password)
		writeErr(w, http.StatusUnauthorized, "email or password is not correct")
		return
	}
	if user.DisabledAt != 0 || !auth.VerifyPassword(user.PasswordHash, in.Password) {
		writeErr(w, http.StatusUnauthorized, "email or password is not correct")
		return
	}
	if _, err := s.newSession(w, r, user); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not start a session")
		return
	}
	s.event(user.ID, "ui", "sign-in", "", map[string]any{"name": user.Name, "email": user.Email})
	writeJSON(w, http.StatusOK, user)
}

func (s *server) signOut(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		hash := auth.HashToken(c.Value)
		if user, ok, _ := s.st.SessionUser(hash, time.Now().UnixMilli()); ok {
			s.event(user.ID, "ui", "sign-out", "", map[string]any{"name": user.Name, "email": user.Email})
		}
		_ = s.st.DeleteSession(hash)
	}
	s.clearSession(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	if user, ok := userFrom(r); ok {
		writeJSON(w, http.StatusOK, user)
		return
	}
	if s.authMode == AuthNone {
		writeJSON(w, http.StatusOK, map[string]any{"name": "", "role": "owner", "implicit": true})
		return
	}
	writeErr(w, http.StatusUnauthorized, "sign in required")
}

func (s *server) listTokens(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	tokens, err := s.st.ListAPITokens(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"tokens": tokens})
}

func (s *server) createToken(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	raw, err := auth.NewToken()
	if err != nil {
		writeErr(w, 500, "could not create a token")
		return
	}
	token := "reet_" + raw
	tok, err := s.st.CreateAPIToken(s.uid(r), in.Name, auth.HashToken(token))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.event(s.uid(r), "ui", "token-create", "", map[string]any{"name": tok.Name})
	writeJSON(w, 200, map[string]any{"id": tok.ID, "name": tok.Name, "token": token, "createdAt": tok.CreatedAt})
}

func (s *server) revokeToken(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil || id == 0 {
		writeErr(w, 400, "id required")
		return
	}
	if err := s.st.RevokeAPIToken(s.uid(r), id); errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "token not found")
		return
	} else if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.event(s.uid(r), "ui", "token-revoke", "", map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) adminUsers(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	users, err := s.st.ListUsersMeta()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"users": users})
}

func (s *server) adminTarget(w http.ResponseWriter, r *http.Request, id int64) (store.User, bool) {
	target, ok, err := s.st.UserByID(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return store.User{}, false
	}
	if !ok {
		writeErr(w, 404, "user not found")
		return store.User{}, false
	}
	actor := s.currentUser(r)
	if target.ID == actor.ID {
		writeErr(w, 400, "use another admin account to change this one")
		return store.User{}, false
	}
	if target.Role == "owner" {
		writeErr(w, 403, "the owner account cannot be changed here")
		return store.User{}, false
	}
	if actor.Role == "admin" && target.Role == "admin" {
		writeErr(w, 403, "only the owner can change an admin")
		return store.User{}, false
	}
	return target, true
}

func (s *server) adminUserUpdate(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	var in struct {
		ID       int64   `json:"id"`
		Role     *string `json:"role"`
		Disabled *bool   `json:"disabled"`
	}
	if !decode(w, r, &in) {
		return
	}
	target, ok := s.adminTarget(w, r, in.ID)
	if !ok {
		return
	}
	actor := s.currentUser(r)
	if in.Role != nil {
		role := *in.Role
		if role == "owner" {
			writeErr(w, 400, "use transfer ownership")
			return
		}
		if role != "admin" && role != "member" && role != "viewer" {
			writeErr(w, 400, "invalid role")
			return
		}
		if role == "admin" && actor.Role != "owner" {
			writeErr(w, 403, "only the owner can make an admin")
			return
		}
		if err := s.st.UpdateUserRole(target.ID, role); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		s.event(actor.ID, "ui", "user-role", "", map[string]any{"name": target.Name, "email": target.Email, "role": role})
	}
	if in.Disabled != nil {
		if err := s.st.SetUserDisabled(target.ID, *in.Disabled); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if *in.Disabled {
			_ = s.st.DeleteUserSessions(target.ID)
			_ = s.st.RevokeUserTokens(target.ID)
		}
		s.event(actor.ID, "ui", "user-disable", "", map[string]any{"name": target.Name, "email": target.Email, "disabled": *in.Disabled})
	}
	s.adminUsers(w, r)
}

func (s *server) adminUserDelete(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil || id == 0 {
		writeErr(w, 400, "id required")
		return
	}
	target, ok := s.adminTarget(w, r, id)
	if !ok {
		return
	}
	if err := s.st.DeleteUser(target.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.event(s.uid(r), "ui", "user-delete", "", map[string]any{"name": target.Name, "email": target.Email})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) adminUserReset(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	var in struct {
		ID int64 `json:"id"`
	}
	if !decode(w, r, &in) {
		return
	}
	target, ok := s.adminTarget(w, r, in.ID)
	if !ok {
		return
	}
	raw, err := auth.NewToken()
	if err != nil {
		writeErr(w, 500, "could not reset the password")
		return
	}
	password := raw[:16]
	hash, err := auth.HashPassword(password)
	if err != nil {
		writeErr(w, 500, "could not reset the password")
		return
	}
	if err := s.st.SetUserPassword(target.ID, hash); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = s.st.RevokeUserTokens(target.ID)
	s.event(s.uid(r), "ui", "user-reset", "", map[string]any{"name": target.Name, "email": target.Email})
	writeJSON(w, 200, map[string]any{"password": password})
}

func (s *server) adminUserRevoke(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	var in struct {
		ID int64 `json:"id"`
	}
	if !decode(w, r, &in) {
		return
	}
	target, ok := s.adminTarget(w, r, in.ID)
	if !ok {
		return
	}
	if err := s.st.RevokeUserTokens(target.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.event(s.uid(r), "ui", "user-revoke", "", map[string]any{"name": target.Name, "email": target.Email})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) adminUserTransfer(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	var in struct {
		ID int64 `json:"id"`
	}
	if !decode(w, r, &in) {
		return
	}
	target, ok, err := s.st.UserByID(in.ID)
	if err != nil || !ok {
		writeErr(w, 404, "user not found")
		return
	}
	actor := s.currentUser(r)
	if target.ID == actor.ID {
		writeErr(w, 400, "already the owner")
		return
	}
	if err := s.st.TransferOwner(actor.ID, target.ID); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.event(actor.ID, "ui", "user-transfer", "", map[string]any{"name": target.Name, "email": target.Email})
	s.adminUsers(w, r)
}

func (s *server) adminInvite(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	var in struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !decode(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !strings.Contains(email, "@") || len(email) > 254 {
		writeErr(w, 400, "enter a valid email address")
		return
	}
	if _, exists, _ := s.st.UserByEmail(email); exists {
		writeErr(w, http.StatusConflict, "that email is already registered")
		return
	}
	role := in.Role
	if role == "" {
		role = "member"
	}
	if role != "admin" && role != "member" && role != "viewer" {
		writeErr(w, 400, "invalid role")
		return
	}
	actor := s.currentUser(r)
	if role == "admin" && actor.Role != "owner" {
		writeErr(w, 403, "only the owner can invite an admin")
		return
	}
	raw, err := auth.NewToken()
	if err != nil {
		writeErr(w, 500, "could not create an invite")
		return
	}
	expires := time.Now().Add(7 * 24 * time.Hour)
	if err := s.st.CreateInvite(auth.HashToken(raw), email, role, actor.ID, expires.UnixMilli()); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.event(actor.ID, "ui", "invite", "", map[string]any{"email": email, "role": role})
	writeJSON(w, 200, map[string]any{
		"url":       "/join?token=" + raw,
		"token":     raw,
		"email":     email,
		"role":      role,
		"expiresAt": expires.UnixMilli(),
	})
}

func (s *server) inviteInfo(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		writeErr(w, 400, "token required")
		return
	}
	invite, ok, err := s.st.InviteByHash(auth.HashToken(token), time.Now().UnixMilli())
	if err != nil || !ok {
		writeErr(w, 404, "invite not found or expired")
		return
	}
	writeJSON(w, 200, invite)
}

func (s *server) join(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	var in struct {
		Token    string `json:"token"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	invite, ok, err := s.st.InviteByHash(auth.HashToken(in.Token), time.Now().UnixMilli())
	if err != nil || !ok {
		writeErr(w, 404, "invite not found or expired")
		return
	}
	if len(in.Password) < 8 {
		writeErr(w, 400, "password must be at least 8 characters")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeErr(w, 500, "could not create the account")
		return
	}
	user, err := s.st.CreateUser(invite.Email, strings.TrimSpace(in.Name), invite.Role, hash)
	if errors.Is(err, store.ErrUserExists) {
		writeErr(w, 409, "that email is already registered")
		return
	} else if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = s.st.DeleteInvite(auth.HashToken(in.Token))
	if _, err := s.newSession(w, r, user); err != nil {
		writeErr(w, 500, "could not start a session")
		return
	}
	s.event(user.ID, "ui", "join", "", map[string]any{"name": user.Name, "email": user.Email})
	writeJSON(w, 200, user)
}

func settingEnabled(st *store.Store, key string) bool {
	v, _ := st.GetSetting(key)
	return v != "0"
}

func (s *server) probeSkillsSh(ctx context.Context) bool {
	if s.f.PageBase == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.f.PageBase, "/")+"/", nil)
	if err != nil {
		return false
	}
	client := s.f.Client
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode == http.StatusOK
}

func (s *server) adminStatus(w http.ResponseWriter, r *http.Request) {
	skills, counts, err := s.st.ListFor(s.uid(r), store.Filter{})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	stages := map[string]int{"use": 0, "try": 0, "shelf": 0}
	for _, sk := range skills {
		if sk.Used {
			stages["use"]++
		} else if sk.Bookmarked {
			stages["try"]++
		} else {
			stages["shelf"]++
		}
	}
	envs, err := s.st.ListEnvironmentsFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	events, err := s.st.ListAllEvents(100, "")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	size, _ := s.st.Size()
	topicsSynced, _ := s.st.GetSetting("topics_last_synced")
	topicsError, _ := s.st.GetSetting("topics_last_error")
	metaOK, _ := s.st.GetSetting("meta_last_ok_at")
	metaError, _ := s.st.GetSetting("meta_last_error")
	instanceName, _ := s.st.GetSetting("instance_name")
	writeJSON(w, 200, map[string]any{
		"version": s.version,
		"commit":  s.commit,
		"db": map[string]any{
			"path": filepath.Join(s.st.Path(), "reet.db"),
			"size": size,
		},
		"counts":        counts,
		"stages":        stages,
		"environments":  envs,
		"events":        events,
		"skillsSh":      s.probeSkillsSh(r.Context()),
		"metaLastOK":    metaOK,
		"metaLastError": metaError,
		"topics": map[string]any{
			"lastSynced": topicsSynced,
			"lastError":  topicsError,
		},
		"settings": map[string]any{
			"refreshEnabled":   settingEnabled(s.st, "refresh_enabled"),
			"topicsEnabled":    settingEnabled(s.st, "topics_enabled"),
			"linkListsEnabled": s.linkListsEnabled(),
			"rollupIndividual": s.rollupIndividual(),
			"instanceName":     instanceName,
		},
	})
}

func (s *server) adminSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RefreshEnabled   *bool   `json:"refreshEnabled"`
		TopicsEnabled    *bool   `json:"topicsEnabled"`
		LinkListsEnabled *bool   `json:"linkListsEnabled"`
		RollupIndividual *bool   `json:"rollupIndividual"`
		InstanceName     *string `json:"instanceName"`
	}
	if !decode(w, r, &in) {
		return
	}
	detail := map[string]any{}
	if in.RefreshEnabled != nil {
		value := "0"
		if *in.RefreshEnabled {
			value = "1"
		}
		if err := s.st.SetSetting("refresh_enabled", value); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		detail["refreshEnabled"] = *in.RefreshEnabled
	}
	if in.TopicsEnabled != nil {
		value := "0"
		if *in.TopicsEnabled {
			value = "1"
		}
		if err := s.st.SetSetting("topics_enabled", value); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		detail["topicsEnabled"] = *in.TopicsEnabled
	}
	if in.LinkListsEnabled != nil {
		value := "0"
		if *in.LinkListsEnabled {
			value = "1"
		}
		if err := s.st.SetSetting("link_lists_enabled", value); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		detail["linkListsEnabled"] = *in.LinkListsEnabled
	}
	if in.RollupIndividual != nil {
		value := "0"
		if *in.RollupIndividual {
			value = "1"
		}
		if err := s.st.SetSetting("rollup_individual", value); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		detail["rollupIndividual"] = *in.RollupIndividual
	}
	if in.InstanceName != nil {
		if err := s.st.SetSetting("instance_name", strings.TrimSpace(*in.InstanceName)); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		detail["instanceName"] = strings.TrimSpace(*in.InstanceName)
	}
	s.event(s.uid(r), "ui", "settings", "", detail)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *server) export(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	switch format {
	case "md", "markdown":
		skills, _, err := s.st.ListFor(s.uid(r), filterFromRequest(r))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		s.event(s.uid(r), "ui", "export", "", map[string]any{"format": "markdown"})
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="reet-shelf.md"`)
		w.Write([]byte(markdown(skills)))
		return
	case "csv":
		skills, _, err := s.st.ListFor(s.uid(r), filterFromRequest(r))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		s.event(s.uid(r), "ui", "export", "", map[string]any{"format": "csv"})
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="reet-shelf.csv"`)
		w.Write([]byte(csvExport(skills)))
		return
	}
	includeEvents := r.URL.Query().Get("events") != "0"
	snap, err := s.st.ExportFor(s.uid(r), includeEvents)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.event(s.uid(r), "ui", "export", "", map[string]any{"format": "json", "events": includeEvents})
	w.Header().Set("Content-Disposition", `attachment; filename="reet-export.json"`)
	writeJSON(w, 200, snap)
}

func installCommand(s store.Skill) string {
	var source string
	switch s.SourceType {
	case "github":
		source = "https://github.com/" + s.Source
	case "hosted":
		source = s.URL
	case "well-known":
		source = "https://" + s.Source
	default:
		return ""
	}
	return fmt.Sprintf("npx skills add %s --skill %s", source, s.Slug)
}

func markdown(skills []store.Skill) string {
	var b strings.Builder
	b.WriteString("# reet.sh shelf\n")
	groups := []struct {
		title string
		match func(store.Skill) bool
	}{
		{"In use", func(s store.Skill) bool { return s.Used }},
		{"To test", func(s store.Skill) bool { return !s.Used && s.Bookmarked }},
		{"On the shelf", func(s store.Skill) bool { return !s.Used && !s.Bookmarked }},
	}
	for _, group := range groups {
		var items []store.Skill
		for _, s := range skills {
			if group.match(s) {
				items = append(items, s)
			}
		}
		if len(items) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n\n", group.title)
		for _, s := range items {
			name := s.Name
			if name == "" {
				name = s.Slug
			}
			desc := strings.Join(strings.Fields(s.Description), " ")
			if s.URL != "" {
				fmt.Fprintf(&b, "- [%s](%s)", name, s.URL)
			} else {
				fmt.Fprintf(&b, "- %s", name)
			}
			if desc != "" {
				fmt.Fprintf(&b, " — %s", desc)
			}
			b.WriteString("\n")
			if cmd := installCommand(s); cmd != "" {
				fmt.Fprintf(&b, "  ```\n  %s\n  ```\n", cmd)
			}
		}
	}
	return b.String()
}

func csvExport(skills []store.Skill) string {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"id", "name", "stage", "source", "url", "installs", "installs_week", "liked", "bookmarked", "used", "notes", "tags", "environments"})
	for _, s := range skills {
		var tags, envs []string
		for _, t := range s.Tags {
			tags = append(tags, t.Name)
		}
		for _, e := range s.Environments {
			envs = append(envs, e.Name)
		}
		_ = w.Write([]string{
			s.ID, s.Name, stageLabel(s), s.Source, s.URL,
			strconv.FormatInt(s.Installs, 10), strconv.FormatInt(s.InstallsWeek, 10),
			strconv.FormatBool(s.Liked), strconv.FormatBool(s.Bookmarked), strconv.FormatBool(s.Used),
			s.Notes, strings.Join(tags, "; "), strings.Join(envs, "; "),
		})
	}
	w.Flush()
	return b.String()
}

func (s *server) restore(w http.ResponseWriter, r *http.Request) {
	var in store.Snapshot
	if !decode(w, r, &in) {
		return
	}
	if in.Skills == nil {
		writeErr(w, 400, "backup has no skills")
		return
	}
	if err := s.st.RestoreSnapshotFor(s.uid(r), in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.event(s.uid(r), "ui", "restore", "", map[string]any{"restored": len(in.Skills), "ignored": len(in.Ignored)})
	writeJSON(w, 200, map[string]int{"restored": len(in.Skills)})
}

func (s *server) quick(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	yes := true
	var p store.Patch
	var verb, alreadyVerb, actionName string
	switch q.Get("action") {
	case "like":
		p.Liked, verb, alreadyVerb, actionName = &yes, "Liked", "Already liked", "like"
	case "save":
		p.Bookmarked, verb, alreadyVerb, actionName = &yes, "Saved", "Already saved", "stage"
	case "used":
		p.Used, verb, alreadyVerb, actionName = &yes, "Marked as used", "Already marked as used", "stage"
	default:
		quickPage(w, 400, "Unknown action")
		return
	}
	rf, err := s.parseSkillInput(r, q.Get("url"))
	if err != nil {
		quickPage(w, 400, "Not a skills.sh skill page")
		return
	}
	old, found, err := s.st.GetFor(s.uid(r), rf.ID)
	if err != nil {
		quickPage(w, 500, "Could not save")
		return
	}
	already := found &&
		((p.Liked != nil && old.Liked) || (p.Bookmarked != nil && old.Bookmarked) || (p.Used != nil && old.Used))
	_ = s.st.UnignoreFor(s.uid(r), rf.ID)
	sk, err := s.st.UpsertFor(s.uid(r), rf, p)
	if err != nil {
		quickPage(w, 500, "Could not save")
		return
	}
	s.enqueue(rf)
	detail := map[string]any{"name": sk.Name, "already": already}
	if actionName == "stage" {
		detail["from"] = stageLabel(old)
		detail["to"] = stageLabel(sk)
	}
	s.event(s.uid(r), "quick", actionName, sk.ID, detail)
	msg := verb + ": " + sk.Name
	if already {
		msg = alreadyVerb + ": " + sk.Name
	}
	quickPage(w, 200, msg)
}

func quickPage(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintf(w, `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Bookmarklet · reet.sh</title>
<style>
:root{--bg:#f7f6f2;--card:#fffdf9;--ink:#1d1c1a;--muted:#5f5c54;--line:#e4e1d8;--accent:#c2410c;--accent-ink:#fff;color-scheme:light dark}
@media(prefers-color-scheme:dark){:root{--bg:#151413;--card:#1d1c1a;--ink:#efece6;--muted:#b2aca2;--line:#49453e;--accent:#fb923c;--accent-ink:#1a1208}}
*{box-sizing:border-box}body{margin:0;min-height:100dvh;padding:24px;display:grid;place-items:center;background:var(--bg);color:var(--ink);font:16px/1.55 system-ui,sans-serif}
main{width:100%%;max-width:440px;padding:32px;border:1px solid var(--line);border-radius:16px;background:var(--card)}
.eyebrow{margin:18px 0 0;color:var(--accent);font-size:11px;font-weight:700;letter-spacing:.12em;text-transform:uppercase}
h1{margin:12px 0;font:500 28px/1.2 Georgia,serif;letter-spacing:-.025em;overflow-wrap:anywhere}p{color:var(--muted);font-size:14px}
a{display:inline-flex;align-items:center;justify-content:center;min-height:44px;margin-top:12px;padding:10px 16px;border-radius:8px;background:var(--accent);color:var(--accent-ink);font-size:14px;font-weight:600;text-decoration:none}
a:focus-visible{outline:2px solid var(--accent);outline-offset:4px}
</style></head><body><main><img src="/favicon.svg" width="40" height="40" alt=""><p class="eyebrow">reet.sh · Bookmarklet</p><h1>%s</h1><p>This window will close automatically. You can also return to your shelf.</p><a href="/">Open your shelf</a></main>
<script>setTimeout(function(){window.close()},1500)</script></body></html>`, html.EscapeString(msg))
}
