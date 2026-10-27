# Launch requests: starting an agent in a terminal session

A launch request asks a machine to start one of its installed agent CLIs (`claude`,
`codex`, `opencode`, `gemini`) in a chosen folder, with an optional model and an
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
operation and no decoder relaxation. The control frame limit (16 KiB) is
unchanged.

## Wire fields and limits

`terminalproto.Create` gains four optional flat string fields, so it stays a
comparable struct and `omitempty` keeps a plain create unchanged. They are named
`tool`, not `agent`, because `agentId` already means the machine.

| Field | Meaning | Limits |
| --- | --- | --- |
| `tool` | Tool id | One of `claude`, `codex`, `opencode`, `gemini` |
| `model` | Model id, optional | 1 to 100 bytes; letters, digits and `. _ : / @ + -`; the first character must be a letter or digit |
| `dir` | Folder, optional | Absolute; at most 4,096 bytes; valid UTF-8; no control characters |
| `prompt` | Opening prompt, optional | At most 2,000 bytes; valid UTF-8; no control characters except newline; must not begin with `-` |

`model`, `dir` or `prompt` without `tool` is refused. Strict decoding is
unchanged: an unknown field is refused. The controller and the connector both
validate; the connector validates again before it starts anything.

The leading `-` rule exists so that a prompt can never be read as a flag by a tool
that takes the prompt after a flag of its own (see the presets).

## Presets

The table is built into the connector. A tool is offered only if it is in the
table and `exec.LookPath` finds its binary on the connector's `PATH`. The resolved
path must also be absolute and contain only letters, digits and `. _ + @ / -`,
because tmux hands a one-argument command to `sh -c`; a binary at any other path
is treated as not installed. The model and the prompt are each exactly one
argument element. No preset uses an approval-bypass, auto-approve, print, exec or
other one-shot flag; a test pins the only flags used to `--model`, `--prompt` and
`--prompt-interactive`.

| Tool | Arguments (with a model and a prompt) | Confirmed from |
| --- | --- | --- |
| `claude` | `claude --model <model> -- <prompt>` | `claude --help` on Claude Code 2.1.310: "Usage: claude [options] [command] [prompt]", "starts an interactive session by default, use -p/--print for non-interactive output", "--model <model>  Model for the current session". The bundled argument parser was read to confirm that `--` ends options. |
| `codex` | `codex --model <model> -- <prompt>` | https://learn.chatgpt.com/docs/developer-commands?surface=cli (reached from https://developers.openai.com/codex/cli/reference): "Running `codex` with no subcommand launches the interactive terminal UI", `--model, -m`, and an optional `PROMPT`. Source: `codex-rs/tui/src/cli.rs` in openai/codex has a positional `prompt: Option<String>`. |
| `opencode` | `opencode --model <model> --prompt <prompt>` | https://opencode.ai/docs/cli/: "opencode [project]" starts the terminal UI, with `--prompt` and `--model`. The positional is a project path, so the prompt goes through the flag. |
| `gemini` | `gemini --model <model> --prompt-interactive <prompt>` | https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/cli-reference.md and `packages/cli/src/config/config.ts`: `--prompt-interactive` runs the prompt and continues interactively; `--model` sets the model. `-p/--prompt` is headless and is deliberately not used. |

With no model the `--model <model>` pair is left out; with no prompt the prompt
part is left out. All rows were confirmed on 2026-10-25. Only `claude` was
confirmed against a binary's own help text; the others were confirmed from the
documentation and source cited. A tool whose syntax cannot be confirmed is not
offered. `pi` and `cursor` are not offered for that reason.

### The one-word prompt rule (claude and codex)

`claude` and `codex` take the prompt as an operand after `--`. Their parsers can
still treat the first operand as a subcommand name after `--`: Claude Code's
parser would run `claude purge`, `claude update` or `claude install` for a prompt
that is just `purge`, `update` or `install`. A subcommand name is one word, so for
these two tools a prompt must contain a space or a newline. The same rule is
applied to `codex` as defence in depth; whether codex needs it is not documented
(see "Known limits"). `opencode` and `gemini` take the prompt as the value of a
flag and are not affected. The controller refuses a one-word prompt before it
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
- `opencode` and `gemini` are safe from a prompt being read as a flag only because
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
that token. Without it, the connector reports only the original four tool ids
and never sends the added capability fields. OS version is informational: Linux
reports its kernel release and macOS its product version; a failed local reading
leaves it absent.

Create accepts optional `base`, `effort` and `yolo`. An older peer receives none
of those fields and no new tool id: the controller answers 409
`launch_option_unsupported` with "Update reet on that machine to use that option."
A named base requires a worktree. The connector resolves a local branch first,
then a remote branch, to a commit before creating the worktree. A missing branch
answers 422 `base_missing`, naming the machine in the sentence. An empty base
keeps the existing HEAD behaviour.

Effort accepts `low`, `medium` or `high`. Cline 3.0.74 maps these to its
confirmed `--thinking` option; other presets have no effort mapping until their
exact CLI arguments have captured help evidence. An
unsupported mapping answers `launch_option_unsupported`; it never silently
ignores the choice. YOLO remains refused (`yolo_refused`) in this pass.

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

The session listing retains tool, model and effort; it never includes the opening
prompt. The launch audit records `effort`, `base` and `yolo`, without prompt text.
