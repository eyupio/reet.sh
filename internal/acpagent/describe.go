//go:build linux || darwin

package acpagent

import "github.com/eyupio/reet.sh/internal/terminalproto"

// Describe uses installation evidence only. No program runs: available means a
// reviewed adapter is installed; --version and initialize remain launch gates.
func Describe(tool string, installed bool, version string) terminalproto.ACPAgent {
	spec, reviewed := Lookup(tool)
	entry := terminalproto.ACPAgent{Tool: tool, Adapter: spec.Command, State: "missing", MinimumVersion: spec.MinVersion}
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
	// Every reviewed adapter was measured for history replay through
	// session/load, images in the prompt, a confirmed model switch and native
	// approval requests. For Claude and Codex the adapter advertises image
	// prompts and forwards the block; whether a real model accepts one is not
	// measured, and the docs say so. Questions stay off for all of them. Each
	// agent's profile controls its permission policy.
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
