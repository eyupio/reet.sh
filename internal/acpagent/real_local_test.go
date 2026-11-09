//go:build linux || darwin

package acpagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
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
// that names a model other than the adapter's own works because the profile
// confirms the model switch by read-back.
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
	// The profile confirms a chosen model by switching to it and reading the switch back.
	started, err := Start(ctx, chosen)
	if err != nil {
		t.Fatalf("launch with model %q and effort low: %v", other, err)
	}
	defer started.Close()
	if got := settingOf(t, started.Controls(), "thought_level").Current; got != "low" || started.Model() != other {
		t.Fatalf("started on model %q with thinking level %q", started.Model(), got)
	}
	t.Logf("launch with model %q and effort low works", other)
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

// TestRealClaudeHistoryReplay is opt-in and sends no prompt. REET_CLAUDE_STORED_HOME and REET_CLAUDE_STORED_SESSION
// name the throwaway home and the session id that a `scripts/probe-claude-lifecycle.py load` run left behind. Opening
// that conversation with session/load must succeed under the same id, and what the adapter replays reaches OnEvent only
// before the chat is ready, which is where the connector drops it because the controller keeps its own transcript.
func TestRealClaudeHistoryReplay(t *testing.T) {
	binary, home, stored := os.Getenv("REET_CLAUDE_ACP"), os.Getenv("REET_CLAUDE_STORED_HOME"), os.Getenv("REET_CLAUDE_STORED_SESSION")
	if binary == "" || home == "" || stored == "" {
		t.Skip("set REET_CLAUDE_ACP, REET_CLAUDE_STORED_HOME and REET_CLAUDE_STORED_SESSION from a probe-claude-lifecycle.py load run")
	}
	dir := os.Getenv("REET_CLAUDE_STORED_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var mu sync.Mutex
	var replayed []string
	ready := false
	base := []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8"}
	session, err := Start(ctx, Options{Agent: ClaudeCandidate, Binary: binary, Dir: dir, Home: home, BaseEnv: base, Resume: stored,
		PermissionTimeout: time.Second, TurnTimeout: 5 * time.Second,
		OnEvent: func(e Event) {
			mu.Lock()
			defer mu.Unlock()
			if !ready {
				replayed = append(replayed, e.Kind)
			}
		},
		OnReady: func() { mu.Lock(); ready = true; mu.Unlock() }})
	if err != nil {
		t.Fatalf("session/load of the stored conversation: %v", err)
	}
	defer session.Close()
	if session.SessionID() != stored {
		t.Fatalf("reopened under %q, want %q", session.SessionID(), stored)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Logf("reopened %s; %d events arrived before the chat was ready (the connector drops them): %v", stored, len(replayed), replayed)
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

// codexHome is a throwaway home whose Codex folder holds a placeholder API key, which is all the adapter needs to answer
// session/new: it makes no model call until a prompt is sent, so these tests need no account and no network. A home without
// the folder is the signed-out machine.
func codexHome(t *testing.T, signedIn bool) string {
	t.Helper()
	home := t.TempDir()
	if signedIn {
		if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, ".codex", "auth.json"), []byte(`{"OPENAI_API_KEY":"sk-placeholder-not-a-key","auth_mode":"apikey"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

// TestRealCodexStart is opt-in and sends no prompt. Against the real adapter, through the profile reet launches it with, it pins what
// the Codex parity work depends on, as measured signed-out with a placeholder login (a signed-in run adds what needs a real session):
// a machine without a Codex login is told to sign in; a chat starts in read-only; the reasoning effort is offered, reads back when
// set and is refused for a value the model does not offer; and a model switch keeps the chosen effort when the new model still
// offers it and never leaves one it does not offer (the fall-back to medium is measured by the probe's effortCarried sweep, which
// switches every model, and only logged here). A conversation the adapter never stored is refused.
func TestRealCodexStart(t *testing.T) {
	binary := os.Getenv("REET_CODEX_ACP")
	if binary == "" {
		t.Skip("set REET_CODEX_ACP to a codex-acp executable from an npm install (its codex dependency sits beside it in node_modules/.bin)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	opts := func(home string) Options {
		return Options{Agent: CodexCandidate, Binary: binary, Dir: t.TempDir(), Home: t.TempDir(), BaseEnv: []string{"PATH=" + filepath.Dir(binary) + ":" + os.Getenv("PATH"), "HOME=" + home, "LANG=C.UTF-8"}, PermissionTimeout: time.Second, TurnTimeout: 5 * time.Second}
	}

	// A Codex folder with no login in it: the person is told to sign in. When there is no Codex folder at all (someone who has never
	// used Codex) the profile still points CODEX_HOME at it, and codex refuses to start ("CODEX_HOME points to ..., but that path does
	// not exist"), so the chat reports that the agent exited instead of asking for a sign-in. That is logged, not pinned: the parity work fixes it.
	emptyFolder := codexHome(t, false)
	if err := os.MkdirAll(filepath.Join(emptyFolder, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if session, err := Start(ctx, opts(emptyFolder)); err == nil || !errors.Is(err, ErrCodexLogin) {
		if session != nil {
			session.Close()
		}
		t.Fatalf("a Codex folder with no login: %v", err)
	}
	if session, err := Start(ctx, opts(codexHome(t, false))); err == nil {
		session.Close()
		t.Fatal("started with no Codex folder")
	} else {
		t.Logf("no Codex folder at all: %v", err)
	}

	session, err := Start(ctx, opts(codexHome(t, true)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		session.Close()
		if !session.GroupRetired() {
			t.Error("Close did not confirm that every process the adapter started is gone")
		}
	}()
	if session.SessionID() == "" || session.Model() == "" {
		t.Fatalf("session %q model %q", session.SessionID(), session.Model())
	}
	effort := settingOf(t, session.Controls(), "thought_level")
	if effort.ID != "reasoning_effort" || effort.Current != "low" {
		t.Fatalf("reasoning effort at session/new: %+v", effort)
	}
	for _, want := range []string{"low", "medium", "high", "xhigh"} {
		if !slices.Contains(effort.Values, want) {
			t.Fatalf("efforts %v lack %q", effort.Values, want)
		}
	}
	if err := session.SetOption(ctx, effort.ID, "high"); err != nil {
		t.Fatal(err)
	}
	if got := settingOf(t, session.Controls(), "thought_level").Current; got != "high" {
		t.Fatalf("effort after a confirmed set: %q", got)
	}
	if err := session.SetOption(ctx, effort.ID, "not-an-effort"); !errors.Is(err, ErrControl) {
		t.Fatalf("an unlisted effort: %v", err)
	}
	if got := settingOf(t, session.Controls(), "thought_level").Current; got != "high" {
		t.Fatalf("a refused effort changed the setting to %q", got)
	}

	// A model switch keeps the chosen effort where the new model offers it, and never leaves one the model does not.
	model := settingOf(t, session.Controls(), "model")
	if len(model.Values) < 2 {
		t.Fatalf("no second model to switch to in %+v", model)
	}
	if err := session.SetOption(ctx, effort.ID, "ultra"); err != nil {
		t.Fatalf("choosing ultra on %q: %v", session.Model(), err)
	}
	sawFallback := false
	for _, value := range model.Values {
		before := settingOf(t, session.Controls(), "thought_level").Current
		if err := session.SetOption(ctx, model.ID, value); err != nil {
			t.Fatalf("switching to model %q: %v", value, err)
		}
		after := settingOf(t, session.Controls(), "thought_level")
		if !slices.Contains(after.Values, after.Current) {
			t.Fatalf("model %q left effort %q outside the values it offers %v", value, after.Current, after.Values)
		}
		if slices.Contains(after.Values, before) {
			if after.Current != before {
				t.Fatalf("model %q offers %q but the switch changed the effort from it to %q", value, before, after.Current)
			}
		} else {
			sawFallback = true
		}
		if err := session.SetOption(ctx, "reasoning_effort", "ultra"); err != nil && slices.Contains(after.Values, "ultra") {
			t.Fatalf("choosing ultra on %q: %v", value, err)
		}
	}
	t.Logf("across %d models: an effort the new model lacked was replaced=%v", len(model.Values), sawFallback)

	// A conversation the adapter never stored cannot be opened. Its refusal is a generic internal error that names the missing
	// thread, not the stored-conversation code the other adapters use; mapping it is part of the history work.
	stray, err := Start(ctx, func() Options {
		o := opts(codexHome(t, true))
		o.Resume = "6f1c2c3a-0b4d-4f6e-8a2b-9d3e5f7a1b2c"
		return o
	}())
	if stray != nil {
		stray.Close()
		t.Fatal("opened a conversation that was never stored")
	}
	if err == nil {
		t.Fatal("no error for a conversation that was never stored")
	}
	t.Logf("load of a conversation that was never stored: %v (is ErrNoStoredConversation: %v)", err, errors.Is(err, ErrNoStoredConversation))
}
