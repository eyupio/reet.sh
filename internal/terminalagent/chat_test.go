//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/opencodeacp"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type fakeChat struct {
	mu        sync.Mutex
	onEvent   func(opencodeacp.Event)
	done      chan struct{}
	allow     []bool
	prompted  string
	busy      bool
	cancelled bool
}

func (f *fakeChat) Prompt(ctx context.Context, text string) error {
	f.mu.Lock()
	f.prompted = text
	f.busy = true
	f.mu.Unlock()
	<-ctx.Done()
	f.mu.Lock()
	f.busy = false
	f.mu.Unlock()
	return ctx.Err()
}
func (f *fakeChat) Cancel() error {
	f.mu.Lock()
	f.cancelled = true
	f.mu.Unlock()
	return nil
}
func (f *fakeChat) Allow(_ string, allow bool) error {
	f.mu.Lock()
	f.allow = append(f.allow, allow)
	f.mu.Unlock()
	return nil
}
func (f *fakeChat) SessionID() string     { return "ses_fake000000000000000000000001" }
func (f *fakeChat) Working() bool         { f.mu.Lock(); defer f.mu.Unlock(); return f.busy }
func (f *fakeChat) Close()                { f.done <- struct{}{} }
func (f *fakeChat) Done() <-chan struct{} { return f.done }

type sent struct {
	mu   sync.Mutex
	msgs []terminalproto.Message
}

func (s *sent) send(_ context.Context, m terminalproto.Message) error {
	s.mu.Lock()
	s.msgs = append(s.msgs, m)
	s.mu.Unlock()
	return nil
}
func (s *sent) hasSession(state terminalproto.SessionState) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.msgs {
		if c, ok := m.Body.(*terminalproto.SessionChanged); ok && c.State == state {
			return true
		}
	}
	return false
}

func testChatHost(t *testing.T) (*chatHost, *fakeChat, *sent) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	home := t.TempDir()
	dir := t.TempDir()
	out := &sent{}
	proc := &fakeChat{done: make(chan struct{}, 1)}
	h := newChatHost(ctx, terminalhost.ManagerOptions{Home: home}, 1, out.send, true)
	h.lookPath = func(string) (string, error) { return "/bin/true", nil }
	h.start = func(_ context.Context, opts opencodeacp.Options) (chatProcess, error) {
		proc.onEvent = opts.OnEvent
		return proc, nil
	}
	_ = dir
	return h, proc, out
}

func TestChatHostYoloAllowsOnce(t *testing.T) {
	h, proc, out := testChatHost(t)
	h.opts.YoloConsent = func() error { return nil }
	if _, err := h.open("request-0123456789", "session-0123456789", terminalproto.Create{
		Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Yolo: true, Dir: h.opts.Home,
	}); err != nil {
		t.Fatal(err)
	}
	proc.onEvent(opencodeacp.Event{Kind: "permission", PermissionID: "permission-0123456789", Title: "run tests"})
	proc.mu.Lock()
	defer proc.mu.Unlock()
	if len(proc.allow) != 1 || !proc.allow[0] {
		t.Fatalf("allow: %v", proc.allow)
	}
	out.mu.Lock()
	defer out.mu.Unlock()
	var permission, resolved bool
	for _, m := range out.msgs {
		if e, ok := m.Body.(*terminalproto.ACPEvent); ok {
			if e.Kind == "permission" {
				permission = true
			}
			if e.Kind == "permission_resolved" && e.Allow != nil && *e.Allow {
				resolved = true
			}
		}
	}
	if !permission || !resolved {
		t.Fatalf("events: permission=%v resolved=%v", permission, resolved)
	}
}

func TestChatHostYoloRefusedWithoutMachineConsent(t *testing.T) {
	h, _, _ := testChatHost(t)
	if _, err := h.open("request-0123456789", "session-0123456789", terminalproto.Create{
		Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Yolo: true, Dir: h.opts.Home,
	}); !errors.Is(err, terminalhost.ErrYoloRefused) {
		t.Fatalf("refused: %v", err)
	}
	h.opts.YoloConsent = func() error { return errors.New("off") }
	if _, err := h.open("request-9876543210", "session-9876543210", terminalproto.Create{
		Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Yolo: true, Dir: h.opts.Home,
	}); !errors.Is(err, terminalhost.ErrYoloRefused) {
		t.Fatalf("refused off: %v", err)
	}
}

