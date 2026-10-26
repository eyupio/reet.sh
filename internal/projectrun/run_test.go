package projectrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// sentinel is the shared prompt; it must reach the broker's create and
// nothing else.
const sentinel = "quokka-sentinel shared prompt for every repository"

type sent struct {
	agent, request string
	in             terminalproto.Create
}

type answer struct {
	err   error
	state terminalproto.SessionState // default Running
}

// fakeBroker records every create. answer picks the outcome per call; when
// gate is set, every call waits for a token from it (or for cancellation).
type fakeBroker struct {
	mu          sync.Mutex
	calls       []sent
	inFlight    int
	peak        int
	notReady    map[string]bool
	answer      func(sent) answer
	gate        chan struct{}
	sessions    map[string]string // request id -> session id
	nextSession int
}

func (f *fakeBroker) Ready(uid int64, agent string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.notReady[agent]
}

func (f *fakeBroker) setReady(agent string, ready bool) {
	f.mu.Lock()
	if f.notReady == nil {
		f.notReady = map[string]bool{}
	}
	f.notReady[agent] = !ready
	f.mu.Unlock()
}

func (f *fakeBroker) CreateSession(ctx context.Context, uid int64, agent, request string, in terminalproto.Create) (store.TerminalSession, bool, error) {
	c := sent{agent, request, in}
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.inFlight++
	f.peak = max(f.peak, f.inFlight)
	gate, decide := f.gate, f.answer
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.inFlight--; f.mu.Unlock() }()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return store.TerminalSession{}, false, terminalbroker.ErrWorktreeReplyLost
		}
	}
	a := answer{}
	if decide != nil {
		a = decide(c)
	}
	if a.err != nil {
		return store.TerminalSession{}, false, a.err
	}
	if a.state == "" {
		a.state = terminalproto.Running
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sessions == nil {
		f.sessions = map[string]string{}
	}
	id, replay := f.sessions[request]
	if !replay {
		f.nextSession++
		id = fmt.Sprintf("session-%016d", f.nextSession)
		f.sessions[request] = id
	}
	s := store.TerminalSession{ID: id, AgentID: agent, State: a.state}
	if in.Worktree != "" {
		repo := terminalproto.Slug(in.Dir[strings.LastIndex(in.Dir, "/")+1:], "repo")
		s.WorktreePath = "/home/t/.reet/worktrees/" + in.Project + "/" + repo + "/" + in.Worktree
		s.WorktreeRepo = repo
		s.WorktreeBranch = "reet/" + in.Project + "-" + in.Worktree
	}
	return s, !replay && a.state == terminalproto.Running, nil
}

func (f *fakeBroker) sent() []sent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sent(nil), f.calls...)
}

func (f *fakeBroker) inflight() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inFlight
}

type fixture struct {
	st      *store.Store
	uid     int64
	laptop  string
	server  string
	broker  *fakeBroker
	logs    *strings.Builder
	logMu   sync.Mutex
	enabled bool
	mu      sync.Mutex
	started []string // session ids the Launched hook heard of
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	uid, err := st.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	laptop, _, err := st.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	server, _, err := st.EnrollSyncAgent(uid, "server", "test")
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{st: st, uid: uid, laptop: laptop.ID, server: server.ID, broker: &fakeBroker{}, logs: &strings.Builder{}, enabled: true}
}

func (f *fixture) orchestrator(t *testing.T, cfg ...func(*Config)) *Orchestrator {
	t.Helper()
	c := Config{Store: f.st, Broker: f.broker, Poll: 5 * time.Millisecond,
		Enabled: func() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.enabled },
		Launched: func(uid int64, agent string, in terminalproto.Create, s store.TerminalSession) {
			f.mu.Lock()
			f.started = append(f.started, s.ID)
			f.mu.Unlock()
		},
		Logf: func(format string, args ...any) {
			f.logMu.Lock()
			fmt.Fprintf(f.logs, format+"\n", args...)
			f.logMu.Unlock()
		}}
	for _, fn := range cfg {
		fn(&c)
	}
	o := New(c)
	t.Cleanup(o.Close)
	return o
}

