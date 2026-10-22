# Local AI agent setup discovery

`reet setup scan` is the first slice of [portable setup management](agent-setup-plan.md).
It is available without a server, account, token or network connection.

```sh
reet setup scan
reet setup scan --json
reet setup scan --project /path/to/project
reet setup scan --components --json
```

The default checks known global paths under the user's home directory and looks
for commands on PATH. `--project` additionally checks known locations directly
inside the selected project. It does not search child projects or parent folders.
A relative project path is resolved from the current directory. The project
argument must resolve to a directory; a final symlink is rejected. No project is
inspected implicitly just because the command runs inside one.

## Discovery coverage

| Agent | Command on PATH | Default global locations | Project locations |
| --- | --- | --- | --- |
| Claude Code | `claude` | `~/.claude`, `~/.claude.json` | `.claude`, `CLAUDE.md`, `.mcp.json` |
| Codex | `codex` | `~/.codex`, shared `~/.agents/skills` | `.codex`, shared `.agents/skills`, shared `AGENTS.md` |
| OpenCode | `opencode` | `~/.config/opencode` | `.opencode`, `opencode.json`, `opencode.jsonc` |
| Pi | `pi` | `~/.pi/agent` | `.pi` |
| Cursor | `cursor` | `~/.cursor` | `.cursor`, `.cursorrules` |
| Gemini CLI | `gemini` | `~/.gemini` | `.gemini`, `GEMINI.md` |

`CLAUDE_CONFIG_DIR`, `CODEX_HOME` and `PI_CODING_AGENT_DIR` override the corresponding
global directory. OpenCode honours `XDG_CONFIG_HOME` (with `/opencode` appended)
and additionally reports `OPENCODE_CONFIG_DIR` and `OPENCODE_CONFIG` when set.
Overrides must be absolute paths; a relative override is reported as an error
without falling back to the default. An unexpanded `~` is not an absolute path.
The default `~/.claude.json` location is also checked independently.

These are known location checks, not exhaustive platform/application discovery.
For example, Cursor's full desktop application preferences and agent session
storage outside these locations are not inventoried. Custom locations not listed
above are not discovered. No backup or restore coverage is claimed on any OS.

## Interpreting the report

An agent is `detected` when its command is available on PATH or an agent-specific
expected file/directory exists. This is not a health check or proof of installation:
configuration can remain after uninstalling an application, and a same-named
command could belong to another program. Shared skills/instructions do not alone
identify Codex. Executable and location observations are always reported separately.

`not_detected` means none of the checked agent-specific paths or executable names
was found. `unknown` means discovery found no positive evidence but encountered
an error, symlink or unexpected file type. A detected agent may still have errors
on individual locations; inspect those observations rather than treating the
agent state as a completeness assertion.

Location states:

- `present`: metadata matches the expected regular file or directory. Contents
  and directory readability were not checked.
- `missing`: the path was not found.
- `symlink`: the final path entry is a symlink; its target was not inspected.
- `error`: metadata lookup failed or an override was invalid.
- `unsupported`: the path exists with an unexpected type, such as a file where
  a configuration directory was expected.

The default scanner does not walk directories or read files. Normal OS path resolution
can traverse symlinks in ancestor directories; this metadata-only scan is not an
archive containment mechanism. Later backup collection requires separate path
containment and credential-classification checks.

## Output and limitations

Default JSON uses `schema_version: 1` with capture time, runtime OS/architecture, optional
project path, agent observations, capabilities and limitations. Each adapter's
capabilities are `discovery: metadata_only`, `backup: not_implemented` and
`restore: not_implemented`. Consumers must inspect the schema version before
interpreting the report. Paths may contain private project/user names, so review
reports before sharing them.

Per-location discovery issues remain in successful output so one inaccessible
agent does not hide other observations. Invalid command arguments, an invalid
project/home directory and output failures return command errors. Scripted users
should inspect location/executable states in JSON for incomplete observations.

Without `--components`, MCP definitions and configuration contents are not read.
Versions, package provenance, memory and conversation history are not collected. Agents and hooks
are never executed. Nothing is installed, changed or sent to a server. The report
is inventory metadata, not a recoverable backup. Existing `reet scan`, `reet sync`
and `reet backup` retain their existing meanings.

## Opt-in component summaries (S1a)

`--components` reads selected configuration files locally and returns schema v2.
The original metadata-only schema v1 remains unchanged without the flag. The
capability becomes `metadata_and_config_summary`; backup and restore remain
`not_implemented`.

| Agent | Selected configuration sources | Supported summary |
| --- | --- | --- |
| Claude Code | `~/.claude.json`; `settings.json` and `settings.local.json` under discovered global/project config directories; project `.mcp.json` | Selected settings and top-level `mcpServers` |
| Codex | `config.toml` under discovered global/project config directories | Present files explicitly reported as unsupported in this stage |
| OpenCode | `opencode.json` / `opencode.jsonc` under discovered config directories and project root; explicit `OPENCODE_CONFIG` | Strict JSON settings and `mcp`; JSONC explicitly unsupported |
| Pi | `settings.json` under discovered global/project config directories | Selected settings; no built-in MCP parser claimed |
| Cursor | `mcp.json` under discovered global/project config directories | `mcpServers` |
| Gemini CLI | `settings.json` under discovered global/project config directories | Selected settings and `mcpServers`, including `httpUrl` |

Path overrides from discovery are honoured. No directory walking, nested project
config evaluation, include expansion, shell evaluation, network calls or provider
authentication occurs. Each source is observed independently: agent-specific merge
precedence and effective configuration are not resolved.

Settings flags record the presence of selected top-level model/defaultModel,
hooks, permissions and packages fields; they do not validate their values. Top-level
`env` fields are counted. MCP summaries report names, transport shape, argument
count, and environment/header field counts. Commands, URLs, arguments, environment
values, headers, unknown values and raw parser errors are never returned.

Whole-value `${NAME}` and `{env:NAME}` strings count as reference-shaped fields;
this does not prove an agent resolves that syntax or that the environment variable
exists. Mixed templates, defaults and other values count conservatively as
literals. Counts cover selected environment/header fields only and cannot prove
that other fields are free of secrets. This is not a sanitised configuration export.
MCP names and source paths are identifiers and may still contain private information.

A source is `parsed`, `missing`, `error`, `symlink` or `unsupported`. Parsed means
strict JSON was decoded, not that the agent configuration is valid. MCP state
distinguishes `absent_in_file`, `parsed`, `partial`, `unsupported` and
`not_supported_by_adapter`. Never interpret errors, unsupported formats or missing
files as a complete absence of MCP servers across the user's setup.

Reads are limited to regular files up to 1 MiB. Duplicate JSON keys, nesting over
64 levels, over 20,000 values and trailing data are rejected. MCP maps over 256
entries are rejected; names over 256 bytes are omitted with a partial diagnostic.
Symlink files are skipped; opens are confined to the selected parent directory
and checked against the observed inode/size/mtime. Unix opens are nonblocking to
avoid hanging if a file is replaced with a FIFO. This is best-effort observation,
not a transactional snapshot or a hardened archive collector.

Current fixtures validate declared JSON shapes rather than installed agent release
versions. TOML/JSONC parsing, full version/provenance capture, memory/history export
policy and restore verification are subsequent stages in the implementation plan.
