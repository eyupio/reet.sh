package store

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/skillbundle"
)

// dropLibraryTriggers stops shelf and hosted rows from making library items,
// so a test can add rows the way they were before skill-states-v1, same-name
// copies included.
func dropLibraryTriggers(t *testing.T, s *Store) {
	t.Helper()
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type='trigger' AND name LIKE 'library_%'`)
	if err != nil {
		t.Fatal(err)
	}
	var triggers []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		triggers = append(triggers, name)
	}
	rows.Close()
	for _, name := range triggers {
		if _, err = s.db.Exec(`DROP TRIGGER ` + name); err != nil {
			t.Fatal(err)
		}
	}
}

// legacyLibrary puts the library back as it was before skill-states-v1: the
// library tables are dropped with the migration records and rebuilt by
// identity-v1 from the shelf and hosted rows, with In use, To test and On the
// shelf taken from the shelf flags. No library triggers run until the upgrade.
func legacyLibrary(t *testing.T, s *Store) {
	t.Helper()
	dropLibraryTriggers(t, s)
	for _, table := range []string{"library_aliases", "library_items", "library_legacy_stages", "library_migrations"} {
		if _, err := s.db.Exec(`DROP TABLE ` + table); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.migrateCanonicalLibrary(); err != nil {
		t.Fatal(err)
	}
}

func upgradeSkillStates(t *testing.T, s *Store) {
	t.Helper()
	if err := s.migrateSkillStates(); err != nil {
		t.Fatal(err)
	}
}

type skillStateRow struct {
	id, name, status, prevStatus, upgradedFrom, origin, bundle, revision string
	statusAt, deletedAt                                                  int64
}

func skillStatesNamed(t *testing.T, s *Store, uid int64, name string) []skillStateRow {
	t.Helper()
	rows, err := s.db.Query(`SELECT id,name,status,prev_status,upgraded_from,origin,bundle_id,revision_id,status_at,deleted_at FROM library_items WHERE user_id=? AND lower(name)=lower(?) ORDER BY id`, uid, name)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []skillStateRow
	for rows.Next() {
		var r skillStateRow
		if err = rows.Scan(&r.id, &r.name, &r.status, &r.prevStatus, &r.upgradedFrom, &r.origin, &r.bundle, &r.revision, &r.statusAt, &r.deletedAt); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func skillStateNamed(t *testing.T, s *Store, uid int64, name string) skillStateRow {
	t.Helper()
	rows := skillStatesNamed(t, s, uid, name)
	if len(rows) != 1 {
		t.Fatalf("want one library item named %s, got %+v", name, rows)
	}
	return rows[0]
}

func skillRemovals(t *testing.T, s *Store, uid int64) map[string]SkillRemoval {
	t.Helper()
	list, err := s.SkillRemovalsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]SkillRemoval{}
	for _, r := range list {
		out[r.Name] = r
	}
	return out
}

func namedPrivateWrite(name, requestID string) PrivateSkillWrite {
	base := ""
	return PrivateSkillWrite{RequestID: requestID, ExpectedBase: &base, Name: name, Files: []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: " + name + "\ndescription: Kept in reet\n---\nUse it.\n")}}}
}

// savedWrittenSkill saves a written skill, which reet holds a copy of.
func savedWrittenSkill(t *testing.T, s *Store, uid int64, name string, published bool) skillStateRow {
	t.Helper()
	if username, err := s.PublishingUsername(uid); err != nil {
		t.Fatal(err)
	} else if username == "" {
		if err = s.ClaimPublishingUsername(uid, "writer"); err != nil {
			t.Fatal(err)
		}
	}
	skill := canonicalHostedFixture(name, "Written in reet")
	skill.Published = published
	if _, err := s.SaveHostedSkill(uid, skill); err != nil {
		t.Fatal(err)
	}
	row := skillStateNamed(t, s, uid, name)
	if row.revision == "" {
		t.Fatalf("written skill %s has no copy in reet", name)
	}
	return row
}

func TestSkillStatesUpgradeMapsStages(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	savedWrittenSkill(t, s, uid, "turned-off", false)
	for name, patch := range map[string]Patch{"in-use": {Used: bp(true)}, "to-test": {Bookmarked: bp(true)}, "parked": {}} {
		if _, err := s.UpsertFor(uid, mustRef(t, "a/b/"+name), patch); err != nil {
			t.Fatal(err)
		}
	}
	legacyLibrary(t, s)
	if _, err := s.db.Exec(`UPDATE library_items SET status='disabled' WHERE user_id=? AND name='turned-off'`, uid); err != nil {
		t.Fatal(err)
	}
	upgradeSkillStates(t, s)
	for name, want := range map[string][2]string{
		"in-use":     {SkillActive, ""},
		"to-test":    {SkillActive, ""},
		"parked":     {SkillArchived, "parked"},
		"turned-off": {SkillArchived, ""},
	} {
		got := skillStateNamed(t, s, uid, name)
		if got.status != want[0] || got.upgradedFrom != want[1] {
			t.Errorf("%s: got status %q from %q, want %q from %q", name, got.status, got.upgradedFrom, want[0], want[1])
		}
	}
	notice, err := s.SkillUpgradeNoticeFor(uid)
	if err != nil || notice == nil || *notice != (UpgradeNotice{Parked: 1, Removed: 0}) {
		t.Fatalf("notice %+v, %v", notice, err)
	}
	// reet holds a copy of the written skill, so it is taken off machines; the
	// parked one is only on machines, so it stays there until reet has a copy.
	removals := skillRemovals(t, s, uid)
	if len(removals) != 1 || removals["turned-off"].Reason != "archived" {
		t.Fatalf("removal list %+v", removals)
	}
	if err = s.DismissSkillUpgradeNoticeFor(uid); err != nil {
		t.Fatal(err)
	}
	if notice, err = s.SkillUpgradeNoticeFor(uid); err != nil || notice != nil {
		t.Fatalf("dismissed notice still shown: %+v, %v", notice, err)
	}
}

func TestSkillStatesUpgradeKeepsEveryItemAndAlias(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	savedWrittenSkill(t, s, uid, "written", true)
	for _, name := range []string{"one", "two"} {
		if _, err := s.UpsertFor(uid, mustRef(t, "a/b/"+name), Patch{}); err != nil {
			t.Fatal(err)
		}
	}
	legacyLibrary(t, s)
	count := func(table string) int {
		t.Helper()
		var n int
		if err := s.db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	items, aliases := count("library_items"), count("library_aliases")
	upgradeSkillStates(t, s)
	if count("library_items") != items || count("library_aliases") != aliases {
		t.Fatalf("upgrade lost rows: %d/%d items, %d/%d aliases", count("library_items"), items, count("library_aliases"), aliases)
	}
	var schema string
	if err := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='library_aliases'`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(schema, "REFERENCES library_items(user_id,id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED") {
		t.Fatalf("library_aliases lost its foreign key: %s", schema)
	}
	one := skillStateNamed(t, s, uid, "one")
	if _, err := s.db.Exec(`DELETE FROM library_items WHERE user_id=? AND id=?`, uid, one.id); err != nil {
		t.Fatal(err)
	}
	if count("library_aliases") != aliases-1 {
		t.Fatal("deleting an item no longer deletes its aliases")
	}
}

