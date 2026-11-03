# Skill card: review a removal from a machine

Date: 2 November 2026. Status: done. Scope: the **Remove…** link beside an installed
skill badge and the Sync page's prepared exact-set review. No files are removed by
this pass; it prepares the existing review-before-removal flow.

## Why

A person could see **Installed** on a machine but had no route from that fact to
removing the skill. Doing it directly from the badge would be unsafe: sync removal
needs an exact set, and that set can also remove other extras. The safe route is to
prepare the review in Sync and let the person save, preview and approve there.

## Contract

1. **An installed badge offers Remove…** only for a writer and only when that machine
   has a saved sync set. The link names the skill and machine for assistive tech and
   opens `/sync?machine=<host>&remove=<skill id>`.
2. **Sync prepares the review.** With the `remove` parameter, Sync selects that
   machine, opens **Advanced sync**, switches the mode to **Exact set**, unticks the
   named skill in the desired set, and says **Removal prepared. Review the exact set
   below, then save; no files change until you preview and approve.**
3. **Nothing is written by the link.** Opening the prepared review makes no API
   write. Saving the desired set, previewing and approving remain separate actions.
4. **Other extras stay visible.** If the exact set would remove other skills, the
   existing counts, differences and confirmation show them; the removal is not
   presented as affecting only the named skill.
5. **Shelf entries, notes and machine files are untouched by preparation.**
6. **No new API or permission.**

## Rulings

- **Remove is a link to review, not a one-click delete.** *Cost if wrong:* the person
  takes an extra step through Sync.
- **The link appears only with a saved set.** Without one, Sync has no selection to
  prepare; the machine card's setup route is the right place to begin. *Cost if
  wrong:* an installed extra with no saved set has no link here.
- **The mode becomes Exact set.** Additive mode would keep the file after unticking
  it, which would make the removal promise false. *Cost if wrong:* approving the
  exact set can also remove other extras, which the review lists.
- **No automatic preview or approval.** *Cost if wrong:* the person must press Save,
  Preview and Approve as before.

## Testing

- Browser: `skill-card.e2e.mjs` checks the link's accessible name and URL;
  `sync-remove.e2e.mjs` checks the prepared exact-set review, the unticked skill, the
  kept selection, that no write was made, and the phone layout at 390 and 320 px.
- Regression: `control.e2e.mjs`, `phone-zoom.e2e.mjs`, `npm run check` and the
  production build.
