# Factory pathways and templates: audit

Date: 27 November 2026. Scope: how Factory pathways and Factory templates are defined, stored, listed, selected and
consumed today, and whether each earns its keep. This is a reading of the code, tests, data shapes and shipped docs on
`main` at 834121f9; no application was run and no benchmark was taken. Method and limits are in section 7.

**Bottom line.** Pathways are a complete, account-scoped, well-tested feature with real CRUD, optimistic concurrency,
owner isolation, a working editor, a start path through the browser and MCP, and a Gaffer planning suggestion. Their
weak points are discoverability, not plumbing: the description written to help a person judge fit is never shown in
the chooser, the choice cannot be made on the pathways page, and an ordinary Agent session cannot use a pathway at
all. Templates are not pointless, but for a pathway user they are close to a dead end: a successful pathway outcome
cannot be saved as a template, a template's plan cannot be edited except by running it again, and neither Gaffer nor
MCP can see templates. The verdict is **keep Templates and fix their relationship to pathways**, not remove them
(section 5). Guidance recommendations are in section 6.

## 1. Where the pieces live

| Piece | Server | Store | Browser | Docs |
| --- | --- | --- | --- | --- |
| Pathway CRUD and skills shelf | `internal/api/gaffer_pathways.go` | `internal/store/store_gaffer_pathways.go` | `web/src/lib/factory/pathways/` | `docs/factory.md` "Pathways" |
| Pathway rules and start | `internal/gaffer/pathways.go` | | `web/src/lib/factory/pathways.ts` | |
| Pathway in planning | `internal/gaffer/outcomes.go` | | `web/src/lib/GafferOutcome.svelte` | |
| Pathway in MCP | `internal/api/gaffer_factory_mcp.go` | | `mcp/src/server.ts` | `docs/mcp.md` |
| Template CRUD, versions, launch | `internal/api/gaffer_templates.go`, `internal/gaffer/templates.go` | `internal/store/store_gaffer_templates.go` | `web/src/lib/factory/Templates.svelte`, `Template*.svelte` | `docs/factory.md` "Templates" |

The two features are deliberately separate: the pathway design rejected growing Templates into pathways and kept
"no change to Factory templates" (`docs/superpowers/specs/2026-11-25-factory-pathways-design.md:30,49-51`).

## 2. Pathway CRUD

### 2.1 Routes, access and ownership

All six routes are registered in `internal/api/gaffer_pathways.go:15-27`:

```go
GET    /api/v2/gaffer/pathways
POST   /api/v2/gaffer/pathways
GET    /api/v2/gaffer/pathways/skills
GET    /api/v2/gaffer/pathways/{id}
PUT    /api/v2/gaffer/pathways/{id}
DELETE /api/v2/gaffer/pathways/{id}
```

Every route is wrapped in `featureAccess` (`internal/api/gaffer_pathways.go:16-20`), which admits only a signed-in
browser account, refuses API tokens and read-only accounts, and requires a same-origin request for anything that is not
a `GET` (`internal/api/projects.go:57-75`). Owner isolation is enforced in every SQL statement, not in the handler
(`WHERE user_id=?` at `internal/store/store_gaffer_pathways.go:79,96,125,135,151`), and the API tests cover signed-out,
read-only and another account's pathway (`internal/api/gaffer_pathways_test.go:161-221`). There is no MCP or API-token
write path; the only MCP route is a read (section 4.4).

### 2.2 Create

`gafferPathwayCreate` (`internal/api/gaffer_pathways.go:82-96`) decodes a strict JSON body of at most 128 KiB, forces
`ID`, `CreatedAt` and `UpdatedAt` to zero, and calls `SavePathway(uid, in, 0)`. The service trims and validates the
pathway, gives it a new chat ID, and asks the store to insert (`internal/gaffer/pathways.go:146-160`). The store checks
the account limit of 50 and inserts with the new timestamps (`internal/store/store_gaffer_pathways.go:12,112-122`).
A successful create is `201` with the saved pathway plus its current `problems` (`internal/api/gaffer_pathways.go:94-95`).

