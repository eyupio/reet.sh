package terminalhost

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A matched dialog of Claude Code or Codex is also read for its question and
// numbered options, so the controller can show them and the person can pick
// one from there. Only those lines leave the machine, cleaned and bounded;
// everything else on the screen stays as agent_prompt.go says.

var (
	// questionLine is a line a matcher already recognises as the agent's
	// question; parsedTools are the agents whose dialogs are read this far.
	questionLine = regexp.MustCompile(`(?i)\bdo you want to\b|\bwould you like to\b|\ballow command\?`)
	// optionLine is a numbered option once the border is gone: an optional
	// selection mark, the number, a dot and the label.
	optionLine  = regexp.MustCompile(`^(?:([❯›>])\s*)?(\d+)\.\s+(\S.*)$`)
	parsedTools = map[string]bool{"claude": true, "codex": true}
)

// screenLine is one tail line with its border, control and format characters
// gone: col is the column its text starts at, text the rest.
type screenLine struct {
	col  int
	text string
}

func isBorder(r rune) bool { return r >= 0x2500 && r <= 0x257f || r == '|' }

// cleanLine drops every control and format character (C0, C1, bidi controls,
// zero-width characters) and the line/paragraph separators, then the
// box-drawing borders and spaces at both ends.
func cleanLine(raw string) screenLine {
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r == '\t':
			b.WriteRune(' ')
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), r == '\u2028', r == '\u2029', r == utf8.RuneError:
		default:
			b.WriteRune(r)
		}
	}
	s := strings.TrimRightFunc(b.String(), func(r rune) bool { return isBorder(r) || unicode.IsSpace(r) })
	rest := strings.TrimLeftFunc(s, func(r rune) bool { return isBorder(r) || unicode.IsSpace(r) })
	return screenLine{col: utf8.RuneCountInString(s) - utf8.RuneCountInString(rest), text: rest}
}

// normalise collapses runs of white space, so a redraw that pads lines or
// wraps them elsewhere reads the same.
func normalise(s string) string { return strings.Join(strings.Fields(s), " ") }

// capRunes cuts s to max runes, the last of them an ellipsis.
func capRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max-1]) + "…"
}

// questionOnScreen reads the question an agent is waiting on from its screen:
// the line a matcher recognises as the question (joined with the next when it
// wraps), and the numbered options under it with the one selected. It is nil
// unless the screen is a dialog AwaitingAnswer knows, of an agent whose
// dialogs are parsed, and it reads cleanly: one block of options numbered 1
// to n (n at most 9) with exactly one selected.
func questionOnScreen(tool, pane, screen string) *terminalproto.TerminalQuestion {
	if !parsedTools[tool] || !AwaitingAnswer(tool, screen) {
		return nil
	}
	var lines []screenLine
	for _, raw := range strings.Split(promptTail(screen), "\n") {
		if l := cleanLine(raw); l.text != "" {
			lines = append(lines, l)
		}
	}
	// The question is the last line a matcher knows that has an option under it.
	asked := -1
	for i, l := range lines {
		if optionLine.MatchString(l.text) {
			continue
		}
		if !questionLine.MatchString(l.text) {
			continue
		}
		for _, later := range lines[i+1:] {
			if optionLine.MatchString(later.text) {
				asked = i
				break
			}
		}
	}
	if asked < 0 {
		return nil
	}
	q := &terminalproto.TerminalQuestion{Question: lines[asked].text}
	first := asked + 1
	if !strings.HasSuffix(q.Question, "?") && first < len(lines) && !optionLine.MatchString(lines[first].text) && strings.HasSuffix(lines[first].text, "?") {
		q.Question += " " + lines[first].text
	}
	for first < len(lines) && !optionLine.MatchString(lines[first].text) {
		first++
	}
	// One block of options: each option line, and under it the lines that
	// start at or right of its label, which are that label wrapped.
	labelCol, ended, selected := 0, false, 0
	for _, l := range lines[first:] {
		m := optionLine.FindStringSubmatchIndex(l.text)
		switch {
		case m != nil && ended:
			return nil
		case m != nil:
			n, err := strconv.Atoi(l.text[m[4]:m[5]])
			if err != nil || n != len(q.Options)+1 || n > terminalproto.MaxTerminalOptions {
				return nil
			}
			o := terminalproto.TerminalOption{Index: n, Label: l.text[m[6]:], Selected: m[2] >= 0}
			if o.Selected {
				selected++
			}
			q.Options = append(q.Options, o)
			labelCol = l.col + utf8.RuneCountInString(l.text[:m[6]])
		case !ended && l.col >= labelCol:
			last := &q.Options[len(q.Options)-1]
			last.Label += " " + l.text
		default:
			ended = true
		}
	}
	if len(q.Options) == 0 || selected != 1 {
		return nil
	}
	q.Question = capRunes(normalise(q.Question), terminalproto.MaxTerminalQuestionRunes)
	sum := sha256.New()
	sum.Write([]byte(pane + "\x00" + q.Question))
	for i := range q.Options {
		q.Options[i].Label = capRunes(normalise(q.Options[i].Label), terminalproto.MaxTerminalOptionRunes)
		// The selection is left out: moving it does not change the question,
		// and the keys are worked out from a fresh read anyway.
		sum.Write([]byte("\x00" + strconv.Itoa(q.Options[i].Index) + "\x1f" + q.Options[i].Label))
	}
	q.Fingerprint = hex.EncodeToString(sum.Sum(nil))
	if !q.Valid() {
		return nil
	}
	return q
}

// answerKeys are the tmux keys that choose option on q: arrow keys from the
// selected option to it, then Enter. Both agents' dialogs move with the
// arrows and confirm with Enter. A digit is never sent: whether a number key
// chooses at once or only moves the selection is the agent's to decide, and
// an Enter after a digit that already chose would land on the next screen.
func answerKeys(q *terminalproto.TerminalQuestion, option int) ([]string, bool) {
	from, found := 0, false
	for _, o := range q.Options {
		if o.Selected {
			from = o.Index
		}
		found = found || o.Index == option
	}
	if !found || from == 0 {
		return nil, false
	}
	var keys []string
	for ; from < option; from++ {
		keys = append(keys, "Down")
	}
	for ; from > option; from-- {
		keys = append(keys, "Up")
	}
	return append(keys, "Enter"), true
}
