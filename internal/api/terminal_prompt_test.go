package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestPromptEndpointAnswersTheOwnerOnly(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	code, out, raw := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "claude", "prompt": launchTestPrompt}))
	id, _ := out["id"].(string)
	if code != 200 || id == "" {
		t.Fatal("create", code, raw)
	}
	path := "/api/v2/terminal/sessions/" + id + "/prompt"

	r := httptestGet(t, f, f.cookie, path)
	if r.Code != 200 || r.Header().Get("Cache-Control") != "no-store" || !strings.Contains(r.Body.String(), `"prompt":"`+launchTestPrompt+`"`) {
		t.Fatal("owner", r.Code, r.Header(), r.Body.String())
	}

	other, err := f.st.CreateUser("prompt-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	// Another account's session looks exactly like one that does not exist.
	if r := httptestGet(t, f, terminalLogin(t, f.st, other.ID), path); r.Code != 404 || strings.Contains(r.Body.String(), launchTestPrompt) {
		t.Fatal("other account", r.Code, r.Body.String())
	}
	if r := httptestGet(t, f, f.cookie, "/api/v2/terminal/sessions/"+terminalbroker.ID()+"/prompt"); r.Code != 404 {
		t.Fatal("unknown id", r.Code, r.Body.String())
	}
	if r := httptestGet(t, f, f.cookie, "/api/v2/terminal/sessions/short/prompt"); r.Code != 400 {
		t.Fatal("malformed id", r.Code, r.Body.String())
	}

	if err = f.st.ObserveTerminalStateFor(f.uid, f.agent.ID, id, f.peer.Generation, terminalproto.Lost, nil); err != nil {
		t.Fatal(err)
	}
	if err = f.st.ArchiveTerminalFor(f.uid, id); err != nil {
		t.Fatal(err)
	}
	if r := httptestGet(t, f, f.cookie, path); r.Code != 200 || strings.TrimSpace(r.Body.String()) != `{"prompt":""}` {
		t.Fatal("archived", r.Code, r.Body.String())
	}
}

func TestLaunchAuditNeverCarriesThePrompt(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	const marker = "secret-marker-7f3"
	if code, _, raw := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "claude", "prompt": "please look at " + marker + " today"})); code != 200 {
		t.Fatal("create", code, raw)
	}
	events, err := f.st.ListEventsFor(f.uid, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(launchEvents(t, f.st, f.uid)) != 1 {
		t.Fatal("the launch was not audited")
	}
	for _, e := range events {
		if strings.Contains(e.Detail, marker) || strings.Contains(e.Action, marker) || strings.Contains(e.Source, marker) {
			t.Fatal("an audit row carries the prompt", e)
		}
	}
}

func httptestGet(t *testing.T, f *launchAPI, cookie, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("GET", path, nil)
	r.Header.Set("Origin", "https://reet.example")
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	return w
}

// Reading a prompt is controller-local metadata, so it stays available when the
// server has turned terminal execution off; the account checks still apply.
func TestPromptEndpointWorksWithTerminalsDisabledButStillChecksTheCaller(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	code, out, raw := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "claude", "prompt": launchTestPrompt}))
	id, _ := out["id"].(string)
	if code != 200 || id == "" {
		t.Fatal("create", code, raw)
	}
	f.srv.SetRemoteTerminals(false)
	path := "/api/v2/terminal/sessions/" + id + "/prompt"
	if r := httptestGet(t, f, f.cookie, path); r.Code != 200 || !strings.Contains(r.Body.String(), launchTestPrompt) {
		t.Fatal("owner with terminals disabled", r.Code, r.Body.String())
	}
	viewer, err := f.st.CreateUser("prompt-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if r := httptestGet(t, f, terminalLogin(t, f.st, viewer.ID), path); r.Code != 403 {
		t.Fatal("viewer", r.Code, r.Body.String())
	}
	r := httptest.NewRequest("GET", path, nil)
	r.Header.Set("Origin", "https://reet.example")
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	r.Header.Set("Authorization", "Bearer some-token")
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("token caller", w.Code, w.Body.String())
	}
}
