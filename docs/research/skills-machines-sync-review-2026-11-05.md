# Skills, machines, binary and installer review

Reviewed 5 November 2026 against the checkout at `a53ddf1f`. Scope: Skills,
written skills, machine inventory, sync scheduling and receipts, enrollment,
`reet agent`, native startup, and `scripts/install.sh`.

## Finding and product contract

The original system had a capable preview/apply engine, but no dependable
account-level library workflow. The user had to coordinate shelf membership,
drafts, machine folders, saved selections, local apply consent and background
automation. A successful scan could look like successful setup even though no
installation would run. Most of the problem was the contract between these
pieces, rather than the visual styling.

The implemented default is:

1. The account has one library: shelf skills plus all its written skills.
2. Every newly enrolled machine follows it. A complete scan prepares the
   supported home folders and queues the existing preview/apply chain.
3. New library entries and written revisions follow on authenticated polls,
   without a browser. A single Sync all machines action establishes or retries
   this policy across the fleet.
4. Written content travels as private immutable revisions. Publishing is a
   separate visibility choice. A machine-only skill is captured privately from
   an enrolled source, including supporting files.
5. Offline machines retain the policy and catch up. Paused machines stay paused.
   In-flight plans keep their revision until they settle. Failures remain
   actionable; continuous polling does not repeatedly retry a broken source.
6. The primary workflow adds library skills and backs up replacements. It does
   not delete unrelated local files. Projects and deliberate removals are
   advanced operations.

## Code and usability findings

| Priority | Defect and user consequence | Change |
| --- | --- | --- |
| P0 | Enrollment stored an agent but no shared selection. Keep synced could be enabled with nothing to maintain. Every new machine required another manual selection exercise. | API enrollment enables the shared policy. `prepareLibraryTargets` creates home-folder plans after the first complete inventory. Sync all machines also brings existing enrollments onto it. |
| P0 | Add a machine omitted `--allow-apply`; the UI offered installation that the local binary would refuse. A blank machine might report no supported folder at all. | The connection installer explicitly opts into skill apply and prepares `~/.agents/skills`. The low-level CLI retains its explicit local consent flag. |
| P0 | Whole-shelf automation omitted drafts written after the selection. A draft appeared in the browser's candidate list but disappeared from later background refreshes. | `SyncLibraryFor` merges shelf and owned written skills. Both initial setup and background refresh use this same collection. |
| P0 | `MaintainSync` compared only IDs. Editing a private draft changed its revision without changing its ID, so automation missed the change. | Compare the full ordered candidate records. Fleet-written skills use private revisions even when published, giving content edits a changed source digest. |
| P1 | Installed/missing calculations used IDs without verifying private bytes. Scans could report a written skill as `local/<name>`, creating a second identity or reporting it missing forever. | Private completion uses fresh content digests. Scans of saved private written installs reuse the owned written identity; unpublished projections stay hidden from the shelf. |
| P1 | A no-change automatic preview rescanned, invalidating its previous content observation, but did not record an applied revision or send a replacement observation. This could produce an endless stream of successful no-op jobs. | No-change completion records the current revision. The agent reports content after no-change previews as well as applies. The integration test asserts that subsequent polls queue no new jobs. |
| P1 | “Ready” meant connector plus scan, before the machine received its skills. | New shared enrollments show Syncing your skill library and expose failures; Ready requires the library targets to have no missing content and no pending jobs. |
| P1 | The copied command downloaded a binary, then relied on PATH and several independent shell commands. A failed step could leave a misleading partial setup. The development channel was configured as stable. | The installer accepts site/name and performs the steps sequentially with the verified binary's absolute path. Stable/dev selection is preserved. Matching enrollment retries are idempotent; unrelated enrollments are refused. |
| P1 | `curl \| sh` gives the script the process input. A connection command that prompts there cannot safely read the token as normal terminal input. | Connection setup uses the controlling terminal when available; credentials remain outside the copied command and browser storage. |
| P1 | Native enrollment startup was coupled to the terminal wizard and tmux. A detached agent could stop at logout/reboot without automatic recovery. | `agent service install --sync-only` installs monitoring/sync independently. Native-manager unavailability is detected before creating a unit; the installer falls back to a detached agent and prints the restart boundary. |
| P1 | Automatic home-folder sync and advanced per-folder selections could silently undo each other's intent. | Automatic sync owns home-folder plans. Saving a custom home plan while it is enabled gives an explicit error. The UI offers an Automatic sync switch; projects stay explicit. |
| P1 | Failed/expired private captures retained their idempotency key forever, so a whole-library retry could keep returning the same dead request. | Explicit fleet retries release terminal library-capture keys while retaining their records. Normal polls preserve backoff. One private capture serves the fleet. |
| P2 | The main action required choosing skills, machines and install locations. Offline enrollments without inventory were absent, making the fleet look smaller than it was. | One fleet action, counts, and a row per enrollment. Offline, paused, syncing and failed states have distinct text. Library details and custom controls are secondary. |
| P2 | A source conflict after a successful old run could still look like a healthy machine. Duplicate-name validation compared only the first two candidates. | Machine summaries expose current problems/failures. Library candidates identify name conflicts, preserve the previous good selection, and validation checks every selected source. |
| P2 | Quick binary-update retries can share a millisecond timestamp, causing the Machines UI to display an older failed update as the latest. | Use insertion order as the timestamp tie-breaker. The regression test forces equal timestamps. |
| P2 | A per-machine Add a skill action called the fleet reset, enabling automatic sync and replacing custom plans on unrelated machines. | Destination actions pass the selected enrollment to the controller. Only that machine's policy, plans and failed private-copy requests are retried; the explicit fleet action retains its fleet scope. |
| P2 | An unrelated unreadable or oversized skill made a whole content observation incomplete, hiding complete matching private skills and repeatedly scheduling no-op jobs ahead of later folders. | Fresh incomplete observations can prove individual complete matching digests. Stale, paused and invalidated observations cannot. A regression verifies the next folder receives its work. |

