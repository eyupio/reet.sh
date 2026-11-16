package terminalbroker

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type deskMachine struct {
	b     *Broker
	st    *store.Store
	p     *Peer
	uid   int64
	agent string
	mu    sync.Mutex
	sent  []terminalproto.Create
	reply func(terminalproto.Create) terminalproto.Body
}

func (d *deskMachine) creates() []terminalproto.Create {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]terminalproto.Create(nil), d.sent...)
}

// deskMachineFixture connects an in-memory machine with caps that records
// every create it is sent and answers it as running unless reply says else.
func deskMachineFixture(t *testing.T, caps terminalproto.Capabilities) *deskMachine {
	t.Helper()
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
	record, err := st.BeginRemoteConnectionFor(uid, a.ID, ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	d := &deskMachine{b: New(st), st: st, uid: uid, agent: a.ID}
	t.Cleanup(d.b.Close)
	d.p, err = d.b.Connect(uid, a.ID, record.Generation, true, func(ctx context.Context, m terminalproto.Message) error {
		reply := m
		switch op := m.Body.(type) {
		case terminalproto.Reconcile:
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
		case terminalproto.Create:
			d.mu.Lock()
			d.sent = append(d.sent, op)
			respond := d.reply
			d.mu.Unlock()
			reply.Body = terminalproto.SessionChanged{State: terminalproto.Running}
			if respond != nil {
				reply.Body = respond(op)
			}
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
		return d.p.Receive(decoded)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return d
}

func deskCaps(deskShell bool) terminalproto.Capabilities {
	return terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", HealthEnabled: true, TerminalEnabled: true, TerminalControl: true, ShellDir: true, DeskShell: deskShell, Readiness: terminalproto.Ready}
}

const brokerDeskPane = "0123456789abcdef"

// A machine that did not negotiate desk-shell-v1 is never sent a desk
// reference: the create is refused before anything is reserved.
func TestCreateDeskRefNotSentToOldMachine(t *testing.T) {
	d := deskMachineFixture(t, deskCaps(false))
	for _, desk := range []terminalproto.DeskRef{{Pane: brokerDeskPane}, {Pane: brokerDeskPane, Dir: "/srv/app"}} {
		_, err := d.b.Create(t.Context(), d.uid, d.agent, ID(), terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24, Desk: desk})
		if !errors.Is(err, ErrDeskShellUnsupported) {
			t.Fatalf("%+v: %v", desk, err)
		}
	}
	if sent := d.creates(); len(sent) != 0 {
		t.Fatalf("sent to an older machine: %+v", sent)
	}
	if list, _ := d.st.TerminalSessionsFor(d.uid); len(list) != 0 {
		t.Fatalf("reserved: %+v", list)
	}
	// The same machine still opens a plain shell in a folder (shell-dir-v1).
	if _, err := d.b.Create(t.Context(), d.uid, d.agent, ID(), terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24, Dir: "/home/test/app"}); err != nil {
		t.Fatal(err)
	}
}

// A desk-shell-v1 machine is sent the reference as it is, even with no folder
// and on a machine that cannot start agents.
func TestCreateDeskRefSentToDeskShellMachine(t *testing.T) {
	d := deskMachineFixture(t, deskCaps(true))
	in := terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24, Desk: terminalproto.DeskRef{Pane: brokerDeskPane}}
	if _, err := d.b.Create(t.Context(), d.uid, d.agent, ID(), in); err != nil {
		t.Fatal(err)
	}
	in.Desk.Dir = "/srv/app"
	if _, err := d.b.Create(t.Context(), d.uid, d.agent, ID(), in); err != nil {
		t.Fatal(err)
	}
	sent := d.creates()
	if len(sent) != 2 || sent[0].Desk != (terminalproto.DeskRef{Pane: brokerDeskPane}) || sent[1].Desk.Dir != "/srv/app" || sent[1].Dir != "" {
		t.Fatalf("sent: %+v", sent)
	}
}

// The machine's folder-gone answer reaches the caller as ErrFolderGone, and a
// replay of the same request says the same.
func TestCreateDeskFolderGone(t *testing.T) {
	d := deskMachineFixture(t, deskCaps(true))
	d.reply = func(terminalproto.Create) terminalproto.Body {
		return terminalproto.Result{Code: terminalproto.LaunchFailed, Detail: terminalproto.FolderGone}
	}
	request := ID()
	in := terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24, Desk: terminalproto.DeskRef{Pane: brokerDeskPane, Dir: "/srv/gone"}}
	if _, err := d.b.Create(t.Context(), d.uid, d.agent, request, in); !errors.Is(err, ErrFolderGone) {
		t.Fatalf("first: %v", err)
	}
	if _, err := d.b.Create(t.Context(), d.uid, d.agent, request, in); !errors.Is(err, ErrFolderGone) {
		t.Fatalf("replay: %v", err)
	}
}

// The reserved hook runs once the session is reserved and before the machine
// is asked; a hook that refuses leaves nothing sent and the session lost.
func TestCreateSessionReservedHook(t *testing.T) {
	d := deskMachineFixture(t, deskCaps(true))
	in := terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24, Desk: terminalproto.DeskRef{Pane: brokerDeskPane}}
	var seen string
	s, started, err := d.b.CreateSessionReserved(t.Context(), d.uid, d.agent, ID(), in, func(r store.TerminalSession) error {
		seen = r.ID
		if len(d.creates()) != 0 {
			t.Error("hook ran after the machine was asked")
		}
		return nil
	})
	if err != nil || !started || seen != s.ID {
		t.Fatalf("%+v %v %v %q", s, started, err, seen)
	}
	refusal := errors.New("pane changed")
	s, started, err = d.b.CreateSessionReserved(t.Context(), d.uid, d.agent, ID(), in, func(store.TerminalSession) error { return refusal })
	if !errors.Is(err, refusal) || started {
		t.Fatalf("%v %v", started, err)
	}
	if len(d.creates()) != 1 {
		t.Fatalf("a refused hook still sent: %+v", d.creates())
	}
	got, err := d.st.TerminalSessionFor(d.uid, s.ID)
	if err != nil || got.State == terminalproto.Creating || got.State == terminalproto.Running {
		t.Fatalf("refused reservation: %+v %v", got, err)
	}
	// Nothing ever ran in it, so Sessions does not show it.
	if archived, err := d.st.TerminalArchivedFor(d.uid, s.ID); err != nil || !archived {
		t.Fatalf("refused reservation left in history: %v %v", archived, err)
	}
	list, err := d.st.TerminalSessionsFor(d.uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, listed := range list {
		if listed.ID == s.ID {
			t.Fatalf("refused reservation listed: %+v", listed)
		}
	}
}

// A folder that arrives with a session change the controller could not
// observe (here a change for a session that has ended) is not recorded.
func TestLiveDirOnlyAfterObserve(t *testing.T) {
	b, p, uid, agent := fixture(t)
	s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.End(t.Context(), uid, s.ID, ID()); err != nil {
		t.Fatal(err)
	}
	m := terminalproto.Message{Version: terminalproto.Version, RequestID: ID(), SessionID: s.ID, Generation: p.Generation, Body: &terminalproto.SessionChanged{State: terminalproto.Running, Dir: "/home/me/late"}}
	if err := p.Receive(m); err != nil {
		t.Fatal(err)
	}
	p.settleObservations(5e9)
	got, err := b.st.TerminalSessionFor(uid, s.ID)
	if err != nil || got.LiveDir != "" {
		t.Fatalf("%+v %v", got, err)
	}
}
