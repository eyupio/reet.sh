# UI verification — 21 October 2026

## UX follow-up: board and connection flow

The first review implementation builds on `216b3aa` (merged PR #27). It adds an optional, browser-persisted Board/List switch; desktop board columns follow On the shelf → To test → In use and stack below 900px. Stage buttons retain keyboard/touch operation and focus after moving a card. Board cards group History, metadata refresh and removal under More; Escape closes that disclosure and returns focus. Stage changes update shelf flags, not installed files. The existing list layout remains the default.

List settings now clone the saved audience into an independent draft and discard it on Cancel, closing Settings, or switching to People. Publishing an existing non-public list opens a confirmation with the actual description, skills and shared notes before a write. JSON is labelled as a personal shelf backup, with its exclusions described beside the export control and in the README.

Account creates named tokens inside Connect a machine, includes the fresh token in a copyable connection command, and acknowledges a complete recent scan for the chosen machine name. Tokens remain in memory only and disappear on reload. Token revocation requires confirmation. First-run shelf guidance and advanced tools are simplified. Shell installation now prints a safely quoted PATH command and the next steps for connecting or hosting; binary help describes the two journeys and server environment settings.

Validation on the updated source:

- `npm run check`: four machine tests pass; zero Svelte errors or warnings.
- `npm run build`: pass.
- `go test ./...`: all packages pass. Installer fixtures also ran uncached; a copied PATH command resolves the correct binary with spaces, an apostrophe and shell substitution characters in its path.
- 48 browser layout checks: first shelf, List, Board, publication confirmation, account connection, and viewer account × light/dark × 320/375/768/1440px. No document overflow.
- 24 axe scans across those states at 375/1440px: no reported WCAG 2/2.1/2.2 A/AA rule violations.
- Ten additional interactions pass: token creation/copy without local storage, complete-scan acknowledgement, token disappearance/revocation, draft cancellation including nested collaborators, publication preview/Escape, explicit publication, persisted board and keyboard stage moves, More-menu Escape/focus, empty search recovery, and accurate backup coverage. No browser runtime errors.

Browser tests used isolated SQLite fixtures and local Chromium with reduced motion. No production instance, native service installation or deployment was changed. Physical touch, screen readers, Safari/Firefox and native macOS lifecycle behaviour remain outside these checks. Consolidating Sharing & access, team administration, and the mobile navigation are subsequent review work.

This records the initial concepts and shared foundation. See [Library implementation and binary validation](library-implementation.md) for the selected production design and its follow-up checks.

## Collapsible skill cards

Shelf cards in both List and Board now start collapsed. Their overview keeps the skill name and source link, explicit stage, available install counts/weekly trend, deduplicated machine count, tag/list counts and a personal-note indicator. Long names are limited to two lines and long sources are ellipsised in the collapsed view; expanding reveals their full text. Stage moves, Like, Copy install and Copy prompt remain available while collapsed, subject to the account's write permissions. Source links open the original repository/site in a new tab; local-only skills show Unlinked source until linked. Detailed descriptions, tag editors, machine/list chips, history and notes are only rendered when expanded. These detailed actions replace the initial board's More menu. Desktop List cards use a denser two-column row for the overview and actions.

Per-skill expansion is held by the shelf during the current visit, so it survives card moves, filters and switching layouts. An explicit Expand all / Collapse all control operates on the visible shelf. Folding a note editor retains its unsaved draft until reopened on the same card. Native disclosure buttons expose expanded state and reference stable detail containers. Contextual copy-button labels also retain the visible Copied text after activation for speech-input compatibility.

Environment badges remain visible in both card states. They aggregate agent inventories per machine: green Installed means at least one complete, recent agent scan includes the skill; amber Missing requires all reported agent scans to be complete and recent with no occurrence. Neutral Not confirmed covers stale, partial or heartbeat-only inventories. Recent uses the same 14-day window as machine comparison. Status text accompanies the colour, and report freshness updates every minute while the page is open. The shelf indexes each machine's confirmed skill IDs once and shares that index across cards.

## Backup/profile integration (PR #28)

Integrated main through `2676376` (PR #29 destination testing/storage controls), preserving the sealed personal/instance backup service and new Account sections. List/Board and default card expansion now persist in the existing account profile. The toolbar saves layout changes; individual and bulk expansion overrides last for the visit. Read-only users may change display preferences, while name changes and backup writes remain restricted. Migration preserves existing profile preferences and defaults existing users to List with collapsed cards.

Tools links to Account and Admin backups instead of recommending manual database copies. Personal restore wording describes its merge behaviour: matching saved flags, notes and tags are replaced, later-added skills remain, and lists/teams/account settings are excluded. Local-copy and destination removal use keyboard-accessible confirmation dialogs. Backup tables expose labelled, focusable horizontal scrolling regions.

Validation covers profile migration/reopen persistence and invalid layouts, defaults across browser contexts, temporary expansion overrides, toolbar persistence, personal backup creation/verification/download/restore/deletion, instance creation/verification, and viewer preference isolation/write restrictions. Backup pages are checked at 375/1440px with axe. S3 tests use the project's test fixture; no real off-site destination or production restore was exercised.

## Scope and method

The requested Web Design Builder skill was run and its complete output read. Playwright MCP is unavailable, so its MCP-specific phase was skipped. Separate local Playwright tests drove the installed Chromium browser, and axe-core checked rendered pages against its WCAG 2 A/AA and WCAG 2.1 A/AA rules.

The real app was tested against Go servers using isolated SQLite databases under the temporary work directory. Test accounts, invitations, API tokens, skills, lists, and settings were created only in those databases. No existing instance or data was changed. Browser tooling was installed outside the app; the app package manifest and lockfile are unchanged.

## Results

| Check | Result |
| --- | --- |
| `npm run check` | Pass; zero errors and zero warnings |
| `npm run build` | Pass |
| `go vet ./...` | Pass |
| `go test -race ./...` | Pass; all Go packages |
| `git diff --check` | Pass |
| Concept layout matrix | 342 passes: 19 views × 3 directions × 2 themes × 3 widths |
| Concept axe scans | 114 scans; no reported violations: 19 views × 3 directions × 2 themes at desktop width |
| Real app layout and axe matrix | 114 passes: 19 rendered states × 2 themes × 3 widths; no document overflow or reported violations |
| Browser runtime errors | None in completed concept and app runs |

Widths were 375, 768, and 1440 CSS pixels. Desktop captures used a 1000-pixel viewport height; phone captures used 812 pixels. Reduced motion was enabled for the main runs. Wide tables scroll within their labelled regions.

The 19 app states were account sign-in, token sign-in, owner setup with bootstrap, first-run token shelf, populated shelf, tools expanded, tag management dialog, account, shared list, list people, list settings, confirmation dialog, admin with users/rollup/settings, public list, unavailable public link, invitation acceptance, expired invitation, bookmarklet success, and bookmarklet error. Owner setup without bootstrap and a first-run account shelf also passed in the initial integration run.

## Interaction evidence

The explorer passed password mismatch handling, password visibility, sample sign-in navigation, search, likes, notes, removal confirmation, Escape to close the dialog, undo, state switching, and keyboard tab navigation. Phones use a labelled screen selector.

Real server-backed checks passed:

- Owner setup rejects mismatched passwords and creates a session after valid input.
- Show/hide password preserves the bound value.
- Shelf search changes server-backed results.
- Restore from JSON opens the file chooser from a focused button using Enter.
- A personal token is shown once; reloading removes its secret from the page.
- A public list excludes member identity.
- An invitation creates the intended account; a member cannot open admin.
- Failed account sign-in is announced; valid credentials open the shelf.
- Legacy token sign-in rejects a wrong token and accepts the temporary valid token.
- A configured bootstrap credential remains required for owner setup.

## Issues corrected

- Orange selection text measured 4.36:1. It now uses ink with the brand tint.
- Adoption badges measured 4.49:1. The success colour was darkened.
- Public-list primary button text was overridden by a generic action rule. Primary buttons now retain their foreground colour.
- A hidden username input inherited full width and caused token sign-in overflow. The width rule now applies only to visible inputs.
- Admin role selects now have accessible names identifying their user.
- Restore now uses a keyboard-operable button to activate its file input.

Screenshots in `docs/images/ui/` show the final concepts and implemented form. The explorer is `web/concepts/controller.html`.

## Limits

Automated axe results are not a WCAG certification. Screen-reader behaviour, physical touch interaction, high zoom, long translated content, Safari, and Firefox still need manual validation. Checks exercised representative data and roles, not every permission combination or every control on every page. Explorer failure states demonstrate designs rather than real network recovery.

The explorer has no external asset dependencies. The production build emits roughly 62.4 kB of CSS and 177.1 kB of JavaScript (10.5 kB and 54.5 kB gzip). These are build sizes, not measured load times. The explorer is outside the production entry graph.

No deployment was performed. The three directions remain concepts; the app contains the shared Library foundation and improvements described in `ui-redesign.md`.

## Screenshot follow-up: navigation, density and reports

Built on main `8351479` after PR #30, then integrated `4b5a6ff` (PR #31 backup compression/password support) before opening the follow-up PR. The navbar has Account/Admin dropdowns on desktop and a nested Menu on phone/tablet widths, with native links, Escape/focus return and outside dismissal. Each section has its own refreshable URL and section menu:

- Admin: `/admin/overview`, `/admin/environments`, `/admin/users`, `/admin/team`, `/admin/backups`, `/admin/settings`, `/admin/activity`.
- Account: `/account/profile`, `/account/security`, `/account/sessions`, `/account/machines`, `/account/sync`, `/account/tokens`, `/account/backups`, `/account/activity`.

Old Account/Admin hash links resolve to their equivalent page. The old Connect a machine link opens setup on Machines. Navigation uses normal links so Back/Forward and open-in-new-tab work; profile edits and fresh tokens belong to the page and are discarded on navigation. Tokens have a standalone creation/copy/revocation page; setup can also create a token inline. Machine inventory, comparison/reference controls and token lists no longer share one long page. User/rollup data is fetched on the appropriate Admin page.

The shelf puts advanced filters/lists behind a disclosure, removes redundant heading counts/help, uses a compact capture row and stops pinning a tall toolbar on smaller screens. Empty Board stages are hidden below 900px, with their zero counts retained in the stage controls. Collapsed cards prioritise Installed/Missing before unknown reports and show up to three environment badges; +N more expands the card. Copy install, Copy prompt and source links remain visible.

Server-only heartbeat records are excluded from shelf filters/badges and machine comparison, while empty/stale agent inventories remain visible. Reports are retained: Admin Environments and Account Machines have separate collapsed heartbeat diagnostics. Machine/agent inventories are grouped into disclosures. Team usage uses cards at widths up to 1000px, keeping the skill identity with its metrics and expanding person/agent detail on demand. Desktop tables retain the skill/user column when scrolled. The former misleading Machines count is labelled Agent reports. User management actions fold behind Manage.

Validation used an isolated 64-skill fixture with all skills In use, multiple agent inventories and repeated container heartbeat IDs, plus a viewer account:

- Eight pure inventory/status tests pass, including exclusion of heartbeat-only machines without losing empty, stale or server-with-agent inventories.
- `npm run check`: zero errors/warnings; production build passes. `go vet ./...` and `go test -race ./...` pass.
- All seven Admin and eight Account routes load directly. Legacy links, Back, refresh, nested navbar/submenu navigation, collapsed setup, inventory separation and token creation/copy/revocation pass.
- 48 layouts: eight pages × 375/800/1440px × light/dark; no document overflow. Sixteen axe scans cover phone/tablet pages; three additional scans cover open menus at phone/tablet/desktop widths. Final team copy was checked again with axe.
- Menu keyboard opening, Escape/focus return, outside dismissal, bounded badges/reveal, collapsed prompt/source actions, viewer menus and write restrictions pass. No browser runtime errors.

Review captures (fixture data):

![Tablet shelf](images/ui/responsive-shelf-tablet.png)
![Phone shelf, dark theme](images/ui/responsive-shelf-phone.png)
![Tablet team usage](images/ui/responsive-team-tablet.png)
![Phone navigation](images/ui/responsive-navigation-phone.png)

These are Chromium checks, not physical Android/iOS, Safari/Firefox or screen-reader certification. No production data was changed or removed.

## Yorkshire voice follow-up

Based on main `f53fe91` (PR #32). Restored Yorkshire character to populated, everyday pages: the shelf introduction and stage hints, sign-in, community, lists, teams, profile, admin and successful saves. Navigation, actions, installation states, privacy statements and destructive/error language retain their precise labels. `docs/voice-and-copy.md` records the approach for future changes. Existing installer/CLI completion copy already carries the voice and was retained.

Validation used a separate copy of the isolated 64-skill fixture:

- `npm run check`: eight inventory/status tests pass, zero Svelte errors/warnings; production build passes. `go vet ./...` and `go test -race ./...` pass.
- Six populated/representative pages at 375/800/1440px in light/dark: 36 layouts without document overflow. Seven axe scans cover these pages and signed-out sign-in at phone width, with no violations of the selected WCAG A/AA rules.
- Populated shelf heading and stage hint render; collapsed Copy prompt copies the real command, source remains available, and the new empty-search message recovers through Clear filters. Signed-out welcome checked; no browser runtime errors.
- Reviewed tablet light and phone dark captures below. Static wording changes have browser verification rather than unit tests that mirror strings. No dependencies added, production data changed or deployment performed. Physical device, other browser and screen-reader checks remain outside this verification.

![Yorkshire shelf, tablet light](images/ui/yorkshire-shelf-tablet.png)
![Yorkshire shelf, phone dark](images/ui/yorkshire-shelf-phone.png)

## Machine alias repair and self maintenance

Built on main `f53fe91`. Account → Machines now supports reviewing a merge/rename, forgetting a whole machine, and generating inventory-only fleet refresh commands. Matching agent/project reports keep the newest scan without unioning stale installs; distinct groups remain. Account-scoped aliases survive restarts and redirect ordinary old-name imports, pruning, heartbeat registration, machine filters and restores. Saved reference snapshots are retargeted without changing their desired contents/revisions; conflicting references abort the entire transaction. Updated sync binaries accept historical host/target aliases.

`reet scan --prune --reconcile --env NAME` explicitly associates the actual hostname with the preferred name before importing a fresh scan. Dry runs send neither repair nor inventory writes. The fleet check counts only complete reports received after its start; it requires running the command on each physical machine and from each tracked project. No install/remove commands are issued. Offline or unscanned scopes are not claimed as refreshed.

Validation:

- `go vet ./...`, `go test -race ./...`, frontend check and production build pass. Zero Svelte errors/warnings; eight existing frontend inventory/status tests remain passing.
- Store/API tests cover duplicate groups, latest inventory selection, preserving reference snapshots/legacy IDs, conflict rollback, account isolation, rename chains/back-to-old-name, restart persistence, alias-aware restore/filtering/heartbeats, whole-machine forget and shelf retention. Viewer writes and anonymous requests are denied.
- CLI tests cover actual-hostname reconciliation before import, preserving installed files, dry-run/no-name/conflict handling, no credential output, old host/target sync aliases and rejection of unrelated hosts.
- Chromium with real CLI execution against isolated fixtures: two machine names/eight agent reports become one preferred name/four agent groups. A refresh prunes five stale reported skill links, updates all four complete reports, moves progress from 0/4 to 4/4, and retains the saved reference. Rename survives reload and old-hostname imports; forgetting removes reports while keeping both shelf skills. Merge cancellation and forget Escape work.
- Six maintenance layouts at 375/800/1440px in light/dark, three axe scans, no document overflow or runtime errors.

![Merged machine with aliases and maintenance controls](images/ui/machine-aliases-tablet.png)

This does not alter production records until deployed and used. The repair tests use temporary databases and fake skill folders; physical device/browser and screen-reader checks remain manual.

## Private skills dialog: layout, focus and motion matrix — 25 October 2026

Built on main `e5399b6` plus the fix round `d75b873`. This records what the browser fixture
`web/tests/skill-content.e2e.mjs` now checks for the [Private skills dialog](skill-content-review-ui.md),
what an accessibility scan found, and what was not checked. It adds no component change: the
matrix found no layout or focus defect.

**Layout matrix.** Ten states, each at 1360, 768, 390 and 320px wide in light and dark (eight
size and theme pairs per state): the Pin step with two pins (one with a very long unbroken name)
and two ownership records (one damaged); the Check step before a check, while waiting for the
machine, and with a found folder of 60 long unicode file names; the Adopt step with nothing chosen
and with the 256-file breakdown including a very long unicode path; the record confirmation; the
read-only "Up to date" result with three ownership records; the release confirmation; and the same result after the release, with two records. For each
state the page has no horizontal overflow, the dialog sits inside the window with **Close** in the
header and **Done** in the footer in view, no scrolling table or file list overflows sideways, each
primary action is scrolled to inside the dialog's own body and a hit test at its centre lands on it
(nothing covers it), and a confirmation sits inside the window with both its buttons reachable. The
primary actions are in the scrolling body (**Save pin**, **Check this machine** or **Check again**,
**Record ownership**, **Release ownership**) and in the confirmations; the fixed footer holds **Done**.

**Focus and Escape.** One walk through the dialog is made at 1360 light, 768 dark, 390 light and 320
dark, opened by a real click on the scope's button. Escape in the record confirmation closes only that
confirmation, returns focus to **Record ownership** and sends nothing; **Cancel** does the same;
**Confirm and record** sends one request and focus moves to the Check step's heading because the
button has gone. The same three ways out of the release confirmation leave focus on **Release
ownership** (cancelled) or the records heading (released). Escape on the dialog closes it, writes
nothing and returns focus to the opener, and so does **Done** on a second opening.

**Reduced motion.** The 320px walk runs with `prefers-reduced-motion: reduce`. In every state, and with
both confirmations open, no animation in any open dialog is running and no element in an open
dialog has an animation or a non-zero transition in its computed style. Without the preference the
buttons do transition, so the check is not vacuous; removing the no-preference guard from the
confirmation's entrance animation, as a deliberate break, made the fixture fail.

**Runs.** Four consecutive local runs of the fixture passed against the production build (126, 120,
127 and 123 seconds; the file took about 80 seconds before the matrix). An earlier run of the first
version of the matrix hit a race in an older check, the one that moves the browser clock on by a
minute while the tab is hidden: a read that was already under way landed after the clock had moved
and was stamped as fresh. The fixture now waits for that read before moving the clock
(`settleReads`), and the four runs above are after that fix. The machine-import, control and
public-workspace fixtures also passed.

**Accessibility scan.** `axe-core` 4.10.2 was installed with `npm` in a temporary directory outside
the repository (nothing was added to `package.json` or the lock file) and run through Playwright over a
copy of the same walk, scoped to the topmost open dialog, with the `wcag2a`, `wcag2aa`, `wcag21a`,
`wcag21aa` and `wcag22aa` rule tags. That is 40 scans: the ten states above at 375 and 1440px in light
and dark. With transitions off (reduced motion) there were no violations, and the same with
transitions on and a 600 millisecond wait before each scan. A first pass without that wait reported
`color-contrast` on buttons in 17 of the 40 scans, which disappeared once the transitions had
finished; axe was reading colours part-way through a 150 millisecond transition, so I treat those as
a timing artefact of the scan and not as a contrast defect, though I did not measure the contrast
ratios by hand. Axe also reported `color-contrast` as "incomplete" (it could not decide) in 34 to 36 of the 40
scans and `th-has-data-cells` in 4, and these were not reviewed further. The scan is not a
CI gate and the script is not part of the repository.

**Not checked.** No physical phone or tablet, no screen reader, no Safari or Firefox: only Chromium,
driven by a fixture that answers every API call with synthetic data. The fixture proves what the page
does with those answers, not what a connector or the server would send. Touch target sizes were not
measured. No production data, machine, terminal session or deployment was involved.

## Add a machine in context: layout, focus and motion matrix — 25 October 2026

Built on main `6675d2d` plus the Private skills entry and landing commit `186bbe9`. This records what
the browser fixture `web/tests/add-machine.e2e.mjs` checks for the
[Add a machine dialog](add-machine-in-context.md), what an accessibility scan found, and what was not
checked. The matrix found no layout or focus defect. Three small fixes from earlier reviews went in
first, each with a fixture assertion that failed before the change: the landing now judges the folder
again at the instant it opens the Private skills dialog (a folder that has stopped qualifying opens
nothing and the page says "Private skills can't open on that folder right now."); focus falls back to
the **Add a machine** button when the folder's own button is not drawn; and the landing note goes into
a status region that is already on the page. Two comments (why a context with neither a skill nor an
agent is a Machines-page context, and why the automatic-continue timer is tagged) and two unit
assertion on that context were added too.

**Layout matrix.** Ten states of the dialog, each at 1360, 768, 390 and 320px wide in light and dark
(eight size and theme pairs per state), and the sign-in card the page shows when the sign-in is lost
mid-watch (the dialog's own signed-out sentence is replaced by that card, so the card is what is
checked): loading; the name's validation message; waiting; connected with a paused connector;
waiting for the first scan; ready; still waiting at the 15-minute cap; already here, with the note
that says what an alias was saved under; ready from the Private skills dialog with the three-second
continue notice and its **Cancel**; and the Private skills dialog's landing note. A name of about 100
unbroken characters is used wherever the command block shows. For each state the page does not scroll
sideways, the dialog sits inside the window with neither it nor its body scrolling sideways, the
command block wraps inside its container, and each primary action (**Use**, **Check again**,
**Cancel**, **Copy commands**, the name field or the close button, whichever the state offers) is
scrolled into view and a hit test at its centre lands on it. The dark surface is checked to differ
from the light one, so the scheme change is real. Breaking the block's wrapping on purpose made the
fixture fail at the first size.

**Focus and Escape.** At each of the eight size and theme pairs the dialog is opened by a script
click (no focus moves, as on Safari and Firefox on a Mac), the name is typed and Escape is pressed:
the dialog closes, focus is on **Add a machine**, and no request other than a read was sent. The
same is done from the Private skills dialog through **Not listed? Add a machine**: Escape returns focus to
the folder's button. Closing with the button and the Use landings are checked in the earlier parts of
the file.

**Reduced motion.** With `prefers-reduced-motion: reduce`, no element in the open dialog has an
animation or a transform transition with a non-zero duration, and no animation or transform
transition in it is running (the copy button still fades its opacity over 100 milliseconds, which is
allowed). Landing a new machine from Machines then scrolls with `auto` instead of `smooth` (the same
landing without the preference scrolls `smooth`), and the card keeps its highlight as a still ring
and a changed border colour, with no animation on it.

**Runs.** Four consecutive local runs of the fixture passed against the production build (53, 52, 56
and 52 seconds; the file took about 34 seconds before the matrix), with
`PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/opt/pw-browsers/chromium` and the preview server on port 4173.
The skill-content, machine-import, control and public-workspace fixtures also passed. Full validation
results are recorded in the pull request.

**Accessibility scan.** `axe-core` 4.10.2 was installed with `npm` in a temporary directory outside
the repository (nothing was added to `package.json` or the lock file) and run through Playwright over
a copy of the same walk, scoped to the topmost open dialog (the page for the sign-in card), with the
`wcag2a`, `wcag2aa`, `wcag21a`, `wcag21aa` and `wcag22aa` rule tags and a 700 millisecond wait before
each scan so that transitions had finished. That is 44 scans: the eleven surfaces above at 375 and
1440px in light and dark. There were no violations. Axe reported `color-contrast` as "incomplete" (it
could not decide) in 40 of the 44 scans, and I did not review those or measure the contrast ratios by
hand. The scan is not a CI gate and the script is not part of the repository.

**Not checked.** No physical phone or tablet, no screen reader, no Safari or Firefox: only Chromium,
driven by a fixture that answers every API call with synthetic data. That a screen reader announces
the landing note was not heard; the note is only shown to arrive after its region exists. The landing's
second look is exercised by moving the browser clock on past the 14-day window between pressing
**Use** and the page drawing the landing, not by pausing a connector in the same instant. Touch target
sizes were not measured. No production data, machine, terminal session or deployment was involved.

## Start an agent session: layout, focus and motion matrix — 25 October 2026

This section was built on `f7e47ae` plus the hardening commit `0d09906` and the matrix commit
`721988a`. It records three things for the [Start an agent dialog](start-agent-session.md):

- what the browser fixture `web/tests/start-agent.e2e.mjs` checks;
- what an accessibility scan found;
- what was not checked.

**Defects found and fixed.** The matrix and the scan found three defects. Each fix has a fixture
assertion that failed before the change.

1. **The footer covered fields on narrow screens.** The footer with the status line and **Start**
   was sticky inside the scrolling form. At 390 and 320px, a field scrolled into view (the Folder
   select first) sat underneath it, so a hit test at its centre landed on **Start**. The footer now
   sits below the scrolling fields, outside the scrolling part.
2. **The "Not installed on this machine." reason failed contrast.** It sat in a row faded to 75%
   opacity, and axe reported a `color-contrast` violation in 21 of the dialog scans. Now only the
   agent's name is dimmed, and the reason keeps full contrast.
3. **The locked form couldn't be scrolled by keyboard.** While a request was held or its outcome
   unknown, every field in the scrolling part was disabled, so a keyboard user couldn't reach it to
   scroll (axe `scrollable-region-focusable`, 12 dialog scans). While locked, it is now a focusable
   region named "Your choices".

An earlier review had also found that a validation message stayed in the status line after the
field was fixed. It is now cleared as soon as a field is edited, and replaced on the next attempt.

**Layout matrix.** 24 states, each at 1360, 768, 390 and 320px wide in light and dark (eight size
and theme pairs per state):

- both closed entry points (the Terminals sidebar's **New agent session**, and the machine cards'
  **Start an agent**, including a disabled one);
- the dialog as it opens, with OpenCode not installed;
- a machine with a 60-character unbroken name and a project folder of about 130 characters, with
  three agents not installed;
- the sentence line with a prompt (the process-list line);
- the machine you came from not ready;
- no machine ready;
- an old machine;
- **Other…** with its message;
- a typed relative path with its message;
- a 2,001-byte prompt with its counter;
- the leading-dash rule in the status line;
- **Starting…** while held;
- the lost reply with **Try the same request again**;
- the landing on the new session;
- the nine refusals: folder, agent not installed, one-word prompt, update reet, session limit,
  request changed, access refused, terminals switched off on the server, and reconnecting (kept for
  retry).

For each state:

- the page does not scroll sideways;
- the dialog sits inside the window;
- neither the dialog, its scrolling part, nor any element inside it reaches past the dialog's edges;
- each primary action for that state is scrolled into view, and a hit test at its centre lands on
  it. The primary actions are **Start**, **Try the same request again**, **Starting…**, **Close**,
  the field the message is about, the status line, the entry buttons, or the session pane.

For the long names, the machine and folder selects show what fits. The fixture checks that the
whole value is the selected option's text and that the sentence line, which wraps, holds both in
full. The dark surface is checked to differ from the light one, so the scheme change is real.

Three mutants made the fixture fail:

- `white-space: pre` on the sentence line;
- a fixed `min-width: 640px` on the dialog;
- putting the sticky footer back.

Removing only the `min(…)` cap did not make it fail, because the browser's own dialog style still
caps the width.

**Focus and Escape.** At each of the eight size and theme pairs, the dialog is opened from the
Terminals sidebar and then from the long-named machine's card. A prompt is typed and Escape pressed.
The dialog closes, focus returns to the button that opened it, and no request is sent. Earlier parts
of the file check **Close**, Escape on a held first **Start** and on a held retry (both wait), and
focus after each refusal.

**Reduced motion.** Without the preference, the dialog's buttons have a press (transform)
transition, so the check is not empty. With `prefers-reduced-motion: reduce`, no element in the
open dialog has an animation or a transform transition, and none is running.

**Storage.** During the matrix, nothing was written to browser storage under a key that hadn't
already been written before it: only the workspace's layout preference and the sign-in token's
clean-up. Nothing about a prompt, model or folder was written. The whole file also records IndexedDB
opens, deletes and store writes, and finds none apart from its own recorder self-check.

**Runs.** Four consecutive local runs of the fixture passed against the production build (50, 50,
47 and 49 seconds; the file took about 8 seconds before the matrix), with
`PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/opt/pw-browsers/chromium` and the preview server on port 4173.
The public-workspace, control, machine-import, skill-content, add-machine, terminal-workspace,
terminal-lifecycle and terminal-organisation fixtures also passed. Full validation results are
recorded in the pull request.

**Accessibility scan.**
- **Setup.** `axe-core` 4.10.2 was installed with `npm` in a temporary directory outside the
  repository. Nothing was added to `package.json` or the lock file.
- **How it ran.** It ran through Playwright over a copy of the same walk, with the `wcag2a`,
  `wcag2aa`, `wcag21a`, `wcag21aa` and `wcag22aa` rule tags. Each scan waited 700 milliseconds
  first and covered the open dialog, or the whole page for the closed entry points and the landing.
- **Scope.** 96 scans: the 24 states at 375 and 1440px in light and dark.
- **The dialog, after the fixes.** 84 dialog scans, no violations.
- **The page.**
  - In 4 of the 12 page scans (the Terminals page and the landing, at 375px), axe reported
    `scrollable-region-focusable` on the terminal pane's mobile key row, `.mobile-keys`. That is
    outside this dialog and was not changed.
  - Axe reported `aria-prohibited-attr` as "incomplete" in all 12 page scans: on the terminal
    toolbar and the machine cards' agent chips. I did not review those.
- **Contrast.** Axe reported `color-contrast` as "incomplete" (it couldn't decide) in all 96 scans.
  I did not review those or measure the contrast ratios by hand, so contrast is not shown to pass.
- **Status.** The scan is not a CI gate, and the script is not part of the repository.

**Not checked.**
- No physical phone or tablet, no screen reader, no Safari or Firefox. Only Chromium, driven by a
  fixture that answers every API call with synthetic data.
- Whether a screen reader announces the status line or the locked region's name was not heard.
- Touch target sizes were not measured.
- Landscape phone sizes were not in the matrix.
- No live machine, agent CLI, production service or deployment was involved, so nothing here shows
  an agent starting.

## Phone terminal: keyboard, size and repaint — 26 October 2026

This section was built on main `188236e`. It records what the new browser fixture
`web/tests/terminal-phone.e2e.mjs` checks for the [phone terminal rules](terminal-workspace.md#phones),
how the rules were mutation-checked, and what was not checked.

**Starting point (same synthetic fixture, 390 × 844, keyboard shrinking the visual viewport to 464).**
41 × 10 sent with the keyboard open, 42 × 14 maximised; 14 px text. Every keyboard animation frame
sent a resize, identical sizes were resent, and the size could drop to 1 row.

**What the fixture checks.**
- **Phone (390 × 844, touch).**
  - No stored font gives 11 px and a ticket of at least 50 columns (53 here); a stored 16 still wins.
  - The pane header, terminal toolbar and key row are each at most 42 px tall. Every toolbar button
    and extra key is at least 40 × 40 px (fix round; at first only the heights were 40 px), and the
    header controls are 40 px tall. The user/host stays in the header.
  - Both sideways rows overflow, carry the end fade, fade at the start once scrolled, and have
    `overflow-x: auto`. The key row takes focus and scrolls with the arrow keys.
  - The Alt+Shift+M advice is in the DOM but hidden when maximised; Show sessions stays in the
    terminal toolbar.
  - Keyboard open while typing in the terminal: the site header, the workspace toolbar and the pane
    footer are hidden, and Show sessions is visible. A ten-step keyboard animation sends exactly one
    resize (53 × 24 here, at least 15 rows asserted); the pane ends 0–10 px above the keyboard; the
    same height again sends nothing; closing the keyboard brings everything back.
  - Maximised with the keyboard open: at least 15 rows (55 × 25 here), the pane ends within 2 px of the
    keyboard, and the renderer fits.
  - The page shrunk for the keyboard (`resizes-content`) while the visual viewport reports 91 px less:
    the pane still ends at the keyboard.
  - A 330 px tall screen sends 53 × 8: every frame is at least 20 × 8 and the last row (the agent's
    input line) stays visible.
  - Returning to the page sends exactly two frames on the same socket, one row shorter and then the
    real size. A reattach does the same, ending on the fitted size. The first attach sends no nudge.
  - Fix round: a shrinking viewport with the focus outside the terminal keeps the header; at the
    floor the return nudge sends 9 then 8 rows; a second return within 5 s sends nothing; the
    viewport meta has `interactive-widget=resizes-content` on the terminal page only (`/`,
    `/hosting` and `/machines` keep the default).
- **Viewport readout (`?viewport-debug=1`).** Shows `innerHeight 844` and `keyboard closed`, then the
  shrunk `visualViewport.height`, the used height and `keyboard open` while the terminal keeps the
  focus; `pointer-events: none`; nothing in browser storage; absent without the parameter.
- **Sideways phones (844 × 390 and 915 × 412, touch).** 11 px; the toolbar and extra keys share a row
  and the footer is hidden; the toolbar fades and scrolls to its last control (Maximise). Visible
  rows: 13 and 15 with the keyboard closed (at least 12 asserted), 8 and 8 with it open (at least 6),
  9 and 10 maximised with it open; the site header and pane header step aside while typing; the pane
  ends at the keyboard; every frame is at least 20 × 8.
- **1360 × 900 and 768 × 900 (mouse).** 14 px text; the pane header, toolbar and screen frame keep
  their padding (`7px 12px`, `5px 8px`, `12px 10px 8px 14px`); no fade; the key row stays hidden; the
  footer is visible; the maximised pane sits 12 px from each edge; the Alt+Shift+M advice shows. A
  shrinking visual viewport while typing does not start the keyboard layout. A 390 × 844 window
  without touch keeps its header too. At 1360 × 900 a quick return to the page sends no frame; a
  return after 31 s (simulated clock) sends the two-frame nudge.
- **Before and after, measured.** A separate probe measured the site header, workspace toolbar, pane,
  pane header, toolbar, screen frame, screen, footer and the size sent, normal and maximised. At
  1360 × 900 and 768 × 900 every number was identical before and after. A 768 × 1024 touch tablet
  differed only by the hidden Alt+Shift+M advice (31 px), which gave the maximised screen two more rows.

**Mutation checks.** Each rule was broken in the source, the bundle rebuilt and the fixture run
against it; the unit tests in `web/tests/terminal-sizing.test.mjs` were run too. All 14 mutants were
caught by the fixture; 6 were also caught by the unit tests.

| Rule broken | Caught by |
| --- | --- |
| Floor at 1 row, 2 columns | unit, fixture (8-row floor) |
| No debounce | fixture (one frame per keyboard animation) |
| Identical sizes resent | unit, fixture (same height sends nothing) |
| No repaint on return | fixture (two frames on return) |
| Nudge on the first attach too | fixture (first attach) |
| Visual viewport height always used | unit, fixture (under-reported viewport) |
| Advice shown on touch | fixture |
| 14 px on phones | unit, fixture |
| Site header not hidden / workspace toolbar not hidden | fixture (each) |
| Keyboard layout without touch | unit, fixture (narrow window without touch) |
| Rows keep their padding | fixture (row height) |
| No fade | fixture |
| Floored grid not shifted to show its last rows | fixture (last row visible) |

**Fix-round mutation checks.** 13 more mutants, all caught by the fixture (4 also by the unit tests):
nudge below the floor; return gate ignoring touch; return always nudging on desktop; no cooldown;
keyboard layout without terminal focus; no sideways compact rules; no shared tool row; pane header
kept while typing sideways; keyboard layout portrait-only; 30 px toolbar buttons; readout taking
touches; readout always on; viewport meta global again. Resizing the grid once at the floor
(`proposeDimensions`) has no visible effect and was not mutation-checked.

**Runs.** The full fixture list of `.github/workflows/terminal-browser.yml` ran twice against the
production build, including the terminal workspace, lifecycle, organisation and phone fixtures. The
existing terminal-workspace bounds held: 390 × 844 now has a 576 px screen (was 529) and 390 × 420
has 152 px (was 105). The real browser, broker and tmux test
(`TestTerminalPrivateLifecyclePreservesActualShell`) passed twice with a UTF-8 locale; it now counts
the attachment ticket as a sent size and waits for the debounced phone resize. Without a UTF-8 locale
its emoji step fails in this container on main as well. After the fix round, the 20 fixtures again
passed twice each (one pass took 532 s locally, the phone fixture 23 s of it) and the real test passed
twice.

**Not checked.**
- No physical phone. The keyboard is simulated by replacing `visualViewport` or resizing the page;
  whether a given Android browser honours `interactive-widget=resizes-content`, and the toolbar
  under-report itself, need the device check in `handoff.md`.
- No real agent: the repaint nudge was checked on the wire and, through the existing real tmux
  fixture, against a shell; whether a particular agent repaints in full after it is a device check.
- Sideways phones: only the terminal is covered; dialogs and other pages are not.
- The viewport meta is restored when the workspace unmounts, but the app loads each page in full, so
  that path is not reached by the fixture.
- No accessibility scan was run for this change; no Safari, Firefox or screen reader.

## Projects: layout, focus and motion matrix — 26 October 2026

*Historical: the Projects page and its dialogs were retired in PR B (task B6). `projects.e2e.mjs` now checks the
redirects and the read-only runs in Activity; the rail that replaced the page is recorded under "Folder-project rail"
below.*

This section was built on main `cfde296` (Projects PR 3) plus the matrix commit `7d95f48`. It
records what the browser fixture `web/tests/projects.e2e.mjs` checks for [Projects](projects.md)
beyond the behaviour already in it, what an accessibility scan found, and what was not checked.

**Defects found and fixed.**

1. **The Remove worktree details couldn't be scrolled by keyboard.** The dialog's middle part (the
   folder, counts and up to 20 changed paths) scrolls on a small screen but held nothing focusable,
   so a keyboard user couldn't reach it to scroll (axe `scrollable-region-focusable`, 6 dialog
   scans). While it scrolls it is now a focusable region named "Worktree details", with its focus
   ring drawn inside it (outside, the dialog clipped its sides and the footer covered its bottom, a
   review found; the locked Start all and Project forms had the same ring and got the same fix). When
   everything fits it is no tab stop. The fixture presses Tab from **Close** at 320px and expects
   that region focused (it failed before the change), checks the ring's computed offset is inside,
   and checks there is no region or tab stop when the confirmation fits at 1360px. A rerun of the axe
   scan after this change found the same results as below.
2. **The fixture's dark scheme was not dark.** Its synthetic profile saved the theme `light`, which
   pins the light colours whatever the system scheme says, so the earlier "320px in light and dark"
   check ran in light twice. The profile now follows the system scheme, and the matrix checks that the
   page and dialog surfaces differ between the two schemes.

**Layout matrix.** 19 states, each at 1360, 768, 390 and 320px wide in light and dark (eight size
and theme pairs per state), changed under the open page or dialog without a reload:

- the Projects list empty, and loaded with an 80-character unbroken Project name;
- **New project** as it opens, and with a repository on a machine with a 60-character unbroken name
  and a 130-character folder, a typed relative path and the refusal Save gives;
- the Project page with a run whose five repositories are started (one with that long machine and
  folder), failed (attempt 3), removed and pending, with **Open**, **Retry** and **Remove worktree**;
- **Change** with five repositories (one on a machine no longer on the account), and its 409
  `project_changed` with **Reload the Project**;
- **Start all** with every reason shown and **Add a machine**, a lost reply with **Try the same
  request again**, and a 409 `run_in_progress` refusal;
- **Remove worktree** blocked by every check at once (23 changed files with 20 long paths shown,
  commits on no branch, hidden edits, a nested repository, an incomplete check), its confirmation, and
  git refusing the check with its detail line;
- **Delete** and its 409 `project_changed`;
- a Project page that couldn't be read (503), one not on the account, one just deleted, and the
  read-only account's notice.

For each state and pair:

- the page does not scroll sideways;
- an open dialog sits inside the window, and neither it, its scrolling part, nor any element inside
  it reaches past its edges;
- each primary action for that state is scrolled into view and a hit test at its centre lands on it
  (for a link that wraps, the centre of its first line). The primary actions are **New project**, the
  Project link, **Start all**, **Change**, **Delete**, **Retry**, **Remove worktree**, **Open**, the
  dialogs' Save, Start, Remove, Check again, Reload, Close and Keep it buttons, **Add a repository**,
  **Add a machine**, the field a message is about, the status line, the error notice and **Back to
  Projects**, whichever the state has. A box may sit up to 1px past the window's edge after
  scrolling.

Five mutants each made the fixture fail: no `overflow-wrap` on a run member's lines, no
`overflow-wrap` on a Project card, a `min-width: 640px` on the Start all sheet, an entrance
animation on that sheet without the reduced-motion guard, and the Delete dialog not returning focus
on close. The first three were also caught by the fixture's earlier 320px checks; with those switched
off in a copy, the matrix caught each of them.

**Focus and Escape.** At each of the eight size and theme pairs, **New project** (with a name typed),
**Change**, **Start all**, **Delete** and **Remove worktree** (once its check has answered) are opened
and closed with Escape. Each dialog closes, focus returns to the button that opened it, and nothing
but reads is sent. The openers are pressed by a script click with nothing focused, as a click on
Safari, or on Firefox on a Mac, focuses no button, so the returned focus is the dialog's own doing:
Chromium returns focus to what was focused before a modal opened, which hid the focus mutant above
until the click was changed.

**Reduced motion.** Without the preference, the buttons have a press (transform) transition, so the
check is not empty. With `prefers-reduced-motion: reduce`, nothing on the Project page (with a run
open) and nothing in any of the five dialogs has an animation or a transform transition, and no
animation is running.

**Storage.** During the matrix nothing was written to browser storage under a key not written before
it, and nothing holding the long Project name, a folder or a Project id.

**Runs.** Four consecutive local runs of the fixture passed against the production build (131, 129,
128 and 130 seconds; the file took about 98 seconds before the matrix, most of it two deliberate
15-second read timeouts), with `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/opt/pw-browsers/chromium` and
the preview server on port 4173. The navigation and start-agent fixtures also passed.

**Accessibility scan.**
- **Setup.** `axe-core` 4.10.2 was installed with `npm` in a temporary directory outside the
  repository. Nothing was added to `package.json` or the lock file.
- **How it ran.** Through Playwright over a copy of the same walk, with the `wcag2a`, `wcag2aa`,
  `wcag21a`, `wcag21aa` and `wcag22aa` rule tags, at 375 and 1440px in light and dark. Each scan
  waited 700 milliseconds first and covered the topmost open dialog, or the whole page.
- **Scope.** 76 scans: the 19 states at those four pairs (48 dialog scans, 28 page scans).
- **Before the fix.** 6 violations, all `scrollable-region-focusable` on the Remove worktree
  dialog's details (blocked at 375 and 1440px, and the confirmation at 375px), fixed as above.
- **After the fix.** No violations.
- **Incomplete.** Axe reported `color-contrast` as "incomplete" (it couldn't decide) in 62 of the 76
  scans, 298 element results in all: the element's background "partially overlaps other elements"
  (108) or is "partially obscured by another element" (176), or its text is "too short to
  determine" (14).
  I did not review those or measure the contrast ratios by hand, so contrast is not shown to pass.
  Nothing else was incomplete.
- **Status.** The scan is not a CI gate, and the script is not part of the repository.

**Not checked.**
- No physical phone or tablet, no screen reader, no Safari or Firefox. Only Chromium, driven by a
  fixture that answers every API call with synthetic data that imitates the Go handlers; it proves
  what the pages do with those answers, not what a server or machine would send.
- Whether a screen reader announces the status lines, the run summary or the region's name was not
  heard.
- Touch target sizes were not measured. Landscape phone sizes were not in the matrix.
- A Project with 12 repositories, or 50 Projects, was not in the matrix.
- No live machine, agent CLI, git repository, production service or deployment was involved, so
  nothing here shows an agent starting or a worktree being removed.

## Phones: long names and page zoom — 26 October 2026

Built on main `a97b28c`. A phone audit found that one long host, path or skill name made the
browser lay a page out wider than the screen and zoom the whole page out. The document then reports
`scrollWidth == innerWidth`, so a plain "no horizontal overflow" check passes. Only a mobile browser
context (`isMobile`, which honours the viewport meta tag) shows it.

**Before (same fixture, Chromium mobile emulation).** The list page was laid out 581 px wide (0.55
scale at 320 px, 0.71 at 412). Machines and every dialog opened from it: 401 px, 511 px with
"Agents on this machine" open. Admin environments with a machine open: about 491 px (0.65–0.84).
Sync cut off "Review local & web changes", "Preview" and "Stop tracking" with `overflow: clip`.

**What the fixture checks.** `web/tests/phone-zoom.e2e.mjs`, with the fake API in
`web/tests/helpers/longtext.mjs`: a 57-character host and a 70-character path, plus long skill,
session, token and tag names. None contains a hyphen, because a browser may break a line after
one. 36 screens: control, machines and its start-agent, settings and commands dialogs, a machine's
own page, sync, skills, setups, lists and a list, projects and a project, hosting, spaces, support
and a ticket, notifications, eight account pages and ten admin pages. Each runs at 320, 360, 390 and
412 px with `isMobile` and `hasTouch`, first with every `<details>` closed and then open. It asserts:
- `innerWidth` is the device width and `visualViewport.scale` is 1;
- every control is on the screen, or inside a sideways scroller that shows there is more (an item
  peeking past its edge, or a fade mask);
- no control is cut off by an `overflow: hidden` or `clip` box, runs out of the bordered box around
  it, or has a label more than 8 px wider than itself;
- the long skill name fits its heading once its card is expanded.

It takes about 30–40 s and runs as its own `phone-zoom` job in `terminal-browser.yml`.

**Fixes.** List page add-skill form (grid column `minmax(0, 1fr)`); machines agent chips, scan-only
agent names, setup steps, setup card and command box clipping, upgrade and refresh copy buttons;
sync target card (no clip, wrapping names, shrinkable column); admin environments scope rows; shelf
filter chips; account token form and token names; active session cards on the control page (at
most 85% of the strip, so the next one peeks in at 320 px).

**Mutation check.** Each fix was put back as it was, the app rebuilt and the fixture run on the
affected screens. All 15 mutants below failed the fixture except one:
- list form column; machines agent chip; scan-only agent name; refresh copy label; setup steps;
  upgrade copy button; sync card (clip, column and name together); sync scope name alone; admin
  environments rows; active sessions peek; skill heading wrap; shelf filter chip; account token row
  column; account form column — all failed it.
- Token names wrapping (`.rows strong`) did not: without it the name overflows its row as text, and
  the fixture checks controls, not text.

**Global default not added.** An `overflow-wrap: anywhere` default for headings, `code`, `kbd` and
badges in `app.css` was tried. Comparing every element's box at 1360 and 768 px with and without it,
table columns moved by 1–5 px and two small notes changed height on the account and admin backup,
email, users, sessions and tokens pages. No screen in the fixture needed it, so it was left out.

**Desktop.** Element boxes on the same 36 screens at 1360 and 768 px, before and after, differ only
where a long name used to run out of its box: the shelf filter chip (its label ran over the next
chip; it now wraps inside it), admin environment scope rows and the machine cards' scope names (now
wrap). Other differences were within the 1 px noise seen between two runs of the same build.

**Not checked.** No physical phone, Safari or Firefox; Chromium only, with synthetic API answers.
Terminals, project dialogs, sign-in pages and sideways phones are not in the matrix. Text that
overflows without a control in it (as with the token name above) is not detected.

## Phones: dialogs — 26 October 2026

Phone audit slice 2, on `8c4fa56`. Every modal dialog now uses one shared shell
(`web/src/lib/dialogShell.ts` and `web/src/dialog-shell.css`, loaded from `main.ts`).

**Before (Chromium mobile emulation, same fixture).** Scrolling inside a dialog scrolled the page
behind it (scrollY 0 → 2479 at 360 px), so pull-to-refresh could fire. With the keyboard open, the
pinned footer covered the field being typed into (New list name, Start an agent prompt, project
name). Held sideways (844 × 390, 915 × 412) a dialog left 122–190 px for its content. Dialogs were
centred cards on a 320 px phone, and close buttons were 38 px. The new contract, run against the
old build, failed in every category: among others, at least 118 unlocked pages, 61 page scrolls
(0 → 3231 behind New project at 844 × 390), 89 scroll chains, 118 non-sheets, 41 hidden focused
fields, 28 dialogs taller than the part above the keyboard, 18 unreachable main actions and 21
close buttons under 44 px; sideways, New project and Start an agent had 122 px of content. After,
the smallest content area sideways is 206 px (terminal workspace at 915 × 412).

**The shell, for a new dialog.** A dialog keeps its own markup, frame, focus return, Escape
handling, in-flight guard and labelling. To adopt the shell:
1. `import { shell } from './dialogShell'` and put `use:shell` on the `<dialog>`. The default is a
   full-screen sheet on an upright phone; pass `use:shell={{ sheet: 'bottom' }}` for a short
   confirmation, which sits on the bottom edge instead.
2. Mark its parts with attributes (all optional):
   - `data-shell-head` — a header that carries its own padding (slimmed on a short screen);
   - `data-shell-body` — the part that scrolls;
   - `data-shell-foot` — the footer with the main action (one row on a short screen; pinned to the
     bottom edge when it is `position: sticky`);
   - `data-shell-card` — the visible card, when the `<dialog>` itself is transparent around it;
   - `data-shell-aside` — helper text that a short screen hides (eyebrows, ledes, reassurance);
   - `data-shell-close` — the close control.
3. Add the dialog to `DIALOGS` in `web/tests/phone-dialogs.e2e.mjs`, with `field` if it has a
   text field and `sheet` (`'full'` or `'bottom'`).

What it guarantees, on a phone or touch screen (`pointer: coarse`, or narrower than 560 px):
- the page behind does not scroll while any dialog is open (`html:has(dialog[data-shell][open])`
  gets `overflow: hidden`), and scrolling inside a dialog never chains to the page
  (`overscroll-behavior: contain` on the dialog and its body);
- under 560 px wide the dialog is a full-screen sheet (or a bottom sheet), with the safe-area
  insets as a border in the card's colour (0 until the page sets `viewport-fit=cover`);
- the close control is at least 44 × 44 px;
- with the keyboard open, the dialog fits the part of the screen above it (the visual viewport, for
  a browser that only shrinks that), and the focused field is scrolled into the visible part of
  each scrolling region: below a pinned header, above a pinned footer and above the keyboard, on
  focus and whenever the visible area changes size;
- on a short screen (500 px tall or less, or the keyboard open, whether the browser shrank the
  visual viewport or resized the page) the header and padding are slimmer, `data-shell-aside` text
  is hidden, and the footer is one row (a status line takes its own row when there are two
  buttons or more).
A desktop with a mouse is unchanged, unless its window is 500 px tall or less.

**What the fixture checks.** `web/tests/phone-dialogs.e2e.mjs`: 20 cases (15 dialogs, New list
twice, once typing into a field low in the form, and the terminal's appearance, history, saved
commands and command editor drawers, where the last control in each must also be reachable) at 320 × 568, 360 × 640, 390 × 844 and 412 × 915
upright and 844 × 390 and 915 × 412 sideways, `isMobile` and `hasTouch`, with the page made 3000 px
longer. At rest: the page does not move under wheel and touch scrolls over the body and the header,
the root has `overflow: hidden`, no scrolling part chains, the dialog fits, upright it is the sheet
it should be, Close is at least 44 × 44 px and hit-testable, the main action can be scrolled to and
is hit-testable, and sideways the scrolling part has at least 140 px (and 40% of the screen) and
the footer's buttons share a row when their own widths fit. With the keyboard simulated twice (the
page resized to 55% of its height, and only `visualViewport.height` shrunk to 55%) for each dialog
with a text field: the same, plus the focused field wholly in view and not under a pinned footer or
header. About 50–55 s; its own `phone-dialogs` job in `terminal-browser.yml`.
`REET_E2E_ONLY=<regexp>`, `REET_E2E_VIEWPORTS=320x568,844x390`, `REET_DIALOG_SHOTS=<dir>` and
`REET_DIALOG_LOG=1` narrow or explain a run.

**Mutation check.** Each rule was broken in turn, the app rebuilt and the whole fixture run; every
mutant failed it (failures in brackets):
- page lock (192), `overscroll-behavior: contain` (153), full-screen sheet (182), bottom sheet (16),
  44 px close (36);
- scrolling the focused field into view (61), keeping it clear of a pinned footer (4: New list's
  people field), the pinned footer's `bottom: 0` (28);
- fitting the dialog above the keyboard in CSS (30), detecting a keyboard over the visual viewport
  (92), detecting a keyboard that resized the page (2);
- the short-screen flag (49), hiding asides (8), slimmer header, body and footer (8), the one-row
  footer (16), a status line on its own row (2);
- New list without `use:shell` (200).

**Review round 1 (drawers).** Sideways, the short-screen rule raised the terminal drawers' height
limit to the full screen but kept their 76 px top inset, so a tall drawer ran to 466 px on a 390 px
screen and its bottom controls (Save command, Terminal output history) were out of reach. The
short-screen rule now also sets the vertical insets to the visible part (`--shell-top` and
`--shell-bottom`, 0 without the keyboard), so a dialog placed lower moves up. The four drawer cases
failed before the fix (24 problems, sideways at rest and with a page-resizing keyboard) and pass
after; removing the insets again fails them. No other dialog sets its own insets: all of them already
passed the "fits the visible part" check sideways. The fixture's clipping walk now stops at a modal
dialog, since the top layer is not clipped by the page around it (the drawers live inside the
terminal pane, which hides overflow).

**Desktop and tablet.** Every element box in each of the 15 dialogs at 1360 × 900 and 768 × 1024
with a mouse is the same before and after, and so are the screenshots, apart from the preview port
in Machine commands' command text and page-behind noise that also appears between two runs of the
old build. `overscroll-behavior` was first applied everywhere; it changed how dialog text was
painted (128 pixels of anti-aliasing) on desktop, so it is now limited to touch and narrow screens,
as is the pinned footer's `bottom: 0` (it moved New list's buttons 7 px on a desktop). On a touch
screen at those sizes the differences are the intended ones: the close control's 44 px box (and
the 6 px it takes from the title beside a 38 px icon button), the page lock, and New list's pinned
buttons.

**Not checked.** No physical phone, Safari or Firefox; Chromium only, with synthetic API answers
and a simulated keyboard. iOS before 16 ignores `overflow: hidden` on the root, so its page can
still be dragged from outside a scrolling part. Safe-area insets are 0 in the emulator. A page
resized for the keyboard before a dialog opens is not recognised as short until it changes size.

## Agents home shell and Home — 26 October 2026

Built on `6c0f1c1` (PR D, tasks D1–D4). This section records what the fixtures check for the app shell
(top tabs, bottom tab bar, bell, Jump, More) and [Home](terminal-workspace.md#home-and-the-tabs), what an
accessibility scan found, how the rules were mutation-checked, and what was not checked. Everything below ran in
Chromium (`/opt/pw-browsers/chromium`, Playwright 1.64.1) against the production build. Every API answer was
synthetic. Phones and tablets were Chromium's mobile emulation (`isMobile`, `hasTouch`). Nothing was checked on a
real phone, a real tablet, Safari or Firefox.

**Defects found and fixed.**

1. **The logo link was a 26 px target on touch screens** (104 × 26 at 834 px). It is now at least 44 px tall on a
   coarse pointer. The matrix below found it.
2. **Jump's results ran under a phone keyboard.** With the keyboard taking the screen below 400 px, the results
   list ended at 539 px at 320 px wide, so the last results could not be reached. The list's height is now capped at
   the room left in the visual viewport, and every result scrolls into view above the keyboard.
3. **Start an agent's fields were 14 px on phones** (machine, model, folder and prompt), so focusing one zoomed the
   page. The dialog's own `font: inherit` beat the 16 px rule in `app.css`. The dialog and New project now set 16 px
   themselves at 560 px and narrower. New project had the same rule; no fixture opens it on a phone.
4. **Two colour pairs were below AA.**
   - Dark theme, the pressed Liked filter: white on `--heart` (`#ff7a9c`) was 2.47:1. Its text is now `--bg`:
     7.63:1 in dark (`#14110f` on `#ff7a9c`) and 5.57:1 in light (`#f7f6f2` on `#be1f4a`). This also removes a
     literal colour from `App.svelte`.
   - Light theme, `--accent-text` on `--tint` (the numbered steps in Add a machine, the account avatar, hover rows
     in the session sidebar): `#c2410c` on `#fde8d8` was 4.37:1. The light `--tint` is now `#feefe3`, which gives
     4.60:1 (`--ink` 15.14:1, `--muted` 5.94:1). The cost is that a tinted selected state is closer to the card:
     1.11:1 against `--card`, where it was 1.17:1. Most selected states also change weight, border or an inset
     bar, but not all: the Sync scope option and the Hosting skill choice change border and tint only.
   - `theme.test.mjs` now checks `--ink` and `--accent-text` on `--tint`, and `--bg` on `--heart`, in both themes.
5. **A machine whose sync agent is paused was called "online".** It now says "paused", the word Control uses
   (`fleet.ts`), with a grey square dot. A paused machine that stopped reporting says "offline". This applies to
   the sync-agent fallback (single-token and open sign-ins) and beside a health report alike.

**Known gap, not fixed: field and button borders.** `--line-strong` draws the borders of text fields, selects and
`.btn`. Against the surfaces it sits on, it is 1.78:1 (dark, on `--card` `#1a1613`), 1.86:1 (dark, on `--bg`),
1.74:1 (dark, on `--card-raised`), 1.73:1 (light, on `--card` `#fffdf9`) and 1.62:1 (light, on `--bg`). WCAG 1.4.11
asks for 3:1 for the visual boundary of a control that has no other cue. These values are the spec's token table,
and fields also differ from the page by their fill and their placeholder, so changing them is a design decision for
the spec, not this pull request. The focus ring is the accent (`--accent` on `--bg`: 3.63:1 dark, 4.79:1 light;
3.47:1 on the dark `--card`). The tabs and bottom bar draw `--accent-text` (8.28:1 on the dark `--bg`).

**The matrix** (`web/tests/home.e2e.mjs`, "matrix: widths, themes, targets, focus, state words and motion"). It
covers Home and the shell at 1360 px with a mouse, 834 px as a touch tablet, and 390 and 320 px as a phone. Each is
checked in light and dark, switching the system scheme with the profile on "system" and checking that the page
background changes. At each width and theme:
- the page does not scroll sideways, `innerWidth` is the device width, and the visual viewport is not zoomed;
- every visible link, button, summary and Group by option in the header, the navigation and `main` is at least
  44 × 44 px on touch (24 × 24 px with a mouse), and the phone's **Start an agent** is at least 48 px tall;
- every machine names its status in its accessible name. One that is not online shows the word, and the online and
  offline dots differ in border or fill, not only colour. Each In flight row shows its state in words. Needs you
  shows its count as a number.

At each width, the first 30 tab stops each draw a ring: an outline, or an inset shadow on the control, its label or
Jump's field box. With no motion preference a press transition exists, so the check is not empty. With
`prefers-reduced-motion: reduce`, nothing in the header, navigation or `main` has an animation or a transform
transition, and no animation is running.

Other new cases:
- `home.e2e.mjs` "a paused agent that still reports reads paused": a single-token sign-in, a writer and a phone.
- `home.test.mjs` "a paused agent that still reports reads paused, and one that stopped reporting reads offline".
- `phone-layout.e2e.mjs` "Jump search with the keyboard open", at 320 and 390 px. Jump has 80-character unbroken
  session, Project and machine names. The field is 16 px, and the bottom bar steps aside. The eight results end
  above a 400 px keyboard. Each result, scrolled into the list, is at least 44 px tall and inside the screen. The
  page neither scrolls sideways nor zooms.
- `phone-zoom.e2e.mjs` has four new screens: Home, the More list over Home, Jump's results, and Start an agent
  opened from Home. Home has an 80-character machine, session and Project name (`web/tests/helpers/longtext.mjs`,
  `homeExtra`). `/control` keeps its own screen. Every screen is checked at 320, 360, 390 and 412 px, and now
  also checks that every text field is at least 16 px.

**Mutation checks.** Each rule was broken in the source, the bundle rebuilt where a fixture needed it, and the
named check run. Each file was then restored from a copy and compared byte for byte.

| Rule broken | Caught by |
| --- | --- |
| "No reading" for a null CPU (shown as 0%) | `home.e2e.mjs` "missing CPU reads No reading"; `home.test.mjs` |
| "skills in sync" without a check (the unchecked target ignored) | `home.test.mjs` (2 tests). The e2e fixture has no unchecked target, so it passed |
| A worktree figure without a source (added to each card) | `home.e2e.mjs` "no worktree figure without a source" |
| A literal colour in a component (`#ff0000` in `NeedsYou.svelte`) | `theme.test.mjs` "no literal colour in a component" (with a git base) |
| The 16 px field rule removed from `app.css` | `phone-zoom.e2e.mjs`: 576 problems across the account, admin and other screens |
| A session counted in the bell | `home.e2e.mjs` "Needs you says Nothing needs you and the bell shows no badge" |
| A paused agent called online | the new e2e case and unit test |
| No focus ring on the top tabs | the matrix (1360 px, "Home") |
| Jump's results not capped above the keyboard | "Jump search with the keyboard open" (before the fix) |
| A stray colour token in a palette block, or the old light `--tint` | `theme.test.mjs` "app.css matches TOKENS", "every text pair passes AA" |
| The `.woff2` registration removed from `embed.go` | `TestPackageRegistersTypesTheHostMayLack` |

**Test fixes from earlier reviews.**
- `theme.test.mjs`'s literal-colour check needs a git base. The terminal-browser CI job now checks out the full
  history (`fetch-depth: 0`), so the check diffs the branch against `origin/main`. Without a base (a shallow clone,
  or a fork with no `origin/main`), the test prints "no git base … so only Logo.svelte was checked" and still
  passes. The image and release workflows still clone shallow, so they check `Logo.svelte` only.
- "app.css matches TOKENS" now also fails on a colour token that a palette block sets but `TOKENS` does not list.
- The `.woff2` type test no longer depends on the host: the registration is checked directly, because a host with
  `/etc/mime.types` serves `font/woff2` without it.

**Accessibility scan.**
- **Setup.** `axe-core` 4.10.2 was installed with `npm` in a temporary directory outside the repository. Nothing
  was added to `package.json` or the lock file, and the script is not part of the repository. The scan is not a
  CI gate.
- **How it ran.** Through Playwright, with the `wcag2a`, `wcag2aa`, `wcag21a`, `wcag21aa` and `wcag22aa` rule
  tags, at 390, 834 and 1360 px in light and dark. The 390 and 834 px runs were touch emulation. Each scan
  waited 700 ms first. It ran after the fixes above.
- **States.** Six per width and theme, 36 scans in all:
  - Home (writer) and Home for a viewer;
  - the bell's panel open;
  - Jump with results;
  - More (the bottom sheet at 390 and 834 px, the account menu at 1360 px);
  - Start an agent opened from Home.

  Dialogs were scanned on their own. Every other state was scanned as the whole page.
- **Violations.** 2, both `target-size`, both on Jump's results at 390 px (light and dark). An In flight row under
  the open results list is "partially obscured" and has 4.8 px of space to its neighbour. The row is covered by
  the open list and cannot be reached until the list closes, so this was not changed.
- **Incomplete.** Axe reported `color-contrast` as "incomplete" (it couldn't decide) in 24 of the 36 scans, 192
  element results in all. The reasons were:
  - the background is overlapped by another element (134);
  - it is partially obscured (28);
  - the content is too short to decide (24);
  - the element partially overlaps others (6).

  These results include the Add a machine tile, Jump's results over the page, and the dialog and account menu
  text. I did not review them one by one. Contrast is shown only for the token pairs `theme.test.mjs` computes,
  not for every element on the page. Nothing else was incomplete.

**Runs.** These ran against the production build with the preview server:
- `npm run check`: all unit tests pass; svelte-check reports 0 errors and 0 warnings.
- `go vet ./...`.
- `go test ./web ./internal/api`.
- Every fixture in `.github/workflows/terminal-browser.yml`, plus `phone-zoom`, `phone-dialogs` and `home`.

Local times:
- `home.e2e.mjs` takes 24.6 s, which was 20 s before this change. It now runs in the `phone-zoom` job, not the
  terminal-browser job.
- `phone-zoom.e2e.mjs` takes 33 s, which was 29 s.
- `phone-layout.e2e.mjs` takes 5.8 s, which was 5 s.

**Not checked.**
- No real phone, tablet, Safari or Firefox. Every check above that names a phone or a tablet was Chromium's mobile
  emulation, with a simulated keyboard (a replaced `visualViewport`). Safe-area insets are 0 in the emulator.
- No screen reader. Whether the status words, the live sentences or the bell's count are announced was not heard.
- Landscape phones were not in the matrix.
- The contrast of every rendered element was not measured. The tinted selected states were not compared by eye
  after the `--tint` change.
- No live machine, agent CLI, production service or deployment was involved, so nothing here shows a machine
  reporting, a session starting or a sync check running.

## Folder-project rail — 27 October 2026

PR B, task B6, on top of `d52c8d1`. Home's rail now lists folder projects, Add opens the folder browser, and the
Projects page is retired. The mock-up is `docs/design/agents-home/Main.dc.html` (desktop) with
`ref-home-desktop.png`; `Tablet.dc.html` and `PhoneHome.dc.html` draw no rail on Home (it lives on the Sessions tab,
PR H), and `PhoneRail.dc.html` is PR H's. Fidelity audit item P-3 (rail) is folded into this task. Rendered in
Chromium against the production build with synthetic answers shaped like the mock-up: machines Elitebook, Server
and Desk; projects reet, jiggered, zoomies-platform (`develop`), wireguard.sh and dynamicweb; four sessions, three
in reet and one in zoomies-platform, each in a worktree. Boxes from `getBoundingClientRect`, type and colour from
`getComputedStyle`; dark and light. Nothing was checked on a real phone or tablet, Safari or Firefox.

**1440 × 900 (desktop): matches, with the deviations listed.**
- Column: the rail is 301 px (272 px cards inside 14 px padding and a 1 px rule, as the audit's R1 measured the
  reference); its rule is at x = 300 in `--line-soft`, and the main column's first card starts at x = 325, as drawn.
- Heading row at y = 79: "PROJECTS" 600 11 px Plex Sans, .09em, `--muted`; **Add** 66 × 30, radius 8, 13 px / 500,
  border `--line-mid` (`#3d342d`, the new token, light `#d4cfc4`) with the 14 px plus.
- Cards: x 14–286 (272 wide), first at y = 121, 12 px apart (R4), 62 px tall against the drawn 61 (the machine tag's
  1 px padding and border make row one 18.3 px against 18.2; R5's 69 px is gone with line-height 1.3 on the rail).
  Fill `--card`, border `--line`, radius 11, padding 10 × 12. Row one: the repository diamond (14 px, `--repo`) or a
  folder icon, the name 600 14 px with an ellipsis, the machine tag on the right (11 px `--muted`, 1 px `--line-mid`,
  radius 99, padding 1 × 8) (R2). Row two, 12 px with 10 px gaps: the base branch in Plex Mono, the state in its
  colour ("3 running" in `--good`, "idle" in `--muted`, "machine offline" in `--warn`), and the worktrees in use
  right-aligned (R3).
- Foot: the 16 px monitor icon, gap 10, padding 10 × 12, dashed `--line-mid`, radius 11, 13 px `--muted`, "Folders are
  browsed live on the machine" (R9), at the foot of the column.
- Side by side with the reference (`cmp-rail.png` in the task's scratchpad): same columns, rows, type and colours.

**1239 × 900 and 1100 × 900 (collapsed): the 56 px strip** runs the full height (R12: `min-height` on the strip), with
Add, **Open the project rail** and one 44 px wide button per project named in full ("reet, Elitebook, main, 3 running,
3 worktrees in use"). In place, each shows its initial, its base branch (10 px mono, cut at 40 px), an 8 px repository
diamond, a 7 px running dot and the worktrees in use as a 15 px count on its corner. Open the project rail carries a
dot when a migration note or saved folders are waiting (named in words) and opens the full rail as a 301 px panel over
the page, where every card menu, note Dismiss and the saved-folders prompt are; Escape gives focus back. Fix round 1
of B6; `home.e2e.mjs` "collapsed strip reaches every rail action by keyboard" at 1239 and 1100 px.

**834 × 1112 (tablet) and 390 × 844 (phone): the rail sits under In flight**, as the plan's B6 tweak says, until the
Sessions tab takes it (PR H). It keeps the desktop cards at full width (786 and 358 px), 62 px tall, with "Projects"
as a 21 px Fraunces heading like In flight's, and the card menu button always shown at 44 × 44 px on touch.

**Fleet card.** The worktree figure joins the skills line ("3 worktrees in use", 12 px `--muted`, gap 12, as the drawn
"N worktrees · skills" row). With four machines at 1360 px the four machine cards still fit (220 px each, x 325–1247)
with the Add tile scrolling as before; the figure adds no height (the skills line already wraps there).

**Deviations, each with its reason.**
1. *"3 worktrees in use" where the mock-up says "3 worktrees", and nothing at 0 on a card (the mock-up says "no
   worktrees").* No record says which worktrees reet made and has not removed: the session listing has no removal
   record. The figure counts only worktrees live sessions are in, says so, and claims nothing when there are none
   (R12: no figure without a source). The fleet card does say "no worktrees in use", which is true of live sessions.
2. *No chosen card colours (`#201813` / `#6a3b22`).* Choosing a card sets the launch bar's project, which PR E brings;
   a card named by `/?project=<id>` is marked with an `--accent` border and `aria-current` until then.
3. *No "needs sign-in" or "waiting" state.* Those need the attention signal (PR G, deferred); the states are "N
   running", "machine offline" and "idle".
4. *The card menu button (⋯).* The mock-up draws no control, but the spec gives each card a menu. With a mouse it
   shows on hover or keyboard focus (26 × 26 px, top right, over the tag); on touch it is always shown at 44 × 44 px
   and the card keeps 56 px on its right for it.
5. *The base branch shows only when the project has one.* The drawn "main" is the project's own branch; a project
   that uses the repository's default has none saved, and reet does not invent one.
6. *Rail under In flight on tablet and phone.* The tablet and phone mock-ups put the rail on the Sessions tab (PR H);
   until then the plan places it under In flight so projects stay reachable.
7. *The one-time migration note and the "remaining saved Projects" prompt* are spec copy the mock-up's frame does not
   show; they sit inside the card and above the list, in the rail's own type.
8. *Light theme.* The mock-up is dark only; light uses the tokens' light values (`--line-mid` `#d4cfc4`).
9. *The collapsed strip's panel.* The mock-up draws the strip as initials only and has no way to manage projects at
   that width. The rail is now the only place to manage them, so the strip gains **Open the project rail**, which shows
   the full rail as a panel over the page (the least invasive fit for 56 px: one button, the full rail's own controls),
   and each project button shows its branch and worktree count in place as the plan's B6 tweak asks, at 10 px (the full
   values are in the button's name and tooltip).

**What the fixtures check.** `home.e2e.mjs`: "rail lists folder projects with their menu" (the card's words and
icons, 301 px, choosing a card starts in its folder with its project id, the menu's items and keys, Move down with
the versions read, Edit saving only what changed, Remove from reet after a confirmation, the 56 px strip), "worktrees
menu lists recorded worktrees with Check and remove", "Add opens the folder browser", "migration note shown once",
"remaining saved Projects come in when asked", "worktree figure counts only worktrees in use", "a slow refresh does
not drop a just-added project", and the existing layout and matrix sections with the rail under In flight at 834, 390
and 320 px (44 px targets, focus rings, no sideways scroll, reduced motion). `projects.e2e.mjs` (now in the
`phone-zoom` job): the redirects with the one-time note, the old id resolved to its folder project, the read-only
run in Activity with Check and remove, and no runs for a viewer. `navigation.e2e.mjs`: "Projects no longer under
More". `add-project-edit.e2e.mjs` (its own step in the `phone-zoom` job): the stale edit reload, branch suggestions
on retyping, a failed branch read retried, and a filter typed while a folder opens. `phone-zoom.e2e.mjs`: the rail's
menu, the Edit sheet and Runs from Projects at 320–412 px, and the Add project sheet now opened from Home.
`phone-dialogs.e2e.mjs`: Add, Edit, Worktrees, Remove from reet and the old run's Remove worktree, at six screens.

