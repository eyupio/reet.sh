## FACTORY-BAR-LAUNCHER-1: Gaffer's floating button rests in the Factory action bar's empty side (26 November 2026)

On the Factory page's pipeline detail, Gaffer's floating button climbed above the whole **What do you want to do?**
bar even when the bar's buttons sat at its left and its right side was empty, so the button floated over the detail's
History row. The bar was one full-width `data-floating-clearance="avoid"` region, and the launcher avoids such a
region by moving above the whole of it.

What changed.

- `PipelineDetail.svelte`: the action bar is marked `data-floating-clearance="controls"` instead of `avoid`.
- `floatingLauncher.ts`: a `controls` region does not move the launcher above the whole region; only its buttons
  (`button, a[href], input, select, textarea`) are obstacles, measured from the resting floor. `avoid` regions behave as
  before, and the result never sits lower than a position the `avoid` regions require.
- The button now rests in the bar's empty right side on wide screens. When the bar's buttons reach its column
  (narrower panes, wrapped choices), it still climbs above them.

Checked.

- Web unit tests (`npm run check`: 132 files passed) and `svelte-check` with 0 errors and 0 warnings.
- Browser, against a built UI: `factory-pieces` (all steps, including the new assertion that at 1440 the button's top is
  level with or below the bar's top), and these pass unchanged: `home-waiting`, `gaffer-layout`, `gaffer-widget-ux`,
  `gaffer-factory-session`, `sessions-phone`, `navigation`, `home`, `gaffer-factory`, `factory-decision`,
  `factory-templates`.
- `factory-pieces` also holds the button off every button and link in the bar at 1440, 834 and 390 px.

Not checked.

- No manual screenshot of every route; only the suites above were run.
- Light and dark are covered by those suites; the new resting spot was not measured by hand at other zoom levels.
