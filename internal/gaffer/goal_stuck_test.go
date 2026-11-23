package gaffer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func shortenWaits(t *testing.T, delivery, idle time.Duration) {
	t.Helper()
	d, i := instructionDeliveryWait, idleWorkerWait
	instructionDeliveryWait, idleWorkerWait = delivery, idle
	t.Cleanup(func() { instructionDeliveryWait, idleWorkerWait = d, i })
}

// An instruction that stays undelivered while the working model is not busy
// is not waited for until the goal expires: the goal needs the person, says
// why, and the stop is in the activity log. A busy model is still waited for.
func TestAnInstructionThatIsNeverDeliveredBringsTheGoalBackToThePerson(t *testing.T) {
	s, st, p := settingsFixture(t)
	log := recordAudit(s)
	worker := &goalWorker{fakeWorker: fakeWorker{st: st}}
	s.SetWorker(worker)
	var calls atomic.Int32
	fake := advancingGoalProvider(&calls)
	defer fake.Close()
	_, g := goalFixture(t, s, st, p, fake.URL, 12)
	workerProgress(t, st, g, 1)
	s.reviewDueGoals(context.Background())
	g = finishReview(t, s, g)
	if m, err := st.WorkerMessageFor(p.UserID, "gaffer", g.ConversationID, g.SessionID, g.PendingID); err != nil || m.State != "queued" {
		t.Fatalf("the instruction should be queued: %+v %v", m, err)
	}
	// Within the wait, and past it while the model works, Gaffer keeps waiting.
	g = dueGoal(t, st, g)
	s.reviewDueGoals(context.Background())
	shortenWaits(t, 0, time.Hour)
	worker.working.Store(true)
	g = dueGoal(t, st, g)
	s.reviewDueGoals(context.Background())
	if row, _ := st.GafferGoalFor(p.UserID, g.ConversationID); row.State != "active" || row.Note != "Waiting for the existing instruction's delivery." {
		t.Fatalf("a busy model's queue was not waited for: %+v", row)
	}
	// The model is idle and the instruction is still queued: nothing more will happen by waiting.
	worker.working.Store(false)
	reviews := calls.Load()
	g = dueGoal(t, st, g)
	s.reviewDueGoals(context.Background())
	row, _ := st.GafferGoalFor(p.UserID, g.ConversationID)
	if row.State != "needs_user" || row.WaitingOnWorker || !strings.Contains(row.Note, "without reaching the working model") || calls.Load() != reviews {
		t.Fatalf("undelivered: %+v calls=%d", row, calls.Load())
	}
	log.wait(t, "gaffer-goal-needs-you")
	if reason := log.all()[0].detail["reason"]; reason != row.Note {
		t.Fatalf("reason %q", reason)
	}
}

