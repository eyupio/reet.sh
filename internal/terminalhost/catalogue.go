package terminalhost

import (
	"sort"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// ModelList is the connector-owned command that lists a preset's models.
// Argv is appended to the resolved binary; Env adds fixed opt-outs to the
// minimal listing environment. No controller-supplied argument is ever used.
type ModelList struct {
	Argv []string
	Env  []string
}

// Preset is connector-owned data. No controller-supplied command line is used.
// DisabledReason keeps unconfirmed commands unavailable even when installed.
type Preset struct {
	ID, Name, Binary   string
	ModelFlag          string
	PromptPositional   bool
	PromptFlag         string
	EffortArgs         map[string][]string
	YoloArgs, SafeArgs []string
	// ResumeArgs continues the tool's last conversation in the launch folder.
	// A preset without them is never offered or sent a resume.
	ResumeArgs     []string
	ACP            string
	DisabledReason string
	ModelList      *ModelList
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
	{ID: "codex", Name: "Codex", Binary: "codex", ModelFlag: "--model", PromptPositional: true, YoloArgs: []string{"--dangerously-bypass-approvals-and-sandbox"}, ACP: "adapter"},
	{ID: "opencode", Name: "OpenCode", Binary: "opencode", ModelFlag: "--model", PromptFlag: "--prompt", YoloArgs: []string{"--auto"}, ResumeArgs: []string{"--continue"}, ACP: "native", ModelList: &ModelList{Argv: []string{"models"}, Env: []string{"OPENCODE_DISABLE_MODELS_FETCH=1", "OPENCODE_DISABLE_AUTOUPDATE=1", "NO_COLOR=1", "LANG=C.UTF-8", "CI=1"}}},
	{ID: "gemini", Name: "Gemini CLI", Binary: "gemini", ModelFlag: "--model", PromptFlag: "--prompt-interactive"},
	{ID: "pi", Name: "Pi", Binary: "pi", DisabledReason: "This CLI has no built-in approval policy for tool execution."},
	{ID: "cursor", Name: "Cursor Agent", Binary: "cursor-agent", DisabledReason: "CLI help and permission defaults have not been confirmed."},
	{ID: "cline", Name: "Cline", Binary: "cline", ModelFlag: "--model", PromptPositional: true, YoloArgs: []string{"--yolo", "--tui"}, SafeArgs: []string{"--auto-approve", "false", "--tui"}, EffortArgs: map[string][]string{"low": {"--thinking", "low"}, "medium": {"--thinking", "medium"}, "high": {"--thinking", "high"}}, ACP: "native"},
}

// launchPreset is a private execution view of the confirmed catalogue rows.
type launchPreset struct {
	binary, modelFlag, promptFlag  string
	effortArgs                     map[string][]string
	safeArgs, yoloArgs, resumeArgs []string
	modelList                      *ModelList
}

var launchPresets = func() map[string]launchPreset {
	out := make(map[string]launchPreset)
	for _, p := range Catalogue {
		if p.DisabledReason != "" {
			continue
		}
		out[p.ID] = launchPreset{binary: p.Binary, modelFlag: p.ModelFlag, promptFlag: p.PromptFlag, effortArgs: p.EffortArgs, safeArgs: p.SafeArgs, yoloArgs: p.YoloArgs, resumeArgs: p.ResumeArgs, modelList: p.ModelList}
	}
	return out
}()

// SupportsResume reports whether a preset has a reviewed command that
// continues its last conversation.
func SupportsResume(tool string) bool {
	p, ok := launchPresets[tool]
	return ok && len(p.resumeArgs) > 0
}

// ListsModels reports whether an enabled preset has a connector-owned
// listing. A disabled or unknown tool never has one, and nothing runs for it.
func ListsModels(tool string) bool {
	p, ok := launchPresets[tool]
	return ok && p.modelList != nil && len(p.modelList.Argv) > 0
}

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
