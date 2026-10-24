package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestTerminalArchiveAuthorizationAndDisabledExecution(t *testing.T) {
	srv, st, a, _, token := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	cookie := terminalLogin(t, st, uid)
	srv.s.publicURL = "https://reet.example"
	srv.SetRemoteTerminals(false)
	caps, err := st.BeginRemoteConnectionFor(uid, a.ID, terminalbroker.ID(), terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "tester", TerminalEnabled: true, Readiness: terminalproto.Ready})
	if err != nil {
		t.Fatal(err)
	}
	makeSession := func() string {
		t.Helper()
		s, _, err := st.ReserveTerminalCreateFor(uid, a.ID, terminalbroker.ID(), caps.Generation, terminalproto.Create{Title: "History", Cols: 80, Rows: 24})
		if err != nil {
			t.Fatal(err)
		}
		return s.ID
	}
	closed := makeSession()
	if err = st.ObserveTerminalStateFor(uid, a.ID, closed, caps.Generation, terminalproto.Lost, nil); err != nil {
		t.Fatal(err)
	}
	active := makeSession()
	request := func(method, path, body, login, origin, bearer string) int {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		if login != "" {
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: login})
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		return w.Code
	}
	path := "/api/v2/terminal/sessions/" + closed + "/archive"
	other, err := st.CreateUser("archive-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := st.CreateUser("archive-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, login, origin, token, path, body string
		status                                 int
	}{
		{"no cookie", "", "https://reet.example", "", path, `{}`, 401},
		{"API token", "", "https://reet.example", token, path, `{}`, 403},
		{"other account", terminalLogin(t, st, other.ID), "https://reet.example", "", path, `{}`, 403},
		{"viewer", terminalLogin(t, st, viewer.ID), "https://reet.example", "", path, `{}`, 403},
		{"missing origin", cookie, "", "", path, `{}`, 403},
		{"foreign origin", cookie, "https://evil.example", "", path, `{}`, 403},
		{"URL credential", cookie, "https://reet.example", "", path + "?ticket=bad", `{}`, 400},
		{"unknown body", cookie, "https://reet.example", "", path, `{"deleteFiles":true}`, 400},
		{"invalid ID", cookie, "https://reet.example", "", "/api/v2/terminal/sessions/short/archive", `{}`, 400},
		{"active", cookie, "https://reet.example", "", "/api/v2/terminal/sessions/" + active + "/archive", `{}`, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if code := request("POST", tc.path, tc.body, tc.login, tc.origin, tc.token); code != tc.status {
				t.Fatalf("got %d want %d", code, tc.status)
			}
		})
	}
	// The narrow metadata exception does not permit existing shell mutations.
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/api/v2/terminal/sessions", `{}`},
		{"PATCH", "/api/v2/terminal/sessions/" + active, `{"title":"Changed"}`},
		{"POST", "/api/v2/terminal/sessions/" + active + "/end", `{"requestId":"` + terminalbroker.ID() + `"}`},
		{"POST", "/api/v2/terminal/sessions/" + active + "/attachments", `{}`},
	} {
		if code := request(tc.method, tc.path, tc.body, cookie, "https://reet.example", ""); code != 403 {
			t.Fatal("disabled execution opened", tc.path, code)
		}
	}
	for i := 0; i < 2; i++ {
		if code := request("POST", path, `{}`, cookie, "https://reet.example", ""); code != 200 {
			t.Fatal("archive unavailable while disabled", code)
		}
	}
	rows, err := st.TerminalSessionsFor(uid)
	if err != nil || len(rows) != 1 || rows[0].ID != active {
		t.Fatal("archive visibility", rows, err)
	}
	retained, err := st.TerminalSessionFor(uid, closed)
	if err != nil || retained.State != terminalproto.Lost {
		t.Fatal("archive deleted identity", retained, err)
	}
	// Anonymous and shared-token deployments cannot inherit interactive metadata writes.
	for _, mode := range []AuthMode{AuthNone, AuthToken} {
		srv.s.authMode = mode
		srv.s.token = "shared"
		if code := request("POST", path, `{}`, cookie, "https://reet.example", "shared"); code != 403 {
			t.Fatal("non-account archive", mode, code)
		}
	}
}
