package gaffer

import (
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func readExample(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/workorder_example.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestWorkOrderExample parses the skill's own worked example, so the
// published example can never drift from the parser.
func TestWorkOrderExample(t *testing.T) {
	order, problems := ParseWorkOrder(readExample(t))
	if len(problems) != 0 {
		t.Fatalf("problems: %+v", problems)
	}
	if order.Objective != "Add CSV export to the reports page" {
		t.Errorf("objective: %q", order.Objective)
	}
	if order.Project != "reports-api" {
		t.Errorf("project: %q", order.Project)
	}
	if got, want := order.Crew["heavy"], (WorkOrderMember{Tool: "claude", Model: "opus", Effort: "high"}); got != want {
		t.Errorf("crew[heavy]: %+v", got)
	}
	if got, want := order.Crew["standard"], (WorkOrderMember{Tool: "codex"}); got != want {
		t.Errorf("crew[standard]: %+v", got)
	}
	if len(order.Sessions) != 1 {
		t.Fatalf("sessions: %+v", order.Sessions)
	}
	session := order.Sessions[0]
	if session.Title != "Export endpoint" {
		t.Errorf("title: %q", session.Title)
	}
	wantStages := []OutcomeStageInput{
		{Kind: "plan"},
		{Kind: "code", Part: "Streaming writer", Tier: "heavy", Brief: "Write rows as they are read, never the whole file in memory."},
		{Kind: "code", Part: "Endpoint and auth", Tier: "heavy", Brief: "Wire the handler and reuse the report's read permission."},
		{Kind: "review", Tier: "standard"},
		{Kind: "test"},
		{Kind: "pr"},
	}
	if !reflect.DeepEqual(session.Stages, wantStages) {
		t.Fatalf("stages:\n got: %+v\nwant: %+v", session.Stages, wantStages)
	}
	if len(session.Criteria) != 2 {
		t.Fatalf("criteria: %+v", session.Criteria)
	}
}

// TestWorkOrderTolerates checks the forms a work order may arrive in: with
// Windows line endings, or wrapped in one fenced code block with a sentence
// either side, and that IsWorkOrder recognises all three while staying false
// for ordinary prose, even prose that mentions the header mid-sentence.
func TestWorkOrderTolerates(t *testing.T) {
	clean := readExample(t)
	crlf := strings.ReplaceAll(clean, "\n", "\r\n")
	fenced := "Here is the work order you asked for.\n\n```markdown\n" + clean + "\n```\n\nHope that helps.\n"

	cleanOrder, problems := ParseWorkOrder(clean)
	if len(problems) != 0 {
		t.Fatalf("clean: %+v", problems)
	}
	crlfOrder, problems := ParseWorkOrder(crlf)
	if len(problems) != 0 {
		t.Fatalf("crlf: %+v", problems)
	}
	fencedOrder, problems := ParseWorkOrder(fenced)
	if len(problems) != 0 {
		t.Fatalf("fenced: %+v", problems)
	}
	if !reflect.DeepEqual(cleanOrder, crlfOrder) {
		t.Errorf("crlf parsed differently:\n%+v\n%+v", cleanOrder, crlfOrder)
	}
	if !reflect.DeepEqual(cleanOrder, fencedOrder) {
		t.Errorf("fenced parsed differently:\n%+v\n%+v", cleanOrder, fencedOrder)
	}
	for _, text := range []string{clean, crlf, fenced} {
		if !IsWorkOrder(text) {
			t.Errorf("IsWorkOrder false for a work order: %q", text[:40])
		}
	}
	if IsWorkOrder("Add CSV export") {
		t.Error("IsWorkOrder true for plain prose")
	}
	if IsWorkOrder("Please remember that reet: work-order/1 is the format to use.") {
		t.Error("IsWorkOrder true for a mid-sentence mention")
	}
}

// TestWorkOrderNestedFenceInBrief guards against a fenced code sample inside
// a pipeline's brief being mistaken for the fence that wraps the whole work
// order: the spec allows "any Markdown" in a brief, and a code sample is an
// entirely foreseeable thing for a feature describing coding work.
func TestWorkOrderNestedFenceInBrief(t *testing.T) {
	clean := readExample(t)
	// Give the first pipeline's brief a fenced code sample of its own.
	withSample := strings.Replace(clean, "up to the first ### heading)\n",
		"up to the first ### heading)\n\n```go\nfunc Example() {}\n```\n", 1)
	if withSample == clean {
		t.Fatal("the brief text to extend was not found")
	}

	// Unwrapped: a plain paste whose brief happens to contain a fence must
	// still be read as the whole work order, not misread as itself being
	// wrapped in that inner fence.
	plainOrder, problems := ParseWorkOrder(withSample)
	if len(problems) != 0 {
		t.Fatalf("a brief's own fence broke the unwrapped paste: %+v", problems)
	}
	if !IsWorkOrder(withSample) {
		t.Error("IsWorkOrder false for an unwrapped paste with a fence in its brief")
	}

	// Wrapped in one outer fence, with a sentence either side, same as
	// TestWorkOrderTolerates: the inner fence must not be mistaken for the
	// outer one's close, which would truncate the real work order.
	wrapped := "Here is the work order you asked for.\n\n```markdown\n" + withSample + "\n```\n\nHope that helps.\n"
	wrappedOrder, problems := ParseWorkOrder(wrapped)
	if len(problems) != 0 {
		t.Fatalf("a brief's own fence broke the outer wrapping fence: %+v", problems)
	}
	if !IsWorkOrder(wrapped) {
		t.Error("IsWorkOrder false for a wrapped work order with a fence in its brief")
	}
	if !reflect.DeepEqual(plainOrder, wrappedOrder) {
		t.Errorf("wrapping changed the result:\n%+v\n%+v", plainOrder, wrappedOrder)
	}
	if !strings.Contains(plainOrder.Sessions[0].Prompt, "func Example() {}") {
		t.Errorf("the brief's own fenced sample was lost: %q", plainOrder.Sessions[0].Prompt)
	}
}

// TestWorkOrderFenceWithTrailingSample guards the other direction: an
// agent's closing remark, after the outer fence that wraps the whole work
// order, may itself show a fenced sample. That must not be mistaken for the
// wrapper's own close, which would make the scan run on into the remark and
// swallow it, and the fence after it, as if they were still the work order.
func TestWorkOrderFenceWithTrailingSample(t *testing.T) {
	clean := readExample(t)
	wrapped := "Here is the work order you asked for.\n\n```markdown\n" + clean + "\n```\n\n" +
		"By the way, here is how the export endpoint is called:\n\n```go\nhttp.Get(url)\n```\n\nHope that helps.\n"

	cleanOrder, problems := ParseWorkOrder(clean)
	if len(problems) != 0 {
		t.Fatalf("clean: %+v", problems)
	}
	wrappedOrder, problems := ParseWorkOrder(wrapped)
	if len(problems) != 0 {
		t.Fatalf("a fenced sample in the trailing remark broke the wrapper: %+v", problems)
	}
	if !IsWorkOrder(wrapped) {
		t.Error("IsWorkOrder false for a wrapper followed by a remark with its own fence")
	}
	if !reflect.DeepEqual(cleanOrder, wrappedOrder) {
		t.Errorf("the trailing remark's fence changed the result:\n%+v\n%+v", cleanOrder, wrappedOrder)
	}
}

func lineOf(t *testing.T, text, marker string) int {
	t.Helper()
	idx := strings.Index(text, marker)
	if idx < 0 {
		t.Fatalf("marker %q not found in text", marker)
	}
	return strings.Count(text[:idx], "\n") + 1
}

// TestWorkOrderRefuses is the refusal table: each case introduces exactly
// one defect into an otherwise sound work order, and is refused with one
// problem. Where the defect sits on an identifiable line, that line is
// checked too.
func TestWorkOrderRefuses(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		marker     string // "" skips the exact-line check
		wantSubstr string
	}{
		{
			name:   "wrong version",
			text:   "---\nreet: work-order/2\nobjective: Fix the bug\n---",
			marker: "reet: work-order/2", wantSubstr: "work-order/1",
		},
		{
			name:   "unknown header key",
			text:   "---\nreet: work-order/1\nobjective: Fix the bug\nyolo: true\n---",
			marker: "yolo: true", wantSubstr: "yolo",
		},
		{
			name:   "duplicate header key",
			text:   "---\nreet: work-order/1\nobjective: Fix the bug\nobjective: Fix it again\n---",
			marker: "objective: Fix it again", wantSubstr: "objective",
		},
		{
			name:       "missing objective",
			text:       "---\nreet: work-order/1\n---",
			wantSubstr: "objective",
		},
		{
			name:       "objective too long",
			text:       "---\nreet: work-order/1\nobjective: " + strings.Repeat("x", MaxOutcomeObjectiveBytes+1) + "\n---",
			marker:     "objective: " + strings.Repeat("x", 1),
			wantSubstr: strconv.Itoa(MaxOutcomeObjectiveBytes),
		},
		{
			name:       "no pipeline",
			text:       "---\nreet: work-order/1\nobjective: Fix the bug\n---\n",
			wantSubstr: "Pipeline",
		},
		{
			name: "six pipelines",
			text: "---\nreet: work-order/1\nobjective: Fix the bug\n---\n\n" +
				"## Pipeline: One\n\nDo one.\n\n" +
				"## Pipeline: Two\n\nDo two.\n\n" +
				"## Pipeline: Three\n\nDo three.\n\n" +
				"## Pipeline: Four\n\nDo four.\n\n" +
				"## Pipeline: Five\n\nDo five.\n\n" +
				"## Pipeline: Six\n\nDo six.\n",
			marker: "## Pipeline: Six", wantSubstr: strconv.Itoa(MaxOutcomeSessions),
		},
		{
			name: "duplicate pipeline title",
			text: "---\nreet: work-order/1\nobjective: Fix the bug\n---\n\n" +
				"## Pipeline: Dup\n\nDo it.\n\n" +
				"## Pipeline: Dup\n\nDo it again.\n",
			marker: "## Pipeline: Dup\n\nDo it again.", wantSubstr: "Dup",
		},
		{
			name: "stages out of order",
			text: "---\nreet: work-order/1\nobjective: Fix the bug\n---\n\n" +
				"## Pipeline: One\n\nstages: code, plan\n\nDo it.\n",
			marker: "stages: code, plan", wantSubstr: "order",
		},
		{
			name: "unknown stage",
			text: "---\nreet: work-order/1\nobjective: Fix the bug\n---\n\n" +
				"## Pipeline: One\n\nstages: deploy\n\nDo it.\n",
			marker: "stages: deploy", wantSubstr: "deploy",
		},
		{
			name: "bad tier value",
			text: "---\nreet: work-order/1\nobjective: Fix the bug\n---\n\n" +
				"## Pipeline: One\n\nstages: code\ntiers: code=huge\n\nDo it.\n",
			marker: "tiers: code=huge", wantSubstr: "huge",
		},
		{
			name: "tier names a stage not in stages",
			text: "---\nreet: work-order/1\nobjective: Fix the bug\n---\n\n" +
				"## Pipeline: One\n\nstages: code\ntiers: review=standard\n\nDo it.\n",
			marker: "tiers: review=standard", wantSubstr: "review",
		},
		{
			name: "part of a stage that cannot be split",
			text: "---\nreet: work-order/1\nobjective: Fix the bug\n---\n\n" +
				"## Pipeline: One\n\nstages: code, pr\n\nDo it.\n\n" +
				"### Part: pr / Open it\n\nOpen the pull request.\n\n" +
				"### Part: pr / Tidy it\n\nTidy the description.\n",
			marker: "### Part: pr / Open it", wantSubstr: "pr",
		},
		{
			name: "a split with only one part",
			text: "---\nreet: work-order/1\nobjective: Fix the bug\n---\n\n" +
				"## Pipeline: One\n\nstages: code\n\nDo it.\n\n" +
				"### Part: code / Only one\n\nThe only part.\n",
			marker: "### Part: code / Only one", wantSubstr: "two",
		},
		{
			name: "unknown section",
			text: "---\nreet: work-order/1\nobjective: Fix the bug\n---\n\n" +
				"## Pipeline: One\n\nDo it.\n\n### Notes\n\nSome notes.\n",
			marker: "### Notes", wantSubstr: "Notes",
		},
		{
			name: "unknown top-level heading",
			text: "---\nreet: work-order/1\nobjective: Fix the bug\n---\n\n" +
				"## Something else\n\nStray text.\n",
			marker: "## Something else", wantSubstr: "Pipeline",
		},
		{
			name: "a heading inside a brief",
			text: "---\nreet: work-order/1\nobjective: Fix the bug\n---\n\n" +
				"## Pipeline: One\n\nDo it.\n\n## Oops\n\nmore\n",
			marker: "## Oops", wantSubstr: "Pipeline",
		},
		{
			name: "a stray --- inside the body",
			text: "---\nreet: work-order/1\nobjective: Fix the bug\n---\n\n" +
				"## Pipeline: One\n\nDo it.\n\n---\n\nmore\n",
			marker: "---\n\nmore", wantSubstr: "header",
		},
		{
			name: "eleven criteria",
			text: "---\nreet: work-order/1\nobjective: Fix the bug\n---\n\n" +
				"## Pipeline: One\n\nDo it.\n\n### Criteria\n\n" +
				"- one\n- two\n- three\n- four\n- five\n- six\n- seven\n- eight\n- nine\n- ten\n- eleven\n",
			marker: "- eleven", wantSubstr: strconv.Itoa(10),
		},
		{
			name: "pathway with a pipeline's own stages",
			text: "---\nreet: work-order/1\nobjective: Fix the bug\npathway: My pathway\n---\n\n" +
				"## Pipeline: One\n\nstages: code\n\nDo it.\n",
			marker: "stages: code", wantSubstr: "pathway",
		},
		{
			name:   "pathway with crew",
			text:   "---\nreet: work-order/1\nobjective: Fix the bug\npathway: My pathway\ncrew: heavy=claude\n---\n",
			marker: "crew: heavy=claude", wantSubstr: "pathway",
		},
		{
			name:   "crew names a tier twice",
			text:   "---\nreet: work-order/1\nobjective: Fix the bug\ncrew: heavy=claude, heavy=codex\n---\n",
			marker: "crew: heavy=claude, heavy=codex", wantSubstr: "twice",
		},
		{
			name:   "crew names an unknown tier",
			text:   "---\nreet: work-order/1\nobjective: Fix the bug\ncrew: huge=claude\n---\n",
			marker: "crew: huge=claude", wantSubstr: "huge",
		},
		{
			name:   "crew names an agent reet does not run",
			text:   "---\nreet: work-order/1\nobjective: Fix the bug\ncrew: heavy=nosuchagent\n---\n",
			marker: "crew: heavy=nosuchagent", wantSubstr: "nosuchagent",
		},
		{
			name:   "continues with crew",
			text:   "---\nreet: work-order/1\ncontinues: abc123\ncrew: heavy=claude\n---\n",
			marker: "crew: heavy=claude", wantSubstr: "continues",
		},
		{
			name: "two work orders in one paste",
			text: "---\nreet: work-order/1\nobjective: Fix the bug\n---\n\n" +
				"## Pipeline: One\n\nDo it.\n\n" +
				"---\nreet: work-order/1\nobjective: A second order\n---\n",
			wantSubstr: "one at a time",
		},
		{
			name:       "the whole text is too long",
			text:       strings.Repeat("x", MaxWorkOrderBytes+1),
			wantSubstr: strconv.Itoa(MaxWorkOrderBytes),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			order, problems := ParseWorkOrder(tc.text)
			if len(problems) != 1 {
				t.Fatalf("problems: %+v (order %+v)", problems, order)
			}
			if !strings.Contains(problems[0].Text, tc.wantSubstr) {
				t.Errorf("text %q does not contain %q", problems[0].Text, tc.wantSubstr)
			}
			if tc.marker != "" {
				if want := lineOf(t, tc.text, tc.marker); problems[0].Line != want {
					t.Errorf("line = %d, want %d (problem: %q)", problems[0].Line, want, problems[0].Text)
				}
			}
			if order.Objective != "" || len(order.Sessions) != 0 {
				t.Errorf("a refused work order must not be usable: %+v", order)
			}
		})
	}
}

// the whole text too long" case above has no line 0 check built in; confirm it directly.
func TestWorkOrderTooLongIsLineZero(t *testing.T) {
	_, problems := ParseWorkOrder(strings.Repeat("x", MaxWorkOrderBytes+1))
	if len(problems) != 1 || problems[0].Line != 0 {
		t.Fatalf("problems: %+v", problems)
	}
}

// TestWorkOrderReusesValidation proves the brief byte limit is read from
// MaxOutcomePromptBytes itself, not copied: a brief of exactly that many
// bytes is accepted, and one byte more is refused.
func TestWorkOrderReusesValidation(t *testing.T) {
	build := func(n int) string {
		return "---\nreet: work-order/1\nobjective: Fix the bug\n---\n\n## Pipeline: One\n\n" + strings.Repeat("x", n) + "\n"
	}
	if _, problems := ParseWorkOrder(build(MaxOutcomePromptBytes)); len(problems) != 0 {
		t.Fatalf("exactly the limit refused: %+v", problems)
	}
	if _, problems := ParseWorkOrder(build(MaxOutcomePromptBytes + 1)); len(problems) != 1 {
		t.Fatalf("one byte over the limit not refused: %+v", problems)
	}
}