The table is `gaffer_pathways`, with `UNIQUE(user_id,name)` and the name column `COLLATE NOCASE`
(`internal/store/store_gaffer_pathways.go:47-56`). A duplicate name returns `409 pathway_name`
(`internal/store/store_gaffer_pathways.go:17,139-141`; `internal/api/gaffer_pathways.go:35-36`).

**Finding P1 (latent API gap, minor).** `cleanPathway` never sets `Enabled`, and the store inserts whatever it is
given (`internal/gaffer/pathways.go:52-127`; `internal/store/store_gaffer_pathways.go:121-122`). The design says a new
pathway starts enabled (`docs/superpowers/specs/2026-11-25-factory-pathways-design.md:73`). The web client does send
`enabled: true` (`web/src/lib/factory/pathways.ts:25`), so this is invisible today, but any other writer that omits
the field creates a pathway that is off. The existing API test deliberately creates one with `"enabled":false` and
asserts it stays off (`internal/api/gaffer_pathways_test.go:100-118`), so the omission is not currently caught.

### 2.3 Read

- List: `gafferPathways` reads all of the account's pathways and adds `problems` to each (`internal/api/gaffer_pathways.go:58-70`).
  Ordering is by name without case, then id (`internal/store/store_gaffer_pathways.go:78-93`).
- One: `gafferPathway` returns the same view for one id (`internal/api/gaffer_pathways.go:72-80`).
- Skills shelf: `gafferPathwaySkills` lists built-in skills that are on plus the account's Active library skills, each
  with the bytes it adds (`internal/api/gaffer_pathways.go:129-164`).

All three are used: the list by the Pathways page and both start dialogs, the single read by the editor, and the
skills shelf by the editor's palette. There is no dead read route. Disabled pathways are returned by the browser
list, which is how the list can show them and offer the switch; MCP filters them out (section 4.4).

**Finding P2 (performance, unmeasured).** A list read computes `problems` for every pathway
(`internal/api/gaffer_pathways.go:50-56,65-68`). `PathwayProblems` loads every skill on every stage
(`internal/gaffer/pathways.go:171-210`), and each load calls `buddyLoad`
(`internal/api/gaffer_templates.go:52-54`), which calls `buddySystem` every time (`internal/api/skill_buddy.go:137-148`).
`buddySystem` reads a setting and rebuilds the built-ins list (`internal/api/skill_buddy_admin.go:25-46`), and a library
skill adds two more store reads (`internal/api/skill_buddy.go:149-174`). Fifty pathways of twelve stages with two
skills each is up to 1,200 such loads for one list request. I did not measure the cost, so this is a code-shape
observation, not a confirmed latency problem. It also means the same skill is loaded repeatedly within one response.

### 2.4 Update

`gafferPathwayEdit` requires `updatedAt` in the body to be at least 1, takes the id from the path (ignoring any id in
the body), and passes the read time to the service (`internal/api/gaffer_pathways.go:98-117`). The store compares that
read time with the row's `updated_at` and refuses with `ErrGafferPathwayChanged` if it has moved on
(`internal/store/store_gaffer_pathways.go:123-136`). The new `updated_at` is `max(now, old+1)`, so two quick saves
cannot share a timestamp (`internal/store/store_gaffer_pathways.go:134`). The web client sends the pathway's
`updatedAt` on every save, including the enable switch (`web/src/lib/factory/pathways.ts:175-178`;
`web/src/lib/factory/pathways/PathwayList.svelte:33-47`). A stale PUT is `409 pathway_changed`
(`internal/api/gaffer_pathways.go:30-32`), covered by `TestPathwayCreateListAndStalePut`
(`internal/api/gaffer_pathways_test.go:100-140`).

### 2.5 Delete

`gafferPathwayDelete` deletes by owner and id and returns `204`; a missing row is `404`
(`internal/api/gaffer_pathways.go:119-127`; `internal/store/store_gaffer_pathways.go:150-158`). Running work is safe:
the outcome copied the pathway's stages when it started, and later edits or a delete do not touch it
(`internal/gaffer/outcomes.go:630-642`; `TestRunningWorkKeepsItsStages` at `internal/gaffer/pathways_test.go:195`).
The UI confirmation says exactly that (`web/src/lib/factory/pathways/PathwayList.svelte:164`).

### 2.6 Validation

