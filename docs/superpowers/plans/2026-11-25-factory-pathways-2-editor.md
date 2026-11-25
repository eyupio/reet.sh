# Factory pathways, part 2: the list, the canvas editor and choosing a pathway — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A person can build, edit, enable, disable, duplicate and delete pathways on a drag-and-drop canvas, and choose one when starting work.

**Architecture:** One lazily loaded page component owns `/factory/pathways`, `/factory/pathways/new` and `/factory/pathways/<id>` and switches between a list and an editor with `history.pushState`, as `web/src/lib/desks/DesksPage.svelte` does. All pathway rules live in one pure TypeScript module that the editor, the palette and the tests share; the server stays the authority. Dragging is one small pointer-event helper (mouse, touch and pen), with a button and keyboard way for every action.

**Tech Stack:** Svelte 5 (runes), TypeScript, Go for one small endpoint. No new dependency.

**Spec:** `docs/superpowers/specs/2026-11-25-factory-pathways-design.md` (sections "The pages", "What would make it a failure"). Mockups the owner approved: https://claude.ai/artifact/F2AcstMVfPu6ZQ58jAUVEc (editor dark, editor mid-drag light, list, choosing a pathway, phone editor). They show layout and content; styling comes from the app's tokens.

## Global Constraints

- British spelling, plain words, no em dashes in UI copy. Commit subjects are an imperative sentence with no prefix.
- Colours only from the tokens in `web/src/app.css` (a literal colour fails `web/src/lib/theme.test.mjs`). Sizes in px. Type no smaller than 14 px. The product's own marks (✓ ! ✕ · …). 44 px touch targets on coarse pointers.
- Motion only inside `@media (prefers-reduced-motion: no-preference)`.
- `web/src/lib/api.ts` may import only `./inflight` at run time (`import type` is fine).
- Nothing on the canvas can be done only by dragging: every drag has a button or keyboard way. Dragging works by touch.
- Limits, as the server enforces them: name 1 to 60 characters; description up to 300 bytes; 1 to 12 stages; a built-in kind at most once; merge last; babysit and merge need pr before them; custom stage name 1 to 40 characters, unique without case, never a built-in kind's name or `custom`; prompt up to 4,000 bytes, required for custom; up to 2 skills and 2,000 bytes of guidance a stage; 50 pathways.
- New pathways start enabled. A disabled pathway is offered nowhere.
- Checked in light and dark, at 390 px and desktop.
- The pull request adds `docs/handoffs/log/PATHWAYS-2.md` and `changelog.d/pathways-editor.md`.
- Do not skip, retry or lengthen a timeout to get green.

## Review Focus

1. Leaving the editor with unsaved changes (back link, browser back, closing the tab): the person is asked first, and Discard really discards.
2. Someone else's save landing first (`pathway_changed`): the editor says so, keeps the person's edits on screen, and offers "Load the saved one" or "Keep editing"; it never overwrites silently.
3. A drop that would break an order rule (merge not last, babysit before pr, a second copy of a built-in kind): not offered while dragging, refused with a sentence when tried by button.
4. A skill that has gone from the shelf on a saved stage: shown on the card and in the inspector with `!`, removable, and Save says why it cannot be used.
5. A long pathway (12 stages) at 390 px and at desktop: every card reachable by scroll and by keyboard, and the selected card scrolls into view.

## File structure

- `internal/api/gaffer_pathways.go` (+ test): `GET /api/v2/gaffer/pathways/skills`.
- `internal/webroute/routes.go`, `web/embed_test.go`, `internal/webroute/routes_test.go`: the new paths.
- `web/src/lib/api.ts`: client functions and `pathway` on the two start calls.
- `web/src/lib/factory/pathways.ts` (+ `pathways.test.mjs`): types, edits, rules.
- `web/src/lib/factory/pathways/PathwaysPage.svelte` (routing shell), `PathwayList.svelte`, `PathwayEditor.svelte`, `PathwayPalette.svelte`, `PathwayLine.svelte`, `StageCard.svelte`, `StageInspector.svelte`, `drag.ts` (+ `drag.test.mjs`), `PathwayChoice.svelte` (the select used by both dialogs).
- `web/src/App.svelte`, `web/src/lib/factory/Factory.svelte`, `web/src/lib/GafferOutcome.svelte`, `web/src/lib/factory/AddPipelines.svelte`: entry points.
- `web/tests/pathways.e2e.mjs`, `web/tests/ci-gaffer-settings.txt`.

