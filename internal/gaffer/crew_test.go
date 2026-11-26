package gaffer

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

// Gaffer advises: its plan rates how demanding each stage is and says why.
// The rating is a tier name and a reason, never an agent or a model.
func TestPlanOutcomeRatesEachStageAndSaysWhy(t *testing.T) {
	s, _, p := settingsFixture(t)
	args := `{"summary":"One piece.","sessions":[{"title":"Export API","prompt":"Add the CSV export endpoint with tests. Raise a pull request. Do not merge.","stages":["code","review","pr"],"ratings":[{"stage":"code","tier":"heavy","why":"The export touches the billing ledger's locking."},{"stage":"review","tier":"standard"}]}]}`
	server, sent := planServer(t, func() string { return planCall("propose_sessions", args) })
	row := planProvider(t, s, p, server.URL, true)
	plan, err := s.PlanOutcome(context.Background(), p, row.ID, OutcomePlanInput{Revision: row.Revision, Objective: "Ship CSV export.", Project: "billing", Sessions: 3})
	if err != nil {
		t.Fatal(err)
	}
	stages := plan.Sessions[0].Stages
	if len(stages) != 3 || stages[0].Tier != "heavy" || stages[0].TierWhy != "The export touches the billing ledger's locking." || stages[1].Tier != "standard" || stages[1].TierWhy != "" {
		t.Fatalf("rated stages: %+v", stages)
	}
	// A stage Gaffer did not rate has no tier of Gaffer's: it takes its kind's default.
	if stages[2].Tier != "" || stages[0].Tool != "" || stages[0].Model != "" {
		t.Fatalf("an unrated stage, or an agent chosen by the plan: %+v", stages)
	}
	for _, want := range []string{"light", "standard", "heavy", "ratings"} {
		if !strings.Contains(sent()[0], want) {
			t.Fatalf("the provider was not asked to rate stages (%q): %s", want, sent()[0])
		}
	}
	// A rating for a stage the pipeline does not have, a tier that is not one, or the same stage twice is not a plan.
	for name, bad := range map[string]string{
		"a stage it lacks": `"ratings":[{"stage":"test","tier":"heavy"}]`,
		"an unknown tier":  `"ratings":[{"stage":"code","tier":"enormous"}]`,
		"a stage twice":    `"ratings":[{"stage":"code","tier":"heavy"},{"stage":"code","tier":"light"}]`,
		"a model":          `"ratings":[{"stage":"code","tier":"heavy","model":"opus"}]`,
	} {
		raw := `{"summary":"x","sessions":[{"title":"A","prompt":"Do it. Do not merge.","stages":["code"],` + bad + `}]}`
		if _, _, _, err := outcomeSessions([]byte(raw), 3, nil); err == nil {
			t.Errorf("a plan rating %s was accepted", name)
		}
	}
}

func TestEachStageKindHasADefaultTier(t *testing.T) {
	want := map[string]string{"plan": "heavy", "design": "heavy", "review": "heavy", "code": "standard", "test": "standard", "pr": "light", "babysit": "light", "merge": "light"}
	for _, kind := range StageKinds {
		if got := DefaultStageTier(kind); got != want[kind] {
			t.Errorf("%s: %s, want %s", kind, got, want[kind])
		}
	}
}

// The tiers a person saved are theirs: validated, kept per account, and
// another account sees none of them.
func TestTheCrewIsSavedPerAccountAndChecked(t *testing.T) {
	s, st, p := settingsFixture(t)
	if crew, err := s.Crew(p.UserID); err != nil || crew.Saved() {
		t.Fatalf("before any save: %+v %v", crew, err)
	}
	crew := store.GafferCrew{
		Light:    store.GafferCrewTier{Tool: "codex", Model: "small", Effort: "low"},
		Standard: store.GafferCrewTier{Tool: "opencode", Model: "mid/model"},
		Heavy:    store.GafferCrewTier{Tool: "claude", Model: "opus", Effort: "high"},
	}
	saved, err := s.SaveCrew(p.UserID, crew)
	if err != nil || !saved.Saved() || !reflect.DeepEqual(saved.Heavy, crew.Heavy) || saved.UpdatedAt == 0 {
		t.Fatalf("saved: %+v %v", saved, err)
	}
	if again, err := s.Crew(p.UserID); err != nil || !reflect.DeepEqual(again.Light, crew.Light) || !reflect.DeepEqual(again.Standard, crew.Standard) {
		t.Fatalf("read back: %+v %v", again, err)
	}
	other, err := st.CreateUser("crew-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if theirs, err := s.Crew(other.ID); err != nil || theirs.Saved() {
		t.Fatalf("another account's crew: %+v %v", theirs, err)
	}
	for name, bad := range map[string]store.GafferCrewTier{
		"an agent that cannot chat": {Tool: "bash"},
		"a model with no agent":     {Model: "opus"},
		"a control character":       {Tool: "claude", Model: "op\nus"},
		"a source for another":      {Tool: "claude", Provider: "cline-pass"},
	} {
		wrong := crew
		wrong.Standard = bad
		if _, err := s.SaveCrew(p.UserID, wrong); err == nil {
			t.Errorf("a tier with %s was saved", name)
		}
	}
	// A tier left empty is allowed: it falls back to the outcome's own agent.
	if _, err := s.SaveCrew(p.UserID, store.GafferCrew{Heavy: crew.Heavy}); err != nil {
		t.Fatalf("a crew with one tier set: %v", err)
	}
}

