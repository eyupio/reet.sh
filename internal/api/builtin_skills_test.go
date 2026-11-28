package api

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/gaffer"
)

const integrationSlug = "reet-integration"

var integrationTopics = []string{"work-order.md", "work-report.md", "tiers-and-crew.md", "pathways.md", "continuing.md", "mcp.md"}

func TestIntegrationSkillServesTopics(t *testing.T) {
	srv, _ := newAccountsSrvStore(t, "")
	code, raw := do(t, srv, "GET", "/eyupio/"+integrationSlug+"/.well-known/agent-skills/index.json", "")
	if code != 200 {
		t.Fatalf("index: %d %s", code, raw)
	}
	var index struct {
		Skills []struct {
			Files []string `json:"files"`
		} `json:"skills"`
	}
	if err := json.Unmarshal([]byte(raw), &index); err != nil || len(index.Skills) != 1 {
		t.Fatalf("index.json: %v %s", err, raw)
	}
	for _, topic := range append([]string{"SKILL.md"}, integrationTopics...) {
		found := false
		for _, f := range index.Skills[0].Files {
			if f == topic {
				found = true
			}
		}
		if !found {
			t.Errorf("index.json does not list %s: %v", topic, index.Skills[0].Files)
		}
		path := "/eyupio/" + integrationSlug + "/.well-known/agent-skills/" + integrationSlug + "/" + topic
		if topic == "SKILL.md" {
			path = "/eyupio/" + integrationSlug + "/SKILL.md"
		}
		if code, body := do(t, srv, "GET", path, ""); code != 200 || body == "" {
			t.Errorf("GET %s: %d", path, code)
		}
	}

	code, raw = do(t, srv, "GET", "/eyupio/reet-babysit/.well-known/agent-skills/index.json", "")
	if code != 200 {
		t.Fatalf("reet-babysit index: %d %s", code, raw)
	}
	index.Skills = nil
	if err := json.Unmarshal([]byte(raw), &index); err != nil || len(index.Skills) != 1 || len(index.Skills[0].Files) != 1 || index.Skills[0].Files[0] != "SKILL.md" {
		t.Fatalf("reet-babysit should list only SKILL.md: %v %s", err, raw)
	}
}

func readTopic(t *testing.T, name string) string {
	t.Helper()
	b, err := builtinSkills.ReadFile("builtin_skills/" + integrationSlug + "/" + name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

// firstFence returns the content of the first ```-fenced block in s, without
// its fence lines.
func firstFence(t *testing.T, s string) string {
	t.Helper()
	lines := strings.Split(s, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatal("no fenced block found")
	}
	end := -1
	for i := start + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "```" {
			end = i
			break
		}
	}
	if end < 0 {
		t.Fatal("fenced block never closes")
	}
	return strings.Join(lines[start+1:end], "\n") + "\n"
}

func TestIntegrationSkillExampleParses(t *testing.T) {
	workOrder := readTopic(t, "work-order.md")
	fence := firstFence(t, workOrder)
	wantBytes, err := os.ReadFile("../gaffer/testdata/workorder_example.md")
	if err != nil {
		t.Fatal(err)
	}
	if fence != string(wantBytes) {
		t.Fatalf("work-order.md's first fence does not match workorder_example.md byte for byte:\n%q\nwant:\n%q", fence, string(wantBytes))
	}
	if _, problems := gaffer.ParseWorkOrder(fence); len(problems) != 0 {
		t.Fatalf("the published example does not parse: %v", problems)
	}
}

func TestIntegrationSkillLimits(t *testing.T) {
	workOrder := readTopic(t, "work-order.md")
	for name, value := range gaffer.WorkOrderLimits() {
		if !strings.Contains(workOrder, strconv.Itoa(value)) {
			t.Errorf("work-order.md does not mention %s's enforced figure %d", name, value)
		}
	}
}

var topicLinkRE = regexp.MustCompile(`\]\(([a-z0-9-]+\.md)\)`)

func TestIntegrationSkillLinksResolve(t *testing.T) {
	skill := mustBuiltinContent(t, integrationSlug)
	links := map[string]bool{}
	for _, m := range topicLinkRE.FindAllStringSubmatch(skill, -1) {
		links[m[1]] = true
	}
	for _, topic := range integrationTopics {
		if !links[topic] {
			t.Errorf("SKILL.md does not link to %s", topic)
		}
	}
	for link := range links {
		found := false
		for _, topic := range integrationTopics {
			if link == topic {
				found = true
			}
		}
		if !found {
			t.Errorf("SKILL.md links to %s, which is not an embedded topic file", link)
		}
	}
}

func mustBuiltinContent(t *testing.T, slug string) string {
	t.Helper()
	b, err := builtinSkills.ReadFile("builtin_skills/" + slug + "/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestIntegrationSkillListsBuiltins(t *testing.T) {
	skill := mustBuiltinContent(t, integrationSlug)
	entries, err := builtinSkills.ReadDir("builtin_skills")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == integrationSlug {
			continue
		}
		if !strings.Contains(skill, e.Name()) {
			t.Errorf("SKILL.md does not name the built-in skill %q", e.Name())
		}
	}
}
