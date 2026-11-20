package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/skillbundle"
)

func canonicalItems(t *testing.T, s *Store, uid int64) []LibraryItem {
	t.Helper()
	items, err := s.LibraryFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	return items
}
func canonicalHostedFixture(name, body string) HostedSkill {
	return HostedSkill{Slug: name, Content: "---\nname: " + name + "\ndescription: Library instructions\n---\n" + body + "\n", Files: map[string][]byte{"reference.bin": {0, 255, 1}}}
}

func TestCanonicalLibraryAuthoringStatusAndOrganisation(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if err := s.ClaimPublishingUsername(uid, "writer"); err != nil {
		t.Fatal(err)
	}
	skill := canonicalHostedFixture("demo", "first version")
	if _, err := s.SaveHostedSkill(uid, skill); err != nil {
		t.Fatal(err)
	}
	items := canonicalItems(t, s, uid)
	if len(items) != 1 || items[0].Status != SkillActive || items[0].Published || items[0].RevisionID == "" {
		t.Fatalf("private authoring did not create one Active complete item: %+v", items)
	}
	first := items[0]
	rf, err := ref.ParseAt("https://reet.example/writer/demo", "reet.example")
	if err != nil {
		t.Fatal(err)
	}
	yes, no := true, false
	notes := "keep my notes"
	if _, err = s.UpsertFor(uid, rf, Patch{Liked: &yes, Bookmarked: &yes, Notes: &notes}); err != nil {
		t.Fatal(err)
	}
	if err = s.AddSkillTagsFor(uid, rf.ID, []string{"work"}); err != nil {
		t.Fatal(err)
	}
	list, err := s.CreateList(uid, "Reviews", "Library examples", "personal")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AddListSkill(list.ID, rf.ID, "list note", uid); err != nil {
		t.Fatal(err)
	}
	item := canonicalItems(t, s, uid)[0]
	if item.ID != first.ID || len(canonicalItems(t, s, uid)) != 1 {
		t.Fatal("publishing provenance duplicated the authored identity")
	}
	if item.Metadata == nil || !item.Metadata.Liked || item.Metadata.Notes != notes || len(item.Metadata.Lists) != 1 || len(item.Metadata.Tags) == 0 {
		t.Fatalf("organisation not projected: %+v", item)
	}
	if err = s.SetLibraryStatusFor(uid, item.ID, SkillArchived, item.Version); err != nil {
		t.Fatal(err)
	}
	item, _ = s.LibraryItemFor(uid, item.ID)
	if _, err = s.PatchFor(uid, rf.ID, Patch{Liked: &no}); err != nil {
		t.Fatal(err)
	}
	item, _ = s.LibraryItemFor(uid, item.ID)
	if item.Status != SkillArchived || item.Metadata.Liked {
		t.Fatalf("organisation reset archived status: %+v", item)
	}
	if item.Metadata.Status != SkillArchived || item.Metadata.LibraryItemID != item.ID {
		t.Fatal("legacy metadata did not expose authoritative identity/status")
	}
	archived, _, err := s.ListFor(uid, Filter{Stage: "archived"})
	if err != nil || len(archived) != 1 {
		t.Fatal("archived status is not readable through legacy APIs", archived, err)
	}
	active, _, err := s.ListFor(uid, Filter{Stage: "active"})
	if err != nil || len(active) != 0 {
		t.Fatal("archived item was classified as Active", active, err)
	}
	if err = s.SetLibraryStatusFor(uid, item.ID, SkillActive, first.Version); !errors.Is(err, ErrLibraryConflict) {
		t.Fatal("stale lifecycle write accepted", err)
	}
	// A bulk status change moves the canonical status.
	if _, _, err = s.BulkUpdateFor(uid, []string{rf.ID}, BulkChange{Status: SkillActive, StatusAt: time.Now()}, false); err != nil {
		t.Fatal(err)
	}
	item, _ = s.LibraryItemFor(uid, item.ID)
	if item.Status != SkillActive {
		t.Fatal("explicit legacy stage did not update canonical status", item.Status)
	}
	if _, err = s.PatchManyFor(uid, []string{rf.ID}, Patch{Used: &yes}); err != nil {
		t.Fatal(err)
	}
	item, _ = s.LibraryItemFor(uid, item.ID)
	if item.Status != SkillActive {
		t.Fatal("bulk lifecycle diverged", item.Status)
	}
	skill.Content = strings.Replace(skill.Content, "first version", "second version", 1)
	skill.Published = true
	if _, err = s.SaveHostedSkill(uid, skill); err != nil {
		t.Fatal(err)
	}
	second := canonicalItems(t, s, uid)[0]
	if second.ID != first.ID || second.RevisionID == first.RevisionID || second.Status != SkillActive || !second.Published || second.Metadata.Notes != notes || len(second.Metadata.Lists) != 1 {
		t.Fatalf("edit/publication changed identity, status or metadata: %+v", second)
	}
	if _, err = s.hostedPrivateSource(uid, canonicalHostedFixture("demo", "first version")); !errors.Is(err, ErrPrivateSkillConflict) {
		t.Fatal("stale authored snapshot was allowed to replace the current head", err)
	}
	current, _ := s.LibraryItemFor(uid, first.ID)
	if current.RevisionID != second.RevisionID {
		t.Fatal("stale authored read moved the head backwards")
	}
	old, err := s.PrivateSkillRevisionFor(uid, first.BundleID, first.RevisionID)
	if err != nil || !bytes.Contains(old.Files[0].Data, []byte("first version")) {
		t.Fatal("previous immutable revision lost", err)
	}
	skill.Published = false
	if _, err = s.SaveHostedSkill(uid, skill); err != nil {
		t.Fatal(err)
	}
	if got := canonicalItems(t, s, uid); len(got) != 1 || got[0].ID != first.ID || got[0].Published || got[0].Status != SkillActive {
		t.Fatal("unpublishing changed lifecycle or identity", got)
	}
	newRef, err := ref.ParseAt("https://reet.example/writer/renamed", "reet.example")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RenameHostedSkill(uid, "demo", "renamed", rf, newRef); err != nil {
		t.Fatal(err)
	}
	got := canonicalItems(t, s, uid)
	if len(got) != 1 || got[0].ID != first.ID || got[0].Name != "renamed" || got[0].Metadata.Notes != notes || len(got[0].Metadata.Lists) != 1 {
		t.Fatal("rename lost stable identity or organisation", got)
	}
}

