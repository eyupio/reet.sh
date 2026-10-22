# AI agent setup handoff

## Goal and agreed scope

Expand reet from skills management to portable AI agent setups. The first
milestone is complete backup and restore of supported setups on a new machine;
profile sync follows, then reviewed updates/diagnostics/rollback.

The authoritative detailed plan is [docs/agent-setup-plan.md](docs/agent-setup-plan.md).
It records the owner's decisions, architecture, delivery slices, acceptance gates,
validation and compatibility constraints. Do not reopen settled choices without
new evidence.

Key choices: broad popular-agent discovery (Claude Code, Codex, OpenCode, Pi and
others); explicit verified support/gaps; standalone local CLI; local preview and
confirmation; verified cross-OS restore; exact recorded versions; per-conflict
choices and rollback copies; reusable memory included; history opt-in; portable
credentials optionally encrypted with a user-held key/passphrase; local encrypted
storage plus optional S3; named profiles with machine/project overrides.

## Branch and baseline

- S0 merged in [PR #39](https://github.com/eyupio/reet.sh/pull/39) after both CI workflows passed.
- S1a merged in [PR #40](https://github.com/eyupio/reet.sh/pull/40) after both CI workflows passed.
- S1b merged in [PR #41](https://github.com/eyupio/reet.sh/pull/41) after both CI workflows passed.
- Version probes merged in [PR #44](https://github.com/eyupio/reet.sh/pull/44) after both CI workflows passed.
- Current branch: `feat/setup-installation-evidence`.
- Base: `origin/main` at `cb3e88d`.
- Existing skill scan/sync, hosted MCP, lifecycle and backup behaviour must remain
  compatible. Hosted MCP access is distinct from managing installed MCP servers.
- No implementation capability is implied by this planning document.

## Progress

- [x] Inspect existing scanner, CLI, backup primitives and repository guidance.
- [x] Write the detailed plan and this handoff.
- [x] Open a draft PR containing the plan before beginning implementation: [#39](https://github.com/eyupio/reet.sh/pull/39).
- [x] Implement S0: local agent discovery and capability reporting.
- [x] Add fixtures and CLI tests; update README with actual commands/limitations.
- [x] Run required checks and record results below.

## Implemented slice

`internal/setup` and `reet setup scan [--json] [--project PATH]` now provide
metadata-only path discovery and executable lookup without executing agents
or reading configuration contents. Six declarative adapters cover Claude Code,
Codex, OpenCode, Pi, Cursor and Gemini CLI. Schema v1 reports explicit observation
states and backup/restore as not implemented. Shared files do not establish that
an agent is installed. Optional path overrides and explicit project scope are
covered by tests. Details: [setup discovery](docs/setup-discovery.md).

## Next implementation task

S1a is merged: `reet setup scan --components` opts into bounded
strict-JSON settings/MCP summaries with schema v2. Default schema v1 remains
metadata-only. Per-source partial/error/unsupported states, credential/reference
counts, parser limits and inode checks are tested. No config values are returned.
See the discovery guide for exact coverage and limitations.

S1b is merged: Codex TOML and OpenCode JSONC parsers use the same
summary/output contract. Codex authentication references and its permission
settings are recognised. TOML uses pinned `go-toml/v2 v2.4.3` rather than a partial
home-grown decoder. JSONC comments/trailing commas are normalised before strict
JSON validation. Format-specific failures remain constant diagnostic codes.

The first S1c slice is merged: `--probe-versions` invokes discovered
absolute executables with a fixed `--version` argument. Per-command timeout,
bounded stdout, discarded stderr, recognised-version-only output and cancellation
are covered by tests. `application_version` is separate from the existing
controller reporter `version`; installation method remains explicitly unknown.

The next S1c slice adds `--inspect-installation`: bounded local Node package
metadata inspection for Claude Code, Codex, OpenCode, Pi and Gemini CLI. It checks
the known package name and declared executable against the discovered file, with
nearest-package boundaries and confined bin paths. This is local association
evidence only; package manager and authenticity remain unknown. Native installers,
Cursor packaging and Windows shims remain unsupported. No commands or registry
requests run during inspection.

Next: implement explicit component export policies; authenticated provenance,
package-manager receipts and install resolution still need dedicated support
before their restore paths can be enabled. Current slices do not meet
the complete S1 acceptance gate. Do not copy
configuration trees generically: they may mix secrets, history, caches and hooks.
No setup backup/restore command is implemented yet.

## Validation and continuation

Repository guidance is in `CONTRIBUTING.md`; no AGENTS.md was found in the
workspace during initial inspection. Required commands:

```sh
cd web && npm run check
cd .. && go vet ./... && go test -race ./...
```

This workspace has toolchains in `/workspace/.reet-tools/activate.sh`; source it
before local Go/Node commands. This is a workspace convenience, not a product
runtime requirement. Network commands may require sandbox network permission.

Validation completed on Linux on 22 October 2026:

- `go test ./internal/setup` and `go test -race ./internal/setup`: passed.
- `go vet ./...`: passed.
- `go test -race ./...`: passed across all packages.
- `npm run check` in `web/`: 8 tests passed; Svelte reported zero errors/warnings.
- `npm ci && npm test` in `mcp/`: 14 tests passed, including OAuth and real API tests.
- `go build -o /tmp/reet-setup-discovery ./cmd/reet`: passed.
- Compiled CLI smoke test with temporary home/project: JSON/text/help, explicit
  project scope, omitted synthetic secret, unchanged configuration and rejection
  of the unimplemented restore command all passed.
- `git diff --check`: passed.

Initial checks needed the existing Go/MCP dependencies downloaded with sandbox
network access; reruns then passed. No dependency or lockfile changes were made.
No macOS/Windows runtime or cross-OS restore verification has been performed.
S0 merged after image and binary-release CI passed for `2d99832`.

## S1a validation

S1a validation on Linux: focused setup race tests, `go vet ./...`, full
`go test -race ./...`, web checks (8 tests; zero Svelte diagnostics) and MCP tests
(14 tests) all passed. Compiled CLI smoke tests verified schema v1/v2 opt-in,
credential-value omission, literal/reference counts, text output and explicit
unsupported TOML reporting. `git diff --check` passed. No new dependencies.

## S1b validation

S1b focused race tests passed, including TOML/JSONC parsing, credentials omitted
from JSON/text, malformed/duplicate/oversized trees and invalid auth references.
`go vet ./...`, `go test -race ./...`, web checks (8 tests; zero Svelte diagnostics),
and MCP tests (14) passed. A short JSONC fuzz run passed 51,557 executions with
no failures. Compiled CLI smoke tests parsed both formats without exposing values.
`git diff --check` passed. Dependency change: `go-toml/v2 v2.4.3`, pinned for correct
TOML decoding; `go mod tidy` also recorded an existing test dependency checksum.

## Version-probe validation and connected sources

Version-probe focused race tests, `go vet ./...`, and full `go test -race ./...`
passed. Web checks passed (8 tests; zero Svelte diagnostics), as did all 16 MCP
tests. Compiled CLI fixture smoke tests passed for default discovery, opt-in
probes, and combined component/probe flags. `git diff --check` passed. No new
dependencies. Runtime verification remains Linux-only.
The reet MCP connection was verified with read-only environment and sync-target
queries. Its existing environment `version` is reporter metadata, confirmed by
the scanner/store code; keep application versions separate. No live inventory,
host identifiers or personal configuration has been copied into test fixtures or
this repository, and no live MCP data was modified.

Merge each stage only after its PR CI passes; the owner has authorised merging
green stages and continuing on fresh branches. Preserve this rule in subsequent
handoffs. Do not equate fixture-backed format parsing with verified restoration.

## Installation-evidence validation

Focused setup race tests passed, covering recognised packages, false associations,
ancestor/read bounds, malformed metadata, symlinks, path escapes, unavailable
executables, opt-in behaviour and output omission. `go vet ./...` and full
`go test -race ./...` passed. Web checks passed (8 tests; zero Svelte diagnostics)
and all 16 MCP tests passed. Compiled CLI fixtures verified default/combined flags
and distinct package/command versions. Setup tests cross-compiled for Windows
amd64 and macOS arm64; runtime verification remains Linux-only.
`git diff --check` passed. No new dependencies. Package identity is not cryptographically authenticated.

PR review identified renamed OpenCode/Pi packages. Registry metadata confirmed
`@opencode/cli` and `@earendil-works/pi-coding-agent`; the latter was also checked
against the upstream package manifest. Both current names and legacy aliases are
recognised, with fixtures asserting the actual observed identity is preserved.

## Upgrade regression interruption

User testing exposed that plain upgrade defaulted from dev to latest stable,
removing new commands. The published dev artifact itself was verified as correctly
labelled `dev` at `40b7c65`; the misleading version in the preview came from stale
deployment metadata paired with the current CLI commit. The fix preserves the dev
channel for the actual target build, keeps Compose selection tied to its image
record, and labels CLI versus recorded deployment versions separately. Explicit
version choices still override defaults. `healthcheck --url` now parses and uses
its requested target instead of silently ignoring arguments. Setup expansion
continues with component export policies after this regression fix.

Regression validation: targeted lifecycle/CLI race tests, full `go vet ./...` and
`go test -race ./...`, web checks (8 tests, zero Svelte diagnostics) and all 16 MCP
tests passed. Compiled CLI smoke tests verified explicit health base paths,
unknown-flag rejection and the current setup scan options. No new dependencies.
