//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// gatedChat is a fake chat whose Close waits on gate, so a test can hold a
// swap inside chats.end.
type gatedChat struct {
	*fakeChat
	gate    chan struct{}
	entered chan struct{}
	// lingers makes Close fail to confirm the process group gone.
	lingers bool
}

func (g *gatedChat) GroupRetired() bool { return !g.lingers }

func (g *gatedChat) Close() {
	if g.gate != nil {
		select {
		case g.entered <- struct{}{}:
		default:
		}
		<-g.gate
	}
	g.fakeChat.Close()
}

// swapRig is a control connection whose terminal side is a fakeShells and
// whose chats are fake processes.
type swapRig struct {
	handle func(context.Context, terminalproto.Message) error
	out    *sent
	sup    *chatSupervisor
	shells *fakeShells
	home   string

	mu        sync.Mutex
	procs     []*gatedChat
	startErrs []error // consumed one per chat start; nil entries succeed
}

func swapFeatures() string {
	return terminalproto.LaunchFeature + "," + terminalproto.ACPFeature + "," + terminalproto.ACPHistoryFeature + "," + terminalproto.SessionModeFeature
}

func newSwapRig(t *testing.T) *swapRig {
	t.Helper()
	ctx := t.Context()
	r := &swapRig{out: &sent{}, shells: newFakeShells(), home: t.TempDir()}
	opts := terminalhost.ManagerOptions{Home: r.home}
	r.sup = newChatSupervisor(ctx, opts)
	r.sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	r.sup.start = r.start
	previous := swapShells
	swapShells = func(*terminalhost.Manager) shellHost { return r.shells }
	t.Cleanup(func() { swapShells = previous })
	headers := http.Header{terminalproto.FeatureHeader: {swapFeatures()}}
	handle, cleanup := FactoryWithChats(opts, &ChatSupervisor{s: r.sup})(ctx, http.DefaultClient, headers, 1, r.out.send)
	t.Cleanup(cleanup)
	r.handle = handle
	return r
}

func (r *swapRig) start(_ context.Context, opts acpagent.Options) (chatProcess, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.startErrs) > 0 {
		err := r.startErrs[0]
		r.startErrs = r.startErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	p := &gatedChat{fakeChat: &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{}), onEvent: opts.OnEvent}}
	r.procs = append(r.procs, p)
	return p, nil
}

func (r *swapRig) chatStarts() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.procs)
}

func (r *swapRig) send(t *testing.T, requestID string, body terminalproto.Body) {
	t.Helper()
	msg := terminalproto.Message{Version: terminalproto.Version, Generation: 1, RequestID: requestID, SessionID: swapSession, Body: body}
	if _, ok := body.(*terminalproto.Reconcile); ok {
		msg.SessionID = ""
	}
	if err := msg.Validate(); err != nil {
		t.Fatalf("not a valid message: %v", err)
	}
	if err := r.handle(t.Context(), msg); err != nil {
		t.Fatal(err)
	}
}

func (r *swapRig) reply(t *testing.T, requestID string) terminalproto.Body {
	t.Helper()
	return r.out.waitFor(t, func(m terminalproto.Message) bool { return m.RequestID == requestID }).Body
}

func (r *swapRig) openChat(t *testing.T) {
	t.Helper()
	r.send(t, "request-open-000001", &terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: r.home})
	if changed, ok := r.reply(t, "request-open-000001").(terminalproto.SessionChanged); !ok || changed.State != terminalproto.Running {
		t.Fatalf("chat did not open: %+v", changed)
	}
}

func (r *swapRig) openTerminal(t *testing.T) {
	t.Helper()
	if _, err := r.shells.Create(t.Context(), "request-0123456789", swapSession, terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", LaunchSession: swapACP}); err != nil {
		t.Fatal(err)
	}
}

func isConflict(body terminalproto.Body) bool {
	r, ok := body.(terminalproto.Result)
	return ok && r.Code == terminalproto.Conflict
}

