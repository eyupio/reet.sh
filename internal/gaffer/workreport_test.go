package gaffer

import (
	"os"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

// sampleWorkReportOutcome is a stored outcome with two pipelines: one
// finished, with a handover on its code stage; one in delivery review, with
// one criterion passed (from a manual note) and one never given evidence, so
// it stays unverified.
func sampleWorkReportOutcome() store.GafferOutcome {
	return store.GafferOutcome{
		ID:        "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4",
		Objective: "Add CSV export to the reports page",
		State:     "running",
		Sessions: []store.GafferOutcomeSession{
			{
				Key:   "p1",
				Title: "Export endpoint",
				State: "done",
				Stage: 1,
				Stages: []store.GafferOutcomeStage{
					{Kind: "code", Tier: "heavy", State: "done", Summary: "Added GET /reports/{id}/export.csv with a streaming CSV writer. Reused the report's existing read permission check."},
					{Kind: "review", State: "done"},
				},
			},
			{
				Key:   "p2",
				Title: "Export performance",
				State: "delivery_review",
				Stage: 1,
				Stages: []store.GafferOutcomeStage{
					{Kind: "code", Tier: "standard", State: "done"},
					{Kind: "test", State: "running"},
					{Kind: "pr", State: "pending"},
				},
				Criteria: []store.GafferCriterion{
					{ID: "c1", Text: "An export of 10,000 rows finishes in under 5 seconds", Revision: 1},
					{ID: "c2", Text: "A reader of another team's report is refused", Revision: 1},
				},
				CriteriaRevision: 1,
				Evidence: []store.GafferEvidence{
					{ID: "ev1", CriterionID: "c1", Kind: "note", Verdict: CriterionPassed, Text: "Timed it at 2.1s for 10,000 rows.", CriteriaRevision: 1},
				},
			},
		},
	}
}

// TestWorkReportGolden checks WorkReport against a golden file built from
// its own reviewed output, and the few things the brief fixes regardless of
// wording: the header's shape, that handovers=false drops handover text, and
// the fixed closing paragraph.
func TestWorkReportGolden(t *testing.T) {
	o := sampleWorkReportOutcome()

	got := WorkReport(o, true)
	want, err := os.ReadFile("testdata/workreport_golden.md")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("WorkReport(o, true):\n%s\nwant:\n%s", got, string(want))
	}

	if without := WorkReport(o, false); strings.Contains(without, "### Handover") {
		t.Errorf("WorkReport(o, false) still carries a handover:\n%s", without)
	}

	lines := strings.SplitN(got, "\n", 3)
	if len(lines) < 2 || lines[0] != "---" || lines[1] != "reet: work-report/1" {
		t.Errorf("header's first two lines: %q", lines[:min(2, len(lines))])
	}
	if !strings.Contains(got, "outcome: "+o.ID) {
		t.Errorf("missing \"outcome: %s\"", o.ID)
	}

	if trimmed := strings.TrimRight(got, "\n"); !strings.Contains(trimmed, "continues: "+o.ID) || !strings.HasSuffix(got, "\n") {
		t.Errorf("does not end with the fixed paragraph carrying \"continues: %s\":\n%s", o.ID, got)
	}
}
