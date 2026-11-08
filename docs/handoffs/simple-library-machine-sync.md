# reet.sh: simple library and machine sync

Staged implementation handoff · 8 November 2026

**Purpose:** make the basic cross-machine workflow dependable and simple while preserving reet's existing skill authoring, lists, likes and organisation features. This document can be handed to successive implementation models without the original conversation.

**Status:** Stage 0 completed on 8 November 2026; see the [Stage 0 results and implementation handover](simple-library-machine-sync-stage-0.md). Stages 1–7 are not implemented. The reported production Tailcat disconnections remain undiagnosed.

**Source baseline inspected:** `eyupio/reet.sh`, commit `40b5f83389b812da7dcbfb8b450400b2c85acc57`. Recheck the current checkout and its instructions before implementing. Earlier repository reviews are context, not proof of current runtime behaviour.

## Start here

1. Read the confirmed decisions and behaviour contract below. They take precedence over older plans describing whole-library rollout or manual preview/apply as the normal workflow.
2. Read applicable repository instructions, inspect the current Git state, and compare the checkout with the baseline above. Preserve unrelated work.
3. Find the first incomplete stage. Read its predecessors' handovers, then implement within the scope authorised for your session.
4. Reuse existing storage, content-transfer and filesystem protections where they satisfy this contract. Converge on one authoritative sync policy and execution path.
5. Run the stage's acceptance checks and relevant repository checks. Record actual evidence and limitations in the handover template at the end.
6. Keep stages reviewable. Completion of one stage is not evidence that later stages, real-machine recovery or provisioning have passed.

## 1. Product promise

**Connect a machine once. Keep its selected skills, agent definitions and portable settings in sync through one personal reet.sh library. Set up another machine through the same flow.**

The user should understand two things: their library, and their machines. Selecting an eligible item for a machine is sufficient to maintain it automatically. Discovery, content capture, planning, backup, transfer, verification and recoverable retries happen behind that intent.

A failure affecting one item, folder, application or machine must not stop unrelated work. Asking the user to repair an unrelated condition before a valid operation can proceed is a product failure.

### First release scope

- Linux machines.
- Claude Code, Codex, OpenCode and AGY / Antigravity: inventory, relevant portable configuration, and installation/configuration during new-machine provisioning.
- Shared/default skill locations and explicitly added custom or project locations.
- Reet-authored and imported skills, reusable agent definitions, per-machine selection, automatic content sync, version history, local-deletion restoration and library backup/recovery.
- Existing skills, authoring, private/public visibility, lists, likes, bookmarks, notes and tags remain available. Preserve existing sharing and permissions.

The exact Linux distributions, architectures, official application artifacts and application-specific writable formats need verification during implementation. Linux support is not permission to claim every distribution or architecture works. All four named applications remain in the requested scope; document any genuine support gap rather than silently dropping an application.

Existing session, terminal and other workspace features remain available. The two-view design below describes the library/machine experience, not deletion of the rest of reet.

## 2. Confirmed user decisions

| Area | Required behaviour |
| --- | --- |
| First useful journey | Connect two machines, import skills and agents, and copy selected items between them. |
| Source of truth | The personal reet.sh library holds the canonical content. Local edits can update it automatically. |
| Destinations | Each machine receives only selected items. Offer a simple **Select all** action. |
| Ongoing sync | Changes propagate automatically to the machines that select the item; an open browser is unnecessary. |
| Concurrent edits | Prefer the most recently saved version and preserve the other version in history. |
| Locations | Support shared/default and explicitly specified skill locations. |
| Connection library | Tailcat is the project's connection library; do not confuse it with a request to configure Tailscale. |
| Provisioning | Install and configure selected agent applications when setting up a new machine. |
| Local deletion | Restore a locally deleted managed skill/agent file from reet.sh, and inform the user through a web UI notification. |
| Native authoring | Keep creating and editing skills in reet.sh. Those skills participate in the same sync system as imported skills. |
| Skill status | Only **In use** skills participate in normal automatic distribution. To test, On the shelf, Disabled and other inactive states stay in the library. |
| Leaving In use | Remove reet-managed installed copies from machines, retaining the library entry, history and machine selections. Offline machines apply the latest intent when they reconnect. |
| Organisation | Keep lists, liked skills and existing organisation features. These do not override status or destination selection. |
| Model handoffs | Implementation must be staged, with explicit inputs, outputs, acceptance checks and handover notes. |

