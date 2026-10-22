package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func findPolicySource(t *testing.T, r Report, agent, suffix string) SourceExportPolicy {
	t.Helper()
	a := findAgent(t, r, agent)
	if a.ExportPolicy == nil {
		t.Fatal("export policy missing")
	}
	for _, source := range a.ExportPolicy.Sources {
		if strings.HasSuffix(source.Path, suffix) {
			return source
		}
	}
	t.Fatalf("policy source %q missing for %s", suffix, agent)
	return SourceExportPolicy{}
}

func componentKinds(source SourceExportPolicy) []string {
	kinds := make([]string, 0, len(source.Components))
	for _, component := range source.Components {
		kinds = append(kinds, component.Kind)
	}
	return kinds
}

func TestExportPolicyCoversEverySupportedAgentSource(t *testing.T) {
	e := fixture(t)
	e.project = t.TempDir()
	writeConfig(t, filepath.Join(e.home, ".claude", "settings.json"), `{}`)
	writeConfig(t, filepath.Join(e.home, ".claude", "settings.local.json"), `{}`)
	writeConfig(t, filepath.Join(e.home, ".claude.json"), `{}`)
	writeConfig(t, filepath.Join(e.home, ".codex", "config.toml"), "")
	writeConfig(t, filepath.Join(e.home, ".config", "opencode", "opencode.json"), `{}`)
	writeConfig(t, filepath.Join(e.home, ".pi", "agent", "settings.json"), `{}`)
	writeConfig(t, filepath.Join(e.home, ".cursor", "mcp.json"), `{}`)
	writeConfig(t, filepath.Join(e.home, ".gemini", "settings.json"), `{}`)
	writeConfig(t, filepath.Join(e.project, ".mcp.json"), `{}`)

	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	ApplyExportPolicies(&r)
	if r.SchemaVersion != 2 || !r.ExportPolicies {
		t.Fatalf("policy report identity missing: %+v", r)
	}
	for _, agent := range r.Agents {
		if agent.Capabilities.ExportPolicy != "declared_not_implemented" || agent.Capabilities.Backup != "not_implemented" || agent.Capabilities.Restore != "not_implemented" || agent.ExportPolicy == nil {
			t.Fatalf("policy overstated a capability: %+v", agent)
		}
		if agent.ExportPolicy.PolicyVersion != exportPolicyVersion {
			t.Fatalf("wrong policy version: %+v", agent.ExportPolicy)
		}
	}

	for _, tc := range []struct {
		agent, suffix string
		kinds         []string
	}{
		{"claude", ".claude.json", []string{"settings", "mcp"}},
		{"claude", "settings.local.json", []string{"settings", "mcp"}},
		{"claude", ".mcp.json", []string{"mcp"}},
		{"codex", "config.toml", []string{"settings", "mcp"}},
		{"opencode", "opencode.json", []string{"settings", "mcp"}},
		{"pi", "settings.json", []string{"settings"}},
		{"cursor", "mcp.json", []string{"mcp"}},
		{"gemini", "settings.json", []string{"settings", "mcp"}},
	} {
		source := findPolicySource(t, r, tc.agent, tc.suffix)
		if source.Observed != "present" || source.Scope == "" || source.Format == "" {
			t.Fatalf("incomplete source policy: %+v", source)
		}
		got := strings.Join(componentKinds(source), ",")
		if got != strings.Join(tc.kinds, ",") {
			t.Fatalf("%s %s: wrong components %q", tc.agent, tc.suffix, got)
		}
	}
	local := findPolicySource(t, r, "claude", "settings.local.json")
	for _, component := range local.Components {
		if component.Selection != "opt_in" {
			t.Fatalf("local settings not opt-in: %+v", component)
		}
	}
}

