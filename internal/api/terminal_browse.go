package api

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Folder browse and branch listing (docs/folder-browse.md): two read-only
// requests to one machine. The controller forwards them to a connection that
// reported the capability and keeps nothing from the answer; the only thing
// written is one folder-browse audit row per opened browser.

func (s *server) registerBrowseRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/machines/{agentId}/browse", s.terminal(s.sameOriginRead(s.machineBrowse)))
	mux.HandleFunc("GET /api/v2/machines/{agentId}/branches", s.terminal(s.sameOriginRead(s.machineBranches)))
}

var (
	// browseLimiter and branchesLimiter allow 20 browses and 10 listings per
	// 10 seconds per account and machine.
	browseLimit, branchesLimit = 20, 10
	// browseBrowsersPerUser bounds the browsers remembered per account (each
	// is one audit row); past it, and after browseBrowserIdle without a page,
	// a browser is forgotten. browsePagesCounted caps a row's page count.
	browseBrowsersPerUser = 32
	browseBrowserIdle     = 30 * time.Minute
	browsePagesCounted    = 1000
)

const (
	sentenceSlowDown        = "Slow down a moment, then try again."
	sentenceOutsideHome     = "That folder is outside the home folder reet was set up with."
	sentenceCantRead        = "reet can't read that folder."
	sentenceMachineTimeout  = "That machine didn't answer in time. Try again."
	sentenceMachineAnswer   = "That machine sent an answer reet didn't expect. Try again."
	sentenceMachineOffline  = "That machine is reconnecting. Try again in a moment."
	sentenceBranchesDenied  = "That machine isn't offering its branches right now. Check that terminal access is on there and git is installed."
	sentenceBrowseOff       = "That machine has folder browsing switched off."
	sentenceBrowseOld       = "Update reet on that machine to browse its folders."
	sentenceBranchesOld     = "Update reet on that machine to list its branches."
	sentenceBrowseBusy      = "That machine is already listing a folder. Try again in a moment."
	sentenceBranchesBusy    = "That machine is already listing branches. Try again in a moment."
	sentenceBranchesGit     = "Git couldn't list the branches there."
	sentenceBranchesNotRoot = "That folder isn't the top of a git repository."
)

// browseLimits holds the per-machine rate limits; Allow and Hit run under one
// lock so concurrent requests cannot both take the last slot.
type browseLimits struct {
	mu       sync.Mutex
	browse   *auth.Limiter
	branches *auth.Limiter
}

func (l *browseLimits) take(limiter func(*browseLimits) *auth.Limiter, key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim := limiter(l)
	if !lim.Allow(key) {
		return false
	}
	lim.Hit(key)
	return true
}

// browseVisit is one opened browser on one machine: its audit row and the
// pages the machine answered for it.
type browseVisit struct {
	id    int64
	path  string
	pages int
	last  time.Time
	used  uint64 // order of last use, for forgetting the least recently used
}

// browseAudit remembers the open browsers, so each writes one audit row. A
// browser is remembered only once its machine answered a page.
type browseAudit struct {
	mu    sync.Mutex
	users map[int64]map[string]*browseVisit
	clock uint64
}

func (s *server) browseState() (*browseLimits, *browseAudit) {
	s.browseOnce.Do(func() {
		window := 10 * time.Second
		s.browseLimits = &browseLimits{browse: auth.NewLimiter(browseLimit, window), branches: auth.NewLimiter(branchesLimit, window)}
		s.browseAudit = &browseAudit{users: map[int64]map[string]*browseVisit{}}
	})
	return s.browseLimits, s.browseAudit
}

// visit finds the browser named by nonce on agent, or starts it. Browsers
// idle for browseBrowserIdle are forgotten; at the bound the least recently
// used is forgotten (if it goes on, it starts a second audit row).
func (a *browseAudit) visit(uid int64, agent, nonce string, now time.Time) *browseVisit {
	visits := a.users[uid]
	if visits == nil {
		visits = map[string]*browseVisit{}
		a.users[uid] = visits
	}
	a.clock++
	key := agent + "/" + nonce
	if v := visits[key]; v != nil {
		v.last, v.used = now, a.clock
		return v
	}
	var oldest string
	for k, v := range visits {
		if now.Sub(v.last) > browseBrowserIdle {
			delete(visits, k)
		} else if oldest == "" || v.used < visits[oldest].used {
			oldest = k
		}
	}
	if len(visits) >= browseBrowsersPerUser && oldest != "" {
		delete(visits, oldest)
	}
	v := &browseVisit{last: now, used: a.clock}
	visits[key] = v
	return v
}

