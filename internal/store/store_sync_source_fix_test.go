package store

import (
	"strings"
	"testing"
)

func sourceFixFixture(t *testing.T) (*Store, int64, string) {
	t.Helper()
	s := newStore(t)
	uid, err := s.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.EnrollSyncAgent(uid, "laptop", "test"); err != nil {
		t.Fatal(err)
	}
	if err = s.TouchEnvironmentFor(uid, "laptop", "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	target := EnvironmentIDFor(uid, "laptop", "agents", "")
	if err = s.RecordInventoryFor(uid, target, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	if err = s.TouchEnvironmentFor(uid, "laptop", "claude", "", ""); err != nil {
		t.Fatal(err)
	}
	claude := EnvironmentIDFor(uid, "laptop", "claude", "")
	if err = s.RecordInventoryFor(uid, claude, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	installed, err := s.UpsertFor(uid, mustRef(t, "x/y/one"), Patch{})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecordEnvironmentFor(uid, installed.ID, "laptop", "claude", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpsertFor(uid, mustRef(t, "a/b/one"), Patch{}); err != nil {
		t.Fatal(err)
	}
	candidates, err := s.SyncCandidatesFor(uid, "owner", 0)
	if err != nil {
		t.Fatal(err)
	}
	want := ""
	for _, candidate := range candidates {
		if candidate.ID == "a/b/one" {
			want = candidate.ID
		}
	}
	if want == "" {
		t.Fatal("desired candidate missing", candidates)
	}
	if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: target, SourceID: "shelf", SkillIDs: []string{want}, Mode: "additive"}); err != nil {
		t.Fatal(err)
	}
	return s, uid, target
}

func TestResolveSyncSourceConflictFollowsInstalledCopy(t *testing.T) {
	s, uid, target := sourceFixFixture(t)
	before, err := s.SyncTargetsFor(uid)
	if err != nil || len(before) != 1 || before[0].Desired[0].Source != "a/b" {
		t.Fatalf("fixture desired: %+v %v", before, err)
	}
	preview, err := s.ResolveSyncSourceConflictsFor(uid, []string{target}, true)
	if err != nil || len(preview) != 1 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if preview[0].From != "a/b" || preview[0].To != "x/y" || preview[0].Agent != "claude" {
		t.Fatalf("preview change: %+v", preview[0])
	}
	still, _ := s.SyncTargetsFor(uid)
	if still[0].Desired[0].Source != "a/b" {
		t.Fatal("a dry run changed the saved plan")
	}
	applied, err := s.ResolveSyncSourceConflictsFor(uid, []string{target}, false)
	if err != nil || len(applied) != 1 {
		t.Fatalf("apply: %+v %v", applied, err)
	}
	after, _ := s.SyncTargetsFor(uid)
	if after[0].Desired[0].Source != "x/y" || after[0].Revision == before[0].Revision {
		t.Fatalf("plan not updated: %+v", after[0])
	}
}

func TestResolveSyncSourceConflictRefusesAmbiguousMachine(t *testing.T) {
	s, uid, target := sourceFixFixture(t)
	if err := s.TouchEnvironmentFor(uid, "laptop", "cursor", "", ""); err != nil {
		t.Fatal(err)
	}
	cursor := EnvironmentIDFor(uid, "laptop", "cursor", "")
	if err := s.RecordInventoryFor(uid, cursor, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	other, err := s.UpsertFor(uid, mustRef(t, "z/w/one"), Patch{})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecordEnvironmentFor(uid, other.ID, "laptop", "cursor", "", "", ""); err != nil {
		t.Fatal(err)
	}
	_, err = s.ResolveSyncSourceConflictsFor(uid, []string{target}, false)
	if err == nil || !strings.Contains(err.Error(), "more than one source") {
		t.Fatalf("ambiguous machine accepted: %v", err)
	}
	after, _ := s.SyncTargetsFor(uid)
	if after[0].Desired[0].Source != "a/b" {
		t.Fatal("a refused fix changed the saved plan")
	}
}

func TestMaintainSyncFollowsUnambiguousInstalledSourceAndReportsIt(t *testing.T) {
	s, uid, target := sourceFixFixture(t)
	agents, err := s.SyncAgentsFor(uid)
	if err != nil || len(agents) != 1 {
		t.Fatalf("agents: %+v %v", agents, err)
	}
	agent := agents[0]
	if err = s.ControlSyncAgent(uid, agent.ID, "keep-sync-on"); err != nil {
		t.Fatal(err)
	}
	agents, err = s.SyncAgentsFor(uid)
	if err != nil || len(agents) != 1 || !agents[0].KeepSynced {
		t.Fatalf("keep synced: %+v %v", agents, err)
	}
	agent = agents[0]
	job, err := s.QueueSyncJob(uid, agent.ID, target, "auto")
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := s.ClaimSyncJob(uid, agent)
	if err != nil || claimed == nil || claimed.ID != job.ID {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	plan := []byte(`{"problems":{"one":"Claude Code already has this skill from x/y, so reet left it alone."}}`)
	if err = s.FinishSyncJob(uid, agent.ID, job.ID, "awaiting-approval", strings.Repeat("a", 64), "", plan); err != nil {
		t.Fatal(err)
	}
	if err = s.MaintainSync(uid, "owner", agent); err != nil {
		t.Fatal(err)
	}
	after, err := s.SyncTargetsFor(uid)
	if err != nil || len(after) != 1 || after[0].Desired[0].Source != "x/y" {
		t.Fatalf("source not followed: %+v %v", after, err)
	}
	next, err := s.ClaimSyncJob(uid, agent)
	if err != nil || next == nil || next.Kind != "auto" || !strings.Contains(next.Message, "Auto-fixed") {
		t.Fatalf("repair job: %+v %v", next, err)
	}
	again, err := s.AutoResolveSyncSourceConflictsFor(uid, target)
	if err != nil || len(again) != 0 {
		t.Fatalf("the repair was not idempotent: %+v %v", again, err)
	}
}

func TestMaintainSyncLeavesAmbiguousSourceConflictReported(t *testing.T) {
	s, uid, target := sourceFixFixture(t)
	if err := s.TouchEnvironmentFor(uid, "laptop", "cursor", "", ""); err != nil {
		t.Fatal(err)
	}
	cursor := EnvironmentIDFor(uid, "laptop", "cursor", "")
	if err := s.RecordInventoryFor(uid, cursor, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	other, err := s.UpsertFor(uid, mustRef(t, "z/w/one"), Patch{})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecordEnvironmentFor(uid, other.ID, "laptop", "cursor", "", "", ""); err != nil {
		t.Fatal(err)
	}
	agents, err := s.SyncAgentsFor(uid)
	if err != nil || len(agents) != 1 {
		t.Fatalf("agents: %+v %v", agents, err)
	}
	agent := agents[0]
	if err = s.ControlSyncAgent(uid, agent.ID, "keep-sync-on"); err != nil {
		t.Fatal(err)
	}
	agents, err = s.SyncAgentsFor(uid)
	if err != nil || len(agents) != 1 || !agents[0].KeepSynced {
		t.Fatalf("keep synced: %+v %v", agents, err)
	}
	agent = agents[0]
	job, err := s.QueueSyncJob(uid, agent.ID, target, "auto")
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := s.ClaimSyncJob(uid, agent)
	if err != nil || claimed == nil || claimed.ID != job.ID {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	plan := []byte(`{"problems":{"one":"Claude Code already has this skill from x/y, so reet left it alone."}}`)
	if err = s.FinishSyncJob(uid, agent.ID, job.ID, "awaiting-approval", strings.Repeat("b", 64), "", plan); err != nil {
		t.Fatal(err)
	}
	if err = s.MaintainSync(uid, "owner", agent); err != nil {
		t.Fatal(err)
	}
	after, err := s.SyncTargetsFor(uid)
	if err != nil || len(after) != 1 || after[0].Desired[0].Source != "a/b" {
		t.Fatalf("ambiguous conflict changed the plan: %+v %v", after, err)
	}
	if !strings.Contains(after[0].Problem, "more than one source") || !strings.Contains(after[0].Problem, "Auto fix could not settle") {
		t.Fatalf("ambiguous conflict was not reported: %+v", after[0])
	}
	jobs, _ := s.SyncJobsFor(uid)
	if len(jobs) != 1 {
		t.Fatalf("an ambiguous conflict queued another job: %+v", jobs)
	}
	if err = s.MaintainSync(uid, "owner", agent); err != nil {
		t.Fatal(err)
	}
	jobs, _ = s.SyncJobsFor(uid)
	if len(jobs) != 1 {
		t.Fatalf("an ambiguous conflict queued repeated jobs: %+v", jobs)
	}
}
