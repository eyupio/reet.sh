package gaffer

import (
	"strings"

	"github.com/eyupio/reet.sh/internal/store"
)

// workReportHandoverBytes clips a stage's handover as reet_get_factory_outcome does.
const workReportHandoverBytes = 600

// oneLine makes stored text safe for a single report line.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// WorkReport writes an outcome as the Markdown work report an outside agent
// reads (work-report/1). Handovers are words taken from sessions, so they are
// included only when asked and every line of one is quoted with "> ": a
// handover can then never open a heading, a "---" header or a section.
// Criteria status comes from the latest recorded verdict for the present
// wording; checks are read live from GitHub and so are shown as unverified.
func WorkReport(o store.GafferOutcome, handovers bool) string {
	var b strings.Builder
	b.WriteString("---\nreet: work-report/1\noutcome: " + o.ID + "\nobjective: " + oneLine(o.Objective) + "\nstate: " + o.State + "\n---\n")
	for _, piece := range o.Sessions {
		b.WriteString("\n## Pipeline: " + oneLine(piece.Title) + "\nstate: " + piece.State + "\n")
		if piece.Stage < len(piece.Stages) && piece.Stages[piece.Stage].State != "done" {
			b.WriteString("stage in hand: " + oneLine(piece.Stages[piece.Stage].Label()) + "\n")
		}
		if len(piece.Stages) > 0 {
			b.WriteString("\n")
		}
		for _, st := range piece.Stages {
			b.WriteString("- " + oneLine(st.Label()) + ": " + st.State)
			if st.Tier != "" {
				b.WriteString(" (" + oneLine(st.Tier) + ")")
			}
			b.WriteString("\n")
		}
		for _, st := range piece.Stages {
			if handovers && st.Summary != "" {
				b.WriteString("\n### Handover: " + oneLine(st.Label()) + "\n")
				for _, line := range strings.Split(strings.ReplaceAll(clipTo(st.Summary, workReportHandoverBytes), "\r", ""), "\n") {
					b.WriteString("> " + line + "\n")
				}
			}
		}
		if len(piece.Criteria) > 0 {
			b.WriteString("\n### Criteria\n")
			for _, c := range piece.Criteria {
				b.WriteString("- [" + reportCriterionStatus(piece, c) + "] " + oneLine(c.Text) + "\n")
			}
		}
		switch piece.State {
		case "delivery_review":
			b.WriteString("\n### Waiting for the person\nDelivery review.\n")
		case "ready_to_merge":
			b.WriteString("\n### Waiting for the person\nReady to merge.\n")
		}
	}
	b.WriteString("\nTo add work to this outcome, answer with a work order whose header carries `continues: " + o.ID + "` and one `## Pipeline: <title>` section per new pipeline. Its objective is ignored.\n")
	return b.String()
}

func reportCriterionStatus(piece store.GafferOutcomeSession, c store.GafferCriterion) string {
	status := CriterionUnverified
	for _, ev := range piece.Evidence {
		if ev.CriterionID == c.ID && ev.Kind != "check" && ev.CriteriaRevision >= c.Revision && ev.Verdict != "" {
			status = ev.Verdict
		}
	}
	return status
}
