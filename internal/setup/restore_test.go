package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/sealed"
)

func testManifest(id string) sealed.Manifest {
	return sealed.Manifest{ID: id, Kind: setupSnapshotKind, Target: setupSnapshotKind, Version: setupSnapshotVersion, TakenAt: time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC).UnixMilli(), SHA256: "digest"}
}

func planForSelection(t *testing.T, selection SnapshotSelection, destination Report) RestorePlan {
	t.Helper()
	return BuildRestorePlan(BuildSetupSnapshot(selection), testManifest("setup-plan"), destination)
}

func findRestoreAction(t *testing.T, plan RestorePlan, destination string) RestoreAction {
	t.Helper()
	for _, action := range plan.Actions {
		if action.Destination == destination {
			return action
		}
	}
	t.Fatalf("restore action for %q missing: %+v", destination, plan.Actions)
	return RestoreAction{}
}

func TestRestorePlanMapsSourcesAndReportsConflicts(t *testing.T) {
	selection := fixtureSelection(t)
	destination := fixture(t)
	report, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	plan := planForSelection(t, selection, report)
	if plan.SchemaVersion != restorePlanSchemaVersion || plan.SnapshotID != "setup-plan" || plan.Fingerprint == "" || plan.Destination.OS == "" {
		t.Fatalf("incomplete plan: %+v", plan)
	}
	path := filepath.Join(destination.home, ".claude.json")
	action := findRestoreAction(t, plan, path)
	if action.State != "create" || strings.Join(action.Components, ",") != "mcp,settings" || len(action.Sources) != 1 {
		t.Fatalf("wrong create action: %+v", action)
	}
	reasons := map[string]bool{}
	for _, unsupported := range plan.Unsupported {
		reasons[unsupported.Reason] = true
	}
	for _, want := range []string{"class_unsupported", "class_exclude", "reference_names_only"} {
		if !reasons[want] {
			t.Fatalf("missing unsupported reason %q: %+v", want, plan.Unsupported)
		}
	}
	again := planForSelection(t, selection, report)
	if again.Fingerprint != plan.Fingerprint {
		t.Fatal("plan fingerprint is not stable")
	}
	writeConfig(t, path, `{"model":"DESTINATION"}`)
	report, err = discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	conflict := planForSelection(t, selection, report)
	action = findRestoreAction(t, conflict, path)
	if action.State != "conflict" || action.Reason != "destination_source_exists" {
		t.Fatalf("existing file not a conflict: %+v", action)
	}
	if conflict.Fingerprint == plan.Fingerprint {
		t.Fatal("changed destination kept the same fingerprint")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != `{"model":"DESTINATION"}` {
		t.Fatal("planning changed the destination")
	}
	encoded, err := json.Marshal(conflict)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"SYNTHETIC-WRITE-SECRET-DO-NOT-RETURN", "DESTINATION"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("plan exposed %q", secret)
		}
	}
	var text bytes.Buffer
	if err := renderRestorePlan(&text, conflict); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "Plan fingerprint") || strings.Contains(text.String(), "SYNTHETIC-WRITE-SECRET-DO-NOT-RETURN") {
		t.Fatalf("bad plan text: %s", text.String())
	}
}

func TestRestorePlanBlocksSymlinkedDestinationSources(t *testing.T) {
	selection := fixtureSelection(t)
	destination := fixture(t)
	target := filepath.Join(t.TempDir(), "target.json")
	writeConfig(t, target, `{}`)
	if err := os.Symlink(target, filepath.Join(destination.home, ".claude.json")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	report, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	plan := planForSelection(t, selection, report)
	action := findRestoreAction(t, plan, filepath.Join(destination.home, ".claude.json"))
	if action.State != "blocked" || action.Reason != "destination_symlink" {
		t.Fatalf("symlinked destination not blocked: %+v", action)
	}
}

func TestRestorePlanMapsProjectSourcesOnlyWithProject(t *testing.T) {
	e := fixture(t)
	e.project = t.TempDir()
	writeConfig(t, filepath.Join(e.project, ".mcp.json"), `{"mcpServers":{"x":{"command":"server"}}}`)
	sourceReport, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(sourceReport)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := BuildSetupSnapshot(selection)
	if len(snapshot.Sources) != 1 || snapshot.Sources[0].Component != "mcp" {
		t.Fatalf("project source missing: %+v", snapshot.Sources)
	}
	destination := fixture(t)
	noProject, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	plan := BuildRestorePlan(snapshot, testManifest("setup-project"), noProject)
	if len(plan.Actions) != 0 {
		t.Fatalf("project source mapped without a project: %+v", plan.Actions)
	}
	var projectReason bool
	for _, unsupported := range plan.Unsupported {
		if unsupported.Reason == "project_not_selected" {
			projectReason = true
		}
	}
	if !projectReason {
		t.Fatalf("project omission missing: %+v", plan.Unsupported)
	}
	destination.project = t.TempDir()
	withProject, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	plan = BuildRestorePlan(snapshot, testManifest("setup-project"), withProject)
	action := findRestoreAction(t, plan, filepath.Join(destination.project, ".mcp.json"))
	if action.State != "create" || action.Scope != "project" {
		t.Fatalf("project source not mapped: %+v", action)
	}
}

func TestRestorePlanCLI(t *testing.T) {
	home := isolatedCLI(t)
	const secret = "SYNTHETIC-RESTORE-CLI-SECRET"
	writeConfig(t, filepath.Join(home, ".claude.json"), `{"model":"`+secret+`"}`)
	dir := t.TempDir()
	passFile := writePassphrase(t, t.TempDir(), testSetupPassphrase)
	var written bytes.Buffer
	if err := Run([]string{"backup", "--password-file", passFile, "--dir", dir, "--json"}, &written); err != nil {
		t.Fatal(err)
	}
	var result SetupWriteResult
	if err := json.Unmarshal(written.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	var planOut bytes.Buffer
	if err := Run([]string{"restore", "--password-file", passFile, "--dir", dir, "--json", result.ID}, &planOut); err != nil {
		t.Fatal(err)
	}
	var plan RestorePlan
	if err := json.Unmarshal(planOut.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.SnapshotID != result.ID || len(plan.Actions) == 0 || strings.Contains(planOut.String(), secret) || strings.Contains(planOut.String(), testSetupPassphrase) {
		t.Fatalf("bad CLI plan: %+v", plan)
	}
	var text bytes.Buffer
	if err := Run([]string{"restore", "--password-file", passFile, "--dir", dir, result.ID}, &text); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "Setup restore plan") || strings.Contains(text.String(), secret) {
		t.Fatalf("bad restore text: %s", text.String())
	}
	for _, args := range [][]string{
		{"restore", "--apply", "--password-file", passFile, "--dir", dir, result.ID},
		{"restore", "--dir", dir, result.ID},
		{"restore", "--password-file", passFile, "--dir", dir, "missing"},
	} {
		var out bytes.Buffer
		if err := Run(args, &out); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
