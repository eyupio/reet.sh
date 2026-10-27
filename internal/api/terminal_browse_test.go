package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/privateconn"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	browseTestHome    = "/home/tester"
	readinessSentence = "That machine is reconnecting. Try again in a moment."
	slowDownSentence  = "Slow down a moment, then try again."
)

// browseAPI is a server whose store lives in a known folder, with machines
// connected in memory that answer browse and branches with answer (nil holds
// the request in held for a later reply).
type browseAPI struct {
	srv    *Server
	st     *store.Store
	dir    string
	uid    int64
	agent  string
	cookie string
	peers  map[string]*terminalbroker.Peer
	mu     sync.Mutex
	asked  []terminalproto.Body
	answer func(terminalproto.Body) terminalproto.Body
	held   []terminalproto.Message
}

func browseCaps() terminalproto.Capabilities {
	caps := worktreeCaps("claude")
	caps.Browse, caps.Branches = true, true
	return caps
}

func browseAPIFixture(t *testing.T, caps terminalproto.Capabilities) *browseAPI {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	uid, _ := st.DefaultOwner()
	if _, err := st.CreateAPIToken(uid, "owner", auth.HashToken("fixture-owner")); err != nil {
		t.Fatal(err)
	}
	srv := New(st, deadFetcher(t), "", WithAuthMode(AuthAccounts), WithRemoteTerminals(true), WithPublicURL("https://reet.example"))
	t.Cleanup(func() { srv.Close() })
	f := &browseAPI{srv: srv, st: st, dir: dir, uid: uid, cookie: terminalLogin(t, st, uid), peers: map[string]*terminalbroker.Peer{}}
	f.agent = f.addMachine(t, "studio", caps)
	f.answering(func(body terminalproto.Body) terminalproto.Body {
		if _, ok := body.(terminalproto.Browse); ok {
			return browseHomeListing()
		}
		return terminalproto.BranchesResult{Current: "main", Default: "main", Branches: []string{"main"}}
	})
	return f
}

// addMachine enrolls a machine on the account and connects it.
func (f *browseAPI) addMachine(t *testing.T, host string, caps terminalproto.Capabilities) string {
	t.Helper()
	agent, _, err := f.st.EnrollSyncAgent(f.uid, host, "test")
	if err != nil {
		t.Fatal(err)
	}
	record, err := f.st.BeginRemoteConnectionFor(f.uid, agent.ID, terminalbroker.ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	var p *terminalbroker.Peer
	p, err = f.srv.s.terminals.Connect(f.uid, agent.ID, record.Generation, true, func(ctx context.Context, m terminalproto.Message) error {
		reply := m
		switch op := m.Body.(type) {
		case terminalproto.Reconcile:
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
		case terminalproto.Browse, terminalproto.Branches:
			f.mu.Lock()
			f.asked = append(f.asked, op)
			answer := f.answer
			f.mu.Unlock()
			reply.Body = answer(op)
			if reply.Body == nil {
				f.mu.Lock()
				f.held = append(f.held, reply)
				f.mu.Unlock()
				return nil
			}
		default:
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
		}
		data, err := terminalproto.Encode(reply)
		if err != nil {
			return err
		}
		decoded, err := terminalproto.Decode(data)
		if err != nil {
			return err
		}
		return p.Receive(decoded)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.peers[agent.ID] = p
	return agent.ID
}

func (f *browseAPI) answering(fn func(terminalproto.Body) terminalproto.Body) {
	f.mu.Lock()
	f.answer = fn
	f.mu.Unlock()
}

func (f *browseAPI) sentRequests() []terminalproto.Body {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]terminalproto.Body(nil), f.asked...)
}

func (f *browseAPI) heldCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.held)
}

// get sends a same-origin browser GET (no Origin header, as browsers send it).
func (f *browseAPI) get(t *testing.T, cookie, path string, headers ...string) (int, map[string]any, string) {
	t.Helper()
	r := httptest.NewRequest("GET", path, nil)
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code == 200 && w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("a machine answer may be cached", w.Header())
	}
	return w.Code, out, w.Body.String()
}

func browsePath(agent, session string, q url.Values) string {
	if q == nil {
		q = url.Values{}
	}
	if session != "" {
		q.Set("session", session)
	}
	path := "/api/v2/machines/" + agent + "/browse"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return path
}

func branchesPath(agent, dir, filter string) string {
	q := url.Values{"dir": {dir}}
	if filter != "" {
		q.Set("filter", filter)
	}
	return "/api/v2/machines/" + agent + "/branches?" + q.Encode()
}

func browseHomeListing() terminalproto.BrowseResult {
	return terminalproto.BrowseResult{Path: browseTestHome, Home: browseTestHome, Repo: terminalproto.BrowseRepoNone,
		Entries: []terminalproto.BrowseEntry{{Name: "site", Dir: true, RepoRoot: true}, {Name: "work", Dir: true}}, Scanned: 4, Omitted: 2}
}

func browseNonce() string { return terminalbroker.ID()[:32] }

func browseEvents(t *testing.T, st *store.Store, uid int64) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, e := range worktreeEvents(t, st, uid, "folder-browse") {
		detail := map[string]any{}
		if err := json.Unmarshal([]byte(e.Detail), &detail); err != nil {
			t.Fatal(err)
		}
		out = append(out, detail)
	}
	return out
}

