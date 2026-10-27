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

Pi 0.73.1 is installed and captured but remains disabled. Its packaged README says:

> **No permission popups.** Run in a container, or build your own confirmation flow with [extensions](#extensions) inline with your environment and security requirements.

Its packaged `dist/cli/args.js` treats an `@`-prefixed argument as a file operand
and `--` as an unknown flag that can consume the next argument, rather than as
an end-of-options separator. No safe ask-for-permission mode was found; a
no-tools launch would be a different product behaviour and is not enabled.

Cursor Agent remains disabled because the official `https://cursor.com/install`
destination was denied by the environment proxy. An unrelated package named
`cursor-agent` was not used as a substitute.
