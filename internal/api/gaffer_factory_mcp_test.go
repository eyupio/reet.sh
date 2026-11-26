package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
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
	write := mcpAccess{User: user, Connection: store.MCPConnection{ID: 2, Name: "manager", Scope: "write", ApproveMerges: true}}
	viewer := mcpAccess{User: store.User{ID: user.ID, Role: "viewer"}, Connection: store.MCPConnection{ID: 3, Name: "viewer", Scope: "write"}}
	ctx := context.Background()
	for _, access := range []mcpAccess{read, write} {
		out, err := s.executeMCPFactory(ctx, access, "reet_list_factory", map[string]any{})
		if err != nil || len(out["items"].([]any)) != 0 {
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
	// Nor try a pipeline again, move it on or approve its merge: each sets
	// an agent that asks nobody going, or lets it merge.
	for _, name := range []string{"reet_tell_pipeline_agent", "reet_add_pipelines", "reet_send_pipeline_back", "reet_retry_pipeline", "reet_move_pipeline_on", "reet_approve_pipeline_merge"} {
		_, err := s.executeMCPFactory(ctx, write, name, map[string]any{"outcome_id": yolo.ID, "pipeline": "a", "confirm": true, "stage": "code", "reason": "Again.", "text": "Do more.", "request_id": strings.Repeat("c", 32), "seen_updated_at": float64(1), "pipelines": []any{map[string]any{"title": "More", "brief": "Do more."}}})
		if err == nil || !strings.Contains(err.Error(), "runs on YOLO") {
			t.Fatalf("%s on a YOLO outcome: %v", name, err)
		}
	}
	if after, err := s.st.GafferOutcomeFor(user.ID, yolo.ID); err != nil || after.Revision != yolo.Revision {
		t.Fatalf("a refused control changed the YOLO outcome: %+v %v", after, err)
	}
	// It can still be read, made to wait for approval and stopped.
	if _, err := s.executeMCPFactory(ctx, write, "reet_get_factory_outcome", map[string]any{"outcome_id": yolo.ID}); err != nil {
		t.Fatalf("reading a YOLO outcome: %v", err)
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

// A client reads, beside what a stage's session is on, the effort that was
// chosen for it when its agent did not offer that effort for the model: the
// stage started on the agent's default, its choice confirmed with the reason.
func TestFactoryMCPSaysWhichEffortWasNotOffered(t *testing.T) {
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
	note := "You chose the effort high; OpenCode does not offer it for this model here, so the session was started on the agent's default effort."
	o, err := st.CreateGafferOutcomeFor(user.ID, store.GafferOutcome{ID: strings.Repeat("b", 32), Objective: "Ship it.", Sessions: []store.GafferOutcomeSession{{Key: "a", Title: "Part", Prompt: "Do it.", RequestID: strings.Repeat("8", 32), Stages: []store.GafferOutcomeStage{
		{Kind: "code", Tool: "opencode", Model: "cheap/coder", EffortNotOffered: "high", AppliedTool: "opencode", AppliedModel: "cheap/coder", AppliedEffort: "medium", ChoiceCheck: "applied", ChoiceNote: note, RequestID: strings.Repeat("8", 32)},
		{Kind: "review", Tool: "opencode", Effort: "low", RequestID: strings.Repeat("9", 32)},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	read := mcpAccess{User: user, Connection: store.MCPConnection{ID: 1, Name: "reader", Scope: "read"}}
	out, err := h.s.executeMCPFactory(context.Background(), read, "reet_get_factory_outcome", map[string]any{"outcome_id": o.ID})
	if err != nil {
		t.Fatal(err)
	}
	stages := out["outcome"].(map[string]any)["pipelines"].([]map[string]any)[0]["stages"].([]map[string]any)
	code, review := stages[0], stages[1]
	if code["effort_not_offered"] != "high" || code["effort"] != nil || code["choice"] != "applied" || code["applied_effort"] != "medium" || code["choice_note"] != note {
		t.Fatalf("code stage: %v", code)
	}
	if _, ok := review["effort_not_offered"]; ok || review["effort"] != "low" {
		t.Fatalf("review stage: %v", review)
	}
}

// factoryMCPPathways is factoryMCPFixture with a default Gaffer provider, a
// project whose machine runs OpenCode chats, and the enabled pathway Careful:
// code, a custom Audit stage with a skill, and pr.
func factoryMCPPathways(t *testing.T) (*server, store.User, mcpAccess, string, store.GafferPathway) {
	t.Helper()
	s, srv, _, user := factoryMCPServer(t)
	write := mcpAccess{User: user, Connection: store.MCPConnection{ID: 2, Name: "manager", Scope: "write"}}
	if _, err := s.gafferProviders.Save(gaffer.Principal{UserID: user.ID, CanUseLocal: true}, gaffer.Input{Name: "Brains", Kind: "openai_compatible", BaseURL: srv.URL, LocalOnly: true, Enabled: true, MakeDefault: true, Model: "fake-model"}); err != nil {
		t.Fatal(err)
	}
	agent, _, err := s.st.EnrollSyncAgent(user.ID, "machine", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.st.BeginRemoteConnectionFor(user.ID, agent.ID, strings.Repeat("1", 32), terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", TerminalEnabled: true, TerminalControl: true, Readiness: terminalproto.Ready, ACP: true}); err != nil {
		t.Fatal(err)
	}
	project, err := s.st.CreateFolderProjectFor(user.ID, store.FolderProject{Host: agent.ID, Dir: "/work/billing", Name: "billing", IsRepo: true})
	if err != nil {
		t.Fatal(err)
	}
	careful := savePathway(t, s, user.ID, "Careful", true, store.GafferPathwayStage{Kind: "code"}, store.GafferPathwayStage{Kind: "custom", Name: "Audit", Prompt: "SECRET_AUDIT_PROMPT", Skills: []string{"builtin:review"}}, store.GafferPathwayStage{Kind: "pr"})
	return s, user, write, project.ID, careful
}

func savePathway(t *testing.T, s *server, uid int64, name string, enabled bool, stages ...store.GafferPathwayStage) store.GafferPathway {
	t.Helper()
	p, err := s.gafferProviders.SavePathway(uid, store.GafferPathway{Name: name, Description: name + " work.", Enabled: enabled, Stages: stages}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func pathwayStartArgs(project, pathway string, pipeline map[string]any) map[string]any {
	if pipeline == nil {
		pipeline = map[string]any{"title": "Logs", "prompt": "Tidy the logs. Do not merge."}
	}
	return map[string]any{"objective": "Tidy the logs.", "project_id": project, "agent": "opencode", "confirm": true, "pathway": pathway, "pipelines": []any{pipeline}}
}

func stageLabels(piece store.GafferOutcomeSession) string {
	var labels []string
	for _, st := range piece.Stages {
		labels = append(labels, st.Label())
	}
	return strings.Join(labels, ",")
}

// A client lists the account's enabled pathways with their stages by name,
// never a stage's prompt, and never another account's.
func TestMCPListsEnabledPathways(t *testing.T) {
	t.Parallel()
	s, user, write, _, _ := factoryMCPPathways(t)
	savePathway(t, s, user.ID, "Quick", true, store.GafferPathwayStage{Kind: "code"}, store.GafferPathwayStage{Kind: "pr"})
	savePathway(t, s, user.ID, "Old", false, store.GafferPathwayStage{Kind: "code"})
	other, err := s.st.CreateUser("other@example.test", "Other", "member", "")
	if err != nil {
		t.Fatal(err)
	}
	savePathway(t, s, other.ID, "Theirs", true, store.GafferPathwayStage{Kind: "code"})
	out, err := s.executeMCPFactory(context.Background(), write, "reet_list_pathways", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	var got struct {
		Pathways []struct {
			ID, Name, Description string
			Stages                []string
			Problems              []string
		}
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Pathways) != 2 || got.Pathways[0].Name != "Careful" || got.Pathways[1].Name != "Quick" {
		t.Fatalf("pathways: %s", raw)
	}
	if strings.Join(got.Pathways[0].Stages, ",") != "code,Audit,pr" || got.Pathways[0].Description != "Careful work." || got.Pathways[0].ID == "" || got.Pathways[0].Problems == nil {
		t.Fatalf("careful: %s", raw)
	}
	if strings.Contains(string(raw), "SECRET_AUDIT_PROMPT") {
		t.Fatalf("a stage's prompt was listed: %s", raw)
	}
	if !mcpFactoryReads["reet_list_pathways"] || gafferToolReturnsSessionText("reet_list_pathways") {
		t.Fatal("reet_list_pathways is a Factory read that returns no session text")
	}
}

// A client starts an outcome on a pathway by its id or by its name in any
// case: the pipelines get the pathway's stages and its skills' receipts.
func TestMCPStartsAnOutcomeOnAPathway(t *testing.T) {
	t.Parallel()
	for name, by := range map[string]func(store.GafferPathway) string{
		"id":   func(p store.GafferPathway) string { return p.ID },
		"name": func(store.GafferPathway) string { return "careful" },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, user, write, project, careful := factoryMCPPathways(t)
			out, err := s.executeMCPFactory(context.Background(), write, "reet_start_outcome", pathwayStartArgs(project, by(careful), nil))
			if err != nil || out["applied"] != true {
				t.Fatalf("start: %v %v", out, err)
			}
			rows, err := s.st.GafferOutcomesFor(user.ID)
			if err != nil || len(rows) != 1 {
				t.Fatalf("outcomes: %d %v", len(rows), err)
			}
			piece := rows[0].Sessions[0]
			if stageLabels(piece) != "code,Audit,pr" || piece.PathwayName != "Careful" {
				t.Fatalf("stages %s pathway %q", stageLabels(piece), piece.PathwayName)
			}
			if !stageSkillShown(t, s.st, rows[0].ID, piece.Key, 1, "builtin:review") {
				t.Fatal("the custom stage's skill has no receipt")
			}
		})
	}
}

// A pathway and stages of a pipeline's own are refused together, as are a
// pathway that does not exist and one that is switched off, with the names of
// those that are on. Nothing is started.
func TestMCPPathwayStartRefusals(t *testing.T) {
	t.Parallel()
	s, user, write, project, careful := factoryMCPPathways(t)
	savePathway(t, s, user.ID, "Off", false, store.GafferPathwayStage{Kind: "code"})
	savePathway(t, s, user.ID, "Quick", true, store.GafferPathwayStage{Kind: "code"})
	own := func(key string, v any) map[string]any {
		return map[string]any{"title": "Logs", "prompt": "Tidy the logs.", key: v}
	}
	for name, c := range map[string]struct {
		args map[string]any
		want string
	}{
		"stages":  {pathwayStartArgs(project, careful.ID, own("stages", []any{"code"})), "A pipeline takes a pathway or its own stages, not both. Nothing was started."},
		"tiers":   {pathwayStartArgs(project, careful.ID, own("tiers", map[string]any{"code": "heavy"})), "A pipeline takes a pathway or its own stages, not both. Nothing was started."},
		"parts":   {pathwayStartArgs(project, careful.ID, own("parts", []any{map[string]any{"stage": "code", "title": "A", "brief": "a"}})), "A pipeline takes a pathway or its own stages, not both. Nothing was started."},
		"unknown": {pathwayStartArgs(project, "Hasty", nil), "No pathway called “Hasty” is switched on. The ones that are: Careful, Quick. Nothing was started."},
		"off":     {pathwayStartArgs(project, "off", nil), "No pathway called “off” is switched on. The ones that are: Careful, Quick. Nothing was started."},
	} {
		if _, err := s.executeMCPFactory(context.Background(), write, "reet_start_outcome", c.args); err == nil || err.Error() != c.want {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if rows, err := s.st.GafferOutcomesFor(user.ID); err != nil || len(rows) != 0 {
		t.Fatalf("an outcome was saved: %d %v", len(rows), err)
	}
}

// Pipelines added on a pathway get its stages and receipts too.
func TestMCPAddsPipelinesOnAPathway(t *testing.T) {
	t.Parallel()
	s, user, write, project, careful := factoryMCPPathways(t)
	ctx := context.Background()
	if _, err := s.executeMCPFactory(ctx, write, "reet_start_outcome", pathwayStartArgs(project, "", nil)); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.st.GafferOutcomesFor(user.ID)
	out, err := s.executeMCPFactory(ctx, write, "reet_add_pipelines", map[string]any{"outcome_id": rows[0].ID, "confirm": true, "pathway": "Careful",
		"pipelines": []any{map[string]any{"title": "Metrics", "prompt": "Tidy the metrics. Do not merge."}}})
	if err != nil || out["applied"] != true {
		t.Fatalf("add: %v %v", out, err)
	}
	o, err := s.st.GafferOutcomeFor(user.ID, rows[0].ID)
	if err != nil || len(o.Sessions) != 2 {
		t.Fatalf("outcome: %+v %v", o, err)
	}
	if stageLabels(o.Sessions[1]) != "code,Audit,pr" || o.Sessions[1].PathwayID != careful.ID || !stageSkillShown(t, s.st, o.ID, o.Sessions[1].Key, 1, "builtin:review") {
		t.Fatalf("added: %s %q", stageLabels(o.Sessions[1]), o.Sessions[1].PathwayID)
	}
	if o.Sessions[0].PathwayID != "" {
		t.Fatalf("the first pipeline is on a pathway: %q", o.Sessions[0].PathwayID)
	}
}

// A start with no pathway is as it always was: the usual six stages.
func TestMCPStandardStartStillDefaultsItsStages(t *testing.T) {
	t.Parallel()
	s, user, write, project, _ := factoryMCPPathways(t)
	if _, err := s.executeMCPFactory(context.Background(), write, "reet_start_outcome", pathwayStartArgs(project, "", nil)); err != nil {
		t.Fatal(err)
	}
	rows, err := s.st.GafferOutcomesFor(user.ID)
	if err != nil || len(rows) != 1 || stageLabels(rows[0].Sessions[0]) != "code,review,test,pr,babysit,merge" || rows[0].Sessions[0].PathwayID != "" {
		t.Fatalf("standard start: %+v %v", rows, err)
	}
}

// A custom stage is read by its name, sent back to by it, and a stage the
// pipeline does not have is refused with the names of those it has.
func TestMCPNamesACustomStage(t *testing.T) {
	t.Parallel()
	s, user, write, _, careful := factoryMCPPathways(t)
	ctx := context.Background()
	o := factoryRecord(t, s, user.ID, "f", "following", func(p *store.GafferOutcomeSession) {
		p.Stage, p.PathwayID, p.PathwayName = 2, careful.ID, careful.Name
		p.Stages = []store.GafferOutcomeStage{{Kind: "code", Tool: "opencode", State: "done"}, {Kind: "custom", Name: "Audit", Prompt: "SECRET_AUDIT_PROMPT", Tool: "opencode", State: "done"}, {Kind: "pr", Tool: "opencode", State: "pending"}}
	})
	out, err := s.executeMCPFactory(ctx, write, "reet_get_factory_outcome", map[string]any{"outcome_id": o.ID})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if !strings.Contains(string(raw), `"kind":"custom","name":"Audit"`) || !strings.Contains(string(raw), `"pathway":"Careful"`) || strings.Contains(string(raw), "SECRET_AUDIT_PROMPT") {
		t.Fatalf("outcome view: %s", raw)
	}
	for _, tool := range []string{"reet_send_pipeline_back", "reet_move_pipeline_on"} {
		_, err := s.executeMCPFactory(ctx, write, tool, map[string]any{"outcome_id": o.ID, "pipeline": "a", "stage": "Zed", "reason": "Again.", "confirm": true})
		if err == nil || err.Error() != "That pipeline has no “Zed” stage. Its stages are: code, Audit, pr. Nothing was changed." {
			t.Fatalf("%s to Zed: %v", tool, err)
		}
	}
	if _, err := s.executeMCPFactory(ctx, write, "reet_send_pipeline_back", map[string]any{"outcome_id": o.ID, "pipeline": "a", "stage": "Audit", "reason": "Look again.", "confirm": true}); err != nil {
		t.Fatalf("send back to Audit: %v", err)
	}
	if now, err := s.st.GafferOutcomeFor(user.ID, o.ID); err != nil || now.Sessions[0].Stage != 1 || now.Sessions[0].Stages[1].State != "pending" {
		t.Fatalf("after send-back: %+v %v", now.Sessions[0], err)
	}
}
