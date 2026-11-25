package gaffer

import (
	"context"
	"encoding/json"
	"errors"
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

// setAutomation gives Gaffer the outcome's decisions, retrying while the
// supervisor records other progress. The person's card does the same after a
// reload when the outcome changed under it.
func setAutomation(t *testing.T, r *outcomeRig, id string, on bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		_, err := r.s.SetOutcomeAutomation(r.p.UserID, id, mustOutcome(t, r, id).Revision, on)
		if err == nil {
			return
		}
		if !errors.Is(err, store.ErrGafferOutcomeBusy) || time.Now().After(deadline) {
			t.Fatalf("set automation: %v", err)
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
	setAutomation(t, r, created.ID, true)
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

// A controller that stopped between claiming a stop and recording Gaffer's
// decision must not leave the pipeline waiting for ever: a claim older than
// the model's bound is put to Gaffer again, and asking again does not spend
// another of the stage's three decisions.
func TestAStopWhoseAnswerNeverCameIsAskedAgain(t *testing.T) {
	r := outcomeFixture(t)
	var calls atomic.Int32
	decideWith(r, &calls, "decide", `{"choice":"move","why":"The endpoint and its tests are in the handover."}`)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	stopStage(t, r, code)
	o := mustOutcome(t, r, created.ID)
	stop, ok := r.s.pipelineStop(o, o.Sessions[0])
	if !ok {
		t.Fatalf("the stopped stage has no decision to ask: %+v", o.Sessions[0])
	}
	// The claim a controller that died between the request and the decision leaves behind.
	if _, err = r.s.outcomeStep(r.p.UserID, created.ID, func(o *store.GafferOutcome) bool {
		stage := &o.Sessions[0].Stages[0]
		stage.Decided, stage.DecidedFor = 1, stop.sign
		stage.DecidingSince = time.Now().Add(-3 * decisionClaimTTL).UnixMilli()
		return true
	}); err != nil {
		t.Fatal(err)
	}
	setAutomation(t, r, created.ID, true)
	got := decisions(t, r, created.ID, 1)[0]
	if got.Choice != "move" || got.Failed || calls.Load() != 1 {
		t.Fatalf("the lost stop was not asked again: calls %d, %+v", calls.Load(), got)
	}
	if used := mustOutcome(t, r, created.ID).Sessions[0].Stages[0].Decided; used != 1 {
		t.Fatalf("asking a lost stop again spent another decision: %d", used)
	}
}

// A fresh claim is left alone (Gaffer is still thinking), and a claim a failed
// decision already answered is not asked again: both leave the stop with the
// person only when it is truly theirs.
func TestAStopIsNotAskedAgainWhileClaimedOrOnceAnswered(t *testing.T) {
	r := outcomeFixture(t)
	var calls atomic.Int32
	decideWith(r, &calls, "decide", `{"choice":"move","why":"It is finished."}`)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	stopStage(t, r, code)
	o := mustOutcome(t, r, created.ID)
	stop, ok := r.s.pipelineStop(o, o.Sessions[0])
	if !ok {
		t.Fatalf("the stopped stage has no decision to ask: %+v", o.Sessions[0])
	}
	key := o.Sessions[0].Key
	claim := func(since int64, failed bool) {
		t.Helper()
		if _, err := r.s.outcomeStep(r.p.UserID, created.ID, func(o *store.GafferOutcome) bool {
			stage := &o.Sessions[0].Stages[0]
			stage.Decided, stage.DecidedFor, stage.DecidingSince = 1, stop.sign, since
			if failed {
				o.Decide(store.GafferOutcomeDecision{At: since + 1, Key: key, Stage: stage.Kind, About: "question", Failed: true, Why: "Gaffer could not take this decision."})
			}
			return true
		}); err != nil {
			t.Fatal(err)
		}
	}
	claim(time.Now().UnixMilli(), false)
	setAutomation(t, r, created.ID, true)
	time.Sleep(120 * time.Millisecond)
	if calls.Load() != 0 || len(mustOutcome(t, r, created.ID).Decisions) != 0 {
		t.Fatalf("a stop still being decided was asked again: calls %d", calls.Load())
	}

	// The claim is old now, but a failed decision already answers it.
	claim(time.Now().Add(-3*decisionClaimTTL).UnixMilli(), true)
	r.s.autoDecisions(context.Background())
	time.Sleep(80 * time.Millisecond)
	if calls.Load() != 0 {
		t.Fatalf("a stop Gaffer already left to the person was asked again: %d", calls.Load())
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

// busyGate takes the account's two places for provider requests, as two slow
// requests would, until the returned function is called.
func (r *outcomeRig) busyGate() (release func()) {
	r.s.mu.Lock()
	r.s.active[1<<40], r.s.active[1<<41] = activeProbe{user: r.p.UserID, cancel: func() {}}, activeProbe{user: r.p.UserID, cancel: func() {}}
	r.s.mu.Unlock()
	return func() {
		r.s.mu.Lock()
		delete(r.s.active, 1<<40)
		delete(r.s.active, 1<<41)
		r.s.mu.Unlock()
	}
}

// A provider gate that is full refuses the request before it is made. That
// is not a decision: Gaffer does not record that it could not take the stop,
// the stage keeps its decision, and the stop is put again once the gate is free.
func TestABusyProviderLeavesAStopUndecidedAndItsDecisionUnspent(t *testing.T) {
	r := outcomeFixture(t)
	var calls atomic.Int32
	decideWith(r, &calls, "decide", `{"choice":"move","why":"The endpoint and its tests are in the handover."}`)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	stopStage(t, r, code)
	release := r.busyGate()
	defer release()
	setAutomation(t, r, created.ID, true)
	// The supervisor and these looks all find the gate full.
	for i := 0; i < 3; i++ {
		r.s.autoDecisions(context.Background())
		time.Sleep(50 * time.Millisecond)
	}
	o := mustOutcome(t, r, created.ID)
	if stage := o.Sessions[0].Stages[0]; calls.Load() != 0 || len(o.Decisions) != 0 || stage.Decided > 1 || o.Sessions[0].Stage != 0 {
		t.Fatalf("a busy provider decided or spent decisions: calls %d, %+v, %+v", calls.Load(), o.Decisions, stage)
	}
	release()
	r.s.autoDecisions(context.Background())
	got := decisions(t, r, created.ID, 1)[0]
	if got.Choice != "move" || got.Failed || calls.Load() != 1 {
		t.Fatalf("the stop was not put again once the gate was free: calls %d, %+v", calls.Load(), got)
	}
	if used := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.Stage == 1 }).Stages[0].Decided; used != 1 {
		t.Fatalf("the refusals spent decisions: %d", used)
	}
}
