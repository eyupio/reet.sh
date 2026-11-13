package gaffer

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

type auditRow struct {
	uid    int64
	action string
	detail map[string]any
}

// auditLog is the activity log a test installs with SetAudit.
type auditLog struct {
	mu   sync.Mutex
	rows []auditRow
}

func recordAudit(s *Service) *auditLog {
	l := &auditLog{}
	s.SetAudit(func(uid int64, action string, detail map[string]any) {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.rows = append(l.rows, auditRow{uid, action, detail})
	})
	return l
}

// lines is each recorded event as "action stage by", oldest first.
func (l *auditLog) lines() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, r := range l.rows {
		line := r.action
		for _, k := range []string{"stage", "by"} {
			if v, ok := r.detail[k]; ok {
				line += " " + fmt.Sprint(v)
			}
		}
		out = append(out, line)
	}
	return out
}

func (l *auditLog) all() []auditRow {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]auditRow(nil), l.rows...)
}

// wait blocks until the log reads as want, since the controller records a step just after saving it.
func (l *auditLog) wait(t *testing.T, want ...string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := l.lines()
		if strings.Join(got, "\n") == strings.Join(want, "\n") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("activity log:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Every stage Gaffer hands over, every stage the controller starts and the
// stop at the merge decision is in the activity log with the outcome,
// pipeline, stage and the controller's reason. The handover's own words are
// session text and stay on the stage.
func TestAPipelinesMovesAreRecordedWithTheirReason(t *testing.T) {
	r := outcomeFixture(t)
	log := recordAudit(r.s)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	log.wait(t, "gaffer-stage-started code")
	r.s.stageHandedOver(r.p.UserID, code.ConversationID, "Endpoint and tests are committed. secret-handover-words", "gaffer")
	review := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 1 })
	log.wait(t, "gaffer-stage-started code", "gaffer-stage-handed-over code gaffer", "gaffer-stage-started review", "gaffer-session-ended")
	// The person moves review on; test is handed over by Gaffer and the pipeline stops for the merge decision.
	if _, err = r.s.MovePipelineOn(r.p.UserID, created.ID, review.Key, ""); err != nil {
		t.Fatal(err)
	}
	test := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 2 })
	r.s.stageHandedOver(r.p.UserID, test.ConversationID, "All checks pass.", "gaffer")
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "ready_to_merge" })
	log.wait(t,
		"gaffer-stage-started code", "gaffer-stage-handed-over code gaffer",
		"gaffer-stage-started review", "gaffer-session-ended", "gaffer-stage-handed-over review person",
		"gaffer-stage-started test", "gaffer-stage-handed-over test gaffer",
		"gaffer-pipeline-waiting-merge merge")
	for _, row := range log.all() {
		if row.uid != r.p.UserID || row.detail["outcome"] != created.ID || row.detail["pipeline"] != code.Key || row.detail["reason"] == "" {
			t.Fatalf("event without its outcome, pipeline or reason: %+v", row)
		}
		if strings.Contains(fmt.Sprint(row.detail), "secret-handover-words") {
			t.Fatalf("session text reached the activity log: %+v", row)
		}
	}
	// Each done stage keeps who closed it, so the page can mark a stage nobody reviewed.
	final := mustOutcome(t, r, created.ID).Sessions[0].Stages
	if final[0].ClosedBy != "gaffer" || final[1].ClosedBy != "person" || final[2].ClosedBy != "gaffer" || final[3].ClosedBy != "" {
		t.Fatalf("closed by: %q %q %q %q", final[0].ClosedBy, final[1].ClosedBy, final[2].ClosedBy, final[3].ClosedBy)
	}
	last := log.all()[7]
	if last.detail["state"] != "ready_to_merge" || !strings.Contains(fmt.Sprint(last.detail["reason"]), "Nothing merges until you approve it") {
		t.Fatalf("merge wait: %+v", last)
	}
}

