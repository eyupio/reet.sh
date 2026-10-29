# Structured OpenCode chats

A structured session is a conversation with **OpenCode** over the Agent Client Protocol (ACP). It is a chat, not a
terminal: OpenCode streams messages and tool calls, and reet shows an approval card before an edit or command runs.
Terminal sessions are unchanged.

Slice 1 is deliberately narrow: OpenCode only, trusted folders only, and **Allow once** or **Deny** only. Always allow is
not offered. The probe is [OpenCode `opencode acp` hands-on probe](research/probe-report.md); the plan is
[Program 2, slice 1](superpowers/plans/2026-10-29-program-2-slice-1.md).

## Starting one

In **Start an agent**, choose OpenCode on a machine that advertises `acp`, then **Chat, not a terminal**. The model and
thinking controls are replaced with a note: a chat uses the model configured in OpenCode. The opening prompt becomes the
first message. The dialog states that the folder's `.opencode` plugins run when the chat starts and that history lives in
memory.

The session appears in the usual list with its title and state. Its screen opens on **Chat**; **Terminal** is disabled
because there is no shell to attach to. Closing the tab, **End session** or losing the machine connection ends the chat and
its process.

## The seven conditions

The controller never runs OpenCode. The connector starts `opencode acp` as its own process, one per chat, in the session's
folder, as the connector's user. All seven conditions are mandatory:

1. **reet owns the permission config.** `OPENCODE_CONFIG_CONTENT` asks for edit, bash and external-directory permission at
   top level and for `build`, `plan`, `general` and `explore`; `task` and `webfetch` are denied; `default_agent` is `build`.
   `OPENCODE_DISABLE_PROJECT_CONFIG=1` ignores the repository's config. The inherited environment is scrubbed of every
   `OPENCODE_*`, so `OPENCODE_PERMISSION`, `OPENCODE_CONFIG` and `OPENCODE_CONFIG_DIR` cannot override it. After
   `session/new`, the connector refuses to continue unless the mode is `build`. Mode switching is not offered.
2. **A random server password.** Each process gets a fresh `OPENCODE_SERVER_PASSWORD`, so another local process cannot
   approve permissions through OpenCode's local HTTP API. A tool the person approves runs as the same user and can read its
   own environment, including that password; this is accepted for slice 1.
3. **No subagents.** `task: deny` removes the `task` tool. OpenCode does not forward a subagent's permission request over
   ACP, so allowing subagents would hang the turn and hide an approval.
4. **Deadlines.** One card waits at most five minutes; a turn is cancelled after ten minutes and says "still working" after
   twenty seconds of silence. On cancel or timeout reet closes its own cards and sends `session/cancel`, because OpenCode
   never withdraws an outstanding `request_permission`.
5. **An allowlist environment and opt-outs.** Only PATH, HOME, XDG directories, TMPDIR/TEMP/TMP, locale, timezone and the
   connector's own OpenCode settings reach the process. `SSH_AUTH_SOCK`, `DISPLAY`, DBus, GitHub/cloud tokens and shell
   settings do not. The model fetch, autoupdate, default plugins, LSP download, Claude Code compatibility and external
   skills are off. OpenCode may still install `@opencode-ai/plugin` from npm in the background; the probe saw that with the
   opt-outs set. Provider credentials come from OpenCode's own auth store, not from reet's environment.
6. **Trusted folders.** If `.opencode/plugin*` or `.opencode/package.json` exists, the machine refuses a chat unless the
   start carries the person's trust confirmation. The Start dialog always shows the warning and sends the confirmation
   when Chat is chosen. An unreadable `.opencode` refuses the chat rather than hiding a plugin.
7. **The exact version.** The connector runs `opencode --version` and requires `1.19.5` before it starts a process. Another
   version is refused; a future version must be probed again before it is accepted. ACP's `initialize` version is checked
   as well.

## Reopening a chat and history

A structured session row keeps OpenCode's own session id (`ses_…`) once the machine reports it. When a chat is closed, its
actions menu offers **Continue chat with history…**. That creates a **new** reet session row with `resumeAcp` set to the
old id; the old row stays in history. The machine runs `session/load` with the same folder, checks mode `build`, and
replays OpenCode's stored conversation before the new chat is ready. The controller keeps no transcript: the history is
read from OpenCode each time, in memory, and is lost if that machine's OpenCode store is gone.

Replayed tools left `pending` or `in_progress` are shown as interrupted, not as still working. Pending approvals are not
restored; a new prompt starts cleanly. The controller will only load an id that belongs to one of the account's own closed
structured sessions on that machine, so a crafted `resumeAcp` cannot read another project's OpenCode history.

## Images

In a chat, **Attach image** (or paste/drop) sends the file through reet's launch-upload path before the prompt. The machine
stores it under its private `launch-uploads/` folder and the prompt carries the absolute path as text; it is not an embedded
ACP image. The limits and sweep are the same as the launch bar's: PNG, JPEG, GIF or WebP, 5 MiB each, 20 files and 50 MiB
together, removed by the existing 24-hour sweep. An agent that needs to look at the image opens that path with its own read
tool, and that tool may ask for approval. A path may be gone after the sweep or when OpenCode's history is reopened later.

## Approvals

The card names the tool, the folder and, for a command, the full command text; for an edit it shows the whole-file diff
from the permission request. The machine cuts a diff or command that is too large for one control frame and marks the card
truncated; **Allow once** is disabled then, and only **Deny** is available. The browser offers no Always allow, and the
machine ignores any ACP `allow_always` option. An unknown answer is treated as Deny.

A card is for one tool call. Answering it closes it; a later call asks again. The agent's completion update and the
person's answer both appear in the chat.

## Limits

- **History is in memory.** The controller keeps at most 1,000 events / 256 KiB per chat and does not write a transcript.
  A closed chat can be reopened from OpenCode's own store with Continue chat with history…; the live event ring still
  resets on a server restart, and archiving a session drops its controller-side ring.
- **A machine connection loss ends its chats.** The connector starts one process per chat and kills its process group when
  the control connection closes. Reconnecting does not resume a live chat; the session becomes lost, but its stored
  OpenCode id can be reopened if that store survives.
- **Only OpenCode, only 1.19.5.** No other agent has an ACP adapter in this slice. A machine with an older OpenCode shows the
  version refusal; a newer one waits for a probe.
- **The model is OpenCode's configured one.** The picker is hidden for a chat in this slice. Favourites and account/project
  defaults do not choose a chat model.
- **Images are paths.** Attachments go through the launch-upload path and the prompt names the file; no image bytes are
  embedded in ACP. Modes, MCP servers, resume of a live turn and other agents are not in this slice.
- **No physical phone, Safari or live provider run** was part of the checks. The fake ACP child covers the protocol; the
  real 1.19.5 binary opened and then loaded a session in a throwaway HOME, but no model turn was sent.
