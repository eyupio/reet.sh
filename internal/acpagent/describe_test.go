//go:build linux || darwin

package acpagent

import (
	"slices"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

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
		{"claude", true, "", "available"},
		{"codex", true, "", "available"},
		{"claude", false, "", "missing"},
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

// Claude reports what the probe proved and nothing more: approvals, a confirmed model switch, stored history, in-place
// reconnect and image prompts (the adapter advertises them and forwards the block). It reports no questions and no modes.
func TestDescribeClaudeReportsOnlyProvenCapabilities(t *testing.T) {
	spec, ok := Lookup("claude")
	if !ok || spec.Key != "claude" || spec.RequireMode != "default" {
		t.Fatalf("lookup: %+v %v", spec, ok)
	}
	got := Describe("claude", true, "")
	if got.State != "available" || !got.Valid() || got.Adapter != "claude-agent-acp" || got.MinimumVersion != "0.99.1" {
		t.Fatalf("claude: %+v", got)
	}
	if !got.Model || !got.Approval || !got.History || !got.Reattach || !got.Images {
		t.Fatalf("a proven capability is missing: %+v", got)
	}
	if got.Questions || len(got.Modes) != 0 {
		t.Fatalf("an unproven capability was reported: %+v", got)
	}
	if missing := Describe("claude", false, ""); missing.State != "missing" || !missing.Valid() || missing.History || missing.Reattach {
		t.Fatalf("missing: %+v", missing)
	}
}

// Codex starts in read-only (the only mode that can ask) and reports approvals only.
func TestCodexReportsApprovalsOnly(t *testing.T) {
	spec, ok := Lookup("codex")
	if !ok || spec.RequireMode != "read-only" || spec.StartMode != "read-only" || spec.Command != "codex-acp" {
		t.Fatalf("lookup: %+v %v", spec, ok)
	}
	if len(spec.ChoosableModes) != 2 || spec.ChoosableModes[0] != "read-only" || spec.ChoosableModes[1] != "agent" {
		t.Fatalf("choosable modes: %v", spec.ChoosableModes)
	}
	// A chat opens in the first choosable mode: the broker records it as the start mode.
	if spec.ChoosableModes[0] != spec.StartMode {
		t.Fatalf("the first choosable mode %q is not the start mode %q", spec.ChoosableModes[0], spec.StartMode)
	}
	got := Describe("codex", true, "")
	if got.State != "available" || !got.Valid() || got.Adapter != "codex-acp" || got.MinimumVersion != "1.13.1" {
		t.Fatalf("codex: %+v", got)
	}
	if !got.Approval || got.Model || got.History || got.Images || got.Questions {
		t.Fatalf("unverified capabilities were reported: %+v", got)
	}
	if old := Describe("codex", true, "1.0.0"); old.State != "incompatible" || !old.Valid() {
		t.Fatalf("old codex: %+v", old)
	}
}

// The descriptor carries the option values measured from each pinned adapter,
// in the adapter's order, and a caller cannot mutate the profile through it.
func TestDescribeAgentOptionValues(t *testing.T) {
	for _, tc := range []struct {
		tool, version string
		want          []terminalproto.ACPAgentOption
	}{
		{"claude", "0.99.1", []terminalproto.ACPAgentOption{
			{ID: "effort", Category: terminalproto.AgentOptionThoughtLevel, Values: []string{"default", "low", "medium", "high", "xhigh", "max"}},
			{ID: "fast", Category: terminalproto.AgentOptionModelConfig, Values: []string{"on", "off"}},
			{ID: "ultracode", Category: terminalproto.AgentOptionModelConfig, Values: []string{"on", "off"}},
		}},
		{"codex", "1.13.1", []terminalproto.ACPAgentOption{
			{ID: "collaboration_mode", Category: terminalproto.AgentOptionCollaborationMode, Values: []string{"default", "plan"}},
			{ID: "reasoning_effort", Category: terminalproto.AgentOptionThoughtLevel, Values: []string{"low", "medium", "high", "xhigh", "max", "ultra"}},
			{ID: "fast-mode", Category: terminalproto.AgentOptionModelConfig, Values: []string{"off", "on"}},
		}},
	} {
		got := Describe(tc.tool, true, tc.version)
		if got.State != "available" || !got.Valid() {
			t.Fatalf("%s descriptor: %+v", tc.tool, got)
		}
		if len(got.Options) != len(tc.want) {
			t.Fatalf("%s options: %+v", tc.tool, got.Options)
		}
		for i := range tc.want {
			if got.Options[i].ID != tc.want[i].ID || got.Options[i].Category != tc.want[i].Category || !slices.Equal(got.Options[i].Values, tc.want[i].Values) {
				t.Fatalf("%s option %d: %+v, want %+v", tc.tool, i, got.Options[i], tc.want[i])
			}
		}
		// The descriptor is a copy, not the profile's slice.
		spec, _ := Lookup(tc.tool)
		got.Options[0].Values[0] = "mutated"
		if spec.Options[0].Values[0] == "mutated" {
			t.Fatalf("%s descriptor aliased the profile", tc.tool)
		}
	}
	if got := Describe("opencode", true, "1.19.5"); len(got.Options) != 0 || !got.Valid() {
		t.Fatalf("opencode options: %+v", got)
	}
}

// A descriptor claims in-place reattach only for an adapter whose profile can do it, because the connector refuses a
// reattach for any other (ErrUnsupported). Claiming it for the rest would offer a reconnect that can only fail.
func TestDescribeClaimsReattachOnlyWhereTheProfileCanReattach(t *testing.T) {
	want := map[string]bool{"opencode": true, "cline": true, "antigravity": false, "claude": true, "codex": false}
	for tool, reattach := range want {
		spec, ok := Lookup(tool)
		if !ok {
			t.Fatalf("%s has no profile", tool)
		}
		if spec.Reattach != reattach {
			t.Fatalf("%s profile Reattach = %v, want %v", tool, spec.Reattach, reattach)
		}
		got := Describe(tool, true, "")
		if got.State != "available" || !got.Valid() {
			t.Fatalf("%s descriptor: %+v", tool, got)
		}
		if got.Reattach != reattach {
			t.Errorf("%s descriptor claims Reattach = %v, but its profile says %v", tool, got.Reattach, reattach)
		}
	}
}

// Claude Code and Codex are registered like every other agent. Whether an account sees their chats is the controller's
// stored setting; no environment variable on the machine decides it.
func TestClaudeAndCodexAreRegisteredWithoutAnEnvironmentSwitch(t *testing.T) {
	t.Setenv("REET_EXPERIMENTAL_CLAUDE_CHAT", "")
	t.Setenv("REET_EXPERIMENTAL_CODEX_CHAT", "")
	for _, tool := range []string{"claude", "codex"} {
		if _, ok := Lookup(tool); !ok {
			t.Fatalf("%s is not registered", tool)
		}
		if got := Describe(tool, true, ""); got.State != "available" || !got.Valid() {
			t.Fatalf("%s with its adapter installed: %+v", tool, got)
		}
		if got := Describe(tool, false, ""); got.State != "missing" || !got.Valid() {
			t.Fatalf("%s without its adapter: %+v", tool, got)
		}
	}
}
