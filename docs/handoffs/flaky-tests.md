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
