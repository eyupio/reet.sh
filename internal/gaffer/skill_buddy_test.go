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