`cleanPathway` (`internal/gaffer/pathways.go:52-128`) and `validPathwayOrder` (`internal/gaffer/pathways.go:31-49`)
enforce:

| Rule | Code |
| --- | --- |
| Name 1 to 60 plain characters, unique without case | `internal/gaffer/pathways.go:54-56`; `internal/store/store_gaffer_pathways.go:51-54,139-141` |
| Description at most 300 bytes of worker text | `internal/gaffer/pathways.go:57-59` |
| 1 to 12 stages | `internal/gaffer/pathways.go:60-62` |
| A built-in stage at most once | `internal/gaffer/pathways.go:34-40` |
| Merge last | `internal/gaffer/pathways.go:44-46` |
| Babysit and Merge after a PR stage | `internal/gaffer/pathways.go:41-43` |
| Custom stage: name 1 to 40, not `custom`, not a built-in, unique in the pathway, non-empty prompt | `internal/gaffer/pathways.go:69-82` |
| A built-in stage has no name of its own | `internal/gaffer/pathways.go:83-87` |
| Prompt at most 4,000 bytes of plain text, custom or built-in | `internal/gaffer/pathways.go:88-90` |
| A stage has a tier or an agent, not both; default tier when neither | `internal/gaffer/pathways.go:91-108` |
| At most 2 skills, each once | `internal/gaffer/pathways.go:109-120` |
| Stage 50-per-account limit | `store_gaffer_pathways.go:12,112-119` |

Skill IDs are checked for shape only at save: existence, size and availability are checked when the pathway is
listed or started (`internal/gaffer/pathways.go:186-199,207-210`). That is why a pathway can be saved with a missing
skill and only later show a `problems` sentence; the API test covers that (`internal/api/gaffer_pathways_test.go:142-159`).
This is deliberate and correct for a skill that can be archived after the pathway is saved.

**Finding P3 (duplication, maintenance).** The order rules and most field rules exist twice: in Go
(`internal/gaffer/pathways.go:31-128`) and in TypeScript (`web/src/lib/factory/pathways.ts:40-52,95-132`). The client
copy exists so the editor can refuse before the server is asked, and its comment says it mirrors the server
(`web/src/lib/factory/pathways.ts:4-5`). The two are currently consistent, but any change has to be made in both
places, and there is no test that compares the two rule sets.

## 3. Selection and usage

### 3.1 A Factory outcome picks a pathway in the browser

`web/src/lib/GafferOutcome.svelte` reads the account's pathways when it opens (`web/src/lib/GafferOutcome.svelte:446`) and renders
`PathwayChoice` when any are offered (`web/src/lib/GafferOutcome.svelte:688`). The choice lists only pathways that are
on, shows a `!` and a first problem for one that cannot start, and always offers "Standard stages" as the empty value
(`web/src/lib/factory/pathways/PathwayChoice.svelte:15-18,82-103`). Choosing a pathway hides the per-pipeline stage
ticks and replaces them with the pathway's line and the agent each stage runs on
(`web/src/lib/GafferOutcome.svelte:699-721`).

On start, the browser sends `pathway: pathway.id` and omits each pipeline's own stages
(`web/src/lib/GafferOutcome.svelte:573-574`). The API decodes a `pathway` field beside the normal outcome input
(`internal/api/gaffer_outcomes.go:107,134`), and `UsePathway` refuses if a pipeline already has stages, if the pathway
is missing or off, or if a skill cannot be loaded, and otherwise copies the stages into every pipeline and records a
`GafferPathwayRef` (`internal/gaffer/pathways.go:216-243`). `outcomePieces` then re-checks the copied line for pathway
shape and order (`internal/gaffer/outcome_runs.go:630-642`). The pipeline stores `pathwayId` and `pathwayName`
(`internal/store/store_gaffer_outcomes.go:199-202`), and the pipeline detail shows the name
(`web/src/lib/factory/PipelineDetail.svelte:423`).

`web/src/lib/factory/AddPipelines.svelte` uses the same choice (`web/src/lib/factory/AddPipelines.svelte:175`), starts on the pathway the
outcome's first pipeline used if it is still on and startable (`web/src/lib/factory/AddPipelines.svelte:162-165`), and
sends the chosen pathway with the new pipelines (`web/src/lib/factory/AddPipelines.svelte:168`).

