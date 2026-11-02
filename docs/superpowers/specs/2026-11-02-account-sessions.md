# Account → Sessions: the browser list

Date: 2 November 2026. Status: done. Scope: the **Sessions** section of the account
page (`/account/sessions`), its browser list and the existing sign-out actions.

## Why

The sessions were a thin list inside one block with no count, search or way to tell
the current browser from the others except a small tag. They now use the collection
grammar used by the refreshed directory and Admin pages.

## Contract

1. **One Your sessions panel.** It carries a live count, a search and **All / This
   browser / Other browsers** chips whose figures follow the search.
2. **One row per browser.** The friendly device name, the **this browser** tag, when
   it was last seen and signed in, and **Sign out** for every other browser.
3. **The panel keeps the actions.** **Refresh** re-reads the list; **Sign out the
   others** stays one global action, not narrowed by the search or chip.
4. **Loading, empty and error live in the panel.** A first read shows a skeleton; an
   empty search offers **Clear filters**.
5. **Search reads the friendly browser and device**, not the raw user-agent string:
   a Chrome user-agent that contains “Safari” must not match a Safari search.
6. **The session footer stays.** The separate **Session / Sign out** card remains
   below the panel.
7. **No API, session lifetime or permission change.**

## Rulings

- **The friendly device is the searchable text.** This avoids the Chrome user-agent
  matching “safari” and keeps the placeholder honest. *Cost if wrong:* a raw
  user-agent token that is not in the friendly name cannot be searched.
- **The chips are exclusive.** *Cost if wrong:* a session is one of current or
  other, so no overlap is needed here.
- **Sign out the others ignores the filter.** It is the same bulk action as before.
  *Cost if wrong:* a person may expect it to sign out only the filtered browsers.
- **The footer sign-out stays separate.** *Cost if wrong:* there are two sign-out
  actions on the page, one for this browser and one for the others.

## Testing

- Browser: `account-sessions.e2e.mjs` covers the panel count, all three chips,
  search by browser, empty recovery, the other-browser filter, one sign-out and the
  bulk sign-out, and phone layouts at 390 and 320 px with no overflow.
- Regression: `phone-zoom.e2e.mjs` (the account sessions screen),
  `navigation.e2e.mjs`, `npm run check` and the production build.
