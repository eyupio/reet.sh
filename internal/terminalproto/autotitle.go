package terminalproto

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Automatic session names. A session started without a title is named here, in
// the controller, so every way of starting one (the browser, the API, a project
// run, a saved draft) gets the same name for the same request. The browser
// mirrors these rules (web/src/lib/agentLaunch.ts: promptTitle and titleFor)
// so a suggestion shown before Start is what the controller would choose; both
// are checked against testdata/autotitle.json. Nothing here calls out to a
// model or the network: the name is a tidy-up of what the person wrote, or the
// agent and folder when they wrote nothing.

const (
	// titleCap is how long a name taken from a prompt may be before it is cut
	// at a word. MaxTitleRunes stays the most the protocol accepts.
	titleCap = 60
	// titleMinWord is how early in the title a space may end it: a title cut at
	// "ab" would say nothing.
	titleMinWord = 20
	// sentenceMinHead is the shortest start of a prompt that a full stop,
	// question mark or exclamation mark may end.
	sentenceMinHead = 8
	// titleTrail is what is trimmed from the end of a name.
	titleTrail = " ,:;.!?-–—"
)

// toolNames are the agents' display names, as the browser's catalogue shows
// them. A test keeps every launchable agent listed.
var toolNames = map[string]string{
	"claude":      "Claude Code",
	"codex":       "Codex",
	"opencode":    "OpenCode",
	"cline":       "Cline",
	"antigravity": "Google Antigravity (AGY)",
}

var (
	// Leading words that ask rather than say: greetings, "please", "can you",
	// "I'd like you to", "help me", "let's". English only; any other language
	// passes through untouched.
	titleFiller = []*regexp.Regexp{
		regexp.MustCompile(`(?i)^(?:hi|hey|hello|hiya|yo)\b(?: (?:claude|codex|opencode|gemini|cline|agy|antigravity|there|team|all|everyone))?[ ,!.:;-]*`),
		regexp.MustCompile(`(?i)^(?:please|pls|kindly)\b[ ,:-]*`),
		regexp.MustCompile(`(?i)^(?:can|could|would|will) you(?: please| kindly)?\b[ ,:-]*`),
		regexp.MustCompile("(?i)^i(?:['’]d| would)? (?:like|love|want|need) (?:you )?to\\b[ ,:-]*"),
		regexp.MustCompile(`(?i)^help me(?: (?:to|with))?\b[ ,:-]*`),
		regexp.MustCompile("(?i)^let['’]?s\\b[ ,:-]*"),
	}
	// The line the skills picker puts in a prompt: not what the work is.
	titleSkillLine = regexp.MustCompile(`(?i)^use the .+ skill\.?$`)
	// A heading, quote or list marker at the start of a line.
	titleMarker = regexp.MustCompile(`^(?:[#>*-]+ )+`)
	// The `Image:` / `Images:` line the Start dialog appends for uploads. It
	// is matched after cleanTitleLine, so tabs are spaces and runs are folded.
	titleImageLead = regexp.MustCompile(`(?i)^images?: `)
	// One upload reference: bare, or POSIX single-quoted when the path needs it
	// (internal/api/terminal_upload.go: uploadReference), where a quote inside
	// the path is written '\''. Quoted references may contain spaces.
	titleImageToken = regexp.MustCompile(`(?:\\'|[^ ']|'[^']*')+`)
	titleImageExt   = regexp.MustCompile(`(?i)\.(?:png|jpe?g|gif|webp)'?$`)
)

// isImageLine reports whether a cleaned line is only `Image:` / `Images:`
// followed by image file references.
func isImageLine(line string) bool {
	loc := titleImageLead.FindStringIndex(line)
	if loc == nil {
		return false
	}
	rest := line[loc[1]:]
	tokens := titleImageToken.FindAllString(rest, -1)
	if len(tokens) == 0 || strings.Join(tokens, " ") != rest {
		return false
	}
	for _, t := range tokens {
		if !titleImageExt.MatchString(t) {
			return false
		}
	}
	return true
}

// PromptTitle is a short name for the work a prompt asks for, or "" when the
// prompt says nothing a name could be made from. It takes the first line that
// says something, without greetings or "please", up to the end of its first
// sentence, and at most 60 characters (cut at a word, with an ellipsis). A
// final `Image:` / `Images:` line of image paths is not task text.
func PromptTitle(prompt string) string {
	lines := strings.Split(prompt, "\n")
	// Only the last non-empty line counts, so an Image line at the top or in
	// the middle of a prompt is never dropped.
	for i := len(lines) - 1; i >= 0; i-- {
		if line := cleanTitleLine(lines[i]); line != "" {
			if isImageLine(line) {
				lines = lines[:i]
			}
			break
		}
	}
	for _, line := range lines {
		line = cleanTitleLine(line)
		if skipTitleLine(line) {
			continue
		}
		line = titleMarker.ReplaceAllString(line, "")
		line = stripTitleFiller(line)
		text := strings.TrimRight(string(cutSentence([]rune(line))), titleTrail)
		if text == "" {
			continue
		}
		title := capTitle(capitalise(text))
		if !ValidTitle(title) {
			return ""
		}
		return title
	}
	return ""
}

