# Skills: Active, Archived, Deleted — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A skill in reet.sh is Active (on every machine), Archived (kept in reet, off every machine) or Deleted. A
plain reconcile loop makes machines match, and the Skills page prunes in place.

**Architecture:**
- **Store.** It gains skill states, a removal list, a desired state with a 10-second grace period, and per-machine
  reports. Every skill is a reet bundle: public ones are fetched by the controller, others are captured from a
  machine.
- **Agent.** On each poll it receives the desired state, installs or backs up and removes in the four managed home
  folders, and reports skill by skill.
- **Web.** The page and Sync page are rebuilt on one `/api/v2/skills` API.
- **Old engine.** Plan, preview, approve, targets and Advanced sync are deleted last, once nothing calls them.

**Tech Stack:** Go 1.27 (`net/http`, SQLite via `database/sql`), Svelte 5 + TypeScript (Vite), node unit tests
(`*.test.mjs`), Playwright e2e scripts (`web/tests/*.e2e.mjs`).

**Spec:** `docs/superpowers/specs/2026-11-18-skills-active-archive-design.md`. Read it first; this plan argues from
it and cites its sections in *italics*.

## Global Constraints

- **The rule.** reet.sh is the source of truth, and every connected machine matches it. A skill is Active or
  Archived, or it is Deleted.
- **Active** = every enrolled, unpaused machine with allow-apply on, in each managed folder that exists there:
  `~/.agents/skills`, `~/.claude/skills`, `~/.cursor/skills`, `~/.config/opencode/skills`.
- **Never touched:** project folders, `.codex/skills`, and a custom `CLAUDE_CONFIG_DIR` or `XDG_CONFIG_HOME`.
- **Timings:**
  - Grace period before machines act: **10 seconds**.
  - Toast with Undo: **about 8 seconds**.
  - Machine backups kept **30 days**.
  - A deleted name is forgotten once every unpaused machine has confirmed, or after **30 days**.
- **Limits.**
  - Per skill: 8 MiB and 256 files (`internal/skillbundle/manifest.go:25-26`).
  - Per account: 1,024 bundles, 4,096 revisions, 1 GiB (`store_private_skills.go:18-22`).
- **Protocol header:** `X-Reet-Skills-Protocol: 2`.
- **Never removing the only copy.** Archive copies the files into reet first. A failed copy keeps the skill on its
  machines. Delete asks once when reet has no copy.
- **Every skill is a reet.sh skill.** The original link or machine is a reference only, never an install source or
  part of a prompt. Every skill is private until published.
- **No skill-sync job is planned, previewed or approved anywhere.** `sync_jobs` remains only for binary updates.
- **UI rules:**
  - The app's own tokens, 14 px type floor and 44 px touch targets, in light and dark, at 1440, 1024 and 390 px.
  - Copy follows `docs/voice-and-copy.md`.
  - Pruning never switches tab, scrolls, or navigates.
