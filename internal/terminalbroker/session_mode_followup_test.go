package terminalbroker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// eventsNamed returns the details of the audit events with that action.
func eventsNamed(t *testing.T, b *Broker, uid int64, action string) []map[string]string {
	t.Helper()
	events, err := b.st.ListEventsFor(uid, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]string
	for _, e := range events {
		if e.Action != action {
			continue
		}
		meta := map[string]string{}
		if err := json.Unmarshal([]byte(e.Detail), &meta); err != nil {
			t.Fatal(err)
		}
		out = append(out, meta)
	}
	return out
}

func setMode(machine *switchMachine, reply func(*Peer, terminalproto.Message, *terminalproto.SessionMode) terminalproto.Body) {
	machine.mu.Lock()
	machine.reply = reply
	machine.mu.Unlock()
}

// C1: a second tab that read the row before the first tab's switch finished
// decides from the row it finds once it holds the switch slot: the session is
// already there, so nothing is sent and nothing is audited twice.
func TestSwitchDecidesFromTheRowAfterClaiming(t *testing.T) {
	b, _, uid, agent, machine := switchFixture(t, true)
	s := switchChat(t, b, uid, agent)
	first := true
	old := afterSessionModeRead
	t.Cleanup(func() { afterSessionModeRead = old })
	afterSessionModeRead = func() {
		if !first {
			return
		}
		first = false
		// The other tab's switch runs and finishes in between.
		if err := b.SwitchSessionMode(context.Background(), uid, s.ID, terminalproto.SessionModeTerminal, false); err != nil {
			t.Errorf("other tab: %v", err)
		}
	}
	if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false); err != nil {
		t.Fatalf("stale switch: %v", err)
	}
	if n := len(machine.sent()); n != 1 {
		t.Fatalf("machine was asked %d times, want once", n)
	}
	if got := row(t, b, uid, s.ID); got.Structured || got.State != terminalproto.Running {
		t.Fatalf("row %+v", got)
	}
	if events := modeEvents(t, b, uid); len(events) != 1 {
		t.Fatalf("audit events %+v", events)
	}
}

// C2: a switch the machine could neither finish nor roll back ends the row;
// the person is told it ended rather than to try again.
func TestSwitchThatEndedTheSessionSaysSo(t *testing.T) {
	const sentence = "The session could not be switched and has ended. Start a new session or continue it with history."
	t.Run("ended notice before the failure", func(t *testing.T) {
		b, _, uid, agent, machine := switchFixture(t, true)
		s := switchChat(t, b, uid, agent)
		setMode(machine, func(p *Peer, m terminalproto.Message, _ *terminalproto.SessionMode) terminalproto.Body {
			// The connector retires the row (its ended notice), then answers
			// the swap with the start's failure.
			body := terminalproto.SessionChanged{State: terminalproto.Exited}
			if err := p.Receive(terminalproto.Message{Version: 1, RequestID: ID(), SessionID: m.SessionID, Generation: p.Generation, Body: &body}); err != nil {
				t.Error(err)
			}
			return terminalproto.Result{Code: terminalproto.LaunchFailed}
		})
		err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false)
		if !errors.Is(err, ErrSessionModeEnded) || err.Error() != sentence {
			t.Fatalf("err %v", err)
		}
		if got := row(t, b, uid, s.ID); got.State == terminalproto.Running {
			t.Fatalf("row %+v", got)
		}
		if len(modeEvents(t, b, uid)) != 0 {
			t.Fatal("a failed switch was audited")
		}
	})
	t.Run("answer says the session exited", func(t *testing.T) {
		b, _, uid, agent, machine := switchFixture(t, true)
		s := switchChat(t, b, uid, agent)
		if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false); err != nil {
			t.Fatal(err)
		}
		setMode(machine, func(*Peer, terminalproto.Message, *terminalproto.SessionMode) terminalproto.Body {
			return terminalproto.SessionChanged{State: terminalproto.Exited}
		})
		err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeChat, true)
		if !errors.Is(err, ErrSessionModeEnded) {
			t.Fatalf("err %v", err)
		}
		if got := row(t, b, uid, s.ID); got.State == terminalproto.Running || got.Structured {
			t.Fatalf("row %+v", got)
		}
		if b.chatExists(uid, s.ID) {
			t.Fatal("ring left behind")
		}
	})
	t.Run("failure with the row still running stays retryable", func(t *testing.T) {
		b, _, uid, agent, machine := switchFixture(t, true)
		s := switchChat(t, b, uid, agent)
		setMode(machine, func(*Peer, terminalproto.Message, *terminalproto.SessionMode) terminalproto.Body {
			return terminalproto.Result{Code: terminalproto.LaunchFailed}
		})
		if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false); !errors.Is(err, ErrSessionModeFailed) {
			t.Fatalf("err %v", err)
		}
		if got := row(t, b, uid, s.ID); got.State != terminalproto.Running || !got.Structured {
			t.Fatalf("row %+v", got)
		}
	})
}

