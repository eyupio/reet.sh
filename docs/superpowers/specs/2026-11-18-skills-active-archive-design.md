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

<!-- ENGINE: filled in from the engine map -->

## The upgrade

- In use and To test become **Active**. Parked becomes **Archived**. Skills on the ignore list with reason
  *removed* become **Archived**, so the five left on Elitebook are taken off.
- A one-time notice on the Skills page: *8 Parked skills are now Archived and were taken off your machines.*
  **Restore all**.
- Likes, notes, tags and lists are kept.
- Saved per-machine plans, custom folder plans, lists used as sync sources and pending sync jobs are dropped. Every
  machine follows the library from then on.
- Skills already on a machine that are not in the library appear as **New on a machine**.

## What is deleted

<!-- ENGINE: filled in from the engine map -->

## Left to the builder

<!-- filled in after the builder check -->

## Testing

<!-- ENGINE: filled in from the engine map -->
