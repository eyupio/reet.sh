package terminalbroker

import (
	"errors"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// claudeChatFixture is a machine with Claude Code installed and reporting a chat, and a way to flip the account's switch.
func claudeChatFixture(t *testing.T) (b *Broker, uid int64, agent string, setChats func(bool)) {
	t.Helper()
	b, _, uid, agent = chatFixtureScript(t, nil, func(c *terminalproto.Capabilities) {
		c.Tools = []string{"claude", "opencode"}
		c.AgentsV2 = true
		c.ChatReattach, c.ACPAgentReattach = true, true
		agents := []terminalproto.ACPAgent{{Tool: "claude", Adapter: "claude-agent-acp", State: "available", MinimumVersion: "0.99.1", Model: true, Approval: true, History: true, Reattach: true}}
		c.ACPAgents = &agents
	}, nil, func(m terminalproto.Message) (terminalproto.Body, bool) {
		// Claude names its sessions with UUIDs, not the OpenCode-shaped id the fixture answers a create with.
		switch m.Body.(type) {
		case terminalproto.Create, *terminalproto.ChatReattach:
			return terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: "5d0a3b57-7c2e-4f58-9d1b-3b8a2c1e9f40"}, true
		}
		return nil, true
	})
	setChats = func(on bool) {
		t.Helper()
		profile, err := b.st.ProfileFor(uid)
		if err != nil {
			t.Fatal(err)
		}
		profile.ChatAgents = map[string]bool{"claude": on}
		if err := b.st.SaveProfileFor(uid, profile); err != nil {
			t.Fatal(err)
		}
	}
	return b, uid, agent, setChats
}

// A person who turns a gated chat off closes the chats already running with it: they stop taking messages, answers and
// changes for one clear reason, instead of half-working with refusals that blame the machine. Stopping one still works.
func TestTurningAGatedChatOffClosesRunningChats(t *testing.T) {
	b, uid, agent, setChats := claudeChatFixture(t)
	setChats(true)
	s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "claude", Structured: true, Trust: true, Dir: "/tmp"})
	if err != nil {
		t.Fatalf("a chat with the switch on: %v", err)
	}
	if err := b.PromptChat(t.Context(), uid, s.ID, "hello", nil); err != nil {
		t.Fatalf("a message with the switch on: %v", err)
	}

	setChats(false)
	for name, err := range map[string]error{
		"prompt":     b.PromptChat(t.Context(), uid, s.ID, "hello again", nil),
		"permission": b.PermissionChat(t.Context(), uid, s.ID, "perm-1", true, false),
		"model":      b.ModelChat(t.Context(), uid, s.ID, "claude-opus"),
		"mode":       b.SetAgentMode(t.Context(), uid, s.ID, "agent"),
	} {
		if !errors.Is(err, ErrChatOff) {
			t.Errorf("%s with the switch off: %v, want ErrChatOff", name, err)
		}
	}
	if err := b.CancelChat(t.Context(), uid, s.ID); errors.Is(err, ErrChatOff) {
		t.Errorf("stopping a running chat was refused because its switch is off: %v", err)
	}
	if _, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "claude", Structured: true, Trust: true, Dir: "/tmp"}); !errors.Is(err, ErrChatOff) {
		t.Errorf("a new chat with the switch off: %v, want ErrChatOff", err)
	}
}

// A closed Claude Code chat continues with its history in its own folder, on its own machine and for its own account,
// and for nothing else: the binding is the only guard, because the adapter itself does not tie history to a folder.
func TestContinueClaudeWithHistoryIsBoundToItsFolder(t *testing.T) {
	b, uid, agent, setChats := claudeChatFixture(t)
	setChats(true)
	first, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "claude", Structured: true, Trust: true, Dir: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.End(t.Context(), uid, first.ID, ID()); err != nil {
		t.Fatal(err)
	}
	resume := func(user int64, machine, dir string) error {
		_, err := b.Create(t.Context(), user, machine, ID(), terminalproto.Create{Title: "Again", Cols: 100, Rows: 30, Tool: "claude", Structured: true, Trust: true, Dir: dir, ResumeACP: first.ACPSessionID})
		return err
	}
	if err := resume(uid, agent, "/tmp"); err != nil {
		t.Fatalf("continue in its own folder: %v", err)
	}
	for name, err := range map[string]error{
		"another folder":  resume(uid, agent, "/var"),
		"another machine": resume(uid, ID(), "/tmp"),
		"another account": resume(uid+1, agent, "/tmp"),
	} {
		if !errors.Is(err, ErrACPResumeNotFound) {
			t.Errorf("continue for %s: %v, want ErrACPResumeNotFound", name, err)
		}
	}
	// With the account's switch off the refusal names the setting, not the machine.
	setChats(false)
	if err := resume(uid, agent, "/tmp"); !errors.Is(err, ErrChatOff) {
		t.Errorf("continue with the switch off: %v, want ErrChatOff", err)
	}
}

// A lost Claude Code chat reconnects under its own session id, and not when the account has turned Claude off.
func TestALostClaudeChatReconnectsInPlace(t *testing.T) {
	b, uid, agent, setChats := claudeChatFixture(t)
	setChats(true)
	chat, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "claude", Structured: true, Trust: true, Dir: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	record, err := b.st.RemoteCapabilitiesRawFor(uid, agent)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.st.ObserveTerminalStateFor(uid, agent, chat.ID, record.Generation, terminalproto.Lost, nil); err != nil {
		t.Fatal(err)
	}
	setChats(false)
	if err := b.ReattachChat(t.Context(), uid, chat.ID, true); !errors.Is(err, ErrChatOff) {
		t.Fatalf("a reconnect with the switch off: %v, want ErrChatOff", err)
	}
	setChats(true)
	if err := b.ReattachChat(t.Context(), uid, chat.ID, true); err != nil {
		t.Fatalf("a lost Claude chat did not reconnect: %v", err)
	}
	if got, err := b.st.TerminalSessionFor(uid, chat.ID); err != nil || got.State != terminalproto.Running {
		t.Fatalf("after the reconnect: %+v %v", got, err)
	}
}

// Claude Code's chat takes YOLO through the same three gates as every other agent: the account's Allow YOLO setting, the
// machine's switch, and turning it off is always allowed.
func TestClaudeChatYoloHitsTheSameGates(t *testing.T) {
	b, uid, agent, setChats := claudeChatFixture(t)
	setChats(true)
	chat, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "claude", Structured: true, Trust: true, Dir: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.SetChatYolo(t.Context(), uid, chat.ID, true); !errors.Is(err, ErrYoloNotAllowed) {
		t.Fatalf("account setting off: %v, want ErrYoloNotAllowed", err)
	}
	if err := b.st.SetSetting("terminal_allow_yolo", "1"); err != nil {
		t.Fatal(err)
	}
	if err := b.SetChatYolo(t.Context(), uid, chat.ID, true); !errors.Is(err, ErrYoloRefused) {
		t.Fatalf("machine switch off: %v, want ErrYoloRefused", err)
	}
	if err := b.SetChatYolo(t.Context(), uid, chat.ID, false); err != nil {
		t.Fatalf("turning it off: %v", err)
	}
}
