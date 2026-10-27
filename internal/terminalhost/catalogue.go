package terminalhost

import (
	"sort"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Preset is connector-owned data. No controller-supplied command line is used.
// DisabledReason keeps unconfirmed commands unavailable even when installed.
type Preset struct {
	ID, Name, Binary   string
	ModelFlag          string
	PromptPositional   bool
	PromptFlag         string
	EffortArgs         map[string][]string
	YoloArgs, SafeArgs []string
	ACP                string
	DisabledReason     string
}

// Existing argument evidence predates this refactor: Claude Code 2.1.310
// --help; Codex CLI reference and codex-rs/tui/src/cli.rs (also the captured
// codex-cli 0.163.0 help); opencode.ai/docs/cli/; and the Gemini CLI reference
// plus packages/cli/src/config/config.ts. Those rows preserve exact argv.
//
// Catalogue preserves the four shipped presets' previously reviewed arguments.
// New commands require captured help and safest permission defaults; a matching
// binary name alone never enables them. Cline 3.0.74 requires explicit approval
// denial and interactive mode; its default otherwise auto-approves tools.
var Catalogue = []Preset{
	{ID: "claude", Name: "Claude Code", Binary: "claude", ModelFlag: "--model", PromptPositional: true, ACP: "adapter"},
	{ID: "codex", Name: "Codex", Binary: "codex", ModelFlag: "--model", PromptPositional: true, ACP: "adapter"},
	{ID: "opencode", Name: "OpenCode", Binary: "opencode", ModelFlag: "--model", PromptFlag: "--prompt", ACP: "native"},
	{ID: "gemini", Name: "Gemini CLI", Binary: "gemini", ModelFlag: "--model", PromptFlag: "--prompt-interactive"},
	{ID: "pi", Name: "Pi", Binary: "pi", DisabledReason: "This CLI has no built-in approval policy for tool execution."},
	{ID: "cursor", Name: "Cursor Agent", Binary: "cursor-agent", DisabledReason: "CLI help and permission defaults have not been confirmed."},
	{ID: "cline", Name: "Cline", Binary: "cline", ModelFlag: "--model", PromptPositional: true, SafeArgs: []string{"--auto-approve", "false", "--tui"}, EffortArgs: map[string][]string{"low": {"--thinking", "low"}, "medium": {"--thinking", "medium"}, "high": {"--thinking", "high"}}, ACP: "native"},
}

// launchPreset is a private execution view of the confirmed catalogue rows.
type launchPreset struct {
	binary, modelFlag, promptFlag string
	effortArgs                    map[string][]string
	safeArgs                      []string
}

var launchPresets = func() map[string]launchPreset {
	out := make(map[string]launchPreset)
	for _, p := range Catalogue {
		if p.DisabledReason != "" {
			continue
		}
		out[p.ID] = launchPreset{binary: p.Binary, modelFlag: p.ModelFlag, promptFlag: p.PromptFlag, effortArgs: p.EffortArgs, safeArgs: p.SafeArgs}
	}
	return out
}()

// InstalledTools lists sorted, confirmed preset ids found on PATH. With no v2
// argument it retains the old four-tool contract for existing callers.
func InstalledTools(lookPath func(string) (string, error), v2 ...bool) []string {
	extended := len(v2) > 0 && v2[0]
	out := []string{}
	for id, p := range launchPresets {
		if !extended && !terminalproto.ValidTool(id) {
			continue
		}
		if _, ok := resolveBinary(p, lookPath); ok {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
