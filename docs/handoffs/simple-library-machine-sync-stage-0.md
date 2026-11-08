# Simple library and machine sync: Stage 0

8 November 2026 · implementation handover

Parent specification: [Simple library and machine sync](simple-library-machine-sync.md).

**Status:** Stage 0 complete. Baseline, reusable subprocess harness, migration/source map and five contract failures are recorded. The focused integration controls pass with race detection. No production policy, schema, UI or installation behaviour has been changed.

## Baseline and scope

| Fact | Observed value |
| --- | --- |
| Starting source commit | `40b5f83389b812da7dcbfb8b450400b2c85acc57` |
| Branch | `work` |
| Starting uncommitted work | Only the previously prepared `docs/handoffs/simple-library-machine-sync.md` was untracked. |
| Repository instructions | No `AGENTS.md` was found in the checkout or its workspace ancestors. Recheck before continuing. |
| Local test host | Debian GNU/Linux 13.6, Linux amd64. |
| Toolchain | The module declares Go 1.27.1. The supplied base executable is Go 1.26.10; automatic toolchain selection downloaded and used Go 1.27.1. Module/toolchain caches stayed under `/workspace/.reet-tools`. |
| Runtime reach | Local real HTTP controller and separate connector processes are available. No access to production connector logs or native-service recovery was established. |

The new code is test infrastructure and opt-in desired-behaviour tests. Existing runtime behaviour is deliberately unchanged in Stage 0.

## Reproduced product gaps

The contract tests run actual controller routes, actual connector enrollment/poll/apply code, a temporary SQLite store and real files. They are guarded by the `sync_contract` build tag because the approved behaviour will be implemented in later stages.

| Contract test | Observation at the baseline | Relevant implementation |
| --- | --- | --- |
| `TestSimpleSyncContractSelectedItemsOnly` | Both machines accept a selection containing one active skill, then install the other unselected active skill as well. | `prepareLibraryTargets` rewrites a home target to the whole library and following-source policy. |
| `TestSimpleSyncContractInactiveAuthoredSkillsStayOffMachines` | Both an unpublished draft and a published authored skill whose shelf status is To test are installed. | `SyncLibraryViewFor` merges authored content into candidates; `SyncSkill` carries no lifecycle field and the shared-library path does not filter on `used`. |
| `TestSimpleSyncContractLocalDeletionRestoresAndNotifies` | Deleting the managed `SKILL.md` leaves it missing after eight connector cycles. The wired notification centre contains no restore notice. | The current scan/planner/retry path does not implement the agreed automatic local-deletion restoration policy. Inspect the captured jobs/target problem before changing its filesystem guards. |
| `TestSimpleSyncContractLeavingInUseRemovesManagedCopy` | Setting `used=false` retains the library entry but leaves its managed deployment installed. | Status does not change candidate eligibility, and automatic library targets use additive mode. |
| `TestSimpleSyncContractImportRecoversAfterConnectorRestart` | Restarting the actual source process after content preview leaves the destination without its skill, despite retaining both enrollments. | `ClaimRemoteImportFor` fences the old process-bound request as expired; background library retries do not restart that terminal capture automatically. |

These are reproduced application behaviours, not claims about the cause of the user's production Tailcat outage.

### Existing behaviour that passed

- `TestLibrarySyncNewMachinesDraftEditsAndReconnect`: the pre-existing two-home integration test passes, including private authored edits, machine-only content with supporting files and delayed polling.
- `TestScopedLibrarySyncPreservesOtherMachinePolicies`: the pre-existing scoped library request preserves the other machine's policy.
- The new harness proves separate process/home/config/enrollment isolation, real private-content transfer, a temporary HTTP failure on one connector, catch-up after reconnection, and persistence/replay of a controller-accepted result whose acknowledgement is lost. Final verification results are recorded below.

The normal machine-only import succeeds with one persistent connector process per machine. Restarting the source between preview and upload reproduces a separate recovery defect. Keep the process-instance fence: Stage 1 should safely requeue/repreview the capture under the new process rather than accepting a stale upload.

## Current code and authority map

### Identity, observation and transport

