package store

import (
	"errors"
	"testing"
	"time"
)

// Two routes answer a caller that has shown no credential: the controller proof
// and a code enrolment. Neither may queue behind (or in front of) every agent
// poll on the one write connection, so a flood of guesses cannot slow the fleet.
func TestUnauthenticatedLookupsDoNotWaitForTheWriteConnection(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	hold, err := s.db.Begin() // the only write connection, held for the rest of the test
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Rollback()

	within := func(name string, f func()) {
		t.Helper()
		done := make(chan struct{})
		go func() { f(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s waits for the write connection", name)
		}
	}
	within("AgentSecretHash", func() {
		if _, ok, err := s.AgentSecretHash(a.ID); err != nil || !ok {
			t.Errorf("the record was not read: %v %v", ok, err)
		}
	})
	within("EnrolWithCode with a code that does not exist", func() {
		if _, _, _, _, err := s.EnrolWithCode("no-such-code-hash", "desktop", "test", ""); !errors.Is(err, ErrEnrolCodeInvalid) {
			t.Errorf("an unknown code: %v", err)
		}
	})
}
