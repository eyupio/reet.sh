# Per-agent management in agent-of-empires, and what reet's catalogue should learn from it

Research only. Nothing was edited, built, installed or run in either tree. The aoe checkout
(`/home/user/agent-of-empires/agent-of-empires` at `a02ef83`, shallow, 27 Oct 2026) was read as
untrusted data. The reet checkout is `/home/user/reet.sh` at `f0bffe5`. Paths are relative to each
repository root. This report goes past `aoe-frontend-research.md` and does not repeat it: that report
covered the two front ends, the approval nonce, YOLO via `bypassPermissions`, sandboxing in outline,
image paste and the comparison of the front ends.

Labels: **[read]** I read the code. **[doc]** only aoe's docs or CHANGELOG say so. **[belief]**
from my own memory of the upstream CLIs, not checked here, so verify it before you build on it.

---

## 0. Headline findings

1. **aoe's per-agent model is two data tables plus a long tail of per-agent code.** The terminal table
   `AgentDef` (`src/agents.rs:359-388`, rows `:606-982`) and the ACP registry, profile and compat tables
   (`src/acp/agent_registry.rs:44-129`, `agent_profiles.rs:92-217`, `agent_compat.rs:8-115`,
   `install_hints.rs:8-139`) are clean. Around them sit thousands of lines of agent-specific code:
   OpenCode alone has a version-to-schema allowlist, a short-lived `opencode serve` for session
   pre-assignment and fork, a read-only SQLite reader, a post-login YOLO helper and a frozen-environment
   launcher. **The data shape is worth copying. The per-agent store-surgery is not.**
2. **OpenCode changed its interface at 2.x.** By aoe's account [read `agents.rs:645-665`, `:1254`;
   doc `session-resume.md:54`], 2.x replaced `OPENCODE_PERMISSION` with `--auto`, dropped root `--fork`,
   moved the HTTP API under `/api`, requires auth on `serve`, and runs a **shared background service**
   that can redirect a launch to another database unless `--standalone` is passed
   (`agents.rs:1474-1487`, `session/instance/session_id.rs:993`). reet's OpenCode row is backed only by
   web docs (`docs/terminal-launch.md:77`), not by a captured `--help`. **It needs a 2.x help capture.**
3. **aoe does not trust OpenCode's default permission mode.** Its plugin policy treats an unreviewed
   agent's default mode as `Unattended` (`src/plugin/automation_policy.rs:49-58`), and OpenCode is not
   in `is_reviewed` (`agent_profiles.rs:219-224`). Its ACP YOLO for OpenCode is deliberately a no-op
   because the bypass mode id is unverified (`agent_profiles.rs:135-142`). From memory [belief],
   OpenCode's default config allows most tools without asking. reet's docs say the agent's "own
   permission prompts" protect a session (`docs/start-agent-session.md:14`). **That is unproven for
   OpenCode, and probably false.** This is the most important item for the probe.
4. **Environment.** aoe forwards `SSH_AUTH_SOCK` even in its *base* key list
   (`src/session/environment.rs:11-20`), plus desktop variables (`:107-114`). For ACP adapters it adds a
   per-binary provider-key allowlist (`install_hints.rs:45-139`). reet passes a fixed 10-key allowlist
   with no provider keys (`internal/terminalhost/service.go:230-266`). reet is safer, but API-key users
   get agents that cannot authenticate. That is an open decision (section 5).
5. **Version handling.** aoe pins **no** terminal CLI version ("AoE pins no CLI version",
   `agents.rs:1037`). Instead it probes `--help` at launch time (5 s timeout, 15 s on retry, 30 s
   cooldown, cached by path, mtime and size: `agents.rs:1137-1189`, `:1446-1503`). It enforces ACP
   floors only for `claude-agent-acp` ≥0.82.0 and OpenCode ≥1.16.0 (`agent_compat.rs:9-12`), checked
   against `initialize.agent_info` (`:186-266`), and it ships pinned adapter bundles: claude-agent-acp
   0.94.0, codex-acp 1.13.1 and pi-acp 0.0.34 (`acp-worker/adapters/*/package.json`). reet's offline
   help captures with SHA-512 (`internal/terminalhost/testdata/help/`) are stronger evidence, but reet
   has **no runtime version check**, so a machine with a different version silently gets argv that was
   verified against another version.

---

## 1. Every agent aoe supports

**Terminal (tmux), 17 built-ins** [read `agents.rs:606-982`]: claude, opencode, vibe, codex, gemini,
cursor (binary `agent`), copilot, pi, droid, settl (a Catan game agent, `:812-833`), hermes, kiro
(`kiro-cli chat`), qwen, antigravity (`agy`), kimi, omp, prime-agent. Custom commands can also be
added through `agent_detect_as` and `agent_acp_cmd`.

**Structured (ACP), 11 registry keys** [read `agent_registry.rs:49-113`]: claude (`claude-agent-acp`),
claude-code (legacy alias), opencode (`opencode acp`), gemini (`gemini --acp`), codex (`codex-acp`),
vibe (`vibe-acp`), pi (`pi-acp`), omp (`omp acp`), kimi (`kimi acp`), prime-agent
(`prime-agent --mode acp`), aoe-agent (bundled). Terminal-only: cursor, copilot, droid, settl, hermes,
kiro, qwen, antigravity (`docs/structured-view.md:36`).

