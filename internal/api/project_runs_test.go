package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/projectrun"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// runMachine is one enrolled machine connected through the real broker. It
// keeps the sessions it started, reports them on reconcile, and can drop a
// create's reply (the agent starts but the controller never hears).
type runMachine struct {
	*launchAPI
	logs    *bytes.Buffer
	mu      sync.Mutex
	got     []terminalproto.Message
	running map[string]terminalproto.SessionSnapshot
	// decide answers a create; nil means start it and report the worktree.
	decide func(terminalproto.Create) terminalproto.Body
	drop   bool
}

func runMachineFixture(t *testing.T) *runMachine {
	t.Helper()
	var logs bytes.Buffer
	original := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(original) })
	srv, st, agent, _, _ := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	f := &runMachine{launchAPI: &launchAPI{srv: srv, st: st, uid: uid, agent: agent, cookie: terminalLogin(t, st, uid)}, logs: &logs, running: map[string]terminalproto.SessionSnapshot{}}
	record, err := st.BeginRemoteConnectionFor(uid, agent.ID, terminalbroker.ID(), worktreeCaps("claude", "codex"))
	if err != nil {
		t.Fatal(err)
	}
	var p *terminalbroker.Peer
	p, err = srv.s.terminals.Connect(uid, agent.ID, record.Generation, true, func(ctx context.Context, m terminalproto.Message) error {
		reply := m
		switch op := m.Body.(type) {
		case terminalproto.Reconcile:
			f.mu.Lock()
			sessions := []terminalproto.SessionSnapshot{}
			for _, s := range f.running {
				sessions = append(sessions, s)
			}
			f.mu.Unlock()
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: sessions}
		case terminalproto.Create:
			f.mu.Lock()
			f.got = append(f.got, m)
			decide, drop := f.decide, f.drop
			f.mu.Unlock()
			var body terminalproto.Body
			if decide != nil {
				body = decide(op)
			}
			if body == nil {
				repo := terminalproto.Slug(path.Base(op.Dir), "repo")
				changed := terminalproto.SessionChanged{State: terminalproto.Running}
				if op.Worktree != "" {
					changed.WorktreePath = "/home/tester/.reet/worktrees/" + op.Project + "/" + repo + "/" + op.Worktree
					changed.WorktreeRepo = repo
				}
				body = changed
				f.mu.Lock()
				f.running[m.SessionID] = terminalproto.SessionSnapshot{ID: m.SessionID, Title: op.Title, State: terminalproto.Running}
				f.mu.Unlock()
			}
			if drop {
				return nil // the agent started; the answer never arrives
			}
			reply.Body = body
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
	if err := p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.peer = p
	return f
}

func (f *runMachine) creates() []terminalproto.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]terminalproto.Message(nil), f.got...)
}

// project saves a Project with two repositories on the machine.
func (f *runMachine) project(t *testing.T) string {
	t.Helper()
	body := map[string]any{"name": "Shop", "defaultTool": "claude", "sharedPrompt": projectSentinel, "members": []map[string]any{
		{"host": f.agent.ID, "dir": "/home/tester/site"}, {"host": f.agent.ID, "dir": "/home/tester/api", "tool": "codex"}, {"host": f.agent.ID, "dir": "/home/tester/docs"}}}
	code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/projects", body)
	if code != 201 {
		t.Fatal(code, raw)
	}
	return out["id"].(string)
}

