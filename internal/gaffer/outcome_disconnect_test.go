package gaffer

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// These tests drop the machine, or stop the controller, at the four moments a
// pipeline is moved by the controller: launch, fallback, approval and
// handover. Each records a transition that holds.

// restart stops the controller and starts another on the same store, with the
// same machine behind it. The old controller's in-flight calls end with it.
func (r *outcomeRig) restart(t *testing.T) {
	t.Helper()
	r.s.Close()
	seal, err := sealed.NewSealer(strings.Repeat("test-key-", 8), "test")
	if err != nil {
		t.Fatal(err)
	}
	next := New(r.st, seal)
	t.Cleanup(next.Close)
	next.SetWorker(&goalWorker{fakeWorker: fakeWorker{st: r.st}})
	next.SetSessionStarter(r.starter)
	next.StartGoalSupervisor()
	r.s = next
}

// setOffline makes every launch answer as a machine that cannot be reached.
func (r *outcomeRig) setOffline(on bool) {
	r.starter.mu.Lock()
	r.starter.offline = on
	r.starter.mu.Unlock()
}

// headUnreadable makes the machine unable to say where a folder stands.
func (r *outcomeRig) headUnreadable(on bool) {
	r.starter.mu.Lock()
	defer r.starter.mu.Unlock()
	if !on {
		r.starter.work = nil
		return
	}
	r.starter.work = func(string, bool) (terminalproto.InspectResult, error) {
		return terminalproto.InspectResult{}, errors.New("the machine is offline")
	}
}

// retry tries a pipeline again on the revision it reads, as the page does.
func (r *outcomeRig) retry(t *testing.T, id string) {
	t.Helper()
	o := r.settle(t, id)
	if _, err := r.s.RetryPipeline(r.p.UserID, id, "a", o.Revision); err != nil {
		t.Fatal(err)
	}
}

// startsNothingMore wakes the controller a few times and checks no launch
// was asked for meanwhile: nothing is retried on its own.
func (r *outcomeRig) startsNothingMore(t *testing.T, launches int) {
	t.Helper()
	for i := 0; i < 3; i++ {
		r.s.wakeOutcomes()
		time.Sleep(30 * time.Millisecond)
	}
	if n := len(r.starter.asked()); n != launches {
		t.Fatalf("launches asked for on their own: %d, want %d", n, launches)
	}
}

// Launch, machine unreachable: the pipeline says so and waits for Try again,
// which starts the one session once the machine is back.
func TestALaunchThatCannotReachTheMachineWaitsAndStartsOnceWhenTriedAgain(t *testing.T) {
	r := outcomeFixture(t)
	r.setOffline(true)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	failed := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "failed" })
	if failed.SessionID != "" || !strings.Contains(failed.Note, "offline") || r.starter.sessions() != 0 {
		t.Fatalf("after an unreachable launch: %+v", failed)
	}
	r.startsNothingMore(t, 1)
	r.setOffline(false)
	r.retry(t, created.ID)
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if r.starter.sessions() != 1 || code.Stage != 0 || code.ConversationID == "" {
		t.Fatalf("after Try again: %+v sessions=%d", code, r.starter.sessions())
	}
}

// Launch, controller restarted after the machine started the session and
// before its answer was recorded: the next controller asks under the same
// request and takes that session up.
func TestALaunchWhoseAnswerWasLostToARestartIsTakenUp(t *testing.T) {
	r := outcomeFixture(t)
	r.starter.late = "Export API · code"
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for r.starter.sessions() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the launch never reached the machine")
		}
		time.Sleep(10 * time.Millisecond)
	}
	r.restart(t)
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	asked := r.starter.asked()
	if r.starter.sessions() != 1 || len(asked) != 2 || asked[1].RequestID != asked[0].RequestID || code.SessionID == "" {
		t.Fatalf("after the restart: %+v sessions=%d launches=%d", code, r.starter.sessions(), len(asked))
	}
}

// Handover, controller restarted between recording the handover and
// starting the next stage: the next controller starts that stage once.
func TestAHandoverRecordedJustBeforeARestartStartsTheNextStageOnce(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	r.settle(t, created.ID)
	r.s.Close()
	r.s.stageHandedOver(r.p.UserID, code.ConversationID, "Endpoint and tests are committed.", "gaffer")
	if p := mustOutcome(t, r, created.ID).Sessions[0]; p.State != "advancing" || p.Stages[0].State != "done" {
		t.Fatalf("handover not recorded before the restart: %+v", p)
	}
	r.restart(t)
	review := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 1 })
	if r.starter.sessions() != 2 || len(r.starter.asked()) != 2 || review.SessionID == code.SessionID || review.Stages[0].Summary != "Endpoint and tests are committed." {
		t.Fatalf("after the restart: %+v sessions=%d", review, r.starter.sessions())
	}
	if review.Stages[1].RequestID != r.starter.asked()[1].RequestID || review.WorkDir != code.WorkDir {
		t.Fatalf("the next stage was not started where the first worked: %+v", review)
	}
}

// Handover, machine unreachable for the next stage: the handover stays
// recorded, the pipeline says the stage did not start, and Try again starts
// it once in the same folder.
func TestANextStageThatCannotReachTheMachineKeepsTheHandoverAndStartsOnceWhenTriedAgain(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	r.setOffline(true)
	r.s.stageHandedOver(r.p.UserID, code.ConversationID, "Endpoint and tests are committed.", "gaffer")
	stopped := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "unfollowed" })
	if stopped.Stage != 1 || stopped.Stages[0].State != "done" || stopped.Stages[0].Summary == "" || !strings.Contains(stopped.Note, "review stage was not started") || !strings.Contains(stopped.Note, "offline") {
		t.Fatalf("after an unreachable next stage: %+v", stopped)
	}
	r.startsNothingMore(t, 2)
	r.setOffline(false)
	r.retry(t, created.ID)
	review := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 1 })
	asked := r.starter.asked()
	if r.starter.sessions() != 2 || asked[len(asked)-1].Dir != code.WorkDir || !strings.Contains(asked[len(asked)-1].Prompt, "Endpoint and tests are committed.") || review.SessionID == code.SessionID {
		t.Fatalf("after Try again: %+v sessions=%d", review, r.starter.sessions())
	}
}

