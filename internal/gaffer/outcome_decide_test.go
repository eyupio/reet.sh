package gaffer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/elicore"
	"github.com/eyupio/reet.sh/internal/store"
)

// decideWith makes the rig's provider answer every request with one tool call.
func decideWith(r *outcomeRig, calls *atomic.Int32, name, args string) {
	r.reply = func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		call, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "d1", "type": "function", "function": map[string]any{"name": name, "arguments": args}}}}, "finish_reason": "tool_calls"}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", call)
	}
}

// stopStage leaves the stage in hand needing a decision, as a review that
// asked the person something does.
func stopStage(t *testing.T, r *outcomeRig, piece store.GafferOutcomeSession) store.GafferGoal {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		g, err := r.st.GafferGoalFor(r.p.UserID, piece.ConversationID)
		if err == nil && g.ActiveTurnID == "" {
			if err = r.st.ObserveGafferGoal(g, "needs_user", "Gaffer needs your decision. Read its latest review.", 0); err == nil {
				g, _ = r.st.GafferGoalFor(r.p.UserID, piece.ConversationID)
				return g
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("the stage's goal could not be stopped: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func decisions(t *testing.T, r *outcomeRig, id string, n int) []store.GafferOutcomeDecision {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		o := mustOutcome(t, r, id)
		if len(o.Decisions) >= n {
			return o.Decisions
		}
		if time.Now().After(deadline) {
			t.Fatalf("decisions: %+v", o.Decisions)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// An outcome whose decisions the person kept is never decided for them, and
// costs no model request. One they gave to Gaffer is: Gaffer picks one of the
// stage's own choices, the controller carries it out, and the outcome keeps
// what was decided and why.
func TestGafferTakesAStoppedStagesDecisionOnlyWhenItWasGivenThem(t *testing.T) {
	r := outcomeFixture(t)
	log := recordAudit(r.s)
	var calls atomic.Int32
	decideWith(r, &calls, "decide", `{"choice":"move","why":"The endpoint and its tests are in the handover; nothing more belongs in this stage."}`)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	stopStage(t, r, code)
	r.s.autoDecisions(context.Background())
	time.Sleep(80 * time.Millisecond)
	if o := mustOutcome(t, r, created.ID); calls.Load() != 0 || len(o.Decisions) != 0 || o.Sessions[0].Stage != 0 {
		t.Fatalf("decided for a person who kept the decisions: calls %d, %+v", calls.Load(), o.Decisions)
	}

	// The person gives Gaffer the decisions on the card.
	if _, err = r.s.SetOutcomeAutomation(r.p.UserID, created.ID, mustOutcome(t, r, created.ID).Revision, true); err != nil {
		t.Fatal(err)
	}
	got := decisions(t, r, created.ID, 1)[0]
	if got.Choice != "move" || got.Failed || got.Key != code.Key || got.Stage != "code" || got.About != "question" || !strings.Contains(got.Why, "handover") || got.Model != "fake-model" {
		t.Fatalf("decision: %+v", got)
	}
	review := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.Stage == 1 && p.State == "following" })
	if done := review.Stages[0]; done.State != "done" || done.ClosedBy != "auto" || !strings.Contains(done.Summary, "Gaffer moved this pipeline on") || done.Decided != 1 {
		t.Fatalf("the stage Gaffer moved on from: %+v", done)
	}
	if calls.Load() != 1 {
		t.Fatalf("model requests: %d", calls.Load())
	}
	found := false
	for _, row := range log.all() {
		found = found || row.action == "gaffer-decision" && row.detail["choice"] == "move" && row.detail["pipeline"] == code.Key
	}
	if !found {
		t.Fatalf("the decision was not recorded in the activity log: %+v", log.all())
	}
}

// One stop is put to Gaffer once, a stage gets three decisions at most, and a
// choice that is not one of the stage's own changes nothing and is recorded
// as a decision Gaffer could not take.
func TestGafferDecidesEachStopOnceAndOnlyAmongTheStagesChoices(t *testing.T) {
	r := outcomeFixture(t)
	var calls atomic.Int32
	// Merging is never Gaffer's to choose.
	decideWith(r, &calls, "decide", `{"choice":"merge","why":"It looks finished."}`)
	in := pipelineInput(r, false)
	in.Automation = true
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	stopStage(t, r, code)
	r.s.autoDecisions(context.Background())
	got := decisions(t, r, created.ID, 1)[0]
	if !got.Failed || got.Choice != "" || !strings.Contains(got.Why, "it is yours") {
		t.Fatalf("a choice outside the stage's own: %+v", got)
	}
	for i := 0; i < 3; i++ {
		r.s.autoDecisions(context.Background())
	}
	time.Sleep(80 * time.Millisecond)
	o := mustOutcome(t, r, created.ID)
	if calls.Load() != 1 || len(o.Decisions) != 1 || o.Sessions[0].Stage != 0 || o.Sessions[0].State != "following" {
		t.Fatalf("the same stop was decided again or the pipeline moved: calls %d, %+v", calls.Load(), o.Sessions[0])
	}

	// A stage that has used its three decisions is the person's.
	if _, err = r.s.outcomeStep(r.p.UserID, created.ID, func(o *store.GafferOutcome) bool {
		o.Sessions[0].Stages[0].Decided, o.Sessions[0].Stages[0].DecidedFor = maxStageDecisions, ""
		return true
	}); err != nil {
		t.Fatal(err)
	}
	r.s.autoDecisions(context.Background())
	time.Sleep(80 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatalf("a stage past its limit was decided: %d", calls.Load())
	}
}

// What a stage's stop offers Gaffer: never a merge, never an agent's own
// question, and a new budget only once.
func TestWhatGafferMayDecideForAStoppedStage(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	piece := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	o := mustOutcome(t, r, created.ID)
	if _, ok := r.s.pipelineStop(o, piece); ok {
		t.Fatal("a stage that is under way has nothing to decide")
	}
	g, err := r.st.GafferGoalFor(r.p.UserID, piece.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.st.HoldGafferGoalForWorker(g, workerWaitingNote, 0); err != nil {
		t.Fatal(err)
	}
	if held, _ := r.st.GafferGoalFor(r.p.UserID, piece.ConversationID); held.State != "needs_user" || !held.WaitingOnWorker {
		t.Fatalf("held goal: %+v", held)
	}
	if _, ok := r.s.pipelineStop(o, piece); ok {
		t.Fatal("an agent waiting for the person's answer or approval is not Gaffer's to decide")
	}
	piece.State = "ready_to_merge"
	if _, ok := r.s.pipelineStop(o, piece); ok {
		t.Fatal("a merge is not Gaffer's to decide")
	}
}

func TestADecisionIsOneListedChoiceWithAReason(t *testing.T) {
	choices := []string{"again", "tell", "move"}
	for raw, want := range map[string]string{
		`{"choice":"move","why":"Done."}`:                                             "move",
		`{"choice":"tell","why":"Tests are missing.","instruction":"Add the tests."}`: "tell",
		`{"choice":"tell","why":"Tests are missing."}`:                                "",
		`{"choice":"restart","why":"x"}`:                                              "",
		`{"choice":"move"}`:                                                           "",
		`{"choice":"move","why":"x","agent":"claude"}`:                                "",
	} {
		choice, _, _, err := decision(json.RawMessage(raw), choices)
		if choice != want || (err == nil) != (want != "") {
			t.Fatalf("%s: %q %v", raw, choice, err)
		}
	}
}

// Gaffer's recommendation travels with a stage's question; one that is not a
// choice the person has, or has no reason, is dropped and the question kept.
func TestAStageQuestionCarriesGaffersRecommendation(t *testing.T) {
	call := func(args string) elicore.ToolCall {
		return elicore.ToolCall{ID: "q1", Name: "ask_user", Arguments: json.RawMessage(args)}
	}
	summary, advice, err := goalQuestion(call(`{"summary":"The tests are missing.","recommend":"tell","why":"The endpoint is there but untested.","instruction":"Add tests for the export endpoint."}`))
	if err != nil || summary != "The tests are missing." || advice == nil || advice.Choice != "tell" || advice.Text != "Add tests for the export endpoint." {
		t.Fatalf("%q %+v %v", summary, advice, err)
	}
	for _, args := range []string{`{"summary":"x","recommend":"merge","why":"y"}`, `{"summary":"x","recommend":"move"}`, `{"summary":"x"}`} {
		if summary, advice, err = goalQuestion(call(args)); err != nil || summary != "x" || advice != nil {
			t.Fatalf("%s: %q %+v %v", args, summary, advice, err)
		}
	}
}
