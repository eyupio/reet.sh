package terminalproto

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const LaunchFeature = "launch-v1"
const MaxModelBytes = 100
const MaxDirBytes = 4096
const MaxPromptBytes = 2000
const MaxTools = 8

var launchTools = map[string]bool{"claude": true, "codex": true, "opencode": true, "gemini": true}

// SupportsLaunch reports whether the peer advertised launch-v1 in
// X-Reet-Terminal-Features. Launch fields are never sent without it.
func SupportsLaunch(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == LaunchFeature {
			return true
		}
	}
	return false
}

func ValidTool(id string) bool { return launchTools[id] }

func validTools(tools []string) bool {
	if len(tools) > MaxTools {
		return false
	}
	for i, tool := range tools {
		if !ValidTool(tool) || i > 0 && tools[i-1] >= tool {
			return false
		}
	}
	return true
}

func validModel(model string) bool {
	if model == "" || len(model) > MaxModelBytes {
		return false
	}
	for i := 0; i < len(model); i++ {
		b := model[i]
		alnum := b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
		if !alnum && (i == 0 || !strings.ContainsRune("._:/@+-", rune(b))) {
			return false
		}
	}
	return true
}

func validDir(dir string) bool {
	if !strings.HasPrefix(dir, "/") || len(dir) > MaxDirBytes || !utf8.ValidString(dir) {
		return false
	}
	for _, r := range dir {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validPrompt(prompt string) bool {
	if prompt == "" || len(prompt) > MaxPromptBytes || !utf8.ValidString(prompt) || prompt[0] == '-' {
		return false
	}
	for _, r := range prompt {
		if unicode.IsControl(r) && r != '\n' {
			return false
		}
	}
	return true
}

func (c Create) validLaunch() bool {
	if c.Tool == "" {
		return c.Model == "" && c.Dir == "" && c.Prompt == ""
	}
	return ValidTool(c.Tool) && (c.Model == "" || validModel(c.Model)) && (c.Dir == "" || validDir(c.Dir)) && (c.Prompt == "" || validPrompt(c.Prompt))
}
