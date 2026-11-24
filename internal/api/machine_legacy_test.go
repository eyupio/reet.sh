package api

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

func listedAgent(t *testing.T, e enrolAccounts, id string) map[string]any {
	t.Helper()
	r := mcpJSON(t, e.browser, e.ts, "GET", "/api/v2/sync", nil, 200)
	for _, a := range r["agents"].([]any) {
		if m := a.(map[string]any); m["id"] == id {
			return m
		}
	}
	t.Fatalf("machine %s not listed", id)
	return nil
}

func legacyFlag(t *testing.T, e enrolAccounts, id string) bool {
	t.Helper()
	flag, ok := listedAgent(t, e, id)["legacyToken"].(bool)
	if !ok {
		t.Fatal("no legacyToken in the listing")
	}
	return flag
}

func enrolResult(t *testing.T, send func() (int, string)) (store.SyncAgent, string) {
	t.Helper()
	status, body := send()
	var got struct {
		Agent  store.SyncAgent `json:"agent"`
		Secret string          `json:"secret"`
	}
	if status != 201 || json.Unmarshal([]byte(body), &got) != nil {
		t.Fatalf("enrolment: %d %s", status, body)
	}
	return got.Agent, got.Secret
}

func (e enrolAccounts) revokeTokens(t *testing.T) {
	t.Helper()
	list := mcpJSON(t, e.browser, e.ts, "GET", "/api/v2/tokens", nil, 200)
	for _, tok := range list["tokens"].([]any) {
		id := strconv.FormatInt(int64(tok.(map[string]any)["id"].(float64)), 10)
		if status, _ := doClient(t, e.browser, e.ts, "DELETE", "/api/v2/tokens?id="+id, ""); status != 204 {
			t.Fatalf("revoke %s: %d", id, status)
		}
	}
}

// A machine enrolled with a personal token works only while it presents that
// token, as before enrolment codes: its secret alone opens nothing, and
// revoking the token cuts it off. It stays marked throughout.
func TestATokenEnrolledMachineStopsWithItsToken(t *testing.T) {
	e := newEnrolAccounts(t)
	a, secret := enrolResult(t, func() (int, string) { return e.enrol(t, "studio", "X-Reet-Token", e.token) })
	if !a.LegacyToken || a.EnrolledVia != store.EnrolledByToken || !legacyFlag(t, e, a.ID) {
		t.Fatalf("token enrolment not marked: %+v", a)
	}
	withToken := append(machineHeaders(a.ID, secret), "X-Reet-Token", e.token)
	for _, req := range machineRequests {
		if status, body := do(t, e.ts, req.method, req.path, req.body, withToken...); status != req.want {
			t.Errorf("%s %s with its token: %d, want %d: %s", req.method, req.path, status, req.want, body)
		}
		if status, _ := do(t, e.ts, req.method, req.path, req.body, machineHeaders(a.ID, secret)...); status != 401 {
			t.Errorf("%s %s by its secret alone: %d", req.method, req.path, status)
		}
	}
	if !legacyFlag(t, e, a.ID) {
		t.Fatal("mark cleared by a request")
	}
	e.revokeTokens(t)
	for _, req := range machineRequests {
		for name, hdr := range map[string][]string{"revoked token": withToken, "secret alone": machineHeaders(a.ID, secret)} {
			if status, _ := do(t, e.ts, req.method, req.path, req.body, hdr...); status != 401 {
				t.Errorf("%s %s after revocation, %s: %d", req.method, req.path, name, status)
			}
		}
	}
	if !legacyFlag(t, e, a.ID) {
		t.Fatal("mark cleared after revocation")
	}
}

