# Start an agent session Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** From the Terminals workspace or a ready machine's card, start a browser terminal session that runs a chosen AI agent CLI (optional model and opening prompt) in a chosen folder, and land in it.

**Architecture:** An optional, negotiated extension of the private create-session call: four flat string fields on `terminalproto.Create`, a `launch` capability plus the installed `tools`, and a connector-side presets table that builds the program and arguments passed to `tmux new-session` as separate arguments. The web adds a pure `agentLaunch.ts`, one `StartAgentDialog.svelte` and two entry points.

**Tech Stack:** Go 1.27.1, tmux 3.2+, Svelte 5 runes + TypeScript in `web/` (build before Go tests), Node 22 test runner, Playwright fixtures.

**Spec:** `docs/superpowers/specs/2026-10-25-start-agent-session-design.md`

## Global Constraints

- Never name competing products in repo docs or UI.
- "verified" and "healthy" never describe a machine, a session or an ownership record; a launched session is "started".
- Voice: plain exact wording for states, errors and confirmations; at most one light Yorkshire phrase in a heading, lede or success line.
- `terminalproto.Create` stays a comparable struct (the host compares `*op.Create != in`); new fields are `string` with `omitempty`; no slices or maps.
- The page never supplies a binary name or a shell string; the connector's built-in table decides every program and flag. No permission-bypass flags for any tool.
- Limits: `Model` matches `^[A-Za-z0-9][A-Za-z0-9._:/@+-]{0,99}$`; `Dir` absolute, at most 4,096 bytes, no control characters; `Prompt` at most 2,000 bytes, valid UTF-8, no control characters except newline, must not begin with `-`; `Model` or `Prompt` without `Tool` is refused; `Tool` is one of `claude`, `codex`, `opencode`, `gemini`.
- Feature token `launch-v1` in `X-Reet-Terminal-Features`; capability flags `launch` (requires `terminalControl`) and `tools`. No protocol version bump. A peer without the flag is never sent launch fields.
- Nothing about a session, folder, prompt or model goes to browser storage; the prompt is never written to audit or logs.
- Build the frontend before Go checks; tmux on PATH; attribution trailers on commits.

## Review Focus

- A prompt starting with `-` or containing `--` must reach the agent as one argument, never as a flag (test with the recording fake).
- A `Dir` that is a symlink leaving the home, a file, a missing path or a relative path is refused as `launch_failed` and no tmux session is left behind.
- A retry of the same request id with identical launch fields starts no second process; with a changed field it is the existing 409.
- An old connector (no `launch` flag) asked for a launch gets "Update reet on that machine", and a plain shell create still works on both sides.
- A tool listed in `tools` whose binary vanishes before start fails as `launch_failed`, the session ends `lost` or failed, never "running".
- The dialog refuses a second Start while one is in flight and keeps one request id across Retry.

## File Structure

- Create `internal/terminalproto/launch.go` (+ `launch_test.go`): feature token, `SupportsLaunch`, limits, `Create.validateLaunch`, tool-id set.
- Modify `internal/terminalproto/protocol.go`: `Create` fields, `Capabilities` fields and validation, `LaunchFailed` result code.
- Create `internal/terminalhost/launch.go` (+ `launch_test.go`): presets table, `LaunchCommand(in Create, lookPath) (argv []string, err)`, `InstalledTools(lookPath) []string`, `ResolveDir(home, dir string) (string, error)`.
- Modify `internal/terminalhost/manager.go`: `Create` builds the tmux arguments through the launch helpers.
- Modify `internal/agenthealth/agent.go`: advertise the feature header, set `Launch` and `Tools`.
- Modify `internal/terminalbroker/broker.go`, `internal/api/terminal.go`, `internal/api/private_control.go`: accept fields, refuse unsupported, map errors, audit event.
- Create `web/src/lib/agentLaunch.ts` (+ `agentLaunch.test.mjs`), `web/src/lib/StartAgentDialog.svelte`, `web/tests/start-agent.e2e.mjs`.
- Modify `web/src/lib/terminal/api.ts`, `SessionSidebar.svelte`/`TerminalWorkspace.svelte`, `MachineFleet.svelte`, `.github/workflows/terminal-browser.yml`.
- Create `docs/terminal-launch.md`; modify `docs/remote-machine-stage1.md`, `docs/ui-verification.md`, `handoff.md`, `README.md`.

### Task 1: Protocol fields, limits, feature token

**Files:** `internal/terminalproto/launch.go`, `launch_test.go`, `protocol.go`

**Interfaces:**
- Produces: `Create{Title, Cols, Rows, Tool, Model, Dir, Prompt string}` (last four `json:"…,omitempty"`); `const LaunchFeature = "launch-v1"`; `func SupportsLaunch(features string) bool`; `func ValidTool(id string) bool`; `Capabilities.Launch bool` (`launch,omitempty`) and `Capabilities.Tools []string` (`tools,omitempty`, each a valid tool, at most 8, sorted, no duplicates; **`Capabilities` is not used as a map key or compared, so a slice is allowed there**); `LaunchFailed ResultCode = "launch_failed"` and `ResultCode.Valid` accepts it.
- `Create.Validate` additionally enforces the Global Constraints limits.

