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
- Installation evidence merged in [PR #45](https://github.com/eyupio/reet.sh/pull/45).
- Upgrade/health regression fixes merged in [PR #46](https://github.com/eyupio/reet.sh/pull/46), `5373885`.
- CLI defaults and branding merged in [PR #47](https://github.com/eyupio/reet.sh/pull/47), `666964a1ab44d2e16853b93d26115805c7e9a36a`.
- Handoff refresh merged in [PR #48](https://github.com/eyupio/reet.sh/pull/48), `9fb357598aba062838249db10d1f76a92bbda2c7`.
- Latest verified source baseline: `origin/main` at `9fb3575` (2026-10-22).
- Export policy declarations are in [PR #49](https://github.com/eyupio/reet.sh/pull/49) on
  `feat/setup-export-policy`; fetch remote status before continuing and preserve any
  uncommitted work.
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
- [x] Verify PR #47's final CI and published dev artifact.
- [x] Implement explicit component export policies and document their limits.
- [x] Implement policy-driven backup preview without writing an archive.

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

The merged installation-evidence S1c slice adds `--inspect-installation`: bounded local Node package
metadata inspection for Claude Code, Codex, OpenCode, Pi and Gemini CLI. It checks
the known package name and declared executable against the discovered file, with
nearest-package boundaries and confined bin paths. This is local association
evidence only; package manager and authenticity remain unknown. Native installers,
Cursor packaging and Windows shims remain unsupported. No commands or registry
requests run during inspection.

Policy declarations are now opt-in: `reet setup scan --export-policy` (alone or
with `--components`) adds schema v2 per-agent policies without reading configuration
values. Each discovered source lists its component selection, allowed fields,
encrypted-only credential fields, preserved references and explicit exclusions.
Unlisted settings fields are excluded; memory, history, instructions, behaviour,
hooks and caches have explicit class policies; `settings.local.json` is opt-in.
Unknown formats and unsupported adapters remain explicit. This is a policy
declaration, not an export or backup. Authenticated provenance, package-manager
receipts and install resolution still need dedicated support before their restore
paths can be enabled. Current slices do not meet the complete S1 acceptance gate.
Do not copy configuration trees generically: they may mix secrets, history, caches
and hooks. No setup backup/restore command is implemented yet.

S2a now applies those policies in memory through
`reet setup backup --preview [--json] [--project PATH]`. It reports included,
credential-only, reference and excluded field names, MCP declaration counts, unknown
fields, per-component digests and fixed omission reasons. Opt-in sources are not
read; selected values never appear in the preview; nothing is written. `reet setup
backup` without `--preview` returns an explicit not-implemented error.

Next implementation slice: create an encrypted local snapshot from the selected
content, with user-held key/passphrase handling, atomic private writes, verification,
listing/inspection and an omissions report. Keep values inside the encrypted payload.
Do not start with restore execution or claim cross-OS recovery.

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

## CLI defaults and branding follow-up

PR #46 merged as `5373885` and its published dev binary was downloaded through
the installer and verified for version, explicit health URL and setup scan flags.
The user then requested REET_VERSION, target URL persistence, missing-value prompts,
and terminal branding/animation. This work merged in PR #47 as `666964a`;
its implementation branch was `fix/reet-version-environment`.

Implemented and merged: explicit flag > exported REET_VERSION/REET_URL > saved
`reet/client.env` defaults; fallback to the installed upgrade channel. The shell
installer and managed Compose install also honour exported REET_VERSION. New
`reet configure` saves both values, with TTY-only prompts. Missing required URL
prompts are integrated into healthcheck, scan, sync, enrollment and text doctor;
unknown upgrade versions can prompt. No token storage or project .env sourcing.
Local setup scans remain independent. Container checks explicitly use --local.

Terminal branding follows the existing burnt-orange palette; status animations
cover real update checks/downloads and remote health checks. JSON/redirected output
is untouched. NO_COLOR, TERM=dumb and REET_NO_ANIMATION disable animation.
The existing transitive go-isatty dependency is promoted to direct use; no new
module or version is introduced. Full Go vet/race checks, web checks (8 tests,
zero Svelte diagnostics), and all 16 MCP tests pass. A compiled-binary PTY smoke
test verified branding, interactive prompts, private saved defaults, later health
checks, animation and all disable controls, plus clean redirected output.
This was followed by the export policy slice below.

## PR #47 release verification and export policy slice

PR #47's final checks all passed: CodeQL, image `test` and binary `source`, `web`,
`verify` and all four build jobs. Its main-push binary release run `39769605396`
also completed successfully. The main-push image run for `666964a` was cancelled
because PR #48 superseded it; the PR image run passed. Later main-push workflows for
`9fb3575` refreshed the `dev` release.

The published `dev` Linux amd64 artifact was downloaded into an isolated temporary
home after the `9fb3575` refresh. It reported `reet dev
(9fb357598aba062838249db10d1f76a92bbda2c7)`, listed the setup, configure and
healthcheck commands, saved `REET_VERSION=dev` and `REET_URL=https://reet.sh` in a
mode-0600 client file, and used an explicit `--url` health target instead of the
saved value. No PR #47 regression was confirmed. Automated Codex review for PR #47
still shows only the quota-failure comment and no review findings; that is not an
automated approval.

[PR #49](https://github.com/eyupio/reet.sh/pull/49) adds
`reet setup scan --export-policy` as an independent opt-in (schema v2,
`export_policies: true`). It declares per-source component selection and
field handling for future encrypted snapshots without reading configuration values.
Supported settings use explicit allowlists; unlisted fields are excluded. MCP
launch parameters are selected, URL and literal environment/header values are
encrypted-only, and environment reference names are preserved without resolution.
Hooks, credential helpers, project history and account identity are excluded.
Class policies explicitly mark instructions, behaviour, memory and history as not
implemented (history is opt-in), credentials as references-only, and hooks and
caches as never collected. `settings.local.json` is opt-in. Unknown formats and
unsupported adapters remain explicit. Policy source observations use the exact
candidate file state for present locations and inherit missing/symlink/error states
without inspecting targets or contents.

Policy validation: focused setup tests cover every adapter/source, component and
field policy values, credential-field exclusion, local-settings opt-in, unsupported
formats/adapters, class coverage, CLI opt-in, schema preservation and secret absence
in JSON/text. Full `go vet ./...` and `go test -race ./...` passed. Web checks passed
(8 tests; zero Svelte diagnostics) and all 16 MCP tests passed. A compiled-binary
smoke scan verified default schema v1, opt-in policy output and no secret leakage.
`git diff --check` passed. No dependency changed.

## Policy-driven backup preview

The current stage on `feat/setup-snapshot-manifest` adds
`reet setup backup --preview [--json] [--project PATH]`. It applies the declared
policies to bounded known sources and reports included, credential-only, reference
and excluded field names, MCP declaration counts, unknown-field counts,
per-component SHA-256 digests and fixed omission reasons. Selected values stay in
unexported in-memory content only; the preview never prints them and writes nothing.
Opt-in sources such as `settings.local.json` are not read. Credential-bearing fields
are counted but not selected, references retain names without resolution, and
unsupported classes remain explicit. `reet setup backup` without `--preview` returns
an explicit not-implemented error.

Preview validation: focused setup tests cover policy application, settings/MCP field
selection, omitted credentials and unknown fields, opt-in sources not being read,
symlinked and oversized sources, invalid MCP declarations, stable preview identity,
CLI preview-only behaviour, no writes and secret absence in JSON/text. Full
`go vet ./...` and `go test -race ./...` passed. Web checks passed (8 tests; zero
Svelte diagnostics) and all 16 MCP tests passed. A compiled-binary smoke test verified
preview output, secret absence and refusal of backup writing; setup tests cross-compile
for Windows amd64 and macOS arm64. `git diff --check` passed. No dependency changed.

This is still not a backup: encryption, key/passphrase handling, archive writing,
listing/inspection, S3 delivery, restore planning/execution/verification and
cross-OS translation are not implemented. Digests cover selected component content,
not complete files or provenance.

## Resume checklist (2026-10-22)

1. Read this file, `docs/agent-setup-plan.md`, `docs/setup-discovery.md`, and current
   repository instructions. Fetch GitHub state; this document records observations,
   not a guarantee that CI or release state remains unchanged.
2. PR #47's final checks and published `dev` artifact were verified at `9fb3575`
   as recorded above. Recheck live CI/release state before relying on it.
3. Automated Codex review for PR #47 and PR #48 failed to run because the account
   review quota was exhausted. This is not a completed review or a validated code
   finding. Inspect actual review threads and perform code review independently;
   do not claim automated approval.
4. Explicit component export policies and the policy-driven backup preview are now
   implemented. The next small slice is encrypted local snapshot writing from the
   selected content, with user-held key/passphrase handling, atomic private writes,
   verification, listing/inspection and an omissions report. Keep values inside the
   encrypted payload and unknown formats unsupported. Do not enable restore execution
   or cross-OS claims before their acceptance gates. Consult the detailed plan for
   encryption, key handling, restore and compatibility requirements.
5. Create a PR per stage, run required Go/web checks plus MCP tests, review changes,
   merge only when checks are green, then continue on a fresh branch. The owner
   already authorised this workflow; do not ask again. Keep this handoff current
   with actual commits, tests, limitations and next work. New product decisions
   should use clickable options with a recommendation when the UI supports them.

Important boundaries: no complete AI setup backup/restore has shipped. Existing
`reet backup` is not evidence that the planned setup milestone is complete.
Saved REET_URL selects client requests; it does not make `reet upgrade` remotely
manage that server. No production host was upgraded by this session. Do not copy
live MCP inventory or credentials into fixtures, logs or documentation.

After the new dev release is verified, a user with the old stable CLI can recover
with the installer, then save their preferences:

```sh
curl -fsSL https://raw.githubusercontent.com/eyupio/reet.sh/main/scripts/install.sh | sh -s -- --version dev
hash -r
reet --version
reet configure --version dev --url https://reet.sh
reet upgrade --check
reet healthcheck
```

Installing the CLI does not restart a separately running server. Inspect its
actual managed deployment before applying any host upgrade.