- **Commits.** Plain-sentence subjects as in `git log` (e.g. *Make Home's empty state one line*), no `feat:`
  prefixes, ending with the session's attribution trailers. No model identifiers anywhere in the repository.
- **Checks before every commit:**
  - Go changes: `go vet ./...` and `go test -race ./internal/<pkg>/...`.
  - Web changes: `npm --prefix web run check`.
  - Last task: the full `go test -race ./...`.

## Review Focus

1. **An agent killed between backup and install.** It leaves a `.reet-private-*` staging folder and the skill
   missing. The next cycle must clean the staging folder and install the skill. Test: Task 7,
   `TestReconcileRecoversFromInterruptedInstall`.
2. **A folder whose name differs only in case** (`SEO-Audit` beside Active `seo-audit`). It is the same skill: never
   installed twice, never reported unknown. Test: Task 7, `TestReconcileMatchesNamesIgnoringCase`.
3. **A second cycle with the same desired version** makes no fetches and no writes. Test: Task 7,
   `TestReconcileIsIdempotent`.
4. **A bundle download that fails half-way.** That skill is reported `failed`, its staging is removed, and every
   other skill still installs. Test: Task 7, `TestReconcileContinuesPastFailedDownload`.
5. **The controller restarting inside the grace period.** The change is still held until 10 seconds after it was
   made, because the time is stored, not kept in memory. Test: Task 2, `TestDesiredHoldsChangeAcrossReopen`.

---

## File map

**New, Go**
| File | Responsibility |
| --- | --- |
| `internal/store/store_skill_states.go` | Statuses, removal list, Archive/Restore/Delete/Undo, upgrade migration `skill-states-v1`, upgrade notice |
| `internal/store/store_skill_desired.go` | `DesiredSkillsFor`: the desired state, its version and the grace period |
| `internal/store/store_skill_reports.go` | Per-machine reports, health, *New on a machine*, activity, forgetting deleted names |
| `internal/store/store_skill_content.go` | Copy needs, copy problems, storing fetched or captured content, *Use this version* |
| `internal/skillfetch/fetch.go` | Fetch a public skill's folder from GitHub or a well-known host into checked files |
| `internal/api/skills_v2.go` | The Skills page API |
| `internal/api/skill_reconcile.go` | Desired state on poll, report endpoint, the copy worker |
| `internal/scan/reconcile.go` | The agent's reconcile pass |
| `internal/scan/backups.go` | Dated backups, 30-day pruning, list and restore |

**New, web**
| File | Responsibility |
| --- | --- |
| `web/src/lib/skills/skillsPage.ts` | Pure helpers: health lines, toasts, focus after removal, prompts, sync sentence |
| `web/src/lib/skills/SkillsPage.svelte` | The page: header, add, banner, tabs, filters, list, bulk bar, toast |
| `web/src/lib/skills/SkillRow.svelte` | One row, plus its opened detail |
| `web/src/lib/skills/NewOnMachineDialog.svelte` | Keep / Archive review |
| `web/src/lib/skills/UpdateFromOriginalDialog.svelte` | Changed files, then confirm |
| `web/src/lib/sync/SyncPage.svelte` | One sentence, machine rows with Pause, activity |

**Modified:** `internal/store/store_private_skills.go` (limits), `internal/api/sync_control.go` (poll),
`internal/api/api.go` (scan import, add by link), `internal/scan/agent.go` (cycle, header), `internal/scan/restore.go`,
`internal/setup/adapters.go`, `internal/store/store_lists.go`, `internal/api/terminal_skills_install.go`,
`internal/api/mcp_server.go`, `mcp/src/server.ts`, `web/src/App.svelte`, `web/src/lib/api.ts`,
`web/src/lib/install.ts`, `web/src/lib/Community.svelte`, `web/src/lib/ListView.svelte`, `web/src/lib/terminal/skills.ts`,
`web/src/lib/terminal/SkillInstallPanel.svelte`, `web/src/lib/home/whatWaits.ts`, `web/package.json` (test script).

**Deleted:** the list in the spec's *What is deleted*; Tasks 16 and 17.

---

## Phase 1: the store

### Task 1: Skill states, the removal list and the upgrade

**Files:**
- Create: `internal/store/store_skill_states.go`, `internal/store/store_skill_states_test.go`
- Modify: `internal/store/store.go` (call `migrateSkillStates` after `migrateCanonicalLibrary`, `store.go:534`),
  `internal/store/store_canonical_library.go` (status constants, `validLibraryStatus`, the shelf-flag triggers
  at 145-161)

**Interfaces:**
- Produces:
  ```go
  const SkillActive, SkillArchived = "active", "archived"
  var ErrSkillNameTaken = errors.New("a skill with that name is already in your library")
  var ErrSkillNoCopy    = errors.New("reet has no copy of this skill")
  func (s *Store) SetSkillStatusFor(uid int64, ids []string, status string, at time.Time) error
  func (s *Store) DeleteSkillsFor(uid int64, ids []string, noCopyConfirmed bool, at time.Time) error
  func (s *Store) UndeleteSkillsFor(uid int64, ids []string) error
  type SkillRemoval struct{ Name, Reason string; CreatedAt int64 } // Reason "archived" | "deleted"
  func (s *Store) SkillRemovalsFor(uid int64) ([]SkillRemoval, error)
  const SkillUndoWindow = time.Minute
  func (s *Store) PurgeDeletedSkillsFor(uid int64, now time.Time) error // items deleted more than SkillUndoWindow ago: their hosted skill, bundle and revisions go
  type UpgradeNotice struct{ Parked, Removed int }
  func (s *Store) SkillUpgradeNoticeFor(uid int64) (*UpgradeNotice, error) // nil once dismissed
  func (s *Store) DismissSkillUpgradeNoticeFor(uid int64) error
  func (s *Store) RestoreParkedSkillsFor(uid int64, at time.Time) error
  ```
  - IDs are `library_items.id`.
  - `library_items` gains `prev_status`, `status_at` (ms), `origin`, `upgraded_from` (`parked` | `removed` | `''`)
    and `copy_problem`, and its status CHECK becomes `('active','archived')`.
  - New tables: `skill_removals(user_id, name, reason, created_at, PK(user_id,name))`,
    `skill_removal_confirms(user_id, name, agent_id, at, PK(user_id,name,agent_id))` and
    `skill_notices(user_id PK, parked, removed, dismissed)`.

- [ ] **Step 1: Write the failing tests** in `store_skill_states_test.go`, using `newStore(t)` and a store built by
      `Upsert` with the old flags.
  - `TestSkillStatesUpgradeMapsStages`: In use and To test become `active`; On the shelf becomes `archived` with
    `upgraded_from='parked'`; `disabled` becomes `archived`. The notice is `{Parked: 1, Removed: 0}` for one Parked
    skill.
  - `TestSkillStatesUpgradeTurnsIgnoredIntoArchived`: an `ignored` row with a library item becomes `archived`, and
    one without an item becomes a new `archived` item with that name and no content. Both have
    `upgraded_from='removed'`, and the `ignored` table is empty.
  - `TestSkillStatesUpgradeFoldsDuplicateNames`: a written skill `askme` (`to_test`) and a machine copy `askme`
    (`in_use`) fold into the written one, keeping the machine copy's tags, notes and like. Two linked items fold
    into the In use one.
  - `TestSkillStatesArchiveRestoreDelete`:
    - Archive sets `archived`, `prev_status='active'` and `status_at=at`, and adds `skill_removals{reason:"archived"}`.
    - Restore removes that row.
    - Delete sets `deleted_at` and the reason `deleted`.
    - Undelete clears both.
  - `TestSkillStatesDeleteWithoutCopyNeedsConfirmation`: with no `revision_id`, `DeleteSkillsFor(…, false, …)`
    returns `ErrSkillNoCopy`, and `true` succeeds.
  - `TestSkillStatesNamesAreUnique`: making a second item named `seo-audit` returns `ErrSkillNameTaken`.
  - `TestSkillStatesDeletedWrittenSkillPurges`:
    - Deleting a published written skill makes its public page unavailable at once: `HostedSkillsFor` no longer
      lists it as published.
    - `UndeleteSkillsFor` inside the minute brings it back.
    - `PurgeDeletedSkillsFor(now+61s)` removes the hosted skill, bundle and revisions, after which undelete fails.
  - `TestSkillStatesUpgradeKeepsBinaryUpdateJobs`: a `sync_jobs` row of kind `update` survives the upgrade.
- [ ] **Step 2: Run them to see them fail.** Run `go test -race ./internal/store -run 'TestSkillStates'`. Expected:
      FAIL, undefined `SetSkillStatusFor` and the others.
- [ ] **Step 3: Implement the migration `skill-states-v1`** in `migrateSkillStates()`, recorded in
      `library_migrations` like `identity-v1` (`store_canonical_library.go:85-117`).
  - Rebuild `library_items` with the new columns and CHECK, keeping every row and the `library_aliases` foreign
    key.
  - Map the statuses, then convert `ignored`.
  - Fold duplicates in the spec's order: written, then imported, then machine copy; among equals, In use, then
    newest.
  - Write `skill_notices`.
  - Drop the triggers that mirror `shelf.used` / `shelf.bookmarked` into status. The shelf columns stay until Task
    16.
- [ ] **Step 4: Implement the functions** in `store_skill_states.go`.
  - Each write runs in one `withTx`.
  - Names are compared lower-cased.
- [ ] **Step 5: Run the tests.** Run `go test -race ./internal/store -run 'TestSkillStates'`. Expected: PASS. Then
      `go test -race ./internal/store/...` still passes.
- [ ] **Step 6: Commit:** *Give library skills Active and Archived states with a removal list and an upgrade*.

### Task 2: The desired state

**Files:**
- Create: `internal/store/store_skill_desired.go`, `internal/store/store_skill_desired_test.go`

**Interfaces:**
- Consumes: Task 1's statuses, `status_at`, `prev_status` and `skill_removals`.
- Produces:
  ```go
  const SkillGrace = 10 * time.Second
  type DesiredSkill   struct{ Name, Item, Revision, Bundle, Digest string }
  type DesiredRemoval struct{ Name, Reason string }
  type DesiredState   struct{ Version string; Active []DesiredSkill; Remove []DesiredRemoval }
  func (s *Store) DesiredSkillsFor(uid int64, now time.Time) (DesiredState, error)
  ```
  - The state is the same for every machine.
  - An item whose `status_at` is within `SkillGrace` of `now` counts as `prev_status`.
  - An Active item with no `revision_id` is left out of both lists: it is held.
  - An Archived item without content is left out of `Remove` until it has content, or until it has a
    `copy_problem`, in which case it stays out.
  - `Version` is the hex SHA-256 of the sorted lists.

- [ ] **Step 1: Write the failing tests.**
  - `TestDesiredListsActiveWithContent`: three Active items with revisions, sorted by name, each with its bundle,
    revision and digest.
  - `TestDesiredHoldsChangeForGrace`: Archive at `t0`; at `t0+9s` the item is still in `Active`; at `t0+10s` it is
    in `Remove` with reason `archived`.
  - `TestDesiredHoldsChangeAcrossReopen`: Archive, close the store, reopen it from the same directory with `Open`.
    At `t0+5s` it is still Active (Review Focus 5).
  - `TestDesiredLeavesOutItemsWithoutContent`: an Active item without a revision is in neither list.
  - `TestDesiredVersionChangesOnlyWithContent`: the same inputs give the same `Version`; a new revision changes it.
  - `TestDesiredIncludesDeletedUntilForgotten`: a deleted name is in `Remove` with reason `deleted`.
- [ ] **Step 2: Run them to see them fail.** Run `go test -race ./internal/store -run 'TestDesired'`. Expected: FAIL.
- [ ] **Step 3: Implement `DesiredSkillsFor`** from `library_items`, its head revision (as
      `store_canonical_selections.go:140-156` reads it), and `skill_removals`.
- [ ] **Step 4: Run the tests.** Run `go test -race ./internal/store -run 'TestDesired'`. Expected: PASS.
- [ ] **Step 5: Commit:** *Compute the skills every machine should have, holding changes for ten seconds*.

### Task 3: Machine reports, health and New on a machine

**Files:**
- Create: `internal/store/store_skill_reports.go`, `internal/store/store_skill_reports_test.go`
- Modify: `internal/store/store_sync_control.go` (add column `sync_agents.skills_protocol INTEGER NOT NULL
  DEFAULT 0`, and `SetAgentSkillsProtocolFor(uid int64, agentID string, protocol int) error`)

**Interfaces:**
- Consumes: Task 1 and Task 2.
- Produces:
  ```go
  type SkillReportEntry struct{ Folder, Name, State, Digest, Detail, Backup string }
  // State: installed | removed | failed | unknown | edited
  type SkillReport struct{ Version, Blocked string; Entries []SkillReportEntry } // Blocked: "" | "allow-apply"
  func (s *Store) RecordSkillReportFor(uid int64, agentID string, rep SkillReport, now time.Time) error
  type MachineSkillState struct{ Host, AgentID, State, Detail string }
  type SkillHealth struct {
      State   string // ok | installing | removing | failed | offline | blocked | copying | copy-failed | getting | fetch-failed | needs-update | off
      Pending, Total int
      Hosts   []string
      Reason  string
      Machines []MachineSkillState
  }
  func (s *Store) SkillHealthFor(uid int64, now time.Time) (map[string]SkillHealth, error) // key: item id
  type NewSkillCopy struct{ AgentID, Host, Folder, Digest string; SeenAt int64 }
  type NewSkill struct{ Name string; Copies []NewSkillCopy; DigestsDiffer bool }
  func (s *Store) NewOnMachinesFor(uid int64) ([]NewSkill, error)
  type SkillActivity struct{ At int64; Host, Name, Action, Detail string }
  func (s *Store) SkillActivityFor(uid int64, limit int) ([]SkillActivity, error)
  ```
  - Tables: `machine_skills(user_id, agent_id, folder, name, state, digest, detail, backup, version, at,
    PK(user_id,agent_id,folder,name))` and `skill_activity(user_id, at, agent_id, name, action, detail)`, keeping
    the last 500 rows per user.
  - JSON field names are lowerCamel, matching Task 9's TypeScript types (`pending`, `total`, `hosts`, `reason`,
    `machines[].host|state|detail`).
  - Machines counted in `Total` are enrolled, unpaused agents with `skills_protocol = 2`.
  - A protocol-2 agent unseen for more than 5 minutes is `offline` for skills still pending on it.
  - Agents below protocol 2 make health `needs-update` only on the Sync page, never on a skill row.

