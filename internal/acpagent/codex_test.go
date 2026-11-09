//go:build linux || darwin

package acpagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexUsesOnlyItsOwnLoginFolder(t *testing.T) {
	for _, tc := range []struct{ name, override, want string }{
		{"default", "", "/person/.codex"},
		{"custom", "/person/custom-codex", "/person/custom-codex"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := []string{"HOME=/person", "CODEX_HOME=" + tc.override, "PATH=/bin", "XDG_CONFIG_HOME=/person/.config", "XDG_DATA_HOME=/person/.local/share", "OPENAI_API_KEY=private", "GH_TOKEN=private", "SSH_AUTH_SOCK=/person/ssh"}
			for name, env := range map[string][]string{
				"probe": minimalEnvironment(CodexCandidate, "/reet/private", base),
				"agent": agentEnvironment(CodexCandidate, "/reet/private", base, LaunchEnv{}),
			} {
				joined := "\n" + strings.Join(env, "\n") + "\n"
				for _, want := range []string{"HOME=/reet/private", "CODEX_HOME=" + tc.want, "PATH=/bin"} {
					if !strings.Contains(joined, "\n"+want+"\n") {
						t.Errorf("%s missing %s: %s", name, want, joined)
					}
				}
				for _, bad := range []string{"HOME=/person", "XDG_CONFIG_HOME=", "XDG_DATA_HOME=", "OPENAI_API_KEY=", "GH_TOKEN=", "SSH_AUTH_SOCK="} {
					if strings.Contains(joined, "\n"+bad) {
						t.Errorf("%s inherited %s", name, bad)
					}
				}
			}
			// The same folder must not leak into another agent's environment.
			if strings.Contains(strings.Join(agentEnvironment(ClaudeCandidate, "/reet/private", base, LaunchEnv{}), "\n"), "CODEX_HOME=") {
				t.Fatal("Codex login leaked to Claude")
			}
		})
	}
}

// codexFolderHome is a throwaway HOME that has a Codex folder, as a machine that has used Codex does; what is in the folder is the test's business.
func codexFolderHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	return "HOME=" + home
}

func TestCodexStartUsesExistingLoginWithoutAuthenticating(t *testing.T) {
	opts, got := options(t, "codex-login", "1.13.1")
	opts.Agent = CodexCandidate
	opts.BaseEnv = append(opts.BaseEnv, codexFolderHome(t))
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if session.Mode() != "read-only" {
		t.Fatalf("mode %q", session.Mode())
	}
	got.mu.Lock()
	defer got.mu.Unlock()
	found := false
	for _, e := range got.all {
		if strings.Contains(e.Text, "private@example.test") {
			t.Fatal("email escaped")
		}
		if e.Kind == "notice" && e.Text == "Codex sign-in: ChatGPT Pro" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no login notice: %+v", got.all)
	}
}

func TestCodexMissingLoginHasActionableStartError(t *testing.T) {
	for _, scenario := range []string{"codex-auth-required", "codex-other-error", "other-auth-required"} {
		version := "1.13.1"
		if scenario == "other-auth-required" {
			version = testVersion
		}
		opts, _ := options(t, scenario, version)
		if strings.HasPrefix(scenario, "codex-") {
			opts.Agent = CodexCandidate
		}
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
			t.Fatal(err)
		}
		opts.BaseEnv = append(opts.BaseEnv, "HOME="+home)
		session, err := Start(context.Background(), opts)
		if session != nil {
			session.Close()
			t.Fatal("failed start returned a chat")
		}
		if err == nil {
			t.Fatal("missing startup error")
		}
		actionable := err.Error() == "Sign in to Codex on that machine first."
		if actionable != (scenario == "codex-auth-required") {
			t.Fatalf("%s: %v", scenario, err)
		}
	}
}

func TestCodexAuthNoticeDiscardsPrivateAndMalformedFields(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{`{"authStatus":{"kind":"none","label":"Not logged in","account":{"email":"private@example.test"}}}`, "Codex sign-in: Not logged in"},
		{`{"authStatus":{"kind":"api_key","label":"OpenAI API key","account":{"email":"private@example.test"}}}`, "Codex sign-in: OpenAI API key"},
		{`{"authStatus":{"kind":"account","label":"ChatGPT Pro","account":{"email":"private@example.test"}}}`, "Codex sign-in: ChatGPT Pro"},
		{`{"authStatus":{"kind":"account","label":"private@example.test"}}`, ""},
		{`{"authStatus":{"kind":"account","label":"bad\nlabel"}}`, ""},
		{`{"authStatus":{"label":"missing kind"}}`, ""},
		{`{"authStatus":{"kind":"account","label":42}}`, ""},
		{`{"authStatus":{"kind":"unknown","label":"unknown"}}`, ""},
		{`{"authStatus":{"kind":"account","label":"` + strings.Repeat("x", 257) + `"}}`, ""},
	} {
		opts, got := options(t, "", "1.13.1")
		opts.Agent = CodexCandidate
		s := &Session{opts: opts}
		s.handle(rpcMessage{Method: "_auth/status_update", Params: json.RawMessage(tc.raw)})
		got.mu.Lock()
		events := append([]Event(nil), got.all...)
		got.mu.Unlock()
		if tc.want == "" {
			if len(events) != 0 {
				t.Fatalf("unexpected event: %+v", events)
			}
			continue
		}
		if len(events) != 1 || events[0].Kind != "notice" || events[0].Text != tc.want {
			t.Fatalf("got %+v want %s", events, tc.want)
		}
	}
}

