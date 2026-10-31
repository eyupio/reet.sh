//go:build linux || darwin

package terminalagent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// questionChat is a fake chat process that can resolve typed questions.
type questionChat struct {
	*fakeChat
	qmu      sync.Mutex
	answered []acpagent.QuestionResolution
	ids      []string
	err      error
}

func (q *questionChat) AnswerQuestion(id string, r acpagent.QuestionResolution) error {
	q.qmu.Lock()
	q.ids = append(q.ids, id)
	q.answered = append(q.answered, r)
	err := q.err
	q.qmu.Unlock()
	return err
}

func (q *questionChat) last() (string, acpagent.QuestionResolution, int) {
	q.qmu.Lock()
	defer q.qmu.Unlock()
	if len(q.ids) == 0 {
		return "", acpagent.QuestionResolution{}, 0
	}
	return q.ids[len(q.ids)-1], q.answered[len(q.answered)-1], len(q.ids)
}

func questionHost(t *testing.T, negotiated bool) (*chatHost, *questionChat, *sent) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	out := &sent{}
	proc := &questionChat{fakeChat: &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}}
	sup := newChatSupervisor(ctx, terminalhost.ManagerOptions{Home: t.TempDir()})
	h := sup.attach(ctx, 1, out.send, true, true, negotiated)
	h.sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	h.sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
		proc.onEvent = opts.OnEvent
		return proc, nil
	}
	if _, err := h.open("request-0123456789", "session-0123456789", terminalproto.Create{
		Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: h.sup.opts.Home,
	}); err != nil {
		t.Fatal(err)
	}
	return h, proc, out
}

func (s *sent) events(kind string) []terminalproto.ACPEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var found []terminalproto.ACPEvent
	for _, m := range s.msgs {
		if e, ok := m.Body.(*terminalproto.ACPEvent); ok && e.Kind == kind {
			found = append(found, *e)
		}
	}
	return found
}

func sampleAgentQuestion() *acpagent.Question {
	return &acpagent.Question{Message: "Which option?", ToolCallID: "toolu_q", Fields: []acpagent.QuestionField{
		{Key: "question_0", Title: "Probe", Kind: acpagent.QuestionSingle, Required: true, Options: []acpagent.QuestionOption{
			{Value: "Option A", Label: "Option A"}, {Value: "Option B", Label: "Option B"}}},
	}}
}

const connectorQuestionID = "question-0123456789abcdef"

func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestQuestionReachesAControllerThatNegotiatedIt(t *testing.T) {
	_, proc, out := questionHost(t, true)
	proc.onEvent(acpagent.Event{Kind: "question", QuestionID: connectorQuestionID, ToolCallID: "toolu_q", Question: sampleAgentQuestion()})
	events := out.events("question")
	if len(events) != 1 || events[0].QuestionID != connectorQuestionID || events[0].Question == nil || !events[0].Question.Valid() ||
		events[0].Question.Fields[0].Options[1].Value != "Option B" || events[0].ToolCallID != "toolu_q" {
		t.Fatalf("question events %+v", events)
	}
	if _, _, n := proc.last(); n != 0 {
		t.Fatalf("a delivered question was cancelled")
	}
	// Resolution events pass through on the same link.
	proc.onEvent(acpagent.Event{Kind: "question_resolved", QuestionID: connectorQuestionID, Reason: "accept"})
	proc.onEvent(acpagent.Event{Kind: "question_cancelled", QuestionID: connectorQuestionID, Reason: "timeout"})
	if len(out.events("question_resolved")) != 1 || len(out.events("question_cancelled")) != 1 {
		t.Fatal("resolution events were not delivered")
	}
}

func TestQuestionIsCancelledForAControllerThatDidNotNegotiateIt(t *testing.T) {
	_, proc, out := questionHost(t, false)
	proc.onEvent(acpagent.Event{Kind: "question", QuestionID: connectorQuestionID, Question: sampleAgentQuestion()})
	waitUntil(t, "the question to be cancelled", func() bool { _, _, n := proc.last(); return n == 1 })
	id, resolution, _ := proc.last()
	if id != connectorQuestionID || resolution.Action != acpagent.QuestionCancel {
		t.Fatalf("answered %q %+v", id, resolution)
	}
	proc.onEvent(acpagent.Event{Kind: "question_cancelled", QuestionID: connectorQuestionID, Reason: "cancelled"})
	for _, kind := range []string{"question", "question_resolved", "question_cancelled"} {
		if got := out.events(kind); len(got) != 0 {
			t.Fatalf("an older controller was sent %s: %+v", kind, got)
		}
	}
}

func TestQuestionThatCannotBeSentIsCancelled(t *testing.T) {
	_, proc, out := questionHost(t, true)
	huge := sampleAgentQuestion()
	huge.Message = strings.Repeat("m", terminalproto.MaxACPQuestionMessage+1)
	proc.onEvent(acpagent.Event{Kind: "question", QuestionID: connectorQuestionID, Question: huge})
	waitUntil(t, "the question to be cancelled", func() bool { _, _, n := proc.last(); return n == 1 })
	if len(out.events("question")) != 0 {
		t.Fatal("an invalid question was sent")
	}
}

