package setup

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSupportMatrixCLI(t *testing.T) {
	var out bytes.Buffer
	if err := Run([]string{"support", "--json"}, &out); err != nil {
		t.Fatal(err)
	}
	var matrix SupportMatrix
	if err := json.Unmarshal(out.Bytes(), &matrix); err != nil {
		t.Fatal(err)
	}
	if matrix.SchemaVersion != setupSupportSchemaVersion || len(matrix.Agents) != 6 || matrix.OS == "" || matrix.Architecture == "" {
		t.Fatalf("incomplete support matrix: %+v", matrix)
	}
	byID := map[string]SupportRow{}
	for _, row := range matrix.Agents {
		byID[row.AgentID] = row
	}
	if byID["claude"].Settings != "supported" || byID["pi"].MCP != "unsupported" || byID["cursor"].Settings != "unsupported" || byID["gemini"].Skills != "unsupported" || byID["codex"].Memory != "supported" {
		t.Fatalf("wrong support rows: %+v", byID)
	}
	if len(matrix.Limitations) == 0 || !strings.Contains(strings.Join(matrix.Limitations, " "), "not runtime verification") {
		t.Fatalf("limitations missing: %+v", matrix)
	}
	var text bytes.Buffer
	if err := Run([]string{"support"}, &text); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "Claude Code") || !strings.Contains(text.String(), "restore=files_and_fields") || !strings.Contains(text.String(), "AI agent setup support") {
		t.Fatalf("bad support text: %s", text.String())
	}
	for _, output := range []string{out.String(), text.String()} {
		if strings.Contains(output, "password") || strings.Contains(output, "token=") {
			t.Fatal("support matrix exposed a secret")
		}
	}
}
