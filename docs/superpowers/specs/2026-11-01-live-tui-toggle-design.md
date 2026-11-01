# Live toggle between the reet TUI and the OpenCode TUI

Status: design, awaiting review. Date: 2026-11-01.

## Goal

The **reet TUI / OpenCode TUI** toggle in the session header becomes a working control on a live session. Switching keeps
the same session row (title, folder, model, OpenCode conversation). Today the toggle is inert: a row's `structured`
flag is set once at creation and the inactive button is `aria-disabled`.

## Scope

- OpenCode only, on a machine that negotiates the new feature.
- **Round-trip for reet-started sessions.** A session that began as a reet TUI chat can switch to the OpenCode TUI and
  back, because the row already stores OpenCode's session id (`acp_session_id`). A session that began as an OpenCode TUI
  keeps the reet TUI button disabled with a reason, because nothing records its OpenCode session id.
- The switch is allowed only while the agent is idle: no turn running, no approval card or question pending.
- Out of scope: discovering the OpenCode session id of a terminal-started session; carrying terminal scrollback across;
  other agents.

## Design

### 1. Protocol

- New feature `session-mode-v1`, requiring `acp-v1`, `acp-history-v1` and `launch-v1`. The controller advertises it in
  `internal/api/private_control.go`; the connector negotiates it in `internal/agenthealth/agent.go`.
- New control body `session_mode {id, to: "terminal" | "chat", trust, cols, rows}`, added to the closed body set and to
  `Message.Validate` in `internal/terminalproto/protocol.go`. The reply is the existing `session_changed`.
- The strict decoder rejects unknown kinds, so the body is never sent to a peer that did not negotiate the feature. An
  older connector or controller keeps the current "Continue as a new row" flow and the toggle shows why it is disabled.

### 2. Connector hand-off (`internal/terminalagent`)

The swap is atomic on the connector so the controller never observes `ended`/`exited` (the state machine in
`CanTransition` is one-way).

1. Reject if the session is not in the expected mode, or a turn, approval or question is in flight.
2. Stop the old process completely. Two OpenCode processes must never drive one session database.
   - chat to terminal: `chats.end` and wait for the process group to close.
   - terminal to chat: end the tmux session.
3. Purge the old entry (tmux registry entry or chat ended receipt) so the id is unique again. Without this, `Manager.Create`
   and `chats.open` refuse the reused id and a duplicate id invalidates the whole `Reconcile` message.
4. Start the new process under the same id.
   - to terminal: `opencode --session <ses_…>` with `--model` and, when the row is YOLO, `--auto`. This adds a
     session-id launch preset beside the existing static `ResumeArgs: ["--continue"]` in `terminalhost/catalogue.go`.
   - to chat: `chats.open` with `Resume` set to the stored id (`session/load`), `Trust` as supplied.
5. If the new process fails to start, try once to restore the previous mode; if that also fails, end the row with a
   sentence-shaped reason.
6. Both modes count against `MaxSessionsPerAgent`; the swap must not transiently count twice.

### 3. Controller and store

- New transactional store method (for example `SetTerminalStructuredFor`) is the only writer of `structured` after
  creation. It does not change `state`. `tool`, `dir`, `model`, `yolo` and `acp_session_id` carry over.
- Switching to chat requires a non-empty `acp_session_id` on the row. This is the rule that limits v1 to reet-started
  sessions; no new column is needed.
- `CanResumeACPToolSessionFor` and `RecordTerminalACPSessionFor` keep their structured-only guards; a row in terminal mode
  retains the id it already has.
- The in-memory chat ring, pending permissions and question state for the session are reset on the flip, and refilled by
  the `session/load` history replay when switching to chat.

### 4. API

`POST /api/v2/terminal/sessions/{id}/mode {mode, trust}`. Refusals use the existing sentence-shaped errors:

- agent busy (turn running, approval or question pending),
- machine did not negotiate `session-mode-v1`,
- no known OpenCode session id,
- trust required (the folder has `.opencode` plugins) and not supplied,
- unknown session or session of another account.

### 5. Web

- `SessionScreen.svelte`: both header buttons become real controls with click handlers. Disabled reasons: busy, no known
  session id, machine too old, trust needed. `CHAT_WHY` and `TERMINAL_WHY` become state-dependent.
- A confirmation dialog states that one process is stopped and another started, that terminal scrollback is not kept, and
  (to chat) shows the existing **I trust this folder** confirmation when needed.
- `TerminalWorkspace.svelte`: key the pane `{#key session.id + mode}` so `ChatPane` and `TerminalPane` actually swap.
  Approvals UI, status pill and `decisionsLine` follow the new mode.
- `web/src/lib/terminal/api.ts`: new `setSessionMode` call; refresh the session list after it.

### 6. Failure and durability

- A session switched into the reet TUI is as durable as any chat: it is lost if the agent process restarts; a terminal
  session survives in tmux.
- Terminal scrollback is not carried into the chat, and the chat transcript is rebuilt from OpenCode's store.
- Losing the control connection mid-switch: the connector finishes or rolls back locally and reports the resulting mode on
  reconcile; the controller trusts the reconcile snapshot.

## Testing

- Go: protocol round-trip and strict-decode entries; feature negotiation; store method (state unchanged, `structured`
  flips, refuses without an id); broker gating (busy, capability, trust, other account); connector hand-off using the
  `fakeChat` / `testChatHost` harness, including duplicate-id purge and failed-start rollback; launch argv for the
  session-id preset.
- Web: update `sessions.e2e.mjs` (reet TUI disabled with its reason) and `chat.e2e.mjs` (`aria-disabled` on OpenCode
  TUI), which assert the old behaviour; add toggle, disabled-reason and confirm-dialog cases; unit tests for the new API
  helper.
- Docs: update `docs/structured-sessions.md` and `docs/start-agent-session.md`.

## Open risks

- `opencode --session <id>` is documented in the probe report for 1.19.5 but should be re-verified against the real
  binary during implementation.
- OpenCode project plugins run when a session opens; the trust confirmation is mandatory when switching to chat.
