# Home Rework Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give Home one "Waiting for you" list that mixes Factory pipelines and sessions, each with its action, in place of the separate "Factory pipelines" and "Needs you" lists, keeping the Projects panel, both views and Your machines.

**Architecture:** A pure model (`homeWaiting.ts`) merges the Factory's items (`factoryItems`, already built) with Home's session cards (`needsYou` in `home.ts`) into one ordered list. The Factory page's choice handling and its dialogs move out of `Factory.svelte` into one component both pages mount, so an action behaves the same wherever it is pressed. Home renders the list with the Factory's `ListSection` and `WaitingItem`, plus one new item component for session cards.

**Tech Stack:** Svelte 5 (runes), TypeScript, plain `node` unit tests (`*.test.mjs`), Playwright browser suites in `web/tests/*.e2e.mjs`.

**Spec:** `docs/superpowers/specs/2026-11-17-factory-home-rework-design.md`, section "Home" and "The shared pieces". Mocks: `docs/images/ui/rework/HomeWide.png`, `HomeBoard.png`, `HomeTablet.png`. The Factory half is merged (#825); read `handoff.md` FACTORY-REWORK-1 for what exists.

**Decisions the owner made after the spec** (they override it where they differ):
- The floating Gaffer button floats over the list on both pages. The reserved strip #825 gave it on the Factory page is removed.
- A session that has only finished is not in Waiting for you (already in the spec).

## Global Constraints

- No controller, API, MCP or stored-field change. Web only.
- Type floor 14 px. No literal colours: use the tokens the pages already use.
- Touch targets at least 44 px under `@media (pointer: coarse)`. Real `<button>` and `<a href>` only.
- Add no new choice: a Factory item's choices come from `needsYou` / `waitingDecisions` / `sessionEndedChoices` in `web/src/lib/gafferOutcome.ts`, exactly as on the Factory page.
- Kept as they are: the Projects panel on a wide screen, the **Continue working** and **Planning board** views with their remembered choice, **Your machines**, the plan meters, New session and Give Gaffer an outcome, every dialog, and everything Home shows an account that cannot start sessions.
- Narrow is the page's existing `NARROW = '(max-width: 1099px)'`.
- Every new unit test file is appended to the `test` script in `web/package.json`.
- Browser suites run against a production build (see `handoff.md` for the commands).
- Voice for copy, commits and docs: British spelling, no em dashes, plain words. No model names.

## Review Focus

1. **The same session twice.** A Factory pipeline's session that holds a permission is also a session card: it must appear once, as the Factory item (Task 1 test, Task 4 browser check).
2. **The bell and the page title.** They count what is in Waiting for you: Factory items join, plainly finished sessions leave (Task 1 test, Task 4 browser check on the title).
3. **An account with no Factory** (no accounts sign-in, or a read-only account): Home shows session items only, no Factory section, and nothing throws (Task 4).
4. **Acting from Home.** A choice pressed on Home does what it does on the Factory page, including its confirmation, and the list updates without a reload (Task 2 regression, Task 4 browser check).
5. **Nothing waiting.** The section says so in one line and the views below are not pushed down by an empty box (Task 4).

---

### Task 1: The merged list

**Files:**
- Create: `web/src/lib/home/homeWaiting.ts`
- Test: `web/src/lib/home/homeWaiting.test.mjs` (add to `test`)

**Interfaces:**
- Consumes: `FactoryItem`, `inSection` from `web/src/lib/factory/factoryList.ts`; `NeedCard` from `web/src/lib/home.ts`.
- Produces:

```ts
export type WaitingEntry =
  | { source: 'factory'; id: string; since: number; item: FactoryItem }
  | { source: 'session'; id: string; since: number; card: NeedCard }
export function homeWaiting(items: FactoryItem[], cards: NeedCard[]): WaitingEntry[]
export function waitingSummary(entries: WaitingEntry[], now: number): string
export function workingSummary(items: FactoryItem[]): string
```

Rules:
- Factory entries: `inSection(items, 'waiting')` that are not `archived`; `id` is the item's `id`; `since` the item's.
- Session entries: every card except `kind === 'finished'`, and except a card whose `sessionId` is the session in hand of any Factory item passed in (waiting, working or finished). `id` is `session:<sessionId>`; `since` is the card's `at`.
- Order: oldest `since` first; ties by `id`.
- `waitingSummary`: counts by kind in this order, singular or plural, joined by ", " then " · oldest <age>": "1 decision, 2 approvals, 1 merge, 1 question, 1 failed, 1 to review · oldest 1 h". Kinds: Factory `decision`, `approval`, `merge`, `stopped` ("stopped"), `failed`; session cards with an approval count as approvals, with only a question as questions, `failed` as failed, `review` as "to review". Empty list → `''`. Age words as `sectionSummary` in `factoryList.ts` gives them.
- `workingSummary`: the titles of `inSection(items, 'working')`, as `sectionSummary` does for Working ("A, B, C and 2 more").

- [ ] **Step 1: Write the failing tests** (the transpile-and-import pattern of `web/src/lib/factory/factoryList.test.mjs`):

```js
test('a finished session is not waiting, a failed one and one to review are', …)
test('a pipeline’s own session appears once, as the Factory item', …)   // card.sessionId === item.sessionId → only the factory entry
test('entries come oldest first across both sources', …)              // since 300 (session), 100 (factory), 200 (session) → 100, 200, 300
test('an archived pipeline is not waiting', …)
test('the summary counts each kind and names the oldest', …)           // exact string above for a mixed list; '' when empty
test('the working summary names the pipelines', …)
```

- [ ] **Step 2: Run** `cd web && node src/lib/home/homeWaiting.test.mjs`. Expected: module not found.
- [ ] **Step 3: Implement.** `FactoryItem` does not carry the session id today: add `sessionId: string` to it in `factoryList.ts` (from the pipeline's `sessionId`, `''` when none), with one assertion in `factoryList.test.mjs`.
- [ ] **Step 4: Run** both test files and `npm run check`.
- [ ] **Step 5: Commit** "Merge what the Factory and the sessions are waiting for into one list".

### Task 2: One place that carries out a pipeline's choice

**Files:**
- Create: `web/src/lib/factory/PipelineChoices.svelte`
- Modify: `web/src/lib/factory/Factory.svelte`

**Interfaces:**
- Produces: `<PipelineChoices bind:this={choices} {userId} {timeZone} {dateFormat} goals={byConversation} onchanged={() => void} onnotice={(text: string, bad?: boolean) => void} onsignedout={…} />` with one exported method `choose(choice: NeedChoice, record: GafferOutcome, piece: GafferOutcomeSession): void`, and exported `busy: boolean` (bindable).
- It owns, moved out of `Factory.svelte` unchanged in behaviour: the `choose` function and what it calls (resume the goal, retry, move on, open, answer, restart, stop), and the dialogs those open: move on, tell the agent (with Gaffer's suggested words prefilled), approve merge, stop following, goal and limits. Their confirmations, request ids, error handling and notices stay as they are.
- `Factory.svelte` mounts it once and routes every choice through it (`WaitingItem` and `PipelineDetail` already call the page's `choose`). Dialogs that are not about a pipeline's choice (send back, add pipelines, YOLO, the merge switch's YOLO confirmation, archive, delete) stay in `Factory.svelte`.

- [ ] **Step 1:** This is a move with no behaviour change, so the existing suites are the test. Run `factory-pieces`, `factory-decision`, `gaffer-factory`, `gaffer-factory-session`, `gaffer-outcome` on a fresh build first and keep the passing output.
- [ ] **Step 2: Move the code.** No assertion in any suite changes in this task.
- [ ] **Step 3: Run** `npm run check` and the five suites again. Expected: all pass, unchanged.
- [ ] **Step 4: Commit** "Carry out a pipeline's choice in one component, for the Factory page and Home".

### Task 3: Items for Home

**Files:**
- Modify: `web/src/lib/factory/WaitingItem.svelte`
- Create: `web/src/lib/home/SessionWaitingItem.svelte`
- Modify: `web/src/lib/home/NeedsYou.svelte` (only to share its card's pieces, if that is how it is done)

**Interfaces:**
- `WaitingItem` `layout="row"` (it renders as `card` today): one row with four parts: the chip in a fixed 124 px column; the text (title, then "Factory · <objective>" in the muted colour, then the sentence); the stage pipeline with `pipeline="dots"` in a fixed 180 px column; the buttons in a fixed, right-aligned column. Below 720 px of its own width it falls back to `card`. Same buttons, same `Approval`, same Recommended caption as `card`.
- `<SessionWaitingItem card={NeedCard} layout={'card' | 'row'} onseen={(sessionId: string) => void} onanswered={() => void} />`: the chip (Approval, Question, Failed, To review, from the card's reasons), the title, "Session · <where>", one sentence (the approval's title and command, the question's message, or the card's label and age), then: for an approval the existing `Approval` component and an **Open** link; for a question a filled **Answer in chat** link; for failed a filled **Open** link; for review a filled **Open** link and **Mark seen**. Everything `NeedsYou` shows on a card today that still applies (a second reason from Gaffer, the worktree target's link) is kept. The `row` layout uses the same column widths as `WaitingItem`'s, with the pipeline column empty.

- [ ] **Step 1: Write the failing browser checks** in a new suite `web/tests/home-waiting.e2e.mjs` (routes copied from `web/tests/home-followups.e2e.mjs`), run in Task 4 when Home mounts the items: at 1440 the Factory row's dots column and the session row's buttons line up (the buttons' right edges are equal within 1 px across rows); at 390 each item is a card and `scrollWidth <= 390`.
- [ ] **Step 2: Implement** both components.
- [ ] **Step 3: Run** `npm run check`.
- [ ] **Step 4: Commit** "Home's waiting items: a row on a wide screen, a card on a narrow one".

### Task 4: Put Home together

**Files:**
- Modify: `web/src/lib/home/Home.svelte`
- Delete: `web/src/lib/home/FactoryStatus.svelte` if nothing else uses it (and `factoryRows` / `FactoryRow` in `gafferOutcome.ts` with their tests if then unused)
- Test: `web/tests/home-waiting.e2e.mjs`; update `web/tests/home.e2e.mjs`, `web/tests/home-followups.e2e.mjs` and any other suite that reads Home's "Factory pipelines" or "Needs you" (grep `web/tests`)

**Interfaces:**
- Consumes: Tasks 1 to 3; `factoryItems`, `ListSection`, `readSections` / `writeSections`, `ProjectPicker` from `web/src/lib/factory/`.
- Produces: Home, top to bottom, for an account that can start sessions:
  - Head: the title, one line of counts ("`n` waiting for you · `n` pipelines working · `n` sessions open", leaving out a part that is zero except the first), the plan meters, Give Gaffer an outcome (where the Factory is available) and New session. The summary strip and its Gaffer pill go; the "Factory" and "Projects" head buttons go (the tab bar has Factory; the rail has its own collapse control).
  - `ListSection` **Waiting for you**: `homeWaiting(items, needs)`, each entry a `WaitingItem` or `SessionWaitingItem`, `row` when not narrow, `card` when narrow, with `pipeline` `dots` on a row, `labelled` on a card from 600 px and `circles` below. Summary from `waitingSummary`. With no entries the section shows one line, "Nothing is waiting for you.", and stays open. Factory items act through a mounted `PipelineChoices`; **Details** goes to `/factory?pipeline=<item.id>`. Today's "Show n more" paging of Needs you is kept for the merged list (10 at first, 10 more each time).
  - `ListSection` **Factory, working without you** (only when there is a working pipeline): one line per pipeline (title, objective, dots with caption), each a link to `/factory?pipeline=<item.id>`, and a link "Open the Factory". Closed the first time.
  - The view tabs and the chosen view, unchanged. A finished session that has not been seen is marked "not seen yet" in Continue working and on the board's card (use what those components already know: `changedSinceSeen`).
  - `ListSection` **Your machines**, closed the first time, with today's summary and content.
  - First-time open state `{ waiting: true }`, remembered under `reet.home.sections.<userId>`.
  - `onattention` is called with `homeWaiting(...).length` and the session cards among them, so the bell and the title follow the section.
  - Narrow: no rail (as today) and a `ProjectPicker` under the head that sets Home's existing `scope`. The scope narrows the waiting list and the working section as it narrows the views.
- An account that cannot start sessions sees what it sees today.

- [ ] **Step 1: Write the failing checks** in `home-waiting.e2e.mjs`, with a fixture holding: one Factory decision with Gaffer's advice, one Factory pipeline whose session holds a permission, one pipeline ready to merge, one working pipeline, one session with a question, one finished unseen session, one failed session:
  - the section "Waiting for you" lists five entries in oldest-first order, and its header reads the `waitingSummary` string for them;
  - the finished session is not in it and its row in Continue working says "not seen yet";
  - the permission pipeline appears once, with Allow and Deny;
  - pressing "Move to next stage" on the decision opens the same confirmation as the Factory page, and confirming posts to the pipeline's `/advance` route once and the entry leaves the list;
  - the document title starts with "(5)" (or whatever `titleWithAttention` gives for 5);
  - "Factory, working without you" is closed, names the working pipeline, and after a click on its header and a reload it is open;
  - both views still switch, and the choice survives a reload;
  - a fixture with no outcomes and no session cards shows "Nothing is waiting for you." and no Factory section;
  - at 834 and 390 the items are cards, the Projects button narrows the list, and `scrollWidth` does not exceed the width.
- [ ] **Step 2: Run.** Expected: fail.
- [ ] **Step 3: Recompose `Home.svelte`.** Keep its loaders, handlers and dialogs.
- [ ] **Step 4: Update the existing Home suites** to the new structure. An assertion that no longer applies is replaced by the check for the new behaviour, never only deleted; list each in the report.
- [ ] **Step 5: Run** `npm run check`, `home-waiting`, `home`, `home-followups`, the phone suites that open Home (find them in `.github/workflows/`), and the five Factory suites from Task 2. Add `home-waiting` to CI beside `home-followups`.
- [ ] **Step 6: Commit** "Home opens on one list of what is waiting".

### Task 5: The Gaffer button floats, and what #825 left

**Files:**
- Modify: `web/src/lib/factory/Factory.svelte`, `web/src/lib/factory/PipelineDetail.svelte`, `web/src/lib/floatingLauncher.ts`, `web/src/lib/GafferChatWidget.svelte`, `web/src/lib/shell/Approval.svelte`
- Test: `web/tests/factory-pieces.e2e.mjs`, `web/tests/home-waiting.e2e.mjs`

**Interfaces:**
- The reserved "dock" strips #825 added on the Factory page (the foot of a narrow list, the last 80 px of the wide page, the end of the action bar's first line on a narrow detail) are removed, with the `dock` case in `floatingLauncher.ts` and its use in `GafferChatWidget.svelte`, unless another page uses it (none should). The button floats as on every other page. Filled buttons, the action bar and each item's button row carry `data-floating-clearance`, the app's existing way of moving the button off a control, on both pages. The narrow detail's action bar stays flush with the bottom of the window (above the phone tab bar), with no blank band.
- From #825's known list:
  - After archiving or deleting from a narrow detail, one Back press leaves the page (no duplicate `/factory` history entry).
  - Allow once and Deny are at least 44 px tall under `pointer: coarse` wherever `Approval` renders (the rule in `Approval.svelte` itself).
  - A detail row left open closes when another pipeline is chosen (the detail's rows reset with `item.id`).

- [ ] **Step 1: Write the failing checks:** in `factory-pieces`: at 390 and 834 no element with class `rest` or `data-floating-dock` (use whatever marks the strip today) exists and the list's last card can be scrolled fully into view above the tab bar; at 834 after archive, one `page.goBack()` leaves `/factory`; Allow's box is at least 44 px tall in a touch context; open the History row, choose another pipeline, History is closed. In `home-waiting`: at 390 the floating button's box does not intersect any filled button's box in the first two items.
- [ ] **Step 2: Run.** Expected: fail.
- [ ] **Step 3: Implement.** Replace #825's overlap test (a 7 × 7 grid over the strip) with the control-overlap check above.
- [ ] **Step 4: Run** `npm run check`, `factory-pieces`, `home-waiting`, and the suites that cover the floating button (grep `web/tests` for `floating`).
- [ ] **Step 5: Commit** "Let the Gaffer button float on the Factory page and Home, and tidy what the rebuild left".

### Task 6: Captures, docs and notes

**Files:**
- Modify: `docs/ui-verification.md`, `CHANGELOG.md` (Unreleased), `handoff.md`, the user docs that describe Home (find them: `docs/index.md`, `docs/session-board.md`, `docs/gaffer-ai.md`, `docs/factory.md`, `README.md`; read each against the new page), and the docs pictures of Home if `scripts/capture-docs-workspace.mjs` makes any
- Create: captures `docs/images/ui/home-rework-<width>-<theme>.png`; retake `docs/images/ui/factory-rework-*.png` (the strip is gone)

- [ ] **Step 1:** `home-waiting.e2e.mjs` saves, when `REET_UI_SCREENSHOTS` is set, at `deviceScaleFactor: 2` and `fullPage: false`: 1440 light (Continue working, Waiting for you open), 1440 dark (Planning board, sections closed), 834 light, 390 dark. `factory-pieces` retakes its five.
- [ ] **Step 2:** Look at every capture. Fix anything misaligned, clipped, overlapping or cramped before going on, and retake.
- [ ] **Step 3:** Write the `docs/ui-verification.md` section, the CHANGELOG entry (for a person running reet, no internal names), the handoff section HOME-REWORK-1 (what was built, where each thing from the old Home went, the owner's decisions, what was not done, what was not checked), and bring the user docs up to date. Retake any docs pictures of Home with the repository's script.
- [ ] **Step 4:** Run `npm run check`, every suite from Tasks 4 and 5, `mkdocs build --strict` and `scripts/check-docs.py` if they can run here.
- [ ] **Step 5: Commit** "Record what the Home rework was checked against".