- [ ] **Step 1: Write the failing tests.**
  - `TestReportHealthAllInstalled`: two machines report `installed` at the desired digest, giving `ok`, `Total 2`.
  - `TestReportHealthInstallingAndFailed`: one `installed` and one missing gives `installing`, `Pending 1`. A
    `failed` with Detail `no write access` gives `failed`, the host, and that Reason.
  - `TestReportHealthRemovingAndOffline`: an archived item still `installed` on a machine last seen 10 minutes ago
    gives `offline` with that host.
  - `TestReportBlockedByAllowApply`: `Blocked:"allow-apply"` makes every pending item `blocked` with that host.
  - `TestReportUnknownBecomesNew`:
    - `unknown` `cq` on two machines with different digests gives one `NewSkill` with `DigestsDiffer` true.
    - An `unknown` name that is Archived is not new.
    - An `unknown` name that is Active is not new.
  - `TestReportConfirmsDeletionAndForgets`:
    - Once both unpaused machines report the deleted name gone, `SkillRemovalsFor` no longer lists it.
    - Any name older than 30 days is forgotten as well.
    - A later `unknown` of that name is new.
  - `TestReportArchivedReappearingIsRemovedAgain`: after `removed`, a later `installed` of an archived name records
    the activity *removed again*.
  - `TestReportEditedRecordsBackup`: an `edited` entry keeps `Backup` and writes the activity *edited on {host};
    reet's copy put back*.
- [ ] **Step 2: Run them to see them fail.** Run `go test -race ./internal/store -run 'TestReport'`. Expected: FAIL.
- [ ] **Step 3: Implement.** `RecordSkillReportFor` replaces that agent's rows in one transaction, appends activity
      only for changes, and confirms removals.
- [ ] **Step 4: Run the tests.** Run `go test -race ./internal/store -run 'TestReport'`. Expected: PASS.
- [ ] **Step 5: Commit:** *Record what each machine did with each skill and read health from it*.

## Phase 2: content

### Task 4: Fetching a public skill

**Files:**
- Create: `internal/skillfetch/fetch.go`, `internal/skillfetch/fetch_test.go`

**Interfaces:**
- Consumes: `ref.Ref` (`internal/ref/ref.go`: `ID, Source, Slug, SourceType, URL`), `skillimport.Select`
  (`internal/skillimport/selection.go:25`), `skillbundle.ContentFile`.
- Produces:
  ```go
  type Result struct{ Files []skillbundle.ContentFile; Manifest skillbundle.Manifest; Origin string }
  var ErrNotFound, ErrTooLarge = errors.New("skill not found in its source"), errors.New("skill is over 8 MiB or 256 files")
  type Fetcher struct{ Client *http.Client; GitHub string /* default https://codeload.github.com */ }
  func (f Fetcher) Fetch(ctx context.Context, r ref.Ref) (Result, error)
  ```
  - **GitHub.** Download `{GitHub}/{owner}/{repo}/tar.gz/HEAD`. Choose the folder holding a `SKILL.md` whose folder
    name or front-matter `name` equals `r.Slug`, preferring `skills/<slug>`. Read the archive streaming, stopping
    at 8 MiB or 256 files in that folder. `Origin` is `https://github.com/{owner}/{repo}/tree/HEAD/{path}`.
  - **Well-known.** Read `https://{host}/.well-known/agent-skills/index.json`, then each listed file, the format
    reet serves in `internal/api/hosted.go:25-26`. `Origin` is the skill's page URL.
  - Files pass through `skillimport.Select(files, nil)`.

- [ ] **Step 1: Write the failing tests** against an `httptest` server serving a built `.tar.gz`.
  - `TestFetchGitHubFindsSkillFolder`: a repo with `skills/pdf/SKILL.md` and `README.md` gives only the pdf folder's
    files, with `Origin` `https://github.com/o/r/tree/HEAD/skills/pdf`.
  - `TestFetchGitHubMatchesFrontMatterName`: folder `svg-logo-designer` with `name: SVG Logo Designer` and slug
    `svg-logo-designer` is found.
  - `TestFetchRefusesOverLimits`: 257 files or 9 MiB gives `ErrTooLarge`, and the stream is not read to the end.
  - `TestFetchRefusesCredentials`: a `.env` holding a key is blocked by `Select`.
  - `TestFetchNotFound`: a 404 or no matching folder gives `ErrNotFound`.
  - `TestFetchWellKnown`: an index plus two files gives both files.
- [ ] **Step 2: Run them to see them fail.** Run `go test -race ./internal/skillfetch`. Expected: FAIL (no package).
- [ ] **Step 3: Implement `Fetcher.Fetch`** with `archive/tar` and `compress/gzip`, and an `http.Client` with
      timeouts and no redirects off GitHub's hosts.
- [ ] **Step 4: Run the tests.** Run `go test -race ./internal/skillfetch`. Expected: PASS.
- [ ] **Step 5: Commit:** *Fetch a public skill's folder into checked files*.

### Task 5: Every skill has a reet copy

**Files:**
- Create: `internal/store/store_skill_content.go`, `internal/store/store_skill_content_test.go`,
  `internal/api/skill_copies.go`, `internal/api/skill_copies_test.go`
- Modify: `internal/store/store_private_skills.go:18-22` (limits to 1024, 4096, `1 << 30`), `internal/api/api.go`
  (adding by link calls the copy worker)

