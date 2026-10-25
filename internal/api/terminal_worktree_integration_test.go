//go:build linux || darwin

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/eyupio/reet.sh/internal/agenthealth"
	"github.com/eyupio/reet.sh/internal/privateconn"
	"github.com/eyupio/reet.sh/internal/terminalagent"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// integrationGit runs real git for test setup with a clean environment of its
// own (HOME is the test's home; no system config, no GIT_* variables).
func integrationGit(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	env := []string{"GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0"}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := integrationGit(t, dir, args...)
	if err != nil {
		t.Fatalf("git %q in %s: %v\n%s", args, dir, err, out)
	}
	return out
}

func integrationRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "init", "-q", "-b", "main")
	mustGit(t, dir, "config", "user.name", "Reet Test")
	mustGit(t, dir, "config", "user.email", "test@example.invalid")
	mustGit(t, dir, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "README")
	mustGit(t, dir, "commit", "-q", "-m", "first")
}

// testMachine is one enrolled machine: its own tmux server and a connector
// that negotiates exactly the features it is given.
type testMachine struct {
	home       string
	tmux       string
	socket     string
	controlled func() bool
}

func (m *testMachine) sessions() []string {
	out, _ := exec.Command(m.tmux, "-S", m.socket, "list-sessions", "-F", "#{session_name}").Output()
	text := strings.TrimSpace(string(out))
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func startTestMachine(t *testing.T, srv *Server, uid int64, agentID, secret, dir, home, features string, caps terminalproto.Capabilities) *testMachine {
	t.Helper()
	tmux, err := terminalhost.FindTmux(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	layout, err := terminalhost.NewEnrollmentLayout(filepath.Join(dir, agentID+".json"), agentID)
	if err != nil {
		t.Fatal(err)
	}
	if err := terminalhost.Ensure(layout); err != nil {
		t.Fatal(err)
	}
	tmuxProcess := exec.Command(tmux.Path, "-D", "-S", layout.Socket, "-f", layout.Config)
	tmuxProcess.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "SHELL=/bin/sh", "TERM=xterm-256color"}
	if err := tmuxProcess.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		exec.Command(tmux.Path, "-S", layout.Socket, "kill-server").Run()
		tmuxProcess.Process.Kill()
		tmuxProcess.Wait()
	})
	deadline := time.Now().Add(3 * time.Second)
	for !terminalhost.SocketActive(layout) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	exec.Command(tmux.Path, "-S", layout.Socket, "set-option", "-g", "default-command", "exec /bin/sh -i").Run()

	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.s.privateHandler().ServeHTTP(w, r.WithContext(privateconn.WithPrivateTransport(r.Context())))
	}))
	t.Cleanup(private.Close)
	base, _ := url.Parse(private.URL)
	privateClient := &http.Client{Transport: terminalTestTransport{base, http.DefaultTransport}}
	headers := http.Header{"X-Reet-Agent": {agentID}, "X-Reet-Agent-Key": {secret}, "X-Reet-Version": {"test"}}
	ctx, cancel := context.WithCancel(context.Background())
	c, response, err := websocket.Dial(ctx, private.URL+"/private/v1/control", &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if !terminalproto.SupportsWorktree(response.Header.Get(terminalproto.FeatureHeader)) {
		cancel()
		t.Fatal("controller did not advertise worktrees", response.Header.Get(terminalproto.FeatureHeader))
	}
	// The connector records only the features it understands, exactly as
	// agenthealth does; an older connector simply never claims worktree-v1.
	headers.Set(terminalproto.FeatureHeader, features)
	handshake, _ := terminalproto.EncodeHandshake(terminalproto.Handshake{ProtocolVersion: 1, RequestID: terminalbroker.ID(), AgentInstanceID: terminalbroker.ID(), Capabilities: caps})
	if err = c.Write(ctx, websocket.MessageText, handshake); err != nil {
		cancel()
		t.Fatal(err)
	}
	_, data, err := c.Read(ctx)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	reply, err := terminalproto.DecodeHandshakeReply(data)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	var mu sync.Mutex
	send := func(ctx context.Context, m terminalproto.Message) error {
		data, err := terminalproto.Encode(m)
		if err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		return c.Write(ctx, websocket.MessageText, data)
	}
	factory := terminalagent.Factory(terminalhost.ManagerOptions{EnrollmentID: agentID, Layout: layout, Home: home, Consent: func() error { return nil }})
	handle, cleanup := factory(ctx, privateClient, headers, reply.Generation, agenthealth.Sender(send))
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer cleanup()
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			m, err := terminalproto.Decode(data)
			if err != nil {
				return
			}
			if probe, ok := m.Body.(*terminalproto.Probe); ok {
				m.Body = terminalproto.ProbeResult(*probe)
				if send(ctx, m) != nil {
					return
				}
			} else if handle(ctx, m) != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		cancel()
		c.CloseNow()
		<-done
	})
	deadline = time.Now().Add(3 * time.Second)
	for !srv.s.terminals.Ready(uid, agentID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !srv.s.terminals.Ready(uid, agentID) {
		t.Fatal("private connection never became terminal-ready")
	}
	return &testMachine{home: home, tmux: tmux.Path, socket: layout.Socket}
}

