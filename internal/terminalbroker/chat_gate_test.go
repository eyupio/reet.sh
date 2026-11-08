package terminalbroker

import (
	"errors"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A person who turns a gated chat off closes the chats already running with it: they stop taking messages, answers and
// changes for one clear reason, instead of half-working with refusals that blame the machine. Stopping one still works.
func TestTurningAGatedChatOffClosesRunningChats(t *testing.T) {
	b, _, uid, agent := chatFixtureScript(t, nil, func(c *terminalproto.Capabilities) {
		c.Tools = []string{"claude", "opencode"}
		c.AgentsV2 = true
		agents := []terminalproto.ACPAgent{{Tool: "claude", Adapter: "claude-agent-acp", State: "available", MinimumVersion: "0.99.1", Model: true, Approval: true}}
		c.ACPAgents = &agents
	}, nil, func(m terminalproto.Message) (terminalproto.Body, bool) {
		// Claude names its sessions with UUIDs, not the OpenCode-shaped id the fixture answers a create with.
		if _, ok := m.Body.(terminalproto.Create); ok {
			return terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: "5d0a3b57-7c2e-4f58-9d1b-3b8a2c1e9f40"}, true
		}
		return nil, true
	})
	setChats := func(on bool) {
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
