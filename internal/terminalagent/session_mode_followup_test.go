//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// idEntries returns the reconcile entries that name the swapped session.
func idEntries(rec terminalproto.Reconcile) []terminalproto.SessionSnapshot {
	var out []terminalproto.SessionSnapshot
	for _, s := range rec.Sessions {
		if s.ID == swapSession {
			out = append(out, s)
		}
	}
	return out
}

// A swap whose start and rollback both fail retires the id. A reconcile taken
// after the swap mark is released and before the ended notice is sent must not
// see the dead id as live, and the chat quota must not count it.
func TestRetiredSwapNeverReportedLive(t *testing.T) {
	r := newSwapRig(t)
	r.openChat(t)
	r.shells.createErr = terminalhost.ErrNotRunning
	r.mu.Lock()
	r.startErrs = []error{errors.New("restore failed")}
	r.mu.Unlock()
	var (
		seen    []terminalproto.SessionSnapshot
		live    int
		hookErr error
		ran     bool
	)
	previous := beforeRetireNotice
	beforeRetireNotice = func() {
		ran = true
		sw := &agentSwitch{chats: &chatHost{sup: r.sup, link: &chatLink{history: true}}, shells: r.shells}
		rec, err := sw.reconcile(context.Background(), swapFeatures())
		hookErr = err
		seen = idEntries(rec)
		live = r.sup.liveTool("opencode")
	}
	t.Cleanup(func() { beforeRetireNotice = previous })
	body := toTerminal(r.home)
	r.send(t, "request-swap-000001", &body)
	if _, ok := r.reply(t, "request-swap-000001").(terminalproto.Result); !ok {
		t.Fatal("swap did not fail")
	}
	if !ran || hookErr != nil {
		t.Fatalf("hook ran %v err %v", ran, hookErr)
	}
	if len(seen) > 1 {
		t.Fatalf("reconcile lists the id %d times: %+v", len(seen), seen)
	}
	for _, s := range seen {
		if s.State.ReservesQuota() {
			t.Fatalf("reconcile between retire and notice reports the dead id %s", s.State)
		}
	}
	if live != 0 {
		t.Fatalf("liveTool counts the dead id: %d", live)
	}
	retiredOnce(t, r)
}

// When the chat's receipt cannot be forgotten after it was closed (forced here
// by removing it while Close runs), the swap is refused and the controller is
// still told the row ended through the retire path.
func TestSwapForgetConflictRetiresRow(t *testing.T) {
	r := newSwapRig(t)
	r.openChat(t)
	proc := r.procs[0]
	proc.gate, proc.entered = make(chan struct{}), make(chan struct{}, 1)
	body := toTerminal(r.home)
	r.send(t, "request-swap-000001", &body)
	<-proc.entered
	r.sup.mu.Lock()
	delete(r.sup.sessions, swapSession)
	r.sup.mu.Unlock()
	close(proc.gate)
	if got := r.reply(t, "request-swap-000001"); !isConflict(got) {
		t.Fatalf("swap reply %#v, want conflict", got)
	}
	if n := r.shells.createCount(); n != 0 {
		t.Fatalf("a terminal was started: %d", n)
	}
	retiredOnce(t, r)
}

