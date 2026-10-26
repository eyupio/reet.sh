# Agents Home Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Control-first navigation with top tabs and a mission-control Home (fleet strip, In flight, Needs you, project rail, launch sentence), make a project one folder on one machine picked by browsing it, add the agent catalogue, YOLO and attention, and give each session a screen with its context.

**Architecture:** Four new negotiated terminal-protocol features (`browse-v1`, `branches-v1`, `agents-v2`, `attention-v1`) carry closed, validated fields the connector re-checks; the controller adds a folder-project registry, YOLO gates and attention derivation, and reuses the existing launch, worktree and run orchestration paths. The web gets theme tokens, a new app shell, pure logic modules (`folderBrowser.ts`, `home.ts`, `launchBar.ts`, `catalogue.ts`, `sessions.ts`) and thin Svelte components over them, on the shared dialog shell.

**Tech Stack:** Go 1.27.1, git, tmux 3.2+, SQLite store, Svelte 5 runes + TypeScript in `web/` (build before Go tests), Node 22, Playwright fixtures, axe-core.

**Spec:** `docs/superpowers/specs/2026-10-26-agents-home-design.md` (read it; the plan argues from it, and its rulings R1–R22 are binding). Shipped building blocks: `docs/terminal-launch.md`, `docs/terminal-worktrees.md`, `docs/start-agent-session.md`, `docs/projects.md`, `docs/terminal-workspace.md`.

## Global Constraints

- Never name the products behind the reference designs or other products in repo files or UI; say "the reference designs". Example machines in tests and docs: `studio`, `server`, `desk`; projects `reet`, `site`, `notes`.
- "Started" means the connector started the program in tmux; "verified" and "healthy" never describe a machine, session or worktree; no step count or progress bar for a terminal session (R10).
- Voice (`docs/voice-and-copy.md`): Yorkshire only in headings, ledes and success lines; buttons, states, refusals and the YOLO dialog are plain. Copy is taken verbatim from the spec.
- `terminalproto.Create` stays comparable; new fields are flat with `omitempty`: `Base string`, `Effort string` (`low|medium|high`), `Yolo bool`, all under `agents-v2` (R14). `Base` requires `Worktree`. No protocol version bump. A peer without a token is never sent its fields or messages.
- Feature tokens and capabilities: `browse-v1` → `browse` (requires `terminalControl`); `branches-v1` → `branches` (requires `worktree`); `agents-v2` → extended `tools`; `yolo` capability (requires `launch`, absent when the machine refuses); `attention-v1` → `SessionSnapshot.{activityAt, bellAt, exitStatus}`.
- Browse: stat-only, directories only, at most 5,000 entries read, 200 returned, names ≤255 bytes valid UTF-8 without control characters, reply ≤12 KiB, 3 s machine deadline, 6 s controller deadline, one at a time per connection, 20 per 10 s per account and machine. Branches: the three exact git argument vectors in the spec, 5 s timeout, ≤200 names, 10 per 10 s. Neither is stored by the controller.
- Git rules from `worktree-v1` unchanged: argument vectors only, minimal environment, every `GIT_*` removed, hooks disabled on create.
- Projects: name 1–80 characters unique per account ignoring case; at most 100 per account (`project_limit`); machine+folder once (`project_exists`); `If-Match` (`project_changed`); starter prompt follows the opening-prompt rules (≤2,000 bytes). Project writes 30 per minute per account. Session limits 8 per machine, 32 per account.
- YOLO: account setting off by default, owner/admin only; machine accepts unless `reet terminal yolo off`; Approvals resets to *Ask me* after every launch and is never stored.
- Prompts and starter prompts never in audit, logs, errors, browser storage or URLs; only ids in URLs; nothing of projects or sessions in browser storage except per-browser view choices (group by, rail collapsed).
- Theme tokens only in components (no literal colours); fonts self-hosted (R15); every input 16 px on phones; touch targets ≥44 px (48 px phone primary actions).
- Breakpoints: phone `PHONE_SCREEN` (`web/src/lib/terminal/sizing.ts`), tablet 651–1099 px, desktop ≥1100 px.
- Platform: Linux and macOS; tests resolve `/tmp` symlinks and never assume `sh`. Build the frontend before Go checks. Commit trailers as the session requires.
- Do not edit the shared dialog shell (`web/src/lib/dialogShell.ts`, `web/src/dialog-shell.css`); consume its `shell` action. PR D starts after it merges.

## Review Focus

- **A machine goes offline mid-browse.** The request in flight fails at once with the readiness sentence and **Try again**; breadcrumb, list and typed path stay; the person can still add the project by path. Test: `add-project.e2e.mjs` "offline mid-browse keeps the list" (Task D6) and `TestBrowseMachineDisconnectsMidRequest` (Task A4).
- **A path with unusual characters** (spaces, quotes, `#`, a leading `-`, newline or invalid UTF-8 in a name, a 255-byte name, a symlink loop). Displayable names are listed and round-trip into `Path`; undisplayable ones are omitted and counted; a loop never hangs. Tests: `TestBrowseUnusualNames`, `TestBrowseSymlinkLoop` (Task A2), `folderBrowser.test.mjs` "breadcrumb of a path with spaces and #" (Task D5), and a launch of a project whose folder name starts with `-` (Task E2).
- **Two people (or two tabs) edit one project.** The second save gets 409 `project_changed` with "Someone changed this project. Reload it, then try again." and keeps their input; a delete racing an edit never resurrects the project. Tests: `TestFolderProjectStaleIfMatch`, `TestFolderProjectDeleteThenStalePatch` (Task B3), `add-project.e2e.mjs` "stale edit keeps input" (Task D6).
- **A session started while the controller restarts.** The launch bar keeps the request id and the same parameters; **Try again** sends exactly the same request and never a second session or worktree; a several-project run resumes only pending members. Tests: `launch.e2e.mjs` "lost reply retries the same request" (Task E4), `TestSelectionRunResumesOnlyPendingAfterRestart` (Task E3).
- **A phone with the keyboard up.** Focus in the launch prompt or the session screen hides the top bar and tab bar, keeps Start (48 × 48 px) and the focused field above the keyboard, and the page never zooms or scrolls sideways. Tests: `phone-layout.e2e.mjs` "launch prompt with keyboard open" and "session with keyboard open" at 320 and 390 px (Task H3).

## File Structure

- `internal/terminalproto/`: create `browse.go`, `branches.go`, `agents.go`, `attention.go` (+ tests); modify `protocol.go` (Create fields, Capabilities, SessionSnapshot).
- `internal/terminalhost/`: create `browse.go`, `branches.go`, `catalogue.go`, `attention.go` (+ tests, `testdata/help/*.txt`); modify `launch.go` (presets move to `catalogue.go`), `worktree.go` (base), `manager_tmux.go` (monitor-bell, activity read).
- `internal/terminalagent/agent.go`, `internal/scan/agent.go`, `internal/scan/agent_terminal.go` (local `browse`/`yolo` switches).
- `internal/terminalbroker/`: create `browse.go` (+ test); modify `broker.go`.
- `internal/api/`: create `terminal_browse.go`, `folder_projects.go`, `yolo_settings.go`, `attention.go` (+ tests); modify `terminal.go`, `projects.go`, `project_runs.go`, `api.go`.
- `internal/store/`: create `store_folder_projects.go`, `store_attention.go` (+ tests); modify `store_projects.go`, `store_terminal.go`, `store_backups.go` (profile defaults).
- `internal/projectrun/run.go`: selection runs.
- `web/src/app.css`, `web/public/fonts/*` (+ `OFL.txt`), `web/src/App.svelte`.
- `web/src/lib/`: create `theme.ts`, `folderBrowser.ts`, `home.ts`, `launchBar.ts`, `catalogue.ts`, `sessions.ts` (+ `.test.mjs` each); create `shell/AppShell.svelte`, `shell/TopTabs.svelte`, `shell/Bell.svelte`, `shell/BottomTabs.svelte`, `home/Home.svelte`, `home/FleetStrip.svelte`, `home/InFlight.svelte`, `home/NeedsYou.svelte`, `home/ProjectRail.svelte`, `home/AddProjectDialog.svelte`, `home/FolderBrowser.svelte`, `launch/LaunchBar.svelte`, `launch/SlotPicker.svelte`, `launch/YoloDialog.svelte`, `sessions/SessionsPage.svelte`, `sessions/SessionScreen.svelte`, `sessions/SessionTabs.svelte`, `sessions/ContextPanel.svelte`, `ActivityPage.svelte`; modify `terminal/api.ts`, `api.ts`, `events.ts`; delete `ProjectsPage.svelte`, `ProjectPage.svelte`, `ProjectDialog.svelte`, `StartRunDialog.svelte`, `DeleteProjectDialog.svelte`, `ControlCenter.svelte` usage on `/`.
- `web/tests/`: create `home.e2e.mjs`, `add-project.e2e.mjs`, `launch.e2e.mjs`, `yolo.e2e.mjs`, `sessions.e2e.mjs`; modify `phone-zoom.e2e.mjs`, `phone-layout.e2e.mjs`, `navigation.e2e.mjs`, `projects.e2e.mjs` (becomes redirect checks); `.github/workflows/terminal-browser.yml`.
- Docs: create `docs/folder-browse.md`; modify `docs/projects.md`, `docs/terminal-launch.md`, `docs/start-agent-session.md`, `docs/terminal-worktrees.md`, `docs/terminal-workspace.md`, `docs/control-centre.md`, `docs/roadmap.md`, `docs/ui-verification.md`, `handoff.md`, the start-agent spec's "No permission-bypass flags" line.

