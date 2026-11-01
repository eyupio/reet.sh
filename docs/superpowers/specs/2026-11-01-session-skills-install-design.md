# Installing a reet-hosted skill from the session picker (Phase 2)

Status: draft for review. Nothing here is built.

Builds on `2026-11-01-session-skills-design.md` (Phase 1, merged). Phase 1 lists reet-hosted skills in the picker but
cannot insert them, because they are not on the session's machine. Phase 2 puts one on the machine, from the picker, so
it becomes a normal native skill that can be inserted.

## Goal

A person with a session open presses **Install** on a published reet-hosted skill, confirms what will be written and where,
and the skill is installed in the session agent's user-level skills folder on that machine. When it finishes, the picker
reloads and the skill appears as a native skill, ready to insert. It works from both the reet TUI and the agent TUI,
because both use the same picker.

## Decisions already made with the user

- **Confirmation is inside the picker.** The picker shows what will be added and where, the person presses Install, then
  sees the machine's previewed plan and presses **Approve and install**. The existing preview-then-approve job runs behind
  it. No trip to another page for the normal case.
- **Only published skills can be installed.** An unpublished hosted skill stays listed and says "Publish this skill to install
  it", with a link. Nothing is made public by this feature.
- **Install goes to the user-level folder** for the session's agent (for Claude, the folder in the home directory), not the
  session's project folder.

## What exists today (checked in the code)

- **The machinery is already there.** `PUT /api/v2/sync/desired` saves the skills a target folder should hold
  (`store.SaveDesiredFor`, `DesiredSelection` with a source of `hosted`, explicit `skillIds` and a `mode`); `POST
  /api/v2/sync/jobs` queues a job for a target (`QueueSyncJob`, kinds `scan`, `preview`, `auto`); the connector on the
  machine computes a plan and reports it with a hash; `PATCH /api/v2/sync/jobs` with `action: "approve"` and that hash turns
  it into an `apply` job, which the connector runs; `action: "cancel"` cancels a queued or awaiting job. `GET /api/v2/sync`
  returns the jobs. The web client already has `getSyncState`, `controlSyncJob` and `queueSyncJob` (`web/src/lib/api.ts`).
- **A plan is a list of actions.** `scan.SyncPlan` has the target, the directory and `Actions`, each with `Add`, `Replace`
  and the skill. A plan hash is what approval checks (`ControlSyncJob`).
- **Applying rescans the machine.** The connector rescans after a successful apply (`rescan` in `internal/scan/sync.go`), so
  the picker's catalogue is fresh without a manual step.
- **Additive mode never removes.** In `additive` mode the connector keeps extras, including local-only skills
  (`internal/scan/sync.go`, the `target.Mode == "additive"` branch). `mirror` mode plans removals.
- **Only published hosted skills are installable.** `SyncHostedFor` skips unpublished ones, and installs come from the skill's
  published URL (`syncSource`).

## The danger this design avoids

`SaveDesiredFor` **replaces** a target's saved skill set with the selection. If the browser called it with one skill, a
machine that already had a saved set (say twenty skills, in mirror mode) would lose its saved set, and the next run would
remove the rest. It also races with other edits through its `revision` check. So the install must not be built from
browser-side calls to the existing endpoints. It is one new server-side operation that reads the current saved set and
merges the new skill into it in one transaction.

## Design

### The new endpoint

`POST /api/v2/terminal/sessions/{id}/skills/install`, registered with `s.requireWriter(s.terminal(...))`-equivalent
protection (accounts-only browser session, and a viewer is refused, as the other sync writers are). Body:
`{ "skill": "reet:<slug>" }`, the `id` of an entry from the Phase 1 catalogue whose `source` is `reet`.

The handler, in order:

1. Finds the session (same 403 as the Phase 1 endpoint for an unknown or foreign session) and its machine and agent.
2. Resolves the **target**: the account's environment on that machine (after machine-alias resolution) for the session's
   agent with no project, that is the agent's user-level skills folder. If there is none, or its scan is not complete and
   recent, or the agent is not one sync supports (`Syncable`), it refuses.
3. Checks the machine's sync connector is enrolled for this account and not paused.
4. Looks up the skill among the account's **published** hosted skills (`SyncHostedFor`); an unpublished or missing one is
   refused.
