package ref

import "testing"

func TestParseInstallCommand(t *testing.T) {
	want, _ := Parse("anthropics/skills/mcp-builder")
	for _, input := range []string{
		"npx skills add https://github.com/anthropics/skills --skill mcp-builder",
		"  npx skills add anthropics/skills --skill mcp-builder  ",
		`npx skills add "https://github.com/anthropics/skills" --skill "mcp-builder"`,
		"npx -y skills@latest add 'https://github.com/anthropics/skills.git/' --skill 'mcp-builder' --global --yes",
		"npx --yes skills add anthropics/skills --skill=mcp-builder -g",
		"npx\tskills\tadd\tanthropics/skills\t--skill\tmcp-builder",
	} {
		got, err := Parse(input)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", input, got, err, want)
		}
	}
}

func TestParseRejectsInvalidInstallCommand(t *testing.T) {
	for _, input := range []string{
		"npx",
		"npx skills add anthropics/skills",
		"npx skills add anthropics/skills --skill",
		"npx skills remove anthropics/skills --skill mcp-builder",
		"npx other add anthropics/skills --skill mcp-builder",
		"npx skills add https://example.com/anthropics/skills --skill mcp-builder",
		"npx skills add https://github.com/anthropics/skills/tree/main --skill mcp-builder",
		"npx skills add https://github.com/anthropics/skills?x=1 --skill mcp-builder",
		"npx skills add anthropics/skills --skill '*'",
		"npx skills add anthropics/skills --skill ../mcp-builder",
		"npx skills add anthropics/skills --skill mcp-builder another-skill",
		"npx skills add anthropics/skills --skill mcp-builder --skill pptx",
		"npx skills add anthropics/skills --skill 'mcp-builder",
		"npx skills add anthropics/skills --skill mcp-builder && echo hello",
		"npx skills add anthropics/skills --skill $(echo mcp-builder)",
		"npx skills add anthropics/skills --skill `whoami`",
	} {
		if got, err := Parse(input); err == nil {
			t.Errorf("Parse(%q) = %+v; want error", input, got)
		}
	}
}

func TestParseGithubURL(t *testing.T) {
	r, err := Parse("https://www.skills.sh/vercel-labs/skills/find-skills")
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "vercel-labs/skills/find-skills" || r.SourceType != "github" ||
		r.Source != "vercel-labs/skills" || r.Slug != "find-skills" {
		t.Fatalf("unexpected ref: %+v", r)
	}
	if r.URL != "https://www.skills.sh/vercel-labs/skills/find-skills" {
		t.Fatalf("unexpected url: %s", r.URL)
	}
}

func TestParseVariants(t *testing.T) {
	want := "vercel-labs/skills/find-skills"
	for _, in := range []string{
		"https://www.skills.sh/vercel-labs/skills/find-skills/",
		"https://skills.sh/vercel-labs/skills/find-skills?x=1",
		"https://www.skills.sh/vercel-labs/skills/find-skills#h",
		"https://www.skills.sh/vercel-labs/skills/find-skills/security/socket",
		"vercel-labs/skills/find-skills",
		"skills.sh/vercel-labs/skills/find-skills",
		"  www.skills.sh/vercel-labs/skills/find-skills  ",
	} {
		r, err := Parse(in)
		if err != nil || r.ID != want {
			t.Errorf("Parse(%q) = %+v, %v; want id %s", in, r, err, want)
		}
	}
}

func TestParseWellKnown(t *testing.T) {
	for _, in := range []string{
		"https://www.skills.sh/site/open.feishu.cn/lark-doc",
		"open.feishu.cn/lark-doc",
	} {
		r, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if r.ID != "open.feishu.cn/lark-doc" || r.SourceType != "well-known" ||
			r.URL != "https://www.skills.sh/site/open.feishu.cn/lark-doc" {
			t.Errorf("Parse(%q) = %+v", in, r)
		}
	}
}

func TestParseRejects(t *testing.T) {
	for _, in := range []string{
		"https://github.com/vercel-labs/skills/find-skills",
		"",
		"find-skills",
		"owner/skill",
	} {
		if r, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) = %+v; want error", in, r)
		}
	}
}

func TestParseHostedInstallCommand(t *testing.T) {
	for _, in := range []string{"https://reet.sh/alice/demo", "npx skills add https://reet.sh/alice/demo", `npx -y skills add "https://reet.sh/alice/demo" --skill demo --yes`} {
		r, err := ParseAt(in, "reet.sh")
		if err != nil || r.ID != "hosted/reet.sh/alice/demo" || r.Source != "reet.sh/alice" || r.Slug != "demo" || r.SourceType != "hosted" || r.URL != "https://reet.sh/alice/demo" {
			t.Errorf("ParseAt(%q) = %+v %v", in, r, err)
		}
	}
	for _, in := range []string{"https://other.example/alice/demo", "npx skills add https://reet.sh/alice/demo --skill other", "npx skills add https://reet.sh/alice/demo && echo bad", "https://reet.sh/alice/../demo", "https://reet.sh@other.example/alice/demo"} {
		if r, err := ParseAt(in, "reet.sh"); err == nil {
			t.Errorf("accepted %q: %+v", in, r)
		}
	}
}

// The install command reet copies out offers a skill's own name after its
// slug, because the skills CLI matches on the name. Pasting that command back
// into reet must still find the skill by its slug.
func TestParseAcceptsACommandThatAlsoNamesTheSkill(t *testing.T) {
	for _, input := range []string{
		"npx skills add https://github.com/rknall/claude-skills --skill svg-logo-designer 'SVG Logo Designer'",
		`npx skills add https://github.com/rknall/claude-skills --skill svg-logo-designer "SVG Logo Designer" --global`,
		"npx skills add rknall/claude-skills --skill c 'C++' -y",
	} {
		got, err := Parse(input)
		if err != nil || (got.ID != "rknall/claude-skills/svg-logo-designer" && got.ID != "rknall/claude-skills/c") {
			t.Errorf("%s: %+v %v", input, got, err)
		}
	}
	for _, input := range []string{
		"npx skills add rknall/claude-skills --skill 'SVG Logo Designer'",
		"npx skills add rknall/claude-skills --skill svg-logo-designer SVG Logo Designer -y",
		"npx skills add rknall/claude-skills --skill svg-logo-designer 'SVG Logo Designer",
		"npx skills add rknall/claude-skills --skill svg-logo-designer 'SVG Logo' && echo hello",
		"npx skills add rknall/claude-skills --skill svg-logo-designer --unknown",
		"npx skills add rknall/claude-skills --skill",
	} {
		if got, err := Parse(input); err == nil {
			t.Errorf("%s: accepted as %+v", input, got)
		}
	}
}
