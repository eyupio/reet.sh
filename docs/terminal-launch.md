# Launch requests: starting an agent in a terminal session

A launch request asks a machine to start one of its installed agent CLIs (`claude`,
`codex`, `opencode`, `cline`, `antigravity`) in a chosen folder, with an optional model and an
optional opening prompt, instead of the plain login shell that a terminal session
starts today. It is an optional field set on the existing create-session call.
Nothing changes for a plain shell: the arguments the connector gives tmux for a
create without a tool are byte-identical to before.

The controller never sends a shell string or a program path. It names a tool id.
The connector looks that id up in a table built into its own binary and decides
the program and arguments itself.

This document describes what the code does. The web dialog and the entry points
that call it are described in [Start an agent session](start-agent-session.md).

## Compatible private extension

After authenticating an agent, the HTTP websocket upgrade advertises
`X-Reet-Terminal-Features: command-history-v1,launch-v1`. A connector that
understands `launch-v1` records it and reports the capability below. An older
connector splits the header on commas and ignores the token it does not know, so
it connects unchanged.

A connector reports `launch:true` and `tools:[...]` in its capabilities only when
all of these hold: `launch-v1` was negotiated, it has the terminal dispatcher, it
was given a tool lister, and it runs on Linux or macOS. `launch` requires
`terminalControl`. `tools` is the sorted, de-duplicated list of preset tool ids
whose binary was found on the connector's `PATH`; it is omitted when empty and
cannot be set without `launch`. The capabilities already reach
`/api/v2/terminal/machines` and `/api/v2/machines/health`, so both show `launch`
and `tools` for a launch-capable machine and omit them for an older one.

The broker checks the stored capabilities for the current connection generation
before it sends any launch field. A peer without `launch` is never sent one and
nothing is reserved. The connector also refuses, with `launch_failed`, a create
that carries a tool on a connection that did not negotiate `launch-v1`; it never
substitutes a shell. There is no protocol version bump, no arbitrary exec
operation and no decoder relaxation. The control frame limit is 64 KiB, so an
8,000-byte prompt still fits when every byte is escaped in JSON.

## Wire fields and limits

`terminalproto.Create` gains four optional flat string fields, so it stays a
comparable struct and `omitempty` keeps a plain create unchanged. They are named
`tool`, not `agent`, because `agentId` already means the machine.

| Field | Meaning | Limits |
| --- | --- | --- |
| `tool` | Tool id | One of `claude`, `codex`, `opencode`; `cline` and `antigravity` under `agents-v2`. `gemini` is retired and refused like any unknown tool |
| `model` | Model id, optional | 1 to 100 bytes; letters, digits and `. _ : / @ + -`; the first character must be a letter or digit |
| `dir` | Folder, optional | Absolute; at most 4,096 bytes; valid UTF-8; no control characters |
| `prompt` | Opening prompt, optional | At most 8,000 bytes; valid UTF-8; no control characters except newline; must not begin with `-` |

`model`, `dir` or `prompt` without `tool` is refused. Strict decoding is
unchanged: an unknown field is refused. The controller and the connector both
validate; the connector validates again before it starts anything.

The leading `-` rule exists so that a prompt can never be read as a flag by a tool
that takes the prompt after a flag of its own (see the presets).

## What is kept

The opening prompt is kept with the session, in the controller's database, capped at 8,000 bytes (the
limit above). Only the session's owner can read it. The session list carries a 300-character excerpt
(`promptExcerpt`, with `promptMore` when there was more), and a create reply carries the same excerpt
because the row is read back after it is written. The whole text comes from
`GET /api/v2/terminal/sessions/{id}/prompt`, which answers `{"prompt": "..."}` with `Cache-Control: no-store`;
a session that is not the caller's answers 404 "session not found", as the inspect route does, and a malformed id is a
400. Archiving the session clears the prompt, both from the session and from the copy kept with its create
request, so the endpoint then answers an empty prompt and a retry of that create request that carried a
prompt answers a conflict instead of replaying. The prompt is never written to the audit trail or to logs.