func TestCodexRefusesInvalidLoginPathsBeforeLaunch(t *testing.T) {
	for _, base := range [][]string{
		{"HOME=/person", "CODEX_HOME=relative"},
		{"HOME=/person", "CODEX_HOME=/bad\npath"},
		{"HOME=relative"},
		nil,
	} {
		opts, _ := options(t, "codex-login", "1.13.1")
		opts.Agent = CodexCandidate
		opts.BaseEnv = base
		session, err := Start(context.Background(), opts)
		if session != nil {
			session.Close()
		}
		if err == nil || !strings.Contains(err.Error(), "absolute CODEX_HOME") {
			t.Fatalf("bad path was not refused: %v", err)
		}
	}
}

func TestOtherAgentsIgnoreCodexAuthNotifications(t *testing.T) {
	opts, got := options(t, "", testVersion)
	s := &Session{opts: opts}
	s.handle(rpcMessage{Method: "_auth/status_update", Params: json.RawMessage(`{"authStatus":{"kind":"account","label":"ChatGPT Pro"}}`)})
	if len(got.all) != 0 {
		t.Fatalf("non-Codex notice: %+v", got.all)
	}
}

// Someone who has never used Codex has no ~/.codex, and codex refuses to start when CODEX_HOME names a folder that is not there. That is a
// missing sign-in, so the person is told to sign in, and the adapter is not started to find it out.
func TestCodexWithNoLoginFolderAsksForSignIn(t *testing.T) {
	for _, scenario := range []string{"codex-login", "codex-other-error"} {
		opts, _ := options(t, scenario, "1.13.1")
		opts.Agent = CodexCandidate
		opts.BaseEnv = append(opts.BaseEnv, "HOME="+t.TempDir())
		session, err := Start(context.Background(), opts)
		if session != nil {
			session.Close()
			t.Fatalf("%s: started a chat with no Codex folder", scenario)
		}
		if !errors.Is(err, ErrCodexLogin) {
			t.Fatalf("%s: no Codex folder: %v", scenario, err)
		}
	}
	// An explicit CODEX_HOME that is not there is the same.
	opts, _ := options(t, "codex-login", "1.13.1")
	opts.Agent = CodexCandidate
	opts.BaseEnv = append(opts.BaseEnv, "HOME="+t.TempDir(), "CODEX_HOME="+filepath.Join(t.TempDir(), "missing"))
	if session, err := Start(context.Background(), opts); !errors.Is(err, ErrCodexLogin) {
		if session != nil {
			session.Close()
		}
		t.Fatalf("a CODEX_HOME that does not exist: %v", err)
	}
}

// Codex answers a conversation it never stored with a generic internal error that names the missing thread. That is the missing
// stored conversation; an internal error that names anything else is not.
func TestCodexUnstoredLoadIsTheMissingConversation(t *testing.T) {
	for scenario, want := range map[string]bool{"codex-no-store": true, "codex-load-error": false} {
		opts, _ := options(t, scenario, "1.13.1")
		opts.Agent = CodexCandidate
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
			t.Fatal(err)
		}
		opts.BaseEnv = append(opts.BaseEnv, "HOME="+home)
		opts.Resume = "6f1c2c3a-0b4d-4f6e-8a2b-9d3e5f7a1b2c"
		session, err := Start(context.Background(), opts)
		if session != nil {
			session.Close()
			t.Fatalf("%s: opened a conversation", scenario)
		}
		if errors.Is(err, ErrNoStoredConversation) != want {
			t.Fatalf("%s: stored-conversation refusal=%v, want %v (%v)", scenario, errors.Is(err, ErrNoStoredConversation), want, err)
		}
	}
}

// A stored Codex conversation reopens under its own id, and the chat still starts in read-only: the adapter loads it in its default
// mode, which asks less, so the profile switches it and confirms the switch before the chat is offered.
func TestCodexLoadsAStoredConversationInReadOnly(t *testing.T) {
	opts, _ := options(t, "codex-load", "1.13.1")
	opts.Agent = CodexCandidate
	opts.BaseEnv = append(opts.BaseEnv, codexFolderHome(t))
	opts.Resume = "6f1c2c3a-0b4d-4f6e-8a2b-9d3e5f7a1b2c"
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if session.SessionID() != opts.Resume || session.Mode() != "read-only" {
		t.Fatalf("reopened %q in mode %q", session.SessionID(), session.Mode())
	}
}
