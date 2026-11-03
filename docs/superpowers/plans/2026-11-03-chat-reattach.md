# Seamless chat reattach Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A lost or crashed Gaffer (OpenCode) chat reconnects by itself in the same row, URL and transcript, with a visible "Reconnecting…" state, no duplicate rows and a retry card on failure.

**Architecture:** A new `chat_reattach` control message (feature `chat-reattach-v1`, modelled on `session_mode`) asks the machine to start OpenCode with `session/load` under the **existing reet session id**, dropping the replay (the controller already holds the transcript). The controller's store already allows `lost → running` for structured chats; it gains `exited → running`. The browser triggers one automatic attempt per lost row and shows state in the pane.

**Tech Stack:** Go (`internal/terminalproto`, `terminalagent`, `terminalbroker`, `store`, `api`, `agenthealth`), Svelte 5 + TypeScript (`web/src/lib`), Playwright e2e (`web/tests`).

**Spec:** `docs/superpowers/specs/2026-11-03-chat-reattach-design.md`

## Deviations from the spec (recorded in Task 1's spec edit)

Found while mapping the code; each is simpler and meets the same goal:

1. **Body carries the row's launch data**, like `session_mode` does (`Tool, Dir, Model, Yolo, ACPSessionID, Title, Trust`), not just `{id}`. The machine then does not depend on its restart registry still holding a record.
2. **Single-flight is in controller memory** (a map beside `switching`), not a `terminal_operations` row. A controller restart mid-attempt leaves the row `lost`; the next auto-attempt re-sends and the machine adopts the live process, so nothing durable is needed.
3. **No `reattach` field on the session payload.** The HTTP call is held until the machine answers (like create and `session_mode`); the browser that made it shows "Reconnecting…", and a second caller joins the in-flight call and gets its result. Other tabs see `lost → running` through normal polling.

## Global Constraints

- Feature name is exactly `chat-reattach-v1`; wire body kind is exactly `chat_reattach`; capability JSON field is `chatReattach`.
- `chat-reattach-v1` requires `acp-v1` and `acp-history-v1`; it is never claimed without both.
- OpenCode only (`tool == "opencode"`); Claude/Codex are out of scope.
- Reattach is allowed only from states `lost` and `exited`. A user-ended chat (`ended`) keeps **Continue in Gaffer with history…** (a new row).
- A peer that did not negotiate the feature is never sent the body (strict decoder rejects unknown kinds).
- Never read `terminal_sessions` rows of another account; every broker call takes `uid`.
- Quote exact UI copy: status "Reconnecting to your conversation…"; failure actions **Try again** and **Start a new chat**; manual button label **Reconnect**.
- Repo checks before every push: `go vet ./...`, `go test -race ./internal/...` for touched packages, and in `web/`: `npm run check`.

## Review Focus

1. Row is `exited` (crash), not just `lost`: must reattach and must not resurrect a **non-structured** shell. (Task 3 test)
2. Two tabs (or double click) request reattach at once: exactly one machine call, both callers get the same result. (Task 5)
3. Machine still has the old process alive when the controller marks the row lost: adopted, not a 409. (Task 4)
4. A different row already owns the same OpenCode conversation: refused with a conflict message, not adopted. (Task 4)
5. Machine goes offline mid-attempt: row stays `lost`, error is retryable, no stale `creating` row exists. (Task 5)

---

### Task 1: Protocol feature, body, capability (and spec deviations)

**Files:**
- Modify: `internal/terminalproto/acp.go` (beside `SessionModeFeature`, ~line 83; `SessionMode` struct ~line 359)
- Modify: `internal/terminalproto/protocol.go` (`Capabilities` ~line 168; validator ~line 239; session-kind list line 582; decoder switch ~line 703)
- Modify: `internal/terminalproto/acp_agents.go` (beside `SessionModeFor`, ~line 265)
- Modify: `docs/superpowers/specs/2026-11-03-chat-reattach-design.md` (apply the three deviations above)
- Test: `internal/terminalproto/acp_test.go` (or the file holding the existing `session_mode` decode tests; `grep -n session_mode internal/terminalproto/*_test.go`)

