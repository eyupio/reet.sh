# Private skills dialog (S4 review UI)

Nowt on screen says more than the machine has told us. The **Private skills** dialog on the
Machines page is the browser face of three APIs that already existed: [pins](skill-content-targets.md),
[negotiated checks](skill-content-preview.md) and [ownership receipts with adoption](skill-content-receipts.md).
From one dialog the signed-in owner can pin a library revision to a machine folder, ask the
machine what it holds, read an honest answer, record ownership of a folder that is already
there, and release that record again. Nothing is written to any machine.

## Purpose and non-goals

The dialog answers four questions, one skill at a time: which library revision should this
folder have, what does the folder hold right now, would reet be allowed to claim what is
there, and does reet currently hold a record of owning it.

It does not apply, install, update, remove or verify anything on a machine, and it does not
dispatch any work except the read-only check. It has no MCP tool, no fleet overview, no bulk
actions and no private-skill editing or publication. Terminal sessions and controls, the
deferred user-controlled security/PIN preference and the legacy sync flows are untouched.
Applying changes belongs to the reviewed executor (see [Continuation](#continuation)).

## Entry points and eligibility

Two buttons open the same dialog (`MachineSkillContent.svelte`) for one machine folder:

- **Private skills**, on every agent scope on the Machines page.
- **Pin a library revision**, beside **Import privately** on each skill a scope reports. It opens the
  dialog with that skill's name used to put the matching library skill first.

Neither button exists for a viewer or when the server is not in accounts mode. Otherwise the
rules in `scopeEligibility` (`web/src/lib/contentReview.ts`) decide, and the first one that
fails shows its plain reason beside a disabled **Private skills** button; the row-level
**Pin a library revision** button is simply absent for a scope that cannot take a pin.

| Rule | Reason shown |
| --- | --- |
| Account can only view | Your account can only view, so it can't pin or check private skills. |
| Scan-only Codex | This agent is scan-only, so private skills can't be pinned to it. |
| Any other agent than shared agents, Claude Code, Cursor or opencode | This agent doesn't support private skills. |
| A project folder for any agent but Claude Code or shared agents | Project folders support private skills for Claude Code and shared agents only. |
| A project path that is not absolute | This project's folder isn't an absolute path, so it can't be used for private skills. |
| No complete scan of the folder in the last 14 days | This machine hasn't reported a complete scan of this folder in the last 14 days. |
| No connector enrolled on the host | No connector is enrolled on this machine, so it can't be checked. |
| Every connector on the host is paused | The connector on this machine is paused. Resume it to check this machine. |

The server decides again on every write; this table only saves the person a refused click.
If the folder stops qualifying while the dialog is open, saving is switched off with the reason
shown, and removing a pin and releasing a record still work.

## The three steps

The dialog has a fixed header and a fixed footer (**Done**), and a body that scrolls. It shows
the folder's existing pins and ownership records, and one skill moves through three steps.

### 1. Pin

The Pin step lists the pins for this folder, each with its revision (named by the date it was
made and marked when it is not the newest in the library), its mode and a line from its latest
check. **Check**, **Change pin** and **Remove pin** sit on each row.

Below it, **Pin a library revision** chooses a library skill (those whose name matches the
reported skill come first, and one is chosen for you only when exactly one matches), a revision
(newest first and chosen first, older ones there for a rollback) and a mode: **Add and update**,
or **Also remove files reet owns that a newer revision drops**. A pin is intent only. It installs
nothing and writes nothing to the machine. Changing or removing a pin sends the pin's current
version, and a version conflict reads the pins again and says what changed. Removing a pin
removes intent only and leaves any ownership record exactly as it was.

### 2. Check

**Check this machine** asks the connector to read file names, sizes and digests of the pinned
skill's folder. It never reads file contents. The result is headed with the presence label,
then shows, for a current result only:

- the physical-folder warning when another agent scope reaches the same folder or that could
  not be told, in which case no file list and no verdict are shown;
- the files found, with **Managed by reet**, **Not managed by reet** or **Not known** (when the
  check carried no plan) against each;
- the planner's verdict in words: **Up to date. The files reet has recorded here match the
  pinned revision.**, a read-only list of what a later apply would create, update or remove
  followed by "Applying changes isn't available yet.", the blocking conflicts, or, for a folder
  nothing owns yet, the planner's `adoption_required` sentence and the Adopt step;
- **Files left alone**, the files that stay yours or are kept because the pin only adds and
  updates.

A check that is out of date, overtaken, expired, stopped or failed shows its reasons and
**Check again**, never a verdict from its plan. A check made for an older version of the pin is
treated as overtaken whatever the server last said, and says which revision it was made for.

### 3. Adopt (Record ownership)

This step appears inside the Check step only while the check is current and adoptable: the
folder is found, in a root no other scope reaches, with a complete listing, and the planner's
only conflict is `adoption_required`. It lists the library revisions whose files match some of
the folder, each with its counts, and nothing is chosen for you unless the offer holds exactly
one revision (and even then the table and the confirmation still follow).

Choosing a revision reads its [file breakdown](#the-breakdown-endpoint) and shows a captioned table:
**Claimed**, **Stays yours**, **Only in the library**, or **Differs from this revision** (a path
whose bytes or mode differ on the machine and in the library is one row, and the screen does not
guess why). **Record ownership** stays disabled until the table for the chosen revision, read for
the review hash on screen, has arrived and at least one file would be claimed. It opens a
confirmation titled "Record ownership of {skill}?" that says what would be recorded, names the
revision, repeats the ownership sentence below and says three things: nothing on the machine is
changed, files not listed as claimed stay yours, and the record can be released later.

Confirming reads the check once more. Nothing is sent unless that read is still the same current,
adoptable check with the same review hash and revision. Then one request goes out, with the hash
exactly as the server gave it. When it succeeds the ownership records are read again, a new check
starts (when the connector is ready to check) so the screen can say what the folder looks like now, and focus moves to the Check step's
heading (the button that asked has gone).

### Ownership records and Release

Below either step, **Ownership records in this folder** lists each record: skill, revision (by
date, or "an older revision" when the library no longer lists it or its list could not be read), file count, when it was
recorded, and the ownership sentence. A record that cannot be read is listed by name only, with
"This record can't be read. It can still be released." and no claims. **Release ownership** asks
first, says it removes the record only (nothing on the machine changes and the files stay where
they are) and sends the record's current version. Afterwards focus goes to the records heading, or
to the dialog's title when no record is left.

## Labels

These are the exact sentences, held in one place (`web/src/lib/contentReview.ts`). A code the
page has no words for reads **Something changed. Check again.** and never enables Adopt.

| Presence | Label |
| --- | --- |
| `present` | Found |
| `absent` | Not there yet |
| `unknown` | Couldn't tell |

| Sentence | Where |
| --- | --- |
| **Recorded by reet from a recent check, not verified on the machine.** | Beside each ownership record that can be read (a record that cannot be read shows no claims), in the confirmation and in the success message. The word appears only to deny it. |
| **Applying changes isn't available yet.** | Under the read-only change list. |
| **Something changed. Check again.** | The fallback for any unrecognised state, reason, code or mode, and when a confirmation finds the check has moved on. |

| Check state | Label (pin row: "Last check: ...") |
| --- | --- |
| `queued` | Waiting for the machine |
| `capturing` | Reading the machine |
| `captured` | Checked |
| `failed` | Couldn't check |
| `cancelled` | Stopped |
| `expired` | Ran out of time |
| `superseded` | Out of date |
| `stale` | Too old |

A captured check that still carries reasons is shown as Out of date on the pin row.

| Reason (why a check is out of date) | Sentence |
| --- | --- |
| `pin_changed` | The pin changed after this check was made. |
| `pin_removed` | The pin was removed after this check was made. |
| `connector_paused` | The connector on this machine is paused. |
| `connector_removed` | The connector on this machine was removed. |
| `process_changed` | The connector restarted or stopped reporting for this folder since the check. |
| `inventory_changed` | The machine reported a newer scan after this check was made. |
| `scope_removed` | The machine no longer reports this folder. |
| `receipt_changed` | The ownership record for this skill changed after this check was made. |
| `expired` | This check ran out of time before it finished. |
| `stale` | This check is more than five minutes old. |

| Failure | Sentence |
| --- | --- |
| `capture_failed` | The machine couldn't read this skill folder. |
| `scope_unavailable` | The connector can't use this folder for private skills. |
| `plan_unavailable` | The folder was read, but the changes couldn't be worked out. If an ownership record is damaged, release it and check again. |

The planner's blocking codes (`adoption_required`, `unmanaged_path`, `local_modified`,
`both_modified`, `local_deleted`, `managed_target_missing`, `baseline_target_mismatch`,
`baseline_bundle_mismatch`, `root_shared`, `root_unknown` and the rest of its closed set) each have a
plain sentence in `PLAN_CODES`. The folder warning for a shared root reads "Another agent scope, or
a link, reaches this same folder, so reet will not touch it." and for an unknown one "Couldn't tell
whether another agent scope reaches this folder, so reet will not touch it." A root that is
exclusive says nothing. The two reasons a file is left alone are "Stays yours: this file isn't in
the pinned revision, so reet leaves it alone." and "Kept: the pinned revision no longer has this
file, but this pin only adds and updates."

## Polling, freshness and fences

All of this is in `checkWatcher.svelte.ts` and the dialog shell, and is held to the dialog's
lifetime.

- **Reading a check that can change.** While a check is queued or capturing it is read every three
  seconds. A captured check is read again every ten seconds while the Check step shows it, so the
  server's `stale` or `superseded` verdict reaches an open Adopt step within seconds. A final state
  (failed, cancelled, expired, superseded, stale) stops the reading.
- **The tab.** Nothing is read while the tab is hidden. Coming back reads at once.
- **Freshness.** The browser records the time each response *arrived* (`readAt`) and never compares
  its clock with the server's. A captured result whose last read was more than 30 seconds ago is
  only "refreshing", with the verdict and Adopt step hidden, until it has been read again. The Check
  step asks again every second, so this happens on screen without a click. If a read fails and
  thirty seconds pass with no good one, an open Adopt step and any confirmation in it close, and the
  person has to choose the revision again: it fails closed rather than letting an old answer stand.
- **Fences.** Each read, and the watcher itself, carries a generation counter and an abort
  controller, and checks that the dialog is alive, the same pin is selected and the Check step is
  open. A late answer for a check the person has left changes nothing. Closing the dialog aborts
  everything.
- **Request keys.** Keys are generated in memory only. A check is requested with one key per pin
  version, and an adoption with one key per check, review hash and revision. After an uncertain
  failure (no reply arrived) the key is kept, the list is read for the result, and the next press
  sends the same request, which the server answers once. Once the server has answered, the key is
  dropped, so a new deliberate press is a new request.
- **Before a check.** The pin is read again, and a pin that has moved or gone means no check is
  made and the screen says what changed.
- **Busy.** The server allows one active check per connector. A busy answer reads the checks again;
  if the active one belongs to a pin in this dialog the button becomes **Resume the check for {skill}**,
  and if it belongs to another folder the screen says to try again when it has finished.
- **Connector readiness.** Checking is switched off, with the reason beside it, until the connector
  has recently reported in (the server's rule is within 90 seconds). While it has not, the dialog reads
  the checks every ten seconds so the button returns by itself.
- **Browser storage.** Nothing about a pin, check or record is put in `localStorage`,
  `sessionStorage` or IndexedDB.

## The breakdown endpoint

The offer carries counts only and the library's revision read returns file bytes, which the browser
must not fetch to draw a table. The Adopt step therefore reads
`GET /api/v2/skill-content-previews/{id}/adoption/{revision}`, which answers file by file with
path, size, digest, executable bit and a status, and nothing else. It is computed on the server
with the same claim function adoption uses, so the screen cannot promise a claim adoption would
refuse. It is described, with its errors, in the
[receipts contract](skill-content-receipts.md#file-breakdown). A refusal (409 or 404) reads "This
revision can't be compared with the folder any more. Check again." The page never calls the
revision-bytes read, and a fixture fails if it does.

## Keyboard and phone behaviour

- The dialog is a native modal `<dialog>`: focus stays inside it, **Escape** closes it, and focus goes
  back to the button that opened it (which is set explicitly, because not every browser focuses a
  button when it is clicked). Escape in a confirmation closes only the confirmation and returns focus
  to the button that asked; neither writes anything.
- After an action removes the control that had focus, focus is placed on a heading in the dialog
  rather than lost: the Check step's heading, the pins heading after a removal, or the records
  heading after a release.
- Progress, outcomes and the breakdown summary are announced through polite live regions, and state is
  always in text as well as colour. Tables are captioned, and each scrolling table container is
  labelled "(scrolls)" and can be focused so a keyboard user can scroll it.
- The dialog is at most 880px wide and fills the window less 14px a side on a phone, with a fixed
  header and **Done** footer and a scrolling body. The file tables scroll inside their own container
  (a 256-file list and a very long unicode path are exercised) so the page itself never scrolls
  sideways. Confirmation buttons stack at full width on narrow screens. With a reduced-motion
  preference nothing in the dialog animates or transitions.

## What is not established

- The browser fixtures use synthetic API responses in Chromium. No physical phone or tablet, screen
  reader, Safari or Firefox check was made; see [UI verification](ui-verification.md) for what was
  run and what was not.
- Nothing is written to or verified on any machine by this dialog, and a check reads only file
  names, sizes and digests. A check is a point-in-time read of a disk that keeps changing.
- **Up to date** means the files reet has recorded match the pinned revision. It does not mean the
  machine is healthy, that the files are intact on disk now, or that anything was installed.
- An ownership record is the controller's attestation, made from a recent check. It is not a
  verification of the disk, and the screen never calls it verified.
- The breakdown does not check the account's 1,024-record limit, so an account at the limit can see
  claims that adoption then refuses with its capacity message, which the screen words in its own way.

## Continuation

The [reviewed executor](skill-content-receipts.md#granular-continuation) (staging, lock,
revalidation against the review hash, journal, rollback and verified `applied` receipts) is next,
and it will reuse this dialog for approval. MCP exposure for pins, checks and records, a fleet
overview, bulk actions and a receipts-aware prune guard stay on the follow-up list in
[handoff.md](../handoff.md).
