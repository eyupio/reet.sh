# Live reet TUI / OpenCode TUI Toggle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the header toggle switch a live OpenCode session between the reet TUI (ACP chat) and the OpenCode TUI (tmux terminal) on the same session row.

**Architecture:** A new negotiated feature `session-mode-v1` adds one control body, `session_mode`. The connector swaps the process atomically (stop old, purge old entry, start new under the same id) so the controller row stays `running`. The controller gains one store writer for `structured`, one broker method, one API route; the web toggle calls it.

**Tech Stack:** Go (controller `internal/api`, `internal/terminalbroker`, `internal/store`; connector `internal/terminalagent`, `internal/terminalhost`, `internal/agenthealth`; wire `internal/terminalproto`), Svelte 5 + Playwright `.mjs` e2e in `web/`.

**Spec:** `docs/superpowers/specs/2026-11-01-live-tui-toggle-design.md`

## Global Constraints

- OpenCode only; `ReviewedChatTool(tool)` must hold. Reuse the OpenCode floor `1.19.5`; no new version check.
- Round-trip scope: switching **to chat** requires a non-empty `acp_session_id` on the row. A terminal-started row keeps the reet TUI button disabled with a reason.
- Switch allowed only while idle: no running turn, no pending approval, no pending question.
- `session-mode-v1` requires `acp-v1`, `acp-history-v1` and `launch-v1`. The body is never sent to a peer that did not negotiate it; the connector returns `terminalproto.ErrInvalid` for it when not negotiated (same pattern as `acp_mode`).
- The strict decoder uses `DisallowUnknownFields` with a closed body set: every new body or field is feature-gated.
- The row's `state` never passes through `ended`/`exited` during a switch (`CanTransition` is one-way).
- Never run two OpenCode processes on one session id: the old process is fully stopped before the new one starts.
- Refusal copy is a full sentence, like the existing `ErrACPModelUnsupported`.
- Both modes count against `MaxSessionsPerAgent` (8); a switch must not transiently count twice.
- Docs: update `docs/structured-sessions.md` and `docs/start-agent-session.md`.
- Repo checks before each push: `go vet ./...`, `go test -race ./...`, `npm run check` in `web/`.

## Review Focus

1. Double click / two overlapping switch requests: second is refused as busy, exactly one process runs (Task 4, Task 5).
2. Row has no `acp_session_id` (terminal-started): refused with the "no known session" sentence, button disabled with reason (Task 2, Task 5, Task 6).
3. A tool-approval card or question is pending when the switch is requested: refused, nothing stopped (Task 4, Task 5).
4. The new process fails to start (OpenCode missing, trust refused): previous mode is restored, else the row ends with the reason (Task 4).
5. Connection lost mid-switch: after reconnect the controller's `structured` matches the mode the connector reports on reconcile (Task 5).

---

### Task 1: Protocol: feature, body and capability

**Files:**
- Modify: `internal/terminalproto/acp.go` (feature constant, `SupportsSessionMode`, `SessionMode` body)
- Modify: `internal/terminalproto/protocol.go` (Capabilities field + validation at ~221, `session` kind list at ~557, decode switch at ~656)
- Test: `internal/terminalproto/acp_test.go`, `internal/terminalproto/protocol_test.go`

**Interfaces:**
- Produces:
  - `const SessionModeFeature = "session-mode-v1"`; `func SupportsSessionMode(features string) bool`
  - `type SessionMode struct { To string \`json:"to"\`; Tool string \`json:"tool"\`; Dir string \`json:"dir"\`; Model string \`json:"model,omitempty"\`; Yolo bool \`json:"yolo,omitempty"\`; ACPSessionID string \`json:"acpSessionId"\`; Title string \`json:"title,omitempty"\`; Trust bool \`json:"trust,omitempty"\`; Cols, Rows int }` with `kind() == "session_mode"`; `To` is `"terminal"` or `"chat"`
  - `Capabilities.SessionMode bool` (`json:"sessionMode,omitempty"`), valid only with `ACP && ACPHistory && TerminalControl`
  - `const (SessionModeTerminal = "terminal"; SessionModeChat = "chat")`

