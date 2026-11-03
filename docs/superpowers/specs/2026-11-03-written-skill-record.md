# One card per written skill

Date: 3 November 2026. Status: done. Builds on the vocabulary pass and
`docs/research/skills-sync-ux-review.md`.

## One place on the Skills page

A published skill someone wrote already has a shelf card. The separate
**Skills you write** row was a second copy of the same skill with different
controls. The Skills page now shows it once:

- **Your drafts** lists only unpublished written skills, with their saved-plan
  and observed-machine line and the existing Publish, Sync and Edit controls.
- A **published** written skill appears only as its shelf card.
- The shelf card's quick actions gain **Edit written skill**, opening
  `/hosting?edit=<slug>`, where visibility, sharing and content live.

Publishing a draft removes it from **Your drafts** and, after the shelf reload,
adds its card. Unpublishing on the authoring page brings the draft back.

## Machines on the authoring page

The authoring page's skill list gains a **Machines** line for every skill, using
the same order as the Skills page: the saved sync plan first, then the last
complete scans. It reads **Not set to sync**, **Set to sync to …**, **Pending on
… · synced to …**, **On N machines**, **Missing on N machines**, or **Not on a
machine yet**. It never claims a file is installed from intent alone.

## Rulings

- **The writing block is “Your drafts”, not “Skills you write”.** *Cost if
  wrong:* a published written skill is no longer listed under the authoring
  block on Skills; its shelf card carries the edit link instead.
- **The two server records stay.** A hosted skill and its shelf entry are still
  separate rows; the UI presents one card for a published skill. *Cost if
  wrong:* the data model keeps a duplicate that a future lifecycle merge could
  remove.
- **Published skills are managed from the shelf, not the drafts block.** *Cost
  if wrong:* unpublishing takes two steps: Edit written skill, then uncheck
  Publish.
- **The Machines line is informational.** It is not a new filter or write path.
  *Cost if wrong:* the authoring page shows state but cannot act on it.

## Testing

- `skillSync.test.mjs` covers the saved-plan-first and observed-scan fallbacks.
- New `written-skill-record.e2e.mjs` checks one published card, a drafts-only
  block, the shelf edit link, publish/unpublish movement and the authoring
  Machines line.
- `skills-sync-simple.e2e.mjs` covers the simplified Sync page and draft sync.
