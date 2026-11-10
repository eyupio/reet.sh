# UX review — web interface

This review covers the signed-in and public web UI under `web/src`. It is
UI-only: no route, API or data-model behaviour changed here.

## How this was reviewed

- Walked the route table in `web/src/App.svelte` and every routed component.
- Walked the shell (`web/src/lib/shell/`) and shared tokens
  (`web/src/app.css`, `web/src/workspace.css`, `web/src/dialog-shell.css`).
- Exercised the empty, loading and error states through the Playwright fixtures
  in `web/tests/` (`empty-states`, `account-sessions`, `setups`, `lists`,
  `supporting-nav`, `navigation`).
- Checked the primary journeys: first load, empty workspace, start a session,
  connect a machine, sync, browse lists, account settings.

## Route and surface inventory

| Route | Main surfaces | Disposition |
| --- | --- | --- |
| `/` | `home/FleetStrip`, `home/NeedsYou`, `home/InFlight`, `home/SessionBoard`, `ProjectRailHost`, launch dialogs | Keep. Overview/detail split with `/machines` and `/sessions`. |
| `/sessions` | `sessions/SessionsPage`, terminal workspace, `SessionScreen`, context panel | Keep. |
| `/machines` | `EnvironmentSync` → `MachineFleet`, `MachineSetup`, `SyncActivity`, `MachineSkillImports`, `AddMachineDialog`; Projects drawer | Keep; setup panel already demoted behind Add a machine. |
| `/sync`, `/machines?view=compare` | `SyncControl` (compare view), `SelectedLibrary` | Keep; naming unified (below). |
| `/skills` | Shelf groups list/board, filters, `SkillCard`, `BulkBar`, sync overlay | Keep. |
| `/activity` (no accounts) | `ActivityPage` → `ActivitySection` | Keep; same section is reused inside Account. |
| `/account/*` | Profile, Agents, Gaffer AI, Password, Single sign-on, Sign-ins, API tokens, MCP access, Backups, Maintenance, Activity | Keep; one section renamed (below). |
| `/community` | Public lists and skills | Keep. |
| `/shared`, `/lists/:id`, `/l/:slug` | `SharedLists`, `ListView`, `PublicList`, `CollectionPanel` | Keep. |
| `/spaces` | Teams and organisations | Keep. |
| `/profiles`, `/setups` | `SetupProfiles` master/detail | Keep; naming unified. |
| `/prompts` | Prompt library | Keep. |
| `/hosting` | Write a skill | Keep. |
| `/notifications`, `/support`, `/support/:id` | Notifications, support desk | Keep. |
| `/admin/*` | Instance administration | Keep. |
| `/signin`, `/signup`, `/join`, `/reset`, `/setup`, `/welcome` | Auth and setup | Keep. |
| 404 | `NotFound` | Keep. |

Shell surfaces reviewed: `TopTabs`, `BottomTabs`, `MoreSheet`,
`WorkspaceSidebar`, `JumpSearch`, `Bell`, `AccountMenu`, `SectionNav`.
No navigation entry was removed: the More groups already give every supporting
page one home.

## Findings, ranked by user impact

### 1. Dead components still shipped in the source tree — removed

Four components had no reference in `src`, tests, scripts or docs:

- `web/src/lib/ControlRoom.svelte` (and its only style import
  `web/src/lib/landingTerminal.css`)
- `web/src/lib/MachineComparison.svelte`
- `web/src/lib/TrendSparkline.svelte`
- `web/src/lib/LoadingBoundary.svelte`

`compareInventories` in `web/src/lib/machines.ts` existed only for
`MachineComparison` and its unit tests, so it and those tests were removed too.
This removes code, not a user-facing capability: no route imported any of them.

### 2. “Sessions” meant two different things — renamed the account one

The top navigation's **Sessions** is agent sessions. Account → Security →
**Sessions** was browser sign-ins. Two unrelated things shared one word.
`web/src/lib/Account.svelte` now uses **Sign-ins** for the section, its panel,
search, filter group and counts. The route `/account/sessions` is unchanged.

### 3. The sync page had three names — unified on “Sync”

The same page was called **Sync** (`web/src/lib/Machines.svelte`,
`web/src/App.svelte` document title), **Compare & sync**
(`web/src/lib/Account.svelte`) and **Compare & sync**
(`web/src/lib/EnvironmentSync.svelte`). All visible labels now say **Sync**;
the descriptive sentence under the heading still explains what it does.

### 4. Setups were called “setup profiles” in places — unified on “Setups”

