package terminalbroker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store/storetest"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// applyPeer is a connected machine that can apply software and never answers: a
// request sent to it is recorded and held, so a test decides what happens next.
type applyPeer struct {
	b     *Broker
	uid   int64
	agent string
	peer  *Peer
	mu    sync.Mutex
	sent  []terminalproto.SoftwareApply
}

func applyFixture(t *testing.T) *applyPeer {
	t.Helper()
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	uid, _ := st.DefaultOwner()
	a, _, err := st.EnrollSyncAgent(uid, "studio", "test")
	if err != nil {
		t.Fatal(err)
	}
	caps := worktreeCapabilities("claude")
	caps.SoftwareApply = true
	record, err := st.BeginRemoteConnectionFor(uid, a.ID, ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	f := &applyPeer{b: New(st), uid: uid, agent: a.ID}
	t.Cleanup(f.b.Close)
	f.peer, err = f.b.Connect(uid, a.ID, record.Generation, true, func(ctx context.Context, m terminalproto.Message) error {
		reply := m
		switch op := m.Body.(type) {
		case terminalproto.Reconcile:
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
		case terminalproto.SoftwareApply:
			f.mu.Lock()
			f.sent = append(f.sent, op)
			f.mu.Unlock()
			return nil // the machine has the request and does not answer
		default:
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
		}
		data, err := terminalproto.Encode(reply)
		if err != nil {
			return err
		}
		decoded, err := terminalproto.Decode(data)
		if err != nil {
			return err
		}
		return f.peer.Receive(decoded)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.peer.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *applyPeer) sentCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

func reviewedApply() terminalproto.SoftwareApply {
	return terminalproto.SoftwareApply{Tool: "opencode", Action: "update", ExpectedState: "installed", ExpectedVersion: "1.18.0", ExpectedManaged: true}
}

// A software rollout may skip a machine that was not connected when its turn came,
// but only if nothing reached it. A machine that was never sent the request says
// so, and is still an offline machine to every caller that only asks that.
func TestSoftwareApplyToAMachineThatIsNotConnectedWasNeverSent(t *testing.T) {
	f := applyFixture(t)
	f.peer.Close()
	_, err := f.b.SoftwareApply(t.Context(), f.uid, f.agent, reviewedApply())
	if !errors.Is(err, ErrNotSent) {
		t.Fatalf("an offline machine was not reported as never sent: %v", err)
	}
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("callers that only ask whether the machine is offline stopped matching: %v", err)
	}
	if f.sentCount() != 0 {
		t.Fatal("a request was sent to a machine that is not connected")
	}
}

// If the connection drops after the request went out, the machine may be part-way
// through installing, or finished. That must never read as "never sent", or a
// rollout would skip over an outcome nobody knows.
func TestSoftwareApplyThatLosesItsConnectionAfterSendingIsNotNeverSent(t *testing.T) {
	f := applyFixture(t)
	go func() {
		for f.sentCount() == 0 {
			time.Sleep(5 * time.Millisecond)
		}
		f.peer.Close()
	}()
	start := time.Now()
	_, err := f.b.SoftwareApply(t.Context(), f.uid, f.agent, reviewedApply())
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("a dropped connection should still be offline: %v", err)
	}
	if errors.Is(err, ErrNotSent) {
		t.Fatal("an apply the machine had already received was reported as never sent")
	}
	if f.sentCount() != 1 {
		t.Fatalf("expected the request to have been sent once: %d", f.sentCount())
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("the request waited past the disconnect", time.Since(start))
	}
}

// Evidence from an older connection is not a reason to send. That is also a
// request that never left, not an uncertain one.
func TestSoftwareApplyWithStaleEvidenceWasNeverSent(t *testing.T) {
	f := applyFixture(t)
	record, err := f.b.st.BeginRemoteConnectionFor(f.uid, f.agent, ID(), func() terminalproto.Capabilities {
		caps := worktreeCapabilities("claude")
		caps.SoftwareApply = true
		return caps
	}())
	if err != nil || record.Generation == f.peer.Generation {
		t.Fatalf("expected a newer generation of evidence: %+v %v", record, err)
	}
	_, err = f.b.SoftwareApply(t.Context(), f.uid, f.agent, reviewedApply())
	if !errors.Is(err, ErrNotSent) {
		t.Fatalf("stale evidence should be reported as never sent: %v", err)
	}
	if f.sentCount() != 0 {
		t.Fatal("a request was sent on stale evidence")
	}
}
