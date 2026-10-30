# Reet central-control implementation handoff

reet.sh is a self-hosted AI fleet control room: machines, the AI agents installed on them, skills,
reviewed sync, browser terminals and, now, an agents home with folder projects. This file is the
current state and the standing rules; start here. Where things are: [README.md](README.md) and
CONTRIBUTING.md; [docs/](docs/) for shipped behaviour (for example
[control centre](docs/control-centre.md), [terminal workspace](docs/terminal-workspace.md),
[projects](docs/projects.md), [folder browse](docs/folder-browse.md),
[remote machine operations](docs/remote-machine-operations.md),
[controller terminal commands](docs/terminal-controller-commands.md),
[OpenCode in the reet TUI](docs/structured-sessions.md) and
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

### Resume here (Home OpenCode reet TUI, 29 October 2026)

**#258 (reet TUI history, continue, images) merged as `2ee12f6`; #256 (OpenCode in the reet TUI) as `7555da2`;
#255 (models UI) as `32866ad`; #252 as `3be0eaf`; #247 as `16da0bb`; #245 as `1371cb5`;
#242 as `961d530`; #231 as `7886511`.**

- **Branch and PR:** `ccr-home-chat` holds the Home launch bar change: OpenCode on an `acp` machine starts in the reet TUI by
  default, with a **TUI** word to switch to the agent TUI. The PR is this one; it is **open** until its
  checks are green and it is merged.
- **Next stage after this PR:** no further Program 2 slice is queued. Other agents need their own probes; the
  deferred F (YOLO) and G (Needs you) work remains. Model work is done.
- **Do first:** check the PR's final head with the REST forms
  (`gh api repos/eyupio/reet.sh/pulls/<n>` and `gh api repos/eyupio/reet.sh/commits/<head sha>/check-runs`;
  `gh pr view`/`gh pr checks` use GraphQL, refused with 403 here). Merge only when every check is green and Codex has
  reviewed it, or the owner gives an explicit exception.
- **Release:** R22 lets B, C, E, copy/paste, models and the reet TUI go out together. The owner's call; no
  release tag was cut or checked (`gh release list --repo eyupio/reet.sh`). F and G come later.
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
| H | `/sessions` (the Sessions page), the session screen, tablet and phone layouts, the last redirects, the phone rail on Sessions | merged, [#231](https://github.com/eyupio/reet.sh/pull/231) as `7886511` (owner's decision; no Codex review possible: quota exhausted). H1 `f0bffe5` Sessions page and Control's address retired (`/control` → `/`, `/terminals` → `/sessions`), fixes `048e431` (session colours, **Saved commands** button: owner ruling), `b85338a`; H2 `f74001f` session screen (tabs, header with the state pill, Chat \| Terminal with Chat disabled, context panel), fixes `f026cfb`, `c02fec6`; H3 `7581ad0` tablet and phone layouts (no rail on narrow Home, the rail on Sessions, slim phone header, `keyboardChrome('session')`), fixes `a984b5e` (44 px tablet targets, cases moved to `sessions-phone.e2e.mjs`), `3f1e89b` (separator grip); `521c406` CI fix (first-terminal checklist off `/sync`); merges of main `9c88a46` (#232, #233), `ae763c8` (#234) and `57d8f53` (#235, #236); H4 `1a5018d` (research copied into `docs/research/`), `c349a2b` (the matrix, phone target fixes, docs, this handoff), `1ff4230` (the research notes name the reference tool by role); final fix wave after the broad review `b53c7dd` (`/sessions/`, viewers at a session address), `adec499` (steady Sessions rows, menu focus, the connection in the phone header, tablet swatches and grip, context panel, tab tooltip, New → `/#launch`), `dd31068` (dead code), `8e6ce3e` (checks that went with Control, steadier phone tests), `6778211` (start-agent expects the Machines title) and the docs commit after them |
| Copy/paste | Terminal paste sanitiser and bracket-aware review, copy screen/last lines/all, Select text sheet, copy-on-select, gated OSC 52 (`clipboard-v1`), image attach/paste/drop with `upload-v1` and private per-session storage | merged, [#242](https://github.com/eyupio/reet.sh/pull/242) as `961d530` (owner's instruction; Codex quota exhausted). C1 `81b0e74`, C2 `dc93c39`, C3 `93e7e37`, C4 `7a75e8a`, C5 `f90e1e3`, U1 `64b2240`, U2 `497eda7`, U3 `cb606bc`, fence `1614b44`, U4 `4e9ac2f`, U5 `b209152`, review fixes `81b9e34`, `aa532b6`; the final CodeQL response/headers work and this follow-up are in #245 |
| Copy/paste follow-up | `Exited` cleanup, running 24 h sweep, two concurrent uploads and third refusal at the connector, lost-reply timeout, Scrollback-mid-upload wording, this handoff | merged, [#245](https://github.com/eyupio/reet.sh/pull/245) as `1371cb5` (owner approval; Codex re-review clean) |
| Models backend | `models-v1`, OpenCode listing, local switch, per-machine cache, read/refresh/list API, favourites, account defaults, project effort, Cline project default | merged, [#247](https://github.com/eyupio/reet.sh/pull/247) as `16da0bb` (owner exception; Codex quota). M0 `c1152ff`, M1 `f4bf81c`, M2 `11afb42`, M3 `9d7f234`, M4 `5490a6c`, docs `3cd34f0` (plan `2a54133`); review fixes `56f5f5b`, `39a61a8`, `a8c9310` |
| Models UI (M5–M7) | `models.ts`, model/favourite clients, picker star/status/Refresh/Use, launch bar defaults-first, Home account defaults, automatic machine read | **[#252](https://github.com/eyupio/reet.sh/pull/252), open**. Plan `d802ff0`, M5 `a7e0d7d`, picker `9202695`, bar `ca0f972`, review fixes `67feb99`, evidence `24df6cc` |
| Models UI (M8–M11) | Start an agent picker with project/account defaults and Cline thinking; Add project/Project settings defaults; Settings › Agents; Machines “Agents” cache state and guarded Refresh; Activity/hover-card model lines; docs and phone evidence | merged, [#255](https://github.com/eyupio/reet.sh/pull/255) as `32866ad` (owner's instruction; Codex reviewed an earlier head, the runner timed out on the final head and a rerun passed). Plan `docs/superpowers/plans/2026-10-29-agent-models-ui-2.md` |
| Program 2, slice 1 | OpenCode in the reet TUI: `acp-v1`, machine-side ACP supervisor with the seven probe conditions, controller relay and SSE, reet TUI, Allow once / Deny, docs | merged, [#256](https://github.com/eyupio/reet.sh/pull/256) as `7555da2` (owner's instruction; Codex findings fixed, all checks green) |
| Program 2, slice 2 | Reopen a closed reet session with `session/load` history, continue it, and attach images as machine paths | merged, [#258](https://github.com/eyupio/reet.sh/pull/258) as `2ee12f6` (Codex mixed-version finding fixed with `acp-history-v1`; all checks green) |
| Home OpenCode reet TUI | OpenCode on an `acp` machine starts in the reet TUI from Home's launch bar; the TUI word switches to the agent TUI | **this PR, open** on `ccr-home-chat`; plan `docs/superpowers/plans/2026-10-29-home-opencode-chat.md` |

Merged into main by the owner while this work was open: #222, #223 (`4791d89`), #224, #225, #228, #229, #230, #232, #233,
#234, #235 and #236 (each merged into the open branch before its next push; #235 and #236 in `57d8f53`), then #237–#244
and the Sync follow-ups while #242 was open (merged into it before each push). #242 then merged as `961d530`; #245 as
`1371cb5`; #246 and later main work landed while the models backend was open.

**Merge rule:** a PR in this work merges only when all checks are green on its final head **and** Codex has reviewed it
cleanly, or the owner gives an explicit exception. So far: #227's exception is recorded on the PR (a comment); #221 and
#226 merged on green CI without a Codex review and without an exception comment; #231 merged on the owner's decision
after green checks; #242 merged on the owner's 28 October instruction to create it and merge when green; #245 merged
on the owner's approval of the follow-up recommendation with a clean Codex re-review; #247 merged as `16da0bb` under the
owner's merge instruction; #252 and #255 merged on the owner's instruction to merge when green; #255's final head passed
after a terminal-browser rerun. CodeQL alert #20 on #242 was dismissed as a false positive (the
machine path is validated plain text, URL-escaped into response headers, and never in the JSON body; JSON plus nosniff).

**Rulings beyond R1–R22** (all recorded in the session notes and kept here so they survive):

- Owner, 27 Oct: the quicker plan above; Pi stays disabled; Cursor stays blocked until its installer host is allowed.
- Owner, 28 Oct: a **Saved commands** button on the Sessions page; copy and paste is its own PR after H, with pasted
  images in a private per-session folder (0700/0600, random names); Program 2 slice 1 is trusted workspaces only (OpenCode,
  Allow once / Deny only) behind the seven probe conditions; the order after H is copy/paste, models, Program 2; pause after
  PR H. Later that day: create the copy/paste PR and merge it when green. Copy/paste is #242 (merged as `961d530`);
  its image path is text only (per-agent attachment behaviour unverified). Then approve the follow-up #245 and start
  the models backend. 29 Oct: the models backend is #247; the web picker follows it as UI-1 (#252, M5–M7) and UI-2
  (M8–M11). The owner instructed: keep going, create PRs and merge when green.
- Controller defaults, 28 Oct (owner may revisit): models D2 a per-machine default is deferred, D3 a favourite can be the
  "default for this agent", D4 the model-name rule stays as it is.

### Limits and known gaps

After the copy/paste PR:

- Pasted images reach an agent as a **path**, not as an attachment reet creates. The UI and docs say so. Per-agent
  image-path behaviour at the pinned CLI versions has not been checked; do not claim one before checking it.
- Uploads are a new `upload-v1` feature and are on by default; `reet terminal uploads off` refuses them and the browser
  shows the sentence. An older controller never negotiates the token and never sends one. The machine stores at most 20
  images / 50 MiB per session, sweeps files older than 24 h (at start and about hourly), and removes the folder when the
  session ends. The controller audits machine, host, tool, MIME and a size bucket only.
- OSC 52 forwarding needs `clipboard-v1` and `reet terminal clipboard on`; `allow-passthrough` stays off. The browser
  still writes only after a gesture or **Copy it**, write-only, at most 1 MiB. Old browsers ignore the sequence.
- Image uploads write to the machine's private terminal directory, outside the project. They are derived state, not in
  the registry. A machine that ends a session mid-upload may make that one upload fail; nothing is retried.

After the models UI:

- Only OpenCode reports a machine-owned model list. Favourites, reet's suggestions and Other… are offered for every
  agent; they are suggestions, not proof that a model exists. A per-machine default stays deferred (D2); the account
  and project defaults are per account/project.
- **Refresh** reads the machine only when it is online and models-capable. Offline shows the last list with its age and
  disables Refresh; a failed refresh keeps the last good list and age. A cache read never contacts a machine.
- No real provider-backed OpenCode list was used; the tests use fake machines and synthetic API answers. A 500-model
  list was not load-tested, and no physical phone, Safari or live agent CLI run was part of this PR.
- The new terminal controls have no mock-up element. They join the existing toolbar, menu and banner styles; at 1440
  the toolbar scrolls as it did before. The Select text sheet, upload chip, copy-on-select checkbox and the uploads-off
  note are the additions listed in [docs/ui-verification.md](docs/ui-verification.md).
- `terminal-clipboard.e2e.mjs` and `terminal-upload.e2e.mjs` are in the `phone-dialogs` CI job. Nothing new was added to
  `terminal-browser`. The local matrix's session screen passes 40 states.
- Still true from PR H: the session screen keeps the sessions panel and workspace toolbar (the mock-up has neither); the
  tabs share the toolbar row and the pane keeps its own header. Old `/sessions` links on a rolled-back server still land
  on `/` or an unknown page. Needs you stays empty until G. The site footer and phone terminal chrome keep their
  documented 40 px targets. Nothing has been checked on a real phone or in Safari.

After the models backend:

- Only OpenCode has a machine-read list. Claude Code, Codex, Gemini CLI and Cline keep reet's static suggestions,
  favourites and Other…; the web picker is not in #247, so the browser does not show a machine list yet. The
  favourites/defaults API is likewise not wired to a page.
- A listed model is a suggestion, never proof it works, and it never blocks a launch. Env-only OpenCode providers
  are missing because the listing environment is an allowlist; say so when the picker lands.
- The cache keeps the last good list through a failed refresh and is deleted with the machine. A first read is
  synchronous only for an online machine that reports `models`; refresh is 6/min per account and machine and is the
  only audited path. A failure is visible as its own state, not as an empty successful list.
- The account defaults are for sessions outside a project; the browser resolves explicit > project > account >
  agent default. A per-machine default remains deferred (D2). A project default model still needs a project agent;
  an existing profile model without an agent stays readable and does not block unrelated edits.
- No real provider or paid model was used in the backend tests; the listing runner uses fake binaries, the API uses
  synthetic machines, and a physical phone/Safari was not checked.

After the models picker/launch bar:

- The launch bar now opens on the resolved defaults, not the last launch; the last models are a Recent group. The
  model slot has a star, filter, Use ‘x’ and a status/Refresh line; tablet/phone keep the model in the Change sheet.
- Only OpenCode has a machine list; other agents use favourites/suggestions/Other. The account defaults and
  favourite manager are under Account → Agents; the Start dialog, Add project/Project settings, Machines → Settings
  → Agents, Activity and the hover card are in #255. A per-machine default remains deferred.
- Home does not load favourites itself; the bar does (a recorded plan deviation). A model read failure never
  blocks Start. The real cached-offline shape is overlaid in the bar, not by the API.
- No physical phone/Safari; a 500-model list under a slow reader is not load-tested. The picker/star additions are
  not in the drawings; `docs/ui-verification.md` lists them.

After the reet TUI slices:

- **Home starts OpenCode in the reet TUI by default.** The launch sentence shows **in reet TUI** for OpenCode on an
  `acp` machine; the **TUI** word switches to **in OpenCode TUI**. The reet TUI runs in the project folder, not a
  worktree, and uses OpenCode's configured model. Home requires **I trust this folder** before Start, because `.opencode`
  plugins run when the reet TUI starts. A multi-project selection must switch to the agent TUI; it starts in one folder.
  A session's actions menu offers **Open a terminal here…**, a plain shell in the same folder as a separate session; it
  needs `shell-dir-v1` on the machine. The right-hand side-by-side terminal panel is not built yet.
- **reet TUI history is in memory, but OpenCode keeps it.** The controller keeps at most 1,000 events / 256 KiB per reet session and
  writes no transcript. A closed reet session can be reopened with Continue in reet TUI with history… as a new row that runs
  `session/load`; the old row stays. A server restart clears the live ring; archiving drops it. A lost machine connection
  kills every reet process group; a reconnect marks the live session lost, and its stored id can still be reopened if
  OpenCode's own store survives. Pending approvals are not restored; replayed tools are shown interrupted.
- **OpenCode 1.19.5 only.** The connector refuses another version before starting a process. The seven conditions in
  [structured-sessions.md](docs/structured-sessions.md) are mandatory; a repository with `.opencode` plugins needs the
  Start dialog's trust confirmation. Always allow is never offered, and a diff or command too large for one control frame
  cannot be allowed.
- **No live model turn** was sent in the checks. The real 1.19.5 binary opened and then loaded a session in a throwaway
  HOME; the protocol tests use a fake ACP child. Modes, MCP servers and other agents are not in this slice. No physical
  phone, Safari or Firefox was checked.
- **Images are paths.** reet TUI attachments use the launch-upload folder and the prompt names the file; no image bytes are
  embedded in ACP. The 5 MiB / 20 files / 50 MiB limits and 24-hour sweep apply. The reet TUI's model is OpenCode's configured
  model; the picker is hidden. The process can still install `@opencode-ai/plugin` from npm in the background.

Before this PR:

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
- `terminal-phone.e2e.mjs`'s 20 × 8 floor case used to read 13 rows in about one run in five: the workspace height was
  measured while the shell held the top bar aside and not measured again when it came back. Fixed and hardened in
  `5abe001` (the workspace measures again whenever the shell toggles its chrome; the test waits for the layout to
  settle instead of a fixed 450 ms); 10 runs in a row and one under CPU load pass, with the same figures.

Still true from earlier PRs:

- YOLO is shown disabled and refused (409 `yolo_refused`) until F. The first line of a prompt becomes the stored session
  title. The Start an agent dialog now offers Cline on an agents-v2 machine; Pi and Cursor stay disabled.
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

**1. Program 2, slice 2: reet TUI history, continue and images** (this PR). Input: the slice 1 probe, the real
`session/load` result (configOptions, mode `build`, replay before the result), and the Home launch-upload path.
Slice 1 is merged in #256; its conditions and limits remain binding.
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
- **Merges:** the owner merges other PRs while one is open (#228–#241 during this work). Always `git fetch origin main`
  and merge it before pushing.
- **Git and processes:** never `git checkout -p`; commit with pathspecs; never `pkill -f` (it can match the session's own
  processes); never a shared tmux `kill-server`. The stop hook nags while subagents are still working; that is expected.
- **Commit trailers:** new commits follow the session's attribution reminder (`Co-Authored-By` and `Claude-Session`);
  every commit in this PR carries Claude Sonnet 5.5.
- **Upload wire**: the machine reads exactly the declared size and then returns one `UploadResult` frame; the controller
  waits for that frame before closing, because a websocket peer may not be able to write after a close. The host reserves
  quota under `uploadMu` and releases it while streaming; cleanup never takes `uploadMu`, or an ended session stalls the
  connector. The 24 h sweep runs at start and hourly while `Run` is alive.
- **New UI fixtures** go in `phone-dialogs`; `terminal-clipboard.e2e.mjs` and `terminal-upload.e2e.mjs` were added there.
  Run them plus `phone-dialogs`, `phone-zoom`, `sessions`, `sessions-phone` and the `terminal-browser` files in workflow
  order before the final push. The local matrix session screen is `REET_MATRIX_ONLY=session`.
- **Models listing**: only connector-owned argv, a fresh temp cwd, the launch allowlist minus `TERM`/`SHELL`, the
  fetch/autoupdate opt-outs, a 10 s process-group kill and 64 KiB stdout. Never run `--refresh` (it ignores the
  fetch opt-out) and never forward stderr. The controller caches the last good list; a failed refresh must not
  erase it. The first read is synchronous only for a machine that reports the capability.
- **Models UI**: the launch/e2e fixture must answer `/api/v2/profile/model-favourites` and
  `/api/v2/machines/<id>/models` (the bar reads both without blocking Start); `phone-zoom`/`phone-dialogs` use
  `helpers/longtext.mjs` and need the same routes. The picker's filter is pinned and the sheet body scrolls; keep it
  that way for 500 models. Star focus must be restored after the row moves groups.
- **Mixed versions:** `acpSessionId`, `resumeAcp` and the `acpHistory` capability are sent only after
  `acp-history-v1`. A slice-1 controller negotiated `acp-v1` but its strict decoder rejects those fields; never send them
  without the separate feature. `Continue in reet TUI` is offered only when the machine reports `acpHistory`.
- **reet TUI history:** a closed reet session is reopened as a **new** row with `resumeAcp`; the machine runs `session/load` and
  replays before its result. Store the `acpSessionId`, never a transcript. `CanResumeACPSessionFor` restricts the id to
  this account's closed structured sessions on that machine. Replayed pending tools are marked interrupted; pending
  approvals are not restored. The old row stays in history.
- **reet TUI:** the machine-side `internal/opencodeacp` starts exactly `opencode-ai@1.19.5` with
  `OPENCODE_CONFIG_CONTENT`, `OPENCODE_DISABLE_PROJECT_CONFIG=1`, a random `OPENCODE_SERVER_PASSWORD`, `task: deny` and
  the environment allowlist. It answers `fs/write_text_file` with success without writing (OpenCode already wrote the
  file), checks mode `build`, offers `once`/`reject` only, closes its own cards on cancel/timeout, and kills the process
  group on end. Do not forward raw ACP JSON to the browser; map it to `ACPEvent` and keep it inside the control frame.
- **reet TUI transport:** events are a bounded ring in `internal/terminalbroker/chat.go` and SSE at
  `/api/v2/terminal/sessions/{id}/chat`; prompts, cancel and approvals are POSTs. A browser reconnect resets and replays
  the ring. A structured session is an ordinary row with `structured=1`; it has no terminal, so do not attach to it.
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
| OpenCode in the reet TUI | `internal/opencodeacp` (process, config, env, permissions), `internal/terminalagent/chat.go` (dispatch), `internal/terminalbroker/chat.go` (ring/commands), `internal/api/terminal_chat.go` (SSE and POSTs), `web/src/lib/chat.ts` and `terminal/ChatPane.svelte` |
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