// A stage that cannot start is recorded as the pipeline stopping, with why.
func TestAPipelineThatStopsIsRecordedWithWhy(t *testing.T) {
	r := outcomeFixture(t)
	log := recordAudit(r.s)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	r.starter.mu.Lock()
	r.starter.refuse["Export API · review"] = "the machine is offline"
	r.starter.mu.Unlock()
	r.s.stageHandedOver(r.p.UserID, code.ConversationID, "Done.", "gaffer")
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "unfollowed" })
	log.wait(t, "gaffer-stage-started code", "gaffer-stage-handed-over code gaffer", "gaffer-pipeline-stopped review")
	stopped := log.all()[2]
	if stopped.detail["state"] != "unfollowed" || !strings.Contains(fmt.Sprint(stopped.detail["reason"]), "the machine is offline") {
		t.Fatalf("stopped: %+v", stopped)
	}
}

// A goal held while the working model waits for the person is recorded once
// when held (not at every look) and once when released; a goal that ends up
// needing the person is recorded with the reason.
func TestAGoalsStopsAndReleasesAreRecordedOnce(t *testing.T) {
	s, st, p := settingsFixture(t)
	log := recordAudit(s)
	worker := &goalWorker{fakeWorker: fakeWorker{st: st}, accept: true}
	s.SetWorker(worker)
	var calls atomic.Int32
	fake := advancingGoalProvider(&calls)
	defer fake.Close()
	_, g := goalFixture(t, s, st, p, fake.URL, 4)
	workerProgress(t, st, g, 1)
	worker.waiting.Store(true)
	makeDue := func() {
		t.Helper()
		row, _ := st.GafferGoalFor(p.UserID, g.ConversationID)
		if row.WaitingOnWorker {
			if err := st.HoldGafferGoalForWorker(row, row.Note, 0); err != nil {
				t.Fatal(err)
			}
		}
	}
	for i := 0; i < 3; i++ {
		s.reviewDueGoals(context.Background())
		makeDue()
	}
	log.wait(t, "gaffer-goal-held")
	held := log.all()[0]
	if held.uid != p.UserID || held.detail["conversation"] != g.ConversationID || held.detail["session"] != g.SessionID || held.detail["goal"] != g.ID || held.detail["reason"] != workerWaitingNote {
		t.Fatalf("held: %+v", held)
	}
	worker.waiting.Store(false)
	s.reviewDueGoals(context.Background())
	log.wait(t, "gaffer-goal-held", "gaffer-goal-released")

	// The provider loses its permission: the goal needs the person, and says why.
	row, _ := st.GafferGoalFor(p.UserID, g.ConversationID)
	if row.State != "active" {
		t.Fatalf("released goal: %+v", row)
	}
	provider, _ := st.GafferProviderFor(p.UserID, g.ProviderID)
	in := inputFrom(provider)
	in.AllowCode = false
	if _, err := s.Save(p, in); err != nil {
		t.Fatal(err)
	}
	if err := st.ObserveGafferGoal(row, "active", "", 0); err != nil {
		t.Fatal(err)
	}
	s.reviewDueGoals(context.Background())
	s.reviewDueGoals(context.Background())
	log.wait(t, "gaffer-goal-held", "gaffer-goal-released", "gaffer-goal-needs-you")
	if needs := log.all()[2]; !strings.Contains(fmt.Sprint(needs.detail["reason"]), "permission changed") {
		t.Fatalf("needs you: %+v", needs)
	}
	if calls.Load() != 0 {
		t.Fatalf("a model request was made: %d", calls.Load())
	}
}

