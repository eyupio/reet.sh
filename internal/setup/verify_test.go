package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifySetupSnapshotAfterApply(t *testing.T) {
	selection, report, home := applyFixture(t)
	plan := planForSelection(t, selection, report)
	if _, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	updated, err := discover(fixtureWithHome(t, home))
	if err != nil {
		t.Fatal(err)
	}
	result := VerifySetupSnapshot(BuildSetupSnapshot(selection), testManifest("setup-plan"), updated)
	if result.SchemaVersion != setupVerifySchemaVersion || result.SnapshotID != "setup-plan" || result.Summary.Verified != 2 || result.Summary.Changed != 0 || result.Summary.Missing != 0 {
		t.Fatalf("wrong verification: %+v", result)
	}
	path := filepath.Join(home, ".claude.json")
	// Extra destination fields do not invalidate selected content.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	object["extra"] = "ignored"
	data, _ = json.Marshal(object)
	writeConfig(t, path, string(data))
	extra := VerifySetupSnapshot(BuildSetupSnapshot(selection), testManifest("setup-plan"), updated)
	if extra.Summary.Verified != 2 {
		t.Fatalf("extra destination field failed verification: %+v", extra)
	}
	// A changed selected field is reported without values.
	object["model"] = "CHANGED"
	data, _ = json.Marshal(object)
	writeConfig(t, path, string(data))
	infoBefore, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := VerifySetupSnapshot(BuildSetupSnapshot(selection), testManifest("setup-plan"), updated)
	if changed.Summary.Changed < 1 || changed.Summary.Verified != 1 {
		t.Fatalf("changed value not reported: %+v", changed)
	}
	var mismatch string
	for _, component := range changed.Components {
		if len(component.Mismatches) > 0 {
			mismatch = component.Mismatches[0]
		}
	}
	if mismatch != "model" {
		t.Fatalf("wrong mismatch list: %+v", changed.Components)
	}
	encoded, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"SYNTHETIC-APPLY-SECRET-DO-NOT-RETURN", "CHANGED"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("verification exposed %q", secret)
		}
	}
	// A missing destination is reported, and verification never writes.
	infoAfter, err := os.Stat(path)
	if err != nil || !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
		t.Fatal("verification changed the destination")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	missing := VerifySetupSnapshot(BuildSetupSnapshot(selection), testManifest("setup-plan"), updated)
	if missing.Summary.Missing != 2 || missing.Summary.Verified != 0 {
		t.Fatalf("missing destination not reported: %+v", missing)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("verification recreated the destination")
	}
}

func TestVerifySetupSnapshotProjectSources(t *testing.T) {
	e := fixture(t)
	e.project = t.TempDir()
	writeConfig(t, filepath.Join(e.project, ".mcp.json"), `{"mcpServers":{"x":{"command":"server"}}}`)
	source, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(source)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := BuildSetupSnapshot(selection)
	destination := fixture(t)
	noProject, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	result := VerifySetupSnapshot(snapshot, testManifest("setup-project"), noProject)
	if result.Summary.Unsupported != 1 || result.Components[0].Reason != "project_not_selected" {
		t.Fatalf("project source not unsupported: %+v", result)
	}
	destination.project = t.TempDir()
	withProject, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	plan := BuildRestorePlan(snapshot, testManifest("setup-project"), withProject)
	if _, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	withProject, err = discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	verified := VerifySetupSnapshot(snapshot, testManifest("setup-project"), withProject)
	if verified.Summary.Verified != 1 || verified.Summary.Missing != 0 {
		t.Fatalf("project source not verified: %+v", verified)
	}
}

func TestVerifySetupSnapshotCLI(t *testing.T) {
	sourceHome := isolatedCLI(t)
	const secret = "SYNTHETIC-VERIFY-CLI-SECRET"
	writeConfig(t, filepath.Join(sourceHome, ".claude.json"), `{"model":"`+secret+`"}`)
	dir := t.TempDir()
	passFile := writePassphrase(t, t.TempDir(), testSetupPassphrase)
	var written bytes.Buffer
	if err := Run([]string{"backup", "--password-file", passFile, "--dir", dir, "--json"}, &written); err != nil {
		t.Fatal(err)
	}
	var snapshot SetupWriteResult
	if err := json.Unmarshal(written.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	t.Setenv("HOME", destination)
	t.Setenv("USERPROFILE", destination)
	if err := Run([]string{"restore", "--apply", "--password-file", passFile, "--dir", dir, snapshot.ID}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var verified bytes.Buffer
	if err := Run([]string{"verify", "--password-file", passFile, "--dir", dir, "--json", snapshot.ID}, &verified); err != nil {
		t.Fatal(err)
	}
	var result SetupVerifyResult
	if err := json.Unmarshal(verified.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Summary.Verified != 1 || strings.Contains(verified.String(), secret) || strings.Contains(verified.String(), testSetupPassphrase) {
		t.Fatalf("bad CLI verification: %+v", result)
	}
	if result.Limitations[1] != "No agent executable is run; application versions, authentication, MCP health and runtime behaviour are not verified." {
		t.Fatalf("runtime limitation missing: %+v", result.Limitations)
	}
	var text bytes.Buffer
	if err := Run([]string{"verify", "--password-file", passFile, "--dir", dir, snapshot.ID}, &text); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "Setup verification") || strings.Contains(text.String(), secret) {
		t.Fatalf("bad verification text: %s", text.String())
	}
	for _, args := range [][]string{
		{"verify", "--dir", dir, snapshot.ID},
		{"verify", "--password-file", passFile, "--dir", dir, "missing"},
	} {
		if err := Run(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
