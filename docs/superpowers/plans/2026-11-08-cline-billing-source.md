# Cline billing source Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Cline chat can run on Usage-Billing, ClinePass or Free, chosen in the live chat and in Start an agent, with a saved default source.

**Architecture:** Cline's ACP session already reports a `provider` option. The connector stops dropping it and switches it with `session/set_config_option`, confirmed by read-back, exactly as it does the model. A new negotiated feature `acp-provider-v1` carries the setting and a create-time `provider` field. The account profile stores `defaultClineSource`. One pure web module owns labels and default rules; the live chat, Start dialog and Settings › Agents use it.

**Tech Stack:** Go (connector, controller, SQLite store), Svelte 5 + TypeScript (`web/`), plain Node Playwright scripts, a Python probe.

**Spec:** `docs/superpowers/specs/2026-11-08-cline-billing-source-design.md`

## Global Constraints

- Names: feature `acp-provider-v1`; capability JSON `acpProvider`; setting category `provider`; create field `provider`; profile field `defaultClineSource`.
- Provider values pass the existing model-name rule (`ValidModel`); at most `MaxACPThoughtLevelValues` (16) values; one `provider` setting per snapshot; the controls option limit rises from two to three.
- A switch is confirmed only when the returned `configOptions` read back that value, and is refused while a turn is running.
- Launch order is provider, then model, then effort, each confirmed before the opening prompt. A refused or unconfirmed provider refuses the launch.
- `session/load` restores what Cline stored; reet never re-applies a source on reattach, and the source is not stored on the session record.
- Precedence: explicit choice, then saved default source, then Cline's own. The default model prefills only while the chosen source equals the saved default source, or when none is saved.
- Labels: `cline` is Usage-Billing, `cline-pass` is ClinePass; any other id is shown as Cline reports it. Free joins the label table only when Task 1's probe shows its id. Wording says "Source", never "for this chat only".
- Credit hint: "This source has no credit. Switch to ClinePass or Free above, or add credits with Cline, then send a message to continue." It names only the other sources the session offers.
- Not in this pass: other agents, a per-project default, automatic fallback, reading Cline's balance, Home's launch bar.
- No new dependency. British spelling in copy. Before each commit run `go vet ./...`, `go test -race ./...` and `cd web && npm run check` for the parts touched.
- The third-party project the owner named as a reference is never named in commits, PR text, code comments or docs.
- Every commit message ends with these two lines, after a blank line: `Co-Authored-By: Claude <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_011MNNznG9VgYznXqLkTHZYo`.

## Review Focus

