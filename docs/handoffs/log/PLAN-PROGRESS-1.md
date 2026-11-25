## PLAN-PROGRESS-1: the dialog shows the step a Gaffer plan is on, and Stop ends it (25 November 2026)

While Gaffer planned an outcome the dialog showed only a button reading "Gaffer is planning…" for up to 90 seconds.
The owner asked for the wait to show what is happening, and chose real steps reported by the controller over a timer
alone or a live stream of the plan (a stream needs a held connection that proxies buffer, and many models send the
plan in one piece).

What changed.

- `internal/gaffer/outcomes.go`: a plan job has a step. `executePlan` takes a `step` function and reports
  `PlanSending` until the provider accepts the request, `PlanWriting` while it answers and `PlanChecking` once the
  answer is in. `PlanPhase` reads it; a failed job keeps the step it failed on. `StopPlan` cancels the provider request
  and forgets the job, which frees the person's place (a person may have two provider requests at once).
- `internal/api/gaffer_settings.go`: the poll answers `{"state":"running","phase":"…"}`, and
  `DELETE /api/v2/gaffer/providers/{id}/plan/{job}` stops a job (204, or 404 for a job that is not this person's).
- `web/src/lib/api.ts`: `planGafferOutcome` takes an `onPhase` callback, and when its signal is aborted it sends the
  DELETE, so closing the dialog ends the plan too.
- `web/src/lib/GafferOutcome.svelte`: a `planProgress` snippet, used by the first ask and by a reply. Three steps with
  the product's marks (✓ done, … current, · to come, ✕ failed), the time so far against the 1:30 limit, and a Stop
  button. A failed plan keeps the list, with ✕ on the last step the browser was told about, above the sentence saying
  why. The current mark pulses, not under reduced motion. The list is a `status` region; the clock is outside it so a
  screen reader is not told every second.

Tests. `TestAPlanJobReportsItsStepAndStoppingItEndsTheProviderRequest` (`internal/gaffer`), the stop route in
`TestGafferOutcomePlanNeedsTheOwningBrowserAndStartsNothing` (`internal/api`), and a new case at the end of
`web/tests/gaffer-outcome.e2e.mjs` at 1440 px light and 390 px dark: running, stopped, failed.

Known limits.

- The browser polls once a second, so a fast plan can skip a step on screen, and a plan that fails just after a step
  changes shows ✕ on the step before.
- "Checking the plan" takes milliseconds; it is in the list because a plan can fail there.
- The 1:30 in the copy is the 90 seconds in `StartPlan`. Change both together.