// liveOnce reports how many live processes hold the session: a live chat
// plus a live terminal.
func (r *swapRig) live(t *testing.T) int {
	t.Helper()
	n := 0
	if s, ok := (&chatHost{sup: r.sup, link: &chatLink{}}).snapshot(swapSession); ok && s.State.ReservesQuota() {
		n++
	}
	sessions, _ := r.shells.List(t.Context())
	for _, s := range sessions {
		if s.ID == swapSession && s.State.ReservesQuota() {
			n++
		}
	}
	return n
}

// An End that arrives while a chat-to-terminal swap is closing the chat is
// refused busy, and the swap finishes with exactly one process.
func TestSwapRefusesEndWhileChatToTerminal(t *testing.T) {
	r := newSwapRig(t)
	r.openChat(t)
	proc := r.procs[0]
	proc.gate, proc.entered = make(chan struct{}), make(chan struct{}, 1)
	body := toTerminal(r.home)
	r.send(t, "request-swap-000001", &body)
	<-proc.entered
	r.send(t, "request-end-0000001", &terminalproto.End{})
	close(proc.gate)
	if got := r.reply(t, "request-end-0000001"); !isConflict(got) {
		t.Fatalf("End during the swap answered %#v, want conflict", got)
	}
	if changed, ok := r.reply(t, "request-swap-000001").(terminalproto.SessionChanged); !ok || changed.State != terminalproto.Running {
		t.Fatalf("swap reply %#v", changed)
	}
	if n := r.live(t); n != 1 {
		t.Fatalf("live processes for the id: %d", n)
	}
}

// An End or Create that arrives while a terminal-to-chat swap has ended the
// terminal is refused busy, and the swap finishes with exactly one process.
func TestSwapRefusesEndWhileTerminalToChat(t *testing.T) {
	r := newSwapRig(t)
	r.openTerminal(t)
	r.shells.forgetGate, r.shells.forgetEntered = make(chan struct{}), make(chan struct{}, 1)
	body := toChat(r.home)
	r.send(t, "request-swap-000001", &body)
	<-r.shells.forgetEntered
	r.send(t, "request-end-0000001", &terminalproto.End{})
	r.send(t, "request-open-000002", &terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: r.home})
	if got := r.reply(t, "request-end-0000001"); !isConflict(got) {
		t.Fatalf("End during the swap answered %#v, want conflict", got)
	}
	if got := r.reply(t, "request-open-000002"); !isConflict(got) {
		t.Fatalf("Create during the swap answered %#v, want conflict", got)
	}
	close(r.shells.forgetGate)
	if changed, ok := r.reply(t, "request-swap-000001").(terminalproto.SessionChanged); !ok || changed.State != terminalproto.Running {
		t.Fatalf("swap reply %#v", changed)
	}
	if n := r.live(t); n != 1 || r.chatStarts() != 1 {
		t.Fatalf("live processes for the id: %d, chats started: %d", n, r.chatStarts())
	}
}

// reconcileDuring lands a whole swap between the terminal and chat reads of a
// dispatcher reconcile, and returns how often the reply lists the id.
func reconcileDuring(t *testing.T, r *swapRig, in terminalproto.SessionMode) []terminalproto.SessionSnapshot {
	t.Helper()
	var once sync.Once
	previous := betweenReconcileReads
	betweenReconcileReads = func() {
		once.Do(func() {
			body := in
			r.send(t, "request-swap-000001", &body)
			if _, ok := r.reply(t, "request-swap-000001").(terminalproto.SessionChanged); !ok {
				t.Error("the swap failed")
			}
		})
	}
	t.Cleanup(func() { betweenReconcileReads = previous })
	r.send(t, "request-reconcile-01", &terminalproto.Reconcile{})
	rec, ok := r.reply(t, "request-reconcile-01").(terminalproto.Reconcile)
	if !ok {
		t.Fatal("no reconcile reply")
	}
	var found []terminalproto.SessionSnapshot
	for _, s := range rec.Sessions {
		if s.ID == swapSession {
			found = append(found, s)
		}
	}
	return found
}