func TestBrowseRouteAccess(t *testing.T) {
	f := browseAPIFixture(t, browseCaps())
	viewer, err := f.st.CreateUser("browse-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	viewerCookie := terminalLogin(t, f.st, viewer.ID)
	routes := []string{browsePath(f.agent, browseNonce(), nil), branchesPath(f.agent, browseTestHome+"/site", "")}
	refused := func(name string, code int, raw string) {
		t.Helper()
		if code != 401 && code != 403 {
			t.Fatal(name, code, raw)
		}
	}
	for _, path := range routes {
		code, _, raw := f.get(t, viewerCookie, path)
		if code != 403 {
			t.Fatal("viewer", path, code, raw)
		}
		code, _, raw = f.get(t, "", path)
		refused("signed out", code, raw)
		code, _, raw = f.get(t, f.cookie, path, "Authorization", "Bearer fixture-owner")
		refused("API token", code, raw)
		code, _, raw = f.get(t, "", path, "Authorization", "Bearer fixture-owner")
		refused("API token alone", code, raw)
		code, _, raw = f.get(t, f.cookie, path, "Authorization", "Bearer reet_mcp_fake")
		refused("MCP token", code, raw)
		code, _, raw = f.get(t, f.cookie, path, "X-Reet-Token", "fixture-owner")
		refused("token header", code, raw)
		code, _, raw = f.get(t, f.cookie, path+"&t=fixture-owner")
		if code != 400 && code != 403 {
			t.Fatal("token in the URL", path, code, raw)
		}
		for _, h := range [][]string{
			{"Origin", "https://elsewhere.example"},
			{"Sec-Fetch-Site", "cross-site"},
			{"Sec-Fetch-Site", "same-site"},
			{"Origin", "null"},
		} {
			code, _, raw = f.get(t, f.cookie, path, h...)
			if code != 403 {
				t.Fatal("other origin", h, path, code, raw)
			}
		}
		// The positive control: the owner's own browser at the canonical origin.
		code, _, raw = f.get(t, f.cookie, path)
		if code != 200 {
			t.Fatal("owner", path, code, raw)
		}
		code, _, raw = f.get(t, f.cookie, path, "Origin", "https://reet.example")
		if code != 200 {
			t.Fatal("owner with its own origin", path, code, raw)
		}
	}
	if sent := f.sentRequests(); len(sent) != 4 {
		t.Fatal("a refused request reached the machine", len(sent), sent)
	}
	for _, mode := range []AuthMode{AuthNone, AuthToken} {
		f.srv.s.authMode = mode
		for _, path := range routes {
			code, _, raw := f.get(t, f.cookie, path)
			refused(fmt.Sprint("single-token or open mode ", mode), code, raw)
			code, _, raw = f.get(t, f.cookie, path, "Authorization", "Bearer fixture-owner")
			refused(fmt.Sprint("token in mode ", mode), code, raw)
		}
	}
	f.srv.s.authMode = AuthAccounts
	f.srv.SetRemoteTerminals(false)
	for _, path := range routes {
		if code, _, raw := f.get(t, f.cookie, path); code != 403 {
			t.Fatal("remote terminals off", path, code, raw)
		}
	}
	if sent := f.sentRequests(); len(sent) != 4 {
		t.Fatal("a refused request reached the machine", len(sent), sent)
	}
	if len(browseEvents(t, f.st, f.uid)) != 1 {
		t.Fatal("audit rows", browseEvents(t, f.st, f.uid))
	}
}

// Another account's machine is answered exactly as an offline one.
func TestBrowseIsScopedToTheOwner(t *testing.T) {
	f := browseAPIFixture(t, browseCaps())
	other, err := f.st.CreateUser("browse-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	otherCookie := terminalLogin(t, f.st, other.ID)
	for _, path := range []string{browsePath(f.agent, browseNonce(), nil), branchesPath(f.agent, browseTestHome+"/site", "")} {
		code, out, raw := f.get(t, otherCookie, path)
		if code != 503 || out["code"] != "machine_unavailable" || out["error"] != readinessSentence {
			t.Fatal(path, code, raw)
		}
	}
	if len(f.sentRequests()) != 0 {
		t.Fatal("another account reached the machine")
	}
}

func TestBrowseOnOldPeerAsksForUpdate(t *testing.T) {
	f := browseAPIFixture(t, worktreeCaps("claude"))
	code, out, raw := f.get(t, f.cookie, browsePath(f.agent, browseNonce(), nil))
	if code != 409 || out["code"] != "browse_unsupported" || out["error"] != "Update reet on that machine to browse its folders." {
		t.Fatal(code, raw)
	}
	code, out, raw = f.get(t, f.cookie, branchesPath(f.agent, browseTestHome+"/site", ""))
	if code != 409 || out["code"] != "branches_unsupported" || out["error"] != "Update reet on that machine to list its branches." {
		t.Fatal(code, raw)
	}
	// A current machine with folder browse switched off still lists branches.
	off := browseCaps()
	off.Browse, off.BrowseOff = false, true
	offAgent := f.addMachine(t, "laptop", off)
	code, out, raw = f.get(t, f.cookie, browsePath(offAgent, browseNonce(), nil))
	if code != 409 || out["code"] != "browse_off" || out["error"] != "That machine has folder browsing switched off." {
		t.Fatal("switched off", code, raw)
	}
	// One with browse off and no git is not told to update reet.
	offNoGit := off
	offNoGit.Branches = false
	offNoGitAgent := f.addMachine(t, "tablet", offNoGit)
	code, out, raw = f.get(t, f.cookie, browsePath(offNoGitAgent, browseNonce(), nil))
	if code != 409 || out["code"] != "browse_off" {
		t.Fatal("switched off, no git", code, raw)
	}
	code, out, raw = f.get(t, f.cookie, branchesPath(offNoGitAgent, browseTestHome+"/site", ""))
	if code != 409 || out["code"] != "branches_denied" {
		t.Fatal("switched off, no git, branches", code, raw)
	}
	noGit := browseCaps()
	noGit.Branches = false
	noGitAgent := f.addMachine(t, "desktop", noGit)
	code, out, raw = f.get(t, f.cookie, branchesPath(noGitAgent, browseTestHome+"/site", ""))
	if code != 409 || out["code"] != "branches_denied" || out["error"] != branchesDeniedSentence {
		t.Fatal("branches not offered", code, raw)
	}
	if sent := f.sentRequests(); len(sent) != 0 {
		t.Fatal("a request reached a machine without its capability", sent)
	}
	if len(browseEvents(t, f.st, f.uid)) != 0 {
		t.Fatal("a refused browse was audited")
	}
	// The old machine's connection is untouched.
	select {
	case <-f.peers[f.agent].Done():
		t.Fatal("a refused browse ended the old machine's connection")
	default:
	}
	if !f.srv.s.terminals.Ready(f.uid, f.agent) {
		t.Fatal("the old machine is no longer ready")
	}
}

const branchesDeniedSentence = "That machine isn't offering its branches right now. Check that terminal access is on there and git is installed."

func TestBrowseRateLimit(t *testing.T) {
	f := browseAPIFixture(t, browseCaps())
	second := f.addMachine(t, "laptop", browseCaps())
	nonce := browseNonce()
	for i := 0; i < 20; i++ {
		if code, _, raw := f.get(t, f.cookie, browsePath(f.agent, nonce, nil)); code != 200 {
			t.Fatal(i, code, raw)
		}
	}
	code, out, raw := f.get(t, f.cookie, browsePath(f.agent, nonce, nil))
	if code != 429 || out["code"] != "browse_rate_limited" || out["error"] != slowDownSentence {
		t.Fatal("21st browse", code, raw)
	}
	if len(f.sentRequests()) != 20 {
		t.Fatal("the 21st browse reached the machine", len(f.sentRequests()))
	}
	// The limit is per machine: another machine of the account still answers.
	if code, _, raw := f.get(t, f.cookie, browsePath(second, nonce, nil)); code != 200 {
		t.Fatal("other machine", code, raw)
	}
	// Branches have their own limit.
	if code, _, raw := f.get(t, f.cookie, branchesPath(f.agent, browseTestHome+"/site", "")); code != 200 {
		t.Fatal("branches after browses", code, raw)
	}
	pages := browseEvents(t, f.st, f.uid)
	if len(pages) != 2 {
		t.Fatal("audit", pages)
	}
}

func TestBrowseOfflineMachineAnswersAtOnce(t *testing.T) {
	f := browseAPIFixture(t, browseCaps())
	never, _, err := f.st.EnrollSyncAgent(f.uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	f.peers[f.agent].Close()
	for _, agent := range []string{f.agent, never.ID, "not-a-machine-of-this-account"} {
		for _, path := range []string{browsePath(agent, browseNonce(), nil), branchesPath(agent, browseTestHome+"/site", "")} {
			start := time.Now()
			code, out, raw := f.get(t, f.cookie, path)
			if code != 503 || out["code"] != "machine_unavailable" || out["error"] != readinessSentence {
				t.Fatal(path, code, raw)
			}
			if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
				t.Fatal("an offline machine took", elapsed)
			}
		}
	}
	if len(f.sentRequests()) != 0 || len(browseEvents(t, f.st, f.uid)) != 0 {
		t.Fatal("an offline browse was sent or audited")
	}
}

func TestBrowseMachineDisconnectsMidRequest(t *testing.T) {
	f := browseAPIFixture(t, browseCaps())
	f.answering(func(terminalproto.Body) terminalproto.Body { return nil })
	go func() {
		for f.heldCount() == 0 {
			time.Sleep(5 * time.Millisecond)
		}
		f.peers[f.agent].Close()
	}()
	start := time.Now()
	code, out, raw := f.get(t, f.cookie, browsePath(f.agent, browseNonce(), nil))
	if code != 503 || out["code"] != "machine_unavailable" || out["error"] != readinessSentence {
		t.Fatal(code, raw)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatal("the request in flight waited past the disconnect", elapsed)
	}
}

// A machine that never answers is given up on at the controller's deadline.
func TestBrowseMachineThatNeverAnswersTimesOut(t *testing.T) {
	ordinary, branches := terminalbroker.BrowseCallTimeout, terminalbroker.BranchesCallTimeout
	terminalbroker.BrowseCallTimeout, terminalbroker.BranchesCallTimeout = 150*time.Millisecond, 150*time.Millisecond
	t.Cleanup(func() { terminalbroker.BrowseCallTimeout, terminalbroker.BranchesCallTimeout = ordinary, branches })
	f := browseAPIFixture(t, browseCaps())
	f.answering(func(terminalproto.Body) terminalproto.Body { return nil })
	for _, path := range []string{browsePath(f.agent, browseNonce(), nil), branchesPath(f.agent, browseTestHome+"/site", "")} {
		code, out, raw := f.get(t, f.cookie, path)
		if code != 504 || out["code"] != "machine_timeout" || out["error"] != "That machine didn't answer in time. Try again." {
			t.Fatal(path, code, raw)
		}
	}
	if terminalbroker.BrowseCallTimeout != 150*time.Millisecond || ordinary != 6*time.Second {
		t.Fatal("the controller's browse deadline is", ordinary)
	}
}

func TestBrowseAnswersInTheDocumentedShape(t *testing.T) {
	f := browseAPIFixture(t, browseCaps())
	f.answering(func(body terminalproto.Body) terminalproto.Body {
		if req, ok := body.(terminalproto.Browse); ok && req.Path == browseTestHome+"/site/docs" {
			return terminalproto.BrowseResult{Path: req.Path, Home: browseTestHome, Parent: browseTestHome + "/site", Repo: terminalproto.BrowseRepoInside, RepoTop: browseTestHome + "/site",
				Entries: []terminalproto.BrowseEntry{{Name: ".cache", Dir: true, Hidden: true}}, More: true, Partial: true, Scanned: 9, Omitted: 3}
		}
		if _, ok := body.(terminalproto.Browse); ok {
			return terminalproto.BrowseResult{Path: browseTestHome, Home: browseTestHome, Repo: terminalproto.BrowseRepoNone}
		}
		return terminalproto.BranchesResult{}
	})
	q := url.Values{"path": {browseTestHome + "/site/docs"}, "after": {".a"}, "filter": {"ca"}, "hidden": {"true"}}
	code, _, raw := f.get(t, f.cookie, browsePath(f.agent, browseNonce(), q))
	want := `{"path":"/home/tester/site/docs","home":"/home/tester","parent":"/home/tester/site","repo":"inside","repoTop":"/home/tester/site","entries":[{"name":".cache","dir":true,"repoRoot":false,"hidden":true}],"more":true,"partial":true,"scanned":9,"omitted":3}`
	if code != 200 || strings.TrimSpace(raw) != want {
		t.Fatal(code, raw)
	}
	code, _, raw = f.get(t, f.cookie, browsePath(f.agent, browseNonce(), nil))
	if code != 200 || strings.TrimSpace(raw) != `{"path":"/home/tester","home":"/home/tester","parent":"","repo":"none","repoTop":"","entries":[],"more":false,"partial":false,"scanned":0,"omitted":0}` {
		t.Fatal("empty home", code, raw)
	}
	code, _, raw = f.get(t, f.cookie, branchesPath(f.agent, browseTestHome+"/site", ""))
	if code != 200 || strings.TrimSpace(raw) != `{"current":"","default":"","branches":[],"more":false}` {
		t.Fatal("no branches", code, raw)
	}
	sent := f.sentRequests()
	if len(sent) != 3 || sent[0] != (terminalproto.Browse{Path: browseTestHome + "/site/docs", After: ".a", Filter: "ca", Hidden: true}) || sent[1] != (terminalproto.Browse{}) || sent[2] != (terminalproto.Branches{Dir: browseTestHome + "/site"}) {
		t.Fatal("requests not forwarded exactly", sent)
	}
}

func TestBrowseRequestsAreStrict(t *testing.T) {
	f := browseAPIFixture(t, browseCaps())
	nonce := browseNonce()
	for name, path := range map[string]string{
		"no session":         browsePath(f.agent, "", nil),
		"short session":      browsePath(f.agent, "abc", nil),
		"session with a dot": browsePath(f.agent, strings.Repeat("a", 20)+".b", nil),
		"relative path":      browsePath(f.agent, nonce, url.Values{"path": {"work"}}),
		"path too long":      browsePath(f.agent, nonce, url.Values{"path": {"/" + strings.Repeat("a", terminalproto.MaxDirBytes)}}),
		"newline in path":    browsePath(f.agent, nonce, url.Values{"path": {browseTestHome + "/a\nb"}}),
		"long filter":        browsePath(f.agent, nonce, url.Values{"filter": {strings.Repeat("f", 65)}}),
		"long after":         browsePath(f.agent, nonce, url.Values{"after": {strings.Repeat("a", 256)}}),
		"hidden as a word":   browsePath(f.agent, nonce, url.Values{"hidden": {"yes"}}),
		"unknown parameter":  browsePath(f.agent, nonce, url.Values{"depth": {"2"}}),
		"repeated path":      browsePath(f.agent, nonce, url.Values{"path": {browseTestHome, browseTestHome + "/site"}}),
		"bad query":          browsePath(f.agent, nonce, nil) + "&%zz",
		"branches no dir":    "/api/v2/machines/" + f.agent + "/branches",
		"branches relative":  branchesPath(f.agent, "site", ""),
		"branches filter":    branchesPath(f.agent, browseTestHome+"/site", strings.Repeat("f", 65)),
		"branches session":   branchesPath(f.agent, browseTestHome+"/site", "") + "&session=" + nonce,
		"branches unknown":   branchesPath(f.agent, browseTestHome+"/site", "") + "&all=1",
	} {
		if code, _, raw := f.get(t, f.cookie, path); code != 400 {
			t.Fatal(name, code, raw)
		}
	}
	if len(f.sentRequests()) != 0 || len(browseEvents(t, f.st, f.uid)) != 0 {
		t.Fatal("an invalid request reached the machine or the audit", f.sentRequests())
	}
}

func TestBrowseRefusalsMapToSentences(t *testing.T) {
	f := browseAPIFixture(t, browseCaps())
	for _, tc := range []struct {
		reply    terminalproto.BrowseResult
		status   int
		code     string
		sentence string
	}{
		{terminalproto.BrowseResult{Reason: terminalproto.BrowseNotFound}, 404, "not_found", "That folder isn't on studio."},
		{terminalproto.BrowseResult{Reason: terminalproto.BrowseNotDir}, 404, "not_dir", "That folder isn't on studio."},
		{terminalproto.BrowseResult{Reason: terminalproto.BrowseOutsideHome}, 422, "outside_home", "That folder is outside the home folder reet was set up with."},
		{terminalproto.BrowseResult{Reason: terminalproto.BrowseDenied}, 422, "denied", "reet can't read that folder."},
		{terminalproto.BrowseResult{Reason: terminalproto.BrowseUnreadable}, 422, "unreadable", "reet can't read that folder."},
		{terminalproto.BrowseResult{Reason: terminalproto.BrowseConflict}, 409, "browse_busy", "That machine is already listing a folder. Try again in a moment."},
	} {
		f.answering(func(terminalproto.Body) terminalproto.Body { return tc.reply })
		code, out, raw := f.get(t, f.cookie, browsePath(f.agent, browseNonce(), url.Values{"path": {browseTestHome + "/gone"}}))
		if code != tc.status || out["code"] != tc.code || out["error"] != tc.sentence {
			t.Fatal(tc.reply.Reason, code, raw)
		}
	}
	for _, tc := range []struct {
		reply    terminalproto.BranchesResult
		status   int
		code     string
		sentence string
	}{
		{terminalproto.BranchesResult{Reason: terminalproto.BranchesNotRepoRoot}, 422, "not_repo_root", "That folder isn't the top of a git repository."},
		{terminalproto.BranchesResult{Reason: terminalproto.BranchesOutsideHome}, 422, "outside_home", "That folder is outside the home folder reet was set up with."},
		{terminalproto.BranchesResult{Reason: terminalproto.BranchesGitFailed}, 422, "git_failed", "Git couldn't list the branches there."},
		{terminalproto.BranchesResult{Reason: terminalproto.BranchesDenied}, 409, "branches_denied", branchesDeniedSentence},
		{terminalproto.BranchesResult{Reason: terminalproto.BranchesConflict}, 409, "branches_busy", "That machine is already listing branches. Try again in a moment."},
	} {
		f.answering(func(terminalproto.Body) terminalproto.Body { return tc.reply })
		code, out, raw := f.get(t, f.cookie, branchesPath(f.agent, browseTestHome+"/site", ""))
		if code != tc.status || out["code"] != tc.code || out["error"] != tc.sentence {
			t.Fatal(tc.reply.Reason, code, raw)
		}
	}
	// off is the machine's switch; denied is about the folder, even from a
	// machine whose switch went off while it answered.
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.BrowseResult{Reason: terminalproto.BrowseOff}
	})
	code, out, raw := f.get(t, f.cookie, browsePath(f.agent, browseNonce(), nil))
	if code != 409 || out["code"] != "browse_off" || out["error"] != "That machine has folder browsing switched off." {
		t.Fatal("switched off", code, raw)
	}
	off := browseCaps()
	off.Browse, off.BrowseOff = false, true
	record, _ := f.st.RemoteCapabilitiesFor(f.uid, f.agent)
	f.answering(func(terminalproto.Body) terminalproto.Body {
		if err := f.st.UpdateRemoteCapabilitiesFor(f.uid, f.agent, record.InstanceID, record.Generation, off); err != nil {
			t.Error(err)
		}
		return terminalproto.BrowseResult{Reason: terminalproto.BrowseDenied}
	})
	code, out, raw = f.get(t, f.cookie, browsePath(f.agent, browseNonce(), nil))
	if code != 422 || out["code"] != "denied" || out["error"] != "reet can't read that folder." {
		t.Fatal("denied", code, raw)
	}
}

