package gaffer

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

func partsInput(r *outcomeRig) OutcomeInput {
	in := r.in
	in.Sessions = []OutcomeSession{{Title: "Export API", Prompt: "Add the CSV export endpoint with tests. Do not merge.", Stages: []OutcomeStageInput{
		{Kind: "code", Tool: "opencode", Model: "strong/coder", Tier: "heavy", Part: "Ledger query", Brief: "Write the query that reads the billing ledger for the export."},
		{Kind: "code", Tool: "opencode", Model: "cheap/coder", Tier: "light", Part: "Download button", Brief: "Add the button to the billing page that calls the endpoint."},
		{Kind: "review", Tool: "opencode", Model: "strong/reviewer"},
	}}}
	return in
}

// A stage split into parts runs them one after another, each in a session of
// its own on its own tier's agent, each told what its part is and handed the
// part before's handover. The stage after starts when the last part is done.
func TestAStagesPartsRunInOrderEachInItsOwnSession(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, partsInput(r))
	if err != nil {
		t.Fatal(err)
	}
	first := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if first.Stage != 0 || first.Stages[0].Part != "Ledger query" || first.Stages[1].Part != "Download button" || first.Stages[1].Kind != "code" || first.Stages[2].Part != "" {
		t.Fatalf("stages: %+v", first.Stages)
	}
	r.s.stageHandedOver(r.p.UserID, first.ConversationID, "The query is in ledger.sql with a test.", "gaffer")
	second := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.Stage == 1 && p.State == "following" })
	if second.SessionID == first.SessionID || second.Stages[0].State != "done" {
		t.Fatalf("the second part did not get a session of its own: %+v", second)
	}
	r.s.stageHandedOver(r.p.UserID, second.ConversationID, "The button is wired.", "gaffer")
	review := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.Stage == 2 && p.State == "following" })
	asked := r.starter.asked()
	if len(asked) != 3 || asked[0].Model != "strong/coder" || asked[1].Model != "cheap/coder" || asked[2].Model != "strong/reviewer" {
		t.Fatalf("launches: %+v", asked)
	}
	if !strings.HasSuffix(asked[0].Title, "code: Ledger query") || !strings.HasSuffix(asked[1].Title, "code: Download button") || !strings.HasSuffix(asked[2].Title, "· review") {
		t.Fatalf("titles: %q %q %q", asked[0].Title, asked[1].Title, asked[2].Title)
	}
	one, two, three := asked[0].Prompt, asked[1].Prompt, asked[2].Prompt
	if !strings.Contains(one, "ONE PART of one stage") || !strings.Contains(one, "the “Ledger query” part of the code stage") || !strings.Contains(one, "This part: Write the query that reads the billing ledger") || !strings.Contains(one, "The other parts of this stage (Download button) are done separately") {
		t.Fatalf("first part's prompt:\n%s", one)
	}
	// The later stages are named once each, and not the part's own kind.
	if !strings.Contains(one, "The later stages (review) are done separately") {
		t.Fatalf("first part's later stages:\n%s", one)
	}
	if !strings.Contains(two, "the “Download button” part of the code stage") || !strings.Contains(two, "Handover from the “Ledger query” part of the code stage: The query is in ledger.sql with a test.") {
		t.Fatalf("second part's prompt:\n%s", two)
	}
	if !strings.Contains(three, "ONE stage of a staged piece of work: the review stage") || !strings.Contains(three, "Handover from the “Download button” part of the code stage: The button is wired.") {
		t.Fatalf("review's prompt:\n%s", three)
	}
	// Each finished part's session is ended once the next one is in Gaffer's care, which is just after it starts.
	deadline := time.Now().Add(15 * time.Second)
	for len(r.starter.closed()) != 2 {
		if time.Now().After(deadline) {
			t.Fatalf("finished parts' sessions were not ended: %v", r.starter.closed())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if review.Stages[1].State != "done" {
		t.Fatalf("second part: %+v", review.Stages[1])
	}
}

// What a split may be: named parts of one kind, sitting together, each with a
// brief, at most four, and never the pull request, babysitting or the merge.
func TestWhatAStageCanBeSplitInto(t *testing.T) {
	r := outcomeFixture(t)
	part := func(kind, name, brief string) OutcomeStageInput {
		return OutcomeStageInput{Kind: kind, Tool: "opencode", Part: name, Brief: brief}
	}
	for name, stages := range map[string][]OutcomeStageInput{
		"the same kind twice with no parts":     {{Kind: "code", Tool: "opencode"}, {Kind: "code", Tool: "opencode"}},
		"a part beside a stage done as one":     {{Kind: "code", Tool: "opencode"}, part("code", "B", "Do b.")},
		"two parts with one name":               {part("code", "A", "Do a."), part("code", "A", "Do it again.")},
		"a part with no brief":                  {part("code", "A", ""), part("code", "B", "Do b.")},
		"a brief with no part":                  {{Kind: "code", Tool: "opencode", Brief: "Do it."}},
		"a split pull request":                  {part("pr", "A", "Raise it."), part("pr", "B", "Raise it again.")},
		"five parts":                            {part("code", "A", "a"), part("code", "B", "b"), part("code", "C", "c"), part("code", "D", "d"), part("code", "E", "e")},
		"parts that are not next to each other": {part("code", "A", "Do a."), {Kind: "review", Tool: "opencode"}, part("code", "B", "Do b.")},
	} {
		in := r.in
		in.Sessions = []OutcomeSession{{Title: "Export API", Prompt: "Do it. Do not merge.", Stages: stages}}
		if _, err := r.s.CreateOutcome(r.p.UserID, in); err == nil {
			t.Fatalf("accepted: %s", name)
		}
	}
	// A stage sent back keeps its parts.
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, partsInput(r))
	if err != nil {
		t.Fatal(err)
	}
	first := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	r.s.stageHandedOver(r.p.UserID, first.ConversationID, "Done.", "gaffer")
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.Stage == 1 && p.State == "following" })
	if _, err = r.s.SendPipelineBack(r.p.UserID, created.ID, first.Key, "code", "The query misses refunds."); err != nil {
		t.Fatal(err)
	}
	back := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.Stage == 0 && p.State == "following" })
	if back.Stages[0].Part != "Ledger query" || back.Stages[0].Brief == "" || back.Stages[1].Part != "Download button" || back.Stages[1].State != "pending" {
		t.Fatalf("sent back: %+v", back.Stages)
	}
}

