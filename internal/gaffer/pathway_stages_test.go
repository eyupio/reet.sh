package gaffer

import (
	"strings"
	"testing"
	"time"
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

// pathwayOutcome is a stored, stopped outcome whose one pipeline holds these stages, at stage at.
func (r *outcomeRig) pathwayOutcome(t *testing.T, at int, stages ...store.GafferOutcomeStage) store.GafferOutcome {
	t.Helper()
	o := r.followedOutcome(t, "1", "", "unfollowed")
	for i := range stages {
		stages[i].Tool, stages[i].RequestID = "opencode", strings.Repeat(string(rune('a'+i)), 32)
		stages[i].State = "done"
		if i == at {
			stages[i].State = "running"
		} else if i > at {
			stages[i].State = "pending"
		}
	}
	o, err := r.st.UpdateGafferOutcomeFor(r.p.UserID, o.ID, o.Revision, func(o *store.GafferOutcome) bool {
		o.State = "stopped"
		o.Sessions[0].Stages, o.Sessions[0].Stage = stages, at
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestSendBackToACustomStage(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	o := r.pathwayOutcome(t, 2, store.GafferOutcomeStage{Kind: "code"}, store.GafferOutcomeStage{Kind: "custom", Name: "Audit", Prompt: "Audit the logging."}, store.GafferOutcomeStage{Kind: "pr"})
	back, err := r.s.SendPipelineBack(r.p.UserID, o.ID, "a", "Audit", "Look again.")
	if err != nil {
		t.Fatal(err)
	}
	row := back.Sessions[0]
	if row.Stage != 1 || row.Stages[1].State != "pending" || row.Stages[1].Name != "Audit" || row.Stages[1].Prompt != "Audit the logging." {
		t.Fatalf("sent back to %d: %+v", row.Stage, row.Stages[1])
	}
	if row.Note != "Sent back to the Audit stage. Starting it again." {
		t.Fatalf("note = %q", row.Note)
	}
}

func TestOldOutcomeStillSendsBackByKind(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	o := r.pathwayOutcome(t, 1, store.GafferOutcomeStage{Kind: "code"}, store.GafferOutcomeStage{Kind: "review"})
	back, err := r.s.SendPipelineBack(r.p.UserID, o.ID, "a", "code", "Again.")
	if err != nil {
		t.Fatal(err)
	}
	if row := back.Sessions[0]; row.Stage != 0 || row.Note != "Sent back to the code stage. Starting it again." {
		t.Fatalf("sent back to %d: %q", row.Stage, row.Note)
	}
}

func pathwayInput(r *outcomeRig, stages ...OutcomeStageInput) OutcomeInput {
	in := r.in
	for i := range stages {
		stages[i].Tool, stages[i].Model = "opencode", "cheap/coder"
	}
	in.Sessions = []OutcomeSession{{Title: "Logs", Prompt: "Tidy the logs.", Stages: stages, Pathway: &store.GafferPathwayRef{ID: "pw1", Name: "Careful"}}}
	return in
}

func TestMoveOnNamesACustomStage(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pathwayInput(r, OutcomeStageInput{Kind: "custom", Name: "Audit", Prompt: "Audit the logging."}, OutcomeStageInput{Kind: "pr"}))
	if err != nil {
		t.Fatal(err)
	}
	audit := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if _, err := r.s.MovePipelineOn(r.p.UserID, created.ID, audit.Key, "code"); err == nil || !strings.Contains(err.Error(), "is now at Audit.") {
		t.Fatalf("a move from another stage: %v", err)
	}
	if _, err := r.s.MovePipelineOn(r.p.UserID, created.ID, audit.Key, "Audit"); err != nil {
		t.Fatal(err)
	}
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 1 })
}

func TestDeliveryListsCustomStagesApart(t *testing.T) {
	item := store.GafferOutcomeSession{Stage: 3, Stages: []store.GafferOutcomeStage{{Kind: "code"}, {Kind: "custom", Name: "A"}, {Kind: "custom", Name: "B"}, {Kind: "merge"}}}
	view := deliveryView(store.GafferOutcome{}, item, deliveryHead{})
	if strings.Join(view.Stages, ",") != "code,A,B" || view.DefaultStage != "code" {
		t.Fatalf("stages %v, default %q", view.Stages, view.DefaultStage)
	}
}

func TestMergeAfterACustomStageStillWaits(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pathwayInput(r, OutcomeStageInput{Kind: "code"}, OutcomeStageInput{Kind: "custom", Name: "A", Prompt: "Check A."}, OutcomeStageInput{Kind: "pr"}, OutcomeStageInput{Kind: "merge"}))
	if err != nil {
		t.Fatal(err)
	}
	for at := 0; at < 3; at++ {
		p := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == at })
		r.s.stageHandedOver(r.p.UserID, p.ConversationID, "Done.", "gaffer")
	}
	waiting := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "ready_to_merge" })
	launched := len(r.starter.asked())
	time.Sleep(100 * time.Millisecond)
	o, err := r.s.Outcome(r.p.UserID, created.ID)
	if err != nil || o.Sessions[0].State != "ready_to_merge" || waiting.Stage != 3 || waiting.Stages[3].SessionID != "" || len(r.starter.asked()) != launched {
		t.Fatalf("the merge did not wait: %+v %v", o.Sessions[0], err)
	}
}
