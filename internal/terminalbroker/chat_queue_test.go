package terminalbroker

import (
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestSteeringEchoDoesNotReopenCompletedTurn(t *testing.T) {
	l := newChatLog()
	l.publish(terminalproto.ACPEvent{Kind: "status", State: "working"})
	l.publish(terminalproto.ACPEvent{Kind: "status", State: "idle"})
	completed := l.completedAt
	l.publishSteered("injected just before completion")
	if l.running || l.lastState != "finished" || l.completedAt != completed {
		t.Fatal("steering echo reopened completed turn")
	}
	if last := l.events[len(l.events)-1]; last.Kind != "user" || last.Text != "injected just before completion" {
		t.Fatal("steering echo missing", last)
	}
}

func TestQueuePausesAfterAgentFailureUntilExplicitRetry(t *testing.T) {
	prompts := 0
	b, _, uid, agent := chatFixtureScript(t, nil, func(c *terminalproto.Capabilities) { c.ACPLiveControls = true }, nil, func(m terminalproto.Message) (terminalproto.Body, bool) {
		switch m.Body.(type) {
		case *terminalproto.ACPControl:
			return terminalproto.ACPControlResult{Controls: terminalproto.ACPControls{Options: []terminalproto.ACPSetting{}}}, true
		case *terminalproto.ACPPrompt:
			prompts++
		}
		return nil, true
	})
	s := chatSession(t, b, uid, agent)
	b.publishChat(uid, s.ID, terminalproto.ACPEvent{Kind: "status", State: "error", Detail: "provider quota"})
	if err := b.st.EnqueueTerminalFor(uid, s.ID, ID(), "next", nil); err != nil {
		t.Fatal(err)
	}
	b.stepChatQueue(uid, s.ID)
	q, _ := b.st.TerminalQueueFor(uid, s.ID)
	if prompts != 0 || len(q) != 1 || q[0].State != "failed" {
		t.Fatal("failed provider drained the queue", prompts, q)
	}
	if err := b.st.ChangeTerminalQueueFor(uid, s.ID, q[0].ID, q[0].Revision, "retry", "", false); err != nil {
		t.Fatal(err)
	}
	b.stepChatQueue(uid, s.ID)
	if prompts != 1 {
		t.Fatal("explicit retry stayed paused")
	}
}

func TestQueueWaitsForActualTurnAfterStop(t *testing.T) {
	working := true
	prompts, cancels := 0, 0
	b, _, uid, agent := chatFixtureScript(t, nil, func(c *terminalproto.Capabilities) { c.ACPLiveControls = true }, nil, func(m terminalproto.Message) (terminalproto.Body, bool) {
		switch m.Body.(type) {
		case *terminalproto.ACPControl:
			return terminalproto.ACPControlResult{Controls: terminalproto.ACPControls{Working: working, Options: []terminalproto.ACPSetting{}}}, true
		case *terminalproto.ACPPrompt:
			prompts++
		case *terminalproto.ACPCancel:
			cancels++
		}
		return nil, true
	})
	s := chatSession(t, b, uid, agent)
	id := ID()
	if err := b.st.EnqueueTerminalFor(uid, s.ID, id, "next", nil); err != nil {
		t.Fatal(err)
	}
	q, _ := b.st.TerminalQueueFor(uid, s.ID)
	if err := b.st.ChangeTerminalQueueFor(uid, s.ID, id, q[0].Revision, "stop", "", false); err != nil {
		t.Fatal(err)
	}
	b.stepChatQueue(uid, s.ID)
	if cancels != 1 || prompts != 0 {
		t.Fatal("stop did not precede delivery", cancels, prompts)
	}
	b.publishChat(uid, s.ID, terminalproto.ACPEvent{Kind: "status", State: "cancelled"})
	b.stepChatQueue(uid, s.ID)
	if prompts != 0 {
		t.Fatal("early cancelled event dispatched a new turn")
	}
	working = false
	b.stepChatQueue(uid, s.ID)
	if prompts != 1 || cancels != 1 {
		t.Fatal("did not deliver once when actually idle", prompts, cancels)
	}
	if err := b.st.EnqueueTerminalFor(uid, s.ID, id, "next", nil); err != nil {
		t.Fatal(err)
	}
	b.stepChatQueue(uid, s.ID)
	if prompts != 1 {
		t.Fatal("HTTP retry duplicated delivery")
	}
}

func TestQueueSteeringOutcomesNeverResendUncertainty(t *testing.T) {
	for _, outcome := range []string{"injected", "promptRequired", "rejected", "unknown"} {
		t.Run(outcome, func(t *testing.T) {
			prompts, steers := 0, 0
			b, _, uid, agent := chatFixtureScript(t, nil, func(c *terminalproto.Capabilities) { c.ACPLiveControls = true }, nil, func(m terminalproto.Message) (terminalproto.Body, bool) {
				switch op := m.Body.(type) {
				case *terminalproto.ACPControl:
					answer := terminalproto.ACPControlResult{Controls: terminalproto.ACPControls{Working: true, Steering: true, Options: []terminalproto.ACPSetting{}}}
					if op.Action == "steer" {
						steers++
						answer.Outcome = outcome
					}
					return answer, true
				case *terminalproto.ACPPrompt:
					prompts++
				}
				return nil, true
			})
			s := chatSession(t, b, uid, agent)
			id := ID()
			if err := b.st.EnqueueTerminalFor(uid, s.ID, id, "urgent", nil); err != nil {
				t.Fatal(err)
			}
			q, _ := b.st.TerminalQueueFor(uid, s.ID)
			if err := b.st.ChangeTerminalQueueFor(uid, s.ID, id, q[0].Revision, "send", "", false); err != nil {
				t.Fatal(err)
			}
			b.stepChatQueue(uid, s.ID)
			b.stepChatQueue(uid, s.ID)
			if steers != 1 {
				t.Fatal("steering repeated", steers)
			}
			want := 0
			if outcome == "promptRequired" {
				want = 1
			}
			if prompts != want {
				t.Fatal("unsafe prompt fallback", prompts)
			}
			q, _ = b.st.TerminalQueueFor(uid, s.ID)
			if outcome == "unknown" && (len(q) != 1 || q[0].State != "uncertain") {
				t.Fatal("uncertain message lost", q)
			}
			if outcome == "rejected" && (len(q) != 1 || q[0].State != "failed") {
				t.Fatal("refusal lost", q)
			}
		})
	}
}

func TestQueueWillNotPassWaitingOrUncertainMessage(t *testing.T) {
	waiting := true
	prompts := 0
	b, _, uid, agent := chatFixtureScript(t, nil, func(c *terminalproto.Capabilities) { c.ACPLiveControls = true }, nil, func(m terminalproto.Message) (terminalproto.Body, bool) {
		switch m.Body.(type) {
		case *terminalproto.ACPControl:
			return terminalproto.ACPControlResult{Controls: terminalproto.ACPControls{Waiting: waiting, Options: []terminalproto.ACPSetting{}}}, true
		case *terminalproto.ACPPrompt:
			prompts++
		}
		return nil, true
	})
	s := chatSession(t, b, uid, agent)
	for range 2 {
		if err := b.st.EnqueueTerminalFor(uid, s.ID, ID(), "queued", nil); err != nil {
			t.Fatal(err)
		}
	}
	b.stepChatQueue(uid, s.ID)
	if prompts != 0 {
		t.Fatal("queue passed waiting approval")
	}
	q, _ := b.st.TerminalQueueFor(uid, s.ID)
	ok, err := b.st.TransitionTerminalQueueFor(uid, s.ID, q[0], "sending", "wait", "", "old-controller")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := b.st.RecoverTerminalQueueFor(uid, s.ID, b.queueOwner); err != nil {
		t.Fatal(err)
	}
	waiting = false
	b.stepChatQueue(uid, s.ID)
	if prompts != 0 {
		t.Fatal("queue passed uncertain delivery")
	}
	q, _ = b.st.TerminalQueueFor(uid, s.ID)
	if len(q) != 2 || q[0].State != "uncertain" {
		t.Fatal(q)
	}
}

func TestEnqueueChatSendsAtOnceWhenIdleAndQueuesWhenWorking(t *testing.T) {
	working := false
	prompts := 0
	b, _, uid, agent := chatFixtureScript(t, nil, func(c *terminalproto.Capabilities) { c.ACPLiveControls = true }, nil, func(m terminalproto.Message) (terminalproto.Body, bool) {
		switch m.Body.(type) {
		case *terminalproto.ACPControl:
			return terminalproto.ACPControlResult{Controls: terminalproto.ACPControls{Working: working, Options: []terminalproto.ACPSetting{}}}, true
		case *terminalproto.ACPPrompt:
			prompts++
		}
		return nil, true
	})
	s := chatSession(t, b, uid, agent)
	if err := b.EnqueueChat(uid, s.ID, ID(), "hello", nil); err != nil {
		t.Fatal(err)
	}
	if prompts != 1 {
		t.Fatalf("idle message waited for the queue worker: %d prompts", prompts)
	}
	if q, _ := b.st.TerminalQueueFor(uid, s.ID); len(q) != 0 {
		t.Fatalf("delivered message stayed in the queue: %+v", q)
	}
	working = true
	if err := b.EnqueueChat(uid, s.ID, ID(), "next", nil); err != nil {
		t.Fatal(err)
	}
	if prompts != 1 {
		t.Fatalf("message for a working agent was sent directly: %d prompts", prompts)
	}
	q, _ := b.st.TerminalQueueFor(uid, s.ID)
	if len(q) != 1 || q[0].State != "queued" {
		t.Fatalf("busy message was not queued: %+v", q)
	}
}

func TestAFencedControllerDeliversNoQueuedMessageUntilTheLift(t *testing.T) {
	prompts := 0
	b, _, uid, agent := chatFixtureScript(t, nil, func(c *terminalproto.Capabilities) { c.ACPLiveControls = true }, nil, func(m terminalproto.Message) (terminalproto.Body, bool) {
		switch m.Body.(type) {
		case *terminalproto.ACPControl:
			return terminalproto.ACPControlResult{Controls: terminalproto.ACPControls{Options: []terminalproto.ACPSetting{}}}, true
		case *terminalproto.ACPPrompt:
			prompts++
		}
		return nil, true
	})
	s := chatSession(t, b, uid, agent)
	if err := b.st.EnqueueTerminalFor(uid, s.ID, ID(), "next", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.st.SetFence(""); err != nil {
		t.Fatal(err)
	}
	b.stepChatQueue(uid, s.ID)
	if q, _ := b.st.TerminalQueueFor(uid, s.ID); prompts != 0 || len(q) != 1 || q[0].State != "queued" {
		t.Fatal("a queued message was delivered while fenced", prompts, q)
	}
	if _, err := b.st.LiftFence(); err != nil {
		t.Fatal(err)
	}
	b.stepChatQueue(uid, s.ID)
	if prompts != 1 {
		t.Fatalf("after the lift %d prompts were sent, want 1", prompts)
	}
}

func TestQueueRetriesWhenControlReadConflicts(t *testing.T) {
	prompts, controls := 0, 0
	b, _, uid, agent := chatFixtureScript(t, nil, func(c *terminalproto.Capabilities) { c.ACPLiveControls = true }, nil, func(m terminalproto.Message) (terminalproto.Body, bool) {
		switch m.Body.(type) {
		case *terminalproto.ACPControl:
			controls++
			if controls == 1 {
				// The machine may be temporarily at its control-request limit.
				return terminalproto.Result{Code: terminalproto.Conflict}, true
			}
			return terminalproto.ACPControlResult{Controls: terminalproto.ACPControls{}}, true
		case *terminalproto.ACPPrompt:
			prompts++
		}
		return nil, true
	})
	s := chatSession(t, b, uid, agent)
	if err := b.st.EnqueueTerminalFor(uid, s.ID, ID(), "follow on", nil); err != nil {
		t.Fatal(err)
	}
	b.stepChatQueue(uid, s.ID)
	if prompts != 0 {
		t.Fatalf("conflicting read still sent the message: %d prompts", prompts)
	}
	q, _ := b.st.TerminalQueueFor(uid, s.ID)
	if len(q) != 1 || q[0].State != "queued" || q[0].Note != "" {
		t.Fatalf("transient overload lost the queued message: %+v", q)
	}
	b.stepChatQueue(uid, s.ID)
	if prompts != 1 {
		t.Fatalf("recovered control capacity did not send exactly once: %d", prompts)
	}
}

func TestQueueFailsWhenSessionEnds(t *testing.T) {
	b, p, uid, agent := chatFixtureScript(t, nil, func(c *terminalproto.Capabilities) { c.ACPLiveControls = true }, nil, nil)
	s := chatSession(t, b, uid, agent)
	if err := b.st.EnqueueTerminalFor(uid, s.ID, ID(), "follow on", nil); err != nil {
		t.Fatal(err)
	}
	if err := b.st.ObserveTerminalStateFor(uid, agent, s.ID, p.Generation, terminalproto.Exited, nil); err != nil {
		t.Fatal(err)
	}
	q, err := b.ChatQueue(uid, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(q) != 1 || q[0].State != "failed" || q[0].Note != "The session ended before this message was sent. Reconnect it, then retry." {
		t.Fatalf("queued message outlived its session: %+v", q)
	}
}
