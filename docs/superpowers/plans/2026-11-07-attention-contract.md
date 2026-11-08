# Attention Contract Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every count and badge for "needs you" (Home, bell, tab title, workspace sidebar) uses one rule, a Gaffer goal for a session that already has a card is counted once, and answering an approval refreshes every surface immediately.

**Architecture:** A pure rule module (`attention.ts`) decides whether a session needs the person. `needsYou()` (cards) and `panelState.ts` (sidebar) both call it, so they cannot disagree. A small fold function merges Gaffer goals into cards by session id. A tiny pub/sub (`attentionSignal.ts`) makes the bell's loader and the workspace list refresh as soon as an answer or mark-seen happens. Web only: no API, protocol or server change.

**Tech Stack:** Svelte 5 + TypeScript (`web/src/lib`), Node unit tests (`*.test.mjs`, listed in `web/package.json` `test` script), Playwright browser scripts (`web/tests/*.e2e.mjs`, listed in `.github/workflows/`).

**Spec:** `docs/superpowers/specs/2026-11-07-attention-contract-design.md` (currently in the scratchpad until the repo branch is free).

**Deviation from spec section 5 (for the reviewer to confirm):** the spec sketched "one shared store, one poll". The workspace sidebar's list is that page's own data with stale-poll fencing and #642 holds, so this plan keeps each page's list and poll and adds a refresh signal instead. The spec text was updated to match.

## Global Constraints

- No API, protocol, Go or server change. Counts stay **one card per session**; bell count = Home card count = tab-title number.
- 24-hour window for unseen failed/finished is unchanged (`FINISHED_KEPT_MS` = one day in `web/src/lib/home.ts`); approval is shown before question.
- Blocked rule: `attention === 'waiting'` counts if the session has a pending item (`pendingPermission.permissionId` or `pendingQuestion.questionId`) regardless of `changedSinceSeen`; otherwise only while `changedSinceSeen`. Failed/finished count only while `changedSinceSeen` and within the window.
- Preserve: #642 confirmed-end dismissal, pending/uncertain/lost holds, newer selection and explicit history reopening; #643 direct Followed sessions entry with no implicit grants or model writes; same-process Chat/Terminal switching.
- The only visible text added is one muted reason line on a Needs-you card for a folded Gaffer goal; the Gaffer dialog's "Needs you" label is renamed "Needs review". No other UI or layout change.
- Every new `*.test.mjs` is added to the `test` script in `web/package.json`; every new or changed browser script is listed in `.github/workflows/terminal-browser.yml` (or the workflow that already lists it).
- Never commit `node_modules`.

## Review Focus

- A terminal bell the person has opened (no pending item) must still clear, as today.
- A chat with a pending approval that the person has opened must stay counted on Home, bell, title and sidebar until answered.
- A Gaffer goal whose session is not in the list (archived, past the 100-session cap, missing) must stay in the Gaffer count, never throw, never vanish.
- A viewer or account without Gaffer access (goals undefined) must see unchanged counts and no error.
- Answering the last pending approval must drop the count on every surface at once; the optimistic mark-seen must not bring it back, and a failed list read must keep the last good count.

---

### Task 1: The rule module

**Files:**
- Create: `web/src/lib/attention.ts`
- Create: `web/src/lib/attention.test.mjs`
- Modify: `web/package.json` (add `node src/lib/attention.test.mjs &&` to the `test` script, next to `home.test.mjs`)
- Modify: `web/src/lib/home.ts:67` (`export const FINISHED_KEPT_MS = DAY` becomes an alias of `ATTENTION_WINDOW_MS`)

**Interfaces:**
- Produces (all in `attention.ts`, importing only types from `./terminal/api`):
  - `export const ATTENTION_WINDOW_MS: number` (24 hours in ms)
  - `export function hasPending(s: Pick<TerminalSession, 'pendingPermission' | 'pendingQuestion'>): boolean`
  - `export function isBlocked(s: Pick<TerminalSession, 'attention' | 'changedSinceSeen' | 'pendingPermission' | 'pendingQuestion'>): boolean`
  - `export type AttentionRule = 'waiting' | 'failed' | 'finished'`
  - `export function attentionRuleOf(s: Pick<TerminalSession, 'attention' | 'changedSinceSeen' | 'updatedAt' | 'pendingPermission' | 'pendingQuestion'>, now: number): AttentionRule | null`

