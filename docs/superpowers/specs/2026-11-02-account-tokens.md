# Account → API tokens: the token list

Date: 2 November 2026. Status: done. Scope: the **API tokens** section of the account
page (`/account/tokens`), its create form and token list.

## Why

The create form and the token list shared one block and one heading, so a token
looked like part of the form and the list had no count, search or way to separate
used tokens from unused ones. The create action now has its own card and the tokens
use the collection grammar.

## Contract

1. **A Create a token card.** It keeps the existing name field, **Create token**
   action, one-time secret and copy button.
2. **One Personal API tokens panel.** It carries a live count, a search and **All /
   Used / Never used** chips whose figures follow the search.
3. **One row per token.** Name, created and last-used time, and the existing
   **Revoke** action with its confirmation.
4. **Used means the server recorded a use.** A token with no `lastUsedAt` is
   **Never used**.
5. **Creating a token adds it to the list** and leaves the one-time secret in the
   create card until the page is left.
6. **Loading, empty and error live in the panel.** A first read shows a skeleton; an
   empty search offers **Clear filters**.
7. **Viewer accounts keep their message** and see no create form.
8. **No API, token lifetime or permission change.**

## Rulings

- **The create form is its own card.** It follows the Admin invite card pattern and
  keeps the one-time secret away from the token rows. *Cost if wrong:* the page has
  two cards instead of one.
- **Used and never used are exclusive chips.** *Cost if wrong:* no “unused for a
  while” view exists.
- **Search reads the token name only.** That is the only searchable field the API
  exposes; dates are displayed, not searched. *Cost if wrong:* searching a date or
  ID does not find a token.
- **Revoke keeps its confirmation.** *Cost if wrong:* one extra step before a token
  stops working.

## Testing

- Browser: `account-tokens.e2e.mjs` covers the panel count, all three chips, search,
  empty recovery, creating a token and its one-time secret, revoking a token, and
  phone layouts at 390 and 320 px with no overflow.
- Regression: `phone-zoom.e2e.mjs` (the account tokens screen),
  `navigation.e2e.mjs`, `npm run check` and the production build.
