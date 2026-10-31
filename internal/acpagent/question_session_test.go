//go:build linux || darwin

package acpagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// waitFor reads events until one matches or the deadline passes.
func waitFor(t *testing.T, got *events, match func(Event) bool) Event {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case event := <-got.wait:
			if match(event) {
				return event
			}
		case <-deadline:
			t.Fatal("expected event never arrived")
		}
	}
}

func kindIs(kind string) func(Event) bool { return func(e Event) bool { return e.Kind == kind } }

// echoed returns what the fake agent received as the elicitation result.
func echoed(t *testing.T, got *events) map[string]any {
	t.Helper()
	event := waitFor(t, got, func(e Event) bool { return e.Kind == "agent" && strings.Contains(e.Text, "action") })
	var result map[string]any
	if err := json.Unmarshal([]byte(event.Text), &result); err != nil {
		t.Fatalf("echo %q: %v", event.Text, err)
	}
	return result
}

func startQuestion(t *testing.T, scenario string, questions bool) (*Session, *events, chan error) {
	t.Helper()
	opts, got := options(t, scenario, testVersion)
	opts.Questions = questions
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Close)
	done := make(chan error, 1)
	go func() { done <- session.Prompt(context.Background(), "ask me") }()
	return session, got, done
}

func answers(t *testing.T, raw string) map[string]json.RawMessage {
	t.Helper()
	parsed, err := DecodeAnswers([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestQuestionAcceptIsValidatedAndSingleUse(t *testing.T) {
	session, got, done := startQuestion(t, "question", true)
	event := waitFor(t, got, kindIs("question"))
	if event.QuestionID == "" || event.Question == nil || event.ToolCallID != "toolu_q" || len(event.Question.Fields) != 2 {
		t.Fatalf("question event %+v", event)
	}
	if event.Question.Fields[0].Key != "question_0" || event.Question.Fields[0].Kind != QuestionSingle || !event.Question.Fields[0].Required {
		t.Fatalf("field %+v", event.Question.Fields[0])
	}
	// A rejected answer must not consume the question.
	for _, bad := range []string{`{"question_0":"Option C"}`, `{"question_0":"Option A","extra":"x"}`, `{"question_0_custom":"only note"}`, `{"question_0":["Option A"]}`} {
		var answerErr *AnswerError
		if err := session.AnswerQuestion(event.QuestionID, QuestionResolution{Action: QuestionAccept, Answers: answers(t, bad)}); !errors.As(err, &answerErr) {
			t.Fatalf("%s accepted: %v", bad, err)
		}
	}
	if err := session.AnswerQuestion(event.QuestionID, QuestionResolution{Action: QuestionAccept,
		Answers: answers(t, `{"question_0":"Option A","question_0_custom":"a note"}`)}); err != nil {
		t.Fatal(err)
	}
	result := echoed(t, got)
	content, _ := result["content"].(map[string]any)
	if result["action"] != "accept" || content["question_0"] != "Option A" || content["question_0_custom"] != "a note" || len(content) != 2 {
		t.Fatalf("agent received %v", result)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	// Single use: a replay, even with a valid answer, is refused.
	if err := session.AnswerQuestion(event.QuestionID, QuestionResolution{Action: QuestionAccept, Answers: answers(t, `{"question_0":"Option B"}`)}); !errors.Is(err, ErrNoQuestion) {
		t.Fatalf("replay: %v", err)
	}
	if err := session.AnswerQuestion("forged-id", QuestionResolution{Action: QuestionDecline}); !errors.Is(err, ErrNoQuestion) {
		t.Fatalf("forged: %v", err)
	}
}

func TestQuestionDeclineAndCancelAreDistinct(t *testing.T) {
	for action, want := range map[QuestionAction]string{QuestionDecline: "decline", QuestionCancel: "cancel"} {
		t.Run(string(action), func(t *testing.T) {
			session, got, done := startQuestion(t, "question", true)
			event := waitFor(t, got, kindIs("question"))
			if err := session.AnswerQuestion(event.QuestionID, QuestionResolution{Action: action}); err != nil {
				t.Fatal(err)
			}
			result := echoed(t, got)
			if result["action"] != want || result["content"] != nil {
				t.Fatalf("agent received %v", result)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestQuestionUnsupportedFormIsCancelledWithNotice(t *testing.T) {
	_, got, done := startQuestion(t, "question-bad", true)
	if result := echoed(t, got); result["action"] != "cancel" {
		t.Fatalf("agent received %v", result)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestQuestionsAreOffByDefault(t *testing.T) {
	_, got, done := startQuestion(t, "question-off", false)
	if result := echoed(t, got); result["action"] != "cancel" {
		t.Fatalf("agent received %v", result)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestQuestionRetiresWithTheTurn(t *testing.T) {
	session, got, _ := startQuestion(t, "question", true)
	event := waitFor(t, got, kindIs("question"))
	if err := session.Cancel(); err != nil {
		t.Fatal(err)
	}
	cancelled := waitFor(t, got, kindIs("question_cancelled"))
	if cancelled.QuestionID != event.QuestionID || cancelled.Reason != "cancelled" {
		t.Fatalf("cancelled %+v", cancelled)
	}
	if err := session.AnswerQuestion(event.QuestionID, QuestionResolution{Action: QuestionAccept, Answers: answers(t, `{"question_0":"Option A"}`)}); !errors.Is(err, ErrNoQuestion) {
		t.Fatalf("answer after cancel: %v", err)
	}
}

func TestQuestionTimesOut(t *testing.T) {
	opts, got := options(t, "question", testVersion)
	opts.Questions = true
	opts.PermissionTimeout = 150 * time.Millisecond
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	go func() { _ = session.Prompt(context.Background(), "ask me") }()
	event := waitFor(t, got, kindIs("question"))
	cancelled := waitFor(t, got, kindIs("question_cancelled"))
	if cancelled.QuestionID != event.QuestionID || cancelled.Reason != "timeout" {
		t.Fatalf("cancelled %+v", cancelled)
	}
}

func TestQuestionClosesWithTheSession(t *testing.T) {
	session, got, _ := startQuestion(t, "question", true)
	event := waitFor(t, got, kindIs("question"))
	session.Close()
	cancelled := waitFor(t, got, kindIs("question_cancelled"))
	if cancelled.QuestionID != event.QuestionID {
		t.Fatalf("cancelled %+v", cancelled)
	}
}