## Session titles

`title` is optional on the create call. A request that leaves it out, or sends only spaces, is named by the
controller before anything else is checked, so the browser, the API, a project run and a saved draft
(a draft with a prompt but no title) all name a session the same way. A title that is sent is validated and
kept exactly as sent: 1 to 80 plain characters, as before.

The name is made from the request alone, with no model, no network and no clock, so a retry of a create
whose reply was lost asks for the same session under the same name. It is:

1. **The work the prompt asks for**, if the prompt says anything. The first line with something to say,
   without control characters. Blank lines, code fences, the `Use the {name} skill.` line and lines with
   no letter or digit are skipped. A leading heading, quote or list marker, and leading English filler
   (a greeting, "please", "can you", "I'd like you to", "help me", "let's"), are removed. The name
   ends at the first sentence: a `?` or `!`, or a `.` after a word of three or more characters with no
   full stop in it ("e.g." does not end one), once at least 8 characters have been read. It is trimmed
   of trailing punctuation and an ASCII first letter is capitalised; other scripts are left alone. Over
   60 characters it is cut at the last space (not earlier than the 20th character), or at 60 if there is
   none, and ends in an ellipsis.
2. **Otherwise the agent and folder**: the agent's name ("Claude Code", "Codex", "OpenCode",
   "Cline", "Google Antigravity (AGY)") or "Terminal" for a plain shell, then · and the last
   part of the folder when there is one and it holds no control or format characters.

The rules live in `terminalproto.PromptTitle` and `terminalproto.AutoTitle`. The browser mirrors them
(`promptTitle` and `titleFor` in `web/src/lib/agentLaunch.ts`) so the title the dialog suggests is the
one the controller would give, and both are tested against the same cases in
`internal/terminalproto/testdata/autotitle.json`. A session that already has a title is never renamed by
this; Rename works as before.

## Structured chats