// C3: a chat restored by a switch with an empty replay is not made busy by
// stale row fields from before the switch.
func TestSwitchBackToChatIsNotSeededBusy(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stale func(t *testing.T, b *Broker, p *Peer, uid int64, agent, session string)
	}{
		{"working status", func(t *testing.T, b *Broker, _ *Peer, uid int64, _, session string) {
			if err := b.st.SetTerminalChatAttentionFor(uid, session, time.Now().UnixMilli(), 0, "working", 0); err != nil {
				t.Fatal(err)
			}
		}},
		{"recent activity", func(t *testing.T, b *Broker, _ *Peer, uid int64, _, session string) {
			if err := b.st.SetTerminalChatAttentionFor(uid, session, time.Now().UnixMilli(), 0, "", 0); err != nil {
				t.Fatal(err)
			}
		}},
		{"terminal bell", func(t *testing.T, b *Broker, p *Peer, uid int64, agent, session string) {
			if err := b.SwitchSessionMode(t.Context(), uid, session, terminalproto.SessionModeTerminal, false); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UnixMilli()
			if err := b.st.ObserveTerminalAttentionFor(uid, agent, session, p.Generation, now, now); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, p, uid, agent, _ := switchFixture(t, true)
			s := switchChat(t, b, uid, agent)
			tc.stale(t, b, p, uid, agent, s.ID)
			if row(t, b, uid, s.ID).Structured {
				if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false); err != nil {
					t.Fatal(err)
				}
			}
			if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeChat, true); err != nil {
				t.Fatal(err)
			}
			if b.chatBusy(uid, s.ID) {
				t.Fatal("an empty replay left the chat busy from stale row fields")
			}
			if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false); err != nil {
				t.Fatalf("switch after an empty replay: %v", err)
			}
		})
	}
}