**Interfaces:**
- Produces:
  - `const ChatReattachFeature = "chat-reattach-v1"`
  - `func SupportsChatReattach(features string) bool`
  - `type ChatReattach struct { Tool, Dir string; Model string; Yolo bool; ACPSessionID string; Title string; Trust bool }` with JSON tags `tool, dir, model(omitempty), yolo(omitempty), acpSessionId, title(omitempty), trust(omitempty)`; `kind() == "chat_reattach"`; `valid()` requires `ValidACPSessionIDFor(Tool, ACPSessionID) && ValidDir(Dir) && (Model=="" || ValidModel(Model)) && (Title=="" || ValidTitle(Title))`.
  - `Capabilities.ChatReattach bool` (`json:"chatReattach,omitempty"`); invalid when set without `ACP && ACPHistory`.
  - `func (c Capabilities) ChatReattachFor(tool string) bool` → `c.ChatReattach && tool == "opencode"`.

- [ ] **Step 1: Write failing tests** in the terminalproto test file: `TestChatReattachDecodes` (round-trip a `Message` with `SessionID` and `&ChatReattach{Tool:"opencode",Dir:"/home/u/site",ACPSessionID:"ses_fake000000000000000000000001"}`; assert decoded body type `*ChatReattach`), `TestChatReattachRejectsBadFields` (empty ACPSessionID, Dir with `..`, tool `"claude"` with an OpenCode-shaped id each return `ErrInvalid`), `TestCapabilitiesChatReattachNeedsHistory` (`Capabilities{ChatReattach:true, ACP:true}` validates to `ErrInvalid`; with `ACPHistory:true` plus the fields the existing session-mode validator test sets, nil), `TestSupportsChatReattach` (header string `"acp-v1, chat-reattach-v1"` true; `"acp-v1"` false).
- [ ] **Step 2: Run** `go test ./internal/terminalproto/ -run 'ChatReattach|SupportsChatReattach' -v`. Expected: FAIL (undefined symbols).
- [ ] **Step 3: Implement** the interface block above. Add `"chat_reattach"` to the session-kind list at `protocol.go:582` and `case "chat_reattach": b = &ChatReattach{}` to the decoder. Copy the shape of `SessionMode`/`SupportsSessionMode`.
- [ ] **Step 4: Edit the spec** to match the three deviations (replace the `chat_reattach {id}` line, the `terminal_operations` bullets and the payload `reattach` bullet).
- [ ] **Step 5: Run** `go test ./internal/terminalproto/ -v`. Expected: PASS.
- [ ] **Step 6: Commit** `git add internal/terminalproto docs/superpowers/specs && git commit -m "Add chat-reattach-v1 protocol body and capability"`.

### Task 2: Negotiation on connector and controller

**Files:**
- Modify: `internal/api/private_control.go:19` (add `terminalproto.ChatReattachFeature` after `SessionModeFeature`)
- Modify: `internal/agenthealth/agent.go` (config field near `cfg.sessionMode` ~line 752; feature append ~line 830; `Capabilities{…}` literal ~line 1005)
- Modify: `internal/terminalagent/agent.go` (inbound feature gate, beside the `*terminalproto.SessionMode` case at ~line 280)
- Test: `internal/agenthealth/acp_agents_test.go`, `internal/terminalagent/chat_test.go`

**Interfaces:**
- Consumes: Task 1 symbols.
- Produces: a connector reports `Capabilities.ChatReattach == true` only when negotiated, `acp`, `acpHistory` and OpenCode is among chat agents; the connector refuses `*ChatReattach` from a peer that did not negotiate it.

