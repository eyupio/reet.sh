package gaffer

import (
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

// A stage sent back moves up one tier, once, when its first choice is still
// its tier's crew member, and never above heavy.
func TestRaiseTierMovesACrewStageUpOnce(t *testing.T) {
	crew := store.GafferCrew{
		Standard: store.GafferCrewTier{Tool: "opencode", Model: "mid/model"},
		Heavy:    store.GafferCrewTier{Tool: "claude", Model: "opus", Effort: "high", Backup: &store.GafferStageAgent{Tool: "codex"}},
	}
	stage := store.GafferOutcomeStage{Kind: "code", Tier: "standard", Tool: "opencode", Model: "mid/model"}
	raiseTier(&stage, crew)
	if stage.Tier != "heavy" || stage.TierFrom != "standard" || stage.Tool != "claude" || stage.Model != "opus" || stage.Effort != "high" || stage.Backup == nil || stage.TierWhy == "" {
		t.Fatalf("not moved up: %+v", stage)
	}
	again := stage
	raiseTier(&again, crew)
	if again.Tier != "heavy" || again.TierFrom != "standard" {
		t.Fatalf("moved twice: %+v", again)
	}
	for name, s := range map[string]store.GafferOutcomeStage{
		"picked by hand":    {Kind: "code", Tier: "standard", Tool: "codex", Model: "own"},
		"already heavy":     {Kind: "review", Tier: "heavy", Tool: "claude", Model: "opus", Effort: "high"},
		"no tier":           {Kind: "code", Tool: "opencode", Model: "mid/model"},
		"own tier unset":    {Kind: "pr", Tier: "light"},
		"moved once before": {Kind: "code", Tier: "standard", TierFrom: "light", Tool: "opencode", Model: "mid/model"},
	} {
		before := s
		raiseTier(&s, crew)
		if s.Tier != before.Tier || s.Tool != before.Tool || s.TierFrom != before.TierFrom {
			t.Errorf("%s: moved %+v", name, s)
		}
	}
	// A stage on its backup is judged by its first choice.
	onBackup := store.GafferOutcomeStage{Kind: "code", Tier: "standard", Tool: "codex", First: &store.GafferStageAgent{Tool: "opencode", Model: "mid/model"}, Balanced: "plan low"}
	raiseTier(&onBackup, crew)
	if onBackup.Tier != "heavy" || onBackup.Tool != "claude" || onBackup.First != nil || onBackup.Balanced != "" {
		t.Fatalf("a stage on its backup was not moved from its first choice: %+v", onBackup)
	}
}

// Sending a pipeline back moves the stage it goes back to up a tier when the
// crew chose its agent; the stages after it keep theirs.
func TestSendingBackMovesTheStageUpATier(t *testing.T) {
	r := outcomeFixture(t)
	if _, err := r.st.SetGafferCrewFor(r.p.UserID, store.GafferCrew{
		Standard: store.GafferCrewTier{Tool: "opencode", Model: "mid/model"},
		Heavy:    store.GafferCrewTier{Tool: "opencode", Model: "strong/model"},
	}); err != nil {
		t.Fatal(err)
	}
	in := r.in
	in.Sessions = []OutcomeSession{{Title: "Export API", Prompt: "Add the CSV export endpoint with tests.", Stages: []OutcomeStageInput{
		{Kind: "code"},
		{Kind: "test"},
	}}}
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if _, err = r.s.SendPipelineBack(r.p.UserID, created.ID, "a", "code", "The endpoint misses the header row."); err != nil {
		t.Fatal(err)
	}
	again := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 0 })
	code, test := again.Stages[0], again.Stages[1]
	if code.Tier != "heavy" || code.TierFrom != "standard" || code.Model != "strong/model" {
		t.Fatalf("the stage sent back was not moved up: %+v", code)
	}
	if test.Tier != "standard" || test.Model != "mid/model" {
		t.Fatalf("a later stage moved too: %+v", test)
	}
	if asked := r.starter.asked(); asked[len(asked)-1].Model != "strong/model" {
		t.Fatalf("the stage done again did not start on the heavier model: %+v", asked[len(asked)-1])
	}
}
