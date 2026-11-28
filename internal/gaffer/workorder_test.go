package gaffer

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func workOrderExample(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/workorder_example.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func mustParseWorkOrder(t *testing.T, text string) WorkOrder {
	t.Helper()
	order, problems := ParseWorkOrder(text)
	if len(problems) != 0 {
		t.Fatalf("problems: %+v", problems)
	}
	return order
}

func TestWorkOrderExample(t *testing.T) {
	order := mustParseWorkOrder(t, workOrderExample(t))
	if order.Objective != "Add CSV export to the reports page" || order.Project != "reports-api" {
		t.Fatalf("header: %+v", order)
	}
	if order.Crew["heavy"] != (WorkOrderMember{"claude", "opus", "high"}) || order.Crew["standard"] != (WorkOrderMember{"codex", "", ""}) || len(order.Crew) != 2 {
		t.Fatalf("crew: %+v", order.Crew)
	}
	if len(order.Sessions) != 1 || order.Sessions[0].Title != "Export endpoint" || len(order.Sessions[0].Criteria) != 2 {
		t.Fatalf("sessions: %+v", order.Sessions)
	}
	type row struct{ Kind, Part, Tier string }
	var got []row
	for _, s := range order.Sessions[0].Stages {
		got = append(got, row{s.Kind, s.Part, s.Tier})
	}
	want := []row{{"plan", "", ""}, {"code", "Streaming writer", "heavy"}, {"code", "Endpoint and auth", "heavy"}, {"review", "", "standard"}, {"test", "", ""}, {"pr", "", ""}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stages: %+v", got)
	}
}

func TestWorkOrderTolerates(t *testing.T) {
	clean := workOrderExample(t)
	want := mustParseWorkOrder(t, clean)
	crlf := strings.ReplaceAll(clean, "\n", "\r\n")
	fenced := "Here is the work order.\n\n```markdown\n" + clean + "```\n\nTell me what you think.\n"
	for name, text := range map[string]string{"crlf": crlf, "fenced": fenced} {
		if !IsWorkOrder(text) {
			t.Errorf("%s: not recognised", name)
		}
		if got := mustParseWorkOrder(t, text); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: differs: %+v", name, got)
		}
	}
	if !IsWorkOrder(clean) || IsWorkOrder("Add CSV export") || IsWorkOrder("Use reet: work-order/1 for this.") {
		t.Error("recognition")
	}
}