- [ ] **Step 1: Write failing tests**: `TestChatReattachCapabilityNeedsNegotiationAndHistory` (table: negotiated+history → true; negotiated without history → false; history without negotiation → false) following the existing session-mode capability test in `acp_agents_test.go`; `TestChatReattachRefusedWithoutFeature` in `chat_test.go` (a `ChatReattach` message on a connection whose feature header lacks `chat-reattach-v1` returns `ErrInvalid`, mirroring the existing `SessionMode` refusal test).
- [ ] **Step 2: Run** `go test ./internal/agenthealth/ ./internal/terminalagent/ -run ChatReattach -v`. Expected: FAIL.
- [ ] **Step 3: Implement** `cfg.chatReattach = terminalproto.SupportsChatReattach(advertised) && cfg.acp && cfg.acpHistory`, append the feature when set, set `ChatReattach: acp && cfg.acpHistory && cfg.chatReattach && slices.Contains(chatToolNames, "opencode")` (use the same OpenCode check `SessionMode` uses), and add the `case *terminalproto.ChatReattach:` gate returning `ErrInvalid` when `!SupportsChatReattach(headers.Get(FeatureHeader))`.
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Commit** `git commit -am "Negotiate chat-reattach-v1 between controller and connector"`.

### Task 3: Store allows `exited → running` for structured chats

**Files:**
- Modify: `internal/store/store_terminal.go:721-731` (`ObserveTerminalStateFor`); `:858-865` (`CanResumeACPToolSessionFor`)
- Test: `internal/store/store_terminal_test.go`

**Interfaces:**
- Produces: `ObserveTerminalStateFor(…, Running, nil)` succeeds for a structured row in `exited` (and still for `lost`); returns `ErrTerminalConflict` for a non-structured row in either state and for `ended`. `CanResumeACPToolSessionFor` no longer accepts a row whose `create` operation has `state='failed'` (a continuation that never started).

- [ ] **Step 1: Write failing tests**: `TestObserveRunningReopensExitedStructuredChat` (structured row `exited` → running ok); `TestObserveRunningNeverReopensExitedShell` (non-structured `exited` → `ErrTerminalConflict`; `lost` likewise); `TestObserveRunningNeverReopensEndedChat`; `TestCannotResumeFromFailedContinuationRow` (row `lost` with failed create op and same `acp_session_id` → `false`; a row whose create op succeeded → `true`). Use the existing store test helpers that create terminal sessions and operations in this file.
- [ ] **Step 2: Run** `go test ./internal/store/ -run 'ReopensExited|NeverReopens|FailedContinuation' -v`. Expected: FAIL.
- [ ] **Step 3: Implement** by widening the condition at line 721 to `(t.State == Lost || t.State == Exited) && next == Running` with the existing `!t.Structured` guard, and adding `AND NOT EXISTS(SELECT 1 FROM terminal_operations o WHERE o.session_id=terminal_sessions.id AND o.user_id=terminal_sessions.user_id AND o.kind='create' AND o.state='failed')` to the `EXISTS` subquery in `CanResumeACPToolSessionFor`.
- [ ] **Step 4: Run** `go test -race ./internal/store/ -v`. Expected: PASS (existing continuation tests still pass).
- [ ] **Step 5: Commit** `git commit -am "Let a structured chat reopen from exited; stop resuming from never-started rows"`.

### Task 4: Machine handler

**Files:**
- Create: `internal/terminalagent/chat_reattach.go`
- Modify: `internal/terminalagent/chat.go` (`chatHost.open`, line ~327: split into `openChat`, see below)
- Modify: `internal/terminalagent/agent.go` (dispatch beside the `*terminalproto.SessionMode` case, ~line 618)
- Test: `internal/terminalagent/chat_reattach_test.go`

**Interfaces:**
- Consumes: `terminalproto.ChatReattach`; existing `chatSupervisor` (`sessions`, `conversationInUseLocked`, `remember`, `announceRunning`, `ended`), `reportedACP`.
- Produces:
  - `func (h *chatHost) openChat(requestID, sessionID string, in terminalproto.Create, dropReplay bool) (terminalproto.SessionChanged, error)`; `open` becomes a one-line call with `dropReplay=false`. With `dropReplay=true` the `OnEvent` callback forwards nothing until the process is started (copy the `restored atomic.Bool` gate from `resumeChat`, `chat_registry.go:319`).
  - `func (h *chatHost) reattach(requestID, sessionID string, in terminalproto.ChatReattach) (terminalproto.SessionChanged, error)`:
    1. If `h.sup.sessions[sessionID]` exists with a live process → return `Running` with `reportedACP` (adopt; no restart).
    2. If `conversationInUseLocked(in.Tool, in.ACPSessionID)` by a different id → `terminalhost.ErrOperationConflict`.
    3. Otherwise call `openChat(requestID, sessionID, Create{Tool, Dir, Model, Yolo, Trust, Structured:true, Title, ResumeACP: ACPSessionID}, true)`.
    4. Idempotent on `requestID` via the existing `requests` map.
  - Dispatch: same goroutine/`addWork`/timeout shape as `SessionMode`, using a 60 s context.

