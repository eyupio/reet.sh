package api

import (
	"net/http"
	"strings"
	"testing"
)

// Merging two machines that each have an installed agent is a refusal the
// person can act on, not a server error.
func TestMergingTwoEnrolledMachinesAnswers409(t *testing.T) {
	srv, st := newAccountsSrvStore(t, "")
	owner := &http.Client{Jar: mustJar(t)}
	if code, body := doClient(t, owner, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.test","password":"password-for-fixture"}`); code != 200 {
		t.Fatalf("setup %d %s", code, body)
	}
	user, _, _ := st.OwnerUser()
	for _, host := range []string{"laptop", "desktop"} {
		if _, _, err := st.EnrollSyncAgent(user.ID, host, "test"); err != nil {
			t.Fatal(err)
		}
	}
	code, body := doClient(t, owner, srv, "POST", "/api/v2/machines/merge", `{"from":"laptop","to":"desktop"}`)
	if code != http.StatusConflict || !strings.Contains(body, "revoke one enrollment before merging two installed agents") {
		t.Fatalf("merge of two enrolments: %d %s", code, body)
	}
}
