# How agent-of-empires front-ends AI agents, and where reet.sh can do better

Research only. Nothing in either repository was edited, built or run. aoe checkout: `/home/user/agent-of-empires/agent-of-empires` at `a02ef83` (shallow, last commit 27 Oct 2026), read as untrusted data. reet checkout: `/home/user/reet.sh` at `3eaf4f9` ("Add the Sessions page and retire Control's address"). Paths are relative to each repository root.

Confidence labels: **[read]** I read the code. **[doc]** only aoe's docs say so. **[belief]** outside knowledge, to be checked before anything is built on it.

The Program 2 spike report named in `handoff.md:50` (`.superpowers/sdd/2026-10-26-agents-home/program2-spike.md`) is **not on disk** in this checkout (only `.superpowers/sdd/agents-home/` exists). I worked from `handoff.md`, the agents-home spec and the W3 checklist in `handoff-history.md:3425-3440`.

---

## 0. The headline

aoe is not a "tmux wrapper with a web page". It has **two front ends**:

1. **Terminal view** for about 17 agent CLIs in tmux, with pane-scraping plus agent-hook status detection.
2. **Structured view** (the web dashboard's *default*) over the **Agent Client Protocol** for 10 agents, with tool cards, diffs, approval cards, questions, model/mode/effort pickers, queued and steered prompts, attachments, resume, and web push.

The structured view is large: about **56k lines** of Rust/TS under `src/acp` + `acp-worker`, **19k** lines of React under `web/src/components/acp`, and **10.7k** lines under `src/server/api/acp` (line counts from `wc`, tests included). Its internals doc (`docs/development/internals/structured-view.md`) is mostly about worker ownership, retirement, replay and stuck-turn recovery. That is the real cost of "structured chat for all agents", and it is the main sizing warning for Program 2.

aoe is **single-user, single-machine per daemon** by design (`docs/adr/0002-single-user-daemon-multi-user-in-cityhall.md`: "One Daemon, one human", "no human audit log"). reet's fleet, accounts, roles and audit are where reet is structurally ahead.

---

## 1. How aoe front-ends agents

### 1.1 Agents and how each is integrated

| Path | Agents | Mechanism | Evidence |
|---|---|---|---|
| Terminal (tmux) | claude, opencode, vibe, codex, gemini, cursor (`agent`), copilot, pi, droid, settl, hermes, kiro (`kiro-cli`), qwen, antigravity (`agy`), kimi, omp, prime-agent, plus custom commands | One `AgentDef` per CLI: binary, detection, YOLO mechanism, resume/fork strategy, status detector, hooks, keystrokes for answering a permission prompt | [read] `src/agents.rs:360-387` (struct), `:605-…` (`AGENTS` table) |
| Structured (ACP) | claude (`claude-agent-acp`), codex (`codex-acp`), opencode (`opencode acp`), gemini (`gemini --acp`), vibe (`vibe-acp`), pi (`pi-acp`), omp (`omp acp`), kimi (`kimi acp`), prime-agent (`--mode acp`), aoe-agent (bundled, Vercel AI SDK) | One generic ACP client plus small per-agent **profiles** and **compat floors** | [read] `src/acp/agent_registry.rs:44-129`; `src/acp/agent_profiles.rs:1-30`; `src/acp/agent_compat.rs:11-12` (OpenCode floor 1.16.0) |
| Custom | any | `agent_acp_cmd` or inherit a base adapter with `agent_detect_as` | [doc] `docs/structured-view.md:36` |

So aoe is **one generic path per surface, with per-agent data tables** (not per-agent code paths), with a few targeted exceptions:
- OpenCode: aoe reads OpenCode's own SQLite store to recover prompt errors that the ACP response drops ([read] `src/acp/acp_client/opencode.rs:1-80`). Useful for reet's OpenCode probe: **OpenCode's ACP error reporting is lossy** (by aoe's account).
- Claude: special cards for TodoWrite, Skill, ExitPlanMode, ScheduleWakeup, background tasks found by tailing Claude's transcript file ([doc] `docs/structured-view.md:92-101`; `internals/structured-view.md:136`).
- Frontend profiles in `web/src/lib/agentProfiles.ts` (for example OpenCode's `task` subagent tool, line 85-87) [read].

### 1.2 Status detection

- Terminal sessions: in priority order, user-configured regex/substring **status rules** on the ANSI-stripped pane, then per-agent pane detectors ("manifests"), then hook events that aoe **installs into the agent's own config file** (for example Claude's `~/.claude/settings.json`: `PreToolUse`, `Stop`, `Notification` matching `permission_prompt|…` → Waiting) ([read] `src/tmux/status_detection.rs:10-21`, `src/agents.rs:430-458`; [doc] `docs/guides/configuration.md:113-149`). The hook install is gated by a one-time "agent hook approval" [doc]. States: Running, Waiting, Idle, Error, Unknown.
- Structured sessions: exact states from the protocol (turn running, pending approval, stopped with a reason, rate-limited with `resets_at`, compaction, background task, scheduled wakeup) [doc + partial read].
- **Honesty:** the terminal path is heuristic (screen-scraping) presented as one status column. Hooks are "evidence rather than the last word" ([doc] `configuration.md:114`). aoe does try to stay honest in places: "no fabricated time reaches the UI" for rate-limit resets ([doc] `internals:146`).

