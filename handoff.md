# Reet improvements handoff

## Current status — 23 October 2026

This file is the current implementation summary. Historical delivery notes and
PR evidence are preserved in [handoff-history.md](handoff-history.md); statements
in that archive describe their original stage, not today's capability.
The detailed scope and acceptance gates remain in
[docs/agent-setup-plan.md](docs/agent-setup-plan.md).

The improvements branch is `improvements/setup-safety-and-ux`, based on code baseline `1a25202` and synchronized with the documentation updates
through `1ff8cf9` (PR #113). These improvements require the normal PR checks and
release workflows. Do not infer that published artifacts contain them before those
workflows finish.

## Product organisation

The main destinations are **Skills**, **Lists**, **Setups** and **Machines**.
Community, Teams, hosting skills, Account and Admin sit under More. Existing
`/profiles`, `/account/machines` and `/account/sync` links still work; `/setups`
is also accepted. The main machine routes are `/machines` and `/machines?view=compare`.
The rose, warm Library palette and Yorkshire voice remain in place.

- Skills retain personal shelf stages. A stage is independent of installation
  inventory and membership in a setup snapshot.
- Lists offer compact rows and cards, owner/audience/updated details, search and
  an All accessible filter. Explicit sharing is separate from admin access.
- Prepare a machine on a list opens comparison with that list selected; it does
  not select a target, save desired state, install files or approve a plan.
- Setups show coverage after global and selected machine/project exclusions,
  alongside captured counts. Snapshot metadata and captured contents are disclosures.
  Revisions can be selected and compared to their predecessor using metadata only.
- Prepare this machine explains install/authentication, profile pull, encrypted
  snapshot transfer/S3 fetch, local preview, conflict choices, apply and verification.
  The browser cannot establish snapshot availability or destination runtime health.
- Machine onboarding presents Install, Authenticate and Scan one step at a time,
  and separates connecting to this site from hosting another site.
  It shows server version separately from local CLI capability checks. The default
  release channel follows a recognised server build; the user can choose another.
  A generated connection command contains no token; tokens use hidden CLI input.

## Current setup capabilities

Local setup commands discover six agent families, summarise supported settings/MCP,
report software evidence, declare/export selection policy, preview and write encrypted
snapshots, list/inspect snapshots, plan/apply/verify supported restores, journal and
roll back completed applies, resume eligible interrupted applies, and report health
and support coverage. Optional S3 delivery, fetch, listing, destination tests and
protected pruning/deletion are supported.

Snapshots can include supported global/project instructions, whole bounded skill
folders with sanitised provenance and lock-file creation/merge, optional portable
literal credentials, and consistent copies of supported Codex memory databases.
Named local profiles support revisions, exclusions, machine/project scopes,
rename/remove, server push/pull and interactive create/restore wizards. Web setup
profiles can be renamed and edited through new exclusion revisions.

This remains **partial file-level recovery**, not complete working-setup recovery.
Other memory adapters, conversation history, hooks, caches, device-bound credentials,
agent dependency installation, verified cross-OS recovery and runtime health remain
unsupported or unverified. Do not reopen settled product choices without evidence.

## Safety and conflict handling

- Restore verification uses format-specific limits: configurations 1 MiB, whole
  skill folders 8 MiB, supported memory databases 64 MiB. Capture/restore agree.
- An action is marked as written before verification, so a verification or journal
  persistence failure includes the current action in rollback. Write errors propagate.
- Codex memory restore/verification uses the destination's effective CODEX_HOME.
  SQLite sidecars are rechecked immediately before writes. Destination agents must
  be stopped; sidecar absence does not prove an application is inactive.
- Memory verification continues after a blocked database. Each skill produces one
  final result; an unsafe file cannot also make the skill count as verified.
- The restore wizard prints mapped paths and warnings, asks per-destination conflict
  choices and confirms the final plan. Incomplete file verification returns an error, including the standalone verify command; JSON reports are still emitted.
- Server profile writes require an identical history or an exact append, with an
  atomic payload comparison protecting concurrent writers. Stale/divergent pushes
  return 409. Web edits carry the viewed base revision; stale edits are rejected.
- Local pulls preserve divergent/newer local histories. Preserve a local copy with
  `reet setup profile rename NAME NAME-local`, then pull the server copy and review.
  Local saves/pulls/rename/remove share an exclusive `profiles/.write.lock`.
  Remove a stale lock only after confirming its owning process has stopped.
- A failed profile deletion can be retried without reloading the application.

## Installation and release changes

The shell installer verifies checksums, runs the staged candidate's version before
replacement, preserves a regular existing binary as `reet.previous`, and reports
PATH shadowing. It refuses an existing symlink rather than silently replacing it.
Existing data and managed deployment files remain separate.

Dev publication keeps legacy rolling names for existing clients, and also uploads
commit-addressed archives `reet_dev_SHA_OS_ARCH.tar.gz` with `checksums-SHA.txt`.
Those assets are checked on a rerun and never overwritten. Only after they are
available does the publisher update `dev-build.txt`. New shell installers and Go
upgrades resolve that pointer once and download the corresponding immutable pair.
Only a missing (404) pointer falls back to legacy releases; other errors fail closed.
No assets, tags, deployments or production hosts are published/upgraded by local tests.

## Validation and resuming work

Run the repository checks after changes:

```sh
cd web && npm run check && npm run build && cd ..
go vet ./...
go test -race ./...
cd mcp && npm test
```

Regression coverage includes large restores and rollback after verification failure,
CODEX_HOME and late SQLite sidecars, complete/non-contradictory verification,
stale/divergent server pushes, guarded local pulls, scope-aware UI coverage and
revision comparison, admin versus shared-list relationships, staged installer
version failures, previous-binary preservation and immutable dev downloads.

Browser checks should cover light/dark themes, desktop/phone widths, keyboard menus,
revision selection/comparison, exclusions, failed deletion/retry, list preparation,
and both onboarding purposes, using synthetic accounts/data and temporary homes.
Physical macOS service-manager and cross-OS restore verification remain separate gates.

Keep the existing stage-per-PR workflow and required CI checks. Do not claim a release,
merge, working recovery or remote upgrade without checking the actual resulting state.

### Validation recorded for this branch

- Go vet and the full Go race suite passed. Affected-package race checks passed
  again after final verification-exit and routing changes. Wizard tests cover
  cancel after review, replace with successful verification, and skip with an
  explicit incomplete-verification error.
- Web checks passed: eight inventory/comparison tests and four rebalance tests,
  zero Svelte errors or warnings, and a production build.
- All 16 MCP integration tests passed.
- Local Chromium checks passed with synthetic profiles/lists: scope-aware counts,
  revision selection/comparison, exclusion revision saving, failed deletion/retry,
  list rows/cards, preselected list preparation, connection versus hosting,
  light/dark phone widths, Escape menu dismissal and no runtime errors.
- A separate account-mode browser run created a synthetic owner and machine token,
  checked that the token stayed out of commands/localStorage, exercised progressive
  connection steps and hosting, and checked viewer control visibility with a
  mocked viewer status. Backend role isolation remains covered by the API/MCP tests.
- Installer/publication tests use local archive and GitHub fixtures; no live release
  was published and no native service or Compose deployment was changed.
