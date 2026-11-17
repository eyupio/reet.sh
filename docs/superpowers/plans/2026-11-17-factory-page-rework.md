# Factory Page Rework Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Factory page's outcome cards with a list of pipelines in collapsible sections and one detail view per pipeline, so what is waiting comes first and every action and setting has one place.

**Architecture:** A pure model (`factoryList.ts`) turns the outcome records the page already loads into flat items, each in one section with one primary choice. Four small Svelte components render it: a stage pipeline in three sizes, a collapsible section, a waiting item and the pipeline detail. `Factory.svelte` keeps its loading, handlers and dialogs and has its template replaced by a composition of those pieces.

**Tech Stack:** Svelte 5 (runes), TypeScript, plain `node` unit tests (`*.test.mjs`), Playwright browser suites in `web/tests/*.e2e.mjs`.

**Spec:** `docs/superpowers/specs/2026-11-17-factory-home-rework-design.md`. Mocks: `docs/images/ui/rework/Main.png`, `ATabletList.png`, `ATabletDetail.png`, `APhoneList.png`. Home is a second plan and a second pull request.

## Global Constraints

- No controller, API, MCP or stored-field change. Web only.
- Type floor 14 px. No literal colours: use the tokens `Factory.svelte` already uses (`var(--accent)`, `var(--good)`, `var(--line)`, `var(--card)`, `var(--muted)`, `var(--ink)` and so on).
- Touch targets at least 44 px under `@media (pointer: coarse)`. Real `<button>` and `<a href>` only.
- The filled button is the choice the existing code marks `primary` (`needsYou`, `waitingDecisions`, `sessionEndedChoices`). Add no new choice.
- Narrow is the page's existing `NARROW = '(max-width: 1099px)'`.
- Deep links keep working: `/factory#<pipelineAnchor>`, `?project=`, `?outcome=1`.
- Every new unit test file is appended to the `test` script in `web/package.json`.
- Browser suites run against a production build: `cd web && npm run build && npm run preview -- --host 127.0.0.1 --port 4173`, then `REET_UI_URL=http://127.0.0.1:4173 PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/opt/pw-browsers/chromium node tests/<suite>.e2e.mjs`.
- Commit trailers on every commit:
  `Co-Authored-By: Claude Fable 5.2 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_011MNNzram3Xr2uZ9UFhX8yQ`.
- Voice for copy, commits and docs: British spelling, no em dashes, plain words.

## Review Focus

1. **An outcome with many pipelines (5) and eight stages each on a 390 px phone:** nothing overflows sideways and every stage is reachable (Task 2 browser check, Task 7).
2. **A pipeline with no stages** (`stages` absent): it lists, opens and shows its actions without a stage pipeline (Task 1 test, Task 5).
3. **The selected pipeline leaves its section while open** (a decision is taken and it moves to Working): the detail stays on that pipeline and does not jump to another (Task 6 test).
4. **An archived outcome or one whose read failed:** archived pipelines show only when "Show archived" is on, with Restore as their only action; a failed read shows the last successful list and says so (Task 6).
5. **A split stage with four parts and long part names:** the labelled pipeline wraps names without overlapping circles (Task 2 browser check).

---

### Task 1: The list model

**Files:**
- Create: `web/src/lib/factory/factoryList.ts`
- Test: `web/src/lib/factory/factoryList.test.mjs` (add to `web/package.json` `test`)

**Interfaces:**
- Consumes from `web/src/lib/gafferOutcome.ts`: `track`, `waitingDecisions`, `pipelineStatus`, `sessionLost`, `stageLabel`, types `TrackNode`, `Need`, `NeedChoice`.
- Produces:

```ts
export type FactorySection = 'waiting' | 'working' | 'finished'
export type ItemKind = 'decision' | 'approval' | 'merge' | 'stopped' | 'failed' | 'running' | 'queued' | 'done' | 'halted'
export type FactoryItem = {
  id: string            // `${outcomeId}.${pipelineKey}`
  outcomeId: string; key: string; title: string; objective: string; projectId: string
  index: number; of: number          // pipeline n of m in its outcome, 1-based
  section: FactorySection; kind: ItemKind; chip: string   // chip is the words on the kind chip
  why: string                         // one sentence; '' when nothing to say
  need?: Need                         // present for every waiting item except kind 'failed'
  since: number                       // ms; when it began waiting or last changed
  nodes: TrackNode[]; at: number      // at = index of the stage in hand, -1 with no stages
  archived: boolean
}
export function factoryItems(records: ListRecord[], goals: ReadonlyMap<string, ListGoal>, permissions?: ReadonlySet<string>): FactoryItem[]
export function inSection(items: FactoryItem[], section: FactorySection): FactoryItem[]
export function stageCaption(nodes: TrackNode[], at: number, size: 'dots' | 'circles'): string
export function sectionSummary(items: FactoryItem[], section: FactorySection, now: number): string
```

