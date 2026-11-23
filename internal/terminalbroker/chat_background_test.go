package terminalbroker

import (
	"strings"
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
		"stale":  {terminalproto.BackgroundHoldLimit + time.Minute, "unwatched"},
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
			return row.ChatStatus == c.want && (c.want != "unwatched" || row.CompletedAt == want)
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
		return row.ChatStatus == "unwatched" && row.CompletedAt > 0
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

func TestAHoldTheMachineGaveUpOnIsStoppedWaitingNotFinished(t *testing.T) {
	b, p, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "working"})
	before := time.Now().UnixMilli()
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "idle", Background: 2})
	row, _ := b.st.TerminalSessionFor(uid, s.ID)
	if row.HeldSince < before {
		t.Fatalf("held since %d, want the hold's start", row.HeldSince)
	}
	// One task was given up on; another still runs, then ends.
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "idle", Background: 1, Code: terminalproto.ACPCodeUnwatched})
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "idle"})
	row, _ = b.st.TerminalSessionFor(uid, s.ID)
	if row.ChatStatus != "unwatched" || row.CompletedAt == 0 || row.HeldSince != 0 {
		t.Fatalf("stored %q completed %d held %d, want stopped waiting", row.ChatStatus, row.CompletedAt, row.HeldSince)
	}
	b.ApplyChatAttention(uid, &row)
	if row.Attention != "finished" || row.ChatStatus != "unwatched" {
		t.Fatalf("attention %q chat %q", row.Attention, row.ChatStatus)
	}
	// A new message starts afresh: its own clean end is finished.
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "user", Text: "carry on"})
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "idle"})
	if row, _ = b.st.TerminalSessionFor(uid, s.ID); row.ChatStatus != "finished" {
		t.Fatalf("the next turn read %q", row.ChatStatus)
	}
}

func TestTheBrokersOwnLimitStopsWaitingAndSaysSo(t *testing.T) {
	old := backgroundHold
	backgroundHold = 50 * time.Millisecond
	t.Cleanup(func() { backgroundHold = old })
	b, p, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "working"})
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "idle", Background: 1})
	waitStored(t, func() bool {
		row, _ := b.st.TerminalSessionFor(uid, s.ID)
		return row.ChatStatus == "unwatched"
	}, "broker limit")
	snapshot, _, _, cancel := b.SubscribeChat(uid, s.ID)
	cancel()
	for _, event := range snapshot {
		if event.Kind == "notice" && strings.Contains(event.Text, "reet stopped waiting; it may still be running on") {
			return
		}
	}
	t.Fatalf("no transcript line for the controller's own limit: %+v", snapshot)
}

func TestARestoredHoldKeepsItsStartAndAStoppedWaitStays(t *testing.T) {
	b, _, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	start := time.Now().Add(-5 * time.Minute).UnixMilli()
	if err := b.st.SetTerminalChatStateFor(uid, s.ID, time.Now().UnixMilli(), 0, "background", 0, start); err != nil {
		t.Fatal(err)
	}
	b.closeChat(uid, s.ID)
	row, _ := b.st.TerminalSessionFor(uid, s.ID)
	b.ApplyChatAttention(uid, &row)
	if row.HeldSince != start || row.ChatStatus != "background" {
		t.Fatalf("restored hold since %d (%q), want %d", row.HeldSince, row.ChatStatus, start)
	}
	other := chatSession(t, b, uid, agent)
	if err := b.st.SetTerminalChatStateFor(uid, other.ID, start, 0, "unwatched", start, 0); err != nil {
		t.Fatal(err)
	}
	b.closeChat(uid, other.ID)
	row, _ = b.st.TerminalSessionFor(uid, other.ID)
	b.ApplyChatAttention(uid, &row)
	if row.ChatStatus != "unwatched" || row.Attention != "finished" {
		t.Fatalf("restored stopped wait read %q %q", row.ChatStatus, row.Attention)
	}
}