### 3.2 Gaffer's suggestion

When planning, the server lists up to 20 enabled pathways by name, description and stage labels, never prompts, models
or skills (`internal/gaffer/outcomes.go:254-287`), and adds an optional `pathway` and `pathway_why` to the planning tool
only when at least one pathway exists (`internal/gaffer/outcomes.go:104-111`). The returned name is matched without case
against the offered pathways, and anything else is dropped (`internal/gaffer/outcomes.go:620-638`). The dialog
auto-selects the suggestion only while the person has not touched the choice, and only if the pathway is on and has no
problems (`web/src/lib/GafferOutcome.svelte:103-117`); the reason is said on the row
(`web/src/lib/factory/pathways/PathwayChoice.svelte:88`). The behaviour is documented at `docs/factory.md:501-509`.

The suggestion is plan-only. `AddPipelines` renders the choice without a `suggested` prop
(`web/src/lib/factory/AddPipelines.svelte:175`), so adding pipelines gets no suggestion even though Gaffer planned the
original outcome. A revised plan that does not name a pathway leaves the earlier suggestion in place
(`web/src/lib/GafferOutcome.svelte:509-510`).

### 3.3 An ordinary Agent session does not pick a pathway

No ordinary session path reads pathways. No terminal, session or broker package mentions one; outside the store, the
Gaffer package, the Factory API handlers and the MCP tool code, the server references are route registration
(`internal/api/api.go:432`) and the browser-route list (`internal/webroute/routes.go:14,41-42`). The Start Agent dialog
has no pathway control (`web/src/lib/StartAgentDialog.svelte`), and `docs/start-agent-session.md` does not mention
pathways. A pathway today only shapes a Factory pipeline's stages. The word "pathway" in the product does not mean
"how an Agent session starts", even though a person could reasonably read it that way.

### 3.4 What happens when the wrong pathway is picked

Nothing checks that a pathway fits the objective. The outcome copies its stages and records its id and name; after
that the stages are the pipeline's own (`internal/gaffer/outcomes.go:630-642`;
`internal/store/store_gaffer_outcomes.go:199-202`). There is no route or UI to change a started pipeline's pathway.
The person can stop the outcome, send a stage back with a reason, or move it on, but not switch the line. The pipeline
detail only displays the pathway name (`web/src/lib/factory/PipelineDetail.svelte:423`). A wrong choice is corrected by
stopping the outcome or by working within the wrong line; the pathway cannot be swapped. The start dialogs do show the
full line and each stage's agent before the start button (`web/src/lib/GafferOutcome.svelte:709-721`), which is the
only guard.

### 3.5 Discoverability in the chooser

**Finding S1 (strong).** The description is the field written for judging fit. The editor invites it with
"Describe when to use this pathway" (`web/src/lib/factory/pathways/PathwayEditor.svelte:509-513`), the list shows it
(`web/src/lib/factory/pathways/PathwayList.svelte:130`), Gaffer is given it (`internal/gaffer/outcomes.go:268-286`),
and the design says it is "shown in the list and given to Gaffer so it can judge fit"
(`docs/superpowers/specs/2026-11-25-factory-pathways-design.md:72`). The chooser does not show it. It shows the name,
the stage chips, Gaffer's reason and the first problem (`web/src/lib/factory/pathways/PathwayChoice.svelte:86-90`); the
description is used only as a hidden search field (`web/src/lib/factory/pathways/PathwayChoice.svelte:27`). Two
pathways with similar stages and names are therefore indistinguishable at the moment of choice.

**Finding S2 (consistency, minor).** The design says "Standard stages" comes first in the select
(`docs/superpowers/specs/2026-11-25-factory-pathways-design.md:228`). The implementation renders it last, after every
pathway (`web/src/lib/factory/pathways/PathwayChoice.svelte:98-103`). With more than six pathways it also sits after the
folded list of six, immediately before the "Show all" control (`web/src/lib/factory/pathways/PathwayChoice.svelte:98-105`). This favours the saved
pathways over the default, which may be intended, but it is not what the design said and it makes the no-pathway
choice harder to find.

