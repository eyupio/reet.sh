package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/eyupio/reet.sh/internal/ref"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func controlFixture(t *testing.T) (*Store, int64, string) {
	t.Helper()
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	s.TouchEnvironmentFor(uid, "laptop", "agents", "", "")
	id := EnvironmentIDFor(uid, "laptop", "agents", "")
	s.RecordInventoryFor(uid, id, true, MachineInfo{})
	s.UpsertFor(uid, mustRef(t, "a/b/one"), Patch{})
	return s, uid, id
}
func TestDesiredSelectionIsolationAndRevision(t *testing.T) {
	s, uid, id := controlFixture(t)
	in := DesiredSelection{TargetID: id, SourceID: "shelf", SkillIDs: []string{"a/b/one"}, Mode: "additive"}
	if err := s.SaveDesiredFor(uid, "owner", in); err != nil {
		t.Fatal(err)
	}
	targets, _ := s.SyncTargetsFor(uid)
	if len(targets) != 1 || targets[0].Mode != "additive" || len(targets[0].Missing) != 1 {
		t.Fatalf("%+v", targets)
	}
	if err := s.SaveDesiredFor(uid, "owner", in); err == nil {
		t.Fatal("stale write accepted")
	}
	in.Revision = targets[0].Revision
	in.SkillIDs = []string{"other/private/skill"}
	if err := s.SaveDesiredFor(uid, "owner", in); err == nil {
		t.Fatal("unowned skill accepted")
	}
	in.SkillIDs = []string{"a/b/one"}
	if err := s.SaveDesiredFor(uid, "viewer", in); err == nil {
		t.Fatal("viewer write accepted")
	}
	s.RecordInventoryFor(uid, id, false, MachineInfo{})
	if err := s.SaveDesiredFor(uid, "owner", in); err == nil {
		t.Fatal("incomplete target accepted")
	}
}
func TestSyncJobApprovalIdentityAndReceipt(t *testing.T) {
	s, uid, id := controlFixture(t)
	if err := s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", SkillIDs: []string{"a/b/one"}, Mode: "additive"}); err != nil {
		t.Fatal(err)
	}
	a, secret, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateSyncAgent(uid, a.ID, "wrong", "test"); err == nil {
		t.Fatal("wrong enrollment secret accepted")
	}
	a, err = s.AuthenticateSyncAgent(uid, a.ID, secret, "test")
	if err != nil {
		t.Fatal(err)
	}
	other, _, _ := s.EnrollSyncAgent(uid, "desktop", "test")
	if _, err = s.QueueSyncJob(uid, other.ID, id, "preview"); err == nil {
		t.Fatal("wrong physical target queued")
	}
	j, err := s.QueueSyncJob(uid, a.ID, id, "preview")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ControlSyncJob(uid, j.ID, "approve", "x"); err == nil {
		t.Fatal("unpreviewed approval accepted")
	}
	s.ControlSyncAgent(uid, a.ID, "pause")
	paused, _ := s.AuthenticateSyncAgent(uid, a.ID, secret, "test")
	if job, _ := s.ClaimSyncJob(uid, paused); job != nil {
		t.Fatal("paused agent claimed work")
	}
	s.ControlSyncAgent(uid, a.ID, "resume")
	job, err := s.ClaimSyncJob(uid, a)
	if err != nil || job == nil {
		t.Fatal(err)
	}
	if again, _ := s.ClaimSyncJob(uid, a); again != nil {
		t.Fatal("duplicate execution")
	}
	hash := strings.Repeat("a", 64)
	plan := json.RawMessage(`{"actions":[]}`)
	if err = s.FinishSyncJob(uid, a.ID, j.ID, "awaiting-approval", hash, "", plan); err != nil {
		t.Fatal(err)
	}
	if err = s.ControlSyncJob(uid, j.ID, "approve", "wrong"); err == nil {
		t.Fatal("wrong plan accepted")
	}
	if err = s.ControlSyncJob(uid, j.ID, "approve", hash); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishSyncJob(uid, a.ID, j.ID, "awaiting-approval", hash, "", plan); err != nil {
		t.Fatal("lost preview acknowledgment must be idempotent:", err)
	}
	job, err = s.ClaimSyncJob(uid, a)
	if err != nil || job == nil || job.Kind != "apply" {
		t.Fatalf("%+v %v", job, err)
	}
	if err = s.FinishSyncJob(uid, a.ID, j.ID, "succeeded", hash, "", plan); err != nil {
		t.Fatal(err)
	}
	targets, _ := s.SyncTargetsFor(uid)
	if targets[0].AppliedRevision != targets[0].Revision {
		t.Fatal("revision not acknowledged")
	}
	s.ControlSyncAgent(uid, a.ID, "revoke")
	if _, err = s.AuthenticateSyncAgent(uid, a.ID, secret, "test"); err == nil {
		t.Fatal("revoked agent accepted")
	}
}
func TestSyncPlanExpiresOnDesiredChangeAndLease(t *testing.T) {
	s, uid, id := controlFixture(t)
	selection := DesiredSelection{TargetID: id, SourceID: "shelf", SkillIDs: []string{"a/b/one"}, Mode: "mirror"}
	s.SaveDesiredFor(uid, "owner", selection)
	a, _, _ := s.EnrollSyncAgent(uid, "laptop", "test")
	j, _ := s.QueueSyncJob(uid, a.ID, id, "preview")
	s.ClaimSyncJob(uid, a)
	hash := strings.Repeat("b", 64)
	s.FinishSyncJob(uid, a.ID, j.ID, "awaiting-approval", hash, "", json.RawMessage(`{}`))
	targets, _ := s.SyncTargetsFor(uid)
	selection.Revision = targets[0].Revision
	s.SaveDesiredFor(uid, "owner", selection)
	if err := s.ControlSyncJob(uid, j.ID, "approve", hash); err == nil {
		t.Fatal("outdated revision approved")
	}
	s.ControlSyncJob(uid, j.ID, "cancel", "")
	j, _ = s.QueueSyncJob(uid, a.ID, id, "preview")
	s.ClaimSyncJob(uid, a)
	s.db.Exec(`UPDATE sync_jobs SET updated_at=? WHERE id=?`, time.Now().Add(-11*time.Minute).UnixMilli(), j.ID)
	if job, err := s.ClaimSyncJob(uid, a); err != nil || job != nil {
		t.Fatal("expired work replayed")
	}
	jobs, _ := s.SyncJobsFor(uid)
	found := false
	for _, job := range jobs {
		if job.ID == j.ID && job.Status == "failed" {
			found = true
		}
	}
	if !found {
		t.Fatal("expired execution not visible")
	}
}
func TestReconcileThreeWayAndDeletions(t *testing.T) {
	one := SyncSkill{ID: "a/b/one", Slug: "one", Source: "a/b", SourceType: "github"}
	two := SyncSkill{ID: "a/b/two", Slug: "two", Source: "a/b", SourceType: "github"}
	changed := one
	changed.ID = "c/d/one"
	changed.Source = "c/d"
	out, conflicts := ReconcileSkills([]SyncSkill{one}, []SyncSkill{one, two}, nil)
	if len(conflicts) != 0 || len(out) != 1 || out[0].ID != two.ID {
		t.Fatalf("deletion resurrected: %v %v", out, conflicts)
	}
	_, conflicts = ReconcileSkills([]SyncSkill{one}, nil, []SyncSkill{changed})
	if len(conflicts) != 1 {
		t.Fatal("delete/change conflict lost")
	}
}
func TestAdoptPreservesShelfAndListAccess(t *testing.T) {
	s, uid, id := controlFixture(t)
	r := mustRef(t, "a/b/one")
	notes := "keep"
	used := false
	s.UpsertFor(uid, r, Patch{Notes: &notes, Used: &used})
	s.RecordEnvironmentFor(uid, r.ID, "laptop", "agents", "", "", "")
	if err := s.AdoptSyncSkillsFor(uid, "owner", id, []string{r.ID}, 0); err != nil {
		t.Fatal(err)
	}
	sk, _, _ := s.GetFor(uid, r.ID)
	if sk.Used || sk.Notes != "keep" {
		t.Fatal("adoption changed preferences")
	}
	if err := s.AdoptSyncSkillsFor(uid, "owner", id, []string{"not/observed/skill"}, 0); err == nil {
		t.Fatal("unobserved adoption accepted")
	}
	member, _ := s.CreateUser("member@sync.invalid", "Member", "member", "")
	list, _ := s.CreateList(uid, "Private", "", "personal")
	if _, err := s.SyncCandidatesFor(member.ID, "member", list.ID); err == nil {
		t.Fatal("private list leaked")
	}
}

