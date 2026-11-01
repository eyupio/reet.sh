package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

func tsSkill(id, name, slug string, envs ...store.Environment) store.Skill {
	return store.Skill{ID: id, Name: name, Slug: slug, Description: name + " desc", Environments: envs}
}

func tsEnv(host, agent string, seen int64) store.Environment {
	return store.Environment{Host: host, Agent: agent, LastSeen: seen}
}

func TestSkillInvocationIsASingleSafeLine(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain", "pdf", "Use the pdf skill."},
		{"empty", "", ""},
		{"newline", "a\nb", ""},
		{"tab", "a\tb", ""},
		{"escape", "a\x1b[2Jb", ""},
		{"81 runes", strings.Repeat("é", 81), ""},
		{"80 runes", strings.Repeat("é", 80), "Use the " + strings.Repeat("é", 80) + " skill."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := skillInvocation("claude", c.in); got != c.want {
				t.Fatalf("skillInvocation(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestBuildSessionSkillsSplitsNativeOtherAndReet(t *testing.T) {
	skills := []store.Skill{
		tsSkill("s1", "Zeta", "zeta", tsEnv("box", "claude", 10)),
		tsSkill("s2", "alpha", "alpha", tsEnv("box", "agents", 20)),
		tsSkill("s3", "Codex Only", "codex-only", tsEnv("box", "codex", 5)),
	}
	hosted := []store.HostedSkill{{Slug: "mine", Description: "hosted desc"}}
	got := buildSessionSkills("claude", "box", skills, hosted)
	if len(got.Skills) != 4 {
		t.Fatalf("got %d skills: %+v", len(got.Skills), got.Skills)
	}
	order := []string{"s2", "s1", "s3", "reet:mine"}
	for i, id := range order {
		if got.Skills[i].ID != id {
			t.Fatalf("position %d = %q, want %q", i, got.Skills[i].ID, id)
		}
	}
	if s := got.Skills[0]; !s.Native || !s.OnMachine || s.Source != "machine" || s.Agent != "agents" || s.Invocation != "Use the alpha skill." {
		t.Fatalf("agents skill wrong: %+v", s)
	}
	if s := got.Skills[1]; !s.Native || s.Agent != "claude" || s.Invocation != "Use the Zeta skill." {
		t.Fatalf("claude skill wrong: %+v", s)
	}
	if s := got.Skills[2]; s.Native || !s.OnMachine || s.Agent != "codex" || s.Invocation != "" {
		t.Fatalf("codex skill wrong: %+v", s)
	}
	if s := got.Skills[3]; s.Source != "reet" || s.OnMachine || s.Native || s.Invocation != "" || s.Name != "mine" || s.Description != "hosted desc" {
		t.Fatalf("reet skill wrong: %+v", s)
	}
}

func TestBuildSessionSkillsIgnoresOtherMachinesAndMatchesHostCaseInsensitively(t *testing.T) {
	skills := []store.Skill{
		tsSkill("a", "here", "here", tsEnv("BOX", "claude", 1)),
		tsSkill("b", "elsewhere", "elsewhere", tsEnv("other", "claude", 99)),
	}
	got := buildSessionSkills("claude", "box", skills, nil)
	if len(got.Skills) != 1 || got.Skills[0].ID != "a" {
		t.Fatalf("got %+v", got.Skills)
	}
	if got.MachineSeen != 1 {
		t.Fatalf("MachineSeen = %d, want 1", got.MachineSeen)
	}
}

func TestBuildSessionSkillsDoesNotRepeatAHostedSkillThatIsInstalled(t *testing.T) {
	skills := []store.Skill{tsSkill("a", "Mine", "mine", tsEnv("box", "claude", 1))}
	hosted := []store.HostedSkill{{Slug: "mine"}, {Slug: "other"}}
	got := buildSessionSkills("claude", "box", skills, hosted)
	if len(got.Skills) != 2 || got.Skills[0].ID != "a" || got.Skills[1].ID != "reet:other" {
		t.Fatalf("got %+v", got.Skills)
	}
}

func TestBuildSessionSkillsMachineSeenAndEmpty(t *testing.T) {
	skills := []store.Skill{
		tsSkill("a", "a", "a", tsEnv("box", "claude", 5), tsEnv("box", "codex", 40)),
		tsSkill("b", "b", "b", tsEnv("box", "claude", 12), tsEnv("other", "claude", 500)),
	}
	if got := buildSessionSkills("claude", "box", skills, nil); got.MachineSeen != 40 {
		t.Fatalf("MachineSeen = %d, want 40", got.MachineSeen)
	}
	empty := buildSessionSkills("claude", "unscanned", nil, nil)
	if empty.MachineSeen != 0 || len(empty.Skills) != 0 || empty.Skills == nil {
		t.Fatalf("empty = %+v", empty)
	}
	b, err := json.Marshal(empty)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"skills":[]`) {
		t.Fatalf("json = %s", b)
	}
}

func TestBuildSessionSkillsListsAnUnsafeNameWithoutAnInvocation(t *testing.T) {
	skills := []store.Skill{tsSkill("a", "bad\nname", "bad", tsEnv("box", "claude", 1))}
	got := buildSessionSkills("claude", "box", skills, nil)
	if len(got.Skills) != 1 {
		t.Fatalf("got %+v", got.Skills)
	}
	if s := got.Skills[0]; !s.Native || s.Invocation != "" {
		t.Fatalf("got %+v", s)
	}
}