**Finding S3 (discovery, minor).** There is no place to compare pathways. The Pathways page shows stage lines,
descriptions and problems (`web/src/lib/factory/pathways/PathwayList.svelte:120-152`), but a person who is in the
start dialog cannot open it without leaving the dialog, and the chooser's "Edit pathway" link opens only the chosen
pathway in another tab (`web/src/lib/factory/pathways/PathwayChoice.svelte:94`). There is no "which pathway fits?"
affordance; the only guidance is Gaffer's one-off suggestion during planning.

## 4. Other consumption paths

### 4.1 Stage prompts

A pathway supplies each stage's prompt. For a custom stage it replaces the built-in stage instructions; for a built-in
stage it is appended as extra instructions (`internal/gaffer/outcome_runs.go:320-332`). The pipeline's own prompt still
carries the work brief (`internal/gaffer/outcome_runs.go:370-375`), so a wrong pathway plus the right brief can send
conflicting instructions. Nothing reconciles the two.

### 4.2 Gaffer's context

A running pipeline's context names its pathway (`internal/gaffer/context.go:156,189`) and a custom stage by its own name,
never its prompt (`internal/gaffer/context.go:166-169`). Gaffer therefore knows which line the session is on but not
the custom stage's instructions.

### 4.3 Status sharing

When status sharing is on, a Factory session's status includes the names of its stages and its pathway
(`web/src/lib/ContextSharing.svelte:59`). This is a privacy consequence of naming pathways, not a CRUD defect.

### 4.4 MCP and AI clients

`reet_list_pathways` exists (`internal/api/mcp_tools.json:847-849`) and returns enabled pathways only, by id, name,
description, stage labels and problems; it pages the result (`internal/api/gaffer_factory_mcp.go:389-412`). It filters
out disabled pathways at `internal/api/gaffer_factory_mcp.go:396-398`. `reet_start_outcome` and `reet_add_pipelines` take an optional
`pathway` by id or exact name, matched id-first, and refuse a disabled or unknown name with the names that are on
(`internal/api/gaffer_factory_mcp.go:620-660`). An AI client cannot create, edit, enable, disable or delete a pathway.
That matches the design's "Gaffer does not create or edit pathways" (`docs/superpowers/specs/2026-11-25-factory-pathways-design.md:28`) and
is documented (`docs/factory.md:512-514`), but it leaves pathway management browser-only.

## 5. Templates

### 5.1 What a template is

A template is a snapshot of a successful outcome's plan: objective, review limits, and per pipeline the title, prompt,
merge setting, stages (kind, part, brief, tier and tier reason), criteria, and the skills with their approved revisions.
It keeps nothing that ties it to a run: no machine, project, agent, sign-in, YOLO, trust or images
(`internal/gaffer/templates.go:15-18,128-161`; types at `internal/store/store_gaffer_templates.go:29-77`). It is private
to the account, has no MCP tool (`docs/factory.md:418-419`), and is versioned: each save is an immutable row, up to 20
versions, with the plan changed only by saving a new version
(`internal/store/store_gaffer_templates.go:14-18,112-166`).

### 5.2 Lifecycle and where it is consumed

- **Save:** offered on a finished outcome's pipeline bar when the outcome is successful and has no pathway pipeline
  (`web/src/lib/factory/PipelineDetail.svelte:177`; `web/src/lib/factory/templates.ts:91-98`). The server repeats the
  check and refuses a pathway pipeline with `template_pathway`
  (`internal/gaffer/templates.go:122-161`; test `internal/gaffer/templates_test.go:669`).
- **List and read:** the Factory header button opens a dialog (`web/src/lib/factory/Factory.svelte:572`), which lists
  templates and opens one with its versions and diff (`web/src/lib/factory/Templates.svelte:26-57,100-150`).
- **Start:** "Use" opens the New Factory outcome dialog on the template's read-only plan
  (`web/src/lib/factory/Factory.svelte:626`; `web/src/lib/factory/Templates.svelte:147`), or "Start from a template" in
  the objective step (`web/src/lib/GafferOutcome.svelte:632`). `startFromTemplate` sends the launch with the crew,
  trust, YOLO and other choices of the moment (`web/src/lib/GafferOutcome.svelte:591-610`). The server loads the
  pinned skills, refuses if the gaps shown are not the gaps skipped, and creates the outcome
  (`internal/gaffer/templates.go:515-572`).
