# Unstable tests

A test that fails at random gets a line here with an owner, what is known, and a date by which it is fixed or deleted.
When it is fixed, delete the line and say how in the handoff entry. Never skip a test to hide it. The owner is the
person who answers for it, not necessarily who fixes it.

| Test | Where | Seen | What is known | Owner | Fix by |
| --- | --- | --- | --- | --- | --- |
| `TestFetchStopsReadingStream` | `internal/skillfetch` | once, in a loaded full run | The fake server keeps writing ahead of the client, so under load it can pass the half-way mark before the client closes. Passes alone 20 times. Fix by making the server wait for the client's read instead of writing ahead. | jnnngs | 8 December 2026 |
| `TestAMachineThatWillNotFollowTellsTheOldControllerOnce`, `TestSetServerClearsAPendingMoveAndTellsTheOldController` | `internal/scan` | whenever run with `-count` above 1 | State leaks between repeats. CI runs them once, so it does not show there. | jnnngs | 8 December 2026 |
| `TestExclusiveLockReleasedByProcessDeath` | `internal/filelock` | once, in a loaded full run | Passed 250 runs afterwards. Cause not known. | jnnngs | 8 December 2026 |
| `terminal-phone` browser suite | `web/tests` | once, on a pull request | Failed on a resize after attach; passed 4 of 4 runs locally. Cause not found. | jnnngs | 8 December 2026 |
| `navigation` browser suite | `web/tests` | once | "Browser has been closed", then passed. Cause not found. | jnnngs | 8 December 2026 |
| `add-machine` browser suite, re-enrol step | `web/tests/add-machine.e2e.mjs` (about line 674) | once, on #918 (passed on re-run and on its other runs) | Clicking "I've run it — check again" in the Add a machine dialog timed out after 30 s: Playwright reported the button "not stable" and "detached from the DOM" and kept retrying. The button sits in `{#if host && !canUse}` in `AddMachineDialog.svelte` and the dialog re-reads on a timer, so a read landing at the click may re-create it; not reproduced. | jnnngs | 8 December 2026 |
| `phone-dialogs` job setup | `terminal-browser.yml` | once, on main | `npm ci && npm run build` failed in the job's setup step before any suite ran; a network or runner fault, not a test. Not reproduced. | jnnngs | 8 December 2026 |