The requested benefits also include dependable backups and new-machine setup. Version history alone is not an independent backup of the controller's data.

## 3. Minimal experience

### Connect an existing machine

1. Choose **Connect machine** and run one copyable command, with a single pairing/consent step where required.
2. Reet starts its supervised background connector and discovers standard application and skill locations. **Add folder** covers custom/project locations.
3. Recognised content is captured privately into the library, including supporting files. Discovery metadata alone is not a completed import or backup.
4. Choose which library items the machine should maintain. Existing local content is identified without being needlessly rewritten. Reet continues in the background after the dialog closes.

The flow must not require a separate tour of token settings, private-bundle dialogs, comparison screens, saved sets, scans, previews and approvals. Pairing still establishes the actual local permissions needed for ongoing managed writes; simplify that consent instead of silently bypassing it.

### Set up a new machine

Use the same connection flow. Choose agent applications and library items. Reet installs supported applications, applies portable settings, places selected active skills/definitions in their compatible locations and reports verified progress.

An application's sign-in can be a separate remaining task while file sync and other applications continue. Report applications, files and authentication separately; a successful download is not a ready-to-use authenticated application.

### Write or edit a skill in reet.sh

Save it in the library privately or publicly according to its visibility setting. Its deployment status and selected machines determine distribution. An In use edit generates a new immutable revision and propagates without republishing, manually adding a shelf duplicate or reselecting every machine.

### Two primary views

| View | Main content and actions |
| --- | --- |
| **Library** | Skills and agent definitions/settings; authoring; existing status, likes and lists; search/filter; selected machines and sync summary; history and restore. One entry per logical item. |
| **Machines** | Persistent machine rows, connection state, last successful sync, selected applications and items, folders, and current progress. Connect a machine or change its selection here. |

Keep familiar Skills routes usable, redirecting or integrating older routes as necessary. Lists and liked skills can remain simple Library views/filters. Detailed activity, application-specific paths and diagnostics belong behind item/machine details.

Use plain states such as **Up to date**, **Syncing**, **Offline — updates waiting**, and **Needs attention**. Connectivity and verified sync completion are distinct facts. A machine can be reachable while one item is pending.

## 4. Behaviour contract

### 4.1 Skill status and machine selection

For each skill and destination:

```text
should_be_installed = skill.status == "in_use" AND destination_is_selected
```

| Situation | Required result |
| --- | --- |
| In use and selected | Maintain the current canonical revision automatically. |
| In use and unselected | Do not install it; remove an existing reet-managed copy for that deselected destination. |
| To test / On the shelf / Disabled | Keep the library entry, contents, metadata and history; remove reet-managed deployments. |
| Change back to In use | Resume on the retained selected destinations. |
| Like a skill or add it to a list | Update organisation only. |
| Publish/unpublish a skill | Change visibility; preserve identity, status, selected destinations and private sync capability. |

Skill status does not uninstall agent applications. App installation and selected portable configuration/definition items have their own explicit machine intent; do not add redundant sync switches to every item.

**Select all** selects the current eligible items shown by the selection UI. Proposed implementation default: it is a selection shortcut, not a hidden subscription to future library/list additions. Lists help choose groups of items; list membership alone does not cause a deployment.

### 4.2 Local deletion versus authorised removal

Distinguish these causes before acting:

- **Unexpected local deletion:** an item should still exist according to the current status and selection. Restore its missing managed content and emit an informational web notification after verification, for example: `Restored code-review on dev-box after it was deleted locally.`
- **Status changed out of In use:** remove reet-managed copies, keeping selections and history. This is intentional and must not cause restoration or an accidental-deletion notification.
- **Machine deselected in reet.sh:** remove that destination's managed copy and stop maintaining it there. Other selected machines remain unchanged.
- **Library deletion:** treat an explicit library deletion as removal intent for its managed deployments; retain recoverability through the existing trash/history/backup policy. Do not infer library deletion from a missing local file.

