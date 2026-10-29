# Agent models backend: `models-v1`, the machine list, cache, favourites and defaults

Plan for the models PR after copy/paste (#242/#245). Input:
[docs/research/models-design.md](../../research/models-design.md) (tasks M0–M11). This plan covers the **Go
backend, M0–M4**; the web pieces M5–M11 are the next PR. Owner decisions already taken: **D1** the bar opens with
defaults first and the last used as a Recent group; **D5** the list loads automatically; **D2** a per-machine default
is deferred; **D3** a favourite can carry “default for this agent” (yes); **D4** the model-name rule stays; **D6**
capturing `opencode models` from the approved npm package is approved (the probe already did it).

## Global constraints

- `models-v1` is a negotiated optional feature. An old controller never sends a models request; an old connector is
  never sent one and advertises no capability. A new field or message is only sent after negotiation.
- No machine model list can block a launch. The launch path still uses the explicit `Create.Model`; the cache is a
  suggestion only.
- Only connector-owned argv runs. No controller-supplied argument reaches the listing. OpenCode only in v1; other
  tools answer `unsupported` without exec. Disabled presets (Pi, Cursor) never list.
- Minimal environment, fresh 0700 temp cwd, stdin `/dev/null`, 10 s timeout, own process group, 64 KiB stdout cap,
  4 KiB discarded stderr, 60 s per-tool floor. No agent config files are read, no stderr forwarded, no keys in env.
- The controller caches per account/machine/tool and never stores stderr or paths. A failed refresh keeps the last good
  list and its `fetched_at`; the UI can be honest about age and failure.
- Favourites and defaults are per account; project defaults are per folder project. Favourites at most 50; a favourite
  or default never overrides an explicit choice. The server never substitutes a default into a launch.
- The known gap is fixed: a project's default agent accepts `ValidToolV2`, so Cline can be one.
- No web changes in this PR. No new UI copy. The web continues to use its static suggestions until M5–M11.

## M0 — Capture evidence

**Files:** `internal/terminalhost/testdata/help/opencode.txt` (new), `sources.json`, `README.md`.

The probe report already holds the verbatim `opencode --version`, `opencode models --help` and `opencode models`
empty-HOME output at pinned `opencode-ai@1.19.5`, and confirmed `OPENCODE_DISABLE_MODELS_FETCH=1` makes no network
call. Copy that evidence into the testdata with a source entry (package, version, tarball, integrity, commands,
`OPENCODE_DISABLE_MODELS_FETCH=1`), and note the capture came from the recorded probe rather than a new run. No new
third-party command is run in this task.

## M1 — Protocol `models-v1`

**Files:** create `internal/terminalproto/models.go`, `models_test.go`; modify `protocol.go`, `protocol_test.go`.

**Interfaces**

```go
const ModelsFeature = "models-v1"
func SupportsModels(features string) bool
const MaxModels = 500
const MaxModelsReplyBytes = 12 << 10 // fits the 16 KiB control frame with its envelope

type Models struct { Tool string `json:"tool"` }                 // kind "models"
type ModelEntry struct { ID string `json:"id"`; Provider string `json:"provider"` }
type ModelsReason string // "", not_installed, unsupported, timed_out, too_large, exited, unreadable, denied, conflict, off
type ModelsResult struct {
    Tool string `json:"tool"`
    Models []ModelEntry `json:"models,omitempty"`
    More bool `json:"more,omitempty"`
    Version string `json:"version,omitempty"`
    Reason ModelsReason `json:"reason,omitempty"`
}
```

- `Models.valid`: `ValidToolV2(Tool)`; no SessionID binding.
- `ModelsResult.valid`: tool valid; if `Reason != ""`, no models/more/version; otherwise at most 500 unique entries,
  each ID passes `ValidModel`, provider is the text before the first `/` (or `""`), plain text and ≤64 bytes; version
  plain text ≤64; encoded reply ≤64 KiB.
- Capabilities gain `Models bool json:"models,omitempty"` and `ModelsOff bool json:"modelsOff,omitempty"`; both
  require `TerminalControl`; never both. Token enforcement happens at the capability builder, not `Validate`.
- `Decode` accepts `models` and `models_result`; `testMessage` binds no session; `Receive` late-drop handling for
  `ModelsResult`.

**Tests:** token match; round-trip maximal/minimal; all reason codes; reject unknown reason, >500 entries, duplicate
IDs, invalid IDs, provider mismatch, oversize, version too long, models with a reason; capabilities structural and
legacy JSON; strict decoding and binding.

## M2 — Connector listing

**Files:** modify `internal/terminalhost/catalogue.go`, `launch.go` (no argv change), create
`internal/terminalhost/models.go`, `models_test.go`; modify `internal/terminalagent/agent.go`, its tests;
`internal/agenthealth/agent.go` (+ tests); `internal/scan/agent.go`, `agent_terminal.go`,
`agent_terminal_control.go` (+ tests); `cmd/reet/terminal.go` (+ test).

**Behaviour**

- `Preset.ModelList *ModelList` with `Argv []string` and `Env []string`; only OpenCode:
  `{"models"}`, env `OPENCODE_DISABLE_MODELS_FETCH=1`, `OPENCODE_DISABLE_AUTOUPDATE=1`, `NO_COLOR=1`,
  `LANG=C.UTF-8`, `CI=1`. Disabled rows never list. `launchPresets` carries the row.
- `terminalhost.ListModels(ctx, tool, lookPath, ...) ModelsResult`:
  - find the enabled preset and resolve the binary as launch does; not installed → `not_installed`;
    no `ModelList` → `unsupported`.
  - run the connector-owned argv with `safeEnvironment()` minus `TERM`/`SHELL`, plus the row env; temp 0700 cwd;
    stdin `/dev/null`; 10 s timeout; process group kill on timeout; stdout cap 64 KiB; stderr 4 KiB discarded.
  - parse lines: trim, drop empty, reject any byte < 0x20 except `\n`/`\t` or ESC (whole result `unreadable`), each
    line must pass `ValidModel` (skip invalid), unique, cap 500 (`More`). Provider = text before `/`.
  - timeout → `timed_out`; overflow → `too_large`; non-zero exit → `exited`; unreadable → `unreadable`; no models
    (empty output) is a valid empty list (not a reason) or `unsupported`? A listed tool with empty output is a valid
    empty list.
- Connector dispatch `*terminalproto.Models`: only on a `models-v1` connection; validate tool; local consent and
  `reet terminal models on|off|status` (`ModelsOff`) → `off`; one listing at a time (second → `conflict`); a 60 s
  per-tool floor returns the previous result; otherwise run and reply `ModelsResult`. Never run for a tool without a
  preset/ModelList.
- `agenthealth`: negotiate `models`, `terminalFeatures`, `Capabilities.Models/ModelsOff`, `ModelsAllowed` switch
  re-hello; `scan` sets it from the agent config; CLI switch.
- Add `models-v1` to the controller's agreed features and expected feature-list test.

**Tests:** fake binary scripts for normal, empty, invalid lines skipped, ESC/control output, huge output, hang
(timeout kills the group), non-zero exit, unsupported/not-installed/disabled tools, env allowlist and opt-outs,
stderr never in the reply, one-at-a-time/conflict, 60 s floor, local switch on/off/status, capability and re-hello.

## M3 — Broker, cache and API

**Files:** create `internal/terminalbroker/models.go`, `models_test.go`, `internal/store/store_models.go`,
`store_models_test.go`, `internal/api/models.go`, `models_test.go`; modify `broker.go` (Receive), `api.go`
(routes/server state), `store.go` (migrations), `store_remote_health.go`/`store_machines.go` (forget cleanup),
`cmd/reet` if needed.

**Behaviour**

- `Broker.Models(ctx, uid, agentID, tool)` validates the request, finds a ready peer and the stored capabilities;
  old → `ErrModelsUnsupported`, off → `ErrModelsOff`, otherwise `p.ask` with `ModelsCallTimeout = 12s` and returns the
  `ModelsResult`. No cache in the broker.
- New table `machine_agent_models(user_id, agent_id, tool, state, reason, models, more, source, agent_version,
  fetched_at, checked_at, PRIMARY KEY(user_id, agent_id, tool))`, `models` JSON ≤500. A failed refresh updates
  `checked_at`/`state`/`reason` but keeps the last good `models`, `fetched_at`, `source` and `version`; a success
  replaces them. Rows are deleted when a machine is forgotten. Not backed up.
- API:
  - `GET /api/v2/machines/{agentId}/models?tool=opencode` via the terminal wrapper: cache first; if never read and the
    machine is online with `Models`, read once synchronously (≤12 s) and cache; otherwise return `offline`, `update`,
    `off`, `unsupported` or `failed` with the last cache. Shape: `{tool, state, models, more, source, version,
    fetchedAt, checkedAt, reason, sentence}`.
  - `POST /api/v2/machines/{agentId}/models/refresh {tool}` via the terminal wrapper; per user+machine 6/min
    (`models_rate_limited`), 409 `models_busy`; audit `machine-models-read` only for this person-pressed path.
  - `GET /api/v2/models?tool=opencode` cache only, one row per machine (`{machines:[{agentId,host,state,fetchedAt,
    models,...}]}`), never live.
  - Errors/sentences: old `Update reet on that machine to read its models.`; off `Models are switched off on that
    machine.`; offline `That machine is reconnecting. Try again in a moment.`; timeout, busy, failed.
- Tests: store migrate/upsert/keep-last-good/forget cleanup; broker capability gating and forwarding; API first read,
  cache hit, offline/update/off/unsupported/failed, refresh rate limit and audit, two-machine list, owner scope.

## M4 — Favourites, defaults and project effort

**Files:** create `internal/store/store_models_favourites.go` (or extend `store_models.go`), `store_models_favourites_test.go`,
`internal/api/model_favourites.go`, `model_favourites_test.go`; modify `store_backups.go`, `api/profile.go`,
`store_folder_projects.go`, `api/folder_projects.go`, `api/api.go` (routes), `internal/api/project_runs.go` if the
default validation lives there, and the relevant tests.

**Behaviour**

- `model_favourites(user_id, tool, model, position, agent_default, created_at, updated_at)` with
  `PRIMARY KEY(user_id, tool, model)`, max 50 per account, tool `ValidToolV2`, model `ValidModel`; at most one
  `agent_default` per tool (clear the old one in the same transaction).
- `GET /api/v2/profile/model-favourites` → `{version, favourites:[{tool,model,agentDefault}]}`; `POST` star
  (idempotent 200/201), `DELETE ?tool=&model=` (idempotent 204), `PUT /order {version, items}` (409 stale),
  `PUT /default {tool, model}` (empty clears). Version is monotonic per account (millisecond plus increment under the
  store transaction). Audit add/remove/default changes.
- Account profile gains `defaultTool`, `defaultModel`, `defaultEffort`; validation: tool `ValidToolV2`, model
  `ValidModel` and requires tool, effort `ValidEffort` and requires a tool that `ToolTakesEffort`; existing rows are
  not rewritten. `profile-saved` detail includes the new fields. Backup/restore carries them.
- Folder projects gain `default_effort`; `default_tool` accepts `ValidToolV2` (Cline); model requires tool; effort
  requires an effort-taking tool; existing rows untouched. Project views/inputs carry `defaultEffort`.
- Tests: idempotent star/unstar, order 409 and cap 50, one default per tool, invalid tool/model/effort, profile
  defaults, backup/restore round-trip, Cline project default, existing row compatibility, owner isolation.

## Test and CI matrix

| Check | Where |
| --- | --- |
| protocol, host listing, connector, broker, store, API | `go test -race -timeout 40m ./...` |
| no new web fixtures | this PR is Go only; `npm run check` still runs |
| old/new compatibility | protocol round-trip and broker capability tests with old caps |
| docs | add `docs/agent-models.md`, update `docs/terminal-launch.md`, `docs/projects.md`, `handoff.md` |

## Compatibility notes

- A controller with an old connector: no `models` capability → the API returns `update` and the launch path is
  unchanged.
- A connector with an old controller: no token → `cfg.models` false → no capability; the feature is never served.
- A failed listing never deletes the last good cache; a model removed from the machine is still a suggestion.
- The API read is cache-first; the first read is synchronous only for a machine that reports the capability.
- The cache key includes the account and machine; forgetting a machine deletes its rows.
- Project defaults never enter a request unless the browser resolved them; the server validates and refuses invalid
  combinations but does not substitute.
