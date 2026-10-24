# Reet central-control implementation handoff

## Read this first — 24 October 2026

This is the current implementation baseline **and** the continuation plan. Current
capabilities below are implemented; unchecked S/A/W/L tasks are future work. Start
with **S1**, not the whole roadmap. Historical delivery evidence remains in
[handoff-history.md](handoff-history.md); old stage notes are not current feature
limitations. The previous handoff's "no controller-authorized terminal" statement
and old improvements-branch pointer are superseded by this file.

Read alongside:

- [Control-centre behaviour and boundaries](docs/control-centre.md).
- [Controller sync](docs/controller-sync.md) and
  [verification evidence](docs/controller-sync-verification.md).
- [Portable agent setup architecture](docs/agent-setup-plan.md), especially format,
  ownership, credential, rollback and partial-recovery requirements.
- [Remote machine architecture](remote-machine-implementation-plan.md), with
  [stage 7](docs/remote-machine-stage7.md), [stage 8](docs/remote-machine-stage8.md),
  [stage 9](docs/remote-machine-stage9.md), and
  [controller terminal commands](docs/terminal-controller-commands.md).
- CONTRIBUTING.md and [voice/copy](docs/voice-and-copy.md).

### Baseline and publication state

- Local implementation baseline inspected: `183beb7569921ac3c687ec52171bebdf3a5d9461`
  on `feat/fleet-control-centre`, based on main `72244c1` (PR #134).
- [PR #135](https://github.com/eyupio/reet.sh/pull/135), the fleet-first redesign,
  was confirmed **merged** through GitHub on 24 October 2026; returned merge commit
  `ed08496e3ef5097604fdaf019e7b08ff16d40f6c`. This local checkout was still on the
  feature commit when this handoff was written. Refresh remote state next session.
- PR #134 adds controller-side `reet terminal start|stop|restart|status`; these
  manage browser-terminal availability, not AI application processes.
- No production deployment, release artifact, real machine upgrade or live fleet
  content sync was verified in this conversation. A merge is not deployment.
- This handoff-only update has not itself been published by writing this file.
  Preserve it if changing branches. Do not append implementation to a merged PR.

## Implemented baseline: preserve these behaviours

### Product organisation and fleet controls

Signed-in home is **Control** at `/` and `/control`. Main destinations are
**Machines** (`/machines`), **Skills** (`/skills`), **Sync** (`/sync`) and
**Terminals** (`/terminals`, subject to account/write access). Lists, portable
setups, community and hosting are under More. `/profiles`, `/setups`,
`/account/machines`, `/account/sync` and legacy `/machines?view=compare|terminal`
links remain supported. Preserve aliases when extending routing.

- Control shows reported inventory, unique reported skills, saved sync scopes,
  recent jobs awaiting approval, fleet actions and machine/terminal links.
- Sync takes skills from the library/list, published hosted skills, or a reported
  machine scope. Choose one machine or All machines, then individual supported
  agent scopes. All machines currently means the reported scopes selected at
  that moment; it includes offline inventory, not automatic future enrollment.
- Saved desired state is separate from preview, exact plan approval and apply.
  Scan all queues available enrolled machines. Sync all previews available saved
  target sets; it does not replace desired state with the latest library.
- Distinct scopes may queue previews on one machine. Duplicate active previews
  for a target are blocked; active scans/applies block new previews; execution
  remains serial per machine. Existing plan hashes/revisions gate approval/apply.
- Paused/offline/pending/stale/incomplete destinations are skipped with reasons.
  Batch save/queue operations preserve earlier successes and report later failures.
  A lost response is ambiguous; refresh before retrying, never blindly replay.
- Source loading failure cannot become an empty desired set. Expected revisions
  are captured for the reviewed save. Scan freshness/completeness and source
  conflicts still gate sync. Unknown local-only sources cannot be installed by
  inventing an upstream source.
- Incoming skill adoption currently adds references/metadata to reet. **Live sync
  compares skill identity/source and membership, not file contents or versions.**
  Editing `SKILL.md` on one machine does not yet create a reviewed library bundle
  revision or propagate those changed bytes to other machines.
- Library stages, notes, lists and removal are distinct from machine installation.
  Removing a library entry leaves installed files and saved desired sets alone.
  Users maintain their authored bundles in Hosting; importing must never publish
  private machine content as a side effect.

### Remote terminals and machine health

The repository contains a real controller-authorized terminal implementation:
private connector transport, durable host session registry, broker/tickets/leases,
browser WebSocket attachment, xterm UI, bounded history, health and gated rollout.
Treat stage documents as historical increments, not proof that later code is absent.

- Account authorization, server terminal enablement and host-local opt-in are
  independent. Browser attachment uses account sessions, not leaked enrollment
  capabilities. Keep enrollment/owner/role checks and request limits intact.
- `reet terminal start|stop|restart|status` manages controller availability.
  `reet agent ...` manages the Reet connector and host services. Neither is a
  Claude/Codex/OpenCode package installation or AI conversation lifecycle API.
- Shells persist in independent tmux sessions across browser disconnects and
  connector restarts. Stopping browser availability is not ending host shells.
- Terminal creation has durable intent/request IDs and ambiguous-retry recovery;
  disconnects must not create duplicate shells or replay disconnected keystrokes.
- Current UI supports creating/selecting/renaming/ending shells, reconnect/search,
  machine health/history and local readiness/setup guidance. A running shell is
  not semantic evidence that an AI agent is working/waiting/finished.
- Current terminals start under the configured host user/home. Managed project
  launch, registered AI-agent launch, worktrees, code diffs, structured tool
  approvals and conversation resume/fork remain roadmap items.
- Terminal output stays in bounded host/tmux buffers/history; do not introduce
  central transcript recording as an unannounced consequence of activity tracking.

### Portable setup and supporting infrastructure

Local setup commands discover six agent families, summarise supported settings/MCP,
inspect bounded software evidence, declare/export selection policy, preview and
write encrypted snapshots, list/inspect snapshots, plan/apply/verify supported
restores, journal/rollback, resume eligible interrupted applies and report coverage.
Optional S3 delivery, fetch, listing, tests and protected pruning/deletion exist.

Snapshots can include supported global/project instructions, whole bounded skill
folders with sanitised provenance and lock-file creation/merge, optional selected
portable credentials and consistent copies of supported Codex memory databases.
Named setup profiles support revisions, exclusions, machine/project scopes,
rename/remove, server push/pull and interactive create/restore workflows. Web
profiles support exclusion revisions, coverage and metadata revision comparison.

This is **partial file-level recovery**, not complete working-agent recovery.
Broader memory/history/hooks/caches, agent dependency installation, verified
cross-OS recovery and provider runtime health remain unsupported or unverified.
Do not replace old capability disclosures with blanket "restore everything" claims.

Reet also already has accounts/roles, lists/sharing/teams, notifications/email,
MCP access to reet, sealed backups, authored/public skills and self-install/upgrade.
Extend these foundations instead of rebuilding capabilities that already exist.

## Code map: read before proposing new types or endpoints

| Area | Existing entry points |
| --- | --- |
| App/routes and Control | `web/src/App.svelte`, `web/src/lib/ControlCenter.svelte` |
| Machines/onboarding | `Machines.svelte`, `MachineFleet.svelte`, `MachineSetup.svelte`, `machineSetup.ts`, `machines.ts` under `web/src/lib/` |
| Sync/fleet UI | `EnvironmentSync.svelte`, `SyncControl.svelte`, `FleetActions.svelte`, `fleet.ts`, `api.ts` under `web/src/lib/` |
| Live sync domain/execution | `internal/scan/sync.go`, `agent.go`, `restore.go`; adjacent tests |
| Desired sets/jobs/receipts | `internal/store/store_sync.go`, `store_sync_control.go`; `internal/api/sync.go`, `sync_control.go` |
| Store/migrations and route registration | `internal/store/store.go`, `internal/api/api.go`; inspect feature-local initialisation too |
| Hosted bundle validation/editor | `internal/store/store_hosted.go`, `internal/api/hosted.go`, `web/src/lib/Hosting.svelte` |
| Snapshot whole-skill collection/verification | `internal/setup/skills.go`, `skills_verify.go`, `snapshot.go`, `restore_apply.go`, `rollback.go` |
| Agent adapters/discovery/evidence | `internal/setup/adapters.go`, `discovery.go`, `installation.go`, `health.go`, config/policy files and tests |
| Setup profiles | `internal/setup/profile.go`, `internal/profilehistory`, `internal/store/store_setup_profiles.go`, `internal/api/setup_profiles.go` |
| Host shell registry and services | `internal/terminalhost/manager.go`, `manager_tmux.go`, `registry.go`, `service.go`, `readiness.go` |
| Typed private transport and broker | `internal/terminalproto`, `internal/terminalagent`, `internal/terminalbroker`, `internal/privateconn` |
| Terminal API and records | `internal/api/terminal.go`, `internal/store/store_terminal.go` |
| Terminal UI and connection state | `web/src/lib/terminal/TerminalWorkspace.svelte`, `TerminalPane.svelte`, `api.ts`, `connection.ts` |
| Controller enablement | `cmd/reet/terminal.go`, `internal/terminalcontrol`, `docs/terminal-controller-commands.md` |
| Machine health | `internal/agenthealth`, `internal/store/store_remote_health.go`, `internal/api/health.go` |
| Notifications | `internal/notify`, `internal/store/store_notify.go`, `internal/api/notifications.go`, `web/src/lib/Notifications.svelte` |
| Existing regression fixtures | `internal/store/store_sync_control_test.go`, `web/src/lib/fleet.test.mjs`, `web/tests/control.e2e.mjs`, `web/tests/terminal.test.mjs`, `web/tests/terminal.e2e.mjs`, `internal/api/terminal_integration_test.go` |

Names such as `Skill`, `SyncSkill`, `SyncTarget`, `SyncPlan`, `SetupSkill`,
`SnapshotSkill`, `HostedSkill`, `TerminalSession` and `SyncAgent` already exist.
Read their actual contracts before adding a similarly named concept. Especially,
`SyncPlan.Hash()` hashes the planned actions; it is not an installed-content hash.

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


## Accepted product direction and priority

The user wants reet.sh to be the central control unit for multiple machines:
maintain skills on reet, connect machines, import local skills, distribute to one,
selected, or all machines, review changes in both directions, and use installed
AI agents through the web terminal. Agent software install/update/remove and
cross-machine configuration management are future requirements. The user accepted
the central-control roadmap on 24 October 2026 and requested this detailed handoff.
This records the accepted direction; this turn authorises the handoff update,
not implementation of every milestone or production changes. Future sessions
should implement the task requested within this direction, and verify its gate.

Priorities, in order:

1. **S — Complete the skill foundation:** versioned bundles, content-level drift,
   reviewed local edits, safe one/many-machine transfer and verification.
2. **A — Make remote work convenient:** registered agent capabilities, project
   selection, explicit launch, semantic activity and an attention queue.
3. **W — Make parallel work reviewable:** managed worktrees, files/diffs and
   structured agent interaction where supported.
4. **L — Fleet lifecycle:** observed agent versions, reviewed install/update/remove,
   desired MCP configuration and optional isolated agent sessions.

Conversation resume/fork, granular machine/project permissions, read-only terminal
observation and OIDC login are valuable later extensions. A full terminal TUI,
themes, sound effects and a broad plugin marketplace are lower priority than the
four outcomes above. Keep the warm Reet branding, Yorkshire rose and familiar
navigation; read [voice and copy](docs/voice-and-copy.md) for user-facing changes.

The older setup plan records an earlier delivery order (backup/restore, profiles,
maintenance). Preserve its data-safety and recovery requirements. This roadmap is
the newer ordering for the central-control product; do not attempt complete setup
recovery before the next bounded skill-content task. Do not label partial file
recovery as a fully working recovered agent environment.

## Execution rules for the next session

1. Read this file, CONTRIBUTING.md, applicable AGENTS.md files, and the entry-point
   files for the single task selected below. Check the actual checkout; paths and
   branch state here are a dated baseline, not permission to reset local work.
2. Run `git status --short`, inspect the current branch/log and fetch the current
   base when network/authentication are available. Preserve existing edits.
   PR #135's branch is already merged; use a new branch from current main for the
   next implementation PR, carrying any authorised local edits safely.
3. Pick **one unchecked task**, starting with S1. Do not build a later UI button
   or remote mutation endpoint while its prerequisite is absent. Do not turn
   this whole roadmap into one PR.
4. Before editing, state the task ID, files affected, expected behaviour and checks.
   Resolve routine implementation choices from existing conventions. If a domain
   decision genuinely remains open, record the question and continue independent
   work; never invent an unsafe default just to make a control appear functional.
5. Keep domain code separate from HTTP handlers, SQLite records and Svelte views.
   Reuse existing parsing, bounds, ownership, locks and recovery code where its
   contract fits. Do not copy a whole subsystem or introduce a dependency casually.
6. Add meaningful regression tests for new runtime behaviour and failures. Use
   synthetic homes, databases and fixtures; do not enroll or alter real machines
   or invoke a real paid AI provider as a test side effect.
7. Show observed/unknown/stale/incomplete/unsupported states distinctly. A request
   accepted by the controller is not an observed successful change on the host.
8. Update this file with completed task IDs, commit/PR, checks, limitations and the
   exact next task. Keep the stage-per-PR workflow and required CI. Do not merge,
   publish a release or change production merely because local tests pass.

### Terms that must not be conflated

| Term | Meaning |
| --- | --- |
| Reet connector / `reet agent` | Background machine enrollment, reporting and command transport; not Claude Code or Codex |
| AI agent application | A CLI such as Claude Code, Codex or OpenCode |
| Terminal session | A persistent shell; `running` does not mean an AI agent is working |
| Agent run | Proposed record binding a terminal/runtime to a registered AI agent, machine and project |
| Skill scope | A particular agent/global/project destination; several scopes may share physical files |
| Library membership | Personal shelf/list organisation; not install ownership or deletion approval |
| Desired set | Saved installation intent for a target; not a verified filesystem observation |
| Bundle revision | Proposed immutable skill contents and provenance; not a shelf row timestamp |
| Setup profile | Existing revisioned snapshot/exclusion workflow; not automatically a live agent launch profile |
| MCP endpoint | Reet's own API for clients; distinct from configuring MCP servers used by other agents |

## S — Skill-content sync: small implementation tasks

**End-to-end acceptance story:** edit `SKILL.md` or a supporting script on machine A,
review its exact bundle diff in reet, create a private immutable revision, select
machine B and/or other targets, preview, approve, apply, and verify the expected
contents. A conflicting edit on B must stop that destination without undoing or
concealing success elsewhere. Nothing becomes publicly hosted merely by importing.

### S1 [ ] Read-only local bundle manifest — START HERE

Scope: a deterministic manifest and local comparison helper with fixture tests.
Do not add remote uploads, SQLite migrations, a web editor or filesystem writes.

Entry points: `internal/setup/skills.go`, `skills_verify.go`, `skills_test.go`,
`internal/scan/sync.go`, `sync_test.go`, `internal/store/store_hosted.go`.

Steps:

1. Read the existing snapshot collector and verifier, including how they handle
   modes, paths, symlinks, size limits and digests. Locate the live sync planner's
   identity/source comparison. Write a short contract comment explaining why a
   matching source is not evidence of matching bytes.
2. Introduce a small reusable domain helper/package, or extract a suitable pure
   part of the existing collector. Avoid making `internal/scan` depend on setup
   CLI/HTTP code. Keep existing snapshot formats and digest compatibility intact.
3. Define a schema-versioned manifest: relative file path, byte size, content
   digest, supported executable-bit semantics, total counts/bytes, manifest digest
   algorithm/version and explicit errors/omissions. Keep skill identity/source
   separate from contents. Do not include absolute host paths or credentials.
4. Specify canonical path ordering and serialization. Hash exact bytes; do not
   silently normalize line endings, rewrite YAML or treat executable scripts as
   equivalent after dropping their executable bit. Reuse/version the current
   digest scheme when suitable; do not reinterpret an old digest as a new format.
5. Bound traversal, individual reads, file counts and aggregate bytes; reject
   unsafe/ambiguous paths, symlink roots/entries and special files. Account for
   case collisions and changes while reading. An unreadable or omitted entry
   makes the bundle incomplete, never an empty/equal bundle. Preserve the existing
   scanner's documented shared-root behavior rather than globally banning all
   scanner symlinks as a shortcut.
6. Add a pure diff: added/removed/modified files; equal/changed/unknown bundle
   outcome. A missing prior observation is unknown, not an empty baseline.
7. Document capability limits. Snapshot skills currently allow 256 files and
   8 MiB per skill, with separate aggregate limits; hosted bundles have a smaller
   100-supporting-file/2 MiB contract. Re-read the constants before coding. Do not
   silently widen either format or truncate a bundle to fit a receiver.

Acceptance fixtures: byte-identical trees created in different orders; edit only
`SKILL.md`; edit only `scripts/run.sh`; add/remove/rename; empty file; executable-bit
change on supported platforms; same name/source with different bytes; unreadable,
missing and over-limit entries; symlink/path escape; case collision; a read-time
change. The test must fail if a same-source content edit is treated as equal.

Gate: targeted Go tests and race tests pass, snapshot compatibility is preserved,
and no test/command writes outside its temporary fixture. **S1 alone must not
change the public meaning of "in sync" or claim live content sync exists.**

### S2 [ ] Private immutable revision store and ownership

Depends on S1. One persistence/API-contract PR, without a remote apply UI.

1. Inspect `internal/store/store.go`, `store_hosted.go`, `store_sync.go`,
   `store_sync_control.go`, `store_setup_profiles.go`, plus their API handlers.
2. Define a private bundle identity/revision model with owner, canonical manifest,
   immutable content references, origin/provenance, creation time and schema version.
   Preserve aliases/references to upstream identity separately from owned copies.
3. Distinguish immutable revision identity from manifest digest. Design all content
   lookups and reuse around account ownership; knowing another user's digest must
   not grant content access or reveal that their bundle exists.
4. Add migrations using existing transaction/idempotency conventions. Old records
   remain readable and have content status unknown until actually observed.
5. Store incoming contents privately and separately from public publication. Reuse
   the hosted editor validation where appropriate, not the public hosting routes
   as a private transport. Editing somebody else's/public upstream skill should
   create a clearly owned revision/fork, never silently overwrite its publisher.
6. Add expected-base-revision checks for mutable pointers. Bound all requests and
   return a conflict for stale concurrent edits. Decide backup/export inclusion
   explicitly; do not claim personal shelf JSON contains bundle payloads.

Tests: owner/viewer/other-account isolation; guessed digest; immutable revisions;
stale base; interrupted transaction; idempotent retry; upgrading an old database;
private import never changes public visibility. Gate: storage and API tests pass,
with a documented compatibility path for old clients and restore/backups.

### S3 [ ] Machine content observations and explicit import

Depends on S1/S2. Split observation metadata and payload transfer into separate
PRs if either becomes large.

1. Extend negotiated connector capability/inventory metadata with manifest schema,
   observed digest, completeness and time. Old connectors remain inventory-only;
   show "content not reported" rather than treating an absent digest as equality.
2. Keep periodic scans metadata-only. Uploading skill bytes is an explicit,
   account-authorized request scoped to an enrolled machine and skill root.
3. Preview origin, files, sizes and completeness before import. Respect local
   content-selection/exclusion rules; credentials/configuration must not hitchhike
   into a skill upload. Any detected sensitive/unsupported content needs an
   explicit blocked/review state rather than an automatic public/private upload.
4. Recompute and verify received manifests/digests; do not trust an agent-supplied
   hash alone. Reject changed-during-transfer, partial, oversized or unsafe payloads.
5. Create a private immutable revision only after a complete verified transfer.
   Retrying the same request returns its result; a lost acknowledgment must not
   create multiple revisions or restart an unrelated host operation.
6. Retain the existing metadata-only "Add selected to reet.sh" behaviour until
   bundle import is explicitly offered. Label the two actions distinctly.

Tests: old connector, stale/incomplete scan, changed file between preview/upload,
wrong machine/account, lost response/retry, corrupt chunk/manifest, cancellation,
revoked enrollment and private imported contents. Gate: two synthetic machine
fixtures can import different revisions without changing the existing shelf API.

### S4 [ ] Content-aware preview and reviewed single-target apply

Depends on S2/S3. First implement the planner, then the apply/receipt path.

1. Extend desired target state with a pinned bundle revision. Preserve legacy
   reference-based targets; mark their content-verification limits explicitly.
2. Bind preview/approval to target identity, desired revision, expected installed
   manifest, observed baseline, selected paths/actions and adapter/schema version.
   A hash of install names alone is insufficient for a content-replacement plan.
3. Plan create/update/remove with path-level differences and ownership. Never take
   over unrelated local files, local-only skills or a shared physical directory
   just because a display name matches. No broad directory deletion shortcut.
4. Stage and validate the full payload before writing. Acquire the correct local
   scope lock, re-read the preconditions immediately before mutation, and preserve
   recoverable backups/journal entries before replacement. Reuse existing restore
   lessons: include a partially written/current action in recovery accounting.
5. Verify the actual installed contents afterwards. Persist a receipt before
   acknowledging; include verified digest and per-action results. Incomplete
   verification is failure/incomplete, not applied or "in sync".
6. Reconcile interrupted/ambiguous execution from journal/receipt and local files.
   Never blindly replay an approved write because the controller missed a reply.

Tests: same source/different content; stale approved plan; local edit after preview;
changed desired revision; shared root ownership; disk/write/verification failures;
crash before/after write and receipt; rollback failure reported; retry without
reapplying; old connector refusal. Gate: real temporary filesystem fixtures prove
preservation of unrelated files and recovery, not just mocked API success.

### S5 [ ] Three-way content review and fleet rollout UI

Depends on S4. Deliver single-skill review first, then the fleet composition.

1. Compare last verified common baseline, current library revision and current
   machine manifest. With no baseline, show an explicit import/adopt decision.
2. Classify unchanged, library-only change, machine-only change, same change on
   both sides, conflicting edits and deletions. Do not auto-merge instruction text
   initially. Present library/local/keep-both choices; conflicting binary files
   require an explicit whole-file choice. Deletion/edit is a conflict.
3. Show file path and text diff where safe; escape content, cap previews, label
   binary/large files and never render skill HTML/scripts in the app origin.
4. Let a user save a private library revision independently from deploying it.
   Library removal, public unpublication and machine removal stay separate actions.
5. Review selected destinations and pinned revision per scope before queueing.
   Show queued/running/needs-review/succeeded/failed/skipped/unknown results per
   destination and aggregate them without claiming cross-machine atomicity.
6. Preserve current queue invariants: distinct scope previews may queue, execution
   is serial per machine, duplicate scope previews are rejected, and each exact
   mutation plan needs approval. Deduplicate shared physical writes and expose
   conflicting intents rather than guessing their ownership.
7. Explicitly distinguish one-time "all currently selected machines" from a future
   policy for new machines. Automatic inheritance is a separate reviewed feature.

Tests: conflict matrix including deletes; stale review; cancellation writes
nothing; mixed old/new connectors; one destination failing while another succeeds;
offline/paused targets; content source unavailable cannot imply an empty desired
set; keyboard/mobile review. Gate: the S acceptance story passes against real
local controller + two synthetic enrolled machine roots. Mocked browser fixtures
alone do not establish fleet transfer or filesystem correctness.

## A — Agent sessions and attention

Start after the content foundation; work on independent read-only discovery is
possible earlier, but it must not expand the next S1 PR.

### A1 [ ] Registered AI agent capabilities and project selection

Entry points: `internal/setup/adapters.go`, `discovery.go`, `installation.go`,
`internal/agenthealth`, `internal/terminalproto`, `internal/store/store_terminal.go`,
`web/src/lib/terminal/TerminalWorkspace.svelte` and `MachineFleet.svelte`.

1. Reuse the agent registry and bounded executable/package evidence. Separate
   installed executable, observed version, configured provider/auth readiness and
   actual runtime health. Never run a discovered binary just to enumerate it.
2. Add typed, bounded, time-limited probes only for supported adapters. Report the
   exact evidence and unknown states; do not upload tokens or local configuration.
3. Add an account/machine-scoped project registry: stable ID, displayed name,
   validated host-side root, repository metadata and capability observations.
   A browser-provided path is not automatically an authorised host directory.
4. Handle moved/missing projects, machine aliases and reconnects. Global and
   project skill scopes must resolve to their effective physical destinations.

Tests: absent executable, wrapper/unrecognised version, malformed probe, timeout,
read permission failure, another account's project, path/symlink escape and stale
inventory. Gate: UI displays launch readiness with reasons, without launching or
installing anything.

### A2 [ ] Explicit typed agent launch on a chosen machine

1. Define a distinct AgentRun record referring to connector, terminal/runtime,
   registered AI agent, project and selected/pinned skills. Do not reinterpret the
   existing terminal `agentId` (connector identity) as an AI application ID.
2. Extend the existing create path using a negotiated typed capability. Keep plain
   terminal creation backward compatible. Prefer supported executable + argv + cwd
   over a controller-supplied shell command string. Custom launch commands are a
   separately reviewed future capability.
3. Preview machine, OS user, project, agent, skill revisions and permission mode.
   Launch only on explicit action after checking local consent and actual host
   capabilities. A permission/readiness failure must not trigger a reinstall.
4. Preserve current request IDs, durable host registry, lease/attachment and
   no-replay behaviour. Credentials remain machine-local; no central provider-key
   copying or prompt injection through environment/shell interpolation.
5. UI: New agent session wizard; retain New terminal. Show unsupported adapters
   and missing prerequisites plainly. Closing the browser leaves the host process
   alive; ending a run clearly describes what process will stop.

Tests: selected cwd/argv, unsupported/changed executable, duplicate creation/lost
response, local opt-in disabled, revoked account/enrollment, disconnect/reconnect,
agent exit and connector restart. Gate: use a harmless fixture executable to
prove exact launch and persistent reattachment; no live paid-model call required.

### A3 [ ] Semantic activity, attention queue and useful notifications

1. Add versioned per-adapter events for starting/working/waiting-for-input/
   approval-needed/idle/completed/error/unknown. Do not infer "working" from CPU
   usage or a live shell. Start with one supported adapter and a fake event fixture.
2. Prefer documented hooks or structured events. Bound and redact event payloads;
   authenticate the local run/event source. Unsupported adapters remain unknown.
3. Keep process state, connector availability and semantic activity separate.
   Deduplicate event IDs, reject stale/out-of-order events and preserve observation
   times. A disconnected machine is not a completed run.
4. Add a Control attention list deep-linking to machine/run. Extend
   `internal/notify`, `store_notify.go`, notification preferences and UI with
   per-user events. Avoid repeated alerts for the same waiting state.
5. Add Web Push as a separate optional PR after durable attention events work.
   The existing web manifest alone does not provide background push. Cover HTTPS,
   permission denial, unsubscribed/stale devices and account revocation.

Gate: fixture run transitions generate one correct attention event/notification;
another user's run cannot be read or acted on. Raw terminal text is not centrally
recorded merely to derive status.

## W — Projects, parallel work and structured review

### W1 [ ] Managed worktrees and project-bound parallel runs

Depends on A1/A2. Store each run's repository, base, branch and worktree identity.
Create worktrees through bounded host operations with validated argv, ownership and
request IDs. Preserve existing/dirty worktrees; do not move a running checkout or
remove a worktree belonging to another run. Preview cleanup, detect uncommitted
work and record partial create/delete failures. Start with one repo; multi-repo
workspaces are a later small PR. Tests must prove two runs can edit independently
and cancelled/failed cleanup preserves user work.

### W2 [ ] Files, code diffs and execution results beside a run

Depends on A1/A2; use W1 metadata when present. Add authenticated, bounded read-only
file/diff APIs confined to authorised run/project roots. Resolve base branches
explicitly; distinguish tracked/untracked/deleted/binary/oversized files. Do not
expose arbitrary host paths, execute Git hooks or render active HTML/SVG/scripts
under the application origin. Add changed-files and diff panels with mobile and
keyboard support. Keep code diffs separate from skill deployment diffs. Start
read-only; commit/push/PR mutations are separately scoped work, never implied by
viewing a diff. Test traversal/symlinks, other-user access, huge/binary files and a
real temporary repository with known changes.

### W3 [ ] Structured agent interface and tool approvals

Depends on A2/A3. Start with one negotiated ACP/agent adapter and keep the terminal
fallback. Persist typed run/turn/tool/question/approval IDs; do not screen-scrape
terminal text into authority to approve a tool. Approval must target an exact live
request, with expiry and expected state; stale/duplicate decisions and reconnects
must not execute twice. Separate tool execution permission from skill sync and
software installation approval. Advertise model/reasoning/mode selectors only
when the adapter supports them. Default to existing/local permission policy;
never enable bypass/auto-approve silently. Test competing devices, timeout,
malformed events and reconnect during a pending question using a fake adapter.

## L — Fleet software and environment management

### L1 [ ] Agent software inventory and plan-only maintenance

Depends on A1; extends the setup plan, not `reet install` or connector service
controls. Pick one supported AI CLI, one OS and one install method first. Define
capability evidence for exact installed version, manager, executable path,
platform/architecture, source and supported operations. Unrecognised wrappers or
installations stay inspect-only. Resolve available versions from approved sources
with bounded network calls, never guess that "latest" is compatible. Create
reviewable install/update/remove plans without executing them; record effects on
running sessions and configuration separately. Test unsupported methods, missing
versions, multiple executables/PATH shadowing and unavailable package sources.

### L2 [ ] Verified single-machine maintenance, then fleet composition

Depends on L1. Stage verified artifacts or use a specifically supported package
manager adapter; no generic curl-to-shell installer or arbitrary controller shell
command. Bind approval to exact package/version/source/platform and observed
installation. Require explicit handling of active runs; never stop them silently.
Keep configuration, skills, histories and credentials separate from binary removal.
Use journals/receipts, preserve a rollback route where supported, verify the
resulting executable/version and report unsupported rollback honestly. First
prove install/update/remove and interruption recovery locally; only then reuse
per-machine fleet reporting for bulk operations. Default to a reviewed pilot
machine before wider rollout. Test failed download/install/verification/rollback,
changed install after approval, reconnect and dependency ownership.

### L3 [ ] Desired MCP configuration and health across machines

Entry points: setup config/MCP parsers, policies and restore code; existing
`internal/api/mcp*` concerns clients accessing reet and is not the new MCP control
plane. Model server definitions, source/precedence, per-agent/project assignment,
secret references and compatibility. Begin with read-only effective configuration
and drift. Then add reviewed writes through format-specific adapters, preserving
unknown fields and native unrelated entries. Separate "configuration saved" from
"server reachable/authenticated/tools usable". Runtime/authentication tests are
explicit bounded actions; running a repository-defined stdio server needs trust
review. OAuth grants and device-bound credentials require independent destination
authentication where not portable. Test conflicting layers, missing secrets,
malformed TOML/JSONC, stale writes and unavailable endpoints before fleet rollout.

### L4 [ ] Optional isolated agent execution

Depends on A2 and verified project/skill mappings. Begin with one detected container
runtime and one supported image/platform. Review image digest, workspace mounts,
credential references, network policy and CPU/memory limits. Give each session its
own writable history/configuration store; do not mount the whole host home or
Docker socket by default. Keep host and sandbox terminal identities explicit.
Persist container ownership and creation receipts, handle interrupted cleanup,
and preserve user data on failure. Reet's server Docker deployment is not evidence
of remote-session isolation. Test mount escapes, concurrent sessions, runtime loss,
resource exhaustion and cleanup of only owned artifacts.

## Later extensions and non-goals for the next implementation

- **Conversation resume/fork:** after A2/W3, use qualified native conversation IDs
  with agent/store/project/version compatibility; persistent tmux alone is not
  conversation recovery. Do not claim cross-machine migration by copying an ID.
- **Team controls:** explicit machine/project roles and separate read-only terminal
  observation before collaborative control. Do not relax today's viewer/owner
  checks to make a demo work. OIDC login is distinct from reet's MCP OAuth server.
- **Launch templates:** compose pinned agent version, project rules, skills and MCP
  references after those components work; retain per-machine exceptions. Do not
  silently reinterpret setup snapshot profiles as this new model.
- **New-machine policies/scheduling:** automatic desired-set inheritance, scheduled
  previews and approved rollout windows are separate opt-in capabilities. Today's
  All machines selection is a point-in-time set, not a durable membership policy.
- Preserve one-binary self-hosting and local setup backup/restore without a server.
  No cloud account, provider credential copying or central transcript recording
  is required merely to use skill management or existing remote terminals.

## Product acceptance and evidence boundaries

This roadmap follows the user's central-control requirements. Its value is in
controlled cross-machine skill contents, versions and agent execution. Keep these
outcomes measurable against reet's own behaviour and acceptance fixtures.

Software discovery does not establish install/update/remove capability. Shell
persistence does not establish conversation recovery. Configuration restoration
does not establish authenticated runtime health. Local checks do not establish a
production deployment or verified cross-platform support.

Reet already has accounts, teams, sharing, private connections, snapshots and
durable shells. Extend those foundations. Keep task scope, capability claims and
validation evidence specific to reet; describe user outcomes without competitor
names, links or comparison tables in repository documents and handoffs.

## Validation commands and interpretation

For documentation-only changes, check Markdown links, whitespace and consistency
with the inspected code; do not rerun the entire application suite just to validate
prose. For implementation, run targeted regression tests while developing, then
required repository checks before PR handoff. Use tools from the current environment;
never assume a previous session's binaries or caches are present.

Prerequisites: versions from `go.mod` and `web/package.json` (at this baseline Go
1.27.1 and Node 22), dependency installation when missing, tmux 3.3+ for terminal
service/integration tests, and a supported Chromium for browser tests. Read the
managed environment runtime instructions before network/container work. Never
print environment secrets, commit credentials or change global system directories
just to reconstruct this previous session's environment.

From the repository root, run sequentially:

```sh
(cd web && npm run check && npm run build)
go vet ./...
go test -race ./...
(cd mcp && npm test)
git diff --check
```

**Finish the web build before starting Go builds/tests.** Go embeds `web/dist`;
concurrent Vite cleaning/rebuilding can produce missing embedded asset failures
that are test-environment races, not application regressions. Keep inputs stable
while checking. The full API race package can take several minutes on cold caches;
do not remove timeouts, skip checks or assume a silent process is hung.

Examples of narrow checks (add the relevant new package/tests for the chosen task):

```sh
go test ./internal/setup ./internal/scan ./internal/store
go test -race ./internal/setup ./internal/scan ./internal/store
```

For the existing synthetic Control browser contract, start the dev server in a
separate terminal and keep it running:

```sh
cd web
npm run dev -- --host 127.0.0.1 --port 4173
```

Then, from the repository root:

```sh
node web/tests/control.e2e.mjs
```

`REET_UI_URL` can select another dev URL. `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH`
selects a browser (the current fixture otherwise uses `/usr/bin/chromium`).
`REET_UI_SCREENSHOTS` is an optional output directory for synthetic screenshots.
This fixture mocks APIs; pair it with real domain/API/host tests for new effects.

For real terminal browser regression, follow `.github/workflows/terminal-browser.yml`
and the integration fixture, after building web and making tmux/browser available:

```sh
REET_BROWSER_TESTS=1 go test -race -count=1 -timeout=3m ./internal/api -run '^TestTerminalPrivateLifecyclePreservesActualShell$' -v
```

This exercises a real local shell/broker/PTY with test transports. It does not
establish a production relay deployment, cross-OS recovery, or a live fleet rollout.
Physical macOS service-manager and supported cross-OS checks remain separate gates.

### Evidence recorded for the PR #135 implementation baseline

- Web unit checks and Svelte diagnostics passed (zero errors/warnings); production
  build passed. Main JS was approximately 500.4 kB minified / 148.4 kB gzip,
  slightly above Vite's 500 kB advisory threshold. Do not mask that warning by
  increasing its threshold; optimise only when a scoped performance task needs it.
- Go vet and the full Go race suite passed with workspace-local tmux 3.5a available.
  The initial environment lacked tmux; several service tests failed until that
  prerequisite was supplied. Do not weaken those tests to make a bare image pass.
- All 16 MCP tests passed. A cold fixture build initially timed out while other
  Go work compiled; the complete suite was rerun successfully with warm caches.
- Control browser checks passed with synthetic inventories for all-machine saves,
  incoming skills from different scopes, machine-source distribution, cancellation,
  partial save/queue failure, unavailable sources/inventory, viewer controls,
  legacy comparison routes and light/dark desktop/tablet/phone layouts.
- After rebasing the implementation onto main with PR #134, CLI/store race tests
  and whitespace checks passed. Vet and CLI/web compilation were checked again
  against the completed frontend bundle.
- No production host, release, agent installation or live fleet was changed.
  No roadmap S/A/W/L milestone was implemented by the handoff documentation update.

### Record after every implementation task

Replace the relevant unchecked task with its completed state only after recording:

```text
Task ID and scope:
Branch / commit / PR:
Behaviour added:
Compatibility / migration behaviour:
Checks actually run and their results:
Platform or live-environment checks not performed:
Known limitations or unresolved decisions:
Exact next unchecked task and first files to read:
Publication / deployment state (verified, not inferred):
```

If a task cannot meet its acceptance gate, leave it incomplete and describe the
failing case and recoverable state. "Code compiles", "API returned 200", a mocked
success response or a green screenshot does not establish successful remote writes,
working agent authentication or complete environment recovery.

## Ready-to-use continuation brief

> Continue reet.sh from handoff.md. First inspect the actual branch, worktree and
> current main without resetting any edits. Implement only S1: a deterministic,
> bounded, read-only local skill-bundle manifest and pure file-diff helper, reusing
> appropriate snapshot code and preserving old formats. Read the S1 files and
> existing tests before choosing the package/API. Test same-source/different-byte
> skills, supporting scripts, executable bits, incomplete reads, limits and path
> safety. Do not add remote uploads, database writes, content apply, agent launch,
> real-machine changes or production publication in this task. Run the relevant
> tests and repository gates, then update task status, evidence and exact next
> step in handoff.md. Keep any resulting PR limited to S1.