func TestEnrolledRenamePreservesModeAndCancelsOldPlans(t *testing.T) {
	s, uid, id := controlFixture(t)
	if err := s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", SkillIDs: []string{"a/b/one"}, Mode: "additive"}); err != nil {
		t.Fatal(err)
	}
	a, secret, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.QueueSyncJob(uid, a.ID, id, "preview")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MergeMachinesFor(uid, "laptop", "workstation"); err != nil {
		t.Fatal(err)
	}
	a, err = s.AuthenticateSyncAgent(uid, a.ID, secret, "test")
	if err != nil || a.Host != "workstation" {
		t.Fatalf("identity lost: %+v %v", a, err)
	}
	targets, _ := s.SyncTargetsFor(uid)
	if targets[0].Mode != "additive" || targets[0].Target.Host != "workstation" {
		t.Fatalf("policy lost: %+v", targets)
	}
	jobs, _ := s.SyncJobsFor(uid)
	for _, job := range jobs {
		if job.ID == j.ID && job.Status != "cancelled" {
			t.Fatal("old approval survived rename")
		}
	}
}

func TestDraftSyncCandidatesArePrivateSources(t *testing.T) {
	s, uid, id := controlFixture(t)
	if err := s.ClaimPublishingUsername(uid, "publisher"); err != nil {
		t.Fatal(err)
	}
	content := func(slug, body string) string {
		return "---\nname: " + slug + "\ndescription: Fixture skill\n---\n\n" + body
	}
	for _, slug := range []string{"published", "draft"} {
		_, err := s.SaveHostedSkill(uid, HostedSkill{Slug: slug, Description: "Fixture skill", Content: content(slug, "Test"), Files: map[string][]byte{"notes/a.txt": []byte("a")}, Published: slug == "published"})
		if err != nil {
			t.Fatal(err)
		}
	}
	candidates, err := s.SyncHostedFor(uid, "https://reet.sh")
	if err != nil || len(candidates) != 2 {
		t.Fatalf("%+v %v", candidates, err)
	}
	var published, draft SyncSkill
	for _, sk := range candidates {
		if sk.Slug == "published" {
			published = sk
		} else {
			draft = sk
		}
	}
	// A published skill keeps its public install source; a draft is served privately from the controller.
	if published.ID != "hosted/reet.sh/publisher/published" || published.SourceType != "hosted" {
		t.Fatalf("published candidate changed: %+v", published)
	}
	if draft.ID != "hosted/reet.sh/publisher/draft" || draft.SourceType != "private" || draft.URL != "" {
		t.Fatalf("draft candidate: %+v", draft)
	}
	parts := strings.Split(draft.Source, ":")
	if len(parts) != 4 || parts[0] != "private" || len(parts[1]) != 32 || len(parts[2]) != 32 || len(parts[3]) != 64 {
		t.Fatalf("draft source: %q", draft.Source)
	}
	// The private revision holds the exact SKILL.md and supporting files the agent will write.
	revision, err := s.PrivateSkillRevisionFor(uid, parts[1], parts[2])
	if err != nil || revision.ManifestDigest != parts[3] || len(revision.Files) != 2 {
		t.Fatalf("%+v %v", revision, err)
	}
	for _, f := range revision.Files {
		if f.Path == "SKILL.md" && string(f.Data) != content("draft", "Test") || f.Path == "notes/a.txt" && string(f.Data) != "a" {
			t.Fatalf("revision file %s: %q", f.Path, f.Data)
		}
	}
	// Listing again writes nothing new; editing the draft appends one revision with a new digest.
	if again, err := s.SyncHostedFor(uid, "https://reet.sh"); err != nil || len(again) != 2 {
		t.Fatal(err)
	}
	if revisions, err := s.PrivateSkillRevisionsFor(uid, parts[1]); err != nil || len(revisions) != 1 {
		t.Fatalf("unchanged draft gained a revision: %d %v", len(revisions), err)
	}
	if _, err := s.SaveHostedSkill(uid, HostedSkill{Slug: "draft", Description: "Fixture skill", Content: content("draft", "Edited"), Files: map[string][]byte{"notes/a.txt": []byte("a")}}); err != nil {
		t.Fatal(err)
	}
	edited, err := s.SyncHostedFor(uid, "https://reet.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, sk := range edited {
		if sk.Slug == "draft" {
			next := strings.Split(sk.Source, ":")
			if next[1] != parts[1] || next[2] == parts[2] || next[3] == parts[3] {
				t.Fatalf("edited draft source %q should keep the bundle and move the revision from %q", sk.Source, draft.Source)
			}
		}
	}
	if revisions, err := s.PrivateSkillRevisionsFor(uid, parts[1]); err != nil || len(revisions) != 2 {
		t.Fatalf("edited draft revisions: %d %v", len(revisions), err)
	}
	// A draft can be saved into a shelf-sourced selection beside shelf skills, so the simple Sync page can choose it.
	if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", Origin: "https://reet.sh", SkillIDs: []string{"a/b/one", draft.ID}, Mode: "additive"}); err != nil {
		t.Fatal(err)
	}
	targets, _ := s.SyncTargetsFor(uid)
	if len(targets) != 1 || len(targets[0].Desired) != 2 {
		t.Fatalf("%+v", targets)
	}
	for _, sk := range targets[0].Desired {
		if sk.ID == draft.ID && (sk.SourceType != "private" || !strings.HasPrefix(sk.Source, "private:"+parts[1]+":")) {
			t.Fatalf("desired draft lost its private source: %+v", sk)
		}
	}
	// Whole-source following still needs public sources, and nothing leaks to another account.
	if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "hosted", Origin: "https://reet.sh", FollowSource: true, Mode: "additive", Revision: targets[0].Revision}); err == nil {
		t.Fatal("following the hosted collection should be refused")
	}
	other, _ := s.CreateUser("other@hosted.invalid", "Other", "member", "")
	if candidates, err = s.SyncHostedFor(other.ID, "https://reet.sh"); err != nil || len(candidates) != 0 {
		t.Fatal("another user's hosted source leaked")
	}
	if _, err := s.PrivateSkillRevisionFor(other.ID, parts[1], parts[2]); err == nil {
		t.Fatal("another account read a draft's private revision")
	}
	// Unpublishing keeps the shelf entry publishing created; the shelf must now offer the draft's private bytes, not a
	// public URL that no longer serves, wherever candidates come from.
	if _, err := s.SaveHostedSkill(uid, HostedSkill{Slug: "published", Description: "Fixture skill", Content: content("published", "Test"), Published: true}); err != nil {
		t.Fatal(err)
	}
	publishedRef, err := ref.ParseAt("https://reet.sh/publisher/published", "reet.sh")
	if err != nil {
		t.Fatal(err)
	}
	s.UpsertFor(uid, publishedRef, Patch{})
	if _, err := s.SaveHostedSkill(uid, HostedSkill{Slug: "published", Description: "Fixture skill", Content: content("published", "Test"), Published: false}); err != nil {
		t.Fatal(err)
	}
	shelf, err := s.SyncCandidatesFor(uid, "owner", 0)
	if err != nil {
		t.Fatal(err)
	}
	var unpublished *SyncSkill
	for i := range shelf {
		if shelf[i].ID == "hosted/reet.sh/publisher/published" {
			unpublished = &shelf[i]
		}
	}
	if unpublished == nil || unpublished.SourceType != "private" || !strings.HasPrefix(unpublished.Source, "private:") {
		t.Fatalf("unpublished skill on the shelf kept a public source: %+v", unpublished)
	}
	if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", Origin: "https://reet.sh", SkillIDs: []string{"hosted/reet.sh/publisher/published"}, Mode: "additive", Revision: targets[0].Revision}); err != nil {
		t.Fatal(err)
	}
	targets, _ = s.SyncTargetsFor(uid)
	if len(targets[0].Desired) != 1 || targets[0].Desired[0].SourceType != "private" {
		t.Fatalf("desired unpublished skill: %+v", targets[0].Desired)
	}
	// One draft that cannot be prepared is listed with its problem and is refused on save; the others stay usable.
	if _, err := s.db.Exec(`UPDATE users SET role='viewer' WHERE id=?`, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveHostedSkill(uid, HostedSkill{Slug: "broken", Description: "Fixture skill", Content: content("broken", "Test")}); err != nil {
		t.Fatal(err)
	}
	listed, err := s.SyncHostedFor(uid, "https://reet.sh")
	if err != nil || len(listed) != 3 {
		t.Fatalf("one unpreparable draft hid the collection: %+v %v", listed, err)
	}
	for _, sk := range listed {
		if sk.Slug == "broken" && (sk.Problem == "" || sk.Source != "" || sk.SourceType != "local") {
			t.Fatalf("broken draft: %+v", sk)
		}
		if sk.Slug == "draft" && sk.SourceType != "private" {
			t.Fatalf("usable draft lost its source beside a broken one: %+v", sk)
		}
	}
	if err := s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "hosted", Origin: "https://reet.sh", SkillIDs: []string{"hosted/reet.sh/publisher/broken"}, Mode: "additive", Revision: targets[0].Revision}); err == nil || !strings.Contains(err.Error(), "cannot be synced privately") {
		t.Fatalf("a draft with a problem was saved as installable, or refused for another reason: %v", err)
	}
	s.db.Exec(`UPDATE users SET role='owner' WHERE id=?`, uid)
	// Deleting the draft drops its binding; the immutable revisions stay, as the private store is append-only.
	if err := s.DeleteHostedSkill(uid, "draft"); err != nil {
		t.Fatal(err)
	}
	var bound int
	s.db.QueryRow(`SELECT count(*) FROM hosted_sync_sources WHERE user_id=? AND slug='draft'`, uid).Scan(&bound)
	if bound != 0 {
		t.Fatal("deleted draft kept its sync binding")
	}
}

