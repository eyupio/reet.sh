//go:build linux || darwin

package scan

import (
	"io"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/filelock"
)

// reet agent lock waits for another writer of the enrolment (here one that
// read the file before the lock and writes after, as a move being followed
// does) and then changes only the lock, so the machine ends locked and the
// other writer's change is kept.
func TestALockWrittenBesideAnotherWriterIsKept(t *testing.T) {
	config := enrolledMachine(t, "https://old.example.com")
	held, err := filelock.Acquire(config + ".edit.lock")
	if err != nil {
		t.Fatal(err)
	}
	stale := loaded(t, config)
	done := make(chan error, 1)
	go func() { done <- RunAgent([]string{"lock", "--config", config}, io.Discard, "test") }()
	time.Sleep(200 * time.Millisecond)
	stale.Server = "https://new.example.com"
	if err := writePrivate(config, stale); err != nil {
		t.Fatal(err)
	}
	held.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the lock never finished")
	}
	if now := loaded(t, config); !now.Locked || now.Server != "https://new.example.com" {
		t.Fatalf("saved %+v", now)
	}
}

// A writer that cannot get the edit lock in time says so and changes nothing.
func TestAnEditThatCannotWaitChangesNothing(t *testing.T) {
	config := enrolledMachine(t, "https://old.example.com")
	held, err := filelock.Acquire(config + ".edit.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	before := fileBytes(t, config)
	old := configEditWait
	configEditWait = 50 * time.Millisecond
	t.Cleanup(func() { configEditWait = old })
	if err := RunAgent([]string{"lock", "--config", config}, io.Discard, "test"); err == nil {
		t.Fatal("a lock was written past another writer")
	}
	if string(before) != string(fileBytes(t, config)) {
		t.Fatal("the enrolment changed")
	}
}
