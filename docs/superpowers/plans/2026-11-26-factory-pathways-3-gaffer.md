# Factory pathways, part 3: Gaffer and the MCP tools — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Gaffer and MCP clients can read the person's enabled pathways, start work on one, name any stage of a pathway pipeline, and Gaffer's planning can suggest a pathway that the person sees already selected.

**Architecture:** The MCP tool schemas are written in `mcp/src/server.ts` and exported to `internal/api/mcp_tools.json`; Gaffer's in-process tools reuse those definitions, so one change serves both. Starting on a pathway over MCP goes through the same `UsePathway` and skill-receipt code as the HTTP handlers. Planning gives the model only each enabled pathway's name, description and stage names, never its prompts, and the browser preselects the suggestion.

**Tech Stack:** Go, TypeScript (the MCP wrapper, zod), Svelte 5. No new dependency.

**Spec:** `docs/superpowers/specs/2026-11-25-factory-pathways-design.md`, section "Gaffer and the MCP tools".

## Global Constraints

- British spelling, plain words, no em dashes in UI copy or tool descriptions. Commit subjects are an imperative sentence with no prefix. Do not name other products.
- `internal/api/mcp_tools.json` is generated: change `mcp/src/server.ts`, then `npm run export-tools` in `mcp/`. `npm test` there fails on a stale file.
- A disabled pathway is offered nowhere: not listed by the tool, not given to the planning model, refused at start.
- Pathway prompts never go to Gaffer's planning model or into a tool result. Names, descriptions and stage names do.
- Gaffer never starts work on a pathway except through the start tools under the person's existing off/ask/allow settings.
- Tool errors are sentences ending in what did not happen ("Nothing was started."), as the Factory tools' are today.
- Standard starts over MCP (no `pathway`) behave exactly as before, including the default stages.
- Colours only from tokens; type no smaller than 14 px; 44 px targets on coarse pointers.
- The pull request adds `docs/handoffs/log/PATHWAYS-3.md` and `changelog.d/pathways-gaffer.md`.
- Do not skip, retry or lengthen a timeout to get green.

## Review Focus

1. An MCP start that names a pathway AND gives stages, tiers or parts: refused with one sentence, nothing started.
2. A pathway named over MCP by a name that two pathways could match (names are unique without case, so "ship" and "Ship" are the same one), by an unknown name, or by one that is switched off: the right one, or a refusal that lists the enabled names.
3. Gaffer's planning model answering with a pathway that does not exist, is off, or has a problem: the suggestion is dropped silently and the plan is otherwise used.
4. `reet_send_pipeline_back` or `reet_move_pipeline_on` with a stage name that is not in that pipeline: refused, with the pipeline's stage names in the sentence.
5. An account with no pathways: the planning prompt, the plan and every tool result are exactly as they are today.

---

### Task 1: The MCP tools

**Files:** Modify `mcp/src/server.ts` (:118-162), `internal/api/mcp_tools.json` (generated), `internal/api/gaffer_factory_mcp.go`, `internal/api/gaffer_tools.go` (:51-53), `internal/api/gaffer_factory_mcp_test.go`, `internal/api/mcp_test.go` (:334, :341), `mcp/test/protocol.test.mjs` (:12, :217, :218), `mcp/test/oauth.test.mjs` (:40).

**Interfaces — Produces:**

- New read tool `reet_list_pathways` (input `{response_format}` only; not `{write:true}`): `{"pathways":[{"id","name","description","stages":["Plan","Security audit",…],"problems":[…]}]}`, enabled pathways only, stage names as labels (built-in label or the custom name), `problems` from `PathwayProblems`. Added to `mcpFactoryReads`, and to the exclusion in `gafferToolReturnsSessionText` (it returns no session text).
- `reet_start_outcome` and `reet_add_pipelines` gain optional top-level `pathway` (string, 1 to 60): a pathway's id or its exact name without case. With it: `mcpPipelines` must NOT fill in default stages; a pipeline that gives `stages`, `tiers` or `parts` is refused ("A pipeline takes a pathway or its own stages, not both. Nothing was started."); the handler resolves the name to an id among enabled pathways (unknown or off: "No pathway called “x” is switched on. The ones that are: A, B. Nothing was started."), then calls `UsePathway(uid, id, sessions, s.templateSkills(uid))` and, after a successful create or add, `recordStageSkills` exactly as the HTTP handlers do (`gaffer_outcomes.go:124-137`, `:498-512`); lift the shared part into one helper both paths call.
- In `server.ts`, one shared `stageKind` zod enum replaces the four inline copies for `stages`, `tiers`; the `stage` argument of `reet_move_pipeline_on` and `reet_send_pipeline_back` becomes a string of 1 to 40 characters described as "The stage's name as the outcome lists it: a built-in stage (plan, design, code, review, test, pr, babysit, merge) or one of the pathway's own." The Go handlers already match by label; when no stage of that pipeline has the label, the refusal lists the pipeline's stage labels.
- `mcpOutcome`'s stage view (:170) gains `"name": stage.Label()` beside `"kind"`, and the pipeline gains `"pathway": pathwayName` when it has one.

