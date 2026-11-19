package store

import (
	"errors"
	"testing"
	"time"
)

func TestManagedBackupPath(t *testing.T) {
	for p, want := range map[string]string{
		"~/.agents/.reet-backups/2026-11-18/agents-demo":            "agents",
		"~/.claude/.reet-backups/2026-11-18/claude-demo-2":          "claude",
		"~/.cursor/.reet-backups/2026-02-28/cursor-demo":            "cursor",
		"~/.config/opencode/.reet-backups/2026-11-18/opencode-demo": "opencode",
		"~/.agents/.reet-sync-backups/removed-20261101/demo":        "agents",
		"~/.agents/skills/demo":                                     "",
		"~/.agents/skills/.reet-backups/2026-11-18/agents-demo":     "",
		"~/.agents/.reet-backups/2026-11-18":                        "",
		"~/.agents/.reet-backups/2026-11-18/agents-demo/SKILL.md":   "",
		"~/.agents/.reet-backups/2026-02-30/agents-demo":            "",
		"~/.agents/.reet-backups/2026-11-18/../../skills/demo":      "",
		"~/.agents/.reet-backups/2026-11-18/./agents-demo":          "",
		"~/.agents//.reet-backups/2026-11-18/agents-demo":           "",
		"~/.agents/.reet-sync-backups/removed-/demo":                "",
		"~/.agents/.reet-sync-backups/kept-1/demo":                  "",
		"~/.codex/.reet-backups/2026-11-18/codex-demo":              "",
		"~/.config/.reet-backups/2026-11-18/demo":                   "",
		"~/project/.agents/.reet-backups/2026-11-18/agents-demo":    "",
		"/home/me/.agents/.reet-backups/2026-11-18/agents-demo":     "",
		"~\\.agents\\.reet-backups\\2026-11-18\\agents-demo":        "",
		"~/.agents/.reet-backups/2026-11-18/agents-demo\x00":        "",
		"": "",
		"~/.agents/.reet-backups/2026-11-18/agents-demo\n~/.claude/skills": "",
	} {
		agent, ok := managedBackupPath(p)
		if agent != want || ok != (want != "") {
			t.Errorf("managedBackupPath(%q) = %q %t, want %q", p, agent, ok, want)
		}
	}
}

func unknownOn(name, digest string) SkillReportEntry {
	return SkillReportEntry{Folder: "~/.agents/skills", Name: name, State: "unknown", Digest: digest}
}

