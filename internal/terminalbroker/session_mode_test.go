package terminalbroker

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const switchACPID = "ses_fake000000000000000000000001"

// switchMachine is a scripted connector for session-mode tests: it answers
// creates and ends like chatFixture and records every SessionMode it is sent.
type switchMachine struct {
	mu sync.Mutex
	// reply answers a SessionMode; nil acknowledges it as running.
	reply func(p *Peer, m terminalproto.Message, in *terminalproto.SessionMode) terminalproto.Body
	// sessions is what the next reconcile reports; nil reports none.
	sessions []terminalproto.SessionSnapshot
	modes    []terminalproto.Message
}

func (m *switchMachine) sent() []terminalproto.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]terminalproto.Message(nil), m.modes...)
}

func switchFixture(t *testing.T, sessionMode bool) (*Broker, *Peer, int64, string, *switchMachine) {
	t.Helper()
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	uid, _ := st.DefaultOwner()
	a, _, err := st.EnrollSyncAgent(uid, "switch", "test")
	if err != nil {
		t.Fatal(err)
	}
	caps := terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", HealthEnabled: true, TerminalEnabled: true, TerminalControl: true, Launch: true, Tools: []string{"opencode"}, Readiness: terminalproto.Ready, ACP: true, ACPHistory: true, ACPApproval: true, ACPModel: true, SessionMode: sessionMode}
	record, err := st.BeginRemoteConnectionFor(uid, a.ID, ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	b := New(st)
	t.Cleanup(b.Close)
	machine := &switchMachine{}
	var p *Peer
	p, err = b.Connect(uid, a.ID, record.Generation, true, func(ctx context.Context, m terminalproto.Message) error {
		data, err := terminalproto.Encode(m)
		if err != nil {
			return err
		}
		in, err := terminalproto.Decode(data)
		if err != nil {
			return err
		}
		reply := in
		machine.mu.Lock()
		switch op := in.Body.(type) {
		case *terminalproto.Reconcile:
			sessions := machine.sessions
			if sessions == nil {
				sessions = []terminalproto.SessionSnapshot{}
			}
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: sessions}
		case *terminalproto.Create:
			reply.Body = terminalproto.SessionChanged{State: terminalproto.Running}
			if op.Structured {
				reply.Body = terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: switchACPID}
			}
		case *terminalproto.End:
			reply.Body = terminalproto.SessionChanged{State: terminalproto.Ended}
		case *terminalproto.SessionMode:
			machine.modes = append(machine.modes, in)
			script := machine.reply
			machine.mu.Unlock()
			reply.Body = terminalproto.SessionChanged{State: terminalproto.Running}
			if op.To == terminalproto.SessionModeChat {
				reply.Body = terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: op.ACPSessionID}
			}
			if script != nil {
				reply.Body = script(p, in, op)
			}
			machine.mu.Lock()
		default:
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
		}
		machine.mu.Unlock()
		if reply.Body == nil {
			// A script that answers nil holds the reply back.
			return nil
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
	if err = p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return b, p, uid, a.ID, machine
}

func switchChat(t *testing.T, b *Broker, uid int64, agent string) store.TerminalSession {
	t.Helper()
	s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Structured || s.ACPSessionID != switchACPID {
		t.Fatalf("session %+v", s)
	}
	return s
}

func row(t *testing.T, b *Broker, uid int64, session string) store.TerminalSession {
	t.Helper()
	s, err := b.st.TerminalSessionFor(uid, session)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// modeEvents returns the session-mode audit details, in any order.
func modeEvents(t *testing.T, b *Broker, uid int64) []map[string]string {
	t.Helper()
	events, err := b.st.ListEventsFor(uid, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]string
	for _, e := range events {
		if e.Action != "session-mode" {
			continue
		}
		meta := map[string]string{}
		if err := json.Unmarshal([]byte(e.Detail), &meta); err != nil {
			t.Fatal(err)
		}
		out = append(out, meta)
	}
	return out
}

func TestSwitchToTerminalFlipsRowAndAudits(t *testing.T) {
	b, _, uid, agent, machine := switchFixture(t, true)
	s := switchChat(t, b, uid, agent)
	if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false); err != nil {
		t.Fatal(err)
	}
	got := row(t, b, uid, s.ID)
	if got.Structured || got.State != terminalproto.Running || got.ACPSessionID != switchACPID || got.Tool != "opencode" || got.Dir != "/tmp" {
		t.Fatalf("row after switch %+v", got)
	}
	if b.chatExists(uid, s.ID) {
		t.Fatal("chat ring kept after switching to the terminal")
	}
	sent := machine.sent()
	if len(sent) != 1 {
		t.Fatalf("sent %d session_mode messages", len(sent))
	}
	in := sent[0].Body.(*terminalproto.SessionMode)
	if in.To != terminalproto.SessionModeTerminal || in.Tool != "opencode" || in.Dir != "/tmp" || in.ACPSessionID != switchACPID || in.Trust || in.Cols != switchCols || in.Rows != switchRows || sent[0].SessionID != s.ID {
		t.Fatalf("sent %+v", in)
	}
	events := modeEvents(t, b, uid)
	if len(events) != 1 {
		t.Fatalf("audit events %+v", events)
	}
	meta := events[0]
	if meta["machine"] != agent || meta["tool"] != "opencode" || meta["session"] != s.ID || meta["mode"] != terminalproto.SessionModeTerminal {
		t.Fatalf("audit meta %+v", meta)
	}
	// A second switch sends a fresh request id.
	if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeChat, false); err != nil {
		t.Fatal(err)
	}
	if sent := machine.sent(); len(sent) != 2 || sent[0].RequestID == sent[1].RequestID {
		t.Fatalf("request ids reused: %+v", sent)
	}
}

