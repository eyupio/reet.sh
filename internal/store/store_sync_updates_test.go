package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func updateFixture(t *testing.T) (*Store, int64, SyncAgent, SyncJob) {
	t.Helper()
	s, uid, _ := controlFixture(t)
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.QueueAgentUpdate(uid, a.ID, "latest")
	if err != nil {
		t.Fatal(err)
	}
	return s, uid, a, job
}

func TestAnUpdateJobCarriesItsTimeline(t *testing.T) {
	s, uid, a, job := updateFixture(t)
	if job.Stage != UpdateStageQueued || len(job.Stages) != len(UpdateStageKeys) {
		t.Fatalf("queued timeline: %+v", job)
	}
	if job.Stages[0].Status != "running" || job.Stages[1].Status != "pending" {
		t.Fatalf("queued stage states: %+v", job.Stages)
	}
	claimed, err := s.ClaimSyncJob(uid, a)
	if err != nil || claimed == nil {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	if claimed.Stage != UpdateStageDownload || claimed.Stages[0].Status != "done" || claimed.Stages[1].Status != "running" {
		t.Fatalf("claimed timeline: %+v", claimed)
	}
	if err = s.SetAgentUpdateStage(uid, a.ID, job.ID, UpdateStageVerify, "Checking the downloaded build"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetAgentUpdateStage(uid, a.ID, job.ID, UpdateStageRestart, "Restarting into v1.3.0"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetAgentUpdateStage(uid, a.ID, job.ID, UpdateStageVerify, "too late"); err == nil {
		t.Fatal("a stage moved backwards")
	}
	if err = s.SetAgentUpdateStage(uid, a.ID, job.ID, "invented", ""); err == nil {
		t.Fatal("an unknown stage was accepted")
	}
	if err = s.FinishSyncJob(uid, a.ID, job.ID, "succeeded", "", "Updated from v1.2.0 to v1.3.0.", nil); err != nil {
		t.Fatal(err)
	}
	jobs, _ := s.SyncJobsFor(uid)
	if len(jobs) != 1 || jobs[0].Status != "succeeded" || jobs[0].Stage != UpdateStageCheck {
		t.Fatalf("finished update: %+v", jobs)
	}
	for _, stage := range jobs[0].Stages {
		if stage.Status != "done" {
			t.Fatalf("stage left unfinished: %+v", jobs[0].Stages)
		}
	}
	if jobs[0].Stages[stageIndex(jobs[0].Stages, UpdateStageRestart)].Detail != "Restarting into v1.3.0" {
		t.Fatalf("detail lost: %+v", jobs[0].Stages)
	}
}

func TestAFailedUpdateStopsAtTheStageAndARollbackSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status string
		last   string
	}{
		{"failed", "failed", UpdateStageCheck},
		{"rolled back", "rolled_back", UpdateStageRollback},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, uid, a, job := updateFixture(t)
			if _, err := s.ClaimSyncJob(uid, a); err != nil {
				t.Fatal(err)
			}
			if err := s.SetAgentUpdateStage(uid, a.ID, job.ID, UpdateStageCheck, "Waiting for the new agent"); err != nil {
				t.Fatal(err)
			}
			if err := s.FinishSyncJob(uid, a.ID, job.ID, tc.status, "", "The new build did not report in.", nil); err != nil {
				t.Fatal(err)
			}
			jobs, _ := s.SyncJobsFor(uid)
			got := jobs[0]
			if got.Status != tc.status || got.Stage != tc.last {
				t.Fatalf("outcome: %+v", got)
			}
			if got.Stages[stageIndex(got.Stages, UpdateStageCheck)].Status != "failed" {
				t.Fatalf("check not failed: %+v", got.Stages)
			}
			if tc.status == "rolled_back" {
				if stageIndex(got.Stages, UpdateStageRollback) < 0 || got.Stages[len(got.Stages)-1].Status != "done" {
					t.Fatalf("rollback missing: %+v", got.Stages)
				}
			}
		})
	}
}

