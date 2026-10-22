package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func skillFixture(t *testing.T) (environment, string) {
	t.Helper()
	e := fixture(t)
	const secret = "SYNTHETIC-SKILL-SECRET-DO-NOT-RETURN"
	root := filepath.Join(e.home, ".claude", "skills", "alpha")
	mkdir(t, filepath.Join(root, "scripts"))
	writeConfig(t, filepath.Join(root, "SKILL.md"), "---\nname: alpha\n---\nUse the secret "+secret+" carefully.\n")
	writeConfig(t, filepath.Join(root, "scripts", "run.sh"), "#!/bin/sh\necho alpha\n")
	if err := os.Chmod(filepath.Join(root, "scripts", "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	return e, secret
}

func TestSkillSelectionCapturesWholeFolderWithoutPreviewValues(t *testing.T) {
	e, secret := skillFixture(t)
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.Preview.Skills) != 1 {
		t.Fatalf("skill not selected: %+v", selection.Preview.Skills)
	}
	skill := selection.Preview.Skills[0]
	if skill.Name != "alpha" || skill.AgentID != "claude" || skill.Scope != "global" || skill.Files != 2 || skill.Bytes == 0 || skill.Digest == "" {
		t.Fatalf("wrong skill preview: %+v", skill)
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
		if strings.Contains(output, secret) || strings.Contains(output, "echo alpha") {
			t.Fatal("skill preview exposed file content")
		}
	}
	snapshot := BuildSetupSnapshot(selection)
	if len(snapshot.Skills) != 1 || len(snapshot.Skills[0].Files) != 2 {
		t.Fatalf("skill content missing from snapshot: %+v", snapshot.Skills)
	}
	result, err := WriteSetupSnapshot(selection, SetupWriteOptions{Dir: t.TempDir(), Password: testSetupPassphrase})
	if err != nil {
		t.Fatal(err)
	}
	if result.Skills != 1 || result.SkillFiles != 2 {
		t.Fatalf("skill write result wrong: %+v", result)
	}
	raw, err := os.ReadFile(result.Path)
	if err != nil || bytes.Contains(raw, []byte(secret)) {
		t.Fatalf("skill content leaked outside the sealed snapshot: %v", err)
	}
	opened, err := OpenSetupSnapshot(raw, testSetupPassphrase)
	if err != nil || len(opened.Skills) != 1 || len(opened.Skills[0].Files) != 2 {
		t.Fatalf("skill content missing after opening: %+v %v", opened.Skills, err)
	}
	content := false
	for _, file := range snapshot.Skills[0].Files {
		if bytes.Contains(file.Data, []byte(secret)) {
			content = true
		}
	}
	if !content {
		t.Fatal("expected skill content inside the payload")
	}
}

func TestSkillRestoreAndVerify(t *testing.T) {
	e, secret := skillFixture(t)
	source, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := fixture(t)
	report, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	plan := planForSelection(t, selection, report)
	var skillActions []RestoreAction
	for _, action := range plan.Actions {
		if action.Format == "skill" {
			skillActions = append(skillActions, action)
		}
	}
	if len(skillActions) != 2 || skillActions[0].State != "create" {
		t.Fatalf("skill restore actions missing: %+v", plan.Actions)
	}
	encoded, err := json.Marshal(plan)
	if err != nil || strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), "echo alpha") {
		t.Fatal("restore plan exposed skill content")
	}
	result, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(destination.home, ".claude", "skills", "alpha", "SKILL.md")
	script := filepath.Join(destination.home, ".claude", "skills", "alpha", "scripts", "run.sh")
	if len(result.Created) != 2 {
		t.Fatalf("wrong created paths: %+v", result.Created)
	}
	if data, err := os.ReadFile(manifest); err != nil || !strings.Contains(string(data), secret) {
		t.Fatalf("skill manifest not restored: %v", err)
	}
	info, err := os.Stat(script)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("skill script mode not restored: %+v %v", info, err)
	}
	updated, err := discover(fixtureWithHome(t, destination.home))
	if err != nil {
		t.Fatal(err)
	}
	verified := VerifySetupSnapshot(BuildSetupSnapshot(selection), testManifest("setup-skills"), updated)
	if verified.Summary.Verified != 1 || verified.Summary.Changed != 0 || verified.Summary.Missing != 0 {
		t.Fatalf("skill verification failed: %+v", verified)
	}
	for _, component := range verified.Components {
		if component.Kind == "skills" && component.Destination == "" {
			t.Fatalf("skill destination missing: %+v", component)
		}
	}
	if err := os.WriteFile(manifest, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed := VerifySetupSnapshot(BuildSetupSnapshot(selection), testManifest("setup-skills"), updated)
	if changed.Summary.Changed != 1 || changed.Summary.Verified != 0 {
		t.Fatalf("changed skill not reported: %+v", changed)
	}
	encoded, err = json.Marshal(changed)
	if err != nil || strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), "echo alpha") {
		t.Fatal("skill verification exposed content")
	}
}

