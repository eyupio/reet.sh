# Admin → Users: the people list

Date: 2 November 2026. Status: done. Scope: the **Users** section of Instance
administration (`/admin/users`). It is the proving page for the Account & Admin
family.

## Why

The people list was a wide table with five columns, a sticky first column, role
selects inside cells and a **Manage** disclosure in the last cell. On a phone it
was a horizontal scroller; on a desktop the management controls read as table
furniture rather than one person's controls. The invite form shared the same card,
so the list had no count or search of its own.

## Contract

1. **The invite form is its own card.** It keeps the email, role and
   **Create invite** controls, and the invitation link and temporary password
   notices sit with it.
2. **The people list is one collection panel.** A live count (`4 people` or
   `1 of 4 people`), **Refresh** opposite it, and a search over name, address and
   role.
3. **Role chips carry honest figures.** **All**, **Owners**, **Admins**,
   **Members** and **Viewers** count the people the current search leaves, so a
   chip never promises more than pressing it shows.
4. **One row per person.** Name and address, role, tokens, last active and a
   **Disabled** badge where it applies. The whole row is not a link; the role select
   and **Manage** disclosure are its controls.
5. **Management stays where it is.** An owner keeps the inline role select; the
   Manage disclosure keeps Disable/Enable, reset password, revoke tokens, make owner
   and delete, with the same confirmations.
6. **No horizontal scroller.** A phone stacks the identity, metadata and controls.
7. **No API or permission change.**

## Rulings

- **Invite stays above the list as its own card.** It is the page's primary action.
  *Cost if wrong:* the list starts lower; on a phone the invite form comes first.
- **The role badge is hidden when the role select is shown.** The select already
  says the role, so the row does not print it twice. *Cost if wrong:* a user whose
  role can be changed has no separate role word in the metadata.
- **Search reads name, address and role only.** Disabled people are found by name
  and are labelled **Disabled**. *Cost if wrong:* typing “disabled” does not filter
  to disabled people; the label is still on each row.
- **The outer administration wrapper is a div, not a section.** The page's h1
  already names the page and the inner panel names the list; two nested regions
  called Users were ambiguous to assistive technology.

## Testing

- Unit: `adminUsers.test.mjs` covers search, role counts under a search, filtering,
  order and the count label.
- Browser: `admin-users.e2e.mjs` covers the count, every role chip, the search and
  empty recovery, the disabled badge, inline role changes, the Manage actions and
  phone layouts at 390 and 320 px.
- Regression: `single-sign-on.e2e.mjs`, `phone-zoom.e2e.mjs`, `navigation.e2e.mjs`,
  `npm run check` and the production build.
