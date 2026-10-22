package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstructionsSelectionSnapshotRestoreAndVerify(t *testing.T) {
	e := fixture(t)
	e.project = t.TempDir()
	const secret = "SYNTHETIC-INSTRUCTION-SECRET"
	files := map[string]string{
		"CLAUDE.md":    "Claude instructions\n" + secret,
		"AGENTS.md":    "Shared agent instructions\n" + secret,
		".cursorrules": "Cursor rules\n" + secret,
		"GEMINI.md":    "Gemini instructions\n" + secret,
	}
	for name, body := range files {
		writeConfig(t, filepath.Join(e.project, name), body)
	}
	source, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.sources) != len(files) {
		t.Fatalf("instructions not selected: %+v", selection.sources)
	}
	for _, source := range selection.sources {
		if source.Component != "instructions" || source.Text == "" {
			t.Fatalf("wrong instruction source: %+v", source)
		}
	}
	previewJSON, err := json.Marshal(selection.Preview)
	if err != nil || strings.Contains(string(previewJSON), secret) {
		t.Fatal("preview exposed instruction text")
	}
	dir := t.TempDir()
	snapshot, err := WriteSetupSnapshot(selection, SetupWriteOptions{Dir: dir, Password: testSetupPassphrase})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(snapshot.Path)
	if err != nil || bytes.Contains(raw, []byte(secret)) || bytes.Contains(raw, []byte("Claude instructions")) {
		t.Fatal("instruction text was not sealed")
	}
	inspection, err := InspectSetupSnapshot(snapshot.ID, testSetupPassphrase, dir)
	if err != nil || len(inspection.Sources) != len(files) {
		t.Fatalf("inspection missing instructions: %+v %v", inspection.Sources, err)
	}
	inspectionJSON, err := json.Marshal(inspection)
	if err != nil || strings.Contains(string(inspectionJSON), secret) {
		t.Fatal("inspection exposed instruction text")
	}

	destination := fixture(t)
	destination.project = t.TempDir()
	report, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	plan := BuildRestorePlan(BuildSetupSnapshot(selection), testManifest("setup-instructions"), report)
	if len(plan.Actions) != len(files) {
		t.Fatalf("instructions not planned: %+v", plan.Actions)
	}
	for _, action := range plan.Actions {
		if action.State != "create" || action.Format != "text" {
			t.Fatalf("wrong instruction action: %+v", action)
		}
	}
	result, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Created) != len(files) {
		t.Fatalf("instructions not restored: %+v", result.Created)
	}
	for name, body := range files {
		path := filepath.Join(destination.project, name)
		data, err := os.ReadFile(path)
		if err != nil || string(data) != body {
			t.Fatalf("restored instruction %s wrong: %q %v", name, data, err)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("instruction permissions for %s: %+v %v", name, info, err)
		}
	}
	destEnv := fixtureWithHome(t, destination.home)
	destEnv.project = destination.project
	verifiedReport, err := discover(destEnv)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := BuildSnapshotSelection(source)
	if err != nil {
		t.Fatal(err)
	}
	resultVerify := VerifySetupSnapshot(BuildSetupSnapshot(verified), testManifest("setup-instructions"), verifiedReport)
	if resultVerify.Summary.Verified != len(files) || resultVerify.Summary.Changed != 0 {
		t.Fatalf("instructions not verified: %+v", resultVerify)
	}
	writeConfig(t, filepath.Join(destination.project, "CLAUDE.md"), "changed")
	verifiedReport, err = discover(destEnv)
	if err != nil {
		t.Fatal(err)
	}
	changed := VerifySetupSnapshot(BuildSetupSnapshot(verified), testManifest("setup-instructions"), verifiedReport)
	if changed.Summary.Changed != 1 {
		t.Fatalf("changed instruction not reported: %+v", changed)
	}
	if strings.Contains(string(mustJSON(t, changed)), secret) {
		t.Fatal("verify exposed instruction text")
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGlobalInstructionsDiscoveryExportRestore(t *testing.T) {
	e := fixture(t)
	const secret = "SYNTHETIC-GLOBAL-INSTRUCTION-SECRET"
	files := map[string]string{
		".claude/CLAUDE.md":          "Claude global\n" + secret,
		".codex/AGENTS.md":           "Codex global\n" + secret,
		".config/opencode/AGENTS.md": "OpenCode global\n" + secret,
		".pi/agent/AGENTS.md":        "Pi global\n" + secret,
		".gemini/GEMINI.md":          "Gemini global\n" + secret,
	}
	for name, body := range files {
		writeConfig(t, filepath.Join(e.home, filepath.FromSlash(name)), body)
	}
	source, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, agent := range source.Agents {
		for _, loc := range agent.Locations {
			if loc.Kind == "instructions" && loc.State == "present" {
				seen[agent.ID] = true
			}
		}
	}
	if len(seen) != 5 {
		t.Fatalf("global instructions not discovered: %+v", seen)
	}
	selection, err := BuildSnapshotSelection(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.sources) != len(files) {
		t.Fatalf("global instructions not selected: %+v", selection.sources)
	}
	if strings.Contains(string(mustJSON(t, selection.Preview)), secret) {
		t.Fatal("global instruction text leaked in preview")
	}
	destination := fixture(t)
	report, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	plan := BuildRestorePlan(BuildSetupSnapshot(selection), testManifest("setup-global-instructions"), report)
	if len(plan.Actions) != len(files) {
		t.Fatalf("global instructions not planned: %+v", plan.Actions)
	}
	result, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Created) != len(files) {
		t.Fatalf("global instructions not restored: %+v", result.Created)
	}
	for name, body := range files {
		data, err := os.ReadFile(filepath.Join(destination.home, filepath.FromSlash(name)))
		if err != nil || string(data) != body {
			t.Fatalf("global instruction %s wrong: %q %v", name, data, err)
		}
	}
	updated, err := discover(fixtureWithHome(t, destination.home))
	if err != nil {
		t.Fatal(err)
	}
	verified := VerifySetupSnapshot(BuildSetupSnapshot(selection), testManifest("setup-global-instructions"), updated)
	if verified.Summary.Verified != len(files) {
		t.Fatalf("global instructions not verified: %+v", verified)
	}
}
