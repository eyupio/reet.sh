package terminalproto

import (
	"encoding/json"
	"strings"
	"testing"
)

func sampleTerminalQuestion() *TerminalQuestion {
	return &TerminalQuestion{
		Question: "Do you want to proceed?",
		Options: []TerminalOption{
			{Index: 1, Label: "Yes", Selected: true},
			{Index: 2, Label: "No, and tell Claude what to do differently (esc)", Truncated: true},
		},
		Fingerprint: strings.Repeat("ab", 32),
	}
}

func questionChange(q *TerminalQuestion) Message {
	return Message{Version: Version, RequestID: "req_1234567890123456", SessionID: "session_1234567890", Generation: 1,
		Body: SessionChanged{State: Running, TerminalQuestion: q}}
}

// A question rides on a session change only to a controller that negotiated
// terminal-question-v1: an older controller's strict decoder would refuse it.
func TestTerminalQuestionOnlyWhenNegotiated(t *testing.T) {
	asked := sampleTerminalQuestion()
	asked.Question, asked.Truncated = "Bash command\ngo test ./...\nDo you want to proceed?", true
	changed := SessionChanged{State: Running, TerminalQuestion: asked}
	raw, err := json.Marshal(changed.ForFeatures(AttentionFeature))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "terminalQuestion") || strings.Contains(string(raw), "proceed") {
		t.Fatalf("unnegotiated change carried the question: %s", raw)
	}
	with := changed.ForFeatures(AttentionFeature + ", " + TerminalQuestionFeature)
	encoded, err := Encode(questionChange(with.TerminalQuestion))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	got := decoded.Body.(*SessionChanged).TerminalQuestion
	if got != nil && (got.Options[0].Truncated || !got.Options[1].Truncated) {
		t.Fatalf("the options' cut marks did not survive the round trip: %+v", got.Options)
	}
	if got == nil || got.Question != "Bash command\ngo test ./...\nDo you want to proceed?" || !got.Truncated || len(got.Options) != 2 || !got.Options[0].Selected || got.Fingerprint != strings.Repeat("ab", 32) {
		t.Fatalf("round trip lost the question: %+v", got)
	}
	if !SupportsTerminalQuestion("x,"+TerminalQuestionFeature) || SupportsTerminalQuestion(AttentionFeature) {
		t.Fatal("SupportsTerminalQuestion does not read the header")
	}
}

