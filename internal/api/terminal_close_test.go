package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
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

// Stopping a session that a Factory pipeline works in can put the outcome
// away in the same action: stop following it, then archive or delete it.
func TestTerminalEndCanReleaseTheFactoryOutcomeTheSessionWorksIn(t *testing.T) {
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
	caps, err := st.BeginRemoteConnectionFor(uid, agent.ID, terminalbroker.ID(), terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "tester", TerminalEnabled: true, Readiness: terminalproto.Ready})
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := st.ReserveTerminalCreateFor(uid, agent.ID, terminalbroker.ID(), caps.Generation, terminalproto.Create{Title: "Pipeline", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := st.CreateGafferOutcomeFor(uid, store.GafferOutcome{ID: strings.Repeat("a", 32), Objective: "Ship it.", Sessions: []store.GafferOutcomeSession{{Key: "a", Title: "Part", Prompt: "Do it. Do not merge.", RequestID: strings.Repeat("8", 32)}}})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err = st.UpdateGafferOutcomeFor(uid, outcome.ID, outcome.Revision, func(o *store.GafferOutcome) bool {
		o.State, o.Sessions[0].State, o.Sessions[0].SessionID = "settled", "following", session.ID
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v2/terminal/sessions/" + session.ID + "/end"
	if w := request("POST", path, `{"requestId":"`+terminalbroker.ID()+`","removeHistory":false,"factoryOutcome":"nonsense"}`); w.Code != 400 {
		t.Fatalf("unknown action: %d %s", w.Code, w.Body.String())
	}
	if w := request("POST", path, `{"requestId":"`+terminalbroker.ID()+`","removeHistory":false,"factoryOutcome":"delete"}`); w.Code != 200 {
		t.Fatalf("end: %d %s", w.Code, w.Body.String())
	}
	if _, err := st.GafferOutcomeFor(uid, outcome.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("the outcome was not deleted with the session: %v", err)
	}
}