| Concern | Current authority and entry points |
| --- | --- |
| Enrollment | `sync_agents` owns the random enrollment ID and hashed installation credential. `EnrollSyncAgentBuild` / `AuthenticateSyncAgentBuild` in `internal/store/store_sync_control.go` are reached through `internal/api/sync_control.go`. |
| Local identity | `internal/scan/agent.go` persists the enrollment ID, secret, account credential, hostname/home binding, projects and local apply consent in the machine's private config. The test harness never exports that config. |
| Human machine identity | `internal/store/store_machines.go` resolves host aliases and merges/forgets reports. `EnvironmentIDFor` identifies scopes using account/host/agent/project; it is not the random enrollment ID. |
| Ordinary sync channel | `internal/scan/agent.go` polls `/api/v2/sync/poll`, processes durable `.result` acknowledgements, inventories and file jobs. The foreground loop polls every five seconds; ordinary scan cadence is one minute. |
| Private monitoring | A separate goroutine calls `agenthealth.RunAgent` from the same agent process. `internal/agenthealth/agent.go` maintains the private connection with backoff; `internal/privateconn/client.go` documents replacing the native Tailcat client after connection loss. |
| Process identity | Private monitoring/import sessions have process/connection instance IDs. They must not become replacement enrollment identities. |
| Local lifecycle | `internal/scan/agent_daemon.go`, `internal/terminalhost/service_agent.go` and `scripts/install.sh` manage background/native-service operation. A process launched with `run --once` tests none of the supervisor guarantees. |
| UI reachability | `web/src/lib/machines.ts` treats authenticated agent `lastSeen` within 90 seconds as online. `internal/scan/agent_status.go` separately reports private-connection state and last successful inventory/job cycle. Preserve that distinction. |

### Writers that must converge on the new policy

The following are the runtime writer families found by tracing the shelf, content-head and target SQL writes. They must delegate to the same authoritative policy after migration.

| Writer family | Current entry points | Written state / migration requirement |
| --- | --- | --- |
| Shelf add and patch | `UpsertFor`, `UpsertLocalFor`, `PatchFor`, `PatchManyFor` in `internal/store/store_users.go` | `shelf.used/bookmarked/liked/notes`. Add/upsert ORs supplied flags into existing flags; a patch can clear them. Preserve that compatibility while centralising lifecycle transitions. |
| Bulk stage changes | `BulkUpdateFor` in `internal/store/store_tidy.go` | Direct shelf flag writes plus tags/list membership. Updating only `PatchFor` would miss this path. |
| Legacy single-owner methods | `Patch` / `PatchMany` and upsert methods in `internal/store/store.go` | Older catalogue flag representation. Recheck current callers and translate through the same account lifecycle policy where still supported. |
| Web/API and MCP clients | `add` / `patch` / bulk handlers in `internal/api/api.go`; `mcp/src/server.ts`; `internal/api/mcp_tools.json` | MCP update/bulk tools call the existing HTTP shelf API. Preserve the tool surface; no new machine-execution endpoint permission is required. |
| Scan/import | `importEntry` in `internal/api/api.go`, reached by `internal/scan/scan.go` | Newly discovered entries are marked used; explicit flags on existing entries are retained. This import default differs from the handoff's proposed To test default for new unclassified imports. |
| Authored content | `SaveHostedSkill`, `RenameHostedSkill`, `DeleteHostedSkill` in `internal/store/store_hosted.go`; authoring routes in `internal/api/hosted.go` | `hosted_skills` has content/files/visibility but no authoritative lifecycle field. Publishing creates a bookmarked shelf projection; unpublished content can exist without one. Preserve identity across visibility changes. |
| Canonical private head | `savePrivateSkillRevisionWithReceipt` in `internal/store/store_private_skills.go` | Writes immutable revisions/files/blobs and CAS-updates `private_skill_bundles.head` transactionally. Reuse its account boundary, expected-base check and request receipts. |
| Authored/private binding | `hostedPrivateSource` in `internal/store/store_hosted_sync.go` | Materialises an authored revision and updates `hosted_sync_sources` when bytes change. Even candidate listing can trigger this write; centralise it rather than assuming it is a pure read. |
| Machine content import | `SaveMachineSkillImportFor` in `internal/store/store_machine_skill_import.go`; `RequestMachineSkillSync` / `AdvanceMachineSkillSyncs` / `bindPrivateSync` in `internal/store/store_private_sync.go` | Captures bytes through authenticated remote import and binds private sync sources. Generalise item types without making fake `SKILL.md` files for agent definitions. |
| Explicit desired set | `SaveDesiredFor` in `internal/store/store_sync_control.go`; `SaveSyncTargetFor` / `DeleteSyncTargetFor` in `internal/store/store_sync.go` | Writes `sync_targets`, `sync_options` and `sync_policies` after ownership, inventory, revision and pending-install checks. Current HTTP endpoint: `PUT /api/v2/sync/desired`. |
| Automatic library policy | `prepareLibraryTargets` / `syncLibraryMachinesFor` in `internal/store/store_sync_library.go`; `MaintainSync` in `internal/store/store_sync_automation.go` | Rewrites whole-library desired state and schedules jobs. Enrollment enables `keep_synced` in `internal/api/sync_control.go`. This is the selected-only migration's main integration point. |
| Session skill install/cancel | `InstallHostedSkillFor` / `CancelInstallFor` / `revertInstallTx` in `internal/store/store_skill_install.go`; `internal/api/terminal_skills_install.go` | Directly adds/removes desired entries and restores previous selections. Translate these operations into explicit item intent; do not leave a parallel writer. |
| Source-conflict repair | `ResolveSyncSourceConflictsFor` in `internal/store/store_sync_source_fix.go` | Rewrites desired sources/revisions. Preserve item identity and isolate conflicts to their affected destination. |
| Machine rename/merge | `MergeMachinesFor` in `internal/store/store_machines.go` | Moves environment IDs, source IDs and target IDs; invokes target/receipt migration helpers. A label change must not become an item or enrollment identity change. |
| Private pins and ownership | `SaveSkillContentTargetFor` / `DeleteSkillContentTargetFor` in `internal/store/store_content_targets.go`; `internal/store/store_content_receipts.go` | Existing exact-content intent/ownership baseline alongside legacy desired sets. Preserve receipts and converge on one destination writer. |
| Backup/restore and setup restore | `internal/backup/restore.go`; `internal/setup/restore_apply.go`; `internal/store/store_setup_profiles.go` | Restoring archived state or local config can reintroduce old intent/content. Refresh control versions and observations before dispatch after restore. |

