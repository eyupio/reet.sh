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
	b.WriteString("---\nreet: work-report/1\noutcome: " + o.ID + "\nobjective: " + oneLine(o.Objective) + "\nstate: " + o.State + "\n")
	if o.ArchivedAt > 0 {
		b.WriteString("archived: yes\n")
	}
	b.WriteString("---\n")
	for _, piece := range o.Sessions {
		b.WriteString("\n## Pipeline: " + oneLine(piece.Title) + "\nstate: " + piece.State + "\n")
		if piece.PathwayName != "" {
			b.WriteString("pathway: " + oneLine(piece.PathwayName) + "\n")
		}
		if piece.Stage < len(piece.Stages) && piece.Stages[piece.Stage].State != "done" {
			b.WriteString("stage in hand: " + oneLine(piece.Stages[piece.Stage].Label()) + "\n")
		}
		if len(piece.Stages) > 0 {
			b.WriteString("\n")
		}
		for _, st := range piece.Stages {
			b.WriteString("- " + stageName(st) + ": " + st.State)
			if st.Tier != "" {
				b.WriteString(" (" + oneLine(st.Tier) + ")")
			}
			b.WriteString("\n")
		}
		for _, st := range piece.Stages {
			if handovers && st.Summary != "" {
				b.WriteString("\n### Handover: " + stageName(st) + "\n")
				for _, line := range strings.Split(strings.ReplaceAll(clipTo(st.Summary, workReportHandoverBytes), "\r", ""), "\n") {
					b.WriteString(strings.TrimRight("> "+line, " ") + "\n")
				}
			}
		}
		if len(piece.Criteria) > 0 {
			b.WriteString("\n### Criteria\n")
			for _, c := range piece.Criteria {
				b.WriteString("- [" + reportCriterionStatus(piece, c) + "] " + oneLine(c.Text) + "\n")
			}
			b.WriteString("Statuses are as last recorded; the delivery review in reet is the live view.\n")
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

// stageName is a stage's label, with the part's title for one part of a split stage.
func stageName(st store.GafferOutcomeStage) string {
	if st.Part != "" {
		return oneLine(st.Label()) + " / " + oneLine(st.Part)
	}
	return oneLine(st.Label())
}

// reportCriterionStatus is the delivery review's own rule (criterionView),
// applied to what is stored. Nothing stores the commit the review last read,
// so the reviewed commit is taken to be the one on the latest evidence, with
// no live checks. A recorded "passed" is kept only while the pipeline waits
// (delivery review, ready to merge, done) and no stage started after the
// latest evidence: otherwise work may have moved the commit, and the report
// says "unverified". "needs_work" is always shown.
func reportCriterionStatus(piece store.GafferOutcomeSession, c store.GafferCriterion) string {
	head := deliveryHead{state: commitNone}
	var last int64
	for _, ev := range piece.Evidence {
		last = max(last, ev.At)
	}
	if n := len(piece.Evidence); n > 0 && piece.Evidence[n-1].Commit != "" {
		head = deliveryHead{state: commitVerified, commit: piece.Evidence[n-1].Commit}
	}
	status := criterionView(c, piece.Evidence, head).Status
	if status != CriterionPassed {
		return status
	}
	switch piece.State {
	case "delivery_review", "ready_to_merge", "done":
	default:
		return CriterionUnverified
	}
	for _, st := range piece.Stages {
		if st.StartedAt > last {
			return CriterionUnverified
		}
	}
	return status
}
