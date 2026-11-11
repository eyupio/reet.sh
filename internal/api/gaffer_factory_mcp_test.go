package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
)

// The Factory's MCP tools read for any writer's connection and control only
// for one with management scope; a read-only account gets neither.
func TestFactoryMCPToolsReadForWritersAndControlOnlyWithManagementScope(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seal, _ := sealed.NewSealer(strings.Repeat("gaffer-test-", 8), "test")
	h := New(st, meta.Fetcher{}, "bootstrap", WithAuthMode(AuthAccounts), WithPublicURL("http://localhost:8080"), WithGafferProviders(seal))
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
	s := h.s
	read := mcpAccess{User: user, Connection: store.MCPConnection{ID: 1, Name: "reader", Scope: "read"}}
	write := mcpAccess{User: user, Connection: store.MCPConnection{ID: 2, Name: "manager", Scope: "write"}}
	viewer := mcpAccess{User: store.User{ID: user.ID, Role: "viewer"}, Connection: store.MCPConnection{ID: 3, Name: "viewer", Scope: "write"}}
	ctx := context.Background()
	for _, access := range []mcpAccess{read, write} {
		out, err := s.executeMCPFactory(ctx, access, "reet_list_factory", map[string]any{})
		if err != nil || len(out["outcomes"].([]map[string]any)) != 0 {
			t.Fatalf("list as %s: %v %v", access.Connection.Name, out, err)
		}
	}
	missing := strings.Repeat("a", 32)
	for name := range mcpFactoryReads {
		if _, err := s.executeMCPFactory(ctx, viewer, name, map[string]any{"outcome_id": missing}); !errors.Is(err, gaffer.ErrContextForbidden) {
			t.Fatalf("viewer %s: %v", name, err)
		}
	}
	for name := range mcpFactoryWrites {
		args := map[string]any{"outcome_id": missing, "pipeline": "a", "mode": "by_itself", "confirm": true}
		if _, err := s.executeMCPFactory(ctx, read, name, args); !errors.Is(err, gaffer.ErrContextForbidden) {
			t.Fatalf("read scope %s: %v", name, err)
		}
		if _, err := s.executeMCPFactory(ctx, viewer, name, args); !errors.Is(err, gaffer.ErrContextForbidden) {
			t.Fatalf("viewer %s: %v", name, err)
		}
		// With the scope, a control still changes nothing for an outcome that is not there.
		if out, err := s.executeMCPFactory(ctx, write, name, args); err == nil {
			t.Fatalf("%s on a missing outcome: %v", name, out)
		}
	}
	// What cannot be undone, or lets a merge happen unasked, needs the explicit confirmation.
	for _, name := range []string{"reet_move_pipeline_on", "reet_approve_pipeline_merge", "reet_stop_outcome", "reet_start_outcome", "reet_add_pipelines", "reet_send_pipeline_back"} {
		_, err := s.executeMCPFactory(ctx, write, name, map[string]any{"outcome_id": missing, "pipeline": "a", "mode": "by_itself"})
		if err == nil || !strings.Contains(err.Error(), "confirm=true") {
			t.Fatalf("%s without confirm: %v", name, err)
		}
	}
	// A client can make a pipeline wait, never let it merge by itself.
	if _, err := s.executeMCPFactory(ctx, write, "reet_set_pipeline_merge", map[string]any{"outcome_id": missing, "pipeline": "a", "mode": "by_itself", "confirm": true}); err == nil || !strings.Contains(err.Error(), "only be made to wait") {
		t.Fatalf("merge by itself over MCP: %v", err)
	}
	// Every Factory tool is defined, and only the two reads are marked read-only.
	seen := 0
	for _, tool := range mcpToolDefinitions {
		if !mcpFactoryTool(tool.Name) {
			continue
		}
		seen++
		if tool.Annotations.ReadOnlyHint != mcpFactoryReads[tool.Name] {
			t.Fatalf("%s read-only hint is %v", tool.Name, tool.Annotations.ReadOnlyHint)
		}
	}
	if seen != len(mcpFactoryReads)+len(mcpFactoryWrites) {
		t.Fatalf("defined Factory tools: %d", seen)
	}
}
