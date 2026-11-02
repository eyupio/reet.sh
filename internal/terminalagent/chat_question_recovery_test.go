//go:build linux || darwin

package terminalagent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func pendingRecoveryQuestion(t *testing.T) (*chatHost, *questionChat, acpagent.Event) {
	t.Helper()
	h, proc, _ := questionHost(t, true)
	t.Cleanup(h.sup.Close)
	event := acpagent.Event{Kind: "question", QuestionID: connectorQuestionID, ToolCallID: "toolu_q", Question: sampleAgentQuestion()}
	proc.onEvent(event)
	proc.onEvent(acpagent.Event{Kind: "status", State: "waiting"})
	h.close()
	return h, proc, event
}

func TestPendingQuestionSurvivesReconnectBacklog(t *testing.T) {
	for _, tc := range []struct {
		name        string
		count, size int
	}{
		{"no backlog", 0, 0}, {"event limit", maxDetachedChatEvents + 8, 1}, {"byte limit", 128, 4096},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, proc, event := pendingRecoveryQuestion(t)
			fillDetachedChat(proc.fakeChat, tc.count, tc.size)
			out := &sent{}
			next := h.sup.attach(h.sup.ctx, 2, out.send, true, true, true)
			defer next.close()
			cards := out.events("question")
			expected := event.Question.Wire()
			if len(cards) != 1 || cards[0].QuestionID != connectorQuestionID || cards[0].ToolCallID != event.ToolCallID || !reflect.DeepEqual(cards[0].Question, &expected) {
				t.Fatalf("pending question not recovered intact: %+v", cards)
			}
			out.mu.Lock()
			for _, message := range out.msgs {
				if message.Generation != 2 {
					t.Errorf("old generation: %d", message.Generation)
				}
			}
			out.mu.Unlock()
			answer := terminalproto.ACPQuestionAnswer{QuestionID: connectorQuestionID, Action: "accept", Answers: json.RawMessage(`{"question_0":"Option A"}`)}
			if result, err := next.question("session-0123456789", &answer); err != nil || result.Code != terminalproto.OK {
				t.Fatalf("answer: %+v %v", result, err)
			}
			id, resolution, n := proc.last()
			if n != 1 || id != connectorQuestionID || resolution.Action != acpagent.QuestionAccept {
				t.Fatalf("original process: %q %+v (%d)", id, resolution, n)
			}
		})
	}
}

func TestQuestionTimeoutWhileDetachedDoesNotReviveWaiting(t *testing.T) {
	h, proc, _ := pendingRecoveryQuestion(t)
	proc.onEvent(acpagent.Event{Kind: "question_cancelled", QuestionID: connectorQuestionID, Reason: "timeout"})
	fillDetachedChat(proc.fakeChat, maxDetachedChatEvents+8, 1)
	out := &sent{}
	next := h.sup.attach(h.sup.ctx, 2, out.send, true, true, true)
	defer next.close()
	if cards := out.events("question"); len(cards) != 0 {
		t.Fatalf("revived question: %+v", cards)
	}
	statuses := out.events("status")
	if len(statuses) == 0 || statuses[len(statuses)-1].State != "idle" {
		t.Fatalf("timeout left waiting: %+v", statuses)
	}
}

func TestQuestionRecoveryToOlderPeerCancelsWithoutQuestionFrames(t *testing.T) {
	h, proc, _ := pendingRecoveryQuestion(t)
	// Leave a historical frame too, so strict old decoders never see it.
	out := &sent{}
	next := h.sup.attach(h.sup.ctx, 2, out.send, true, true, false)
	defer next.close()
	waitUntil(t, "pending question cancellation", func() bool { _, _, n := proc.last(); return n == 1 })
	id, resolution, _ := proc.last()
	if id != connectorQuestionID || resolution.Action != acpagent.QuestionCancel {
		t.Fatalf("cancellation: %q %+v", id, resolution)
	}
	proc.onEvent(acpagent.Event{Kind: "question_cancelled", QuestionID: connectorQuestionID, Reason: "cancelled"})
	for _, kind := range []string{"question", "question_resolved", "question_cancelled"} {
		if len(out.events(kind)) != 0 {
			t.Fatalf("old peer received %s", kind)
		}
	}
}