func TestCanonicalLibraryTypedContentAndReceiptCompatibility(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	base := ""
	cases := []PrivateSkillWrite{
		{Name: "review", ItemType: LibraryAgent, Format: "markdown", Application: "claude", Files: []skillbundle.ContentFile{{Path: "review.md", Data: []byte("---\nname: review\ndescription: Review code\n---\nInspect the diff.\n")}}},
		{Name: "codex-review", ItemType: LibraryAgent, Format: "toml", Application: "codex", Files: []skillbundle.ContentFile{{Path: "review.toml", Data: []byte("developer_instructions = 'Review the patch'\nmodel = 'example-model'\n")}}},
		{Name: "preferences", ItemType: LibrarySettings, Format: "json", Application: "claude", Files: []skillbundle.ContentFile{{Path: "settings.json", Data: []byte("{\"model\":\"example-model\",\"alwaysThinkingEnabled\":true}\n")}}},
		{Name: "codex-preferences", ItemType: LibrarySettings, Format: "toml", Application: "codex", Files: []skillbundle.ContentFile{{Path: "config.toml", Data: []byte("model = 'example-model'\napproval_policy = 'on-request'\n")}}},
	}
	for i, in := range cases {
		in.RequestID = fmt.Sprintf("typed_request_%08d", i)
		in.ExpectedBase = &base
		out, err := s.SavePrivateSkillRevisionFor(uid, "", in)
		if err != nil {
			t.Fatal(in.Name, err)
		}
		read, err := s.PrivateSkillRevisionFor(uid, out.Bundle.ID, out.Revision.ID)
		if err != nil || !reflect.DeepEqual(read.Files, in.Files) || out.Bundle.ItemType != in.ItemType {
			t.Fatalf("typed contents required a fake skill or changed bytes: %+v %v", read, err)
		}
		in.ItemType = LibrarySkill
		if _, err = s.SavePrivateSkillRevisionFor(uid, out.Bundle.ID, in); err == nil {
			t.Fatal("item type was mutable")
		}
	}
	for _, data := range []string{`{"env":{"API_KEY":"private"}}`, `{"model":"x","hooks":{"onStart":"run"}}`, `{"unlisted":"value"}`} {
		in := cases[2]
		in.RequestID = "reject_settings_0001"
		in.ExpectedBase = &base
		in.Files = []skillbundle.ContentFile{{Path: "settings.json", Data: []byte(data)}}
		if _, err := s.SavePrivateSkillRevisionFor(uid, "", in); !errors.Is(err, ErrPrivateSkillInvalid) {
			t.Fatal("excluded settings accepted", err)
		}
	}
	// Omitted and explicit default types produce the original skill fingerprint.
	in := machineImportFixture(t)
	write := PrivateSkillWrite{Name: in.Name, RequestID: "legacy_receipt_0001", ExpectedBase: &base, Files: in.Files}
	first, err := s.SavePrivateSkillRevisionFor(uid, "", write)
	if err != nil {
		t.Fatal(err)
	}
	write.ItemType = LibrarySkill
	again, err := s.SavePrivateSkillRevisionFor(uid, "", write)
	if err != nil || again.Revision.ID != first.Revision.ID {
		t.Fatal("additive type broke legacy receipt replay", err)
	}
}

