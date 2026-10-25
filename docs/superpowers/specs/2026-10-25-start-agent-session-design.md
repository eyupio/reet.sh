# Start an agent session (S6 design)

Date: 2026-10-25. Status: design for review; no code written.

Pick a machine, a folder, an AI agent and a model, press Start, and land in a live session already
running that agent. The product owner calls this core to the new reet.sh: agents, models and the
ability to start working on one are what turns the machines list into a place to do work. This is
the first of two sub-projects. The second, **Projects** (choosing several repositories to work on
together), gets its own design afterwards and reuses the folder list this one builds.

## Intent and success

**Outcome.** A signed-in owner with a write role can start, from the Terminals workspace or from a
ready machine's card, a browser terminal session that runs a chosen AI agent CLI with an optional
model and opening prompt in a chosen folder on that machine, and be taken straight into it.

**Who it is for.** The account owner on a desktop or a phone, on machines they have already
enrolled and consented to browser terminals on.

**Success looks like.**

- Starting an agent is one dialog and one press. A failure says in plain words what to do next.
- The controller never sends a shell string. The machine decides how each agent is launched from its
  own allowlisted table, and an older machine is told to update rather than dropped.
- Nothing on screen claims more than happened. A launched session is **started**, not "working" or
  "healthy". Agent status is a later slice.
- Retrying a lost request never starts a second agent.

## Decisions already taken with the product owner

| Question | Decision |
| --- | --- |
| How does the agent start? | A first-class launch request in the create-session call, validated and executed by the connector against its own allowlisted presets. Not a line typed into a shell. |
| How is the model chosen? | Suggestions per agent plus an "Other" field that accepts any model id made of letters, digits and `. : / @ + -`; passed as one argument, never through a shell. |
| Where does it start? | A folder picked from folders the machine already reports plus a typed path, checked by the connector. |
| Where is it offered? | "New agent session" beside "New shell" in the Terminals workspace, and "Start an agent" on each ready machine's card. |
| What about Projects? | Its own design afterwards. |

## Out of scope

Agent status (working, waiting, idle), resuming a past agent conversation, choosing several folders
or repositories (Projects), worktrees, MCP exposure, applying skills before launch, permission-bypass
or "dangerously skip" flags for any agent, per-agent settings screens, Windows, and any change to the
security/PIN preference or the legacy sync flows.

## What the code does today (verified)

- The browser calls `createTerminal(agentId, requestId, title, cols, rows)`; the body has no folder,
  command, tool or model. In the transport "agent" means an enrolled **machine** (`agentId`).
- The connector starts every session with `tmux new-session -d -s <hashed name> -x <cols> -y <rows>
  -c <home> -e HOME=<home>`. There is no command argument, so tmux runs the user's default shell in
  their home directory. The working directory is fixed to the enrolled home.
- The control protocol is strict: unknown fields close the connection. `terminalproto.Create` must
  stay a **comparable struct**, because the host compares `*op.Create != in` to detect a retry with
  different parameters. The idempotency key is the stored JSON of the whole `Create`.
- Optional features are negotiated with a header on the private upgrade (`X-Reet-Terminal-Features`)
  and a capability flag, the way command history is (`docs/terminal-command-history.md`). A peer
  without the flag is refused with a plain 409 and never sent the new field.
- Capabilities (`terminalproto.Capabilities`) already flow from the connector to
  `/api/v2/terminal/machines` and `/api/v2/machines/health` and the web's `Capabilities` type.
- The agent CLI identifiers already exist for skills (`internal/setup/adapters.go`: `claude`,
  `codex`, `opencode`, `pi`, `cursor`, `gemini`, with a binary name each, found with `exec.LookPath`).
  Nothing in the repository knows any launch flag.
- Terminal create writes no audit event today.

## The flow

1. **Open.** *New agent session* in the Terminals workspace's sessions sidebar (beside *New terminal*)
   or *Start an agent* on a ready machine's card opens `StartAgentDialog.svelte`. Viewers and legacy
   auth modes do not see either. A machine that is not ready is disabled with the existing plain
   readiness sentence.
2. **Choose.** One screen, no wizard:
   - **Machine**: the ready machines, with the one you came from preselected. A final link, *Not
     listed? Add a machine*, opens the add-a-machine dialog (S5).
   - **Agent**: the tools this machine reports as installed, each with its display name. A tool the
     machine does not have is listed disabled with "Not installed on this machine." An old machine
     shows "Update reet on this machine to start agents."
   - **Model** (optional): suggestions for the chosen agent, *Agent default*, and *Other* (a text field
     checked against the character rule). Nothing is sent when it is left on the default.
   - **Folder**: the machine's home and the project folders it reports, plus *Type a path*. The
     connector decides whether a path is allowed; the dialog only checks it is absolute and shows the
     plain refusal if the connector declines.
   - **Opening prompt** (optional, up to 2,000 bytes): what the agent should begin with.
   - A line says exactly what will happen: "Starts <Agent> in <folder> on <machine>. Nothing runs
     until you press Start." and, when a prompt is given, "The prompt is visible to other users of
     that machine in its process list."
