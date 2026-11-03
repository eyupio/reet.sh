# Admin → Notifications: the instance defaults

Date: 2 November 2026. Status: done. Scope: the **Notifications** section of
Instance administration (`/admin/notifications`) and its default notification kinds.

## Why

The defaults were a bare sequence of fieldsets with the Save action floating above
them and no count of what the page contains. The page now uses one bounded panel,
like the other Account & Admin sections, while keeping the notification kinds
grouped by the area they belong to.

## Contract

1. **One Notification defaults panel.** The panel carries a live count of the kinds
   and **Save defaults** in its head; the owner/admin note sits beneath.
2. **Grouped rows.** Each area is a fieldset with its own legend. A row says what
   happens and offers **On**, **Delivery**, and for staff notices **Owner** and
   **Admins**.
3. **Locked notices stay facts.** A locked kind, such as a security notice, shows
   the server's sentence and has no editable controls; it is never sent in the save
   payload.
4. **One save.** The panel sends every editable kind with its current values and
   replaces the list from the answer.
5. **Empty and error live in the panel.** An unreadable list offers **Try again**;
   an empty list says no kinds are available.
6. **No search.** Every default is on one screen and the groups are the map; a
   search would add a control without hiding anything. *Cost if wrong:* a very
   large defaults catalogue would need one.
7. **No API, permission or default change.**

## Rulings

- **The panel keeps the group fieldsets.** A flat list would lose the area names.
  *Cost if wrong:* the panel has several nested borders.
- **The owner/admin note is a toolbar line, not a column heading.** It applies to
  only some rows, so it reads as a whole-panel note. *Cost if wrong:* each staff row
  repeats the same explanation.
- **The count is the number of kinds, not groups.** *Cost if wrong:* the two figures
  differ when a group has more than one kind.
- **The Save action sits in the panel head.** *Cost if wrong:* it is further from the
  last field than a footer button.

## Testing

- Unit: `adminNotifications.test.mjs` covers grouping order, the save payload
  leaving locked kinds out, and the count label.
- Browser: `admin-notifications.e2e.mjs` covers the panel count, every group, the
  locked security row with no controls, a staff row's three controls, a save's
  payload, an unreadable list and its retry, and phone layouts at 390 and 320 px
  with no overflow.
- Regression: `phone-zoom.e2e.mjs` (the admin notifications screen),
  `navigation.e2e.mjs`, `npm run check` and the production build.