### Static scheduling constraints to investigate

These are code observations, not separately reproduced production failures:

- `MaintainSync` waits while any non-update job for that machine is queued, running or awaiting approval. An unrelated pending job can hold the machine-wide scheduler.
- It requires a recent complete scope inventory before continuing that target.
- A failed/cancelled latest job for the same desired revision is not automatically retried.
- `ClaimRemoteImportFor` expires a preview/upload tied to an older import process instance; `remoteImportInstance` changes after a process restart. Completed and in-flight authorization fences must remain intact while adding automatic recovery.
- Legacy shelf/machine selections can still reject missing sources or require a separate capture/review step.
- Automatic library targets are additive; eligibility filtering alone will not remove inactive managed copies.
- Current known-path scanning and setup discovery are different subsystems. Supporting a path in discovery does not prove the writable sync adapter accepts it.

## Chosen integration approach and migration plan

This section records implementation direction, not schema changes already made.

### One authority per responsibility

1. **Enrollment:** retain existing stable `sync_agents.id`. Separate reachability, private-session state, last successful sync and desired work. Never reconstruct identity from a temporary Tailcat address or hostname alone.
2. **Content:** reuse immutable private revision/blob/receipt storage as the recoverable byte authority. Add explicit item type/validator support for definitions and portable settings in Stage 2. Preserve authored/public catalogue references as provenance.
3. **Lifecycle:** add one authoritative library lifecycle linked to stable logical item identity. Make shelf, authoring and MCP/API reads/writes project through it atomically. Likes and list membership remain organisation metadata.
4. **Destinations:** store explicit enrollment/item/location assignments. Status changes retain selection; deselection changes it. A local location reference is machine-owned and resolves only through authorised roots.
5. **Execution:** adapt the existing durable controller jobs and connector filesystem guards to the resulting per-item intent. Retire whole-library rewrites for migrated destinations. Do not introduce a competing sync daemon or let two schedulers write the same slot.

The exact new table/field names belong in the Stage 2 schema change and its handover. The existing separation between immutable bytes, deployment intent and verified ownership should remain explicit.

### Capability boundary

Negotiate the new semantics on the authenticated HTTPS sync channel. Relying only on capability advertisements from a live Tailcat session would recreate the connection dependency.

A versioned library-sync capability must cover selected-only intent, lifecycle eligibility, managed removal/restoration and intent-version fencing before those operations are sent to a connector. Finalise its name and exact wire fields with protocol tests; none are advertised by Stage 0.

Older connectors retain inspectable state and get a focused upgrade requirement for unsupported operations. They must not fall back to a broad whole-library policy or be treated as successfully synchronised.

### Expand, backfill, cut over

