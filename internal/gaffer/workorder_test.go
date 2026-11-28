package gaffer

import (
	"os"
	"reflect"
	"strings"
	"testing"
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
		{"six pipelines", head + "---\n" + six.String(), 0, "at most"},
		{"same title", head + "---\n" + pipe("One") + pipe("One"), 0, "own title"},
		{"stage order", head + "---\n\n## Pipeline: One\nstages: code, plan\n\nBrief.\n", 7, "order"},
		{"unknown stage", head + "---\n\n## Pipeline: One\nstages: deploy\n\nBrief.\n", 7, "deploy"},
		{"bad tier", head + "---\n\n## Pipeline: One\nstages: code\ntiers: code=huge\n\nBrief.\n", 8, "huge"},
		{"tier of absent stage", head + "---\n\n## Pipeline: One\nstages: code\ntiers: review=heavy\n\nBrief.\n", 8, "review"},
		{"unsplittable part", ok + "\n### Part: pr / x\n\nBrief.\n", 10, "pr"},
		{"one part", head + "---\n\n## Pipeline: One\nstages: code\n\nBrief.\n\n### Part: code / x\n\nBrief.\n", 11, "two to four"},
		{"unknown section", ok + "\n### Notes\n\nx\n", 0, "Notes"},
		{"other h2", ok + "\n## Something else\n", 0, "Something else"},
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
		{"two orders", ok + "\n---\nreet: work-order/1\nobjective: Again\n---\n" + pipe("Two"), 0, "one work order"},
		{"too big", ok + "\n" + repeat("x", MaxWorkOrderBytes), 0, "64"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, problems := ParseWorkOrder(tc.text)
			if len(problems) != 1 {
				t.Fatalf("want one problem, got %+v", problems)
			}
			if problems[0].Line != tc.line && tc.line != 0 || !strings.Contains(strings.ToLower(problems[0].Text), strings.ToLower(tc.has)) {
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
	if got["pipelines"] != MaxOutcomeSessions || got["brief_bytes"] != MaxOutcomePromptBytes || got["parts"] != MaxStageParts || len(got) != 8 {
		t.Fatalf("%+v", got)
	}
}
