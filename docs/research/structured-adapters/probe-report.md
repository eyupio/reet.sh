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
container has no `bwrap`; the re-run below shows Codex uses its own Linux sandbox helper, which is active here.

- **Identity and auth.** Auth methods `api-key` and `chat-gpt`; a synthetic `api-key`
  authenticate succeeded. Modes: `read-only` ("Ask for approval"), `agent` ("Approve for me",
  the default, only asks for actions it deems unsafe) and `agent-full-access`. Models are
  IDs with the effort embedded, such as `gpt-6-astra[low]`.
- **Tools offered.** `exec_command`, `write_stdin`, `request_user_input`, `view_image`,
  `multi_agent_v1`, `get_goal`, `create_goal`, `update_goal`, `web_search`. The command
  tool is `exec_command` with a `cmd` argument, not `shell_command`.
- **No approval request in any mode (superseded: see "Codex gates, re-run" below; the probe's "outside" path was inside Codex's default writable roots).** A harmless write ran with **no
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

## Codex gates, re-run (1 November 2026)

Same adapter (`codex-acp@1.13.1`) and CLI (`@openai/codex@0.156.1`), loopback stub, temporary `CODEX_HOME`, synthetic credentials; `scripts/probe-codex-turn.py` gained
`--marker-path` (write to an exact path, refused if it already exists, removed afterwards), `--escalate`, `--run-command` and a `cancel-running` decision. Results:

- **The sandbox is active here, and the earlier "outside the project" probe did not test it.** The native CLI's own sandbox (`codex sandbox -- ...`, a
  helper it ships; no `bwrap` needed) refuses a write to `/usr` with "Read-only file system". The earlier probe's "outside" marker lived under `/tmp`, which Codex's default
  sandbox treats as writable (it lists `/tmp`, `$TMPDIR` and the project as write roots), so it proved nothing about approvals.
- **A write to a path outside every writable root is blocked silently, in `read-only` and `agent` modes.** No `session/request_permission`, the file is not created, the turn
  ends `end_turn`. In `agent-full-access` the same write succeeds, again with no request.
- **Approvals are model-driven, not per command.** `exec_command` takes `sandbox_permissions` (`use_default` or `require_escalated`), `justification` and `prefix_rule`. Without
  `require_escalated` every command runs inside the sandbox with no prompt, including writes inside the project. Only a model that asks to leave the sandbox raises an approval,
  so reet's "every command asks first" contract does not hold for Codex.
- **`read-only` ("Ask for approval") with an escalation request raises one request**, kind `execute`, title "Run command", `rawInput.command` and `rawInput.cwd` (a card can be
  reviewed), the model's justification in `_meta.permission.description`, and three options: `allow_once`, **`accept_execpolicy_amendment` (kind `allow_always`: "don't ask again for
  commands that start with ...", a persistent rule)**, and `cancel` (kind `reject_once`, "No, and tell Codex what to do differently"). Allow runs the command outside the sandbox
  (file created). Reject and cancel both leave the file absent and **end the whole turn as `cancelled`**, not just the one tool.
- **`agent` ("Approve for me") with the same escalation request raises nothing**: no request, no file, turn `end_turn`. An automatic reviewer decides, so the person is never asked
  and is not told what was refused.
- **Cancel does not stop a running command.** With `sleep 47` running, `session/cancel` ends the turn as `cancelled` in about a second, but the `sleep` process was still alive
  15 seconds later in both modes (until the probe killed it). A user pressing Stop would be told it stopped while the command runs on.
- **Provider failure** is unchanged (above): a disguised `end_turn` with `usage: null` after retries.

**What this means for an integration.** Codex cannot be wired as a drop-in for the OpenCode and Claude approval card. Needed first, as product decisions: whether `read-only` is the only
mode reet starts (it is the only one that can ask); not offering `allow_always` or mapping it to this session only, since it writes a persistent rule; showing the justification and
that a reject ends the turn; treating a silent sandbox denial and an `agent`-mode refusal as events the person should see; and retiring the process group on Stop, since cancel alone
leaves the command running. None is built.

## Codex start, probed (1 November 2026)

`scripts/probe-codex-start.py` against `codex-acp@1.13.1`: temporary HOME and CODEX_HOME, no credential variables, **no model call and no network**. Observed:

- `initialize` reports `agentInfo.name` `@agentclientprotocol/codex-acp`, version `1.13.1`, `authMethods` `api-key` and `chat-gpt`. The profile's expected name is right.
- `session/new` before `authenticate` fails with code `-32000`, "Authentication required". It is not a silent success as with Claude; a chat started without an authenticate step cannot start.
- After a synthetic `api-key` authenticate, `session/new` succeeds starting in `agent`, with `modes` `read-only`, `agent`, `agent-full-access` and options `mode`, `collaboration_mode`, `model`, `reasoning_effort`, `fast-mode`.
- **`session/set_config_option` with `configId: mode` works and confirms**: the reply carries the full option list with `mode` now `read-only`. This is the switch reet's profile uses, and its reply is authoritative.
- `session/set_mode` replies `{}`, which confirms nothing; reet does not use it.

**ChatGPT sign-in (`--auth chat-gpt`, no login on the machine).** `authenticate` with `chat-gpt` does not return a URL or wait for a token: it tries to open a browser on the host
(`spawn xdg-open ENOENT` here, code `-32603`), and `session/new` still fails with "Authentication required". The adapter also sent an `_auth/status_update` notification with
`authStatus: {kind: "none", label: "Not logged in"}`. So reet cannot sign a person in from a chat: calling `chat-gpt` on a logged-out machine starts an interactive browser flow
on that machine. What a logged-in machine sends (the `authStatus` kind, and whether `authenticate chat-gpt` then succeeds without opening anything) is **not observed**, and
needs a real login; `CODEX_HOME` and `HOME` would also have to reach the person's own login, which the connector's replaced `HOME` does not. reet's decided approach is therefore not to call `authenticate` at all (see the plan).

Still open: how a real person authenticates. `api-key` needs a key sent in `_meta`; `chat-gpt` is a browser sign-in. The connector replaces `HOME`, so a login under the real home is not visible to the adapter, and nothing in the profile calls `authenticate` yet.

## Claude environment (observed)

`probe-claude-lifecycle.py --drop VAR --add KEY=VAL` removes or adds adapter environment
variables. With the loopback stub, removing any one of `XDG_CONFIG_HOME`, `XDG_DATA_HOME`,
`XDG_CACHE_HOME`, `LANG`, `CLAUDE_CONFIG_DIR`, `ANTHROPIC_MODEL`,
`CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC`, `DISABLE_AUTOUPDATER`, `DISABLE_TELEMETRY` or
`CLAUDE_CODE_ENABLE_PROMPT_SUGGESTION` left the turn, approval card and file effect
unchanged, and so did removing all ten together, or `PATH`, or `ANTHROPIC_API_KEY`
(the stub does not check credentials, so a missing-credential error shape is **not**
observed). `--version` prints `0.99.1` in about 0.4 s (the version probe allows 2 s). `initialize` reports no
`authMethods`. The adapter source (read, not run) handles `ANTHROPIC_API_KEY`,
`ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`, `CLAUDE_CODE_USE_BEDROCK` and
`CLAUDE_CODE_USE_VERTEX`, so those can redirect or replace the credential.
`acpagent.ClaudeCandidate` (unregistered) therefore inherits only `ANTHROPIC_API_KEY` and
`CLAUDE_CODE_OAUTH_TOKEN` on top of the infrastructure allowlist, never provider
redirects or `CLAUDE_CONFIG_DIR`, sets the four background-traffic switches, requires the
adapter's `default` (Manual) mode and carries `ClaudeSessionMeta`. Unverified: real
login credentials under the real home, proxy variables, a missing-credential error,
Bedrock/Vertex.

## Integration gates still open

