package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

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
	if len(items) != 1 || items[0].Status != LibraryToTest || items[0].Published || items[0].RevisionID == "" {
		t.Fatalf("private authoring did not create one inactive complete item: %+v", items)
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
	if item.Metadata == nil || !item.Metadata.Liked || !item.Metadata.Bookmarked || item.Metadata.Notes != notes || len(item.Metadata.Lists) != 1 || len(item.Metadata.Tags) == 0 {
		t.Fatalf("organisation not projected: %+v", item)
	}
	if err = s.SetLibraryStatusFor(uid, item.ID, LibraryDisabled, item.Version); err != nil {
		t.Fatal(err)
	}
	item, _ = s.LibraryItemFor(uid, item.ID)
	if _, err = s.PatchFor(uid, rf.ID, Patch{Liked: &no}); err != nil {
		t.Fatal(err)
	}
	item, _ = s.LibraryItemFor(uid, item.ID)
	if item.Status != LibraryDisabled || item.Metadata.Used || !item.Metadata.Bookmarked {
		t.Fatalf("organisation reset disabled status: %+v", item)
	}
	if item.Metadata.Status != LibraryDisabled || item.Metadata.LibraryItemID != item.ID {
		t.Fatal("legacy metadata did not expose authoritative identity/status")
	}
	disabled, _, err := s.ListFor(uid, Filter{Stage: "disabled"})
	if err != nil || len(disabled) != 1 {
		t.Fatal("disabled stage is not readable through legacy APIs", disabled, err)
	}
	trying, _, err := s.ListFor(uid, Filter{Stage: "try"})
	if err != nil || len(trying) != 0 {
		t.Fatal("disabled item was classified as To test", trying, err)
	}
	if err = s.SetLibraryStatusFor(uid, item.ID, LibraryInUse, first.Version); !errors.Is(err, ErrLibraryConflict) {
		t.Fatal("stale lifecycle write accepted", err)
	}
	if _, _, err = s.BulkUpdateFor(uid, []string{rf.ID}, BulkChange{Used: &no, Bookmarked: &yes}, false); err != nil {
		t.Fatal(err)
	}
	item, _ = s.LibraryItemFor(uid, item.ID)
	if item.Status != LibraryToTest {
		t.Fatal("explicit legacy stage did not update canonical status", item.Status)
	}
	if _, err = s.PatchManyFor(uid, []string{rf.ID}, Patch{Used: &yes}); err != nil {
		t.Fatal(err)
	}
	item, _ = s.LibraryItemFor(uid, item.ID)
	if item.Status != LibraryInUse {
		t.Fatal("bulk lifecycle diverged", item.Status)
	}
	// Organisation alone never creates a machine selection.
	targets, err := s.SyncTargetsFor(uid)
	if err != nil || len(targets) != 0 {
		t.Fatal("metadata created deployment intent", targets, err)
	}
	skill.Content = strings.Replace(skill.Content, "first version", "second version", 1)
	skill.Published = true
	if _, err = s.SaveHostedSkill(uid, skill); err != nil {
		t.Fatal(err)
	}
	second := canonicalItems(t, s, uid)[0]
	if second.ID != first.ID || second.RevisionID == first.RevisionID || second.Status != LibraryInUse || !second.Published || second.Metadata.Notes != notes || len(second.Metadata.Lists) != 1 {
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
	if got := canonicalItems(t, s, uid); len(got) != 1 || got[0].ID != first.ID || got[0].Published || got[0].Status != LibraryInUse {
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

func TestCanonicalLibraryLocationIdentityIsolationAndPartialCapture(t *testing.T) {
	s, uid, a, _ := observationFixture(t)
	location := LibraryLocation{ID: strings.Repeat("a", 32), Application: "shared", Path: "/fixture/skills"}
	if _, err := s.RegisterLibraryLocationFor(uid, a.ID, location); err != nil {
		t.Fatal(err)
	}
	fixture := machineImportFixture(t)
	base := ""
	in := LibraryLocationImport{LocationID: location.ID, Name: "demo", RequestID: "location_create_0001", ExpectedBase: &base, Files: fixture.Files}
	first, err := s.ImportLibraryLocationFor(uid, a.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	location2 := LibraryLocation{ID: strings.Repeat("b", 32), Application: "codex", Project: "/project", Path: "/project/custom/skills"}
	if _, err = s.RegisterLibraryLocationFor(uid, a.ID, location2); err != nil {
		t.Fatal(err)
	}
	secondIn := in
	secondIn.LocationID = location2.ID
	secondIn.RequestID = "location_create_0002"
	second, err := s.ImportLibraryLocationFor(uid, a.ID, secondIn)
	if err != nil {
		t.Fatal(err)
	}
	items := canonicalItems(t, s, uid)
	if len(items) != 2 || items[0].ID == items[1].ID || first.Bundle.ID == second.Bundle.ID {
		t.Fatal("same-name locations were silently merged", items)
	}
	bad := in
	bad.Name = "broken"
	bad.RequestID = "location_broken_0001"
	bad.Files = []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("invalid metadata")}}
	if _, err = s.ImportLibraryLocationFor(uid, a.ID, bad); !errors.Is(err, ErrPrivateSkillInvalid) {
		t.Fatal("malformed capture accepted", err)
	}
	bad = in
	bad.RequestID = "location_secret_0001"
	bad.Files = append(append([]skillbundle.ContentFile{}, in.Files...), skillbundle.ContentFile{Path: ".env", Data: []byte("EXAMPLE_SECRET=private")})
	if _, err = s.ImportLibraryLocationFor(uid, a.ID, bad); !errors.Is(err, ErrPrivateSkillInvalid) {
		t.Fatal("blocked capture accepted", err)
	}
	changed := in
	changed.RequestID = "location_update_0001"
	changed.ExpectedBase = &first.Revision.ID
	changed.Files = append([]skillbundle.ContentFile{}, in.Files...)
	changed.Files[0].Data = bytes.ReplaceAll(changed.Files[0].Data, []byte("Use this skill."), []byte("Updated instructions."))
	updated, err := s.ImportLibraryLocationFor(uid, a.ID, changed)
	if err != nil {
		t.Fatal(err)
	}
	late, err := s.ImportLibraryLocationFor(uid, a.ID, in)
	if err != nil || late.Revision.ID != first.Revision.ID {
		t.Fatal("lost acknowledgement replay was not idempotent", err)
	}
	entries, err := s.RegisterLibraryLocationFor(uid, a.ID, location)
	if err != nil || len(entries) != 1 || entries[0].RevisionID != updated.Revision.ID {
		t.Fatal("late receipt moved the canonical head backwards", entries, err)
	}
	if len(canonicalItems(t, s, uid)) != 2 {
		t.Fatal("failed capture changed healthy items")
	}
	if _, err = s.ImportLibraryLocationFor(uid+999, a.ID, in); err == nil {
		t.Fatal("other account imported this location")
	}
	other, _, err := s.EnrollSyncAgent(uid, "other", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ImportLibraryLocationFor(uid, other.ID, in); err == nil {
		t.Fatal("other enrollment imported this location")
	}
	if err = s.ControlSyncAgent(uid, a.ID, "revoke"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ImportLibraryLocationFor(uid, a.ID, in); err == nil {
		t.Fatal("revoked connector replayed an import")
	}
	// The source may now be unavailable; both exact captured revisions remain.
	read, err := s.PrivateSkillRevisionFor(uid, first.Bundle.ID, first.Revision.ID)
	if err != nil || !reflect.DeepEqual(read.Files, in.Files) {
		t.Fatal("offline source made private bytes unavailable", err)
	}
}

func TestCanonicalLibraryMigrationPreservesLegacyStagesAndSelections(t *testing.T) {
	s, uid, a, env := observationFixture(t)
	yes, no := true, false
	for i, patch := range []Patch{{Used: &yes, Liked: &yes}, {Bookmarked: &yes}, {Used: &no, Bookmarked: &no}} {
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
	if _, err := s.db.Exec(`INSERT INTO sync_targets(user_id,source_id,source_name,target_id,desired,revision,updated_at) VALUES(?,'manual','Manual',?,'[]','legacy-revision',?)`, uid, env, now()); err != nil {
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
	for name, want := range map[string]string{"skill-0": LibraryInUse, "skill-1": LibraryToTest, "skill-2": LibraryOnShelf, "private-draft": LibraryToTest} {
		if statuses[name] != want {
			t.Fatal(name, statuses[name], want)
		}
	}
	var desired, revision string
	if err = s.db.QueryRow(`SELECT desired,revision FROM sync_targets WHERE user_id=? AND target_id=?`, uid, env).Scan(&desired, &revision); err != nil || desired != "[]" || revision != "legacy-revision" {
		t.Fatal("migration changed saved intent", desired, revision, err)
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
	location := LibraryLocation{ID: strings.Repeat("c", 32), Application: "shared", Path: "/fixture/home/.agents/skills", LegacyAgent: "agents"}
	entries, err := s.RegisterLibraryLocationFor(uid, agent.ID, location)
	if err != nil || len(entries) != 1 || entries[0].ItemID != id || entries[0].BundleID != first.Bundle.ID {
		t.Fatal("known root duplicated the proven capture", entries, err)
	}
	// A different, explicit folder with the same name is never adopted by name.
	custom := LibraryLocation{ID: strings.Repeat("d", 32), Application: "shared", Path: "/fixture/custom"}
	entries, err = s.RegisterLibraryLocationFor(uid, agent.ID, custom)
	if err != nil || len(entries) != 0 {
		t.Fatal("custom root claimed unrelated same-name content", entries, err)
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

func TestCanonicalLibraryTypedContentCannotEnterSkillOnlyTargets(t *testing.T) {
	s, uid, agent, environment := observationFixture(t)
	base := ""
	in := PrivateSkillWrite{Name: "review", RequestID: "typed_target_0001", ExpectedBase: &base, ItemType: LibraryAgent, Application: "claude", Format: "markdown", Files: []skillbundle.ContentFile{{Path: "review.md", Data: []byte("---\nname: review\ndescription: Review\n---\nInspect the patch.\n")}}}
	out, err := s.SaveLibraryContentFor(uid, "", in)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveSkillContentTargetFor(uid, SkillContentTargetWrite{EnvironmentID: environment, AgentID: agent.ID, BundleID: out.Bundle.ID, RevisionID: out.Revision.ID, Mode: "additive", ExpectedVersion: &base})
	if !errors.Is(err, ErrSkillContentTargetInvalid) {
		t.Fatal("typed item was dispatched to a skill-only adapter", err)
	}
	targets, err := s.SkillContentTargetsFor(uid)
	if err != nil || len(targets) != 0 {
		t.Fatal("unsupported deployment intent was saved", targets, err)
	}
}
