//go:build linux || darwin

package terminalagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestFailedChatRestoreKeepsConversationID(t *testing.T) {
	home, layoutDir := t.TempDir(), t.TempDir()
	const id = "session-restore0123456789"
	const acpID = "ses_original000000000000000001"
	writeChatRegistry(t, layoutDir, chatRecord{
		ID: id, Title: "Retry me", Tool: "opencode", Dir: home, Trust: true,
		ACPSID: acpID, UpdatedAt: time.Now().UnixMilli(),
	})
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	sup := newChatSupervisor(ctx, terminalhost.ManagerOptions{Home: home, Layout: terminalhost.Layout{Dir: layoutDir}})
	t.Cleanup(sup.Close)
	sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	starts := 0
	sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
		starts++
		if opts.Resume != acpID || opts.Dir != home {
			t.Fatalf("restore changed the conversation: id=%q dir=%q", opts.Resume, opts.Dir)
		}
		return nil, errors.New("temporarily unavailable")
	}
	sup.resumePersisted()
	out := &sent{}
	h := sup.attach(ctx, 1, out.send, true, true)
	snapshot, ok := h.snapshot(id)
	if !ok || snapshot.State != terminalproto.Exited || snapshot.ACPSessionID != acpID {
		t.Fatalf("failed restore discarded its history handle: %+v, present=%v", snapshot, ok)
	}
	if starts != 1 {
		t.Fatalf("restore attempts=%d, want one", starts)
	}
	records := newChatRegistry(filepath.Join(layoutDir, "structured-chats.json")).snapshot()
	if len(records) != 1 || records[0].ACPSID != acpID || records[0].Attempts != 1 {
		t.Fatalf("failed restore lost its retry record: %+v", records)
	}
	if notices := out.events("notice"); len(notices) != 1 || !strings.Contains(notices[0].Text, "Continue with history") {
		t.Fatalf("missing recovery guidance: %+v", notices)
	}
}

func TestManualChatRecoveryReplacesRestartRecord(t *testing.T) {
	home, layoutDir := t.TempDir(), t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	proc := &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}
	const oldID, newID = "session-old0123456789", "session-new0123456789"
	path := writeChatRegistry(t, layoutDir, chatRecord{
		ID: oldID, Title: "Old", Tool: "opencode", Dir: home, Trust: true,
		ACPSID: proc.SessionID(), UpdatedAt: time.Now().UnixMilli(), Attempts: 1,
	})
	sup := newChatSupervisor(ctx, terminalhost.ManagerOptions{Home: home, Layout: terminalhost.Layout{Dir: layoutDir}})
	t.Cleanup(sup.Close)
	sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	starts := 0
	sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
		starts++
		if opts.Resume != proc.SessionID() {
			t.Fatalf("recovery lost the original conversation: %q", opts.Resume)
		}
		return proc, nil
	}
	sup.sessions[oldID] = &chatSession{id: oldID, tool: "opencode", acpID: proc.SessionID(), state: terminalproto.Exited}
	h := sup.attach(ctx, 1, (&sent{}).send, true, true)
	in := terminalproto.Create{Title: "Recovered", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: home, ResumeACP: proc.SessionID()}
	if _, err := h.open("request-new0123456789", newID, in); err != nil {
		t.Fatal(err)
	}
	records := newChatRegistry(path).snapshot()
	if len(records) != 1 || records[0].ID != newID || records[0].ACPSID != proc.SessionID() {
		t.Fatalf("next restart would reopen multiple copies: %+v", records)
	}
	if _, err := h.open("request-new0123456789", newID, in); err != nil {
		t.Fatalf("retrying the same recovery operation was refused: %v", err)
	}
	if _, err := h.open("request-again0123456789", "session-again0123456789", in); !errors.Is(err, terminalhost.ErrOperationConflict) {
		t.Fatalf("duplicate recovery was not refused: %v", err)
	}
	// A restart sweep may already hold a snapshot of the superseded record.
	sup.resumeChat(chatRecord{ID: "session-stale0123456789", Title: "Old", Tool: "opencode", Dir: home, ACPSID: proc.SessionID(), Trust: true})
	if starts != 1 {
		t.Fatalf("started %d processes for one conversation", starts)
	}
	if proc.prompted != "" {
		t.Fatal("history recovery sent a prompt automatically")
	}
}

func TestChatRecoveryRefusesConversationInUse(t *testing.T) {
	for _, state := range []terminalproto.SessionState{terminalproto.Creating, terminalproto.Running, terminalproto.Exited} {
		t.Run(string(state), func(t *testing.T) {
			h, proc, _ := testChatHost(t)
			t.Cleanup(h.sup.Close)
			h.sup.sessions["session-existing0123456789"] = &chatSession{
				tool: "opencode", acpID: proc.SessionID(), state: state, stray: state == terminalproto.Exited,
			}
			starts := 0
			h.sup.start = func(context.Context, acpagent.Options) (chatProcess, error) {
				starts++
				return proc, nil
			}
			_, err := h.open("request-another0123456789", "session-another0123456789", terminalproto.Create{
				Title: "Again", Tool: "opencode", Structured: true, Dir: h.sup.opts.Home, ResumeACP: proc.SessionID(),
			})
			if !errors.Is(err, terminalhost.ErrOperationConflict) || starts != 0 {
				t.Fatalf("loaded a conversation still in use: err=%v starts=%d", err, starts)
			}
		})
	}
}

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
	sup.startResume()

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
	sup2.startResume()
	select {
	case opts := <-resumed:
		if opts.Resume != first.SessionID() {
			t.Fatalf("resume id %q, want %q", opts.Resume, first.SessionID())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("remembered chat was not resumed")
	}
}