func (f *runMachine) settled(t *testing.T, runID string, done func(store.Run) bool) store.Run {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		r, err := f.st.RunFor(f.uid, runID)
		if err != nil {
			t.Fatal(err)
		}
		if done(r) {
			return r
		}
		if time.Now().After(deadline) {
			t.Fatalf("run never settled: %+v", r.Members)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func notRunning(r store.Run) bool { return r.State != store.RunRunning }

func TestProjectRunStartsAtOnceAndEachMemberStartsInItsOwnWorktree(t *testing.T) {
	f := runMachineFixture(t)
	id := f.project(t)
	code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/projects/"+id+"/runs", map[string]any{"positions": []int{0, 2}})
	if code != 201 || out["state"] != "running" || out["projectId"] != id || strings.Contains(raw, projectSentinel) {
		t.Fatal(code, raw)
	}
	members := out["members"].([]any)
	if len(members) != 2 {
		t.Fatal(raw)
	}
	for _, m := range members {
		if m.(map[string]any)["state"] != "pending" {
			t.Fatal(raw)
		}
	}
	runID := out["id"].(string)
	run := f.settled(t, runID, notRunning)
	if run.State != store.RunComplete {
		t.Fatalf("%+v", run.Members)
	}
	sent := f.creates()
	if len(sent) != 2 {
		t.Fatalf("%d creates", len(sent))
	}
	tokens := map[string]bool{}
	for _, m := range run.Members {
		var c terminalproto.Message
		for _, s := range sent {
			if s.Body.(terminalproto.Create).Dir == m.Dir {
				c = s
			}
		}
		in := c.Body.(terminalproto.Create)
		if c.RequestID != projectrun.RequestID(runID, m.Position, 1) || in.Worktree != projectrun.WorktreeToken(runID, m.Position) || in.Project != "shop" || in.Prompt != projectSentinel {
			t.Fatalf("%+v %+v", c, in)
		}
		tokens[in.Worktree] = true
		session, err := f.st.TerminalSessionFor(f.uid, m.SessionID)
		if err != nil || m.State != store.RunMemberStarted || m.WorktreePath != session.WorktreePath || m.WorktreeRepo != session.WorktreeRepo ||
			m.WorktreeBranch != "reet/shop-"+in.Worktree || m.WorktreePath == "" {
			t.Fatalf("%v %+v %+v", err, m, session)
		}
	}
	if len(tokens) != 2 {
		t.Fatal("members shared a worktree token")
	}
	runs := projectEvents(t, f.st, f.uid, "project-run")
	if len(runs) != 1 || runs[0]["project"] != id || runs[0]["run"] != runID || runs[0]["members"] != float64(2) || len(runs[0]) != 3 {
		t.Fatal(runs)
	}
	if launches := projectEvents(t, f.st, f.uid, "terminal-launch"); len(launches) != 2 {
		t.Fatal(launches)
	}
	// The run reads back without the prompt, and nothing recorded it.
	events, _ := json.Marshal(worktreeEvents(t, f.st, f.uid, "project-run"))
	launches, _ := json.Marshal(worktreeEvents(t, f.st, f.uid, "terminal-launch"))
	_, _, list := f.do(t, f.cookie, "GET", "/api/v2/projects/"+id+"/runs", nil)
	for _, text := range []string{string(events), string(launches), list, f.logs.String()} {
		if strings.Contains(text, projectSentinel) || strings.Contains(text, "zebra-sentinel") {
			t.Fatal("the prompt leaked:", text)
		}
	}
}

func TestProjectRunRetryUsesTheNextAttemptAfterARefusal(t *testing.T) {
	f := runMachineFixture(t)
	refuse := true
	f.decide = func(in terminalproto.Create) terminalproto.Body {
		if refuse && strings.HasSuffix(in.Dir, "/api") {
			return terminalproto.Result{Code: terminalproto.LaunchFailed}
		}
		return nil
	}
	id := f.project(t)
	_, out, _ := f.do(t, f.cookie, "POST", "/api/v2/projects/"+id+"/runs", map[string]any{"positions": []int{0, 1, 2}})
	runID := out["id"].(string)
	run := f.settled(t, runID, notRunning)
	failed := run.Members[1]
	if run.State != store.RunPartial || failed.State != store.RunMemberFailed || failed.Code != "folder_unavailable" {
		t.Fatalf("%+v", run.Members)
	}
	_, _, raw := f.do(t, f.cookie, "GET", "/api/v2/projects/"+id+"/runs", nil)
	if !strings.Contains(raw, "That folder isn't available on that machine.") {
		t.Fatal(raw)
	}
	f.mu.Lock()
	refuse = false
	f.mu.Unlock()
	code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/project-runs/"+runID+"/members/1/retry", map[string]any{})
	if code != 200 || out["id"] != runID {
		t.Fatal(code, raw)
	}
	run = f.settled(t, runID, notRunning)
	if run.State != store.RunComplete || run.Members[1].Attempt != 2 {
		t.Fatalf("%+v", run.Members)
	}
	sent := f.creates()
	if len(sent) != 4 || sent[3].RequestID != projectrun.RequestID(runID, 1, 2) {
		t.Fatalf("%d creates", len(sent))
	}
	if code, out, _ := f.do(t, f.cookie, "POST", "/api/v2/project-runs/"+runID+"/members/1/retry", map[string]any{}); code != 409 || out["code"] != "not_failed" {
		t.Fatal(code, out)
	}
	if retries := projectEvents(t, f.st, f.uid, "project-run-retry"); len(retries) != 1 || retries[0]["attempt"] != float64(2) || retries[0]["position"] != float64(1) {
		t.Fatal(retries)
	}
}

// A create whose answer was lost while the controller stopped is sent again,
// with the same request id, when the controller resumes. The broker finds the
// reservation, asks the machine, and records the session it already started:
// the machine receives one create and starts one agent.
func TestProjectRunResumedAfterALostAnswerAdoptsTheSessionAndStartsNoSecondAgent(t *testing.T) {
	f := runMachineFixture(t)
	f.drop = true
	id := f.project(t)
	_, out, _ := f.do(t, f.cookie, "POST", "/api/v2/projects/"+id+"/runs", map[string]any{"positions": []int{0}})
	runID := out["id"].(string)
	deadline := time.Now().Add(10 * time.Second)
	for len(f.creates()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no create")
		}
		time.Sleep(5 * time.Millisecond)
	}
	f.srv.s.runs.Close() // the controller stops while the answer is outstanding
	run, err := f.st.RunFor(f.uid, runID)
	if err != nil || run.Members[0].State != store.RunMemberPending {
		t.Fatalf("%v %+v", err, run.Members)
	}
	first := f.creates()[0]

	f.srv.s.runs = f.srv.s.newProjectRuns()
	f.srv.ResumeProjectRuns(t.Context())
	run = f.settled(t, runID, notRunning)
	m := run.Members[0]
	if m.State != store.RunMemberStarted || m.SessionID != first.SessionID {
		t.Fatalf("%+v want session %s", m, first.SessionID)
	}
	if n := len(f.creates()); n != 1 {
		t.Fatalf("the machine received %d creates", n)
	}
	// The same create replayed directly returns the same session again.
	s, started, err := f.srv.s.terminals.CreateSession(t.Context(), f.uid, f.agent.ID, first.RequestID, first.Body.(terminalproto.Create))
	if err != nil || started || s.ID != first.SessionID || len(f.creates()) != 1 {
		t.Fatal(err, started, s, len(f.creates()))
	}
	// The worktree reply was lost, so the member has no worktree facts.
	if m.WorktreePath != "" || m.WorktreeRepo != "" {
		t.Fatalf("%+v", m)
	}
	if launches := projectEvents(t, f.st, f.uid, "terminal-launch"); len(launches) != 1 {
		t.Fatal(launches)
	}
}

func TestProjectRunRoutesAreScopedToTheOwner(t *testing.T) {
	f := runMachineFixture(t)
	id := f.project(t)
	run, err := f.st.CreateRunFor(f.uid, id, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.SetRunMemberState(f.uid, run.ID, 0, store.RunMemberFailed, "folder_unavailable", "", "", "", "", 1); err != nil {
		t.Fatal(err)
	}
	other, err := f.st.CreateUser("runs-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	b := terminalLogin(t, f.st, other.ID)
	if code, out, raw := f.do(t, b, "POST", "/api/v2/projects/"+id+"/runs", map[string]any{"positions": []int{0}}); code != 404 || out["code"] != "not_found" {
		t.Fatal(code, raw)
	}
	if code, out, raw := f.do(t, b, "POST", "/api/v2/project-runs/"+run.ID+"/members/0/retry", map[string]any{}); code != 404 || out["code"] != "run_not_found" {
		t.Fatal(code, raw)
	}
	time.Sleep(50 * time.Millisecond)
	after, _ := f.st.RunFor(f.uid, run.ID)
	if len(f.creates()) != 0 || after.Members[0].State != store.RunMemberFailed {
		t.Fatal("another account started something")
	}
	if runs, _ := f.st.RunsFor(f.uid, id, 20); len(runs) != 1 {
		t.Fatal("another account made a run")
	}
}

func TestProjectRunMembersOnAForgottenMachineAreNeverSent(t *testing.T) {
	f := runMachineFixture(t)
	id := f.project(t)
	if err := f.st.ControlSyncAgent(f.uid, f.agent.ID, "revoke"); err != nil {
		t.Skip("cannot forget the machine in this fixture:", err)
	}
	_, out, raw := f.do(t, f.cookie, "POST", "/api/v2/projects/"+id+"/runs", map[string]any{"positions": []int{0}})
	if out["id"] == nil {
		t.Fatal(raw)
	}
	run := f.settled(t, out["id"].(string), notRunning)
	if run.Members[0].Code != "unknown_machine" || len(f.creates()) != 0 {
		t.Fatalf("%+v", run.Members)
	}
}

func TestProjectRunRoutesRefuseViewersLegacyAuthTokensOtherOriginsAndDisabledTerminals(t *testing.T) {
	f := runMachineFixture(t)
	id := f.project(t)
	run, err := f.st.CreateRunFor(f.uid, id, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.SetRunMemberState(f.uid, run.ID, 0, store.RunMemberFailed, "folder_unavailable", "", "", "", "", 1); err != nil {
		t.Fatal(err)
	}
	viewer, err := f.st.CreateUser("runs-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	viewerCookie := terminalLogin(t, f.st, viewer.ID)
	p := &projectAPI{launchAPI: f.launchAPI}
	routes := []struct{ path, body string }{
		{"/api/v2/projects/" + id + "/runs", `{"positions":[0]}`},
		{"/api/v2/project-runs/" + run.ID + "/members/0/retry", `{}`},
	}
	origin := "https://reet.example"
	for _, rt := range routes {
		if code, raw := p.raw(t, "POST", rt.path, rt.body, viewerCookie, origin, ""); code != 403 || !strings.Contains(raw, "read_only") {
			t.Fatal("viewer", rt.path, code, raw)
		}
		if code, raw := p.raw(t, "POST", rt.path, rt.body, "", origin, "fixture-owner"); code != 403 || !strings.Contains(raw, "accounts_required") {
			t.Fatal("token", rt.path, code, raw)
		}
		if code, raw := p.raw(t, "POST", rt.path, rt.body, f.cookie, origin, "fixture-owner"); code != 403 {
			t.Fatal("token beside a cookie", rt.path, code, raw)
		}
		if code, _ := p.raw(t, "POST", rt.path, rt.body, "", origin, ""); code != 401 {
			t.Fatal("signed out", rt.path, code)
		}
		for _, bad := range []string{"", "https://evil.example", "http://reet.example", "null"} {
			if code, raw := p.raw(t, "POST", rt.path, rt.body, f.cookie, bad, ""); code != 403 || !strings.Contains(raw, "origin_refused") {
				t.Fatal("origin", bad, rt.path, code, raw)
			}
		}
	}
	for _, mode := range []AuthMode{AuthNone, AuthToken} {
		f.srv.s.authMode, f.srv.s.token = mode, "single-token"
		for _, rt := range routes {
			for _, token := range []string{"", "single-token"} {
				if code, raw := p.raw(t, "POST", rt.path, rt.body, f.cookie, origin, token); code != 403 && code != 401 {
					t.Fatal("legacy", mode, rt.path, code, raw)
				}
			}
		}
	}
	f.srv.s.authMode, f.srv.s.token = AuthAccounts, ""
	f.srv.SetRemoteTerminals(false)
	for _, rt := range routes {
		if code, raw := p.raw(t, "POST", rt.path, rt.body, f.cookie, origin, ""); code != 403 || !strings.Contains(raw, "terminals_disabled") {
			t.Fatal("terminals off", rt.path, code, raw)
		}
	}
	time.Sleep(50 * time.Millisecond)
	if runs, _ := f.st.RunsFor(f.uid, id, 20); len(runs) != 1 || len(f.creates()) != 0 {
		t.Fatal("a refused caller started a run")
	}
	if after, _ := f.st.RunFor(f.uid, run.ID); after.Members[0].State != store.RunMemberFailed || after.Members[0].Attempt != 1 {
		t.Fatal("a refused caller retried", after.Members)
	}
}

func TestProjectRunRequestsAreStrict(t *testing.T) {
	f := runMachineFixture(t)
	id := f.project(t)
	p := &projectAPI{launchAPI: f.launchAPI}
	origin := "https://reet.example"
	start := "/api/v2/projects/" + id + "/runs"
	for body, want := range map[string]string{
		``:                                  "invalid_request",
		`[0]`:                               "invalid_request",
		`{"positions":"0"}`:                 "invalid_request",
		`{"positions":[0],"prompt":"x"}`:    "invalid_request",
		`{"positions":[0]}{}`:               "invalid_request",
		`{"positions":[0]} x`:               "invalid_request",
		`{"positions":[0.5]}`:               "invalid_request",
		`{}`:                                "no_members",
		`{"positions":[]}`:                  "no_members",
		`{"positions":null}`:                "no_members",
		`{"positions":[3]}`:                 "unknown_position",
		`{"positions":[-1]}`:                "unknown_position",
		`{"positions":[0,0]}`:               "unknown_position",
		`{"positions":[0,1,2,99]}`:          "unknown_position",
		`{"positions":[0],"positions2":[]}`: "invalid_request",
	} {
		code, raw := p.raw(t, "POST", start, body, f.cookie, origin, "")
		if !strings.Contains(raw, `"code":"`+want+`"`) || code < 400 {
			t.Fatal(body, code, raw)
		}
		if want == "unknown_position" && code != 422 {
			t.Fatal(body, code)
		}
	}
	if code, raw := p.raw(t, "POST", "/api/v2/projects/"+strings.Repeat("0", 32)+"/runs", `{"positions":[0]}`, f.cookie, origin, ""); code != 404 {
		t.Fatal(code, raw)
	}
	run, err := f.st.CreateRunFor(f.uid, id, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.SetRunMemberState(f.uid, run.ID, 0, store.RunMemberFailed, "folder_unavailable", "", "", "", "", 1); err != nil {
		t.Fatal(err)
	}
	retry := func(pos string) string { return "/api/v2/project-runs/" + run.ID + "/members/" + pos + "/retry" }
	for _, c := range []struct{ path, body, want string }{
		{retry("0"), ``, "invalid_request"},
		{retry("0"), `{"attempt":2}`, "invalid_request"},
		{retry("0"), `{}{}`, "invalid_request"},
		{retry("00"), `{}`, "unknown_position"},
		{retry("-1"), `{}`, "unknown_position"},
		{retry("x"), `{}`, "unknown_position"},
		{retry("1"), `{}`, "unknown_position"},
		{"/api/v2/project-runs/" + strings.Repeat("f", 32) + "/members/0/retry", `{}`, "run_not_found"},
	} {
		if code, raw := p.raw(t, "POST", c.path, c.body, f.cookie, origin, ""); !strings.Contains(raw, `"code":"`+c.want+`"`) || code < 400 {
			t.Fatal(c.path, c.body, code, raw)
		}
	}
	time.Sleep(50 * time.Millisecond)
	if runs, _ := f.st.RunsFor(f.uid, id, 20); len(runs) != 1 || len(f.creates()) != 0 {
		t.Fatal("a refused request started something", len(runs), len(f.creates()))
	}
}

func TestProjectRunFailureCodesHaveSentences(t *testing.T) {
	for _, code := range []string{projectrun.CodeUnknownOutcome, projectrun.CodeSessionLost, projectrun.CodeTerminalsDisabled, projectrun.CodeUnknownMachine,
		projectrun.CodeMachineUnavailable, projectrun.CodeSessionLimit, projectrun.CodeLaunchUnsupported, projectrun.CodeWorktreeUnsupported,
		projectrun.CodeToolNotInstalled, projectrun.CodeFolderUnavailable, projectrun.CodePromptUnsupported, projectrun.CodeWorktreeRefused,
		projectrun.CodeWorktreeMismatch, projectrun.CodeInvalidLaunch, "not_repo_root", "repo_busy", "worktree_exists", "git_failed", "outside_home"} {
		if s := runMemberSentence(store.RunMemberFailed, code); s == "" || s == "Something changed. Check again." {
			t.Fatal(code, s)
		}
	}
	if got := runMemberSentence(store.RunMemberFailed, projectrun.CodeUnknownOutcome); got != "We didn't hear back, so the agent may have started. Check your sessions before retrying." {
		t.Fatal(got)
	}
}
