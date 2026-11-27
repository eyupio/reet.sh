## STEADY-HEADER-1: the top bar and logo stay put between signed-in pages (27 November 2026)

The compact top bar used by an open session workspace and an open desk had its own height, padding and, on a phone,
logo width. Moving between Home (or Sessions, Machines, Skills) and a session or desk therefore shrank the bar and
moved the logo: 4 px shorter and 8 px left on a desktop, 8 px shorter and 8 px left on a tablet, and a 120 px logo
became 88 px on a phone.

What changed.

- `web/src/lib/shell/AppShell.svelte`: the compact bar no longer overrides height, padding or gap, so it keeps the
  normal bar's geometry at every width. One logo width rule serves both bars (134 px at 1100 px and wider, 120 px
  below that, 88 px under 368 px where the compact bar's extra More button leaves no room). The 88 px rule applies to
  the normal bar too, so the two bars still agree at 320 px.
- `web/index.html`: the boot shell's workspace variant now uses the same bar height, padding and logo as every other
  signed-in page, and the workspace's boot main height follows the bar's 60/64/56 px heights (plus its 1 px rule).
- `web/tests/loading-width.e2e.mjs` now compares the session workspace's logo box with Home's at each width, and
  `web/tests/shell-fidelity.e2e.mjs` expects the 88 px logo at 320 px.

Checked against the production build with Chromium 1255 and synthetic API answers: `loading-width`, `shell-fidelity`,
`navigation`, `sessions-phone`, `terminal-phone` and `desks-phone` pass. Screenshot measurements before the change
showed the desktop compact bar at 56 px with the logo at x=16 against 60 px and x=24 elsewhere; after it both read
60 px and x=24 (tablet 64 px and x=24, phone 56 px and x=16).

Not checked: a real phone, tablet, Safari or Firefox. The public (signed-out, shared-list, join) header is a separate
design and keeps its own larger logo and reading-width layout; this entry only covers the signed-in app's bar.
