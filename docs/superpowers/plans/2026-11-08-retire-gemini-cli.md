# Retire Gemini CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Gemini CLI is no longer offered, started, validated or managed, while older machines that still report it stay connected and past data keeps working.

**Architecture:** A small `RetiredTool` rule in the protocol package lets an older connector's `Tools` list keep validating, then Gemini is removed from the launch tool set, presets, connector catalogue, Setup adapter, web catalogue and docs. A one-time, idempotent store migration blanks stale defaults and deletes derived rows; history is never touched.

**Tech Stack:** Go (protocol, connector, controller, SQLite store, Setup), Svelte 5 + TypeScript (`web/`), plain Node Playwright scripts, MkDocs.

**Spec:** `docs/superpowers/specs/2026-11-08-retire-gemini-cli-design.md`

## Global Constraints

- The retired id is exactly `gemini`. Function names: `terminalproto.RetiredTool(id string) bool` and `terminalproto.RetiredTools() []string`; web `RETIRED_AGENTS` and `agentDisplayName`.
- A capability `Tools` list may contain a retired tool (ordering rule unchanged); it is never offered, launched or required.
- Migration set, in one transaction and idempotent: blank default agent, model and effort for the account profile and folder projects; blank agent, model and effort on backlog drafts (text kept); delete `model_favourites` and `machine_agent_models` rows for the retired tool. Terminal sessions, selection-run records, audit events and Setup history are untouched. The legacy-project migration blanks a retired agent instead of copying it.
- Antigravity's `GEMINI_API_KEY` login variable and Google's Gemini model names are not Gemini CLI and stay. `docs/research/*`, the handoff files and these two superpowers documents stay.
- No new dependency. British spelling in copy. Before each commit run `go vet ./...`, the Go packages touched (`go test <pkgs> -count=1`) and, for web changes, `cd web && npm run check`.
- Commit messages and PR text name only the removed agent; they refer to the rest as "the other agents".
- Every commit message ends with these two lines, after a blank line: `Co-Authored-By: Claude <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_011MNNznG9VgYznXqLkTHZYo`.

## Review Focus

- An older connector reports `gemini` in its `Tools`: the capability record still validates and the machine stays ready, and a create naming `gemini` is refused. Pinned in Tasks 1 and 3.
- An older browser saves `gemini` as a default after the upgrade: refused as an unknown agent, not stored. Pinned in Task 3.
- The migration only touches rows naming `gemini`, leaves other agents' rows, sessions and events alone, and a second open changes nothing (including row versions). Pinned in Task 4.
- A machine that still has `~/.gemini`: Setup discovery ignores it without error. Pinned in Task 5.
- A past `gemini` session reads "Gemini CLI", never "gemini", and is never offered Continue. Pinned in Task 6.

---

### Task 1: The retired-agents rule

**Files:**
- Modify: `internal/terminalproto/launch.go` (`validTools`, new vars and functions)
- Test: `internal/terminalproto/launch_test.go`

**Interfaces:**
- Produces: `func RetiredTool(id string) bool`; `func RetiredTools() []string` (sorted ids; today `["gemini"]`).

- [ ] **Step 1: Write the failing test** `TestRetiredToolsAreToleratedInAToolsList`: `RetiredTool("gemini")` is true and false for `"claude"`, `"antigravity"`, `""`, `"vim"`; `RetiredTools()` equals `[]string{"gemini"}`; a `Capabilities{ProtocolVersion:1, OS:"linux", OSUser:"dev", TerminalEnabled:true, TerminalControl:true, Launch:true, Tools:[]string{"claude","gemini"}, Readiness:Ready}` has `ValidateFor(LaunchFeature)` and `ValidateFor(LaunchFeature+","+AgentsFeature)` both nil.
- [ ] **Step 2: Run** `go test ./internal/terminalproto/ -run RetiredTools -count=1`. Expected: FAIL (undefined names).
- [ ] **Step 3: Implement.** `retiredTools = map[string]bool{"gemini": true}`; `validTools` also accepts `RetiredTool(tool)`; `RetiredTools` returns the sorted keys.
- [ ] **Step 4: Run** `go test ./internal/terminalproto/ -count=1`. Expected: PASS.
- [ ] **Step 5: Commit** `Add a rule for retired agents`.

