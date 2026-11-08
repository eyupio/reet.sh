# Simple library and machine sync: Stage 2

8 November 2026 · implementation handover

Parent: [product contract and staged plan](simple-library-machine-sync.md). Predecessors: [Stage 0 authority/migration map](simple-library-machine-sync-stage-0.md), [Stage 1 recovery](simple-library-machine-sync-stage-1.md).

Pull request: [#664](https://github.com/eyupio/reet.sh/pull/664). Initial implementation commit: `85f72eb74074434b900f9c219f0b2a116c710454`; the follow-up records final compatibility fixes and validation.

**Status:** Stage 2 complete. The repository-wide race run passed all 44 test packages. Supplemental race checks cover the final import, backup and account-migration refinements. Web check/build, MCP tests and vet pass. Stage 3 is next; no preview or production deployment was created.

## Baseline and review boundary

- Starting commit: `6835558d0fdfc5da30142741998eb37d325dd033`.
- Branch: `feat/canonical-library-stage-2`, based on `feat/simple-library-sync-recovery` because [PR #662](https://github.com/eyupio/reet.sh/pull/662) is still open. The new PR must target that branch so its diff contains Stage 2 only. Retarget/rebaseline after Stage 1 merges; do not merge the dependency as part of this task.
- `origin/main` was fetched at `4e06d65c`; its unrelated changes were not folded into the open Stage 1 branch.
- No applicable `AGENTS.md` was found in the checkout or workspace ancestors.
- Stage 1's final head passed GitHub's image/full-suite, private-connection and native-service workflows. One shared-native-view run was cancelled and benchmarks were still running when checked. Those observations establish the dependency baseline, not Stage 2 CI.
- No production inventory, tokens or machines were changed. No preview URL, deployment or application installation was created.

## What is authoritative now

`internal/store/store_canonical_library.go` adds account-owned logical items. The new `/api/v2/library` projection preserves different items with the same name. It does not reuse the legacy sync candidate list's name-based deduplication as library identity.

| Responsibility | Authority |
| --- | --- |
| Logical identity | Random 32-character item ID, scoped to the account. Names, public URLs, publication and revision IDs are separate. |
| Provenance | `library_aliases`: shelf/catalog reference, authored slug, immutable bundle and authenticated machine/location capture. Aliases are joined only through a proven binding. |
| Lifecycle | `library_items.status`: `in_use`, `to_test`, `on_shelf`, `disabled`. Versioned status writes reject stale control updates. |
| Complete content | Existing immutable private bundles/revisions/files/blobs, including exact bytes and executable-bit presence. The item points at its current complete revision. |
| Organisation | Existing shelf flags/notes, tags and list membership. The library projects them rather than copying them into another mutable store. |
| Local source location | `library_locations`, scoped to an enrollment. A path is local metadata, never a destination path copied to another machine. |
| Destination intent/execution | Existing saved targets, policies, jobs and ownership receipts. This stage does not cut over their scheduler. |

Authored saves materialise private immutable content without publication. A later edit changes the revision, and publish/unpublish and rename preserve the logical item ID and lifecycle. A delayed authored snapshot cannot replace a newer saved head. A bad capture retains the editor's source bytes, the last complete revision and an item-specific problem. Restoring the last good bytes clears that problem without creating a redundant revision.

New canonical-only captures/types can have no legacy shelf metadata row yet. When the later interface adds organisation controls for them, use ID-based compatible metadata links; do not key unrelated items by name or make metadata creation eligible for legacy automatic rollout.

`contentState` is explicit: `pending_capture`, `complete`, `previous_revision` or `unavailable`. Metadata-only discoveries are not presented as complete backups. Catalogue items without captured bytes remain pending.

## Schema, migration and retention

The additive migration creates:

- `library_items`, `library_aliases` and `library_locations`.
- `library_migrations`, with the atomic `identity-v1` completion marker.
- `library_legacy_stages`, retaining the original shelf flags/notes as migration evidence.
- `library_projection_guard`, used only inside SQLite trigger transactions while projecting a canonical stage back to legacy flags. It is not an application lock or queued user task.

Private bundles gain `item_type` (default `skill`), `format` and `application`. Existing bundle/revision IDs and contents remain readable. Omitted and explicit default skill types preserve the old request fingerprint, so old receipt retries remain valid.

Database `user_version` becomes **2**. New typed revisions can omit `SKILL.md`, which a Stage 1 private-content reader cannot interpret. A backup made with this format must be rejected by an older restore reader rather than described as completely readable. The backup refusal test now derives its expected current version from `store.SchemaVersion`.

The migration runs through production `store.Open` in one transaction. It snapshots original stage evidence, links known authored/shelf/private records, and records completion. It does not delete catalogue rows, metadata, content, selections or receipts. Reopening/repeating it preserves generated item IDs. It is an expansion step, not the later destination-policy cutover or an independent backup.

Migration evidence belongs to its account and cascades on account deletion; the reopen/migration test verifies private notes are not left behind. Existing backup import now counts library items and private bundles when deciding whether a controller already holds user data. A library-only controller cannot be silently treated as empty. Restore reports, including the existing web report, show the added contents. This is compatibility with existing database backups; independent backup/release proof remains Stage 7.

Legacy stages map as agreed: `used=true` → In use; otherwise `bookmarked=true` → To test; otherwise On the shelf. Existing explicit stages win over the authored default. New authored items and new registered-root captures default to To test. The old scan/import pipeline keeps its historical defaults until Stage 3 translates its intent; do not silently infer a fleet-wide policy migration from this expansion.

Completed captures, explicit user uploads and authored content protect their current canonical revision from reclamation. Bare old transfer copies with no trusted binding retain their previous bounded cleanup policy; they are not silently adopted as permanent library entries. Backfill trusts existing source bindings or completed capture receipts, not a caller-supplied origin string alone. Known local scopes can reuse a proven capture from the same enrollment/scope. Custom/overridden folders do not claim a same-name legacy item.

Existing bounds remain 128 bundles, 256 revisions and 64 MiB of unique bytes per account. Existing bounded history reclamation still applies to superseded, unpinned revisions and eligible dead copies; this is not unlimited history. Current claimed content, ownership receipts, pins and other live references remain protected. One item reaching a limit must leave other complete items usable.

## Lifecycle and existing clients

Shelf insert/stage triggers cover normal upserts, import, restore and direct bulk flag writes. Canonical-to-shelf projection runs in the same transaction with a guard against feedback loops. `PatchFor`, `PatchManyFor` and `BulkUpdateFor` also handle an explicit legacy stage request whose flags happen to be unchanged, including dry-run rollback.

Likes/notes edits do not reset Disabled. Publishing an existing skill no longer forces its bookmark/stage back to To test. Public visibility remains separate from deployment lifecycle.

Owned shelf reads now include additive `libraryItemId` and `status` fields. Existing `stage` output/filtering uses the canonical lifecycle, including Disabled. MCP's existing list tool accepts the additional `disabled` read filter; its generated hosted-tool contract is updated. Legacy three-stage flag writes continue through the same authority. No MCP machine-execution or private-content permission was added.

**Activation boundary:** status is now authoritative data. The legacy whole-library scheduler still has its old deployment behaviour. Selected-only enforcement is Stage 3; removal/restoration is Stage 4. Do not advertise that setting Disabled already removes a legacy installation, or expose a new UI promising that behaviour before its execution path exists.

## Typed content

The private content API accepts optional `itemType`, `format` and `application` fields. Bundle type/format/application are immutable. `skillbundle.FromFiles` reuses the same bounded path, bytes and executable-mode identity for non-skill files; existing `FromContents` still requires a real skill manifest.

| Type | Validation and current scope |
| --- | --- |
| `skill` | Existing matching-name `SKILL.md` validation and complete supporting files. |
| `agent_definition` | One non-executable UTF-8 file, up to 1 MiB. Claude/OpenCode Markdown validates frontmatter/body; Codex TOML requires developer instructions. AGY Markdown instruction storage is syntax validation, not a claim of verified AGY runtime compatibility or automatic conversion. |
| `portable_settings` | One non-executable file, up to 1 MiB, using the existing setup format parser and export allowlist. Credentials, hooks, project/account state and unlisted fields are rejected. Claude/Codex/OpenCode policies are reused; AGY settings remain unsupported pending its actual adapter/format work. |

The complete bundle still has the existing path/size bounds. Credential-like content checks remain in place for portable captures. Stored bytes are never executed by import.

AGY remains required for the first provisioning release. Its custom skill roots work in this stage. Verify its concrete settings/definition formats during adapter/provisioning work; do not silently remove it from Stage 6 or claim compatibility from generic Markdown storage.

Typed agents/settings do not enter the legacy skill-only content-target adapter. Their placement and ongoing sync need compatible item adapters in the later scheduler/provisioning stages.

## Explicit folder capture

An enrolled connector can run:

```sh
reet agent library-import --config /absolute/path/agent.json \
  --folder /absolute/path/custom-skills --application claude
```

This Stage 2 inspection/import command reads known roots plus enrolled projects and repeatable explicit folders. It prints counts and bounded issue codes, never file contents or enrollment credentials. It does not install files or create destination selections.

- Known roots cover shared `.agents`, Claude, Codex and OpenCode skill directories, with `CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `OPENCODE_CONFIG_DIR` and `XDG_CONFIG_HOME` resolved locally.
- Project roots remain project-local. Custom roots use explicit absolute paths and may be labelled `shared`, `claude`, `codex`, `opencode` or `agy`.
- AGY automatic default discovery is not inferred from the name of its CLI; use an explicit verified folder for now.
- Each root has a stable local ID. The server records it only with the owning connector's account token and enrollment credential, and rejects retargeting an existing ID.
- Read-root negotiation happens before file contents are read. Relative paths, unsafe entries, symlinks, unstable snapshots and blocked skill files are refused locally or by ingestion validation.
- Complete skills retain binary supporting files and executable flags. Unchanged captures do not create another revision.
- Files from distinct same-name roots stay distinct. Failed/missing/incomplete observations never delete old canonical content. Healthy items continue, with a partial-result exit if another item fails.
- Source loss and token revocation do not make previously captured private content depend on the source machine.

This command is a staged verification surface, not another mandatory step in the final user journey. Stage 3 must integrate capture into the background connector with the agreed one-time folder consent and causal edit tracking. Do not blindly use a fresh server head as the base of an automatic offline edit: this explicit snapshot-import helper is not the latest-save conflict algorithm.

Missing files in a managed bundle remain restoration candidates under the parent contract. Do not turn an explicit snapshot-import helper into automatic permission to delete files from the canonical manifest.

A malformed application override also leaves other valid roots running. `CollectLibrarySkillLocations` can return valid locations together with a configuration error; callers must retain/process those locations. The command reports the partial configuration issue without reading an invalid relative path.

## API boundaries

| Endpoint | Purpose |
| --- | --- |
| `GET /api/v2/library` | Canonical owned projection; materialises valid authored bytes and reports item-local capture problems. |
| `GET /api/v2/library/{item}` | One owned item and its provenance/organisation/current complete content reference. |
| `PATCH /api/v2/library/{item}` | `{status, expectedVersion}` lifecycle update, preserving selection state. |
| `POST /api/v2/library/locations` | Authenticated connector registers its own root and reads current item heads. |
| `POST /api/v2/library/locations/content` | Complete registered-root skill capture with request ID and explicit expected base. |
| Existing private bundle endpoints | Typed immutable create/read/revision/history contract. |

These use the existing active-writer private-content boundary, same-origin browser writes or personal tokens, strict bounded decoding and no-store responses. Location writes additionally require the paired enrollment credential and check pause/revocation/account ownership. A request's origin cannot supply another account or arbitrary server filesystem root.

Receipt replay returns the original immutable result. Only newly committed captures change canonical machine bindings; an old receipt cannot rewind a later capture, including when a legacy capture created a different bundle. Existing preview/process-generation fences from Stage 1 remain in place.

## Validation and evidence

Local host: Debian 13.6 amd64; Go 1.27.1; Node 22.23.4; workspace tmux 3.5a. The inherited proxy/package setup and per-command child-subreaper workaround from Stage 1 are reused. No system services or real applications were installed locally.

| Check | Observed result |
| --- | --- |
| New canonical store/API/connector checks | Passed in focused runs: identity/status/metadata, actual migration/reopen, typed formats, same-name roots, exact restore with source absent, incomplete/failing captures, owner/enrollment boundaries and receipt replay. |
| Stage 1 baseline probe | The new typed API test fails on `6835558d` with HTTP 400; the same test expects successful typed capture and one canonical entry. Scratch worktree and log recorded below. |
| Broad non-race package checks | API, scan, setup, skillbundle and CLI checks passed. Initial store/backup failures identified an eager-capture fixture ordering change, temporary-copy retention and a hard-coded schema-version expectation; focused correction checks pass. |
| Web check/build | Passed; zero Svelte errors or warnings. |
| Fresh embedded-web Go race check after the final report change | Passed; package time 0.211 seconds. |
| MCP `npm test` | Passed: 16 tests, including hosted/stdio forwarding and account scope. Generated tool contract checked. |
| `go vet ./...` | Passed again after the final runtime refinements. |
| Final focused race run on Stage 2 and content-target boundaries | Passed: store 48.127 seconds, API 15.679 seconds, scan 7.418 seconds. Includes typed skill-target rejection and capture repair back to the last good bytes. |
| Full repository race suite | Passed all 44 test packages, exit 0. API 1003.190 seconds; store 410.336 seconds; backup 158.849 seconds. No reported race. |
| Supplemental final compatibility race run | Passed: backup 153.082 seconds, store 130.867 seconds, API 150.215 seconds, scan 15.786 seconds. Covers new-only backup import protection and existing capture/restore paths. |
| Invalid-override isolation under race detection | Passed: API 5.423 seconds, scan 0.593 seconds. A healthy shared root imports despite a malformed Claude override. |
| Account migration/deletion under race detection | Passed: store 4.375 seconds. Original metadata/intent survives migration and private migration notes cascade when that account is deleted. |
| Four future-stage contract probes | Four expected failures reproduced; import-restart regression passed. Package time 10.926 seconds, intentional exit 1. |

Commands from the repository root, with the workspace toolchain activated:

```sh
go test ./internal/store ./internal/api ./internal/scan ./internal/backup \
  -run '^Test(CanonicalLibrary|LibrarySkillLocations|LibraryCaptured|UnusedCapture|SyncAllMachinesSkipsASkillThatCannot|ARestoreFromANewer)' -count=1

GORACE=atexit_sleep_ms=0 go test -race ./internal/store ./internal/api ./internal/scan \
  -run '^Test(CanonicalLibrary|LibrarySkillLocations|LibraryCaptured|SkillContentTarget)' -count=1 -timeout=5m

PATH="/workspace/.reet-tools/tmux/usr/bin:$PATH" \
GORACE=atexit_sleep_ms=0 \
REET_SYNC_CONTRACT_EVIDENCE_DIR=/workspace/work/stage2-evidence \
python3 /workspace/work/stage1/run_with_reaper.py \
  go test -race -p 2 ./... -count=1 -timeout=60m

go vet ./...
npm test --prefix mcp

GORACE=atexit_sleep_ms=0 go test -race \
  ./internal/backup ./internal/store ./internal/api ./internal/scan \
  -run 'Test(CanonicalLibrary|LibrarySkillLocations|LibraryCaptured|SkillContentTarget|LibraryOnly|.*Backup|.*Restore|.*Import|.*Holdings)' \
  -count=1 -timeout=10m

go test -tags=sync_contract ./internal/api \
  -run '^TestSimpleSyncContract' -count=1 -v
```

Run `npm run check` and `npm run build` in `web/`; build fresh assets before Go's embedded-web checks. `/workspace/.reet-tools/activate.sh`, the subreaper runner and scratch logs are environment setup/evidence, not repository prerequisites. Reproduce on a normal Linux host or a container with a reaping init.

Scratch logs are `/workspace/work/stage2-*.log`; fixture evidence is `/workspace/work/stage2-evidence`. The isolated Stage 1 baseline worktree was `/workspace/work/stage2-baseline`, with a copy of `TestCanonicalLibraryTypedPrivateAPI` using only the older fixture and generic request maps. A compiler/setup error is not the expected baseline failure; the observed failure is the HTTP 400 assertion.

The broad race run was followed by targeted runs for the final compatibility refinements, rather than describing those later edits as part of its earlier build. The supplemental logs are `stage2-final-compat-race.log`, `stage2-invalid-override-race.log` and `stage2-account-migration-race.log`. The temporary baseline probe/worktree was removed after recording its result.

## What the user can test in the web UI

**Availability:** no preview/deployment URL has been created. The implementation build must be running before any browser test applies to it.

1. In the existing authoring pages, save a private skill, edit it, publish and unpublish it. Check that the item and its existing likes, notes, lists and status survive. Revisit after a controller restart.
2. With an enrolled disposable Linux connector pointed at that controller, run the import command against an explicit folder plus standard/project roots. Fetch the account-owned `/api/v2/library` JSON while signed in; check complete entries and distinct same-name captures. This endpoint is a development inspection surface, not the new Library screen.
3. Disconnect the source connector and fetch its captured revision through the private-content API. Supporting bytes remain controller-owned. Automated tests additionally restore them through the production installer and verify the executable-mode digest.

Existing backup import/restore reports also expose library/file-bundle counts. A controller with only library content now requires the existing explicit replacement choice before importing over it.

Selected-item automatic distribution is Stage 3, managed removal/restoration is Stage 4, and the new minimal web interface is Stage 5. Do not describe these API/CLI checks as a completed redesigned browser journey.

## Stage 3 entry and remaining work

1. Rebaseline the stacked PR against its merged predecessor/current main. Read the original authority map and this schema/compatibility record.
2. Materialise explicit per-machine/item/location selections, preserving old explicit intent and snapshotting whole-library subscriptions into current IDs. Keep inactive selections without deploying them. Do not subscribe to future additions.
3. Move destination scheduling to one writer with capability negotiation on authenticated HTTPS; retire the legacy whole-library rewrite for each migrated destination. Translate old desired-set, session-install, source-resolution and machine-merge actions through the same intent.
4. Reuse canonical item IDs and complete revision storage. Respect proven root/capture aliases; metadata-only legacy local names do not prove identity for a second source.
5. Integrate background discovery/capture with causal base revisions, source save time and stable edit event IDs. Test concurrent offline edits and an older save arriving last before claiming latest-save behaviour. Preserve losing versions and avoid reimport loops.
6. Retain the Stage 1 restart/preview fences and new receipt/retention protections. Extend the actual agent/settings placement adapters before sending those types to a machine.
7. Make the selected-only/inactive eligibility probes pass as their execution semantics land. Keep local-deletion restoration/notification and inactive removal explicitly assigned to Stage 4. Existing UI/API controls must allow a user to test Stage 3 before the Stage 5 redesign.

## Handover and rollback

- **Status:** complete; full repository race run and supplemental compatibility checks passed, along with web/MCP/vet checks. Stage 1 PR #662 merged as `8076f814`; PR #664 now targets main; GitHub checks on the latest push are separate from these recorded local results.
- **Scope:** Stage 2 foundation. No selected-only scheduler cutover, removal/restoration policy, four-app provisioning, independent backup rollout or production deployment.
- **Runtime:** canonical library and locations store/API; typed private content; authored private binding; shelf status/read adapters; explicit connector import; existing setup policy validation.
- **Migration/activation:** automatic additive `store.Open` migration, schema format 2. New data APIs/explicit capture are available; destination policy remains legacy.
- **Rollback:** stop new ingestion, retain a consistent full database copy and all post-migration content/aliases. Prefer a forward-compatible fix. A Stage 1 binary cannot fully read typed content, and an old restore must refuse schema-2 archives. Do not automatically overwrite newer content with an old snapshot or reactivate a whole-library writer as a migration rollback.
- **Field limits:** local subprocesses/HTTP/SQLite and temporary files, not two physical machines or a new production Tailcat diagnosis. Stage 1's field evidence gap remains. No application sign-in/provider calls were made.
- **Next:** Stage 3. Merge the green Stage 2 PR, then implement explicit item/location selections and the single scheduler cutover. Keep status versions, content expected-base revisions and destination intent generations distinct.

### PR check follow-up: library list performance

The benchmark check on `ee7f2c5e` reported statistically significant regressions above its 15% budget for compact skill lists and paged API reads. The added identity/status metadata query scanned three extra values for every shelf item. Identity and status now come from the existing indexed shelf read, as one internal joined value; all public fields and canonical status filtering are unchanged. No benchmark threshold or fixture was relaxed.

Focused store lifecycle/list/migration race tests passed (92.633s). Local diagnostic runs under concurrent Stage 3 tests reduced allocations from 9,842 to 8,613 per full list and 7,962 to 6,733 per compact list. Timing samples on the shared executor were noisy, so the GitHub paired benchmark remains the acceptance gate. The Codex review service reported its account usage limit; there are no inline review threads to resolve.

The next paired GitHub run still found excessive paged-read cost: joining identity before `LIMIT` made SQLite look up canonical metadata across the whole shelf. The follow-up limits/materializes the requested page before those joins, retains the requested outer ordering, and adds covering indexes for the alias/status projection. Unpaged reads and single-item reads also use the covering indexes. The 15% gate and its workloads remain unchanged. Focused lifecycle/list/status race tests passed (14.221s); the final page-order checks and GitHub paired measurements remain separate evidence.

The paired run on `87d23448` still exceeded the budget by 17–23%. An actual SQLite query-plan inspection showed non-covering unique-index lookups; explicitly forcing the covering indexes did not improve local timings, so that experiment was discarded. The list reader now reuses SQL destinations across rows, including tag metadata. Returned rows remain independent value copies with fresh trend slices. A regression test covers distinct identities, flags, tags and trends across full, paged and compact results, including a missing trend between populated trends. Initial local list measurements reduced allocations from 8,615 to 7,418 and compact allocations from 6,735 to 5,538 before the additional tag-buffer reuse. Focused API list tests pass (4.017s). Final focused store race tests pass (63.686s), store vet passes, and final allocations are 6,821 full / 4,941 compact. The GitHub paired benchmark remains required before merge; the performance limit and test fixtures are unchanged.

A further local comparison found paged API medians still 16–19% above the Stage 1 samples. The next correction adds an idempotent account/recent-time shelf index so selecting a page does not first sort the complete shelf. Its prototype brought page medians to roughly 3–5% above those samples; full-list timing remained variable, so this is diagnostic evidence rather than a paired CI acceptance pass. Canonical joins explicitly use the existing covering indexes, and visible-tag scans omit the always-zero hidden column. The separate-field and forced-primary-index experiments were discarded. Final list/lifecycle/migration/tag race tests pass (42.790s), API list tests pass (2.297s), and store vet passes. The unchanged GitHub 15% paired benchmark remains mandatory before merging this head.
