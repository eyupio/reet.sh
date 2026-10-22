# Portable AI agent setups

Status: agreed product direction; implementation is incremental. This document
describes planned capabilities, not a claim that setup restoration already works.
See [handoff](../handoff.md) for current progress and the next engineering task.

## Outcome

reet becomes the home for a user's AI agent setup: discover, install, configure,
back up, restore, sync and maintain agents and their supporting tools. The first
release milestone is restoring a supported working setup onto a new machine.
Users must be able to see what was recovered, what was verified and what still
needs their attention. An unsupported item must never silently disappear.

Keep the existing one-binary, self-hosted model. Local backup and restore must
work without an account or running reet server. Network access may still be
necessary to download dependencies and authenticate providers.

## Decisions agreed with the product owner

| Decision | Requirement |
| --- | --- |
| Delivery order | Complete setup backup/restore, then profile sync, then maintenance |
| Agent breadth | Discover popular agents broadly, including Claude Code, Codex, OpenCode and Pi; extend through adapters |
| Capability claims | Publish verified support per agent, component, OS and version; clearly flag gaps |
| Execution | Local CLI previews changes and applies only after confirmation |
| Credentials | Optional encrypted backup of portable credentials; guide fresh authentication for non-portable credentials |
| Recovery | User-held recovery key or passphrase, separate from reet login |
| Platforms | Cross-OS restore where verified, with path adaptation and incompatibility reports |
| Memory/history | Include supported reusable memory; conversations and sessions are opt-in |
| Conflicts | Preview each conflict, let users choose, preserve rollback copies |
| Storage | Encrypted local files and optional S3-compatible storage |
| Versions | Recover recorded versions; ask before substituting unavailable versions |
| Sync | Named desired profiles with machine and project overrides |
| Maintenance | Reviewed updates, compatibility checks, health diagnostics and rollback |

## Existing foundations and boundaries

- `internal/scan` discovers skills, reports inventory and applies skill sync.
  Preserve its freshness, completeness, provenance and shared-folder semantics.
- Current controller enrollment and skill sync remain compatible. New setup
  inventory must not change the meaning of existing skill inventory endpoints.
- `internal/backup` provides sealed archives, local retention and S3 delivery.
  Existing instance and personal shelf backups are separate backup kinds.
- `internal/lifecycle` installs and upgrades reet itself. Installing agent
  applications is a new capability, not a reinterpretation of `reet install`.
- `internal/store` and `internal/api` provide accounts, machines and references.
  `web/` provides the library, machine views and account backup UI.
- The hosted `/mcp` endpoint lets agents use reet. Managing a user's MCP servers
  is a separate capability with separate inventory and permissions.

## Components in a setup

1. Agent applications: identity, observed version, installation method, source,
   OS/architecture and supported installation/update actions.
2. MCP: local packages and runtimes, remote endpoints, client assignments,
   launch parameters, configuration, secret references and health checks.
3. Skills/extensions: sources, revisions, checksums, local modifications,
   custom commands, plugins and dependency relationships.
4. Behaviour: rules, instructions, hooks, agent definitions and workflows.
5. Configuration: models/providers, permissions, global and project settings.
6. State: reusable memory; optional supported conversation/session history.
7. Credentials: references by default, explicitly selected portable values in
   encrypted payloads only. OS keychain and device-bound sessions may require
   fresh authentication; copying their backing files is not portable recovery.

An agent application and a custom agent definition have distinct component kinds.
Sharing a directory does not establish shared ownership. Track logical ownership
and deduplicate physical content without removing another agent's dependencies.

## Architecture

### Local core and adapters

Introduce `internal/setup` as the local domain, independent of HTTP and SQLite.
Begin with a registry of discovery adapters and explicit capabilities. Expand
each adapter with version-specific parsing, export, plan, apply and verify
operations as those operations gain fixtures and platform validation.

Adapter metadata includes stable ID, display name, recognised configuration
locations, environment overrides, executable names, component types, tested
versions/platforms and support limitations. Discovery must distinguish executable
presence, configuration presence and actual runtime health. A leftover directory
is not proof that an application is installed. Missing, unreadable and unexamined
locations are different states.

Never execute discovered binaries just to enumerate files. Later version/health
probes need explicit bounded commands, timeouts and documented side effects.
Discovery must not upload configuration contents or credential values.

### Versioned records

Use separate versioned models for:

- Observed inventory: OS/architecture, capture time, adapter versions, sources,
  component observations, scope, completeness and diagnostic codes.
- Snapshot manifest: immutable snapshot ID, source platform, selected components,
  content digests, dependency graph, recorded versions, inclusion choices and
  omissions. Store sensitive metadata inside the encrypted payload.
- Desired profile: named revision with components plus machine/project overrides.
  Creating a new backup must not silently change a profile.
- Restore plan: source snapshot digest, destination facts, ordered actions,
  conflicts, choices, prerequisites, unsupported items and expected validation.
