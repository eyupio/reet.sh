# Account → Profile: the preferences panel

Date: 2 November 2026. Status: done. Scope: the **Profile** section of the account
page (`/account/profile`) and its existing preferences.

## Why

The profile preferences were one long form with no count or grouping and the Save
action at the bottom. A profile with no stored date format also showed a blank
**Dates** select even though the page read “This device's order”. The preferences now
use one bounded panel, grouped by what they affect, and a blank date format reads as
the device's order.

## Contract

1. **One Your preferences panel.** It carries a count of the eight preferences and
   **Save profile** in its head.
2. **Four groups.** **Identity** (name), **Appearance** (theme, time zone, dates),
   **Shelf and terminal** (terminal history default, shelf layout, default cards) and
   **Publishing** (hosted skill prefix).
3. **One save.** The panel sends the same profile fields as before and reports
   **Reet, defaults saved.**
4. **A blank stored date format reads as `locale`.** The select shows “This device's
   order” rather than a blank option.
5. **A viewer's name is still read-only**; the other preferences remain editable.
6. **No account-menu theme picker is changed.** The page form still follows a theme
   change made elsewhere until it is saved.
7. **No API, permission or default change.**

## Rulings

- **The four groups are the map.** *Cost if wrong:* four legends and borders replace
  one flat form.
- **The count is eight preferences.** *Cost if wrong:* it must be updated if a
  preference is added or removed.
- **Save sits in the panel head.** *Cost if wrong:* it is further from the last field
  than a footer button.
- **A blank date format is `locale`, not an empty option.** *Cost if wrong:* an old
  profile that stored nothing now shows the device order explicitly.

## Testing

- Browser: `account-profile.e2e.mjs` covers the panel count, the four groups, the
  blank-date fallback, the save payload and phone layouts at 390 and 320 px with no
  overflow.
- Regression: `phone-zoom.e2e.mjs` (the account profile screen),
  `navigation.e2e.mjs`, `npm run check` and the production build.