func TestSwapReconcileConsistentTerminalToChat(t *testing.T) {
	r := newSwapRig(t)
	r.openTerminal(t)
	if found := reconcileDuring(t, r, toChat(r.home)); len(found) != 1 || found[0].State != terminalproto.Running {
		t.Fatalf("reconcile lists the id %d times: %+v", len(found), found)
	}
}

func TestSwapReconcileConsistentChatToTerminal(t *testing.T) {
	r := newSwapRig(t)
	r.openChat(t)
	if found := reconcileDuring(t, r, toTerminal(r.home)); len(found) != 1 || found[0].State != terminalproto.Running {
		t.Fatalf("reconcile lists the id %d times: %+v", len(found), found)
	}
}

// endedNotice reports whether the controller was told the id ended.
func endedNotice(out *sent) bool {
	out.mu.Lock()
	defer out.mu.Unlock()
	for _, m := range out.msgs {
		if c, ok := m.Body.(*terminalproto.SessionChanged); ok && m.SessionID == swapSession && !c.State.ReservesQuota() {
			return true
		}
	}
	return false
}

func TestSwapDoubleFailureReportsEndChatToTerminal(t *testing.T) {
	r := newSwapRig(t)
	r.openChat(t)
	r.shells.createErr = terminalhost.ErrNotRunning
	r.mu.Lock()
	r.startErrs = []error{errors.New("restore failed")}
	r.mu.Unlock()
	body := toTerminal(r.home)
	r.send(t, "request-swap-000001", &body)
	if got := r.reply(t, "request-swap-000001"); isConflict(got) {
		t.Fatalf("swap reply %#v", got)
	}
	if !endedNotice(r.out) {
		t.Fatal("the controller was never told the session ended")
	}
	r.send(t, "request-reconcile-01", &terminalproto.Reconcile{})
	rec := r.reply(t, "request-reconcile-01").(terminalproto.Reconcile)
	n := 0
	for _, s := range rec.Sessions {
		if s.ID == swapSession {
			n++
			if s.State.ReservesQuota() {
				t.Fatalf("reconcile reports %s", s.State)
			}
		}
	}
	if n > 1 {
		t.Fatalf("reconcile lists the id %d times", n)
	}
}

func TestSwapDoubleFailureReportsEndTerminalToChat(t *testing.T) {
	r := newSwapRig(t)
	r.openTerminal(t)
	r.shells.createErr = terminalhost.ErrNotRunning
	r.mu.Lock()
	r.startErrs = []error{errors.New("chat failed")}
	r.mu.Unlock()
	body := toChat(r.home)
	r.send(t, "request-swap-000001", &body)
	if _, ok := r.reply(t, "request-swap-000001").(terminalproto.Result); !ok {
		t.Fatal("swap did not fail")
	}
	if !endedNotice(r.out) {
		t.Fatal("the controller was never told the session ended")
	}
	r.send(t, "request-reconcile-01", &terminalproto.Reconcile{})
	rec := r.reply(t, "request-reconcile-01").(terminalproto.Reconcile)
	n := 0
	for _, s := range rec.Sessions {
		if s.ID == swapSession {
			n++
			if s.State.ReservesQuota() {
				t.Fatalf("reconcile reports %s", s.State)
			}
		}
	}
	if n > 1 {
		t.Fatalf("reconcile lists the id %d times", n)
	}
}

