package terminalbroker

import (
	"context"
	"errors"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// lockMachine records what a locked machine reports: terminal access off,
// with machine-lock-v1's Locked set.
func lockMachine(t *testing.T, b *Broker, p *Peer, uid int64, agent string) {
	t.Helper()
	rec, err := b.st.RemoteCapabilitiesRawFor(uid, agent)
	if err != nil {
		t.Fatal(err)
	}
	caps := rec.Capabilities
	caps.TerminalEnabled, caps.Readiness, caps.Locked = false, terminalproto.DisabledLocally, true
	if err = b.st.UpdateRemoteCapabilitiesFor(uid, agent, rec.InstanceID, rec.Generation, caps); err != nil {
		t.Fatal(err)
	}
	p.Capabilities(false)
}

func TestALockedMachineIsRefusedAsLockedBeforeAnythingIsReserved(t *testing.T) {
	b, p, uid, agent := fixture(t)
	if b.Locked(uid, agent) {
		t.Fatal("an unlocked machine read as locked")
	}
	lockMachine(t, b, p, uid, agent)
	if !b.Locked(uid, agent) || b.Ready(uid, agent) {
		t.Fatal("a locked machine read as ready")
	}
	_, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Work", Cols: 80, Rows: 24})
	if !errors.Is(err, ErrLocked) || err.Error() != terminalproto.LockedSentence {
		t.Fatalf("create on a locked machine: %v", err)
	}
	// Callers that treat a machine that is not ready keep doing so.
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("a locked machine stopped reading as not ready: %v", err)
	}
	if sessions, _ := b.st.TerminalSessionsFor(uid); len(sessions) != 0 {
		t.Fatalf("a locked machine had work reserved: %+v", sessions)
	}
	if ErrLocked != store.ErrMachineLocked {
		t.Fatal("the broker and the store disagree on the lock")
	}
}

// Whatever the request, a locked answer from the machine is ErrLocked.
func TestALockedAnswerIsErrLocked(t *testing.T) {
	_, p, _, _ := fixture(t)
	p.send = func(_ context.Context, m terminalproto.Message) error {
		m.Body = terminalproto.Result{Code: terminalproto.Locked}
		data, err := terminalproto.Encode(m)
		if err != nil {
			return err
		}
		decoded, err := terminalproto.Decode(data)
		if err != nil {
			return err
		}
		return p.Receive(decoded)
	}
	if _, err := p.call(t.Context(), terminalproto.Message{Body: terminalproto.Browse{Path: "~"}}); !errors.Is(err, ErrLocked) {
		t.Fatalf("a locked answer read as %v", err)
	}
}