| Step | Required migration behaviour |
| --- | --- |
| M0: preserve evidence | Take a consistent recoverable snapshot of relevant content, metadata, selections and receipts before applying the migration. Record which local files have proven ownership; unknown files remain untouched. |
| M1: add identity/lifecycle | Add stable logical item links for authored, private and catalogue records. Use proven provenance/receipts to join aliases, preserving distinct same-name imports. Backfill explicit `used` / `bookmarked` stages without resetting likes, notes, lists or visibility. |
| M2: materialise selections | Preserve explicit old selections. Snapshot a legacy whole-library subscription into its current selected IDs, retaining inactive selections but excluding them from active deployment. Do not subscribe to future additions. |
| M3: prepare each destination | Require compatible local permissions/capability. Establish fresh root/ownership observations; finish or fence old in-flight writes before giving the new scheduler ownership. Do not turn an unknown copy into a removal target. |
| M4: switch one writer | Atomically mark the destination migrated, retire its old following-source rewrite and bump its control version. Translate older UI/API/session-install actions through the same intent. A late old result must not resurrect superseded intent. |
| M5: verify | Compare actual file digests and current intent before reporting Up to date. An error at one destination leaves other destinations progressing. |
| M6: rollback | Stop dispatch from the new policy first. Preserve new content/revisions and the migration journal; rollback must not reactivate a whole-library subscription or overwrite post-migration edits from an old database snapshot. |

This stage implements none of these migration steps. Stage 1 can repair connection/lifecycle behaviour independently; activate the new deployment policy only when the later stages implement its complete semantics.

## Reusable test harness

Files:

- `internal/api/sync_contract_harness_test.go`: shared fixture, connector subprocess entry point, fault links, optional evidence export, passing baseline tests.
- `internal/api/sync_contract_test.go`: opt-in approved-behaviour tests under `//go:build sync_contract`.

The fixture uses `api.New` and a real temporary SQLite database through `storetest.Open`. Each connector has an independent persistent child process, home, config and real enrollment. A small JSON driver requests controlled cycles of production `scan.RunAgentInput` with `run --once` while preserving that connector's process-scoped state between cycles. Its account token is disposable fixture data.

Each machine connects through its own loopback HTTP reverse proxy. This permits a controlled HTTP 503 on all its requests without disrupting another machine, or suppression of a successful result acknowledgement after the controller accepts it. The proxies preserve the controller Host so separate fault links cannot manufacture different hosted identities.

The harness imports machine-only skills through the actual capture path and verifies supporting-file bytes in the destination and canonical private revision. App configuration selectors are removed from child environments so the fixture does not scan the task's actual user setup.

**Limits:** these are separate connector processes on one Linux host, not two physical machines. HTTP fault injection is not a Tailcat packet-loss test. The harness does not run the production timed poll loop, private monitoring, native service installation, reboot/logout recovery, provider sign-in or application installers. Stage 1 must add the relevant long-running/private-channel tests. A separate tagged test deliberately restarts the source process during import.

### Run the passing infrastructure controls

From the repository root, with Go 1.27.1 available:

```sh
go test ./internal/api \
  -run '^(TestSimpleSyncHarness.*|TestSimpleSyncConnectorProcess)$' \
  -count=1 -v
```

Race checks:

```sh
GORACE=atexit_sleep_ms=0 go test -race ./internal/api \
  -run '^(TestSimpleSyncHarness.*|TestSimpleSyncConnectorProcess)$' \
  -count=1 -v
```

The race option removes the race runtime's exit delay from connector subprocesses. Race detection and its failure exit code remain enabled. The harness closes each worker's input and checks its graceful exit, so a child race-detector failure is not hidden by killing the process during cleanup.

### Run the desired behaviour probes

```sh
go test -tags=sync_contract ./internal/api \
  -run '^TestSimpleSyncContract' \
  -count=1 -v
```

**At Stage 0 this command is expected to exit 1 with the five product failures above.** A compiler error, fixture setup failure or failing infrastructure control is not an expected contract failure. Normal tests do not include these tagged probes. Later stages should make their relevant probes pass, then promote those assertions into the ordinary test suite when the contract is active.

For evidence, set `REET_SYNC_CONTRACT_EVIDENCE_DIR` to a writable directory. JSON reports include machine enrollment IDs, fixture file SHA-256 values, request counts, lost-ack counts, target summaries, job status and import state/error. They exclude tokens, installation secrets and raw file contents. Temporary homes and databases are cleaned up after evidence is collected.