- [ ] **Step 1: failing tests.** Go, modelled on `factoryMCPFixture` and `TestFactoryMCPStartIsRefusedWhenTheMachineHasNotSaidWhatItRuns`: `TestMCPListsEnabledPathways` (two enabled, one disabled, one of another account: only the two, with stage labels, no prompt text anywhere in the result); `TestMCPStartsAnOutcomeOnAPathway` (by id and by name in another case; stored stages are the pathway's; a skill receipt exists for a stage with a skill); `TestMCPPathwayStartRefusals` (pathway plus stages; unknown name lists the enabled ones; disabled); `TestMCPAddsPipelinesOnAPathway`; `TestMCPStandardStartStillDefaultsItsStages` (no pathway: code, review, test, pr, babysit, merge as today); `TestMCPNamesACustomStage` (outcome view carries `name`; send-back to "Audit" works; "Zed" is refused and the sentence lists the stages). Update the tool counts (49 → 50; the member read count and the read-scope bridge count each +1). `mcp/`: update the expected counts.
- [ ] **Step 2:** `go test ./internal/api -run 'MCP|Factory'` and `cd mcp && npm test` fail. **Step 3:** implement; `npm run export-tools`. **Step 4:** both pass; `go test ./internal/api` passes whole.
- [ ] **Step 5:** Commit "Let MCP clients list and start pathways".

### Task 2: What Gaffer sees of a pathway's stages

**Files:** Modify `internal/gaffer/context.go` (:162-208), `internal/api/skill_buddy.go` (:55, :72), tests beside each.

- `ContextPipelineStage` gains `Name string` (`json:"name,omitempty"`), set to the stage's label when the kind is `custom`; the pipeline's context entry gains its pathway's name when it has one. Wherever the context is rendered to text for the model, a custom stage reads by its name.
- `buddyScope`: for a custom stage, pass no stage kind to `skillbuddy.Recommend` and add the stage's name and prompt to the task words, so recommendations follow what the stage is for. Built-in stages are unchanged.

- [ ] **Step 1: failing tests:** `TestContextNamesACustomStage` (a pathway pipeline's context lists "Audit", not "custom", and carries the pathway's name); `TestSkillBuddyRanksACustomStageByItsPrompt` (a custom stage whose prompt is about security ranks a security skill above an unrelated one; a built-in stage's ranking is unchanged from a captured expectation).
- [ ] **Step 2:** fail. **Step 3:** implement. **Step 4:** `go test ./internal/gaffer ./internal/api ./internal/skillbuddy` passes. **Step 5:** Commit "Show Gaffer a pathway stage by its name".

### Task 3: Planning suggests a pathway

**Files:** Modify `internal/gaffer/outcomes.go` (`planSessionsTool` :90, `outcomeSystem` :92-97, `preparePlan` :201-224, `outcomeSessions` :415-541, `OutcomePlan` :81-88, the comment at :19), `internal/gaffer/outcomes_test.go`.

**Interfaces — Produces:**

```go
// OutcomePlan gains:
Pathway *PlanPathway `json:"pathway,omitempty"`
type PlanPathway struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Why  string `json:"why,omitempty"` // one line, at most 200 bytes
}
```

- `preparePlan` reads the account's enabled pathways (`s.store.GafferPathwaysFor`), at most 20 by name, and when there are any appends to the user text: `Pathways the person has saved (optional): ` then one line each, `- <name>: <description> [<stage labels joined by " › ">]`. No prompts, models or skills. The system text gains one sentence: "If one of the person's saved pathways clearly fits the objective, name it in `pathway` with a short reason in `pathway_why`; otherwise leave both out. Still propose stages for every session."
- `planSessionsTool` gains optional `pathway` (string) and `pathway_why` (string) at the top level only when the account has enabled pathways (so with none the schema sent is byte-for-byte today's).
- Parsing: the answer's `pathway` is matched without case against the enabled pathways' names; no match, or an empty value, drops the suggestion and keeps the plan. `Why` is clipped to 200 bytes and must pass `store.ValidWorkerText`, else it is dropped (the suggestion stays).
- The comment at :19 is rewritten to say exactly what planning now reads.

- [ ] **Step 1: failing tests** (with `planServer`/`planCall`): `TestPlanSuggestsAPathway` (the request's user text lists the two enabled pathways with their stage names and contains none of their prompt text; the disabled one is absent; the answer names one in another case → `plan.Pathway` has its id, name and why); `TestPlanDropsAnUnknownPathway` (plan still returned, `Pathway` nil); `TestPlanWithNoPathwaysIsUnchanged` (Review Focus 5: the request body sent to the provider equals the body sent before this change, captured as a golden string in the test).
- [ ] **Step 2:** fail. **Step 3:** implement. **Step 4:** `go test ./internal/gaffer` passes. **Step 5:** Commit "Let Gaffer's plan suggest a saved pathway".

### Task 4: The dialog shows the suggestion

**Files:** Modify `web/src/lib/api.ts` (`GafferOutcomePlan` :295), `web/src/lib/GafferOutcome.svelte` (`ask()` :448-460, `sendReply()` :469-490, the `<PathwayChoice>` at :659), `web/tests/pathways-choice.e2e.mjs`.

- `GafferOutcomePlan` gains `pathway?: { id: string; name: string; why?: string }`.
- When a plan arrives with a pathway that is in the dialog's offered list (enabled, no problems), set `pathwayId` to it and pass `suggested={{ id, why }}` to `PathwayChoice` (which already renders "Gaffer suggests this: …"). If the person has already chosen something in this dialog (including "Standard stages") a later plan (a reply) does not change their choice; it only updates `suggested`. A suggestion that is not offered is ignored.
- The suggested row is always in the folded list of six.

- [ ] **Step 1:** Extend `pathways-choice.e2e.mjs`: a plan with a pathway preselects it and shows the reason; the POST carries it; choosing "Standard stages" then asking Gaffer again keeps "Standard stages" and still shows the suggestion; a plan naming a pathway with problems selects nothing; the standard baselines still pass. Fail first.
- [ ] **Step 2:** implement. **Step 3:** suite passes; `npm run check` passes; the six dialog suites pass (factory-templates, gaffer-outcome, gaffer-factory, factory-delivery, session-end-gaffer, workbench).
- [ ] **Step 4:** Commit "Preselect the pathway Gaffer suggests".

### Task 5: Polish carried from part 2

**Files:** `web/src/lib/factory/pathways/PathwayChoice.svelte`, `PathwayEditor.svelte`, tests in the two pathways suites.

- [ ] Fix, each with a check: (1) PathwayChoice: a chosen pathway lifted to the top of the folded list stays in the list when another row is chosen, so rows never shift under the pointer (keep the folded set stable until the list is reopened). (2) The "Edit pathway" link sits outside the `<label>` (still inside the chosen row visually) so it is not part of the radio's name. (3) The drag hint stays inside the window: when it would fall below the fold it sits above the line instead. (4) The palette dock's height leaves room for the fixed bottom tab bar when that bar is showing.
- [ ] Both pathways suites pass at their widths and themes; `npm run check` passes. Commit "Steady the pathway choice and keep the drag hint in view".

### Task 6: Docs and notes

**Files:** `docs/mcp.md` (:174-230), `docs/factory.md` (:500-507 and the Pathways section), `docs/handoffs/log/PATHWAYS-3.md`, `changelog.d/pathways-gaffer.md`.

- [ ] Document `reet_list_pathways`, `pathway` on the two start tools, and that a stage is named as the outcome lists it. In the Pathways section add "With Gaffer and AI clients": Gaffer may suggest a pathway when it plans, shown already selected with its reason; it reads names, descriptions and stage names only. Handoff entry (what, why, checked, not done) and changelog fragment. `cd mcp && npm test`, `cd web && npm run check`. Commit "Document pathways for Gaffer and AI clients".

### Task 7: Resolve a pathway's agents in the dialogs as the server does

Carried from the automated review of the previous pull request (both findings checked against the code and confirmed).

**Files:** `web/src/lib/GafferOutcome.svelte` (`savedHere` :220, `pathwayRuns` :358-361, `runTools` :363, the launch summary :367-368), `web/src/lib/factory/AddPipelines.svelte` (its pathway checks), `web/tests/pathways-choice.e2e.mjs`.

- A pathway stage on a crew tier runs on the SAVED tier member whenever that tier is set, whether or not the machine has that agent (`outcomePieces`, `internal/gaffer/outcome_runs.go:673`); only an unset tier falls back to the outcome's own agent. The dialogs must resolve the same way and then report a saved agent that is not on the machine as a blocker naming the stages and the tier ("Your Standard crew is Codex, which is not on <machine>. …"), not swap in the outcome's agent.
- The agents a pathway "can run on" include each tier's backups (`member.Backup`, `member.More`), as `outcomeRunsOn`/`stageAgents` count them on the server: the folder-trust box, the Antigravity blocker and Add pipelines' untrusted-outcome refusal must consider them.

- [ ] **Step 1: failing checks** in `pathways-choice.e2e.mjs`: a pathway with a Standard-tier stage, saved Standard crew on an agent the machine does not have → Approve is held with the sentence naming the stage and tier (today it shows the outcome's agent and allows it); saved tier whose backup is Google Antigravity, primary needs no trust → the trust box shows, and with it ticked the POST carries `trust: true`; the same two for Add pipelines where they apply. The Standard stages baselines still pass.
- [ ] **Step 2:** implement. **Step 3:** suite, `npm run check` and the six dialog suites pass. **Step 4:** Commit "Resolve a pathway's agents in the dialogs as the server does".
