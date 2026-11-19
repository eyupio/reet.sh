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

// Removing one of several shelf rows that stand for one library skill takes
// only that row off the shelf: the skill, its copy and its other rows stay.
// The skill is deleted when its last row goes.
func TestRemoveShelfSkillsKeepsTheSkillWhileAnotherRowStandsForIt(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := importedItem(t, s, uid, "vercel-labs/skills/find-skills")
	if _, err := s.SetSkillContentFor(uid, item.id, contentFiles("find-skills", "Find."), PrivateSkillOrigin{Kind: "original", Reference: "https://github.com/vercel-labs/skills"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertLocalFor(uid, "find-skills", Patch{}); err != nil {
		t.Fatal(err)
	}
	var aliases int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM library_aliases WHERE user_id=? AND kind='shelf' AND item_id=?`, uid, item.id).Scan(&aliases); err != nil || aliases != 2 {
		t.Fatalf("both rows stand for the skill: %d %v", aliases, err)
	}
	removed, _, err := s.RemoveShelfSkillsFor(uid, []string{"local/find-skills"}, false, false, time.Now())
	if err != nil || removed != 1 {
		t.Fatalf("remove the duplicate: %d %v", removed, err)
	}
	r := skillStateNamed(t, s, uid, "find-skills")
	if r.deletedAt != 0 || r.revision == "" {
		t.Fatalf("removing a duplicate row deleted the skill: %+v", r)
	}
	if _, ok := skillRemovals(t, s, uid)["find-skills"]; ok {
		t.Fatal("removing a duplicate row put the name on the removal list")
	}
	if _, ok, _ := s.GetFor(uid, "local/find-skills"); ok {
		t.Fatal("the duplicate row is still on the shelf")
	}
	if _, ok, _ := s.GetFor(uid, "vercel-labs/skills/find-skills"); !ok {
		t.Fatal("the other row went too")
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM library_aliases WHERE user_id=? AND kind='shelf' AND reference='local/find-skills'`, uid).Scan(&aliases); err != nil || aliases != 0 {
		t.Fatalf("the removed row still stands for the skill: %d %v", aliases, err)
	}
	if removed, _, err = s.RemoveShelfSkillsFor(uid, []string{"vercel-labs/skills/find-skills"}, false, false, time.Now()); err != nil || removed != 1 {
		t.Fatalf("remove the last row: %d %v", removed, err)
	}
	if skillStateNamed(t, s, uid, "find-skills").deletedAt == 0 {
		t.Fatal("the last row went and the skill stayed")
	}
}

// The old Remove refuses a skill the person wrote, by its origin, and
// changes nothing; an imported skill that was edited in Write a skill (it
// has a hosted alias) is removable all the same.
func TestRemoveShelfSkillsRefusesOnlyWrittenSkills(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	imported := importedItem(t, s, uid, "vercel-labs/skills/find-skills")
	if _, err := s.SetSkillContentFor(uid, imported.id, contentFiles("find-skills", "Find."), PrivateSkillOrigin{Kind: "original", Reference: "https://github.com/vercel-labs/skills"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO library_aliases(user_id,kind,reference,item_id) VALUES(?,'hosted','find-skills',?)`, uid, imported.id); err != nil {
		t.Fatal(err)
	}
	written := importedItem(t, s, uid, "me/skills/mine")
	if _, err := s.SetSkillContentFor(uid, written.id, contentFiles("mine", "Mine."), PrivateSkillOrigin{Kind: "edit"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE library_items SET origin='written' WHERE id=?`, written.id); err != nil {
		t.Fatal(err)
	}
	removed, refused, err := s.RemoveShelfSkillsFor(uid, []string{"vercel-labs/skills/find-skills", "me/skills/mine"}, true, true, time.Now())
	if !errors.Is(err, ErrSkillWritten) || removed != 0 || len(refused) != 1 || refused[0] != "mine" {
		t.Fatalf("remove with a written skill: %d %v %v", removed, refused, err)
	}
	for _, id := range []string{"vercel-labs/skills/find-skills", "me/skills/mine"} {
		if _, ok, _ := s.GetFor(uid, id); !ok {
			t.Fatalf("a refused remove took %s off the shelf", id)
		}
	}
	if ignored, _ := s.IsIgnoredFor(uid, "vercel-labs/skills/find-skills"); ignored {
		t.Fatal("a refused remove ignored a skill")
	}
	if removed, _, err = s.RemoveShelfSkillsFor(uid, []string{"vercel-labs/skills/find-skills"}, false, false, time.Now()); err != nil || removed != 1 {
		t.Fatalf("remove of an imported skill with a hosted alias: %d %v", removed, err)
	}
	if skillStateNamed(t, s, uid, "find-skills").deletedAt == 0 || skillStateNamed(t, s, uid, "mine").deletedAt != 0 {
		t.Fatal("wrong skill deleted")
	}
}

// Keep takes the files from the copy the page lists first, whatever the
// order the machines reported in, when the copies were seen at the same
// time.
func TestKeepTakesTheCopyListedFirst(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	var agents []string
	for _, host := range []string{"zeta", "alpha", "beta"} {
		a, _, err := s.EnrollSyncAgentBuild(uid, host, "1.0", "")
		if err != nil {
			t.Fatal(err)
		}
		if err = s.SetAgentSkillsProtocolFor(uid, a.ID, 2); err != nil {
			t.Fatal(err)
		}
		agents = append(agents, a.ID)
		reportOf(t, s, uid, a.ID, time.Now(), unknownOn("cq", "d1"))
	}
	if _, err := s.db.Exec(`UPDATE machine_skills SET at=1000 WHERE user_id=?`, uid); err != nil {
		t.Fatal(err)
	}
	found, err := s.NewOnMachinesFor(uid)
	if err != nil || len(found) != 1 || len(found[0].Copies) != 3 {
		t.Fatalf("new: %+v %v", found, err)
	}
	first := found[0].Copies[0]
	if first.Host != "alpha" {
		t.Fatalf("listed first: %+v", found[0].Copies)
	}
	kept, err := s.KeepNewSkillsFor(uid, []string{"cq"}, SkillActive, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if kept[0].AgentID != first.AgentID || kept[0].Folder != first.Folder {
		t.Fatalf("kept from %s, the page lists %s first", kept[0].AgentID, first.AgentID)
	}
}

// The page labels a skill written exactly when Delete and Remove treat it as
// written: when its origin is written. A skill put into reet with no origin
// recorded and nothing referring to it reads as kept, as the upgrade read
// such skills.
func TestSkillsPageWrittenMatchesTheServer(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	base := ""
	result, err := s.SaveLibraryContentFor(uid, "", PrivateSkillWrite{RequestID: "upload-without-origin-1", ExpectedBase: &base, Name: "uploaded", Files: contentFiles("uploaded", "Up.")})
	if err != nil {
		t.Fatal(err)
	}
	item := skillStateNamed(t, s, uid, "uploaded")
	if item.origin != "" || item.bundle != result.Bundle.ID {
		t.Fatalf("fixture: %+v", item)
	}
	page, err := s.SkillsPageFor(uid, SkillActive, time.Now())
	if err != nil || len(page.Skills) != 1 {
		t.Fatalf("page: %+v %v", page.Skills, err)
	}
	refs, err := s.SkillItemsFor(uid, []string{item.id})
	if err != nil {
		t.Fatal(err)
	}
	row := page.Skills[0]
	if row.Yours != refs[0].Written || row.OriginKind != "kept" || row.Yours {
		t.Fatalf("page says %s (yours %t); the server's written is %t", row.OriginKind, row.Yours, refs[0].Written)
	}
}