Claude remains disabled. Done in this checkpoint: the repository-hooks gate (`ClaudeSessionMeta`), adapter-aware UUID validation
(gated behind the reviewed-tool list) and bounded typed edit-diff normalization.
Also done (local): `Session.Close` now kills the group and confirms no live member remains
(`GroupRetired`, zombie-aware on Linux), and a `think` permission shows the subagent type
and bounded prompt in the command field, staying unapprovable without a prompt.
Also done (local): `SetModel` reads the applied model back and refuses a silent reset;
subagent cards are labelled. Also done (local): the Claude environment profile (`ClaudeCandidate`). Still required before
enabling Claude: a real-credential/missing-credential check, registering the profile behind
the descriptor and launch gates, and the SC3 question contract.
Also done (local, SC4 slice 1): Claude is registered but dark. A machine opts in with `REET_EXPERIMENTAL_CLAUDE_CHAT=1`
on the connector; without it Claude is `unverified` exactly as before. Opted in, its descriptor reports only what the probe
verified (approvals and a read-back model switch); native-session replay, images and questions stay off. Descriptors are bound
to the adapter reviewed for the tool, and a peer that sent no descriptors is still assumed OpenCode-only. The browser does not
offer Claude chats yet. Still required before turning it on: the real-credential and missing-credential check on a real
machine, the browser launch and view changes, and Claude-specific recovery.
Keep question support disabled until the SC3 contract is implemented.

For Codex, additionally establish whether a per-command approval exists with a working
sandbox (none appeared here), handle the disguised provider error, and verify cancel on a
running command. Its handshake or synthetic authentication alone
is insufficient to enable it. Candidate order: Claude first, conditional on these
gates; Codex remains a separately verified integration.

Codex probes: `python3 scripts/probe-codex-turn.py <allow|deny|cancel|error> --tool-name exec_command --mode read-only [--outside] --workspace "$probe_dir"` (add `--discover` to list tools).

## Claude credential check (SC4 gate, 1 November 2026)

`scripts/check-claude-credentials.py` runs the pinned adapter with the connector profile's environment shape and records what
the adapter does about credentials. Two scenarios: `session` (initialize and create a session; no model call, cannot spend
anything) and `prompt` (also sends one short prompt that allows no tools; needs `--yes-spend` because a logged-in account is
billed a small amount). `--home isolated|real` chooses an empty temporary home or this machine's own home.

Observed in the SC4 session (adapter 0.99.1, no credential variables, empty temporary home):

- `initialize` advertises **no** login methods (`authMethods` is empty), and `session/new` **succeeds**. Session creation
  therefore does not reveal a missing login; the first prompt would.
- The run nevertheless authenticated: a one-word prompt completed with token usage, and the adapter wrote its synced settings
  into the temporary home. The sandbox supplies credentials outside the process, so **this session cannot test the
  logged-out case**. The first draft of the script claimed a prompt with no credentials could not bill; that was wrong here
  and one small turn was spent before it was corrected. The script now makes no model call unless asked to.

Still to run by a person, on a machine or OS account that is logged out of Claude:

1. `python3 scripts/check-claude-credentials.py prompt --workspace "$probe_dir" --yes-spend` and keep the JSON: the error
   shape (code, message, whether `authMethods` appears) is what the browser will have to explain.
2. The same with `--home real` on a logged-in machine: confirms a real login is found and the reply is `ok`.
3. Confirm that an `ANTHROPIC_API_KEY` or `CLAUDE_CODE_OAUTH_TOKEN` in the environment is the only way the connector
   forwards a credential (the profile forwards nothing else).

Until 1 and 2 are recorded, Claude chats stay behind `REET_EXPERIMENTAL_CLAUDE_CHAT`.

### Claude profile against the real adapter (1 November 2026)

`TestRealClaudeStart` (opt-in: `REET_CLAUDE_ACP=<path to claude-agent-acp@0.99.1>`) starts the real adapter through
`ClaudeCandidate` with no prompt, so no model call. Observed: the adapter identity and version floor pass, the session starts in
the required Manual (`default`) mode with a UUID-form id, `Close` returns with the process group retired, and a profile whose floor is
above the installed adapter refuses to start it. It does not test credentials, turns, approvals or resume.


## Codex login source check (2 November 2026; runtime check still pending)

The owner approved implementing the login route from source evidence and keeping the real signed-in run as final verification. The pinned
`@agentclientprotocol/codex-acp@1.13.1` package source was inspected, without reading a person's credentials or making a model call:

- `checkAuthorization` uses `authRequired`, which calls `accountRead({refreshToken: false})` and returns `requiresOpenaiAuth && !account`.
  An existing account therefore needs no ACP `authenticate` call according to the source. This has not yet been observed with a real ChatGPT login.
