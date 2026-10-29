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
	h := newChatHost(ctx, terminalhost.ManagerOptions{Home: home}, 1, out.send)
	h.lookPath = func(string) (string, error) { return "/bin/true", nil }
	h.start = func(_ context.Context, opts opencodeacp.Options) (chatProcess, error) {
		proc.onEvent = opts.OnEvent
		return proc, nil
	}
	_ = dir
	return h, proc, out
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
