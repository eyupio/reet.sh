# Ownership Receipts and Adoption Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an account record which files reet manages on a machine (ownership receipts) and bring an existing skill folder under management by an explicit, preview-bound adoption, with no machine writes.

**Architecture:** A new `skill_content_receipts` table (one current row per account, environment and skill slot) feeds the existing pure planner as its `Baseline`. Adoption is a create-only store transaction that recomputes the claimable file subset server-side from a captured preview and a library revision. Planner baseline mismatches become blocked plans; previews bind the receipt version so adopting or releasing supersedes old reviews.

**Tech Stack:** Go 1.27.1 (stdlib `net/http`, `database/sql`, pure-Go SQLite), Svelte/TypeScript only for two Activity labels. No new dependency.

**Spec:** `docs/superpowers/specs/2026-10-24-ownership-receipts-adoption-design.md` (approved). Read it first; this plan only decides what the spec leaves open.

**Execution note:** this work continues on branch `claude/gallant-darwin-n9oqfj`, so its commits join draft PR #173, which currently carries only the spec and this plan. Retitle that PR and rewrite its body in Task 8. Merge only if the user has authorised it for this PR.

## Global Constraints

- Go 1.27.1 and Node 22; no new dependency; stdlib and existing packages only.
- Planner `Version` stays `1`. Baseline target or bundle mismatch becomes a **blocked plan** (`baseline_target_mismatch`, `baseline_bundle_mismatch`); an `OwnerID` mismatch, an invalid baseline revision ID and an invalid baseline manifest stay **errors**.
- Receipt `provenance` is `"adopted"`; API `verification` is `"controller_attested"`, never "verified on disk". At most 256 managed files per receipt and 1,024 receipts per account.
- Adoption is **create-only**; release uses compare-and-set on `version`. Replace-by-version is not offered.
- Every mutating transaction's first statement takes the account write lock (`privateSkillOwner`); never start with a read.
- Strict JSON (unknown fields and trailing data rejected), 16 KiB request limit, no query parameters, accounts mode and active writer only, same-origin browser write or owning personal token, `Cache-Control: no-store`. The MCP allowlist (`mcpAPIPath`) must stay closed to every new route.
- Responses and audit events carry metadata only: no file bytes, no absolute paths.
- Nothing writes to a machine; terminal sessions and controls are untouched.
- Error text is plain and direct (British spelling, no reassurance around failures). Dialect belongs only in documentation. Name no competing product anywhere.
- Commits: imperative plain sentence, no prefix, each buildable, ending with the two trailers `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01PEfyhZfyBpPYYrRYh8Jy4q`.
- Validation order before any claim of success: `(cd web && npm run check && npm run build)`, then `go vet ./...`, `go test -race ./...` with tmux on PATH, `(cd mcp && npm ci && npm test)`, `git diff --check`.

## Review Focus

Inputs the spec implies that a person will meet, each pinned by a named test below.

1. A file whose bytes match the revision but whose **executable bit differs** must not be claimed (Task 2, Task 5).
2. **`managed` mode** with an obsolete adopted file removes only claimed files and leaves an unclaimed look-alike alone (Task 4).
3. **Release then identical re-adopt** must not revive an older review (Task 4, Task 5).
4. A skill **deleted after adoption** blocks as `managed_target_missing` rather than silently reinstalling; release clears it (Task 4).
5. **Two simultaneous adoptions** of one slot leave exactly one receipt (Task 5).

---

## File Structure

| File | Responsibility |
| --- | --- |
| `internal/skillplan/plan.go` (modify) | Baseline mismatch blocks |
| `internal/skillplan/claim.go` (create) | Pure claim and subset computation |
| `internal/store/store_content_receipts.go` (create) | Receipt table, reads, release, baseline lookup, adoption, rename move |
| `internal/store/store_content_previews.go` (modify) | Baseline from receipt, `receiptVersion`, `receipt_changed`, `adoption` offer |
| `internal/store/store_machines.go`, `store_tidy.go` (modify) | Rename move, merge fence |
| `internal/store/store.go` (modify) | Migration call |
| `internal/api/content_receipts.go` (create) | Routes, handlers, audit events |
| `internal/api/content_previews.go` (modify) | Share the strict decoder |
| `web/src/lib/events.ts` (modify) + `events.test.mjs` (create) | Two Activity labels |
| `docs/skill-content-receipts.md` (create) and four existing docs, `README.md`, `handoff.md` | Contract and handoff |

