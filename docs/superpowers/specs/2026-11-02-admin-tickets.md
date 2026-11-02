# Admin → Support: the ticket queue

Date: 2 November 2026. Status: done. Scope: the **Support** section of Instance
administration (`/admin/support`), its queue and the existing ticket detail.

## Why

The queue had status filters and a search, but no count of what was shown, its
status figures came from the server's totals rather than the current search, and it
re-fetched one status at a time. It was the last admin list outside the collection
grammar used by Users and Team usage.

## Contract

1. **One queue panel.** **Ticket queue** has a live ticket count and **Refresh**;
   the search and **Open / Waiting / Closed / All** chips sit beneath.
2. **Counts follow the search.** Status figures come from the tickets the current
   search leaves. The panel count is `N tickets` or `N of M tickets`.
3. **One load, client-side filters.** The queue reads every ticket once and the
   status chip filters in the browser, so switching status does not re-fetch and a
   chip cannot promise more than pressing it shows. The default is still **Open**.
4. **One row anatomy.** Subject, number, opener/team, message count and age; a
   compact status badge; the assignee. A selected row is tinted.
5. **The conversation stays.** The detail panel, thread, reply/note composer, status
   and assign controls are unchanged. The detail remains when a filter changes.
6. **Empty, loading and error live in the panel.** A search matching nothing offers
   **Clear filters**.
7. **No API or permission change.**

## Rulings

- **The queue reads all tickets once.** This is what makes the status counts honest
  under a search. *Cost if wrong:* a larger initial response; the admin queue is
  bounded by actual tickets.
- **The default status remains Open.** *Cost if wrong:* a closed-only queue starts
  on an empty view with **All** showing the figure.
- **The detail stays put while the queue filters.** *Cost if wrong:* the selected
  ticket can be outside the visible queue; the detail still says which one it is.
- **The queue remains sticky on desktop.** *Cost if wrong:* the panel's own scroll
  area can be awkward with very long queues.

## Testing

- Unit: `adminTickets.test.mjs` covers search by subject, opener, team, assignee and
  id, counts under a search, filtering order and the count label.
- Browser: `admin-tickets.e2e.mjs` covers the count and status chips, search,
  empty recovery, selecting a ticket and its conversation, and phone layouts at 390
  and 320 px with no overflow.
- Regression: `phone-zoom.e2e.mjs`, `navigation.e2e.mjs`, `npm run check` and the
  production build.