// Machines enrolled by code, or by a signed-in browser, do not depend on any
// personal token.
func TestACodeOrBrowserEnrolledMachineOutlivesTokenRevocation(t *testing.T) {
	e := newEnrolAccounts(t)
	id, secret := e.enrolledMachine(t, "studio")
	browser, browserSecret := enrolResult(t, func() (int, string) {
		return doClient(t, e.browser, e.ts, "POST", "/api/v2/sync/agents", `{"host":"desk","version":"test"}`)
	})
	if browser.EnrolledVia != store.EnrolledBySession || browser.LegacyToken {
		t.Fatalf("browser enrolment: %+v", browser)
	}
	e.revokeTokens(t)
	for _, m := range [][2]string{{id, secret}, {browser.ID, browserSecret}} {
		if status, _ := do(t, e.ts, "POST", "/api/v2/sync/poll", `{}`, machineHeaders(m[0], m[1])...); status != 200 {
			t.Fatalf("machine %s after token revocation: %d", m[0], status)
		}
		if legacyFlag(t, e, m[0]) || listedAgent(t, e, m[0])["enrolledVia"] == store.EnrolledByToken {
			t.Fatalf("machine %s marked", m[0])
		}
	}
}

// Enrolling a token-enrolled machine again with a code keeps its id, gives it
// a new secret that works alone, retires the old one, and clears the mark.
// The answer is the one a new machine gets, and the code is spent.
func TestEnrollingAgainWithACodeClearsTheMark(t *testing.T) {
	e := newEnrolAccounts(t)
	old, oldSecret := enrolResult(t, func() (int, string) { return e.enrol(t, "studio", "X-Reet-Token", e.token) })
	code := e.newCode(t)
	again, secret := enrolResult(t, func() (int, string) { return e.enrol(t, "Studio", "X-Reet-Token", code) })
	if again.ID != old.ID || again.LegacyToken || again.EnrolledVia != store.EnrolledByCode || secret == oldSecret {
		t.Fatalf("again by code: %+v", again)
	}
	if legacyFlag(t, e, old.ID) {
		t.Fatal("still marked")
	}
	if status, _ := do(t, e.ts, "POST", "/api/v2/sync/identity", `{}`, machineHeaders(old.ID, secret)...); status != 200 {
		t.Fatalf("new secret alone: %d", status)
	}
	if status, _ := do(t, e.ts, "POST", "/api/v2/sync/identity", `{}`, append(machineHeaders(old.ID, oldSecret), "X-Reet-Token", e.token)...); status != 403 {
		t.Fatalf("old secret with the token: %d", status)
	}
	if status, _ := e.enrol(t, "fresh", "X-Reet-Token", code); status != 401 {
		t.Fatalf("code reused: %d", status)
	}
	if agents, _ := e.st.SyncAgentsFor(e.uid); len(agents) != 1 {
		t.Fatalf("machines: %+v", agents)
	}
}

// A paused machine gets the same answer by its own secret as a paused machine
// with a personal token: authentication does not look at the pause, and the
// routes that must not act for a paused machine refuse it themselves.
func TestAPausedMachineIsTreatedAlikeHoweverItAuthenticates(t *testing.T) {
	e := newEnrolAccounts(t)
	codeID, codeSecret := e.enrolledMachine(t, "studio")
	old, oldSecret := enrolResult(t, func() (int, string) { return e.enrol(t, "laptop", "X-Reet-Token", e.token) })
	for _, id := range []string{codeID, old.ID} {
		mcpJSON(t, e.browser, e.ts, "PATCH", "/api/v2/sync/agents", map[string]any{"id": id, "action": "pause"}, 200)
	}
	for _, req := range machineRequests {
		if req.path == "/api/import" {
			continue // its body names the machine's own host
		}
		bySecret, _ := do(t, e.ts, req.method, req.path, req.body, machineHeaders(codeID, codeSecret)...)
		byToken, _ := do(t, e.ts, req.method, req.path, req.body, append(machineHeaders(old.ID, oldSecret), "X-Reet-Token", e.token)...)
		if bySecret != byToken || bySecret == 401 {
			t.Errorf("%s %s paused: %d by secret, %d with a token", req.method, req.path, bySecret, byToken)
		}
	}
	if status, _ := do(t, e.ts, "GET", "/api/v2/sync/skill-import", "", machineHeaders(codeID, codeSecret)...); status != 409 {
		t.Fatalf("skill import by a paused machine: %d", status)
	}
}
