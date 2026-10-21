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