// Approval, machine unreachable when the person accepts: nothing is recorded
// and the pipeline still waits for the decision.
func TestADeliveryIsNotAcceptedWhileItsMachineCannotBeReached(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, deliveryInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	toReview(t, r, created.ID)
	v := r.review(t, created.ID)
	r.headUnreadable(true)
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", decisionFor(v, "accept", 1)); conflictCode(err) != "delivery_head_unverified" {
		t.Fatalf("accept while unreachable: %v", err)
	}
	if p := mustOutcome(t, r, created.ID).Sessions[0]; p.State != "delivery_review" || len(p.Deliveries) != 0 || p.DeliveryAccepted || p.ReviewRevision != v.ReviewRevision {
		t.Fatalf("after the refused decision: %+v", p)
	}
	r.headUnreadable(false)
	if _, _, _, recorded, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", decisionFor(v, "accept", 1)); err != nil || !recorded {
		t.Fatalf("accept once reachable: %v %v", err, recorded)
	}
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "ready_to_merge" })
}

// Approval, machine unreachable when the merge is about to start: the head
// cannot be read, so the pipeline goes back to its review with the
// acceptance and the approval cleared, the decision kept in its history and
// nothing launched. Once reachable, deciding again starts the merge once.
func TestAnApprovedMergeWhoseHeadCannotBeReadGoesBackToReviewAndStartsNothing(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, deliveryInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	toReview(t, r, created.ID)
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", decisionFor(r.review(t, created.ID), "accept", 1)); err != nil {
		t.Fatal(err)
	}
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "ready_to_merge" })
	launches := len(r.starter.asked())
	r.headUnreadable(true)
	r.approveMerge(t, created.ID)
	back := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "delivery_review" })
	if back.DeliveryAccepted || back.MergeApproved || len(back.Deliveries) != 1 || back.Deliveries[0].Decision != "accept" || !strings.Contains(back.Note, "could not be read") {
		t.Fatalf("back for review: %+v", back)
	}
	r.startsNothingMore(t, launches)
	r.headUnreadable(false)
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", decisionFor(r.review(t, created.ID), "accept", 2)); err != nil {
		t.Fatal(err)
	}
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "ready_to_merge" })
	r.approveMerge(t, created.ID)
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 2 })
	if got := len(r.starter.asked()); got != launches+1 {
		t.Fatalf("merge launches: %d", got-launches)
	}
}

// Approval, controller restarted right after the acceptance and again right
// after the merge approval was recorded: each decision stands and the merge
// stage starts once.
func TestDecisionsRecordedJustBeforeARestartStandAndTheMergeStartsOnce(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, deliveryInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	toReview(t, r, created.ID)
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", decisionFor(r.review(t, created.ID), "accept", 1)); err != nil {
		t.Fatal(err)
	}
	r.restart(t)
	ready := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "ready_to_merge" })
	if !ready.DeliveryAccepted || len(ready.Deliveries) != 1 {
		t.Fatalf("the acceptance did not survive the restart: %+v", ready)
	}
	launches := len(r.starter.asked())
	o := r.settle(t, created.ID)
	r.s.Close()
	if _, err := r.s.ApproveMerge(r.p.UserID, created.ID, "a", o.Revision); err != nil {
		t.Fatal(err)
	}
	r.restart(t)
	merge := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 2 })
	if !merge.MergeApproved || !merge.DeliveryAccepted || len(r.starter.asked()) != launches+1 {
		t.Fatalf("after the restart: %+v launches=%d", merge, len(r.starter.asked())-launches)
	}
}

// Approval, controller restarted while the merge stage's launch was in flight
// and the machine had started it: with the head unchanged and readable, the
// next controller takes that session up under the same request.
func TestAMergeLaunchInFlightAtARestartIsTakenUpWhenTheHeadStands(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, deliveryInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	toReview(t, r, created.ID)
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", decisionFor(r.review(t, created.ID), "accept", 1)); err != nil {
		t.Fatal(err)
	}
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "ready_to_merge" })
	r.starter.mu.Lock()
	r.starter.late = "Export API · merge"
	r.starter.mu.Unlock()
	before := r.starter.sessions()
	r.approveMerge(t, created.ID)
	deadline := time.Now().Add(10 * time.Second)
	for r.starter.sessions() == before {
		if time.Now().After(deadline) {
			t.Fatal("the merge launch never reached the machine")
		}
		time.Sleep(10 * time.Millisecond)
	}
	r.restart(t)
	merge := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 2 })
	asked := r.starter.asked()
	if r.starter.sessions() != before+1 || asked[len(asked)-1].RequestID != asked[len(asked)-2].RequestID || !merge.MergeApproved {
		t.Fatalf("after the restart: %+v sessions=%d", merge, r.starter.sessions()-before)
	}
}

