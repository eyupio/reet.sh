package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func mcpAccounts(t *testing.T) (*httptest.Server, *store.Store, *http.Client, int64) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(st, meta.Fetcher{}, "", WithAuthMode(AuthAccounts)))
	t.Cleanup(func() { ts.Close(); st.Close() })
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	code, _ := doClient(t, client, ts, "POST", "/api/v2/auth/setup", `{"name":"Ada","email":"ada@example.com","password":"test password"}`, "Content-Type", "application/json")
	if code != 200 {
		t.Fatalf("setup: %d", code)
	}
	u, _, err := st.OwnerUser()
	if err != nil {
		t.Fatal(err)
	}
	rf, _ := ref.Parse("demo/skills/private")
	notes := "Ada's private notes"
	_, err = st.UpsertFor(u.ID, rf, store.Patch{Notes: &notes})
	if err != nil {
		t.Fatal(err)
	}
	return ts, st, client, u.ID
}

func mcpJSON(t *testing.T, c *http.Client, ts *httptest.Server, method, path string, body any, want int) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(body)
	status, text := doClient(t, c, ts, method, path, string(raw), "Content-Type", "application/json")
	if status != want {
		t.Fatalf("%s %s: got %d want %d: %s", method, path, status, want, text)
	}
	out := map[string]any{}
	if text != "" {
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func mcpManual(t *testing.T, c *http.Client, ts *httptest.Server, scope string) (string, int64) {
	t.Helper()
	mcpJSON(t, c, ts, "PATCH", "/api/v2/mcp", map[string]any{"enabled": true}, 200)
	r := mcpJSON(t, c, ts, "POST", "/api/v2/mcp/connections", map[string]any{"name": "Desktop test", "scope": scope}, 200)
	return r["token"].(string), int64(r["connection"].(map[string]any)["id"].(float64))
}

type mcpBearerTransport struct{ token string }

func (b mcpBearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func mcpSession(t *testing.T, ts *httptest.Server, token string) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	client := mcp.NewClient(&mcp.Implementation{Name: "reet-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp", HTTPClient: &http.Client{Transport: mcpBearerTransport{token}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func TestMCPHostedScopeAndLifecycle(t *testing.T) {
	ts, st, browser, uid := mcpAccounts(t)
	token, id := mcpManual(t, browser, ts, "read")
	session := mcpSession(t, ts, token)
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if !tool.Annotations.ReadOnlyHint {
			t.Fatalf("write tool exposed: %s", tool.Name)
		}
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "reet_list_skills", Arguments: map[string]any{"limit": 1}})
	if err != nil || result.IsError {
		t.Fatalf("list: %v %+v", err, result)
	}
	if _, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "reet_remove_skill", Arguments: map[string]any{"id": "demo/skills/private"}}); err == nil {
		t.Fatal("read-only connection accepted a write tool")
	}
	for _, path := range []string{"/api/v2/tokens", "/api/v2/admin/users", "/api/v2/mcp"} {
		status, _ := do(t, ts, "GET", path, "", "Authorization", "Bearer "+token)
		if status != 403 {
			t.Fatalf("credential accessed %s: %d", path, status)
		}
	}
	mcpJSON(t, browser, ts, "PATCH", fmt.Sprintf("/api/v2/mcp/connections/%d", id), map[string]any{"enabled": false}, 200)
	if _, err = session.ListTools(context.Background(), nil); err == nil {
		t.Fatal("paused connection remained usable")
	}
	mcpJSON(t, browser, ts, "PATCH", fmt.Sprintf("/api/v2/mcp/connections/%d", id), map[string]any{"enabled": true}, 200)
	rotated := mcpJSON(t, browser, ts, "POST", fmt.Sprintf("/api/v2/mcp/connections/%d/rotate", id), map[string]any{}, 200)
	if _, _, ok, _ := st.MCPUser(auth.HashToken(token), time.Now().UnixMilli()); ok {
		t.Fatal("old token survived rotation")
	}
	newToken := rotated["token"].(string)
	if _, _, ok, _ := st.MCPUser(auth.HashToken(newToken), time.Now().UnixMilli()); !ok {
		t.Fatal("new token is invalid")
	}
	if _, _, ok, _ := st.MCPUser(auth.HashToken(newToken), time.Now().Add(31*24*time.Hour).UnixMilli()); ok {
		t.Fatal("expired token accepted")
	}
	if err := st.SetMCPEnabled(uid, false); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, _ := st.MCPUser(auth.HashToken(newToken), time.Now().UnixMilli()); ok {
		t.Fatal("master pause failed")
	}
	if err := st.SetMCPEnabled(uid, true); err != nil {
		t.Fatal(err)
	}
	mcpJSON(t, browser, ts, "DELETE", fmt.Sprintf("/api/v2/mcp/connections/%d", id), nil, 204)
	if _, _, ok, _ := st.MCPUser(auth.HashToken(newToken), time.Now().UnixMilli()); ok {
		t.Fatal("revoked token accepted")
	}
}

func TestMCPMaintenanceIsSessionOnlyAndBrowserToolsEnforceScope(t *testing.T) {
	ts, st, browser, uid := mcpAccounts(t)
	status, _ := do(t, ts, "GET", "/mcp", "")
	if status != 401 {
		t.Fatalf("anonymous MCP: %d", status)
	}
	resp, err := browser.Get(ts.URL + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatal("session cookie authenticated the MCP transport")
	}
	token, _ := mcpManual(t, browser, ts, "write")
	status, _ = do(t, ts, "GET", "/api/v2/mcp", "", "Authorization", "Bearer "+token)
	if status != 403 {
		t.Fatal("MCP token can maintain credentials")
	}
	apiToken := "general-api-test"
	_, err = st.CreateAPIToken(uid, "general", auth.HashToken(apiToken))
	if err != nil {
		t.Fatal(err)
	}
	status, _ = do(t, ts, "GET", "/api/v2/mcp", "", "Authorization", "Bearer "+apiToken)
	if status != 403 {
		t.Fatal("general API token can maintain MCP credentials")
	}
	status, _ = doClient(t, browser, ts, "PATCH", "/api/v2/mcp", `{"enabled":true}`, "Content-Type", "application/json", "Origin", "https://evil.invalid")
	if status != 403 {
		t.Fatal("cross-origin maintenance accepted")
	}
	mcpJSON(t, browser, ts, "PATCH", "/api/v2/mcp", map[string]any{"browserEnabled": true}, 200)
	tools := mcpJSON(t, browser, ts, "GET", "/api/v2/mcp/browser/tools", nil, 200)
	for _, entry := range tools["tools"].([]any) {
		if entry.(map[string]any)["annotations"].(map[string]any)["readOnlyHint"] != true {
			t.Fatal("browser read scope exposed writes")
		}
	}
	mcpJSON(t, browser, ts, "POST", "/api/v2/mcp/browser/call", map[string]any{"name": "reet_list_skills", "arguments": map[string]any{}}, 200)
	mcpJSON(t, browser, ts, "POST", "/api/v2/mcp/browser/call", map[string]any{"name": "reet_list_skills", "arguments": map[string]any{"limit": 0}}, 400)
	mcpJSON(t, browser, ts, "POST", "/api/v2/mcp/browser/call", map[string]any{"name": "reet_remove_skill", "arguments": map[string]any{"id": "demo/skills/private"}}, 400)
	mcpJSON(t, browser, ts, "PATCH", "/api/v2/mcp", map[string]any{"browserScope": "write"}, 200)
	mcpJSON(t, browser, ts, "POST", "/api/v2/mcp/browser/call", map[string]any{"name": "reet_update_skill", "arguments": map[string]any{"id": "demo/skills/private", "liked": true}}, 200)
	mcpJSON(t, browser, ts, "PATCH", "/api/v2/mcp", map[string]any{"enabled": false}, 200)
	mcpJSON(t, browser, ts, "GET", "/api/v2/mcp/browser/tools", nil, 403)
}

func oauthClientAndQuery(t *testing.T, ts *httptest.Server) (map[string]any, url.Values, string) {
	t.Helper()
	client := mcpJSON(t, http.DefaultClient, ts, "POST", "/oauth/register", map[string]any{"client_name": "Claude test", "redirect_uris": []string{"https://client.example/callback"}, "token_endpoint_auth_method": "none"}, 201)
	verifier := strings.Repeat("a", 43)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	q := url.Values{"client_id": {client["client_id"].(string)}, "redirect_uri": {"https://client.example/callback"}, "response_type": {"code"}, "code_challenge_method": {"S256"}, "code_challenge": {challenge}, "resource": {ts.URL + "/mcp"}, "scope": {"mcp:read mcp:write"}, "state": {"client-state"}}
	return client, q, verifier
}

func oauthForm(t *testing.T, ts *httptest.Server, form url.Values, want int) map[string]any {
	t.Helper()
	resp, err := http.PostForm(ts.URL+"/oauth/token", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		t.Fatalf("token status %d want %d: %s", resp.StatusCode, want, raw)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMCPOAuthPKCECodeBindingRefreshRotationAndRevocation(t *testing.T) {
	ts, st, browser, uid := mcpAccounts(t)
	metadata := mcpJSON(t, http.DefaultClient, ts, "GET", "/.well-known/oauth-protected-resource/mcp", nil, 200)
	if metadata["resource"] != ts.URL+"/mcp" {
		t.Fatal(metadata)
	}
	_, q, verifier := oauthClientAndQuery(t, ts)
	mcpJSON(t, browser, ts, "GET", "/api/v2/mcp/authorization?"+q.Encode(), nil, 200)
	consent := mcpJSON(t, browser, ts, "POST", "/api/v2/mcp/authorization", map[string]any{"query": q.Encode(), "approve": true, "scope": "read"}, 200)
	callback, _ := url.Parse(consent["redirect"].(string))
	if callback.Query().Get("state") != "client-state" {
		t.Fatal("state was not preserved")
	}
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {q.Get("client_id")}, "redirect_uri": {q.Get("redirect_uri")}, "resource": {q.Get("resource")}, "code": {callback.Query().Get("code")}, "code_verifier": {strings.Repeat("b", 43)}}
	oauthForm(t, ts, form, 400) // Bad PKCE must not consume a valid code.
	form.Set("code_verifier", verifier)
	form.Set("redirect_uri", "https://client.example/other")
	oauthForm(t, ts, form, 400)
	form.Set("redirect_uri", q.Get("redirect_uri"))
	tokens := oauthForm(t, ts, form, 200)
	oauthForm(t, ts, form, 400)
	access := tokens["access_token"].(string)
	if _, _, ok, _ := st.MCPUserFor(auth.HashToken(access), time.Now().UnixMilli(), "https://other.example/mcp"); ok {
		t.Fatal("access token accepted for the wrong resource")
	}
	if tokens["scope"] != "mcp:read" {
		t.Fatal("consent downscope was ignored")
	}
	user, c, ok, err := st.MCPUser(auth.HashToken(access), time.Now().UnixMilli())
	if err != nil || !ok || user.ID != uid || c.Scope != "read" {
		t.Fatalf("issued access token: %v %v %+v", err, ok, c)
	}
	if _, _, ok, _ = st.MCPUser(auth.HashToken(access), time.Now().Add(2*time.Hour).UnixMilli()); ok {
		t.Fatal("access token outlived one hour")
	}
	// Raising a connection's configured scope cannot expand the original OAuth grant.
	if err := st.UpdateMCPConnection(uid, c.ID, c.Name, "write", true, c.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	_, current, _, _ := st.MCPUser(auth.HashToken(access), time.Now().UnixMilli())
	if current.Scope != "read" {
		t.Fatal("OAuth scope expanded beyond consent")
	}
	refresh := url.Values{"grant_type": {"refresh_token"}, "client_id": {q.Get("client_id")}, "resource": {q.Get("resource")}, "refresh_token": {tokens["refresh_token"].(string)}}
	newTokens := oauthForm(t, ts, refresh, 200)
	if newTokens["refresh_token"] == tokens["refresh_token"] {
		t.Fatal("refresh token was not rotated")
	}
	oauthForm(t, ts, refresh, 400) // Reuse revokes the entire connection family.
	if _, _, ok, _ := st.MCPUser(auth.HashToken(newTokens["access_token"].(string)), time.Now().UnixMilli()); ok {
		t.Fatal("replayed refresh did not revoke the connection")
	}
}

func TestMCPOAuthRejectsUnregisteredRedirectsPlainPKCEAndCrossUserManagement(t *testing.T) {
	ts, st, browser, _ := mcpAccounts(t)
	mcpJSON(t, http.DefaultClient, ts, "POST", "/oauth/register", map[string]any{"redirect_uris": []string{"http://evil.example/callback"}}, 400)
	_, q, _ := oauthClientAndQuery(t, ts)
	bad := q.Encode()
	q.Set("code_challenge_method", "plain")
	mcpJSON(t, browser, ts, "GET", "/api/v2/mcp/authorization?"+q.Encode(), nil, 400)
	q, _ = url.ParseQuery(bad)
	q.Set("redirect_uri", "https://evil.example/callback")
	mcpJSON(t, browser, ts, "GET", "/api/v2/mcp/authorization?"+q.Encode(), nil, 400)
	q, _ = url.ParseQuery(bad)
	q.Set("resource", "https://evil.example/mcp")
	mcpJSON(t, browser, ts, "GET", "/api/v2/mcp/authorization?"+q.Encode(), nil, 400)
	other, err := st.CreateUser("other@example.com", "Other", "viewer", "")
	if err != nil {
		t.Fatal(err)
	}
	connection, err := st.CreateMCPConnection(other.ID, "Other client", "read", auth.HashToken("other-token"), time.Now().Add(time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	mcpJSON(t, browser, ts, "DELETE", fmt.Sprintf("/api/v2/mcp/connections/%d", connection.ID), nil, 404)
}

func TestMCPNativeLoopbackCallbackPortMatching(t *testing.T) {
	if !mcpRedirectMatches("http://127.0.0.1:1234/callback", "http://127.0.0.1:5678/callback") {
		t.Fatal("native loopback port changes must be supported")
	}
	for _, requested := range []string{"http://127.0.0.2:5678/callback", "http://127.0.0.1:5678/other", "http://127.0.0.1:5678/callback?extra=1", "http://evil.example:5678/callback"} {
		if mcpRedirectMatches("http://127.0.0.1:1234/callback", requested) {
			t.Fatalf("callback mismatch accepted: %s", requested)
		}
	}
}

func TestMCPExistingClientsFollowRoleChangesDisablingAndAdminRevocation(t *testing.T) {
	ts, st, _, _ := mcpAccounts(t)
	u, err := st.CreateUser("member@example.com", "Member", "member", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = st.SetMCPEnabled(u.ID, true); err != nil {
		t.Fatal(err)
	}
	token := "reet_mcp_role_test"
	if _, err = st.CreateMCPConnection(u.ID, "Existing desktop", "write", auth.HashToken(token), time.Now().Add(time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	session := mcpSession(t, ts, token)
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) != 30 {
		t.Fatalf("member tools: %v", err)
	}
	if err = st.UpdateUserRole(u.ID, "viewer"); err != nil {
		t.Fatal(err)
	}
	tools, err = session.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) != 11 {
		t.Fatalf("viewer tools: %v", err)
	}
	if err = st.SetUserDisabled(u.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err = session.ListTools(context.Background(), nil); err == nil {
		t.Fatal("disabled account retained MCP access")
	}
	if err = st.SetUserDisabled(u.ID, false); err != nil {
		t.Fatal(err)
	}
	if err = st.RevokeUserTokens(u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = session.ListTools(context.Background(), nil); err == nil {
		t.Fatal("admin token revocation missed MCP access")
	}
}

func TestMCPHostedSkillReferences(t *testing.T) {
	for _, transport := range []string{"hosted", "browser"} {
		t.Run(transport, func(t *testing.T) {
			ts, _, browser, _ := mcpAccounts(t)
			mcpJSON(t, browser, ts, "POST", "/api/v2/hosting/username", map[string]any{"username": "ada"}, 200)
			mcpJSON(t, browser, ts, "PUT", "/api/v2/hosting/skills/demo", store.HostedSkill{
				Published: true, Content: "---\nname: demo\ndescription: A demo skill.\n---\n\nInstructions.",
			}, 200)
			call := mcpToolCaller(t, ts, browser, transport)
			input := ts.URL + "/ada/demo"
			failed, text := call("reet_add_skill", map[string]any{"input": input})
			if failed || !strings.Contains(text, `"sourceType":"hosted"`) {
				t.Fatalf("add hosted skill: %s", text)
			}
			created := mcpJSON(t, browser, ts, "POST", "/api/v2/lists", map[string]any{"name": "Hosted references", "visibility": "personal"}, 200)
			failed, text = call("reet_add_list_skill", map[string]any{"list_id": created["id"], "input": "npx skills add " + input})
			if failed {
				t.Fatalf("add hosted skill to list: %s", text)
			}
		})
	}
}

func TestMCPWriteResponseLimits(t *testing.T) {
	for _, transport := range []string{"hosted", "browser"} {
		t.Run(transport, func(t *testing.T) {
			ts, st, browser, uid := mcpAccounts(t)
			call := mcpToolCaller(t, ts, browser, transport)
			// Under the character budget but over 25,000 UTF-8 bytes.
			notes := strings.Repeat("漢", 10000)
			failed, text := call("reet_update_skill", map[string]any{"id": "demo/skills/private", "notes": notes})
			if failed || !strings.Contains(text, notes) {
				t.Fatalf("Unicode write failed: %s", text)
			}
			failed, text = call("reet_list_skills", map[string]any{"limit": 1})
			if failed || !strings.Contains(text, notes) {
				t.Fatalf("Unicode page failed: %s", text)
			}
			// Valid input can still exceed the response budget after JSON escaping.
			notes = strings.Repeat("\n", 14000)
			failed, text = call("reet_update_skill", map[string]any{"id": "demo/skills/private", "notes": notes})
			if !failed || !strings.Contains(text, "the change succeeded") {
				t.Fatalf("oversized write must report that it succeeded: %s", text)
			}
			sk, ok, err := st.GetFor(uid, "demo/skills/private")
			if err != nil || !ok || sk.Notes != notes {
				t.Fatalf("write was not persisted: %v", err)
			}
			failed, text = call("reet_get_skill", map[string]any{"id": "demo/skills/private"})
			if !failed || strings.Contains(text, "change succeeded") {
				t.Fatalf("oversized read must not claim a write succeeded: %s", text)
			}
		})
	}
}

// Exercise both entry points through their public protocols.
func mcpToolCaller(t *testing.T, ts *httptest.Server, browser *http.Client, transport string) func(string, map[string]any) (bool, string) {
	t.Helper()
	token, _ := mcpManual(t, browser, ts, "write")
	if transport == "browser" {
		mcpJSON(t, browser, ts, "PATCH", "/api/v2/mcp", map[string]any{"browserEnabled": true, "browserScope": "write"}, 200)
		return func(name string, args map[string]any) (bool, string) {
			t.Helper()
			raw, _ := json.Marshal(map[string]any{"name": name, "arguments": args})
			status, text := doClient(t, browser, ts, "POST", "/api/v2/mcp/browser/call", string(raw), "Content-Type", "application/json")
			return status != 200, text
		}
	}
	session := mcpSession(t, ts, token)
	return func(name string, args map[string]any) (bool, string) {
		t.Helper()
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		return result.IsError, result.Content[0].(*mcp.TextContent).Text
	}
}

func TestMCPLocalSkillsAndMachineCleanup(t *testing.T) {
	for _, transport := range []string{"hosted", "browser"} {
		t.Run(transport, func(t *testing.T) {
			ts, _, browser, _ := mcpAccounts(t)
			imp := func(host string, skills ...string) {
				items := []map[string]any{}
				for _, name := range skills {
					items = append(items, map[string]any{"name": name, "host": host, "agent": "agents"})
				}
				mcpJSON(t, browser, ts, "POST", "/api/import", map[string]any{
					"machine": map[string]any{"host": host}, "prune": true, "items": items,
					"environments": []map[string]any{{"host": host, "agent": "agents", "skills": skills}},
				}, 200)
			}
			imp("empty-box")
			imp("renamed-box")
			imp("main-box", "mine")
			call := mcpToolCaller(t, ts, browser, transport)

			// Local skills can be added to a list by shelf ID, and exactly one of id/input is required.
			created := mcpJSON(t, browser, ts, "POST", "/api/v2/lists", map[string]any{"name": "Local", "visibility": "personal"}, 200)
			failed, text := call("reet_add_list_skill", map[string]any{"list_id": created["id"], "id": "local/mine"})
			if failed || !strings.Contains(text, `"skillCount":1`) {
				t.Fatalf("add local skill by id: %s", text)
			}
			for _, args := range []map[string]any{
				{"list_id": created["id"]},
				{"list_id": created["id"], "id": "local/mine", "input": "o/r/s"},
			} {
				if failed, text = call("reet_add_list_skill", args); !failed || !strings.Contains(text, "exactly one") {
					t.Fatalf("id/input rule not enforced: %s", text)
				}
			}

			// Forgetting is a dry run until confirmed, and refuses machines that still list skills.
			failed, text = call("reet_forget_machine", map[string]any{"host": "empty-box"})
			if failed || !strings.Contains(text, `"dry_run":true`) || !strings.Contains(text, `"applied":false`) {
				t.Fatalf("dry run: %s", text)
			}
			failed, text = call("reet_list_environments", map[string]any{"host": "empty-box"})
			if failed || !strings.Contains(text, `"total_count":2`) {
				t.Fatalf("dry run must not delete: %s", text)
			}
			if failed, text = call("reet_forget_machine", map[string]any{"host": "main-box", "confirm": true}); !failed || !strings.Contains(text, "still lists 1 skills") {
				t.Fatalf("machine with skills must need force: %s", text)
			}
			failed, text = call("reet_forget_machine", map[string]any{"host": "empty-box", "confirm": true})
			if failed || !strings.Contains(text, `"applied":true`) {
				t.Fatalf("forget: %s", text)
			}
			failed, text = call("reet_list_environments", map[string]any{"host": "empty-box"})
			if failed || !strings.Contains(text, `"total_count":0`) {
				t.Fatalf("machine still listed: %s", text)
			}
			if failed, text = call("reet_forget_machine", map[string]any{"host": "empty-box", "confirm": true}); !failed {
				t.Fatalf("unknown host must fail: %s", text)
			}

			// Merge is also a dry run until confirmed.
			if failed, text = call("reet_merge_machines", map[string]any{"from": "renamed-box", "to": "main-box"}); failed || !strings.Contains(text, `"applied":false`) {
				t.Fatalf("merge dry run: %s", text)
			}
			if failed, text = call("reet_merge_machines", map[string]any{"from": "renamed-box", "to": "main-box", "confirm": true}); failed || !strings.Contains(text, `"applied":true`) {
				t.Fatalf("merge: %s", text)
			}
			if failed, text = call("reet_list_environments", map[string]any{"host": "renamed-box"}); failed || !strings.Contains(text, `"total_count":0`) {
				t.Fatalf("merged host still listed: %s", text)
			}
		})
	}
}

func TestMCPMachineToolsNeedWriteScope(t *testing.T) {
	ts, _, browser, _ := mcpAccounts(t)
	token, _ := mcpManual(t, browser, ts, "read")
	tools, err := mcpSession(t, ts, token).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if tool.Name == "reet_forget_machine" || tool.Name == "reet_merge_machines" {
			t.Fatalf("read-only connection exposes %s", tool.Name)
		}
	}
}
