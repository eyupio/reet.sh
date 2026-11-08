# Claude Code, fully supported (design)

Date: 2026-11-08. Status: design approved in conversation by the product owner; this spec awaits their review
before an implementation plan is written.

Claude Code is one of four agents that are not yet at the bar the best-supported agents reach. This is the first
of four specs (Claude Code, then Codex, Pi, Cursor Agent), each with its own plan and pull requests. The bar, as
the owner set it, is the one OpenCode, Google Antigravity (AGY) and the other fully supported agent already meet,
"as far as the agent's CLI allows": a Gaffer chat with models, approvals, stored history and in-place reconnect,
and a terminal launch with resume.

## Where Claude Code stands

- **Gaffer chat.** Built end to end, but dark: a machine reports it only when its agent process runs with
  `REET_EXPERIMENTAL_CLAUDE_CHAT=1`, an environment variable on the connector. The owner has ruled that settings
  live in the database, not in environment variables, so this switch is replaced (see "The switch moves to the
  database"). The descriptor reports approvals and a read-back model switch. History,
  images and in-place reattach are off because they were not verified. The options the adapter reports
  (`effort`, `fast`, `ultracode`) are listed in the descriptor but have no control.
- **The adapter is a second program.** The `claude` CLI does not speak ACP. Gaffer talks to the separately
  installed `claude-agent-acp` adapter (pinned 0.99.1), which must be on the machine's PATH. Without it Claude
  is reported missing with no explanation.
- **Terminal launch.** Works, with a model and an opening prompt only. No resume, no open-session, no YOLO, no
  effort. The web catalogue marks Claude without `effort`, `resume` or `yolo`.
- **The gate.** `docs/device-checks.md` holds the checks only a signed-in device can close. For Claude, the
  signed-in run (Run B) has not been recorded, so the chat stays off by default.

## Intent and success

**Outcome.** A person with Claude Code on a machine gets what they get with the best-supported agents: start a
Gaffer chat with a model and thinking level, continue a closed chat with its history, have a lost chat reconnect
where it was, and start or resume a terminal session with the usual launch options.

**Success looks like.**

- Every capability the descriptor turns on was measured in a recorded probe. None is assumed from another agent.
- A Claude chat starts with a chosen effort, and the chat refuses to start if the adapter does not confirm it.
- A closed Claude chat can be continued with its stored history. A lost one reconnects under the same reet
  session id.
- Terminal Claude offers resume, open-session, YOLO and effort through the same controls and gates as the other
  agents. YOLO is still refused unless the account setting, the machine switch and the confirmation all allow it.
- A machine that has `claude` but not its adapter says so and shows the exact install command.
- Docs say precisely what was measured and what was not.

**Not in this pass.** Managed install of the adapter (reet does not install it). Claude's other permission modes:
the profile keeps requiring `default`. Typed questions (the shared phone and screen-reader gate). Codex, Pi and
Cursor Agent. Making Claude chats on by default for every account: that is the last step and waits for the owner's
signed-in run. Other environment variables the product reads are not touched.

## The switch moves to the database

Settings live in the database, never in environment variables. Today `REET_EXPERIMENTAL_CLAUDE_CHAT` (and Codex's
twin) decides, on the connector, whether Claude's chat is reported. That decision becomes a stored setting that the
controller applies:

- **A per-account setting.** For each gated agent (today `claude` and `codex`) an account has an on/off choice, kept
  in the account profile beside the saved default model. Only explicit choices are stored; an account with none gets
  the agent's built-in default, which is off for both until the owner records the signed-in run. It is read by any
  signed-in account, written by writers only, audited by agent and value, and part of the profile backup. It is shown in
  Settings > Agents as "Gaffer chats still being verified", with a line saying the agent has not been checked with a
  signed-in account.
- **The controller decides what the web sees.** Whatever a machine reports, the controller returns a gated agent's
  chat descriptor as unverified, with no capability flags, for an account that has it off. A create that names the
  agent is refused because it is not advertised. The change takes effect at once, with no reconnect.
  The mask is applied when a record is read for use and never to the stored record: code that reads a record in
  order to write it back, such as the software-update merge, uses an unmasked read, so the connector's own report is
  never overwritten and turning the chat on takes effect without a new report.
- **The connector stops deciding.** It reports Claude and Codex as it finds them (the adapter on PATH), but only to a
  controller that advertises a new feature, `chat-gate-v1`. Toward an older controller, which cannot apply the setting,
  the two stay unverified exactly as before. The environment variables, their constants and their checks are deleted.
- **Making Claude on by default later** is a one-line change to the built-in default plus tests, with no data
  migration, because only explicit choices are stored and an account's own choice is kept.

This is its own slice, before the parity work, so no later slice is built on the environment switch.

## Slices

One spec, five slices. Each is its own pull request and is green before the next starts.

### 1. Probe (no product change)

Run the pinned adapter against the local loopback stub, which needs no login, and record what reet's client
actually sees. Extend `scripts/probe-claude-lifecycle.py` or add a sibling script for:

- `session/load` replay through reet's client: that the replay is dropped and the controller keeps its own
  transcript, as it does for the other agents.
- Image prompts: that an image reaches the stub, and what the adapter does with an unsupported type.
- Setting `effort`, `fast`, `ultracode` and the model by `session/set_config_option`: that the reply reads the
  value back, what an unknown value does, and whether the option list changes with the model.
- The credential-free path: the "Authentication required" refusal at the first prompt, unchanged.

The output is an update to `docs/research/structured-adapters/probe-report.md` and opt-in real-adapter tests
beside `TestRealClaudeStart`. Nothing in this slice changes what a user sees.

### 1b. The switch moves to the database

As described in "The switch moves to the database" above: the policy and the `chat-gate-v1` feature, the stored setting and the controller's mask, the Settings control, and the deletion of the environment switches. It follows slice 1 and #666 and precedes slice 2.

### 2. Gaffer parity, behind the account setting

Turn on only what slice 1 proved, in the descriptor (`internal/acpagent/describe.go`) and the Claude profile
(`internal/acpagent/claude.go`):

- **History and reattach.** `History` for Claude, and `Spec.Reattach` so that `Describe` advertises in-place
  reconnect and the connector accepts it (the descriptor takes the claim from the profile, so the two cannot
  disagree). A closed chat continues with its stored history as a new row. A chat the adapter never stored
  cannot be loaded and the refusal says exactly that. The binding of stored history to account, machine, tool
  and folder is kept, because the adapter loads any folder.
- **Images.** Only if slice 1 shows the adapter takes them.
- **Effort.** The Start dialog's thinking control and the live control read their values from the session's own
  option list, never from a hard-coded list, since the adapter says the values can differ by model. At launch the
  chosen effort is applied and must be confirmed before the opening prompt; an unconfirmed effort refuses the
  chat. The adapter's `default` is shown as the agent's default and sends nothing.
- **Other options.** `fast` and `ultracode` stay listed and uncontrolled unless slice 1 shows a safe confirmed
  switch, in which case they follow the same rule.
- `ClaudeSessionMeta` (the repository's own settings and hooks are not read) stays on both new and load.

This slice relies on the AGY reconnect fix (#666) being merged first: it makes the descriptor take its reattach
claim from the profile, which is what lets Claude's `Spec.Reattach` switch the claim on.

No new wire feature is expected: history, images, options, approvals and reattach already have negotiated flags,
and the descriptor already carries Claude's options. Slice 2 confirms this in code and adds one only if it finds a
gap.

### 3. Terminal parity and web

The installed `claude` 2.1.319 offers `-c/--continue`, `--resume <session-id>`, `--dangerously-skip-permissions`
and `--effort <level>` (low, medium, high, xhigh, max). In `internal/terminalhost/catalogue.go` the Claude row
gains `ResumeArgs`, `SessionArgs`, `YoloArgs` and `EffortArgs`, pinned to a fresh help capture stored with the
other captures (`internal/terminalhost/testdata/help/claude.txt` and its `sources.json` entry); the recorded
2.1.310 evidence predates the flags. The one-word prompt rule stays. The web catalogue marks Claude `effort`,
`resume` and `yolo`, and the docs list Claude among the agents that take YOLO. A resume or open-session request
uses the Claude session identifiers the controller already validates.

### 4. Adapter hint, final docs and the held flip

The last slice is one pull request, written last and left unmerged until the owner's signed-in run is recorded.
Slices 2 and 3 update the docs for what they change; this one adds the rest.

- **Hint.** When `claude` is on the machine's PATH but its chat adapter is not, the Start dialog and Machine
  settings say so and give the exact pinned install command. Because the controller reports a gated agent as
  unverified for an account that has it off, the hint shows only to accounts that turned the chat on. It ships with
  the final slice.
- **Docs.** A "Claude Code in Gaffer" section in `docs/structured-sessions.md`, and updates to
  `docs/start-agent-session.md`, `docs/terminal-launch.md` (the Claude row and the YOLO list),
  `docs/device-checks.md` and `docs/roadmap.md`. Each says what was measured and what was not.
- **The default.** Claude chat becomes on by default (an account's own off choice is kept), with the hint above.
  The owner's signed-in run is recorded in `docs/device-checks.md` first.

## Verification boundary

What the sandbox can prove: adapter behaviour through the loopback stub, a help capture of the installed
`claude`, fake-agent integration tests, and Playwright against stubbed APIs for the descriptor-gated interface.

What it cannot: a signed-in turn (Run B), whether the real model accepts images (the stub shows only that the
bytes arrive), the phone and screen-reader gate, and the effort values a real account is offered per model.

So every capability the descriptor enables comes from a recorded probe, effort is read live, the docs name what
stays unchecked, and the final flip is the owner's.

## Compatibility and failure

- An older controller never receives new descriptor fields, and an older connector's Claude descriptor is read as
  before. An account with the chat off sees nothing change.
- A controller that does not advertise `chat-gate-v1` (an older one) never sees Claude or Codex reported as available.
- A history load that fails or finds nothing stored says so and offers a new chat.
- An unconfirmed effort, or a model the adapter silently reset, refuses the chat rather than claiming a setting
  it cannot confirm.
- A terminal resume that the CLI refuses ends the pane as any failed launch does.

## Testing

- Go: descriptor and profile pinned together for every chat agent (extending the test that ties a descriptor's
  reattach claim to its profile); fake-agent sessions for effort confirmed, refused and unconfirmed, for history
  continue and for reattach; the terminal row's argument vectors for resume, open-session, YOLO and effort, and
  the refusals that already exist for the other agents.
- The switch: the policy and the feature in the protocol package; the connector reporting a gated agent as available only
  when the gate is negotiated; the stored setting's validation, permission, audit, migration and backup; the controller's
  mask on what a machine reports; a create refused for an account that has the chat off; and no environment variable
  left that decides a setting.
- Opt-in real-adapter tests from slice 1, run against the pinned adapter and the loopback stub.
- Web: unit tests for the catalogue flags and the effort control's values; Playwright for the Start dialog and
  the live chat with a stubbed Claude descriptor, including the missing-adapter hint.
- No real provider or paid model is used.

## Delivery

Spec, then plan, then five draft pull requests in order, each green before the next starts. The fifth carries the
hint, the final docs and the new default, and is not merged until the owner's signed-in run is recorded. The next spec is
Codex, which reuses this one's shape.