// usageLimited runs a one-stage pipeline with a backup until its agent stops
// at its usage limit and Gaffer's review has seen it.
func usageLimited(t *testing.T, r *outcomeRig) (store.GafferOutcome, store.GafferOutcomeSession) {
	t.Helper()
	if _, err := r.s.SaveCrew(r.p.UserID, store.GafferCrew{Standard: store.GafferCrewTier{Tool: "opencode", Model: "opencode/primary", Backup: &store.GafferStageAgent{Tool: "opencode", Model: "paid/backup"}}}); err != nil {
		t.Fatal(err)
	}
	in := pipelineInput(r, false)
	in.Sessions[0].Stages = []OutcomeStageInput{{Kind: "code"}}
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	first := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	r.settle(t, created.ID)
	goal, err := r.st.GafferGoalFor(r.p.UserID, first.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.st.AppendTerminalChatEventFor(r.p.UserID, first.SessionID, 1, time.Now().UnixMilli(), []byte(`{"kind":"status","state":"error","detail":"Free usage limit reached."}`)); err != nil {
		t.Fatal(err)
	}
	r.s.reviewGoal(t.Context(), goal)
	return mustOutcome(t, r, created.ID), first
}

// Fallback after a usage limit, machine unreachable for the backup: the
// switch stays recorded, the pipeline says it did not start, and Try again
// starts the backup once.
func TestABackupThatCannotReachTheMachineIsStartedOnceWhenTriedAgain(t *testing.T) {
	r := outcomeFixture(t)
	o, first := usageLimited(t, r)
	r.setOffline(true)
	if _, err := r.s.RecoverPipelineWorker(r.p.UserID, o.ID, first.Key, first.SessionID, o.Revision, nil); err != nil {
		t.Fatal(err)
	}
	failed := r.piece(t, o.ID, func(p store.GafferOutcomeSession) bool { return p.State == "failed" })
	if stage := failed.Stages[0]; stage.Model != "paid/backup" || !stage.UsageFailover || failed.SessionID != "" || !strings.Contains(failed.Note, "offline") {
		t.Fatalf("after an unreachable backup: %+v", failed)
	}
	r.startsNothingMore(t, 2)
	r.setOffline(false)
	r.retry(t, o.ID)
	again := r.piece(t, o.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	asked := r.starter.asked()
	if r.starter.sessions() != 2 || asked[len(asked)-1].Model != "paid/backup" || asked[len(asked)-1].Dir != first.WorkDir || again.SessionID == first.SessionID {
		t.Fatalf("after Try again: %+v sessions=%d", again, r.starter.sessions())
	}
}

// Fallback after a usage limit, controller restarted while the backup's
// launch was in flight: the next controller asks under the same request and
// one backup session runs.
func TestABackupLaunchInFlightAtARestartIsStartedOnce(t *testing.T) {
	r := outcomeFixture(t)
	o, first := usageLimited(t, r)
	r.starter.mu.Lock()
	r.starter.late = "Export API · code"
	r.starter.mu.Unlock()
	if _, err := r.s.RecoverPipelineWorker(r.p.UserID, o.ID, first.Key, first.SessionID, o.Revision, nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for r.starter.sessions() < 2 {
		if time.Now().After(deadline) {
			t.Fatal("the backup launch never reached the machine")
		}
		time.Sleep(10 * time.Millisecond)
	}
	r.restart(t)
	again := r.piece(t, o.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	asked := r.starter.asked()
	if r.starter.sessions() != 2 || len(asked) != 3 || asked[2].RequestID != asked[1].RequestID || asked[2].Model != "paid/backup" || again.Stages[0].Model != "paid/backup" {
		t.Fatalf("after the restart: %+v sessions=%d launches=%d", again, r.starter.sessions(), len(asked))
	}
}

// Fallback by balancing before the start, machine unreachable: the stage is
// recorded on its backup and Try again starts it there once.
func TestABalancedStageThatCannotReachTheMachineStartsOnItsBackupWhenTriedAgain(t *testing.T) {
	r := outcomeFixture(t)
	now := time.Now()
	low := []terminalproto.AgentAccount{planAccount("opencode", now, window("week", 30, ""), window("week", 97, "cheap"))}
	if err := r.st.SaveMachineAccountsFor(r.p.UserID, r.project.Host, low, now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	in := r.in
	in.Sessions = []OutcomeSession{{Title: "Export API", Prompt: "Add the CSV export endpoint.", Stages: []OutcomeStageInput{
		{Kind: "code", Tool: "opencode", Model: "cheap/coder", Backup: &store.GafferStageAgent{Tool: "opencode", Model: "other/coder"}},
	}}}
	r.setOffline(true)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	failed := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "failed" })
	if stage := failed.Stages[0]; stage.Model != "other/coder" || stage.First == nil || stage.Balanced == "" {
		t.Fatalf("the balancing was not recorded: %+v", stage)
	}
	r.setOffline(false)
	r.retry(t, created.ID)
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	asked := r.starter.asked()
	if r.starter.sessions() != 1 || asked[len(asked)-1].Model != "other/coder" {
		t.Fatalf("after Try again: sessions=%d %+v", r.starter.sessions(), asked[len(asked)-1])
	}
}

// lostReply is the machine as the real starter reports a create whose reply
// was lost after it was sent: the session starts, and the controller is told
// only LaunchUnknown (internal/api/gaffer_outcomes.go maps a lost reply and a
// deadline to it).
type lostReply struct {
	*outcomeStarterFake
	lose atomic.Int32
}

func (l *lostReply) StartOutcomeSession(ctx context.Context, uid int64, in OutcomeLaunch) (string, string, error) {
	id, dir, err := l.outcomeStarterFake.StartOutcomeSession(ctx, uid, in)
	if err == nil && l.lose.Add(-1) >= 0 {
		return "", "", LaunchUnknown("That machine is offline.")
	}
	return id, dir, err
}

// Launch, reply lost after the machine started the session: the pipeline
// says it cannot tell, and Try again takes that session up.
func TestALaunchWhoseReplyWasLostIsNotStartedTwiceWhenTriedAgain(t *testing.T) {
	r := outcomeFixture(t)
	machine := &lostReply{outcomeStarterFake: r.starter}
	machine.lose.Store(1)
	r.s.SetSessionStarter(machine)
	r.in.Sessions = r.in.Sessions[:1]
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, r.in)
	if err != nil {
		t.Fatal(err)
	}
	failed := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "failed" })
	if r.starter.sessions() != 1 {
		t.Fatalf("the machine did not start the first session: %d", r.starter.sessions())
	}
	if failed.UnsureRequest != r.starter.asked()[0].RequestID || strings.HasPrefix(failed.Note, "Not started") || !strings.Contains(failed.Note, "cannot tell") {
		t.Fatalf("after the lost reply: unsure=%q note=%q", failed.UnsureRequest, failed.Note)
	}
	r.retry(t, created.ID)
	again := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if n := r.starter.sessions(); n != 1 || again.UnsureRequest != "" {
		t.Fatalf("Try again started a second session beside the one whose reply was lost: %d sessions, %+v", n, again)
	}
}