3. **Start.** One request with a request id kept for retries. On success the dialog closes and the
   session is opened in the workspace (or the Terminals page from the Machines page), titled
   `<Agent> · <folder name>` unless the person typed a title. On failure the dialog stays open and
   says what went wrong; *Retry* reuses the same request id and the same parameters.

### Words

State and error sentences are plain and exact; at most one light Yorkshire phrase in a heading, lede
or success line. The word "verified" never describes a machine or a session. Refusals map to
sentences:

| Cause | Sentence |
| --- | --- |
| Machine offline or reconnecting | "That machine is reconnecting. Try again in a moment." |
| Tool not installed (known to the controller from the capability) | "<Agent> isn't installed on that machine." |
| Machine too old for launch requests | "Update reet on that machine to start agents." |
| Folder refused by the connector | "That folder isn't available on that machine." |
| Session limit | "That machine already has the most sessions it allows." |
| Local consent switched off | "Browser terminals are switched off on that machine." |
| Same request id, different parameters | "That request changed. Start it again." |
| Anything else | "Something changed. Check again." |

## The launch request

A negotiated, optional extension to the create-session call. Nothing changes for a plain shell.

- **Wire fields.** `terminalproto.Create` gains four optional **flat string** fields so it stays
  comparable and `omitempty` works: `Tool` (the tool id, for example `claude`), `Model`, `Dir`
  (absolute path) and `Prompt`. They are named `tool`, not `agent`, because `agentId` already means the
  machine. The whole `Create` stays the idempotency key: a retry with different launch fields is the
  existing 409.
- **Validation (controller and connector both).** `Tool` is one of the closed set of ids the
  connector's table knows, `Model` matches `^[A-Za-z0-9][A-Za-z0-9._:/@+-]{0,99}$`, `Dir` is
  absolute, at most 4,096 bytes with no control characters, `Prompt` is at most 2,000 bytes, valid
  UTF-8, with no control characters other than newline, and **must not begin with `-`** (so it cannot
  be read as a flag). `Model` or `Prompt` without `Tool` is refused. The frame limit
  (`MaxControlBytes`, 16 KiB) is unchanged.
- **Negotiation.** A new token in `X-Reet-Terminal-Features` and a capability flag `launch`
  (omitempty, requires `terminalControl`) plus `tools` (the installed tool ids, omitempty). The broker
  refuses to send launch fields to a peer without the flag and the API answers with the "Update reet"
  sentence. No protocol version bump.
- **Connector presets.** One table in the connector, keyed by tool id, holds the binary name, how a
  model is passed (`--model <id>` or `-m <id>` as one `flag value` pair of arguments) and how an
  opening prompt is passed. It is built into the binary, never taken from the controller. A tool is
  offered only if `exec.LookPath` finds its binary on the connector's `PATH`; the installed ids are
  reported as `tools`. **Each preset's flags are checked against the real CLI's help text on a machine
  that has it, or against a recording fake in tests; a tool whose syntax cannot be confirmed ships
  disabled.** The first version targets `claude`, `codex`, `opencode` and `gemini`; `pi` and `cursor`
  follow once their launch syntax is confirmed.
- **Starting.** The connector appends the program and its arguments to `tmux new-session` as separate
  arguments (tmux executes several arguments directly rather than through a shell; the real PTY test proves
  this on the tmux 3.2 minimum the connector already requires, and the design falls back to refusing
  the launch, never to a shell string, if it ever did not hold). The working directory is the validated `Dir` instead of the home. The
  environment is the existing sanitised allowlist; the model never travels as an environment
  variable. `remain-on-exit` stays on, so a session whose agent exits keeps its exit status visible.
- **Folder policy.** `Dir` defaults to the enrolled home. If given, the connector resolves it with
  symlinks followed and requires it to exist, to be a directory and to lie inside the enrolled home
  (this slice keeps the home as the boundary; wider roots would be a deliberate later setting). A
  refusal is the new result code `launch_failed`, mapped to the folder sentence.
- **Errors.** `launch_failed` is a new `ResultCode` used for a folder refusal, a missing binary or a
  failed start; the controller marks the create failed, the session never appears as running, and the
  web gets a 422 with the matching sentence. The connector's unknown-error mapping to `Unavailable`
  is unchanged for everything else.
