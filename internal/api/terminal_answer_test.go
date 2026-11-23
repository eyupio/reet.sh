package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// answerFixture starts a terminal session on a machine that does (or does
// not) offer terminal questions, and returns the session's answer path.
func answerFixture(t *testing.T, offer bool) (*launchAPI, string, string) {
	t.Helper()
	caps := launchCaps("claude")
	caps.Attention, caps.TerminalQuestion = true, offer
	f := launchAPIFixture(t, caps)
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "claude"}))
	if code != 200 {
		t.Fatalf("create: %d %s", code, body)
	}
	id := out["id"].(string)
	return f, id, "/api/v2/terminal/sessions/" + id + "/answer"
}

func apiQuestion(fp string) *terminalproto.TerminalQuestion {
	return &terminalproto.TerminalQuestion{Question: "Bash command\ngo test ./...\nDo you want to proceed?", Truncated: true, Fingerprint: fp,
		Options: []terminalproto.TerminalOption{{Index: 1, Label: "Yes", Selected: true}, {Index: 2, Label: "No, and", Truncated: true}}}
}

// ask has the machine report q for the session, and waits until the session
// list shows what it reported.
func (f *launchAPI) ask(t *testing.T, session string, q *terminalproto.TerminalQuestion) {
	t.Helper()
	msg := terminalproto.Message{Version: terminalproto.Version, RequestID: terminalbroker.ID(), SessionID: session, Generation: f.peer.Generation,
		Body: terminalproto.SessionChanged{State: terminalproto.Running, TerminalQuestion: q}}
	data, err := terminalproto.Encode(msg)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := terminalproto.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.peer.Receive(decoded); err != nil {
		t.Fatal(err)
	}
	want := ""
	if q != nil {
		want = q.Fingerprint
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, _, raw := f.do(t, f.cookie, "GET", "/api/v2/terminal/sessions", nil)
		if want != "" && strings.Contains(raw, want) || want == "" && !strings.Contains(raw, "terminalQuestion") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the session list never showed %q: %s", want, raw)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (f *launchAPI) answerCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.answers)
}

// The session list carries the question as an optional field, and an answer
// reaches the machine once.
func TestTerminalAnswerRoute(t *testing.T) {
	f, id, path := answerFixture(t, true)
	fp := strings.Repeat("1", 64)
	f.ask(t, id, apiQuestion(fp))
	_, out, raw := f.do(t, f.cookie, "GET", "/api/v2/terminal/sessions", nil)
	session := out["sessions"].([]any)[0].(map[string]any)
	q, ok := session["terminalQuestion"].(map[string]any)
	if !ok || q["question"] != "Bash command\ngo test ./...\nDo you want to proceed?" || q["truncated"] != true || q["fingerprint"] != fp || len(q["options"].([]any)) != 2 {
		t.Fatalf("listed: %s", raw)
	}
	if opt := q["options"].([]any)[1].(map[string]any); opt["truncated"] != true || q["options"].([]any)[0].(map[string]any)["truncated"] != nil {
		t.Fatalf("option cut marks: %s", raw)
	}
	code, out, raw := f.do(t, f.cookie, "POST", path, map[string]any{"fingerprint": fp, "option": 2})
	if code != 200 || out["answered"] != true || out["alreadyAnswered"] == true {
		t.Fatalf("answer: %d %s", code, raw)
	}
	if f.answerCount() != 1 || f.answers[0] != (terminalproto.TerminalAnswer{Fingerprint: fp, Option: 2}) {
		t.Fatalf("sent %+v", f.answers)
	}
	code, out, raw = f.do(t, f.cookie, "POST", path, map[string]any{"fingerprint": fp, "option": 2})
	if code != 200 || out["alreadyAnswered"] != true {
		t.Fatalf("repeat: %d %s", code, raw)
	}
	if f.answerCount() != 1 {
		t.Fatalf("a repeat reached the machine: %+v", f.answers)
	}
	f.ask(t, id, nil)
}

func TestTerminalAnswerRefusals(t *testing.T) {
	f, id, path := answerFixture(t, true)
	fp := strings.Repeat("2", 64)
	f.ask(t, id, apiQuestion(fp))
	f.mu.Lock()
	f.answerCode = terminalproto.QuestionChanged
	f.mu.Unlock()
	code, out, raw := f.do(t, f.cookie, "POST", path, map[string]any{"fingerprint": fp, "option": 1})
	if code != http.StatusConflict || out["code"] != "question_changed" || out["error"] != "The question on the terminal changed. Look at it again before answering." {
		t.Fatalf("changed: %d %s", code, raw)
	}
	for name, body := range map[string]any{
		"no fingerprint":   map[string]any{"option": 1},
		"a bad option":     map[string]any{"fingerprint": fp, "option": 10},
		"raw keys":         map[string]any{"fingerprint": fp, "option": 1, "keys": "y\r"},
		"option as string": map[string]any{"fingerprint": fp, "option": "1"},
	} {
		if code, _, raw := f.do(t, f.cookie, "POST", path, body); code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", name, code, raw)
		}
	}
	// Another account's session is not there, a viewer and an API token may
	// not type into a terminal.
	other, err := f.st.CreateUser("answer-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if code, _, raw := f.do(t, terminalLogin(t, f.st, other.ID), "POST", path, map[string]any{"fingerprint": fp, "option": 1}); code != http.StatusNotFound {
		t.Fatalf("other account: %d %s", code, raw)
	}
	viewer, err := f.st.CreateUser("answer-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if code, _, raw := f.do(t, terminalLogin(t, f.st, viewer.ID), "POST", path, map[string]any{"fingerprint": fp, "option": 1}); code != http.StatusForbidden {
		t.Fatalf("viewer: %d %s", code, raw)
	}
	r := httptest.NewRequest("POST", path, strings.NewReader(`{"fingerprint":"`+fp+`","option":1}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://reet.example")
	r.Header.Set("Authorization", "Bearer fixture-owner")
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("API token: %d %s", w.Code, w.Body.String())
	}
	if f.answerCount() != 1 {
		t.Fatalf("refused answers reached the machine: %+v", f.answers)
	}
}

func TestTerminalAnswerNeedsAnUpdatedMachine(t *testing.T) {
	f, _, path := answerFixture(t, false)
	code, out, raw := f.do(t, f.cookie, "POST", path, map[string]any{"fingerprint": strings.Repeat("3", 64), "option": 1})
	if code != http.StatusConflict || out["code"] != "update_required" || out["error"] != "Update reet on this machine to answer from here." {
		t.Fatalf("%d %s", code, raw)
	}
	if f.answerCount() != 0 {
		t.Fatal("an answer reached a machine without the feature")
	}
}
