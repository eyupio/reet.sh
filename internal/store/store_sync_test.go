package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestSyncSnapshotPersistenceAndIsolation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	owner, err := s.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	member, err := s.CreateUser("sync@example.com", "Member", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	r := mustRef(t, "a/b/alpha")
	if _, err := s.UpsertFor(owner, r, Patch{}); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"laptop", "server"} {
		if err := s.TouchEnvironmentFor(owner, host, "agents", "", ""); err != nil {
			t.Fatal(err)
		}
		if err := s.RecordInventoryFor(owner, EnvironmentIDFor(owner, host, "agents", ""), true, MachineInfo{OS: "linux", Arch: "amd64"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RecordEnvironmentFor(owner, r.ID, "laptop", "agents", "", "", ""); err != nil {
		t.Fatal(err)
	}
	source, target := EnvironmentIDFor(owner, "laptop", "agents", ""), EnvironmentIDFor(owner, "server", "agents", "")
	if err := s.SaveSyncTargetFor(member.ID, source, target); !errors.Is(err, ErrSync) {
		t.Fatalf("cross-account config allowed: %v", err)
	}
	if err := s.SaveSyncTargetFor(owner, source, target); err != nil {
		t.Fatal(err)
	}
	plans, err := s.SyncTargetsFor(owner)
	if err != nil || len(plans) != 1 || len(plans[0].Missing) != 1 {
		t.Fatalf("plans: %+v %v", plans, err)
	}
	revision := plans[0].Revision
	if got, err := s.SyncTargetsFor(member.ID); err != nil || len(got) != 0 {
		t.Fatalf("other account sees plans: %+v %v", got, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	plans, err = s.SyncTargetsFor(owner)
	if err != nil || len(plans) != 1 || plans[0].Revision != revision {
		t.Fatalf("lost persisted config: %+v %v", plans, err)
	}
	if _, err := s.PruneEnvironmentFor(owner, source, nil); err != nil {
		t.Fatal(err)
	}
	plans, err = s.SyncTargetsFor(owner)
	if err != nil || len(plans[0].Desired) != 1 {
		t.Fatalf("reference changed saved snapshot: %+v %v", plans, err)
	}
	if err := s.RecordEnvironmentFor(owner, r.ID, "server", "agents", "", "", ""); err != nil {
		t.Fatal(err)
	}
	plans, err = s.SyncTargetsFor(owner)
	if err != nil || len(plans[0].Missing) != 0 || len(plans[0].Extra) != 0 {
		t.Fatalf("target drift incorrect: %+v %v", plans, err)
	}
	if err := s.ForgetEnvironmentFor(member.ID, target); err != nil {
		t.Fatal(err)
	}
	plans, _ = s.SyncTargetsFor(owner)
	if len(plans) != 1 {
		t.Fatal("another user forgot the target")
	}
	if err := s.ForgetEnvironmentFor(owner, source); err != nil {
		t.Fatal(err)
	}
	plans, _ = s.SyncTargetsFor(owner)
	if len(plans) != 1 || len(plans[0].Desired) != 1 {
		t.Fatal("forgetting reference lost saved snapshot")
	}
	if err := s.DeleteSyncTargetFor(member.ID, target); err != nil {
		t.Fatal(err)
	}
	plans, _ = s.SyncTargetsFor(owner)
	if len(plans) != 1 {
		t.Fatal("other user deleted config")
	}
	if err := s.DeleteSyncTargetFor(owner, target); err != nil {
		t.Fatal(err)
	}
	plans, _ = s.SyncTargetsFor(owner)
	if len(plans) != 0 {
		t.Fatal("config not deleted")
	}
}

func TestSyncRequiresCompleteFreshInventories(t *testing.T) {
	s := newStore(t)
	owner, _ := s.DefaultOwner()
	source, target := EnvironmentIDFor(owner, "source", "agents", ""), EnvironmentIDFor(owner, "target", "agents", "")
	for _, host := range []string{"source", "target"} {
		if err := s.TouchEnvironmentFor(owner, host, "agents", "", ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SaveSyncTargetFor(owner, source, target); !errors.Is(err, ErrSync) {
		t.Fatalf("unscanned allowed: %v", err)
	}
	for _, id := range []string{source, target} {
		if err := s.RecordInventoryFor(owner, id, true, MachineInfo{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SaveSyncTargetFor(owner, source, target); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE environment_scans SET scanned_at=? WHERE env_id=?`, time.Now().Add(-15*24*time.Hour).UnixMilli(), source); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSyncTargetFor(owner, source, target); !errors.Is(err, ErrSync) {
		t.Fatalf("stale source allowed: %v", err)
	}
	if err := s.SaveSyncTargetFor(owner, target, target); !errors.Is(err, ErrSync) {
		t.Fatalf("self mirror allowed: %v", err)
	}
}

func TestProjectEnvironmentIDsDoNotCollide(t *testing.T) {
	a := EnvironmentIDFor(1, "machine", "claude", "/work/a-b/c")
	b := EnvironmentIDFor(1, "machine", "claude", "/work/a/b-c")
	if a == b {
		t.Fatal("different project paths share an environment")
	}
	if a != EnvironmentIDFor(1, "machine", "claude", "/work/a-b/c") {
		t.Fatal("project ID is unstable")
	}
}
