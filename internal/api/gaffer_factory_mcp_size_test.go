package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

// factoryBulkOutcome saves an outcome of n pipelines of six stages, as a real
// run leaves one: every stage with its agent, tier, backup, read-back choice,
// timings and token figures, and a handover of the largest size a done stage keeps.
func factoryBulkOutcome(t *testing.T, s *server, uid int64, id string, n int, state string) store.GafferOutcome {
	t.Helper()
	pipelines := []store.GafferOutcomeSession{}
	for i := range n {
		stages := []store.GafferOutcomeStage{}
		for _, kind := range []string{"code", "review", "test", "pr", "babysit", "merge"} {
			stages = append(stages, store.GafferOutcomeStage{
				Kind: kind, Tool: "claude", Model: "claude-opus-4", Effort: "high", Tier: "heavy", Backup: &store.GafferStageAgent{Tool: "codex"},
				AppliedTool: "claude", AppliedModel: "claude-opus-4", AppliedEffort: "high", ChoiceCheck: "applied", ChoiceNote: "Read back from the session: nothing differs.",
				StartedAt: 1_700_000_000_000, EndedAt: 1_700_000_900_000, ClosedBy: "gaffer", AgentUsageReported: true, AgentInputTokens: 120_000, AgentOutputTokens: 9_000, AgentCachedTokens: 80_000,
				RequestID: strings.Repeat(string(rune('1'+i)), 32), Summary: strings.Repeat("h", 4000),
			})
		}
		pipelines = append(pipelines, store.GafferOutcomeSession{Key: string(rune('a' + i)), Title: "Pipeline " + string(rune('A'+i)), Prompt: "Do it.", RequestID: strings.Repeat(string(rune('1'+i)), 32), Stages: stages})
	}
	o, err := s.st.CreateGafferOutcomeFor(uid, store.GafferOutcome{ID: strings.Repeat(id, 32), Objective: strings.Repeat("Ship the whole export feature. ", 20), Sessions: pipelines})
	if err != nil {
		t.Fatal(err)
	}
	o, err = s.st.UpdateGafferOutcomeFor(uid, o.ID, o.Revision, func(o *store.GafferOutcome) bool {
		o.State = "settled"
		for i := range o.Sessions {
			o.Sessions[i].State, o.Sessions[i].Stage, o.Sessions[i].UpdatedAt = state, 5, 5000
			for j := range o.Sessions[i].Stages {
				o.Sessions[i].Stages[j].State = "done"
			}
			o.Sessions[i].Stages[5].State = "pending"
		}
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestMeasureFactoryReplies(t *testing.T) {
	s, user, write := factoryMCPFixture(t)
	ctx := context.Background()
	for _, n := range []int{1, 3, 5} {
		o := factoryBulkOutcome(t, s, user.ID, string(rune('0'+n)), n, "ready_to_merge")
		out, err := s.executeMCPFactory(ctx, write, "reet_get_factory_outcome", map[string]any{"outcome_id": o.ID})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(map[string]any{"data": out})
		t.Logf("get %d pipelines: %d bytes", n, len(raw))
		readonly := mcpAccess{User: user, Connection: store.MCPConnection{ID: 1, Name: "r", Scope: "read"}}
		out, _ = s.executeMCPFactory(ctx, readonly, "reet_get_factory_outcome", map[string]any{"outcome_id": o.ID})
		raw, _ = json.Marshal(map[string]any{"data": out})
		t.Logf("get %d pipelines, read scope (no handovers): %d bytes", n, len(raw))
		args := map[string]any{"outcome_id": o.ID, "pipeline": "a", "confirm": true, "seen_updated_at": float64(5000)}
		out, err = s.executeMCPFactory(ctx, write, "reet_approve_pipeline_merge", args)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ = json.Marshal(map[string]any{"data": out})
		t.Logf("approve %d pipelines: %d bytes", n, len(raw))
	}
	for i := range 6 {
		factoryBulkOutcome(t, s, user.ID, string(rune('a'+i)), 3, "done")
	}
	out, err := s.executeMCPFactory(ctx, write, "reet_list_factory", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"data": out})
	t.Logf("list 9 outcomes (3 pipelines each; 3 earlier of 1,3,5): %d bytes", len(raw))
}
