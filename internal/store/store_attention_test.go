package store

import (
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestAttentionWorkingThenQuiet(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	s := Session{State: "running", ActivityAt: now.Add(-10 * time.Second).Unix()}
	state, source := AttentionState(s, now)
	if state != "working" || source != "terminal" {
		t.Fatalf("got %q/%q, want working/terminal", state, source)
	}
	state, _ = AttentionState(s, now.Add(31*time.Second))
	if state != "quiet" {
		t.Fatalf("after 31s got %q, want quiet", state)
	}
}

func TestAttentionWaitingOnlyAfterBell(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	quiet := Session{State: "running", ActivityAt: now.Add(-45 * time.Second).Unix()}
	if state, _ := AttentionState(quiet, now); state != "quiet" {
		t.Fatalf("quiet without bell got %q, want quiet", state)
	}
	belled := Session{
		State:      "running",
		ActivityAt: now.Add(-5 * time.Second).Unix(),
		BellAt:     now.Add(-2 * time.Second).Unix(),
	}
	if state, source := AttentionState(belled, now); state != "waiting" || source != "terminal" {
		t.Fatalf("after bell got %q/%q, want waiting/terminal", state, source)
	}
	belled.SeenAt = now.Add(-1 * time.Second).Unix()
	if state, _ := AttentionState(belled, now); state != "working" {
		t.Fatalf("after seeing bell got %q, want working", state)
	}
}

func TestAttentionExitStatus(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	zero, two := 0, 2
	if state, source := AttentionState(Session{State: "exited", ExitCode: &zero}, now); state != "finished" || source != "terminal" {
		t.Fatalf("exit 0 got %q/%q, want finished/terminal", state, source)
	}
	if state, source := AttentionState(Session{State: "exited", ExitCode: &two}, now); state != "failed" || source != "terminal" {
		t.Fatalf("exit 2 got %q/%q, want failed/terminal", state, source)
	}
	if state, source := AttentionState(Session{State: "lost", AttentionObserved: true}, now); state != "failed" || source != "terminal" {
		t.Fatalf("lost got %q/%q, want failed/terminal", state, source)
	}
}

func TestSeenIsPerPerson(t *testing.T) {
	s, uid1, a, c := terminalFixture(t)
	u2, err := s.CreateUser("second@example.com", "Second", "password1234", "member")
	if err != nil {
		t.Fatal(err)
	}
	sess := createTerminal(t, s, uid1, a, c)
	if err := s.ObserveTerminalStateFor(uid1, a.ID, sess.ID, c.Generation, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	bellAt := time.Now().Add(-5 * time.Second).UnixMilli()
	if err := s.ObserveTerminalAttentionFor(uid1, a.ID, sess.ID, c.Generation, bellAt, bellAt); err != nil {
		t.Fatal(err)
	}
	before, err := s.TerminalSessionFor(uid1, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Attention != "waiting" || !before.ChangedSinceSeen {
		t.Fatalf("before seen got attention=%q changedSinceSeen=%v", before.Attention, before.ChangedSinceSeen)
	}
	seenAt := time.Now().UnixMilli()
	if err := s.MarkSessionSeenFor(uid1, sess.ID, seenAt); err != nil {
		t.Fatal(err)
	}
	if got1, err := s.SessionSeenFor(uid1, sess.ID); err != nil || got1 != seenAt {
		t.Fatalf("uid1 seenAt = %d, err = %v, want %d", got1, err, seenAt)
	}
	if got2, err := s.SessionSeenFor(u2.ID, sess.ID); err != nil || got2 != 0 {
		t.Fatalf("uid2 seenAt = %d, err = %v, want 0", got2, err)
	}
	after, err := s.TerminalSessionFor(uid1, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Attention == "waiting" || after.ChangedSinceSeen {
		t.Fatalf("after seen got attention=%q changedSinceSeen=%v", after.Attention, after.ChangedSinceSeen)
	}
}

func TestOldPeerSessionsHaveNoAttention(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	sess := createTerminal(t, s, uid, a, c)
	zero := 0
	if err := s.ObserveTerminalStateFor(uid, a.ID, sess.ID, c.Generation, terminalproto.Exited, &zero); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.TerminalSessionFor(uid, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Attention != "" || loaded.AttentionSource != "" || loaded.ChangedSinceSeen {
		t.Fatalf("old peer session got attention=%q source=%q changedSinceSeen=%v", loaded.Attention, loaded.AttentionSource, loaded.ChangedSinceSeen)
	}
	if state, source := AttentionState(loaded, time.Now()); state != "" || source != "" {
		t.Fatalf("AttentionState on old peer session got %q/%q, want empty", state, source)
	}
}
