package terminalbroker

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	testRun     = "0123456789abcdef0123"
	testProject = "my-project"
)

func worktreeCapabilities(tools ...string) terminalproto.Capabilities {
	caps := launchCapabilities(tools...)
	caps.Worktree = true
	return caps
}

func worktreeCreate() terminalproto.Create {
	return terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24, Tool: "claude", Dir: "/home/test/site", Prompt: "fix the tests", Worktree: testRun, Project: testProject}
}

func (f *launchPeer) worktreeSent() []terminalproto.Body {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]terminalproto.Body(nil), f.worktrees...)
}

func TestWorktreeIsNeverSentToAPeerWithoutTheWorktreeCapability(t *testing.T) {
	for name, caps := range map[string]terminalproto.Capabilities{
		"launch only": launchCapabilities("claude"),
		"legacy":      legacyCapabilities(),
	} {
		f := launchFixture(t, caps)
		if _, err := f.b.Create(t.Context(), f.uid, f.agent, ID(), worktreeCreate()); !errors.Is(err, ErrWorktreeUnsupported) {
			t.Fatal(name, "worktree to an old peer", err)
		}
		if sent := f.sent(); len(sent) != 0 {
			t.Fatal(name, "worktree fields reached an old peer", sent)
		}
		if sessions, _ := f.st.TerminalSessionsFor(f.uid); len(sessions) != 0 {
			t.Fatal(name, "refused worktree reserved a session", sessions)
		}
		if _, err := f.b.WorktreeStatus(t.Context(), f.uid, f.agent, testProject, "site", testRun); !errors.Is(err, ErrWorktreeUnsupported) {
			t.Fatal(name, "status on an old peer", err)
		}
		if _, err := f.b.WorktreeRemove(t.Context(), f.uid, f.agent, testProject, "site", testRun, WorktreeCounts{}); !errors.Is(err, ErrWorktreeUnsupported) {
			t.Fatal(name, "remove on an old peer", err)
		}
		if sent := f.worktreeSent(); len(sent) != 0 {
			t.Fatal(name, "worktree request reached an old peer", sent)
		}
	}
	// A launch without a worktree still works on a launch-only peer.
	f := launchFixture(t, launchCapabilities("claude"))
	in := worktreeCreate()
	in.Worktree, in.Project = "", ""
	if s, err := f.b.Create(t.Context(), f.uid, f.agent, ID(), in); err != nil || s.State != terminalproto.Running {
		t.Fatal("launch on a launch-only peer", s, err)
	}
	if sent := f.sent(); len(sent) != 1 || sent[0] != in {
		t.Fatal("launch changed", sent)
	}
}

func TestWorktreeFieldsAreValidatedBeforeAnyPeerState(t *testing.T) {
	f := launchFixture(t, worktreeCapabilities("claude"))
	for name, mutate := range map[string]func(*terminalproto.Create){
		"no tool":           func(c *terminalproto.Create) { c.Tool, c.Prompt = "", "" },
		"no folder":         func(c *terminalproto.Create) { c.Dir = "" },
		"no project":        func(c *terminalproto.Create) { c.Project = "" },
		"project alone":     func(c *terminalproto.Create) { c.Worktree = "" },
		"upper-case token":  func(c *terminalproto.Create) { c.Worktree = "0123456789ABCDEF0123" },
		"short token":       func(c *terminalproto.Create) { c.Worktree = "0123" },
		"dotted project":    func(c *terminalproto.Create) { c.Project = "../etc" },
		"project and shell": func(c *terminalproto.Create) { c.Tool, c.Prompt, c.Dir, c.Worktree = "", "", "", "" },
	} {
		in := worktreeCreate()
		mutate(&in)
		if _, err := f.b.Create(t.Context(), f.uid, f.agent, ID(), in); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatal(name, err)
		}
	}
	if len(f.sent()) != 0 {
		t.Fatal("an invalid worktree create reached the connector", f.sent())
	}
	if sessions, _ := f.st.TerminalSessionsFor(f.uid); len(sessions) != 0 {
		t.Fatal("an invalid worktree create reserved a session", sessions)
	}
}

