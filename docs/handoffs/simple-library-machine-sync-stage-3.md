# Stage 3: selected-item automatic sync — validation in progress

Parent: [product contract](simple-library-machine-sync.md). Dependency: [Stage 2](simple-library-machine-sync-stage-2.md), PR #664. Draft PR: [#665](https://github.com/eyupio/reet.sh/pull/665).

**Status: implementation and affected-package local validation complete; current-head CI and PR integration remain pending. Do not advance to Stage 4 yet.** No preview or production deployment has been created. The production Tailcat incident is still unverified.

## Implemented process

New connectors negotiate protocol 1 over the authenticated controller HTTP channel before reconciliation. This freezes existing library membership once and prevents new machines/folders or later library additions from becoming implicit subscriptions. Existing custom policies are retained. Legacy compatibility is tested with an explicitly simulated older connector; supported new connectors exercise normal negotiation, without a prototype flag.

On the existing `/sync` page, **Skills to sync** lets the user choose an enrolled machine folder, select canonical library items, select all current In use skills, and save. Saving intent requires neither connectivity nor a complete inventory. Inactive selections and pending captures remain recorded; only ready In use skills enter the additive execution plan. Clearing a selection stops updates in this stage; managed removal follows in Stage 4.

Canonical identity survives authored renames and registered-root capture. Install observations reuse the proven canonical identity instead of manufacturing another same-name shelf item. Normal shelf metadata, authoring, likes and lists are preserved.

## Protocol and state

- `GET /api/v2/library` and item/status routes remain the Stage 2 projection.
- `GET /api/v2/library/selections/{target}` and `PUT /api/v2/library/selections` read/save `{targetId,itemIds,version}`. Version is a compare-and-swap token; an empty list is explicit empty intent. Invalid/foreign/deleted items are refused.
- `POST /api/v2/library/protocol` requires the enrollment secret in addition to account access, and returns `{schemaVersion:1,serverTime}`. Poll requests must advertise `X-Reet-Library-Protocol: 1` to collect canonical jobs. An older connector cannot collect those jobs.
- `POST /api/v2/library/bases` pins `{bindingId,itemId,revisionId}` for an authenticated selected destination or registered source. The binding ID is a path hash, not a controller filesystem instruction. Pins prevent quota reclamation from deleting the base of an offline edit.
- `POST /api/v2/library/edits` accepts a durable `{eventId,itemId,base,savedAt,uncertainty,files}`. `GET /api/v2/library-history/{item}` returns event receipts and ordering evidence; immutable private revision routes retain the contents.
- New tables: canonical selections, account/legacy selection snapshots, protocol capability receipts, edit events, installed/source bases, and authored-editor digests. All content and intent remain account-owned. Enrollment revocation stops edits and releases that enrollment's base pins.

Database format is **3**. An older controller must reject this database/backup rather than reopen it and reinstate whole-library policies it does not understand. Back up before upgrading; rollback means restore a compatible pre-upgrade backup or deploy a format-3 reader. Do not delete selection rows to reactivate broad subscriptions.

## Machine edits and ordering

The connector watches only complete content explicitly imported with local consent or installed from a canonical private revision. It does not indiscriminately upload folders. Existing `AllowApply` consent gates background content capture. Explicit `reet agent library-import` registers source receipts; app overrides, custom roots and projects retain Stage 2 capture rules.

The private `<config>.library` journal retains exact bytes, source base, event ID and save time before any network request. It uses atomic private writes and a crash-released enrollment lock, so a daemon and an explicit import cannot replace each other's journal. Queue limits are 256 events / 64 MiB; a full/failed capture leaves changed content protected from overwrite. A missing managed file is never a central deletion. Removal/restoration and notifications are Stage 4.

Save time is the latest filesystem modification time across the captured skill's files, adjusted by the last controller clock calibration. Calibration records half the HTTP round trip as uncertainty; uncertainty grows at 100 ppm while offline. Supporting bytes and executable flags are retained. Editor bytes have a separate digest so projecting a machine edit into the authored editor cannot clear executable modes or create an import loop.