- [ ] **Step 1: Write the failing tests** in `attention.test.mjs` (follow `home.test.mjs` for how it imports TS): `isBlocked`: waiting+unseen → true; waiting+seen+pendingPermission → true; waiting+seen+pendingQuestion → true; waiting+seen+no pending → false; working+pending → false. `attentionRuleOf`: blocked → `'waiting'`; failed unseen at `now - 1h` → `'failed'`; finished unseen at `now - 25h` → `null`; failed seen → `null`; no attention → `null`; approval-only pending with `attention: 'waiting'`, seen → `'waiting'`.
- [ ] **Step 2: Run** `node web/src/lib/attention.test.mjs` — expect FAIL (module missing).
- [ ] **Step 3: Implement** the four functions and constant above; `home.ts` imports the constant.
- [ ] **Step 4: Run** `node web/src/lib/attention.test.mjs && node web/src/lib/home.test.mjs` — expect PASS.
- [ ] **Step 5: Commit** — `git add web && git commit -m "Add the single attention rule"`.

---

### Task 2: Cards use the rule; blocked cards survive mark-seen

**Files:**
- Modify: `web/src/lib/home.ts` (`NeedCard` type ~line 45; `needsYou` ~line 329)
- Modify: `web/src/lib/home/Home.svelte:100-103` (optimistic `markSeen`)
- Test: `web/src/lib/home.test.mjs`

**Interfaces:**
- Consumes: Task 1 `attentionRuleOf`, `hasPending`.
- Produces: `export type NeedReason = 'approval' | 'question' | 'failed' | 'finished' | 'review' | 'gaffer'`; `NeedCard` gains `reasons: NeedReason[]` (waiting card: `['approval']`, `['question']`, or both approval-first; failed → `['failed']`; finished → `['finished']`; worktree finished → `['review']`).

- [ ] **Step 1: Write the failing tests** in `home.test.mjs`: (a) a waiting chat with `pendingPermission.permissionId`, `changedSinceSeen: false` produces a `waiting` card with `approval` and `reasons: ['approval']`; (b) a waiting terminal bell with `changedSinceSeen: false` and no pending produces no card; (c) approval and question both pending → `reasons: ['approval','question']`; (d) existing cases (24h window, seen failed excluded, ordering by recency) unchanged; (e) a Home-style optimistic update `{...s, changedSinceSeen:false}` on a pending-approval chat still yields its card.
- [ ] **Step 2: Run** `node web/src/lib/home.test.mjs` — expect FAIL on (a), (c), (e).
- [ ] **Step 3: Implement**: replace the inline waiting/failed/finished conditions in `needsYou` with `attentionRuleOf(s, now)`; fill `reasons`. In `Home.svelte` `markSeen`, keep `changedSinceSeen: false` but only rewrite `attention` from `'waiting'` to `'working'` when `!hasPending(s)`.
- [ ] **Step 4: Run** `npm --prefix web run check` — expect all unit suites pass and svelte-check reports 0 errors/0 warnings.
- [ ] **Step 5: Commit** — `git add web && git commit -m "Count a blocked chat until it is answered"`.

---

### Task 3: The sidebar uses the same rule

**Files:**
- Modify: `web/src/lib/terminal/panelState.ts:15-56` (`needsInput`, `statusLine`, `needCount`)
- Test: `web/src/lib/terminal/panelState.test.mjs`

**Interfaces:**
- Consumes: Task 1 `isBlocked`, `attentionRuleOf`.
- Produces: `needsInput(session)` returns `'approval' | 'question' | null` using `isBlocked`; `needCount(sessions, now = Date.now()): number` counts sessions where `attentionRuleOf(s, now) !== null`. `statusLine` unchanged in shape. `SessionSidebar.svelte` calls (`needCount(projectSessions(p))`, `needCount(matches)`) keep working with the default `now`.

- [ ] **Step 1: Write the failing tests** in `panelState.test.mjs`: a seen waiting chat with a pending approval → `needsInput` = `'approval'` and `statusLine` = `Needs you · approval`; a seen bell → `null`; `needCount` over `[blocked-seen chat, unseen failed 1h old, unseen finished 25h old, seen failed, working]` with a fixed `now` = 2; existing approval-before-question and count tests unchanged.
- [ ] **Step 2: Run** `node web/src/lib/terminal/panelState.test.mjs` — expect FAIL.
- [ ] **Step 3: Implement** the three functions to delegate to `attention.ts`.
- [ ] **Step 4: Run** `npm --prefix web run check` — expect PASS, 0 diagnostics.
- [ ] **Step 5: Commit** — `git add web && git commit -m "Make the sidebar badge follow the shared rule"`.