### Task 2: Connector, catalogue and scan

**Files:**
- Modify: `internal/terminalhost/catalogue.go` (delete the `gemini` preset row; drop the Gemini clause from the evidence comment and say "three" where it says "four"), `internal/acpagent/describe.go` (drop `"gemini"` from `adapters`), `internal/scan/agent_terminal_control.go` (drop `"gemini"` from the tool loop and the sentence at line 177)
- Test: `internal/terminalhost/catalogue_test.go`, `launch_test.go`, `manager_test.go`, `internal/acpagent/describe_test.go`, `internal/agenthealth/launch_test.go`

**Interfaces:**
- Consumes: nothing new. Produces: a connector that never reports `gemini` in `InstalledTools`.

- [ ] **Step 1: Update the tests to the new truth (they fail first).** `catalogue_test.go`: the installed-tools expectation becomes `[]string{"claude","codex","opencode"}`. `launch_test.go`: delete the `"gemini"` argv case and drop `gemini` from every `fakeLookPath` list and from the `InstalledTools` expectations. `manager_test.go`: the "flag prompt only semicolon" row uses `Tool:"opencode"` with `Prompt:";"` and wants `[]string{"--prompt", ";"}`; keep the `Tool:"gemini"` row in the refused list (a retired tool is refused). `describe_test.go`: delete the `gemini` rows. `agenthealth/launch_test.go`: the fixture tool list uses `"claude","codex"`.
- [ ] **Step 2: Run** `go test ./internal/terminalhost/ ./internal/acpagent/ ./internal/scan/ ./internal/agenthealth/ -count=1`. Expected: FAIL in the updated tests.
- [ ] **Step 3: Remove** the catalogue row, the `adapters` entry and the scan loop entry as listed above.
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Commit** `Stop the connector offering Gemini CLI`.

### Task 3: Remove it from the protocol and the controller

**Files:**
- Modify: `internal/terminalproto/launch.go` (drop `gemini` from `launchTools`), `internal/terminalproto/agents.go` (drop the `gemini` preset row), `internal/terminalproto/autotitle.go` (drop the name entry), `internal/store/store_folder_projects.go` and `internal/store/store_projects.go` (drop `gemini` from the three "agent must be one of…" messages and leave the rest of each message unchanged)
- Test: `internal/terminalproto/launch_test.go`, `acp_test.go`, `internal/terminalbroker/launch_test.go`, `internal/api/terminal_launch_test.go`, `terminal_launch_integration_test.go`, `terminal_chat_test.go`, `session_backlog_test.go`, `terminal_agents_test.go`, `projects_migrate_test.go`, `internal/store/store_projects_test.go`, `store_projects_migrate_test.go`

**Interfaces:**
- Consumes: `RetiredTool` (Task 1). Produces: `ValidTool("gemini")` and `ValidToolV2("gemini")` are false.