func TestWorktreeCreateIsSentToAWorktreePeerAndReplaysIdempotently(t *testing.T) {
	f := launchFixture(t, worktreeCapabilities("claude"))
	in := worktreeCreate()
	request := ID()
	first, started, err := f.b.CreateSession(t.Context(), f.uid, f.agent, request, in)
	if err != nil || !started || first.State != terminalproto.Running {
		t.Fatal(first, started, err)
	}
	again, started, err := f.b.CreateSession(t.Context(), f.uid, f.agent, request, in)
	if err != nil || started || again.ID != first.ID {
		t.Fatal("replay", again, started, err)
	}
	if sent := f.sent(); len(sent) != 1 || sent[0] != in {
		t.Fatal("worktree create not forwarded exactly once", sent)
	}
	changed := in
	changed.Project = "other-project"
	if _, _, err := f.b.CreateSession(t.Context(), f.uid, f.agent, request, changed); !errors.Is(err, store.ErrTerminalConflict) {
		t.Fatal("changed project was not a conflict", err)
	}
	if len(f.sent()) != 1 {
		t.Fatal("changed request dispatched")
	}
}

func TestWorktreeRefusalKeepsItsReasonAndIsNotResent(t *testing.T) {
	f := launchFixture(t, worktreeCapabilities("claude"))
	f.reply = func(terminalproto.Create) terminalproto.Body {
		return terminalproto.Result{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeRepoBusy, Detail: "a rebase is in progress"}
	}
	in := worktreeCreate()
	request := ID()
	s, started, err := f.b.CreateSession(t.Context(), f.uid, f.agent, request, in)
	var refusal *WorktreeError
	if !errors.Is(err, ErrWorktreeRefused) || !errors.As(err, &refusal) || refusal.Reason != terminalproto.WorktreeRepoBusy || refusal.Detail != "a rebase is in progress" || started {
		t.Fatal(s, started, err)
	}
	if errors.Is(err, ErrLaunchFailed) {
		t.Fatal("a worktree refusal read as a launch failure")
	}
	if stored, _ := f.st.TerminalSessionFor(f.uid, s.ID); stored.State != terminalproto.Lost {
		t.Fatal("refused worktree left a live record", stored)
	}
	// A replay reports the same refusal, reason and detail included.
	_, started, err = f.b.CreateSession(t.Context(), f.uid, f.agent, request, in)
	refusal = nil
	if !errors.Is(err, ErrWorktreeRefused) || !errors.As(err, &refusal) || refusal.Reason != terminalproto.WorktreeRepoBusy || refusal.Detail != "a rebase is in progress" || started {
		t.Fatal("replay lost the worktree refusal", err)
	}
	if len(f.sent()) != 1 {
		t.Fatal("replay dispatched again", f.sent())
	}
	// launch_failed for a worktree create keeps the launch mapping (folder or tool).
	f.reply = func(terminalproto.Create) terminalproto.Body {
		return terminalproto.Result{Code: terminalproto.LaunchFailed}
	}
	if _, err := f.b.Create(t.Context(), f.uid, f.agent, ID(), in); !errors.Is(err, ErrLaunchFailed) || errors.Is(err, ErrWorktreeRefused) {
		t.Fatal("launch_failed on a worktree create", err)
	}
}

