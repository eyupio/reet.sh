//go:build linux || darwin

package acpagent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// Claude creates a session without a login and answers "Authentication required"
// at the first prompt (measured with claude-agent-acp 0.99.1, 4 November 2026).
// That refusal has to reach the person as the sign-in step, not as a generic
// failure that looks like the agent broke.
func TestClaudeMissingLoginAtTheFirstPromptIsActionable(t *testing.T) {
	opts, got := options(t, "claude-prompt-auth-required", ClaudeCandidate.MinVersion)
	opts.Agent = ClaudeCandidate
	opts.BaseEnv = append(opts.BaseEnv, "HOME="+t.TempDir())
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer session.Close()
	if err := session.Prompt(context.Background(), "say hello"); !errors.Is(err, ErrClaudeLogin) {
		t.Fatalf("prompt: %v", err)
	}
	// The browser reads the status event, not the returned error, so the sentence
	// and the reason it stands for have to travel with the event.
	for {
		select {
		case event := <-got.wait:
			if event.Kind != "status" || event.State != "error" {
				continue
			}
			if event.Code != "login" || event.Detail != ErrClaudeLogin.Error() {
				t.Fatalf("login status event: %+v", event)
			}
			return
		case <-time.After(3 * time.Second):
			t.Fatal("no login status event reached the browser")
		}
	}
}

// Codex normally refuses at session/new, but the same refusal can arrive at the
// first prompt; it names Codex wherever it is seen.
func TestCodexMissingLoginAtTheFirstPromptIsActionable(t *testing.T) {
	opts, _ := options(t, "codex-prompt-auth-required", "1.13.1")
	opts.Agent = CodexCandidate
	opts.BaseEnv = append(opts.BaseEnv, "HOME="+t.TempDir())
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer session.Close()
	if err := session.Prompt(context.Background(), "say hello"); !errors.Is(err, ErrCodexLogin) {
		t.Fatalf("prompt: %v", err)
	}
}

// An agent without a reviewed chat login keeps its own words: reet must not
// invent a sign-in instruction for a tool it cannot start a chat with anyway.
func TestOtherAgentLoginRefusalIsNotRewritten(t *testing.T) {
	opts, _ := options(t, "prompt-auth-required", testVersion)
	opts.BaseEnv = append(opts.BaseEnv, "HOME="+t.TempDir())
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer session.Close()
	err = session.Prompt(context.Background(), "say hello")
	if err == nil || errors.Is(err, ErrClaudeLogin) || errors.Is(err, ErrCodexLogin) {
		t.Fatalf("prompt: %v", err)
	}
	if !strings.Contains(err.Error(), "Authentication required") {
		t.Fatalf("prompt: %v", err)
	}
}

// claudeOptions is the fake Claude Code adapter with Claude's profile and a home that holds no login.
func claudeOptions(t *testing.T, scenario string) (Options, *events) {
	t.Helper()
	opts, got := options(t, scenario, ClaudeCandidate.MinVersion)
	opts.Agent = ClaudeCandidate
	opts.BaseEnv = append(opts.BaseEnv, "HOME="+t.TempDir())
	return opts, got
}

// session/load reopens the stored conversation under the same reet session id, which is what an in-place reconnect
// asks for. The adapter's replay is dropped: the controller keeps its own transcript.
func TestClaudeReattachesInPlace(t *testing.T) {
	opts, _ := claudeOptions(t, "claudecode")
	opts.Resume = "5d0a3b57-7c2e-4f58-9d1b-3b8a2c1e9f40"
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if session.SessionID() != opts.Resume || session.Mode() != "default" {
		t.Fatalf("reattached %q in mode %q", session.SessionID(), session.Mode())
	}
}

// A conversation the adapter never stored cannot be loaded: the refusal names that case, not a generic start failure.
func TestClaudeReattachWithoutAStoredConversationIsRefused(t *testing.T) {
	opts, _ := claudeOptions(t, "claudecode-no-store")
	opts.Resume = "5d0a3b57-7c2e-4f58-9d1b-3b8a2c1e9f40"
	if session, err := Start(context.Background(), opts); !errors.Is(err, ErrNoStoredConversation) {
		if session != nil {
			session.Close()
		}
		t.Fatalf("no stored conversation: %v", err)
	}
}
