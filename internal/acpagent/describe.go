//go:build linux || darwin

package acpagent

import "github.com/eyupio/reet.sh/internal/terminalproto"

// Describe uses installation evidence only. No program runs: available means a
// reviewed adapter is installed; --version and initialize remain launch gates.
func Describe(tool string, installed bool, version string) terminalproto.ACPAgent {
	adapters := map[string]string{"opencode": "opencode", "claude": "claude-agent-acp", "codex": "codex-acp", "gemini": "gemini"}
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
	entry.History, entry.Images, entry.Model, entry.Approval = true, true, true, true
	return entry
}