// A reply of another kind than the request's is not "reconnecting": it gets
// a sentence of its own, and the connection stays.
func TestBrowseWrongKindAnswerKeepsTheConnection(t *testing.T) {
	f := browseAPIFixture(t, browseCaps())
	f.answering(func(body terminalproto.Body) terminalproto.Body {
		if _, ok := body.(terminalproto.Browse); ok {
			return terminalproto.BranchesResult{Branches: []string{"main"}}
		}
		return browseHomeListing()
	})
	for _, path := range []string{browsePath(f.agent, browseNonce(), nil), branchesPath(f.agent, browseTestHome+"/site", "")} {
		code, out, raw := f.get(t, f.cookie, path)
		if code != 502 || out["code"] != "machine_error" || out["error"] != "That machine sent an answer reet didn't expect. Try again." {
			t.Fatal(path, code, raw)
		}
	}
	select {
	case <-f.peers[f.agent].Done():
		t.Fatal("a wrong-kind answer ended the connection")
	default:
	}
	if len(browseEvents(t, f.st, f.uid)) != 0 {
		t.Fatal("a wrong-kind answer was audited as a page")
	}
}

// storeDump reads every row of every table in the store's database through a
// connection of its own.
func storeDump(t *testing.T, dir string) map[string][]string {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(dir, "reet.db")+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		tables = append(tables, name)
	}
	rows.Close()
	out := map[string][]string{}
	for _, table := range tables {
		rows, err := db.Query(`SELECT * FROM "` + table + `"`)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			values := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range values {
				ptrs[i] = &values[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			var b strings.Builder
			for i, v := range values {
				if raw, ok := v.([]byte); ok {
					v = string(raw)
				}
				fmt.Fprintf(&b, "%s=%v;", cols[i], v)
			}
			out[table] = append(out[table], b.String())
		}
		rows.Close()
		sort.Strings(out[table])
	}
	return out
}

