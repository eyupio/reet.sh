package gaffer

import (
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// mergeWaits fails when the pipeline's merge stage is started, and returns
// once the pipeline has sat at Ready to merge for a moment.
func (r *outcomeRig) mergeWaits(t *testing.T, id string, launches int) store.GafferOutcomeSession {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		o, err := r.st.GafferOutcomeFor(r.p.UserID, id)
		if err != nil {
			t.Fatal(err)
		}
		piece := o.Sessions[0]
		if piece.State == "following" || piece.Stage < len(piece.Stages) && piece.Stages[piece.Stage].Kind == "merge" && piece.Stages[piece.Stage].State == "running" {
			t.Fatalf("the merge stage started without an approval: %+v", piece)
		}
		if piece.State == "ready_to_merge" && o.State != "starting" {
			time.Sleep(60 * time.Millisecond)
			if got := len(r.starter.asked()); got != launches {
				t.Fatalf("launches while waiting for the merge decision: %d, want %d", got, launches)
			}
			return piece
		}
		if time.Now().After(deadline) {
			t.Fatalf("pipeline never waited for its merge: %+v", piece)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Stop following and then Try again must not be a way round the merge gate: a
// pipeline that was waiting for the person's approval waits for it again.
func TestRetryingAPipelineThatWaitsForItsMergeDoesNotStartTheMerge(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	in := pipelineInput(r, false)
	in.Sessions[0].Stages = []OutcomeStageInput{{Kind: "code", Tool: "opencode"}, {Kind: "merge", Tool: "opencode"}}
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	r.s.stageHandedOver(r.p.UserID, code.ConversationID, "Done.", "gaffer")
	ready := r.mergeWaits(t, created.ID, 1)
	o := mustOutcome(t, r, created.ID)
	if o, err = r.s.StopFollowingOutcome(r.p.UserID, created.ID, o.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = r.s.RetryPipeline(r.p.UserID, created.ID, ready.Key, o.Revision); err != nil {
		t.Fatal(err)
	}
	again := r.mergeWaits(t, created.ID, 1)
	if again.MergeApproved || again.Stages[1].State != "pending" {
		t.Fatalf("after the retry: %+v", again)
	}
	if g, err := r.st.GafferGoalFor(r.p.UserID, code.ConversationID); err == nil && g.State == "active" && strings.Contains(g.Text, "\nStage: merge\n") {
		t.Fatalf("a merge goal was set without an approval: %q", g.Text)
	}
	// The person's approval still starts it.
	if _, err = r.s.ApproveMerge(r.p.UserID, created.ID, ready.Key, mustOutcome(t, r, created.ID).Revision); err != nil {
		t.Fatal(err)
	}
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 1 })
}

// A pipeline whose only or first stage is the merge is held at the gate like
// any other: nothing is launched until the person approves.
func TestAPipelineThatBeginsWithItsMergeWaitsForApproval(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	in := pipelineInput(r, false)
	in.Sessions[0].Stages = []OutcomeStageInput{{Kind: "merge", Tool: "opencode"}}
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	ready := r.mergeWaits(t, created.ID, 0)
	if _, err = r.s.ApproveMerge(r.p.UserID, created.ID, ready.Key, mustOutcome(t, r, created.ID).Revision); err != nil {
		t.Fatal(err)
	}
	merge := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if merge.Stage != 0 || len(r.starter.asked()) != 1 {
		t.Fatalf("approved merge: %+v launches %d", merge, len(r.starter.asked()))
	}
}

// The person stops following while the controller is still starting a
// session. When the launch answers, the stop stands: the session is kept on
// the record so it can be found, and nothing follows it.
func TestStoppingFollowingDuringALaunchIsNotUndoneWhenTheLaunchReturns(t *testing.T) {
	r := outcomeFixture(t)
	r.starter.hold, r.starter.entered = make(chan struct{}), make(chan struct{})
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, r.in)
	if err != nil {
		t.Fatal(err)
	}
	<-r.starter.entered
	if _, err = r.s.StopFollowingOutcome(r.p.UserID, created.ID, mustOutcome(t, r, created.ID).Revision); err != nil {
		t.Fatal(err)
	}
	close(r.starter.hold)
	left := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.SessionID != "" && p.State != "starting" })
	time.Sleep(100 * time.Millisecond)
	o := mustOutcome(t, r, created.ID)
	if left.State != "unfollowed" || o.Sessions[0].State != "unfollowed" || o.State != "stopped" || o.Sessions[0].ConversationID != "" {
		t.Fatalf("the stop was undone: outcome %s piece %+v", o.State, o.Sessions[0])
	}
	if len(r.starter.asked()) != 1 {
		t.Fatalf("launches after the stop: %d", len(r.starter.asked()))
	}
}

