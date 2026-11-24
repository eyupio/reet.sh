package api

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

// What a token can do to its own account that would lock its person out or
// let it keep itself alive: end their sessions, change the password, unlink a
// sign-in method, add a device that receives their notifications.
var accountBrowserRoutes = []struct {
	method, path, body, refused string
}{
	{"GET", "/api/v2/profile/sessions", "", "manage your sessions from a signed-in browser"},
	{"DELETE", "/api/v2/profile/sessions/x", "", "manage your sessions from a signed-in browser"},
	{"POST", "/api/v2/profile/sessions/end-others", "{}", "manage your sessions from a signed-in browser"},
	{"POST", "/api/v2/profile/password", `{"current":"x","password":"y"}`, "change your password from a signed-in browser"},
	{"DELETE", "/api/v2/auth/oidc/identity", "", "unlink single sign-on from a signed-in browser"},
	{"POST", "/api/v2/push/subscriptions", `{"endpoint":"https://push.example/x"}`, "add a notification device from a signed-in browser"},
}

func TestAccountLockoutRoutesNeedASignedInBrowser(t *testing.T) {
	e := newEnrolAccounts(t)
	for _, rt := range accountBrowserRoutes {
		name := rt.method + " " + rt.path
		for _, hdr := range [][]string{{"X-Reet-Token", e.token}, {"Authorization", "Bearer " + e.token}} {
			if status, text := do(t, e.ts, rt.method, rt.path, rt.body, hdr...); status != 403 || !strings.Contains(text, rt.refused) {
				t.Errorf("%s with %s alone: %d %s", name, hdr[0], status, text)
			}
			if status, text := doClient(t, e.browser, e.ts, rt.method, rt.path, rt.body, hdr...); status != 403 || !strings.Contains(text, rt.refused) {
				t.Errorf("%s with %s beside a session: %d %s", name, hdr[0], status, text)
			}
		}
		if status, _ := do(t, e.ts, rt.method, rt.path, rt.body); status != 401 {
			t.Errorf("%s without a session: %d", name, status)
		}
		if status, text := doClient(t, e.browser, e.ts, rt.method, rt.path, rt.body, "Origin", "https://evil.invalid"); status != 403 || !strings.Contains(text, "cross-site") {
			t.Errorf("%s from another site's Origin: %d %s", name, status, text)
		}
		if status, text := doClient(t, e.browser, e.ts, rt.method, rt.path, rt.body); status == 401 || status == 403 {
			t.Errorf("%s from the browser: %d %s", name, status, text)
		}
	}
}

// A leaked token used to list the account's sessions and end each one, over
// and over, so its person could never stay signed in long enough to revoke it.
func TestALeakedTokenCannotKeepItsPersonSignedOut(t *testing.T) {
	e := newEnrolAccounts(t)
	raw, _ := auth.NewToken()
	if err := e.st.CreateSession(auth.HashToken(raw), e.uid, time.Now().Add(time.Hour).UnixMilli(), "phone"); err != nil {
		t.Fatal(err)
	}
	before, err := e.st.SessionsFor(e.uid)
	if err != nil || len(before) != 2 {
		t.Fatalf("sessions before: %d %v", len(before), err)
	}
	for range 3 {
		if status, _ := do(t, e.ts, "GET", "/api/v2/profile/sessions", "", "Authorization", "Bearer "+e.token); status != 403 {
			t.Fatalf("the token listed the sessions: %d", status)
		}
		for _, x := range before {
			if status, _ := do(t, e.ts, "DELETE", "/api/v2/profile/sessions/"+x.ID, "", "Authorization", "Bearer "+e.token); status != 403 {
				t.Fatalf("the token ended a session: %d", status)
			}
		}
	}
	after, _ := e.st.SessionsFor(e.uid)
	if len(after) != 2 {
		t.Fatalf("sessions after the loop: %d", len(after))
	}
	// The person's browser lists them and ends the other one.
	list := mcpJSON(t, e.browser, e.ts, "GET", "/api/v2/profile/sessions", nil, 200)
	if n := len(list["sessions"].([]any)); n != 2 {
		t.Fatalf("the browser listed %d sessions", n)
	}
	if status, text := doClient(t, e.browser, e.ts, "DELETE", "/api/v2/profile/sessions/"+auth.HashToken(raw), ""); status != 204 {
		t.Fatalf("the browser ended a session: %d %s", status, text)
	}
}

