package setup

import (
	"os"
	"path/filepath"
	"strings"
)

const exportPolicyVersion = 1

// ExportPolicy declares which components and fields a future encrypted snapshot
// may select. It is built from known locations only and never reads values.
type ExportPolicy struct {
	PolicyVersion int                  `json:"policy_version"`
	Sources       []SourceExportPolicy `json:"sources"`
	Classes       []ClassExportPolicy  `json:"classes"`
}

type SourceExportPolicy struct {
	Path       string                  `json:"path"`
	Scope      string                  `json:"scope"`
	Format     string                  `json:"format"`
	Observed   string                  `json:"observed"`
	Components []ComponentExportPolicy `json:"components"`
}

// ComponentExportPolicy is a policy for one source and component kind. Field
// paths are top-level names, or *.name inside each MCP declaration.
type ComponentExportPolicy struct {
	Kind           string              `json:"kind"`
	Selection      string              `json:"selection"`
	Handling       string              `json:"handling"`
	Fields         []FieldExportPolicy `json:"fields"`
	UnlistedFields string              `json:"unlisted_fields"`
	Reason         string              `json:"reason,omitempty"`
}

type FieldExportPolicy struct {
	Path  string `json:"path"`
	State string `json:"state"`
}

// ClassExportPolicy covers component kinds that are not tied to one parsed file.
type ClassExportPolicy struct {
	Kind      string `json:"kind"`
	Selection string `json:"selection"`
	Handling  string `json:"handling"`
	Reason    string `json:"reason"`
}

// ApplyExportPolicies is opt-in and metadata-only. It declares explicit
// selection rules for future snapshots; it does not read configuration values,
// collect files or make backup/restore available.
func ApplyExportPolicies(r *Report) {
	r.SchemaVersion = 2
	r.ExportPolicies = true
	r.Limitations = append(r.Limitations,
		"Export policies declare intended selection only; no setup export, backup or restore command is implemented.",
		"Policy reports contain known paths and field names, never configuration values; unlisted fields are excluded, not copied generically.",
	)
	for i := range r.Agents {
		a := &r.Agents[i]
		a.Capabilities.ExportPolicy = "declared_not_implemented"
		p := &ExportPolicy{
			PolicyVersion: exportPolicyVersion,
			Sources:       []SourceExportPolicy{},
			Classes:       classExportPolicies(),
		}
		seen := map[string]bool{}
		for _, loc := range a.Locations {
			if loc.Kind != "configuration" && loc.Kind != "mcp_configuration" && loc.Kind != "instructions" && loc.Kind != "shared_instructions" {
				continue
			}
			for _, path := range configPaths(a.ID, loc) {
				key := loc.Scope + "\x00" + path
				if seen[key] {
					continue
				}
				seen[key] = true
				components := componentExportPolicies(a.ID, loc.Kind, path)
				if len(components) == 0 {
					continue
				}
				observed := loc.State
				if observed == "present" {
					observed = observeSource(path)
				}
				p.Sources = append(p.Sources, SourceExportPolicy{
					Path:       path,
					Scope:      loc.Scope,
					Format:     strings.TrimPrefix(filepath.Ext(path), "."),
					Observed:   observed,
					Components: components,
				})
			}
		}
		a.ExportPolicy = p
	}
}

func observeSource(path string) string {
	info, err := os.Lstat(path)
	switch {
	case os.IsNotExist(err):
		return "missing"
	case err != nil:
		return "error"
	case info.Mode()&os.ModeSymlink != 0:
		return "symlink"
	case info.Mode().IsRegular():
		return "present"
	default:
		return "unsupported"
	}
}

