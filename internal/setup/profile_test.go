package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func profileFixture(t *testing.T) (string, SetupWriteResult) {
	t.Helper()
	e := fixture(t)
	const secret = "SYNTHETIC-PROFILE-SECRET"
	writeConfig(t, filepath.Join(e.home, ".claude.json"), `{"model":"PROFILE-MODEL","env":{"TOKEN":"`+secret+`"}}`)
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(r)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	result, err := WriteSetupSnapshot(selection, SetupWriteOptions{Dir: dir, Password: testSetupPassphrase})
	if err != nil {
		t.Fatal(err)
	}
	return dir, result
}

func TestSetupProfileSaveListShowAndResolve(t *testing.T) {
	dir, snapshot := profileFixture(t)
	first, revision1, err := SaveSetupProfile("work", testSetupPassphrase, snapshot.ID, dir, time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC))
	if err != nil || revision1.Revision != 1 || revision1.SnapshotID != snapshot.ID || revision1.Preview.SchemaVersion != snapshotPreviewSchemaVersion {
		t.Fatalf("first save: %+v %+v %v", first, revision1, err)
	}
	_, revision2, err := SaveSetupProfile("work", testSetupPassphrase, snapshot.ID, dir, time.Date(2026, 10, 22, 13, 0, 0, 0, time.UTC))
	if err != nil || revision2.Revision != 2 {
		t.Fatalf("second save: %+v %v", revision2, err)
	}
	profile, selected, err := LoadProfile("work", dir, 0)
	if err != nil || selected.Revision != 2 || len(profile.Revisions) != 2 {
		t.Fatalf("latest revision: %+v %+v %v", profile, selected, err)
	}
	if _, old, err := LoadProfile("work", dir, 1); err != nil || old.Revision != 1 {
		t.Fatalf("old revision: %+v %v", old, err)
	}
	if _, _, err := LoadProfile("work", dir, 99); err == nil {
		t.Fatal("missing revision accepted")
	}
	if got, err := ResolveProfileSnapshot("work", dir); err != nil || got != snapshot.ID {
		t.Fatalf("resolve: %q %v", got, err)
	}
	list, err := ListSetupProfiles(dir)
	if err != nil || len(list.Profiles) != 1 || list.Profiles[0].Revision != 2 || list.Profiles[0].Revisions != 2 {
		t.Fatalf("profile list: %+v %v", list, err)
	}
	path, err := profilePath("work", dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), "SYNTHETIC-PROFILE-SECRET") {
		t.Fatal("profile exposed a credential value")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("profile permissions: %+v %v", info, err)
	}
	for _, name := range []string{"", "../x", "a/b", strings.Repeat("x", 65)} {
		if validProfileName(name) {
			t.Fatalf("invalid profile name accepted: %q", name)
		}
	}
	var text bytes.Buffer
	if err := renderProfileList(&text, list); err != nil || !strings.Contains(text.String(), "work") {
		t.Fatalf("profile text: %v %s", err, text.String())
	}
}