- [ ] **Step 1: Write failing tests** in `acp_test.go`: `TestSessionModeValid` (valid chat and terminal bodies pass; `To:"x"`, empty `ACPSessionID`, bad id via `ValidACPSessionIDFor(tool, id)`, `Trust` with `To:"terminal"`, non-reviewed tool all fail `valid()`); `TestSessionModeRoundTrip` (encode/decode through the message codec; an unknown extra field is rejected); in `protocol_test.go`: `TestCapabilitiesSessionModeNeedsACPHistoryAndLaunch`.
- [ ] **Step 2: Run** `go test ./internal/terminalproto/ -run 'SessionMode' -v`; expect FAIL (undefined).
- [ ] **Step 3: Implement** the constants, helper (copy the `SupportsACPApproval` shape), `SessionMode` with `valid()`, the `Capabilities.SessionMode` rule next to the existing `c.ACPApproval && !c.ACP` line, `session_mode` in the `session` kind list in `Message.Validate`, and its case in the body decode switch.
- [ ] **Step 4: Run** `go test ./internal/terminalproto/...`; expect PASS. Fix any test enumerating message kinds.
- [ ] **Step 5: Commit** `git add internal/terminalproto && git commit -m "Add session_mode protocol body and feature"`

### Task 2: Store: mutable `structured`

**Files:**
- Modify: `internal/store/store_terminal.go` (new method beside `SetTerminalYoloFor`, ~877)
- Test: `internal/store/store_terminal_test.go`