func TestAuthenticateRemoteAgentUsesEnrollmentCredentialsOnly(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	a, secret, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AuthenticateRemoteAgent(a.ID, "wrong", "v1"); err == nil {
		t.Fatal("wrong secret accepted")
	}
	if _, _, err := s.AuthenticateRemoteAgent("not-an-enrollment", secret, "v1"); err == nil {
		t.Fatal("unknown enrollment accepted")
	}
	gotUID, got, err := s.AuthenticateRemoteAgent(a.ID, secret, "v2")
	if err != nil || gotUID != uid || got.ID != a.ID {
		t.Fatalf("authenticate: uid=%d agent=%+v err=%v", gotUID, got, err)
	}
	if _, err = s.AuthenticateSyncAgent(uid, a.ID, secret, "v2"); err != nil {
		t.Fatal(err)
	}
	if _, after, err := s.AuthenticateRemoteAgent(a.ID, secret, ""); err != nil || after.Version != "v2" {
		t.Fatalf("version update: %+v %v", after, err)
	}
	if err = s.ControlSyncAgent(uid, a.ID, "revoke"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AuthenticateRemoteAgent(a.ID, secret, "v3"); err == nil {
		t.Fatal("revoked enrollment accepted")
	}
	viewer, _ := s.CreateUser("viewer@remote.invalid", "Viewer", "viewer", "")
	other, otherSecret, _ := s.EnrollSyncAgent(viewer.ID, "desktop", "test")
	if gotUID, _, err := s.AuthenticateRemoteAgent(other.ID, otherSecret, "v3"); err != nil || gotUID != viewer.ID {
		t.Fatalf("enrollment owner mismatch: %d %v", gotUID, err)
	}
	if err := s.SetUserDisabled(viewer.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AuthenticateRemoteAgent(other.ID, otherSecret, "v3"); err == nil {
		t.Fatal("disabled account accepted")
	}
}

func TestFleetPreviewsArePerScopeAndExecutionStaysSerial(t *testing.T) {
	s, uid, first := controlFixture(t)
	s.TouchEnvironmentFor(uid, "laptop", "claude", "", "")
	second := EnvironmentIDFor(uid, "laptop", "claude", "")
	s.RecordInventoryFor(uid, second, true, MachineInfo{})
	for _, id := range []string{first, second} {
		if err := s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", SkillIDs: []string{"a/b/one"}, Mode: "additive"}); err != nil {
			t.Fatal(err)
		}
	}
	agent, _, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{first, second} {
		if _, err := s.QueueSyncJob(uid, agent.ID, id, "preview"); err != nil {
			t.Fatal("distinct scope must queue", err)
		}
	}
	if _, err := s.QueueSyncJob(uid, agent.ID, first, "preview"); err == nil {
		t.Fatal("duplicate active preview accepted")
	}
	if _, err := s.QueueSyncJob(uid, agent.ID, "", "scan"); err == nil {
		t.Fatal("scan must wait for previews")
	}
	one, err := s.ClaimSyncJob(uid, agent)
	if err != nil || one == nil {
		t.Fatal("first preview not claimed", err)
	}
	if other, err := s.ClaimSyncJob(uid, agent); err != nil || other != nil {
		t.Fatal("parallel execution accepted", err)
	}
	hash := strings.Repeat("c", 64)
	if err := s.FinishSyncJob(uid, agent.ID, one.ID, "awaiting-approval", hash, "", json.RawMessage(`{"actions":[]}`)); err != nil {
		t.Fatal(err)
	}
	two, err := s.ClaimSyncJob(uid, agent)
	if err != nil || two == nil || two.TargetID == one.TargetID {
		t.Fatal("second scope preview not claimed", err)
	}
	if err := s.FinishSyncJob(uid, agent.ID, two.ID, "awaiting-approval", hash, "", json.RawMessage(`{"actions":[]}`)); err != nil {
		t.Fatal(err)
	}
	if job, err := s.ClaimSyncJob(uid, agent); err != nil || job != nil {
		t.Fatal("preview applied without approval", err)
	}
	if err := s.ControlSyncJob(uid, one.ID, "approve", "wrong"); err == nil {
		t.Fatal("wrong hash approved")
	}
	if err := s.ControlSyncJob(uid, one.ID, "approve", hash); err != nil {
		t.Fatal(err)
	}
	if err := s.ControlSyncJob(uid, two.ID, "cancel", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.QueueSyncJob(uid, agent.ID, two.TargetID, "preview"); err == nil {
		t.Fatal("new preview queued while apply pending")
	}
	apply, err := s.ClaimSyncJob(uid, agent)
	if err != nil || apply == nil || apply.Kind != "apply" || apply.ID != one.ID {
		t.Fatal("approved plan not claimed", err)
	}
	if job, err := s.ClaimSyncJob(uid, agent); err != nil || job != nil {
		t.Fatal("parallel apply accepted", err)
	}
}

func TestDesiredSelectionDeduplicatesLocalCopies(t *testing.T) {
	s, uid, id := controlFixture(t)
	local, err := s.UpsertLocalFor(uid, "improve", Patch{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := s.UpsertFor(uid, mustRef(t, "owner/skills/improve"), Patch{})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", SkillIDs: []string{local.ID, linked.ID}, Mode: "additive"}); err != nil {
		t.Fatal(err)
	}
	targets, err := s.SyncTargetsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || len(targets[0].Desired) != 1 || targets[0].Desired[0].ID != linked.ID {
		t.Fatalf("local duplicate was not deduplicated: %+v", targets)
	}
	if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", SkillIDs: []string{local.ID, linked.ID}, Mode: "additive", Revision: targets[0].Revision}); err != nil {
		t.Fatalf("reviewed duplicate save failed: %v", err)
	}
	if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", SkillIDs: []string{local.ID}, Mode: "additive"}); err == nil {
		t.Fatal("local-only selection accepted")
	}
}

func TestSyncRunChainsScopesAndAutoApprovesAdditivePlans(t *testing.T) {
	s, uid, first := controlFixture(t)
	s.TouchEnvironmentFor(uid, "laptop", "claude", "", "")
	second := EnvironmentIDFor(uid, "laptop", "claude", "")
	s.RecordInventoryFor(uid, second, true, MachineInfo{})
	for _, id := range []string{first, second} {
		if err := s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", SkillIDs: []string{"a/b/one"}, Mode: "additive"}); err != nil {
			t.Fatal(err)
		}
	}
	agent, _, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.StartSyncRun(uid, agent.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Targets) != 2 || run.Status != "running" {
		t.Fatalf("bad run: %+v", run)
	}
	job, err := s.ClaimSyncJob(uid, agent)
	if err != nil || job == nil || job.Kind != "auto" || job.TargetID != first {
		t.Fatalf("first auto job: %+v %v", job, err)
	}
	hash := strings.Repeat("d", 64)
	plan := json.RawMessage(`{"actions":[{"add":true,"skill":{"slug":"one"}}]}`)
	if err = s.FinishSyncJob(uid, agent.ID, job.ID, "awaiting-approval", hash, "", plan); err != nil {
		t.Fatal(err)
	}
	apply, err := s.ClaimSyncJob(uid, agent)
	if err != nil || apply == nil || apply.Kind != "apply" || apply.ID != job.ID {
		t.Fatalf("additive plan was not auto-approved: %+v %v", apply, err)
	}
	if err = s.FinishSyncJob(uid, agent.ID, apply.ID, "succeeded", hash, "", plan); err != nil {
		t.Fatal(err)
	}
	next, err := s.ClaimSyncJob(uid, agent)
	if err != nil || next == nil || next.Kind != "auto" || next.TargetID != second {
		t.Fatalf("run did not chain to second scope: %+v %v", next, err)
	}
	removal := json.RawMessage(`{"actions":[{"add":false,"skill":{"slug":"extra"}}]}`)
	if err = s.FinishSyncJob(uid, agent.ID, next.ID, "awaiting-approval", hash, "", removal); err != nil {
		t.Fatal(err)
	}
	jobs, _ := s.SyncJobsFor(uid)
	for _, item := range jobs {
		if item.ID == next.ID && (item.Kind != "auto" || item.Status != "awaiting-approval" || !strings.Contains(item.Message, "review")) {
			t.Fatalf("removal was not held for review: %+v", item)
		}
	}
	runs, _ := s.SyncRunsFor(uid)
	if len(runs) == 0 || runs[0].Status != "review" || runs[0].Cursor != 2 {
		t.Fatalf("review run: %+v", runs)
	}
}