The page title was **Setups**, but the collection panel said **Setup profiles**
and the empty connect step said **No setup profiles yet.** Both now say
**Setups** (`web/src/lib/SetupProfiles.svelte`,
`web/src/lib/MachineSetup.svelte`). The CLI command `reet setup profile` is
unchanged because it is a product command, not a UI label.

### 5. Two settings pages ignored the account’s date format — fixed

`web/src/lib/SetupProfiles.svelte` and `web/src/lib/Hosting.svelte` formatted
dates with bare `toLocaleDateString()`, so they ignored the date order and time
zone chosen in Account → Profile. They now receive `timeZone` / `dateFormat`
from `web/src/App.svelte` and use `formatWhen` from `web/src/lib/time.ts`, the
same helper the rest of the UI uses. The step-by-step machine setup’s update
times were brought along too (`web/src/lib/MachineSetup.svelte`).

### 6. Several “Sync now / Sync all machines” entry points — flagged, not merged

Sync actions exist in `FleetActions.svelte`, `MachineFleet.svelte`,
`SyncControl.svelte`, `SyncSetup.svelte` and `Hosting.svelte`. Some are the same
action in different contexts (fleet-wide, per machine, per scope, after saving a
skill). Consolidating them safely needs a product decision about which scopes
should remain primary; removing one could remove the only sync action in that
context. Left for follow-up; see below.

### 7. “Needs you” appears in the bell and on Home — kept

`web/src/lib/shell/Bell.svelte` is global quick access; `web/src/lib/home/NeedsYou.svelte`
is the page's work queue. They answer different questions (“is anything waiting
anywhere?” vs “what should I do next here?”). Removing either would hide a
genuine status, so both stay.

### 8. Home fleet cards repeat Machines data — kept

`home/FleetStrip.svelte` repeats CPU, memory, disk, agents, skills and MCP
counts that also appear on Machines. On Home these are the at-a-glance decision
inputs for “which machine has room / is healthy”, so they stay. The detail and
actions remain on Machines.

## Consistency drift recorded

| Area | Drift | Disposition |
| --- | --- | --- |
| Navigation labels | Sync / Compare & sync | Fixed to Sync. |
| Account labels | Sessions (auth) vs Sessions (agents) | Fixed to Sign-ins. |
| Setups | Setups / Setup profiles | Fixed to Setups. |
| Dates | `formatWhen` vs raw `toLocaleDateString` | Fixed on Setups, Write a skill and machine setup. Other raw timestamps are mostly `title` tooltips; listed for follow-up. |
| Units | Byte counts already use `bytes()`/`toLocaleString`; ages use `elapsed`/`shortAge` | No change needed. |
| Empty states | “No setups yet.”, “No lists yet.”, “Nothing in flight.” are consistent in tone | No change. |
| Actions | Primary filled button per screen mostly holds; sync is the exception | Flagged above. |

## Journeys checked

- **First load:** boot shell → `RouteSkeleton` → page. No change.
- **Empty:** no machines (`MachineSetup` step-by-step), no setups, no lists,
  no sessions. Every empty state keeps one clear next step.
- **Primary task:** start a session from Home, connect a machine, sync, create
  a list, open Account settings.
- **Loading:** skeletons are shared (`Skeleton`, `RouteSkeleton`); no new loader
  introduced.
- **Error:** `PageLoadError`, inline `role="alert"` notices and retry buttons are
  untouched.
- **Settings:** Account sections still have one address each; sign-in sessions
  moved label only, not route.

## Deliberately not changed

- **Legacy addresses** (`/terminals`, `/control`, `/projects`, `/account/sync`,
  `/machines?view=compare`, `/setups`) still resolve. `CONTRIBUTING.md`
  asks for old paths to stay stable; they are redirects, not duplicate UI.
- **Backend error text** that says “Compare & sync” is quoted from the API
  (`web/src/lib/syncProblems.test.mjs`) and must not be rewritten in the UI.
- **`docs/loading.md`** references `web/src/lib/components/LoadingBoundary.svelte`
  from another project (`eyupio/zoomies`), not this repository's removed file.
- **Account → Machines / Sync** remain links into the Machines page rather than
  duplicated panels.
- **Raw `toLocaleString()` timestamps** used only as hover titles in session,
  terminal and activity rows: changing every one needs the preference passed
  through many terminal components. Left for follow-up so this diff stays
  reviewable.
- **Sync action consolidation** (findings 6): ambiguous; kept and flagged.

## Follow-ups

1. Decide the primary sync scope (fleet vs machine vs target) and retire the
   duplicate entry points.
2. Pass `timeZone` / `dateFormat` into terminal and session components that use
   timestamps only in `title` attributes.
3. Consider renaming the auth-session sign-out notices (“That session is signed
   out.”) to “browser” wording to match the Sign-ins section.