---

### Task 1: Skills a stage can load, and the page paths

**Files:** Modify `internal/api/gaffer_pathways.go`, `internal/api/gaffer_pathways_test.go`, `internal/webroute/routes.go`, `internal/webroute/routes_test.go`, `web/embed_test.go`.

**Interfaces — Produces:** `GET /api/v2/gaffer/pathways/skills` → `{"skills":[{"id","name","description","bytes"}]}`: every skill `buddyCatalogue` (internal/api/skill_buddy.go:84) offers the account (built-ins that are on, Active library skills), sorted by name without case, `bytes` being `len(skillbuddy.Prompt(skill))` as `UsePathway` measures it. Registered **before** `GET /api/v2/gaffer/pathways/{id}` is matched (Go's mux prefers the literal path; assert it in the test). The server answers the SPA for `/factory/pathways`, `/factory/pathways/new` and `/factory/pathways/<one segment>` and for nothing deeper, following the desks special case in `routes.go:36-40`.

- [ ] **Step 1: failing tests.** `TestPathwaySkillsListsWhatAStageCanLoad` (a built-in that is on and an Active library skill are listed with their byte sizes; an archived library skill is not; another account's library skill is not; signed out is 401). `routes_test.go`: the three paths are pages, `/factory/pathways/a/b` is not. Add the paths wherever `web/embed_test.go` lists `/factory` (:102, :139, :159, :195-196, :238).
- [ ] **Step 2:** `go test ./internal/api -run PathwaySkills ./internal/webroute ./web` fails. **Step 3:** implement. **Step 4:** passes, and `go test ./internal/api ./internal/webroute ./web` passes.
- [ ] **Step 5:** Commit "List the skills a pathway stage can load".

### Task 2: The pathway model and API client

**Files:** Create `web/src/lib/factory/pathways.ts`, `web/src/lib/factory/pathways.test.mjs`. Modify `web/src/lib/api.ts` (+ the test that compiles it, as `templateApi.test.mjs` does).

**Interfaces — Produces** (`pathways.ts`; every edit returns a new pathway and never mutates):

```ts
export type PathwayAgent = { tool: string; model?: string; effort?: string; provider?: string }
export type PathwayStage = { kind: string; name?: string; prompt?: string; tier?: CrewTier; agent?: PathwayAgent; skills?: string[] }
export type Pathway = { id: string; name: string; description: string; enabled: boolean; stages: PathwayStage[]; createdAt: number; updatedAt: number; problems?: string[] }
export type PathwaySkill = { id: string; name: string; description: string; bytes: number }
export const MAX_PATHWAY_STAGES = 12, MAX_STAGE_SKILLS = 2, MAX_STAGE_GUIDANCE_BYTES = 2000, MAX_STAGE_PROMPT_BYTES = 4000, MAX_STAGE_NAME = 40, MAX_PATHWAY_NAME = 60, MAX_PATHWAY_DESCRIPTION_BYTES = 300
export const blankPathway = (): Pathway                       // enabled, no stages, id ''
export const standardPathway = (): Pathway                    // code, review, test, pr, babysit, merge on their default tiers
export const newStage = (kind: string, taken: PathwayStage[]): PathwayStage  // custom gets name "Stage N" (first N free) and an empty prompt; built-in gets its default tier
export function canInsert(stages: PathwayStage[], stage: PathwayStage, at: number): string  // '' or the sentence saying why not
export function canMove(stages: PathwayStage[], from: number, to: number): string
export function insertStage(p: Pathway, stage: PathwayStage, at: number): Pathway
export function moveStage(p: Pathway, from: number, to: number): Pathway
export function removeStage(p: Pathway, at: number): Pathway
export function patchStage(p: Pathway, at: number, patch: Partial<PathwayStage>): Pathway   // setting tier clears agent and the reverse
export function canAddSkill(stage: PathwayStage, skill: PathwaySkill, shelf: PathwaySkill[]): string
export function addSkill(p: Pathway, at: number, id: string): Pathway
export function removeSkill(p: Pathway, at: number, id: string): Pathway
export type StageProblem = { at: number; field: 'name' | 'prompt' | 'skills' | 'order' | 'who'; text: string }
export function pathwayProblems(p: Pathway, shelf: PathwaySkill[] | null): { pathway: string[]; stages: StageProblem[] }  // shelf null = not loaded, skill checks skipped
export const stageTitle = (stage: PathwayStage): string       // custom name, else the built-in label
export const stageWho = (stage: PathwayStage): string         // "heavy · crew" or "model · effort"
export const byteLength = (text: string): number              // UTF-8 bytes, via TextEncoder
export const sameEdit = (a: Pathway, b: Pathway): boolean     // equal ignoring updatedAt/problems: the unsaved-changes test
```

`canInsert`/`canMove` apply the three order rules and the 12-stage limit to the result. `pathwayProblems` mirrors every server rule in Global Constraints so Save is disabled with reasons before the server is asked.

`api.ts` gains, in the style of the template functions at :439-449: `listGafferPathways(signal?)`, `getGafferPathway(id, signal?)`, `createGafferPathway(input, signal?)`, `saveGafferPathway(pathway, signal?)` (PUT with the pathway's `updatedAt`), `deleteGafferPathway(id, signal?)`, `listGafferPathwaySkills(signal?)`; `GafferOutcomeInput` gains `pathway?: string`; `addGafferPipelines` gains a fourth parameter `pathway?: string` sent top-level, with each session's `stages` optional when it is set.

- [ ] **Step 1: failing tests** (`pathways.test.mjs`): `canInsert` refuses a second `code`, anything after `merge`, `merge` not at the end, `babysit` before `pr`, a 13th stage, and allows `review` before `code` and a custom stage anywhere before `merge`; `canMove` refuses moving `pr` after `babysit` and moving anything past `merge`; `newStage('custom', …)` names "Stage 1", then "Stage 2"; `patchStage` with a tier clears the agent; `canAddSkill` refuses a third skill, a repeat, and a pair over 2,000 bytes with a sentence naming the limit; `pathwayProblems` reports each rule (empty custom prompt, custom named "Merge" and " merge ", two customs "Audit"/"audit", 4,001-byte prompt counted in UTF-8 bytes with `é`, name of 61 characters, no stages, a skill id missing from the shelf) and nothing for `standardPathway()`; edits do not mutate their input (deep-freeze the argument); `sameEdit` ignores `updatedAt`. The api test: `saveGafferPathway` PUTs to `/api/v2/gaffer/pathways/<id>` with `updatedAt` in the body; `addGafferPipelines(o, sessions, undefined, 'p1')` sends `pathway: 'p1'`.
- [ ] **Step 2:** `cd web && node scripts/run-tests.mjs` fails on the new files. **Step 3:** implement. **Step 4:** `npm run check` passes. **Step 5:** Commit "Model a pathway and its rules in the client".

### Task 3: The pathways page and list

**Files:** Create `web/src/lib/factory/pathways/PathwaysPage.svelte`, `PathwayList.svelte`. Modify `web/src/App.svelte` (a `Pathways` entry in `pages` :45-76; `const isPathways = /^\/factory\/pathways(?:\/[^/]+)?\/?$/.test(location.pathname)`, tested **before** `isFactory`; join every place `isFactory` is used: layout :203, tab `current` :207, title :573), `web/src/lib/factory/Factory.svelte:570-575` (`<a class="btn quiet" href="/factory/pathways">Pathways</a>` beside Templates).

**Interfaces — Produces:** `PathwaysPage` props `{ userId, onsignedout }`; it parses the route (`list` | `new` | `{ id }`), moves with `history.pushState` and a `popstate` listener as `DesksPage.svelte:88-98` does, and renders `PathwayList` or (Task 4) `PathwayEditor`. `PathwayList` props `{ pathways: Pathway[]; onopen(id); onnew(start: 'blank' | 'standard'); onchange() }`.

Behaviour, as the "Pathways list" mockup: `PageHeader` with group "Factory" → `/factory`, title "Pathways", the lede from the mockup, and a "New pathway" primary button. Each row: name (opens the editor), the stage line as chips (custom stages in the accent style), description, any `problems` as a `!` line, a `role="switch"` button (copy `MachineSettings.svelte:272` and its CSS :529-538) labelled "<name> is on, switch off" / "…off, switch on" that saves at once and rolls back with a notice if the save fails, and a `…` menu (`aria-haspopup="menu"`, pattern of `home/ProjectRail.svelte:633`) with Edit, Duplicate and Delete. Duplicate creates "<name> copy" (then "copy 2"…, trimmed to 60) disabled-state copied, and opens it. Delete uses `ConfirmDialog` (danger) whose body says work already running on it is not affected. Empty state: one sentence on what a pathway is, "New pathway" and "Start from the standard stages". At 50 pathways "New pathway" is disabled with the reason. Loading uses the page skeleton pattern; a load failure shows a notice with Retry.

- [ ] **Step 1:** In `web/tests/pathways.e2e.mjs`, modelled on `factory-templates.e2e.mjs` (`fixture()` routes map, `openWorkspace`), write the list checks: three pathways render with their stage lines; the switch PUTs `enabled:false` with the pathway's `updatedAt`; Delete asks, then DELETEs and the row goes; Duplicate POSTs "<name> copy"; the Factory page's Pathways link lands on the list; browser back returns to the Factory. Run it against a built UI (see AGENTS.md "Browser suites") and watch it fail.
- [ ] **Step 2:** implement. **Step 3:** the suite's list checks pass at desktop width and 390 px, light and dark (`openWorkspace` takes `width` and `theme`); `npm run check` passes.
- [ ] **Step 4:** Commit "Add the pathways list".

### Task 4: The editor, without dragging

**Files:** Create `PathwayEditor.svelte`, `PathwayPalette.svelte`, `PathwayLine.svelte`, `StageCard.svelte`, `StageInspector.svelte` in `web/src/lib/factory/pathways/`. Modify `PathwaysPage.svelte`.

**Interfaces — Produces:** `PathwayEditor` props `{ id: string | null; start: 'blank' | 'standard'; onclose(); onsaved(p: Pathway) }`. It holds `saved: Pathway` and `draft: Pathway` (`$state`), `selected: number` (-1 none), and loads in parallel: the pathway (when `id`), `listGafferPathwaySkills`, `readPromptLibrary`, `listModelFavourites`, the crew. Children get data and callbacks only; all edits go through `pathways.ts`.

- `PathwayPalette` props `{ tab; stages: PathwayStage[]; shelf; prompts; favourites; crew; onadd(kind); onmodel(choice); onskill(id); onprompt(text); canskill(id): string }`. Four tabs as a `role="tablist"`; a search box filters the open tab. Stages: the eight built-ins (one already on the line is shown "in use" and disabled) and "Your own stage". Models: the three crew tiers (`CREW_TIERS`, with what each tier's crew member is) then favourites, then "Other…" which focuses the inspector's model picker. Skills: the shelf with name, description; an item the selected stage cannot take is disabled with the reason as its hint. Prompts: the library, title and first line. Every item is a `<button>`: activating a stage block adds it at the last place `canInsert` allows (before `merge` if there is one) and selects it; a model, skill or prompt applies to the selected stage, and with none selected a hint says "Select a stage first".
- `PathwayLine` props `{ stages; selected; problems: StageProblem[]; onselect(at); onmove(from, to); onremove(at); onaddend() }`. A `role="list"` of `StageCard`s with arrows between and a `+` at the end (opens the Stages tab). Horizontal with sideways scroll from 700 px wide, top to bottom below. The selected card is scrolled into view.
- `StageCard` shows the title, "Built in" or "Your own" (merge adds "waits for you"), `stageWho`, the skill count, "prompt ✓" when it has one, `!` with the first problem. It is a `<button>` (`aria-pressed` when selected); with focus on it Alt+ArrowLeft/Right (ArrowUp/Down when the line is vertical) moves it, Delete or Backspace removes it; each move is announced in an `aria-live="polite"` region ("Security audit moved to place 2 of 5").
- `StageInspector` props `{ stage; at; count; shelf; machine models…; onpatch(patch); onmove(to); onremove(); onremoveskill(id) }`: "Stage N of M" with Earlier, Later and Remove buttons (disabled with a `title` reason when `canMove` refuses); Name (custom only); Who does it as two radio pills, "Crew tier" (a select of the three tiers) or "A named model" (agent select, `GafferModelSelect`, the effort select from `crewChoices`, `ClineSourceSelect` when the agent takes a source, as `GafferCrewSettings.svelte:95-108` builds a row) with the line "A named model has no stand-in when its plan runs low. A crew tier does."; Skills as removable chips with "N of 2" and bytes used; Prompt textarea labelled "Prompt" (custom: "This is the stage's work.") or "Extra instructions" (built-in), with "N of 4,000 bytes". Below 700 px the inspector and palette are bottom sheets as in the phone mockup.
- Top bar: back link "‹ Pathways", the name as an inline input, the on/off switch (part of the draft; saved with Save), a description input, "Unsaved changes" when `!sameEdit(saved, draft)`, Discard, Save. Save is disabled while `pathwayProblems` reports anything, with the problems listed beside it; saving creates or PUTs, then replaces `saved` and `draft` with the answer. `pathway_changed` → the choice in Review Focus 2. `pathway_name` → the name field is marked with "You already have a pathway with that name." `pathway_limit` → a notice. Leaving with unsaved changes (back link, `popstate`, `beforeunload`) asks through `ConfirmDialog` ("Leave without saving?" / Discard changes / Keep editing).

- [ ] **Step 1:** Extend `pathways.e2e.mjs`, keyboard and buttons only: start from the standard stages; add "Your own stage", rename it "Security audit", type a prompt, move it earlier twice with Alt+ArrowLeft and hear the live region; put it on a named model; add two skills and see the third refused with its reason; apply a library prompt; try to add a second Code (disabled, "in use"); Save posts the expected body (assert the JSON); an edit then the back link asks before leaving; a mocked `pathway_changed` shows the two choices and keeps the draft; a pathway whose stage names a skill missing from the shelf shows `!` and cannot be saved until it is removed. Watch it fail.
- [ ] **Step 2:** implement. **Step 3:** passes at desktop and 390 px, light and dark; `npm run check` passes.
- [ ] **Step 4:** Commit "Add the pathway editor".

### Task 5: Dragging

**Files:** Create `web/src/lib/factory/pathways/drag.ts`, `drag.test.mjs`. Modify `PathwayPalette.svelte`, `PathwayLine.svelte`, `StageCard.svelte`, `PathwayEditor.svelte`.

**Interfaces — Produces:**

```ts
export type DragItem = { type: 'stage'; kind: string } | { type: 'card'; from: number } | { type: 'model'; choice: ModelChoice } | { type: 'skill'; id: string } | { type: 'prompt'; text: string }
export type DropTarget = { type: 'slot'; at: number } | { type: 'card'; at: number } | null
export type DragState = { item: DragItem; x: number; y: number; target: DropTarget; why: string } | null
// Pure: which target the pointer is over, from the cards' rectangles and the line's direction.
export function targetAt(item: DragItem, x: number, y: number, cards: DOMRect[], line: DOMRect, vertical: boolean): DropTarget
// A Svelte action for anything that can be picked up.
export function draggable(node: HTMLElement, opts: { item: () => DragItem; onstart(s: DragState); onmove(x: number, y: number); onend(drop: boolean) }): { destroy(): void }
```

`draggable` follows `desks/DeskTabs.svelte:87-120` but for every pointer type: on `pointerdown` remember the point; start the drag after 6 px of movement for a mouse or pen, or after a 250 ms hold without moving more than 8 px for touch (so a touch scroll of the palette or line still scrolls); then `setPointerCapture`, set `touch-action: none` for the drag's length, call `onstart`; `pointermove` → `onmove`; `pointerup` → `onend(true)`; `pointercancel` or Escape → `onend(false)`; swallow the click that follows a drag. While dragging near an edge of the line's scroller, scroll it. The editor owns `DragState`: for a `stage` or `card` item the target is a slot, offered only where `canInsert`/`canMove` returns ''; for `model`, `skill` and `prompt` the target is a card, ringed when it can take the drop and dimmed with `why` set when it cannot. It draws the ghost (a copy of the block, following the pointer, `pointer-events: none`), the insertion bar in the offered slot, the ring on a target card, and one hint line under the line ("Drop to give Code the skill test-first. Security audit already has 2."). A drop calls the same `pathways.ts` edit the buttons call and selects the stage. Grips (`⠿`) get `cursor: grab`; reduced motion removes the ghost's tilt and the cards' slide.

- [ ] **Step 1: failing tests.** `drag.test.mjs` for `targetAt`: before the first card → slot 0, between cards 1 and 2 → slot 2, past the last → slot n, over a card with a `skill` item → that card, outside the line → null, and the same vertically. Extend `pathways.e2e.mjs` with real pointer input (`page.mouse` down/move in steps/up): drag "Design" from the palette to between Plan and Code; drag a card two places later; drag a skill on to a card and see the count change; drag a skill over a full card and see the reason and no change on drop; press Escape mid-drag and see nothing change; drag "Merge" from the palette and see the bar offered only at the end. Then one touch case with `openWorkspace({ touch: true, width: 390 })` driving `page.touchscreen`/CDP touch events: hold a card, move it down one place, release, and the order changed; a quick swipe on the line scrolls and moves nothing. Watch them fail.
- [ ] **Step 2:** implement. **Step 3:** unit and browser checks pass, light and dark, desktop and 390 px. Take the four screenshots the mockups correspond to (deviceScaleFactor 2) into `docs/images/ui/pathways/` and look at each one against its mockup; fix what does not match before going on.
- [ ] **Step 4:** Commit "Drag stages, models, skills and prompts on to a pathway".

### Task 6: Choose a pathway when starting work

**Files:** Create `web/src/lib/factory/pathways/PathwayChoice.svelte`. Modify `web/src/lib/GafferOutcome.svelte` (stage controls :655-671, body :520-526), `web/src/lib/factory/AddPipelines.svelte` (:152-197, plans :88-92, `AddedPipeline` :16), `web/src/lib/factory/Factory.svelte:333`.

**Interfaces — Produces:** `PathwayChoice` props `{ pathways: Pathway[]; value: string ($bindable, '' = standard stages); suggested?: { id: string; why: string } }` (the `suggested` prop is for part 3; unused here). A radio group as the "Choosing a pathway" mockup: each enabled pathway with its stage line, then "Standard stages" with "Choose the stages, tiers and parts yourself, as now." A pathway with `problems` is shown disabled with its first problem. With one chosen, a link "Edit pathway" opens `/factory/pathways/<id>` in a new tab so the dialog is not lost. With no enabled pathways the component renders nothing and the dialogs look as they do today.

Both dialogs load `listGafferPathways()` when opened (a failure hides the choice and changes nothing else). With a pathway chosen the stage chips, tier selects and parts controls are replaced by the pathway's read-only stage line; the body sends `pathway: id` and sessions with no `stages`; the "Start N pipelines" count and every other field are unchanged. Add pipelines starts on the pathway the outcome's first pipeline used (`pathwayId`) when it is still enabled.

- [ ] **Step 1:** Extend `pathways.e2e.mjs`: the new-outcome dialog with two enabled and one disabled pathway offers the two and "Standard stages"; choosing one hides the stage ticks, and the POST carries `pathway` and no `stages`; "Standard stages" sends the body it sends today (compare with the body captured before the change); Add pipelines preselects the outcome's pathway and sends `pathway` top-level; with no pathways the dialog's markup has no choice. Fail first.
- [ ] **Step 2:** implement. **Step 3:** passes; the existing dialog suites still pass: run `factory-templates.e2e.mjs` and every suite in `web/tests/ci-gaffer-settings.txt` that opens these dialogs. `npm run check` passes.
- [ ] **Step 4:** Commit "Choose a pathway when starting work".

### Task 7: The suite in CI, and the notes

**Files:** Modify `web/tests/ci-gaffer-settings.txt` (add `pathways.e2e.mjs` after the factory-* suites at :12-16). Create `docs/handoffs/log/PATHWAYS-2.md`, `changelog.d/pathways-editor.md`. Modify the user docs page that describes Factory templates (find it with `rg -l 'Factory templates' docs README.md`) to add a short "Pathways" section with one of the screenshots.

- [ ] **Step 1:** Run the whole of `scripts/run-browser-suites.sh` for the gaffer-settings list locally as CI does and see it pass. `go vet ./... && go test ./internal/api ./internal/webroute ./web` and `cd web && npm run check` pass.
- [ ] **Step 2:** Write the handoff entry (what, why, checked including light/dark and 390 px, not done: Gaffer's suggestion and the MCP tools are the next pull request) and the changelog fragment (a person can now build pathways under Factory › Pathways and pick one when starting work).
- [ ] **Step 3:** Commit "Run the pathways suite in CI and document pathways".