// Approval, controller restarted while the merge launch was in flight and
// the machine had started it, with the head unreadable afterwards: the
// request recorded before the launch was sent is taken up, so the merge
// session running on the machine is the one the pipeline follows.
func TestAMergeLaunchInFlightAtARestartIsNotForgottenWhenTheHeadCannotBeRead(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, deliveryInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	toReview(t, r, created.ID)
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", decisionFor(r.review(t, created.ID), "accept", 1)); err != nil {
		t.Fatal(err)
	}
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "ready_to_merge" })
	r.starter.mu.Lock()
	r.starter.late = "Export API · merge"
	r.starter.mu.Unlock()
	before := r.starter.sessions()
	r.approveMerge(t, created.ID)
	deadline := time.Now().Add(10 * time.Second)
	for r.starter.sessions() == before {
		if time.Now().After(deadline) {
			t.Fatal("the merge launch never reached the machine")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// The machine drops as the controller restarts.
	r.headUnreadable(true)
	r.restart(t)
	merge := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 2 })
	if r.starter.sessions() != before+1 || !merge.MergeApproved || !merge.DeliveryAccepted || merge.UnsureRequest != "" {
		t.Fatalf("a merge session runs on the machine and the pipeline does not follow it: %+v sessions=%d", merge, r.starter.sessions()-before)
	}
}

// A launch that never reached the machine, or that the machine refused,
// holds nothing back: Try again balances the stage as any start does.
func TestALaunchNeverSentOrRefusedLeavesTheStageToBeBalanced(t *testing.T) {
	r := outcomeFixture(t)
	now := time.Now()
	plenty := []terminalproto.AgentAccount{planAccount("opencode", now, window("week", 30, ""), window("week", 40, "cheap"))}
	if err := r.st.SaveMachineAccountsFor(r.p.UserID, r.project.Host, plenty, now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	in := r.in
	in.Sessions = []OutcomeSession{{Title: "Export API", Prompt: "Add the CSV export endpoint.", Stages: []OutcomeStageInput{
		{Kind: "code", Tool: "opencode", Model: "cheap/coder", Backup: &store.GafferStageAgent{Tool: "opencode", Model: "other/coder"}},
	}}}
	r.starter.refuse = map[string]string{"Export API · code": "That machine could not start the session."}
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	refused := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "failed" })
	if refused.UnsureRequest != "" || !strings.HasPrefix(refused.Note, "Not started") {
		t.Fatalf("after a refusal: unsure=%q note=%q", refused.UnsureRequest, refused.Note)
	}
	r.starter.mu.Lock()
	r.starter.refuse = nil
	r.starter.mu.Unlock()
	r.setOffline(true)
	r.retry(t, created.ID)
	unsent := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "failed" && p.Note != refused.Note })
	if unsent.UnsureRequest != "" || !strings.HasPrefix(unsent.Note, "Not started") || !strings.Contains(unsent.Note, "offline") {
		t.Fatalf("after a launch that was never sent: unsure=%q note=%q", unsent.UnsureRequest, unsent.Note)
	}
	low := []terminalproto.AgentAccount{planAccount("opencode", now, window("week", 30, ""), window("week", 97, "cheap"))}
	if err = r.st.SaveMachineAccountsFor(r.p.UserID, r.project.Host, low, now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	r.setOffline(false)
	r.retry(t, created.ID)
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if asked := r.starter.asked(); r.starter.sessions() != 1 || asked[len(asked)-1].Model != "other/coder" {
		t.Fatalf("the stage was not balanced when tried again: sessions=%d %+v", r.starter.sessions(), asked[len(asked)-1])
	}
}

// A start without the effort that never reached the machine holds nothing
// back either: Try again asks for the effort again.
func TestAStartWithoutTheEffortThatWasNeverSentAsksForTheEffortWhenTriedAgain(t *testing.T) {
	r := outcomeFixture(t)
	r.starter.refusal = func(n int, _ OutcomeLaunch) error {
		switch n {
		case 0:
			return effortRefusal
		case 1:
			return LaunchUnknown("That machine is offline.")
		}
		return nil
	}
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, effortInput(r))
	if err != nil {
		t.Fatal(err)
	}
	failed := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "failed" })
	if failed.UnsureRequest != "" || !strings.HasPrefix(failed.Note, "Not started") || failed.Stages[0].EffortNotOffered != "high" {
		t.Fatalf("after the unsent start: %+v", failed)
	}
	r.retry(t, created.ID)
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if asked := r.starter.asked(); len(asked) != 3 || asked[2].Effort != "high" {
		t.Fatalf("the effort was not asked for again: %+v", asked)
	}
}

// Approval, controller restarted after the merge launch was recorded and
// before it was sent: nothing reached the machine, so the next controller
// reads the head again, and with it unreadable the pipeline goes back to its
// review and starts nothing.
func TestAMergeLaunchRecordedButNeverSentRunsTheMergeGateAgain(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, deliveryInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	toReview(t, r, created.ID)
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", decisionFor(r.review(t, created.ID), "accept", 1)); err != nil {
		t.Fatal(err)
	}
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "ready_to_merge" })
	hold := make(chan struct{})
	t.Cleanup(func() { close(hold) })
	r.starter.mu.Lock()
	r.starter.hold, r.starter.holdOnly, r.starter.entered = hold, "Export API · merge", make(chan struct{})
	r.starter.mu.Unlock()
	launches, before := len(r.starter.asked()), r.starter.sessions()
	r.approveMerge(t, created.ID)
	<-r.starter.entered
	if p := mustOutcome(t, r, created.ID).Sessions[0]; p.UnsureRequest != p.Stages[2].RequestID {
		t.Fatalf("the merge launch was not recorded before it was sent: %+v", p)
	}
	r.headUnreadable(true)
	r.restart(t)
	back := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "delivery_review" })
	if back.DeliveryAccepted || back.MergeApproved || back.UnsureRequest != "" || len(r.starter.asked()) != launches+1 || r.starter.sessions() != before {
		t.Fatalf("back for review: %+v launches=%d", back, len(r.starter.asked())-launches)
	}
}

