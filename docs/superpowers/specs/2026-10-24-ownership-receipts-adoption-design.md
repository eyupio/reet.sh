# Ownership receipts and adoption (S4 design)

Date: 2026-10-24. Status: design for review; no code written.

Nowt gets called ours without proof. This slice lets an account record which files on
a machine reet manages, and lets an existing, unowned skill folder be brought under
management by an explicit, reviewed decision. It builds on the merged
[pins](../../skill-content-targets.md), [planner](../../skill-content-planner.md) and
[negotiated previews](../../skill-content-preview.md). It writes nothing to any machine.

## Purpose and success criteria

Today no ownership baseline exists, so every existing skill folder previews as
`adoption_required`, even when its bytes equal the library's. That blocks the
reviewed executor and every upgrade story that follows.

Success means all of these hold, and tests prove each:

1. An account owner can adopt a skill folder that was observed by a fresh preview,
   naming the library revision it came from, and reet records exactly the files that are
   **provably that revision's** and nothing else.
2. The next preview of that slot uses the receipt as the planner's baseline, so adopting
   an older revision while the pin names a newer one yields a real upgrade plan.
3. Matching names or matching bytes alone never create ownership. Only the explicit,
   hash-bound decision does.
4. A receipt that no longer fits (root moved, other bundle pinned, skill deleted) blocks
   with a clear code instead of being reused or silently dropped. Re-enrolment needs a
   revoke first, which removes the receipt.
5. Nothing in this slice writes to a machine, reuses import approval, or authorises
   an apply. A receipt is a claim; a future executor must re-observe under a local lock.

## Decisions taken with the product owner (24 October 2026)

| Decision | Choice |
| --- | --- |
| What adoption claims | Only files whose size, mode and digest equal a chosen revision of the pinned bundle. `SKILL.md` must be among them. Locally edited or unrecognised files are never claimed. |
| Slice scope | Receipt storage, adopt and release API, planner baseline from receipts, fences and tests. No browser UI, no executor, no machine writes. |
| Confirmation authority | The signed-in account owner, bound to a fresh preview's review hash. No new connector protocol and no machine-side confirmation. |
| Replacing a claim | Not offered. Adoption is create-only; release, preview again and re-adopt. A slot that has a receipt can never preview as `adoption_required`, so replacement could not pass the adoption checks. |
| Mismatching receipts in the planner | Blocked diagnostic plans with codes, not errors. Plan version stays 1. |

## Non-goals

The executor and any filesystem write; `applied` receipts (reserved, never created
here); ownership transfer between bundles; claiming locally edited files, which is
S5's three-way review; any browser control; machine-side confirmation. The executor
will separately need the machine's local opt-in (`AllowApply`), a lock and fresh
re-observation, and must treat a receipt as a claim to re-verify.

## Approach

A dedicated `skill_content_receipts` table, one current row per account, environment and
skill slot, with an independent CAS version. Rejected: storing the baseline on the pin
(removing or changing a pin would silently change ownership of files on disk) and
keeping it inside the preview record (previews are capped, expire and can be dismissed;
ownership must outlive them). Adoption and release are audited through the existing
events system rather than a new table.

## Data model

```
skill_content_receipts(
  id TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  environment_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
  agent_id TEXT NOT NULL,                 -- connector (enrolment); generation is the same ID
  name TEXT NOT NULL,                     -- skill slot, from the owned bundle
  bundle_id TEXT NOT NULL, revision_id TEXT NOT NULL,   -- revision the subset was verified against
  root_id TEXT NOT NULL,                  -- adapter-derived physical root identity
  adapter TEXT NOT NULL, schema_version INTEGER NOT NULL,
  managed TEXT NOT NULL,                  -- canonical managed-subset manifest, JSON
  provenance TEXT NOT NULL,               -- 'adopted' here; 'applied' reserved
  evidence TEXT NOT NULL,                 -- JSON, metadata only
  request_key TEXT NOT NULL,
  version TEXT NOT NULL,                  -- independent random CAS version
  created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
  UNIQUE(user_id, environment_id, name),
  UNIQUE(user_id, request_key),
  FOREIGN KEY(agent_id, user_id) REFERENCES sync_agents(id, user_id) ON DELETE CASCADE,
  FOREIGN KEY(user_id, bundle_id, revision_id)
    REFERENCES private_skill_revisions(user_id, bundle_id, id) ON DELETE CASCADE)
```

- `managed` is the planner's `Baseline.Manifest`: complete, canonical, at most 256 files,
  validated through the S1 identity format on write and on every read, and checked on
  every read against the recorded revision (each managed file must equal that revision's
  file). A failure ends the preview as `plan_unavailable`; a damaged receipt is never
  accepted quietly.
- Mode (`additive` or `managed`) is not stored: it comes from the pin when each plan is
  built, so changing it never needs a new receipt.
- `evidence` holds preview ID, review hash, capture hash, observed manifest digest,
  pin ID and version, capture time, adoption time and the adopting account. No bytes.