- `_auth/status_update` carries `authStatus.kind` and `label`. ChatGPT is kind `account`, with a label such as "ChatGPT Pro" and a separate
  `account.email`. Other kinds are `none` ("Not logged in"), `api_key` ("OpenAI API key"), `gateway` and `external`. The first push is scheduled
  after initialize; later account changes can push another status.
- The integration uses the person's actual `CODEX_HOME` (or original `HOME/.codex`), while keeping replacement `HOME`. It decodes only the public label
  and known kind, discarding account and detail. It never calls `authenticate` or copies `auth.json`.

The fake-child regressions establish reet's routing, mode confirmation and error/notice handling; they do not establish provider authentication.
Section 3 Run B in `docs/device-checks.md` remains the signed-in release gate. Neither switch is retired by this change.


## Startup model read-back (2 November 2026)

Linux x86-64, Node v24.19.0; pinned OpenCode 1.19.5 and Claude ACP 0.99.1 (resolved SDK 0.3.312).
Installed outside the repository with `npm install --ignore-scripts --no-audit --no-fund`. Because OpenCode's postinstall is disabled, the probe used
`node_modules/opencode-linux-x64/bin/opencode` directly. Npm verified the package-lock integrity records. Lock SHA-256 values:

- OpenCode: `9f3dcbcf850bf978445e289651b2e29ea2125c48a841af0916c27375ba090bc6`.
- Claude: `8cd73ef47f65f97faf30ea5089a609c9965fa458d9c325bd093d3886430ede5e`.

A direct protocol probe used isolated HOME/project/config/data/cache, the launch profile's config, synthetic Claude credentials and a closed loopback
provider endpoint. It called only `initialize`, `session/new` and `session/load`, never `session/prompt`, `authenticate` or a model-setting RPC.
Sanitized results are recorded in `fixtures/startup-models.json` (transient session IDs omitted).

| Agent / requested model | New and load model `currentValue` | Mode |
| --- | --- | --- |
| OpenCode / agent default | `opencode/big-pickle` | `build` |
| OpenCode / private config `anthropic/claude-sonnet-4-5` | `opencode/big-pickle` | `build` |
| Claude / agent default | `default` | `default` |

Both adapters report these through `configOptions`, not a legacy `models.currentModelId`. OpenCode's private config alone did not establish the
alternate selection. Preserve mismatch refusal; these results do not justify silently setting a startup model with another RPC.

The enhanced opt-in profile tests reproduce explicit matching read-back using the actual Go launch profiles:

```sh
REET_OPENCODE_1195=/path/to/opencode-linux-x64/bin/opencode \
REET_CLAUDE_ACP=/path/to/claude/node_modules/.bin/claude-agent-acp \
go test ./internal/acpagent -run '^TestReal(OpenCode|Claude)Start$' -v -count=1
```

Both passed: OpenCode new/load confirm the explicitly selected observed default and preserve resume identity; Claude new confirms `default`,
retires its process group and retains the strict-version refusal check. The direct probe also measured Claude load, but the Go profile test does not
claim Claude resume coverage. No prompt or model call was sent. No real-login, provider-turn or physical-device gate is closed by these checks.

## Structured chat option values (2 November 2026)

Linux x86-64, Node v24.21.0; pinned Claude ACP 0.99.1 and Codex ACP 1.13.1. A new
`scripts/probe-agent-options.py` starts one adapter with an isolated HOME and project, calls `initialize` and `session/new`, and prints
the `configOptions` whose category is neither `mode` nor `model`. It sent no `session/prompt`, made no model call and used no real
credential; Codex's `session/new` needs a synthetic `api-key` authenticate, which the probe sends as a placeholder. Session IDs are omitted.

```sh
python3 -I scripts/probe-agent-options.py --workspace /path/to/probe --agent claude
python3 -I scripts/probe-agent-options.py --workspace /path/to/probe --agent codex
```

Sanitized results are in `fixtures/agent-options.json`. Claude reported:

| Option id | Category | Values |
| --- | --- | --- |
| `effort` | `thought_level` | `default`, `low`, `medium`, `high`, `xhigh`, `max` |
| `fast` | `model_config` | `on`, `off` |
| `ultracode` | `model_config` | `on`, `off` |

Codex reported:

