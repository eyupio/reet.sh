//go:build linux || darwin

package acpagent

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Claude creates a session without a login and answers "Authentication required"
// at the first prompt (measured with claude-agent-acp 0.99.1, 4 November 2026).
// That refusal has to reach the person as the sign-in step, not as a generic
// failure that looks like the agent broke.
func TestClaudeMissingLoginAtTheFirstPromptIsActionable(t *testing.T) {
	opts, _ := options(t, "claude-prompt-auth-required", ClaudeCandidate.MinVersion)
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
