---
title: One source for each skill · reet.sh
description: Where reet keeps the canonical copy of a skill, and how automatic sync and manual imports both reach it.
topic: SKILLS & SYNC
category: skills
---

# One source for each skill

Your personal **library** is the single source of truth for a skill. Everything
else is either a way into it or a copy of it:

- **Automatic** — a connected machine reports the skills it has, and background
  sync keeps the skills you selected on that machine. Both read the library.
- **Manual** — you import a skill found on a machine, write a hosted skill, or
  link an existing entry to its install source. Each of these ends in the same
  library item.

A skill is shown once. When a machine has a copy of a skill that already has a
known source in your library, that copy is an observation of the item, not a
second entry. You do not need to keep a separate machine-only card beside it.

## What counts as the same skill

The library keeps one entry per logical item. A name alone is not identity:

- An item with a known source (a hosted or authored skill, or a skill linked to
  an install URL) is linked.
- An item captured from a machine without a source is unlinked.
- An unlinked machine copy of a name that already has a linked item is folded
  into that item for the Library and for sync.
- Two same-name copies that each have their own known source stay separate, and
  sync settles which copy one machine receives.
- Two explicitly imported same-name machine copies also stay separate. reet does
  not guess that they are the same skill just because the names match.

## The two paths

| Path | What you do | Where it lands |
| --- | --- | --- |
| Automatic | Connect a machine and select a skill for it. Scans and background sync then maintain the selected items. | The same library item, with the machine as a destination. |
| Manual | Import a skill from a machine, write a skill, or link a source. | The same library item, now with a known source. |

Saving an edit or selecting an item does not create a second copy. A machine
copy is only promoted to a source of its own when you import it deliberately or
link it to an install source; a linked item of the same name then becomes the
one the Library shows.

## Verify it by hand

Before this behaviour, a machine that reported a skill you already hosted showed
two cards with the same name: one **Available from your machines** and one from
its hosted URL, and both could be selected for the same machine.

After it:

1. Host or link a skill, for example `reet-babysit`, and note that its Library
   entry is the linked one.
2. On a machine with a local copy of the same name, scan or connect. The
   machine copy appears on that entry's machine list; the Library still shows
   one entry for the name.
3. Select it for a machine in **Sync**. The plan names the linked source, not a
   second shelf copy.
4. Import a skill that exists only on a machine. It is kept as its own library
   item until you give it a source, so the manual path still works.

## Limits

- A name match alone never merges two different skills. Only a linked item folds
  an unlinked machine copy of the same name.
- The library entry is displayed once; the machine's files are not rewritten by
  this view.
- If you want to keep a machine-only skill separate, import it and do not link
  it to the same source as an existing item.