func TestSkillProfileOverrideAndSymlinkOmission(t *testing.T) {
	e, _ := skillFixture(t)
	target := filepath.Join(t.TempDir(), "target")
	mkdir(t, target)
	writeConfig(t, filepath.Join(target, "SKILL.md"), "linked\n")
	if err := os.Symlink(target, filepath.Join(e.home, ".claude", "skills", "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.Preview.Skills) != 1 {
		t.Fatalf("unexpected skills: %+v", selection.Preview.Skills)
	}
	found := false
	for _, agent := range selection.Preview.Agents {
		for _, omitted := range agent.Omitted {
			if omitted.Kind == "skills" && omitted.Reason == "skill_symlink" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("symlinked skill not omitted: %+v", selection.Preview.Agents)
	}
	snapshot := BuildSetupSnapshot(selection)
	filtered := ApplyProfileOverrides(snapshot, &SetupProfileOverrides{ExcludeKinds: []string{"skills"}}, "", "")
	if len(filtered.Skills) != 0 || len(filtered.Preview.Skills) != 0 {
		t.Fatalf("skill override did not remove content: %+v", filtered.Preview.Skills)
	}
	if snapshot.Preview.Skills[0].Name != "alpha" {
		t.Fatalf("unexpected skill order: %+v", snapshot.Preview.Skills)
	}
}

func TestSkillRestoreKeepsSharedAndAgentSpecificRootsApart(t *testing.T) {
	e := fixture(t)
	for _, path := range []string{
		filepath.Join(e.home, ".agents", "skills", "shared"),
		filepath.Join(e.home, ".codex", "skills", "codex-only"),
	} {
		mkdir(t, path)
		writeConfig(t, filepath.Join(path, "SKILL.md"), "---\nname: "+filepath.Base(path)+"\n---\n")
	}
	source, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := fixture(t)
	report, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	plan := planForSelection(t, selection, report)
	for _, want := range []string{
		filepath.Join(destination.home, ".agents", "skills", "shared", "SKILL.md"),
		filepath.Join(destination.home, ".codex", "skills", "codex-only", "SKILL.md"),
	} {
		action := findRestoreAction(t, plan, want)
		if action.State != "create" || action.AgentID != "codex" {
			t.Fatalf("wrong skill root mapping for %q: %+v", want, action)
		}
	}
}

func TestSkillReplaceRollbackKeepsBackupMode(t *testing.T) {
	e, _ := skillFixture(t)
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
	manifestPath := filepath.Join(destination.home, ".claude", "skills", "alpha", "SKILL.md")
	mkdir(t, filepath.Dir(manifestPath))
	writeConfig(t, manifestPath, "original\n")
	if err := os.Chmod(manifestPath, 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	plan := BuildRestorePlan(snapshot, testManifest("setup-skill-rollback"), report)
	action := findRestoreAction(t, plan, manifestPath)
	if action.State != "conflict" {
		t.Fatalf("skill file not a conflict: %+v", action)
	}
	journalDir := t.TempDir()
	applied, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: journalDir, Conflict: "replace"})
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(manifestPath); err != nil || strings.Contains(string(data), "original") {
		t.Fatalf("skill file was not replaced: %q %v", data, err)
	}
	updated, err := discover(fixtureWithHome(t, destination.home))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RollbackRestore(applied.JournalPath, snapshot, testManifest("setup-skill-rollback"), updated, RestoreRollbackOptions{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil || string(data) != "original\n" {
		t.Fatalf("skill file was not rolled back: %q %v", data, err)
	}
	info, err := os.Stat(manifestPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("rollback lost the original mode: %+v %v", info, err)
	}
}

func TestSkillProvenanceFromLockFiles(t *testing.T) {
	e, _ := skillFixture(t)
	mkdir(t, filepath.Join(e.home, ".agents"))
	writeConfig(t, filepath.Join(e.home, ".agents", ".skill-lock.json"), `{"version":3,"skills":{"Display Name":{"source":"owner/repo","sourceType":"github","skillPath":"alpha/SKILL.md"}}}`)
	e.project = t.TempDir()
	mkdir(t, filepath.Join(e.project, ".claude", "skills", "project-skill"))
	writeConfig(t, filepath.Join(e.project, ".claude", "skills", "project-skill", "SKILL.md"), "---\nname: project-skill\n---\n")
	writeConfig(t, filepath.Join(e.project, "skills-lock.json"), `{"skills":[{"name":"project-skill","source":"publisher.example","sourceType":"well-known"}]}`)
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(r)
	if err != nil {
		t.Fatal(err)
	}
	origins := map[string]SnapshotSkill{}
	for _, skill := range selection.Preview.Skills {
		origins[skill.Name] = skill
	}
	if origins["alpha"].Origin != "owner/repo" || origins["alpha"].OriginType != "github" {
		t.Fatalf("global skill provenance missing: %+v", origins["alpha"])
	}
	if origins["project-skill"].Origin != "publisher.example" || origins["project-skill"].OriginType != "well-known" {
		t.Fatalf("project skill provenance missing: %+v", origins["project-skill"])
	}
	for _, output := range []string{string(mustJSON(t, selection.Preview)), string(mustJSON(t, BuildSetupSnapshot(selection)))} {
		if strings.Contains(output, "?token=") {
			t.Fatal("unsafe provenance was retained")
		}
	}
	unsafe := []string{"https://example.test/x?token=secret", "owner/repo?token=secret", "owner/repo#secret"}
	for _, source := range unsafe {
		if origin, _ := safeSkillOrigin(source, "github"); origin != "" {
			t.Fatalf("unsafe origin accepted: %q", source)
		}
	}
}

func TestSkillLockIsRestoredOnlyWhenMissing(t *testing.T) {
	e, _ := skillFixture(t)
	mkdir(t, filepath.Join(e.home, ".agents"))
	writeConfig(t, filepath.Join(e.home, ".agents", ".skill-lock.json"), `{"version":3,"skills":{"alpha":{"source":"owner/repo","sourceType":"github","skillPath":"skills/alpha/SKILL.md"}}}`)
	source, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := fixture(t)
	report, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	plan := planForSelection(t, selection, report)
	lockPath := filepath.Join(destination.home, ".agents", ".skill-lock.json")
	action := findRestoreAction(t, plan, lockPath)
	if action.State != "create" || action.Format != "json" {
		t.Fatalf("missing lock action: %+v", action)
	}
	applied, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		Version int `json:"version"`
		Skills  map[string]struct {
			Source     string `json:"source"`
			SourceType string `json:"sourceType"`
			SkillPath  string `json:"skillPath"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(data, &lock); err != nil {
		t.Fatal(err)
	}
	entry := lock.Skills["alpha"]
	if lock.Version != 3 || entry.Source != "owner/repo" || entry.SourceType != "github" || entry.SkillPath != "skills/alpha/SKILL.md" {
		t.Fatalf("wrong restored lock: %+v", lock)
	}
	updated, err := discover(fixtureWithHome(t, destination.home))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RollbackRestore(applied.JournalPath, BuildSetupSnapshot(selection), testManifest("setup-plan"), updated, RestoreRollbackOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatal("rollback kept the restored lock")
	}

	existing := fixture(t)
	mkdir(t, filepath.Join(existing.home, ".agents"))
	writeConfig(t, filepath.Join(existing.home, ".agents", ".skill-lock.json"), `{"version":3,"skills":{"other":{"source":"other/repo"}}}`)
	existingReport, err := discover(existing)
	if err != nil {
		t.Fatal(err)
	}
	existingPlan := planForSelection(t, selection, existingReport)
	lockPath = filepath.Join(existing.home, ".agents", ".skill-lock.json")
	action = findRestoreAction(t, existingPlan, lockPath)
	if action.State != "merge" {
		t.Fatalf("existing lock was not merged: %+v", action)
	}
	if data, err := os.ReadFile(lockPath); err != nil || !strings.Contains(string(data), "other/repo") {
		t.Fatal("planning changed the existing lock")
	}
	existingApplied, err := ApplyRestorePlan(existingPlan, RestoreApplyOptions{JournalDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "other/repo") || !strings.Contains(string(data), "owner/repo") {
		t.Fatalf("lock merge lost an entry: %s", data)
	}
	existingUpdated, err := discover(fixtureWithHome(t, existing.home))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RollbackRestore(existingApplied.JournalPath, BuildSetupSnapshot(selection), testManifest("setup-plan"), existingUpdated, RestoreRollbackOptions{}); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(lockPath)
	if err != nil || strings.Contains(string(data), "owner/repo") || !strings.Contains(string(data), "other/repo") {
		t.Fatalf("rollback did not restore the original lock: %s %v", data, err)
	}
}

func TestSkillLockMergeRefusesAChangedFile(t *testing.T) {
	e, _ := skillFixture(t)
	mkdir(t, filepath.Join(e.home, ".agents"))
	writeConfig(t, filepath.Join(e.home, ".agents", ".skill-lock.json"), `{"version":3,"skills":{"alpha":{"source":"owner/repo","sourceType":"github","skillPath":"skills/alpha/SKILL.md"},"other":{"source":"other/repo"}}}`)
	source, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := fixture(t)
	mkdir(t, filepath.Join(destination.home, ".agents"))
	lockPath := filepath.Join(destination.home, ".agents", ".skill-lock.json")
	writeConfig(t, lockPath, `{"version":3,"skills":{"other":{"source":"other/repo"}}}`)
	report, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	plan := planForSelection(t, selection, report)
	if action := findRestoreAction(t, plan, lockPath); action.State != "merge" {
		t.Fatalf("lock not planned as merge: %+v", action)
	}
	writeConfig(t, lockPath, `{"version":3,"skills":{"other":{"source":"changed/repo"}}}`)
	before, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir()}); err == nil {
		t.Fatal("changed lock file was overwritten")
	}
	after, err := os.ReadFile(lockPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("changed lock file was modified")
	}
}
