package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/store"
)

func mcpWorkOrder(header, body string) string {
	return "---\nreet: work-order/1\n" + header + "---\n" + body
}

const mcpOrderBody = "## Pipeline: Export API\n\nAdd the export.\n\n### Criteria\n- It exports CSV\n- It is tested\n"

func mcpOrderArgs(project, order string) map[string]any {
	return map[string]any{"work_order": order, "project_id": project, "agent": "opencode", "confirm": true}
}

func TestMCPChecksAWorkOrder(t *testing.T) {
	t.Parallel()
	s, _, write, _, _ := factoryMCPPathways(t)
	ctx := context.Background()
	out, err := s.executeMCPFactory(ctx, write, "reet_check_work_order", map[string]any{"work_order": mcpWorkOrder("objective: Ship CSV export.\nproject: Billing\ncrew: heavy=claude\n", mcpOrderBody)})
	if err != nil || out["ok"] != true {
		t.Fatalf("check: %v %v", out, err)
	}
	if p, _ := out["problems"].([]gaffer.WorkOrderProblem); p == nil {
		t.Fatalf("problems must be an array: %v", out)
	}
	if !strings.Contains(out["note"].(string), "Over MCP the saved crew decides.") {
		t.Fatalf("note: %v", out["note"])
	}
	plan := out["plan"].([]map[string]any)
	if len(plan) != 1 || plan[0]["title"] != "Export API" || len(plan[0]["stages"].([]map[string]any)) != 6 || plan[0]["criteria"] != 2 {
		t.Fatalf("plan: %+v", plan)
	}
	if out["project_id"] == "" {
		t.Fatalf("project hint: %v", out)
	}
	// A bad order says what is wrong and plans nothing.
	out, err = s.executeMCPFactory(ctx, write, "reet_check_work_order", map[string]any{"work_order": "no order here"})
	if err != nil || out["ok"] != false || len(out["problems"].([]gaffer.WorkOrderProblem)) == 0 {
		t.Fatalf("bad: %v %v", out, err)
	}
	// A pathway that is not on and a continues that is not running are problems.
	out, _ = s.executeMCPFactory(ctx, write, "reet_check_work_order", map[string]any{"work_order": mcpWorkOrder("objective: x\npathway: Nowhere\ncontinues: "+strings.Repeat("a", 32)+"\n", mcpOrderBody)})
	if out["ok"] != false || len(out["problems"].([]gaffer.WorkOrderProblem)) != 2 {
		t.Fatalf("lookups: %v", out)
	}
	// A pathway plan names the pathway's stages.
	out, _ = s.executeMCPFactory(ctx, write, "reet_check_work_order", map[string]any{"work_order": mcpWorkOrder("objective: x\npathway: careful\n", mcpOrderBody)})
	if out["ok"] != true || out["pathway_id"] == "" || out["plan"].([]map[string]any)[0]["stages"].([]map[string]any)[1]["name"] != "Audit" {
		t.Fatalf("pathway plan: %v", out)
	}
}

func TestMCPStartsAWorkOrder(t *testing.T) {
	t.Parallel()
	s, user, write, project, _ := factoryMCPPathways(t)
	ctx := context.Background()
	out, err := s.executeMCPFactory(ctx, write, "reet_start_outcome", mcpOrderArgs(project, mcpWorkOrder("objective: Ship CSV export.\nproject: BILLING\ncrew: heavy=claude\n", mcpOrderBody)))
	if err != nil || out["applied"] != true {
		t.Fatalf("start: %v %v", out, err)
	}
	rows, _ := s.st.GafferOutcomesFor(user.ID)
	o, err := s.st.GafferOutcomeFor(user.ID, rows[0].ID)
	if err != nil || len(o.Sessions) != 1 {
		t.Fatalf("outcome: %+v %v", o, err)
	}
	if !strings.Contains(out["note"].(string), workOrderCrewNote) {
		t.Fatalf("a start must say the crew was not applied: %v", out["note"])
	}
	if o.Objective != "Ship CSV export." || o.Yolo || o.Sessions[0].AutoMerge || stageLabels(o.Sessions[0]) != "code,review,test,pr,babysit,merge" || len(o.Sessions[0].Criteria) != 2 {
		t.Fatalf("started: %+v", o)
	}
}