// AutoTitle is the name for a session that was started without one: the
// prompt's own name when it has one, otherwise the agent (or "Terminal" for a
// plain shell) and the last part of the folder, as "Claude Code · app".
func AutoTitle(tool, prompt, dir string) string {
	if title := PromptTitle(prompt); title != "" {
		return title
	}
	name := "Terminal"
	if tool != "" {
		if name = toolNames[tool]; name == "" {
			name = tool
		}
	}
	title := name
	if last := lastFolder(dir); last != "" {
		title = name + " · " + last
	}
	if r := []rune(title); len(r) > MaxTitleRunes {
		title = string(r[:MaxTitleRunes])
	}
	if !ValidTitle(title) {
		title = name
	}
	if !ValidTitle(title) {
		title = "Session"
	}
	return title
}

// TitleOrigin says who owns a session's title: a person (manual), the first
// prompt's own name (prompt), or the generic agent-and-folder name (fallback).
// Only a fallback title may later be replaced from the first Chat message.
type TitleOrigin string

const (
	TitleManual   TitleOrigin = "manual"
	TitlePrompt   TitleOrigin = "prompt"
	TitleFallback TitleOrigin = "fallback"
)

// ValidTitleOrigin reports whether s is exactly one of the three origins.
func ValidTitleOrigin(s string) bool {
	switch TitleOrigin(s) {
	case TitleManual, TitlePrompt, TitleFallback:
		return true
	}
	return false
}

// PromptOrigin is the origin AutoTitle gives a session started with this
// prompt: prompt when the prompt has a name of its own, otherwise fallback.
func PromptOrigin(prompt string) TitleOrigin {
	if PromptTitle(prompt) != "" {
		return TitlePrompt
	}
	return TitleFallback
}

// ResolveTitle settles the title and its origin for a create request. A blank
// title (after trimming) is named by AutoTitle. A supplied title keeps the
// origin the client claims only when it is exactly what AutoTitle would have
// chosen and the claim matches PromptOrigin; any other title, or a missing or
// mismatched claim, is the person's own and manual.
func ResolveTitle(title, claim, tool, prompt, dir string) (string, TitleOrigin) {
	auto := AutoTitle(tool, prompt, dir)
	origin := PromptOrigin(prompt)
	if strings.TrimSpace(title) == "" {
		return auto, origin
	}
	if title == auto && (claim == string(TitlePrompt) || claim == string(TitleFallback)) && claim == string(origin) {
		return title, origin
	}
	return title, TitleManual
}

// cleanTitleLine turns tabs into spaces, drops control and format characters
// and the two Unicode line separators, and folds runs of spaces into one.
func cleanTitleLine(line string) string {
	var b strings.Builder
	lastSpace := true
	for _, r := range line {
		if r == '\t' {
			r = ' '
		}
		if r != ' ' && (unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029') {
			continue
		}
		if r == ' ' {
			if lastSpace {
				continue
			}
			lastSpace = true
		} else {
			lastSpace = false
		}
		b.WriteRune(r)
	}
	return strings.TrimRight(b.String(), " ")
}

// skipTitleLine reports whether a line cannot name the work: empty, a code
// fence, a skills-picker line, or nothing but punctuation.
func skipTitleLine(line string) bool {
	if line == "" || strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") || titleSkillLine.MatchString(line) {
		return true
	}
	return !strings.ContainsFunc(line, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) })
}

func stripTitleFiller(line string) string {
	for range 8 {
		changed := false
		for _, re := range titleFiller {
			if loc := re.FindStringIndex(line); loc != nil && loc[1] > 0 {
				line = line[loc[1]:]
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return line
}

// cutSentence ends the text at its first sentence. A full stop only ends one
// when it follows a word of three or more characters with no full stop inside
// it ("e.g." does not), and no sentence may be shorter than sentenceMinHead.
func cutSentence(r []rune) []rune {
	tokenStart := 0
	for i, c := range r {
		if c == ' ' {
			tokenStart = i + 1
			continue
		}
		if c != '.' && c != '!' && c != '?' {
			continue
		}
		if (i+1 < len(r) && r[i+1] != ' ') || i < sentenceMinHead {
			continue
		}
		if c == '.' {
			if token := r[tokenStart:i]; len(token) < 3 || slices.Contains(token, '.') {
				continue
			}
		}
		return r[:i]
	}
	return r
}

// capitalise upper-cases an ASCII first letter. Other scripts are left alone
// on purpose: the browser and the controller must agree to the character.
func capitalise(s string) string {
	if s != "" && s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0]-'a'+'A') + s[1:]
	}
	return s
}

// capTitle keeps a name to titleCap characters, cutting at the last space (not
// earlier than titleMinWord) and adding an ellipsis, or at titleCap when there
// is no such space.
func capTitle(s string) string {
	r := []rune(s)
	if len(r) <= titleCap {
		return s
	}
	cut := titleCap
	for i := titleCap; i >= titleMinWord; i-- {
		if r[i] == ' ' {
			cut = i
			break
		}
	}
	return strings.TrimRight(string(r[:cut]), titleTrail) + "…"
}

// lastFolder is the last part of an absolute folder path, or "" when there is
// none or it holds text a title may not.
func lastFolder(dir string) string {
	parts := strings.Split(dir, "/")
	last := ""
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			last = parts[i]
			break
		}
	}
	if last == "" || !utf8.ValidString(last) {
		return ""
	}
	for _, r := range last {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return ""
		}
	}
	return last
}