Fence in-flight operations against the latest intent. A delayed install, stale local edit, old restore job or late acknowledgement must not resurrect a skill after deactivation, deselection or deletion.

Removal applies only to content reet can prove it manages. Preserve unrelated files, unmanaged installations and locally modified versions that have not yet been captured. Capture recoverable bytes before replacing/removing them. Do not make the whole fleet wait for an unrelated ownership ambiguity.

For the initial implementation, missing files inside a managed bundle are also local deletions to restore, not implicit permission to delete those files from the canonical manifest. Central edits can deliberately change the manifest. Debounce atomic-save patterns and observe a stable snapshot before classifying a disappearance; routine editor saves must not generate restore loops or notification spam.

### 4.3 Content, identity and ordering

- Store complete private bytes and manifests, including supporting files and executable flags. No public URL or live source machine should be needed after capture.
- Validate skills, agent definitions and portable settings according to their actual item type. Reuse blob/manifest storage where appropriate; do not require a fake `SKILL.md` wrapper for an agent definition or configuration file.
- Use stable logical item IDs. A display name, folder basename, hostname or upstream URL is not sufficient identity.
- Link a discovered local installation to its known item through receipts/provenance and content evidence. Separate unrelated same-name imports rather than silently merging them.
- Two different items competing for one destination slot affect that slot only. Preserve both library entries, keep the last verified deployment intact and explain the collision locally. Do not offer a fleet-wide dedupe prerequisite.
- Revisions are immutable. Retry receipts and deployment acknowledgements are idempotent. A replayed response cannot move the canonical head backwards.
- The deployment unit is a complete item revision. Do not create an accidental mixture of files from competing revisions.
- Preserve previous and concurrent versions for user-visible restoration. A quota or retention operation must not silently destroy the version just promised as recoverable.

**Most recently saved needs an explicit ordering rule.** Arrival at the controller is not the same as save time: an old offline edit may reconnect late. Record the base revision, source save time, machine identity and a stable event ID/counter. Respect causal ordering; compare concurrent edits using validated save-time information, and define a deterministic tie-breaker. Offline clocks can be wrong, so document the uncertainty and fallback rather than claiming impossible perfect time ordering. Preserve both versions and continue automatically; ambiguous ordering must not introduce a blocking conflict-resolution workflow. Include tests where an older edit arrives last and where clocks disagree.

Applying a remote revision must not be reimported as a new user edit. Use recorded receipts/digests plus observation of genuine local changes to avoid feedback loops.

### 4.4 Connectivity and scheduling

- Persist the machine's enrollment identity, selected items, folders and pending work independently of the current Tailcat connection, process instance, hostname label or IP address.
- Sleep, a dropped connection, controller restart or connector restart makes the same enrolled machine temporarily offline; it does not forget the machine or create a duplicate enrollment.
- Reconnect automatically with bounded backoff and resume from durable acknowledgements and current desired state.
- Background library sync must not depend on a live terminal/interactive session. The inspected code already has HTTPS sync polling alongside private monitoring: verify and repair this separation before proposing another transport.
- If HTTPS still works while the Tailcat connection is down, file sync should continue. If all connectivity is unavailable, retain work and show honest offline status.
- Supervise the connector so supported installations recover after crash/reboot. A detached unsupervised process is not proof of restart resilience.
- Queue requests for offline machines without requiring a recent whole-machine scan. Fresh filesystem/permission checks still occur before writes on the relevant destination.
- Authentication revocation and genuine permission problems remain visible and specific; they do not delete saved selections or masquerade as an ordinary connection retry.

### 4.5 Partial failure and observation

- Track progress and errors per item/destination. An unreadable folder, malformed skill, failed capture or blocked app install cannot stop healthy unrelated items.
- Incomplete observations are not evidence of deletion. Keep previously verified content and inventory where the new scan cannot establish absence.
- Never display Up to date from desired selection, successful enrollment, a heartbeat, metadata discovery, an HTTP success or an unverified apply response alone. Verify the deployed content against the current desired revision.
- Retry transient failures automatically with backoff. Persistent problems keep a concise explanation and focused retry/action beside the affected item.
- Intent can be saved while a machine is offline; background reconciliation handles prerequisites in order. Normal users do not orchestrate the internal job graph.
- Notifications report completed restores or actionable failures; batch repeated events and deduplicate reconnect/replay events. A notification is not a permission gate for unrelated work.

