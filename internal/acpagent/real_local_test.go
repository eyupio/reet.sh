//go:build linux || darwin

package acpagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

// TestRealClaudeOptions is opt-in and sends no prompt, so it makes no model call.
// Against the real adapter, in the environment the connector profile gives it, it
// pins what the parity work depends on: the thinking level is offered at
// session/new for the adapter's default model, reads back when set and is refused
// for an unlisted value; a launch that names only an effort works; and a launch
// that names a model other than the adapter's own is refused by the profile as it
// stands but works once the model switch is confirmed by read-back.
func TestRealClaudeOptions(t *testing.T) {
	binary := os.Getenv("REET_CLAUDE_ACP")
	if binary == "" {
		t.Skip("set REET_CLAUDE_ACP to a claude-agent-acp@0.99.1 executable")
	}
	home, dir := t.TempDir(), t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	base := []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8"}
	options := Options{Agent: ClaudeCandidate, Binary: binary, Dir: dir, Home: home, BaseEnv: base, PermissionTimeout: time.Second, TurnTimeout: 5 * time.Second}
	session, err := Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	effort := settingOf(t, session.Controls(), "thought_level")
	for _, want := range []string{"default", "low", "medium", "high"} {
		if !slices.Contains(effort.Values, want) {
			t.Fatalf("thinking levels %v lack %q", effort.Values, want)
		}
	}
	if effort.Current != "default" {
		t.Fatalf("thinking level at session/new: %q", effort.Current)
	}
	if err := session.SetOption(ctx, effort.ID, "high"); err != nil {
		t.Fatal(err)
	}
	if got := settingOf(t, session.Controls(), "thought_level").Current; got != "high" {
		t.Fatalf("thinking level after a confirmed set: %q", got)
	}
	if err := session.SetOption(ctx, effort.ID, "not-an-effort"); !errors.Is(err, ErrControl) {
		t.Fatalf("an unlisted thinking level: %v", err)
	}
	if got := settingOf(t, session.Controls(), "thought_level").Current; got != "high" {
		t.Fatalf("a refused thinking level changed the setting to %q", got)
	}

	// A launch that names only an effort.
	only := options
	only.Effort = "low"
	startedOnlyEffort, err := Start(ctx, only)
	if err != nil {
		t.Fatalf("launch with effort low and no model: %v", err)
	}
	defer startedOnlyEffort.Close()
	if got := settingOf(t, startedOnlyEffort.Controls(), "thought_level").Current; got != "low" {
		t.Fatalf("started with thinking level %q", got)
	}

	// A launch that names a model other than the adapter's own.
	model := settingOf(t, session.Controls(), "model")
	other := ""
	for _, value := range model.Values {
		if value != "default" && value != session.Model() {
			other = value
			break
		}
	}
	if other == "" {
		t.Fatalf("no second model to choose in %+v", model)
	}
	chosen := options
	chosen.Model, chosen.Effort = other, "low"
	// Pinned as the profile stands today; the slice that sets ConfirmModelBySwitch flips this assertion.
	if started, err := Start(ctx, chosen); !errors.Is(err, ErrModel) {
		if started != nil {
			started.Close()
		}
		t.Fatalf("launch with model %q and effort low as the profile stands: %v", other, err)
	}
	// The same launch when a model switch is confirmed by reading it back.
	switched := options
	switched.Agent.ConfirmModelBySwitch = true
	switched.Model, switched.Effort = other, "low"
	started, err := Start(ctx, switched)
	if err != nil {
		t.Fatalf("launch with model %q and effort low when the switch is confirmed by read-back: %v", other, err)
	}
	defer started.Close()
	if got := settingOf(t, started.Controls(), "thought_level").Current; got != "low" || started.Model() != other {
		t.Fatalf("started on model %q with thinking level %q", started.Model(), got)
	}
	t.Logf("launch with model %q and effort low works when the model switch is confirmed by read-back", other)
}

// TestRealClaudeLoadUnstored is opt-in and sends no prompt. A conversation the
// adapter never stored cannot be loaded, and the refusal is the stored-conversation
// error, not a generic start failure.
func TestRealClaudeLoadUnstored(t *testing.T) {
	binary := os.Getenv("REET_CLAUDE_ACP")
	if binary == "" {
		t.Skip("set REET_CLAUDE_ACP to a claude-agent-acp@0.99.1 executable")
	}
	home, dir := t.TempDir(), t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	base := []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8"}
	session, err := Start(ctx, Options{Agent: ClaudeCandidate, Binary: binary, Dir: dir, Home: home, BaseEnv: base, Resume: "6f1c2c3a-0b4d-4f6e-8a2b-9d3e5f7a1b2c", PermissionTimeout: time.Second, TurnTimeout: 5 * time.Second})
	if session != nil {
		session.Close()
	}
	if !errors.Is(err, ErrNoStoredConversation) {
		t.Fatalf("load of a conversation that was never stored: %v", err)
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
