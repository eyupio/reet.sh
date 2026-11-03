# Admin → Activity: the audit list

Date: 2 November 2026. Status: done. Scope: the **Activity** section of Instance
administration (`/admin/activity`), its event list and the existing event labels.

## Why

The Admin Activity page showed the newest 20 events as a thin list with no count,
search or way to narrow by area. The Account Activity page already has a richer
filtered view; this page now gives the instance audit the same collection grammar.

## Contract

1. **One Recent activity panel.** It carries a live count, a search and **All /
   Agents / Projects / Skills / Lists / Account / Admin** chips whose figures follow
   the search.
2. **The newest 100 events are shown.** The server already returns up to 100; the
   page no longer slices that to 20. The panel says so.
3. **One event row.** The existing sentence, the area it belongs to, and its
   relative age; the exact time and source stay in the row's title.
4. **Search** reads the sentence, actor, source, action, raw detail and area, so a
   ticket number, host or skill name finds its events.
5. **Area chips use the existing event taxonomy** (`eventCategory`) and are
   exclusive, so their figures add up to All.
6. **Empty, loading and error live in the panel.** A search matching nothing offers
   **Clear filters**.
7. **No API, retention or event-label change.**

## Rulings

- **Show the newest 100, not 20.** The server already sends them and the page can
  carry them; the old slice hid most of the audit. *Cost if wrong:* the page is
  longer.
- **Use the existing areas rather than a new taxonomy.** *Cost if wrong:* an
  unusual action is grouped as Account, as the existing mapping already does.
- **Search includes the raw detail.** This is what lets a person find a ticket,
  host or file without a second filter. *Cost if wrong:* a term in an unshown
  detail can match.
- **No per-row expansion.** The exact time and source remain in the title; the row
  stays one line of prose. *Cost if wrong:* the detail JSON is not browsable on the
  page.

## Testing

- Browser: `admin-activity.e2e.mjs` covers the panel count, every area chip, search
  by event text and detail, empty recovery, an area filter, and phone layouts at 390
  and 320 px with no overflow.
- Regression: `phone-zoom.e2e.mjs` (the admin activity screen), `navigation.e2e.mjs`,
  `npm run check` and the production build.
