package terminalproto

import "strings"

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
	// Questions reports typed agent questions. No reviewed adapter offers them
	// yet, so the connector's descriptor builder never sets it; a controller
	// still honours it only on a connection that negotiated acp-question-v1.
	Questions bool `json:"questions,omitempty"`
}

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
	}
	return ""
}

func (a ACPAgent) Valid() bool {
	if !ValidToolV2(a.Tool) || !plainText(a.Adapter, 64) || a.Adapter == "" ||
		!plainText(a.Version, 64) || !plainText(a.MinimumVersion, 64) {
		return false
	}
	switch a.State {
	case "available":
		return ReviewedChatTool(a.Tool) && a.Adapter == chatAdapter(a.Tool) && a.MinimumVersion != ""
	case "missing", "unverified", "incompatible":
		return !a.History && !a.Images && !a.Model && !a.Approval && !a.Questions
	}
	return false
}

func (c Capabilities) validACPAgents() bool {
	if c.ACPAgents == nil {
		return true
	}
	if !c.TerminalControl || !c.TerminalEnabled || len(*c.ACPAgents) > MaxACPAgents {
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
			if !c.ACP || !installed || a.History && !c.ACPHistory || a.Images && !c.ACPImages || a.Model && !c.ACPModel || a.Approval && !c.ACPApproval || a.Questions && !c.ACPQuestions {
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
			if a.Tool == tool && a.State == "available" && a.Valid() {
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
	}
	return false
}

// validUUIDSessionID accepts the canonical lower-case 8-4-4-4-12 form observed
// from the Claude and Codex ACP adapters (SC2 probe report).
func validUUIDSessionID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
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