func TestQuestionAnswerRouting(t *testing.T) {
	h, proc, _ := questionHost(t, true)
	answer := func(a terminalproto.ACPQuestionAnswer) (terminalproto.Result, error) {
		return h.question("session-0123456789", &a)
	}
	accept := terminalproto.ACPQuestionAnswer{QuestionID: connectorQuestionID, Action: "accept", Answers: json.RawMessage(`{"question_0":"Option A"}`)}
	if r, err := answer(accept); err != nil || r.Code != terminalproto.OK {
		t.Fatalf("accept: %+v %v", r, err)
	}
	id, resolution, _ := proc.last()
	if id != connectorQuestionID || resolution.Action != acpagent.QuestionAccept || string(resolution.Answers["question_0"]) != `"Option A"` {
		t.Fatalf("machine received %q %+v", id, resolution)
	}
	if r, err := answer(terminalproto.ACPQuestionAnswer{QuestionID: connectorQuestionID, Action: "decline"}); err != nil || r.Code != terminalproto.OK {
		t.Fatalf("decline: %+v %v", r, err)
	}
	if _, resolution, _ := proc.last(); resolution.Action != acpagent.QuestionDecline || resolution.Answers != nil {
		t.Fatalf("decline carried answers: %+v", resolution)
	}
	// Duplicate keys are rejected before the machine sees them.
	dup := terminalproto.ACPQuestionAnswer{QuestionID: connectorQuestionID, Action: "accept", Answers: json.RawMessage(`{"a":"1","a":"2"}`)}
	before := func() int { _, _, n := proc.last(); return n }()
	if r, err := answer(dup); err != nil || r.Code != terminalproto.QuestionInvalid || before != func() int { _, _, n := proc.last(); return n }() {
		t.Fatalf("duplicate keys: %+v %v", r, err)
	}
	// A validation failure is a question_invalid result, not a dropped connection.
	proc.qmu.Lock()
	proc.err = &acpagent.AnswerError{Field: "question_0", Reason: "that choice was not offered"}
	proc.qmu.Unlock()
	r, err := answer(accept)
	if err != nil || r.Code != terminalproto.QuestionInvalid || !strings.Contains(r.Detail, "question_0") {
		t.Fatalf("invalid: %+v %v", r, err)
	}
	// A question that is no longer waiting is a conflict.
	proc.qmu.Lock()
	proc.err = acpagent.ErrNoQuestion
	proc.qmu.Unlock()
	if _, err := answer(accept); !errors.Is(err, terminalhost.ErrOperationConflict) {
		t.Fatalf("stale: %v", err)
	}
	if _, err := h.question("session-missing000000", &accept); !errors.Is(err, terminalhost.ErrSessionMissing) {
		t.Fatalf("unknown session: %v", err)
	}
}

func TestQuestionAnswerNeedsAProcessThatCanAsk(t *testing.T) {
	h, _, _ := testChatHost(t) // its fake chat has no AnswerQuestion
	if _, err := h.open("request-0123456789", "session-0123456789", terminalproto.Create{
		Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: h.sup.opts.Home,
	}); err != nil {
		t.Fatal(err)
	}
	a := terminalproto.ACPQuestionAnswer{QuestionID: connectorQuestionID, Action: "decline"}
	if _, err := h.question("session-0123456789", &a); !errors.Is(err, terminalhost.ErrOperationConflict) {
		t.Fatalf("got %v", err)
	}
}

// A question answer is refused at the dispatcher, before any chat is looked up,
// unless the connection negotiated both acp-v1 and acp-question-v1.
func TestQuestionAnswerRequiresNegotiatedFeatures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		features string
		refused  bool
	}{
		{"none", "", true},
		{"chat only", terminalproto.ACPFeature, true},
		{"question without chat", terminalproto.ACPQuestionFeature, true},
		{"both", terminalproto.ACPFeature + "," + terminalproto.ACPQuestionFeature, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := make(replies, 4)
			headers := http.Header{}
			if tc.features != "" {
				headers.Set(terminalproto.FeatureHeader, tc.features)
			}
			handle, cleanup := Factory(terminalhost.ManagerOptions{})(t.Context(), http.DefaultClient, headers, 1, got.send)
			defer cleanup()
			msg := terminalproto.Message{Version: terminalproto.Version, Generation: 1, RequestID: "request-0123456789", SessionID: "session-0123456789",
				Body: &terminalproto.ACPQuestionAnswer{QuestionID: connectorQuestionID, Action: "decline"}}
			if err := msg.Validate(); err != nil {
				t.Fatal("not a valid message", err)
			}
			err := handle(t.Context(), msg)
			if tc.refused != errors.Is(err, terminalproto.ErrInvalid) {
				t.Fatalf("features %q: err = %v, refused want %v", tc.features, err, tc.refused)
			}
		})
	}
}