func TestChatHostCreatePromptPermissionEnd(t *testing.T) {
	h, proc, out := testChatHost(t)
	changed, err := h.open("request-0123456789", "session-0123456789", terminalproto.Create{
		Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: h.opts.Home,
	})
	if err != nil || changed.State != terminalproto.Running {
		t.Fatalf("open: %+v %v", changed, err)
	}
	if snap, ok := h.snapshot("session-0123456789"); !ok || snap.State != terminalproto.Running {
		t.Fatalf("snapshot: %+v %v", snap, ok)
	}
	if err := h.prompt("session-0123456789", "hello"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		proc.mu.Lock()
		prompted := proc.prompted
		proc.mu.Unlock()
		if prompted == "hello" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("prompted %q", prompted)
		}
		time.Sleep(time.Millisecond)
	}
	if err := h.prompt("session-0123456789", "again"); !errors.Is(err, terminalhost.ErrOperationConflict) {
		t.Fatalf("busy prompt: %v", err)
	}
	proc.onEvent(opencodeacp.Event{Kind: "permission", PermissionID: "permission-0123456789", Title: "write", ToolKind: "edit", Path: "/home/u/a", Diff: "@@ -0,0 +1 @@\n+x"})
	if err := h.permission("session-0123456789", "permission-0123456789", true); err != nil {
		t.Fatal(err)
	}
	proc.mu.Lock()
	if len(proc.allow) != 1 || !proc.allow[0] {
		t.Fatalf("allow %v", proc.allow)
	}
	proc.mu.Unlock()
	if _, err := h.end("session-0123456789"); err != nil {
		t.Fatal(err)
	}
	if snap, ok := h.snapshot("session-0123456789"); !ok || snap.State != terminalproto.Exited {
		t.Fatalf("ended snapshot %+v %v", snap, ok)
	}
	if !out.hasSession(terminalproto.Exited) {
		out.mu.Lock()
		t.Fatalf("end was not reported: %+v", out.msgs)
	}
}

func TestChatHostTrustAndVersionErrors(t *testing.T) {
	for name, err := range map[string]error{
		"trust":   &opencodeacp.TrustError{Plugins: []string{"plugin.ts"}},
		"version": opencodeacp.ErrVersion,
	} {
		r := chatResult(err)
		if r.Code == "" {
			t.Fatalf("%s: no code", name)
		}
	}
}

func TestChatHostEndKeepsItsReceipt(t *testing.T) {
	h, _, _ := testChatHost(t)
	if _, err := h.open("request-0123456789", "session-0123456789", terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: h.opts.Home}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.end("session-0123456789"); err != nil {
		t.Fatal(err)
	}
	changed, err := h.open("request-0123456789", "session-0123456789", terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: h.opts.Home})
	if err != nil || changed.State != terminalproto.Exited {
		t.Fatalf("replay: %+v %v", changed, err)
	}
}

func TestWireEventTrimsAndValidates(t *testing.T) {
	event := opencodeacp.Event{Kind: "permission", PermissionID: "permission-0123456789", Title: "write", Diff: string(make([]byte, 12000))}
	wire, ok := wireEvent(event)
	if !ok || !wire.Truncated || wire.Kind != "permission" || wire.Diff != "" {
		t.Fatalf("oversize card: %+v %v", wire, ok)
	}
	event.Diff = "x"
	wire, ok = wireEvent(event)
	if !ok || wire.Kind != "permission" || wire.PermissionID != "permission-0123456789" {
		t.Fatalf("wire %+v %v", wire, ok)
	}
	_ = time.Second
}

func TestReconcileSnapshotKeepsLiveChats(t *testing.T) {
	shells := make([]terminalproto.SessionSnapshot, 32)
	for i := range shells {
		shells[i] = terminalproto.SessionSnapshot{ID: "shell-" + string(rune('a'+i)), Title: "Shell", State: terminalproto.Exited}
	}
	chat := terminalproto.SessionSnapshot{ID: "chat-0123456789", Title: "Chat", State: terminalproto.Running}
	got := reconcileSnapshot(shells, []terminalproto.SessionSnapshot{chat})
	if len(got.Sessions) != 32 || got.Sessions[0].ID != chat.ID {
		t.Fatalf("live chat was omitted: %+v", got.Sessions[:2])
	}
}

func TestChatHostOmitsHistoryFieldsForAnOldController(t *testing.T) {
	h, _, out := testChatHost(t)
	h.history = false
	changed, err := h.open("request-0123456789", "session-0123456789", terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: h.opts.Home})
	if err != nil {
		t.Fatal(err)
	}
	if changed.ACPSessionID != "" {
		t.Fatal("create reported an ACP id without acp-history-v1")
	}
	if snap, _ := h.snapshot("session-0123456789"); snap.ACPSessionID != "" {
		t.Fatal("snapshot reported an ACP id")
	}
	if _, err := h.end("session-0123456789"); err != nil {
		t.Fatal(err)
	}
	out.mu.Lock()
	defer out.mu.Unlock()
	for _, m := range out.msgs {
		if s, ok := m.Body.(*terminalproto.SessionChanged); ok && s.ACPSessionID != "" {
			t.Fatal("session change reported an ACP id")
		}
	}
}

func TestChatHostReportsCreatingInSnapshots(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	out := &sent{}
	h := newChatHost(ctx, terminalhost.ManagerOptions{Home: t.TempDir()}, 1, out.send, true)
	h.lookPath = func(string) (string, error) { return "/bin/true", nil }
	gate := make(chan struct{})
	proc := &fakeChat{done: make(chan struct{}, 1)}
	h.start = func(_ context.Context, _ opencodeacp.Options) (chatProcess, error) {
		<-gate
		return proc, nil
	}
	done := make(chan error, 1)
	go func() {
		_, err := h.open("request-0123456789", "session-0123456789", terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: h.opts.Home})
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		if snap, ok := h.snapshot("session-0123456789"); ok {
			if snap.State != terminalproto.Creating {
				t.Fatalf("state %q", snap.State)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("creating session was not in the snapshot")
		}
		time.Sleep(time.Millisecond)
	}
	close(gate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	h.close()
}
