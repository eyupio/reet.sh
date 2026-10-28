# Reet central-control implementation handoff

reet.sh is a self-hosted AI fleet control room: machines, the AI agents installed on them, skills,
reviewed sync, browser terminals and, now, an agents home with folder projects. This file is the
current state and the standing rules; start here. Where things are: [README.md](README.md) and
CONTRIBUTING.md; [docs/](docs/) for shipped behaviour (for example
[control centre](docs/control-centre.md), [terminal workspace](docs/terminal-workspace.md),
[projects](docs/projects.md), [folder browse](docs/folder-browse.md),
[remote machine operations](docs/remote-machine-operations.md),
[controller terminal commands](docs/terminal-controller-commands.md) and
[voice and copy](docs/voice-and-copy.md)); the agents-home mock-up in
[docs/design/agents-home/](docs/design/agents-home/), its
[plan](docs/superpowers/plans/2026-10-26-agents-home.md) and
[spec](docs/superpowers/specs/2026-10-26-agents-home-design.md); what each UI change was checked
against in [docs/ui-verification.md](docs/ui-verification.md); the research behind the queued work in
[docs/research/](docs/research/); and older narrative in
[handoff-history.md](handoff-history.md).

**Docs tidy-up (27 October 2026).** Finished plans, specs, stage documents and audits were removed
on 27 October 2026; they live in git history before commit `bd9e56c` (for example
`docs/superpowers/plans|specs/2026-10-2[145]*`, `docs/agent-setup-plan.md`, `remote-machine-*` and
`docs/remote-machine-stage*.md`, and the root files `FEATURES_AUDIT.md`, `GEO-ANALYSIS.md` and `UI_AUDIT.md`). Older per-feature delivery notes (single sign-on PR logs, terminal
sudo repair, roadmap checkpoints, terminal polish, terminal-first workspace requirements, the
implemented baseline, S/A/W/L task lists, start-agent-session protocol notes, Projects PR 1 and
delivered, phone terminal/zoom/dialog notes, the ready-to-use continuation brief) are in
[handoff-history.md](handoff-history.md), under "Moved from handoff.md on 27 October 2026".

## Agents home redesign (27–28 October 2026)

### Resume here (paused after PR H, 28 October 2026)

**The owner asked to pause after PR H.** Nothing of the queued work below has been started.

- **Branch and PR:** `ccr-7a7f5a9b-to5imh` is [PR #231](https://github.com/eyupio/reet.sh/pull/231) (PR H). It holds
  H1–H4 and the final fix wave after the broad review (list in the state table), and is awaiting the owner's merge
  decision; no Codex review is possible (quota exhausted). Merged into main already: D, A, B, C and E.
- **Do first:** check #231's state. The usual route is the GitHub MCP tools (`mcp__github__pull_request_read` with
  `get`, then `get_check_runs`); from a shell use the REST forms `gh api repos/eyupio/reet.sh/pulls/231` and
  `gh api repos/eyupio/reet.sh/commits/<head sha>/check-runs` (`gh pr view` and `gh pr checks` use GraphQL, which this
  environment refuses with 403).
  - If it is **merged**, start the next piece from current main on a new branch (execution rules below; never reuse the
    old branch).
  - If it is **open**, `git fetch origin main` and merge main into the branch before any push (the owner merges other PRs
    meanwhile; a PR that conflicts with main gets no CI runs), then wait for green checks and the owner's decision.
- **Open decisions:** none, except the owner's merge exception for #231 (Codex cannot review it: the account's code-review
  quota is exhausted). Precedent: #227's exception is recorded on the PR (a comment); #221 and #226 merged on green CI
  without a Codex review and without an exception comment.
