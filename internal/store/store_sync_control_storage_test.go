package store

import (
	"errors"
	"testing"
)

// A storage failure is not a revoked enrolment, and a failed last-seen stamp
// does not refuse a machine that proved its secret.
func TestAgentAuthenticationKeepsStorageFailuresApartFromRevocation(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	a, secret, err := s.EnrollSyncAgent(uid, "laptop", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateSyncAgentBuild(uid, a.ID, "wrong", "v1", ""); !errors.Is(err, ErrUnknownMachine) {
		t.Fatalf("wrong secret: %v", err)
	}
	if _, _, err = s.AuthenticateRemoteAgent(a.ID, "wrong", "v1"); !errors.Is(err, ErrUnknownMachine) {
		t.Fatalf("wrong secret, remote: %v", err)
	}
	if _, _, err = s.AuthenticateRemoteAgent("", secret, "v1"); !errors.Is(err, ErrUnknownMachine) {
		t.Fatalf("no id, remote: %v", err)
	}

	if _, err = s.db.Exec(`CREATE TRIGGER no_stamps BEFORE UPDATE ON sync_agents BEGIN SELECT RAISE(ABORT,'disk fault'); END`); err != nil {
		t.Fatal(err)
	}
	if got, err := s.AuthenticateSyncAgentBuild(uid, a.ID, secret, "v2", ""); err != nil || got.ID != a.ID {
		t.Fatalf("failed stamp refused the machine: %+v %v", got, err)
	}
	if _, got, err := s.AuthenticateRemoteAgent(a.ID, secret, "v3"); err != nil || got.ID != a.ID {
		t.Fatalf("failed stamp refused the machine, remote: %+v %v", got, err)
	}

	s.read.Close()
	if _, err = s.AuthenticateSyncAgentBuild(uid, a.ID, secret, "v2", ""); err == nil || errors.Is(err, ErrUnknownMachine) {
		t.Fatalf("storage failure: %v", err)
	}
	if _, _, err = s.AuthenticateRemoteAgent(a.ID, secret, "v2"); err == nil || errors.Is(err, ErrUnknownMachine) {
		t.Fatalf("storage failure, remote: %v", err)
	}
}
