# OpenCode in the reet TUI

A structured session is **OpenCode in the reet TUI**, a conversation over the Agent Client Protocol (ACP). It is not a
terminal: OpenCode streams messages and tool calls, and reet shows an approval card before an edit or command runs.
Terminal sessions are unchanged.

Slice 1 is deliberately narrow: OpenCode only, trusted folders only, and **Allow once** or **Deny** only. Always allow is
not offered; later slices add a per-session command/file allowance and a live Ask me ↔ YOLO switch, both described below. The probe is [OpenCode `opencode acp` hands-on probe](research/probe-report.md); the plan is
[Program 2, slice 1](superpowers/plans/2026-10-29-program-2-slice-1.md).

## Per-agent availability

A connector that negotiates `acp-agents-v1` reports an `acpAgents` list alongside
its older `acp` flags. Each entry names the tool, adapter command, minimum
version, installation version when readable, state and supported chat features
(history, embedded images, live model and approval changes). It refreshes with
the connection's capabilities rather than querying a package registry.

- `available`: reet has a reviewed integration and resolves its executable.
  An unknown installation version can still be available: starting the chat
  checks `--version` and ACP's reported identity/version before sending a prompt.
- `missing`: the executable needed for that integration is absent.
- `unverified`: an adapter is present but reet has not verified its integration.
- `incompatible`: readable installation evidence does not meet the reviewed floor.

OpenCode is the only available integration in this stage. Claude Code, Codex
and Gemini entries describe missing or unverified support; installing an adapter
alone does not enable its chat. No CLI is executed to build these descriptors,
and availability does not establish a successful model turn or authentication.

An explicit list is authoritative: a missing tool or an empty list does not fall
back to machine-wide flags. Older peers omit the list and retain the OpenCode-only
behaviour. New descriptors are never sent to an older controller. Launches and
history continuation both enforce the per-agent result; stored history must also
belong to the same account, machine, tool and folder, from a closed reet session.
No transcript, database migration or new execution control is introduced.

## Starting one

In **Start an agent**, choose OpenCode on a machine that advertises `acp`. The **Session** choice is **reet TUI** by
default; choose **OpenCode TUI** for the terminal UI. The **Model** control chooses the model for the chat; leave it at
**Agent default** to use the model configured in OpenCode. Thinking stays OpenCode's own. The opening prompt becomes the
first message. The dialog states that the folder's `.opencode` plugins run when it starts and that history lives in
memory.

