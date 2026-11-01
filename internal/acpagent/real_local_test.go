//go:build linux || darwin

package acpagent

import (
	"context"
	"os"
	"regexp"
	"testing"
	"time"
)

// TestRealOpenCodeStart is opt-in: it runs the exact probed binary against a
// throwaway HOME. It is skipped in CI, which has no OpenCode install.
func TestRealOpenCodeStart(t *testing.T) {
	binary := os.Getenv("REET_OPENCODE_1195")
	if binary == "" {
		t.Skip("set REET_OPENCODE_1195 to an opencode-ai@1.19.5 binary")
	}
	home, dir := t.TempDir(), t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	session, err := Start(ctx, Options{Agent: OpenCode, Binary: binary, Dir: dir, Home: home, BaseEnv: []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8"}, PermissionTimeout: time.Second, TurnTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if session.SessionID() == "" || session.Model() == "" {
		t.Fatalf("session %q model %q", session.SessionID(), session.Model())
	}
	id := session.SessionID()
	session.Close()
	resumed, err := Start(ctx, Options{Agent: OpenCode, Binary: binary, Dir: dir, Home: home, BaseEnv: []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8"}, Resume: id, PermissionTimeout: time.Second, TurnTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	if resumed.SessionID() != id {
		t.Fatalf("loaded %q, wanted %q", resumed.SessionID(), id)
	}
}

// TestRealClaudeStart is opt-in: it starts the real Claude ACP adapter through the
// Claude profile and closes it. It sends no prompt, so it makes no model call and
// cannot spend anything. It checks the version floor, the adapter identity, the
// required Manual mode and that Close leaves no live process behind. It needs an
// adapter install (see scripts/check-claude-credentials.py) and Node on PATH.
func TestRealClaudeStart(t *testing.T) {
	binary := os.Getenv("REET_CLAUDE_ACP")
	if binary == "" {
		t.Skip("set REET_CLAUDE_ACP to a claude-agent-acp@0.99.1 executable")
	}
	home, dir := t.TempDir(), t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	base := []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8"}
	session, err := Start(ctx, Options{Agent: ClaudeCandidate, Binary: binary, Dir: dir, Home: home, BaseEnv: base, PermissionTimeout: time.Second, TurnTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if session.SessionID() == "" {
		t.Fatal("no session id")
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`).MatchString(session.SessionID()) {
		t.Fatalf("session id %q is not the measured UUID form", session.SessionID())
	}
	session.Close()
	if !session.GroupRetired() {
		t.Fatal("Close did not confirm that every process the adapter started is gone")
	}
	// A profile whose floor is above the installed adapter must refuse it.
	strict := ClaudeCandidate
	strict.MinVersion = "99.0.0"
	if refused, err := Start(ctx, Options{Agent: strict, Binary: binary, Dir: dir, Home: home, BaseEnv: base, PermissionTimeout: time.Second, TurnTimeout: 5 * time.Second}); err == nil {
		refused.Close()
		t.Fatal("an adapter below the version floor was started")
	}
}