func TestExportPolicyClassifiesFieldsWithoutSelectingSecrets(t *testing.T) {
	e := fixture(t)
	const secret = "SYNTHETIC-POLICY-SECRET-DO-NOT-RETURN"
	writeConfig(t, filepath.Join(e.home, ".claude", "settings.json"), `{"env":{"TOKEN":"`+secret+`"},"hooks":{},"apiKeyHelper":"`+secret+`"}`)
	writeConfig(t, filepath.Join(e.home, ".claude.json"), `{"projects":{"x":{"history":["`+secret+`"]}},"oauthAccount":{"token":"`+secret+`"},"mcpServers":{"x":{"command":"`+secret+`","env":{"TOKEN":"`+secret+`"}}}}`)
	writeConfig(t, filepath.Join(e.home, ".codex", "config.toml"), "model_providers = { x = { experimental_bearer_token = '"+secret+"' } }\n")
	writeConfig(t, filepath.Join(e.home, ".config", "opencode", "opencode.json"), `{"provider":{"x":{"options":{"apiKey":"`+secret+`"}}},"mcp":{"x":{"command":["server"],"environment":{"TOKEN":"`+secret+`"}}}}`)
	r := inspectFixture(t, e)
	ApplyExportPolicies(&r)

	validState := map[string]bool{"include": true, "encrypted_only": true, "reference_only": true, "exclude": true}
	validSelection := map[string]bool{"include": true, "opt_in": true, "exclude": true, "unsupported": true}
	validHandling := map[string]bool{"portable": true, "references_only": true, "never": true, "not_implemented": true}
	secretNames := map[string]bool{
		"env": true, "environment": true, "headers": true, "http_headers": true,
		"env_http_headers": true, "provider": true, "providers": true,
		"model_providers": true, "shell_environment_policy": true,
		"apikeyhelper": true, "oauth": true, "credentials": true,
	}
	for _, agent := range r.Agents {
		for _, source := range agent.ExportPolicy.Sources {
			if source.Observed == "" {
				t.Fatalf("unobserved source: %+v", source)
			}
			for _, component := range source.Components {
				if !validSelection[component.Selection] || !validHandling[component.Handling] || component.UnlistedFields != "exclude" {
					t.Fatalf("invalid component policy: %+v", component)
				}
				seen := map[string]bool{}
				for _, field := range component.Fields {
					if !validState[field.State] || seen[field.Path] {
						t.Fatalf("invalid or duplicate field policy: %+v", field)
					}
					seen[field.Path] = true
					name := strings.ToLower(strings.TrimPrefix(field.Path, "*."))
					if field.State == "include" && secretNames[name] {
						t.Fatalf("credential-bearing field selected by default: %+v", field)
					}
					if field.State == "include" && (name == "token" || name == "api_key" || name == "apikey" || strings.Contains(name, "secret") || strings.Contains(name, "credential")) {
						t.Fatalf("credential-like field selected by default: %+v", field)
					}
				}
			}
		}
	}
	claudeSettings := findPolicySource(t, r, "claude", ".claude/settings.json")
	env, hooks := fieldPolicy(claudeSettings, "settings", "env"), fieldPolicy(claudeSettings, "settings", "hooks")
	if env != "encrypted_only" || hooks != "exclude" {
		t.Fatalf("Claude credential/hook policy wrong: env=%q hooks=%q", env, hooks)
	}
	codex := findPolicySource(t, r, "codex", "config.toml")
	if got := fieldPolicy(codex, "settings", "model_providers"); got != "encrypted_only" {
		t.Fatalf("Codex provider credentials not encrypted-only: %q", got)
	}
	opencode := findPolicySource(t, r, "opencode", "opencode.json")
	if got := fieldPolicy(opencode, "settings", "provider"); got != "encrypted_only" {
		t.Fatalf("OpenCode provider credentials not encrypted-only: %q", got)
	}
	if got := fieldPolicy(opencode, "mcp", "*.environment"); got != "encrypted_only" {
		t.Fatalf("OpenCode MCP environment not encrypted-only: %q", got)
	}

	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var text bytes.Buffer
	if err := render(&text, r); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{string(encoded), text.String()} {
		if strings.Contains(output, secret) || strings.Contains(output, "SYNTHETIC") {
			t.Fatal("policy output exposed a configuration value")
		}
	}
}

func fieldPolicy(source SourceExportPolicy, kind, path string) string {
	for _, component := range source.Components {
		if component.Kind != kind {
			continue
		}
		for _, field := range component.Fields {
			if field.Path == path {
				return field.State
			}
		}
	}
	return ""
}