- [ ] **Step 1: Write the new failing tests.** `terminalproto`: `TestRetiredToolCannotBeLaunched` (`ValidTool`, `ValidToolV2`, `ToolTakesModel` are false for `gemini`; `Create{Title:"A", Cols:80, Rows:24, Tool:"gemini"}.Validate()` is non-nil for a plain and for a `Structured` create). `terminalbroker`: `TestRetiredToolIsRefusedButAnOlderMachineStaysReady` (`launchFixture(t, launchCapabilities("claude", "gemini"))` is built without error, and `f.b.Create` with `Tool:"gemini"` returns an error and sends nothing to the machine). `api`: `TestRetiredToolIsNotAcceptedAsADefault` (`PATCH /api/v2/profile {"defaultTool":"gemini"}` is 400 with the existing "defaultTool must be blank or one of the agents reet can start" message and changes nothing; a folder-project save with `defaultTool:"gemini"` is refused too).
- [ ] **Step 2: Run** `go test ./internal/terminalproto/ ./internal/terminalbroker/ ./internal/api/ -run 'Retired' -count=1`. Expected: FAIL (gemini is still valid).
- [ ] **Step 3: Make the removals** listed under Files.
- [ ] **Step 4: Fix every other test that named Gemini CLI.** Run `go test ./... -count=1 2>&1 | grep -E "^(--- FAIL|FAIL)"` and repair each by these rules: where `gemini` was a stand-in for "another reviewed tool", substitute `opencode` or `codex` (keep the assertion); where Gemini's own behaviour was under test, delete that case; never weaken an assertion; leave `acp_agents_test.go`'s `nativeIDMatches("gemini", …)` as it is (an unknown tool still matches nothing). The `"other tool"` row in `acp_test.go` becomes `"vim"`.
- [ ] **Step 5: Run** `go vet ./... && go test ./... -count=1`. Expected: PASS in every package.
- [ ] **Step 6: Commit** `Remove Gemini CLI from the launch rules and presets`.

### Task 4: One-time migration of stored data

**Files:**
- Create: `internal/store/store_retired_agents.go`
- Modify: `internal/store/store.go` (call it in `Open` straight after `migrateSessionBacklog`), `internal/store/store_projects_migrate.go` (`migratedFolderProject`)
- Test: `internal/store/store_retired_agents_test.go` (create), `internal/store/store_projects_migrate_test.go`

**Interfaces:**
- Consumes: `terminalproto.RetiredTools()`, `terminalproto.RetiredTool`. Produces: `func (s *Store) retireAgents() error`.

- [ ] **Step 1: Write the failing tests.** `TestRetiredAgentDefaultsAreClearedOnOpenAndHistoryIsKept`: open a store; create rows with the store's own APIs using `claude`, then set `gemini` on them by SQL so validation is bypassed: the owner's `user_profiles` default tool, model and effort; a folder project's defaults; a backlog draft (its JSON `body` carries `tool`, `model`, `effort` and a text field); a model favourite; a `machine_agent_models` row; a terminal session. Add a second set of rows for another agent. Close and reopen the store. Assert: the retired profile, project and draft rows have blank tool, model and effort (the draft text unchanged, the project and draft `version` increased by one); the retired favourite and cached list are gone; the terminal session still says `gemini`; the other agent's rows are identical; closing and reopening again changes no row and no `version`. `TestLegacyProjectMemberWithARetiredAgentMigratesWithoutIt` in `store_projects_migrate_test.go`: seed a legacy project whose member names `gemini` by SQL (follow the existing tests there); after the migration the folder project exists with an empty default agent and model.
- [ ] **Step 2: Run** `go test ./internal/store/ -run 'Retired|RetiredAgent' -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement** `retireAgents`: one transaction, looping over `terminalproto.RetiredTools()` with these statements: `UPDATE user_profiles SET default_tool='', default_model='', default_effort='' WHERE default_tool=?`; `UPDATE folder_projects SET default_tool='', default_model='', default_effort='', version=version+1, updated_at=? WHERE default_tool=?`; `UPDATE session_backlog SET body=json_set(body,'$.tool','','$.model','','$.effort',''), version=version+1, updated_at=? WHERE json_extract(body,'$.tool')=?`; `DELETE FROM model_favourites WHERE tool=?`; `DELETE FROM machine_agent_models WHERE tool=?`. In `migratedFolderProject`, after `resolvedLaunch`, `if terminalproto.RetiredTool(tool) { tool, model = "", "" }`.
- [ ] **Step 4: Run** `go test ./internal/store/ -count=1`. Expected: PASS.
- [ ] **Step 5: Commit** `Clear stale Gemini CLI defaults when the store opens`.

### Task 5: Setup

**Files:**
- Modify: `internal/setup/adapters.go` (delete the `gemini` adapter), `components.go` (`case "pi", "gemini":` becomes `case "pi":`), `installation.go` (delete the map entry), `policy.go` (delete the `case "gemini":` block), `summaries.go` (delete the `httpUrl` special case and its comment), `support.go` (delete the row), `versions.go` (delete the prefix entry)
- Test: `internal/setup/cli_test.go`, `components_test.go`, `discovery_test.go`, `installation_test.go`, `instructions_test.go`, `policy_test.go`, `support_test.go`, `versions_test.go`

**Interfaces:** none new.

- [ ] **Step 1: Write the failing test** `TestDiscoveryIgnoresAMachineThatStillHasGeminiFiles` in `discovery_test.go`: build the existing test environment, add `~/.gemini/settings.json`, `~/.gemini/GEMINI.md` and a project `GEMINI.md`, run discovery; the report has no agent with id `gemini`, returns no error, and none of those paths appears in the report.
- [ ] **Step 2: Update the existing tests** to the new truth: remove the `.gemini` fixtures and expectations in `cli_test.go` (including the "Gemini CLI: detected" line), `components_test.go` (the HTTP-transport case), `instructions_test.go` (both `GEMINI.md` fixtures and their expectations), `policy_test.go`, `installation_test.go`, `versions_test.go` (the parse row, the probe output and the expected-state map) and `support_test.go` (the `byID["gemini"]` assertion); drop `gemini` from the id list in `discovery_test.go`.
- [ ] **Step 3: Run** `go test ./internal/setup/ -count=1`. Expected: FAIL.
- [ ] **Step 4: Remove** the adapter and every entry listed under Files.
- [ ] **Step 5: Run** `go test ./internal/setup/ -count=1`. Expected: PASS.
- [ ] **Step 6: Commit** `Stop managing Gemini CLI's configuration in Setup`.

