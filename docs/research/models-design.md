# Agent models: loaded from the machine, defaults and favourites — design

Date: 28 Oct 2026. Read-only investigation of the tree at `f0bffe5` (PR H / #231 branch, H1 committed).
Owner request: "Agent models need to dynamically load those available, plus allow the user to set a
default per project, for non-project new sessions, and favourites. All locations on reet.sh."

Evidence labels used below: **[reet]** verified in reet's own evidence (code, `testdata/help`
captures, reet docs); **[docs]** stated in the agent's public docs that reet already cites, but never
captured or run by reet; **[memory]** my recollection, unverified, must be captured before use.
Nothing third-party was run for this report.

---

## 0. Summary

- Today a model is **free text with a static hint list** (web `catalogue.ts`), validated by one
  regex (`terminalproto.validModel`) in the browser, controller and connector. Nothing is ever read
  from a machine. Defaults exist only per **folder project** (`defaultTool` + `defaultModel`, no
  effort) and are applied only when the project also names a default agent. The account profile has
  launch defaults for **base branch and worktrees only**, and no UI edits them. There are no
  favourites anywhere.
- Of the five enabled agents only **OpenCode** has a plausible safe listing command
  (`opencode models`, [docs]). Codex has a JSON-RPC `model/list` in `app-server` ([memory]); Claude
  Code, Gemini CLI and Cline 3.0.74 have no listing command in the evidence (Cline's is
  [reet]-verified absent). ACP `session/new` model lists ([memory], probe blocked) would create a
  real agent session just to read a list — not recommended for v1.
- So the design is: a new **`models-v1`** feature in which the connector runs a **connector-owned,
  per-preset listing argv** (OpenCode only at first; others report `unsupported`), with minimal env,
  timeout, size cap and line validation; the controller **caches per machine per agent** with
  `fetchedAt` and an honest `state`; the web merges *favourites → this machine's listed models
  (grouped by provider) → reet's curated suggestions → Other…*. Free text always works; a stale or
  missing list never blocks a launch.
- Defaults: **project** (`defaultTool/defaultModel/defaultEffort`), **account "new session without a
  project"** (new profile fields), optional **per-machine** (recommend deferring). Precedence:
  explicit choice > project default > (machine default) > account default > agent's own default.
- Favourites: per account, ordered `(tool, model)` pairs, starred from every model picker, managed
  in a new **Settings › Agents** section that also holds the account default.
- 11 small tasks (M1–M11) below; Go tasks can start now beside PR H, web integrations after H merges.

---

## 1. Map: every place a model is chosen, shown, stored or validated

### 1.1 Go — protocol and connector

