package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func contextAPIJSON(t *testing.T, c *http.Client, ts *httptest.Server, method, path string, in any, want int) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(in)
	code, body := doClient(t, c, ts, method, path, string(raw), "Content-Type", "application/json", "Origin", ts.URL)
	if code != want {
		t.Fatalf("%s %s: got %d want %d: %s", method, path, code, want, body)
	}
	var out map[string]any
	if json.Unmarshal([]byte(body), &out) != nil {
		t.Fatal(body)
	}
	return out
}

func contextAPISession(t *testing.T, st *store.Store, uid int64) store.TerminalSession {
	t.Helper()
	a, _, err := st.EnrollSyncAgent(uid, "context-reader-machine", "test")
	if err != nil {
		t.Fatal(err)
	}
	caps, err := st.BeginRemoteConnectionFor(uid, a.ID, strings.Repeat("1", 32), terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", TerminalEnabled: true, TerminalControl: true, Readiness: terminalproto.Ready, ACP: true})
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := st.ReserveTerminalCreateFor(uid, a.ID, strings.Repeat("2", 32), caps.Generation, terminalproto.Create{Title: "SELECTED_TITLE", Tool: "opencode", Model: "worker", Dir: "/PRIVATE_DIRECTORY", Structured: true, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.AppendTerminalChatEventFor(uid, session.ID, 1, time.Now().UnixMilli(), []byte(`{"kind":"user","text":"SELECTED_WORKER_TEXT"}`)); err != nil {
		t.Fatal(err)
	}
	return session
}

func TestContextMCPAndBrowserGrantsRemainSeparateAndRecheckAccess(t *testing.T) {
	t.Parallel()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(nil)
	h := New(st, meta.Fetcher{}, "", WithAuthMode(AuthAccounts), WithPublicURL("http://"+ts.Listener.Addr().String()))
	ts.Config.Handler = h
	ts.Start()
	t.Cleanup(func() { ts.Close(); h.Close(); st.Close() })
	browser := &http.Client{Jar: mustJar(t)}
	contextAPIJSON(t, browser, ts, "POST", "/api/v2/auth/setup", map[string]any{"name": "Owner", "email": "context-owner@example.test", "password": "long context password"}, 200)
	u, _, err := st.OwnerUser()
	if err != nil {
		t.Fatal(err)
	}
	selected := contextAPISession(t, st, u.ID)
	token, id := mcpManual(t, browser, ts, "read")
	client := mcpSession(t, ts, token)
	// Null is invalid, but must be rejected without taking down the controller.
	bad, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "reet_list_context_access", Arguments: map[string]any(nil)})
	if err == nil && !bad.IsError {
		t.Fatal("null arguments were not rejected")
	}
	empty, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "reet_list_context_access"})
	if err != nil || empty.IsError {
		t.Fatal("omitted arguments did not mean an empty object", err, empty)
	}
	denied := func(name string, args map[string]any) {
		t.Helper()
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err == nil && !result.IsError {
			t.Fatal("ungranted read succeeded", name, result)
		}
	}
	denied("reet_get_context", map[string]any{})
	if data := mcpData(t, client, "reet_list_context_access", map[string]any{}); len(data["scopes"].([]any)) != 0 {
		t.Fatal("shelf scope implied context", data)
	}
	path := fmt.Sprintf("/api/v2/mcp/connections/%d/context", id)
	contextAPIJSON(t, browser, ts, "PUT", path, map[string]any{"revision": 0, "status": true}, 200)
	fleet := mcpData(t, client, "reet_get_context", map[string]any{})
	raw, _ := json.Marshal(fleet)
	if !strings.Contains(string(raw), "controller_retained_not_live") || strings.Contains(string(raw), "SELECTED_") {
		t.Fatal("account projection", string(raw))
	}
	denied("reet_get_context", map[string]any{"session_id": selected.ID})
	sessionPath := path + "?sessionId=" + selected.ID
	contextAPIJSON(t, browser, ts, "PUT", sessionPath, map[string]any{"revision": 0, "history": true}, 200)
	metadata := mcpData(t, client, "reet_get_context", map[string]any{"session_id": selected.ID})
	raw, _ = json.Marshal(metadata)
	if strings.Contains(string(raw), "SELECTED_WORKER_TEXT") || strings.Contains(string(raw), "SELECTED_TITLE") || strings.Contains(string(raw), "PRIVATE_DIRECTORY") {
		t.Fatal("metadata leaked text", string(raw))
	}
	otherToken, _ := mcpManual(t, browser, ts, "write")
	other := mcpSession(t, ts, otherToken)
	r, err := other.CallTool(context.Background(), &mcp.CallToolParams{Name: "reet_get_context", Arguments: map[string]any{"session_id": selected.ID}})
	if err == nil && !r.IsError {
		t.Fatal("another client inherited a grant")
	}
	mcpJSON(t, browser, ts, "PATCH", "/api/v2/mcp", map[string]any{"browserEnabled": true}, 200)
	call := map[string]any{"name": "reet_get_context", "arguments": map[string]any{"session_id": selected.ID}}
	contextAPIJSON(t, browser, ts, "POST", "/api/v2/mcp/browser/call", call, 400)
	contextAPIJSON(t, browser, ts, "PUT", "/api/v2/mcp/browser/context?sessionId="+selected.ID, map[string]any{"revision": 0, "history": true, "text": true}, 200)
	got := contextAPIJSON(t, browser, ts, "POST", "/api/v2/mcp/browser/call", call, 200)
	raw, _ = json.Marshal(got)
	if !strings.Contains(string(raw), "SELECTED_WORKER_TEXT") {
		t.Fatal("browser grant was not applied", string(raw))
	}
	metadata = mcpData(t, client, "reet_get_context", map[string]any{"session_id": selected.ID})
	raw, _ = json.Marshal(metadata)
	if strings.Contains(string(raw), "SELECTED_WORKER_TEXT") {
		t.Fatal("browser grant widened a hosted credential")
	}
	// MCP credentials cannot grant themselves context, even with a browser cookie.
	code, _ := do(t, ts, "PUT", sessionPath, `{"revision":1,"text":true,"history":true}`, "Content-Type", "application/json", "Authorization", "Bearer "+token, "Origin", ts.URL)
	if code != 403 {
		t.Fatal("credential configured a grant", code)
	}
	code, _ = doClient(t, browser, ts, "PUT", sessionPath, `{"revision":1,"history":true}`, "Content-Type", "application/json", "Origin", "https://other.example")
	if code != 403 {
		t.Fatal("cross-site grant", code)
	}
	if err = st.UpdateUserRole(u.ID, "viewer"); err != nil {
		t.Fatal(err)
	}
	denied("reet_get_context", map[string]any{"session_id": selected.ID})
	contextAPIJSON(t, browser, ts, "GET", sessionPath, nil, 403)
}