func TestSwitchToChatFlipsRowAndRestoresRing(t *testing.T) {
	b, _, uid, agent, machine := switchFixture(t, true)
	s := switchChat(t, b, uid, agent)
	if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false); err != nil {
		t.Fatal(err)
	}
	// The session/load replay streams before the ack, so the ring must exist
	// before the request is sent.
	machine.mu.Lock()
	machine.reply = func(p *Peer, m terminalproto.Message, in *terminalproto.SessionMode) terminalproto.Body {
		receive(t, p, m.SessionID, terminalproto.ACPEvent{Kind: "agent", MessageID: "msg_1", Text: "earlier answer"})
		return terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: in.ACPSessionID}
	}
	machine.mu.Unlock()
	if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeChat, true); err != nil {
		t.Fatal(err)
	}
	got := row(t, b, uid, s.ID)
	if !got.Structured || got.State != terminalproto.Running || got.ACPSessionID != switchACPID {
		t.Fatalf("row after switch %+v", got)
	}
	snapshot, _, _, cancel := b.SubscribeChat(uid, s.ID)
	defer cancel()
	if len(snapshot) != 1 || snapshot[0].Text != "earlier answer" {
		t.Fatalf("ring after switch %+v", snapshot)
	}
	sent := machine.sent()
	in := sent[len(sent)-1].Body.(*terminalproto.SessionMode)
	if in.To != terminalproto.SessionModeChat || !in.Trust || in.ACPSessionID != switchACPID {
		t.Fatalf("sent %+v", in)
	}
	if events := modeEvents(t, b, uid); len(events) != 2 || events[0]["mode"] != terminalproto.SessionModeChat && events[1]["mode"] != terminalproto.SessionModeChat {
		t.Fatalf("audit events %+v", events)
	}
	// The chat works again on the same row.
	if err := b.PromptChat(t.Context(), uid, s.ID, "hello again", nil); err != nil {
		t.Fatal(err)
	}
}

