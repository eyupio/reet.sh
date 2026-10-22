package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestListRestoreJournalsAndRollbackLast(t *testing.T) {
	selection, report, home, claude := rollbackFixture(t)
	journalDir := t.TempDir()
	plan := planForSelection(t, selection, report)
	result, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: journalDir, Conflict: "replace"})
	if err != nil {
		t.Fatal(err)
	}
	writeConfig(t, filepath.Join(journalDir, "restore-invalid.json"), `{"schema_version":2}`)
	list, err := ListRestoreJournals(journalDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Journals) != 1 || list.Journals[0].State != "completed" || list.Journals[0].SnapshotID != "setup-plan" || list.Journals[0].Created != 1 || list.Journals[0].Replaced != 1 {
		t.Fatalf("wrong journal list: %+v", list.Journals)
	}
	if len(list.Skipped) != 1 || list.Skipped[0].Reason != "invalid_journal" {
		t.Fatalf("invalid journal not skipped: %+v", list.Skipped)
	}
	var text bytes.Buffer
	if err := renderRestoreJournalList(&text, list); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(text.Bytes(), []byte("completed")) || !bytes.Contains(text.Bytes(), []byte("created=1")) {
		t.Fatalf("bad journal text: %s", text.String())
	}
	updated, err := discover(fixtureWithHome(t, home))
	if err != nil {
		t.Fatal(err)
	}
	rolled, err := RollbackRestore(result.JournalPath, BuildSetupSnapshot(selection), testManifest("setup-plan"), updated, RestoreRollbackOptions{})
	if err != nil || len(rolled.Restored) != 1 || rolled.Restored[0] != claude {
		t.Fatalf("rollback failed: %+v %v", rolled, err)
	}
	list, err = ListRestoreJournals(journalDir)
	if err != nil || list.Journals[0].State != "rolled_back" {
		t.Fatalf("rolled_back state missing: %+v %v", list, err)
	}
	if _, err := latestCompletedJournal(journalDir, "setup-plan"); err == nil {
		t.Fatal("rolled-back journal was still considered completed")
	}
}

func TestSetupJournalsCLI(t *testing.T) {
	sourceHome := isolatedCLI(t)
	writeConfig(t, filepath.Join(sourceHome, ".claude.json"), `{"model":"JOURNAL-MODEL"}`)
	passFile := writePassphrase(t, t.TempDir(), testSetupPassphrase)
	dir := t.TempDir()
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
	var applied bytes.Buffer
	if err := Run([]string{"restore", "--apply", "--password-file", passFile, "--dir", dir, "--json", snapshot.ID}, &applied); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run([]string{"journals", "--dir", dir, "--json"}, &out); err != nil {
		t.Fatal(err)
	}
	var list RestoreJournalList
	if err := json.Unmarshal(out.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Journals) != 1 || list.Journals[0].State != "completed" || list.Journals[0].Created != 1 {
		t.Fatalf("wrong CLI journal list: %+v", list)
	}
	var text bytes.Buffer
	if err := Run([]string{"journals", "--dir", dir}, &text); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(text.Bytes(), []byte("completed")) {
		t.Fatalf("bad journals text: %s", text.String())
	}
	var rolled bytes.Buffer
	if err := Run([]string{"rollback", "--last", "--password-file", passFile, "--dir", dir, "--json", snapshot.ID}, &rolled); err != nil {
		t.Fatal(err)
	}
	var result RestoreRollbackResult
	if err := json.Unmarshal(rolled.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Removed) != 1 {
		t.Fatalf("last journal was not used: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(destination, ".claude.json")); !os.IsNotExist(err) {
		t.Fatal("rollback --last did not remove the created file")
	}
	if err := Run([]string{"rollback", "--last", "--password-file", passFile, "--dir", dir, snapshot.ID}, &bytes.Buffer{}); err == nil {
		t.Fatal("rolled-back journal was used again")
	}
}
