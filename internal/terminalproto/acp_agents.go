package terminalproto

import (
	"slices"
	"strings"
)

// ACPAgentsFeature negotiates per-tool structured-chat descriptors. An absent
// list is a legacy OpenCode-only peer; an explicit empty list supports no chats.
const ACPAgentsFeature = "acp-agents-v1"
const MaxACPAgents = 8

func SupportsACPAgents(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == ACPAgentsFeature {
			return true
		}
	}
	return false
}

// ACPAgent describes local launch support, not a successful model turn. Version
// is installation evidence when available; launch still checks the executable
// and initialize identity/version. Unreviewed adapters never become available.
type ACPAgent struct {
	Tool           string `json:"tool"`
	Adapter        string `json:"adapter"`
	State          string `json:"state"`
	Version        string `json:"version,omitempty"`
	MinimumVersion string `json:"minimumVersion,omitempty"`
	History        bool   `json:"history,omitempty"`
	Images         bool   `json:"images,omitempty"`
	Model          bool   `json:"model,omitempty"`
	Approval       bool   `json:"approval,omitempty"`
	// Modes lists the agent modes a person may switch a live chat to, in the
	// order to show them. Empty means the agent offers no switch. Sent only on a
	// connection that negotiated acp-agent-mode-v1.
	Modes []string `json:"modes,omitempty"`
	// Questions reports typed agent questions. No reviewed adapter offers them
	// yet, so the connector's descriptor builder never sets it; a controller
	// still honours it only on a connection that negotiated acp-question-v1.
	Questions bool `json:"questions,omitempty"`
	// Options describes the other selectable options the adapter was measured
	// to offer. It is descriptive only: Modes remains the permission-sensitive
	// list a person may switch, and Model reports the dynamic model catalogue.
	// Sent only on a connection that negotiated acp-agent-options-v1.
	Options []ACPAgentOption `json:"options,omitempty"`
}

// ACPAgentOption is one non-permission selectable option, with the values the
// pinned adapter was measured to offer. The values can depend on the current
// model, so they describe what may be offered, not what is valid now.
type ACPAgentOption struct {
	ID       string   `json:"id"`
	Category string   `json:"category"`
	Values   []string `json:"values"`
}

// The measured non-permission option categories. A mode or model option is
// represented by ACPAgent.Modes and ACPAgent.Model instead, because those are
// permission-sensitive and dynamically listed respectively.
const (
	AgentOptionThoughtLevel      = "thought_level"
	AgentOptionModelConfig       = "model_config"
	AgentOptionCollaborationMode = "collaboration_mode"
)

// MaxACPAgentOptions and MaxACPAgentOptionValues bound a descriptor's option
// contract. The measured adapters offer at most three options and six values.
const (
	MaxACPAgentOptions      = 8
	MaxACPAgentOptionValues = 16
)

// ReviewedChatTool is the closed set with a verified structured integration.
// Merely advertising another tool cannot enable its launch or history access.
func ReviewedChatTool(tool string) bool { return chatAdapter(tool) != "" }

// chatAdapter is the adapter name a reviewed tool's descriptor must carry. A
// descriptor can only ever be "available" for the adapter that was reviewed for
// that tool, never for another one claiming its name.
func chatAdapter(tool string) string {
	switch tool {
	case "opencode":
		return "opencode"
	case "claude":
		return "claude-agent-acp"
	case "codex":
		return "codex-acp"
	case "cline":
		return "cline"
	}
	return ""
}

func (a ACPAgent) Valid() bool {
	validTool := ValidToolV2(a.Tool) && plainText(a.Adapter, 64) && a.Adapter != ""
	validText := plainText(a.Version, 64) && plainText(a.MinimumVersion, 64)
	if !validTool || !validText {
		return false
	}
	switch a.State {
	case "available":
		return ReviewedChatTool(a.Tool) && a.Adapter == chatAdapter(a.Tool) && a.MinimumVersion != "" && validAgentModes(a.Modes) && validAgentOptions(a.Options)
	case "missing", "unverified", "incompatible":
		return !a.History && !a.Images && !a.Model && !a.Approval && !a.Questions && len(a.Modes) == 0 && len(a.Options) == 0
	}
	return false
}

// MaxACPAgentModes bounds a descriptor's mode list.
const MaxACPAgentModes = 4

func validAgentModes(modes []string) bool {
	if len(modes) > MaxACPAgentModes {
		return false
	}
	for i, mode := range modes {
		if !ValidAgentMode(mode) || slices.Contains(modes[:i], mode) {
			return false
		}
	}
	return true
}

func validAgentOptions(options []ACPAgentOption) bool {
	if len(options) > MaxACPAgentOptions {
		return false
	}
	ids := map[string]bool{}
	for _, option := range options {
		validID := validAgentOptionToken(option.ID) && !ids[option.ID]
		validCategory := validAgentOptionCategory(option.Category)
		validValues := len(option.Values) >= 1 && len(option.Values) <= MaxACPAgentOptionValues
		valid := validID && validCategory && validValues
		if !valid {
			return false
		}
		ids[option.ID] = true
		values := map[string]bool{}
		for _, value := range option.Values {
			if !validAgentOptionToken(value) || values[value] {
				return false
			}
			values[value] = true
		}
	}
	return true
}

func validAgentOptionCategory(category string) bool {
	switch category {
	case AgentOptionThoughtLevel, AgentOptionModelConfig, AgentOptionCollaborationMode:
		return true
	}
	return false
}