func TestCanonicalLibraryMigrationPreservesLegacyStagesAndSelections(t *testing.T) {
	s, uid, a, env := observationFixture(t)
	member, err := s.CreateUser("migration-member@example.com", "Member", "member", "")
	if err != nil {
		t.Fatal(err)
	}
	memberNote := "private migration notes"
	if _, err = s.UpsertFor(member.ID, mustRef(t, "a/b/member-only"), Patch{Notes: &memberNote}); err != nil {
		t.Fatal(err)
	}
	yes := true
	for i, patch := range []Patch{{Liked: &yes}, {}, {}} {
		if _, err := s.UpsertFor(uid, mustRef(t, fmt.Sprintf("a/b/skill-%d", i)), patch); err != nil {
			t.Fatal(err)
		}
	}
	note := "migration notes"
	if _, err := s.PatchFor(uid, "a/b/skill-0", Patch{Notes: &note}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddSkillTagsFor(uid, "a/b/skill-0", []string{"migration-tag"}); err != nil {
		t.Fatal(err)
	}
	list, err := s.CreateList(uid, "Migration list", "Keep its membership", "personal")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AddListSkill(list.ID, "a/b/skill-0", "list notes", uid); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimPublishingUsername(uid, "writer"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveHostedSkill(uid, canonicalHostedFixture("private-draft", "Keep private")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SavePrivateSkillRevisionFor(uid, "", PrivateSkillWrite{Name: "demo", RequestID: "migration_legacy_0001", ExpectedBase: ptrString(""), Files: machineImportFixture(t).Files}); err != nil {
		t.Fatal(err)
	}
	// The stages and a saved selection, as a database from before
	// skills-engine-drop-v1 holds them.
	addLegacyShelfFlags(t, s, legacyShelfStage{uid, "a/b/skill-0", true, false}, legacyShelfStage{uid, "a/b/skill-1", false, true})
	if _, err := s.db.Exec(`CREATE TABLE sync_targets(user_id INTEGER NOT NULL,target_id TEXT NOT NULL,source_id TEXT NOT NULL,source_name TEXT NOT NULL,desired TEXT NOT NULL,revision TEXT NOT NULL,updated_at INTEGER NOT NULL,PRIMARY KEY(user_id,target_id))`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO sync_targets(user_id,source_id,source_name,target_id,desired,revision,updated_at) VALUES(?,'manual','Manual',?,'[]','legacy-revision',?)`, uid, env, now()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM library_migrations WHERE name='skills-engine-drop-v1'`); err != nil {
		t.Fatal(err)
	}
	before, _, err := s.ListFor(uid, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	for i := range before {
		before[i].LibraryItemID = ""
		before[i].Status = ""
	}
	rawBefore, _ := json.Marshal(before)
	// Remove only Stage 2's additive schema to exercise actual Open migration
	// against an existing account, authored/private bytes and saved selection.
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
	for _, name := range []string{"library_locations", "library_aliases", "library_projection_guard", "library_legacy_stages", "library_migrations", "library_items"} {
		if _, err = s.db.Exec(`DROP TABLE ` + name); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"item_type", "format", "application"} {
		if _, err = s.db.Exec(`ALTER TABLE private_skill_bundles DROP COLUMN ` + name); err != nil {
			t.Fatal(err)
		}
	}
	dir := s.Dir()
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	after, _, err := s.ListFor(uid, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	for i := range after {
		after[i].LibraryItemID = ""
		after[i].Status = ""
	}
	rawAfter, _ := json.Marshal(after)
	if !bytes.Equal(rawBefore, rawAfter) {
		t.Fatal("migration changed existing shelf metadata")
	}
	items := canonicalItems(t, s, uid)
	statuses := map[string]string{}
	for _, item := range items {
		statuses[item.Name] = item.Status
	}
	for name, want := range map[string]string{"skill-0": SkillActive, "skill-1": SkillActive, "skill-2": SkillArchived, "private-draft": SkillActive} {
		if statuses[name] != want {
			t.Fatal(name, statuses[name], want)
		}
	}
	// The saved selection went with the old engine, and so did the flags.
	if schemaObjectExists(t, s, "sync_targets") || shelfHasColumn(t, s, "used") {
		t.Fatal("the upgrade kept the old engine's selection or the shelf flags")
	}
	agents, err := s.SyncAgentsFor(uid)
	if err != nil || len(agents) != 1 || agents[0].ID != a.ID {
		t.Fatal("migration changed enrollment", err)
	}
	ids := map[string]string{}
	for _, item := range items {
		ids[item.Name] = item.ID
	}
	if err = s.migrateCanonicalLibrary(); err != nil {
		t.Fatal(err)
	}
	for _, item := range canonicalItems(t, s, uid) {
		if ids[item.Name] != item.ID {
			t.Fatal("migration was not idempotent")
		}
	}
	var retained int
	if err = s.db.QueryRow(`SELECT count(*) FROM library_legacy_stages WHERE user_id=?`, member.ID).Scan(&retained); err != nil || retained != 1 {
		t.Fatal("member migration evidence was not recorded", retained, err)
	}
	if err = s.DeleteUser(member.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow(`SELECT count(*) FROM library_legacy_stages WHERE user_id=?`, member.ID).Scan(&retained); err != nil || retained != 0 {
		t.Fatal("deleted account left private migration notes behind", retained, err)
	}
}

func ptrString(value string) *string { return &value }

func TestCanonicalLibraryKnownRootReusesProvenMachineCapture(t *testing.T) {
	s, uid, agent, _ := observationFixture(t)
	in := machineImportFixture(t)
	first, err := s.SaveMachineSkillImportFor(uid, agent.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	items := canonicalItems(t, s, uid)
	if len(items) != 1 {
		t.Fatal("completed machine capture not in library", items)
	}
	id := items[0].ID
	if items[0].BundleID != first.Bundle.ID {
		t.Fatal("the capture is not the item's content", items[0])
	}
	// Old capture APIs can start a fresh bundle for the same proven source.
	in.RequestID = "machine_recapture_0001"
	in.Files = append([]skillbundle.ContentFile{}, in.Files...)
	in.Files[0].Data = bytes.ReplaceAll(in.Files[0].Data, []byte("Use this skill."), []byte("More recent capture."))
	manifest, err := skillbundle.FromContents(in.Files)
	if err != nil {
		t.Fatal(err)
	}
	in.Digest = manifest.Digest
	newer, err := s.SaveMachineSkillImportFor(uid, agent.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveMachineSkillImportFor(uid, agent.ID, machineImportFixture(t)); err != nil {
		t.Fatal(err)
	}
	items = canonicalItems(t, s, uid)
	if len(items) != 1 || items[0].ID != id || items[0].RevisionID != newer.Revision.ID {
		t.Fatal("old receipt rewound or duplicated canonical capture", items)
	}
}

func TestCanonicalLibraryFailedAuthoredCaptureKeepsPreviousCompleteRevision(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if err := s.ClaimPublishingUsername(uid, "writer"); err != nil {
		t.Fatal(err)
	}
	skill := canonicalHostedFixture("demo", "good contents")
	if _, err := s.SaveHostedSkill(uid, skill); err != nil {
		t.Fatal(err)
	}
	before := canonicalItems(t, s, uid)[0]
	// The editor can retain a source file that the portable-content validator
	// rejects. It must not publish that partial capture as a complete new head.
	skill.Files["CON"] = []byte("unsupported portable file name")
	if _, err := s.SaveHostedSkill(uid, skill); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveHostedSkill(uid, canonicalHostedFixture("healthy", "other item still captures")); err != nil {
		t.Fatal(err)
	}
	items := canonicalItems(t, s, uid)
	if len(items) != 2 {
		t.Fatal(items)
	}
	for _, item := range items {
		if item.ID == before.ID {
			if item.RevisionID != before.RevisionID || item.Problem == "" || item.ContentState != "previous_revision" {
				t.Fatal("failed capture replaced or misreported the previous revision", item)
			}
		} else if item.ContentState != "complete" || item.RevisionID == "" {
			t.Fatal("unrelated item was blocked", item)
		}
	}
	if _, err := s.PrivateSkillRevisionFor(uid, before.BundleID, before.RevisionID); err != nil {
		t.Fatal("previous complete bytes lost", err)
	}
	delete(skill.Files, "CON")
	if _, err := s.SaveHostedSkill(uid, skill); err != nil {
		t.Fatal(err)
	}
	for _, item := range canonicalItems(t, s, uid) {
		if item.ID == before.ID && (item.Problem != "" || item.ContentState != "complete" || item.RevisionID != before.RevisionID) {
			t.Fatal("repair to the last good bytes stayed stuck or created a redundant revision", item)
		}
	}
}

// A linked library item is the single source for a name. Capturing a machine
// copy of the same name is refused rather than made a second library entry, so
// the Library shows the linked item once and sync keeps maintaining it.
func TestCanonicalLibraryShowsOneEntryWhenAMachineCopyDuplicatesALinkedSkill(t *testing.T) {
	s, uid, agent, _ := observationFixture(t)
	if err := s.ClaimPublishingUsername(uid, "writer"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveHostedSkill(uid, canonicalHostedFixture("demo", "hosted instructions")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveMachineSkillImportFor(uid, agent.ID, machineImportFixture(t)); !errors.Is(err, ErrSkillNameTaken) {
		t.Fatal("a machine copy of a library name was captured as a second item", err)
	}
	items, err := s.LibraryViewFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("a linked skill and its machine copy became %d library entries: %+v", len(items), items)
	}
	if !hasAlias(items[0], "hosted") {
		t.Fatalf("the linked item was not the one kept: %+v", items[0])
	}
}

// A skill that exists only on a machine can still be added to the library by
// hand. It then holds its name, so a written skill cannot take it.
func TestCanonicalLibraryKeepsAManuallyAddedMachineCopyAndItsName(t *testing.T) {
	s, uid, agent, _ := observationFixture(t)
	if _, err := s.SaveMachineSkillImportFor(uid, agent.ID, machineImportFixture(t)); err != nil {
		t.Fatal(err)
	}
	items, err := s.LibraryViewFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || linkedLibraryItem(items[0]) {
		t.Fatalf("a manual machine copy was not kept as its own source: %+v", items)
	}
	if err := s.ClaimPublishingUsername(uid, "writer"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveHostedSkill(uid, canonicalHostedFixture("demo", "hosted instructions")); !errors.Is(err, ErrSkillNameTaken) {
		t.Fatal("a written skill took the manual copy's name", err)
	}
	items, err = s.LibraryViewFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || hasAlias(items[0], "hosted") {
		t.Fatalf("the manual copy was replaced: %+v", items)
	}
}

func hasAlias(item LibraryItem, kind string) bool {
	for _, alias := range item.Aliases {
		if alias.Kind == kind {
			return true
		}
	}
	return false
}

// Selection and scheduler code reads LibraryFor, so it must keep the manual
// copy even when the Library view folds it under a linked same-name item. Such
// a copy is left from before names were unique: the low-level private write
// below makes one without the name check every library path now applies.
func TestCanonicalLibraryForKeepsEveryItemForSelection(t *testing.T) {
	s, uid, _, _ := observationFixture(t)
	if err := s.ClaimPublishingUsername(uid, "writer"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveHostedSkill(uid, canonicalHostedFixture("demo", "hosted instructions")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SavePrivateSkillRevisionFor(uid, "", privateWrite("legacy_duplicate_0001", "", "left from before")); err != nil {
		t.Fatal(err)
	}
	all, err := s.LibraryFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("LibraryFor dropped an item a selection could name: %+v", all)
	}
	view, err := s.LibraryViewFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(view) != 1 {
		t.Fatalf("the view did not fold the duplicate: %+v", view)
	}
}

// No two library items share a name: a linked copy of a written skill's name is
// refused, and a copy sighted on a machine joins that skill.
func TestCanonicalLibraryRefusesALinkedCopyAndJoinsAMachineSighting(t *testing.T) {
	s, uid, _, _ := observationFixture(t)
	if err := s.ClaimPublishingUsername(uid, "writer"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveHostedSkill(uid, canonicalHostedFixture("demo", "hosted instructions")); err != nil {
		t.Fatal(err)
	}
	r, err := ref.Parse("owner/repo/demo")
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	if _, err := s.UpsertFor(uid, r, Patch{Used: &yes}); !errors.Is(err, ErrSkillNameTaken) {
		t.Fatal("a linked copy of the written skill's name was accepted", err)
	}
	if _, ok, err := s.GetFor(uid, r.ID); err != nil || ok {
		t.Fatal("the refused link is on the shelf", err)
	}
	if _, err := s.UpsertLocalFor(uid, "demo", Patch{}); err != nil {
		t.Fatal(err)
	}
	view, err := s.LibraryViewFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(view) != 1 || !hasAlias(view[0], "hosted") || !hasAlias(view[0], "shelf") {
		t.Fatalf("the machine sighting did not join the written skill: %+v", view)
	}
}

func TestLinkedLibraryItemTreatsALegacySourceAsLinked(t *testing.T) {
	cases := []struct {
		name string
		item LibraryItem
		want bool
	}{
		{"hosted alias", LibraryItem{Aliases: []LibraryAlias{{Kind: "hosted", Reference: "demo"}}}, true},
		{"github source", LibraryItem{Metadata: &Skill{Source: "owner/repo", SourceType: "github"}}, true},
		{"source saved before its type", LibraryItem{Metadata: &Skill{Source: "owner/repo"}}, true},
		{"local source", LibraryItem{Metadata: &Skill{Source: "demo", SourceType: "local"}}, false},
		{"no source", LibraryItem{Metadata: &Skill{}}, false},
		{"no metadata", LibraryItem{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := linkedLibraryItem(c.item); got != c.want {
				t.Fatalf("linkedLibraryItem = %v, want %v", got, c.want)
			}
		})
	}
}

// The Skills page asks for one card per name. It must not need a second copy
// of a skill the account already has from a known source.
func TestListCanonicalFoldsAnUnlinkedCopyOfALinkedSkill(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	yes := true
	linked, err := ref.Parse("owner/repo/demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertFor(uid, linked, Patch{Used: &yes}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertLocalFor(uid, "demo", Patch{}); err != nil {
		t.Fatal(err)
	}
	all, counts, err := s.ListFor(uid, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || counts.All != 2 {
		t.Fatalf("fixture has %d skills, counts %+v", len(all), counts)
	}
	one, canonical, err := s.ListFor(uid, Filter{Canonical: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 1 || one[0].SourceType == "local" || canonical.All != 1 || canonical.Active != 1 {
		t.Fatalf("canonical list = %+v, counts %+v", one, canonical)
	}
}

func TestListCanonicalKeepsAManualCopyAndOneLinkedSourcePerName(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if _, err := s.UpsertLocalFor(uid, "solo", Patch{}); err != nil {
		t.Fatal(err)
	}
	one, _, err := s.ListFor(uid, Filter{Canonical: true})
	if err != nil || len(one) != 1 || one[0].SourceType != "local" {
		t.Fatalf("manual copy = %+v %v", one, err)
	}
	first, _ := ref.Parse("owner/repo/demo")
	second, _ := ref.Parse("other/repo/demo")
	if _, err := s.UpsertFor(uid, first, Patch{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertFor(uid, second, Patch{}); !errors.Is(err, ErrSkillNameTaken) {
		t.Fatal("a second linked source took the name demo", err)
	}
	both, counts, err := s.ListFor(uid, Filter{Canonical: true})
	if err != nil || len(both) != 2 || counts.All != 2 {
		t.Fatalf("want the manual copy and one linked source: %+v %+v %v", both, counts, err)
	}
}

// Identity is the exact name, as the sync library uses it. A case difference
// is not silently merged on the shelf.
func TestListCanonicalMatchesTheExactName(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	linked, _ := ref.Parse("owner/repo/demo")
	if _, err := s.UpsertFor(uid, linked, Patch{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertLocalFor(uid, "Demo", Patch{}); err != nil {
		t.Fatal(err)
	}
	all, _, err := s.ListFor(uid, Filter{Canonical: true})
	if err != nil || len(all) != 2 {
		t.Fatalf("case difference was folded: %+v %v", all, err)
	}
}

// Hiding the second card must not hide the machine that reported the copy: the
// view carries the observed folders onto the linked item.
func TestCanonicalLibraryViewMergesTheFoldedMachineObservation(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	linked, _ := ref.Parse("owner/repo/demo")
	if _, err := s.UpsertFor(uid, linked, Patch{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertLocalFor(uid, "demo", Patch{}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEnvironmentFor(uid, "local/demo", "laptop", "claude", "", "/home/me/.claude/skills", "/home/me/.claude/skills/demo"); err != nil {
		t.Fatal(err)
	}
	view, err := s.LibraryViewFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(view) != 1 || view[0].Metadata == nil {
		t.Fatalf("view = %+v", view)
	}
	if len(view[0].Metadata.Environments) != 1 || view[0].Metadata.Environments[0].Host != "laptop" || view[0].Metadata.EnvironmentCount != 1 {
		t.Fatalf("machine observation was not carried onto the linked item: %+v", view[0].Metadata)
	}
}
