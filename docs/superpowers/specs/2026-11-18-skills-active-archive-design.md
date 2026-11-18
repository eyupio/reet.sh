# Skills: Active, Archived, Deleted, and machines that match

Date: 18 November 2026. Status: awaiting the owner's approval.

## Why

The owner's words: the UX of skills "still isn't right". A person "needs to be able to archive or delete skills in
reet.sh that then reflect on the machines and within reet.sh", and the flow "needs to be super simple; we have over
complicated it and over engineered it". Their example: pressing **Stop using** sends the person to the **To test**
section, so they have to find their way back before pruning the next skill.

What is true today (18 November 2026, `9c03201`):

- **The shelf stages do nothing to machines.** In use, To test and Parked are labels. Every library skill goes to
  every machine whatever its stage: `SyncCandidatesFor` (`internal/store/store_sync_control.go:51`) lists the whole
  shelf with no stage filter. On the owner's instance 118 of 139 skills are In use, 13 To test, 8 Parked.
- **Stop using lands in To test.** `stageOf` (`web/src/lib/shelf.ts`) reads `used` before `bookmarked`, and **Mark in
  use** never clears `bookmarked`, so **Stop using** drops the skill back into To test. `refocus`
  (`web/src/App.svelte:677`) then moves focus to the card's new place, which scrolls the page away.
- **Remove does not reach the machines.** **Remove from library** puts the skill on the scanner's ignore list; the
  files stay. On the owner's instance five removed skills (`microsoft-foundry`, `improve-animations`,
  `ruthless-ux-ui-designer`, …) are still in Elitebook's folders.
- **Taking a skill off a machine is six steps:** Remove… → Advanced sync → Exact set → Save → Preview → Approve
  (`docs/superpowers/specs/2026-11-02-skill-remove-from-machine.md`).
- **The engine carries more than the product needs:** per-machine sync plans, lists as sync sources, additive,
  mirror, bidirectional and managed modes, exact sets, previews, approvals and Advanced sync.

## The rule

**reet.sh is the source of truth, and every connected machine matches it.**

## Goals

1. A skill is **Active** or **Archived**, or it is **Deleted**.
   - **Active:** on every connected machine, in every supported agent's home skills folder.
   - **Archived:** kept in reet.sh and taken off every machine.
   - **Deleted:** gone from reet.sh and from every machine.
2. Pruning keeps the person's place. Archive, Delete and Restore never change the tab, never scroll the page and
   never open another page.
3. A skill found on a machine that reet.sh did not put there is asked about. It is never spread to other machines,
   and never removed, without the person saying so.
4. Archive and Delete reach the machines with no review step. Undo covers mistakes, and each machine keeps a backup
   of whatever it removes.
5. The sync engine is small enough to explain in one paragraph (see *The reconcile loop*).

## Not goals

- **The public skills and lists directory** ("first-class, like skills.sh"). The owner chose to do it next, as its
  own project, starting from what Community already does. Nothing here should make it harder.
- **Per-machine or per-agent choices.** No skill is on some machines and not others.
- **Project folders inside repositories.** reet manages agents' home skill folders only. Project folders belong to
  git and are neither scanned for new skills nor written.
- **Changes to writing a skill, publishing, the session skill picker or Setups**, except that each must keep working
  once the old engine is gone (see *What is deleted*).
- **A classic view or a switch back.** The old Skills page and sync pages are replaced outright; going back is
  reverting the change.

## What would make it a failure

