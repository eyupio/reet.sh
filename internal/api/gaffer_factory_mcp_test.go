package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
)

// The Factory's MCP tools read for any writer's connection and control only
// for one with management scope; a read-only account gets neither.
func TestFactoryMCPToolsReadForWritersAndControlOnlyWithManagementScope(t *testing.T) {
	t.Parallel()
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
		if _, err := s.executeMCPFactory(ctx, viewer, name, map[string]any{"outcome_id": missing}); !errors.Is(err, errFactoryForbidden) {
			t.Fatalf("viewer %s: %v", name, err)
		}
	}
	for name := range mcpFactoryWrites {
		args := map[string]any{"outcome_id": missing, "pipeline": "a", "mode": "by_itself", "confirm": true}
		if _, err := s.executeMCPFactory(ctx, read, name, args); !errors.Is(err, errFactoryForbidden) {
			t.Fatalf("read scope %s: %v", name, err)
		}
		if _, err := s.executeMCPFactory(ctx, viewer, name, args); !errors.Is(err, errFactoryForbidden) {
			t.Fatalf("viewer %s: %v", name, err)
		}
		// With the scope, a control still changes nothing for an outcome that is not there.
		if out, err := s.executeMCPFactory(ctx, write, name, args); err == nil {
			t.Fatalf("%s on a missing outcome: %v", name, out)
		}
	}
	// What cannot be undone, or lets a merge happen unasked, needs the explicit confirmation.
	for _, name := range []string{"reet_tell_pipeline_agent", "reet_move_pipeline_on", "reet_approve_pipeline_merge", "reet_stop_outcome", "reet_start_outcome", "reet_add_pipelines", "reet_send_pipeline_back"} {
		_, err := s.executeMCPFactory(ctx, write, name, map[string]any{"outcome_id": missing, "pipeline": "a", "mode": "by_itself"})
		if err == nil || !strings.Contains(err.Error(), "confirm=true") {
			t.Fatalf("%s without confirm: %v", name, err)
		}
	}
	// A missing outcome is named as such, not as a skill.
	if _, err := s.executeMCPFactory(ctx, write, "reet_get_factory_outcome", map[string]any{"outcome_id": missing}); err == nil || !strings.Contains(err.Error(), "That outcome is not available") {
		t.Fatalf("missing outcome: %v", err)
	}
	// An outcome on YOLO has agents that act without approvals: a client cannot
	// tell them, add to them or send their work back.
	yolo, err := s.st.CreateGafferOutcomeFor(user.ID, store.GafferOutcome{ID: strings.Repeat("b", 32), Objective: "Ship it.", Yolo: true, Sessions: []store.GafferOutcomeSession{{Key: "a", Title: "Part", Prompt: "Do it.", RequestID: strings.Repeat("8", 32)}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"reet_tell_pipeline_agent", "reet_add_pipelines", "reet_send_pipeline_back"} {
		_, err := s.executeMCPFactory(ctx, write, name, map[string]any{"outcome_id": yolo.ID, "pipeline": "a", "confirm": true, "stage": "code", "reason": "Again.", "text": "Do more.", "request_id": strings.Repeat("c", 32), "pipelines": []any{map[string]any{"title": "More", "brief": "Do more."}}})
		if err == nil || !strings.Contains(err.Error(), "runs on YOLO") {
			t.Fatalf("%s on a YOLO outcome: %v", name, err)
		}
	}
	// A fenced controller accepts no Factory change over MCP, as its browser routes accept none.
	if _, err := s.st.SetFence("https://new.example"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.executeMCPFactory(ctx, write, "reet_stop_outcome", map[string]any{"outcome_id": yolo.ID, "confirm": true}); err == nil || strings.Contains(err.Error(), "confirm") {
		t.Fatalf("a write on a fenced controller: %v", err)
	}
	if _, err := s.executeMCPFactory(ctx, write, "reet_list_factory", map[string]any{}); err != nil {
		t.Fatalf("a read on a fenced controller: %v", err)
	}
	if after, err := s.st.GafferOutcomeFor(user.ID, yolo.ID); err != nil || after.Revision != yolo.Revision {
		t.Fatalf("the fenced record changed: %+v %v", after, err)
	}
	if _, err := s.st.LiftFence(); err != nil {
		t.Fatal(err)
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
