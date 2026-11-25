# FOCUS-RING-DARK-1: focus ring clears 3:1 in the dark theme

**What:** `web/src/app.css` gets a `--focus` token (the accent text tone) used by the global `:focus-visible` outline and by
the border of a focused field. Dark: `#ed976e` (about 6.4:1 on the card), light: unchanged `#b84216`. `theme.test.mjs` checks
the ring against every surface in both palettes and that the CSS really uses the token (it fails without the CSS change).

**Why:** the independent measurement of the new setup copy dialog found the ring at 2.66:1 on the dark card; the ring is the
shared accent used everywhere.

**Checked:** theme tests (23 pass), `npm run check`, a built preview in dark and light: the focused button's computed outline
colour is `rgb(237, 151, 110)` in dark and `rgb(184, 66, 22)` in light.

**Not checked:** every screen by eye. Components with their own `outline: ... var(--accent)` are not changed: only
`DeskMenu.svelte` (a selected colour swatch, not a focus ring) has one. Safari and Firefox rendering.
