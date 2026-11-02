//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const recoverySessionID = "session-recovery0123456789"
const recoveryPermissionID = "permission-recovery0123456789"

func openRecoveryChat(t *testing.T) (*chatHost, *fakeChat) {
	t.Helper()
	h, proc, _ := testChatHost(t)
	t.Cleanup(h.sup.Close)
	if _, err := h.open("request-recovery0123456789", recoverySessionID, terminalproto.Create{
		Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: h.sup.opts.Home,
	}); err != nil {
		t.Fatal(err)
	}
	return h, proc
}

func recoveryApproval() acpagent.Event {
	return acpagent.Event{Kind: "permission", PermissionID: recoveryPermissionID, Title: "Run command", ToolKind: "execute", Command: strings.Repeat("x", 6000)}
}

func fillDetachedChat(proc *fakeChat, count, textBytes int) {
	for i := 0; i < count; i++ {
		proc.onEvent(acpagent.Event{Kind: "agent", MessageID: "msg_backlog", Text: strings.Repeat("x", textBytes)})
	}
}

func reattachRecoveryChat(t *testing.T, h *chatHost) (*chatHost, *sent) {
	t.Helper()
	out := &sent{}
	next := h.sup.attach(h.sup.ctx, 2, out.send, true, true)
	t.Cleanup(next.close)
	// Recovery must fit the existing ring budgets, rather than fixing lost cards
	// by letting the detached transcript grow without bounds.
	out.mu.Lock()
	count, bytes := 0, 0
	for _, message := range out.msgs {
		if event, ok := message.Body.(*terminalproto.ACPEvent); ok {
			count++
			bytes += chatEventBytes(*event)
			if message.Generation != 2 {
				t.Errorf("old connection generation: %d", message.Generation)
			}
		}
	}
	out.mu.Unlock()
	if count > maxDetachedChatEvents || bytes > maxDetachedChatBytes {
		t.Fatalf("unbounded replay: %d events, %d bytes", count, bytes)
	}
	return next, out
}

func TestPendingChatApprovalSurvivesFullDetachedBacklog(t *testing.T) {
	for _, tc := range []struct {
		name             string
		count, textBytes int
	}{
		{"event limit", maxDetachedChatEvents + 8, 1},
		{"byte limit", 128, 4096},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, proc := openRecoveryChat(t)
			approval := recoveryApproval()
			proc.onEvent(approval)
			proc.onEvent(acpagent.Event{Kind: "status", State: "waiting"})
			h.close()
			fillDetachedChat(proc, tc.count, tc.textBytes)
			next, out := reattachRecoveryChat(t, h)
			cards := out.events("permission")
			if len(cards) != 1 {
				t.Fatalf("recovered %d pending approvals, want one", len(cards))
			}
			if cards[0].PermissionID != recoveryPermissionID || cards[0].Command != approval.Command || cards[0].Bad || cards[0].Truncated {
				t.Fatalf("recovered approval changed: id=%q command bytes=%d bad=%v truncated=%v", cards[0].PermissionID, len(cards[0].Command), cards[0].Bad, cards[0].Truncated)
			}
			statuses := out.events("status")
			if len(statuses) == 0 || statuses[len(statuses)-1].State != "waiting" {
				t.Fatalf("lost waiting state: %+v", statuses)
			}
			select {
			case <-proc.closed:
				t.Fatal("recovery replaced the native process")
			default:
			}
			if err := next.permission(recoverySessionID, recoveryPermissionID, true); err != nil {
				t.Fatal(err)
			}
			proc.mu.Lock()
			allowed := len(proc.allow) == 1 && proc.allow[0]
			proc.mu.Unlock()
			if !allowed || len(out.events("permission_resolved")) != 1 {
				t.Fatal("recovered approval did not reach the original process")
			}
		})
	}
}

func TestResolvedChatApprovalIsNotRevivedByBacklogRecovery(t *testing.T) {
	h, proc := openRecoveryChat(t)
	proc.onEvent(recoveryApproval())
	proc.onEvent(acpagent.Event{Kind: "status", State: "waiting"})
	h.close()
	proc.onEvent(acpagent.Event{Kind: "permission_cancelled", PermissionID: recoveryPermissionID, Reason: "timeout"})
	fillDetachedChat(proc, maxDetachedChatEvents+8, 1)
	_, out := reattachRecoveryChat(t, h)
	if cards := out.events("permission"); len(cards) != 0 {
		t.Fatalf("revived a cancelled approval: %+v", cards)
	}
	receipts := out.events("permission_cancelled")
	if len(receipts) != 1 || receipts[0].PermissionID != recoveryPermissionID || receipts[0].Reason != "timeout" {
		t.Fatal("lost the cancellation receipt during overflow")
	}
	statuses := out.events("status")
	if len(statuses) == 0 || statuses[len(statuses)-1].State != "idle" {
		t.Fatalf("lost resolved state: %+v", statuses)
	}
}

