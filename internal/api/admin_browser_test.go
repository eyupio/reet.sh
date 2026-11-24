package api

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
)

// adminRoute is one owner or admin route, with a body that gets past the
// gate without changing anything the rest of the test depends on.
type adminRoute struct{ method, path, body string }

// browserOnlyAdminRoutes are the owner and admin routes a personal API token
// may not call: they hand over the database or the account, or change how the
// whole instance signs people in, mails them or keeps its copies.
func browserOnlyAdminRoutes(member int64) []adminRoute {
	id := strconv.FormatInt(member, 10)
	return []adminRoute{
		{"PATCH", "/api/v2/admin/users", `{"id":0}`},
		{"DELETE", "/api/v2/admin/users?id=0", ""},
		{"POST", "/api/v2/admin/users/reset", `{"id":0}`},
		{"POST", "/api/v2/admin/users/revoke", `{"id":` + id + `}`},
		{"POST", "/api/v2/admin/users/transfer", `{"id":0}`},
		{"POST", "/api/v2/admin/invites", `{"email":"not an address","role":"member"}`},
		{"PATCH", "/api/admin/settings", `{}`},
		{"PUT", "/api/v2/settings/yolo", `{}`},
		{"PUT", "/api/v2/maintenance/worktrees", `{}`},
		{"PUT", "/api/v2/settings/branch-prune", `{}`},
		{"PUT", "/api/v2/settings/software-mirror", `{}`},
		{"POST", "/api/v2/settings/software-mirror/cache", `{}`},
		{"PATCH", "/api/v2/admin/notifications/defaults", `{}`},
		{"PATCH", "/api/v2/admin/backups", `{}`},
		{"POST", "/api/v2/admin/backups/run", `{}`},
		{"POST", "/api/v2/admin/backups/remotes", `{}`},
		{"POST", "/api/v2/admin/backups/remotes/test", `{}`},
		{"PATCH", "/api/v2/admin/backups/remotes/r2", `{}`},
		{"DELETE", "/api/v2/admin/backups/remotes/r2", ""},
		{"POST", "/api/v2/admin/backups/remotes/r2/test", `{}`},
		{"DELETE", "/api/v2/admin/backups/remotes/r2/copies/x", ""},
		{"POST", "/api/v2/admin/backups/offsite", `{}`},
		{"POST", "/api/v2/admin/backups/prune", `{}`},
		{"GET", "/api/v2/admin/backups/items/x/download", ""},
		{"DELETE", "/api/v2/admin/backups/items/x", ""},
		{"GET", "/api/v2/admin/backups/import", ""},
		{"POST", "/api/v2/admin/backups/import", `{}`},
		{"DELETE", "/api/v2/admin/backups/import", ""},
		{"POST", fencePath, `{"unknown":1}`},
		{"PATCH", fencePath, `{"unknown":1}`},
		{"DELETE", fencePath, ""},
		{"PATCH", "/api/v2/admin/mail", `{"mode":"nonsense"}`},
		{"POST", "/api/v2/admin/mail/check", `{"mode":"nonsense"}`},
		{"POST", "/api/v2/admin/mail/test", `{}`},
		{"POST", "/api/v2/admin/mail/outbox/resend", `{"id":0}`},
		{"PATCH", "/api/v2/admin/sso", `{"unknown":1}`},
	}
}

// sessionAs signs a user in directly and returns the cookie header pair.
func sessionAs(t *testing.T, e enrolAccounts, email, role string) ([]string, int64) {
	t.Helper()
	u, err := e.st.CreateUser(email, role, role, "hash")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := auth.NewToken()
	if err := e.st.CreateSession(auth.HashToken(raw), u.ID, time.Now().Add(time.Hour).UnixMilli(), "test"); err != nil {
		t.Fatal(err)
	}
	return []string{"Cookie", sessionCookie + "=" + raw}, u.ID
}