func TestQuestionRecoveryBytesIncludeSchema(t *testing.T) {
	question := sampleAgentQuestion().Wire()
	question.Message = strings.Repeat("x", 1000)
	event := terminalproto.ACPEvent{Kind: "question", QuestionID: connectorQuestionID, Question: &question}
	raw, _ := json.Marshal(question)
	if chatEventBytes(event) < len(raw)+len(connectorQuestionID) {
		t.Fatalf("schema not counted: %d < %d", chatEventBytes(event), len(raw)+len(connectorQuestionID))
	}
}

func TestPendingQuestionSurvivesStalledRecovery(t *testing.T) {
	for _, tc := range []struct {
		name          string
		fail, timeout bool
	}{
		{name: "blocked send"}, {name: "failed send", fail: true}, {name: "timeout during replay", timeout: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, proc, _ := pendingRecoveryQuestion(t)
			fillDetachedChat(proc.fakeChat, maxDetachedChatEvents+8, 1)
			entered, release := make(chan struct{}), make(chan struct{})
			var once, released sync.Once
			unblock := func() { released.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			out := &sent{}
			attached := make(chan *chatHost, 1)
			go func() {
				attached <- h.sup.attach(h.sup.ctx, 2, func(ctx context.Context, message terminalproto.Message) error {
					first := false
					once.Do(func() { first = true; close(entered); <-release })
					if first && tc.fail {
						return errors.New("link closed")
					}
					return out.send(ctx, message)
				}, true, true, true)
			}()
			waitUntil(t, "first recovery send", func() bool {
				select {
				case <-entered:
					return true
				default:
					return false
				}
			})
			if tc.timeout {
				proc.onEvent(acpagent.Event{Kind: "question_cancelled", QuestionID: connectorQuestionID, Reason: "timeout"})
			}
			fillDetachedChat(proc.fakeChat, maxDetachedChatEvents+8, 1)
			h.sup.mu.Lock()
			cs := h.sup.sessions["session-0123456789"]
			count, bytes := len(cs.outbox), cs.outboxBytes
			h.sup.mu.Unlock()
			if count > maxDetachedChatEvents || bytes > maxDetachedChatBytes {
				t.Errorf("overflow: %d events, %d bytes", count, bytes)
			}
			unblock()
			var next *chatHost
			waitUntil(t, "recovery completion", func() bool {
				select {
				case next = <-attached:
					return true
				default:
					return false
				}
			})
			defer next.close()
			if tc.fail {
				out = &sent{}
				final := h.sup.attach(h.sup.ctx, 3, out.send, true, true, true)
				defer final.close()
			}
			if tc.timeout {
				statuses := out.events("status")
				if len(statuses) == 0 || statuses[len(statuses)-1].State != "idle" {
					t.Fatalf("stale waiting status: %+v", statuses)
				}
			} else if cards := out.events("question"); len(cards) != 1 || cards[0].QuestionID != connectorQuestionID {
				t.Fatalf("recovered cards: %+v", cards)
			}
		})
	}
}

func TestQuestionFirstAskedWhileDetachedIsStillCancelled(t *testing.T) {
	h, proc, _ := questionHost(t, true)
	t.Cleanup(h.sup.Close)
	h.close()
	proc.onEvent(acpagent.Event{Kind: "question", QuestionID: connectorQuestionID, Question: sampleAgentQuestion()})
	waitUntil(t, "new detached question cancellation", func() bool { _, _, n := proc.last(); return n == 1 })
	out := &sent{}
	next := h.sup.attach(h.sup.ctx, 2, out.send, true, true, true)
	defer next.close()
	if len(out.events("question")) != 0 {
		t.Fatal("replayed a question never delivered live")
	}
}