5. In one transaction, reads the target's saved set, mode and follow-source flag, and:
   - if the target follows another machine (`follow_source`), refuses, because the next sync would remove the skill;
   - if a preview, auto or apply job is already queued, running or awaiting approval for this target: when that job was
     started by an install of **the same skill**, returns it (`reused: true`); when it is anything else, including an
     install of a different skill, refuses with `install-in-progress` and changes nothing. A pending job is tied to the
     saved set's revision (`ClaimSyncJob`, `ControlSyncJob` and the apply result all reject a job whose revision no longer
     matches), so merging a second skill under a waiting job would break that job. The person finishes or cancels the first
     install, then installs the second;
   - otherwise merges the skill into the saved set (the set stays as it was, plus the new skill), keeps the existing mode,
     or creates an `additive` set when the target had none, and queues a **`preview`** job for the target. The job kind is
     always `preview`, never `auto`: `FinishSyncJob` turns an additive `auto` plan straight into a queued apply, which would
     install the skill without the picker's Approve step. A test pins the queued kind.
   The install records, on the job, that it was started by an install of this skill and whether the install created the saved
   set (so Cancel can undo exactly that, below).
6. Writes a sync event (`s.eventRequest`, action `skill-install`) with the session, machine, skill and job id, like other
   sync writes. The skill's content is never logged.

Response: `{ "job": "<id>", "reused": false, "directory": "<folder>", "host": "<machine>" }`.

Errors are `{ "error": "<sentence>", "code": "<code>" }` with one stable code each: `unpublished`, `connector-offline`,
`no-scan`, `unsupported-agent`, `follows-source`, `slug-conflict` (a different skill with the same name is already in the saved list), `install-in-progress` (a different job is pending on this folder),
`already-installed` (the skill is already on this machine for this agent), `read-only`. The sentences are written for the person, for example "dev-laptop's skills follow another machine, so this
skill would be removed the next time it syncs."

The controller adds **no code that writes files on the machine**. Writing is the connector's existing apply.

### The catalogue gains one field

Phase 1's `SessionSkill` gets `published: boolean`. It is true for a machine skill (it is already there) and for a hosted
skill that is published. The picker uses it to show **Install** or "Publish this skill to install it".

### The picker's install states

The picker gains an install panel for one skill at a time. The states, each with its own text and a visible, announced
status line:

1. **Confirm.** "Add `pdf` to the Claude skills folder on dev-laptop. Nothing runs until you approve the plan." Buttons:
   Install, Cancel.
2. **Waiting for the machine.** The job is queued or running a preview. Shows "Waiting for dev-laptop…" and **Cancel**.
   After 60 seconds it adds "Is reet running on dev-laptop?" and keeps waiting. It never cancels by itself.
3. **Review the plan.** The job is awaiting approval. The picker shows the plan's directory and each action in words, for
   example "Add pdf". Buttons: **Approve and install**, Cancel.
   - Inline approval is offered **only when every action is a plain add of the requested skill**. If the plan contains a
     replace (a same-name local copy would be moved to a backup) or any removal, or an action for another skill, the picker
     does not offer Approve. It says "This install would change other skills. Review it on the Sync page." with a link and a
     Cancel button. This keeps the one-press path to the case the person just asked for.
4. **Installing.** After approval, "Installing on dev-laptop…" until the job succeeds or fails.
5. **Done.** The catalogue reloads, the skill appears as a native skill, and focus goes to it. The picker does not insert it
   for the person.
6. **Failed.** The connector's message in a sentence, plus Close. A cancelled job returns to the list.

Progress comes from the existing `getSyncState` polled every two seconds while the panel is open and stopped when it closes
or the job ends. Approve and Cancel use the existing `controlSyncJob`. The only new network call is the install POST.

The install panel works the same in both TUIs and as a bottom sheet on a phone. Closing the picker mid-install does not cancel
the job; the job stays visible in the existing Needs you and Sync surfaces.

### Cancel undoes the merge

Pressing Cancel in the picker must not leave the skill in the saved set, because a later `auto` sync run would then install
it (additive plans are applied without a second approval) although the person cancelled. So Cancel is a second endpoint,
`POST /api/v2/terminal/sessions/{id}/skills/install/cancel` with body `{ "job": "<id>" }`, which in one transaction:

