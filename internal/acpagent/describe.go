//go:build linux || darwin

package acpagent

import "github.com/eyupio/reet.sh/internal/terminalproto"

// claudeTakesImages is true because the probe measured that the adapter advertises image prompts and forwards the block
// to the provider request. Whether the real model accepts an image is not measured, and the docs say so.
const claudeTakesImages = true

// Describe uses installation evidence only. No program runs: available means a
// reviewed adapter is installed; --version and initialize remain launch gates.
func Describe(tool string, installed bool, version string) terminalproto.ACPAgent {
	adapters := map[string]string{"opencode": "opencode", "claude": "claude-agent-acp", "codex": "codex-acp", "cline": "cline", "antigravity": "agy"}
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
		// Only what the probe proved: approvals, a read-back model switch, stored
		// history that session/load reopens, in-place reconnect, and image prompts
		// (the adapter advertises them and forwards the block). Questions stay off.
		entry.Model, entry.Approval, entry.History = true, true, true
		entry.Images = claudeTakesImages
		entry.Reattach = spec.Reattach
		return entry
	}
	if tool == "codex" {
		// Only approvals: Codex asks only when the model asks to leave its sandbox
		// (see the probe report). Model switching, replay and images are unverified.
		entry.Approval = true
		return entry
	}
	// OpenCode, Cline and AGY were measured end to end: history replay through
	// session/load, images in the prompt, a confirmed model switch and native
	// approval requests. Each agent's profile controls its permission policy.
	entry.History, entry.Images, entry.Model, entry.Approval = true, true, true, true
	// Only an adapter whose profile can reopen a stored conversation in place
	// says so. The controller offers that only on a connection that negotiated
	// acp-agent-reattach-v1, and the connector refuses a reattach for any other
	// profile, so claiming it would offer a reconnect that can only fail.
	entry.Reattach = spec.Reattach
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
