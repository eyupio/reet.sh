# Local AI agent setup discovery

`reet setup scan` is the first slice of [portable setup management](agent-setup-plan.md).
It is available without a server, account, token or network connection.

```sh
reet setup scan
reet setup scan --json
reet setup scan --project /path/to/project
reet setup scan --components --json
reet setup scan --export-policy --json
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
Without `--probe-versions`, application versions are not collected and agents are
never executed. Package provenance, memory and conversation history are not collected.
The scanner installs nothing and sends no reports to a server. The report
is inventory metadata, not a recoverable backup. Existing `reet scan`, `reet sync`
and `reet backup` retain their existing meanings.

## Opt-in component summaries (S1a/S1b)

`--components` reads selected configuration files locally and returns schema v2.
The original metadata-only schema v1 remains unchanged without the flag. The
capability becomes `metadata_and_config_summary`; backup and restore remain
`not_implemented`.

| Agent | Selected configuration sources | Supported summary |
| --- | --- | --- |
| Claude Code | `~/.claude.json`; `settings.json` and `settings.local.json` under discovered global/project config directories; project `.mcp.json` | Selected settings and top-level `mcpServers` |
| Codex | `config.toml` under discovered global/project config directories | TOML settings and `mcp_servers` |
| OpenCode | `opencode.json` / `opencode.jsonc` under discovered config directories and project root; explicit `OPENCODE_CONFIG` | JSON/JSONC settings and `mcp` |
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

For Codex, `approval_policy` / `sandbox_mode` also set the permissions-field flag;
OpenCode recognises its singular `permission` setting. Codex MCP summaries support
stdio commands and HTTP URLs, `env`, `env_vars`, `http_headers`,
`env_http_headers` and `bearer_token_env_var`. Environment/header reference names
are counted without being returned or resolved. The bearer-token reference adds
to `reference_fields` without adding to environment/header map counts.

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

Reads are limited to regular files up to 1 MiB. Duplicate keys and invalid format
syntax are rejected. JSON/JSONC parsing rejects nesting over 64 levels, over
20,000 values and trailing data. JSONC allows line/block comments and trailing
commas; quoted strings are preserved and duplicate keys remain errors. TOML uses
the pinned `github.com/pelletier/go-toml/v2` parser (v2.4.3), which limits value
nesting during decoding; the decoded tree then receives the same inventory
depth/node limits, including dotted table/key depth. MCP maps over 256
entries are rejected; names over 256 bytes are omitted with a partial diagnostic.
Symlink files are skipped; opens are confined to the selected parent directory
and checked against the observed inode/size/mtime. Unix opens are nonblocking to
avoid hanging if a file is replaced with a FIFO. This is best-effort observation,
not a transactional snapshot or a hardened archive collector.

Current fixtures validate declared JSON, TOML and JSONC shapes rather than installed
agent release versions. Other formats remain unsupported. Verified package provenance,
content capture, encrypted snapshots and restore verification remain later stages in
the implementation plan.

## Opt-in export policy declarations (S1c)

```sh
reet setup scan --export-policy --json
reet setup scan --components --export-policy --json
```

`--export-policy` adds schema v2 `export_policy` declarations to each agent and
sets `export_policies: true`. It is independent of `--components` and reads no
configuration contents. The default metadata-only schema v1 remains unchanged
without the flag. Policies describe intended selection for a future encrypted
snapshot; no setup export, backup or restore command is implemented.

Each known configuration or MCP source has a `SourceExportPolicy` with its path,
scope, format and observed source state. A source policy lists the component
kinds that apply to it:

| Component | Selection | Handling |
| --- | --- | --- |
| `settings` | `include` or `opt_in` | `portable` |
| `mcp` | `include` or `opt_in` | `portable` |
| unknown source format or unsupported adapter | `unsupported` | `not_implemented` |

`settings.local.json` is `opt_in` because it is machine-specific. MCP declarations
follow their source's selection. For a present location, a `SourceExportPolicy`'s
`observed` value is the state of that exact candidate file, not the containing
directory. Missing, symlink, error and unsupported location states are inherited
without inspecting targets.

Field states are explicit:

- `include`: selected with the component.
- `encrypted_only`: selected only when portable credentials are explicitly chosen
  for an encrypted snapshot.
- `reference_only`: the reference name is preserved; the referenced value is never
  read or resolved.
- `exclude`: never copied, even with credentials.

`unlisted_fields` is always `exclude`, so a settings file is never copied
generically. `*.command` and similar entries describe fields inside each MCP
declaration. URL and literal environment/header values are `encrypted_only`;
reference fields such as `env_vars`, `env_http_headers` and
`bearer_token_env_var` are `reference_only` where a format defines them.
Hooks, credential helpers, project history, account identities, provider secrets
and unknown fields are not selected by default. Commands and arguments are classed
as launch parameters; only URLs and environment/header values receive the
`encrypted_only` classification, so review command and argument values before any
future export.

Component kinds not tied to one parsed file are classified separately for every
agent:

| Class | Selection | Handling |
| --- | --- | --- |
| instructions | unsupported | not_implemented |
| behaviour | unsupported | not_implemented |
| memory | unsupported | not_implemented |
| history | opt_in | not_implemented |
| credentials | include | references_only |
| hooks | exclude | never |
| caches | exclude | never |

These classes make the current gaps explicit rather than implying that memory,
history, hooks or portable credentials can already be exported. `history` is an
intended opt-in; `credentials` means reference names only, and device-bound
keychains or sessions still require fresh authentication. No class policy is a
claim that its adapter or restore path exists.

Policy paths and field names may identify private projects, services or settings,
so review a report before sharing it. No configuration values, commands, URLs,
environment values, headers or credential material are returned. Unknown formats
and unsupported adapters remain explicit. The policy is not a sanitised
configuration export, a backup manifest or evidence that restoration is supported.
See the [implementation plan](agent-setup-plan.md) for the snapshot, encryption and
restore acceptance gates.

## Backup preview (S2a)

```sh
reet setup backup --preview
reet setup backup --preview --json
reet setup backup --preview --project /path/to/project
```

`--preview` applies the export policies to the discovered sources and reports the
selection without writing an archive. It emits schema v1 with a policy version,
captured OS/architecture, per-agent selected components, included field names,
credential-bearing field names, preserved reference names, excluded field names,
unknown-field counts, MCP declaration counts and a SHA-256 digest of each selected
component. It also lists omissions with fixed reasons such as `source_opt_in`,
`source_symlink`, `file_too_large`, `credentials_not_selected`,
`excluded_by_policy`, `unlisted_fields`, `declarations_without_selected_fields` and
`invalid_mcp_declarations`.

The preview reads only present, regular, supported files and keeps selected values
in memory. It never prints selected values. Opt-in sources such as
`settings.local.json` are not read. Credential-bearing fields are counted but not
selected; reference names are kept without resolving their values. Component
classes not yet implemented are listed with their policy, not collected.

This is not a backup or a manifest. Writing snapshots, encryption, key/passphrase
management, S3 delivery, restore planning, execution, verification and cross-OS
translation remain unimplemented. Digests cover selected component content only;
they are not file digests, repository provenance or proof that a setup can be
restored. Run `reet setup backup --preview` before `reet setup backup` writes
anything; writing still returns an explicit not-implemented error.

## Encrypted local setup snapshots (S2b)

```sh
reet setup backup --password-file ~/.config/reet/setup.pass
reet setup backup --password-file - --json
reet setup backup --password-file FILE --dir /path/to/snapshots
```

Without `--preview`, `reet setup backup` applies the same policy-driven selection
and writes one encrypted local snapshot. The passphrase must be at least 12
characters and comes from a regular file or standard input (`-`); there is no
command-line passphrase value and no stored key. A passphrase file must be a
regular file, not a symlink, and at most 4 KiB. The passphrase is not written to
the snapshot, logs or output.

Snapshots are written atomically with mode 0600 into a directory with mode 0700.
The default directory is `REET_SETUP_DIR` when set, otherwise the user config
directory under `reet/setup`; `--dir` overrides both. The writer refuses a
snapshot directory that is a symlink or not a directory. Each file is named
`setup-<UTC timestamp>-<nonce>.reetsetup`.

The existing shared archive primitive encrypts the gzip-compressed JSON payload
with AES-256-GCM and an Argon2id-derived key. The clear archive header contains
only the snapshot ID, kind `setup`, target `setup`, recorded time, format version,
payload size, payload SHA-256, protection and compression. Source paths, selected
settings values, MCP declarations and omission details stay inside the encrypted
payload. The writer opens the sealed bytes before writing and opens the file on
disk again afterwards; a mismatch fails and removes the file.

The encrypted payload includes the policy preview, selected source content and the
omissions report. It does not include credential-bearing values, opt-in sources,
memory, history, hooks, caches or unsupported classes. The snapshot is not a
complete AI setup: restore, list/inspect, S3 delivery, dependency resolution,
cross-OS translation and post-restore verification remain unimplemented.
`--preview` remains read-only.

## Listing and inspecting local snapshots (S2c)

```sh
reet setup list
reet setup list --json --dir /path/to/snapshots
reet setup inspect setup-20261022T120000Z-a1b2c3d4
reet setup inspect --password-file FILE --json setup-20261022T120000Z-a1b2c3d4
```

`reet setup list` reads clear archive headers without a passphrase. It lists regular
`.reetsetup` files by recorded time, newest first, with ID, path, size, protection
and compression. Symlinks, temporary `.reet-setup-*` files, invalid headers and other
archive kinds are reported as skipped; unrelated files are ignored. A missing
directory is an empty list, not an error.

`reet setup inspect` accepts a snapshot ID (resolved under `--dir`,
`REET_SETUP_DIR` or the default directory) or a `.reetsetup` path. It always shows
the clear header. With `--password-file FILE` (or `-`) it opens and verifies the
encrypted payload, then reports the policy preview and per-source summaries: path,
component, field/declaration counts and recorded digest. It never prints selected
values, file contents or credential material. A wrong passphrase or damaged archive
fails without partial output. Flags must come before the snapshot argument.

Neither command writes, restores or repairs anything. Listing does not prove that a
snapshot is complete or restorable, and inspection does not verify application health
or cross-OS compatibility. S3 delivery, restore planning/execution and post-restore
verification remain unimplemented.

## Restore planning (S3)

```sh
reet setup restore --password-file FILE SNAPSHOT
reet setup restore --password-file FILE --project /path/to/project --json SNAPSHOT
```

`reet setup restore` opens and verifies the snapshot with the passphrase, discovers
the destination metadata-only, and prints a plan. It writes nothing and refuses
`--apply` with an explicit not-implemented error.

Global sources are mapped to the destination's known agent locations by agent,
scope and file name. Project sources require `--project`; without it they are listed
as `project_not_selected`. A source with no matching destination location is
`destination_location_not_found` rather than being written to a guessed path. The map
is based on declared locations and names, not a full cross-OS path translation.

Each planned file is `create` when missing, `conflict` when a destination source
already exists, or `blocked` when the destination is a symlink, unreadable or an
unexpected type. Conflicts are not merged or resolved yet. Omitted policy records and
unsupported classes remain in the plan's unsupported list. The plan fingerprint is a
SHA-256 of the snapshot identity, destination facts, actions and unsupported entries;
it contains no selected values.

No files, installers, hooks, package managers or authentication flows run without
`--apply`, and a plan is not a promise that a working setup can be restored.

### Applying a create-only plan

```sh
reet setup restore --apply --password-file FILE SNAPSHOT
```

`--apply` is a local confirmation. It writes only when every action is `create`; any
`conflict` or `blocked` action refuses the whole run before writing. Destinations are
rechecked after the plan, and a changed destination invalidates the run. It writes the
selected settings and MCP declarations as a new JSON, JSONC or TOML file with mode
0600, creating missing parent directories with mode 0700. Existing JSONC comments are
not preserved because the destination is a new file. Each written file is parsed again
and compared with the selected content; a mismatch removes it.

A journal is written atomically under the snapshot directory (`--dir`,
`REET_SETUP_DIR` or the default). It records destinations, states and errors, never
selected values. If a later write fails, files created by that run are removed and the
journal records `failed`/`rolled_back`. `--apply` does not run installers, hooks,
package managers or authentication, and it does not verify runtime health.

Per-conflict choices, replacement, merge, rollback copies, a rollback command,
dependency/version resolution, cross-OS translation and post-restore verification
remain unimplemented. Applying a plan is not the complete setup milestone.

## Opt-in application version probes

```sh
reet setup scan --probe-versions --json
reet setup scan --components --probe-versions --json
```

`--probe-versions` explicitly invokes each discovered absolute executable path
with one fixed argument, `--version`. It does not use commands from configuration
files or a shell. Commands inherit the current environment and working directory;
they may perform their own file/network operations. This flag is opt-in precisely
because an executable's behaviour cannot be guaranteed by metadata discovery.

Each command has a three-second timeout and at most 8 KiB of stdout retained.
Stderr and unrecognised stdout are discarded. Only a recognised three-part version
token (optionally with prerelease/build metadata) from the first line is returned.
This may still identify a private build. Unix cancellation terminates the process
group; elsewhere cancellation targets the direct process. Waiting for inherited
output pipes is bounded. This is a bounded probe, not an execution sandbox.
Interrupting the scan cancels the active probe and prevents subsequent probes.

An opt-in report uses schema v2 and `version_probes: true`, with separate
`application_version` observations and a `version: command_report_only` capability.
The observation distinguishes `reported`, `not_detected`, `unknown`, `error` and
`timeout`. A missing executable does not prove the agent is uninstalled; a failed
probe does not invalidate other inventory results. Individual probe failures are
reported in otherwise successful output; user cancellation returns a command error.

The source is explicitly `executable --version`, and `install_method` remains
`unknown`. A same-named executable can be a different program, so reported versions
are not verified provenance. Do not interpret them as a signature, package-manager
record or guarantee of a reproducible installation. Existing controller environment
`version` fields describe the reet reporter; they are not repurposed for agent
application versions. These local observations are not yet uploaded through MCP.

## Opt-in installation evidence

```sh
reet setup scan --inspect-installation --json
reet setup scan --components --inspect-installation --probe-versions --json
```

`--inspect-installation` reads local Node package metadata without invoking agents,
package managers, lifecycle scripts or network requests. It is independent of
configuration reads and version probes; the default metadata-only scan is unchanged.
Reports use schema v2, `installation_inspection: true`, and a separate `installation`
observation per agent. The installation capability is `local_node_package_evidence`
for the five supported adapters and `not_supported` for Cursor.

| Agent | Recognised package name |
| --- | --- |
| Claude Code | `@anthropic-ai/claude-code` |
| Codex | `@openai/codex` |
| OpenCode | `@opencode/cli`, legacy `opencode-ai` |
| Pi | `@earendil-works/pi-coding-agent`, legacy `@mariozechner/pi-coding-agent` |
| Gemini CLI | `@google/gemini-cli` |

Inspection resolves the executable's symlinks, then checks up to six directories,
starting with its containing directory, for the nearest `package.json`. It stops
at that boundary even if the package is unrelated or malformed. Metadata must be
a regular non-symlink file, at most 1 MiB, and valid bounded JSON without duplicate
keys. Its package name must match the adapter, its version must be a recognised
three-part token, and its explicit `bin` map entry for the discovered command must
identify the same regular file. The declared path must remain within the package
root, including through symlinks. Inspection rechecks executable identity, size
and modification time before returning a match; this is an observation, not an
atomic snapshot or proof that executable bytes are unmodified.

`matched` records `kind: node_package`, the recognised package/version, metadata
path and `evidence: package_bin_matches_executable`. Other results are
`not_detected`, `unknown` or `error` with fixed diagnostic codes. Unknown package
names, other metadata values and raw filesystem/parser errors are never returned.
Reported paths and prerelease/build version identifiers may still be private.

Local metadata can be edited or forged. `manager` always remains `unknown` because
npm, pnpm, Yarn, Bun and manual copies can share these layouts. A package version
is distinct from the application's `--version` report; neither overwrites the
other. Matching a wrapper does not inspect its native payload or dependencies.
Native installers, OS package receipts, Windows command shims, string-form `bin`
entries, Cursor packaging, signatures and registry authenticity are not verified.
Failure to find supported metadata does not imply the agent is uninstalled.
These observations must not automatically select an installer or authorise restore.

Package names and bin-map conventions were checked against the public npm registry
metadata on 22 October 2026. Tests use synthetic local fixtures; no installed
package code or registry payload is executed by the inspector or those tests.
