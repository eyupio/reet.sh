package main

import (
	"context"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// A session that runs out while the controller is up is dropped without a
// restart.
func TestPruneEveryDropsWhatExpiresWhileRunning(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	user, err := st.CreateUser("prune@example.com", "Prune", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	if err := st.CreateSession("expired", user.ID, now-1, "test"); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateSession("current", user.ID, now+int64(time.Hour/time.Millisecond), "test"); err != nil {
		t.Fatal(err)
	}
	// SessionUser looks past an expiry time of zero, so it sees a row until
	// the row is gone.
	held := func(id string) bool {
		_, ok, err := st.SessionUser(id, 0)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !held("expired") {
		t.Fatal("the expired session was not stored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { pruneEvery(ctx, st, 10*time.Millisecond); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for held("expired") {
		if time.Now().After(deadline) {
			t.Fatal("the expired session was never pruned")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !held("current") {
		t.Fatal("a session still in date was pruned")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("pruneEvery did not stop with its context")
	}
}