Every PR closes with the same **validation, review and merge** steps: (1) `cd web && npm run check && npm run build && npm test`; `go vet ./...`; `go test -race -count=1 ./...`; `cd mcp && npm test`; (2) broad review of the whole PR diff on the most capable model against the spec and this plan, fixing findings; (3) open the PR, CI green on Linux and macOS on the final head; (4) Codex review clean (fix and re-run until clean); (5) merge; append the PR number and any ruling to `handoff.md`.

---

## PR A — browse and branches on the wire

### Task A1: Protocol messages

**Files:** Create `internal/terminalproto/browse.go`, `browse_test.go`, `branches.go`, `branches_test.go`; modify `protocol.go`.

**Interfaces:**
- Consumes: `ValidDir`, `Capabilities.Validate`, the message registry pattern of `worktree.go`.
- Produces: `const BrowseFeature = "browse-v1"`, `const BranchesFeature = "branches-v1"`; `SupportsBrowse(features string) bool`, `SupportsBranches(features string) bool`; `Capabilities.Browse bool` (`browse,omitempty`, invalid without `TerminalControl`), `Capabilities.Branches bool` (`branches,omitempty`, invalid without `Worktree`); `Browse{Path, After, Filter string; Hidden bool}` (kind `browse`); `BrowseEntry{Name string; Dir, RepoRoot, Hidden bool}`; `BrowseResult{Path, Home, Parent, Repo, RepoTop string; Entries []BrowseEntry; More, Partial bool; Scanned, Omitted int; Reason BrowseReason}` (kind `browse_result`); `BrowseReason` ∈ `""`, `not_found`, `not_dir`, `outside_home`, `denied`, `unreadable`, `conflict`; `Repo` ∈ `root|inside|none`; `Branches{Dir, Filter string}` (kind `branches`); `BranchesResult{Current, Default string; Branches []string; More bool; Reason BranchesReason}`; `BranchesReason` ∈ `""`, `not_repo_root`, `outside_home`, `git_failed`, `conflict`; `const MaxBrowseEntries = 200, MaxBrowseReplyBytes = 12 << 10, MaxBranchNames = 200`.

- [ ] Step 1: failing tests: `TestBrowseRoundTripIsStrict` (unknown field rejected), `TestBrowseRequestLimits` (`After` 256 bytes, `Filter` 65 bytes, relative `Path`, control character → invalid; empty `Path` valid), `TestBrowseResultLimits` (201 entries, a name of 256 bytes, a name with `\n`, `Repo:"inside"` without `RepoTop` → invalid; encoded 200 maximal entries ≤ `MaxBrowseReplyBytes`), `TestBranchesRoundTripAndLimits` (201 names, an invalid ref name `a..b` → invalid), `TestCapabilitiesBrowseRequiresTerminalControl`, `TestCapabilitiesBranchesRequiresWorktree`, `TestBrowseIsComparable`.
- [ ] Step 2: `go test ./internal/terminalproto/ -run 'Browse|Branches' -v` — expect FAIL.
- [ ] Step 3: implement following `worktree.go`; ref-name validity reuses the worktree branch rule.
- [ ] Step 4: `go test -race ./internal/terminalproto/ -count=1` — PASS. Commit "Add folder browse and branch listing to the terminal protocol".

### Task A2: Browse on the machine

**Files:** Create `internal/terminalhost/browse.go`, `browse_test.go`.

**Interfaces:**
- Consumes: Task A1; `ResolveDir(home, dir string) (string, error)`.
- Produces: `type FS interface { ReadDir(string) ([]os.DirEntry, error); Lstat(string) (os.FileInfo, error); Stat(string) (os.FileInfo, error) }`; `func Browse(ctx context.Context, home string, req terminalproto.Browse, fsys FS, now func() time.Time) terminalproto.BrowseResult` (3 s deadline taken from `ctx`).

- [ ] Step 1: failing tests on real temp trees (`t.TempDir()` resolved with `filepath.EvalSymlinks`): `TestBrowseListsOnlyFolders` (a file never appears; `Omitted` counts it), `TestBrowseNeverOpensFiles` (an unreadable `0000` file leaves the reply identical; a recording `FS` sees no open), `TestBrowseContainment` (`..`, a symlink to `/`, a symlink to a sibling outside home → `outside_home` or omitted; a symlink to a folder inside home listed under its own name), `TestBrowseRepoDetection` (`.git` dir with `HEAD` → `RepoRoot`; `.git` file → false; `.git` dir without `HEAD` → false; `Repo:"inside"` with `RepoTop` for a nested folder; never walks above home), `TestBrowseHiddenAndFilter` (`.config` absent unless `Hidden`; filter `REE` matches `reet`), `TestBrowsePaging` (450 folders → 200, 200, 50 with `More` and `After`), `TestBrowseEntryCap` (6,000 entries → `Scanned` 5,000, `More` true), `TestBrowseDeadline` (slowed `FS` fake → partial page, `Partial` true), `TestBrowseUnusualNames` (spaces, quotes, `#`, leading `-`, 255-byte name listed; newline and invalid UTF-8 names omitted and counted), `TestBrowseSymlinkLoop` (a→b→a returns within the deadline, entry omitted), `TestBrowseReplyFitsFrame`.
- [ ] Step 2: `go test ./internal/terminalhost/ -run Browse -v` — FAIL.
- [ ] Step 3: implement; sort by `strings.ToLower` then bytes; stop reading at 5,000 entries.
- [ ] Step 4: `go test -race ./internal/terminalhost/ -run Browse -count=1` — PASS. Commit "Browse a machine's folders without opening a file".

### Task A3: Branches, connector wiring and the local switch

**Files:** Create `internal/terminalhost/branches.go`, `branches_test.go`; modify `internal/terminalagent/agent.go`, `internal/scan/agent.go`, `internal/scan/agent_terminal.go`, their tests.

**Interfaces:**
- Consumes: Tasks A1–A2; `GitRunner`, the worktree repository-root check.
- Produces: `func ListBranches(ctx context.Context, home string, req terminalproto.Branches, git GitRunner) terminalproto.BranchesResult`; the connector answers `browse`/`branches` with one in flight each (second → `conflict`); `agentConfig.BrowseOff bool` (`browseOff,omitempty`); `reet terminal browse on|off|status`; capabilities `Browse` (negotiated, terminal control on, not `BrowseOff`) and `Branches` (negotiated and `Worktree`).