func TestWorktreeStatusAndRemoveAreSentExactlyAndMapped(t *testing.T) {
	f := launchFixture(t, worktreeCapabilities("claude"))
	state := terminalproto.WorktreeState{Path: "/home/test/.reet/worktrees/my-project/site/" + testRun, Branch: "reet/my-project-" + testRun, Changed: 2, Unpushed: 1, Paths: []string{"a.go", "b.go"}, Hidden: 1, Nested: true}
	f.worktree = func(body terminalproto.Body) terminalproto.Body {
		if _, ok := body.(terminalproto.WorktreeStatus); ok {
			return state
		}
		return terminalproto.WorktreeResult{Code: terminalproto.OK, Path: state.Path, Branch: state.Branch}
	}
	got, err := f.b.WorktreeStatus(t.Context(), f.uid, f.agent, testProject, "site", testRun)
	if err != nil || got.Path != state.Path || got.Branch != state.Branch || got.Changed != 2 || got.Unpushed != 1 || len(got.Paths) != 2 || got.Hidden != 1 || !got.Nested {
		t.Fatal("status", got, err)
	}
	removed, err := f.b.WorktreeRemove(t.Context(), f.uid, f.agent, testProject, "site", testRun, WorktreeCounts{Changed: 0, Unpushed: 1})
	if err != nil || removed.Path != state.Path || removed.Branch != state.Branch {
		t.Fatal("remove", removed, err)
	}
	sent := f.worktreeSent()
	if len(sent) != 2 || sent[0] != (terminalproto.WorktreeStatus{Run: testRun, Project: testProject, Repo: "site"}) || sent[1] != (terminalproto.WorktreeRemove{Run: testRun, Project: testProject, Repo: "site", Changed: 0, Unpushed: 1}) {
		t.Fatal("worktree requests not forwarded exactly", sent)
	}

	for _, tc := range []struct {
		name  string
		reply terminalproto.Body
		check func(error) bool
	}{
		{"not found", terminalproto.WorktreeResult{Code: terminalproto.NotFound}, func(err error) bool { return errors.Is(err, ErrWorktreeNotFound) }},
		{"changed", terminalproto.WorktreeResult{Code: terminalproto.WorktreeChanged}, func(err error) bool { return errors.Is(err, ErrWorktreeChanged) }},
		{"refused", terminalproto.WorktreeResult{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeHasChanges}, func(err error) bool {
			var refusal *WorktreeError
			return errors.Is(err, ErrWorktreeRefused) && errors.As(err, &refusal) && refusal.Reason == terminalproto.WorktreeHasChanges
		}},
		{"refused with detail", terminalproto.WorktreeResult{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeGitFailed, Detail: "fatal: locked"}, func(err error) bool {
			var refusal *WorktreeError
			return errors.As(err, &refusal) && refusal.Reason == terminalproto.WorktreeGitFailed && refusal.Detail == "fatal: locked"
		}},
		{"denied", terminalproto.WorktreeResult{Code: terminalproto.Denied}, func(err error) bool { return errors.Is(err, ErrDenied) }},
		{"busy", terminalproto.WorktreeResult{Code: terminalproto.Conflict}, func(err error) bool { return errors.Is(err, ErrWorktreeBusy) }},
		{"unavailable", terminalproto.WorktreeResult{Code: terminalproto.Unavailable}, func(err error) bool { return errors.Is(err, ErrWorktreeUnavailable) }},
		{"failed", terminalproto.WorktreeResult{Code: terminalproto.Failed}, func(err error) bool { return errors.Is(err, ErrWorktreeUnavailable) }},
		{"capacity", terminalproto.WorktreeResult{Code: terminalproto.Capacity}, func(err error) bool { return errors.Is(err, ErrWorktreeUnavailable) }},
		{"wrong reply type", terminalproto.Result{Code: terminalproto.OK}, func(err error) bool { return errors.Is(err, ErrOffline) }},
	} {
		f.worktree = func(terminalproto.Body) terminalproto.Body { return tc.reply }
		if _, err := f.b.WorktreeStatus(t.Context(), f.uid, f.agent, testProject, "site", testRun); !tc.check(err) {
			t.Fatal("status", tc.name, err)
		}
		if _, err := f.b.WorktreeRemove(t.Context(), f.uid, f.agent, testProject, "site", testRun, WorktreeCounts{}); !tc.check(err) {
			t.Fatal("remove", tc.name, err)
		}
	}
	// A state is never an answer to a remove.
	f.worktree = func(terminalproto.Body) terminalproto.Body { return state }
	if _, err := f.b.WorktreeRemove(t.Context(), f.uid, f.agent, testProject, "site", testRun, WorktreeCounts{}); !errors.Is(err, ErrOffline) {
		t.Fatal("state answered a remove", err)
	}
}

