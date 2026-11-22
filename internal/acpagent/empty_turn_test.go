package acpagent

import (
	"context"
	"strings"
	"testing"
)

func emptyTurnNotices(t *testing.T, spec Spec) []Event {
	t.Helper()
	opts, got := options(t, "empty-turn", testVersion)
	opts.Agent = spec
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Prompt(context.Background(), "hello"); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	got.mu.Lock()
	defer got.mu.Unlock()
	var notices []Event
	idleAt, noticeAt := -1, -1
	for i, event := range got.all {
		if event.Kind == "notice" && event.Bad && strings.Contains(event.Text, "ended the turn without a reply") {
			notices = append(notices, event)
			noticeAt = i
		}
		if event.Kind == "status" && event.State == "idle" {
			idleAt = i
		}
	}
	if len(notices) > 0 && noticeAt > idleAt {
		t.Fatalf("the notice came after the idle status, so the chat read as done first: %+v", got.all)
	}
	return notices
}

// An agent with no way to read why a turn failed still says an empty turn was not a finished one, and points at its
// terminal instead of the provider.
func TestEmptyTurnWithoutRecoveryPointsAtTheTerminal(t *testing.T) {
	spec := OpenCode
	spec.RecoverError = nil
	notices := emptyTurnNotices(t, spec)
	if len(notices) != 1 || !strings.Contains(notices[0].Text, "restart it in the terminal") {
		t.Fatalf("notices: %+v", notices)
	}
}

// An agent whose log is read keeps the provider advice when the log has nothing to say.
func TestEmptyTurnWithRecoveryKeepsTheProviderAdvice(t *testing.T) {
	spec := OpenCode
	spec.RecoverError = func(string, string, int64) string { return "" }
	notices := emptyTurnNotices(t, spec)
	if len(notices) != 1 || !strings.Contains(notices[0].Text, "Check the model and provider") {
		t.Fatalf("notices: %+v", notices)
	}
}

// Antigravity, Claude Code and Codex have no recovery, so each gets the terminal notice.
func TestEmptyTurnNoticeCoversTheAgentsWithoutRecovery(t *testing.T) {
	for _, key := range []string{"antigravity", "claude", "codex"} {
		spec, ok := Lookup(key)
		if !ok {
			t.Fatal(key, "is not registered")
		}
		if spec.RecoverError != nil {
			t.Fatalf("%s now recovers its own turn errors; give it provider advice and drop it from this list", key)
		}
		if text := emptyTurnNotice(spec); !strings.Contains(text, spec.displayName()) || !strings.Contains(text, "terminal") {
			t.Fatalf("%s: %q", key, text)
		}
	}
}