- [ ] Step 1: failing tests: `TestListBranchesArgv` (recording runner sees exactly `-C <dir> for-each-ref --count=201 --sort=-committerdate --format=%(refname) refs/heads refs/remotes`, then `symbolic-ref --quiet --short HEAD`, then `symbolic-ref --quiet --short refs/remotes/origin/HEAD`, with the worktree environment), `TestListBranchesRealGit` (newest first, detached HEAD → `Current` empty, `Default` from `origin/HEAD`, 201 refs → 200 and `More`), `TestListBranchesRefusesNonRoot`, `TestAgentBrowseOneAtATime`, `TestAgentBrowseNotNegotiatedIsNeverAnswered`, `TestTerminalBrowseOffRemovesCapability` (`reet terminal browse off` → capability absent, request answered `denied`).
- [ ] Step 2: run — FAIL. Step 3: implement. Step 4: `go test -race ./internal/terminalhost/ ./internal/terminalagent/ ./internal/scan/ -count=1` — PASS. Commit "List branches and answer browse requests on the machine".

### Task A4: Broker, API, audit, two-host test, docs

**Files:** Create `internal/terminalbroker/browse.go`, `browse_test.go`, `internal/api/terminal_browse.go`, `terminal_browse_test.go`; modify `broker.go`, `api.go`, `internal/api/terminal_worktree_integration_test.go`; create `docs/folder-browse.md`.

**Interfaces:**
- Consumes: Tasks A1–A3.
- Produces: `func (b *Broker) Browse(ctx context.Context, uid int64, agent string, req terminalproto.Browse) (terminalproto.BrowseResult, error)`, `func (b *Broker) Branches(ctx context.Context, uid int64, agent string, req terminalproto.Branches) (terminalproto.BranchesResult, error)`; `GET /api/v2/machines/{agentId}/browse?path=&after=&filter=&hidden=` and `GET /api/v2/machines/{agentId}/branches?dir=&filter=` (JSON lower-camel of the result types); errors 409 `browse_unsupported` "Update reet on that machine to browse its folders.", 409 `branches_unsupported`, 429 `browse_rate_limited` / `branches_rate_limited` "Slow down a moment, then try again.", 404/422 reasons mapped to "That folder isn't on studio.", "That folder is outside the home folder reet was set up with.", "reet can't read that folder."; audit `folder-browse` `{machine, path, pages}` written once per `?session=` browser-session id (a client nonce) on its first page and updated with the page count.

- [ ] Step 1: failing tests: `TestBrowseRouteAccess` (viewer, API token, MCP token, single-token mode, other origin → refused), `TestBrowseOnOldPeerAsksForUpdate`, `TestBrowseRateLimit` (21st in 10 s → 429), `TestBrowseOfflineMachineAnswersAtOnce` (readiness sentence, <100 ms), `TestBrowseMachineDisconnectsMidRequest` (pending request fails with the readiness sentence, no hang past 6 s), `TestBrowseStoresNothing` (no store write besides the audit row; the audit row has no entry names), `TestBrowseAuditOncePerBrowser`, `TestBranchesRouteAndRateLimit`; extend the real two-host worktree test with `TestTerminalPrivateBrowseAndBranches` (browse home, enter a repo, list its branches over a real connection).
- [ ] Step 2: run — FAIL. Step 3: implement; write `docs/folder-browse.md` (negotiation, rules, limits, sentences, what is not established).
- [ ] Step 4: `go test -race -count=1 ./internal/terminalbroker/ ./internal/api/ -run 'Browse|Branches'` — PASS. Commit "Offer folder browse and branch listing through the API".

**PR A validation, review and merge:** the common steps above. PR title "Browse a machine's folders and list its branches".

## PR B — project registry, migration, retiring /projects

### Task B1: Folder project store

**Files:** Create `internal/store/store_folder_projects.go`, `store_folder_projects_test.go`; modify `store.go` (migrate call).

**Interfaces:**
- Consumes: `terminalproto.ValidDir`, `ValidTool`, `ValidModel`, `ValidPrompt`, `nameKey`.
- Produces: `FolderProject{ID, Host, Dir, Name string; IsRepo bool; BaseBranch, WorktreeDefault, DefaultTool, DefaultModel, StarterPrompt string; Position int; LegacyProjectID string; CreatedAt, UpdatedAt, Version int64}`; `CreateFolderProjectFor(uid int64, p FolderProject) (FolderProject, error)`, `UpdateFolderProjectFor(uid int64, p FolderProject, expect int64) (FolderProject, error)`, `DeleteFolderProjectFor(uid int64, id string, expect int64) error`, `FolderProjectFor(uid int64, id string) (FolderProject, error)`, `ListFolderProjectsFor(uid int64) ([]FolderProject, error)`, `ReorderFolderProjectsFor(uid int64, ids []string, versions []int64) error`; errors `ErrFolderProjectLimit`, `ErrFolderProjectExists`, `ErrFolderProjectChanged`, `ErrFolderProjectName`; `const MaxFolderProjects = 100`.

