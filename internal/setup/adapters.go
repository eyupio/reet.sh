package setup

// Keep the registry declarative: adding discovery does not imply that a config
// format can be parsed, exported or translated to another agent or OS.
type adapter struct {
	id, name, command string
	locations         []locationSpec
}

type locationSpec struct {
	scope, kind, expected string
	path                  string
	env, suffix           string
}

func directory(scope, kind, path, env, suffix string) locationSpec {
	return locationSpec{scope, kind, "directory", path, env, suffix}
}

func file(scope, kind, path, env, suffix string) locationSpec {
	return locationSpec{scope, kind, "file", path, env, suffix}
}

func adapters() []adapter {
	return []adapter{
		{"claude", "Claude Code", "claude", []locationSpec{
			directory("global", "configuration", ".claude", "CLAUDE_CONFIG_DIR", ""),
			file("global", "configuration", ".claude.json", "", ""),
			file("global", "instructions", ".claude/CLAUDE.md", "CLAUDE_CONFIG_DIR", "CLAUDE.md"),
			directory("project", "configuration", ".claude", "", ""),
			file("project", "instructions", "CLAUDE.md", "", ""),
			file("project", "mcp_configuration", ".mcp.json", "", ""),
			directory("global", "skills", ".claude/skills", "CLAUDE_CONFIG_DIR", "skills"),
			directory("project", "skills", ".claude/skills", "", ""),
		}},
		{"codex", "Codex", "codex", []locationSpec{
			directory("global", "configuration", ".codex", "CODEX_HOME", ""),
			file("global", "instructions", ".codex/AGENTS.md", "CODEX_HOME", "AGENTS.md"),
			directory("global", "shared_skills", ".agents/skills", "", ""),
			directory("global", "skills", ".codex/skills", "CODEX_HOME", "skills"),
			directory("project", "configuration", ".codex", "", ""),
			directory("project", "shared_skills", ".agents/skills", "", ""),
			directory("project", "skills", ".codex/skills", "", ""),
			file("project", "shared_instructions", "AGENTS.md", "", ""),
		}},
		{"opencode", "OpenCode", "opencode", []locationSpec{
			directory("global", "configuration", ".config/opencode", "XDG_CONFIG_HOME", "opencode"),
			file("global", "instructions", ".config/opencode/AGENTS.md", "XDG_CONFIG_HOME", "opencode/AGENTS.md"),
			directory("global", "configuration", "", "OPENCODE_CONFIG_DIR", ""),
			file("global", "configuration", "", "OPENCODE_CONFIG", ""),
			directory("project", "configuration", ".opencode", "", ""),
			file("project", "configuration", "opencode.json", "", ""),
			file("project", "configuration", "opencode.jsonc", "", ""),
			directory("global", "skills", ".config/opencode/skills", "XDG_CONFIG_HOME", "opencode/skills"),
			directory("global", "skills", "", "OPENCODE_CONFIG_DIR", "skills"),
			directory("project", "skills", ".opencode/skills", "", ""),
		}},
		{"pi", "Pi", "pi", []locationSpec{
			directory("global", "configuration", ".pi/agent", "PI_CODING_AGENT_DIR", ""),
			file("global", "instructions", ".pi/agent/AGENTS.md", "PI_CODING_AGENT_DIR", "AGENTS.md"),
			directory("global", "skills", ".pi/agent/skills", "PI_CODING_AGENT_DIR", "skills"),
			directory("project", "configuration", ".pi", "", ""),
			directory("project", "skills", ".pi/skills", "", ""),
		}},
		{"cursor", "Cursor", "cursor", []locationSpec{
			directory("global", "configuration", ".cursor", "", ""),
			directory("project", "configuration", ".cursor", "", ""),
			file("project", "instructions", ".cursorrules", "", ""),
			directory("global", "skills", ".cursor/skills", "", ""),
			directory("project", "skills", ".cursor/skills", "", ""),
		}},
		{"gemini", "Gemini CLI", "gemini", []locationSpec{
			directory("global", "configuration", ".gemini", "", ""),
			file("global", "instructions", ".gemini/GEMINI.md", "", ""),
			directory("project", "configuration", ".gemini", "", ""),
			file("project", "instructions", "GEMINI.md", "", ""),
		}},
	}
}