### Task 6: Web

**Files:**
- Modify: `web/src/lib/catalogue.ts` (delete the `gemini` entry; `ORIGINAL` becomes `claude`, `codex`, `opencode`; add `RETIRED_AGENTS` and `agentDisplayName`), `web/src/lib/agentLaunch.ts` (delete the `TOOLS` entry; drop `gemini|` from the title-filler pattern; `nameOf` uses `agentDisplayName`), `web/src/lib/models.ts` (`agentNameOf` uses `agentDisplayName`)
- Test: `web/src/lib/catalogue.test.mjs`, `agentLaunch.test.mjs`, `draftOptions.test.mjs`, `events.test.mjs`, `launchBar.test.mjs`, `models.test.mjs`, `web/tests/add-project.e2e.mjs`, `launch.e2e.mjs`, `start-agent.e2e.mjs`

**Interfaces:**
- Produces: `export const RETIRED_AGENTS: Record<string, string>` (`{ gemini: 'Gemini CLI' }`); `export function agentDisplayName(id: string): string` (catalogue name, else retired label, else the id).

- [ ] **Step 1: Write the failing unit tests.** `catalogue.test.mjs`: `agentDisplayName('gemini')` is `Gemini CLI`, `agentDisplayName('claude')` is `Claude Code`, an unknown id is returned as given; `agentChoices` for a machine with `capabilities.tools:['claude','gemini']` contains no Gemini choice; for a machine with `launch:true` and no `agentsV2` the available choices are exactly the three original agents. `agentLaunch.test.mjs`: `toolName('gemini')` is `Gemini CLI`; `TOOLS` has no `gemini`. `sessions` coverage (add to an existing test file that already imports `canContinue`, else `agentLaunch.test.mjs`): `canContinue({state:'ended', tool:'gemini', dir:'/work', projectId:null}, {capabilities:{resume:true}}, [])` is false.
- [ ] **Step 2: Run** `cd web && node src/lib/catalogue.test.mjs && node src/lib/agentLaunch.test.mjs`. Expected: FAIL.
- [ ] **Step 3: Implement** the changes under Files.
- [ ] **Step 4: Update the other unit tests** that named Gemini (`draftOptions`, `events`, `launchBar`, `models`) by the Task 3 rules: substitute `opencode` or `codex` for a stand-in, delete a case that tested Gemini itself. Run `cd web && npm run check`. Expected: PASS.
- [ ] **Step 5: Update the e2e files.** `add-project.e2e.mjs`: the Agent option lists drop `Gemini CLI`. `launch.e2e.mjs`: drop `gemini` from `caps().tools` and delete the `pick(page,'Agent','Gemini CLI')` case. `start-agent.e2e.mjs`: delete the Gemini picks and the one-word-prompt case that depended on it; keep `gemini` in the `tools` list of the machine at line 379 and assert the Agent select offers no Gemini option (an older connector's report is ignored).
- [ ] **Step 6: Run** the three e2e files against the built app (as in `.github/workflows/terminal-browser.yml`: `npm run build`, `npm run preview -- --host 127.0.0.1 --port 4173`, `node web/tests/<file>.e2e.mjs`; `session-board` needs `REET_UI_URL=http://127.0.0.1:4173`). Expected: each prints its pass line.
- [ ] **Step 7: Commit** `Remove Gemini CLI from the web catalogue and keep it labelled in history`.

### Task 7: Docs and the final sweep

**Files:**
- Modify: `README.md` (lines 286, 363, 490), `docs/setup-discovery.md`, `docs/terminal-launch.md`, `docs/compare-open-webui.md`, `docs/quick-start.md`, `docs/run-claude-code-on-a-vps.md`, `docs/compare-librechat.md`, `docs/compare-skills-sh.md`, `docs/structured-chat-plan.md`, `docs/self-hosted-ai-agent-control-room.md`, `docs/agent-models.md`, `docs/index.md`, `docs/start-agent-session.md`, `docs/structured-sessions.md`, `docs/assets/stylesheets/reet.css` (the three `.agent-glyph.gemini` rules, only if no page or override still uses that class)

- [ ] **Step 1: Edit the docs.** Remove Gemini CLI from every agent list and sentence; the README line about the other supported agent no longer says "alongside Gemini CLI"; the README's discovery lists drop Gemini and `GEMINI.md`. Where a page says "the original four" agents, say three. Add one short note to `docs/start-agent-session.md` and `docs/setup-discovery.md`: Gemini CLI is retired; reet no longer starts or manages it, an older connector that still reports it is ignored, and existing history is kept. Check usage with `grep -rn "agent-glyph" docs overrides | grep -i gemini` before deleting the CSS rules.
- [ ] **Step 2: Sweep.** Run `grep -rIil gemini --exclude-dir=node_modules --exclude-dir=.git --exclude-dir=dist --exclude-dir=site .`. Every remaining file must be in this allowlist, each for a stated reason: Antigravity code and tests (`GEMINI_API_KEY`, Google model names), `docs/research/*`, the handoff files, the two superpowers documents, and the retired-agent code and tests from Tasks 1, 3, 4 and 6. Anything else is a miss to fix.
- [ ] **Step 3: Verify.** `python3 -m venv` a scratch environment, install `docs/requirements.txt`, run `DOCS_SITE_URL=https://reet.sh/ mkdocs build --strict -q && python scripts/check-docs.py`. Expected: "Docs checks passed".
- [ ] **Step 4: Full verification.** `go vet ./... && go test ./... -count=1` and `cd web && npm run check`. Expected: all pass.
- [ ] **Step 5: Commit** `Document the retirement of Gemini CLI`.

---

**Delivery.** One pull request, seven commits in this order. Tasks 1 to 3 leave the build green after each commit; Task 3 is the switch that makes `gemini` invalid, so it carries every test repair that follows from it.
