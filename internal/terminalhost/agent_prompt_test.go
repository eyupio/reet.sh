package terminalhost

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
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

const claudePlanPrompt = `
╭──────────────────────────────────────────────────────────────╮
│ Ready to code?                                               │
│                                                              │
│ Here is Claude's plan:                                       │
│ ╭──────────────────────────────────────────────────────────╮ │
│ │ 1. Parse the question                                    │ │
│ │ 2. Send it to the controller                             │ │
│ ╰──────────────────────────────────────────────────────────╯ │
│                                                              │
│ Would you like to proceed?                                   │
│                                                              │
│ ❯ 1. Yes, and auto-accept edits                              │
│   2. Yes, and manually approve edits                         │
│   3. No, keep planning                                       │
╰──────────────────────────────────────────────────────────────╯
`

const claudeWrappedOption = `
╭──────────────────────────────────────────────────────────────╮
│ Bash command                                                 │
│                                                              │
│   rm -rf build && make                                       │
│                                                              │
│ Do you want to proceed?                                      │
│ ❯ 1. Yes                                                     │
│   2. Yes, and don't ask again for rm commands in             │
│      /home/me/projects/a-folder-with-a-long-name             │
│   3. No, and tell Claude what to do differently (esc)        │
╰──────────────────────────────────────────────────────────────╯
`

// hostileOption is drawn by a command the agent was asked to run: it tries to
// smuggle terminal escapes, bidi overrides and zero-width characters into
// what the controller shows, and to pass a very long option.
var hostileOption = "\u202eYes\u202c, \u200bdelete \x1b[31meverything\x1b[0m \u0085now " + strings.Repeat("and more ", 40)

var claudeHostile = `
╭──────────────────────────────────────────────────────────────╮
│ Bash command                                                 │
│                                                              │
│   ` + "\x1b[2J./install.sh \u202e--force\u202c" + `                          │
│                                                              │
│ Do you want to ` + "\u2066proceed\u2069?\u200d" + `                                      │
│ ❯ 1. ` + hostileOption + `
│   2. No                                                      │
╰──────────────────────────────────────────────────────────────╯
`

func optionLabels(q *terminalproto.TerminalQuestion) []string {
	var out []string
	for _, o := range q.Options {
		out = append(out, o.Label)
	}
	return out
}

func TestQuestionOnScreenReadsEachAgentsDialog(t *testing.T) {
	for name, tc := range map[string]struct {
		tool, screen, question string
		truncated              bool
		options                []string
		selected               int
	}{
		"a claude command approval": {"claude", claudeCommandPrompt, "Bash command\ngo test ./...\nRun the test suite\nDo you want to proceed?", false,
			[]string{"Yes", "Yes, and don't ask again for go test commands", "No, and tell Claude what to do differently (esc)"}, 1},
		"a claude plan approval": {"claude", claudePlanPrompt, "Ready to code?\nHere is Claude's plan:\n1. Parse the question\n2. Send it to the controller\nWould you like to proceed?", false,
			[]string{"Yes, and auto-accept edits", "Yes, and manually approve edits", "No, keep planning"}, 1},
		"a codex approval": {"codex", codexCommandPrompt, "$ make migrate\nWould you like to run the following command?", false,
			[]string{"Yes, proceed", "Yes, and don't ask again for this command", "No, and tell Codex what to do differently"}, 1},
		"a wrapped long option": {"claude", claudeWrappedOption, "Bash command\nrm -rf build && make\nDo you want to proceed?", false,
			[]string{"Yes", "Yes, and don't ask again for rm commands in /home/me/projects/a-folder-with-a-long-name", "No, and tell Claude what to do differently (esc)"}, 1},
		"the selection moved": {"claude", strings.Replace(strings.Replace(claudeCommandPrompt, "❯ 1. Yes", "  1. Yes", 1), "  3. No", "❯ 3. No", 1), "Bash command\ngo test ./...\nRun the test suite\nDo you want to proceed?", false,
			[]string{"Yes", "Yes, and don't ask again for go test commands", "No, and tell Claude what to do differently (esc)"}, 3},
		// Without its border the start of a Claude dialog cannot be told, so
		// the card says the rest is on the terminal.
		"a claude dialog without a border": {"claude", claudeBorderless, "Do you want to proceed?", true,
			[]string{"Yes", "No"}, 1},
	} {
		q := questionOnScreen(tc.tool, "%3", tc.screen)
		if q == nil {
			t.Errorf("%s: no question read", name)
			continue
		}
		if !q.Valid() {
			t.Errorf("%s: the question read is not valid on the wire: %+v", name, q)
		}
		if q.Question != tc.question || q.Truncated != tc.truncated {
			t.Errorf("%s: question %q truncated %v, want %q truncated %v", name, q.Question, q.Truncated, tc.question, tc.truncated)
		}
		if got := optionLabels(q); strings.Join(got, "|") != strings.Join(tc.options, "|") {
			t.Errorf("%s: options %q, want %q", name, got, tc.options)
		}
		for i, o := range q.Options {
			if o.Index != i+1 || o.Selected != (o.Index == tc.selected) {
				t.Errorf("%s: option %d is %+v, want index %d selected %v", name, i, o, i+1, i+1 == tc.selected)
			}
		}
	}
}

