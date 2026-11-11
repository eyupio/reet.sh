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
	log.wait(t, "gaffer-stage-started code", "gaffer-stage-handed-over code gaffer", "gaffer-stage-started review")
	// The person moves review on; test is handed over by Gaffer and the pipeline stops for the merge decision.
	if _, err = r.s.MovePipelineOn(r.p.UserID, created.ID, review.Key); err != nil {
		t.Fatal(err)
	}
	test := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 2 })
	r.s.stageHandedOver(r.p.UserID, test.ConversationID, "All checks pass.", "gaffer")
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "ready_to_merge" })
	log.wait(t,
		"gaffer-stage-started code", "gaffer-stage-handed-over code gaffer",
		"gaffer-stage-started review", "gaffer-stage-handed-over review person",
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
	last := log.all()[6]
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