### 4.6 Application and folder boundaries

Discover known shared/default paths and honour applicable application overrides. Let users register additional folders explicitly. Store machine-local folder references; do not copy one machine's absolute home/project path onto another.

Each adapter must declare its actual file formats and placement rules. Keep app-specific definitions distinct where formats differ. Do not promise automatic conversion between incompatible agent formats. A new machine should be able to create a supported missing skills folder without requiring the agent app to have run once.

Portable settings exclude machine credentials and authentication databases by default. Install each application using its supported source, and guide any necessary local sign-in separately. Reuse existing configuration classification and setup code rather than copying entire application directories blindly.

## 5. Existing implementation: starting points and differences

These references were inspected at the source baseline. Revalidate them; file presence or earlier passing tests do not prove the new contract is already implemented.

| Concern | Existing entry points | Implication |
| --- | --- | --- |
| Skill metadata/status | `internal/store/store.go`; `web/src/lib/shelf.ts`; `web/src/lib/SkillCard.svelte` | Current shelf stage is derived as `used ? use : bookmarked ? try : shelf`. Preserve likes/bookmarks and make deployment eligibility authoritative across callers. |
| Shared-library selection | `internal/store/store_sync_library.go` (`SyncLibraryFor`, `prepareLibraryTargets`); `internal/api/sync_control.go` | Current automatic home-folder policy gathers the shared library and enrollment enables `KeepSynced`. Replace whole-library rollout with explicit machine selections and status eligibility. |
| Scheduling and jobs | `internal/store/store_sync_automation.go`; `internal/store/store_sync_control.go`; `internal/api/sync_control.go` | Reuse durable jobs where appropriate, but align them to one intent model. Managed removals must no longer stall behind the old per-plan review workflow after authorised status/selection changes. |
| Private contents and imports | `internal/store/store_private_skills.go`; `internal/store/store_private_reclaim.go`; `internal/store/store_machine_skill_import.go`; `internal/api/machine_skill_import.go`; `internal/skillbundle/` | Private immutable revisions and complete bundle handling already exist. Unify their identity/projection with authored and imported library items; verify history retention against quota reclamation. |
| Private target execution | `internal/store/store_private_sync.go`; `internal/skillplan/`; `internal/scan/private_sync.go`; `internal/scan/agent_apply.go` | Preserve containment, ownership, stale-plan and acknowledgement protections while simplifying orchestration. Do not run separate competing policies against one destination. |
| Connector and recovery | `internal/scan/agent.go`; `internal/scan/agent_daemon.go`; `internal/scan/agent_status.go`; `internal/terminalhost/service_agent.go`; `scripts/install.sh` | HTTPS polling and private monitoring run alongside one another. Investigate real disconnect/lifecycle behaviour instead of equating a dead private connection with a lost enrollment. |
| Tailcat/private connection | `internal/agenthealth/agent.go`; `internal/agenthealth/hub.go`; `internal/privateconn/` | Preserve stable enrollment across changing connection/process instances. No root cause of the user's disconnects was established in this planning task. |
| Current UI | `web/src/lib/Machines.svelte`; `web/src/lib/SyncSetup.svelte`; `web/src/lib/machineJoin.ts`; `web/src/lib/AddMachineDialog.svelte`; `web/src/lib/Hosting.svelte`; `web/src/lib/HostedShelf.svelte` | Integrate authoring, private items and sync intent into Library/Machines. Replace duplicate user journeys rather than adding a new setup screen beside them. |
| Setup/configuration | `internal/setup/adapters.go`; `internal/setup/`; `internal/store/store_setup_profiles.go`; `internal/api/setup_profiles.go` | Reuse portable configuration discovery/restore. Current known-path discovery is not support for arbitrary custom roots or all AGY settings. |
| Application installation | `internal/agentsoftware/`; `internal/terminalagent/agent.go`; `docs/agent-software.md` | Inspected managed installer supports a reviewed OpenCode Linux amd64 artifact. Claude Code, Codex and AGY provisioning need verified implementation, not UI-only support claims. |
| Backup/recovery | `internal/backup/`; `internal/store/store_backups.go`; `internal/api/backups.go`; `cmd/reet/backup.go` | Extend consistent archives to the canonical library, metadata and selections. Prove restore, rather than treating shelf JSON or discovery as a backup. |