func TestFinishSyncJobDistinguishesRefusalsFromTransientErrors(t *testing.T) {
	s, uid, id := controlFixture(t)
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", SkillIDs: []string{"a/b/one"}, Mode: "additive"}); err != nil {
		t.Fatal(err)
	}
	job, err := s.QueueSyncJob(uid, a.ID, id, "preview")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimSyncJob(uid, a); err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("a", 64)
	if err = s.FinishSyncJob(uid, a.ID, job.ID, "succeeded", hash, "", json.RawMessage(`{"actions":[]}`)); err != nil {
		t.Fatal(err)
	}
	var refused SyncJobResultRefused
	if err = s.FinishSyncJob(uid, a.ID, job.ID, "failed", strings.Repeat("b", 64), "", json.RawMessage(`{}`)); !errors.As(err, &refused) {
		t.Fatalf("closed job refused without a terminal marker: %v", err)
	}
	if err = s.FinishSyncJob(uid, a.ID, "missing-job", "succeeded", "", "", nil); !errors.As(err, &refused) {
		t.Fatalf("missing job refused without a terminal marker: %v", err)
	}
}

func TestQueueAgentUpdateUsesExistingJobGate(t *testing.T) {
	s, uid, id := controlFixture(t)
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.QueueAgentUpdate(uid, a.ID, "latest")
	if err != nil || job.Kind != "update" || job.Revision != "latest" || job.TargetID != "" {
		t.Fatalf("update job: %+v %v", job, err)
	}
	if _, err = s.QueueSyncJob(uid, a.ID, id, "preview"); err == nil {
		t.Fatal("preview queued while update pending")
	}
	if _, err = s.QueueAgentUpdate(uid, a.ID, "not-a-tag"); err == nil {
		t.Fatal("invalid version accepted")
	}
	other, _ := s.CreateUser("other@update.invalid", "Other", "member", "")
	foreign, _, _ := s.EnrollSyncAgent(other.ID, "desktop", "v1.0.0")
	if _, err = s.QueueAgentUpdate(uid, foreign.ID, "latest"); err == nil {
		t.Fatal("cross-account update accepted")
	}
}

