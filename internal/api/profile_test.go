package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

func TestPasswordChangeLeavesOnlyTheNewSession(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ts := httptest.NewServer(New(st, meta.Fetcher{}, "", WithAuthMode(AuthAccounts)))
	defer ts.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	post := func(path string, body any) *http.Response {
		raw, _ := json.Marshal(body)
		resp, err := client.Post(ts.URL+path, "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	resp := post("/api/v2/auth/setup", map[string]any{"name": "Ada", "email": "ada@example.com", "password": "old password"})
	if resp.StatusCode != 200 {
		t.Fatalf("setup: %s", resp.Status)
	}
	resp.Body.Close()
	resp = post("/api/v2/profile/password", map[string]any{"current": "old password", "password": "new password"})
	if resp.StatusCode != 200 {
		var e map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&e)
		t.Fatalf("password: %s %v", resp.Status, e)
	}
	resp.Body.Close()
	resp, err = client.Get(ts.URL + "/api/v2/profile/sessions")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("sessions: %v %v", err, resp)
	}
	var out struct {
		Sessions []struct {
			Current bool `json:"current"`
		} `json:"sessions"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	if len(out.Sessions) != 1 || !out.Sessions[0].Current {
		t.Fatalf("after a password change the sessions were %+v; want exactly the new one", out.Sessions)
	}
}