- [ ] **Step 1: Write failing tests** (use the fake ACP process helpers already in `chat_test.go`): `TestReattachStartsLoadUnderSameSessionID` (fake agent records `session/load` with the saved id; reply state `Running`, `ACPSessionID` echoed; `chats.snapshot(sessionID)` exists); `TestReattachDropsReplay` (fake agent replays 3 `user`/`assistant` events during load; the controller-side `publish` sink receives none of them, but receives a post-start event); `TestReattachAdoptsLiveProcess` (existing running chat for the same id → reply `Running`, fake agent saw exactly one `session/load` total); `TestReattachRefusedWhenAnotherRowOwnsConversation` (running chat with a different session id and the same ACP id → `ErrOperationConflict`); `TestReattachIsIdempotentPerRequestID`; `TestReattachFailureLeavesNothingRunning` (fake agent errors on load → error returned, `snapshot` absent, quota unchanged).
- [ ] **Step 2: Run** `go test -race ./internal/terminalagent/ -run Reattach -v`. Expected: FAIL.
- [ ] **Step 3: Implement** per the Interfaces block. The refactor of `open` must not change behaviour for non-reattach callers (existing `chat_test.go` stays green).
- [ ] **Step 4: Run** `go test -race ./internal/terminalagent/ -v`. Expected: PASS, no new race reports.
- [ ] **Step 5: Commit** `git add internal/terminalagent && git commit -m "Reattach a chat under its own session id on the machine"`.

### Task 5: Controller `ReattachChat` with single-flight