| Location | What exists | Default today | What favourites/dynamic lists touch |
|---|---|---|---|
| `internal/terminalproto/protocol.go:258` `Create.Model` | plain string, `omitempty` | — | unchanged; still the only way a model reaches the agent |
| `internal/terminalproto/launch.go:10` `MaxModelBytes = 100`; `:33` `ValidModel`; `:50-63` `validModel` (first char alnum, then `A-Za-z0-9._:/@+-`); `:92` `validLaunch` (model only with a tool) | the single shared rule, used by connector, broker, API and store | — | listed names must pass the same rule or be dropped (see §3.5; Claude's `sonnet[1m]`-style aliases [memory] would fail) |
| `internal/terminalproto/agents.go:22-28` `agentOptions` (`model: true` for claude, codex, opencode, gemini, cline); `:40` `ToolTakesModel` | which agents take a model | — | add nothing; a `models` listing kind is per preset in the connector, mirrored by a new `ToolListsModels` only if the controller needs to know in advance (it doesn't: capability says) |
| `internal/terminalproto/protocol.go:123-143` `Capabilities`, `:153-185` `ValidateFor` | feature-gated booleans (`Browse`, `Branches`, `BrowseOff`…) | — | add `Models bool` / `ModelsOff bool`, valid only when `models-v1` negotiated (pattern of `branches.go:8-25`) |
| `internal/terminalproto/branches.go` (whole file) | request/result pair with closed reason enum, size cap | — | template for new `models.go` (`Models`, `ModelsResult`, `ModelsReason`) |
| `internal/terminalhost/catalogue.go:10-20` `Preset` (ID, Name, Binary, ModelFlag, …); `:32-38` rows | `ModelFlag: "--model"` for the five enabled agents; no model data | — | add `ModelList *ModelList` (argv, env additions, parser id) to the row; only OpenCode gets one at first |
| `internal/terminalhost/launch.go:89-93` | `argv = append(argv, p.modelFlag, in.Model)` (one argv element) | — | unchanged |
| `internal/terminalhost/service.go:230-262` `safeEnvironment()`; `manager_tmux.go:34-43` `environment()` | the sanitised env allowlist (PATH HOME USER LOGNAME SHELL TMPDIR LANG LC_* TERM) | — | listing reuses the allowlist, minus TERM/SHELL, plus fixed opt-out variables |
| `internal/terminalhost/manager_tmux.go:45-60` `cappedBuffer` + 5 s timeout | output-capped command runner | — | reuse the capped buffer for the listing |
| `internal/terminalhost/catalogue_test.go`, `launch_test.go` | pin argv and keep `agentOptions` equal to the catalogue | — | add: listing argv pinned; disabled presets never list |
| `internal/terminalhost/testdata/help/{codex,cline,pi}.txt`, `sources.json`, `README.md` | captured help; Codex 0.163.0 has no `models` command; Cline 3.0.74 has none; Pi has `--list-models` | — | new capture needed: `opencode --version`, `opencode models --help` (and `opencode models` against an empty HOME) before M2 ships |
| `internal/terminalagent/agent.go:216-253` | dispatch for `Browse`/`Branches` gated on the feature header, run "aside" in a slot with a conflict answer | — | add `*terminalproto.Models` case the same way (own slot, own conflict/failed answers) |
| `internal/agenthealth/agent.go:42-45, 646-665` `terminalFeatures`, `:331-345` `gateMachineRequests`, local switches `BrowseAllowed` | feature list and the local browse switch | — | add `models-v1` to the list; optional local switch `reet terminal models on|off` (same pattern as browse) |
| `internal/terminalbroker/broker.go:498, 546` | refuses model for a tool that takes none | — | unchanged |
| `internal/terminalbroker/browse.go:148-170` `Broker.Branches` | controller→machine request with capability check | — | add `Broker.Models` beside it |
| `internal/projectrun/run.go:532` | old-Projects run member `Create{Model: m.Model}` | member's model | unchanged (old flow), but see store_projects below |
| `internal/setup/summaries.go:5-21` | setup discovery reads agent settings JSON and records only **whether** `model`/`defaultModel` exists (`ModelConfigured`) | — | optional later: report the configured default model **value** (Claude `settings.json` `model`, OpenCode `opencode.json` `model`, Gemini `settings.json` model) to show "agent's own default: X". Not in v1 (privacy review needed: it's config, not a private store, but it widens what discovery sends) |

### 1.2 Go — controller API and store

| Location | What exists | Default stored | Favourites / defaults touch |
|---|---|---|---|
| `internal/api/terminal.go:199` `Model` in create body; `:214` copied into `Create`; `:313` audit detail `model` for `terminal-launch` | free text, validated by the proto decoder | — | unchanged; audit gains `modelSource` (`chosen`/`project`/`account`) only if we want it (optional) |
| `internal/api/terminal_browse.go:25, 241-270` `GET /api/v2/machines/{agentId}/branches` with rate limit `browseLimits` | live, uncached machine read | — | template for `GET/POST /api/v2/machines/{agentId}/models` |
| `internal/api/profile.go:46-57` `profileSave` input (`defaultBaseBranch`, `worktreesByDefault`); `:81-85`, `:158-164` validation and audit detail | — | **account launch defaults: base branch, worktrees only. No tool/model/effort.** | add `defaultTool`, `defaultModel`, `defaultEffort` (validated with `ValidToolV2`, `ValidModel`, `ToolTakesEffort`) |
| `internal/store/store_backups.go:100-103` `Profile.DefaultBaseBranch`; `:165-195` `user_profiles` + column migration list; `:464-488` select/upsert | profile row | base branch, worktrees | three new columns; same migration list; backup/restore carries them because the row is backed up whole (verify in M4 test) |
| `internal/api/folder_projects.go:112-125` `folderProjectInput.DefaultTool/DefaultModel`; `:149` `set`; `:276-281` validation; `:313, 327` view | free text | **per project: tool + model, no effort** | add `defaultEffort`; **fix existing gap:** `:276` and `store_folder_projects.go:215` use `ValidTool` (original four), so Cline cannot be a project default |
| `internal/store/store_folder_projects.go:73` struct; `:98` DDL `default_tool, default_model`; `:215-220` validation; `:304-414` columns/insert/update | — | tool, model | add `default_effort` column |
| `internal/api/projects.go:125` `sentenceModel`; `:180, 196, 208, 221, 324` old saved-Project and run views | members and Project defaults carry model | Project `defaultModel`, member `model` | display only; no favourites (old flow, migrated) |
| `internal/api/project_runs.go:162-221` `POST /api/v2/folder-projects/runs` (start in several projects) | `model` validated (`:208`), refused for a tool without model (`:212`) | — | unchanged: the bar resolves defaults before sending; the server never substitutes (keeps "explicit request = what runs") |
| `internal/store/store_projects.go:127-128, 263, 391-394, 705-711` old Projects | `default_model` applied only when the member's tool equals the Project's default tool | Project default | none |
| `internal/store/store_projects_migrate.go:480-490` | migration resolves member/Project model into folder project `DefaultModel` | — | none |
| `internal/store/store_terminal.go:33-69, 304-316, 442-446` | `terminal_sessions.model` column, backfilled from the create operation | — | read by the bar's "last launch" (`launchBar.ts:103`) and by a "Recent" group (§4.5) |
| `internal/api/mcp_server.go` | no launch tool | — | none |

### 1.3 Web

| Location | What exists | Default | Favourites / dynamic touch |
|---|---|---|---|
| `web/src/lib/catalogue.ts:3-13` `CATALOGUE[].models` | static hints: claude `sonnet, opus, haiku`; codex `gpt-5-codex`; gemini `gemini-2.5-pro, gemini-2.5-flash`; opencode/cline `[]` | — | becomes "Suggested" group (fallback); stays display-only |
| `web/src/lib/agentLaunch.ts:17` `MODEL` regex (mirror of Go); `:38-41` `launchActivity` ("Started X with M"); `:44` `modelSuggestions`; `:48-53` `cleanModel`/`validateModel`; `:223-226` `rekey` (machine/agent change resets model) | — | — | `modelSuggestions` replaced by `modelOptions(tool, machineList, favourites, recent)`; `rekey` should reset to the **resolved default**, not to '' |
| `web/src/lib/launchBar.ts:11-14` `LaunchChoice.model`; `:28-29` `LaunchDefaults` (profile base/worktrees); `:72-93` `initialChoice` (last launch first, else first project's `defaultTool/defaultModel`, else `CATALOGUE[0]`); `:103-115` `lastChoice` (newest session's model); `:118-125` `withProject` (project model only if project has `defaultTool`); `:192` sentence "with [model]" (`default`); `:218` validate; `:261-262` send; `:287` selection-run body; `:299` refusal copy | — | project default; last launch | resolution function `resolveLaunchDefaults` (precedence §3.2); `LaunchDefaults` gains account tool/model/effort |
| `web/src/lib/launch/LaunchBar.svelte:53` labels; `:174-175` tablet hides model slot; `:202-206` model options = `default` + hints + own; `:233` value; `:244` agent change clears model; `:245` set; `:666` `SlotPicker other={label:'Other…', validate: validateModel}` | — | — | model slot uses the new `ModelPicker` options + star; reads machine list via API; agent change applies resolved default |
| `web/src/lib/launch/SlotPicker.svelte:3, 10-30` options with `group`, `detail`, `disabled/reason`, `filter`, `other`, `status` | generic listbox/sheet | — | needs: optional per-option **star toggle** (secondary action), filter for the model slot, `status` line "Read from studio 3 h ago · Refresh" |
| `web/src/lib/StartAgentDialog.svelte:47` state; `:61-73` rekey; `:82-83` suggestions/`model` derived (a hint or Other); `:96` error; `:138` send; `:251-262` `<select>` + Other input + hint "Suggestions are hints…" | — | — (entry points pass `projectId` but not its default model) | replace `<select>` with the same picker (combobox) or extended `<select>` with optgroups; star button beside it; apply project/account default |
| Entry points of StartAgentDialog: `terminal/TerminalWorkspace.svelte:338`, `ProjectsBar.svelte:144`, `MachineFleet.svelte:446` (Machines page "Start agent") | — | — | inherit the dialog change |
| `web/src/lib/home/AddProjectDialog.svelte:6` imports `TOOLS` (original four); `:59` `defaultModel` state; `:206-236` save; `:243-247, 317, 330` field map, summary, validation; `:473-490` "More" fold: Default base branch, agent, **free-text "Default model" input (placeholder "Agent default")** | free text, no list, no effort | project default | model picker (machine of the project is known → its list); add **Default thinking** (effort) for agents that take it; Cline in agent list |
| `web/src/lib/home/Home.svelte:62, 344` reads profile launch defaults once | — | account base/worktrees | also read account tool/model/effort and favourites |
| `web/src/lib/terminal/api.ts:4` `TerminalSession.model`; `:25, 55` `LaunchRequest.model`; `:115` `SelectionRunBody.model`; `:146, 156` `FolderProject.defaultModel` / input | types | — | add `defaultEffort`; new `listModels`/`refreshModels`; favourites client |
| `web/src/lib/api.ts:865-867` `Profile` type (`defaultBaseBranch`, `worktreesByDefault`) | — | — | add `defaultTool/defaultModel/defaultEffort` |
| `web/src/lib/Account.svelte:314-360` Profile section (theme, timezone, prefix…) | **no launch-defaults UI exists today** (base branch/worktrees settable only by API) | — | new "Agents" section: new-session default + favourites manager (+ the existing base-branch/worktree defaults, which have no UI) |
| `web/src/lib/sessions.ts:35-38` search includes model; `:96-98` `rowMeta` "agent · model · branch" | display | — | optional star-state glyph? No — keep rows plain |
| `web/src/lib/sessions/SessionsPage.svelte:175, 285` | shows `rowMeta` | — | none (display) |
| `web/src/lib/terminal/SessionHoverCard.svelte` | **does not show the model** | — | optional: add "agent · model" line (cheap; H2 session header already plans `agent · model · started`) |
| `web/src/lib/home/InFlight.svelte:68` | agent name, no model | — | none |
| `web/src/lib/RunView.svelte:98` | `model || 'Agent default'` per run member | — | none |
| `web/src/lib/projects.ts:8-24, 109` old Project types/refusal codes | — | — | none |
| `web/src/lib/events.ts:39`, Activity page | `terminal-launch` detail `model` → "Started X with M in …" | — | new Activity lines: "Changed your default model", "Starred/unstarred M for X", "Read models on HOST" (only refreshes the person pressed) |
| `web/src/lib/MachineSettings.svelte:53-110` per-machine dialog (Updates, Sync, Access) | no agent list | — | new "Agents" section: installed agents, "Models read 3 h ago / could not read / not supported", Refresh |
| `web/src/lib/MachineFleet.svelte` machine cards | — | — | nothing on the card (keep it calm); link into MachineSettings |
| Planned (PR H2) `sessions/SessionScreen.svelte` header `agent · model · started` (spec `:357`) | — | — | show model; no picker |

### 1.4 Docs

`docs/terminal-launch.md:51` (model field rule), `:68-80` (argv table), `:103` (env), `:173` (audit),
`:200-206` "does not show that a model name exists" (must say: a listed model was listed by the
agent on that machine at time T, still not proof it works); `docs/start-agent-session.md:49, 58, 84,
94, 117, 121, 181-199, 209`; `docs/projects.md:6, 80, 124, 136, 166-168`;
`docs/terminal-workspace.md:41`; spec `docs/superpowers/specs/2026-10-26-agents-home-design.md:
413, 542, 698, 774-779` (web holds model hints); new `docs/agent-models.md` (protocol, cache,
security); `docs/roadmap.md`; `handoff.md`; `README.md` feature list; `docs/ui-verification.md`
matrix rows.

---

## 2. Where available models could come from, per agent

| Agent | Candidate source | Evidence | Verdict for v1 |
|---|---|---|---|
| **OpenCode** | `opencode models [provider]` subcommand, one `provider/model` per line; `--refresh` refetches the models.dev catalogue; `--verbose` adds metadata | [docs] opencode.ai/docs/cli (the page reet already cites for `--model`/`--prompt`); not captured in `testdata/help` | **Use**, after capture. Run without `--refresh` and without `--verbose`. Lists only providers OpenCode can see (its own auth file, read by OpenCode itself, and env keys — which the minimal env drops, so env-only providers will be missing; say so). Network: may fetch models.dev; `OPENCODE_DISABLE_MODELS_FETCH=1` and `OPENCODE_DISABLE_AUTOUPDATE=1` exist per [memory] and the probe report marks them **unverified** — must be confirmed in the capture. |
| OpenCode | ACP `opencode acp` → `session/new` result `models.availableModels` / config options of category `model` | [memory]; aoe research cites aoe using `category:model` config options and `unstable_session_model` (`aoe-agents-research.md:88`); probe row 2 UNTESTED (blocked) | Not v1: creates a real session in OpenCode's store and starts the agent; revisit in Program 2 where an ACP session exists anyway (read its model list for free). |
| **Codex** | `codex app-server` JSON-RPC `model/list` | [memory]; 0.163.0 help shows `app-server` as "[experimental]" and the shared daemon caveat (`codex.txt:17-18`) [reet] | Not v1 (experimental, long-running, shared daemon). Capture `codex app-server --help` / protocol schema later. |
| Codex | a `debug` listing subcommand | `debug  Debugging tools` exists [reet]; contents unknown | Capture `codex debug --help` once; use only if it is a plain read-only list. |
| Codex | `~/.codex/config.toml` `model` (its configured default) | setup discovery parses Codex settings already [reet `setup/summaries.go`] | Later, as "agent's own default", not as the list. |
| **Claude Code** | none in 2.1.310 help (reet docs quote only `--model <model>`) [reet docs]; aliases `sonnet/opus/haiku` (+ `default`, `opusplan`, `sonnet[1m]`) [memory]; Anthropic `/v1/models` needs the user's key — **forbidden** | — | Curated aliases (web `catalogue.ts`) + favourites + Other…. `claude-agent-acp` session model list [memory] belongs to Program 2. |
| **Gemini CLI** | none; in-TUI `/model` only [memory]; `gemini --acp` session modes/models [memory, unstable] | — | Curated (`gemini-2.5-pro`, `gemini-2.5-flash`, maybe `-flash-lite` [memory]) + favourites + Other…. |
| **Cline** 3.0.74 | no models command (commands list `cline.txt:54-70`) [reet]; `--model <model-id>` "with the selected provider" [reet]; `cline config` "Show current configuration" [reet] — output unknown, may include provider settings/keys | — | **Do not run `cline config`** (could print secrets). Free text + favourites only. |
| Pi (disabled) | `--list-models [search]`, `--offline` [reet `pi.txt:47`, `:49`] | — | Note for when Pi is enabled: `pi --list-models --offline …` is the cleanest source of all. |
| Cursor (disabled) | unknown | — | none. |

Conclusion: in v1 only OpenCode returns a machine-read list; the rest are "curated on reet +
favourites + Other…", presented honestly ("reet's suggestions" vs "read from studio"). The protocol
is general so Codex/Pi/ACP sources plug in later as connector-side rows without controller changes.

---

## 3. Design

### 3.1 Data model

**Per machine, per agent model list (controller cache)** — new table `machine_agent_models`:

```
user_id INTEGER, agent_id TEXT (machine enrollment id), tool TEXT,
state TEXT  -- 'listed' | 'unsupported' | 'failed' | 'off'
reason TEXT -- closed enum when not listed: not_installed, unsupported, timed_out, too_large,
            --   exited, unreadable, denied, conflict
models TEXT -- JSON [{"id":"anthropic/claude-sonnet-4-5","provider":"anthropic"}], ≤ 500
more INTEGER, source TEXT ('opencode models'), agent_version TEXT (from --version, optional),
fetched_at INTEGER (last successful list), checked_at INTEGER (last attempt),
PRIMARY KEY(user_id, agent_id, tool)
```
A failed refresh updates `checked_at`/`reason` but **keeps** the last good `models` and
`fetched_at`, so the UI can say "Last read 2 days ago; reading again failed (timed out)". Rows go
when a machine is forgotten (hook into existing forget). Not backed up (cache), or backed up
harmlessly — choose not.

**Favourites (per account)** — new table `model_favourites(user_id, tool, model, position,
created_at, PRIMARY KEY(user_id, tool, model))`, max 50 per account, `model` passes `ValidModel`,
`tool` passes `ValidToolV2`. Optional flag `agent_default` (at most one per tool): "use this when I
switch to this agent" — see decision D3.

**Defaults**
- Account (new-session-without-a-project): `user_profiles.default_tool`, `default_model`,
  `default_effort` (all `''` = agent's own / reet's first agent).
- Project: existing `folder_projects.default_tool`, `default_model` + new `default_effort`.
- Machine (optional, D2): `machine_launch_defaults(user_id, agent_id, tool, model, effort)`.
  Recommendation: defer.

### 3.2 Precedence (resolved in the browser, `launchBar.ts` pure function)

```
model(agent, project?, machine?) =
  explicit choice in this bar/dialog (incl. Other…)
  > project.defaultModel   if project.defaultTool == agent (or project names no tool → see below)
  > machine default        if set for agent (optional)
  > account default        if account.defaultTool == agent
  > favourite marked "default for this agent" (D3)
  > ''  (agent's own default; nothing sent)
```
Agent itself: explicit > project.defaultTool > account.defaultTool > first available agent.
Effort follows the same chain, only for agents that take it.

Rules: a default model is only ever applied together with its agent (today's `withProject`
behaviour, keep it; and make the store refuse `defaultModel` without `defaultTool` going forward,
leaving existing rows alone). The server never substitutes a default into a request: what the
browser sends is what runs, and the Activity line says it. Changing agent in the bar re-resolves
the model from the chain instead of clearing to ''. The "last launch" restore (`initialChoice`) is
D1.

### 3.3 Protocol (`models-v1`)

```go
const ModelsFeature = "models-v1"
type Models struct { Tool string `json:"tool"` }                      // kind "models"
type ModelEntry struct { ID, Provider string }                        // ID passes validModel; Provider = prefix before first '/' or ""
type ModelsResult struct {                                            // kind "models_result"
    Tool    string       `json:"tool"`
    Models  []ModelEntry `json:"models,omitempty"`                   // ≤ 500, unique, sorted as the agent printed
    More    bool         `json:"more,omitempty"`
    Version string       `json:"version,omitempty"`                  // plain text ≤ 64, optional
    Reason  ModelsReason `json:"reason,omitempty"`                   // closed enum; with Reason nothing else
}
```
Capabilities gain `Models` (connector will answer) and `ModelsOff` (negotiated but locally off),
valid only on a `models-v1` connection — an old controller never sees them, an old connector is
never sent `models`. Reply ≤ 64 KiB JSON (like `MaxBrowseReplyBytes`).

### 3.4 Connector listing rules (`terminalhost/models.go`)

- Only presets with a `ModelList` row run anything; others answer `unsupported` without exec.
  Disabled presets (Pi, Cursor) never list. Binary resolved the same way as launch (`resolveBinary`).
- argv only, from the catalogue row: OpenCode `{"opencode","models"}` — no controller-supplied
  argument at all (no provider filter in v1; the web filters).
- Env: `safeEnvironment()` minus TERM/SHELL, plus `NO_COLOR=1`, `LANG=C.UTF-8`, `CI=1`, and the
  row's opt-outs (`OPENCODE_DISABLE_AUTOUPDATE=1`, `OPENCODE_DISABLE_MODELS_FETCH=1` once verified).
  No provider keys, no SSH_AUTH_SOCK, no tokens (the allowlist already excludes them).
- cwd: a fresh empty 0700 temp dir (so no project `opencode.json` is read and nothing lands in a
  repo); removed after. stdin `/dev/null`; own process group, killed on timeout.
- Timeout 10 s; stdout cap 64 KiB (overflow → `too_large`, nothing sent); stderr read into a 4 KiB
  buffer and **discarded** (never sent: it can hold paths, URLs, key fragments).
- Parse: split lines, trim, drop empty; reject the whole output if it has any byte < 0x20 other
  than `\n`/`\t`, or ESC; each line must pass `validModel` else skipped (count skipped, report
  nothing about their content); de-duplicate; cap 500 (`More`). Provider = text before first `/`.
- One listing at a time per machine ("aside" slot like branches; second → `conflict`); connector
  also enforces a minimum 60 s between runs per tool (answer from its own last result within that).
- Gated on terminal consent and an optional local switch (`reet terminal models off`) → `off`.
- Never reads agent files itself. Never writes anywhere but the temp dir.

### 3.5 Validation rules

- Free text always allowed (Other…), validated by the existing rule; a listed model is just a
  suggestion — the launch path does not look at the cache at all, so a stale/missing list cannot
  block a launch.
- Listed names failing `validModel` are dropped connector-side (so they can't be picked and then
  refused). Known risk: Claude's `[1m]` suffix aliases and any id > 100 bytes — not listable/typable
  today either (D4 decides whether to widen the rule; recommend not now).
- Favourites and defaults validated with the same rule plus tool membership; a favourite for an
  agent not installed on the current machine is shown disabled with "Not installed on studio".
- A favourite/default not in the machine's list is still offered (marked "not in studio's list")
  — lists are incomplete by design (env-only providers, curated agents).

### 3.6 API

```
GET  /api/v2/machines/{agentId}/models?tool=opencode
  → 200 {tool, state:"listed"|"unsupported"|"failed"|"off"|"offline"|"update",
         models:[{id,provider}], more, source, version, fetchedAt|null, checkedAt|null,
         reason, sentence}
     served from cache; if never read and the machine is online with Models capability, reads
     once synchronously (≤ 12 s) — or returns state "unread" and the page calls refresh (pick one
     in M3; recommend synchronous first read, cache afterwards).
POST /api/v2/machines/{agentId}/models/refresh {tool}   (same-origin, writer role)
  → 200 same shape; 429 models_rate_limited (per user+machine, e.g. 6/min); 409 conflict
GET  /api/v2/models?tool=opencode                       (Settings / no machine chosen)
  → {machines:[{agentId, host, state, fetchedAt, models:[…]}]}   cache only, never live
GET  /api/v2/profile/model-favourites      → {version, favourites:[{tool, model, agentDefault}]}
POST /api/v2/profile/model-favourites {tool, model}            → 201/200 idempotent (star)
DELETE /api/v2/profile/model-favourites?tool=&model=           → 204 idempotent (unstar)
PUT  /api/v2/profile/model-favourites/order {version, items:[{tool,model}]} → 200 | 409 stale
PATCH profile (existing profileSave) + defaultTool, defaultModel, defaultEffort
folder projects create/update + defaultEffort; defaultTool accepts ValidToolV2
```
Errors reuse `writeErrCode` codes `invalid_tool`, `invalid_model`, `invalid_effort`,
`favourites_full`. Audit: `profile-saved` detail already lists changed fields (add the three);
`model-favourite-added/removed` {tool, model}; `machine-models-read` {machine, tool, state,
count} only for person-pressed refreshes (not cache reads).

### 3.7 UI

**Shared `ModelPicker` options (pure `modelOptions()` in a new `web/src/lib/models.ts`)**, groups in
order: *Agent default* (sends nothing; shows "studio's OpenCode decides") → *Favourites* (this agent's
stars, in the account order) → *Recent* (last 3 distinct models used with this agent, from the
session list) → *On studio* grouped by provider (`anthropic`, `openai`, …) → *reet's suggestions*
(catalogue hints, deduped) → *Other…*. Each row: label, optional detail ("not in studio's list"),
and a **star toggle** (separate button with `aria-pressed`, label "Star claude-sonnet-4-5 for
OpenCode"). Search field appears when > 8 options; it filters locally; Enter on a typed value that
matches nothing offers "Use ‘xyz’" (= Other… prefilled). Status line: "Read from studio 3 h ago ·
Refresh" / "Couldn't read studio's models (timed out). Last read 2 days ago." / "studio is offline —
showing the last list from 2 days ago" / "Update reet on studio to read its models" / "reet can't
read Claude Code's models; showing suggestions."

- **Launch bar** model slot (`LaunchBar.svelte:202, 666`): `SlotPicker` gains star buttons and the
  filter for `model`; list read for the chosen project's machine (or Home-folder machine). Several
  projects on different machines: groups per machine are not shown; favourites + suggestions only,
  with "Models differ per machine" note.
- **Start an agent** dialog (`StartAgentDialog.svelte:251-262`): replace the `<select>` with the
  same listbox (combobox button + popover/sheet) so star and search exist there too; apply project
  / account defaults when opened with a `projectId`.
- **Add project / Project settings** (`AddProjectDialog.svelte:473-490`): Default agent (now incl.
  Cline), Default model (picker for the project's machine), Default thinking (only for agents that
  take effort). Hint: "Used when you start a session in this project. You can still change it."
- **Settings › Agents** (new section in `Account.svelte`): "New sessions without a project" —
  agent, model, thinking; plus the existing base branch / worktrees defaults (finally given a UI);
  "Favourite models" — list grouped by agent, drag-free reordering with Move up/Move down buttons
  (keyboard friendly), Remove, "Add a model" (agent + picker over all machines' cached lists +
  Other…), and per agent "Use as default when I pick this agent" (if D3 yes).
- **Machines** (`MachineSettings.svelte`): new "Agents" section listing installed agents with
  "Models: 42 read 3 h ago · Refresh" / "not supported — reet uses suggestions" / "Couldn't read:
  timed out (2 min ago)". Refresh disabled while offline with "studio is offline". Optional per-machine
  default (D2) would live here.
- **Display-only**: hover card (add model), Sessions rows (already), session screen header (H2),
  RunView (already), Activity lines.

**Offline machine**: cached list shown with its age, Refresh disabled, launch itself already refused
by readiness. **Old connector**: "Update reet on that machine to read its models"; suggestions still
offered. **Phones**: picker is the existing bottom sheet; star is a 44×44 px target at the row's
end; search field pinned at the sheet top, `font-size: 16px` (no zoom); groups as sticky headings;
the summary line keeps the model hidden (Change sheet shows it, as now). **Accessibility**: listbox
with options; the star is *not* inside the option (separate button in the row, reachable by Tab
within the sheet, or Shift+Enter / "s" shortcut announced in the hint); `aria-live=polite` status
for "Reading studio's models…" and results count; disabled options keep their reason text; star
state announced ("Starred"); no colour-only states.

---

## 4. All locations that must change (the owner's "all locations")

Go: `terminalproto/models.go` (new), `protocol.go` (Capabilities + validation), `agents.go` (no
change unless `ToolListsModels`), `terminalhost/catalogue.go`, new `terminalhost/models.go`,
`terminalhost/testdata/help/` (OpenCode capture + `sources.json`), `terminalagent/agent.go`
(dispatch), `agenthealth/agent.go` (feature + local switch + capability), CLI for the local switch
(`reet terminal models on|off`, where `browse` lives), `terminalbroker/browse.go` (or new
`models.go`), `api/terminal_browse.go` (or new `api/models.go`) + routes, `api/profile.go`,
`api/folder_projects.go` (effort, ValidToolV2), `store/store_backups.go` (profile columns),
`store/store_folder_projects.go` (effort column, ValidToolV2), new `store/store_models.go`
(cache + favourites), machine-forget cleanup.

Web: `catalogue.ts`, `agentLaunch.ts`, new `models.ts` (+ test), `launchBar.ts` (+ test),
`launch/LaunchBar.svelte`, `launch/SlotPicker.svelte`, `StartAgentDialog.svelte` (and its three
entry points inherit), `home/AddProjectDialog.svelte`, `home/Home.svelte`, `terminal/api.ts`,
`api.ts`, `Account.svelte` (Settings › Agents), `MachineSettings.svelte`, `events.ts` /
`ActivityPage.svelte`, `terminal/SessionHoverCard.svelte` (optional), H2 `SessionScreen` header,
e2e: `launch.e2e`, `start-agent` e2e, `home.e2e`, `account` e2e, `machines` e2e, phone/zoom
matrices, `.github/workflows/terminal-browser.yml` if new specs.

Docs: listed in §1.4 plus new `docs/agent-models.md`.

---

## 5. Tasks (small, independently reviewable)

Branch/PR: own PR **"M — agent models"** after PR H merges; M1–M4 may be developed in parallel
with H (no shared files with H's web work). The copy/paste PR also adds a feature token in
`agenthealth.terminalFeatures` and a dispatch case in `terminalagent/agent.go`: whichever lands
second rebases a few lines; no design conflict. Recommend: copy/paste PR protocol first (owner
already ordered it after H1), then M.

| # | Task | Files | Tests |
|---|---|---|---|
| M0 | **Capture** OpenCode `--version`, `models --help`, `models` output against an empty HOME, with network watched; confirm opt-out env vars; record in `testdata/help` + `sources.json`. Owner approval needed (runs a third-party CLI). | testdata only | — (evidence) |
| M1 | `models-v1` wire types, capability fields, validation, size cap | `terminalproto/models.go`, `protocol.go` | round-trip; reject unknown reason, >500, invalid ids, oversize; capability invalid without feature |
| M2 | Connector listing: catalogue `ModelList` row (OpenCode only), runner (env, temp cwd, timeout, caps, parser), dispatch, local switch, rate floor | `terminalhost/catalogue.go`, `terminalhost/models.go`, `terminalagent/agent.go`, `agenthealth/agent.go`, CLI | fake binary scripts: normal, ESC in output, huge output, hang (timeout kills group), non-zero exit, invalid lines skipped, disabled preset never execs, env has no extra vars, stderr never in reply, conflict |
| M3 | Broker + cache + API (`GET/POST models`, `GET /models`), rate limit, offline/old-machine states, forget cleanup, audit | `terminalbroker`, `store/store_models.go`, `api/models.go` | store migrate; failed refresh keeps last good list; 429; offline returns cache with state; old connector `update`; two-machine integration test |
| M4 | Favourites + account defaults + project effort store/API; Cline as project default | `store_models.go`, `store_backups.go`, `store_folder_projects.go`, `api/profile.go`, `api/folder_projects.go` | idempotent star/unstar, order 409, cap 50, invalid tool/model/effort refused, effort refused for non-effort tool, model without tool refused (new rows), backup/restore round-trip of profile columns |
| M5 | Web `models.ts` pure logic: `modelOptions`, `resolveLaunchDefaults` (precedence), API clients | `models.ts`, `terminal/api.ts`, `api.ts` | unit: ordering, dedupe, provider groups, stale/offline sentences, precedence table (each rung), agent switch re-resolves |
| M6 | Picker UI: `SlotPicker` star + model filter + "Use ‘x’"; accessible star | `launch/SlotPicker.svelte` | e2e keyboard: star by keyboard, announce, sheet on touch, 16 px field |
| M7 | Launch bar integration (after H) | `launchBar.ts`, `LaunchBar.svelte`, `Home.svelte` | launch.e2e: list from machine, offline cache, Other…, favourite first, defaults applied, never blocked by failed read |
| M8 | Start an agent dialog integration | `StartAgentDialog.svelte`, `agentLaunch.ts` | dialog e2e from Workspace, Projects bar, Machines card |
| M9 | Add project / Project settings: model picker, thinking, Cline | `home/AddProjectDialog.svelte` | home.e2e edit round-trip, 409 conflict path keeps fields |
| M10 | Settings › Agents (account default, favourites manager, base-branch/worktree defaults UI) | `Account.svelte` (+ small component) | account e2e, axe |
| M11 | Machines "Agents" section with read time + Refresh; Activity lines; hover card model; docs (all §1.4), handoff, phone/zoom matrix rows | `MachineSettings.svelte`, `events.ts`, `SessionHoverCard.svelte`, docs | machines e2e, phone-layout/zoom, axe |

M4–M6 have no machine dependency and can merge before M2/M3 work end to end (the picker simply
shows "reet can't read models on that machine yet").

---

## 6. Risks and security notes

- **Running a third-party binary unprompted.** Launch runs the agent only on a person's Start; a
  listing runs it on picker open/refresh. Mitigations: only connector-owned argv for confirmed
  presets, temp cwd, minimal env, timeout + group kill, rate floor, local off switch, audit of
  person-pressed refreshes, and D5 (automatic vs only-on-Refresh).
- **Network and self-update side effects** (OpenCode may fetch models.dev or update itself) —
  unverified; M0 must observe it. If the opt-outs don't work, ship OpenCode listing behind
  Refresh-only.
- **Secrets**: no keys in env; stderr never forwarded; output must be plain validated ids; nothing
  of the agent's config files read by reet. Env-only providers therefore won't appear (honest
  note in UI).
- **Untrusted text**: model ids rendered as text only (Svelte escapes); provider names derived from
  the id, not free text; ids pass the launch regex so they're safe as one argv element.
- **Stale lists**: a model removed by the provider still shows until refresh; harmless because the
  agent reports unknown models itself (existing doc stance).
- **Precedence surprise** (D1): changing today's "last launch wins" behaviour will be noticed.
- **Cross-machine favourites**: a favourite may not exist on another machine/provider setup; shown,
  not hidden, with "not in studio's list".
- **Conflicts with PR H** in `LaunchBar.svelte`, `Home.svelte`, `launchBar.ts` — hence M7+ after H.
- Uncertain: exact `opencode models` output format and flags (docs-only), Codex `model/list`
  existence, ACP model fields — all must be captured before use.

---

## 7. Open decisions for the owner (few, high impact)

- **D1. What does the launch bar open with?** (a) keep today's *last launch* (session history),
  defaults only when a project is chosen; (b) **defaults first** (project/account), last-used shown as
  a *Recent* group. *Recommend (b)*: the owner asked for defaults; it's predictable; Recent keeps
  the convenience.
- **D2. Per-machine default model?** *Recommend defer*: project defaults already bind a machine;
  adds a fourth rung and another settings place.
- **D3. Per-agent default via favourites** ("use this when I pick this agent")? *Recommend yes*: makes
  switching agents land on a sensible model without a separate settings matrix; one flag on a star.
- **D4. Widen the model-id rule** (e.g. allow `[`, `]` for Claude's `sonnet[1m]`)? *Recommend no for
  now*; revisit with a Claude capture. It's a connector-wide protocol change.
- **D5. When does a machine read its models?** (a) automatically on first picker open, then at most
  once per 24 h, plus Refresh; (b) only when the person presses Refresh. *Recommend (a)* with the
  local off switch, **if** M0 shows `opencode models` makes no unexpected network/update calls;
  otherwise (b).
- **D6. Approve M0** (installing and running `opencode models` / `--help` sandboxed, pinned npm
  `opencode-ai@<version>` as already approved for the probe). Required before M2 ships.
