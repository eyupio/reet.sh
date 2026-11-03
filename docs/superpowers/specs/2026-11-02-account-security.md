# Account → Password: the password panel

Date: 2 November 2026. Status: done. Scope: the **Password** section of the account
page (`/account/security`) and its existing change-password form.

## Why

The password form was a block with a heading, three fields and the action at the
bottom. It now uses the same bounded panel and head action as the rest of the
Account pages, without changing the request or the session rule.

## Contract

1. **One Password panel.** It carries a count of the three fields and **Change
   password** in its head.
2. **Three fields.** Current password, new password and new password again, with the
   existing minimum length and browser autocomplete hints.
3. **The mismatch is checked before the request.** Different new passwords show
   **The new passwords do not match.** and send nothing.
4. **One successful change** sends the same `{ current, password }` body as before,
   reports **Password changed. Every other session was signed out.** and reloads the
   session list.
5. **The panel note says the session rule**: every other browser is signed out; this
   one stays.
6. **No API, validation or session change.**

## Rulings

- **The count is three fields**, not “one password”. *Cost if wrong:* it must change
  if the form gains a field.
- **The action sits in the panel head.** *Cost if wrong:* it is further from the last
  field than a footer button.
- **No separate “current password” card.** *Cost if wrong:* the page has one panel
  where the account pages usually have a create card and a list.
- **The mismatch remains a local check.** *Cost if wrong:* the server is not asked to
  reject two different new passwords.

## Testing

- Browser: `account-security.e2e.mjs` covers the panel count, the mismatch path
  sending no request, the successful request body and notice, and phone layouts at
  390 and 320 px with no overflow.
- Regression: `phone-zoom.e2e.mjs` (the account security screen),
  `navigation.e2e.mjs`, `npm run check` and the production build.
