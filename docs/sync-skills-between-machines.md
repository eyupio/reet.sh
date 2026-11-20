---
title: Sync AI agent skills between machines · reet.sh
description: Keep Active skills on every connected machine, archive them in place, and recover saved copies.
topic: SKILLS & SYNC
category: skills
art: skills
art_icon: branch
art_label: IN GOOD ORDER
card_title: Active skills. Every connected machine.
hero_title: Same skills. Every machine.
hero_accent: One library to follow.
related:
- controller-sync.md
- machine-skill-import.md
layout: article
---

# Sync skills between machines

Your library in reet is the source of truth. Every connected machine follows it automatically.

| Choice | In reet | On connected machines |
| --- | --- | --- |
| Active | Content, notes, tags and history are kept | Installs the current saved version |
| Archived | Kept for later | Backs up and removes the skill |
| Delete | Removes the entry | Backs up and removes its copies |

## Get started

1. [Connect a machine](controller-sync.md#connect-a-machine) and allow local skill writes.
2. Add a skill by link or write one. New skills start Active. Community **Save** and a list's **Add all** offer Active or Archived before saving.
3. Open **Sync** to see each machine's progress. No separate sync plan needs saving or approving.

An Active skill applies to every connected machine. Lists, tags and likes organise skills; they do not choose destinations.

## Review skills found on a machine

A skill that reet did not install waits under **Skills → New on a machine**, also linked from Home's **Waiting for you**. It is left alone until you decide:

- **Keep** takes a private copy from that machine and makes it Active. Other machines receive it once the capture succeeds.
- **Archive** keeps it in reet and takes it off machines.

An Archived skill that reappears is removed again. A Deleted skill that reappears is treated as new, so you decide again. An inventory scan alone never adds a skill to your library.

## Prune without losing your place

Use **Archive**, **Restore** or **Delete** on a card or a selection of cards. The current tab, filters and scroll position stay in place. **Undo** reverses a mistaken action. A short grace period lets Undo cancel work before it reaches machines.

Deleting your own written skill asks for confirmation. If reet has no saved content for a skill, deletion requires confirmation that there is no copy to recover from reet. Machine backups remain a separate recovery route.

## Follow progress

**Sync** shows a sentence for the fleet, a row for each machine and recent activity. A change saved in reet can still be waiting for a machine; it is not reported as installed or removed until the machine answers.

- An offline machine catches up when it reconnects.
- A paused machine is left alone. Resume it when it should follow the library again.
- **Retry** asks a failed machine to try again on its next poll, clearing its retry wait. It does not mean the repair has succeeded.
- An older connector may need updating before it can accept skill work.
- A machine that cannot write skills shows the local command needed to allow it.

## Where files go

Reet manages `~/.agents/skills`, creating it when needed, and the existing `~/.claude/skills`, `~/.cursor/skills` and `~/.config/opencode/skills` folders. Cursor and OpenCode copies link to the shared agents copy; Claude keeps its own copy. Folders that resolve to the same managed location are handled once.

Project folders, `~/.codex/skills` and custom agent configuration roots are outside this reconcile loop. Reet does not install the agent application as part of skill sync.

## Edits, originals and recovery

Edit an opened skill in reet to save a new revision. All machines receive that revision. The original link remains a reference: **Update from original** shows the file changes and asks before replacing reet's copy. It can also fetch the first copy when reet has no content yet.

If someone edits a managed copy on a machine, reet backs up that edit and replaces it with its saved version. **Use this version** in the opened skill adopts the machine's saved edit into reet, after which machines follow it. **History → Put back** restores an earlier saved library version after confirmation.

Machine backups are retained for 30 days. For local recovery, see [setup and recovery](controller-sync.md#recover-a-machine-copy).

## Upgrading from the old model

In use and To test become Active; Parked becomes Archived. Archive and Delete now affect machine files. Update the controller and connectors together; connectors below skills protocol 2 receive no skill work.

Per-machine skill selections, exact sets, list-based plans, preview/approve jobs and the old sync engine are superseded. `reet sync` and `reet agent library-import` only print what replaced them and exit with code 2. Setups skip skill folders, including those in older snapshots; skills come from the library.