func TestSwitchRefusedWithoutACPSessionID(t *testing.T) {
	b, _, uid, agent, machine := switchFixture(t, true)
	s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "TUI", Cols: 100, Rows: 30, Tool: "opencode", Dir: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Structured || s.ACPSessionID != "" {
		t.Fatalf("session %+v", s)
	}
	err = b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeChat, true)
	if !errors.Is(err, ErrSessionModeNoSession) || err.Error() != "This session started in the OpenCode TUI, so the reet TUI cannot pick up its conversation." {
		t.Fatalf("err %v", err)
	}
	if len(machine.sent()) != 0 || row(t, b, uid, s.ID).Structured {
		t.Fatal("refused switch reached the machine or changed the row")
	}
}

func TestSwitchRefusedWhenBusyPendingApprovalOrQuestion(t *testing.T) {
	b, p, uid, agent, machine := switchFixture(t, true)
	s := switchChat(t, b, uid, agent)
	busy := []terminalproto.ACPEvent{
		{Kind: "status", State: "working"},
		{Kind: "permission", PermissionID: "permission-0123456789", Title: "run tests", ToolKind: "execute", Command: "go test ./..."},
		{Kind: "question", QuestionID: brokerQuestionID, Question: brokerQuestion()},
	}
	for _, event := range busy {
		receive(t, p, s.ID, event)
		err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false)
		if !errors.Is(err, ErrSessionModeBusy) || err.Error() != "Wait for the agent to finish or answer what it is asking, then switch." {
			t.Fatalf("%s: err %v", event.Kind, err)
		}
		receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "status", State: "idle"})
	}
	if len(machine.sent()) != 0 || !row(t, b, uid, s.ID).Structured {
		t.Fatal("busy switch reached the machine or changed the row")
	}
	// The connector's own refusal (it saw a turn start) is the same sentence,
	// and leaves the row as it was.
	machine.mu.Lock()
	machine.reply = func(*Peer, terminalproto.Message, *terminalproto.SessionMode) terminalproto.Body {
		return terminalproto.Result{Code: terminalproto.Conflict}
	}
	machine.mu.Unlock()
	if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false); !errors.Is(err, ErrSessionModeBusy) {
		t.Fatalf("connector conflict: %v", err)
	}
	if got := row(t, b, uid, s.ID); !got.Structured || len(modeEvents(t, b, uid)) != 0 {
		t.Fatalf("conflict changed the row %+v", got)
	}
	// A failed swap is a sentence and changes nothing either.
	machine.mu.Lock()
	machine.reply = func(*Peer, terminalproto.Message, *terminalproto.SessionMode) terminalproto.Body {
		return terminalproto.Result{Code: terminalproto.LaunchFailed}
	}
	machine.mu.Unlock()
	if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false); !errors.Is(err, ErrSessionModeFailed) {
		t.Fatalf("failed swap: %v", err)
	}
	if got := row(t, b, uid, s.ID); !got.Structured || len(modeEvents(t, b, uid)) != 0 {
		t.Fatalf("failure changed the row %+v", got)
	}
}

func TestSwitchRefusedWhenUnsupportedOrOffline(t *testing.T) {
	b, _, uid, agent, machine := switchFixture(t, false)
	s := switchChat(t, b, uid, agent)
	err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false)
	if !errors.Is(err, ErrSessionModeUnsupported) || err.Error() != "Update reet on this machine to switch between the reet TUI and the OpenCode TUI." {
		t.Fatalf("unsupported: %v", err)
	}
	if len(machine.sent()) != 0 || !row(t, b, uid, s.ID).Structured {
		t.Fatal("unsupported switch reached the machine or changed the row")
	}

	b, p, uid, agent, machine := switchFixture(t, true)
	s = switchChat(t, b, uid, agent)
	p.Close()
	if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false); !errors.Is(err, ErrOffline) {
		t.Fatalf("offline: %v", err)
	}
	if len(machine.sent()) != 0 || !row(t, b, uid, s.ID).Structured {
		t.Fatal("offline switch reached the machine or changed the row")
	}
	if err := b.SwitchSessionMode(t.Context(), uid, s.ID, "sideways", false); !errors.Is(err, terminalproto.ErrInvalid) {
		t.Fatalf("bad mode: %v", err)
	}
}

