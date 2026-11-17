package gaffer

import (
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

// Found on a real run: Gaffer's report called a 900-character brief cut off
// mid-sentence, and could not say what the second of two code stages was for.
func TestGafferIsShownAPipelinesBriefAndItsParts(t *testing.T) {
	brief := strings.Repeat("Review the recent changes carefully. ", 40) + "Do not merge anything."
	o := store.GafferOutcome{Objective: "Fix the top five", ProjectName: "reet", Sessions: []store.GafferOutcomeSession{{
		Title: "Review and fix", State: "following", Prompt: brief,
		Stages: []store.GafferOutcomeStage{
			{Kind: "plan", State: "running"},
			{Kind: "code", State: "pending", Part: "Fixes 1 to 3", Brief: "Fix the three most serious."},
			{Kind: "code", State: "pending", Part: "Fixes 4 and 5", Brief: "Fix the other two and write the notes."},
		},
	}}}
	got := (&Service{}).outcomeEvidence(o)
	if !strings.Contains(got, "Do not merge anything.") {
		t.Fatalf("a brief of %d bytes was cut:\n%s", len(brief), got)
	}
	for _, want := range []string{"- code, part \"Fixes 1 to 3\": pending. Part brief: Fix the three most serious.", "- code, part \"Fixes 4 and 5\": pending. Part brief: Fix the other two and write the notes."} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestABriefTooLongForGaffersEvidenceSaysItWasShortened(t *testing.T) {
	o := store.GafferOutcome{Objective: "x", Sessions: []store.GafferOutcomeSession{{Title: "Long", State: "following", Prompt: strings.Repeat("word ", 1500)}}}
	got := (&Service{}).outcomeEvidence(o)
	if !strings.Contains(got, "[Brief shortened here; the agent has all of it.]") {
		t.Fatalf("a shortened brief does not say so:\n%s", got[:400])
	}
	if len(got) > 4000 {
		t.Fatalf("evidence for one long brief is %d bytes", len(got))
	}
}

// Found on the same run: the plan stage's handover described the plan
// ("a ranked shortlist ... stated in the plan body") without the list, and the
// code stage started in a session that could not read it.
func TestAStageHandoverIsToldToCarryThePlanItself(t *testing.T) {
	for name, text := range map[string]string{"rule": stageHandoverRule, "tool": handOverTool.Description} {
		if !strings.Contains(text, "cannot read this session") && !strings.Contains(text, "may not be able to read this session") {
			t.Fatalf("%s does not say the next stage cannot read this session: %s", name, text)
		}
	}
	if !strings.Contains(stageHandoverRule, "each item or decision in order") {
		t.Fatal(stageHandoverRule)
	}
}
