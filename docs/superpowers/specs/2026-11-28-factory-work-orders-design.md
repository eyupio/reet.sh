# Factory work orders: a public contract for outside agents

Date: 2026-11-28. Status: design, awaiting approval.

## Intent

An agent that is not part of reet (any AI client, anywhere) should be able to hand the Factory a piece of work in a
precise, versioned format, and stay in step with it until the outcome is what was specified. The agent learns the
format from a public reet skill, fetched with the usual one-line prompt:

```
Run `npx skills use "https://<controller>/eyupio/reet-integration" --skill "reet-integration"` and follow the
generated skill instructions now. …
```

Why: today only a client already connected over MCP can start Factory work, and the contract lives in MCP tool
schemas. There is nothing a person can paste, nothing an unconnected agent can write, and no way for the work to
report back in a form the agent is told how to read.

## Decided with the owner

1. **The contract is a public skill**, used the way every hosted skill is used (`npx skills use … --skill …`).
2. **Two routes, one format.** Paste is the primary route: the outside agent prints a work order, the person pastes it
   into reet, reet recognises it and shows the pipelines and stages visually, and the person confirms and starts. A
   connected agent submits the same text over MCP.
3. **Two-way by copy-back plus MCP.** An outcome can produce a *work report* in the contract's format for the person to
   copy back to their agent; the agent answers with a follow-up work order. Over MCP the same report and follow-up are
   tool calls. reet never calls out to an address an agent supplied.
4. **The syntax is Markdown with a header**, not JSON and not a new language.

## Goals

- A work order maps exactly onto what the plan review already edits: `gaffer.OutcomePlan` / `gaffer.OutcomeSession`
  (title, brief, stages, tiers, parts, acceptance criteria, pathway).
- One parser and validator, in Go, behind both routes. Its refusals are exact and fixable: line number, what was
  found, what is allowed.
- A pasted work order starts nothing. It opens the existing plan review, filled in.
- The contract an agent reads is the contract the controller it is talking to enforces.

## Non-goals

- Webhooks or callbacks from reet to an outside agent.
- Steering a running pipeline by paste (tell the agent, send a stage back). The person has buttons for that, and a
  connected agent has the MCP tools. Paste continuation adds pipelines only.
- Letting an MCP client name an agent or model for a stage. The existing guard stays.
- Saving a work order as a template or pathway. The person can do that from the plan review as now.

## Anti-goals

- A work order must never switch on YOLO, folder trust, a merge that happens by itself, who takes decisions, or the
  review limits. It has no fields for them, and a header key it does not know is a refusal, not ignored.
- A nearly-right work order must not start as something subtly different. Strict parsing: unknown keys, unknown
  sections, unknown stage names and over-long text are problems, never silently dropped or clipped.
- No product reet was compared with is named anywhere in this work.

## Approaches considered

- **A marked JSON block.** Strictest, one standard-library parser, a JSON Schema to ship. Less pleasant to read raw.
  Recommended by the assistant; the owner preferred readability.
- **Markdown with a header (chosen).** Briefs read as prose under headings. Costs a purpose-written parser, which is
  kept strict (below) so that "readable" does not become "loose".
- **A purpose-made mini language.** Everything about it would be new. Dropped.

## The work order, version 1

Plain text. The header is flat `key: value` lines between `---` lines. It is **not YAML**: no nesting, no quoting
rules, one value per line, parsed with string cuts as `builtinSkill` parses front matter. The body is Markdown.

```markdown
---
reet: work-order/1
objective: Add CSV export to the reports page
project: reports-api
crew: heavy=claude/opus/high, standard=codex
---

## Pipeline: Export endpoint

stages: plan, code, review, test, pr
tiers: code=heavy, review=standard

Add GET /reports/{id}/export.csv … (the brief: self-contained prose, any Markdown,
up to the first ### heading)

### Part: code / Streaming writer
tier: heavy

Write rows as they are read, never the whole file in memory.

### Part: code / Endpoint and auth

Wire the handler and reuse the report's read permission.

### Criteria

- An export of 10,000 rows finishes in under 5 seconds
- A reader of another team's report is refused
```

Header keys:

| Key | Meaning |
| --- | --- |
| `reet` | Required, first. `work-order/1`. Another version is refused with the versions this controller reads. |
| `objective` | Required. The outcome, under the existing 4,000-byte limit. |
| `project` | Optional hint: a project's name. Matched exactly, any letter case; no match is a note, and the person chooses. Never an id, never a path. |
| `crew` | Optional suggestion of who does the work, per tier: `<tier>=<agent>[/<model>[/<effort>]]`, comma-separated, each tier at most once. The agent is one of reet's agent names (`claude`, `codex`, `opencode`, `cline`, `antigravity`); another is refused. See Trust. |
| `pathway` | Optional: a saved pathway's exact name. Then no pipeline may give `stages`, `tiers` or parts (the rule `mcpPipelines` already enforces), and `crew` is refused. |
| `continues` | Optional: an outcome's id, taken from a work report. The pipelines are then *added* to that outcome; `objective` is not required and is ignored. |

Each `## Pipeline: <title>` section: optional setting lines (`stages:`, `tiers:`) directly under the
heading, then the brief, then optional `### Part: <stage> / <title>` sections (an optional `tier:` line, then the
part's brief) and one optional `### Criteria` list. Limits are the ones the code enforces today, read from the same
constants and validators and never restated in the parser: five pipelines; pipeline and part titles as
`terminalproto.ValidTitle` allows (80 runes); brief `MaxOutcomePromptBytes` (8,000 bytes); the eight built-in stage
names in their fixed order; two to four parts per split stage, part brief `maxPartBriefBytes` (1,200 bytes); ten
criteria of 240 bytes. The skill publishes these enforced figures, not the smaller ones in the provider tool's
schema. A test fails if the skill's figures and the constants differ.

Recognition: reet looks for a header whose first key is `reet: work-order/`. Text before the opening `---` and after
the last section is ignored only if the whole work order sits in one fenced code block (agents wrap output in
fences); otherwise the work order must be the whole paste. One work order per paste.

## The work report, version 1

What reet gives back, in the same style, built from what `reet_get_factory_outcome` already assembles:

```markdown
---
reet: work-report/1
outcome: <id>
objective: …
state: running
---

## Pipeline: Export endpoint
state: delivery review
stage in hand: test

- plan: done
- code: done (heavy)
- review: done
- test: in hand

### Handover: code
(the stage's handover note, clipped to 600 characters as over MCP)

### Criteria
- [passed] An export of 10,000 rows finishes in under 5 seconds
- [unverified] A reader of another team's report is refused

### Waiting for the person
Delivery review.
```

The builder decides the exact lines for states reet already has; the rule is that every state word is one the
Factory page and `reet_get_factory_outcome` already use. The report ends with one fixed paragraph telling the agent
that it can answer with a work order carrying `continues: <id>` to add work. A criterion's status is one of the
code's own: `passed`, `needs_work`, `unverified`. Handover text is session-derived, so
over MCP it follows the existing rule (management connections only); the person's own **Copy** always includes it.

## The integration skill: an entry point with topics

Added by the owner after the first approval: the skill is a foundation to extend, not one fixed page. An outside
agent reads a short entry page, and follows links to exactly the detail it needs. Its conversation with these pages
ends when it prints (or submits) a work order; from there reet takes over, after the person has confirmed and
refined the plan.

- **Entry page** `SKILL.md` (name `reet-integration`, matching the other built-ins' `reet-*` names; assistant's
  call, the owner's example said `factory-contract`): what reet's Factory is in five lines, the two routes, the
  smallest valid work order, and a table **If you need more** of topic files, each with one line on when to read it.
- **Topics** are supporting files of the same skill, flat names, served by the existing
  `/{username}/{slug}/.well-known/agent-skills/{name}/{file...}` route and fetched by `npx skills use` into its
  supporting-files directory. Version 1 ships: `work-order.md` (the full format and limits), `work-report.md`,
  `tiers-and-crew.md`, `pathways.md`, `continuing.md`, `mcp.md` (the tools, for a connected agent). Adding a
  capability later is adding a file and a table row. `builtinSkill` loads every other file in the skill's embedded
  directory into `HostedSkill.Files`; nothing else about serving changes.
- **Other reet skills.** The entry page ends with the built-in skills an agent may ask for by name in a brief (the
  `reet-*` skills), each with its own `npx skills use` line. The list is generated from the embedded directory in a
  test, so it cannot go stale.
- **The slightly dynamic part.** Hosted pages are public and the same for everyone, so nothing about an account is
  in them. What is particular to the person travels in the prompt they copy: **Copy prompt for an outside agent** in
  the New Factory outcome dialog gives the `npx skills use` line followed by a short **Context** block built in the
  browser from what the dialog already holds: the chosen project's name, the names of the pathways that are on, and
  which tiers have a saved crew. A connected agent reads the same through `reet_list_factory_projects` and
  `reet_list_pathways`. No new public endpoint returns account data.

## Trust

- Everything in a work order is untrusted data written by an agent the controller knows nothing about. It is shown
  to the person; nothing in it is obeyed beyond the fields above.
- **Paste:** the plan review already chooses agent, model and effort per tier, not per stage, so `crew` suggests
  per tier (decided with the owner). It pre-fills the dialog's crew choice for this outcome only, with the source
  shown as **Suggested by the work order** beside the existing **Saved crew** and **Your choice**, and a way back to
  the saved crew. The saved crew itself is never changed. Launch readiness already says what the machine cannot run.
- **MCP:** `crew` is not applied. The answer says so in its `note`. Tiers and the saved crew decide, as now.
- Acceptance criteria may now arrive from a work order on either route. They can only add a stop (a delivery
  review), never remove one.
- An outcome started from a work order over MCP keeps every existing MCP rule: `confirm: true`, ordinary approvals,
  never YOLO, every pipeline stops at Ready to merge.

## Parts to build

1. **Parser and validator**, `internal/gaffer` (new file). Text in; out comes an `OutcomePlan`, the header's hints
   (`project`, `pathway`, `continues`), the per-tier crew suggestions, and a list of problems with line numbers. It
   reuses the existing validation of sessions, stages, parts and criteria; it adds only reading the text. A table
   test with the skill's own worked example as the first case, so the published example can never drift from the
   parser.
2. **Report writer**, beside it: an outcome in, work report text out. Tested by parsing nothing; a golden text.
3. **HTTP**: `POST /api/v2/gaffer/work-orders/check` (text in; plan, hints, suggestions, problems out; starts
   nothing) and `GET /api/v2/gaffer/outcomes/{id}/work-report`. Registered beside the outcome routes in
   `internal/api/gaffer_outcomes.go`, same access wrapper.
4. **The skill**, `internal/api/builtin_skills/reet-integration/`. See "The integration skill" below.
5. **Web, New Factory outcome** (`web/src/lib/GafferOutcome.svelte`): when the outcome text is a work order, the
   dialog says **This is a work order** and **Ask Gaffer for a plan** becomes **Read the work order**. It calls
   `check`; problems are listed with their line numbers; a clean one opens the existing plan review with pipelines,
   stages, tiers, parts, criteria, crew, project and pathway filled in. With `continues`, the same plan review opens
   in an adding mode on that outcome and submits through the add-pipelines request. This is new plumbing:
   `AddPipelines.svelte` takes titles, prompts and stage names only and is mounted from `Factory.svelte`, so it is
   not reused as it stands. `crew` with `continues` is refused: the outcome's crew was chosen when it started. No model request is made. The dialog also offers **Copy prompt for an
   outside agent**, the `npx skills use` line for this controller.
6. **Web, outcome detail**: **Copy for your agent** in the outcome's menu copies the work report.
7. **MCP** (`internal/api/gaffer_factory_mcp.go`, `gaffer_tools.go`): `reet_check_work_order` (read, any writer
   connection; the dry run); `reet_get_work_report` (read); and `work_order` accepted by `reet_start_outcome` and
   `reet_add_pipelines`. The reads are open to the connections the existing Factory reads are. With `work_order`:
   `pipelines`, `objective` and `pathway` arguments must be absent (given too is a refusal, nothing started);
   `project_id`, `agent`, `model`, `confirm` and `request_id` stay arguments as now, and the header's `project`
   must name that project or be absent; `continues` sent to `reet_start_outcome` is refused with the tool to use,
   and sent to `reet_add_pipelines` must equal the outcome id given. The tool descriptions name the skill.
8. **Words**: a section in `docs/factory.md` and `docs/mcp.md`, a changelog fragment, a handoff entry, and the
   skill's page generated by `hooks/docs.py` as for the other built-ins.

## Left to the builder

- The parser's internal shape and exact wording of each problem sentence (house style: plain, British, says what to
  change).
- Exact report lines per state.
- Where in the dialog the two new controls sit; check light and dark, 390 px and desktop; colours from tokens.
- Whether the MCP package under `mcp/` lists tools and needs the three additions.

## Risks

- A hand-written Markdown parser is where "precise" can leak. Mitigation: strictness rules above, and the skill's
  example is a test fixture.
- Version drift between a public copy of the skill and a self-hosted controller. Mitigation: the skill is served by
  the controller itself and the header carries the version; a mismatch is a clear refusal.
