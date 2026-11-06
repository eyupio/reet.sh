package terminalbroker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestReconnectResumesPersistedQueueWithoutBrowserAndNeverRetriesUncertain(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		name := "queued"
		if uncertain {
			name = "uncertain"
		}
		t.Run(name, func(t *testing.T) {
			old, _, uid, agent := chatFixtureScript(t, nil, func(c *terminalproto.Capabilities) { c.ACPLiveControls = true }, nil, nil)
			session := chatSession(t, old, uid, agent)
			id := ID()
			if err := old.st.EnqueueTerminalFor(uid, session.ID, id, "Already authorised, do not merge.", nil); err != nil {
				t.Fatal(err)
			}
			if uncertain {
				rows, _ := old.st.TerminalQueueFor(uid, session.ID)
				if ok, err := old.st.TransitionTerminalQueueFor(uid, session.ID, rows[0], "sending", "wait", "", "previous-controller"); err != nil || !ok {
					t.Fatal(ok, err)
				}
			}
			old.Close()
			if err := old.st.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := store.Open(old.st.Dir())
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			b := New(db)
			defer b.Close()
			caps, err := db.RemoteCapabilitiesFor(uid, agent)
			if err != nil {
				t.Fatal(err)
			}
			record, err := db.BeginRemoteConnectionFor(uid, agent, ID(), caps.Capabilities)
			if err != nil {
				t.Fatal(err)
			}
			var peer *Peer
			var prompts atomic.Int32
			peer, err = b.Connect(uid, agent, record.Generation, true, func(_ context.Context, m terminalproto.Message) error {
				reply := m
				switch m.Body.(type) {
				case terminalproto.Reconcile:
					reply.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{{ID: session.ID, Title: session.Title, State: terminalproto.Running, ACPSessionID: session.ACPSessionID}}}
				case *terminalproto.ACPControl:
					reply.Body = terminalproto.ACPControlResult{Controls: terminalproto.ACPControls{Options: []terminalproto.ACPSetting{}}}
				case *terminalproto.ACPPrompt:
					prompts.Add(1)
					reply.Body = terminalproto.Result{Code: terminalproto.OK}
				default:
					reply.Body = terminalproto.Result{Code: terminalproto.OK}
				}
				raw, err := terminalproto.Encode(reply)
				if err != nil {
					return err
				}
				decoded, err := terminalproto.Decode(raw)
				if err != nil {
					return err
				}
				return peer.Receive(decoded)
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := peer.Synchronize(t.Context()); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				rows, err := db.TerminalQueueFor(uid, session.ID)
				if err != nil {
					t.Fatal(err)
				}
				if !uncertain && prompts.Load() == 1 && len(rows) == 0 {
					return
				}
				if uncertain && len(rows) == 1 && rows[0].State == "uncertain" && prompts.Load() == 0 {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			rows, _ := db.TerminalQueueFor(uid, session.ID)
			t.Fatal("reconnection did not reconcile and resume safely without a browser", prompts.Load(), rows)
		})
	}
}