// An archived outcome is put away. Nothing sets it going again until the
// person brings it back.
func TestAnArchivedOutcomeIsNotSetGoingAgain(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	r.settle(t, created.ID)
	o, err := r.s.StopFollowingOutcome(r.p.UserID, created.ID, mustOutcome(t, r, created.ID).Revision)
	if err != nil {
		t.Fatal(err)
	}
	if o, err = r.s.ArchiveOutcome(r.p.UserID, created.ID, o.Revision, true); err != nil {
		t.Fatal(err)
	}
	if _, err = r.s.RetryPipeline(r.p.UserID, created.ID, code.Key, o.Revision); err == nil {
		t.Error("an archived outcome's pipeline was tried again")
	}
	if _, err = r.s.SendPipelineBack(r.p.UserID, created.ID, code.Key, "code", "Do it again."); err == nil {
		t.Error("an archived outcome's pipeline was sent back")
	}
	if _, err = r.s.AddPipelines(r.p.UserID, created.ID, AddPipelinesInput{Confirmed: true, Sessions: []OutcomeSession{{Title: "More", Prompt: "Do more. Do not merge."}}}); err == nil {
		t.Error("a pipeline was added to an archived outcome")
	}
	after := mustOutcome(t, r, created.ID)
	if after.ArchivedAt == 0 || after.State == "starting" || len(after.Sessions) != 1 || after.Sessions[0].State != "unfollowed" {
		t.Fatalf("the archived outcome changed: %+v", after)
	}
	// Brought back, it can be tried again.
	if o, err = r.s.ArchiveOutcome(r.p.UserID, created.ID, after.Revision, false); err != nil {
		t.Fatal(err)
	}
	if _, err = r.s.RetryPipeline(r.p.UserID, created.ID, code.Key, o.Revision); err != nil {
		t.Fatalf("retry after bringing the outcome back: %v", err)
	}
}

// A pipeline that can still be tried again works in its folder, whatever
// stopped it: only a finished pipeline or an archived outcome lets go of it.
func TestAPipelineThatCanBeTriedAgainKeepsItsFolder(t *testing.T) {
	r := outcomeFixture(t)
	uid := r.p.UserID
	o := r.followedOutcome(t, "c", "", "failed")
	for _, state := range []string{"failed", "waiting", "cancelled", "unfollowed", "starting"} {
		var err error
		if o, err = r.st.UpdateGafferOutcomeFor(uid, o.ID, o.Revision, func(o *store.GafferOutcome) bool {
			o.Sessions[0].State, o.Sessions[0].WorkDir = state, "/work/billing/.reet/worktrees/abc"
			return true
		}); err != nil {
			t.Fatal(err)
		}
		if !r.s.WorkDirInUse(uid, "/work/billing/.reet/worktrees/abc") {
			t.Errorf("a %s pipeline's folder was reported free", state)
		}
	}
	o, err := r.st.UpdateGafferOutcomeFor(uid, o.ID, o.Revision, func(o *store.GafferOutcome) bool {
		o.Sessions[0].State = "done"
		return true
	})
	if err != nil || r.s.WorkDirInUse(uid, "/work/billing/.reet/worktrees/abc") {
		t.Fatalf("a finished pipeline still holds its folder: %v", err)
	}
}

// The controller stopped after it gave a stage's session its goal and before
// it recorded that the pipeline is followed. Picking the pipeline up again
// finds its own goal there and carries on, instead of calling the pipeline
// unfollowed because "this session already has a goal".
func TestAStageGoalSetJustBeforeARestartIsAdopted(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	o := r.settle(t, created.ID)
	if _, err = r.st.UpdateGafferOutcomeFor(r.p.UserID, created.ID, o.Revision, func(o *store.GafferOutcome) bool {
		o.State = "starting"
		o.Sessions[0].State, o.Sessions[0].ConversationID, o.Sessions[0].Stages[0].ConversationID = "starting", "", ""
		return true
	}); err != nil {
		t.Fatal(err)
	}
	r.s.wakeOutcomes()
	again := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State != "starting" })
	if again.State != "following" || again.ConversationID != code.ConversationID || again.SessionID != code.SessionID {
		t.Fatalf("the stage's own goal was not adopted: %+v", again)
	}
	if g, err := r.st.GafferGoalFor(r.p.UserID, code.ConversationID); err != nil || g.State != "active" {
		t.Fatalf("goal: %+v %v", g, err)
	}
}

// The next stage could not be started in a session of its own. The session
// the stage before worked in is finished with all the same, and is ended.
func TestTheLastStagesSessionIsEndedWhenTheNextStageCannotStart(t *testing.T) {
	r := outcomeFixture(t)
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
	stuck := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "unfollowed" })
	if stuck.Stage != 1 || stuck.SessionID != "" {
		t.Fatalf("review not started: %+v", stuck)
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(r.starter.closed()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the code stage's session was left running with nothing referring to it")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := r.starter.closed(); got[0] != code.SessionID {
		t.Fatalf("ended %v, want %s", got, code.SessionID)
	}
}