func TestSkillStatesUpgradeRestoresParkedSkills(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if _, err := s.UpsertFor(uid, mustRef(t, "a/b/parked"), Patch{}); err != nil {
		t.Fatal(err)
	}
	legacyLibrary(t, s)
	upgradeSkillStates(t, s)
	at := time.UnixMilli(1_800_000_000_000)
	if err := s.RestoreParkedSkillsFor(uid, at); err != nil {
		t.Fatal(err)
	}
	got := skillStateNamed(t, s, uid, "parked")
	if got.status != SkillActive || got.prevStatus != SkillArchived || got.statusAt != at.UnixMilli() {
		t.Fatalf("parked skill not restored: %+v", got)
	}
	if notice, err := s.SkillUpgradeNoticeFor(uid); err != nil || notice != nil {
		t.Fatalf("notice still shown after restoring: %+v, %v", notice, err)
	}
}

func TestSkillStatesUpgradeTurnsIgnoredIntoArchived(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if _, err := s.UpsertFor(uid, mustRef(t, "a/b/was-removed"), Patch{Used: bp(true)}); err != nil {
		t.Fatal(err)
	}
	legacyLibrary(t, s)
	// Removing kept the library item and put the skill on the ignore list.
	if err := s.IgnoreFor(uid, "a/b/was-removed", "was-removed", "removed"); err != nil {
		t.Fatal(err)
	}
	if err := s.IgnoreFor(uid, "local/never-kept", "never-kept", "removed"); err != nil {
		t.Fatal(err)
	}
	upgradeSkillStates(t, s)
	for _, name := range []string{"was-removed", "never-kept"} {
		got := skillStateNamed(t, s, uid, name)
		if got.status != SkillArchived || got.upgradedFrom != "removed" || got.revision != "" {
			t.Errorf("%s: %+v", name, got)
		}
	}
	var ignored int
	if err := s.db.QueryRow(`SELECT count(*) FROM ignored`).Scan(&ignored); err != nil || ignored != 0 {
		t.Fatalf("ignore list kept %d rows, %v", ignored, err)
	}
	notice, err := s.SkillUpgradeNoticeFor(uid)
	if err != nil || notice == nil || *notice != (UpgradeNotice{Parked: 0, Removed: 2}) {
		t.Fatalf("notice %+v, %v", notice, err)
	}
	// Archive remembers: the removed skill's identity is the archived item.
	if _, err = s.UpsertLocalFor(uid, "never-kept", Patch{}); err != nil {
		t.Fatal(err)
	}
	if got := skillStateNamed(t, s, uid, "never-kept"); got.status != SkillArchived {
		t.Fatalf("seeing the skill again made a new item: %+v", got)
	}
}

