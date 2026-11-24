package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

// enrolledMachine enrols a machine with a code and returns its id and secret.
func (e enrolAccounts) enrolledMachine(t *testing.T, host string) (string, string) {
	t.Helper()
	status, body := e.enrol(t, host, "X-Reet-Token", e.newCode(t))
	var got struct {
		Agent  store.SyncAgent `json:"agent"`
		Secret string          `json:"secret"`
	}
	if status != 201 || json.Unmarshal([]byte(body), &got) != nil {
		t.Fatalf("enrolment: %d %s", status, body)
	}
	return got.Agent.ID, got.Secret
}

func machineHeaders(id, secret string) []string {
	return []string{"X-Reet-Agent", id, "X-Reet-Agent-Key", secret, "X-Reet-Version", "test", "X-Reet-Skills-Protocol", "2"}
}

// What a machine sends after enrolment, traced from its agent: each is opened
// by the machine's own secret, with no personal token.
var machineRequests = [][3]string{
	{"POST", "/api/v2/sync/identity", `{}`},
	{"POST", "/api/v2/sync/poll", `{"skillsVersion":""}`},
	{"POST", "/api/v2/sync/result", `{"id":"no-such-job","status":"succeeded"}`},
	{"POST", "/api/v2/sync/agents/update/stage", `{"id":"no-such-job","stage":"downloading"}`},
	{"POST", "/api/v2/sync/private", `{}`},
	{"POST", "/api/v2/sync/skills/report", `{}`},
	{"POST", "/api/v2/sync/content-observations/prepare", `{}`},
	{"POST", "/api/v2/sync/content-observations", `{}`},
	{"GET", "/api/v2/sync/skill-import", ``},
	{"POST", "/api/v2/sync/skill-import", `{}`},
	{"POST", "/api/v2/sync/skill-import/work", `{}`},
	{"POST", "/api/v2/sync/skill-import/preview", `{}`},
	{"POST", "/api/v2/sync/skill-import/transfer", `{}`},
	{"POST", "/api/v2/sync/skill-import/failure", `{}`},
	{"GET", "/api/v2/skill-bundles/no-such/revisions/1", ``},
	{"POST", "/api/import", `{"items":[],"machine":{"host":"studio"}}`},
}

func TestAMachineSecretOpensItsOwnRoutesAndNoOthers(t *testing.T) {
	e := newEnrolAccounts(t)
	id, secret := e.enrolledMachine(t, "studio")
	machine := machineHeaders(id, secret)
	for _, req := range machineRequests {
		if status, body := do(t, e.ts, req[0], req[1], req[2], machine...); status == 401 || status == 403 {
			t.Errorf("%s %s by the machine: %d %s", req[0], req[1], status, body)
		}
		if status, _ := do(t, e.ts, req[0], req[1], req[2]); status != 401 {
			t.Errorf("%s %s with no credential: %d", req[0], req[1], status)
		}
		wrong := machineHeaders(id, "not-the-secret")
		if status, _ := do(t, e.ts, req[0], req[1], req[2], wrong...); status != 401 {
			t.Errorf("%s %s with a wrong secret: %d", req[0], req[1], status)
		}
	}
	if status, body := do(t, e.ts, "POST", "/api/v2/sync/identity", `{}`, machine...); status != 200 || !strings.Contains(body, id) {
		t.Fatalf("identity: %d %s", status, body)
	}
	// A sample of every other family of routes.
	for _, req := range [][3]string{
		{"GET", "/api/skills", ``}, {"POST", "/api/skills", `{"input":"a/b/c"}`}, {"PATCH", "/api/skill", `{}`},
		{"POST", "/api/v2/skills/bulk", `{}`}, {"GET", "/api/v2/skill-bundles", ``}, {"POST", "/api/v2/skill-bundles", `{}`},
		{"POST", "/api/v2/skill-imports", `{}`}, {"GET", "/api/tags", ``}, {"GET", "/api/v2/lists", ``},
		{"GET", "/api/v2/tokens", ``}, {"POST", "/api/v2/tokens", `{"name":"x"}`}, {"POST", "/api/v2/sync/enrol-codes", `{}`},
		{"GET", "/api/v2/mcp", ``}, {"GET", "/api/v2/auth/me", ``}, {"GET", "/api/v2/account", ``},
		{"GET", "/api/admin/status", ``}, {"GET", "/api/v2/admin/users", ``}, {"GET", "/api/v2/admin/backups", ``},
		{"GET", "/api/export", ``}, {"POST", "/api/restore", `{}`},
		{"GET", "/api/v2/terminal/sessions", ``}, {"POST", "/api/v2/terminal/sessions", `{}`},
		{"GET", "/api/v2/sync", ``}, {"POST", "/api/v2/sync/agents", `{"host":"another"}`}, {"PATCH", "/api/v2/sync/agents", `{"id":"` + id + `","action":"pause"}`},
		{"POST", "/api/v2/sync/agents/update", `{}`}, {"POST", "/api/v2/sync/jobs", `{}`}, {"DELETE", "/api/v2/machines", `{}`},
		{"POST", "/api/v2/machines/merge", `{}`}, {"GET", "/api/events", ``}, {"DELETE", "/api/v2/sync/poll", ``},
		{"GET", "/api/v2/skill-bundles/no-such", ``}, {"POST", "/api/v2/skill-bundles/no-such/revisions", `{}`},
	} {
		if status, body := do(t, e.ts, req[0], req[1], req[2], machine...); status != 401 {
			t.Errorf("%s %s by a machine secret: %d %s", req[0], req[1], status, body)
		}
	}
	// A revoked machine is refused everywhere.
	mcpJSON(t, e.browser, e.ts, "PATCH", "/api/v2/sync/agents", map[string]any{"id": id, "action": "revoke"}, 200)
	for _, req := range machineRequests {
		if status, _ := do(t, e.ts, req[0], req[1], req[2], machine...); status != 401 {
			t.Errorf("%s %s by a revoked machine: %d", req[0], req[1], status)
		}
	}
}
