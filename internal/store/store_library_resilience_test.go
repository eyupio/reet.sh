package store

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

// libraryFixture is the state an account is in when it presses Sync all
// machines: one laptop with a complete home-folder scan, the shared library on,
// and a shelf holding a/b/one.
func libraryFixture(t *testing.T) (*Store, int64, string) {
	t.Helper()
	s, uid, id := controlFixture(t)
	if err := s.SetSyncLibraryOrigin(uid, "https://reet.example"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.EnrollSyncAgent(uid, "laptop", "test"); err != nil {
		t.Fatal(err)
	}
	return s, uid, id
}

func desiredIDs(t *testing.T, s *Store, uid int64, target string) []string {
	t.Helper()
	targets, err := s.SyncTargetsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range targets {
		if item.Target.ID == target {
			ids := []string{}
			for _, sk := range item.Desired {
				ids = append(ids, sk.ID)
			}
			slices.Sort(ids)
			return ids
		}
	}
	t.Fatalf("no plan for %s: %+v", target, targets)
	return nil
}

// Two shelf entries claiming one name used to flag every copy, make Sync all
// machines fail and put every machine into "Sync needs attention", which held
// back the rest of the library. One copy is chosen without asking.
func TestSyncAllMachinesChoosesOneCopyOfADuplicatedName(t *testing.T) {
	s, uid, id := controlFixture(t)
	// A plan saved before the second copy arrived already installs the first.
	if err := s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", Mode: "additive", SkillIDs: []string{"a/b/one"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSyncLibraryOrigin(uid, "https://reet.example"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.EnrollSyncAgent(uid, "laptop", "test"); err != nil {
		t.Fatal(err)
	}
	for _, in := range []string{"c/d/one", "e/f/two"} {
		if _, err := s.UpsertFor(uid, mustRef(t, in), Patch{}); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.SyncLibraryMachinesFor(uid, "owner", "https://reet.example"); err != nil {
		t.Fatalf("a duplicated name stopped Sync all machines: %v", err)
	}

	if got := desiredIDs(t, s, uid, id); !slices.Equal(got, []string{"a/b/one", "e/f/two"}) {
		t.Fatalf("wrong skills selected: %v", got)
	}
	agents, err := s.SyncAgentsFor(uid)
	if err != nil || len(agents) != 1 || !agents[0].KeepSynced {
		t.Fatalf("automatic sync was not switched on: %+v %v", agents, err)
	}
	if err = s.MaintainSync(uid, "owner", agents[0]); err != nil {
		t.Fatal(err)
	}
	targets, err := s.SyncTargetsFor(uid)
	if err != nil || len(targets) != 1 || targets[0].Problem != "" {
		t.Fatalf("the machine was still marked as needing attention: %+v %v", targets, err)
	}
	library, err := s.SyncLibraryFor(uid, "owner")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]int{}
	for _, sk := range library {
		names[sk.Slug]++
		if sk.Problem != "" {
			t.Fatalf("a duplicated name is still reported as a problem: %+v", sk)
		}
	}
	if names["one"] != 1 || names["two"] != 1 {
		t.Fatalf("the library should hold one copy per name: %v", names)
	}
}

// With no plan to follow, the copy that has been on the shelf longest is kept,
// and the other is reported as left out rather than as an error.
func TestDuplicateNameWithoutAPlanKeepsTheOldestCopy(t *testing.T) {
	s, uid, _ := libraryFixture(t)
	if _, err := s.db.Exec(`DELETE FROM skills WHERE id=?`, "a/b/one"); err != nil {
		t.Fatal(err)
	}
	older, err := s.UpsertFor(uid, mustRef(t, "z/y/one"), Patch{})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	newer, err := s.UpsertFor(uid, mustRef(t, "a/b/one"), Patch{})
	if err != nil {
		t.Fatal(err)
	}

	view, err := s.SyncLibraryViewFor(uid, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Duplicates) != 1 || view.Duplicates[0].Keep.ID != older.ID || len(view.Duplicates[0].Remove) != 1 || view.Duplicates[0].Remove[0].ID != newer.ID {
		t.Fatalf("the oldest copy should be kept: %+v", view.Duplicates)
	}
	for _, sk := range view.Skills {
		if sk.ID == newer.ID {
			t.Fatalf("the left-out copy is still in the library: %+v", sk)
		}
	}
}

