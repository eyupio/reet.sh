package gaffer

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/store"
)

func pathwayStageInputs() []OutcomeStageInput {
	return []OutcomeStageInput{{Kind: "review"}, {Kind: "code"}, {Kind: "custom", Name: "Audit", Prompt: "Audit the logging."}, {Kind: "pr"}, {Kind: "merge"}}
}

func TestLabelFallsBackToKind(t *testing.T) {
	if got := (store.GafferOutcomeStage{Kind: "code"}).Label(); got != "code" {
		t.Fatalf("code label = %q", got)
	}
	if got := (store.GafferOutcomeStage{Kind: "custom", Name: "Audit"}).Label(); got != "Audit" {
		t.Fatalf("custom label = %q", got)
	}
	if got := (store.GafferOutcomeStage{Kind: "custom"}).Label(); got != "custom" {
		t.Fatalf("unnamed custom label = %q", got)
	}
}

func TestPathwayPipelineKeepsItsOrder(t *testing.T) {
	in := []OutcomeSession{{Title: "Logs", Prompt: "Tidy the logs.", Stages: pathwayStageInputs(), Pathway: &store.GafferPathwayRef{ID: "pw1", Name: "Careful"}}}
	pieces, err := outcomePieces(in, "opencode", "", "", store.GafferCrew{}, false, map[string]bool{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	piece := pieces[0]
	if piece.PathwayID != "pw1" || piece.PathwayName != "Careful" {
		t.Fatalf("pathway = %q %q", piece.PathwayID, piece.PathwayName)
	}
	var kinds []string
	for _, st := range piece.Stages {
		kinds = append(kinds, st.Kind)
	}
	if strings.Join(kinds, ",") != "review,code,custom,pr,merge" {
		t.Fatalf("order = %v", kinds)
	}
	if piece.Stages[2].Name != "Audit" || piece.Stages[2].Prompt != "Audit the logging." || piece.Stages[2].Tier != "standard" {
		t.Fatalf("custom stage = %+v", piece.Stages[2])
	}
}

func TestPathwayPipelineRefusesPartsAndBadOrder(t *testing.T) {
	ref := &store.GafferPathwayRef{ID: "pw1", Name: "Careful"}
	parted := pathwayStageInputs()
	parted[1].Part, parted[1].Brief = "API", "Do the API."
	for name, stages := range map[string][]OutcomeStageInput{
		"part":    parted,
		"order":   {{Kind: "merge"}, {Kind: "pr"}},
		"unknown": {{Kind: "deploy"}},
	} {
		in := []OutcomeSession{{Title: "Logs", Prompt: "Tidy the logs.", Stages: stages, Pathway: ref}}
		if _, err := outcomePieces(in, "opencode", "", "", store.GafferCrew{}, false, map[string]bool{}, 0); err == nil {
			t.Fatalf("%s: a pathway pipeline was accepted", name)
		}
	}
}

func TestStandardPipelineStillRefusesReorderAndCustom(t *testing.T) {
	in := []OutcomeSession{{Title: "Logs", Prompt: "Tidy the logs.", Stages: pathwayStageInputs()}}
	_, err := outcomePieces(in, "opencode", "", "", store.GafferCrew{}, false, map[string]bool{}, 0)
	if err == nil || !strings.Contains(err.Error(), "Keep the stages") {
		t.Fatalf("reordered standard pipeline: %v", err)
	}
	in = []OutcomeSession{{Title: "Logs", Prompt: "Tidy the logs.", Stages: []OutcomeStageInput{{Kind: "custom", Name: "Audit", Prompt: "Audit."}}}}
	if _, err := outcomePieces(in, "opencode", "", "", store.GafferCrew{}, false, map[string]bool{}, 0); err == nil {
		t.Fatal("a custom stage with no pathway was accepted")
	}
}

func pathwayItem(prompt string, custom store.GafferOutcomeStage) store.GafferOutcomeSession {
	return store.GafferOutcomeSession{Prompt: prompt, Stages: []store.GafferOutcomeStage{{Kind: "review"}, custom, {Kind: "pr"}, {Kind: "merge"}}}
}

func TestCustomStagePrompt(t *testing.T) {
	item := pathwayItem("Tidy the logs.", store.GafferOutcomeStage{Kind: "custom", Name: "Audit", Prompt: "Audit the logging for secrets."})
	item.Stages[0].Summary = "Reviewed."
	got := stagePrompt(store.GafferOutcome{}, item, 1)
	for _, want := range []string{"the “Audit” stage", "Audit the logging for secrets.", "what this stage’s instructions ask for", "The later stages (pr, merge)", "Handover from the review stage"} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt lacks %q:\n%s", want, got)
		}
	}
	if !strings.HasSuffix(got, " Do not merge.") {
		t.Fatalf("prompt does not end with the boundary:\n%s", got)
	}
}