- The planner `Target` is rebuilt from the row (`user_id`, `environment_id`, `agent_id`,
  `name`, `root_id`, `adapter`, `schema_version`, generation = `agent_id`), so a rename that
  updates `environment_id` keeps the baseline consistent.
- Limit 1,024 receipts per account. Migration is additive (`CREATE TABLE IF NOT EXISTS`),
  after the preview tables. Full SQLite backups include it; personal shelf exports do not.
- API label for adopted receipts: `provenance: "adopted"`, `verification:
  "controller_attested"`. Never "verified on disk".

Receipts are independent of pins and previews: removing a pin, changing it or dismissing
a preview never alters ownership. They are deleted with the account, a revoked
enrolment, or a forgotten environment, so a forgotten and recreated scope cannot
resurrect ownership.

## Adoption

`POST /api/v2/skill-content-receipts`, strict 16 KiB JSON, exactly:

```json
{"requestKey":"…16–80 chars…","previewId":"48-hex","reviewHash":"64-hex",
 "adoptAsRevisionId":"32-hex"}
```

Adoption is **create-only**. A slot that already has a receipt cannot be adopted again:
release it first (below), preview again and re-adopt, so a stale or mistaken claim is
always ended by an explicit, audited step. Replacement by version would be unreachable
anyway: a slot with a receipt previews with that receipt as its baseline, so its plan can
never be `adoption_required`. All of the following run in **one write transaction** whose
first statement takes the account write lock:

1. Active writer account. A repeated `requestKey` with an identical payload returns the
   original receipt; a changed payload conflicts.
2. The preview is owned and its effective state is exactly `captured`: not `stale`,
   `superseded`, `expired`, `failed` or `cancelled`.
3. `reviewHash` equals the preview's stored value. This proves the caller saw this
   capture, plan, pin version, inventory and process.
4. The capture is `present` with `exclusive` ownership, and the plan's only conflict is
   `adoption_required` (a shared, unknown or otherwise blocked root can never be adopted).
5. The pin still exists at the version the preview bound; the connector is unpaused.
6. `adoptAsRevisionId` is an owned revision of the **pin's bundle**.
7. The managed subset is computed **server-side** from the stored capture and that
   revision's stored manifest: a path is claimed only if both sides have it with equal
   size, digest and executable bit. `SKILL.md` must be claimed. Observed files that are not
   claimed are left unmanaged and unmentioned in the receipt.
8. The slot has no receipt (`UNIQUE(user_id, environment_id, name)`). An existing receipt,
   whatever its bundle, is a conflict that says to release it first. Moving ownership
   between bundles therefore needs a deliberate release and is never a retag.

Effects: the receipt row is written and is the authoritative record of the decision (its
`evidence` names who adopted, from which preview, when). The API handler also emits an
audit event (`skill-content-adopt`, metadata only) as a convenience trail; it is sent after
the commit, so a crash between the two can lose the event but never the record. The preview used is consumed: previews bind the
receipt version that existed when they were planned (below), so after adoption it reads
`superseded` with `receipt_changed`, and a new preview shows the new plan.

An uncertain response is reconciled with `GET`; an identical replay while the receipt
exists returns the original. Replaying after it was released conflicts (the preview was
consumed), and never creates a second claim.

### Release

`DELETE /api/v2/skill-content-receipts/{id}` with `{"expectedVersion":"…"}` removes
ownership only. Files, pins and private revisions are untouched. It exists so a user
who deliberately deleted a skill can clear `managed_target_missing`, and to undo a
mistaken adoption. It emits `skill-content-release`.

### Reads

`GET /api/v2/skill-content-receipts` (summaries) and `/{id}` (adds the managed file
metadata). Same boundary as pins: accounts mode only, active writer, same-origin browser
write or owning personal token, no queries, `no-store`, no MCP allowlist change.

The preview item response gains an informational `adoption` block, computed on read for
a `captured` preview whose only conflict is `adoption_required`: each eligible revision of
the pin's bundle (`SKILL.md` matches) with counts of matched, observed-unmatched and
revision-only paths, best match first, at most 16. It never selects a revision for the
caller, and offers none when the root is not exclusive.

## Planner and preview changes

**Baseline source.** When a preview is captured, the store loads the slot's receipt and,
if one exists, passes `Baseline{Target, BundleID, RevisionID, Manifest: managed}` to
`skillplan.Build`.

**Planner (`internal/skillplan`).** The two baseline errors become blocked plans, version
unchanged because no baseline-bearing plan has ever been produced or approved:

| Condition | Result |
| --- | --- |
| `Baseline.Target` differs from the target (root moved, adapter/schema; a re-enrolled connector cannot reach this, see Lifecycle) | block `baseline_target_mismatch` |
| `Baseline.BundleID` differs from the desired bundle | block `baseline_bundle_mismatch` |
| `Baseline.Target.OwnerID` differs (another account) | still an **error**: an integrity violation, not a benign mismatch |
| Invalid baseline revision ID or manifest | still an error |