// sentNotStarted is a machine that received a launch and never started it:
// the create is on record, nothing runs, and asked again it says so, with the
// session recorded as lost as the real starter finds it.
type sentNotStarted struct {
	*outcomeStarterFake
	title string
	sent  sync.Map
}

func (m *sentNotStarted) StartOutcomeSession(ctx context.Context, uid int64, in OutcomeLaunch) (string, string, error) {
	if in.Title != m.title {
		return m.outcomeStarterFake.StartOutcomeSession(ctx, uid, in)
	}
	m.mu.Lock()
	m.launches = append(m.launches, in)
	m.mu.Unlock()
	create := terminalproto.Create{Title: in.Title, Tool: in.Tool, Structured: true, Dir: in.Dir, Prompt: in.Prompt, Cols: 100, Rows: 30}
	row, _, err := m.st.ReserveTerminalCreateFor(uid, in.AgentID, in.RequestID, m.gen, create)
	if err != nil {
		return "", "", err
	}
	if _, again := m.sent.LoadOrStore(in.RequestID, true); again {
		if err = m.st.ObserveTerminalStateFor(uid, in.AgentID, row.ID, m.gen, terminalproto.Lost, nil); err != nil {
			return "", "", err
		}
		return "", "", errors.New("That machine didn't start the agent.")
	}
	return "", "", LaunchUnknown("That machine did not answer.")
}

// Approval, merge launch unanswered and then found never to have started:
// the merge gate it was spared runs before anything else is sent, so with the
// head unreadable the pipeline goes back to its review and starts nothing.
func TestAnUnansweredMergeLaunchFoundNotStartedRunsTheMergeGateAgain(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, deliveryInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	toReview(t, r, created.ID)
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", decisionFor(r.review(t, created.ID), "accept", 1)); err != nil {
		t.Fatal(err)
	}
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "ready_to_merge" })
	r.s.SetSessionStarter(&sentNotStarted{outcomeStarterFake: r.starter, title: "Export API · merge"})
	before := r.starter.sessions()
	r.approveMerge(t, created.ID)
	unsure := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "unfollowed" })
	if unsure.UnsureRequest == "" || strings.Contains(unsure.Note, "was not started") {
		t.Fatalf("after the unanswered merge launch: %+v", unsure)
	}
	launches := len(r.starter.asked())
	r.headUnreadable(true)
	r.retry(t, created.ID)
	back := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "delivery_review" })
	asked := r.starter.asked()
	if back.DeliveryAccepted || back.MergeApproved || back.UnsureRequest != "" || len(asked) != launches+1 || asked[launches].RequestID != unsure.UnsureRequest || r.starter.sessions() != before {
		t.Fatalf("back for review: %+v launches=%d", back, len(asked)-launches)
	}
}

// endFails is the machine unreachable for an end request only. tries counts
// the end requests it was sent.
type endFails struct {
	*outcomeStarterFake
	tries atomic.Int32
}

func (m *endFails) EndOutcomeSession(context.Context, int64, string) error {
	m.tries.Add(1)
	return errors.New("That machine is offline.")
}

// endLost is a machine that ends a session and loses the reply.
type endLost struct{ *outcomeStarterFake }

func (m endLost) EndOutcomeSession(ctx context.Context, uid int64, session string) error {
	if err := m.outcomeStarterFake.EndOutcomeSession(ctx, uid, session); err != nil {
		return err
	}
	return errors.New("That machine is offline.")
}

