package terminalbroker

import (
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// fallbackChat reserves a structured chat the way the create handler does for
// a launch with no prompt: its title is the generic fallback.
func fallbackChat(t *testing.T, b *Broker, uid int64, agent string) store.TerminalSession {
	t.Helper()
	s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "OpenCode · app", TitleOrigin: terminalproto.TitleFallback, Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := b.st.TerminalTitleOriginFor(uid, s.ID); err != nil || got != terminalproto.TitleFallback {
		t.Fatalf("reserved origin %q (%v), want fallback", got, err)
	}
	return s
}

func titleOf(t *testing.T, b *Broker, uid int64, session string) string {
	t.Helper()
	s, err := b.st.TerminalSessionFor(uid, session)
	if err != nil {
		t.Fatal(err)
	}
	return s.Title
}

func TestPromptChatRetitlesFallbackChatOnce(t *testing.T) {
	b, _, uid, agent := chatFixture(t)
	s := fallbackChat(t, b, uid, agent)
	if err := b.PromptChat(t.Context(), uid, s.ID, "hi", nil); err != nil {
		t.Fatal(err)
	}
	if got := titleOf(t, b, uid, s.ID); got != "OpenCode · app" {
		t.Fatalf("a greeting renamed the chat to %q", got)
	}
	if err := b.PromptChat(t.Context(), uid, s.ID, "Please fix session end navigation", nil); err != nil {
		t.Fatal(err)
	}
	if got := titleOf(t, b, uid, s.ID); got != "Fix session end navigation" {
		t.Fatalf("title %q, want the task", got)
	}
	if err := b.PromptChat(t.Context(), uid, s.ID, "Now add a dark mode toggle to settings", nil); err != nil {
		t.Fatal(err)
	}
	if got := titleOf(t, b, uid, s.ID); got != "Fix session end navigation" {
		t.Fatalf("a later prompt renamed the chat to %q", got)
	}
}

// A message the agent did not accept is not the first task, so it neither
// renames the chat nor spends one of its naming tries.
func TestPromptChatFailedSendDoesNotRetitle(t *testing.T) {
	refuse := true
	b, _, uid, agent := chatFixtureScript(t, nil, nil, nil, func(m terminalproto.Message) (terminalproto.Body, bool) {
		if _, ok := m.Body.(*terminalproto.ACPPrompt); ok && refuse {
			return terminalproto.Result{Code: terminalproto.Conflict}, true
		}
		return nil, true
	})
	s := fallbackChat(t, b, uid, agent)
	if err := b.PromptChat(t.Context(), uid, s.ID, "Please fix session end navigation", nil); err == nil {
		t.Fatal("the refused message was accepted")
	}
	if got := titleOf(t, b, uid, s.ID); got != "OpenCode · app" {
		t.Fatalf("a refused message renamed the chat to %q", got)
	}
	// Three tries are allowed. If the refusal had spent one, only two greetings
	// would fit before the task and the task would no longer rename the chat.
	refuse = false
	for _, text := range []string{"hi", "hey there!"} {
		if err := b.PromptChat(t.Context(), uid, s.ID, text, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.PromptChat(t.Context(), uid, s.ID, "Please fix session end navigation", nil); err != nil {
		t.Fatal(err)
	}
	if got := titleOf(t, b, uid, s.ID); got != "Fix session end navigation" {
		t.Fatalf("title %q: the refused message spent a naming try", got)
	}
}

func TestPromptChatQueueDrainRetitles(t *testing.T) {
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
	s := fallbackChat(t, b, uid, agent)
	if err := b.st.EnqueueTerminalFor(uid, s.ID, ID(), "Please fix session end navigation", nil); err != nil {
		t.Fatal(err)
	}
	b.stepChatQueue(uid, s.ID)
	if prompts != 1 {
		t.Fatalf("the queue sent %d prompts, want 1", prompts)
	}
	if got := titleOf(t, b, uid, s.ID); got != "Fix session end navigation" {
		t.Fatalf("title %q, want the queued task", got)
	}
}

func TestPromptChatNeverRetitlesPlainSession(t *testing.T) {
	b, _, uid, agent := chatFixture(t)
	plain, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Terminal", TitleOrigin: terminalproto.TitleFallback, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.PromptChat(t.Context(), uid, plain.ID, "Please fix session end navigation", nil); err == nil {
		t.Fatal("plain session accepted a chat prompt")
	}
	if got := titleOf(t, b, uid, plain.ID); got != "Terminal" {
		t.Fatalf("plain session renamed to %q", got)
	}
}

// A real task sent "now" while the agent is still replying goes through a
// steer, not a prompt. If the agent injects it, it is still the first task.
func TestSteeredQueueMessageRetitles(t *testing.T) {
	for _, tc := range []struct {
		name   string
		manual bool
		want   string
	}{
		{"fallback chat is renamed", false, "Fix session end navigation"},
		{"manual chat keeps its title", true, "Chosen by hand"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			steers, prompts := 0, 0
			b, _, uid, agent := chatFixtureScript(t, nil, func(c *terminalproto.Capabilities) { c.ACPLiveControls = true }, nil, func(m terminalproto.Message) (terminalproto.Body, bool) {
				switch op := m.Body.(type) {
				case *terminalproto.ACPControl:
					answer := terminalproto.ACPControlResult{Controls: terminalproto.ACPControls{Working: true, Steering: true, Options: []terminalproto.ACPSetting{}}}
					if op.Action == "steer" {
						steers++
						answer.Outcome = "injected"
					}
					return answer, true
				case *terminalproto.ACPPrompt:
					prompts++
				}
				return nil, true
			})
			var s store.TerminalSession
			if tc.manual {
				var err error
				s, err = b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Chosen by hand", TitleOrigin: terminalproto.TitleManual, Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: "/tmp"})
				if err != nil {
					t.Fatal(err)
				}
			} else {
				s = fallbackChat(t, b, uid, agent)
			}
			before := titleOf(t, b, uid, s.ID)
			// The first greeting is accepted normally and names nothing.
			if err := b.PromptChat(t.Context(), uid, s.ID, "hi", nil); err != nil {
				t.Fatal(err)
			}
			if got := titleOf(t, b, uid, s.ID); got != before {
				t.Fatalf("a greeting renamed the chat to %q", got)
			}
			// The real task arrives while a turn is active and is steered in.
			id := ID()
			if err := b.st.EnqueueTerminalFor(uid, s.ID, id, "Please fix session end navigation", nil); err != nil {
				t.Fatal(err)
			}
			q, _ := b.st.TerminalQueueFor(uid, s.ID)
			if err := b.st.ChangeTerminalQueueFor(uid, s.ID, id, q[0].Revision, "send", "", false); err != nil {
				t.Fatal(err)
			}
			b.stepChatQueue(uid, s.ID)
			if steers != 1 || prompts != 1 {
				t.Fatalf("steers=%d prompts=%d, want the task steered once after the one greeting prompt", steers, prompts)
			}
			if got := titleOf(t, b, uid, s.ID); got != tc.want {
				t.Fatalf("title %q, want %q", got, tc.want)
			}
		})
	}
}