Useful prior context: `docs/research/skills-sync-ux-review.md`, `docs/research/skills-machines-sync-review-2026-11-05.md`, `docs/remote-machine-operations.md`, `docs/private-skill-revisions.md`, `docs/setup-discovery.md` and `docs/controller-sync.md`. Some record earlier implementations and superseded vocabulary.

### Migration and compatibility defaults

- Preserve existing skills, authored content, public/private visibility, lists and memberships, likes, bookmarks, notes, tags, machine IDs and valid ownership receipts.
- Existing `used=true` maps to In use; otherwise `bookmarked=true` maps to To test; otherwise On the shelf. Introduce Disabled explicitly if the implementation needs it, without clearing organisation metadata.
- Give authored skills a deployment status without requiring a second shelf object. Proposed default for genuinely new skills or unclassified imports: To test. Existing explicit status wins over scan observations. Original unmanaged source files are not removed merely because an import is inactive.
- Project old API/MCP flag reads and writes through the same authoritative status/intent logic. Avoid two writable representations that drift. Existing supported integrations must continue to work without obtaining new machine-execution privileges.
- Preserve existing explicit destination intent where it can be mapped. Snapshot any legacy whole-library policy into explicit selections; do not silently re-enable subscriptions or select all future items.
- Unknown ownership is not permission to remove. Establish an adoption baseline from authorised local evidence, or preserve the specific unknown copy while continuing other work.
- Negotiate controller/connector capabilities. Do not send automatic managed-removal semantics to an older connector that cannot enforce them. Upgrades must not reactivate the old broad rollout as a fallback.
- Retire competing writers for a migrated destination. Keep a reversible migration record; never let old and new schedulers maintain the same files simultaneously.

## 6. Implementation stages

Stages are sequential integration checkpoints. A later model must receive the earlier model's evidence, schema/API decisions and unresolved issues. Internal rollout controls may keep incomplete policy changes inactive; they are not additional user-facing setup switches.

### Stage 0 — Rebaseline and make the contract testable

**Entry:** this document and the current checkout.

**Deliver:** a short code map of current identity, content, status, scheduling and ownership flows; a migration plan; and a reusable two-connector test harness. Identify the current production failure with focused logs/reproduction when accessible, keeping evidence separate from hypotheses. Record the supported Linux/app/path matrix and investigate official provisioning sources.

**Acceptance:**

- Record the actual starting commit, dirty files and applicable instructions.
- Locate every current writer of library heads, status and destination intent, including authoring, scans and existing API/MCP paths.
- Reproduce at least one relevant failure where possible; if unavailable, record exactly what runtime evidence is missing. Do not label the Tailcat issue fixed from source inspection.
- Demonstrate that the harness starts two separately enrolled connectors with separate homes/configs and a real controller; it can interrupt connectivity and inspect actual files/results.
- Include failing contract scenarios for selected-only rollout, inactive authored skills and local-delete restoration if the current code does not satisfy them. These must exercise behaviour, not just mock labels.

**Handover:** updated map, chosen authoritative integration points, capability/migration approach, fixture layout, reproduction and next stage scope.

### Stage 1 — Persistent machine identity and connection recovery

**Depends on:** Stage 0.

**Deliver:** durable enrollment/selection state, correct reachability reporting, supervised connector lifecycle and automatic recovery. Fix actual lifecycle/connection defects demonstrated by evidence. Preserve independent HTTPS file-sync progress when private monitoring/Tailcat is interrupted.

**Acceptance:**

