package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

// factoryMCPFixture is a controller with remote sessions on and one owner,
// whose Factory tools are called as a connection with management scope that
// the owner has allowed to approve merges.
func factoryMCPFixture(t *testing.T) (*server, store.User, mcpAccess) {
	t.Helper()
	s, _, _, user := factoryMCPServer(t)
	return s, user, mcpAccess{User: user, Connection: store.MCPConnection{ID: 2, Name: "manager", Scope: "write", ApproveMerges: true}}
}

// factoryRecord saves an outcome with one pipeline in the given state.
func factoryRecord(t *testing.T, s *server, uid int64, id, state string, change func(*store.GafferOutcomeSession)) store.GafferOutcome {
	t.Helper()
	o, err := s.st.CreateGafferOutcomeFor(uid, store.GafferOutcome{ID: strings.Repeat(id, 32), Objective: "Ship it.", Sessions: []store.GafferOutcomeSession{{Key: "a", Title: "Part", Prompt: "Do it.", RequestID: strings.Repeat("8", 32), Stages: []store.GafferOutcomeStage{
		{Kind: "code", Tool: "opencode", State: "done"}, {Kind: "merge", Tool: "opencode", State: "pending"},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	o, err = s.st.UpdateGafferOutcomeFor(uid, o.ID, o.Revision, func(o *store.GafferOutcome) bool {
		o.State, o.Sessions[0].State = "settled", state
		if change != nil {
			change(&o.Sessions[0])
		}
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// An approval over MCP is of the pipeline as it was read. Work that was sent
// back and done again since is a different thing to approve: the earlier
// approval, or one Gaffer asked for a while ago, does not carry over to it.
func TestFactoryMCPMergeApprovalIsOfThePipelineAsItWasRead(t *testing.T) {
	t.Parallel()
	s, user, write := factoryMCPFixture(t)
	ctx := context.Background()
	o := factoryRecord(t, s, user.ID, "c", "ready_to_merge", func(p *store.GafferOutcomeSession) { p.Stage, p.UpdatedAt = 1, 5000 })
	args := func(seen any) map[string]any {
		out := map[string]any{"outcome_id": o.ID, "pipeline": "a", "confirm": true}
		if seen != nil {
			out["seen_updated_at"] = seen
		}
		return out
	}
	for name, seen := range map[string]any{"no read": nil, "an older read": float64(4000)} {
		if _, err := s.executeMCPFactory(ctx, write, "reet_approve_pipeline_merge", args(seen)); err == nil || !strings.Contains(err.Error(), "Nothing was approved") {
			t.Fatalf("approval with %s: %v", name, err)
		}
		if now, err := s.st.GafferOutcomeFor(user.ID, o.ID); err != nil || now.Sessions[0].MergeApproved || now.Sessions[0].State != "ready_to_merge" {
			t.Fatalf("approval with %s changed the pipeline: %+v %v", name, now.Sessions[0], err)
		}
	}
	out, err := s.executeMCPFactory(ctx, write, "reet_approve_pipeline_merge", args(float64(5000)))
	if err != nil || out["applied"] != true {
		t.Fatalf("approval of what was read: %v %v", out, err)
	}
	if now, err := s.st.GafferOutcomeFor(user.ID, o.ID); err != nil || !now.Sessions[0].MergeApproved {
		t.Fatalf("approved: %+v %v", now.Sessions[0], err)
	}
}

// A control that changed nothing says so: stopping an outcome that has
// already finished is not reported as applied.
func TestFactoryMCPStopOfAFinishedOutcomeIsNotReportedAsApplied(t *testing.T) {
	t.Parallel()
	s, user, write := factoryMCPFixture(t)
	o := factoryRecord(t, s, user.ID, "d", "done", nil)
	out, err := s.executeMCPFactory(context.Background(), write, "reet_stop_outcome", map[string]any{"outcome_id": o.ID, "confirm": true})
	if err != nil || out["applied"] != false {
		t.Fatalf("stop of a finished outcome: %v %v", out, err)
	}
	if now, err := s.st.GafferOutcomeFor(user.ID, o.ID); err != nil || now.Revision != o.Revision {
		t.Fatalf("the finished record changed: %v", err)
	}
}

// An outcome is started over MCP only on a machine known to be able to run
// the agent as a chat. A machine that has not said what it can run is not
// taken to be able: the outcome is refused before it is saved.
func TestFactoryMCPStartIsRefusedWhenTheMachineHasNotSaidWhatItRuns(t *testing.T) {
	t.Parallel()
	s, user, write := factoryMCPFixture(t)
	agent, _, err := s.st.EnrollSyncAgent(user.ID, "dark-machine", "test")
	if err != nil {
		t.Fatal(err)
	}
	project, err := s.st.CreateFolderProjectFor(user.ID, store.FolderProject{Host: agent.ID, Dir: "/work/billing", Name: "billing", IsRepo: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.executeMCPFactory(context.Background(), write, "reet_start_outcome", map[string]any{
		"objective": "Ship CSV export.", "project_id": project.ID, "agent": "opencode", "confirm": true,
		"pipelines": []any{map[string]any{"title": "Export", "prompt": "Add CSV export. Do not merge."}},
	})
	if err == nil || !strings.Contains(err.Error(), "Nothing was started") {
		t.Fatalf("start on a machine that reported nothing: %v", err)
	}
	if rows, err := s.st.GafferOutcomesFor(user.ID); err != nil || len(rows) != 0 {
		t.Fatalf("an outcome was saved: %d %v", len(rows), err)
	}
}

// A client that did not hear back and asks again under the same request_id
// is answered with the outcome that request started, and told nothing more
// was started.
func TestFactoryMCPStartUnderARequestAlreadyUsedStartsNothingMore(t *testing.T) {
	t.Parallel()
	s, user, write := factoryMCPFixture(t)
	o := factoryRecord(t, s, user.ID, "e", "following", nil)
	out, err := s.executeMCPFactory(context.Background(), write, "reet_start_outcome", map[string]any{
		"objective": "Ship CSV export.", "project_id": "missing", "agent": "opencode", "confirm": true, "request_id": o.ID,
		"pipelines": []any{map[string]any{"title": "Export", "prompt": "Add CSV export. Do not merge."}},
	})
	if err != nil || out["applied"] != false || out["outcome"].(map[string]any)["id"] != o.ID {
		t.Fatalf("a repeated request: %v %v", out, err)
	}
	if rows, err := s.st.GafferOutcomesFor(user.ID); err != nil || len(rows) != 1 {
		t.Fatalf("outcomes after a repeated request: %d %v", len(rows), err)
	}
}

// The tools whose answers carry handover notes and reports are marked, so a
// change Gaffer asks for after reading them waits for the person.
func TestGafferToolsThatReturnSessionTextAreMarked(t *testing.T) {
	t.Parallel()
	s, user, _ := factoryMCPFixture(t)
	modes := map[string]string{}
	for _, tool := range mcpToolDefinitions {
		if gafferToolOffered(tool.Name) {
			modes[tool.Name] = "allow"
		}
	}
	if err := s.st.SetGafferToolModesFor(user.ID, modes); err != nil {
		t.Fatal(err)
	}
	marked := map[string]bool{}
	for _, tool := range (gafferToolHost{s}).GafferTools(user.ID) {
		marked[tool.Name] = tool.SessionText
	}
	if !marked["reet_get_factory_outcome"] || !marked["reet_report_outcome"] || !marked["reet_retry_pipeline"] {
		t.Fatalf("tools that return handovers or reports are not marked: %v", marked)
	}
	if marked["reet_list_factory"] || marked["reet_list_factory_projects"] || marked["reet_list_skills"] {
		t.Fatalf("tools that return no session text are marked: %v", marked)
	}
}

// An MCP client can say how demanding a stage is, which picks the agent and
// backup from the person's saved crew. It names no agent, and a tier for a
// stage the pipeline does not have, or a tier that is not one, is refused.
func TestFactoryMCPPipelinesCarryStageTiers(t *testing.T) {
	t.Parallel()
	got, err := mcpPipelines([]any{map[string]any{"title": "Export", "prompt": "p", "stages": []any{"code", "pr"}, "tiers": map[string]any{"code": "heavy"}}}, false)
	if err != nil || len(got) != 1 || len(got[0].Stages) != 2 || got[0].Stages[0].Tier != "heavy" || got[0].Stages[0].Tool != "" || got[0].Stages[1].Tier != "" {
		t.Fatalf("%+v %v", got, err)
	}
	// With no stages given the usual six are used, and a tier can still be set on one.
	if got, err = mcpPipelines([]any{map[string]any{"title": "Export", "prompt": "p", "tiers": map[string]any{"review": "light"}}}, false); err != nil || len(got[0].Stages) != 6 || got[0].Stages[1].Tier != "light" {
		t.Fatalf("%+v %v", got, err)
	}
	for _, bad := range []map[string]any{{"code": "enormous"}, {"design": "heavy"}} {
		if _, err = mcpPipelines([]any{map[string]any{"title": "Export", "prompt": "p", "stages": []any{"code"}, "tiers": bad}}, false); err == nil {
			t.Fatalf("accepted tiers %v", bad)
		}
	}
}

// An MCP client can split a stage into parts: they replace the stage, in
// order, and take the stage's tier unless they give their own.
func TestFactoryMCPPipelinesCanSplitAStage(t *testing.T) {
	t.Parallel()
	got, err := mcpPipelines([]any{map[string]any{"title": "Export", "prompt": "p", "stages": []any{"code", "pr"}, "tiers": map[string]any{"code": "heavy"},
		"parts": []any{map[string]any{"stage": "code", "title": "Query", "brief": "Write the query."}, map[string]any{"stage": "code", "title": "Button", "brief": "Add the button.", "tier": "light"}}}}, false)
	if err != nil || len(got[0].Stages) != 3 {
		t.Fatalf("%+v %v", got, err)
	}
	st := got[0].Stages
	if st[0].Part != "Query" || st[0].Tier != "heavy" || st[0].Brief != "Write the query." || st[1].Part != "Button" || st[1].Tier != "light" || st[2].Kind != "pr" || st[2].Part != "" {
		t.Fatalf("stages: %+v", st)
	}
	for name, parts := range map[string][]any{
		"one part":         {map[string]any{"stage": "code", "title": "Query", "brief": "b"}},
		"a stage it lacks": {map[string]any{"stage": "design", "title": "A", "brief": "a"}, map[string]any{"stage": "design", "title": "B", "brief": "b"}},
	} {
		if _, err = mcpPipelines([]any{map[string]any{"title": "Export", "prompt": "p", "stages": []any{"code"}, "parts": parts}}, false); err == nil {
			t.Fatalf("accepted: %s", name)
		}
	}
}

// The exported schema lets a client give a tier for some stages only: no
// stage is required, as the tool's own description says.
func TestTheStartToolsTiersAreAPartialRecord(t *testing.T) {
	t.Parallel()
	for _, tool := range mcpToolDefinitions {
		if tool.Name != "reet_start_outcome" {
			continue
		}
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema struct {
			Properties struct {
				Pipelines struct {
					Items struct {
						Properties struct {
							Tiers struct {
								Required []string `json:"required"`
							} `json:"tiers"`
						} `json:"properties"`
					} `json:"items"`
				} `json:"pipelines"`
			} `json:"properties"`
		}
		if err = json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		if got := schema.Properties.Pipelines.Items.Properties.Tiers.Required; len(got) != 0 {
			t.Fatalf("tiers requires %v", got)
		}
		return
	}
	t.Fatal("reet_start_outcome is not exported")
}
