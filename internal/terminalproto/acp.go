package terminalproto

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"
)

// ACPFeature negotiates structured OpenCode chats over the private protocol.
// A controller without it never receives the capability or sends chat bodies.
const ACPFeature = "acp-v1"

// ACPHistoryFeature lets a new controller accept the OpenCode session id and
// resume fields. A slice-1 controller negotiated acp-v1 but its strict decoder
// rejects those fields, so they are never sent without this feature.
const ACPHistoryFeature = "acp-history-v1"
const ACPModelFeature = "acp-model-v1"

func SupportsACPModel(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == ACPModelFeature {
			return true
		}
	}
	return false
}

// ACPImageFeature lets a controller send images with a prompt. The machine
// reads the uploaded files and embeds their bytes in the ACP prompt so the
// agent sees the image itself rather than a file path.
const ACPImageFeature = "acp-image-v1"

// ACPToolDetailsFeature lets a new controller accept the extra fields a tool row shows (command, path, diff and its
// counts, and the duration). A connector on an older controller keeps sending the plain tool event.
const ACPToolDetailsFeature = "acp-tool-details-v1"

// ACPApprovalFeature lets a controller switch a live chat between asking the
// person and answering every permission itself (YOLO). A controller without it
// never sends acp_mode, and a connector without it refuses the request.
const ACPApprovalFeature = "acp-approval-v1"

// ACPQuestionFeature lets a controller answer typed agent questions
// (acp_question_answer) and lets a connector send question events. Questions
// are a different mechanism from approvals: a card for a tool permission never
// becomes a question. A peer without it never sees either, and a connector
// refuses an answer when it was not negotiated.
const ACPQuestionFeature = "acp-question-v1"

// SessionModeFeature lets a controller switch a live session between the
// OpenCode terminal and the structured chat (session_mode). A peer without it
// never sends or receives the body.
const SessionModeFeature = "session-mode-v1"

// SessionModeTimeout bounds one session_mode swap on the machine: stopping
// the old process, starting the other and, on failure, restoring the first.
// The connector enforces it and the controller waits longer than it.
const SessionModeTimeout = 3 * time.Minute

const (
	// MaxChatPromptBytes bounds one chat message. 2,000 bytes is the largest
	// text whose JSON encoding is certain to fit the 16 KiB control frame, even
	// when every byte is a character encoding/json escapes to six bytes.
	MaxChatPromptBytes = 2000
	// MaxACPEventBytes keeps one structured event inside MaxControlBytes after
	// its envelope; a larger tool output is cut and marked truncated.
	MaxACPEventBytes = 10 << 10
	// MaxACPLocations bounds the paths a tool call reports.
	MaxACPLocations = 8
	// MaxACPImages bounds the images one prompt may carry. The machine reads
	// each path from its private upload folder and embeds the bytes.
	MaxACPImages = 8
	// MaxACPImagePathBytes bounds one upload path sent with a prompt.
	MaxACPImagePathBytes = 1024
)

// SupportsACP reports whether the peer advertised acp-v1.
func SupportsACP(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == ACPFeature {
			return true
		}
	}
	return false
}

// SupportsACPToolDetails reports whether the peer negotiated acp-tool-details-v1.
func SupportsACPToolDetails(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == ACPToolDetailsFeature {
			return true
		}
	}
	return false
}

// SupportsACPApproval reports whether the peer negotiated acp-approval-v1.
func SupportsACPApproval(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == ACPApprovalFeature {
			return true
		}
	}
	return false
}

// SupportsSessionMode reports whether the peer negotiated session-mode-v1.
func SupportsSessionMode(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == SessionModeFeature {
			return true
		}
	}
	return false
}

// SupportsACPQuestion reports whether the peer negotiated acp-question-v1.
func SupportsACPQuestion(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == ACPQuestionFeature {
			return true
		}
	}
	return false
}

// SupportsACPHistory reports whether the peer negotiated acp-history-v1.
func SupportsACPHistory(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == ACPHistoryFeature {
			return true
		}
	}
	return false
}

// SupportsACPImages reports whether the peer negotiated acp-image-v1.
func SupportsACPImages(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == ACPImageFeature {
			return true
		}
	}
	return false
}

