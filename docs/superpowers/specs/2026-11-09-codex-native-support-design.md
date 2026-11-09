# Codex fully supported natively: design (draft)

Status: agreed in outline with the owner; slice 1 is done (see the probe report).
Modelled on `2026-11-08-claude-code-native-support-design.md`. Evidence comes from a read-only survey of the code, `docs/device-checks.md`
and the captured help, and, for slice 1, from running the real adapter signed out (see the probe report).

## Goal

Codex is a first-class agent in both ways of working with it: a structured chat in Gaffer (via the `codex-acp` adapter) and a terminal
session launched from the Start dialog. "Fully supported" means each capability reet shows for Codex is one that was measured to work,
not assumed, and each one that does not work is hidden or explained.

## Starting point

- The per-account chat gate (slice 1b of the Claude plan) already covers Codex. Chats are off by default (`ChatDefaultOn` is false). Turning them
  on by default is the final, separate step.
- Chat works with approvals only. The descriptor reports `Approval` and nothing else; model, history, images, reattach and questions are false.
- Permissions are built: a chat starts in `read-only`, a switch is confirmed, escalations show a card with the justification, rejecting ends the turn.
- Effort and model have generic paths (live control, Start dialog) that were never proven against `codex-acp`.
- Terminal presets cover model, prompt and a YOLO flag only. Resume and effort are refused.
- Only a signed-out run (Run A, 4 November) was ever recorded. The signed-in run (Run B) was deferred by the owner.

## Where Codex differs from Claude Code

- Sign-in is checked at `session/new`, not at the first prompt, so Run B gates starting any chat, not only the default-on flip.
- Effort is `reasoning_effort` (`low` to `ultra`, no `default`) and its values depend on the model; the adapters measured on 9 November carry no effort in model ids. A model switch keeps the chosen effort where the new model offers it.
- Modes are three sandbox modes plus `collaboration_mode` (`default`/`plan`, not exposed). Approvals are model-driven escalations. `allow_always` is a persistent rule and is not offered.
- Terminal: `resume` is a subcommand (`codex resume [--last] [id]`), effort is a `-c key=value` override with no flag, the existing YOLO flag keeps the sandbox, and the real bypass is a different flag.
- Two packages are involved (the adapter and `@openai/codex`), and the captured help (0.163.0) and the chat probes (0.156.1) are different versions.

## Slices

1. **Probe, no product change.** Extend the Codex probe to measure: effort and model read-back after `set_config_option`, options per model,
   image capability and the unstored-load error code, with an opt-in `TestRealCodexStart`. Done, results in the probe report ("Codex parity probes"); history replay needs a stored turn and waits for Run B. The pinned CLI help capture and its `sources.json` entry move to slice 3, where the terminal preset needs them, as they did for Claude Code.
2. **Gaffer parity, behind the setting. Done, with the lifecycle measured against a stand-in provider** (see "Codex lifecycle probes" in the probe report) so history, images and effort did not have to wait for Run B. Descriptor flags exactly as the probe shows (history, model, images, reattach, read-back confirmation), fake-frame tests,
   a chat-YOLO test for Codex, and wording for what YOLO means in `read-only` versus `agent`.
3. **Terminal and web.** Resume, effort and image argv for the terminal preset once the decisions below are made; the web catalogue and Start dialog follow; docs rows.
4. **Hint, docs, flip.** A missing-adapter hint generalised per tool (naming both packages), a "Codex in Gaffer" docs section, then the default-on flip as its own small PR after Run B.

## Decisions

Settled with the owner:

1. **Run B** is recorded in the same sitting as Claude Code's signed-in run (steps in `docs/device-checks.md`). Slice 1 and every signed-out part proceed now; rows that need a signed-in session are marked "waiting for Run B" and are not claimed until measured.
2. **Terminal YOLO** stays `--ask-for-approval never` (the sandbox stays). Wording says so; no full-bypass option.
3. **Terminal effort** uses `-c model_reasoning_effort=<value>` only once a probe confirms Codex accepts it, with the allowed values taken from that probe. Until then Codex terminals keep refusing effort.

Defaults I will follow unless the owner says otherwise (each is cheap to reverse):

4. **Resume** is decided from the probe: it ships only if a chat's session id is shown to open with `codex resume <id>`; otherwise terminal resume stays off for Codex and the docs say why.
5. **Chat YOLO** auto-allows escalations in `read-only` and is refused in `agent`, where Codex already decides.
6. **Not exposed:** `collaboration_mode` plan, `fast-mode` and `request_user_input`, until measured.
7. **Pins** are a version floor, as for Claude. Slice 1 records the CLI and adapter versions it measured; raising them is a slice 1 outcome, not a decision up front.

## Not verified

Real-login behaviour; whether current `codex-acp` still matches the 1.13.1 measurements; anything needing the `codex` binary.