**Files:**
- Create: `internal/terminalbroker/chat_reattach.go`
- Modify: `internal/terminalbroker/broker.go` (add `reattaching map[chatKey]*reattachCall` beside `switching`, ~line 44)
- Test: `internal/terminalbroker/chat_reattach_test.go` (reuse `switchFixtureCaps`/`switchMachine` from `session_mode_test.go`; add `ChatReattach` to the scripted machine's recorded messages)

**Interfaces:**
- Consumes: Task 1 body/capability; `Peer.callWithin`, `ObserveTerminalStateFor`, `RecordTerminalACPSessionFor`, `chatLog`, `restoreChat`, `AddEventFor`.
- Produces:
  - `func (b *Broker) ReattachChat(ctx context.Context, uid int64, session string) error`
  - Errors (exported vars, user-facing sentences): `ErrReattachUnsupported` ("Update reet on this machine to reconnect this chat."), `ErrReattachNotClosed` ("This chat is still running."), `ErrReattachNoConversation` ("This chat has no saved conversation to reconnect."), `ErrReattachConflict` ("This conversation is open in another session."), `ErrReattachFailed` ("The chat could not be reconnected. Try again, or start a new chat.").
- Behaviour: row must be `lost|exited`, structured, tool `opencode`, valid `ACPSessionID`; peer generation matches caps with `ChatReattachFor(tool)` and `ChatAgent(tool).History`. Single-flight: a second concurrent call waits for the first's result and returns it, sending no second message. Call uses a fresh `ID()` request id, `context.WithoutCancel`, timeout `60*time.Second + 15*time.Second`. Ensure `b.chatLog(uid, session)` exists before sending; **do not** close the ring (the transcript is kept). On a `SessionChanged` with `Running` → `ObserveTerminalStateFor(…, Running, nil)`, record a changed ACP id if reported, `restoreChat`, `AddEventFor(uid,"machine","chat-reattach","",{machine,tool,session})`. A `Result` with `ACPTrustRequired|ACPVersion|ACPUnavailable` maps through `acpRefusal`; a conflict code maps to `ErrReattachConflict`; anything else `ErrReattachFailed`. On any error the row state is untouched.

- [ ] **Step 1: Write failing tests**: `TestReattachLostChatBecomesRunningInPlace` (row id unchanged, state `running`, exactly one session row for the account, ring still holds earlier events); `TestReattachExitedChat`; `TestReattachRefusedForEndedAndRunningAndShell`; `TestReattachRefusedWithoutFeatureOrHistory` (`ErrReattachUnsupported`, machine receives nothing); `TestReattachRefusedWithoutACPSessionID`; `TestReattachSingleFlightSendsOnce` (10 goroutines, `machine.sent()` has one `ChatReattach`, all return nil); `TestReattachOfflineLeavesRowLostAndNoCreatingRows` (peer error → retryable error, state `lost`, no row in `creating`); `TestReattachMachineConflictMapsToErrReattachConflict`; `TestReattachOtherAccountRefused`.
- [ ] **Step 2: Run** `go test -race ./internal/terminalbroker/ -run Reattach -v`. Expected: FAIL.
- [ ] **Step 3: Implement** per the Interfaces block; model the guard/ack code on `SwitchSessionMode` and `sessionModeAck` in `session_mode.go`.
- [ ] **Step 4: Run** `go test -race ./internal/terminalbroker/ -v`. Expected: PASS.
- [ ] **Step 5: Commit** `git add internal/terminalbroker && git commit -m "Reattach a lost chat in place from the controller"`.

### Task 6: HTTP route

**Files:**
- Modify: `internal/api/terminal_chat.go` (route near line 25; handler and error mapper beside `terminalSessionMode`/`terminalSessionModeError`)
- Test: `internal/api/terminal_chat_test.go` (follow the existing `/mode` route tests)

**Interfaces:**
- Consumes: `Broker.ReattachChat`.
- Produces: `POST /api/v2/terminal/sessions/{id}/reattach` (no body) → `204` on success; error codes: `chat_reattach_unsupported` 409, `chat_reattach_not_closed` 409, `chat_reattach_no_conversation` 409, `chat_reattach_conflict` 409, `chat_reattach_failed` 502; unknown session 404; invalid id 400; offline → the existing offline mapping used by `terminalChatError`.

- [ ] **Step 1: Write failing tests**: `TestReattachRouteSuccess204`, `TestReattachRouteErrorCodes` (table over the five errors + 404 + 400), `TestReattachRouteRequiresSession` (other account's id → 404).
- [ ] **Step 2: Run** `go test ./internal/api/ -run Reattach -v`. Expected: FAIL.
- [ ] **Step 3: Implement** handler mirroring `terminalSessionMode` (same `terminal(...)` wrapper, no request deadline) and an error mapper using `errors.Is`.
- [ ] **Step 4: Run** `go vet ./... && go test -race ./internal/... ` (whole Go suite). Expected: PASS.
- [ ] **Step 5: Commit** `git commit -am "Add the chat reattach route"`.

### Task 7: Web logic and copy

**Files:**
- Modify: `web/src/lib/terminal/api.ts` (add `chatReattach?: boolean` to `Capabilities` line 22; add function)
- Modify: `web/src/lib/sessions.ts` (beside `canContinueChat`, line ~154)
- Modify: `web/src/lib/chat.ts` (`endedReason`, line ~403)
- Test: `web/src/lib/sessions.test.mjs`, `web/src/lib/chat.test.mjs`

**Interfaces:**
- Produces:
  - `export async function reattachChat(sessionId: string, signal?: AbortSignal): Promise<void>` — `terminalRequest('/api/v2/terminal/sessions/'+id+'/reattach',{method:'POST',signal})`; throws `TerminalError` (carries `status`, `code`, message).
  - `export function canReattachChat(s: Pick<TerminalSession,'state'|'tool'|'structured'|'acpSessionId'>, machine: Pick<TerminalMachine,'capabilities'|'ready'> | undefined): boolean` — true only for `lost|exited`, `structured`, `tool==='opencode'`, non-empty `acpSessionId`, `machine?.ready`, `machine.capabilities.chatReattach === true`, and `structuredAgent(caps, tool)?.history === true`.
  - `export function reattachCopy(phase: 'pending' | 'failed', reason?: string): { status: string; detail: string }` in `chat.ts`: pending → `{status:'Reconnecting to your conversation…', detail:'Your earlier messages stay here. Any interrupted reply needs a new prompt.'}`; failed → `{status:'Could not reconnect', detail: reason || 'The saved conversation could not be loaded.'}`.
  - `endedReason(state, name, canContinue, canReattach=false)`: when `canReattach` the lost/exited sentence is followed by `Reconnect to carry on where you left off.` instead of the "Try Continue with history…" sentence.

- [ ] **Step 1: Write failing tests** with exact assertions: `canReattachChat` true for `{state:'lost',tool:'opencode',structured:true,acpSessionId:'ses_a'}` on a ready machine with `chatReattach:true` and a history descriptor; false for each of: `state:'ended'`, `state:'running'`, no `acpSessionId`, `structured:false`, machine not ready, capability absent, `history:false`, `tool:'claude'`. `reattachCopy('pending').status === 'Reconnecting to your conversation…'`. `endedReason('lost','Gaffer',false,true)` contains `Reconnect` and not `Continue with history`.
- [ ] **Step 2: Run** `cd web && node src/lib/sessions.test.mjs && node src/lib/chat.test.mjs`. Expected: FAIL.
- [ ] **Step 3: Implement** the three functions and the `endedReason` parameter (default keeps all current call sites and tests unchanged).
- [ ] **Step 4: Run** `cd web && npm run check`. Expected: PASS, 0 svelte-check errors.
- [ ] **Step 5: Commit** `git commit -am "Add reattach helpers and copy to the web client"`.

### Task 8: Web UI: auto-reattach in place and legacy guard

**Files:**
- Modify: `web/src/lib/terminal/TerminalWorkspace.svelte` (state and function near `continueSession`, line ~359; `ChatPane` usage line ~537)
- Modify: `web/src/lib/terminal/ChatPane.svelte` (props line 18-29; ended banner lines 529-536; composer disabled state)
- Modify: `web/src/lib/terminal/SessionSidebar.svelte` (state label near `dotShape`, line 46)
- Test: `web/tests/chat-recovery.e2e.mjs` (Task 9); unit helper `web/src/lib/terminal/reattachState.ts` + `web/src/lib/terminal/reattachState.test.mjs` for the decision logic (add the test file to the `test` script in `web/package.json`)

**Interfaces:**
- Produces:
  - `reattachState.ts`: `export type ReattachPhase = 'idle' | 'pending' | 'failed'`; `export function shouldAutoReattach(args:{canReattach:boolean; phase:ReattachPhase; tried:ReadonlySet<string>; sessionId:string}): boolean` — true only when `canReattach && phase==='idle' && !tried.has(sessionId)`.
  - `TerminalWorkspace`: `let reattach = $state<Record<string,{phase:'pending'|'failed'; reason?:string}>>({})`, `const autoTried = new Set<string>()`, `async function reattachSession(target: TerminalSession, auto=false)` — no-op if `phase==='pending'` for that id; sets pending, `await reattachChat(target.id, AbortSignal.timeout(80000))`, then `inventoryVersion++; await refresh()` and clears the entry; on `TerminalError` sets `failed` with the message; never navigates. An `$effect` calls `reattachSession(session, true)` when `shouldAutoReattach(...)` for the selected session, adding its id to `autoTried` first (one automatic attempt per id per page load).
  - `ChatPane` new props `reattach?: {phase:'pending'|'failed'; reason?:string}` and `onreattach?: () => void`. Pending: show `reattachCopy('pending')` as the `role="status"` line inside the ended-note, hide both buttons, disable the composer with that reason. Failed: show the reason in the pane with **Try again** (`onreattach`) and **Start a new chat**. Idle with `onreattach`: the primary button reads **Reconnect**. The ended-note and "Earlier messages … no longer available" red line are not rendered while pending.
  - Legacy path (`oncontinue`, peers without the feature): while `continuations[target.id]` is in flight pass `reattach={{phase:'pending'}}`-style status "Starting a new session…" (button disabled), and show a failure in the pane instead of only the global `error`.
  - Sidebar: a `lost` row with `reattach[id]?.phase==='pending'` shows the label **Reconnecting** and the `starting` dot.

- [ ] **Step 1: Write failing unit tests** for `shouldAutoReattach` (true for first lost; false when `tried` has the id; false when pending or failed; false when `canReattach` false) in `reattachState.test.mjs`.
- [ ] **Step 2: Run** `cd web && node src/lib/terminal/reattachState.test.mjs`. Expected: FAIL (module missing).
- [ ] **Step 3: Implement** `reattachState.ts`, wire the workspace state/effect, ChatPane props and banner, sidebar label, and the legacy in-pane status.
- [ ] **Step 4: Run** `cd web && npm run check && npm run build`. Expected: PASS, build succeeds.
- [ ] **Step 5: Commit** `git add web && git commit -m "Reconnect a lost chat automatically in place"`.

### Task 9: Browser contract

**Files:**
- Modify: `web/tests/chat-recovery.e2e.mjs`

**Interfaces:**
- Consumes: Tasks 7-8. The fixture `machine.capabilities` gains `chatReattach: true` for the new cases and omits it for the legacy case (`machineLegacy`). Add the route `'/api/v2/terminal/sessions/:id/reattach'` in the `routes` map pattern this file uses, recording requests and flipping the fixture row to `running` on success.

- [ ] **Step 1: Write the failing cases** (keep the existing four state×viewport cases, now against `machineLegacy`):
  1. `auto-reattach in place`: open a `lost` row on the new machine → exactly one POST to `/reattach`, URL still `session=${S}`, the page never navigates to `NEXT`, transcript container still present, composer enabled after the row flips to `running`, **no POST to `/sessions`** and the listing contains one row.
  2. `pending state`: hold the route; assert `Reconnecting to your conversation…` is visible, buttons hidden, composer disabled, sidebar label `Reconnecting`; no horizontal overflow at 320 and 390 px, light and dark.
  3. `failure then retry`: first call returns `502 {code:'chat_reattach_failed'}` → pane shows the reason, **Try again** and **Start a new chat**; no second automatic call (`requests.reattach` length stays 1); clicking **Try again** sends exactly one more and succeeds.
  4. `no duplicate on double click`: click **Try again** twice rapidly → one POST.
  5. `ended chat keeps the manual path`: `ended` row shows **Continue with history**, no auto POST.
  6. `legacy machine`: existing flow unchanged, and while the create is held the pane shows `Starting a new session…` with the button disabled.
- [ ] **Step 2: Run** `cd web && npm run build && npx vite preview --port 4173 & sleep 3; node tests/chat-recovery.e2e.mjs`. Expected: FAIL before Task 8 is applied, PASS after. (Re-run after Task 8 is in.)
- [ ] **Step 3: Fix** any UI gaps the contract finds in Task 8's files (keep fixes there).
- [ ] **Step 4: Run** `node tests/chat-recovery.e2e.mjs && node tests/chat.e2e.mjs && node tests/sessions.e2e.mjs`. Expected: all PASS (no regression in the existing chat contracts).
- [ ] **Step 5: Commit** `git add web/tests && git commit -m "Cover in-place chat reattach in the browser contract"`.

### Task 10: Docs and handoff

**Files:**
- Modify: `docs/structured-sessions.md` (section "Reopening a reet session and history", lines 182-204; capability list near lines 262 and 309)
- Modify: `handoff.md` (new `CHAT-REATTACH-1` entry above `CHAT-RECOVERY-1`, listing checks run)
- Modify: `README.md` only if it names "Continue with history" (`grep -n "Continue with history" README.md`)

- [ ] **Step 1: Edit** `structured-sessions.md`: document `chat-reattach-v1`, automatic in-place reconnect from `lost`/`exited`, the **Reconnect** and **Try again** copy, that `ended` chats keep **Continue in Gaffer with history…**, and that older connectors keep the manual new-row path with its single-flight guard.
- [ ] **Step 2: Add** the handoff entry with PR link, commands run and results (fill with real output).
- [ ] **Step 3: Run** the full checks: `go vet ./... && go test -race ./...` and `cd web && npm run check && npm run build`. Expected: all PASS.
- [ ] **Step 4: Commit** `git commit -am "Document in-place chat reattach"`.