- **Edit:** only the name and description, saved as a new version; the plan itself is read-only
  (`web/src/lib/factory/Templates.svelte:2-4,113-123`; `docs/factory.md:437-441`).

### 5.3 What value they add

Templates hold things pathways do not: a whole multi-pipeline plan (titles, briefs, merge settings), acceptance
criteria, review limits, and the exact skill revisions a plan was approved with
(`internal/gaffer/templates.go:147-159`; `internal/gaffer/templates.go:457-478`). They let a person replay a proven plan
without asking Gaffer to plan again, and they keep a historical version boundary that running outcomes rely on
(`docs/factory.md:416-470`). That is real value, and it is why "remove" is not the right verdict.

### 5.4 The problem: pathways are a one-way door out of templates

**Finding T1 (strong).** An outcome with any pipeline started on a pathway cannot be saved as a template at all. The
server refuses the whole outcome on the first pathway pipeline (`internal/gaffer/templates.go:134-138`), and the UI
hides the save action for it (`web/src/lib/factory/templates.ts:91-98`). The stated reason is that a template holds
built-in stages only, so a custom stage would be lost. That is true, but it means the newer way to shape work cannot
feed the older reuse feature. A person who builds a pathway, runs it successfully and wants to repeat
that exact outcome plan with its criteria and limits must rebuild it by hand. A mixed outcome, with one pathway
pipeline and one standard pipeline, is refused entirely, not just for the pathway pipeline.

**Finding T2 (workflow).** A template's plan cannot be edited. "Rename or change the description" is the only edit, and
it saves a new version with the same plan (`web/src/lib/factory/Templates.svelte:113-123`). To change stages, prompts
or criteria, a person must start from the template, adjust the live plan, finish the outcome, and only then save it as
a new version (`docs/factory.md:437-441`; `web/src/lib/factory/Templates.svelte:116`). That loop requires a full run to
change a saved plan.

**Finding T3 (asymmetry).** Templates have no MCP tool and are not offered to Gaffer (`docs/factory.md:418-419`). The
pathway is both suggestible and startable through the MCP tools, while a template is only reachable through the
browser dialog. A person who works through an AI client cannot reuse a template at all.

**Finding T4 (navigation).** Templates live behind a button in the Factory header that opens a dialog, while Pathways
is a link to its own route (`web/src/lib/factory/Factory.svelte:571-573`). The docs call Templates "The Templates
page" (`docs/factory.md:433`), but it is a dialog (`web/src/lib/factory/Templates.svelte:97`). The two saved-work
features therefore have different navigation, different discovery and no cross-link.

### 5.5 Verdict

**Keep Templates. Fix them, do not remove them.** They are not pointless as things stand: they are the only record of a
whole outcome plan with criteria, review limits and pinned skill revisions, and the launch path is complete and
tested. Removing them would delete capabilities pathways do not provide. But their value is undermined by three
fixable decisions: refuse every pathway outcome, allow no plan edit, and stay invisible to Gaffer and MCP. If the
owner wants one concept instead of two, the cheaper direction is to let a template be created from a pathway outcome by
copying the pipeline brief, criteria and stage labels while dropping custom-stage prompts (or mapping them to a
built-in stage plus brief), and to let a template be started on a pathway. That is a design decision, not something to
do in this audit.

## 6. Guidance: how a person could know which pathway to use

The current guidance is the stage line, the name, a first problem and, sometimes, Gaffer's suggestion. The following
are recommendations only, in the order they would help most.

**R1. Show the description in the chooser.** The field exists, the editor asks for it, Gaffer reads it and the list
shows it. Show it under the name in `PathwayChoice` before the stage line
(`web/src/lib/factory/pathways/PathwayChoice.svelte:86-90`). This is the single cheapest fix and directly addresses
Finding S1.

**R2. Let Gaffer suggest outside the first plan.** Add the suggestion to the Add pipelines choice
(`web/src/lib/factory/AddPipelines.svelte:175`), and consider an explicit "Ask Gaffer which pathway fits" action on the
Pathways page and in the choice. Today the only way to get a suggestion is to write an outcome and ask for a plan
(`internal/gaffer/outcomes.go:254-287`). A person choosing a pathway for an existing outcome gets none.

