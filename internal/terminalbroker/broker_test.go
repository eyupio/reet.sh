package terminalbroker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func fixture(t *testing.T) (*Broker, *Peer, int64, string) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	uid, _ := st.DefaultOwner()
	a, _, err := st.EnrollSyncAgent(uid, "fixture", "test")
	if err != nil {
		t.Fatal(err)
	}
	caps := terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", HealthEnabled: true, TerminalEnabled: true, TerminalControl: true, Readiness: terminalproto.Ready}
	record, err := st.BeginRemoteConnectionFor(uid, a.ID, ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	b := New(st)
	t.Cleanup(b.Close)
	var p *Peer
	var mu sync.Mutex
	sessions := map[string]terminalproto.SessionSnapshot{}
	p, err = b.Connect(uid, a.ID, record.Generation, true, func(ctx context.Context, m terminalproto.Message) error {
		reply := m
		mu.Lock()
		defer mu.Unlock()
		switch op := m.Body.(type) {
		case terminalproto.Reconcile:
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
		case terminalproto.Create:
			s := terminalproto.SessionSnapshot{ID: m.SessionID, Title: op.Title, State: terminalproto.Running}
			sessions[s.ID] = s
			reply.Body = terminalproto.SessionChanged{State: s.State}
		case terminalproto.End:
			delete(sessions, m.SessionID)
			reply.Body = terminalproto.SessionChanged{State: terminalproto.Ended}
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
	if err = p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return b, p, uid, a.ID
}
func TestTicketsAreSingleUseBoundToLoginAndExpire(t *testing.T) {
	b, _, uid, agent := fixture(t)
	s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Build", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Cols: 80, Rows: 24}
	id, err := b.Ticket(uid, "login-one", s.ID, opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = b.consume(id, uid, "login-two"); !errors.Is(err, ErrTicket) {
		t.Fatal("ticket moved between logins", err)
	}
	if _, err = b.consume(id, uid, "login-one"); !errors.Is(err, ErrTicket) {
		t.Fatal("ticket replay accepted", err)
	}
	id, _ = b.Ticket(uid, "login-one", s.ID, opts)
	var wg sync.WaitGroup
	success := make(chan bool, 20)
	for range 20 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := b.consume(id, uid, "login-one"); success <- err == nil }()
	}
	wg.Wait()
	close(success)
	n := 0
	for ok := range success {
		if ok {
			n++
		}
	}
	if n != 1 {
		t.Fatal("non-atomic ticket consumption", n)
	}
	id, _ = b.Ticket(uid, "login-one", s.ID, opts)
	b.mu.Lock()
	v := b.tickets[id]
	v.expires = time.Now().Add(-time.Second)
	b.tickets[id] = v
	b.mu.Unlock()
	if _, err = b.consume(id, uid, "login-one"); !errors.Is(err, ErrTicket) {
		t.Fatal("expired ticket accepted", err)
	}
	if _, err = b.Ticket(uid+100, "login-one", s.ID, opts); err == nil {
		t.Fatal("cross-account ticket issued")
	}
}

func TestResetFencesTicketsWithoutEndingSessions(t *testing.T) {
	b, p, uid, agent := fixture(t)
	session, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Keep job", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := b.Ticket(uid, "login", session.ID, Options{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	b.ResetConnections()
	select {
	case <-p.Done():
	default:
		t.Fatal("old connection remained authorized")
	}
	if _, err = b.consume(ticket, uid, "login"); !errors.Is(err, ErrTicket) {
		t.Fatal("old ticket survived", err)
	}
	saved, err := b.st.TerminalSessionFor(uid, session.ID)
	if err != nil || saved.State != terminalproto.Running {
		t.Fatal("reset ended a durable shell", saved, err)
	}
	b.mu.Lock()
	closed := b.closed
	b.mu.Unlock()
	if closed {
		t.Fatal("reset permanently closed broker")
	}
}
func TestOfflineCreateDoesNotReserveAndEndRemainsDurable(t *testing.T) {
	b, p, uid, agent := fixture(t)
	s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Work", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	p.Close()
	if _, err = b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Surprise", Cols: 80, Rows: 24}); !errors.Is(err, ErrOffline) {
		t.Fatal(err)
	}
	sessions, _ := b.st.TerminalSessionsFor(uid)
	if len(sessions) != 1 {
		t.Fatal("offline create reserved work", sessions)
	}
	s, err = b.End(t.Context(), uid, s.ID, ID())
	if err != nil || s.State != terminalproto.EndRequested {
		t.Fatal("offline end lost", s, err)
	}
}
func TestReverseBindingAndOldGenerationAreFenced(t *testing.T) {
	b, p, uid, agent := fixture(t)
	h, e, err := p.expect(ID(), ID(), "attach")
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	defer p.forget(h)
	bad := h
	bad.Token = ID()
	if err = b.Bind(t.Context(), uid, agent, bad, nil); !errors.Is(err, ErrDenied) {
		t.Fatal("wrong token accepted", err)
	}
	bad = h
	bad.Operation = "history"
	if err = b.Bind(t.Context(), uid, agent, bad, nil); !errors.Is(err, ErrDenied) {
		t.Fatal("wrong operation accepted", err)
	}
	if err = b.Bind(t.Context(), uid+1, agent, h, nil); err == nil {
		t.Fatal("cross-account reverse stream")
	}
	next, err := b.Connect(uid, agent, p.Generation+1, true, func(context.Context, terminalproto.Message) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	select {
	case <-p.Done():
	default:
		t.Fatal("old control generation survived")
	}
	if err = b.Bind(t.Context(), uid, agent, h, nil); err == nil {
		t.Fatal("old reverse stream accepted")
	}
}
func TestReconnectDoesNotReplayUncertainCreate(t *testing.T) {
	b, p, uid, agent := fixture(t)
	s, _, err := b.st.ReserveTerminalCreateFor(uid, agent, ID(), p.Generation, terminalproto.Create{Title: "Uncertain", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	s, err = b.st.TerminalSessionFor(uid, s.ID)
	if err != nil || s.State != terminalproto.Lost {
		t.Fatal("uncertain create was not reconciled absent", s, err)
	}
}

func TestTerminalTicketCannotCrossConnectionGenerations(t *testing.T) {
	b, p, uid, agent := fixture(t)
	s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Work", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := b.Ticket(uid, "login", s.ID, Options{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	next, err := b.Connect(uid, agent, p.Generation+1, true, func(context.Context, terminalproto.Message) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	next.mu.Lock()
	next.ready = true
	next.mu.Unlock()
	if err = b.Serve(t.Context(), uid, "login", ticket, nil, func() bool { return true }); !errors.Is(err, ErrTicket) {
		t.Fatal("old generation ticket accepted", err)
	}
}