- Cline reports `provider` with one value (today's fake does): no Source control, and a launch is not blocked. Pinned in Task 7 (`sourceChoices`) and Task 8 (e2e).
- A source switch while a turn runs: refused with `ErrBusy`, control disabled. Pinned in Tasks 3 and 8.
- A saved default source reet has no label for: Start shows it as given and sends it, never claiming the machine lacks it (the review changed this from a "not available" note). Pinned in Tasks 7 and 9.
- An old connector or controller without `acp-provider-v1`: the setting is stripped, `Provider` is never sent, the control is hidden. Pinned in Tasks 4, 5 and 9.
- A switch reply with no model option: the previous model values stay and the current model is forgotten. Pinned in Task 3.

---

### Task 1: Probe script for the three unverified facts

**Files:**
- Create: `scripts/probe-cline-provider.py` (follow `scripts/probe-agent-options.py`: argparse, JSON-RPC over stdio, no model call)

**Interfaces:**
- Produces: a JSON report on stdout with keys `providerValues` (list), `switches` (list of `{value, ok, code, readBack, modelCount}`), `persistedAfterRestart` (bool: a fresh `session/new` in the same private copy reports the switched provider), `changedFiles` (paths under the private `CLINE_DIR` copy that changed), `freeLike` (model or provider ids containing "free"). Task 11 records these.

- [ ] **Step 1: Write the script.** It takes `--cline` (binary, default `cline`) and `--cline-dir` (default `~/.cline`), copies that folder to a temp directory and runs `cline --acp --auto-approve false` against the copy with `CLINE_DIR` set, so the real sign-in is never changed. It calls `initialize`, `session/new`, reads the `provider` option's values, sets each in turn with `session/set_config_option {configId:"provider"}`, records the read-back and model count, then starts a second process on the same copy and reads the provider again. It prints the report and deletes the copy.
- [ ] **Step 2: Verify it parses and explains itself.** Run `python3 -m py_compile scripts/probe-cline-provider.py && python3 scripts/probe-cline-provider.py --help`. Expected: exit 0, usage text naming `--cline` and `--cline-dir`.
- [ ] **Step 3: Commit** `Add a probe for Cline's provider option`.

The owner runs it on a signed-in machine; it cannot run here.

### Task 2: Protocol — feature, setting category, capability, create field

**Files:**
- Modify: `internal/terminalproto/acp_controls.go`, `internal/terminalproto/protocol.go`, `internal/terminalproto/launch.go`, `internal/terminalproto/agents.go`
- Test: `internal/terminalproto/acp_controls_test.go`, `internal/terminalproto/launch_test.go`

**Interfaces:**
- Produces: `const ACPProviderFeature = "acp-provider-v1"`; `func SupportsACPProvider(features string) bool`; `Capabilities.ACPProvider bool` (`json:"acpProvider,omitempty"`); `Create.Provider string` (`json:"provider,omitempty"`); `func ValidProvider(provider string) bool` (empty or `ValidModel`); `func ToolTakesProvider(id string) bool` (true for `cline` only). `ACPSetting.Category` may be `"provider"`.

- [ ] **Step 1: Write failing tests.** In `acp_controls_test.go`: `TestProviderSettingFitsTheControlsSnapshot` (model + thought_level + provider is valid; a fourth option, a second provider setting, 17 provider values, a provider value failing `ValidModel` are each invalid; the existing two-option snapshots stay valid) and `TestProviderCapabilityNeedsItsFeatureAndLiveControls` (`ACPProvider` with `ACPLiveControls` and the feature: `ValidateFor` is nil; missing either: `ErrInvalid`). In `launch_test.go`: `TestCreateProviderIsAClineChatOnly` (valid for `{Tool:"cline", Structured:true, Provider:"cline-pass"}`; invalid for `codex`, for a non-structured Cline create, for `"bad value"`; empty is valid everywhere).
- [ ] **Step 2: Run them.** `go test ./internal/terminalproto/ -run 'Provider' -count=1`. Expected: FAIL (undefined names).
- [ ] **Step 3: Implement** the constants, `Supports…`, the `Valid` change (limit 3, accept category `provider` with `MaxACPThoughtLevelValues` values), `Capabilities.Validate`/`ValidateFor` (feature appended when set; invalid without feature or without `ACPLiveControls`), `Create.Provider`, `ValidProvider`, `ToolTakesProvider`, and `validLaunch` (a provider needs `Tool=="cline"`, `Structured`, and `validModel`; the tool-less branch requires it empty).
- [ ] **Step 4: Run** `go test ./internal/terminalproto/ -count=1`. Expected: PASS.
- [ ] **Step 5: Commit** `Add the acp-provider-v1 protocol pieces`.

### Task 3: Connector session — read, switch and launch with a provider

**Files:**
- Modify: `internal/acpagent/controls.go` (`readSettings`, `SetOption`), `internal/acpagent/options.go`, `internal/acpagent/spec.go`, `internal/acpagent/session.go` (`Start`)
- Test: `internal/acpagent/session_test.go` (fake), `internal/acpagent/cline_test.go`

**Interfaces:**
- Consumes: `terminalproto.ACPSetting` category `provider` (Task 2).
- Produces: `Options.Provider string`; `ErrProvider` and `ErrProviderUnconfirmed` (wraps `ErrProvider`) in `spec.go`; `Controls().Options` includes `{ID:"provider", Category:"provider", Current, Values}` for Cline.

- [ ] **Step 1: Extend the fake Cline in `session_test.go`.** Its `provider` option offers `cline` and `cline-pass`, current `cline`. A provider switch sets the provider, replaces the model catalogue (`cline` → `stub/model`, `provider/other`; `cline-pass` → `cline-pass/deepseek-v4`, `cline-pass/other`), sets the current model to the first of the new list, and replies with the whole option list. The model switch refuses (`-32602`) a value not in the current catalogue. New scenarios: `cline-provider-refused` (provider set answers `-32602`), `cline-provider-unconfirmed` (reply keeps the old provider), `cline-provider-no-models` (reply omits the model option).
- [ ] **Step 2: Write failing tests in `cline_test.go`.** Update `TestClineStartsWithApprovalsOnAndItsModelCatalogue` to find options by category (now a provider and a model setting). New: `TestClineAdmitsItsProviderOption` (setting `{provider, provider, "cline", [cline cline-pass]}`); `TestClineAppliesAChosenProviderBeforeTheModel` (`Provider:"cline-pass"`, `Model:"cline-pass/deepseek-v4"` → `Model()` is that and provider current is `cline-pass`); `TestClineProviderSwitchSwapsTheModelCatalogue` (after `SetOption(ctx,"provider","cline-pass")` the model values are the `cline-pass/` list); `TestClineProviderSwitchWithNoModelOptionForgetsTheModel` (`cline-provider-no-models`: old model values kept, `Model()` empty); `TestClineRefusedOrUnconfirmedProviderRefusesTheChat` (both scenarios → `errors.Is(err, ErrProvider)`, no session); `TestClineProviderSwitchWhileWorkingIsBusy` (start a turn the way `TestClinePermissionCardsAreReviewable` does, then `SetOption` on `provider` → `ErrBusy`); `TestClineSwitchRejectsAnUnlistedProvider` (`ErrControl`); `TestClineReattachDoesNotReapplyAProvider` (`cline-provider-refused` with `Resume` set and `Provider` set starts fine).
- [ ] **Step 3: Run** `go test ./internal/acpagent/ -run Cline -count=1`. Expected: FAIL.
- [ ] **Step 4: Implement.** `readSettings` admits category `_provider` (or id `provider` with no category) as category `provider`, ids only, capped at `MaxACPThoughtLevelValues`. `SetOption`: capture `s.working || s.nativeBusy` inside the first `turnMu` section; for a `provider` setting return `ErrBusy` when busy and `ErrControl` when the value is not in `setting.Values`. `applySettingReply`: when the reply parses but has no `model` option, keep the old model values and clear `s.model`. `Start`: after `updateSettings(raw)` and before the model block, when `opts.Provider != ""` and `opts.Resume == ""`, find the `provider` setting (none: close, `ErrProvider`), skip if already current, else `SetOption`; any error or a different read-back closes the session and returns `ErrProvider`/`ErrProviderUnconfirmed`.
- [ ] **Step 5: Run** `go test ./internal/acpagent/ -count=1`. Expected: PASS (fix any other test that counted Cline's options).
- [ ] **Step 6: Commit** `Switch Cline's provider like its model`.

### Task 4: Connector wiring — negotiation, create gating, stripping

**Files:**
- Modify: `internal/agenthealth/agent.go`, `internal/api/private_control.go`, `internal/terminalagent/agent.go`, `internal/terminalagent/chat.go`, `internal/terminalagent/chat_controls.go`
- Test: `internal/agenthealth/acp_agents_test.go`, `internal/terminalagent/chat_catalogue_test.go`, `internal/terminalagent/code_test.go`

**Interfaces:**
- Consumes: `SupportsACPProvider`, `Capabilities.ACPProvider`, `Create.Provider`, `Options.Provider`, `ErrProvider`.
- Produces: capability `ACPProvider` reported when negotiated and live controls are on; chat links carry `provider bool`; `forConnection` strips `provider` settings for a peer without it.

- [ ] **Step 1: Write failing tests.** `acp_agents_test.go`: `TestACPProviderCapabilityNeedsLiveControlsAndItsFeature`, modelled on `TestStartupOptionsDoNotLeakToLiveControlsOnlyPeer`. `chat_catalogue_test.go`: `TestProviderSettingIsStrippedForAPeerWithoutTheFeature`. `code_test.go`: `TestCreateWithProviderNeedsTheNegotiatedFeature` (`LaunchOptionUnsupported` without it).
- [ ] **Step 2: Run them.** `go test ./internal/agenthealth/ ./internal/terminalagent/ -run Provider -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement.** `agenthealth`: `cfg.acpProvider = SupportsACPProvider(advertised)`, append the feature in the features builder, `ACPProvider: acp && cfg.acpLiveControls && cfg.acpProvider` in `capabilitiesFor`. `private_control.go`: add `ACPProviderFeature` to the advertised list. `terminalagent/agent.go`: refuse a create with `Provider` when the feature is not negotiated (as `needsStartOptions` is), and pass `SupportsACPProvider(...)` as the fourth `attachChats` flag. `chat.go`: `chatLink.provider` from `flags[3]`, `Provider: in.Provider` into the `acpagent.Options`, and a result for `ErrProvider`: "The agent did not use the chosen source, so the chat was not started." `forConnection` in `chat_controls.go` drops `provider` settings when `!h.link.provider`.
- [ ] **Step 4: Run** `go test ./internal/agenthealth/ ./internal/terminalagent/ ./internal/api/ -count=1`. Expected: PASS.
- [ ] **Step 5: Commit** `Negotiate acp-provider-v1 between connector and controller`.

### Task 5: Controller — gate and pass the provider through create

**Files:**
- Modify: `internal/terminalbroker/broker.go`, `internal/api/terminal.go`
- Test: `internal/terminalbroker/chat_controls_test.go`, `internal/api/terminal_chat_test.go`

**Interfaces:**
- Consumes: `Capabilities.ACPProvider`, `Create.Provider`, `ToolTakesProvider`.
- Produces: `POST /api/v2/terminal/sessions` accepts `provider` (string) and sends it in `Create`; its audit detail gains `provider`.

- [ ] **Step 1: Write failing tests.** `chat_controls_test.go`: `TestProviderRequiresTheNegotiatedCapability` (`ErrLaunchOptionUnsupported` when `ACPProvider` is false; accepted when true; refused for a non-structured create or a non-Cline tool). `terminal_chat_test.go`: `TestTerminalCreateSendsAClineProvider` (the `Create` the fake machine receives carries `Provider:"cline-pass"`; the audit detail has `provider`).
- [ ] **Step 2: Run** `go test ./internal/terminalbroker/ ./internal/api/ -run Provider -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement.** `launchRequested` counts `in.Provider != ""`; in the capability block refuse with `ErrLaunchOptionUnsupported` unless `caps.Capabilities.ACPProvider && in.Structured && ToolTakesProvider(in.Tool)`. `terminalCreate` reads `Provider string \`json:"provider"\``, sets it on `Create`, adds it to the audit map. Reattach (`chat_reattach.go`) is not changed.
- [ ] **Step 4: Run** `go test ./internal/terminalbroker/ ./internal/api/ -count=1`. Expected: PASS.
- [ ] **Step 5: Commit** `Pass a Cline provider through terminal create`.

### Task 6: Account default source

**Files:**
- Modify: `internal/store/store_backups.go` (`Profile`, column list, `ProfileFor`, `SaveProfileFor`), `internal/api/profile.go` (`profileSave`)
- Test: `internal/store/profile_defaults_test.go`, `internal/api/agents_home_defaults_test.go`

**Interfaces:**
- Produces: `Profile.DefaultClineSource string` (`json:"defaultClineSource"`); `PATCH /api/v2/profile` accepts `defaultClineSource` (blank or `ValidProvider`), audited by value, refused 403 for a viewer, 400 `defaultClineSource must be blank or a source id reet can pass on` when invalid.

- [ ] **Step 1: Write failing tests.** Store: `TestDefaultClineSourceMigratesAndSurvivesReopen` (modelled on `TestShelfDefaultsMigrateAndSurviveReopen`) and a backup round trip in the existing backup test that already covers `defaultEffort` (find it with `grep -ln defaultEffort internal/store/*_test.go`). API: `TestProfileDefaultClineSource` (saves and reads back; blank clears; an invalid id is 400 and changes nothing; a viewer is 403; the audit row names the value).
- [ ] **Step 2: Run** `go test ./internal/store/ ./internal/api/ -run 'ClineSource' -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement** the column `default_cline_source TEXT NOT NULL DEFAULT ''` in the existing migration list, the scan and upsert, the `Profile` field, and the PATCH field with its checks.
- [ ] **Step 4: Run** `go test ./internal/store/ ./internal/api/ -count=1`. Expected: PASS.
- [ ] **Step 5: Commit** `Save a default Cline source on the profile`.

### Task 7: Web foundation — types, helper module, default rule

**Files:**
- Create: `web/src/lib/clineSource.ts`, `web/src/lib/clineSource.test.mjs`
- Modify: `web/src/lib/terminal/api.ts` (`Capabilities` gains `acpProvider?: boolean`; `ChatSetting.category` gains `'provider'`; `LaunchRequest` gains `provider?: string` and `createTerminal`'s key list gains `'provider'`), `web/src/lib/api.ts` (`Profile` and `saveProfile`'s input gain `defaultClineSource?: string`), `web/src/lib/models.ts`, `web/src/lib/models.test.mjs`, `web/package.json` (append `&& node src/lib/clineSource.test.mjs` to `test`)

**Interfaces:**
- Produces in `clineSource.ts`: `KNOWN_SOURCES: readonly {id:string; label:string}[]` (`cline`/Usage-Billing, `cline-pass`/ClinePass); `sourceLabel(id: string): string`; `sourceChoices(values: readonly string[]): {id:string; label:string}[]` (empty for fewer than two values); `resolveStartSource(input: {explicit?: string; saved?: string; offered: readonly string[]}): {source: string; note: string}`; `creditGuidance(input: {acpModel: boolean; offered: readonly string[]; current: string}): string`.
- Produces in `models.ts`: `resolveLaunchDefaults` takes `chosenSource?: string`; `LaunchDefaultSource` gains `defaultClineSource?: string`.

- [ ] **Step 1: Write failing tests** (style of `models.test.mjs`: compile with `ts.transpileModule`, import by data URL). `clineSource.test.mjs`: `sourceLabel('cline')` is `Usage-Billing`, `'cline-pass'` is `ClinePass`, `'x-other'` is `x-other`; `sourceChoices(['cline'])` is `[]` and two values give two labelled entries; `resolveStartSource` returns the explicit value, else the saved one when offered, else `{source:'', note:'Your saved source, gone, isn’t available on this machine, so Cline’s own choice is used.'}` for a saved id not offered, and `{source:'', note:''}` with nothing saved; `creditGuidance` with `offered:['cline','cline-pass'], current:'cline'` is "This source has no credit. Switch to ClinePass above, or add credits with Cline, then send a message to continue.", with `offered:['cline','cline-pass','x-other']` it says "Switch to ClinePass or x-other above", and with only the current source offered it returns the existing text "This model has reached its usage or credit limit. Choose another model below, or add credits with your provider, then send a message to continue." (`acpModel` true) or its Add-credits variant (false). `models.test.mjs`: with `account={defaultTool:'cline', defaultClineSource:'cline-pass'}` and a favourite default `m/fav`, `chosenSource:'cline'` gives model `''`, `'cline-pass'` gives `m/fav`, no saved source gives `m/fav`, and an undefined `chosenSource` gives `m/fav`.
- [ ] **Step 2: Run** `cd web && node src/lib/clineSource.test.mjs && node src/lib/models.test.mjs`. Expected: FAIL.
- [ ] **Step 3: Implement** the module and the type changes. `creditGuidance` names the other offered sources by `sourceLabel`, joined with " or ".
- [ ] **Step 4: Run** `cd web && npm run check`. Expected: PASS.
- [ ] **Step 5: Commit** `Add the Cline source helpers and types`.

### Task 8: Live chat Source control and credit hint

**Files:**
- Modify: `web/src/lib/terminal/ChatPane.svelte` (near `liveEffort` at line 70, the toolbar at lines 973-993, `guidance` at lines 146-160, and `onMount` reads)
- Create: `web/tests/chat-cline-source.e2e.mjs` (copy the structure of `chat-controls.e2e.mjs`; Cline machine with `acp`, `acpModel`, `acpLiveControls`, `acpProvider`)
- Modify: `.github/workflows/terminal-browser.yml` (run it after `chat-cline.e2e.mjs`)

**Interfaces:**
- Consumes: `sourceChoices`, `sourceLabel`, `creditGuidance`, `saveProfile`, `getProfile`, `changeSetting`.

- [ ] **Step 1: Write the failing e2e.** Cases: provider values `[cline, cline-pass]` show a **Source** select reading Usage-Billing, and choosing ClinePass POSTs `{action:'set', configId:'provider', value:'cline-pass'}` to `/chat/controls`; one value shows no Source control; `working:true` disables it; the star button **Make default** sends `PATCH /api/v2/profile` with `{defaultClineSource:'cline-pass'}` and then reads **Default**, clicking again sends `''`; a status error `Insufficient balance` shows the credit hint naming ClinePass.
- [ ] **Step 2: Run it** (build and preview as in `terminal-browser.yml`; `node web/tests/chat-cline-source.e2e.mjs`). Expected: FAIL (no Source control).
- [ ] **Step 3: Implement.** `liveProvider = $derived(runtime?.options.find(o => o.category === 'provider'))`; render a `.source-controls` block after `.model-controls` when `sourceChoices(liveProvider.values).length > 0`: label **Source**, a `<select>` of those choices, disabled when `changingModel || runtime?.configuring || working || chat.connection !== 'open'`, `onchange` calls `changeSetting(liveProvider.id, value)`; a star like the model's, backed by the saved `defaultClineSource` read once with `getProfile()`. Replace the credit branch of `guidance` with `creditGuidance({acpModel, offered: liveProvider?.values ?? [], current: liveProvider?.current ?? ''})`. Reuse the `.star` and `.effort-controls` styles.
- [ ] **Step 4: Run** the e2e, then `cd web && npm run check`, then `node web/tests/chat-controls.e2e.mjs` and `node web/tests/chat-cline.e2e.mjs`. Expected: PASS.
- [ ] **Step 5: Commit** `Add a Source control to the Cline chat`.

### Task 9: Start an agent — source when Cline is chosen

**Files:**
- Modify: `web/src/lib/StartAgentDialog.svelte` (account defaults at line 89 and `loadAccount` at 459-466, defaults at 105-146, the Effort block at 749-758, `start()` at 501-530)
- Test: `web/tests/start-agent.e2e.mjs` (Cline cases near lines 604-700 are the template; the machine needs `acp`, `acpLiveControls`, `acpStartOptions`, `acpProvider` and a Cline `acpAgents` entry)

**Interfaces:**
- Consumes: `KNOWN_SOURCES`, `resolveStartSource`, `resolveLaunchDefaults({chosenSource})`, `saveProfile`, `LaunchRequest.provider`.

- [ ] **Step 1: Write failing e2e cases.** The **Source** select appears for a Cline chat when the machine has `acpProvider` and not otherwise; it is preselected from `profileDefaults={defaultClineSource:'cline-pass'}`; choosing ClinePass makes the create POST include `provider:'cline-pass'`; choosing "Cline’s own" sends no `provider`; with the saved default `gone` the note is shown and Start still posts without `provider`; with a saved default of `cline-pass` and a favourite default model, choosing Usage-Billing leaves the model on Agent default; a refusal whose detail is "The agent did not use the chosen source, so the chat was not started." is shown in the dialog's refusal text; the star sets the default via `PATCH /api/v2/profile`.
- [ ] **Step 2: Run** `node web/tests/start-agent.e2e.mjs`. Expected: FAIL.
- [ ] **Step 3: Implement.** State `source` and `explicitSource` (like `effort`/`explicitEffort`); offered ids come from `KNOWN_SOURCES`; show the field only when `chosenTool === 'cline' && structured && machine?.capabilities?.acpProvider`; initial value from `resolveStartSource({explicit, saved: accountDefaults.defaultClineSource, offered})`, whose `note` renders under the field; pass `chosenSource: source` into `resolveLaunchDefaults`; add `launch.provider = source` in `start()` only when shown and non-empty; add `defaultClineSource` to the `accountDefaults` type and `loadAccount`.
- [ ] **Step 4: Run** `node web/tests/start-agent.e2e.mjs`, `node web/tests/chat-cline.e2e.mjs`, `node web/tests/launch.e2e.mjs`, and `cd web && npm run check`. Expected: PASS.
- [ ] **Step 5: Commit** `Choose a Cline source in Start an agent`.

### Task 10: Settings › Agents default source

**Files:**
- Modify: `web/src/lib/AgentsSettings.svelte` (state at lines 25-26, `fillProfile` 94-102, `save` 366-398, the form at 415-460)
- Test: `web/tests/account.e2e.mjs`

**Interfaces:**
- Consumes: `KNOWN_SOURCES`, `saveProfile`.

- [ ] **Step 1: Write the failing e2e.** With **Default agent** set to Cline a **Default source** select (Cline’s own, Usage-Billing, ClinePass) appears; choosing ClinePass and saving sends `PATCH /api/v2/profile` with `{defaultClineSource:'cline-pass'}`; for another agent the select is absent and a stale value is not sent.
- [ ] **Step 2: Run** `node web/tests/account.e2e.mjs`. Expected: FAIL.
- [ ] **Step 3: Implement** `defaultClineSource` state, `loaded.source`, the diff in `save`, and the select after the Thinking block, shown when `defaultTool === 'cline'`.
- [ ] **Step 4: Run** `node web/tests/account.e2e.mjs` and `cd web && npm run check`. Expected: PASS.
- [ ] **Step 5: Commit** `Set a default Cline source in Settings`.

### Task 11: Docs and probe results

**Files:**
- Modify: `docs/agent-models.md`, `docs/start-agent-session.md`, `docs/structured-sessions.md`, `docs/research/cline-acp-probe.md`

- [ ] **Step 1: Write the docs.** `agent-models.md`: the saved default source, its precedence, and how the default model follows it. `start-agent-session.md`: the **Source** row for Cline and its refusal. `structured-sessions.md`: the live chat **Source** control and the credit hint. `cline-acp-probe.md`: a section recording the probe's output if the owner has run it (provider settable, how Free is exposed, whether a switch persists machine-wide); if not run, say plainly that all three are unchecked. If Free is its own provider id, add it to `KNOWN_SOURCES` in `web/src/lib/clineSource.ts` with its test; if it is a model filter, say so and leave the table.
- [ ] **Step 2: Run** `python3 scripts/check-docs.py`. Expected: exit 0.
- [ ] **Step 3: Commit** `Document the Cline billing source`.

---

**Delivery.** Tasks 1-5 are group A (connector and protocol), Task 6 is B, Tasks 7-8 are C, Tasks 9-10 are D, Task 11 is E. Each group leaves the build green, so the branch can be split into separate PRs in that order without rework.
