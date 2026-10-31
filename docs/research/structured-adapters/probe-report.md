# Structured adapter probe checkpoint

31 October 2026. SC2 probe checkpoint; neither candidate is enabled in reet.
This is separate from SC1's per-agent capability implementation in
[PR #300](https://github.com/eyupio/reet.sh/pull/300). The implementation sequence
is in [the structured chat plan](../../structured-chat-plan.md).

## Scope and evidence

Real adapter binaries were run on Linux x86-64 with Node 22.23.4, temporary
homes/projects, a fresh environment allowlist and synthetic credentials. The
Claude turn tests used a local Anthropic-compatible stub; it returned fixed tool
calls and text, with no real model turn or provider authentication. CLI and npm
installs were confined to a separate probe directory, with lifecycle scripts
disabled. No user configuration, existing agent installation or live machine
was changed.

**Observed** below means a recorded runtime result. **Read** means a claim from
installed adapter source, requiring a runtime check before relying on it.
[contracts.json](fixtures/contracts.json) narrows the captured frames to the
fields needed by a future integration. `${PROBE_ROOT}` replaces temporary path
prefixes. Full traces and generated lockfiles remain in the local probe workspace;
lock digests in the fixture identify that dependency selection. Stub token counts
are synthetic output, not evidence of model billing or provider compatibility.

## Exact packages and runtime identity

| Candidate | Adapter | Runtime |
| --- | --- | --- |
| Claude Code | `@agentclientprotocol/claude-agent-acp@0.99.1` | SDK `@anthropic-ai/claude-agent-sdk@0.3.312`; native `2.1.312 (Claude Code)` |
| Codex | `@agentclientprotocol/codex-acp@1.13.1` | explicitly installed `@openai/codex@0.156.1`; native `codex-cli 0.156.1` |

The adapter tarballs were downloaded separately and their SHA-512 values
recomputed against recorded registry metadata:

```text
@agentclientprotocol/claude-agent-acp@0.99.1
sha512-tK5lNtoI85IS0K8PVKYK86t/ARgCS1AjEw+u6QSNmdu0tbhyeB6FTNzFnGNIgsL/3xYwrQUMFDC6osT1aIPReQ==

@agentclientprotocol/codex-acp@1.13.1
sha512-NAbXTb6GRReox7B+8RN9VRB+sKUqFJh5Vg7Ex7RskYCO8EsYPJKN1WJvN7mOqjCKKK9lVDrNxtP7Bp/OUKPyAg==
```

This establishes byte identity with that registry metadata, not upstream build
provenance or a compatibility floor covering other versions. Transitive packages
were resolved by npm and recorded in generated locks; a replay with a new lock
must revalidate differences. The Codex adapter's native dependency uses a range,
so the native CLI was explicitly pinned for this probe.

## Observed contracts

| Check | Claude | Codex |
| --- | --- | --- |
| `--version` | `0.99.1` | `@agentclientprotocol/codex-acp 1.13.1` |
| ACP initialize | protocol 1; expected adapter package name/version | protocol 1; expected adapter package name/version |
| Delegated callbacks advertised by probe | filesystem false, terminal false | filesystem false, terminal false |
| Session creation | succeeded without delegated callbacks | succeeded after explicit synthetic-key ACP authentication |
| Native ID | UUID-style, without OpenCode's `ses_` prefix | UUID-style, without OpenCode's `ses_` prefix |
| Initial mode | `default` | `agent` |
| History capability | `loadSession: true` advertised; replay not verified | `loadSession: true` advertised; replay not verified |
| Tool approval/effect | Bash and Write allow/deny verified against local stub | not yet verified |
| Typed question | form capability, request and answer verified against local stub | not yet verified |

The first Codex attempt failed because its explicitly selected `CODEX_HOME`
directory did not exist. Creating that directory fixed the harness setup. Without
an ACP authenticate request, session/new then returned `Authentication required`;
a synthetic `api-key` authenticate request followed by session/new succeeded.
That is a local protocol result, not successful authentication with a real service.

The first Claude session/new attempt timed out with socket operations denied by
the execution environment. With the local socket permission needed by the SDK,
initialization and session creation succeeded. This is not evidence that reet
should widen its inherited environment or skip a launch deadline.

### Claude command and write permissions

The probe's private settings explicitly asked for `Bash(*)`, `Edit(*)` and
`Write(*)` and disabled hooks. The stub requested one harmless marker write in
its synthetic project. Both Bash and Write produced `session/request_permission`
with `allow_once` / `reject_once` options, IDs `allow-once` / `reject`:

- **Deny:** the turn finished and the marker did not exist.
- **Allow once:** the turn finished and the marker contained exactly `reet-probe`.

No filesystem or terminal reverse callback was needed for those operations.
This verifies these two tool paths under the tested settings, not an ask-for-all
policy covering every native tool, subagent or external-directory operation.

For Bash, `rawInput.command` and kind `execute` fit reet's existing shape. Write
uses kind `edit`, `rawInput.file_path` / `content`, and a typed diff in
`toolCall.content`: `{type: "diff", path, oldText: null, newText: "reet-probe"}`.
It does not provide OpenCode's `rawInput.diff` string. Reet's current parser would
mark this card incomplete and disable Allow once. The integration must normalize
that typed diff with existing bounds; do not simply bypass the incomplete-card
check or silently treat a missing diff as approval-ready.

### Claude questions

Without elicitation form support, AskUserQuestion was absent from the observed
model tool catalogue. Advertising `clientCapabilities.elicitation.form: {}` made
it available. A fixed AskUserQuestion tool call produced `elicitation/create`
with `mode: "form"`, native session ID, toolCallId and requestedSchema.

The observed schema had a `question_0` string with `oneOf` const/label/description
options, plus an optional `question_0_custom` string. Returning
`{action: "accept", content: {question_0: "Option A"}}` completed the turn. No
command/edit permission card or marker write occurred in that question case.

This gives SC3 a measured starting shape. Multi-select, numeric/boolean forms,
request-scoped startup forms, cancellation and invalid/stale answers remain
unverified. Do not advertise the capability in reet before those supported
shapes have backend validation and an answer transport.

## Read-only source findings

The installed Claude adapter uses the agent SDK and selects a native CLI from
its platform dependency. Its session options read user/project/local settings,
set a permission callback and expose configuration/mode descriptors. Its source
suppresses AskUserQuestion without form support, matching the runtime result.
Repository settings/hooks, permission overrides and environment injection still
need adversarial runtime probes; a clean temporary project is not that evidence.

The installed Codex adapter starts a separate native app-server. Its API-key
method accepts explicit ACP authentication metadata. Its mode/model/config and
tool approval paths differ from Claude's and need separate tests. Do not copy
OpenCode's environment, database recovery or permission options into either.

## Reproducing the checkpoint

Use an empty directory outside the repository and prepare the pinned candidates:

```sh
probe_dir=/absolute/path/to/empty-probe-workspace
npm install --ignore-scripts --no-audit --no-fund --prefix "$probe_dir/claude" @agentclientprotocol/claude-agent-acp@0.99.1
npm install --ignore-scripts --no-audit --no-fund --prefix "$probe_dir/codex" @agentclientprotocol/codex-acp@1.13.1 @openai/codex@0.156.1
python3 scripts/probe-structured-adapters.py --workspace "$probe_dir"
python3 scripts/probe-claude-turn.py deny --tool Bash --workspace "$probe_dir"
python3 scripts/probe-claude-turn.py allow --tool Bash --workspace "$probe_dir"
python3 scripts/probe-claude-turn.py deny --tool Write --workspace "$probe_dir"
python3 scripts/probe-claude-turn.py allow --tool Write --workspace "$probe_dir"
python3 scripts/probe-claude-turn.py allow --tool AskUserQuestion --workspace "$probe_dir"
for s in trust cancel load model subagent subagent-nested; do python3 scripts/probe-claude-lifecycle.py $s --workspace "$probe_dir"; done
```

The scripts install nothing. They require Python 3, Node 22+, the preinstalled
packages and local process/socket support. `--node` can select Node explicitly.
Each run creates a new temporary home/project under the supplied workspace and
retains it and the JSON trace for inspection. Caller-owned successful artifacts
are not auto-deleted. The environment is constructed from a small explicit set;
real provider credentials are never inherited. Synthetic credentials and
loopback provider endpoints are deliberate test inputs. The turn script binds
a loopback HTTP stub and executes a harmless write only when the selected test
answers Allow once. It asserts turn completion, permission/question presence
and the expected marker effect, returning nonzero on a mismatch.

On the managed test host, the scripts ran beneath a test-only child subreaper
because PID 1 does not collect orphan zombies. That harness requirement is not
a product change. Formatting and syntax checks passed; the scripts and their
assertions were run against the real pinned candidates. No physical phone,
macOS, Windows, genuine provider authentication or paid turn was checked.

## Claude lifecycle probes (second checkpoint)

Run with `scripts/probe-claude-lifecycle.py <trust|cancel|load|model|subagent>`
against the same pinned adapter 0.99.1 / SDK 0.3.312 and the loopback stub, on
Linux x86-64 with Node 22. Same limits as above: no real provider, credentials,
phone or other OS.

- **Repository trust (observed).** A project `.claude/settings.json` with
  `permissions.allow: ["Bash(*)"]` and `defaultMode: "bypassPermissions"`, plus a
  `settings.local.json` allow, did **not** suppress the approval request while the
  user settings asked for Bash: one `session/request_permission` arrived and the
  marker appeared only after Allow once. The same repository's `PreToolUse` hook,
  however, **ran** (a marker file written by the hook) with hooks enabled. A
  repository can therefore execute commands through its own hooks without a
  reet approval card. Enabling Claude requires launching with project hooks
  disabled or a documented trust gate; this probe did not find a safe flag-free
  way to rely on the adapter alone. **Mitigation (observed):** sending
  `_meta.claudeCode.options.settingSources: ["user"]` on `session/new` (probe flag
  `--setting-sources user`) stopped the repository hook (`hookRan: false`) while the
  approval card still appeared; the adapter default (`user,project,local`) ran it.
  reet carries this as `acpagent.ClaudeSessionMeta`, sent on new and load, so a
  repository's own settings and CLAUDE-style project config are not read: that is
  a deliberate context cost. Other override paths (managed settings,
  `--settings`, MCP servers declared by the project) remain unprobed.
- **Cancellation and retirement (observed).** `session/cancel` sent in place of a
  permission answer ended the turn with `stopReason: "cancelled"` and no file
  effect. A cancel sent during a slow upstream request returned `stopReason:
  "end_turn"` within about two seconds, not `cancelled`, so reet must not read
  `end_turn` as proof the work was stopped. After SIGTERM to the adapter's process
  group, one group member was still alive after the leader exited; leader exit is
  not group retirement, so reet must confirm the whole group is gone and escalate.
- **History (observed).** A second adapter process in the same home loaded the
  session by ID and replayed `user_message_chunk`, `tool_call`,
  `tool_call_update` and `agent_message_chunk`. `session/load` with a **different
  `cwd` succeeded**: the adapter does not bind history to the folder, so reet's
  account/machine/tool/folder binding is the only guard. An unknown UUID returned
  `-32002 Resource not found`. `session/list` returns IDs, `cwd`, title and
  `updatedAt`; it was not used to authorise anything.
- **Model and mode (observed).** `session/new` reports no `models` field, only
  `configOptions` for `mode` (default, acceptEdits, plan, auto) and `model`
  (aliases such as `default`, `opus`, `sonnet`, `haiku` plus a full ID); later
  responses added `effort`, `fast` and `ultracode` options, so the option list
  changes. `session/set_model` is not implemented (`-32601`). `session/set_config_option`
  with a valid alias returned the updated list with the new `currentValue`; an
  **unknown model value was accepted and silently reset to `default`**. An unknown
  mode through `session/set_mode` was rejected (`-32602`). Report an applied state
  only after reading `currentValue` back. The stub-served model name followed the
  resulting option, not the request.
- **Subagents (observed, narrow).** The model tool catalogue lists `Agent` (not
  `Task`). A stubbed `Task`/subagent call produced one permission request with
  kind `think`, the subagent description as title and `description`, `prompt`,
  `subagent_type` in `rawInput`; the subagent's own work in this stub did nothing,
  so nested tool permission was not exercised. reet's parser has no card for kind
  `think`; it must show the prompt or refuse.

### Claude subagents, nested (observed)

`scripts/probe-claude-lifecycle.py subagent-nested`: a stub subagent asked to run
Bash. Two separate permission requests arrived: the delegation (kind `think`) and the
subagent's own `execute` call, whose `toolCall._meta.claudeCode.parentToolUseId` named
the delegation's `toolCallId`. Approving the delegation did not approve the nested
command, so ask-for-all holds inside a subagent under the tested settings. reet
prefixes such cards "Subagent:" (`permissionTitle`); a dedicated parent field would need
a wire change and was not made.

## Codex lifecycle probes (observed, partial)

`scripts/probe-codex-turn.py` against `@agentclientprotocol/codex-acp@1.13.1` with
`@openai/codex@0.156.1`, a loopback Responses-API stub, a temporary `CODEX_HOME`, synthetic
credentials and a `model_providers` entry in `config.toml`. Same limits as above; this
container has no `bwrap`, so the Codex sandbox could not be shown to be active.

- **Identity and auth.** Auth methods `api-key` and `chat-gpt`; a synthetic `api-key`
  authenticate succeeded. Modes: `read-only` ("Ask for approval"), `agent` ("Approve for me",
  the default, only asks for actions it deems unsafe) and `agent-full-access`. Models are
  IDs with the effort embedded, such as `gpt-6-astra[low]`.
- **Tools offered.** `exec_command`, `write_stdin`, `request_user_input`, `view_image`,
  `multi_agent_v1`, `get_goal`, `create_goal`, `update_goal`, `web_search`. The command
  tool is `exec_command` with a `cmd` argument, not `shell_command`.
- **No approval request in any mode (open gap).** A harmless write ran with **no
  `session/request_permission`** in `agent` and in `read-only`, for allow, deny and
  cancel decisions, both inside the project and to a path outside it. Approvals are
  sandbox- and mode-driven, not per command, so reet's "every command asks" contract is
  not established for Codex. Whether a working sandbox changes this was not checked here.
- **Provider failure is disguised.** A 500 from the stub made the turn end with
  `stopReason: "end_turn"` and `usage: null` after about 25 seconds of retries; the only
  signal was `session_info_update` with `_meta.codex.error` ("Reconnecting... n/5",
  `willRetry`). reet must treat an `end_turn` with no content and an error update as a
  failed turn, as it already does for OpenCode's silent provider errors.
- **Config strictness.** `approval_policy = "untrusted"` in `config.toml` is rejected
  ("no longer supported"), and a bad user config kills the process during initialize.
  Codex also warns it will not create helper binaries under a temp directory.

## Integration gates still open

Claude remains disabled. Done in this checkpoint: the repository-hooks gate (`ClaudeSessionMeta`), adapter-aware UUID validation
(gated behind the reviewed-tool list) and bounded typed edit-diff normalization.
Also done (local): `Session.Close` now kills the group and confirms no live member remains
(`GroupRetired`, zombie-aware on Linux), and a `think` permission shows the subagent type
and bounded prompt in the command field, staying unapprovable without a prompt.
Also done (local): `SetModel` reads the applied model back and refuses a silent reset;
subagent cards are labelled. Still required before enabling Claude: required-environment
checks and the SC3 question contract.
Keep question support disabled until the SC3 contract is implemented.

For Codex, additionally establish whether a per-command approval exists with a working
sandbox (none appeared here), handle the disguised provider error, and verify cancel on a
running command. Its handshake or synthetic authentication alone
is insufficient to enable it. Candidate order: Claude first, conditional on these
gates; Codex remains a separately verified integration.

Codex probes: `python3 scripts/probe-codex-turn.py <allow|deny|cancel|error> --tool-name exec_command --mode read-only [--outside] --workspace "$probe_dir"` (add `--discover` to list tools).
