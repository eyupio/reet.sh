package gaffer

import (
	"context"
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
