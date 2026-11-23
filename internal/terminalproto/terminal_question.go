package terminalproto

import "strings"

// TerminalQuestionFeature lets a machine send the question an agent in a
// terminal session is waiting on (SessionChanged.TerminalQuestion), and lets a
// controller answer it with one of its options (TerminalAnswer). Only a
// dialog the machine recognised is sent: its question and options, bounded
// below, and never the rest of the screen. A connection without it never
// carries either.
const TerminalQuestionFeature = "terminal-question-v1"

const (
	MaxTerminalQuestionRunes = 500
	MaxTerminalOptions       = 9
	MaxTerminalOptionRunes   = 200
)

// SupportsTerminalQuestion reports whether the peer advertised terminal-question-v1.
func SupportsTerminalQuestion(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == TerminalQuestionFeature {
			return true
		}
	}
	return false
}

// TerminalOption is one numbered answer as the agent draws it. Index is the
// number on screen, 1 to 9; Selected marks the one the agent's cursor is on;
// Truncated says Label is not the whole of what the agent drew.
type TerminalOption struct {
	Index     int    `json:"index"`
	Label     string `json:"label"`
	Selected  bool   `json:"selected,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// TerminalQuestion is the dialog an agent is waiting on. Question is the
// dialog's own text, one line per screen line: its subject (the command, the
// file, the plan's heading) and then the question sentence. Truncated says
// some of the dialog's text is not in Question, because it did not fit or
// started above the visible screen. Fingerprint is the SHA-256, in lower-case
// hex, the machine took of the pane and the whole dialog; an answer must name
// it.
type TerminalQuestion struct {
	Question    string           `json:"question"`
	Truncated   bool             `json:"truncated,omitempty"`
	Options     []TerminalOption `json:"options"`
	Fingerprint string           `json:"fingerprint"`
}

// Valid applies the caps and the plain-text rule: a question of non-blank
// lines joined by single newlines (the only control character allowed), one
// to nine options numbered 1 to 9 without repeats, at most one selected, and
// a fingerprint.
func (q TerminalQuestion) Valid() bool {
	if !plainText(strings.ReplaceAll(q.Question, "\n", " "), MaxTerminalQuestionRunes) {
		return false
	}
	for _, line := range strings.Split(q.Question, "\n") {
		if strings.TrimSpace(line) == "" {
			return false
		}
	}
	if len(q.Options) == 0 || len(q.Options) > MaxTerminalOptions || !ValidFingerprint(q.Fingerprint) {
		return false
	}
	seen, selected := map[int]bool{}, 0
	for _, o := range q.Options {
		validLabel := plainText(o.Label, MaxTerminalOptionRunes) && strings.TrimSpace(o.Label) != ""
		if o.Index < 1 || o.Index > MaxTerminalOptions || seen[o.Index] || !validLabel {
			return false
		}
		seen[o.Index] = true
		if o.Selected {
			selected++
		}
	}
	return selected <= 1
}

// ValidFingerprint accepts a SHA-256 in lower-case hex.
func ValidFingerprint(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// TerminalAnswer asks the machine to choose one option of the question with
// this fingerprint. The machine reads the screen again and works out the
// keys itself; it answers ok, question_changed when the screen no longer
// shows that question, or already_answered when it already chose for it.
type TerminalAnswer struct {
	Fingerprint string `json:"fingerprint"`
	Option      int    `json:"option"`
}

func (TerminalAnswer) kind() string { return "terminal_answer" }
func (a TerminalAnswer) valid() bool {
	return ValidFingerprint(a.Fingerprint) && a.Option >= 1 && a.Option <= MaxTerminalOptions
}