func componentExportPolicies(agent, kind, path string) []ComponentExportPolicy {
	format := strings.TrimPrefix(filepath.Ext(path), ".")
	if kind == "instructions" || kind == "shared_instructions" {
		reason := "The whole discovered instruction file is selected as bounded text; it may contain private context and is only written inside an encrypted snapshot."
		if kind == "shared_instructions" {
			reason = "The discovered shared instruction file is selected as bounded text; other agents may use the same file, so restore it deliberately."
		}
		return []ComponentExportPolicy{{
			Kind: "instructions", Selection: "include", Handling: "portable",
			Fields: []FieldExportPolicy{}, UnlistedFields: "exclude", Reason: reason,
		}}
	}
	if kind == "mcp_configuration" {
		if agent == "pi" || !supportedFormat(format, agent) {
			return []ComponentExportPolicy{unsupportedMCPPolicy()}
		}
		return []ComponentExportPolicy{mcpExportPolicy(agent, "include")}
	}
	if agent == "cursor" {
		if !supportedFormat(format, agent) {
			return []ComponentExportPolicy{unsupportedMCPPolicy()}
		}
		return []ComponentExportPolicy{mcpExportPolicy(agent, "include")}
	}
	settings := settingsExportPolicy(agent, filepath.Base(path))
	if !supportedFormat(format, agent) {
		settings.Selection = "unsupported"
		settings.Handling = "not_implemented"
		settings.Reason = "The source format is not parsed by this adapter; no fields are selected."
		if agent == "pi" {
			return []ComponentExportPolicy{settings}
		}
		return []ComponentExportPolicy{settings, unsupportedMCPPolicy()}
	}
	if agent == "pi" {
		return []ComponentExportPolicy{settings}
	}
	return []ComponentExportPolicy{settings, mcpExportPolicy(agent, settings.Selection)}
}

func unsupportedMCPPolicy() ComponentExportPolicy {
	return ComponentExportPolicy{
		Kind:           "mcp",
		Selection:      "unsupported",
		Handling:       "not_implemented",
		Fields:         []FieldExportPolicy{},
		UnlistedFields: "exclude",
		Reason:         "MCP declarations are not parsed for this agent or source format.",
	}
}

func mcpExportPolicy(agent, selection string) ComponentExportPolicy {
	container := mcpContainer(agent)
	fields := []FieldExportPolicy{
		{container, "include"},
		{"*.type", "include"},
		{"*.command", "include"},
		{"*.args", "include"},
		{"*.url", "encrypted_only"},
		{"*.httpUrl", "encrypted_only"},
		{"*.env", "encrypted_only"},
		{"*.environment", "encrypted_only"},
		{"*.headers", "encrypted_only"},
		{"*.http_headers", "encrypted_only"},
		{"*.env_vars", "reference_only"},
		{"*.env_http_headers", "reference_only"},
		{"*.bearer_token_env_var", "reference_only"},
		{"*.oauth", "exclude"},
	}
	return ComponentExportPolicy{
		Kind:           "mcp",
		Selection:      selection,
		Handling:       "portable",
		Fields:         fields,
		UnlistedFields: "exclude",
		Reason:         "Declarations and launch parameters are selected; URLs and literal values in environment or header fields require explicit encrypted credential selection, while reference names are preserved without resolution. Review command and argument values separately.",
	}
}