Ordering is a logical save time, followed by a deterministic enrollment/event hash. A source time later than its causal base is used when the calibration is usable. A time more than five minutes ahead of the controller, more than five minutes behind its base, or with more than 30 seconds uncertainty falls back to **base logical time + 1**, never arrival time. Any descendant is at least base + 1. Ties use the stable event hash. This converges automatically without a blocking conflict workflow; it cannot establish perfect real-world ordering for an incorrect clock or a tool that preserves old mtimes. Original save time, uncertainty, fallback and causal base remain inspectable.

An identical event retry returns the original receipt without moving any head. Reusing an ID with different content/time/base fails. Concurrent writes append immutable revisions; a losing revision is retained while the canonical and bundle heads stay on the winner. Edit revisions, their bases, and offline installed/source bases are protected from reclamation. Existing account storage bounds still apply (128 bundles, 256 revisions, 64 MiB); hitting the bound preserves local work and does not block other items/destinations. History is not an independent backup.

## Execution and failure isolation

One existing sync installer remains responsible for all filesystem writes. Canonical selections project immutable private sources into additive jobs, with item IDs attached to the legacy adapter contract. No second sync writer was introduced.

Selection/status/content changes invalidate pending target generations. The connector rechecks intent and the exact local plan before applying. A queued or unuploaded local edit excludes only that item from overwrite. Canonical additive planning skips unreadable/unsafe individual entries, retains per-item problems in the plan and still applies healthy content. A separate automatic canonical job does not cancel another scope's manual review. Failed/partial scopes back off for a minute, and another destination can proceed.

Existing stale previews, enrollment checks, local consent, checksums, filesystem locks and backup-before-replacement checks remain active. This stage does not implement managed deselection/status removal, deletion restoration or their notifications.

## Validation ledger

Passed before the final refinements:

- `TestCanonicalSelectionOfflineRenameAndNewMachine`: offline/stale inventory intent, compare-and-swap refusal, A/B installation, rename preserving selected ID and C excluded.
- `TestCanonicalSelectedMachineEditsAndOfflineOrdering`: A edit, central edit, offline B edit, process/controller restart, older save arriving last, retained losing bytes and no upload loop.
- Registered shared/custom root source edits and blocked-neighbour isolation exposed duplicate catalog identities; the canonical observation binding fixes that issue. The subsequent complete canonical focused run passed (10.622s, non-race).
- Web check (zero errors/warnings), production build and Playwright at 390/1280px passed. Browser test saves explicit IDs for an offline/incomplete destination, excludes To test from select-all, retains pending captures and checks no overflow/runtime errors.

Still running / must record before completion:

- Broad Go race run, then final affected-package checks after schema/base-pin/clock/locking refinements. Do not describe an earlier compiled snapshot as the final build.
- Final canonical race acceptance, including supported private adapters, clock fallback, event replay/enrollment fences and actual foreground normal/reconnect latency.
- Final vet/MCP checks, generated contracts and docs/link checks; refresh web verification if the component changes.
- Integrate Stage 2's latest performance fix and public-doc link correction; maintain stacked PR bases as parents merge.
- Promote selected-only/initial-inactive contracts only when normal negotiated operation passes. Stage 4 probes must explicitly select initial fixtures before testing removal/restoration.

Reproducible focused commands:

```sh
go test -race ./internal/api -run 'TestCanonical|TestSimpleSyncContractSelectedItemsOnly|TestSimpleSyncContractInactiveAuthored' -count=1 -v
go test -race ./internal/store -run 'TestCanonical|TestPrivateStorageReclaimCoversEveryReference' -count=1
npm run check --prefix web
npm run build --prefix web
# Start npm run preview --prefix web -- --host 127.0.0.1 --port 4173, then:
node web/tests/selected-library.e2e.mjs
npm test --prefix mcp
go vet ./...
```

The real controller/subprocess harness runs without production machines, external provider credentials or actual Tailcat access. Its compatibility fixtures deliberately remove the new capability; canonical fixtures use the new production negotiation. Controller tests are evidence for HTTP recovery, not a claim that production Tailcat reconnect is fixed.

## Next work after Stage 3 validation

