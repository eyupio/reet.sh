package api

import (
	"net/http"
	"strings"
	"testing"
)

// agentCall sends a request as an enrolled machine, with its own credentials.
func (tg target) agentCall(t *testing.T, method, path, id, secret, body string) reply {
	t.Helper()
	req, _ := http.NewRequest(method, tg.url+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Reet-Agent", id)
	req.Header.Set("X-Reet-Agent-Key", secret)
	return doImport(t, req)
}

func machineState(t *testing.T, tg target, id string) map[string]any {
	t.Helper()
	r := tg.fenceCall(t, "GET", fenceMachinesPath, "owner-token", "")
	if r.code != 200 {
		t.Fatalf("machines: %d %s", r.code, r.text)
	}
	list, _ := r.v["machines"].([]any)
	for _, m := range list {
		if row, _ := m.(map[string]any); row["id"] == id {
			return row
		}
	}
	t.Fatalf("machine %s not listed: %s", id, r.text)
	return nil
}

// An owner who fenced before the new address was known gives it later, with
// the fence still up and its date kept; a controller that is not fenced has
// nothing to readdress.
func TestTheAddressCanBeGivenWhileFenced(t *testing.T) {
	tg := newTarget(t, false)
	if r := tg.fenceCall(t, "PATCH", fencePath, "owner-token", `{"movedTo":"https://new.example.com"}`); r.code != http.StatusConflict {
		t.Fatalf("not fenced: %d %s", r.code, r.text)
	}
	raised := tg.fenceCall(t, "POST", fencePath, "owner-token", "")
	wantFence(t, raised, true, "")
	changed := tg.fenceCall(t, "PATCH", fencePath, "owner-token", `{"movedTo":"https://new.example.com"}`)
	wantFence(t, changed, true, "https://new.example.com")
	if changed.v["since"] != raised.v["since"] {
		t.Fatal("giving the address moved the fence's date")
	}
	if r := tg.fenceCall(t, "PATCH", fencePath, "owner-token", `{"movedTo":"ftp://x"}`); r.code != 400 {
		t.Fatalf("bad address: %d %s", r.code, r.text)
	}
	// A refused write now names the address, which is what machines follow.
	if r := tg.fenceCall(t, "POST", "/api/v2/lists", "owner-token", ""); r.code != http.StatusLocked || r.v["movedTo"] != "https://new.example.com" {
		t.Fatalf("refusal: %d %s", r.code, r.text)
	}
}

func TestOnlyTheOwnerReaddressesTheFence(t *testing.T) {
	tg := newTarget(t, true)
	tg.fenceCall(t, "POST", fencePath, "owner-token", "")
	if r := tg.fenceCall(t, "PATCH", fencePath, "member-token", `{"movedTo":"https://evil.example.com"}`); r.code != 403 {
		t.Fatalf("a member readdressed the fence: %d %s", r.code, r.text)
	}
	if r := tg.fenceCall(t, "GET", fenceMachinesPath, "member-token", ""); r.code != 403 {
		t.Fatalf("a member read the machines: %d %s", r.code, r.text)
	}
}

// The Move page counts machines from what they did: turned away by the fence
// (told, on their way) and then saying they now point at the new controller.
func TestTheFenceReportsWhichMachinesHaveMoved(t *testing.T) {
	tg := newTarget(t, false)
	owner, _, _ := tg.st.OwnerUser()
	studio, secret, _ := tg.st.EnrollSyncAgent(owner.ID, "studio", "test")
	desk, _, _ := tg.st.EnrollSyncAgent(owner.ID, "desk", "test")
	tg.fenceCall(t, "POST", fencePath, "owner-token", `{"movedTo":"https://new.example.com"}`)

	if m := machineState(t, tg, studio.ID); m["toldAt"].(float64) != 0 || m["leftAt"].(float64) != 0 {
		t.Fatalf("before any request: %v", m)
	}
	// The machine's next request is refused, and that is recorded against it.
	if r := tg.agentCall(t, "POST", "/api/v2/sync/identity", studio.ID, secret, `{}`); r.code != http.StatusLocked {
		t.Fatalf("a fenced controller took an agent's write: %d %s", r.code, r.text)
	}
	if m := machineState(t, tg, studio.ID); m["toldAt"].(float64) == 0 {
		t.Fatalf("the refusal was not recorded: %v", m)
	}
	// A wrong secret records nothing.
	tg.agentCall(t, "POST", "/api/v2/sync/identity", desk.ID, "not-its-secret", `{}`)
	if m := machineState(t, tg, desk.ID); m["toldAt"].(float64) != 0 {
		t.Fatalf("a wrong secret was recorded: %v", m)
	}
	if r := tg.agentCall(t, "POST", fenceLeftPath, desk.ID, "not-its-secret", `{"to":"https://new.example.com"}`); r.code != http.StatusUnauthorized {
		t.Fatalf("a wrong secret said it left: %d %s", r.code, r.text)
	}

	// The machine says it has gone; the fence lets that one write through.
	if r := tg.agentCall(t, "POST", fenceLeftPath, studio.ID, secret, `{"to":"https://new.example.com"}`); r.code != http.StatusNoContent {
		t.Fatalf("left: %d %s", r.code, r.text)
	}
	if m := machineState(t, tg, studio.ID); m["leftAt"].(float64) == 0 || m["leftTo"] != "https://new.example.com" {
		t.Fatalf("not counted as moved: %v", m)
	}
	for _, bad := range []string{`{}`, `{"held":"whatever"}`, `{"to":"ftp://x"}`, `{"to":"https://x.example.com","extra":1}`} {
		if r := tg.agentCall(t, "POST", fenceLeftPath, studio.ID, secret, bad); r.code != 400 {
			t.Fatalf("%s: %d %s", bad, r.code, r.text)
		}
	}
	if r := tg.agentCall(t, "POST", fenceLeftPath, desk.ID, "x", `{"held":"follow-off"}`); r.code != http.StatusUnauthorized {
		t.Fatalf("held with a wrong secret: %d %s", r.code, r.text)
	}

	// Lifting the fence ends this move, and what machines said of it.
	tg.fenceCall(t, "DELETE", fencePath, "owner-token", "")
	if m := machineState(t, tg, studio.ID); m["leftAt"].(float64) != 0 || m["toldAt"].(float64) != 0 {
		t.Fatalf("the lifted move was kept: %v", m)
	}
	if r := tg.agentCall(t, "POST", fenceLeftPath, studio.ID, secret, `{"to":"https://new.example.com"}`); r.code != http.StatusConflict {
		t.Fatalf("left an unfenced controller: %d %s", r.code, r.text)
	}
}
