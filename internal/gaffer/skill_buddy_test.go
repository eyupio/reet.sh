package gaffer

import (
	"github.com/eyupio/reet.sh/internal/skillbuddy"
	"github.com/eyupio/reet.sh/internal/store"
	"strings"
	"testing"
)

func TestSelectedSkillsReachOnlyTheirStageAndKeepTheBoundary(t *testing.T) {
	sk := reviewSkill()
	item := store.GafferOutcomeSession{Prompt: strings.Repeat("The complete project brief. ", 1000), Stages: []store.GafferOutcomeStage{{Kind: "code"}, {Kind: "review", SkillGuidance: []skillbuddy.Skill{sk}}, {Kind: "test"}}}
	review := stagePrompt(store.GafferOutcome{}, item, 1)
	if !strings.Contains(review, sk.Instructions) || !strings.HasSuffix(review, "Do not merge.") || len(review) > MaxOutcomePromptBytes {
		t.Fatalf("invalid reviewed stage prompt %d: %s", len(review), review)
	}
	if strings.Contains(stagePrompt(store.GafferOutcome{}, item, 0), sk.Instructions) {
		t.Fatal("review guidance leaked into code stage")
	}
}

func reviewSkill() skillbuddy.Skill {
	for _, sk := range skillbuddy.Builtins() {
		if sk.ID == "builtin:review" {
			return sk
		}
	}
	panic("missing review skill")
}

func TestSendingPipelineBackPreservesReviewedSkillGuidance(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	uid := r.p.UserID
	o := r.followedOutcome(t, "1", "", "unfollowed")
	first := reviewSkill()
	future := first
	future.ID, future.Name, future.Revision, future.Instructions = "builtin:test", "Pinned test guidance", "reviewed-revision", "Verify the tenant boundary."
	var err error
	o, err = r.st.UpdateGafferOutcomeFor(uid, o.ID, o.Revision, func(o *store.GafferOutcome) bool {
		o.State = "stopped"
		o.Sessions[0].Stages = []store.GafferOutcomeStage{
			{Kind: "review", Tool: "opencode", State: "running", RequestID: strings.Repeat("6", 32), SkillGuidance: []skillbuddy.Skill{first}},
			{Kind: "test", Tool: "opencode", State: "pending", RequestID: strings.Repeat("7", 32), SkillGuidance: []skillbuddy.Skill{future}},
		}
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	back, err := r.s.SendPipelineBack(uid, o.ID, "a", "review", "Recheck the tenant boundary.")
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []skillbuddy.Skill{first, future} {
		stage := back.Sessions[0].Stages[i]
		if len(stage.SkillGuidance) != 1 || stage.SkillGuidance[0].ID != want.ID || stage.SkillGuidance[0].Revision != want.Revision || stage.SkillGuidance[0].Instructions != want.Instructions {
			t.Fatalf("reviewed guidance lost during send-back: %+v", stage)
		}
		if !strings.Contains(stagePrompt(back, back.Sessions[0], i), want.Instructions) {
			t.Fatal("restarted prompt omitted pinned guidance")
		}
	}
}

func TestSelectedSkillsRemainInUsageFailoverPrompt(t *testing.T) {
	sk := reviewSkill()
	item := store.GafferOutcomeSession{Prompt: strings.Repeat("Project context. ", 1000), Stages: []store.GafferOutcomeStage{{Kind: "review", UsageFailover: true, SkillGuidance: []skillbuddy.Skill{sk}}, {Kind: "test"}}}
	prompt := stagePrompt(store.GafferOutcome{}, item, 0)
	if !strings.Contains(prompt, sk.Instructions) || !strings.Contains(prompt, "previous worker reached its usage limit") || !strings.HasSuffix(prompt, "Do not merge.") || len(prompt) > MaxOutcomePromptBytes {
		t.Fatalf("failover lost guidance or stage boundary: %s", prompt)
	}
}
