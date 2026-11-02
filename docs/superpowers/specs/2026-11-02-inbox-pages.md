# Inbox pages: Notifications and Support

Date: 2 November 2026. Status: done. Scope: the notification centre
(`/notifications`) and Support (`/support`). Activity already uses the family's
category chips, day groups and feed language, so it is unchanged.

## Why

Both pages were well made but sat outside the collection grammar the rest of the
product now shares. Notifications used a bordered card with a separate
“Unread only” button and a disabled checkbox for security notices; Support used a
different card with filter buttons whose counts ignored the search. The result was
two more ways to read a count, a filter and a record.

## Contract

1. **One collection panel per page.** Notifications sits in a panel labelled
   “Your notifications”; Support's tickets sit in “Tickets you can see”. Both carry
   a live count, the page's own quiet actions opposite it, and one row anatomy.
2. **Scope chips, honest counts.** Notifications has **All** and **Unread** with the
   loaded counts. Support has **All**, **With support**, **Waiting on you** and
   **Closed**; their figures are taken from the tickets the current search leaves,
   so pressing a chip always shows that many.
3. **Rows say what they are.** A notification row shows unread state, title, body
   and age; reading it marks it read in place and follows its link. A ticket row
   shows subject, opener/team, message count, age and status.
4. **Empty, loading and error live in the panel.** A search that matches nothing
   offers a clear way back; a failed read offers a retry.
5. **Preferences are a separate record panel.** Security notices are shown as an
   **Always on** fact rather than a disabled checkbox. Delivery choices remain
   editable for the kinds that can change.
6. **The Support form stays the page's primary action.** It remains its own card
   and the first thing on a phone; the ticket panel follows.
7. **No new surface.** No API, write or permission change.

## Rulings

- **Notifications keeps the server-side Unread query.** The All list is the loaded
  page; the Unread chip and Mark all read use the server's count, so an old unread
  beyond the newest page stays reachable. *Cost if wrong:* switching tabs makes one
  request rather than filtering in memory.
- **A locked security notice is a fact, not a disabled control.** *Cost if wrong:*
  a person cannot see the control in its disabled state; the badge says the same
  thing without implying it can be changed.
- **Support keeps the form first on a phone.** *Cost if wrong:* existing tickets sit
  below the form; someone checking a reply scrolls further.
- **Activity is unchanged.** It already groups by day and filters by category with
  counts, so repainting it would add churn without a UX gain.

## Testing

- Browser: `inbox.e2e.mjs` covers notification counts, unread/all chips, reading a
  row in place, mark-all-read, the locked-preference badge, support counts under a
  search, status chips, empty recovery and phone targets.
- Regression: `phone-zoom.e2e.mjs` covers both pages at 320–412 px;
  `navigation.e2e.mjs` and `phone-dialogs.e2e.mjs` cover the shell around them.
- `npm run check` and the production build.
