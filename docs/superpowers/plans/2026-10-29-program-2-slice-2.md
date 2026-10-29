# Program 2, slice 2: chat history, continue and images

After slice 1 (#256). The probe showed `session/load` replays the whole conversation before its result, returns
`configOptions` with mode `build`, and that a tool left `in_progress` in the stored session is not re-asked and must be
shown interrupted. This slice reopens a closed chat against OpenCode's own session store, continues it, and lets a person
attach images as machine paths through the launch-upload path.

## Constraints

- Slice 1's seven conditions remain mandatory. A reopened chat runs the same reet config, environment, password, version
  and plugin trust checks before `session/load`.
- The controller stores only the OpenCode session id (`ses_…`) on the existing session row. It still writes no transcript.
  History comes from OpenCode on the machine each time a chat is reopened.
- A reopened chat is a **new reet session row** with `resumeAcp` set to the old session's id; the old row stays in history.
  This avoids an illegal Exited → Running transition and keeps the old record honest.
- Replay is shown in order. Replayed `in_progress`/`pending` tools are marked interrupted, never left "working". Pending
  approvals are not restored; the UI says so in the reopened chat.
- Images use the existing launch-upload path (`/api/v2/machines/{id}/uploads`): the machine stores the file under
  `launch-uploads/`, and the prompt carries its path. It is a path, not an embedded ACP image, and the same 5 MiB /
  20 files / 50 MiB / 24-hour rules apply.
- `acp-v1` gains the resume fields only where a slice-1 machine never sends them; older controllers cannot create chats.
- No new `terminal-browser` fixtures. Browser cases go in `phone-dialogs`; `chat.e2e.mjs` extends.

## T1 — persist the OpenCode session id

- `terminalproto.SessionChanged` and `SessionSnapshot` gain `acpSessionId` (validated `ses_…`), sent only for structured
  sessions. `Create` gains `resumeAcp`, valid only with `structured`.
- `terminal_sessions.acp_session_id`; `TerminalSession.ACPSessionId`; reserve stores `resumeAcp`, and a machine report
  records/confirms it. A different id for the same session is a conflict.
- Broker `CreateSession`, `Peer.observe` and `Synchronize` record the id. Tests cover create, replay, reconcile and conflict.

## T2 — load history on the machine

- `opencodeacp.Options.Resume`; after initialize use `session/load` instead of `session/new`, using the same cwd and empty
  MCP list, and use its returned `configOptions` for the mode/model checks.
- Track tool states during replay; after the load result emit `failed` tool updates for every replayed `pending`/
  `in_progress` tool with "Interrupted before this chat was reopened." Never emit a fake permission.
- Keep the replay order: user, agent, tool chunks are already mapped by `session/update`.
- The connector's `chatHost.open` passes `Resume`; the create reply and later reports carry the id. Tests use the fake ACP
  child for load replay, mode refusal and interrupted tools; the real 1.19.5 binary opens a loaded session.

## T3 — continue a chat

- `sessions.ts`: `continueChatLaunch`/`canContinueChat` build a structured launch with `resumeAcp`, the old folder, title
  and project; only for a closed structured session with an id and a machine advertising `acp`.
- `TerminalWorkspace.continueSession` branches to that launch; `SessionScreen` labels the action **Continue chat with
  history…**. The new session opens on Chat and receives the replay.
- The old session is not mutated. No resume is offered when the id is missing or the machine lacks `acp`.
- Tests: unit launch resolution, e2e the menu, the create body and the replay.

## T4 — images in the chat

- `ChatPane` gets attach/paste/drop using `terminal/upload.ts` validation and `launchUpload(session.agentId, file)`.
  Chips show name/size, can be removed, and are limited to 20 files/50 MiB; the send is disabled while uploading.
- On send, upload each file, append `imagePrompt(draft, references)`, check the 2,000-byte prompt limit, then POST the
  prompt. Upload failure keeps the chips and says so; nothing is stored in the browser.
- Hide the control when the machine does not advertise uploads or has them off, with the existing sentence.
- Tests: `chat.e2e.mjs` uploads a PNG before a prompt and asserts `Image: /…/launch-uploads/…` in the POST; phone fit stays.

## T5 — docs and evidence

`docs/structured-sessions.md` (history/reopen, interrupted tools, images as paths, old row retained),
`docs/start-agent-session.md`, `docs/terminal-launch.md`, `docs/ui-verification.md`, `handoff.md`, roadmap. Record the real
1.19.5 load result and the fake replay cases.
