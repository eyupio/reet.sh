//go:build linux || darwin

package acpagent

// Cline is the launch profile for Cline's built-in ACP mode (Cline CLI 3.0.74,
// measured 5 November 2026 with `cline --acp`; see
// docs/research/cline-acp-probe.md).
//
// Unlike Claude Code and Codex, the ACP server is the cline binary itself, so
// there is no adapter to install beside it. --auto-approve false is part of the
// launch, not a suggestion: without it the CLI auto-approves every tool and no
// approval card is ever raised. RequireOptions re-checks that the session
// actually reports auto_approve=false, so a changed default cannot quietly skip
// the approval contract.
//
// The measured session starts in "act" and offers "plan"; session/new reports
// the model catalogue and mode through ACP configOptions, set_config_option
// confirms both a mode and a model switch, and session/load replays a stored
// conversation. Cline ignores a model at session/new, so a chosen model is
// applied after the session opens and read back.
var Cline = Spec{
	Key:          "cline",
	Command:      "cline",
	Args:         []string{"--acp", "--auto-approve", "false"},
	DisplayName:  "Cline",
	ExpectedName: "cline",
	MinVersion:   "3.0.74",
	RequireMode:  "act",
	StartMode:    "act",
	// The person may move a chat between making changes and planning only; the
	// mode that runs changes is the one a new chat starts in.
	ChoosableModes: []string{"act", "plan"},
	// Every tool call is refused at launch unless the session confirms the CLI's
	// auto-approve switch is off.
	RequireOptions:       map[string]string{"auto_approve": "false"},
	ConfirmModelBySwitch: true,
	// session/load replays a stored conversation, so a lost chat reconnects in
	// place under the same reet session id.
	Reattach: true,
	// Cline keeps its sign-in, settings, hooks and sessions under ~/.cline.
	// CLINE_DIR routes that folder without inheriting a provider redirect, and
	// XDG directories are not inherited for a profile that uses this route.
	LoginHomeEnv: "CLINE_DIR",
	LoginHomeDir: ".cline",
	// Only the credential the CLI was verified to read. Provider redirects and
	// other agents' keys are never inherited.
	EnvAllowlist: []string{"CLINE_API_KEY"},
}