// A chat whose process group Close could not confirm gone keeps counting
// against the chat quota and liveTool (so a software apply cannot replace the
// binary under it) while its row is reported ended; once a later Close
// confirms the group gone, it stops counting.
func TestSwapChatStillRunningCountsUntilGone(t *testing.T) {
	previous := strayRecheckInterval
	strayRecheckInterval = 10 * time.Millisecond
	t.Cleanup(func() { strayRecheckInterval = previous })
	r := newSwapRig(t)
	r.openChat(t)
	proc := r.procs[0]
	proc.lingers.Store(true)
	body := toTerminal(r.home)
	r.send(t, "request-swap-000001", &body)
	if _, ok := r.reply(t, "request-swap-000001").(terminalproto.Result); !ok {
		t.Fatal("swap did not fail")
	}
	retiredOnce(t, r)
	if n := r.sup.liveTool("opencode"); n != 1 {
		t.Fatalf("liveTool = %d while the old group may be alive, want 1", n)
	}
	r.sup.mu.Lock()
	quota := r.sup.quotaLocked()
	r.sup.mu.Unlock()
	if quota != 1 {
		t.Fatalf("chat quota counts %d, want 1", quota)
	}
	time.Sleep(50 * time.Millisecond)
	if n := r.sup.liveTool("opencode"); n != 1 {
		t.Fatalf("stopped counting before the group was confirmed gone: %d", n)
	}
	proc.lingers.Store(false)
	deadline := time.Now().Add(2 * time.Second)
	for r.sup.liveTool("opencode") != 0 {
		if time.Now().After(deadline) {
			t.Fatal("still counted after the group was confirmed gone")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// The row stays retired: confirming the group gone sends nothing more.
	r.send(t, "request-reconcile-02", &terminalproto.Reconcile{})
	rec := r.reply(t, "request-reconcile-02").(terminalproto.Reconcile)
	for _, s := range idEntries(rec) {
		if s.State.ReservesQuota() {
			t.Fatalf("reconcile reports %s", s.State)
		}
	}
}

// A switch to the chat in a folder that does not resolve is refused with the
// folder error before the terminal is stopped.
func TestSwapToChatUnresolvableFolderLeavesTerminalUntouched(t *testing.T) {
	h, _, _ := testChatHost(t)
	starts := 0
	start := h.sup.start
	h.sup.start = func(ctx context.Context, opts acpagent.Options) (chatProcess, error) {
		starts++
		return start(ctx, opts)
	}
	shells := newFakeShells()
	if _, err := shells.Create(t.Context(), "request-0123456789", swapSession, terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", LaunchSession: swapACP}); err != nil {
		t.Fatal(err)
	}
	sw := &agentSwitch{chats: h, shells: shells}
	_, err := sw.swap(t.Context(), "request-swap-000001", swapSession, toChat(filepath.Join(h.sup.opts.Home, "missing")))
	if !errors.Is(err, terminalhost.ErrLaunchUnavailable) {
		t.Fatalf("err = %v, want the folder refusal", err)
	}
	// The same answer a chat start in that folder gives.
	if got := chatResult(err); got.Code != terminalproto.LaunchFailed {
		t.Fatalf("answered %+v", got)
	}
	shells.mu.Lock()
	ends, creates := shells.ends, len(shells.creates)
	s := shells.sessions[swapSession]
	shells.mu.Unlock()
	if ends != 0 || creates != 1 || s.State != terminalproto.Running {
		t.Fatalf("the terminal was touched: ends %d creates %d state %s", ends, creates, s.State)
	}
	if starts != 0 {
		t.Fatalf("chat starts = %d", starts)
	}
	if _, hidden := h.sup.swapView(); len(hidden) != 0 {
		t.Fatalf("the swap mark was left behind: %v", hidden)
	}
}

// churnLister is a terminal listing that lands a swap every time it is read,
// so listShells never sees a stable epoch.
type churnLister struct {
	sup   *chatSupervisor
	calls atomic.Int64
}

func (c *churnLister) List(context.Context) ([]terminalhost.ManagedSession, error) {
	c.calls.Add(1)
	c.sup.mu.Lock()
	c.sup.swapEpoch++
	c.sup.mu.Unlock()
	return nil, nil
}

// listShells yields between retries: under constant swap churn it ends with
// the context and reads the listing at most about once a millisecond.
func TestListShellsYieldsBetweenRetries(t *testing.T) {
	h, _, _ := testChatHost(t)
	lister := &churnLister{sup: h.sup}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, _, err := listShells(ctx, lister, h.sup)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("listShells never returned")
	}
	if n := lister.calls.Load(); n > 60 {
		t.Fatalf("listShells retried %d times in 50ms without yielding", n)
	}
}

// A prompt that arrives while a chat-to-terminal swap is closing the chat is
// refused busy and never reaches the old process; the swap still finishes.
func TestSwapRefusesPromptWhileChatToTerminal(t *testing.T) {
	r := newSwapRig(t)
	r.openChat(t)
	proc := r.procs[0]
	proc.gate, proc.entered = make(chan struct{}), make(chan struct{}, 1)
	body := toTerminal(r.home)
	r.send(t, "request-swap-000001", &body)
	<-proc.entered
	r.send(t, "request-prompt-0001", &terminalproto.ACPPrompt{Text: "hello"})
	if got := r.reply(t, "request-prompt-0001"); !isConflict(got) {
		t.Fatalf("prompt during the swap answered %#v, want conflict", got)
	}
	close(proc.gate)
	if changed, ok := r.reply(t, "request-swap-000001").(terminalproto.SessionChanged); !ok || changed.State != terminalproto.Running {
		t.Fatalf("swap reply %#v", changed)
	}
	proc.mu.Lock()
	prompted := proc.prompted
	proc.mu.Unlock()
	if prompted != "" {
		t.Fatalf("the swapped-out chat was prompted %q", prompted)
	}
	if n := r.live(t); n != 1 {
		t.Fatalf("live processes for the id: %d", n)
	}
}

// A prompt that arrives while a terminal-to-chat swap is still stopping the
// terminal finds no chat: it is answered not found (the controller's prompt is
// not queued for the chat that opens next), and the swap still finishes.
func TestSwapRefusesPromptWhileTerminalToChat(t *testing.T) {
	r := newSwapRig(t)
	r.openTerminal(t)
	r.shells.forgetGate, r.shells.forgetEntered = make(chan struct{}), make(chan struct{}, 1)
	body := toChat(r.home)
	r.send(t, "request-swap-000001", &body)
	<-r.shells.forgetEntered
	r.send(t, "request-prompt-0001", &terminalproto.ACPPrompt{Text: "hello"})
	got, ok := r.reply(t, "request-prompt-0001").(terminalproto.Result)
	if !ok || got.Code != terminalproto.NotFound {
		t.Fatalf("prompt during the swap answered %#v, want not_found", got)
	}
	close(r.shells.forgetGate)
	if changed, ok := r.reply(t, "request-swap-000001").(terminalproto.SessionChanged); !ok || changed.State != terminalproto.Running {
		t.Fatalf("swap reply %#v", changed)
	}
	if r.chatStarts() != 1 {
		t.Fatalf("chats started: %d", r.chatStarts())
	}
	proc := r.procs[0]
	proc.mu.Lock()
	prompted := proc.prompted
	proc.mu.Unlock()
	if prompted != "" {
		t.Fatalf("the new chat was prompted %q", prompted)
	}
}

// A control connection closed while a terminal-to-chat swap on the real tmux
// manager is starting the chat still lets the swap finish: the chat runs and
// the terminal is gone.
func TestSwapSurvivesConnectionCleanupTerminalToChat(t *testing.T) {
	opts, home := realTmux(t, "#!/bin/sh\nexec sleep 600\n")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	r := &swapRig{out: &sent{}, home: home}
	r.sup = newChatSupervisor(ctx, opts)
	r.sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	gate, entered := make(chan struct{}), make(chan struct{}, 1)
	var gateOnce sync.Once
	r.sup.start = func(c context.Context, o acpagent.Options) (chatProcess, error) {
		gateOnce.Do(func() {
			entered <- struct{}{}
			<-gate
		})
		return r.start(c, o)
	}
	headers := http.Header{terminalproto.FeatureHeader: {swapFeatures()}}
	connCtx, drop := context.WithCancel(ctx)
	handle, cleanup := FactoryWithChats(opts, &ChatSupervisor{s: r.sup})(connCtx, http.DefaultClient, headers, 1, r.out.send)
	r.handle = handle
	r.send(t, "request-tui-0000001", &terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Dir: home, LaunchSession: swapACP})
	if changed, ok := r.reply(t, "request-tui-0000001").(terminalproto.SessionChanged); !ok || changed.State != terminalproto.Running {
		t.Fatalf("terminal did not start: %#v", changed)
	}
	body := toChat(home)
	r.send(t, "request-swap-000001", &body)
	select {
	case <-entered:
	case <-time.After(30 * time.Second):
		t.Fatal("the swap never reached the chat start")
	}
	drop()
	cleaned := make(chan struct{})
	go func() { cleanup(); close(cleaned) }()
	time.Sleep(50 * time.Millisecond)
	close(gate)
	select {
	case <-cleaned:
	case <-time.After(90 * time.Second):
		t.Fatal("cleanup never finished")
	}

	m, err := terminalhost.NewManager(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	sessions, err := m.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	terminals := 0
	for _, s := range sessions {
		if s.ID == swapSession && s.State.ReservesQuota() {
			terminals++
		}
	}
	chat := 0
	if s, ok := (&chatHost{sup: r.sup, link: &chatLink{}}).snapshot(swapSession); ok && s.State.ReservesQuota() {
		chat++
	}
	if terminals != 0 || chat != 1 {
		t.Fatalf("after the swap: %d live terminals, %d live chats (%+v)", terminals, chat, sessions)
	}
}
