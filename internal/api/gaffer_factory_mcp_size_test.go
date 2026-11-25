package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/store"
)

// factoryBulkOutcome saves an outcome of n pipelines of six stages, as a real
// run leaves one: every stage with its agent, tier, backup, read-back choice,
// timings and token figures, and a handover of the largest size a stage keeps.
// Each pipeline is in the given state at its merge stage.
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

// factoryReply formats data as the server does for a client, which refuses a
// reply over the response limit, and returns its length in characters.
func factoryReply(t *testing.T, data map[string]any, wrote bool) int {
	t.Helper()
	raw, err := mcpResultJSON(data, wrote)
	if err != nil {
		t.Fatalf("the reply was refused: %v", err)
	}
	return utf8.RuneCount(raw)
}

// The Factory's list is a short row for each outcome, paged like the other
// lists, so six retained outcomes of three six-stage pipelines fit in a reply
// however long their stages are. Archived outcomes are left out unless asked for.
func TestFactoryMCPListIsShortRowsThatPageAndLeaveOutArchived(t *testing.T) {
	t.Parallel()
	s, user, write := factoryMCPFixture(t)
	ctx := context.Background()
	for _, id := range []string{"1", "2", "3", "4", "5", "6"} {
		factoryBulkOutcome(t, s, user.ID, id, 3, "done")
	}
	out, err := s.executeMCPFactory(ctx, write, "reet_list_factory", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	size := factoryReply(t, out, false)
	items := out["items"].([]any)
	if len(items) != 6 || out["total_count"] != 6 || out["has_more"] != false {
		t.Fatalf("list: %d items, %v of %v", len(items), out["total_count"], out["has_more"])
	}
	row := items[0].(map[string]any)
	pipelines := row["pipelines"].([]map[string]any)
	if len(pipelines) != 3 || pipelines[0]["stages"] != nil || pipelines[0]["stage_in_hand"] != "merge" || pipelines[0]["updated_at"] != int64(5000) || utf8.RuneCountInString(row["objective"].(string)) > 210 {
		t.Fatalf("row: %v", row)
	}
	if out["controller"] == nil || out["machines"] == nil {
		t.Fatalf("a page lacks the builds: %v", out)
	}
	t.Logf("a page of six outcomes of three pipelines is %d characters", size)

	// It pages with limit and offset, as the other lists do.
	first, err := s.executeMCPFactory(ctx, write, "reet_list_factory", map[string]any{"limit": float64(4)})
	if err != nil || len(first["items"].([]any)) != 4 || first["has_more"] != true || first["next_offset"] != 4 {
		t.Fatalf("first page: %v %v", first, err)
	}
	next, err := s.executeMCPFactory(ctx, write, "reet_list_factory", map[string]any{"limit": float64(4), "offset": float64(4)})
	if err != nil || len(next["items"].([]any)) != 2 || next["has_more"] != false {
		t.Fatalf("second page: %v %v", next, err)
	}

	// An archived outcome is not listed unless include_archived says so.
	rows, err := s.st.GafferOutcomesFor(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.st.UpdateGafferOutcomeFor(user.ID, rows[0].ID, rows[0].Revision, func(o *store.GafferOutcome) bool { o.ArchivedAt = 9000; return true }); err != nil {
		t.Fatal(err)
	}
	if out, err = s.executeMCPFactory(ctx, write, "reet_list_factory", map[string]any{}); err != nil || out["total_count"] != 5 {
		t.Fatalf("list with one archived: %v %v", out["total_count"], err)
	}
	if out, err = s.executeMCPFactory(ctx, write, "reet_list_factory", map[string]any{"include_archived": true}); err != nil || out["total_count"] != 6 {
		t.Fatalf("list with archived asked for: %v %v", out["total_count"], err)
	}
}

// A control answers with the outcome in short, so an approval that is applied
// is not reported as too large to read, however long the outcome's pipelines are.
func TestFactoryMCPControlsAnswerWithTheOutcomeInShort(t *testing.T) {
	t.Parallel()
	s, user, write := factoryMCPFixture(t)
	o := factoryBulkOutcome(t, s, user.ID, "7", 5, "ready_to_merge")
	out, err := s.executeMCPFactory(context.Background(), write, "reet_approve_pipeline_merge", map[string]any{"outcome_id": o.ID, "pipeline": "a", "confirm": true, "seen_updated_at": float64(5000)})
	if err != nil || out["applied"] != true || !strings.Contains(out["note"].(string), "reet_get_factory_outcome") {
		t.Fatalf("approval: %v %v", out, err)
	}
	if !factoryMergeApproved(t, s, user.ID, o.ID) {
		t.Fatal("the approval was not applied")
	}
	t.Logf("the answer to an approval on five pipelines is %d characters", factoryReply(t, out, true))
	if pipelines := out["outcome"].(map[string]any)["pipelines"].([]map[string]any); len(pipelines) != 5 || pipelines[0]["stages"] != nil {
		t.Fatalf("the answer is not in short: %v", pipelines)
	}
}

// A handover is clipped, so an outcome of a few pipelines is read whole, and
// one too long for that can be read a pipeline at a time.
func TestFactoryMCPReadClipsHandoversAndReadsOnePipeline(t *testing.T) {
	t.Parallel()
	s, user, write := factoryMCPFixture(t)
	ctx := context.Background()
	three := factoryBulkOutcome(t, s, user.ID, "8", 3, "done")
	out, err := s.executeMCPFactory(ctx, write, "reet_get_factory_outcome", map[string]any{"outcome_id": three.ID})
	if err != nil {
		t.Fatal(err)
	}
	stages := out["outcome"].(map[string]any)["pipelines"].([]map[string]any)[0]["stages"].([]map[string]any)
	if got := stages[0]["handover"].(string); len(got) > mcpHandoverBytes+3 || !strings.HasSuffix(got, "...") {
		t.Fatalf("handover of %d bytes", len(got))
	}
	t.Logf("a read of three pipelines is %d characters", factoryReply(t, out, false))

	five := factoryBulkOutcome(t, s, user.ID, "9", 5, "done")
	raw, _ := json.Marshal(map[string]any{"data": mustFactoryRead(t, s, write, five.ID, "")})
	t.Logf("a read of five pipelines whole would be %d characters", utf8.RuneCount(raw))
	if utf8.RuneCount(raw) <= mcpResponseLimit {
		t.Fatal("five pipelines were read whole: the test no longer shows why a pipeline can be asked for")
	}
	one := mustFactoryRead(t, s, write, five.ID, "c")
	if pipelines := one["outcome"].(map[string]any)["pipelines"].([]map[string]any); len(pipelines) != 1 || pipelines[0]["key"] != "c" {
		t.Fatalf("one pipeline: %v", pipelines)
	}
	t.Logf("a read of one pipeline of five is %d characters", factoryReply(t, one, false))
	if _, err = s.executeMCPFactory(ctx, write, "reet_get_factory_outcome", map[string]any{"outcome_id": five.ID, "pipeline": "z"}); err == nil || !strings.Contains(err.Error(), "no pipeline") {
		t.Fatalf("a pipeline it does not have: %v", err)
	}
}

func mustFactoryRead(t *testing.T, s *server, access mcpAccess, id, pipeline string) map[string]any {
	t.Helper()
	args := map[string]any{"outcome_id": id}
	if pipeline != "" {
		args["pipeline"] = pipeline
	}
	out, err := s.executeMCPFactory(context.Background(), access, "reet_get_factory_outcome", args)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Moving a pipeline on names the stage the caller read as in hand: without it
// a pipeline that has moved on since would be moved again.
func TestFactoryMCPMovePipelineOnNeedsTheStage(t *testing.T) {
	t.Parallel()
	s, user, write := factoryMCPFixture(t)
	o := factoryRecord(t, s, user.ID, "a", "following", nil)
	_, err := s.executeMCPFactory(context.Background(), write, "reet_move_pipeline_on", map[string]any{"outcome_id": o.ID, "pipeline": "a", "confirm": true})
	if err == nil || !strings.Contains(err.Error(), "Pass stage") || !strings.Contains(err.Error(), "Nothing was changed") {
		t.Fatalf("move on without a stage: %v", err)
	}
	// With the stage it goes on to the controller's own check.
	if _, err = s.executeMCPFactory(context.Background(), write, "reet_move_pipeline_on", map[string]any{"outcome_id": o.ID, "pipeline": "a", "confirm": true, "stage": "code"}); err == nil || strings.Contains(err.Error(), "Pass stage") {
		t.Fatalf("move on with a stage: %v", err)
	}
	for _, tool := range mcpToolDefinitions {
		if tool.Name != "reet_move_pipeline_on" {
			continue
		}
		raw, _ := json.Marshal(tool.InputSchema)
		var schema struct {
			Required []string `json:"required"`
		}
		if err = json.Unmarshal(raw, &schema); err != nil || !strings.Contains(strings.Join(schema.Required, ","), "stage") {
			t.Fatalf("the tool does not require stage: %v %v", schema.Required, err)
		}
	}
}
