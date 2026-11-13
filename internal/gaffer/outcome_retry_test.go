package gaffer

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// A pipeline released because its session was removed says "Retry it to start
// a new session". The retry must do that: the stage in hand gets a session of
// its own in the pipeline's folder, not another attempt to follow the one
// that no longer exists.
func TestRetryingAPipelineWhoseSessionIsGoneStartsItsStageInANewSession(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	r.endSession(t, code.SessionID, true)
	gone := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "unfollowed" })
	// The controller writes once more after a pipeline is followed, to settle the outcome: the revision is read after that.
	r.settle(t, created.ID)
	o, err := r.s.Outcome(r.p.UserID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	before := len(r.starter.asked())
	if _, err := r.s.RetryPipeline(r.p.UserID, created.ID, gone.Key, o.Revision); err != nil {
		t.Fatal(err)
	}
	again := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	asked := r.starter.asked()
	if len(asked) != before+1 {
		t.Fatalf("launches after the retry: %d, was %d (piece %+v)", len(asked), before, again)
	}
	last := asked[len(asked)-1]
	if again.SessionID == code.SessionID || again.Stage != 0 || last.Dir != code.WorkDir || last.ProjectID != "" || last.RequestID == code.Stages[0].RequestID {
		t.Fatalf("stage in a new session: launch %+v piece %+v", last, again)
	}
	if g, err := r.st.GafferGoalFor(r.p.UserID, again.ConversationID); err != nil || g.State != "active" {
		t.Fatalf("goal of the new session: %+v %v", g, err)
	}
}

