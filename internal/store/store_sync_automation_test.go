package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestMachineKeepSyncedPersistsAndRunsWithoutBrowser(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	uid, _ := s.DefaultOwner()
	s.TouchEnvironmentFor(uid, "laptop", "agents", "", "")
	id := EnvironmentIDFor(uid, "laptop", "agents", "")
	s.RecordInventoryFor(uid, id, true, MachineInfo{})
	s.UpsertFor(uid, mustRef(t, "a/b/one"), Patch{})
	if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", Mode: "additive", FollowSource: true}); err != nil {
		t.Fatal(err)
	}
	a, secret, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MaintainSync(uid, "owner", a); err != nil {
		t.Fatal(err)
	}
	jobs, _ := s.SyncJobsFor(uid)
	if len(jobs) != 0 {
		t.Fatal("sync ran before opt-in")
	}
	if err = s.ControlSyncAgent(uid, a.ID, "keep-sync-on"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a, err = s.AuthenticateSyncAgent(uid, a.ID, secret, "test")
	if err != nil || !a.KeepSynced {
		t.Fatal("setting lost", err)
	}
	s.UpsertFor(uid, mustRef(t, "a/b/two"), Patch{})
	if err = s.MaintainSync(uid, "owner", a); err != nil {
		t.Fatal(err)
	}
	targets, _ := s.SyncTargetsFor(uid)
	if len(targets[0].Desired) != 2 {
		t.Fatalf("new shelf skill not followed: %+v", targets)
	}
	jobs, _ = s.SyncJobsFor(uid)
	if len(jobs) != 1 || jobs[0].Kind != "auto" {
		t.Fatalf("%+v", jobs)
	}
	if err = s.MaintainSync(uid, "owner", a); err != nil {
		t.Fatal(err)
	}
	jobs, _ = s.SyncJobsFor(uid)
	if len(jobs) != 1 {
		t.Fatal("duplicate sync queued")
	}
	job, err := s.ClaimSyncJob(uid, a)
	if err != nil || job == nil {
		t.Fatal(err)
	}
	if err = s.FinishSyncJob(uid, a.ID, job.ID, "failed", "", "permission denied", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err = s.MaintainSync(uid, "owner", a); err != nil {
		t.Fatal(err)
	}
	jobs, _ = s.SyncJobsFor(uid)
	if len(jobs) != 1 {
		t.Fatal("failed job retried indefinitely")
	}
	if err = s.ControlSyncAgent(uid, a.ID, "keep-sync-off"); err != nil {
		t.Fatal(err)
	}
	a, _ = s.AuthenticateSyncAgent(uid, a.ID, secret, "test")
	if a.KeepSynced {
		t.Fatal("toggle did not turn off")
	}
}

func TestContinuousSyncDoesNotQueueAlreadyMatchingSet(t *testing.T) {
	s, uid, id := controlFixture(t)
	s.RecordEnvironmentFor(uid, "a/b/one", "laptop", "agents", "", "", "")
	if err := s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", Mode: "additive", SkillIDs: []string{"a/b/one"}}); err != nil {
		t.Fatal(err)
	}
	a, _, _ := s.EnrollSyncAgent(uid, "laptop", "test")
	a.KeepSynced = true
	for range 3 {
		if err := s.MaintainSync(uid, "owner", a); err != nil {
			t.Fatal(err)
		}
	}
	jobs, _ := s.SyncJobsFor(uid)
	if len(jobs) != 0 {
		t.Fatalf("matching set looped: %+v", jobs)
	}
}

