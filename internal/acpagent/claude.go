//go:build linux || darwin

package acpagent

// ClaudeCandidate is the measured launch profile for the Claude Code ACP
// adapter (SC2 probe, 31 October 2026). It is deliberately NOT in the agents
// registry: Claude stays disabled until the remaining gates in the probe report
// are closed and the descriptor/launch gates admit the tool.
//
// The command is the adapter, not the Claude CLI: the CLI does not take ACP
// arguments. The adapter pulls in its own native binary.
var ClaudeCandidate = Spec{
	Key:          "claude",
	Command:      "claude-agent-acp",
	DisplayName:  "Claude Code",
	ExpectedName: "@agentclientprotocol/claude-agent-acp",
	MinVersion:   "0.99.1",
	// "default" is the adapter's Manual mode, which asks before changes. A
	// session that starts in any other mode (acceptEdits, plan, auto) is refused
	// rather than switched, so a profile can never weaken the approval contract.
	RequireMode: "default",
	// Only credentials the probe and the adapter source show it reads. Provider
	// redirects (ANTHROPIC_BASE_URL, ANTHROPIC_AUTH_TOKEN, CLAUDE_CODE_USE_BEDROCK,
	// CLAUDE_CODE_USE_VERTEX, CLAUDE_CONFIG_DIR, ANTHROPIC_MODEL) are never
	// inherited, so an ambient variable cannot send credentials to another host
	// or change the configuration directory.
	EnvAllowlist: []string{"ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN"},
	SessionMeta:  ClaudeSessionMeta,
	ConfigEnv:    claudeConfigEnv,
}

// claudeConfigEnv is the reet-owned part of the adapter environment. Each
// variable was removed and restored in the probe without changing a turn, so
// these only turn background behaviour off.
func claudeConfigEnv(LaunchEnv) map[string]string {
	return map[string]string{
		"DISABLE_AUTOUPDATER":                      "1",
		"DISABLE_TELEMETRY":                        "1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"CLAUDE_CODE_ENABLE_PROMPT_SUGGESTION":     "false",
	}
}
