# Ownership receipts and adoption (S4 baseline slice)

Nowt gets called ours without proof. An account can now record which files of one
skill folder on one enrolled machine its private library provably published, by an
explicit decision bound to a fresh [preview](skill-content-preview.md). That record,
an ownership receipt, becomes the [planner's](skill-content-planner.md) baseline for
the slot, so an adopted folder previews as a real plan instead of `adoption_required`.
Adoption itself writes nothing to, reads nothing from and verifies nothing on any
machine, dispatches no work, and a receipt is not permission to write. There is no MCP tool
for receipts. The [Private skills dialog](skill-content-review-ui.md) is the browser control
for adoption and release; the API below is the contract it uses and the later executor will
build on.

## What a receipt is and is not

| It is | It is not |
| --- | --- |
| The controller's attestation that these files (path, size, digest, executable bit) were published from one revision of one private bundle into one skill slot | Proof of what is on the disk now |
| Metadata: paths, sizes, modes, digests, identifiers and times | File contents, local paths or a way to choose a directory |
| The planner's baseline for that slot | A pin, a preview, an approval, apply authority or an `applied` receipt |
| Independent of pins and previews | Something that removing a pin or dismissing a preview can change |

A matching name, source URL or even byte-identical files never create ownership. Only
the explicit, hash-bound decision below does.

## The record

One current receipt per account, environment and skill slot.

| Field | Meaning |
| --- | --- |
| `id`, `version` | 48-character opaque IDs. The version is an independent random compare-and-swap value, fresh on every change |
| `environmentId`, `agentId`, `name` | The slot: the reported scope, the enrolled connector and the skill name taken from the owned bundle |
| `bundleId`, `revisionId` | The revision the managed files were verified against when adopted |
| `managedDigest`, `fileCount` | Digest and size of the managed manifest |
| `managed` | The complete canonical manifest of the claimed files. Item reads only |
| `provenance` | `adopted`. `applied` is reserved for a future executor and is never written here |
| `verification` | `controller_attested`. Never "checked on disk" |
| `evidence` | Preview ID, review hash, capture hash, digest of the whole observed folder, pin ID and version, capture time, adoption time and adopting account. No bytes, no paths |
| `damaged` | Present (`true`) only on a list row whose stored data no longer decodes |
| `createdAt`, `updatedAt` | Milliseconds. A machine rename bumps `updatedAt` |

The root identity, adapter, schema version and request key are stored so the planner's
target can be rebuilt from the row, but they are not sent. `applied` receipts do not
exist: initial installs record nothing until a future executor verifies the installed
tree. A stored row with any other provenance reads as damaged.

## What adoption claims

The managed subset is computed on the server from the stored capture and the stored
manifest of the chosen revision. The caller supplies four values (below) and never a
file list.

| Observed file against the chosen revision | Result |
| --- | --- |
| Same path, size, digest **and** executable bit | Claimed |
| Same path, different bytes or mode (a local edit) | Left unmanaged, not mentioned in the receipt |
| Only in the folder | Left unmanaged, not mentioned in the receipt |
| Only in the revision | Not claimed. A later plan creates it |
| `SKILL.md` not among the claimed files | The whole adoption is refused |

Locally edited or unrecognised files are never claimed; that is the S5 three-way
review. A receipt holds at most 256 files.

## Adopt

`POST /api/v2/skill-content-receipts`, strict JSON of at most 16 KiB, exactly:

```json
{"requestKey":"16 to 80 letters, digits, _ or -","previewId":"48 lowercase hex",
 "reviewHash":"64 lowercase hex","adoptAsRevisionId":"32 lowercase hex"}
```

Adoption is create-only and binds to a fresh preview's review hash. All the checks run
in one write transaction whose first statement takes the account write lock, so two
adoptions of one slot cannot both succeed, even through separate Store connections.

| Step | Check | Otherwise |
| --- | --- | --- |
| 1 | The account is an active writer. A repeated request key is answered first (see below) | 403 at the route for a viewer; the store itself answers 404 |
| 2 | The preview is owned and its effective state is exactly `captured`: not `queued`, `capturing`, `stale`, `superseded`, `expired`, `failed` or `cancelled` | 404 foreign or unknown, 409 otherwise |
| 3 | `reviewHash` equals the preview's stored value, so the caller saw this capture, plan, pin version, inventory, process and receipt version | 409 |
| 4 | The capture is `present` in an `exclusive` root and the plan's only conflict is `adoption_required` | 409 |
| 5 | The pin still exists at the version the preview bound, and the connector exists and is not paused | 409 |
| 6 | `adoptAsRevisionId` is an owned revision of the **pin's bundle** | 404 unknown or foreign, 409 another bundle |
| 7 | The claim (above) succeeds, with `SKILL.md` among the claimed files | 409 |
| 8 | The slot has no receipt of any bundle, even a damaged one, and the account holds fewer than 1,024 | 409 |

Effects, all in that transaction: the receipt is written, and the preview used is
**consumed**. It is stored as `superseded` with `receipt_changed`, so it stays
superseded even after a later release and can never claim the folder twice. Nothing is
sent to the machine, no job or preview work is queued, and no private revision, pin or
legacy desired set changes. The response is the receipt exactly as an item read returns
it, proved against its revision before the commit.

Re-adopting is deliberate: release, preview again, adopt. Moving ownership between
bundles is therefore never a retag. A slot that has a receipt can never preview as
`adoption_required`, so replacement could not pass the checks anyway.

### Idempotency

- A repeat of the same key **and** the same preview, review hash and revision, while the
  receipt exists, returns the original receipt with HTTP 200 and logs no second audit
  event. A changed payload under that key conflicts.
- After the receipt is released, replaying the identical request conflicts, because its
  preview was consumed.
- Request keys end with their row: there is no tombstone. The same key with a **new**
  preview after a release is a new decision and makes a new receipt.
- After an uncertain response, reconcile with `GET`.

## Release

`DELETE /api/v2/skill-content-receipts/{id}` with `{"expectedVersion":"..."}` (required,
at most 4 KiB) ends ownership and nothing else: files on the machine, the pin and the
private revisions are untouched. A stale version conflicts and keeps the receipt. It
exists to clear `managed_target_missing` after a user deliberately deleted a skill, to
undo a mistaken adoption, and as the first step of any re-adoption. It works on a
damaged receipt too, using the version the list shows, and it never reinstates the
consumed preview.

## Reads and the adoption offer

| Read | What it returns |
| --- | --- |
| `GET /api/v2/skill-content-receipts` | `{receipts}`: metadata summaries ordered by environment, name and ID. No managed files |
| `GET /api/v2/skill-content-receipts/{id}` | One receipt with `managed`, after proving it against the recorded revision |

The list shows the digest and file count from the stored manifest **without** having
verified it against its revision: a summary is not a verification. A row whose stored
data no longer decodes does not hide the others. It is listed with `damaged: true`, its
identity, slot and version, and empty provenance, verification and evidence, so it can
still be found and released with its listed version.

The item read is the strict one. The stored manifest must be exactly what the S1 identity
format rebuilds from its own files (at most 256, `SKILL.md` among them, canonical), and
every managed file must equal the same path of the recorded revision. A failure is the
plain integrity error, HTTP 500 "Could not access private skill storage.", never a 404 or
a 409. A foreign, unknown or malformed ID is 404.

### The `adoption` offer

A single-preview read (`GET /api/v2/skill-content-previews/{id}`, and the cancel response
that re-reads it) gains an informational `adoption` block when the preview is
`captured`, present, in an `exclusive` root, and its only conflict is
`adoption_required`.

- `candidates` lists revisions of the **pin's bundle** where the claim would succeed,
  best match first and then newest, at most 16. Each has `revisionId`, `matched`,
  `observedOnly` (folder files not claimed) and `revisionOnly` (revision files the folder
  does not hold identically).
- A revision whose stored manifest does not decode or validate is left out, so one damaged
  sibling cannot hide the review. An adoptable folder no revision can prove gets an empty
  list.
- It never selects a revision for the caller, is computed on read from stored manifests,
  is never stored (writes strip it), and never appears in lists.
- `POST /api/v2/skill-content-previews` replaying a captured request returns **no**
  `adoption` block, while `GET /{id}` does.

Each offer decodes up to 256 stored manifests inside one read transaction. That is fine at
the limits; a cheaper prefilter is a follow-up.

### File breakdown

`GET /api/v2/skill-content-previews/{id}/adoption/{revision}` explains one candidate file
by file, so a review can show what adoption would claim and what it would leave. The offer
carries counts only, and the library's revision read returns file bytes, which a browser
must not fetch to draw a table.

```json
{"revisionId":"32 hex","files":[
  {"path":"SKILL.md","bytes":120,"digest":"64 hex","executable":false,"status":"claimed"},
  {"path":"notes/mine.txt","bytes":12,"digest":"64 hex","executable":false,"status":"stays_yours"},
  {"path":"scripts/run.sh","bytes":80,"digest":"64 hex","executable":true,"status":"library_only"}],
 "matched":1,"observedOnly":1,"revisionOnly":1}
```

| Status | Meaning |
| --- | --- |
| `claimed` | A file adoption would record: the same path, size, digest and executable bit on the machine and in the revision |
| `stays_yours` | The machine's copy of a file adoption would not claim: one the revision lacks, or one whose bytes or mode differ. It stays unmanaged |
| `library_only` | The revision's copy of a file the folder does not hold identically: missing, edited or in another mode. If the folder lacks the file, a later plan creates it. If the folder holds a different copy, that copy is `stays_yours` and a later plan against a revision containing the path stops with `unmanaged_path` until the person resolves it; an unclaimed file is never updated |

- A path whose bytes or mode differ appears twice, once as `stays_yours` (the machine's
  copy) and once as `library_only` (the library's). Rows are sorted by path, then
  `claimed`, `stays_yours`, `library_only`, so a path has at most two rows and
  `matched + observedOnly + revisionOnly` is the number of rows. The counts are the
  offer's for that revision.
- Metadata only: path, size, digest and executable bit. No file bytes, no private skill
  text and no host path, which is why this exists instead of the revision read.
- It is computed on read from the stored capture and the stored manifest with the same
  claim function adoption uses, so the screen cannot promise a claim adoption would
  refuse. It stores, consumes and audits nothing.
- It asks for what adoption asks of the preview and the revision, and answers with the
  same kinds of error. It looks the revision up before reading the preview, so a 404 can
  carry either message ("Private skill unavailable." or "Ownership record or preview
  unavailable.") when both IDs are wrong; each message depends only on the caller's own
  rows, so neither reveals anything about another account. 404: an unknown, foreign or
  malformed preview or revision ID, or an account that can no longer read the library. 409: a preview that is not a current `captured`
  check of a present folder in an exclusive root whose only conflict is
  `adoption_required` (already receipted, superseded, stale and the rest), a revision of
  another bundle than the pin's, or a revision whose `SKILL.md` differs (the adoption
  message). A revision whose stored manifest is damaged is the same 500 fault as an
  adoption of it. The 409 text is shared with adoption ("release existing ownership
  before adopting again"), so a client should act on the status and the screen's own
  words, not on that text. The breakdown does not check the 1,024-receipt limit, so an
  account at the limit sees claims that adoption would then refuse with its capacity
  message.
- Same boundary as the other routes here: accounts mode, an active writer (a viewer is
  403), no query parameters, `no-store`, and not on the MCP allowlist.

## Planner codes and the upgrade case

The slot's receipt is the planner baseline. Two former planner errors are now blocked,
diagnostic plans, with the plan version still 1 because no baseline-bearing plan had ever
been produced or approved.

| Situation | Result |
| --- | --- |
| The receipt's target differs from the preview's: scope root moved or replaced, adapter or schema changed | Blocked `baseline_target_mismatch` |
| The pin now selects another bundle than the receipt's | Blocked `baseline_bundle_mismatch` |
| The root is absent but a receipt exists (the skill was deleted) | Blocked `managed_target_missing` until the receipt is released |
| A present folder and no receipt | Blocked `adoption_required`, which adoption now resolves |
| The receipt belongs to another account | Still an **error**: an integrity violation, not a mismatch |
| Invalid baseline revision ID or manifest | Still an error |

Precedence among blocks: adapter, root ownership, generation, inventory, unknown
observation, staleness, baseline target, baseline bundle, then the presence logic. A
re-enrolled machine cannot reach `baseline_target_mismatch`: one enrolment exists per host,
so re-enrolment first needs a revoke, which cascades the receipts, and the next preview is
`adoption_required` again.

**Upgrade case.** Adopt a folder as an older revision while the pin names a newer one: the
plan updates only files that were adopted, creates new files, and blocks
`unmanaged_path` on any newer-revision path that exists locally but was not claimed.
Nothing unproven is overwritten, and a deleted adopted skill never recreates itself.
Adopting the pin's current revision gives a ready, no-op plan. The mode (`additive` or
`managed`) is not stored on the receipt: it comes from the pin each time a plan is built.

## Preview binding

A preview stores `receiptVersion` (empty when the slot had no receipt), read in the
transaction that built its plan, and the review hash includes it. A captured preview
becomes `superseded` with the reason `receipt_changed` when the slot's receipt is created,
released or replaced. Versions are random, so releasing and then adopting identical
content also supersedes older reviews (the same ABA rule as pins). Only a built plan binds
a receipt version; a request still queued or capturing reads whatever receipt exists when
its plan is built.

Three behaviours to know:

- **Fences are live.** Only the preview used for adoption is persisted as consumed.
  Another preview of the same slot that read `superseded` only because a receipt existed
  can read `captured` again after that receipt is released. It is bounded by the five-minute
  capture window and the inventory fence, and it never allows two receipts for one slot,
  because adoption needs a receipt-free slot and consumes the preview it uses.
- **Fences compare versions only.** A receipt that became unreadable without a version
  change leaves a captured review current. A future executor must therefore re-read the
  baseline through the store's validating lookup inside its own transaction and must not
  trust a stored review alone.
- **Plan failures are terminal.** Any error building a preview's plan, including a damaged
  receipt, ends that request as `failed` with `plan_unavailable`. A transient database
  error is currently treated the same way; that is a known limitation. Release the damaged
  receipt and preview again.

## Lifecycle

| Event | Effect on receipts |
| --- | --- |
| Machine rename within one enrolment | Receipts move with the machine: same ID, bundle and revision, a new random version and a bumped `updatedAt`. Earlier reviews are superseded |
| Rename into a destination that already holds a receipt for the same name | The whole rename fails atomically with HTTP 409 and no alias is written ("these reports have different ownership receipts; release one before merging") |
| Generic report merge into another scope | Refused with 409 `environment_has_receipts` while the source holds a receipt. Pins are checked first (`environment_has_pins`). The merge dry run now previews the same refusal. Merge has no `force` |
| Machine rename or merge with conflicting pins | Also fails atomically with 409. Machine-merge 409s carry the message only, with no `code` field |
| Environment delete without `force` | Refused with `environment_has_receipts` ("environment still holds ownership receipts; release them first") while the report holds receipts. A report that also has skills reports `environment_has_skills` first |
| Environment delete with `force`, forgetting a machine or environment, revoking an enrolment, deleting an account | Receipts cascade, by design. A forgotten and recreated scope cannot resurrect ownership |
| Removing or changing a pin, dismissing a preview | Receipts are untouched |
| Scope root moved, adapter or schema changed | The next preview blocks with `baseline_target_mismatch`; release and adopt again if appropriate |
| Re-enrolment | Needs a revoke first, which cascades the receipts |

Not guarded: the automatic prune of empty server-side reports, which is practically
unreachable for a report that holds a receipt, and the receipt count in a non-forced
environment delete, which is read just before rather than inside the delete transaction
(a negligible window).

## Browser and token API

Same boundary as the pin and preview routes: accounts mode only, an active writer, no
query parameters, strict JSON (unknown fields and trailing data rejected), `no-store`,
same-origin browser writes or the owning personal token. MCP credentials keep their
endpoint allowlist, which does not include these routes. There is no MCP tool for
receipts.

| Method | Path | Result |
| --- | --- | --- |
| GET | `/api/v2/skill-content-receipts` | `{receipts}` summaries |
| POST | `/api/v2/skill-content-receipts` | Adopt `{requestKey, previewId, reviewHash, adoptAsRevisionId}` (16 KiB). Answers 200 with the receipt, also for an identical replay |
| GET | `/api/v2/skill-content-receipts/{id}` | One receipt with `managed` |
| DELETE | `/api/v2/skill-content-receipts/{id}` | `{"expectedVersion":"..."}` required (4 KiB). Answers `{"removed":true}` |
| GET | `/api/v2/skill-content-previews/{id}/adoption/{revision}` | The [file breakdown](#file-breakdown) of one captured check against one revision of the pin's bundle. Metadata only, no audit event |

| Status | When |
| --- | --- |
| 400 | A malformed key or hash, or a malformed ID on `DELETE` (a malformed ID on `GET /{id}` is 404); an unknown or extra JSON field, trailing data or an oversized body; a missing or null `expectedVersion`; any query string |
| 401 / 403 | Not signed in (401). Legacy auth modes, viewers and writes without a same-origin request or owning token (403) |
| 404 | Receipt, preview or private revision unknown, foreign or unavailable to this account ("Ownership record or preview unavailable." or "Private skill unavailable.") |
| 409 | The revision's `SKILL.md` differs; 1,024 receipts reached; a preview that is not `captured`, a wrong review hash, a changed or removed pin, a paused connector, a slot that already has a receipt, a changed payload under a used key, or a stale `expectedVersion` |
| 500 | Stored data that fails integrity validation on an item read, and any other fault: "Could not access private skill storage." |

Two 409 messages: "That revision does not match this folder's SKILL.md, so nothing can be
claimed." and "This ownership record is unavailable, blocked, changed or already exists.
Preview again, and release existing ownership before adopting again."

### Audit and Activity

| Event | Detail | Activity label |
| --- | --- | --- |
| `skill-content-adopt` | `{name, files, receiptId}` | Recorded ownership of N files of NAME ("1 file" for one) |
| `skill-content-release` | `{name, receiptId}`, or `{receiptId}` when the name could not be read | Released ownership of NAME ("a skill" without one) |

Both carry metadata only: no paths, hashes or bytes. An adoption is logged once per
decision, after the commit, so a crash between the two can lose the event but never the
record. The receipt's own `evidence` is the authoritative trail.

## Storage and limits

`skill_content_receipts` is an additive table (`CREATE TABLE IF NOT EXISTS`), migrated
after the preview tables. It is account- and enrolment-scoped, with `ON DELETE CASCADE`
foreign keys to the account, the environment, the enrolment and the exact private
revision. Private revisions are immutable and nothing deletes one except deleting the
account. Full SQLite backups include it; personal shelf exports do not.

| Limit | Value |
| --- | --- |
| Receipts per account | 1,024 |
| Managed files per receipt | 256 |
| Request key | 16 to 80 letters, digits, `_` or `-` |
| Adopt body, release body | 16 KiB, 4 KiB |
| Offer candidates | 16 |

## Compatibility

Old connectors and the connector channel are unchanged: adoption and release are
controller-side only and never reach a connector. No field or hash meaning in `SyncSkill`,
`SyncTarget`, legacy desired sets, observations, imports or terminal messages changed. A
preview gains an optional `receiptVersion` and an item-read-only `adoption` block; previews
captured before this slice carry no receipt version and read as before, because stored
review hashes are never recomputed. The planner version stays 1. The MCP environment tools
keep their names and gain the receipt and pin refusals, with the codes
`environment_has_receipts` and `environment_has_pins`.

## What this does not establish

Nothing was written to or verified on any machine. A receipt rests on a capture of at most
five minutes old and the owner's say-so; it cannot see the machine afterwards, and later
local edits or deletions only surface as conflicts on the next preview. There is no
executor, staging, local lock, journal, rollback or `applied` receipt, no ownership
transfer between bundles, no claiming of locally edited files (that is S5) and no fleet
apply. "Exclusive" root ownership is among the connector's configured scopes only, and
capture is a point-in-time read of a changing disk. Tests use synthetic homes, SQLite and
HTTP; no live machine, production service or terminal session was involved, and no
physical cross-platform or fleet check was made. The whole-binary Windows build fails on
untouched `main` in `internal/scan/terminal_setup.go`; that predates this slice and is not
a CI target.

## Granular continuation

1. **Done in a following slice:** the review UI. The [Private skills
   dialog](skill-content-review-ui.md) has pin selection, check review and receipt review
   together: truthful labels (`controller_attested`, never "verified"), the `adoption` offer
   shown with its counts and a file breakdown as a decision and never a one-click default,
   `damaged` rows with a release action, and keyboard and phone layouts. MCP exposure for
   receipts is a separate decision.
2. **Reviewed executor.** Full payload staging, a local lock, revalidation against the
   review hash, a baseline re-read through the store's validating lookup inside its own
   transaction, journal and backup before replacement, installed verification and durable
   `applied` receipts, with temporary-filesystem failure, crash and recovery gates. Do not
   close S4 or start S5 fleet apply before they pass.
3. **S5.** Three-way review, which is where locally edited files could be claimed by an
   explicit decision and where any ownership transfer between bundles belongs.

Known follow-ups, none of which blocks the above:

- Machine-merge 409s carry no `code` field, so MCP and stdio clients lose the message.
- A receipts-aware guard for the automatic prune of empty server-side reports.
- Optionally persist `superseded` on every captured preview of the slot at adoption.
- A cheaper prefilter for the offer.
- Treat a transient database error while building a plan as retryable instead of
  `plan_unavailable`.
- Name the receipt in the release audit event with a single-row lookup instead of decoding
  the account's list, so a row the list cannot scan is named too.
- Record how many receipts a forced environment delete cascaded, since no
  `skill-content-release` event is written for them.
- A test that puts both skills and a receipt on one report, to pin the order in which the
  delete dry run and the real delete report them; the merge dry run checks only the source
  report, as the real merge does, and a destination holding a pin or receipt must keep
  returning 200.
- The browser's per-report Forget drops receipts with the report and says installed files
  are unchanged. The Private skills dialog does not mention that yet, so it is still to do. `AdoptSkillContentPreviewFor` has no
  production caller now that the API uses the once-only variant.

Preserve terminal sessions and controls, the approved sudo template behaviour and the
deferred user-controlled security/PIN preference noted in `handoff.md`.

## Validation recorded for this implementation

Every task was written test-first, with the failing run seen for the right reason before
the code, against real SQLite and real HTTP. A separate reviewer examined each task and
sent three back for one fix round each: the damaged-row listing, the adoption offer and
the merge dry run. Guarantees were then broken on purpose to confirm a named test fails,
with the file restored byte for byte, in the task that added each one: claim equality, the
`SKILL.md` rule, create-only adoption, the review-hash binding, the preview-consumed rule,
the account lock, the offer's bundle limit and its never-stored rule, the rename move and
its collision, the scope merge fence, the typed refusals and the audit events. A closing
pass on the final head broke eight in turn (both baseline blocks, claim equality, the
`SKILL.md` rule, create-only adoption, the review-hash binding, `receipt_changed` and the
rename move) and a named test failed every time; the planner's baseline blocks were
otherwise covered only by their own unit tests before that pass. A broad review of the
storage and adoption range found nothing critical; its API-facing findings (typed 409s and
the receipt-aware delete) were fixed in the API slice, and a second broad review of the API
slice found nothing critical. The full local suite and the published workflows are
recorded in `handoff.md` and on the pull request.