---

### Task 4: Count a Gaffer goal once

**Files:**
- Modify: `web/src/lib/home.ts` (add `foldGafferGoals`)
- Modify: `web/src/lib/home/Home.svelte` (`followupsNeedReview` at ~line 85; the `needs` derivation at ~line 98)
- Modify: `web/src/lib/home/NeedsYou.svelte` (one muted reason line)
- Modify: `web/src/lib/GafferFollowedSessions.svelte:22,53,55` (label "Needs you" → "Needs review"; keep the `'attention'` filter value)
- Test: `web/src/lib/home.test.mjs`; browser: `web/tests/home-followups.e2e.mjs`, `web/tests/gaffer-followed.e2e.mjs`

**Interfaces:**
- Consumes: `NeedCard`, `NeedReason` (Task 2); `GafferGoalOverview` (`web/src/lib/api.ts:282`: `{ goal, session?: { id }, attention: boolean, ... }`).
- Produces: `export function foldGafferGoals(cards: NeedCard[], goals: readonly GafferGoalOverview[] | undefined): { cards: NeedCard[]; unfolded: number }`. A goal with `attention` true and `session.id` matching a card's `sessionId` appends `'gaffer'` to that card's `reasons` (once) and is not counted in `unfolded`; every other `attention` goal counts in `unfolded`. `goals` undefined → `{ cards, unfolded: 0 }`. Input cards are not mutated.

- [ ] **Step 1: Write the failing tests** in `home.test.mjs`: goal for a carded session → one card with `reasons` ending in `'gaffer'`, `unfolded` 0; goal with no matching card → `unfolded` 1, cards unchanged; two goals for one session → one `'gaffer'` reason; goal with `attention: false` ignored; `undefined` goals; goal whose `session` is missing.
- [ ] **Step 2: Run** `node web/src/lib/home.test.mjs` — expect FAIL.
- [ ] **Step 3: Implement** `foldGafferGoals`; in `Home.svelte`, `needs` = folded cards, `followupsNeedReview` = `unfolded`; the Home header count stays `needs.length`. In `NeedsYou.svelte`, when `card.reasons` includes `'gaffer'`, render one muted line "Gaffer follow-up also needs review" using an existing text style. Rename the dialog label as listed.
- [ ] **Step 4: Update the two Gaffer browser scripts** for the new label and the folded count, and add one assertion in `home-followups.e2e.mjs`: a goal on a session that has an approval card shows that card with the Gaffer line and the Gaffer button count excludes it.
- [ ] **Step 5: Run** `npm --prefix web run check && npm --prefix web run build`, then serve the preview (`npm --prefix web run preview -- --host 127.0.0.1 --port 4173 &`) and run `REET_UI_URL=http://127.0.0.1:4173 PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/usr/bin/google-chrome node web/tests/home-followups.e2e.mjs` and `.../gaffer-followed.e2e.mjs` — expect PASS.
- [ ] **Step 6: Commit** — `git add web && git commit -m "Count a Gaffer goal once with its session"`.

---

### Task 5: Refresh every surface when something is answered

**Files:**
- Create: `web/src/lib/attentionSignal.ts`, `web/src/lib/attentionSignal.test.mjs`
- Modify: `web/package.json` (add the new test to the `test` script)
- Modify: `web/src/lib/shell/Approval.svelte:~22`, `web/src/lib/terminal/ChatPane.svelte:~527` and the question-answer path next to it (`chatQuestionAnswer`), `web/src/lib/terminal/api.ts` `markSessionSeen` callers where a seen-mark is made (only call the signal from the UI code, not inside `api.ts`)
- Modify (subscribers): `web/src/App.svelte` (`loadAttention` at ~line 394), `web/src/lib/terminal/TerminalWorkspace.svelte` (its `refresh` at ~line 606)

**Interfaces:**
- Produces: `export function signalAttentionChanged(): void`; `export function onAttentionChanged(handler: () => void): () => void` (returns an unsubscribe function; handlers run synchronously, a throwing handler does not stop the others; a duplicate subscription of the same function is called once).