// ACPEvent is one bounded, structured update from a chat. The machine
// translates ACP updates into these; no raw ACP JSON reaches the browser.
// Kind selects which fields are meaningful:
//
//	user                 one complete user message (Text)
//	agent                one agent message chunk (MessageID, Text)
//	tool                 a tool call state (ToolCallID, Title, ToolKind, Status, Text, Command, Path, Diff,
//	                     Additions, Deletions, DurationMs, Locations)
//	notice               a line for the person (Text, Bad)
//	status               a turn or connection state (State, Detail)
//	permission           an approval card (PermissionID, ToolCallID, Title, ToolKind, Command, Path, Diff, Truncated)
//	permission_resolved  the person's answer (PermissionID, Allow)
//	permission_cancelled the card closed without an answer (PermissionID, Reason)
//	question             a typed agent question (QuestionID, ToolCallID, Question); only with acp-question-v1
//	question_resolved    the person's answer was delivered (QuestionID, Reason is accept or decline)
//	question_cancelled   the question closed without an answer (QuestionID, Reason)
type ACPEvent struct {
	Kind         string   `json:"kind"`
	MessageID    string   `json:"messageId,omitempty"`
	ToolCallID   string   `json:"toolCallId,omitempty"`
	PermissionID string   `json:"permissionId,omitempty"`
	Title        string   `json:"title,omitempty"`
	ToolKind     string   `json:"toolKind,omitempty"`
	Status       string   `json:"status,omitempty"`
	State        string   `json:"state,omitempty"`
	Text         string   `json:"text,omitempty"`
	Command      string   `json:"command,omitempty"`
	Path         string   `json:"path,omitempty"`
	Diff         string   `json:"diff,omitempty"`
	Additions    int      `json:"additions,omitempty"`
	Deletions    int      `json:"deletions,omitempty"`
	DurationMs   int64    `json:"durationMs,omitempty"`
	Locations    []string `json:"locations,omitempty"`
	Allow        *bool    `json:"allow,omitempty"`
	Bad          bool     `json:"bad,omitempty"`
	Truncated    bool     `json:"truncated,omitempty"`
	Reason       string   `json:"reason,omitempty"`
	Detail       string   `json:"detail,omitempty"`
	// QuestionID and Question carry a typed question. They are meaningful only on
	// the question kinds and must be absent on every other kind.
	QuestionID string       `json:"questionId,omitempty"`
	Question   *ACPQuestion `json:"question,omitempty"`
}

func (ACPEvent) kind() string { return "acp_event" }

func (e ACPEvent) valid() bool {
	switch e.Kind {
	case "question", "question_resolved", "question_cancelled":
		return e.validQuestionEvent()
	}
	if e.QuestionID != "" || e.Question != nil {
		return false
	}
	switch e.Kind {
	case "user":
		return e.MessageID == "" && e.ToolCallID == "" && e.PermissionID == "" && e.Allow == nil && validChatText(e.Text, MaxACPEventBytes) && e.Text != ""
	case "agent":
		return validACPID(e.MessageID) && e.ToolCallID == "" && e.PermissionID == "" && e.Allow == nil && validChatText(e.Text, MaxACPEventBytes)
	case "tool":
		return validACPID(e.ToolCallID) && e.PermissionID == "" && e.Allow == nil && validChatText(e.Title, 200) &&
			validToolKind(e.ToolKind) && validToolStatus(e.Status) && validChatText(e.Text, MaxACPEventBytes) &&
			validChatText(e.Command, 6<<10) && validChatText(e.Path, MaxDirBytes) && validChatText(e.Diff, 9<<10) &&
			e.Additions >= 0 && e.Additions <= 1<<20 && e.Deletions >= 0 && e.Deletions <= 1<<20 &&
			e.DurationMs >= 0 && e.DurationMs <= 24*60*60*1000 && validLocations(e.Locations)
	case "notice":
		return e.MessageID == "" && e.ToolCallID == "" && e.PermissionID == "" && e.Allow == nil && e.Text != "" && validChatText(e.Text, MaxACPEventBytes)
	case "status":
		return validACPState(e.State) && validChatText(e.Detail, 500)
	case "permission":
		return validACPID(e.PermissionID) && (e.ToolCallID == "" || validACPID(e.ToolCallID)) &&
			validChatText(e.Title, 200) && (e.ToolKind == "" || validToolKind(e.ToolKind)) &&
			validChatText(e.Command, MaxACPEventBytes) && validChatText(e.Path, MaxDirBytes) &&
			validChatText(e.Diff, MaxACPEventBytes) && validLocations(e.Locations) &&
			(e.Command != "" || e.Path != "" || e.Diff != "")
	case "permission_resolved":
		return validACPID(e.PermissionID) && e.Allow != nil && e.ToolCallID == "" && e.Text == "" && e.Command == "" && e.Path == "" && e.Diff == ""
	case "permission_cancelled":
		return validACPID(e.PermissionID) && e.Allow == nil && validChatText(e.Reason, 100)
	}
	return false
}

