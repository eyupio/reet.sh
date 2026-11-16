package gaffer

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

func choiceInput(r *outcomeRig) OutcomeInput {
	in := pipelineInput(r, false)
	in.Sessions[0].Stages = []OutcomeStageInput{
		{Kind: "code", Tool: "opencode", Model: "cheap/coder", Effort: "low"},
		{Kind: "review", Tool: "opencode", Model: "strong/reviewer", Effort: "high"},
		{Kind: "test", Tool: "opencode", Model: "strong/reviewer", Effort: "high"},
	}
	return in
}

// checked waits for the stage in hand to carry the result of its check.
func (r *outcomeRig) checked(t *testing.T, id string, stage int) store.GafferOutcomeStage {
	t.Helper()
	piece := r.piece(t, id, func(p store.GafferOutcomeSession) bool {
		return p.State == "following" && p.Stage == stage && p.Stages[stage].ChoiceCheck != ""
	})
	return piece.Stages[stage]
}

// The agent, model and effort chosen for a stage are a request. Once the
// stage's session is up the controller reads back from the machine what the
// session is actually on and keeps it with the stage, so the page can say
// whether the choice was applied instead of repeating the choice.
func TestAStageRecordsThatItsAgentModelAndEffortWereApplied(t *testing.T) {
	r := outcomeFixture(t)
	log := recordAudit(r.s)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, choiceInput(r))
	if err != nil {
		t.Fatal(err)
	}
	code := r.checked(t, created.ID, 0)
	if code.ChoiceCheck != "applied" || code.AppliedTool != "opencode" || code.AppliedModel != "cheap/coder" || code.AppliedEffort != "low" || code.ChoiceNote != "" || code.AppliedAt == 0 {
		t.Fatalf("code stage: %+v", code)
	}
	// A stage with a session of its own is checked against its own choice.
	piece := mustOutcome(t, r, created.ID).Sessions[0]
	r.s.stageHandedOver(r.p.UserID, piece.ConversationID, "Done.", "gaffer")
	review := r.checked(t, created.ID, 1)
	if review.ChoiceCheck != "applied" || review.AppliedModel != "strong/reviewer" || review.AppliedEffort != "high" {
		t.Fatalf("review stage: %+v", review)
	}
	// A stage on the same agent and model as the one before has its own session, and is checked like any other.
	piece = mustOutcome(t, r, created.ID).Sessions[0]
	launches := len(r.starter.asked())
	r.s.stageHandedOver(r.p.UserID, piece.ConversationID, "Done.", "gaffer")
	test := r.checked(t, created.ID, 2)
	if test.ChoiceCheck != "applied" || test.AppliedModel != "strong/reviewer" || len(r.starter.asked()) != launches+1 || test.SessionID == piece.SessionID {
		t.Fatalf("test stage: %+v launches %d", test, len(r.starter.asked()))
	}
	for _, row := range log.all() {
		if row.action == "gaffer-stage-choice-not-applied" {
			t.Fatalf("an applied choice was logged as not applied: %+v", row)
		}
	}
}

// A session that is on another model or effort than the stage chose is said
// to be, with both named, on the stage and in the activity log. The pipeline
// is not stopped by it: the person decides.
func TestAStageSaysWhenItsChoiceWasNotApplied(t *testing.T) {
	r := outcomeFixture(t)
	log := recordAudit(r.s)
	r.starter.choice = func(_ string, asked OutcomeLaunch) (SessionChoice, error) {
		return SessionChoice{Tool: asked.Tool, Model: "other/model", Effort: "medium"}, nil
	}
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, choiceInput(r))
	if err != nil {
		t.Fatal(err)
	}
	code := r.checked(t, created.ID, 0)
	if code.ChoiceCheck != "mismatch" || code.AppliedModel != "other/model" || code.AppliedEffort != "medium" {
		t.Fatalf("code stage: %+v", code)
	}
	for _, want := range []string{"cheap/coder", "other/model", "low", "medium"} {
		if !strings.Contains(code.ChoiceNote, want) {
			t.Fatalf("the note should name %q: %q", want, code.ChoiceNote)
		}
	}
	if piece := mustOutcome(t, r, created.ID).Sessions[0]; piece.State != "following" {
		t.Fatalf("a mismatch stopped the pipeline: %+v", piece)
	}
	log.wait(t, "gaffer-stage-started code", "gaffer-stage-choice-not-applied code")
	row := log.all()[1]
	if row.detail["outcome"] != created.ID || row.detail["stage"] != "code" || !strings.Contains(fmt.Sprint(row.detail["reason"]), "other/model") {
		t.Fatalf("audit: %+v", row)
	}
}

