package gaffer

import (
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// Every stage is a session of its own, in the pipeline's folder, whatever
// agent and model do it: the same agent on the same model starts afresh with
// the stage's own instruction and the handover, and the session of the stage
// before is ended.
func TestEveryStageRunsInASessionOfItsOwn(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	in := pipelineInput(r, true)
	in.Sessions[0].Stages = []OutcomeStageInput{
		{Kind: "code", Tool: "opencode", Model: "same/model"},
		{Kind: "review", Tool: "opencode", Model: "same/model"},
		{Kind: "merge", Tool: "opencode", Model: "same/model"},
	}
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var before store.GafferOutcomeSession
	for stage, kind := range []string{"code", "review", "merge"} {
		piece := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == stage })
		if piece.SessionID == "" || seen[piece.SessionID] || piece.Stages[stage].SessionID != piece.SessionID {
			t.Fatalf("the %s stage has no session of its own: %+v", kind, piece)
		}
		seen[piece.SessionID] = true
		asked := r.starter.asked()
		if len(asked) != stage+1 {
			t.Fatalf("launches by the %s stage: %d", kind, len(asked))
		}
		launch := asked[stage]
		if launch.Model != "same/model" || !strings.HasPrefix(launch.Prompt, "You are doing ONE stage of a staged piece of work: the "+kind+" stage.") {
			t.Fatalf("%s launch: %+v", kind, launch)
		}
		if stage > 0 {
			// It works where the stage before did and is told what that stage left.
			if launch.Dir != before.WorkDir || launch.ProjectID != "" || !strings.Contains(launch.Prompt, "Handover from the "+before.Stages[stage-1].Kind+" stage: Done with "+before.Stages[stage-1].Kind+".") {
				t.Fatalf("%s launch does not follow on: %+v", kind, launch)
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				closed := strings.Join(r.starter.closed(), ",")
				if strings.Contains(closed, before.SessionID) {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("the %s stage's session was left running: closed %q", before.Stages[stage-1].Kind, closed)
				}
				time.Sleep(10 * time.Millisecond)
			}
			// The earlier stage keeps the session it ran in, for the record.
			if piece.Stages[stage-1].SessionID != before.SessionID {
				t.Fatalf("the %s stage lost its session: %+v", before.Stages[stage-1].Kind, piece.Stages[stage-1])
			}
		}
		before = piece
		if kind != "merge" {
			r.s.stageHandedOver(r.p.UserID, piece.ConversationID, "Done with "+kind+".", "gaffer")
		}
	}
}
