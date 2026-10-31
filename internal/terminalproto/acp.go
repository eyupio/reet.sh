package terminalproto

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// ACPFeature negotiates structured OpenCode chats over the private protocol.
// A controller without it never receives the capability or sends chat bodies.
const ACPFeature = "acp-v1"

// ACPHistoryFeature lets a new controller accept the OpenCode session id and
// resume fields. A slice-1 controller negotiated acp-v1 but its strict decoder
// rejects those fields, so they are never sent without this feature.
const ACPHistoryFeature = "acp-history-v1"

// ACPImageFeature lets a controller send images with a prompt. The machine
// reads the uploaded files and embeds their bytes in the ACP prompt, the way
// AOE does, so the agent sees the image itself rather than a file path.
const ACPImageFeature = "acp-image-v1"

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
//	tool                 a tool call state (ToolCallID, Title, ToolKind, Status, Text, Locations)
//	notice               a line for the person (Text, Bad)
//	status               a turn or connection state (State, Detail)
//	permission           an approval card (PermissionID, ToolCallID, Title, ToolKind, Command, Path, Diff, Truncated)
//	permission_resolved  the person's answer (PermissionID, Allow)
//	permission_cancelled the card closed without an answer (PermissionID, Reason)
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
	Locations    []string `json:"locations,omitempty"`
	Allow        *bool    `json:"allow,omitempty"`
	Bad          bool     `json:"bad,omitempty"`
	Truncated    bool     `json:"truncated,omitempty"`
	Reason       string   `json:"reason,omitempty"`
	Detail       string   `json:"detail,omitempty"`
}

func (ACPEvent) kind() string { return "acp_event" }

func (e ACPEvent) valid() bool {
	switch e.Kind {
	case "user":
		return e.MessageID == "" && e.ToolCallID == "" && e.PermissionID == "" && e.Allow == nil && validChatText(e.Text, MaxACPEventBytes) && e.Text != ""
	case "agent":
		return validACPID(e.MessageID) && e.ToolCallID == "" && e.PermissionID == "" && e.Allow == nil && validChatText(e.Text, MaxACPEventBytes)
	case "tool":
		return validACPID(e.ToolCallID) && e.PermissionID == "" && e.Allow == nil && validChatText(e.Title, 200) &&
			validToolKind(e.ToolKind) && validToolStatus(e.Status) && validChatText(e.Text, MaxACPEventBytes) &&
			validLocations(e.Locations)
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