// realTmux starts a private tmux server whose opencode is script, and returns
// the manager options and home for it.
func realTmux(t *testing.T, script string) (terminalhost.ManagerOptions, string) {
	t.Helper()
	tmux, err := terminalhost.FindTmux(t.Context())
	if err != nil {
		t.Skip(err)
	}
	base, err := os.MkdirTemp("/tmp", "reet-swap-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	if base, err = filepath.EvalSymlinks(base); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(base, "home")
	bin := filepath.Join(base, "bin")
	for _, dir := range []string{home, bin} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(bin, "opencode"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	config := filepath.Join(base, "agent.json")
	if err := os.WriteFile(config, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	enrollment := "enrollment-0123456789"
	layout, err := terminalhost.NewEnrollmentLayout(config, enrollment)
	if err != nil {
		t.Fatal(err)
	}
	if err := terminalhost.Ensure(layout); err != nil {
		t.Fatal(err)
	}
	server := exec.Command(tmux.Path, "-D", "-S", layout.Socket, "-f", layout.Config)
	server.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = exec.Command(tmux.Path, "-S", layout.Socket, "kill-server").Run()
		_ = server.Process.Kill()
		_ = server.Wait()
	})
	deadline := time.Now().Add(3 * time.Second)
	for !terminalhost.SocketActive(layout) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	opts := terminalhost.ManagerOptions{EnrollmentID: enrollment, Layout: layout, Home: home, Consent: func() error { return nil }}
	return opts, home
}

// A control connection closed while a chat-to-terminal swap is closing the
// chat still lets the swap start its terminal on the real manager.
func TestSwapSurvivesConnectionCleanup(t *testing.T) {
	opts, home := realTmux(t, "#!/bin/sh\nexec sleep 600\n")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	r := &swapRig{out: &sent{}, home: home}
	r.sup = newChatSupervisor(ctx, opts)
	r.sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	r.sup.start = r.start
	headers := http.Header{terminalproto.FeatureHeader: {swapFeatures()}}
	connCtx, drop := context.WithCancel(ctx)
	handle, cleanup := FactoryWithChats(opts, &ChatSupervisor{s: r.sup})(connCtx, http.DefaultClient, headers, 1, r.out.send)
	r.handle = handle
	r.openChat(t)
	proc := r.procs[0]
	proc.gate, proc.entered = make(chan struct{}), make(chan struct{}, 1)
	body := toTerminal(home)
	body.Model = ""
	r.send(t, "request-swap-000001", &body)
	<-proc.entered
	// The connection is lost: its context ends and its cleanup runs while the
	// swap is still closing the chat.
	drop()
	cleaned := make(chan struct{})
	go func() { cleanup(); close(cleaned) }()
	time.Sleep(50 * time.Millisecond)
	close(proc.gate)
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
		if s.ID == swapSession && s.State == terminalproto.Running {
			terminals++
		}
	}
	chat := 0
	if s, ok := (&chatHost{sup: r.sup, link: &chatLink{}}).snapshot(swapSession); ok && s.State.ReservesQuota() {
		chat++
	}
	if terminals+chat != 1 {
		t.Fatalf("after the swap: %d running terminals, %d live chats (%+v)", terminals, chat, sessions)
	}
	if terminals != 1 {
		t.Fatalf("the swap rolled back instead of finishing: %+v", sessions)
	}
}

// retiredOnce asserts the controller was told the id ended and that a
// reconcile lists it at most once, never as live.
func retiredOnce(t *testing.T, r *swapRig) {
	t.Helper()
	if !endedNotice(r.out) {
		t.Fatal("the controller was never told the session ended")
	}
	r.send(t, "request-reconcile-01", &terminalproto.Reconcile{})
	rec := r.reply(t, "request-reconcile-01").(terminalproto.Reconcile)
	n := 0
	for _, s := range rec.Sessions {
		if s.ID == swapSession {
			n++
			if s.State.ReservesQuota() {
				t.Fatalf("reconcile reports %s", s.State)
			}
		}
	}
	if n > 1 {
		t.Fatalf("reconcile lists the id %d times", n)
	}
}

// A chat whose process group cannot be confirmed gone is not replaced, and
// the row is retired rather than left running with nothing behind it.
func TestSwapChatStillRunningRetiresRow(t *testing.T) {
	r := newSwapRig(t)
	r.openChat(t)
	r.procs[0].lingers = true
	body := toTerminal(r.home)
	r.send(t, "request-swap-000001", &body)
	if _, ok := r.reply(t, "request-swap-000001").(terminalproto.Result); !ok {
		t.Fatal("swap did not fail")
	}
	if n := r.shells.createCount(); n != 0 {
		t.Fatalf("a terminal was started beside the old chat: %d", n)
	}
	retiredOnce(t, r)
}