func TestSyncProgressIsOwnedTypedAndMonotonic(t *testing.T) {
	s, uid, id := controlFixture(t)
	s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", Mode: "additive", SkillIDs: []string{"a/b/one"}})
	a, _, _ := s.EnrollSyncAgent(uid, "laptop", "test")
	j, _ := s.QueueSyncJob(uid, a.ID, id, "auto")
	s.ClaimSyncJob(uid, a)
	if err := s.SetSyncStage(uid, "other", j.ID, "read", ""); err == nil {
		t.Fatal("other agent wrote progress")
	}
	if err := s.SetSyncStage(uid, a.ID, j.ID, "shell", ""); err == nil {
		t.Fatal("unknown stage accepted")
	}
	if err := s.SetSyncStage(uid, a.ID, j.ID, "plan", "Planning"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSyncStage(uid, a.ID, j.ID, "read", ""); err == nil {
		t.Fatal("stage rewound")
	}
	if err := s.FinishSyncJob(uid, a.ID, j.ID, "awaiting-approval", strings.Repeat("a", 64), "", json.RawMessage(`{"actions":[]}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSyncStage(uid, a.ID, j.ID, "apply", ""); err == nil {
		t.Fatal("finished job wrote progress")
	}
}

func TestExplicitAddCanSupplementMachineSourceWithoutChangingSource(t *testing.T) {
	s, uid, id := controlFixture(t)
	s.TouchEnvironmentFor(uid, "reference", "agents", "", "")
	source := EnvironmentIDFor(uid, "reference", "agents", "")
	s.RecordInventoryFor(uid, source, true, MachineInfo{})
	s.RecordEnvironmentFor(uid, "a/b/one", "reference", "agents", "", "", "")
	s.UpsertFor(uid, mustRef(t, "a/b/two"), Patch{})
	in := DesiredSelection{TargetID: id, SourceID: source, SkillIDs: []string{"a/b/one", "a/b/two"}, Mode: "additive"}
	if err := s.SaveDesiredFor(uid, "owner", in); err != nil {
		t.Fatal(err)
	}
	targets, _ := s.SyncTargetsFor(uid)
	if len(targets[0].Desired) != 2 || targets[0].SourceID != source {
		t.Fatalf("%+v", targets)
	}
	in.Revision = targets[0].Revision
	in.SkillIDs = append(in.SkillIDs, "not/owned/private")
	if err := s.SaveDesiredFor(uid, "owner", in); err == nil {
		t.Fatal("unowned addition accepted")
	}
}

func TestRunFinishesAfterDelayedApproval(t *testing.T) {
	s, uid, id := controlFixture(t)
	s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", SkillIDs: []string{"a/b/one"}, Mode: "mirror"})
	a, _, _ := s.EnrollSyncAgent(uid, "laptop", "test")
	if _, err := s.StartSyncRun(uid, a.ID, []string{id}); err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimSyncJob(uid, a)
	if err != nil || job == nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("b", 64)
	if err = s.FinishSyncJob(uid, a.ID, job.ID, "awaiting-approval", hash, "", json.RawMessage(`{"actions":[{"add":false,"skill":{"slug":"extra"}}]}`)); err != nil {
		t.Fatal(err)
	}
	runs, _ := s.SyncRunsFor(uid)
	if runs[0].Status != "review" {
		t.Fatalf("%+v", runs)
	}
	if err = s.ControlSyncJob(uid, job.ID, "approve", hash); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimSyncJob(uid, a); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishSyncJob(uid, a.ID, job.ID, "succeeded", hash, "", json.RawMessage(`{"actions":[]}`)); err != nil {
		t.Fatal(err)
	}
	runs, _ = s.SyncRunsFor(uid)
	if runs[0].Status != "completed" || runs[0].Cursor != 1 {
		t.Fatalf("review never settled: %+v", runs)
	}
}

func TestFollowedSourceProblemPreservesLastGoodSelection(t *testing.T) {
	s, uid, id := controlFixture(t)
	if err := s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", Mode: "additive", FollowSource: true}); err != nil {
		t.Fatal(err)
	}
	a, _, _ := s.EnrollSyncAgent(uid, "laptop", "test")
	a.KeepSynced = true
	if _, err := s.UpsertLocalFor(uid, "local-only", Patch{}); err != nil {
		t.Fatal(err)
	}
	if err := s.MaintainSync(uid, "owner", a); err != nil {
		t.Fatal(err)
	}
	targets, _ := s.SyncTargetsFor(uid)
	if len(targets[0].Desired) != 1 || targets[0].Problem == "" {
		t.Fatalf("lost good selection or hid source failure: %+v", targets)
	}
	jobs, _ := s.SyncJobsFor(uid)
	if len(jobs) != 0 {
		t.Fatal("queued work with unresolved sources")
	}
}

func TestAutomationFindsPendingWorkOutsideVisibleHistory(t *testing.T) {
	s, uid, id := controlFixture(t)
	s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", Mode: "additive", SkillIDs: []string{"a/b/one"}})
	a, _, _ := s.EnrollSyncAgent(uid, "laptop", "test")
	a.KeepSynced = true
	j, _ := s.QueueSyncJob(uid, a.ID, id, "auto")
	s.db.Exec(`UPDATE sync_jobs SET created_at=0 WHERE id=?`, j.ID)
	// A busy account can push an old pending job out of the bounded activity read.
	for range 510 {
		if _, err := s.db.Exec(`INSERT INTO sync_jobs(id,user_id,agent_id,target_id,kind,status,revision,created_at,updated_at) VALUES(?,?,?,'','scan','succeeded','',1,1)`, syncID(), uid, a.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MaintainSync(uid, "owner", a); err != nil {
		t.Fatal(err)
	}
	var pending int
	s.db.QueryRow(`SELECT COUNT(*) FROM sync_jobs WHERE agent_id=? AND status='queued'`, a.ID).Scan(&pending)
	if pending != 1 {
		t.Fatalf("duplicate work from truncated history: %d", pending)
	}
}
