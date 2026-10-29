package store

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

func machineSyncFixture(t *testing.T) (*Store, int64, SyncAgent, SyncAgent, string, RemoteImport, []skillbundle.ContentFile) {
	t.Helper()
	s, uid, a, env := observationFixture(t)
	if _, err := s.UpsertLocalFor(uid, "demo", Patch{}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEnvironmentFor(uid, "local/demo", a.Host, "agents", "", "", ""); err != nil {
		t.Fatal(err)
	}
	b, _, err := s.EnrollSyncAgent(uid, "desktop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.TouchEnvironmentFor(uid, b.Host, "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	dest := EnvironmentIDFor(uid, b.Host, "agents", "")
	if err = s.RecordInventoryFor(uid, dest, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimRemoteImportFor(uid, a.ID, strings.Repeat("a", 48)); err != nil {
		t.Fatal(err)
	}
	r, err := s.RequestMachineSkillSync(uid, "local/demo", b.Host, env, "machine_sync_request_0001", dest, false)
	if err != nil {
		t.Fatal(err)
	}
	return s, uid, a, b, dest, r, machineImportFixture(t).Files
}
func finishMachineCopy(t *testing.T, s *Store, uid int64, a SyncAgent, r RemoteImport, files []skillbundle.ContentFile) RemoteImport {
	t.Helper()
	r = reviewRemote(t, s, uid, a, r, files)
	if r.State != "approved" {
		t.Fatalf("explicit safe copy should proceed without second approval: %+v", r)
	}
	if _, err := s.ClaimRemoteImportFor(uid, a.ID, r.Instance); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CommitRemoteImportFor(uid, a.ID, r.ID, r.Instance, r.Preview.SourceDigest, files); err != nil {
		t.Fatal(err)
	}
	r, err := s.RemoteImportFor(uid, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestMachineSkillSyncDurablePrivateAndIdempotent(t *testing.T) {
	s, uid, a, b, dest, r, files := machineSyncFixture(t)
	again, err := s.RequestMachineSkillSync(uid, "local/demo", b.Host, r.EnvironmentID, r.RequestKey, dest, false)
	if err != nil || again.ID != r.ID {
		t.Fatal(again, err)
	}
	if _, err = s.RequestMachineSkillSync(uid, "local/demo", b.Host, r.EnvironmentID, r.RequestKey, dest, true); err == nil {
		t.Fatal("changed follow policy reused request")
	}
	r = finishMachineCopy(t, s, uid, a, r, files)
	candidates, err := s.SyncCandidatesFor(uid, "owner", 0)
	if err != nil || len(candidates) != 1 || candidates[0].SourceType != "private" || !strings.Contains(candidates[0].Source, r.Receipt.Revision.ID) {
		t.Fatal(candidates, err)
	}
	// No browser is involved: the destination poll advances the saved transfer.
	if err = s.AdvanceMachineSkillSyncs(uid, "owner"); err != nil {
		t.Fatal(err)
	}
	r, _ = s.RemoteImportFor(uid, r.ID)
	targets, _ := s.SyncTargetsFor(uid)
	if r.SyncRunID == "" || len(targets) != 1 || len(targets[0].Desired) != 1 || targets[0].Desired[0].ID != "local/demo" {
		t.Fatal(r, targets)
	}
	revision := targets[0].Revision
	job, err := s.ClaimSyncJob(uid, b)
	if err != nil || job == nil || job.TargetID != dest {
		t.Fatal(job, err)
	}
	// Simulate a crash after run commit, before recording its receipt.
	if err = s.withTx(func(tx *sql.Tx) error {
		current, e := readImport(tx, uid, r.ID)
		if e != nil {
			return e
		}
		current.SyncRunID = ""
		return writeImport(tx, uid, current)
	}); err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceMachineSkillSyncs(uid, "owner"); err != nil {
		t.Fatal(err)
	}
	targets, _ = s.SyncTargetsFor(uid)
	runs, _ := s.SyncRunsFor(uid)
	recovered, _ := s.RemoteImportFor(uid, r.ID)
	if len(runs) != 1 || targets[0].Revision != revision || recovered.SyncRunID != r.SyncRunID {
		t.Fatal("transfer replayed", runs, targets, recovered)
	}
	sk, _, _ := s.GetFor(uid, "local/demo")
	if sk.SourceType != "local" {
		t.Fatal("private bytes published as public source", sk)
	}
}
func TestMachineSkillSyncWaitsForDestinationAndRecoversStaleScan(t *testing.T) {
	s, uid, a, b, dest, r, files := machineSyncFixture(t)
	r = finishMachineCopy(t, s, uid, a, r, files)
	if _, err := s.QueueSyncJob(uid, b.ID, "", "scan"); err != nil {
		t.Fatal(err)
	}
	if err := s.AdvanceMachineSkillSyncs(uid, "owner"); err != nil {
		t.Fatal(err)
	}
	current, _ := s.RemoteImportFor(uid, r.ID)
	if current.SyncRunID != "" {
		t.Fatal("changed destination while busy", current)
	}
	if _, err := s.db.Exec(`UPDATE sync_jobs SET status='completed'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE environment_scans SET scanned_at=1 WHERE env_id=?`, dest); err != nil {
		t.Fatal(err)
	}
	if err := s.AdvanceMachineSkillSyncs(uid, "owner"); err != nil {
		t.Fatal("stale transfer blocked polls", err)
	}
	current, _ = s.RemoteImportFor(uid, r.ID)
	if current.SyncError == "" || current.SyncRunID != "" {
		t.Fatal(current)
	}
	if err := s.RecordInventoryFor(uid, dest, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	if err := s.AdvanceMachineSkillSyncs(uid, "owner"); err != nil {
		t.Fatal(err)
	}
	current, _ = s.RemoteImportFor(uid, r.ID)
	if current.SyncError != "" || current.SyncRunID == "" {
		t.Fatal("refresh did not recover", current)
	}
}
func TestMachineSkillSyncCannotBypassFileReviewOrOwnership(t *testing.T) {
	s, uid, a, _, dest, r, files := machineSyncFixture(t)
	files = append(files, skillbundle.ContentFile{Path: ".env", Data: []byte("PRIVATE_TOKEN=no-upload")})
	current := reviewRemote(t, s, uid, a, r, files)
	if current.State != "reviewing" {
		t.Fatal("unsafe files automatically uploaded", current)
	}
	if _, err := s.RequestMachineSkillSync(uid, "local/absent", "desktop", r.EnvironmentID, "machine_sync_unknown_0001", dest, false); err == nil {
		t.Fatal("unreported skill accepted")
	}
	if _, err := s.CreateRemoteImportFor(uid, RemoteImportCreate{RequestKey: "machine_sync_forged_0001", AgentID: a.ID, EnvironmentID: r.EnvironmentID, Name: "different", SyncSkillID: "local/demo", DestinationID: dest}); err == nil {
		t.Fatal("mismatched skill accepted")
	}
	if _, err := s.RequestMachineSkillSync(uid+999, "local/demo", "desktop", r.EnvironmentID, "machine_sync_foreign_0001", dest, false); err == nil {
		t.Fatal("foreign skill accepted")
	}
}
