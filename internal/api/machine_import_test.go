package api

import (
	"strings"
	"testing"
)

// A machine's own secret reports only its own inventory: a report naming
// another of the account's machines, anywhere in it, is refused and changes
// nothing.
func TestAMachineSecretReportsOnlyItsOwnInventory(t *testing.T) {
	e := newEnrolAccounts(t)
	id, secret := e.enrolledMachine(t, "studio")
	e.enrolledMachine(t, "laptop")
	machine := machineHeaders(id, secret)
	if status, body := do(t, e.ts, "POST", "/api/import", `{"items":[{"name":"kept","host":"laptop","agent":"claude"}],"environments":[{"host":"laptop","agent":"claude","skills":["kept"]}],"machine":{"host":"laptop"}}`, "X-Reet-Token", e.token); status != 200 {
		t.Fatalf("laptop's own report: %d %s", status, body)
	}
	before, _ := e.st.ListEnvironmentsFor(e.uid)
	for name, body := range map[string]string{
		"machine":         `{"items":[],"machine":{"host":"laptop","server":true}}`,
		"item":            `{"items":[{"name":"x","host":"laptop","agent":"claude"}],"machine":{"host":"studio"}}`,
		"environment":     `{"items":[],"environments":[{"host":"laptop","agent":"claude","skills":[]}],"prune":true,"machine":{"host":"studio"}}`,
		"case and spaces": `{"items":[],"environments":[{"host":" LAPTOP ","agent":"claude","skills":[]}],"prune":true,"machine":{"host":"studio"}}`,
		"no machine":      `{"items":[],"environments":[{"host":"studio","agent":"claude","skills":[]}]}`,
		"free text":       `{"text":"some-skill","machine":{"host":"studio"}}`,
	} {
		if status, text := do(t, e.ts, "POST", "/api/import", body, machine...); status != 403 || !strings.Contains(text, "only its own inventory") {
			t.Errorf("%s: %d %s", name, status, text)
		}
	}
	after, _ := e.st.ListEnvironmentsFor(e.uid)
	if len(after) != len(before) {
		t.Fatalf("environments changed: %+v -> %+v", before, after)
	}
	// Its own host, in another case, is its own.
	if status, body := do(t, e.ts, "POST", "/api/import", `{"items":[],"environments":[{"host":"Studio","agent":"claude","skills":[]}],"machine":{"host":"STUDIO"}}`, machine...); status != 200 {
		t.Fatalf("own report: %d %s", status, body)
	}
}