func TestMCPWorkOrderRefusals(t *testing.T) {
	t.Parallel()
	s, _, write, project, _ := factoryMCPPathways(t)
	ctx := context.Background()
	good := mcpWorkOrder("objective: Ship.\n", mcpOrderBody)
	if _, err := s.executeMCPFactory(ctx, write, "reet_start_outcome", pathwayStartArgs(project, "", nil)); err != nil {
		t.Fatal(err)
	}
	started, _ := s.st.GafferOutcomesFor(write.User.ID)
	id := started[0].ID
	second := runningOutcome(t, s.st, write.User.ID, strings.Repeat("d", 32)).ID
	cases := []struct {
		name, tool string
		args       map[string]any
		want       string
	}{
		{"with pipelines", "reet_start_outcome", map[string]any{"work_order": good, "pipelines": []any{}}, "Give a work order or these arguments, not both. Nothing was started."},
		{"with objective", "reet_start_outcome", map[string]any{"work_order": good, "objective": "x"}, "Give a work order or these arguments, not both. Nothing was started."},
		{"with pathway", "reet_add_pipelines", map[string]any{"work_order": good, "outcome_id": id, "pathway": "x"}, "Give a work order or these arguments, not both. Nothing was started."},
		{"other project", "reet_start_outcome", map[string]any{"work_order": mcpWorkOrder("objective: Ship.\nproject: Other\n", mcpOrderBody)}, "project"},
		{"continues on start", "reet_start_outcome", map[string]any{"work_order": mcpWorkOrder("objective: Ship.\ncontinues: "+id+"\n", mcpOrderBody)}, "reet_add_pipelines"},
		{"continues missing", "reet_add_pipelines", map[string]any{"work_order": mcpWorkOrder("continues: "+strings.Repeat("b", 32)+"\n", mcpOrderBody), "outcome_id": id}, workOrderNotRunning},
		{"continues another running", "reet_add_pipelines", map[string]any{"work_order": mcpWorkOrder("continues: "+second+"\n", mcpOrderBody), "outcome_id": id}, "continues another outcome"},
		{"parse problem", "reet_start_outcome", map[string]any{"work_order": mcpWorkOrder("objective: Ship.\n", "## Pipeline: One\n")}, "Nothing was started."},
	}
	for _, c := range cases {
		args := mcpOrderArgs(project, "")
		delete(args, "work_order")
		for k, v := range c.args {
			args[k] = v
		}
		if c.tool == "reet_add_pipelines" {
			delete(args, "project_id")
			delete(args, "agent")
		}
		_, err := s.executeMCPFactory(ctx, write, c.tool, args)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
	}
	rows, _ := s.st.GafferOutcomesFor(write.User.ID)
	if len(rows) != 2 {
		t.Fatalf("a refusal started %d outcomes", len(rows)-2)
	}
}

func TestMCPAddsAWorkOrderAndKeepsItsRefusals(t *testing.T) {
	t.Parallel()
	s, user, write, project, _ := factoryMCPPathways(t)
	ctx := context.Background()
	if _, err := s.executeMCPFactory(ctx, write, "reet_start_outcome", pathwayStartArgs(project, "", nil)); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.st.GafferOutcomesFor(user.ID)
	id := rows[0].ID
	add := func(header string) (map[string]any, error) {
		return s.executeMCPFactory(ctx, write, "reet_add_pipelines", map[string]any{"outcome_id": id, "confirm": true, "work_order": mcpWorkOrder(header, mcpOrderBody)})
	}
	if _, err := add("continues: " + id + "\n"); err != nil {
		t.Fatal(err)
	}
	o, _ := s.st.GafferOutcomeFor(user.ID, id)
	if len(o.Sessions) != 2 || len(o.Sessions[1].Criteria) != 2 {
		t.Fatalf("added: %+v", o.Sessions)
	}
	// An order with no confirm changes nothing.
	if _, err := s.executeMCPFactory(ctx, write, "reet_add_pipelines", map[string]any{"outcome_id": id, "work_order": mcpWorkOrder("", mcpOrderBody)}); err == nil || !strings.Contains(err.Error(), "confirm=true") {
		t.Fatalf("confirm: %v", err)
	}
	// YOLO is refused as ever.
	// The outcome supervisor writes the outcome in the background, so a
	// revision read here can be stale; the store hands back the current one.
	o, _ = s.st.GafferOutcomeFor(user.ID, id)
	for {
		cur, err := s.st.UpdateGafferOutcomeFor(user.ID, id, o.Revision, func(o *store.GafferOutcome) bool { o.Yolo = true; return true })
		if err == nil {
			break
		}
		if !errors.Is(err, store.ErrGafferOutcomeBusy) || cur.Revision == o.Revision {
			t.Fatal(err)
		}
		o = cur
	}
	if _, err := add(""); err == nil || !strings.Contains(err.Error(), "YOLO") {
		t.Fatalf("yolo: %v", err)
	}
	if _, err := add("continues: " + id + "\n"); err == nil || !strings.Contains(err.Error(), "YOLO") {
		t.Fatalf("yolo, continued: %v", err)
	}
}

func TestMCPWorkOrderContinuesOnAStoreFaultIsNotNotFound(t *testing.T) {
	t.Parallel()
	s, user, write, _, _ := factoryMCPPathways(t)
	ctx := context.Background()
	id := runningOutcome(t, s.st, user.ID, strings.Repeat("d", 32)).ID
	breakOutcome(t, s.st, id)
	order := mcpWorkOrder("continues: "+id+"\n", mcpOrderBody)
	if _, err := s.executeMCPFactory(ctx, write, "reet_add_pipelines", map[string]any{"outcome_id": id, "confirm": true, "work_order": order}); err == nil || err.Error() == workOrderNotRunning {
		t.Fatalf("add: %v", err)
	}
	if out, err := s.executeMCPFactory(ctx, write, "reet_check_work_order", map[string]any{"work_order": order}); err == nil {
		t.Fatalf("check: %v", out)
	}
}

