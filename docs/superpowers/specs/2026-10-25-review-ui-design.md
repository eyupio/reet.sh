# Review UI for pins, checks and ownership (S4 design)

Date: 2026-10-25. Status: design for review; no code written.

Nowt on screen should say more than the machine has told us. This slice gives the signed-in
owner a browser flow over the three APIs that already exist: pinning a library revision to a
machine scope ([pins](../../skill-content-targets.md)), checking what is on that machine
([negotiated previews](../../skill-content-preview.md)) and recording ownership of what is
already there ([receipts and adoption](../../skill-content-receipts.md)). It writes nothing to
any machine and does not apply anything.

## Intent and success

**Outcome.** A person who has imported or created private skills can, from the Machines
page, say "this machine should have this library revision", ask the machine what it holds,
read an honest answer, and, if a matching folder is already there, bring it under reet's
ownership record by an explicit, reviewed decision. They can also release that record.

**Who it is for.** The account owner with a write role, on a desktop or a phone.

**Success looks like.**

- Every state the server can report has a plain, tested sentence, and an unknown state falls
  back to "check again", never to a reassuring one.
- No one can adopt without seeing which files would be claimed and which stay theirs.
- The screen never implies a file was verified on disk, installed, or will be changed.
- Nothing about a pin, check or ownership record is kept in browser storage.

## Decisions already taken with the product owner

| Question | Decision |
| --- | --- |
| How much does the first slice cover? | Pin to adopt, end to end, for one skill on one machine. Pin, check, review, adopt, release. |
| Where does it live? | A scope dialog on the Machines page, opened from an agent scope and from a reported skill's row beside "Import privately". |
| How deliberate is adoption? | Choose the library revision (preselected only when the offer holds exactly one), review the claimed and unmanaged files, then confirm in a dialog. |
| How is it built? | A dialog shell, three step components and a pure, tested logic module. |

## Out of scope

Applying changes (the reviewed executor), MCP exposure of pins, checks or receipts, a
fleet-wide overview page, bulk actions, private skill editing, publication, and any change to
terminals, the PIN/security preference or the legacy sync flows.

## The flow

Entry points. Each supported agent scope on Machines gets a **Private skills** action. Each
reported skill row in the scope's content list gets **Pin a library revision** beside
**Import privately**. Both open `MachineSkillContent.svelte` for that machine and scope, the
second with the skill name preselected. For viewers and legacy auth modes the action is absent.
For scan-only Codex, a scope without a complete scan in the last 14 days, and an unenrolled or
paused connector it is disabled, with the plain reason beside it.

The dialog shows the scope's existing pins, each with its mode, selected revision, and a
status line from the latest check. One skill at a time moves through three steps.

1. **Pin** (`ContentPinStep.svelte`). Choose a library skill (bundles whose name matches the
   reported skill come first) and a revision (newest preselected, older ones available for
   rollback). Choose a mode: *Add and update* (default) or *Also remove files reet owns that a
   newer revision drops*. The copy says a pin is intent only and installs nothing. Saving sends
   the pin's current version when changing one; removing a pin removes intent only and leaves
   any ownership record in place.
2. **Check** (`ContentCheckStep.svelte`). *Check this machine* reads file names, sizes and
   digests only. The result reads *Found*, *Not there yet* or *Couldn't tell*, with the
   physical-folder warning where the connector reports a shared or unknowable root, the files
   found (managed and not), and the planner's verdict in words. A stale, superseded, expired,
   failed or cancelled check shows its reason and offers *Check again*.
3. **Adopt** (`ContentAdoptStep.svelte`). Offered only when the folder is found and nothing is
   recorded for it. The adoption offer's candidates are listed with their match counts. The
   person picks one (nothing is preselected unless the offer holds exactly one candidate, and
   even then the breakdown and the confirmation still follow), the file breakdown for that
   revision appears (*claimed*, *stays yours*, *only in the library*), and **Record
   ownership** opens a confirmation. The confirmation says: ownership is recorded from a check
   made a few minutes ago; nothing on the machine is changed or verified; files not listed as
   claimed stay yours; and the record can be released.

