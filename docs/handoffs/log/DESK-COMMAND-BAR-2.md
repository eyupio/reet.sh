## DESK-COMMAND-BAR-2: the command bar is drawn inside the focused pane (26 November 2026)

The dynamic prompt extensions - the command bar's next-action and completion suggestions - were drawn in one bar
under the desk's panes. They now render inside the terminal area of the focused pane, at the foot of the pane's body,
above nothing else. The reading of "the terminal area" is the in-app pane that owns the terminal surface, not the shell
prompt line inside the terminal emulator.

What changed.

- `DeskPane.svelte` takes an optional `bar` snippet and renders it at the end of the pane's `.body`, directly under
  the terminal. The bar is therefore inside the pane's border and shrinks the terminal grid rather than floating over
  it, so xterm's own `ResizeObserver` refits and redraws when the bar appears or the pane is resized.
- `DeskScreen.svelte` passes one `CommandBar` snippet to the focused pane only. The old full-width bar under `.area`
  is gone, not kept alongside. All of the bar's props, its suggestion logic, read timing, keyboard handling and
  disabled states are unchanged.
- `docs/desks.md` says where the bar now sits.

Checked: `npm run check` in `web/` (132 test files, svelte-check clean); `npm run build`; the command-bar part of
`desks.e2e.mjs` now also asserts the bar sits inside `.desk-pane[data-pane=...]` and that no bar remains a direct child
of the desk screen. That whole browser file was started against a built preview, but it stopped earlier in the pane
state part (a reopen expected `folderGone` and saw `lost`), so the command-bar part has not been run to completion in
this environment. The same file passed its earlier parts before that point.

Not done, not checked.

- No visual check in light and dark or on a phone after the move; the bar keeps its old CSS.
- The separate Sessions screen's single terminal still has no bar, as before.