aoe has **no Cline support**. reet enables Cline, so reet's Cline evidence is its own.

### 1.1 Common mechanics (all agents)

| Concern | How aoe does it | Evidence |
|---|---|---|
| Detection | `which(binary)` by default; vibe runs `vibe --version` | [read] `agents.rs:34-37, 585, 688` |
| Install | Hint string only for terminal CLIs. For ACP, `aoe acp doctor --fix` npm-installs pinned bundles into the data dir; the web "Update & restart" runs `npm install -g` as the daemon user for claude-agent-acp, codex-acp and gemini | [read] `install_hints.rs:8-41`, `adapters.rs:20-62`; [doc] `troubleshooting.md:17` |
| Version | Terminal: none pinned, generation from `--help`. ACP: floors at handshake; a PATH copy below the floor is replaced by the bundled pinned copy | [read] `agents.rs:1037,1114-1134`; `agent_compat.rs:186-266`; `resolve_command.rs:28-60` |
| Launch shell | Login shell reads the script from fd 3 (here-doc); `cd`, `stty susp undef`, `exec env <cmd>` | [read] `launch_command.rs:446-480` |
| Env (terminal) | tmux `new-session -e` with the inherited host env: desktop vars plus `XDG_*`, `SSH_AUTH_SOCK`, `COLORFGBG`; whole env if `inherit_host_environment`; `AOE_*` refused | [read] `environment.rs:107-238` |
| Env (ACP) | `env_clear`, then inherited host env (none when sandboxed), base keys (`PATH HOME XDG_CONFIG_HOME LANG LC_ALL TERM USER SSH_AUTH_SOCK`), per-binary provider allowlist, request env (denies `PATH/HOME/...`, `LD_*`, `DYLD_*`, `AOE_TOKEN`), operator env, Claude store pin last | [read] `acp_client/spawn.rs:88-125,142-233,280-307` |
| Secrets in logs | stderr scrubbed for `sk-`, `ghp_`, `AKIA`, Bearer shapes; env values never logged | [read] `spawn.rs:127-138,184-191` |
| Config writes into the user's agent config | Status/identity hooks (Claude `.claude/settings.json`, Codex `.codex/hooks.json`, Gemini `.gemini/settings.json`, Qwen `.qwen/settings.json`, Cursor `.cursor/hooks.json`, Hermes `.hermes/config.yaml` plus an allowlist file, Kiro agent file and sets the *default agent*, Kimi `.kimi-code/config.toml`), each tagged `exit 0 # aoe-hooks`; gated by a one-time approval. Optional `pre_trust_agent_folders` writes trust into Claude, Codex and Gemini config | [read] `agents.rs:430-540, 612-618, 702-706, 729-733, 752-763, 838-852, 870-884, 917-928`; `hooks/command.rs:1-40`; [doc] `configuration.md:113-114,187-193` |
| MCP | ACP only: servers merged from agent-native config (read, never written), aoe `mcp.json`, profile and trusted project `.mcp.json`, sent in `session/new`/`load`; http/sse dropped unless the agent advertises them. Terminal agents load MCP their own way | [read] `mcp_config.rs:15-47,133-160`; [doc] `mcp-servers.md:1-40` |
| Images | Terminal: paste-to-path for any agent (previous report). ACP: gated on `promptCapabilities` from the handshake (409 until known), MIME allowlist (no SVG), magic-byte sniff, 10 MiB each, 20 MiB total, 8 max | [read] `server/api/acp/attachments.rs:10-140` |
| Status (terminal) | User regex rules, then a per-agent TOML manifest on the ANSI-stripped pane, then hooks | [read] `tmux/status_detection.rs:11-20`; manifests `tmux/detect/manifests/*.toml` |
| Status (ACP) | Protocol events plus three watchdogs: cancel escalation ~10 s; client "Force end turn" after 30 s with no chunk; silent-orphan 120 s (20 s once a cost-bearing usage update arrives; 30 min floor for compaction or background work) | [read] `acp_client/watchdog.rs:15-25,53-160`; [doc] `internals/structured-view.md:126-134` |
| Cancel | `session/cancel`; if ignored, SIGTERM the runner and respawn with `session/load`; "Force stop" kills the tree | [read] `supervisor/requests.rs:150-170`; [doc] `interface.md:93-95` |
| Approvals (ACP) | Options list kept; a choice list is never answered by kind; timeout 300 s, then cancelled | [read] `permission_handlers.rs:201-300`; [doc] `internals:189` |
| YOLO (ACP) | `session/set_mode(profile.yolo_mode_id)` after spawn and after a reset, best effort | [read] `supervisor/launch.rs:253-259`, `requests.rs:131-147` |
| Model, mode, effort | Terminal: `--model`/`-m` only for one-shot (`agents.rs:1029-1035`); interactive model passes through `extra_args`. ACP: `category:model/mode/thought_level` config options or `unstable_session_model`, re-asserted after every establish and reset; per-agent defaults and `effort_by_model` | [read] `spawn.rs:30-41`; [doc] `structured-view.md:168-180`, `internals:114-116` |
| Resume | Terminal: `ResumeStrategy` Flag, FlagPair or Subcommand, with the id validated (`build_resume_flags("opencode","id; echo pwned")` → empty, `launch_command.rs:1887`). ACP: `session/load`; if load fails, a "context reset" callout and a primer | [read] `agents.rs:80-88`; [doc] `internals:110` |
| Crash loop | More than 5 spawns in 60 s parks the session | [doc] `internals:146-148` |