const claudeBorderless = `
 Bash command

   make deploy

 Do you want to proceed?
 ❯ 1. Yes
   2. No
`

// longPlan is a plan approval whose plan has the given steps, each the given
// words long.
func longPlan(steps, words int) string {
	var b strings.Builder
	b.WriteString("╭────────────────────────────────────╮\n│ Ready to code?                     │\n│ Here is Claude's plan:             │\n")
	for i := 1; i <= steps; i++ {
		b.WriteString("│  " + strconv.Itoa(i) + ". Step" + strings.Repeat(" word", words) + "\n")
	}
	b.WriteString("│ Would you like to proceed?         │\n│ ❯ 1. Yes                           │\n│   2. No, keep planning             │\n╰────────────────────────────────────╯\n")
	return b.String()
}

// A dialog whose text does not all fit, or began above the screen, is sent
// from the end nearest the options, cut at a line, and marked truncated.
func TestQuestionOnScreenMarksALongPlanTruncated(t *testing.T) {
	for name, screen := range map[string]string{
		"a plan longer than the screen":        longPlan(20, 1),
		"a plan longer than the character cap": longPlan(4, 30),
	} {
		q := questionOnScreen("claude", "%3", screen)
		if q == nil || !q.Valid() {
			t.Fatalf("%s: %+v", name, q)
		}
		if !q.Truncated {
			t.Errorf("%s: not marked truncated: %q", name, q.Question)
		}
		lines := strings.Split(q.Question, "\n")
		if lines[len(lines)-1] != "Would you like to proceed?" || !regexp.MustCompile(`^\d+\. Step( word)+$`).MatchString(lines[0]) {
			t.Errorf("%s: not cut at a line, from the end: %q", name, q.Question)
		}
		if n := len([]rune(q.Question)); n > terminalproto.MaxTerminalQuestionRunes {
			t.Errorf("%s: %d runes", name, n)
		}
	}
	if q := questionOnScreen("claude", "%3", longPlan(2, 1)); q == nil || q.Truncated || !strings.HasPrefix(q.Question, "Ready to code?\n") {
		t.Errorf("a short plan was cut: %+v", q)
	}
}

