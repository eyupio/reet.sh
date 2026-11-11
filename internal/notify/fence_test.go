package notify

import (
	"context"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

// A fenced controller prunes nothing; the loop goes on pruning once the fence
// is lifted.
func TestAFencedNotifierPrunesNothingUntilTheLift(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	uid, err := st.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	n, _, err := st.RecordNotification(store.Notification{UserID: uid, Kind: "test", Key: "old", Title: "Old"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.MarkNotificationsRead(uid, []int64{n.ID}, true); err != nil {
		t.Fatal(err)
	}
	old := retention
	retention = -time.Hour // everything read is old enough
	t.Cleanup(func() { retention = old })
	exists := func() bool { _, err := st.NotificationByID(n.ID); return err == nil }

	if _, err := st.SetFence(""); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{Store: st}
	loopDone := make(chan struct{})
	go func() { defer close(loopDone); s.Loop(ctx, 5*time.Millisecond) }()
	// The loop reads retention, which the cleanup above restores: stop it and
	// wait for it first, or the race detector sees the two meet.
	defer func() { cancel(); <-loopDone }()
	time.Sleep(100 * time.Millisecond)
	if !exists() {
		t.Fatal("a notification was pruned while fenced")
	}
	if _, err := st.LiftFence(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for exists() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if exists() {
		t.Fatal("nothing was pruned after the lift")
	}
}