After adoption the check repeats. If the folder matches the pin the verdict is **Up to date**.
Otherwise the changes are listed read-only (*create*, *update*, *remove*, and the files that
stay local) with the notice "Applying changes isn't available yet." Each ownership record has
**Release ownership**, with a confirmation that it removes the record only.

### Words

Presence: `present` is *Found*, `absent` is *Not there yet*, `unknown` is *Couldn't tell*.
Ownership of the folder: `exclusive` shows nothing; `shared` says another agent scope reaches the
same folder, so reet will not touch it; `unknown` says it could not tell. The `controller_attested`
label reads **Recorded by reet from a recent check, not verified on the machine.** The word
"verified" never describes an ownership record.

Check states and reasons, planner block codes (`adoption_required`, `baseline_target_mismatch`,
`unmanaged_path`, and the rest of the planner's closed set), failure codes (`capture_failed`,
`scope_unavailable`, `plan_unavailable`), fence reasons (`pin_changed`, `receipt_changed`,
`connector_paused`, `process_changed`, `inventory_changed`, `scope_removed`, `expired`, `stale`)
and API refusals each get one plain sentence in `contentReview.ts`. Anything not in the table
falls back to "Something changed. Check again."

Voice. At most one light Yorkshire phrase in a heading, lede or success message, as the voice
guide allows. Confirmations, errors, state labels and ownership language stay exact.

## The one server addition

The adoption offer carries counts only, and the library's revision read returns file bytes,
which the browser must not fetch to draw a table. Add

`GET /api/v2/skill-content-previews/{id}/adoption/{revision}`

returning, for a captured, adoptable check and a revision of the pin's own bundle,

```json
{
  "revisionId": "…",
  "files": [
    {"path": "SKILL.md", "bytes": 120, "digest": "…", "executable": false, "status": "claimed"},
    {"path": "notes/mine.txt", "bytes": 12, "digest": "…", "executable": false, "status": "stays_yours"},
    {"path": "scripts/run.sh", "bytes": 80, "digest": "…", "executable": true, "status": "library_only"}
  ],
  "matched": 1, "observedOnly": 1, "revisionOnly": 1
}
```

A path whose bytes or mode differ between the machine and the library appears twice, once as
`stays_yours` (the machine's copy) and once as `library_only` (the library's copy), and the
browser shows such a pair as one row, "edited on this machine".

It is metadata only (no bytes, no private text), computed on the server from the stored check
and the stored revision with the same claim function adoption uses, so the screen cannot
promise a claim adoption would refuse. A revision whose `SKILL.md` does not match answers 409
with the same message as adoption. It sits behind `privateSkillAccess` with the preview
routes' boundary, is `no-store`, is not in the MCP allowlist, and writes no audit event (it is
a read). Files are sorted by path and bounded by the 256-file manifest limit. The store method
is `SkillContentAdoptionBreakdownFor(uid, previewID, revisionID)` beside the offer code in
`store_content_receipts.go`, reusing the offer's bundle and account checks. Another bundle,
another account, a preview that is not captured or adoptable, and a damaged revision all
return the errors adoption would return.

## Browser structure

- `web/src/lib/api.ts`: types and calls for pins, checks, ownership records and the breakdown.
  Wrappers only; no logic.
- `web/src/lib/contentReview.ts` (+ `contentReview.test.mjs`, added to `npm test`): pure
  functions for scope eligibility, the label and reason tables, check-state classification,
  plan summary (up to date, changes, conflicts), adoption candidate ordering and claim summary,
  and the polling decision. This is where "what is true now" lives.
