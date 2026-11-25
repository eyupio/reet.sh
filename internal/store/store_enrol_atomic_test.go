package store

import (
	"errors"
	"testing"
	"time"
)

// A storage failure after the code is claimed is not a decision about the
// machine: the code is still good for the next try.
func TestEnrolCodeSurvivesAStorageFailureAfterItsClaim(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if err := s.CreateEnrolCode(uid, "code", time.Now().Add(time.Minute).UnixMilli(), 20); err != nil {
		t.Fatal(err)
	}
	// The name lookup comes after the claim; make it fail.
	if _, err := s.db.Exec(`ALTER TABLE machine_aliases RENAME COLUMN alias TO alias_lost`); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := s.EnrolWithCode("code", "laptop", "test", ""); err == nil || errors.Is(err, ErrEnrolCodeInvalid) {
		t.Fatalf("storage failure: %v", err)
	}
	if _, err := s.db.Exec(`ALTER TABLE machine_aliases RENAME COLUMN alias_lost TO alias`); err != nil {
		t.Fatal(err)
	}
	if gotUID, a, _, _, err := s.EnrolWithCode("code", "laptop", "test", ""); err != nil || gotUID != uid || a.Host != "laptop" {
		t.Fatalf("the code was spent by a storage failure: %+v %v", a, err)
	}
}

// A code of an account that has been disabled enrols nothing.
func TestEnrolCodeOfADisabledAccountIsRefused(t *testing.T) {
	s := newStore(t)
	u, err := s.CreateUser("gone@enrol.invalid", "Gone", "member", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CreateEnrolCode(u.ID, "code", time.Now().Add(time.Minute).UnixMilli(), 20); err != nil {
		t.Fatal(err)
	}
	if err = s.SetUserDisabled(u.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err = s.EnrolWithCode("code", "laptop", "test", ""); !errors.Is(err, ErrEnrolCodeInvalid) {
		t.Fatalf("disabled account's code: %v", err)
	}
	if list, _ := s.SyncAgentsFor(u.ID); len(list) != 0 {
		t.Fatalf("machines enrolled for a disabled account: %+v", list)
	}
}