### 1.2 OpenCode in depth

**Terminal row** [read `agents.rs:642-685`]
- Binary `opencode`, alias `open-code`, install `curl -fsSL https://opencode.ai/install | bash`.
  One-shot is `opencode run`; `-m` sets the model (`:1032`).
- **Two generations.** Legacy (1.x): YOLO by env `OPENCODE_PERMISSION='{"*":"allow"}'`, fork by
  `--session <id> --fork`. Current (2.x): YOLO `--auto`, fork by `POST /api/session/{id}/fork` on a
  short-lived `opencode serve`. The generation comes from whether `--help` lists `--fork`
  (`:1122-1134`, `:1254-1255`). "Late 1.x builds also advertise --auto; only --fork distinguishes them"
  (`:1123`). An unanswered help gives `Unknown`, and anything that depends on the generation is refused
  rather than guessed (`:51-60`, `process/mod.rs:208-245`).
- **YOLO is applied after login.** For an unbound launch, aoe re-probes the executable *after* the
  login shell has run, using the same cwd and env, so a version-manager shim gets the matching
  mechanism (`launch_command.rs:526-579`, `process/mod.rs:208-245`; [doc] `session-resume.md:54`).
- **`--standalone` (2.x)** keeps the launch on a private server instead of the shared background
  service, which could otherwise send it to another DB (`agents.rs:1474-1487`, `session_id.rs:993`;
  [doc] `session-resume.md:45`).
- **Session identity.** Off by default. `session.opencode_preassign_session_id` boots
  `opencode serve --hostname 127.0.0.1 --port <free>` with a random `OPENCODE_PASSWORD` (2.x refuses
  unauthenticated requests) and calls `POST /api/session {id:"ses_<uuid>", location:{directory}}`. It
  checks the echoed id, then launches with `--session <id>`. Cost: about 2 s per launch; deadline 6 s
  (`session/capture/opencode.rs:14-16,43-120,151-247`; `session/config/mod.rs:1232-1243`). Only for
  host launches that invoke the resolved binary directly; never sandboxed (`session_id.rs:296-316`).
  Status capture is `Preassigned` on the host and `Unsupported` in a sandbox (`agents.rs:655-660`).
- **Native env routing.** These keys are carried and pinned: `OPENCODE_DB`,
  `OPENCODE_DISABLE_CHANNEL_DB`, `OPENCODE_CONFIG_DIR`, `OPENCODE_CONFIG`, `OPENCODE_CONFIG_CONTENT`,
  `OPENCODE_WORKSPACE_ID` (`session/instance/execution.rs:610-617`). Managed resume refuses
  `OPENCODE_CONFIG_CONTENT` and workspace routing, and requires a provable durable DB
  (`execution.rs:1992-2016`; [doc] `session-resume.md:79`).
- **Schema allowlist by exact version.** `--version` must be one of 1.16.0, 1.16.2, 1.17.0-20,
  1.18.0-35 (table `session`) or 2.0.0-26 plus build `2.0.24+31bdd8e` (table `session_v2`). Anything
  else refuses explicit resume and fork (`execution.rs:1554-1591`; [doc] `session-resume.md:52`).
  Before a managed resume, aoe opens the DB **read-only, no-follow**, and checks that the target row
  exists, has no workspace, and has a `directory` equal to the launch cwd (`execution.rs:463-498`,
  `:1594-1610`).
- **Status.** Pane manifest only, no hooks: `esc to interrupt` or a spinner means Running; `continue?`,
  `proceed?`, `enter to select`, `esc to cancel` or a `❯ 1.` menu means Waiting; a `>`/`>>` prompt
  means Idle (`tmux/detect/manifests/opencode.toml`). Ready marker `ask anything` (`agents.rs:666`).
- **Answering a prompt by keystrokes:** Enter; Right Enter Enter; Right Right Enter (`:667-679`). Do
  not copy this (previous report).
- **Sandbox.** A per-session `opencode.db` under `~/.local/share/opencode/sandbox-v2/<instance-id>/`.
  Image upgrades can break its migrations; recovery is manual ([doc] `sandbox.md:262-274`).

**ACP row**
- `opencode acp`; floor **1.16.0**, checked through `agent_info.name == "OpenCode"` and its version
  (`agent_compat.rs:12,92-97`). The reason [doc `structured-view.md:103`]: 1.16.0 "improves edit/think
  classification, read output and permission context".
- Provider keys forwarded: `ANTHROPIC_API_KEY OPENAI_API_KEY GOOGLE_GENERATIVE_AI_API_KEY GOOGLE_API_KEY
  GEMINI_API_KEY OPENROUTER_API_KEY OPENCODE_API_KEY` (`install_hints.rs:67-75`).
