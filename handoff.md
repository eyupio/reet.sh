# Reet central-control implementation handoff

## Read this first — 24 October 2026

This is the current implementation baseline **and** the continuation plan. Current
capabilities below are implemented; unchecked T/S/A/W/L tasks are future work.
**T1 and T2 are merged through PRs #140/#141.** Review the evidence and publication
state below. **T3 single-session archival is merged through PR #142. T4 terminal polish is merged through PR #145 after all five workflows passed.
T6 final-row clipping is merged through PR #147. T7 command recall is in PR #149. Finish the requested scrollback/theatre view, then the saved T5 setup branch, before resuming the roadmap.** S1 is merged through PR #139; do not repeat
it or start S2 ahead of the remaining terminal priorities. Do not start the whole roadmap.
Historical delivery evidence remains in
[handoff-history.md](handoff-history.md); old stage notes are not current feature
limitations. The previous handoff's "no controller-authorized terminal" statement
and old improvements-branch pointer are superseded by this file.

Read alongside:

- [Control-centre behaviour and boundaries](docs/control-centre.md).
- [Terminal workspace behaviour and implementation map](docs/terminal-workspace.md).
- [Controller sync](docs/controller-sync.md) and
  [verification evidence](docs/controller-sync-verification.md).
- [Portable agent setup architecture](docs/agent-setup-plan.md), especially format,
  ownership, credential, rollback and partial-recovery requirements.
- [Remote machine architecture](remote-machine-implementation-plan.md), with
  [stage 7](docs/remote-machine-stage7.md), [stage 8](docs/remote-machine-stage8.md),
  [stage 9](docs/remote-machine-stage9.md), and
  [controller terminal commands](docs/terminal-controller-commands.md).
- CONTRIBUTING.md and [voice/copy](docs/voice-and-copy.md).

## Latest user priority — terminal polish and connection setup

User steering, 24 October 2026: the terminal must behave like a professional SSH
client, with correct bottom-edge typing/wrapping/scrolling, terminal themes, and
saved CLI commands. The user explicitly chose **account-synced commands across
browsers and machines**. Initial reet installation, new-machine enrollment and reet upgrades must offer the
terminal connection wizard as a core setup step. A reported missing-tmux failure
must have actionable diagnosis/setup guidance. These requests take priority over
remaining skill/content work and bulk history cleanup.

Product scope correction, 24 October: reet is an **AI fleet control room**, covering
machines, installed agent usage, reusable AI setups, skills, reviewed sync and
ongoing work. Terminals remain core, but do not narrow the product to a remote-shell
app. Preserve the Yorkshire welcome and warm/bookish brand. Public copy must label
agent installation/update/removal, richer run orchestration and fleet-wide agent
configuration as planned until the corresponding roadmap stages actually ship.
The entry-page illustration and home toolkit links should convey this breadth.
See `docs/voice-and-copy.md`; keep the new scope when implementing T5 and S/A/W/L.

### Latest terminal follow-ups — finish before returning to the roadmap

The user still observed a clipped final row in `top`, requested correct colour
handling, and asked for searchable **shell command history** with paste/edit and
explicit rerun actions. Finish these and the setup work, merge when green, then
resume S2 and the remaining handoff stages. Do not reopen the overall strategy.

- **T6: final-row fit and colour verification — merged through PR #147.** The cause was `screen` padding:
  xterm's FitAddon measures its immediate parent and subtracts xterm's own padding,
  so padding on that parent produced a 20-pixel oversized grid. The old tests
  checked text presence and outer bounds, allowing text to exist while clipped.
  `fix/terminal-bottom-row` moves spacing to `screen-frame`, leaving the measured
  `screen` host unpadded. Keep PR #141's negotiated-grid lock unchanged. New tests
  compare the actual `.xterm-screen` rectangle against the clipping/content area
  at desktop/tablet/phone/short heights. The real fixture checks a last-row TUI
  marker while waiting for input, actual `top` through viewport changes, and ANSI
  red from the real host under Midnight/Paper palettes. Those local checks pass.
  Themes map text/palette colours; they do not invent highlighting for plain app
  output. Appearance explains that `top` toggles its colour mode with `z`.
- **T7: searchable command recall — implemented in `feat/terminal-command-history`, pending PR checks.** The existing History
  is an output snapshot; the account Command library is manually saved snippets.
  Neither is shell command history. Read the selected machine's Bash/Zsh history
  on explicit user request, scoped to its enrolling OS user, without arbitrary
  paths, subprocess `history` injection, screen scraping or keystroke logging.
  Bound file bytes/entries and reject symlinks/foreign-owner files. Search and
  display escaped command text; paste adds no Enter; rerun is an explicit reviewed
  action bound to the immutable session and current attachment generation.
  Keep history ephemeral unless the user explicitly saves a command. Label the
  source and explain that open shells may not have saved their latest commands.
  A new private-protocol request must be feature-gated without breaking old
  agents. Consider an additive authenticated HTTP upgrade feature header so new
  agents advertise a new capability only when the server supports it; preserve
  strict message decoding and the existing wire version. Reuse bounded history
  transfer and test old/new agent-server compatibility before exposing controls.
  Implementation map: `terminalproto/command_history.go` defines bounded typed
  records/strict codecs and the extension token. `api/private_control.go` advertises
  `X-Reet-Terminal-Features: command-history-v1` after agent authentication.
  `agenthealth/agent.go` advertises optional `commandHistory` only after that
  upgrade header, keeping new agents compatible with old strict V1 servers.
  `terminalbroker/bridge.go` refuses unsupported agents before dispatch; the
  existing bounded history stream carries JSON separately from terminal output.
  `terminalhost/command_history.go` reads only default Bash/Zsh history files in
  the locally bound OS-user home, with O_NOFOLLOW/nonblocking opens, regular-file,
  ownership, no-other-writer and single-link checks. Read caps are 256 KiB per
  file, 500 returned entries, 16 KiB per command and 256 KiB encoded response.
  Timestamped Bash and extended Zsh multiline records are parsed without executing
  anything. Missing/custom/unsaved history is labelled; no file is written.
  `api/terminal_command_history.go` rechecks the account before returning data.
  `CommandHistory.svelte` owns search/source filtering, paste and reviewed rerun;
  terminal output history remains reachable from the same drawer. Run confirmation
  cancellation keeps search results; confirmation binds the original attachment
  generation. No history text is stored in SQLite or browser storage.
  Store/API/host/protocol compatibility tests and synthetic UI checks pass locally.
  The real two-host fixture reads seeded shell files, verifies search/paste with no
  Enter, Run cancellation/confirmation and reconnect fences, and preserves PIDs.
  Local verification passed: frontend check/build (zero warnings), `go vet ./...`,
  `go test -race ./...`, all 16 MCP tests, production Control/workspace/lifecycle/
  organisation/public browser fixtures and the real two-host PTY fixture. PR CI
  and merge state must still be checked for the published head.
- **T8: visible output scrollback and theatre view — implemented in `feat/terminal-scrollback-theatre`, pending verification/publication.** Latest user steering asks for earlier output to be scrollable and a wide theatre layout alongside Maximise. Tmux paints an alternate screen, so xterm's browser scrollback alone cannot expose machine-side history. `OutputScrollback.svelte` reads the existing bounded `/history?lines=2000` snapshot on demand, with native vertical/horizontal scrollbars, Page Up/Down, text selection and literal search/match navigation. It labels the snapshot as read-only, keeps old output marked after refresh failure, clears it on denied access and aborts requests on dismissal. Never persist it. `TerminalPane.svelte` retains the same live xterm DOM/connection behind an inert hidden screen; reading/selection sends no input. The Scrollback toolbar button or Shift+PageUp opens it; Back to live restores terminal focus. The command-recall drawer's output link opens the same reader. No wheel interception: full-screen applications keep their own mouse/scroll behavior.
  `TerminalWorkspace.svelte` owns a bound theatre flag: hide the workspace toolbar and session panel without changing saved panel preferences or replacing the terminal component. Site navigation remains visible, screen fitting resizes the existing PTY, and Exit theatre restores the workspace. Existing Maximise remains a separate viewport overlay. Synthetic tests cover real overflow, keyboard scrolling/search, failed-refresh preservation, no shell input/reattachment, theatre/maximise restoration and saved panel state. Local verification passed: frontend check/build (zero diagnostics), all five production UI fixtures, native mouse-wheel/keyboard scrolling and a real two-host PTY test. It confirms output outside the current screen, search, live return and theatre preserve original shell/background PIDs. PR CI must still pass before merging. After T8 and T5 merge green, return to S2; do not expand this terminal detour.
- **T5 implementation saved remotely:** `feat/terminal-connection-setup`, commit
  `d48ccca`, contains the resumable CLI wizard, enrollment/initial-install/upgrade
  offers, tmux diagnostics, web setup step and real-TTY tests. It is not merged
  or in a PR yet. Targeted race/TTY tests and the new web readiness check passed.
  Merge current main into it, finish docs/handoff and verify the full change;
  do not duplicate those 18 files. Preserve the broader AI positioning from #146.