- Connector and controller restarts retain the same machine ID and selections; no duplicate/lost-machine record is created.
- A private-connection interruption with working HTTPS does not block the file-sync cycle. With all networking interrupted, work remains queued and resumes after reconnection.
- Persist and replay uncertain acknowledgements safely; no duplicate filesystem operation is inferred from a missing reply.
- Validate supported native-service restart behaviour, including reboot/logout constraints. Record unsupported host-manager cases honestly.
- Revoked credentials stop access without deleting the machine's saved configuration or affecting other machines.

**Handover:** failure/recovery traces, lifecycle guarantees, remaining field-only checks, touched protocol/version boundaries.

### Stage 2 — Canonical content, authoring and status

**Depends on:** Stage 1; content-policy changes remain inactive until their execution semantics are available.

**Deliver:** one logical library projection and authoritative item identity/status, complete immutable contents, existing organisation features, custom-root support and migration mappings.

**Acceptance:**

- Import a skill with supporting files and executable flags; fetch/restore its exact bytes with its source machine offline.
- A private reet-authored skill is represented once and sync-capable without publishing. Editing it changes its revision, not its logical identity or selected machines.
- Likes, lists/memberships, bookmarks, notes/tags and existing visibility survive migration and edits. Metadata organisation does not grant deployment intent.
- Preserve existing shelf stages using the verified mapping. Saving an authored skill does not silently make it In use.
- Test known/shared paths, an explicitly registered custom path, a project path, app overrides and two distinct same-name items.
- One failed capture or unreadable location leaves other complete imports available; incomplete observations preserve previous content.

**Handover:** schemas/projections, compatibility translation, migration fixtures, manifest/identity rules and retention behaviour.

### Stage 3 — Automatic selected-item sync and edit propagation

**Depends on:** Stage 2.

**Deliver:** durable per-machine selections and automatic reconciliation through one execution path. Status and selection are evaluated by the scheduler, not only by UI filters. Add/update operations run without a browser or manual preview/apply chain.

**Acceptance:**

- With three enrolled machines, select a skill for A and B only. Actual content reaches A/B; C receives nothing.
- An In use reet-authored skill and an imported skill use the same behaviour. To test/On the shelf/Disabled skills are excluded.
- Edit on A, observe the canonical revision, and verify matching bytes on B. Repeat with a central edit and a B edit. No import/apply loop or endless no-op jobs follow.
- Restart with queued work, disconnect B, make changes, reconnect B and converge to current intent without reselection.
- Preserve two offline edits, choose the latest saved version according to the documented ordering rule, and verify an older edit arriving last does not silently win by arrival time.
- A malformed or blocked item does not prevent another item/destination from completing. Selecting an offline destination is accepted and remains pending.

**Handover:** ordering/idempotency rules, selected-only protocol, automated convergence evidence and measured normal/reconnect sync latency.

### Stage 4 — Managed removal, restoration and notifications

**Depends on:** Stage 3.

**Deliver:** enforce status/deselection removal and the distinct local-deletion restore policy, with recoverable contents and factual web notifications.

**Acceptance:**

- Change In use to To test or Disabled: remove managed deployments, retain library/history/selections and leave unrelated files untouched.
- With B offline, change status, reconnect B and remove according to current intent. Re-enable In use and restore the latest revision to retained selected destinations.
- Deselect only B: remove B's managed copy while A stays current.
- Delete a currently desired managed file locally: restore verified canonical content and create one deduplicated informational notification. An atomic editor save is not misclassified as a deletion.
- Authorised removals never trigger accidental-deletion restores or those notifications.
- Change status while an apply/restore is in flight: stale work cannot resurrect the item. Preserve any unseen local edit before intentional removal.
- An ownership ambiguity or permission failure affects only the relevant deployment; it does not delete foreign files or block the fleet.

**Handover:** transition/race cases, ownership and backup evidence, notification event keys and offline recovery proof.

### Stage 5 — Library and Machines UX

**Depends on:** Stages 1–4.

**Deliver:** the two primary views and one connection/selection flow, backed by verified state. Preserve authoring, status, lists and likes while integrating or redirecting duplicate basic sync journeys.

**Acceptance:**

