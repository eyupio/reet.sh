---
title: One source for each skill · reet.sh
description: Where reet keeps the canonical copy of a skill, and how automatic sync and manual imports both reach it.
topic: SKILLS & SYNC
category: skills
---

# One source for each skill

Reet keeps the saved content of each library skill. Every connected machine follows that copy. An original GitHub or hosted link is a reference, not an instruction for each machine to fetch independently.

| How a skill arrives | What reet keeps |
| --- | --- |
| Add by link | Files fetched by the controller into a private bundle |
| Write a skill | An editable library copy with immutable saved revisions |
| Keep a new skill found on a machine | Files captured from that machine |

New skills start Active. Community Save and a list's Add all let you choose Active or Archived. Active installs the current saved version everywhere; Archived keeps the item in reet and removes machine copies. Delete removes the item and its machine copies.

## Changes follow the library

An edit in reet saves a revision for all machines. **Update from original** shows upstream changes and asks before saving them. Editing a managed machine copy causes reet to back up that edit and replace it with the library version. **Use this version** adopts the saved machine edit as reet's new copy.

Open **History** to see saved versions. **Put back** makes an earlier version current after confirmation. Publication is separate: **Publish** builds a public page from reet's copy, with attribution for an imported skill; **Unpublish** stops serving that page without archiving the library item.

## Discoveries and identity

An inventory scan reports what a machine holds; it does not create library items. An unknown skill in a managed home folder waits under **New on a machine** until you Keep or Archive it. Project skills stay with their repositories.

Names identify skills on disk, so conflicting names need resolution rather than two different copies being sent to the same folder. Machine reports describe observed copies and progress; they do not create a second authority beside the library.

See [sync skills between machines](sync-skills-between-machines.md) for destinations, upgrade behaviour, backups and offline machines.