// stoppedLow runs a one-stage pipeline with a backup until its agent has
// stopped with its plan now low, as balancing finds it.
func stoppedLow(t *testing.T, r *outcomeRig) (string, store.GafferOutcomeSession) {
	t.Helper()
	now := time.Now()
	plenty := []terminalproto.AgentAccount{planAccount("opencode", now, window("week", 30, ""), window("week", 40, "cheap"))}
	if err := r.st.SaveMachineAccountsFor(r.p.UserID, r.project.Host, plenty, now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	in := r.in
	in.Sessions = []OutcomeSession{{Title: "Export API", Prompt: "Add the CSV export endpoint.", Stages: []OutcomeStageInput{
		{Kind: "code", Tool: "opencode", Model: "cheap/coder", Backup: &store.GafferStageAgent{Tool: "opencode", Model: "other/coder"}},
	}}}
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	first := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	r.settle(t, created.ID)
	low := []terminalproto.AgentAccount{planAccount("opencode", now, window("week", 30, ""), window("week", 97, "cheap"))}
	if err = r.st.SaveMachineAccountsFor(r.p.UserID, r.project.Host, low, now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	goal, err := r.st.GafferGoalFor(r.p.UserID, first.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.st.AppendTerminalChatEventFor(r.p.UserID, first.SessionID, 1, time.Now().UnixMilli(), []byte(`{"kind":"status","state":"error","detail":"The agent crashed."}`)); err != nil {
		t.Fatal(err)
	}
	r.s.reviewGoal(t.Context(), goal)
	return created.ID, first
}

// Fallback by balancing after the agent stopped, machine unreachable to end
// its session: the stage is not marked as run again, and once the machine
// answers it is run on its backup.
func TestAStoppedStageWhoseSessionCouldNotBeEndedIsStillRunOnItsBackup(t *testing.T) {
	r := outcomeFixture(t)
	id, first := stoppedLow(t, r)
	r.s.SetSessionStarter(&endFails{outcomeStarterFake: r.starter})
	r.s.rebalanceStopped(context.Background())
	if mid := mustOutcome(t, r, id).Sessions[0]; mid.State != "following" || mid.Stages[0].Rebalanced || mid.SessionID != first.SessionID {
		t.Fatalf("after the failed end: state=%s rebalanced=%v session=%s", mid.State, mid.Stages[0].Rebalanced, mid.SessionID)
	}
	// The machine is back, and the wait before asking it again is over.
	r.s.SetSessionStarter(r.starter)
	r.s.endTried.Store(first.SessionID, time.Now().Add(-endRetryAfter))
	r.s.rebalanceStopped(context.Background())
	again := r.piece(t, id, func(p store.GafferOutcomeSession) bool {
		return p.State == "following" && p.SessionID != first.SessionID
	})
	if r.starter.sessions() != 2 || again.Stages[0].Model != "other/coder" || !again.Stages[0].Rebalanced {
		t.Fatalf("the stage was not run on its backup: %+v sessions=%d", again.Stages[0], r.starter.sessions())
	}
}

// A machine that cannot end the session is not asked again on every pass of
// the controller: each ask can hold it up for its whole wait.
func TestAStoppedStageWhoseSessionCouldNotBeEndedIsNotAskedAgainAtOnce(t *testing.T) {
	r := outcomeFixture(t)
	id, first := stoppedLow(t, r)
	machine := &endFails{outcomeStarterFake: r.starter}
	r.s.SetSessionStarter(machine)
	for i := 0; i < 3; i++ {
		r.s.rebalanceStopped(context.Background())
	}
	if n := machine.tries.Load(); n != 1 {
		t.Fatalf("end requests while the machine could not end the session: %d", n)
	}
	r.s.endTried.Store(first.SessionID, time.Now().Add(-endRetryAfter))
	r.s.rebalanceStopped(context.Background())
	if n := machine.tries.Load(); n != 2 || mustOutcome(t, r, id).Sessions[0].Stages[0].Rebalanced {
		t.Fatalf("end requests once the wait was over: %d", n)
	}
}

// Fallback by balancing, controller stopped after the session was ended and
// before the stage was run again: the next controller runs it on its backup.
func TestAStoppedStageEndedJustBeforeARestartIsRunOnItsBackup(t *testing.T) {
	r := outcomeFixture(t)
	id, first := stoppedLow(t, r)
	// The machine ends the session and its answer is lost.
	r.s.SetSessionStarter(endLost{r.starter})
	r.s.rebalanceStopped(context.Background())
	if mid := mustOutcome(t, r, id).Sessions[0]; mid.Stages[0].Rebalanced || mid.SessionID != first.SessionID || r.s.sessionRunning(r.p.UserID, first.SessionID) {
		t.Fatalf("before the restart: %+v", mid)
	}
	r.restart(t)
	r.s.rebalanceStopped(context.Background())
	again := r.piece(t, id, func(p store.GafferOutcomeSession) bool {
		return p.State == "following" && p.SessionID != first.SessionID
	})
	if r.starter.sessions() != 2 || again.Stages[0].Model != "other/coder" || !again.Stages[0].Rebalanced {
		t.Fatalf("after the restart: %+v sessions=%d", again.Stages[0], r.starter.sessions())
	}
}

// Fallback after a usage limit, machine unreachable to end the exhausted
// session at the switch: once the machine answers again, that session is
// ended beside the backup the pipeline now follows.
func TestTheExhaustedSessionIsEndedWhenItsMachineCouldNotBeReachedAtTheSwitch(t *testing.T) {
	r := outcomeFixture(t)
	o, first := usageLimited(t, r)
	r.s.SetSessionStarter(&endFails{outcomeStarterFake: r.starter})
	if _, err := r.s.RecoverPipelineWorker(r.p.UserID, o.ID, first.Key, first.SessionID, o.Revision, nil); err != nil {
		t.Fatal(err)
	}
	r.piece(t, o.ID, func(p store.GafferOutcomeSession) bool {
		return p.State == "following" && p.SessionID != first.SessionID
	})
	if !r.s.sessionRunning(r.p.UserID, first.SessionID) {
		t.Fatal("the exhausted session was ended while its machine could not be reached")
	}
	// The machine is back, and any wait before asking it again is over.
	r.s.SetSessionStarter(r.starter)
	r.s.endTried.Delete(first.SessionID)
	deadline := time.Now().Add(15 * time.Second)
	for r.s.sessionRunning(r.p.UserID, first.SessionID) {
		if time.Now().After(deadline) {
			t.Fatal("the exhausted session is still running beside its backup")
		}
		r.s.wakeOutcomes()
		time.Sleep(20 * time.Millisecond)
	}
	if n := len(r.starter.asked()); n != 2 {
		t.Fatalf("launches asked for while the session was ended: %d, want 2", n)
	}
}

// A session left behind is ended once its machine answers even when its
// pipeline has moved on past following meanwhile: to its review, its merge
// or its end.
func TestASessionLeftBehindIsEndedAfterItsPipelineHasFinished(t *testing.T) {
	r := outcomeFixture(t)
	o, first := usageLimited(t, r)
	r.s.SetSessionStarter(&endFails{outcomeStarterFake: r.starter})
	if _, err := r.s.RecoverPipelineWorker(r.p.UserID, o.ID, first.Key, first.SessionID, o.Revision, nil); err != nil {
		t.Fatal(err)
	}
	r.piece(t, o.ID, func(p store.GafferOutcomeSession) bool {
		return p.State == "following" && p.SessionID != first.SessionID
	})
	r.settle(t, o.ID)
	if _, err := r.st.UpdateGafferOutcomeFor(r.p.UserID, o.ID, mustOutcome(t, r, o.ID).Revision, func(o *store.GafferOutcome) bool {
		o.Sessions[0].State = "done"
		return true
	}); err != nil {
		t.Fatal(err)
	}
	r.s.SetSessionStarter(r.starter)
	r.s.endTried.Delete(first.SessionID)
	r.s.endLeftBehind()
	if r.s.sessionRunning(r.p.UserID, first.SessionID) {
		t.Fatal("the session left behind runs on after its pipeline finished")
	}
}

// A session another pipeline works in is never ended for being left behind.
func TestALeftBehindSessionAnotherPipelineUsesIsNotEnded(t *testing.T) {
	r := outcomeFixture(t)
	o, first := usageLimited(t, r)
	r.s.SetSessionStarter(&endFails{outcomeStarterFake: r.starter})
	if _, err := r.s.RecoverPipelineWorker(r.p.UserID, o.ID, first.Key, first.SessionID, o.Revision, nil); err != nil {
		t.Fatal(err)
	}
	r.piece(t, o.ID, func(p store.GafferOutcomeSession) bool {
		return p.State == "following" && p.SessionID != first.SessionID
	})
	r.settle(t, o.ID)
	// Another outcome follows the exhausted session.
	other, err := r.st.CreateGafferOutcomeFor(r.p.UserID, store.GafferOutcome{ID: strings.Repeat("e", 32), Objective: "Ship it.", Sessions: []store.GafferOutcomeSession{{Key: "a", Title: "Part", Prompt: "Do it. Do not merge.", RequestID: strings.Repeat("f", 32)}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.st.UpdateGafferOutcomeFor(r.p.UserID, other.ID, other.Revision, func(o *store.GafferOutcome) bool {
		o.State, o.Sessions[0].State, o.Sessions[0].SessionID = "settled", "following", first.SessionID
		return true
	}); err != nil {
		t.Fatal(err)
	}
	r.s.SetSessionStarter(r.starter)
	r.s.endTried.Delete(first.SessionID)
	r.s.endLeftBehind()
	if !r.s.sessionRunning(r.p.UserID, first.SessionID) {
		t.Fatal("a session another pipeline follows was ended")
	}
	// The same pass ends it once nothing else follows it, so the pass above
	// did look at it.
	if _, err = r.st.UpdateGafferOutcomeFor(r.p.UserID, other.ID, mustOutcome(t, r, other.ID).Revision, func(o *store.GafferOutcome) bool {
		o.Sessions[0].State = "done"
		return true
	}); err != nil {
		t.Fatal(err)
	}
	r.s.endTried.Delete(first.SessionID)
	r.s.endLeftBehind()
	if r.s.sessionRunning(r.p.UserID, first.SessionID) {
		t.Fatal("the session left behind was not ended once nothing else followed it")
	}
}

// endIgnored answers an end as done and leaves the session running.
type endIgnored struct {
	*outcomeStarterFake
	tries atomic.Int32
}

func (m *endIgnored) EndOutcomeSession(context.Context, int64, string) error {
	m.tries.Add(1)
	return nil
}

// endRecorded is the real ender while the machine cannot be reached: the end
// is recorded, to be sent when the machine answers, and the call succeeds.
type endRecorded struct{ *outcomeStarterFake }

func (m endRecorded) EndOutcomeSession(_ context.Context, uid int64, session string) error {
	_, err := m.st.RequestTerminalEndFor(uid, m.agent, session, session[:32])
	return err
}

// A session still running after its end was answered was not ended: the
// stage is not run again beside it, and the machine is not asked again at once.
func TestAStoppedStageWhoseSessionStillRunsAfterItsEndIsNotRunOnItsBackup(t *testing.T) {
	r := outcomeFixture(t)
	id, first := stoppedLow(t, r)
	machine := &endIgnored{outcomeStarterFake: r.starter}
	r.s.SetSessionStarter(machine)
	for i := 0; i < 3; i++ {
		r.s.rebalanceStopped(context.Background())
	}
	if mid := mustOutcome(t, r, id).Sessions[0]; machine.tries.Load() != 1 || mid.Stages[0].Rebalanced || mid.SessionID != first.SessionID {
		t.Fatalf("after an end that left the session running: tries=%d %+v", machine.tries.Load(), mid)
	}
}

// An end the machine has yet to carry out counts as done: the stage is run on
// its backup.
func TestAStoppedStageWhoseEndIsRecordedIsRunOnItsBackup(t *testing.T) {
	r := outcomeFixture(t)
	id, first := stoppedLow(t, r)
	r.s.SetSessionStarter(endRecorded{r.starter})
	r.s.rebalanceStopped(context.Background())
	again := r.piece(t, id, func(p store.GafferOutcomeSession) bool {
		return p.State == "following" && p.SessionID != first.SessionID
	})
	if again.Stages[0].Model != "other/coder" || !again.Stages[0].Rebalanced {
		t.Fatalf("the stage was not run on its backup: %+v", again.Stages[0])
	}
}

// Launch, reply lost, then the person changes what the launch asks for
// (here YOLO) before Try again: the request asked again no longer matches the
// machine's record of it, and the session it started is taken up all the same
// instead of a second one being started beside it.
func TestALaunchWhoseReplyWasLostIsTakenUpAfterTheLaunchWasChanged(t *testing.T) {
	r := outcomeFixture(t)
	machine := &lostReplyYolo{lostReply: &lostReply{outcomeStarterFake: r.starter}}
	machine.lose.Store(1)
	r.s.SetSessionStarter(machine)
	r.in.Sessions = r.in.Sessions[:1]
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, r.in)
	if err != nil {
		t.Fatal(err)
	}
	failed := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "failed" })
	op, err := r.st.TerminalOperationFor(r.p.UserID, r.starter.agent, "create", failed.UnsureRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = r.s.SetOutcomeYolo(context.Background(), r.p.UserID, created.ID, r.settle(t, created.ID).Revision, true); err != nil {
		t.Fatal(err)
	}
	r.retry(t, created.ID)
	again := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool {
		return p.State == "following" || p.State == "failed" && p.Note != failed.Note
	})
	if n := r.starter.sessions(); n != 1 || again.SessionID != op.SessionID || again.UnsureRequest != "" {
		t.Fatalf("Try again after the change did not take up the session already started: %d sessions, %+v", n, again)
	}
	// It was started off YOLO, so it is switched to what the outcome asks now.
	if on, ok := machine.sets.Load(op.SessionID); !ok || !on.(bool) {
		t.Fatalf("the session taken up was not put on YOLO: %v %v", on, ok)
	}
}

// lostReplyYolo is lostReply on a machine that can switch a session's approvals.
type lostReplyYolo struct {
	*lostReply
	sets sync.Map
}

func (m *lostReplyYolo) SetOutcomeSessionYolo(_ context.Context, _ int64, session string, on bool) error {
	m.sets.Store(session, on)
	return nil
}

// Send back, while the next stage's launch was not answered: the person
// gives that launch up, so the session it started is ended rather than left
// running with nothing following it, and the stage done again starts its own.
func TestASessionStartedByAnUnansweredLaunchIsEndedWhenThePipelineIsSentBack(t *testing.T) {
	wait := outcomeStartWait
	outcomeStartWait = 150 * time.Millisecond
	t.Cleanup(func() { outcomeStartWait = wait })
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	r.starter.mu.Lock()
	r.starter.late = code.Title + " · review"
	r.starter.mu.Unlock()
	r.s.stageHandedOver(r.p.UserID, code.ConversationID, "Endpoint and tests are committed.", "gaffer")
	stopped := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "unfollowed" })
	if stopped.UnsureRequest == "" || !strings.HasPrefix(stopped.Note, "The review stage") || !strings.Contains(stopped.Note, "cannot tell whether") {
		t.Fatalf("after the unanswered review launch: unsure=%q note=%q", stopped.UnsureRequest, stopped.Note)
	}
	op, err := r.st.TerminalOperationFor(r.p.UserID, r.starter.agent, "create", stopped.UnsureRequest)
	if err != nil {
		t.Fatal(err)
	}
	outcomeStartWait = wait
	if _, err = r.s.SendPipelineBack(r.p.UserID, created.ID, "a", "code", "Redo it please."); err != nil {
		t.Fatal(err)
	}
	again := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 0 })
	if again.UnsureRequest != "" || again.SessionID == op.SessionID || r.s.sessionRunning(r.p.UserID, op.SessionID) {
		t.Fatalf("the review session the unanswered launch started runs on beside the code stage done again: %+v", again)
	}
}

