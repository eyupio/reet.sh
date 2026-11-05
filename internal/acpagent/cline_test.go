//go:build linux || darwin

package acpagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// clineOptions is the fake ACP child with Cline's profile, and a base
// environment whose HOME resolves the real login route.
func clineOptions(t *testing.T, scenario string) (Options, *events) {
	t.Helper()
	opts, got := options(t, scenario, "3.0.74")
	opts.Agent = Cline
	opts.BaseEnv = append(opts.BaseEnv, "HOME="+t.TempDir(), "CLINE_API_KEY=test-key")
	return opts, got
}

// Cline's profile pins the launch that makes its approval contract true, and
// only the reviewed adapter name may report it as available.
func TestClineProfilePinsItsSafeLaunch(t *testing.T) {
	spec, ok := Lookup("cline")
	if !ok {
		t.Fatal("cline is not registered")
	}
	wantArgs := []string{"--acp", "--auto-approve", "false"}
	if spec.Command != "cline" || strings.Join(spec.Args, " ") != strings.Join(wantArgs, " ") {
		t.Fatalf("launch: %+v", spec)
	}
	if spec.ExpectedName != "cline" || spec.MinVersion != "3.0.74" || spec.RequireMode != "act" || spec.StartMode != "act" {
		t.Fatalf("identity: %+v", spec)
	}
	if spec.RequireOptions["auto_approve"] != "false" || !spec.ConfirmModelBySwitch {
		t.Fatalf("safety contract: %+v", spec)
	}
	if strings.Join(spec.ChoosableModes, ",") != "act,plan" || spec.ChoosableModes[0] != spec.StartMode {
		t.Fatalf("modes: %v", spec.ChoosableModes)
	}
	if spec.LoginHomeEnv != "CLINE_DIR" || spec.LoginHomeDir != ".cline" {
		t.Fatalf("login route: %+v", spec)
	}
	if !terminalproto.ReviewedChatTool("cline") || !terminalproto.ValidACPSessionIDFor("cline", "1793848855723_8zvJS_cli") {
		t.Fatal("cline handle is not reviewed")
	}
	desc := Describe("cline", true, "")
	if desc.State != "available" || !desc.Valid() || desc.Adapter != "cline" || desc.MinimumVersion != "3.0.74" {
		t.Fatalf("descriptor: %+v", desc)
	}
	if !desc.History || !desc.Images || !desc.Model || !desc.Approval || desc.Questions {
		t.Fatalf("descriptor capabilities: %+v", desc)
	}
	if missing := Describe("cline", false, ""); missing.State != "missing" || !missing.Valid() {
		t.Fatalf("missing descriptor: %+v", missing)
	}
	if old := Describe("cline", true, "3.0.73"); old.State != "incompatible" || !old.Valid() {
		t.Fatalf("old descriptor: %+v", old)
	}
}

// The person's own Cline folder is routed explicitly, the private HOME is kept,
// and only the one verified credential variable is inherited.
func TestClineEnvironmentKeepsOnlyItsOwnLoginAndKey(t *testing.T) {
	env := agentEnvironment(Cline, "/tmp/private-home", []string{
		"PATH=/bin", "HOME=/home/u", "CLINE_DIR=/home/u/.cline", "LANG=C.UTF-8",
		"CLINE_API_KEY=sk-cline", "CLINE_MODEL=redirected/model", "CLINE_PROVIDER=other",
		"OPENAI_API_KEY=sk-other", "SSH_AUTH_SOCK=/tmp/ssh", "XDG_CONFIG_HOME=/home/u/.config",
	}, LaunchEnv{})
	joined := "\n" + strings.Join(env, "\n") + "\n"
	for _, bad := range []string{"CLINE_MODEL", "CLINE_PROVIDER", "OPENAI_API_KEY", "SSH_AUTH_SOCK", "XDG_CONFIG_HOME", "HOME=/home/u\n"} {
		if strings.Contains(joined, bad) {
			t.Fatalf("kept %s:\n%s", bad, joined)
		}
	}
	for _, good := range []string{"\nHOME=/tmp/private-home\n", "\nCLINE_DIR=/home/u/.cline\n", "\nCLINE_API_KEY=sk-cline\n"} {
		if !strings.Contains(joined, good) {
			t.Fatalf("missing %q:\n%s", good, joined)
		}
	}
	// Without an explicit CLINE_DIR the route is the real home's .cline folder.
	env = agentEnvironment(Cline, "/tmp/private-home", []string{"PATH=/bin", "HOME=/home/u"}, LaunchEnv{})
	if !strings.Contains("\n"+strings.Join(env, "\n")+"\n", "\nCLINE_DIR="+filepath.Join("/home/u", ".cline")+"\n") {
		t.Fatalf("default login route: %v", env)
	}
}