// A pipeline whose agent runs on YOLO does not also merge by itself unless
// the person confirmed exactly that: set to auto-merge at approval, it still
// stops at Ready to merge; the switch refuses without the confirmation and,
// with it, lets the merge stage start.
func TestAYoloPipelineStopsAtTheMergeUnlessThePersonConfirmedIt(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	in := pipelineInput(r, true)
	in.Yolo = true
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil || !created.Yolo || !created.Sessions[0].AutoMerge || MergesByItself(created, created.Sessions[0]) {
		t.Fatalf("outcome: %+v %v", created, err)
	}
	for stage := 0; stage < 3; stage++ {
		at := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == stage })
		r.s.stageHandedOver(r.p.UserID, at.ConversationID, "Done.", "gaffer")
	}
	ready := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "ready_to_merge" })
	if !strings.Contains(ready.Note, "runs on YOLO") || ready.Stages[3].State != "pending" {
		t.Fatalf("a YOLO pipeline went into its merge unconfirmed: %+v", ready)
	}
	if _, err = r.s.SetPipelineAutoMerge(r.p.UserID, created.ID, ready.Key, true, false); err == nil || !strings.Contains(err.Error(), "YOLO") {
		t.Fatalf("merge-by-itself on YOLO was accepted unconfirmed: %v", err)
	}
	on, err := r.s.SetPipelineAutoMerge(r.p.UserID, created.ID, ready.Key, true, true)
	if err != nil || !on.Sessions[0].YoloMergeOK || !MergesByItself(on, on.Sessions[0]) {
		t.Fatalf("confirmed: %+v %v", on.Sessions[0], err)
	}
	// Turning it off again withdraws the confirmation too.
	off, err := r.s.SetPipelineAutoMerge(r.p.UserID, created.ID, ready.Key, false, false)
	if err != nil || off.Sessions[0].AutoMerge || off.Sessions[0].YoloMergeOK {
		t.Fatalf("off: %+v %v", off.Sessions[0], err)
	}
	// An outcome on ordinary approvals is unchanged: auto-merge needs nothing more.
	plain, err := r.s.CreateOutcome(r.p.UserID, func() OutcomeInput { in := pipelineInput(r, true); in.Objective = "Another outcome."; return in }())
	if err != nil || !MergesByItself(plain, plain.Sessions[0]) {
		t.Fatalf("plain auto-merge: %+v %v", plain, err)
	}
}

// A stage that carries on in the same session is given its instruction by
// the controller, not left for Gaffer's first review to send.
func TestAStageInTheSameSessionIsToldItsWorkByTheController(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	r.s.stageHandedOver(r.p.UserID, code.ConversationID, "Endpoint committed.", "gaffer")
	review := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 1 })
	before, err := r.st.TerminalQueueFor(r.p.UserID, review.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	// Review to test keeps the session (same agent and model).
	r.s.stageHandedOver(r.p.UserID, review.ConversationID, "Two fixes committed.", "gaffer")
	test := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 2 })
	if test.SessionID != review.SessionID || test.Note != "The test stage is under way in the same session." {
		t.Fatalf("test stage: %+v", test)
	}
	after, err := r.st.TerminalQueueFor(r.p.UserID, test.SessionID)
	if err != nil || len(after) != len(before)+1 {
		t.Fatalf("queued for the session: %d before, %d after, %v", len(before), len(after), err)
	}
	sent := after[len(after)-1].Text
	if !strings.HasPrefix(sent, "You are doing ONE stage of a staged piece of work: the test stage.") || !strings.Contains(sent, "Handover from the review stage: Two fixes committed.") {
		t.Fatalf("instruction: %q", sent)
	}
}

// The person's own instruction for a pipeline's agent is queued for its
// session. (It was refused: a message with no Gaffer turn was treated as a
// model's and failed its turn check.)
func TestThePersonsInstructionReachesThePipelinesAgent(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	sent, err := r.s.TellPipelineAgent(context.Background(), r.p.UserID, created.ID, code.Key, strings.Repeat("7", 32), "Write the plan as text now. Change no code.")
	if err != nil || sent.State != "queued" && sent.State != "sent" && sent.State != "sending" {
		t.Fatalf("tell: %+v %v", sent, err)
	}
	queue, err := r.st.TerminalQueueFor(r.p.UserID, code.SessionID)
	if err != nil || len(queue) == 0 || queue[len(queue)-1].Text != "Write the plan as text now. Change no code." {
		t.Fatalf("queue: %+v %v", queue, err)
	}
	if _, err = r.s.TellPipelineAgent(context.Background(), r.p.UserID, created.ID, "zz", strings.Repeat("8", 32), "x"); err == nil {
		t.Fatal("an unknown pipeline was told")
	}
}