func TestQueueAgentUpdateWaitsBehindReviewedSkillWork(t *testing.T) {
	s, uid, id := controlFixture(t)
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", SkillIDs: []string{"a/b/one"}, Mode: "additive"}); err != nil {
		t.Fatal(err)
	}
	preview, err := s.QueueSyncJob(uid, a.ID, id, "preview")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.QueueAgentUpdate(uid, a.ID, "latest"); err != nil {
		t.Fatalf("queued preview blocked an update: %v", err)
	}
	if _, err = s.QueueAgentUpdate(uid, a.ID, "latest"); err == nil || !strings.Contains(err.Error(), "already queued") {
		t.Fatalf("duplicate update not identified: %v", err)
	}
	claim, err := s.ClaimSyncJob(uid, a)
	if err != nil || claim == nil || claim.ID != preview.ID {
		t.Fatalf("older queued work must keep its turn: %+v %v", claim, err)
	}
	hash := strings.Repeat("e", 64)
	if err = s.FinishSyncJob(uid, a.ID, preview.ID, "awaiting-approval", hash, "", json.RawMessage(`{"actions":[]}`)); err != nil {
		t.Fatal(err)
	}
	update, err := s.ClaimSyncJob(uid, a)
	if err != nil || update == nil || update.Kind != "update" {
		t.Fatalf("update was not claimed after review work: %+v %v", update, err)
	}
}

