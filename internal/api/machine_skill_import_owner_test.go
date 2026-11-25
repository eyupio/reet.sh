package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestAMachineCannotAddToAnotherMachinesSkillBundle(t *testing.T) {
	srv, st, a, key, token := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	b, bKey, err := st.EnrollSyncAgent(uid, "desktop", "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{a.Host, b.Host} {
		if err = st.TouchEnvironmentFor(uid, host, "agents", "", ""); err != nil {
			t.Fatal(err)
		}
	}
	post := func(agent store.SyncAgent, secret string, in store.MachineSkillImport) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(in)
		r := httptest.NewRequest("POST", "https://reet.example/api/v2/sync/skill-import", strings.NewReader(string(raw)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Reet-Agent", agent.ID)
		r.Header.Set("X-Reet-Agent-Key", secret)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		return w
	}
	first := post(a, key, apiMachineImport(t))
	if first.Code != 200 {
		t.Fatal(first.Code, first.Body.String())
	}
	var made store.PrivateSkillResult
	if err = json.Unmarshal(first.Body.Bytes(), &made); err != nil {
		t.Fatal(err)
	}

	next := apiMachineImport(t)
	next.RequestID = "http_import_request_002"
	next.BundleID = made.Bundle.ID
	head := made.Revision.ID
	next.ExpectedBase = &head
	next.Files = []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: demo\ndescription: Useful instructions\n---\nChanged.\n")}}
	m, err := skillbundle.FromContents(next.Files)
	if err != nil {
		t.Fatal(err)
	}
	next.Digest = m.Digest

	if w := post(b, bKey, next); w.Code != 409 || !strings.Contains(w.Body.String(), "did not start that skill") {
		t.Fatal("another machine:", w.Code, w.Body.String())
	}
	if w := post(a, key, next); w.Code != 200 {
		t.Fatal("the machine that started it:", w.Code, w.Body.String())
	}
}
