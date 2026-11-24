package terminalhost

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// Taking the machine lock cuts every attachment within a second, keeps the
// shell running, and once lifted the session can be attached again.
func TestTheLockClosesAttachmentsKeepsTheShellAndAllowsReattachAfterUnlock(t *testing.T) {
	m, _ := managerFixture(t)
	s := createManaged(t, m, 1)
	pid := managedPID(t, m, s.ID)
	var locked atomic.Bool
	m.locked = locked.Load
	a, _ := attachManaged(t, m, s.ID, 1, false, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	locked.Store(true)
	select {
	case <-a.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the lock left the client attached")
	}
	if managedPID(t, m, s.ID) != pid {
		t.Fatal("the lock killed work")
	}
	locked.Store(false)
	again, _ := attachManaged(t, m, s.ID, 2, false, time.Minute)
	time.Sleep(1500 * time.Millisecond)
	select {
	case <-again.Done():
		t.Fatal("an attachment after unlock was cut")
	default:
	}
	again.Close()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("observer did not stop")
	}
}