func TestQueueAgentUpdateStillRefusesRunningWork(t *testing.T) {
	s, uid, id := controlFixture(t)
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", SkillIDs: []string{"a/b/one"}, Mode: "additive"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.QueueSyncJob(uid, a.ID, id, "preview"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimSyncJob(uid, a); err != nil {
		t.Fatal(err)
	}
	if _, err = s.QueueAgentUpdate(uid, a.ID, "latest"); err == nil || !strings.Contains(err.Error(), "still running") {
		t.Fatalf("running operation did not block an update: %v", err)
	}
}

func TestQueueAgentUpdateIgnoresExpiredRunningWork(t *testing.T) {
	s, uid, id := controlFixture(t)
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", SkillIDs: []string{"a/b/one"}, Mode: "additive"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.QueueSyncJob(uid, a.ID, id, "preview"); err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimSyncJob(uid, a)
	if err != nil || job == nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`UPDATE sync_jobs SET updated_at=? WHERE id=?`, time.Now().Add(-11*time.Minute).UnixMilli(), job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.QueueAgentUpdate(uid, a.ID, "latest"); err != nil {
		t.Fatalf("expired running work blocked an update: %v", err)
	}
}

func TestSyncAgentBuildAndAutoUpdateSettings(t *testing.T) {
	s, uid, _ := controlFixture(t)
	a, secret, err := s.EnrollSyncAgentBuild(uid, "laptop", "dev", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	if a.Commit != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || !a.AutoUpdate {
		t.Fatalf("enrollment defaults: %+v", a)
	}
	reported, err := s.AuthenticateSyncAgentBuild(uid, a.ID, secret, "dev", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if err != nil {
		t.Fatal(err)
	}
	if reported.Commit != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("reported commit not stored: %+v", reported)
	}
	if reported, err = s.AuthenticateSyncAgentBuild(uid, a.ID, secret, "dev", ""); err != nil || reported.Commit != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("empty commit erased the last known build: %+v %v", reported, err)
	}
	if err = s.ControlSyncAgent(uid, a.ID, "auto-update-off"); err != nil {
		t.Fatal(err)
	}
	agents, _ := s.SyncAgentsFor(uid)
	if len(agents) != 1 || agents[0].AutoUpdate {
		t.Fatalf("auto-update did not turn off: %+v", agents)
	}
	if err = s.ControlSyncAgent(uid, a.ID, "auto-update-on"); err != nil {
		t.Fatal(err)
	}
	agents, _ = s.SyncAgentsFor(uid)
	if !agents[0].AutoUpdate {
		t.Fatalf("auto-update did not turn on: %+v", agents)
	}
}