// Keep after a deleted skill's name was forgotten makes a new item, even
// while the deleted one's shelf row is still waiting to be purged.
func TestKeepNewSkillAfterADeletedOneOfItsName(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	old := localItem(t, s, uid, "elitebook", "cq")
	if err := s.DeleteSkillsFor(uid, []string{old.id}, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	agent := reportAgent(t, s, uid, "elitebook")
	reportOf(t, s, uid, agent, time.Now(), unknownOn("cq", "d1"))
	if _, err := s.KeepNewSkillsFor(uid, []string{"cq"}, SkillActive, "", time.Now()); !errors.Is(err, ErrSkillNotNew) {
		t.Fatalf("a name on the removal list is not new: %v", err)
	}
	if _, err := s.db.Exec(`DELETE FROM skill_removals WHERE user_id=?`, uid); err != nil {
		t.Fatal(err)
	}
	keep, err := s.KeepNewSkillsFor(uid, []string{"cq"}, SkillActive, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	kept := keep[0]
	if kept.Item == old.id || kept.AgentID != agent || kept.Folder != "~/.agents/skills" || kept.Name != "cq" {
		t.Fatalf("kept: %+v (old %s)", kept, old.id)
	}
	rows := skillStatesNamed(t, s, uid, "cq")
	live := 0
	for _, r := range rows {
		if r.deletedAt == 0 {
			live++
			if r.id != kept.Item || r.status != SkillActive || r.origin != "machine" {
				t.Fatalf("kept item: %+v", r)
			}
		}
	}
	if live != 1 {
		t.Fatalf("items named cq: %+v", rows)
	}
	if item, err := libraryAliasID(s.db, uid, "shelf", "local/cq"); err != nil || item != kept.Item {
		t.Fatalf("the shelf row stands for %s %v, want %s", item, err, kept.Item)
	}
}

// Archive of a new skill makes an Archived item whose name joins the
// removal list only once its files land.
func TestArchiveNewSkillWaitsForItsCopy(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	agent := reportAgent(t, s, uid, "elitebook")
	reportOf(t, s, uid, agent, time.Now(), unknownOn("paseo-help", "d1"))
	keep, err := s.KeepNewSkillsFor(uid, []string{"paseo-help"}, SkillArchived, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	kept := keep[0]
	row := skillStateNamed(t, s, uid, "paseo-help")
	if row.status != SkillArchived || row.prevStatus != "" || row.revision != "" {
		t.Fatalf("archived item: %+v", row)
	}
	if _, ok := skillRemovals(t, s, uid)["paseo-help"]; ok {
		t.Fatal("the name is on the removal list before reet has a copy")
	}
	if _, err = s.SetSkillContentFor(uid, kept.Item, contentFiles("paseo-help", "Help."), PrivateSkillOrigin{Kind: "capture", Reference: "machine:elitebook:~/.agents/skills"}); err != nil {
		t.Fatal(err)
	}
	if r, ok := skillRemovals(t, s, uid)["paseo-help"]; !ok || r.Reason != "archived" {
		t.Fatalf("removals after the copy: %+v", skillRemovals(t, s, uid))
	}
}

// The old shelf Remove deletes the library skill, refusing first when reet
// has no copy of it, and a shelf row put back inside the undo window brings
// it back.
func TestRemoveShelfSkillsDeletesTheLibrarySkill(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := importedItem(t, s, uid, "vercel-labs/skills/find-skills")
	removed, noCopy, err := s.RemoveShelfSkillsFor(uid, []string{"vercel-labs/skills/find-skills", "not/on/shelf"}, false, false, time.Now())
	if !errors.Is(err, ErrSkillNoCopy) || removed != 0 || len(noCopy) != 1 || noCopy[0] != "find-skills" {
		t.Fatalf("remove without a copy: %d %v %v", removed, noCopy, err)
	}
	if skillStateNamed(t, s, uid, "find-skills").deletedAt != 0 {
		t.Fatal("a refused remove deleted the skill")
	}
	if _, err = s.SetSkillContentFor(uid, item.id, contentFiles("find-skills", "Find."), PrivateSkillOrigin{Kind: "original", Reference: "https://github.com/vercel-labs/skills"}); err != nil {
		t.Fatal(err)
	}
	sk, _, err := s.GetFor(uid, "vercel-labs/skills/find-skills")
	if err != nil {
		t.Fatal(err)
	}
	if removed, _, err = s.RemoveShelfSkillsFor(uid, []string{"vercel-labs/skills/find-skills", "not/on/shelf"}, false, false, time.Now()); err != nil || removed != 1 {
		t.Fatalf("remove: %d %v", removed, err)
	}
	if skillStateNamed(t, s, uid, "find-skills").deletedAt == 0 || skillRemovals(t, s, uid)["find-skills"].Reason != "deleted" {
		t.Fatal("remove left the library skill")
	}
	if _, ok, _ := s.GetFor(uid, "vercel-labs/skills/find-skills"); ok {
		t.Fatal("remove left the shelf row")
	}
	if err = s.RestoreSnapshotFor(uid, Snapshot{Version: 2, Skills: []Skill{sk}}); err != nil {
		t.Fatal(err)
	}
	if r := skillStateNamed(t, s, uid, "find-skills"); r.deletedAt != 0 || r.revision == "" {
		t.Fatalf("undo: %+v", r)
	}
	if _, ok := skillRemovals(t, s, uid)["find-skills"]; ok {
		t.Fatal("undo left the name on the removal list")
	}
}