// storeChanges lists the rows added to or removed from each table. Two
// changes are not the browse's and are left out: the signed-in session's
// last-seen time, which every request refreshes, and SQLite's own counter of
// event ids, which moves with the audit row.
func storeChanges(before, after map[string][]string) map[string][]string {
	changes := map[string][]string{}
	for table, rows := range after {
		if table == "sessions" {
			continue
		}
		if table == "sqlite_sequence" {
			rows = slices.DeleteFunc(slices.Clone(rows), func(r string) bool { return strings.HasPrefix(r, "name=events;") })
			before[table] = slices.DeleteFunc(slices.Clone(before[table]), func(r string) bool { return strings.HasPrefix(r, "name=events;") })
		}
		was := map[string]int{}
		for _, r := range before[table] {
			was[r]++
		}
		for _, r := range rows {
			if was[r] > 0 {
				was[r]--
				continue
			}
			changes[table] = append(changes[table], "+"+r)
		}
		for r, n := range was {
			for ; n > 0; n-- {
				changes[table] = append(changes[table], "-"+r)
			}
		}
	}
	return changes
}

// filesContain reports which files under dir hold any of the needles.
func filesContain(t *testing.T, dir string, needles ...string) []string {
	t.Helper()
	var found []string
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, _ := os.ReadFile(path)
		for _, n := range needles {
			if bytes.Contains(data, []byte(n)) {
				found = append(found, path+": "+n)
			}
		}
		return nil
	})
	return found
}

