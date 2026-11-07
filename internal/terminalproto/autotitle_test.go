package terminalproto

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

// The same file drives the web tests (web/src/lib/agentLaunch.test.mjs), so the
// controller and the browser cannot drift apart on what a session is called.
type autoTitleFixture struct {
	Prompt []struct {
		Name   string `json:"name"`
		Prompt string `json:"prompt"`
		Want   string `json:"want"`
	} `json:"prompt"`
	Auto []struct {
		Name   string `json:"name"`
		Tool   string `json:"tool"`
		Prompt string `json:"prompt"`
		Dir    string `json:"dir"`
		Want   string `json:"want"`
	} `json:"auto"`
	Origin []struct {
		Name   string `json:"name"`
		Prompt string `json:"prompt"`
		Want   string `json:"want"`
	} `json:"origin"`
}

func loadAutoTitleFixture(t *testing.T) autoTitleFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/autotitle.json")
	if err != nil {
		t.Fatal(err)
	}
	var f autoTitleFixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Prompt) < 30 || len(f.Auto) < 10 || len(f.Origin) < 5 {
		t.Fatalf("fixture is too small to trust: %d prompt cases, %d auto cases, %d origin cases", len(f.Prompt), len(f.Auto), len(f.Origin))
	}
	return f
}

func TestPromptTitleFixture(t *testing.T) {
	for _, c := range loadAutoTitleFixture(t).Prompt {
		if got := PromptTitle(c.Prompt); got != c.Want {
			t.Errorf("%s: PromptTitle(%q) = %q, want %q", c.Name, c.Prompt, got, c.Want)
		}
	}
}

func TestAutoTitleFixture(t *testing.T) {
	for _, c := range loadAutoTitleFixture(t).Auto {
		if got := AutoTitle(c.Tool, c.Prompt, c.Dir); got != c.Want {
			t.Errorf("%s: AutoTitle(%q, %q, %q) = %q, want %q", c.Name, c.Tool, c.Prompt, c.Dir, got, c.Want)
		}
	}
}

// Whatever a person pastes, the name must be one the protocol accepts: a
// refused title would turn a missing name into a failed launch.
func TestAutoTitleIsAlwaysAValidTitle(t *testing.T) {
	nasty := []string{
		"", " ", "\x00", "\xff\xfe", "\u2028\u2029", "\u202e\u200b", strings.Repeat("\n", 50),
		strings.Repeat("a", 5000), strings.Repeat("é ", 3000), "```", "Use the skill.", "Use the  skill.",
		"Hi! Hi! Hi! Hi! Hi! Hi! Hi! Hi!", "Please please please please fix", "? ? ? ?", "....x....", "- - - -",
		"\U0001F600\U0001F600\U0001F600", strings.Repeat("\U0001F600", 100), "a.b.c.d. e.f.g.h.", "Fix\u0301 it",
	}
	f := loadAutoTitleFixture(t)
	for _, c := range f.Prompt {
		nasty = append(nasty, c.Prompt)
	}
	for _, prompt := range nasty {
		if got := PromptTitle(prompt); got != "" && (!ValidTitle(got) || utf8.RuneCountInString(got) > MaxTitleRunes) {
			t.Errorf("PromptTitle(%q) = %q is not a valid title", prompt, got)
		}
		for _, tool := range []string{"", "claude", "antigravity", "not-a-tool"} {
			for _, dir := range []string{"", "/", "/a/b", "/a/" + strings.Repeat("y", 300), "/a/\u200bb", "/a/\xff"} {
				if got := AutoTitle(tool, prompt, dir); !ValidTitle(got) {
					t.Errorf("AutoTitle(%q, %q, %q) = %q is not a valid title", tool, prompt, dir, got)
				}
			}
		}
	}
}

// A new agent in agentOptions must get a display name, or its sessions would
// be titled with the bare id.
func TestEveryLaunchToolHasADisplayName(t *testing.T) {
	for id := range agentOptions {
		if toolNames[id] == "" {
			t.Errorf("tool %q has no display name in toolNames", id)
		}
	}
}

func TestPromptOriginFixture(t *testing.T) {
	for _, c := range loadAutoTitleFixture(t).Origin {
		if got := string(PromptOrigin(c.Prompt)); got != c.Want {
			t.Errorf("%s: PromptOrigin(%q) = %q, want %q", c.Name, c.Prompt, got, c.Want)
		}
	}
}

func TestResolveTitle(t *testing.T) {
	const fixPrompt = "Please fix session end navigation"
	const fixTitle = "Fix session end navigation"
	cases := []struct {
		name                            string
		title, claim, tool, prompt, dir string
		wantTitle                       string
		wantOrigin                      TitleOrigin
	}{
		{"blank title takes the prompt", "", "", "claude", fixPrompt, "/home/dev/app", fixTitle, TitlePrompt},
		{"blank title and prompt falls back", "", "", "claude", "", "/home/dev/app", "Claude Code · app", TitleFallback},
		{"whitespace-only title is blank", " \t ", "", "claude", "", "/home/dev/app", "Claude Code · app", TitleFallback},
		{"whitespace-only title takes the prompt", "   ", "", "claude", fixPrompt, "/home/dev/app", fixTitle, TitlePrompt},
		{"suggested prompt title with a matching claim", fixTitle, "prompt", "claude", fixPrompt, "/home/dev/app", fixTitle, TitlePrompt},
		{"suggested prompt title with a mismatched claim", fixTitle, "fallback", "claude", fixPrompt, "/home/dev/app", fixTitle, TitleManual},
		{"suggested prompt title with no claim", fixTitle, "", "claude", fixPrompt, "/home/dev/app", fixTitle, TitleManual},
		{"suggested fallback title with a matching claim", "Claude Code · app", "fallback", "claude", "hi", "/home/dev/app", "Claude Code · app", TitleFallback},
		{"fallback-form title typed by a person", "Claude Code · app", "manual", "claude", "hi", "/home/dev/app", "Claude Code · app", TitleManual},
		{"any other title is manual", "My own name", "prompt", "claude", fixPrompt, "/home/dev/app", "My own name", TitleManual},
		{"any other title is manual with a fallback claim", "My own name", "fallback", "claude", "", "/home/dev/app", "My own name", TitleManual},
		{"the supplied title is compared exactly", " " + fixTitle, "prompt", "claude", fixPrompt, "/home/dev/app", " " + fixTitle, TitleManual},
	}
	for _, c := range cases {
		gotTitle, gotOrigin := ResolveTitle(c.title, c.claim, c.tool, c.prompt, c.dir)
		if gotTitle != c.wantTitle || gotOrigin != c.wantOrigin {
			t.Errorf("%s: ResolveTitle(%q, %q, %q, %q, %q) = (%q, %q), want (%q, %q)",
				c.name, c.title, c.claim, c.tool, c.prompt, c.dir, gotTitle, gotOrigin, c.wantTitle, c.wantOrigin)
		}
	}
}

func TestValidTitleOrigin(t *testing.T) {
	for _, s := range []string{"manual", "prompt", "fallback"} {
		if !ValidTitleOrigin(s) {
			t.Errorf("ValidTitleOrigin(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "Manual", "auto", " prompt"} {
		if ValidTitleOrigin(s) {
			t.Errorf("ValidTitleOrigin(%q) = true, want false", s)
		}
	}
}
