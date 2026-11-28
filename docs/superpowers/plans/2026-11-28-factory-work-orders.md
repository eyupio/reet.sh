# Factory Work Orders Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An outside agent reads a public reet skill, writes a Markdown work order, and the Factory runs it after the
person confirms the plan; a work report carries the result back, by copy and paste or over MCP.

**Architecture:** One Go parser turns work order text into the `gaffer.OutcomeSession` values the plan review and
`CreateOutcome` already take, and one writer turns a stored outcome into work report text. Both are reached by two
HTTP routes (the browser) and three MCP additions. The contract is a built-in skill with topic files, served by the
existing hosted-skill routes.

**Tech Stack:** Go standard library (no YAML, no new dependency), Svelte 5 + TypeScript in `web/`, the MCP package in
`mcp/` (zod schemas), `node --test` for `*.test.mjs`, Playwright for `web/tests/*.e2e.mjs`.

**Spec:** `docs/superpowers/specs/2026-11-28-factory-work-orders-design.md`. Read it first; this plan argues from it.

## Global Constraints

- Read `AGENTS.md` and `CONTRIBUTING.md` before the first edit. Their rules bind every task.
- British spelling, plain words, no em dashes in UI copy. Commit subjects are an imperative sentence with no prefix.
- Never name another product reet was compared with, anywhere.
- UI colours come from the tokens in `web/src/app.css`; sizes in px; reet's own marks (✓ ! ✕ · …); 44 px touch
  targets on coarse pointers; check light and dark, 390 px and desktop.
- `web/src/lib/api.ts` may import only `./inflight`. New client calls go in that file with no new import.
- `npm ci` in `web/` before any Go test that transpiles the web client.
- Do not edit `CHANGELOG.md` or `handoff.md`. The pull request adds `changelog.d/factory-work-orders.md` and
  `docs/handoffs/log/WORK-ORDERS-1.md`.
- Do not skip, retry or lengthen a timeout to get a test green.
- A work order has no field for YOLO, folder trust, auto-merge, who takes decisions or review limits. An unknown
  header key or `###` section is a problem, never ignored.
- Limits are never restated as numbers in the parser: `MaxOutcomeSessions`, `MaxOutcomeObjectiveBytes`,
  `MaxOutcomePromptBytes`, `MaxStageParts`, `maxPartBriefBytes`, `terminalproto.ValidTitle`, and the criteria
  validator `planCriteria`, all in `internal/gaffer`.
- Version strings are exactly `work-order/1` and `work-report/1`. The skill is `reet-integration`.

## Review Focus

1. **A paste with Windows line endings, or wrapped in a code fence with chatter around it.** Expected: read the same
   as the clean text. Pinned in Task 1.
2. **A brief that itself contains `## ` or `### ` headings or a `---` line.** Expected: a clear problem naming the
   line and saying a brief may use `####` and deeper only, never a pipeline silently cut short. Pinned in Task 1.
3. **A very large paste.** Expected: refused before parsing with one problem, at 64 KiB. Pinned in Task 1 and Task 3.
4. **`crew` names an agent the project's machine cannot run.** Expected: the suggestion is shown, Launch readiness
   says **Cannot start**, and **Approve and start** waits, exactly as for a saved crew. Pinned in Task 6.
5. **`continues` names an outcome that is finished, on YOLO, or not the person's.** Expected: finished or unknown is
   a problem from `check` ("no outcome of yours with that id is running"); YOLO over MCP keeps the existing refusal.
   Pinned in Task 3 and Task 5.

---

### Task 1: Read a work order

**Files:**
- Create: `internal/gaffer/workorder.go`
- Test: `internal/gaffer/workorder_test.go`
- Create: `internal/gaffer/testdata/workorder_example.md` (the spec's worked example, verbatim; Task 4 embeds the
  same text in the skill and tests that they are equal)

**Interfaces:**
- Produces:

```go
const MaxWorkOrderBytes = 64 << 10

type WorkOrderMember struct{ Tool, Model, Effort string } // json: tool, model, effort
type WorkOrderProblem struct {
	Line int    `json:"line"` // 1-based; 0 when it is about the whole text
	Text string `json:"text"`
}
type WorkOrder struct {
	Objective string                     `json:"objective"`
	Project   string                     `json:"project,omitempty"`
	Pathway   string                     `json:"pathway,omitempty"`
	Continues string                     `json:"continues,omitempty"`
	Crew      map[string]WorkOrderMember `json:"crew,omitempty"` // key: light|standard|heavy
	Sessions  []OutcomeSession           `json:"sessions"`
}

// IsWorkOrder is the cheap test both routes use to decide the text is meant as one.
func IsWorkOrder(text string) bool
// ParseWorkOrder never returns a usable order together with problems: any problem means start nothing.
func ParseWorkOrder(text string) (WorkOrder, []WorkOrderProblem)
```

- [ ] **Step 1: Write the failing tests** in `workorder_test.go`:

  - `TestWorkOrderExample`: parsing `testdata/workorder_example.md` gives no problems; `Objective == "Add CSV export
    to the reports page"`, `Project == "reports-api"`, `Crew["heavy"] == {claude, opus, high}`,
    `Crew["standard"] == {codex, "", ""}`; one session titled `Export endpoint` whose `Stages` are, in order,
    `plan`, `code` (part `Streaming writer`, tier `heavy`), `code` (part `Endpoint and auth`, tier `heavy`, from the
    stage's `tiers:` line), `review` (tier `standard`), `test`, `pr`; two `Criteria`.
  - `TestWorkOrderTolerates`: the example with `\r\n` endings, and the example inside one ```` ```markdown ```` fence
    with a sentence before and after, both parse equal to the clean example. `IsWorkOrder` is true for all three and
    false for `"Add CSV export"` and for text that merely mentions `reet: work-order/1` mid-sentence.
  - `TestWorkOrderRefuses`, a table; each case asserts one problem with the given line and a substring:
    `reet: work-order/2` → line 2, "work-order/1"; unknown header key `yolo: true` → its line, "yolo"; duplicate
    header key; missing `objective`; `objective` with `continues` absent and over `MaxOutcomeObjectiveBytes`; no
    pipeline; six pipelines; two pipelines with one title; `stages: code, plan` (order); `stages: deploy`;
    `tiers: code=huge`; `tiers:` naming a stage not in `stages:`; `### Part: pr / x` (not splittable); one part
    only for a stage; `### Notes` (unknown section); a `## Something else` heading; a `## ` or `### ` heading or a
    `---` line inside a brief; eleven criteria; `pathway:` with a pipeline that has `stages:`; `pathway:` with
    `crew:`; `crew: heavy=claude, heavy=codex`; `crew: huge=claude`; `crew: heavy=nosuchagent` (the agent must be
    `terminalproto.ReviewedChatTool`); `continues:` with `crew:`; two work orders in one paste;
    text of `MaxWorkOrderBytes+1` bytes → line 0.
  - `TestWorkOrderReusesValidation`: a brief of `MaxOutcomePromptBytes+1` bytes is a problem, and one of exactly
    `MaxOutcomePromptBytes` is not (proves the constant, not a copy, decides).

- [ ] **Step 2: Run** `go test ./internal/gaffer -run WorkOrder` — expected: FAIL, undefined `ParseWorkOrder`.

- [ ] **Step 3: Implement.** A line scanner in three passes over `strings.Split` lines: find the block (fence
  handling, `---` header), read header keys with `strings.Cut(line, ":")`, then walk `## Pipeline:` sections. Build
  each stage as `OutcomeStageInput{Kind, Tier, Part, Brief}`; a stage with parts becomes one entry per part in file
  order; a part without `tier:` takes the stage's tier from `tiers:`. After building, validate the whole thing by
  calling the existing `outcomePieces(order.Sessions, "opencode", "", "", store.GafferCrew{}, false,
  map[string]bool{}, 0)` and turning its error into a problem at line 0, so no session rule is written twice. The
  tool argument is only a stand-in for the outcome's agent, which a work order does not choose: `outcomePieces`
  refuses a stage whose agent is not `terminalproto.ReviewedChatTool`, so it must be a reviewed tool name, and
  `TestWorkOrderExample` fails if it is not. Do
  the parser's own checks (the table above) first so they carry line numbers.

- [ ] **Step 4: Run** `go test ./internal/gaffer -run WorkOrder` — expected: PASS.

- [ ] **Step 5: Commit** — `Read a Factory work order from Markdown`.

### Task 2: Write a work report

**Files:**
- Create: `internal/gaffer/workreport.go`
- Test: `internal/gaffer/workreport_test.go`, `internal/gaffer/testdata/workreport_golden.md`

**Interfaces:**
- Produces: `func WorkReport(o store.GafferOutcome, handovers bool) string`

- [ ] **Step 1: Write the failing test** `TestWorkReportGolden`: build a `store.GafferOutcome` by hand with two
  pipelines (one finished with a handover on `code`; one in delivery review with criteria statuses
  `CriterionPassed` and `CriterionUnverified`) and compare `WorkReport(o, true)` with the golden file. Second
  assertion: `WorkReport(o, false)` contains no `### Handover`. Third: the header's first line after `---` is
  `reet: work-report/1` and it has `outcome: <o.ID>`. Fourth: the text ends with the fixed paragraph containing
  `continues: <o.ID>`.
- [ ] **Step 2: Run** `go test ./internal/gaffer -run WorkReport` — FAIL.
- [ ] **Step 3: Implement** with a `strings.Builder`. State words are the stored state strings as
  `mcpOutcomeFull` in `internal/api/gaffer_factory_mcp.go` already prints them; handovers clipped with the existing
  `clipTo(text, 600)`; criteria lines `- [passed|needs_work|unverified] text`. Read how `mcpOutcomeFull` walks an
  outcome and follow it; do not move that function.
- [ ] **Step 4: Run** — PASS. Write the golden file from the reviewed output.
- [ ] **Step 5: Commit** — `Write a work report for a Factory outcome`.

### Task 3: Two routes for the browser

**Files:**
- Modify: `internal/api/gaffer_outcomes.go` (register beside the outcome routes, same `access` wrapper)
- Create: `internal/api/gaffer_workorder.go`
- Test: `internal/api/gaffer_workorder_test.go` (follow the set-up of `gaffer_outcomes_test.go`)

**Interfaces:**
- Consumes: `gaffer.ParseWorkOrder`, `gaffer.WorkReport`, `s.gafferProviders.Outcome(uid, id)`.
- Produces:
  - `POST /api/v2/gaffer/work-orders/check`, body `{"text": "..."}` →
    `200 {"order": WorkOrder|null, "problems": [...], "projectId": "", "pathwayId": "", "notes": []}`.
    `projectId` is the id of the person's project whose name equals `order.project` in any letter case, else empty
    with a note; `pathwayId` likewise for a switched-on pathway, and a named pathway that is not on is a *problem*.
    `continues` that is not a running, non-archived outcome of the caller is a problem at the header line 0.
    Starts nothing; makes no model request. Body capped with `http.MaxBytesReader` at `MaxWorkOrderBytes + 1024`.
  - `GET /api/v2/gaffer/outcomes/{id}/work-report` → `200 text/plain`, `WorkReport(o, true)`.

- [ ] **Step 1: Failing tests**: clean example → `problems` empty and `order.sessions` length 1; a project named
  `Reports-API` exists → `projectId` set; unknown project → `projectId` empty and one note; `continues` of another
  account's outcome → problem, and the response does not differ from an id that does not exist; a read-only account
  → refused as the outcome create route refuses it; oversized body → 413 or one problem; report route for another
  account's outcome → 404.
- [ ] **Step 2: Run** `go test ./internal/api -run WorkOrder` — FAIL.
- [ ] **Step 3: Implement** both handlers.
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** — `Check a work order and give a work report over HTTP`.

### Task 4: The integration skill and its topics

**Files:**
- Create: `internal/api/builtin_skills/reet-integration/SKILL.md` and, beside it, `work-order.md`,
  `work-report.md`, `tiers-and-crew.md`, `pathways.md`, `continuing.md`, `mcp.md`
- Modify: `internal/api/builtin_skills.go`
- Test: `internal/api/builtin_skills_test.go` (create if absent)

**Interfaces:**
- Produces: `builtinSkill` fills `HostedSkill.Files` with every file in the skill's embedded directory other than
  `SKILL.md`, keyed by file name. Existing skills have none, so they are unchanged.

- [ ] **Step 1: Failing tests**:
  - `TestIntegrationSkillServesTopics`: `GET /eyupio/reet-integration/.well-known/agent-skills/index.json` lists
    `SKILL.md` and the six topics; each topic file is served; `GET /eyupio/reet-babysit/…/index.json` still lists
    `SKILL.md` only.
  - `TestIntegrationSkillExampleParses`: the first fenced block of `work-order.md` equals
    `internal/gaffer/testdata/workorder_example.md` byte for byte and `gaffer.ParseWorkOrder` gives no problems.
  - `TestIntegrationSkillLimits`: `work-order.md` contains each enforced figure, formatted from the constants
    (export what is needed from `gaffer` in an `export_test`-free way: add `func WorkOrderLimits() map[string]int`
    to `workorder.go` returning pipelines, objective bytes, brief bytes, parts, part brief bytes, criteria,
    criterion bytes, title runes).
  - `TestIntegrationSkillLinksResolve`: every `](name.md)` link in `SKILL.md` is an embedded file, and every
    embedded topic is linked from `SKILL.md`.
  - `TestIntegrationSkillListsBuiltins`: `SKILL.md` names every other directory under `builtin_skills`.
- [ ] **Step 2: Run** `go test ./internal/api -run IntegrationSkill` — FAIL.
- [ ] **Step 3: Write the skill.** `SKILL.md` front matter `name: reet-integration` and a one-line `description`
  saying when an agent should use it (handing work to a reet Factory). Body, in this order and under 120 lines:
  what the Factory does with a work order; the two routes ("print the work order in one fenced block for the person
  to paste into New Factory outcome", or "if reet's MCP tools are connected, call `reet_check_work_order`, then
  `reet_start_outcome` with `work_order`"); the rule that the person confirms and may refine before anything
  starts, so the agent should not ask for YOLO, merges or approvals; the smallest valid work order; the table **If
  you need more** (topic, read it when); how to continue from a work report; the other reet skills. Each topic file
  holds the spec's matching section, written for an agent, with the exact refusals it can expect. `mcp.md` lists
  the Factory tools by name and points at the tool descriptions rather than repeating schemas.
- [ ] **Step 4: Implement** the `Files` loading with `builtinSkills.ReadDir`.
- [ ] **Step 5: Run** `go test ./internal/api -run 'IntegrationSkill|Hosted|Builtin'` and
  `python3 -m pytest hooks/docs_test.py` (or the command `CONTRIBUTING.md` gives for it) — PASS.
- [ ] **Step 6: Commit** — `Publish the reet-integration skill with its topics`.

### Task 5: MCP

**Files:**
- Modify: `internal/api/gaffer_factory_mcp.go`, `internal/api/gaffer_tools.go` (tool definitions and the read set
  `mcpFactoryReads`), `mcp/src/server.ts` (zod schemas), `docs/mcp.md`
- Test: `internal/api/gaffer_factory_mcp_test.go`, `internal/api/gaffer_tools_test.go`, the tool-list tests under
  `mcp/test`; grep `reet_list_pathways` across the repository and add the new names everywhere it is listed

**Interfaces:**
- Consumes: `gaffer.ParseWorkOrder`, `gaffer.WorkReport`.
- Produces:
  - `reet_check_work_order {work_order}` — a read, open to the connections `mcpFactoryReads` admits. Answers
    `{ok, problems, plan}` where `plan` is the pipelines as `reet_get_factory_outcome` would name their stages and
    tiers, plus `note` when `crew` was given: "The crew suggestion applies only when a person pastes the work
    order. Over MCP the saved crew decides."
  - `reet_get_work_report {outcome_id}` — a read. `WorkReport(o, management)`: handovers only for a management
    connection, the rule `reet_get_factory_outcome` follows.
  - `reet_start_outcome` and `reet_add_pipelines` take `work_order` (string). With it: `pipelines`, `objective`
    and `pathway` arguments must be absent, else "Give a work order or these arguments, not both. Nothing was
    started."; the header's `project`, when present, must equal the name of `project_id`'s project in any letter
    case; `continues` to `reet_start_outcome` is refused naming `reet_add_pipelines`; `continues` to
    `reet_add_pipelines` must equal the outcome id; any parse problem refuses with the problems, nothing started.
    Criteria from the work order are kept on the pipelines. Everything after that is the existing code path:
    `confirm`, `request_id`, the YOLO refusal, the machine check, `s.mcpUsePathway`.

- [ ] **Step 1: Failing tests** for each refusal above, for a clean start (the outcome has the pipeline, its stage
  tiers, its criteria, and is not YOLO and not auto-merge), for the `crew` note, for a read-only connection calling
  `reet_get_work_report` (no `### Handover`), and for a YOLO outcome with `reet_add_pipelines` + `work_order` (the
  existing refusal, unchanged).
- [ ] **Step 2: Run** `go test ./internal/api -run 'Factory|GafferTools'` — FAIL.
- [ ] **Step 3: Implement.** In the `reet_start_outcome` and `reet_add_pipelines` cases, branch once at the top:
  when `work_order` is set, parse it and substitute its sessions for `mcpPipelines(...)` and its objective and
  pathway for the arguments; leave the rest of each case untouched.
- [ ] **Step 4: Add the zod schemas** in `mcp/src/server.ts` beside the existing Factory tools, and the three rows
  and paragraphs in `docs/mcp.md` under "Read and steer the Factory".
- [ ] **Step 5: Run** the Go tests and `cd mcp && npm test` — PASS.
- [ ] **Step 6: Commit** — `Take work orders and give work reports over MCP`.

### Task 6: Paste a work order into New Factory outcome

**Files:**
- Modify: `web/src/lib/api.ts` (add `checkWorkOrder(text, signal)` and `getWorkReport(id, signal)`; no new import),
  `web/src/lib/gafferOutcome.ts`, `web/src/lib/GafferOutcome.svelte`
- Test: `web/src/lib/gafferOutcome.workorder.test.mjs` (pure functions), `web/tests/work-order.e2e.mjs`

**Interfaces:**
- Consumes: Task 3's `check` response.
- Produces, in `gafferOutcome.ts`:

```ts
export const isWorkOrder = (text: string): boolean            // same rule as gaffer.IsWorkOrder
export function workOrderSessions(order: WorkOrder): PlanSession[]   // stages, ratings (tiers), parts, criteria
export function workOrderCrew(crew: Crew, suggested: WorkOrder['crew'], tools: string[]): { crew: Crew; from: Record<CrewTier, boolean> }
export function outsideAgentPrompt(origin: string, ctx: { project?: string; pathways: string[]; savedTiers: CrewTier[] }): string
```

- [ ] **Step 1: Failing unit tests**: `isWorkOrder` agrees with the Go cases from Task 1 (copy the five inputs);
  `workOrderSessions` on the example's `order` gives one `PlanSession` whose `stagePlan(...)` output has `code`
  twice with parts and `review` on `standard`; `workOrderCrew` replaces only the tiers suggested, keeps backups of
  untouched tiers, drops a suggested tier's saved backups, and marks `from.heavy === true`; a suggested agent not
  in `tools` is still applied (readiness, not this function, reports it); `outsideAgentPrompt` starts with the
  exact sentence `hostedUsePrompt` produces for `<origin>/eyupio/reet-integration` and `reet-integration`, then a
  `Context:` block listing the project, pathway names and tiers with a saved crew, and nothing else about the
  account.
- [ ] **Step 2: Run** `cd web && node --test src/lib/gafferOutcome.workorder.test.mjs` — FAIL.
- [ ] **Step 3: Implement the four functions.**
- [ ] **Step 4: Wire the dialog** (`step === 'objective'`): when `isWorkOrder(objective)`, show one line **This is a
  work order.** and make the primary button **Read the work order** in place of **Ask Gaffer for a plan** (and
  hide **Write the plan myself**). It calls `checkWorkOrder`; problems render as a list, each `Line n: text`, in the
  dialog's existing error style, with focus moved to the list; a clean answer sets `objective = order.objective`,
  `projectId` when given, the pathway when given, `sessions = workOrderSessions(order)`, applies `workOrderCrew`,
  and moves to `step = 'plan'`. Beside a suggested tier in the crew editor and on each stage line the source reads
  **Suggested by the work order** (a third value beside **Saved crew** and **Your choice**), and the crew editor
  gains **Use my saved crew** which restores it. Notes from `check` show above the pipelines. Add the secondary
  control **Copy prompt for an outside agent** on the objective step; it copies `outsideAgentPrompt(...)` and
  confirms with the page's existing copied-feedback pattern.
- [ ] **Step 5: Browser suite** `web/tests/work-order.e2e.mjs`, following an existing Factory suite's mocks: paste
  the example → **Read the work order** → the plan step shows `Export endpoint`, its stages, both parts and both
  criteria, heavy reads **Suggested by the work order**; a suggested agent the mocked machine lacks shows **Cannot
  start** and **Approve and start** is disabled with its reason; a work order with `yolo: true` shows
  `Line 4: …` and stays on the first step; **Approve and start** posts stages whose `tool` for `code` is the
  suggested agent and nothing that sets `yolo`, `trust` or `autoMerge` to true. Run at 390 px and desktop, light
  and dark, as `AGENTS.md` describes (`npm run build`, `vite preview`, stop it by PID).
- [ ] **Step 6: Run** `cd web && npm test` (or the script `CONTRIBUTING.md` names) and the suite — PASS.
- [ ] **Step 7: Commit** — `Open a pasted work order in the plan review`.

### Task 7: Continue an outcome, and copy its report

**Files:**
- Modify: `web/src/lib/GafferOutcome.svelte`, `web/src/lib/factory/Factory.svelte`,
  `web/src/lib/factory/PipelineDetail.svelte` (the outcome's **⋯** menu), `web/src/lib/api.ts` only if
  `addGafferPipelines` does not already send stages, tiers and parts
- Test: extend `web/tests/work-order.e2e.mjs`

**Interfaces:**
- Consumes: `checkWorkOrder` (its `order.continues`), `getWorkReport`, the existing `addGafferPipelines`.

- [ ] **Step 1: Failing browser tests**: the outcome menu has **Copy for your agent**, and choosing it puts text
  starting `---\nreet: work-report/1` on the clipboard; pasting a work order with `continues: <that id>` into New
  Factory outcome and choosing **Read the work order** opens the plan step headed **Add to “<objective>”**, with
  the project, crew and settings controls absent, and **Approve and add** posts to the add-pipelines route for
  that outcome with the stages and criteria.
- [ ] **Step 2: Implement.** Give `GafferOutcome.svelte` an `adding` state holding the outcome; in that state the
  plan step hides what an existing outcome already fixed and submits through `addGafferPipelines`. Check what the
  server's `AddPipelines` takes for stages (`gaffer.AddPipelinesInput.Sessions` is `[]OutcomeSession`, so stages,
  tiers, parts and criteria are already accepted) and send the same `stagePlan(...)` output the start sends.
  `AddPipelines.svelte` stays as it is.
- [ ] **Step 3: Run** the suite — PASS.
- [ ] **Step 4: Commit** — `Continue an outcome from a work order and copy its work report`.

### Task 8: Words, and the whole thing once

**Files:**
- Modify: `docs/factory.md` (a section **Work orders from another agent** after **Outcomes and pipelines**),
  `docs/mcp.md` if Task 5 left anything unsaid, `mkdocs.yml` only if a page is added
- Create: `changelog.d/factory-work-orders.md`, `docs/handoffs/log/WORK-ORDERS-1.md`

- [ ] **Step 1:** Write the docs section: what a work order is, how to get an agent to write one (**Copy prompt for
  an outside agent**), what reet does with a paste, what it will never take from one, **Copy for your agent**, and
  continuing.
- [ ] **Step 2:** The changelog fragment: one bullet with a bold lead, for the person running reet.
- [ ] **Step 3:** The handoff entry: what was built, why, what was checked, what was not.
- [ ] **Step 4:** `go test ./...`, `cd web && npm test`, `cd mcp && npm test`, the docs check
  (`python3 scripts/check-docs.py`), and `web/tests/work-order.e2e.mjs` — all PASS. Paste the commands and their
  last lines into the handoff entry.
- [ ] **Step 5:** A real run: start the controller locally as `CONTRIBUTING.md` describes, fetch
  `/eyupio/reet-integration` and one topic file with `curl`, paste the example into the dialog, and confirm the
  plan step by eye in light and dark at 390 px and desktop.
- [ ] **Step 6: Commit** — `Document Factory work orders`. `git fetch` and merge `origin/main` before pushing;
  open the pull request as a draft using the repository's template.