// Text an agent's command printed is cleaned of every character that could
// change how the controller shows it, and bounded.
func TestQuestionOnScreenCleansHostileText(t *testing.T) {
	q := questionOnScreen("claude", "%3", claudeHostile)
	if q == nil {
		t.Fatal("no question read")
	}
	if !q.Valid() {
		t.Fatalf("not valid on the wire: %+v", q)
	}
	if q.Question != "Bash command\n[2J./install.sh --force\nDo you want to proceed?" || q.Truncated {
		t.Errorf("question %q", q.Question)
	}
	label := q.Options[0].Label
	if !strings.HasPrefix(label, "Yes, delete [31meverything[0m now and more") {
		t.Errorf("label %q", label)
	}
	if n := len([]rune(label)); n != terminalproto.MaxTerminalOptionRunes || !strings.HasSuffix(label, "…") {
		t.Errorf("a long label was not cut to the cap with a mark: %d runes, %q", n, label)
	}
	if len([]rune(hostileOption)) < 300 || !q.Options[0].Truncated || q.Options[1].Truncated {
		t.Errorf("a cut label is not marked, or a whole one is: %+v", q.Options)
	}
	long := strings.Replace(claudeCommandPrompt, "Do you want to proceed?", "Do you want to "+strings.Repeat("really ", 100)+"proceed?", 1)
	q = questionOnScreen("claude", "%3", long)
	if q == nil || len([]rune(q.Question)) != terminalproto.MaxTerminalQuestionRunes || !q.Truncated || !strings.HasPrefix(q.Question, "…") || !strings.HasSuffix(q.Question, "really proceed?") {
		t.Errorf("a long question was not cut to the cap from its end: %+v", q)
	}
}

// Unless the dialog reads cleanly, nothing is sent and the session behaves as
// before: a bell only.
func TestQuestionOnScreenNeedsConfidence(t *testing.T) {
	tenOptions := "Do you want to proceed?\n❯ 1. a\n"
	for i := 2; i <= 10; i++ {
		tenOptions += "  " + string(rune('0'+i%10)) + ". b\n"
	}
	tenOptions = strings.Replace(tenOptions, "  0. b", "  10. b", 1)
	for name, tc := range map[string]struct{ tool, screen string }{
		"ten options":            {"claude", tenOptions},
		"a repeated number":      {"claude", strings.Replace(claudeCommandPrompt, "3. No", "2. No", 1)},
		"numbers out of order":   {"claude", strings.Replace(claudeCommandPrompt, "3. No", "4. No", 1)},
		"no selected option":     {"codex", strings.Replace(codexCommandPrompt, "› 1.", "  1.", 1)},
		"two selected options":   {"claude", strings.Replace(claudeCommandPrompt, "  2. Yes", "❯ 2. Yes", 1)},
		"no question words":      {"claude", claudeMenu},
		"no options":             {"codex", "  Would you like to run the following command?\n\n  $ make\n\n  Press enter to confirm or esc to cancel\n"},
		"a second list after":    {"claude", claudeCommandPrompt + "\n 1. another\n"},
		"a question not waited":  {"claude", claudeWorking},
		"an agent without rules": {"opencode", opencodePrompt},
		"a shell":                {"", claudeCommandPrompt},
	} {
		if q := questionOnScreen(tc.tool, "%3", tc.screen); q != nil {
			t.Errorf("%s: read %+v", name, q)
		}
	}
}

