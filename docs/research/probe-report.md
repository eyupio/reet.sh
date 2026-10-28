# OpenCode `opencode acp` hands-on probe (Program 2, slice 1)

Date: 28 Oct 2026. Run in an isolated scratch directory against a local stub model. No credentials and no real provider were used, and outbound traffic went to a logging sinkhole. The first attempt (GitHub release) was blocked. The owner then approved npm `opencode-ai@1.19.5` as the install channel.

Probe directory (scripts, logs, dummy repo): `/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/`

**Bottom line.** The ACP mechanics work well: streaming, permission cards with real diffs, cancel, `session/load` replay, and errors all behave. As shipped, though, OpenCode's permission model does **not** protect a session:

- By default it never asks.
- A repository can switch asking off.
- A repository's plugin code runs as soon as a session opens, whatever the flags.
- Every `opencode acp` process starts an unauthenticated HTTP API on 127.0.0.1:4096 that can approve permissions behind the ACP client's back.

Slice 1 is **GO only with the adapter conditions in section 6**, and only for workspaces the user trusts. See section 7.

## 1. Version, provenance, integrity

| Item | Value |
|---|---|
| Spike pin | None anywhere in the repo. `program2-spike.md` is missing. The reference-tool notes give a compatibility floor of 1.16.0. |
| Installed | `opencode-ai@1.19.5` from registry.npmjs.org, with the version chosen by the owner. `opencode --version` prints `1.19.5`, and `initialize` reports `agentInfo {"name":"OpenCode","version":"1.19.5"}`. |
| Command | `npm install --ignore-scripts --no-audit --no-fund --prefix <probe>/install opencode-ai@1.19.5`. Run under `env -i` with HOME and the npm cache inside the probe directory. npm needed the session CA bundle (`npm_config_cafile`, `NODE_EXTRA_CA_CERTS`) and the proxy variables. TLS verification stayed on. |
| `opencode-ai-1.19.5.tgz` | integrity `sha512-tEqMvQahfhdDj8vceYpXtoIrvjPXm5XRfetWnPN5YISueU7adNx9JcUA3zZ+2GKuDVgDpGdQe1ZLtCZDjqodvA==` (**matches** the owner's value; I recomputed it from a separately downloaded tarball). sha256 `1f97e49381f8a42a629482eff8be98aa96d62e2e94410a8ec98e738f9c787680` |
| `opencode-linux-x64-1.19.5.tgz` (installed) | `sha512-i1qh7VooLGML9xX6ud93y0JA8j/1aPeDYRKjsmYXrJX1TjIWHgBBMkEbEJPZthpWbsJg//XfBgR6Kb7+NPaJiQ==` (recomputed and matches the lockfile). sha256 `2d7cfc303decc6cbbcdb22ff1be258b03ce2b5eae57583545afc8f12c01dd5e0` |
| `opencode-linux-x64-baseline-1.19.5.tgz` (installed) | `sha512-9kA+st3XXtCXMPx5f3Ld1u19uoHTO2u2Btt8a2cqhJ4GIqVbD1mXLevj3LHFEmtuQoFGk+UstGnl38sCqXj5sQ==` (recomputed and matches). sha256 `2249989386aab6b4055d0e8b79fa09874e715c3b15805728543211514b872d88` |
| ELF binary | `node_modules/opencode-linux-x64/bin/opencode`, 214,386,864 bytes, a Bun 1.4.4 single-file executable. sha256 `7df4e4136301f16589b0f30bf91a61380fca5f8ef22dd36570f5ebe1d4ec802c`. **The x64 and baseline binaries are byte-identical.** |
| Other optional packages (from the lockfile; not downloaded because they are for other platforms) | darwin-arm64 `sha512-0MbSbKkmwVHtgP31GJyyECe6QVy4T7W4Wp7s2TsmXHTPPyf01ne3dWSX97X9dkNK28Lc6kt+zMPQUP/9GgDLZg==`; darwin-x64 `sha512-+DO7IMPTCrX12UJXnL4pL3ckcwR6BR6f6tA9QIlVQ5nZx66T5U7FRMJT4n5rsOnJxLM/pZ2vfX1G8vrnvXO8FA==`; darwin-x64-baseline `sha512-rftvAoHBWPVkvwBLr3SQWRZvcIjyMTN0SlhMzlt+lgvoQBBGNEmZQ/7sS/Ux1oHsJV/++UDuLVso4n5L95YD5Q==`; linux-arm64 `sha512-u2HFRI6v1nNMVKiV34An8XrGBxJeZ+RCg2tYu3SqlGODKqKsr5RoEbaX1tnsfGb+rETh9AJVX9kS8dSZXPufsA==`; linux-arm64-musl `sha512-YxvpVPJ/jeevV93DVRiuOVbhkoJAuwQUadPDHPaw65aZkAKZ6OE7XySwCS+Rjlhzt7fPryisvAOjUtZkwfMBuQ==`; linux-x64-baseline-musl `sha512-kwt6hmTaI6nujjAzdAkXv3j2rDzXNOMfMk8Jf8bwacHtwUl8CWA2psDVg3nWyb4zByAa6F1jSIlmHCPHBq1K/Q==`; linux-x64-musl `sha512-y6nUbH3ViajN37KaVVy0/KnWv/i48Bx5QPv9Ul9ONJya1ossV7CHVb+ZNFc61M5GqlVtjCvZV7T0eBMqhJG7mg==`; windows-arm64 `sha512-aySF/MDji5KO3I5yl9Ec4g8kW/QIx4Jj5FXwBhh/FrDu0w6L8HAOGH7cyjFb3t8TW7pxRkH3Ei3rfkWAE7cScw==`; windows-x64 `sha512-xCVbrA8A/qQTAdBIKyhK5tx3/mcq18yF82Mi8phry0MJb2631nnnpxF9m6FFfWAVHyer4qFGrO7hbWXesHQ1Gg==`; windows-x64-baseline `sha512-yRHN+TsfLjfLmdQVAVWqTGfGP1yq8YtUFy9Jw33XhlPIp77slBdCJrj2PC4amuEl6BJtaIQDhDFbpJ8+a5z7Qw==` |
| Signature | None checked. npm integrity proves the bytes match what the registry serves. It does not prove the upstream build. No GitHub release checksum could be compared (GitHub access is blocked). |

**What the package does (read before running anything).**
- `opencode-ai/package.json`: `bin.opencode = ./bin/opencode.exe`, `scripts.postinstall = node ./postinstall.mjs`, 12 platform `optionalDependencies` at exactly 1.19.5, and no regular dependencies.
- `bin/opencode.exe` is a 479-byte **shell stub**. It only prints "postinstall script was not run".
- `postinstall.mjs` picks the platform package by checking `/proc/cpuinfo` for avx2 and `ldd --version` for musl. It hard-links or copies `opencode-<plat>/bin/opencode` over `bin/opencode.exe` and runs `--version` to check it.
  - If no platform package is installed, it **runs `npm install --ignore-scripts` of `<pkg>@1.19.5` into an `os.tmpdir()` temporary directory** and copies the binary from there.
  - The platform packages contain only `package.json` and the binary. They have no scripts.
- Because scripts were skipped, I ran the platform binary **directly**: `install/node_modules/opencode-linux-x64/bin/opencode`. I always ran it from the dummy repo or `work/` as the cwd, never from inside `install/`.

## 2. Test matrix

Unless a row says otherwise, the setup was:
- Environment: `env -i` with PATH=/usr/bin:/bin, a scratch HOME/XDG/TMPDIR, and HTTP(S)_PROXY set to the local sinkhole.
- Opt-outs set: `OPENCODE_DISABLE_{AUTOUPDATE,MODELS_FETCH,LSP_DOWNLOAD,DEFAULT_PLUGINS,CLAUDE_CODE,EXTERNAL_SKILLS}=1`.
- A user config with the stub provider and `permission: ask`.

The Evidence column gives the log name under `logs/`. Verbatim JSON-RPC is in the appendix.

| # | Test | Result | Evidence and notes |
|---|---|---|---|
| 1 | `initialize` | PASS | protocolVersion 1. `agentCapabilities`: loadSession, mcp http+sse, prompt image+embeddedContext, `sessionCapabilities` close/fork/list/resume. `authMethods` is `[{"id":"opencode-login"}]` (a terminal login; not needed with a configured provider). (01, 10) |
| 2 | `session/new` | PASS | Returns sessionId and `configOptions`: **model** (lists the built-in `opencode/*-free` hosted models plus configured ones) and **mode** (build/plan, plus any custom primary agents). (10) |
| 3 | Streaming | PASS | `agent_message_chunk` per token. `tool_call` (pending) is followed by `tool_call_update` in_progress then completed or failed, with `kind` read/edit/execute/think/other, `locations` (absolute paths), `rawInput`, `rawOutput`. `available_commands_update` arrives at the start of each turn. No `plan` or `agent_thought_chunk` was seen: the stub sends no reasoning or todos, so this is **untested**. (11, 16, 17) |
| 4 | `session/request_permission` options | PASS | Always `[{"optionId":"once","kind":"allow_once","name":"Allow once"},{"optionId":"always","kind":"allow_always","name":"Always allow"},{"optionId":"reject","kind":"reject_once","name":"Reject"}]`. There is no reject_always. The request comes **after** the `in_progress` update. Edit and write requests carry `content:[{type:"diff",path,oldText,newText}]` (whole file) plus `rawInput.diff` (a unified diff). Bash requests carry `rawInput.command`. (11, 12) |
| 5 | Allow once / Reject | PASS | Allow once runs the tool. Reject gives tool status `failed` with the text "The user rejected permission to use this specific tool call.", and the turn ends `end_turn` without another model call. (11, 12) |
| 6 | Unknown optionId | PASS (safe) | `"made-up-option"` is treated as a reject. (15) |
| 7 | Permission never answered | **FAIL (hangs)** | No timeout: the turn was still open after 240 s, with no keep-alive and no notice. (13) |
| 8 | Cancel while a permission request is open | PASS, with a gap | `session/cancel` → the tool fails with "Tool execution aborted" and `stopReason:"cancelled"` arrives within 0.35 s. The agent **never withdraws** its outstanding `request_permission` (id 0). The client must close its own card. (14) |
| 9 | `allow_always` scope | Surprise | It covers the rest of that process (the second prompt was not asked). It is **not persisted** across a restart. The pattern stored is broader than the command shown: `always:["echo *"]` for `echo probe-ok`. (18, 19, 26-permission-list) |
| 10 | Diffs match the disk | PASS, with a quirk | The permission diff (`oldText` "alpha\nbeta\n" → "ALPHA-EDITED\nbeta\n") equals `git diff`. The *completed* update's diff is only the snippet (`oldText:"alpha"`, `newText:"ALPHA-EDITED"`). Use the permission diff or `rawOutput.metadata.filediff`. (17) |
| 11 | Client fs methods | Surprise | The agent **always writes the disk itself** and *also* sends `fs/write_text_file` with the new content, even when the client advertised `writeTextFile:false` (a spec violation). It never used `fs/read_text_file`. Client fs is a mirror, not a gate. (11, 17) |
| 12 | Terminal client methods | N/A | `terminal/create` exists in the binary but was never called. Bash runs internally, and its output arrives in `tool_call_update` content. (16) |
| 13 | `session/cancel` while streaming | PASS | `cancelled` arrives in 70 ms and the stub saw the HTTP stream dropped. (30) |
| 14 | `session/cancel` during a running command (`sleep 45`) | PASS, with a quirk | `cancelled` arrives in 70 ms. The `sleep` child was killed and no processes were left over. The tool is reported `completed` (not failed) with "User aborted the command". (31) |
| 15 | Provider error (HTTP 500 with the marker) | PASS, but slow | The JSON-RPC **error** reply carries the text: `{"code":-32603,"message":"Internal error: STUB-INJECTED-FAILURE-7f3a","data":{"service":"session","errorName":"APIError"}}`. It came after **~74 s of silent retries** (about 7 attempts with backoff; no ACP update meanwhile, retries only in the log). No need to read OpenCode's SQLite. (23, 24) |
| 16 | Tool error | PASS | `status:"failed"`, `rawOutput.error:"File not found: …"`. (22) |
| 17 | Subagent (`task`) | **FAIL (hangs)** | `tool_call kind:"think"` and then nothing more. The subagent's bash permission is **not sent over ACP**: it is visible only at `GET /permission` on the HTTP API, under a child session. The turn hangs until cancelled. `permission.task:"deny"` removes the tool (the model then gets "unavailable tool 'task'"). (32, 34, 95) |
| 18 | `session/load` after `kill -9` | PASS | Full replay before the result: `user_message_chunk`, tool_call plus completed update, agent chunks. The model gets the whole history on the next prompt. (40, 41) |
| 19 | `session/load` after `kill -9` *during a pending permission* | Gap | The tool is replayed as `in_progress` for ever. The permission is lost and not asked again, and the turn is not resumed. A new prompt works. (42, 43) |
| 20 | `session/resume` | PASS | Returns configOptions **without** a history replay. (44) |
| 21 | cwd | PASS | `session/new cwd` is used: bash `pwd` and `PWD` show the dummy repo, and the system prompt contains "Working directory: …". |
| 22 | Environment given to tools | **Risk** | Bash gets the agent's *entire* environment plus `OPENCODE=1`, `AGENT=1`, `OPENCODE_CLIENT=acp`, `OPENCODE_PID`. A test variable `SECRET_SHOULD_LEAK` reached the tool, and so does `OPENCODE_SERVER_PASSWORD`. (33, 86) |
| 23 | Writes outside the cwd | Partly guarded | The write tool asks `external_directory` first (kind `other`), then edit. Bash can write anywhere with only the normal bash approval (the command text is shown). The agent also writes into the repo's **`.git`**: `.git/opencode` (a project id) and loose objects in `.git/objects` (its snapshot repo uses `alternates` pointing at them). (20, 21) |
| 24 | Writes outside the cwd and HOME (the process itself) | PASS (as far as I could observe) | Writes seen: `$XDG_CONFIG_HOME/opencode/{opencode.jsonc,.gitignore}`, `$XDG_DATA_HOME/opencode/{opencode.db*,log/,snapshot/,repos/}`, `$XDG_STATE_HOME/opencode/locks/`, Bun shared objects in `$TMPDIR`, and the repo's `.git`. A `find / -newer` sweep found nothing else I could attribute to OpenCode. Other activity on the host was writing at the same time, and there was no strace, so this is a best effort. |
| 25 | Logging | Info | `$XDG_DATA_HOME/opencode/log/opencode.log` (plain key=value). It contains paths, permission decisions with the **bash command text**, and provider error text. It did not contain prompt text, the API key or the server password. `--print-logs` sends it to stderr. ACP stderr was otherwise empty. |
| 26 | Phoning home | **Yes unless opted out** | Without opt-outs: `models.opencode.ai:443` (the models catalogue) and `registry.npmjs.org:443`. With all opt-outs, still **`registry.npmjs.org`**: a "background dependency install" of `@opencode-ai/plugin` into `~/.config/opencode` and into the project's `.opencode/`. No other host was seen. No non-loopback socket was seen in /proc sampling (0.3 s interval). |
| 27 | Local HTTP server | **Security risk** | Every `opencode acp` listens on **127.0.0.1:4096**, or a random port if 4096 is taken (`--port` says default 0, but it binds 4096 first). The ACP layer itself talks to it over loopback. **No authentication by default.** The API covers sessions of *every* project in HOME, config PATCH, `/auth/{provider}` PUT, `/pty` (shell), and `/permission/{id}/reply`. (02, 03, openapi.json) |
| 28 | Approval spoofing over HTTP | **FAIL** | While the ACP client sat on an unanswered request, `POST /permission/<id>/reply {"reply":"once"}` from another process returned `true` and the command ran. The ACP request was never resolved or withdrawn. The same worked with forged `Host`/`Origin: evil.example:<port>` headers (no Host check, so a DNS-rebinding attack is plausible; not tried from a real browser). A `text/plain` simple cross-site POST gets 415, and CORS sends no ACAO for foreign origins. (26, 91, 92) |
| 29 | `OPENCODE_SERVER_PASSWORD` | PASS, with a caveat | With it set: 401 without auth, 200 with basic auth `opencode:<pw>`, and ACP keeps working. But the password **is inherited by every bash tool**, so a command the user approved once can read it and approve later requests itself. (27, 86) |
| 30 | Shared daemon vs standalone | Standalone | Each `opencode acp` is its own process and server, with no daemon. `--standalone` and `--auto` **do not exist** for `acp` in 1.19.5. An unknown flag prints help on stderr and the process exits 1 (initialize never answers). Concurrent processes share the HOME SQLite db and see each other's sessions over HTTP. (25, 90) |
| 31 | Default permission behaviour | **FAIL (no prompts)** | With a provider-only config (no `permission` key): write, edit and bash **all ran without any `request_permission`**. (50-default-*) |
| 32 | Model listing | See section 5 | |
| 33 | Self-update | Not observed | `upgrade` is a separate command. No update traffic was seen with `OPENCODE_DISABLE_AUTOUPDATE=1` plus `autoupdate:false`. Without them, only the models and npm hosts were seen in init-only runs. |
| 34 | `plan` / thought chunks, images, MCP, terminal | UNTESTED | The stub does not produce them. |

## 3. Permission config precedence

All with the stub, a bash prompt and a client that never answers. "Asked" means a `request_permission` arrived; "ran" means the command executed without one. Logs are 51–80.

| Setup | Outcome |
|---|---|
| No permission config anywhere | ran (31) |
| Project `opencode.json` `permission: ask` | asked |
| User (HOME) config ask, project config `allow` | **ran**: project beats user |
| `OPENCODE_CONFIG_CONTENT` top-level ask, project top-level allow | asked |
| `OPENCODE_CONFIG_CONTENT` ask, project `.opencode/opencode.json` allow | asked |
| `OPENCODE_CONFIG=<file>` ask, project allow | **ran**: the file loads below the project |
| `OPENCODE_CONFIG_CONTENT` ask plus `OPENCODE_PERMISSION={"bash":"allow"}` | **ran**: the env var beats CONFIG_CONTENT |
| `OPENCODE_CONFIG_CONTENT` top-level ask, project `agent.build.permission` allow (JSON, or `.opencode/agent/build.md`) | **ran**: agent-level rules beat top-level |
| Same, user-level `agent.build.permission` allow | **ran** |
| `OPENCODE_PERMISSION` ask, project or user agent-level allow | **ran** |
| Top-level `permission` set to ask, pattern rules `bash: {"echo *":"allow"}` in the project (top-level or agent-level) | asked |
| reet `OPENCODE_CONFIG_CONTENT` that sets ask **at top level and in `agent.{build,plan,general,explore}.permission`** | asked against project JSON agent, project md agent, user agent and `OPENCODE_PERMISSION=allow` |
| Same, project sets `default_agent:"yolo"` with its own allow-all primary agent | **ran** (mode currentValue = yolo) |
| Same plus `"default_agent":"build"` in reet's config, *or* `OPENCODE_DISABLE_PROJECT_CONFIG=1` | asked (mode = build) |
| **Final reet config** (`scripts/reet-final.json`: the above plus `task: deny`, `webfetch: deny`, `default_agent: build`) with `OPENCODE_DISABLE_PROJECT_CONFIG=1`, a hostile user config, a hostile project config and `OPENCODE_PERMISSION={"bash":"allow"}` | asked for write, edit, bash and bash-outside-cwd (96-final-*) |

**Project plugins run regardless.** A harmless `.opencode/plugin/marker.js` (and `.opencode/plugins/`) in the repo was **executed when the session started, without any prompt**. It still ran with `OPENCODE_DISABLE_PROJECT_CONFIG=1`, with `OPENCODE_PURE=1`, and with `acp --pure` (81–85). OpenCode also tries to install npm dependencies into the project's `.opencode/` (blocked here by the sinkhole). **Opening an OpenCode session on a repository runs that repository's code.** I found no flag in 1.19.5 that stops it.

## 4. Surprises (short list)
1. By default nothing is asked. A project can switch prompts off through `agent.*.permission` or `default_agent`. `OPENCODE_PERMISSION` beats the explicit config.
2. An unauthenticated local HTTP control API (shell, auth, permission replies) runs inside every ACP process, on a predictable port.
3. A permission approved over HTTP leaves the ACP request dangling. The ACP card would still show "pending" after the command has run.
4. Subagent permissions are invisible to ACP, so the turn hangs.
5. Project plugin code runs on session start, and `--pure` does not stop it.
6. Disk writes bypass the client fs and are then mirrored to it, even when the capability is false.
7. "Always allow" stores a wildcard (`echo *`) broader than the command shown.
8. A provider error takes ~74 s of silent retries before the reply.
9. Writes go into the user's `.git` (objects plus an `opencode` file).
10. The model picker offers hosted `opencode/*-free` models (opencode.ai inference) by default.
11. `opencode models --refresh` prints "Models cache refreshed" even when every fetch failed.

## 5. Model listing (`opencode models`)

All runs used `env -i`, a 10 s `timeout`, the sinkhole proxy and no credentials. Logs are in `logs/models/`.

| Run | rc | time | stdout | Outbound (sinkhole) |
|---|---|---|---|---|
| empty HOME, no opt-out | 0 | 2.1 s | 9 lines, 259 B | 2× `models.opencode.ai:443` |
| empty HOME, `OPENCODE_DISABLE_MODELS_FETCH=1` | 0 | 2.2 s | same 9 lines | none |
| stub provider config, no opt-out | 0 | 2.3 s | 10 lines, 275 B | 2× `models.opencode.ai` |
| stub config plus opt-out | 0 | 2.1 s | 10 lines | none |
| `models stub` | 0 | 2.2 s | `stub/stub-model` (16 B) | none |
| `models nosuch` | **1** | 2.1 s | empty; stderr `\e[91m\e[1mError: \e[0mProvider not found: nosuch` | none |
| `models stub --verbose` | 0 | 2.5 s | 50 lines, 854 B: the `provider/model` line, then pretty-printed JSON | none |
| `--refresh` (fresh HOME, with or without opt-out) | 0 | 3.4–4.1 s | stderr "Models cache refreshed" (**even though every fetch got 403**) | 4–7× `models.opencode.ai`, plus 1× `registry.npmjs.org` when a provider config exists. **`--refresh` ignores `OPENCODE_DISABLE_MODELS_FETCH`.** |
| `--refresh` in a HOME used by many earlier runs | **124 (hung 10 s)** | 10 s | – | none. Not reproducible: the next run in the same HOME took 4 s. The killed run left `~/.local/state/opencode/locks/*.lock` behind. |
| config `disabled_providers:["opencode"]` | 0 | 2.4 s | `stub/stub-model` only | none |
| config `enabled_providers:["stub"]` | 0 | 2.4 s | `stub/stub-model` only | none |
| `--pure` | 0 | 1.9 s | 10 lines | none |

Format: one `provider/model` per line, sorted, with no header and no colour on stdout. The models catalogue appears to be compiled in: the 9 `opencode/*-free` entries appear with the fetch disabled.

Writes: only `$XDG_CONFIG_HOME/opencode/{opencode.jsonc,.gitignore}` (created if missing), `$XDG_DATA_HOME/opencode/{opencode.db,log/}` and `$XDG_STATE_HOME/opencode/locks/`. Nothing outside the scratch HOME was seen. It did not try to update itself.

`opencode models --help` lists `--verbose` and `--refresh` (verbatim in appendix B). The opt-outs that exist in the binary and were confirmed to work: `OPENCODE_DISABLE_MODELS_FETCH` (except with `--refresh`), and the config keys `disabled_providers` / `enabled_providers`.

Verbatim stdout (empty HOME):
```
opencode/beam-free
opencode/big-pickle
opencode/deepseek-v4.1-flash-free
opencode/flint-40b-free
opencode/glm-5.3-flash-free
opencode/mimo-v2.6-flash-free
opencode/nemotron-3-ultra-free
opencode/nemotron-3.5-lightning-free
opencode/nemotron-3.5-super-vl-free
```
With the stub config, the same list plus `stub/stub-model` as line 10. The `--verbose` output for the stub is in `logs/models/m2-stub-verbose.stdout`.

## 6. Risks for reet and what the adapter must do

**Security**
- *Permission bypass by default or by repo config.* reet **must** inject its own config and must not rely on OpenCode's defaults or on the user's or project's config:
  - Set `OPENCODE_CONFIG_CONTENT` = `scripts/reet-final.json`: ask for edit, bash and external_directory at top level **and** for every built-in agent; `task: deny`; `webfetch: deny`; `default_agent: "build"`.
  - Set `OPENCODE_DISABLE_PROJECT_CONFIG=1`.
  - **Remove** `OPENCODE_PERMISSION`, `OPENCODE_CONFIG`, `OPENCODE_CONFIG_DIR` and all `OPENCODE_*` variables reet does not set itself.
  - After `session/new`, check that `configOptions.mode.currentValue == "build"`, and refuse the session otherwise. User-level custom agents can still add new primary agents, so reet must also not offer mode switching in slice 1.
- *Approval spoofing over the local HTTP API.*
  - Set a fresh random `OPENCODE_SERVER_PASSWORD` per process. This blocks other local processes and browsers.
  - It does **not** stop an approved tool from reading the password out of its own environment. Accept this for slice 1 (anything the user approves already runs as the user), and say so in the threat model.
  - Never pass `--hostname` or `--mdns`.
- *Repository code runs on session start* (project plugins). Slice 1 must be limited to workspaces the user trusts. Before starting, reet shows a warning if `<cwd>/.opencode/plugin*` or `.opencode/package.json` exists. Ideally it asks OpenCode upstream for a flag that disables project plugins.
- *Writes outside the cwd through bash.* The card must show the full command text. "Allow once" is the only allow option, so the `echo *` wildcard problem with Always allow is avoided.
- *Environment leakage.* Start the agent with an allowlist: PATH, HOME, XDG_*, TMPDIR, LANG, the chosen provider key, and reet's own `OPENCODE_*` settings. No SSH_AUTH_SOCK, GH/cloud tokens or DISPLAY. Everything in this environment reaches every tool.
- *Phoning home.* Set `OPENCODE_DISABLE_MODELS_FETCH=1`, `OPENCODE_DISABLE_AUTOUPDATE=1`, `OPENCODE_DISABLE_DEFAULT_PLUGINS=1`, `OPENCODE_DISABLE_LSP_DOWNLOAD=1`, `OPENCODE_DISABLE_CLAUDE_CODE=1`, `OPENCODE_DISABLE_EXTERNAL_SKILLS=1`, `autoupdate:false`, `share:"disabled"`, and `disabled_providers:["opencode"]` unless the user picks it. The npm install of `@opencode-ai/plugin` still happens; document it, or pre-seed or block it.

**Reliability**
- *Turns that never end*: an unanswered permission, subagent permissions, and 74 s of provider retries.
  - Run a per-request permission timeout. On expiry, answer `{"outcome":{"outcome":"cancelled"}}` and then send `session/cancel`.
  - Run a turn-silence watchdog that shows a "still working (provider retrying?)" state after about 20 s, and cancels at a configurable limit.
  - Keep `task: deny` until OpenCode forwards subagent permissions.
- *Stale cards.* On cancel, reet must close its own pending permission card, because the agent never withdraws it. On `session/load`, tools replayed as `in_progress` must be shown as "interrupted".
- *Resume.* `session/load` replay is good. reet should still keep its own short recap, in case the HOME db is lost, and it must say that pending approvals are not restored.
- *Disk.* Treat the client fs calls as notifications. Take diffs from the permission request (`content[type=diff]`, whole file) or `rawOutput.metadata.filediff`, not from the completed update.
- *Process.* One agent process per session. Kill it by PID (SIGTERM was always enough; no orphans were seen, and cancel killed child shells). An unknown flag makes the agent exit with rc 1, so reet must detect startup failure.
- *Minimum version:* **1.19.5** (the only version probed). Refuse lower versions until each is probed. Re-probe on upgrade, because permission precedence, the HTTP API and plugin loading are all behaviour that can change between versions.

## 7. Go / no-go for slice 1 (OpenCode, Allow once / Deny only)

**Conditional GO.** The protocol side is ready:
- the cards get exact diffs and commands;
- Allow once and Deny behave correctly;
- an unknown answer is treated as Deny;
- cancel, errors and load all work.

The conditions are **mandatory**. Without them the approval cards would be decoration, not protection:
1. The reet-owned permission config and environment scrubbing from section 6, with the mode check after `session/new`.
2. A random per-process `OPENCODE_SERVER_PASSWORD`.
3. `task: deny`.
4. A permission timeout plus a turn watchdog, and reet closing its own cards on cancel.
5. An environment allowlist and the opt-outs.
6. Slice 1 limited to trusted workspaces, with a warning when the repo has `.opencode/` plugins.
7. Version pinned at exactly 1.19.5.

reet's docs must stop assuming that "the agent's own prompts protect a session". For OpenCode they do so only under conditions 1–2.

**NO-GO** if slice 1 has to support untrusted repositories: project plugin code runs on session start, and I found no way to stop it.

## 8. What I could not observe
- A real browser DNS-rebinding run: I simulated it with forged Host/Origin headers.
- Plan, thought, image and MCP streaming.
- `terminal/*`.
- The `--port 0` behaviour.
- Connections made below the proxy layer, beyond 0.3 s /proc sampling: no strace. Only the sinkhole's CONNECT log and the socket sampling were available.
- A file-write sweep isolated from other activity on the host.
- The upstream build provenance, because GitHub was not reachable.

## 9. Housekeeping
- Processes started:
  - stub model server: PIDs 1267, 4573, 5084, 15252;
  - sinkhole: PIDs 4574, 15253;
  - one `opencode acp` per scenario, each killed by the client by PID.
  All were killed by PID. `ps` shows no probe processes left (the last two were briefly defunct and are now reaped).
- Nothing outside the probe directory was deleted.
- Nothing in `/home/user/reet.sh` was changed except this git-ignored report. Nothing was committed.

---

## Appendix A: verbatim JSON-RPC (from the probe logs; nothing redacted)

Full logs, one JSON line per message both ways with timestamps, are in `logs/*.jsonl`.

HTTP evidence for A17, `GET /permission` while the ACP request was pending:
```
[{"id":"per_1815411c4001BQeeff7zJOT3GY","sessionID":"ses_e7eabf1c4ffeC6AON7e1NoxxAr","permission":"bash","patterns":["echo probe-ok"],"metadata":{"command":"echo probe-ok"},"always":["echo *"],"tool":{"messageID":"msg_181540e750012Nej9Mbmcpk7XF","callID":"call_1793171132797"}}]
POST /permission/per_1815411c4001BQeeff7zJOT3GY/reply  {"reply":"once"}  -> 200 true
```
Subagent (A14), `GET /permission` during the hang (child session `ses_e7ea8bb3…`, parent `ses_e7ea8bf3…`):
```
[{"id":"per_181574575001ACKQYgUnm0mYsF","sessionID":"ses_e7ea8bb3bffeCP5PIIiOJujGUn","permission":"bash","patterns":["echo probe-ok"],"metadata":{"command":"echo probe-ok"},"always":["echo *"],"tool":{"messageID":"msg_1815744f1001bACDHjFngMzCUA","callID":"call_1793171342628"}}]
```

### A1 initialize + session/new + plain text turn (10-text)

```
c->a {"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": 1, "clientCapabilities": {"fs": {"readTextFile": false, "writeTextFile": false}, "terminal": false}, "clientInfo": {"name": "reet-probe", "version": "0"}}}
a->c {"jsonrpc": "2.0", "id": 1, "result": {"protocolVersion": 1, "agentCapabilities": {"loadSession": true, "mcpCapabilities": {"http": true, "sse": true}, "promptCapabilities": {"embeddedContext": true, "image": true}, "sessionCapabilities": {"close": {}, "fork": {}, "list": {}, "resume": {}}}, "authMethods": [{"description": "Run `opencode auth login` in the terminal", "name": "Login with opencode", "id": "opencode-login"}], "agentInfo": {"name": "OpenCode", "version": "1.19.5"}}}
c->a {"jsonrpc": "2.0", "id": 2, "method": "session/new", "params": {"cwd": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy", "mcpServers": []}}
a->c {"jsonrpc": "2.0", "id": 2, "result": {"sessionId": "ses_e7eb4535bffex3MGDkUhiZsWG0", "configOptions": [{"id": "model", "name": "Model", "category": "model", "type": "select", "currentValue": "stub/stub-model", "options": [{"value": "opencode/beam-free", "name": "OpenCode/Beam Free"}, {"value": "opencode/big-pickle", "name": "OpenCode/Big Pickle"}, {"value": "opencode/deepseek-v4.1-flash-free", "name": "OpenCode/DeepSeek V4.1 Flash Free"}, {"value": "opencode/flint-40b-free", "name": "OpenCode/Flint 40B Free"}, {"value": "opencode/glm-5.3-flash-free", "name": "OpenCode/GLM-5.3-Flash Free"}, {"value": "opencode/mimo-v2.6-flash-free", "name": "OpenCode/MiMo-V2.6-Flash Free"}, {"value": "opencode/nemotron-3-ultra-free", "name": "OpenCode/Nemotron 3 Ultra Free"}, {"value": "opencode/nemotron-3.5-lightning-free", "name": "OpenCode/Nemotron 3.5 Lightning Free"}, {"value": "opencode/nemotron-3.5-super-vl-free", "name": "OpenCode/Nemotron 3.5 Super VL Free"}, {"value": "stub/stub-model", "name": "Local stub/stub-model"}]}, {"id": "mode", "name": "Session Mode", "category": "mode", "type": "select", "currentValue": "build", "options": [{"value": "build", "name": "build", "description": "The default agent. Executes tools based on configured permissions."}, {"value": "plan", "name": "plan", "description": "Plan mode. Disallows all edit tools."}]}]}}
c->a {"jsonrpc": "2.0", "id": 3, "method": "session/prompt", "params": {"sessionId": "ses_e7eb4535bffex3MGDkUhiZsWG0", "prompt": [{"type": "text", "text": "hello there"}]}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eb4535bffex3MGDkUhiZsWG0", "update": {"sessionUpdate": "agent_message_chunk", "messageId": "msg_1814bacd4001jrIOoWiwZEM64y", "content": {"type": "text", "text": "STUB-REPLY: "}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eb4535bffex3MGDkUhiZsWG0", "update": {"sessionUpdate": "agent_message_chunk", "messageId": "msg_1814bacd4001jrIOoWiwZEM64y", "content": {"type": "text", "text": "hello "}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eb4535bffex3MGDkUhiZsWG0", "update": {"sessionUpdate": "agent_message_chunk", "messageId": "msg_1814bacd4001jrIOoWiwZEM64y", "content": {"type": "text", "text": "there "}}}}
a->c {"jsonrpc": "2.0", "id": 3, "result": {"stopReason": "end_turn", "usage": {"inputTokens": 1, "outputTokens": 1, "totalTokens": 2}, "_meta": {}}}
```

### A2 write, Allow once: permission request with diff, client fs mirror call, completion (11-write-allow)

```
c->a {"jsonrpc": "2.0", "id": 2, "method": "session/new", "params": {"cwd": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy", "mcpServers": []}}
a->c {"jsonrpc": "2.0", "id": 2, "result": {"sessionId": "ses_e7eb39339ffelhJHz2QW1iCmJ3", "configOptions": [{"id": "model", "name": "Model", "category": "model", "type": "select", "currentValue": "stub/stub-model", "options": [{"value": "opencode/beam-free", "name": "OpenCode/Beam Free"}, {"value": "opencode/big-pickle", "name": "OpenCode/Big Pickle"}, {"value": "opencode/deepseek-v4.1-flash-free", "name": "OpenCode/DeepSeek V4.1 Flash Free"}, {"value": "opencode/flint-40b-free", "name": "OpenCode/Flint 40B Free"}, {"value": "opencode/glm-5.3-flash-free", "name": "OpenCode/GLM-5.3-Flash Free"}, {"value": "opencode/mimo-v2.6-flash-free", "name": "OpenCode/MiMo-V2.6-Flash Free"}, {"value": "opencode/nemotron-3-ultra-free", "name": "OpenCode/Nemotron 3 Ultra Free"}, {"value": "opencode/nemotron-3.5-lightning-free", "name": "OpenCode/Nemotron 3.5 Lightning Free"}, {"value": "opencode/nemotron-3.5-super-vl-free", "name": "OpenCode/Nemotron 3.5 Super VL Free"}, {"value": "stub/stub-model", "name": "Local stub/stub-model"}]}, {"id": "mode", "name": "Session Mode", "category": "mode", "type": "select", "currentValue": "build", "options": [{"value": "build", "name": "build", "description": "The default agent. Executes tools based on configured permissions."}, {"value": "plan", "name": "plan", "description": "Plan mode. Disallows all edit tools."}]}]}}
c->a {"jsonrpc": "2.0", "id": 3, "method": "session/prompt", "params": {"sessionId": "ses_e7eb39339ffelhJHz2QW1iCmJ3", "prompt": [{"type": "text", "text": "WRITE a file"}]}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eb39339ffelhJHz2QW1iCmJ3", "update": {"sessionUpdate": "tool_call", "toolCallId": "call_1793170632566", "title": "write", "kind": "edit", "status": "pending", "locations": [], "rawInput": {}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eb39339ffelhJHz2QW1iCmJ3", "update": {"sessionUpdate": "tool_call_update", "toolCallId": "call_1793170632566", "status": "in_progress", "kind": "edit", "title": "write", "locations": [{"path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/hello.txt"}], "rawInput": {"filePath": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/hello.txt", "content": "hello from stub\n"}}}}
a->c {"jsonrpc": "2.0", "id": 0, "method": "session/request_permission", "params": {"sessionId": "ses_e7eb39339ffelhJHz2QW1iCmJ3", "toolCall": {"toolCallId": "call_1793170632566", "title": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/hello.txt", "kind": "edit", "status": "pending", "locations": [{"path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/hello.txt"}], "rawInput": {"filepath": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/hello.txt", "diff": "Index: /tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/hello.txt\n===================================================================\n--- /tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/hello.txt\n+++ /tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/hello.txt\n@@ -0,0 +1,1 @@\n+hello from stub\n"}, "content": [{"type": "diff", "path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/hello.txt", "oldText": "", "newText": "hello from stub\n"}]}, "options": [{"optionId": "once", "kind": "allow_once", "name": "Allow once"}, {"optionId": "always", "kind": "allow_always", "name": "Always allow"}, {"optionId": "reject", "kind": "reject_once", "name": "Reject"}]}}
c->a {"jsonrpc": "2.0", "id": 0, "result": {"outcome": {"outcome": "selected", "optionId": "once"}}}
a->c {"jsonrpc": "2.0", "id": 1, "method": "fs/write_text_file", "params": {"sessionId": "ses_e7eb39339ffelhJHz2QW1iCmJ3", "path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/hello.txt", "content": "hello from stub\n"}}
c->a {"jsonrpc": "2.0", "id": 1, "result": null}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eb39339ffelhJHz2QW1iCmJ3", "update": {"sessionUpdate": "tool_call_update", "toolCallId": "call_1793170632566", "status": "completed", "title": "hello.txt", "content": [{"type": "content", "content": {"type": "text", "text": "Wrote file successfully."}}], "rawOutput": {"output": "Wrote file successfully.", "metadata": {"diagnostics": {}, "filepath": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/hello.txt", "exists": false, "truncated": false}}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eb39339ffelhJHz2QW1iCmJ3", "update": {"sessionUpdate": "agent_message_chunk", "messageId": "msg_1814c6fee0011CjrYVvAV29Rze", "content": {"type": "text", "text": "STUB-DONE "}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eb39339ffelhJHz2QW1iCmJ3", "update": {"sessionUpdate": "agent_message_chunk", "messageId": "msg_1814c6fee0011CjrYVvAV29Rze", "content": {"type": "text", "text": "after "}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eb39339ffelhJHz2QW1iCmJ3", "update": {"sessionUpdate": "agent_message_chunk", "messageId": "msg_1814c6fee0011CjrYVvAV29Rze", "content": {"type": "text", "text": "1 "}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eb39339ffelhJHz2QW1iCmJ3", "update": {"sessionUpdate": "agent_message_chunk", "messageId": "msg_1814c6fee0011CjrYVvAV29Rze", "content": {"type": "text", "text": "tool(s): "}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eb39339ffelhJHz2QW1iCmJ3", "update": {"sessionUpdate": "agent_message_chunk", "messageId": "msg_1814c6fee0011CjrYVvAV29Rze", "content": {"type": "text", "text": "Wrote "}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eb39339ffelhJHz2QW1iCmJ3", "update": {"sessionUpdate": "agent_message_chunk", "messageId": "msg_1814c6fee0011CjrYVvAV29Rze", "content": {"type": "text", "text": "file "}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eb39339ffelhJHz2QW1iCmJ3", "update": {"sessionUpdate": "agent_message_chunk", "messageId": "msg_1814c6fee0011CjrYVvAV29Rze", "content": {"type": "text", "text": "successfully. "}}}}
a->c {"jsonrpc": "2.0", "id": 3, "result": {"stopReason": "end_turn", "usage": {"inputTokens": 1, "outputTokens": 1, "totalTokens": 2}, "_meta": {}}}
```

### A3 bash, Reject (12-bash-reject)

```
a->c {"jsonrpc": "2.0", "id": 0, "method": "session/request_permission", "params": {"sessionId": "ses_e7eb354efffeikOBuDNuwMoL2C", "toolCall": {"toolCallId": "call_1793170648643", "title": "echo probe-ok", "kind": "execute", "status": "pending", "locations": [], "rawInput": {"command": "echo probe-ok"}}, "options": [{"optionId": "once", "kind": "allow_once", "name": "Allow once"}, {"optionId": "always", "kind": "allow_always", "name": "Always allow"}, {"optionId": "reject", "kind": "reject_once", "name": "Reject"}]}}
c->a {"jsonrpc": "2.0", "id": 0, "result": {"outcome": {"outcome": "selected", "optionId": "reject"}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eb354efffeikOBuDNuwMoL2C", "update": {"sessionUpdate": "tool_call_update", "toolCallId": "call_1793170648643", "status": "failed", "kind": "execute", "title": "echo probe-ok", "locations": [{"path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy"}], "rawInput": {"command": "echo probe-ok", "description": "harmless echo", "cwd": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy"}, "content": [{"type": "content", "content": {"type": "text", "text": "The user rejected permission to use this specific tool call."}}], "rawOutput": {"error": "The user rejected permission to use this specific tool call."}}}}
a->c {"jsonrpc": "2.0", "id": 3, "result": {"stopReason": "end_turn", "usage": {"inputTokens": 1, "outputTokens": 1, "totalTokens": 2}, "_meta": {}}}
```

### A4 permission never answered: no timeout, turn still open after 240 s (13-perm-never) - last messages

```
a->c {"jsonrpc": "2.0", "id": 0, "method": "session/request_permission", "params": {"sessionId": "ses_e7eb3240dffeVaDji9k8lYMBi5", "toolCall": {"toolCallId": "call_1793170661177", "title": "echo probe-ok", "kind": "execute", "status": "pending", "locations": [], "rawInput": {"command": "echo probe-ok"}}, "options": [{"optionId": "once", "kind": "allow_once", "name": "Allow once"}, {"optionId": "always", "kind": "allow_always", "name": "Always allow"}, {"optionId": "reject", "kind": "reject_once", "name": "Reject"}]}}
```

### A5 session/cancel while a permission request is open (14-perm-cancel)

```
a->c {"jsonrpc": "2.0", "id": 0, "method": "session/request_permission", "params": {"sessionId": "ses_e7eaf4e39ffeQ8bVpOnGV2eX1X", "toolCall": {"toolCallId": "call_1793170912453", "title": "echo probe-ok", "kind": "execute", "status": "pending", "locations": [], "rawInput": {"command": "echo probe-ok"}}, "options": [{"optionId": "once", "kind": "allow_once", "name": "Allow once"}, {"optionId": "always", "kind": "allow_always", "name": "Always allow"}, {"optionId": "reject", "kind": "reject_once", "name": "Reject"}]}}
c->a {"jsonrpc": "2.0", "method": "session/cancel", "params": {"sessionId": "ses_e7eaf4e39ffeQ8bVpOnGV2eX1X"}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eaf4e39ffeQ8bVpOnGV2eX1X", "update": {"sessionUpdate": "tool_call_update", "toolCallId": "call_1793170912453", "status": "failed", "kind": "execute", "title": "echo probe-ok", "locations": [{"path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy"}], "rawInput": {"command": "echo probe-ok", "description": "harmless echo", "cwd": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy"}, "content": [{"type": "content", "content": {"type": "text", "text": "Tool execution aborted"}}], "rawOutput": {"error": "Tool execution aborted", "metadata": {"interrupted": true}}}}}
a->c {"jsonrpc": "2.0", "id": 3, "result": {"stopReason": "cancelled", "usage": {"inputTokens": 0, "outputTokens": 0, "totalTokens": 0}, "_meta": {}}}
```

### A6 made-up optionId is treated as reject (15-perm-wrongid)

```
c->a {"jsonrpc": "2.0", "id": 0, "result": {"outcome": {"outcome": "selected", "optionId": "made-up-option"}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eaf34f1ffeG05f7z3KX5l4lV", "update": {"sessionUpdate": "tool_call_update", "toolCallId": "call_1793170918894", "status": "failed", "kind": "execute", "title": "echo probe-ok", "locations": [{"path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy"}], "rawInput": {"command": "echo probe-ok", "description": "harmless echo", "cwd": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy"}, "content": [{"type": "content", "content": {"type": "text", "text": "The user rejected permission to use this specific tool call."}}], "rawOutput": {"error": "The user rejected permission to use this specific tool call."}}}}
a->c {"jsonrpc": "2.0", "id": 3, "result": {"stopReason": "end_turn", "usage": {"inputTokens": 1, "outputTokens": 1, "totalTokens": 2}, "_meta": {}}}
```

### A7 edit: permission diff vs completion diff, fs/write_text_file (17-edit)

```
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eaed04dffejOKsIl4MhNzp4z", "update": {"sessionUpdate": "tool_call_update", "toolCallId": "call_1793170944634", "status": "completed", "title": "notes.txt", "content": [{"type": "content", "content": {"type": "text", "text": "alpha\nbeta"}}], "rawOutput": {"output": "<path>/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/notes.txt</path>\n<type>file</type>\n<content>\n1: alpha\n2: beta\n\n(End of file - total 2 lines)\n</content>", "metadata": {"preview": "alpha\nbeta", "truncated": false, "loaded": [], "display": {"type": "file", "path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/notes.txt", "text": "alpha\nbeta", "lineStart": 1, "lineEnd": 2, "totalLines": 2, "truncated": false}}}}}}
a->c {"jsonrpc": "2.0", "id": 0, "method": "session/request_permission", "params": {"sessionId": "ses_e7eaed04dffejOKsIl4MhNzp4z", "toolCall": {"toolCallId": "call_1793170944759", "title": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/notes.txt", "kind": "edit", "status": "pending", "locations": [{"path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/notes.txt"}], "rawInput": {"filepath": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/notes.txt", "diff": "Index: /tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/notes.txt\n===================================================================\n--- /tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/notes.txt\n+++ /tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/notes.txt\n@@ -1,2 +1,2 @@\n-alpha\n+ALPHA-EDITED\n beta\n"}, "content": [{"type": "diff", "path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/notes.txt", "oldText": "alpha\nbeta\n", "newText": "ALPHA-EDITED\nbeta\n"}]}, "options": [{"optionId": "once", "kind": "allow_once", "name": "Allow once"}, {"optionId": "always", "kind": "allow_always", "name": "Always allow"}, {"optionId": "reject", "kind": "reject_once", "name": "Reject"}]}}
a->c {"jsonrpc": "2.0", "id": 1, "method": "fs/write_text_file", "params": {"sessionId": "ses_e7eaed04dffejOKsIl4MhNzp4z", "path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/notes.txt", "content": "ALPHA-EDITED\nbeta\n"}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eaed04dffejOKsIl4MhNzp4z", "update": {"sessionUpdate": "tool_call_update", "toolCallId": "call_1793170944759", "status": "completed", "title": "notes.txt", "content": [{"type": "content", "content": {"type": "text", "text": "Edit applied successfully."}}, {"type": "diff", "path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/notes.txt", "oldText": "alpha", "newText": "ALPHA-EDITED"}], "rawOutput": {"output": "Edit applied successfully.", "metadata": {"diagnostics": {}, "diff": "Index: /tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/notes.txt\n===================================================================\n--- /tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/notes.txt\n+++ /tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/notes.txt\n@@ -1,2 +1,2 @@\n-alpha\n+ALPHA-EDITED\n beta\n", "filediff": {"file": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/notes.txt", "patch": "Index: /tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/notes.txt\n===================================================================\n--- /tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/notes.txt\n+++ /tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/notes.txt\n@@ -1,2 +1,2 @@\n-alpha\n+ALPHA-EDITED\n beta\n", "additions": 1, "deletions": 1}, "truncated": false}}}}}
a->c {"jsonrpc": "2.0", "id": 3, "result": {"stopReason": "end_turn", "usage": {"inputTokens": 1, "outputTokens": 1, "totalTokens": 2}, "_meta": {}}}
```

### A8 write outside cwd: external_directory request then edit request (20-outside-write)

```
a->c {"jsonrpc": "2.0", "id": 0, "method": "session/request_permission", "params": {"sessionId": "ses_e7eadcb43ffeHt3h01e3FELt3f", "toolCall": {"toolCallId": "call_1793171011489", "title": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/outside-target", "kind": "other", "status": "pending", "locations": [{"path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/outside-target/outside-write.txt"}, {"path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/outside-target"}], "rawInput": {"filepath": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/outside-target/outside-write.txt", "parentDir": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/outside-target"}}, "options": [{"optionId": "once", "kind": "allow_once", "name": "Allow once"}, {"optionId": "always", "kind": "allow_always", "name": "Always allow"}, {"optionId": "reject", "kind": "reject_once", "name": "Reject"}]}}
c->a {"jsonrpc": "2.0", "id": 0, "result": {"outcome": {"outcome": "selected", "optionId": "once"}}}
a->c {"jsonrpc": "2.0", "id": 1, "method": "session/request_permission", "params": {"sessionId": "ses_e7eadcb43ffeHt3h01e3FELt3f", "toolCall": {"toolCallId": "call_1793171011489", "title": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/outside-target/outside-write.txt", "kind": "edit", "status": "pending", "locations": [{"path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/outside-target/outside-write.txt"}], "rawInput": {"filepath": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/outside-target/outside-write.txt", "diff": "Index: /tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/outside-target/outside-write.txt\n===================================================================\n--- /tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/outside-target/outside-write.txt\n+++ /tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/outside-target/outside-write.txt\n@@ -0,0 +1,1 @@\n+outside\n"}, "content": [{"type": "diff", "path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/outside-target/outside-write.txt", "oldText": "", "newText": "outside\n"}]}, "options": [{"optionId": "once", "kind": "allow_once", "name": "Allow once"}, {"optionId": "always", "kind": "allow_always", "name": "Always allow"}, {"optionId": "reject", "kind": "reject_once", "name": "Reject"}]}}
c->a {"jsonrpc": "2.0", "id": 1, "result": {"outcome": {"outcome": "selected", "optionId": "once"}}}
```

### A9 bash writing outside cwd: only the ordinary bash request (21-outside-bash)

```
a->c {"jsonrpc": "2.0", "id": 0, "method": "session/request_permission", "params": {"sessionId": "ses_e7eadb70bffeJy7ox8v114v2SW", "toolCall": {"toolCallId": "call_1793171016633", "title": "echo via-bash > /tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/outside-target/outside-bash.txt; echo wrote", "kind": "execute", "status": "pending", "locations": [], "rawInput": {"command": "echo via-bash > /tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/outside-target/outside-bash.txt; echo wrote"}}, "options": [{"optionId": "once", "kind": "allow_once", "name": "Allow once"}, {"optionId": "always", "kind": "allow_always", "name": "Always allow"}, {"optionId": "reject", "kind": "reject_once", "name": "Reject"}]}}
```

### A10 provider HTTP 500: error reply after ~74 s of silent retries (24-http500-long)

```
c->a {"jsonrpc": "2.0", "id": 3, "method": "session/prompt", "params": {"sessionId": "ses_e7eac5f52ffeumEEXo0pybyBEG", "prompt": [{"type": "text", "text": "FAIL now"}]}}
a->c {"jsonrpc": "2.0", "id": 3, "error": {"code": -32603, "message": "Internal error: STUB-INJECTED-FAILURE-7f3a", "data": {"service": "session", "errorName": "APIError"}}}
```

### A11 tool failure (22-toolfail)

```
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eada278ffelKsyBYJpmayk2Q", "update": {"sessionUpdate": "tool_call_update", "toolCallId": "call_1793171021900", "status": "failed", "kind": "read", "title": "read", "locations": [{"path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/does-not-exist.txt"}], "rawInput": {"filePath": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/does-not-exist.txt"}, "content": [{"type": "content", "content": {"type": "text", "text": "File not found: /tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/does-not-exist.txt"}}], "rawOutput": {"error": "File not found: /tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/does-not-exist.txt"}}}}
```

### A12 cancel during streaming (30-slow-cancel)

```
c->a {"jsonrpc": "2.0", "method": "session/cancel", "params": {"sessionId": "ses_e7eaa9b99ffe2O7dWgykpXOUvt"}}
a->c {"jsonrpc": "2.0", "id": 3, "result": {"stopReason": "cancelled", "usage": {"inputTokens": 0, "outputTokens": 0, "totalTokens": 0}, "_meta": {}}}
```

### A13 cancel during running shell command (31-sleepbash-cancel)

```
c->a {"jsonrpc": "2.0", "method": "session/cancel", "params": {"sessionId": "ses_e7eaa729dffefGMkhYla8OunJr"}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eaa729dffefGMkhYla8OunJr", "update": {"sessionUpdate": "tool_call_update", "toolCallId": "call_1793171231377", "status": "completed", "title": "sleep 45; echo slept", "content": [{"type": "content", "content": {"type": "text", "text": "(no output)\n\n<shell_metadata>\nUser aborted the command\n</shell_metadata>"}}], "rawOutput": {"output": "(no output)\n\n<shell_metadata>\nUser aborted the command\n</shell_metadata>", "metadata": {"output": "(no output)\n\n<shell_metadata>\nUser aborted the command\n</shell_metadata>", "exit": null, "truncated": false}}}}}
a->c {"jsonrpc": "2.0", "id": 3, "result": {"stopReason": "cancelled", "usage": {"inputTokens": 0, "outputTokens": 0, "totalTokens": 0}, "_meta": {}}}
```

### A14 subagent task: no permission request reaches ACP, turn hangs (34-task-http)

```
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7ea8bf3affeAbo3OfTkzQo6xw", "update": {"sessionUpdate": "tool_call", "toolCallId": "call_1793171342506", "title": "task", "kind": "think", "status": "pending", "locations": [], "rawInput": {}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7ea8bf3affeAbo3OfTkzQo6xw", "update": {"sessionUpdate": "tool_call_update", "toolCallId": "call_1793171342506", "status": "in_progress", "kind": "think", "title": "sub job", "locations": [], "rawInput": {"description": "sub job", "prompt": "BASH please", "subagent_type": "general"}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7ea8bf3affeAbo3OfTkzQo6xw", "update": {"sessionUpdate": "tool_call_update", "toolCallId": "call_1793171342506", "status": "in_progress", "kind": "think", "title": "sub job", "locations": [], "rawInput": {"description": "sub job", "prompt": "BASH please", "subagent_type": "general"}}}}
```

### A15 session/load after kill -9: history replay (41-load)

```
c->a {"jsonrpc": "2.0", "id": 2, "method": "session/load", "params": {"sessionId": "ses_e7ea7e547ffe5f9AyazZCosNef", "cwd": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy", "mcpServers": []}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7ea7e547ffe5f9AyazZCosNef", "update": {"sessionUpdate": "user_message_chunk", "messageId": "msg_181581acd0010DNpDlQThpbTUW", "content": {"type": "text", "text": "WRITE a file"}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7ea7e547ffe5f9AyazZCosNef", "update": {"sessionUpdate": "tool_call", "toolCallId": "call_1793171398155", "title": "hello.txt", "kind": "edit", "status": "pending", "locations": [{"path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/hello.txt"}], "rawInput": {"filePath": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/hello.txt", "content": "hello from stub\n"}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7ea7e547ffe5f9AyazZCosNef", "update": {"sessionUpdate": "tool_call_update", "toolCallId": "call_1793171398155", "status": "completed", "title": "hello.txt", "content": [{"type": "content", "content": {"type": "text", "text": "Wrote file successfully."}}], "rawOutput": {"output": "Wrote file successfully.", "metadata": {"diagnostics": {}, "filepath": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy/hello.txt", "exists": false, "truncated": false}}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7ea7e547ffe5f9AyazZCosNef", "update": {"sessionUpdate": "agent_message_chunk", "messageId": "msg_181581ea3001WUupwr5LuPXk7q", "content": {"type": "text", "text": "STUB-DONE after 1 tool(s): Wrote file successfully. "}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7ea7e547ffe5f9AyazZCosNef", "update": {"sessionUpdate": "user_message_chunk", "messageId": "msg_181581f4a0017Ak3d27d4XdUp3", "content": {"type": "text", "text": "BASH please"}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7ea7e547ffe5f9AyazZCosNef", "update": {"sessionUpdate": "tool_call", "toolCallId": "call_1793171398527", "title": "echo probe-ok", "kind": "execute", "status": "pending", "locations": [{"path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy"}], "rawInput": {"command": "echo probe-ok", "description": "harmless echo", "cwd": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy"}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7ea7e547ffe5f9AyazZCosNef", "update": {"sessionUpdate": "tool_call_update", "toolCallId": "call_1793171398527", "status": "completed", "title": "echo probe-ok", "content": [{"type": "content", "content": {"type": "text", "text": "probe-ok\n"}}], "rawOutput": {"output": "probe-ok\n", "metadata": {"output": "probe-ok\n", "exit": 0, "truncated": false}}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7ea7e547ffe5f9AyazZCosNef", "update": {"sessionUpdate": "agent_message_chunk", "messageId": "msg_181581fe5001w65q2GByoXLlnm", "content": {"type": "text", "text": "STUB-DONE after 1 tool(s): probe-ok\n "}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7ea7e547ffe5f9AyazZCosNef", "update": {"sessionUpdate": "agent_message_chunk", "messageId": "msg_181582d910010b0mPMLqX7GrQE", "content": {"type": "text", "text": "STUB-REPLY: "}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7ea7e547ffe5f9AyazZCosNef", "update": {"sessionUpdate": "agent_message_chunk", "messageId": "msg_181582d910010b0mPMLqX7GrQE", "content": {"type": "text", "text": "hello "}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7ea7e547ffe5f9AyazZCosNef", "update": {"sessionUpdate": "agent_message_chunk", "messageId": "msg_181582d910010b0mPMLqX7GrQE", "content": {"type": "text", "text": "after "}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7ea7e547ffe5f9AyazZCosNef", "update": {"sessionUpdate": "agent_message_chunk", "messageId": "msg_181582d910010b0mPMLqX7GrQE", "content": {"type": "text", "text": "load "}}}}
```

### A16 session/load after kill -9 during a pending permission: tool replayed as in_progress, nothing re-asked (43-load-after-midperm)

```
c->a {"jsonrpc": "2.0", "id": 2, "method": "session/load", "params": {"sessionId": "ses_e7ea79378ffeW6M5SNPXJSIvqa", "cwd": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy", "mcpServers": []}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7ea79378ffeW6M5SNPXJSIvqa", "update": {"sessionUpdate": "tool_call", "toolCallId": "call_1793171419077", "title": "echo probe-ok", "kind": "execute", "status": "pending", "locations": [{"path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy"}], "rawInput": {"command": "echo probe-ok", "description": "harmless echo", "cwd": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy"}}}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7ea79378ffeW6M5SNPXJSIvqa", "update": {"sessionUpdate": "tool_call_update", "toolCallId": "call_1793171419077", "status": "in_progress", "kind": "execute", "title": "echo probe-ok", "locations": [{"path": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy"}], "rawInput": {"command": "echo probe-ok", "description": "harmless echo", "cwd": "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchpad/probe-opencode/work/dummy"}}}}
```

### A17 permission approved via local HTTP API; the ACP request stays unanswered and the tool runs (26-http-spoof)

```
a->c {"jsonrpc": "2.0", "id": 0, "method": "session/request_permission", "params": {"sessionId": "ses_e7eabf1c4ffeC6AON7e1NoxxAr", "toolCall": {"toolCallId": "call_1793171132797", "title": "echo probe-ok", "kind": "execute", "status": "pending", "locations": [], "rawInput": {"command": "echo probe-ok"}}, "options": [{"optionId": "once", "kind": "allow_once", "name": "Allow once"}, {"optionId": "always", "kind": "allow_always", "name": "Always allow"}, {"optionId": "reject", "kind": "reject_once", "name": "Reject"}]}}
a->c {"jsonrpc": "2.0", "method": "session/update", "params": {"sessionId": "ses_e7eabf1c4ffeC6AON7e1NoxxAr", "update": {"sessionUpdate": "tool_call_update", "toolCallId": "call_1793171132797", "status": "completed", "title": "echo probe-ok", "content": [{"type": "content", "content": {"type": "text", "text": "probe-ok\n"}}], "rawOutput": {"output": "probe-ok\n", "metadata": {"output": "probe-ok\n", "exit": 0, "truncated": false}}}}}
a->c {"jsonrpc": "2.0", "id": 3, "result": {"stopReason": "end_turn", "usage": {"inputTokens": 1, "outputTokens": 1, "totalTokens": 2}, "_meta": {}}}
```


## Appendix B: help output (verbatim, 1.19.5)

### `opencode --help`
```
⠀                                ▄
█▀▀█ █▀▀█ █▀▀█ █▀▀▄ █▀▀▀ █▀▀█ █▀▀█ █▀▀█
█  █ █  █ █▀▀▀ █  █ █    █  █ █  █ █▀▀▀
▀▀▀▀ █▀▀▀ ▀▀▀▀ ▀  ▀ ▀▀▀▀ ▀▀▀▀ ▀▀▀▀ ▀▀▀▀

Commands:
  opencode completion          generate shell completion script
  opencode acp                 start ACP (Agent Client Protocol) server
  opencode mcp                 manage MCP (Model Context Protocol) servers
  opencode [project]           start opencode tui                                          [default]
  opencode attach <url>        attach to a running opencode server
  opencode run [message..]     run opencode with a message
  opencode debug               debugging and troubleshooting tools
  opencode providers           manage AI providers and credentials                   [aliases: auth]
  opencode agent               manage agents
  opencode upgrade [target]    upgrade opencode to the latest or a specific version
  opencode uninstall           uninstall opencode and remove all related files
  opencode serve               starts a headless opencode server
  opencode web                 start opencode server and open web interface
  opencode models [provider]   list all available models
  opencode stats               show token usage and cost statistics
  opencode export [sessionID]  export session data as JSON
  opencode import <file>       import session data from JSON file
  opencode github              manage GitHub agent
  opencode pr <number>         fetch and checkout a GitHub PR branch, then run opencode
  opencode session             manage sessions
  opencode plugin <module>     install plugin and update config                      [aliases: plug]
  opencode db                  database tools

Positionals:
  project  path to start opencode in                                                        [string]

Options:
  -h, --help          show help                                                            [boolean]
  -v, --version       show version number                                                  [boolean]
      --print-logs    print logs to stderr                                                 [boolean]
      --log-level     log level                 [string] [choices: "DEBUG", "INFO", "WARN", "ERROR"]
      --pure          run without external plugins                                         [boolean]
      --port          port to listen on                                        [number] [default: 0]
      --hostname      hostname to listen on                          [string] [default: "127.0.0.1"]
      --mdns          enable mDNS service discovery (defaults hostname to 0.0.0.0)
                                                                          [boolean] [default: false]
      --mdns-domain   custom domain name for mDNS service (default: opencode.local)
                                                                [string] [default: "opencode.local"]
      --cors          additional domains to allow for CORS                     [array] [default: []]
  -m, --model         model to use in the format of provider/model                          [string]
  -c, --continue      continue the last session                                            [boolean]
  -s, --session       session id to continue                                                [string]
      --fork          fork the session when continuing (use with --continue or --session)  [boolean]
      --prompt        prompt to use                                                         [string]
      --agent         agent to use                                                          [string]
      --auto          auto-approve permissions that are not explicitly denied (dangerous!)
                                                                          [boolean] [default: false]
      --mini          start the minimal interactive interface             [boolean] [default: false]
      --no-replay     disable mini session history replay on resume and after resize       [boolean]
      --replay-limit  cap visible mini replay to the newest N messages                      [number]
```

### `opencode acp --help`
```
opencode acp

start ACP (Agent Client Protocol) server

Options:
  -h, --help         show help                                                             [boolean]
  -v, --version      show version number                                                   [boolean]
      --print-logs   print logs to stderr                                                  [boolean]
      --log-level    log level                  [string] [choices: "DEBUG", "INFO", "WARN", "ERROR"]
      --pure         run without external plugins                                          [boolean]
      --port         port to listen on                                         [number] [default: 0]
      --hostname     hostname to listen on                           [string] [default: "127.0.0.1"]
      --mdns         enable mDNS service discovery (defaults hostname to 0.0.0.0)
                                                                          [boolean] [default: false]
      --mdns-domain  custom domain name for mDNS service (default: opencode.local)
                                                                [string] [default: "opencode.local"]
      --cors         additional domains to allow for CORS                      [array] [default: []]
      --cwd          working directory
  [string] [default: "/tmp/claude-0/-home-user-reet-sh/4e38f89d-1dd7-565a-8366-b2c97279074d/scratchp
                                                                            ad/probe-opencode/work"]
```

### `opencode models --help`
```
opencode models [provider]

list all available models

Positionals:
  provider  provider ID to filter models by                                                 [string]

Options:
  -h, --help        show help                                                              [boolean]
  -v, --version     show version number                                                    [boolean]
      --print-logs  print logs to stderr                                                   [boolean]
      --log-level   log level                   [string] [choices: "DEBUG", "INFO", "WARN", "ERROR"]
      --pure        run without external plugins                                           [boolean]
      --verbose     use more verbose model output (includes metadata like costs)           [boolean]
      --refresh     refresh the models cache from models.dev                               [boolean]```