## Code map

- [Shared library and automatic folder setup](../../internal/store/store_sync_library.go)
- [Background scheduling](../../internal/store/store_sync_automation.go) and [content-aware completion](../../internal/store/store_sync.go)
- [Controller endpoints](../../internal/api/sync_control.go) and [private inventory identity](../../internal/api/api.go)
- [Primary Sync UI](../../web/src/lib/SyncSetup.svelte) and [machine connection progress](../../web/src/lib/machineJoin.ts)
- [Installer](../../scripts/install.sh), [binary enrollment and polling](../../internal/scan/agent.go), and [independent native startup](../../internal/terminalhost/service_agent.go)
- [Two-machine integration proof](../../internal/api/sync_library_test.go) and [browser scenarios](../../web/tests/skills-sync-simple.e2e.mjs)

## User journeys after the change

### Add a machine

Name it, create its token, and copy one command. The script verifies the release,
configures the correct site/channel, enrolls with local apply consent and starts
the sync service. It does not ask for terminal shell consent or install tmux.
The dialog progresses from connection to first scan to library sync to Ready.
Closing it leaves the work running. Its token is pasted into the local prompt.

Native startup uses the current user's service manager. Linux user services may
need administrator-enabled lingering to run before login or after logout. macOS
uses the logged-in user's LaunchAgent. A host without that manager falls back to
a background process and receives explicit restart instructions.

### Add or write a skill

Adding a shelf skill offers Sync machines immediately. Saving written content
offers Sync all machines. The library includes private drafts without requiring
publication or another per-machine selection. Background agents also discover
the new entry on later polls. Editing written content produces a new immutable
revision; replaced local copies are backed up.

### A machine-only skill

The source machine scans it into the account's shelf. Library automation requests
a private capture from its enrolled agent, validates its manifest and distributes
the resulting revision. Review-required files, capture failures and progress stay
visible in Sync, with a private-copy dialog for review/retry. A source that has
not connected or cannot provide the files blocks that entry visibly.

### Offline, paused and failed machines

An offline machine remains in the list; its policy survives and catches up when
the agent reconnects. Paused machines are not resumed by the fleet action. Current
failures identify the affected machine and remain available in Sync activity.
Retry plans fresh work rather than reusing an old approved filesystem plan.

## Validation

Final checks on this working tree:

| Check | Result |
| --- | --- |
| `go test ./internal/api ./internal/scan` | Passed, including the two-home integration test. |
| `go test ./internal/store ./scripts` | Passed. |
| `go test ./internal/scan ./cmd/reet` | Passed. |
| The two new sync-only native service tests | Passed without tmux. |
| `npm run check` | Passed; zero errors and two existing unused-CSS warnings in LaunchBar and SessionHoverCard. |
| `npm run build` and `go build ./cmd/reet` | Passed. The web build retains its existing large-chunk warning. |
| `skills-sync-simple.e2e.mjs` | Passed on phone/desktop and both themes. |
| `add-machine.e2e.mjs` | Passed, including the new first-scan → syncing → attention → ready scenario. |
| Existing affected browser suites | First skills, empty states, vocabulary, written-skill records, machine transfers, per-machine additions, skill cards, first machine, fleet status, activity and navigation all passed; 13 suites total with the two above. |
| `sh -n scripts/install.sh` and `git diff --check` | Passed. |
| Race-enabled PR follow-up regressions and real two-home integration | Passed. Destination policies and capture retries stay scoped; complete digests advance later folders without trusting stale observations. |
| Docs hook tests, strict build and rendered-link checker | Passed: 28 hook tests and 50 rendered pages. |
| Entire terminal-host package | Not clean in this environment; missing tmux and existing process-timeout failures are detailed below. |

