//go:build linux || darwin

package acpagent

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
// Not yet verified against a real login: the adapter answers session/new with
// "Authentication required" until authenticate is called, and the connector
// replaces HOME, so a ChatGPT login under the real home is not visible. A chat
// on such a machine fails to start rather than guessing. The expected agent name
// follows the Claude adapter's naming and is unconfirmed; a mismatch refuses the
// launch.
var CodexCandidate = Spec{
	Key:          "codex",
	Command:      "codex-acp",
	DisplayName:  "Codex",
	ExpectedName: "@agentclientprotocol/codex-acp",
	MinVersion:   "1.13.1",
	RequireMode:  "read-only",
	StartMode:    "read-only",
	// session/cancel ends a turn in about a second but leaves a running command
	// alive (probe report, "Codex gates, re-run"). Stop therefore ends the chat
	// when a command is running, so nothing keeps running behind a stopped chat.
	EndChatOnStopWhileToolRuns: true,
}
