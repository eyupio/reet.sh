//go:build linux || darwin

package acpagent

import (
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// CodexCandidate is the launch profile for the Codex ACP adapter (SC2 probes and
// the 1 November 2026 gate re-run). Whether an account is offered its chat is the
// controller's per-account setting, never an environment variable.
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
	// codex refuses to start when CODEX_HOME names a folder that does not exist,
	// which is what a person who has never used Codex has (probe report, "Codex
	// parity probes").
	LoginFolderMustExist: true,
	RequireMode:          "read-only",
	StartMode:            "read-only",
	// A person may move a chat between the mode that can ask before a command
	// leaves the sandbox and the one where an automatic reviewer decides. Never
	// "agent-full-access".
	ChoosableModes: []string{"read-only", "agent"},
	// Measured from codex-acp 1.13.1 (2 November 2026). These are descriptive
	// only; no effort or other option control is offered yet. The effort values
	// depend on the model (probe report, "Codex parity probes"); the adapters
	// measured on 9 November carry no effort in a model id.
	Options: []terminalproto.ACPAgentOption{
		{ID: "collaboration_mode", Category: terminalproto.AgentOptionCollaborationMode, Values: []string{"default", "plan"}},
		{ID: "reasoning_effort", Category: terminalproto.AgentOptionThoughtLevel, Values: []string{"low", "medium", "high", "xhigh", "max", "ultra"}},
		{ID: "fast-mode", Category: terminalproto.AgentOptionModelConfig, Values: []string{"off", "on"}},
	},
	// session/cancel ends a turn in about a second but leaves a running command
	// alive (probe report, "Codex gates, re-run"). Stop therefore ends the chat
	// when a command is running, so nothing keeps running behind a stopped chat.
	EndChatOnStopWhileToolRuns: true,
	// A chosen model is applied by switching to it and reading the switch back
	// (measured: every listed model reads back). The effort list follows the
	// model, so a model is always applied before an effort.
	ConfirmModelBySwitch: true,
	// session/load reopens a stored conversation, and one the adapter never
	// stored is refused with a -32603 whose details name the thread, which the
	// connector maps to the missing stored conversation (measured).
	Reattach: true,
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
	valid := label != "" && len(label) <= 256 && utf8.ValidString(label) &&
		!strings.Contains(label, "@") && !strings.ContainsFunc(status.Label, unicode.IsControl)
	if !valid {
		return
	}
	s.emit(Event{Kind: "notice", Text: "Codex sign-in: " + label})
}