func TestWorkOrderRefuses(t *testing.T) {
	head := "---\nreet: work-order/1\nobjective: Do it\n"
	pipe := func(title string) string { return "\n## Pipeline: " + title + "\n\nThe brief.\n" }
	ok := head + "---\n" + pipe("One")
	repeat := func(s string, n int) string { return strings.Repeat(s, n) }
	var six strings.Builder
	for _, n := range []string{"A", "B", "C", "D", "E", "F"} {
		six.WriteString(pipe(n))
	}
	tests := []struct {
		name, text string
		line       int
		has        string
	}{
		{"version", "---\nreet: work-order/2\nobjective: x\n---\n" + pipe("One"), 2, "work-order/1"},
		{"unknown key", head + "yolo: true\n---\n" + pipe("One"), 4, "yolo"},
		{"duplicate key", head + "objective: Again\n---\n" + pipe("One"), 4, "objective"},
		{"no objective", "---\nreet: work-order/1\n---\n" + pipe("One"), 2, "objective"},
		{"long objective", "---\nreet: work-order/1\nobjective: " + repeat("a", MaxOutcomeObjectiveBytes+1) + "\n---\n" + pipe("One"), 3, "objective"},
		{"no pipeline", head + "---\n", 0, "Pipeline"},
		{"six pipelines", head + "---\n" + six.String(), 26, "at most"},
		{"same title", head + "---\n" + pipe("One") + pipe("One"), 10, "own title"},
		{"stage order", head + "---\n\n## Pipeline: One\nstages: code, plan\n\nBrief.\n", 7, "order"},
		{"unknown stage", head + "---\n\n## Pipeline: One\nstages: deploy\n\nBrief.\n", 7, "deploy"},
		{"bad tier", head + "---\n\n## Pipeline: One\nstages: code\ntiers: code=huge\n\nBrief.\n", 8, "huge"},
		{"tier of absent stage", head + "---\n\n## Pipeline: One\nstages: code\ntiers: review=heavy\n\nBrief.\n", 8, "review"},
		{"unsplittable part", ok + "\n### Part: pr / x\n\nBrief.\n", 10, "pr"},
		{"one part", head + "---\n\n## Pipeline: One\nstages: code\n\nBrief.\n\n### Part: code / x\n\nBrief.\n", 11, "2 to 4"},
		{"unknown section", ok + "\n### Notes\n\nx\n", 10, "Notes"},
		{"other h2", ok + "\n## Something else\n", 10, "Something else"},
		{"h2 in brief", head + "---\n\n## Pipeline: One\n\nBrief\n\n## Another\n\nmore\n", 10, "Another"},
		{"h3 in brief", head + "---\n\n## Pipeline: One\n\nBrief\n\n### Context\n\nmore\n", 10, "Context"},
		{"rule in brief", head + "---\n\n## Pipeline: One\n\nBrief\n---\nmore\n", 9, "---"},
		{"eleven criteria", ok + "\n### Criteria\n\n" + repeat("- one\n", 11), 0, "criteria"},
		{"pathway with stages", head + "pathway: Ship it\n---\n\n## Pipeline: One\nstages: code\n\nBrief.\n", 8, "pathway"},
		{"pathway with crew", head + "pathway: Ship it\ncrew: heavy=claude\n---\n" + pipe("One"), 5, "crew"},
		{"repeated tier", head + "crew: heavy=claude, heavy=codex\n---\n" + pipe("One"), 4, "heavy"},
		{"unknown tier", head + "crew: huge=claude\n---\n" + pipe("One"), 4, "huge"},
		{"unknown agent", head + "crew: heavy=nosuchagent\n---\n" + pipe("One"), 4, "nosuchagent"},
		{"continues with crew", head + "continues: " + strings.Repeat("a", 32) + "\ncrew: heavy=claude\n---\n" + pipe("One"), 5, "crew"},
		{"two orders", ok + "\n---\nreet: work-order/1\nobjective: Again\n---\n" + pipe("Two"), 10, "one work order"},
		{"unclosed fence", head + "---\n\n## Pipeline: One\n\nBrief\n```\ncode\n\n## Pipeline: Two\n\nMore\n", 9, "never closed"},
		{"wrong fence closes", head + "---\n\n## Pipeline: One\n\nBrief\n```\ncode\n~~~\n\n## Another\n", 9, "never closed"},
		{"short fence closes", head + "---\n\n## Pipeline: One\n\nBrief\n````\ncode\n```\n", 9, "never closed"},
		{"level one heading", head + "---\n\n## Pipeline: One\n\nBrief\n\n# Top\n", 10, "Top"},
		{"chatter before", "Here you go.\n\n" + ok, 1, "on its own"},
		{"empty project", head + "project:\n---\n" + pipe("One"), 4, "project"},
		{"empty pathway", head + "pathway:\n---\n" + pipe("One"), 4, "pathway"},
		{"empty continues", head + "continues:\n---\n" + pipe("One"), 4, "continues"},
		{"long project", head + "project: " + repeat("p", 200) + "\n---\n" + pipe("One"), 4, "project"},
		{"too big", ok + "\n" + repeat("x", MaxWorkOrderBytes), 0, "64"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, problems := ParseWorkOrder(tc.text)
			if len(problems) != 1 {
				t.Fatalf("want one problem, got %+v", problems)
			}
			if !reflect.DeepEqual(got, WorkOrder{}) {
				t.Fatalf("a refused order returned %+v", got)
			}
			if problems[0].Line != tc.line || !strings.Contains(strings.ToLower(problems[0].Text), strings.ToLower(tc.has)) {
				t.Fatalf("got %+v, want line %d with %q", problems[0], tc.line, tc.has)
			}
		})
	}
}