// page records one page the machine answered: the first writes the browser's
// audit row (machine, starting folder, pages), later ones update its count.
func (s *server) browsePage(uid int64, agent, nonce, start string) {
	_, a := s.browseState()
	a.mu.Lock()
	defer a.mu.Unlock()
	v := a.visit(uid, agent, nonce, time.Now())
	if v.pages >= browsePagesCounted {
		return
	}
	v.pages++
	if v.id == 0 {
		// Cleaned as the machine reads it; the home is "~".
		v.path = "~"
		if start != "" {
			v.path = path.Clean(start)
		}
	}
	detail := map[string]any{"machine": agent, "path": v.path, "pages": v.pages}
	var err error
	if v.id == 0 {
		v.id, err = s.st.AddEventIDFor(uid, "ui", "folder-browse", "", detail)
	} else {
		err = s.st.UpdateEventDetailFor(uid, v.id, "folder-browse", detail)
	}
	if err != nil && s.debug {
		log.Printf("folder-browse audit: %v", err)
	}
}

// sameOriginRead refuses a read that a browser marked as coming from another
// site or origin. A same-origin GET carries no Origin header; one that does
// must name the canonical origin.
func (s *server) sameOriginRead(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site := r.Header.Get("Sec-Fetch-Site")
		if site != "" && site != "same-origin" || r.Header.Get("Origin") != "" && !s.terminalOrigin(r) {
			writeErrCode(w, http.StatusForbidden, "origin_refused", "", "That request didn't come from this reet's own address.", "")
			return
		}
		next(w, r)
	}
}

// machineQuery reads a query with only the named parameters, each at most
// once.
func machineQuery(r *http.Request, names ...string) (url.Values, bool) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, false
	}
	for k, v := range q {
		known := false
		for _, n := range names {
			known = known || n == k
		}
		if !known || len(v) != 1 {
			return nil, false
		}
	}
	return q, true
}