| Option id | Category | Values |
| --- | --- | --- |
| `collaboration_mode` | `collaboration_mode` | `default`, `plan` |
| `reasoning_effort` | `thought_level` | `low`, `medium`, `high`, `xhigh`, `max`, `ultra` |
| `fast-mode` | `model_config` | `off`, `on` |

Both adapters also listed `mode` and `model` options, which the descriptor already represents as `Modes` and `Model`. OpenCode 1.19.5's
measured `configOptions` (see `fixtures/startup-models.json`) contain only `model` and `mode`, so its descriptor carries no other option
contract. The effort values are what these adapters reported for the model the probe opened with; an adapter can offer a different list
for another model, and Codex also embeds effort in some model IDs. The descriptor therefore describes what may be offered and does not
claim that every value is valid for every model. This is installation evidence, not a model turn, login or provider-billing check.

## Claude parity probes (8 November 2026)

What Gaffer could turn on for Claude Code beyond approvals and a read-back model switch, measured against the same pinned adapter
(`@agentclientprotocol/claude-agent-acp@0.99.1`, Node 22, Linux) with no login, no real provider and no model call that costs anything.
Two kinds of run: `python3 scripts/probe-claude-lifecycle.py <load|images|options> --workspace <dir>` (the adapter installed under `<dir>/claude`, as in the Claude probes above) drives the adapter through a loopback Anthropic-compatible
stub with a synthetic key, and the opt-in Go tests `TestRealClaudeOptions` and `TestRealClaudeLoadUnstored`
(`REET_CLAUDE_ACP=<claude-agent-acp> go test ./internal/acpagent -run TestRealClaude -v`) drive it through reet's own client in the
environment the connector profile gives it, with no prompt.

| Fact | Measured | Source |
| --- | --- | --- |
| `session/load` is advertised | `agentCapabilities.loadSession` is true | `load` |
| History replay, adapter side | A second adapter process in the same home replayed `user_message_chunk`, `tool_call`, `tool_call_update` and `agent_message_chunk` | `load` |
| History replay through reet's client | Not measured: it needs a stored turn, which only the Python stub can produce. Slice 2 tests reet's client against a fake that sends the frames above | none |
| A conversation the adapter never stored | `-32002` "Resource not found"; reet reports it as the missing stored conversation | `load`, `TestRealClaudeLoadUnstored` |
| Load with a different folder | Succeeds (the adapter does not bind history to a folder; reet's account, machine, tool and folder binding is the only guard) | `load` |
| Image prompts advertised | `initialize` reports `promptCapabilities.image` as true | `images` |
| An image block reaches the provider request | Yes, the stub saw the `image` block and the turn ended `end_turn`. Whether a real model accepts or understands the image is not measured | `images` |
| Thinking level offered at `session/new` | In the connector's environment (no `ANTHROPIC_MODEL`, model `default`): yes, `effort` with `default, low, medium, high, xhigh, max`, current `default`, beside `fast` and `ultracode`. With an explicit model id in `ANTHROPIC_MODEL` it is not offered until another listed model is chosen | `options` (run with and without `--add ANTHROPIC_MODEL=…`), `TestRealClaudeOptions` |
| Thinking level reads back | Setting `low` reads back `low`. An unlisted value is refused by the adapter ("Invalid value for config option effort") and by reet before anything is sent | `options`, `TestRealClaudeOptions` |
| `fast` reads back | With the model still the one `session/new` opened with (`default`), setting `on` reads back `on`. After choosing `opus`, setting `off` reads back `off`. After choosing `sonnet` the adapter no longer offers `fast` at all (it offers `effort` and `ultracode`), so which options exist depends on the model | `options`, `options --set-model sonnet` |
| A launch naming only an effort | Works: the effort is applied and read back before any prompt | `TestRealClaudeOptions` |
| A launch naming a model other than the adapter's own | Refused by the profile as it stands ("the agent did not use the chosen model"), because the profile does not confirm a model by switching. The same launch works, with the effort applied, once the switch is confirmed by reading it back | `TestRealClaudeOptions` |

**Not measured:** a signed-in turn, a real model accepting images, what the adapter does with an image type it does not support, the
effort values a real account is offered per model, and history replay through reet's own client against the real adapter (the
stored session a `load` run leaves under its throwaway home could be consumed by a later opt-in Go test). The refusal at the first
prompt when there is no login was recorded on 4 November in [Device checks](../../device-checks.md) and was not repeated here.
Nothing here is proof of a working signed-in session.

