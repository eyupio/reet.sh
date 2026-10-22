package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rollbackFixture(t *testing.T) (SnapshotSelection, Report, string, string) {
	t.Helper()
	e := fixture(t)
	writeConfig(t, filepath.Join(e.home, ".claude.json"), `{"model":"SOURCE-CLAUDE"}`)
	writeConfig(t, filepath.Join(e.home, ".codex", "config.toml"), "model = 'SOURCE-CODEX'\n")
	source, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := fixture(t)
	claude := filepath.Join(destination.home, ".claude.json")
	writeConfig(t, claude, `{"model":"KEEP-CLAUDE"}`)
	report, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	return selection, report, destination.home, claude
}

func TestRollbackRestoreReversesCreateAndReplace(t *testing.T) {
	selection, report, home, claude := rollbackFixture(t)
	plan := planForSelection(t, selection, report)
	result, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir(), Conflict: "replace"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Replaced) != 1 || len(result.Created) != 1 || len(result.Backups) != 1 {
		t.Fatalf("wrong apply result: %+v", result)
	}
	codex := filepath.Join(home, ".codex", "config.toml")
	if _, err := os.Stat(codex); err != nil {
		t.Fatal(err)
	}
	updated, err := discover(fixtureWithHome(t, home))
	if err != nil {
		t.Fatal(err)
	}
	rollback, err := RollbackRestore(result.JournalPath, BuildSetupSnapshot(selection), testManifest("setup-plan"), updated, RestoreRollbackOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rollback.Restored) != 1 || rollback.Restored[0] != claude || len(rollback.Removed) != 1 || rollback.Removed[0] != codex {
		t.Fatalf("wrong rollback result: %+v", rollback)
	}
	data, err := os.ReadFile(claude)
	if err != nil || string(data) != `{"model":"KEEP-CLAUDE"}` {
		t.Fatalf("original destination not restored: %q %v", data, err)
	}
	if _, err := os.Stat(codex); !os.IsNotExist(err) {
		t.Fatal("created file was not removed")
	}
	var record restoreJournal
	journalData, err := os.ReadFile(result.JournalPath)
	if err != nil || json.Unmarshal(journalData, &record) != nil || record.State != "rolled_back" {
		t.Fatalf("journal state wrong: %+v %v", record, err)
	}
	// Rerunning a rolled-back journal is a no-op.
	again, err := RollbackRestore(result.JournalPath, BuildSetupSnapshot(selection), testManifest("setup-plan"), updated, RestoreRollbackOptions{})
	if err != nil || len(again.Removed) != 0 || len(again.Restored) != 0 {
		t.Fatalf("rollback rerun was not idempotent: %+v %v", again, err)
	}
}

func TestRollbackRestoreRefusesChangedDestinations(t *testing.T) {
	selection, report, home, _ := rollbackFixture(t)
	plan := planForSelection(t, selection, report)
	result, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir(), Conflict: "replace"})
	if err != nil {
		t.Fatal(err)
	}
	codex := filepath.Join(home, ".codex", "config.toml")
	writeConfig(t, codex, "model = 'USER-CHANGED'\n")
	updated, err := discover(fixtureWithHome(t, home))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RollbackRestore(result.JournalPath, BuildSetupSnapshot(selection), testManifest("setup-plan"), updated, RestoreRollbackOptions{}); err == nil {
		t.Fatal("changed destination was rolled back")
	}
	after, err := os.ReadFile(codex)
	if err != nil || string(after) != "model = 'USER-CHANGED'\n" {
		t.Fatal("refused rollback changed the destination")
	}
	// The replaced file must not be restored when any preflight check fails.
	if _, err := os.Stat(result.Backups[0]); err != nil {
		t.Fatal(err)
	}
}

func TestRollbackRestoreRefusesWrongSnapshotAndJournalShape(t *testing.T) {
	selection, report, home, _ := rollbackFixture(t)
	plan := planForSelection(t, selection, report)
	result, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir(), Conflict: "replace"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := discover(fixtureWithHome(t, home))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RollbackRestore(result.JournalPath, BuildSetupSnapshot(selection), testManifest("other-snapshot"), updated, RestoreRollbackOptions{}); err == nil {
		t.Fatal("journal for another snapshot was accepted")
	}
	invalid := filepath.Join(t.TempDir(), "invalid.json")
	writeConfig(t, invalid, `{"schema_version":2,"state":"completed"}`)
	if _, err := RollbackRestore(invalid, BuildSetupSnapshot(selection), testManifest("setup-plan"), updated, RestoreRollbackOptions{}); err == nil {
		t.Fatal("journal without actions was accepted")
	}
}

func TestRollbackRestoreCLI(t *testing.T) {
	sourceHome := isolatedCLI(t)
	writeConfig(t, filepath.Join(sourceHome, ".claude.json"), `{"model":"SOURCE-MODEL"}`)
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
	path := filepath.Join(destination, ".claude.json")
	writeConfig(t, path, `{"model":"KEEP-MODEL"}`)
	t.Setenv("HOME", destination)
	t.Setenv("USERPROFILE", destination)
	var applied bytes.Buffer
	if err := Run([]string{"restore", "--apply", "--on-conflict", "replace", "--password-file", passFile, "--dir", dir, "--json", snapshot.ID}, &applied); err != nil {
		t.Fatal(err)
	}
	var result RestoreApplyResult
	if err := json.Unmarshal(applied.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	var rolled bytes.Buffer
	if err := Run([]string{"rollback", "--journal", result.JournalPath, "--password-file", passFile, "--dir", dir, "--json", snapshot.ID}, &rolled); err != nil {
		t.Fatal(err)
	}
	var rollback RestoreRollbackResult
	if err := json.Unmarshal(rolled.Bytes(), &rollback); err != nil {
		t.Fatal(err)
	}
	if len(rollback.Restored) != 1 || strings.Contains(rolled.String(), "SOURCE-MODEL") || strings.Contains(rolled.String(), testSetupPassphrase) {
		t.Fatalf("bad rollback result: %+v", rollback)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != `{"model":"KEEP-MODEL"}` {
		t.Fatalf("CLI rollback did not restore: %q %v", data, err)
	}
	var text bytes.Buffer
	if err := Run([]string{"rollback", "--journal", result.JournalPath, "--password-file", passFile, "--dir", dir, snapshot.ID}, &text); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "Nothing needed changing") {
		t.Fatalf("idempotent text rollback missing: %s", text.String())
	}
	if err := Run([]string{"rollback", "--password-file", passFile, "--dir", dir, snapshot.ID}, &bytes.Buffer{}); err == nil {
		t.Fatal("rollback without a journal was accepted")
	}
}
