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
against in [docs/ui-verification.md](docs/ui-verification.md); and older narrative in
[handoff-history.md](handoff-history.md).

**Docs tidy-up (27 October 2026).** Finished plans, specs, stage documents and audits were removed
on 27 October 2026; they live in git history before commit `bd9e56c` (for example
`docs/superpowers/plans|specs/2026-10-2[145]*`, `docs/agent-setup-plan.md`, `remote-machine-*` and
`docs/remote-machine-stage*.md`). Older per-feature delivery notes (single sign-on PR logs, terminal
sudo repair, roadmap checkpoints, terminal polish, terminal-first workspace requirements, the
implemented baseline, S/A/W/L task lists, start-agent-session protocol notes, Projects PR 1 and
delivered, phone terminal/zoom/dialog notes, the ready-to-use continuation brief) are in
[handoff-history.md](handoff-history.md), under "Moved from handoff.md on 27 October 2026".

## Agents home redesign (27 October 2026)

**In progress, updated with every pull request.**
Home, the shell and the machine-folder projects are being rebuilt to the mock-up in
[docs/design/agents-home/](docs/design/agents-home/) (reference renders at 1440, 834 and 390 px; each PR
is checked against them and says so in `docs/ui-verification.md`). The plan, with its rulings and global
constraints, is [docs/superpowers/plans/2026-10-26-agents-home.md](docs/superpowers/plans/2026-10-26-agents-home.md).
Current work is on [PR #227](https://github.com/eyupio/reet.sh/pull/227) (branch `ccr-7a7f5a9b-to5imh`,
based on main after #226 merged). **Quicker plan, agreed 27 October 2026:** the
fidelity polish (P) is folded into the PRs that touch the same screens, YOLO (F) and attention (G) are left out of
the first pass, and the structured-chat spike runs in parallel with the Home work. Order:

| PR | What | State |
| --- | --- | --- |
| D | App shell (top and bottom tabs, bell, jump search, More sheet) and Home | merged, [#218](https://github.com/eyupio/reet.sh/pull/218) |
| A | Machine folder browse and branches (`agents`, `browse-v1`, `branches-v1`), start-agent navigates to the session, dockable and resizable sessions panel | merged, [#219](https://github.com/eyupio/reet.sh/pull/219); contract in [docs/folder-browse.md](docs/folder-browse.md) |
| B | Folder projects: store and API (`/api/v2/folder-projects`, reorder, `import-saved`, `?legacy=`), saved Projects moved over (old create/edit/delete answer 410 `projects_moved`), the Add project dialog with the folder browser, and the rail on Home (card menu, Worktrees, the one-time migration note, the "waiting folders" prompt; a 1100–1239 px strip with a managed panel); `/projects` and `/projects/<id>` redirect to Home, old runs are read-only in Activity (*Runs from Projects*); read [docs/projects.md](docs/projects.md) | merged, [#221](https://github.com/eyupio/reet.sh/pull/221) as `cb89a26`; B1–B6 complete; Codex did not review its final head. No release tag until PR E merges (R22) |
| P | Home fidelity polish: tokens, spacing, top and tab bars go into C; fleet strip, In flight, Needs you and the dock go into E | folded into C and E, no PR of its own |
| C | Agent catalogue and launch fields (`agents-v2`, named worktree base, effort where confirmed, tool on the session listing, `osVersion`) and the folded chrome polish | merged, [#226](https://github.com/eyupio/reet.sh/pull/226) as `540d247` on 27 Oct 2026 17:17 UTC; C1, C2, C3 and C4 and the scoped polish shipped; Cline enabled with confirmed safe arguments. The owner approved keeping Pi disabled; Cursor remains blocked on environment access (limits below). No release tag yet |
| E | Launch bar, start in several projects (PR B already retired the old Projects page; the old Start-all routes and tables go at the later release step). Notes for E: take folder-project versions from the reorder reply (every moved project's version changes); check `dir` (or the worktree's source) for a session's `projectId`, which PR B checks for account and machine only; apply the project's defaults and starter prompt (Use the starter prompt) | **in progress in [#227](https://github.com/eyupio/reet.sh/pull/227)** (draft marked ready by the owner; a Codex review is not possible, the quota is exhausted). Done so far: fixes from a post-merge review of #226 (Go: unsupported tools and options refused before a session is reserved, `pi` and `cursor` out of `ValidToolV2`, the private hello validated against the agreed features, the distro name from os-release on Linux held to the shared plain-text rule, `Capabilities.Yolo` rejected; web: phone tab bar contrast token `--dock-bg`, one source for tool names, fixture hardening, dock shadow and radius; catalogue tests check versions, values and approval-bypass flag pairs; the `image.yml` test job has a 30-minute timeout) and E1, the launch bar logic (`web/src/lib/launchBar.ts`). Still to do: E2 the launch bar component, E3 start in several projects, E4 CI and docs |
| F | YOLO (off by default, per session, audited, machines can refuse) | **deferred past the first pass**; the design decisions stand |
| G | Attention and Needs you | **deferred past the first pass**; Needs you stays an honest empty state until then |
| H | Session screen, tablet and phone | not started |
| Program 2 | Structured chat over the agent protocol, with command-approval cards | research spike done (27 Oct 2026): go with conditions, the owner chose **OpenCode first** (27 Oct 2026) instead of Claude: it speaks the protocol natively (`opencode acp`, MIT, single binary, no adapter to install), so the first slice needs no install step; its resume after a lost runner and its permission behaviour are less documented than Claude's and must be probed before the plan is fixed; first slice is OpenCode with approval cards, the spike's 7–9 engineer-weeks was sized for Claude and needs re-checking; report in `.superpowers/sdd/2026-10-26-agents-home/program2-spike.md` (git-ignored, to be moved into `docs/design/` with the plan); hands-on probe not yet run, it needs the owner's go-ahead; written plan follows |

Merged into main by the owner while #227 was open:

- [#229](https://github.com/eyupio/reet.sh/pull/229) "Add live sync feedback and one-click installs from skills", as `4850c5d`.
- [#228](https://github.com/eyupio/reet.sh/pull/228) "Start sessions from project lists, and paint the chosen theme before first paint", as `4c16e69`.
- #227 merged main (`4c16e69`) in: on Home, a rail card and its menu's New session here set the launch bar's project; the Start an agent dialog stays for Machines and the Sessions panel as #228 built it.

**Merge rule:** a PR in this work merges only when all checks are green on its final head **and** Codex
has reviewed it cleanly, or the owner gives an explicit exception. #221 and #226 merged without a
Codex review.

Until Program 2, reet starts agents in terminal sessions and wraps them with projects, the launch bar,
YOLO, attention and the session screen; it is not yet a structured chat front end.
Things still open: nothing has been checked on a real phone or in Safari (browser emulation only); worktree
git calls share the lazy-fetch and fsmonitor hazard the folder browser already guards against; the
`terminal-browser` CI job takes about 25 of its 30 minutes, so add no new fixtures there (PR B moved
`projects.e2e.mjs` out of it into `phone-zoom`); since PR B the worktree figure on Home is "worktrees in use"
(live sessions only: no record says which worktrees were removed), choosing a rail card opens Start an agent
until the launch bar (E) applies the project's defaults and starter prompt, and the old Projects tables and
their start/retry routes stay one release before they are dropped (ruling: no page calls them; the web client for them
and the run Retry are already removed); a downgrade then re-upgrade leaves Projects saved while downgraded as "waiting
folders" in the rail (not migrated automatically); a session's `projectId` is checked for account and machine, not
folder (see the E row); the import's answer still says "saved repositories" (a Go sentence) where the rail says
"folders"; the MCP test
`mcp/test/oauth.test.mjs` needs the fixture built in CI (the warm-up in `image.yml`).

**CI runners.** CI runs on the Zoomies runners: tmux and Chromium are installed inline per job, and the MCP fixture build is a cache warm-up (`go build -o /dev/null`), not a prebuilt artifact. `terminal-browser` has a 30 minute limit and takes about 25 minutes. The terminal continuation test now times painted rows and asserts one attach and one socket. Codex reviewed `5cd134a` and its P2 was fixed in `0aca903`; Codex did not review the final head of #221 after reaching its usage limit. The owner authorised merging on green CI without that final review. GitHub records final head `68624bc` (the terminal-browser timeout follow-up); all five workflow runs on that head succeeded.

C continuation (27 October): pinned help-only installs were authorised for the three new CLIs; no agent task or ACP probe was run. Cline 3.0.74 requires explicit `--auto-approve false --tui` and supports `--thinking low|medium|high`; the connector supplies those safety arguments and still refuses YOLO. Pi 0.73.1 has no built-in permission prompts, expands `@` operands as files and does not honour the usual `--` prompt boundary, so the owner approved keeping it disabled in C and deferring a safe integration design. Cursor remains disabled because the official installer at `cursor.com` was denied by this workspace network policy; the owner approved adding the host, but the available environment tools are read-only and the setting has not changed. C was published on `codex/agents-home-c-20261027` and merged to main as #226. Existing four-agent launch arguments are preserved, and unsupported effort choices fail explicitly. The chrome and metadata check passed both themes at 1440/1239/1100/834/651/390/320; its recorded scope and remaining Home differences are in `docs/ui-verification.md`. The full Home browser suite also passed after aligning OS labels and refreshing the paused-machine fixture timestamp. The tmux 3.2 CI job runs only a standalone test binary: captured CLI help is embedded in that test binary so its checks do not depend on a repository checkout. The full local Go race run found a stale exact feature-header assertion (now includes `agents-v2`); all other API cases passed and the corrected case passed separately. The final full host/protocol race suites passed under a child-reaping supervisor, which this workspace needs because PID 1 leaves killed children as zombies. Other packages passed in the broad run. #226 was merged by the owner as `540d247` (27 Oct 2026 17:17 UTC). CI was green on the final head `bf3af10`. Codex review was requested but could not run because the account code-review quota was exhausted; #226 has no Codex review and no review exception is recorded.

Limits after #226: Cline cannot be started from the web yet: the Start dialog still uses the old four-agent list; E2 switches the launch bar to the catalogue. Pi and Cursor stay disabled.

**Rule for this work: every PR updates this block (state column and any new limits) before it merges.**

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
| App/routes and Control | `web/src/App.svelte`, `web/src/lib/ControlCenter.svelte` |
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