func TestSwitchToChatNeedsTrustWhenFolderHasPlugins(t *testing.T) {
	b, _, uid, agent, machine := switchFixture(t, true)
	s := switchChat(t, b, uid, agent)
	if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false); err != nil {
		t.Fatal(err)
	}
	const detail = "This folder has agent extensions: .opencode/plugin/x.ts. Start a chat only in a folder you trust."
	machine.mu.Lock()
	machine.reply = func(_ *Peer, _ terminalproto.Message, in *terminalproto.SessionMode) terminalproto.Body {
		if !in.Trust {
			return terminalproto.Result{Code: terminalproto.ACPTrustRequired, Detail: detail}
		}
		return terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: in.ACPSessionID}
	}
	machine.mu.Unlock()
	err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeChat, false)
	var trust *ACPTrustError
	if !errors.Is(err, ErrSessionModeTrust) || !errors.As(err, &trust) || trust.Detail != detail {
		t.Fatalf("untrusted: %v", err)
	}
	if err.Error() != "This folder has OpenCode plugins. Start only in a folder you trust." {
		t.Fatalf("trust sentence %q", err.Error())
	}
	if got := row(t, b, uid, s.ID); got.Structured {
		t.Fatalf("untrusted switch changed the row %+v", got)
	}
	if b.chatExists(uid, s.ID) {
		t.Fatal("refused switch left a chat ring behind")
	}
	if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeChat, true); err != nil {
		t.Fatal(err)
	}
	if !row(t, b, uid, s.ID).Structured {
		t.Fatal("trusted switch did not flip the row")
	}
}

func TestSwitchOtherAccountRefused(t *testing.T) {
	b, _, uid, agent, machine := switchFixture(t, true)
	s := switchChat(t, b, uid, agent)
	err := b.SwitchSessionMode(t.Context(), uid+1, s.ID, terminalproto.SessionModeTerminal, false)
	if !errors.Is(err, store.ErrRemoteNotFound) {
		t.Fatalf("other account: %v", err)
	}
	if err := b.SwitchSessionMode(t.Context(), uid, ID(), terminalproto.SessionModeTerminal, false); !errors.Is(err, store.ErrRemoteNotFound) {
		t.Fatalf("unknown session: %v", err)
	}
	if len(machine.sent()) != 0 || !row(t, b, uid, s.ID).Structured {
		t.Fatal("other account's switch reached the machine or changed the row")
	}
}

func TestReconcileAdoptsConnectorMode(t *testing.T) {
	b, p, uid, agent, machine := switchFixture(t, true)
	s := switchChat(t, b, uid, agent)
	set := func(snapshots ...terminalproto.SessionSnapshot) {
		machine.mu.Lock()
		machine.sessions = snapshots
		machine.mu.Unlock()
	}
	// The connector reports a terminal: the row follows, keeping its id.
	set(terminalproto.SessionSnapshot{ID: s.ID, Title: "Chat", State: terminalproto.Running})
	if err := p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	got := row(t, b, uid, s.ID)
	if got.Structured || got.State != terminalproto.Running || got.ACPSessionID != switchACPID {
		t.Fatalf("after terminal snapshot %+v", got)
	}
	if b.chatExists(uid, s.ID) {
		t.Fatal("chat ring kept for a terminal")
	}
	// The connector reports a chat again.
	set(terminalproto.SessionSnapshot{ID: s.ID, Title: "Chat", State: terminalproto.Running, ACPSessionID: switchACPID})
	if err := p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	got = row(t, b, uid, s.ID)
	if !got.Structured || got.ACPSessionID != switchACPID {
		t.Fatalf("after chat snapshot %+v", got)
	}
	if !b.chatExists(uid, s.ID) {
		t.Fatal("chat ring not restored")
	}
	// A terminal started in the OpenCode TUI has no id and is never made a chat.
	tui, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "TUI", Cols: 100, Rows: 30, Tool: "opencode", Dir: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	set(terminalproto.SessionSnapshot{ID: s.ID, Title: "Chat", State: terminalproto.Running, ACPSessionID: switchACPID},
		terminalproto.SessionSnapshot{ID: tui.ID, Title: "TUI", State: terminalproto.Running})
	if err := p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := row(t, b, uid, tui.ID); got.Structured || got.State != terminalproto.Running {
		t.Fatalf("tui row %+v", got)
	}
	// A closed row is never touched.
	if _, err := b.End(t.Context(), uid, s.ID, ID()); err != nil {
		t.Fatal(err)
	}
	set(terminalproto.SessionSnapshot{ID: tui.ID, Title: "TUI", State: terminalproto.Running})
	if err := p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := row(t, b, uid, s.ID); !got.Structured || got.State.ReservesQuota() || got.ACPSessionID != switchACPID {
		t.Fatalf("closed row %+v", got)
	}
}