func TestQuestionHistoryIsNotSentToAnOlderPeerAfterExit(t *testing.T) {
	h, proc, _ := questionHost(t, true)
	t.Cleanup(h.sup.Close)
	h.link.send = func(context.Context, terminalproto.Message) error { return errors.New("link failed") }
	proc.onEvent(acpagent.Event{Kind: "question", QuestionID: connectorQuestionID, Question: sampleAgentQuestion()})
	proc.onEvent(acpagent.Event{Kind: "status", State: "ended"})
	out := &sent{}
	next := h.sup.attach(h.sup.ctx, 2, out.send, true, true, false)
	defer next.close()
	if len(out.events("question")) != 0 {
		t.Fatal("old peer received historical question after exit")
	}
}

func TestWaitingStatusRetainsTheOtherKindOfPendingCard(t *testing.T) {
	for _, resolveQuestion := range []bool{false, true} {
		cs := &chatSession{lastStatus: &terminalproto.ACPEvent{Kind: "status", State: "waiting"}}
		recordChatSessionEvent(cs, terminalproto.ACPEvent{Kind: "permission", PermissionID: recoveryPermissionID})
		recordChatSessionEvent(cs, terminalproto.ACPEvent{Kind: "question", QuestionID: connectorQuestionID})
		if resolveQuestion {
			recordChatSessionEvent(cs, terminalproto.ACPEvent{Kind: "question_cancelled", QuestionID: connectorQuestionID})
		} else {
			recordChatSessionEvent(cs, terminalproto.ACPEvent{Kind: "permission_cancelled", PermissionID: recoveryPermissionID})
		}
		if cs.lastStatus.State != "waiting" {
			t.Fatal("another card is still waiting")
		}
	}
}

func TestOlderPeerNeverReceivesLateRequeuedQuestion(t *testing.T) {
	h, proc, _ := questionHost(t, true)
	t.Cleanup(h.sup.Close)
	oldEntered, oldRelease := make(chan struct{}), make(chan struct{})
	replayEntered, replayRelease := make(chan struct{}), make(chan struct{})
	var releaseOld, releaseReplay sync.Once
	unblockOld := func() { releaseOld.Do(func() { close(oldRelease) }) }
	unblockReplay := func() { releaseReplay.Do(func() { close(replayRelease) }) }
	t.Cleanup(unblockOld)
	t.Cleanup(unblockReplay)
	h.link.send = func(context.Context, terminalproto.Message) error {
		close(oldEntered)
		<-oldRelease
		return errors.New("old link failed")
	}
	published := make(chan struct{})
	go func() {
		defer close(published)
		proc.onEvent(acpagent.Event{Kind: "question", QuestionID: connectorQuestionID, Question: sampleAgentQuestion()})
	}()
	waitUntil(t, "old send", func() bool {
		select {
		case <-oldEntered:
			return true
		default:
			return false
		}
	})
	h.close()
	fillDetachedChat(proc.fakeChat, 1, 1)
	out := &sent{}
	var first sync.Once
	attached := make(chan *chatHost, 1)
	go func() {
		attached <- h.sup.attach(h.sup.ctx, 2, func(ctx context.Context, message terminalproto.Message) error {
			first.Do(func() { close(replayEntered); <-replayRelease })
			return out.send(ctx, message)
		}, true, true, false)
	}()
	waitUntil(t, "replacement replay", func() bool {
		select {
		case <-replayEntered:
			return true
		default:
			return false
		}
	})
	unblockOld()
	waitUntil(t, "old send failure", func() bool {
		select {
		case <-published:
			return true
		default:
			return false
		}
	})
	unblockReplay()
	var next *chatHost
	waitUntil(t, "replacement attach", func() bool {
		select {
		case next = <-attached:
			return true
		default:
			return false
		}
	})
	defer next.close()
	if len(out.events("question")) != 0 {
		t.Fatal("older peer received a question requeued by the old send")
	}
}