func TestMCPGivesAWorkReport(t *testing.T) {
	t.Parallel()
	s, user, write, project, _ := factoryMCPPathways(t)
	ctx := context.Background()
	if !mcpFactoryReads["reet_get_work_report"] || !mcpFactoryReads["reet_check_work_order"] {
		t.Fatal("the work order tools are Factory reads")
	}
	if !gafferToolReturnsSessionText("reet_get_work_report") || gafferToolReturnsSessionText("reet_check_work_order") {
		t.Fatal("a report carries session text; a check does not")
	}
	if _, err := s.executeMCPFactory(ctx, write, "reet_start_outcome", pathwayStartArgs(project, "", nil)); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.st.GafferOutcomesFor(user.ID)
	if _, err := s.st.UpdateGafferOutcomeFor(user.ID, rows[0].ID, rows[0].Revision, func(o *store.GafferOutcome) bool {
		o.Sessions[0].Stages[0].Summary = "HANDOVER_NOTE"
		o.Sessions[0].Stages[0].State = "done"
		return true
	}); err != nil {
		t.Fatal(err)
	}
	out, err := s.executeMCPFactory(ctx, write, "reet_get_work_report", map[string]any{"outcome_id": rows[0].ID})
	if err != nil || !strings.Contains(out["report"].(string), "Tidy the logs") {
		t.Fatalf("report: %v %v", out, err)
	}
	if !strings.Contains(out["report"].(string), "### Handover") {
		t.Fatalf("a management connection reads handovers: %v", out["report"])
	}
	read := mcpAccess{User: write.User, Connection: store.MCPConnection{ID: 3, Name: "reader", Scope: "read"}}
	out, err = s.executeMCPFactory(ctx, read, "reet_get_work_report", map[string]any{"outcome_id": rows[0].ID})
	if err != nil || strings.Contains(out["report"].(string), "### Handover") || strings.Contains(out["report"].(string), "HANDOVER_NOTE") {
		t.Fatalf("a read connection sees no handover: %v %v", out, err)
	}
}

func TestMCPAddWorkOrderSaysTheCrewWasNotApplied(t *testing.T) {
	t.Parallel()
	s, user, write, project, _ := factoryMCPPathways(t)
	ctx := context.Background()
	if _, err := s.executeMCPFactory(ctx, write, "reet_start_outcome", pathwayStartArgs(project, "", nil)); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.st.GafferOutcomesFor(user.ID)
	out, err := s.executeMCPFactory(ctx, write, "reet_add_pipelines", map[string]any{"outcome_id": rows[0].ID, "confirm": true, "work_order": mcpWorkOrder("objective: More.\ncrew: heavy=claude\n", mcpOrderBody)})
	if err != nil || !strings.Contains(out["note"].(string), workOrderCrewNote) {
		t.Fatalf("add: %v %v", out, err)
	}
}

// Not yours, missing and finished are one answer, so nothing leaks.
func TestMCPAddWorkOrderContinuesGivesOneAnswer(t *testing.T) {
	t.Parallel()
	s, user, write, _, _ := factoryMCPPathways(t)
	ctx := context.Background()
	other, err := s.st.CreateUser("other-wo@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	theirs := runningOutcome(t, s.st, other.ID, strings.Repeat("e", 32)).ID
	mine := runningOutcome(t, s.st, user.ID, strings.Repeat("f", 32))
	if _, err := s.st.UpdateGafferOutcomeFor(user.ID, mine.ID, mine.Revision, func(o *store.GafferOutcome) bool {
		o.State, o.Sessions[0].State = "stopped", "stopped"
		return true
	}); err != nil {
		t.Fatal(err)
	}
	var seen []string
	for _, id := range []string{theirs, strings.Repeat("9", 32), mine.ID} {
		_, err := s.executeMCPFactory(ctx, write, "reet_add_pipelines", map[string]any{"outcome_id": id, "confirm": true, "work_order": mcpWorkOrder("continues: "+id+"\n", mcpOrderBody)})
		if err == nil {
			t.Fatalf("%s: no refusal", id)
		}
		seen = append(seen, err.Error())
	}
	if seen[0] != workOrderNotRunning || seen[0] != seen[1] || seen[1] != seen[2] {
		t.Fatalf("answers differ: %q", seen)
	}
}

// An agent that meets work_order learns where the format is.
func TestMCPWorkOrderToolsNameTheSkill(t *testing.T) {
	const where = "factory-contract skill, served at /eyupio/factory-contract"
	for _, tool := range mcpToolDefinitions {
		text := tool.Description
		if tool.Name == "reet_start_outcome" || tool.Name == "reet_add_pipelines" {
			raw, _ := json.Marshal(tool.InputSchema)
			text = string(raw)
		} else if tool.Name != "reet_check_work_order" {
			continue
		}
		if !strings.Contains(text, where) {
			t.Errorf("%s does not name the skill", tool.Name)
		}
	}
}
