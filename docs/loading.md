# Page loading

Page modules are imported on demand in `web/src/App.svelte`. The shell and shared
controls stay outside each await boundary. Chunk downloads use the same delayed
progress indicator as API reads. A failed download gives a reload action; a slow
one gets a recovery message after ten seconds. Authentication and public pages
use the same mechanism as signed-in pages.

`RouteSkeleton.svelte` supplies forms, lists, card grids, the landing hero, Home
and workspace placeholders. The existing `Skeleton.svelte` remains the data
loading primitive, including rows for collections and a grid that fits narrow
phones. Both use theme tokens and disable shimmer under reduced motion.

Reference checked: `eyupio/zoomies` at
`85a7f6ca16fa6bce5c598632399cb5f74b94e3f8`, specifically
`web/src/App.svelte`, `web/src/lib/components/Skeleton.svelte` and
`web/src/lib/components/LoadingBoundary.svelte`. The useful patterns are a shell
outside the loading boundary, placeholders matching the incoming content, and
separate loading, error and empty states.

From `web/`, build with `npm run build`, start `npm run preview -- --port 4173`,
then run `node tests/loading.e2e.mjs`.
The browser test holds production chunks to check public and signed-in pages at
375 and 1440 pixels in both themes, stable header geometry, reduced motion,
failed download recovery and that unrelated administration code is not fetched.

A comparison build with the previous App entry measured the initial JavaScript
at 1,375 kB before and 297 kB after; initial CSS at 487 kB before and 106 kB after.
The terminal workspace remains a separate, existing large chunk.