- [ ] **Step 1: Write the failing test** in `attentionSignal.test.mjs`: handler called once per signal; unsubscribe stops calls; a throwing handler does not prevent a second handler; two signals call twice.
- [ ] **Step 2: Run** `node web/src/lib/attentionSignal.test.mjs` — expect FAIL.
- [ ] **Step 3: Implement** the module (a module-level `Set`). Call `signalAttentionChanged()` after a successful `chatAnswer`/`chatQuestionAnswer` in `ChatPane.svelte` and `Approval.svelte`, and after a successful mark-seen in Home, the Sessions list and `App.svelte`'s `onMarkSeen`. Subscribe in `App.svelte` (call `loadAttention`) and `TerminalWorkspace.svelte` (call its list refresh), unsubscribing on teardown. Do not signal on failure.
- [ ] **Step 4: Run** `npm --prefix web run check` — expect PASS, 0 diagnostics.
- [ ] **Step 5: Commit** — `git add web && git commit -m "Refresh attention at once after an answer"`.

---

### Task 6: Browser acceptance and handoff

**Files:**
- Create: `web/tests/attention-contract.e2e.mjs`; list it in `.github/workflows/terminal-browser.yml` next to `compact-work-items.e2e.mjs` (line ~80)
- Modify: `web/tests/terminal-organisation.e2e.mjs` (sidebar badge expectation, ~lines 268-304), `web/tests/session-end-gaffer.e2e.mjs`, `web/tests/terminal-lifecycle.e2e.mjs`
- Create: `docs/handoffs/attention-contract.md`

- [ ] **Step 1: Write `attention-contract.e2e.mjs`** using the mocked-controller style of `home.e2e.mjs`/`compact-work-items.e2e.mjs`. Seed: one chat with a pending approval (seen), one unseen failed session 1h old, one unseen finished 25h old, one seen bell. Assert on Home: header and "Needs you" count = bell count = title prefix = 2; the same count on `/sessions` and on another page (as `navigation.e2e.mjs` does); the workspace sidebar badge = 2. Open the blocked chat (mark seen): count stays 2. Answer the approval from the bell: count drops to 1 on every surface without waiting 15 seconds (use the page's fake clock or assert within 2 seconds). Fail the list read once: the count keeps its last value.
- [ ] **Step 2: Run it against a built preview** (as in Task 4 Step 5) — expect PASS now that Tasks 1-5 are in. If it fails, decide whether the script or the product is wrong before changing anything; do not weaken an assertion to get green.
- [ ] **Step 3: Update the existing scripts.** `terminal-organisation.e2e.mjs`: the sidebar badge now also counts unseen failed/finished. `session-end-gaffer.e2e.mjs` and `terminal-lifecycle.e2e.mjs`: add attention-count assertions across confirmed-end dismissal, pending/uncertain/lost holds and explicit history reopening, and one case for a user-ended non-chat terminal: it must not raise a failed or finished card after dismissal. If that case fails, report it as a finding with the code that produces it (do not widen the change silently).
- [ ] **Step 4: Run** `home.e2e.mjs`, `compact-work-items.e2e.mjs`, `navigation.e2e.mjs`, `terminal-organisation.e2e.mjs`, `terminal-lifecycle.e2e.mjs`, `session-end-gaffer.e2e.mjs`, `attention-contract.e2e.mjs` — expect all PASS.
- [ ] **Step 5: Write `docs/handoffs/attention-contract.md`** (under ~30 lines): the rule, the card/reasons identity, the Gaffer fold, the refresh signal, behaviour changes, non-goals (server-owned items, the 100-session cap, Gaffer lag), links to the spec and plan, and that deployment is separate and unverified.
- [ ] **Step 6: Commit** — `git add web docs .github && git commit -m "Pin the attention contract in the browser and record it"`.

---

## Self-review

- **Spec coverage:** identity and reasons (Task 2); one derivation incl. sidebar (Tasks 1-3); blocked-until-resolved with the bell exception (Tasks 1-2); Gaffer fold and label (Task 4); refresh without staleness (Task 5); behaviour changes, preserved tests, new session-end/lifecycle assertions, workflow listing, handoff (Task 6). The spec's single-store wording is the one deliberate deviation, stated at the top.
- **Types:** `attentionRuleOf`, `isBlocked`, `hasPending`, `NeedReason`, `NeedCard.reasons`, `foldGafferGoals`, `signalAttentionChanged`, `onAttentionChanged` are named identically wherever used.
- **Proportion:** six tasks, tests named with assertions, bodies left to the implementer except the rule itself.
