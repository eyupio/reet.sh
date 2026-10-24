package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestSyncControlsRequireAccountAndEnrollment(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	uid, _ := st.DefaultOwner()
	ownerToken, err := st.CreateAPIToken(uid, "owner", auth.HashToken("fixture-owner"))
	if err != nil {
		t.Fatal(err)
	}
	viewer, _ := st.CreateUser("viewer@sync.invalid", "Viewer", "viewer", "")
	st.CreateAPIToken(viewer.ID, "viewer", auth.HashToken("fixture-viewer"))
	member, _ := st.CreateUser("member@sync.invalid", "Member", "member", "")
	st.CreateAPIToken(member.ID, "member", auth.HashToken("fixture-member"))
	srv := httptest.NewServer(New(st, deadFetcher(t), "", WithAuthMode(AuthAccounts)))
	defer srv.Close()
	call := func(method, path, body, token string, extra ...string) (int, string) {
		headers := []string{"X-Reet-Token", token}
		headers = append(headers, extra...)
		return doClient(t, srv.Client(), srv, method, path, body, headers...)
	}
	code, body := call("POST", "/api/v2/sync/agents", `{"host":"laptop","version":"test"}`, "fixture-owner")
	if code != 201 {
		t.Fatal(code, body)
	}
	var enrolled struct {
		Agent  store.SyncAgent
		Secret string
	}
	if err = json.Unmarshal([]byte(body), &enrolled); err != nil {
		t.Fatal(err)
	}
	code, body = call("GET", "/api/v2/sync", "", "fixture-member")
	var state struct {
		Agents []store.SyncAgent
		Jobs   []store.SyncJob
	}
	json.Unmarshal([]byte(body), &state)
	if code != 200 || len(state.Agents) != 0 || len(state.Jobs) != 0 {
		t.Fatal("cross-account control state leaked", body)
	}
	for _, path := range []string{"/api/v2/sync/agents", "/api/v2/sync/jobs", "/api/v2/sync/poll", "/api/v2/sync/adopt", "/api/v2/sync/run", "/api/v2/sync/agents/update"} {
		if code, _ := call("POST", path, `{}`, "fixture-viewer"); code != 403 {
			t.Fatalf("viewer %s: %d", path, code)
		}
	}
	if code, _ := call("POST", "/api/v2/sync/poll", `{}`, "fixture-owner", "X-Reet-Agent", enrolled.Agent.ID, "X-Reet-Agent-Key", "wrong"); code != 403 {
		t.Fatal("invalid enrollment credential accepted", code)
	}
	if code, _ := call("POST", "/api/v2/sync/poll", `{}`, "fixture-member", "X-Reet-Agent", enrolled.Agent.ID, "X-Reet-Agent-Key", enrolled.Secret); code != 403 {
		t.Fatal("cross-account enrollment accepted", code)
	}
	if code, _ := call("POST", "/api/v2/sync/poll", `{}`, "fixture-owner", "X-Reet-Agent", enrolled.Agent.ID, "X-Reet-Agent-Key", enrolled.Secret); code != 200 {
		t.Fatal("valid enrollment rejected", code)
	}
	st.RevokeAPIToken(uid, ownerToken.ID)
	if code, _ := call("POST", "/api/v2/sync/poll", `{}`, "fixture-owner", "X-Reet-Agent", enrolled.Agent.ID, "X-Reet-Agent-Key", enrolled.Secret); code != 401 {
		t.Fatal("revoked personal token accepted", code)
	}
}