func TestExportPolicyClassifiesMemoryHistoryCredentialsHooksAndCaches(t *testing.T) {
	r, err := discover(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	ApplyExportPolicies(&r)
	policies := findAgent(t, r, "claude").ExportPolicy
	want := map[string]struct{ selection, handling string }{
		"instructions": {"include", "portable"},
		"behavior":     {"unsupported", "not_implemented"},
		"memory":       {"unsupported", "not_implemented"},
		"history":      {"opt_in", "not_implemented"},
		"credentials":  {"include", "references_only"},
		"hooks":        {"exclude", "never"},
		"caches":       {"exclude", "never"},
	}
	if len(policies.Classes) != len(want) {
		t.Fatalf("wrong class count: %+v", policies.Classes)
	}
	for _, class := range policies.Classes {
		expected, ok := want[class.Kind]
		if !ok || class.Selection != expected.selection || class.Handling != expected.handling || class.Reason == "" {
			t.Fatalf("wrong class policy: %+v", class)
		}
	}
}

func TestExportPolicyUnsupportedFormatsAndAdaptersStayExplicit(t *testing.T) {
	for _, tc := range []struct {
		agent, kind, path string
		want              []string
		selection         string
	}{
		{"opencode", "configuration", "/tmp/opencode.yaml", []string{"settings", "mcp"}, "unsupported"},
		{"claude", "configuration", "/tmp/settings.yaml", []string{"settings", "mcp"}, "unsupported"},
		{"pi", "configuration", "/tmp/settings.json", []string{"settings"}, "include"},
		{"pi", "mcp_configuration", "/tmp/mcp.json", []string{"mcp"}, "unsupported"},
		{"cursor", "configuration", "/tmp/mcp.json", []string{"mcp"}, "include"},
	} {
		components := componentExportPolicies(tc.agent, tc.kind, tc.path)
		if got := strings.Join(componentKinds(SourceExportPolicy{Components: components}), ","); got != strings.Join(tc.want, ",") {
			t.Fatalf("%s %s: wrong components %q", tc.agent, tc.path, got)
		}
		for _, component := range components {
			if component.Selection != tc.selection {
				t.Fatalf("%s %s: wrong selection %+v", tc.agent, tc.path, component)
			}
		}
	}
}

func TestExportPolicyCLIIsOptInAndOmitsValues(t *testing.T) {
	home := isolatedCLI(t)
	const secret = "SYNTHETIC-CLI-POLICY-SECRET"
	writeConfig(t, filepath.Join(home, ".claude.json"), `{"model":"`+secret+`","projects":{"x":{"history":["`+secret+`"]}}}`)
	for _, enabled := range []bool{false, true} {
		args := []string{"scan", "--json"}
		if enabled {
			args = append(args, "--export-policy")
		}
		var out bytes.Buffer
		if err := Run(args, &out); err != nil {
			t.Fatal(err)
		}
		var r Report
		if err := json.Unmarshal(out.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		a := findAgent(t, r, "claude")
		if (a.ExportPolicy != nil) != enabled || r.ExportPolicies != enabled || (r.SchemaVersion == 2) != enabled {
			t.Fatalf("opt-in contract broken: %+v", r)
		}
		var text bytes.Buffer
		if enabled {
			if err := Run([]string{"scan", "--export-policy"}, &text); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(text.String(), "Export policy (version 1; declared only") {
				t.Fatalf("text policy missing: %s", text.String())
			}
		}
		if strings.Contains(out.String()+text.String(), secret) {
			t.Fatal("secret leaked through policy scan")
		}
	}
}

func TestExportPolicyDoesNotInspectSymlinkedLocations(t *testing.T) {
	e := fixture(t)
	target := t.TempDir()
	writeConfig(t, filepath.Join(target, "settings.json"), `{}`)
	if err := os.Symlink(target, filepath.Join(e.home, ".claude")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	ApplyExportPolicies(&r)
	if source := findPolicySource(t, r, "claude", "settings.json"); source.Observed != "symlink" {
		t.Fatalf("policy inspected a symlinked location: %+v", source)
	}
}
