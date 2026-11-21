package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
)

// factoryMCPServer is a controller with remote sessions on and one owner,
// signed in from a browser.
func factoryMCPServer(t *testing.T) (*server, *httptest.Server, *http.Client, store.User) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seal, _ := sealed.NewSealer(strings.Repeat("gaffer-test-", 8), "test")
	h := New(st, meta.Fetcher{}, "bootstrap", WithAuthMode(AuthAccounts), WithPublicURL("http://localhost:8080"), WithRemoteTerminals(true), WithGafferProviders(seal))
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); h.Close(); st.Close() })
	owner := &http.Client{Jar: mustJar(t)}
	if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.test","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatalf("setup %d", code)
	}
	user, ok, err := st.UserByEmail("owner@example.test")
	if err != nil || !ok {
		t.Fatalf("owner: %v %v", ok, err)
	}
	return h.s, srv, owner, user
}

// factoryReadyToMerge saves an outcome whose pipeline waits for its merge to
// be approved, and the arguments that approve it as it was read.
func factoryReadyToMerge(t *testing.T, s *server, uid int64, id string) (store.GafferOutcome, map[string]any) {
	t.Helper()
	o := factoryRecord(t, s, uid, id, "ready_to_merge", func(p *store.GafferOutcomeSession) { p.Stage, p.UpdatedAt = 1, 5000 })
	return o, map[string]any{"outcome_id": o.ID, "pipeline": "a", "confirm": true, "seen_updated_at": float64(5000)}
}

func factoryMergeApproved(t *testing.T, s *server, uid int64, id string) bool {
	t.Helper()
	o, err := s.st.GafferOutcomeFor(uid, id)
	if err != nil {
		t.Fatal(err)
	}
	return o.Sessions[0].MergeApproved
}

// A stage agent uses the same connection as the person's own client on its
// machine, so a connection approves a merge only when the person has allowed
// it to. Gaffer's own tools and the signed-in browser are the person's, and
// approve as they did.
func TestFactoryMCPMergeApprovalNeedsAConnectionAllowedToApprove(t *testing.T) {
	t.Parallel()
	s, user, allowed := factoryMCPFixture(t)
	ctx := context.Background()
	o, args := factoryReadyToMerge(t, s, user.ID, "c")
	for name, c := range map[string]store.MCPConnection{
		"a bearer token":                   {ID: 2, Name: "manager", Scope: "write", Kind: "bearer"},
		"an OAuth client":                  {ID: 3, Name: "client", Scope: "write", Kind: "oauth"},
		"a connection of no known kind":    {Name: "unknown", Scope: "write"},
		"a stored connection named Gaffer": {ID: 4, Name: "Gaffer", Scope: "write", Kind: "gaffer"},
	} {
		// It is told so whatever else the call got wrong.
		for _, a := range []map[string]any{args, {"outcome_id": o.ID, "pipeline": "a"}} {
			if _, err := s.executeMCPFactory(ctx, mcpAccess{User: user, Connection: c}, "reet_approve_pipeline_merge", a); err == nil || !strings.Contains(err.Error(), "may not approve merges") {
				t.Fatalf("approval by %s: %v", name, err)
			}
		}
		if factoryMergeApproved(t, s, user.ID, o.ID) {
			t.Fatalf("approval by %s changed the pipeline", name)
		}
	}
	// Only the approval is held back: the same connection can still make a pipeline wait.
	wait, _ := factoryReadyToMerge(t, s, user.ID, "f")
	if _, err := s.executeMCPFactory(ctx, mcpAccess{User: user, Connection: store.MCPConnection{ID: 2, Name: "manager", Scope: "write", Kind: "bearer"}}, "reet_set_pipeline_merge", map[string]any{"outcome_id": wait.ID, "pipeline": "a"}); err != nil {
		t.Fatalf("making a pipeline wait: %v", err)
	}
	if out, err := s.executeMCPFactory(ctx, allowed, "reet_approve_pipeline_merge", args); err != nil || out["applied"] != true || !factoryMergeApproved(t, s, user.ID, o.ID) {
		t.Fatalf("approval by a connection allowed to: %v %v", out, err)
	}
	for id, c := range map[string]store.MCPConnection{
		"d": {Name: "Gaffer", Scope: "write", Kind: "gaffer"},
		"e": {Name: "Browser WebMCP", Scope: "write", Kind: "browser"},
	} {
		o, args := factoryReadyToMerge(t, s, user.ID, id)
		if out, err := s.executeMCPFactory(ctx, mcpAccess{User: user, Connection: c}, "reet_approve_pipeline_merge", args); err != nil || out["applied"] != true || !factoryMergeApproved(t, s, user.ID, o.ID) {
			t.Fatalf("approval by %s: %v %v", c.Name, out, err)
		}
	}
}

