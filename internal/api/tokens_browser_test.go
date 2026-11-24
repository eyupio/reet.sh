package api

import (
	"strconv"
	"strings"
	"testing"
)

func TestAPITokensAreManagedOnlyFromASignedInBrowser(t *testing.T) {
	e := newEnrolAccounts(t)
	const refused = "manage API tokens from a signed-in browser"
	for _, route := range [][2]string{{"GET", "/api/v2/tokens"}, {"POST", "/api/v2/tokens"}, {"DELETE", "/api/v2/tokens?id=1"}} {
		body := ""
		if route[0] == "POST" {
			body = `{"name":"replacement"}`
		}
		for _, hdr := range [][]string{{"X-Reet-Token", e.token}, {"Authorization", "Bearer " + e.token}} {
			status, text := do(t, e.ts, route[0], route[1], body, hdr...)
			if status != 403 || !strings.Contains(text, refused) {
				t.Fatalf("%s %s with %s: %d %s", route[0], route[1], hdr[0], status, text)
			}
			// Even beside a valid browser session.
			status, text = doClient(t, e.browser, e.ts, route[0], route[1], body, hdr...)
			if status != 403 || !strings.Contains(text, refused) {
				t.Fatalf("%s %s with %s and a session: %d %s", route[0], route[1], hdr[0], status, text)
			}
		}
		if status, _ := do(t, e.ts, route[0], route[1], body); status != 401 {
			t.Fatalf("%s %s without a session: %d", route[0], route[1], status)
		}
		if status, text := do(t, e.ts, route[0], route[1], body, "Authorization", "Bearer "+e.mcpToken); status != 403 || !strings.Contains(text, "MCP credentials") {
			t.Fatalf("%s %s with an MCP token: %d %s", route[0], route[1], status, text)
		}
		if route[0] != "GET" {
			if status, _ := doClient(t, e.browser, e.ts, route[0], route[1], body, "Origin", "https://evil.invalid"); status != 403 {
				t.Fatalf("%s %s cross-site: %d", route[0], route[1], status)
			}
		}
	}
	// The session still lists, makes and revokes them.
	list := mcpJSON(t, e.browser, e.ts, "GET", "/api/v2/tokens", nil, 200)
	if n := len(list["tokens"].([]any)); n != 1 {
		t.Fatalf("tokens: %v", list)
	}
	made := mcpJSON(t, e.browser, e.ts, "POST", "/api/v2/tokens", map[string]any{"name": "ci"}, 200)
	id := strconv.FormatInt(int64(made["id"].(float64)), 10)
	if status, _ := doClient(t, e.browser, e.ts, "DELETE", "/api/v2/tokens?id="+id, ""); status != 204 {
		t.Fatalf("revoke from the browser: %d", status)
	}
}
