package setup

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCodexTOMLSummariesAndCredentialReferences(t *testing.T) {
	e := fixture(t)
	const secret = "SYNTHETIC-PRIVATE-TOML-VALUE"
	writeConfig(t, filepath.Join(e.home, ".codex", "config.toml"), `
model = "`+secret+`"
approval_policy = "on-request"
sandbox_mode = "workspace-write"
[mcp_servers.local]
command = "`+secret+`"
args = ["`+secret+`"]
env_vars = ["PASS_THROUGH"]
[mcp_servers.local.env]
TOKEN = "`+secret+`"
[mcp_servers.remote]
url = "https://example.test/?token=`+secret+`"
bearer_token_env_var = "BEARER_TOKEN"
[mcp_servers.remote.http_headers]
X-Token = "`+secret+`"
[mcp_servers.remote.env_http_headers]
Authorization = "HEADER_TOKEN"
`)
	r := inspectFixture(t, e)
	source := findSource(t, r, "codex", "config.toml")
	if source.State != "parsed" || source.MCPState != "parsed" || len(source.MCP) != 2 || !source.Settings.ModelConfigured || !source.Settings.PermissionsConfigured {
		t.Fatalf("missing Codex summary: %+v", source)
	}
	local, remote := source.MCP[0], source.MCP[1]
	if local.Transport != "stdio" || local.ArgumentCount != 1 || local.EnvironmentFields != 2 || local.ReferenceFields != 1 || local.LiteralFields != 1 {
		t.Fatalf("incorrect local Codex summary: %+v", local)
	}
	if remote.Transport != "http" || remote.HeaderFields != 2 || remote.ReferenceFields != 2 || remote.LiteralFields != 1 {
		t.Fatalf("incorrect remote Codex summary: %+v", remote)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var text bytes.Buffer
	if err := render(&text, r); err != nil {
		t.Fatal(err)
	}
	for _, sensitive := range []string{secret, "PASS_THROUGH", "BEARER_TOKEN", "HEADER_TOKEN", "https://example.test"} {
		if strings.Contains(string(data), sensitive) || strings.Contains(text.String(), sensitive) {
			t.Fatalf("value exposed: %s", sensitive)
		}
	}
}

func TestOpenCodeJSONCCommentsAndTrailingCommas(t *testing.T) {
	e := fixture(t)
	writeConfig(t, filepath.Join(e.home, ".config", "opencode", "opencode.jsonc"), `{
  // comments do not change source provenance
  "permission": {"edit":"ask",},
  "mcp": {
    "local": {"type":"local", "command":["server", "arg",],},
    "remote": {"type":"remote", "url":"https://example.test/a//b/*c*/", "headers":{"Authorization":"{env:TOKEN}"},},
  }, /* final comment */
}`)
	source := findSource(t, inspectFixture(t, e), "opencode", "opencode.jsonc")
	if source.State != "parsed" || source.MCPState != "parsed" || len(source.MCP) != 2 || source.MCP[0].ArgumentCount != 1 || source.MCP[1].ReferenceFields != 1 || !source.Settings.PermissionsConfigured {
		t.Fatalf("JSONC not summarised: %+v", source)
	}
}

func TestJSONCRejectsInvalidOrAmbiguousInput(t *testing.T) {
	for _, input := range []string{
		`{"a":1, "a":2}`, `{"a":1/*comment*/2}`, `{"a":"unterminated}`,
		`{"a":1} /* unterminated`, `{,}`, `{"a":[,]}`, `{"a":[1,,]}`,
		`{"a":,}`, `{"a":1,,}`, `{"a":1} /`, `{'a':1}`, `{} {}`,
		`{"a":` + strings.Repeat("[", 65) + `0` + strings.Repeat("]", 65) + `}`,
	} {
		if _, err := parseSource([]byte(input), "jsonc"); err == nil {
			t.Fatalf("accepted invalid JSONC beginning %.60q", input)
		}
	}
	for _, input := range []string{
		`{"a":"https://a//b/*c*/", "b":"escaped \" quote",}`,
		"// lead\n{\"a\":1,}\n// eof",
		"{\"a\":[1,/* comment */],\"b\":{},}",
	} {
		if _, err := parseSource([]byte(input), "jsonc"); err != nil {
			t.Fatalf("rejected valid JSONC: %v", err)
		}
	}
}

func TestTOMLRejectsDuplicatesAndExcessiveTrees(t *testing.T) {
	for _, input := range []string{
		"a=1\na=2", "[a]\nb=1\n[a]\nb=2", "a='unterminated",
		"a=" + strings.Repeat("[", 65) + "0" + strings.Repeat("]", 65),
		"[" + strings.Repeat("a.", 65) + "z]\nx=1",
		"a=[" + strings.Repeat("0,", 20000) + "0]",
	} {
		if _, err := parseSource([]byte(input), "toml"); err == nil {
			t.Fatalf("accepted invalid/complex TOML beginning %.60q", input)
		}
	}
}

func TestFormatErrorsDoNotExposeInput(t *testing.T) {
	for _, tc := range []struct{ agent, dir, file, input string }{
		{"codex", ".codex", "config.toml", `model = "SYNTHETIC-MALFORMED-SECRET`},
		{"opencode", ".config/opencode", "opencode.jsonc", `{"model":"SYNTHETIC-MALFORMED-SECRET`},
	} {
		e := fixture(t)
		writeConfig(t, filepath.Join(e.home, tc.dir, tc.file), tc.input)
		r := inspectFixture(t, e)
		source := findSource(t, r, tc.agent, tc.file)
		if source.State != "error" || !strings.HasPrefix(source.Issue, "invalid_or_complex_") || source.MCPState != "" {
			t.Fatalf("bad error contract: %+v", source)
		}
		data, err := json.Marshal(r)
		if err != nil || strings.Contains(string(data), "SYNTHETIC-MALFORMED-SECRET") {
			t.Fatalf("parser error leaked input: %v", err)
		}
	}
}

func TestCodexInvalidReferenceShapesArePartial(t *testing.T) {
	for _, extra := range []string{
		`env_vars = "TOKEN"`, `env_vars = [123]`, `env_vars = ["invalid name"]`,
		`bearer_token_env_var = false`, `env_http_headers = { Authorization = 123 }`,
	} {
		e := fixture(t)
		writeConfig(t, filepath.Join(e.home, ".codex", "config.toml"), "[mcp_servers.remote]\nurl = 'https://example.test'\n"+extra)
		source := findSource(t, inspectFixture(t, e), "codex", "config.toml")
		if source.MCPState != "partial" || source.MCP[0].State != "unsupported" {
			t.Fatalf("invalid reference accepted: %+v", source)
		}
	}
}

func TestFormatSupportIsScopedToAdapters(t *testing.T) {
	for _, tc := range []struct {
		format, agent string
		supported     bool
	}{
		{"toml", "codex", true}, {"jsonc", "opencode", true}, {"json", "pi", true},
		{"yaml", "opencode", false}, {"toml", "claude", false}, {"jsonc", "cursor", false},
	} {
		if got := supportedFormat(tc.format, tc.agent); got != tc.supported {
			t.Errorf("%s/%s: %v", tc.agent, tc.format, got)
		}
	}
	if _, err := parseSource([]byte(strings.Repeat(" ", maxConfigBytes+1)), "toml"); err == nil {
		t.Fatal("unbounded format input")
	}
}

func FuzzJSONCPreservesStrictJSON(f *testing.F) {
	for _, seed := range []string{`{}`, `{"a":"https://a//b/*c*/","b":"\\\""}`, `{"a":[1,2],"b":{"c":true}}`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > maxConfigBytes {
			t.Skip()
		}
		strict, err := parseConfig([]byte(input))
		if err != nil {
			return
		}
		comments, err := parseSource([]byte(input), "jsonc")
		if err != nil || !reflect.DeepEqual(strict, comments) {
			t.Fatalf("JSONC changed valid JSON: %v", err)
		}
	})
}