func TestSetupProfileRestoreAndVerifyCLI(t *testing.T) {
	dir, snapshot := profileFixture(t)
	_ = snapshot
	passFile := writePassphrase(t, t.TempDir(), testSetupPassphrase)
	var saved bytes.Buffer
	if err := Run([]string{"profile", "save", "--dir", dir, "--password-file", passFile, "--json", "work", snapshot.ID}, &saved); err != nil {
		t.Fatal(err)
	}
	var saveResult setupProfileSaveResult
	if err := json.Unmarshal(saved.Bytes(), &saveResult); err != nil || saveResult.Revision.Revision != 1 {
		t.Fatalf("save CLI: %+v %v", saveResult, err)
	}
	destination := t.TempDir()
	t.Setenv("HOME", destination)
	t.Setenv("USERPROFILE", destination)
	var applied bytes.Buffer
	if err := Run([]string{"restore", "--profile", "work", "--apply", "--password-file", passFile, "--dir", dir, "--json"}, &applied); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, ".claude.json")); err != nil {
		t.Fatal(err)
	}
	var verified bytes.Buffer
	if err := Run([]string{"verify", "--profile", "work", "--password-file", passFile, "--dir", dir, "--json"}, &verified); err != nil {
		t.Fatal(err)
	}
	var result SetupVerifyResult
	if err := json.Unmarshal(verified.Bytes(), &result); err != nil || result.Summary.Verified == 0 {
		t.Fatalf("verify CLI: %+v %v", result, err)
	}
	var listed bytes.Buffer
	if err := Run([]string{"profile", "list", "--dir", dir, "--json"}, &listed); err != nil {
		t.Fatal(err)
	}
	var list SetupProfileList
	if err := json.Unmarshal(listed.Bytes(), &list); err != nil || len(list.Profiles) != 1 {
		t.Fatalf("list CLI: %+v %v", list, err)
	}
	var shown bytes.Buffer
	if err := Run([]string{"profile", "show", "--dir", dir, "--json", "work"}, &shown); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(shown.String(), "SYNTHETIC-PROFILE-SECRET") {
		t.Fatal("profile show exposed credentials")
	}
	var inspected bytes.Buffer
	if err := Run([]string{"inspect", "--profile", "work", "--password-file", passFile, "--dir", dir, "--json"}, &inspected); err != nil {
		t.Fatal(err)
	}
	var inspection SetupSnapshotInspection
	if err := json.Unmarshal(inspected.Bytes(), &inspection); err != nil || !inspection.Opened || inspection.Header.ID != snapshot.ID {
		t.Fatalf("inspect by profile: %+v %v", inspection, err)
	}
	var rolled bytes.Buffer
	if err := Run([]string{"rollback", "--profile", "work", "--last", "--password-file", passFile, "--dir", dir, "--json"}, &rolled); err != nil {
		t.Fatal(err)
	}
	var rollback RestoreRollbackResult
	if err := json.Unmarshal(rolled.Bytes(), &rollback); err != nil || len(rollback.Removed) != 1 {
		t.Fatalf("rollback by profile: %+v %v", rollback, err)
	}
	if err := Run([]string{"inspect", "--profile", "work", snapshot.ID, "--password-file", passFile, "--dir", dir}, &bytes.Buffer{}); err == nil {
		t.Fatal("profile and snapshot together were accepted")
	}
}

