package setup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, path, data string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func findSource(t *testing.T, r Report, agent, suffix string) ConfigSource {
	t.Helper()
	a := findAgent(t, r, agent)
	if a.Components == nil {
		t.Fatal("component inspection missing")
	}
	for _, source := range a.Components.Sources {
		if strings.HasSuffix(source.Path, suffix) {
			return source
		}
	}
	t.Fatalf("source %q missing for %s", suffix, agent)
	return ConfigSource{}
}

func inspectFixture(t *testing.T, e environment) Report {
	t.Helper()
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	InspectComponents(&r)
	return r
}

func TestComponentSummariesOmitAllValues(t *testing.T) {
	e := fixture(t)
	const secret = "SYNTHETIC-SECRET-DO-NOT-RETURN"
	path := filepath.Join(e.home, ".claude.json")
	data := `{"model":"` + secret + `","hooks":{"anything":"` + secret + `"},"permissions":{"allow":["` + secret + `"]},"unknown":{"key":"` + secret + `"},"mcpServers":{"local":{"command":"` + secret + `","args":["` + secret + `"],"env":{"TOKEN":"` + secret + `","REF":"${API_KEY}"}},"remote":{"type":"http","url":"https://example.test/?key=` + secret + `","headers":{"Authorization":"` + secret + `"}}}}`
	writeConfig(t, path, data)
	r := inspectFixture(t, e)
	source := findSource(t, r, "claude", ".claude.json")
	if source.State != "parsed" || source.MCPState != "parsed" || len(source.MCP) != 2 || !source.Settings.ModelConfigured || !source.Settings.HooksConfigured || !source.Settings.PermissionsConfigured {
		t.Fatalf("missing summary: %+v", source)
	}
	local := source.MCP[0]
	if local.Name != "local" || local.Transport != "stdio" || local.ArgumentCount != 1 || local.EnvironmentFields != 2 || local.ReferenceFields != 1 || local.LiteralFields != 1 {
		t.Fatalf("incorrect MCP summary: %+v", local)
	}
	if source.MCP[1].HeaderFields != 1 || source.MCP[1].LiteralFields != 1 {
		t.Fatalf("missing credential hints: %+v", source.MCP[1])
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
		if strings.Contains(output, secret) || strings.Contains(output, "API_KEY") || strings.Contains(output, "https://example.test") {
			t.Fatal("configuration values leaked into inventory")
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != data {
		t.Fatal("scan changed the configuration")
	}
}

func TestComponentFormatsAndAgentSpecificMCP(t *testing.T) {
	e := fixture(t)
	e.project = t.TempDir()
	writeConfig(t, filepath.Join(e.home, ".codex", "config.toml"), "[mcp_servers.secret]\ncommand = 'DO-NOT-PARSE'")
	writeConfig(t, filepath.Join(e.home, ".config", "opencode", "opencode.jsonc"), "// pending support\n{}")
	writeConfig(t, filepath.Join(e.project, "opencode.json"), `{"mcp":{"remote":{"type":"remote","url":"https://example.test","headers":{"Authorization":"{env:TOKEN}"}},"local":{"type":"local","command":["npx","server"],"environment":{"TOKEN":"literal"}}}}`)
	writeConfig(t, filepath.Join(e.home, ".pi", "agent", "settings.json"), `{"defaultModel":"model","packages":["npm:package"],"mcpServers":{"ignored":{}}}`)
	writeConfig(t, filepath.Join(e.home, ".gemini", "settings.json"), `{"mcpServers":{"remote":{"httpUrl":"https://example.test"}}}`)
	writeConfig(t, filepath.Join(e.home, ".cursor", "mcp.json"), `{"mcpServers":{"local":{"command":"server"}}}`)
	r := inspectFixture(t, e)
	for _, tc := range []struct{ agent, suffix string }{{"codex", "config.toml"}, {"opencode", "opencode.jsonc"}} {
		source := findSource(t, r, tc.agent, tc.suffix)
		if source.State != "unsupported" || source.Issue != "format_not_supported" || source.Settings != nil || len(source.MCP) != 0 {
			t.Fatalf("unsupported format claimed as parsed: %+v", source)
		}
	}
	var opencode ConfigSource
	for _, source := range findAgent(t, r, "opencode").Components.Sources {
		if source.Path == filepath.Join(e.project, "opencode.json") {
			opencode = source
		}
	}
	if opencode.Scope != "project" || opencode.MCPState != "parsed" || len(opencode.MCP) != 2 || opencode.MCP[0].ArgumentCount != 1 || opencode.MCP[1].ReferenceFields != 1 {
		t.Fatalf("incorrect OpenCode declaration: %+v", opencode)
	}
	pi := findSource(t, r, "pi", "settings.json")
	if !pi.Settings.ModelConfigured || !pi.Settings.PackagesConfigured || pi.MCPState != "not_supported_by_adapter" || len(pi.MCP) != 0 {
		t.Fatalf("Pi support overstated: %+v", pi)
	}
	gemini := findSource(t, r, "gemini", "settings.json")
	if gemini.MCPState != "parsed" || gemini.MCP[0].Transport != "http" {
		t.Fatalf("Gemini HTTP config missing: %+v", gemini)
	}
	if source := findSource(t, r, "cursor", "mcp.json"); source.MCPState != "parsed" {
		t.Fatalf("Cursor MCP missing: %+v", source)
	}
}

func TestMalformedAndPartialSourcesNeverClaimAbsence(t *testing.T) {
	for _, tc := range []struct{ input, state, mcp string }{
		{`{"secret":"unterminated`, "error", ""},
		{`{"mcpServers":{},"mcpServers":{"hidden":{}}}`, "error", ""},
		{`{"mcpServers":[]}`, "parsed", "unsupported"},
		{`{"mcpServers":{"bad":false,"good":{"command":"server"}}}`, "parsed", "partial"},
		{`{"mcpServers":{"bad":{"type":123,"command":"server"}}}`, "parsed", "partial"},
		{`{"mcpServers":{"bad":{"command":"server","env":{"TOKEN":123}}}}`, "parsed", "partial"},
		{`{}`, "parsed", "absent_in_file"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			e := fixture(t)
			writeConfig(t, filepath.Join(e.home, ".claude.json"), tc.input)
			source := findSource(t, inspectFixture(t, e), "claude", ".claude.json")
			if source.State != tc.state || source.MCPState != tc.mcp {
				t.Fatalf("wrong completeness: %+v", source)
			}
			if source.State == "error" && source.Issue != "invalid_or_complex_json" {
				t.Fatalf("parser error should be a constant code: %+v", source)
			}
		})
	}
}

func TestConfigParserLimitsAndStrictness(t *testing.T) {
	for _, input := range []string{
		`null`, `[]`, `{} {}`, `{"a":1,}`, `/* comment */ {}`,
		`{"nested":{"a":1,"a":2}}`,
		`{"a":` + strings.Repeat("[", 65) + `0` + strings.Repeat("]", 65) + `}`,
		`{"a":[` + strings.Repeat("0,", 20000) + `0]}`,
	} {
		if _, err := parseConfig([]byte(input)); err == nil {
			t.Fatalf("accepted invalid/complex input beginning %.60q", input)
		}
	}
	if _, err := parseConfig([]byte(`{"n":123456789012345678901234567890,"array":[true,null,"string"]}`)); err != nil {
		t.Fatal(err)
	}
}

func TestConfigReadLimitsAndSymlinks(t *testing.T) {
	e := fixture(t)
	path := filepath.Join(e.home, ".claude.json")
	writeConfig(t, path, strings.Repeat(" ", maxConfigBytes+1))
	if source := findSource(t, inspectFixture(t, e), "claude", ".claude.json"); source.Issue != "file_too_large" {
		t.Fatalf("size limit not enforced: %+v", source)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target.json")
	writeConfig(t, target, `{"mcpServers":{"secret":{"command":"secret"}}}`)
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if source := findSource(t, inspectFixture(t, e), "claude", ".claude.json"); source.State != "symlink" || source.Settings != nil {
		t.Fatalf("symlink content inspected: %+v", source)
	}
}

func TestConfigReadRejectsReplacementAndMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfig(t, path, `{}`)
	before, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if _, issue := readConfig(path, before); issue != "read_unavailable" {
		t.Fatalf("missing file accepted: %q", issue)
	}
	writeConfig(t, path, `{}`)
	if data, issue := readConfig(path, before); issue != "file_changed" || data != nil {
		t.Fatalf("replacement accepted: %q", issue)
	}
}

func TestReferenceHintsAreConservative(t *testing.T) {
	for _, value := range []string{"${TOKEN}", "{env:TOKEN_2}"} {
		if !pureReference(value) {
			t.Errorf("reference not recognised: %s", value)
		}
	}
	for _, value := range []any{"literal", "Bearer ${TOKEN}", "${TOKEN:-secret}", "${}", "{env:}", "${2INVALID}", 123, nil} {
		if pureReference(value) {
			t.Errorf("literal incorrectly classified: %v", value)
		}
	}
}

func TestComponentCLIIsOptInAndUsesSchemaTwo(t *testing.T) {
	home := isolatedCLI(t)
	writeConfig(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"test":{"command":"server"}}}`)
	for _, enabled := range []bool{false, true} {
		args := []string{"scan", "--json"}
		if enabled {
			args = append(args, "--components")
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
		if (a.Components != nil) != enabled || (r.SchemaVersion == 2) != enabled || a.Capabilities.Restore != "not_implemented" {
			t.Fatalf("opt-in/schema contract broken: %+v", r)
		}
	}
}

func TestExcessiveServerListsAndNamesAreReported(t *testing.T) {
	e := fixture(t)
	servers := map[string]any{}
	for i := 0; i < 257; i++ {
		servers[fmt.Sprint(i)] = map[string]any{"command": "server"}
	}
	data, err := json.Marshal(map[string]any{"mcpServers": servers})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(e.home, ".claude.json")
	writeConfig(t, path, string(data))
	source := findSource(t, inspectFixture(t, e), "claude", ".claude.json")
	if source.MCPState != "unsupported" || len(source.MCP) != 0 {
		t.Fatalf("server count not bounded: %+v", source)
	}
	writeConfig(t, path, `{"mcpServers":{"`+strings.Repeat("x", 257)+`":{"command":"server"}}}`)
	source = findSource(t, inspectFixture(t, e), "claude", ".claude.json")
	if source.MCPState != "partial" || source.Issue != "mcp_name_too_long" || len(source.MCP) != 0 {
		t.Fatalf("long name silently lost: %+v", source)
	}
}
