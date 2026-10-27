package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
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
	// worktrees are the worktrees the machine made, by project/repo/run.
	worktrees map[string]string
	// decide answers a create; nil means start it and report the worktree.
	decide func(terminalproto.Create) terminalproto.Body
	drop   bool
	// hold, when set, keeps every create waiting until it is closed.
	hold chan struct{}
}

func runMachineFixture(t *testing.T) *runMachine {
	t.Helper()
	var logs bytes.Buffer
	original := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(original) })
	srv, st, agent, _, _ := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	f := &runMachine{launchAPI: &launchAPI{srv: srv, st: st, uid: uid, agent: agent, cookie: terminalLogin(t, st, uid)}, logs: &logs, running: map[string]terminalproto.SessionSnapshot{}, worktrees: map[string]string{}}
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
			decide, drop, hold := f.decide, f.drop, f.hold
			f.mu.Unlock()
			if hold != nil {
				<-hold
			}
			var body terminalproto.Body
			if decide != nil {
				body = decide(op)
			}
			if body == nil {
				repo := terminalproto.Slug(path.Base(op.Dir), "repo")
				changed := terminalproto.SessionChanged{State: terminalproto.Running}
				if op.Worktree != "" {
					changed.WorktreePath = "/home/tester/.reet/worktrees/" + op.Project + "/" + repo + "/" + op.Worktree
					f.mu.Lock()
					f.worktrees[op.Project+"/"+repo+"/"+op.Worktree] = changed.WorktreePath
					f.mu.Unlock()
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
		case terminalproto.WorktreeStatus:
			f.mu.Lock()
			path := f.worktrees[op.Project+"/"+op.Repo+"/"+op.Run]
			f.mu.Unlock()
			reply.Body = terminalproto.WorktreeResult{Code: terminalproto.NotFound}
			if path != "" {
				reply.Body = terminalproto.WorktreeState{Path: path, Branch: "reet/" + op.Project + "-" + op.Run}
			}
		case terminalproto.WorktreeRemove:
			key := op.Project + "/" + op.Repo + "/" + op.Run
			f.mu.Lock()
			path := f.worktrees[key]
			delete(f.worktrees, key)
			f.mu.Unlock()
			reply.Body = terminalproto.WorktreeResult{Code: terminalproto.NotFound}
			if path != "" {
				reply.Body = terminalproto.WorktreeResult{Code: terminalproto.OK, Path: path, Branch: "reet/" + op.Project + "-" + op.Run}
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
	return storeProject(t, f.st, f.uid, body).ID
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
	code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/projects/"+id+"/runs", map[string]any{"requestId": terminalbroker.ID(), "positions": []int{0, 2}})
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
	_, out, _ := f.do(t, f.cookie, "POST", "/api/v2/projects/"+id+"/runs", map[string]any{"requestId": terminalbroker.ID(), "positions": []int{0, 1, 2}})
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
	_, out, _ := f.do(t, f.cookie, "POST", "/api/v2/projects/"+id+"/runs", map[string]any{"requestId": terminalbroker.ID(), "positions": []int{0}})
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
	if code, out, raw := f.do(t, b, "POST", "/api/v2/projects/"+id+"/runs", map[string]any{"requestId": terminalbroker.ID(), "positions": []int{0}}); code != 404 || out["code"] != "not_found" {
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
	_, out, raw := f.do(t, f.cookie, "POST", "/api/v2/projects/"+id+"/runs", map[string]any{"requestId": terminalbroker.ID(), "positions": []int{0}})
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
		{"/api/v2/projects/" + id + "/runs", `{"requestId":"` + terminalbroker.ID() + `","positions":[0]}`},
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
	rq := `"requestId":"` + terminalbroker.ID() + `",`
	for body, want := range map[string]string{
		``:                            "invalid_request",
		`[0]`:                         "invalid_request",
		`{` + rq + `"positions":"0"}`: "invalid_request",
		`{` + rq + `"positions":[0],"prompt":"x"}`:             "invalid_request",
		`{` + rq + `"positions":[0]}{}`:                        "invalid_request",
		`{` + rq + `"positions":[0]} x`:                        "invalid_request",
		`{` + rq + `"positions":[0.5]}`:                        "invalid_request",
		`{"requestId":7,"positions":[0]}`:                      "invalid_request",
		`{}`:                                                   "request_id_required",
		`{"positions":[0]}`:                                    "request_id_required",
		`{"requestId":"short","positions":[0]}`:                "request_id_required",
		`{"requestId":"has space in it here","positions":[0]}`: "request_id_required",
		`{` + rq + `"positions":[]}`:                           "no_members",
		`{` + rq + `"positions":null}`:                         "no_members",
		`{"requestId":"` + terminalbroker.ID() + `"}`:          "no_members",
		`{` + rq + `"positions":[3]}`:                          "unknown_position",
		`{` + rq + `"positions":[-1]}`:                         "unknown_position",
		`{` + rq + `"positions":[0,0]}`:                        "unknown_position",
		`{` + rq + `"positions":[0,1,2,99]}`:                   "unknown_position",
		`{` + rq + `"positions":[0],"positions2":[]}`:          "invalid_request",
	} {
		code, raw := p.raw(t, "POST", start, body, f.cookie, origin, "")
		if !strings.Contains(raw, `"code":"`+want+`"`) || code < 400 {
			t.Fatal(body, code, raw)
		}
		if want == "unknown_position" && code != 422 {
			t.Fatal(body, code)
		}
	}
	if code, raw := p.raw(t, "POST", "/api/v2/projects/"+strings.Repeat("0", 32)+"/runs", `{"requestId":"`+terminalbroker.ID()+`","positions":[0]}`, f.cookie, origin, ""); code != 404 {
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
		projectrun.CodeWorktreeMismatch, projectrun.CodeInvalidLaunch, projectrun.CodeMachineRefused, "not_repo_root", "repo_busy", "worktree_exists", "git_failed", "outside_home"} {
		if s := runMemberSentence(store.RunMemberFailed, code); s == "" || s == "Something changed. Check again." {
			t.Fatal(code, s)
		}
	}
	if got := runMemberSentence(store.RunMemberFailed, projectrun.CodeUnknownOutcome); got != "We didn't hear back, so the agent may have started. Check your sessions before retrying." {
		t.Fatal(got)
	}
}

// The machine's own refusals of a create (capacity, denied, conflict, failed)
// read as what they are, not as a lost session. All but a conflict leave the
// member free to take a new attempt.
func TestProjectRunMachineRefusalsHaveTheirOwnSentences(t *testing.T) {
	f := runMachineFixture(t)
	codes := map[string]terminalproto.ResultCode{"/home/tester/site": terminalproto.Capacity, "/home/tester/api": terminalproto.Denied, "/home/tester/docs": terminalproto.Conflict}
	f.decide = func(in terminalproto.Create) terminalproto.Body { return terminalproto.Result{Code: codes[in.Dir]} }
	id := f.project(t)
	_, out, raw := f.do(t, f.cookie, "POST", "/api/v2/projects/"+id+"/runs", map[string]any{"requestId": terminalbroker.ID(), "positions": []int{0, 1, 2}})
	if out["id"] == nil {
		t.Fatal(raw)
	}
	run := f.settled(t, out["id"].(string), notRunning)
	want := map[string]string{"/home/tester/site": "session_limit", "/home/tester/api": "machine_refused", "/home/tester/docs": "machine_refused"}
	for _, m := range run.Members {
		// A conflict may mean the machine already has the agent: the request
		// id is kept. Capacity and denied started nothing.
		if m.State != store.RunMemberFailed || m.Code != want[m.Dir] || m.Sent != (m.Dir == "/home/tester/docs") {
			t.Fatalf("%+v", m)
		}
	}
	_, _, list := f.do(t, f.cookie, "GET", "/api/v2/projects/"+id+"/runs", nil)
	if !strings.Contains(list, "The terminal session limit has been reached.") || !strings.Contains(list, "That machine refused to start the agent.") {
		t.Fatal(list)
	}
	f.mu.Lock()
	f.decide = nil
	f.mu.Unlock()
	if code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/project-runs/"+run.ID+"/members/0/retry", map[string]any{}); code != 200 || out["id"] != run.ID {
		t.Fatal(code, raw)
	}
	run = f.settled(t, run.ID, func(r store.Run) bool { return r.Members[0].State != store.RunMemberPending })
	if m := run.Members[0]; m.State != store.RunMemberStarted || m.Attempt != 2 {
		t.Fatalf("%+v", m)
	}
	// Neither the run nor the retry audit carries the prompt.
	for _, action := range []string{"project-run", "project-run-retry", "terminal-launch"} {
		rows, _ := json.Marshal(worktreeEvents(t, f.st, f.uid, action))
		if len(worktreeEvents(t, f.st, f.uid, action)) == 0 || strings.Contains(string(rows), projectSentinel) {
			t.Fatal(action, string(rows))
		}
	}
	if strings.Contains(f.logs.String(), projectSentinel) {
		t.Fatal("prompt in the log")
	}
}

// The run JSON carries exactly what the worktree status and remove routes
// take: the machine, the Project slug the run used, the repository name the
// machine reported and the member's run token.
func TestProjectRunJSONNamesEachWorktreeForStatusAndRemove(t *testing.T) {
	f := runMachineFixture(t)
	id := f.project(t)
	_, out, _ := f.do(t, f.cookie, "POST", "/api/v2/projects/"+id+"/runs", map[string]any{"requestId": terminalbroker.ID(), "positions": []int{0, 2}})
	runID := out["id"].(string)
	f.settled(t, runID, notRunning)
	// A rename afterwards changes neither the slug the run used nor the run.
	renamed, err := f.st.ProjectFor(f.uid, id)
	if err != nil {
		t.Fatal(err)
	}
	renamed.Name, renamed.Version = "Renamed Shop", 0
	if _, err := f.st.UpdateProjectFor(f.uid, renamed); err != nil {
		t.Fatal(err)
	}
	_, view, raw := f.do(t, f.cookie, "GET", "/api/v2/project-runs/"+runID, nil)
	if view["worktreeProject"] != "shop" {
		t.Fatal(raw)
	}
	creates := f.creates()
	for i, m := range view["members"].([]any) {
		m := m.(map[string]any)
		in := creates[i].Body.(terminalproto.Create)
		if m["worktreeRun"] != in.Worktree || m["worktreeProject"] != in.Project || m["sharesCheckout"] != false || m["host"] != f.agent.ID {
			t.Fatalf("%v %+v", m, in)
		}
		target := map[string]any{"agentId": m["host"], "project": m["worktreeProject"], "repo": m["worktreeRepo"], "run": m["worktreeRun"]}
		code, state, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/worktrees/status", target)
		if code != 200 || state["path"] != m["worktreePath"] || state["branch"] != m["worktreeBranch"] {
			t.Fatal(code, raw)
		}
	}
}

func TestProjectRunStartIsIdempotentAndRefusesAnOverlappingRun(t *testing.T) {
	f := runMachineFixture(t)
	id := f.project(t)
	hold := make(chan struct{})
	f.mu.Lock()
	f.hold = hold
	f.mu.Unlock()
	req := terminalbroker.ID()
	body := map[string]any{"requestId": req, "positions": []int{0, 1}}
	code, first, raw := f.do(t, f.cookie, "POST", "/api/v2/projects/"+id+"/runs", body)
	if code != 201 {
		t.Fatal(code, raw)
	}
	runID := first["id"].(string)
	code, again, raw := f.do(t, f.cookie, "POST", "/api/v2/projects/"+id+"/runs", map[string]any{"requestId": req, "positions": []int{1, 0}})
	if code != 200 || again["id"] != runID {
		t.Fatal("replay", code, raw)
	}
	if code, out, _ := f.do(t, f.cookie, "POST", "/api/v2/projects/"+id+"/runs", map[string]any{"requestId": req, "positions": []int{0}}); code != 409 || out["code"] != "request_changed" || out["error"] != "That request changed. Start it again." {
		t.Fatal("changed", code, out)
	}
	if code, out, _ := f.do(t, f.cookie, "POST", "/api/v2/projects/"+id+"/runs", map[string]any{"requestId": terminalbroker.ID(), "positions": []int{2}}); code != 409 || out["code"] != "run_in_progress" ||
		out["error"] != "This Project is still starting. Wait for it to finish, then start again." {
		t.Fatal("overlap", code, out)
	}
	close(hold)
	run := f.settled(t, runID, notRunning)
	if run.State != store.RunComplete || len(f.creates()) != 2 {
		t.Fatalf("%+v, %d creates", run.Members, len(f.creates()))
	}
	// A replay after it finished still answers with that run, sending nothing.
	if code, again, _ = f.do(t, f.cookie, "POST", "/api/v2/projects/"+id+"/runs", body); code != 200 || again["id"] != runID || again["state"] != "complete" {
		t.Fatal(code, again)
	}
	time.Sleep(30 * time.Millisecond)
	if len(f.creates()) != 2 || len(projectEvents(t, f.st, f.uid, "project-run")) != 1 {
		t.Fatal("a replay started or audited again")
	}
	// Now a new run may start.
	if code, _, raw := f.do(t, f.cookie, "POST", "/api/v2/projects/"+id+"/runs", map[string]any{"requestId": terminalbroker.ID(), "positions": []int{2}}); code != 201 {
		t.Fatal(code, raw)
	}
	// Another account using the same request id gets its own run.
	other, err := f.st.CreateUser("runs-same-id@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	otherAgent, _, err := f.st.EnrollSyncAgent(other.ID, "desk", "test")
	if err != nil {
		t.Fatal(err)
	}
	b := terminalLogin(t, f.st, other.ID)
	theirs := storeProject(t, f.st, other.ID, map[string]any{"name": "Shop", "defaultTool": "claude", "members": []map[string]any{{"host": otherAgent.ID, "dir": "/x"}}})
	code, out, raw := f.do(t, b, "POST", "/api/v2/projects/"+theirs.ID+"/runs", map[string]any{"requestId": req, "positions": []int{0}})
	if code != 201 || out["id"] == runID {
		t.Fatal(code, raw)
	}
}

func TestProjectRunWithoutWorktreesSaysItSharesTheCheckout(t *testing.T) {
	f := runMachineFixture(t)
	p := storeProject(t, f.st, f.uid, map[string]any{"name": "Plain", "defaultTool": "claude", "useWorktrees": false,
		"members": []map[string]any{{"host": f.agent.ID, "dir": "/home/tester/site"}}})
	_, out, raw := f.do(t, f.cookie, "POST", "/api/v2/projects/"+p.ID+"/runs", map[string]any{"requestId": terminalbroker.ID(), "positions": []int{0}})
	m := out["members"].([]any)[0].(map[string]any)
	if out["worktreeProject"] != "" || m["sharesCheckout"] != true || m["worktreeRun"] != "" || m["worktreeProject"] != "" {
		t.Fatal(raw)
	}
	f.settled(t, out["id"].(string), notRunning)
	if in := f.creates()[0].Body.(terminalproto.Create); in.Worktree != "" || in.Project != "" {
		t.Fatal(in)
	}
}

func TestProjectRunConcurrentStartsMakeOneRun(t *testing.T) {
	f := runMachineFixture(t)
	id := f.project(t)
	hold := make(chan struct{})
	f.mu.Lock()
	f.hold = hold
	f.mu.Unlock()
	req := terminalbroker.ID()
	var wg sync.WaitGroup
	var mu sync.Mutex
	codes := map[int]int{}
	runs := map[string]bool{}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := req
			if i%2 == 1 {
				r = terminalbroker.ID() // a different request for the same Project
			}
			code, out, _ := f.do(t, f.cookie, "POST", "/api/v2/projects/"+id+"/runs", map[string]any{"requestId": r, "positions": []int{0, 1}})
			mu.Lock()
			codes[code]++
			if code < 300 {
				runs[out["id"].(string)] = true
			}
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	close(hold)
	if len(runs) != 1 || codes[201] != 1 || codes[201]+codes[200]+codes[409] != 8 {
		t.Fatal(codes, runs)
	}
	for runID := range runs {
		f.settled(t, runID, notRunning)
	}
	if len(f.creates()) != 2 {
		t.Fatal("creates", len(f.creates()))
	}
}

func TestProjectRunResumeWatcherEndsWhenTheServerCloses(t *testing.T) {
	f := runMachineFixture(t)
	f.srv.ResumeProjectRuns(context.Background()) // a context that never ends
	done := f.srv.s.resumeDone
	f.srv.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the resume watcher outlived the Server")
	}
}

func TestProjectRunMemberBecomesRemovedWhenItsWorktreeIsRemoved(t *testing.T) {
	f := runMachineFixture(t)
	id := f.project(t)
	_, out, _ := f.do(t, f.cookie, "POST", "/api/v2/projects/"+id+"/runs", map[string]any{"requestId": terminalbroker.ID(), "positions": []int{0, 1}})
	runID := out["id"].(string)
	f.settled(t, runID, notRunning)
	_, view, _ := f.do(t, f.cookie, "GET", "/api/v2/project-runs/"+runID, nil)
	m := view["members"].([]any)[0].(map[string]any)
	target := map[string]any{"agentId": m["host"], "project": m["worktreeProject"], "repo": m["worktreeRepo"], "run": m["worktreeRun"], "changed": 0, "unpushed": 0}
	// Another account cannot remove it, and the member stays started.
	other, err := f.st.CreateUser("runs-remove-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if code, _, _ := f.do(t, terminalLogin(t, f.st, other.ID), "POST", "/api/v2/terminal/worktrees/remove", target); code < 400 {
		t.Fatal("another account removed it", code)
	}
	if code, _, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/worktrees/remove", target); code != 200 {
		t.Fatal(code, raw)
	}
	_, view, raw := f.do(t, f.cookie, "GET", "/api/v2/project-runs/"+runID, nil)
	members := view["members"].([]any)
	if members[0].(map[string]any)["state"] != "removed" || members[0].(map[string]any)["sentence"] != "" ||
		members[1].(map[string]any)["state"] != "started" || view["state"] != "complete" {
		t.Fatal(raw)
	}
	// Removing a worktree no run made changes nothing.
	f.mu.Lock()
	f.worktrees["shop/elsewhere/"+strings.Repeat("e", 24)] = "/home/tester/.reet/worktrees/shop/elsewhere/" + strings.Repeat("e", 24)
	f.mu.Unlock()
	stray := map[string]any{"agentId": f.agent.ID, "project": "shop", "repo": "elsewhere", "run": strings.Repeat("e", 24), "changed": 0, "unpushed": 0}
	if code, _, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/worktrees/remove", stray); code != 200 {
		t.Fatal(code, raw)
	}
	if run, _ := f.st.RunFor(f.uid, runID); run.Members[1].State != store.RunMemberStarted {
		t.Fatal(run.Members)
	}
}

