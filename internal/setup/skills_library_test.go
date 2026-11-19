package setup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Skills come from the person's reet library, which puts every Active skill
// on every machine, so a Setup no longer captures skill folders.
func TestSetupSnapshotHasNoSkillFolders(t *testing.T) {
	e := fixture(t)
	for _, root := range []string{filepath.Join(e.home, ".claude", "skills", "alpha"), filepath.Join(e.home, ".agents", "skills", "beta")} {
		mkdir(t, root)
		writeConfig(t, filepath.Join(root, "SKILL.md"), "---\nname: "+filepath.Base(root)+"\n---\nBody\n")
	}
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range r.Agents {
		for _, loc := range agent.Locations {
			if loc.Kind == "skills" || loc.Kind == "shared_skills" {
				t.Fatalf("a skill folder is still a Setup location: %s %+v", agent.ID, loc)
			}
		}
	}
	selection, err := BuildSnapshotSelection(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.Preview.Skills) != 0 {
		t.Fatalf("the preview lists skill folders: %+v", selection.Preview.Skills)
	}
	if snapshot := BuildSetupSnapshot(selection); len(snapshot.Skills) != 0 {
		t.Fatalf("the snapshot holds skill folders: %+v", snapshot.Skills)
	}
}

// A Setup saved before holds skill folders. Restoring it writes none of them,
// and says so once, however many it holds.
func TestSetupRestoreSkipsSkillFoldersOfAnOldSnapshot(t *testing.T) {
	source := fixture(t)
	writeConfig(t, filepath.Join(source.home, ".claude.json"), `{"model":"MODEL"}`)
	r, err := discover(source)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(r)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := BuildSetupSnapshot(selection)
	file := []SetupSkillFile{{Path: "SKILL.md", Mode: 0o644, Data: []byte("---\nname: alpha\n---\nBody\n")}}
	snapshot.Skills = []SetupSkill{
		{Name: "alpha", AgentID: "codex", Scope: "global", Root: filepath.Join(source.home, ".agents", "skills"), LocationKind: "shared_skills", LocationSource: ".agents/skills", Files: file, Digest: setupSkillDigest(file)},
		{Name: "beta", AgentID: "claude", Scope: "global", Root: filepath.Join(source.home, ".claude", "skills"), LocationKind: "skills", LocationSource: ".claude/skills", Files: file, Digest: setupSkillDigest(file)},
	}
	destination := fixture(t)
	mkdir(t, filepath.Join(destination.home, ".agents", "skills"))
	mkdir(t, filepath.Join(destination.home, ".claude", "skills"))
	report, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	plan := BuildRestorePlan(snapshot, testManifest("old-setup"), report)
	const note = "Skipped skill folders; your skills come from your library."
	if len(plan.Notes) != 1 || plan.Notes[0] != note {
		t.Fatalf("plan notes: %q", plan.Notes)
	}
	for _, action := range plan.Actions {
		if action.Format == "skill" || strings.Contains(action.Destination, filepath.Join(".agents", "skills")) || strings.Contains(action.Destination, filepath.Join(".claude", "skills")) {
			t.Fatalf("the plan restores a skill folder: %+v", action)
		}
	}
	for _, unsupported := range plan.Unsupported {
		if unsupported.Kind == "skills" && unsupported.Source != "" {
			t.Fatalf("skill folders are listed one by one: %+v", plan.Unsupported)
		}
	}
	var text bytes.Buffer
	if err = renderRestorePlan(&text, plan); err != nil {
		t.Fatal(err)
	}
	if strings.Count(text.String(), note) != 1 {
		t.Fatalf("the plan should say it once:\n%s", text.String())
	}
	result, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Notes) != 1 || result.Notes[0] != note {
		t.Fatalf("result notes: %q", result.Notes)
	}
	text.Reset()
	if err = renderRestoreApply(&text, result); err != nil {
		t.Fatal(err)
	}
	if strings.Count(text.String(), note) != 1 {
		t.Fatalf("the restore should say it once:\n%s", text.String())
	}
	for _, dir := range []string{filepath.Join(destination.home, ".agents", "skills"), filepath.Join(destination.home, ".claude", "skills")} {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 0 {
			t.Fatalf("%s was written: %v %v", dir, entries, err)
		}
	}
	updated, err := discover(fixtureWithHome(t, destination.home))
	if err != nil {
		t.Fatal(err)
	}
	verified := VerifySetupSnapshot(snapshot, testManifest("old-setup"), updated)
	for _, component := range verified.Components {
		if component.Kind == "skills" {
			t.Fatalf("verify checks skill folders: %+v", verified.Components)
		}
	}
	if verified.Summary.Missing != 0 {
		t.Fatalf("verify reports missing skills: %+v", verified.Summary)
	}
}
