# Admin → Backups: copies and runs

Date: 2 November 2026. Status: done. Scope: the **Copies** and **Recent runs** lists
under Instance administration (`/admin/backups`). The schedule, destinations,
import and restore controls are unchanged.

## Why

Copies and runs were the last two wide tables under Instance administration. On a
phone each scrolled sideways, neither had a search or a count of what was shown, and
a damaged copy was visible only by scrolling its row. They now follow the collection
grammar used by the other admin sections.

## Contract

1. **One Backup copies panel.** It carries a live count, a search and **All / This
   machine / Off-site / Damaged** chips. The count is `N copies` or `N of M copies`.
2. **Overlapping location chips.** A copy can be both on this machine and off-site,
   so those two figures may overlap and do not add up to All. A copy is **Damaged**
   only after **Verify** has found a problem.
3. **One copy row.** Its id and verification badge; when it was taken, its size and
   version; its location tags; and the existing **Verify**, **Download**, **Restore**
   and **Delete** actions. A damaged row states the problem.
4. **One Recent runs panel.** It carries a live count, a search and **All /
   Succeeded / Failed / Running** chips. Each run is one row: trigger, start time,
   outcome, size, destination and any error.
5. **Filters are client-side.** Both lists are already in the loaded backup status,
   so search and chips do not re-fetch. Chip figures follow the search.
6. **Empty, loading and error live in the panels.** A search matching nothing offers
   **Clear filters**.
7. **The rest of the page stays.** The key and schedule cards, findings, schedule
   form, destinations, import, restore and confirmations are unchanged.
8. **No API, permission or backup behaviour change.**

## Rulings

- **Location chips overlap on purpose.** They answer “is this copy here?” and “is it
  off-site?”; forcing one exclusive location would hide that a copy is both. *Cost if
  wrong:* the chip figures cannot be added up to All.
- **Damaged is a verification result, not a stored field.** It appears only after a
  person runs Verify. *Cost if wrong:* an unverified copy is never called damaged.
- **Running has its own run chip.** *Cost if wrong:* a run in progress otherwise
  appears only under All.
- **The tables become rows.** *Cost if wrong:* comparing many copies by column takes
  more vertical space.
- **The actions wrap under the row on a phone.** *Cost if wrong:* rows are taller.

## Testing

- Unit: `adminBackups.test.mjs` covers copy and run search, the overlapping copy
  chips, damaged verification, ordering and the count labels.
- Browser: `admin-backups.e2e.mjs` covers both panel counts, every chip, copy and run
  search, empty recovery, a verify that marks a copy damaged, and phone layouts at
  390 and 320 px with no overflow.
- Regression: `phone-zoom.e2e.mjs` (the admin backups screen), `navigation.e2e.mjs`,
  `npm run check` and the production build.