// A connector without session-mode-v1 reports a structured chat without its id
// only when it predates acp-history-v1; such a snapshot never demotes the row.
func TestReconcileWithoutSessionModeKeepsStructured(t *testing.T) {
	b, p, uid, agent, machine := switchFixture(t, false)
	s := switchChat(t, b, uid, agent)
	machine.mu.Lock()
	machine.sessions = []terminalproto.SessionSnapshot{{ID: s.ID, Title: "Chat", State: terminalproto.Running}}
	machine.mu.Unlock()
	if err := p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := row(t, b, uid, s.ID); !got.Structured || got.ACPSessionID != switchACPID {
		t.Fatalf("row %+v", got)
	}
}

// eventually waits for cond, which the observation loop settles asynchronously.
func eventually(t *testing.T, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// The controller waits longer than the machine may take, so an answer is
// not lost to the controller's own deadline.
func TestSwitchWaitsLongerThanTheMachine(t *testing.T) {
	if sessionModeCallTimeout <= terminalproto.SessionModeTimeout {
		t.Fatalf("controller waits %v, machine may take %v", sessionModeCallTimeout, terminalproto.SessionModeTimeout)
	}
}

// A to-chat answer that came after the call gave up still settles the row:
// it carries the OpenCode session id, so the machine runs a chat.
func TestLateChatReplySettlesTheRow(t *testing.T) {
	b, p, uid, agent, machine := switchFixture(t, true)
	s := switchChat(t, b, uid, agent)
	other := switchChat(t, b, uid, agent)
	for _, id := range []string{s.ID, other.ID} {
		if err := b.SwitchSessionMode(t.Context(), uid, id, terminalproto.SessionModeTerminal, false); err != nil {
			t.Fatal(err)
		}
	}
	old := sessionModeCallTimeout
	sessionModeCallTimeout = 50 * time.Millisecond
	t.Cleanup(func() { sessionModeCallTimeout = old })
	machine.mu.Lock()
	machine.reply = func(*Peer, terminalproto.Message, *terminalproto.SessionMode) terminalproto.Body { return nil }
	machine.mu.Unlock()
	for _, id := range []string{s.ID, other.ID} {
		if err := b.SwitchSessionMode(t.Context(), uid, id, terminalproto.SessionModeChat, true); !errors.Is(err, ErrOffline) {
			t.Fatalf("held reply: %v", err)
		}
	}
	if row(t, b, uid, s.ID).Structured || b.chatExists(uid, s.ID) {
		t.Fatal("unanswered switch changed the row")
	}
	sent := machine.sent()
	late := func(m terminalproto.Message, body terminalproto.SessionChanged) {
		t.Helper()
		if err := p.Receive(terminalproto.Message{Version: 1, RequestID: m.RequestID, SessionID: m.SessionID, Generation: p.Generation, Body: &body}); err != nil {
			t.Fatal(err)
		}
	}
	// An answer without the id says nothing about a chat: the row stays a
	// terminal. The observation loop is FIFO, so once the second answer has
	// settled its row, the first has been read.
	late(sent[len(sent)-2], terminalproto.SessionChanged{State: terminalproto.Running})
	late(sent[len(sent)-1], terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: switchACPID})
	if !eventually(t, func() bool { return row(t, b, uid, other.ID).Structured }) {
		t.Fatal("late chat answer did not settle the row")
	}
	if !b.chatExists(uid, other.ID) {
		t.Fatal("late chat answer did not restore the ring")
	}
	if got := row(t, b, uid, other.ID); got.ACPSessionID != switchACPID || got.State != terminalproto.Running {
		t.Fatalf("settled row %+v", got)
	}
	if got := row(t, b, uid, s.ID); got.Structured || b.chatExists(uid, s.ID) {
		t.Fatalf("answer without an id flipped the row %+v", got)
	}
}