func TestOpenMigratesAgentBuildAndAutoUpdateColumns(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "reet.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE sync_agents (id TEXT PRIMARY KEY, user_id INTEGER NOT NULL, host TEXT NOT NULL, secret_hash TEXT NOT NULL, version TEXT NOT NULL, last_seen INTEGER NOT NULL, paused INTEGER NOT NULL DEFAULT 0, UNIQUE(user_id,host))`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	uid, _ := s.DefaultOwner()
	a, _, err := s.EnrollSyncAgentBuild(uid, "laptop", "dev", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil || a.Commit == "" || !a.AutoUpdate {
		t.Fatalf("migrated enrollment: %+v %v", a, err)
	}
}

func TestSyncRunReplansStalePlanAndKeepsGoing(t *testing.T) {
	s, uid, first := controlFixture(t)
	s.TouchEnvironmentFor(uid, "laptop", "claude", "", "")
	second := EnvironmentIDFor(uid, "laptop", "claude", "")
	s.RecordInventoryFor(uid, second, true, MachineInfo{})
	for _, id := range []string{first, second} {
		if err := s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", SkillIDs: []string{"a/b/one"}, Mode: "additive"}); err != nil {
			t.Fatal(err)
		}
	}
	agent, _, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartSyncRun(uid, agent.ID, nil); err != nil {
		t.Fatal(err)
	}
	stale, err := s.ClaimSyncJob(uid, agent)
	if err != nil || stale == nil || stale.Kind != "auto" || stale.TargetID != first {
		t.Fatalf("first auto job: %+v %v", stale, err)
	}
	// The machine changed underneath the approved plan. It reports "replan"
	// before writing anything, and the controller must repair the same scope.
	if err = s.FinishSyncJob(uid, agent.ID, stale.ID, "replan", "", "local skills changed; preview again", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	replacement, err := s.ClaimSyncJob(uid, agent)
	if err != nil || replacement == nil || replacement.Kind != "auto" || replacement.TargetID != first {
		t.Fatalf("scope was not re-planned: %+v %v", replacement, err)
	}
	hash := strings.Repeat("d", 64)
	plan := json.RawMessage(`{"actions":[{"add":true,"skill":{"slug":"one"}}]}`)
	if err = s.FinishSyncJob(uid, agent.ID, replacement.ID, "awaiting-approval", hash, "", plan); err != nil {
		t.Fatal(err)
	}
	apply, err := s.ClaimSyncJob(uid, agent)
	if err != nil || apply == nil || apply.Kind != "apply" || apply.ID != replacement.ID {
		t.Fatalf("replanned additive job was not approved: %+v %v", apply, err)
	}
	if err = s.FinishSyncJob(uid, agent.ID, apply.ID, "succeeded", hash, "", plan); err != nil {
		t.Fatal(err)
	}
	next, err := s.ClaimSyncJob(uid, agent)
	if err != nil || next == nil || next.Kind != "auto" || next.TargetID != second {
		t.Fatalf("run did not continue after a re-plan: %+v %v", next, err)
	}
	runs, _ := s.SyncRunsFor(uid)
	if len(runs) == 0 || runs[0].Status != "running" || runs[0].Cursor != 1 {
		t.Fatalf("run state after re-plan: %+v", runs)
	}
	jobs, _ := s.SyncJobsFor(uid)
	seenReplan := false
	for _, job := range jobs {
		if job.ID == stale.ID {
			seenReplan = job.Status == "failed" && strings.Contains(job.Message, "fresh plan")
		}
	}
	if !seenReplan {
		t.Fatalf("stale attempt was not recorded honestly: %+v", jobs)
	}
}

func TestSyncRunStopsAfterReplanLimit(t *testing.T) {
	s, uid, first := controlFixture(t)
	if err := s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: first, SourceID: "shelf", SkillIDs: []string{"a/b/one"}, Mode: "additive"}); err != nil {
		t.Fatal(err)
	}
	agent, _, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartSyncRun(uid, agent.ID, nil); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= syncRunReplanLimit; attempt++ {
		job, err := s.ClaimSyncJob(uid, agent)
		if err != nil || job == nil {
			t.Fatalf("attempt %d: %+v %v", attempt, job, err)
		}
		if err = s.FinishSyncJob(uid, agent.ID, job.ID, "replan", "", "local skills changed; preview again", json.RawMessage(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	if job, err := s.ClaimSyncJob(uid, agent); err != nil || job != nil {
		t.Fatalf("a fourth attempt was queued: %+v %v", job, err)
	}
	runs, _ := s.SyncRunsFor(uid)
	if len(runs) == 0 || runs[0].Status != "failed" {
		t.Fatalf("run did not stop at the limit: %+v", runs)
	}
}
