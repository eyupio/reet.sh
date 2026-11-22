package gaffer

import (
	"context"
	"errors"
	"strings"
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
