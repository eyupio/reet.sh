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
	// What push and Gaffer read is the stored row: no finished turn to tell yet.
	if row, _ := b.st.TerminalSessionFor(uid, s.ID); row.ChatStatus != "background" || row.CompletedAt != 0 {
		t.Fatalf("stored %q completed %d while the work runs", row.ChatStatus, row.CompletedAt)
	}
	// A held tool's own output is not the work ending.
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "tool", ToolCallID: "call_bg", Title: "npm test", ToolKind: "execute", Status: "in_progress", Text: "still going"})
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "idle", Background: 1})
	if r := look(); r.ChatStatus != "background" {
		t.Fatalf("one task left read as %q", r.ChatStatus)
	}
	before := time.Now().UnixMilli()
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "idle"})
	if row, _ := b.st.TerminalSessionFor(uid, s.ID); row.ChatStatus != "finished" || row.CompletedAt < before {
		t.Fatalf("stored %q completed %d once the work ended", row.ChatStatus, row.CompletedAt)
	}
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

func TestRestoredBackgroundHoldEndsAtTheLimitAndIsStored(t *testing.T) {
	b, _, uid, agent := chatFixture(t)
	for name, c := range map[string]struct {
		age  time.Duration
		want string
	}{
		"recent": {time.Minute, "background"},
		"stale":  {terminalproto.BackgroundHoldLimit + time.Minute, "finished"},
	} {
		s := chatSession(t, b, uid, agent)
		at := time.Now().Add(-c.age).UnixMilli()
		if err := b.st.SetTerminalChatAttentionFor(uid, s.ID, at, 0, "background", 0); err != nil {
			t.Fatal(err)
		}
		b.closeChat(uid, s.ID)
		row, err := b.st.TerminalSessionFor(uid, s.ID)
		if err != nil {
			t.Fatal(err)
		}
		b.ApplyChatAttention(uid, &row)
		want := at + terminalproto.BackgroundHoldLimit.Milliseconds()
		waitStored(t, func() bool {
			row, _ := b.st.TerminalSessionFor(uid, s.ID)
			return row.ChatStatus == c.want && (c.want != "finished" || row.CompletedAt == want)
		}, name)
	}
}

func waitStored(t *testing.T, ok func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("%s: the stored chat status never came", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestALiveBackgroundHoldEndsAtTheBrokersOwnLimit(t *testing.T) {
	old := backgroundHold
	backgroundHold = 50 * time.Millisecond
	t.Cleanup(func() { backgroundHold = old })
	b, p, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "working"})
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "idle", Background: 1})
	// No further word from the machine.
	waitStored(t, func() bool {
		row, _ := b.st.TerminalSessionFor(uid, s.ID)
		return row.ChatStatus == "finished" && row.CompletedAt > 0
	}, "live hold")
}

func TestAFailedOrStoppedTurnIsNotReopenedByAHeldCount(t *testing.T) {
	for _, end := range []string{"error", "cancelled"} {
		b, p, uid, agent := chatFixture(t)
		s := chatSession(t, b, uid, agent)
		l := b.chatLog(uid, s.ID)
		state := func() (string, int64) { l.mu.Lock(); defer l.mu.Unlock(); return l.lastState, l.completedAt }
		receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "working"})
		receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "idle", Background: 2})
		receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "user", Text: "next"})
		receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: end, Detail: "provider failed"})
		before, completed := state()
		receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "idle", Background: 1})
		receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "idle"})
		if got, at := state(); got != before || at != completed {
			t.Fatalf("%s: a held count turned %q into %q", end, before, got)
		}
	}
}