// project saves a Project whose members are the given (machine, folder) pairs.
func (f *fixture) project(t *testing.T, worktrees bool, members ...[2]string) store.Project {
	t.Helper()
	p := store.Project{Name: "Shop Front", DefaultTool: "claude", DefaultModel: "sonnet", SharedPrompt: sentinel, UseWorktrees: worktrees}
	for _, m := range members {
		p.Members = append(p.Members, store.ProjectMember{Host: m[0], Dir: m[1]})
	}
	saved, err := f.st.CreateProjectFor(f.uid, p)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func (f *fixture) run(t *testing.T, p store.Project, positions ...int) store.Run {
	t.Helper()
	r, err := f.st.CreateRunFor(f.uid, p.ID, positions...)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// settle waits until the orchestrator is idle and returns the run.
func settle(t *testing.T, o *Orchestrator, st *store.Store, uid int64, runID string) store.Run {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		o.mu.Lock()
		idle := len(o.runs) == 0
		o.mu.Unlock()
		if idle {
			r, err := st.RunFor(uid, runID)
			if err != nil {
				t.Fatal(err)
			}
			return r
		}
		if time.Now().After(deadline) {
			t.Fatal("orchestrator never settled")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for " + what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func member(r store.Run, position int) store.RunMember {
	for _, m := range r.Members {
		if m.Position == position {
			return m
		}
	}
	return store.RunMember{}
}

func TestDerivedRequestIDsAndTokens(t *testing.T) {
	run := "0123456789abcdef0123456789abcdef"
	if got := RequestID(run, 3, 2); len(got) != 48 || got != RequestID(run, 3, 2) {
		t.Fatal(got)
	}
	// SHA-256("run|position|attempt"), first 48 hex characters.
	if !terminalproto.ValidID(RequestID("r", 1, 1)) {
		t.Fatal("request id must be a valid id")
	}
	seen := map[string]bool{}
	for pos := 0; pos < 12; pos++ {
		for attempt := 1; attempt <= 3; attempt++ {
			id := RequestID(run, pos, attempt)
			if seen[id] {
				t.Fatalf("request id repeats at %d/%d", pos, attempt)
			}
			seen[id] = true
		}
		tok := WorktreeToken(run, pos)
		if len(tok) != 24 || !terminalproto.ValidRunToken(tok) || seen[tok] {
			t.Fatalf("token %q", tok)
		}
		seen[tok] = true
	}
	if WorktreeToken(run, 0) == WorktreeToken("ffffffffffffffffffffffffffffffff", 0) {
		t.Fatal("tokens must differ per run")
	}
	if Title("Shop", "/home/a/site") != "Shop · site" || Title("Shop", "/") != "Shop" || len([]rune(Title(strings.Repeat("x", 80), "/a/b"))) != 80 {
		t.Fatal(Title("Shop", "/home/a/site"))
	}
}

func TestRequestIDIsTheDocumentedHash(t *testing.T) {
	// echo -n 'run|0|1' | sha256sum, cut to 48; echo -n 'worktree|run|0' | sha256sum, cut to 24.
	if got, want := RequestID("run", 0, 1), sha("run|0|1")[:48]; got != want {
		t.Fatal(got, want)
	}
	if got, want := WorktreeToken("run", 0), sha("worktree|run|0")[:24]; got != want {
		t.Fatal(got, want)
	}
}

func TestRunStartsEveryMemberWithItsOwnRequestAndWorktree(t *testing.T) {
	f := newFixture(t)
	o := f.orchestrator(t)
	p := f.project(t, true, [2]string{f.laptop, "/home/t/site"}, [2]string{f.laptop, "/home/t/api"}, [2]string{f.server, "/srv/site"})
	r := f.run(t, p)
	o.Start(f.uid, r.ID)
	got := settle(t, o, f.st, f.uid, r.ID)
	if got.State != store.RunComplete {
		t.Fatalf("%+v", got)
	}
	calls := f.broker.sent()
	if len(calls) != 3 {
		t.Fatalf("%d creates", len(calls))
	}
	tokens := map[string]bool{}
	for _, c := range calls {
		var pos = -1
		for _, m := range got.Members {
			if m.Dir == c.in.Dir {
				pos = m.Position
			}
		}
		m := member(got, pos)
		if c.request != RequestID(r.ID, pos, 1) || c.in.Worktree != WorktreeToken(r.ID, pos) || c.in.Project != "shop-front" ||
			c.in.Prompt != sentinel || c.in.Tool != "claude" || c.in.Model != "sonnet" || c.agent != m.Host || c.in.Validate() != nil {
			t.Fatalf("create for %d: %+v", pos, c)
		}
		tokens[c.in.Worktree] = true
		if m.State != store.RunMemberStarted || m.SessionID == "" || m.WorktreeToken != c.in.Worktree ||
			m.WorktreePath == "" || m.WorktreeRepo == "" || m.WorktreeBranch != "reet/shop-front-"+c.in.Worktree || m.Attempt != 1 {
			t.Fatalf("member %+v", m)
		}
	}
	if len(tokens) != 3 {
		t.Fatal("two members shared a worktree token")
	}
	if len(f.started) != 3 {
		t.Fatalf("launched %v", f.started)
	}
}

func TestRunWithoutWorktreesSendsNoWorktreeFields(t *testing.T) {
	f := newFixture(t)
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, false, [2]string{f.laptop, "/home/t/site"}))
	o.Start(f.uid, r.ID)
	got := settle(t, o, f.st, f.uid, r.ID)
	c := f.broker.sent()
	if len(c) != 1 || c[0].in.Worktree != "" || c[0].in.Project != "" || got.Members[0].WorktreeToken != "" || got.State != store.RunComplete {
		t.Fatalf("%+v %+v", c, got)
	}
}

func TestRunHasAtMostTwoInFlight(t *testing.T) {
	f := newFixture(t)
	f.broker.gate = make(chan struct{})
	o := f.orchestrator(t)
	p := f.project(t, true, [2]string{f.laptop, "/a/1"}, [2]string{f.laptop, "/a/2"}, [2]string{f.laptop, "/a/3"}, [2]string{f.server, "/a/4"}, [2]string{f.server, "/a/5"})
	r := f.run(t, p)
	o.Start(f.uid, r.ID)
	for released := 0; released < 5; released++ {
		want := min(2, 5-released)
		waitFor(t, "creates in flight", func() bool { return f.broker.inflight() == want })
		time.Sleep(20 * time.Millisecond) // a third would have arrived by now
		if n := f.broker.inflight(); n != want {
			t.Fatalf("%d in flight after %d released", n, released)
		}
		if n := len(f.broker.sent()); n != released+want {
			t.Fatalf("%d sent after %d released", n, released)
		}
		f.broker.gate <- struct{}{}
	}
	got := settle(t, o, f.st, f.uid, r.ID)
	if got.State != store.RunComplete || f.broker.peak != 2 {
		t.Fatalf("peak %d, %+v", f.broker.peak, got)
	}
	// Members are started in position order: the first two go first.
	first := map[string]bool{}
	for _, c := range f.broker.sent()[:2] {
		first[c.request] = true
	}
	if !first[RequestID(r.ID, 0, 1)] || !first[RequestID(r.ID, 1, 1)] {
		t.Fatal("members were not started in position order")
	}
}

func TestRunStartsOnlyTheChosenMembers(t *testing.T) {
	f := newFixture(t)
	o := f.orchestrator(t)
	p := f.project(t, true, [2]string{f.laptop, "/a/0"}, [2]string{f.laptop, "/a/1"}, [2]string{f.server, "/a/2"})
	r := f.run(t, p, 2, 0)
	o.Start(f.uid, r.ID)
	got := settle(t, o, f.st, f.uid, r.ID)
	calls := f.broker.sent()
	byRequest := map[string]sent{}
	for _, c := range calls {
		byRequest[c.request] = c
	}
	if len(got.Members) != 2 || len(calls) != 2 || byRequest[RequestID(r.ID, 0, 1)].in.Dir != "/a/0" || byRequest[RequestID(r.ID, 2, 1)].in.Worktree != WorktreeToken(r.ID, 2) {
		t.Fatalf("%+v %+v", got.Members, calls)
	}
}

func TestOneMembersFailureNeverStopsTheOthersAndEachGetsItsCode(t *testing.T) {
	f := newFixture(t)
	// The laptop reports claude installed; the server reports nothing.
	caps := terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "t", TerminalControl: true, TerminalEnabled: true, Readiness: terminalproto.Ready, Launch: true, Worktree: true, Tools: []string{"claude"}}
	if _, err := f.st.BeginRemoteConnectionFor(f.uid, f.laptop, terminalbroker.ID(), caps); err != nil {
		t.Fatal(err)
	}
	if _, err := f.st.BeginRemoteConnectionFor(f.uid, f.server, terminalbroker.ID(), terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "t", TerminalControl: true, TerminalEnabled: true, Readiness: terminalproto.Ready, Launch: true}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		host, dir string
		err       error
		state     terminalproto.SessionState
		code      string
	}{
		{f.laptop, "/c/ok-first", nil, "", ""},
		{f.laptop, "/c/launch", terminalbroker.ErrLaunchUnsupported, "", CodeLaunchUnsupported},
		{f.laptop, "/c/worktree", terminalbroker.ErrWorktreeUnsupported, "", CodeWorktreeUnsupported},
		{f.laptop, "/c/exists", &terminalbroker.WorktreeError{Reason: terminalproto.WorktreeExists}, "", "worktree_exists"},
		{f.laptop, "/c/root", &terminalbroker.WorktreeError{Reason: terminalproto.WorktreeNotRepoRoot}, "", "not_repo_root"},
		{f.laptop, "/c/busy", &terminalbroker.WorktreeError{Reason: terminalproto.WorktreeRepoBusy}, "", "repo_busy"},
		{f.laptop, "/c/git", &terminalbroker.WorktreeError{Reason: terminalproto.WorktreeGitFailed, Detail: "fatal"}, "", "git_failed"},
		{f.laptop, "/c/home", &terminalbroker.WorktreeError{Reason: terminalproto.WorktreeOutsideHome}, "", "outside_home"},
		{f.laptop, "/c/folder", terminalbroker.ErrLaunchFailed, "", CodeFolderUnavailable},
		{f.server, "/c/tool", terminalbroker.ErrLaunchFailed, "", CodeToolNotInstalled},
		{f.server, "/c/limit", store.ErrTerminalCapacity, "", CodeSessionLimit},
		{f.server, "/c/lost", nil, terminalproto.Lost, CodeSessionLost},
	}
	byDir := map[string]answer{}
	var members [][2]string
	for _, c := range cases {
		byDir[c.dir] = answer{err: c.err, state: c.state}
		members = append(members, [2]string{c.host, c.dir})
	}
	f.broker.answer = func(s sent) answer { return byDir[s.in.Dir] }
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, members...))
	o.Start(f.uid, r.ID)
	got := settle(t, o, f.st, f.uid, r.ID)
	for i, c := range cases {
		m := member(got, i)
		if c.code == "" {
			if m.State != store.RunMemberStarted {
				t.Fatalf("%s: %+v", c.dir, m)
			}
			continue
		}
		if m.State != store.RunMemberFailed || m.Code != c.code || m.SessionID != "" {
			t.Fatalf("%s: want %s, got %+v", c.dir, c.code, m)
		}
	}
	if got.State != store.RunPartial || len(f.broker.sent()) != len(cases) {
		t.Fatalf("%s, %d sent", got.State, len(f.broker.sent()))
	}
}

