package store

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNoChangePreviewReplansWhenLibraryRevisionChanged(t *testing.T) {
	s, uid, id := controlFixture(t)
	if err := s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", Mode: "additive", SkillIDs: []string{"a/b/one"}}); err != nil {
		t.Fatal(err)
	}
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartSyncRun(uid, a.ID, []string{id}); err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimSyncJob(uid, a)
	if err != nil || job == nil {
		t.Fatal(err)
	}
	if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", Mode: "additive", SkillIDs: []string{"a/b/one"}, Revision: job.Revision}); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishSyncJob(uid, a.ID, job.ID, "awaiting-approval", strings.Repeat("a", 64), "", json.RawMessage(`{"actions":[]}`)); err != nil {
		t.Fatal(err)
	}
	jobs, _ := s.SyncJobsFor(uid)
	if len(jobs) != 2 {
		t.Fatalf("fresh plan not queued: %+v", jobs)
	}
	for _, next := range jobs {
		if next.ID != job.ID && (next.Status != "queued" || next.Revision == job.Revision || next.Kind != "auto") {
			t.Fatalf("wrong recovery: %+v", next)
		}
	}
}

func TestSyncLibraryAllMachinesPersistsPolicyAndKeepsProjectsExplicit(t *testing.T) {
	s, uid, _ := controlFixture(t)
	for _, host := range []string{"laptop", "desktop"} {
		for _, scope := range []struct{ agent, project string }{{"agents", ""}, {"claude", ""}, {"claude", "/work/project"}} {
			if err := s.TouchEnvironmentFor(uid, host, scope.agent, scope.project, ""); err != nil {
				t.Fatal(err)
			}
			if err := s.RecordInventoryFor(uid, EnvironmentIDFor(uid, host, scope.agent, scope.project), true, MachineInfo{}); err != nil {
				t.Fatal(err)
			}
		}
		if _, _, err := s.EnrollSyncAgent(uid, host, "test"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SyncLibraryMachinesFor(uid, "owner", "https://reet.example"); err != nil {
		t.Fatal(err)
	}
	targets, err := s.SyncTargetsFor(uid)
	if err != nil || len(targets) != 4 {
		t.Fatalf("targets: %+v %v", targets, err)
	}
	for _, target := range targets {
		if target.SourceID != "library" || !target.FollowSource || target.Mode != "additive" || target.Target.Project != "" {
			t.Fatalf("wrong library policy: %+v", target)
		}
	}
	agents, _ := s.SyncAgentsFor(uid)
	jobs, _ := s.SyncJobsFor(uid)
	for _, agent := range agents {
		if !agent.KeepSynced {
			t.Fatal("sync not persisted", agent)
		}
		if err := s.MaintainSync(uid, "owner", agent); err != nil {
			t.Fatal(err)
		}
	}
	after, _ := s.SyncJobsFor(uid)
	if len(after) != len(jobs) {
		t.Fatal("duplicate jobs while pending")
	}
}

func TestLibrarySourceConflictPreservesLastGoodSelection(t *testing.T) {
	s, uid, id := controlFixture(t)
	if err := s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", Mode: "additive", SkillIDs: []string{"a/b/one"}}); err != nil {
		t.Fatal(err)
	}
	s.EnrollSyncAgent(uid, "laptop", "test")
	s.UpsertFor(uid, mustRef(t, "c/d/one"), Patch{})
	if err := s.SyncLibraryMachinesFor(uid, "owner", "https://reet.example"); err == nil {
		t.Fatal("ambiguous source accepted")
	}
	targets, _ := s.SyncTargetsFor(uid)
	if len(targets) != 1 || len(targets[0].Desired) != 1 || targets[0].Desired[0].ID != "a/b/one" {
		t.Fatalf("last good selection changed: %+v", targets)
	}
}
