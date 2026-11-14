package terminalhost

import (
	"strings"
	"testing"
)

// The screens below are written for these tests, in the shape each agent
// draws its question. They are not captures of a signed-in run.

const claudeCommandPrompt = `
● I'll run the tests now.

╭──────────────────────────────────────────────────────────────╮
│ Bash command                                                 │
│                                                              │
│   go test ./...                                              │
│   Run the test suite                                         │
│                                                              │
│ Do you want to proceed?                                      │
│ ❯ 1. Yes                                                     │
│   2. Yes, and don't ask again for go test commands           │
│   3. No, and tell Claude what to do differently (esc)        │
╰──────────────────────────────────────────────────────────────╯
`

const claudeWorking = `
● I'll run the tests now.

✻ Running tests… (12s · esc to interrupt)

╭──────────────────────────────────────────────────────────────╮
│ >                                                            │
╰──────────────────────────────────────────────────────────────╯
  ? for shortcuts
`

const claudeIdle = `
● Done. All 42 tests pass.

╭──────────────────────────────────────────────────────────────╮
│ > Try "fix the lint errors"                                  │
╰──────────────────────────────────────────────────────────────╯
  ? for shortcuts
`

const claudeMenu = `
 Select model

 ❯ 1. Default (recommended)
   2. Opus
   3. Haiku

 Enter to confirm · Esc to cancel
`

const codexCommandPrompt = `
• I need to run the migration.

  Would you like to run the following command?

  $ make migrate

› 1. Yes, proceed
  2. Yes, and don't ask again for this command
  3. No, and tell Codex what to do differently

  Press enter to confirm or esc to cancel
`

const codexWorking = `
• Working (12s • esc to interrupt)

› Ask Codex to do anything
`

const opencodePrompt = `
  △ Permission required

  bash: rm -rf build

  Allow once   Allow always   Reject
`

func TestAwaitingAnswerKnowsEachAgentsQuestion(t *testing.T) {
	for name, tc := range map[string]struct {
		tool, screen string
		want         bool
	}{
		"claude asks before a command": {"claude", claudeCommandPrompt, true},
		"claude's own menu":            {"claude", claudeMenu, true},
		"claude working":               {"claude", claudeWorking, false},
		"claude idle":                  {"claude", claudeIdle, false},
		"codex asks before a command":  {"codex", codexCommandPrompt, true},
		"codex working":                {"codex", codexWorking, false},
		"opencode asks":                {"opencode", opencodePrompt, true},
		"an empty screen":              {"claude", "\n\n   \n", false},
		// Each agent's rules are its own.
		"claude's question on a codex screen":    {"codex", claudeMenu, false},
		"opencode's question on a claude screen": {"claude", opencodePrompt, false},
		// An agent with no rules is never said to be waiting, and neither is a shell.
		"an agent with no rules": {"cline", claudeCommandPrompt, false},
		"a shell":                {"", claudeCommandPrompt, false},
	} {
		if got := AwaitingAnswer(tc.tool, tc.screen); got != tc.want {
			t.Errorf("%s: AwaitingAnswer = %v, want %v", name, got, tc.want)
		}
	}
	if !ReadsPrompts("claude") || !ReadsPrompts("codex") || !ReadsPrompts("opencode") || ReadsPrompts("cline") || ReadsPrompts("") {
		t.Fatal("ReadsPrompts does not follow the rules table")
	}
}

// Words alone are not a question: an answer, a file or a quoted log can
// contain them. A rule needs the marks of a question the agent drew.
func TestAwaitingAnswerNeedsMoreThanTheWords(t *testing.T) {
	for name, screen := range map[string]string{
		"the phrase in an answer":   "● The script prints \"Do you want to proceed?\" and waits.\n\n╭────╮\n│ >  │\n╰────╯\n  ? for shortcuts\n",
		"a numbered list in prose":  "● Options:\n  1. Yes\n  2. No\n\n╭────╮\n│ >  │\n╰────╯\n",
		"a shell prompt with a dot": "> 1.2.3 is the version\n",
	} {
		if AwaitingAnswer("claude", screen) {
			t.Errorf("%s was taken for a question", name)
		}
	}
	if AwaitingAnswer("opencode", "The docs say: Permission required for bash.\n") {
		t.Error("the heading alone was taken for OpenCode's question")
	}
}

// Only the last lines count: a question that was answered and has scrolled up
// must not keep the session waiting.
func TestAwaitingAnswerLooksOnlyAtTheLastLines(t *testing.T) {
	later := strings.Repeat("● a line of the answer that came after\n", promptTailLines)
	if AwaitingAnswer("claude", claudeCommandPrompt+later+claudeIdle) {
		t.Fatal("a question that scrolled up still counts")
	}
	// Blank padding under the question does not push it out of view.
	if !AwaitingAnswer("claude", claudeCommandPrompt+strings.Repeat("\n", 40)) {
		t.Fatal("blank lines under the question hid it")
	}
	// A very long screen is bounded, and its end is what is read.
	if !AwaitingAnswer("codex", strings.Repeat("x", 2*promptScreenBytes)+"\n"+codexCommandPrompt) {
		t.Fatal("the end of a long screen was not read")
	}
}