**Interfaces:**
- Consumes: `SaveLibraryContentFor(uid, id, PrivateSkillWrite)` (`store_canonical_library.go:255`),
  `PrivateSkillOrigin{Kind, Reference, DerivedFrom}`, the existing machine capture (`captureMachineLibraryContent`,
  `store_library_capture.go:32`; `machine_import_requests`), and Task 4's `skillfetch.Fetcher`.
- Produces:
  ```go
  type SkillCopyNeed struct{ Item, Name, Kind, AgentID, Folder string } // Kind: "fetch" | "capture"
  func (s *Store) SkillsNeedingCopyFor(uid int64) ([]SkillCopyNeed, error)
  func (s *Store) SetSkillContentFor(uid int64, item string, files []skillbundle.ContentFile, origin PrivateSkillOrigin) (PrivateSkillResult, error)
  func (s *Store) SetSkillCopyProblemFor(uid int64, item, problem string) error
  func (s *Store) RequestSkillCaptureFor(uid int64, item, agentID, path string) error // path must sit inside a managed folder or its .reet-backups
  func (s *Store) SkillEditedSinceImportFor(uid int64, item string) (bool, error)
  // api
  func (s *server) kickSkillCopies(uid int64)                        // single-flight per user; fetches then captures
  func (s *server) originalChanges(ctx context.Context, uid int64, item string) (skillbundle.Diff, string, bool, error) // diff, fetched digest, edited
  ```
  - Origin kinds: `original` (fetched), `capture` (from a machine), `edit` (Write a skill), `use-version`.
  - `library_items.origin` holds the reference: the link, or `machine:{host}:{folder}`.

- [ ] **Step 1: Write the failing tests.**
  - `TestCopyNeedsFetchForImported`: an imported Active item with no revision, ref type github, gives `Kind:"fetch"`.
  - `TestCopyNeedsCaptureForMachineOnly`: a `local/x` item reported `installed` on Elitebook gives `Kind:"capture"`
    with that agent and folder.
  - `TestArchiveWaitsForCopy`: archiving an item with no revision leaves its name out of `DesiredSkillsFor().Remove`.
    After `SetSkillContentFor`, the name appears there.
  - `TestCopyProblemKeepsSkillOnMachines`: after `SetSkillCopyProblemFor("too large")`, the item is archived and
    not in `Remove`, and its health is `copy-failed` with that Reason.
  - `TestStorageLimitsHoldALibrary`: 139 bundles of 4 KiB save without `ErrPrivateSkillCapacity`.
  - `TestCaptureRefusesPathsOutsideManagedFolders`: `~/.ssh` is refused.
  - `TestEditedSinceImport`: an `original` revision, then an `edit` revision, gives true.
  - api `TestAddByLinkFetchesContent`: `POST /api/skills` with a stub fetcher leads to an item with a revision and
    origin `https://github.com/…`.
  - api `TestAddByLinkFetchFailureShowsProblem`: the stub returns `ErrNotFound`, health is `fetch-failed`, and
    `POST /api/v2/skills/retry` tries again.
  - api `TestOriginalChangesShowsDiffAndEdited`: `GET /api/v2/skills/{item}/original` returns changed paths and
    `edited:true` after an edit. `POST` with a stale digest gives 409.
- [ ] **Step 2: Run them to see them fail.** Run `go test -race ./internal/store -run 'TestCopy|TestArchiveWaits|TestStorageLimits|TestCapture|TestEdited'`
      and `go test -race ./internal/api -run 'TestAddByLink|TestOriginal'`. Expected: FAIL.
- [ ] **Step 3: Implement the store functions** and raise the limits.
- [ ] **Step 4: Implement `kickSkillCopies`** and the `original` endpoints. `kickSkillCopies` also calls
      `PurgeDeletedSkillsFor`.
  - Adding by link and **Retry** both call `kickSkillCopies`.
  - The `server` gets a `skillfetch.Fetcher` through an `Option`, so tests pass a stub.
- [ ] **Step 5: Run the tests.** Run the Step 2 commands. Expected: PASS.
- [ ] **Step 6: Commit:** *Keep a reet copy of every skill, fetched or captured, before anything is removed*.

## Phase 3: machines

### Task 6: Dated backups and restore

**Files:**
- Create: `internal/scan/backups.go`, `internal/scan/backups_test.go`
- Modify: `internal/scan/restore.go` (rework `reet agent restore` to `reet agent restore --list` and
  `reet agent restore <name> [--date YYYY-MM-DD]`)

**Interfaces:**
- Produces:
  ```go
  const backupRetention = 30 * 24 * time.Hour
  func backupSkill(folder, name string, now time.Time) (string, error) // → <parent(folder)>/.reet-backups/<YYYY-MM-DD>/<base(parent)>-<name>[-2…]
  func pruneBackups(home string, now time.Time) error
  type backupEntry struct{ Name, Folder, Path, Date string }
  func listBackups(home string) ([]backupEntry, error)
  func restoreBackup(home, name, date string) (string, error) // returns the restored path; refuses when the target exists
  ```
  A symlink is moved as a symlink, as `backupSkillEntry` does today (`sync.go:1409-1435`).

- [ ] **Step 1: Write the failing tests.**
  - `TestBackupMovesFolderAndSymlink`: the folder moves under `.reet-backups/2026-11-18/`, and a symlink moves as a
    link with its target untouched.
  - `TestBackupNameClash`: two backups the same day get `-2`.
  - `TestPruneBackupsKeeps30Days`: entries from 31 days ago are removed and ones from 29 days ago stay.
  - `TestRestoreBackupByNameAndDate`: restores into the original folder, and refuses if the skill exists there.
- [ ] **Step 2: Run them to see them fail.** Run `go test -race ./internal/scan -run 'TestBackup|TestPrune|TestRestoreBackup'`.
      Expected: FAIL.
- [ ] **Step 3: Implement** in `backups.go`, and rewire `restore.go`.
- [ ] **Step 4: Run the tests.** Run the same command. Expected: PASS.
- [ ] **Step 5: Commit:** *Back up removed skills by date, prune after 30 days and restore by name*.

### Task 7: The agent's reconcile pass

**Files:**
- Create: `internal/scan/reconcile.go`, `internal/scan/reconcile_test.go`
- Modify: `internal/scan/agent.go`:
  - `agentRequestWith` (157-187) adds the header `X-Reet-Skills-Protocol: 2`.
  - `agentCycle` (582-781) handles `reply.Desired` and sends `skillsVersion`.

**Interfaces:**
- Consumes: Task 2's `store.DesiredState`, Task 3's `store.SkillReport`, Task 6's `backupSkill`/`pruneBackups`,
  `privateSkillFiles` (`private_sync.go:33`, its digest check), `installPrivateSkill` (`private_sync.go:80`),
  `linkSharedInstall` (`sync.go:1386`).
- Produces:
  ```go
  var managedFolders = []struct{ Dir, Agent string }{{".agents/skills","agents"},{".claude/skills","claude"},{".cursor/skills","cursor"},{".config/opencode/skills","opencode"}}
  type bundleFetcher func(ctx context.Context, s store.DesiredSkill) ([]skillbundle.ContentFile, error)
  func reconcileSkills(ctx context.Context, home string, d store.DesiredState, installed map[string]string, fetch bundleFetcher, now time.Time) (store.SkillReport, map[string]string)
  // installed: "<folder>/<name>" → digest reet last installed; persisted at <config>.installed
  ```
  - Order: `.agents/skills` first; Cursor and OpenCode link to it as today; then `.claude/skills`.
  - **Unknown:** a folder that is neither Active nor removed is left alone and reported `unknown`.
  - **Edited:** a present copy whose digest differs from the desired one and from `installed[...]` is `edited`. It
    is backed up and replaced, with `Backup` set.
  - **Matching:** names are matched ignoring case.
  - **Cleanup:** stale `.reet-private-*` staging folders are removed first.

