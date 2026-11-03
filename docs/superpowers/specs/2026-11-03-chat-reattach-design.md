# Seamless reattach of a lost structured chat

Status: implemented (PRs #451 and #459); see the plan's Deviations for the three recorded changes. Date: 2026-11-03.

## Problem

When reet loses contact with a Gaffer (OpenCode) chat, the session shows "This reet session has ended" with a
**Continue with history** button. In practice:

1. The button creates a **new** session row (`createTerminal` with `resumeAcp`), reserved on the controller before the
   machine answers. A refusal (`conversationInUseLocked` → 409, a failed `session/load`, a timeout) leaves that row in
   `creating` or `lost`. These are the stale rows visible in the sidebar.
2. The new row inherits the same `acp_session_id`, and `CanResumeACPToolSessionFor` accepts any `lost`/`exited`/`ended`
   row, so the new row can itself be continued. Repeated clicks produce many sessions.
3. `continueSession` (`TerminalWorkspace.svelte`) switches view only on success and sets `error` where the user is not
   looking. The user stays on the dead session with no feedback.
4. The machine already recovers a chat **in place** after a connector restart (`resumeChat`, same reet session id, replay
   dropped), and the store already allows `lost → running` for a structured chat (`ObserveTerminalStateFor`). Only the
   controller-initiated trigger is missing.

## Goal

A lost chat comes back by itself, in the same row, same URL and same transcript, with a visible "Reconnecting…" state.
No second row can be created. A failure says why and offers one retry in place. Agents' own continue mechanism is used:
ACP `session/load` on OpenCode's saved session.

## Scope

- OpenCode (Gaffer) structured chats on a machine that negotiates the new feature.
- Reattach from `lost`, and from `exited` when the chat process died without a person ending it.
- User-ended (`ended`) chats keep **Continue in Gaffer with history…** (a deliberate new row). The machine answers a
  person's End of a chat with `exited` (code 0), so the store records that close of an `end_requested` structured row as
  `ended`; rows ended earlier are stored `exited` (or `lost`) and are refused by their end operation (final review fix).
- Out of scope: ACP `session/resume` without replay, Claude/Codex (history unverified), terminal-mode sessions,
  recovering an interrupted reply (a new prompt is still needed, as today).

## Design

### 1. Protocol

- New feature `chat-reattach-v1`, requiring `acp-v1`, `acp-history-v1`. Advertised and negotiated beside
  `session-mode-v1` (`internal/api/private_control.go`, `internal/agenthealth/agent.go`).
- New control body `chat_reattach {tool, dir, model?, yolo?, acpSessionId, title?, trust?}` (the row's launch data, like
  `session_mode`, so the machine does not depend on its restart registry still holding a record) added to the closed
  body set and to `Message.Validate` (`internal/terminalproto/protocol.go`). Reply: the existing `session_changed`.
- The strict decoder rejects unknown kinds, so the body is never sent to a peer that did not negotiate the feature. Older
  peers keep the manual path (section 5).

### 2. Machine (`internal/terminalagent`)

Handler `reattach(requestID, sessionID, body)` (`internal/terminalagent/chat_reattach.go`), driven by the launch data in the
`chat_reattach` body, not by the restart registry (which may no longer hold a record):

1. Refuse any tool but OpenCode.
2. If a live process already owns the conversation under this session id, **adopt it** and report `running`. This replaces
   today's 409 for the same row. A different session holding the conversation, a start or swap in flight for this id, or a
   retired process that may still be alive conflicts.
3. Otherwise drop any stale `exited` receipt for the id and run the existing `openChat` path with `ResumeACP` set: same
   reet id, folder, model, YOLO and trust; `session/load`; replay dropped.
4. Idempotent on the request id; a repeated request answers from the first attempt's receipt.
5. A failed start reports the reason (version, trust, load refusal) and leaves nothing running.

### 3. Controller (`internal/terminalbroker`, `internal/store`)

- `Peer.reattach(ctx, session, request)`: requires state `lost` or `exited` (both are accepted), `structured`, `tool=opencode`,
  no end operation for the row (`chat_reattach_ended` otherwise), a saved `acp_session_id`, the account's Allow-YOLO
  setting for a YOLO row, and `chat.History` on the current peer generation. No row is created.
- The attempt is not persisted: single-flight lives in controller memory (a map beside `switching`), not in a
  `terminal_operations` row. A controller restart mid-attempt leaves the row `lost`; the next automatic attempt re-sends
  and the machine adopts the live process.
  **No new `SessionState`.** Session state stays `lost` until the machine reports `running`
  (`lost → running` already permitted for structured chats). `exited → running` is added for structured chats only,
  with the same guard.
- Single-flight per row (in memory): a second request while one is in flight joins it and gets its result.
- `CanResumeACPToolSessionFor` stops accepting a continuation row whose create operation failed, so it cannot itself be
  continued, closing the many-rows path in the legacy flow.
- The session payload has no `reattach` field. The HTTP call is held until the machine answers (like create and
  `session_mode`); the browser that made it shows "Reconnecting…", and other tabs see `lost → running` through normal
  polling.

### 4. Web (`ChatPane.svelte`, `TerminalWorkspace.svelte`, `chat.ts`)

- The selected `lost` or `exited` OpenCode chat triggers ONE automatic reattach per session id per page load, once the
  machine is `ready`, advertises `chatReattach` and reports history support (`canReattachChat`). The attempt is recorded
  when it starts and is never repeated, whether it succeeds or fails: after a failure only the person retries. If the
  machine is not ready when the chat is first shown, nothing is attempted and the attempt is made the first time the
  machine becomes ready within the same page load. It is also not made while a legacy Continue with history of the same
  chat is in flight.
- An offline machine (reattach needs `machine.ready`) shows the idle ended note with the legacy **Continue with history**
  disabled, not a waiting state and not a banner. A reattach that fails with `ErrOffline` (503 "Reconnecting took too
  long. Try again.") consumes the automatic attempt; **Try again** and **Reconnect** still work.
- While pending: an inline status line "Reconnecting to your conversation…" in the pane; transcript stays visible; the
  composer is disabled with that reason. The "Earlier messages are no longer available" error is shown only if replay
  actually truncated.
- On success the same view continues: no navigation, no new tab, no new row.
- On failure: the reason in the pane (not a global error, announced as an alert) and **Start a new chat**, plus **Try
  again** only for a refusal a retry can fix (keyed off the route code: not for YOLO refusals, capacity, trust, ended,
  unsupported or no conversation). A folder-trust refusal shows the plugin files and offers the explicit **Continue with
  history** (new row, `trust: true`) again, in the pane and the header menu (ruling R10). An automatic attempt refused
  as `chat_reattach_ended` falls back quietly to the ended note with Continue with history. A manual Reconnect or Try again refused this way shows the failure card with the server's message, Continue with history and Start a new chat, and no Try again.
- The button copy changes from "Continue with history" to **Reconnect** for this path. The sidebar shows "Reconnecting"
  rather than "Lost" while pending.

### 5. Older connectors / controllers

A peer without `chat-reattach-v1` keeps today's **Continue with history** (new row). It gains: single-flight per saved
id in the UI, the old row's pane showing "Starting a new session…" until the create returns (then the browser switches
to the new row; ruling R9), and failures shown in the pane.
Failed or refused continuation rows are failed definitively rather than left `creating`.

### 6. Error handling

| Case | Result |
|---|---|
| Machine offline | The pane shows the idle ended note, not a waiting state. A failed automatic attempt consumes the one attempt; Try again and Reconnect still work. |
| OpenCode store missing / `session/load` refused | `failed` with reason; Try again and Start a new chat. |
| Live process already owns the chat | Adopted, state `running`. |
| Another row owns the conversation | `failed`: "This conversation is open in another session." |
| The machine is still starting the chat | `chat_reattach_starting`: "Still reconnecting. Try again in a moment." (the machine's conflict result carries the detail `starting`). |
| Account does not allow YOLO / machine YOLO consent off | `chat_reattach_yolo_not_allowed` / `chat_reattach_yolo_refused`; no Try again. |
| Machine at its session limit | `chat_reattach_capacity`: "This machine has reached its session limit."; no Try again. |
| Folder has OpenCode plugins | `chat_reattach_trust` with the files; Continue with history and Start a new chat, no Try again. |
| Chat was archived (crafted request; archived rows are not listed) | `chat_reattach_archived`: "This chat was archived, so its history is gone."; refused before any machine call, no Try again, no Continue. |
| A person ended the chat | `chat_reattach_ended`: "You ended this chat. Continue it with history as a new session." |
| Reply lost | Operation stays pending; the next call with the same request id checks it. |

## Testing

- Go: reattach state machine (`lost→running`, `exited→running`, refusal of non-structured), single-flight, request-id
  idempotency, adopt-live, no stale `creating` rows after refusal, capability gating, strict decoder with old peers.
- Machine: reattach reuses `resumeChat`, drops replay, quota respected, 409 only for a different row.
- Web unit: `canReattach`, auto-trigger once, no loop after failure, legacy path single-flight.
- Browser contract: extend `web/tests/chat-recovery.e2e.mjs` for auto-reattach in place, failure + retry, offline
  machine, and "exactly one row afterwards".
- Docs: update `docs/structured-sessions.md` "Reopening a reet session and history".

## Open risks

- `exited → running` widens the transition table; the guard (structured, controller-initiated operation only) must be
  covered by tests.
- Auto-reattach on a machine whose OpenCode store was wiped fails fast once and then shows the retry card; acceptable.

Known limitations of what was built:

- Auto-reattach sends no folder-trust confirmation, so a folder with OpenCode plugins refuses with the trust message; the
  person continues with history as a new row (which confirms trust) or starts a new chat. A trust body on the route is a
  follow-up.
- YOLO chats are refused (not downgraded) if the account no longer allows YOLO or local YOLO consent is off.
- While the machine is offline the pane shows the idle ended note (not a waiting state), and a failure consumes the one
  automatic attempt per session per page load.
- Claude and Codex chats are not covered.