func TestWorktreeRequestsAreValidatedBeforeSending(t *testing.T) {
	f := launchFixture(t, worktreeCapabilities("claude"))
	for name, args := range map[string][3]string{
		"bad run":       {testProject, "site", "XYZ"},
		"bad project":   {"My Project", "site", testRun},
		"empty repo":    {testProject, "", testRun},
		"repo path":     {testProject, "../site", testRun},
		"flag-ish repo": {testProject, "-site", testRun},
	} {
		if _, err := f.b.WorktreeStatus(t.Context(), f.uid, f.agent, args[0], args[1], args[2]); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatal("status", name, err)
		}
		if _, err := f.b.WorktreeRemove(t.Context(), f.uid, f.agent, args[0], args[1], args[2], WorktreeCounts{}); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatal("remove", name, err)
		}
	}
	if _, err := f.b.WorktreeRemove(t.Context(), f.uid, f.agent, testProject, "site", testRun, WorktreeCounts{Changed: -1}); !errors.Is(err, terminalproto.ErrInvalid) {
		t.Fatal("negative count", err)
	}
	if sent := f.worktreeSent(); len(sent) != 0 {
		t.Fatal("an invalid worktree request was sent", sent)
	}
}

func TestWorktreeRequestsNeedAReadyPeer(t *testing.T) {
	f := launchFixture(t, worktreeCapabilities("claude"))
	if _, err := f.b.WorktreeStatus(t.Context(), f.uid, "missing-agent", testProject, "site", testRun); !errors.Is(err, ErrOffline) {
		t.Fatal("status without a peer", err)
	}
	p, err := f.b.peer(f.uid, f.agent, false)
	if err != nil {
		t.Fatal(err)
	}
	p.Capabilities(false)
	if _, err := f.b.WorktreeStatus(t.Context(), f.uid, f.agent, testProject, "site", testRun); !errors.Is(err, ErrOffline) {
		t.Fatal("status on a disabled peer", err)
	}
	if _, err := f.b.WorktreeRemove(t.Context(), f.uid, f.agent, testProject, "site", testRun, WorktreeCounts{}); !errors.Is(err, ErrOffline) {
		t.Fatal("remove on a disabled peer", err)
	}
	if len(f.worktreeSent()) != 0 {
		t.Fatal("a worktree request reached a disabled peer")
	}
}

// Worktree requests can run git for up to a minute on the machine, so they
// wait longer than the ordinary ten-second control call.
func TestWorktreeRequestsWaitLongerThanOrdinaryCalls(t *testing.T) {
	ordinary, worktree := callTimeout, WorktreeCallTimeout
	callTimeout, WorktreeCallTimeout = 100*time.Millisecond, 5*time.Second
	t.Cleanup(func() { callTimeout, WorktreeCallTimeout = ordinary, worktree })
	f := launchFixture(t, worktreeCapabilities("claude"))
	f.mu.Lock()
	f.delay = 400 * time.Millisecond
	f.mu.Unlock()
	f.worktree = func(terminalproto.Body) terminalproto.Body {
		return terminalproto.WorktreeState{Path: "/home/test/.reet/worktrees/my-project/site/" + testRun, Branch: "reet/my-project-" + testRun}
	}
	if _, err := f.b.WorktreeStatus(t.Context(), f.uid, f.agent, testProject, "site", testRun); err != nil {
		t.Fatal("slow status", err)
	}
	if s, err := f.b.Create(t.Context(), f.uid, f.agent, ID(), worktreeCreate()); err != nil || s.State != terminalproto.Running {
		t.Fatal("slow worktree create", s, err)
	}
	// A plain create keeps the ordinary limit.
	if _, err := f.b.Create(t.Context(), f.uid, f.agent, ID(), terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24}); !errors.Is(err, ErrOffline) {
		t.Fatal("plain create waited past the ordinary limit", err)
	}
}