1. cancels the job through the existing cancel rule (only a queued or awaiting-approval job can be cancelled; a running
   one cannot, and the picker says so);
2. if the saved set's revision is still the one this install produced, removes the skill from the saved set, and deletes the
   whole saved set if this install had created it; if someone has changed the set since, leaves it alone and says so.

The same revert runs whenever an install job ends without applying, by any path, not only the picker's Cancel: a cancel from
the Needs you list or the Sync page, a removed saved set, a renamed or revoked machine, a preview or apply the machine reports
as **failed**, and a job the controller fails because the machine stopped reporting. The rule is: a skill an install put in a
saved set stays there only while its job is pending or once it has applied. A set the install created is deleted; otherwise the
slug is removed and the set gets back the revision it had before the install (`skill_installs.prev_revision`), so no new
revision without jobs appears for Keep synced to act on. If the set has changed since, it is left alone.

So a failed install also leaves the saved set as it was, and the picker's Failed state offers Retry, which runs the whole
install again (still one press), and Close. There is no separate remove action.

While an install is pending, Keep synced holds off on that machine (it already waits while any job there is pending) and the
Sync page's Run skips that folder; when no folder is left it refuses with "an install is waiting on this folder; finish or
cancel it first" (or "...on each of these folders..." for several). Saving that folder's set (`SaveDesiredFor`, used by the Sync
page and the library's one-click install) is refused with "An install is waiting on this folder. Finish or cancel it first.",
and merging that folder's report into another cancels the pending install (and reverts it) before the set is copied; a merge
while a job runs there is refused.

### Safety

- Every write to the machine goes through the existing preview, hash approval and apply. A plan the person did not see in the
  picker cannot be approved from it (the hash is the plan the connector reported).
- Only the signed-in account's own published hosted skills can be installed, and only through `SyncHostedFor`.
- Viewers cannot install. The endpoint follows the sibling sync writers' role check.
- The merge never removes: the saved set keeps what it had, and an `additive` set is created only when none existed.
- Only `preview` jobs are queued, so every install shows its plan and waits for the hash-checked Approve.
- A cancelled install leaves no trace in the saved set.
- Names and messages are shown as text only.

## Not in scope

- Installing into a project folder; delivering unpublished skills privately; removing or updating an installed skill;
  per-agent slash syntax; a connector-side "install one skill" job (approach C in the design discussion).

## Testing

- Go (store): the merge keeps the existing saved set and mode and adds the skill; an `additive` set is created when none
  exists; a `follow_source` target is refused with no change; the queued job kind is `preview`; a pending job for the same
  skill is returned (`reused`); a pending job for a different skill is refused with `install-in-progress` and leaves the
  saved set and the pending job untouched; two concurrent installs of different skills on one target end with exactly one
  pending job and a saved set containing only the first; Cancel removes the skill again (and removes a set the install
  created), leaves a set someone else changed, and refuses to cancel a running job.
- Go (handler, in the Phase 1 test style): unknown and foreign sessions get 403; a viewer is refused; each error code is
  returned for its condition (unpublished, offline connector, no or stale scan, unsupported agent, already installed,
  follows source); a machine alias still resolves the target; the success path queues exactly one `preview` job for the user-
  level target and records a `skill-install` event.
- Web (node): the plan classifier (only plain adds of the requested slug are approvable) and the status-line wording per
  state, as pure functions.
- Playwright: the install panel with mocked sync state for each state (confirm, waiting, review, installing, done, failed,
  cancelled, plan with a replace or removal showing the Sync page hand-off, offline hint after 60 seconds), keyboard use,
  focus placement, and the phone sheet; and that no approve call is made without the button.

## Open questions for the plan

1. Where the "Publish this skill" link goes (the page where hosted skills are managed).
2. (Resolved: `preview` only; see the endpoint section.)
3. A hosted skill whose slug is already installed on the machine **for another agent** is, in Phase 1, listed once, under
   "other agents", and not repeated in the reet group (the catalogue drops a hosted skill whose slug is installed). So no
   Install is offered for it, although it is not installed for this session's agent. Either accept that for now, or change
   the Phase 1 de-duplication to repeat the hosted entry when the installed copy is for a different agent. The plan must pick
   one; accepting it is the smaller change.
