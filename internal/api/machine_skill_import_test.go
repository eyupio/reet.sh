package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/store"
)

func apiMachineImport(t *testing.T) store.MachineSkillImport {
	t.Helper()
	base := ""
	files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: demo\ndescription: Useful instructions\n---\nUse this skill.\n")}}
	m, err := skillbundle.FromContents(files)
	if err != nil {
		t.Fatal(err)
	}
	return store.MachineSkillImport{SchemaVersion: 1, RequestID: "http_import_request_001", ExpectedBase: &base, Agent: "agents", Name: "demo", Digest: m.Digest, Files: files}
}
func TestMachineSkillImportAPIContractAndIsolation(t *testing.T) {
	srv, st, a, key, token := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	if err := st.TouchEnvironmentFor(uid, a.Host, "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	request := func(method, query, body, secret, cookie, origin string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "https://reet.example/api/v2/sync/skill-import"+query, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Reet-Agent", a.ID)
		r.Header.Set("X-Reet-Agent-Key", secret)
		r.Header.Set("Origin", origin)
		if cookie == "" {
			r.Header.Set("Authorization", "Bearer "+token)
		} else {
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
		}
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "", "", "", "", ""); w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("GET", "", "", key, "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"schemaVersion":1`) {
		t.Fatal(w.Code, w.Body.String())
	}
	in := apiMachineImport(t)
	raw, _ := json.Marshal(in)
	for _, body := range []string{string(raw) + `{}`, strings.TrimSuffix(string(raw), "}") + `,"origin":{"kind":"user"}}`, strings.TrimSuffix(string(raw), "}") + `,"host":"other"}`, strings.Repeat(" ", 16<<20) + string(raw)} {
		if w := request("POST", "", body, key, "", ""); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := request("POST", "?t=not-allowed", string(raw), key, "", ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := request("POST", "", string(raw), key, terminalLogin(t, st, uid), ""); w.Code != 403 {
		t.Fatal("browser origin", w.Code)
	}
	in.Digest = "changed"
	changed, _ := json.Marshal(in)
	if w := request("POST", "", string(changed), key, "", ""); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result store.PrivateSkillResult
	for i := 0; i < 2; i++ {
		w := request("POST", "", string(raw), key, "", "")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var got store.PrivateSkillResult
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			result = got
		} else if got.Revision.ID != result.Revision.ID {
			t.Fatal("duplicated revision")
		}
		if strings.Contains(w.Body.String(), `"data":`) {
			t.Fatal("receipt includes bytes")
		}
	}
	bundles, err := st.PrivateSkillBundlesFor(uid)
	if err != nil || len(bundles) != 1 {
		t.Fatal(bundles, err)
	}
	if err = st.ControlSyncAgent(uid, a.ID, "pause"); err != nil {
		t.Fatal(err)
	}
	if w := request("POST", "", string(raw), key, "", ""); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err = st.ControlSyncAgent(uid, a.ID, "revoke"); err != nil {
		t.Fatal(err)
	}
	if w := request("POST", "", string(raw), key, "", ""); w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestMachineSkillImportRequiresAccountsMode(t *testing.T) {
	for _, mode := range []AuthMode{AuthNone, AuthToken} {
		srv, _, a, key, _ := privateFixture(t, mode, "shared-token")
		r := httptest.NewRequest("GET", "https://reet.example/api/v2/sync/skill-import", nil)
		r.Header.Set("X-Reet-Token", "shared-token")
		r.Header.Set("X-Reet-Agent", a.ID)
		r.Header.Set("X-Reet-Agent-Key", key)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(mode, w.Code, w.Body.String())
		}
	}
}