func TestWorkOrderReusesValidation(t *testing.T) {
	for _, tc := range []struct {
		n   int
		bad bool
	}{{MaxOutcomePromptBytes + 1, true}, {MaxOutcomePromptBytes, false}} {
		text := "---\nreet: work-order/1\nobjective: Do it\n---\n\n## Pipeline: One\n\n" + strings.Repeat("a", tc.n) + "\n"
		_, problems := ParseWorkOrder(text)
		if (len(problems) != 0) != tc.bad {
			t.Errorf("%d bytes: %+v", tc.n, problems)
		}
	}
}

func TestWorkOrderLimits(t *testing.T) {
	got := WorkOrderLimits()
	want := map[string]int{"pipelines": MaxOutcomeSessions, "objective_bytes": MaxOutcomeObjectiveBytes, "brief_bytes": MaxOutcomePromptBytes, "parts": MaxStageParts, "part_brief_bytes": maxPartBriefBytes, "criteria": store.MaxGafferCriteria, "criterion_bytes": store.MaxGafferCriterionText, "title_runes": terminalproto.MaxTitleRunes}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
}

func TestWorkOrderAccepts(t *testing.T) {
	head := "---\nreet: work-order/1\nobjective: Do it\n---\n\n## Pipeline: One\n\n"
	for name, brief := range map[string]string{
		"level 4":  "Brief\n\n#### Context\n\nmore",
		"in fence": "Brief\n```\n## Not a heading\n---\n```\nmore",
		"tilde":    "Brief\n~~~\n```\n## x\n~~~\nmore",
		"long":     "Brief\n````\n```\n## x\n````\nmore",
	} {
		order := mustParseWorkOrder(t, head+brief+"\n")
		if len(order.Sessions) != 1 || order.Sessions[0].Prompt != brief {
			t.Errorf("%s: %+v", name, order.Sessions)
		}
	}
	// BOM and lone carriage returns.
	text := "\ufeff" + strings.ReplaceAll(workOrderExample(t), "\n", "\r")
	if got := mustParseWorkOrder(t, text); !reflect.DeepEqual(got, mustParseWorkOrder(t, workOrderExample(t))) {
		t.Errorf("bom/cr: %+v", got)
	}
	fenced := "Intro.\r\n```markdown\r\n" + strings.ReplaceAll(workOrderExample(t), "\n", "\r\n") + "```\r\nDone.\r\n"
	if got := mustParseWorkOrder(t, fenced); !reflect.DeepEqual(got, mustParseWorkOrder(t, workOrderExample(t))) {
		t.Errorf("fenced crlf: %+v", got)
	}
}

func TestWorkOrderPathwayAndContinues(t *testing.T) {
	pipe := "---\n\n## Pipeline: One\n\nBrief.\n\n### Criteria\n- It works\n"
	order := mustParseWorkOrder(t, "---\nreet: work-order/1\nobjective: Do it\npathway: Ship it\n"+pipe)
	if order.Pathway != "Ship it" || len(order.Sessions) != 1 || len(order.Sessions[0].Stages) != 0 || len(order.Sessions[0].Criteria) != 1 {
		t.Fatalf("pathway: %+v", order)
	}
	id := strings.Repeat("a", 32)
	order = mustParseWorkOrder(t, "---\nreet: work-order/1\ncontinues: "+id+"\n"+pipe)
	if order.Continues != id || order.Objective != "" || len(order.Sessions) != 1 {
		t.Fatalf("continues: %+v", order)
	}
}

func TestWorkOrderJSON(t *testing.T) {
	raw, err := json.Marshal(mustParseWorkOrder(t, workOrderExample(t)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"crew":{"heavy":{"tool":"claude","model":"opus","effort":"high"},"standard":{"tool":"codex"}}`) {
		t.Fatalf("%s", raw)
	}
}