// C4: YOLO is audited on a switch, a switch to the OpenCode TUI is a YOLO
// launch, and it needs the account to still allow YOLO.
func TestSwitchYoloIsAuditedAndRechecked(t *testing.T) {
	b, _, uid, agent, machine := switchFixture(t, true)
	plain := switchChat(t, b, uid, agent)
	if err := b.SwitchSessionMode(t.Context(), uid, plain.ID, terminalproto.SessionModeTerminal, false); err != nil {
		t.Fatal(err)
	}
	if events := modeEvents(t, b, uid); len(events) != 1 || events[0]["yolo"] != "false" {
		t.Fatalf("plain audit %+v", events)
	}
	if launches := eventsNamed(t, b, uid, "yolo-launch"); len(launches) != 0 {
		t.Fatalf("plain switch recorded a YOLO launch %+v", launches)
	}

	yolo := switchChat(t, b, uid, agent)
	if err := b.st.SetSetting("terminal_allow_yolo", "1"); err != nil {
		t.Fatal(err)
	}
	if err := b.st.SetTerminalYoloFor(uid, yolo.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := b.SwitchSessionMode(t.Context(), uid, yolo.ID, terminalproto.SessionModeTerminal, false); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range modeEvents(t, b, uid) {
		if e["session"] == yolo.ID && e["yolo"] == "true" {
			found = true
		}
	}
	if !found {
		t.Fatalf("YOLO switch audit %+v", modeEvents(t, b, uid))
	}
	launches := eventsNamed(t, b, uid, "yolo-launch")
	if len(launches) != 1 || launches[0]["machine"] != agent || launches[0]["tool"] != "opencode" || launches[0]["session"] != yolo.ID {
		t.Fatalf("yolo-launch events %+v", launches)
	}
	if err := b.SwitchSessionMode(t.Context(), uid, yolo.ID, terminalproto.SessionModeChat, true); err != nil {
		t.Fatal(err)
	}
	// The account no longer allows YOLO: refused before the machine is asked.
	if err := b.st.SetSetting("terminal_allow_yolo", "0"); err != nil {
		t.Fatal(err)
	}
	before := len(machine.sent())
	if err := b.SwitchSessionMode(t.Context(), uid, yolo.ID, terminalproto.SessionModeTerminal, false); !errors.Is(err, ErrYoloNotAllowed) {
		t.Fatalf("err %v", err)
	}
	if len(machine.sent()) != before || !row(t, b, uid, yolo.ID).Structured || !b.chatExists(uid, yolo.ID) {
		t.Fatal("a refused YOLO switch reached the machine or changed the row")
	}
	if launches := eventsNamed(t, b, uid, "yolo-launch"); len(launches) != 1 {
		t.Fatalf("yolo-launch events after refusal %+v", launches)
	}
}

// C5: a machine that cannot launch the row's tool in a terminal is not asked.
func TestSwitchNeedsTheToolAmongLaunchTools(t *testing.T) {
	b, _, uid, agent, machine := switchFixtureCaps(t, func(c *terminalproto.Capabilities) {
		c.SessionMode = true
		c.Tools = []string{"claude", "opencode"}
	})
	s := switchChat(t, b, uid, agent)
	// The machine reports again, now without OpenCode among its launch tools
	// (an older connector still claims session-mode-v1 then).
	caps, err := b.st.RemoteCapabilitiesFor(uid, agent)
	if err != nil {
		t.Fatal(err)
	}
	next := caps.Capabilities
	next.Tools = []string{"claude"}
	if err := b.st.UpdateRemoteCapabilitiesFor(uid, agent, caps.InstanceID, caps.Generation, next); err != nil {
		t.Fatal(err)
	}
	if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false); !errors.Is(err, ErrSessionModeUnsupported) {
		t.Fatalf("err %v", err)
	}
	if len(machine.sent()) != 0 || !row(t, b, uid, s.ID).Structured {
		t.Fatal("the machine was asked or the row changed")
	}
}

// C6: a late running-with-id answer is not adopted while a switch of that
// session is in flight; the switch itself settles the row.
func TestLateChatReplyWaitsForAnInFlightSwitch(t *testing.T) {
	b, p, uid, agent, machine := switchFixture(t, true)
	s := switchChat(t, b, uid, agent)
	other := switchChat(t, b, uid, agent)
	for _, id := range []string{s.ID, other.ID} {
		if err := b.SwitchSessionMode(t.Context(), uid, id, terminalproto.SessionModeTerminal, false); err != nil {
			t.Fatal(err)
		}
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	setMode(machine, func(p *Peer, m terminalproto.Message, in *terminalproto.SessionMode) terminalproto.Body {
		go func() {
			close(entered)
			<-release
			body := terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: in.ACPSessionID}
			if err := p.Receive(terminalproto.Message{Version: 1, RequestID: m.RequestID, SessionID: m.SessionID, Generation: p.Generation, Body: &body}); err != nil {
				t.Error(err)
			}
		}()
		return nil
	})
	done := make(chan error, 1)
	go func() {
		done <- b.SwitchSessionMode(context.Background(), uid, s.ID, terminalproto.SessionModeChat, true)
	}()
	<-entered
	late := func(session string) {
		body := terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: switchACPID}
		if err := p.Receive(terminalproto.Message{Version: 1, RequestID: ID(), SessionID: session, Generation: p.Generation, Body: &body}); err != nil {
			t.Fatal(err)
		}
	}
	late(s.ID)
	// The observation loop is FIFO: once the other row settles, the first
	// answer has been read.
	late(other.ID)
	if !eventually(t, func() bool { return row(t, b, uid, other.ID).Structured }) {
		close(release)
		t.Fatal("the other row was not adopted")
	}
	adopted := row(t, b, uid, s.ID).Structured
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if adopted {
		t.Fatal("a late answer was adopted while the switch was in flight")
	}
	if got := row(t, b, uid, s.ID); !got.Structured || !b.chatExists(uid, s.ID) {
		t.Fatalf("switch did not settle the row %+v", got)
	}
}

