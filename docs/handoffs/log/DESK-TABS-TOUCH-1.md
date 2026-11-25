## DESK-TABS-TOUCH-1: the desk's tab strip pans under a finger (25 November 2026)

On a phone, a sideways drag across a desk's tab strip could not reach the later tabs. The strip is the only horizontal
scroller there, but each tab button set `touch-action: pan-y`, so a touch that started on a tab could not pan sideways.
The tab's pointer handlers then took the drag as a mouse-style reorder, and a press that did not reorder could land as a
click that selected the tab.

What changed.

- `web/src/lib/desks/DeskTabs.svelte`: the tab button's `touch-action` is `pan-x pan-y`, so the strip scrolls under a
  finger with the browser's own momentum, while vertical panning is still allowed. `down()` starts a reorder only for a
  mouse pointer, so touch and pen never enter drag-to-reorder. A tap is still a click and selects its tab; mouse and
  trackpad drag-to-reorder are unchanged.
- `web/tests/desks-phone.e2e.mjs`: the phone test sends a real touch drag through Chromium's input pipeline, starting
  on a tab and moving 160 px left. It asserts the strip's `scrollLeft` grew, the active tab and the tab order did not
  change, and the computed `touch-action` allows both axes; the existing tap of a later tab still selects it. Without
  the source change the same test fails: `touch-action` reads `pan-y` and `scrollLeft` stays `0`.
- No other surface uses `DeskTabs`; the pane chips strip in `DeskScreen.svelte` has no pointer reorder and already
  pans. `docs/desks.md` says how a mouse reorders and how a touch screen does.

Checked: `npm run check` (127 test files and svelte-check), `npm run build`, `go vet ./...`,
`web/tests/desks-phone.e2e.mjs` on a 390 x 780 touch context, and `web/tests/desks.e2e.mjs` for the desktop reorder
test. Before the change the same touch drag did not scroll the strip; after it, the drag pans and a tap still selects.

Not checked: a real phone, iOS Safari, and a horizontal drag on a strip short enough not to overflow. On the desk page
the page itself does not scroll vertically, so vertical panning is verified through the computed `touch-action` rather
than by scrolling the page.