Test fixtures already available in `internal/store` tests: `previewFixture(t) previewEnv` (fields `s`, `uid`, `agent`, `write`, `bundle`, `pin`; methods `create`, `claim`, `report`, `state`, `edit`), `exclusiveAbsent()`, `presentOf(m skillbundle.Manifest, ownership string) skillpreview.Capture`, `unknownCapture(ownership, issues...)`, `conflictCodes(p)`, `privateWrite(key, base, text)` (three files: `SKILL.md`, `scripts/run.sh` executable, `assets/empty`), `sp(s)`. In `internal/api` tests: `newPreviewHarness(t)` with `browser`, `connector`, `negotiate`, `createBody`, `absentReport`, and `privateSkillHTTP`.

---

### Task 1: Planner blocks mismatched baselines

**Files:**
- Modify: `internal/skillplan/plan.go` (baseline validation near `Build`'s start; new checks after the `observation_stale` block)
- Test: `internal/skillplan/plan_test.go`

**Interfaces:**
- Produces: conflict codes `baseline_target_mismatch` and `baseline_bundle_mismatch` (both `Conflict{Path: ".", Code}`), emitted after `observation_stale` and before any presence logic, target before bundle. `Build(Snapshot, int64) (Plan, error)` is unchanged.

- [ ] **Step 1: Write the failing tests.** In `TestFailClosedContext` add rows: `baseline root moved` and `baseline generation` (mutating `s.Baseline.Target.RootID = "elsewhere"` / `.Generation = "old"`) and `baseline other connector` (`.AgentID = "agent-2"`) expecting `baseline_target_mismatch`; `baseline other bundle` (`s.Baseline.BundleID = strings.Repeat("d", 32)`) expecting `baseline_bundle_mismatch`. Remove `wrong bundle`, `baseline root`, `baseline generation` from `TestRejectInvalidInputs`; keep `baseline owner`, `baseline revision`, `baseline metadata` there. Add `TestBaselineMismatchPrecedence`: shared root plus root mismatch yields `root_shared`; stale observation plus mismatch yields `observation_stale`; target and bundle both wrong yields only `baseline_target_mismatch`. Add `TestBaselineMismatchPlanIsDiagnosticAndDeterministic`: two builds of the same snapshot (second with baseline files reordered) give equal `Hash`, `Preconditions.Baseline != nil`, and no actions.
- [ ] **Step 2: Run** `go test ./internal/skillplan/ -run 'FailClosedContext|BaselineMismatch|RejectInvalidInputs' -v`. Expected: FAIL (the mismatch cases return an error instead of a blocked plan).
- [ ] **Step 3: Implement** in `plan.go`: keep returning errors for owner mismatch, invalid revision pattern and non-canonical baseline manifest; drop the combined target/bundle error; after the `observation_stale` check add `if b := s.Baseline; b != nil { if b.Target != s.Target { return block("baseline_target_mismatch") }; if b.BundleID != s.Desired.BundleID { return block("baseline_bundle_mismatch") } }`.
- [ ] **Step 4: Run** `go test -race ./internal/skillplan/`. Expected: PASS, including every untouched existing test.
- [ ] **Step 5: Commit** `Block planner baselines that do not fit the target or bundle`.

### Task 2: Pure claim computation

**Files:**
- Create: `internal/skillplan/claim.go`, `internal/skillplan/claim_test.go`

**Interfaces:**
- Produces:
  - `var ErrNotClaimable = errors.New("revision does not match the observed SKILL.md")`
  - `type ClaimCounts struct{ Matched, ObservedOnly, RevisionOnly int }`
  - `func ClaimSubset(observed, revision skillbundle.Manifest) (skillbundle.Manifest, ClaimCounts, error)`: paths present in both with equal `skillbundle.File` (path, bytes, digest, executable); both inputs must pass the package's `canonical` check; `SKILL.md` must be matched, else `ErrNotClaimable`; result is `skillbundle.FromMetadata` of the matched files. `ObservedOnly = len(observed.Files) - Matched`, `RevisionOnly = len(revision.Files) - Matched`.
  - `func IsSubset(managed, revision skillbundle.Manifest) bool`: every managed file equals the revision's file at that path.

- [ ] **Step 1: Write failing tests** (use the file's own `manifest` style helpers): `TestClaimSubsetIdentical` (managed digest equals revision digest, counts `{N,0,0}`); `TestClaimSubsetOlderRevisionWithLocalExtras` (an observed `notes/mine.txt` is absent from the result, `ObservedOnly == 1`); `TestClaimSubsetNeverClaimsDifferingBytesOrMode` (same path, different digest, and same bytes with the executable bit flipped, are both unclaimed and counted in both `ObservedOnly` and `RevisionOnly`); `TestClaimSubsetRequiresSkillMd` (differing `SKILL.md`, or no overlap, gives `errors.Is(err, ErrNotClaimable)`); `TestClaimSubsetRejectsInvalidManifests` (incomplete manifest or forged digest gives an error that is **not** `ErrNotClaimable`); `TestClaimSubsetIsOrderIndependent`; `TestIsSubset` (true for an exact subset; false for an extra path, a different digest and a different mode).
- [ ] **Step 2: Run** `go test ./internal/skillplan/ -run 'ClaimSubset|IsSubset'`. Expected: FAIL, `undefined: ClaimSubset`.
- [ ] **Step 3: Implement** the three declarations using `fileMap` and `canonical` from `plan.go`; no new dependency.
- [ ] **Step 4: Run** `go test -race ./internal/skillplan/`. Expected: PASS.
- [ ] **Step 5: Commit** `Compute the files a library revision can prove it owns`.

### Task 3: Receipt storage, reads and release

**Files:**
- Create: `internal/store/store_content_receipts.go`, `internal/store/store_content_receipts_test.go`
- Modify: `internal/store/store.go` (call `st.migrateSkillContentReceipts()` after `migrateSkillContentPreviews`)

**Interfaces:**
- Consumes: `skillplan.IsSubset`, `privateSkillOwner`, `privateSkillRevision(q, uid, bundleID, revisionID, false)`, `previewQuerier`, `syncID()`, `contentobs.ValidID`.
- Produces:
  - `const MaxSkillContentReceipts = 1024`; errors `ErrSkillContentReceiptInvalid`, `ErrSkillContentReceiptMissing`, `ErrSkillContentReceipt` (any 409: stale, blocked, changed, release first, version conflict), `ErrSkillContentReceiptCapacity`.
  - `type SkillContentReceiptEvidence struct{ PreviewID, ReviewHash, CaptureHash, ObservedDigest, PinID, PinVersion string; CapturedAt, AdoptedAt, AdoptedBy int64 }` (camelCase JSON).
  - `type SkillContentReceipt struct{ ID, EnvironmentID, AgentID, Name, BundleID, RevisionID, ManagedDigest string; FileCount int; Managed *skillbundle.Manifest; Provenance, Verification string; Evidence SkillContentReceiptEvidence; Version string; CreatedAt, UpdatedAt int64 }` (`Managed` omitted from summaries).
  - `func (s *Store) migrateSkillContentReceipts() error` creating the table exactly as in the spec's Data model (column `schema_version`, `UNIQUE(user_id, environment_id, name)`, `UNIQUE(user_id, request_key)`, the two foreign keys with `ON DELETE CASCADE`).
  - `func (s *Store) SkillContentReceiptsFor(uid int64) ([]SkillContentReceipt, error)`: summaries ordered by `environment_id, name, id`; `Managed` nil.
  - `func (s *Store) SkillContentReceiptFor(uid int64, id string) (SkillContentReceipt, error)`: fills `Managed`; verifies it with `skillplan.IsSubset` against the recorded revision, else returns an error that is none of the sentinels (stored data failed integrity validation).
  - `func (s *Store) ReleaseSkillContentReceiptFor(uid int64, id, version string) error`.
  - `func skillContentReceiptBaseline(q privateSkillQuery, uid int64, environmentID, name string) (*skillplan.Baseline, string, error)` (it needs `privateSkillQuery`, not `previewQuerier`, because `privateSkillRevision` also calls `Query`): `(nil, "", nil)` when none; otherwise the `Baseline` whose `Target` is rebuilt from the row (`OwnerID uid`, `AgentID`, `EnvironmentID`, `Generation = AgentID`, `RootID`, `SkillName`, `Adapter`, `Schema = schema_version`) plus the receipt version; an integrity failure is an error.
  - `func currentReceiptVersion(q previewQuerier, uid int64, environmentID, name string) (string, error)`: `""` when none.
- Test helper (test file only): `seedReceipt(t, e previewEnv, revisionID string, managed skillbundle.Manifest, rootID string) SkillContentReceipt` inserting a row directly with provenance `adopted`.

- [ ] **Step 1: Write failing tests:** `TestReceiptReadsAreMetadataOnlyAndOwnerScoped` (list has `Managed == nil`, item has it; JSON of both has no `"data"` key; a foreign account and a viewer get `ErrSkillContentReceiptMissing`); `TestReceiptIntegrityOnRead` (tampering `managed` with an extra path, then with a changed digest, makes `SkillContentReceiptFor` fail while a clean row passes); `TestReleaseRequiresCurrentVersion` (wrong version gives `ErrSkillContentReceipt` and keeps the row; the right one deletes it and leaves the pin and private revision intact; foreign and viewer give Missing); `TestReceiptCascades` (revoking the enrolment via `ControlSyncAgent(uid, id, "revoke")` and deleting the environment each remove the row; removing the pin does not); `TestReceiptBaselineLookup` (`Target` fields equal the row, `Generation == AgentID`, version returned, nil when none, error when damaged); `TestReceiptMigrationIsIdempotentAcrossReopen`.
- [ ] **Step 2: Run** `go test ./internal/store/ -run 'Receipt'`. Expected: FAIL (does not compile).
- [ ] **Step 3: Implement** the declarations above in `store_content_receipts.go`; the `managed` column holds the canonical manifest JSON; reuse the pin store's patterns for CAS and ordering.
- [ ] **Step 4: Run** `go test -race ./internal/store/ -run 'Receipt'`. Expected: PASS.
- [ ] **Step 5: Commit** `Store ownership receipts separately from pins and previews`.

### Task 4: Previews use receipts as the planner baseline

**Files:**
- Modify: `internal/store/store_content_previews.go`
- Test: `internal/store/store_content_previews_test.go`

**Interfaces:**
- Consumes: `skillContentReceiptBaseline`, `currentReceiptVersion`, planner codes from Task 1.
- Produces: `SkillContentPreview.ReceiptVersion string` (`json:"receiptVersion,omitempty"`), set in `RecordSkillContentCaptureFor` from the receipt read in the building transaction; `skillContentReviewIdentity.ReceiptVersion string` (`json:"receiptVersion"`) included in `Hash()`; fence reason `receipt_changed` in `previewFence`, applied only when `p.CapturedAt > 0` and the slot's current receipt version differs from `p.ReceiptVersion`; `buildPreviewPlan` becomes `func buildPreviewPlan(tx *sql.Tx, uid int64, p SkillContentPreview, c skillpreview.Capture, received int64) (skillplan.Plan, string, error)`, passes the baseline to `skillplan.Build` and returns the receipt version (second value) alongside the plan. A damaged receipt makes the report end the preview as `failed` with `plan_unavailable` (the existing planner-error path).

- [ ] **Step 1: Write failing tests:** extend `TestSkillContentPreviewReviewIdentityBindsThePinVersion` with a `receipt` case; then (each builds on `previewFixture`, seeding receipts with `seedReceipt` and observed captures with `presentOf`):
  - `TestPreviewUsesReceiptAsBaseline`: receipt claims all of R1, pin is R1, observed is R1 plus an extra local file: plan `Ready`, zero actions, `ExpectedManaged.Digest` equals R1's, extra file in `Preserved` as `local_only`, `ReceiptVersion` equals the receipt's.
  - `TestPreviewUpgradePlanFromOlderAdoptedRevision`: receipt claims R1, pin moved to R2 (only `SKILL.md` differs): exactly one `update` action on `SKILL.md`.
  - `TestManagedModeRemovesOnlyClaimedObsoleteFiles` (Review Focus 2): R2 drops `scripts/run.sh`, pin mode `managed`, an unclaimed `notes/mine.txt` is observed: one `remove` of `scripts/run.sh`; `notes/mine.txt` in `Preserved`.
  - `TestPreviewBlocksReceiptThatNoLongerFits`: pin changed to a second bundle with the same skill name gives `baseline_bundle_mismatch`; capture with a different `RootID` gives `baseline_target_mismatch`.
  - `TestDeletedSkillWithReceiptBlocksUntilReleased` (Review Focus 4): absent capture gives `managed_target_missing`; after `ReleaseSkillContentReceiptFor` a new absent preview is `Ready` with create actions.
  - `TestDamagedReceiptEndsPreviewAsPlanUnavailable`.
  - `TestReceiptChangeSupersedesCapturedPreview` (Review Focus 3): release, and separately release then `seedReceipt` identical content, each make the earlier captured preview `superseded` with reason `receipt_changed`; a receipt appearing after a no-receipt preview does the same.
- [ ] **Step 2: Run** `go test ./internal/store/ -run 'SkillContentPreview|Preview'`. Expected: FAIL.
- [ ] **Step 3: Implement** the changes above; pass `Baseline` only when the lookup returns one and let the planner decide mismatches.
- [ ] **Step 4: Run** `go test -race ./internal/store/ -run 'SkillContentPreview|Preview|Receipt'`. Expected: PASS, with every earlier preview test unchanged.
- [ ] **Step 5: Commit** `Plan previews from ownership receipts and fence on receipt changes`.

### Task 5: Adoption and the adoption offer

**Files:**
- Modify: `internal/store/store_content_receipts.go`, `internal/store/store_content_previews.go`
- Test: `internal/store/store_content_receipts_test.go`

**Interfaces:**
- Consumes: `skillplan.ClaimSubset`, `effectivePreview`, `readPreview`, `reportedPinScope`, Task 3 and 4 helpers.
- Produces:
  - `type SkillContentAdopt struct{ RequestKey, PreviewID, ReviewHash, AdoptAsRevisionID string }` (JSON `requestKey`, `previewId`, `reviewHash`, `adoptAsRevisionId`).
  - `func (s *Store) AdoptSkillContentPreviewFor(uid int64, in SkillContentAdopt) (SkillContentReceipt, error)` implementing spec Adoption steps 1 to 8 in that order inside one transaction. Invalid shapes (request key pattern, 48-hex preview ID, 64-hex hash, 32-hex revision ID) give `ErrSkillContentReceiptInvalid`; unknown or foreign preview gives `ErrSkillContentPreviewMissing`; unknown or foreign revision gives `ErrPrivateSkillMissing`; a revision whose `SKILL.md` does not match gives an error wrapping `skillplan.ErrNotClaimable`; capacity gives `ErrSkillContentReceiptCapacity`; every other refusal gives `ErrSkillContentReceipt`. Evidence is filled from the preview and the adopting account.
  - `type AdoptionCandidate struct{ RevisionID string; Matched, ObservedOnly, RevisionOnly int }` (camelCase JSON: `revisionId`, `matched`, `observedOnly`, `revisionOnly`), `type SkillContentAdoptionOffer struct{ Candidates []AdoptionCandidate }`, and `SkillContentPreview.Adoption *SkillContentAdoptionOffer` (`json:"adoption,omitempty"`), computed only in `SkillContentPreviewFor` (never in the list) for a `captured` preview that is `present`, `exclusive` and whose only conflict is `adoption_required`; revisions of the pin's bundle where `ClaimSubset` succeeds, ordered by `Matched` descending then newest, at most 16.
- Test helper: `capturedPreview(t, e previewEnv, key string, c skillpreview.Capture) SkillContentPreview`.

- [ ] **Step 1: Write failing tests:**
  - `TestAdoptOlderRevisionThenUpgradePlan`: pin at R2, observed equals R1: the preview's `Adoption` offers R1 first with `Matched == 3`; adopting as R1 yields a receipt with provenance `adopted`, verification `controller_attested`, 3 managed files and full evidence; the old preview reads `superseded` with `receipt_changed`; a new preview is `Ready` with one `update`.
  - `TestAdoptCurrentRevisionIsANoOpPlan`.
  - `TestAdoptClaimsOnlyProvableFiles` (Review Focus 1): extra local file, an edited `scripts/run.sh`, and a file with matching bytes but flipped executable bit are unclaimed and absent from `Managed`; a new preview shows `unmanaged_path` for the edited path when the desired revision contains it.
  - `TestAdoptRefusals` (table, each subtest asserts its expected sentinel and that no row exists): wrong hash, stale, superseded, pin changed after the preview, pin removed after the preview, queued, capturing, failed and cancelled previews, shared, unknown and absent captures, a revision from another bundle, a revision from another account, `SKILL.md` mismatch (`errors.Is(err, skillplan.ErrNotClaimable)`), unknown revision, paused connector, viewer, foreign account's preview, 1,024 existing receipts, and an existing receipt for the slot of the same or another bundle (message path: release first).
  - `TestAdoptIsIdempotentAndConsumesThePreview`: same key and payload returns the same receipt ID and version; changed payload conflicts; after release the same key conflicts and no second row appears; release then adopting again from a **new** preview produces a different `Version` and the first preview stays superseded.
  - `TestAdoptConcurrentRequestsCreateOneReceipt` (Review Focus 5): two captured previews on one slot, two `Store` handles on the same directory, different keys, started behind a barrier: exactly one success and one `ErrSkillContentReceipt`, one row.
  - `TestAdoptDispatchesNothingToTheMachine`: after an adoption there are no `sync_jobs` rows, `ClaimSyncJob` returns nil and `ClaimSkillContentPreviewFor` returns no work.
  - `TestAdoptionOfferOnlyWhereAdoptable`: nil when the capture is shared, when a receipt exists, when the plan is `Ready` and on list reads; candidates exclude revisions whose `SKILL.md` differs.
- [ ] **Step 2: Run** `go test ./internal/store/ -run 'Adopt'`. Expected: FAIL.
- [ ] **Step 3: Implement** as specified. Order the checks as the spec lists them; compute the subset and offer from the stored capture and stored revision manifests only; insert with `syncID()` for ID and version.
- [ ] **Step 4: Run** `go test -race ./internal/store/`. Expected: PASS (full package, about two minutes).
- [ ] **Step 5: Commit** `Adopt a captured skill folder as the files a library revision can prove`.

### Task 6: Rename and merge lifecycle

**Files:**
- Modify: `internal/store/store_content_receipts.go`, `internal/store/store_machines.go` (call beside `moveSkillContentTargets`, before the environment delete), `internal/store/store_tidy.go` (fence beside the pin fence)
- Test: `internal/store/store_content_receipts_test.go`

**Interfaces:**
- Produces: `func moveSkillContentReceipts(tx *sql.Tx, uid int64, from, to string) error`: updates `environment_id` and bumps `version` and `updated_at` for every receipt of `from`; if `to` already has a receipt for the same name it returns `errors.New("these reports have different ownership receipts; release one before merging")`, rolling back the whole rename. The generic scope merge returns `errors.New("release private skill ownership receipts before merging this report into a different scope")` while the source has any receipt.

- [ ] **Step 1: Write failing tests** mirroring `TestSkillContentTargetMachineRenameAndScopeMerge` and `TestSkillContentTargetRenameCollisionIsAtomic`: `TestReceiptFollowsMachineRename` (via `MergeMachinesFor`: same ID and revision, new `EnvironmentID`, new `Version`, an earlier captured preview becomes `superseded` with `receipt_changed`, and a fresh preview after the rename still uses the receipt as baseline); `TestReceiptRenameCollisionIsAtomic` (a second receipt for the destination slot fails the rename, leaves the receipt untouched and writes no alias); `TestReceiptBlocksScopeMerge` (`MergeEnvironmentsFor` errors and the receipt survives).
- [ ] **Step 2: Run** `go test ./internal/store/ -run 'ReceiptFollows|ReceiptRename|ReceiptBlocks'`. Expected: FAIL.
- [ ] **Step 3: Implement** the helper and the two call sites.
- [ ] **Step 4: Run** `go test -race ./internal/store/ -run 'Receipt|MachineRename|Merge'`. Expected: PASS.
- [ ] **Step 5: Commit** `Keep ownership receipts through machine renames and refuse silent merges`.

### Task 7: API routes, audit trail and Activity labels

**Files:**
- Create: `internal/api/content_receipts.go`, `internal/api/content_receipts_test.go`, `web/src/lib/events.test.mjs`
- Modify: `internal/api/private_skills.go` (call `s.registerSkillContentReceipts(mux)` beside the previews), `internal/api/content_previews.go` (extract `func decodeStrictJSON(w http.ResponseWriter, r *http.Request, limit int64, out any, message string) bool` and make `decodePreview` call it), `web/src/lib/events.ts`, `web/package.json` (append `&& node src/lib/events.test.mjs` to the `test` script)

**Interfaces:**
- Consumes: Task 3 and 5 store methods, `privateSkillAccess`, `s.eventRequest(r, action, skillID, detail)`.
- Produces routes (all wrapped in `privateSkillAccess`): `GET /api/v2/skill-content-receipts` returning `{"receipts": [...]}`; `POST /api/v2/skill-content-receipts` adopting (body exactly `requestKey`, `previewId`, `reviewHash`, `adoptAsRevisionId`, 16 KiB) returning the receipt; `GET /api/v2/skill-content-receipts/{id}`; `DELETE /api/v2/skill-content-receipts/{id}` with `{"expectedVersion": "..."}` (required, non-null) returning `{"removed": true}`. `func skillContentReceiptError(w, err)` maps: invalid to 400 "Supply a request key, a captured preview with its review hash, and the library revision it was installed from."; missing (receipt or preview) to 404 "Ownership record or preview unavailable."; `ErrSkillContentReceipt`, preview and pin conflicts, scope, paused connector to 409 "This ownership record is unavailable, blocked, changed or already exists. Preview again, and release existing ownership before adopting again."; `skillplan.ErrNotClaimable` to 409 "That revision does not match this folder's SKILL.md, so nothing can be claimed."; capacity to 409 "Ownership record limit reached (1024 per account)."; anything else through `privateSkillError`. Audit events: `skill-content-adopt` with `{name, files, receiptId}` after a successful adoption, and `skill-content-release` with `{name, receiptId}` (read the receipt first to get the name). `events.ts` labels: adopt gives `Recorded ownership of ${files} files of ${name}`, release gives `Released ownership of ${name}` (add `files?: number` to `Detail`).

- [ ] **Step 1: Write failing tests:**
  - `internal/api`: `TestSkillContentReceiptAPIAdoptsReleasesAndStaysMetadataOnly` using a `presentReport(id, manifest)` helper beside `absentReport`: full flow create preview, claim, report a present capture equal to the pinned revision, `GET` the preview and see `adoption.candidates`, `POST` adoption, `GET` list and item, `DELETE` with the version; assert responses contain no `"data":`, no private skill text, no absolute path and `Cache-Control: no-store`; `ListEventsFor` shows both audit actions with metadata-only detail.
  - `TestSkillContentReceiptAPIBoundaries` table in the style of the previews' table: anonymous 401; viewer 403; missing, foreign and path-bearing origins 403; query string and empty query 400; unknown field (including `expectedVersion` on POST) 400; trailing JSON 400; oversized 400; malformed hash or IDs 400; wrong review hash 409; unknown preview 404; foreign account's preview and receipt 404; adopting a slot that already has a receipt 409; `DELETE` without or with null `expectedVersion` 400 and stale version 409; a **demoted** account (role set to `viewer` after adopting) gets 403 on every route; `mcpAPIPath` false for each route and method; legacy auth modes 403.
  - `web/src/lib/events.test.mjs` (transpile `events.ts` as `machines.test.mjs` does): the adopt label with `files: 3, name: "demo"` equals `Recorded ownership of 3 files of demo`, release equals `Released ownership of demo`, and an unrelated action is unchanged.
- [ ] **Step 2: Run** `go test ./internal/api/ -run 'SkillContentReceipt'` and `(cd web && node src/lib/events.test.mjs)`. Expected: FAIL.
- [ ] **Step 3: Implement** the routes, mapper, handlers, decoder extraction and labels.
- [ ] **Step 4: Run** `(cd web && npm run check && npm run build)`, then `go test -race ./internal/api/ -run 'SkillContent'`. Expected: PASS, zero Svelte diagnostics, no earlier API test changed.
- [ ] **Step 5: Commit** `Serve ownership receipts over the private API and label their audit events`.

### Task 8: Contract documents, handoff, validation and pull request

**Files:**
- Create: `docs/skill-content-receipts.md`
- Modify: `docs/skill-content-planner.md`, `docs/skill-content-targets.md`, `docs/skill-content-preview.md`, `README.md`, `handoff.md`

- [ ] **Step 1: Write `docs/skill-content-receipts.md`** covering, in this order: purpose and non-goals, the record and its provenance label, what adoption claims (and what it never claims), the adopt flow and checks, release, reads and the `adoption` offer, planner codes and the upgrade case, preview binding (`receiptVersion`, `receipt_changed`), lifecycle, API table with status codes, storage and limits, compatibility, what is **not** established, and the continuation (review UI, executor, S5). Use plain British prose with at most one light Yorkshire expression.
- [ ] **Step 2: Update the four existing documents:** planner doc gains the two block codes and says a baseline now comes from receipts; preview doc gains `receiptVersion`, `receipt_changed`, the `adoption` block, and that `adoption_required` now has a resolution; pin doc's continuation marks receipts done; README gets one sentence beside the pin and preview sentence; `handoff.md` gets the S4 record template (task, behaviour, compatibility, checks actually run, not performed, limitations, next task and first files, publication state), keeping the deferred PIN preference and the unresolved terminal-stop note untouched.
- [ ] **Step 3: Check links** in every edited document resolve (a short script over relative Markdown links) and run `git diff --check`.
- [ ] **Step 4: Run the full validation** in the order given under Global Constraints and record exact results (package counts, MCP test count, tmux version). Add the mutation checks used for earlier slices to the record: break each of baseline blocks, claim equality, the SKILL.md rule, create-only, hash binding, `receipt_changed`, and the rename move, and confirm a test fails each time before restoring.
- [ ] **Step 5: Commit** `Document ownership receipts and record the adoption handoff`, then push the branch, retitle PR #173 and rewrite its body from `.github/PULL_REQUEST_TEMPLATE.md` with the real evidence, mark it ready only after the validation passes, and watch all five workflows on the final head. Merge only if the user has authorised merging this PR; otherwise report it ready.