// C7b: a chat without a recorded OpenCode session id cannot open in the
// OpenCode TUI, and that is not an outdated machine.
func TestSwitchToTerminalWithoutConversation(t *testing.T) {
	b, _, uid, agent, machine := switchFixture(t, true)
	machine.mu.Lock()
	machine.noChatID = true
	machine.mu.Unlock()
	s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Structured || s.ACPSessionID != "" {
		t.Fatalf("session %+v", s)
	}
	err = b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false)
	if !errors.Is(err, ErrSessionModeNoConversation) || err.Error() != "This session has no saved OpenCode conversation yet, so it cannot move to the OpenCode TUI." {
		t.Fatalf("err %v", err)
	}
	if len(machine.sent()) != 0 || !row(t, b, uid, s.ID).Structured {
		t.Fatal("refused switch reached the machine or changed the row")
	}
}

// C7a: a switch to the mode the row is already in changes and records nothing.
func TestSwitchToCurrentModeIsANoOp(t *testing.T) {
	b, _, uid, agent, machine := switchFixture(t, true)
	s := switchChat(t, b, uid, agent)
	if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeChat, false); err != nil {
		t.Fatal(err)
	}
	if len(machine.sent()) != 0 || len(modeEvents(t, b, uid)) != 0 || !row(t, b, uid, s.ID).Structured {
		t.Fatal("a no-op switch reached the machine, was audited or changed the row")
	}
}

// C8: a reply that never comes (timed out, or the connection went) leaves the
// row as it was and drops the ring made for the replay.
func TestSwitchLostReplyLeavesRowAndRing(t *testing.T) {
	old := sessionModeCallTimeout
	sessionModeCallTimeout = 50 * time.Millisecond
	t.Cleanup(func() { sessionModeCallTimeout = old })
	for _, tc := range []struct {
		name  string
		close bool
	}{{"timed out", false}, {"connection lost", true}} {
		t.Run(tc.name, func(t *testing.T) {
			b, _, uid, agent, machine := switchFixture(t, true)
			s := switchChat(t, b, uid, agent)
			// To the terminal: the chat and its ring stay.
			setMode(machine, func(p *Peer, _ terminalproto.Message, _ *terminalproto.SessionMode) terminalproto.Body {
				if tc.close {
					go p.Close()
				}
				return nil
			})
			if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false); !errors.Is(err, ErrOffline) {
				t.Fatalf("to terminal: %v", err)
			}
			if got := row(t, b, uid, s.ID); !got.Structured || got.State != terminalproto.Running || !b.chatExists(uid, s.ID) {
				t.Fatalf("to terminal changed the row or dropped the ring %+v", got)
			}
			if len(modeEvents(t, b, uid)) != 0 {
				t.Fatal("an unanswered switch was audited")
			}
		})
	}
	// To chat: the row stays a terminal and the replay ring is removed.
	for _, tc := range []struct {
		name  string
		close bool
	}{{"timed out", false}, {"connection lost", true}} {
		t.Run("to chat "+tc.name, func(t *testing.T) {
			b, _, uid, agent, machine := switchFixture(t, true)
			s := switchChat(t, b, uid, agent)
			sessionModeCallTimeout = old
			if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false); err != nil {
				t.Fatal(err)
			}
			sessionModeCallTimeout = 50 * time.Millisecond
			setMode(machine, func(p *Peer, m terminalproto.Message, _ *terminalproto.SessionMode) terminalproto.Body {
				receive(t, p, m.SessionID, terminalproto.ACPEvent{Kind: "agent", MessageID: "msg_1", Text: "replayed"})
				if tc.close {
					go p.Close()
				}
				return nil
			})
			if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeChat, true); !errors.Is(err, ErrOffline) {
				t.Fatalf("to chat: %v", err)
			}
			if got := row(t, b, uid, s.ID); got.Structured || got.State != terminalproto.Running {
				t.Fatalf("to chat changed the row %+v", got)
			}
			if b.chatExists(uid, s.ID) {
				t.Fatal("ring made for the replay left behind")
			}
		})
	}
}