func TestSessionLimitMarksTheRestOnThatMachineButNotOthers(t *testing.T) {
	f := newFixture(t)
	f.broker.answer = func(s sent) answer {
		if s.agent == f.laptop {
			return answer{err: store.ErrTerminalCapacity}
		}
		return answer{}
	}
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/1"}, [2]string{f.server, "/a/2"}, [2]string{f.laptop, "/a/3"}, [2]string{f.laptop, "/a/4"}))
	o.Start(f.uid, r.ID)
	got := settle(t, o, f.st, f.uid, r.ID)
	for _, m := range got.Members {
		want := CodeSessionLimit
		if m.Host == f.server {
			want = ""
		}
		if m.Code != want {
			t.Fatalf("%+v", m)
		}
	}
}

func TestMembersOnUnknownUnreadyOrDisabledMachinesAreNeverSent(t *testing.T) {
	f := newFixture(t)
	other, err := f.st.CreateUser("b@example.com", "b", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	stranger, _, err := f.st.EnrollSyncAgent(other.ID, "theirs", "test")
	if err != nil {
		t.Fatal(err)
	}
	f.broker.setReady(f.server, false)
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{stranger.ID, "/a/1"}, [2]string{f.server, "/a/2"}, [2]string{f.laptop, "/a/3"}))
	o.Start(f.uid, r.ID)
	got := settle(t, o, f.st, f.uid, r.ID)
	if c := member(got, 0).Code; c != CodeUnknownMachine {
		t.Fatal(c)
	}
	if c := member(got, 1).Code; c != CodeMachineUnavailable {
		t.Fatal(c)
	}
	if member(got, 2).State != store.RunMemberStarted {
		t.Fatalf("%+v", member(got, 2))
	}
	for _, c := range f.broker.sent() {
		if c.agent != f.laptop {
			t.Fatalf("sent to %s", c.agent)
		}
	}

	f.mu.Lock()
	f.enabled = false
	f.mu.Unlock()
	r2 := f.run(t, f.project2(t))
	o.Start(f.uid, r2.ID)
	got = settle(t, o, f.st, f.uid, r2.ID)
	if c := member(got, 0).Code; c != CodeTerminalsDisabled || len(f.broker.sent()) != 1 {
		t.Fatal(c, len(f.broker.sent()))
	}
}