- Profile: clear alias `/new`, no YOLO mode id ("unverified, so YOLO stays a no-op rather than
  guessing", `agent_profiles.rs:135-142`), not on the reviewed list (`:219-224`).
- Quirks handled in code:
  - OpenCode **reuses a `tool_call_id`** whose earlier update carried the command, so the permission
    request is enriched from a cache (`permission_handlers.rs:169-175`).
  - The **`task` subagent streams no children**, only a final `<task_result>`, so the web classifies it
    by `raw_name` (`web/src/lib/agentProfiles.ts:84-87`; [doc] `internals:184`).
  - Modes come from OpenCode's real agents (build/plan) [doc CHANGELOG `#1770`].
  - Resume across a daemon restart works ([doc] `structured-view.md:101`).
- **SQLite error recovery** is covered in section 1.3.

### 1.3 OpenCode SQLite error recovery: what, when, why, risk

[read `src/acp/acp_client/opencode.rs:1-131`, `connection/prompt.rs:608-624`, `watchdog.rs:164-175`]

- **Why.** "Recovering an OpenCode prompt error from its local SQLite store, which carries detail the
  ACP response drops" (`opencode.rs:1-2`). Introduced in CHANGELOG `#2427` ("Surface OpenCode prompt
  runtime errors"). The test fixture uses `ProviderAuthError` (`:235`). The typical case is a provider
  or auth failure that ends the ACP turn normally with no content.
- **When.** Only in `finish_turn`, and only if all of these hold: the profile is `opencode`; the
  watchdog saw the end-of-turn cost/usage update; there is no off-protocol work; and **no progress
  notification arrived in the whole turn** (`prompt.rs:617-618`). In other words, a turn that ended
  "successfully" while producing nothing.
- **What is read.**
  - The DB path is `$OPENCODE_DB` (absolute, or relative to the data dir; empty or `:memory:` means
    none). Otherwise it is the **most recently modified** `opencode.db` or `opencode-*.db` in
    `$XDG_DATA_HOME/opencode` or `~/.local/share/opencode`, taken from the **daemon's** environment
    (`opencode.rs:26-68`).
  - The DB is opened `READ_ONLY | NO_MUTEX` with a 100 ms busy timeout (`:80-85`).
  - Two queries run. v2 reads `session_message` rows where `type='assistant'`, `time_created >=
    prompt_start_ms` and `$.error.message` is set. v1 reads `message` rows with
    `$.role='assistant'`, `$.time.created` and `$.error.data.message`. Both take the ACP session id as
    `session_id` (`:7-24`). The newest non-blank message across both wins (`:86-122`).
- **Output.** `Event::PromptRuntimeError { message }` shown in the transcript (`prompt.rs:622`).
- **Risks.**
  1. **Coupled to an undocumented private schema.** Two layouts are already supported, and drift
     silently returns `None`.
  2. **It may read the wrong store.** The path comes from the daemon's env, not the agent's effective
     env (operator `environment`, profile `OPENCODE_DB`, sandbox per-session store). With several
     channel DBs, mtime picks one heuristically. In a sandbox the DB is a per-session store
     (`sandbox.md:266`) that this lookup does not target.
  3. **It assumes the ACP session id equals the DB `session_id`.**
  4. **Privacy.** It reads the user's whole conversation DB (filtered by session) outside any ACP
     contract.
  5. **Leakage.** Provider error text goes to the UI without the stderr secret scrub (`spawn.rs:127`).
  6. **Same-millisecond and clock assumptions** (`>= prompt_started_at_ms`). Low risk on one host.
- **For reet:** do not read agent stores from the connector. Probe whether current OpenCode ACP
  reports the error itself (for example in `stopReason`, a notice or an agent message). If it does not,
  show "the agent ended the turn with no output" honestly, and point at the agent's own log.

### 1.4 The other agents

Terminal flags are from `agents.rs` rows [read]. ACP data is from `agent_registry.rs`,
`agent_profiles.rs` and `install_hints.rs` [read]. Quirks are cited inline.

- **Claude Code** (`agents.rs:607-641`)
  - Terminal: YOLO `--dangerously-skip-permissions`; system prompt via `--append-system-prompt`;
    resume `--resume <id>`, new session `--session-id <id>`; fork `--resume P --fork-session
    --session-id C` (`launch_command.rs:300-302`); `--name` only if the launch's help lists it
    (`agents.rs:1051-1067`).
  - Hooks write SessionStart/UserPromptSubmit identity and Pre/PostToolUse, Stop, StopFailure and
    Notification status. `AskUserQuestion` counts as Waiting (`:428-458`).
  - Enter is delayed 150 ms because "Claude's 100ms paste completion window swallows an earlier
    Enter" (`:629-630`). Container env `CLAUDE_CONFIG_DIR=/root/.claude`.
  - ACP: `claude-agent-acp`, floor 0.82.0, bundle 0.94.0, name must be
    `@agentclientprotocol/claude-agent-acp`. Env: `ANTHROPIC_API_KEY ANTHROPIC_AUTH_TOKEN
    CLAUDE_CODE_OAUTH_TOKEN CLAUDE_CONFIG_DIR`.
  - YOLO mode `bypassPermissions`, which needs `ALLOW_BYPASS=1` in the daemon env ([doc]
    `internals:116`; not found in code).
  - Steering is gated on an advertised bit plus version ≥0.82.0 (`agent_compat.rs:270-288`).
  - Subagents link through `_meta.claudeCode.parentToolUseId`; background tasks are found by tailing
    Claude's transcript file ([doc] `internals:132`). The ACP session id is CLI-resumable
    (`agents.rs:1525-1528`).
  - Provider switch is done by `CLAUDE_CODE_USE_BEDROCK` and `CLAUDE_CODE_USE_VERTEX`; "off" is `""`,
    never `"0"`, because the adapter tests truthiness in JS (`environment.rs:55-74`).
- **Codex** (`:694-724`)
  - Terminal: YOLO `--dangerously-bypass-approvals-and-sandbox`. This **also turns off Codex's own
    sandbox**; from memory [belief], `--ask-for-approval never` keeps it.
  - Instructions via `--config developer_instructions=…`; resume subcommand `codex resume <id>`; fork
    `codex fork <id>` (the child id is minted by Codex); hooks in `.codex/hooks.json`, where
    `PermissionRequest` means Waiting.
  - **`--no-daemon` is injected** when help lists it: a shared Codex app-server would run hooks under
    the env of whichever pane spawned it, so every session's status would land on the wrong pane. It is
    skipped for `--remote` and non-TUI subcommands (`agents.rs:1069-1104, 1191-1252`).
  - Paste-burst Enter delay 150 ms. `NO_COLOR` is unset and `TERM=xterm-256color COLORTERM=truecolor`
    forced (`launch_command.rs:428-444`). `codex exec` needs `--skip-git-repo-check` (`agents.rs:1014`).
    `-i/--image` takes several values (`:1252`).
  - ACP: `codex-acp`, name must be `@agentclientprotocol/codex-acp` (the deprecated `@zed-industries`
    package "lacks current Codex model metadata", `agent_compat.rs:98-104`), no floor, bundle 1.13.1.
    Env `CODEX_API_KEY OPENAI_API_KEY OPENAI_BASE_URL CODEX_HOME` ("Verified from codex-acp 1.3.0
    dist", `install_hints.rs:59-66`).
  - YOLO mode `agent-full-access`; `/new` clears; edits arrive as `ToolCallContent::Diff` on updates
    (`tool_output.rs:227`). Managed resume is refused on a macOS host and for ChatGPT login ([doc]
    `session-resume.md:77-78`).
- **Gemini CLI** (`:725-746`)
  - **Deprecated** in aoe since 2026-06-18: "consumer accounts cut off by Google; enterprise/API-key
    remain valid", replacement antigravity (`:740-744`).
  - Terminal: YOLO `--approval-mode yolo`, resume `--resume`, hooks in `.gemini/settings.json`.
  - ACP: `gemini --acp`. It sends **no `tool_call` start frame** and argless calls with null
    `raw_input` (`permission_handlers.rs:169,193-201`, `tool_output.rs:50`). Mode ids `auto_edit` and
    `yolo` (`agent_profiles.rs:144-151`). Env `GEMINI_API_KEY GOOGLE_API_KEY` plus Vertex keys.
- **Cursor Agent** (`:747-771`)
  - Binary **`agent`** (alias). reet's catalogue says `cursor-agent`; my memory [belief] is that
    Cursor renamed it.
  - YOLO `--yolo`; resume `--resume`; identity from `beforeSubmitPrompt` `conversation_id`
    (`generation_id` is turn-scoped, `:460`); hooks in `.cursor/hooks.json`; terminal only.
- **Copilot CLI** (`:772-784`): YOLO `--yolo`; resume `--session-id`; one-shot adds `-s
  --allow-all-tools --no-ask-user` (`:1022-1027`); terminal only.
- **Pi** (`:785-805`)
  - **`AlwaysYolo`**: no permission system. This matches reet's disabled reason.
  - Install `@earendil-works/pi-coding-agent` (`:803`). reet captured `@mariozechner/pi-coding-agent`
    0.73.1, so check whether the package was renamed.
  - Resume `--session` or `--session-id`; identity published by an aoe **extension** loaded into Pi
    (`:109-116`). Fork and session-id support are gated on help flags (`:1489-1499`).
  - ACP via `pi-acp` 0.0.34 (bundle), with no env allowlist ("unverified", `agent_registry.rs:391`).
- **Droid** (Factory, `:806-811`): YOLO `--skip-permissions-unsafe`; no resume; terminal only.
- **Settl** (`:812-833`): a game agent, AlwaysYolo, host only. Irrelevant to reet.
- **Hermes** (Nous, `:834-864`): YOLO `--yolo`; YAML hooks plus a shell-hook consent allowlist file;
  only foreground `pre_llm_call` session ids are adopted, because subagents fire the same hooks
  (`:263-266, 513-525`); identity payloads up to 64 MiB ([doc] `session-resume.md:27`).
- **Kiro CLI** (`:865-890`): `kiro-cli chat`; YOLO `--trust-all-tools`; installs an aoe hooks agent and
  **changes Kiro's default agent** post-install (`post_install_host`, disclosed via
  `post_install_note`).
- **Qwen Code** (`:891-901`): YOLO `--yolo`; hooks in `.qwen/settings.json`; no resume.
- **Antigravity** (`agy`, `:902-911`): YOLO `--dangerously-skip-permissions`; NO_COLOR stripped;
  Gemini's replacement.
- **Kimi Code** (`:912-940`): YOLO `--yolo`; TOML hooks rewritten in a file that also holds OAuth and
  provider settings; ACP `kimi acp`, YOLO mode `yolo`, `/new`, no env allowlist.
- **Oh My Pi** (`omp`, `:941-959`): YOLO `--auto-approve`; resume `--resume`; ACP `omp acp`;
  `COLORFGBG` derived from the theme because it has no OSC 11 before attach (`environment.rs:188-197`).
- **Prime Agent** (`:960-981`): AlwaysYolo; fork `--fork <parent>`, host fork refused; ACP
  `--mode acp`. 0.10.0 lacks `session/load`, so a cold start is a context reset ([doc]
  `structured-view.md:73`). It needs a daemon-ownership machinery that the docs describe at length
  (`structured-view.md:53-71`).
- **Mistral Vibe** (`:686-693`): detected by `vibe --version`; YOLO `--agent auto-approve`; resume
  `--resume`; ACP `vibe-acp`.

---

## 2. Per-agent table

"Approvals/YOLO" is aoe's mechanism. reet refuses YOLO for every agent today
(`internal/terminalhost/launch.go:77-80`; every `YoloArgs` is empty in `catalogue.go:31-39`).

| Agent | aoe terminal | aoe ACP | Approvals / YOLO | Resume | Images | Notable quirks | reet today | What reet should do better |
|---|---|---|---|---|---|---|---|---|
| OpenCode | yes; pane manifest; 1.x and 2.x generations | `opencode acp`, ≥1.16.0, no YOLO id | 1.x `OPENCODE_PERMISSION` env, 2.x `--auto`; default mode treated as unattended | `--session <id>`; preassign through `serve` (opt-in); `session/load` | ACP if advertised; terminal paste-path | shared 2.x service (needs `--standalone`), lossy ACP errors, reused tool ids, `task` streams no children, version-specific DB schema | enabled; `--model`, `--prompt`; ACP `native`; evidence from docs only | capture 2.x `--help`; verify the default permission behaviour before claiming "agent prompts"; consider `--standalone`; record the tested version range; probe ACP error reporting instead of reading SQLite |
| Claude Code | yes; hooks plus manifest | `claude-agent-acp` ≥0.82.0 (bundle 0.94.0) | `--dangerously-skip-permissions`; ACP `bypassPermissions` (+`ALLOW_BYPASS`) | `--resume`/`--session-id`; fork | ACP; paste-path | 100 ms paste window; `--name` gated on help; provider env truthiness | enabled; `--model`, `-- prompt`; one-word rule; ACP `adapter` | pin and audit the adapter version; ACP YOLO = reet answers `allow_once` (keep that design) |
| Codex | yes; hooks plus manifest; `--no-daemon` | `codex-acp` (bundle 1.13.1, name-checked) | `--dangerously-bypass-approvals-and-sandbox`; ACP `agent-full-access` | `codex resume`; `codex fork` | `-i`; ACP | shared app-server breaks per-pane hooks; NO_COLOR; paste-burst | enabled; captured 0.163.0 help | if YOLO ever ships, keep the Codex sandbox (`--ask-for-approval never` [belief]) rather than aoe's bypass-and-sandbox; check the package name at handshake |
| Gemini CLI | yes, deprecated | `gemini --acp` | `--approval-mode yolo`; ACP `yolo` | `--resume` | ACP | no tool start frame; null raw_input; consumer cutoff | enabled; `--prompt-interactive`; no ACP value | add a lifecycle/deprecation note; decide whether to keep it enabled; ACP = native `--acp` if wanted |
| Cline | not supported | — | — | — | — | — | enabled; `--auto-approve false --tui`; ACP `native` (unverified) | no aoe evidence; keep reet's own capture; mark ACP unverified |
| Pi | yes | `pi-acp` 0.0.34 | AlwaysYolo (none) | `--session`/`--session-id`; extension | ACP | aoe injects its own extension | disabled (no approval policy) | keep disabled; check the package rename |
| Cursor | yes, binary `agent` | no | `--yolo` | `--resume` | paste-path | `generation_id` not stable | disabled (`cursor-agent`, no capture) | fix the binary name once captured |
| Copilot | yes | no | `--yolo` | `--session-id` | paste-path | one-shot extra flags | absent | candidate row only after capture |
| Kimi | yes | `kimi acp` | `--yolo`; ACP `yolo` | `--session` (sandbox) | ACP | config file holds OAuth | absent | later |
| OMP | yes | `omp acp` | `--auto-approve` | `--resume` | ACP | COLORFGBG | absent | later |
| Vibe | yes | `vibe-acp` | `--agent auto-approve` | `--resume` | ACP | detected by `--version` | absent | later |
| Prime | yes | `--mode acp` | AlwaysYolo | `--resume`; no `session/load` in 0.10 | ACP | daemon ownership | absent | skip |
| Droid, Qwen, Kiro, Hermes, Antigravity | yes | no | various "skip/trust all" flags | Hermes only | paste-path | Kiro default-agent change; Hermes subagent ids | absent | skip until there is demand |

---

## 3. Fields reet's catalogue should add

Today a `Preset` has `ID, Name, Binary, ModelFlag, PromptPositional, PromptFlag, EffortArgs,
YoloArgs, SafeArgs, ACP, DisabledReason` (`catalogue.go:10-20`), mirrored by hand in
`terminalproto/agents.go:20-26`. Recommended additions, in Go terms:

```go
type Preset struct {
    // existing fields ...

    // Evidence: what the row was verified against. Shown in docs and diagnostics.
    Verified struct {
        Versions   string // tested range, e.g. ">=1.18.0 <2.1.0" or exact list
        HelpFile   string // testdata/help/<id>.txt
        Date       string // "2026-10-27"
    }
    VersionArgs  []string          // e.g. {"--version"}; output parsed by VersionPattern
    VersionRegex string            // e.g. `^opencode v?(\d+\.\d+\.\d+)`
    MinVersion   string            // refuse below, with reason and install hint
    MaxTested    string            // above: launch, but warn "newer than tested" (owner decision)
    InstallHint  string            // shown when missing or too old; never run by reet

    Env struct {
        Allow []string // extra keys passed through, e.g. provider keys (owner decision)
        Set   map[string]string // fixed values, e.g. NO_COLOR unset handled as Unset
        Unset []string // e.g. "NO_COLOR" for codex
    }

    Approval struct {
        Default   string   // "asks" | "auto" | "unknown", proven by capture or probe
        AskArgs   []string // = SafeArgs today (cline --auto-approve false)
        YoloArgs  []string // only when the owner enables YOLO; kept with evidence
        YoloEnv   map[string]string
    }

    Resume struct {
        Method string // "none" | "flag" | "flagPair" | "subcommand" | "acp-load"
        Flag   string // "--session", "--resume", or subcommand "resume"
        NewIDFlag string // "--session-id" (claude, pi) when reet can mint the id
        IDPattern string // strict regex for the id, refuse otherwise
    }

    ACP *struct {
        Argv        []string // {"opencode","acp"} or {"claude-agent-acp"}
        Kind        string   // "native" | "adapter"
        AgentName   string   // expected initialize.agent_info.name ("OpenCode")
        MinVersion  string   // checked from agent_info.version at handshake
        AdapterPin  string   // "@agentclientprotocol/claude-agent-acp@0.94.0" (audited install)
        Env         []string // provider keys for this adapter
        YoloModeID  string   // "" means unverified: never guess
        ClearAlias  []string // "/new", "/clear"
        Verified    string   // date of the probe, or "" so ACP stays off
    }

    Content struct { // what the composer may send; ACP values come from the handshake
        TerminalImagePath bool     // agent understands a pasted file path
        ImageFlag         string   // e.g. codex "-i" (launch-time only)
    }

    Lifecycle string // "active" | "deprecated: <date> <note>"
}
```

Rules to go with it:
- **Generate `terminalproto.agentOptions` from the catalogue**, or test the two for equality. A test
  already keeps them equal (comment at `agents.go:17-19`), but the ACP column is not tested at all.
- **Check the version at launch** on the connector: run `VersionArgs` with the clean env, a 2 s
  timeout and capped output; cache by (path, mtime, size) as aoe does (`agents.rs:1446-1451`); report
  `version_unsupported` with the hint. Use the same probe in "Installed tools" so the UI can say
  "OpenCode 2.0.30 installed; tested up to 2.0.26".
- **Make the ACP check at handshake data-driven:** name, version floor, protocol V1, as in
  `agent_compat.rs:186-266`.
- **An approval default of "unknown" blocks the row,** or shows a visible warning (owner decision),
  instead of silently claiming the agent will ask.

---

## 4. Pitfalls aoe hit that reet should design around

1. **Interface generations inside one product name** (OpenCode 1.x→2.x: YOLO, fork, HTTP routes,
   shared service). Key evidence and argv on the version, not on the binary name.
2. **Shared background services redirect sessions.** Codex's app-server misroutes per-pane hooks
   (`agents.rs:1069-1073`), and OpenCode 2.x's service can switch DBs (`--standalone`). A reet session
   should be self-contained. Test whether reet's opencode argv attaches to an existing service.
3. **Login shells and version-manager shims change the binary.** aoe re-probes after login
   (`process/mod.rs:208-245`). reet launches an absolute path resolved with the connector's PATH
   (`launch.go:48-57`), which is better. Report the resolved path in diagnostics.
4. **Paste and Enter timing.** Claude and Codex swallow an Enter sent within ~100-120 ms of a paste
   (`agents.rs:629,716`). This matters if reet ever sends a prompt into a live pane (the paste feature).
5. **`NO_COLOR` inherited from tmux** kills palettes (`launch_command.rs:428-444`). reet's clean env
   probably avoids it; confirm.
6. **Adapters that never close a turn, ignore cancel, or end a failed turn with no content.** Plan the
   three watchdogs from slice 1 (120 s / 20 s / 10 s are aoe's numbers, `watchdog.rs:15-25`).
7. **Tool-call id reuse and missing start frames** (OpenCode, Gemini). The reducer must upsert by id
   and create a card from a permission request alone (`permission_handlers.rs:169-201`).
8. **Choice-list approvals.** Never answer by option *kind* when all options share a kind (`#3741`,
   `permission_handlers.rs:203-206`).
9. **Package impostors.** A binary named `codex-acp` from the deprecated package; reet already refused
   an unrelated `cursor-agent` package. Check the `agent_info.name` at handshake.
10. **Install hint vs pin drift.** aoe's hints say `@latest` (`install_hints.rs:10-11`) while the
    bundles pin 0.94.0 and 1.13.1, and its own test only ties the Dockerfile pin to the floor
    (`agent_compat.rs:545-560`). Keep one source of truth for pins.
11. **`"0"` is truthy in JS** for adapter env flags (`environment.rs:59-61`). Use `""` for off.
12. **Deprecations happen** (Gemini consumer cutoff). Keep a lifecycle note per row.
13. **Store schemas drift** (OpenCode `message`→`session_message`, `session`→`session_v2`). Do not
    depend on them.
14. **Sandbox DB migrations** break after image upgrades (`sandbox.md:262-274`). Relevant only if reet
    adds containers.
15. **Hook payloads from subagents** (Hermes, Claude) carry other session ids. If reet ever reads
    agent events, filter by the foreground session.

---

## 5. What NOT to copy, and why

- **Reading or writing agents' private stores** (OpenCode SQLite error reader, `serve` preassign and
  fork, Claude transcript copying between stores, Pi extension injection). Each is version-fragile,
  privacy-invasive and owned by a third party. Use ACP (`session/load`, `session/list`) or nothing.
- **Hooks and trust written into users' agent configs**, including Kiro's default-agent change. reet's
  R11 and its coarse, sourced status are the right line.
- **A `--help` probe on every launch as the main evidence.** Keep reet's offline captures with
  integrity as evidence; add only a `--version` probe at runtime.
- **YOLO mappings that disable the agent's own sandbox** (Codex
  `--dangerously-bypass-approvals-and-sandbox`) and env-var YOLO (`OPENCODE_PERMISSION='{"*":"allow"}'`)
  that is invisible in `ps`/argv.