func TestProfileOverridesExcludeKindsAndSources(t *testing.T) {
	e := fixture(t)
	e.project = t.TempDir()
	writeConfig(t, filepath.Join(e.home, ".claude.json"), `{"model":"OVERRIDE-MODEL"}`)
	instruction := filepath.Join(e.project, "CLAUDE.md")
	writeConfig(t, instruction, "excluded instructions")
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(r)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	snapshot, err := WriteSetupSnapshot(selection, SetupWriteOptions{Dir: dir, Password: testSetupPassphrase})
	if err != nil {
		t.Fatal(err)
	}
	_, revision, err := SaveSetupProfileWithOptions("work", testSetupPassphrase, snapshot.ID, dir, time.Now(), SaveSetupProfileOptions{ExcludeKinds: []string{"instructions"}})
	if err != nil || revision.Overrides == nil || len(revision.Overrides.ExcludeKinds) != 1 {
		t.Fatalf("overrides not saved: %+v %v", revision, err)
	}
	if _, _, err := SaveSetupProfileWithOptions("bad", testSetupPassphrase, snapshot.ID, dir, time.Now(), SaveSetupProfileOptions{ExcludeKinds: []string{"history"}}); err == nil {
		t.Fatal("unknown excluded kind accepted")
	}
	loaded, _, err := LoadProfile("work", dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	filtered := ApplyProfileOverrides(BuildSetupSnapshot(selection), loaded.Revisions[0].Overrides)
	for _, source := range filtered.Sources {
		if source.Component == "instructions" {
			t.Fatalf("instruction source survived override: %+v", source)
		}
	}
	if filtered.Preview.Agents[0].Omitted == nil {
		t.Fatal("override omission missing")
	}
	destination := fixture(t)
	report, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	plan := BuildRestorePlan(filtered, testManifest(snapshot.ID), report)
	for _, action := range plan.Actions {
		if action.Scope == "project" {
			t.Fatalf("excluded project source planned: %+v", action)
		}
	}
	var text bytes.Buffer
	if err := renderProfile(&text, loaded, loaded.Revisions[0]); err != nil || !strings.Contains(text.String(), "excluded kinds") {
		t.Fatalf("profile override text: %v %s", err, text.String())
	}
}

func TestProfileOverrideCLI(t *testing.T) {
	e := fixture(t)
	e.project = t.TempDir()
	writeConfig(t, filepath.Join(e.home, ".claude.json"), `{"model":"OVERRIDE-CLI"}`)
	writeConfig(t, filepath.Join(e.project, "CLAUDE.md"), "excluded")
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(r)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	snapshot, err := WriteSetupSnapshot(selection, SetupWriteOptions{Dir: dir, Password: testSetupPassphrase})
	if err != nil {
		t.Fatal(err)
	}
	passFile := writePassphrase(t, t.TempDir(), testSetupPassphrase)
	if err := Run([]string{"profile", "save", "--exclude-kind", "instructions", "--dir", dir, "--password-file", passFile, "work", snapshot.ID}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	t.Setenv("HOME", destination)
	t.Setenv("USERPROFILE", destination)
	if err := Run([]string{"restore", "--profile", "work", "--apply", "--password-file", passFile, "--dir", dir}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, ".claude.json")); err != nil {
		t.Fatal("non-excluded setting missing")
	}
	if _, err := os.Stat(filepath.Join(destination, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatal("excluded instruction restored without a project")
	}
}

func TestProfileRevisionSelectionCLI(t *testing.T) {
	e := fixture(t)
	dir := t.TempDir()
	passFile := writePassphrase(t, t.TempDir(), testSetupPassphrase)
	var ids []string
	for _, model := range []string{"OLD-MODEL", "NEW-MODEL"} {
		writeConfig(t, filepath.Join(e.home, ".claude.json"), `{"model":"`+model+`"}`)
		r, err := discover(e)
		if err != nil {
			t.Fatal(err)
		}
		selection, err := BuildSnapshotSelection(r)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := WriteSetupSnapshot(selection, SetupWriteOptions{Dir: dir, Password: testSetupPassphrase})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, snapshot.ID)
	}
	for _, id := range ids {
		if err := Run([]string{"profile", "save", "--dir", dir, "--password-file", passFile, "work", id}, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		revision string
		want     string
	}{{"1", "OLD-MODEL"}, {"2", "NEW-MODEL"}} {
		destination := t.TempDir()
		t.Setenv("HOME", destination)
		t.Setenv("USERPROFILE", destination)
		if err := Run([]string{"restore", "--profile", "work", "--revision", tc.revision, "--apply", "--password-file", passFile, "--dir", dir}, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(destination, ".claude.json"))
		if err != nil || !strings.Contains(string(data), tc.want) {
			t.Fatalf("revision %s used wrong snapshot: %q %v", tc.revision, data, err)
		}
	}
	if err := Run([]string{"restore", "--revision", "1", "--apply", "--password-file", passFile, "--dir", dir, ids[0]}, &bytes.Buffer{}); err == nil {
		t.Fatal("revision without profile accepted")
	}
}

func TestProfileRenameAndRemoveCLI(t *testing.T) {
	dir, snapshot := profileFixture(t)
	passFile := writePassphrase(t, t.TempDir(), testSetupPassphrase)
	if err := Run([]string{"profile", "save", "--dir", dir, "--password-file", passFile, "work", snapshot.ID}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"profile", "rename", "--dir", dir, "work", "personal"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadProfile("work", dir, 0); err == nil {
		t.Fatal("old profile still loads after rename")
	}
	renamed, revision, err := LoadProfile("personal", dir, 0)
	if err != nil || renamed.Name != "personal" || revision.SnapshotID != snapshot.ID {
		t.Fatalf("renamed profile wrong: %+v %+v %v", renamed, revision, err)
	}
	if _, err := os.Stat(snapshot.Path); err != nil {
		t.Fatal("rename removed a snapshot")
	}
	if err := Run([]string{"profile", "remove", "--dir", dir, "personal"}, &bytes.Buffer{}); err == nil {
		t.Fatal("remove without --yes accepted")
	}
	if err := Run([]string{"profile", "remove", "--yes", "--dir", dir, "personal"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadProfile("personal", dir, 0); err == nil {
		t.Fatal("removed profile still loads")
	}
	if _, err := os.Stat(snapshot.Path); err != nil {
		t.Fatal("remove deleted a snapshot")
	}
	if err := Run([]string{"profile", "rename", "--dir", dir, "missing", "x"}, &bytes.Buffer{}); err == nil {
		t.Fatal("missing profile renamed")
	}
}
