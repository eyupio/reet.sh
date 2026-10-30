package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestTerminalSessionSeenClearsWaitingAndFinishedNeeds(t *testing.T) {
	srv, st, a, _, _ := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	cookie := terminalLogin(t, st, uid)
	srv.s.publicURL = "https://reet.example"

	caps := terminalproto.Capabilities{
		ProtocolVersion: 1,
		OS:              "linux",
		OSUser:          "tester",
		TerminalEnabled: true,
		TerminalControl: true,
		HealthEnabled:   true,
		Readiness:       terminalproto.Ready,
		Attention:       true,
	}
	gen, err := st.BeginRemoteConnectionFor(uid, a.ID, terminalbroker.ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	sess, _, err := st.ReserveTerminalCreateFor(uid, a.ID, terminalbroker.ID(), gen.Generation, terminalproto.Create{Title: "Agent run", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ObserveTerminalStateFor(uid, a.ID, sess.ID, gen.Generation, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	bellAt := time.Now().Add(-2 * time.Second).UnixMilli()
	if err := st.ObserveTerminalAttentionFor(uid, a.ID, sess.ID, gen.Generation, bellAt, bellAt); err != nil {
		t.Fatal(err)
	}

	readSessions := func() []store.TerminalSession {
		r := httptest.NewRequest(http.MethodGet, "/api/v2/terminal/sessions", nil)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("GET /api/v2/terminal/sessions: %d %s", w.Code, w.Body.String())
		}
		var out struct {
			Sessions []store.TerminalSession `json:"sessions"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.Sessions
	}

	got := readSessions()
	if len(got) != 1 || got[0].Attention != "waiting" || got[0].AttentionSource != "terminal" || !got[0].ChangedSinceSeen {
		t.Fatalf("before seen got %+v", got)
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v2/terminal/sessions/"+sess.ID+"/seen", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://reet.example")
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("POST seen: %d %s", w.Code, w.Body.String())
	}

	got = readSessions()
	if len(got) != 1 || got[0].Attention != "working" || got[0].ChangedSinceSeen {
		t.Fatalf("after seen got %+v", got)
	}
}

func TestStructuredChatPermissionResolvedRestoresWorkingAttention(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{
		"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp",
	}))
	if code != 200 {
		t.Fatalf("create: %d %s", code, body)
	}
	session, _ := out["id"].(string)

	send := func(ev terminalproto.ACPEvent) {
		if err := f.peer.Receive(terminalproto.Message{
			Version: 1, RequestID: terminalbroker.ID(), SessionID: session, Generation: f.peer.Generation,
			Body: &ev,
		}); err != nil {
			t.Fatal(err)
		}
	}
	send(terminalproto.ACPEvent{Kind: "status", State: "working"})
	send(terminalproto.ACPEvent{Kind: "permission", PermissionID: "permission-0123456789", Title: "go test", Command: "go test"})
	send(terminalproto.ACPEvent{Kind: "status", State: "waiting"})

	readOne := func() store.TerminalSession {
		r := httptest.NewRequest(http.MethodGet, "/api/v2/terminal/sessions", nil)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
		w := httptest.NewRecorder()
		f.srv.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("GET /api/v2/terminal/sessions: %d %s", w.Code, w.Body.String())
		}
		var out struct {
			Sessions []store.TerminalSession `json:"sessions"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if len(out.Sessions) != 1 {
			t.Fatalf("expected 1 session, got %d", len(out.Sessions))
		}
		return out.Sessions[0]
	}

	got := readOne()
	if got.Attention != "waiting" || got.AttentionSource != "chat" || !got.ChangedSinceSeen {
		t.Fatalf("while permission pending got %+v", got)
	}

	allow := true
	send(terminalproto.ACPEvent{Kind: "permission_resolved", PermissionID: "permission-0123456789", Allow: &allow})

	got = readOne()
	if got.Attention != "working" || got.AttentionSource != "chat" {
		t.Fatalf("after permission resolved got %+v", got)
	}
}