- [ ] Step 1: failing tests: `TestFolderProjectLimit` (101st → `ErrFolderProjectLimit`), `TestFolderProjectUniqueMachineFolder` (same host and cleaned dir `/a/b/` vs `/a/b` → exists), `TestFolderProjectNameDefaultsAndClash` (empty name → folder name; clash → `reet (2)`; explicit clash case-insensitive → `ErrFolderProjectName`), `TestFolderProjectValidation` (relative dir, 81-character name, starter prompt starting with `-` or over 2,000 bytes, bad ref name, `worktreeDefault:"maybe"` → refused), `TestFolderProjectVersioning` (stale `expect` → `ErrFolderProjectChanged`; `expect` 0 → last write wins), `TestFolderProjectScopedByAccount`, `TestFolderProjectReorderChecksVersions`.
- [ ] Step 2–4: FAIL, implement (`migrateFolderProjects` with the repo's migration pattern), `go test -race ./internal/store/ -run FolderProject -count=1` PASS. Commit "Store folder projects".

### Task B2: Profile defaults and session project id

**Files:** Modify `internal/store/store_backups.go` (profile), `store_terminal.go`, tests; `internal/api/terminal.go`.

**Interfaces:**
- Produces: profile fields `DefaultBaseBranch string` (`defaultBaseBranch`) and `WorktreesByDefault bool` (`worktreesByDefault`, default true) on the existing profile API; terminal session records gain `ProjectID string` (nullable), and the session listing returns `tool`, `model`, `effort`, `yolo`, `projectId`, `worktreeBranch` (never the prompt).

- [ ] Step 1: failing tests: `TestProfileDefaultsRoundTrip` (new user → `worktreesByDefault` true, base branch blank; invalid ref refused), `TestSessionListingCarriesLaunchMetadataNotPrompt`, `TestCreateWithUnknownProjectIDRefused` (another account's id → 404 `project_not_found`).
- [ ] Step 2–4: FAIL, implement, `go test -race ./internal/store/ ./internal/api/ -run 'Profile|SessionListing|ProjectID' -count=1` PASS. Commit "Record each session's project and the person's launch defaults".

### Task B3: Folder project API

**Files:** Create `internal/api/folder_projects.go`, `folder_projects_test.go`; modify `api.go`, `web/src/lib/terminal/api.ts`.

**Interfaces:**
- Consumes: Task B1.
- Produces: `GET/POST /api/v2/folder-projects`, `GET/PATCH/DELETE /api/v2/folder-projects/{id}` (`ETag: "<version>"`, `If-Match`), `POST /api/v2/folder-projects/reorder` `{ids, versions}`; errors 409 `project_exists` "studio already has a project for that folder.", 409 `project_limit` "You have 100 projects, the most reet keeps. Remove one first.", 409 `project_changed` "Someone changed this project. Reload it, then try again.", 422 per field; delete answers `{message:"Project removed. Its sessions and any worktrees stay on studio."}`; 30 writes per minute per account (429 `project_rate_limited`); audit `project-add`, `project-update`, `project-remove` (machine, folder, name; never the starter prompt); web client `listFolderProjects()`, `createFolderProject(input)`, `updateFolderProject(id, input, version)`, `deleteFolderProject(id, version)`, `reorderFolderProjects(ids, versions)`, `browseFolder(agentId, {path, after, filter, hidden, session})`, `listBranches(agentId, dir, filter)`.

- [ ] Step 1: failing tests: `TestFolderProjectRoutesAccess`, `TestFolderProjectCreateValidatesAndAudits` (audit has no starter prompt), `TestFolderProjectStaleIfMatch` (two PATCHes with one version → second 409 with the sentence), `TestFolderProjectDeleteThenStalePatch` (PATCH after delete → 404, nothing recreated), `TestFolderProjectWriteRateLimit`, `TestFolderProjectStrictJSON`, `TestFolderProjectNoStore` (`Cache-Control: no-store`).
- [ ] Step 2–4: FAIL, implement, `go test -race ./internal/api/ -run FolderProject -count=1` PASS. Commit "Add the folder projects API".

### Task B4: Migrate saved Projects

**Files:** Modify `internal/store/store_projects.go`, create `internal/store/store_projects_migrate_test.go`; modify `internal/api/projects.go`.

**Interfaces:**
- Consumes: Tasks B1, B3; existing `Project`, `ProjectMember`, `Run`.
- Produces: `func (s *Store) MigrateProjectsToFolders() (MigrationReport, error)` run once at controller start inside one transaction, idempotent (a `projects_migrated` setting); `MigrationReport{Users int; Created, Skipped int}`; per user one `projects-migrated` audit event per Project `{project, folderProjects:[ids]}`; folder projects get `LegacyProjectID` (first Project) and `StarterPrompt` (R2: the oldest Project's prompt wins; a later conflicting Project's name is recorded in `FolderProjectNote` — add `Note string` to `FolderProject`, shown once in the rail); old Projects become read-only (`PATCH`/`DELETE`/start → 410 `projects_moved` "Projects now live in the rail on Home.").

- [ ] Step 1: failing tests: `TestMigrationDedupesByMachineAndFolder` (two Projects sharing `/w/reet` on studio → one folder project), `TestMigrationNamesAndDefaults` (member tool beats Project default; `useWorktrees` → `yes`/`no`; name clash → ` (2)`), `TestMigrationCarriesStarterPrompts` (prompt copied to every folder created from it; conflicting prompts keep the oldest and note the other Project's name), `TestMigrationSkipsRegisteredFolder` (existing folder project untouched, prompt not overwritten), `TestMigrationIsIdempotent` (second run creates 0), `TestMigrationKeepsRuns` (runs and their worktree records still load; worktree remove still allowed), `TestMigrationAuditHasNoPrompt`, `TestOldProjectRoutesAnswer410`.
- [ ] Step 2–4: FAIL, implement, `go test -race ./internal/store/ ./internal/api/ -run 'Migration|OldProject' -count=1` PASS. Commit "Move saved Projects into folder projects, keeping prompts and runs".

### Task B5: Retire the Projects page

**Files:** Modify `web/src/App.svelte`, `web/src/lib/Account.svelte` (activity list extracted), create `web/src/lib/ActivityPage.svelte`; delete `ProjectsPage.svelte`, `ProjectPage.svelte`, `ProjectDialog.svelte`, `StartRunDialog.svelte`, `DeleteProjectDialog.svelte`; keep `RunView.svelte` read-only; rewrite `web/tests/projects.e2e.mjs` as redirect checks; modify `docs/projects.md`.

**Interfaces:**
- Consumes: Task B4.
- Produces: `/projects` → `/` and `/projects/<id>` → `/?project=<folderProjectId>` (via `GET /api/v2/folder-projects?legacy=<id>`), each with the one-time status note "Projects now live in the rail on Home." (shown once per page load, not stored); `/activity` route rendering the account activity list plus *Runs from Projects* (read-only `RunView` with Check and remove worktree); `eventLabel` for `projects-migrated`, `project-add`, `project-update`, `project-remove`, `folder-browse`.

- [ ] Step 1: failing fixture cases in `projects.e2e.mjs`: "old list redirects with the note", "old project id redirects to its folder project", "old run is read-only in Activity and its worktree can be checked and removed", "viewer sees no Runs from Projects"; `events.test.mjs` labels for the five events.
- [ ] Step 2: `cd web && npm run build && node tests/projects.e2e.mjs` — FAIL.
- [ ] Step 3: implement; rewrite `docs/projects.md` for folder projects (registry, limits, migration, starter prompts, runs in Activity).
- [ ] Step 4: run the fixture 4 times and `npm test` — PASS. Commit "Retire the Projects page; keep its runs in Activity".

**PR B validation, review and merge:** the common steps. Do not tag a release until PR E merges (R22).

## PR C — agent catalogue and launch fields

### Task C1: `agents-v2` on the wire

**Files:** Create `internal/terminalproto/agents.go`, `agents_test.go`; modify `protocol.go`, `launch.go`.

**Interfaces:**
- Produces: `const AgentsFeature = "agents-v2"`, `SupportsAgents(features string) bool`; `ValidToolV2(id string) bool` over `claude, codex, opencode, gemini, pi, cursor, cline` (`ValidTool` stays the four); `Create.Base string` (`base,omitempty`, valid ref name ≤255 bytes, requires `Worktree`), `Create.Effort string` (`effort,omitempty`, `low|medium|high`, requires `Tool`), `Create.Yolo bool` (`yolo,omitempty`, requires `Tool`); `Capabilities.Yolo bool` (`yolo,omitempty`, invalid without `Launch`); `func (c Capabilities) ValidateFor(features string) error` (new ids accepted only when `agents-v2` negotiated); `WorktreeReason` gains `base_missing`; `ResultCode` gains `launch_option_unsupported`, `yolo_refused`.

- [ ] Step 1: failing tests: `TestCreateNewFieldsOmittedWhenEmpty` (old-shape JSON byte-identical), `TestCreateBaseRequiresWorktree`, `TestCreateEffortValues`, `TestCreateStillComparable`, `TestCapabilitiesNewToolIDsNeedAgentsV2` (`pi` without the token → invalid), `TestCapabilitiesYoloRequiresLaunch`.
- [ ] Step 2–4: FAIL, implement, `go test -race ./internal/terminalproto/ -count=1` PASS. Commit "Add agents-v2 launch fields to the terminal protocol".

### Task C2: Connector catalogue and new presets

**Files:** Create `internal/terminalhost/catalogue.go`, `catalogue_test.go`, `testdata/help/{claude,codex,opencode,gemini,pi,cursor-agent,cline}.txt`; modify `launch.go`.

**Interfaces:**
- Consumes: Task C1.
- Produces: `type Preset struct { ID, Name, Binary string; ModelFlag string; PromptPositional bool; EffortArgs map[string][]string; YoloArgs []string; SafeArgs []string; ACP string /* "native"|"adapter"|"" */ }`; `var Catalogue []Preset` (the seven ids; `launchPresets` becomes a view of it); `func InstalledTools(lookPath, v2 bool) []string` (only the four without `v2`); `LaunchArgv(in terminalproto.Create, lookPath func(string) (string, error)) ([]string, error)` appends `SafeArgs`, maps `Effort` via `EffortArgs` (missing → `launch_option_unsupported`) and never reads `Yolo` in this PR (true → `yolo_refused`).

- [ ] Step 1: write the help-text test data by running each CLI's `--help` on a machine that has it (record the CLI version in the file's first line). Then failing tests: `TestPresetFlagsAppearInHelpText` (every flag in `ModelFlag`, `EffortArgs`, `YoloArgs`, `SafeArgs` occurs verbatim in that preset's help file; a preset whose file is absent has none of them set), `TestClineAlwaysPassesAutoApproveFalse` (argv contains `--auto-approve false` exactly once, with and without a prompt), `TestOldControllerSeesOnlyFourTools`, `TestLaunchArgvEffortMapping` (unsupported effort → `launch_option_unsupported`), `TestLaunchArgvYoloRefusedBeforeF`, `TestLaunchArgvNewPresets` (exact argv for `pi`, `cursor`, `cline` with model and prompt, per their confirmed help text).
- [ ] Step 2–4: FAIL, implement, `go test -race ./internal/terminalhost/ -count=1` PASS. Commit "Add the agent catalogue with Pi, Cursor Agent and Cline".

### Task C3: Worktree from a named base

**Files:** Modify `internal/terminalhost/worktree.go`, `worktree_test.go`, `manager.go`.

**Interfaces:**
- Produces: `PrepareWorktree(ctx, home, repo, project, run, base string, git GitRunner)` (new `base` parameter; empty = `HEAD`): resolves `rev-parse --verify --end-of-options refs/heads/<base>^{commit}`, then `refs/remotes/<base>^{commit}`, and passes the commit to the one `worktree add`; unknown → `ErrWorktreeRefused` reason `base_missing`.

- [ ] Step 1: failing tests (real git): `TestWorktreeFromLocalBase`, `TestWorktreeFromRemoteBase`, `TestWorktreeBaseMissing` (nothing created), `TestWorktreeBaseArgvExact` (recording runner; `--end-of-options` present; a base of `-x` is refused by validation before git).
- [ ] Step 2–4: FAIL, implement, `go test -race ./internal/terminalhost/ -run Worktree -count=1` PASS. Commit "Create a session's worktree from a chosen branch".

### Task C4: Controller and web catalogue

**Files:** Modify `internal/terminalbroker/broker.go`, `internal/api/terminal.go`, tests; create `web/src/lib/catalogue.ts`, `catalogue.test.mjs`; modify `web/src/lib/terminal/api.ts`, `web/package.json` (test script), `docs/terminal-launch.md`.

**Interfaces:**
- Consumes: Tasks C1–C3.
- Produces: API create accepts `base`, `effort`, `yolo` and refuses them to a peer without `agents-v2` (409 `launch_option_unsupported` "Update reet on that machine to use that option."); 422 `base_missing` "That branch isn't in the repository on studio."; `terminal-launch` audit gains `effort`, `base`, `yolo`; `catalogue.ts`: `type CatalogueAgent = { id: string; name: string; models: string[]; effort: boolean; enabled: boolean; disabledReason?: string }`, `const CATALOGUE: CatalogueAgent[]` (the seven enabled plus the owner's remaining agents with `enabled:false`, reason "reet doesn't start this agent yet."), `agentChoices(machine: TerminalMachine): {agent: CatalogueAgent; available: boolean; reason?: string}[]` (reasons "Not installed on studio", "Update reet on that machine").

- [ ] Step 1: failing tests: `TestCreateNewFieldsRefusedToOldPeer`, `TestCreateBaseMissingSentence`, `TestLaunchAuditCarriesEffortBaseYolo`; `catalogue.test.mjs` "installed first, then not installed, then disabled", "old machine offers only four".
- [ ] Step 2–4: FAIL, implement, `go test -race ./internal/api/ -run Create -count=1` and `cd web && npm test` PASS; update `docs/terminal-launch.md` (catalogue, effort, base). Commit "Offer the catalogue, effort and base branch through the API".

**PR C validation, review and merge:** the common steps.

## PR D — the app shell and Home

Starts after the shared dialog shell has merged.

### Task D1: Theme tokens, fonts, logo

**Files:** Modify `web/src/app.css`, `web/src/lib/Logo.svelte`; create `web/public/fonts/` (Fraunces 600, IBM Plex Sans 400/500/600, IBM Plex Mono 400 as subset WOFF2, `OFL.txt`), `web/src/lib/theme.ts`, `theme.test.mjs`.

**Interfaces:**
- Produces: tokens `--bg --card --card-raised --line-soft --line --line-strong --ink --muted --accent --accent-text --accent-ink --good --warn --info --repo --bad` with the spec's dark and light values under the existing `prefers-color-scheme` / `data-theme` selectors; `--serif: 'Fraunces', …`, `--sans: 'IBM Plex Sans', …`, `--mono: 'IBM Plex Mono', …` via `@font-face` with `font-display: swap`; `a { color: var(--accent-text) }`; `theme.ts`: `export const TOKENS: Record<'dark'|'light', Record<string,string>>` and `contrast(fg: string, bg: string): number`.

- [ ] Step 1: failing `theme.test.mjs`: "every text pair passes AA" (`--ink`, `--muted`, `--accent-text`, `--good`, `--warn`, `--info`, `--repo` on `--bg` and `--card` ≥ 4.5; `--accent-ink` on `--accent` ≥ 4.5) in both palettes; "app.css matches TOKENS" (parses `app.css`).
- [ ] Step 2–4: FAIL, implement, `npm test` PASS; check no request leaves the origin for fonts (`navigation.e2e.mjs` asserts every font request is same-origin). Commit "Give reet its warm ink palette and self-hosted type".

### Task D2: App shell and navigation

**Files:** Create `web/src/lib/shell/AppShell.svelte`, `TopTabs.svelte`, `Bell.svelte`, `BottomTabs.svelte`; modify `web/src/App.svelte`, `NavigationMenu.svelte`, `AccountMenu.svelte`, `web/tests/navigation.e2e.mjs`.

**Interfaces:**
- Consumes: Task D1, Task B5's `/activity`.
- Produces: `AppShell` props `{ tabs: Tab[]; attention: number; compact: boolean }`; `type Tab = { href: string; label: string; current: boolean }`; desktop tabs Home `/`, Sessions `/sessions`, Machines `/machines`, Skills `/skills`, Activity `/activity`; tablet and phone tabs Home, Sessions, Machines, More (More holds Skills, Activity, Setups, Sync, Lists, Community, Support, Teams & organisations, Host skills); account menu **More** group on desktop (R13); `Bell` button named "Needs you: N" (0 hides the badge; count wired in PR G); "Jump to a session, project or machine" search (`/` focuses) over sessions, projects and machines already loaded; redirects `/control` → `/`, `/terminals[?agent&session]` → `/sessions[?…]`; viewers and non-accounts modes: Home with fleet strip only (R16).

- [ ] Step 1: failing `navigation.e2e.mjs` cases: "writer desktop shows five tabs with Home current", "phone shows four bottom tabs and More lists the rest", "every former nav destination reachable", "/control and /terminals redirect keeping the query", "viewer sees no Sessions tab and no bell", "tab links use aria-current=page".
- [ ] Step 2–4: FAIL, implement, run the fixture 4 times and every existing fixture — PASS. Commit "Add the top tabs, bell and responsive navigation".

### Task D3: Home logic

**Files:** Create `web/src/lib/home.ts`, `home.test.mjs`.

**Interfaces:**
- Consumes: `TerminalSession`, `MachineHealth`, `SyncState`, `FolderProject` (web type from B3).
- Produces: `fleetCards(machines: MachineHealth[], sessions: TerminalSession[], sync: SyncState, worktrees: Record<string, number>, now: number): FleetCard[]` with `FleetCard = { agentId: string; name: string; os: string; status: 'online'|'offline'|'reconnecting'; agentsRunning: number; cpu: number | null; worktrees: number; skills: { text: string; tone: 'good'|'warn'|'muted' } }` (skills text "skills in sync" only when the last check found no differences, "1 skill out of date", else "Not checked"); `inFlight(sessions, projects, groupBy: 'project'|'machine'|'state', now): Group[]` (finished kept 24 h unless seen); `rowSubtitle(session, project): string` (`project · machine · branch`); `stateWord(session): {word: string; source: string; tone}` (never a step count for terminal sessions); `needsYou(sessions, now): NeedCard[]` with kinds `waiting|failed|finished|review` (review = finished worktree session).

- [ ] Step 1: failing tests: "cpu null shows No reading", "agents running counts only live agent sessions", "skills line never says in sync without a check", "group by project puts unmatched sessions under Other on studio", "finished leaves In flight after 24 h", "no step text for terminal sessions", "needs you newest first, one card per session".
- [ ] Step 2–4: FAIL, implement, `npm test` PASS. Commit "Add the pure logic for Home".

### Task D4: Home page

**Files:** Create `web/src/lib/home/Home.svelte`, `FleetStrip.svelte`, `InFlight.svelte`, `NeedsYou.svelte`, `ProjectRail.svelte`; modify `App.svelte`.

**Interfaces:**
- Consumes: Tasks D2, D3, B3; existing `/api/v2` sessions, health and sync loaders; worktree counts from the session listing.
- Produces: Home at `/` for writers; layout per the spec's desktop and tablet drawings (rail 272 px, collapsing to 56 px under 1240 px; Needs you 380 px; tablet stacks machines, Needs you, In flight); `ProjectRail` emits `choose(projectId)` and `selection(ids: string[])`; project card menu (New session here, Edit, Worktrees, Remove from reet) with confirmations; Group by stored per account under `reet.home.groupBy.<uid>` (try/catch); polling 15 s while visible; empty states "Nowt waiting on you." and "Add a project to start an agent in one of your folders."; a placeholder region for the launch bar (filled in PR E).

- [ ] Step 1: failing `home.e2e.mjs` cases (synthetic API): "fleet strip shows each figure with its label", "Add a machine opens the existing dialog", "In flight groups by project, machine and state", "YOLO badge on a YOLO session", "Needs you cards for waiting, failed and ready to review", "offline machine keeps its sessions with the sentence", "refresh error keeps the last list", "nothing of projects or sessions in storage".
- [ ] Step 2–4: FAIL, implement, run 4 times — PASS. Commit "Build Home: fleet strip, In flight, Needs you and the project rail".

### Task D5: Folder browser and Add project dialog

**Files:** Create `web/src/lib/folderBrowser.ts`, `folderBrowser.test.mjs`, `web/src/lib/home/FolderBrowser.svelte`, `AddProjectDialog.svelte`.

**Interfaces:**
- Consumes: Task B3 client (`browseFolder`, `createFolderProject`, `updateFolderProject`), the shared dialog shell `shell` action (`sheet: 'full'` on phones).
- Produces: `breadcrumbs(path: string, home: string): {label: string; path: string}[]` (home shown as `~`); `repoMessage(result: BrowseResult): string | null` (the spec's two sentences); `countLabel(n: number, more: boolean): string` ("14 folders", "200+ folders"); `validateFolderInput(path: string): string | null`; `AddProjectDialog` props `{ machines; project?: FolderProject; onadded(p) }` doing add and edit; fields per the spec (Machine, Folder with Browse/Hide browser, Name, Worktree for each session, Starter prompt and More defaults collapsed); success line "Sorted. reet added."; phone sheet with chosen folder and buttons pinned.

- [ ] Step 1: failing `folderBrowser.test.mjs`: "breadcrumb of a path with spaces and #", "never above home", "inside a repository offers the top folder", "count labels"; then `add-project.e2e.mjs`: "browse, enter, Use this folder", "paging with Show more", "hidden toggle and filter", "not a repository message", "inside a repository button", "offline machine adds by typed path", "offline mid-browse keeps the list", "rate limited sentence", "stale edit keeps input", "phone full-screen sheet with pinned buttons", "focus returns to Add on close".
- [ ] Step 2–4: FAIL, implement, run both 4 times — PASS. Commit "Add projects by browsing a machine's folders".

### Task D6: Fixtures in CI and the zoom matrix

**Files:** Modify `web/tests/phone-zoom.e2e.mjs`, `.github/workflows/terminal-browser.yml`, `docs/terminal-workspace.md`, `docs/control-centre.md`.

- [ ] Step 1: add Home and the Add project sheet with a 120-character path to `phone-zoom.e2e.mjs` (320–412 px: no layout wider than the device, inputs 16 px); add `home.e2e.mjs` and `add-project.e2e.mjs` to the workflow split across the existing jobs (≤60 s added).
- [ ] Step 2: mutation-check: remove the browse containment message, the 16 px input rule and the "never in sync without a check" rule one at a time; each fixture must fail; restore.
- [ ] Step 3: update `docs/terminal-workspace.md` (Home, rail) and `docs/control-centre.md` (folded into Home, R4). Commit "Check Home and Add project in CI".

**PR D validation, review and merge:** the common steps.

## PR E — the launch bar

### Task E1: Launch bar logic

**Files:** Create `web/src/lib/launchBar.ts`, `launchBar.test.mjs`.

**Interfaces:**
- Consumes: `catalogue.ts`, `FolderProject`, profile defaults (B2), `listBranches` (B3).
- Produces: `type LaunchChoice = { agent: string; model: string; projectIds: string[]; homeMachine?: string; branch: string; worktree: boolean; effort: ''|'low'|'medium'|'high'; prompt: string; approvals: 'ask'|'yolo' }`; `initialChoice(last: LaunchChoice | null, projects, profile): LaunchChoice` (approvals always `ask`); `baseBranchFor(project, profile, branches: BranchesResult): string` (project → profile → `default` → `current`); `sentence(choice, projects, machines): {slot: string; text: string}[]` ("Run … with … on … at … from … in a fresh worktree"; "3 projects", "each project's default"); `startable(choice, machines): string | null` (reason or null); `toCreate(choice, project, requestId, runToken): LaunchRequest` (adds `base`, `effort`, `yolo` only when set); `defaultTitle(prompt: string): string` (first line, ≤80 characters); `sendKey(e: KeyboardEvent, touch: boolean): boolean` (R8).

- [ ] Step 1: failing tests: "base precedence project, profile, default, current", "checkout hides branch choice and reads Works in the checkout on main", "non-repository hides branch and worktree", "approvals reset to ask", "sentence for several projects", "Enter never sends on touch; Ctrl+Enter and Cmd+Enter send", "title from the first line", "a prompt starting with - is refused with the launch sentence".
- [ ] Step 2–4: FAIL, implement, `npm test` PASS. Commit "Add the launch sentence logic".

### Task E2: Launch bar component

**Files:** Create `web/src/lib/launch/LaunchBar.svelte`, `SlotPicker.svelte`; modify `home/Home.svelte`, `web/src/lib/terminal/api.ts`.

**Interfaces:**
- Consumes: Task E1, the existing idempotent create client and lost-reply handling from `agentLaunch.ts`.
- Produces: the docked bar on Home (desktop: prompt above the sentence; tablet: below; phone: one-line prompt, round Start 48 × 48 px and a summary line with **Change** opening a bottom sheet); slot pickers as listbox popovers (desktop) or bottom sheets (touch); status line "Starting Claude Code on studio…"; on success navigate to `/sessions?agent=…&session=…`; secrets note under the box; **Use the starter prompt** chip when the box is empty; Approvals radio group (YOLO half disabled until PR F, with its reason).

- [ ] Step 1: failing `launch.e2e.mjs` cases: "one press starts with the last choices", "each slot opens and sets its value", "unready machine disabled with its sentence", "starter prompt is offered, never filled", "lost reply retries the same request" (same request id and body), "a project folder named -x launches with dir unchanged", "bar locked while starting", "phone Change sheet", "nothing in storage".
- [ ] Step 2–4: FAIL, implement, run 4 times — PASS. Commit "Start sessions from the launch sentence".

### Task E3: Start in several projects

**Files:** Modify `internal/store/store_projects.go`, `internal/projectrun/run.go`, `run_test.go`, `internal/api/project_runs.go`, tests; `web/src/lib/launch/LaunchBar.svelte`, `home/ProjectRail.svelte`.

**Interfaces:**
- Consumes: Task B1, existing `StartRunAtFor`, orchestrator rules (2 in flight, 1 per machine, derived ids).
- Produces: `func (s *Store) StartSelectionRunFor(uid int64, requestID string, projectIDs []string, launch RunLaunch) (Run, bool, error)` with `RunLaunch{Tool, Model, Effort, Prompt string; Worktree, Yolo bool}` snapshotting one member per folder project (its host, dir, `baseBranch`); runs gain `origin` (`set`|`selection`); `POST /api/v2/folder-projects/runs` `{requestId, projectIds, tool, model, effort, prompt, worktree, yolo}` (2–12 projects); status sheet "2 of 3 started" with per-member Retry.

- [ ] Step 1: failing tests: `TestSelectionRunSnapshotsFolderProjects`, `TestSelectionRunLimits` (1 or 13 projects refused), `TestSelectionRunResumesOnlyPendingAfterRestart`, `TestSelectionRunMemberUsesProjectBase`, `TestSelectionRunAuditHasNoPrompt`; `launch.e2e.mjs` "select three projects and start; one fails and retries".
- [ ] Step 2–4: FAIL, implement, `go test -race ./internal/store/ ./internal/projectrun/ ./internal/api/ -run SelectionRun -count=1` and the fixture PASS. Commit "Start in several projects from the rail".

### Task E4: CI and docs

**Files:** Modify `.github/workflows/terminal-browser.yml`, `web/tests/phone-zoom.e2e.mjs`, `docs/start-agent-session.md`, `docs/terminal-worktrees.md`.

- [ ] Step 1: add `launch.e2e.mjs` to the workflow; add the phone launch sheet to `phone-zoom.e2e.mjs`.
- [ ] Step 2: mutation-check: send on Enter on touch, drop the request id on retry, forget to reset approvals; each fixture must fail; restore.
- [ ] Step 3: docs: the launch sentence, base branch and branch listing. Commit "Check the launch bar in CI".

**PR E validation, review and merge:** the common steps; PR B to PR E may now be released together.

## PR F — YOLO

### Task F1: Account setting

**Files:** Create `internal/api/yolo_settings.go`, `yolo_settings_test.go`; modify `api.go`, admin settings UI (`web/src/lib/Admin.svelte`).

**Interfaces:**
- Produces: setting key `terminal_allow_yolo` (default `false`) via `SetSetting`/`GetSetting`; `GET /api/v2/settings/yolo` → `{allowed, changedBy, changedAt}` (writers); `PUT` owners and admins only; audit `yolo-allowed` / `yolo-disallowed`; an **Allow YOLO** switch in Admin with the sentence "Lets people start a session where the agent doesn't ask before acting. Each session still needs its own confirmation."

- [ ] Step 1: failing tests: `TestYoloSettingDefaultsOff`, `TestYoloSettingOnlyOwnersAndAdmins`, `TestYoloSettingAudited`. Step 2–4: FAIL, implement, PASS. Commit "Add the account's Allow YOLO setting".

### Task F2: Machine side

**Files:** Modify `internal/terminalhost/catalogue.go`, `catalogue_test.go`, `testdata/help/*`, `internal/scan/agent.go`, `agent_terminal.go`, `internal/terminalagent/agent.go`.

**Interfaces:**
- Produces: `agentConfig.YoloOff bool` (`yoloOff,omitempty`); `reet terminal yolo on|off|status`; `Capabilities.Yolo` true when `agents-v2` negotiated and not `YoloOff`, re-read from config on every create; `LaunchArgv` maps `Yolo` to `YoloArgs` (Cline: replaces `--auto-approve false`), refuses with `yolo_refused` when off or the preset has none; candidate options checked against help text in Step 1: Claude Code `--dangerously-skip-permissions`, Codex `--dangerously-bypass-approvals-and-sandbox`, Gemini CLI `--yolo`; any not present in its help file leaves that preset without YOLO.

- [ ] Step 1: failing tests: `TestYoloArgsAppearInHelpText`, `TestYoloOffRemovesCapabilityAndRefuses` (switch flipped between two creates → second refused), `TestYoloNeverWithoutFlag` (Yolo false → no YoloArgs for every preset), `TestClineYoloReplacesAutoApproveFalse`.
- [ ] Step 2–4: FAIL, implement, `go test -race ./internal/terminalhost/ ./internal/scan/ ./internal/terminalagent/ -count=1` PASS. Commit "Let machines start YOLO sessions unless refused".

### Task F3: Controller gates and audit

**Files:** Modify `internal/api/terminal.go`, `internal/terminalbroker/broker.go`, tests, `internal/api/terminal_launch_integration_test.go`.

**Interfaces:**
- Produces: create with `yolo:true` refused 403 `yolo_not_allowed` "An owner or admin can allow YOLO for this account in Settings." when the setting is off, 409 `yolo_refused` "studio doesn't accept YOLO sessions." without the capability, 409 `launch_option_unsupported` "This agent has no option reet can use for that."; `terminal-launch` with `yolo:true` plus `yolo-launch` `{machine, tool, session, worktree}`; session listing `yolo` true; `GET /api/v2/machines/{agentId}/yolo` → `{accepts, checkedAt}` for the dialog checklist.

- [ ] Step 1: failing tests: `TestYoloCreateGates` (each of the three refusals), `TestYoloLaunchAudited`, `TestYoloNeverSentToPeerWithoutCapability`; real two-host `TestTerminalPrivateYoloLaunchUsesTheAgentsOption` (recording fake CLI sees the option exactly once).
- [ ] Step 2–4: FAIL, implement, PASS. Commit "Gate and audit YOLO launches".

### Task F4: Approvals, confirmation dialog, badges

**Files:** Create `web/src/lib/launch/YoloDialog.svelte`, `web/tests/yolo.e2e.mjs`; modify `LaunchBar.svelte`, `launchBar.ts` (+ test), `home/InFlight.svelte`, `.github/workflows/terminal-browser.yml`.

**Interfaces:**
- Consumes: Tasks F1–F3, the shared dialog shell (`alertdialog`).
- Produces: `yoloGates(choice, setting, machines, agent): {label: string; ok: boolean; detail: string}[]` (the three checklist lines, the worktree line as a recommendation); the dialog's copy verbatim from the spec with the terminal-agent variants; **Keep asking me** default focus; **Start with YOLO** launches; YOLO badge on rows, cards, tabs and the session header; Approvals back to *Ask me* after launch.

- [ ] Step 1: failing `yolo.e2e.mjs` cases: "YOLO opens the dialog before the control moves", "Keep asking me leaves Ask me", "each failed gate disables Start with YOLO with its sentence", "several projects list every machine", "badge everywhere the session appears", "approvals reset after launch", "nothing stored"; `launchBar.test.mjs` "yolo gates".
- [ ] Step 2–4: FAIL, implement, run 4 times — PASS; mutation-check YOLO without the account setting and the reset. Commit "Ask before starting a session on YOLO".

### Task F5: Docs that said auto-approve is not offered

**Files:** Modify `docs/start-agent-session.md`, `docs/terminal-launch.md`, `docs/superpowers/specs/2026-10-25-start-agent-session-design.md` ("No permission-bypass flags" line amended), `docs/roadmap.md`.

- [ ] Step 1: amend each passage to say YOLO exists, is off by default, needs the account setting, the machine's acceptance and a per-session confirmation, and is audited; update the test that pins the only flags (`TestPresetFlags…`) to allow exactly `YoloArgs` when `Yolo` is true. Commit "Document YOLO and its gates".

**PR F validation, review and merge:** the common steps.

## PR G — attention

### Task G1: `attention-v1` on the machine

**Files:** Create `internal/terminalproto/attention.go`, `attention_test.go`, `internal/terminalhost/attention.go`, `attention_test.go`; modify `protocol.go`, `manager_tmux.go`.

**Interfaces:**
- Produces: `const AttentionFeature = "attention-v1"`; `SessionSnapshot.ActivityAt int64` (`activityAt,omitempty`), `BellAt int64` (`bellAt,omitempty`), `ExitStatus *int` (`exitStatus,omitempty`), sent only when negotiated; tmux sessions get `monitor-bell on`; `func ReadAttention(ctx, tmux, session string) (activityAt, bellAt int64, err error)` from `#{window_activity}` and `#{window_bell_flag}` (bell time recorded by the connector when the flag is first seen set, then cleared).

- [ ] Step 1: failing tests: `TestSnapshotAttentionFieldsOnlyWhenNegotiated`, `TestReadAttentionActivityAndBell` (real tmux: `printf '\a'` sets `BellAt`; output moves `ActivityAt`), `TestAttentionCarriesNoOutput` (snapshot JSON contains no pane text).
- [ ] Step 2–4: FAIL, implement, PASS. Commit "Report terminal activity and bell under attention-v1".

### Task G2: Derived state and seen

**Files:** Create `internal/store/store_attention.go`, `store_attention_test.go`, `internal/api/attention.go`, `attention_test.go`.

**Interfaces:**
- Produces: `func AttentionState(s Session, now time.Time) (state, source string)` with states `starting|working|quiet|waiting|finished|failed` and source `terminal` (30 s working window; waiting when `BellAt > seenAt`); `MarkSessionSeenFor(uid int64, sessionID string, at int64) error`; listing adds `attention`, `attentionSource`, `changedSinceSeen`; `POST /api/v2/terminal/sessions/{id}/seen`.

- [ ] Step 1: failing tests: `TestAttentionWorkingThenQuiet`, `TestAttentionWaitingOnlyAfterBell`, `TestAttentionExitStatus` (0 → finished, 2 → failed "Exited 2"), `TestSeenIsPerPerson`, `TestOldPeerSessionsHaveNoAttention`.
- [ ] Step 2–4: FAIL, implement, PASS. Commit "Derive attention and remember what each person has seen".

### Task G3: Needs you queue, bell, title, live region

**Files:** Modify `web/src/lib/home.ts`, `home.test.mjs`, `home/NeedsYou.svelte`, `shell/Bell.svelte`, `App.svelte`, `web/tests/home.e2e.mjs`.

**Interfaces:**
- Produces: `needsYou` uses `attention`; `attentionCount(cards): number`; bell popover listing the same cards (phones: Home's Needs you); document title `(3) Home · reet.sh`; polite live region announcing transitions into Waiting at most once per session per minute; **Mark seen** calls the seen route.

- [ ] Step 1: failing tests: `home.test.mjs` "bell count equals queue length", "announce at most once a minute"; `home.e2e.mjs` "bell opens the queue", "title count", "Mark seen removes a finished card", "state word says From terminal activity".
- [ ] Step 2–4: FAIL, implement, run 4 times — PASS; mutation-check the bell count. Commit "Show what needs you across every machine".

**PR G validation, review and merge:** the common steps.

## PR H — session screen, tablet and phone

### Task H1: Sessions page

**Files:** Create `web/src/lib/sessions.ts`, `sessions.test.mjs`, `web/src/lib/sessions/SessionsPage.svelte`, `web/tests/sessions.e2e.mjs`; modify `App.svelte`.

**Interfaces:**
- Consumes: `home.ts` grouping, the existing session actions menu from `SessionSidebar.svelte` (moved, not rewritten).
- Produces: `searchSessions(sessions, projects, machines, q): TerminalSession[]` (title, agent, model, branch, project, machine); `filterSessions(sessions, f: {state?, agent?, machine?, yoloOnly?, worktreeOnly?})`; `/sessions` with Group by, search, filters, rows, actions menu, and the *Projects · N* section with **Add a project**.

- [ ] Step 1: failing `sessions.test.mjs` and `sessions.e2e.mjs` cases: "search matches branch and machine", "filters combine", "actions menu by right-click and Shift+F10", "offline machine group sentence", "rows at least 44 px on touch". Step 2–4: FAIL, implement, PASS. Commit "Add the Sessions page".

### Task H2: Session screen

**Files:** Create `web/src/lib/sessions/SessionScreen.svelte`, `SessionTabs.svelte`, `ContextPanel.svelte`; modify `TerminalWorkspace.svelte` (host the pane), `sessions.e2e.mjs`.

**Interfaces:**
- Consumes: `TerminalPane`, worktree status client, `fleetCards` skills line.
- Produces: tab strip of sessions opened in this tab (≤8, memory only, R20); header with state pill and source, YOLO badge, **Chat | Terminal** (Chat disabled: "Chat arrives with structured sessions."), actions menu; context panel sections *Where it's working*, *Changed files* (≤20 paths and total, refresh button, no polling), *Skills on this machine* with **Manage**, *Decisions in this session* ("Asked in the terminal." or the YOLO line); panel collapses behind **Details** under 1100 px.

- [ ] Step 1: failing `sessions.e2e.mjs` cases: "opening a row adds a tab; closing a tab never ends the session", "ninth tab drops the oldest", "context panel shows machine, project, branch, worktree", "changed files fetched once and on refresh", "Chat is disabled with its reason", "YOLO session shows its decisions line". Step 2–4: FAIL, implement, PASS. Commit "Give each session a screen with its context".

### Task H3: Tablet and phone layouts

**Files:** Modify `shell/BottomTabs.svelte`, `home/*.svelte`, `launch/LaunchBar.svelte`, `sessions/*.svelte`, `web/src/lib/terminal/sizing.ts` (only a new exported `keyboardChrome(focus: 'launch'|'session'|null, open: boolean): boolean`), `web/tests/phone-layout.e2e.mjs`, `terminal-phone.e2e.mjs`.

**Interfaces:**
- Produces: tablet (834 px): machines scroll sideways, Needs you first, docked bar, 44 px targets, Details side sheet; phone: bottom tabs 56 px safe-area padded, machine chips, Needs you first with 48 px full-width Open, Sessions screen with the switcher, session screen slim header, chrome hidden with the keyboard up using the existing usable-height rule.

- [ ] Step 1: failing cases: `phone-layout.e2e.mjs` "launch prompt with keyboard open" and "session with keyboard open" at 320 and 390 px (top and tab bars hidden; Start and the field inside the visual viewport; no horizontal scroll), "sideways 844 × 390", tablet "Needs you before In flight"; `terminal-phone.e2e.mjs` still green.
- [ ] Step 2–4: FAIL, implement, run 4 times — PASS. Commit "Lay out Home and sessions for tablets and phones".

### Task H4: Full matrix, axe, docs, handoff

**Files:** Modify every new fixture, `phone-zoom.e2e.mjs`, `.github/workflows/terminal-browser.yml`, `docs/ui-verification.md`, `docs/terminal-workspace.md`, `docs/roadmap.md`, `handoff.md`, `README.md`.

- [ ] Step 1: run the matrix in `home`, `add-project`, `launch`, `yolo`, `sessions` fixtures: 1440, 1360, 1024, 834 px; 412, 390, 360, 320 px with `isMobile`/`hasTouch`; 844 × 390; light and dark; reduced motion; keyboard-open emulation. Assert no horizontal scroll, targets ≥44 px, focus visible, state never colour alone.
- [ ] Step 2: axe-core at 390, 834 and 1440 px in both themes; record violations and the `incomplete` count honestly in `docs/ui-verification.md` ("Agents home: layout, focus and motion matrix — <date>"); fix real defects.
- [ ] Step 3: mutation checks from the spec's list not yet covered (no step bar on a terminal session, drawer and sheet closing on selection); keep the CI budget (≤120 s added, `phone-zoom` <60 s).
- [ ] Step 4: docs: `docs/terminal-workspace.md` (Sessions, session screen, phones), `docs/roadmap.md` (Program 2 next), `README.md` sentence, `handoff.md` (PR numbers, rulings R1–R22, what remains). Commit "Verify the Agents home across screens and themes".

**PR H validation, review and merge:** the common steps.

---

## Program 2 — not part of this plan

Structured chat gets its own spec and plan after PR H merges: the ACP runner inside tmux; the confirmed, audited install of a pinned adapter for Claude Code (then Codex); the chat view (user messages, plan checklist, tool and edit cards with diffs and +/− counts, the permission decision card with **Allow once** / **Deny** and A/D keys, "Approvals are logged"); command approval and sign-in cards in Needs you with inline actions; exact attention states and step progress (R10, R11); `yolo-permission` and `adapter-install` audit events; machine-only transcripts fetched on demand; the phone decision card pinned above the composer; Chat enabled in the Chat | Terminal switch; notifications outside the page (R9).

## Self-review

- **Spec coverage:** browse and branches (A1–A4); registry, defaults, project id, API, migration with starter prompts, redirects, runs in Activity (B1–B5); catalogue, `agents-v2`, Effort, Base (C1–C4); tokens, fonts, logo, tabs, bell, redirects, Home, rail, Add project (D1–D6); launch sentence, starter prompt chip, several projects (E1–E4); YOLO setting, machine switch, flags, gates, dialog, badges, docs (F1–F5); attention, seen, queue, bell, title (G1–G3); Sessions, session screen, context panel, Chat | Terminal, tablet and phone, matrix and axe (H1–H4). Program 2 items are listed and excluded.
- **Type consistency:** `FolderProject` (B1) is the web type from B3 used in D3–E3; `BrowseResult`/`BranchesResult` (A1) flow to `folderBrowser.ts` and `launchBar.ts`; `Capabilities.Yolo` (C1) is set in F2 and read in F3; `RunLaunch` (E3) carries the C1 fields.
- **Review Focus:** each of the five lines names its test and owning task.
- **Proportion:** 35 tasks over eight pull requests, signatures and test names only; no bodies.