// No refusal ever repeats the shared prompt it was sent.
func TestProjectRefusalsNeverEchoTheSharedPrompt(t *testing.T) {
	f := runMachineFixture(t)
	id := f.project(t)
	run, err := f.st.CreateRunFor(f.uid, id, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.SetRunMemberState(f.uid, run.ID, 0, store.RunMemberFailed, "folder_unavailable", "", "", "", "", 1); err != nil {
		t.Fatal(err)
	}
	p := &projectAPI{launchAPI: f.launchAPI}
	quoted, _ := json.Marshal(projectSentinel)
	S := string(quoted[1 : len(quoted)-1])
	member := `{"host":"` + f.agent.ID + `","dir":"/home/tester/site"}`
	bodies := []string{
		`{"name":"","defaultTool":"claude","sharedPrompt":"` + S + `"}`,
		`{"name":"P1","defaultTool":"claude","sharedPrompt":"-` + S + `"}`,
		`{"name":"P2","defaultTool":"claude","sharedPrompt":"` + S + strings.Repeat("x", 2100) + `"}`,
		`{"name":"P3","defaultTool":"claude","sharedPrompt":"` + S + `\u0007"}`,
		`{"name":"P4","defaultTool":"nope","sharedPrompt":"` + S + `"}`,
		`{"name":"P5","defaultTool":"claude","sharedPrompt":"` + S + `","bogus":1}`,
		`{"name":"Shop","defaultTool":"claude","sharedPrompt":"` + S + `"}`,
		`{"name":"P6","defaultTool":"claude","sharedPrompt":"` + S + `","members":[{"host":"nobody","dir":"/x"}]}`,
		`{"name":"P7","defaultTool":"claude","sharedPrompt":"` + S + `","members":[` + member + `,` + member + `]}`,
		`{"name":"P8","defaultTool":"claude","sharedPrompt":"` + S + `","members":[{"host":"` + f.agent.ID + `","dir":"relative"}]}`,
		`{"name":"P9","defaultTool":"claude","sharedPrompt":"` + strings.Repeat(S, 3000) + `"}`,
		`{"name":"P10","defaultTool":"claude","sharedPrompt":"` + S + `"} trailing`,
	}
	origin := "https://reet.example"
	check := func(method, path, body string) {
		t.Helper()
		code, raw := p.raw(t, method, path, body, f.cookie, origin, "")
		if code < 400 {
			t.Fatal("accepted", method, path, code, body[:min(len(body), 80)])
		}
		if strings.Contains(raw, "zebra-sentinel") {
			t.Fatal("the prompt was echoed", method, path, code, raw)
		}
	}
	for _, b := range bodies {
		check("POST", "/api/v2/projects", b)
		if !strings.Contains(b, `"name":"Shop"`) {
			check("PATCH", "/api/v2/projects/"+id, b)
		}
	}
	for _, b := range []string{
		`{"requestId":"` + terminalbroker.ID() + `","positions":[0],"prompt":"` + S + `"}`,
		`{"requestId":"` + S + `","positions":[0]}`,
		`{"requestId":"` + terminalbroker.ID() + `","positions":[0],"sharedPrompt":"` + S + `"}`,
	} {
		check("POST", "/api/v2/projects/"+id+"/runs", b)
	}
	check("POST", "/api/v2/project-runs/"+run.ID+"/members/0/retry", `{"prompt":"`+S+`"}`)
	check("POST", "/api/v2/project-runs/"+run.ID+"/members/9/retry", `{}`)
	if strings.Contains(f.logs.String(), "zebra-sentinel") {
		t.Fatal("the prompt reached the log")
	}
}

func TestProjectRunStartHonoursIfMatch(t *testing.T) {
	f := runMachineFixture(t)
	id := f.project(t)
	start := func(version string, body map[string]any) (int, map[string]any, string) {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "/api/v2/projects/"+id+"/runs", strings.NewReader(string(data)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "https://reet.example")
		if version != "" {
			r.Header.Set("If-Match", version)
		}
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
		w := httptest.NewRecorder()
		f.srv.ServeHTTP(w, r)
		out := map[string]any{}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out, w.Body.String()
	}
	// Edited in another tab after the sheet was made from version 1. The edit drops a member, so the
	// sheet's positions no longer all exist: the answer is still that the Project changed.
	edit := func(change func(*store.Project)) {
		t.Helper()
		p, err := f.st.ProjectFor(f.uid, id)
		if err != nil {
			t.Fatal(err)
		}
		change(&p)
		p.Version = 0
		if _, err := f.st.UpdateProjectFor(f.uid, p); err != nil {
			t.Fatal("edit", err)
		}
	}
	edit(func(p *store.Project) {
		p.Members = []store.ProjectMember{{Host: f.agent.ID, Dir: "/home/tester/elsewhere"}}
	})
	req := terminalbroker.ID()
	for _, positions := range [][]int{{0}, {0, 2}} {
		code, out, raw := start(`"1"`, map[string]any{"requestId": req, "positions": positions})
		if code != 409 || out["code"] != "project_changed" || out["error"] != "Someone changed this Project. Reload it and try again." {
			t.Fatal("stale start", positions, code, raw)
		}
	}
	if runs, _ := f.st.RunsFor(f.uid, id, 20); len(runs) != 0 || len(projectEvents(t, f.st, f.uid, "project-run")) != 0 {
		t.Fatal("a stale start created or audited a run", runs)
	}
	time.Sleep(30 * time.Millisecond)
	if len(f.creates()) != 0 {
		t.Fatal("a stale start reached the machine")
	}
	for _, bad := range []string{"abc", "0", `W/"2"`} {
		if code, out, raw := start(bad, map[string]any{"requestId": req, "positions": []int{0}}); code != 400 || out["code"] != "invalid_version" {
			t.Fatal("bad If-Match", bad, code, raw)
		}
	}
	code, first, raw := start(`"2"`, map[string]any{"requestId": req, "positions": []int{0}})
	if code != 201 {
		t.Fatal("current start", code, raw)
	}
	runID := first["id"].(string)
	f.settled(t, runID, notRunning)
	// Edited again: the same request resent with the version it was made from still answers with its run.
	edit(func(p *store.Project) { p.Name = "Shop, renamed" })
	if code, again, raw := start(`"2"`, map[string]any{"requestId": req, "positions": []int{0}}); code != 200 || again["id"] != runID {
		t.Fatal("replay after an edit", code, raw)
	}
	// Without If-Match a start goes ahead against the saved Project, as an update does.
	if code, _, raw := start("", map[string]any{"requestId": terminalbroker.ID(), "positions": []int{0}}); code != 201 {
		t.Fatal("no header", code, raw)
	}
}
