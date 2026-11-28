package gaffer

import (
	"strings"

	"github.com/eyupio/reet.sh/internal/store"
)

// A work report is the mirror of a work order: an outcome already running
// in the Factory, written back out in the same Markdown-with-a-header style,
// for a person to copy to an outside agent or a connected agent to read with
// a future MCP tool. It is pure: built only from what the outcome already
// holds, with the same state words the Factory page and mcpOutcome already
// use (see internal/api/gaffer_factory_mcp.go), never a live Git or GitHub
// read. A criterion's status therefore comes from stored evidence only,
// read the same way criterionView already reads evidence recorded against a
// Git-less folder: commitNone, the existing fallback for "this decision was
// recorded without a commit", used here because WorkReport has no machine to
// ask for one either.

// workReportHandoverBytes is how much of a stage's handover note a work
// report carries: the same bound as mcpHandoverBytes in
// internal/api/gaffer_factory_mcp.go, which cannot be reused directly since
// internal/api imports internal/gaffer, not the other way round.
const workReportHandoverBytes = 600

// WorkReport is the work report for outcome o: the header, each pipeline's
// state, stages, acceptance criteria and (when handovers is true) its
// stages' handover notes, and a fixed closing paragraph telling the reader
// it can answer with a work order carrying "continues: <id>" to add more
// pipelines to this outcome. handovers is false where the words are to go
// somewhere that session text must not: the existing rule for stage
// handovers over MCP. The person's own copy always passes true.
func WorkReport(o store.GafferOutcome, handovers bool) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("reet: work-report/1\n")
	b.WriteString("outcome: " + o.ID + "\n")
	b.WriteString("objective: " + o.Objective + "\n")
	b.WriteString("state: " + stateWord(o.State) + "\n")
	b.WriteString("---\n")
	for _, piece := range o.Sessions {
		writeWorkReportPipeline(&b, piece, handovers)
	}
	b.WriteString("\nTo add more work to this outcome, answer with a work order whose header carries continues: " + o.ID + ".\n")
	return b.String()
}

// stateWord is a stored state string as a reader sees it: a space where the
// store keeps an underscore, nothing more. Every word it can produce is one
// the Factory page and mcpOutcome already print; this never restates what
// each one means.
func stateWord(state string) string { return strings.ReplaceAll(state, "_", " ") }

// writeWorkReportPipeline writes one "## Pipeline:" section: its state, the
// stage in hand if any, every stage with its own status, its handovers
// (when asked for), its acceptance criteria with their live status, and,
// for a pipeline waiting on the person, why.
func writeWorkReportPipeline(b *strings.Builder, piece store.GafferOutcomeSession, handovers bool) {
	b.WriteString("\n## Pipeline: " + piece.Title + "\n")
	b.WriteString("state: " + stateWord(piece.State) + "\n")
	inHand := -1
	if piece.Stage < len(piece.Stages) && piece.Stages[piece.Stage].State != "done" {
		inHand = piece.Stage
		b.WriteString("stage in hand: " + piece.Stages[piece.Stage].Label() + "\n")
	}
	if len(piece.Stages) > 0 {
		b.WriteString("\n")
		for i, stage := range piece.Stages {
			b.WriteString("- " + stage.Label() + ": " + stageReportStatus(stage, i == inHand) + "\n")
		}
	}
	if handovers {
		for _, stage := range piece.Stages {
			if stage.Summary == "" {
				continue
			}
			b.WriteString("\n### Handover: " + stage.Label() + "\n" + clipTo(stage.Summary, workReportHandoverBytes) + "\n")
		}
	}
	if len(piece.Criteria) > 0 {
		b.WriteString("\n### Criteria\n")
		for _, c := range piece.Criteria {
			view := criterionView(c, piece.Evidence, deliveryHead{state: commitNone})
			b.WriteString("- [" + view.Status + "] " + view.Text + "\n")
		}
	}
	if note := waitingForPersonNote(piece.State); note != "" {
		b.WriteString("\n### Waiting for the person\n" + note + "\n")
	}
}

// stageReportStatus is one stage's line in a pipeline's stage list: "in
// hand" for the one awaiting work, "done" with its tier (when the stage was
// judged to need one) for a finished stage, or its own raw state, read the
// same minimal way, for anything else. A stage is given "pending" as soon as
// it joins its outcome (outcome_manage.go), so a future, not-yet-started
// stage already reads that way; a blank state only ever exists in memory
// before that write, so it falls back to the same word rather than the
// empty string.
func stageReportStatus(stage store.GafferOutcomeStage, inHand bool) string {
	switch {
	case inHand:
		return "in hand"
	case stage.State == "done":
		if stage.Tier != "" {
			return "done (" + stage.Tier + ")"
		}
		return "done"
	case stage.State == "":
		return "pending"
	default:
		return stateWord(stage.State)
	}
}

// waitingForPersonNote is the sentence under "### Waiting for the person"
// for a pipeline's state, or "" where that pipeline needs nothing from the
// person right now.
func waitingForPersonNote(state string) string {
	switch state {
	case "delivery_review", "ready_to_merge":
		word := stateWord(state)
		return strings.ToUpper(word[:1]) + word[1:] + "."
	}
	return ""
}
