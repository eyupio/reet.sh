# Session Overview Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the session workspace Overview show what was asked, how far the work has got (Code, PR, Checks, Review, Merge) and what to do next, with a prompt dialog, run clock, PR card and link, copy and prefill actions.

**Architecture:** Three pull requests in order. Part A stores and serves the opening prompt on the controller. Part B extends the machine's read-only GitHub read. Part C rebuilds the Overview as small Svelte components fed by pure, unit-tested modules. Part C degrades cleanly when A or B is not deployed.

**Tech Stack:** Go (store, API, terminalproto, terminalhost), Svelte 5 + TypeScript, `node:test` unit tests, Playwright e2e against `vite preview`.

**Spec:** `docs/superpowers/specs/2026-11-05-session-overview-design.md` (read it first; this plan cites its sections).

## Global Constraints

- Prompt cap is `terminalproto.MaxPromptBytes` (8000 bytes); the session list carries a 300-character excerpt only.
- Inspection stays read-only and github.com only. Every link is `https:` and goes through `safeInsightURL`. No new library.
- The composer textarea has `maxlength="2000"`: a prefilled message plus the existing draft must fit, else copy instead.
- Copy follows `docs/voice-and-copy.md` (British spelling, familiar state labels, no cheerful reassurance around failures).
- Overview blocks that change get fixed-height slots; 44 px targets on `pointer: coarse`; design tokens only, both themes.
- Go: `go test ./... -count=1` and `go vet ./...`. Web unit tests are hand-listed in `web/package.json` `scripts.test`: append each new `*.test.mjs` as `&& node <path>`. Also `cd web && npm run check`.
- e2e: `cd web && npm run build && npx vite preview --port 4173 &`, then `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/opt/pw-browsers/chromium node tests/<name>.e2e.mjs`.
- Pure modules use `import type` only; a runtime import from a neighbour is rewritten inside the test the way `web/src/lib/agentLaunch.test.mjs` does it.
- Test names are sentences about behaviour. Commit messages are imperative sentence-case prose with no prefix.
- Each part updates its docs and adds its `handoff.md` entry (`OVERVIEW-1`, `-2`, `-3`) in the same pull request. The pull request says which tests were seen failing with the rule removed.

## Review Focus

Inputs the spec implies but its table of tasks would not otherwise test. Each line is pinned by a test in the named task.

1. A prompt cut at 300 characters through a multibyte character (emoji) must stay valid text (Task 1).
2. A PR with no reported checks and everything else green must not show Checks as done or Merge as ready (Task 8).
3. `completedAt` earlier than `createdAt` (clock skew) must never print a negative or absurd length (Task 5).
4. A person's half-typed draft near the 2,000-character limit must not be overwritten or truncated by a prefill (Tasks 7, 13).
5. A prompt fetch that resolves after the person switched sessions must never show the other session's prompt (Task 10).

---

## Part A: Controller keeps and serves the opening prompt (PR 1)

### Task 1: The store keeps the prompt and clears it on archive

**Files:**
- Modify: `internal/store/store_terminal.go` (column list near line 49; `TerminalSession` struct near 317 and its comment near 332; `terminalSessionCols` and `readTerminal` near 393-400; the `INSERT` in `ReserveTerminalCreateFor` near 585; `ArchiveTerminalFor` near 488; `archiveTerminalOnClose` near 750)
- Test: `internal/store/store_terminal_test.go`

**Interfaces:**
- Produces: `TerminalSession.PromptExcerpt string` (`json:"promptExcerpt,omitempty"`), `TerminalSession.PromptMore bool` (`json:"promptMore,omitempty"`), and `func (s *Store) TerminalPromptFor(uid int64, sessionID string) (string, error)`, which returns `terminalproto.ErrInvalid` for a malformed id and `ErrRemoteNotFound` for an unknown session or another account's.