func (s *server) machineBrowse(w http.ResponseWriter, r *http.Request) {
	q, ok := machineQuery(r, "path", "after", "filter", "hidden", "session")
	if !ok {
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	req := terminalproto.Browse{Path: q.Get("path"), After: q.Get("after"), Filter: q.Get("filter")}
	switch q.Get("hidden") {
	case "", "false":
	case "true":
		req.Hidden = true
	default:
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	agent, nonce := r.PathValue("agentId"), q.Get("session")
	if !terminalproto.ValidID(nonce) || !terminalproto.ValidID(agent) || terminalbroker.CheckBrowse(req) != nil {
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	uid := s.uid(r)
	// Only the account's own ready machine gets a rate-limit bucket; any
	// other id is answered at once as an offline machine.
	if !s.terminals.Ready(uid, agent) {
		s.machineRequestError(w, terminalbroker.ErrOffline, uid, agent)
		return
	}
	limits, _ := s.browseState()
	if !limits.take(func(l *browseLimits) *auth.Limiter { return l.browse }, strconv.FormatInt(uid, 10)+"/"+agent) {
		writeErrCode(w, http.StatusTooManyRequests, "browse_rate_limited", "", sentenceSlowDown, "")
		return
	}
	result, err := s.terminals.Browse(r.Context(), uid, agent, req)
	var refusal *terminalbroker.BrowseError
	if err == nil || errors.As(err, &refusal) {
		s.browsePage(uid, agent, nonce, req.Path)
	}
	if err != nil {
		s.machineRequestError(w, err, uid, agent)
		return
	}
	writeJSON(w, http.StatusOK, browseView(result))
}

func (s *server) machineBranches(w http.ResponseWriter, r *http.Request) {
	q, ok := machineQuery(r, "dir", "filter")
	if !ok {
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	req := terminalproto.Branches{Dir: q.Get("dir"), Filter: q.Get("filter")}
	agent := r.PathValue("agentId")
	if !terminalproto.ValidID(agent) || terminalbroker.CheckBranches(req) != nil {
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	uid := s.uid(r)
	if !s.terminals.Ready(uid, agent) {
		s.machineRequestError(w, terminalbroker.ErrOffline, uid, agent)
		return
	}
	limits, _ := s.browseState()
	if !limits.take(func(l *browseLimits) *auth.Limiter { return l.branches }, strconv.FormatInt(uid, 10)+"/"+agent) {
		writeErrCode(w, http.StatusTooManyRequests, "branches_rate_limited", "", sentenceSlowDown, "")
		return
	}
	result, err := s.terminals.Branches(r.Context(), uid, agent, req)
	if err != nil {
		s.machineRequestError(w, err, uid, agent)
		return
	}
	out := branchesJSON{Current: result.Current, Default: result.Default, Branches: result.Branches, More: result.More}
	if out.Branches == nil {
		out.Branches = []string{}
	}
	writeJSON(w, http.StatusOK, out)
}

type branchesJSON struct {
	Current  string   `json:"current"`
	Default  string   `json:"default"`
	Branches []string `json:"branches"`
	More     bool     `json:"more"`
}

type browseEntryJSON struct {
	Name     string `json:"name"`
	Dir      bool   `json:"dir"`
	RepoRoot bool   `json:"repoRoot"`
	Hidden   bool   `json:"hidden"`
}

type browseJSON struct {
	Path    string            `json:"path"`
	Home    string            `json:"home"`
	Parent  string            `json:"parent"`
	Repo    string            `json:"repo"`
	RepoTop string            `json:"repoTop"`
	Entries []browseEntryJSON `json:"entries"`
	More    bool              `json:"more"`
	Partial bool              `json:"partial"`
	Scanned int               `json:"scanned"`
	Omitted int               `json:"omitted"`
}

func browseView(r terminalproto.BrowseResult) browseJSON {
	out := browseJSON{Path: r.Path, Home: r.Home, Parent: r.Parent, Repo: string(r.Repo), RepoTop: r.RepoTop, Entries: []browseEntryJSON{},
		More: r.More, Partial: r.Partial, Scanned: r.Scanned, Omitted: r.Omitted}
	for _, e := range r.Entries {
		out.Entries = append(out.Entries, browseEntryJSON{Name: e.Name, Dir: e.Dir, RepoRoot: e.RepoRoot, Hidden: e.Hidden})
	}
	return out
}

// machineRequestError maps a browse or branches outcome to a code and one
// plain sentence. It never echoes the path or anything the machine sent.
func (s *server) machineRequestError(w http.ResponseWriter, err error, uid int64, agent string) {
	var browse *terminalbroker.BrowseError
	var branches *terminalbroker.BranchesError
	switch {
	case errors.As(err, &browse):
		switch browse.Reason {
		case terminalproto.BrowseNotFound, terminalproto.BrowseNotDir:
			host := s.machineHost(uid, agent)
			if host == "" {
				host = "that machine"
			}
			writeErrCode(w, http.StatusNotFound, string(browse.Reason), "path", "That folder isn't on "+host+".", "")
		case terminalproto.BrowseOutsideHome:
			writeErrCode(w, http.StatusUnprocessableEntity, "outside_home", "path", sentenceOutsideHome, "")
		case terminalproto.BrowseDenied:
			writeErrCode(w, http.StatusUnprocessableEntity, "denied", "path", sentenceCantRead, "")
		default:
			writeErrCode(w, http.StatusUnprocessableEntity, "unreadable", "path", sentenceCantRead, "")
		}
	case errors.As(err, &branches):
		switch branches.Reason {
		case terminalproto.BranchesNotRepoRoot:
			writeErrCode(w, http.StatusUnprocessableEntity, "not_repo_root", "dir", sentenceBranchesNotRoot, "")
		case terminalproto.BranchesOutsideHome:
			writeErrCode(w, http.StatusUnprocessableEntity, "outside_home", "dir", sentenceOutsideHome, "")
		case terminalproto.BranchesDenied:
			writeErrCode(w, http.StatusConflict, "branches_denied", "", sentenceBranchesDenied, "")
		default:
			writeErrCode(w, http.StatusUnprocessableEntity, "git_failed", "dir", sentenceBranchesGit, "")
		}
	case errors.Is(err, terminalbroker.ErrBrowseUnsupported):
		writeErrCode(w, http.StatusConflict, "browse_unsupported", "", sentenceBrowseOld, "")
	case errors.Is(err, terminalbroker.ErrBranchesUnsupported):
		writeErrCode(w, http.StatusConflict, "branches_unsupported", "", sentenceBranchesOld, "")
	case errors.Is(err, terminalbroker.ErrBrowseOff):
		writeErrCode(w, http.StatusConflict, "browse_off", "", sentenceBrowseOff, "")
	case errors.Is(err, terminalbroker.ErrBranchesUnavailable):
		writeErrCode(w, http.StatusConflict, "branches_denied", "", sentenceBranchesDenied, "")
	case errors.Is(err, terminalbroker.ErrBrowseBusy):
		writeErrCode(w, http.StatusConflict, "browse_busy", "", sentenceBrowseBusy, "")
	case errors.Is(err, terminalbroker.ErrBranchesBusy):
		writeErrCode(w, http.StatusConflict, "branches_busy", "", sentenceBranchesBusy, "")
	case errors.Is(err, terminalbroker.ErrMachineAnswer):
		writeErrCode(w, http.StatusBadGateway, "machine_error", "", sentenceMachineAnswer, "")
	case errors.Is(err, terminalbroker.ErrMachineTimeout):
		writeErrCode(w, http.StatusGatewayTimeout, "machine_timeout", "", sentenceMachineTimeout, "")
	case errors.Is(err, terminalproto.ErrInvalid):
		terminalError(w, err)
	default:
		// Offline, reconnecting, disconnected mid-request, or not this
		// account's machine: the readiness sentence, at once.
		writeErrCode(w, http.StatusServiceUnavailable, "machine_unavailable", "", sentenceMachineOffline, "")
	}
}