// A working model that sits idle after Gaffer has reviewed everything, with
// nothing queued for it, is usually waiting for the person. The goal says so
// instead of looking active; a resume starts the clock again.
func TestAnIdleWorkingModelWithNothingQueuedBringsTheGoalBackToThePerson(t *testing.T) {
	s, st, p := settingsFixture(t)
	log := recordAudit(s)
	worker := &goalWorker{fakeWorker: fakeWorker{st: st}, accept: true}
	s.SetWorker(worker)
	var calls atomic.Int32
	fake := advancingGoalProvider(&calls)
	defer fake.Close()
	_, g := goalFixture(t, s, st, p, fake.URL, 12)
	workerProgress(t, st, g, 1)
	s.reviewDueGoals(context.Background())
	g = finishReview(t, s, g)
	reviews := calls.Load()
	// Not idle for long enough yet.
	g = dueGoal(t, st, g)
	s.reviewDueGoals(context.Background())
	if row, _ := st.GafferGoalFor(p.UserID, g.ConversationID); row.State != "active" {
		t.Fatalf("too early: %+v", row)
	}
	shortenWaits(t, time.Hour, 0)
	// A model that is still working is left alone however long it takes.
	worker.working.Store(true)
	g = dueGoal(t, st, g)
	s.reviewDueGoals(context.Background())
	if row, _ := st.GafferGoalFor(p.UserID, g.ConversationID); row.State != "active" {
		t.Fatalf("a working model was called idle: %+v", row)
	}
	worker.working.Store(false)
	// Nor is one whose ended turn left work running.
	worker.background.Store(1)
	g = dueGoal(t, st, g)
	s.reviewDueGoals(context.Background())
	if row, _ := st.GafferGoalFor(p.UserID, g.ConversationID); row.State != "active" {
		t.Fatalf("a model with background work was called idle: %+v", row)
	}
	worker.background.Store(0)
	g = dueGoal(t, st, g)
	s.reviewDueGoals(context.Background())
	row, _ := st.GafferGoalFor(p.UserID, g.ConversationID)
	if row.State != "needs_user" || row.WaitingOnWorker || !strings.Contains(row.Note, "has been idle") || calls.Load() != reviews {
		t.Fatalf("idle: %+v calls=%d", row, calls.Load())
	}
	log.wait(t, "gaffer-goal-needs-you")

	// The person resumes: the idle time counts from now, not from the old review.
	idleWorkerWait = time.Hour
	v, err := s.Goal(p.UserID, g.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ControlGoal(p.UserID, g.ConversationID, row.Revision, "resume", v.Pins, true); err != nil {
		t.Fatal(err)
	}
	s.reviewDueGoals(context.Background())
	if again, _ := st.GafferGoalFor(p.UserID, g.ConversationID); again.State != "active" || again.ResumedAt == 0 {
		t.Fatalf("after resume: %+v", again)
	}
}

// A review of Gaffer's own that fails leaves the goal with the reason, marked
// so the page can offer to try again; resuming clears the mark.
func TestAFailedReviewSaysWhyAndCanBeTriedAgain(t *testing.T) {
	s, st, p := settingsFixture(t)
	log := recordAudit(s)
	worker := &goalWorker{fakeWorker: fakeWorker{st: st}, accept: true}
	s.SetWorker(worker)
	var calls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, `{"error":{"message":"model overloaded"}}`, http.StatusServiceUnavailable)
	}))
	defer fake.Close()
	_, g := goalFixture(t, s, st, p, fake.URL, 12)
	workerProgress(t, st, g, 1)
	s.reviewDueGoals(context.Background())
	deadline := time.Now().Add(5 * time.Second)
	var row = g
	for time.Now().Before(deadline) {
		row, _ = st.GafferGoalFor(p.UserID, g.ConversationID)
		if row.State == "needs_user" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if row.State != "needs_user" || !row.ReviewFailed || !strings.Contains(row.Note, "did not complete (failed)") || !strings.Contains(row.Note, "resume the goal to try again") {
		t.Fatalf("failed review: %+v", row)
	}
	log.wait(t, "gaffer-goal-needs-you")
	v, err := s.Goal(p.UserID, g.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ControlGoal(p.UserID, g.ConversationID, row.Revision, "resume", v.Pins, true); err != nil {
		t.Fatal(err)
	}
	if again, _ := st.GafferGoalFor(p.UserID, g.ConversationID); again.ReviewFailed || again.State != "active" {
		t.Fatalf("after resume: %+v", again)
	}
}

// A review that only writes prose leaves the goal saying so, not "needs your
// decision": nothing was asked of the person.
func TestAReviewThatChoosesNoStepSaysSoInsteadOfAskingForADecision(t *testing.T) {
	s, st, p := settingsFixture(t)
	worker := &goalWorker{fakeWorker: fakeWorker{st: st}, accept: true}
	s.SetWorker(worker)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"All quiet; nothing needs you yet.\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer fake.Close()
	_, g := goalFixture(t, s, st, p, fake.URL, 12)
	workerProgress(t, st, g, 1)
	s.reviewDueGoals(context.Background())
	deadline := time.Now().Add(5 * time.Second)
	row := g
	for time.Now().Before(deadline) {
		row, _ = st.GafferGoalFor(p.UserID, g.ConversationID)
		if row.State == "needs_user" && row.NoStep {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if row.State != "needs_user" || !row.NoStep || row.Note != goalNoStepNote || row.ReviewFailed {
		t.Fatalf("no-step review: %+v", row)
	}
	v, _ := s.Goal(p.UserID, g.ConversationID)
	if _, err := s.ControlGoal(p.UserID, g.ConversationID, row.Revision, "resume", v.Pins, true); err != nil {
		t.Fatal(err)
	}
	if again, _ := st.GafferGoalFor(p.UserID, g.ConversationID); again.NoStep {
		t.Fatalf("resume kept the mark: %+v", again)
	}
}

// An approval nobody answers expires and stops the agent's turn. That is not
// the person having answered: the goal says what happened, and resuming it
// has Gaffer review where the agent stopped instead of asking again.
func TestAnApprovalThatExpiredIsSaidToHaveExpiredAndAResumeCarriesOn(t *testing.T) {
	s, st, p := settingsFixture(t)
	worker := &goalWorker{fakeWorker: fakeWorker{st: st}, accept: true}
	s.SetWorker(worker)
	var calls atomic.Int32
	fake := advancingGoalProvider(&calls)
	defer fake.Close()
	_, g := goalFixture(t, s, st, p, fake.URL, 4)
	workerProgress(t, st, g, 1)
	worker.waiting.Store(true)
	s.reviewDueGoals(context.Background())
	held, _ := st.GafferGoalFor(p.UserID, g.ConversationID)
	if !held.WaitingOnWorker {
		t.Fatalf("held: %+v", held)
	}
	// The request expires: the agent stops waiting and its turn is cancelled.
	worker.waiting.Store(false)
	if err := st.AppendTerminalChatEventFor(p.UserID, g.SessionID, 2, time.Now().UnixMilli(), []byte(`{"kind":"status","state":"cancelled"}`)); err != nil {
		t.Fatal(err)
	}
	if err := st.HoldGafferGoalForWorker(held, held.Note, 0); err != nil {
		t.Fatal(err)
	}
	s.reviewDueGoals(context.Background())
	expired, _ := st.GafferGoalFor(p.UserID, g.ConversationID)
	if expired.State != "needs_user" || expired.WaitingOnWorker || !strings.Contains(expired.Note, "until it expired") || calls.Load() != 0 {
		t.Fatalf("an expired request must not read as answered: %+v calls=%d", expired, calls.Load())
	}
	// The person resumes. The stop they resumed from is not asked about again: the goal is reviewed.
	time.Sleep(5 * time.Millisecond)
	v, err := s.Goal(p.UserID, g.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ControlGoal(p.UserID, g.ConversationID, v.Goal.Revision, "resume", v.Pins, true); err != nil {
		t.Fatal(err)
	}
	s.reviewDueGoals(context.Background())
	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	after, _ := st.GafferGoalFor(p.UserID, g.ConversationID)
	if calls.Load() == 0 || strings.Contains(after.Note, "stopped or failed") {
		t.Fatalf("a resume after the stop must be reviewed, not sent back: %+v calls=%d", after, calls.Load())
	}
	finishReview(t, s, g)
}

// Gaffer's instruction failed: the queue knows it was not delivered and why.
// The goal says so at once, with that reason, instead of waiting half an hour
// for progress that an agent which was never told anything will not make.
func TestAnInstructionThatFailedBringsTheGoalBackWithTheReason(t *testing.T) {
	s, st, p := settingsFixture(t)
	worker := &goalWorker{fakeWorker: fakeWorker{st: st}}
	s.SetWorker(worker)
	var calls atomic.Int32
	fake := advancingGoalProvider(&calls)
	defer fake.Close()
	_, g := goalFixture(t, s, st, p, fake.URL, 12)
	workerProgress(t, st, g, 1)
	s.reviewDueGoals(context.Background())
	g = finishReview(t, s, g)
	m, err := st.WorkerMessageFor(p.UserID, "gaffer", g.ConversationID, g.SessionID, g.PendingID)
	if err != nil || m.State != "queued" {
		t.Fatalf("the instruction should be queued: %+v %v", m, err)
	}
	queue, err := st.TerminalQueueFor(p.UserID, g.SessionID)
	if err != nil || len(queue) != 1 {
		t.Fatalf("queue: %+v %v", queue, err)
	}
	if ok, err := st.TransitionTerminalQueueFor(p.UserID, g.SessionID, queue[0], "failed", "wait", "The agent's approval mode is unknown.", "test"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	reviews := calls.Load()
	g = dueGoal(t, st, g)
	s.reviewDueGoals(context.Background())
	row, _ := st.GafferGoalFor(p.UserID, g.ConversationID)
	if row.State != "needs_user" || !strings.Contains(row.Note, "did not reach the working model") || !strings.Contains(row.Note, "approval mode is unknown") || calls.Load() != reviews {
		t.Fatalf("a failed instruction: %+v calls=%d", row, calls.Load())
	}
}

// askingGoalProvider answers a goal review with ask_user: the goal stops with
// the person, as it does when Gaffer brings a question back.
func askingGoalProvider(calls *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct {
			Tools []json.RawMessage `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return
		}
		if len(body.Tools) == 0 {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Is this done?\"}}]}\n\ndata: [DONE]\n\n")
			return
		}
		raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{
			"index": 0, "id": "call_one", "type": "function",
			"function": map[string]any{"name": "ask_user", "arguments": `{"summary":"Is this done?"}`},
		}}}}}})
		fmt.Fprintf(w, "data: %s\n\n", raw)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
	}))
}

// A goal that stopped for the person keeps watching its session. Once the
// working model moves on by itself, what Gaffer stopped on no longer holds: it
// follows again without the person resuming it, and the new progress is reviewed.
func TestAGoalThatStoppedForThePersonFollowsAgainWhenTheSessionMovesOn(t *testing.T) {
	s, st, p := settingsFixture(t)
	worker := &goalWorker{fakeWorker: fakeWorker{st: st}, accept: true}
	s.SetWorker(worker)
	var calls atomic.Int32
	fake := askingGoalProvider(&calls)
	defer fake.Close()
	_, g := goalFixture(t, s, st, p, fake.URL, 12)
	workerProgress(t, st, g, 1)
	s.reviewDueGoals(context.Background())
	g = finishReview(t, s, g)
	if g.State != "needs_user" || g.ReviewFailed || g.WaitingOnWorker {
		t.Fatalf("the review should have brought the goal back: %+v", g)
	}
	reviews := calls.Load()
	// Nothing new yet: it stays with the person, and no model request is made.
	s.reviewStoppedGoal(context.Background(), g)
	if row, _ := st.GafferGoalFor(p.UserID, g.ConversationID); row.State != "needs_user" || calls.Load() != reviews {
		t.Fatalf("an unchanged session must not be resumed: %+v calls=%d", row, calls.Load())
	}
	// The working session moves on without the person touching the goal.
	workerProgress(t, st, g, 2)
	row, err := st.GafferGoalFor(p.UserID, g.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	s.reviewStoppedGoal(context.Background(), row)
	released, _ := st.GafferGoalFor(p.UserID, g.ConversationID)
	if released.State != "active" || released.WaitingOnWorker || calls.Load() != reviews {
		t.Fatalf("new progress should release the goal: %+v calls=%d", released, calls.Load())
	}
	// The progress that released it is reviewed like any other.
	s.reviewDueGoals(context.Background())
	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() == reviews && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if calls.Load() == reviews {
		t.Fatal("the new progress was not reviewed")
	}
	finishReview(t, s, g)
}

// A review that failed is the person's to retry: new worker progress alone does
// not clear it, so the reason is not silently lost.
func TestAFailedReviewStaysWithThePersonWhenTheSessionMovesOn(t *testing.T) {
	s, st, p := settingsFixture(t)
	worker := &goalWorker{fakeWorker: fakeWorker{st: st}, accept: true}
	s.SetWorker(worker)
	var calls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, `{"error":{"message":"model overloaded"}}`, http.StatusServiceUnavailable)
	}))
	defer fake.Close()
	_, g := goalFixture(t, s, st, p, fake.URL, 12)
	workerProgress(t, st, g, 1)
	s.reviewDueGoals(context.Background())
	deadline := time.Now().Add(5 * time.Second)
	row := g
	for time.Now().Before(deadline) {
		row, _ = st.GafferGoalFor(p.UserID, g.ConversationID)
		if row.State == "needs_user" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !row.ReviewFailed {
		t.Fatalf("the failed review should need the person: %+v", row)
	}
	workerProgress(t, st, g, 2)
	s.reviewStoppedGoal(context.Background(), row)
	if again, _ := st.GafferGoalFor(p.UserID, g.ConversationID); again.State != "needs_user" || !again.ReviewFailed {
		t.Fatalf("a failed review must stay with the person: %+v", again)
	}
}

// A session that is working now but has not retained a new event since the
// stop is still followed again: the agent's own live report says it moved on.
func TestALiveWorkingSessionReleasesAGoalWithoutANewEvent(t *testing.T) {
	s, st, p := settingsFixture(t)
	worker := &goalWorker{fakeWorker: fakeWorker{st: st}, accept: true}
	s.SetWorker(worker)
	var calls atomic.Int32
	fake := askingGoalProvider(&calls)
	defer fake.Close()
	_, g := goalFixture(t, s, st, p, fake.URL, 12)
	workerProgress(t, st, g, 1)
	s.reviewDueGoals(context.Background())
	g = finishReview(t, s, g)
	if g.State != "needs_user" || g.ReviewFailed {
		t.Fatalf("the review should have brought the goal back: %+v", g)
	}
	if err := st.SetTerminalChatAttentionFor(p.UserID, g.SessionID, time.Now().UnixMilli()+1000, 0, "working", 0); err != nil {
		t.Fatal(err)
	}
	s.reviewStoppedGoal(context.Background(), g)
	if row, _ := st.GafferGoalFor(p.UserID, g.ConversationID); row.State != "active" {
		t.Fatalf("a working session should release the goal: %+v", row)
	}
}
