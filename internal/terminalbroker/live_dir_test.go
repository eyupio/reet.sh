package terminalbroker

import (
	"context"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/store/storetest"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A folder the machine reports on a session change is kept on the record; a
// change without one leaves the folder as it was.
func TestSessionChangeRecordsLiveDir(t *testing.T) {
	b, p, uid, agent := fixture(t)
	s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"/home/me/app", ""} {
		m := terminalproto.Message{Version: terminalproto.Version, RequestID: ID(), SessionID: s.ID, Generation: p.Generation, Body: &terminalproto.SessionChanged{State: terminalproto.Running, Dir: dir}}
		if err := p.Receive(m); err != nil {
			t.Fatal(err)
		}
		p.settleObservations(5e9)
		got, err := b.st.TerminalSessionFor(uid, s.ID)
		if err != nil || got.LiveDir != "/home/me/app" {
			t.Fatalf("after %q: %+v %v", dir, got, err)
		}
	}
	select {
	case <-p.Done():
		t.Fatal("the folder cost the machine its link")
	default:
	}
}

// The reconcile a connection starts with carries each session's folder.
func TestSynchronizeRecordsLiveDir(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	uid, _ := st.DefaultOwner()
	a, _, err := st.EnrollSyncAgent(uid, "fixture", "test")
	if err != nil {
		t.Fatal(err)
	}
	caps := terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", HealthEnabled: true, TerminalEnabled: true, TerminalControl: true, DeskShell: true, Readiness: terminalproto.Ready}
	record, err := st.BeginRemoteConnectionFor(uid, a.ID, ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	b := New(st)
	t.Cleanup(b.Close)
	var mu sync.Mutex
	sessions := map[string]terminalproto.SessionSnapshot{}
	var p *Peer
	p, err = b.Connect(uid, a.ID, record.Generation, true, func(ctx context.Context, m terminalproto.Message) error {
		mu.Lock()
		defer mu.Unlock()
		reply := m
		switch op := m.Body.(type) {
		case terminalproto.Reconcile:
			list := []terminalproto.SessionSnapshot{}
			for _, s := range sessions {
				list = append(list, s)
			}
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: list}
		case terminalproto.Create:
			sessions[m.SessionID] = terminalproto.SessionSnapshot{ID: m.SessionID, Title: op.Title, State: terminalproto.Running}
			reply.Body = terminalproto.SessionChanged{State: terminalproto.Running}
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
		return p.Receive(decoded)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	s, err := b.Create(t.Context(), uid, a.ID, ID(), terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	snap := sessions[s.ID]
	snap.Dir = "/srv/project with space"
	sessions[s.ID] = snap
	mu.Unlock()
	if err := p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, err := st.TerminalSessionFor(uid, s.ID)
	if err != nil || got.LiveDir != "/srv/project with space" {
		t.Fatalf("%+v %v", got, err)
	}
}