- [ ] Step 1: Write failing tests in `launch_test.go`: strict round trip of a launch `Create`; a plain `Create` marshals with none of the four keys; table cases for each limit (model charset and 100-byte bound, `Dir` relative / 4,097 bytes / NUL, prompt 2,001 bytes / invalid UTF-8 / tab / leading `-`, model or prompt without tool, unknown tool); `Create` comparison with `==` compiles and differs when only `Prompt` differs; `Capabilities{Launch:true}` without `TerminalControl` is invalid; `Tools` with a duplicate or unknown id is invalid; unknown JSON fields are still rejected.
- [ ] Step 2: Run `go test ./internal/terminalproto/ -run Launch -v`. Expected: FAIL (fields undefined).
- [ ] Step 3: Implement the fields, `validateLaunch` called from `Create.Validate`, `SupportsLaunch` mirroring `SupportsCommandHistory`, the capability validation, and the result code.
- [ ] Step 4: Run `go test ./internal/terminalproto/ -v`. Expected: PASS.
- [ ] Step 5: Commit "Add launch fields and the launch feature to the terminal protocol".

### Task 2: Connector launch helpers and manager

**Files:** `internal/terminalhost/launch.go`, `launch_test.go`, `manager.go`, `manager_test.go`

**Interfaces:**
- Consumes: Task 1 types.
- Produces: `func LaunchArgv(in terminalproto.Create, lookPath func(string) (string, error)) ([]string, error)` (nil argv and nil error for a plain shell; `ErrLaunchUnavailable` when the tool is unknown or its binary is not found); `func InstalledTools(lookPath func(string) (string, error)) []string`; `func ResolveDir(home, dir string) (string, error)` (symlinks followed, must exist, be a directory and lie within `home`; empty `dir` returns `home`; failures wrap `ErrLaunchUnavailable`). `Manager.Create` appends `argv...` after the existing arguments and uses the resolved directory for `-c`.

- [ ] Step 1: Write failing tests: exact argv for each shipped tool with neither / model / prompt / both (using a table `want [][]string` per tool; the flags come from the tool's help text on a machine that has it or from the recorded fixture in `testdata/`, and **a tool whose syntax is not confirmed is left out of the table and the spec's list is trimmed to match**); a prompt such as `-rf`, `--help` or `a b "c"` is a single element; `ResolveDir` cases (home, subdirectory, symlink inside, symlink escaping, file, missing, relative, outside home, trailing slash); `InstalledTools` with a fake `lookPath`; a manager test with a fake tmux recording its arguments proving the launch args follow the `-e HOME=` pair and a plain create is byte-identical to today's arguments.
- [ ] Step 2: Run `go test ./internal/terminalhost/ -run Launch -v`. Expected: FAIL.
- [ ] Step 3: Implement the presets as an unexported table keyed by tool id (binary name, model flag form, prompt form); build argv with `lookPath` resolved absolute paths; wire `Manager.Create`, mapping `ErrLaunchUnavailable` to `terminalproto.LaunchFailed` at the point the controller reads the result, and make sure a failed resolve happens **before** any registry intent is saved.
- [ ] Step 4: Run `go test -race ./internal/terminalhost/ -v`. Expected: PASS (existing tests unchanged).
- [ ] Step 5: Commit "Start a chosen agent CLI through tmux from allowlisted presets".

### Task 3: Agent, broker and API wiring, audit, real PTY test

**Files:** `internal/agenthealth/agent.go`, `internal/terminalbroker/broker.go`, `internal/api/terminal.go`, `internal/api/private_control.go`, `internal/api/*_test.go`, `internal/terminalbroker/*_test.go`, a new two-host test beside `TestTerminalPrivateLifecyclePreservesActualShell`

**Interfaces:**
- Consumes: Tasks 1–2. Produces: the `X-Reet-Terminal-Features` header carries `launch-v1` alongside `command-history-v1`; `Capabilities.Launch` / `Tools` set from `InstalledTools`; `terminalbroker.ErrLaunchUnsupported`; API create accepts `tool`, `model`, `dir`, `prompt` (strict JSON, unknown fields still 400), answers 409 "Update reet on that machine to start agents." for an unsupporting peer, 422 with a stable `code` (`tool_not_installed`, `folder_unavailable`) for `launch_failed`, and writes an audit event `terminal-launch` with machine, tool, model and directory (never the prompt).

- [ ] Step 1: Write failing tests: capability and header negotiation; broker refuses launch fields to a peer without the flag and still creates plain shells; API validation and each error mapping; audit row contents (prompt absent); idempotent retry with identical fields returns the same session and changed fields give 409.
- [ ] Step 2: Run the focused tests. Expected: FAIL.
- [ ] Step 3: Implement. Follow `terminalCommandHistory` and `ErrCommandHistoryUnsupported` as the pattern; viewers and legacy auth remain blocked by the existing guards.
- [ ] Step 4: Write the real two-host PTY test using **recording fake CLI binaries** on the connector's `PATH` that write argv and working directory to a file; assert the exact argv, the directory, one process after a retry, a plain create still opening a shell, and a refused folder leaving no tmux session. Run `go test -race ./internal/api/ ./internal/terminalbroker/ ./internal/agenthealth/ -count=1`. Expected: PASS.
- [ ] Step 5: Commit "Offer launch requests through the broker and API".