### Public entry points — terminal-first redesign

The signed-out `/` and `/welcome` explain machines, persistent terminals and
self-hosting. The illustrative workspace is explicitly labelled; its machine and
session names are examples. Signup, owner setup, account/token sign-in, invitation
copy, social metadata and the installed-app manifest use workspace language.
The share-image source is `brand/og-image.html`; regenerate its committed
`web/public/og-image.png` when changing the public positioning.
Actual access rules, invite-only restrictions and bootstrap-token requirements
are unchanged. Private inventories must never be fetched by these public pages.

`web/tests/public-workspace.e2e.mjs` exercises invite-only, signup-enabled,
first-owner/bootstrap and token entry points at 1440, 390 and 320 pixels in light
and dark mode. It checks sign-in errors, keyboard focus, no horizontal overflow,
and no private API requests. It runs in the terminal-browser CI workflow. Run
against the built UI with `REET_UI_URL=http://127.0.0.1:4173 node
web/tests/public-workspace.e2e.mjs`; optionally set `REET_UI_SCREENSHOTS` to retain
screenshots. Keep public claims limited to shipped functionality when adding
terminal appearance, account command libraries or future agent maintenance.

### T4 [x] Modern terminal interaction, appearance and saved commands — merged through PR #145

1. Reproduce ordinary-height, bottom-of-screen input after several screens of
   output; verify host `stty size` matches xterm rows/columns before and after
   sidebar/font/maximise/phone viewport changes. Keep the final prompt and wrapped
   input visible. The negotiated-grid fix in PR #141 is already merged; do not
   replace it with timeout changes, forced keystrokes or accessibility-mode hacks.
2. Add a compact, coherent terminal toolbar and an Appearance surface with named,
   contrasting terminal palettes and font/cursor preferences. Terminal appearance
   is independent of app chrome. Persist non-secret preferences and preserve
   reconnect/layout identity. Verify light and dark app chrome, desktop and phone.
3. Add a private saved-command library persisted under the user's reet account,
   reusable across browsers/machines. Support search, create/edit/delete and clear
   empty/error/loading states. Preserve exact text; do not scrape shell output,
   substitute credentials or auto-run commands when a card is selected.
4. Use explicit insertion into the chosen live session with no extra Enter. Multiple
   lines/control characters retain paste review and may execute only after the
   existing explicit confirmation. Bind pending insertion to immutable session and
   attachment generation; never carry it into a different connection silently.
5. Add owner/viewer/other-account tests, reload/second-browser persistence, denied
   requests and safe insertion/multiline/reconnect tests. Keep APIs metadata-only;
   command management must not require a connected host or weaken shell gates.

### T4 implementation and latest user additions

- `SessionSidebar.svelte` owns all/single/multiple-machine filtering, search,
  custom/name/activity/created/state sorting, machine/state/colour grouping,
  colour markers, native drag order and keyboard Move up/down. Right-click,
  Shift+F10 and row action buttons share an accessible menu. The parent captures
  the clicked ID/title, without changing the selected attachment. Stop, rename,
  disconnect and closed-history removal reuse the existing action boundaries.
- Desktop splitters support pointer capture and Left/Right/Home/End. Width,
  collapse and organisation persist in browser storage under the signed-in user
  ID. This is **browser-local**, not an account-wide layout sync feature.
- The stop confirmation includes removal after host-confirmed closure and an
  account-default toggle. `Profile.TerminalRemoveOnClose` migrates with default
  false; `Account.svelte` exposes it. End-operation parameters bind the explicit
  `removeHistory` choice to immutable retries. `archiveTerminalOnClose` inserts
  the tombstone only on authenticated closure, including failed creation. Pending
  sessions remain visible; receipts and host files are never deleted.
- Command metadata: `terminal_commands` table, `store_terminal_commands.go`,
  `api/terminal_commands.go`, `CommandLibrary.svelte`. Owner-serialized writes,
  revision conflicts and exact UTF-8 text; account cookies/write roles only.
  JSON bodies allow 128 KiB for escaped 16 KiB command text without increasing
  the wire-protocol control-frame limit. No host dispatch from library APIs.
- Appearance: `themes.ts`, `TerminalAppearance.svelte` and `TerminalPane.svelte`.
  Compact toolbar, separate app/terminal colours, local font/cursor preferences.
- Home now prioritises active sessions and terminal-ready machine cards. Skills
  and Sync are under More; Sync retains existing bulk actions and review gates.
- `terminal-organisation.e2e.mjs` covers cross-machine immutable row actions,
  stop/removal defaults, keyboard menus, colour/sort/group, drag order, resize and
  collapse persistence, themes, conflicts and guarded insertion. Add it to CI.
  The real-shell fixture checks a separate browser sees account commands, no
  implicit Enter, host tmux and shell `stty` dimensions, and bottom-row wrapping
  after more than two screens of output. Retain existing continuity/lease tests.
- User's latest sequencing: finish these terminal changes and T5, merge only
  when green, then resume the missing capabilities in the roadmap. Take S2 as
  the next content stage; keep agent capabilities/launch/activity (A1–A3), managed
  workspaces (W1–W3) and software maintenance (L1–L4) as bounded follow-up PRs.

T4 local validation on 24 October: frontend unit checks and svelte-check (zero
errors/warnings), production build, all Go vet/race checks, 16 MCP tests, existing
Control/workspace/lifecycle browser fixtures, the organisation fixture, and the
real two-host PTY browser fixture passed. That real fixture preserves both original
shell PIDs and the primary background job across reconnect/restart and checks
account command persistence in a separate browser. CI/merge state must be checked
for the exact published head; this local evidence does not imply deployment.

### T5 [ ] First-class terminal connection wizard during initial install, enrollment and upgrade

Read `internal/scan/terminal_setup.go`, `agent.go`, `agent_terminal.go`,
`internal/terminalhost/tmux.go`, `cmd/reet/terminal.go`, and the actual deployment
installer/upgrader in **`internal/lifecycle/cli.go`**. Also read `scripts/install.sh`
and the machine connection UI. `internal/setup/installation.go` observes AI-agent
installations; it is not the reet binary installer.

Expose a resumable `reet agent terminal setup` entry point around the existing
wizard. When enrollment itself offers that wizard, its internal enrollment call
must explicitly skip the post-enrollment offer to prevent recursive prompts.
Successful enrollment/install/upgrade must remain successful if optional terminal
setup is declined or deferred; print a resume command and preserve the enrollment.
Treat `--yes`, dry runs and noninteractive operation as unattended: do not install
packages, grant shell consent or block on new wizard prompts. Test real controlling
TTY behavior and missing/old tmux separately from fake service runners.

1. Reuse the existing guided host setup: check tmux 3.3+, explicit local shell
   consent (including root), independent services and readiness verification.
   Offer it during initial `reet install` as a first-class setup step, immediately
   after first enrollment, and after successful reet upgrade.
   Do not silently enable consent, re-enroll an existing host or kill tmux sessions.
2. Add a discoverable standalone host-terminal setup command for retry/repair.
   Missing/unsupported tmux explains version/PATH and local package-manager steps.
   Never weaken the minimum version to bypass the error.
3. Interactive flows offer setup; automation never hangs waiting for input or
   installs packages unexpectedly. Print exact follow-up commands when skipped.
   Preserve successful install/enrollment/upgrade if optional terminal setup is declined
   or remains pending. Preserve existing consent and running shell processes.
4. Web Add machine shows terminal setup/readiness as a primary part of connecting,
   with copyable guidance and truthful pending/offline/ready states. Do not mark a
   machine remotely terminal-ready based only on successful local service install.
5. Test real TTY questions, redirected input, skip/decline/repair, tmux unavailable
   or old, existing root consent, service refresh and failed optional setup after a
   successful install/upgrade. Document platform/runtime limits and exact verification.

## Terminal-first workspace requirements and delivery

**User steering, 24 October 2026:** terminals are becoming the primary everyday
use case. Users should land directly in a usable terminal workspace, without
scrolling past a stack of setup/help/health sections. Session connection and
maintenance actions need to be easy to find and unambiguous. This requirement
moved **T1 first**, ahead of S2 and the remaining skill/agent-management roadmap.
T1 is merged. S1 is merged as a separate bounded
stage. Review the T4 delivery above, then take T5; do not expand terminal work into S2.

### Required product behaviour

- `/terminals` is a primary workspace, not a form with a terminal appended below.
  Use the available width and remaining viewport height. A selected active shell
  appears immediately on desktop and phone without scrolling the page to find it.
- Keep machine, session, host user, connection status and primary actions in a
  compact persistent workspace header. Put machine/session navigation beside the
  screen on desktop and in a usable drawer/sheet on phones. Preserve the existing
  Control dashboard; improving terminal prominence does not silently change the
  home route or remove the skill library.