func TestBrowseStoresNothing(t *testing.T) {
	f := browseAPIFixture(t, browseCaps())
	secrets := []string{"zebra-secret-folder", "quokka-hidden-repo", "narwhal-branch-name", "okapi-filter-text"}
	f.answering(func(body terminalproto.Body) terminalproto.Body {
		switch req := body.(type) {
		case terminalproto.Browse:
			if req.Path == "" {
				return terminalproto.BrowseResult{Path: browseTestHome, Home: browseTestHome, Repo: terminalproto.BrowseRepoNone,
					Entries: []terminalproto.BrowseEntry{{Name: ".quokka-hidden-repo", Dir: true, RepoRoot: true, Hidden: true}, {Name: "zebra-secret-folder", Dir: true}}}
			}
			return terminalproto.BrowseResult{Path: req.Path, Home: browseTestHome, Parent: browseTestHome, Repo: terminalproto.BrowseRepoRoot, RepoTop: req.Path}
		case terminalproto.Branches:
			return terminalproto.BranchesResult{Current: "narwhal-branch-name", Branches: []string{"narwhal-branch-name"}}
		}
		return nil
	})
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	f.srv.s.debug = true
	// Let anything the fixture itself wrote settle before the snapshot.
	time.Sleep(50 * time.Millisecond)
	before := storeDump(t, f.dir)

	nonce := browseNonce()
	if code, _, raw := f.get(t, f.cookie, browsePath(f.agent, nonce, url.Values{"hidden": {"true"}})); code != 200 || !strings.Contains(raw, "zebra-secret-folder") {
		t.Fatal("home", code, raw)
	}
	repo := browseTestHome + "/.quokka-hidden-repo"
	if code, _, raw := f.get(t, f.cookie, browsePath(f.agent, nonce, url.Values{"path": {repo}, "filter": {"okapi-filter-text"}})); code != 200 {
		t.Fatal("enter", code, raw)
	}
	if code, _, raw := f.get(t, f.cookie, branchesPath(f.agent, repo, "okapi-filter-text")); code != 200 || !strings.Contains(raw, "narwhal-branch-name") {
		t.Fatal("branches", code, raw)
	}
	time.Sleep(50 * time.Millisecond)
	after := storeDump(t, f.dir)

	changes := storeChanges(before, after)
	events := changes["events"]
	delete(changes, "events")
	if len(changes) != 0 {
		t.Fatal("a browse wrote to the store", changes)
	}
	if len(events) != 1 || !strings.HasPrefix(events[0], "+") || !strings.Contains(events[0], "action=folder-browse;") {
		t.Fatal("audit rows written", events)
	}
	rows := browseEvents(t, f.st, f.uid)
	if len(rows) != 1 || len(rows[0]) != 3 || rows[0]["machine"] != f.agent || rows[0]["path"] != "~" || rows[0]["pages"] != 2.0 {
		t.Fatal("audit row", rows)
	}
	if found := filesContain(t, f.dir, secrets...); len(found) != 0 {
		t.Fatal("the store's files hold names from the machine", found)
	}
	for _, s := range secrets {
		if strings.Contains(logs.String(), s) {
			t.Fatal("a name from the machine was logged", s, logs.String())
		}
	}
}

