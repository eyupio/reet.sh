## HOME-FIRST-READ-1: Home holds its skeleton until its first read settles (27 November 2026)

Why. On a refresh, `document.documentElement.dataset.ready` is set at the first auth answer, so the static boot shell
and the route skeleton hand over before Home's own reads finish. Home then drew a confident empty page: "0 waiting for
you", "Nothing is waiting for you.", "Nothing in flight." and an empty project rail, replaced moments later by the real
content. The other pages already showed their reading state; Home's body and counts did not.

What changed.

- `web/src/lib/home/homeFirstRead.ts`: new pure helper `homeFirstRead({settled, failed, writer, hasSessions,
  hasHealth})`, returning `loading`, `error` or `ready`. Before the first read settles it is `loading`; a failed first
  read with no primary data (no sessions for a writer, no health or sync for a viewer) is `error`; anything else is
  `ready`, so a partial failure keeps what was read.
- `web/src/lib/home/Home.svelte`: local `firstReadSettled` and `firstReadFailed`, set once by the first `load()` (polls
  and explicit refreshes leave them alone). Until then the whole `.home` body is replaced by the existing
  `RouteSkeleton`, variant `home` for a writer and `cards` otherwise, the same shape `App.svelte` shows while the Home
  chunk downloads. After a failed first read with nothing usable it shows the existing `PageLoadError`, whose Reload
  retries. Partial failure shows the real content plus the existing problem line.
- No central change: `data-ready`, the static boot shell, `App.svelte`, the API, auth and the build are untouched. No
  artificial minimum display time. The signed-out landing page is unchanged.

Tests. `web/src/lib/home/homeFirstRead.test.mjs` tables the helper over before-settle, full failure for a writer and a
viewer, and partial data. `web/tests/home-refresh.e2e.mjs` (built UI) gates `/api/v2/terminal/sessions`: the `home`
skeleton is visible and "Nothing is waiting for you." / "0 waiting for you" are absent, then release shows the heading,
real content and no skeleton. A 503 first read shows the alert and Reload with no skeleton and no empty-Home copy. A
viewer gets the `cards` skeleton while `/api/v2/machines/health` is held, then their Home. `home-refresh` runs in the
Home browser CI step (`terminal-browser.yml`). `npm test` and `svelte-check` pass.

Review. Re-ran `home-refresh`, `home`, `projects`, `home-waiting`, `home-followups` and `loading` on a fresh build, plus
`svelte-check`: all green. The other browser suites were not rerun here.

Not checked: the known out-of-scope case of a signed-out visitor refreshing a private route, which still gets the app
boot shell before sign-in.

Babysit. CI's `phone-gaffer` shard failed in `workbench`, which still clicked "Continue working" after a 503 first
sessions read, a path the change replaced with `PageLoadError`. The test now expects the alert and Reload instead;
`workbench`, `home` and `home-refresh` pass locally against a fresh build.
