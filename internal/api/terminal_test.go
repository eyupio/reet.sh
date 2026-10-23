package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func terminalLogin(t *testing.T, st *store.Store, uid int64) string {
	t.Helper()
	cookie := terminalbroker.ID()
	if err := st.CreateSession(auth.HashToken(cookie), uid, time.Now().Add(time.Hour).UnixMilli(), "terminal test"); err != nil {
		t.Fatal(err)
	}
	return cookie
}
func TestTerminalRequiresBrowserLoginAndCanonicalOrigin(t *testing.T) {
	srv, st, a, _, token := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	cookie := terminalLogin(t, st, uid)
	srv.s.publicURL = "https://reet.example"
	request := func(method, path, body, cookie, origin, token string) int {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
		}
		r.Header.Set("Origin", origin)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		return w.Code
	}
	for _, path := range []string{"/api/v2/terminal/machines", "/api/v2/terminal/sessions"} {
		if code := request("GET", path, "", "", "", token); code != 403 {
			t.Fatal("API token granted terminal access", code)
		}
		if code := request("GET", path, "", cookie, "", ""); code != 200 {
			t.Fatal("browser read", code)
		}
	}
	for _, origin := range []string{"", "https://evil.example", "null", "https://reet.example.evil"} {
		if code := request("POST", "/api/v2/terminal/sessions", `{}`, cookie, origin, ""); code != 403 {
			t.Fatal("invalid origin accepted", origin, code)
		}
	}
	if code := request("GET", "/api/v2/terminal/socket?ticket=secret", "", cookie, "https://reet.example", ""); code != 400 {
		t.Fatal("URL ticket accepted", code)
	}
	if code := request("GET", "/api/v2/terminal/machines", "", cookie, "", "reet_mcp_fake"); code != 401 && code != 403 {
		t.Fatal("MCP token accepted", code)
	}
	other, err := st.CreateUser("other-terminal@example.com", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	otherCookie := terminalLogin(t, st, other.ID)
	caps := terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "tester", TerminalEnabled: true, HealthEnabled: true, Readiness: terminalproto.Ready}
	generation, err := st.BeginRemoteConnectionFor(uid, a.ID, terminalbroker.ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := st.ReserveTerminalCreateFor(uid, a.ID, terminalbroker.ID(), generation.Generation, terminalproto.Create{Title: "Private", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if code := request("POST", "/api/v2/terminal/sessions/"+session.ID+"/end", `{"requestId":"`+terminalbroker.ID()+`"}`, otherCookie, "https://reet.example", ""); code != 403 {
		t.Fatal("admin accessed someone else's shell", code)
	}
	viewer, err := st.CreateUser("viewer-terminal@example.com", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if code := request("GET", "/api/v2/terminal/sessions", "", terminalLogin(t, st, viewer.ID), "", ""); code != 403 {
		t.Fatal("viewer granted terminals", code)
	}
	for _, mode := range []AuthMode{AuthNone, AuthToken} {
		srv.s.authMode = mode
		srv.s.token = "single-token"
		if code := request("GET", "/api/v2/terminal/machines", "", cookie, "", "single-token"); code != 403 {
			t.Fatal("non-account mode allowed", mode, code)
		}
	}
}

func TestStreamHelloRejectsConfusedBindings(t *testing.T) {
	h := terminalproto.StreamHello{Version: 1, RequestID: terminalbroker.ID(), SessionID: terminalbroker.ID(), AttachmentID: terminalbroker.ID(), Generation: 1, AttachmentGeneration: 1, Token: terminalbroker.ID(), Operation: "attach"}
	data, _ := json.Marshal(h)
	if _, err := terminalproto.DecodeStreamHello(data); err != nil {
		t.Fatal(err)
	}
	if _, err := terminalproto.DecodeStreamHello(append(data[:len(data)-1], []byte(`,"operation":"history"}`)...)); err == nil {
		t.Fatal("duplicate operation accepted")
	}
}