- `web/src/lib/MachineSkillContent.svelte`: the dialog shell. Holds the selected skill, the
  current pin, the active check and the list of ownership records; owns polling and the
  confirmation dialogs; renders one step at a time.
- `ContentPinStep.svelte`, `ContentCheckStep.svelte`, `ContentAdoptStep.svelte`: presentational
  steps taking data and callbacks.
- `MachineFleet.svelte` and `ContentObservation.svelte`: the two entry points, mirroring how
  `MachineSkillImports` is opened.

Client lifecycle rules (mirroring the import dialog). Request keys stay in memory and are
reused after an uncertain create failure. Polling runs every three seconds while a check is
queued or reading, pauses while the tab is hidden, stops on any final state, and is fenced by
the dialog's lifetime and the selected check with abort controllers and a generation counter.
One active check per machine: a busy answer becomes *Resume that check*. Every pin change and
removal re-reads the pin and sends its current version; a version conflict reloads and says what
changed. The review hash and receipt versions are opaque and are only passed back unchanged. The
revision read that returns bytes is never called.

## Safety, accessibility and layout

- Accounts mode only, hidden for legacy auth, read-only for viewers (the APIs refuse them too).
- No pin, check or ownership data in `localStorage`, `sessionStorage` or IndexedDB.
- Native `<dialog>` with focus returned to its opener, a polite live region for check
  progress, captioned tables, keyboard-only operation, status shown in text as well as colour,
  reduced motion respected.
- Fixed header and action footer with a scrolling body, no horizontal page scroll at 320, 375,
  768 and 1440px, in light and dark.
- Confirmations say what the action does not do. Release removes a record only; Remove pin
  removes intent only.

## Testing

- Node unit tests for `contentReview.ts`: every table row, the unknown fallback, eligibility
  edge cases, candidate ordering, the preselect-only-when-unambiguous rule, claim summaries, and
  the polling decision.
- Go tests with real SQLite and HTTP for the breakdown endpoint: the preview routes' boundary
  table, another bundle or account, non-captured and non-adoptable checks, a damaged revision,
  a `SKILL.md` mismatch, metadata-only responses, and that the file statuses agree with what
  adoption then claims. Mutation checks include dropping the bundle filter and computing the
  breakdown without the claim function.
- A Playwright fixture (`web/tests/skill-content.e2e.mjs`, run beside the import fixture) with
  synthetic API responses: pin create, change and removal; check queued to captured; stale,
  superseded, expired and lost-reply paths; adopt with the file table and confirmation; release;
  viewer; unsupported and unenrolled scopes; phone and desktop in light and dark; axe scans at
  375 and 1440px. The UI's key guards are mutation-checked: the preselect rule, the hash is passed
  back unchanged, the version is sent on every write, and no bytes endpoint is called.

## Delivery

Three reviewable pull requests, each merged when its checks are green, with a broad review
before the last merges.

1. The breakdown endpoint: store method, route, tests, mutation checks, and a short addition to
   `docs/skill-content-receipts.md`.
2. The client API, `contentReview.ts` and its tests, the dialog shell, the Pin step and both entry
   points.
3. The Check, Adopt and Release steps, the browser fixture, and the docs (a new
   `docs/skill-content-review-ui.md`, README and handoff).

## What this does not establish

Nothing is written to or verified on any machine, and the UI says so. A check is a point-in-time
read, an ownership record is the controller's attestation, and "Up to date" means "the recorded
files match the pinned revision", not "the machine is healthy". No physical device, screen reader,
Safari or Firefox check is planned beyond the existing Chromium fixtures. Terminal sessions and
controls and the deferred user-controlled security/PIN preference are untouched.

## Continuation

The reviewed executor (staging, lock, revalidation against the review hash, journal, rollback and
verified `applied` receipts) is next, and it will reuse this dialog for approval. MCP exposure, a
fleet overview, bulk actions and a receipts-aware prune guard stay on the follow-up list in
`handoff.md`.
