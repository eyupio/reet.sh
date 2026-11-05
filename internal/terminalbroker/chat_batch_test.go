package terminalbroker

import (
	"fmt"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func batchedChatLog(t *testing.T) (*chatLog, *store.Store) {
	t.Helper()
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser("chat-batch@example.com", "chat-batch", "owner", "hash")
	if err != nil {
		t.Fatal(err)
	}
	l := newChatLog()
	l.st, l.uid, l.sessionID = st, u.ID, "chat-session-0001"
	return l, st
}

func waitForChatEvents(t *testing.T, st *store.Store, uid int64, session string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		events, err := st.TerminalChatEventsFor(uid, session)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("stored %d events, want %d", len(events), want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestChatLogFlushesQuietTailAfterDelay(t *testing.T) {
	l, st := batchedChatLog(t)
	l.publish(terminalproto.ACPEvent{Kind: "agent", MessageID: "msg", Text: "one"})
	waitForChatEvents(t, st, l.uid, l.sessionID, 1)
}

func TestChatLogFlushesBatchAtThreshold(t *testing.T) {
	l, st := batchedChatLog(t)
	for i := range chatWriteBatch {
		l.publish(terminalproto.ACPEvent{Kind: "agent", MessageID: "msg", Text: fmt.Sprintf("chunk %d", i)})
	}
	waitForChatEvents(t, st, l.uid, l.sessionID, chatWriteBatch)
}

func TestChatLogCloseFlushesPendingTail(t *testing.T) {
	l, st := batchedChatLog(t)
	l.publish(terminalproto.ACPEvent{Kind: "agent", MessageID: "msg", Text: "tail"})
	l.close()
	events, err := st.TerminalChatEventsFor(l.uid, l.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("close left %d events, want 1", len(events))
	}
}

func TestChatLogCloseWaitsForDetachedFlush(t *testing.T) {
	l, st := batchedChatLog(t)
	l.publish(terminalproto.ACPEvent{Kind: "agent", MessageID: "msg", Text: "tail"})

	// Simulate a timer callback that has taken the batch and released l.mu but
	// has not reached the store yet.
	l.mu.Lock()
	batch, trimSeq := l.takeWritesLocked()
	l.mu.Unlock()
	if len(batch) == 0 {
		t.Fatal("expected a pending write")
	}

	closed := make(chan struct{})
	go func() {
		l.close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("close returned while a detached flush was still pending")
	case <-time.After(50 * time.Millisecond):
	}

	l.persist(batch, trimSeq)
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("close did not return after the detached flush")
	}
	events, err := st.TerminalChatEventsFor(l.uid, l.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("stored %d events, want 1", len(events))
	}
}
