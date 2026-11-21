package api

import (
	"testing"
	"time"
)

func TestBacklogStartLockSerializesOnlyTheSameDraft(t *testing.T) {
	var s server
	unlockA := s.backlogStarts.lock("1/draft-a")
	// Many other drafts, more than the old 64 stripes, start while A is held.
	for i := range 200 {
		other := make(chan func(), 1)
		go func() { other <- s.backlogStarts.lock("2/draft-" + string(rune('a'+i))) }()
		select {
		case unlock := <-other:
			unlock()
		case <-time.After(2 * time.Second):
			t.Fatalf("draft %d waited for an unrelated draft's start", i)
		}
	}
	same := make(chan func(), 1)
	go func() { same <- s.backlogStarts.lock("1/draft-a") }()
	select {
	case <-same:
		t.Fatal("two starts of the same draft ran together")
	case <-time.After(50 * time.Millisecond):
	}
	unlockA()
	select {
	case unlock := <-same:
		unlock()
	case <-time.After(2 * time.Second):
		t.Fatal("the second start of the draft never ran")
	}
	s.backlogStarts.mu.Lock()
	left := len(s.backlogStarts.locks)
	s.backlogStarts.mu.Unlock()
	if left != 0 {
		t.Fatalf("%d locks kept after every start finished", left)
	}
}