func TestBrowseAuditOncePerBrowser(t *testing.T) {
	f := browseAPIFixture(t, browseCaps())
	first, second := browseNonce(), browseNonce()
	for i := 0; i < 3; i++ {
		if code, _, raw := f.get(t, f.cookie, browsePath(f.agent, first, url.Values{"path": {[]string{"", browseTestHome + "/work", browseTestHome + "/site"}[i]}})); code != 200 {
			t.Fatal(i, code, raw)
		}
	}
	rows := browseEvents(t, f.st, f.uid)
	if len(rows) != 1 || len(rows[0]) != 3 || rows[0]["machine"] != f.agent || rows[0]["path"] != "~" || rows[0]["pages"] != 3.0 {
		t.Fatal("one browser", rows)
	}
	// A second browser starting at a typed folder is its own row; a folder
	// the machine refused still counts as a page it answered.
	if code, _, raw := f.get(t, f.cookie, browsePath(f.agent, second, url.Values{"path": {browseTestHome + "/work"}})); code != 200 {
		t.Fatal(code, raw)
	}
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.BrowseResult{Reason: terminalproto.BrowseNotFound}
	})
	if code, _, raw := f.get(t, f.cookie, browsePath(f.agent, second, url.Values{"path": {browseTestHome + "/gone"}})); code != 404 {
		t.Fatal(code, raw)
	}
	// A busy machine answered no page.
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.BrowseResult{Reason: terminalproto.BrowseConflict}
	})
	if code, _, raw := f.get(t, f.cookie, browsePath(f.agent, second, nil)); code != 409 {
		t.Fatal(code, raw)
	}
	rows = browseEvents(t, f.st, f.uid)
	if len(rows) != 2 {
		t.Fatal("two browsers", rows)
	}
	byPath := map[any]map[string]any{}
	for _, r := range rows {
		byPath[r["path"]] = r
	}
	if byPath["~"]["pages"] != 3.0 || byPath[browseTestHome+"/work"]["pages"] != 2.0 {
		t.Fatal("pages", rows)
	}
	// The same nonce on another machine is another browser.
	other := f.addMachine(t, "laptop", browseCaps())
	f.answering(func(terminalproto.Body) terminalproto.Body { return browseHomeListing() })
	if code, _, raw := f.get(t, f.cookie, browsePath(other, first, nil)); code != 200 {
		t.Fatal(code, raw)
	}
	if rows = browseEvents(t, f.st, f.uid); len(rows) != 3 {
		t.Fatal("same nonce, other machine", rows)
	}
}

