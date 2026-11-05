//go:build linux || darwin

package acpagent

import "github.com/eyupio/reet.sh/internal/terminalproto"

// Antigravity is the native AGY 1.5.0 ACP server. The compatibility probe used
// its real binary with a local Gemini stub: streaming, command/edit approvals,
// model/effort read-back and session/load replay were verified without a real
// provider call. Its configured Google login stays on the machine.
var Antigravity = Spec{
	Key:                  "antigravity",
	Command:              "agy",
	Args:                 []string{"acp", "--disable-telemetry"},
	DisplayName:          "Google Antigravity (AGY)",
	ExpectedName:         "Antigravity ACP Server",
	MinVersion:           "1.5.0",
	RequireMode:          "default",
	StartMode:            "default",
	ConfirmModelBySwitch: true,
	RequireOptions:       map[string]string{"mode": "default"},
	EnvAllowlist:         []string{"GEMINI_API_KEY"},
	Options: []terminalproto.ACPAgentOption{
		{ID: "thought_level", Category: terminalproto.AgentOptionThoughtLevel, Values: []string{"low", "medium", "high"}},
	},
	// A cancelled command may have descendants. Retire the complete ACP process
	// group when Stop is pressed during a tool, rather than leave them running.
	EndChatOnStopWhileToolRuns: true,
	Trust:                      antigravityTrust,
}

// AGY discovers hooks, plugins and inherited customizations from the workspace
// and its ancestors. A filename scan cannot establish that no executable
// customization will load, so every AGY chat requires the folder acknowledgement.
func antigravityTrust(string) ([]string, error) {
	return []string{"Google Antigravity workspace hooks and customizations"}, nil
}
