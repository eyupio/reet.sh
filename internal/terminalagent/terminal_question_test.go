package terminalagent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func listedQuestion(fingerprint string) *terminalproto.TerminalQuestion {
	return &terminalproto.TerminalQuestion{Question: "Do you want to proceed?", Fingerprint: fingerprint,
		Options: []terminalproto.TerminalOption{{Index: 1, Label: "Yes", Selected: true}, {Index: 2, Label: "No"}}}
}

// A question goes out when it appears, changes or goes, and only on a
// connection that negotiated terminal-question-v1.
func TestQuestionSentOnChangeOnly(t *testing.T) {
	features := terminalproto.AttentionFeature + "," + terminalproto.TerminalQuestionFeature
	observed := map[string]terminalproto.SessionChanged{}
	sent := map[string]string{}
	s := terminalhost.ManagedSession{ID: "session-0123456789", State: terminalproto.Running}
	observeShell(observed, sent, s, features)

	s.Question = listedQuestion(strings.Repeat("a", 64))
	asked, send := observeShell(observed, sent, s, features)
	if !send || asked.TerminalQuestion == nil || asked.TerminalQuestion.Fingerprint != s.Question.Fingerprint {
		t.Fatalf("a new question was not sent: %+v %v", asked, send)
	}
	if again, send := observeShell(observed, sent, s, features); send || again.TerminalQuestion == nil {
		t.Fatalf("an unchanged question was news: %+v %v", again, send)
	}
	s.Question = listedQuestion(strings.Repeat("b", 64))
	if other, send := observeShell(observed, sent, s, features); !send || other.TerminalQuestion.Fingerprint != s.Question.Fingerprint {
		t.Fatalf("another question was not sent: %+v %v", other, send)
	}
	s.Question = nil
	if gone, send := observeShell(observed, sent, s, features); !send || gone.TerminalQuestion != nil {
		t.Fatalf("a question going was not sent as cleared: %+v %v", gone, send)
	}

	old := map[string]terminalproto.SessionChanged{}
	s.Question = nil
	observeShell(old, map[string]string{}, s, terminalproto.AttentionFeature)
	s.Question = listedQuestion(strings.Repeat("a", 64))
	body, send := observeShell(old, map[string]string{}, s, terminalproto.AttentionFeature)
	raw, _ := json.Marshal(body)
	if send || strings.Contains(string(raw), "proceed") {
		t.Fatalf("a question reached an older controller: %s %v", raw, send)
	}
}

// An answer is served only on a connection that negotiated the feature, and
// its refusals keep their own codes.
func TestTerminalAnswerNeedsTheFeature(t *testing.T) {
	answer := &terminalproto.TerminalAnswer{Fingerprint: strings.Repeat("a", 64), Option: 1}
	if bodyFeatureError(answer, terminalproto.ParseFeatures(terminalproto.AttentionFeature)) == nil {
		t.Fatal("an answer was served without terminal-question-v1")
	}
	if err := bodyFeatureError(answer, terminalproto.ParseFeatures(terminalproto.TerminalQuestionFeature)); err != nil {
		t.Fatal(err)
	}
	if code(terminalhost.ErrQuestionChanged) != terminalproto.QuestionChanged || code(terminalhost.ErrAlreadyAnswered) != terminalproto.AlreadyAnswered {
		t.Fatal("answer refusals lost their codes")
	}
}

// The first look after a connection is the controller's reconcile, except
// for a question, which the reconcile does not carry: a controller that
// restarted gets it back at once.
func TestFirstLookSendsOnlyAQuestion(t *testing.T) {
	plain := terminalproto.SessionChanged{State: terminalproto.Running}
	asked := terminalproto.SessionChanged{State: terminalproto.Running, TerminalQuestion: listedQuestion(strings.Repeat("a", 64))}
	if reportShell(true, true, plain) || !reportShell(true, true, asked) {
		t.Fatal("the first look reports the wrong sessions")
	}
	if !reportShell(false, true, plain) || reportShell(false, false, asked) {
		t.Fatal("a later look does not follow the change")
	}
}
