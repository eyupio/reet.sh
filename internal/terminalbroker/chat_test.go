package terminalbroker

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func chatFixture(t *testing.T) (*Broker, *Peer, int64, string) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	uid, _ := st.DefaultOwner()
	a, _, err := st.EnrollSyncAgent(uid, "chat", "test")
	if err != nil {
		t.Fatal(err)
	}
	caps := terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", HealthEnabled: true, TerminalEnabled: true, TerminalControl: true, Launch: true, Tools: []string{"opencode"}, Readiness: terminalproto.Ready, ACP: true, ACPHistory: true}
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
		switch op := m.Body.(type) {
		case terminalproto.Reconcile:
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
		case terminalproto.Create:
			s := terminalproto.SessionSnapshot{ID: m.SessionID, Title: op.Title, State: terminalproto.Running}
			sessions[s.ID] = s
			reply.Body = terminalproto.SessionChanged{State: terminalproto.Running}
			if op.Structured {
				reply.Body = terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: "ses_fake000000000000000000000001"}
			}
		case terminalproto.End:
			delete(sessions, m.SessionID)
			reply.Body = terminalproto.SessionChanged{State: terminalproto.Ended}
		case terminalproto.ACPPrompt, terminalproto.ACPCancel, terminalproto.ACPPermission:
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
		default:
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
		}
		mu.Unlock()
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

func chatSession(t *testing.T, b *Broker, uid int64, agent string) store.TerminalSession {
	t.Helper()
	s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Structured || s.ACPSessionID != "ses_fake000000000000000000000001" {
		t.Fatalf("session %+v", s)
	}
	return s
}

func TestChatPromptPublishesUserAndAcceptsMachineEvent(t *testing.T) {
	b, p, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	snapshot, _, sub, cancel := b.SubscribeChat(uid, s.ID)
	defer cancel()
	if len(snapshot) != 0 {
		t.Fatalf("new chat has %d events", len(snapshot))
	}
	if err := b.PromptChat(t.Context(), uid, s.ID, "hello", nil); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-sub:
		if event.Kind != "user" || event.Text != "hello" {
			t.Fatalf("event %+v", event)
		}
	default:
		t.Fatal("no user event")
	}
	if err := p.Receive(terminalproto.Message{Version: 1, RequestID: ID(), SessionID: s.ID, Generation: p.Generation,
		Body: &terminalproto.ACPEvent{Kind: "agent", MessageID: "msg_1", Text: "hi"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-sub:
		if event.Kind != "agent" || event.Text != "hi" || event.Seq != 2 {
			t.Fatalf("event %+v", event)
		}
	default:
		t.Fatal("no agent event")
	}
}

func TestChatRejectsPlainSessionsAndOtherAccounts(t *testing.T) {
	b, _, uid, agent := chatFixture(t)
	plain, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.PromptChat(t.Context(), uid, plain.ID, "hello", nil); !errors.Is(err, ErrACPUnsupported) {
		t.Fatalf("plain: %v", err)
	}
	s := chatSession(t, b, uid, agent)
	if err := b.PromptChat(t.Context(), uid+1, s.ID, "hello", nil); err == nil {
		t.Fatal("other account accepted")
	}
}

func TestChatRingOverflowAndReset(t *testing.T) {
	b, p, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	for i := 0; i < maxChatEvents+10; i++ {
		_ = p.Receive(terminalproto.Message{Version: 1, RequestID: ID(), SessionID: s.ID, Generation: p.Generation,
			Body: &terminalproto.ACPEvent{Kind: "notice", Text: "n"}})
	}
	snapshot, truncated, _, cancel := b.SubscribeChat(uid, s.ID)
	defer cancel()
	if !truncated || len(snapshot) > maxChatEvents || len(snapshot) == 0 {
		t.Fatalf("ring: %d truncated=%v", len(snapshot), truncated)
	}
}

func TestChatResumeMustBelongToAClosedChat(t *testing.T) {
	b, _, uid, agent := chatFixture(t)
	first := chatSession(t, b, uid, agent)
	if _, err := b.End(t.Context(), uid, first.ID, ID()); err != nil {
		t.Fatal(err)
	}
	second, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Again", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: "/tmp", ResumeACP: first.ACPSessionID})
	if err != nil {
		t.Fatal(err)
	}
	if second.ACPSessionID != first.ACPSessionID {
		t.Fatalf("resume id %q", second.ACPSessionID)
	}
	if _, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Crafted", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: "/tmp", ResumeACP: "ses_nosuch000000000000000000001"}); !errors.Is(err, ErrACPResumeNotFound) {
		t.Fatalf("crafted resume: %v", err)
	}
}

func TestChatCloseClosesSubscribers(t *testing.T) {
	b, _, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	_, _, sub, cancel := b.SubscribeChat(uid, s.ID)
	defer cancel()
	b.closeChat(uid, s.ID)
	if _, ok := <-sub; ok {
		t.Fatal("subscriber channel stayed open")
	}
}

func TestPreviewTextBoundsAndFlagsTruncation(t *testing.T) {
	got, truncated := previewText("short command", 20)
	if got != "short command" || truncated {
		t.Fatalf("short command: got %q, truncated=%v", got, truncated)
	}

	got, truncated = previewText("1234567890abcdef", 10)
	if got != "1234567890…" || !truncated {
		t.Fatalf("truncated command: got %q, truncated=%v", got, truncated)
	}

	// Rune boundary handling: ensure multi-byte character is not sliced midway.
	text := "hello 日本語 world"
	// Slicing midway through '日' (at index 8, where '日' starts at index 6 and takes 3 bytes)
	got, truncated = previewText(text, 8)
	if got != "hello …" || !truncated {
		t.Fatalf("rune boundary command: got %q, truncated=%v", got, truncated)
	}
}
