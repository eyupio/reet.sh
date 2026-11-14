package api

import (
	"context"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A call whose caller has already given up must not end the machine's control
// link, which every session on the machine shares. Leaving a chat abandons the
// page's request for its live controls; on a busy controller that request was
// often still waiting to be sent, and sending it ended the link. A goal check
// that then found the machine reconnecting waited a minute before looking
// again.
func TestACallThatGaveUpLeavesTheMachineLinkUp(t *testing.T) {
	t.Parallel()
	h := newGM6Harness(t)
	h.linkMu.Lock()
	link, done := h.conn, h.workerDone
	h.linkMu.Unlock()
	broker := h.current.Load().s.terminals
	session := h.workers["ALPHA"].Session.ID
	gaveUp, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 40; i++ {
		if _, err := broker.ChatControl(gaveUp, h.uid, session, terminalproto.ACPControl{Action: "get"}); err == nil {
			t.Fatalf("call %d was answered though its caller had given up", i)
		}
	}
	select {
	case <-done:
		h.linkMu.Lock()
		why := h.linkErr
		h.linkMu.Unlock()
		t.Fatalf("calls that had given up ended the machine's link: %v", why)
	case <-time.After(200 * time.Millisecond):
	}
	ctx, stop := context.WithTimeout(t.Context(), 5*time.Second)
	defer stop()
	if _, err := broker.ChatControl(ctx, h.uid, session, terminalproto.ACPControl{Action: "get"}); err != nil {
		t.Fatalf("the machine did not answer after calls that gave up: %v", err)
	}
	h.linkMu.Lock()
	same := h.conn == link
	h.linkMu.Unlock()
	if !same {
		t.Fatal("the worker had to dial again")
	}
}
