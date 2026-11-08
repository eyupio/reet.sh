# Claude Code, Fully Supported Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring Claude Code to the bar OpenCode, Google Antigravity (AGY) and the other fully supported agent meet: a Gaffer chat with models, effort, stored history and in-place reconnect, and a terminal launch with resume, open-session, YOLO and effort.

**Architecture:** Five slices, each its own draft pull request. Slice 1 only measures (a probe and opt-in tests). Slice 1b replaces the environment-variable switch with a stored per-account setting that the controller applies to what a machine reports. Slice 2 turns on in the descriptor and profile only what slice 1 proved. Slice 3 adds terminal parity to the connector's preset and the controller's option table. Slice 4 adds the missing-adapter hint, final docs and the switch flip, and is held until the owner records the signed-in run.

**Tech Stack:** Go (connector `internal/acpagent`, `internal/terminalhost`, `internal/terminalproto`, `internal/terminalagent`, `internal/terminalbroker`), SQLite (`internal/store`), Svelte 5 + TypeScript (`web/src/lib`), Python probe scripts, MkDocs docs.

**Spec:** `docs/superpowers/specs/2026-11-08-claude-code-native-support-design.md`

## Global Constraints

- The adapter is `@agentclientprotocol/claude-agent-acp`, pinned **0.99.1** (`ClaudeCandidate.MinVersion`). The `claude` CLI does not speak ACP.
- `Create.Effort` stays `low|medium|high` (`terminalproto.ValidEffort`). No new wire feature is added for effort. The Start dialog offers only those three; the live chat control offers every value the session reports.
- `fast` and `ultracode` stay listed in the descriptor and uncontrolled. The live chat has no control for the `model_config` category for any agent; building one is a separate change.
- `ClaudeSessionMeta` (`settingSources: ["user"]`) stays on both new and load.
- The profile keeps requiring the adapter's `default` mode. Claude's other permission modes are out of scope.
- The terminal one-word prompt rule for `claude` stays.
- YOLO needs the account setting, the machine switch and the confirmation, unchanged.
- Nothing is turned on that a recorded probe did not show.
- Settings live in the database. No environment variable carries a setting (the owner's ruling), so `REET_EXPERIMENTAL_CLAUDE_CHAT` and its Codex twin are replaced by a stored per-account setting that defaults to off until the owner records the signed-in run. Only the held Task 16 changes Claude's default to on. Opt-in real-adapter tests may read `REET_CLAUDE_ACP`; that is a test input, not a setting.
- Commit messages and pull request text name only Claude Code, OpenCode and Google Antigravity; they say "the other agents" otherwise. Commits end with the two attribution lines from the session reminder.
- Each slice is a draft pull request off the latest `main`, subscribed, and green before the next slice starts. Slice 1b and slice 2 start only after the AGY reconnect fix (#666) has merged, and slice 2 only after slice 1b.

## Review Focus

- An account with Claude chats off: the web receives Claude's descriptor as `unverified` whatever the machine reports, with no new flags and no hint, and a create naming Claude is refused (Tasks 5, 8).
- An effort the adapter does not confirm: the chat refuses and no opening prompt is sent (Task 10).
- Stored history opened for another folder, machine or account: still refused by the existing binding, now reachable for Claude (Task 9).
- Chat YOLO becomes available for Claude as a side effect of `yolo: true` (`terminalbroker/chat.go` checks `ToolTakesYolo`); it must hit the same three gates as every other agent (Task 13).
- An older controller or connector: no new descriptor fields reach a peer that did not negotiate them; Claude's `Reattach` is stripped without `acp-agent-reattach-v1` (Task 8), and Claude and Codex stay `unverified` when the controller does not advertise the chat gate (Task 4).

---

## Slice 1: Probe (pull request 1, no product change)

### Task 1: Probe scenarios for images and options

**Files:**
- Modify: `scripts/probe-claude-lifecycle.py` (the `scenario` choices at line 15, the `Stub` request log, new scenario bodies)

**Interfaces:**
- Produces: scenarios `images` and `options`, each printing one JSON object to stdout (keys below), consumed by Task 3's results table.

- [ ] **Step 1: Install the pinned adapter in a scratch directory.**

```bash
probe_dir=$(mktemp -d); npm install --ignore-scripts --no-audit --no-fund --prefix "$probe_dir/claude" @agentclientprotocol/claude-agent-acp@0.99.1
```
Expected: exits 0 and `$probe_dir/claude/node_modules/.bin/claude-agent-acp` exists.

- [ ] **Step 2: Add `images` and `options` to the scenario choices and record every request body the stub receives** (a list on the `Stub` class, reset per scenario).

- [ ] **Step 3: Implement scenario `images`.** Start a session, send one prompt whose content has a text block and an `image` block (a 1x1 PNG, base64, `mimeType: "image/png"`). Print `{"imageBlockSeenByStub": bool, "promptStopReason": str, "initializeImageCapability": bool}` where the capability is `agentCapabilities.promptCapabilities.image` from `initialize`.

- [ ] **Step 4: Implement scenario `options`.** After `session/new`, print `{"newOptionIds": [...], "effortValues": [...], "effortInNew": bool}`. Then call `session/set_config_option` for `effort` with a listed value, a listed value on `fast`, and an unknown effort value; print `{"effortReadBack": str, "fastReadBack": str, "unknownEffortCurrent": str, "unknownEffortError": str|null}` from the replies' `currentValue`/error. Then set the model alias `sonnet` and print `{"optionIdsAfterModel": [...]}`.

- [ ] **Step 5: Run both scenarios and keep the JSON.**

```bash
python3 scripts/probe-claude-lifecycle.py images --workspace "$probe_dir" > "$probe_dir/images.json"; python3 scripts/probe-claude-lifecycle.py options --workspace "$probe_dir" > "$probe_dir/options.json"; cat "$probe_dir"/images.json "$probe_dir"/options.json
```
Expected: two valid JSON objects with every key above present.

- [ ] **Step 6: Commit** `Probe Claude Code image prompts and option switches`.

### Task 2: Opt-in real-adapter tests for options and the unstored load

**Files:**
- Modify: `internal/acpagent/real_local_test.go` (beside `TestRealClaudeStart`)

**Interfaces:**
- Consumes: `ClaudeCandidate`, `Start`, `Options`, `Session.Controls()`, `Session.SetOption`.
- Produces: `TestRealClaudeOptions` and `TestRealClaudeLoadUnstored`, both skipped unless `REET_CLAUDE_ACP` is set, neither sends a prompt.

- [ ] **Step 1: Write `TestRealClaudeOptions`.** Start through `ClaudeCandidate`, find the `thought_level` setting in `session.Controls().Options`, assert it lists at least `low`, `medium`, `high`, call `SetOption` with `high` and assert `session.Controls()` then reads `high`, and assert an unknown value does not read back as itself. If `effort` is absent from `session/new`, the test logs that and asserts the absence instead of failing, so it records the real behaviour.

- [ ] **Step 2: Write `TestRealClaudeLoadUnstored`.** Start with `opts.Resume` set to a fresh UUID the adapter has never stored; assert the error satisfies `errors.Is(err, ErrNoStoredConversation)`.

- [ ] **Step 3: Run both against the pinned adapter.**

Run: `REET_CLAUDE_ACP="$probe_dir/claude/node_modules/.bin/claude-agent-acp" go test ./internal/acpagent -run 'TestRealClaude(Start|Options|LoadUnstored)' -v -count=1`
Expected: PASS. If `TestRealClaudeLoadUnstored` fails because the adapter's error is not mapped to `ErrNoStoredConversation`, note the actual error code in the ledger for Task 9, which fixes the mapping.

- [ ] **Step 4: Commit** `Add opt-in real-adapter checks for Claude Code options and unstored history`.

### Task 3: Record the results and open pull request 1

**Files:**
- Modify: `docs/research/structured-adapters/probe-report.md` (new section at the end), `docs/device-checks.md` (one line under Recorded results)

- [ ] **Step 1: Run the existing `load` scenario** and keep its JSON: `python3 scripts/probe-claude-lifecycle.py load --workspace "$probe_dir"`.

- [ ] **Step 2: Add a section "Claude parity probes (8 November 2026)"** with one results table, one row per fact, each with the measured value and the scenario or test that produced it: history replay delivered, unstored-load error code, `image` prompt capability, image block seen by the stub, `effort` present in `session/new`, effort read-back, unknown-effort behaviour, `fast`/`ultracode` read-back, option list after a model change. State plainly that the stub shows bytes arriving, not that a real model accepts images.

- [ ] **Step 3: Verify the docs build.**

Run: `DOCS_SITE_URL=https://reet.sh/ mkdocs build --strict -q && python3 scripts/check-docs.py`
Expected: "Docs checks passed".

- [ ] **Step 4: Commit** `Record the Claude Code parity probe results`, push, open **draft pull request 1** (probe only), subscribe to it, and wait for green. The table is the gate for Tasks 9 to 11.

---

## Slice 1b: The chat switch moves to the database (pull request 1b)

The owner ruled that settings live in the database, not in environment variables. `REET_EXPERIMENTAL_CLAUDE_CHAT` and `REET_EXPERIMENTAL_CODEX_CHAT` are replaced by a stored per-account setting. The connector stops deciding: it reports Claude and Codex as it finds them, but only to a controller that advertises the new `chat-gate-v1` feature, and the controller then hides them from each account that has not turned them on. Start from the latest `main` after #666 has merged (that PR adds a test that sets the old switch).

### Task 4: The policy, the feature and the connector's reporting

**Files:**
- Modify: `internal/terminalproto/acp_agents.go`, `internal/terminalproto/acp_agents_test.go`, `internal/api/private_control.go`, `internal/agenthealth/agent.go`, `internal/agenthealth/acp_agents_test.go`

**Interfaces:**
- Produces: `const ChatGateFeature = "chat-gate-v1"`; `func SupportsChatGate(features string) bool`; `func GatedChatTools() []string` (sorted: `claude`, `codex`); `func ChatDefaultOn(tool string) bool` (false for both); `func (c Capabilities) WithoutChats(tools []string) Capabilities`, a copy in which every descriptor for a listed tool, whatever its state, becomes `ACPAgent{Tool: ..., Adapter: ..., State: "unverified"}`, every other field is untouched, the original is not modified, and the result passes `Validate()`.

- [ ] **Step 1: Write the failing protocol tests** in `acp_agents_test.go`: `TestGatedChatToolsAreClaudeAndCodexAndOffByDefault`, `TestWithoutChatsMasksOnlyTheListedTools` (an available Claude and an available OpenCode; masking `claude` leaves OpenCode unchanged, turns Claude `unverified` with no capability flags, leaves the input untouched, and the result validates), `TestWithoutChatsMasksAMissingDescriptorToo`, `TestSupportsChatGate`.

- [ ] **Step 2: Write the failing connector and controller tests.** In `agenthealth/acp_agents_test.go`: `TestGatedChatsAreReportedAvailableOnlyWhenTheGateIsNegotiated` (the fake structured-agent source returns Claude and Codex as available and OpenCode as available; without `chat-gate-v1` the report has Claude and Codex `unverified` and OpenCode `available`; with it, all three `available`). In `internal/api`, beside the existing private-control feature tests: `TestControllerAdvertisesTheChatGate` (`privateControlFeatures` contains the token).

- [ ] **Step 3: Run them to see them fail.**

Run: `go test ./internal/terminalproto ./internal/agenthealth ./internal/api -run 'Gated|WithoutChats|ChatGate' -count=1`
Expected: FAIL (undefined names).

- [ ] **Step 4: Implement** the exact signatures above. `agreedControlFeatures` (`private_control.go`) adds the token. In `agenthealth/agent.go` negotiation sets a `chatGate` flag from the token (beside `acpAgentReattach`), and where `capabilitiesFor` builds the described entries it replaces any entry for a `GatedChatTools()` tool with the `unverified` shape when the flag is not set.

- [ ] **Step 5: Run the three packages.**

Run: `go test ./internal/terminalproto ./internal/agenthealth ./internal/api -count=1`
Expected: PASS.

- [ ] **Step 6: Commit** `Add the chat gate: a feature and a policy for agents that are not on by default`.

### Task 5: The stored setting and what the controller shows

**Files:**
- Modify: `internal/store/store_backups.go` (the `Profile` struct, the column list near line 205, the read and write SQL), `internal/store/store_terminal.go` (`RemoteCapabilitiesFor`), `internal/store/store_gaffer_setup.go` (the capabilities join near line 30: apply the same mask or confirm it never uses descriptors), `internal/api/profile.go`, `internal/api/software.go` (`recordSoftwareEntry`); tests in `internal/store/profile_defaults_test.go`, `internal/store/store_terminal_test.go`, `internal/api/agents_home_defaults_test.go`, and the broker's create tests

**Interfaces:**
- Consumes: `terminalproto.GatedChatTools`, `ChatDefaultOn`, `Capabilities.WithoutChats` (Task 4).
- Produces: `Profile.ChatAgents map[string]bool` (`json:"chatAgents"`), the account's explicit choices, stored in column `chat_agents TEXT NOT NULL DEFAULT '{}'`; `func (p Profile) ChatEnabled(tool string) bool` (the explicit choice, else `ChatDefaultOn`); `GET /api/v2/profile` returns `profile.chatAgents` as the effective value for every gated tool; `PATCH /api/v2/profile` accepts a partial `chatAgents` object, refuses an unknown tool or a non-boolean with the status and code style of the other profile fields, is writer-only, and audits tool and value only; `RemoteCapabilitiesFor` returns the capabilities with `WithoutChats` applied for every gated tool the machine owner has off; `func (s *Store) RemoteCapabilitiesRawFor(uid int64, agentID string) (RemoteAgentCapabilities, error)` returns the stored record unmasked and is used only by code that reads a record in order to write it back (`recordSoftwareEntry` in `internal/api/software.go`, which merges a software entry and stores the whole record through `UpdateRemoteCapabilitiesFor`), so the mask is never persisted over what the connector reported.

- [ ] **Step 1: Write the failing store tests.** `TestChatAgentsRoundTripAndDefaultToOff` (no explicit choice: `ChatEnabled("claude")` false; after saving `{claude: true}` it reads back true and Codex stays false), `TestChatAgentsColumnMigratesAnOldDatabase` (a database whose `user_profiles` lacks the column opens, gains it, and a second open changes nothing), `TestChatAgentsAreInTheProfileBackup` (export and import keep the choices), `TestRemoteCapabilitiesAreMaskedByTheAccountSetting` (a stored record with available Claude, Codex and OpenCode descriptors: with the setting off Claude and Codex read back `unverified` and OpenCode `available`; with `{claude: true}` Claude reads back `available`; a different account's setting does not affect the first; `RemoteCapabilitiesRawFor` returns the descriptors unmasked whatever the setting).

- [ ] **Step 2: Write the failing API and broker tests.** In the api software tests: `TestSoftwareRefreshWhileChatsAreOffDoesNotPersistTheMask` (store a record with an available Claude descriptor, keep the account's Claude chat off, record a software entry through `recordSoftwareEntry`, then turn Claude chat on: Claude reads back `available` with no new connector report). In `agents_home_defaults_test.go`: `TestProfileChatAgentsValidationPermissionAndAudit` (an unknown tool is refused, a string value is refused, a viewer gets 403, a writer's change appears in `GET`, and the audit row names tool and value only). In the broker create tests: `TestStructuredClaudeCreateFollowsTheAccountSetting` (a machine whose stored descriptors list Claude as available: a structured create naming Claude is refused with `ErrACPUnsupported` while the account has it off and accepted when it is on).

- [ ] **Step 3: Run them to see them fail.**

Run: `go test ./internal/store ./internal/api ./internal/terminalbroker -run 'ChatAgents|AccountSetting' -count=1`
Expected: FAIL.

- [ ] **Step 4: Implement** the interfaces above following the shape of the existing saved-default columns (`grep -n 'default_effort' internal/store/store_backups.go internal/api/profile.go`): column, field, validation and audit. `RemoteCapabilitiesFor` reads the owner's `chat_agents` (no row means all defaults) and applies the mask; `UpdateRemoteCapabilitiesFor`, which compares and stores the connector's own report, is not masked, and `recordSoftwareEntry` switches to `RemoteCapabilitiesRawFor`. Check with `grep -rn 'RemoteCapabilitiesFor' internal --include=*.go` that no other caller reads a record to write it back.

- [ ] **Step 5: Run the three packages.**

Run: `go test ./internal/store ./internal/api ./internal/terminalbroker -count=1`
Expected: PASS.

- [ ] **Step 6: Commit** `Store which gated chats an account has turned on and apply it to what machines report`.

### Task 6: The control in Settings

**Files:**
- Modify: the Agents section of Settings (`grep -rn "defaultEffort" web/src/lib`), the profile type beside it, `web/package.json` if a new unit test file is added, `web/tests/account.e2e.mjs`

**Interfaces:**
- Consumes: `profile.chatAgents: Record<string, boolean>` from `GET /api/v2/profile`; `PATCH /api/v2/profile` with `{chatAgents: {<tool>: boolean}}`.
- Produces: a "Gaffer chats still being verified" group in Settings > Agents with one checkbox per gated tool ("Claude Code in Gaffer", "Codex in Gaffer"), disabled with a reason for a viewer, with the help line "Not yet checked with a signed-in account. Turn on to try it on your machines that have its chat adapter installed."

- [ ] **Step 1: Write the failing tests.** A unit test for the pure part (labels, effective values, the PATCH body for one toggle) and, in `account.e2e.mjs`, a case that shows both checkboxes unchecked by default, checks Claude and sees the PATCH `{chatAgents: {claude: true}}`, and shows both disabled for a viewer.

- [ ] **Step 2: Run to see them fail.**

Run: `cd web && npm run check` and `node tests/account.e2e.mjs` against `npm run preview`
Expected: FAIL (no control).

- [ ] **Step 3: Implement** the group and the type, in the same style as the saved default model controls.

- [ ] **Step 4: Run the checks.**

Run: `cd web && npm run check`, then `node tests/account.e2e.mjs` and `node tests/phone-dialogs.e2e.mjs`
Expected: pass.

- [ ] **Step 5: Commit** `Add the Gaffer chat switches to Settings`.

### Task 7: Retire the environment switches and open pull request 1b

**Files:**
- Modify: `internal/acpagent/spec.go` (`Lookup`, `ClaudeSwitch`, `ClaudeEnabled`, `CodexSwitch`, `CodexEnabled`), `internal/acpagent/codex.go` (its comment), every test that sets either switch (`grep -rn 'ClaudeSwitch\|CodexSwitch' internal`), `docs/structured-sessions.md` (the "Experimental" section), `docs/device-checks.md`, `docs/roadmap.md`

- [ ] **Step 1: Write the failing test** `TestClaudeAndCodexAreRegisteredWithoutAnEnvironmentSwitch` in `spec_test.go`: with neither variable set, `Lookup("claude")` and `Lookup("codex")` return their candidates, and `Describe` reports an installed adapter as `available`.

- [ ] **Step 2: Run it to see it fail.**

Run: `go test ./internal/acpagent -run TestClaudeAndCodexAreRegisteredWithoutAnEnvironmentSwitch -count=1`
Expected: FAIL.

- [ ] **Step 3: Implement.** `Lookup` returns both candidates unconditionally; delete the two constants and the two `Enabled` functions; remove the `t.Setenv` calls from the tests that set them (their intent, "the connector reports it", is now true by default; the case "not reported" is covered by Task 4's gate test). Confirm `grep -rn 'REET_EXPERIMENTAL' --include=*.go --include=*.ts --include=*.svelte .` finds nothing.

- [ ] **Step 4: Docs.** Replace the "Experimental: Claude Code and Codex chats" section of `docs/structured-sessions.md` with what is now true: the chats are off for every account until turned on in Settings > Agents, the setting is stored with the account, a machine needs the adapter installed and a controller that advertises the gate, and the sign-in refusal text is unchanged. Update the gate wording in `device-checks.md` and `roadmap.md`. The dated notes under `docs/research` and `docs/structured-chat-plan.md` are history and stay.

- [ ] **Step 5: Full verification.**

Run: `go vet ./... && go test ./... -count=1`, `cd web && npm run check`, the docs build and `python3 scripts/check-docs.py`
Expected: all pass.

- [ ] **Step 6: Commit** `Replace the Claude Code and Codex environment switches with an account setting`, push, open **draft pull request 1b**, subscribe, and wait for green.

---

## Slice 2: Gaffer parity behind the account setting (pull request 2)

Start from the latest `main` after pull requests 1 and 1b and #666 have merged. Read Task 3's table first; each conditional step below says which row it depends on.

### Task 8: Descriptor and profile report only what was proven

**Files:**
- Modify: `internal/acpagent/describe.go` (the `if tool == "claude"` arm), `internal/acpagent/claude.go` (`ClaudeCandidate`), `internal/acpagent/describe_test.go`

**Interfaces:**
- Produces: for an available Claude descriptor, `Model`, `Approval`, `History` and `Reattach` true; `Images` true only if Task 3 shows the adapter takes image blocks; `Questions` false; `Modes` empty. `ClaudeCandidate.Reattach = true`.

- [ ] **Step 1: Write the failing tests.** In `describe_test.go`: `TestDescribeClaudeReportsOnlyProvenCapabilities` asserts the exact flags above; update the `claude` row of `TestDescribeClaimsReattachOnlyWhereTheProfileCanReattach` to expect `true` and `Reattach: true` in the profile, and update the existing Claude descriptor assertions in `describe_test.go` that expect only `Model` and `Approval`.

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./internal/acpagent -run 'TestDescribeClaude|TestDescribeClaimsReattach' -count=1`
Expected: FAIL (Claude reports no history or reattach).

- [ ] **Step 3: Implement.** In `Describe`, the Claude arm sets `entry.History = true` and `entry.Reattach = spec.Reattach` besides `Model` and `Approval`, and `entry.Images = claudeTakesImages`, a named constant that is `true`: slice 1 measured that the adapter advertises image prompts and forwards the block (the real model's acceptance is unmeasured, and the docs say so). Set `Reattach: true` on `ClaudeCandidate`.

- [ ] **Step 4: Add the peer-negotiation test.** In `internal/agenthealth/acp_agents_test.go`, beside `TestAgentReattachClaimEnablesAnotherAgentsReattach`, add `TestClaudeReattachIsStrippedWithoutTheAgentReattachFeature`: with the feature not negotiated, Claude's descriptor has `Reattach` false and `History` follows `acpHistory`; with it negotiated, `Reattach` is true.

- [ ] **Step 5: Run the touched packages.**

Run: `go test ./internal/acpagent ./internal/agenthealth ./internal/terminalproto -count=1`
Expected: PASS.

- [ ] **Step 6: Commit** `Report the Claude Code chat capabilities the probe proved`.

### Task 9: History continue and in-place reattach for Claude

**Files:**
- Modify: `internal/acpagent/claude_test.go`, `internal/acpagent/session.go` (only if the unstored-load error is not already mapped), `internal/terminalagent/chat_reattach_test.go` or the existing reattach test file, `internal/terminalbroker` continue-with-history test file

**Interfaces:**
- Consumes: `Start(ctx, Options{Resume: ...})`, `ErrNoStoredConversation`, `Describe(...).History`, `acpagent.Lookup("claude")`.
- Produces: `claudeOptions(t *testing.T, scenario string) (Options, *recorder)` helper in `claude_test.go`, mirroring the existing per-agent options helper (`grep -n 'func .*Options(t' internal/acpagent/*_test.go`).

- [ ] **Step 1: Write the failing tests.** `TestClaudeReattachesInPlace` and `TestClaudeReattachWithoutAStoredConversationIsRefused` in `claude_test.go`, mirroring the existing in-place reattach and unstored-conversation tests for the other reattach-capable agent (`grep -n 'ReattachesInPlace\|WithoutAStoredConversation' internal/acpagent/*_test.go`), with the fake agent speaking the frames Task 3 recorded (use the unstored-load error code from the table). In the terminalagent reattach test file, `TestChatReattachAcceptsClaudeWhenTheProfileAllowsIt` (with the switch on, `h.reattach` does not return `acpagent.ErrUnsupported`). In the broker's continue-with-history tests, `TestContinueClaudeWithHistoryIsBoundToItsFolder`: a closed Claude session continues in its own folder and is refused for another folder, machine or account.

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./internal/acpagent ./internal/terminalagent ./internal/terminalbroker -run 'Claude' -count=1`
Expected: FAIL only where the behaviour is missing (reattach before Task 8's profile flag, the unmapped error code).

- [ ] **Step 3: Implement** only what the failures show. Slice 1 measured that the adapter's unstored-load error (`-32002` "Resource not found") already maps to `ErrNoStoredConversation` (`noStoredConversation` in `session.go`), so no mapping change is expected; add one only if a test here shows a gap.

- [ ] **Step 3a: Opt-in real replay test (deferred from slice 1).** Add `TestRealClaudeHistoryReplay` beside the other `TestRealClaude*` tests: skipped unless `REET_CLAUDE_ACP` and `REET_CLAUDE_STORED_HOME`/`REET_CLAUDE_STORED_SESSION` are set, where those name the throwaway home and session id a `scripts/probe-claude-lifecycle.py load` run left behind; it calls `Start` with `Home`, `Dir` and `Resume` set and asserts the session opens and the replay is dropped (the controller keeps its own transcript). Record the result in `docs/research/structured-adapters/probe-report.md` or, if it cannot be run here, state that in the ledger and leave it for the owner's device checks.

- [ ] **Step 4: Run the three packages.**

Run: `go test ./internal/acpagent ./internal/terminalagent ./internal/terminalbroker -count=1`
Expected: PASS.

- [ ] **Step 5: Commit** `Reconnect and continue Claude Code chats from their stored history`.

### Task 10: Effort and a chosen model at start, confirmed

**Files:**
- Modify: `internal/acpagent/claude.go` (`ClaudeCandidate`), `internal/acpagent/claude_test.go`, `internal/acpagent/real_local_test.go`, `web/src/lib/terminal/launchOptions.ts`, `web/src/lib/models.test.mjs`

**Interfaces:**
- Consumes: `Options.Effort`, `Options.Model` and the existing `thought_level` and model handling in `Start` (`session.go`); `claudeOptions` from Task 9.
- Produces: `ClaudeCandidate.ConfirmModelBySwitch = true`; `launchEfforts(tool, caps, chat)` returns only values in `{low, medium, high}` for a chat (plus the leading empty string).

Slice 1 measured, in the connector's environment: the adapter offers `effort` at `session/new` for its default model and reads a set back; it refuses an unlisted value; a launch naming only an effort works; a launch naming a model other than the adapter's own is refused today (`ErrModel`) and works once the profile confirms the model switch by read-back. Which options exist depends on the model: after choosing `sonnet` the adapter offers `effort` and `ultracode` but not `fast`.

- [ ] **Step 1: Write the failing Go tests** in `claude_test.go`, using the frames slice 1 recorded: `TestClaudeStartsWithAConfirmedEffort` (the fake reports `effort` at `session/new`, accepts `high`, reads it back; the `thought_level` setting reads `high` before `OnReady`), `TestClaudeRefusesAnEffortTheAdapterDoesNotConfirm` (the fake echoes the old value; `Start` fails, no prompt is sent, the process group is retired), `TestClaudeStartsOnAChosenModelAndEffort` (the fake lists models; with the profile as it stands `Start` returns `ErrModel`, so this fails until Step 3), and `TestClaudeRefusesAnEffortTheChosenModelDoesNotOffer` (the fake drops `effort` after the model switch; `errors.Is(err, ErrControl)`).

- [ ] **Step 2: Write the failing web test** in `web/src/lib/models.test.mjs`: with a Claude descriptor whose `thought_level` values are `default, low, medium, high, xhigh, max`, `launchEfforts('claude', caps, true)` equals `['', 'low', 'medium', 'high']`, and a Codex-shaped list with `ultra` drops it likewise.

- [ ] **Step 3: Run both to see them fail.**

Run: `go test ./internal/acpagent -run 'TestClaude.*(Effort|Model)' -count=1` and `cd web && node src/lib/models.test.mjs`
Expected: FAIL (`ErrModel` for a chosen model; the web list still includes `xhigh`, `max`).

- [ ] **Step 4: Implement.** Set `ConfirmModelBySwitch: true` on `ClaudeCandidate`: the profile then confirms a chosen model by switching and reading it back, and the adapter's silent reset of an unknown model to `default` is caught by that read-back. `launchEfforts` keeps only `low`, `medium` and `high` from the descriptor's values for a chat, since `Create.Effort` cannot carry the rest; add one line to its comment saying so. Flip the pin in `TestRealClaudeOptions` (`real_local_test.go`): the launch naming a model and an effort now succeeds on the profile as it stands, so replace the `ErrModel` assertion with the success assertions the `switched` case has, and delete the `switched` copy.

- [ ] **Step 5: Run the checks.**

Run: `go test ./internal/acpagent -count=1`, `REET_CLAUDE_ACP=<adapter> go test ./internal/acpagent -run TestRealClaude -count=1`, then `cd web && npm run check`
Expected: PASS, 0 errors.

- [ ] **Step 6: Commit** `Start Claude Code chats on a chosen model and thinking level, confirmed`.

### Task 11: Images and the docs for this slice

**Files:**
- Modify: `internal/acpagent/claude_test.go`, `docs/structured-sessions.md`

- [ ] **Step 1: Images.** Slice 1 measured that the adapter advertises image prompts and the block reaches the provider request. Write `TestClaudePromptCarriesImages` (a fake agent that records the prompt; an image attachment arrives as an `image` block) and make it pass. The docs state that a real model's acceptance of an image is unmeasured.

- [ ] **Step 2: Docs.** In `docs/structured-sessions.md` replace the sentence that Claude reports "approvals and the model switch only" with what the descriptor now reports, and state which rows of the probe table back each capability and that a signed-in turn has not been run.

- [ ] **Step 3: Full verification.**

Run: `go vet ./... && go test ./... -count=1` then `cd web && npm run check` then `DOCS_SITE_URL=https://reet.sh/ mkdocs build --strict -q && python3 scripts/check-docs.py`
Expected: all pass.

- [ ] **Step 4: Commit** `Document Claude Code history, reconnect and thinking levels in Gaffer`, push, open **draft pull request 2**, subscribe, and wait for green.

---

## Slice 3: Terminal parity and web (pull request 3)

Start from the latest `main` after pull request 2 has merged.

### Task 12: Pinned help capture for the installed Claude Code

**Files:**
- Create: `internal/terminalhost/testdata/help/claude.txt`
- Modify: `internal/terminalhost/testdata/help/sources.json`, `internal/terminalhost/testdata/help/README.md`

- [ ] **Step 1: Capture.** The file is `claude --version` output followed by `claude --help` output, exactly, from the installed binary.

```bash
(claude --version; claude --help) > internal/terminalhost/testdata/help/claude.txt; head -1 internal/terminalhost/testdata/help/claude.txt
```
Expected: the first line contains the installed version (2.1.319 at the time of writing).

- [ ] **Step 2: Record the source.** Add an entry to `sources.json` with `capture: "claude"`, `package: "@anthropic-ai/claude-code"`, the captured `version`, and `tarball` and `integrity` read from the registry (`npm view @anthropic-ai/claude-code@<version> dist.tarball dist.integrity`), and `commands: ["claude --version", "claude --help"]`. Add two lines to `README.md` saying Claude Code's capture shows `-c/--continue`, `--resume`, `--dangerously-skip-permissions` and `--effort`.

- [ ] **Step 3: Run the catalogue tests.**

Run: `go test ./internal/terminalhost -run 'TestPresetFlags|TestHelpToken|TestPresetFlagValues' -count=1`
Expected: PASS; the existing flag-in-help checks now cover `claude`.

- [ ] **Step 4: Commit** `Capture the installed Claude Code help for the launch presets`.

### Task 13: Resume, open-session, YOLO and effort in the preset

**Files:**
- Modify: `internal/terminalhost/catalogue.go` (the `claude` row), `internal/terminalproto/agents.go` (`agentOptions["claude"]`), `internal/terminalhost/catalogue_test.go`, `internal/terminalhost/launch_test.go`, `internal/terminalbroker/chat_test.go`

**Interfaces:**
- Produces: `Catalogue` row `claude` with `ResumeArgs: []string{"--continue"}`, `SessionArgs: []string{"--resume"}`, `YoloArgs: []string{"--dangerously-skip-permissions"}`, `EffortArgs` mapping `low`, `medium`, `high` to `{"--effort", level}`; `agentOptions["claude"] = {effort: true, model: true, yolo: true}`.

- [ ] **Step 1: Write the failing tests.** In `launch_test.go`: `TestLaunchArgvClaudeSession` mirroring `TestLaunchArgvOpenCodeSession` (`claude --dangerously-skip-permissions --resume <uuid> --model m`, then without YOLO and model), `TestLaunchArgvClaudeResume` (`claude --continue`), and a case in `TestLaunchArgvEffortMapping`'s style for `--effort high`. In `catalogue_test.go`: add `{"claude", []string{"--dangerously-skip-permissions"}}` to `TestYoloNeverWithoutFlag`, extend `TestPresetFlagsAppearInHelpText` so `ResumeArgs` and `SessionArgs` flags must also appear in the captured help, and assert `SupportsResume("claude")`. In `internal/terminalbroker/chat_test.go` beside `TestSetChatYoloChecksGatesAndTurnsOff`: `TestClaudeChatYoloHitsTheSameGates` (account setting off refuses, machine switch off refuses, both on allows, off turns it off).

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./internal/terminalhost ./internal/terminalbroker -run 'Claude|Yolo|Preset|Resume' -count=1`
Expected: FAIL (the row has none of the arguments).

- [ ] **Step 3: Implement** the row and the option-table entry exactly as in Interfaces. `TestControllerOptionTableMatchesCatalogue` keeps the two in step.

- [ ] **Step 4: Run the guard tests.**

Run: `go test ./internal/terminalhost ./internal/terminalproto ./internal/terminalbroker ./internal/api -count=1`
Expected: PASS, including `TestApprovalBypassDenylist` and `TestCatalogueKeepsApprovalBypassBehindYolo`.

- [ ] **Step 5: Commit** `Add resume, open-session, YOLO and effort to the Claude Code preset`.

### Task 14: Web catalogue, docs and pull request 3

**Files:**
- Modify: `web/src/lib/catalogue.ts` (the `claude` entry), `web/src/lib/catalogue.test.mjs`, `web/src/lib/models.test.mjs`, `docs/terminal-launch.md`, `docs/start-agent-session.md`

- [ ] **Step 1: Write the failing tests.** In `catalogue.test.mjs`: the Claude entry has `effort`, `resume` and `yolo` true; in `models.test.mjs`: `effortChoices('claude')` is `['', 'low', 'medium', 'high']`.

- [ ] **Step 2: Run them to see them fail.**

Run: `cd web && node src/lib/catalogue.test.mjs && node src/lib/models.test.mjs`
Expected: FAIL.

- [ ] **Step 3: Implement** `effort: true, resume: true, yolo: true` on the `claude` entry.

- [ ] **Step 4: Docs.** In `docs/terminal-launch.md`: replace the Claude row's arguments, add Claude Code to the YOLO list (`--dangerously-skip-permissions`) and to the effort and resume statements, and replace "Only `claude` was confirmed against a binary's own help text" with the pinned capture's version. In `docs/start-agent-session.md`: Claude Code takes effort, resume and YOLO. Note in both that chat YOLO for Claude now exists and uses the same three gates.

- [ ] **Step 5: Full verification.**

Run: `go vet ./... && go test ./... -count=1` then `cd web && npm run check` then the docs build and `python3 scripts/check-docs.py`.
Expected: all pass. Run `launch.e2e.mjs` and `start-agent.e2e.mjs` against `npm run preview`.

- [ ] **Step 6: Commit** `Offer Claude Code's thinking, resume and YOLO options in the web`, push, open **draft pull request 3**, subscribe, and wait for green.

---

## Slice 4: Hint, final docs and the held flip (pull request 4)

Start from the latest `main` after pull request 3 has merged. Write it; do not merge it until the owner has recorded device-check Run B.

### Task 15: The missing-adapter hint

**Files:**
- Create: `web/src/lib/claudeAdapter.ts`, `web/src/lib/claudeAdapter.test.mjs`
- Modify: `web/src/lib/StartAgentDialog.svelte` (beside the OpenCode hint at line 741), `web/src/lib/MachineSettings.svelte` (the Agents section), `web/package.json` (add the test to `test`), `web/tests/start-agent.e2e.mjs`

**Interfaces:**
- Produces: `claudeAdapterHint(machine: TerminalMachine | undefined): string | null`, and the exported constant `CLAUDE_ADAPTER_INSTALL = 'npm install --global @agentclientprotocol/claude-agent-acp@0.99.1'`.

- [ ] **Step 1: Write the failing unit tests.** `claudeAdapterHint` returns a sentence containing `CLAUDE_ADAPTER_INSTALL` when `capabilities.tools` includes `claude` and `capabilities.acpAgents` has a `claude` entry in state `missing`; returns `null` when the entry is `available`, when `claude` is not in `tools`, when `acpAgents` is absent (an older connector), and when the entry is `unverified` (the account has Claude chats off, or the controller does not advertise the gate).

- [ ] **Step 2: Run to see it fail.**

Run: `cd web && node src/lib/claudeAdapter.test.mjs`
Expected: FAIL (module missing).

- [ ] **Step 3: Implement** `claudeAdapter.ts`, show the sentence in the Start dialog (a `.hint` paragraph when the chosen agent is `claude`) and as a note under Claude's row in Machine settings' Agents section, and add a Playwright case to `start-agent.e2e.mjs`: a machine with `claude` in `tools` and a `missing` descriptor shows the hint with the install command; an `available` one does not.

- [ ] **Step 4: Run the checks.**

Run: `cd web && npm run check` and `node tests/start-agent.e2e.mjs` against `npm run preview`
Expected: pass.

- [ ] **Step 5: Commit** `Say what a machine needs to chat with Claude Code`.

### Task 16: Claude chats on by default (held)

**Files:**
- Modify: `internal/terminalproto/acp_agents.go` (`ChatDefaultOn`), `internal/terminalproto/acp_agents_test.go`, `internal/store/profile_defaults_test.go`, `docs/structured-sessions.md`, `docs/device-checks.md`

The setting already exists (Task 5) and stores only an account's explicit choices, so changing the default needs no data migration and keeps every choice an account made.

- [ ] **Step 1: Write the failing tests.** `TestClaudeChatIsOnByDefaultAndCodexIsNot` (`ChatDefaultOn("claude")` true, `ChatDefaultOn("codex")` false) and, in the store test, `TestAnExplicitOffChoiceSurvivesTheDefaultChange` (`Profile{ChatAgents: {claude: false}}.ChatEnabled("claude")` is false; an empty profile's is true).

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./internal/terminalproto ./internal/store -run 'ChatIsOnByDefault|ExplicitOffChoice' -count=1`
Expected: FAIL.

- [ ] **Step 3: Implement.** `ChatDefaultOn` returns true for `claude` only. Update the earlier tests that assumed the default was off (Tasks 4 and 5's) to set the choice explicitly.

- [ ] **Step 4: Docs.** In `docs/device-checks.md` record the owner's signed-in Run B result and its date; in `docs/structured-sessions.md` say Claude chats are on unless an account turned them off.

- [ ] **Step 5: Run everything.**

Run: `go vet ./... && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 6: Commit** `Offer Claude Code chats by default`.

### Task 17: Final docs and pull request 4

**Files:**
- Modify: `docs/structured-sessions.md`, `docs/start-agent-session.md`, `docs/device-checks.md`, `docs/roadmap.md`

- [ ] **Step 1: Docs.** Add a "Claude Code in Gaffer" section to `docs/structured-sessions.md` in the shape of the other per-agent sections: the adapter and its pin, what is offered, the sign-in refusal and `code: "login"`, the missing-adapter hint, history and reconnect, thinking levels, and an explicit list of what was not measured (a signed-in turn, real image acceptance, per-model effort values). Update the `roadmap.md` structured-chats line and `device-checks.md` (the Claude row now records the owner's Run B result, filled in when the owner provides it).

- [ ] **Step 2: Full verification.**

Run: `go vet ./... && go test ./... -count=1`, `cd web && npm run check`, then the docs build and `python3 scripts/check-docs.py`.
Expected: all pass.

- [ ] **Step 3: Commit** `Document Claude Code in Gaffer`, push, open **draft pull request 4** with a note that it is held until the owner records Run B, subscribe, and leave it unmerged.
