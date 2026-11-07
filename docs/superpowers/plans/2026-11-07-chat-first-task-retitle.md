# Name Chat Sessions From Their First Real Task Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Chat session started without a real prompt is renamed once, from the first of its first three accepted prompts that yields a title, and a name a person chose is never touched.

**Architecture:** `terminal_sessions` gains `title_origin` (`manual` | `prompt` | `fallback`, default `manual`) and `retitle_tries`. The controller resolves the origin at create from the request (`terminalproto.ResolveTitle`); `PromptChat` calls a guarded store method after the agent acknowledges a prompt; `RenameTerminalFor` locks the origin to `manual`. The web client sends an explicit `titleOrigin` on launches that derive their own title. Open views see the new title on their next session-list refresh (Home polls every 15 s); there is no push channel for titles.

**Tech Stack:** Go (`internal/terminalproto`, `internal/store`, `internal/terminalbroker`, `internal/api`), SQLite, Svelte/TypeScript (`web/`), Node unit tests and Playwright scripts.

**Spec:** `docs/superpowers/specs/2026-11-07-chat-first-task-retitle-design.md`

## Global Constraints

- No model, provider or network call: the name is `terminalproto.PromptTitle(text)`.
- Chat sessions only (`structured = 1`). Terminal (TUI) sessions are never renamed.
- Cap: `retitleMaxTries = 3` accepted prompts considered per session; the rename happens at most once.
- Existing rows default to `manual`. Older clients that send no `titleOrigin` produce `manual`.
- `Create.TitleOrigin` is `json:"-"` like `ProjectID`: never sent to a machine, never part of the stored create parameters.
- Titles stay valid per `terminalproto.ValidTitle` (1–80 plain characters, `MaxTitleRunes`).
- A plain terminal create (no `tool`) stays byte-identical: `titleOrigin` is added to a request body only for a launch that has a `tool`.
- No lifecycle, process, worktree, approval or Chat/Terminal view-switching change; no connector or protocol change.
- Any new or changed browser script must be listed in `.github/workflows/terminal-browser.yml` (PR #644 failed CI because a suite was outside the local validation list).
- Never commit `node_modules` symlinks.

## Review Focus

- Greeting-only first message ("hi", "thanks"): no rename, session stays eligible until the third prompt.
- Image-only message (empty text): counts as a try, yields no title, never errors.
- A failed or unacknowledged send: no rename and no try counted.
- A person renames at the same moment the first message is accepted: the manual title wins.
- A queued message sent later by the queue drain, and a replayed or repeated message: renames once, never again.
- Title text that `ValidTitle` rejects after `PromptTitle`: skipped like a greeting, no error.
- An old client or hand-written API call that claims `prompt`/`fallback` with a title that is not what `AutoTitle` returns: stored as `manual`.

---

### Task 1: Title origin in `terminalproto`

**Files:**
- Modify: `internal/terminalproto/autotitle.go`
- Modify: `internal/terminalproto/protocol.go` (the `Create` struct, next to `ProjectID` at line 485)
- Modify: `internal/terminalproto/testdata/autotitle.json`
- Test: `internal/terminalproto/autotitle_test.go`

**Interfaces:**
- Produces:
  - `type TitleOrigin string` with constants `TitleManual = "manual"`, `TitlePrompt = "prompt"`, `TitleFallback = "fallback"`.
  - `func ValidTitleOrigin(s string) bool` (true for exactly the three values).
  - `func PromptOrigin(prompt string) TitleOrigin` — `TitlePrompt` when `PromptTitle(prompt) != ""`, else `TitleFallback`.
  - `func ResolveTitle(title, claim, tool, prompt, dir string) (string, TitleOrigin)`.
  - `Create.TitleOrigin TitleOrigin` tagged `json:"-"`.
- Fixture: a new top-level `"origin"` array in `autotitle.json`, entries `{"name", "prompt", "want"}` where `want` is `"prompt"` or `"fallback"`. The web test in Task 4 reads the same entries.

- [ ] **Step 1: Write the failing tests** in `autotitle_test.go` (extend `autoTitleFixture` with `Origin []struct{Name, Prompt, Want string}` `json:"origin"`):
  - `TestPromptOriginFixture`: for every fixture entry, `string(PromptOrigin(c.Prompt)) == c.Want`.
  - `TestResolveTitle` (table): 
    - blank title, prompt "Please fix session end navigation" → `("Fix session end navigation", TitlePrompt)`.
    - blank title, blank prompt, tool "claude", dir "/home/dev/app" → `("Claude Code · app", TitleFallback)`.
    - title equal to `AutoTitle` result, matching claim (`"prompt"`) → origin `TitlePrompt`, title unchanged.
    - same title, claim `"fallback"` (mismatch) → `TitleManual`.
    - same title, empty claim (old client) → `TitleManual`.
    - fallback-form title typed by a person with claim `"manual"` → `TitleManual`.
    - any other title with any claim → `TitleManual`, title unchanged.
    - whitespace-only title is treated as blank.
  - `TestValidTitleOrigin`: the three values true; `""`, `"Manual"`, `"auto"` false.
- [ ] **Step 2: Add the fixture entries** to `autotitle.json` `"origin"`: a task prompt → `prompt`; `"hi"` → `fallback`; `""` → `fallback`; `"Use the reet skill."` (skills-picker line, per `titleSkillLine`) → `fallback`; a prompt whose first line is a greeting but second line is a task → `prompt`.
- [ ] **Step 3: Run** `go test ./internal/terminalproto -run 'Origin|ResolveTitle' -v` — expect FAIL (undefined symbols).
- [ ] **Step 4: Implement** the type, constants, `ValidTitleOrigin`, `PromptOrigin` and `ResolveTitle` in `autotitle.go` and the `Create.TitleOrigin` field in `protocol.go`. `ResolveTitle`: blank (after `strings.TrimSpace`) → `AutoTitle` and `PromptOrigin`; otherwise compare `title` to `AutoTitle(tool, prompt, dir)` and `claim` to `PromptOrigin(prompt)`; both equal and claim is `prompt`/`fallback` → that claim, else `TitleManual`. Comparison is exact (no trimming of the supplied title).
- [ ] **Step 5: Run** `go test ./internal/terminalproto -v` and `go vet ./internal/terminalproto` — expect PASS (the existing `TestPromptTitleFixture` and `TestAutoTitleFixture` still pass).
- [ ] **Step 6: Commit** — `git add internal/terminalproto && git commit -m "Add title origin and request-time title resolution"`.

---

### Task 2: Store — columns, create, manual lock, guarded retitle

**Files:**
- Modify: `internal/store/store_terminal.go` (the `migrateAttention` call at line 88; the `INSERT` at line 635; `RenameTerminalFor` at line 1090)
- Create: `internal/store/store_title_origin.go`
- Test: `internal/store/store_title_origin_test.go` (uses `terminalFixture` and `reserveChatRow` from `store_terminal_test.go`)

**Interfaces:**
- Consumes: Task 1's `terminalproto.TitleOrigin`, `terminalproto.PromptTitle`, `terminalproto.ValidTitle`, `Create.TitleOrigin`.
- Produces:
  - `func (s *Store) migrateTitleOrigin() error` — idempotent, `hasColumn` + `ALTER TABLE terminal_sessions ADD COLUMN`, same loop as `migrateAttention`:
    `title_origin TEXT NOT NULL DEFAULT 'manual' CHECK(title_origin IN ('manual','prompt','fallback'))` and `retitle_tries INTEGER NOT NULL DEFAULT 0`. Called right after `migrateAttention()` at `store_terminal.go:88`.
  - `const retitleMaxTries = 3`.
  - `func (s *Store) RetitleChatFromPrompt(uid int64, sessionID, text string) (renamed bool, err error)`.
  - `func (s *Store) TerminalTitleOriginFor(uid int64, sessionID string) (terminalproto.TitleOrigin, error)` — test and diagnostics read of the stored origin (not added to the JSON listing).

- [ ] **Step 1: Write the failing tests:**
  - `TestTitleOriginMigrationKeepsExistingRowsManual`: build the table without the two columns (as `store_agents_home_test.go:203` drops columns), insert a row, run `migrateTitleOrigin` twice, assert origin `manual` and tries `0`.
  - `TestReserveStoresOriginFromCreate`: reserve with `Create{TitleOrigin: TitleFallback}` → stored `fallback`; with empty `TitleOrigin` → `manual`.
  - `TestRenameTerminalForLocksManual`: reserve a `fallback` chat, call `RenameTerminalFor`, assert title changed and origin `manual`.
  - `TestRetitleRenamesOnceFromFirstRealTask` (fallback structured session): text `"hi"` → `(false, nil)`, tries 1, title unchanged; then `"Please fix session end navigation"` → `(true, nil)`, title `"Fix session end navigation"`, origin `prompt`, tries 2; then a third text → `(false, nil)`, title unchanged.
  - `TestRetitleStopsAfterThreeTries`: three greetings, then a real task → `(false, nil)`, title unchanged, tries 3.
  - `TestRetitleSkipsManualPromptAndPlainSessions`: a `manual` structured row, a `prompt` row and a `fallback` non-structured (TUI) row are never changed and `tries` is not incremented.
  - `TestRetitleImageOnlyCountsATry`: text `""` → `(false, nil)`, tries 1.
  - `TestRetitleManualRenameWins`: rename manually first, then `RetitleChatFromPrompt` with a real task → `(false, nil)`, manual title kept.
  - `TestRetitleWrongOwner`: another `uid` → `(false, nil)`, nothing changes.
- [ ] **Step 2: Run** `go test ./internal/store -run 'TitleOrigin|Retitle|RenameTerminalForLocks|ReserveStoresOrigin' -v` — expect FAIL.
- [ ] **Step 3: Implement** the migration and its call, the `INSERT` column (`title_origin`; value is `in.TitleOrigin`, or `manual` when empty or invalid), the `RenameTerminalFor` `UPDATE` (`SET title=?, title_origin='manual', updated_at=?`), and `RetitleChatFromPrompt` in one `s.withTx`: select `structured,title_origin,retitle_tries` for `id=? AND user_id=?`; return `(false, nil)` unless structured, origin `fallback` and tries `< retitleMaxTries`; increment tries; derive `PromptTitle(text)`; if empty or `!ValidTitle` return `(false, nil)`; run `UPDATE … SET title=?, title_origin='prompt', updated_at=? WHERE id=? AND user_id=? AND title_origin='fallback'` and report whether a row changed. The tries increment and the title update must commit together.
- [ ] **Step 4: Run** `go test ./internal/store -v` and `go vet ./internal/store` — expect PASS (existing store tests unaffected).
- [ ] **Step 5: Commit** — `git add internal/store && git commit -m "Track who owns a session title and retitle a fallback once"`.

---

### Task 3: Controller wiring — create and `PromptChat`

**Files:**
- Modify: `internal/api/terminal.go` (the `terminalCreate` body struct near line 273 and the title block at line 308)
- Modify: `internal/terminalbroker/chat.go` (`PromptChat`, final `return nil` at line 993)
- Test: `internal/api/terminal_title_origin_test.go` (create), `internal/terminalbroker/chat_retitle_test.go` (create; uses `fixture` from `broker_test.go`)

**Interfaces:**
- Consumes: Task 1 `ResolveTitle`, Task 2 `RetitleChatFromPrompt`, `TerminalTitleOriginFor`.
- Produces: the create body accepts optional `"titleOrigin": "prompt" | "fallback" | "manual"`; unknown values are treated as empty (never an error). `PromptChat` calls `b.st.RetitleChatFromPrompt(uid, session, text)` only after a successful acknowledgement and ignores its error (logged at most; a naming failure never fails a send).

- [ ] **Step 1: Write the failing tests:**
  - API (`terminalCreate`, using the existing handler test harness in `internal/api`): `TestCreateStoresResolvedTitleOrigin` table — blank title + blank prompt → stored `fallback`; blank title + task prompt → `prompt`; title equal to `AutoTitle` with matching `titleOrigin` → that origin; mismatched claim → `manual`; no `titleOrigin` with a supplied title → `manual`; a retried create (same `requestId`) returns the same session and origin.
  - Broker: `TestPromptChatRetitlesFallbackChatOnce` — reserve a `fallback` structured session, send `"hi"` (yields no title, session unchanged), then `"Please fix session end navigation"`: title becomes `"Fix session end navigation"`; a third prompt changes nothing. `TestPromptChatFailedSendDoesNotRetitle` — peer returns a non-OK result for the prompt (as the existing "not sent" tests do): title and tries unchanged. `TestPromptChatQueueDrainRetitles` — a queued message sent through `chat_queue.go` renames a fallback session. `TestPromptChatNeverRetitlesPlainSession` — the existing plain-session refusal path leaves the title alone.
- [ ] **Step 2: Run** `go test ./internal/api ./internal/terminalbroker -run 'TitleOrigin|Retitle' -v` — expect FAIL.
- [ ] **Step 3: Implement** the `TitleOrigin string json:"titleOrigin"` request field and replace the `AutoTitle` block with `create.Title, create.TitleOrigin = terminalproto.ResolveTitle(in.Title, in.TitleOrigin, in.Tool, in.Prompt, in.Dir)`; keep the retry-safety comment (the result depends only on the request). In `PromptChat`, call the store method just before the final `return nil`.
- [ ] **Step 4: Run** `go vet ./... && go test -race ./internal/api ./internal/terminalbroker ./internal/store ./internal/terminalproto` — expect PASS.
- [ ] **Step 5: Commit** — `git add internal && git commit -m "Resolve title origin at create and retitle after the first accepted Chat task"`.

---

### Task 4: Web client sends `titleOrigin`

**Files:**
- Modify: `web/src/lib/agentLaunch.ts` (next to `promptTitle`/`titleFor`, lines ~118–200)
- Modify: `web/src/lib/terminal/api.ts` (`LaunchRequest` line 45; `createTerminal` body loop line ~178)
- Modify: `web/src/lib/launchBar.ts` (`toCreate`, line ~345)
- Modify: `web/src/lib/StartAgentDialog.svelte` (line ~532 where `taskTitle` and `launch` are built)
- Test: `web/src/lib/agentLaunch.test.mjs`, `web/src/lib/launchBar.test.mjs`, `web/tests/start-agent.e2e.mjs`

**Interfaces:**
- Consumes: the `"origin"` fixture entries from Task 1.
- Produces: `export function titleOriginFor(prompt: string): 'prompt' | 'fallback'` in `agentLaunch.ts` (`promptTitle(prompt) ? 'prompt' : 'fallback'`); `LaunchRequest.titleOrigin?: 'prompt' | 'fallback'`; `createTerminal` adds `titleOrigin` to the body only when `launch?.tool` is set.

- [ ] **Step 1: Write the failing tests:**
  - `agentLaunch.test.mjs`: for every entry in `autotitle.json`'s `"origin"` array, `titleOriginFor(entry.prompt) === entry.want`.
  - `launchBar.test.mjs`: `toCreate(...)` returns `launch.titleOrigin === 'prompt'` for a choice with a task prompt and `'fallback'` for a blank prompt; a plain-terminal choice keeps `launch.tool` unset and has no `titleOrigin`.
  - `start-agent.e2e.mjs`: update the exact-body expectations that include a `tool` (lines 316 and 363 and any other `toEqual` on a launch body) to include `titleOrigin:'prompt'` (or `'fallback'` for blank-prompt launches); keep the plain terminal body assertions at lines 339–340 unchanged to prove it stays byte-identical. Add one assertion that a blank-prompt Claude launch sends `titleOrigin:'fallback'` with title `Claude Code · <folder>` and an image-attached launch still sends `titleOrigin` derived from the typed task, not the upload paths.
- [ ] **Step 2: Run** `node web/src/lib/agentLaunch.test.mjs && node web/src/lib/launchBar.test.mjs` — expect FAIL.
- [ ] **Step 3: Implement** `titleOriginFor`, the `LaunchRequest` field, the `createTerminal` body rule and set `titleOrigin` in `toCreate` and in the dialog's `launch` object from the original task text (before attachment paths are appended — the same text `taskTitle` uses). Do not change `promptTitle`, `titleFor` or any title text.
- [ ] **Step 4: Run** `npm --prefix web run check && npm --prefix web run build`, then serve a preview and run `REET_UI_URL=http://127.0.0.1:4173 node web/tests/start-agent.e2e.mjs` and `node web/tests/launch-options.e2e.mjs` — expect all PASS, zero Svelte diagnostics. Confirm both scripts appear in `.github/workflows/terminal-browser.yml`.
- [ ] **Step 5: Commit** — `git add web && git commit -m "Send the title origin with launches that derive their own title"`.

---

### Task 5: Handoff — record the shipped contract

**Files:**
- Modify: `docs/handoffs/compact-work-items.md` (a short "Later-message retitling" section)

- [ ] **Step 1:** Add the shipped contract in four lines (origin values, Chat only, three-try cap, manual wins), link the spec and plan, and state that open views pick the new title up on their next session-list refresh. Leave attention identity and navigation listed as separate work. Record that deployment of the change is a separate verification.
- [ ] **Step 2: Run** `git diff --stat` — expect only that file.
- [ ] **Step 3: Commit** — `git add docs && git commit -m "Record the title ownership contract in the handoff"`.

---

## Self-review

- **Spec coverage:** ownership columns and defaults (Task 2); create-time origin and explicit `titleOrigin` (Tasks 1, 3, 4); manual lock (Task 2); post-ack rename, cap, race safety, idempotence (Tasks 2, 3); Chat-only (Task 2 structured guard, Task 3 plain-session test); preserved behaviour and no protocol change (Global Constraints, `json:"-"`); shared fixture risk (Task 1 fixture `"origin"` read by Go and web); migration risk (Task 2 migration test); CI-list risk (Task 4 Step 4). The spec's event-stream wording was wrong (titles are polled, not pushed) and is corrected in the spec itself, alongside this plan.
- **Types:** `TitleOrigin`, `ResolveTitle`, `PromptOrigin`, `RetitleChatFromPrompt`, `TerminalTitleOriginFor`, `titleOriginFor` and `LaunchRequest.titleOrigin` are named identically wherever used.
- **Proportion:** five tasks, tests specified by name and assertion, no function bodies except where the spec pins behaviour.