// A session ended by the person (its history kept) cannot be followed either:
// the retry gives the stage a new session rather than failing on the old one.
func TestRetryingAPipelineWhoseSessionEndedStartsItsStageInANewSession(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	r.endSession(t, code.SessionID, false)
	// The controller writes once more after a pipeline is followed, to settle the outcome: the revision is read after that.
	r.settle(t, created.ID)
	o, err := r.s.Outcome(r.p.UserID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if o, err = r.s.StopFollowingOutcome(r.p.UserID, created.ID, o.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.RetryPipeline(r.p.UserID, created.ID, code.Key, o.Revision); err != nil {
		t.Fatal(err)
	}
	again := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if again.SessionID == code.SessionID || again.SessionID == "" {
		t.Fatalf("an ended session was followed again: %+v", again)
	}
}

// Stop following closes the pipeline's goal. Trying the pipeline again while
// its session still runs follows that same session under a fresh goal.
func TestRetryingAfterStopFollowingFollowsTheRunningSessionAgain(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	// The controller writes once more after a pipeline is followed, to settle the outcome: the revision is read after that.
	r.settle(t, created.ID)
	o, err := r.s.Outcome(r.p.UserID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if o, err = r.s.StopFollowingOutcome(r.p.UserID, created.ID, o.Revision); err != nil {
		t.Fatal(err)
	}
	before := len(r.starter.asked())
	if _, err := r.s.RetryPipeline(r.p.UserID, created.ID, code.Key, o.Revision); err != nil {
		t.Fatal(err)
	}
	again := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if again.SessionID != code.SessionID || len(r.starter.asked()) != before {
		t.Fatalf("the running session must be kept: %+v launches %d was %d", again, len(r.starter.asked()), before)
	}
	if g, err := r.st.GafferGoalFor(r.p.UserID, again.ConversationID); err != nil || g.State != "active" {
		t.Fatalf("goal after the retry: %+v %v", g, err)
	}
}

// A pipeline the page shows as "Not started: this outcome was stopped first"
// can be tried again, and trying one again starts only that one.
func TestRetryingOnePipelineOfAStoppedOutcomeStartsOnlyThatOne(t *testing.T) {
	r := outcomeFixture(t)
	r.starter.hold, r.starter.entered = make(chan struct{}), make(chan struct{})
	r.in.Sessions = append(r.in.Sessions, OutcomeSession{Title: "Export docs", Prompt: "Document the export. Do not merge."})
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, r.in)
	if err != nil {
		t.Fatal(err)
	}
	<-r.starter.entered
	current, err := r.s.Outcome(r.p.UserID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.StopOutcome(r.p.UserID, created.ID, current.Revision); err != nil {
		t.Fatal(err)
	}
	close(r.starter.hold)
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.Key == "a" && p.State == "following" })
	o, err := r.s.Outcome(r.p.UserID, created.ID)
	if err != nil || o.Sessions[1].State != "cancelled" || o.Sessions[2].State != "cancelled" {
		t.Fatalf("stopped outcome: %+v %v", o, err)
	}
	if _, err := r.s.RetryPipeline(r.p.UserID, created.ID, "b", o.Revision); err != nil {
		t.Fatalf("a pipeline shown as not started must be retryable: %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		now, err := r.st.GafferOutcomeFor(r.p.UserID, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if now.Sessions[1].State == "following" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the retried pipeline never started: %+v", now.Sessions[1])
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	o, err = r.s.Outcome(r.p.UserID, created.ID)
	if err != nil || o.Sessions[2].State != "cancelled" || o.Sessions[2].SessionID != "" || len(r.starter.asked()) != 2 {
		t.Fatalf("only the retried pipeline may start: %+v launches %d %v", o.Sessions, len(r.starter.asked()), err)
	}
}

// A stage done again runs on the billing source the person approved for it.
func TestSendingAClineStageBackKeepsItsBillingSource(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	uid := r.p.UserID
	o := r.followedOutcome(t, "1", "", "unfollowed")
	o, err := r.st.UpdateGafferOutcomeFor(uid, o.ID, o.Revision, func(o *store.GafferOutcome) bool {
		o.State = "stopped"
		o.Sessions[0].Stages = []store.GafferOutcomeStage{
			{Kind: "code", Tool: "cline", Model: "cline-pass/deepseek-v4", Provider: "cline-pass", State: "running", RequestID: strings.Repeat("6", 32)},
			{Kind: "review", Tool: "cline", Model: "cline-pass/deepseek-v4", Provider: "cline-pass", State: "pending", RequestID: strings.Repeat("7", 32)},
		}
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	back, err := r.s.SendPipelineBack(uid, o.ID, "a", "code", "Start over.")
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range back.Sessions[0].Stages {
		if stage.Provider != "cline-pass" || stage.Model != "cline-pass/deepseek-v4" {
			t.Fatalf("the source was lost when the stage was sent back: %+v", back.Sessions[0].Stages)
		}
	}
}

// The person ended the session of the stage in hand and kept its history.
// The pipeline is still theirs to decide about: trying it again runs the
// stage in a new session in the same folder, and the old goal is closed.
func TestAPipelineWhoseSessionThePersonEndedIsTriedAgainInANewSession(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	// The controller writes once more after a pipeline is followed, to settle the outcome: the revision is read after that.
	r.settle(t, created.ID)
	o, err := r.s.Outcome(r.p.UserID, created.ID)
	if err != nil || o.Sessions[0].SessionState != "running" {
		t.Fatalf("a live session reads as running: %+v %v", o.Sessions[0], err)
	}
	if _, err := r.s.RetryPipeline(r.p.UserID, created.ID, code.Key, o.Revision); err == nil {
		t.Fatal("a pipeline whose session still runs was tried again")
	}
	r.endSession(t, code.SessionID, false)
	// The controller writes once more after a pipeline is followed, to settle the outcome: the revision is read after that.
	r.settle(t, created.ID)
	o, err = r.s.Outcome(r.p.UserID, created.ID)
	if err != nil || o.Sessions[0].State != "following" || o.Sessions[0].SessionState != "ended" {
		t.Fatalf("an ended session is the person's to decide about, and reads as ended: %+v %v", o.Sessions[0], err)
	}
	if _, err := r.s.RetryPipeline(r.p.UserID, created.ID, code.Key, o.Revision); err != nil {
		t.Fatal(err)
	}
	again := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool {
		return p.State == "following" && p.SessionID != code.SessionID
	})
	asked := r.starter.asked()
	if last := asked[len(asked)-1]; last.Dir != code.WorkDir || last.ProjectID != "" || again.Stage != 0 {
		t.Fatalf("the stage again, in the same folder: %+v %+v", last, again)
	}
	if old, err := r.st.GafferGoalFor(r.p.UserID, code.ConversationID); err != nil || old.State != "stopped" {
		t.Fatalf("goal of the ended session: %+v %v", old, err)
	}
}

// A session a pipeline has finished with is ended, its history kept: the one
// a stage left for a session of its own, and the last one when every stage is
// done. A session the next stage carries on in is left running.
func TestSessionsAPipelineHasFinishedWithAreEnded(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, true))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	r.s.stageHandedOver(r.p.UserID, code.ConversationID, "Endpoint committed.", "gaffer")
	review := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 1 })
	wait := func(n int) []string {
		deadline := time.Now().Add(10 * time.Second)
		for len(r.starter.closed()) < n && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		return r.starter.closed()
	}
	if got := wait(1); len(got) != 1 || got[0] != code.SessionID {
		t.Fatalf("the code stage's session after review started in its own: %v", got)
	}
	if archived, err := r.st.TerminalArchivedFor(r.p.UserID, code.SessionID); err != nil || archived {
		t.Fatalf("its history must be kept: %v %v", archived, err)
	}
	// Review to test is the same agent and model: the session carries on.
	r.s.stageHandedOver(r.p.UserID, review.ConversationID, "Reviewed.", "gaffer")
	test := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 2 })
	if test.SessionID != review.SessionID || len(r.starter.closed()) != 1 {
		t.Fatalf("a session the next stage carries on in must stay open: %+v %v", test, r.starter.closed())
	}
	r.s.stageHandedOver(r.p.UserID, test.ConversationID, "Green.", "gaffer")
	merge := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 3 })
	r.s.stageHandedOver(r.p.UserID, merge.ConversationID, "Merged.", "gaffer")
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "done" })
	if got := wait(2); len(got) != 2 || got[1] != merge.SessionID {
		t.Fatalf("the last session when the pipeline is done: %v", got)
	}
}