Precedence among blocks: adapter, root ownership, generation, inventory, unknown
observation, stale, then baseline target, then baseline bundle, then the existing
presence logic (including `managed_target_missing` for an absent root with a baseline and
`adoption_required` for a present one without). Three existing planner test cases move
from "invalid input" to the block table (`wrong bundle`, `baseline root`, `baseline
generation`); `baseline owner`, `baseline revision` and `baseline metadata` stay errors.

**Preview binding.** A preview stores `receiptVersion` (empty when none) as seen in the
transaction that built its plan. A new fence reason `receipt_changed` supersedes a
`captured` preview if the slot's current receipt version differs. The review hash
includes `receiptVersion`, so a receipt that is released and re-adopted identically still
invalidates old reviews (the ABA rule, as for pins).

**Upgrade case.** Adopting a folder as an older revision while the pin names a newer one
updates only the files that were adopted as the older revision, creates new files, and
blocks (`unmanaged_path`) any newer-revision path that exists locally but was not claimed.
Nothing unproven is overwritten, and a deleted adopted skill never recreates itself.

## Lifecycle

- **Machine rename** within one enrolment moves receipts to the new environment ID and
  bumps their versions, as pins do. Two receipts for the same slot conflict and roll the
  whole rename back with HTTP 409 (`release one before merging`).
- **Generic report merge** into another scope is refused while the source has a receipt.
- **Revoke, forget, account deletion** cascade.
- **Pin removal or change, preview dismissal** never touch receipts.
- **Root moved or adapter/schema changed:** the next preview reports
  `baseline_target_mismatch`; the user releases and, if appropriate, adopts again.
- **Re-enrolled:** one enrolment exists per host, so re-enrolling needs a revoke first,
  which cascades the receipt. The next preview is `adoption_required` again.

## Errors

400 invalid input (including unknown or extra JSON fields, a bad hash, a missing
`SKILL.md` match); 404 foreign or missing; 409 stale or superseded preview, changed pin,
blocked plan, version conflict, other-bundle receipt, paused connector or full capacity.
Messages are plain and direct, with no reassurance around failures.

## Security notes

- A receipt can only claim bytes that equal a revision the account owns, so a mistaken or
  malicious adoption cannot extend a later overwrite to unproven content; at worst it lets
  a future apply move library-identical files to a newer library revision, which is the
  feature.
- Later local edits surface as `local_modified` conflicts; the executor must still
  re-observe under a lock before any change.
- The review hash binds the exact capture the caller saw; replaying an old hash after any
  fence change fails.
- Responses carry metadata only. Audit events carry IDs and counts, never paths or bytes.
- Compromise of the controller account is out of scope, as it is for pins.

## Testing

Written test-first, with real SQLite and HTTP, then mutation-checked as for previews.

- **Adoption:** older revision then upgrade plan; current revision then ready no-op plan;
  `SKILL.md` mismatch refused; extra local files preserved; a differing file stays
  `unmanaged_path`; empty subset refused.
- **Fences:** wrong review hash; stale, superseded, expired, failed preview; changed or
  removed pin; paused connector; shared, unknown or non-present capture; revision from
  another bundle or account; foreign account; viewer; MCP allowlist closed.
- **Idempotency and concurrency:** identical replay; changed payload; competing Store
  connections; capacity; adopting a slot that already has a receipt (same or other
  bundle) refused with the release-first message; a damaged stored receipt ends the
  preview as `plan_unavailable`.
- **Release and lifecycle:** release; rename moves and conflicts; merge fence; revoke and
  forget cascades; pin removal leaves the receipt; migration and reopen.
- **Preview and planner:** `receipt_changed` fence; release then identical re-adopt still
  invalidates; `baseline_target_mismatch`, `baseline_bundle_mismatch`,
  `managed_target_missing`; owner mismatch still an error; moved tests keep their intent.
- **Boundaries:** nothing in the machine's filesystem changes; no bytes in any response.

## Files

New: `internal/store/store_content_receipts.go` and tests, `internal/api/content_receipts.go`
and tests, `docs/skill-content-receipts.md` (contract, written with the implementation).
Changed: `internal/skillplan/plan.go` and tests; `internal/store/store_content_previews.go`
(baseline lookup, `receiptVersion`, fence, `adoption` block); `store_machines.go` (rename),
`store_tidy.go` (merge fence); `store.go` (migration); README, the pin, planner and preview
docs, and `handoff.md`.

## Limits worth stating plainly

Adoption relies on a capture of at most five minutes old and the owner's say-so; it cannot
see the machine afterwards. "Exclusive" still means exclusive among configured scopes.
Locally edited installs cannot be adopted until S5. There is no UI, so adoption is
API-only for now, as pins and previews are. No live machine was involved or is needed.

## What follows

Review UI with pin selection, then the reviewed executor (staging, local lock, journal and
backup, installed verification, `applied` receipts, temporary-filesystem failure and
recovery gates), then S5 three-way review.