// A new Cline chat starts in act mode with auto-approve off, keeps its dynamic
// model and offers the model switch as the only live selector.
func TestClineStartsWithApprovalsOnAndItsModelCatalogue(t *testing.T) {
	opts, _ := clineOptions(t, "cline")
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if session.Mode() != "act" || session.SessionID() != "1793848855723_8zvJS_cli" {
		t.Fatalf("started in %q with id %q", session.Mode(), session.SessionID())
	}
	controls := session.Controls()
	if controls.Model != "stub/model" || len(controls.Options) != 1 || controls.Options[0].Category != "model" {
		t.Fatalf("controls: %+v", controls)
	}
	if strings.Join(controls.Options[0].Values, ",") != "stub/model,provider/other" {
		t.Fatalf("model values: %+v", controls.Options[0])
	}
}

// The launch flag is a request: a session that reports auto-approve on is
// refused rather than trusted.
func TestClineRefusesASessionThatAutoApproves(t *testing.T) {
	opts, _ := clineOptions(t, "cline-approve-on")
	session, err := Start(context.Background(), opts)
	if err == nil || !errors.Is(err, ErrSafety) {
		t.Fatalf("auto-approve on: session=%v err=%v", session, err)
	}
	if session != nil {
		session.Close()
	}
}

// Cline ignores a model at session/new, so a chosen model is applied by a
// confirmed switch. A refused switch refuses the chat.
func TestClineAppliesAChosenModelByAConfirmedSwitch(t *testing.T) {
	opts, _ := clineOptions(t, "cline-default-model")
	opts.Model = "stub/model"
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if session.Model() != "stub/model" {
		t.Fatalf("model after switch: %q", session.Model())
	}

	opts, _ = clineOptions(t, "cline-model-refused")
	opts.Model = "stub/model"
	if session, err = Start(context.Background(), opts); err == nil || !errors.Is(err, ErrModel) {
		t.Fatalf("refused switch: session=%v err=%v", session, err)
	}
}

// Every Cline card carries something a person can review: a read has its file,
// an editor write is shown as the text it will write, and a command list is the
// command, so Allow once stays available for each.
func TestClinePermissionCardsAreReviewable(t *testing.T) {
	for _, tc := range []struct {
		scenario      string
		kind          string
		path, command string
		diff          string
	}{
		{"cline-read", "read", "/work/notes.txt", "", ""},
		{"cline-editor", "edit", "/work/new.txt", "", "+reet-probe"},
		{"cline-command", "execute", "", "go test ./...", ""},
	} {
		t.Run(tc.scenario, func(t *testing.T) {
			opts, got := clineOptions(t, tc.scenario)
			session, err := Start(context.Background(), opts)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			done := make(chan error, 1)
			go func() { done <- session.Prompt(context.Background(), "go on") }()
			var event Event
			for event = got.next(t); event.Kind != "permission"; event = got.next(t) {
			}
			if event.ToolKind != tc.kind || event.Truncated {
				t.Fatalf("card: %+v", event)
			}
			if event.Path != tc.path || event.Command != tc.command || !strings.Contains(event.Diff, tc.diff) {
				t.Fatalf("card detail: %+v", event)
			}
			if err := session.Allow(event.PermissionID, true); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("the answered turn did not end")
			}
		})
	}
}

// The editor's old_text is shown as removed lines, so a replacement never hides
// what it takes away.
func TestClineEditorReplacementShowsBothSides(t *testing.T) {
	oldText := "old line"
	diff, cut := renderTextDiff("/work/new.txt", &oldText, "new line")
	if cut || !strings.Contains(diff, "-old line") || !strings.Contains(diff, "+new line") {
		t.Fatalf("diff: %q cut=%v", diff, cut)
	}
}

