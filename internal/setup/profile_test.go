package setup

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/clientconfig"
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
	filtered := ApplyProfileOverrides(BuildSetupSnapshot(selection), loaded.Revisions[0].Overrides, "", "")
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

type fakeProfileRemote struct {
	pushed      *SetupProfile
	server, tok string
	pulled      SetupProfile
}

func (f *fakeProfileRemote) Push(server, token string, profile SetupProfile) error {
	f.pushed, f.server, f.tok = &profile, server, token
	return nil
}

func (f *fakeProfileRemote) Pull(server, token, name string) (SetupProfile, error) {
	return f.pulled, nil
}

func TestProfilePushPullCLI(t *testing.T) {
	dir, snapshot := profileFixture(t)
	passFile := writePassphrase(t, t.TempDir(), testSetupPassphrase)
	if err := Run([]string{"profile", "save", "--dir", dir, "--password-file", passFile, "work", snapshot.ID}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	remote := &fakeProfileRemote{}
	if err := Run([]string{"profile", "push", "--dir", dir, "work"}, &bytes.Buffer{}, remote); err != nil {
		t.Fatal(err)
	}
	if remote.pushed == nil || remote.pushed.Name != "work" || remote.pushed.Revisions[0].SnapshotID != snapshot.ID {
		t.Fatalf("push payload wrong: %+v", remote.pushed)
	}
	remote.pulled = *remote.pushed
	remote.pulled.Name = "personal"
	for i := range remote.pulled.Revisions {
		remote.pulled.Revisions[i].Revision = i + 1
	}
	dst := t.TempDir()
	if err := Run([]string{"profile", "pull", "--dir", dst, "personal"}, &bytes.Buffer{}, remote); err != nil {
		t.Fatal(err)
	}
	profile, revision, err := LoadProfile("personal", dst, 0)
	if err != nil || profile.Name != "personal" || revision.SnapshotID != snapshot.ID {
		t.Fatalf("pulled profile wrong: %+v %+v %v", profile, revision, err)
	}
	if err := Run([]string{"profile", "push", "--dir", dir, "work"}, &bytes.Buffer{}); err == nil {
		t.Fatal("push without a remote accepted")
	}
}

func TestProfileMachineAndProjectOverrides(t *testing.T) {
	e := fixture(t)
	e.project = t.TempDir()
	writeConfig(t, filepath.Join(e.home, ".claude.json"), `{"model":"SCOPED"}`)
	writeConfig(t, filepath.Join(e.project, "CLAUDE.md"), "instructions")
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
	_, revision, err := SaveSetupProfileWithOptions("work", testSetupPassphrase, snapshot.ID, dir, time.Now(), SaveSetupProfileOptions{
		ExcludeKinds: []string{"instructions"}, Machines: []string{"laptop"}, Projects: []string{e.project},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(revision.Overrides.ExcludeKinds) != 0 || revision.Overrides.Machines["laptop"].ExcludeKinds[0] != "instructions" || revision.Overrides.Projects[e.project].ExcludeKinds[0] != "instructions" {
		t.Fatalf("scoped overrides wrong: %+v", revision.Overrides)
	}
	payload := BuildSetupSnapshot(selection)
	unscoped := ApplyProfileOverrides(payload, revision.Overrides, "", "")
	scoped := ApplyProfileOverrides(payload, revision.Overrides, "laptop", "")
	foundUnscoped := false
	for _, source := range unscoped.Sources {
		if source.Component == "instructions" {
			foundUnscoped = true
		}
	}
	if !foundUnscoped {
		t.Fatal("unscoped application lost instructions")
	}
	for _, source := range scoped.Sources {
		if source.Component == "instructions" {
			t.Fatal("machine override did not exclude instructions")
		}
	}
	if _, _, err := SaveSetupProfileWithOptions("bad", testSetupPassphrase, snapshot.ID, dir, time.Now(), SaveSetupProfileOptions{Machines: []string{"laptop"}}); err == nil {
		t.Fatal("empty scoped override accepted")
	}
}

func TestProfileSaveJustWorksWithoutPassphraseOrSnapshot(t *testing.T) {
	dir, snapshot := profileFixture(t)
	if err := Run([]string{"profile", "save", "--dir", dir, "work"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	profile, revision, err := LoadProfile("work", dir, 0)
	if err != nil || revision.SnapshotID != snapshot.ID || revision.Preview.SchemaVersion != 0 {
		t.Fatalf("header-only profile wrong: %+v %+v %v", profile, revision, err)
	}
	destination := fixture(t)
	passFile := writePassphrase(t, t.TempDir(), testSetupPassphrase)
	t.Setenv("HOME", destination.home)
	t.Setenv("USERPROFILE", destination.home)
	if err := Run([]string{"restore", "--profile", "work", "--apply", "--password-file", passFile, "--dir", dir}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination.home, ".claude.json")); err != nil {
		t.Fatal("header-only profile did not restore")
	}
	if err := Run([]string{"profile", "save", "--dir", dir, "bad", "SNAPSHOT"}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "omit SNAPSHOT") {
		t.Fatalf("placeholder snapshot error: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "FILE")
	if err := Run([]string{"profile", "save", "--dir", dir, "--password-file", missing, "other"}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "omit --password-file") {
		t.Fatalf("missing passphrase guidance: %v", err)
	}
}

func TestProfileSaveFlagsAfterArguments(t *testing.T) {
	dir, snapshot := profileFixture(t)
	passFile := writePassphrase(t, t.TempDir(), testSetupPassphrase)
	var out bytes.Buffer
	if err := Run([]string{"profile", "save", "--dir", dir, "work", snapshot.ID, "--password-file", passFile}, &out); err != nil {
		t.Fatal(err)
	}
	_, revision, err := LoadProfile("work", dir, 0)
	if err != nil || revision.Preview.SchemaVersion == 0 {
		t.Fatalf("trailing flags did not record preview: %+v %v", revision, err)
	}
	if err := Run([]string{"profile", "save", "--dir", dir, "bad", snapshot.ID, "--password-file", snapshot.Path}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "not the snapshot") {
		t.Fatalf("snapshot as passphrase file: %v", err)
	}
}

func TestProfilePushUsesDotEnv(t *testing.T) {
	dir, snapshot := profileFixture(t)
	passFile := writePassphrase(t, t.TempDir(), testSetupPassphrase)
	if err := Run([]string{"profile", "save", "--dir", dir, "--password-file", passFile, "work", snapshot.ID}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	envDir := t.TempDir()
	writeConfig(t, filepath.Join(envDir, ".env"), "REET_URL=https://reet.example\nREET_TOKEN='secret-token'\n")
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(envDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	remote := &fakeProfileRemote{}
	if err := Run([]string{"profile", "push", "--dir", dir, "work"}, &bytes.Buffer{}, remote); err != nil {
		t.Fatal(err)
	}
	if remote.server != "https://reet.example" || remote.tok != "secret-token" {
		t.Fatalf("env file not used: server=%q token=%q", remote.server, remote.tok)
	}
}

type tokenRemote struct {
	calls int
}

func (r *tokenRemote) Push(server, token string, profile SetupProfile) error {
	r.calls++
	if token == "good-token" {
		return nil
	}
	return ErrProfileUnauthorized
}

func (r *tokenRemote) Pull(server, token, name string) (SetupProfile, error) {
	return SetupProfile{}, nil
}

func TestProfilePushRetriesSavedToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
	if _, err := clientconfig.SaveToken("good-token"); err != nil {
		t.Fatal(err)
	}
	remote := &tokenRemote{}
	profile := SetupProfile{SchemaVersion: 1, Name: "work", Revisions: []SetupProfileRevision{{Revision: 1, SnapshotID: "setup-20261022T120000Z-abcdef01", SnapshotDigest: "digest"}}}
	if err := pushProfile(remote, "https://reet.example", "bad-token", profile, io.Discard); err != nil {
		t.Fatal(err)
	}
	if remote.calls != 2 {
		t.Fatalf("saved token not retried: %d calls", remote.calls)
	}
}

func TestProfilePullPreservesDivergentLocalHistory(t *testing.T) {
	dir := t.TempDir()
	original := SetupProfile{SchemaVersion: 1, Name: "work", Revisions: []SetupProfileRevision{{Revision: 1, SnapshotID: "a", SnapshotDigest: "digest"}}}
	if err := WriteSetupProfile(original, dir); err != nil {
		t.Fatal(err)
	}
	path, err := profilePath("work", dir)
	if err != nil {
		t.Fatal(err)
	}
	next := original
	next.Revisions = append(append([]SetupProfileRevision(nil), original.Revisions...), SetupProfileRevision{Revision: 2, SnapshotID: "b", SnapshotDigest: "digest-b"})
	if err := WriteSetupProfile(next, dir); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if err := WriteSetupProfile(original, dir); err == nil {
		t.Fatal("stale pull replaced local revisions")
	}
	fork := original
	fork.Revisions = []SetupProfileRevision{{Revision: 1, SnapshotID: "other", SnapshotDigest: "other"}}
	if err := WriteSetupProfile(fork, dir); err == nil {
		t.Fatal("divergent pull replaced local revisions")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("rejected pull changed local file")
	}
	unlock, err := lockProfiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if err := WriteSetupProfile(next, dir); err == nil {
		t.Fatal("concurrent profile writer accepted")
	}
}