- [ ] **Step 1: Write the failing tests** in a temporary home, with a stub `bundleFetcher` that counts calls.
  - `TestReconcileInstallsMissing`: two Active skills land in `.agents/skills`, and are linked from `.cursor/skills`
    when that folder exists. Both entries are `installed`.
  - `TestReconcileRefusesBadDigest`: the fetcher returns files whose digest differs, giving `failed`, nothing
    written.
  - `TestReconcileReplacesEditedCopy`: a changed `SKILL.md` gives `edited` with `Backup` set, and reet's files are in
    place.
  - `TestReconcileRemovesWhatReetNeverInstalled`: an archived name present as a folder, a symlink, and an edited
    copy is backed up and reported `removed` each time.
  - `TestReconcileLeavesUnknownAlone`: `cq` is untouched and reported `unknown` with its digest.
  - `TestReconcileRecoversFromInterruptedInstall` (Review Focus 1).
  - `TestReconcileMatchesNamesIgnoringCase` (Review Focus 2).
  - `TestReconcileIsIdempotent`: a second run with the returned `installed` map makes 0 fetches and no file changes
    (Review Focus 3).
  - `TestReconcileContinuesPastFailedDownload`: the fetcher errors for `b` only; `a` and `c` are installed, and no
    staging folder is left (Review Focus 4).
  - `TestReconcileSkipsUnmanagedFolders`: `.codex/skills` and a project folder are never read or written.
  - `TestAgentCycleBlockedWithoutAllowApply`: with allow-apply off, the cycle posts a report with
    `Blocked:"allow-apply"` and writes nothing.
- [ ] **Step 2: Run them to see them fail.** Run `go test -race ./internal/scan -run 'TestReconcile|TestAgentCycleBlocked'`.
      Expected: FAIL.
- [ ] **Step 3: Implement `reconcileSkills`.**
- [ ] **Step 4: Wire it into `agentCycle`.**
  - After the poll, when `reply.Agent.Paused` is false and `reply.Desired != nil`: read allow-apply fresh, as at
    695-704.
  - Run `reconcileSkills`, write `<config>.installed`, `POST /api/v2/sync/skills/report`, then `pruneBackups`.
  - The poll body becomes `{"skillsVersion": "<last reported version>"}`.
- [ ] **Step 5: Run the tests.** Run the same command, then `go test -race ./internal/scan/...`. Expected: PASS.
- [ ] **Step 6: Commit:** *Make each machine's skill folders match reet on every poll*.

### Task 8: The controller's side of the loop, and an end-to-end test

**Files:**
- Create: `internal/api/skill_reconcile.go`, `internal/api/skill_reconcile_test.go`,
  `internal/scan/reconcile_e2e_test.go`
- Modify: `internal/api/sync_control.go`:
  - `pollSyncAgent` (347-369) changes as in Step 3.
  - `agentIdentity` (286-296) reads the new header.
  - The route for `POST /api/v2/sync/skills/report` is added.

**Interfaces:**
- Consumes: Tasks 2, 3 and 5; `ClaimSyncJob` (`store_sync_control.go:685`).
- Produces:
  ```go
  func (s *Store) ClaimUpdateJob(uid int64, a SyncAgent) (*SyncJob, error) // only kind="update"
  // poll reply: {"agent": SyncAgent, "job": *SyncJob, "desired": *DesiredState /* omitted when version matches */}
  ```

- [ ] **Step 1: Write the failing tests.**
  - `TestPollSendsDesiredToProtocol2`: with the header, the reply has `desired`; with `skillsVersion` equal to it,
    the reply omits `desired`.
  - `TestPollGivesProtocol2OnlyUpdateJobs`: a queued old-engine job is not claimed; an `update` job is.
  - `TestPollWithoutProtocolGetsNoSkillWork`: no `desired`, and the agent shows as `needs-update`.
  - `TestReportEndpointRecords`: a POSTed report changes `SkillHealthFor`, and a report for another user's agent
    gives 403.
  - `TestPollKicksCopies`: an Active item without content triggers one `kickSkillCopies`.
  - `TestSkillsEndToEnd` in `internal/scan/reconcile_e2e_test.go`. It uses a real `api.New` server and store, and
    runs `RunAgent([]string{"run","--once",…})` against a temporary home with allow-apply on. Steps:
    1. Add a skill with a stub fetcher; one cycle installs it on disk.
    2. Archive it; after the grace period, one cycle removes it and leaves a backup.
    3. Restore it; it is installed again.
    4. Put `cq` in `.agents/skills` by hand, giving one `NewSkill`.
    5. Keep it; a second home gets it.
    6. Delete a skill; it is gone from disk, and its name is forgotten after the confirmation.
- [ ] **Step 2: Run them to see them fail.** Run `go test -race ./internal/api -run 'TestPoll|TestReportEndpoint'` and
      `go test -race ./internal/scan -run TestSkillsEndToEnd`. Expected: FAIL.
- [ ] **Step 3: Implement the protocol-2 poll path.**
  - A protocol-2 agent skips `AdvanceMachineSkillSyncs` and `MaintainSync`, and claims only update jobs.
  - Record `SetAgentSkillsProtocolFor`, add `desired`, and kick copies.
  - Agents below protocol 2 keep today's path until Task 16.
- [ ] **Step 4: Run the tests.** Run the Step 2 commands. Expected: PASS.
- [ ] **Step 5: Commit:** *Send machines their skills on each poll and take their reports*.

## Phase 4: the pages

### Task 9: The Skills page API

**Files:**
- Create: `internal/api/skills_v2.go`, `internal/api/skills_v2_test.go`
- Modify: `web/src/lib/api.ts` (types and client functions below)

**Interfaces:**
- Consumes: Tasks 1, 3 and 5.
- Produces (HTTP, writer role required for every POST):
  ```
  GET  /api/v2/skills?status=active|archived → SkillsPageData
  POST /api/v2/skills/status   {ids, status}            → SkillsPageData
  POST /api/v2/skills/delete   {ids, noCopyConfirmed}    → 409 {"code":"no-copy","names":[…]} when needed
  POST /api/v2/skills/undelete {ids}
  POST /api/v2/skills/retry    {id}
  POST /api/v2/skills/new      {names, action:"keep"|"archive", from?: agentId}
  POST /api/v2/skills/notice   {action:"dismiss"|"restore-parked"}
  POST /api/v2/skills/{item}/use-version {agentId}
  GET|POST /api/v2/skills/{item}/original                (Task 5)
  ```
  ```ts
  export type HealthState = 'ok'|'installing'|'removing'|'failed'|'offline'|'blocked'|'copying'|'copy-failed'|'getting'|'fetch-failed'|'needs-update'|'off'
  export type SkillHealth = { state: HealthState; pending: number; total: number; hosts: string[]; reason?: string; machines: { host: string; state: string; detail?: string }[] }
  export type SkillRow = { id: string; shelfId: string; name: string; description: string; status: 'active'|'archived'; originKind: 'written'|'imported'|'kept'; origin: string; yours: boolean; published: boolean; publicUrl?: string; hasCopy: boolean; liked: boolean; notes: string; tags: Tag[]; lists: ListRef[]; health: SkillHealth; upgradedFrom?: 'parked'|'removed'; editedOn?: { agentId: string; host: string } }
  export type NewSkill = { name: string; copies: { agentId: string; host: string; folder: string }[]; digestsDiffer: boolean }
  export type SkillsPageData = { skills: SkillRow[]; counts: { active: number; archived: number }; newOnMachines: NewSkill[]; machines: number; notice: { parked: number; removed: number } | null }
  export function loadSkillsPage(status: 'active'|'archived'): Promise<SkillsPageData>
  export function setSkillStatus(ids: string[], status: 'active'|'archived'): Promise<SkillsPageData>
  export function deleteSkills(ids: string[], noCopyConfirmed?: boolean): Promise<void>
  export function undeleteSkills(ids: string[]): Promise<void>
  export function retrySkill(id: string): Promise<void>
  export function decideNewSkills(names: string[], action: 'keep'|'archive', from?: string): Promise<void>
  export function skillNotice(action: 'dismiss'|'restore-parked'): Promise<void>
  export function useMachineVersion(id: string, agentId: string): Promise<void>
  ```
  Likes, notes, tags and lists keep their existing endpoints, keyed by `shelfId`.