### Task 4: Protocol docs (PR 1 close)

**Files:** `docs/terminal-launch.md` (new; mirror the shape of `docs/terminal-command-history.md`'s "Compatible private extension"), `docs/remote-machine-stage1.md` (protocol statement), `handoff.md` (a Start an agent session section stating exactly what is and is not established, including that the prompt is stored with the session record).

- [ ] Step 1: Write the docs from the spec, stating limits, negotiation, folder policy, the preset table with how each flag was confirmed, and the error codes.
- [ ] Step 2: Run the link check used by CI (`npm run check` in `web/` does not cover docs; run the repository's docs link script if one exists in `scripts/`) and `git diff --check`.
- [ ] Step 3: Full validation in required order (web check/build, `go vet ./...`, `go test -race -count=1 ./...`, mcp tests). Commit "Document launch requests"; open PR 1.

### Task 5: Pure launch logic

**Files:** `web/src/lib/agentLaunch.ts`, `web/src/lib/agentLaunch.test.mjs` (add to the `npm test` script)

**Interfaces:**
- Produces: `TOOLS: {id, name}[]`; `modelSuggestions(tool): string[]`; `validateModel(s): string | null`; `validatePrompt(s): string | null` (byte length via `TextEncoder`, leading `-`, control characters); `folderChoices(machine, snapshot): {path, label}[]` (home first, then reported project folders, deduplicated, absolute only); `launchSentence(tool, dir, machine, hasPrompt): string`; `refusalText(status, code): string` (the table in the spec); `titleFor(tool, dir): string`.

- [ ] Step 1: Write failing tests with the spec's exact sentences, byte-limit edge cases (3-byte characters at the boundary), leading `-`, control characters, folder de-duplication, unknown status mapping to "Something changed. Check again."
- [ ] Step 2: `cd web && node --test src/lib/agentLaunch.test.mjs`. Expected: FAIL.
- [ ] Step 3: Implement. Step 4: Expected PASS and `npm run check` clean. Step 5: Commit.

### Task 6: API client, dialog, entry points

**Files:** `web/src/lib/terminal/api.ts`, `web/src/lib/StartAgentDialog.svelte`, `SessionSidebar.svelte` / `TerminalWorkspace.svelte`, `MachineFleet.svelte`

**Interfaces:**
- Consumes: Task 5. `createTerminal(agentId, requestId, title, cols, rows, launch?: {tool, model?, dir?, prompt?})` sends the four fields only when set; `TerminalMachine.capabilities` gains `launch?: boolean` and `tools?: string[]`. `StartAgentDialog` props: `{ machines, snapshot, initialHost?, onstarted(session), onaddmachine(), onclose() }`.

- [ ] Step 1: Add unit coverage where logic is pure; build the dialog as one immutable operation per attempt (request id plus exact parameters), a busy guard, the existing 15-second abort, the three disabled states, focus return to the opener, the prompt cleared on close and never persisted.
- [ ] Step 2: Add *New agent session* beside *New terminal* in the sessions sidebar and *Start an agent* on ready machine cards, hidden for viewers and legacy auth; landing reuses the workspace session switch and the `/terminals?agent=…&session=…` navigation.
- [ ] Step 3: `cd web && npm run check && npm run build && npm test`. Expected: 0 errors, 0 warnings, all pass. Commit.

### Task 7: Browser fixture (PR 2 close)

**Files:** `web/tests/start-agent.e2e.mjs`, `.github/workflows/terminal-browser.yml`

- [ ] Step 1: Write the fixture against synthetic API responses covering: both entry points, the not-ready / not-installed / old-agent states, model suggestions and *Other*, prompt rules, refused folder, lost reply retried with the same request id and body, the session opening, viewer and legacy auth seeing nothing, nothing in browser storage, no second Start while busy.
- [ ] Step 2: Run it four times plus the existing five fixtures with `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/opt/pw-browsers/chromium` against the preview. Expected: all pass. Mutation checks: break the leading-`-` rule, the busy guard, the same-request-id retry and "send launch fields only to launch-capable machines"; each must fail the fixture.
- [ ] Step 3: Add the fixture to the workflow; commit; open PR 2.

### Task 8: Matrix, docs, handoff, broad review (PR 3)

- [ ] Layout and theme matrix (320, 375, 768, 1440; light and dark; reduced motion) for every dialog state in the fixture's style; fix real defects only; record results in `docs/ui-verification.md` with honest limits (no screen reader, no other browsers; any axe incomplete counts stated).
- [ ] `docs/start-agent-session.md` for people; README sentence; `handoff.md` record with PR numbers; broad review on the most capable model before the last merge; full validation; PR 3.