// Audit rows are bounded: a browser's page count stops at a cap, and an
// account can have only so many browsers open at once.
func TestBrowseAuditIsBounded(t *testing.T) {
	browsers, pages := browseBrowsersPerUser, browsePagesCounted
	browseBrowsersPerUser, browsePagesCounted = 3, 2
	t.Cleanup(func() { browseBrowsersPerUser, browsePagesCounted = browsers, pages })
	f := browseAPIFixture(t, browseCaps())
	page := func(nonce string) {
		t.Helper()
		if code, _, raw := f.get(t, f.cookie, browsePath(f.agent, nonce, nil)); code != 200 {
			t.Fatal(code, raw)
		}
	}
	first, second, third, fourth := browseNonce(), browseNonce(), browseNonce(), browseNonce()
	for i := 0; i < 4; i++ {
		page(first)
	}
	if rows := browseEvents(t, f.st, f.uid); len(rows) != 1 || rows[0]["pages"] != 2.0 {
		t.Fatal("pages past the cap", rows)
	}
	page(second)
	page(third)
	// A fourth browser is served: the least recently used one is forgotten.
	page(fourth)
	if rows := browseEvents(t, f.st, f.uid); len(rows) != 4 {
		t.Fatal("a fourth browser", rows)
	}
	// The forgotten browser starts a row of its own if it goes on (and
	// forgets the next least recently used, the second); the third is kept.
	page(first)
	page(third)
	rows := browseEvents(t, f.st, f.uid)
	if len(rows) != 5 || rows[0]["pages"] != 1.0 {
		t.Fatal("the forgotten browser", rows)
	}
	total := 0.0
	for _, r := range rows {
		total += r["pages"].(float64)
	}
	if total != 2+1+2+1+1 {
		t.Fatal("pages", rows)
	}
}

// A browser takes one of the account's slots only once its machine answered
// a page: requests that never reached a machine cannot lock browsing out.
func TestBrowseSlotsAreTakenOnlyByAnsweredPages(t *testing.T) {
	f := browseAPIFixture(t, browseCaps())
	offline, _, err := f.st.EnrollSyncAgent(f.uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < browseBrowsersPerUser+4; i++ {
		agent := fmt.Sprintf("not-a-machine-%08d", i)
		if i%2 == 1 {
			agent = offline.ID
		}
		if code, _, raw := f.get(t, f.cookie, browsePath(agent, browseNonce(), nil)); code != 503 {
			t.Fatal(i, code, raw)
		}
	}
	if code, _, raw := f.get(t, f.cookie, browsePath(f.agent, browseNonce(), nil)); code != 200 {
		t.Fatal("browsing after requests that reached no machine", code, raw)
	}
	// Answers that are not pages (busy, timed out) do not take a slot from
	// a browser that is open: with room for one, it keeps its row.
	browsers := browseBrowsersPerUser
	browseBrowsersPerUser = 1
	t.Cleanup(func() { browseBrowsersPerUser = browsers })
	open := browseNonce()
	if code, _, raw := f.get(t, f.cookie, browsePath(f.agent, open, nil)); code != 200 {
		t.Fatal(code, raw)
	}
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.BrowseResult{Reason: terminalproto.BrowseConflict}
	})
	for i := 0; i < 3; i++ {
		if code, _, raw := f.get(t, f.cookie, browsePath(f.agent, browseNonce(), nil)); code != 409 {
			t.Fatal(code, raw)
		}
	}
	f.answering(func(terminalproto.Body) terminalproto.Body { return browseHomeListing() })
	if code, _, raw := f.get(t, f.cookie, browsePath(f.agent, open, nil)); code != 200 {
		t.Fatal(code, raw)
	}
	rows := browseEvents(t, f.st, f.uid)
	if len(rows) != 2 || rows[0]["pages"] != 2.0 {
		t.Fatal("a busy answer took the open browser's slot", rows)
	}
}

// The audit row keeps the starting folder cleaned, as the machine reads it.
func TestBrowseAuditCleansTheStartingFolder(t *testing.T) {
	f := browseAPIFixture(t, browseCaps())
	if code, _, raw := f.get(t, f.cookie, browsePath(f.agent, browseNonce(), url.Values{"path": {browseTestHome + "/work/../site//./"}})); code != 200 {
		t.Fatal(code, raw)
	}
	if rows := browseEvents(t, f.st, f.uid); len(rows) != 1 || rows[0]["path"] != browseTestHome+"/site" {
		t.Fatal(rows)
	}
}

// Rate-limit buckets are made only for the account's own connected
// machines: other ids cannot make keys without end.
func TestBrowseRateLimitKeysOnlyTheAccountsMachines(t *testing.T) {
	f := browseAPIFixture(t, browseCaps())
	for i := 0; i < 50; i++ {
		agent := fmt.Sprintf("not-a-machine-%08d", i)
		if code, _, raw := f.get(t, f.cookie, browsePath(agent, browseNonce(), nil)); code != 503 {
			t.Fatal(code, raw)
		}
		if code, _, raw := f.get(t, f.cookie, branchesPath(agent, browseTestHome+"/site", "")); code != 503 {
			t.Fatal(code, raw)
		}
	}
	limits, _ := f.srv.s.browseState()
	if n, m := limits.browse.Len(), limits.branches.Len(); n != 0 || m != 0 {
		t.Fatal("rate-limit keys for machines that are not the account's", n, m)
	}
	f.get(t, f.cookie, browsePath(f.agent, browseNonce(), nil))
	f.get(t, f.cookie, branchesPath(f.agent, browseTestHome+"/site", ""))
	if n, m := limits.browse.Len(), limits.branches.Len(); n != 1 || m != 1 {
		t.Fatal("the account's own machine", n, m)
	}
}

func TestBranchesRouteAndRateLimit(t *testing.T) {
	f := browseAPIFixture(t, browseCaps())
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.BranchesResult{Current: "feature", Default: "main", Branches: []string{"feature", "main", "origin/main"}, More: true}
	})
	for i := 0; i < 10; i++ {
		code, _, raw := f.get(t, f.cookie, branchesPath(f.agent, browseTestHome+"/site", "a"))
		if code != 200 || strings.TrimSpace(raw) != `{"current":"feature","default":"main","branches":["feature","main","origin/main"],"more":true}` {
			t.Fatal(i, code, raw)
		}
	}
	code, out, raw := f.get(t, f.cookie, branchesPath(f.agent, browseTestHome+"/site", ""))
	if code != 429 || out["code"] != "branches_rate_limited" || out["error"] != slowDownSentence {
		t.Fatal("11th listing", code, raw)
	}
	if sent := f.sentRequests(); len(sent) != 10 || sent[0] != (terminalproto.Branches{Dir: browseTestHome + "/site", Filter: "a"}) {
		t.Fatal("sent", len(sent))
	}
	// Browse keeps its own allowance.
	f.answering(func(terminalproto.Body) terminalproto.Body { return browseHomeListing() })
	if code, _, raw := f.get(t, f.cookie, browsePath(f.agent, browseNonce(), nil)); code != 200 {
		t.Fatal("browse after listings", code, raw)
	}
	all, _ := f.st.ListEventsFor(f.uid, 100, "")
	for _, e := range all {
		if e.Action != "folder-browse" {
			t.Fatal("a branch listing was audited or stored", e)
		}
	}
}