PR #572 follow-up verification also covers destination-scoped requests, preserving
another machine's disabled automation and custom mirror plan, rejecting an unknown
enrollment, and fresh incomplete observations versus stale, paused and invalidated
ones. Each reported regression was reproduced before applying its fix. The docs
review link now points to its repository source because research notes are excluded
from the published site; strict docs build and rendered-link checks pass.

The full `go test -race -p 2 ./...` run completed with no reported data races. Its
only failing package was `internal/terminalhost`: three tests require missing tmux,
and two process-group timeout tests reported surviving children. Other packages,
including API, store, scanner, CLI and installer, passed. The combined full-suite
check remains incomplete; passing scoped regressions do not replace it.

- Controller/store/scanner/installer package tests cover scheduling, ownership,
  durable policies, previews, applies, receipts, source conflicts and recovery.
- A real HTTP-controller integration test runs the binary's enrollment and
  polling implementation with two isolated home directories. It verifies first
  machine sync, a subsequently added machine, private written edits, a newly
  written skill, a machine-only skill with supporting content, reconnection,
  no duplicate local written identity, stable idle polling and account isolation.
- Store tests cover all supported home folders across machines, explicit project
  boundaries, preservation of good selections and duplicate-job prevention.
- Installer fixtures verify stable/dev channels, checksum failure preservation,
  exact quoted machine names, connection failure short-circuiting, the sync-only
  service command and native-manager fallback. Enrollment retry tests reject a
  different site or machine.
- New service tests use a fake native manager and a PATH without tmux. They check
  that sync installation preserves an existing terminal unit, never invokes its
  service, refuses another enrollment and leaves no unit when no manager exists.
- Browser checks cover desktop and phone, light and dark, own drafts, offline and
  paused machines, API failures, empty fleet, viewer restrictions and horizontal
  overflow. Empty-fleet Add a machine remains visible and tappable at 320×700;
  the connected-fleet sync action stays above phone navigation at 390×812.
  The existing comprehensive Add a machine browser test also runs.
- The web unit suite, Svelte check and production build are part of validation.

Native service commands are checked using fixtures, not a running systemd or
launchd host. The broader terminal-host test run also encounters environmental
failures: tmux is absent, and two existing process-group tests report children
surviving timeout. These do not establish a passing native-platform integration
run. The new sync-only service tests do not need tmux.

## Boundaries and follow-up work

These are limits of the current implementation, not hidden guarantees:

1. **Third-party upstream content is not commit-pinned.** The default converges
   library membership; private/written copies converge by digest. Public GitHub
   sources still use the existing installer and source identity. A moving upstream
   repository can produce different bytes at different installation times.
   Before promising byte-identical third-party content, snapshot every library
   source centrally or pin its upstream commit and verified manifest.
2. **A captured machine skill is a snapshot.** Editing files on a machine is not
   multi-master authoring. Use Reet's written editor or a reviewed import/revision
   for updates. Existing private library content can replace a divergent local
   copy, with a backup. A future local-edit workflow needs explicit conflict
   handling; newest timestamp alone is insufficient.
3. **Storage remains bounded:** 128 private bundles, 256 private revisions and
   64 MiB per account; written bundles are limited to 2 MiB and 100 supporting
   files. Frequent edits can exhaust revisions. The UI carries the capacity
   error, but retention/garbage collection and a usage meter are still needed
   before general availability.
4. **Projects and legacy Codex folders are separate.** Automatic destinations
   are Shared agents / Codex, Claude, Cursor and OpenCode home folders. Existing
   `.codex/skills` is scanned but remains a legacy scan-only location. Project
   scopes are not enrolled implicitly.
5. **Deletions are deliberate.** Removing an item from the shelf does not delete
   its installed copies through the additive default. A later scan can rediscover
   it. Fleet-wide removal needs a tombstone/exclusion model and a reviewed delete
   policy before it can behave like a conventional exact mirror.
6. **Release coordination is required.** The controller, embedded web UI,
   installer and binary must ship together. Publish a matching binary before
   directing users to the new installer connection mode. Existing locally
   scan-only agents still require local `allow-apply on`; the controller cannot
   grant itself filesystem consent.

The beta changes intentionally replace home-folder selection-based automation
with the shared library when automatic sync is enabled. No changes were deployed
or published during this review.
