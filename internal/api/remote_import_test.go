package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillimport"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestRemoteImportAPIEnforcesOwnersAndMetadataOnlyReview(t *testing.T) {
	srv, st, a, secret, token := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	if err := st.TouchEnvironmentFor(uid, a.Host, "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	request := func(path, body, key string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("POST", "https://reet.example"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Reet-Agent", a.ID)
		r.Header.Set("X-Reet-Agent-Key", key)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		return w
	}
	instance := strings.Repeat("a", 48)
	workBody := `{"schemaVersion":1,"instanceId":"` + instance + `"}`
	if w := request("/api/v2/sync/skill-import/work", workBody, ""); w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("/api/v2/sync/skill-import/work", workBody, secret); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	in := store.RemoteImportCreate{RequestKey: "http_remote_import_001", AgentID: a.ID, EnvironmentID: store.EnvironmentIDFor(uid, a.Host, "agents", ""), Name: "demo"}
	raw, _ := json.Marshal(in)
	w := request("/api/v2/skill-imports", string(raw), "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var created store.RemoteImport
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	w = request("/api/v2/sync/skill-import/work", workBody, secret)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: demo\ndescription: Test\n---\nPRIVATE_INSTRUCTIONS\n")}}
	manifest, _ := skillbundle.FromContents(files)
	p := skillimport.Preview{SchemaVersion: 1, SourceDigest: manifest.Digest, SkillValid: true, Files: []skillimport.PreviewFile{{File: manifest.Files[0]}}}
	preview := map[string]any{"id": created.ID, "instanceId": instance, "preview": p}
	raw, _ = json.Marshal(preview)
	if w = request("/api/v2/sync/skill-import/preview", string(raw), secret); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, bad := range []string{strings.TrimSuffix(string(raw), "}") + `,"files":[]}`, string(raw) + `{}`, strings.Repeat(" ", 1<<20) + string(raw)} {
		if w = request("/api/v2/sync/skill-import/preview", bad, secret); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, method := range []string{"GET", "POST", "DELETE"} {
		if mcpAPIPath(method, "/api/v2/skill-imports") || mcpAPIPath(method, "/api/v2/sync/skill-import/transfer") {
			t.Fatal("MCP import allowlist expanded")
		}
	}
	other, err := st.CreateUser("remote-api-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.RemoteImportFor(other.ID, created.ID); err == nil {
		t.Fatal("foreign read")
	}
	requests, _, err := st.RemoteImportsFor(uid)
	if err != nil || len(requests) != 1 || requests[0].Preview != nil {
		t.Fatal("unbounded list preview", requests, err)
	}
	current, err := st.RemoteImportFor(uid, created.ID)
	if err != nil || current.Preview == nil {
		t.Fatal(current, err)
	}
	if strings.Contains(string(mustRemoteJSON(t, current)), "PRIVATE_INSTRUCTIONS") {
		t.Fatal("review returned raw bytes")
	}
	// The browser cannot send file bytes to an upload endpoint without enrollment
	// credentials; even an owned account's approval is not an agent credential.
	transfer, _ := json.Marshal(map[string]any{"id": created.ID, "instanceId": instance, "sourceDigest": manifest.Digest, "files": files})
	if w = request("/api/v2/sync/skill-import/transfer", string(transfer), ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w = request("/api/v2/sync/skill-import/transfer", string(transfer), secret); w.Code == 200 {
		t.Fatal("transfer before approval")
	}
}
func mustRemoteJSON(t *testing.T, in any) []byte {
	t.Helper()
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestMachineSkillSyncAPIRequiresOwnedReportedSkill(t *testing.T) {
	srv, st, a, secret, token := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	if err := st.TouchEnvironmentFor(uid, a.Host, "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertLocalFor(uid, "demo", store.Patch{}); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordEnvironmentFor(uid, "local/demo", a.Host, "agents", "", "", ""); err != nil {
		t.Fatal(err)
	}
	dest, _, err := st.EnrollSyncAgent(uid, "destination", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = st.TouchEnvironmentFor(uid, dest.Host, "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	env := store.EnvironmentIDFor(uid, dest.Host, "agents", "")
	if err = st.RecordInventoryFor(uid, env, true, store.MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	if _, err = st.ClaimRemoteImportFor(uid, a.ID, strings.Repeat("a", 48)); err != nil {
		t.Fatal(err)
	}
	body := `{"skillId":"local/demo","destination":"destination","requestKey":"api_machine_sync_000001"}`
	call := func(method, path, body, auth string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "https://reet.example"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		return w
	}
	if w := call("POST", "/api/v2/sync/from-machine", body, ""); w.Code == 200 {
		t.Fatal("unauthenticated transfer")
	}
	w := call("POST", "/api/v2/sync/from-machine", body, token)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var imported store.RemoteImport
	if err = json.Unmarshal(w.Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}
	if imported.SyncSkillID != "local/demo" || imported.DestinationID != env {
		t.Fatal(imported)
	}
	if w = call("POST", "/api/v2/sync/from-machine", strings.Replace(body, "local/demo", "local/absent", 1), token); w.Code == 200 {
		t.Fatal("unreported skill transfer")
	}
	if w = call("POST", "/api/v2/sync/from-machine", strings.TrimSuffix(body, "}")+`,"files":[]}`, token); w.Code != 400 {
		t.Fatal(w.Code)
	}
	// A browser's sync request does not bypass the source binary's enrollment.
	req := httptest.NewRequest("POST", "https://reet.example/api/v2/sync/skill-import/work", strings.NewReader(`{"schemaVersion":1,"instanceId":"`+strings.Repeat("a", 48)+`"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Reet-Agent", a.ID)
	req.Header.Set("X-Reet-Agent-Key", secret)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), imported.ID) {
		t.Fatal(w.Code, w.Body.String())
	}
}