func TestInstanceTokenStillListsSessionsWithoutAccounts(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "instance-token")
	if status, text := do(t, srv, "GET", "/api/v2/profile/sessions", "", "X-Reet-Token", "instance-token"); status != 200 {
		t.Fatalf("sessions with the instance token: %d %s", status, text)
	}
}

// Behind a proxy that rewrites Host, the browser's Origin is the public
// address, not the Host the controller sees; that is still the same site.
func TestTheBrowserCheckAcceptsThePublicOriginBehindAProxy(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(st, meta.Fetcher{}, "", WithAuthMode(AuthAccounts), WithPublicURL("https://reet.example")))
	t.Cleanup(func() { srv.Close(); st.Close() })
	owner, err := st.CreateUser("owner@example.com", "Owner", "owner", "hash")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := auth.NewToken()
	if err := st.CreateSession(auth.HashToken(raw), owner.ID, time.Now().Add(time.Hour).UnixMilli(), "test"); err != nil {
		t.Fatal(err)
	}
	cookie := []string{"Cookie", sessionCookie + "=" + raw}
	for _, rt := range []adminRoute{{"PATCH", "/api/admin/settings", `{}`}, {"GET", "/api/v2/profile/sessions", ""}} {
		name := rt.method + " " + rt.path
		if status, text := do(t, srv, rt.method, rt.path, rt.body, append(cookie, "Origin", "https://reet.example")...); status != 200 {
			t.Errorf("%s from the public origin with another Host: %d %s", name, status, text)
		}
		for _, foreign := range []string{"https://evil.invalid", "http://reet.example", "https://reet.example.evil.invalid", "null"} {
			if status, text := do(t, srv, rt.method, rt.path, rt.body, append(cookie, "Origin", foreign)...); status != 403 || !strings.Contains(text, "cross-site") {
				t.Errorf("%s from %s: %d %s", name, foreign, status, text)
			}
		}
		if status, text := do(t, srv, rt.method, rt.path, rt.body, cookie...); status != 200 {
			t.Errorf("%s with no Origin: %d %s", name, status, text)
		}
	}
}

// Every owner or admin write must be behind a browser-only wrapper, unless it
// is in the list below on purpose. ServeMux cannot list its routes, so this
// reads the route table from the package's source.
func TestEveryOwnerAndAdminWriteNeedsABrowser(t *testing.T) {
	open := map[string]string{
		"PATCH /api/tag":                               "renames a tag; shelf content, not the instance",
		"POST /api/tags/merge":                         "merges tags; shelf content, not the instance",
		"POST /api/v2/admin/tickets/{id}/reply":        "support desk work",
		"POST /api/v2/admin/tickets/{id}/note":         "support desk work",
		"POST /api/v2/admin/tickets/{id}/status":       "support desk work",
		"POST /api/v2/admin/tickets/{id}/assign":       "support desk work",
		"POST /api/v2/admin/backups/items/{id}/verify": "a monitoring script checks copies; rate-limited",
	}
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatal("no source", err)
	}
	route := regexp.MustCompile(`mux\.HandleFunc\("(GET|HEAD|POST|PUT|PATCH|DELETE) ([^"]*)"(\+(\w+))?, (.*)\)\s*$`)
	consts := map[string]string{"fencePath": fencePath, "fenceMachinesPath": fenceMachinesPath}
	seen := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			m := route.FindStringSubmatch(line)
			if m == nil || m[1] == "GET" || m[1] == "HEAD" {
				continue
			}
			wrappers := m[5]
			if !strings.Contains(wrappers, "requireOwner(") && !strings.Contains(wrappers, "requireAdmin(") {
				continue
			}
			path := m[2]
			if m[4] != "" {
				c, ok := consts[m[4]]
				if !ok {
					t.Fatalf("%s:%d: unknown path constant %s", name, i+1, m[4])
				}
				path += c
			}
			key := m[1] + " " + path
			seen++
			gated := strings.Contains(wrappers, "adminBrowser(") || strings.Contains(wrappers, "browserOnly(") || strings.Contains(wrappers, "terminalAccess(")
			if _, ok := open[key]; ok == gated {
				if gated {
					t.Errorf("%s:%d: %s is gated but listed as deliberately open", name, i+1, key)
				} else {
					t.Errorf("%s:%d: %s changes what an owner or admin owns but a personal token can call it; put it behind adminBrowser or list it as open with the reason", name, i+1, key)
				}
			}
		}
	}
	if seen < 40 {
		t.Fatalf("only %d owner and admin writes found; the route pattern no longer matches the source", seen)
	}
}
