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

func TestTerminalRolloutFlagAndSecureOriginFailClosed(t *testing.T) {
	srv, st, _, _, _ := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	cookie := terminalLogin(t, st, uid)
	for _, tc := range []struct {
		enabled bool
		origin  string
		allowed bool
	}{
		{false, "https://reet.example", false}, {true, "http://reet.example", false},
		{true, "https://reet.example", true}, {true, "http://127.0.0.1:8080", true}, {true, "http://[::1]:8080", true},
		{true, "http://localhost.evil", false}, {true, "https://reet.example/path", false},
	} {
		srv.s.remoteTerminals = tc.enabled
		srv.s.publicURL = tc.origin
		if got := srv.s.terminalEnabled(); got != tc.allowed {
			t.Fatalf("enabled=%t origin=%q: %t", tc.enabled, tc.origin, got)
		}
		for _, path := range []string{"/api/v2/terminal/machines", "/api/v2/terminal/sessions"} {
			r := httptest.NewRequest("GET", path, nil)
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, r)
			if w.Code != 200 {
				t.Fatalf("read-only metadata unavailable during rollback: %d", w.Code)
			}
		}
		if tc.allowed {
			continue
		}
		for _, request := range []struct{ method, path string }{
			{"POST", "/api/v2/terminal/sessions"}, {"PATCH", "/api/v2/terminal/sessions/session"},
			{"POST", "/api/v2/terminal/sessions/session/end"}, {"POST", "/api/v2/terminal/sessions/session/attachments"},
			{"GET", "/api/v2/terminal/sessions/session/history"}, {"GET", "/api/v2/terminal/socket"},
		} {
			r := httptest.NewRequest(request.method, request.path, strings.NewReader(`{}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", tc.origin)
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, r)
			if w.Code != 403 {
				t.Fatalf("disabled route %s: %d", request.path, w.Code)
			}
		}
		r := httptest.NewRequest("GET", "/api/v2/machines/health", nil)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal("terminal flag disabled health", w.Code)
		}
	}
	defaultServer := newServer(st, deadFetcher(t), "", WithAuthMode(AuthAccounts), WithPublicURL("https://reet.example"))
	defer defaultServer.terminals.Close()
	defer defaultServer.healthHub.Close()
	if defaultServer.terminalEnabled() {
		t.Fatal("remote shells enabled by default")
	}
}

func TestTerminalHistoryIsInertDownloadableText(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v2/terminal/sessions/test/history", nil)
	w := httptest.NewRecorder()
	payload := []byte(`<script>window.location='https://evil.example'</script>`)
	serveTerminalHistory(w, r, payload)
	if w.Code != 200 || w.Body.String() != string(payload) || w.Header().Get("Content-Type") != "text/plain; charset=utf-8" || w.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("history response could execute markup", w.Code, w.Header())
	}
}
