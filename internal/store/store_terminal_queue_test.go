package store

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func queueFixture(t *testing.T) (*Store, int64, string) {
	t.Helper()
	s, uid, a, c := terminalFixture(t)
	q := createTerminal(t, s, uid, a, c)
	if _, err := s.db.Exec(`UPDATE terminal_sessions SET structured=1,state='running' WHERE id=?`, q.ID); err != nil {
		t.Fatal(err)
	}
	return s, uid, q.ID
}

func TestDurableQueueClaimsAndIdempotency(t *testing.T) {
	s, uid, session := queueFixture(t)
	id := syncID()
	if err := s.EnqueueTerminalFor(uid, session, id, "one", nil); err != nil {
		t.Fatal(err)
	}
	q, _ := s.TerminalQueueFor(uid, session)
	old := q[0]
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := s.TransitionTerminalQueueFor(uid, session, old, "sending", "wait", "", "owner-one")
			if err != nil {
				t.Error(err)
			}
			if ok {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("%d workers claimed the message", wins)
	}
	if err := s.ChangeTerminalQueueFor(uid, session, id, old.Revision, "remove", "", false); !errors.Is(err, ErrChatQueueConflict) {
		t.Fatal("removed a claimed message", err)
	}
	if err := s.RecoverTerminalQueueFor(uid, session, "owner-two"); err != nil {
		t.Fatal(err)
	}
	other, err := Open(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	q, err = other.TerminalQueueFor(uid, session)
	if err != nil || len(q) != 1 || q[0].State != "uncertain" {
		t.Fatalf("restart lost uncertain message: %+v %v", q, err)
	}
	if err := s.ChangeTerminalQueueFor(uid, session, id, q[0].Revision, "retry", "", false); !errors.Is(err, ErrChatQueueConflict) {
		t.Fatal("uncertainty silently retried", err)
	}
	if err := s.ChangeTerminalQueueFor(uid, session, id, q[0].Revision, "retry", "", true); err != nil {
		t.Fatal(err)
	}
	q, _ = s.TerminalQueueFor(uid, session)
	if ok, err := s.TransitionTerminalQueueFor(uid, session, q[0], "sent", "wait", "", "owner-two"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := s.EnqueueTerminalFor(uid, session, id, "one", nil); err != nil {
		t.Fatal(err)
	}
	q, _ = s.TerminalQueueFor(uid, session)
	if len(q) != 0 {
		t.Fatal("a repeated enqueue resurrected a sent message", q)
	}
	if _, err := s.TerminalQueueFor(uid+1, session); err == nil {
		t.Fatal("foreign account read queue")
	}
}

func TestDurableQueueBoundsAndEdits(t *testing.T) {
	s, uid, session := queueFixture(t)
	for i := range 10 {
		if err := s.EnqueueTerminalFor(uid, session, fmt.Sprintf("queue-message-%04d", i), "queued", nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.EnqueueTerminalFor(uid, session, syncID(), "overflow", nil); !errors.Is(err, ErrChatQueueFull) {
		t.Fatal(err)
	}
	q, _ := s.TerminalQueueFor(uid, session)
	if err := s.ChangeTerminalQueueFor(uid, session, q[0].ID, q[0].Revision, "edit", "updated", false); err != nil {
		t.Fatal(err)
	}
	if err := s.ChangeTerminalQueueFor(uid, session, q[0].ID, q[0].Revision, "remove", "", false); !errors.Is(err, ErrChatQueueConflict) {
		t.Fatal("stale browser overwrote edit", err)
	}
	q, _ = s.TerminalQueueFor(uid, session)
	if q[0].Text != "updated" {
		t.Fatal(q)
	}
	if err := s.EnqueueTerminalFor(uid+1, session, syncID(), "foreign", nil); !errors.Is(err, ErrRemoteNotFound) {
		t.Fatal(err)
	}
	if err := s.EnqueueTerminalFor(uid, session, syncID(), "", []string{"relative.png"}); !errors.Is(err, terminalproto.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestQueueCapacityAcrossStoreHandlesAndArchiveCleanup(t *testing.T) {
	s, uid, session := queueFixture(t)
	other, err := Open(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted := 0
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db := s
			if i%2 == 0 {
				db = other
			}
			err := db.EnqueueTerminalFor(uid, session, fmt.Sprintf("cross-handle-%05d", i), "shared", nil)
			if err == nil {
				mu.Lock()
				accepted++
				mu.Unlock()
			} else if !errors.Is(err, ErrChatQueueFull) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if accepted != 10 {
		t.Fatalf("capacity admitted %d messages", accepted)
	}
	if _, err := s.db.Exec(`UPDATE terminal_sessions SET state='ended' WHERE id=?`, session); err != nil {
		t.Fatal(err)
	}
	if err := s.ArchiveTerminalFor(uid, session); err != nil {
		t.Fatal(err)
	}
	q, err := s.TerminalQueueFor(uid, session)
	if err != nil || len(q) != 0 {
		t.Fatal("archive retained queue", q, err)
	}
	var payload int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM terminal_chat_queue WHERE session_id=? AND (text!='' OR images!='[]')`, session).Scan(&payload); err != nil || payload != 0 {
		t.Fatal("archive retained content", payload, err)
	}
}

func TestFailQueuedLeavesClaimedAndUncertainRows(t *testing.T) {
	s, uid, session := queueFixture(t)
	queued, claimed := syncID(), syncID()
	for _, id := range []string{queued, claimed} {
		if err := s.EnqueueTerminalFor(uid, session, id, "message", nil); err != nil {
			t.Fatal(err)
		}
	}
	q, _ := s.TerminalQueueFor(uid, session)
	byID := map[string]TerminalQueuedMessage{}
	for _, item := range q {
		byID[item.ID] = item
	}
	if ok, err := s.TransitionTerminalQueueFor(uid, session, byID[claimed], "sending", "wait", "", "owner-one"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := s.RecoverTerminalQueueFor(uid, session, "owner-two"); err != nil {
		t.Fatal(err)
	}
	if err := s.FailQueuedTerminalFor(uid, session, "ended"); err != nil {
		t.Fatal(err)
	}
	q, _ = s.TerminalQueueFor(uid, session)
	state := map[string]string{}
	for _, item := range q {
		state[item.ID] = item.State
	}
	if state[queued] != "failed" {
		t.Fatalf("queued row not failed: %+v", state)
	}
	if state[claimed] != "uncertain" {
		t.Fatalf("claimed delivery was rewritten: %+v", state)
	}
}
