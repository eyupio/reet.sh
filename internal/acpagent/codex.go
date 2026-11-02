//go:build linux || darwin

package acpagent

import (
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"
)

// CodexCandidate is the launch profile for the Codex ACP adapter (SC2 probes and
// the 1 November 2026 gate re-run). It is reachable only on a machine that sets
// CodexSwitch.
//
// Codex starts in "agent" ("Approve for me"), where an automatic reviewer decides
// and the person is never asked. reet starts every chat in "read-only", the only
// mode that can ask before a command leaves the sandbox, and refuses the chat if
// the adapter does not confirm it. Choosing "agent" for a chat is a later,
// explicit step in the browser, never the default.
//
// The adapter uses the person's existing Codex folder, never an ACP authenticate
// call. The signed-in runtime check remains a release gate.
var CodexCandidate = Spec{
	Key:          "codex",
	Command:      "codex-acp",
	DisplayName:  "Codex",
	ExpectedName: "@agentclientprotocol/codex-acp",
	MinVersion:   "1.13.1",
	LoginHomeEnv: "CODEX_HOME",
	LoginHomeDir: ".codex",
	RequireMode:  "read-only",
	StartMode:    "read-only",
	// A person may move a chat between the mode that can ask before a command
	// leaves the sandbox and the one where an automatic reviewer decides. Never
	// "agent-full-access".
	ChoosableModes: []string{"read-only", "agent"},
	// session/cancel ends a turn in about a second but leaves a running command
	// alive (probe report, "Codex gates, re-run"). Stop therefore ends the chat
	// when a command is running, so nothing keeps running behind a stopped chat.
	EndChatOnStopWhileToolRuns: true,
}

// handleAuthStatus forwards only the adapter's bounded public label. The
// account/email and gateway detail are deliberately never decoded or retained.
// A notice uses the existing bounded chat ring and works with older peers.
func (s *Session) handleAuthStatus(raw json.RawMessage) {
	if s.opts.Agent.Key != CodexCandidate.Key {
		return
	}
	var params struct {
		AuthStatus struct {
			Kind  string `json:"kind"`
			Label string `json:"label"`
		} `json:"authStatus"`
	}
	if json.Unmarshal(raw, &params) != nil {
		return
	}
	status := params.AuthStatus
	switch status.Kind {
	case "none", "account", "api_key", "gateway", "external":
	default:
		return
	}
	label := strings.TrimSpace(status.Label)
	if label == "" || len(label) > 256 || !utf8.ValidString(label) || strings.Contains(label, "@") || strings.ContainsFunc(status.Label, unicode.IsControl) {
		return
	}
	s.emit(Event{Kind: "notice", Text: "Codex sign-in: " + label})
}