**Interfaces:**
- Produces: `func (s *Store) SetTerminalStructuredFor(uid int64, sessionID string, structured bool) error`. Errors: `ErrTerminalNotFound` for an unknown or other-account session (use the file's existing not-found error); `ErrTerminalConflict` when `structured` is true and `acp_session_id` is empty, or the row's state is not `running`. A no-op (already that value) returns nil. It does not touch `state`, `model`, `yolo`, `dir` or `acp_session_id`.

- [ ] **Step 1: Write failing tests**: `TestSetTerminalStructuredForFlipsRunningRow` (create structured row with id, running; set false → `Structured` false, id and state unchanged; set true → back), `TestSetTerminalStructuredForRefusesWithoutACPSessionID` (terminal row, set true → `ErrTerminalConflict`), `TestSetTerminalStructuredForRefusesClosedRow`, `TestSetTerminalStructuredForOtherAccount`.
- [ ] **Step 2: Run** `go test ./internal/store/ -run SetTerminalStructuredFor -v`; expect FAIL.
- [ ] **Step 3: Implement** with a single guarded `UPDATE ... WHERE id=? AND uid=? AND state='running'` (and the id predicate when enabling), then classify zero rows affected by re-reading the row.
- [ ] **Step 4: Run** `go test ./internal/store/...`; expect PASS (existing `TestStructuredSessionPersistsAndReadsBack` stays green).
- [ ] **Step 5: Commit** `git commit -m "Add store writer for terminal session mode"`

### Task 3: Launch preset: open a terminal on a given OpenCode session

**Files:**
- Modify: `internal/terminalproto/protocol.go` (`Create`: add `LaunchSession string \`json:"-"\`` next to `ProjectID`; never encoded)
- Modify: `internal/terminalhost/catalogue.go` (add `SessionArgs []string` to the preset struct and to `launchPreset`; OpenCode: `SessionArgs: []string{"--session"}`)
- Modify: `internal/terminalhost/launch.go` (`launchArgv`)
- Test: `internal/terminalhost/launch_test.go`

**Interfaces:**
- Produces: `launchArgv` appends `p.sessionArgs..., in.LaunchSession` after the yolo/safe args and before effort/model/prompt. `LaunchSession` set with a tool whose preset has no `SessionArgs` returns `ErrLaunchOptionUnsupported`; set together with `Resume` returns `terminalproto.ErrInvalid` (`Create.Validate` rule: `LaunchSession` requires `ValidACPSessionIDFor(Tool, LaunchSession)` and `!Resume && !Structured`).

- [ ] **Step 1: Write failing tests**: `TestLaunchArgvOpenCodeSession` expects `[path, "--session", "ses_abc…"]` plus `--model m` and `--auto` when `Yolo`; `TestLaunchArgvSessionRefusesResumeAndStructured`; `TestLaunchArgvSessionUnsupportedTool`.
- [ ] **Step 2: Run** `go test ./internal/terminalhost/ -run LaunchArgv -v`; expect FAIL.
- [ ] **Step 3: Implement** as described. Before finishing, run `opencode --help` on a machine that has OpenCode 1.19.5 if available; if `--session` is not accepted, stop and report (Spec "Open risks").
- [ ] **Step 4: Run** `go test ./internal/terminalhost/... ./internal/terminalproto/...`; expect PASS.
- [ ] **Step 5: Commit** `git commit -m "Launch OpenCode terminal on a given session id"`

### Task 4: Connector: atomic mode swap

**Files:**
- Modify: `internal/terminalhost/manager.go` (add `Forget`)
- Modify: `internal/terminalagent/chat.go` (add `forget`, `idle`)
- Create: `internal/terminalagent/session_mode.go` (the swap)
- Modify: `internal/terminalagent/agent.go` (feature check beside `ACPMode` at ~253; dispatch beside `Create`/`End` at ~500)
- Test: `internal/terminalagent/session_mode_test.go` (reuse `fakeChat` / `testChatHost` from `chat_test.go`)

**Interfaces:**
- Consumes: Task 1 `SessionMode`, Task 3 `Create.LaunchSession`, existing `chatHost.open/end/snapshot`, `Manager.Create/End/List`.
- Produces:
  - `func (m *Manager) Forget(ctx context.Context, sessionID string) error`: removes an ended/exited/lost session and its operation receipts from the registry; `ErrOperationConflict`-style refusal for a session that still reserves quota.
  - `func (s *chatSupervisor) forget(sessionID string) bool`: drops the ended receipt; false if the chat is not ended.
  - `func (h *chatHost) idle(sessionID string) bool`: no `busy`, no `pending` approval, no pending question.
  - `func (a *agentSwitch) swap(ctx context.Context, requestID, sessionID string, in terminalproto.SessionMode) (terminalproto.SessionChanged, error)` in `session_mode.go`, serialised per session (a second concurrent call returns `ErrOperationConflict`, mapped to the busy refusal).

Behavior of `swap`: reject if current mode already equals `in.To` or a chat is not `idle`; stop old (`chats.end` + wait, or `Manager.End`); `forget` the old entry; start new (`chats.open` with `Create{Structured:true, ResumeACP:in.ACPSessionID, Trust:in.Trust, Tool, Dir, Model, Yolo}` for chat; `Manager.Create` with `Create{Tool, Dir, Model, Yolo, Cols, Rows, Title, LaunchSession:in.ACPSessionID}` for terminal); on start failure, try once to start the previous mode the same way; if that fails too, return the original error and leave the row to end through the normal path. Quota: stop-and-forget happens before the new open, so the count never exceeds the old one.

- [ ] **Step 1: Write failing tests**: `TestSwapChatToTerminalKeepsIDAndReleasesChat`, `TestSwapTerminalToChatResumesStoredSession` (fake host records `Resume == in.ACPSessionID`), `TestSwapRefusedWhileTurnRunning`, `TestSwapRefusedWithPendingApproval`, `TestSwapRefusedWithPendingQuestion`, `TestSwapConcurrentSecondCallRefused` (two goroutines, exactly one starts a process), `TestSwapRestoresPreviousModeWhenNewStartFails`, `TestSwapDoesNotInvalidateReconcile` (after a swap `reconcileSnapshot` has no duplicate id), `TestSwapNotNegotiatedIsErrInvalid`, `TestManagerForgetRefusesLiveSession`.
- [ ] **Step 2: Run** `go test ./internal/terminalagent/ ./internal/terminalhost/ -run 'Swap|Forget' -v`; expect FAIL.
- [ ] **Step 3: Implement** the helpers and `swap`; wire dispatch in `agent.go` in a goroutine tracked with `chats.sup.addWork()` like structured `Create`, replying with `SessionChanged` (or `chatResult(err)`); add the `session_mode` feature check returning `terminalproto.ErrInvalid`.
- [ ] **Step 4: Run** `go test -race ./internal/terminalagent/... ./internal/terminalhost/...`; expect PASS.
- [ ] **Step 5: Commit** `git commit -m "Connector: swap a live session between chat and terminal"`

### Task 5: Negotiation, capability report and controller broker

**Files:**
- Modify: `internal/api/private_control.go:19` (advertise `terminalproto.SessionModeFeature`)
- Modify: `internal/agenthealth/agent.go` (~721 and ~786: negotiate and report `Capabilities.SessionMode`)
- Modify: `internal/terminalbroker/chat.go` (or a new `internal/terminalbroker/session_mode.go`: `SwitchSessionMode`; reconcile adoption near `restoreAgentChats`/`~531`)
- Test: `internal/agenthealth/acp_agents_test.go`, `internal/terminalbroker/session_mode_test.go`

**Interfaces:**
- Consumes: Task 1 body and capability, Task 2 store method, Task 4 connector behavior.
- Produces:
  - `func (b *Broker) SwitchSessionMode(ctx context.Context, uid int64, session, to string, trust bool) error`
  - Errors (full sentences): `ErrSessionModeBusy` ("Wait for the agent to finish or answer what it is asking, then switch."), `ErrSessionModeUnsupported` ("Update reet on this machine to switch between the reet TUI and the OpenCode TUI."), `ErrSessionModeNoSession` ("This session started in the OpenCode TUI, so the reet TUI cannot pick up its conversation."), `ErrSessionModeTrust` (reuse the existing trust refusal sentence), plus existing `ErrOffline`.

Behavior: load the row (`b.peer`, `RemoteCapabilitiesFor`, generation check as in `SetChatYolo`); refuse unless `caps.Capabilities.SessionMode`; to chat requires `s.ACPSessionID != ""` and, when the folder needs trust, `trust`; if the row is structured, refuse with `ErrSessionModeBusy` when the `chatLog` has `running`, `len(pending)>0` or `len(questions)>0`; send `SessionMode` with `callWithin(..., chatCallTimeout)` extended to the 60 s used for chat creates; on ack call `SetTerminalStructuredFor`; on flip to chat call `restoreChat`/reset the ring, on flip to terminal call `closeChat`; add audit event `session-mode` with `{machine, tool, session, mode}`. On reconcile, when a snapshot's mode (non-empty `ACPSessionID` ⇒ chat) differs from `row.Structured`, call `SetTerminalStructuredFor` to match.

- [ ] **Step 1: Write failing tests**: `TestNegotiationAdvertisesSessionMode` and `TestCapabilitiesReportSessionModeOnlyWithHistory` (agenthealth); broker: `TestSwitchToTerminalFlipsRowAndAudits`, `TestSwitchToChatFlipsRowAndRestoresRing`, `TestSwitchRefusedWithoutACPSessionID`, `TestSwitchRefusedWhenBusyPendingApprovalOrQuestion`, `TestSwitchRefusedWhenUnsupportedOrOffline`, `TestSwitchToChatNeedsTrustWhenFolderHasPlugins`, `TestSwitchOtherAccountRefused`, `TestReconcileAdoptsConnectorMode`.
- [ ] **Step 2: Run** `go test ./internal/agenthealth/ ./internal/terminalbroker/ -run 'SessionMode|Switch|Reconcile' -v`; expect FAIL.
- [ ] **Step 3: Implement** as above.
- [ ] **Step 4: Run** `go test -race ./internal/agenthealth/... ./internal/terminalbroker/... ./internal/api/...`; expect PASS.
- [ ] **Step 5: Commit** `git commit -m "Controller: negotiate session-mode-v1 and switch a session"`

### Task 6: API route

**Files:**
- Modify: `internal/api/terminal_chat.go` (route next to line 23; handler `terminalSessionMode`; error mapping next to ~228)
- Test: `internal/api/terminal_chat_test.go`

**Interfaces:**
- Consumes: `Broker.SwitchSessionMode`.
- Produces: `POST /api/v2/terminal/sessions/{id}/mode` with body `{"mode":"terminal"|"chat","trust":bool}`; 204 on success; 400 for a bad `mode`; 409 for `ErrSessionModeBusy` (code `session_mode_busy`) and `ErrSessionModeNoSession` (`session_mode_no_session`); 409 `acp_version`-style code `session_mode_unsupported` for `ErrSessionModeUnsupported`; trust refusal reuses its existing code; unknown or other-account session 404. Error body shape follows `writeErrCode`.

- [ ] **Step 1: Write failing tests**: `TestSessionModeEndpointSwitchesAndReturnsNoContent`, `TestSessionModeEndpointRejectsBadMode`, `TestSessionModeEndpointMapsRefusals` (table over the five errors with their status and code), `TestSessionModeEndpointRequiresWriter`.
- [ ] **Step 2: Run** `go test ./internal/api/ -run SessionModeEndpoint -v`; expect FAIL.
- [ ] **Step 3: Implement** the route and handler wrapped in `s.terminal(...)` like its neighbours.
- [ ] **Step 4: Run** `go test -race ./internal/api/...`; expect PASS.
- [ ] **Step 5: Commit** `git commit -m "Add session mode API route"`

### Task 7: Web toggle, confirm dialog, e2e and docs

**Files:**
- Modify: `web/src/lib/terminal/api.ts` (`setSessionMode`, capability field `sessionMode`)
- Modify: `web/src/lib/sessions/SessionScreen.svelte` (~32-35 and ~232-237)
- Modify: `web/src/lib/terminal/TerminalWorkspace.svelte` (~467-471 `{#key}`; handler wiring)
- Create: `web/src/lib/sessions/SwitchModeDialog.svelte`
- Modify: `web/tests/sessions.e2e.mjs:631-640,1073`, `web/tests/chat.e2e.mjs:96-97,291-297`
- Test: `web/src/lib/sessions.test.mjs` (pure helper), new cases in the two e2e files
- Docs: `docs/structured-sessions.md`, `docs/start-agent-session.md`

**Interfaces:**
- Produces:
  - `export async function setSessionMode(id: string, mode: 'terminal' | 'chat', trust = false): Promise<void>` in `api.ts` (POST to `/api/v2/terminal/sessions/{id}/mode`).
  - `export function switchModeState(session: TerminalSession, caps: Capabilities | undefined, busy: boolean): { to: 'terminal' | 'chat'; enabled: boolean; why: string }` in `web/src/lib/sessions.ts`. Disabled reasons (exact copy): busy → "Wait for the agent to finish or answer what it is asking."; no `acpSessionId` when going to chat → "This session started in the OpenCode TUI, so the reet TUI cannot pick up its conversation."; machine without `sessionMode` → "Update reet on this machine to switch between the reet TUI and the OpenCode TUI."; session not live → "Only a running session can switch."
  - `{#key session.id + ':' + (session.structured ? 'chat' : 'term')}` so the pane remounts on a flip.

- [ ] **Step 1: Write failing tests**: `sessions.test.mjs` table for `switchModeState` (one row per disabled reason and the enabled case, both directions); update `sessions.e2e.mjs` "Chat is disabled with its reason" to the new rule (reet-started session: the inactive button is enabled; a terminal-started session: reet TUI `aria-disabled=true` with the no-session sentence); add e2e `toggle switches a live chat to the OpenCode TUI and back` (route `POST /mode`, asserts the confirm dialog text, then that `TerminalPane`/`ChatPane` swap and `aria-pressed` follows), `switch is disabled while an approval card waits`, `trust confirmation is required to return to the reet TUI when the folder has plugins`. Update `chat.e2e.mjs` assertions at 96-97 and the continue-menu summary logs.
- [ ] **Step 2: Run** `cd web && node src/lib/sessions.test.mjs` and the two e2e files with `node tests/sessions.e2e.mjs` / `node tests/chat.e2e.mjs`; expect FAIL on the new/changed cases.
- [ ] **Step 3: Implement** the helper, API call, dialog (states: one process stops and another starts; terminal scrollback is not kept; shows **I trust this folder** when switching to chat and the folder needs it), enabled buttons with `aria-disabled`/`title`/`aria-describedby` taken from `switchModeState().why`, refresh of the session list after success, and busy state while the request runs (buttons disabled, so a double click sends one request).
- [ ] **Step 4: Run** `cd web && npm run check && npm test` and the e2e files above; expect PASS. Also check light and dark, phone width (`sessions-phone.e2e.mjs`, `phone-dialogs.e2e.mjs`).
- [ ] **Step 5: Docs and commit**: in `structured-sessions.md` replace "the agent TUI is disabled because there is no shell to attach to" with the toggle behavior, scope (reet-started sessions), idle-only rule, scrollback and durability limits; mirror in `start-agent-session.md`. `git add -A web docs && git commit -m "Web: live reet TUI / OpenCode TUI toggle"`

---

## Final verification

- [ ] `go vet ./... && go test -race ./...` and `cd web && npm run check && npm test` all pass.
- [ ] Manually, with a real OpenCode ≥ 1.19.5 on a connected machine: start a reet TUI chat, send a message, switch to the OpenCode TUI (conversation visible), switch back (history replays), confirm no duplicate `opencode` processes (`pgrep -fa opencode`).
- [ ] Push and mark PR ready only after the user asks.
