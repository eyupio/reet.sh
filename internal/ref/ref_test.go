package ref

import "testing"

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
