package terminalbroker

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store/storetest"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// endRecorder connects a machine with the given desk-shell support and
// records every end it is sent, as the machine decodes it.
func endRecorder(t *testing.T, deskShell bool) (*Broker, int64, string, func() []terminalproto.End) {
	t.Helper()
	b, _, uid, agent, ends := endRecorderWith(t, deskShell, nil)
	return b, uid, agent, ends
}

// endRecorderWith is endRecorder whose answer to an end comes from answer
// when it is set (an error means no answer at all).
func endRecorderWith(t *testing.T, deskShell bool, answer func(terminalproto.End) (terminalproto.Body, error)) (*Broker, *Peer, int64, string, func() []terminalproto.End) {
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
	caps := terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", HealthEnabled: true, TerminalEnabled: true, TerminalControl: true, DeskShell: deskShell, Readiness: terminalproto.Ready}
	record, err := st.BeginRemoteConnectionFor(uid, a.ID, ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	b := New(st)
	t.Cleanup(b.Close)
	var mu sync.Mutex
	var ends []terminalproto.End
	var p *Peer
	p, err = b.Connect(uid, a.ID, record.Generation, true, func(ctx context.Context, m terminalproto.Message) error {
		data, err := terminalproto.Encode(m)
		if err != nil {
			return err
		}
		heard, err := terminalproto.Decode(data)
		if err != nil {
			return err
		}
		reply := m
		switch op := heard.Body.(type) {
		case *terminalproto.Reconcile:
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
		case *terminalproto.Create:
			reply.Body = terminalproto.SessionChanged{State: terminalproto.Running}
		case *terminalproto.End:
			mu.Lock()
			ends = append(ends, *op)
			mu.Unlock()
			reply.Body = terminalproto.SessionChanged{State: terminalproto.Ended}
			if answer != nil {
				body, err := answer(*op)
				if err != nil {
					return err
				}
				reply.Body = body
			}
		default:
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
		}
		data, err = terminalproto.Encode(reply)
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
	return b, p, uid, a.ID, func() []terminalproto.End {
		mu.Lock()
		defer mu.Unlock()
		return append([]terminalproto.End(nil), ends...)
	}
}

// Ending a desk pane's shell for good asks a desk-shell-v1 machine to remove
// the pane's history; a machine without the feature is never sent the field
// (its strict decoder would refuse the end), and a plain end never asks.
func TestForgetNotSentToOldMachine(t *testing.T) {
	for _, tc := range []struct {
		name      string
		deskShell bool
		desk      bool
		want      bool
	}{
		{"old machine, desk end", false, true, false},
		{"desk-shell machine, desk end", true, true, true},
		{"desk-shell machine, plain end", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, uid, agent, ends := endRecorder(t, tc.deskShell)
			s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24})
			if err != nil {
				t.Fatal(err)
			}
			if tc.desk {
				_, err = b.EndDeskPane(t.Context(), uid, s.ID, ID(), true)
			} else {
				_, err = b.End(t.Context(), uid, s.ID, ID(), true)
			}
			if err != nil {
				t.Fatal(err)
			}
			got := ends()
			if len(got) != 1 || got[0].ForgetDesk != tc.want {
				t.Fatalf("ends sent %+v, want one with ForgetDesk %v", got, tc.want)
			}
			// The mark is dropped once the machine has answered.
			b.mu.Lock()
			left := len(b.forgetDesk)
			b.mu.Unlock()
			if left != 0 {
				t.Fatalf("%d forget marks left after the answer", left)
			}
		})
	}
}

// An old machine's strict decoder refuses an end that carries forgetDesk; the
// wire form of a plain end is unchanged.
func TestEndWireFormUnchangedWithoutForget(t *testing.T) {
	data, err := json.Marshal(terminalproto.End{})
	if err != nil || string(data) != "{}" {
		t.Fatalf("plain end encodes as %s (%v)", data, err)
	}
}

func forgetMarks(b *Broker) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.forgetDesk)
}

func waitNoForgetMarks(t *testing.T, b *Broker) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for forgetMarks(b) != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("%d forget marks left", forgetMarks(b))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// End the shell on a desk pane whose shell already exited (the person typed
// exit) still removes the pane's history on a desk-shell-v1 machine.
func TestForgetSentForExitedDeskShell(t *testing.T) {
	b, p, uid, agent, ends := endRecorderWith(t, true, nil)
	s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	code := 0
	if err := p.Receive(terminalproto.Message{Version: terminalproto.Version, RequestID: ID(), SessionID: s.ID, Generation: p.Generation, Body: &terminalproto.SessionChanged{State: terminalproto.Exited, ExitCode: &code}}); err != nil {
		t.Fatal(err)
	}
	p.settleObservations(5e9)
	if got, err := b.st.TerminalSessionFor(uid, s.ID); err != nil || got.State != terminalproto.Exited {
		t.Fatalf("session not exited: %+v %v", got, err)
	}
	if _, err := b.EndDeskPane(t.Context(), uid, s.ID, ID(), true); err != nil {
		t.Fatal(err)
	}
	got := ends()
	if len(got) != 1 || !got[0].ForgetDesk {
		t.Fatalf("ends sent %+v, want one with ForgetDesk", got)
	}
	waitNoForgetMarks(t, b)
}

// A mark is dropped once the machine answers the end with any result, not
// only a session change.
func TestForgetMarkClearedOnAnyAnswer(t *testing.T) {
	b, _, uid, agent, ends := endRecorderWith(t, true, func(terminalproto.End) (terminalproto.Body, error) {
		return terminalproto.Result{Code: terminalproto.Failed}, nil
	})
	s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.EndDeskPane(t.Context(), uid, s.ID, ID(), true); err != nil {
		t.Fatal(err)
	}
	if got := ends(); len(got) == 0 || !got[0].ForgetDesk {
		t.Fatalf("ends sent %+v", got)
	}
	waitNoForgetMarks(t, b)
}

// A mark is dropped when the end will not be sent again: here the machine
// never answers and the shell had already exited, so nothing is retried.
func TestForgetMarkClearedWhenEndDropped(t *testing.T) {
	var mu sync.Mutex
	silent := false
	b, p, uid, agent, _ := endRecorderWith(t, true, func(terminalproto.End) (terminalproto.Body, error) {
		mu.Lock()
		defer mu.Unlock()
		if silent {
			return nil, errors.New("no answer")
		}
		return terminalproto.SessionChanged{State: terminalproto.Ended}, nil
	})
	s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	code := 0
	if err := p.Receive(terminalproto.Message{Version: terminalproto.Version, RequestID: ID(), SessionID: s.ID, Generation: p.Generation, Body: &terminalproto.SessionChanged{State: terminalproto.Exited, ExitCode: &code}}); err != nil {
		t.Fatal(err)
	}
	p.settleObservations(5e9)
	mu.Lock()
	silent = true
	mu.Unlock()
	if _, err := b.EndDeskPane(t.Context(), uid, s.ID, ID(), true); err != nil {
		t.Fatal(err)
	}
	waitNoForgetMarks(t, b)
}