// ACPPrompt sends one message to a chat. A chat turn is one prompt; a second
// prompt while it is working is refused. Images lists upload paths the machine
// reads and embeds as ACP image content blocks; it never passes the path to
// the agent. They are sent only when acp-image-v1 was negotiated.
type ACPPrompt struct {
	Text   string   `json:"text"`
	Images []string `json:"images,omitempty"`
}

func (ACPPrompt) kind() string { return "acp_prompt" }
func (p ACPPrompt) valid() bool {
	if !validChatText(p.Text, MaxChatPromptBytes) || !validACPImagePaths(p.Images) {
		return false
	}
	return strings.TrimSpace(p.Text) != "" || len(p.Images) > 0
}

// validACPImagePaths accepts absolute upload paths inside the size bounds. The
// machine additionally checks that every path is inside its own upload roots;
// a controller cannot use this field to read an arbitrary file.
func validACPImagePaths(paths []string) bool {
	if len(paths) > MaxACPImages {
		return false
	}
	for _, path := range paths {
		if !strings.HasPrefix(path, "/") || len(path) > MaxACPImagePathBytes || !plainText(path, MaxACPImagePathBytes) {
			return false
		}
	}
	return true
}

type ACPModel struct {
	Model string `json:"model"`
}

func (ACPModel) kind() string  { return "acp_model" }
func (p ACPModel) valid() bool { return p.Model != "" && ValidModel(p.Model) }

// ACPCancel asks the machine to stop the current turn. The machine also closes
// any card it still holds, because OpenCode does not withdraw it.
type ACPCancel struct{}

func (ACPCancel) kind() string { return "acp_cancel" }
func (ACPCancel) valid() bool  { return true }

// ACPPermission answers one card. Allow is Allow once; false is Deny. There is
// no Always allow in this protocol.
type ACPPermission struct {
	PermissionID string `json:"permissionId"`
	Allow        bool   `json:"allow"`
}

func (ACPPermission) kind() string  { return "acp_permission" }
func (p ACPPermission) valid() bool { return validACPID(p.PermissionID) }

// ACPMode switches a live chat's approval behaviour. Yolo true answers every
// permission with allow once; false goes back to asking the person.
type ACPMode struct {
	Yolo bool `json:"yolo"`
}

func (ACPMode) kind() string { return "acp_mode" }
func (ACPMode) valid() bool  { return true }

// Targets of a session_mode switch.
const (
	SessionModeTerminal = "terminal"
	SessionModeChat     = "chat"
)

// SessionMode asks the machine to move a session to the OpenCode terminal or
// the structured chat. Cols and Rows size the terminal and are ignored for chat (current controllers send zero).
// Trust applies only when switching to chat.
type SessionMode struct {
	To           string `json:"to"`
	Tool         string `json:"tool"`
	Dir          string `json:"dir"`
	Model        string `json:"model,omitempty"`
	Yolo         bool   `json:"yolo,omitempty"`
	ACPSessionID string `json:"acpSessionId"`
	Title        string `json:"title,omitempty"`
	Trust        bool   `json:"trust,omitempty"`
	Cols         int    `json:"cols"`
	Rows         int    `json:"rows"`
}

func (SessionMode) kind() string { return "session_mode" }
func (m SessionMode) valid() bool {
	switch m.To {
	case SessionModeTerminal:
		if m.Trust || !ValidDimensions(m.Cols, m.Rows) {
			return false
		}
	case SessionModeChat:
		// Cols and Rows are ignored: a controller from before they were
		// zeroed for chat still sends them, and must keep working.
	default:
		return false
	}
	return ValidACPSessionIDFor(m.Tool, m.ACPSessionID) && ValidDir(m.Dir) &&
		(m.Model == "" || ValidModel(m.Model)) && (m.Title == "" || ValidTitle(m.Title))
}