// A stage that names no agent is done by its tier's: the tier Gaffer or the
// person gave it, else its kind's default. This is what an outcome started
// over MCP, which names only stage kinds, runs on. A stage that names its own
// agent keeps it, and a tier nobody set falls back to the outcome's agent.
func TestAStageWithNoAgentTakesItsTiersFromTheSavedCrew(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	if _, err := r.s.SaveCrew(r.p.UserID, store.GafferCrew{
		Standard: store.GafferCrewTier{Tool: "opencode", Model: "mid/model", Effort: "medium"},
		Heavy:    store.GafferCrewTier{Tool: "opencode", Model: "big/model", Effort: "high"},
	}); err != nil {
		t.Fatal(err)
	}
	in := pipelineInput(r, false)
	in.Model = "outcome/model"
	in.Sessions[0].Stages = []OutcomeStageInput{
		{Kind: "code", Tier: "heavy", TierWhy: "Touches the ledger."},
		{Kind: "review"},
		{Kind: "test", Tool: "opencode", Model: "own/model"},
		{Kind: "pr"},
	}
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	got := created.Sessions[0].Stages
	type want struct{ tier, why, model, effort string }
	for i, w := range []want{
		{"heavy", "Touches the ledger.", "big/model", "high"}, // Gaffer's rating, above code's default
		{"heavy", "", "big/model", "high"},                    // review's default tier
		{"standard", "", "own/model", ""},                     // names its own agent and model: kept
		{"light", "", "outcome/model", ""},                    // the light tier is not set: the outcome's agent
	} {
		if got[i].Tier != w.tier || got[i].TierWhy != w.why || got[i].Model != w.model || got[i].Effort != w.effort || got[i].Tool != "opencode" {
			t.Errorf("stage %s: %+v, want %+v", got[i].Kind, got[i], w)
		}
	}
	// A tier that is not one, or a reason with a control character, is refused.
	for name, stage := range map[string]OutcomeStageInput{"tier": {Kind: "code", Tier: "enormous"}, "reason": {Kind: "code", Tier: "heavy", TierWhy: "a\x00b"}} {
		bad := pipelineInput(r, false)
		bad.Sessions[0].Title, bad.Sessions[0].Stages = "Another "+name, []OutcomeStageInput{stage}
		if _, err := r.s.CreateOutcome(r.p.UserID, bad); err == nil {
			t.Errorf("a stage with a bad %s was accepted", name)
		}
	}
}

// Folder trust and YOLO are judged on the agents that will run. A stage that
// names no agent takes its tier's from the saved crew, so trust for that
// agent is accepted, and required where it must be, although the request
// itself names another.
func TestTrustAndYoloAreJudgedOnTheAgentsTheSavedCrewResolvesTo(t *testing.T) {
	r := outcomeFixture(t)
	if _, err := r.s.SaveCrew(r.p.UserID, store.GafferCrew{Standard: store.GafferCrewTier{Tool: "opencode", Model: "cheap/coder"}}); err != nil {
		t.Fatal(err)
	}
	in := r.in
	in.Tool, in.Trust = "claude", true
	in.Sessions = []OutcomeSession{{Title: "Export API", Prompt: "Add the CSV export endpoint.", Stages: []OutcomeStageInput{{Kind: "code"}}}}
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil || !created.Trust || created.Sessions[0].Stages[0].Tool != "opencode" {
		t.Fatalf("trust for the crew's OpenCode was refused: %+v %v", created, err)
	}
	// No stage runs on an agent that asks for trust: it is still refused.
	in.Sessions = []OutcomeSession{{Title: "Export docs", Prompt: "Write the docs.", Stages: []OutcomeStageInput{{Kind: "pr", Tool: "claude"}}}}
	if _, err = r.s.CreateOutcome(r.p.UserID, in); err == nil || !strings.Contains(err.Error(), "trust") {
		t.Fatalf("trust with no agent that asks for it: %v", err)
	}
	// Antigravity from the crew, or as a backup, needs the trust it always needs.
	if _, err = r.s.SaveCrew(r.p.UserID, store.GafferCrew{Standard: store.GafferCrewTier{Tool: "antigravity"}, Light: store.GafferCrewTier{Tool: "claude", Backup: &store.GafferStageAgent{Tool: "antigravity"}}}); err != nil {
		t.Fatal(err)
	}
	in.Trust = false
	for _, kind := range []string{"code", "pr"} {
		in.Sessions = []OutcomeSession{{Title: "Export " + kind, Prompt: "Do it.", Stages: []OutcomeStageInput{{Kind: kind}}}}
		if _, err = r.s.CreateOutcome(r.p.UserID, in); err == nil || !strings.Contains(err.Error(), "Antigravity") {
			t.Fatalf("%s: Antigravity from the crew without trust: %v", kind, err)
		}
	}
}
