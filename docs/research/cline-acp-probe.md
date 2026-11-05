# Cline ACP probe

5 November 2026. Cline CLI 3.0.74, Linux x86-64. This records what `cline --acp`
does so [Gaffer](../structured-sessions.md) can offer a Cline chat without
guessing. It is the evidence behind `internal/acpagent/cline.go`.

## Scope and method

The CLI was already installed (`cline` 3.0.74, a compiled
`@cline/cli-linux-x64` binary behind a Node resolver). Probes ran as a child
process over stdin/stdout with a temporary project folder and a private `HOME`,
and the machine's own signed-in `~/.cline` reached through `CLINE_DIR`. Framing
is newline-delimited JSON-RPC, which is what reet's client reads.

No prompt was sent except one read request and one write request, both refused
with Cline's own `reject_once`; no file was created or changed. A turn that
needed a second provider was also run once and failed in Cline's own store
without any ACP frame, which is recorded below.

## Exact identity

| Check | Observed |
| --- | --- |
| `cline --version` | `3.0.74` |
| ACP `initialize` | protocol 1; `agentInfo` `{name: "cline", version: "3.0.74"}` |
| Agent capabilities | `loadSession: true`; `promptCapabilities` image true, audio false, embeddedContext false |
| Auth methods | `cline`, `cline-pass`, `openai-codex` (no `authenticate` call is made by reet) |
| Transport | one JSON object per line on stdio; a banner goes to stderr |
| Signed out (`CLINE_DIR` pointing at an empty folder) | `session/new` fails `-32000` `Authentication required: Call authenticate before starting a session` |

The last row is why the connector names the sign-in step instead of retrying.

## A session

`session/new` with `--auto-approve false` returned:

- `sessionId`: `<13-digit millisecond task id>_<5 alphanumeric>_cli`, for
  example `1793848855723_8zvJS_cli`.
- `modes`: `plan` and `act`, with `act` current. The descriptions are Cline's
  own: plan "explore the codebase and plan changes without modifying files", act
  "make changes to the codebase".
- `models`: the provider's catalogue and the current model.
- `configOptions`: `provider` (category `_provider`), `model` (category
  `model`), `mode` (category `mode`), and `auto_approve`, a **boolean** whose
  value was `false` because of the launch flag.

`session/set_config_option` accepts `{configId: "model" | "mode", value}` and
answers with the whole `configOptions` list, so a switch is confirmed by reading
the value back. An unknown `configId` is refused with `-32602`. The older
`session/set_mode` also works.

Cline ignores `--provider` and `--model` in ACP mode: it reads `CLINE_PROVIDER`
and `CLINE_MODEL` from the environment instead. Reet does not inherit those
redirects, so a chat uses the provider Cline restores and the model the person
picks in Gaffer.

`session/load` replays the stored conversation as ordinary `session/update`
frames and returns the modes and model list. It loaded both a session this probe
created and a task id created by Cline's own TUI (`1793848303972_xxmw4`).

A session Cline never stored answers `-32002 Resource not found: <id>`, which is
what a chat with no prompt gets: reet reports that as "no stored conversation"
rather than retrying a load that cannot succeed.

## Approvals

With `--auto-approve false`, every tool call produced
`session/request_permission` before it ran, with the options `allow_once`,
`allow_always` and `reject_once`:

| Tool call | `kind` | `rawInput` | Card reet builds |
| --- | --- | --- | --- |
| `read_files` | `read` | `{files: [{path}]}` | the first file as the card's path |
| `editor` | `edit` | `{path, new_text, old_text?, insert_line?}` | `+` lines from `new_text`, with `-` lines when `old_text` is present |
| `run_commands` | `execute` | `{commands: [..]}` | the commands, one per line |

A rejection came back to the model as a failed tool call, the turn continued and
no file was created. This verifies the three paths above; it does not cover MCP
tools, prompts with images or subagents.

The boolean `auto_approve` option is the acknowledgement reet pins. The launch
flag is a request; a session that reported `true` would be refused before its
first prompt.

## Failure without an ACP frame

One turn used a provider whose balance was empty. Cline wrote the provider error
into its own message store as a display-only error and ended the turn with no
`agent_message_chunk`, so nothing reached the client. reet therefore shows a
finished turn with no reply. This is a known limit: Cline has no log mirror like
OpenCode's for reet to read the reason from.

## What this does not establish

- A successful turn with images, MCP tools or a subagent.
- That every future 3.0.x reports the same ids, option names or kinds. The
  version floor is 3.0.74, and a changed shape is refused at launch rather than
  guessed.
- That a signed-in Cline on another machine has the same provider selection; the
  descriptor carries no version or login claim.