- Help, command reference, setup, quotas and detailed machine health belong in
  secondary panels/drawers. A blocking readiness problem gets a concise visible
  explanation and a clear setup action; long instructions must not push an
  existing usable terminal below the fold.
- Distinguish **browser attachment**, **host shell lifecycle**, **machine connector
  status**, and future **AI agent activity**. "Disconnected" does not mean the shell
  stopped. An online connector does not mean the selected shell is attached.
- Existing terminals already have Detach/Reconnect, Rename, End session, Search,
  Copy/Paste, font controls and Maximise. Improve their naming, placement and
  discoverability. Do not rebuild working transport because the controls are hard
  to find. Maximise should be optional; users should not need it to get a useful
  default workspace.

### Active-session hero band on Control

The main page must make ongoing work visible before general inventory and setup
cards. Add a compact, prominent active-session band near the top of Control with
an active count and direct links/cards for the user's sessions, grouped or labelled
by machine. Each item shows session title, machine, shell lifecycle and whether
connection information is current. Use plain shell/session labels; working/waiting
AI-agent status must wait for a real semantic adapter.

A click deep-links to `/terminals?agent=CONNECTOR_ID&session=SESSION_ID` and resumes
that existing session immediately. No new-session wizard, duplicate shell or scroll
hunt. Preserve the selected session on refresh and provide a Back to active sessions
route. Anchor links may jump to an already-rendered workspace; do not use an anchor
as a substitute for selecting the correct immutable machine/session IDs.

Use existing account-scoped terminal session APIs, rather than counting sync jobs
as sessions or inventing state from health. Respect account/write access and do not
request private session data for viewers. Poll only when visible, label stale or
unavailable data, and preserve last-known entries on transient failure; an API error
must not become "no active sessions". Creating and end-requested sessions remain
visible with their actual state. Do not label a generic record's updated timestamp
as live terminal activity. Keep quick links usable on mobile and with many sessions.

Fixture acceptance: create sessions on two machines, return to Control, see both
without scrolling, open either exact session with one click, and prove its original
PID and output remain. A delayed/offline session gives reconnect guidance without
creating a replacement. In a local healthy fixture target attachment within two
seconds after selection; report real timings and do not claim that as a remote
network SLA. Switching contexts must not terminate the previous session.

### SSH-style keyboard, mouse and clipboard interaction

The user explicitly requested familiar SSH-terminal behaviour for Ctrl+C,
Ctrl+V/Ctrl+X and right-click paste. Apply these rules to existing remote terminals
and preserve them for any future SSH session adapter. Do not replace the current
private transport or claim direct SSH host support merely by changing the UI.

- **Interrupt:** Ctrl+C must reliably reach the live shell as interrupt when no
  terminal text is selected. With selected terminal output, provide copy-selection
  behaviour and a clearly visible Interrupt/Send Ctrl+C action so interrupt is
  always available. Ctrl+Shift+C remains an explicit copy shortcut. On macOS,
  Cmd+C copies; Ctrl+C retains terminal interrupt behaviour.
- **Paste:** Ctrl+V and Cmd+V use native paste events where supported; provide
  Ctrl+Shift+V and an explicit Paste action. Send pasted text once only: do not
  process the same input through both a key handler and a paste event. Clipboard
  permission denial gives a useful fallback, never a silent no-op.
- **Cut:** Ctrl+X/Cmd+X works normally in editable app fields. Terminal output is
  read-only and cannot be "cut"; inside a shell/editor, preserve Ctrl+X as terminal
  input where appropriate. Do not erase scrollback, send delete/kill commands or
  pretend to know the remote line editor's buffer to implement desktop-style cut.
- **Right click:** provide a terminal-local Copy/Paste context menu. A paste-on-right-
  click preference may be offered explicitly, using the same safety checks; Shift+
  right-click should still allow the browser's native menu. Do not suppress context
  menus globally or surprise-paste when the user is simply selecting output.
- Preserve bracketed-paste semantics where the terminal supports them. Existing
  multiline/large/control-character paste review applies equally to keyboard,
  toolbar, mobile and context-menu paste. Cancellation sends no bytes. Never paste
  while disconnected or replay it when a connection comes back.
- Keep focus predictable: copy/menus do not lose the selected session; paste and
  explicit terminal actions return focus appropriately. App shortcuts must not
  steal Ctrl+C, Ctrl+X, editor keys or ordinary typing from a focused terminal.

Test Windows/Linux modifier conventions and macOS equivalents where runners are
available; use browser event/clipboard fixtures for deterministic coverage. Verify
copy with/without a selection, real shell interrupt, editable-field cut, shell
Ctrl+X forwarding, clipboard denial, context-menu paste, duplicate-paste prevention,
multiline approval/cancel, disconnected input and mobile long-press behaviour.

Direct SSH host connections, if required later, need a separate adapter contract:
host-key verification, machine-local key/agent references, authentication, resize/
signal forwarding and persistence guarantees. A raw SSH connection does not promise
survival across disconnects without a host session manager. Never introduce central
private-key copying or weaken host verification as a UI convenience.

### Action vocabulary and consequences

| User action | Required consequence |
| --- | --- |
| Connect / Reconnect | Attach to the selected existing session; no implicit new shell |
| Disconnect | Detach this browser; shell and its work keep running; intentional disconnect stays disconnected through polling/online events |
| Switch session/machine | Move browser attachment to the chosen context; preserve the previous shell |
| Rename | Change display name only; preserve shell/process identity |
| End session | Explicitly stop the shell and its running work; confirm the exact selected session; offline requests remain visibly pending |
| Remove from history | Hide/archive a confirmed terminal session that is ended/exited/lost; preserve durable IDs/receipts; must not kill a live shell or remove machine files |
| Forget machine / revoke enrollment | Separate machine-level management with existing consequences; never substitute either for disconnecting a terminal |

The current API does **not** provide session-history removal. Do not label the
existing End action Delete/Remove or invent a successful local-only deletion.
T1/T2 reuse supported actions; implement the T3 contract before exposing persistent
history removal. Read-only observation/shared control are later permission work;
do not relax current account ownership or viewer restrictions for this redesign.

### T1 [x] Terminal-first viewport and layout — merged in PR #140

Scope: improve the existing terminal route and active-session band using current
session APIs. No
agent launcher, new transport, history-deletion endpoint or unrelated app redesign.

Read first: `web/src/App.svelte` (main/header/footer widths and route classes),
`web/src/app.css`, `web/src/lib/Machines.svelte`, then
`web/src/lib/terminal/TerminalWorkspace.svelte`, `TerminalPane.svelte`,
`TerminalCommandReference.svelte`, `connection.ts`, `HealthHistory.svelte`,
`MachineHealthStrip.svelte`, and `web/tests/terminal.e2e.mjs`. Also inspect `ControlCenter.svelte` and
`web/src/lib/terminal/api.ts` for the active-session band.

1. Reproduce the current stacked layout with synthetic data and one active session.
   Record where the screen begins and which elements consume the first viewport.
   Include the PR #136 command-reference addition in this inspection.
2. Add a terminal-route layout class so app/main/footer padding and content-width
   constraints do not letterbox the workspace or force document scrolling. Keep
   navigation/account access available in a compact form. Do not change other
   pages' layout as a side effect.
3. Use a viewport-height flex/grid workspace with `min-height: 0` and contained
   scrolling: compact toolbar, optional navigation column, remaining space for
   the live screen. The terminal output buffer scrolls within xterm; secondary
   panels scroll independently. Move the decorative heading/help stack out of
   the critical path.
4. Add the active-session band on Control and exact-session quick links, following
   the visibility, freshness, ownership and continuation contract above. Move
   command reference and detailed health into explicit drawers/panels while
   retaining local setup commands, read-only diagnostics and offline guidance.
   Avoid tall always-open panels above an active terminal.
5. Fit the existing xterm instance when its container, drawer/sidebar or visual
   viewport changes. Keep session identity stable during status refreshes; do not
   unmount/recreate a shell attachment just because health or layout changed.
6. Implement/verify the SSH-style interaction contract above while preserving
   focus/copy/paste confirmation, safe clipboard/link handling, UTF-8,
   accessibility, font controls and mobile extra keys. App shortcuts must not
   consume normal shell keys. Keyboard opening must resize the usable screen
   rather than hide the active prompt behind the phone keyboard.

Acceptance: at 1360x900 the active terminal is visible at page scroll position 0
and its screen uses at least 60% of viewport height. At 390x844 with keyboard closed,
its screen uses at least 50%, with primary context/actions available without
scrolling down the document. Validate narrower/shorter screens with usable contained
scrolling rather than clipping. Sidebar/help toggles and resize keep the same shell
process. Check light/dark, keyboard focus, phone rotation and long session names.
Use measured browser bounds plus screenshots; a screenshot alone is not a fit test.
Include the Control active-band continuation and clipboard/interrupt cases above.
Gate: web checks/build and existing real terminal browser/lifecycle tests pass;
no regression in other routes and no hidden change to terminal authorization.

### T1 implementation record — 24 October 2026

