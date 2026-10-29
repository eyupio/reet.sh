# Agent models: read from the machine, favourites and defaults

A session can carry a model. Today the value is free text with static hints in
the browser; the model-name rule is the same in the browser, controller and
connector (`internal/terminalproto/launch.go`). This PR adds a negotiated
`models-v1` extension so a machine can list the models its agent reports, caches
that list on the controller, and lets an account keep favourites and launch
defaults. The web picker is the next PR; nothing here changes a launch path.

## What a machine can list

Only **OpenCode** has a connector-owned listing in v1: `opencode models`, one
`provider/model` per line. The connector runs it with:

- a fresh 0700 temporary working directory, so no project `opencode.json` is read;
- the launch allowlist minus `TERM` and `SHELL`, plus `OPENCODE_DISABLE_MODELS_FETCH=1`,
  `OPENCODE_DISABLE_AUTOUPDATE=1`, `NO_COLOR=1`, `LANG=C.UTF-8` and `CI=1`; no provider
  keys, SSH agent or tokens are passed;
- stdin as `/dev/null`, a 10 s deadline, its own process group killed on timeout,
  64 KiB of stdout and 4 KiB of discarded stderr;
- no controller-supplied argument at all. The argv is a catalogue row.

Other enabled agents answer `unsupported` without running anything: Claude Code,
Codex and Gemini CLI have no confirmed listing command, and Cline's `config`
command is not run because it may print provider settings. Disabled presets (Pi,
Cursor) never list. A machine with the token but not an installed binary answers
`not_installed`.

The parser rejects the whole output if it contains a control byte or ESC, drops
lines that fail the model-name rule, de-duplicates, caps at 500 entries, and
trims the encoded answer to 12 KiB (inside the 16 KiB control frame) with `more`
set. The provider is the text before the first `/`. A listed name is a
suggestion: the launch path still validates the explicit `Create.Model` and never
reads the cache.

`opencode models` is documented by OpenCode; the capture in
`internal/terminalhost/testdata/help/opencode.txt` and
`opencode-models.txt` is the 1.19.5 probe's verbatim output with a fresh HOME and
`OPENCODE_DISABLE_MODELS_FETCH=1`. The probe observed no outbound call with that
setting. With the user's HOME the command may list providers that OpenCode's own
auth makes visible; env-only providers that the allowlist removes will be
missing, which the UI must state honestly. `--refresh` is never used: it ignores
the fetch opt-out and contacts models.dev.

## Capability, switch and request

`models-v1` is advertised with the other private terminal features. A connector
reports `models` only when the token was negotiated, terminal control is on,
terminal access is enabled and the local switch is on; it reports `modelsOff`
when the token is negotiated but the switch is off. Both require terminal
control and never appear together.

```
reet terminal models on|off|status        # also: reet agent terminal models …
```

It is on by default and refused while terminal access is off. A request is
`Models{tool}`; the reply is `ModelsResult{tool, models:[{id,provider}], more,
version, reason}`. The reason set is `not_installed`, `unsupported`, `timed_out`,
`too_large`, `exited`, `unreadable`, `denied`, `conflict` and `off`. One listing
runs at a time per connection (a second answers `conflict`); the connector
answers its last result for 60 s per tool instead of running again. stderr is
never forwarded.

## Controller cache and API

The controller keeps one row per account, machine and tool in
`machine_agent_models`. A listed answer replaces the models, provider metadata,
source (`opencode models`), version and `fetched_at`. Any other answer changes
only the state, reason and `checked_at`, so a failed refresh keeps the last good
list and its age. Forgetting a machine deletes its rows. The cache is derived
state and is not part of a reviewed backup.

All routes are account-scoped and behind the terminal wrapper:

```
GET  /api/v2/machines/{agentId}/models?tool=opencode
POST /api/v2/machines/{agentId}/models/refresh   {"tool":"opencode"}
GET  /api/v2/models?tool=opencode                # cache only, every machine
```

The read is cache first. If no row exists and the machine is online with the
`models` capability, the controller reads once synchronously (12 s) and stores
the answer; an offline machine, an old connector, a local off switch, an
unsupported tool or a failed read are returned as `offline`, `update`, `off`,
`unsupported` or `failed`, with the last good list where there is one. A refresh
is rate limited to 6 per minute per account and machine, answers 409
`models_busy` when the machine is already listing, and writes the
`machine-models-read` audit row (machine, tool, state, count) only for a
person-pressed refresh. A cache hit never contacts a machine. The response
carries `{tool, state, models, more, source, version, fetchedAt, checkedAt,
reason, sentence}`; the browser adds the relative age.

## Favourites and defaults

- Favourites are per account, at most 50, ordered, validated by the model-name
  rule and `ValidToolV2`. A favourite can be the **default for this agent**
  (at most one per agent). A picker can star/unstar, reorder with a version check
  and set or clear the agent default.
- The account profile gains `defaultTool`, `defaultModel` and `defaultEffort`
  for sessions outside a project. A model needs its agent; an effort needs an
  agent whose preset takes one. An existing profile row with a model but no agent
  stays readable; editing the launch defaults is validated as a whole, while an
  unrelated profile edit is not blocked by that legacy pairing.
- A folder project gains `defaultEffort`, and its `defaultTool` now accepts
  Cline (`ValidToolV2`), which fixes the old four-agent check. A project model
  needs its project agent; an effort needs an agent that takes one.
- The **browser** resolves the precedence: explicit choice, then the project
  default, then the account default, then a favourite marked for that agent,
  then the agent's own default. The server stores and validates these values but
  never substitutes one into a launch; what the browser sends is what runs.

API:

```
GET    /api/v2/profile/model-favourites
POST   /api/v2/profile/model-favourites          {tool, model}
DELETE /api/v2/profile/model-favourites?tool=&model=
PUT    /api/v2/profile/model-favourites/order    {version, items:[{tool,model}]}
PUT    /api/v2/profile/model-favourites/default  {tool, model}   # "" clears
PATCH  /api/v2/profile                            defaultTool/defaultModel/defaultEffort
PATCH  /api/v2/folder-projects/{id}               defaultEffort (and Cline as defaultTool)
```

Errors use `invalid_tool`, `invalid_model`, `invalid_effort`, `favourites_full`,
`favourite_not_found` and `favourites_stale`. Mutations are writer-only; reads
are available to any signed-in account. Audit rows name the tool and model only:
`model-favourite-added`, `model-favourite-removed`, `model-favourite-default`.

## What is here, and what is next

The launch bar now opens on the project, account and favourite defaults, reads
the chosen machine's list automatically, and shows the agent default, favourites,
Recent, the machine list by provider, reet's suggestions and Other…, with a star
and an honest status/Refresh line. A read failure never blocks Start. See
[terminal layout](ui-verification.md) for the size/touch evidence.

The Start an agent dialog, Add project/Project settings, Settings › Agents (the
account default and favourites manager), the Machines "Agents" section, Activity
and the hover-card model are the next UI PR (M8–M11). Until then the profile
defaults and favourites are editable only through the API in those places, the
Start dialog still has its old select, and only OpenCode has a machine list.

No real provider or paid model was used to test this backend: the listing parser
and runner are tested with fake executables, the protocol with round-trip and
limit tests, and the cache/API with synthetic machines. A physical phone, Safari
and a live machine with provider credentials are not checked.

See [the design](../docs/research/models-design.md) for the task split and the
reasoning, and [terminal launch](terminal-launch.md) for the model field itself.
