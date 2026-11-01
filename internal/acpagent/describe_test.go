//go:build linux || darwin

package acpagent

import "testing"

func TestDescribeInstalledAdapterEvidence(t *testing.T) {
	for _, tc := range []struct {
		tool           string
		installed      bool
		version, state string
	}{
		{"opencode", false, "", "missing"},
		{"opencode", true, "1.19.5", "available"},
		{"opencode", true, "", "available"}, // executable/handshake are checked at launch
		{"opencode", true, "1.19.4", "incompatible"},
		{"opencode", true, "garbled", "incompatible"},
		{"claude", true, "", "unverified"},
		{"codex", true, "", "unverified"},
		{"gemini", false, "", "missing"},
	} {
		got := Describe(tc.tool, tc.installed, tc.version)
		if got.State != tc.state || !got.Valid() {
			t.Fatalf("%+v: %+v", tc, got)
		}
		if got.State != "available" && (got.History || got.Images || got.Model || got.Approval) {
			t.Fatalf("unverified features: %+v", got)
		}
	}
}

// Claude is registered but dark: without the machine's opt-in it is only
// "unverified", exactly as before; with it, it is available with just the
// capabilities the probe verified.
func TestClaudeIsDarkUntilTheMachineOptsIn(t *testing.T) {
	t.Setenv(ClaudeSwitch, "")
	if _, ok := Lookup("claude"); ok {
		t.Fatal("claude is registered without the switch")
	}
	if got := Describe("claude", true, ""); got.State != "unverified" || got.Model || got.Approval {
		t.Fatalf("dark claude: %+v", got)
	}
	for _, off := range []string{"0", "true", "yes", " 1"} {
		t.Setenv(ClaudeSwitch, off)
		if _, ok := Lookup("claude"); ok {
			t.Fatalf("%q enabled claude", off)
		}
	}
	t.Setenv(ClaudeSwitch, "1")
	spec, ok := Lookup("claude")
	if !ok || spec.Key != "claude" || spec.RequireMode != "default" {
		t.Fatalf("lookup: %+v %v", spec, ok)
	}
	got := Describe("claude", true, "")
	if got.State != "available" || !got.Valid() || got.Adapter != "claude-agent-acp" || got.MinimumVersion != "0.99.1" {
		t.Fatalf("opted-in claude: %+v", got)
	}
	if !got.Model || !got.Approval || got.History || got.Images || got.Questions {
		t.Fatalf("unverified capabilities were reported: %+v", got)
	}
	if missing := Describe("claude", false, ""); missing.State != "missing" || !missing.Valid() {
		t.Fatalf("missing: %+v", missing)
	}
	// The Claude switch never admits Codex.
	if _, ok := Lookup("codex"); ok {
		t.Fatal("codex is registered by the claude switch")
	}
}

// Codex is registered but dark behind its own switch, starts in read-only (the
// only mode that can ask), and reports approvals only.
func TestCodexIsDarkUntilTheMachineOptsIn(t *testing.T) {
	t.Setenv(CodexSwitch, "")
	t.Setenv(ClaudeSwitch, "1")
	if _, ok := Lookup("codex"); ok {
		t.Fatal("codex is registered without its switch")
	}
	if got := Describe("codex", true, ""); got.State != "unverified" || got.Approval || got.Model {
		t.Fatalf("dark codex: %+v", got)
	}
	t.Setenv(CodexSwitch, "1")
	spec, ok := Lookup("codex")
	if !ok || spec.RequireMode != "read-only" || spec.StartMode != "read-only" || spec.Command != "codex-acp" {
		t.Fatalf("lookup: %+v %v", spec, ok)
	}
	got := Describe("codex", true, "")
	if got.State != "available" || !got.Valid() || got.Adapter != "codex-acp" || got.MinimumVersion != "1.13.1" {
		t.Fatalf("opted-in codex: %+v", got)
	}
	if !got.Approval || got.Model || got.History || got.Images || got.Questions {
		t.Fatalf("unverified capabilities were reported: %+v", got)
	}
	if old := Describe("codex", true, "1.0.0"); old.State != "incompatible" || !old.Valid() {
		t.Fatalf("old codex: %+v", old)
	}
	t.Setenv(ClaudeSwitch, "")
	if _, ok := Lookup("claude"); ok {
		t.Fatal("the codex switch registered claude")
	}
}
