# Account → MCP access: the connected clients

Date: 2 November 2026. Status: done. Scope: the **Connected clients** list on the
account MCP access page (`/account/mcp`). The connect card, status line, browser
WebMCP panel and manual bearer-token form are unchanged.

## Why

OAuth sign-ins and bearer tokens were one list with a heading and Refresh but no
count, search or way to narrow the two kinds. They now use the collection grammar,
while each connection keeps its existing card, state and actions.

## Contract

1. **One Connected clients panel.** It carries a live count, a search and **All /
   OAuth / Bearer token** chips whose figures follow the search.
2. **Kind chips are exclusive.** Every connection is one kind, so the two figures add
   up to All.
3. **One row per connection.** Name, kind, state, permission, last use and expiry,
   plus the existing **Manage**, **Pause / Resume**, **Replace token** and **Revoke**
   actions.
4. **Search reads the name and kind**, not the state or dates.
5. **Empty, loading and error live in the panel.** A first read is covered by the
   page skeleton; an empty search offers **Clear filters**.
6. **Revoke all stays below the panel**, outside the search.
7. **No API, permission or connection behaviour change.**

## Rulings

- **Kind is the chip dimension, not state.** Kind never changes for a connection and
  the page already explains both kinds; state is shown per row. *Cost if wrong:* there
  is no “active only” view.
- **The cards remain.** They carry different actions for OAuth and bearer tokens, so
  flattening them into one row would hide controls. *Cost if wrong:* the list is
  tall.
- **The count is connections, not clients.** *Cost if wrong:* a client with two
  connections counts twice.
- **Revoke all ignores the search and chip**, as before. *Cost if wrong:* a person may
  expect it to revoke only the visible connections.

## Testing

- Browser: `account-mcp.e2e.mjs` covers the panel count, all three kind chips, search,
  empty recovery, the OAuth filter, pausing a connection and phone layouts at 390 and
  320 px with no overflow.
- Regression: `phone-zoom.e2e.mjs` (the account MCP screen),
  `navigation.e2e.mjs`, `npm run check` and the production build.