// validAgentOptionToken accepts the short lower-case ids and values the
// measured adapters use ("reasoning_effort", "fast-mode", "xhigh").
func validAgentOptionToken(token string) bool {
	if token == "" || len(token) > 32 {
		return false
	}
	for _, r := range token {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func (c Capabilities) validACPAgents() bool {
	if c.ACPAgents == nil {
		return true
	}
	enabled := c.TerminalControl && c.TerminalEnabled
	if !enabled || len(*c.ACPAgents) > MaxACPAgents {
		return false
	}
	seen := map[string]bool{}
	for _, a := range *c.ACPAgents {
		if !a.Valid() || seen[a.Tool] {
			return false
		}
		seen[a.Tool] = true
		if a.State == "available" {
			installed := false
			for _, tool := range c.Tools {
				if tool == a.Tool {
					installed = true
				}
			}
			validBase := c.ACP && installed
			validHistory := !a.History || c.ACPHistory
			validImages := !a.Images || c.ACPImages
			validModel := !a.Model || c.ACPModel
			validApproval := !a.Approval || c.ACPApproval
			validQuestions := !a.Questions || c.ACPQuestions
			validModes := len(a.Modes) == 0 || c.ACPAgentMode
			validOptions := len(a.Options) == 0 || c.ACPAgentOptions
			valid := validBase && validHistory && validImages && validModel &&
				validApproval && validQuestions && validModes && validOptions
			if !valid {
				return false
			}
		}
	}
	return true
}

// ChatAgent applies the authoritative descriptor or the legacy OpenCode-only
// fallback. Explicit missing/unverified/incompatible or empty data never falls
// back to a machine-wide boolean.
func (c Capabilities) ChatAgent(tool string) (ACPAgent, bool) {
	if !c.ACP || !ReviewedChatTool(tool) {
		return ACPAgent{}, false
	}
	if c.ACPAgents != nil {
		for _, a := range *c.ACPAgents {
			matches := a.Tool == tool && a.State == "available"
			if matches && a.Valid() {
				return a, true
			}
		}
		return ACPAgent{}, false
	}
	if tool != "opencode" {
		// A peer that sent no descriptors predates every adapter but OpenCode.
		return ACPAgent{}, false
	}
	return ACPAgent{Tool: tool, Adapter: "opencode", State: "available", MinimumVersion: "1.19.5", History: c.ACPHistory, Images: c.ACPImages, Model: c.ACPModel, Approval: c.ACPApproval}, true
}

// ValidACPSessionIDFor retains each reviewed adapter's identifier contract.
// Unknown tools cannot borrow OpenCode's native history namespace.
func ValidACPSessionIDFor(tool, id string) bool {
	return ReviewedChatTool(tool) && nativeIDMatches(tool, id)
}

// ValidACPHandle accepts a native session handle in any reviewed adapter's
// namespace. A session snapshot and a session change carry no tool, so they must
// still accept every handle the connector can report; a caller that knows the
// tool uses ValidACPSessionIDFor, where the namespaces stay disjoint.
func ValidACPHandle(s string) bool {
	return ValidACPSessionID(s) || validUUIDSessionID(s) || validClineSessionID(s)
}

// nativeIDMatches checks the identifier namespace measured for each candidate
// adapter. It is deliberately independent of ReviewedChatTool so the formats
// can be tested before a candidate is enabled; ValidACPSessionIDFor still
// refuses every unreviewed tool. Namespaces are disjoint: one agent's id never
// validates for another.
func nativeIDMatches(tool, id string) bool {
	switch tool {
	case "opencode":
		return ValidACPSessionID(id)
	case "claude", "codex":
		return validUUIDSessionID(id)
	case "cline":
		return validClineSessionID(id)
	}
	return false
}

// validClineSessionID accepts the form Cline 3.0.74's ACP session/new returns
// and session/load reads: the millisecond task id, an underscore, five
// characters from the adapter's base64url-style alphabet and the "_cli" suffix
// (for example "1793848855723_8zvJS_cli" or "1793850943237_9D-BF_cli"). The TUI's
// own task ids have no suffix and are never stored as a chat handle.
func validClineSessionID(s string) bool {
	const suffix = "_cli"
	if !strings.HasSuffix(s, suffix) {
		return false
	}
	body := s[:len(s)-len(suffix)]
	at := strings.IndexByte(body, '_')
	if at <= 0 || at > 20 {
		return false
	}
	for i := 0; i < at; i++ {
		if body[i] < '0' || body[i] > '9' {
			return false
		}
	}
	tail := body[at+1:]
	if len(tail) != 5 {
		return false
	}
	for i := 0; i < len(tail); i++ {
		c := tail[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// validUUIDSessionID accepts the canonical lower-case 8-4-4-4-12 form observed
// from the Claude and Codex ACP adapters (SC2 probe report).
func validUUIDSessionID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := range s {
		switch i {
		case 8, 13, 18, 23:
			if s[i] != '-' {
				return false
			}
		default:
			c := s[i]
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return false
			}
		}
	}
	return true
}

// ChatReattachFor reports whether a lost chat of tool can be reattached in
// place on this machine. Only OpenCode supports it.
func (c Capabilities) ChatReattachFor(tool string) bool {
	return c.ChatReattach && tool == "opencode"
}

// SessionModeFor reports whether a session of tool can switch between the
// chat and the tool's own terminal on this machine: it negotiated
// session-mode-v1 and can launch the tool in a terminal (launch-v1 with the
// tool among its launch tools). The validator requires launch-v1 but not the
// tool, because an older connector reports session mode on a machine whose
// chats come from another adapter; such a machine is simply not asked.
func (c Capabilities) SessionModeFor(tool string) bool {
	return c.SessionMode && c.Launch && slices.Contains(c.Tools, tool)
}