- Branch: `feat/terminal-first-workspace`, based on main `cdafcd0` after PR #139
  merged. Implementation commit: `6bc7a78`. PR [#140](https://github.com/eyupio/reet.sh/pull/140)
  was merged on 24 October 2026 as `5fca4f43d90bd6a3a06fe404629adcd0020bba31`,
  after terminal-browser, image and binary-release workflows all passed on head
  `51ef5f0`. Automated review could not run because of its usage allowance; there
  were no inline review threads. A source merge does not establish deployment.
- Added compact, full-width terminal chrome and a visual-viewport-sized workspace;
  session navigation is beside the screen on desktop and toggleable on phones.
  Help, machine health, setup and command reference are in a native dialog.
- Added the Control active-session hero with exact machine/session continuation,
  account/write gating, visible-only polling, last-known state and revoked-access
  clearing. It makes no attachment or creation request until explicitly selected.
- Added terminal-local Copy/Paste menu, native paste handling, shortcut classifier,
  selected-output copy, explicit Interrupt and guarded paste focus/feedback.
  Existing generation fences, quotas, consent and attachment leases remain intact.
- Rename/End moved to Manage. Disconnect/Reconnect is visible and intentional
  disconnect survives online/visibility events. No removal/archive API was added.
- Real browser/CI testing exposed blank screen-reader rows despite visible output.
  The pane now refreshes xterm after every parsed output batch and fit, so a full
  redraw and subsequent resize update accessibility rows as well as pixels.
  Terminal modes remain supported. Four consecutive local real-shell browser
  runs passed after this correction. The real fixture asserts the
  restored prompt, subsequent output and alternate-screen restoration.
- Added `terminal-workspace.e2e.mjs` and CI production-preview browser coverage;
  extended the real shell fixture for dashboard continuation, disconnect/reconnect,
  usable screen bounds and updated toolbar/dialog controls. Resize checks wait for
  the new dimensions rather than reusing a pre-maximise frame. CI exposed a
  viewport-shrink assertion race; the fixture now waits for the resized screen
  bottom to fit as well as checking minimum usable height.
- Checks passed: web unit/Svelte checks (zero errors/warnings), production build,
  Go vet, full Go race suite, all 16 MCP tests, synthetic Control/workspace browser
  fixtures and the real Linux tmux/PTY/browser lifecycle fixture. Vite retains its
  existing bundle-size advisory. See [implementation map](docs/terminal-workspace.md)
  and verification commands below.
- Production-bundle synthetic measurements: screen top 214px / height 638px at
  1360x900 (71%); top 193px / height 560px at 390x844 (66%); 390x420 remained usable
  with a 136px screen and contained controls. No document scroll or horizontal
  overflow. Synthetic continuation measured 450ms; real continuation also passed
  the local under-two-second gate with the same original shell PID.
- A dev-server timing attempt during concurrent Go compilation exceeded two
  seconds; production-bundle measurement above was run without that contention.
  These are local fixture timings, not a production network SLA.
- Tested two-machine API fixtures, transient 503, denied session access, viewer
  isolation, native Ctrl+C/Ctrl+X/Ctrl+V, selected-output copy, explicit interrupt,
  ordinary-field cut, denied clipboard, right-click paste review/cancel, reconnect
  paste fences, light/dark and desktop/tablet/phone layouts.
- Not performed: physical phone/IME/OS keyboard testing, native macOS/Windows
  runtime, production relay/fleet deployment or AI-agent activity detection.
- T2 completed the follow-up below in the current branch. **T3 is merged; historical contract:**
  implement its persistent history contract before exposing Remove from history.
  Do not repeat T1 or S1.

### T2 [x] Session navigation and clear lifecycle controls — merged in PR #141

Depends on T1. Reuse existing APIs and `TerminalConnection.detach/connect/recover`.

**Already delivered with T1:** machine selector, searchable per-machine session
sidebar, selected/lifecycle labels, Manage → Rename/End, explicit New session,
Disconnect/Reconnect, exact deep links, immutable confirmation IDs and intentional
disconnect recovery test. T2 completes the following focused follow-up scope;
read its implementation record before repeating any of this work:

- Make machine selection searchable at larger fleet sizes; expose enough selected
  host/user context when the phone sidebar is closed.
- Exercise two real shells across machine/session switches, delayed poll/create
  responses, offline pending end, quotas, revoked access and takeover using actual
  controls. Preserve IDs, leases and the old shell throughout; fix evidenced races.
- Confirm rename and successful End target only the immutable confirmed session
  while selection changes. Add ambiguous-create-response UI retry coverage beyond
  the existing in-flight machine-switch test.
- Retain original requirements below as the acceptance checklist. T1 synthetic
  coverage and transport/API tests do not by themselves complete this entire gate.


1. Add a searchable machine/session list with clear selected context, session
   title, lifecycle state and attachment status. Keep unavailable machines and
   existing sessions visible with reasons. Avoid a long list of unlabelled tabs.
2. Put Connect/Reconnect, Disconnect, New terminal, Rename and End session in a
   predictable toolbar/context menu. Keep End visually distinct; explain its
   effect in confirmation. Creation must remain explicit and preserve retry IDs.
3. Retain deep links and selected IDs across appropriate navigation/reloads without
   storing terminal text or credentials in browser storage. Returning to a session
   uses the existing shell; polling never steals a user's selection.
4. Make intentional Disconnect sticky until the user reconnects. Preserve takeover
   controls and single-controller lease semantics. Do not auto-replay keystrokes
   or reconnect after an intentional disconnect merely because the network wakes.
5. Handle cancel, failure, pending end, lost shell, quota and revoked access visibly.
   Bind confirmations to immutable session ID, not whatever row is selected later.

Tests: create once under an ambiguous response; switch between two machines/shells;
disconnect/reconnect preserves PID; online/visibility events do not undo deliberate
disconnect; rename preserves process; End affects only the confirmed session;
pending offline end is not shown as completed; takeover and denied access stay safe.
Gate: browser tests exercise the actual controls, not only mocked state labels;
ordinary typing, control keys and multiline-paste confirmation still work.

### T2 implementation record — 24 October 2026

- Branch: `feat/terminal-lifecycle-controls`, based on main `5fca4f4` after PR #140
  passed its workflows and merged. Implementation commit: `c751e11`; PR
  [#141](https://github.com/eyupio/reet.sh/pull/141) opened on 24 October 2026.
  PR #141 merged as `b9630e87ae8c7b91400c4842c044ccedc911ae81` after all five
  workflows passed on `9bd06b1`, including the negotiated-grid regression fix.
  This does not establish release/deployment state.
- Added Find machine: search enrolled host/shell-user names and retain unavailable
  hosts with setup/reconnect guidance. Returning to a machine restores its last
  selected session while mounted. User/host remains visible on phones; narrowing
  from desktop closes the navigation overlay. Search receives keyboard focus.
- Fixed an evidenced creation-retry bug: an accepted creation with an interrupted
  response lost its retry context when users switched machines. The browser
  regression failed against T1 with A's original title replaced by B's title.
  Each machine now keeps its immutable request ID, original title and uncertainty
  message; retries cannot silently retarget or create a fresh operation.
- Fixed pre-mutation polling overwriting a newly confirmed session. Successful
  create/rename/end invalidates older observations and queues a fresh list.
  A separate status error clears on recovery. Status requests time out at 10s and
  abort unfinished sibling reads; creation waits at most 15s but retains its retry.
- Metadata 401/403 clears session/machine data, closes the attachment and explains
  lost access. Lost shells have explicit no-replacement guidance. No implicit
  shell creation, input replay, permission relaxation or transport rewrite.
- CI and instrumented replay exposed an initial-grid race: a restoration status
  message fitted xterm to 44 rows while the host's ticket still used 46. The first
  redraw scrolled the prompt out of view; its accessibility tree correctly stayed
  empty until a later host redraw. The pane now holds the negotiated xterm grid
  during restoration and refits/sends current dimensions after becoming live.
  Assertions, timeouts, input fencing and transport authorization are unchanged.
- Added `web/tests/terminal-lifecycle.e2e.mjs` to terminal-browser CI: per-machine
  immutable retries, delayed list/rename responses, selection targeting, mobile
  context/control bounds, unavailable-host search, pending end/retry IDs, explicit
  takeover, lost shell, quota and account revocation.
- Extended the real browser/PTY fixture with two independently enrolled local
  hosts, each with its own tmux server, registry and original shell PID. It aborts
  an accepted creation response, switches hosts, retries the exact request,
  transfers the lease through two browser pages, renames, and ends only the
  offline builder. The original primary background job survives throughout.
- Checks: web unit/Svelte check (zero errors/warnings), production build, Go vet,
  full Go race suite, all 16 MCP tests, Control/workspace/lifecycle Chromium
  fixtures, and the extended real-shell browser gate. The existing Vite bundle-size
  advisory remains. See verification commands below and the implementation map.
- Limits: the two real connectors run on one local Linux fixture, not two physical
  machines. Native macOS/Windows, physical phone/IME and production relay/fleet
  rollout were not exercised. Pending retry metadata and remembered per-machine
  selections are in memory for the mounted workspace, not durable browser storage.
  Reloads resume reported session IDs and never create a replacement implicitly.
- **T3 is merged.** Its persistent archive/store/API contract is retained below.
  Keep End session and Remove from history separate. Do not start S2 or agent
  lifecycle installation merely because the terminal layout is now primary.

### T3 [x] Single-session removal from history — merged through PR #142

Depends on T2. Add a small explicit lifecycle/API contract before exposing this UI.
Read `internal/store/store_terminal.go`, the schema/migration definitions in
`internal/store`, `internal/api/terminal.go`, host registry and operation receipts,
`web/src/lib/terminal/api.ts`, `TerminalWorkspace.svelte`, `TerminalPane.svelte`,
`terminal-lifecycle.e2e.mjs` and the real integration fixtures. Keep this first PR
limited to single-session archival; bulk cleanup follows only after that gate.

Start by writing down the endpoint, allowed states and visibility rules. Prefer
an archive timestamp/tombstone over deleting rows. Account-scoped listing should
hide archived closed records while receipt/retry lookup and host reconciliation
still find the original immutable IDs. Use one transaction for the state check and
archive write. Decide explicitly whether archival remains available when remote
terminal execution is disabled; preserving account/write/origin checks must not
accidentally enable other terminal operations. Never rely on browser state to
prove a shell is closed, and never use a host-file deletion as archival.

1. Choose an archive/tombstone model for account-owned terminal history. Reject
   removal of creating/running/end-requested sessions, even if a browser believes
   they are closed. Do not delete durable operation receipts or free active quota
   through archiving. Do not confuse unknown/offline with confirmed terminal end.
2. Add owner-scoped, role-checked, idempotent API support. Archive visibility must
   survive reloads/controller restarts. Preserve pending operations, replay fences
   and host-registry identity, so a stale creation retry cannot recreate a shell.
3. UI uses the exact label Remove from history and explains that this does not
   remove host files. Offer End first for live sessions. Keep errors visible; a
   failed removal must not disappear only from local component state.
4. Add reviewed bulk cleanup for selected closed sessions only after single-session
   removal works. Report per-item failures; never include active sessions by default.

Tests: owner/viewer/other-account, already archived retry, stale active state,
concurrent end/archive, offline machine, restart and old creation/end receipts.
Gate: an archived closed record stays hidden after reload without killing another
process, altering machine files, bypassing quota or making a shell recreatable.

### T3 implementation record — 24 October 2026

- Branch: `feat/terminal-history-archive`, based on main `b9630e8` after PR #141
  merged. Implementation `88a2257`, PR [#142](https://github.com/eyupio/reet.sh/pull/142).
  Single-session archival is implemented; bulk cleanup remains unchecked.
  Verify branch/PR state before repeating work. No production deployment is implied.
- [Archive contract](docs/terminal-history-archive.md): owner-scoped idempotent
  POST `/api/v2/terminal/sessions/{id}/archive` with `{}`; only ended/exited/lost.
  Active and pending states return 409. Cookie/account/write/origin checks remain.
  This controller-only route works when terminal execution is disabled; the
  exception does not enable create/rename/end/attachment/socket operations.
- Added `terminal_session_archives` as a child/tombstone table. Normal history
  excludes archived IDs, while internal identity/receipt/reconciliation reads
  remain unchanged. The state check and archive write serialize in one transaction.
  No terminal/operation record, machine file or host process is deleted.
- UI exposes Manage → Remove from history only for closed records, captures exact
  ID/title for confirmation, keeps failed entries visible, and preserves a newer
  selected session when a delayed archive response returns. Replayed closed create
  receipts do not optimistically reinsert archived entries into local history.
- Tests cover all lifecycle states, viewer/other-account/disabled-user boundaries,
  migration, restart, stable repeated archival timestamp, cross-handle end/archive
  race, quota preservation and old create/end receipts. API tests cover disabled
  execution, canonical Origin, token rejection and unchanged shell-operation gates.
- Chromium lifecycle checks cover cancel, failure, retry, selection during delayed
  archival, execution-disabled UI and reload. Real two-host browser checks remove
  only the ended builder, replay its original creation receipt without recreating
  a host shell, and prove the primary job/PID survives.
- Validation: web checks/build, relevant store/API race tests, three synthetic
  browser fixtures, real two-host/PTY browser fixture, Go vet/full race suite and
  all 16 MCP tests. Record final command results with the PR. Vite's existing
  bundle advisory remains. Physical mobile/IME, native cross-OS archive runtime
  and production rollout are not claimed.
- Migration creates a child table only. An older binary ignores tombstones and may
  show closed records again; it retains identities/receipts. No undo or bulk API
  exists in this first stage.
- **T4 is implemented in the current feature branch; next is T5 above.** Do not start S2 or bulk archival ahead
  of the explicitly requested terminal interaction/onboarding work.

### T3b [ ] Reviewed bulk removal of closed history

After T4/T5 and single-session archival review: add explicit selection of closed
records, show the exact count/titles, reject active/pending states transactionally,
report per-item errors and preserve IDs/receipts. Do not label End as Remove.

### Baseline and publication state

- Fleet redesign implementation inspected: `183beb7569921ac3c687ec52171bebdf3a5d9461`
  (PR #135). The documentation branch was rebased onto main `2e7176e` on
  24 October 2026, including the terminal setup/reference changes from PR #136.
- [PR #135](https://github.com/eyupio/reet.sh/pull/135), the fleet-first redesign,
  was confirmed **merged** through GitHub on 24 October 2026; returned merge commit
  `ed08496e3ef5097604fdaf019e7b08ff16d40f6c`. Refresh remote state next session;
  do not assume this dated baseline remains the latest main.
- PR #134 adds controller-side `reet terminal start|stop|restart|status`; these
  manage browser-terminal availability, not AI application processes. PR #136
  adds guided persistent terminal setup and a live command reference; inspect
  `cmd/reet/terminal_setup.go` and `TerminalCommandReference.svelte` before adding
  related setup UI.
- No production deployment, release artifact, real machine upgrade or live fleet
  content sync was verified in this conversation. A merge is not deployment.
- This handoff is saved through a separate documentation PR. Check its actual
  merge status before branching. Keep each implementation task on a separate
  branch/PR; do not append implementation to a merged PR.

## Implemented baseline: preserve these behaviours

### Product organisation and fleet controls

Signed-in home is **Control** at `/` and `/control`. Main destinations are
**Machines** (`/machines`), **Skills** (`/skills`), **Sync** (`/sync`) and
**Terminals** (`/terminals`, subject to account/write access). Lists, portable
setups, community and hosting are under More. `/profiles`, `/setups`,
`/account/machines`, `/account/sync` and legacy `/machines?view=compare|terminal`
links remain supported. Preserve aliases when extending routing.

- Control shows reported inventory, unique reported skills, saved sync scopes,
  recent jobs awaiting approval, fleet actions and machine/terminal links.
- Sync takes skills from the library/list, published hosted skills, or a reported
  machine scope. Choose one machine or All machines, then individual supported
  agent scopes. All machines currently means the reported scopes selected at
  that moment; it includes offline inventory, not automatic future enrollment.
- Saved desired state is separate from preview, exact plan approval and apply.
  Scan all queues available enrolled machines. Sync all previews available saved
  target sets; it does not replace desired state with the latest library.
- Distinct scopes may queue previews on one machine. Duplicate active previews
  for a target are blocked; active scans/applies block new previews; execution
  remains serial per machine. Existing plan hashes/revisions gate approval/apply.
- Paused/offline/pending/stale/incomplete destinations are skipped with reasons.
  Batch save/queue operations preserve earlier successes and report later failures.
  A lost response is ambiguous; refresh before retrying, never blindly replay.
- Source loading failure cannot become an empty desired set. Expected revisions
  are captured for the reviewed save. Scan freshness/completeness and source
  conflicts still gate sync. Unknown local-only sources cannot be installed by
  inventing an upstream source.
- Incoming skill adoption currently adds references/metadata to reet. **Live sync
  compares skill identity/source and membership, not file contents or versions.**
  Editing `SKILL.md` on one machine does not yet create a reviewed library bundle
  revision or propagate those changed bytes to other machines.
- Library stages, notes, lists and removal are distinct from machine installation.
  Removing a library entry leaves installed files and saved desired sets alone.
  Users maintain their authored bundles in Hosting; importing must never publish
  private machine content as a side effect.

### Remote terminals and machine health

The repository contains a real controller-authorized terminal implementation:
private connector transport, durable host session registry, broker/tickets/leases,
browser WebSocket attachment, xterm UI, bounded history, health and gated rollout.
Treat stage documents as historical increments, not proof that later code is absent.

- Account authorization, server terminal enablement and host-local opt-in are
  independent. Browser attachment uses account sessions, not leaked enrollment
  capabilities. Keep enrollment/owner/role checks and request limits intact.
- `reet terminal start|stop|restart|status` manages controller availability.
  `reet agent ...` manages the Reet connector and host services. Neither is a
  Claude/Codex/OpenCode package installation or AI conversation lifecycle API.
- Shells persist in independent tmux sessions across browser disconnects and
  connector restarts. Stopping browser availability is not ending host shells.
- Terminal creation has durable intent/request IDs and ambiguous-retry recovery;
  disconnects must not create duplicate shells or replay disconnected keystrokes.
- Current UI supports creating/selecting/renaming/ending shells, reconnect/search,
  machine health/history and local readiness/setup guidance. A running shell is
  not semantic evidence that an AI agent is working/waiting/finished.
- Current terminals start under the configured host user/home. Managed project
  launch, registered AI-agent launch, worktrees, code diffs, structured tool
  approvals and conversation resume/fork remain roadmap items.
- Terminal output stays in bounded host/tmux buffers/history; do not introduce
  central transcript recording as an unannounced consequence of activity tracking.

### Portable setup and supporting infrastructure

Local setup commands discover six agent families, summarise supported settings/MCP,
inspect bounded software evidence, declare/export selection policy, preview and
write encrypted snapshots, list/inspect snapshots, plan/apply/verify supported
restores, journal/rollback, resume eligible interrupted applies and report coverage.
Optional S3 delivery, fetch, listing, tests and protected pruning/deletion exist.

Snapshots can include supported global/project instructions, whole bounded skill
folders with sanitised provenance and lock-file creation/merge, optional selected
portable credentials and consistent copies of supported Codex memory databases.
Named setup profiles support revisions, exclusions, machine/project scopes,
rename/remove, server push/pull and interactive create/restore workflows. Web
profiles support exclusion revisions, coverage and metadata revision comparison.

This is **partial file-level recovery**, not complete working-agent recovery.
Broader memory/history/hooks/caches, agent dependency installation, verified
cross-OS recovery and provider runtime health remain unsupported or unverified.
Do not replace old capability disclosures with blanket "restore everything" claims.

Reet also already has accounts/roles, lists/sharing/teams, notifications/email,
MCP access to reet, sealed backups, authored/public skills and self-install/upgrade.
Extend these foundations instead of rebuilding capabilities that already exist.

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
   session navigation/controls and safe history maintenance (T1–T3). Finish
   reviewing T2 and implementing T3 next; do not start S2 ahead of terminal maintenance.
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
   PR #135's branch is already merged; use a new branch from current main for the
   next implementation PR, carrying any authorised local edits safely.
3. Pick **one unchecked task**, starting with T3 after reviewing T2. Do not build a later UI button
   or remote mutation endpoint while its prerequisite is absent. Do not turn
   this whole roadmap into one PR.
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

## S — Skill-content sync: small implementation tasks

**End-to-end acceptance story:** edit `SKILL.md` or a supporting script on machine A,
review its exact bundle diff in reet, create a private immutable revision, select
machine B and/or other targets, preview, approve, apply, and verify the expected
contents. A conflicting edit on B must stop that destination without undoing or
concealing success elsewhere. Nothing becomes publicly hosted merely by importing.

### S1 [x] Read-only local bundle manifest — merged in PR #139

Scope: a deterministic manifest and local comparison helper with fixture tests.
Do not add remote uploads, SQLite migrations, a web editor or filesystem writes.

Entry points: `internal/setup/skills.go`, `skills_verify.go`, `skills_test.go`,
`internal/scan/sync.go`, `sync_test.go`, `internal/store/store_hosted.go`.

Steps:

1. Read the existing snapshot collector and verifier, including how they handle
   modes, paths, symlinks, size limits and digests. Locate the live sync planner's
   identity/source comparison. Write a short contract comment explaining why a
   matching source is not evidence of matching bytes.
2. Introduce a small reusable domain helper/package, or extract a suitable pure
   part of the existing collector. Avoid making `internal/scan` depend on setup
   CLI/HTTP code. Keep existing snapshot formats and digest compatibility intact.
3. Define a schema-versioned manifest: relative file path, byte size, content
   digest, supported executable-bit semantics, total counts/bytes, manifest digest
   algorithm/version and explicit errors/omissions. Keep skill identity/source
   separate from contents. Do not include absolute host paths or credentials.
4. Specify canonical path ordering and serialization. Hash exact bytes; do not
   silently normalize line endings, rewrite YAML or treat executable scripts as
   equivalent after dropping their executable bit. Reuse/version the current
   digest scheme when suitable; do not reinterpret an old digest as a new format.
5. Bound traversal, individual reads, file counts and aggregate bytes; reject
   unsafe/ambiguous paths, symlink roots/entries and special files. Account for
   case collisions and changes while reading. An unreadable or omitted entry
   makes the bundle incomplete, never an empty/equal bundle. Preserve the existing
   scanner's documented shared-root behavior rather than globally banning all
   scanner symlinks as a shortcut.
6. Add a pure diff: added/removed/modified files; equal/changed/unknown bundle
   outcome. A missing prior observation is unknown, not an empty baseline.
7. Document capability limits. Snapshot skills currently allow 256 files and
   8 MiB per skill, with separate aggregate limits; hosted bundles have a smaller
   100-supporting-file/2 MiB contract. Re-read the constants before coding. Do not
   silently widen either format or truncate a bundle to fit a receiver.

Acceptance fixtures: byte-identical trees created in different orders; edit only
`SKILL.md`; edit only `scripts/run.sh`; add/remove/rename; empty file; executable-bit
change on supported platforms; same name/source with different bytes; unreadable,
missing and over-limit entries; symlink/path escape; case collision; a read-time
change. The test must fail if a same-source content edit is treated as equal.

Gate: targeted Go tests and race tests pass, snapshot compatibility is preserved,
and no test/command writes outside its temporary fixture. **S1 alone must not
change the public meaning of "in sync" or claim live content sync exists.**

### S1 implementation record — 24 October 2026

- Branch: `feat/skill-bundle-manifests`, rebased onto main `6c28b50` after the
  documentation PR [#137](https://github.com/eyupio/reet.sh/pull/137) merged.
- Source implementation commit: `5f18459e26a7605217d2f6642f66ed879de29ddf`.
  PR [#139](https://github.com/eyupio/reet.sh/pull/139) was verified merged on
  24 October 2026; main is `cdafcd0` at the start of T1 final verification.
  Merged source does not establish production deployment.
- Added `internal/skillbundle`: read-only bounded root-confined Scan, schema-v1
  SHA-256 manifests, POSIX executable-bit semantics, pure directional Compare and
  fail-closed unknown outcomes for incomplete/incompatible/malformed observations.
- Added fixtures for exact bytes, scripts/modes/line endings, add/remove/rename,
  unreadable/missing/incomplete data, traversal/case collisions, limits, read-time
  changes, FIFO replacement, failed-read aggregate budgets and canonical hashing.
- Existing encrypted setup/snapshot digests and shared scanner behaviour are
  unchanged. There are no new endpoints, CLI commands, migrations or remote writes.
- Contract and limits: [skill bundle manifests](docs/skill-bundle-manifests.md).
- Checks passed: new package race tests (90% statement coverage), setup/scan/store
  race regressions, web checks (zero Svelte errors/warnings) and build, Go vet,
  full Go race suite, all 16 MCP tests, and Darwin arm64/Windows amd64 package/test
  cross-compilation. Cross-compilation is not native platform runtime validation.
- No UI/session transport changed; no real machine, provider, release or production
  deployment was used. Vite retains its existing bundle-size advisory.
- **T1 is merged; T2 is implemented in the current branch; T3 is next.** Do not start S2
  merely because it is the next content dependency.

### S2 [ ] Private immutable revision store and ownership

Depends on S1. One persistence/API-contract PR, without a remote apply UI.

1. Inspect `internal/store/store.go`, `store_hosted.go`, `store_sync.go`,
   `store_sync_control.go`, `store_setup_profiles.go`, plus their API handlers.
2. Define a private bundle identity/revision model with owner, canonical manifest,
   immutable content references, origin/provenance, creation time and schema version.
   Preserve aliases/references to upstream identity separately from owned copies.
3. Distinguish immutable revision identity from manifest digest. Design all content
   lookups and reuse around account ownership; knowing another user's digest must
   not grant content access or reveal that their bundle exists.
4. Add migrations using existing transaction/idempotency conventions. Old records
   remain readable and have content status unknown until actually observed.
5. Store incoming contents privately and separately from public publication. Reuse
   the hosted editor validation where appropriate, not the public hosting routes
   as a private transport. Editing somebody else's/public upstream skill should
   create a clearly owned revision/fork, never silently overwrite its publisher.
6. Add expected-base-revision checks for mutable pointers. Bound all requests and
   return a conflict for stale concurrent edits. Decide backup/export inclusion
   explicitly; do not claim personal shelf JSON contains bundle payloads.

Tests: owner/viewer/other-account isolation; guessed digest; immutable revisions;
stale base; interrupted transaction; idempotent retry; upgrading an old database;
private import never changes public visibility. Gate: storage and API tests pass,
with a documented compatibility path for old clients and restore/backups.

### S3 [ ] Machine content observations and explicit import

Depends on S1/S2. Split observation metadata and payload transfer into separate
PRs if either becomes large.

1. Extend negotiated connector capability/inventory metadata with manifest schema,
   observed digest, completeness and time. Old connectors remain inventory-only;
   show "content not reported" rather than treating an absent digest as equality.
2. Keep periodic scans metadata-only. Uploading skill bytes is an explicit,
   account-authorized request scoped to an enrolled machine and skill root.
3. Preview origin, files, sizes and completeness before import. Respect local
   content-selection/exclusion rules; credentials/configuration must not hitchhike
   into a skill upload. Any detected sensitive/unsupported content needs an
   explicit blocked/review state rather than an automatic public/private upload.
4. Recompute and verify received manifests/digests; do not trust an agent-supplied
   hash alone. Reject changed-during-transfer, partial, oversized or unsafe payloads.
5. Create a private immutable revision only after a complete verified transfer.
   Retrying the same request returns its result; a lost acknowledgment must not
   create multiple revisions or restart an unrelated host operation.
6. Retain the existing metadata-only "Add selected to reet.sh" behaviour until
   bundle import is explicitly offered. Label the two actions distinctly.

Tests: old connector, stale/incomplete scan, changed file between preview/upload,
wrong machine/account, lost response/retry, corrupt chunk/manifest, cancellation,
revoked enrollment and private imported contents. Gate: two synthetic machine
fixtures can import different revisions without changing the existing shelf API.

### S4 [ ] Content-aware preview and reviewed single-target apply

Depends on S2/S3. First implement the planner, then the apply/receipt path.

1. Extend desired target state with a pinned bundle revision. Preserve legacy
   reference-based targets; mark their content-verification limits explicitly.
2. Bind preview/approval to target identity, desired revision, expected installed
   manifest, observed baseline, selected paths/actions and adapter/schema version.
   A hash of install names alone is insufficient for a content-replacement plan.
3. Plan create/update/remove with path-level differences and ownership. Never take
   over unrelated local files, local-only skills or a shared physical directory
   just because a display name matches. No broad directory deletion shortcut.
4. Stage and validate the full payload before writing. Acquire the correct local
   scope lock, re-read the preconditions immediately before mutation, and preserve
   recoverable backups/journal entries before replacement. Reuse existing restore
   lessons: include a partially written/current action in recovery accounting.
5. Verify the actual installed contents afterwards. Persist a receipt before
   acknowledging; include verified digest and per-action results. Incomplete
   verification is failure/incomplete, not applied or "in sync".
6. Reconcile interrupted/ambiguous execution from journal/receipt and local files.
   Never blindly replay an approved write because the controller missed a reply.

Tests: same source/different content; stale approved plan; local edit after preview;
changed desired revision; shared root ownership; disk/write/verification failures;
crash before/after write and receipt; rollback failure reported; retry without
reapplying; old connector refusal. Gate: real temporary filesystem fixtures prove
preservation of unrelated files and recovery, not just mocked API success.

### S5 [ ] Three-way content review and fleet rollout UI

Depends on S4. Deliver single-skill review first, then the fleet composition.

1. Compare last verified common baseline, current library revision and current
   machine manifest. With no baseline, show an explicit import/adopt decision.
2. Classify unchanged, library-only change, machine-only change, same change on
   both sides, conflicting edits and deletions. Do not auto-merge instruction text
   initially. Present library/local/keep-both choices; conflicting binary files
   require an explicit whole-file choice. Deletion/edit is a conflict.
3. Show file path and text diff where safe; escape content, cap previews, label
   binary/large files and never render skill HTML/scripts in the app origin.
4. Let a user save a private library revision independently from deploying it.
   Library removal, public unpublication and machine removal stay separate actions.
5. Review selected destinations and pinned revision per scope before queueing.
   Show queued/running/needs-review/succeeded/failed/skipped/unknown results per
   destination and aggregate them without claiming cross-machine atomicity.
6. Preserve current queue invariants: distinct scope previews may queue, execution
   is serial per machine, duplicate scope previews are rejected, and each exact
   mutation plan needs approval. Deduplicate shared physical writes and expose
   conflicting intents rather than guessing their ownership.
7. Explicitly distinguish one-time "all currently selected machines" from a future
   policy for new machines. Automatic inheritance is a separate reviewed feature.

Tests: conflict matrix including deletes; stale review; cancellation writes
nothing; mixed old/new connectors; one destination failing while another succeeds;
offline/paused targets; content source unavailable cannot imply an empty desired
set; keyboard/mobile review. Gate: the S acceptance story passes against real
local controller + two synthetic enrolled machine roots. Mocked browser fixtures
alone do not establish fleet transfer or filesystem correctness.

## A — Agent sessions and attention

Start after the content foundation; work on independent read-only discovery is
possible earlier, but it must not expand a bounded terminal or content-stage PR.

### A1 [ ] Registered AI agent capabilities and project selection

Entry points: `internal/setup/adapters.go`, `discovery.go`, `installation.go`,
`internal/agenthealth`, `internal/terminalproto`, `internal/store/store_terminal.go`,
`web/src/lib/terminal/TerminalWorkspace.svelte` and `MachineFleet.svelte`.

1. Reuse the agent registry and bounded executable/package evidence. Separate
   installed executable, observed version, configured provider/auth readiness and
   actual runtime health. Never run a discovered binary just to enumerate it.
2. Add typed, bounded, time-limited probes only for supported adapters. Report the
   exact evidence and unknown states; do not upload tokens or local configuration.
3. Add an account/machine-scoped project registry: stable ID, displayed name,
   validated host-side root, repository metadata and capability observations.
   A browser-provided path is not automatically an authorised host directory.
4. Handle moved/missing projects, machine aliases and reconnects. Global and
   project skill scopes must resolve to their effective physical destinations.

Tests: absent executable, wrapper/unrecognised version, malformed probe, timeout,
read permission failure, another account's project, path/symlink escape and stale
inventory. Gate: UI displays launch readiness with reasons, without launching or
installing anything.

### A2 [ ] Explicit typed agent launch on a chosen machine

1. Define a distinct AgentRun record referring to connector, terminal/runtime,
   registered AI agent, project and selected/pinned skills. Do not reinterpret the
   existing terminal `agentId` (connector identity) as an AI application ID.
2. Extend the existing create path using a negotiated typed capability. Keep plain
   terminal creation backward compatible. Prefer supported executable + argv + cwd
   over a controller-supplied shell command string. Custom launch commands are a
   separately reviewed future capability.
3. Preview machine, OS user, project, agent, skill revisions and permission mode.
   Launch only on explicit action after checking local consent and actual host
   capabilities. A permission/readiness failure must not trigger a reinstall.
4. Preserve current request IDs, durable host registry, lease/attachment and
   no-replay behaviour. Credentials remain machine-local; no central provider-key
   copying or prompt injection through environment/shell interpolation.
5. UI: New agent session wizard; retain New terminal. Show unsupported adapters
   and missing prerequisites plainly. Closing the browser leaves the host process
   alive; ending a run clearly describes what process will stop.

Tests: selected cwd/argv, unsupported/changed executable, duplicate creation/lost
response, local opt-in disabled, revoked account/enrollment, disconnect/reconnect,
agent exit and connector restart. Gate: use a harmless fixture executable to
prove exact launch and persistent reattachment; no live paid-model call required.

### A3 [ ] Semantic activity, attention queue and useful notifications

1. Add versioned per-adapter events for starting/working/waiting-for-input/
   approval-needed/idle/completed/error/unknown. Do not infer "working" from CPU
   usage or a live shell. Start with one supported adapter and a fake event fixture.
2. Prefer documented hooks or structured events. Bound and redact event payloads;
   authenticate the local run/event source. Unsupported adapters remain unknown.
3. Keep process state, connector availability and semantic activity separate.
   Deduplicate event IDs, reject stale/out-of-order events and preserve observation
   times. A disconnected machine is not a completed run.
4. Add a Control attention list deep-linking to machine/run. Extend
   `internal/notify`, `store_notify.go`, notification preferences and UI with
   per-user events. Avoid repeated alerts for the same waiting state.
5. Add Web Push as a separate optional PR after durable attention events work.
   The existing web manifest alone does not provide background push. Cover HTTPS,
   permission denial, unsubscribed/stale devices and account revocation.

Gate: fixture run transitions generate one correct attention event/notification;
another user's run cannot be read or acted on. Raw terminal text is not centrally
recorded merely to derive status.

## W — Projects, parallel work and structured review

### W1 [ ] Managed worktrees and project-bound parallel runs

Depends on A1/A2. Store each run's repository, base, branch and worktree identity.
Create worktrees through bounded host operations with validated argv, ownership and
request IDs. Preserve existing/dirty worktrees; do not move a running checkout or
remove a worktree belonging to another run. Preview cleanup, detect uncommitted
work and record partial create/delete failures. Start with one repo; multi-repo
workspaces are a later small PR. Tests must prove two runs can edit independently
and cancelled/failed cleanup preserves user work.

### W2 [ ] Files, code diffs and execution results beside a run

Depends on A1/A2; use W1 metadata when present. Add authenticated, bounded read-only
file/diff APIs confined to authorised run/project roots. Resolve base branches
explicitly; distinguish tracked/untracked/deleted/binary/oversized files. Do not
expose arbitrary host paths, execute Git hooks or render active HTML/SVG/scripts
under the application origin. Add changed-files and diff panels with mobile and
keyboard support. Keep code diffs separate from skill deployment diffs. Start
read-only; commit/push/PR mutations are separately scoped work, never implied by
viewing a diff. Test traversal/symlinks, other-user access, huge/binary files and a
real temporary repository with known changes.

### W3 [ ] Structured agent interface and tool approvals

Depends on A2/A3. Start with one negotiated ACP/agent adapter and keep the terminal
fallback. Persist typed run/turn/tool/question/approval IDs; do not screen-scrape
terminal text into authority to approve a tool. Approval must target an exact live
request, with expiry and expected state; stale/duplicate decisions and reconnects
must not execute twice. Separate tool execution permission from skill sync and
software installation approval. Advertise model/reasoning/mode selectors only
when the adapter supports them. Default to existing/local permission policy;
never enable bypass/auto-approve silently. Test competing devices, timeout,
malformed events and reconnect during a pending question using a fake adapter.

## L — Fleet software and environment management

### L1 [ ] Agent software inventory and plan-only maintenance

Depends on A1; extends the setup plan, not `reet install` or connector service
controls. Pick one supported AI CLI, one OS and one install method first. Define
capability evidence for exact installed version, manager, executable path,
platform/architecture, source and supported operations. Unrecognised wrappers or
installations stay inspect-only. Resolve available versions from approved sources
with bounded network calls, never guess that "latest" is compatible. Create
reviewable install/update/remove plans without executing them; record effects on
running sessions and configuration separately. Test unsupported methods, missing
versions, multiple executables/PATH shadowing and unavailable package sources.

### L2 [ ] Verified single-machine maintenance, then fleet composition

Depends on L1. Stage verified artifacts or use a specifically supported package
manager adapter; no generic curl-to-shell installer or arbitrary controller shell
command. Bind approval to exact package/version/source/platform and observed
installation. Require explicit handling of active runs; never stop them silently.
Keep configuration, skills, histories and credentials separate from binary removal.
Use journals/receipts, preserve a rollback route where supported, verify the
resulting executable/version and report unsupported rollback honestly. First
prove install/update/remove and interruption recovery locally; only then reuse
per-machine fleet reporting for bulk operations. Default to a reviewed pilot
machine before wider rollout. Test failed download/install/verification/rollback,
changed install after approval, reconnect and dependency ownership.

### L3 [ ] Desired MCP configuration and health across machines

Entry points: setup config/MCP parsers, policies and restore code; existing
`internal/api/mcp*` concerns clients accessing reet and is not the new MCP control
plane. Model server definitions, source/precedence, per-agent/project assignment,
secret references and compatibility. Begin with read-only effective configuration
and drift. Then add reviewed writes through format-specific adapters, preserving
unknown fields and native unrelated entries. Separate "configuration saved" from
"server reachable/authenticated/tools usable". Runtime/authentication tests are
explicit bounded actions; running a repository-defined stdio server needs trust
review. OAuth grants and device-bound credentials require independent destination
authentication where not portable. Test conflicting layers, missing secrets,
malformed TOML/JSONC, stale writes and unavailable endpoints before fleet rollout.

### L4 [ ] Optional isolated agent execution

Depends on A2 and verified project/skill mappings. Begin with one detected container
runtime and one supported image/platform. Review image digest, workspace mounts,
credential references, network policy and CPU/memory limits. Give each session its
own writable history/configuration store; do not mount the whole host home or
Docker socket by default. Keep host and sandbox terminal identities explicit.
Persist container ownership and creation receipts, handle interrupted cleanup,
and preserve user data on failure. Reet's server Docker deployment is not evidence
of remote-session isolation. Test mount escapes, concurrent sessions, runtime loss,
resource exhaustion and cleanup of only owned artifacts.

## Later extensions and non-goals for the next implementation

- **Conversation resume/fork:** after A2/W3, use qualified native conversation IDs
  with agent/store/project/version compatibility; persistent tmux alone is not
  conversation recovery. Do not claim cross-machine migration by copying an ID.
- **Team controls:** explicit machine/project roles and separate read-only terminal
  observation before collaborative control. Do not relax today's viewer/owner
  checks to make a demo work. OIDC login is distinct from reet's MCP OAuth server.
- **Launch templates:** compose pinned agent version, project rules, skills and MCP
  references after those components work; retain per-machine exceptions. Do not
  silently reinterpret setup snapshot profiles as this new model.
- **New-machine policies/scheduling:** automatic desired-set inheritance, scheduled
  previews and approved rollout windows are separate opt-in capabilities. Today's
  All machines selection is a point-in-time set, not a durable membership policy.
- Preserve one-binary self-hosting and local setup backup/restore without a server.
  No cloud account, provider credential copying or central transcript recording
  is required merely to use skill management or existing remote terminals.

## Product acceptance and evidence boundaries

This roadmap follows the user's central-control requirements. Its value is in
controlled cross-machine skill contents, versions and agent execution. Keep these
outcomes measurable against reet's own behaviour and acceptance fixtures.

Software discovery does not establish install/update/remove capability. Shell
persistence does not establish conversation recovery. Configuration restoration
does not establish authenticated runtime health. Local checks do not establish a
production deployment or verified cross-platform support.

Reet already has accounts, teams, sharing, private connections, snapshots and
durable shells. Extend those foundations. Keep task scope, capability claims and
validation evidence specific to reet; describe user outcomes without competitor
names, links or comparison tables in repository documents and handoffs.

## Validation commands and interpretation

For documentation-only changes, check Markdown links, whitespace and consistency
with the inspected code; do not rerun the entire application suite just to validate
prose. For implementation, run targeted regression tests while developing, then
required repository checks before PR handoff. Use tools from the current environment;
never assume a previous session's binaries or caches are present.

Prerequisites: versions from `go.mod` and `web/package.json` (at this baseline Go
1.27.1 and Node 22), dependency installation when missing, tmux 3.3+ for terminal
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

### Evidence recorded for the PR #135 implementation baseline

- Web unit checks and Svelte diagnostics passed (zero errors/warnings); production
  build passed. Main JS was approximately 500.4 kB minified / 148.4 kB gzip,
  slightly above Vite's 500 kB advisory threshold. Do not mask that warning by
  increasing its threshold; optimise only when a scoped performance task needs it.
- Go vet and the full Go race suite passed with workspace-local tmux 3.5a available.
  The initial environment lacked tmux; several service tests failed until that
  prerequisite was supplied. Do not weaken those tests to make a bare image pass.
- All 16 MCP tests passed. A cold fixture build initially timed out while other
  Go work compiled; the complete suite was rerun successfully with warm caches.
- Control browser checks passed with synthetic inventories for all-machine saves,
  incoming skills from different scopes, machine-source distribution, cancellation,
  partial save/queue failure, unavailable sources/inventory, viewer controls,
  legacy comparison routes and light/dark desktop/tablet/phone layouts.
- After rebasing the implementation onto main with PR #134, CLI/store race tests
  and whitespace checks passed. Vet and CLI/web compilation were checked again
  against the completed frontend bundle.
- No production host, release, agent installation or live fleet was changed.
  No roadmap S/A/W/L milestone was implemented by the handoff documentation update.

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

## Ready-to-use continuation brief

> Continue reet.sh from handoff.md. Inspect branch, worktree, main and PR states
> without resetting edits. T1/T2 are merged, including PR #141's negotiated-grid
> race fix. T3 history archival and the public landing redesign are merged through
> PRs #142/#144. Verify the T4 feature branch/PR before repeating its implementation.
> T4 delivers: correct bottom-edge terminal typing/scrolling, polished themes/controls and a
> private account-synced saved CLI command library (explicitly chosen by the user).
> Then T5: offer the terminal connection wizard on initial install/enrollment/upgrade and repair
> missing-tmux guidance. Read the detailed T4/T5 steps near the top. Preserve local
> shell consent, immutable IDs, leases, no input replay, explicit paste review and
> canonical Origin/account boundaries. Do not auto-run a saved command or enable
> shells silently. Validate host stty versus xterm at ordinary/full/phone sizes,
> real shell process continuity, two-browser command persistence and actual TTY
> setup questions. Finish web build before Go checks, record actual evidence and
> limitations, update the handoff and create a reviewable PR for each bounded stage.
> Keep S2 and bulk archival behind these user-requested terminal priorities.
