package projectrun

import (
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
)

// A member on a locked machine is refused as one on a machine that is not
// ready is, never sent, and its code names the lock.
func TestAMemberOnALockedMachineIsNeverSentAndSaysWhy(t *testing.T) {
	f := newFixture(t)
	f.broker.setReady(f.server, false)
	f.broker.mu.Lock()
	f.broker.locked = map[string]bool{f.server: true}
	f.broker.mu.Unlock()
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.server, "/a/1"}, [2]string{f.laptop, "/a/2"}))
	o.Start(f.uid, r.ID)
	got := settle(t, o, f.st, f.uid, r.ID)
	if m := member(got, 0); m.Code != CodeMachineLocked || m.Sent {
		t.Fatal(m)
	}
	if member(got, 1).State != store.RunMemberStarted {
		t.Fatalf("%+v", member(got, 1))
	}
	for _, c := range f.broker.sent() {
		if c.agent == f.server {
			t.Fatal("a create reached the locked machine")
		}
	}
}

// A machine locked between the precheck and the create refuses it as locked.
func TestACreateRefusedAsLockedNamesTheLock(t *testing.T) {
	f := newFixture(t)
	f.broker.answer = func(sent) answer {
		return answer{kind: refuse, err: store.LockedError{Was: terminalbroker.ErrOffline}}
	}
	o := f.orchestrator(t)
	r := f.run(t, f.project(t, true, [2]string{f.laptop, "/a/1"}))
	o.Start(f.uid, r.ID)
	if m := settle(t, o, f.st, f.uid, r.ID).Members[0]; m.Code != CodeMachineLocked || m.Sent {
		t.Fatal(m)
	}
}
