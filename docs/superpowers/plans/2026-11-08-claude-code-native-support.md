# Claude Code, Fully Supported Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring Claude Code to the bar OpenCode, Google Antigravity (AGY) and the other fully supported agent meet: a Gaffer chat with models, effort, stored history and in-place reconnect, and a terminal launch with resume, open-session, YOLO and effort.

**Architecture:** Four slices, each its own draft pull request. Slice 1 only measures (a probe and opt-in tests). Slice 2 turns on in the descriptor and profile only what slice 1 proved. Slice 3 adds terminal parity to the connector's preset and the controller's option table. Slice 4 adds the missing-adapter hint, final docs and the switch flip, and is held until the owner records the signed-in run.

**Tech Stack:** Go (connector `internal/acpagent`, `internal/terminalhost`, `internal/terminalproto`, `internal/terminalagent`, `internal/terminalbroker`), Svelte 5 + TypeScript (`web/src/lib`), Python probe scripts, MkDocs docs.

**Spec:** `docs/superpowers/specs/2026-11-08-claude-code-native-support-design.md`

## Global Constraints

- The adapter is `@agentclientprotocol/claude-agent-acp`, pinned **0.99.1** (`ClaudeCandidate.MinVersion`). The `claude` CLI does not speak ACP.
- `Create.Effort` stays `low|medium|high` (`terminalproto.ValidEffort`). No new wire feature is added for effort. The Start dialog offers only those three; the live chat control offers every value the session reports.
- `fast` and `ultracode` stay listed in the descriptor and uncontrolled. The live chat has no control for the `model_config` category for any agent; building one is a separate change.
- `ClaudeSessionMeta` (`settingSources: ["user"]`) stays on both new and load.
- The profile keeps requiring the adapter's `default` mode. Claude's other permission modes are out of scope.
- The terminal one-word prompt rule for `claude` stays.
- YOLO needs the account setting, the machine switch and the confirmation, unchanged.
- Nothing is turned on that a recorded probe did not show. The switch `REET_EXPERIMENTAL_CLAUDE_CHAT` stays until Task 12, which is held.
- Commit messages and pull request text name only Claude Code, OpenCode and Google Antigravity; they say "the other agents" otherwise. Commits end with the two attribution lines from the session reminder.
- Each slice is a draft pull request off the latest `main`, subscribed, and green before the next slice starts. Slice 2 starts only after the AGY reconnect fix (#666) has merged.

## Review Focus

- A machine with the switch off: Claude's descriptor stays `unverified`/`missing` with no new flags and no hint (Tasks 4, 11).
- An effort the adapter does not confirm: the chat refuses and no opening prompt is sent (Task 6).
- Stored history opened for another folder, machine or account: still refused by the existing binding, now reachable for Claude (Task 5).
- Chat YOLO becomes available for Claude as a side effect of `yolo: true` (`terminalbroker/chat.go` checks `ToolTakesYolo`); it must hit the same three gates as every other agent (Task 9).
- An older controller or connector: no new descriptor fields reach a peer that did not negotiate them; Claude's `Reattach` is stripped without `acp-agent-reattach-v1` (Task 4).

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
Expected: PASS. If `TestRealClaudeLoadUnstored` fails because the adapter's error is not mapped to `ErrNoStoredConversation`, note the actual error code in the ledger for Task 5, which fixes the mapping.

- [ ] **Step 4: Commit** `Add opt-in real-adapter checks for Claude Code options and unstored history`.

### Task 3: Record the results and open pull request 1

**Files:**
- Modify: `docs/research/structured-adapters/probe-report.md` (new section at the end), `docs/device-checks.md` (one line under Recorded results)

- [ ] **Step 1: Run the existing `load` scenario** and keep its JSON: `python3 scripts/probe-claude-lifecycle.py load --workspace "$probe_dir"`.

- [ ] **Step 2: Add a section "Claude parity probes (8 November 2026)"** with one results table, one row per fact, each with the measured value and the scenario or test that produced it: history replay delivered, unstored-load error code, `image` prompt capability, image block seen by the stub, `effort` present in `session/new`, effort read-back, unknown-effort behaviour, `fast`/`ultracode` read-back, option list after a model change. State plainly that the stub shows bytes arriving, not that a real model accepts images.

- [ ] **Step 3: Verify the docs build.**

Run: `DOCS_SITE_URL=https://reet.sh/ mkdocs build --strict -q && python3 scripts/check-docs.py`
Expected: "Docs checks passed".

- [ ] **Step 4: Commit** `Record the Claude Code parity probe results`, push, open **draft pull request 1** (probe only), subscribe to it, and wait for green. The table is the gate for Tasks 5 to 7.

---

## Slice 2: Gaffer parity behind the switch (pull request 2)

Start from the latest `main` after pull requests 1 and #666 have merged. Read Task 3's table first; each conditional step below says which row it depends on.

### Task 4: Descriptor and profile report only what was proven

**Files:**
- Modify: `internal/acpagent/describe.go` (the `if tool == "claude"` arm), `internal/acpagent/claude.go` (`ClaudeCandidate`), `internal/acpagent/describe_test.go`

**Interfaces:**
- Produces: for an available Claude descriptor, `Model`, `Approval`, `History` and `Reattach` true; `Images` true only if Task 3 shows the adapter takes image blocks; `Questions` false; `Modes` empty. `ClaudeCandidate.Reattach = true`.

- [ ] **Step 1: Write the failing tests.** In `describe_test.go`: `TestDescribeClaudeReportsOnlyProvenCapabilities` asserts the exact flags above (with the switch on); `TestDescribeClaudeWithTheSwitchOffReportsNothing` asserts `Describe("claude", true, "")` is `unverified` with every capability false when `REET_EXPERIMENTAL_CLAUDE_CHAT` is unset; update the `claude` row of `TestDescribeClaimsReattachOnlyWhereTheProfileCanReattach` to expect `true` and `Reattach: true` in the profile, and update the existing Claude descriptor assertions in `describe_test.go` that expect only `Model` and `Approval`.

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./internal/acpagent -run 'TestDescribeClaude|TestDescribeClaimsReattach' -count=1`
Expected: FAIL (Claude reports no history or reattach).

- [ ] **Step 3: Implement.** In `Describe`, the Claude arm sets `entry.History = true` and `entry.Reattach = spec.Reattach` besides `Model` and `Approval`, and `entry.Images = true` only behind a named constant `claudeTakesImages` whose value is Task 3's measured result. Set `Reattach: true` on `ClaudeCandidate`.

- [ ] **Step 4: Add the peer-negotiation test.** In `internal/agenthealth/acp_agents_test.go`, beside `TestAgentReattachClaimEnablesAnotherAgentsReattach`, add `TestClaudeReattachIsStrippedWithoutTheAgentReattachFeature`: with the switch on and the feature not negotiated, Claude's descriptor has `Reattach` false and `History` follows `acpHistory`; with it negotiated, `Reattach` is true.

- [ ] **Step 5: Run the touched packages.**

Run: `go test ./internal/acpagent ./internal/agenthealth ./internal/terminalproto -count=1`
Expected: PASS.

- [ ] **Step 6: Commit** `Report the Claude Code chat capabilities the probe proved`.

### Task 5: History continue and in-place reattach for Claude

**Files:**
- Modify: `internal/acpagent/claude_test.go`, `internal/acpagent/session.go` (only if the unstored-load error is not already mapped), `internal/terminalagent/chat_reattach_test.go` or the existing reattach test file, `internal/terminalbroker` continue-with-history test file

**Interfaces:**
- Consumes: `Start(ctx, Options{Resume: ...})`, `ErrNoStoredConversation`, `Describe(...).History`, `acpagent.Lookup("claude")`.
- Produces: `claudeOptions(t *testing.T, scenario string) (Options, *recorder)` helper in `claude_test.go`, mirroring the existing per-agent options helper (`grep -n 'func .*Options(t' internal/acpagent/*_test.go`).

- [ ] **Step 1: Write the failing tests.** `TestClaudeReattachesInPlace` and `TestClaudeReattachWithoutAStoredConversationIsRefused` in `claude_test.go`, mirroring the existing in-place reattach and unstored-conversation tests for the other reattach-capable agent (`grep -n 'ReattachesInPlace\|WithoutAStoredConversation' internal/acpagent/*_test.go`), with the fake agent speaking the frames Task 3 recorded (use the unstored-load error code from the table). In the terminalagent reattach test file, `TestChatReattachAcceptsClaudeWhenTheProfileAllowsIt` (with the switch on, `h.reattach` does not return `acpagent.ErrUnsupported`). In the broker's continue-with-history tests, `TestContinueClaudeWithHistoryIsBoundToItsFolder`: a closed Claude session continues in its own folder and is refused for another folder, machine or account.

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./internal/acpagent ./internal/terminalagent ./internal/terminalbroker -run 'Claude' -count=1`
Expected: FAIL only where the behaviour is missing (reattach before Task 4's profile flag, the unmapped error code).

- [ ] **Step 3: Implement** only what the failures show: map the adapter's unstored-load error to `ErrNoStoredConversation` in `session.go` (the single place that already does it for `session/load` replies), nothing else.

- [ ] **Step 4: Run the three packages.**

Run: `go test ./internal/acpagent ./internal/terminalagent ./internal/terminalbroker -count=1`
Expected: PASS.

- [ ] **Step 5: Commit** `Reconnect and continue Claude Code chats from their stored history`.

### Task 6: Effort at start, confirmed

**Files:**
- Modify: `internal/acpagent/claude_test.go`, `web/src/lib/terminal/launchOptions.ts`, `web/src/lib/models.test.mjs`

**Interfaces:**
- Consumes: `Options.Effort` and the existing `thought_level` handling in `Start` (`session.go`, the block after the model check); `claudeOptions` from Task 5.
- Produces: `launchEfforts(tool, caps, chat)` returns only values in `{low, medium, high}` for a chat (plus the leading empty string).

- [ ] **Step 1: Write the failing Go tests** in `claude_test.go`: `TestClaudeStartsWithAConfirmedEffort` (fake reports `effort` in `session/new`, accepts `high`, reads it back; the session's `thought_level` setting reads `high` before `OnReady`), `TestClaudeRefusesAnEffortTheAdapterDoesNotConfirm` (fake echoes the old value; `Start` returns an error, no prompt is sent, the process group is retired) and `TestClaudeWithoutAnEffortOptionRefusesAnEffort` (`errors.Is(err, ErrControl)`). Skip the first test's assertions and say so in the ledger if Task 3 shows `effort` is not in `session/new`.

- [ ] **Step 2: Write the failing web test** in `web/src/lib/models.test.mjs`: with a Claude descriptor whose `thought_level` values are `default, low, medium, high, xhigh, max`, `launchEfforts('claude', caps, true)` equals `['', 'low', 'medium', 'high']`, and a Codex-shaped list with `ultra` drops it likewise.

- [ ] **Step 3: Run both to see them fail.**

Run: `go test ./internal/acpagent -run 'TestClaude.*Effort' -count=1` and `cd web && node src/lib/models.test.mjs`
Expected: FAIL (the web list still includes `xhigh`, `max`).

- [ ] **Step 4: Implement.** `launchEfforts` keeps only `low`, `medium` and `high` from the descriptor's values for a chat, since `Create.Effort` cannot carry the rest; add one line to its comment saying so. The Go side needs no change if Step 1's tests already pass.

- [ ] **Step 5: Run the checks.**

Run: `go test ./internal/acpagent -count=1 && cd web && npm run check`
Expected: PASS, 0 errors.

- [ ] **Step 6: Commit** `Offer Claude Code's thinking levels in Gaffer and confirm them at start`.

### Task 7: Images (only if proven) and the docs for this slice

**Files:**
- Modify: `internal/acpagent/claude_test.go`, `docs/structured-sessions.md`

- [ ] **Step 1: Gate.** If Task 3's table says the adapter takes image blocks, write `TestClaudePromptCarriesImages` (a fake agent that records the prompt; an image attachment arrives as an `image` block) and make it pass; otherwise skip this task's code, record `Ruling: images stay off for Claude — the probe showed <result>` in the ledger, and keep `claudeTakesImages` false.

- [ ] **Step 2: Docs.** In `docs/structured-sessions.md` replace the sentence that Claude reports "approvals and the model switch only" with what the descriptor now reports, and state which rows of the probe table back each capability and that a signed-in turn has not been run.

- [ ] **Step 3: Full verification.**

Run: `go vet ./... && go test ./... -count=1` then `cd web && npm run check` then `DOCS_SITE_URL=https://reet.sh/ mkdocs build --strict -q && python3 scripts/check-docs.py`
Expected: all pass.

- [ ] **Step 4: Commit** `Document Claude Code history, reconnect and thinking levels in Gaffer`, push, open **draft pull request 2**, subscribe, and wait for green.

---

## Slice 3: Terminal parity and web (pull request 3)

Start from the latest `main` after pull request 2 has merged.

### Task 8: Pinned help capture for the installed Claude Code

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

### Task 9: Resume, open-session, YOLO and effort in the preset

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

### Task 10: Web catalogue, docs and pull request 3

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

### Task 11: The missing-adapter hint

**Files:**
- Create: `web/src/lib/claudeAdapter.ts`, `web/src/lib/claudeAdapter.test.mjs`
- Modify: `web/src/lib/StartAgentDialog.svelte` (beside the OpenCode hint at line 741), `web/src/lib/MachineSettings.svelte` (the Agents section), `web/package.json` (add the test to `test`), `web/tests/start-agent.e2e.mjs`

**Interfaces:**
- Produces: `claudeAdapterHint(machine: TerminalMachine | undefined): string | null`, and the exported constant `CLAUDE_ADAPTER_INSTALL = 'npm install --global @agentclientprotocol/claude-agent-acp@0.99.1'`.

- [ ] **Step 1: Write the failing unit tests.** `claudeAdapterHint` returns a sentence containing `CLAUDE_ADAPTER_INSTALL` when `capabilities.tools` includes `claude` and `capabilities.acpAgents` has a `claude` entry in state `missing`; returns `null` when the entry is `available`, when `claude` is not in `tools`, when `acpAgents` is absent (an older connector), and when the entry is `unverified` (the switch is off).

- [ ] **Step 2: Run to see it fail.**

Run: `cd web && node src/lib/claudeAdapter.test.mjs`
Expected: FAIL (module missing).

- [ ] **Step 3: Implement** `claudeAdapter.ts`, show the sentence in the Start dialog (a `.hint` paragraph when the chosen agent is `claude`) and as a note under Claude's row in Machine settings' Agents section, and add a Playwright case to `start-agent.e2e.mjs`: a machine with `claude` in `tools` and a `missing` descriptor shows the hint with the install command; an `available` one does not.

- [ ] **Step 4: Run the checks.**

Run: `cd web && npm run check` and `node tests/start-agent.e2e.mjs` against `npm run preview`
Expected: pass.

- [ ] **Step 5: Commit** `Say what a machine needs to chat with Claude Code`.

### Task 12: Retire the switch (held)

**Files:**
- Modify: `internal/acpagent/spec.go` (`Lookup`, `ClaudeSwitch`, `ClaudeEnabled`), every test that sets `ClaudeSwitch` (`grep -rn ClaudeSwitch internal scripts docs`), `docs/structured-sessions.md`, `docs/device-checks.md`

- [ ] **Step 1: Write the failing test** `TestClaudeIsRegisteredWithoutASwitch` in `spec_test.go`: with `REET_EXPERIMENTAL_CLAUDE_CHAT` unset, `Lookup("claude")` returns `ClaudeCandidate, true` and `Describe("claude", true, "")` is `available`.

- [ ] **Step 2: Run to see it fail.**

Run: `go test ./internal/acpagent -run TestClaudeIsRegisteredWithoutASwitch -count=1`
Expected: FAIL.

- [ ] **Step 3: Implement.** `Lookup` returns `ClaudeCandidate` for `claude` unconditionally; delete `ClaudeSwitch` and `ClaudeEnabled`; remove `t.Setenv(ClaudeSwitch, ...)` from the tests that set it. Codex is untouched.

- [ ] **Step 4: Update `TestDescribeClaudeWithTheSwitchOffReportsNothing`** (Task 4) into `TestDescribeClaudeAvailableWhenTheAdapterIsInstalled`; the `unverified` case now applies only to an absent adapter.

- [ ] **Step 5: Run everything.**

Run: `go vet ./... && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 6: Commit** `Offer Claude Code chats without a per-machine switch`.

### Task 13: Final docs and pull request 4

**Files:**
- Modify: `docs/structured-sessions.md`, `docs/start-agent-session.md`, `docs/device-checks.md`, `docs/roadmap.md`

- [ ] **Step 1: Docs.** Add a "Claude Code in Gaffer" section to `docs/structured-sessions.md` in the shape of the other per-agent sections: the adapter and its pin, what is offered, the sign-in refusal and `code: "login"`, the missing-adapter hint, history and reconnect, thinking levels, and an explicit list of what was not measured (a signed-in turn, real image acceptance, per-model effort values). Replace the "Experimental: Claude Code and Codex chats" section's Claude half. Update the `roadmap.md` structured-chats line and `device-checks.md` (the Claude row now records the owner's Run B result, filled in when the owner provides it).

- [ ] **Step 2: Full verification.**

Run: `go vet ./... && go test ./... -count=1`, `cd web && npm run check`, then the docs build and `python3 scripts/check-docs.py`.
Expected: all pass.

- [ ] **Step 3: Commit** `Document Claude Code in Gaffer`, push, open **draft pull request 4** with a note that it is held until the owner records Run B, subscribe, and leave it unmerged.