func TestOwnerAndAdminChangesNeedASignedInBrowser(t *testing.T) {
	e := newEnrolAccounts(t)
	const refused = "do this from a signed-in browser"
	viewer, _ := sessionAs(t, e, "viewer@example.com", "viewer")
	member, memberID := sessionAs(t, e, "member@example.com", "member")
	for _, rt := range browserOnlyAdminRoutes(memberID) {
		name := rt.method + " " + rt.path
		for _, hdr := range [][]string{{"X-Reet-Token", e.token}, {"Authorization", "Bearer " + e.token}} {
			if status, text := do(t, e.ts, rt.method, rt.path, rt.body, hdr...); status != 403 || !strings.Contains(text, refused) {
				t.Errorf("%s with %s alone: %d %s", name, hdr[0], status, text)
			}
			if status, text := doClient(t, e.browser, e.ts, rt.method, rt.path, rt.body, hdr...); status != 403 || !strings.Contains(text, refused) {
				t.Errorf("%s with %s beside a session: %d %s", name, hdr[0], status, text)
			}
		}
		if status, _ := do(t, e.ts, rt.method, rt.path, rt.body); status != 401 {
			t.Errorf("%s without a session: %d", name, status)
		}
		if status, text := do(t, e.ts, rt.method, rt.path, rt.body, "Authorization", "Bearer "+e.mcpToken); status != 403 || !strings.Contains(text, "MCP credentials") {
			t.Errorf("%s with an MCP token: %d %s", name, status, text)
		}
		if status, text := doClient(t, e.browser, e.ts, rt.method, rt.path, rt.body, "Sec-Fetch-Site", "cross-site"); status != 403 || !strings.Contains(text, "cross-site") {
			t.Errorf("%s cross-site: %d %s", name, status, text)
		}
		for who, cookie := range map[string][]string{"viewer": viewer, "member": member} {
			if status, text := do(t, e.ts, rt.method, rt.path, rt.body, cookie...); status != 403 || !strings.Contains(text, "only") {
				t.Errorf("%s as a %s: %d %s", name, who, status, text)
			}
		}
		// The owner's browser gets past the gate to the handler, as before.
		if status, text := doClient(t, e.browser, e.ts, rt.method, rt.path, rt.body); status == 401 || status == 403 || status == 423 {
			t.Errorf("%s from the owner's browser: %d %s", name, status, text)
		}
	}
}

// What a script may still read or do with a personal token: listings and
// states without secrets.
func TestAdminReadsStillTakeAPersonalToken(t *testing.T) {
	e := newEnrolAccounts(t)
	for _, rt := range []adminRoute{
		{"GET", "/api/v2/admin/users", ""},
		{"GET", "/api/admin/status", ""},
		{"GET", "/api/admin/rollup", ""},
		{"GET", fencePath, ""},
		{"GET", fenceMachinesPath, ""},
		{"GET", "/api/v2/admin/mail", ""},
		{"GET", "/api/v2/admin/mail/outbox", ""},
		{"GET", "/api/v2/admin/sso", ""},
		{"GET", "/api/v2/admin/notifications/defaults", ""},
		{"GET", "/api/v2/admin/backups", ""},
		{"GET", "/api/v2/admin/backups/remotes", ""},
		{"GET", "/api/v2/admin/backups/remotes/r2/copies", ""},
		{"POST", "/api/v2/admin/backups/items/x/verify", `{}`},
	} {
		// Some parts are not wired in this test server, so the handler may stop
		// at 503 or 404; what matters is that the token reached it.
		if status, text := do(t, e.ts, rt.method, rt.path, rt.body, "Authorization", "Bearer "+e.token); status == 401 || status == 403 {
			t.Errorf("%s %s with a token: %d %s", rt.method, rt.path, status, text)
		}
	}
}

// Without accounts the instance's one token is what its browser sends too, so
// the gate has nothing to tell apart and the token still does the owner's work.
func TestInstanceTokenStillChangesSettingsWithoutAccounts(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "instance-token")
	if status, text := do(t, srv, "PATCH", "/api/admin/settings", `{"instanceName":"Bench"}`, "X-Reet-Token", "instance-token"); status != 200 {
		t.Fatalf("settings with the instance token: %d %s", status, text)
	}
	for _, rt := range []adminRoute{{"PUT", "/api/v2/settings/branch-prune", `{}`}, {"GET", "/api/v2/admin/backups/import", ""}, {"DELETE", "/api/v2/admin/backups/import", ""}} {
		if status, text := do(t, srv, rt.method, rt.path, rt.body, "X-Reet-Token", "instance-token"); status == 401 || status == 403 || status == 404 {
			t.Fatalf("%s %s with the instance token: %d %s", rt.method, rt.path, status, text)
		}
	}
}