// Where the choice was left to the agent, there is nothing to compare: what
// the session turned out to be on is recorded. And a machine that cannot be
// asked leaves the stage marked as not checked, never as applied.
func TestAStageRecordsTheAgentsDefaultAndSaysWhenItCouldNotCheck(t *testing.T) {
	r := outcomeFixture(t)
	r.starter.choice = func(string, OutcomeLaunch) (SessionChoice, error) {
		return SessionChoice{Tool: "opencode", Model: "default/model", Effort: "medium"}, nil
	}
	r.s.StartGoalSupervisor()
	in := pipelineInput(r, false)
	in.Sessions[0].Stages = []OutcomeStageInput{{Kind: "code", Tool: "opencode"}}
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	code := r.checked(t, created.ID, 0)
	if code.ChoiceCheck != "applied" || code.AppliedModel != "default/model" || code.AppliedEffort != "medium" {
		t.Fatalf("agent default: %+v", code)
	}

	r2 := outcomeFixture(t)
	r2.starter.choice = func(string, OutcomeLaunch) (SessionChoice, error) {
		return SessionChoice{}, errors.New("the machine is offline")
	}
	r2.s.StartGoalSupervisor()
	created, err = r2.s.CreateOutcome(r2.p.UserID, choiceInput(r2))
	if err != nil {
		t.Fatal(err)
	}
	code = r2.checked(t, created.ID, 0)
	if code.ChoiceCheck != "unchecked" || code.AppliedModel != "" || !strings.Contains(code.ChoiceNote, "the machine is offline") {
		t.Fatalf("unchecked: %+v", code)
	}
}

// A model or effort changed in the session's own chat after the stage began
// shows on the stage the next time the page reads it.
func TestAStageShowsALaterChangeOfModelOrEffort(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, choiceInput(r))
	if err != nil {
		t.Fatal(err)
	}
	r.checked(t, created.ID, 0)
	piece := mustOutcome(t, r, created.ID).Sessions[0]
	if err = r.st.RecordTerminalControlsFor(r.p.UserID, piece.SessionID, "cheap/coder", "xhigh"); err != nil {
		t.Fatal(err)
	}
	o, err := r.s.Outcome(r.p.UserID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	live := o.Sessions[0].Stages[0]
	if live.ChoiceCheck != "mismatch" || live.AppliedEffort != "xhigh" || !strings.Contains(live.ChoiceNote, "xhigh") || strings.Contains(live.ChoiceNote, "model") {
		t.Fatalf("live stage: %+v", live)
	}
}

func TestStageChoiceCheckComparesOnlyWhatWasChosen(t *testing.T) {
	stage := store.GafferOutcomeStage{Tool: "codex", Model: "gpt-x", Effort: "high"}
	for name, c := range map[string]struct {
		got   SessionChoice
		check string
	}{
		"all the same":       {SessionChoice{Tool: "codex", Model: "gpt-x", Effort: "high"}, "applied"},
		"another agent":      {SessionChoice{Tool: "claude", Model: "gpt-x", Effort: "high"}, "mismatch"},
		"model not reported": {SessionChoice{Tool: "codex", Effort: "high"}, "mismatch"},
		"another effort":     {SessionChoice{Tool: "codex", Model: "gpt-x", Effort: "low"}, "mismatch"},
	} {
		if check, note := stageChoiceCheck(stage, c.got); check != c.check || (check == "applied") != (note == "") {
			t.Errorf("%s: %s %q", name, check, note)
		}
	}
	if check, _ := stageChoiceCheck(store.GafferOutcomeStage{Tool: "codex"}, SessionChoice{Tool: "codex", Model: "anything", Effort: "low"}); check != "applied" {
		t.Errorf("an agent default is never a mismatch: %s", check)
	}
}

// A pipeline with no stages runs on the outcome's own agent and model, and
// its session is read back and compared with those like a stage's.
func TestAPipelineWithNoStagesSaysWhetherTheOutcomesChoiceWasApplied(t *testing.T) {
	r := outcomeFixture(t)
	r.in.Model = "provider/model-a"
	r.in.Sessions = r.in.Sessions[:1]
	r.starter.mu.Lock()
	r.starter.choice = func(string, OutcomeLaunch) (SessionChoice, error) {
		return SessionChoice{Tool: "opencode", Model: "provider/model-b"}, nil
	}
	r.starter.mu.Unlock()
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, r.in)
	if err != nil {
		t.Fatal(err)
	}
	piece := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Applied != nil })
	if got := piece.Applied; got.Check != "mismatch" || got.Model != "provider/model-b" || got.ChosenModel != "provider/model-a" || !strings.Contains(got.Note, "the session is on provider/model-b") || len(piece.Stages) != 0 {
		t.Fatalf("applied: %+v", got)
	}
}