**R3. Explain the difference at the point of use.** Both the Templates and Pathways entry points should say, in one
sentence, what each holds: a pathway is a line of stages reused whenever work starts; a template is a whole finished
plan (pipelines, criteria, review limits) saved to start again. Put the sentence in both empty states and beside the
Factory header controls (`web/src/lib/factory/pathways/PathwayList.svelte:144-151`;
`web/src/lib/factory/Templates.svelte:136-138`). This directly addresses T4 and the vocabulary risk.

**R4. Make the post-start state truthful and navigable.** The pipeline detail already names the pathway
(`web/src/lib/factory/PipelineDetail.svelte:423`). Make it a link to the pathway and, when Gaffer suggested it, keep
the reason on the outcome so a person can see why it was chosen after the dialog has closed. There is nowhere to
recover the suggestion reason today: it lives only in `GafferOutcome`'s transient state
(`web/src/lib/GafferOutcome.svelte:106-115`).

**R5. Decide where Gaffer should sit in pathway authoring.** Gaffer can suggest and start a pathway but cannot create
or edit one (`docs/superpowers/specs/2026-11-25-factory-pathways-design.md:28`). A sensible next step is a
suggest-only authoring flow: Gaffer proposes a pathway (name, description, stages, tiers) from an objective or a
finished outcome, and the person reviews and saves it in the existing editor. Do not let Gaffer save one without the
person seeing it; the design's existing rule is that Gaffer never starts work on a pathway the person has not seen,
except through the start tools under the person's tool settings
(`docs/superpowers/specs/2026-11-25-factory-pathways-design.md:154-155`).

**R6. Bridge Templates and pathways.** Either allow "Save as template" for a pathway outcome by keeping its pipeline
briefs, criteria and stage labels and dropping or mapping custom-stage prompts, or make the refusal explicit in the
pipeline detail and offer "Save the plan as a pathway instead". Today the button simply disappears
(`web/src/lib/factory/templates.ts:91-98`), so the person is not told why.

**R7. If pathways are meant for Agent sessions, say so or wire it.** Today an ordinary Agent session cannot choose a
pathway (section 3.3). Either add a pathway choice to the Start Agent dialog, or state in `docs/factory.md` and in the
Pathways empty state that a pathway shapes Factory pipelines only. The current name invites the opposite assumption.

## 7. What was checked and what was not

**Checked by reading:** every route, handler, service and store function named above; the pathway and template
tests listed below; the browser components that read or write them; the shipped docs; and the 25 November pathway
design. Every claim in sections 2 to 6 is traced to a file and line.

**Tests that already exist:** `internal/gaffer/pathways_test.go` (validation, defaults, start, refusals, running work),
`internal/api/gaffer_pathways_test.go` (create/list/stale PUT, bad stages, owner and read-only isolation, skills panel,
full-size pathway, skills shelf, retried start), `internal/store/store_gaffer_pathways_test.go` (round trip, unique
names, stale save, limit, ownership), `internal/gaffer/templates_test.go` (save, launch, skill gaps, versions, diff,
pathway refusal), `internal/api/gaffer_templates_test.go` (routes, save/edit/compare/delete, skill gaps),
`internal/store/store_gaffer_templates_test.go` (versions, idempotent request, concurrency, limits, ownership),
`web/src/lib/factory/pathways.test.mjs`, `web/src/lib/factory/templates.test.mjs`, and the browser suites
`web/tests/pathways.e2e.mjs`, `web/tests/pathways-choice.e2e.mjs` and `web/tests/factory-templates.e2e.mjs`.

**Not verified.** I did not run the application, the Go suite, the web unit tests or any browser suite, so no finding
here is a runtime observation. I did not measure the list-endpoint skill-loading cost in Finding P2. I did not test a
real phone or screen reader. I did not inspect every MCP client, only the server's tool definitions and the wrapper in
`mcp/src/server.ts`. I did not verify whether the description is deliberately withheld from the chooser; the design
only says "shown in the list", so a narrower reading is possible. I did not look at portable settings export, since
the design left pathways there as an open question
(`docs/superpowers/specs/2026-11-25-factory-pathways-design.md:255`).
