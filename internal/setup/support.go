package setup

import "runtime"

const setupSupportSchemaVersion = 1

// SupportRow is a conservative statement of what this build can capture and
// restore for an agent. It is not a claim that a restored setup works at runtime.
type SupportRow struct {
	AgentID      string `json:"agent_id"`
	Name         string `json:"name"`
	Settings     string `json:"settings"`
	MCP          string `json:"mcp"`
	Instructions string `json:"instructions"`
	Skills       string `json:"skills"`
	Memory       string `json:"memory"`
	History      string `json:"history"`
	Credentials  string `json:"credentials"`
	Restore      string `json:"restore"`
	Notes        string `json:"notes,omitempty"`
}

type SupportMatrix struct {
	SchemaVersion int          `json:"schema_version"`
	OS            string       `json:"os"`
	Architecture  string       `json:"architecture"`
	Agents        []SupportRow `json:"agents"`
	Limitations   []string     `json:"limitations"`
}

func BuildSupportMatrix() SupportMatrix {
	rows := []SupportRow{
		{AgentID: "claude", Name: "Claude Code", Settings: "supported", MCP: "supported", Instructions: "supported", Skills: "supported", Memory: "unsupported", History: "unsupported", Credentials: "references_only", Restore: "files_and_fields", Notes: "Project and global settings, MCP declarations, instructions and skill folders are selected; memory and sessions are not."},
		{AgentID: "codex", Name: "Codex", Settings: "supported", MCP: "supported", Instructions: "supported", Skills: "supported", Memory: "supported", History: "unsupported", Credentials: "references_only", Restore: "files_and_fields", Notes: "TOML settings and MCP declarations are supported; the Codex memory database is copied consistently, but session history is not."},
		{AgentID: "opencode", Name: "OpenCode", Settings: "supported", MCP: "supported", Instructions: "supported", Skills: "supported", Memory: "unsupported", History: "unsupported", Credentials: "references_only", Restore: "files_and_fields", Notes: "JSON/JSONC settings and MCP declarations are supported; memory and sessions are not."},
		{AgentID: "pi", Name: "Pi", Settings: "supported", MCP: "unsupported", Instructions: "supported", Skills: "supported", Memory: "unsupported", History: "unsupported", Credentials: "references_only", Restore: "files_and_fields", Notes: "Settings are supported; MCP declarations are not parsed for this adapter."},
		{AgentID: "cursor", Name: "Cursor", Settings: "unsupported", MCP: "supported", Instructions: "supported", Skills: "supported", Memory: "unsupported", History: "unsupported", Credentials: "references_only", Restore: "files_and_fields", Notes: "The MCP file and instructions are supported; full desktop preferences are not."},
		{AgentID: "gemini", Name: "Gemini CLI", Settings: "supported", MCP: "supported", Instructions: "supported", Skills: "unsupported", Memory: "unsupported", History: "unsupported", Credentials: "references_only", Restore: "files_and_fields", Notes: "Settings and the adapter's MCP policy are supported; Gemini skills locations are not inventoried."},
	}
	return SupportMatrix{
		SchemaVersion: setupSupportSchemaVersion,
		OS:            runtime.GOOS, Architecture: runtime.GOARCH, Agents: rows,
		Limitations: []string{
			"Support is limited to known locations, fields and formats in this build; a successful restore is not runtime verification.",
			"Only the Codex memory database is captured; other memory stores, conversation history, hooks, caches, keychains and device-bound sessions are not.",
			"Cross-OS restore and application version substitution remain unverified; review plan warnings before applying.",
			"Skills are copied as bounded whole folders; symlinked or special files make that skill an explicit omission.",
		},
	}
}
