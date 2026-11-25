## PATHWAYS-1: The server can store, serve and start Factory pathways, and the web client names a custom stage (25 November 2026)

**What:** the engine for Factory pathways (a person's saved, ordered line of stages, built from the eight built-in
kinds and stages of their own). The server stores pathways, serves them at `/api/v2/gaffer/pathways`, and can start an
outcome on one. A stage of kind `custom` carries a `name` and a `prompt`; a pipeline from a pathway carries `pathwayId`
and `pathwayName`. Wherever a `stage` value is sent or returned (send-back, move-on, decisions, the delivery review's
stage list) the server now names a stage by its label: the name for a custom stage, the kind for a built-in one. The
web client shows and sends labels: `stageName` (what is shown) and `stageKey` (what is sent and compared) in
`web/src/lib/gafferOutcome.ts`, used by the track, the send-back and move-on dialogs, the decision cards, the usage
limit note and the Factory list. A pipeline's settings say "Pathway: <name>", and a pipeline from a pathway is not
offered as a template.

**Why:** the owner asked for pathways (see PATHWAYS-0). This is the first of three pull requests; it leaves a person
nothing new to click, but everything after it stands on it.

**Checked:** the web unit tests (`stageName`, `stageKey`, `canSaveAsTemplate` with a pathway) and `npm run check` in
`web/`. Standard pipelines are untouched: for a built-in stage `stageName` and `stageKey` give what `stageLabel` and
`kind` gave before. The Go suite and the browser suites were run by the earlier tasks and were not needed for this one.

**Not done:** there is no page to build a pathway yet (the next pull request). Gaffer and the MCP tools cannot use
pathways yet (the third pull request), and over MCP an agent cannot yet see the name of an earlier custom stage.
