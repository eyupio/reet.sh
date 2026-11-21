//go:build linux || darwin

package terminalagent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// An agent that re-execs itself closes the supervisor first: the chat process
// must be gone when Close returns, and still be recorded for the next process.
func TestSupervisorCloseEndsChatsAndKeepsThemRecorded(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "work")
	if err := os.MkdirAll(work, 0700); err != nil {
		t.Fatal(err)
	}
	opts := terminalhost.ManagerOptions{Home: home, Layout: terminalhost.Layout{Dir: t.TempDir()}}
	proc := &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}
	inner := newChatSupervisor(context.Background(), opts)
	inner.lookPath = func(string) (string, error) { return "/bin/true", nil }
	inner.start = func(context.Context, acpagent.Options) (chatProcess, error) { return proc, nil }
	h := inner.attach(context.Background(), 1, (&sent{}).send, true, true)
	const id = "session-0123456789"
	if _, err := h.open("request-0123456789", id, terminalproto.Create{
		Title: "Running", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: work,
	}, true); err != nil {
		t.Fatal(err)
	}
	(&ChatSupervisor{s: inner}).Close()
	select {
	case <-proc.closed:
	default:
		t.Fatal("Close returned with the chat process still running")
	}
	records := newChatRegistry(filepath.Join(opts.Layout.Dir, "structured-chats.json")).snapshot()
	if len(records) != 1 || records[0].ID != id {
		t.Fatalf("records after Close: %+v", records)
	}
	// A nil supervisor has nothing to close.
	var none *ChatSupervisor
	none.Close()
}
