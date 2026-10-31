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
}

// ReviewedChatTool is the closed set with a verified structured integration.
// Merely advertising another tool cannot enable its launch or history access.
func ReviewedChatTool(tool string) bool { return tool == "opencode" }

func (a ACPAgent) Valid() bool {
	if !ValidToolV2(a.Tool) || !plainText(a.Adapter, 64) || a.Adapter == "" ||
		!plainText(a.Version, 64) || !plainText(a.MinimumVersion, 64) {
		return false
	}
	switch a.State {
	case "available":
		return ReviewedChatTool(a.Tool) && a.Adapter == "opencode" && a.MinimumVersion != ""
	case "missing", "unverified", "incompatible":
		return !a.History && !a.Images && !a.Model && !a.Approval
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
			if !c.ACP || !installed || a.History && !c.ACPHistory || a.Images && !c.ACPImages || a.Model && !c.ACPModel || a.Approval && !c.ACPApproval {
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
	return ACPAgent{Tool: tool, Adapter: "opencode", State: "available", MinimumVersion: "1.19.5", History: c.ACPHistory, Images: c.ACPImages, Model: c.ACPModel, Approval: c.ACPApproval}, true
}

// ValidACPSessionIDFor retains each reviewed adapter's identifier contract.
// Unknown tools cannot borrow OpenCode's native history namespace.
func ValidACPSessionIDFor(tool, id string) bool {
	return ReviewedChatTool(tool) && ValidACPSessionID(id)
}