// The fingerprint names the question, not the drawing: a redraw that moves
// the cursor or pads the lines keeps it; another question, option, command
// or pane does not.
func TestQuestionFingerprint(t *testing.T) {
	base := questionOnScreen("claude", "%3", claudeCommandPrompt)
	if base == nil || !terminalproto.ValidFingerprint(base.Fingerprint) {
		t.Fatalf("no fingerprint: %+v", base)
	}
	padded := strings.ReplaceAll(claudeCommandPrompt, "\n", "   \n")
	// A cursor blinking in the agent's output above the dialog.
	cursor := strings.Replace(claudeCommandPrompt, "tests now.", "tests now.█", 1)
	moved := strings.Replace(strings.Replace(claudeCommandPrompt, "❯ 1. Yes", "  1. Yes", 1), "  2. Yes", "❯ 2. Yes", 1)
	for name, screen := range map[string]string{"trailing spaces": padded, "a cursor blink": cursor, "the selection moved": moved} {
		if q := questionOnScreen("claude", "%3", screen); q == nil || q.Fingerprint != base.Fingerprint {
			t.Errorf("%s changed the fingerprint: %+v", name, q)
		}
	}
	other := strings.Replace(claudeCommandPrompt, "Do you want to proceed?", "Do you want to make this edit?", 1)
	if q := questionOnScreen("claude", "%3", other); q == nil || q.Fingerprint == base.Fingerprint {
		t.Errorf("another question kept the fingerprint: %+v", q)
	}
	option := strings.Replace(claudeCommandPrompt, "go test commands", "go vet commands", 1)
	if q := questionOnScreen("claude", "%3", option); q == nil || q.Fingerprint == base.Fingerprint {
		t.Errorf("another option kept the fingerprint: %+v", q)
	}
	// A status line ticking above the dialog is not part of it.
	tick := func(n string) string {
		return "✻ Working… (" + n + "s · esc to interrupt)\n" + claudeCommandPrompt
	}
	if a, b := questionOnScreen("claude", "%3", tick("12")), questionOnScreen("claude", "%3", tick("13")); a == nil || b == nil || a.Fingerprint != b.Fingerprint || a.Fingerprint != base.Fingerprint {
		t.Errorf("a ticking line above the dialog changed the fingerprint: %+v %+v", a, b)
	}
	// The same question about another command is another question.
	command := strings.Replace(claudeCommandPrompt, "go test ./...", "rm -rf ~/   ", 1)
	if q := questionOnScreen("claude", "%3", command); q == nil || q.Fingerprint == base.Fingerprint {
		t.Errorf("another command kept the fingerprint: %+v", q)
	}
	codex := questionOnScreen("codex", "%3", codexCommandPrompt)
	if q := questionOnScreen("codex", "%3", strings.Replace(codexCommandPrompt, "make migrate", "make drop-db", 1)); q == nil || codex == nil || q.Fingerprint == codex.Fingerprint {
		t.Errorf("another codex command kept the fingerprint: %+v", q)
	}
	if q := questionOnScreen("claude", "%4", claudeCommandPrompt); q == nil || q.Fingerprint == base.Fingerprint {
		t.Errorf("another pane kept the fingerprint: %+v", q)
	}
}

// The keys an answer sends move from the selected option to the chosen one
// and confirm it; digits are never sent (see answerKeys).
func TestAnswerKeys(t *testing.T) {
	q := questionOnScreen("claude", "%3", strings.Replace(strings.Replace(claudeCommandPrompt, "❯ 1. Yes", "  1. Yes", 1), "  2. Yes", "❯ 2. Yes", 1))
	for option, want := range map[int]string{1: "Up Enter", 2: "Enter", 3: "Down Enter"} {
		keys, ok := answerKeys(q, option)
		if !ok || strings.Join(keys, " ") != want {
			t.Errorf("option %d: keys %q, want %q", option, keys, want)
		}
	}
	if _, ok := answerKeys(q, 4); ok {
		t.Error("keys for an option the question does not have")
	}
}

// printedDialog is a dialog's words printed by a command, as tool output,
// with Claude's own input box under it: the agent is not asking anything.
const printedDialog = `
● Here are the notes:
  Do you want to proceed?
  ❯ 1. Yes, run it
    2. No

╭──────────────────────────────────────────────────────────────╮
│ >                                                            │
╰──────────────────────────────────────────────────────────────╯
  ? for shortcuts
`

const codexPrintedDialog = `
• Here are the notes:
  Would you like to run the following command?
  › 1. Yes, run it
    2. No

› Ask Codex to do anything
`

const claudeOptionsOutsideTheBox = `
╭──────────────────────────────────────────────────────────────╮
│ Bash command                                                 │
│   make deploy                                                │
│ Do you want to proceed?                                      │
╰──────────────────────────────────────────────────────────────╯
 ❯ 1. Yes
   2. No
`