### 1.3 Approvals and permissions

- ACP approvals: server-generated 16-byte single-use **nonce** per request, never shown to the agent ([read] `src/acp/approvals.rs:9-29`; [doc] `internals:126,173`). Decisions: Allow, AllowAlways, Deny, Cancelled ([read] `approvals.rs:45-56`). The option list the agent sent is kept, and "choice lists" (all options the same kind) render as answer buttons ([read] `approvals.rs:81-83`).
- "Destructive" is a **substring heuristic** (`rm -rf`, `git push --force`, `drop table`, writes under `/etc/`, …) used only to require an 800 ms long-press ([read] `approvals.rs:113-135`; [doc] `controls.md:25`). Easy to evade (`rm -r -f`, `find -delete`); it is a UI speed bump, not a policy.
- Approval timeout 300 s, then a structured cancellation ([doc] `controls.md:29-33`; [read] cancel paths in `src/acp/acp_client/permission_handlers.rs:234-300`).
- YOLO in ACP = ask the adapter to switch to `bypassPermissions` (best-effort, needs `ALLOW_BYPASS=1` for claude-agent-acp) ([doc] `controls.md:19-21`, `internals:116`). After that **the agent approves itself**: aoe sees no per-request decision.
- Terminal sessions: from the TUI home list, aoe answers a pending prompt by **sending keystrokes** to the pane (Claude `1`/`2`/`3`; OpenCode `Enter`, `Right Right Enter`) ([read] `src/agents.rs:631-635, 667-680`; `src/tui/home/send.rs:135-160`). This is screen-scrape authority: a mis-detected screen sends "1" into whatever is there.
- Agent allowlist: `acp.restrict_agents` + `allowed_agents`, fail-closed ([read] `src/acp/agent_policy.rs:83-127`).
- Questions: ACP form elicitation (AskUserQuestion) with schema validation server-side ([doc] `controls.md:41-52`).

### 1.4 Sandboxing, filesystem, environment