func validACPID(s string) bool {
	if len(s) < 2 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == ':') {
			return false
		}
	}
	return true
}

func validChatText(s string, max int) bool {
	if len(s) > max || !utf8.ValidString(s) {
		return false
	}
	// A chat body is shown as text, never interpreted. NUL is refused because
	// it hides the rest of a value in several renderers and in SQLite tools.
	return !strings.ContainsRune(s, '\x00')
}

func validToolKind(s string) bool {
	switch s {
	case "", "read", "edit", "delete", "move", "search", "execute", "think", "fetch", "switch_mode", "other":
		return true
	}
	return false
}

func validToolStatus(s string) bool {
	switch s {
	case "", "pending", "in_progress", "completed", "failed", "cancelled":
		return true
	}
	return false
}

func validACPState(s string) bool {
	switch s {
	case "starting", "working", "waiting", "idle", "cancelled", "error", "ended":
		return true
	}
	return false
}

func validLocations(paths []string) bool {
	if len(paths) > MaxACPLocations {
		return false
	}
	for _, p := range paths {
		if p == "" || !validChatText(p, MaxDirBytes) {
			return false
		}
	}
	return true
}

// ACPEventSize reports whether one event fits the control frame after its
// envelope. Callers trim text before constructing the event; this is the last
// check before sending.
func ACPEventSize(e ACPEvent) bool {
	if e.valid() != true {
		return false
	}
	data, err := json.Marshal(e)
	return err == nil && len(data) <= MaxControlBytes-512
}

// ValidChatPrompt exposes the exact prompt rule to controller handlers.
func ValidChatPrompt(text string) bool { return (ACPPrompt{Text: text}).valid() }

// ValidACPPrompt is ValidChatPrompt with the image paths a prompt may carry.
func ValidACPPrompt(text string, images []string) bool {
	return (ACPPrompt{Text: text, Images: images}).valid()
}

// ValidPermissionID accepts the machine-generated approval id in a URL.
func ValidPermissionID(s string) bool { return validACPID(s) }