// Only a dialog the agent drew is a question: nothing but its own footer may
// follow the options, the selection mark is the agent's own, and a boxed
// dialog has its options inside the box. Text a command printed is not one,
// even when it carries every word of a dialog.
func TestQuestionOnScreenRefusesPrintedDialogs(t *testing.T) {
	for name, tc := range map[string]struct{ tool, screen string }{
		"claude: printed above the input box":        {"claude", printedDialog},
		"claude: printed with an ascii mark":         {"claude", strings.Replace(printedDialog, "  ❯ 1. Yes, run it\n    2. No", "  1. Yes, run it\n  > 2. No", 1)},
		"codex: printed above the input":             {"codex", codexPrintedDialog},
		"claude: a shell prompt under the dialog":    {"claude", claudeCommandPrompt + "me@host:~$ \n"},
		"codex: a shell prompt under the dialog":     {"codex", codexCommandPrompt + "me@host:~$ \n"},
		"claude: an ascii selection mark":            {"claude", strings.Replace(claudeCommandPrompt, "❯ 1. Yes", "> 1. Yes", 1)},
		"codex: an ascii selection mark":             {"codex", strings.Replace(codexCommandPrompt, "› 1.", "> 1.", 1)},
		"codex: claude's selection mark":             {"codex", strings.Replace(codexCommandPrompt, "› 1.", "❯ 1.", 1)},
		"claude: options outside the dialog's box":   {"claude", claudeOptionsOutsideTheBox},
		"claude: a box that never closes":            {"claude", strings.TrimSuffix(strings.TrimRight(claudeCommandPrompt, "\n"), strings.Split(strings.TrimRight(claudeCommandPrompt, "\n"), "\n")[len(strings.Split(strings.TrimRight(claudeCommandPrompt, "\n"), "\n"))-1])},
		"claude: another box opens under the dialog": {"claude", claudeCommandPrompt + claudeIdle},
	} {
		if q := questionOnScreen(tc.tool, "%3", tc.screen); q != nil {
			t.Errorf("%s: read %+v", name, q)
		}
	}
	// The agent's own footer under the options is part of the dialog.
	footed := strings.Replace(claudeBorderless, "   2. No\n", "   2. No\n\n Esc to cancel · Tab to amend\n", 1)
	if q := questionOnScreen("claude", "%3", footed); q == nil {
		t.Error("claude's own footer hid the dialog")
	}
}

// A dialog that heads its box with its own question shows the question once.
func TestQuestionOnScreenDropsARepeatedQuestion(t *testing.T) {
	headed := strings.Replace(claudeCommandPrompt, "│ Bash command                                                 │", "│ Do you want to proceed?                                      │\n│ Bash command                                                 │", 1)
	q := questionOnScreen("claude", "%3", headed)
	if q == nil || q.Question != "Bash command\ngo test ./...\nRun the test suite\nDo you want to proceed?" {
		t.Fatalf("%+v", q)
	}
	for _, o := range questionOnScreen("claude", "%3", claudeCommandPrompt).Options {
		if o.Truncated {
			t.Fatalf("a whole option is marked cut: %+v", o)
		}
	}
}

// Everything the fingerprint covers is hashed whole, before any cap: two
// dialogs whose first option differs only past the 200 characters sent are
// two questions.
func TestQuestionFingerprintCoversWholeOptions(t *testing.T) {
	long := strings.Repeat("and more ", 30)
	a := questionOnScreen("claude", "%3", strings.Replace(claudeCommandPrompt, "❯ 1. Yes", "❯ 1. Yes "+long+"then stop", 1))
	b := questionOnScreen("claude", "%3", strings.Replace(claudeCommandPrompt, "❯ 1. Yes", "❯ 1. Yes "+long+"then wipe the disk", 1))
	if a == nil || b == nil || !a.Options[0].Truncated || a.Options[0].Label != b.Options[0].Label {
		t.Fatalf("the labels sent should be the same cut text: %+v %+v", a, b)
	}
	if a.Fingerprint == b.Fingerprint {
		t.Fatal("options that differ past the cap share a fingerprint")
	}
}
