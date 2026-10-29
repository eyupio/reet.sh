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

## Approvals

The card names the tool, the folder and, for a command, the full command text; for an edit it shows the whole-file diff
from the permission request. The machine cuts a diff or command that is too large for one control frame and marks the card
truncated; **Allow once** is disabled then, and only **Deny** is available. The browser offers no Always allow, and the
machine ignores any ACP `allow_always` option. An unknown answer is treated as Deny.

A card is for one tool call. Answering it closes it; a later call asks again. The agent's completion update and the
person's answer both appear in the chat.

## Limits

- **History is in memory.** The controller keeps at most 1,000 events / 256 KiB per chat and does not write a transcript.
  A server restart loses it; the session record and its final state remain. Archiving a session drops its chat history.
- **A machine connection loss ends its chats.** The connector starts one process per chat and kills its process group when
  the control connection closes. Reconnecting does not resume a chat; the session becomes lost.
- **Only OpenCode, only 1.19.5.** No other agent has an ACP adapter in slice 1. A machine with an older OpenCode shows the
  version refusal; a newer one waits for a probe.
- **The model is OpenCode's configured one.** The picker is hidden for a chat in this slice. Favourites and account/project
  defaults do not choose a chat model.
- **No images, file attachments, modes, MCP servers or resume.** The chat carries text and command/diff approvals only.
- **No physical phone, Safari or live provider run** was part of the checks. The fake ACP child covers the protocol; the
  real 1.19.5 binary opened a session in a throwaway HOME, but no model turn was sent.