func (f *fixture) project2(t *testing.T) store.Project {
	t.Helper()
	p, err := f.st.CreateProjectFor(f.uid, store.Project{Name: "Second", DefaultTool: "claude", SharedPrompt: sentinel, Members: []store.ProjectMember{{Host: f.laptop, Dir: "/b/1"}}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRetryAfterAnUnknownOutcomeReusesTheRequestID(t *testing.T) {
	for _, lost := range []error{terminalbroker.ErrWorktreeReplyLost, terminalbroker.ErrOffline, context.DeadlineExceeded, terminalbroker.ErrConflict, errors.New("anything else")} {
		f := newFixture(t)
		calls := 0
		f.broker.answer = func(s sent) answer {
			calls++
			if calls == 1 {
				return answer{err: lost}
			}
			return answer{}
		}
		o := f.orchestrator(t)
		r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/1"}))
		o.Start(f.uid, r.ID)
		got := settle(t, o, f.st, f.uid, r.ID)
		if m := got.Members[0]; m.State != store.RunMemberFailed || m.Code != CodeUnknownOutcome || m.Attempt != 1 {
			t.Fatalf("%v: %+v", lost, m)
		}
		m, err := o.Retry(f.uid, r.ID, 0)
		if err != nil || m.Attempt != 1 {
			t.Fatal(err, m)
		}
		got = settle(t, o, f.st, f.uid, r.ID)
		sentNow := f.broker.sent()
		if len(sentNow) != 2 || sentNow[0].request != RequestID(r.ID, 0, 1) || sentNow[1].request != sentNow[0].request || sentNow[1].in != sentNow[0].in {
			t.Fatalf("%v: %+v", lost, sentNow)
		}
		if got.Members[0].State != store.RunMemberStarted || got.Members[0].Attempt != 1 {
			t.Fatalf("%+v", got.Members[0])
		}
	}
}

func TestRetryAfterADefiniteRefusalUsesTheNextAttempt(t *testing.T) {
	f := newFixture(t)
	calls := 0
	f.broker.answer = func(s sent) answer {
		calls++
		if calls == 1 {
			return answer{err: &terminalbroker.WorktreeError{Reason: terminalproto.WorktreeRepoBusy}}
		}
		return answer{}
	}
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/1"}))
	o.Start(f.uid, r.ID)
	settle(t, o, f.st, f.uid, r.ID)
	if m, err := o.Retry(f.uid, r.ID, 0); err != nil || m.Attempt != 2 {
		t.Fatal(err, m)
	}
	got := settle(t, o, f.st, f.uid, r.ID)
	s := f.broker.sent()
	if len(s) != 2 || s[0].request != RequestID(r.ID, 0, 1) || s[1].request != RequestID(r.ID, 0, 2) || s[1].in.Worktree != s[0].in.Worktree {
		t.Fatalf("%+v", s)
	}
	if m := got.Members[0]; m.State != store.RunMemberStarted || m.Attempt != 2 || m.WorktreeToken != WorktreeToken(r.ID, 0) {
		t.Fatalf("%+v", m)
	}
}

func TestRetryAfterAWorktreeMismatchReusesTheRequestID(t *testing.T) {
	f := newFixture(t)
	calls := 0
	f.broker.answer = func(s sent) answer {
		calls++
		if calls == 1 {
			return answer{err: terminalbroker.ErrWorktreeMismatch}
		}
		return answer{}
	}
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/1"}))
	o.Start(f.uid, r.ID)
	if m := settle(t, o, f.st, f.uid, r.ID).Members[0]; m.Code != CodeWorktreeMismatch {
		t.Fatal(m)
	}
	if m, err := o.Retry(f.uid, r.ID, 0); err != nil || m.Attempt != 1 {
		t.Fatal(err, m)
	}
	settle(t, o, f.st, f.uid, r.ID)
	if s := f.broker.sent(); len(s) != 2 || s[1].request != s[0].request {
		t.Fatalf("%+v", s)
	}
}

func TestRetryRefusesMembersThatDidNotFail(t *testing.T) {
	f := newFixture(t)
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/1"}))
	o.Start(f.uid, r.ID)
	settle(t, o, f.st, f.uid, r.ID)
	if _, err := o.Retry(f.uid, r.ID, 0); !errors.Is(err, ErrNotFailed) {
		t.Fatal(err)
	}
	if _, err := o.Retry(f.uid, r.ID, 7); !errors.Is(err, ErrNoSuchMember) {
		t.Fatal(err)
	}
	other, _ := f.st.CreateUser("b@example.com", "b", "member", "hash")
	if _, err := o.Retry(other.ID, r.ID, 0); !errors.Is(err, store.ErrRunNotFound) {
		t.Fatal(err)
	}
	if len(f.broker.sent()) != 1 {
		t.Fatal("a refused retry sent a create")
	}
}

func TestShutdownLeavesMembersPendingAndResumeStartsOnlyThoseWithTheSameRequestIDs(t *testing.T) {
	f := newFixture(t)
	gate := make(chan struct{})
	f.broker.gate = gate
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/0"}, [2]string{f.laptop, "/a/1"}, [2]string{f.server, "/a/2"}, [2]string{f.server, "/a/3"}))
	o.Start(f.uid, r.ID)
	waitFor(t, "two creates", func() bool { return f.broker.inflight() == 2 })
	gate <- struct{}{} // one of the first two starts
	waitFor(t, "the third create", func() bool { return len(f.broker.sent()) == 3 && f.broker.inflight() == 2 })
	o.Close() // the controller stops with two calls in flight and one never sent
	got, err := f.st.RunFor(f.uid, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	startedBefore := ""
	pending := 0
	for _, m := range got.Members {
		switch m.State {
		case store.RunMemberStarted:
			startedBefore = m.SessionID
		case store.RunMemberPending:
			pending++
		default:
			t.Fatalf("shutdown failed a member: %+v", m)
		}
	}
	if startedBefore == "" || pending != 3 {
		t.Fatalf("%+v", got.Members)
	}
	first := f.broker.sent()

	// A new controller on the same database: nothing is claimed any more.
	f.broker = &fakeBroker{nextSession: 100}
	o2 := f.orchestrator(t)
	if err := o2.Resume(); err != nil {
		t.Fatal(err)
	}
	got = settle(t, o2, f.st, f.uid, r.ID)
	if got.State != store.RunComplete {
		t.Fatalf("%+v", got)
	}
	again := f.broker.sent()
	if len(again) != 3 {
		t.Fatalf("resume sent %d creates", len(again))
	}
	sentBefore := map[string]bool{}
	for _, c := range first {
		sentBefore[c.request] = true
	}
	for _, c := range again {
		var pos int
		fmt.Sscanf(c.in.Dir, "/a/%d", &pos)
		if member(got, pos).SessionID == startedBefore {
			t.Fatal("a started member was started again")
		}
		if c.request != RequestID(r.ID, pos, 1) || c.in.Worktree != WorktreeToken(r.ID, pos) {
			t.Fatalf("resumed with a new identity: %+v", c)
		}
		if pos < 3 && !sentBefore[c.request] {
			t.Fatalf("an in-flight member was resumed with a different request id")
		}
	}
	// Nothing pending is left to resume, and a second resume sends nothing.
	if err := o2.Resume(); err != nil {
		t.Fatal(err)
	}
	settle(t, o2, f.st, f.uid, r.ID)
	if len(f.broker.sent()) != 3 {
		t.Fatal("a second resume sent again")
	}
}

func TestResumeWaitsForTheMachineToReconnect(t *testing.T) {
	f := newFixture(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/0"}, [2]string{f.server, "/a/1"}))
	f.broker.setReady(f.laptop, false)
	f.broker.setReady(f.server, false)
	o := f.orchestrator(t, func(c *Config) { c.ResumeWait = 300 * time.Millisecond })
	if err := o.Resume(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	f.broker.setReady(f.laptop, true) // the laptop reconnects; the server never does
	got := settle(t, o, f.st, f.uid, r.ID)
	if member(got, 0).State != store.RunMemberStarted || member(got, 1).Code != CodeMachineUnavailable {
		t.Fatalf("%+v", got.Members)
	}
}

func TestTwoOrchestratorsNeverStartOneMemberTwice(t *testing.T) {
	f := newFixture(t)
	f.broker.gate = make(chan struct{})
	a, b := f.orchestrator(t), f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/0"}, [2]string{f.laptop, "/a/1"}, [2]string{f.server, "/a/2"}))
	a.Start(f.uid, r.ID)
	b.Start(f.uid, r.ID)
	a.Start(f.uid, r.ID)
	waitFor(t, "creates", func() bool { return f.broker.inflight() >= 2 })
	time.Sleep(20 * time.Millisecond)
	close(f.broker.gate)
	settle(t, a, f.st, f.uid, r.ID)
	got := settle(t, b, f.st, f.uid, r.ID)
	seen := map[string]bool{}
	for _, c := range f.broker.sent() {
		if seen[c.request] {
			t.Fatalf("member sent twice: %s", c.in.Dir)
		}
		seen[c.request] = true
	}
	if got.State != store.RunComplete || len(seen) != 3 {
		t.Fatalf("%+v %d", got, len(seen))
	}
}

func TestDuplicateRepositoriesAreRefusedButOneMachineRunsTwoMembersApart(t *testing.T) {
	f := newFixture(t)
	if _, err := f.st.CreateProjectFor(f.uid, store.Project{Name: "Dup", DefaultTool: "claude", Members: []store.ProjectMember{{Host: f.laptop, Dir: "/a/x"}, {Host: f.laptop, Dir: "/a/x/"}}}); !errors.Is(err, store.ErrDuplicateMember) {
		t.Fatal(err)
	}
	f.broker.gate = make(chan struct{})
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/x"}, [2]string{f.laptop, "/a/y"}))
	o.Start(f.uid, r.ID)
	waitFor(t, "both in flight together", func() bool { return f.broker.inflight() == 2 })
	close(f.broker.gate)
	settle(t, o, f.st, f.uid, r.ID)
	s := f.broker.sent()
	if s[0].in.Worktree == s[1].in.Worktree || s[0].request == s[1].request {
		t.Fatal("two members shared a token or a request id")
	}
}

func TestThePromptReachesOnlyTheCreate(t *testing.T) {
	f := newFixture(t)
	f.broker.answer = func(s sent) answer {
		if strings.HasSuffix(s.in.Dir, "1") {
			return answer{err: &terminalbroker.WorktreeError{Reason: terminalproto.WorktreeGitFailed, Detail: "fatal: bad"}}
		}
		return answer{}
	}
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/0"}, [2]string{f.laptop, "/a/1"}))
	o.Start(f.uid, r.ID)
	got := settle(t, o, f.st, f.uid, r.ID)
	if _, err := o.Retry(f.uid, r.ID, 0); err == nil {
		t.Fatal("retry of a started member")
	} else if strings.Contains(err.Error(), sentinel) {
		t.Fatal("prompt in an error")
	}
	f.logMu.Lock()
	logs := f.logs.String()
	f.logMu.Unlock()
	if strings.Contains(logs, sentinel) || strings.Contains(fmt.Sprintf("%+v", got.Members), sentinel) {
		t.Fatal("prompt leaked")
	}
	for _, c := range f.broker.sent() {
		if c.in.Prompt != sentinel {
			t.Fatal("prompt not sent")
		}
	}
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
