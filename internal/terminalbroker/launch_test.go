package terminalbroker

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type launchPeer struct {
	b       *Broker
	st      *store.Store
	uid     int64
	agent   string
	mu      sync.Mutex
	creates []terminalproto.Create
	reply   func(terminalproto.Create) terminalproto.Body
}

func (f *launchPeer) sent() []terminalproto.Create {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]terminalproto.Create(nil), f.creates...)
}

func launchFixture(t *testing.T, caps terminalproto.Capabilities) *launchPeer {
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
	record, err := st.BeginRemoteConnectionFor(uid, a.ID, ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	f := &launchPeer{b: New(st), st: st, uid: uid, agent: a.ID}
	t.Cleanup(f.b.Close)
	var p *Peer
	p, err = f.b.Connect(uid, a.ID, record.Generation, true, func(ctx context.Context, m terminalproto.Message) error {
		reply := m
		switch op := m.Body.(type) {
		case terminalproto.Reconcile:
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
		case terminalproto.Create:
			f.mu.Lock()
			f.creates = append(f.creates, op)
			respond := f.reply
			f.mu.Unlock()
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
		return p.Receive(decoded)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return f
}

func legacyCapabilities() terminalproto.Capabilities {
	return terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", HealthEnabled: true, TerminalEnabled: true, TerminalControl: true, Readiness: terminalproto.Ready}
}

func launchCapabilities(tools ...string) terminalproto.Capabilities {
	caps := legacyCapabilities()
	caps.Launch = true
	caps.Tools = tools
	return caps
}

func TestLaunchIsNeverSentToAPeerWithoutTheLaunchCapability(t *testing.T) {
	f := launchFixture(t, legacyCapabilities())
	for _, in := range []terminalproto.Create{
		{Title: "Agent", Cols: 80, Rows: 24, Tool: "claude"},
		{Title: "Agent", Cols: 80, Rows: 24, Tool: "gemini", Model: "pro", Dir: "/home/test/site", Prompt: "fix it"},
	} {
		if _, err := f.b.Create(t.Context(), f.uid, f.agent, ID(), in); !errors.Is(err, ErrLaunchUnsupported) {
			t.Fatal("launch to a legacy peer", err)
		}
	}
	if sent := f.sent(); len(sent) != 0 {
		t.Fatal("launch fields reached a legacy peer", sent)
	}
	if sessions, _ := f.st.TerminalSessionsFor(f.uid); len(sessions) != 0 {
		t.Fatal("refused launch reserved a session", sessions)
	}
	// A plain shell still works with the same old peer.
	s, err := f.b.Create(t.Context(), f.uid, f.agent, ID(), terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24})
	if err != nil || s.State != terminalproto.Running {
		t.Fatal("plain shell on a legacy peer", s, err)
	}
	if sent := f.sent(); len(sent) != 1 || sent[0] != (terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24}) {
		t.Fatal("plain create changed", sent)
	}
}

func TestLaunchSendsFieldsToALaunchPeerAndReplaysIdempotently(t *testing.T) {
	f := launchFixture(t, launchCapabilities("claude"))
	in := terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24, Tool: "claude", Model: "sonnet", Dir: "/home/test/site", Prompt: "fix the tests"}
	request := ID()
	first, started, err := f.b.CreateSession(t.Context(), f.uid, f.agent, request, in)
	if err != nil || !started || first.State != terminalproto.Running {
		t.Fatal(first, started, err)
	}
	again, started, err := f.b.CreateSession(t.Context(), f.uid, f.agent, request, in)
	if err != nil || started || again.ID != first.ID {
		t.Fatal("replay", again, started, err)
	}
	if sent := f.sent(); len(sent) != 1 || sent[0] != in {
		t.Fatal("replay dispatched again", sent)
	}
	changed := in
	changed.Prompt = "fix the other tests"
	if _, _, err := f.b.CreateSession(t.Context(), f.uid, f.agent, request, changed); !errors.Is(err, store.ErrTerminalConflict) {
		t.Fatal("changed launch field was not a conflict", err)
	}
	if len(f.sent()) != 1 {
		t.Fatal("changed request dispatched")
	}
}

func TestLaunchRefusesSingleWordPositionalPromptBeforeSending(t *testing.T) {
	f := launchFixture(t, launchCapabilities("claude", "codex", "gemini"))
	for _, tool := range []string{"claude", "codex"} {
		if _, err := f.b.Create(t.Context(), f.uid, f.agent, ID(), terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24, Tool: tool, Prompt: "purge"}); !errors.Is(err, ErrPromptUnsupported) {
			t.Fatal(tool, err)
		}
	}
	if len(f.sent()) != 0 {
		t.Fatal("single-word prompt reached the connector")
	}
	if sessions, _ := f.st.TerminalSessionsFor(f.uid); len(sessions) != 0 {
		t.Fatal("refused prompt reserved a session", sessions)
	}
	if _, err := f.b.Create(t.Context(), f.uid, f.agent, ID(), terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24, Tool: "gemini", Prompt: "hello"}); err != nil {
		t.Fatal("flag-form single word refused", err)
	}
}

func TestLaunchFailedIsReportedOnFirstAttemptAndReplay(t *testing.T) {
	f := launchFixture(t, launchCapabilities("claude"))
	f.reply = func(terminalproto.Create) terminalproto.Body {
		return terminalproto.Result{Code: terminalproto.LaunchFailed}
	}
	in := terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24, Tool: "claude", Dir: "/elsewhere"}
	request := ID()
	s, started, err := f.b.CreateSession(t.Context(), f.uid, f.agent, request, in)
	if !errors.Is(err, ErrLaunchFailed) || started || s.State == terminalproto.Running {
		t.Fatal(s, started, err)
	}
	if stored, _ := f.st.TerminalSessionFor(f.uid, s.ID); stored.State != terminalproto.Lost {
		t.Fatal("refused launch left a live record", stored)
	}
	if _, started, err = f.b.CreateSession(t.Context(), f.uid, f.agent, request, in); !errors.Is(err, ErrLaunchFailed) || started {
		t.Fatal("replay lost the launch failure", err)
	}
	if len(f.sent()) != 1 {
		t.Fatal("replay dispatched again", f.sent())
	}
	// Other refusals keep their existing outcome.
	f.reply = func(terminalproto.Create) terminalproto.Body {
		return terminalproto.Result{Code: terminalproto.Capacity}
	}
	if s, err := f.b.Create(t.Context(), f.uid, f.agent, ID(), in); err != nil || s.State != terminalproto.Lost {
		t.Fatal("capacity refusal changed", s, err)
	}
}