- [ ] **Step 1: Write the failing tests:** `TestSkillsPageListsByStatus`, `TestSkillsPageBulkStatus`,
      `TestSkillsPageDeleteNeedsConfirmWithoutCopy`, `TestSkillsPageUndelete`, `TestSkillsPageNewKeepAndArchive`,
      `TestSkillsPageNoticeDismissAndRestoreParked`, `TestSkillsPageReadOnlyRefused` (a viewer's POST gives 403).
      Each asserts the response shape and the store's state afterwards.
- [ ] **Step 2: Run them to see them fail.** Run `go test -race ./internal/api -run TestSkillsPage`. Expected: FAIL.
- [ ] **Step 3: Implement the handlers** in `skills_v2.go`, registered beside the existing skill routes, and the
      client functions in `api.ts`.
- [ ] **Step 4: Run the tests.** Run the same command, then `npm --prefix web run check`. Expected: PASS.
- [ ] **Step 5: Commit:** *Serve the Skills page from one API keyed by library item*.

### Task 10: Page helpers

**Files:**
- Create: `web/src/lib/skills/skillsPage.ts`, `web/src/lib/skills/skillsPage.test.mjs`
- Modify: `web/package.json` (add `node src/lib/skills/skillsPage.test.mjs` to `test`), `web/src/lib/install.ts`
  (`skillBlurb` exported)

**Interfaces:**
- Consumes: Task 9's types.
- Produces:
  ```ts
  export function healthLine(h: SkillHealth): { text: string; tone: 'ok'|'work'|'bad'|'off'; retry: boolean }
  export function toastText(action: 'archived'|'deleted'|'restored'|'kept', names: string[], machines: number): string
  export function focusAfterRemoval(order: string[], removed: string[]): string | null
  export function deleteConfirm(rows: SkillRow[]): { title: string; body: string } | null
  export function copyPrompt(row: Pick<SkillRow,'name'|'description'>): string
  export function installCommandFor(row: SkillRow): string | null
  export function syncSentence(m: { failed: number; working: number }): string
  ```

- [ ] **Step 1: Write the failing tests** (node `assert`, in the style of `skillPrompt.test.mjs`), with these exact
      strings:
  - **`healthLine`:**
    - `{state:'ok',total:3}` → `On all 3 machines`, ok
    - `{state:'installing',pending:2,total:3}` → `Installing on 2 of 3`
    - `{state:'removing',pending:3}` → `Coming off 3 machines`
    - `{state:'removing',pending:1,hosts:['ThinkPad']}` → `Coming off ThinkPad`
    - `{state:'failed',hosts:['ThinkPad']}` → `Failed on ThinkPad`, bad, retry true
    - `{state:'offline',hosts:['studio']}` → `Removing from studio when it's back online`
    - `{state:'blocked',hosts:['ThinkPad']}` → `Can't write skills on ThinkPad`
    - `{state:'copying',hosts:['Elitebook']}` → `Copying into reet from Elitebook`
    - `{state:'copy-failed',reason:'over 8 MiB',hosts:['Elitebook']}` →
      `Couldn't copy into reet: over 8 MiB. It stays on Elitebook until you delete it.`
    - `{state:'getting',hosts:['Elitebook']}` → `Getting a copy from Elitebook`
    - `{state:'fetch-failed'}` → `Couldn't fetch the files`, retry true
    - `{state:'off'}` → `Off every machine`
  - **`toastText`:**
    - `('archived',['seo-bing'],3)` → `Archived seo-bing · coming off 3 machines`
    - `('archived',['a','b'],1)` → `Archived 2 skills · coming off 1 machine`
    - `('deleted',['x'],3)` → `Deleted x · coming off 3 machines`
    - `('restored',['x'],3)` → `Restored x · going back on 3 machines`
    - `('kept',['paseo-help'],3)` → `Kept paseo-help · going on every machine`
  - **`focusAfterRemoval`:**
    - `(['a','b','c'],['b'])` → `'c'`
    - `(['a','b','c'],['c'])` → `'b'`
    - `(['a','b'],['a','b'])` → `null`
  - **`deleteConfirm`:**
    - a `yours` row → title `Delete reet-babysit?`, body starting `You wrote this one.`
    - a row with `hasCopy:false` → body starting `reet has no copy of this skill.`
    - a plain row → `null`
  - **`copyPrompt`** `{name:'seo-audit',description:'Full SEO audit.'}` → `Use the seo-audit skill: Full SEO audit.`
  - **`installCommandFor`:** an unpublished row → `null`. A published row → a command containing its `publicUrl` and
    never its `origin`.
  - **`syncSentence`:**
    - `{failed:0,working:0}` → `Everything is in sync.`
    - `{failed:0,working:2}` → `Installing on 2 machines…`
    - `{failed:1,working:0}` → `1 machine needs a look.`
- [ ] **Step 2: Run them to see them fail.** Run `node web/src/lib/skills/skillsPage.test.mjs`. Expected: FAIL
      (module not found).
- [ ] **Step 3: Implement the helpers.**
- [ ] **Step 4: Run the tests.** Run `npm --prefix web run test`. Expected: PASS.
- [ ] **Step 5: Commit:** *Add the Skills page's wording and focus helpers*.

### Task 11: The Skills page

**Files:**
- Create: `web/src/lib/skills/SkillsPage.svelte`, `web/src/lib/skills/SkillRow.svelte`,
  `web/src/lib/skills/NewOnMachineDialog.svelte`, `web/tests/skills-prune.e2e.mjs`,
  `web/tests/skills-new.e2e.mjs`
- Modify: `web/src/App.svelte`:
  - `{:else if isSkills}` (1347) renders `<SkillsPage {canWrite} />`.
  - The shelf state, filters, `patch`/`refocus`/`bulk*` functions and `SkillCard` use for `/skills` are removed
    (`App.svelte` 92, 231-345, 677-949, 1347-1578).
  - The jump-to-skill flash moves into `SkillsPage`.

**Interfaces:**
- Consumes: Task 9's client and Task 10's helpers.
- Produces:
  - `SkillsPage` props `{ canWrite: boolean }`.
  - `SkillRow` props `{ row: SkillRow; selecting: boolean; selected: boolean; open: boolean; machines: number; canWrite: boolean; onarchive, onrestore, ondelete, onselect, onopen, onretry, onlike }`.
  - Every row action button has `data-act="archive"|"restore"|"delete"`, which focus moves to.
- The layout is the spec's *The Skills page* sketch and the reviewed mock:
  - header: `EY UP. YOUR SKILLS.`, `Skills.`, and the sync chip using `syncSentence`
  - the add form and Write a skill
  - the New banner, `{n} new skills turned up on {host}`, with **Review**
  - Active / Archived tabs as `role="tablist"`, with counts
  - search, a Liked toggle, Tag, List, Sort and **Select**
  - one bordered list of rows
  - a fixed bulk bar and toast
- Removal flow:
  1. Call the API.
  2. Remove the rows from local state without reloading the list's scroll container.
  3. Focus `focusAfterRemoval(...)`'s row `[data-act]` button, or the tab when the list empties.
  4. Show the toast for 8 seconds with **Undo**: `setSkillStatus` back, or `undeleteSkills`.
- Confirmations: `deleteConfirm` decides the dialog. A `no-copy` 409 opens the same dialog with
  `noCopyConfirmed:true` on confirm.
- The Archived tab shows the upgrade notice, with **Restore the Parked ones** and **Got it**.

- [ ] **Step 1: Write the failing e2e tests** with `openWorkspace` and routes for `/api/v2/skills*`.
  - `skills-prune.e2e.mjs`:
    - Archive the 2nd of 6 rows: the tab is still Active, `window.scrollY` is unchanged, focus is on row 3's
      Archive button, and the toast reads `Archived seo-bing · coming off 3 machines`. Undo puts the row back.
    - Delete `reet-babysit` (`yours`) opens a dialog titled `Delete reet-babysit?`. Delete on a plain row has no
      dialog.
    - Select 3, bulk Archive: one request with 3 ids, and the toast reads `Archived 3 skills · coming off 3 machines`.
    - On the Archived tab, Restore and the notice's **Restore the Parked ones** each send their request.
    - Retry on a `failed` row posts `/api/v2/skills/retry`.
    - At 390 px, light and dark: no horizontal scroll, and every row button is at least 44 px.
    - Prints `Skills pruning passed: …`.
  - `skills-new.e2e.mjs`:
    - The banner names `paseo-help, cq`. Review opens the dialog: **Keep** posts `{action:'keep'}`, **Archive** posts
      `{action:'archive'}`, and **Keep all** sends both names.
    - Differing digests show a machine choice.
    - Prints `New on a machine passed: …`.
- [ ] **Step 2: Run them to see them fail.** Run `npm --prefix web run build`, then
      `npx --prefix web vite preview --port 4173 &`, then `node web/tests/skills-prune.e2e.mjs`. Expected: FAIL.
- [ ] **Step 3: Implement** `SkillsPage.svelte`, `SkillRow.svelte` (collapsed row) and
      `NewOnMachineDialog.svelte`, and swap them into `App.svelte`.
- [ ] **Step 4: Run the tests.** Rebuild, then run both e2e files and `npm --prefix web run check`. Expected: both
      "passed" lines, exit 0.
- [ ] **Step 5: Commit:** *Rebuild the Skills page around Active and Archived, pruning in place*.

### Task 12: An opened skill

**Files:**
- Create: `web/src/lib/skills/UpdateFromOriginalDialog.svelte`, `web/tests/skill-open.e2e.mjs`
- Modify:
  - `web/src/lib/skills/SkillRow.svelte` (opened detail).
  - `web/src/lib/install.ts`: Copy prompt and the install command come from Task 10's helpers; `sourceURL` and
    `installPrompt` are no longer used for library skills.
  - `web/src/lib/Hosting.svelte`: **Write a skill** opens any library item with `/hosting?item={id}`.
  - `internal/api/skills_v2.go` adds:
    - `GET|PUT /api/v2/skills/{item}/content`: head revision files in, a new revision with origin kind `edit` out,
      through `SaveLibraryContentFor`.
    - `POST /api/v2/skills/{item}/publish {public: bool}`: creates or updates the item's hosted skill from its head
      revision. An imported skill's public page (`internal/api/hosted.go` template) shows *Imported from {origin}*.
- Test: `internal/api/skills_v2_test.go`
  - `TestSkillContentEditMakesRevision`: a PUT creates a revision with kind `edit`, and `SkillEditedSinceImportFor`
    is true.
  - `TestPublishImportedSkillCreditsOriginal`: the public page HTML contains `Imported from https://github.com/…`.
    Unpublishing hides it.

**Interfaces:**
- Consumes: Task 9's `useMachineVersion` and the `original` endpoints, and Task 10's `copyPrompt` and
  `installCommandFor`.
- The opened detail, from the spec's *Every skill is a reet.sh skill* and the mock:
  - **Machines**, from `health.machines`, each with its state and detail.
  - **Tags** and **Lists**, with Add.
  - **Imported from** `{origin}`, or **Kept from** `{host}`, with *For reference. reet installs its own copy.* and,
    for imported skills, **Update from original**.
  - **Notes**.
  - **Edit**, which links to `/hosting?edit={name}`, **Copy prompt**, **Publish**/**Unpublish**, and **Copy install
    command** only when published.
  - When `editedOn` is set: *{name} was edited on {host}; reet's copy put back.* with **Use this version**.

- [ ] **Step 1: Write the failing e2e test** `skill-open.e2e.mjs`:
  - Pressing the name opens the detail, with `aria-expanded="true"`.
  - Copy prompt writes `Use the seo-cluster skill: …` to the clipboard, never `github.com`.
  - On a private row there is no Copy install command button.
  - **Update from original** shows the changed paths and, when `edited:true`, the warning
    `Your edits will be replaced; they stay in this skill's history.` Confirm posts the fetched digest.
  - **Use this version** posts the agent id.
  - Prints `Opened skill passed: …`.
- [ ] **Step 2: Run them to see them fail.** Run `node web/tests/skill-open.e2e.mjs` after a build, and
      `go test -race ./internal/api -run 'TestSkillContentEdit|TestPublishImported'`. Expected: FAIL.
- [ ] **Step 3: Implement** the content and publish endpoints, Write a skill on library items, the detail and the
      dialog.
- [ ] **Step 4: Run the tests.** Run the Step 2 commands and `npm --prefix web run check`. Expected: PASS.
- [ ] **Step 5: Commit:** *Show where a skill came from, update it from its original and keep a machine's edit*.

### Task 13: The Sync page

**Files:**
- Create: `web/src/lib/sync/SyncPage.svelte`, `web/tests/sync-page.e2e.mjs`, and an API handler in
  `internal/api/skill_reconcile.go`: `GET /api/v2/sync/skills` →
  `{ machines: {agentId, host, paused, online, protocol, state, failed: {name, detail}[], waiting: string[], lastSeen}[], activity: SkillActivity[] }`
- Modify: `web/src/App.svelte` (`isSync` renders `SyncPage`; the `/sync/advanced` route goes in Task 17)

**Interfaces:**
- Consumes: Task 3's `SkillActivityFor` and health, Task 10's `syncSentence`, and the existing `PATCH
  /api/v2/sync/agents` with `pause`/`resume`.
- Layout from the spec's *Machines and the Sync page* and the mock:
  - an `h1` with the sentence
  - one row per machine, with its state line, a **Retry** for its failures, and a **Sync on / Paused** switch with
    `role="switch"`
  - for a machine below protocol 2: *Update reet on {host} to keep its skills in sync*, with the existing update
    action
  - recent activity, newest first

- [ ] **Step 1: Write the failing tests.**
  - Go `TestSyncSkillsState`: the shape of the response, a paused machine, a protocol-1 machine, and activity
    ordering.
  - e2e `sync-page.e2e.mjs`: the sentence for each state; Pause sends `PATCH` with `pause`; Retry posts per failed
    skill; 1440 and 390 px; prints `Sync page passed: …`.
- [ ] **Step 2: Run them to see them fail.** Run `go test -race ./internal/api -run TestSyncSkillsState` and the e2e
      file. Expected: FAIL.
- [ ] **Step 3: Implement** the handler and `SyncPage.svelte`.
- [ ] **Step 4: Run the tests.** Same commands, plus `npm --prefix web run check`. Expected: PASS.
- [ ] **Step 5: Commit:** *Make Sync one sentence, one row per machine and its activity*.

## Phase 5: the other ways in

### Task 14: Session picker, Community, lists, Home, Setups and scans

**Files:**
- Modify:
  - `internal/api/terminal_skills_install.go`, `web/src/lib/terminal/SkillInstallPanel.svelte`,
    `web/src/lib/terminal/skills.ts`
  - `web/src/lib/Community.svelte:36-48`, `web/src/lib/ListView.svelte`,
    `internal/store/store_lists.go` (`AddListToShelf(userID, listID int64, status string) (int, error)`)
  - `web/src/lib/home/whatWaits.ts`
  - `internal/setup/adapters.go:33-55` (the `skills` and `shared_skills` directory entries go), and setup restore
    (skip skill folders in old snapshots and say so once)
  - `internal/api/api.go:1503-1619` (`importEntry` stops creating library items for home folders; reports only)
- Test: `internal/api/terminal_skills_install_test.go`, `internal/store/store_lists_test.go`,
  `internal/setup/*_test.go`, `web/src/lib/home/whatWaits.test.mjs`, `web/src/lib/terminal/skills.test.mjs`,
  `web/tests/community.e2e.mjs`

- [ ] **Step 1: Write the failing tests.**
  - `TestPickerAddToYourSkillsMakesActive`: adding a published hosted skill makes an Active item with content, and
    no `sync_jobs` row.
  - `skills.test.mjs`: the *Your skills* group is gone; machine, project and unknown skills are still listed.
  - `TestAddListToShelfUsesStatus`: `archived` and `active` both land with that status.
  - `community.e2e.mjs`:
    - Save shows **Active** preselected and **Archived**, and posts the chosen one.
    - **Add all** shows `Put 12 skills on all 3 machines?` before sending.
  - `whatWaits.test.mjs`: `newOnMachines` with 2 names gives one entry, `2 new skills on Elitebook`, linking to
    `/skills?review=new`.
  - Setups: a new snapshot has no skill folders; restoring an old one with `.agents/skills` writes nothing there
    and returns the note `Skipped skill folders; your skills come from your library.`
  - `TestScanImportDoesNotCreateItems`: an import of an unknown home skill leaves `LibraryFor` unchanged.
- [ ] **Step 2: Run them to see them fail.** Run the Go tests with
      `go test -race ./internal/api ./internal/store ./internal/setup -run 'TestPicker|TestAddListToShelf|Setup|TestScanImport'`,
      then `npm --prefix web run test`. Expected: FAIL.
- [ ] **Step 3: Implement** each change.
- [ ] **Step 4: Run the tests.** Same commands, plus the community e2e. Expected: PASS.
- [ ] **Step 5: Commit:** *Route the picker, Community, lists, Home and Setups through Active and Archived*.

### Task 15: MCP tools and the CLI

**Files:**
- Modify: `internal/api/mcp_server.go` (478, 538-540, and the skill tools), `mcp/src/server.ts` (269 and the skill
  tools), `cmd/reet/main.go` (CLI registration), `internal/cliui/commands.go`, `docs/cli-reference.md`,
  `docs/mcp.md`
- Test: `internal/api/mcp_server_test.go` or the existing MCP test file, and `mcp/` tests if present

**Interfaces:**
- `reet_list_skills` and `reet_update_skill` take and return `status: "active"|"archived"`.
- `reet_remove_skill` deletes, taking `noCopyConfirmed`.
- `reet_list_archived` and `reet_restore_skill` replace `reet_list_ignored` and `reet_unignore_skill`.
- `reet_list_sync_targets` goes.
- CLI: `reet sync` and `reet agent library-import` go; `reet agent restore` comes from Task 6.

- [ ] **Step 1: Write the failing tests** asserting the tool list and each tool's schema and result.
- [ ] **Step 2: Run them to see them fail.** Run `go test -race ./internal/api -run MCP`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run the tests.** Same command; also run `mcp/`'s test script if one exists. Expected: PASS.
- [ ] **Step 5: Commit:** *Give the MCP tools and CLI Active, Archived and Delete*.

## Phase 6: remove the old engine

### Task 16: Delete the Go engine and its tables

**Files:**
- Delete or trim every Go file, table, trigger and flag in the spec's *What is deleted* (**Go** and **Tables**).
  `syncproof`, `/sync/agent-proof`, `/sync/moved`, enroll, identity, poll, update jobs, captures, hosted sync and
  private skills stay.
- Delete their tests, as listed in the spec's *Testing*.
- Add the migration `skills-engine-drop-v1`: drop the tables, the `shelf.used`/`bookmarked` columns, and every
  queued or awaiting-approval `sync_jobs` row whose kind is not `update`.
- Agents below protocol 2: the poll returns `needs-update` only.

- [ ] **Step 1: Write the failing test** `TestEngineDropMigration`: the tables are gone, update jobs stay, and other
      jobs are gone.
- [ ] **Step 2: Run it to see it fail.** Run `go test -race ./internal/store -run TestEngineDropMigration`.
      Expected: FAIL.
- [ ] **Step 3: Delete and trim**, using `go build ./...` and `go vet ./...` to find callers.
- [ ] **Step 4: Run everything.** Run `go vet ./...` and `go test -race ./...`. Expected: PASS, with no references
      left, checked with `grep -rn "SyncTarget\|skillplan\|REET_LIBRARY_MANAGED_REMOVALS" --include=*.go .` (empty).
- [ ] **Step 5: Commit:** *Remove the plan, preview and approve sync engine*.

### Task 17: Delete the web engine

**Files:**
- Delete the components and helpers in the spec's *What is deleted* (**Web**), and their `*.test.mjs` and e2e specs.
- Remove their entries from `web/package.json`'s `test` script.
- Update the specs that only mock `/api/v2/sync` (`add-machine`, `connect-flow`, `home` and the others the
  `grep -l "api/v2/sync" web/tests` lists) to the new shapes.
- Remove the `/sync/advanced` route and the More menu's remaining entries for it.

- [ ] **Step 1: Delete** the files and entries.
- [ ] **Step 2: Run the checks.** Run `npm --prefix web run check` and `npm --prefix web run build`. Expected: exit 0,
      and `grep -rn "SyncControl\|syncSelection\|SyncActivity" web/src` is empty.
- [ ] **Step 3: Run every e2e spec that mocks `/api/v2/sync`**, with the preview server. Expected: each prints its
      "passed" line.
- [ ] **Step 4: Commit:** *Remove Advanced sync and the shelf stages from the web app*.

### Task 18: Docs, changelog and the full check

**Files:**
- Modify: `README.md` (Features, the Skills/Sync paragraphs at 45-56, *Adding skills*, and *Nothing is sent to
  skills.sh*, which now says the controller fetches skill files from GitHub), `SECURITY.md` (the new outbound
  fetch), `docs/skills-machines-ux.md`, `docs/sync-skills-between-machines.md`, `docs/controller-sync.md`,
  `docs/skills-single-source.md`, the `docs/skill-content-*.md` pages, `CHANGELOG.md`, `handoff.md`, and
  `docs/ui-verification.md` (a new dated section with captures at 1440 and 390, light and dark, in
  `docs/images/ui/`)

- [ ] **Step 1: Rewrite the docs** for the new model. Superseded specs in `docs/superpowers/specs/` stay as history.
- [ ] **Step 2: Run everything.** Run `go vet ./...`, `go test -race ./...` and `npm --prefix web run check`, then
      every e2e file touched in Tasks 11-14 and 17. Expected: all pass. Record the exact commands and their outcome
      in `docs/ui-verification.md`.
- [ ] **Step 3: Commit:** *Document Active, Archived and Deleted skills and the simpler sync*.