// The controller re-applies every cap when it decodes a machine's update.
func TestTerminalQuestionValidation(t *testing.T) {
	long := func(n int) string { return strings.Repeat("x", n) }
	nine := make([]TerminalOption, 9)
	for i := range nine {
		nine[i] = TerminalOption{Index: i + 1, Label: "o"}
	}
	ten := append(append([]TerminalOption{}, nine...), TerminalOption{Index: 10, Label: "o"})
	for name, tc := range map[string]struct {
		edit func(q *TerminalQuestion)
		ok   bool
	}{
		"the sample":                {func(q *TerminalQuestion) {}, true},
		"a 500 rune question":       {func(q *TerminalQuestion) { q.Question = long(500) }, true},
		"a 501 rune question":       {func(q *TerminalQuestion) { q.Question = long(501) }, false},
		"a blank question":          {func(q *TerminalQuestion) { q.Question = "  " }, false},
		"a subject on its own line": {func(q *TerminalQuestion) { q.Question = "Bash command\ngo test ./...\nDo you want to proceed?" }, true},
		"500 runes with newlines":   {func(q *TerminalQuestion) { q.Question = long(249) + "\n" + long(250) }, true},
		"501 runes with newlines":   {func(q *TerminalQuestion) { q.Question = long(250) + "\n" + long(250) }, false},
		"an empty line":             {func(q *TerminalQuestion) { q.Question = "a\n\nb" }, false},
		"a trailing newline":        {func(q *TerminalQuestion) { q.Question = "a?\n" }, false},
		"a carriage return":         {func(q *TerminalQuestion) { q.Question = "a\r\nb" }, false},
		"a tab":                     {func(q *TerminalQuestion) { q.Question = "a\tb" }, false},
		"a newline in an option":    {func(q *TerminalQuestion) { q.Options[0].Label = "Yes\nNo" }, false},
		"a line separator":          {func(q *TerminalQuestion) { q.Question = "a\u2028b" }, false},
		"an option marked cut":      {func(q *TerminalQuestion) { q.Options[1].Truncated = true }, true},
		"marked truncated":          {func(q *TerminalQuestion) { q.Truncated = true }, true},
		"a control character":       {func(q *TerminalQuestion) { q.Question = "Do\x1b[2J you" }, false},
		"a bidi override":           {func(q *TerminalQuestion) { q.Options[0].Label = "Yes\u202e" }, false},
		"a zero width space":        {func(q *TerminalQuestion) { q.Options[0].Label = "Y\u200bes" }, false},
		"a 200 rune option":         {func(q *TerminalQuestion) { q.Options[0].Label = long(200) }, true},
		"a 201 rune option":         {func(q *TerminalQuestion) { q.Options[0].Label = long(201) }, false},
		"a blank option":            {func(q *TerminalQuestion) { q.Options[1].Label = "" }, false},
		"no options":                {func(q *TerminalQuestion) { q.Options = nil }, false},
		"nine options":              {func(q *TerminalQuestion) { q.Options = nine }, true},
		"ten options":               {func(q *TerminalQuestion) { q.Options = ten }, false},
		"a duplicate number":        {func(q *TerminalQuestion) { q.Options[1].Index = 1 }, false},
		"a number out of range":     {func(q *TerminalQuestion) { q.Options[1].Index = 0 }, false},
		"two selected":              {func(q *TerminalQuestion) { q.Options[1].Selected = true }, false},
		"a short fingerprint":       {func(q *TerminalQuestion) { q.Fingerprint = "abcd" }, false},
		"an upper fingerprint":      {func(q *TerminalQuestion) { q.Fingerprint = strings.Repeat("AB", 32) }, false},
	} {
		q := sampleTerminalQuestion()
		tc.edit(q)
		_, err := Encode(questionChange(q))
		if (err == nil) != tc.ok {
			t.Errorf("%s: encode error %v, want ok %v", name, err, tc.ok)
		}
	}
}

// An answer names the question by its fingerprint and one option; it is a
// session's message, and its refusals are codes of their own.
func TestTerminalAnswerMessage(t *testing.T) {
	m := Message{Version: Version, RequestID: "req_1234567890123456", SessionID: "session_1234567890", Generation: 1,
		Body: TerminalAnswer{Fingerprint: strings.Repeat("0f", 32), Option: 3}}
	encoded, err := Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if a := decoded.Body.(*TerminalAnswer); a.Option != 3 || a.Fingerprint != strings.Repeat("0f", 32) {
		t.Fatalf("round trip: %+v", a)
	}
	for _, bad := range []TerminalAnswer{{Fingerprint: strings.Repeat("0f", 32), Option: 0}, {Fingerprint: strings.Repeat("0f", 32), Option: 10}, {Fingerprint: "nope", Option: 1}} {
		m.Body = bad
		if _, err := Encode(m); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	m.Body = TerminalAnswer{Fingerprint: strings.Repeat("0f", 32), Option: 1}
	m.SessionID = ""
	if _, err := Encode(m); err == nil {
		t.Error("an answer without a session was accepted")
	}
	for _, code := range []ResultCode{QuestionChanged, AlreadyAnswered} {
		if !(Result{Code: code}).valid() {
			t.Errorf("%s is not a valid result", code)
		}
	}
}

func TestTerminalQuestionCapabilityNeedsFeatureAndControl(t *testing.T) {
	c := Capabilities{ProtocolVersion: Version, OS: "linux", OSUser: "me", TerminalEnabled: true, Readiness: Ready, TerminalQuestion: true}
	if c.Validate() == nil {
		t.Fatal("terminal questions accepted without terminal control")
	}
	c.TerminalControl = true
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.ValidateFor(AttentionFeature) == nil {
		t.Fatal("terminal questions accepted without the feature")
	}
	raw, _ := json.Marshal(Capabilities{ProtocolVersion: 1})
	if strings.Contains(string(raw), "terminalQuestion") {
		t.Fatalf("unset capability encoded: %s", raw)
	}
}