- Operation journal: completed/failed actions, rollback locations, remaining
  work and final verification results. Never log credential values.

Keep original bytes for recovery alongside structured fields for portable
translation. Do not rewrite unknown settings or comments when a format adapter
cannot preserve them. Unknown future schema versions fail before mutation.

### Archive and key handling

Reuse established archive primitives where appropriate, with an explicit setup
kind and schema. Audit header exposure, size bounds and memory use before reuse:
existing in-memory sealing must not imply unbounded session-history support.
Do not place secrets, endpoint tokens or private file paths in clear headers.

Provide key/passphrase input without command-line secret values, a documented
recovery-key export, wrong-key/tamper handling, integrity verification and bounded
decompression. Encrypt before local persistence or S3 transfer. Credentials are
an explicit inclusion choice, not a side effect of selecting a configuration
folder; config adapters must separate inline secrets from ordinary settings.

Content collection uses allowlisted components, size/count limits, consistent
reads, and explicit exclusions for caches, sockets and unrelated files. Reject
traversal and escaping archive paths, protect destination symlinks, and preserve
only supported permissions. Never follow arbitrary symlinks into secrets or
capture a whole home directory because an override points there.

### Restore execution

1. Open and authenticate the archive locally; validate schema and content hashes.
2. Discover the destination, map project paths and resolve target OS differences.
3. Resolve dependencies and exact versions; identify unavailable packages before
   writes. Ask for any substitution instead of silently using latest.
4. Produce a plan showing installations, files, conflicts, credential handling,
   non-portable items and health checks. Allow skip/keep/replace choices per
   conflict; structured merge only where verified.
5. Confirm locally, recheck destination fingerprints and acquire an operation
   lock. Changed inputs invalidate the plan.
6. Preserve rollback copies and journal each action. Install in dependency order
   and restore files atomically where possible. Never interpolate archive strings
   into a shell command. Validate sources before running installers or hooks.
7. Verify resulting files and supported runtime health; report partial results.
   A successful file copy alone is not a successful working-setup restore.

Rollback is best-effort across external package managers and authentication.
State which actions are reversible; do not promise one filesystem transaction.
Interruption recovery must use the journal, and rerunning must be idempotent.
Historical backups containing enabled hooks require explicit execution review.

## Proposed CLI and UI

Reserve a separate `reet setup` command family. Initial implementation:

```sh
reet setup scan
reet setup scan --json
reet setup scan --project /path/to/project
```

Later command names below are proposals until each slice lands:

```text
reet setup backup       select components, seal and verify a snapshot
reet setup inspect      explain snapshot contents and restore support
reet setup restore      preview; --apply confirms and executes locally
reet setup verify       verify a restored setup without silently repairing it
reet setup rollback     preview and reverse supported journalled actions
```

Do not change existing `reet backup` restore behaviour or formats. Document the
difference between instance recovery, personal shelf recovery and setup recovery.

Evolve navigation towards Overview, Library, Machines, Profiles, Backups and
Updates as features land. Keep skills in Library. Machine detail should show
agents/components with observed state, report age and capability limitations.
Backup selection distinguishes configuration, reusable memory, optional history
and optional portable credentials. Restore UI prepares a plan/command; execution
and confirmation remain local. Do not expose credential values in web previews.

## Delivery plan and acceptance gates

### S0 — Discovery foundation (first implementation PR)

- Add a local adapter registry for Claude Code, Codex, OpenCode, Pi and additional
  popular tools where configuration locations are known.
- Add text/JSON `reet setup scan`, scoped to known global locations and an
  explicitly selected project. Honour supported path overrides.
- Report application executable discovery separately from configuration paths;
  surface inaccessible/unsupported locations and explicit restore limitations.
- Do not parse config contents, execute agents, contact services or write files.
- Tests cover absent/config-only/binary-only states, overrides, project scope,
  unusual file types, unreadable paths and output with no secret contents.
- Gate: deterministic fixture-backed local inventory and existing checks pass.

### S1 — Typed component inventory and export policy

Deliver S1 as separately tested slices: S1a adds opt-in bounded strict-JSON
settings/MCP summaries and credential hints; S1b adds TOML/JSONC format adapters;
S1c covers version/provenance capture and component export policies. S1a must not
claim that the complete S1 acceptance gate or any restore capability has landed.
The first S1c slice adds explicit `--probe-versions` command reports; verified
installation provenance and export policies remain separate work. Keep application
versions distinct from existing controller/scanner version fields.
The next S1c slice adds opt-in local Node package evidence by checking declared
executables against observed files. This verifies a local association only;
package-manager receipts, publisher authenticity and install resolution remain
unverified. Do not treat editable metadata as permission to reinstall a package.
An opt-in `--export-policy` now declares, per source, the settings fields, MCP
declarations and credential-bearing fields a future encrypted snapshot may select,
plus explicit unsupported classes for memory, history, instructions, behaviour,
hooks and caches. It reads no configuration values and implements no export.