Local preparation used `/workspace/work/stage0-evidence` and log files under `/workspace/work`. Those are scratch evidence locations, not prerequisites for future models: all fixtures and commands needed to reproduce the results are in the repository.

## Linux and application/path matrix

| Application | Known paths/configuration at the source baseline | Writable sync / provisioning evidence |
| --- | --- | --- |
| Claude Code | `~/.claude/skills`; `CLAUDE_CONFIG_DIR` in setup discovery; project `.claude/skills`. Settings/instructions/MCP support exists in `internal/setup`. | Fixed Claude home/project skill roots are writable in the legacy sync adapter. General custom/override support must be aligned across discovery and execution. Managed app installation is not implemented. |
| Codex | Shared `~/.agents/skills` and legacy `~/.codex/skills`; `CODEX_HOME` in setup discovery; project `.agents/skills` / `.codex/skills` / `AGENTS.md`. | Shared `agents` roots are writable; legacy `codex` sync scope is scan-only. TOML/settings restore exists in setup. Managed Codex installation is not implemented. |
| OpenCode | `~/.config/opencode/skills`; `XDG_CONFIG_HOME` / `OPENCODE_CONFIG_DIR` / `OPENCODE_CONFIG` in discovery; project `.opencode/skills`. | Current managed installer verifies OpenCode 1.19.5 for Linux amd64. Global OpenCode skill sync is supported; project/general custom roots need alignment. |
| AGY / Antigravity | Native `agy` ACP integration exists. The prior 1.5.0 probe describes workspace customisation roots; the cask identifies `~/.gemini/antigravity-cli` as application data. | No AGY setup-discovery/configuration adapter or managed installation is present in the inspected registry. Do not infer AGY format/skill-path support just because shared `.agents` content exists. |

The path inventory is grounded in `internal/scan/scan.go`, `internal/scan/skill_import.go`, `internal/store/store_sync.go`, `internal/setup/adapters.go`, `internal/setup/support.go` and `internal/agentsoftware`. `internal/setup/policy.go` already classifies portable fields versus credential references; reuse it.

The local harness was exercised on Debian 13.6 amd64. It establishes neither general distro support nor successful provisioning of these applications.

### Official installation sources investigated

HTTP reads succeeded on 8 November 2026. Versions below are observed metadata, not newly verified runtime compatibility or permission to install an unreviewed latest version.