- [ ] **Step 1: Write the failing tests** (reuse the file's existing reserve and archive helpers):
  - `TestReserveStoresTheOpeningPromptAndTheListCarriesOnlyAnExcerpt`: a 1,000-character prompt gives `PromptExcerpt` equal to its first 300 characters, `PromptMore` true, and `TerminalPromptFor` returns all 1,000; a 120-character prompt gives `PromptMore` false.
  - `TestAnExcerptNeverSplitsAMultibyteCharacter`: 299 `a` then two emoji; `utf8.ValidString(excerpt)` and `utf8.RuneCountInString(excerpt) == 300`.
  - `TestARetriedCreateKeepsTheFirstPrompt`: same request id, different prompt; `TerminalPromptFor` returns the first.
  - `TestArchivingClearsThePromptOnBothPaths`: one session through `ArchiveTerminalFor`, one ended with history removal and observed closed (the `archiveTerminalOnClose` path); both give `""` from `TerminalPromptFor` and an empty `PromptExcerpt`.
  - `TestAnotherAccountCannotReadAPrompt`: `errors.Is(err, ErrRemoteNotFound)`.
  - `TestAnOlderDatabaseGainsThePromptColumn`: a database whose `terminal_sessions` lacks `prompt` opens, `hasColumn` is true, old rows read `""`.
- [ ] **Step 2: Run to verify failure:** `go test ./internal/store/ -run 'Prompt|Excerpt|Archiving|OlderDatabase' -count=1`. Expected: FAIL (undefined fields and method).
- [ ] **Step 3: Implement.** Append `{"terminal_sessions", "prompt TEXT NOT NULL DEFAULT ''"}` to the column list; add `prompt` and `in.Prompt` to the reserve `INSERT`; append `substr(prompt,1,300),length(prompt)>300` to `terminalSessionCols` after `agent_mode` and scan into the two fields (SQLite counts characters for both). `TerminalPromptFor` uses the same ownership and enabled-account conditions as `TerminalSessionFor`. Add `clearArchivedPrompt(tx *sql.Tx, uid int64, sessionID string) error` that runs `UPDATE terminal_sessions SET prompt='' WHERE id=? AND user_id=? AND EXISTS(SELECT 1 FROM terminal_session_archives a WHERE a.session_id=terminal_sessions.id)` and call it straight after each archive `INSERT`. Replace the "deliberately absent" comment with the new rule.
- [ ] **Step 4: Find the account-deletion path** (`grep -n "DELETE FROM users" internal/store/*.go`) and add `TestDeletingAnAccountRemovesItsSessionRowsAndPrompts`. If session rows survive account deletion, clear `prompt` in that path too and say so in the pull request.
- [ ] **Step 5: Run** `go test ./internal/store/ -count=1 && go vet ./internal/store/`. Expected: PASS.
- [ ] **Step 6: Commit:** `Keep a capped copy of the opening prompt and clear it when the session is archived`.

### Task 2: The prompt endpoint, the audit check and the docs

**Files:**
- Create: `internal/api/terminal_prompt.go`, `internal/api/terminal_prompt_test.go`
- Modify: `internal/api/terminal.go` (route beside line 46), `docs/terminal-launch.md` (a "What is kept" paragraph after the `prompt` row), the docs that list terminal routes (`grep -rln "sessions/{id}/inspect" docs`), `handoff.md` (entry `OVERVIEW-1`)

**Interfaces:**
- Consumes: `Store.TerminalPromptFor` (Task 1).
- Produces: `GET /api/v2/terminal/sessions/{id}/prompt` answering `200 {"prompt": string}` with `Cache-Control: no-store`; `404 "session not found"` for an unknown or another account's session; `400` for a malformed id.

- [ ] **Step 1: Write the failing tests:** `TestPromptEndpointAnswersTheOwnerOnly` (owner 200 with the text and the header; other account 404; unknown id 404; archived session 200 with `""`) and `TestLaunchAuditNeverCarriesThePrompt` (create with prompt `secret-marker-7f3`, then no audit row contains that string).
- [ ] **Step 2: Run:** `go test ./internal/api/ -run 'PromptEndpoint|LaunchAuditNever' -count=1`. Expected: FAIL (404 route, then missing handler).
- [ ] **Step 3: Implement** `func (s *server) terminalPrompt(w http.ResponseWriter, r *http.Request)` through `s.terminal(...)`, mapping `store.ErrRemoteNotFound` to 404 and `terminalproto.ErrInvalid` to 400 as the neighbouring routes do (`terminal.go` near line 193). The text is never logged. If the audit test fails, fix `auditTerminalLaunch` so it records no prompt.
- [ ] **Step 4: Docs.** State in `docs/terminal-launch.md` that the opening prompt is kept with the session, capped at 8,000 bytes, shown only to its owner and cleared when the session is archived. Run `grep -rniE "prompt.*(not kept|never stored|not stored|kept only)" README.md docs/*.md` and correct any hit. Add the `OVERVIEW-1` entry.
- [ ] **Step 5: Run** `go test ./... -count=1 && go vet ./...`. Expected: PASS.
- [ ] **Step 6: Commit and open the draft pull request for Part A:** `Serve a session's stored opening prompt to its owner`.

---

## Part B: Machine extends the GitHub read (PR 2)

### Task 3: Wire types and their bounds

**Files:**
- Modify: `internal/terminalproto/inspect.go` (`InspectPull` near line 70, `InspectResult` near 83, `valid()` near 103)
- Test: `internal/terminalproto/inspect_test.go` (follow `TestInspectProtocolBoundaries`)

**Interfaces:**
- Produces (all optional, additive; `InspectFeature` stays `session-inspect-v1`):

```go
type InspectReviews struct{ Approved, ChangesRequested, Commented, Requested int } // json: approved, changesRequested, commented, requested
type InspectThreads struct{ Open, Resolved int; More bool }                         // json: open, resolved, more (omitempty)
// InspectPull gains: Base, Head string; CreatedAt, MergedAt, ClosedAt, LastPushAt int64 (ms);
//   AutoMerge bool; Reviews *InspectReviews; Threads *InspectThreads; DetailKnown bool `json:"detailKnown,omitempty"`
// InspectResult gains: Remote string `json:"remote,omitempty"` // "owner/name"
```

- [ ] **Step 1: Write the failing test** `TestInspectPullDetailBoundaries`: a fully populated valid result passes `valid()`; each of these is refused: a negative count, a count above 10,000, `Base` or `Head` over 255 bytes or with a control character, a timestamp refused by `validAttentionTime`, `Remote` not matching `^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$` or over 140 bytes. Also `TestAnOlderMachinesPullReadsAsDetailNotKnown`: the existing payload shape decodes, is valid, and has `DetailKnown == false`. And a full 40-check result with every new field still fits the frame limit `fitInspect` enforces.
- [ ] **Step 2: Run:** `go test ./internal/terminalproto/ -run 'Inspect' -count=1`. Expected: FAIL (undefined fields).
- [ ] **Step 3: Implement** the types and the `valid()` checks above.
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Commit:** `Describe a pull request's reviews, threads and times on the inspect wire`.

### Task 4: The machine reads the extra fields

**Files:**
- Modify: `internal/terminalhost/inspect.go` (`inspectGitHub` near 375; where `InspectSession` near 23 sets `Branch` for the status view), `docs/terminal-workspace.md` (the GitHub paragraph), `handoff.md` (`OVERVIEW-2`)
- Test: `internal/terminalhost/inspect_test.go` (the fake `gh` script pattern in `TestInspectGitHubRemoteAndPriority`; make the script log each invocation to a file)

**Interfaces:**
- Consumes: Task 3 types.
- Produces: `func inspectThreads(ctx context.Context, dir, repo string, number int) (*terminalproto.InspectThreads, int64, error)` (threads and last commit time in ms) and `func inspectRemote(git func(...string) ([]byte, error)) string` (`"owner/name"`, or `""` for any origin `inspectGitHubRepo` refuses; strip its `github.com/` prefix).

- [ ] **Step 1: Write the failing tests:**
  - `TestAnOpenPullReadsReviewsThreadsAndLastPush`: view JSON with `latestReviews` (one `APPROVED`, one `CHANGES_REQUESTED`), one `reviewRequests`, non-null `autoMergeRequest`, `baseRefName`, `headRefName`; GraphQL JSON with 3 unresolved and 2 resolved threads and a `committedDate`. Expect `DetailKnown`, `Reviews{1,1,0,1}`, `Threads{Open:3,Resolved:2}`, `AutoMerge`, `LastPushAt`, `Base`, `Head`.
  - `TestAFailedThreadReadKeepsTheRest`: the GraphQL call exits 1; checks and review stay, `Threads == nil`, `DetailKnown` true.
  - `TestANullPullRequestFromGraphQLLeavesThreadsUnread`: `{"data":{"repository":{"pullRequest":null}}}` gives `Threads == nil` and no panic.
  - `TestAMergedPullRequestMakesNoThreadRead`: state `MERGED` logs exactly one `gh` call and sets `MergedAt`.
  - `TestMoreThanAHundredThreadsAreMarked`: `totalCount` 130 with 100 nodes sets `More`.
  - `TestRemoteIsReportedBeforeAnyPullRequestExists`: the status view reports `Remote == "o/r"` with `Pull == nil`; origin `https://evil.test/o/r` gives `""`.
- [ ] **Step 2: Run:** `go test ./internal/terminalhost/ -run 'OpenPull|FailedThread|NullPull|MergedPull|HundredThreads|RemoteIsReported' -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement.** Extend the `--json` list with `createdAt,mergedAt,closedAt,baseRefName,headRefName,autoMergeRequest,latestReviews,reviewRequests` and do **not** add `commits` (a long PR would overrun the 128 KiB output cap). `Reviews` counts `latestReviews[].state` (`APPROVED`, `CHANGES_REQUESTED`, `COMMENTED`; ignore the rest) and `Requested = len(reviewRequests)`. For open PRs only, call `gh api graphql` with a fixed query constant and `owner`, `name`, `number` passed as flag variables (never interpolated), with the first call's safeguards: absolute `gh` path, `GH_PROMPT_DISABLED=1`, `GH_HOST=github.com`, `NO_COLOR=1`, process-group kill, `WaitDelay`, a size-capped buffer. It reads `reviewThreads(first:100){totalCount nodes{isResolved}}` and the last commit's `committedDate`. Any failure leaves `Threads` nil; success of the first call sets `DetailKnown`. Set `Remote` in both the status and GitHub views.
- [ ] **Step 4: Docs.** In `docs/terminal-workspace.md`, add that review threads use one extra read-only GraphQL request for open pull requests and need the machine's `gh` sign-in to allow it. Add `OVERVIEW-2`.
- [ ] **Step 5: Run** `go test ./... -count=1 && go vet ./...`. Expected: PASS.
- [ ] **Step 6: Commit and open the draft pull request for Part B:** `Read review threads, merge times and the repository name for a session's pull request`.

---

## Part C: The Overview (PR 3)

### Task 5: Run clock and the session type fields

**Files:**
- Create: `web/src/lib/sessions/runClock.ts`, `web/src/lib/sessions/runClock.test.mjs`
- Modify: `web/src/lib/terminal/api.ts` (add `promptExcerpt?: string; promptMore?: boolean; completedAt?: number` to `TerminalSession`), `web/package.json`

**Interfaces:**
- Produces:

```ts
export function formatLength(ms: number): string // 'under a minute' | '7m' | '1h 12m' | '2d 3h'; negative or NaN => 'under a minute'
export function startLabel(createdAt: number, now: number, locale?: string): string // '14:02' today, else '4 Nov, 14:02'
export function runLine(s: Pick<TerminalSession, 'state' | 'createdAt' | 'completedAt'>, now: number, locale?: string): string
// live (creating|running|end_requested): 'Started 14:02 · running 1h 12m'; finished with completedAt: 'Ran 1h 12m'; finished without it: 'Started 14:02'
```

- [ ] **Step 1: Write the failing tests** (set `process.env.TZ = 'UTC'` first; pass locale `en-GB`): `formatLength` rows for 0, 59,999, 60,000 (`1m`), 3,600,000 (`1h 0m`), 4,320,000 (`1h 12m`), 183,600,000 (`2d 3h`), `-5`, `NaN`; `a session that finished before it started reads as under a minute, never negative` (`completedAt < createdAt`); a finished session without `completedAt` shows its start and no length; the three `runLine` shapes above.
- [ ] **Step 2: Run:** `node src/lib/sessions/runClock.test.mjs`. Expected: FAIL (module missing).
- [ ] **Step 3: Implement** with `Intl.DateTimeFormat` for the clock time; the panel adds no timer and follows the `now` that `SessionsPage` ticks.
- [ ] **Step 4: Run** the test, append it to `scripts.test`, run `npm run check`. Expected: PASS.
- [ ] **Step 5: Commit:** `Add the run clock and carry completedAt and the prompt excerpt on the session type`.

### Task 6: Extract the pull request verdict

**Files:**
- Create: `web/src/lib/sessions/pullVerdict.ts`, `web/src/lib/sessions/pullVerdict.test.mjs`
- Modify: `web/src/lib/sessions/GitHubPane.svelte` (use the module in place of its inline `verdict` and the failed, running and passed groupings), `web/package.json`

**Interfaces:**
- Produces:

```ts
export type Tone = 'bad' | 'warn' | 'good' | 'muted'
export type Verdict = { tone: Tone; icon: 'conflict' | 'clock' | 'check' | 'github'; title: string; detail: string }
export function splitChecks(checks: InsightCheck[]): { failed: InsightCheck[]; running: InsightCheck[]; passed: InsightCheck[] } // by checkTone
export function pullVerdict(pr: InsightPull): Verdict
```

- [ ] **Step 1: Write the failing characterisation test:** one case per branch of the current inline logic in `GitHubPane.svelte` (merged; closed; conflict alone, with failing checks, with changes requested; failing checks, with changes requested; changes requested; running; draft; review required; no checks; approved with checks passed; passed without a decision) asserting the exact current `title` and `tone`, copied from the component.
- [ ] **Step 2: Run:** `node src/lib/sessions/pullVerdict.test.mjs`. Expected: FAIL (module missing).
- [ ] **Step 3: Implement** by moving the logic verbatim (including `count` and `nameList`), then switch `GitHubPane.svelte` to import it.
- [ ] **Step 4: Run** the test, append it to `scripts.test`, `npm run check`, and `node tests/session-workspace.e2e.mjs` (its GitHub tab checks must still pass unchanged). Expected: PASS.
- [ ] **Step 5: Commit:** `Share the pull request verdict between the GitHub tab and the Overview`.

### Task 7: Fix messages, prefill rule and prompt source

**Files:**
- Create: `web/src/lib/sessions/fixMessages.ts`, `fixMessages.test.mjs`, `promptSource.ts`, `promptSource.test.mjs` (all in `web/src/lib/sessions/`)
- Modify: `web/package.json`

**Interfaces:**
- Produces:

```ts
export type FixKind = 'conflicts' | 'checks' | 'threads'
export function fixMessage(kind: FixKind, facts: { base?: string; checks?: string[]; threads?: number }): string
export function prefillText(draft: string, message: string, limit?: number): string | null // draft + '\n' + message; null when over limit (default 2000)
export type PromptView =
  | { kind: 'stored'; excerpt: string; more: boolean }
  | { kind: 'chat'; text: string }
  | { kind: 'none'; why: 'old' | 'unloaded' }
export function promptView(s: Pick<TerminalSession, 'promptExcerpt' | 'promptMore' | 'structured'>, chat?: ChatState): PromptView
```

- [ ] **Step 1: Write the failing tests.** `fixMessage`: `Resolve the merge conflicts with main on this branch, then push.` (default `the base branch`); checks are quoted, stripped of control characters and URLs, cut to 60 characters with `…`, limited to three with `(and N more)`; threads singular and plural (`There are 3 unresolved review threads on the pull request. Please read each one and address it.`); every output is at most 2,000 characters. `prefillText`: empty draft returns the message alone; an existing draft is kept and the message follows on a new line; a draft of 1,990 characters returns `null`. `promptView`: stored excerpt wins; else the first `user` item of a chat whose `truncated` is false; else `none` with `unloaded` for a truncated chat and `old` otherwise.
- [ ] **Step 2: Run both:** `node src/lib/sessions/fixMessages.test.mjs && node src/lib/sessions/promptSource.test.mjs`. Expected: FAIL.
- [ ] **Step 3: Implement** per the signatures.
- [ ] **Step 4: Run** both, append to `scripts.test`, `npm run check`. Expected: PASS.
- [ ] **Step 5: Commit:** `Add the prefilled fix messages and the rule for choosing a session's prompt`.

### Task 8: Stage derivation

**Files:**
- Create: `web/src/lib/sessions/overviewStages.ts`, `web/src/lib/sessions/overviewStages.test.mjs`
- Modify: `web/src/lib/sessionInsights.ts` (add to `InsightPull`: `base?: string; head?: string; createdAt?: number; mergedAt?: number; closedAt?: number; lastPushAt?: number` (times in ms), `autoMerge?: boolean`, `reviews?: { approved: number; changesRequested: number; commented: number; requested: number }`, `threads?: { open: number; resolved: number; more?: boolean }`, `detailKnown?: boolean`; add `remote?: string` to `SessionInsight`), `web/package.json`

**Interfaces:**
- Consumes: `Tone` (Task 6), `FixKind` (Task 7).
- Produces:

```ts
export type StepKey = 'code' | 'pr' | 'checks' | 'review' | 'merge'
export type StepState = 'done' | 'current' | 'attention' | 'upcoming' | 'unknown'
export type Step = { key: StepKey; label: string; state: StepState; detail: string } // label is 'Planning' for code while planning
export type NextAction = { label: string; kind: 'session' | 'link'; href?: string }
export type Next = { text: string; tone: Tone; primary?: NextAction; fix?: FixKind }
export type StageInput = { live: boolean; planning: boolean; needsInput: string; active: boolean; pull?: InsightPull; pullReason?: string }
export type Stages = { steps: Step[]; next: Next; merged?: { at?: number; url?: string } }
export function deriveStages(input: StageInput): Stages
```

- [ ] **Step 1: Write the failing table-driven tests** from spec section 4: one case per cell of its stage table, and per Next rule in priority order (input needed > conflicts > failing checks > changes requested > open threads > running checks > draft > review required > merged > nothing), each asserting `steps[].state`, `next.text`, `next.primary` and `next.fix`. Named cases that must exist: `a pull request with no reported checks never marks Checks done or Merge ready`; `threads not read on an older machine are unknown, never zero` (`detailKnown` absent); `a merged pull request collapses to a merged row`; `planning relabels the Code step and marks it current`; `github unavailable leaves PR, Checks, Review and Merge unknown with the reason`; `merge is never claimed from green checks alone` (text `Ready. GitHub decides whether it can merge.`).
- [ ] **Step 2: Run:** `node src/lib/sessions/overviewStages.test.mjs`. Expected: FAIL.
- [ ] **Step 3: Implement.** The input-needed Next shows `needsInput` verbatim with a `session` action labelled `Review`. Conflicts offer `fix: 'conflicts'` and an Open pull request link; failing checks offer `fix: 'checks'` and an Open failing check link (the first failing check's `safeInsightURL`); threads and changes requested offer `fix: 'threads'` only when `threads.open > 0`. The runtime import of `checkTone`/`safeInsightURL` is rewritten in the test as in `agentLaunch.test.mjs`.
- [ ] **Step 4: Run** the test, append to `scripts.test`, `npm run check`. Expected: PASS.
- [ ] **Step 5: Commit:** `Derive the Overview's five stages and its next step from facts only`.

### Task 9: Extract the Overview into its own component (no behaviour change)

**Files:**
- Create: `web/src/lib/sessions/OverviewTab.svelte`
- Modify: `web/src/lib/sessions/ContextPanel.svelte` (replace the `{#if tab === 'overview'}` block at lines 151-171 with `<OverviewTab />`; move the rules only the Overview uses out of its `<style>`)
- Test: the existing `web/tests/session-workspace.e2e.mjs` and `web/tests/sessions.e2e.mjs`

**Interfaces:**
- Produces: `OverviewTab` props `{ id, session, host, status, project, skills, now, chat, supported, workspace, pending, errors, baseline, newEvents, terminalAdvanced, onterminal, onpick }`. Display-only values (`headline`, `subline`, `countsLine`, `running`, `errorDetail`, `attention`, `sessionState`) are derived inside it. Polling, `read()`, the visit baseline and the non-Overview tabs stay in `ContextPanel`.

- [ ] **Step 1: Record the baseline.** Run both e2e files (expected PASS) and, with a temporary script, print the bounding-box height of each `.panel-body > section` in the existing fixture at 360 px in both themes. Do not commit the numbers.
- [ ] **Step 2: Move** the markup and its props. Scoped styles do not follow markup: move each rule only the Overview uses; where the Changes and Files tabs share a rule (`h3`, `h4`, `.quiet`, `.bad`, `.empty`, `.callout`), keep it in `ContextPanel` and re-declare the few lines in `OverviewTab`.
- [ ] **Step 3: Verify:** the same e2e files pass, the recorded heights are identical, and `npm run check` is clean.
- [ ] **Step 4: Commit:** `Move the Overview into its own component without changing what it shows`.

### Task 10: Prompt card and dialog

**Files:**
- Create: `web/src/lib/sessions/PromptCard.svelte`, `web/src/lib/sessions/PromptDialog.svelte`
- Modify: `web/src/lib/terminal/api.ts` (add `sessionPrompt`), `OverviewTab.svelte` (insert the card under the header), `web/tests/session-workspace.e2e.mjs`

**Interfaces:**
- Consumes: `promptView` (Task 7), `startLabel` (Task 5), `CopyButton.svelte` (`{ text, label?, copiedLabel?, compact?, primary?, oncopy?, ariaLabel? }`), `use:shell` from `dialogShell.ts`.
- Produces: `export async function sessionPrompt(id: string, signal?: AbortSignal): Promise<string>` (GET `/api/v2/terminal/sessions/${id}/prompt` through `terminalRequest`); `PromptCard` props `{ session, chat }`; `PromptDialog` props `{ session, text, onclose }`.

- [ ] **Step 1: Write the failing e2e checks** (extend the fixture with `promptExcerpt`, `promptMore`, and a held prompt route): the card shows a two-line quote and **Read full prompt**; the dialog opens with the full text, Escape closes it and focus returns to the trigger; **Copy prompt** puts the text on the clipboard and reads "Copied"; `a prompt fetch that resolves after switching sessions never shows the other session's prompt` (hold the fetch, switch to a second fixture session, release, assert the dialog is closed or shows nothing from the first); a fixture without `promptExcerpt` shows "This session started before reet kept opening prompts." with no console errors; a 404 from the prompt route shows the same "not kept" line; a failed fetch keeps the excerpt and shows "Couldn't load the full prompt. Try again." with a working retry.
- [ ] **Step 2: Run:** `node tests/session-workspace.e2e.mjs`. Expected: FAIL on the new checks.
- [ ] **Step 3: Implement.** The card sits in a fixed-height slot (two clamped lines) that also holds loading and "not kept". The dialog is a native `<dialog>` with `use:shell`, `aria-labelledby`, a scrolling `white-space: pre-wrap` body, chips for agent, model, effort and start time (and the auto-approve decision when `session.yolo`). Use one `AbortController` per open, aborted on close and when `session.id` changes, and compare the id before applying a result.
- [ ] **Step 4: Run** the e2e and `npm run check`. Expected: PASS.
- [ ] **Step 5: Commit:** `Show what the session was asked, with the full prompt one tap away`.

### Task 11: Run clock line, Where card and the GitHub tab dot

**Files:**
- Create: `web/src/lib/sessions/WhereCard.svelte`
- Modify: `OverviewTab.svelte` (the `runLine` under the subline; swap in `WhereCard`), `ContextPanel.svelte` (the tab dot), `web/tests/session-workspace.e2e.mjs`

**Interfaces:**
- Consumes: `runLine`, `startLabel` (Task 5), `pullVerdict` (Task 6), `CopyButton.svelte`.
- Produces: `WhereCard` props `{ session, host, status, project, branch, base, remote, now }`.

- [ ] **Step 1: Write the failing e2e checks:** a live session reads `Started HH:MM · running …`; a finished fixture with `completedAt` reads `Ran 1h 12m`; the repo links to `https://github.com/<remote>` only when `remote` matches `owner/name`, else shows `worktreeRepo` as text; **Copy** on branch and worktree path works; `Base → main` shows when `pull.base` is known; the GitHub tab shows a dot for failing, conflicting and merged fixtures with an accessible name such as "GitHub, needs attention", and none without a PR.
- [ ] **Step 2: Run** the e2e. Expected: FAIL on the new checks.
- [ ] **Step 3: Implement.** Worktree reads `Own worktree` or `Shared checkout` with the path middle-ellipsised (full path in `title`). The dot uses `pullVerdict(pull).tone` and never colour alone (the accessible name carries the state).
- [ ] **Step 4: Run** the e2e and `npm run check`. Expected: PASS.
- [ ] **Step 5: Commit:** `Show the run clock, a copyable Where card and a status dot on the GitHub tab`.

### Task 12: Stage tracker, Next line and pull request card

**Files:**
- Create: `web/src/lib/sessions/StageTracker.svelte`, `web/src/lib/sessions/PullCard.svelte`
- Modify: `ContextPanel.svelte` (read `github` on the Overview tab too: in `pick('overview')`, the 5-second timer and `visibility()`, change the `tab === 'github'` conditions to `tab === 'github' || tab === 'overview'`, keeping the 120 s cadence and the Live updates switch), `OverviewTab.svelte`, `web/tests/session-workspace.e2e.mjs`

**Interfaces:**
- Consumes: `deriveStages` (Task 8), `pullVerdict`, `splitChecks` (Task 6), `FixKind` (Task 7).
- Produces: `StageTracker` props `{ stages: Stages, onsession: () => void, onfix: (kind: FixKind) => void }`; `PullCard` props `{ pull: InsightPull, onfull: () => void }`.

- [ ] **Step 1: Write the failing e2e checks** with fixtures for conflict, failing, running, approved with no open threads, merged, an older machine (no `detailKnown`), `github_unavailable` and `no_pr`: assert each step's `data-state` and word, the Next text, that the current step has `aria-current="step"`, that links are `https:` with `rel="noopener noreferrer"`, the placeholder row when there is no PR, "not read on this machine" for threads on an older machine, the merged collapse row, and `with Live updates off the Overview makes no further GitHub reads`.
- [ ] **Step 2: Run** the e2e. Expected: FAIL.
- [ ] **Step 3: Implement.** The tracker is an `<ol>` of five steps with icon and word; the Next line has a 60 px slot, one primary button and the secondary fix button. `PullCard` holds a reserved height while loading and a 69 px placeholder row when there is no PR; it shows the verdict, the checks meter and chips, reviews, open threads, conflicts, `+/-` totals, last push, and buttons **Open pull request**, **Open failing check**, **Open review** and **Full detail** (switches to the GitHub tab).
- [ ] **Step 4: Run** the e2e and `npm run check`. Expected: PASS.
- [ ] **Step 5: Commit:** `Show where the work has got to and what to do next`.

### Task 13: Prefill the composer with a fix message

**Files:**
- Create: `web/src/lib/sessions/composerPrefill.ts`
- Modify: `web/src/lib/terminal/ChatPane.svelte` (one `$effect` beside the composer's `draft` state at line 177), `ContextPanel.svelte` / `OverviewTab.svelte` (the `onfix` handler), `web/tests/session-workspace.e2e.mjs`

**Interfaces:**
- Produces: `export const composerPrefill: Writable<{ sessionId: string; text: string; nonce: number } | null>`. The handler builds the text with `fixMessage` and `prefillText`; in a chat it publishes to the store, otherwise (or when `prefillText` returns `null`) it copies the message with `copyText` from `web/src/lib/clipboard.ts` and says so.

- [ ] **Step 1: Write the failing e2e checks:** the fix button in a Gaffer chat fills the composer and focuses it; a half-typed draft is kept with the message on a new line; a 1,990-character draft is untouched and the message is copied instead with the button reading "Copied. The composer is nearly full."; a terminal session shows **Copy message for the agent**; and `nothing is sent`: after the click, no request other than the inspect reads is issued.
- [ ] **Step 2: Run** the e2e. Expected: FAIL.
- [ ] **Step 3: Implement.** `ChatPane` applies a prefill only when `sessionId === session.id` and the `nonce` is new, through `prefillText`, then focuses `composerInput`. Keep the change to that one effect.
- [ ] **Step 4: Run** the e2e, `node tests/chat.e2e.mjs` and `node tests/chat-controls.e2e.mjs` (the composer must be unaffected), and `npm run check`. Expected: PASS.
- [ ] **Step 5: Commit:** `Offer to fill the composer with a fix request, never sending it`.

### Task 14: Stability, narrow widths, docs and full verification

**Files:**
- Modify: `web/tests/session-workspace.e2e.mjs`, `docs/terminal-workspace.md` (rewrite the Overview bullet; add the prompt retention sentence to the inspection paragraph), `docs/ui-verification.md`, `handoff.md` (`OVERVIEW-3`)

- [ ] **Step 1: Extend the existing layout-stability check** to the new slots: change the headline, notice, activity and PR state, and assert the vertical position of every later block is unchanged; run it in both themes at panel widths 300, 340 and 520 px and in the phone sheet at 340 px, with no horizontal overflow and 44 px targets under `pointer: coarse`.
- [ ] **Step 2: Run the full verification:** `go vet ./... && go test ./... -count=1`; `cd web && npm test && npm run check && npm run build`; then against `vite preview`: `session-workspace`, `sessions`, `chat`, `chat-controls`, `terminal`. Expected: all PASS.
- [ ] **Step 3: Docs.** Update `docs/terminal-workspace.md` (Overview contents, stage rules, actions, that nothing is sent) and add a `docs/ui-verification.md` entry listing the exact commands run and the tests seen failing with their rule removed. Add `OVERVIEW-3`.
- [ ] **Step 4: Commit and open the draft pull request for Part C:** `Finish the Overview: stability, widths and documentation`.
