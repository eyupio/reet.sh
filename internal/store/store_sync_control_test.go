package store

import (
	"database/sql"
	"errors"
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

func TestFinishSyncJobDistinguishesRefusalsFromTransientErrors(t *testing.T) {
	s, uid, _ := controlFixture(t)
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.QueueAgentUpdate(uid, a.ID, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimSyncJob(uid, a); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishSyncJob(uid, a.ID, job.ID, "succeeded", ""); err != nil {
		t.Fatal(err)
	}
	// The same result again, after a lost acknowledgement, is taken.
	if err = s.FinishSyncJob(uid, a.ID, job.ID, "succeeded", ""); err != nil {
		t.Fatalf("a repeated result was refused: %v", err)
	}
	var refused SyncJobResultRefused
	if err = s.FinishSyncJob(uid, a.ID, job.ID, "failed", ""); !errors.As(err, &refused) {
		t.Fatalf("closed job refused without a terminal marker: %v", err)
	}
	if err = s.FinishSyncJob(uid, a.ID, "missing-job", "succeeded", ""); !errors.As(err, &refused) {
		t.Fatalf("missing job refused without a terminal marker: %v", err)
	}
}

func TestQueueAgentUpdateUsesExistingJobGate(t *testing.T) {
	s, uid, _ := controlFixture(t)
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.QueueAgentUpdate(uid, a.ID, "latest")
	if err != nil || job.Kind != "update" || job.Revision != "latest" || job.TargetID != "" {
		t.Fatalf("update job: %+v %v", job, err)
	}
	if _, err = s.QueueSyncJob(uid, a.ID, "scan"); err == nil || !strings.Contains(err.Error(), "laptop follows your library") {
		t.Fatalf("a scan was queued: %v", err)
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

func TestQueueAgentUpdateStillRefusesRunningWork(t *testing.T) {
	s, uid, _ := controlFixture(t)
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	// An update the machine is running now.
	if _, err = s.QueueAgentUpdate(uid, a.ID, "latest"); err != nil {
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
	s, uid, _ := controlFixture(t)
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	// An update the machine is running now.
	if _, err = s.QueueAgentUpdate(uid, a.ID, "latest"); err != nil {
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
