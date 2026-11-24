//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func lockedSupervisor(t *testing.T, home, layoutDir string, locked *atomic.Bool) *chatSupervisor {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	sup := newChatSupervisor(ctx, terminalhost.ManagerOptions{Home: home, Layout: terminalhost.Layout{Dir: layoutDir}, Locked: locked.Load, Consent: func() error { return nil }})
	t.Cleanup(sup.Close)
	sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	return sup
}

// A locked machine does not start the chats it recorded when the agent
// restarts, and keeps the records so they come back after an unlock.
func TestALockedMachineDoesNotRestoreItsChats(t *testing.T) {
	home, layoutDir := t.TempDir(), t.TempDir()
	writeChatRegistry(t, layoutDir, chatRecord{ID: "session-restore0123456789", Title: "Back", Tool: "opencode", Dir: home, Trust: true,
		ACPSID: "ses_original000000000000000001", UpdatedAt: time.Now().UnixMilli()})
	var locked atomic.Bool
	locked.Store(true)
	sup := lockedSupervisor(t, home, layoutDir, &locked)
	var starts atomic.Int32
	sup.start = func(context.Context, acpagent.Options) (chatProcess, error) {
		starts.Add(1)
		return &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}, nil
	}
	sup.resumePersisted()
	if starts.Load() != 0 {
		t.Fatalf("a locked machine started %d chats on restart", starts.Load())
	}
	if records := newChatRegistry(filepath.Join(layoutDir, "structured-chats.json")).snapshot(); len(records) != 1 {
		t.Fatalf("the lock lost the records: %+v", records)
	}
}

// A lock taken while a chat is still starting stops it: the process that
// finishes starting is closed and nothing is left running or sent.
func TestALockTakenDuringAChatStartStopsIt(t *testing.T) {
	home := t.TempDir()
	var locked atomic.Bool
	sup := lockedSupervisor(t, home, t.TempDir(), &locked)
	proc := &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}
	sup.start = func(context.Context, acpagent.Options) (chatProcess, error) {
		locked.Store(true)
		return proc, nil
	}
	h := sup.attach(t.Context(), 1, (&sent{}).send, true, true)
	_, err := h.open("request-0123456781", "session-0123456781", terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: home, Prompt: "go"}, false)
	if !errors.Is(err, terminalhost.ErrLocked) {
		t.Fatalf("open = %v", err)
	}
	select {
	case <-proc.closed:
	default:
		t.Fatal("the process that finished starting was left running")
	}
	if n := (&ChatSupervisor{s: sup}).Running(); n != 0 {
		t.Fatalf("still running: %d", n)
	}
}

// Ending every chat also stops one that is still starting.
func TestEndAllStopsAChatStillStarting(t *testing.T) {
	home := t.TempDir()
	var locked atomic.Bool
	sup := lockedSupervisor(t, home, t.TempDir(), &locked)
	inStart := make(chan struct{})
	sup.start = func(ctx context.Context, _ acpagent.Options) (chatProcess, error) {
		close(inStart)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	h := sup.attach(t.Context(), 1, (&sent{}).send, true, true)
	opened := make(chan error, 1)
	go func() {
		_, err := h.open("request-0123456781", "session-0123456781", terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: home}, false)
		opened <- err
	}()
	<-inStart
	locked.Store(true)
	if n := (&ChatSupervisor{s: sup}).EndAll(); n != 1 {
		t.Fatalf("ended: %d", n)
	}
	select {
	case err := <-opened:
		if err == nil {
			t.Fatal("the chat finished starting")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the chat was left starting")
	}
}