**What this lets slice 2 turn on:** history and in-place reconnect (the adapter loads stored sessions and refuses unknown ones in a form
reet already maps), image prompts (with the caveat above), a thinking level at launch, and a launch on a chosen model once the profile
confirms the model switch by read-back. `fast` and `ultracode` stay listed and uncontrolled.

## Codex parity probes (9 November 2026)

What Gaffer could turn on for Codex beyond approvals, measured against the adapter as reet starts it, with no login, no network and no model call.
Linux x86-64, Node 22. Two adapters, each installed with the `codex` it depends on: `@agentclientprotocol/codex-acp@1.13.1` (codex-cli 0.156.1,
the version the earlier probes pinned) and `@agentclientprotocol/codex-acp@2.6.0` (codex-cli 0.166.0, the latest on the registry that day).
`session/new` needs only a placeholder API key, either from `authenticate` (the probe) or from `auth.json` in the Codex folder (the Go test, which
is how reet reaches the adapter). Nothing sends `session/prompt`.

```sh
python3 -I scripts/probe-codex-start.py --workspace /path/to/probe        # needs <workspace>/codex with the adapter installed
REET_CODEX_ACP=/path/to/node_modules/.bin/codex-acp go test ./internal/acpagent -run TestRealCodexStart -v
```

| Fact | Measured (both adapters unless stated) | Source |
| --- | --- | --- |
| Image prompts advertised | `initialize` reports `promptCapabilities.image` true (and `embeddedContext`). The descriptor still says no images | probe `capabilities` |
| `session/load` and other session methods advertised | `loadSession` true; session methods `resume`, `list`, `close`, `delete`, `fork`, `additionalDirectories`, `subagents` (2.6.0 adds `title` and `systemPrompt`) | probe `capabilities` |
| A conversation the adapter never stored | `-32603` "Internal error" with `data.details` "no rollout found for thread id …". Not the `-32002` "Resource not found" the connector maps, so reet reports it as a generic "the agent exited" | probe `unstoredLoad`, `TestRealCodexStart` |
| Reasoning effort offered | `reasoning_effort`, current `low`. Values depend on the model: `low` to `ultra` on `gpt-6-astra`, `gpt-6-sol`, `gpt-5.6-sol`, `gpt-5.6-terra` (and `gpt-6.1-sol`, `gpt-6.1-terra` on 2.6.0); `low` to `max` on the two `luna` models; `low` to `xhigh` on `gpt-5.5` | probe `sweeps` |
| Effort reads back | On every model, every effort it offers reads back when set on a session already switched to that model (38 sets on 1.13.1, 50 on 2.6.0, no mismatch). Model ids in these adapters carry no effort suffix (the 2 November note that effort can be embedded in a model id is not what these two versions report) | probe `sweeps` (`effortPerModel`) |
| An effort the model does not offer | Refused by reet before anything is sent (`ErrControl`), checked on the default model | `TestRealCodexStart` |
| Model switch and the chosen effort | The effort is kept when the new model offers it; when it does not (`ultra` carried to a `luna` model or `gpt-5.5`) the adapter falls back to `medium`. The effort list follows the model | probe `sweeps` (`effortCarried`); the test checks the effort always stays inside the new model's list |
| Model switch reads back | Every listed model reads back when set, signed-out | probe `sweeps` |
| Modes | `read-only`, `agent`, `agent-full-access` on 1.13.1; 2.6.0 adds `workspace-write`. Each reads back when set. Starting mode is `agent`; reet starts the chat in `read-only` | probe `sweeps` |
| Default model | `gpt-6-astra` on 1.13.1, `gpt-6.1-sol` on 2.6.0. The offered list changes between adapter versions | probe `options` |
| A Codex folder that exists but holds no login | `session/new` fails `-32000` "Authentication required"; reet reports "Sign in to Codex on that machine first." | `TestRealCodexStart` |
| No Codex folder at all | codex exits with code 1 (`CODEX_HOME points to …, but that path does not exist`), because the profile sets `CODEX_HOME` to `~/.codex` whether or not it exists. reet reports "the agent exited", not the sign-in message. A person who has never used Codex hits this | probe with `--codex-home` set to a missing path (the summary's `initialize.error` or `session/new`), `TestRealCodexStart` (logged) |
| Terminal resume and effort (help only) | `codex resume [SESSION_ID] [PROMPT]` is a subcommand (`--last` picks the latest; a UUID or name is accepted); `-c, --config <key=value>` overrides any config value. No effort flag. Help only, nothing was run | `codex resume --help` on both CLIs |

**Not measured:** anything that needs a signed-in session or a model turn: history replay (a stored turn), a real model accepting an image, the effort
values and model list a real account is offered (these lists come from the adapter signed-out and may differ for an account), whether a chat's session
id opens in `codex resume`, whether `-c model_reasoning_effort=<value>` is honoured by the terminal CLI, and `authStatus` on a logged-in machine.
These wait for Run B in [Device checks](../../device-checks.md).

**What this gives the parity slice:** the descriptor can claim images (the adapter advertises them; a real model accepting one is not measured) and
reconnect to a stored conversation (the adapter advertises `session/load` and rejects unknown ids in a known form, once that error is mapped; replaying history
waits for Run B); model and effort can be offered and confirmed by read-back, with the effort list refreshed after a model
switch; the Codex-folder-missing start failure should read as a sign-in request; and the pin is a floor, not a version, because the offered models
and modes move between adapter versions.

## Codex lifecycle probes (9 November 2026)

What a stored conversation, an image and a chosen effort or model do, measured with the new `scripts/probe-codex-lifecycle.py` against the same two adapters as
above (`@agentclientprotocol/codex-acp@1.13.1` with codex-cli 0.156.1, and `2.6.0` with 0.166.0). The adapter runs in a throwaway HOME and CODEX_HOME with a
`model_providers` entry that points at a loopback Responses-API stand-in, so no account, network or real model is involved; the stand-in answers every request with
one short text message and records what it was sent. Nothing here shows what a real model does with an image or a stored conversation.

```sh
python3 -I scripts/probe-codex-lifecycle.py load --workspace /path/to/probe
python3 -I scripts/probe-codex-lifecycle.py images --workspace /path/to/probe
python3 -I scripts/probe-codex-lifecycle.py options --workspace /path/to/probe
```

| Fact | Measured (both adapters) | Source |
| --- | --- | --- |
| A stored conversation loads | After one turn and a stop, a second adapter process on the same Codex folder answers `session/load` for the stored id | `load` |
| What the replay carries | `user_message_chunk` and `agent_message_chunk` for the earlier turn, `available_commands_update` and `session_info_update`. reet drops replay before the chat is ready, since it keeps its own transcript | `load` |
| The loaded conversation continues | A follow-up prompt on the loaded session reaches the provider with the earlier reply in its input | `load` |
| Load from a different folder | Succeeds. The adapter does not bind a conversation to a folder, so reet's account, machine, tool and folder binding is the only guard | `load` |
| A conversation never stored | `-32603` "Internal error" with `data.details` "no rollout found for thread id …". reet now maps exactly that detail to the missing stored conversation, and any other internal error is still a generic failure | `load`, `TestRealCodexStart`, `TestCodexUnstoredLoadIsTheMissingConversation` |
| An image block reaches the provider | The request's input carries it as an `input_image` part, and the turn ends `end_turn`. Whether a real model accepts the image is not measured | `images` |
| A chosen model reaches the provider | After `session/set_config_option` the request's `model` is the chosen one. The adapter also sends a small helper request on a built-in model of its own that reet does not control | `options` |
| A chosen effort reaches the provider | After the model switch, setting `high` and then `low` sends `reasoning.effort` `high` and then `low`. The effort option is not offered for the stand-in's own made-up model, so a listed model must be chosen first | `options` |

**What this turned on in the product:** the Codex descriptor now reports a read-back model switch, history, in-place reconnect and image prompts (and still no
questions). The profile applies a chosen model by switching to it and reading it back, before any chosen effort. A machine with no Codex folder is told to sign in
instead of seeing "the agent exited". YOLO answers Codex permission cards only in `read-only`.

**Still not measured:** a signed-in turn, a real model accepting an image, the lists a real account is offered, and whether a Codex chat's id opens in `codex resume`.
These wait for Run B in [Device checks](../../device-checks.md).