The session appears in the usual list with its title and state. Its screen opens on **reet TUI**. The header's **OpenCode
TUI** button switches the same live session to OpenCode's own terminal UI, and back, on a machine that negotiated
`session-mode-v1` ([below](#switching-between-the-reet-tui-and-the-opencode-tui)); otherwise it is disabled with the reason.
Closing the tab or **End session** ends the reet session and its process. Losing the
control connection detaches it without killing the agent; the next connection adopts the running session and drains events
queued while it was away. Restarting the agent process ends its chats.

## The transcript

Every tool call is one row: a state dot, its kind (**Bash**, **Read**, **Edit**, **Search**…), the command or the file path
relative to the session's folder, and the line count, duration and status. Where the connector can read them it also shows
the added and removed line counts; opening the row reveals the full command, output or unified diff and any reported paths.
A failed row opens on its error; the rest stay folded so a long turn reads as a list rather than a wall of output.

The connector sends those tool details only to a controller that negotiated **`acp-tool-details-v1`**; an older controller
receives the plain tool event and the row falls back to its title and location. Thinking is a quiet, foldable aside that
stops running when the turn waits, finishes or stops. An agent message keeps its headings, lists and quotes, and its fenced code is rendered in its own block; an approval
remains the one card that asks for an answer.

## The seven conditions

The controller never runs OpenCode. The connector starts `opencode acp` as its own process, one per reet session, in the session's
folder, as the connector's user. All seven conditions are mandatory:

1. **reet owns the permission config.** `OPENCODE_CONFIG_CONTENT` asks for edit, bash and external-directory permission at
   top level and for `build`, `plan`, `general` and `explore`; `task` and `webfetch` are denied; `default_agent` is `build`.
   `OPENCODE_DISABLE_PROJECT_CONFIG=1` ignores the repository's config. The inherited environment is scrubbed of every
   `OPENCODE_*`, so `OPENCODE_PERMISSION`, `OPENCODE_CONFIG` and `OPENCODE_CONFIG_DIR` cannot override it. After
   `session/new`, the connector refuses to continue unless the mode is `build`. OpenCode's own agent-mode switching is not
   offered; approval mode (Ask me or YOLO) is separate and can switch while a chat runs.
2. **A random server password.** Each process gets a fresh `OPENCODE_SERVER_PASSWORD`, so another local process cannot
   approve permissions through OpenCode's local HTTP API. A tool the person approves runs as the same user and can read its
   own environment, including that password; this is accepted for slice 1.
3. **No subagents.** `task: deny` removes the `task` tool. OpenCode does not forward a subagent's permission request over
   ACP, so allowing subagents would hang the turn and hide an approval.
4. **Deadlines.** One card waits at most five minutes; a turn is cancelled after ten minutes and says "still working" after
   twenty seconds of silence. If OpenCode logged a provider error during that silence but did not send it over ACP, reet
   shows the error and stops the turn instead of waiting for the ten-minute deadline. On cancel or timeout reet closes its
   own cards and sends `session/cancel`, because OpenCode never withdraws an outstanding `request_permission`.
5. **An allowlist environment and opt-outs.** Only PATH, HOME, XDG directories, TMPDIR/TEMP/TMP, locale, timezone and the
   connector's own OpenCode settings reach the process. `SSH_AUTH_SOCK`, `DISPLAY`, DBus, GitHub/cloud tokens and shell
   settings do not. The model fetch, autoupdate, default plugins, LSP download, Claude Code compatibility and external
   skills are off. OpenCode may still install `@opencode-ai/plugin` from npm in the background; the probe saw that with the
   opt-outs set. OpenCode reads credentials from its own auth store or from the verified provider keys reet forwards
   (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, the Google/Gemini keys, `OPENROUTER_API_KEY`, `OPENCODE_API_KEY`); no other
   inherited variable reaches it.
6. **Trusted folders.** If `.opencode/plugin*` or `.opencode/package.json` exists, the machine refuses the reet TUI unless the
   start carries the person's trust confirmation. The Start dialog always shows the warning and sends the confirmation
   when the reet TUI is chosen. An unreadable `.opencode` refuses the reet TUI rather than hiding a plugin.
7. **The version floor.** The connector runs `opencode --version` and requires at least `1.19.5` before it starts a process.
   ACP's `initialize` must also report protocol 1, the agent name `OpenCode`, and a version at or above the floor; a newer
   release is accepted, but lowering the floor needs a fresh probe.

## Reopening a reet session and history

A structured session row keeps OpenCode's own session id (`ses_…`) once the machine reports it. When a reet session is closed, its
actions menu offers **Continue in reet TUI with history…**. That creates a **new** reet session row with `resumeAcp` set to the
old id; the old row stays in history. The machine runs `session/load` with the same folder, checks mode `build`, and
replays OpenCode's stored conversation before the new reet TUI is ready. The controller keeps no transcript: the history is
read from OpenCode each time, in memory, and is lost if that machine's OpenCode store is gone.

Replayed tools left `pending` or `in_progress` are shown as interrupted, not as still working. Pending approvals are not restored; a new prompt starts cleanly. The controller will only load an id that belongs to one of the account's own closed
structured sessions on that machine, so a crafted `resumeAcp` cannot read another project's OpenCode history.

A connector that predates `acp-history-v1` can still run a new reet TUI, but it does not report or accept an OpenCode session
id, so its reet sessions have no Continue action. The `acpHistory` capability is sent only to a controller that negotiated the
feature, and the older controller's strict decoder never sees the new fields.

## Images

In the reet TUI, **Attach image** (or paste/drop) uploads the file to the machine over the launch-upload route, then sends
the image with the prompt: the connector reads its own uploaded file and embeds the bytes as an ACP image
content block. The agent sees the image itself, not a path it has to open with a tool, so no extra read approval is needed.
The prompt can be images alone. Limits are PNG, JPEG, GIF or WebP, 5 MiB each and at most eight in one prompt. Uploads live
under the machine's private `launch-uploads/` folder and are removed by the existing 24-hour sweep.

A connector that predates `acp-image-v1` keeps the old behaviour: the prompt carries the absolute path as text, and an
agent that needs the image opens it with its own read tool. The controller never sends the image fields to such a machine.

## Approvals

The card names the tool, the folder and, for a command, the full command text; for an edit it shows the whole-file diff
from the permission request. The machine cuts a diff or command that is too large for one control frame and marks the card
truncated; **Allow** actions are disabled then, and only **Deny** is available. The browser never sends the agent's own
`allow_always` option. An unknown answer is treated as Deny.

A card offers **Allow once**, **Deny**, and, when the card names a command or a path, **Allow this command/file for this
session**. The last choice is remembered by the controller for that chat only, by exact command or path and tool kind:
a repeat is answered automatically, every automatic answer is audited as `session-allow`, and the approval is forgotten
when the chat ends, is archived or the controller restarts. It never widens to other commands or files, and it is never
sent to the agent as `allow_always`.

A card is for one tool call. Answering it closes it; a later call asks again unless the person allowed that exact
command or file for the session. The agent's completion update and the person's answer both appear in the reet TUI.

## Switching Ask me and YOLO while it runs

A structured session's header shows **Ask me** and **YOLO** for a live chat on a machine that negotiated
`acp-approval-v1`. Switching to YOLO asks for the same per-session confirmation as a launch and passes the same gates:
the account's Allow YOLO setting, the machine's `yolo` capability and the agent's reviewed YOLO option. The controller
then sends `acp_mode`; the connector sets its own auto-answer for this chat and answers any card already waiting. The
store's `yolo` flag, the YOLO badge and the audit (`yolo-mode`, then `yolo-allow` for each answer) follow. Switching back
to Ask me stops the auto-answer immediately. A connector older than `acp-approval-v1` reports no `acpApproval` capability,
so the switch is not offered; terminal sessions keep the mode they launched with. A refusal is sentence-shaped and leaves
the mode as it was.

## Switching between the reet TUI and the OpenCode TUI

A running OpenCode session can move between the reet TUI and the OpenCode TUI under the same session row. The session
header's **reet TUI** | **OpenCode TUI** pair shows the one in use; the other one switches (on a phone, the session's
actions menu has **Switch to the reet TUI…** or **Switch to the OpenCode TUI…**). A confirmation says what happens: the
machine stops one process and starts the other on the same OpenCode conversation in the same folder, terminal scrollback
is not kept, and the switch can take a few minutes. Until the machine answers, both buttons wait and the status line says
**Switching to the …**; the browser sets no deadline of its own. Then the pane changes to the chat or the terminal, and
the status pill, the **Approvals** line and the context panel's decisions follow the new mode.

- **Only reet-started sessions return to the reet TUI.** Going to the reet TUI needs OpenCode's session id, which reet
  records only for a session that started (or has been) in the reet TUI. A session started in the OpenCode TUI keeps the
  **reet TUI** button disabled: "This session started in the OpenCode TUI, so the reet TUI cannot pick up its
  conversation." Going the other way needs the same id: a reet TUI session whose OpenCode id was never recorded is
  refused with "This session has no saved OpenCode conversation yet, so it cannot move to the OpenCode TUI."
- **Idle only.** A chat with a running turn, an approval card or a question waiting does not switch: "Wait for the agent
  to finish or answer what it is asking." The controller refuses it too.
- **`session-mode-v1` machines only.** A connector that did not negotiate it, has no chat history for OpenCode, or
  cannot launch OpenCode in a terminal (OpenCode is not among its launch tools) is not asked: "Update reet on this
  machine to switch between the reet TUI and the OpenCode TUI." Only a running session can switch. Asking for the mode
  the session is already in changes nothing.
- **YOLO stays YOLO, and is checked again.** A YOLO session moving to the OpenCode TUI starts it with `--auto`, so the
  switch is refused if the account no longer allows YOLO ("An owner or admin can allow YOLO for this account in
  Settings."), before anything is stopped. The switch is audited (`session-mode`, with `yolo`), and a YOLO switch to the
  OpenCode TUI also records a `yolo-launch` event like a launch.
- **Terminal scrollback is not carried over.** The reet TUI rebuilds the conversation from OpenCode's own store with
  `session/load`, as **Continue in reet TUI with history…** does; the OpenCode TUI opens that conversation on a fresh
  screen.
- **The OpenCode TUI is stopped before the chat opens.** The machine waits up to five seconds for the TUI to exit after its
  tmux session closes. A TUI that has not exited by then is sent SIGTERM and, if it is still there, SIGKILL; the chat
  opens only once its process group is gone, so two OpenCode processes never share one conversation. If a TUI survives
  even that, no chat is opened and the session ends.
- **A failed switch says whether the session survived.** When the other mode cannot start, the machine restores the
  previous one and the switch reports "The session could not be switched. Try again, or end it and start a new
  session." When neither mode could be left running, the machine ends the session and the switch reports "The session
  could not be switched and has ended. Start a new session or continue it with history."
- **The reet TUI is as durable as any chat.** A session in the OpenCode TUI runs in tmux and survives an agent restart;
  once switched to the reet TUI it ends if the agent process restarts (its OpenCode id can still be continued).
- **Folder trust when returning to the reet TUI.** OpenCode project plugins run when the reet TUI opens the session. If
  the folder has `.opencode` plugins and the request does not carry the person's trust, the machine refuses it before
  stopping anything: the OpenCode TUI keeps running untouched. The dialog then shows the plugin warning and an **I trust
  this folder** confirmation, and sends the switch again with the person's trust.

## Limits

- **History is in memory.** The controller keeps at most 1,000 events / 256 KiB per reet session and does not write a transcript.
  A closed reet TUI session can be reopened from OpenCode's own store with Continue in reet TUI with history…; the live event ring still
  resets on a server restart, and archiving a session drops its controller-side ring.
- **The browser's log is bounded too.** The reet TUI keeps at most 400 items / 256 KiB of the conversation, drops the
  oldest first and says when earlier messages are no longer shown; an approval still waiting for an answer is never
  dropped. A continued chat replaying a long OpenCode history therefore stays responsive instead of building an
  unbounded page.
- **A control connection loss does not end a reet session.** The connector owns one process per reet session for the life of
  the agent process. A reconnect adopts the running sessions, includes them in reconciliation, and drains the bounded event
  queue produced while detached. Restarting the agent ends them; their stored OpenCode ids can still be reopened if that
  store survives.
- **Only OpenCode, at least 1.19.5.** No other agent has an ACP adapter in this slice. A machine with an older OpenCode
  shows the version refusal; a newer release passes the name and semver checks.
- **The model is chosen at launch.** **Agent default** uses OpenCode's configured model. A chosen model is passed in
  `OPENCODE_CONFIG_CONTENT` and must match the model OpenCode reports after `session/new`; a mismatch refuses the start.
  Thinking remains OpenCode's own. Continuing a chat keeps the model recorded on the closed session.
- **Switching TUI is for a running, idle OpenCode session** on a `session-mode-v1` machine, and only a session reet
  started in the reet TUI can return to it. Terminal scrollback is not carried over, a switch can take a few minutes, and
  a session in the reet TUI is lost if the agent process restarts.
- **Approval mode can change while a structured chat runs**, on a connector that negotiated `acp-approval-v1`. The
  per-session approval card and the Ask me/YOLO switch are described above. A terminal session cannot change the flags
  it was launched with.
- **Dropped provider errors are recovered.** When OpenCode ends a turn without streaming any progress, the connector reads
  the newest matching error from OpenCode's local store and shows it instead of leaving a silently empty turn. A provider
  error OpenCode writes to its log during a silent turn is read the same way; the connector shows it and stops that turn
  rather than leaving "still working" until the deadline.
- **Images are embedded when the machine supports it.** An `acp-image-v1` machine embeds the uploaded image in the ACP prompt
  (at most eight per prompt); an older machine names the path as text. OpenCode's agent modes, MCP servers, resume of a live turn and other
  agents are not in this slice.
- **No physical phone, Safari or live provider run** was part of the checks. The fake ACP child covers the protocol; the
  real 1.19.5 binary opened and then loaded a session in a throwaway HOME, but no model turn was sent.