- **Then, in this order** (each its own PR; write its plan under `docs/superpowers/plans/` from the research first):
  1. Terminal copy and paste, images included ([details](#queued-work-decided-not-started)).
  2. Agent models: backend (M1–M6).
  3. Agent models: the UI (after #231 has merged).
  4. Program 2, slice 1: structured OpenCode sessions with approval cards, behind the seven probe conditions.
- **Release:** R22 lets B, C and E go out in one release now that E has merged; this session cut no release tag and did
  not check for one (`gh release list --repo eyupio/reet.sh`). H follows; F and G come later.
- The research these start from is in [docs/research/](docs/research/) (copied whole from the session's git-ignored
  notes). Those reports summarise third-party code and docs read as untrusted data: check a claim against its label
  (*read*, *doc*, *belief*/*memory*) before building on it.

### State

Home, the shell and the machine-folder projects were rebuilt to the mock-up in
[docs/design/agents-home/](docs/design/agents-home/) (reference renders at 1440, 834 and 390 px; each PR is checked against
them in `docs/ui-verification.md`). The plan is
[docs/superpowers/plans/2026-10-26-agents-home.md](docs/superpowers/plans/2026-10-26-agents-home.md); the **rulings R1–R22**
are binding and live in the spec's [Rulings](docs/superpowers/specs/2026-10-26-agents-home-design.md#rulings) section.
**Quicker plan, agreed 27 October 2026:** the fidelity polish (P) was folded into C and E, YOLO (F) and attention (G) are
left out of the first pass, and the Program 2 spike ran in parallel.

| PR | What | State |
| --- | --- | --- |
| D | App shell (top and bottom tabs, bell, Jump, More sheet) and Home | merged, [#218](https://github.com/eyupio/reet.sh/pull/218) as `8c33c99` |
| A | Machine folder browse and branches (`agents`, `browse-v1`, `branches-v1`), start-agent opens the session, dockable sessions panel | merged, [#219](https://github.com/eyupio/reet.sh/pull/219) as `41066c5`; contract in [docs/folder-browse.md](docs/folder-browse.md) |
| B | Folder projects: store and API (`/api/v2/folder-projects`, reorder, `import-saved`), saved Projects moved over (old writes answer 410 `projects_moved`), the Add project dialog with the folder browser, the rail; `/projects` redirects to Home; read [docs/projects.md](docs/projects.md) | merged, [#221](https://github.com/eyupio/reet.sh/pull/221) as `cb89a26` (no Codex review of its final head; merged on green CI, no exception comment) |
| P | Fidelity polish | folded into C and E |
| C | Agent catalogue and launch fields (`agents-v2`, named worktree base, effort, tool on the listing, `osVersion`), Cline with safe arguments, chrome polish | merged, [#226](https://github.com/eyupio/reet.sh/pull/226) as `540d247` (no Codex review: quota exhausted; merged on green CI, no exception comment) |
| E | Launch bar on Home, Start in several projects, fixes from the #226 review; read [docs/start-agent-session.md](docs/start-agent-session.md#on-home-the-launch-bar) | merged, [#227](https://github.com/eyupio/reet.sh/pull/227) as `fe3e9b7` (owner exception, recorded on the PR) |
| F | YOLO (off by default, per session, audited, machines can refuse) | **deferred past the first pass**; the design decisions stand; the launch bar shows it disabled and the API refuses it (409 `yolo_refused`) |
| G | Attention and Needs you | **deferred past the first pass**; Needs you stays an honest empty state |
| H | `/sessions` (the Sessions page), the session screen, tablet and phone layouts, the last redirects, the phone rail on Sessions | **[#231](https://github.com/eyupio/reet.sh/pull/231), in review, awaiting the owner's merge decision** (no Codex review possible: quota exhausted). H1 `f0bffe5` Sessions page and Control's address retired (`/control` → `/`, `/terminals` → `/sessions`), fixes `048e431` (session colours, **Saved commands** button: owner ruling), `b85338a`; H2 `f74001f` session screen (tabs, header with the state pill, Chat \| Terminal with Chat disabled, context panel), fixes `f026cfb`, `c02fec6`; H3 `7581ad0` tablet and phone layouts (no rail on narrow Home, the rail on Sessions, slim phone header, `keyboardChrome('session')`), fixes `a984b5e` (44 px tablet targets, cases moved to `sessions-phone.e2e.mjs`), `3f1e89b` (separator grip); `521c406` CI fix (first-terminal checklist off `/sync`); merges of main `9c88a46` (#232, #233), `ae763c8` (#234) and `57d8f53` (#235, #236); H4 `1a5018d` (research copied into `docs/research/`), `c349a2b` (the matrix, phone target fixes, docs, this handoff), `1ff4230` (the research notes name the reference tool by role); final fix wave after the broad review `b53c7dd` (`/sessions/`, viewers at a session address), `adec499` (steady Sessions rows, menu focus, the connection in the phone header, tablet swatches and grip, context panel, tab tooltip, New → `/#launch`), `dd31068` (dead code), `8e6ce3e` (checks that went with Control, steadier phone tests), `6778211` (start-agent expects the Machines title) and the docs commit after them |
| Program 2 | Structured chat over the agent protocol, with approval cards | spike done; OpenCode chosen (27 Oct); hands-on probe of `opencode acp` 1.19.5 done (28 Oct): conditional go for trusted workspaces only; queued as slice 1 below |

Merged into main by the owner while this work was open: #222, #224, #225, #228, #229, #230, #232, #233, #234, #235 and
#236 (each merged into the open branch before its next push; #235 and #236 in `57d8f53`).

**Merge rule:** a PR in this work merges only when all checks are green on its final head **and** Codex has reviewed it
cleanly, or the owner gives an explicit exception. So far: #227's exception is recorded on the PR (a comment); #221 and
#226 merged on green CI without a Codex review and without an exception comment; #231 is waiting for the owner's decision.

**Rulings beyond R1–R22** (all recorded in the session notes and kept here so they survive):

- Owner, 27 Oct: the quicker plan above; Pi stays disabled; Cursor stays blocked until its installer host is allowed.
- Owner, 28 Oct: a **Saved commands** button on the Sessions page; copy and paste is its own PR after H, with pasted
  images in a private per-session folder (0700/0600, random names); Program 2 slice 1 is trusted workspaces only (OpenCode,
  Allow once / Deny only) behind the seven probe conditions; the order after H is copy/paste, models, Program 2; pause after
  PR H.
- Controller defaults, 28 Oct (owner may revisit): models D2 a per-machine default is deferred, D3 a favourite can be the
  "default for this agent", D4 the model-name rule stays as it is.

### Limits and known gaps

After PR H:

- The session screen keeps the sessions panel and the workspace toolbar (the mock-up has neither); the tabs share the
  toolbar row, and the pane keeps its own header (some information is shown twice). A later PR may drop the panel there.
- Old `/sessions` links and sign-in resumes that reach an older server (a rollback) land on `/` or on an unknown page:
  PR H changed no Go protocol and nothing redirects `/sessions` there.
- `/sessions` with `session`, `agent` or `view=terminal` in the address is the workspace; bare `/sessions` is the list.
  The app has no client-side router: links are full page loads.
- Sessions outside a project are grouped "Other on <machine>" only (the listing has no folder to match a project by).
  The **Needs you** filter matches nothing until G. Check and remove worktree is offered only when the worktree path and
  branch name the run.
- `railState` puts "N running" ahead of "machine offline", so a project on a machine that is not reporting shows its
  last known count without saying so.
- Two sentences the spec does not word: **Start in these on Home** and "Projects now live on the Sessions screen."
- The site footer (account name, Admin, the developer credit) has targets under 44 px on touch; it predates the agents
  home and was left as it is.
- On a touch phone the terminal's chrome (workspace toolbar, pane header and footer, terminal keys) is 40 × 40 px, not
  44, so the terminal keeps its rows; documented in [docs/terminal-workspace.md](docs/terminal-workspace.md#phones). H4's
  40 px footer button costs one row with the keyboard down at 390 × 844 (39 → 38); keyboard-open and sideways rows are
  unchanged.
- `terminal-phone.e2e.mjs` failed twice locally during PR H, each time in a long sequential run (its 20 × 8 floor case
  read 13 rows), and passed every run on its own (more than ten). Its test and the sizing code are unchanged by PR H's
  fix wave. Watch it in CI's `terminal-browser` job.

Still true from earlier PRs:

- YOLO is shown disabled and refused (409 `yolo_refused`) until F. The first line of a prompt becomes the stored session
  title. The Start an agent dialog (Machines and the Sessions panel) still uses the old four-agent list, so Cline starts
  only from Home's launch bar. Pi and Cursor stay disabled.
- A base a member only inherits from the account's default is not sent to a machine without `agents-v2`; a project's
  own base is sent, and such a machine refuses it with "Update reet on that machine…". Rolling back to an older server
  while a start in several projects has members pending may fail those members or start them without base or effort.
- A replayed start is checked again against today's rules; a start in several projects checks "one start per folder" by
  machine and folder.
- The old Projects tables and their start/retry routes stay one release before they are dropped; a downgrade then
  re-upgrade leaves Projects saved while downgraded as "waiting folders"; a session's `projectId` is checked for account
  and machine, not folder; the import's answer still says "saved repositories".
- Worktree git calls share the lazy-fetch and fsmonitor hazard the folder browser already guards against.
- Nothing has been checked on a real phone or in Safari (browser emulation only), including the touch long press.
- The tablet shows its own **Change** button for model and worktree (approved deviation). Per-screen differences from
  the drawings are listed per PR in [docs/ui-verification.md](docs/ui-verification.md).

### Queued work (decided, not started)

**1. Terminal copy and paste, images included** (its own PR, next). Input:
[docs/research/paste-research.md](docs/research/paste-research.md) (tasks T1–T14). Decisions:

Numbering as in the report's section 3.6 (D1–D8):

- **Owner decisions (28 Oct):** its own PR after H; **D1** pasted images go to a private folder per session on the machine,
  outside the project (directory 0700, files 0600, random names).
- **Controller defaults (owner may revisit):** **D2** skip the paste review for agents that use bracketed paste when the
  text has no control characters and is at most 64 KiB (and always strip ESC and the bracketed-paste markers); **D3** an
  agent's own copy (OSC 52) reaches the clipboard only right after a user gesture, write-only, at most 1 MiB; **D4**
  copy-on-select is an option, off by default; **D5** uploads are audited as metadata only; **D6** uploads are on by
  default behind a new `upload-v1` feature token, which a machine accepts unless a local setting turns it off; **D7**
  limits: 5 MiB per image, 20 images or 50 MiB per session, 24 h lifetime, PNG/JPEG/GIF/WebP checked by magic bytes, no
  SVG; **D8** phones get a file picker and a **Select text** sheet first, drag and drop after.

**2. Agent models** (backend M1–M6, then the UI after #231 merges). Input:
[docs/research/models-design.md](docs/research/models-design.md) (tasks M0–M11).

- New feature token `models-v1`. In v1 only OpenCode lists models from the machine: `opencode models` run with
  `OPENCODE_DISABLE_MODELS_FETCH=1`, so it makes no network call (the probe showed it calls models.opencode.ai otherwise,
  and that `--refresh` ignores the setting). Other agents keep suggestions, favourites and Other….
- Favourites per account (at most 50); an account default for sessions outside a project; a project default for agent,
  model and effort. Precedence: explicit choice, then project, then account, then the agent's own default.
- Owner: the bar opens with the defaults first and the last used as a Recent group; loading is automatic because the
  capture is clean with the setting above. Controller rulings D2–D4 above. D6 (capturing `opencode models` from the
  approved npm package) was approved and is partly done by the probe.
- Known gap to fix in it: a project's default agent is checked against the original four agents only, so Cline cannot be
  one.

**3. Program 2, slice 1: structured OpenCode sessions** (after models). Input:
[docs/research/probe-report.md](docs/research/probe-report.md),
[reference-tool-frontend-research.md](docs/research/reference-tool-frontend-research.md) and
[reference-tool-agents-research.md](docs/research/reference-tool-agents-research.md). Owner decision: trusted workspaces only, OpenCode, Allow
once / Deny only, and only with all seven conditions:

1. A reet-owned ask-everywhere config in `OPENCODE_CONFIG_CONTENT` (edit, bash and external_directory ask, for every
   built-in agent; `default_agent: "build"`; `task: deny`; `webfetch: deny`), `OPENCODE_DISABLE_PROJECT_CONFIG=1`, and the environment
   scrubbed of `OPENCODE_PERMISSION`, `OPENCODE_CONFIG`, `OPENCODE_CONFIG_DIR` (and any `OPENCODE_*` reet does not set);
   check the mode is `build` after `session/new`.
2. A random `OPENCODE_SERVER_PASSWORD` per process: every `opencode acp` starts an unauthenticated local HTTP API on
   127.0.0.1:4096 that can answer permissions behind the client's back.
3. `task: deny` (subagent permissions never reach the protocol).
4. A permission timeout and a turn watchdog, with reet closing its own pending cards on cancel.
5. An environment allowlist and the opt-outs (models fetch, autoupdate, default plugins, LSP download and the rest).
6. A warning when the repository has `.opencode` plugins (repository plugins run at session start).
7. OpenCode pinned at exactly `opencode-ai@1.19.5` (integrity in the probe report). There is no `--standalone` or
   `--auto` on `acp` in 1.19.5.

**The probe's blocking findings** (full text: [probe-report.md](docs/research/probe-report.md), sections 4, 6 and 7):

- By default OpenCode asks nothing: writes, edits and shell commands ran without a permission request.
- A repository's config, the user's config or `OPENCODE_PERMISSION` can switch asking off.
- Every ACP process serves an unauthenticated control API on a predictable local port; an approval over it leaves the
  ACP card pending.
- Repository plugin code runs when a session opens, whatever the flags: no-go for untrusted repositories.
- No permission timeout; subagent permissions never reach the client, so a turn can hang; provider errors take ~74 s.
- "Always allow" stores a wildcard broader than the command shown; writes land in the user's `.git`.
- It contacts npmjs for `@opencode-ai/plugin` even with the opt-outs set.

**What not to copy from the other front end studied** (full lists: section 5 of each `reference-tool-*` report):

- Answering terminal prompts by sending keystrokes; regex status rules; writing hooks or trust into users' agent configs.
- Reading or writing agents' private stores (session databases, transcripts); use the protocol's load and list or nothing.
- YOLO that disables the agent's own sandbox, or set through an environment variable that `ps` cannot show.
- Forwarding `SSH_AUTH_SOCK`, `DISPLAY` or DBus to agents; `npm install -g` from a web button; `@latest` hints.
- Pasted images written into the worktree and cleaned by an in-memory timer.
- A `--help` probe on every launch as the main evidence (keep reet's captured help with integrity; add a `--version`
  check only).
- Breadth before depth (seventeen agents); unauthenticated modes and tokens in URLs.

### Pitfalls for the next session

- **CI** runs on the Zoomies runners (`zoomies-linux-x64`); tmux and Chromium are installed per job.
  - `terminal-browser` takes 17–25 of its 30 minutes: **add no fixtures or cases to that job**.
  - `phone-zoom` and `phone-dialogs` (10 minutes each) have headroom; new browser cases go there.
  - CI's Chromium (1248) has no DejaVu Sans Mono, so terminal rows differ from a local run: never assert an absolute row
    count; compare with a layout measured in the same browser.
  - The real-browser Go harness needs `LANG=C.UTF-8 LC_ALL=C.UTF-8` locally, or terminal.e2e's UTF-8 split check fails.
  - CI does not run workflows while a PR conflicts with main.
  - `mcp/test/oauth.test.mjs` needs its fixture built first in CI (the warm-up step in `image.yml`).
- **Merges:** the owner merges other PRs while one is open (#228–#236 during this work). Always `git fetch origin main`
  and merge it before pushing.
- **Git and processes:** never `git checkout -p`; commit with pathspecs; never `pkill -f` (it can match the session's own
  processes); never a shared tmux `kill-server`. The stop hook nags while subagents are still working; that is expected.
- **Commit trailers:** new commits follow the session's attribution reminder (`Co-Authored-By` and `Claude-Session`);
  this PR's commits use Claude Sonnet 5.5 (a few early ones say Opus 5.5).
- **Process used** (subagent-driven development): an implementer per task with a brief, a task review, fix rounds
  (at most five) with a scoped re-review, then a final broad review split by area and a single fix wave; mutation checks
  restore files byte-identical (`cp` before, `cmp` after); fidelity checks at 1440, 834 and 390 px recorded in
  `docs/ui-verification.md`. The full local matrix is `web/tests/matrix/agents-home.matrix.mjs` (not in CI).

**Rule for this work: every PR updates this block (state table, limits, queued work) before it merges.**

### Standing notes carried over from the moved sections

- Terminal security: a later, user-agreed security pass for privilege-capable browser terminals (a PIN
  or similar preference) is deferred; nothing of it is implemented. Read "Later: user-controlled
  terminal security preferences" in [handoff-history.md](handoff-history.md) before starting it.
- A reported terminal-stop incident (Stop/End with removal interrupting other tmux work) is unresolved;
  do not disturb the user's work or ask them to reproduce it. Never use a shared or default tmux
  `kill-server`. Details and what to collect if it recurs: "Reported terminal-stop incident" in
  [handoff-history.md](handoff-history.md).
- Shared dialog shell for phones: `web/src/lib/dialogShell.ts` and `web/src/dialog-shell.css`; how to
  adopt it is in "Phones: dialogs" in [handoff-history.md](handoff-history.md).
- Product acceptance and evidence boundaries: discovery is not install capability, shell persistence
  is not conversation recovery, configuration restore is not runtime health, and local checks are not
  a deployment; describe outcomes without competitor names (full text in the history).

## Code map: read before proposing new types or endpoints

| Area | Existing entry points |
| --- | --- |
| App/routes and the shell | `web/src/App.svelte`, `web/src/lib/shell/*` (AppShell, TopTabs, BottomTabs, MoreSheet, JumpSearch, Bell) |
| Home and projects | `web/src/lib/home/*` (Home, FleetStrip, InFlight, NeedsYou, ProjectRail, ProjectRailHost, AddProjectDialog, FolderBrowser), `web/src/lib/home.ts` |
| Launch bar | `web/src/lib/launch/LaunchBar.svelte`, `SlotPicker.svelte`, `web/src/lib/launchBar.ts`, `web/src/lib/catalogue.ts` |
| Sessions page and session screen | `web/src/lib/sessions/*` (SessionsPage, SessionScreen, SessionTabs, ContextPanel), `web/src/lib/sessions.ts` |
| Machines/onboarding | `Machines.svelte`, `MachineFleet.svelte`, `MachineSetup.svelte`, `machineSetup.ts`, `machines.ts` under `web/src/lib/` |
| Sync/fleet UI | `EnvironmentSync.svelte`, `SyncControl.svelte`, `FleetActions.svelte`, `fleet.ts`, `api.ts` under `web/src/lib/` |
| Live sync domain/execution | `internal/scan/sync.go`, `agent.go`, `restore.go`; adjacent tests |
| Desired sets/jobs/receipts | `internal/store/store_sync.go`, `store_sync_control.go`; `internal/api/sync.go`, `sync_control.go` |
| Store/migrations and route registration | `internal/store/store.go`, `internal/api/api.go`; inspect feature-local initialisation too |
| Hosted bundle validation/editor | `internal/store/store_hosted.go`, `internal/api/hosted.go`, `web/src/lib/Hosting.svelte` |
| Read-only content manifests (S1) | `internal/skillbundle`, [contract](docs/skill-bundle-manifests.md); independent of live sync/snapshot formats |
| Snapshot whole-skill collection/verification | `internal/setup/skills.go`, `skills_verify.go`, `snapshot.go`, `restore_apply.go`, `rollback.go` |
| Agent adapters/discovery/evidence | `internal/setup/adapters.go`, `discovery.go`, `installation.go`, `health.go`, config/policy files and tests |
| Setup profiles | `internal/setup/profile.go`, `internal/profilehistory`, `internal/store/store_setup_profiles.go`, `internal/api/setup_profiles.go` |
| Host shell registry and services | `internal/terminalhost/manager.go`, `manager_tmux.go`, `registry.go`, `service.go`, `readiness.go` |
| Typed private transport and broker | `internal/terminalproto`, `internal/terminalagent`, `internal/terminalbroker`, `internal/privateconn` |
| Terminal API and records | `internal/api/terminal.go`, `internal/store/store_terminal.go` |
| Terminal UI and connection state | `web/src/lib/terminal/TerminalWorkspace.svelte`, `TerminalPane.svelte`, `api.ts`, `connection.ts` |
| Controller enablement | `cmd/reet/terminal.go`, `internal/terminalcontrol`, `docs/terminal-controller-commands.md` |
| Machine health | `internal/agenthealth`, `internal/store/store_remote_health.go`, `internal/api/health.go` |
| Notifications | `internal/notify`, `internal/store/store_notify.go`, `internal/api/notifications.go`, `web/src/lib/Notifications.svelte` |
| Agents home fixtures | `web/tests/home.e2e.mjs`, `launch.e2e.mjs`, `add-project.e2e.mjs`, `sessions.e2e.mjs`, `sessions-phone.e2e.mjs`; the local matrix `web/tests/matrix/agents-home.matrix.mjs` |
| Existing regression fixtures | `internal/store/store_sync_control_test.go`, `web/src/lib/fleet.test.mjs`, `web/tests/control.e2e.mjs`, `web/tests/terminal.test.mjs`, `web/tests/terminal.e2e.mjs`, `internal/api/terminal_integration_test.go` |

Names such as `Skill`, `SyncSkill`, `SyncTarget`, `SyncPlan`, `SetupSkill`,
`SnapshotSkill`, `HostedSkill`, `TerminalSession` and `SyncAgent` already exist.
Read their actual contracts before adding a similarly named concept. Especially,
`SyncPlan.Hash()` hashes the planned actions; it is not an installed-content hash.

## Safety and conflict handling

- Restore verification uses format-specific limits: configurations 1 MiB, whole
  skill folders 8 MiB, supported memory databases 64 MiB. Capture/restore agree.
- An action is marked as written before verification, so a verification or journal
  persistence failure includes the current action in rollback. Write errors propagate.
- Codex memory restore/verification uses the destination's effective CODEX_HOME.
  SQLite sidecars are rechecked immediately before writes. Destination agents must
  be stopped; sidecar absence does not prove an application is inactive.
- Memory verification continues after a blocked database. Each skill produces one
  final result; an unsafe file cannot also make the skill count as verified.
- The restore wizard prints mapped paths and warnings, asks per-destination conflict
  choices and confirms the final plan. Incomplete file verification returns an error, including the standalone verify command; JSON reports are still emitted.
- Server profile writes require an identical history or an exact append, with an
  atomic payload comparison protecting concurrent writers. Stale/divergent pushes
  return 409. Web edits carry the viewed base revision; stale edits are rejected.
- Local pulls preserve divergent/newer local histories. Preserve a local copy with
  `reet setup profile rename NAME NAME-local`, then pull the server copy and review.
  Local saves/pulls/rename/remove share an exclusive `profiles/.write.lock`.
  Remove a stale lock only after confirming its owning process has stopped.
- A failed profile deletion can be retried without reloading the application.

## Installation and release changes

The shell installer verifies checksums, runs the staged candidate's version before
replacement, preserves a regular existing binary as `reet.previous`, and reports
PATH shadowing. It refuses an existing symlink rather than silently replacing it.
Existing data and managed deployment files remain separate.

Dev publication keeps legacy rolling names for existing clients, and also uploads
commit-addressed archives `reet_dev_SHA_OS_ARCH.tar.gz` with `checksums-SHA.txt`.
Those assets are checked on a rerun and never overwritten. Only after they are
available does the publisher update `dev-build.txt`. New shell installers and Go
upgrades resolve that pointer once and download the corresponding immutable pair.
Only a missing (404) pointer falls back to legacy releases; other errors fail closed.
No assets, tags, deployments or production hosts are published/upgraded by local tests.


## Accepted product direction and priority

The user wants reet.sh to be the central control unit for multiple machines:
maintain skills on reet, connect machines, import local skills, distribute to one,
selected, or all machines, review changes in both directions, and use installed
AI agents through the web terminal. Agent software install/update/remove and
cross-machine configuration management are future requirements. The user accepted
the central-control roadmap on 24 October 2026 and requested this detailed handoff.
This records the accepted direction; this turn authorises the handoff update,
not implementation of every milestone or production changes. Future sessions
should implement the task requested within this direction, and verify its gate.
Later steering prioritises the terminal workspace as the main everyday use case.

Priorities, in order (terminal UX moved first by the user on 24 October):

1. **T — Make terminals the primary usable workspace:** viewport layout, clear
   session navigation/controls and safe history maintenance (T1–T3). T1–T9 are
   merged (see handoff-history.md).
2. **S — Complete the skill foundation:** versioned bundles, content-level drift,
   reviewed local edits, safe one/many-machine transfer and verification.
3. **A — Make remote work convenient:** registered agent capabilities, project
   selection, explicit launch, semantic activity and an attention queue.
4. **W — Make parallel work reviewable:** managed worktrees, files/diffs and
   structured agent interaction where supported.
5. **L — Fleet lifecycle:** observed agent versions, reviewed install/update/remove,
   desired MCP configuration and optional isolated agent sessions.

Conversation resume/fork, granular machine/project permissions, read-only terminal
observation and OIDC login are valuable later extensions. A full terminal TUI,
themes, sound effects and a broad plugin marketplace are lower priority than the
outcomes above. Keep the warm Reet branding, Yorkshire rose and familiar
navigation; read [voice and copy](docs/voice-and-copy.md) for user-facing changes.

The older setup plan records an earlier delivery order (backup/restore, profiles,
maintenance). Preserve its data-safety and recovery requirements. This roadmap is
the newer ordering for the central-control product, with terminal UX now first;
do not attempt complete setup recovery before the bounded workspace/content tasks. Do not label partial file
recovery as a fully working recovered agent environment.

## Execution rules for the next session

1. Read this file, CONTRIBUTING.md, applicable AGENTS.md files, and the entry-point
   files for the single task selected below. Check the actual checkout; paths and
   branch state here are a dated baseline, not permission to reset local work.
2. Run `git status --short`, inspect the current branch/log and fetch the current
   base when network/authentication are available. Preserve existing edits.
   Confirm the previous slice's state through its live PR/CI/merge record, then start
   from current main and preserve local edits.
3. Pick **one bounded next slice**: the open agents-home work above, or from the
   continuation lists in the S4 records (now in handoff-history.md) and
   docs/skill-content-receipts.md; the user chooses the order. T1–T9, S1–S3 and the
   S4 planner, pins, preview, ownership receipts and review UI are done; do not restart them. Do not build an execution
   control before its prerequisite is present or turn the whole roadmap into one PR.
4. Before editing, state the task ID, files affected, expected behaviour and checks.
   Resolve routine implementation choices from existing conventions. If a domain
   decision genuinely remains open, record the question and continue independent
   work; never invent an unsafe default just to make a control appear functional.
5. Keep domain code separate from HTTP handlers, SQLite records and Svelte views.
   Reuse existing parsing, bounds, ownership, locks and recovery code where its
   contract fits. Do not copy a whole subsystem or introduce a dependency casually.
6. Add meaningful regression tests for new runtime behaviour and failures. Use
   synthetic homes, databases and fixtures; do not enroll or alter real machines
   or invoke a real paid AI provider as a test side effect.
7. Show observed/unknown/stale/incomplete/unsupported states distinctly. A request
   accepted by the controller is not an observed successful change on the host.
8. Update this file with completed task IDs, commit/PR, checks, limitations and the
   exact next task. Keep the stage-per-PR workflow and required CI. Do not merge,
   publish a release or change production merely because local tests pass.

### Terms that must not be conflated

| Term | Meaning |
| --- | --- |
| Reet connector / `reet agent` | Background machine enrollment, reporting and command transport; not Claude Code or Codex |
| AI agent application | A CLI such as Claude Code, Codex or OpenCode |
| Terminal session | A persistent shell; `running` does not mean an AI agent is working |
| Agent run | Proposed record binding a terminal/runtime to a registered AI agent, machine and project |
| Skill scope | A particular agent/global/project destination; several scopes may share physical files |
| Library membership | Personal shelf/list organisation; not install ownership or deletion approval |
| Desired set | Saved installation intent for a target; not a verified filesystem observation |
| Bundle revision | Proposed immutable skill contents and provenance; not a shelf row timestamp |
| Setup profile | Existing revisioned snapshot/exclusion workflow; not automatically a live agent launch profile |
| MCP endpoint | Reet's own API for clients; distinct from configuring MCP servers used by other agents |

## Validation commands and interpretation

For documentation-only changes, check Markdown links, whitespace and consistency
with the inspected code; do not rerun the entire application suite just to validate
prose. For implementation, run targeted regression tests while developing, then
required repository checks before PR handoff. Use tools from the current environment;
never assume a previous session's binaries or caches are present.

Prerequisites: versions from `go.mod` and `web/package.json` (at this baseline Go
1.27.1 and Node 22), dependency installation when missing, tmux 3.2+ for terminal
service/integration tests, and a supported Chromium for browser tests. Read the
managed environment runtime instructions before network/container work. Never
print environment secrets, commit credentials or change global system directories
just to reconstruct this previous session's environment.

From the repository root, run sequentially:

```sh
(cd web && npm run check && npm run build)
go vet ./...
go test -race ./...
(cd mcp && npm test)
git diff --check
```

**Finish the web build before starting Go builds/tests.** Go embeds `web/dist`;
concurrent Vite cleaning/rebuilding can produce missing embedded asset failures
that are test-environment races, not application regressions. Keep inputs stable
while checking. The full API race package can take several minutes on cold caches;
do not remove timeouts, skip checks or assume a silent process is hung.

Examples of narrow checks (add the relevant new package/tests for the chosen task):

```sh
go test ./internal/setup ./internal/scan ./internal/store
go test -race ./internal/setup ./internal/scan ./internal/store
```

For the existing synthetic Control browser contract, start the dev server in a
separate terminal and keep it running:

```sh
cd web
npm run dev -- --host 127.0.0.1 --port 4173
```

Then, from the repository root:

```sh
node web/tests/control.e2e.mjs
node web/tests/terminal-workspace.e2e.mjs
node web/tests/terminal-lifecycle.e2e.mjs
```

`REET_UI_URL` can select another dev URL. `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH`
selects a browser (the current fixture otherwise uses `/usr/bin/chromium`).
`REET_UI_SCREENSHOTS` is an optional output directory for synthetic screenshots.
This fixture mocks APIs; pair it with real domain/API/host tests for new effects.

For real terminal browser regression, follow `.github/workflows/terminal-browser.yml`
and the integration fixture, after building web and making tmux/browser available:

```sh
REET_BROWSER_TESTS=1 go test -race -count=1 -timeout=3m ./internal/api -run '^TestTerminalPrivateLifecyclePreservesActualShell$' -v
```

This exercises a real local shell/broker/PTY with test transports. It does not
establish a production relay deployment, cross-OS recovery, or a live fleet rollout.
Physical macOS service-manager and supported cross-OS checks remain separate gates.

### Record after every implementation task

Replace the relevant unchecked task with its completed state only after recording:

```text
Task ID and scope:
Branch / commit / PR:
Behaviour added:
Compatibility / migration behaviour:
Checks actually run and their results:
Platform or live-environment checks not performed:
Known limitations or unresolved decisions:
Exact next unchecked task and first files to read:
Publication / deployment state (verified, not inferred):
```

If a task cannot meet its acceptance gate, leave it incomplete and describe the
failing case and recoverable state. "Code compiles", "API returned 200", a mocked
success response or a green screenshot does not establish successful remote writes,
working agent authentication or complete environment recovery.
