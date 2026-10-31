package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const apiQuestionID = "question-0123456789abcdef"

func questionCaps(offer bool) terminalproto.Capabilities {
	caps := chatCaps()
	if offer {
		caps.ACPQuestions = true
		entries := []terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5",
			History: true, Model: true, Approval: true, Questions: true}}
		caps.ACPAgents = &entries
	}
	return caps
}

func startQuestionChat(t *testing.T, offer bool) (*launchAPI, string) {
	t.Helper()
	caps := questionCaps(offer)
	caps.ACPModel, caps.ACPApproval, caps.ACPHistory = true, true, true
	f := launchAPIFixture(t, caps)
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp"}))
	if code != 200 {
		t.Fatalf("create: %d %s", code, body)
	}
	return f, "/api/v2/terminal/sessions/" + out["id"].(string) + "/chat/questions/"
}

func TestChatQuestionAnswerIsDeliveredWhenOffered(t *testing.T) {
	f, base := startQuestionChat(t, true)
	for name, body := range map[string]any{
		"accept":  map[string]any{"action": "accept", "answers": map[string]any{"question_0": "a"}},
		"decline": map[string]any{"action": "decline"},
		"cancel":  map[string]any{"action": "cancel", "answers": nil},
	} {
		if code, _, out := f.do(t, f.cookie, "POST", base+apiQuestionID, body); code != 200 || !strings.Contains(out, "answered") {
			t.Fatalf("%s: %d %s", name, code, out)
		}
	}
}

func TestChatQuestionAnswerIsRefusedWhenNotOffered(t *testing.T) {
	f, base := startQuestionChat(t, false)
	code, _, out := f.do(t, f.cookie, "POST", base+apiQuestionID, map[string]any{"action": "decline"})
	if code != http.StatusConflict || !strings.Contains(out, "cannot answer questions") {
		t.Fatalf("%d %s", code, out)
	}
}

func TestChatQuestionAnswerRejectsBadRequests(t *testing.T) {
	f, base := startQuestionChat(t, true)
	for name, tc := range map[string]struct {
		path string
		body any
	}{
		"bad action":            {apiQuestionID, map[string]any{"action": "allow"}},
		"unknown field":         {apiQuestionID, map[string]any{"action": "decline", "forSession": true}},
		"accept without body":   {apiQuestionID, map[string]any{"action": "accept"}},
		"decline with answers":  {apiQuestionID, map[string]any{"action": "decline", "answers": map[string]any{"a": "1"}}},
		"answers not an object": {apiQuestionID, map[string]any{"action": "accept", "answers": []string{"a"}}},
		"bad question id":       {"x", map[string]any{"action": "decline"}},
	} {
		if code, _, out := f.do(t, f.cookie, "POST", base+tc.path, tc.body); code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", name, code, out)
		}
	}
	// A duplicate key in the answers is refused before it reaches the machine.
	dup := `{"action":"accept","answers":{"a":"1","a":"2"}}`
	if code, _, out := f.do(t, f.cookie, "POST", base+apiQuestionID, dup); code != http.StatusBadRequest {
		t.Fatalf("duplicate answer keys: %d %s", code, out)
	}
}

func TestChatQuestionErrorsAreMappedForThePerson(t *testing.T) {
	s := &server{}
	for name, tc := range map[string]struct {
		err    error
		status int
		text   string
	}{
		"invalid":     {&terminalbroker.QuestionInvalidError{Detail: "answer rejected for question_0: that choice was not offered"}, 422, "question_0"},
		"gone":        {terminalbroker.ErrQuestionGone, 409, "no longer waiting"},
		"unsupported": {terminalbroker.ErrACPQuestionUnsupported, 409, "cannot answer questions"},
	} {
		rec := httptest.NewRecorder()
		s.terminalChatError(rec, tc.err)
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.text) {
			t.Fatalf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	s.terminalChatError(rec, &terminalbroker.QuestionInvalidError{Detail: "x"})
	if !strings.Contains(rec.Body.String(), `"code":"question_invalid"`) {
		t.Fatalf("no machine-readable code: %s", rec.Body.String())
	}
}