// A skill the account wrote itself outranks a third-party copy of the name when
// no saved plan says otherwise.
func TestDuplicateNamePrefersTheAccountsOwnWrittenSkill(t *testing.T) {
	s, uid, _ := libraryFixture(t)
	if err := s.ClaimPublishingUsername(uid, "sam"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertFor(uid, mustRef(t, "x/y/reet-notes"), Patch{}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := s.SaveHostedSkill(uid, HostedSkill{Slug: "reet-notes", Content: "---\nname: reet-notes\ndescription: Mine\n---\nBody\n"}); err != nil {
		t.Fatal(err)
	}
	view, err := s.SyncLibraryViewFor(uid, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Duplicates) != 1 || !strings.HasPrefix(view.Duplicates[0].Keep.ID, "hosted/") || view.Duplicates[0].Keep.SourceType != "private" {
		t.Fatalf("the written skill should win: %+v", view.Duplicates)
	}
}

// One skill that cannot be saved privately is left out and reported. It must not
// stop Sync all machines from switching every machine on, nor take the rest of
// the library with it.
func TestSyncAllMachinesSkipsASkillThatCannotBeStoredPrivately(t *testing.T) {
	s, uid, id := libraryFixture(t)
	if err := s.ClaimPublishingUsername(uid, "sam"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveHostedSkill(uid, HostedSkill{Slug: "reet-draft", Content: "---\nname: reet-draft\ndescription: Draft\n---\nBody\n"}); err != nil {
		t.Fatal(err)
	}
	// Every private bundle slot holds a live skill, so the draft cannot get one.
	for i := 0; i < MaxPrivateSkillBundles; i++ {
		if _, err := s.SavePrivateSkillRevisionFor(uid, "", privateWrite(fmt.Sprintf("full_bundle_%06d", i), "", fmt.Sprintf("skill %d", i))); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.SyncLibraryMachinesFor(uid, "owner", "https://reet.example"); err != nil {
		t.Fatalf("one skill that cannot be stored stopped Sync all machines: %v", err)
	}

	if got := desiredIDs(t, s, uid, id); !slices.Equal(got, []string{"a/b/one"}) {
		t.Fatalf("the rest of the library should still sync: %v", got)
	}
	agents, err := s.SyncAgentsFor(uid)
	if err != nil || len(agents) != 1 || !agents[0].KeepSynced {
		t.Fatalf("automatic sync was not switched on: %+v %v", agents, err)
	}
	if err = s.MaintainSync(uid, "owner", agents[0]); err != nil {
		t.Fatal(err)
	}
	targets, err := s.SyncTargetsFor(uid)
	if err != nil || len(targets) != 1 || targets[0].Problem != "" {
		t.Fatalf("one skill's problem became the machine's: %+v %v", targets, err)
	}
	library, err := s.SyncLibraryFor(uid, "owner")
	if err != nil {
		t.Fatal(err)
	}
	var reported bool
	for _, sk := range library {
		if sk.Slug == "reet-draft" {
			reported = strings.Contains(sk.Problem, "cannot be synced privately")
		}
	}
	if !reported {
		t.Fatalf("the skill left out is not explained: %+v", library)
	}
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
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	first, err := s.SavePrivateSkillRevisionFor(uid, "", privateWrite("reclaim_req_000000", "", "edit 0"))
	if err != nil {
		t.Fatal(err)
	}
	head := first.Revision.ID
	for i := 1; i < MaxPrivateSkillRevisions; i++ {
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
	if counts[0] != 1 || counts[1] >= MaxPrivateSkillRevisions {
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
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	first, err := s.SavePrivateSkillRevisionFor(uid, "", bigRevision("bytes_req_0000000", "", 0))
	if err != nil {
		t.Fatal(err)
	}
	head := first.Revision.ID
	for i := 1; i <= MaxPrivateSkillBytes/(1536*1024); i++ {
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
	if total > MaxPrivateSkillBytes {
		t.Fatalf("stored bytes exceed the bound: %d", total)
	}
}

// Space that nothing else can reclaim is still a hard limit, and a refused write
// leaves nothing behind.
func TestPrivateStorageStillRefusesWhenEverySlotHoldsALiveSkill(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	for i := 0; i < MaxPrivateSkillBundles; i++ {
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
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if _, err := s.db.Exec(`INSERT INTO private_skill_blobs(user_id,digest,data) VALUES(?,?,zeroblob(?))`, uid, strings.Repeat("a", 64), MaxPrivateSkillBytes); err != nil {
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
// need: a bundle head, a bound sync source, a pin or receipt (both foreign keys
// that cascade), a saved plan, or an unfinished job's plan that an agent will
// download by ID. A finished job's plan is history and does not keep one alive.
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
	// 3: a saved plan. 4: a job that has not run yet. 7: a job that already finished.
	exec(`INSERT INTO sync_targets(user_id,target_id,source_id,source_name,desired,revision,updated_at) VALUES(?,?,?,?,?,?,?)`, uid, env, "library", "Library", `[{"id":"x","slug":"x","source":"`+source(3)+`","sourceType":"private"}]`, "r1", stamp)
	exec(`INSERT INTO sync_jobs(id,user_id,agent_id,target_id,kind,status,plan,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, "job_pending", uid, agent.ID, env, "apply", "queued", `{"actions":[{"add":true,"skill":{"source":"`+source(4)+`"}}]}`, stamp, stamp)
	exec(`INSERT INTO sync_jobs(id,user_id,agent_id,target_id,kind,status,plan,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, "job_done", uid, agent.ID, env, "apply", "succeeded", `{"actions":[{"add":true,"skill":{"source":"`+source(7)+`"}}]}`, stamp, stamp)
	// 5: pinned to a machine. 6: recorded in an install receipt.
	exec(`INSERT INTO skill_content_targets(id,user_id,environment_id,agent_id,name,bundle_id,revision_id,mode,version,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, "pin_1", uid, env, agent.ID, "pinned", bundle, revisions[5].Revision.ID, "pin", "v1", stamp)
	exec(`INSERT INTO skill_content_receipts(id,user_id,environment_id,agent_id,name,bundle_id,revision_id,root_id,adapter,schema_version,managed,provenance,evidence,request_key,version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, "receipt_1", uid, env, agent.ID, "received", bundle, revisions[6].Revision.ID, "root", "agents", 1, "{}", "{}", "{}", "receipt_key_000001", "v1", stamp, stamp)

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

	// Gently: the two newest superseded revisions that nothing uses (8 and 0's
	// neighbours) stay as history.
	got := survivors(2)
	for _, used := range []int{1, 2, 3, 4, 5, 6, 9} {
		if !got[used] {
			t.Fatalf("revision %d is in use and was freed: %v", used, got)
		}
	}
	if !got[8] || !got[7] || got[0] {
		t.Fatalf("expected the two newest unused revisions kept as history and the oldest freed: %v", got)
	}

	// Thoroughly: only what something uses remains. The finished job's revision (7)
	// is history, not a use.
	got = survivors(0)
	for _, used := range []int{1, 2, 3, 4, 5, 6, 9} {
		if !got[used] {
			t.Fatalf("revision %d is in use and was freed: %v", used, got)
		}
	}
	if got[0] || got[7] || got[8] {
		t.Fatalf("unused revisions were kept: %v", got)
	}

	var pins, receipts int
	if err = s.db.QueryRow(`SELECT count(*) FROM skill_content_targets WHERE user_id=?`, uid).Scan(&pins); err != nil || pins != 1 {
		t.Fatalf("the pin was cascaded away: %d %v", pins, err)
	}
	if err = s.db.QueryRow(`SELECT count(*) FROM skill_content_receipts WHERE user_id=?`, uid).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatalf("the receipt was cascaded away: %d %v", receipts, err)
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
	// private_skill_revision_files is the revision's own file list. The other two
	// are read by livePrivateRevisions.
	want := []string{"private_skill_revision_files", "skill_content_receipts", "skill_content_targets"}
	if !slices.Equal(got, want) {
		t.Fatalf("a table now references private_skill_revisions. Teach livePrivateRevisions to protect it, then update this list.\n got: %v\nwant: %v", got, want)
	}
}

// libraryPair is two enrolled machines with complete scans: "laptop" holds a
// skill that exists nowhere else, and "desktop" is the machine being brought up
// to date. The shelf holds a/b/one.
func libraryPair(t *testing.T) (s *Store, uid int64, laptop, desktop SyncAgent, desktopEnv string) {
	t.Helper()
	s = newStore(t)
	uid, _ = s.DefaultOwner()
	if err := s.SetSyncLibraryOrigin(uid, "https://reet.example"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertFor(uid, mustRef(t, "a/b/one"), Patch{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertLocalFor(uid, "huge", Patch{}); err != nil {
		t.Fatal(err)
	}
	enrol := func(host string) (SyncAgent, string) {
		agent, _, err := s.EnrollSyncAgent(uid, host, "test")
		if err != nil {
			t.Fatal(err)
		}
		if err = s.TouchEnvironmentFor(uid, host, "agents", "", ""); err != nil {
			t.Fatal(err)
		}
		env := EnvironmentIDFor(uid, host, "agents", "")
		if err = s.RecordInventoryFor(uid, env, true, MachineInfo{}); err != nil {
			t.Fatal(err)
		}
		return agent, env
	}
	laptop, _ = enrol("laptop")
	if err := s.RecordEnvironmentFor(uid, "local/huge", "laptop", "agents", "", "", ""); err != nil {
		t.Fatal(err)
	}
	desktop, desktopEnv = enrol("desktop")
	return
}

// requireDesktopSyncs presses Sync all machines, lets the desktop's agent poll, and
// requires that the desktop is not stuck: it gets a plan holding the skills that
// can be installed and a job to install them, with no problem recorded against it.
func requireDesktopSyncs(t *testing.T, s *Store, uid int64, desktop SyncAgent, desktopEnv string) {
	t.Helper()
	if err := s.SyncLibraryMachinesFor(uid, "owner", "https://reet.example"); err != nil {
		t.Fatalf("Sync all machines failed: %v", err)
	}
	desktop.KeepSynced = true
	for range 3 {
		if err := s.MaintainSync(uid, "owner", desktop); err != nil {
			t.Fatal(err)
		}
	}
	targets, err := s.SyncTargetsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, target := range targets {
		if target.Target.ID != desktopEnv {
			continue
		}
		found = true
		if target.Problem != "" {
			t.Fatalf("one skill that cannot be copied yet put the whole machine into trouble: %q", target.Problem)
		}
		var ids []string
		for _, sk := range target.Desired {
			ids = append(ids, sk.ID)
		}
		if !slices.Contains(ids, "a/b/one") || slices.Contains(ids, "local/huge") {
			t.Fatalf("the skills that can be installed should be planned, and the one that cannot should not: %v", ids)
		}
	}
	if !found {
		t.Fatal("the desktop has no plan")
	}
	jobs, err := s.SyncJobsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range jobs {
		if job.AgentID == desktop.ID {
			return
		}
	}
	t.Fatalf("nothing was queued for the desktop although a/b/one is missing there: %+v", jobs)
}

// A skill that exists only on a machine that is switched off cannot be copied yet.
// That is waiting, not a failure, and it must not freeze every other machine.
func TestMachineOnlySkillOnAnOfflineMachineDoesNotFreezeTheOthers(t *testing.T) {
	s, uid, _, desktop, desktopEnv := libraryPair(t)
	// The laptop has never reported that it can hand over files, so no copy can be requested.
	requireDesktopSyncs(t, s, uid, desktop, desktopEnv)
}

// A copy that is queued for a machine that has since gone offline, or that stopped
// for a person to review some files, is also just waiting.
func TestACopyThatIsStillWaitingDoesNotHoldBackTheMachine(t *testing.T) {
	for _, state := range []string{"queued", "reviewing"} {
		t.Run(state, func(t *testing.T) {
			s, uid, laptop, desktop, desktopEnv := libraryPair(t)
			if _, err := s.ClaimRemoteImportFor(uid, laptop.ID, strings.Repeat("a", 48)); err != nil {
				t.Fatal(err)
			}
			request, err := s.RequestMachineSkillSync(uid, "local/huge", "desktop", "", "library_"+secretHash("local/huge"), desktopEnv, true)
			if err != nil {
				t.Fatal(err)
			}
			// The request was made while the laptop was reachable; it then stopped answering.
			if _, err = s.db.Exec(`UPDATE machine_import_requests SET state=?,data=json_set(data,'$.state',?) WHERE id=?`, state, state, request.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = s.db.Exec(`UPDATE machine_import_capabilities SET observed_at=1 WHERE agent_id=?`, laptop.ID); err != nil {
				t.Fatal(err)
			}
			requireDesktopSyncs(t, s, uid, desktop, desktopEnv)
		})
	}
}

// Pressing Sync all machines is a person asking for another go. A machine whose last
// attempt failed, or was cancelled, is tried again by that press, so the button is
// never a dead end. The machine's own polls still leave it alone: a problem that
// retrying cannot fix must not turn into a stream of failed jobs.
func TestSyncAllMachinesTriesAgainAfterAFailedOrCancelledAttempt(t *testing.T) {
	for _, outcome := range []string{"failed", "cancelled"} {
		t.Run(outcome, func(t *testing.T) {
			s, uid, _, desktop, desktopEnv := libraryPair(t)
			requireDesktopSyncs(t, s, uid, desktop, desktopEnv)
			attempts := func(statuses ...string) []SyncJob {
				t.Helper()
				jobs, err := s.SyncJobsFor(uid)
				if err != nil {
					t.Fatal(err)
				}
				var out []SyncJob
				for _, job := range jobs {
					if job.AgentID == desktop.ID && slices.Contains(statuses, job.Status) {
						out = append(out, job)
					}
				}
				return out
			}
			// The attempt that Sync all machines started ends badly.
			if _, err := s.db.Exec(`UPDATE sync_jobs SET status=?,message=? WHERE user_id=? AND agent_id=?`, outcome, "The machine could not write to its skills folder.", uid, desktop.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := s.db.Exec(`UPDATE sync_runs SET status=? WHERE user_id=? AND agent_id=?`, outcome, uid, desktop.ID); err != nil {
				t.Fatal(err)
			}
			if got := attempts("queued", "running"); len(got) != 0 {
				t.Fatalf("the fixture should have no attempt in progress: %+v", got)
			}

			desktop.KeepSynced = true
			if err := s.MaintainSync(uid, "owner", desktop); err != nil {
				t.Fatal(err)
			}
			if got := attempts("queued", "running"); len(got) != 0 {
				t.Fatalf("the machine's own poll started a new attempt after a %s one: %+v", outcome, got)
			}

			if err := s.SyncLibraryMachinesFor(uid, "owner", "https://reet.example"); err != nil {
				t.Fatalf("Sync all machines failed: %v", err)
			}
			if got := attempts("queued", "running"); len(got) != 1 {
				t.Fatalf("pressing Sync all machines should try the machine again after a %s attempt, but it queued %d: %+v", outcome, len(got), attempts("queued", "running", "failed", "cancelled"))
			}
		})
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
	s, uid, env := controlFixture(t)
	agent, _, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ClaimPublishingUsername(uid, "sam"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		writtenSkill(t, s, uid, fmt.Sprintf("written-%d", i))
	}
	// Every other slot holds a skill somebody saved directly. Those are never freed.
	direct := []string{}
	for i := 6; i < MaxPrivateSkillBundles; i++ {
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
	// written-2 had been pinned to a machine, so it is still in use.
	pinned := bundleFor(t, s, uid, "written-2")
	var pinnedHead string
	if err = s.db.QueryRow(`SELECT head FROM private_skill_bundles WHERE user_id=? AND id=?`, uid, pinned).Scan(&pinnedHead); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO skill_content_targets(id,user_id,environment_id,agent_id,name,bundle_id,revision_id,mode,version,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, "pin_1", uid, env, agent.ID, "pinned", pinned, pinnedHead, "pin", "v1", now()); err != nil {
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
	age("written-2", 1000) // the oldest, but pinned
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

	// One slot is needed, so exactly one unused bundle goes: the oldest that is not pinned.
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
	if err = s.db.QueryRow(`SELECT count(*) FROM private_skill_bundles WHERE user_id=?`, uid).Scan(&bundles); err != nil || bundles != MaxPrivateSkillBundles {
		t.Fatalf("expected the account to be back at its bound: %d %v", bundles, err)
	}

	// Two more written skills need two more slots. written-1 and written-3 are free
	// to go; the pinned one and the two still bound are not.
	for _, slug := range []string{"fresh-2", "fresh-3"} {
		if _, err = s.hostedPrivateSource(uid, writtenSkillOnly(t, s, uid, slug)); err != nil {
			t.Fatalf("%s: %v", slug, err)
		}
	}
	if bundleExists(t, s, uid, ids["written-1"]) || bundleExists(t, s, uid, ids["written-3"]) {
		t.Fatal("unused bundles were kept while skills waited for a slot")
	}
	if _, err = s.hostedPrivateSource(uid, writtenSkillOnly(t, s, uid, "fresh-4")); err == nil {
		t.Fatal("a bundle that is pinned or bound was freed to make room")
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
	for i := 1; i < MaxPrivateSkillBundles; i++ {
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
	if err = s.db.QueryRow(`SELECT count(*) FROM private_skill_bundles WHERE user_id=?`, uid).Scan(&bundles); err != nil || bundles != MaxPrivateSkillBundles {
		t.Fatalf("expected the account to be back at its bound: %d %v", bundles, err)
	}
}

// A binary update can sit queued for a machine that is switched off. It is not
// skill work, so it must not stop Sync all machines from putting that machine on
// the shared library, nor stop a later poll from following the library.
func TestAQueuedUpdateDoesNotBlockTheSharedLibrary(t *testing.T) {
	s, uid, id := controlFixture(t)
	if err := s.SetSyncLibraryOrigin(uid, "https://reet.example"); err != nil {
		t.Fatal(err)
	}
	agent, _, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	// An older custom selection that does not follow the library.
	if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", SkillIDs: []string{"a/b/one"}, Mode: "additive"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.QueueAgentUpdate(uid, agent.ID, "latest"); err != nil {
		t.Fatal(err)
	}

	if err = s.SyncLibraryMachinesFor(uid, "owner", "https://reet.example"); err != nil {
		t.Fatalf("a queued update stopped Sync all machines: %v", err)
	}
	targets, err := s.SyncTargetsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].SourceID != "library" || !targets[0].FollowSource {
		t.Fatalf("the machine was not put on the shared library: %+v", targets)
	}

	// The library gains an entry. The machine's own poll follows it even though
	// the update is still waiting for it. Let the plan the button just queued
	// settle first, so only the update could hold the poll back.
	if _, err = s.db.Exec(`UPDATE sync_jobs SET status='succeeded' WHERE user_id=? AND agent_id=? AND kind<>'update'`, uid, agent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpsertFor(uid, mustRef(t, "c/d/two"), Patch{}); err != nil {
		t.Fatal(err)
	}
	agent.KeepSynced = true
	if err = s.MaintainSync(uid, "owner", agent); err != nil {
		t.Fatalf("a queued update stopped the machine's poll: %v", err)
	}
	targets, err = s.SyncTargetsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, sk := range targets[0].Desired {
		ids = append(ids, sk.ID)
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"a/b/one", "c/d/two"}) {
		t.Fatalf("the poll did not follow the library: %v", ids)
	}
	var updates int
	if err = s.db.QueryRow(`SELECT count(*) FROM sync_jobs WHERE user_id=? AND agent_id=? AND kind='update' AND status='queued'`, uid, agent.ID).Scan(&updates); err != nil || updates != 1 {
		t.Fatalf("the queued update should still wait its turn: %d %v", updates, err)
	}
}
