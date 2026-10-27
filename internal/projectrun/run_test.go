package projectrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
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

// kind is what the fake broker and machine do with a new request id.
type kind int

const (
	// start: the machine starts an agent and the answer arrives.
	start kind = iota
	// lost: the machine starts an agent and the answer never arrives.
	lost
	// refuse: the machine (or the broker after reserving) refuses; the
	// refusal is kept with the request id and replayed.
	refuse
	// before: the broker refuses before reserving anything; nothing is kept.
	before
	// mismatch: the agent starts but the worktree report does not match.
	mismatch
)

type answer struct {
	kind  kind
	err   error
	state terminalproto.SessionState // for start: default running
}

type reservation struct {
	session  string
	state    terminalproto.SessionState
	err      error
	reported bool
}

// fakeBroker models the real reservation semantics: a request id it has
// reserved is answered from that reservation (the session, or the kept
// refusal) and starts nothing; a new request id with an existing worktree
// token starts another agent in that worktree, as the machine would.
type fakeBroker struct {
	mu           sync.Mutex
	calls        []sent
	inFlight     int
	peak         int
	byAgent      map[string]int
	agentPeak    map[string]int
	agentsAtOnce int // most distinct machines with a create in flight at once
	notReady     map[string]bool
	answer       func(sent) answer
	gate         chan struct{}
	delay        time.Duration
	reserved     map[string]*reservation
	agents       map[string]int // agents started, by worktree token (or machine+folder)
	nextSession  int
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

func agentKey(agent string, in terminalproto.Create) string {
	if in.Worktree != "" {
		return in.Worktree
	}
	return agent + "|" + in.Dir
}

func (f *fakeBroker) CreateSession(ctx context.Context, uid int64, agent, request string, in terminalproto.Create) (store.TerminalSession, bool, error) {
	c := sent{agent, request, in}
	f.mu.Lock()
	if f.byAgent == nil {
		f.byAgent, f.agentPeak, f.reserved, f.agents = map[string]int{}, map[string]int{}, map[string]*reservation{}, map[string]int{}
	}
	f.calls = append(f.calls, c)
	f.inFlight++
	f.peak = max(f.peak, f.inFlight)
	f.byAgent[agent]++
	f.agentPeak[agent] = max(f.agentPeak[agent], f.byAgent[agent])
	busy := 0
	for _, n := range f.byAgent {
		if n > 0 {
			busy++
		}
	}
	f.agentsAtOnce = max(f.agentsAtOnce, busy)
	gate, decide, delay := f.gate, f.answer, f.delay
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.inFlight--; f.byAgent[agent]--; f.mu.Unlock() }()
	cut := false
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			cut = true
		}
	}
	if delay > 0 && !cut {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			cut = true
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if r := f.reserved[request]; r != nil {
		if r.err != nil {
			return store.TerminalSession{}, false, r.err
		}
		first := !r.reported
		r.reported = true
		return f.session(r, agent, in), first && r.state == terminalproto.Running, nil
	}
	a := answer{}
	if cut {
		a = answer{kind: lost} // the machine got it; the answer did not come back
	} else if decide != nil {
		f.mu.Unlock()
		a = decide(c)
		f.mu.Lock()
	}
	switch a.kind {
	case before:
		return store.TerminalSession{}, false, a.err
	case refuse:
		f.reserved[request] = &reservation{err: a.err}
		return store.TerminalSession{}, false, a.err
	}
	if a.state == "" {
		a.state = terminalproto.Running
	}
	f.nextSession++
	r := &reservation{session: fmt.Sprintf("session-%016d", f.nextSession), state: a.state}
	f.reserved[request] = r
	if a.state == terminalproto.Running {
		f.agents[agentKey(agent, in)]++
	}
	switch a.kind {
	case lost:
		return store.TerminalSession{}, false, terminalbroker.ErrWorktreeReplyLost
	case mismatch:
		r.reported = true
		return f.session(r, agent, terminalproto.Create{}), false, terminalbroker.ErrWorktreeMismatch
	}
	r.reported = true
	return f.session(r, agent, in), a.state == terminalproto.Running, nil
}

func (f *fakeBroker) session(r *reservation, agent string, in terminalproto.Create) store.TerminalSession {
	s := store.TerminalSession{ID: r.session, AgentID: agent, State: r.state}
	if in.Worktree != "" {
		repo := terminalproto.Slug(in.Dir[strings.LastIndex(in.Dir, "/")+1:], "repo")
		s.WorktreePath = "/home/t/.reet/worktrees/" + in.Project + "/" + repo + "/" + in.Worktree
		s.WorktreeRepo = repo
		s.WorktreeBranch = "reet/" + in.Project + "-" + in.Worktree
	}
	return s
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

// agentsIn reports how many agents the machine started for each worktree
// token (or machine and folder).
func (f *fakeBroker) agentsIn() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]int{}
	for k, v := range f.agents {
		out[k] = v
	}
	return out
}

