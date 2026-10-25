package terminalproto

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func launchCreate() Create {
	return Create{Title: "Agent", Cols: 80, Rows: 24, Tool: "claude", Model: "claude-opus-4.1", Dir: "/home/tester/project", Prompt: "fix the bug\nthen test"}
}

func TestLaunchFeatureToken(t *testing.T) {
	for _, value := range []string{"", "other", "launch-v10", "prefix-launch-v1"} {
		if SupportsLaunch(value) {
			t.Fatal("unadvertised extension enabled", value)
		}
	}
	if !SupportsLaunch("command-history-v1, launch-v1") || LaunchFeature != "launch-v1" {
		t.Fatal("feature not recognized")
	}
}

func TestLaunchCreateRoundTripsStrictly(t *testing.T) {
	m := testMessage(launchCreate())
	encoded, err := Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil || decoded.Body.(*Create) == nil || *decoded.Body.(*Create) != launchCreate() {
		t.Fatal(decoded, err)
	}
	unknown := bytes.Replace(encoded, []byte(`"tool":`), []byte(`"shell":"sh","tool":`), 1)
	if _, err = Decode(unknown); err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestPlainCreateOmitsLaunchKeys(t *testing.T) {
	data, _ := json.Marshal(Create{Title: "Shell", Cols: 80, Rows: 24})
	for _, key := range []string{"tool", "model", "dir", "prompt"} {
		if bytes.Contains(data, []byte(`"`+key+`"`)) {
			t.Fatal("plain create carries", key, string(data))
		}
	}
	if (Create{Title: "Shell", Cols: 80, Rows: 24}).Validate() != nil {
		t.Fatal("plain create invalid")
	}
}

func TestCreateStaysComparable(t *testing.T) {
	a, b := launchCreate(), launchCreate()
	if a != b {
		t.Fatal("identical creates differ")
	}
	b.Prompt += "!"
	if a == b {
		t.Fatal("prompt difference not seen")
	}
}

func TestLaunchLimits(t *testing.T) {
	cases := map[string]func(*Create){
		"model leading dot":   func(c *Create) { c.Model = ".x" },
		"model space":         func(c *Create) { c.Model = "a b" },
		"model semicolon":     func(c *Create) { c.Model = "a;b" },
		"model 101 bytes":     func(c *Create) { c.Model = strings.Repeat("a", 101) },
		"dir relative":        func(c *Create) { c.Dir = "project" },
		"dir 4097 bytes":      func(c *Create) { c.Dir = "/" + strings.Repeat("a", 4096) },
		"dir bad utf8":        func(c *Create) { c.Dir = "/home/a\xffb" },
		"dir C1 control":      func(c *Create) { c.Dir = "/home/a\u0085b" },
		"dir NUL":             func(c *Create) { c.Dir = "/home/a\x00b" },
		"dir newline":         func(c *Create) { c.Dir = "/home/a\nb" },
		"prompt 2001 bytes":   func(c *Create) { c.Prompt = strings.Repeat("a", 2001) },
		"prompt bad utf8":     func(c *Create) { c.Prompt = "a\xffb" },
		"prompt tab":          func(c *Create) { c.Prompt = "a\tb" },
		"prompt leading dash": func(c *Create) { c.Prompt = "-rf" },
		"prompt escape":       func(c *Create) { c.Prompt = "a\x1bb" },
		"unknown tool":        func(c *Create) { c.Tool = "bash" },
		"model without tool":  func(c *Create) { *c = Create{Title: "A", Cols: 80, Rows: 24, Model: "m"} },
		"prompt without tool": func(c *Create) { *c = Create{Title: "A", Cols: 80, Rows: 24, Prompt: "hi"} },
		"dir without tool":    func(c *Create) { *c = Create{Title: "A", Cols: 80, Rows: 24, Dir: "/home/a"} },
	}
	for name, mutate := range cases {
		c := launchCreate()
		mutate(&c)
		if c.Validate() == nil {
			t.Error("accepted:", name)
		}
	}
	good := launchCreate()
	good.Model = strings.Repeat("a", 100)
	good.Dir = "/" + strings.Repeat("a", 4095)
	good.Prompt = strings.Repeat("é", 1000)
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"claude", "codex", "opencode", "gemini"} {
		if !ValidTool(tool) {
			t.Fatal(tool)
		}
		c := Create{Title: "A", Cols: 80, Rows: 24, Tool: tool}
		if c.Validate() != nil {
			t.Fatal("tool alone invalid", tool)
		}
	}
	if ValidTool("") || ValidTool("Claude") {
		t.Fatal("bad tool accepted")
	}
}

func TestLaunchCapabilities(t *testing.T) {
	c := testCapabilities()
	legacy, _ := json.Marshal(c)
	if bytes.Contains(legacy, []byte("launch")) || bytes.Contains(legacy, []byte("tools")) {
		t.Fatal("new fields sent to old server", string(legacy))
	}
	c.Launch = true
	if c.Validate() == nil {
		t.Fatal("launch without terminalControl accepted")
	}
	c.TerminalControl = true
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Tools = []string{"claude", "codex"}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, tools := range map[string][]string{
		"duplicate": {"claude", "claude"},
		"unknown":   {"claude", "vim"},
		"unsorted":  {"codex", "claude"},
	} {
		bad := c
		bad.Tools = tools
		if bad.Validate() == nil {
			t.Error("accepted tools:", name)
		}
	}
	noLaunch := c
	noLaunch.Launch = false
	if noLaunch.Validate() == nil {
		t.Fatal("tools without launch accepted")
	}
}

func TestLaunchFailedResultCode(t *testing.T) {
	if LaunchFailed != "launch_failed" || !LaunchFailed.Valid() || ResultCode("launch_fail").Valid() {
		t.Fatal("launch_failed code")
	}
}

func TestPromptNeedsWordsForPositionalPrompts(t *testing.T) {
	for _, tool := range []string{"claude", "codex"} {
		for _, prompt := range []string{"purge", "update", "hello", "fix-it"} {
			if !PromptNeedsWords(tool, prompt) {
				t.Error("single word accepted", tool, prompt)
			}
		}
		for _, prompt := range []string{"", "fix it", "fix\nit", "purge everything"} {
			if PromptNeedsWords(tool, prompt) {
				t.Error("prompt refused", tool, prompt)
			}
		}
	}
	for _, tool := range []string{"opencode", "gemini", "", "vim"} {
		if PromptNeedsWords(tool, "hello") {
			t.Error("flag-form or unknown tool refused a single word", tool)
		}
	}
}
