package terminalhost

import (
	"context"
	"regexp"
	"strings"
	"time"
)

// An agent in a terminal that stops to ask before it runs a command or edits
// a file does not always ring the terminal bell, so the session looked the
// same as one still working. The host therefore reads the visible screen of an
// agent's session, on this machine, and says only whether it is waiting for an
// answer. The text is matched and dropped: it is never stored, logged or sent.
//
// The rules are deliberately narrow. Each looks only at the last lines of the
// screen, where an agent draws the question it is waiting on, and needs the
// marks of a question the agent itself drew, not just words that an answer or
// a file could contain further up. A screen no rule knows is "not waiting":
// the session then reads as it did before, by its output and its bell.

// promptTailLines is how many of the screen's last non-empty lines a rule
// may look at.
const promptTailLines = 16

// promptScreenBytes bounds the screen text read for one check.
const promptScreenBytes = 64 << 10

type promptRule struct {
	// all must each match somewhere in the tail.
	all []*regexp.Regexp
}

var (
	// A numbered choice with the selection mark on it, as agents draw a
	// question's options: "❯ 1. Yes", "> 2. No".
	chosenOption = regexp.MustCompile(`(?m)^[\s│|]*[❯›>]\s*\d+\.\s+\S`)

	promptRules = map[string][]promptRule{
		// Claude Code asks "Do you want to proceed?" (or to make an edit,
		// create a file…) above numbered options, and marks a menu it waits
		// on with "Esc to cancel".
		"claude": {
			{all: []*regexp.Regexp{chosenOption, regexp.MustCompile(`(?i)\bdo you want to\b|\bwould you like to\b`)}},
			{all: []*regexp.Regexp{chosenOption, regexp.MustCompile(`(?i)\besc to cancel\b`)}},
		},
		// Codex asks to run a command or apply a change and names the keys
		// that answer it.
		"codex": {
			{all: []*regexp.Regexp{regexp.MustCompile(`(?i)\bpress enter to confirm or esc to cancel\b`)}},
			{all: []*regexp.Regexp{chosenOption, regexp.MustCompile(`(?i)\bwould you like to\b|\ballow command\?`)}},
		},
		// OpenCode heads its question "Permission required" and offers to
		// allow or reject.
		"opencode": {
			{all: []*regexp.Regexp{regexp.MustCompile(`(?i)\bpermission required\b`), regexp.MustCompile(`(?i)\ballow\b.*\breject\b|\breject\b.*\ballow\b|\ballow once\b|\ballow always\b`)}},
		},
	}
)

// ReadsPrompts reports whether the host has rules for this agent's questions.
func ReadsPrompts(tool string) bool { return len(promptRules[tool]) > 0 }

// AwaitingAnswer reports whether screen, the visible text of an agent's
// terminal, shows the agent waiting for an answer. It is false for an agent
// with no rules and for a screen no rule matches.
func AwaitingAnswer(tool, screen string) bool {
	rules := promptRules[tool]
	if len(rules) == 0 {
		return false
	}
	tail := promptTail(screen)
	if tail == "" {
		return false
	}
	for _, rule := range rules {
		matched := true
		for _, re := range rule.all {
			if !re.MatchString(tail) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// promptTail is the last promptTailLines non-empty lines of a screen.
func promptTail(screen string) string {
	if len(screen) > promptScreenBytes {
		screen = screen[len(screen)-promptScreenBytes:]
	}
	lines := strings.Split(screen, "\n")
	kept := make([]string, 0, promptTailLines)
	for i := len(lines) - 1; i >= 0 && len(kept) < promptTailLines; i-- {
		line := strings.TrimRight(lines[i], " \t\r")
		if strings.TrimSpace(line) != "" {
			kept = append(kept, line)
		}
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	return strings.Join(kept, "\n")
}

// promptWatch is what the last look at an agent session's screen found.
type promptWatch struct {
	// activity is the window activity, in seconds, the screen was last read at.
	activity int64
	// settled is set once the screen was read after the second of that
	// activity was over: a screen cannot change without activity, so it is
	// not read again until there is some.
	settled bool
	// at is when the screen was last read.
	at time.Time
	// waiting is whether that screen showed a question.
	waiting bool
}

const (
	// promptReadEvery is the least time between two reads of a screen that
	// keeps changing, such as an agent's spinner.
	promptReadEvery = 2 * time.Second
	// promptReadAgain is the least time between any two reads of one screen,
	// however often the sessions are reconciled.
	promptReadAgain = 500 * time.Millisecond
)

// askedOnScreen reports whether the agent in a running session has just put a
// question on its screen: true once, when a look finds a question that the
// look before did not. It reads the visible screen only for a session an
// agent with rules was started in, only while the window's activity says the
// screen may have changed since it was last read, and at most every
// promptReadEvery while it keeps changing. The screen text does not leave
// this function.
func (m *Manager) askedOnScreen(ctx context.Context, id string, s ManagedSession, rows []paneWindow) bool {
	if !ReadsPrompts(s.Tool) || !paneID.MatchString(s.PaneID) {
		return false
	}
	var activity int64
	for _, p := range rows {
		activity = max(activity, p.activity)
	}
	if m.prompts == nil {
		m.prompts = map[string]promptWatch{}
	}
	watch, seen := m.prompts[id]
	now := time.Now()
	changed := activity != watch.activity
	switch {
	case !seen:
	case !changed && watch.settled:
		return false
	case changed && now.Sub(watch.at) < promptReadEvery, now.Sub(watch.at) < promptReadAgain:
		return false
	}
	// tmux counts activity in whole seconds, so more can still be drawn in the
	// second the last output fell in. The screen is settled only once it has
	// been read after that second is over.
	settled := now.Unix() > activity
	data, err := m.command(ctx, promptScreenBytes, "capture-pane", "-p", "-t", s.PaneID)
	if err != nil {
		// A screen that cannot be read says nothing; the next change is read.
		return false
	}
	waiting := AwaitingAnswer(s.Tool, string(data))
	m.prompts[id] = promptWatch{activity: activity, settled: settled, at: now, waiting: waiting}
	return waiting && !watch.waiting
}
