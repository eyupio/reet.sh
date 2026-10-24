package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/contentobs"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestContentObservationAPIEnrollmentAndStrictMetadata(t *testing.T) {
	srv, st, _, _, token := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	a, secret, err := st.EnrollSyncAgent(uid, "observation-machine", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = st.TouchEnvironmentFor(uid, "observation-machine", "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	env := store.EnvironmentIDFor(uid, "observation-machine", "agents", "")
	if err = st.RecordInventoryFor(uid, env, true, store.MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	post := func(path, body, key string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("POST", "https://reet.example/api/v2/sync/content-observations"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Reet-Agent", a.ID)
		r.Header.Set("X-Reet-Agent-Key", key)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		return w
	}
	if w := post("/prepare", "{}", ""); w.Code != 403 {
		t.Fatal("account alone", w.Code, w.Body.String())
	}
	w := post("/prepare", "{}", secret)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var g contentobs.Grant
	if err = json.Unmarshal(w.Body.Bytes(), &g); err != nil {
		t.Fatal(err)
	}
	r := contentobs.Report{SchemaVersion: 1, ScanID: g.ScanID, ObservedAt: 1, Scopes: []contentobs.Scope{{Agent: "agents", Complete: true, Skills: []contentobs.Skill{}}}}
	raw, _ := json.Marshal(r)
	for _, body := range []string{string(raw) + "{}", strings.TrimSuffix(string(raw), "}") + `,"files":[{"path":"secret","data":"private"}]}`, strings.Repeat(" ", contentobs.MaxBody) + string(raw)} {
		if w = post("", body, secret); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w = post("?host=other", string(raw), secret); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = post("", string(raw), secret); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	v, err := st.ContentObservationFor(uid, env)
	if err != nil || v.State != "observed" {
		t.Fatal(v, err)
	}
	if w = post("", string(raw), secret); w.Code != 200 {
		t.Fatal("retry", w.Code, w.Body.String())
	}
	if err = st.ControlSyncAgent(uid, a.ID, "pause"); err != nil {
		t.Fatal(err)
	}
	if w = post("/prepare", "{}", secret); w.Code != 409 {
		t.Fatal("paused", w.Code, w.Body.String())
	}
	if err = st.ControlSyncAgent(uid, a.ID, "revoke"); err != nil {
		t.Fatal(err)
	}
	if w = post("", string(raw), secret); w.Code != 403 {
		t.Fatal("revoked", w.Code, w.Body.String())
	}
}