func TestTerminalPrivateWorktreeStartsTheRecordedAgentInItsOwnWorktree(t *testing.T) {
	if _, err := terminalhost.FindTmux(t.Context()); err != nil {
		if os.Getenv("REET_BROWSER_TESTS") == "1" {
			t.Fatal("worktree gate requires installed tmux", err)
		}
		t.Skip(err)
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		if os.Getenv("REET_BROWSER_TESTS") == "1" {
			t.Fatal("worktree gate requires installed git", err)
		}
		t.Skip("git is not on PATH; the worktree test needs real git")
	}
	tmux, _ := terminalhost.FindTmux(t.Context())
	dir, err := os.MkdirTemp("/tmp", "reet-worktree-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	home := filepath.Join(dir, "home")
	oldHome := filepath.Join(dir, "old-home")
	records := filepath.Join(dir, "records")
	bin := filepath.Join(dir, "bin")
	for _, d := range []string{home, oldHome, records, bin} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	script := strings.Replace(recordingAgent, "RECORDS", records, 1)
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	// Only the recording fake, tmux, git and the base system are on PATH; HOME
	// is the machine's home so no developer git configuration applies.
	t.Setenv("PATH", bin+":"+filepath.Dir(tmux.Path)+":"+filepath.Dir(gitPath)+":/usr/bin:/bin")
	t.Setenv("HOME", home)
	realHome, _ := filepath.EvalSymlinks(home) // /tmp is a symlink on macOS

	site := filepath.Join(home, "work", "site")
	integrationRepo(t, site)
	plainFolder := filepath.Join(home, "work", "plain")
	for _, d := range []string{plainFolder, filepath.Join(site, "docs")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// A repository stopped in the middle of a rebase by a real conflict.
	busy := filepath.Join(home, "work", "busy")
	integrationRepo(t, busy)
	mustGit(t, busy, "checkout", "-q", "-b", "topic")
	os.WriteFile(filepath.Join(busy, "README"), []byte("topic\n"), 0o600)
	mustGit(t, busy, "commit", "-q", "-am", "topic")
	mustGit(t, busy, "checkout", "-q", "main")
	os.WriteFile(filepath.Join(busy, "README"), []byte("main\n"), 0o600)
	mustGit(t, busy, "commit", "-q", "-am", "main")
	mustGit(t, busy, "checkout", "-q", "topic")
	if out, err := integrationGit(t, busy, "rebase", "main"); err == nil {
		t.Fatal("rebase did not stop on a conflict", out)
	}
	oldSite := filepath.Join(oldHome, "site")
	if err := os.MkdirAll(oldSite, 0o700); err != nil {
		t.Fatal(err)
	}

	srv, st, agent, secret, _ := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	cookie := terminalLogin(t, st, uid)
	public := httptest.NewServer(srv)
	defer public.Close()
	srv.s.publicURL = public.URL
	oldAgent, oldSecret, err := st.EnrollSyncAgent(uid, "old-desktop", "test")
	if err != nil {
		t.Fatal(err)
	}

	caps := terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", TerminalControl: true, TerminalEnabled: true, HealthEnabled: true, Readiness: terminalproto.Ready, CommandHistory: true, Launch: true, Tools: []string{"claude"}, Worktree: true}
	machine := startTestMachine(t, srv, uid, agent.ID, secret, dir, home, terminalproto.CommandHistoryFeature+","+terminalproto.LaunchFeature+","+terminalproto.WorktreeFeature, caps)
	oldCaps := caps
	oldCaps.Worktree = false
	old := startTestMachine(t, srv, uid, oldAgent.ID, oldSecret, dir, oldHome, terminalproto.CommandHistoryFeature+","+terminalproto.LaunchFeature, oldCaps)

	call := func(cookie, path string, in any) (int, map[string]any, string) {
		t.Helper()
		data, _ := json.Marshal(in)
		r, _ := http.NewRequestWithContext(t.Context(), "POST", public.URL+path, strings.NewReader(string(data)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", public.URL)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
		resp, err := public.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		out := map[string]any{}
		_ = json.Unmarshal(raw, &out)
		return resp.StatusCode, out, string(raw)
	}
	waitStarts := func(n int) []recordedStart {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			starts := readStarts(t, records)
			if len(starts) >= n || time.Now().After(deadline) {
				return starts
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	worktreeCount := func(repo string) int {
		return strings.Count(mustGit(t, repo, "worktree", "list", "--porcelain"), "worktree ")
	}

	prompt := "fix the worktree bug and explain it"
	project := "demo-project"
	run := "a1b2c3d4e5f60718293a4b5c"
	branch := "reet/" + project + "-" + run
	wantPath := filepath.Join(realHome, ".reet", "worktrees", project, "site", run)
	launch := map[string]any{"agentId": agent.ID, "requestId": terminalbroker.ID(), "title": "Claude", "cols": 80, "rows": 24, "tool": "claude", "dir": site, "prompt": prompt, "worktree": run, "project": project}
	code, first, raw := call(cookie, "/api/v2/terminal/sessions", launch)
	if code != 200 || first["state"] != "running" {
		t.Fatal("worktree launch", code, raw)
	}
	starts := waitStarts(1)
	if len(starts) != 1 || starts[0].dir != wantPath || starts[0].argv0 != filepath.Join(bin, "claude") || len(starts[0].args) != 2 || starts[0].args[1] != prompt {
		t.Fatalf("recorded start %#v, want folder %s", starts, wantPath)
	}
	if got := mustGit(t, site, "for-each-ref", "--format=%(refname)", "refs/heads/reet/"); got != "refs/heads/"+branch {
		t.Fatal("branch", got)
	}
	if listed := mustGit(t, site, "worktree", "list", "--porcelain"); !strings.Contains(listed, "worktree "+wantPath+"\n") || !strings.Contains(listed, "branch refs/heads/"+branch) {
		t.Fatal("git worktree list", listed)
	}
	if mustGit(t, site, "symbolic-ref", "HEAD") != "refs/heads/main" {
		t.Fatal("the repository's own branch moved")
	}
	if len(machine.sessions()) != 1 {
		t.Fatal("tmux sessions after the worktree launch", machine.sessions())
	}

	// A retry of the same request makes nothing new.
	code, again, raw := call(cookie, "/api/v2/terminal/sessions", launch)
	if code != 200 || again["id"] != first["id"] {
		t.Fatal("retry", code, raw)
	}
	time.Sleep(300 * time.Millisecond)
	if len(readStarts(t, records)) != 1 || len(machine.sessions()) != 1 || worktreeCount(site) != 2 {
		t.Fatal("retry made something new", len(readStarts(t, records)), machine.sessions(), worktreeCount(site))
	}

	// A machine without the capability is asked to update and nothing reaches it;
	// a plain shell and a launch without a worktree still work there.
	oldLaunch := map[string]any{"agentId": oldAgent.ID, "requestId": terminalbroker.ID(), "title": "Claude", "cols": 80, "rows": 24, "tool": "claude", "dir": oldSite, "prompt": prompt, "worktree": run, "project": project}
	if code, out, raw := call(cookie, "/api/v2/terminal/sessions", oldLaunch); code != 409 || out["code"] != "worktree_unsupported" || out["error"] != "Update reet on that machine to start agents with worktrees." {
		t.Fatal("old machine", code, raw)
	}
	for _, path := range []string{"/api/v2/terminal/worktrees/status", "/api/v2/terminal/worktrees/remove"} {
		body := map[string]any{"agentId": oldAgent.ID, "project": project, "repo": "site", "run": run}
		if strings.HasSuffix(path, "remove") {
			body["changed"], body["unpushed"] = 0, 0
		}
		if code, out, raw := call(cookie, path, body); code != 409 || out["code"] != "worktree_unsupported" {
			t.Fatal("old machine", path, code, raw)
		}
	}
	if len(old.sessions()) != 0 {
		t.Fatal("the refused worktree launch reached the old machine", old.sessions())
	}
	if code, out, raw := call(cookie, "/api/v2/terminal/sessions", map[string]any{"agentId": oldAgent.ID, "requestId": terminalbroker.ID(), "title": "Shell", "cols": 80, "rows": 24}); code != 200 || out["state"] != "running" {
		t.Fatal("plain shell on the old machine", code, raw)
	}
	delete(oldLaunch, "worktree")
	delete(oldLaunch, "project")
	oldLaunch["requestId"] = terminalbroker.ID()
	if code, out, raw := call(cookie, "/api/v2/terminal/sessions", oldLaunch); code != 200 || out["state"] != "running" {
		t.Fatal("launch on the old machine", code, raw)
	}
	starts = waitStarts(2)
	realOldSite, _ := filepath.EvalSymlinks(oldSite)
	if len(starts) != 2 || starts[0].dir != realOldSite && starts[1].dir != realOldSite {
		t.Fatalf("launch on the old machine %#v", starts)
	}
	if len(old.sessions()) != 2 {
		t.Fatal("old machine sessions", old.sessions())
	}

	// Refusals start nothing and leave no tmux session.
	otherRun := "00112233445566778899aabb"
	mustGit(t, site, "branch", "reet/"+project+"-"+otherRun)
	for _, tc := range []struct {
		name, dir, run, code, sentence string
	}{
		{"not a repository root", plainFolder, "aaaaaaaaaaaaaaaaaaaaaaaa", "not_repo_root", "That folder isn't the top of a git repository."},
		{"subfolder of a repository", filepath.Join(site, "docs"), "bbbbbbbbbbbbbbbbbbbbbbbb", "not_repo_root", "That folder isn't the top of a git repository."},
		{"rebase in progress", busy, "cccccccccccccccccccccccc", "repo_busy", "That repository is in the middle of a merge, rebase or similar. Finish or abort it first."},
		{"branch exists", site, otherRun, "worktree_exists", "A worktree or branch with this run's name is already on that machine, so nothing was started."},
	} {
		body := map[string]any{"agentId": agent.ID, "requestId": terminalbroker.ID(), "title": "Refused", "cols": 80, "rows": 24, "tool": "claude", "dir": tc.dir, "prompt": prompt, "worktree": tc.run, "project": project}
		code, out, raw := call(cookie, "/api/v2/terminal/sessions", body)
		if code != 422 || out["code"] != tc.code || out["error"] != tc.sentence {
			t.Fatal(tc.name, code, raw)
		}
		if names := machine.sessions(); len(names) != 1 {
			t.Fatal(tc.name, "left a tmux session", names)
		}
	}
	if len(readStarts(t, records)) != 2 {
		t.Fatal("a refused worktree launch started an agent")
	}
	if worktreeCount(site) != 2 || worktreeCount(busy) != 1 {
		t.Fatal("a refusal made a worktree", worktreeCount(site), worktreeCount(busy))
	}
	if entries, _ := os.ReadDir(filepath.Join(realHome, ".reet", "worktrees", project)); len(entries) != 1 {
		t.Fatal("a refusal made a folder", entries)
	}

	target := map[string]any{"agentId": agent.ID, "project": project, "repo": "site", "run": run}
	status := func() map[string]any {
		t.Helper()
		code, out, raw := call(cookie, "/api/v2/terminal/worktrees/status", target)
		if code != 200 {
			t.Fatal("status", code, raw)
		}
		return out
	}
	remove := func(changed, unpushed int) (int, map[string]any, string) {
		t.Helper()
		body := map[string]any{"changed": changed, "unpushed": unpushed}
		for k, v := range target {
			body[k] = v
		}
		return call(cookie, "/api/v2/terminal/worktrees/remove", body)
	}

	// Viewers are refused on all three routes.
	viewer, err := st.CreateUser("worktree-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	viewerCookie := terminalLogin(t, st, viewer.ID)
	for path, body := range map[string]any{
		"/api/v2/terminal/sessions":         map[string]any{"agentId": agent.ID, "requestId": terminalbroker.ID(), "title": "Viewer", "cols": 80, "rows": 24, "tool": "claude", "dir": site, "worktree": "dddddddddddddddddddddddd", "project": project},
		"/api/v2/terminal/worktrees/status": target,
		"/api/v2/terminal/worktrees/remove": map[string]any{"agentId": agent.ID, "project": project, "repo": "site", "run": run, "changed": 0, "unpushed": 0},
	} {
		if code, _, raw := call(viewerCookie, path, body); code != 403 {
			t.Fatal("viewer", path, code, raw)
		}
	}

	// A fresh worktree has nothing to lose.
	fresh := status()
	if fresh["path"] != wantPath || fresh["branch"] != branch || fresh["changed"] != 0.0 || fresh["unpushed"] != 0.0 || fresh["unreferenced"] != 0.0 || fresh["hidden"] != 0.0 || fresh["nested"] != false || fresh["incomplete"] != false {
		t.Fatal("fresh status", fresh)
	}

	// The agent's session ends before its worktree is removed, as the page offers it.
	if code, _, raw := call(cookie, "/api/v2/terminal/sessions/"+first["id"].(string)+"/end", map[string]any{"requestId": terminalbroker.ID()}); code != 200 {
		t.Fatal("end", code, raw)
	}

	// An uncommitted edit is reported and blocks removal whatever was shown.
	draft := filepath.Join(wantPath, "draft.txt")
	if err := os.WriteFile(draft, []byte("unsaved\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	edited := status()
	if edited["changed"] != 1.0 || len(edited["paths"].([]any)) != 1 || edited["paths"].([]any)[0] != "draft.txt" {
		t.Fatal("edited status", edited)
	}
	if code, out, raw := remove(0, 0); code != 409 || out["code"] != "worktree_changed" || out["error"] != "That worktree changed since you looked. Check again." {
		t.Fatal("stale counts", code, raw)
	}
	if code, out, raw := remove(1, 0); code != 422 || out["code"] != "has_changes" || out["error"] != "This worktree has changes you haven't saved. Commit, stash or discard them in the worktree, then check again." {
		t.Fatal("shown changes", code, raw)
	}
	if _, err := os.Stat(draft); err != nil {
		t.Fatal("a refused remove lost the edit", err)
	}

	// A commit on the run branch is counted as unpushed and kept with the branch.
	mustGit(t, wantPath, "add", "draft.txt")
	mustGit(t, wantPath, "-c", "user.name=Reet Test", "-c", "user.email=test@example.invalid", "commit", "-q", "-m", "agent work")
	tip := mustGit(t, site, "rev-parse", "refs/heads/"+branch)
	committed := status()
	if committed["changed"] != 0.0 || committed["unpushed"] != 1.0 {
		t.Fatal("committed status", committed)
	}
	if code, out, raw := remove(0, 0); code != 409 || out["code"] != "worktree_changed" {
		t.Fatal("unpushed not shown", code, raw)
	}
	code, out, raw := remove(0, 1)
	if code != 200 || out["ok"] != true || out["path"] != wantPath || out["branch"] != branch {
		t.Fatal("remove", code, raw)
	}
	if _, err := os.Stat(wantPath); !os.IsNotExist(err) {
		t.Fatal("the worktree folder is still there", err)
	}
	if mustGit(t, site, "rev-parse", "refs/heads/"+branch) != tip {
		t.Fatal("the branch was not kept at its tip")
	}
	if strings.Contains(mustGit(t, site, "worktree", "list", "--porcelain"), wantPath) {
		t.Fatal("git still lists the removed worktree")
	}
	if code, out, raw := call(cookie, "/api/v2/terminal/worktrees/status", target); code != 404 || out["code"] != "not_found" {
		t.Fatal("status after remove", code, raw)
	}

	// Audit: one launch with the worktree, one removal, never the prompt.
	events, err := st.ListEventsFor(uid, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	everything, _ := json.Marshal(events)
	if strings.Contains(string(everything), prompt) || strings.Contains(string(everything), "worktree bug") {
		t.Fatal("the prompt was written to audit")
	}
	launches := worktreeEvents(t, st, uid, "terminal-launch")
	removals := worktreeEvents(t, st, uid, "worktree-remove")
	if len(launches) != 2 || len(removals) != 1 {
		t.Fatal("audit rows", len(launches), len(removals))
	}
	var detail map[string]string
	if err := json.Unmarshal([]byte(removals[0].Detail), &detail); err != nil {
		t.Fatal(err)
	}
	if detail["machine"] != agent.ID || detail["path"] != wantPath || detail["branch"] != branch || len(detail) != 4 {
		t.Fatal("worktree-remove audit", removals[0].Detail)
	}
	found := false
	for _, e := range launches {
		found = found || strings.Contains(e.Detail, `"worktreeRun":"`+run+`"`) && strings.Contains(e.Detail, `"branch":"`+branch+`"`)
	}
	if !found {
		t.Fatal("terminal-launch audit lacks the worktree", launches)
	}
}