// A provider failure Cline logged without an ACP frame is read back by session
// and time: the newest error for this chat, never another chat's or an older
// turn's, and never a line that carries no message.
func TestClineLogErrorReadsThisTurnOnly(t *testing.T) {
	root := t.TempDir()
	logDir := filepath.Join(root, "data", "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sessionID := "1793848855723_8zvJS_cli"
	base := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	at := func(offset time.Duration) string { return base.Add(offset).Format(time.RFC3339Nano) }
	lines := []string{
		`{"level":30,"time":"` + at(0) + `","sessionId":"` + sessionID + `","message":"starting"}`,
		`{"level":50,"time":"` + at(-time.Minute) + `","sessionId":"` + sessionID + `","err":{"message":"before this turn"}}`,
		`{"level":50,"time":"` + at(time.Second) + `","sessionId":"another_chat_cli","err":{"message":"another chat"}}`,
		`{"level":50,"time":"` + at(2*time.Second) + `","sessionId":"` + sessionID + `","err":{"message":"Insufficient balance. Your Cline Credits balance is $-0.00 (Request ID: test)"},"msg":"Interactive turn failed"}`,
		`{"level":50,"time":"` + at(3*time.Second) + `","sessionId":"` + sessionID + `","err":{"message":"Unauthorized"},"msg":"[ai-sdk] provider error"}`,
		`{"level":50,"time":"` + at(4*time.Second) + `","sessionId":"` + sessionID + `","msg":"Agent loop caught error"}`,
		`not json`,
	}
	if err := os.WriteFile(filepath.Join(logDir, "cline.log"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := clineLogError(root, sessionID, base.UnixMilli()); got != "Unauthorized" {
		t.Fatalf("newest error: %q", got)
	}
	if got := clineLogError(root, sessionID, base.Add(3*time.Second+time.Millisecond).UnixMilli()); got != "" {
		t.Fatalf("error before the turn start was reported: %q", got)
	}
	if got := clineLogError(root, "another_chat_cli", base.UnixMilli()); got != "another chat" {
		t.Fatalf("another chat: %q", got)
	}
	if got := clineLogError(root, "", base.UnixMilli()); got != "" {
		t.Fatalf("no session id still read the log: %q", got)
	}
	if got := clineLogError("", sessionID, base.UnixMilli()); got != "" {
		t.Fatalf("no login folder still read the log: %q", got)
	}
	if got := clineLogError(t.TempDir(), sessionID, base.UnixMilli()); got != "" {
		t.Fatalf("a missing log reported an error: %q", got)
	}
}

// A turn the provider failed before any frame is not silent: the chat ends with
// the reason Cline logged.
func TestClineSilentTurnShowsItsLoggedProviderError(t *testing.T) {
	opts, got := clineOptions(t, "cline-silent-error")
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	done := make(chan error, 1)
	go func() { done <- session.Prompt(context.Background(), "hello") }()
	for {
		event := got.next(t)
		if event.Kind == "status" && event.State == "error" {
			if event.Detail != "Insufficient balance for this chat" {
				t.Fatalf("detail: %q", event.Detail)
			}
			break
		}
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("prompt: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the failed turn did not end")
	}
}

// session/load reopens the stored conversation under the same reet session id,
// which is what an in-place reconnect asks for.
func TestClineReattachesInPlace(t *testing.T) {
	opts, _ := clineOptions(t, "cline")
	opts.Resume = "1793848855723_8zvJS_cli"
	opts.Model = "stub/model"
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if session.SessionID() != opts.Resume || session.Mode() != "act" || session.Model() != opts.Model {
		t.Fatalf("reattached %q/%q/%q", session.SessionID(), session.Mode(), session.Model())
	}
}

// A chat Cline never stored (no prompt ran) cannot be loaded: the refusal names
// that case instead of a generic start failure.
func TestClineReattachWithoutAStoredConversationIsRefused(t *testing.T) {
	opts, _ := clineOptions(t, "cline-no-store")
	opts.Resume = "1793848855723_8zvJS_cli"
	if session, err := Start(context.Background(), opts); !errors.Is(err, ErrNoStoredConversation) {
		if session != nil {
			session.Close()
		}
		t.Fatalf("no stored conversation: %v", err)
	}
}