`permissions` is the set of session ids that hold a pending permission. Rules, from the spec:
- waiting: everything `waitingDecisions` returns (`kind` 'merge' → `merge`, 'ended' → `stopped`, 'goal' → `approval` when the goal is `waitingOnWorker` and the pipeline's `sessionId` is in `permissions`, else `decision`), plus state `failed` (`kind` 'failed', no `need`).
- working: states `starting`, `following`, `advancing` not caught above (`running`), and `waiting` (`queued`, chip "Waiting its turn").
- finished: `done` (`done`), `cancelled` and `unfollowed` (`halted`, chip "Stopped").
- Chips: Decision, Approval, Ready to merge, Agent stopped, Failed, Running, Waiting its turn, Finished, Stopped.
- `inSection(…, 'waiting')` is oldest `since` first; the other sections keep record order.
- `since`: the goal's `updatedAt` when there is one, else the pipeline's `updatedAt`.

- [ ] **Step 1: Write the failing tests** in `factoryList.test.mjs`, using the transpile-and-import pattern of `web/src/lib/gafferOutcome.test.mjs`:

```js
test('each pipeline is in exactly one section', …)        // a record with states following(attention), ready_to_merge, following, waiting, done, cancelled, failed
//   → sections [waiting, waiting, working, working, finished, finished, waiting], kinds [decision, merge, running, queued, done, halted, failed]
test('a goal waiting on the agent is an approval only when its session holds a permission', …)
//   same record, permissions = new Set([sessionId]) → kind 'approval', chip 'Approval'; without it → 'decision'
test('waiting items come oldest first', …)                // since 300, 100, 200 → ids in order 100, 200, 300
test('a pipeline with no stages has at -1 and an empty caption', …)
test('the caption names the stage in hand and the next', …)
//   8 stages, at 2 → dots: 'Code · 3 of 8 · next Review'; circles: 'Code in hand · Plan, Design done · next Review'
//   last stage → dots: 'Merge · 8 of 8 · the last'; first of six → circles: 'Code in hand · the first stage · next Review'
test('a closed section still says what is in it', …)
//   working with titles A, B, C → 'A, B, C'; more than three → 'A, B, C and 2 more'; waiting → 'oldest 1 h'; empty → ''
```

- [ ] **Step 2: Run** `cd web && node src/lib/factory/factoryList.test.mjs`. Expected: fails, module not found.
- [ ] **Step 3: Implement `factoryList.ts`** to the interface and rules above.
- [ ] **Step 4: Run** the test file, then `npm run check`. Expected: all pass, 0 errors.
- [ ] **Step 5: Commit** "Sort the Factory's pipelines into what waits, what works and what is finished".

### Task 2: The stage pipeline

**Files:**
- Create: `web/src/lib/factory/StagePipeline.svelte`
- Test: `web/tests/factory-pieces.e2e.mjs` (new browser suite; add to `.github/workflows/gaffer-settings.yml` beside `factory-decision.e2e.mjs`)

**Interfaces:**
- Consumes: `TrackNode`, `stageCaption` (Task 1).
- Produces: `<StagePipeline nodes={TrackNode[]} at={number} size={'labelled' | 'circles' | 'dots'} waiting={boolean} selected={number | undefined} onselect={(index: number) => void | undefined} label={string} />`.
  - `labelled`: an `<ol aria-label={label}>`; each stage a circle and its `node.label`; done shows a tick, the stage in hand `!` when `waiting` else its number, failed `×`, moved `»` (as today's `.track`). With `onselect` each stage is a `<button aria-pressed>`; without it, plain items.
  - `circles`: circles only, then `stageCaption(nodes, at, 'circles')` on one line.
  - `dots`: 12 px dots, then `stageCaption(nodes, at, 'dots')`.
  - The stage in hand uses `var(--accent)` when `waiting`, `var(--good)` as a ring when not. `aria-current="step"` on it.
  - Renders nothing when `nodes` is empty.

- [ ] **Step 1: Write the failing browser test.** The suite opens `/factory` with the routes of `factory-decision.e2e.mjs` (copy its `fixture()`), and for this task asserts on the list item of "Export API": `getByRole('list', { name: 'Stages of Export API' })` has 3 items, the first has `aria-current="step"`, and at 390 px wide `document.documentElement.scrollWidth <= 390`.
- [ ] **Step 2: Run it.** Expected: fails (no such list yet). It passes after Task 6; until then run the component check in Step 4.
- [ ] **Step 3: Implement the component.** Layout: a grid of `nodes.length` equal columns with a connecting line behind the circles, the done part of the line in `var(--good)`.
- [ ] **Step 4: Run** `npm run check`. Expected: 0 errors.
- [ ] **Step 5: Commit** "Draw a pipeline's stages in three sizes".

### Task 3: Collapsible sections that remember

**Files:**
- Create: `web/src/lib/factory/ListSection.svelte`, `web/src/lib/factory/sectionMemory.ts`
- Test: `web/src/lib/factory/sectionMemory.test.mjs` (add to `test`)

**Interfaces:**
- Produces:

```ts
export type OpenSections = Record<string, boolean>
export function readSections(storage: Pick<Storage, 'getItem'> | undefined, key: string, first: OpenSections): OpenSections
export function writeSections(storage: Pick<Storage, 'setItem'> | undefined, key: string, value: OpenSections): void
```

  `<ListSection name={string} count={number} summary={string} open={boolean} ontoggle={() => void}>{children}</ListSection>`: a `<section aria-label={name}>` whose header is one `<button aria-expanded>` holding the arrow, the name, `· count` and the summary; children render only when open.
- Storage keys: `reet.factory.sections.<userId>` (Home will use `reet.home.sections.<userId>`).

- [ ] **Step 1: Write the failing tests:** nothing stored → `first`; stored `{"working":true}` merged over `first`; unreadable JSON or a non-object → `first`; a storage that throws on `getItem` or `setItem` → `first`, and no throw; `undefined` storage → `first`.
- [ ] **Step 2: Run.** Expected: module not found.
- [ ] **Step 3: Implement both files.**
- [ ] **Step 4: Run** the test and `npm run check`.
- [ ] **Step 5: Commit** "Sections that open, close and remember".

### Task 4: A waiting item

**Files:**
- Create: `web/src/lib/factory/WaitingItem.svelte`

**Interfaces:**
- Consumes: `FactoryItem` (Task 1), `StagePipeline` (Task 2), `Approval` from `web/src/lib/home/` (as `NeedsYou.svelte` uses it), `NeedChoice`.
- Produces: `<WaitingItem item={FactoryItem} layout={'card' | 'row' | 'compact'} pipeline={'labelled' | 'circles' | 'dots'} selected={boolean} busy={boolean} permission={PendingPermission | undefined} sessionId={string} usable={(choice: NeedChoice) => boolean} onchoose={(choice: NeedChoice) => void} ondetails={() => void} onanswered={() => void} />`.
  - `card` (tablet, phone): chip, title, objective, pipeline, `why`, then buttons.
  - `compact` (the list beside an open detail on a wide screen): chip, title, objective, pipeline; the whole item is one `<button aria-pressed={selected}>` that calls `ondetails`; no action buttons.
  - `row` is for Home and is built in the Home plan; accept the value and render as `card`.
  - Buttons: the first usable choice of `item.need.choices` with `primary`, labelled without a trailing "…" → filled. For `kind === 'approval'` with `permission`, render `<Approval>` in place of the filled button. For `kind === 'failed'`, filled "Try again" calling `onchoose({ id: 'retry-pipeline', label: 'Try again' })`. Then `<button>Details and other actions</button>` ("Details" under 560 px).
  - The item's root has `id={pipelineAnchor}` passed by the page via `{...rest}` so today's hash links land on it.

- [ ] **Step 1: Extend `factory-pieces.e2e.mjs`** (still failing until Task 6): in the fixture's waiting pipeline, `getByRole('region', { name: 'Waiting for you' })` contains a button named exactly "Move to next stage" and one named "Details and other actions"; the working pipeline's item has no filled button.
- [ ] **Step 2: Implement the component.**
- [ ] **Step 3: Run** `npm run check`.
- [ ] **Step 4: Commit** "One item per waiting pipeline, with its action".

### Task 5: The pipeline detail

**Files:**
- Create: `web/src/lib/factory/PipelineDetail.svelte`
- Modify: `web/src/lib/factory/Factory.svelte` (move markup out; no behaviour change in this task)

**Interfaces:**
- Consumes: everything the per-pipeline block of `Factory.svelte` uses today (lines 493-606 at commit `1012eae`): `record`, `piece`, `goal`, `now`, `busy`, the handlers `choose`, `setDecisions`, `setYolo`, `mergeMode`, `report`, `toggleHistory`, `stop`, `putAway`, `retry`, and the setters that open dialogs (`releasing`, `deleting`, `adding`, `moving`, `telling`, `sending`, `merging`, `managing`).
- Produces: `<PipelineDetail {record} {piece} {item} {goal} … narrow={boolean} position={{ at: number; of: number } | undefined} onback onprev onnext />`. The handlers are passed as one `actions` object whose members keep the names above.
- Sections, top to bottom, with these accessible names: head (`<h2>` the pipeline title; "Open session"; the outcome menu `<button aria-label="More for this outcome">`), `<StagePipeline size="labelled" onselect>`, `region "What is waiting"`, `region "<Stage> stage"` (agent, plan left, in this stage, Gaffer reviews, latest review or handover; for a stage not started: "<agent> will do this stage"), then five `<details>` each with its headline in the `<summary>`: "Gaffer’s report", "Usage and budget", "What Gaffer decided", "History", "Settings for this outcome"; then `group "What do you want to do?"` fixed to the bottom.
- Headlines: report → "`n` pipelines proposed" or "written <when>" or "Not asked for yet"; usage → "Gaffer <tokensText> · agents <agentTokensText>"; decided → "`n` decisions" or "Nothing yet"; history → "`n` events · last <ago>" once read, else "Open to read"; settings → "<You decide|Gaffer decides> · <Ask me|YOLO> · <merge waits for you|merges by itself>".
- Settings holds exactly: the who-decides radio group, the approvals control with today's YOLO confirmation, the merge control for this pipeline with today's confirmation, and Reports as text.
- Action bar order: usable `need.choices` (primary first, filled), then, where today's rules allow them, Tell the agent…, Send back…, Move to next stage, Review goal / Goal and limits, Try again. No choice appears twice.
- With `narrow`, a top bar: "‹ Factory" (`onback`), "`at` of `of` waiting for you", and previous / next buttons (`aria-label="Previous waiting item"`, `"Next waiting item"`), disabled at the ends.

- [ ] **Step 1: Extend `factory-pieces.e2e.mjs`** (fails until Task 6): after opening the waiting pipeline's detail, `getByRole('group', { name: 'What do you want to do?' })` holds the choices with "Move to next stage" first; the settings summary reads "You decide · Ask me · merge waits for you"; choosing "Gaffer decides" posts `{ revision, on: true }` to `/automation` (the fixture already records it); selecting the "Review" stage shows a region named "Review stage".
- [ ] **Step 2: Implement by moving markup**, not rewriting it: every handler call, dialog trigger and `usable(...)` filter is carried over as it stands.
- [ ] **Step 3: Run** `npm run check`.
- [ ] **Step 4: Commit** "One detail view for a pipeline: its stages, what waits, its settings and its actions".

### Task 6: Put the page together

**Files:**
- Modify: `web/src/lib/factory/Factory.svelte`
- Test: `web/tests/factory-pieces.e2e.mjs`, and update `web/tests/gaffer-factory.e2e.mjs`, `web/tests/factory-decision.e2e.mjs`, `web/tests/gaffer-factory-session.e2e.mjs` and the phone suites that open `/factory`

**Interfaces:**
- Consumes: Tasks 1 to 5.
- Produces: the page.
  - Head: title, one line of counts ("`n` waiting for you · `n` working · `n` finished"), Gaffer settings, Give Gaffer an outcome. The four tiles go.
  - `items = factoryItems(listedRecords, byConversation, permissions)`, where `permissions` is built from the `sessions` the page already polls (those with `pendingPermission?.permissionId`).
  - Sections in order: "Waiting for you", "Working", "Finished or stopped", "Other sessions" (today's `GafferFollowedSessions`), each a `ListSection`; first-time state `{ waiting: true }`, remembered under `reet.factory.sections.<userId>`.
  - Wide (`!narrow`): rail, list (items as `compact` with `pipeline="dots"`), detail. Selection: `?pipeline=<item.id>` in the address, written with `history.replaceState`; with none, the first waiting item, else the first working one. A hash that matches a pipeline anchor selects that pipeline.
  - Narrow: items as `card` (`pipeline="labelled"` from 600 px, `"circles"` below); the detail replaces the list when `?pipeline=` is set and is never rendered beneath it; back removes the parameter; previous and next walk `inSection(items, 'waiting')`.
  - The selected pipeline stays selected when it changes section.
  - Archived: unchanged "Show archived" control; archived items join "Finished or stopped" with `archived: true`.
  - Setup card, empty states, notice, warning, foot line and every dialog: unchanged.
- [ ] **Step 1: Make the `factory-pieces` assertions from Tasks 2, 4 and 5 the first failing tests**, and add: at 1280 px the list and the detail are both visible and the first waiting pipeline's title is the detail's heading; at 834 px the detail is hidden until "Details and other actions" is pressed, after which the list is hidden, and "‹ Factory" brings it back; taking the decision on the selected pipeline (the fixture moves it to the next stage) leaves the detail on the same pipeline; "Working" starts closed and its header reads its pipelines' names; reload keeps a section the test opened.
- [ ] **Step 2: Run.** Expected: fail.
- [ ] **Step 3: Replace the template** of `Factory.svelte` from the tiles to the foot line with the composition above. Keep the script's loaders and handlers; delete only the code that drove the removed card folding (`outcomeFolds`, `folds`, `jump`, the tiles).
- [ ] **Step 4: Update the existing suites** so each assertion finds its control in the new place (the detail's action bar, the settings row, the outcome menu). Do not delete an assertion without replacing it with the new behaviour's check.
- [ ] **Step 5: Run** `npm run check` and every suite named in Files. Expected: all pass.
- [ ] **Step 6: Commit** "The Factory page opens on what is waiting".

### Task 7: The Projects picker on a narrow screen, and the phone

**Files:**
- Create: `web/src/lib/factory/ProjectPicker.svelte`
- Modify: `web/src/lib/factory/Factory.svelte`
- Test: `web/tests/factory-pieces.e2e.mjs`

**Interfaces:**
- Produces: `<ProjectPicker projects={…} counts={Map<string, { waiting: number; working: number }>} chosen={string} onchoose={(id: string) => void} />`: a button "Projects: all" / "Projects: <name>" that opens a dialog listing "All projects" and each project with "`n` waiting · `n` working". Choosing sets the page's existing `scope`. It does not add, group or edit projects. Shown only when `narrow`.
- On the wide rail each project line gains the same counts (`ProjectRailHost` already accepts per-project text through its `running` prop; if it cannot carry two counts, show "`n` waiting" only and say so in the PR).

- [ ] **Step 1: Failing test:** at 834 px, "Projects: all" opens a dialog; choosing "billing" leaves only billing's pipelines listed and the button reads "Projects: billing". At 390 px with five pipelines of eight stages (extend the fixture), `scrollWidth <= 390`, every button in the first card is at least 44 px tall, and the caption under the circles reads "Code in hand · Plan, Design done · next Review".
- [ ] **Step 2: Implement.**
- [ ] **Step 3: Run** `npm run check` and `factory-pieces`.
- [ ] **Step 4: Commit** "Narrow the Factory to one project on a tablet or phone".

### Task 8: Captures and notes

**Files:**
- Modify: `docs/ui-verification.md`, `CHANGELOG.md` (Unreleased), `handoff.md`, `docs/handoffs/gaffer-factory-floor.md`
- Create: captures under `docs/images/ui/` named `factory-rework-<width>-<theme>.png`

- [ ] **Step 1:** In `factory-pieces.e2e.mjs`, when `REET_UI_SCREENSHOTS` is set, save captures at 1440 light, 1440 dark, 834 light (list), 834 light (detail) and 390 dark, at `deviceScaleFactor: 2`.
- [ ] **Step 2:** Look at each capture. Fix anything misaligned, clipped or below the type floor before going on.
- [ ] **Step 3:** Write the `docs/ui-verification.md` section (what was checked, at which widths, in which suite), the CHANGELOG entry, and the handoff section (what moved where, what was not checked: no real machine, no real tablet).
- [ ] **Step 4:** Run `npm run check` and all the suites from Task 6 once more.
- [ ] **Step 5: Commit** "Record what the Factory rework was checked against".