// tmuxSwapRig is a control connection on a real tmux manager whose opencode
// is script, with an OpenCode TUI running under swapSession.
func tmuxSwapRig(t *testing.T, script string) (*swapRig, terminalhost.ManagerOptions) {
	t.Helper()
	opts, home := realTmux(t, script)
	r := &swapRig{out: &sent{}, home: home}
	r.sup = newChatSupervisor(t.Context(), opts)
	r.sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	r.sup.start = r.start
	headers := http.Header{terminalproto.FeatureHeader: {swapFeatures()}}
	handle, cleanup := FactoryWithChats(opts, &ChatSupervisor{s: r.sup})(t.Context(), http.DefaultClient, headers, 1, r.out.send)
	t.Cleanup(cleanup)
	r.handle = handle
	r.send(t, "request-tui-0000001", &terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Dir: home, LaunchSession: swapACP})
	if changed, ok := r.reply(t, "request-tui-0000001").(terminalproto.SessionChanged); !ok || changed.State != terminalproto.Running {
		t.Fatalf("terminal did not start: %#v", changed)
	}
	return r, opts
}

// tuiGroups returns the process groups the session's panes run.
func tuiGroups(t *testing.T, opts terminalhost.ManagerOptions) []int {
	t.Helper()
	m, err := terminalhost.NewManager(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	pids, err := m.PaneProcesses(t.Context(), swapSession)
	if err != nil || len(pids) == 0 {
		t.Fatalf("pane processes %v %v", pids, err)
	}
	return pids
}

// A switch to the chat waits for the OpenCode TUI to exit before the chat
// opens the same OpenCode store, and then proceeds. This TUI takes a moment
// to exit after its hangup.
func TestSwapToChatWaitsForTerminalExit(t *testing.T) {
	r, opts := tmuxSwapRig(t, "#!/bin/sh\ntrap 'sleep 0.5; exit 0' HUP\nsleep 600 &\nwait\n")
	groups := tuiGroups(t, opts)
	var alive []bool
	start := r.sup.start
	r.sup.start = func(ctx context.Context, o acpagent.Options) (chatProcess, error) {
		for _, g := range groups {
			alive = append(alive, acpagent.GroupAlive(g))
		}
		return start(ctx, o)
	}
	body := toChat(r.home)
	r.send(t, "request-swap-000001", &body)
	if changed, ok := r.reply(t, "request-swap-000001").(terminalproto.SessionChanged); !ok || changed.State != terminalproto.Running {
		t.Fatalf("swap reply %#v", changed)
	}
	if len(alive) != len(groups) {
		t.Fatalf("chat starts saw %d groups", len(alive))
	}
	for i, a := range alive {
		if a {
			t.Fatalf("the chat started while the TUI's group %d was alive", groups[i])
		}
	}
}

// An OpenCode TUI that outlives its tmux session is not joined by a chat on
// the same store: the switch is refused and the row retired.
func TestSwapToChatTerminalStillRunningRetiresRow(t *testing.T) {
	previous := swapShellExitTimeout
	swapShellExitTimeout = 300 * time.Millisecond
	t.Cleanup(func() { swapShellExitTimeout = previous })
	r, opts := tmuxSwapRig(t, "#!/bin/sh\ntrap '' HUP TERM\nexec sleep 600\n")
	groups := tuiGroups(t, opts)
	t.Cleanup(func() {
		for _, g := range groups {
			_ = syscall.Kill(-g, syscall.SIGKILL)
		}
	})
	body := toChat(r.home)
	r.send(t, "request-swap-000001", &body)
	if _, ok := r.reply(t, "request-swap-000001").(terminalproto.Result); !ok {
		t.Fatal("swap did not fail")
	}
	if n := r.chatStarts(); n != 0 {
		t.Fatalf("a chat was started beside the TUI: %d", n)
	}
	if !acpagent.GroupAlive(groups[0]) {
		t.Fatal("fixture: the lingering TUI exited")
	}
	retiredOnce(t, r)
}