- Connect, import, mark In use/select destinations, and see verified progress without visiting a separate scan, source-capture, comparison or approval screen.
- Library status/likes/lists remain available and work for both authored and imported items. Inactive items remain findable and backed up.
- A selected offline machine stays visible and accepts intent changes. A partial failure is explained beside its affected item; successful items remain usable.
- Closing/reopening the browser does not stop sync. Refresh and restored notifications report durable actual events, not local component assumptions.
- Validate desktop and phone layouts, keyboard operation, readable progress and main actions. Capture screenshots of the key empty, active, offline, partial-failure and status-removal states.
- Existing library routes and supported integrations continue to work. Normal sync does not accidentally require terminal consent, tmux or agent authentication.

**Handover:** route/component changes, screenshots, browser scenario results and any remaining legacy entry points with their disposition.

### Stage 6 — Linux application provisioning through the same flow

**Depends on:** Stages 1–5.

**Deliver:** install/configure adapters for Claude Code, Codex, OpenCode and AGY; application selection and progress within the existing machine setup flow. Reuse verified compatible installs and preserve unrelated local configuration.

**Acceptance:**

- Record each application's official source, supported version/artifact, Linux distro/architecture requirements, verification and configuration layout. Do not extrapolate the current OpenCode-only installer to the other apps.
- On a clean supported Linux machine, select applications and items, then verify installed executable/version, permitted portable settings and actual deployed skill/definition files.
- Retry an interrupted setup without duplicating installations, losing existing configuration or reapplying completed work unnecessarily.
- A failed/unsupported application or an app awaiting login does not stop other applications or file sync.
- Report `installed`, `configured`, `files synced` and `sign-in needed` accurately. Do not claim provider-backed runtime usability from `--version` alone.
- Test custom roots and supported per-app formats. Reuse identical known locations and handle shared folders without competing writes.
- Keep credentials local by default. Avoid paid/provider API calls merely to prove file sync or installation.

**Handover:** per-app evidence and exact compatibility matrix, install/retry/configuration results, and unverified sign-in/runtime cases.

### Stage 7 — Independent backup, migration cutover and release proof

**Depends on:** Stages 1–6.

**Deliver:** consistent recoverable library backups, tested upgrade/cutover, documentation and final real-machine evidence.

**Acceptance:**

- Back up and restore complete content/revisions, inactive items, authored skills, statuses, likes, bookmarks, lists/memberships, notes/tags and destination intent. Verify archive consistency during edits.
- Restore onto an empty controller/storage instance and provision from restored content while the original source machine is offline. Verify bytes, permissions/executable flags and metadata.
- Do not clone another machine's local enrollment credentials when provisioning a replacement machine. Restore operational state only through the appropriate account/instance recovery boundary and verified machine identity.
- After restore or rollback, refresh desired intent and ownership before replaying old jobs. Preserve newer local content for reconciliation rather than blindly overwriting it from an old archive.
- Migrate a representative existing account with authored/private/public items, mixed statuses, lists/likes and legacy selections. Preserve data and prevent two schedulers or broad fallback rollout.
- Exercise two real Linux machine installations with browser closed, source offline, private connection dropped, all networking dropped, connector/controller restart, and supported native-service recovery. An isolated two-home harness is valuable but does not establish real sleep/reboot or network resilience.
- Run the cross-stage scenarios below and required CI/repository checks. Record failures and limits honestly. Do not declare the requested four-app provisioning scope complete with an untested app.

**Handover:** restore instructions, archive coverage/retention, migration/rollback runbook, real-machine traces, final check results and documented release limitations.

## 7. Cross-stage acceptance scenarios

Keep a small reusable set of tests based on actual state transitions and filesystem outcomes. Extend it as stages add behaviour; avoid multiple suites that merely restate UI labels.