`terminalproto.Create` also has `structured`, `trust` and `resumeAcp` fields, sent only after `acp-v1` (see
[structured sessions](https://github.com/eyupio/reet.sh/blob/main/docs/structured-sessions.md)). `structured: true` is accepted only with a reviewed chat tool
(`opencode`, `cline`, or Claude Code and Codex for an account that has turned their chats on in Settings > Agents) and without a
worktree, resume, YOLO, base or effort; the machine starts that agent's chat process (`opencode acp`, `cline --acp`, or
the pinned Claude/Codex adapter) instead of tmux. `trust: true` confirms the
repository-plugin warning, which OpenCode is the only reviewed tool to read. `resumeAcp` is a native session id owned by one of the account's closed structured sessions
of the same tool and folder on that machine (OpenCode `ses_…`, Claude and Codex UUIDs, Cline
`<milliseconds>_<5 characters>_cli`); the machine loads its history instead of creating a new session. A structured create reserves the same session record and quota as a shell, but the session has
no terminal: attach, resize, history and command-history requests answer not found. `acp_prompt`, `acp_cancel` and
`acp_permission` carry the chat; `acp_event` carries bounded updates back. The seven conditions and the approval rules are
in [structured sessions](https://github.com/eyupio/reet.sh/blob/main/docs/structured-sessions.md).

## Presets

The table is built into the connector. A tool is offered only if it is in the
table and `exec.LookPath` finds its binary on the connector's `PATH`. The resolved
path must also be absolute and contain only letters, digits and `. _ + @ / -`,
because tmux hands a one-argument command to `sh -c`; a binary at any other path
is treated as not installed. The model and the prompt are each exactly one
argument element. No preset uses an approval-bypass, auto-approve, print, exec or
other one-shot flag unless the person chose YOLO and the machine accepts it. A
test pins the ordinary flags to `--model`, `--prompt` and
`--prompt-interactive`, and the YOLO flags to the reviewed rows below.

| Tool | Arguments (with a model and a prompt) | Confirmed from |
| --- | --- | --- |
| `claude` | `claude [--dangerously-skip-permissions] [--continue \| --resume <id>] [--effort <level>] [--model <model>] [-- <prompt>]` | `claude --help` on Claude Code 2.1.319, captured in `internal/terminalhost/testdata/help/claude.txt` with the registry tarball and integrity in `sources.json`: "Usage: claude [options] [command] [prompt]", `-c, --continue` ("Continue the most recent conversation in the current directory"), `-r, --resume [value]`, `--dangerously-skip-permissions`, `--effort <level>` and `--model <model>`. The bundled argument parser of 2.1.310 was read to confirm that `--` ends options. Only version and help were run. |
| `codex` | `codex [--ask-for-approval never] [resume --last \| resume <id>] [-c model_reasoning_effort=<level>] [--model <model>] [-- <prompt>]` | `codex --help` and `codex resume --help` on codex-cli 0.166.0, captured in `internal/terminalhost/testdata/help/codex.txt` with the registry tarball and integrity in `sources.json`: `--model`, `-a/--ask-for-approval`, `-c/--config <key=value>` and a `resume` subcommand that takes the same options plus `--last`, with an optional `PROMPT`. Codex has no effort flag, so the level is a `model_reasoning_effort` config override, which the help does not name; the argv was run through the real terminal UI under a pty against a stand-in provider (no account, network or real model) on codex-cli 0.156.1 and 0.166.0, and the provider received the chosen model, effort and approval policy and, for `resume`, the earlier turns (probe report, "Codex terminal probes"). |
| `opencode` | `opencode --model <model> --prompt <prompt>` | https://opencode.ai/docs/cli/: "opencode [project]" starts the terminal UI, with `--prompt` and `--model`. The positional is a project path, so the prompt goes through the flag. |
| `antigravity` | `agy --model <model> --prompt-interactive <prompt>` | Antigravity CLI 1.5.0 captured version/help in `internal/terminalhost/testdata/help/agy.txt`; official archive and verified SHA-256 in `sources.json`. `--prompt` is a print-mode alias and is not used. The `agy` alias distinguishes the CLI from the desktop app. |

## Models read from the machine

A connector that negotiated `models-v1` can list the models its agent reports.
In v1 only OpenCode has a confirmed listing (`opencode models`); the other
agents answer `unsupported` and the browser keeps its suggestions plus Other….
A listed name is a suggestion, not proof that the model works: the launch path
still sends the explicit `Create.Model` under the existing rule and never reads
the list. The machine's local switch is
`reet terminal models on|off|status` (on by default, refused while terminal
access is off). The shared picker in the launch bar, Start an agent and Add project/Project
settings reads this cached list, with favourites, reet's suggestions and
Other… when it cannot. The account-default picker in Settings › Agents has no
single machine, so it uses favourites, suggestions and Other; the Add a model
picker there reads the cached lists of every machine. A machine's
**Settings → Agents** shows each installed agent's state last read (“42 read
3 h ago”, “not supported”, “couldn't read: timed out”, “off”, “offline” or
“update”) and offers Refresh only while the machine is online and
models-capable. See [agent models](agent-models.md) for the command's
environment, limits, cache and API.

## Uploaded images

An image attached in the browser is stored by the machine outside the project and
the browser types its path into the session. reet makes no claim that a particular
agent turns that path into an image attachment: the path is text, and the agent and
model decide what to do with it. Per-agent image-path behaviour at the pinned
versions has not been checked, so the interface says it is a path. See
[terminal workspace](terminal-workspace.md) for the upload limits and privacy.

With no model the `--model <model>` pair is left out; with no prompt the prompt
part is left out. Claude Code's row was re-confirmed against its 2.1.319 captured help; the other rows were
confirmed on 2026-10-25 from the documentation and source cited, or from the captured help named in their row. A tool whose syntax cannot be confirmed is not
offered. `pi` and `cursor` are not offered for that reason.

### The one-word prompt rule (claude and codex)

`claude` and `codex` take the prompt as an operand after `--`. Their parsers can
still treat the first operand as a subcommand name after `--`: Claude Code's
parser would run `claude purge`, `claude update` or `claude install` for a prompt
that is just `purge`, `update` or `install`. A subcommand name is one word, so for
these two tools a prompt must contain a space or a newline. The same rule is
applied to `codex` as defence in depth; whether codex needs it is not documented
(see "Known limits"). `opencode` takes the prompt as the value of a
flag and is not affected. The controller refuses a one-word prompt before it
sends anything (`prompt_unsupported`), and the connector refuses it again.

## Starting the program

The connector appends the program and its arguments to `tmux new-session` as
separate arguments; tmux runs several arguments directly, without a shell. The
working directory is the resolved `dir`, or the enrolled home when `dir` is empty.
The environment is the existing sanitised allowlist. The model never travels as an
environment variable. `remain-on-exit` stays on, so a session whose agent exits
keeps its exit status.

### Folder policy

When `dir` is given the connector requires it to be absolute, resolves it with
symlinks followed, and requires the result to exist, to be a directory and to lie
within the enrolled home (also resolved; a sibling such as `home-other` is not
inside). The home is the boundary in this change. A refusal is the result code
`launch_failed`. The folder is resolved before any session, receipt or registry
entry is written, so a refused launch leaves nothing behind on the machine.

### tmux argument escaping

Two tmux behaviours would otherwise change what the program receives, so the
connector escapes them:

- `new-session -c` is format-expanded. A folder named `a#(command)` would run
  `command`, and `#S` or `#{...}` would rewrite the path. The connector doubles
  every `#` in the `-c` value so tmux reads it literally.
- tmux reads an argument that ends in `;` as a command separator unless the
  character before it is a backslash, which it replaces by `;`. The connector
  inserts a backslash before a final `;` in every `new-session` argument (`-c`,
  the `-e` value, the program and each argument), so `fix it;` reaches the program
  as `fix it;`. A `;` anywhere else is literal and left alone.

Both were checked with a real tmux and a recording program.

## Errors

| Cause | Status | Code | Sentence |
| --- | --- | --- | --- |
| One-word prompt for `claude` or `codex` | 422 | `prompt_unsupported` | "Write the opening prompt as at least two words." |
| Machine did not negotiate `launch-v1` | 409 | `launch_unsupported` | "Update reet on that machine to start agents." |
| Connector refused and the tool is not in the machine's reported `tools` | 422 | `tool_not_installed` | "That agent isn't installed on that machine." |
| Connector refused for any other reason | 422 | `folder_unavailable` | "That folder isn't available on that machine." |
| Malformed request (unknown field, bad tool, model, folder or prompt) | 400 | the existing invalid-request response | |

On the wire the connector has one new result code, `launch_failed`, for a refused
folder, a missing binary or a refused prompt. It does not say which, so the
controller decides between the last two rows from the `tools` the machine
reported. If a tool listed in `tools` disappears before the start, the person sees
the folder sentence; the session still ends `lost` and is never shown as running.
A session whose launch was refused is recorded as `lost`, and a replay of that
request id returns the same refusal without contacting the machine.

Viewers, API and MCP tokens, and legacy auth modes are blocked by the existing
terminal access checks, exactly as for a plain create. The per-machine and
per-account session limits and the local consent check are unchanged.

## Idempotency

The whole `Create` is the idempotency key: the controller stores its JSON with the
create request, and the connector compares the whole value on a retry. The same
request id with the same fields returns the same session and starts nothing
second. The same request id with any changed field, launch or not, is a 409.

## Where the prompt is kept

Because the whole `Create` is the idempotency record, the opening prompt is stored
in the controller's database row for that create request and in the machine's
private registry file, which is readable only by its owner. It is not written to
the audit trail, to logs or to browser storage. It is on the command line of the
agent process, so other users of that machine can see it in the process list.
Secrets do not belong in a prompt.

## Audit

A launch writes one audit event, action `terminal-launch`, source `ui`, with the
detail `machine` (the agent id), `host`, `tool`, `model` and `dir`. The prompt is
never included. The event is written only when the call that learned the session
had started is a create with a tool; a replay of a known outcome and a refusal
write nothing.

## Known limits

- If a machine is downgraded after a launch was recorded, replaying that request
  id gets the 409 update sentence rather than the stored session, because the
  capability check runs before the reservation lookup.
- Audit is written at most once in practice, but if the reply to the first create
  is lost, that attempt is not audited and the retry that resolves it is. Two
  concurrent retries racing on exactly that path could both write an event; this is
  unlikely because the resolver uses a try-lock.
- tmux 3.2 is the minimum the connector requires. The semicolon rule was checked
  against real tmux 3.4 and 3.5a and, for 3.2, by reading `cmd_parse_from_arguments`
  in the 3.2 source. tmux 3.2 itself was not run.
- Standard clap behaviour ends subcommand matching after `--`, so `codex` is
  probably safe from the subcommand problem, but codex documents `--` only for
  `codex mcp add`. The one-word rule is applied to it regardless.
- `opencode` is safe from a prompt being read as a flag only because
  the prompt cannot begin with `-`. opencode's current parser was not read from
  source; its flags come from its documentation.
- A retry of an already-recorded request id does not look up the binary or the
  folder again. If the binary disappears between the check and tmux starting it,
  the pane ends and the session is reconciled to `exited`, never `running`.
- Windows is not supported; no launch capability is reported there.

## What this does not establish

A started session means only that the connector started the program in tmux in
that folder. It does not establish that the agent is signed in, has network
access, understands the prompt or is making progress, and it does not show that a
model name exists; the agent will say so itself. It does not stop a person typing
something else into the session afterwards.

The web dialog, its entry points (the Terminals workspace and the machine card)
and the Activity label for `terminal-launch` are described in
[Start an agent session](start-agent-session.md).

## Catalogue and named bases (`agents-v2`)

The controller advertises `agents-v2` alongside the existing feature tokens. A
connector reports `agentsV2:true` and optional `osVersion` only after negotiating
that token. The additional `antigravity-cli-v1` token gates the AGY tool id: an older controller whose `agents-v2` catalogue predates AGY receives no AGY claim. Without `agents-v2`, the connector reports only the original three tool ids
and never sends the added capability fields. OS version is informational: Linux
reports its kernel release and macOS its product version; a failed local reading
leaves it absent.

Create accepts optional `base`, `effort` and `yolo`. An older peer receives none
of those fields and no new tool id: the controller answers 409
`launch_option_unsupported` with "Update reet on that machine to use that option."
A named base requires a worktree. A connector that negotiated `base-fresh-v1`
fetches that branch from a `github.com` origin and starts the worktree from
GitHub's commit, unless the repository's own branch has commits GitHub does not
have; a failed fetch starts from the last known commit, and with no `github.com`
origin it resolves a local branch first, then a remote-tracking branch, as it
always has ([Worktrees](terminal-worktrees.md#creating)). The fetch uses that
machine's own git credential helpers and its `gh` sign-in, and an ssh origin is
fetched over https with `gh`, or not fetched at all when the machine has no
`gh`. An older connector never fetches: it makes the worktree exactly as it
always did, and reports no base. A missing branch answers 422 `base_missing`,
naming the machine in the sentence. An empty base leaves the branch to the
machine: a `base-fresh-v1` connector starts from the repository's default branch
on GitHub, and an older one keeps the existing HEAD behaviour.

Effort accepts `low`, `medium` or `high`. Cline 3.0.74 maps these to its
confirmed `--thinking` option, Claude Code 2.1.319 and Antigravity CLI 1.5.0 to `--effort` (Claude Code's own help also lists `xhigh` and `max`, which a launch does not carry), and Codex to a `-c model_reasoning_effort=<level>` override (its CLI does not check the value, so only these three are sent; `ultra` is not, because a model that lacks it falls back to `medium`); other presets have no effort mapping until their
exact CLI arguments have captured help evidence. An
unsupported mapping answers `launch_option_unsupported`; it never silently
ignores the choice. YOLO maps to a reviewed option only for Claude Code
(`--dangerously-skip-permissions`), Codex (`--ask-for-approval never`), OpenCode (`--auto`), Cline
(`--yolo --tui`) and Antigravity (`--dangerously-skip-permissions`); every other preset refuses it with
`launch_option_unsupported`. A machine switch (`reet terminal yolo off`) refuses
it with `yolo_refused`, and the account setting must allow it first. A Claude Code chat in Gaffer takes YOLO through the
same three gates: the account setting, the machine switch and the confirmation.

Terminal resume exists for OpenCode (`--continue`, and `--session <id>`), Claude Code (`--continue`, and
`--resume <id>`) and Codex (`resume --last`, and `resume <id>`, a subcommand rather than a flag). `--continue` and
`resume --last` continue the tool's most recent conversation in the launch folder. For Codex that is the latest session there that is not a `codex exec` one, and a Gaffer chat's session counts: if a chat in the folder is newer than the terminal session, Continue opens the chat's conversation. When the folder has no such session, Codex starts a new conversation without an error, so a Continue that shows an empty welcome screen was not a resume. Resume, a session id and effort need a `codex` of 0.156.1 or newer, the oldest version they were measured on: the connector reads `codex --version` before starting and refuses an older one with `launch_option_unsupported`, so the machine's CLI never answers with its own parse error. A version it cannot read is not refused. A launch with none of those options, such as a plain start, a model, a prompt or YOLO, never runs `--version`. Older CLIs were probed only far enough to set the floor: `codex resume` exists from 0.38.0 and not in 0.34.0, but through the stand-in provider the effort override reached no request on 0.38.0, 0.60.1 or 0.100.0, so the floor stays at the version that was measured working. The preset carries
`--resume <id>` for a Claude Code session id, but nothing in the browser starts it yet: the only path that sends a session
id is the switch from a running Gaffer chat to the agent's own terminal, which the web offers for OpenCode only. Whether a
Claude Code chat's id is one the CLI's `--resume` opens is not measured, because only the CLI's help was captured. Offering
that switch for Claude Code is a separate, later change. For Codex, a chat's id was measured to open under `codex resume <id>`, from the same
folder and from another, with the earlier turns reaching the provider; that was run against a stand-in provider on codex-cli 0.156.1 and
0.166.0, not with a signed-in account. codex-cli 0.166.0 also keeps a background app-server that can hold a session after its terminal closes,
and a `resume` with a changed configuration can then show "open in another app"; closing that session first avoids it.

Cline 3.0.74 is enabled after a pinned, help-only installation confirmed its
`--auto-approve false --tui` safety and interactive arguments, `--model` and
`--thinking` options. The display catalogue includes Pi and Cursor Agent with the
reason "reet doesn't start this agent yet." Pi 0.73.1 help was captured, but
its lack of a built-in approval policy and its prompt parser's file expansion
leave its safe launch boundary unresolved, so it remains disabled. Cursor Agent
remains disabled because the official download destination was denied by this
environment's network policy and no real help capture could be obtained. That is
a validation limitation, not a runtime refusal shown to users. The owner
authorised the pinned help-only installations; no agent task was run.
The web catalogue holds display names only and never supplies executable paths
or argument vectors. No additional agents were inferred from an unavailable
owner list.

Antigravity CLI 1.5.0 is enabled after capturing only version and help from its
official Linux archive in an isolated home and project. The checksum matches the
Homebrew `antigravity-cli` cask. The connector resolves only `agy`, reports the
`antigravity` tool id only after both `agents-v2` and `antigravity-cli-v1` are negotiated, and passes an opening prompt through
`--prompt-interactive` as one argument. No print, approval-bypass or mode flag is
sent for an ordinary launch. Terminal model listing and terminal resume remain
unreviewed. The separate native Gaffer integration requires `antigravity-acp-v1`;
see [AGY in Gaffer](https://github.com/eyupio/reet.sh/blob/main/docs/structured-sessions.md#google-antigravity-agy-in-gaffer).

The session listing retains tool, model and effort; it never includes the opening
prompt. The launch audit records `effort`, `base` and `yolo`, without prompt text.
