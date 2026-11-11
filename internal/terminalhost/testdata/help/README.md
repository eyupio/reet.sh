# CLI help captures

Each .txt starts with the actual version output followed by its actual help output.
Pi routes these outputs to stderr in this noninteractive capture; its stdout was empty.
`sources.json` records pinned official package sources and SHA-512 integrity.
Captures were made with the owner’s permission for install/version/help only.
No model request, task, interactive agent session or ACP command was run.

Cline 3.0.74 help establishes `--auto-approve <boolean>` (default true),
`--tui`, `--model`, and `--thinking` low/medium/high. The preset explicitly
passes `--auto-approve false --tui` before other options and the positional prompt.
Single-word prompts are refused because command dispatch may treat them as subcommands.
The help also includes `--acp`; that mode was measured separately, without a
model turn except two refused tool calls, in
[docs/research/cline-acp-probe.md](../../docs/research/cline-acp-probe.md).

Claude Code 2.1.319 help establishes `-c, --continue` (continue the most recent conversation in
the current directory), `-r, --resume [value]` (a session id or name), `--dangerously-skip-permissions`
and `--effort <level>` (low, medium, high, xhigh, max). Only version and help were run.

Codex CLI 0.166.0 `--help` and `resume --help` establish `-a/--ask-for-approval`, `-m/--model`, `-c/--config <key=value>` and a `resume` subcommand
(`[SESSION_ID] [PROMPT]`, `--last`) that takes the same options. There is no effort flag: the preset passes `-c model_reasoning_effort=<level>`, whose key the
help does not name and which a stand-in-provider probe showed reaching the provider's request. Only version and help were run for the capture. The earlier
capture was 0.163.0.

Pi 0.73.1 is installed and captured but remains disabled. Its packaged README says:

> **No permission popups.** Run in a container, or build your own confirmation flow with [extensions](#extensions) inline with your environment and security requirements.

Its packaged `dist/cli/args.js` treats an `@`-prefixed argument as a file operand
and `--` as an unknown flag that can consume the next argument, rather than as
an end-of-options separator. No safe ask-for-permission mode was found; a
no-tools launch would be a different product behaviour and is not enabled.

Cursor Agent remains disabled because the official `https://cursor.com/install`
destination was denied by the environment proxy. An unrelated package named
`cursor-agent` was not used as a substitute.


OpenCode 1.19.5 is captured from the 28 October 2026 probe report rather than a new run:
version, top-level help, `models --help` and the empty-HOME `opencode models` output with
`OPENCODE_DISABLE_MODELS_FETCH=1`. The probe saw no outbound call with that setting. Only
version, help and the model listing were run; no agent turn, prompt or ACP session.

Antigravity CLI 1.5.0 was downloaded from the official Google Storage archive
referenced by Homebrew's `antigravity-cli` cask on 5 November 2026. Its SHA-256
matched the cask before extraction. Only `--version` and `--help` ran, under
`work/agy-capture` with isolated home, XDG directories and project and without
credentials. The archive calls its executable `antigravity`; the cask installs
the `agy` alias, which is the only binary this preset resolves. The capture
confirms `--model`, `--prompt-interactive`, `--effort` low/medium/high and the
separate `--dangerously-skip-permissions` flag. `--prompt` aliases print mode and
is never used. Resume, ACP and model listing were not run or enabled.