// browseConnector dials the private control route as a connector that
// negotiated browse-v1 and branches-v1 and hands each browse or branches
// request to the test.
type browseConnector struct {
	c        *websocket.Conn
	requests chan terminalproto.Message
	write    func(terminalproto.Message) error
	closed   chan struct{}
}

func dialBrowseConnector(t *testing.T, srv *Server, agent, secret string, caps terminalproto.Capabilities) (*browseConnector, string) {
	t.Helper()
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.s.privateHandler().ServeHTTP(w, r.WithContext(privateconn.WithPrivateTransport(r.Context())))
	}))
	t.Cleanup(private.Close)
	ctx, cancel := context.WithCancel(context.Background())
	c, response, err := websocket.Dial(ctx, private.URL+"/private/v1/control", &websocket.DialOptions{HTTPHeader: http.Header{"X-Reet-Agent": {agent}, "X-Reet-Agent-Key": {secret}, "X-Reet-Version": {"test"}}})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	handshake, _ := terminalproto.EncodeHandshake(terminalproto.Handshake{ProtocolVersion: 1, RequestID: terminalbroker.ID(), AgentInstanceID: terminalbroker.ID(), Capabilities: caps})
	if err := c.Write(ctx, websocket.MessageText, handshake); err != nil {
		cancel()
		t.Fatal(err)
	}
	if _, _, err := c.Read(ctx); err != nil {
		cancel()
		t.Fatal(err)
	}
	var mu sync.Mutex
	k := &browseConnector{c: c, requests: make(chan terminalproto.Message, 16), closed: make(chan struct{})}
	k.write = func(m terminalproto.Message) error {
		data, err := terminalproto.Encode(m)
		if err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		return c.Write(ctx, websocket.MessageText, data)
	}
	go func() {
		defer close(k.closed)
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			m, err := terminalproto.Decode(data)
			if err != nil {
				return
			}
			switch body := m.Body.(type) {
			case *terminalproto.Probe:
				m.Body = terminalproto.ProbeResult(*body)
				k.write(m)
			case *terminalproto.Reconcile:
				m.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
				k.write(m)
			case *terminalproto.Browse, *terminalproto.Branches:
				k.requests <- m
			}
		}
	}()
	t.Cleanup(func() {
		cancel()
		c.CloseNow()
		<-k.closed
	})
	return k, response.Header.Get(terminalproto.FeatureHeader)
}

// A late browse_result or branches_result is dropped by the controller: the
// connection stays up and later requests are answered normally.
func TestBrowseLateReplyKeepsTheConnection(t *testing.T) {
	ordinary, branches := terminalbroker.BrowseCallTimeout, terminalbroker.BranchesCallTimeout
	terminalbroker.BrowseCallTimeout, terminalbroker.BranchesCallTimeout = 200*time.Millisecond, 200*time.Millisecond
	t.Cleanup(func() { terminalbroker.BrowseCallTimeout, terminalbroker.BranchesCallTimeout = ordinary, branches })
	srv, st, agent, secret, _ := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	k, features := dialBrowseConnector(t, srv, agent.ID, secret, browseCaps())
	if !terminalproto.SupportsBrowse(features) || !terminalproto.SupportsBranches(features) || !terminalproto.SupportsWorktree(features) {
		t.Fatal("features", features)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !srv.s.terminals.Ready(uid, agent.ID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	f := &browseAPI{srv: srv, st: st, uid: uid, agent: agent.ID, cookie: terminalLogin(t, st, uid)}
	if code, _, raw := f.get(t, f.cookie, browsePath(agent.ID, browseNonce(), nil)); code != 504 {
		t.Fatal("unanswered browse", code, raw)
	}
	if code, _, raw := f.get(t, f.cookie, branchesPath(agent.ID, browseTestHome+"/site", "")); code != 504 {
		t.Fatal("unanswered listing", code, raw)
	}
	lateBrowse, lateBranches := <-k.requests, <-k.requests
	lateBrowse.Body = browseHomeListing()
	lateBranches.Body = terminalproto.BranchesResult{Branches: []string{"late"}}
	if err := k.write(lateBrowse); err != nil {
		t.Fatal(err)
	}
	if err := k.write(lateBranches); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for srv.s.terminals.LateReplies() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if srv.s.terminals.LateReplies() != 2 {
		t.Fatal("late replies", srv.s.terminals.LateReplies())
	}
	select {
	case <-k.closed:
		t.Fatal("the controller ended the connection on a late answer")
	default:
	}
	terminalbroker.BrowseCallTimeout = 3 * time.Second
	go func() {
		m := <-k.requests
		m.Body = terminalproto.BrowseResult{Path: browseTestHome + "/work", Home: browseTestHome, Parent: browseTestHome, Repo: terminalproto.BrowseRepoNone}
		k.write(m)
	}()
	code, out, raw := f.get(t, f.cookie, browsePath(agent.ID, browseNonce(), url.Values{"path": {browseTestHome + "/work"}}))
	if code != 200 || out["path"] != browseTestHome+"/work" {
		t.Fatal("browse after late answers", code, raw)
	}
}