func TestBuiltInStageExtraInstructions(t *testing.T) {
	item := store.GafferOutcomeSession{Prompt: "p", Stages: []store.GafferOutcomeStage{{Kind: "code", Prompt: "Use the new logger."}}}
	got := stagePrompt(store.GafferOutcome{}, item, 0)
	work, extra := strings.Index(got, stageWork["code"]), strings.Index(got, "Extra instructions for this stage:\nUse the new logger.")
	if work < 0 || extra < work {
		t.Fatalf("built-in work then extra instructions expected:\n%s", got)
	}
}

func TestLongCustomPromptKeepsTheBoundary(t *testing.T) {
	item := pathwayItem(strings.Repeat("p", MaxOutcomePromptBytes), store.GafferOutcomeStage{Kind: "custom", Name: "Audit", Prompt: strings.Repeat("é", MaxStagePromptBytes/2)})
	item.Stages[0].Summary = strings.Repeat("s", 2000)
	got := stagePrompt(store.GafferOutcome{}, item, 1)
	if len(got) > MaxOutcomePromptBytes || !utf8.ValidString(got) || !strings.Contains(got, "Do not merge.") {
		t.Fatalf("len %d, valid %v, boundary %v", len(got), utf8.ValidString(got), strings.Contains(got, "Do not merge."))
	}
}

func TestLongCustomStageGoalFitsTheGoalLimit(t *testing.T) {
	stage := store.GafferOutcomeStage{Kind: "custom", Name: strings.Repeat("ß", MaxStageNameRunes), Prompt: strings.Repeat("é", MaxStagePromptBytes/2)}
	got := StageGoal(strings.Repeat("o", 4000), strings.Repeat("t", 80), stage)
	if len(got) > store.MaxGafferGoalBytes || !utf8.ValidString(got) || !strings.HasSuffix(got, " Nothing is merged in this stage.") {
		t.Fatalf("len %d, valid %v:\n%s", len(got), utf8.ValidString(got), got)
	}
}

func TestPathwayRefusesUnnamedCustomStage(t *testing.T) {
	in := []OutcomeSession{{Title: "Logs", Prompt: "Tidy the logs.", Stages: []OutcomeStageInput{{Kind: "custom", Prompt: "Audit."}}, Pathway: &store.GafferPathwayRef{ID: "pw1", Name: "Careful"}}}
	if _, err := outcomePieces(in, "opencode", "", "", store.GafferCrew{}, false, map[string]bool{}, 0); err == nil {
		t.Fatal("an unnamed custom stage was accepted")
	}
}

func TestCustomStageGoal(t *testing.T) {
	prompt := strings.Repeat("a", 1200) + strings.Repeat("b", 800)
	got := StageGoal("Ship it.", "Logs", store.GafferOutcomeStage{Kind: "custom", Name: "Audit", Prompt: prompt})
	if !strings.Contains(got, "Stage: Audit") || !strings.Contains(got, strings.Repeat("a", 1200-len("…"))) || strings.Contains(got, "ab") || strings.Contains(got, "bb") {
		t.Fatalf("goal:\n%s", got)
	}
}