func TestLateWorktreeRepliesAreIgnored(t *testing.T) {
	f := launchFixture(t, worktreeCapabilities("claude"))
	p, err := f.b.peer(f.uid, f.agent, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []terminalproto.Body{
		&terminalproto.WorktreeState{Path: "/home/test/x", Branch: "reet/p-" + testRun},
		&terminalproto.WorktreeResult{Code: terminalproto.OK},
		&terminalproto.Result{Code: terminalproto.OK},
	} {
		if err := p.Receive(terminalproto.Message{Version: terminalproto.Version, RequestID: ID(), Generation: p.Generation, Body: body}); err != nil {
			t.Fatalf("late %T refused: %v", body, err)
		}
	}
	if err := p.Receive(terminalproto.Message{Version: terminalproto.Version, RequestID: ID(), Generation: p.Generation, Body: &terminalproto.Probe{Sequence: 1, Nonce: ID()}}); err == nil {
		t.Fatal("an unexpected message type was accepted")
	}
}

// A machine answers a second worktree request busy at once, so a burst of
// status requests while one is running never fills the 32 call slots.
func TestWorktreeStatusBurstWhileOneRunsGetsBusyNotSlotExhaustion(t *testing.T) {
	f := launchFixture(t, worktreeCapabilities("claude"))
	release := make(chan struct{})
	var once sync.Once
	p, err := f.b.peer(f.uid, f.agent, false)
	if err != nil {
		t.Fatal(err)
	}
	f.worktree = func(body terminalproto.Body) terminalproto.Body {
		first := false
		once.Do(func() { first = true })
		if first {
			return nil // answered below, after the burst
		}
		return terminalproto.WorktreeResult{Code: terminalproto.Conflict}
	}
	// The fixture sends nothing for a nil answer; deliver the first state late.
	f.late = func(m terminalproto.Message) {
		go func() {
			<-release
			m.Body = &terminalproto.WorktreeState{Path: "/home/test/.reet/worktrees/my-project/site/" + testRun, Branch: "reet/my-project-" + testRun}
			_ = p.Receive(m)
		}()
	}
	firstDone := make(chan error, 1)
	go func() {
		_, err := f.b.WorktreeStatus(t.Context(), f.uid, f.agent, testProject, "site", testRun)
		firstDone <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	for len(f.worktreeSent()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	start := time.Now()
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.b.WorktreeStatus(t.Context(), f.uid, f.agent, testProject, "site", testRun)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	if time.Since(start) > 2*time.Second {
		t.Fatal("busy answers were slow", time.Since(start))
	}
	for err := range errs {
		if !errors.Is(err, ErrWorktreeBusy) {
			t.Fatal("a request in the burst was not answered busy", err)
		}
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal("the running request", err)
	}
}

func TestWorktreeCreateRecordsWhereTheMachineMadeIt(t *testing.T) {
	f := launchFixture(t, worktreeCapabilities("claude"))
	path := "/home/test/.reet/worktrees/my-project/site/" + testRun
	f.reply = func(terminalproto.Create) terminalproto.Body {
		return terminalproto.SessionChanged{State: terminalproto.Running, WorktreePath: path, WorktreeRepo: "site"}
	}
	request := ID()
	s, started, err := f.b.CreateSession(t.Context(), f.uid, f.agent, request, worktreeCreate())
	if err != nil || !started || s.WorktreePath != path || s.WorktreeRepo != "site" || s.WorktreeBranch != "reet/"+testProject+"-"+testRun {
		t.Fatal("worktree not recorded", s, started, err)
	}
	again, _, err := f.b.CreateSession(t.Context(), f.uid, f.agent, request, worktreeCreate())
	if err != nil || again.WorktreePath != path || len(f.sent()) != 1 {
		t.Fatal("replay lost the worktree", again, err)
	}
	// A late reply (after the call gave up) still records it.
	ordinary := WorktreeCallTimeout
	WorktreeCallTimeout = 100 * time.Millisecond
	t.Cleanup(func() { WorktreeCallTimeout = ordinary })
	f.mu.Lock()
	f.delay = 400 * time.Millisecond
	f.mu.Unlock()
	in := worktreeCreate()
	in.Worktree = "fedcba9876543210fedcba98"
	path = "/home/test/.reet/worktrees/my-project/site/" + in.Worktree
	late, _, err := f.b.CreateSession(t.Context(), f.uid, f.agent, ID(), in)
	if !errors.Is(err, ErrOffline) {
		t.Fatal("expected the call to give up", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := f.st.TerminalSessionFor(f.uid, late.ID)
		if got.State == terminalproto.Running && got.WorktreePath == path {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	got, _ := f.st.TerminalSessionFor(f.uid, late.ID)
	t.Fatal("a late create reply did not record the worktree", got)
}

// A remove whose reply never comes after it was sent is reported as a lost
// reply (still a reconnect to callers that do not check), never as a refusal.
func TestWorktreeRemoveReplyLostIsDistinguished(t *testing.T) {
	ordinary := WorktreeCallTimeout
	WorktreeCallTimeout = 150 * time.Millisecond
	t.Cleanup(func() { WorktreeCallTimeout = ordinary })
	f := launchFixture(t, worktreeCapabilities("claude"))
	f.worktree = func(terminalproto.Body) terminalproto.Body { return nil }
	f.late = func(terminalproto.Message) {}
	_, err := f.b.WorktreeRemove(t.Context(), f.uid, f.agent, testProject, "site", testRun, WorktreeCounts{})
	if !errors.Is(err, ErrWorktreeReplyLost) || !errors.Is(err, ErrOffline) {
		t.Fatal("lost remove reply", err)
	}
	// A request never sent (no ready peer) is a plain reconnect.
	if _, err := f.b.WorktreeRemove(t.Context(), f.uid, "missing-agent", testProject, "site", testRun, WorktreeCounts{}); errors.Is(err, ErrWorktreeReplyLost) || !errors.Is(err, ErrOffline) {
		t.Fatal("unsent remove", err)
	}
}

// The controller waits long enough for a remove queued behind a create.
func TestWorktreeCallTimeoutCoversACreateThenARemove(t *testing.T) {
	if WorktreeCallTimeout != 180*time.Second {
		t.Fatal(WorktreeCallTimeout)
	}
}

// A remove's ok answer that arrives after the call gave up is still
// reported, so the removal can be audited; nothing else is.
func TestLateRemoveAnswerIsReported(t *testing.T) {
	ordinary := WorktreeCallTimeout
	WorktreeCallTimeout = 100 * time.Millisecond
	t.Cleanup(func() { WorktreeCallTimeout = ordinary })
	f := launchFixture(t, worktreeCapabilities("claude"))
	p, err := f.b.peer(f.uid, f.agent, false)
	if err != nil {
		t.Fatal(err)
	}
	type report struct {
		uid    int64
		target WorktreeTarget
		result terminalproto.WorktreeResult
	}
	reports := make(chan report, 4)
	f.b.OnLateWorktreeRemoval(func(uid int64, target WorktreeTarget, result terminalproto.WorktreeResult) {
		reports <- report{uid, target, result}
	})
	path := "/home/test/.reet/worktrees/my-project/site/" + testRun
	branch := "reet/my-project-" + testRun
	var answer terminalproto.WorktreeResult
	f.worktree = func(terminalproto.Body) terminalproto.Body { return nil }
	f.late = func(m terminalproto.Message) {
		late := answer
		go func() {
			time.Sleep(300 * time.Millisecond)
			m.Body = &late
			_ = p.Receive(m)
		}()
	}
	for _, tc := range []struct {
		name     string
		answer   terminalproto.WorktreeResult
		reported bool
	}{
		{"ok", terminalproto.WorktreeResult{Code: terminalproto.OK, Path: path, Branch: branch}, true},
		{"refused", terminalproto.WorktreeResult{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeHasChanges}, false},
		{"wrong branch", terminalproto.WorktreeResult{Code: terminalproto.OK, Path: path, Branch: "main"}, false},
		{"wrong folder", terminalproto.WorktreeResult{Code: terminalproto.OK, Path: "/home/test/elsewhere", Branch: branch}, false},
	} {
		answer = tc.answer
		if _, err := f.b.WorktreeRemove(t.Context(), f.uid, f.agent, testProject, "site", testRun, WorktreeCounts{}); !errors.Is(err, ErrWorktreeReplyLost) {
			t.Fatal(tc.name, err)
		}
		select {
		case r := <-reports:
			if !tc.reported {
				t.Fatal(tc.name, "was reported", r)
			}
			if r.uid != f.uid || r.target != (WorktreeTarget{Agent: f.agent, Project: testProject, Repo: "site", Run: testRun}) || r.result != tc.answer {
				t.Fatal(tc.name, r)
			}
		case <-time.After(time.Second):
			if tc.reported {
				t.Fatal(tc.name, "late ok was not reported")
			}
		}
	}
	// An ok for a request this peer never sent is not reported.
	if err := p.Receive(terminalproto.Message{Version: terminalproto.Version, RequestID: ID(), Generation: p.Generation, Body: &terminalproto.WorktreeResult{Code: terminalproto.OK, Path: path, Branch: branch}}); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-reports:
		t.Fatal("an unrequested ok was reported", r)
	case <-time.After(300 * time.Millisecond):
	}
}

// The controller checks what the machine reports against the request.
func TestWorktreeAnswersMustMatchTheRequest(t *testing.T) {
	f := launchFixture(t, worktreeCapabilities("claude"))
	path := "/home/test/.reet/worktrees/my-project/site/" + testRun
	for name, answer := range map[string]terminalproto.WorktreeResult{
		"wrong branch":  {Code: terminalproto.OK, Path: path, Branch: "reet/other-" + testRun},
		"wrong folder":  {Code: terminalproto.OK, Path: "/home/test/.reet/worktrees/my-project/other/" + testRun, Branch: "reet/my-project-" + testRun},
		"outside reet":  {Code: terminalproto.OK, Path: "/home/test/site", Branch: "reet/my-project-" + testRun},
		"other run dir": {Code: terminalproto.OK, Path: "/home/test/.reet/worktrees/my-project/site/" + testRun + "x", Branch: "reet/my-project-" + testRun},
	} {
		f.worktree = func(terminalproto.Body) terminalproto.Body { return answer }
		if _, err := f.b.WorktreeRemove(t.Context(), f.uid, f.agent, testProject, "site", testRun, WorktreeCounts{}); !errors.Is(err, ErrWorktreeMismatch) {
			t.Fatal(name, err)
		}
	}
	// A create whose reply names a folder that is not this run's is not recorded.
	f.reply = func(terminalproto.Create) terminalproto.Body {
		return terminalproto.SessionChanged{State: terminalproto.Running, WorktreePath: "/home/test/elsewhere", WorktreeRepo: "site"}
	}
	s, _, err := f.b.CreateSession(t.Context(), f.uid, f.agent, ID(), worktreeCreate())
	if !errors.Is(err, ErrWorktreeMismatch) {
		t.Fatal("create with a mismatched folder", err)
	}
	if got, _ := f.st.TerminalSessionFor(f.uid, s.ID); got.WorktreePath != "" {
		t.Fatal("a mismatched folder was recorded", got)
	}
}

// A replay of a request id the controller already reserved answers with that
// session even when the machine's capabilities changed since: the agent may be
// running, and refusing the replay would let a caller think nothing started.
// A new request id is still refused.
func TestReplayOfAReservedCreateIgnoresACapabilityChange(t *testing.T) {
	f := launchFixture(t, worktreeCapabilities("claude"))
	in := worktreeCreate()
	request := ID()
	first, started, err := f.b.CreateSession(t.Context(), f.uid, f.agent, request, in)
	if err != nil || !started {
		t.Fatal(first, started, err)
	}
	caps, err := f.st.RemoteCapabilitiesFor(f.uid, f.agent)
	if err != nil {
		t.Fatal(err)
	}
	for _, flipped := range []terminalproto.Capabilities{launchCapabilities("claude"), legacyCapabilities()} {
		if err := f.st.UpdateRemoteCapabilitiesFor(f.uid, f.agent, caps.InstanceID, caps.Generation, flipped); err != nil {
			t.Fatal(err)
		}
		again, started, err := f.b.CreateSession(t.Context(), f.uid, f.agent, request, in)
		if err != nil || started || again.ID != first.ID {
			t.Fatal("replay after a capability change", again, started, err)
		}
		if _, _, err := f.b.CreateSession(t.Context(), f.uid, f.agent, ID(), in); !errors.Is(err, ErrWorktreeUnsupported) {
			t.Fatal("a new request was not refused", err)
		}
	}
	if len(f.sent()) != 1 {
		t.Fatal("dispatched again", f.sent())
	}
}