| ID | Scenario | Required observation |
| --- | --- | --- |
| A01 | Three machines; select A/B only | Correct bytes on A/B, absent on C. |
| A02 | Private authored skill: To test → In use | No rollout before activation; normal selected rollout afterward. |
| A03 | Authored edit without publishing | Same item, new revision, selected machines updated. |
| A04 | Like/list/bookmark inactive skill | Organisation preserved, no deployment triggered. |
| A05 | In use → inactive while B offline | Online removals complete; B later removes; selections/history retained. |
| A06 | Inactive → In use | Current revision restored to retained selected destinations. |
| A07 | Deselect B | Managed B copy removed; A and library unchanged. |
| A08 | Delete desired file locally | File restored; one factual nonblocking notification. |
| A09 | Atomic save / apply-generated changes | No false delete warning, repeated revision or sync loop. |
| A10 | Two offline edits; older arrives last | Defined latest-save ordering, both versions recoverable. |
| A11 | Connection/controller/connector restart | Same enrollment, durable intent, safe replay and convergence. |
| A12 | Tailcat down; HTTPS available | File-sync progress continues; interactive availability reported separately. |
| A13 | Incomplete scan or one unreadable item | No mass deletion; unrelated items continue. |
| A14 | Same-name unrelated imports | Both retained; any destination collision isolated and explained. |
| A15 | Status changes during queued/applying restore | Latest control intent wins; no resurrection. |
| A16 | Unmanaged/modified files beside a managed item | Foreign files preserved; unseen edits captured before replacement/removal. |
| A17 | Four-app new-machine provisioning and retry | Verified install/config/files per app; partial/auth tasks do not block sync. |
| A18 | Restore with source machine unavailable | Complete library, metadata and selected content recoverable independently. |
| A19 | Legacy account and older connector | Lossless metadata migration; capability-safe policy, no broad rollout fallback. |
| A20 | Other account/viewer/revoked connector | Existing content and execution boundaries remain enforced. |

### Check selection

Use focused Go tests for changed packages and transition/race cases, then relevant integration/browser scenarios. Existing starting points include `internal/api/sync_library_test.go`, `internal/store/store_sync_library_test.go`, `internal/store/store_private_sync_test.go`, `internal/scan/agent_enroll_retry_test.go`, and `web/tests/skills-sync-simple.e2e.mjs` / `web/tests/add-machine.e2e.mjs`. Adapt assertions to the new approved policy; existing whole-library expectations are not the new contract.

Relevant repository commands at the inspected baseline include `go test ./internal/store ./internal/api ./internal/scan`, `npm run check` and `npm run build` from `web/`, and the repository's applicable integration/browser runners. Check current runner requirements and installed tools before invoking them. Run required full/race/CI checks for the final implementation; do not repeatedly broaden test runs after passing checks unless new changes or unresolved failures justify it.

Document exact commands, working directories, exit status and actual coverage. Mocked browser inventory does not prove content transport, service recovery or an application install. A local harness does not prove production Tailcat health.

## 8. Handover template for every implementation stage

Append a completed record when handing work to the next model. Keep this file or its repository copy with the implementation so future models receive the same decisions.

```markdown
### Stage N handover — YYYY-MM-DD

- Stage/status: complete | partial | blocked (with exact reason)
- Starting commit:
- Resulting commit/branch, or uncommitted files:
- Authorised scope completed:
- Behaviour and files changed:
- Schema/API/capability decisions and compatibility:
- Migration/activation state; scheduler ownership:
- Commands run (working directory, exit status, evidence location):
- Acceptance scenarios passed, failed and not run:
- Real machines/apps exercised; mocks/fixtures used:
- Recovery/rollback procedure and data retained:
- Known issues and precise evidence gaps:
- Next stage; concrete first task and remaining dependencies:
```

Do not replace observed failures with a later broad claim of success. Do not require the next model to infer decisions from chat history or recreate the previous model's test environment. Put reusable fixtures/scripts in the repository, and keep credentials and private logs out of the handoff.

## 9. Completion ledger

- [x] Stage 0: baseline, migration design and real-connector harness
- [ ] Stage 1: identity, supervision and connection recovery
- [ ] Stage 2: complete library, authoring, status and metadata
- [ ] Stage 3: selected-item automatic sync and edits
- [ ] Stage 4: removal, restoration and notifications
- [ ] Stage 5: Library and Machines UX
- [ ] Stage 6: Linux provisioning for all four applications
- [ ] Stage 7: independent backup, migration and release proof

**Evidence:** the original specification was prepared from read-only repository inspection and the user's decisions. Stage 0 added a real-controller/subprocess test harness and opt-in contract regressions, with verification and remaining evidence gaps recorded in its linked handover. No runtime defect is claimed fixed and no live provisioning has been performed.