- Parse agent-specific config formats and MCP declarations with bounded reads.
- Record versions, installation provenance, dependencies, global/project scope,
  custom files and local modifications. Add opt-in bounded version probes.
- Classify credentials, memory/history, hooks and unsupported fields; create
  source/destination fixture matrices. Keep discovery breadth wider than restore.
- Gate: inline secrets never enter ordinary inventory; partial scans never imply
  absence; each supported component has a precise export policy.

### S2 — Standalone encrypted setup snapshots

- Add immutable manifest/content storage, local key/passphrase management,
  selection previews, optional portable credentials and optional history.
  An opt-in preview now applies the S1 policies in memory and reports selected
  fields, omissions and per-component digests without writing any archive.
  Encrypted writing now supports explicit `--include-credentials` literal values;
  OS keychains/device-bound stores and optional history remain later work. A first
  writing slice seals that selection into a passphrase-encrypted local snapshot and
  verifies it before and after the atomic private write. A listing/inspection slice
  reads clear headers without a passphrase and opens the encrypted preview only on
  request. A restore-planning slice maps selected sources to destination locations,
  reports create/conflict/blocked actions and unsupported items, and fingerprints the
  plan without writing. Apply now writes supported new or replaced files, verifies
  their content, keeps rollback copies for replacements and rolls back a failed run.
  A verify command now compares selected fields and reports missing/changed/blocked
  components without repairing. A rollback command reverses a completed apply journal
  after checking it against the snapshot and current files; local journals can be listed
  and `--last` selects the newest completed one. Apply supports global and
  per-destination skip/replace choices. Snapshots can now record bounded application
  and package evidence and selected global/project instruction text; verify compares
  software evidence with opt-in probes. Runtime health checks and broader memory
  adapters remain later slices.
- Verify every completed archive, write privately and atomically, and produce an
  omissions report. Add local listing/inspection and optional S3 delivery.
- Keep local recovery usable after complete loss of the controller database.
- Gate: round-trip bytes/metadata; wrong-key/tamper/truncation/oversize tests;
  no plaintext secret remnants in archives, logs or temporary files.

### S3 — Restore planning

- Build dependency/version resolution, destination mapping, compatibility rules,
  conflict choices and stable plan fingerprints. Add unavailable-version flows.
- Reuse local manifests without requiring API calls to a reet instance.
- Gate: preview produces no writes; unsupported translations block or explicitly
  skip affected items; changed inputs invalidate old plans.

### S4 — Apply, verify and recover (first release milestone)

- Add local confirmation, locks, journalled actions, rollback copies and bounded
  installer invocations. Restore supported secrets without logging values.
- Add post-restore health checks and reauthentication guidance.
- Gate: fresh-machine recovery, conflict handling, interrupted restore, retry,
  rollback and selected cross-OS paths verified in disposable environments.
  Publish evidence-backed capability matrix and release limitations.

### S5 — Named profiles and cross-machine sync

- Add revisioned profiles, machine/project overrides, migration/API/UI work and
  selective plan application. Preserve existing skill-sync APIs and references.
- Record drift from observations; stale/incomplete inventory stays unknown.
- Gate: intentional machine differences survive sync; profile revisions are
  explicit; no implicit remote execution or credential sharing.

### S6 — Maintenance

- Add reviewed update plans, compatibility checks, diagnostic suggestions and
  rollback. Separate update intent from exact-version restoration.
- Gate: users can explain an update before applying it and recover from supported
  failures. Curated bundles/team rollouts are later extensions, not launch gates.

## Validation and rollout

Use temporary homes and projects with synthetic credentials; never test by
restoring into a developer's real agent configuration. Unit fixtures cover all
declared formats, with platform tests for Windows/macOS/Linux paths and modes.
Cross-OS restore claims require integration evidence for that source/target pair.
Do not mark restore support verified based solely on path detection tests.

Per PR run repository checks: `npm run check` in `web/`, `go vet ./...`, and
`go test -race ./...`. Add end-to-end CLI tests, archive corruption tests and
disposable restore exercises as the corresponding feature lands. Keep scope
small enough to review; do not publish a finished-product claim before S4.

No new service or dependency is required for S0. Later dependencies need a
specific justification. Evolve schema additively where possible; keep existing
backup formats readable and old clients functional. New server storage belongs
in later explicit migrations rather than coupling the local prototype to SQLite.

## Remaining engineering choices

Resolve these through implementation evidence rather than revisiting product
decisions: exact per-agent supported versions, archive streaming strategy,
package-manager provenance, cross-OS translation coverage, project identity
mapping, S3 credential provisioning, scheduler integration and retention defaults.
Record outcomes and reproducible validation in the handoff as each slice lands.