Stage 4 must add acknowledged ownership/removal receipts and generations, remove only reet-managed copies on deselection/inactivation, restore accidental local deletion, and emit factual notifications. Extend the current installed/source base pins and journal rather than adding a competing writer. Stage 5 replaces the remaining legacy pages; Stage 6 provisions Linux apps; Stage 7 provides independent backup/restore drills.

## PR supervision

The owner authorized fixing checks, review comments and conflicts across all open PRs, and merging completed green work. Stage 1 #662 merged as `8076f814` with all ten workflows green; #664 was retargeted to main. Its real list regression required further buffer reuse in `46607962`; the new paired benchmark is pending, with its 15% gate unchanged. #665 remains draft pending final validation. Native checks on #666/#668 passed with the longer cold-build budget, but #666 then needed its workflow comments aligned with main (`b4302a96`) and #668's author updated its branch (`b6fe31f6`); current-head checks must finish before either merges. There were no inline review threads at that checkpoint. The Codex review service reported its account usage limit on #664; no successful final automated review is claimed.

### Validation follow-up (still in progress)

The first broad Go run was stopped: source changed while Go was compiling dependent packages, producing a mixed-snapshot `LibraryBase` compile error. It is not a completed regression result and must be replaced with a clean build of the final revision. The first expanded race acceptance run passed source capture, selection/rename, offline edits, root-edit isolation and clock/replay fences, but failed its adapter fixture because the app folders had never been created/scanned before selection. Creating the actual supported folders fixes that fixture; the non-race adapter rerun and new consent/deletion checks pass (API 3.272s, scan 0.180s).

Local consent is now reread without restarting the daemon. Protocol negotiation happens even in scanner-only mode, preventing the scanner's old default policies from silently selecting the whole library; capture and uploads still require local consent. The WebUI retains the form after a save failure and offers a reload after stale/conflicting reads. Two Stage 3 contract checks have moved out of the future-stage tag, and Stage 4 fixtures explicitly select the skill before probing removal/restoration.

MCP's 16 tests passed, and the final web check reports zero errors/warnings. A preliminary foreground timing was approximately 11 seconds; the reconnect test has been strengthened to prove both a destination outage and a completed source upload before starting its reconnect measurement. Record the final measured results, not the preliminary observation, before completion.

### Final local acceptance and CI corrections

The final focused race acceptance passed: API 165.086s, scan 1.483s and store 9.632s. Normal foreground propagation measured **10.500s**, and catch-up after a proven destination outage and completed source upload measured **6.301s**, with the production five-second polling loop. The backup package had no tests matching that focused regex; it is covered separately by the broad run, not counted as a focused pass.

The immutable broad run at `b55dd1eb` completed with **43 passing packages**. Its API package had one failure: the pre-existing `TestLibrarySyncNewMachinesDraftEditsAndReconnect` still expected unselected/newly authored skills to install automatically. This failure was also present in GitHub's image check. The test now exercises the supported selected-only process: initial empty machines, explicit current selections, later additions excluded until chosen, exact private imports, reconnect and no repeated jobs. Its updated race run passes (18.955s). This is not described as a clean full-suite run of a later revision; the latest GitHub complete suite is still required.

Additional fixes proved while completing that flow:

- Metadata reported before the first capture reuses its original canonical identity/status only when one recorded source proves exactly that registered standard folder. Multi-source/name-only claims are not merged. The scan-before-import regression and metadata projection checks pass (48.387s and 64.825s race runs); the larger canonical follow-up passed API 274.672s and store 12.135s before the final small metadata assertion.
- An explicit import skips receipted managed sources/copies: their edits use the causal journal, so importing a machine that already received a skill cannot manufacture another item or rebase an unsent edit on a fresh server head.
- A machine edit updates the authored description alongside its exact editor bytes and executable-mode binding.
- GitHub's browser failure identified a real empty-page regression: the selection panel pushed the first-machine CTA below the phone viewport. It now appears only when an enrolled folder exists. Shared browser fixtures supply the new library endpoint. The existing empty-state CTA checks and phone zoom checks pass at 320/360/390/412px; selected-library tests also pass at 320/390/1280px with a 64-character unbroken name and actual mobile contexts.
- Public controller-sync docs now lead with the selected-library process and identify advanced/older connector behaviour separately. Rendered docs checks pass for 51 pages; handoff links exist. CLI reference comparison is unchanged, web check/build and MCP's 16 tests pass.