// ValidACPSessionID accepts OpenCode's own session id, which is the only handle
// a reopened chat has on its stored history.
func ValidACPSessionID(s string) bool {
	if !strings.HasPrefix(s, "ses_") || len(s) < 8 || len(s) > 128 {
		return false
	}
	for _, c := range s[4:] {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// Question bounds. They are the machine's own limits (internal/acpagent) held to
// the control frame: a form that does not fit is refused by the machine, never
// truncated, so a person never answers a different question than the agent asked.
const (
	MaxACPQuestionFields      = 8
	MaxACPQuestionOptions     = 12
	MaxACPQuestionMessage     = 1000
	MaxACPQuestionLabel       = 200
	MaxACPQuestionDescription = 500
	MaxACPQuestionKey         = 64
	MaxACPQuestionValue       = 200
	MaxACPQuestionTextAnswer  = 2000
	MaxACPQuestionAnswerBytes = 8 << 10
	// MaxACPQuestionWireBytes bounds a question as JSON, leaving room for the
	// event envelope inside the control frame.
	MaxACPQuestionWireBytes = 12 << 10
)

// ACPQuestionOption is one choice. Value is what the agent receives.
type ACPQuestionOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// ACPQuestionField is one answerable field. Kind is text, single or multi.
type ACPQuestionField struct {
	Key         string              `json:"key"`
	Title       string              `json:"title,omitempty"`
	Description string              `json:"description,omitempty"`
	Kind        string              `json:"kind"`
	Required    bool                `json:"required,omitempty"`
	Options     []ACPQuestionOption `json:"options,omitempty"`
	MinItems    int                 `json:"minItems,omitempty"`
	MaxItems    int                 `json:"maxItems,omitempty"`
	MaxLength   int                 `json:"maxLength,omitempty"`
}

// ACPQuestion is the normalized form a person answers.
type ACPQuestion struct {
	Message string             `json:"message"`
	Fields  []ACPQuestionField `json:"fields"`
}

// Valid reports whether the question fits the protocol's bounds, including the
// control-frame size. The machine refuses a question that does not.
func (q ACPQuestion) Valid() bool { return q.valid() }

func (q ACPQuestion) valid() bool {
	if q.Message == "" || !validChatText(q.Message, MaxACPQuestionMessage) ||
		len(q.Fields) == 0 || len(q.Fields) > MaxACPQuestionFields {
		return false
	}
	keys := map[string]bool{}
	for _, f := range q.Fields {
		if f.Key == "" || keys[f.Key] || !plainText(f.Key, MaxACPQuestionKey) ||
			!validChatText(f.Title, MaxACPQuestionLabel) || !validChatText(f.Description, MaxACPQuestionDescription) {
			return false
		}
		keys[f.Key] = true
		switch f.Kind {
		case "text":
			if len(f.Options) != 0 || f.MinItems != 0 || f.MaxItems != 0 || f.MaxLength < 1 || f.MaxLength > MaxACPQuestionTextAnswer {
				return false
			}
		case "single", "multi":
			if f.MaxLength != 0 || !validQuestionOptions(f.Options) {
				return false
			}
			if f.Kind == "single" && (f.MinItems != 0 || f.MaxItems != 0) {
				return false
			}
			if f.Kind == "multi" && (f.MinItems < 0 || f.MaxItems < 1 || f.MaxItems > len(f.Options) || f.MinItems > f.MaxItems) {
				return false
			}
		default:
			return false
		}
	}
	data, err := json.Marshal(q)
	return err == nil && len(data) <= MaxACPQuestionWireBytes
}

func validQuestionOptions(options []ACPQuestionOption) bool {
	if len(options) == 0 || len(options) > MaxACPQuestionOptions {
		return false
	}
	values := map[string]bool{}
	for _, o := range options {
		if o.Value == "" || values[o.Value] || !plainText(o.Value, MaxACPQuestionValue) ||
			o.Label == "" || !validChatText(o.Label, MaxACPQuestionLabel) || !validChatText(o.Description, MaxACPQuestionDescription) {
			return false
		}
		values[o.Value] = true
	}
	return true
}

// validQuestionEvent checks the three question kinds. Every field that belongs to
// another kind must be empty, so a question event cannot smuggle an approval card.
func (e ACPEvent) validQuestionEvent() bool {
	if !validACPID(e.QuestionID) || e.PermissionID != "" || e.MessageID != "" || e.Title != "" || e.ToolKind != "" ||
		e.Status != "" || e.State != "" || e.Text != "" || e.Command != "" || e.Path != "" || e.Diff != "" ||
		e.Additions != 0 || e.Deletions != 0 || e.DurationMs != 0 || len(e.Locations) != 0 || e.Allow != nil ||
		e.Bad || e.Truncated || e.Detail != "" {
		return false
	}
	switch e.Kind {
	case "question":
		return e.Question != nil && e.Question.valid() && (e.ToolCallID == "" || validACPID(e.ToolCallID)) && e.Reason == ""
	case "question_resolved":
		return e.Question == nil && e.ToolCallID == "" && (e.Reason == "accept" || e.Reason == "decline")
	}
	return e.Question == nil && e.ToolCallID == "" && validChatText(e.Reason, 100)
}

// ACPQuestionAnswer answers one pending question. Action is accept, decline or
// cancel; Answers is a JSON object of field key to value and is present only for
// accept. The machine validates it against the question it holds; this check is
// only about shape and size.
type ACPQuestionAnswer struct {
	QuestionID string          `json:"questionId"`
	Action     string          `json:"action"`
	Answers    json.RawMessage `json:"answers,omitempty"`
}

// Valid reports whether the answer has a well-formed shape and size. The machine
// still validates the content against the question it holds.
func (a ACPQuestionAnswer) Valid() bool { return a.valid() }

func (ACPQuestionAnswer) kind() string { return "acp_question_answer" }
func (a ACPQuestionAnswer) valid() bool {
	if !validACPID(a.QuestionID) {
		return false
	}
	switch a.Action {
	case "accept":
		return len(a.Answers) > 0 && len(a.Answers) <= MaxACPQuestionAnswerBytes && a.Answers[0] == '{' && json.Valid(a.Answers) && uniqueKeys(a.Answers)
	case "decline", "cancel":
		return len(a.Answers) == 0
	}
	return false
}

// ValidQuestionID accepts the machine-generated question id in a URL.
func ValidQuestionID(s string) bool { return validACPID(s) }