func TestSkillStatesUpgradeFoldsDuplicateNames(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	savedWrittenSkill(t, s, uid, "askme", false)
	// A copy captured from a machine before names were unique, with its own
	// bundle in reet.
	captured, err := s.SavePrivateSkillRevisionFor(uid, "", namedPrivateWrite("askme", "fold_captured_copy_0001"))
	if err != nil {
		t.Fatal(err)
	}
	dropLibraryTriggers(t, s)
	if _, err := s.UpsertLocalFor(uid, "askme", Patch{Used: bp(true), Liked: bp(true), Notes: sp("machine notes")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertFor(uid, mustRef(t, "a/b/seo-audit"), Patch{Bookmarked: bp(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertFor(uid, mustRef(t, "c/d/seo-audit"), Patch{Used: bp(true)}); err != nil {
		t.Fatal(err)
	}
	for id, tag := range map[string]string{"local/askme": "from-machine", "a/b/seo-audit": "from-testing"} {
		if err := s.AddSkillTagsFor(uid, id, []string{tag}); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.CreateList(uid, "Favourites", "", "personal")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AddListSkill(list.ID, "a/b/seo-audit", "", uid); err != nil {
		t.Fatal(err)
	}
	legacyLibrary(t, s)
	upgradeSkillStates(t, s)

	askme := skillStateNamed(t, s, uid, "askme")
	written, err := libraryAliasID(s.db, uid, "hosted", "askme")
	if err != nil || askme.id != written || askme.status != SkillActive || askme.origin != "written" {
		t.Fatalf("askme did not fold into the written skill: %+v, %v", askme, err)
	}
	item, err := s.LibraryItemFor(uid, askme.id)
	if err != nil {
		t.Fatal(err)
	}
	if item.Metadata == nil || !item.Metadata.Liked || item.Metadata.Notes != "machine notes" || !hasTag(item.Metadata.Tags, "from-machine") {
		t.Fatalf("the machine copy's like, notes and tags were lost: %+v", item.Metadata)
	}
	if !hasAlias(item, "hosted") || !hasAlias(item, "bundle") || !hasAlias(item, "shelf") || item.RevisionID == "" {
		t.Fatalf("the written skill lost an alias or its content: %+v", item)
	}
	// The captured copy is deleted from reet, content included.
	if _, err = s.PrivateSkillBundleFor(uid, captured.Bundle.ID); !errors.Is(err, ErrPrivateSkillMissing) {
		t.Fatal("the folded copy's bundle is still in reet", err)
	}
	var left int
	if err = s.db.QueryRow(`SELECT (SELECT count(*) FROM private_skill_revisions WHERE user_id=? AND bundle_id=?)+(SELECT count(*) FROM private_skill_blobs b WHERE b.user_id=? AND NOT EXISTS(SELECT 1 FROM private_skill_revision_files f WHERE f.user_id=b.user_id AND f.digest=b.digest))`, uid, captured.Bundle.ID, uid).Scan(&left); err != nil || left != 0 {
		t.Fatal("the folded copy's revisions or bytes are still in reet", left, err)
	}

	seo := skillStateNamed(t, s, uid, "seo-audit")
	inUse, ok, err := s.GetFor(uid, "c/d/seo-audit")
	if err != nil || !ok || inUse.LibraryItemID != seo.id {
		t.Fatalf("seo-audit did not fold into the In use copy: %+v %+v %v", seo, inUse, err)
	}
	if !hasTag(inUse.Tags, "from-testing") || len(inUse.Lists) != 1 {
		t.Fatalf("the other copy's tags and lists did not move: %+v", inUse)
	}
	if _, ok, err = s.GetFor(uid, "a/b/seo-audit"); err != nil || ok {
		t.Fatal("the folded copy is still in the library", err)
	}
}

func hasTag(tags []Tag, name string) bool {
	for _, tag := range tags {
		if tag.Name == name {
			return true
		}
	}
	return false
}

func TestSkillStatesUpgradeKeepsBinaryUpdateJobs(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	agent, _, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO sync_jobs(id,user_id,agent_id,kind,status,created_at,updated_at) VALUES('update-job',?,?,'update','queued',?,?)`, uid, agent.ID, now(), now()); err != nil {
		t.Fatal(err)
	}
	legacyLibrary(t, s)
	upgradeSkillStates(t, s)
	var status string
	if err = s.db.QueryRow(`SELECT status FROM sync_jobs WHERE id='update-job'`).Scan(&status); err != nil || status != "queued" {
		t.Fatalf("update job %q, %v", status, err)
	}
}

func TestSkillStatesArchiveRestoreDelete(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "seo-audit", false)
	archivedAt := time.UnixMilli(1_800_000_000_000)
	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, archivedAt); err != nil {
		t.Fatal(err)
	}
	got := skillStateNamed(t, s, uid, "seo-audit")
	if got.status != SkillArchived || got.prevStatus != SkillActive || got.statusAt != archivedAt.UnixMilli() {
		t.Fatalf("archive: %+v", got)
	}
	if r := skillRemovals(t, s, uid); len(r) != 1 || r["seo-audit"] != (SkillRemoval{Name: "seo-audit", Reason: "archived", CreatedAt: archivedAt.UnixMilli()}) {
		t.Fatalf("archive removal list: %+v", r)
	}

	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillActive, archivedAt.Add(SkillGrace)); err != nil {
		t.Fatal(err)
	}
	if got = skillStateNamed(t, s, uid, "seo-audit"); got.status != SkillActive || got.prevStatus != SkillArchived {
		t.Fatalf("restore: %+v", got)
	}
	if r := skillRemovals(t, s, uid); len(r) != 0 {
		t.Fatalf("restore left %+v on the removal list", r)
	}

	deletedAt := archivedAt.Add(time.Minute)
	if err := s.DeleteSkillsFor(uid, []string{item.id}, false, deletedAt); err != nil {
		t.Fatal(err)
	}
	if got = skillStateNamed(t, s, uid, "seo-audit"); got.deletedAt != deletedAt.UnixMilli() {
		t.Fatalf("delete: %+v", got)
	}
	if r := skillRemovals(t, s, uid); r["seo-audit"].Reason != "deleted" {
		t.Fatalf("delete removal list: %+v", r)
	}

	if err := s.UndeleteSkillsFor(uid, []string{item.id}); err != nil {
		t.Fatal(err)
	}
	if got = skillStateNamed(t, s, uid, "seo-audit"); got.deletedAt != 0 || got.status != SkillActive {
		t.Fatalf("undelete: %+v", got)
	}
	if r := skillRemovals(t, s, uid); len(r) != 0 {
		t.Fatalf("undelete left %+v on the removal list", r)
	}
	if err := s.SetSkillStatusFor(uid, []string{item.id}, "on_shelf", archivedAt); !errors.Is(err, ErrPrivateSkillInvalid) {
		t.Fatal("an old status was accepted", err)
	}
}

func TestSkillStatesArchiveWithoutCopyLeavesMachinesAlone(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if _, err := s.UpsertLocalFor(uid, "only-on-a-machine", Patch{}); err != nil {
		t.Fatal(err)
	}
	item := skillStateNamed(t, s, uid, "only-on-a-machine")
	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := skillStateNamed(t, s, uid, "only-on-a-machine"); got.status != SkillArchived {
		t.Fatalf("not archived: %+v", got)
	}
	if r := skillRemovals(t, s, uid); len(r) != 0 {
		t.Fatalf("the only copy was put on the removal list: %+v", r)
	}
}

func TestSkillStatesDeleteWithoutCopyNeedsConfirmation(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if _, err := s.UpsertLocalFor(uid, "only-on-a-machine", Patch{}); err != nil {
		t.Fatal(err)
	}
	item := skillStateNamed(t, s, uid, "only-on-a-machine")
	if err := s.DeleteSkillsFor(uid, []string{item.id}, false, time.Now()); !errors.Is(err, ErrSkillNoCopy) {
		t.Fatal("deleted the only copy without asking", err)
	}
	if got := skillStateNamed(t, s, uid, "only-on-a-machine"); got.deletedAt != 0 {
		t.Fatalf("refused delete still deleted: %+v", got)
	}
	if err := s.DeleteSkillsFor(uid, []string{item.id}, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if r := skillRemovals(t, s, uid); r["only-on-a-machine"].Reason != "deleted" {
		t.Fatalf("confirmed delete: %+v", r)
	}
}

func TestSkillStatesNamesAreUnique(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if _, err := s.UpsertFor(uid, mustRef(t, "a/b/seo-audit"), Patch{}); err != nil {
		t.Fatal(err)
	}
	first := skillStateNamed(t, s, uid, "seo-audit")
	// A machine copy by the same name is that skill, not a second one.
	if copy, err := s.UpsertLocalFor(uid, "SEO-Audit", Patch{}); err != nil || copy.LibraryItemID != first.id {
		t.Fatalf("a copy named SEO-Audit made a second item: %+v, %v", copy, err)
	}
	// A pasted link to another source of the name is refused.
	if _, err := s.UpsertFor(uid, mustRef(t, "c/d/seo-audit"), Patch{}); !errors.Is(err, ErrSkillNameTaken) {
		t.Fatal("a second linked seo-audit was accepted", err)
	}
	// The source already on the shelf can still be added again.
	if _, err := s.UpsertFor(uid, mustRef(t, "a/b/seo-audit"), Patch{Liked: bp(true)}); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimPublishingUsername(uid, "writer"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveHostedSkill(uid, canonicalHostedFixture("seo-audit", "Written in reet")); !errors.Is(err, ErrSkillNameTaken) {
		t.Fatal("a written skill took a name already in the library", err)
	}
	if len(skillStatesNamed(t, s, uid, "seo-audit")) != 1 {
		t.Fatal("a refused name still made an item")
	}
	// A deleted name is free again, and then its undo is refused.
	if err := s.DeleteSkillsFor(uid, []string{first.id}, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertLocalFor(uid, "seo-audit", Patch{}); err != nil {
		t.Fatal(err)
	}
	if err := s.UndeleteSkillsFor(uid, []string{first.id}); !errors.Is(err, ErrSkillNameTaken) {
		t.Fatal("undo made a second item with the same name", err)
	}
}

func TestSkillStatesDeletedWrittenSkillPurges(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "seo-audit", true)
	published := func() bool {
		t.Helper()
		hosted, err := s.HostedSkillsFor(uid)
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range hosted {
			if h.Slug == "seo-audit" && h.Published {
				return true
			}
		}
		return false
	}
	if !published() {
		t.Fatal("written skill not published")
	}
	deletedAt := time.UnixMilli(1_800_000_000_000)
	if err := s.DeleteSkillsFor(uid, []string{item.id}, false, deletedAt); err != nil {
		t.Fatal(err)
	}
	if published() {
		t.Fatal("a deleted skill is still published")
	}
	if _, err := s.PublishedHostedSkill("writer", "seo-audit"); !errors.Is(err, ErrNotFound) {
		t.Fatal("a deleted skill's public page is still served", err)
	}
	if err := s.UndeleteSkillsFor(uid, []string{item.id}); err != nil {
		t.Fatal(err)
	}
	if !published() {
		t.Fatal("undo did not bring the public page back")
	}
	if err := s.DeleteSkillsFor(uid, []string{item.id}, false, deletedAt); err != nil {
		t.Fatal(err)
	}
	if err := s.PurgeDeletedSkillsFor(uid, deletedAt.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.HostedSkillExists(uid, "seo-audit"); err != nil || !ok {
		t.Fatal("purged inside the undo window", err)
	}
	if err := s.PurgeDeletedSkillsFor(uid, deletedAt.Add(61*time.Second)); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.HostedSkillExists(uid, "seo-audit"); err != nil || ok {
		t.Fatal("hosted skill survived the purge", err)
	}
	if _, err := s.PrivateSkillBundleFor(uid, item.bundle); !errors.Is(err, ErrPrivateSkillMissing) {
		t.Fatal("bundle survived the purge", err)
	}
	var revisions int
	if err := s.db.QueryRow(`SELECT count(*) FROM private_skill_revisions WHERE user_id=? AND id=?`, uid, item.revision).Scan(&revisions); err != nil || revisions != 0 {
		t.Fatal("revisions survived the purge", revisions, err)
	}
	if err := s.UndeleteSkillsFor(uid, []string{item.id}); err == nil {
		t.Fatal("undo after the purge succeeded")
	}
}

func TestSkillStatesLinkToOwnWrittenSkillJoinsIt(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	written := savedWrittenSkill(t, s, uid, "my-notes", true)
	rf, err := ref.ParseAt("https://reet.example/writer/my-notes", "reet.example")
	if err != nil {
		t.Fatal(err)
	}
	sk, err := s.UpsertFor(uid, rf, Patch{})
	if err != nil || sk.LibraryItemID != written.id {
		t.Fatalf("the link to the account's own written skill was not that skill: %+v, %v", sk, err)
	}
}

func TestSkillStatesLocationImportRefusesATakenName(t *testing.T) {
	s, uid, agent, _ := observationFixture(t)
	if _, err := s.UpsertFor(uid, mustRef(t, "a/b/demo"), Patch{}); err != nil {
		t.Fatal(err)
	}
	location := LibraryLocation{ID: strings.Repeat("c", 32), Application: "shared", Path: "/fixture/skills"}
	if _, err := s.RegisterLibraryLocationFor(uid, agent.ID, location); err != nil {
		t.Fatal(err)
	}
	base := ""
	in := LibraryLocationImport{LocationID: location.ID, Name: "demo", RequestID: "taken_location_0001", ExpectedBase: &base, Files: machineImportFixture(t).Files}
	if _, err := s.ImportLibraryLocationFor(uid, agent.ID, in); !errors.Is(err, ErrSkillNameTaken) {
		t.Fatal("a location copy of a library name became a second item", err)
	}
	if rows := skillStatesNamed(t, s, uid, "demo"); len(rows) != 1 || rows[0].revision != "" {
		t.Fatalf("the refused import changed the library: %+v", rows)
	}
	if bundles, err := s.PrivateSkillBundlesFor(uid); err != nil || len(bundles) != 0 {
		t.Fatal("the refused import kept its bundle", bundles, err)
	}
}

func TestSkillStatesMachineImportRefusesATakenName(t *testing.T) {
	s, uid, agent, _ := observationFixture(t)
	if _, err := s.UpsertFor(uid, mustRef(t, "a/b/demo"), Patch{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveMachineSkillImportFor(uid, agent.ID, machineImportFixture(t)); !errors.Is(err, ErrSkillNameTaken) {
		t.Fatal("a machine capture of a library name became a second item", err)
	}
	if rows := skillStatesNamed(t, s, uid, "demo"); len(rows) != 1 || rows[0].revision != "" {
		t.Fatalf("the refused capture changed the library: %+v", rows)
	}
	if bundles, err := s.PrivateSkillBundlesFor(uid); err != nil || len(bundles) != 0 {
		t.Fatal("the refused capture kept its bundle", bundles, err)
	}
	// A capture of a free name is still made an item.
	other := machineImportFixture(t)
	other.RequestID = "machine_import_free_0001"
	other.Name = "free-name"
	other.Files = namedPrivateWrite("free-name", "unused_request_0001").Files
	m, err := skillbundle.FromContents(other.Files)
	if err != nil {
		t.Fatal(err)
	}
	other.Digest = m.Digest
	if _, err = s.SaveMachineSkillImportFor(uid, agent.ID, other); err != nil {
		t.Fatal(err)
	}
	skillStateNamed(t, s, uid, "free-name")
}

func TestSkillStatesLibraryUploadRefusesATakenName(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if _, err := s.UpsertFor(uid, mustRef(t, "a/b/seo-audit"), Patch{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveLibraryContentFor(uid, "", namedPrivateWrite("seo-audit", "taken_upload_00001")); !errors.Is(err, ErrSkillNameTaken) {
		t.Fatal("an upload of a library name became a second item", err)
	}
	if bundles, err := s.PrivateSkillBundlesFor(uid); err != nil || len(bundles) != 0 {
		t.Fatal("the refused upload kept its bundle", bundles, err)
	}
	// A new name is accepted, and so is a new revision of that item.
	first, err := s.SaveLibraryContentFor(uid, "", namedPrivateWrite("free-name", "free_upload_000001"))
	if err != nil {
		t.Fatal(err)
	}
	next := namedPrivateWrite("free-name", "free_upload_000002")
	next.ExpectedBase = &first.Revision.ID
	next.Files[0].Data = append(next.Files[0].Data, []byte("Second revision.\n")...)
	if _, err = s.SaveLibraryContentFor(uid, first.Bundle.ID, next); err != nil {
		t.Fatal(err)
	}
	if rows := skillStatesNamed(t, s, uid, "free-name"); len(rows) != 1 {
		t.Fatalf("want one item named free-name: %+v", rows)
	}
}

// The Skills page still sets stages until it shows Active and Archived: In use
// or To test makes a skill Active and Parked makes it Archived, with the
// removal list following.
func TestSkillStatesShelfStageMovesTheStatus(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	savedWrittenSkill(t, s, uid, "seo-audit", true)
	rf, err := ref.ParseAt("https://reet.example/writer/seo-audit", "reet.example")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpsertFor(uid, rf, Patch{}); err != nil {
		t.Fatal(err)
	}
	yes, no := true, false
	expect := func(step, status string, removal bool) {
		t.Helper()
		if got := skillStateNamed(t, s, uid, "seo-audit"); got.status != status {
			t.Fatalf("%s: status %q, want %q", step, got.status, status)
		}
		if _, listed := skillRemovals(t, s, uid)["seo-audit"]; listed != removal {
			t.Fatalf("%s: on the removal list = %v, want %v", step, listed, removal)
		}
	}
	if _, err = s.PatchFor(uid, rf.ID, Patch{Used: &no, Bookmarked: &no}); err != nil {
		t.Fatal(err)
	}
	expect("park", SkillArchived, true)
	if _, err = s.PatchFor(uid, rf.ID, Patch{Used: &yes}); err != nil {
		t.Fatal(err)
	}
	expect("mark in use", SkillActive, false)
	if _, err = s.PatchManyFor(uid, []string{rf.ID}, Patch{Used: &no, Bookmarked: &no}); err != nil {
		t.Fatal(err)
	}
	expect("bulk park", SkillArchived, true)
	if _, _, err = s.BulkUpdateFor(uid, []string{rf.ID}, BulkChange{Bookmarked: &yes}, false); err != nil {
		t.Fatal(err)
	}
	expect("bulk to test", SkillActive, false)
	if _, _, err = s.BulkUpdateFor(uid, []string{rf.ID}, BulkChange{Used: &no, Bookmarked: &no}, false); err != nil {
		t.Fatal(err)
	}
	expect("bulk update park", SkillArchived, true)
	// A sighting on a machine joins the Archived skill and leaves it Archived.
	if _, err = s.UpsertLocalFor(uid, "seo-audit", Patch{Used: &yes}); err != nil {
		t.Fatal(err)
	}
	expect("sighting", SkillArchived, true)
	// Marking the skill already on the shelf as used makes it Active.
	if _, err = s.UpsertFor(uid, rf, Patch{Used: &yes}); err != nil {
		t.Fatal(err)
	}
	expect("quick mark in use", SkillActive, false)
}