These fixes are concrete reviewable changes for #665's image/browser failures. Keep the PR draft and Stage 3 unchecked until the new-head CI checks pass. Stage 2's second performance correction materializes the requested page before canonical joins and adds covering indexes; it preserves ordering and does not relax the 15% benchmark gate. All open PRs continue to be monitored; #668 was added during this work and is included in that inventory.

### CI integration checkpoint

The integrated race check after Stage 2's page-query merge passed API 198.599s, store 40.720s, scan 7.748s and embedded web 1.075s; vet and rendered-doc checks passed. The next browser run exposed a second CTA regression on an already connected phone: placing selection before the existing primary sync flow pushed that flow's button below the viewport. Selection now follows that primary flow. The existing shared-library light/dark phone/desktop test, explicit selection test (320/390/1280), first-machine empty-state checks and four-width phone zoom checks pass together. The explicit selection browser test is now part of the terminal-browser CI workflow.

A native acceptance job failed during Go setup, before its tests started, with no downloadable log blob. This is recorded as infrastructure failure, not a passing product check; the new commit retries it. No test assertion, benchmark threshold or per-test deadline was weakened.

PR #667 initially failed automatic approval review because its description required owner review. The owner subsequently explicitly approved merging the plan. The final head `2b3e6911` contained a reviewed clarification about preserving raw capabilities; the documentation merged as `c39e93d2`. This does not authorize final runtime activation: the plan's signed-in acceptance run remains its own release gate.

### Existing-copy upgrade and honest partial receipts

Selecting a directory that already contains exact canonical bytes now runs one reconciliation even if nothing needs installing. With local apply consent, it pins the revision and records the managed file paths without rewriting the directory. A changed pre-upgrade private copy can be adopted only against the controller's previous successful apply baseline, verified against an immutable account-owned revision. Its edits upload in the next cycle; the original baseline paths prevent local file absence becoming central deletion. Without that proof, the item is retained for explicit import while healthy items proceed. Existing legacy policies remain frozen snapshots until explicitly replaced by canonical selection; no arbitrary same-name source is inferred.

Previous apply baselines are protected from private revision reclamation, including when an offline machine has not yet established its new local journal. Partial/no-op plans containing retained-item problems no longer advance the whole destination's applied revision or replace its last complete baseline. They keep per-item problems and the existing one-minute retry backoff. This avoids reporting blocked work as "Already in sync".

The new real-controller/connector upgrade tests pass (2.705s, non-race), covering matching copies, edits made before and after cutover, and an unknown local copy alongside a healthy selected item. Focused partial-receipt/baseline-retention race tests pass (4.732s), and affected-package vet passes. The larger canonical race run passed: API 197.227s, scan 6.564s, store 11.690s. A stronger upgrade race test also passed (50.916s): central content changes while the machine is offline, then its later local edit uploads against the original applied base rather than a manufactured current base. The final partial-result message check passed (4.325s race); the full scanner package passed (2.580s), and rendered docs pass (51 pages). Current-head CI and PR integration remain required before marking Stage 3 complete.

### Latest query integration

Stage 2's further query correction is `40f47ca4`: an account/recent-time shelf index for paged reads, explicit covering canonical lookups, and removal of a constant hidden-tag scan column. Its final focused race tests pass (42.790s), API list tests pass (2.297s), and vet passes. Local page samples improved, but full-list timing remains variable and the unchanged 15% paired CI gate still must pass. #665 includes this correction; it is not a new distribution/protocol change. #668 merged as `1c19ddf8` with its full current-head checks green; #666 is also merged. An hourly automation now checks all open reet PRs, fixes genuine blockers and merges completed green work under the existing owner authorization. Routine pending states are suppressed; explicit owner activation gates remain enforced.

Integrated canonical API checks after this query merge pass (34.051s, non-race). Rendered documentation checks still pass for 51 pages. Current-head GitHub CI remains the merge gate.
