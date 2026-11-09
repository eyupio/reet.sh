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
- `models`: the provider's catalogue and the current model. The catalogue held
  307 ids on the measured machine, so the model selector needs the whole list
  rather than the 16 a live control reply was sized for: reet sends it under
  `acp-model-catalogue-v1` and trims it to 16 for a controller without it.
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
`agent_message_chunk`, so nothing reached the client. Cline also logs the failure
to `CLINE_DIR/data/logs/cline.log` as a JSON line with `level` 50, the session
id, `err.message` and `msg` `Interactive turn failed`; a failed stream adds
`[ai-sdk] provider error` with the provider id. reet reads the newest of those
lines for this chat after the turn started, so the turn now ends with the reason
instead of no reply. The message store itself is still display-only and not read.

## Provider (billing source) switching

Cline's `session/new` reports a `provider` option (category `_provider`) beside the model. reet surfaces it as a
billing source (Usage-Billing for `cline`, ClinePass for `cline-pass`) and switches it with
`session/set_config_option`, confirmed by reading it back, behind the `acp-provider-v1` feature. This has been built
against a fake Cline only. Three facts are **not yet checked against a signed-in Cline**:

1. Whether Cline accepts `provider` in `session/set_config_option` (the probe above proved `model` and `mode`).
2. How a free source is exposed: as its own provider id, or as a filter over the `cline` provider's models. Until it
   is known, Start an agent and Settings offer Usage-Billing and ClinePass, and a live chat shows whatever sources
   Cline reports.
3. Whether a switch is kept for one chat or written to Cline's own folder for the whole machine.

`scripts/probe-cline-provider.py` answers all three without a model call. It copies a signed-in Cline folder to a
private temporary directory, sets each provider the session lists, and reports the read-back, the size of the model
list after each switch, whether a fresh session sees the switch, the files that changed and any provider or model id
that looks like a free source. Run it on a machine where Cline is signed in, then record the result here.

## Gaffer | Cline TUI switching (not yet checked)

The Chat | Terminal switch is offered for OpenCode only (`switchModeState` in `web/src/lib/sessions.ts`), and Cline's
catalogue row has no `SessionArgs`, so `launchArgv` refuses a launch on a chat's id with `launch_option_unsupported`.
Cline's own `--help` lists `--id <session-id>` ("Resume an existing session by ID"), but nobody has checked what
`cline --tui --id <chat id>` does with an id that `cline --acp` created. Three things decide whether the switch can be
built, and the other two parts of it (the controller's `SwitchSessionMode` and the connector's `swap`) already work for
any reviewed chat tool:

1. Does the TUI accept the chat's `_cli` id, or refuse it?
2. Does it keep that id, or start a new task id? reet accepts only `_cli` handles (`validClineSessionID`), so a forked
   id could not be carried back to Gaffer, which is what the TUI's own un-suffixed ids look like.
3. Does `session/load` still open the id after the TUI has run on it?

`scripts/probe-cline-tui-resume.py` answers all three without a model call, against a private copy of `~/.cline`, and
ends with plain-sentence verdicts. Run it on a machine where Cline is signed in, then record the result here before
adding `SessionArgs: []string{"--id"}` to the Cline row and opening the web gate.

## A turn that ends with no reply

Cline ends a failed turn without an ACP frame (see "Failure without an ACP frame"). When it also logs nothing reet can
read for that chat, for example a provider id that Cline does not accept, the chat used to show "Ready for your next
instruction" under an unanswered prompt. It now adds a notice that the agent ended the turn without a reply and gave no
reason. The cause of an empty turn on a real machine is read from the tail of `<CLINE_DIR>/data/logs/cline.log`.

## What this does not establish

- A successful turn with images, MCP tools or a subagent.
- That the `provider` option can be set, how a free source is named, or whether a switch outlives the chat (see above).
- That every future 3.0.x reports the same ids, option names or kinds. The
  version floor is 3.0.74, and a changed shape is refused at launch rather than
  guessed.
- That a signed-in Cline on another machine has the same provider selection; the
  descriptor carries no version or login claim.

### Interpreting the TUI resume probe

A failed or missing history read means continuity is unknown. A TUI process that stays alive with `--id`, or a successful history comparison showing no new task id, does not by itself establish that the TUI displayed the same conversation. A successful `session/load` afterward establishes that the id can be loaded, not that transcript continuity through the TUI was verified. Keep the switch disabled until the signed-in content check establishes that behavior.

Credential-free verdict regressions run through `go test ./scripts -run '^TestClineTUIProbeVerdicts$'` when Python 3 is installed, or directly with `python3 -B scripts/test_probe_cline_tui_resume.py`. They cover failed/missing history, TUI refusal, absent chat ids, no observed new ids and a possible fork. They start no Cline process and spend no model tokens.