func TestAutoUpdateWaitsAfterAnAttemptSettlesButNotWhileOneIsQueued(t *testing.T) {
	s, uid, a, job := updateFixture(t)
	recent := time.Now().Add(-time.Hour).UnixMilli()
	if settled, err := s.AgentUpdateSettledSince(uid, a.ID, recent); err != nil || settled {
		t.Fatalf("a queued attempt counted as settled: %t %v", settled, err)
	}
	if _, err := s.ClaimSyncJob(uid, a); err != nil {
		t.Fatal(err)
	}
	if settled, err := s.AgentUpdateSettledSince(uid, a.ID, recent); err != nil || settled {
		t.Fatalf("a running attempt counted as settled: %t %v", settled, err)
	}
	if err := s.FinishSyncJob(uid, a.ID, job.ID, "succeeded", "", "Updated.", nil); err != nil {
		t.Fatal(err)
	}
	if settled, err := s.AgentUpdateSettledSince(uid, a.ID, recent); err != nil || !settled {
		t.Fatalf("a finished attempt did not count as settled: %t %v", settled, err)
	}
	if settled, err := s.AgentUpdateSettledSince(uid, a.ID, time.Now().Add(time.Minute).UnixMilli()); err != nil || settled {
		t.Fatalf("a future window reported an attempt: %t %v", settled, err)
	}
	job, err := s.QueueAgentUpdate(uid, a.ID, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimSyncJob(uid, a); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishSyncJob(uid, a.ID, job.ID, "rolled_back", "", "Restored the previous binary.", nil); err != nil {
		t.Fatal(err)
	}
	if settled, err := s.AgentUpdateSettledSince(uid, a.ID, time.Now().Add(-time.Minute).UnixMilli()); err != nil || !settled {
		t.Fatalf("a rollback did not count as settled: %t %v", settled, err)
	}
}

func TestAnUpdateStageNeedsItsOwnRunningUpdate(t *testing.T) {
	s, uid, a, job := updateFixture(t)
	if err := s.SetAgentUpdateStage(uid, a.ID, job.ID, UpdateStageDownload, ""); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("stage written before the machine collected the job: %v", err)
	}
	if _, err := s.ClaimSyncJob(uid, a); err != nil {
		t.Fatal(err)
	}
	other, _ := s.CreateUser("other@updates.invalid", "Other", "member", "")
	foreign, _, _ := s.EnrollSyncAgent(other.ID, "desktop", "v1.2.0")
	if err := s.SetAgentUpdateStage(other.ID, foreign.ID, job.ID, UpdateStageVerify, ""); err == nil {
		t.Fatal("a foreign agent wrote a stage")
	}
}

func TestOpenAddsTheUpdateTimelineToExistingJobs(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "reet.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE sync_agents (id TEXT PRIMARY KEY, user_id INTEGER NOT NULL, host TEXT NOT NULL, secret_hash TEXT NOT NULL, version TEXT NOT NULL, source_commit TEXT NOT NULL DEFAULT '', last_seen INTEGER NOT NULL, paused INTEGER NOT NULL DEFAULT 0, auto_update INTEGER NOT NULL DEFAULT 1, UNIQUE(user_id,host))`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE sync_jobs (id TEXT PRIMARY KEY, user_id INTEGER NOT NULL, agent_id TEXT NOT NULL, target_id TEXT NOT NULL DEFAULT '', kind TEXT NOT NULL, status TEXT NOT NULL, revision TEXT NOT NULL DEFAULT '', plan TEXT NOT NULL DEFAULT '{}', hash TEXT NOT NULL DEFAULT '', message TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`)
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
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.QueueAgentUpdate(uid, a.ID, "latest")
	if err != nil || job.Stage != UpdateStageQueued || len(job.Stages) == 0 {
		t.Fatalf("update on a migrated database: %+v %v", job, err)
	}
	jobs, err := s.SyncJobsFor(uid)
	if err != nil || len(jobs) != 1 || jobs[0].Stages[0].Key != UpdateStageQueued {
		t.Fatalf("read back: %+v %v", jobs, err)
	}
}

func TestALateUpdateReportReplacesTheTimeoutItOutlived(t *testing.T) {
	s, uid, a, job := updateFixture(t)
	if _, err := s.ClaimSyncJob(uid, a); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAgentUpdateStage(uid, a.ID, job.ID, UpdateStageRestart, "Restarting into v1.3.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE sync_jobs SET status='failed',message='The machine did not report back. It may still be restarting; check its build and try again.',updated_at=? WHERE id=?`, time.Now().Add(-11*time.Minute).UnixMilli(), job.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSyncJob(uid, a.ID, job.ID, "succeeded", "", "Updated from v1.2.0 to v1.3.0. The agent is reporting again.", nil); err != nil {
		t.Fatal(err)
	}
	jobs, _ := s.SyncJobsFor(uid)
	if len(jobs) != 1 || jobs[0].Status != "succeeded" || jobs[0].Stage != UpdateStageCheck {
		t.Fatalf("late report: %+v", jobs)
	}
	for _, stage := range jobs[0].Stages {
		if stage.Status != "done" {
			t.Fatalf("late report left a stage: %+v", jobs[0].Stages)
		}
	}
}

func TestTheLatestUpdateIsKeptForEachMachineWhenJobHistoryIsBusy(t *testing.T) {
	s, uid, a, first := updateFixture(t)
	if _, err := s.ClaimSyncJob(uid, a); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSyncJob(uid, a.ID, first.ID, "failed", "", "Download failed.", nil); err != nil {
		t.Fatal(err)
	}
	second, err := s.QueueAgentUpdate(uid, a.ID, "dev")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := s.EnrollSyncAgent(uid, "desktop", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	third, err := s.QueueAgentUpdate(uid, other.ID, "latest")
	if err != nil {
		t.Fatal(err)
	}
	latest, err := s.LatestUpdateJobsFor(uid)
	if err != nil || len(latest) != 2 {
		t.Fatalf("latest updates: %+v %v", latest, err)
	}
	byAgent := map[string]string{}
	for _, job := range latest {
		byAgent[job.AgentID] = job.ID
	}
	if byAgent[a.ID] != second.ID || byAgent[other.ID] != third.ID {
		t.Fatalf("wrong jobs: %+v", byAgent)
	}
}