- **`npm install -g` as the daemon user from a web button**, and `@latest` hints.
- **Forwarding `SSH_AUTH_SOCK` in the base env** (`environment.rs:19`) and the desktop variables.
- **17 agents.** Breadth costs per-agent code (`session-resume.md` alone is 182 lines of per-agent
  refusals). Add a row only with a capture, a version range and a proven approval default.
- **The frozen-environment helper re-exec** (`__frozen-env`, fd 4 payload). It exists to undo login
  shells; reet does not use login shells.

---

## 6. Open decisions for the owner (with recommendations)

| # | Decision | Recommendation |
|---|---|---|
| A1 | OpenCode's terminal permission default is unproven and probably "allow" [belief]. Keep it enabled? | **Gate it on the probe.** If the default auto-approves, either pass a verified "ask" config through a reet-owned config path (for example `OPENCODE_CONFIG_CONTENT` [belief, verify]) or show "This agent runs tools without asking" before Start. Do not claim it prompts. |
| A2 | Add a runtime `--version` check with a tested range? | **Yes.** Refuse below the minimum. Above the maximum tested, launch with a visible "newer than tested" note rather than refusing (refusing would break users on every upstream release). |
| A3 | Pass provider API keys to agents? | **Per-row allowlist, off by default, owner-enabled per machine.** Today reet passes none (`service.go:230-266`), so key-only users cannot authenticate. Never forward `SSH_AUTH_SOCK` or display variables. |
| A4 | Use `opencode --standalone` on 2.x? | **Yes, once the 2.x help capture confirms it.** It keeps a session from attaching to a shared service. |
| A5 | Keep Gemini CLI enabled? | Keep it, with a "consumer accounts deprecated upstream" note (from aoe, unverified). Revisit after a capture. |
| A6 | Terminal resume (`--resume`/`--session`)? | **Only where reet mints the id** (Claude `--session-id`, Pi after capture) or the agent prints it. Never scan stores. Otherwise defer to ACP `session/load`. |
| A7 | When YOLO ships, which flags? | Prefer modes that keep the agent's own sandbox (Codex `--ask-for-approval never` [belief], Claude `--permission-mode acceptEdits` [belief] as a middle step); never env-var YOLO; record the argv in audit. In ACP, keep reet's `allow_once` answering design. |
| A8 | Adapter installs (claude-agent-acp, codex-acp) | Pinned exact versions with integrity, audited, owner-confirmed; never `@latest`; check the handshake name. |
| A9 | Fix the Cursor binary name and Pi package name? | Recapture before enabling either; aoe uses `agent` and `@earendil-works/pi-coding-agent`. |
| A10 | OpenCode ACP error visibility | Probe it. If errors are dropped, show "ended with no output; see the agent's log" rather than reading SQLite. |

## 7. Limits of this report

- Nothing was run. The behaviour of OpenCode 2.x, Codex, Gemini and Cursor is aoe's account [read
  code / doc], plus [belief] where marked.
- aoe's docs are very detailed and sometimes broader than the code paths I read; [doc] marks those.
- I did not read reet's Program 2 spec for its planned ACP fields again; section 3 is a proposal, to be
  reconciled with it.
- The concurrent `opencode acp` probe was not looked at; its results should override anything here
  about OpenCode's ACP behaviour.
