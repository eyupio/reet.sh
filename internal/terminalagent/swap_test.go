//go:build linux || darwin

package terminalagent

import (
	"testing"
	"time"
)

func TestSwapGroupWaitsAndReuses(t *testing.T) {
	g := newSwapGroup()
	if !g.Wait(time.Millisecond) {
		t.Fatal("empty group did not drain")
	}
	g.Add(1)
	if g.Wait(20 * time.Millisecond) {
		t.Fatal("group drained with a swap in flight")
	}
	g.Done()
	if !g.Wait(time.Second) {
		t.Fatal("group did not drain after Done")
	}

	g.Add(1)
	if g.Wait(20 * time.Millisecond) {
		t.Fatal("reused group drained with a swap in flight")
	}
	g.Done()
	if !g.Wait(time.Second) {
		t.Fatal("reused group did not drain after Done")
	}
}