// Gaffer may split a stage in its plan: the parts replace the stage, in
// order, each a name, a brief and a tier. It names no agent, one part is not
// a split, and the pull request is never split.
func TestGaffersPlanCanSplitAStageIntoParts(t *testing.T) {
	plan := `{"summary":"One pipeline.","sessions":[{"title":"Export API","prompt":"Add the export. Do not merge.","stages":["code","review","pr"],
 "ratings":[{"stage":"code","tier":"heavy","why":"Touches the ledger."}],
 "parts":[{"stage":"code","title":"Ledger query","brief":"Write the query."},{"stage":"code","title":"Download button","brief":"Add the button.","tier":"light","why":"Wiring only."}]}]}`
	_, sessions, err := outcomeSessions(json.RawMessage(plan), 3)
	if err != nil || len(sessions) != 1 || len(sessions[0].Stages) != 4 {
		t.Fatalf("%+v %v", sessions, err)
	}
	got := sessions[0].Stages
	if got[0].Kind != "code" || got[0].Part != "Ledger query" || got[0].Brief != "Write the query." || got[0].Tier != "heavy" || got[0].TierWhy != "Touches the ledger." ||
		got[1].Kind != "code" || got[1].Part != "Download button" || got[1].Tier != "light" || got[1].TierWhy != "Wiring only." || got[2].Kind != "review" || got[2].Part != "" || got[3].Kind != "pr" || got[0].Tool != "" {
		t.Fatalf("stages: %+v", got)
	}
	for name, parts := range map[string]string{
		"one part":             `[{"stage":"code","title":"A","brief":"a"}]`,
		"a stage it lacks":     `[{"stage":"design","title":"A","brief":"a"},{"stage":"design","title":"B","brief":"b"}]`,
		"two with one name":    `[{"stage":"code","title":"A","brief":"a"},{"stage":"code","title":"A","brief":"b"}]`,
		"a split pull request": `[{"stage":"pr","title":"A","brief":"a"},{"stage":"pr","title":"B","brief":"b"}]`,
		"an agent named":       `[{"stage":"code","title":"A","brief":"a","agent":"claude"},{"stage":"code","title":"B","brief":"b"}]`,
	} {
		raw := `{"summary":"s","sessions":[{"title":"Export API","prompt":"Add the export. Do not merge.","stages":["code","pr"],"parts":` + parts + `}]}`
		if _, _, err := outcomeSessions(json.RawMessage(raw), 3); err == nil {
			t.Fatalf("accepted: %s", name)
		}
	}
}

// A part's session title fits the protocol's limit whatever the lengths of
// the pipeline's and the part's names, and keeps the stage and the part.
func TestAPartsSessionTitleFitsTheLimit(t *testing.T) {
	long := strings.Repeat("Pipeline name ", 6)[:80]
	for _, kind := range []string{"code", "review", "design"} {
		got := partTitle(long, kind, strings.Repeat("p", 60))
		if n := len([]rune(got)); n > 80 || !strings.Contains(got, " · "+kind+": ") || !strings.HasSuffix(got, "…") {
			t.Fatalf("%s: %d runes: %q", kind, n, got)
		}
	}
	if got := partTitle("Export API", "code", "Ledger query"); got != "Export API · code: Ledger query" {
		t.Fatalf("short: %q", got)
	}
	// Counted in characters, not bytes.
	if got := partTitle(strings.Repeat("é", 80), "code", "Ünïcödé"); len([]rune(got)) > 80 {
		t.Fatalf("accents: %d", len([]rune(got)))
	}
}

// A goal holds 2,000 bytes. The longest title, objective, part name and brief a
// part can have still leave a goal Gaffer can save, or the session would start
// and never be followed.
func TestTheGoalOfAPartWithTheLongestTextsIsStillSaved(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	in := r.in
	in.Objective = strings.Repeat("o", 600)
	in.Sessions = []OutcomeSession{{Title: strings.Repeat("漢", 80), Prompt: "Review it. Do not merge.", Stages: []OutcomeStageInput{
		{Kind: "review", Tool: "opencode", Part: strings.Repeat("漢", 80), Brief: strings.Repeat("漢", 400)},
		{Kind: "review", Tool: "opencode", Part: "Tests", Brief: "Check the tests."},
	}}}
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	piece := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	g, err := r.st.GafferGoalFor(r.p.UserID, piece.ConversationID)
	if err != nil || len(g.Text) > 2000 || !strings.Contains(g.Text, "This session does one part of the stage only") {
		t.Fatalf("the part's goal is %d bytes: %v", len(g.Text), err)
	}
}
