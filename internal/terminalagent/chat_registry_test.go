//go:build linux || darwin

package terminalagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func writeChatRegistry(t *testing.T, dir string, rec chatRecord) string {
	t.Helper()
	path := filepath.Join(dir, "structured-chats.json")
	data, err := json.Marshal(chatRegistryFile{Version: 1, Chats: []chatRecord{rec}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSupervisorResumesPersistedChat(t *testing.T) {
	home := t.TempDir()
	layoutDir := t.TempDir()
	work := filepath.Join(home, "work")
	if err := os.MkdirAll(work, 0700); err != nil {
		t.Fatal(err)
	}
	const id = "session-0123456789"
	const acpID = "ses_resume000000000000000000001"
	writeChatRegistry(t, layoutDir, chatRecord{
		ID: id, Title: "Recovered", Tool: "opencode", Dir: work, Model: "test/model",
		Trust: true, ACPSID: acpID, UpdatedAt: time.Now().UnixMilli(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	proc := &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}
	resumed := make(chan acpagent.Options, 1)
	sup := newChatSupervisor(ctx, terminalhost.ManagerOptions{Home: home, Layout: terminalhost.Layout{Dir: layoutDir}})
	sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
		resumed <- opts
		return proc, nil
	}

	var opts acpagent.Options
	select {
	case opts = <-resumed:
	case <-time.After(5 * time.Second):
		t.Fatal("persisted chat was not resumed")
	}
	if opts.Resume != acpID {
		t.Fatalf("resume id %q", opts.Resume)
	}
	if opts.Dir != work {
		t.Fatalf("resume dir %q", opts.Dir)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		sup.mu.Lock()
		cs := sup.sessions[id]
		state := terminalproto.Creating
		if cs != nil {
			state = cs.state
		}
		sup.mu.Unlock()
		if state == terminalproto.Running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("resumed session state %q", state)
		}
		time.Sleep(10 * time.Millisecond)
	}

	h := sup.attach(ctx, 1, (&sent{}).send, true, true)
	if _, err := h.end(id); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(layoutDir, "structured-chats.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file chatRegistryFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Chats) != 0 {
		t.Fatalf("ended chat still recorded: %+v", file.Chats)
	}
}

func TestSupervisorRemembersAndResumesAcrossProcesses(t *testing.T) {
	home := t.TempDir()
	layoutDir := t.TempDir()
	work := filepath.Join(home, "work")
	if err := os.MkdirAll(work, 0700); err != nil {
		t.Fatal(err)
	}
	layout := terminalhost.Layout{Dir: layoutDir}

	ctx1, cancel1 := context.WithCancel(context.Background())
	first := &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}
	sup1 := newChatSupervisor(ctx1, terminalhost.ManagerOptions{Home: home, Layout: layout})
	sup1.lookPath = func(string) (string, error) { return "/bin/true", nil }
	sup1.start = func(_ context.Context, _ acpagent.Options) (chatProcess, error) { return first, nil }
	h1 := sup1.attach(ctx1, 1, (&sent{}).send, true, true)
	const id = "session-0123456789"
	if _, err := h1.open("request-0123456789", id, terminalproto.Create{
		Title: "Remembered", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: work,
	}); err != nil {
		t.Fatal(err)
	}
	cancel1()

	ctx2, cancel2 := context.WithCancel(context.Background())
	t.Cleanup(cancel2)
	second := &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}
	resumed := make(chan acpagent.Options, 1)
	sup2 := newChatSupervisor(ctx2, terminalhost.ManagerOptions{Home: home, Layout: layout})
	sup2.lookPath = func(string) (string, error) { return "/bin/true", nil }
	sup2.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
		resumed <- opts
		return second, nil
	}
	select {
	case opts := <-resumed:
		if opts.Resume != first.SessionID() {
			t.Fatalf("resume id %q, want %q", opts.Resume, first.SessionID())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("remembered chat was not resumed")
	}
}
