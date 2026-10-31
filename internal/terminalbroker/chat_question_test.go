package terminalbroker

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const brokerQuestionID = "question-0123456789abcdef"

func brokerQuestion() *terminalproto.ACPQuestion {
	return &terminalproto.ACPQuestion{Message: "Which option?", Fields: []terminalproto.ACPQuestionField{
		{Key: "question_0", Kind: "single", Required: true, Options: []terminalproto.ACPQuestionOption{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}}},
	}}
}

// offerQuestions marks the machine and its opencode agent as question capable.
func offerQuestions(t *testing.T, b *Broker, p *Peer, uid int64, agent string) {
	t.Helper()
	record, err := b.st.RemoteCapabilitiesFor(uid, agent)
	if err != nil {
		t.Fatal(err)
	}
	caps := record.Capabilities
	caps.ACPQuestions = true
	entries := []terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", MinimumVersion: "1.19.5", State: "available",
		History: true, Model: true, Approval: true, Questions: true}}
	caps.ACPAgents = &entries
	if err := b.st.UpdateRemoteCapabilitiesFor(uid, agent, record.InstanceID, p.Generation, caps); err != nil {
		t.Fatal(err)
	}
}

func receive(t *testing.T, p *Peer, session string, event terminalproto.ACPEvent) {
	t.Helper()
	if err := p.Receive(terminalproto.Message{Version: 1, RequestID: ID(), SessionID: session, Generation: p.Generation, Body: &event}); err != nil {
		t.Fatal(err)
	}
}

func TestQuestionEventsMakeTheChatWaitAndClear(t *testing.T) {
	b, p, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	snapshot, _, sub, cancel := b.SubscribeChat(uid, s.ID)
	defer cancel()
	if len(snapshot) != 0 {
		t.Fatalf("new chat has %d events", len(snapshot))
	}
	question := terminalproto.ACPEvent{Kind: "question", QuestionID: brokerQuestionID, Question: brokerQuestion()}
	receive(t, p, s.ID, question)
	receive(t, p, s.ID, question) // a replay must not duplicate the card
	if event := <-sub; event.Kind != "question" || event.QuestionID != brokerQuestionID || event.Question == nil {
		t.Fatalf("event %+v", event)
	}
	select {
	case event := <-sub:
		t.Fatalf("duplicate question event %+v", event)
	default:
	}
	record, err := b.st.TerminalSessionFor(uid, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	b.ApplyChatAttention(uid, &record)
	if record.Attention != "waiting" {
		t.Fatalf("attention %q while a question waits", record.Attention)
	}
	receive(t, p, s.ID, terminalproto.ACPEvent{Kind: "question_resolved", QuestionID: brokerQuestionID, Reason: "accept"})
	record, _ = b.st.TerminalSessionFor(uid, s.ID)
	b.ApplyChatAttention(uid, &record)
	if record.Attention == "waiting" {
		t.Fatal("the chat kept waiting after the question was answered")
	}
}

func TestAnswerQuestionNeedsTheMachineAndAgentToOfferQuestions(t *testing.T) {
	b, _, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	if err := b.AnswerQuestionChat(t.Context(), uid, s.ID, brokerQuestionID, "decline", nil); !errors.Is(err, ErrACPQuestionUnsupported) {
		t.Fatalf("got %v", err)
	}
}

func TestAnswerQuestionDeliversAndMapsTheMachinesReply(t *testing.T) {
	var mu sync.Mutex
	var seen []*terminalproto.ACPQuestionAnswer
	reply := terminalproto.Result{Code: terminalproto.OK}
	b, p, uid, agent := chatFixtureWith(t, func(a *terminalproto.ACPQuestionAnswer) terminalproto.Result {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, a)
		return reply
	})
	s := chatSession(t, b, uid, agent)
	offerQuestions(t, b, p, uid, agent)
	answers := json.RawMessage(`{"question_0":"a"}`)

	if err := b.AnswerQuestionChat(t.Context(), uid, s.ID, brokerQuestionID, "accept", answers); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if len(seen) != 1 || seen[0].QuestionID != brokerQuestionID || seen[0].Action != "accept" || string(seen[0].Answers) != `{"question_0":"a"}` {
		t.Fatalf("machine received %+v", seen)
	}
	mu.Unlock()
	events, err := b.st.ListEventsFor(uid, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	audited := false
	for _, event := range events {
		if event.Action == "question-answer" {
			audited = true
			if raw, _ := json.Marshal(event); json.Valid(raw) && containsText(raw, "question_0") {
				t.Fatalf("the answer content was logged: %s", raw)
			}
		}
	}
	if !audited {
		t.Fatal("the answer was not audited")
	}

	mu.Lock()
	reply = terminalproto.Result{Code: terminalproto.QuestionInvalid, Detail: "answer rejected for question_0: that choice was not offered"}
	mu.Unlock()
	var invalid *QuestionInvalidError
	if err := b.AnswerQuestionChat(t.Context(), uid, s.ID, brokerQuestionID, "accept", answers); !errors.As(err, &invalid) || invalid.Detail == "" {
		t.Fatalf("got %v", err)
	}
	mu.Lock()
	reply = terminalproto.Result{Code: terminalproto.Conflict}
	mu.Unlock()
	if err := b.AnswerQuestionChat(t.Context(), uid, s.ID, brokerQuestionID, "decline", nil); !errors.Is(err, ErrQuestionGone) {
		t.Fatalf("got %v", err)
	}
	mu.Lock()
	reply = terminalproto.Result{Code: terminalproto.Failed}
	mu.Unlock()
	if err := b.AnswerQuestionChat(t.Context(), uid, s.ID, brokerQuestionID, "decline", nil); !errors.Is(err, ErrACPClosed) {
		t.Fatalf("got %v", err)
	}
}

func TestAnswerQuestionIsScopedAndShapeChecked(t *testing.T) {
	b, p, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	offerQuestions(t, b, p, uid, agent)
	if err := b.AnswerQuestionChat(t.Context(), uid+1, s.ID, brokerQuestionID, "decline", nil); err == nil {
		t.Fatal("another account answered the question")
	}
	for name, call := range map[string]func() error{
		"bad id":         func() error { return b.AnswerQuestionChat(t.Context(), uid, s.ID, "x", "decline", nil) },
		"bad action":     func() error { return b.AnswerQuestionChat(t.Context(), uid, s.ID, brokerQuestionID, "allow", nil) },
		"accept no body": func() error { return b.AnswerQuestionChat(t.Context(), uid, s.ID, brokerQuestionID, "accept", nil) },
		"decline w/ body": func() error {
			return b.AnswerQuestionChat(t.Context(), uid, s.ID, brokerQuestionID, "decline", json.RawMessage(`{}`))
		},
	} {
		if err := call(); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatalf("%s: got %v", name, err)
		}
	}
}

func containsText(raw []byte, text string) bool {
	return len(text) > 0 && string(raw) != "" && indexOf(string(raw), text) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
