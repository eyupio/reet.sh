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