func oneAgentEach(t *testing.T, f *fakeBroker) {
	t.Helper()
	for k, n := range f.agentsIn() {
		if n != 1 {
			t.Fatalf("%d agents started in %s", n, k)
		}
	}
}

type fixture struct {
	st      *store.Store
	uid     int64
	laptop  string
	server  string
	desk    string
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
	desk, _, err := st.EnrollSyncAgent(uid, "desk", "test")
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{st: st, uid: uid, laptop: laptop.ID, server: server.ID, desk: desk.ID, broker: &fakeBroker{}, logs: &strings.Builder{}, enabled: true}
}

func (f *fixture) orchestrator(t *testing.T, cfg ...func(*Config)) *Orchestrator {
	t.Helper()
	broker := f.broker
	c := Config{Store: f.st, Broker: broker, Poll: 2 * time.Millisecond, StartWait: 40 * time.Millisecond, ResumeWait: 40 * time.Millisecond,
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

func (f *fixture) launched() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.started...)
}

// project saves a Project whose members are the given (machine, folder) pairs.
func (f *fixture) project(t *testing.T, worktrees bool, members ...[2]string) store.Project {
	t.Helper()
	return f.named(t, "Shop Front", worktrees, members...)
}

func (f *fixture) named(t *testing.T, name string, worktrees bool, members ...[2]string) store.Project {
	t.Helper()
	p := store.Project{Name: name, DefaultTool: "claude", DefaultModel: "sonnet", SharedPrompt: sentinel, UseWorktrees: worktrees}
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
	deadline := time.Now().Add(20 * time.Second)
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
	deadline := time.Now().Add(20 * time.Second)
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

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestDerivedRequestIDsAndTokens(t *testing.T) {
	run := "0123456789abcdef0123456789abcdef"
	if got := RequestID(run, 3, 2); len(got) != 48 || got != RequestID(run, 3, 2) || !terminalproto.ValidID(got) {
		t.Fatal(got)
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
	if got, want := RequestID("run", 0, 1), sha("run|0|1")[:48]; got != want {
		t.Fatal(got, want)
	}
	if got, want := WorktreeToken("run", 0), sha("worktree|run|0")[:24]; got != want {
		t.Fatal(got, want)
	}
	if RetryAttempt(store.RunMember{Attempt: 3, Sent: true}) != 3 || RetryAttempt(store.RunMember{Attempt: 3}) != 4 {
		t.Fatal("retry attempt rule")
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
		pos := -1
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
		if m.State != store.RunMemberStarted || m.SessionID == "" || m.WorktreeToken != c.in.Worktree || !m.Sent ||
			m.WorktreePath == "" || m.WorktreeRepo == "" || m.WorktreeBranch != "reet/shop-front-"+c.in.Worktree || m.Attempt != 1 {
			t.Fatalf("member %+v", m)
		}
	}
	if len(tokens) != 3 || len(f.launched()) != 3 {
		t.Fatal("tokens", tokens, "launched", f.launched())
	}
	oneAgentEach(t, f.broker)
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

func TestRunHasAtMostTwoInFlightAndOnePerMachine(t *testing.T) {
	f := newFixture(t)
	f.broker.gate = make(chan struct{})
	o := f.orchestrator(t)
	p := f.project(t, true, [2]string{f.laptop, "/a/1"}, [2]string{f.laptop, "/a/2"}, [2]string{f.laptop, "/a/3"}, [2]string{f.server, "/a/4"}, [2]string{f.server, "/a/5"}, [2]string{f.desk, "/a/6"})
	r := f.run(t, p)
	o.Start(f.uid, r.ID)
	for released := 0; released < 6; released++ {
		if released == 0 {
			// Three machines are ready, so the run fills both slots before anything is released: wait for that rather
			// than hoping the second create lands inside a short sleep, which a loaded runner can miss.
			waitFor(t, "two creates in flight", func() bool { return f.broker.inflight() == 2 })
		} else {
			waitFor(t, "a create in flight", func() bool { return f.broker.inflight() > 0 })
		}
		time.Sleep(15 * time.Millisecond)
		if n := f.broker.inflight(); n > 2 {
			t.Fatalf("%d in flight", n)
		}
		f.broker.gate <- struct{}{}
	}
	got := settle(t, o, f.st, f.uid, r.ID)
	if got.State != store.RunComplete || f.broker.peak != 2 || f.broker.agentPeak[f.laptop] != 1 || f.broker.agentPeak[f.server] != 1 || f.broker.agentPeak[f.desk] != 1 {
		t.Fatalf("peak %d %v, %+v", f.broker.peak, f.broker.agentPeak, got)
	}
	// Members on one machine go in position order.
	var laptop []string
	for _, c := range f.broker.sent() {
		if c.agent == f.laptop {
			laptop = append(laptop, c.in.Dir)
		}
	}
	if strings.Join(laptop, ",") != "/a/1,/a/2,/a/3" {
		t.Fatal(laptop)
	}
}

// Eight members on one slow machine, two on another, and a second run on the
// first machine: all start, none fails, the slow machine never has two creates
// at once, and the other machine works in parallel.
func TestOneMachineUnderLoadSerialisesAcrossRunsWhileOthersRunInParallel(t *testing.T) {
	f := newFixture(t)
	f.broker.delay = 15 * time.Millisecond
	o := f.orchestrator(t)
	var members [][2]string
	for i := 0; i < 8; i++ {
		members = append(members, [2]string{f.laptop, fmt.Sprintf("/load/%d", i)})
	}
	members = append(members, [2]string{f.server, "/load/s1"}, [2]string{f.server, "/load/s2"})
	r1 := f.run(t, f.project(t, true, members...))
	r2 := f.run(t, f.named(t, "Other", true, [2]string{f.laptop, "/other/1"}, [2]string{f.laptop, "/other/2"}))
	o.Start(f.uid, r1.ID)
	o.Start(f.uid, r2.ID)
	for _, id := range []string{r1.ID, r2.ID} {
		got := settle(t, o, f.st, f.uid, id)
		if got.State != store.RunComplete {
			t.Fatalf("%+v", got.Members)
		}
	}
	if f.broker.agentPeak[f.laptop] != 1 || f.broker.agentPeak[f.server] != 1 || f.broker.agentsAtOnce != 2 {
		t.Fatalf("per machine %v, machines at once %d", f.broker.agentPeak, f.broker.agentsAtOnce)
	}
	oneAgentEach(t, f.broker)
}

func TestAMachineThatIsBrieflyNotReadyDoesNotFailItsMembers(t *testing.T) {
	f := newFixture(t)
	f.broker.setReady(f.laptop, false)
	o := f.orchestrator(t, func(c *Config) { c.StartWait = 5 * time.Second })
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/0"}, [2]string{f.laptop, "/a/1"}, [2]string{f.laptop, "/a/2"}))
	o.Start(f.uid, r.ID)
	time.Sleep(30 * time.Millisecond)
	f.broker.setReady(f.laptop, true)
	if got := settle(t, o, f.st, f.uid, r.ID); got.State != store.RunComplete {
		t.Fatalf("%+v", got.Members)
	}
}

func TestRunStartsOnlyTheChosenMembers(t *testing.T) {
	f := newFixture(t)
	o := f.orchestrator(t)
	p := f.project(t, true, [2]string{f.laptop, "/a/0"}, [2]string{f.laptop, "/a/1"}, [2]string{f.server, "/a/2"})
	r := f.run(t, p, 2, 0)
	o.Start(f.uid, r.ID)
	got := settle(t, o, f.st, f.uid, r.ID)
	byRequest := map[string]sent{}
	for _, c := range f.broker.sent() {
		byRequest[c.request] = c
	}
	if len(got.Members) != 2 || len(byRequest) != 2 || byRequest[RequestID(r.ID, 0, 1)].in.Dir != "/a/0" || byRequest[RequestID(r.ID, 2, 1)].in.Worktree != WorktreeToken(r.ID, 2) {
		t.Fatalf("%+v %+v", got.Members, byRequest)
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
		host, dir      string
		a              answer
		code           string
		nothingStarted bool
	}{
		{f.laptop, "/c/ok-first", answer{}, "", false},
		{f.laptop, "/c/launch", answer{kind: before, err: terminalbroker.ErrLaunchUnsupported}, CodeLaunchUnsupported, true},
		{f.laptop, "/c/worktree", answer{kind: before, err: terminalbroker.ErrWorktreeUnsupported}, CodeWorktreeUnsupported, true},
		{f.laptop, "/c/exists", answer{kind: refuse, err: &terminalbroker.WorktreeError{Reason: terminalproto.WorktreeExists}}, "worktree_exists", true},
		{f.laptop, "/c/root", answer{kind: refuse, err: &terminalbroker.WorktreeError{Reason: terminalproto.WorktreeNotRepoRoot}}, "not_repo_root", true},
		{f.laptop, "/c/busy", answer{kind: refuse, err: &terminalbroker.WorktreeError{Reason: terminalproto.WorktreeRepoBusy}}, "repo_busy", true},
		{f.laptop, "/c/git", answer{kind: refuse, err: &terminalbroker.WorktreeError{Reason: terminalproto.WorktreeGitFailed, Detail: "fatal"}}, "git_failed", true},
		{f.laptop, "/c/home", answer{kind: refuse, err: &terminalbroker.WorktreeError{Reason: terminalproto.WorktreeOutsideHome}}, "outside_home", true},
		{f.laptop, "/c/folder", answer{kind: refuse, err: terminalbroker.ErrLaunchFailed}, CodeFolderUnavailable, true},
		{f.server, "/c/tool", answer{kind: refuse, err: terminalbroker.ErrLaunchFailed}, CodeToolNotInstalled, true},
		{f.server, "/c/limit", answer{kind: before, err: store.ErrTerminalCapacity}, CodeSessionLimit, true},
		{f.server, "/c/lost", answer{state: terminalproto.Lost}, CodeSessionLost, true},
	}
	byDir := map[string]answer{}
	var members [][2]string
	for _, c := range cases {
		byDir[c.dir] = c.a
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
		if m.State != store.RunMemberFailed || m.Code != c.code || m.SessionID != "" || m.Sent == c.nothingStarted {
			t.Fatalf("%s: want %s (nothing started %v), got %+v", c.dir, c.code, c.nothingStarted, m)
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
			return answer{kind: before, err: store.ErrTerminalCapacity}
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
	if m := member(got, 0); m.Code != CodeUnknownMachine || m.Sent {
		t.Fatal(m)
	}
	if m := member(got, 1); m.Code != CodeMachineUnavailable || m.Sent {
		t.Fatal(m)
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
	r2 := f.run(t, f.named(t, "Second", true, [2]string{f.laptop, "/b/1"}))
	o.Start(f.uid, r2.ID)
	got = settle(t, o, f.st, f.uid, r2.ID)
	if c := member(got, 0).Code; c != CodeTerminalsDisabled || len(f.broker.sent()) != 1 {
		t.Fatal(c, len(f.broker.sent()))
	}
}

func TestRetryAfterAnUnknownOutcomeReusesTheRequestID(t *testing.T) {
	for _, first := range []answer{{kind: lost}, {kind: before, err: terminalbroker.ErrOffline}, {kind: before, err: context.DeadlineExceeded},
		{kind: before, err: terminalbroker.ErrConflict}, {kind: before, err: errors.New("anything else")}, {kind: before, err: store.ErrRemoteNotFound}} {
		f := newFixture(t)
		var calls atomic.Int32
		f.broker.answer = func(s sent) answer {
			if calls.Add(1) == 1 {
				return first
			}
			return answer{}
		}
		o := f.orchestrator(t)
		r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/1"}))
		o.Start(f.uid, r.ID)
		got := settle(t, o, f.st, f.uid, r.ID)
		if m := got.Members[0]; m.State != store.RunMemberFailed || !m.Sent || m.Attempt != 1 {
			t.Fatalf("%v: %+v", first, m)
		}
		m, err := o.Retry(f.uid, r.ID, 0)
		if err != nil || m.Attempt != 1 {
			t.Fatal(err, m)
		}
		got = settle(t, o, f.st, f.uid, r.ID)
		sentNow := f.broker.sent()
		if len(sentNow) != 2 || sentNow[0].request != RequestID(r.ID, 0, 1) || sentNow[1].request != sentNow[0].request || sentNow[1].in != sentNow[0].in {
			t.Fatalf("%v: %+v", first, sentNow)
		}
		if got.Members[0].State != store.RunMemberStarted || got.Members[0].Attempt != 1 || len(f.launched()) != 1 {
			t.Fatalf("%+v %v", got.Members[0], f.launched())
		}
		oneAgentEach(t, f.broker)
	}
}

func TestRetryAfterADefiniteRefusalUsesTheNextAttempt(t *testing.T) {
	f := newFixture(t)
	caps := terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "t", TerminalControl: true, TerminalEnabled: true, Readiness: terminalproto.Ready, Launch: true, Worktree: true, Tools: []string{"codex"}}
	if _, err := f.st.BeginRemoteConnectionFor(f.uid, f.laptop, terminalbroker.ID(), caps); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	f.broker.answer = func(s sent) answer {
		if calls.Add(1) == 1 {
			return answer{kind: refuse, err: terminalbroker.ErrLaunchFailed}
		}
		return answer{}
	}
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/1"}))
	o.Start(f.uid, r.ID)
	if m := settle(t, o, f.st, f.uid, r.ID).Members[0]; m.Code != CodeToolNotInstalled || m.Sent {
		t.Fatal(m)
	}
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
	oneAgentEach(t, f.broker)
}

func TestARefusalBeforeAnythingWasSentAllowsTheNextAttempt(t *testing.T) {
	f := newFixture(t)
	f.broker.setReady(f.laptop, false)
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/1"}))
	o.Start(f.uid, r.ID)
	if m := settle(t, o, f.st, f.uid, r.ID).Members[0]; m.Code != CodeMachineUnavailable || m.Sent || len(f.broker.sent()) != 0 {
		t.Fatal(m)
	}
	f.broker.setReady(f.laptop, true)
	if m, err := o.Retry(f.uid, r.ID, 0); err != nil || m.Attempt != 2 {
		t.Fatal(err, m)
	}
	settle(t, o, f.st, f.uid, r.ID)
	if s := f.broker.sent(); len(s) != 1 || s[0].request != RequestID(r.ID, 0, 2) {
		t.Fatalf("%+v", s)
	}
}

// The reviewer's scenario: the answer to attempt 1 is lost (the agent runs),
// a retry meets the machine reconnecting and is refused before sending, and a
// third try must still send attempt 1's request id, so the machine answers
// with the agent it already has instead of starting a second one.
func TestAPossiblySentCreateStaysStickyThroughAPrecheckRefusal(t *testing.T) {
	f := newFixture(t)
	var calls atomic.Int32
	f.broker.answer = func(s sent) answer {
		if calls.Add(1) == 1 {
			return answer{kind: lost}
		}
		return answer{}
	}
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/1"}))
	o.Start(f.uid, r.ID)
	if m := settle(t, o, f.st, f.uid, r.ID).Members[0]; m.Code != CodeUnknownOutcome || !m.Sent {
		t.Fatal(m)
	}
	f.broker.setReady(f.laptop, false)
	if m, err := o.Retry(f.uid, r.ID, 0); err != nil || m.Attempt != 1 {
		t.Fatal(err, m)
	}
	if m := settle(t, o, f.st, f.uid, r.ID).Members[0]; m.Code != CodeMachineUnavailable || !m.Sent || m.Attempt != 1 {
		t.Fatal("the precheck refusal cleared the mark", m)
	}
	f.broker.setReady(f.laptop, true)
	if m, err := o.Retry(f.uid, r.ID, 0); err != nil || m.Attempt != 1 {
		t.Fatal(err, m)
	}
	got := settle(t, o, f.st, f.uid, r.ID)
	s := f.broker.sent()
	if len(s) != 2 || s[0].request != RequestID(r.ID, 0, 1) || s[1].request != s[0].request {
		t.Fatalf("%+v", s)
	}
	if got.Members[0].State != store.RunMemberStarted || len(f.launched()) != 1 {
		t.Fatalf("%+v %v", got.Members[0], f.launched())
	}
	oneAgentEach(t, f.broker)
}

// The same after a crash: the in-flight member's machine stays away past the
// resume wait, and the retry after that still uses the first request id.
func TestAnInFlightMemberAtACrashStaysStickyThroughAnOfflineResume(t *testing.T) {
	f := newFixture(t)
	f.broker.gate = make(chan struct{})
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/1"}))
	o.Start(f.uid, r.ID)
	waitFor(t, "the create", func() bool { return f.broker.inflight() == 1 })
	o.Close() // the agent starts; the answer is lost with the controller
	broker := f.broker
	broker.mu.Lock()
	broker.gate = nil
	broker.mu.Unlock()
	broker.setReady(f.laptop, false)
	o2 := f.orchestrator(t)
	if err := o2.Resume(); err != nil {
		t.Fatal(err)
	}
	if m := settle(t, o2, f.st, f.uid, r.ID).Members[0]; m.Code != CodeMachineUnavailable || !m.Sent || m.Attempt != 1 {
		t.Fatal(m)
	}
	broker.setReady(f.laptop, true)
	if m, err := o2.Retry(f.uid, r.ID, 0); err != nil || m.Attempt != 1 {
		t.Fatal(err, m)
	}
	got := settle(t, o2, f.st, f.uid, r.ID)
	if s := broker.sent(); len(s) != 2 || s[1].request != s[0].request || s[0].request != RequestID(r.ID, 0, 1) {
		t.Fatalf("%+v", s)
	}
	if got.Members[0].State != store.RunMemberStarted || len(f.launched()) != 1 {
		t.Fatalf("%+v %v", got.Members[0], f.launched())
	}
	oneAgentEach(t, broker)
}

func TestRetryAfterAWorktreeMismatchReusesTheRequestIDAndAuditsOnce(t *testing.T) {
	f := newFixture(t)
	var calls atomic.Int32
	f.broker.answer = func(s sent) answer {
		if calls.Add(1) == 1 {
			return answer{kind: mismatch}
		}
		return answer{}
	}
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/1"}))
	o.Start(f.uid, r.ID)
	if m := settle(t, o, f.st, f.uid, r.ID).Members[0]; m.Code != CodeWorktreeMismatch || !m.Sent {
		t.Fatal(m)
	}
	if m, err := o.Retry(f.uid, r.ID, 0); err != nil || m.Attempt != 1 {
		t.Fatal(err, m)
	}
	got := settle(t, o, f.st, f.uid, r.ID)
	if s := f.broker.sent(); len(s) != 2 || s[1].request != s[0].request {
		t.Fatalf("%+v", s)
	}
	// The replay reports the session as already known; the launch is still
	// audited once, when the member becomes started.
	if got.Members[0].State != store.RunMemberStarted || len(f.launched()) != 1 {
		t.Fatalf("%+v %v", got.Members[0], f.launched())
	}
	oneAgentEach(t, f.broker)
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
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/0"}, [2]string{f.server, "/a/1"}, [2]string{f.laptop, "/a/2"}, [2]string{f.server, "/a/3"}))
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
	sentBefore := map[string]bool{}
	for _, c := range first {
		sentBefore[c.request] = true
	}
	// A new controller on the same database and the same machines.
	broker := f.broker
	broker.mu.Lock()
	broker.gate = nil
	broker.mu.Unlock()
	o2 := f.orchestrator(t)
	if err := o2.Resume(); err != nil {
		t.Fatal(err)
	}
	got = settle(t, o2, f.st, f.uid, r.ID)
	if got.State != store.RunComplete {
		t.Fatalf("%+v", got)
	}
	again := broker.sent()[len(first):]
	if len(again) != 3 {
		t.Fatalf("resume sent %d creates", len(again))
	}
	for _, c := range again {
		var pos int
		fmt.Sscanf(c.in.Dir, "/a/%d", &pos)
		if c.request != RequestID(r.ID, pos, 1) || c.in.Worktree != WorktreeToken(r.ID, pos) {
			t.Fatalf("resumed with a new identity: %+v", c)
		}
		if member(got, pos).SessionID == startedBefore {
			t.Fatal("a started member was started again")
		}
	}
	oneAgentEach(t, broker)
	if len(f.launched()) != 4 {
		t.Fatal("each started member is audited once", f.launched())
	}
	if err := o2.Resume(); err != nil {
		t.Fatal(err)
	}
	settle(t, o2, f.st, f.uid, r.ID)
	if len(broker.sent()) != len(first)+3 {
		t.Fatal("a second resume sent again")
	}
}

func TestResumeWaitsForAMachineWithoutBlockingTheOthers(t *testing.T) {
	f := newFixture(t)
	r := f.run(t, f.project(t, true, [2]string{f.server, "/a/0"}, [2]string{f.desk, "/a/1"}, [2]string{f.laptop, "/a/2"}, [2]string{f.laptop, "/a/3"}, [2]string{f.laptop, "/a/4"}))
	f.broker.setReady(f.server, false)
	f.broker.setReady(f.desk, false)
	f.broker.setReady(f.laptop, false)
	o := f.orchestrator(t, func(c *Config) { c.ResumeWait = 3 * time.Second })
	if err := o.Resume(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	f.broker.setReady(f.laptop, true) // the laptop reconnects; the server and the desk do not
	began := time.Now()
	waitFor(t, "the laptop members", func() bool {
		run, _ := f.st.RunFor(f.uid, r.ID)
		return member(run, 2).State == store.RunMemberStarted && member(run, 3).State == store.RunMemberStarted && member(run, 4).State == store.RunMemberStarted
	})
	if time.Since(began) > 2*time.Second {
		t.Fatal("reachable members waited behind an offline machine")
	}
	got := settle(t, o, f.st, f.uid, r.ID)
	if member(got, 0).Code != CodeMachineUnavailable || member(got, 1).Code != CodeMachineUnavailable {
		t.Fatalf("%+v", got.Members)
	}
}

func TestARetryPassDoesNotInheritTheResumeWait(t *testing.T) {
	f := newFixture(t)
	f.broker.setReady(f.server, false) // keeps the resume pass busy
	f.broker.answer = func(s sent) answer { return answer{kind: before, err: store.ErrTerminalCapacity} }
	r := f.run(t, f.project(t, true, [2]string{f.server, "/a/0"}, [2]string{f.laptop, "/a/1"}))
	o := f.orchestrator(t, func(c *Config) { c.ResumeWait = 400 * time.Millisecond; c.StartWait = time.Millisecond })
	if err := o.Resume(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the laptop member to fail", func() bool {
		run, _ := f.st.RunFor(f.uid, r.ID)
		return member(run, 1).State == store.RunMemberFailed
	})
	f.broker.setReady(f.laptop, false)
	began := time.Now()
	if _, err := o.Retry(f.uid, r.ID, 1); err != nil {
		t.Fatal(err)
	}
	got := settle(t, o, f.st, f.uid, r.ID)
	// The retry pass follows the resume pass (~400ms) and waits StartWait, not
	// another ResumeWait.
	if m := member(got, 1); m.Code != CodeMachineUnavailable || time.Since(began) > 650*time.Millisecond {
		t.Fatal(m, time.Since(began))
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
		t.Fatalf("%+v", got)
	}
	oneAgentEach(t, f.broker)
	if len(f.launched()) != 3 {
		t.Fatal(f.launched())
	}
}

func TestDuplicateRepositoriesAreRefusedButOneMachineRunsTwoMembersApart(t *testing.T) {
	f := newFixture(t)
	if _, err := f.st.CreateProjectFor(f.uid, store.Project{Name: "Dup", DefaultTool: "claude", Members: []store.ProjectMember{{Host: f.laptop, Dir: "/a/x"}, {Host: f.laptop, Dir: "/a/x/"}}}); !errors.Is(err, store.ErrDuplicateMember) {
		t.Fatal(err)
	}
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/x"}, [2]string{f.laptop, "/a/y"}))
	o.Start(f.uid, r.ID)
	settle(t, o, f.st, f.uid, r.ID)
	s := f.broker.sent()
	if len(s) != 2 || s[0].in.Worktree == s[1].in.Worktree || s[0].request == s[1].request {
		t.Fatal("two members shared a token or a request id")
	}
	oneAgentEach(t, f.broker)
}

func TestThePromptReachesOnlyTheCreate(t *testing.T) {
	f := newFixture(t)
	f.broker.answer = func(s sent) answer {
		if strings.HasSuffix(s.in.Dir, "1") {
			return answer{kind: refuse, err: &terminalbroker.WorktreeError{Reason: terminalproto.WorktreeGitFailed, Detail: "fatal: bad"}}
		}
		return answer{}
	}
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/0"}, [2]string{f.laptop, "/a/1"}))
	o.Start(f.uid, r.ID)
	got := settle(t, o, f.st, f.uid, r.ID)
	if _, err := o.Retry(f.uid, r.ID, 0); err == nil || strings.Contains(err.Error(), sentinel) {
		t.Fatal("retry of a started member", err)
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

// Concurrency: run with -race -count=20.

func TestConcurrentRetriesOfOneMemberLetExactlyOneProceed(t *testing.T) {
	f := newFixture(t)
	var calls atomic.Int32
	f.broker.answer = func(s sent) answer {
		if calls.Add(1) == 1 {
			return answer{kind: lost}
		}
		return answer{}
	}
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/0"}))
	o.Start(f.uid, r.ID)
	settle(t, o, f.st, f.uid, r.ID)
	var wg sync.WaitGroup
	var ok, refused atomic.Int32
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := o.Retry(f.uid, r.ID, 0)
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrNotFailed):
				refused.Add(1)
			default:
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got := settle(t, o, f.st, f.uid, r.ID)
	if ok.Load() != 1 || refused.Load() != 5 || got.Members[0].State != store.RunMemberStarted || got.Members[0].Attempt != 1 {
		t.Fatal(ok.Load(), refused.Load(), got.Members[0])
	}
	oneAgentEach(t, f.broker)
}

func TestRetryRacingResumeStartsOneAgent(t *testing.T) {
	f := newFixture(t)
	var calls atomic.Int32
	f.broker.answer = func(s sent) answer {
		if calls.Add(1) == 1 {
			return answer{kind: lost}
		}
		return answer{}
	}
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/0"}, [2]string{f.laptop, "/a/1"}))
	o.Start(f.uid, r.ID)
	settle(t, o, f.st, f.uid, r.ID)
	o2 := f.orchestrator(t)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = o2.Retry(f.uid, r.ID, 0) }()
	go func() { defer wg.Done(); _ = o2.Resume() }()
	wg.Wait()
	got := settle(t, o2, f.st, f.uid, r.ID)
	if got.State != store.RunComplete {
		t.Fatalf("%+v", got.Members)
	}
	oneAgentEach(t, f.broker)
	if len(f.launched()) != 2 {
		t.Fatal(f.launched())
	}
}

func TestRetryWhileTheFirstAttemptIsInFlightIsRefused(t *testing.T) {
	f := newFixture(t)
	f.broker.gate = make(chan struct{})
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/0"}))
	o.Start(f.uid, r.ID)
	waitFor(t, "the create", func() bool { return f.broker.inflight() == 1 })
	if _, err := o.Retry(f.uid, r.ID, 0); !errors.Is(err, ErrNotFailed) {
		t.Fatal(err)
	}
	close(f.broker.gate)
	if got := settle(t, o, f.st, f.uid, r.ID); got.Members[0].State != store.RunMemberStarted || len(f.broker.sent()) != 1 {
		t.Fatal(got.Members[0], len(f.broker.sent()))
	}
}

// flakyStore fails chosen writes once, and can report a create's operation.
type flakyStore struct {
	*store.Store
	mu                               sync.Mutex
	failToken, failMark, failStarted int
	op                               *store.TerminalOperation
}

var errDisk = errors.New("disk I/O error")

func (s *flakyStore) take(n *int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if *n > 0 {
		*n--
		return true
	}
	return false
}

func (s *flakyStore) SetRunMemberWorktreeToken(uid int64, runID string, position int, token string) error {
	if s.take(&s.failToken) {
		return errDisk
	}
	return s.Store.SetRunMemberWorktreeToken(uid, runID, position, token)
}

func (s *flakyStore) MarkRunMemberSent(uid int64, runID string, position, attempt int) error {
	if s.take(&s.failMark) {
		return errDisk
	}
	return s.Store.MarkRunMemberSent(uid, runID, position, attempt)
}

func (s *flakyStore) SetRunMemberState(uid int64, runID string, position int, state, code, sessionID, path, repo, branch string, attempt int) error {
	if state == store.RunMemberStarted && s.take(&s.failStarted) {
		return errDisk
	}
	return s.Store.SetRunMemberState(uid, runID, position, state, code, sessionID, path, repo, branch, attempt)
}

func (s *flakyStore) TerminalOperationFor(uid int64, agent, kind, request string) (store.TerminalOperation, error) {
	s.mu.Lock()
	op := s.op
	s.mu.Unlock()
	if op != nil {
		return *op, nil
	}
	return s.Store.TerminalOperationFor(uid, agent, kind, request)
}

func TestAMachineConflictKeepsTheRequestIDButOtherRefusalsDoNot(t *testing.T) {
	for _, c := range []struct {
		code terminalproto.ResultCode
		sent bool
	}{{terminalproto.Conflict, true}, {terminalproto.Denied, false}, {terminalproto.Failed, false}} {
		f := newFixture(t)
		fs := &flakyStore{Store: f.st, op: &store.TerminalOperation{State: "failed", Code: c.code}}
		var calls atomic.Int32
		f.broker.answer = func(s sent) answer {
			if calls.Add(1) == 1 {
				return answer{state: terminalproto.Lost}
			}
			return answer{}
		}
		o := f.orchestrator(t, func(cfg *Config) { cfg.Store = fs })
		r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/0"}))
		o.Start(f.uid, r.ID)
		m := settle(t, o, f.st, f.uid, r.ID).Members[0]
		if m.Code != CodeMachineRefused || m.Sent != c.sent {
			t.Fatal(c.code, m)
		}
		want := 2
		if c.sent {
			want = 1
		}
		if m, err := o.Retry(f.uid, r.ID, 0); err != nil || m.Attempt != want {
			t.Fatal(c.code, m, err)
		}
	}
}

func TestAStoreErrorFailsTheMemberInsteadOfStrandingIt(t *testing.T) {
	for _, c := range []struct {
		name          string
		set           func(*flakyStore)
		sent          bool
		retryAttempt  int
		createsBefore int
	}{
		{"token", func(s *flakyStore) { s.failToken = 1 }, false, 2, 0},
		{"mark", func(s *flakyStore) { s.failMark = 1 }, false, 2, 0},
		{"started", func(s *flakyStore) { s.failStarted = 1 }, true, 1, 1},
	} {
		f := newFixture(t)
		fs := &flakyStore{Store: f.st}
		c.set(fs)
		o := f.orchestrator(t, func(cfg *Config) { cfg.Store = fs })
		r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/0"}))
		o.Start(f.uid, r.ID)
		m := settle(t, o, f.st, f.uid, r.ID).Members[0]
		if m.State != store.RunMemberFailed || m.Code != CodeInternalError || m.Sent != c.sent || len(f.broker.sent()) != c.createsBefore {
			t.Fatal(c.name, m, len(f.broker.sent()))
		}
		if m, err := o.Retry(f.uid, r.ID, 0); err != nil || m.Attempt != c.retryAttempt {
			t.Fatal(c.name, m, err)
		}
		got := settle(t, o, f.st, f.uid, r.ID)
		if got.Members[0].State != store.RunMemberStarted || len(f.launched()) != 1 {
			t.Fatal(c.name, got.Members[0], f.launched())
		}
		oneAgentEach(t, f.broker)
	}
}

func TestIdleMachinesLeaveNoSlotBehind(t *testing.T) {
	f := newFixture(t)
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/0"}, [2]string{f.server, "/a/1"}, [2]string{f.desk, "/a/2"}))
	o.Start(f.uid, r.ID)
	settle(t, o, f.st, f.uid, r.ID)
	o.mu.Lock()
	n := len(o.hosts)
	o.mu.Unlock()
	if n != 0 {
		t.Fatal(n, "machine slots kept")
	}
}
