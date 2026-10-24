package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestShellCommandHistoryRequiresOwnedBrowserSessionAndAvailableExecution(t *testing.T) {
	srv, st, a, _, token := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	cookie := terminalLogin(t, st, uid)
	caps, err := st.BeginRemoteConnectionFor(uid, a.ID, terminalbroker.ID(), terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "tester", TerminalControl: true, TerminalEnabled: true, Readiness: terminalproto.Ready})
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := st.ReserveTerminalCreateFor(uid, a.ID, terminalbroker.ID(), caps.Generation, terminalproto.Create{Title: "Private", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.ObserveTerminalStateFor(uid, a.ID, session.ID, caps.Generation, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateUser("history-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := st.CreateUser("history-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v2/terminal/sessions/" + session.ID + "/command-history"
	for _, tc := range []struct {
		name, cookie, token, query string
		code                       int
	}{{"no login", "", "", "", 401}, {"API token", "", token, "", 403}, {"viewer", terminalLogin(t, st, viewer.ID), "", "", 403}, {"other admin", terminalLogin(t, st, other.ID), "", "", 403}, {"query rejected", cookie, "", "?q=command", 400}, {"URL credential", cookie, "", "?ticket=unused", 400}, {"offline", cookie, "", "", 503}} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", path+tc.query, nil)
			if tc.cookie != "" {
				r.AddCookie(&http.Cookie{Name: sessionCookie, Value: tc.cookie})
			}
			if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, r)
			if w.Code != tc.code {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	srv.SetRemoteTerminals(false)
	r := httptest.NewRequest("GET", path, nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("disabled execution read host files", w.Code)
	}
}
