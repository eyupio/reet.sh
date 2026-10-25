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
		if err := f.b.WorktreeRemove(t.Context(), f.uid, f.agent, testProject, "site", testRun, WorktreeCounts{}); !errors.Is(err, ErrWorktreeUnsupported) {
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
	// The reason is not stored, so a replay reports the refusal without it.
	_, started, err = f.b.CreateSession(t.Context(), f.uid, f.agent, request, in)
	if !errors.Is(err, ErrWorktreeRefused) || started {
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
		return terminalproto.WorktreeResult{Code: terminalproto.OK}
	}
	got, err := f.b.WorktreeStatus(t.Context(), f.uid, f.agent, testProject, "site", testRun)
	if err != nil || got.Path != state.Path || got.Branch != state.Branch || got.Changed != 2 || got.Unpushed != 1 || len(got.Paths) != 2 || got.Hidden != 1 || !got.Nested {
		t.Fatal("status", got, err)
	}
	if err := f.b.WorktreeRemove(t.Context(), f.uid, f.agent, testProject, "site", testRun, WorktreeCounts{Changed: 0, Unpushed: 1}); err != nil {
		t.Fatal("remove", err)
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
		if err := f.b.WorktreeRemove(t.Context(), f.uid, f.agent, testProject, "site", testRun, WorktreeCounts{}); !tc.check(err) {
			t.Fatal("remove", tc.name, err)
		}
	}
	// A state is never an answer to a remove.
	f.worktree = func(terminalproto.Body) terminalproto.Body { return state }
	if err := f.b.WorktreeRemove(t.Context(), f.uid, f.agent, testProject, "site", testRun, WorktreeCounts{}); !errors.Is(err, ErrOffline) {
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
		if err := f.b.WorktreeRemove(t.Context(), f.uid, f.agent, args[0], args[1], args[2], WorktreeCounts{}); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatal("remove", name, err)
		}
	}
	if err := f.b.WorktreeRemove(t.Context(), f.uid, f.agent, testProject, "site", testRun, WorktreeCounts{Changed: -1}); !errors.Is(err, terminalproto.ErrInvalid) {
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
	if err := f.b.WorktreeRemove(t.Context(), f.uid, f.agent, testProject, "site", testRun, WorktreeCounts{}); !errors.Is(err, ErrOffline) {
		t.Fatal("remove on a disabled peer", err)
	}
	if len(f.worktreeSent()) != 0 {
		t.Fatal("a worktree request reached a disabled peer")
	}
}

// Worktree requests can run git for up to a minute on the machine, so they
// wait longer than the ordinary ten-second control call.
func TestWorktreeRequestsWaitLongerThanOrdinaryCalls(t *testing.T) {
	ordinary, worktree := callTimeout, worktreeCallTimeout
	callTimeout, worktreeCallTimeout = 100*time.Millisecond, 5*time.Second
	t.Cleanup(func() { callTimeout, worktreeCallTimeout = ordinary, worktree })
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
