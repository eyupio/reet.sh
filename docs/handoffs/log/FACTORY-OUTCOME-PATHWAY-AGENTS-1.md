## FACTORY-OUTCOME-PATHWAY-AGENTS-1: the outcome dialog says which agent each pathway stage really runs on (27 November 2026)

When a factory pathway was chosen in the New Factory Outcome dialog, the dialog said its stages, models, skills and
prompts came from the pathway, but it did not say which agent or model each stage would actually use. A pathway stage
can name its own agent; otherwise the controller takes the saved crew member for the stage's tier, and only falls back
to the outcome's own agent when that tier has no saved crew. The dialog's launch readiness knew none of that, so a
stage could look like a failure or a success for the wrong reason.

What changed.

- `resolvePathwayRuns` in `web/src/lib/factory/pathways.ts` now centralises the resolution the controller already did:
  a stage's own agent first, then its tier's saved crew with that member's backups, then the outcome's agent. It returns
  an `origin` for each stage (`pathway`, `saved-crew`, `outcome`, `unknown-crew`) and reuses `GafferCrew` from `api.ts`.
  Runtime behaviour is unchanged; the function replaces the copy of this logic in `GafferOutcome.svelte`.
- `launchReadiness` gained the `pathway` and `unknown` sources. The readiness list now labels `From the pathway`, `Saved crew`,
  `Outcome's agent` or `Crew not known`, so the source of the effective agent is stated rather than implied.
- The pathway section of `GafferOutcome.svelte` lists every stage with its tier and its readiness, and says that a stage
  with its own agent runs on it, that its tier's saved crew is next, and that the agent chosen here is used only where
  no crew is saved. It also warns when a crew tier was edited before the pathway was picked and will not be sent.
- While the saved crew is being read, the stage list and the launch-readiness panel are held behind one "Reading your
  saved Factory crew…" line, so a slow read is not shown as stages with no agent. A failed read now says the saved crew
  could not be read and holds the start; it no longer reports "No agent is chosen for this tier".

Checked. `npm test` in `web/` (133 test files) and `npx svelte-check --tsconfig ./tsconfig.json` (0 errors, 0 warnings).
New tests in `pathways.test.mjs` cover a stage's own agent, a saved crew member with backups, a tier with no crew
falling back to the outcome's agent, a stage's explicit tier, and an unread crew. `launchReadiness.test.mjs` covers the
four source labels and the "crew could not be read" blocker.

Not checked. The dialog was not driven in a browser. The loading-state timing is covered by logic tests only.

Deliberately not changed.

- `AddPipelines.svelte` still falls back to the pipeline's own agent when no saved crew member resolves, and its `catch`
  swallows a failed crew read. It is the same bug class in the other entry point; fixing it changes behaviour and needs a
  decision, so it is left for follow-up.
- The dialog reads the crew once when it opens and does not retry. A transient failure now leaves a blocker that only a
  page reload clears; adding retry is new UI affordance and was left for a decision.
- The outcome's agent is still included in the trust/YOLO checks even when no stage uses it. This predates the change.
