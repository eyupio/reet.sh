package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

func TestTerminalPromptLibraryAccountGatesAndRevision(t *testing.T) {
	srv, st, _, _, token := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	cookie := terminalLogin(t, st, uid)
	srv.s.publicURL = "https://reet.example"
	srv.SetRemoteTerminals(false)
	request := func(method, path, body, login, origin, bearer string) *httptest.ResponseRecorder {
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
		return w
	}
	const path = "/api/v2/terminal/prompts"
	const body = `{"title":"Fix the login","description":"A careful bug fix","body":"Fix the redirect.\nUse {{project}}.","category":"Fixes","tool":"claude","model":"opus","effort":"high","revision":0}`
	viewer, err := st.CreateUser("prompts-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, login, origin, bearer, body string
		code                              int
	}{
		{"no login", "", "https://reet.example", "", body, 401},
		{"API token", "", "https://reet.example", token, body, 403},
		{"viewer", terminalLogin(t, st, viewer.ID), "https://reet.example", "", body, 403},
		{"no origin", cookie, "", "", body, 403},
		{"foreign origin", cookie, "https://elsewhere.example", "", body, 403},
		{"unknown field", cookie, "https://reet.example", "", `{"title":"No","body":"true","execute":true}`, 400},
		{"escape control", cookie, "https://reet.example", "", `{"title":"No","body":"\u001b[2J"}`, 400},
		{"empty body", cookie, "https://reet.example", "", `{"title":"No","body":" "}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := request("POST", path, tc.body, tc.login, tc.origin, tc.bearer)
			if w.Code != tc.code {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	w := request("POST", path, body, cookie, "https://reet.example", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var item store.TerminalPrompt
	if err = json.Unmarshal(w.Body.Bytes(), &item); err != nil || item.Body != "Fix the redirect.\nUse {{project}}." || item.Category != "Fixes" {
		t.Fatal(item, err)
	}
	other, err := st.CreateUser("prompts-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	otherCookie := terminalLogin(t, st, other.ID)
	for _, method := range []string{"PATCH", "DELETE"} {
		body := `{"revision":1,"title":"Other","body":"true"}`
		if method == "DELETE" {
			body = `{"revision":1}`
		}
		w = request(method, path+"/"+item.ID, body, otherCookie, "https://reet.example", "")
		if w.Code != 403 {
			t.Fatal("foreign mutation", method, w.Code, w.Body.String())
		}
	}
	w = request("PATCH", path+"/"+item.ID, `{"revision":1,"title":"Fix the redirect","description":"Reviewed","body":"Fix the redirect.\nUse {{project}}.","category":"Fixes","tool":"claude","model":"opus","effort":"high"}`, cookie, "https://reet.example", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request("DELETE", path+"/"+item.ID, `{"revision":1}`, cookie, "https://reet.example", "")
	if w.Code != 409 {
		t.Fatal("stale delete", w.Code, w.Body.String())
	}
	large, _ := json.Marshal(map[string]any{"title": "Maximum", "body": strings.Repeat("\t", 7999) + "x", "revision": 0})
	w = request("POST", path, string(large), cookie, "https://reet.example", "")
	if w.Code != 200 {
		t.Fatal("8,000-byte escaped body accepted", w.Code, w.Body.String())
	}
	w = request("GET", path, "", cookie, "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"revision":2`) {
		t.Fatal("execution-disabled metadata", w.Code, w.Body.String())
	}
	w = request("DELETE", path+"/"+item.ID, `{"revision":2}`, cookie, "https://reet.example", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}
