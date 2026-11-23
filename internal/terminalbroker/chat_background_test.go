package terminalbroker

import (
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestTurnEndedWithBackgroundWorkReadsAsWorkingUntilItEnds(t *testing.T) {
	b, p, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	look := func() store.TerminalSession {
		t.Helper()
		record, err := b.st.TerminalSessionFor(uid, s.ID)
		if err != nil {
			t.Fatal(err)
		}
		b.ApplyChatAttention(uid, &record)
		return record
	}
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "working"})
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "idle", Background: 2})
	if r := look(); r.Attention != "working" || r.ChatStatus != "background" || r.CompletedAt != 0 {
		t.Fatalf("held turn read as attention %q, chat %q, completed %d", r.Attention, r.ChatStatus, r.CompletedAt)
	}
	// A held tool's own output is not the work ending.
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "tool", ToolCallID: "call_bg", Title: "npm test", ToolKind: "execute", Status: "in_progress", Text: "still going"})
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "idle", Background: 1})
	if r := look(); r.ChatStatus != "background" {
		t.Fatalf("one task left read as %q", r.ChatStatus)
	}
	before := time.Now().UnixMilli()
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "idle"})
	r := look()
	if r.Attention != "finished" || r.ChatStatus != "finished" || r.CompletedAt < before {
		t.Fatalf("ended work read as attention %q, chat %q, completed %d (want >= %d)", r.Attention, r.ChatStatus, r.CompletedAt, before)
	}
}

func TestBackgroundHoldSurvivesANewTurnAndEndsWithTheSession(t *testing.T) {
	b, p, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	l := b.chatLog(uid, s.ID)
	state := func() string { l.mu.Lock(); defer l.mu.Unlock(); return l.lastState }
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "working"})
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "idle", Background: 1})
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "user", Text: "and the docs"})
	if got := state(); got != "working" {
		t.Fatalf("a new message left the chat %q", got)
	}
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "idle", Background: 1})
	if got := state(); got != "background" {
		t.Fatalf("the next turn's end read %q with work still held", got)
	}
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "ended"})
	if got := state(); got != "finished" {
		t.Fatalf("an ended session read %q", got)
	}
}

func TestRestoredBackgroundHoldEndsAtTheLimit(t *testing.T) {
	b, _, uid, _ := chatFixture(t)
	for name, c := range map[string]struct {
		age  time.Duration
		want string
	}{
		"recent": {time.Minute, "background"},
		"stale":  {terminalproto.BackgroundHoldLimit + time.Minute, "finished"},
	} {
		at := time.Now().Add(-c.age).UnixMilli()
		id := "restore-" + name + "-0123456789"
		l := b.restoreChat(uid, store.TerminalSession{ID: id, Structured: true, ChatStatus: "background", ActivityAt: at})
		l.mu.Lock()
		got, completed, running := l.lastState, l.completedAt, l.running
		l.mu.Unlock()
		if got != c.want || running {
			t.Fatalf("%s: restored %q (running %v), want %q", name, got, running, c.want)
		}
		if c.want == "finished" && completed != at {
			t.Fatalf("%s: completed %d, want the last activity %d", name, completed, at)
		}
	}
}