- **Quota and consent** are unchanged: 8 sessions per machine and 32 per account, the local consent
  re-read on every operation, owner-only access, viewers blocked, origin checks.
- **Audit.** A new audit event records a launch (account, machine, tool, model, folder; never the
  prompt) so Activity can say "Started Claude Code in ~/work/site on studio".

## Browser structure

- `web/src/lib/agentLaunch.ts` (+ `agentLaunch.test.mjs`, added to `npm test`): the tool display
  names, the model **suggestion** lists (hints only; the connector decides what runs), `validateModel`,
  `validatePrompt`, `folderChoices(machine, snapshot)`, `launchSentence(...)`, `refusalText(status,
  code)` and `titleFor(tool, dir)`. Pure and tested.
- `web/src/lib/terminal/api.ts`: `createTerminal` takes an optional `launch` object and sends the
  four fields only when set; `TerminalMachine.capabilities` gains `launch` and `tools`.
- `web/src/lib/StartAgentDialog.svelte`: the dialog above. Native `<dialog>`, fenced reads, one
  immutable operation per attempt (request id plus the exact parameters), a busy guard, the
  existing 15-second abort, focus returned to the opener.
- Entry points in `SessionSidebar.svelte`/`TerminalWorkspace.svelte` and the machine card in
  `MachineFleet.svelte`; landing reuses the workspace's session switch and `QuickTerminalButton`'s
  navigation to `/terminals?agent=…&session=…`.
- Nothing in browser storage; the opening prompt is kept in memory only and cleared on close.

## Safety, accessibility and layout

- Accounts mode, write role, canonical origin and the rollout flag, exactly as for shells.
- The controller sends only validated fields; the connector re-validates and resolves everything
  itself. There is no way for the page to name a binary or an argument other than the model and prompt
  values, each of which is one argument element.
- The prompt is user text on a command line: it is visible in the machine's process list. The dialog
  says so. Secrets do not belong in a prompt.
- No permission-bypass flags are exposed for any tool.
- Native `<dialog>`, labelled fields, a polite status region, keyboard-only operation, reduced motion
  respected, no horizontal scroll at 320, 375, 768 and 1440px in light and dark, status shown in text
  as well as colour.

## Testing

- Go: round-trip and negative cases for the new `Create` fields (strictness, bounds, the leading `-`
  rule, model charset, comparability), capability validation, the feature-header negotiation and the
  "update" refusal, store idempotency with and without launch fields, broker handling, API
  validation and error mapping, host folder resolution (symlink escape, non-directory, outside home,
  relative), and the preset builder (exact argument vectors for each tool, with a model, with a
  prompt, with both, with neither).
- A real two-host PTY test beside `TestTerminalPrivateLifecyclePreservesActualShell` using **recording
  fake CLI binaries** on the connector's `PATH` that write their argv and working directory to a file:
  the test asserts the exact argv, the directory, that a retry does not start a second process, and
  that an old-style create still opens a plain shell.
- Web: unit tests for `agentLaunch.ts`, and a Playwright fixture `web/tests/start-agent.e2e.mjs` with
  synthetic API responses: the dialog from both entry points, the three-way disabled states (not
  ready, not installed, old agent), model suggestions and *Other*, the prompt rules, a refused folder,
  a lost reply retried with the same request id and body, the session opening, viewer and legacy
  auth seeing nothing, nothing in browser storage, and the layout and theme matrix. Mutation checks
  cover the leading-`-` rule, the folder boundary, the idempotent retry and "never send launch
  fields to an unsupported machine".

## Delivery

Three reviewable pull requests, each merged when its checks are green, with a broad review before
the last merges and each description stating plainly what is still outstanding.

1. The protocol, store, broker, connector presets, API, capability and audit, with the Go tests, the
   real PTY test and the docs (`docs/terminal-launch.md`, the protocol statement in
   `remote-machine-stage1.md`).
2. The web dialog, both entry points and the fixture.
3. Layout matrix, docs and handoff.

## What this does not establish

That the agent is signed in, has network access, understands the prompt or is making progress: only
that the connector started the program in tmux in that folder. It does not prove a model name exists;
the agent itself will say so. No physical device, screen reader, Safari or Firefox check is planned
beyond the existing Chromium fixtures.

## Continuation

Agent status (working, waiting, idle) by watching the session; resuming an earlier conversation;
**Projects**, a named set of repository folders across machines with a default agent and one action to
open or start them together (reusing the folder list and the launch request); worktrees; wider folder
roots as a deliberate setting; MCP exposure; the reviewed executor for skills.