- Any action on the Skills page that switches tab, scrolls the page, or navigates away.
- A skill reet.sh shows as Archived still sitting on a machine with nothing on the page saying so (for example,
  *Removing from ThinkPad when it's back online*).
- Files removed from a machine with no backup.
- A skill reaching every machine before the person pressed Keep.
- Two places that can disagree about where a skill should be.
- The upgrade removing anything from a machine except the skills that were Parked or already removed.

## The owner's decisions

| Question | Decision |
| --- | --- |
| What a skill's state means | reet and machines in sync: **Active**, **Archived**, **Delete**. In use / To test / Parked go. |
| Where an Active skill goes | **Every machine, always.** No per-machine choice in the main UI. |
| A skill on a machine that reet does not know | **Ask first.** It waits as *New on a machine* until the person presses Keep or Archive. |
| How Archive behaves | **Instant, with Undo.** No dialog; the machine backs up what it removes. |
| Where Delete is offered | **On every card**, beside Archive. |
| What stays on the Skills page | Search, Active/Archived, Archive/Delete, **bulk select, tags, likes and notes, lists**. |
| Public skills and lists | A **separate project, after this one**. |
| Advanced sync, per-machine plans, exact sets, Preview → Approve | **Removed**, UI and code. |
| How today's stages map | In use and To test → **Active**; Parked → **Archived**. |
| How to build it | **Replace the engine with a plain reconcile loop** (approach B below). |
| How a public skill's files get into reet | **The controller fetches them from GitHub** once, as a bundle every machine installs. |
| A skill edited on a machine | **reet's copy wins.** The edit is backed up and replaced, with **Use this version** to adopt it. |

## Approaches considered

- **A. Keep the plan/preview/apply engine and cut around it.** Active/Archived on the existing status column, removals
  of Archived skills approved automatically, the agent taught one new move (back up and remove a named skill), then
  Advanced sync, per-machine plans and exact sets deleted. Medium size, and it keeps the tested content checks,
  private revisions and conflict detection. *Not chosen:* it leaves the machinery the owner calls over-engineered in
  place underneath.
- **B. Replace the engine with a plain reconcile loop.** *Chosen by the owner.* The controller tells each machine
  which skills are Active (with their content) and which are Archived. The agent installs, backs up and removes,
  and reports anything else as new. The plan/preview/approve machinery is deleted. It gives the simplest end state,
  at the cost of rewriting the most-tested part of reet and changing the agent the most.
- **C. New page now, with the engine untouched underneath.** Archive would be implemented as an auto-approved
  exact-set job. *Dropped:* an exact set removes every unknown skill, so *New on a machine* skills would be wiped
  before the person decides. It also cannot remove the five skills already on Elitebook, which reet never installed.

## States and what moves a skill between them

| From | Action | To | On the machines |
| --- | --- | --- | --- |
| (nothing) | Paste a link, write a skill, or **Keep** a new one | Active | Installed on every machine |
| Active | **Archive** | Archived | Backed up and removed everywhere |
| Archived | **Restore** | Active | Installed on every machine again |
| Active or Archived | **Delete** | gone | Backed up and removed everywhere |
| New on a machine | **Keep** | Active | reet keeps that machine's copy as the skill's content, then installs it everywhere |
| New on a machine | **Archive** | Archived | Backed up and removed from that machine |
| Archived | Turns up on a machine again | Archived | Backed up and removed again; noted in activity |
| Deleted | Turns up on a machine again | New on a machine | Left alone until the person decides |

Calls made in the conversation, each the designer's rather than the owner's:

- **Archive remembers; Delete forgets.** That is why an archived skill is removed again if it reappears, while a
  deleted one is asked about again.
- **Delete is instant with Undo too.** The one exception is a skill the person wrote themselves: Delete asks once,
  because their writing and its public link go with it.
- **Undo after the machines have already removed the files** reinstalls from reet's copy. The machine backup is the
  safety net for a copy that was edited on that machine.
- **Offline and paused machines.** An offline machine catches up when it is next seen, and the card says so. A
  machine whose sync is paused is left alone and shown as paused.

## The Skills page

```
Skills                                       [Paste a skills.sh link…] [Add]

  ⚠ 2 new skills found on Elitebook: paseo-help, cq                 [Review]

  [Active 126]  [Archived 13]          Search…   Tags ▾  ♥  Lists ▾  Sort ▾  [Select]

  seo-audit      Audit a site's SEO      ✓ 3 machines       ♡  [Archive] [Delete]
  seo-bing       Bing Webmaster tools    ✓ 3 machines       ♡  [Archive] [Delete]   ← Archive pressed
  seo-cluster    Keyword clustering      ✗ failed on ThinkPad · Retry   [Archive] [Delete]
  ──────────────────────────────────────────────────────────────────────
  Archived seo-bing · removing from 3 machines                               [Undo]
```

- **Two tabs, Active and Archived**, each with its count. An archived card offers **Restore** and **Delete**.
- **Pruning in place.** The card leaves the list where it is, and focus moves to the next card (or the previous one
  if it was last). The tab, scroll position and filters stay as they were. A toast names the skill and the
  machines, with **Undo**. In bulk, it names the number of skills.
- **Machine health replaces machine choices.** A card says *✓ 3 machines*, *Installing on 2 of 3*, *Removing from
  ThinkPad when it's back online*, or *✗ failed on ThinkPad* with **Retry**. It never offers a choice of machines.
- **New on a machine** is one banner above the tabs, naming the machine and the skills, and opening a short review
  of the new skills: each with **Keep** and **Archive**, plus **Keep all**. Home's *Waiting for you* lists the same
  item.
- **Kept:** search, sort, tags, likes, notes, lists (as a filter and as *Add to list*), bulk select (Archive,
  Delete, Restore, Tag), adding by link, Copy install command and Copy prompt.
- **Gone:** the In use / To test / Parked stages and their tabs, Board view, the *Set to sync to…* line, per-machine
  Add now and Remove…, the install-location, *Only on one machine* and sync-state filters, and Link source to sync.
- **The session skill picker** offers Active skills only.

## Machines and the Sync page

- **Sync becomes one sentence and recent activity:** *Everything is in sync*, *Installing on 2 machines…*, or *1
  machine needs a look: failed on ThinkPad · Retry*, with what each machine last did beneath it.
- **Each machine's settings keep one switch, Pause sync.** It is the only per-machine sync choice.
- **Advanced sync, saved sync plans, exact sets, Preview → Approve, Sync to machines… and lists as sync sources are
  removed**, along with their pages, links and API.

## The reconcile loop

In one paragraph: on every poll the controller tells a machine which skills are Active, with the content revision
and digest of each, and which names are Archived or being deleted. The machine makes each managed folder match:
it installs or replaces what is missing or different, backs up and removes what must go, and reports anything else
as unknown. It then reports what it did, skill by skill. Nothing is planned, previewed or approved.

### What stays from today

- **Enrollment, identity and the poll.** The `sync_agents` table, enrollment, the agent's headers and secret, and
  the 5-second loop (`internal/scan/agent.go:486`, `agentCycle` 582-781) stay. So does `POST /api/v2/sync/poll` and
  its server side (`internal/api/sync_control.go:347-369`), with the job dispatch replaced by the desired state
  below.
- **Binary updates.** These run as `sync_jobs` rows with kind `update` (`store_sync_runs.go:371`,
  `store_sync_updates.go`) and are untouched. `sync_jobs` stays for them alone.
- **The set-server identity proof** (`internal/syncproof`, `/sync/agent-proof`) and the fence (`/sync/moved`) are
  not part of skill sync and stay.
- **Pause.** `sync_agents.paused` and its `PATCH /api/v2/sync/agents` control stay. A paused machine does no
  skill work, as today. `keep_synced` goes: every machine follows the library unless it is paused.
- **Local consent.** `reet agent allow-apply on|off` stays (`internal/scan/agent_apply.go`), still checked before
  every write, because the controller cannot grant itself access to a machine's files. A machine with it off shows
  *Can't write skills here* with the command to run. The installer keeps turning it on.
- **Content as bundles.** Private revisions, bundles and the bundle fetch (`GET
  /api/v2/skill-bundles/{b}/revisions/{r}`, digest checked on the machine in `internal/scan/private_sync.go:33-76`)
  stay, along with the staged install (`installPrivateSkill`, `private_sync.go:80-120`).
- **Inventory scans** (`reet scan --prune`, `POST /api/import`) stay for what the session skill picker and machine
  pages show. They stop creating library items (see *New on a machine*).
- **Machine captures** (`internal/scan/remote_import.go`, `/sync/skill-import/*`). These are how **Keep** gets a
  new skill's files from the machine it was found on.

### What the controller sends

The poll response gains one object, sent whenever its version differs from the one the machine last reported:

```
desired: {
  version: "<hash of everything below>",
  active:   [{ name, item, revision, bundle, digest }],
  remove:   [{ name, reason: "archived" | "deleted" }],
  hold:     false
}
```

- **One content path.** Every Active skill reaches machines as a reet bundle with a digest, written skills and
  public ones alike. The agent stops running `npx skills add` (`internal/scan/sync.go:1280-1290`). The controller
  fetches public skills itself (see *Public skill files and edits made on a machine*).
- **Names are the identity on disk.** Active and removed names never overlap, and no two library items share a name
  (see *The upgrade* for today's duplicates).
- **A short grace period.** A status change waits 10 seconds before it is sent, so an Undo pressed in that window
  costs the machines nothing. This is a designer's call.

### What the machine does

For each managed folder that exists on the machine (`~/.agents/skills`, `~/.claude/skills`, `~/.cursor/skills`,
`~/.config/opencode/skills`, from `homeDirs`, `internal/scan/scan.go:43-49`), with allow-apply on and the machine
not paused:

1. **Missing Active skill:** fetch the bundle, check the digest, stage it and rename it into place. Cursor and
   OpenCode keep today's arrangement of linking to the `~/.agents/skills` copy (`linkSharedInstall`,
   `sync.go:1170-1180, 1386-1407`).
2. **Active skill present with a different digest:** back it up, then install reet's revision. A different digest
   means either reet has a newer revision, or the copy was edited on the machine. When the machine's copy matches
   no revision reet has sent it, the report marks it `edited`, so the activity can offer **Use this version**.
3. **A removed name present:** back it up and remove it, whether or not reet installed it, and whether or not it was
   edited.
4. **Any other skill folder:** leave it alone and report it as unknown.
5. **Report**, one entry per skill and folder: `installed`, `removed`, `failed` with a reason, or `unknown` with its
   digest. Then report the desired version it reached. A failure stops only that skill; the rest go ahead.

Backups reuse `backupSkillEntry` (`sync.go:1409-1435`). They go to a single backups folder beside each skills
folder, named by date and skill, and are kept for 30 days. The agent prunes older ones; today nothing prunes them.
`reet agent restore` lists the backups and restores one by name, without needing a sync target. The retention
period is a designer's call.

The managed folders are the four above. Project folders, `.codex/skills` (scanned but never synced today), and a
custom `CLAUDE_CONFIG_DIR` or `XDG_CONFIG_HOME` (refused today, `sync.go:737-746`) are left alone.

### What the controller keeps

- **One row per machine, folder and skill:** state, digest, detail and time. Each skill's health line, each
  machine's Sync row and the *New on a machine* list are all read from it. Nothing else records sync progress.
- **New on a machine.** An `unknown` report whose name is neither Active nor removed becomes a pending item for
  that name, listing the machines it was found on.
  - **Keep** captures the files from the first machine listed, using the existing capture, and creates an Active
    library item from them.
  - If the copies on different machines have different digests, the review asks which machine's copy to keep.
  - **Archive** creates an Archived item, so the name is removed everywhere it was found.
- **Deleting.** Delete marks the item deleted and keeps its name on the removal list. Each machine's report
  confirms the removal. Once every unpaused machine has confirmed, or 30 days have passed, the name is forgotten,
  so a later sighting is *New on a machine* again.
- **Health on the Skills page:**
  - *On all N machines*: every unpaused machine has reported it installed at the current revision.
  - *Installing on 2 of 3* or *Coming off 3 machines*: some machines are still working on it.
  - *Failed on ThinkPad* with **Retry**: a machine reported `failed`. Retry clears the failure, and the machine tries
    again on its next poll.
  - *Removing from studio when it's back online*: the machine hasn't been seen since the change.

### Older agents

The agent sends a new header, `X-Reet-Skills-Protocol: 2`. A machine without it receives no skill work. It shows
*Update reet on ThinkPad to keep its skills in sync*, and the existing update job is offered. The controller,
embedded UI and agent binary ship in the same release.

### Places that used the old engine and need a new path

- **The session skill picker** installs a hosted skill by queuing a preview job and approving its hash
  (`internal/api/terminal_skills_install.go`, `store_skill_install.go:121`, `web/src/lib/terminal/SkillInstallPanel.svelte`).
  It now shows the files, as today, then **Add to your skills**, which makes the skill Active everywhere. It lists
  Active skills only.
- **Written skills** stay library items whose content is their private revision (`store_hosted_sync.go`). Saving
  creates a new revision, which reaches every machine. A new written skill starts Active.
- **Sync all machines** (`POST /api/v2/sync/library`) goes. There is nothing to start: machines follow the library.
- **MCP tools.**
  - `reet_list_skills` and `reet_update_skill` take `active` / `archived`.
  - `reet_remove_skill` deletes.
  - `reet_list_ignored` and `reet_unignore_skill` are replaced by listing Archived skills and restoring them.
  - `reet_list_sync_targets` goes.
  - `reet_merge_machines` no longer moves sync plans.
  - These live in `internal/api/mcp_server.go` and `mcp/src/server.ts`.

## The upgrade

- In use and To test become **Active**. Parked becomes **Archived**. Skills on the ignore list with reason
  *removed* become **Archived**, so the five left on Elitebook are taken off.
- A one-time notice on the Skills page: *8 Parked skills are now Archived and were taken off your machines.*
  **Restore all**.
- Likes, notes, tags and lists are kept.
- Saved per-machine plans, custom folder plans, lists used as sync sources and pending sync jobs are dropped. Every
  machine follows the library from then on.
- Skills already on a machine that are not in the library appear as **New on a machine**.
- **The ignore list** (`ignored`, `store.go:248`; enforced only at `api.go:1581`) becomes Archived items. An ignored
  ID with no library item becomes an Archived item with a name and no content. The table then goes.
- **Duplicate names** that exist today (two linked items with the same name, `docs/skills-single-source.md`) have to
  become one item per name. The In use copy wins, else the most recently updated one. Tags, notes, likes and list
  membership move onto it, and the other copies are deleted without touching machines.
- **Statuses.** Today's `in_use` and `to_test` become `active`; `on_shelf` and `disabled` become `archived`. Whether
  the column is renamed or the values are reused is the builder's call, but the `used` and `bookmarked` flags and
  the triggers that mirror them (`store_canonical_library.go:145-161`) go.
- Each machine's first poll after the upgrade gets the full desired state. The upgrade itself writes nothing to
  machines: the removals of Parked and previously removed skills happen through the loop, with backups.

## What is deleted

The owner chose removal, not hiding. The engine map behind this list was made on `9c03201`. The builder confirms each
item has no other caller before deleting it.

**Go**
- `internal/skillplan` and `internal/skillpreview`: the content planner and preview capture.
- In `internal/scan`, these go: the `reet sync` executor in `sync.go`, except `backupSkillEntry`,
  `linkSharedInstall` and the source helpers the loop reuses; `sync_report.go`; job dispatch in `agent.go` (662-781),
  replaced by the loop; `content_preview.go`; `library_edits.go`, `library_removals.go` and `library_restore.go`.
- In `internal/api`, these go: the engine routes in `sync_control.go` (desired, reconcile, adopt, run, jobs,
  library, dedupe, source-conflicts, candidates), the sync-target parts of `sync.go`, `canonical_library.go`'s
  selection, deployment-intent, install-evidence, deletion and restore routes, and `content_previews.go`,
  `content_receipts.go` and `content_targets.go`. Enroll, identity, poll, update, proof and moved stay.
- In `internal/store`, these go: `store_sync_automation.go`, `store_sync_held.go`, `store_sync_library.go`,
  `store_sync_progress.go`, `store_sync_source_fix.go`, `store_selected_library.go`, `store_canonical_selections.go`,
  `store_skill_install.go`, `store_library_{deployment_intent,install_evidence,removal_fence,restore,differences}.go`
  and `store_content_{targets,previews,receipts}.go`, plus the engine parts of `store_sync.go`,
  `store_sync_control.go`, `store_sync_runs.go` and `store_private_sync.go`. Inventory, `sync_agents`, updates,
  hosted sync, private skills and captures stay.
- The `REET_LIBRARY_MANAGED_REMOVALS` flag (`cmd/reet/main.go:292`) goes. Removal is no longer behind a flag.

**Tables:** `sync_targets`, `sync_options`, `sync_policies`, `sync_runs`, `sync_run_jobs`, `sync_receipts`,
`skill_installs`, `sync_library`, `canonical_library_selections`, `library_selections`, `library_selection_accounts`,
`library_bases`, `library_connectors`, `library_difference_choices`, the `skill_content_*` tables and `ignored`. The
`library_intent_fence` trigger and `library_edit_events` also go. `sync_jobs` stays for binary updates.

**Web**
- `SyncControl.svelte` (the Sync page and Advanced sync) is replaced by the new Sync page.
- These go: `SyncSetup.svelte`, `MachineSkillSync.svelte`, `MachineAddSkill.svelte`, `SelectedLibrary.svelte`,
  `MachineLibrary.svelte`, `SourceFix.svelte`, `MachineSkillContent.svelte` with its `Content*Step` components,
  `ContentReceipts.svelte`, `ContentObservation.svelte` and `Library.svelte`'s stage controls.
- The activity dock (`SyncActivity.svelte`, `syncActivity.ts`) goes. Its job is done by the toast and the Sync
  page's activity.
- These helpers go: `syncSelection.ts`, `syncActions.ts`, `skillSync.ts`, `librarySync.ts`, `fleetSync.ts`,
  `syncRoutes.ts`, `machineItems.ts`, the stage parts of `shelf.ts`, and `App.svelte`'s Add now, Sync all and
  stage code (895-949, 1349).
- These are reworked: `syncOverview.ts` (also used by `MachineFleet.svelte`), `syncProblems.ts`, and
  `FleetActions.svelte` / `fleet.ts`. The builder checks whether the fleet ones are skill sync or software updates;
  the second stays.
- `contentReview.ts` is shared widely and stays unless nothing is left using it.

**CLI:** `reet sync` and `reet agent library-import` go; *New on a machine* replaces the import. `reet agent
restore` is reworked as above. `reet agent allow-apply`, `reet scan --prune` and `reet agent enroll --allow-apply`
stay. The builder checks `reet agent skill` and `reet agent status --remote`.

**Docs:** these are rewritten for the new model: `docs/skills-machines-ux.md`, `docs/sync-skills-between-machines.md`,
`docs/controller-sync.md`, `docs/skills-single-source.md`, the `docs/skill-content-*.md` pages, and the README's
Skills, Sync and *Adding skills* sections. The superseded specs in `docs/superpowers/specs/` stay as history.

## Public skill files and edits made on a machine

Both are the owner's decisions, made after the engine map.

- **The controller fetches public skills from GitHub.** When a skill is added by link, or refreshed, the controller
  downloads its folder once and stores it as a private revision, a bundle with a digest. Every machine installs
  those bytes. Machines no longer need Node or `npx`. This is a new outbound fetch from the controller; today it
  only reads skills.sh pages. The README's *Nothing is sent to skills.sh…* line and `SECURITY.md` say so.
  - **Size and safety limits** are today's private-bundle limits: 2 MiB, 100 supporting files, and the
    credential-path and key-material checks in `internal/skillimport`.
  - **Failures.** A skill that cannot be fetched (repository gone, too large, rate limited) is added with a visible
    *Couldn't fetch the files · Retry* line, and it reaches no machine until it can be fetched.
  - **Refresh** fetches again. If the files changed, it makes a new revision, which reaches every machine.
- **reet's copy wins over an edit made on a machine.** When an Active skill's copy on a machine differs from reet's
  revision, the machine backs it up and installs reet's revision.
  - The activity says *askme was edited on ThinkPad; reet's copy put back*, with **Use this version**.
  - **Use this version** captures the backup, using the existing capture, as a new revision, and every machine gets
    it.
  - Edits never flow back automatically, so `library_edits.go`, `library_bases`, `library_edit_events` and the
    Differences view go.

## Left to the builder

- **How the work is split into pull requests.** The controller, UI and agent must ship in one release.
- **The API shapes for the page:** bulk status change, delete, undo, and the per-machine report rows.
- **Storage:** whether statuses get new names (see *The upgrade*), and the schema of the per-machine report table.
- **Durations:** the toast and grace-period lengths (about 8 and 10 seconds).
- **Focus:** how focus moves after a removal, within the rule in *The Skills page*.
- **Copy and layout** within the app's tokens, fonts, 14 px type floor and 44 px targets, following
  `docs/voice-and-copy.md`.
- **Wording:** Home's *Waiting for you* entry for new skills.

## Testing

Each behaviour below has a test that fails without the change. The engine's own tests go with the engine: the
`internal/scan`, `internal/api` and `internal/store` sync, library, content and selection tests, `skillplan` and
`skillpreview`, the web unit tests for the deleted helpers, and the e2e specs `machine-skill-sync`,
`skills-sync-simple`, `sync-activity`, `sync-in-place`, `sync-left-out`, `sync-remove`, `sync-vocabulary`,
`stage5-acceptance`, `selected-library`, `machine-add-skill`, `machine-import`, `machine-item-states` and
`skill-content`. Specs that only mock `/api/v2/sync` (`add-machine`, `connect-flow`, `home` and others) are updated,
not deleted.

**The agent, in a temporary home folder:**
- installs a missing Active skill and checks the digest
- refuses a bundle whose digest doesn't match
- replaces a different copy after backing it up, and marks a copy edited on the machine as `edited`
- backs up and removes an Archived skill it never installed, including an edited one and a symlinked one
- leaves an unknown skill alone and reports it
- carries on past one failure
- does nothing when paused or when allow-apply is off
- keeps the Cursor and OpenCode links to `~/.agents/skills`
- prunes backups older than 30 days
- restores a backup with `reet agent restore`

**The controller:**
- builds the desired state and its version
- holds a change for the grace period, and Undo inside it sends nothing
- moves through Active → Archived → Restore and Delete
- forgets a deleted name once every unpaused machine has confirmed, and after 30 days
- removes an archived name again when it reappears
- turns an unknown report into *New on a machine*
- Keep captures from the right machine, and asks which copy when digests differ
- reads health from the reports
- Retry clears a failure
- an agent without the new protocol gets no skill work
- fetches a public skill into a bundle (against a stub GitHub), refuses one over the limits or carrying credentials,
  and shows a fetch failure with Retry
- Refresh makes a new revision only when the files changed
- an `edited` report offers **Use this version**, which captures the backup as a new revision that reaches every
  machine

**The upgrade, on a copy of a store with every case:**
- In use, To test and Parked map to the right states
- ignored IDs become Archived, with and without a library item
- duplicate names fold into one item with tags, notes, likes and lists kept
- selections and targets are dropped, and binary-update jobs survive
- the upgrade writes nothing to machines

**End to end:** a real controller and a real agent binary in a temporary home. Add, Archive, Undo, Delete, a new skill
found then kept, and a new skill found then archived, each checked on disk.

**The Skills page (browser):**
- Archive, Delete and Restore leave the tab, scroll position and filters unchanged, and focus lands on the next
  card, or the previous one when it was last
- the toast names the skill and the machines, and Undo puts it back
- deleting a skill you wrote asks once; any other delete doesn't
- bulk Archive, Delete, Restore and Tag work
- the *New on a machine* banner, its review, Keep all, and Home's *Waiting for you* entry
- each health line, and Retry
- the one-time upgrade notice and **Restore the Parked ones**
- at 1440, 1024 and 390 px, light and dark, with nothing under the 14 px type floor or the 44 px touch target

**The Sync page:** the sentence for each state, Retry, Pause, and the activity list.

`npm run check` in `web/`, `go vet ./...` and `go test -race ./...` pass.