// Moving a pipeline on names the stage the person was looking at.
func TestMovingOnIsRefusedWhenThePipelineHasLeftTheStageThePersonSaw(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if _, err := r.s.MovePipelineOn(r.p.UserID, created.ID, code.Key, "review"); err == nil || !strings.Contains(err.Error(), "already left the review stage") {
		t.Fatalf("a move from another stage: %v", err)
	}
	if _, err := r.s.MovePipelineOn(r.p.UserID, created.ID, code.Key, "code"); err != nil {
		t.Fatal(err)
	}
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 1 })
}

// Google Antigravity starts only in a trusted folder, so an outcome that
// names it without trust is refused whole instead of failing stage by stage.
func TestAnAntigravityOutcomeWithoutFolderTrustIsRefusedBeforeAnythingStarts(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	in := r.in
	in.Tool = "antigravity"
	if _, err := r.s.CreateOutcome(r.p.UserID, in); err == nil || !strings.Contains(err.Error(), "trusted") {
		t.Fatalf("main agent without trust: %v", err)
	}
	staged := pipelineInput(r, false)
	staged.Sessions[0].Stages[1].Tool = "antigravity"
	if _, err := r.s.CreateOutcome(r.p.UserID, staged); err == nil {
		t.Fatal("a crew agent without trust was accepted")
	}
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	add := AddPipelinesInput{Confirmed: true, Sessions: []OutcomeSession{{Title: "Docs", Prompt: "Write the docs.", Stages: []OutcomeStageInput{{Kind: "code", Tool: "antigravity"}}}}}
	if _, err := r.s.AddPipelines(r.p.UserID, created.ID, add); err == nil || !strings.Contains(err.Error(), "without folder trust") {
		t.Fatalf("an added pipeline on Antigravity without trust: %v", err)
	}
	if len(r.starter.asked()) > 1 {
		t.Fatalf("launches: %+v", r.starter.asked())
	}
}

