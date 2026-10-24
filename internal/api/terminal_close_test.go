package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestTerminalCloseRemovalIsExplicitAndAccountDefaultIsMetadata(t *testing.T) {
	srv, st, agent, _, _ := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	cookie := terminalLogin(t, st, uid)
	srv.s.publicURL = "https://reet.example"
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "https://reet.example")
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		return w
	}
	w := request("PATCH", "/api/v2/profile", `{"terminalRemoveOnClose":true}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"terminalRemoveOnClose":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
	profile, err := st.ProfileFor(uid)
	if err != nil || !profile.TerminalRemoveOnClose {
		t.Fatal(profile, err)
	}
	caps, err := st.BeginRemoteConnectionFor(uid, agent.ID, terminalbroker.ID(), terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "tester", TerminalEnabled: true, Readiness: terminalproto.Ready})
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := st.ReserveTerminalCreateFor(uid, agent.ID, terminalbroker.ID(), caps.Generation, terminalproto.Create{Title: "Offline", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	requestID := terminalbroker.ID()
	path := "/api/v2/terminal/sessions/" + session.ID + "/end"
	w = request("POST", path, `{"requestId":"`+requestID+`","removeHistory":true}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	pending, err := st.TerminalSessionsFor(uid)
	if err != nil || len(pending) != 1 || !pending[0].RemoveHistory || pending[0].State != terminalproto.EndRequested {
		t.Fatal("hidden before confirmation", pending, err)
	}
	w = request("POST", path, `{"requestId":"`+requestID+`","removeHistory":false}`)
	if w.Code != 409 {
		t.Fatal("changed receipt accepted", w.Code, w.Body.String())
	}
	if err = st.ObserveTerminalStateFor(uid, agent.ID, session.ID, caps.Generation, terminalproto.Ended, nil); err != nil {
		t.Fatal(err)
	}
	visible, err := st.TerminalSessionsFor(uid)
	if err != nil || len(visible) != 0 {
		t.Fatal(visible, err)
	}
	if _, err = st.TerminalSessionFor(uid, session.ID); err != nil {
		t.Fatal("receipt metadata deleted", err)
	}
}
