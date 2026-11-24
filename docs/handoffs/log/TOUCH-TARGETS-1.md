## TOUCH-TARGETS-1: the terminal toolbar and the chat queue reach the 44 px touch rule, except three rows on a portrait phone (24 November 2026)

Two sets of controls were under the product's 44 px rule on a coarse pointer: the terminal pane's toolbar, key row,
header controls, menus and footer button (40 px, the menus 32 px), and the buttons in `ChatQueue.svelte` (30 px).

What changed.

- `ChatQueue.svelte`: on `pointer:coarse` every button is at least 44 × 44 px (Send now, Stop and send, Retry, Edit,
  Remove, Save message, Cancel edit, and the summary line, which already was). The "I checked the chat" label is a
  44 px row (class `confirm`) with a 24 px box. Nothing changes with a mouse.
- `TerminalPane.svelte`: on `pointer:coarse` the toolbar, key row, pane header controls, Manage menu items, the
  copy and paste menu and the footer buttons are 44 × 44 px. The two tablet rules that kept the toolbar at 40 px and
  the key row at 44 px are gone, because the base rule says 44 now. A phone held upright
  (`max-width:650px`, `orientation:portrait`) puts the header controls, toolbar and key row back to 40 × 40 px.
- Held sideways (`max-height:500px`), a keyboard-open screen would have been 2 px short of its fourth row, so
  `.pane.keyboard .screen-frame` there takes 2 px of padding instead of 4 px above and below.
- `docs/terminal-workspace.md` says so.

Why three rows stay at 40 px. `terminal-phone.e2e.mjs` (390 × 844, around line 169) holds `.pane>header`,
`.terminal-toolbar` and `.mobile-keys` to a height of at most 42 px, and the open "More terminal keys" row to 42 px
(around line 258). A 44 px target makes the header and toolbar 45 px and the key row 44 px. The tests'
requirements were not changed, so those rows and their buttons stay at 40 px on a phone held upright. The footer and
the menus are not in that cap and are 44 px.

Numbers (Chromium, `hasTouch`, DejaVu Sans Mono, the fixture app of `terminal-phone.e2e.mjs`, full chrome unless said).

| Screen | Before | After |
| --- | --- | --- |
| 390 × 844 portrait | 53 cols, 33 rows sent (34 whole rows by the test's formula); header 41, toolbar 41, keys 40, footer 44.4 px | 53 cols, 33 rows sent (33 whole rows); header 41, toolbar 41, keys 40, footer 45 px |
| 390 × 844, keyboard open | 53 × 19 | 53 × 19 |
| 390 × 844, focus mode | 55 cols, 49 rows | 55 cols, 49 rows |
| 320 × 568 portrait | 42 cols, 11 rows (8 with the keyboard open) | 42 cols, 11 rows (8) |
| 844 × 390 sideways | 120 cols, 9 rows; header 41, tool row 41; screen 129 px | 120 cols, 9 rows; header 45, tool row 45; screen 121 px |
| 844 × 390, keyboard open | 4 rows | 4 rows (3 without the 2 px padding change) |
| 915 × 412, keyboard open | 4 rows | 4 rows |

Targets: on a portrait phone the toolbar, key row and header controls stay 40 px, the footer button goes from 40 to 44 and
the Manage menu items from 32 to 44. Held sideways and on a touch tablet the toolbar, key row and header controls go from
40 to 44 (the tablet's footer, menus and key row already were). Chat queue buttons go from 30 to 44.

Tests. `terminal-phone.e2e.mjs` has `targetsAtLeast`: the portrait footer and Manage menu, the sideways toolbar, key row
and header controls (844 × 390 and 915 × 412) and the tablet toolbar, key row, header controls and footer are at least
44 px; each fails on the old CSS (the footer measured 40, the toolbar 40). `chat-controls.e2e.mjs` opens a touch
context at 390 px, light and dark, with a queued, a failed and an uncertain message, and asserts every button and the
retry label is at least 44 px (30 before). The same file now builds its workspace with `launch()` so both contexts share
the fixtures.

Not checked: a real phone (all of this is Chromium emulation with synthesised touch), Safari, a screen with the
browser's own chrome shrinking the viewport, the copy and paste menu's 44 px (changed, no assertion), and the
320 px landscape range. The pane's `.agent-copy button` (40 px) and the workspace toolbar's 40 px buttons are not part
of this branch. To make the portrait phone 44 px as well, the three 42 px limits in `terminal-phone.e2e.mjs` have to
move to 46 first; the sent rows at 390 × 844 stay 33 and the columns 53, but a trial with everything at 44 px took 320 × 568
from 11 rows to 10 (the screen from 144 to 132 px).