// Two switches of one session at once: the second is refused before it
// touches the first one's ring, and nothing is sent for it.
func TestConcurrentSwitchToChatKeepsTheFirstReplay(t *testing.T) {
	b, _, uid, agent, machine := switchFixture(t, true)
	s := switchChat(t, b, uid, agent)
	if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false); err != nil {
		t.Fatal(err)
	}
	replaying := make(chan struct{})
	release := make(chan struct{})
	machine.mu.Lock()
	calls := 0
	machine.reply = func(p *Peer, m terminalproto.Message, in *terminalproto.SessionMode) terminalproto.Body {
		if calls++; calls > 1 {
			// The machine refuses a second swap of the same id.
			return terminalproto.Result{Code: terminalproto.Conflict}
		}
		receive(t, p, m.SessionID, terminalproto.ACPEvent{Kind: "agent", MessageID: "msg_1", Text: "earlier answer"})
		close(replaying)
		<-release
		return terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: in.ACPSessionID}
	}
	machine.mu.Unlock()
	first := make(chan error, 1)
	go func() {
		first <- b.SwitchSessionMode(context.Background(), uid, s.ID, terminalproto.SessionModeChat, true)
	}()
	<-replaying
	before := len(machine.sent())
	if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeChat, true); !errors.Is(err, ErrSessionModeBusy) {
		close(release)
		t.Fatalf("second switch: %v", err)
	}
	if len(machine.sent()) != before {
		close(release)
		t.Fatal("second switch reached the machine")
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	snapshot, _, _, cancel := b.SubscribeChat(uid, s.ID)
	cancel()
	if len(snapshot) != 1 || snapshot[0].Text != "earlier answer" {
		t.Fatalf("first switch's replay lost: %+v", snapshot)
	}
	machine.mu.Lock()
	machine.reply = nil
	machine.mu.Unlock()
	if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false); err != nil {
		t.Fatalf("switch after the first finished: %v", err)
	}
}

// A row that changed under the switch (ended meanwhile) is a sentence, and the
// ring made for the replay is not left behind.
func TestSwitchStoreFailureAfterAckIsASentence(t *testing.T) {
	b, _, uid, agent, machine := switchFixture(t, true)
	s := switchChat(t, b, uid, agent)
	if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeTerminal, false); err != nil {
		t.Fatal(err)
	}
	machine.mu.Lock()
	machine.reply = func(p *Peer, m terminalproto.Message, in *terminalproto.SessionMode) terminalproto.Body {
		if err := b.st.ObserveTerminalStateFor(uid, agent, m.SessionID, p.Generation, terminalproto.Lost, nil); err != nil {
			t.Error(err)
		}
		return terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: in.ACPSessionID}
	}
	machine.mu.Unlock()
	if err := b.SwitchSessionMode(t.Context(), uid, s.ID, terminalproto.SessionModeChat, true); !errors.Is(err, ErrSessionModeFailed) {
		t.Fatalf("err %v", err)
	}
	if b.chatExists(uid, s.ID) {
		t.Fatal("ring left behind after the store refused the switch")
	}
}
