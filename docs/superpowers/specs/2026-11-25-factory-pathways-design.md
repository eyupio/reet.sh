# Factory pathways: design your own line of stages

Date: 25 November 2026. Status: awaiting the owner's approval.

## Why

Today every Factory pipeline runs some of eight fixed stages (plan, design, code, review, test, pr, babysit, merge) in
one fixed order. Which model does a stage comes from the Crew, skills are picked stage by stage on a running pipeline,
and a stage has no prompt of the person's own. The owner wants to design the route work takes: "a pathway of stages,
models and skills to use on each", built and kept up on a visual canvas by dragging stages, models, skills and prompts,
and used by both the Factory and Gaffer.

## Goals

1. A person can build a pathway: an ordered line of stages, each with a model, skills and a prompt.
2. Stages are either the eight built-in ones, which keep their behaviour, or the person's own, with a name and prompt.
3. Pathways are built and edited on a canvas by drag and drop, and can be enabled, disabled, duplicated, renamed and
   deleted.
4. An enabled pathway can be chosen when work starts in the Factory, by the person or through Gaffer's tools, and
   Gaffer may suggest one that fits the goal.
5. The canvas is polished: light and dark, 390 px to desktop, touch and keyboard, the app's own tokens and marks.

## Not goals

- No branches, conditions or stages running side by side. A pathway is one line (the owner's choice).
- No splitting a pathway stage into parts. Two stages of the person's own do the same job. Standard pipelines keep
  parts as they are.
- Gaffer does not create or edit pathways in this piece of work. It reads them, suggests one and starts work on one.
- No sharing pathways between accounts, and no import or export.
- No change to Factory templates. "Save as template" is not offered for an outcome started from a pathway.
- No change to how standard pipelines (no pathway chosen) behave.

## What would make it a failure

- Editing or deleting a pathway changes work that is already running.
- A built-in stage loses its behaviour because of where it sits: review must still send work back, merge must still
  wait for approval and the delivery review.
- Anything on the canvas that can only be done by dragging. Every drag has a button or keyboard way.
- Dragging that does not work on a touch tablet.
- A disabled pathway that Gaffer or the new-outcome dialog still offers.
- A literal colour, type under 14 px or a touch target under 44 px.

## Approaches considered

1. **A new "custom" stage kind inside the existing Factory (chosen).** Pathways are saved in a table of their own.
   Starting work copies the pathway's stages into the outcome, and the existing supervisor runs them. One new kind
   carries a name and prompt; pipelines from a pathway run in the pathway's order. Built-in kinds keep their code.
   Least new machinery, and running work is safe from later edits because it holds its own copy.
2. **Grow Factory templates into pathways.** Rejected: templates are fixed, versioned copies of a finished outcome
   with pinned skill revisions and deliberately no model. Making them editable and switchable works against that and
   puts the existing template features at risk.
3. **A separate pathway runner beside the Factory.** Rejected: it would duplicate the supervisor, hand-over, usage
   and reporting, and Gaffer would have two systems to learn.

The owner chose 1.

## What the owner decided

- Stages are fully custom: the person's own stages, alongside the eight built-in ones as ready-made blocks.
- Any order, mixed freely, in one line.
- The person picks a pathway; Gaffer may suggest one. With none chosen, today's stages are used. Disabled pathways
  are hidden from both.
- Built as approach 1.

Everything below this line is the designer's call unless marked otherwise, and open to correction.

## The pathway

A pathway belongs to one account and has:

- a name (1 to 60 plain characters, unique within the account, compared without case),
- a description (up to 300 bytes), shown in the list and given to Gaffer so it can judge fit,
- enabled or disabled (new pathways start enabled),
- 1 to 12 stages, in order.

A stage has:

- **Kind**: one of the eight built-in kinds, or `custom`.
- **Name**: for a custom stage only, 1 to 40 plain characters, unique within the pathway and not the name of a
  built-in kind. Shown wherever a kind's label is shown today.
- **Prompt**: up to 4,000 bytes. For a custom stage it is required and is the stage's work. For a built-in stage it
  is optional extra instructions, added after the built-in wording.
- **Who does it**: either a Crew tier (light, standard or heavy; the default, standard for a custom stage) or a named
  agent, model and effort, exactly as a stage can be given today.
- **Skills**: up to 2 and 2,000 bytes of guidance together, the existing limit for a stage. Saved by skill id; the
  text is read when work starts. The palette lists what a stage can load today: built-in skills and Active library
  skills. A pair over the size limit is refused when dropped and again at save.

Rules on order, each with a plain message in the editor:

- A built-in kind appears at most once.
- Merge, if present, is last.
- Babysit and merge each need pr somewhere before them.

Otherwise any order: review may come before code, a custom "security audit" may sit between test and pr.

A stage is identified, wherever the Factory names one today (send-back, move-on, decisions, the delivery review's
stage list, the MCP tools), by the same `stage` text field: the kind for a built-in stage, the name for a custom one.
The two rules above (one of each built-in kind; custom names unique and never a kind's name) are what make that
unambiguous, so no API or stored shape changes to an index.

## Storage

One table, `gaffer_pathways`, following the pattern of `gaffer_templates` without versions: `id`, `user_id`, `name`,
`enabled`, `created_at`, `updated_at`, `data` (JSON: description and stages). At most 50 pathways an account.

A saved stage (`store.GafferOutcomeStage`) and its input (`OutcomeStageInput`) gain `Name` and `Prompt`. Each
pipeline of an outcome records the pathway it was started from (id and name at that moment) and holds its own copy
of the stages, so later edits and deletion do not reach it. No copy of the whole pathway is kept on the outcome.

## Running a pathway

- `outcomePieces` (internal/gaffer/outcome_runs.go) accepts `custom` and, for a pipeline from a pathway, checks the
  pathway's order rules instead of the fixed order. Standard pipelines are checked as today.
- `stagePrompt`, `StageGoal` and `partTitle` use the stage's name for a custom stage. The saved goal Gaffer judges a
  custom stage against carries its prompt, clipped to 1,200 bytes, where a built-in stage's carries the built-in work. A custom stage's work is its
  prompt; its deliverable is "what this stage's instructions ask for". A built-in stage with a prompt has it added
  after the built-in work, under "Extra instructions for this stage". The existing boundary text ("do not start the
  later stages", "Do not merge.") stays last and names later stages by their names.
- Send-back and move-on (`outcome_manage.go`) already find a stage by scanning the pipeline for its kind; they compare
  a custom stage by name instead. The send-back rebuild copies stage fields one by one and must carry `Name` and
  `Prompt`. The delivery review's stage list merges neighbours of equal kind; custom stages are never merged.
- Merge gates, delivery review, hand-over, usage, failover and recovery are untouched: they already key on
  `Kind == "merge"` or on position.
- Skills are loaded into `SkillGuidance` when work starts, at the skill's current revision. If a skill has gone, is
  no longer Active or the pair has grown past the size limit, the start is refused with a message naming the pathway, stage and skill, and the pathway list marks that pathway `!`.
- A stage on a Crew tier takes that tier's agent at start, with the tier's backups, as today. A stage on a named
  model carries agent, model, effort and (for an agent that takes one) billing source, and no backups: the inspector
  says a named model has no stand-in when its plan runs low, and a tier does.

## API

`/api/v2/gaffer/pathways`, behind the same feature access as the other Gaffer routes:

- `GET` list, `POST` create, `GET /{id}`, `PUT /{id}` (whole pathway, including enabled), `DELETE /{id}`.
- `PUT` carries the `updatedAt` it was read at and is refused if the pathway changed since, so two tabs cannot
  silently overwrite each other.

Starting an outcome and adding pipelines each accept an optional `pathway` id. With it, the pipelines' stages come
from the pathway as it is saved at that moment, and any `stages` sent are refused. A disabled or missing pathway is
refused with a plain message. Pipelines added later may name any enabled pathway or none; the Add pipelines dialog
starts on the pathway the outcome's first pipeline used, if it is still enabled.

## Gaffer and the MCP tools

- New read tool `reet_list_pathways`: enabled pathways with name, description and stage names.
- `reet_start_outcome` and `reet_add_pipelines` gain an optional `pathway` (id or exact name).
- Because Gaffer's own tools reuse the MCP definitions in-process, it gets all three. The read tool is grouped with
  the other Factory read tools; the two start tools keep their existing on/ask/allow setting.
- Planning (`propose_sessions`): the enabled pathways (name, description, stage names; none of the prompts) are listed
  in the planning prompt, and the tool gains an optional `pathway` and one-line `pathwayWhy`: one pathway for the
  whole plan. Gaffer still proposes stages, tiers and parts for every session as today, so switching the select back
  to "Standard stages" has them ready. The new-outcome dialog
  shows the suggestion as "Gaffer suggests: <name>" with its reason, already selected, and the person can change it.
  Gaffer never starts work on a pathway the person has not seen, except through the start tools under the person's
  existing tool settings.

## The pages

### Where it lives

A "Pathways" link on the Factory page, beside Templates, opens `/factory/pathways`. The list is the page; the editor
opens at `/factory/pathways/<id>` (and `/factory/pathways/new`).

### The list

```
Pathways                                              [ New pathway ]
---------------------------------------------------------------------
Ship with audit                                      On [●-]   …
Plan › Code › Security audit › Review › PR › Merge
For changes that touch sign-in or payments.

Docs only                                            Off [-●]  …
Write › Proofread › PR › Merge
```

Each row: name, the stage line, description, an enable switch that saves at once, and a `…` menu with Edit,
Duplicate and Delete. Delete asks first and says running work is not affected. Empty state: one sentence on what a
pathway is and a "New pathway" button; it also offers "Start from the standard stages", which opens the editor with
code, review, test, pr, babysit and merge laid out.

### The editor

```
‹ Pathways   [ Ship with audit        ]   On [●-]     Discard  [ Save ]
----------------------------------------------------------------------
PALETTE            |  CANVAS
[Stages|Models|    |
 Skills|Prompts]   |  ┌────────┐   ┌────────┐   ┌──────────────┐   ┌──────┐
                   |  │ Plan   │ → │ Code   │ → │Security audit│ → │ PR   │ → +
 ⠿ Plan            |  │ heavy  │   │standard│   │ model-name   │   │light │
 ⠿ Design          |  │ 1 skill│   │        │   │ 2 skills ·"…"│   │      │
 ⠿ Code            |  └────────┘   └────────┘   └──────────────┘   └──────┘
 ⠿ …               |
 ⠿ Your own stage  |----------------------------------------------------
                   |  STAGE: Security audit                      Remove
 search…           |  Name  [ Security audit ]
                   |  Does it  (•) Crew tier [standard]  ( ) A named model
                   |  Skills   [threat-model ✕] [deps-check ✕]   Add skill
                   |  Prompt   [ Check every new endpoint for …        ]
```

- **Palette**, four tabs. Stages: the eight built-in blocks (one already on the canvas is shown as used) and "Your
  own stage". Models: the three Crew tiers, then favourite models, with the existing model picker behind "Other…".
  Skills: the shelf, with search. Prompts: the prompt library, with search.
- **Canvas**: stage cards in a line with arrows between and a `+` at the end. A card shows its name, who does it,
  the skill count and whether it has a prompt. The line scrolls sideways when long; under 700 px wide it runs top to
  bottom instead.
- **Dragging**: a stage block dropped on the line adds a stage where the insertion bar shows. A card dragged along
  the line reorders. A model, skill or prompt dropped on a card sets it on that stage; the card under the pointer is
  ringed, and one that cannot take the drop (a third skill) shows why in a short line. A prompt dropped on a stage
  copies its text in; it does not stay linked to the library. Drops that break an order rule are not offered: the
  insertion bar does not appear there.
- **Without dragging**: every palette item is a button. Activating a stage block adds it at the end; activating a
  model, skill or prompt applies it to the selected stage. Each card has Move earlier, Move later and Remove, and
  with focus on a card, Alt+Left/Right moves it and Delete removes it. Moves are announced to screen readers.
- **Inspector**: the selected stage's fields, editable directly, using the existing model picker and effort select.
  On a phone it and the palette are bottom sheets, and adding is by tapping, with drag still available.
- **Saving**: Save is explicit; problems are listed by stage beside the Save button and on the cards (`!`). Leaving
  with unsaved changes asks first. Motion is brief and respects reduced motion.

Dragging is written with pointer events in one small helper in `web/src/lib/factory/`, since the HTML5 drag events
the app uses elsewhere do not fire for touch and the owner uses a tablet. No drag or graph library is added.

### Using a pathway

- **New-outcome dialog and Add pipelines**: a "Pathway" select above the stage ticks, with "Standard stages" first
  and then the enabled pathways. Choosing one replaces the ticks, tier and parts controls with the pathway's stage
  line, read only, and a link "Edit pathway". Every pipeline of the outcome follows the chosen pathway.
- **Factory page**: stage strips and the pipeline detail show a custom stage by its name, and the outcome's settings
  show "Pathway: <name>". Skill choice on a pending stage works as today, so a person can still adjust one run.

## Testing

- Go: pathway validation (the order rules, limits, names); `outcomePieces` with a pathway; `stagePrompt` for a custom
  stage and for a built-in stage with extra instructions, including that the boundary survives a long prompt;
  send-back to a custom stage; a merge stage placed after a custom stage still gates; store round trip; the routes;
  the three MCP tools; a start refused for a disabled pathway and for a missing skill.
- Web unit tests for the pathway model (add, move, remove, rule messages).
- A browser suite `web/tests/pathways.e2e.mjs`: build a pathway by keyboard, build one by pointer drag, save,
  disable, start an outcome on it. Checked in light and dark at 390 px and desktop.

## Build order

Three pull requests, each working by itself: (1) the stage changes, storage, routes and start-on-a-pathway; (2) the
list and canvas editor, and the pathway select in the two dialogs; (3) Gaffer's planning suggestion and the MCP
tools. Each leaves its handoff entry and changelog fragment.

## Left to the builder

- Exact wording of messages and empty states, in the house style.
- The visual detail of cards, arrows, insertion bar and drag ghost, from the existing tokens.
- Whether the editor's state lives in one module or the component, and the shape of the drag helper.
- Whether pathways belong in the portable settings export; follow what templates do.
