# Program 2, slice 1: structured OpenCode chats

Plan for the first structured-session slice after the models UI (#255). The probe is
[probe-report.md](../../research/probe-report.md); the owner's decision is trusted workspaces, OpenCode only, Allow once / Deny
only, behind all seven conditions. One PR: the machine-side ACP supervisor, the private `acp-v1` protocol, the controller relay,
the Chat tab, and the docs.

## Global constraints

- `opencode acp` runs on the machine, as the connector's user, in the session's folder. The controller never runs an agent,
  never sees OpenCode's own store, and never forwards raw ACP JSON to the browser.
- Only `opencode-ai@1.19.5` is accepted. A different version is refused before a process starts.
- The seven conditions are mandatory: reet config + environment scrub + mode `build`; a random
  `OPENCODE_SERVER_PASSWORD`; `task: deny`; permission and turn deadlines with reet closing its own cards; the environment
  allowlist and opt-outs; trusted folders with a warning for `.opencode` plugins; the exact version pin.
- Allow once and Deny only. The browser never offers Always allow, and the controller drops any other ACP option.
- One process per chat. Closing the session, cancelling, a turn deadline or losing the controller connection kills its process
  group by PID. Nothing survives as a hidden ACP process.
- Structured sessions are ordinary `terminal_sessions` rows with `structured=1`, created through the existing quota,
  idempotency, project, list and end paths. Chat events are bounded memory in the controller, not stored transcripts.
- Chat is only offered for OpenCode on a machine that advertises `acp`. The Start dialog gets a Chat choice; the session screen's
  Chat tab is enabled for a structured session and its Terminal tab is disabled.
- CI: new browser cases go in `phone-dialogs`/`phone-zoom`, never `terminal-browser`. Go tests use a fake ACP child; no real
  provider or OpenCode install is needed.
- TDD, failing-first; mutation checks restored byte-identically (`cp`/`cmp`); task and broad reviews; the last task updates
  `handoff.md`, `docs/terminal-workspace.md`, `docs/structured-sessions.md` and `docs/ui-verification.md`.

## P1 — protocol and store

**Files:** `internal/terminalproto/acp.go` (+test), `internal/terminalproto/protocol.go`, `launch.go`,
`internal/store/store_terminal.go` (+test), `internal/terminalproto/agents.go` if the feature list lives there.

- `ACPFeature = "acp-v1"`, `SupportsACP`.
- `Capabilities.ACP` is sent only on a connection that negotiated `acp-v1`; validation requires terminal control.
- `Create.Structured` (only with `Tool=="opencode"`, an absolute or empty `Dir`, no worktree/resume/yolo/base/effort).
- Bodies: `acp_prompt{text}`, `acp_cancel{}`, `acp_permission{permissionId,allow}` and server `acp_event{kind,...}`.
  Bounded plain text, IDs and lists; no raw ACP JSON.
- Result codes `acp_unavailable`, `acp_version`, `acp_trust_required` with a plain detail. The API maps each to a sentence.
- `terminal_sessions.structured` column, `TerminalSession.Structured`, copied through reserve/read/list.

**Tests:** feature gating; create rules; every new body round-trips and rejects oversized/control input; migration from a
database without the column; structured survives reserve/read.

## P2 — machine-side ACP supervisor

**Files:** new `internal/opencodeacp/` (`config.go`, `env.go`, `client.go`, `session.go`, `*_test.go`).

- `ProbeVersion` runs the connector-owned binary with the minimal environment and requires `1.19.5`.
- `Plugins(dir)` reports `.opencode/plugin*` and `.opencode/package.json`.
- `Start` builds `OPENCODE_CONFIG_CONTENT` (top-level and build/plan/general/explore ask for edit, bash and external_directory;
  task/webfetch deny; default_agent build; autoupdate false; share disabled), `OPENCODE_DISABLE_PROJECT_CONFIG=1`, a random
  `OPENCODE_SERVER_PASSWORD`, the fetch/autoupdate/plugin/LSP/Claude/external-skill opt-outs, and an allowlist environment.
- It removes every inherited `OPENCODE_*`; no SSH_AUTH_SOCK, DISPLAY, DBus, GitHub/cloud tokens.
- JSON-RPC line client: initialize, session/new, mode `build` check, prompt, cancel, request-permission, fs mirror response,
  close. Lines capped at 1 MiB; stderr capped and never forwarded.
- Permission deadline (5 min), turn watchdog (20 s “still working”, 10 min cancel), permission cards closed on cancel/timeout,
  unknown option -> Deny, `once`/`reject` only.
- Process group kill on close; no orphan after cancel.

**Tests:** a fake ACP child (the test binary) for initialize/new/prompt/stream/permission allow-deny/cancel/timeout/version/plugin
warning; environment has no secret and no inherited OPENCODE var; mode refusal; process-group cleanup.

## P3 — connector

**Files:** `internal/terminalagent/agent.go` (+test), `internal/agenthealth/agent.go` (feature lists), `cmd` wiring if needed.

- `Factory` owns a bounded chat map for the connection. `Create.Structured` requires `acp-v1` and OpenCode; it resolves the
  folder, checks version/plugins, starts ACP, emits `session_changed{running}`. A structured create with `Trust=false` and
  plugins answers `acp_trust_required` before reserving anything.
- `acp_prompt`, `acp_cancel`, `acp_permission` dispatch to the chat; `end` closes it; attach/resize/history answer unavailable.
- ACP events are sent as `acp_event` messages on the session id; permission events carry the command or whole-file diff.
- Reconcile merges live chats with the tmux snapshot. Connection cleanup kills every chat process group.
- Capability `acp` is true only when `acp-v1` was negotiated and OpenCode is installed.

**Tests:** fake supervisor/open; create/event/prompt/cancel/permission/end; feature refusal; reconcile includes a live chat;
cleanup kills it; no tmux needed for the chat path.

## P4 — controller broker, store and API

**Files:** new `internal/terminalbroker/chat.go` (+test), `broker.go`, `internal/api/terminal_chat.go` (+test),
`internal/api/terminal.go`, `internal/api/private_control.go`.

- Broker chat log per (uid, session): bounded ring (1,000 events / 256 KiB), sequence, reset/truncated marker, subscribers.
  A machine `acp_event` is published; a late event after the ring is full drops the oldest and marks truncation.
- `Create` refuses `Structured` unless the peer advertises `acp`; maps `acp_version`, `acp_trust_required` to typed errors.
- Chat methods: prompt, cancel, permission (Allow once/Deny), all authorized against the account's running structured session
  and the current peer generation. The broker appends the local user message and permission resolution.
- SSE: `GET /api/v2/terminal/sessions/{id}/chat`; commands: `POST .../chat/prompt`, `/cancel`,
  `/permissions/{permissionId}`. Cookie-only, canonical origin, no URL credentials, no-store. A non-structured session is 404.
- `structured` is included in the session JSON; the archive/end paths are unchanged.

**Tests:** ring overflow/reset; subscribers see events and a reconnect replays; prompt/cancel/permission authorization;
wrong user/machine; non-structured 404; ACP errors mapped; SSE framing (unit with httptest).

## P5 — web

**Files:** `web/src/lib/terminal/api.ts`, `web/src/lib/chat.ts` (+test), new
`web/src/lib/terminal/ChatPane.svelte`, `web/src/lib/sessions/SessionScreen.svelte`,
`web/src/lib/terminal/TerminalWorkspace.svelte`, `web/src/lib/StartAgentDialog.svelte`,
`web/src/lib/agentLaunch.ts` (+test), `web/tests/start-agent.e2e.mjs`, new `web/tests/chat.e2e.mjs` (phone-dialogs).

- Types: session `structured`, capability `acp`, launch `structured`; chat clients.
- Start dialog: OpenCode + `acp` shows **Chat** (“OpenCode asks before it edits a file or runs a command”). When chosen, the
  model/thinking controls are replaced with “Uses OpenCode's configured model”; Start sends `structured:true`. The dialog
  states that a chat's history is in memory and that repository plugins run in folders you trust.
- Session screen: Chat enabled for a structured session; Terminal disabled with a reason. `TerminalWorkspace` renders
  `ChatPane` instead of `TerminalPane` for a structured session.
- ChatPane: messages, streaming agent text, tool calls with command/diff, pending Allow once/Deny cards (focus, 44 px targets),
  composer (Enter sends, Shift+Enter newline), Stop, connection/reconnect, watchdog notice, ended state, no browser storage.
- `chat.ts` reducer is pure; unit tests cover chunk append, tool merge, permission resolution, reset/truncation and dedupe.
- `chat.e2e.mjs` uses a fake `EventSource` and mocked API to cover the dialog, chat, permission, deny, cancel, truncation,
  keyboard and phone fit. Register it in `phone-dialogs`; add no case to `terminal-browser`.

## P6 — docs and evidence

`docs/structured-sessions.md` (threat model, the seven conditions, limits), `docs/terminal-workspace.md`,
`docs/start-agent-session.md`, `docs/ui-verification.md`, `handoff.md`, `README.md`/roadmap if appropriate.
Record: one OpenCode process per chat; in-memory history; no transcript store; no Always allow; model is OpenCode's configured
one; connection loss ends chats; version 1.19.5 only; `.opencode` plugins warning; no real provider/phone test.

## Matrix

| Check | Where |
| --- | --- |
| proto/store/opencodeacp/agent/broker/api | `go test ./...` |
| launch/chat reducer | `npm run check` |
| start-agent structured dialog | `terminal-browser` (existing file, no new fixture; keep additions small) |
| chat screen + phone | new `chat.e2e.mjs` in `phone-dialogs` |
| UI evidence | 1440/834/390 local screenshots; no new terminal-browser fixture |
