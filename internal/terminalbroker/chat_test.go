package terminalbroker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func chatFixture(t *testing.T) (*Broker, *Peer, int64, string) {
	t.Helper()
	return chatFixtureWith(t, nil)
}

// chatFixtureWith lets a test script the machine's reply to a question answer.
func chatFixtureWith(t *testing.T, answerReply func(*terminalproto.ACPQuestionAnswer) terminalproto.Result) (*Broker, *Peer, int64, string) {
	t.Helper()
	return chatFixtureCaps(t, answerReply, nil)
}

// chatFixtureCaps also lets a test change the machine's capabilities before it connects.
func chatFixtureCaps(t *testing.T, answerReply func(*terminalproto.ACPQuestionAnswer) terminalproto.Result, mutate func(*terminalproto.Capabilities)) (*Broker, *Peer, int64, string) {
	t.Helper()
	return chatFixtureFull(t, answerReply, mutate, nil)
}

// chatFixtureFull also lets a test script the machine's reply to an agent mode switch.
func chatFixtureFull(t *testing.T, answerReply func(*terminalproto.ACPQuestionAnswer) terminalproto.Result, mutate func(*terminalproto.Capabilities), modeReply func(*terminalproto.ACPAgentMode) (terminalproto.Result, bool)) (*Broker, *Peer, int64, string) {
	t.Helper()
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	uid, _ := st.DefaultOwner()
	a, _, err := st.EnrollSyncAgent(uid, "chat", "test")
	if err != nil {
		t.Fatal(err)
	}
	caps := terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", HealthEnabled: true, TerminalEnabled: true, TerminalControl: true, Launch: true, Tools: []string{"opencode"}, Readiness: terminalproto.Ready, ACP: true, ACPHistory: true, ACPApproval: true, ACPModel: true}
	if mutate != nil {
		mutate(&caps)
	}
	record, err := st.BeginRemoteConnectionFor(uid, a.ID, ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	b := New(st)
	t.Cleanup(b.Close)
	var p *Peer
	var mu sync.Mutex
	sessions := map[string]terminalproto.SessionSnapshot{}
	p, err = b.Connect(uid, a.ID, record.Generation, true, func(ctx context.Context, m terminalproto.Message) error {
		reply := m
		mu.Lock()
		switch op := m.Body.(type) {
		case terminalproto.Reconcile:
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
		case terminalproto.Create:
			s := terminalproto.SessionSnapshot{ID: m.SessionID, Title: op.Title, State: terminalproto.Running}
			sessions[s.ID] = s
			reply.Body = terminalproto.SessionChanged{State: terminalproto.Running}
			if op.Structured {
				reply.Body = terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: "ses_fake000000000000000000000001"}
			}
		case terminalproto.End:
			delete(sessions, m.SessionID)
			reply.Body = terminalproto.SessionChanged{State: terminalproto.Ended}
		case *terminalproto.ACPQuestionAnswer:
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
			if answerReply != nil {
				reply.Body = answerReply(op)
			}
		case *terminalproto.ACPAgentMode:
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
			if modeReply != nil {
				result, deliver := modeReply(op)
				if !deliver {
					// The machine got the request and its reply never arrives.
					mu.Unlock()
					return nil
				}
				reply.Body = result
			}
		case terminalproto.ACPPrompt, terminalproto.ACPCancel, terminalproto.ACPPermission:
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
		default:
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
		}
		mu.Unlock()
		data, err := terminalproto.Encode(reply)
		if err != nil {
			return err
		}
		decoded, err := terminalproto.Decode(data)
		if err != nil {
			return err
		}
		return p.Receive(decoded)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return b, p, uid, a.ID
}

func chatSession(t *testing.T, b *Broker, uid int64, agent string) store.TerminalSession {
	t.Helper()
	s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Structured || s.ACPSessionID != "ses_fake000000000000000000000001" {
		t.Fatalf("session %+v", s)
	}
	return s
}

func TestChatPromptPublishesUserAndAcceptsMachineEvent(t *testing.T) {
	b, p, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	snapshot, _, sub, cancel := b.SubscribeChat(uid, s.ID)
	defer cancel()
	if len(snapshot) != 0 {
		t.Fatalf("new chat has %d events", len(snapshot))
	}
	if err := b.PromptChat(t.Context(), uid, s.ID, "hello", nil); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-sub:
		if event.Kind != "user" || event.Text != "hello" {
			t.Fatalf("event %+v", event)
		}
	default:
		t.Fatal("no user event")
	}
	if err := p.Receive(terminalproto.Message{Version: 1, RequestID: ID(), SessionID: s.ID, Generation: p.Generation,
		Body: &terminalproto.ACPEvent{Kind: "agent", MessageID: "msg_1", Text: "hi"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-sub:
		if event.Kind != "agent" || event.Text != "hi" || event.Seq != 2 {
			t.Fatalf("event %+v", event)
		}
	default:
		t.Fatal("no agent event")
	}
}

func TestChatRejectsPlainSessionsAndOtherAccounts(t *testing.T) {
	b, _, uid, agent := chatFixture(t)
	plain, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.PromptChat(t.Context(), uid, plain.ID, "hello", nil); !errors.Is(err, ErrACPUnsupported) {
		t.Fatalf("plain: %v", err)
	}
	s := chatSession(t, b, uid, agent)
	if err := b.PromptChat(t.Context(), uid+1, s.ID, "hello", nil); err == nil {
		t.Fatal("other account accepted")
	}
}

func TestChatRingOverflowAndReset(t *testing.T) {
	b, p, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	for i := 0; i < maxChatEvents+10; i++ {
		_ = p.Receive(terminalproto.Message{Version: 1, RequestID: ID(), SessionID: s.ID, Generation: p.Generation,
			Body: &terminalproto.ACPEvent{Kind: "notice", Text: "n"}})
	}
	snapshot, truncated, _, cancel := b.SubscribeChat(uid, s.ID)
	defer cancel()
	if !truncated || len(snapshot) > maxChatEvents || len(snapshot) == 0 {
		t.Fatalf("ring: %d truncated=%v", len(snapshot), truncated)
	}
}

func TestChatResumeMustBelongToAClosedChat(t *testing.T) {
	b, _, uid, agent := chatFixture(t)
	first := chatSession(t, b, uid, agent)
	if _, err := b.End(t.Context(), uid, first.ID, ID()); err != nil {
		t.Fatal(err)
	}
	second, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Again", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: "/tmp", ResumeACP: first.ACPSessionID})
	if err != nil {
		t.Fatal(err)
	}
	if second.ACPSessionID != first.ACPSessionID {
		t.Fatalf("resume id %q", second.ACPSessionID)
	}
	if _, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Crafted", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: "/tmp", ResumeACP: "ses_nosuch000000000000000000001"}); !errors.Is(err, ErrACPResumeNotFound) {
		t.Fatalf("crafted resume: %v", err)
	}
}

func TestPermissionKeyIsNarrow(t *testing.T) {
	if got := permissionKey(terminalproto.ACPEvent{ToolKind: "execute", Command: "npm test", Path: "/a"}); got != "execute\x00npm test" {
		t.Fatalf("command key %q", got)
	}
	if got := permissionKey(terminalproto.ACPEvent{ToolKind: "edit", Path: "/a"}); got != "edit\x00/a" {
		t.Fatalf("path key %q", got)
	}
	if permissionKey(terminalproto.ACPEvent{ToolKind: "edit"}) != "" || permissionKey(terminalproto.ACPEvent{Command: "x"}) != "" {
		t.Fatal("unmatchable card remembered")
	}
}

func TestSetChatYoloChecksGatesAndTurnsOff(t *testing.T) {
	b, _, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	if err := b.SetChatYolo(t.Context(), uid, s.ID, true); !errors.Is(err, ErrYoloNotAllowed) {
		t.Fatalf("account gate: %v", err)
	}
	if err := b.st.SetSetting("terminal_allow_yolo", "1"); err != nil {
		t.Fatal(err)
	}
	if err := b.SetChatYolo(t.Context(), uid, s.ID, true); !errors.Is(err, ErrYoloRefused) {
		t.Fatalf("machine gate: %v", err)
	}
	if err := b.SetChatYolo(t.Context(), uid, s.ID, false); err != nil {
		t.Fatalf("turning off: %v", err)
	}
}

func TestChatCloseClosesSubscribers(t *testing.T) {
	b, _, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	_, _, sub, cancel := b.SubscribeChat(uid, s.ID)
	defer cancel()
	b.closeChat(uid, s.ID)
	if _, ok := <-sub; ok {
		t.Fatal("subscriber channel stayed open")
	}
}

func TestPreviewTextBoundsAndFlagsTruncation(t *testing.T) {
	got, truncated := previewText("short command", 20)
	if got != "short command" || truncated {
		t.Fatalf("short command: got %q, truncated=%v", got, truncated)
	}

	got, truncated = previewText("1234567890abcdef", 10)
	if got != "1234567890…" || !truncated {
		t.Fatalf("truncated command: got %q, truncated=%v", got, truncated)
	}

	// Rune boundary handling: ensure multi-byte character is not sliced midway.
	text := "hello 日本語 world"
	// Slicing midway through '日' (at index 8, where '日' starts at index 6 and takes 3 bytes)
	got, truncated = previewText(text, 8)
	if got != "hello …" || !truncated {
		t.Fatalf("rune boundary command: got %q, truncated=%v", got, truncated)
	}
}

func TestPermissionChatRemembersACommandForTheSession(t *testing.T) {
	b, p, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	first := terminalproto.ACPEvent{Kind: "permission", PermissionID: "permission-0123456789", ToolKind: "execute", Command: "go test ./..."}
	if err := p.Receive(terminalproto.Message{Version: 1, RequestID: ID(), SessionID: s.ID, Generation: p.Generation, Body: &first}); err != nil {
		t.Fatal(err)
	}
	if err := b.PermissionChat(t.Context(), uid, s.ID, first.PermissionID, true, true); err != nil {
		t.Fatal(err)
	}
	second := terminalproto.ACPEvent{Kind: "permission", PermissionID: "permission-9876543210", ToolKind: "execute", Command: "go test ./..."}
	if err := p.Receive(terminalproto.Message{Version: 1, RequestID: ID(), SessionID: s.ID, Generation: p.Generation, Body: &second}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		events, err := b.st.ListEventsFor(uid, 100, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if event.Action == "session-allow" {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("a repeated command was not answered and audited for the session")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestChatModelSwitchKeepsHistoryAndRecordsModel(t *testing.T) {
	b, _, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	if err := b.PromptChat(t.Context(), uid, s.ID, "hello", nil); err != nil {
		t.Fatal(err)
	}
	if err := b.ModelChat(t.Context(), uid, s.ID, "provider/other"); err != nil {
		t.Fatal(err)
	}
	updated, err := b.st.TerminalSessionFor(uid, s.ID)
	if err != nil || updated.Model != "provider/other" || updated.ACPSessionID != s.ACPSessionID {
		t.Fatalf("updated session: %+v %v", updated, err)
	}
	history, _, _, cancel := b.SubscribeChat(uid, s.ID)
	defer cancel()
	if len(history) != 1 || history[0].Text != "hello" {
		t.Fatalf("history lost: %+v", history)
	}
	if err := b.ModelChat(t.Context(), uid+1, s.ID, "provider/other"); err == nil {
		t.Fatal("other user switched model")
	}
}

func TestStructuredCreateHonoursExplicitDescriptorRefusal(t *testing.T) {
	for _, state := range []string{"missing", "unverified", "incompatible", "empty"} {
		t.Run(state, func(t *testing.T) {
			b, p, uid, agent := chatFixture(t)
			record, err := b.st.RemoteCapabilitiesFor(uid, agent)
			if err != nil {
				t.Fatal(err)
			}
			entries := []terminalproto.ACPAgent{}
			if state != "empty" {
				entries = append(entries, terminalproto.ACPAgent{Tool: "opencode", Adapter: "opencode", MinimumVersion: "1.19.5", State: state})
			}
			caps := record.Capabilities
			caps.ACPAgents = &entries
			if err := b.st.UpdateRemoteCapabilitiesFor(uid, agent, record.InstanceID, p.Generation, caps); err != nil {
				t.Fatal(err)
			}
			_, _, err = b.CreateSession(context.Background(), uid, agent, ID(), terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true})
			if !errors.Is(err, ErrACPUnsupported) {
				t.Fatalf("descriptor refusal: %v", err)
			}
		})
	}
}

// A mode switch goes to the machine only for a mode the agent's descriptor lists, on a
// machine that negotiated acp-agent-mode-v1; anything else is refused before it is sent.
func TestSetAgentModeIsLimitedToTheDescriptorsModes(t *testing.T) {
	b, _, uid, agent := chatFixtureCaps(t, nil, func(c *terminalproto.Capabilities) {
		c.ACPAgentMode = true
		agents := []terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5", Model: true, Approval: true, Modes: []string{"read-only", "agent"}}}
		c.ACPAgents = &agents
	})
	s := chatSession(t, b, uid, agent)
	if err := b.SetAgentMode(t.Context(), uid, s.ID, "agent"); err != nil {
		t.Fatalf("a listed mode: %v", err)
	}
	if err := b.SetAgentMode(t.Context(), uid, s.ID, "agent-full-access"); !errors.Is(err, ErrACPAgentModeUnsupported) {
		t.Fatalf("a mode the descriptor does not list: %v", err)
	}
	if err := b.SetAgentMode(t.Context(), uid, s.ID, "Not A Mode"); !errors.Is(err, terminalproto.ErrInvalid) {
		t.Fatalf("an invalid mode name: %v", err)
	}
	if err := b.SetAgentMode(t.Context(), uid+1, s.ID, "agent"); err == nil {
		t.Fatal("another user switched the mode")
	}
}

// Without the negotiated feature and a descriptor that lists modes there is no switch.
func TestSetAgentModeNeedsTheFeatureAndAListedMode(t *testing.T) {
	b, _, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	if err := b.SetAgentMode(t.Context(), uid, s.ID, "agent"); !errors.Is(err, ErrACPAgentModeUnsupported) {
		t.Fatalf("a machine that lists no modes: %v", err)
	}
}

// The stored mode is what the browser will show after a reload: the first mode the
// descriptor lists when the chat opens, the new mode after a confirmed switch, and
// unknown (empty) after a switch the machine did not confirm.
func TestAgentModeIsStoredAtCreateAndOnConfirmedSwitches(t *testing.T) {
	refuse := false
	b, _, uid, agent := chatFixtureFull(t, nil, func(c *terminalproto.Capabilities) {
		c.ACPAgentMode = true
		agents := []terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5", Model: true, Approval: true, Modes: []string{"read-only", "agent"}}}
		c.ACPAgents = &agents
	}, func(*terminalproto.ACPAgentMode) (terminalproto.Result, bool) {
		if refuse {
			return terminalproto.Result{Code: terminalproto.Unavailable}, true
		}
		return terminalproto.Result{Code: terminalproto.OK}, true
	})
	s := chatSession(t, b, uid, agent)
	mode := func() string {
		got, err := b.st.TerminalSessionFor(uid, s.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got.AgentMode
	}
	if got := mode(); got != "read-only" {
		t.Fatalf("mode at create %q", got)
	}
	if err := b.SetAgentMode(t.Context(), uid, s.ID, "agent"); err != nil || mode() != "agent" {
		t.Fatalf("after a confirmed switch: %v %q", err, mode())
	}
	refuse = true
	if err := b.SetAgentMode(t.Context(), uid, s.ID, "read-only"); !errors.Is(err, ErrACPAgentModeRejected) {
		t.Fatalf("an unconfirmed switch: %v", err)
	}
	if got := mode(); got != "" {
		t.Fatalf("an unconfirmed switch left the stored mode as %q, not unknown", got)
	}
}

// An agent that lists no modes has no stored mode.
func TestNoModeIsStoredWhenTheAgentOffersNone(t *testing.T) {
	b, _, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	got, err := b.st.TerminalSessionFor(uid, s.ID)
	if err != nil || got.AgentMode != "" {
		t.Fatalf("stored mode %q %v", got.AgentMode, err)
	}
}

// A switch that was sent but whose reply never arrives may have been applied, so the
// stored mode becomes unknown rather than keeping the old one as if it were confirmed.
func TestLostModeSwitchReplyClearsTheStoredMode(t *testing.T) {
	old := chatCallTimeout
	chatCallTimeout = 150 * time.Millisecond
	defer func() { chatCallTimeout = old }()
	drop := false
	b, _, uid, agent := chatFixtureFull(t, nil, func(c *terminalproto.Capabilities) {
		c.ACPAgentMode = true
		agents := []terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5", Model: true, Approval: true, Modes: []string{"read-only", "agent"}}}
		c.ACPAgents = &agents
	}, func(*terminalproto.ACPAgentMode) (terminalproto.Result, bool) {
		return terminalproto.Result{Code: terminalproto.OK}, !drop
	})
	s := chatSession(t, b, uid, agent)
	stored := func() string {
		got, err := b.st.TerminalSessionFor(uid, s.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got.AgentMode
	}
	if got := stored(); got != "read-only" {
		t.Fatalf("mode at create %q", got)
	}
	drop = true
	if err := b.SetAgentMode(t.Context(), uid, s.ID, "agent"); err == nil {
		t.Fatal("a switch with no reply was reported as done")
	}
	if got := stored(); got != "" {
		t.Fatalf("a lost reply left the stored mode as %q, not unknown", got)
	}
}

// While an agent that offers modes is in an unknown mode, a message is refused before it is sent; choosing a
// mode lifts the refusal. An agent that offers no modes is never held up.
func TestPromptIsRefusedWhileTheAgentModeIsUnknown(t *testing.T) {
	b, _, uid, agent := chatFixtureFull(t, nil, func(c *terminalproto.Capabilities) {
		c.ACPAgentMode = true
		agents := []terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5", Model: true, Approval: true, Modes: []string{"read-only", "agent"}}}
		c.ACPAgents = &agents
	}, nil)
	s := chatSession(t, b, uid, agent)
	if err := b.PromptChat(t.Context(), uid, s.ID, "hello", nil); err != nil {
		t.Fatalf("a message in a known mode: %v", err)
	}
	if err := b.st.RecordTerminalAgentModeFor(uid, s.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := b.PromptChat(t.Context(), uid, s.ID, "again", nil); !errors.Is(err, ErrACPModeUnknown) {
		t.Fatalf("a message while the mode is unknown: %v", err)
	}
	if err := b.SetAgentMode(t.Context(), uid, s.ID, "read-only"); err != nil {
		t.Fatal(err)
	}
	if err := b.PromptChat(t.Context(), uid, s.ID, "again", nil); err != nil {
		t.Fatalf("a message after choosing a mode: %v", err)
	}
	plain, _, puid, pagent := chatFixture(t)
	ps := chatSession(t, plain, puid, pagent)
	if err := plain.PromptChat(t.Context(), puid, ps.ID, "hello", nil); err != nil {
		t.Fatalf("an agent that offers no modes was held up: %v", err)
	}
}