// The permission is given from a signed-in browser. The connection it is for
// cannot give it to itself, over the API or through a tool.
func TestMCPMergeApprovalIsAllowedOnlyFromASignedInBrowser(t *testing.T) {
	t.Parallel()
	s, srv, owner, user := factoryMCPServer(t)
	token, id := mcpManual(t, owner, srv, "write")
	path := fmt.Sprintf("/api/v2/mcp/connections/%d", id)
	listed := func(id int64) bool {
		t.Helper()
		for _, entry := range mcpJSON(t, owner, srv, "GET", "/api/v2/mcp", nil, 200)["connections"].([]any) {
			if c := entry.(map[string]any); int64(c["id"].(float64)) == id {
				return c["approveMerges"].(bool)
			}
		}
		t.Fatalf("connection %d is not listed", id)
		return false
	}
	o, args := factoryReadyToMerge(t, s, user.ID, "c")
	session := mcpSession(t, srv, token)
	if listed(id) {
		t.Fatal("a new connection may approve merges")
	}
	if text := mcpError(t, session, "reet_approve_pipeline_merge", args); !strings.Contains(text, "may not approve merges") || factoryMergeApproved(t, s, user.ID, o.ID) {
		t.Fatalf("approval without the permission: %s", text)
	}
	// Its token is refused on the route, alone or beside the browser's
	// session, and no tool's request can reach it.
	for name, c := range map[string]*http.Client{"alone": http.DefaultClient, "beside a session": owner} {
		for _, header := range []string{"Authorization", "X-Reet-Token"} {
			value := token
			if header == "Authorization" {
				value = "Bearer " + token
			}
			if status, _ := doClient(t, c, srv, "PATCH", path, `{"approveMerges":true}`, "Content-Type", "application/json", header, value); status != 401 && status != 403 {
				t.Fatalf("the token %s, as %s, changed its own permission: %d", name, header, status)
			}
		}
	}
	if mcpAPIPath("PATCH", path) {
		t.Fatal("a tool can reach connection settings")
	}
	if status, _ := doClient(t, http.DefaultClient, srv, "PATCH", path, `{"approveMerges":true}`, "Content-Type", "application/json"); status != 401 {
		t.Fatalf("a signed-out request: %d", status)
	}
	if status, _ := doClient(t, owner, srv, "PATCH", path, `{"approveMerges":true}`, "Content-Type", "application/json", "Origin", "https://evil.invalid"); status != 403 {
		t.Fatalf("a cross-site request: %d", status)
	}
	if listed(id) || factoryMergeApproved(t, s, user.ID, o.ID) {
		t.Fatal("a refused request gave the permission")
	}
	// The person allows it, and the next call is approved.
	mcpJSON(t, owner, srv, "PATCH", path, map[string]any{"approveMerges": true}, 200)
	if !listed(id) {
		t.Fatal("the permission was not saved")
	}
	if out := mcpData(t, session, "reet_approve_pipeline_merge", args); out["applied"] != true || !factoryMergeApproved(t, s, user.ID, o.ID) {
		t.Fatalf("approval with the permission: %v", out)
	}
	// Other changes leave it alone. A read-only connection cannot hold it:
	// narrowing takes it away and widening again does not bring it back.
	mcpJSON(t, owner, srv, "PATCH", path, map[string]any{"name": "Renamed"}, 200)
	if !listed(id) {
		t.Fatal("renaming took the permission away")
	}
	mcpJSON(t, owner, srv, "PATCH", path, map[string]any{"scope": "read", "approveMerges": true}, 200)
	if listed(id) {
		t.Fatal("a read-only connection holds the permission")
	}
	mcpJSON(t, owner, srv, "PATCH", path, map[string]any{"scope": "write"}, 200)
	if listed(id) {
		t.Fatal("widening brought the permission back")
	}
	again, args := factoryReadyToMerge(t, s, user.ID, "d")
	if text := mcpError(t, session, "reet_approve_pipeline_merge", args); !strings.Contains(text, "may not approve merges") || factoryMergeApproved(t, s, user.ID, again.ID) {
		t.Fatalf("approval after the permission was taken away: %s", text)
	}
	// An OAuth client is a connection like any other.
	_, q, verifier := oauthClientAndQuery(t, srv)
	q.Set("resource", "http://localhost:8080/mcp")
	consent := mcpJSON(t, owner, srv, "POST", "/api/v2/mcp/authorization", map[string]any{"query": q.Encode(), "approve": true, "scope": "write"}, 200)
	callback, _ := url.Parse(consent["redirect"].(string))
	tokens := oauthForm(t, srv, url.Values{"grant_type": {"authorization_code"}, "client_id": {q.Get("client_id")}, "redirect_uri": {q.Get("redirect_uri")}, "resource": {q.Get("resource")}, "code": {callback.Query().Get("code")}, "code_verifier": {verifier}}, 200)
	client := mcpSession(t, srv, tokens["access_token"].(string))
	_, c, ok, err := s.st.MCPUser(auth.HashToken(tokens["access_token"].(string)), time.Now().UnixMilli())
	if err != nil || !ok || c.Kind != "oauth" || c.ApproveMerges {
		t.Fatalf("OAuth connection: %+v %v %v", c, ok, err)
	}
	if text := mcpError(t, client, "reet_approve_pipeline_merge", args); !strings.Contains(text, "may not approve merges") || factoryMergeApproved(t, s, user.ID, again.ID) {
		t.Fatalf("OAuth approval without the permission: %s", text)
	}
	mcpJSON(t, owner, srv, "PATCH", fmt.Sprintf("/api/v2/mcp/connections/%d", c.ID), map[string]any{"approveMerges": true}, 200)
	if out := mcpData(t, client, "reet_approve_pipeline_merge", args); out["applied"] != true || !factoryMergeApproved(t, s, user.ID, again.ID) {
		t.Fatalf("OAuth approval with the permission: %v", out)
	}
	// Browser WebMCP is the signed-in person, and approves as it did.
	mcpJSON(t, owner, srv, "PATCH", "/api/v2/mcp", map[string]any{"browserEnabled": true, "browserScope": "write"}, 200)
	page, args := factoryReadyToMerge(t, s, user.ID, "e")
	mcpJSON(t, owner, srv, "POST", "/api/v2/mcp/browser/call", map[string]any{"name": "reet_approve_pipeline_merge", "arguments": args}, 200)
	if !factoryMergeApproved(t, s, user.ID, page.ID) {
		t.Fatal("the browser's approval was not recorded")
	}
}
