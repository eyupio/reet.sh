package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func findSnapshotComponent(t *testing.T, preview SnapshotPreview, agentID, kind string) SnapshotComponent {
	t.Helper()
	for _, agent := range preview.Agents {
		if agent.ID != agentID {
			continue
		}
		for _, component := range agent.Components {
			if component.Kind == kind {
				return component
			}
		}
	}
	t.Fatalf("snapshot component %s/%s missing", agentID, kind)
	return SnapshotComponent{}
}

func selectedContent(selection SnapshotSelection, component string) []map[string]any {
	var out []map[string]any
	for _, source := range selection.sources {
		if source.Component == component {
			out = append(out, source.Content)
		}
	}
	return out
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestSnapshotSelectionAppliesPolicyWithoutLeakingValues(t *testing.T) {
	e := fixture(t)
	const secret = "SYNTHETIC-SNAPSHOT-SECRET-DO-NOT-RETURN"
	writeConfig(t, filepath.Join(e.home, ".claude.json"), `{
  "model":"MODEL",
  "permissions":{"allow":["Read"]},
  "env":{"TOKEN":"`+secret+`"},
  "hooks":{"x":"`+secret+`"},
  "projects":{"p":{"history":["`+secret+`"]}},
  "unknown":"`+secret+`",
  "mcpServers":{
    "local":{"command":"server","args":["--x"],"env":{"TOKEN":"`+secret+`"},"headers":{"Authorization":"`+secret+`"}},
    "remote":{"type":"http","url":"https://example.test/?key=`+secret+`"}
  }
}`)
	localPath := filepath.Join(e.home, ".claude", "settings.local.json")
	writeConfig(t, localPath, `{"malformed":"`+secret)
	before, err := os.Stat(localPath)
	if err != nil {
		t.Fatal(err)
	}

	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(r)
	if err != nil {
		t.Fatal(err)
	}
	settings := findSnapshotComponent(t, selection.Preview, "claude", "settings")
	if !hasString(settings.IncludedFields, "model") || !hasString(settings.IncludedFields, "permissions") ||
		!hasString(settings.EncryptedFields, "env") || !hasString(settings.ExcludedFields, "hooks") ||
		!hasString(settings.ExcludedFields, "projects") || settings.UnknownFields != 1 || settings.Digest == "" {
		t.Fatalf("settings selection wrong: %+v", settings)
	}
	mcp := findSnapshotComponent(t, selection.Preview, "claude", "mcp")
	if mcp.Declarations != 2 || mcp.SelectedDeclarations != 2 || !hasString(mcp.IncludedFields, "command") ||
		!hasString(mcp.EncryptedFields, "env") || !hasString(mcp.EncryptedFields, "url") ||
		!hasString(mcp.DeclarationNames, "local") || !hasString(mcp.DeclarationNames, "remote") {
		t.Fatalf("MCP selection wrong: %+v", mcp)
	}

	settingsContent := selectedContent(selection, "settings")
	if len(settingsContent) != 1 || settingsContent[0]["model"] != "MODEL" {
		t.Fatalf("selected settings missing or unexpected: %#v", settingsContent)
	}
	for _, key := range []string{"env", "hooks", "projects", "unknown"} {
		if _, exists := settingsContent[0][key]; exists {
			t.Fatalf("excluded setting selected: %s", key)
		}
	}
	mcpContent := selectedContent(selection, "mcp")
	if len(mcpContent) != 1 {
		t.Fatalf("selected MCP content missing: %#v", mcpContent)
	}
	local, _ := mcpContent[0]["local"].(map[string]any)
	remote, _ := mcpContent[0]["remote"].(map[string]any)
	if local["command"] != "server" || remote["type"] != "http" {
		t.Fatalf("allowed MCP fields missing: %#v", mcpContent[0])
	}
	for _, declaration := range []map[string]any{local, remote} {
		for _, key := range []string{"env", "headers", "url"} {
			if _, exists := declaration[key]; exists {
				t.Fatalf("credential-bearing MCP field selected: %s", key)
			}
		}
	}

	var optIn bool
	for _, omitted := range selection.Preview.Agents[0].Omitted {
		if omitted.Reason == "source_opt_in" && omitted.Source == localPath {
			optIn = true
		}
	}
	if !optIn {
		t.Fatal("local settings were not kept opt-in")
	}
	for _, omitted := range selection.Preview.Agents[0].Omitted {
		if omitted.Source == localPath && strings.HasPrefix(omitted.Reason, "invalid") {
			t.Fatalf("opt-in source was parsed: %+v", omitted)
		}
	}
	for _, source := range selection.sources {
		if source.Path == localPath {
			t.Fatal("opt-in source content was selected")
		}
	}
	after, err := os.Stat(localPath)
	if err != nil || before.ModTime() != after.ModTime() || before.Size() != after.Size() {
		t.Fatal("preview changed the configuration")
	}

	encoded, err := json.Marshal(selection.Preview)
	if err != nil {
		t.Fatal(err)
	}
	var text bytes.Buffer
	if err := renderSnapshotPreview(&text, selection.Preview); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{string(encoded), text.String()} {
		if strings.Contains(output, secret) || strings.Contains(output, "MODEL") || strings.Contains(output, "https://example.test") {
			t.Fatal("snapshot preview exposed a value")
		}
	}
}

func TestSnapshotPreviewCLIIsPreviewOnly(t *testing.T) {
	home := isolatedCLI(t)
	const secret = "SYNTHETIC-SNAPSHOT-CLI-SECRET"
	writeConfig(t, filepath.Join(home, ".claude.json"), `{"model":"`+secret+`","env":{"TOKEN":"`+secret+`"}}`)
	var out bytes.Buffer
	if err := Run([]string{"backup", "--preview", "--json"}, &out); err != nil {
		t.Fatal(err)
	}
	var preview SnapshotPreview
	if err := json.Unmarshal(out.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.SchemaVersion != snapshotPreviewSchemaVersion || preview.PolicyVersion != exportPolicyVersion || len(preview.Agents) != 6 {
		t.Fatalf("wrong preview identity: %+v", preview)
	}
	if strings.Contains(out.String(), secret) {
		t.Fatal("CLI preview exposed a value")
	}
	var refused bytes.Buffer
	if err := Run([]string{"backup"}, &refused); err == nil || !strings.Contains(err.Error(), "passphrase") {
		t.Fatalf("backup without a passphrase was not refused: %v", err)
	}
	var text bytes.Buffer
	if err := Run([]string{"backup", "--preview"}, &text); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "AI setup backup preview") || strings.Contains(text.String(), secret) {
		t.Fatalf("wrong text preview: %s", text.String())
	}
	var help bytes.Buffer
	if err := Run([]string{"--help"}, &help); err != nil || !strings.Contains(help.String(), "setup backup --preview") {
		t.Fatalf("backup help missing: %v %s", err, help.String())
	}
}

func TestSnapshotPreviewOmitsUnsafeAndInvalidSources(t *testing.T) {
	e := fixture(t)
	target := filepath.Join(t.TempDir(), "target.json")
	writeConfig(t, target, `{"model":"SYNTHETIC-SNAPSHOT-SYMLINK"}`)
	if err := os.Symlink(target, filepath.Join(e.home, ".claude.json")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeConfig(t, filepath.Join(e.home, ".codex", "config.toml"), strings.Repeat("x", maxConfigBytes+1))
	writeConfig(t, filepath.Join(e.home, ".config", "opencode", "opencode.json"), `{"mcp":[]}`)

	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(r)
	if err != nil {
		t.Fatal(err)
	}
	reasons := map[string]bool{}
	for _, agent := range selection.Preview.Agents {
		for _, omitted := range agent.Omitted {
			reasons[omitted.Reason] = true
		}
	}
	for _, want := range []string{"source_symlink", "file_too_large", "invalid_mcp_declarations"} {
		if !reasons[want] {
			t.Fatalf("missing omission %q: %+v", want, selection.Preview.Agents)
		}
	}
	for _, source := range selection.sources {
		if strings.Contains(source.Path, ".claude.json") || strings.Contains(source.Path, "config.toml") {
			t.Fatalf("unsafe source selected: %+v", source)
		}
	}
}