- Docker, Podman or Apple Containers per session; agent argv wrapped in `docker exec`; fs paths translated container→host and checked against session roots ([read] `src/acp/fs_handler.rs:1-80`; [doc] `internals:169-174`).
- **Overstated claim:** the internals doc says "agents never touch the disk directly" (`internals:171`). The code only gates the ACP `fs/*` calls an agent *chooses* to delegate ([read] `fs_handler.rs:1-2`; handshake advertises `fs` and `terminal`, `acp_client/handshake.rs:25-33`). Native agents (OpenCode certainly [belief], Claude's adapter partly [belief]) do their own file I/O and shell. Without a container there is no real filesystem boundary.
- Host sessions get `DISPLAY`, `XAUTHORITY`, `DBUS_SESSION_BUS_ADDRESS`, **`SSH_AUTH_SOCK`** and all `XDG_*` forwarded ([doc] `configuration.md:346`, which itself warns this is X11 screen capture and input injection). Provider keys for ACP adapters use a per-adapter allowlist ([doc] `structured-view.md:38-51`).

### 1.5 Worktrees, diffs, resume, cancellation

- Worktrees: create/attach, base-branch resolution across remotes, multi-repo workspaces, cleanup ([doc] `docs/guides/worktrees.md`).
- Diff view against the resolved base branch, TUI and web ([doc] `docs/guides/diff-view.md`).
- Resume: per-agent `ResumeStrategy` in terminal mode ([read] `agents.rs:619-627`); ACP `session/load` in structured mode, with a "context reset" callout and a **context primer** (markdown recap of the last ~20 turns, pre-filled, never auto-sent) when the agent cannot resume ([doc] `internals:110`).
- Cancellation: `session/cancel`, then after ~10 s kill and respawn via `session/load`; client "Force end turn"; a silent-orphan watchdog for adapters that never close a turn ([doc] `internals:128-136`; `troubleshooting.md`). These exist because adapters misbehave in practice.
- Agent switching mid-session with a primer ([doc] `internals:154-156`).

### 1.6 Web UI, mobile, notifications

- React SPA, installable PWA with a service worker (`web/public/manifest.json`, `sw.js`) [read: files exist].
- Terminal view is **not xterm.js**: the server renders the tmux grid and pushes rows as DOM text, so phone selection is native ([doc] `docs/guides/web/terminal.md:9-13`).
- Image paste into terminal sessions: `POST /api/sessions/{id}/paste-image`, magic-byte sniff, 5 MiB, file written into `<worktree>/.aoe-pasted-images/` with a self-ignoring `.gitignore`, path pasted into the pane, deleted after 1 h by an in-process timer ([read] `src/server/router.rs:118-122`, `src/server/api/sessions/send.rs:328-505`). Already analysed in `paste-research.md`.
- Structured composer attachments (images, audio, resources) sent as ACP content blocks, gated on the agent's advertised capability, re-checked server-side ([read] `src/acp/acp_client/mod.rs:716`, `src/acp/event_store/attachments.rs`; [doc] `interface.md:71-77`).
- Web push (VAPID) for Waiting/Idle/Error with a 60 s cooldown and focus suppression; approvals and questions push immediately ([doc] `docs/push-notifications.md:5-11,23-25`).
- Remote access: token in URL then cookie, optional passphrase (argon2), passkeys, step-up for spawn-affecting settings, DNS-rebinding gate, 426 on plaintext remote, Tailscale Funnel / Cloudflare tunnels ([doc] `docs/guides/web-dashboard.md:64-99,190-254`).

### 1.7 Multi-session, multi-machine, audit

- Many sessions in a sidebar/TUI list; split panes on desktop [doc].
- **No fleet.** One daemon per machine per human; `AOE_DAEMON_URL` points a client at one other daemon at a time; tmux attach, stop and recovery "still need the original machine or SSH" ([doc] `structured-view.md:188-197`; ADR 0002).
- **No human audit log** (ADR 0002 line 18). Approval decisions live in the session's transcript event log only.

---

## 2. reet today (as read)

- Agents: `claude`, `codex`, `opencode`, `gemini`, `cline` enabled; `pi`, `cursor` disabled with reasons. Connector-owned presets, no controller-supplied command lines, every option confirmed from captured help or shipped disabled ([read] `internal/terminalhost/catalogue.go:11-40`). `ACP` field: claude/codex `adapter`, opencode/cline `native`, gemini empty (aoe uses `gemini --acp`; worth confirming, with the caveat that aoe's docs say Gemini CLI is deprecated upstream for consumer accounts [doc, unverified]).
- Sessions: tmux on the user's machines through a connector, states `creating|running|end_requested|ended|exited|lost` ([read] `internal/terminalproto/protocol.go:70-101`). xterm.js 6 front end (`web/src/lib/terminal/TerminalPane.svelte`).
- Attention (G) is designed but not built: terminal states are honest and coarse (`window_activity`, the bell, exit status), each labelled with its source; ACP exact states arrive with Program 2 (spec "Attention state", lines 747-770). No `activityAt/bellAt` code exists yet (grep of `internal/`).
- YOLO (F) is designed: account **Allow YOLO** (owner/admin, off), machine refusal, per-session choice with confirm, badge, audit `yolo-launch`; in ACP sessions **reet answers each request with `allow_once`, each audited as `yolo-permission`, never `allow_always`** (spec lines 61, 816-822).
- Program 2: ACP runner inside tmux on the machine, transcripts **on the machine only**, controller relays live events and fetches history on demand; confirmed, audited pinned-adapter installs for Claude/Codex; OpenCode first, probe not yet run (`handoff.md:50`; spec lines 62, 65, 951-952). W3 checklist: typed IDs, approval bound to an exact live request with expiry, no double execution across devices, never silent bypass (`handoff-history.md:3425-3440`).
- Notifications: an in-app centre and email (`internal/notify/notify.go:1-12`); **no web push, no service worker** (only `site.webmanifest`). Spec R9 defers outside-the-page notifications to Program 2.
- Accounts mode, roles (viewer cannot write), SSO, audit events, per-user rate limits, feature negotiation between controller and connector, and a strict "terminal text and prompts are never stored or logged" rule (`paste-research.md` section 2.3).
- Image paste: none today; designed in `paste-research.md` (private per-session upload dir on the machine, negotiated `upload-v1`, per-agent reference).

---

## 3. Comparison table

| Capability | aoe | reet today | Gap / opportunity |
|---|---|---|---|
| Agents in a terminal | ~17 CLIs + custom commands | 5 enabled, 2 disabled with reasons, no custom commands | aoe has breadth; reet's "confirmed or disabled" rule is the better default. Add agents as data rows, not code |
| Structured chat (ACP) | 10 agents, default web view | none (Program 2 on hold) | the main gap; aoe proves ACP works across many agents but at large cost |
| Per-agent integration | generic client + data profiles + a few specific hacks | connector catalogue (data) | keep reet's data-table shape for ACP too |
| Status | regex rules + pane scrape + hooks written into agent configs; ACP exact | lifecycle state only; G designed to be coarse and sourced | reet can be more honest (source labels) and exact via ACP; avoid scraping |
| Approvals (structured) | nonce, allow/always/deny, 300 s timeout, long-press heuristic | designed only | reet adds who decided, from which device, audited, exact-request binding across users |
| Approvals (terminal) | keystrokes into the pane from the TUI | none, by ruling R11 | do not copy |
| YOLO | adapter `bypassPermissions`, invisible per request | designed: reet answers `allow_once` per request, audited, account + machine gates | reet's design is strictly better; keep it |
| Questions / elicitation | form cards, schema-validated | none | Program 2 slice 2 |
| Sandboxing | Docker/Podman/Apple containers | none | worth a later program; not needed for chat |
| FS boundary | only for delegated ACP `fs/*` | n/a | do not claim a boundary you do not enforce |
| Env to agents | forwards SSH agent, X11, DBus to host sessions | connector launch (not reviewed in detail here) | minimal, allowlisted env is a reet differentiator; verify current launch env |
| Worktrees | rich, multi-repo | named worktree base, check-and-remove, worktrees in use | reet fine for now; multi-repo later |
| Diffs | full diff view | changed-file list designed (R19) | read-only diff panel (W2) is a cheap, high-value add |
| Resume / cancel | `session/load`, watchdogs, context primer | terminal reattach only | copy the primer idea; plan watchdogs from day one |
| Multi-machine | one daemon per machine, no fleet view | controller over many machines | reet's core advantage: cross-machine Needs you and approvals |
| Users / audit | single user, no human audit | accounts, roles, SSO, audit | reet advantage; extend audit to every approval |
| Mobile | PWA, native-text terminal, structured view designed for phones | responsive pages, xterm.js | chat view is the real mobile fix; add PWA install + push |
| Notifications | web push + sound, approvals immediate | centre + email; push deferred | push for approvals is the highest-value notification |
| Image/file attachments | terminal paste-to-path; ACP content blocks | none | terminal path designed; ACP blocks come free with chat |
| Prompt queue / steering | server queue, steering for Claude | none | later in Program 2 |
| Transcripts | SQLite on the daemon host, client localStorage cache | designed: on the machine only, never in controller or browser storage | reet's privacy stance is stronger; keep it |

---

## 4. Where reet can do better (ranked)

1. **Approval model with identity and audit.** aoe's nonce is good; copy that part. Then go further: bind each decision to the exact request id, the person, the device and the expiry; record an audit row per decision with metadata only (no command text if prompts are private; or a redacted preview, owner decision); first decision wins across devices and people, later ones get "already decided by X". aoe cannot do this: it has no principal on approvals (ADR 0002).
2. **YOLO that stays visible.** Keep the designed "reet answers `allow_once` per request, each audited" (spec line 61). aoe's `bypassPermissions` hands the decision to the agent and the record disappears. This also lets reet add a **policy** later (for example "auto-allow reads in this worktree") evaluated by reet, not by the agent.
3. **Cross-machine Needs you.** One queue of approvals, questions and failures from every machine, answerable from a phone. aoe cannot, by design. This is reet's strongest story and depends on Program 2 for the approval kinds.
4. **Honest status.** Do not scrape panes or write hooks into users' agent configs to guess "waiting". Keep G's sourced coarse states for terminal sessions and exact ACP states for chat sessions, always labelled ("from terminal activity" / "from the agent"). aoe's regex rules and screen detectors are clever but can be wrong silently.
5. **ACP structured chat, narrower and sturdier.** Start with OpenCode + approvals + text + tool cards + diffs, nothing Claude-specific. Plan, from the first slice, for what aoe learned the hard way: lossy errors (OpenCode), adapters that never close a turn, cancel ignored mid-tool, resume that silently starts fresh. Report each one honestly in the transcript rather than hiding it.
6. **Per-agent adapters as data, one generic path.** reet's catalogue already has an `ACP` column. Extend each row with: ACP command, minimum version (aoe uses 1.16.0 for OpenCode), env allowlist, supported content types, mode ids, and a "verified" date. Agents not verified stay terminal-only. Copy aoe's *shape* (registry + profile + compat floor), not its 10-agent breadth.
7. **Notifications that matter.** Web push only for approvals and questions first (immediate), then Failed/Finished. Avoid aoe's status pushes on heuristic states until the states are exact.
8. **Image and file attachments.** Terminal: ship `paste-research.md`'s private upload dir design (better than aoe writing into the worktree). Chat: send ACP image blocks only when the agent advertises them, with the same sniff and size limits, bytes never stored by the controller.
9. **Environment minimalism.** Do not forward SSH agent, X11 or DBus by default (aoe does). State what each agent receives.
10. **Projects/worktrees and diffs.** reet's folder projects are already cleaner than aoe's profiles. Add a read-only diff panel next (W2), then multi-repo later.
11. **Mobile.** The chat view is the real mobile fix (aoe's phone story is the structured view). Add a service worker only for push and install; keep terminal content out of caches.

---

## 5. What not to copy, and why

- **Answering terminal prompts by sending keystrokes** (`agents.rs:631-680`). Screen-scrape authority can type "1" into the wrong screen. reet's R11 already forbids it.
- **Writing status hooks into users' agent config files.** It changes files reet does not own, needs consent flows and uninstall paths, and still is "evidence, not the last word". Use ACP for exact state.
- **Regex status rules typed by users.** Hard to make honest; a wrong rule shows a confident wrong state.
- **`bypassPermissions` YOLO.** Loses per-request records.
- **"Destructive" substring heuristics as if they were safety.** Fine as a speed bump; never label a request "safe" from it.
- **Claiming a filesystem sandbox from ACP `fs/*` alone.** Native agents bypass it.
- **Forwarding SSH_AUTH_SOCK, DISPLAY, DBus to agents.**
- **Images written into the worktree** (`.aoe-pasted-images/`), cleaned by an in-memory 1 h timer that a restart cancels.
- **Breadth before depth**: 10 ACP agents, a bundled own agent, provider switching, agent switching, Prime-specific ownership receipts. aoe's docs show the maintenance tail (`structured-view.md:53-73`, `internals:7-90`).
- **A server-rendered terminal grid replacing xterm.js.** It explains aoe's good phone copy, but it is a rewrite; reet's planned phone "Select text" sheet gets most of the value.
- **Unauthenticated `--auth none` modes and URL tokens.** reet's accounts-only, cookie-only rules are stronger.

---

## 6. Risks

- **Program 2 size.** aoe's ACP layer is ~86k lines with tests across worker, API and UI. Even a tenth of that is more than the spike's 7-9 engineer-weeks if every recovery path is included. Re-size after the OpenCode probe, with explicit "out of first slice" lines.
- **Adapter churn.** aoe pins version floors and has per-version behaviour notes (OpenCode 1.16.0 changed classification and permission context). reet's connector must refuse below a floor and show the reason.
- **OpenCode specifics** (from aoe, unverified by me): errors dropped from ACP responses; `task` subagent streams no children; resume across restart is listed as supported (`structured-view.md:101`) [doc]. The probe should test exactly these, plus what OpenCode does with a permission request when no client answers (timeout behaviour).
- **Runner lifecycle.** Detached workers, reattach after controller or connector restart, stuck turns. reet's "runner inside tmux on the machine" is simpler than aoe's detached runner + control socket, but the controller is remote, so reconnect and replay must be designed (sequence numbers, bounded buffers, "history unavailable while offline").
- **Privacy vs a chat UI.** Chat makes prompts and tool output visible in the browser; the spec says transcripts stay on the machine. Do not add aoe's localStorage transcript cache.
- **Approval races** across people and devices, and expiry while a phone is locked.
- **Status mismatch** between a session switched between chat and terminal (aoe needed a whole handoff protocol, `internals:29-38`). Recommend: a session is chat or terminal for its life in the first slices.

---

## 7. Proposed sequencing relative to the existing plan

Current order (`handoff.md:39-50`): E merged; F and G deferred; H not started (though `3eaf4f9` adds a Sessions page); Program 2 on hold for the probe; paste work T1-T14 designed.

1. **Now, small and independent:** paste T1 (sanitiser, a real security fix) and T2 (bracket-aware review), T3/T4 (copy). Client-only.
2. **Run the OpenCode ACP probe** (owner go-ahead) in parallel with H. It is the gate for everything structured. Probe list: handshake capabilities, permission request shape and options, no-answer timeout, cancel mid-tool, `session/load` after killing the runner, error detail, image content support, mode ids, env needed.
3. **H (session screen)** with the Chat | Terminal switch disabled (R21), the context panel and the read-only changed-files list. Keep the decision card's slot.
4. **G (attention), terminal part**: sourced coarse states and the cross-machine Needs you. Built so ACP kinds slot in.
5. **Program 2 slice 1 (OpenCode):** runner in tmux, transcript on the machine, text + tool cards + approval cards (Allow once / Deny only; no Always in slice 1), per-decision audit, first-decision-wins across devices, timeout → cancel, honest notices for lost runner and context reset.
6. **F (YOLO)** right after slice 1, so ACP YOLO is reet answering `allow_once` with audit from the start; terminal YOLO uses the confirmed flags as designed.
7. **Web push for approvals** (service worker for push only) once approval cards exist.
8. **Image upload:** terminal T8-T12 can go before or after slice 1; ACP image blocks join Program 2 slice 2 with questions/elicitation and the mode/model pickers.
9. **Then more agents** as catalogue rows, each verified: Claude via pinned adapter (audited install), Codex via adapter, Cline native, Gemini if still viable.

---

## 8. Open decisions only the owner can make

| # | Decision | Recommendation |
|---|---|---|
| O1 | Run the hands-on OpenCode ACP probe now, in parallel with H? | **Yes.** Program 2's size and shape depend on it; aoe's code is a good checklist for what to probe. |
| O2 | Offer **Allow always** in approval cards? | **Not in slice 1.** Allow once / Deny only; later, "always" as a reet-side policy rule that reet evaluates and audits, not the agent's `allow_always`. |
| O3 | Is a session chat or terminal for its whole life? | **Yes, fixed at launch** for the first slices. Switching cost aoe a large handoff protocol. |
| O4 | Approval audit content: metadata only, or include a redacted command preview? | **Metadata plus the tool kind and a short, redacted target** (for example "shell command in worktree X"), never full prompts or output. Owner call on how much. |
| O5 | Who may approve: only the session's starter, or any writer on the account? | **Starter plus owners/admins**, first decision wins, others see who decided. |
| O6 | Status for terminal sessions: stay coarse (bell/activity) or add agent hooks like aoe? | **Stay coarse and labelled.** Exact states only from ACP. |
| O7 | Web push in scope, and for which events? | **Yes, approvals and questions first**, after slice 1; status pushes only for exact states. |
| O8 | Add container sandboxing as a later program? | **Later, separate program.** Do not imply a sandbox meanwhile. |
| O9 | Breadth target for ACP agents | **OpenCode, then Claude (pinned adapter), then Codex.** Each verified; the rest stay terminal-only. |
| O10 | Keep transcripts on the machine only, accepting "history unavailable while offline"? | **Yes** (spec line 62). It is a privacy differentiator over aoe; say so in the UI. |
| O11 | Image placement for terminal paste (paste-research D1) | **Private per-session dir on the machine** (D1-A), as recommended there. |
| O12 | Environment passed to agents | **Confirm reet passes a minimal allowlisted env**; never forward SSH agent or display variables by default. I did not read reet's launch env in this pass. |

## 9. What this report did not establish

- I did not run aoe or any agent. Agent-side ACP behaviour (OpenCode, Claude adapter) is from aoe's code and docs plus memory.
- Whether claude-agent-acp routes file reads/writes through client `fs/*` is from memory [belief].
- aoe's docs are detailed and sometimes ahead of or broader than the code I read; where I only read docs it is marked [doc].
- I did not review reet's connector launch environment or the new Sessions page commit (`3eaf4f9`) in detail.
- The Program 2 spike report was not available.
