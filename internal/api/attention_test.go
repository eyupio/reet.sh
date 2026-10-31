package api

import (
	"context"
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

	// Completing the turn moves attention to finished; marking it seen dismisses
	// it, and a later connector reconciliation that advances UpdatedAt must not
	// resurrect the dismissed finished state.
	send(terminalproto.ACPEvent{Kind: "status", State: "idle"})
	got = readOne()
	if got.Attention != "finished" || !got.ChangedSinceSeen {
		t.Fatalf("after idle got %+v", got)
	}
	f.srv.s.publicURL = "https://reet.example"
	seenReq := httptest.NewRequest(http.MethodPost, "/api/v2/terminal/sessions/"+session+"/seen", strings.NewReader(`{}`))
	seenReq.Header.Set("Content-Type", "application/json")
	seenReq.Header.Set("Origin", "https://reet.example")
	seenReq.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	seenRec := httptest.NewRecorder()
	f.srv.ServeHTTP(seenRec, seenReq)
	if seenRec.Code != http.StatusOK {
		t.Fatalf("POST seen: %d %s", seenRec.Code, seenRec.Body.String())
	}
	time.Sleep(5 * time.Millisecond)
	if err := f.st.ObserveTerminalStateFor(f.uid, f.agent.ID, session, f.peer.Generation, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	got = readOne()
	if got.Attention != "quiet" || got.ChangedSinceSeen {
		t.Fatalf("after reconcile following seen got %+v", got)
	}

	// If a new permission arrives and the controller restarts (replacing the in-memory
	// broker), reconciliation restores chat attention tracking and accepts subsequent
	// ACP events without requiring a browser to open the chat stream first.
	send(terminalproto.ACPEvent{Kind: "status", State: "working"})
	send(terminalproto.ACPEvent{Kind: "permission", PermissionID: "permission-9876543210", Title: "git status", Command: "git status"})
	send(terminalproto.ACPEvent{Kind: "status", State: "waiting"})

	restarted := terminalbroker.New(f.st)
	t.Cleanup(restarted.Close)
	f.srv.s.terminals = restarted
	var peer2 *terminalbroker.Peer
	peer2, err := restarted.Connect(f.uid, f.agent.ID, f.peer.Generation+1, true, func(_ context.Context, m terminalproto.Message) error {
		if _, ok := m.Body.(terminalproto.Reconcile); ok {
			reply := m
			reply.Body = &terminalproto.Reconcile{
				Complete: true,
				Sessions: []terminalproto.SessionSnapshot{{ID: session, Title: "Chat", State: terminalproto.Running}},
			}
			return peer2.Receive(reply)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got = readOne()
	if got.Attention != "waiting" || !got.ChangedSinceSeen {
		t.Fatalf("after controller restart got %+v", got)
	}
	if err := peer2.Receive(terminalproto.Message{
		Version: 1, RequestID: terminalbroker.ID(), SessionID: session, Generation: peer2.Generation,
		Body: &terminalproto.ACPEvent{Kind: "permission_resolved", PermissionID: "permission-9876543210", Allow: &allow},
	}); err != nil {
		t.Fatal(err)
	}
	got = readOne()
	if got.Attention != "working" || got.AttentionSource != "chat" {
		t.Fatalf("after resolving permission on restarted controller got %+v", got)
	}

	// Completing the turn moves attention to finished. If the controller restarts
	// before the person sees it, the finished attention and its original completion
	// timestamp must be recovered rather than disappearing or becoming quiet.
	send2 := func(e terminalproto.ACPEvent) {
		t.Helper()
		if err := peer2.Receive(terminalproto.Message{
			Version: 1, RequestID: terminalbroker.ID(), SessionID: session, Generation: peer2.Generation,
			Body: &e,
		}); err != nil {
			t.Fatal(err)
		}
	}
	send2(terminalproto.ACPEvent{Kind: "status", State: "idle"})
	got = readOne()
	if got.Attention != "finished" || !got.ChangedSinceSeen {
		t.Fatalf("after idle on peer2 got %+v", got)
	}
	originalCompletedAt := got.UpdatedAt
	if originalCompletedAt == 0 {
		t.Fatal("expected non-zero UpdatedAt on finished attention")
	}

	// Restart controller again while unseen finished turn is pending:
	restarted2 := terminalbroker.New(f.st)
	t.Cleanup(restarted2.Close)
	f.srv.s.terminals = restarted2
	got = readOne()
	if got.Attention != "finished" || !got.ChangedSinceSeen || got.UpdatedAt != originalCompletedAt {
		t.Fatalf("after second restart with unseen completed turn got %+v, want finished with UpdatedAt=%d", got, originalCompletedAt)
	}

	// Marking seen dismisses the recovered finished state across further restarts.
	seenReq = httptest.NewRequest(http.MethodPost, "/api/v2/terminal/sessions/"+session+"/seen", strings.NewReader(`{}`))
	seenReq.Header.Set("Content-Type", "application/json")
	seenReq.Header.Set("Origin", "https://reet.example")
	seenReq.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	seenRec = httptest.NewRecorder()
	f.srv.ServeHTTP(seenRec, seenReq)
	if seenRec.Code != http.StatusOK {
		t.Fatalf("POST seen: %d %s", seenRec.Code, seenRec.Body.String())
	}
	got = readOne()
	if got.Attention != "quiet" || got.ChangedSinceSeen {
		t.Fatalf("after mark seen on restarted controller got %+v", got)
	}

	restarted3 := terminalbroker.New(f.st)
	t.Cleanup(restarted3.Close)
	f.srv.s.terminals = restarted3
	got = readOne()
	if got.Attention != "quiet" || got.ChangedSinceSeen {
		t.Fatalf("after third restart following seen got %+v", got)
	}
}
