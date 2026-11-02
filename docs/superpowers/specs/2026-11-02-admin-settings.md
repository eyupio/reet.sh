# Admin → Settings: one instance panel

Date: 2 November 2026. Status: done. Scope: the **Settings** section of Instance
administration (`/admin/settings`), its six instance settings and the three
immediate settings that belong beside them.

## Why

The six instance settings were a narrow run-on form with the Save action at the
bottom. The Approvals, Repository maintenance and Software artifact mirror blocks had
no section condition at all, so they appeared below every Admin page. They now sit
only on Settings, and the six instance settings use one bounded panel grouped by what
they affect.

## Contract

1. **One Instance settings panel.** It carries a count of the six settings and
   **Save settings** in its head; the instance-wide note sits beneath.
2. **Two groups.** **Background jobs** holds metadata refresh, the topic seed and the
   empty reet-server report retention. **Sharing and identity** holds read-only list
   links, per-person rollup rows and the instance name.
3. **One save.** The panel sends every value in the form and reports
   **Sorted. Settings saved.**
4. **The immediate settings are on this page only.** **Approvals**, **Repository
   maintenance** and **Software artifact mirror** keep their own cards and their
   immediate saves, and no longer appear under Users, Backups, Activity or any other
   Admin section.
5. **Read-only for non-owners.** A non-owner sees the same six settings as labelled
   facts and no Save action.
6. **No search.** Six settings fit on one screen and the two groups are the map.
7. **No API, permission or default change.**

## Rulings

- **The three immediate settings stay separate cards.** They save as soon as they
  change, unlike the form; folding them into the panel's Save would make the page
  promise one save for four different writes. *Cost if wrong:* the page has more
  than one card.
- **The six settings are grouped rather than a flat list.** *Cost if wrong:* the
  groups add two legends.
- **The count is six settings.** *Cost if wrong:* it must be updated if a setting is
  added or removed.
- **The immediate settings now live under Settings.** *Cost if wrong:* an admin who
  expected them at the bottom of every Admin page must open Settings instead.

## Testing

- Browser: `admin-settings.e2e.mjs` covers the panel count, both groups, the save
  payload, the YOLO, branch-prune and mirror writes, caching the artifact, the three
  blocks being absent from Activity, and phone layouts at 390 and 320 px with no
  overflow.
- Regression: `phone-zoom.e2e.mjs` (the admin settings screen),
  `navigation.e2e.mjs`, `npm run check` and the production build.
