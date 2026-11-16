package gaffer

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// A launch can start its session on the machine and still not answer in
// time. The pipeline reads as not started. Trying it again must find that
// session, not start a second agent in the same folder.
func TestALaunchThatAnsweredTooLateIsTakenUpWhenTriedAgain(t *testing.T) {
	wait := outcomeStartWait
	outcomeStartWait = 150 * time.Millisecond
	t.Cleanup(func() { outcomeStartWait = wait })
	r := outcomeFixture(t)
	r.in.Sessions = r.in.Sessions[:1]
	r.starter.late = "Export API"
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, r.in)
	if err != nil {
		t.Fatal(err)
	}
	failed := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "failed" })
	if failed.SessionID != "" || r.starter.sessions() != 1 {
		t.Fatalf("after the timeout: %+v sessions %d", failed, r.starter.sessions())
	}
	o := r.settle(t, created.ID)
	outcomeStartWait = wait
	if _, err = r.s.RetryPipeline(r.p.UserID, created.ID, failed.Key, o.Revision); err != nil {
		t.Fatal(err)
	}
	again := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if r.starter.sessions() != 1 {
		t.Fatalf("a second session was started beside the one that answered late: %d", r.starter.sessions())
	}
	if again.SessionID == "" || again.UnsureRequest != "" {
		t.Fatalf("the late session was not taken up: %+v", again)
	}
}

// The controller stopped while it was starting a stage's own session. When it
// picks the pipeline up again and that session is followed, the session of
// the stage before is finished with and is ended, as it is without a restart.
func TestTheStageBeforeIsEndedWhenItsSuccessorStartsAfterARestart(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	o := r.settle(t, created.ID)
	// As a restart leaves it: the code stage handed over, the review stage asked for and not yet answered.
	if _, err = r.st.UpdateGafferOutcomeFor(r.p.UserID, created.ID, o.Revision, func(o *store.GafferOutcome) bool {
		row := &o.Sessions[0]
		row.Stages[0].State, row.Stage, row.State = "done", 1, "starting"
		row.SessionID, row.ConversationID = "", ""
		o.State = "starting"
		return true
	}); err != nil {
		t.Fatal(err)
	}
	r.s.wakeOutcomes()
	review := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 1 })
	if review.SessionID == code.SessionID {
		t.Fatalf("review should have a session of its own: %+v", review)
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(r.starter.closed()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the code stage's session was left running after the restart")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := r.starter.closed(); got[0] != code.SessionID {
		t.Fatalf("ended %v, want %s", got, code.SessionID)
	}
}

// A pipeline's folder is in use for as long as the pipeline has work left,
// however many outcomes were given to Gaffer after it.
func TestAFolderStaysInUseHoweverManyOutcomesCameAfter(t *testing.T) {
	r := outcomeFixture(t)
	uid := r.p.UserID
	for i := 0; i < 70; i++ {
		o, err := r.st.CreateGafferOutcomeFor(uid, store.GafferOutcome{ID: fmt.Sprintf("%032x", i+1), Objective: "Ship it.", Sessions: []store.GafferOutcomeSession{{Key: "a", Title: "Part", Prompt: "Do it. Do not merge.", RequestID: fmt.Sprintf("%032x", 1000+i)}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = r.st.UpdateGafferOutcomeFor(uid, o.ID, o.Revision, func(o *store.GafferOutcome) bool {
			o.State, o.Sessions[0].State, o.Sessions[0].WorkDir = "settled", "following", fmt.Sprintf("/work/billing/.reet/worktrees/%d", i)
			return true
		}); err != nil {
			t.Fatal(err)
		}
	}
	if !r.s.WorkDirInUse(uid, "/work/billing/.reet/worktrees/0") {
		t.Fatal("the oldest pipeline's folder was reported free")
	}
	if r.s.WorkDirInUse(uid, "/work/billing/.reet/worktrees/none") {
		t.Fatal("a folder no pipeline works in was reported in use")
	}
}

// A client that did not hear back and asks again under the same request
// starts the outcome once.
func TestAnOutcomeAskedForTwiceUnderOneRequestStartsOnce(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	in := r.in
	in.Sessions = in.Sessions[:1]
	in.RequestID = fmt.Sprintf("%032x", 77)
	first, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil || first.ID != in.RequestID {
		t.Fatalf("first: %+v %v", first.ID, err)
	}
	r.piece(t, first.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	second, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil || second.ID != first.ID {
		t.Fatalf("second: %q %v", second.ID, err)
	}
	if rows, err := r.st.GafferOutcomesFor(r.p.UserID); err != nil || len(rows) != 1 || r.starter.sessions() != 1 {
		t.Fatalf("outcomes %d sessions %d %v", len(rows), r.starter.sessions(), err)
	}
	in.RequestID = "not-an-id"
	if _, err = r.s.CreateOutcome(r.p.UserID, in); err == nil {
		t.Fatal("a malformed request id was accepted")
	}
}

// Trying again while the machine cannot be reached proves nothing about the
// launch that answered late: its request is kept, so the retry that does
// reach the machine still finds that session instead of starting a second.
func TestALateLaunchIsStillTakenUpAfterARetryThatCouldNotReachTheMachine(t *testing.T) {
	wait := outcomeStartWait
	outcomeStartWait = 150 * time.Millisecond
	t.Cleanup(func() { outcomeStartWait = wait })
	r := outcomeFixture(t)
	r.in.Sessions = r.in.Sessions[:1]
	r.starter.late = "Export API"
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, r.in)
	if err != nil {
		t.Fatal(err)
	}
	failed := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "failed" })
	late := failed.UnsureRequest
	if late == "" {
		t.Fatalf("the unanswered launch was not remembered: %+v", failed)
	}
	outcomeStartWait = wait
	// The machine is offline for the first retry.
	r.starter.mu.Lock()
	r.starter.offline = true
	r.starter.mu.Unlock()
	o := r.settle(t, created.ID)
	if _, err = r.s.RetryPipeline(r.p.UserID, created.ID, failed.Key, o.Revision); err != nil {
		t.Fatal(err)
	}
	again := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "failed" && p.Note != failed.Note })
	if again.UnsureRequest != late || !strings.Contains(again.Note, "offline") {
		t.Fatalf("an unreachable machine was taken as proof the launch failed: %+v", again)
	}
	// Back online: the session that started late is the one followed.
	r.starter.mu.Lock()
	r.starter.offline = false
	r.starter.mu.Unlock()
	o = r.settle(t, created.ID)
	if _, err = r.s.RetryPipeline(r.p.UserID, created.ID, failed.Key, o.Revision); err != nil {
		t.Fatal(err)
	}
	followed := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if r.starter.sessions() != 1 || followed.UnsureRequest != "" {
		t.Fatalf("a second session was started: %d sessions, %+v", r.starter.sessions(), followed)
	}
}
