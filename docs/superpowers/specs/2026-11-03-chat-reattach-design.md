# Seamless reattach of a lost structured chat

Status: design, awaiting review. Date: 2026-11-03.

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
- User-ended (`ended`) chats keep **Continue in Gaffer with history…** (a deliberate new row).
- Out of scope: ACP `session/resume` without replay, Claude/Codex (history unverified), terminal-mode sessions,
  recovering an interrupted reply (a new prompt is still needed, as today).

## Design

### 1. Protocol

- New feature `chat-reattach-v1`, requiring `acp-v1`, `acp-history-v1`. Advertised and negotiated beside
  `session-mode-v1` (`internal/api/private_control.go`, `internal/agenthealth/agent.go`).
- New control body `chat_reattach {id}` added to the closed body set and to `Message.Validate`
  (`internal/terminalproto/protocol.go`). Reply: the existing `session_changed`.
- The strict decoder rejects unknown kinds, so the body is never sent to a peer that did not negotiate the feature. Older
  peers keep the manual path (section 5).

### 2. Machine (`internal/terminalagent`)

Handler `reattach(id)`:

1. Look up the chat record (registry) for the id; unknown → `not_found`.
2. If a live process already owns the conversation, **adopt it** and report `running`. This replaces today's 409 for the
   same row. A different row holding the conversation still conflicts.
3. Otherwise run the existing `resumeChat` path: same reet id, folder, model, trust; `session/load`; replay dropped.
4. Idempotent on the request id; a repeated request returns the current state.
5. A failed start reports the reason (version, trust, load refusal) and leaves nothing running.

### 3. Controller (`internal/terminalbroker`, `internal/store`)

- `Peer.reattach(ctx, session, request)`: requires state `lost` (or `exited` per Scope), `structured`, `tool=opencode`,
  a saved `acp_session_id`, and `chat.History` on the current peer generation. No row is created.
- The attempt is a `terminal_operations` row, kind `reattach`, `pending` → `succeeded|failed` with code and reason.
  **No new `SessionState`.** Session state stays `lost` until the machine reports `running`
  (`lost → running` already permitted for structured chats). `exited → running` is added for structured chats only,
  with the same guard.
- Single-flight per row: a second request while one is pending returns the pending operation. A request id reuses the
  same operation.
- `CanResumeACPToolSessionFor` stops accepting a row that is itself the product of a continuation that has not
  succeeded, closing the many-rows path in the legacy flow.
- The session payload exposes `reattach: {state: "pending"|"failed", reason}` derived from the latest operation.

### 4. Web (`ChatPane.svelte`, `TerminalWorkspace.svelte`, `chat.ts`)

- A chat that is `lost` on an online machine with `canReattach` triggers one automatic reattach on view or on machine
  reconnect. It does not loop: after a failure only the person retries.
- While pending: an inline status line "Reconnecting to your conversation…" in the pane; transcript stays visible; the
  composer is disabled with that reason. The "Earlier messages are no longer available" error is shown only if replay
  actually truncated.
- On success the same view continues: no navigation, no new tab, no new row.
- On failure: the reason in the pane (not a global error) and **Try again** plus **Start a new chat**.
- The button copy changes from "Continue with history" to **Reconnect** for this path. The sidebar shows "Reconnecting"
  rather than "Lost" while pending.

### 5. Older connectors / controllers

A peer without `chat-reattach-v1` keeps today's **Continue with history** (new row). It gains: single-flight per saved
id in the UI, switching to the new row immediately with a "Starting…" pane, and failures shown in the pane.
Failed or refused continuation rows are failed definitively rather than left `creating`.

### 6. Error handling

| Case | Result |
|---|---|
| Machine offline | Pending until it returns; banner says the machine is offline. No attempt is consumed. |
| OpenCode store missing / `session/load` refused | `failed` with reason; Try again and Start a new chat. |
| Live process already owns the chat | Adopted, state `running`. |
| Another row owns the conversation | `failed`: "This conversation is open in another session." |
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