// refusedOnRecord is a machine that receives a launch, holds it until
// released, and then refuses it: the create is on record as failed and
// nothing runs.
type refusedOnRecord struct {
	*outcomeStarterFake
	entered, release chan struct{}
}

func (m *refusedOnRecord) StartOutcomeSession(ctx context.Context, uid int64, in OutcomeLaunch) (string, string, error) {
	m.mu.Lock()
	m.launches = append(m.launches, in)
	m.mu.Unlock()
	create := terminalproto.Create{Title: in.Title, Tool: in.Tool, Structured: true, Dir: in.Dir, Prompt: in.Prompt, Cols: 100, Rows: 30}
	if _, _, err := m.st.ReserveTerminalCreateFor(uid, in.AgentID, in.RequestID, m.gen, create); err != nil {
		return "", "", err
	}
	close(m.entered)
	<-m.release
	if err := m.st.FailTerminalCreateFor(uid, in.AgentID, in.RequestID, m.gen, terminalproto.Capacity); err != nil {
		return "", "", err
	}
	return "", "", errors.New("That machine could not start the session.")
}

// A launch the machine refused after the person stopped the outcome is known
// to have started nothing: it is not kept as unanswered, and Try again on a
// machine that cannot be reached says plainly that nothing started.
func TestALaunchRefusedAfterTheOutcomeWasStoppedIsNotKeptAsUnanswered(t *testing.T) {
	r := outcomeFixture(t)
	machine := &refusedOnRecord{outcomeStarterFake: r.starter, entered: make(chan struct{}), release: make(chan struct{})}
	r.s.SetSessionStarter(machine)
	r.in.Sessions = r.in.Sessions[:1]
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, r.in)
	if err != nil {
		t.Fatal(err)
	}
	<-machine.entered
	if _, err = r.s.StopFollowingOutcome(r.p.UserID, created.ID, mustOutcome(t, r, created.ID).Revision); err != nil {
		t.Fatal(err)
	}
	close(machine.release)
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "cancelled" && p.UnsureRequest == "" })
	r.s.SetSessionStarter(r.starter)
	r.setOffline(true)
	r.retry(t, created.ID)
	failed := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "failed" })
	if failed.UnsureRequest != "" || !strings.HasPrefix(failed.Note, "Not started") || strings.Contains(failed.Note, "earlier") {
		t.Fatalf("after Try again on an unreachable machine: unsure=%q note=%q", failed.UnsureRequest, failed.Note)
	}
}

// Launch in flight at a restart while the account cannot start sessions: the
// request that reached the machine is kept, so once the account may start
// sessions again Try again takes that session up rather than starting another.
func TestALaunchInFlightAtARestartIsKeptWhileTheAccountCannotStartSessions(t *testing.T) {
	r := outcomeFixture(t)
	r.starter.late = "Export API"
	r.in.Sessions = r.in.Sessions[:1]
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, r.in)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for r.starter.sessions() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the launch never reached the machine")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err = r.st.UpdateUserRole(r.p.UserID, "viewer"); err != nil {
		t.Fatal(err)
	}
	r.restart(t)
	refused := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "failed" })
	if refused.UnsureRequest == "" {
		t.Fatalf("the request that reached the machine was forgotten: %+v", refused)
	}
	if err = r.st.UpdateUserRole(r.p.UserID, "owner"); err != nil {
		t.Fatal(err)
	}
	r.retry(t, created.ID)
	again := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if n := r.starter.sessions(); n != 1 || again.UnsureRequest != "" {
		t.Fatalf("Try again started a second session: %d sessions, %+v", n, again)
	}
}