func TestBufferedChatApprovalIsReplayedOnlyOnce(t *testing.T) {
	h, proc := openRecoveryChat(t)
	h.close()
	// Answered cards remain part of history; only a currently pending copy
	// moves to the recovery snapshot at the end of the replay.
	historical := recoveryApproval()
	historical.PermissionID = "permission-answered0123456789"
	proc.onEvent(historical)
	proc.onEvent(acpagent.Event{Kind: "status", State: "waiting"})
	allowed := true
	proc.onEvent(acpagent.Event{Kind: "permission_resolved", PermissionID: historical.PermissionID, Allow: &allowed})
	proc.onEvent(recoveryApproval())
	proc.onEvent(acpagent.Event{Kind: "status", State: "waiting"})
	_, out := reattachRecoveryChat(t, h)
	cards := out.events("permission")
	if len(cards) != 2 || cards[0].PermissionID != historical.PermissionID || cards[1].PermissionID != recoveryPermissionID {
		t.Fatalf("expected one historical and one pending approval, got %d cards", len(cards))
	}
	resolved := out.events("permission_resolved")
	if len(resolved) != 1 || resolved[0].PermissionID != historical.PermissionID || resolved[0].Allow == nil || !*resolved[0].Allow {
		t.Fatal("lost the historical approval's resolution")
	}
}

func TestPendingChatApprovalSurvivesOutputDuringRecovery(t *testing.T) {
	for _, failSend := range []bool{false, true} {
		name := "stalled send"
		if failSend {
			name = "failed send"
		}
		t.Run(name, func(t *testing.T) {
			h, proc := openRecoveryChat(t)
			proc.onEvent(recoveryApproval())
			proc.onEvent(acpagent.Event{Kind: "status", State: "waiting"})
			h.close()
			fillDetachedChat(proc, maxDetachedChatEvents+8, 1)
			entered, release := make(chan struct{}), make(chan struct{})
			var first, released sync.Once
			unblock := func() { released.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			out := &sent{}
			attached := make(chan *chatHost, 1)
			go func() {
				attached <- h.sup.attach(h.sup.ctx, 2, func(ctx context.Context, message terminalproto.Message) error {
					firstSend := false
					first.Do(func() { firstSend = true; close(entered); <-release })
					if firstSend && failSend {
						return errors.New("transport closed")
					}
					return out.send(ctx, message)
				}, true, true)
			}()
			waitUntil(t, "recovery's first send", func() bool {
				select {
				case <-entered:
					return true
				default:
					return false
				}
			})
			// The native process keeps streaming while the replacement transport
			// is blocked. Recovery records must survive this overflow as well.
			fillDetachedChat(proc, maxDetachedChatEvents+8, 1)
			assertRecoveryBudget(t, h)
			unblock()
			var next *chatHost
			waitUntil(t, "recovery to finish", func() bool {
				select {
				case next = <-attached:
					return true
				default:
					return false
				}
			})
			t.Cleanup(next.close)
			assertRecoveryBudget(t, h)
			if failSend {
				// Requeueing a failed send must remain bounded, and the next
				// connection must still receive the original pending approval.
				out = &sent{}
				replacement := h.sup.attach(h.sup.ctx, 3, out.send, true, true)
				t.Cleanup(replacement.close)
			}
			if cards := out.events("permission"); len(cards) != 1 || cards[0].PermissionID != recoveryPermissionID {
				t.Fatalf("approval lost during recovery: %d cards", len(cards))
			}
			statuses := out.events("status")
			if len(statuses) == 0 || statuses[len(statuses)-1].State != "waiting" {
				t.Fatalf("waiting state lost during recovery: %+v", statuses)
			}
		})
	}
}

func assertRecoveryBudget(t *testing.T, h *chatHost) {
	t.Helper()
	h.sup.mu.Lock()
	cs := h.sup.sessions[recoverySessionID]
	count, bytes := len(cs.outbox), cs.outboxBytes
	h.sup.mu.Unlock()
	if count > maxDetachedChatEvents || bytes > maxDetachedChatBytes {
		t.Fatalf("unbounded recovery buffer: %d events, %d bytes", count, bytes)
	}
}

func TestDetachedApprovalStateStillObeysHardQueueLimits(t *testing.T) {
	for _, receipts := range []bool{false, true} {
		name, count := "pending cards exceed bytes", 64
		if receipts {
			name, count = "receipts exceed event count", maxDetachedChatEvents+8
		}
		t.Run(name, func(t *testing.T) {
			h, proc := openRecoveryChat(t)
			h.close()
			for i := 0; i < count; i++ {
				event := recoveryApproval()
				event.PermissionID = "permission-budget-" + strconv.Itoa(i) + "-0123456789"
				if receipts {
					event = acpagent.Event{Kind: "permission_cancelled", PermissionID: event.PermissionID, Reason: "timeout"}
				}
				proc.onEvent(event)
			}
			// Even when control state alone fills the outbox, it must not grow
			// past the existing hard limits or keep retrying eviction forever.
			assertRecoveryBudget(t, h)
		})
	}
}
