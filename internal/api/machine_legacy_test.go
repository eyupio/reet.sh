package api

import (
	"encoding/json"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

func legacyFlag(t *testing.T, e enrolAccounts, id string) bool {
	t.Helper()
	agents, err := e.st.SyncAgentsFor(e.uid)
	if err != nil {
		t.Fatal(err)
	}
	r := mcpJSON(t, e.browser, e.ts, "GET", "/api/v2/sync", nil, 200)
	for _, a := range r["agents"].([]any) {
		if m := a.(map[string]any); m["id"] == id {
			flag, present := m["legacyToken"].(bool)
			for _, stored := range agents {
				if stored.ID == id && stored.LegacyToken != flag {
					t.Fatalf("listing says %v, store %v", flag, stored.LegacyToken)
				}
			}
			if !present {
				t.Fatalf("no legacyToken in %v", m)
			}
			return flag
		}
	}
	t.Fatalf("machine %s not listed", id)
	return false
}

// A machine enrolled with a personal token, and an older one still sending
// it, keep working and are marked; a request with the machine's secret alone
// clears the mark, and a machine enrolled by code never has it.
func TestMachinesStillUsingAPersonalTokenAreMarked(t *testing.T) {
	e := newEnrolAccounts(t)
	status, body := e.enrol(t, "old-laptop", "X-Reet-Token", e.token)
	var old struct {
		Agent  store.SyncAgent `json:"agent"`
		Secret string          `json:"secret"`
	}
	if status != 201 || json.Unmarshal([]byte(body), &old) != nil || !old.Agent.LegacyToken {
		t.Fatalf("token enrolment: %d %s", status, body)
	}
	if !legacyFlag(t, e, old.Agent.ID) {
		t.Fatal("token enrolment not marked")
	}
	legacy := append(machineHeaders(old.Agent.ID, old.Secret), "X-Reet-Token", e.token)
	for _, req := range machineRequests {
		if status, body := do(t, e.ts, req[0], req[1], req[2], legacy...); status == 401 || status == 403 {
			t.Errorf("%s %s by an older machine: %d %s", req[0], req[1], status, body)
		}
	}
	if status, _ := do(t, e.ts, "POST", "/api/v2/sync/poll", `{}`, machineHeaders(old.Agent.ID, old.Secret)...); status != 200 {
		t.Fatalf("poll by secret: %d", status)
	}
	if legacyFlag(t, e, old.Agent.ID) {
		t.Fatal("mark kept after a request with the secret alone")
	}
	if status, _ := do(t, e.ts, "POST", "/api/v2/sync/poll", `{}`, legacy...); status != 200 || !legacyFlag(t, e, old.Agent.ID) {
		t.Fatalf("token poll did not mark the machine: %d", status)
	}

	id, secret := e.enrolledMachine(t, "new-laptop")
	if legacyFlag(t, e, id) {
		t.Fatal("code enrolment marked")
	}
	if status, _ := do(t, e.ts, "POST", "/api/v2/sync/identity", `{}`, machineHeaders(id, secret)...); status != 200 || legacyFlag(t, e, id) {
		t.Fatalf("secret request marked the machine: %d", status)
	}
}
