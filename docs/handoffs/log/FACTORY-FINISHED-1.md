## ID: a factory pipeline that finishes raises one toast and a Factory tab dot (26 November 2026)

What changed.

- A pipeline that moves into `done` is noticed from the outcome lists the app already polls: the Factory's 5 s read,
  the bell's 15 s read on other pages, and Home's 15 s read. There is no new server event or read.
- One toast per read: a single pipeline says “<title>” finished with **Open** to
  `/factory?pipeline=<outcome>.<key>`; several say `<n> pipelines finished.` with **Open** to `/factory`.
- The Factory tab carries an 8 px accent dot until it is visited. Visiting the Factory acknowledges finished pipelines
  silently (no toast) and clears the dot.
- Only `done` counts. `failed`, `cancelled`, `unfollowed` and `ready_to_merge` do not. Archived outcomes are ignored.
- A first read on page load never toasts. A brand-new `reet.factory.finished.<userId>` key seeds what is finished now as
  acknowledged. The in-memory last-seen state stops a repeated snapshot from toasting again, and the stored set of
  acknowledged ids keeps the dot gone across reloads. Leaving `done` clears the acknowledgement, so a later genuine
  finish notifies again.

Files. `web/src/lib/factory/finished.ts` (new pure module), wired in `web/src/App.svelte`, an optional `onoutcomes`
prop on `web/src/lib/home/Home.svelte`, and `badge?: number` rendered as a dot in the tab components.

Tests. `web/src/lib/factory/finished.test.mjs`: first-read silence and seeding, fire-once transitions, acknowledgement
clearing the dot, reload not re-toasting, re-finish after leaving done, terminal non-done states and archived outcomes
ignored, toast wording and hrefs, corrupt-storage recovery and a refusing storage. `npm run check` passes.

Not checked: no e2e test; the toast was not exercised in a browser. Known shortcut: acknowledgements live in this
browser's localStorage per account, so they do not sync across devices. The dot clears on opening the Factory even if
the person never scrolls to its finished section.
