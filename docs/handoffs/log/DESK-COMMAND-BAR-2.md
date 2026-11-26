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
- A press focuses a pane on the click that follows it, not on the press itself; a keyboard focus (Tab to a control, or
  a programmatic focus) still makes it the focused pane at once. Moving the bar between panes changes their heights,
  and focusing on the press moved the pane's buttons before the release, so a press on a card action could miss. The
  press acts first, then the pane focuses and the bar moves.
- `docs/desks.md` says where the bar now sits.

Checked: `npm run check` in `web/` (132 test files, svelte-check clean); `npm run build`; `desks.e2e.mjs` against a
built preview, in full and alone with `REET_DESKS_ONLY=command-bar`. The command-bar part now also asserts the bar sits
inside `.desk-pane[data-pane=...]` and that no bar remains a direct child of the desk screen. An earlier run failed in
the pane state part (`folderGone` expected, `lost` seen); the same file passes on the base commit, so the cause was this
move: focusing on the press shifted the pane's action button before release. Focusing on click fixed it, and the whole
file now passes.

Not done, not checked.

- No visual check in light and dark or on a phone after the move; the bar keeps its old CSS.
- The separate Sessions screen's single terminal still has no bar, as before.
