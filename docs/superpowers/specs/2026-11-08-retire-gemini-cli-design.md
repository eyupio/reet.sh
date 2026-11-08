# Retire Gemini CLI (design)

Date: 2026-11-08. Status: design approved in conversation by the product owner; this spec awaits their review
before an implementation plan is written.

Gemini CLI is deprecated. reet still lists it as an agent to start, keeps a model and effort preset for it,
validates it as a default agent, and manages its settings, MCP servers and instructions under `~/.gemini`. This
design removes it everywhere, and handles what older machines and stored data still say about it without
breaking either.

## Intent and success

**Outcome.** Gemini CLI is no longer offered, started, validated or managed. Nothing breaks for a machine that
still reports it, and nothing is lost from history.

**Success looks like.**

- Start an agent, the launch bar, Add project and Settings › Agents never list Gemini CLI.
- A create that names `gemini` is refused like any unknown agent.
- A machine running an older connector that still reports `gemini` in its tools stays connected and ready; the
  tool is simply never offered.
- After upgrade, no project, account default, backlog draft or favourite names Gemini, and nothing fails
  validation because of it.
- Past sessions, run records and Setup history are untouched and still read as "Gemini CLI".
- reet no longer reads or writes `~/.gemini`. Nothing on a machine's disk is changed or deleted.

**Not in this pass.** Anything about Google Antigravity (AGY): its `GEMINI_API_KEY` login variable and Google's
Gemini model names in its tests are not Gemini CLI. The dated notes under `docs/research/`, which are evidence of
what was measured, are left as they are.

## The retired-agents rule

An older connector may report `gemini` in its capability `Tools` list, and `Capabilities.ValidateFor` rejects the
whole record for any tool it does not recognise, which would make that machine look broken. So the controller
needs one small notion of a retired agent, rather than forgetting the id:

- `terminalproto.RetiredTool(id string) bool` is true for `gemini` only.
- A capability `Tools` list may contain a retired tool (the ordering rule still applies). It is never offered,
  never launched and never required.
- An `acpAgents` descriptor for a retired tool validates too, but only as missing, unverified or incompatible,
  never as an available chat agent. An older connector described Gemini CLI there as well as in `Tools`.
- The same function marks a stored default as stale during the one-time cleanup and the legacy-project
  migration below.
- The web keeps a retired-agent label table (`gemini` → "Gemini CLI") used only to name past sessions. The
  agent catalogue and every picker leave it out.

## What is removed

- **Launch and presets.** The v1 launch tool set and the agent preset (`internal/terminalproto/launch.go`,
  `agents.go`), the auto-title name (`autotitle.go`), the connector's catalogue entry
  (`internal/terminalhost/catalogue.go`) and its help fixture, the descriptor adapter map
  (`internal/acpagent/describe.go`), and the terminal-control tool loop (`internal/scan/agent_terminal_control.go`).
- **Validation text.** The "default agent must be one of…" messages in the store stop naming Gemini.
- **Web.** The agent list and model hints (`agentLaunch.ts`, `catalogue.ts`), the "original four" set for
  connectors without `agents-v2`, which becomes three, and the title filler word.
- **Setup.** The Gemini adapter and its install detection, version probe, policy, component, summary and support
  entries (`internal/setup`). Setup restores work from recorded paths, so existing history is unaffected.
- **Docs.** Every user-facing mention (README, the quick start, terminal launch, start-agent, models, setup
  discovery, structured sessions, the comparison pages and the index). Dated research notes stay.

## Stored data (one-time migration)

Run once at upgrade, idempotent, in one transaction:

- Account profile and folder projects: where the default agent is `gemini`, blank the default agent, model and
  effort.
- Legacy projects: the legacy-to-folder-project migration blanks a retired default agent instead of copying it,
  so a not-yet-migrated member cannot reintroduce it.
- Backlog drafts: blank the agent, model and effort of an unstarted draft that names Gemini, and keep its text.
- Model favourites for `gemini`, and the cached machine model lists for it, are deleted. Both are derived or
  re-creatable.
- Untouched: terminal sessions, selection-run records, audit events and Setup history.

## Compatibility and failure

- A create or run that names `gemini` is refused as an unsupported agent, with the existing message.
- An older browser against a newer controller has nothing to break: it only ever sent a tool the machine
  reported, and the controller refuses `gemini` explicitly.
- A newer connector never reports `gemini`. An older controller's capability check still accepts a list without
  it.
- The migration touches only rows that name `gemini`, so a second run changes nothing.

## Testing

- Protocol: a `Tools` list containing `gemini` still validates but offers nothing; `Create` with `gemini` is
  invalid; `RetiredTool` is true for `gemini` only.
- Store: the migration clears profile, folder-project and draft defaults that name Gemini, deletes its
  favourites and cached lists, leaves other agents' rows and past sessions alone, and a second run is a no-op;
  the legacy-project migration blanks a Gemini default.
- API: a capability record from an older connector that reports `gemini` leaves the machine ready.
- Web: the catalogue and every picker omit Gemini, a past `gemini` session is labelled "Gemini CLI", and the
  "original four" for an older connector is three.
- Existing Go and web tests, fixtures and e2e files that named Gemini CLI are updated or removed.

## Delivery

A single pull request from a fresh branch off the current main.
The AGY reconnect fix found in the Gaffer audit is a separate pull request.
