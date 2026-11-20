package store

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/skillbundle"
)

// legacyShelfCopy puts a linked copy on the shelf the way rows from before
// names were unique sit there: a second source of a name the library already
// has, which UpsertFor now refuses.
func legacyShelfCopy(t *testing.T, s *Store, uid int64, r ref.Ref) Skill {
	t.Helper()
	at := now()
	if _, err := s.db.Exec(`INSERT INTO skills(`+cols+`) VALUES(?,?,?,?,?,?,'','',0,0,'',0,0,0,?,?) ON CONFLICT(id) DO NOTHING`, r.ID, r.Source, r.Slug, r.SourceType, r.URL, r.Slug, at, at); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO shelf(user_id,skill_id,liked,notes,created_at,updated_at) VALUES(?,?,0,'',?,?)`, uid, r.ID, at, at); err != nil {
		t.Fatal(err)
	}
	sk, ok, err := s.GetFor(uid, r.ID)
	if err != nil || !ok {
		t.Fatal("legacy shelf copy missing", err)
	}
	return sk
}

// lowerPrivateSkillLimits shrinks the bundle and revision bounds for one test
// that loops up to them, so it fills the bound in moments rather than writing
// thousands of bundles under the race detector. The bound is still enforced by
// the same code; TestStorageLimitsHoldALibrary runs against the real limits.
func lowerPrivateSkillLimits(t *testing.T, bundles, revisions int) {
	t.Helper()
	oldBundles, oldRevisions := privateSkillBundleLimit, privateSkillRevisionLimit
	privateSkillBundleLimit, privateSkillRevisionLimit = bundles, revisions
	t.Cleanup(func() { privateSkillBundleLimit, privateSkillRevisionLimit = oldBundles, oldRevisions })
}

func bigRevision(key, base string, i int) PrivateSkillWrite {
	blob := make([]byte, 1536*1024)
	for n := range blob {
		blob[n] = byte(i*31 + n)
	}
	return PrivateSkillWrite{RequestID: key, ExpectedBase: &base, Name: "demo", Files: []skillbundle.ContentFile{
		{Path: "SKILL.md", Data: []byte("---\nname: demo\ndescription: Large example\n---\nBody\n")},
		{Path: "assets/blob.bin", Data: blob},
	}}
}

// Editing one skill many times used to fill the account's 256 revisions for good,
// after which no skill could be saved at all. Superseded revisions are freed
// when space is needed.
func TestPrivateStorageFreesSupersededRevisionsInsteadOfFillingUp(t *testing.T) {
	lowerPrivateSkillLimits(t, 32, 64)
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	first, err := s.SavePrivateSkillRevisionFor(uid, "", privateWrite("reclaim_req_000000", "", "edit 0"))
	if err != nil {
		t.Fatal(err)
	}
	head := first.Revision.ID
	for i := 1; i < privateSkillRevisionLimit; i++ {
		saved, err := s.SavePrivateSkillRevisionFor(uid, first.Bundle.ID, privateWrite(fmt.Sprintf("reclaim_req_%06d", i), head, fmt.Sprintf("edit %d", i)))
		if err != nil {
			t.Fatal(err)
		}
		head = saved.Revision.ID
	}

	saved, err := s.SavePrivateSkillRevisionFor(uid, first.Bundle.ID, privateWrite("reclaim_req_999999", head, "one more edit"))
	if err != nil {
		t.Fatalf("an edit was refused although the account stores a single skill: %v", err)
	}

	bundle, err := s.PrivateSkillBundleFor(uid, first.Bundle.ID)
	if err != nil || bundle.Head != saved.Revision.ID {
		t.Fatalf("the head did not move to the new edit: %+v %v", bundle, err)
	}
	counts := privateCounts(t, s)
	if counts[0] != 1 || counts[1] >= privateSkillRevisionLimit {
		t.Fatalf("superseded revisions were not freed: %v", counts)
	}
	if _, err = s.PrivateSkillRevisionFor(uid, first.Bundle.ID, saved.Revision.ID); err != nil {
		t.Fatalf("the new head cannot be read back: %v", err)
	}
	// Some history survives the first reclaim, so recent edits stay recoverable.
	if _, err = s.PrivateSkillRevisionFor(uid, first.Bundle.ID, head); err != nil {
		t.Fatalf("the edit just before the new head was freed: %v", err)
	}
}

// The 64 MiB bound is about stored bytes. Bytes only superseded revisions hold
// are freed when a write needs the room.
func TestPrivateStorageFreesSupersededBytes(t *testing.T) {
	lowerByteLimit(t)
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	first, err := s.SavePrivateSkillRevisionFor(uid, "", bigRevision("bytes_req_0000000", "", 0))
	if err != nil {
		t.Fatal(err)
	}
	head := first.Revision.ID
	for i := 1; i <= int(privateSkillByteLimit/(1536*1024)); i++ {
		saved, err := s.SavePrivateSkillRevisionFor(uid, first.Bundle.ID, bigRevision(fmt.Sprintf("bytes_req_%07d", i), head, i))
		if err != nil {
			t.Fatalf("edit %d of a single large skill was refused: %v", i, err)
		}
		head = saved.Revision.ID
	}
	var total int64
	if err = s.db.QueryRow(`SELECT coalesce(sum(length(data)),0) FROM private_skill_blobs WHERE user_id=?`, uid).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total > privateSkillByteLimit {
		t.Fatalf("stored bytes exceed the bound: %d", total)
	}
}

// Space that nothing else can reclaim is still a hard limit, and a refused write
// leaves nothing behind.
func TestPrivateStorageStillRefusesWhenEverySlotHoldsALiveSkill(t *testing.T) {
	lowerPrivateSkillLimits(t, 32, 64)
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	for i := 0; i < privateSkillBundleLimit; i++ {
		if _, err := s.SavePrivateSkillRevisionFor(uid, "", privateWrite(fmt.Sprintf("live_bundle_%06d", i), "", fmt.Sprintf("skill %d", i))); err != nil {
			t.Fatal(err)
		}
	}
	before := privateCounts(t, s)
	_, err := s.SavePrivateSkillRevisionFor(uid, "", privateWrite("live_bundle_over000", "", "one too many"))
	if !errors.Is(err, ErrPrivateSkillCapacity) {
		t.Fatalf("a full account accepted another skill: %v", err)
	}
	if fmt.Sprint(before) != fmt.Sprint(privateCounts(t, s)) {
		t.Fatalf("a refused write left rows behind: %v -> %v", before, privateCounts(t, s))
	}
}

// Bytes no revision refers to any more were stranded for good, because nothing
// ever deleted a blob. They no longer count against the account.
func TestPrivateStorageFreesBlobsNoRevisionUses(t *testing.T) {
	lowerByteLimit(t)
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if _, err := s.db.Exec(`INSERT INTO private_skill_blobs(user_id,digest,data) VALUES(?,?,zeroblob(?))`, uid, strings.Repeat("a", 64), privateSkillByteLimit); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SavePrivateSkillRevisionFor(uid, "", privateWrite("orphan_blob_00001", "", "fits once the orphan is freed")); err != nil {
		t.Fatalf("stranded bytes blocked a write: %v", err)
	}
	var orphans int
	if err := s.db.QueryRow(`SELECT count(*) FROM private_skill_blobs WHERE user_id=? AND digest=?`, uid, strings.Repeat("a", 64)).Scan(&orphans); err != nil || orphans != 0 {
		t.Fatalf("the stranded blob was kept: %d %v", orphans, err)
	}
}

// Reclaiming deletes rows, so it must never free a revision something can still
// need: a bundle head, a bound sync source, or an unfinished job's plan that an
// agent will download by ID. A finished job's plan is history and does not keep
// one alive.
func TestPrivateStorageKeepsEveryRevisionSomethingUses(t *testing.T) {
	s, uid, env := controlFixture(t)
	agent, _, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.SavePrivateSkillRevisionFor(uid, "", privateWrite("keep_revision_0000", "", "edit 0"))
	if err != nil {
		t.Fatal(err)
	}
	bundle := first.Bundle.ID
	revisions := []PrivateSkillResult{first}
	for i := 1; i < 10; i++ {
		saved, err := s.SavePrivateSkillRevisionFor(uid, bundle, privateWrite(fmt.Sprintf("keep_revision_%04d", i), revisions[i-1].Revision.ID, fmt.Sprintf("edit %d", i)))
		if err != nil {
			t.Fatal(err)
		}
		revisions = append(revisions, saved)
	}
	source := func(i int) string {
		return "private:" + bundle + ":" + revisions[i].Revision.ID + ":" + revisions[i].Revision.ManifestDigest
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := s.db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	stamp := now()
	// 1: a written skill's bound source. 2: a captured skill's bound source.
	exec(`INSERT INTO hosted_sync_sources(user_id,slug,bundle_id,revision_id,digest) VALUES(?,?,?,?,?)`, uid, "written", bundle, revisions[1].Revision.ID, revisions[1].Revision.ManifestDigest)
	exec(`INSERT INTO sync_private_sources(user_id,skill_id,bundle_id,revision_id,digest) VALUES(?,?,?,?,?)`, uid, "a/b/one", bundle, revisions[2].Revision.ID, revisions[2].Revision.ManifestDigest)
	// 4: a job that has not run yet. 7: a job that already finished.
	exec(`INSERT INTO sync_jobs(id,user_id,agent_id,target_id,kind,status,plan,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, "job_pending", uid, agent.ID, env, "apply", "queued", `{"actions":[{"add":true,"skill":{"source":"`+source(4)+`"}}]}`, stamp, stamp)
	exec(`INSERT INTO sync_jobs(id,user_id,agent_id,target_id,kind,status,plan,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, "job_done", uid, agent.ID, env, "apply", "succeeded", `{"actions":[{"add":true,"skill":{"source":"`+source(7)+`"}}]}`, stamp, stamp)

	survivors := func(keep int) map[int]bool {
		t.Helper()
		if err := s.withTx(func(tx *sql.Tx) error { return reclaimPrivateSkillStorage(tx, uid, keep) }); err != nil {
			t.Fatal(err)
		}
		out := map[int]bool{}
		for i, saved := range revisions {
			if _, err := s.PrivateSkillRevisionFor(uid, bundle, saved.Revision.ID); err == nil {
				out[i] = true
			} else if !errors.Is(err, ErrPrivateSkillMissing) {
				t.Fatalf("revision %d is damaged: %v", i, err)
			}
		}
		return out
	}

	// Gently: nothing in use is freed.
	got := survivors(2)
	for _, used := range []int{1, 2, 4, 9} {
		if !got[used] {
			t.Fatalf("revision %d is in use and was freed: %v", used, got)
		}
	}

	// Thoroughly: only what something uses remains. The finished job's revision (7)
	// is history, not a use.
	got = survivors(0)
	for i := range revisions {
		used := i == 1 || i == 2 || i == 4 || i == 9
		if got[i] != used {
			t.Fatalf("revision %d: kept %v, in use %v: %v", i, got[i], used, got)
		}
	}
	// Every blob left belongs to a revision that is left.
	var stranded int
	if err = s.db.QueryRow(`SELECT count(*) FROM private_skill_blobs b WHERE b.user_id=? AND NOT EXISTS (SELECT 1 FROM private_skill_revision_files f WHERE f.user_id=b.user_id AND f.digest=b.digest)`, uid).Scan(&stranded); err != nil || stranded != 0 {
		t.Fatalf("blobs were left behind: %d %v", stranded, err)
	}
}

// If a table ever gains a reference to a revision, reclaiming must learn about it
// before it can silently cascade-delete that data.
func TestPrivateStorageReclaimCoversEveryReferenceToARevision(t *testing.T) {
	s := newStore(t)
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND sql LIKE '%REFERENCES private_skill_revisions%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		got = append(got, name)
	}
	// private_skill_revision_files is the revision's own file list.
	want := []string{"private_skill_revision_files"}
	if !slices.Equal(got, want) {
		t.Fatalf("a table now references private_skill_revisions. Teach livePrivateRevisions to protect it, then update this list.\n got: %v\nwant: %v", got, want)
	}
}

func writtenSkill(t *testing.T, s *Store, uid int64, slug string) HostedSkill {
	t.Helper()
	saved, err := s.SaveHostedSkill(uid, HostedSkill{Slug: slug, Content: "---\nname: " + slug + "\ndescription: Written\n---\nBody\n"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.hostedPrivateSource(uid, saved); err != nil {
		t.Fatal(err)
	}
	return saved
}

func bundleFor(t *testing.T, s *Store, uid int64, slug string) string {
	t.Helper()
	var id string
	if err := s.db.QueryRow(`SELECT id FROM private_skill_bundles WHERE user_id=? AND name=? ORDER BY created_at DESC,rowid DESC LIMIT 1`, uid, slug).Scan(&id); err != nil {
		t.Fatalf("no bundle for %s: %v", slug, err)
	}
	return id
}

func bundleExists(t *testing.T, s *Store, uid int64, id string) bool {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM private_skill_bundles WHERE user_id=? AND id=?`, uid, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

// Deleting or renaming a written skill drops its binding but leaves its private
// bundle behind, and nothing can delete a bundle. Each one kept a slot against the
// bound for good, so an account that renamed or deleted enough skills could never
// sync a new one. Those bundles are copies of a skill's bytes; once nothing uses
// one, it is freed when a slot is needed: oldest first, and only as many as needed.
func TestDeletedWrittenSkillsDoNotKeepTheirBundleSlotForever(t *testing.T) {
	lowerPrivateSkillLimits(t, 32, 64)
	s, uid, _ := controlFixture(t)
	if err := s.ClaimPublishingUsername(uid, "sam"); err != nil {
		t.Fatal(err)
	}
	var err error
	for i := 0; i < 6; i++ {
		writtenSkill(t, s, uid, fmt.Sprintf("written-%d", i))
	}
	// Every other slot holds a skill somebody saved directly. Those are never freed.
	direct := []string{}
	for i := 6; i < privateSkillBundleLimit; i++ {
		saved, err := s.SavePrivateSkillRevisionFor(uid, "", privateWrite(fmt.Sprintf("direct_bundle_%05d", i), "", fmt.Sprintf("direct %d", i)))
		if err != nil {
			t.Fatal(err)
		}
		direct = append(direct, saved.Bundle.ID)
	}
	// written-0 .. written-3 are deleted; written-4 and written-5 stay.
	for i := 0; i < 4; i++ {
		if err = s.DeleteHostedSkill(uid, fmt.Sprintf("written-%d", i), ""); err != nil {
			t.Fatal(err)
		}
	}
	// written-2's bytes became a captured skill's bound source, so it is still in use.
	pinned := bundleFor(t, s, uid, "written-2")
	var pinnedHead string
	if err = s.db.QueryRow(`SELECT head FROM private_skill_bundles WHERE user_id=? AND id=?`, uid, pinned).Scan(&pinnedHead); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO sync_private_sources(user_id,skill_id,bundle_id,revision_id,digest) VALUES(?,'a/b/one',?,?,'')`, uid, pinned, pinnedHead); err != nil {
		t.Fatal(err)
	}
	fresh := writtenSkillOnly(t, s, uid, "fresh-1")

	// Just deleted, the bundles are minutes old at most. A bundle is bound in a
	// statement of its own after the write commits, so a new one looks unused for a
	// moment; it is not touched until it has had time to settle.
	if _, err = s.hostedPrivateSource(uid, fresh); err == nil || !strings.Contains(err.Error(), "private skill storage is full") {
		t.Fatalf("a bundle that was only just made was freed: %v", err)
	}

	age := func(slug string, at int64) {
		t.Helper()
		if _, err := s.db.Exec(`UPDATE private_skill_revisions SET created_at=? WHERE user_id=? AND bundle_id=?`, at, uid, bundleFor(t, s, uid, slug)); err != nil {
			t.Fatal(err)
		}
	}
	age("written-0", 2000)
	age("written-1", 3000)
	age("written-2", 1000) // the oldest, but bound
	age("written-3", 4000)
	// Still bound to live skills, and as old as can be: only the binding protects them.
	age("written-4", 1)
	age("written-5", 1)
	if _, err = s.db.Exec(`UPDATE private_skill_revisions SET created_at=1 WHERE user_id=? AND bundle_id IN (SELECT id FROM private_skill_bundles WHERE user_id=? AND name LIKE 'demo')`, uid, uid); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, slug := range []string{"written-0", "written-1", "written-2", "written-3", "written-4", "written-5"} {
		ids[slug] = bundleFor(t, s, uid, slug)
	}

	// One slot is needed, so exactly one unused bundle goes: the oldest that is not bound.
	if _, err = s.hostedPrivateSource(uid, fresh); err != nil {
		t.Fatalf("a deleted skill's bundle still holds a slot: %v", err)
	}
	if bundleExists(t, s, uid, ids["written-0"]) {
		t.Fatal("the oldest unused bundle was not freed")
	}
	for _, slug := range []string{"written-1", "written-2", "written-3", "written-4", "written-5"} {
		if !bundleExists(t, s, uid, ids[slug]) {
			t.Fatalf("%s was freed, but only one slot was needed or it is in use", slug)
		}
	}
	for _, id := range direct {
		if !bundleExists(t, s, uid, id) {
			t.Fatal("a skill somebody saved directly was freed")
		}
	}
	var bundles int
	if err = s.db.QueryRow(`SELECT count(*) FROM private_skill_bundles WHERE user_id=?`, uid).Scan(&bundles); err != nil || bundles != privateSkillBundleLimit {
		t.Fatalf("expected the account to be back at its bound: %d %v", bundles, err)
	}

	// Two more written skills need two more slots. written-1 and written-3 are free
	// to go; the captured skill's and the two still bound to written skills are not.
	for _, slug := range []string{"fresh-2", "fresh-3"} {
		if _, err = s.hostedPrivateSource(uid, writtenSkillOnly(t, s, uid, slug)); err != nil {
			t.Fatalf("%s: %v", slug, err)
		}
	}
	if bundleExists(t, s, uid, ids["written-1"]) || bundleExists(t, s, uid, ids["written-3"]) {
		t.Fatal("unused bundles were kept while skills waited for a slot")
	}
	if _, err = s.hostedPrivateSource(uid, writtenSkillOnly(t, s, uid, "fresh-4")); err == nil {
		t.Fatal("a bundle that is bound was freed to make room")
	}
	for _, slug := range []string{"written-2", "written-4", "written-5"} {
		if !bundleExists(t, s, uid, ids[slug]) {
			t.Fatalf("%s is in use and was freed", slug)
		}
	}
	// The skills written since each got a bundle of their own.
	for _, slug := range []string{"fresh-1", "fresh-2", "fresh-3"} {
		var bound int
		if err = s.db.QueryRow(`SELECT count(*) FROM hosted_sync_sources WHERE user_id=? AND slug=?`, uid, slug).Scan(&bound); err != nil || bound != 1 {
			t.Fatalf("%s was not bound: %d %v", slug, bound, err)
		}
	}
}

func writtenSkillOnly(t *testing.T, s *Store, uid int64, slug string) HostedSkill {
	t.Helper()
	saved, err := s.SaveHostedSkill(uid, HostedSkill{Slug: slug, Content: "---\nname: " + slug + "\ndescription: Written\n---\nBody\n"})
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func machineCaptureWrite(key, name string) PrivateSkillWrite {
	base := ""
	return PrivateSkillWrite{
		RequestID:    key,
		ExpectedBase: &base,
		Name:         name,
		Origin:       PrivateSkillOrigin{Kind: "import", Reference: `{"Name":"` + name + `"}`},
		Files: []skillbundle.ContentFile{{
			Path: "SKILL.md",
			Data: []byte("---\nname: " + name + "\ndescription: Captured from a machine\n---\nBody\n"),
		}},
	}
}

// A capture that started a new bundle every time left duplicates of one skill
// behind. They are copies, not something a person saved, so they are freed when a
// slot is needed, while the bundle a skill is still bound to stays.
func TestUnusedCaptureBundlesAreFreedButBoundOnesStay(t *testing.T) {
	lowerPrivateSkillLimits(t, 32, 64)
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if _, err := s.UpsertLocalFor(uid, "kept", Patch{}); err != nil {
		t.Fatal(err)
	}
	kept, err := s.SavePrivateSkillRevisionFor(uid, "", machineCaptureWrite("bound_capture_0001", "kept"))
	if err != nil {
		t.Fatal(err)
	}
	// The skill is bound to this bundle; it is in use even when a slot is needed.
	if _, err = s.db.Exec(`INSERT INTO sync_private_sources(user_id,skill_id,bundle_id,revision_id,digest) VALUES(?,?,?,?,?)`, uid, "local/kept", kept.Bundle.ID, kept.Revision.ID, kept.Revision.ManifestDigest); err != nil {
		t.Fatal(err)
	}
	var victim string
	for i := 1; i < privateSkillBundleLimit; i++ {
		saved, err := s.SavePrivateSkillRevisionFor(uid, "", machineCaptureWrite(fmt.Sprintf("unbound_capture_%04d", i), fmt.Sprintf("captured-%d", i)))
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			victim = saved.Bundle.ID
		}
	}
	// Only the oldest unused copy is old enough to be reclaimed.
	if _, err = s.db.Exec(`UPDATE private_skill_revisions SET created_at=1 WHERE user_id=? AND bundle_id=?`, uid, victim); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SavePrivateSkillRevisionFor(uid, "", machineCaptureWrite("new_capture_0001", "new")); err != nil {
		t.Fatalf("unused capture bundles kept the account full: %v", err)
	}
	if bundleExists(t, s, uid, victim) {
		t.Fatal("the oldest unused capture bundle still holds a slot")
	}
	if !bundleExists(t, s, uid, kept.Bundle.ID) {
		t.Fatal("the bundle a skill is bound to was freed")
	}
	var bundles int
	if err = s.db.QueryRow(`SELECT count(*) FROM private_skill_bundles WHERE user_id=?`, uid).Scan(&bundles); err != nil || bundles != privateSkillBundleLimit {
		t.Fatalf("expected the account to be back at its bound: %d %v", bundles, err)
	}
}