// One outcome whose launch is slow does not hold up another's.
func TestASlowLaunchForOneOutcomeDoesNotHoldUpAnother(t *testing.T) {
	r := outcomeFixture(t)
	r.starter.hold, r.starter.holdOnly = make(chan struct{}), "Slow part"
	defer close(r.starter.hold)
	r.s.StartGoalSupervisor()
	slow := r.in
	slow.Sessions = []OutcomeSession{{Title: "Slow part", Prompt: "Take your time. Do not merge."}}
	first, err := r.s.CreateOutcome(r.p.UserID, slow)
	if err != nil {
		t.Fatal(err)
	}
	quick := r.in
	quick.Sessions = []OutcomeSession{{Title: "Quick part", Prompt: "Be quick. Do not merge."}}
	second, err := r.s.CreateOutcome(r.p.UserID, quick)
	if err != nil {
		t.Fatal(err)
	}
	r.piece(t, second.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if held, err := r.st.GafferOutcomeFor(r.p.UserID, first.ID); err != nil || held.Sessions[0].State != "starting" {
		t.Fatalf("the slow outcome is still starting: %+v %v", held.Sessions, err)
	}
}

// The worktree a pipeline's first session was given is still wanted while the
// pipeline has work left, whoever asks to remove it.
func TestAPipelinesFolderIsInUseWhileItHasWorkLeft(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, true))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if code.WorkDir == "" || !r.s.WorkDirInUse(r.p.UserID, code.WorkDir) || r.s.WorkDirInUse(r.p.UserID, "/work/elsewhere") || r.s.WorkDirInUse(r.p.UserID, "") {
		t.Fatalf("a followed pipeline's folder is in use: %q", code.WorkDir)
	}
	// The controller writes once more after a pipeline is followed, to settle the outcome: the revision is read after that.
	r.settle(t, created.ID)
	o, err := r.s.Outcome(r.p.UserID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if o, err = r.s.StopFollowingOutcome(r.p.UserID, created.ID, o.Revision); err != nil || !r.s.WorkDirInUse(r.p.UserID, code.WorkDir) {
		t.Fatalf("a stopped pipeline can still be tried again in its folder: %v", err)
	}
	if _, err := r.s.ArchiveOutcome(r.p.UserID, created.ID, o.Revision, true); err != nil || r.s.WorkDirInUse(r.p.UserID, code.WorkDir) {
		t.Fatalf("an archived outcome has no work left in the folder: %v", err)
	}
}

// An outcome is its owner's: another account cannot read or steer it by its
// ID, and an account that may only look starts and changes nothing.
func TestAnOutcomeCannotBeReadOrSteeredByAnotherAccountOrAViewer(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	other, err := r.st.CreateUser("other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := r.st.CreateUser("viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	// The controller writes once more after a pipeline is followed, to settle the outcome: the revision is read after that.
	r.settle(t, created.ID)
	before, err := r.st.GafferOutcomeFor(r.p.UserID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	for name, uid := range map[string]int64{"another account": other.ID, "a viewer": viewer.ID} {
		if _, err := r.s.Outcome(uid, created.ID); err == nil {
			t.Fatalf("%s read the outcome", name)
		}
		if rows, err := r.s.Outcomes(uid); err == nil && len(rows) != 0 {
			t.Fatalf("%s listed the outcome: %+v", name, rows)
		}
		tries := map[string]error{}
		_, tries["move on"] = r.s.MovePipelineOn(uid, created.ID, code.Key, "")
		_, tries["send back"] = r.s.SendPipelineBack(uid, created.ID, code.Key, "code", "Again.")
		_, tries["merge setting"] = r.s.SetPipelineAutoMerge(uid, created.ID, code.Key, true, false)
		_, tries["tell"] = r.s.TellPipelineAgent(context.Background(), uid, created.ID, code.Key, strings.Repeat("c", 32), "Carry on.")
		_, tries["stop following"] = r.s.StopFollowingOutcome(uid, created.ID, before.Revision)
		_, tries["retry"] = r.s.RetryPipeline(uid, created.ID, code.Key, before.Revision)
		_, tries["approve merge"] = r.s.ApproveMerge(uid, created.ID, code.Key, before.Revision)
		tries["delete"] = r.s.DeleteOutcome(uid, created.ID, before.Revision)
		for what, err := range tries {
			if err == nil {
				t.Fatalf("%s could %s", name, what)
			}
		}
	}
	if after, err := r.st.GafferOutcomeFor(r.p.UserID, created.ID); err != nil || after.Revision != before.Revision {
		t.Fatalf("the outcome changed: %+v %v", after, err)
	}
}
