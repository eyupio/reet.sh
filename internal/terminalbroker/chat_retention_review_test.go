package terminalbroker

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestChatRingRetainsWaitingCardAndCurrentStatus(t *testing.T) {
	for _, bytes := range []int{1, 8192} {
		t.Run(fmt.Sprint(bytes), func(t *testing.T) {
			l := newChatLog()
			l.publish(terminalproto.ACPEvent{Kind: "status", State: "waiting"})
			l.publish(terminalproto.ACPEvent{Kind: "permission", PermissionID: "permission-0123456789", Command: "echo review me", ToolKind: "execute"})
			for i := 0; i < maxChatEvents+10; i++ {
				l.publish(terminalproto.ACPEvent{Kind: "agent", MessageID: "msg_output", Text: strings.Repeat("x", bytes)})
			}
			snapshot, truncated, _, cancel := l.subscribe()
			defer cancel()
			card, status := false, false
			for i, e := range snapshot {
				if i > 0 && e.Seq <= snapshot[i-1].Seq {
					t.Fatal("snapshot sequence regressed")
				}
				card = card || e.Kind == "permission"
				status = status || e.Kind == "status" && e.State == "waiting"
			}
			if !card || !status {
				t.Fatalf("reconnect lost pending state: card=%v, status=%v", card, status)
			}
			if !truncated || len(snapshot) > maxChatEvents || l.bytes > maxChatBytes {
				t.Fatal("ring is not bounded")
			}
			l.publish(terminalproto.ACPEvent{Kind: "permission_resolved", PermissionID: "permission-0123456789"})
			for i := 0; i < maxChatEvents+1; i++ {
				l.publish(terminalproto.ACPEvent{Kind: "notice", Text: "after"})
			}
			for _, e := range l.events {
				if e.Kind == "permission" {
					t.Fatal("resolved card stayed pinned")
				}
			}
		})
	}
}

func TestChatRingFullOfCardsStillBoundsAndDeliversNewOutput(t *testing.T) {
	l := newChatLog()
	for i := 0; i < maxChatEvents+10; i++ {
		l.publish(terminalproto.ACPEvent{Kind: "permission", PermissionID: fmt.Sprintf("permission-%020d", i), ToolKind: "execute", Command: "review"})
	}
	if len(l.events) > maxChatEvents || l.bytes > maxChatBytes {
		t.Fatal("pending state exceeded hard ring bounds")
	}
	_, _, events, cancel := l.subscribe()
	defer cancel()
	l.publish(terminalproto.ACPEvent{Kind: "notice", Text: "new output"})
	got := <-events
	if got.Kind != "notice" || got.Text != "new output" || got.Seq != l.seq {
		t.Fatalf("evicting new history changed the live delivery: %+v", got)
	}
}

func TestChatRingRetainsPendingQuestions(t *testing.T) {
	l := newChatLog()
	l.publish(terminalproto.ACPEvent{Kind: "question", QuestionID: "question-0123456789", Question: brokerQuestion()})
	for i := 0; i < maxChatEvents+10; i++ {
		l.publish(terminalproto.ACPEvent{Kind: "notice", Text: "history"})
	}
	if l.events[0].Kind != "question" {
		t.Fatal("pending question was evicted")
	}
	l.publish(terminalproto.ACPEvent{Kind: "question_cancelled", QuestionID: "question-0123456789"})
	for i := 0; i < maxChatEvents+10; i++ {
		l.publish(terminalproto.ACPEvent{Kind: "notice", Text: "history"})
	}
	for _, event := range l.events {
		if event.Kind == "question" {
			t.Fatal("cancelled question stayed pinned")
		}
	}
}
