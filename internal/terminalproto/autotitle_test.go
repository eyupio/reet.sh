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
	if len(f.Prompt) < 30 || len(f.Auto) < 10 {
		t.Fatalf("fixture is too small to trust: %d prompt cases, %d auto cases", len(f.Prompt), len(f.Auto))
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
