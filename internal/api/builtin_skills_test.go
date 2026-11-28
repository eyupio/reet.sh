package api

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/gaffer"
)

var contractTopics = []string{"continuing.md", "mcp.md", "pathways.md", "tiers-and-crew.md", "work-order.md", "work-report.md"}

func TestIntegrationSkillServesTopics(t *testing.T) {
	srv := newAccountsSrv(t, "")
	files := func(slug string) []string {
		code, raw := do(t, srv, "GET", "/eyupio/"+slug+"/.well-known/agent-skills/index.json", "")
		var index struct{ Skills []struct{ Files []string } }
		if code != 200 || json.Unmarshal([]byte(raw), &index) != nil || len(index.Skills) != 1 {
			t.Fatalf("%s index: %d %s", slug, code, raw)
		}
		return index.Skills[0].Files
	}
	want := append([]string{"SKILL.md"}, contractTopics...)
	if got := files("factory-contract"); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("factory-contract lists %v, want %v", got, want)
	}
	for _, name := range want {
		if code, body := do(t, srv, "GET", "/eyupio/factory-contract/.well-known/agent-skills/factory-contract/"+name, ""); code != 200 || body == "" {
			t.Errorf("%s: %d", name, code)
		}
	}
	if got := files("reet-babysit"); len(got) != 1 || got[0] != "SKILL.md" {
		t.Fatalf("reet-babysit lists %v", got)
	}
}

func contractTopic(t *testing.T, name string) string {
	t.Helper()
	b, err := builtinSkills.ReadFile("builtin_skills/factory-contract/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestIntegrationSkillExampleParses(t *testing.T) {
	doc := contractTopic(t, "work-order.md")
	_, rest, ok := strings.Cut(doc, "\n```markdown\n")
	block, _, closed := strings.Cut(rest, "\n```\n")
	if !ok || !closed {
		t.Fatal("work-order.md has no fenced markdown block")
	}
	want, err := os.ReadFile("../gaffer/testdata/workorder_example.md")
	if err != nil {
		t.Fatal(err)
	}
	if block+"\n" != string(want) {
		t.Fatalf("the first block of work-order.md differs from the fixture:\n%s", block)
	}
	if _, problems := gaffer.ParseWorkOrder(block); len(problems) > 0 {
		t.Fatalf("the example does not parse: %v", problems)
	}
	entry := contractTopic(t, "SKILL.md")
	_, mini, _ := strings.Cut(entry, "```markdown\n")
	mini, _, _ = strings.Cut(mini, "\n```")
	if _, problems := gaffer.ParseWorkOrder(mini); len(problems) > 0 {
		t.Fatalf("the smallest work order in SKILL.md does not parse: %v", problems)
	}
}

func TestIntegrationSkillLimits(t *testing.T) {
	doc := contractTopic(t, "work-order.md")
	labels := map[string]string{
		"pipelines": "Pipelines in one work order", "objective_bytes": "Objective (bytes)", "brief_bytes": "Pipeline brief (bytes)",
		"parts": "Parts in one split stage (at most)", "part_brief_bytes": "Part brief (bytes)", "criteria": "Criteria in one pipeline",
		"criterion_bytes": "One criterion (bytes)", "title_runes": "Pipeline or part title (characters)",
	}
	limits := gaffer.WorkOrderLimits()
	if len(labels) != len(limits) {
		t.Fatalf("the test knows %d limits, gaffer has %d", len(labels), len(limits))
	}
	for key, n := range limits {
		if row := fmt.Sprintf("| %s | %d |", labels[key], n); !strings.Contains(doc, row) {
			t.Errorf("work-order.md lacks the row %q", row)
		}
	}
}

var topicLink = regexp.MustCompile(`\]\(([a-z-]+\.md)\)`)

func TestIntegrationSkillLinksResolve(t *testing.T) {
	linked := map[string]bool{}
	for _, m := range topicLink.FindAllStringSubmatch(contractTopic(t, "SKILL.md"), -1) {
		if _, err := builtinSkills.ReadFile("builtin_skills/factory-contract/" + m[1]); err != nil {
			t.Errorf("SKILL.md links %s, which is not embedded", m[1])
		}
		linked[m[1]] = true
	}
	for _, name := range contractTopics {
		if !linked[name] {
			t.Errorf("SKILL.md does not link %s", name)
		}
	}
}

func TestIntegrationSkillListsBuiltins(t *testing.T) {
	entries, err := builtinSkills.ReadDir("builtin_skills")
	if err != nil {
		t.Fatal(err)
	}
	entry := contractTopic(t, "SKILL.md")
	for _, e := range entries {
		if e.Name() != "factory-contract" && !strings.Contains(entry, e.Name()) {
			t.Errorf("SKILL.md does not name %s", e.Name())
		}
	}
}

func TestIntegrationSkillEveryWorkOrderShownParses(t *testing.T) {
	for _, name := range append([]string{"SKILL.md"}, contractTopics...) {
		rest := contractTopic(t, name)
		for {
			_, after, ok := strings.Cut(rest, "```markdown\n---\nreet: work-order/")
			if !ok {
				break
			}
			block, tail, _ := strings.Cut(after, "\n```")
			if _, problems := gaffer.ParseWorkOrder("---\nreet: work-order/" + block); len(problems) > 0 {
				t.Errorf("%s: a work order shown there does not parse: %v", name, problems)
			}
			rest = tail
		}
	}
}
