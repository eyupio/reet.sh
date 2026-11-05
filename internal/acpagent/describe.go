//go:build linux || darwin

package acpagent

import "github.com/eyupio/reet.sh/internal/terminalproto"

// Describe uses installation evidence only. No program runs: available means a
// reviewed adapter is installed; --version and initialize remain launch gates.
func Describe(tool string, installed bool, version string) terminalproto.ACPAgent {
	adapters := map[string]string{"opencode": "opencode", "claude": "claude-agent-acp", "codex": "codex-acp", "gemini": "gemini", "cline": "cline"}
	entry := terminalproto.ACPAgent{Tool: tool, Adapter: adapters[tool], State: "missing"}
	spec, reviewed := Lookup(tool)
	if reviewed {
		entry.MinimumVersion = spec.MinVersion
	}
	if !installed {
		return entry
	}
	entry.State = "unverified"
	if !reviewed {
		return entry
	}
	entry.Version = version
	if version != "" && checkVersion(spec, version) != nil {
		entry.State = "incompatible"
		return entry
	}
	entry.State = "available"
	entry.Modes = append([]string(nil), spec.ChoosableModes...)
	entry.Options = cloneAgentOptions(spec.Options)
	if tool == "claude" {
		// Only what the probe verified: approvals and a read-back model switch.
		// Native-session replay and image prompts were not verified, so they stay off.
		entry.Model, entry.Approval = true, true
		return entry
	}
	if tool == "codex" {
		// Only approvals: Codex asks only when the model asks to leave its sandbox
		// (see the probe report). Model switching, replay and images are unverified.
		entry.Approval = true
		return entry
	}
	// OpenCode and Cline were measured end to end: history replay through
	// session/load, images in the prompt, a confirmed model switch and approval
	// cards for every tool call.
	entry.History, entry.Images, entry.Model, entry.Approval = true, true, true, true
	return entry
}

func cloneAgentOptions(in []terminalproto.ACPAgentOption) []terminalproto.ACPAgentOption {
	if len(in) == 0 {
		return nil
	}
	out := make([]terminalproto.ACPAgentOption, len(in))
	for i, option := range in {
		out[i] = option
		out[i].Values = append([]string(nil), option.Values...)
	}
	return out
}