func settingsExportPolicy(agent, base string) ComponentExportPolicy {
	policy := ComponentExportPolicy{
		Kind:           "settings",
		Selection:      "include",
		Handling:       "portable",
		UnlistedFields: "exclude",
		Reason:         "Only the listed top-level settings are selected; unknown, machine-specific and executable behaviour fields are excluded.",
	}
	switch agent {
	case "claude":
		if base == ".claude.json" {
			policy.Fields = []FieldExportPolicy{
				{"model", "include"},
				{"defaultModel", "include"},
				{"permissions", "include"},
				{"env", "encrypted_only"},
				{"hooks", "exclude"},
				{"projects", "exclude"},
				{"oauthAccount", "exclude"},
				{"userID", "exclude"},
				{"mcpServers", "exclude"},
			}
			policy.Reason = "Only the listed account-wide settings are selected; project state, account identity and MCP declarations are handled separately or excluded."
			return policy
		}
		policy.Fields = []FieldExportPolicy{
			{"model", "include"},
			{"permissions", "include"},
			{"enabledMcpjsonServers", "include"},
			{"disabledMcpjsonServers", "include"},
			{"enableAllProjectMcpServers", "include"},
			{"outputStyle", "include"},
			{"includeCoAuthoredBy", "include"},
			{"cleanupPeriodDays", "include"},
			{"alwaysThinkingEnabled", "include"},
			{"forceLoginMethod", "include"},
			{"env", "encrypted_only"},
			{"hooks", "exclude"},
			{"apiKeyHelper", "exclude"},
			{"awsAuthRefresh", "exclude"},
			{"awsCredentialExport", "exclude"},
			{"statusLine", "exclude"},
			{"mcpServers", "exclude"},
		}
		if base == "settings.local.json" {
			policy.Selection = "opt_in"
			policy.Reason = "Local settings are machine-specific and require explicit selection; only the listed fields are eligible. Hooks and credential helpers are always excluded."
		}
		return policy
	case "codex":
		policy.Fields = []FieldExportPolicy{
			{"model", "include"},
			{"model_provider", "include"},
			{"approval_policy", "include"},
			{"sandbox_mode", "include"},
			{"model_reasoning_effort", "include"},
			{"model_reasoning_summary", "include"},
			{"model_verbosity", "include"},
			{"model_context_window", "include"},
			{"model_max_output_tokens", "include"},
			{"hide_agent_reasoning", "include"},
			{"file_opener", "include"},
			{"tools", "include"},
			{"model_providers", "encrypted_only"},
			{"shell_environment_policy", "encrypted_only"},
			{"projects", "exclude"},
			{"mcp_servers", "exclude"},
		}
		return policy
	case "opencode":
		policy.Fields = []FieldExportPolicy{
			{"model", "include"},
			{"small_model", "include"},
			{"default_agent", "include"},
			{"theme", "include"},
			{"permission", "include"},
			{"instructions", "include"},
			{"disabled_providers", "include"},
			{"enabled_providers", "include"},
			{"share", "include"},
			{"autoupdate", "include"},
			{"compaction", "include"},
			{"watcher", "include"},
			{"formatter", "include"},
			{"provider", "encrypted_only"},
			{"mcp", "exclude"},
			{"agent", "exclude"},
			{"command", "exclude"},
			{"plugin", "exclude"},
			{"lsp", "exclude"},
			{"experimental", "exclude"},
		}
		return policy
	case "pi":
		policy.Fields = []FieldExportPolicy{
			{"defaultModel", "include"},
			{"defaultProvider", "include"},
			{"packages", "include"},
			{"theme", "include"},
			{"providers", "encrypted_only"},
			{"hooks", "exclude"},
			{"extensions", "exclude"},
		}
		return policy
	case "gemini":
		policy.Fields = []FieldExportPolicy{
			{"model", "include"},
			{"selectedAuthType", "include"},
			{"theme", "include"},
			{"autoAccept", "include"},
			{"fileFiltering", "include"},
			{"sandbox", "include"},
			{"contextFileName", "include"},
			{"checkpointing", "include"},
			{"tools", "include"},
			{"env", "encrypted_only"},
			{"mcpServers", "exclude"},
		}
		return policy
	default:
		policy.Selection = "unsupported"
		policy.Handling = "not_implemented"
		policy.Reason = "No settings policy is defined for this adapter."
		return policy
	}
}

func classExportPolicies() []ClassExportPolicy {
	return []ClassExportPolicy{
		{
			Kind: "instructions", Selection: "include", Handling: "portable",
			Reason: "Discovered project instruction files are selected as whole bounded text; global or unlisted instruction locations are not inventoried.",
		},
		{
			Kind: "behavior", Selection: "unsupported", Handling: "not_implemented",
			Reason: "Rules, custom commands, agent definitions, workflows and plugins need format-specific review; they are not exported.",
		},
		{
			Kind: "memory", Selection: "unsupported", Handling: "not_implemented",
			Reason: "No verified reusable-memory adapter is implemented; memory databases and session state are not exported.",
		},
		{
			Kind: "history", Selection: "opt_in", Handling: "not_implemented",
			Reason: "Conversations and sessions remain opt-in, but no history adapter is implemented.",
		},
		{
			Kind: "credentials", Selection: "include", Handling: "references_only",
			Reason: "Reference names are retained; literal credentials are not implemented, and keychains or device-bound sessions require fresh authentication.",
		},
		{
			Kind: "hooks", Selection: "exclude", Handling: "never",
			Reason: "Hooks execute commands; they are not copied generically and require explicit execution review.",
		},
		{
			Kind: "caches", Selection: "exclude", Handling: "never",
			Reason: "Caches, logs, sockets, temporary files and generated state are never collected.",
		},
	}
}

func mcpContainer(agent string) string {
	switch agent {
	case "codex":
		return "mcp_servers"
	case "opencode":
		return "mcp"
	default:
		return "mcpServers"
	}
}
