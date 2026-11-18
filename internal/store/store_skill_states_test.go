package store

import (
	"errors"
	"strings"
	"testing"
	"time"
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

	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillActive, archivedAt.Add(time.Second)); err != nil {
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
