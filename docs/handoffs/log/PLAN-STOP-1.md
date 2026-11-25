## PLAN-STOP-1: stopping a Gaffer plan while its start is on the way back still stops it (25 November 2026)

Found by the automated review of PLAN-PROGRESS-1. `planGafferOutcome` in `web/src/lib/api.ts` passed the caller's abort
signal to the request that starts the plan, outside the `try` that sends the DELETE. A Stop, or closing the dialog,
between the controller accepting the start and its answer reaching the browser sent no DELETE: the job ran out its 90
seconds, spending provider credits and holding one of the person's two places.

What changed. The start request no longer carries the signal, so its answer (the job ID) is always read. If the caller
has aborted by then, the job is stopped at once. The first step is reported only after that check; reporting it before
brought the progress panel back after Stop, which the new test caught.

Test. The planning case at the end of `web/tests/gaffer-outcome.e2e.mjs` holds the start's answer, presses Stop,
releases it and expects the DELETE and no panel. It was not run against the code before the fix.

Still true. A start that fails on the network after the controller accepted it leaves a job the browser never learns
the ID of. It ends by itself at 90 seconds.
