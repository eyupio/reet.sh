//go:build linux || darwin

package acpagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	id, model := session.SessionID(), session.Model()
	session.Close()
	resumed, err := Start(ctx, Options{Agent: OpenCode, Binary: binary, Dir: dir, Home: home, BaseEnv: []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8"}, Model: model, Resume: id, PermissionTimeout: time.Second, TurnTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	if resumed.SessionID() != id || resumed.Model() != model {
		t.Fatalf("loaded %q/%q, wanted %q/%q", resumed.SessionID(), resumed.Model(), id, model)
	}
	resumed.Close()
	chosen, err := Start(ctx, Options{Agent: OpenCode, Binary: binary, Dir: dir, Home: home, BaseEnv: []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8"}, Model: model, PermissionTimeout: time.Second, TurnTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer chosen.Close()
	if chosen.Model() != model {
		t.Fatalf("new chosen model %q, want %q", chosen.Model(), model)
	}
	t.Logf("new/load confirmed explicitly chosen model %q without a prompt", model)
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
	model := session.Model()
	session.Close()
	if !session.GroupRetired() {
		t.Fatal("Close did not confirm that every process the adapter started is gone")
	}
	chosen, err := Start(ctx, Options{Agent: ClaudeCandidate, Binary: binary, Dir: dir, Home: home, BaseEnv: base, Model: model, PermissionTimeout: time.Second, TurnTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if model == "" || chosen.Model() != model {
		chosen.Close()
		t.Fatalf("chosen Claude model %q, want %q", chosen.Model(), model)
	}
	chosen.Close()
	if !chosen.GroupRetired() {
		t.Fatal("chosen-model process group was not retired")
	}
	t.Logf("new confirmed explicitly chosen model %q without a prompt", model)
	// A profile whose floor is above the installed adapter must refuse it.
	strict := ClaudeCandidate
	strict.MinVersion = "99.0.0"
	if refused, err := Start(ctx, Options{Agent: strict, Binary: binary, Dir: dir, Home: home, BaseEnv: base, PermissionTimeout: time.Second, TurnTimeout: 5 * time.Second}); err == nil {
		refused.Close()
		t.Fatal("an adapter below the version floor was started")
	}
}

// TestRealClineStart is opt-in: it starts the installed Cline CLI through the
// Cline profile and sends no prompt, so it spends nothing. It checks the version
// floor, the reported identity, act mode, the boolean auto-approve
// acknowledgement, the handle shape and a confirmed model switch on start.
func TestRealClineStart(t *testing.T) {
	binary := os.Getenv("REET_CLINE_ACP")
	if binary == "" {
		t.Skip("set REET_CLINE_ACP to a cline binary and CLINE_DIR to a signed-in folder")
	}
	login := os.Getenv("CLINE_DIR")
	if login == "" {
		login = filepath.Join(os.Getenv("HOME"), ".cline")
	}
	home, dir := t.TempDir(), t.TempDir()
	base := []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8", "HOME=" + home, "CLINE_DIR=" + login}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	options := Options{Agent: Cline, Binary: binary, Dir: dir, Home: home, BaseEnv: base, PermissionTimeout: time.Second, TurnTimeout: 5 * time.Second}
	session, err := Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if matched, _ := regexp.MatchString(`^[0-9]{13}_[0-9A-Za-z_-]{5}_cli$`, session.SessionID()); !matched || session.Mode() != "act" || session.Model() == "" {
		t.Fatalf("session %q mode %q model %q", session.SessionID(), session.Mode(), session.Model())
	}
	id, model := session.SessionID(), session.Model()
	// A mode switch is confirmed by the reply, without a prompt.
	if err := session.SetMode(ctx, "plan"); err != nil || session.Mode() != "plan" {
		t.Fatalf("plan switch: %v mode %q", err, session.Mode())
	}
	if err := session.SetMode(ctx, "act"); err != nil || session.Mode() != "act" {
		t.Fatalf("act switch: %v mode %q", err, session.Mode())
	}
	// A chosen model is applied and read back, without a prompt.
	other := "stub/other"
	for _, setting := range session.Controls().Options {
		if setting.Category == "model" && len(setting.Values) > 1 && setting.Values[1] != model {
			other = setting.Values[1]
		}
	}
	if other == model {
		t.Log("the installation offers one model, so the confirmed switch is the recorded one")
	} else if err := session.SetModel(ctx, other); err != nil || session.Model() != other {
		t.Fatalf("model switch to %q: %v (model %q)", other, err, session.Model())
	} else if err := session.SetModel(ctx, model); err != nil || session.Model() != model {
		t.Fatalf("model switch back to %q: %v (model %q)", model, err, session.Model())
	}
	session.Close()
	if !session.GroupRetired() {
		t.Fatal("Close did not confirm that every process the adapter started is gone")
	}
	// A prompt-less chat has nothing stored, so a reconnect says exactly that
	// rather than a generic start failure. A chat that ran a prompt replays
	// through session/load; the probe report records that replay.
	options.Resume, options.Model = id, model
	if refused, err := Start(ctx, options); !errors.Is(err, ErrNoStoredConversation) {
		if refused != nil {
			refused.Close()
		}
		t.Fatalf("prompt-less reconnect: %v", err)
	}
	t.Logf("cline %q in act mode, handle %q, model %q", Cline.MinVersion, id, model)
}
