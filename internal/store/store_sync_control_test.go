package store

import (
	"encoding/json"
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

func TestPublishedSyncCandidatesExcludeDrafts(t *testing.T) {
	s, uid, id := controlFixture(t)
	if err := s.ClaimPublishingUsername(uid, "publisher"); err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"published", "draft"} {
		_, err := s.SaveHostedSkill(uid, HostedSkill{Slug: slug, Description: "Fixture skill", Content: "---\nname: " + slug + "\ndescription: Fixture skill\n---\n\nTest", Published: slug == "published"})
		if err != nil {
			t.Fatal(err)
		}
	}
	candidates, err := s.SyncHostedFor(uid, "https://reet.sh")
	if err != nil || len(candidates) != 1 || candidates[0].ID != "hosted/reet.sh/publisher/published" {
		t.Fatalf("%+v %v", candidates, err)
	}
	if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "hosted", Origin: "https://reet.sh", SkillIDs: []string{candidates[0].ID}, Mode: "additive"}); err != nil {
		t.Fatal(err)
	}
	other, _ := s.CreateUser("other@hosted.invalid", "Other", "member", "")
	if candidates, err = s.SyncHostedFor(other.ID, "https://reet.sh"); err != nil || len(candidates) != 0 {
		t.Fatal("another user's hosted source leaked")
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