| Application | Source evidence | Observed candidate / consequence |
| --- | --- | --- |
| Claude Code | [Anthropic README](https://raw.githubusercontent.com/anthropics/claude-code/main/README.md); [package metadata](https://registry.npmjs.org/@anthropic-ai/claude-code/latest) | README recommends the native installer at `https://claude.ai/install.sh` and marks npm installation deprecated. npm metadata reports 2.1.319, Node >=22 and Linux x64/arm64 plus musl optional packages. Stage 6 should use a verified supported installation route, not assume older Node requirements. |
| Codex | [OpenAI README](https://raw.githubusercontent.com/openai/codex/main/README.md); [package metadata](https://registry.npmjs.org/@openai/codex/latest) | README documents the standalone installer at `https://chatgpt.com/codex/install.sh`, OpenAI release hosting/GitHub fallback and npm. npm reports 0.167.0 with Linux x64/arm64 package aliases. Pin and verify the selected native artifact; no paid API test is needed to prove file provisioning. |
| OpenCode | [package metadata](https://registry.npmjs.org/opencode-ai/latest); existing `internal/agentsoftware/artifact.go` | Metadata reports 1.19.5 and glibc/musl/platform variants. Existing code verifies the Linux x64 1.19.5 archive and binary. Other architectures and application-runtime compatibility still need evidence. |
| AGY | [Current cask](https://raw.githubusercontent.com/Homebrew/homebrew-cask/main/Casks/a/antigravity-cli.rb); `internal/terminalhost/testdata/help/sources.json` | Cask reports 1.5.3 and Google-hosted Linux x64/arm64 archives with checksums, installing the extracted `antigravity` executable as `agy`. Reet's recorded ACP probe is 1.5.0, so latest-version compatibility must not be assumed. |

The previously reviewed AGY 1.5.0 Linux x64 URL still answered HTTP 200 to a HEAD request:

`https://storage.googleapis.com/antigravity-public/antigravity-cli/1.5.0-5269980577660928/linux-x64/cli_linux_x64.tar.gz`

Its recorded SHA-256 is `f65e365a67be49abdaf92f5669c3157bd4b9c569f6b9f448340b00ca261b0f01`. This stage checked availability, not a fresh 69 MB artifact download/checksum or execution. No installer was executed and no provider request was made.

## Production evidence and remaining access gap

The connected read tools `reet_list_environments` and `reet_list_sync_targets` were tried once each. Both returned MCP `-32603: Internal error` without usable inventory or target data.

This does not establish whether the cause is the MCP service, authentication, controller availability or a machine connection. No production credential was changed and no live inventory, selection or file was mutated. There are no production logs in this handoff.

Stage 1 still needs evidence from the affected long-running connector/controller: version and persisted enrollment ID; last successful HTTPS poll; private-session disconnect/reconnect sequence; service supervision/restart state; and whether the record is merely offline, filtered by inventory, or actually missing. Use redacted focused evidence. A synthetic recovery test or the MCP error above cannot establish that root cause.

## Verification record

| Check | Result |
| --- | --- |
| Existing two-home integration | Passed. |
| Existing scoped library request | Passed. |
| New infrastructure controls | Offline recovery, lost acknowledgement and machine-only content import all passed. |
| Final focused race run | The three infrastructure controls, connector subprocess entry point and both existing sync integration tests passed; package test time 26.246 seconds. No reported races. |
| Tagged desired-behaviour probes | Five expected failures, matching the reproduced product gaps. Evidence export is recorded under `/workspace/work/stage0-evidence`. |
| UI / actual application provisioning | Not changed or exercised. |
| Real Tailcat/native-service recovery | Not exercised; evidence gap documented above. |

The final race command, from the repository root, was:

```sh
GORACE=atexit_sleep_ms=0 \
REET_SYNC_CONTRACT_EVIDENCE_DIR=/workspace/work/stage0-evidence \
go test -race ./internal/api \
  -run '^(TestSimpleSyncHarness.*|TestSimpleSyncConnectorProcess|TestLibrarySyncNewMachinesDraftEditsAndReconnect|TestScopedLibrarySyncPreservesOtherMachinePolicies)$' \
  -count=1 -v
```

Its full local log is `/workspace/work/stage0-final-race.log`. The final desired-behaviour probe command is the tagged command above; its full local log is `/workspace/work/stage0-final-contract.log`. The import restart report records `state=expired` / `error=preview_expired` while retaining two machine enrollments. Full repository CI and application provisioning were not run for this test/documentation stage.

## Next model: Stage 1

Start with the reproduced import-restart failure. Preserve the process-instance fence while requeuing a new authenticated preview/capture, using the same enrollment and user intent. The normal import control must keep passing and the restart probe should turn green.

Then use the existing HTTPS/private-monitoring separation and stable enrollment ID. Add a long-running connector test that rejects/drops private bootstrap/control while HTTPS inventory and sync continue.

Verify reconnects preserve the same enrollment and selections, and add controller/connector restart tests before changing any identity or service code. Investigate the actual reported loss when logs are available. Avoid changing the lifecycle/status/selection policy merely to make later-stage red tests pass.

Keep the remaining four tagged contract failures explicit until their respective stages implement them. Selected-only rollout is Stage 3, content/status unification begins in Stage 2, and managed removal/restoration is Stage 4.

## Stage 0 handover — 2026-11-08

- **Status:** complete, with the production Tailcat evidence gap explicitly retained.
- **Starting commit:** `40b5f83389b812da7dcbfb8b450400b2c85acc57`, branch `work`.
- **Result:** uncommitted test/documentation changes; no push, deployment, database migration or production policy change.
- **Files:** `internal/api/sync_contract_harness_test.go`, `internal/api/sync_contract_test.go`, this report, and the parent handoff's progress record.
- **Authorised scope:** Stage 0 only.
- **Contracts/compatibility:** no runtime schema/API/capability changes. Proposed authority/migration boundaries are documented above.
- **Tests:** passing real-HTTP/subprocess infrastructure controls and existing sync integrations under the race detector; five intentionally failing opt-in contract probes.
- **Real runtime coverage:** local Debian 13.6 amd64, two connector processes per transfer test, actual temporary files/SQLite/private content transport. No physical-machine, Tailcat, supervisor/reboot or four-application provisioning claim.
- **Recovery/rollback:** remove the Stage 0 tests/report/progress edits to revert this stage. No user data or runtime migration requires rollback.
- **Next stage:** Stage 1; first make the import-restart regression recover safely without weakening process-instance authorization fencing.
