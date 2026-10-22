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
- Export policy declarations merged in [PR #49](https://github.com/eyupio/reet.sh/pull/49), `bf77b247b8a75903c8d7eab2fe1f82eb30c96d8b`.
- Policy-driven backup preview merged in [PR #51](https://github.com/eyupio/reet.sh/pull/51), `e309a6f`.
- Shared sealed archive primitive merged in [PR #52](https://github.com/eyupio/reet.sh/pull/52), `2e9cfb6`.
- Encrypted local setup snapshot writing merged in [PR #53](https://github.com/eyupio/reet.sh/pull/53), `fd9cdf5`.
- Snapshot listing/inspection merged in [PR #54](https://github.com/eyupio/reet.sh/pull/54), `cf10a02`.
- Restore planning merged in [PR #55](https://github.com/eyupio/reet.sh/pull/55), `36cead3`.
- Create-only restore apply merged in [PR #57](https://github.com/eyupio/reet.sh/pull/57), `ff453c3`.
- Conflict policies and rollback copies merged in [PR #58](https://github.com/eyupio/reet.sh/pull/58), `b505377`.
- Destination verification merged in [PR #59](https://github.com/eyupio/reet.sh/pull/59), `f313ff1ce70d652f45ac418a65b4577f696e04ce`.
- Final handoff documentation merged in [PR #60](https://github.com/eyupio/reet.sh/pull/60), `9c9593b`.
- Rollback of completed apply journals merged in [PR #61](https://github.com/eyupio/reet.sh/pull/61), `c2b129a`.
- Latest verified source baseline: `origin/main` at `c2b129a` (2026-10-22).
- Per-destination conflict choices are in [PR #62](https://github.com/eyupio/reet.sh/pull/62)
  on `feat/setup-per-file-conflicts`; fetch remote status before continuing and preserve
  any uncommitted work.
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
- [x] Extract the shared sealed archive primitive for local setup snapshots.
- [x] Write and verify encrypted local setup snapshots from the policy selection.
- [x] List clear snapshot headers and inspect payload summaries with a passphrase.
- [x] Preview a destination restore plan with conflicts and unsupported items.
- [x] Apply create-only plans with verification, a journal and rollback of that run.
- [x] Add conflict skip/replace policies and replacement rollback copies.
- [x] Verify selected destination fields without repairing or running agents.
- [x] Verify the latest `dev` binary contains the complete setup command family.
- [x] Add rollback of completed apply journals with snapshot and digest checks.
- [x] Add per-destination conflict skip/replace choices.

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
and hooks. Setup backup now writes encrypted local snapshots; setup restore is not
implemented.

S2a now applies those policies in memory through
`reet setup backup --preview [--json] [--project PATH]`. It reports included,
credential-only, reference and excluded field names, MCP declaration counts, unknown
fields, per-component digests and fixed omission reasons. Opt-in sources are not
read; selected values never appear in the preview; the preview itself writes nothing.
Writing an encrypted snapshot is the next slice below.

The sealed archive primitive now lives in `internal/sealed`, shared by instance/shelf
backups and future setup snapshots without making `internal/setup` depend on the
backup service, store or SQLite. The backup package keeps its public names through
aliases and wrappers, so existing formats and callers are unchanged.

S2b now writes a passphrase-encrypted local setup snapshot from that selection using
`internal/sealed`: `reet setup backup --password-file FILE [--dir DIR] [--json]`.
It refuses command-line passphrase values, writes privately and atomically, verifies
the seal before and the file after the write, and keeps paths, selected values and
omissions inside the encrypted payload. Listing/inspection and restore remain later
slices.

S2c now adds `reet setup list [--json] [--dir DIR]` and
`reet setup inspect [--json] [--dir DIR] [--password-file FILE] SNAPSHOT`. Listing
reads clear headers only and never needs a passphrase; inspection shows the header,
then opens and verifies the encrypted preview and source summaries only when a
passphrase is supplied. Neither prints selected values.

S3 restore planning opens and verifies a snapshot, discovers the destination
metadata-only, maps global sources by agent/scope/name, requires `--project` for
project sources, and reports `create`, `conflict` or `blocked` per destination file
plus unsupported/omitted items. It produces a stable plan fingerprint and writes
nothing without `--apply`.

S4a adds `reet setup restore --apply`. Without `--on-conflict`, it writes only when
every action is `create`. `--on-conflict skip` leaves conflicts unchanged and creates
the rest; `--on-conflict replace` keeps a mode-0600 rollback copy and replaces each
conflicting regular file. Blocked actions still refuse the run. New settings/MCP files
are written atomically, rechecked, parsed again and compared with the selected content.
A value-free journal records destinations, states and rollback-copy paths. A failed run
removes created files and restores replaced ones from their copies.

S4b now adds `reet setup verify [--json] [--dir DIR] [--password-file FILE]
[--project PATH] SNAPSHOT`. It compares the snapshot's selected settings fields and MCP
declarations with mapped destination files and reports `verified`, `missing`,
`changed`, `blocked` or `unsupported` per component, listing mismatch names but never
values. It does not repair, rewrite, roll back or run agents. Project sources need
`--project`.

S4c now adds `reet setup rollback --journal FILE --password-file FILE SNAPSHOT`. It
requires the snapshot, verifies the journal's snapshot identity, applied digests,
current destination hashes and rollback-copy paths before changing anything. It removes
files the apply created, restores replaced files from their rollback copies, marks the
journal `rolled_back` and is idempotent when rerun. It refuses changed destinations,
failed journals, mismatched snapshots and unsafe backup paths.

S4d now accepts repeatable `--conflict PATH=skip|replace` choices that override
`--on-conflict` for exact plan destinations. Choices that do not match a conflict, or
that target unknown paths, are refused before writing.

Next implementation slice: add runtime health guidance after restore/verify without
silently repairing or executing arbitrary hooks. Do not claim cross-OS restore or a
complete working-setup restore before their acceptance gates.

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

[PR #51](https://github.com/eyupio/reet.sh/pull/51) added
`reet setup backup --preview [--json] [--project PATH]`. It applies the declared
policies to bounded known sources and reports included, credential-only, reference
and excluded field names, MCP declaration counts, unknown-field counts,
per-component SHA-256 digests and fixed omission reasons. Selected values stay in
unexported in-memory content only; the preview never prints them and writes nothing.
Opt-in sources such as `settings.local.json` are not read. Credential-bearing fields
are counted but not selected, references retain names without resolution, and
unsupported classes remain explicit. The preview itself is read-only; writing was
added in the next slice.

Preview validation: focused setup tests cover policy application, settings/MCP field
selection, omitted credentials and unknown fields, opt-in sources not being read,
symlinked and oversized sources, invalid MCP declarations, stable preview identity,
CLI preview-only behaviour, no writes and secret absence in JSON/text. Full
`go vet ./...` and `go test -race ./...` passed. Web checks passed (8 tests; zero
Svelte diagnostics) and all 16 MCP tests passed. A compiled-binary smoke test verified
preview output, secret absence and writing refusal at that stage; setup tests
cross-compiled for Windows amd64 and macOS arm64. `git diff --check` passed. No
dependency changed.

The preview itself is not a backup. Encryption and writing were added in the next
slice; listing/inspection, S3 delivery, restore planning/execution/verification and
cross-OS translation remain unimplemented. Digests cover selected component content,
not complete files or provenance.

## Shared sealed archive primitive

`internal/sealed` now owns the existing AES-256-GCM/Argon2id archive format, manifest,
password/compression options, digest verification and the 512 MiB in-memory bound.
`internal/backup` aliases the exported types/constants and wraps the entry points, so
its public API, stored formats and tests are unchanged. `internal/setup` now uses the
same verified primitive without importing the backup service, SQLite or server code.

Validation: focused `internal/sealed` tests cover key/password round trips,
compression, wrong key/password, tampering and truncation. Existing backup tests pass
unchanged. Full `go vet ./...` and `go test -race ./...` passed. No dependency or
format changed.

## Encrypted local setup snapshots

The current stage on `feat/setup-snapshot-write` makes `reet setup backup` write one
passphrase-encrypted snapshot when `--preview` is absent. `--password-file FILE`
(or `-` for standard input) supplies a passphrase of at least 12 characters; there is
no command-line passphrase value and no stored key. `--dir` selects the snapshot
directory, otherwise `REET_SETUP_DIR` or the user config `reet/setup` directory is
used. Files are `setup-<timestamp>-<nonce>.reetsetup`, mode 0600 in a mode-0700
directory, written through a synced temporary file and rename. Symlinked or non-
directory destinations are refused.

Snapshots use the shared `internal/sealed` AES-256-GCM/Argon2id archive with gzip.
The clear header carries only the ID, kind/target `setup`, time, format version,
payload size/digest and protection/compression. Source paths, selected values, MCP
content and omissions are inside the encrypted payload. The writer opens the sealed
bytes before writing and opens the file on disk again afterwards; failure removes the
file. Credential-bearing values, opt-in sources, memory, history, hooks and caches
remain excluded by the policy.

Validation: focused setup tests cover encrypted write/read, selected content and
omitted credentials, wrong passphrase, tampering, digest/schema mismatch, atomic
private permissions, no temporary plaintext, missing/short passphrase, symlink and
file destinations, CLI JSON/text output and secret absence. Full `go vet ./...` and
`go test -race ./...` passed. Web checks passed (8 tests; zero Svelte diagnostics) and
all 16 MCP tests passed. A compiled-binary smoke test verified the written
snapshot header, mode-0600 file, clean ciphertext and result output, that preview
still works, and that a missing passphrase is refused. Setup tests cross-compile for
Windows amd64 and macOS arm64. `git diff --check` passed. No dependency changed.

Listing/inspection, S3 delivery, restore planning/execution/verification, dependency
resolution and cross-OS translation are not implemented. This is not yet a complete
AI setup backup: unsupported components remain omissions and no restored setup has
been verified.

## Snapshot listing and inspection

The current stage on `feat/setup-snapshot-list` adds `reet setup list` and
`reet setup inspect`. Listing reads clear headers only, sorts by recorded time and
reports IDs, paths, sizes, protection and compression. It skips symlinks, temporary
files, invalid headers and other archive kinds, ignores unrelated files and treats a
missing directory as empty. Inspection always shows the clear header; with
`--password-file FILE` (or `-`) it verifies and opens the payload, then shows the
policy preview and per-source component/field/declaration counts and digests. It never
prints selected values, contents or credentials. Snapshot IDs resolve under `--dir`,
`REET_SETUP_DIR` or the default directory; flags come before the snapshot argument.

Validation: focused setup tests cover header-only listing, newest-first ordering,
skipped symlinks/temporaries/invalid/other-kind archives, missing and symlinked
directories, locked and opened inspection, wrong passphrases, ID resolution, CLI
JSON/text output and secret absence. Full `go vet ./...` and `go test -race ./...`
passed. Web checks passed (8 tests; zero Svelte diagnostics) and all 16 MCP tests
passed. A compiled-binary smoke test verified backup, list, locked inspection and
opened inspection without value leakage. `git diff --check` passed. No dependency
changed.

Restore execution was added after this slice; conflict choices, rollback copies,
runtime verification, S3 delivery, dependency resolution and cross-OS translation
remain unimplemented. Listing or inspection success is not proof of a complete or
restorable setup.

## Restore planning

The current stage on `feat/setup-restore-plan` adds `reet setup restore` as a
read-only plan. It requires the snapshot passphrase, opens and verifies the payload,
discovers destination metadata, maps global sources by destination agent/scope/file
name and maps project sources only with `--project`. Each destination file is
`create`, `conflict` or `blocked`; conflicts are not merged or resolved yet. Missing
destination locations and project sources without a project are unsupported rather
than guessed. Omitted policy records and unsupported classes remain listed, and a
stable SHA-256 fingerprint covers snapshot identity, destination facts, actions and
unsupported entries. No values are printed and nothing is written; create-only apply
is the next slice.

Validation: focused setup tests cover source mapping, create/conflict/blocked states,
symlinked destinations, project opt-in, unsupported/omitted reporting, stable and
changed fingerprints, CLI JSON/text output, missing/wrong passphrases, plan-only
behaviour and secret absence. Full `go vet ./...` and `go test -race ./...` passed. Web
checks passed (8 tests; zero Svelte diagnostics) and all 16 MCP tests passed. A
compiled-binary smoke test verified create and conflict plans, a stable fingerprint
and destination non-mutation; setup tests cross-compile for Windows amd64 and macOS
arm64. `git diff --check` passed. No
dependency changed.

Restore execution, per-conflict choices, rollback copies, dependency/version
resolution, installers, hooks, authentication and post-restore verification remain
unimplemented. A plan is not proof that a working setup can be restored.

## Create-only restore apply

The current stage on `feat/setup-restore-apply` enables `reet setup restore --apply`.
It creates only new files: the whole run is refused before writing if any action is
`conflict` or `blocked`. Destinations and their immediate parents are rechecked, and
a changed destination invalidates the plan. Settings and MCP content is written as a
new JSON, JSONC or TOML file with mode 0600; missing parent directories use mode 0700.
Each file is parsed again and compared with the selected content, and a mismatch
removes it. A journal under the snapshot directory records destinations, states and
errors, never selected values; files created by a failed run are removed in reverse
order.

Validation: focused setup tests cover create-only apply, settings/MCP round trips,
TOML output, verification and permission checks, conflict refusal, destination change
invalidation, injected mid-run failure with rollback and journal states, CLI JSON/text
output and secret absence. Full `go vet ./...` and `go test -race ./...` passed. Web
checks passed (8 tests; zero Svelte diagnostics) and all 16 MCP tests passed. A
compiled-binary smoke test verified create-only apply, mode-0600 output, clean JSON and
journal output, conflict refusal and destination non-mutation; setup tests cross-compile
for Windows amd64 and macOS arm64. `git diff --check` passed. No
dependency changed.

Per-conflict choices, replacement/merge, rollback copies, a rollback command,
installers, hooks, package managers, authentication, dependency/version resolution,
cross-OS translation and post-restore health verification remain unimplemented. This
is not the complete backup/restore milestone.

## Destination verification

`reet setup verify` merged in [PR #59](https://github.com/eyupio/reet.sh/pull/59). It
opens and verifies the snapshot, discovers the destination and compares the selected
settings fields and MCP declarations with mapped files. Extra destination fields are ignored. Components
are `verified`, `missing`, `changed`, `blocked` or `unsupported`; `changed` reports
field/declaration names only. Project sources need `--project`. Policy omissions and
unsupported classes remain listed.

Verification never repairs, rewrites, rolls back or runs an agent. It does not check
application versions, installation provenance, authentication, MCP health, hooks or
runtime behaviour. It is not proof that the setup works or is portable to another OS.

Validation: focused setup tests cover verified, changed, missing and extra-field cases,
project opt-in, mismatch names without values, no destination mutation, CLI JSON/text
output, runtime limitations and secret absence. Full `go vet ./...` and
`go test -race ./...` passed. Web checks passed (8 tests; zero Svelte diagnostics) and
all 16 MCP tests passed. A compiled-binary smoke test verified apply-then-verify,
changed detection with mismatch names, and value-free output; setup tests cross-compile
for Windows amd64 and macOS arm64. `git diff --check` passed. No dependency changed.

A rollback command, per-file conflict choices, runtime health checks, dependency/version
resolution, installers, authentication, S3 delivery and cross-OS translation remain
unimplemented.

## Final dev release verification

The main-push binary release for `f313ff1` completed successfully and refreshed the
rolling `dev` release. The published Linux amd64 artifact was downloaded into an
isolated temporary home and reported `reet dev
(f313ff1ce70d652f45ac418a65b4577f696e04ce)`. Its help lists `setup scan`, `backup`,
`list`, `inspect`, `restore` and `verify` with the current flags, including
`--export-policy`, passphrase files, conflict policies and `--apply`. No production host
was upgraded and no live configuration was used or modified.

## Rollback of applied setup changes

The current stage on `feat/setup-rollback` adds `reet setup rollback --journal FILE
--password-file FILE SNAPSHOT`. The journal schema now records applied and rollback-copy
SHA-256 values. Before changing anything, rollback checks that the journal belongs to
the snapshot, each applied digest matches the snapshot content and current destination,
and each rollback copy is a regular file inside the journal directory with the recorded
digest. Created files are removed and replaced files are restored; skipped conflicts are
left alone. The journal becomes `rolled_back` (rerunning is a no-op), and rollback copies
are retained.

Validation: focused setup tests cover create/replace reversal, original-content
restoration, created-file removal, idempotent rerun, changed-destination refusal,
wrong-snapshot and malformed-journal refusal, CLI JSON/text output and secret absence.
Full `go vet ./...` and `go test -race ./...` passed. Web checks passed (8 tests; zero
Svelte diagnostics) and all 16 MCP tests passed. A compiled-binary smoke test verified
apply-then-rollback, original-content restoration, journal digests/state and idempotent
rerun; setup tests cross-compile for Windows amd64 and macOS arm64. `git diff --check` passed. No dependency changed.

Per-file conflict choices, runtime health checks, dependency/version resolution,
installers, authentication, S3 delivery and cross-OS translation remain unimplemented.
A rollback interrupted by an OS error is best-effort and records `rollback_failed`; it is
not a filesystem transaction. This is not the complete backup/restore milestone.

## Per-destination conflict choices

The current stage on `feat/setup-per-file-conflicts` adds repeatable
`--conflict PATH=skip|replace` options to `reet setup restore --apply`. The path must
match a destination shown in the plan and must be a conflict; unknown paths, non-conflict
actions, invalid policies and duplicate choices are refused before writing. A per-path
choice overrides `--on-conflict`, so different conflicts can be skipped or replaced in
one run. Journal and rollback behaviour are unchanged.

Validation: focused setup tests cover mixed replace/skip choices, unknown and
non-conflict paths, invalid policies, CLI repeatable flags and destination
non-mutation on refusal. Full `go vet ./...` and `go test -race ./...` passed. Web
checks passed (8 tests; zero Svelte diagnostics) and all 16 MCP tests passed. A
compiled-binary smoke test verified mixed per-file replace/skip choices and refusal of an
unknown destination choice; setup tests cross-compile for Windows amd64 and macOS arm64. `git diff --check` passed. No
dependency changed.

Runtime health checks, merge, dependency/version resolution, installers, authentication,
S3 delivery and cross-OS translation remain unimplemented.

## Resume checklist (2026-10-22)

1. Read this file, `docs/agent-setup-plan.md`, `docs/setup-discovery.md`, and current
   repository instructions. Fetch GitHub state; this document records observations,
   not a guarantee that CI or release state remains unchanged.
2. PR #47's final checks and published `dev` artifact were verified at `9fb3575`
   as recorded above. Recheck live CI/release state before relying on it.
3. Automated Codex review for PR #47 onward failed to run because the account review
   quota was exhausted. This is not a completed review or a validated code finding.
   Inspect actual review threads and perform code review independently; do not claim
   automated approval.
4. Explicit component export policies, the policy-driven preview, the shared sealed
   archive, encrypted local setup snapshot writing, listing/inspection, restore
   planning, create-only apply, conflict skip/replace with rollback copies, destination
   verification, rollback and per-destination conflict choices are now implemented. The
   next small slice is runtime health guidance. Keep values encrypted and unknown formats
   unsupported. Do not claim cross-OS restore or working-setup verification before their
   acceptance gates. Consult the detailed plan for encryption, key handling, restore and
   compatibility requirements.
5. Create a PR per stage, run required Go/web checks plus MCP tests, review changes,
   merge only when checks are green, then continue on a fresh branch. The owner
   already authorised this workflow; do not ask again. Keep this handoff current
   with actual commits, tests, limitations and next work. New product decisions
   should use clickable options with a recommendation when the UI supports them.

Important boundaries: no complete AI setup backup/restore has shipped. Setup backup
now writes a partial encrypted local snapshot and can list, inspect, plan, create or
replace supported files, verify selected destination fields, roll back a completed apply
and apply per-destination conflict choices, but runtime health and cross-OS recovery are
not implemented. Existing `reet backup` is not evidence that
the planned setup milestone is complete.
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

## Scanner token follow-up

The user's terminal output confirms the published PR #47 dev binary at `666964a`
was installed successfully, but `reet scan` prompted only for URL and then got 401.
Branch `fix/scan-token-input` fixes this with explicit `scan --env-file .env`
(reads only literal REET_TOKEN) and a hidden terminal prompt when no token is
available. Precedence: --token > exported REET_TOKEN > explicit file > prompt.
Prompted tokens are session-only; no secrets are saved in client.env. Dry runs do
not read/prompt for tokens. Noninteractive unauthenticated servers remain supported;
401 now gives actionable token guidance. No automatic project .env execution.
`golang.org/x/term v0.47.0` is used for portable terminal echo suppression/restoration.
Continue release verification and component export policies after this fix.

Validation for the token fix: full `go vet ./...` and `go test -race ./...`
passed; focused scan race tests passed after the file-type check refinement.
Web checks passed (8 tests, zero Svelte diagnostics); all 16 MCP tests passed.
A compiled-binary PTY test confirmed hidden token entry, authenticated submission,
no token in terminal output and restoration of terminal echo. Unit/integration
coverage includes precedence, literal env parsing/rejections, no secret diagnostics,
noninteractive behaviour and dry-run isolation. `git diff --check` passed.

### Consistent missing-input behaviour

The owner expanded the requirement from scan-only token prompts to all commands:
resolve existing values first, then prompt for missing command inputs. PR #50 is
being expanded on the same branch with shared `clientconfig.Require` and
`ResolveToken`. Shared tokens/explicit env files now cover scan, sync, enrollment
and text doctor. Sync/skill restore prompt for target/entry IDs. Backup open,
escrow/unescrow and restore prompt for missing files, IDs and needed secrets;
restore confirmation remains explicit. The backup service exposes a typed missing
password error so the CLI prompts only when a stored password is unavailable.
Background reporting and JSON diagnostics suppress interactive input. Enrollment
credentials remain scoped to their server; no token is saved in client.env.
The command coverage and intentional noninteractive behaviour are documented in
`docs/binary-installation.md` under Missing command inputs.

Expanded-scope validation: full Go vet/race suite passed (including API and backup
packages); focused command/clientconfig/scan/lifecycle race tests passed after the
stderr routing refinement. Web checks and all 16 MCP tests passed. Additional PTY
smokes verified sequential target/token and escrow path/passphrase prompts, secret
non-disclosure and restored echo. Required-input unit tests verify existing values
skip prompts and noninteractive missing values fail without consuming stdin.
