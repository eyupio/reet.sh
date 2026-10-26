# Reet central-control implementation handoff

## Read this first — 24 October 2026

**Single sign-on, pull request 3 of 3 (25 October 2026).** The interface is in: the sign-in button (with the
password form folded away where password sign-in is off, so the owner can still use it), one plain sentence
for each `sso_error` code, link and unlink on Account → Single sign-on, and a read-only status panel under
Admin → Users. The words and the decisions live in `web/src/lib/sso.ts` (tested by `sso.test.mjs`), and
`web/tests/single-sign-on.e2e.mjs` answers the API from fixtures. Nothing remains of this feature but a real
identity provider run against a real instance, which has not been done here.

**Single sign-on, pull request 2 of 3 (25 October 2026).** reet.sh can now sign people in through an
OpenID Connect identity provider. The protocol package arrived earlier (`internal/oidc`); this slice
adds the routes (`GET /api/v2/auth/oidc/start`, `GET .../callback`, `POST .../backchannel-logout`,
`GET`/`DELETE .../identity`, `GET /api/v2/admin/sso`), the tables (`sso_identities`, `sso_logins`,
`sessions.sso_*`), the `REET_OIDC_*` settings, `auth/status`'s `sso` object, and disabling an account
now also closes its open terminals. Read [docs/single-sign-on.md](docs/single-sign-on.md). The
interface for it came in pull request 3.
The owner keeps password sign-in, and nothing in setup, personal tokens or backups changed.

**Latest slice (25 October 2026): S4 review UI, the Private skills dialog.** Delivered on
branch `claude/gallant-darwin-n9oqfj` through pull requests #178 to #185 (the design and plan,
the metadata-only adoption breakdown endpoint, the client API and pure review logic, the dialog
with the Pin step, the Check step, and Adopt and Release; #183 in that run of numbers belongs to
another branch) and one closing pull request that adds the size, theme, focus, Escape and
reduced-motion matrix to the browser fixture, the guide
[docs/skill-content-review-ui.md](docs/skill-content-review-ui.md) and this record. From
Machines, the signed-in owner can pin a library revision to a machine folder, check what the
machine holds, review the file breakdown, and record or release ownership. It writes nothing to
any machine, adds no MCP tool and has no apply authority. The reviewed executor, `applied`
receipts and S5 are **not** started. Read the guide first. The pull request links are
authoritative for final heads, CI and merge state.

**Before that: S4 ownership receipts and adoption.** The user authorised stacked pull
requests on branch `claude/gallant-darwin-n9oqfj`: [#173](https://github.com/eyupio/reet.sh/pull/173)
(spec and plan), [#174](https://github.com/eyupio/reet.sh/pull/174) (planner, claim, storage,
previews), [#175](https://github.com/eyupio/reet.sh/pull/175) (adoption and lifecycle) and
[#176](https://github.com/eyupio/reet.sh/pull/176) (API, audit labels, typed refusals,
documents and this record) are all merged. That slice was API only: no browser control, no MCP
tool, no filesystem write and no apply authority. Read
[the receipts contract](docs/skill-content-receipts.md) for the API. The PR links are
authoritative for final heads, CI and merge state.

**Before that: resumed after [PR #171](https://github.com/eyupio/reet.sh/pull/171) merged as `1eb92c0`.**
The user explicitly authorised continuation with one bounded S4 slice: the negotiated
metadata-only machine content preview, in [PR #172](https://github.com/eyupio/reet.sh/pull/172)
(merged as `ebf58f4`). It is API and connector protocol only: no browser
control, no filesystem write and no apply authority. Read
[the preview contract](docs/skill-content-preview.md) next.

This is the current implementation baseline **and** the continuation plan. Current
capabilities below are implemented; unchecked T/S/A/W/L tasks are future work.
**T1 and T2 are merged through PRs #140/#141.** Review the evidence and publication
state below. **T3 single-session archival is merged through PR #142. T4 terminal polish is merged through PR #145 after all five workflows passed.
T6 final-row clipping is merged through PR #147. T7 command recall is merged in PR #149. T8 scrollback/theatre view is merged in PR #152. T5 connection setup is merged in PR #153. T9 quick actions are merged through PR #155. S2 private revision storage/API merged in PR #156 (`ad1502a`) after all five workflows passed for `7e4aeec`. S3 metadata observations merged in PR #161 and CLI import in PR #164 (`7253663`). Central browser import merged in PR #168 (`7e2c992`) after all five workflows passed for `56aefe0fbb76d9cf9ce85bfec2591d58cdcc3eee`. S3 is complete. S4's pure content planner merged in PR #169 (`d494543`) after both workflows passed for `7595bc9b50f9d920da30fc4803f7720dfdc5af2a`. Optional owned revision pins/storage/API merged in PR #171 (`1eb92c0`). The negotiated metadata-only content preview merged in [PR #172](https://github.com/eyupio/reet.sh/pull/172) as `ebf58f4` (API and connector channel only, no UI). Ownership receipts and adoption followed in PRs #173, #174 and #175 (merged) and [#176](https://github.com/eyupio/reet.sh/pull/176) (open; API only, no UI). No machine content apply is wired yet.** S1 is merged through PR #139; do not repeat
it. Preserve the terminal work and do not start the whole roadmap.
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

## Current terminal sudo repair

The user reported sudo failing with “no new privileges” in an Elitebook terminal.
Both generated Linux host units set `NoNewPrivileges=true`; tmux and its shells
inherit this irreversibly. The user explicitly approved changing the generated
agent and tmux templates to `NoNewPrivileges=false`. The fix is in
[PR #165](https://github.com/eyupio/reet.sh/pull/165), merged as `7a8ab4b` after all five workflows passed for `f5fd088d27812a892557eeeba9e0858e9a2530ce`. No live units or sessions have
been changed. It preserves local sudo policy/password checks and does not grant
root access. Install/status inspect the enrollment's server and explain any
pending restart. Native Linux CI checks the real agent, tmux and child process
flags; existing refresh/reconnect tests must continue preserving PIDs.

Read the [migration procedure](docs/terminal-controller-commands.md#sudo-in-a-machine-terminal).
An old live tmux server keeps the flag after a unit refresh; do not silently
restart it. A planned exact terminal-service restart from normal SSH ends that
service's sessions/jobs. Never use a shared/default tmux kill-server operation.
Terminal callback-after-disposal guards merged separately in PR #167 as `144900b`,
after all three workflows passed for `64a636e`. They fix the reproduced xterm
“dimensions” error without weakening browser assertions. The browser-import
branch has integrated both terminal fixes and the live machine wizard from PR #166.

### Later: user-controlled terminal security preferences

The user explicitly requested a later security pass for privilege-capable browser
terminals, with **user preferences for PIN enablement or a similar mechanism**.
Work with the user to agree the interaction and defaults before implementing it;
PIN is a candidate, not a settled requirement. Do not fold this into the current
sudo repair or silently restrict the approved normal OS-user terminal behaviour.

- Consider an opt-in account preference for a PIN, password or passkey challenge
  before opening/resuming/taking control of a terminal, with per-machine policy
  where useful. Decide which actions require reauthentication with the user.
- Design clear locked/unlocked states, manual lock, idle expiry, reconnect and
  browser/device behaviour, including safe recovery if the PIN is forgotten.
- Enforce the gate on the server at authorization/control boundaries. Inspecting
  strings such as `sudo` in terminal input cannot reliably enforce privileges:
  scripts, aliases and other programs can perform the same actions.
- A PIN is an additional application access gate. It must not grant OS privileges,
  replace sudo's own policy/password checks, or collect/cache sudo passwords.
  If PINs are chosen, use secure verification, attempt throttling and a reviewed
  recovery path; never store a plaintext PIN or leak it into logs/terminal history.
- Preserve terminal sessions/jobs when locking or requiring reauthentication.
  Test expiry, disconnect/reconnect, takeover, cross-account/machine isolation and
  recovery before calling this security work complete.

This is a deferred roadmap item. No PIN preference or new authentication prompt
is implemented by the current sudo fix. Continue the bounded S4 work, and
schedule this security design with the user at an appropriate checkpoint.

## Current roadmap checkpoint — 24 October 2026

The user requested a status report and continued roadmap delivery after PR #155.
- **Merged foundation:** terminal T1–T9, machine control, saved commands/history,
  setup/upgrade guidance, control-page actions and Yorkshire/AI-fleet positioning.
- **Skill-content track:** S1 manifests merged in PR #139. S2 private revision
  storage/API merged in PR #156 (`ad1502a`), with all five workflows green for
  final head `7e4aeec090a0946cc1021aec8ce3b7e29b55fa27`. S2 has no new content
  UI or remote apply operation.
- **S3 complete:** metadata observations, explicit CLI import and browser imports
  merged through PRs #161/#164/#168. Final browser-import merge is `7e2c992`, all
  five workflows green for `56aefe0`. Private revision history is available.
- **S4 foundation merged:** pure bounded planner in PR #169 (`d494543`), both
  workflows green for `7595bc9`. Read [its contract](docs/skill-content-planner.md).
- **Additional merged baseline:** PR #170 (`d040518`) adds staged reet connector
  binary updates, progress/outcome reporting and rollback handling. It was merged
  separately and integrated into PR #171; preserve its `AgentUpdate.svelte`,
  `store_sync_updates.go` and connector update-state/protocol code. This concerns
  the reet connector binary, not the future AI-agent software/MCP maintenance stages.
- **S4 pins merged:** optional owned private revision pins, separate storage/API
  in PR #171 (`1eb92c0`). Read
  [the API, boundaries and granular continuation](docs/skill-content-targets.md).
- **S4 preview checkpoint:** the negotiated metadata-only content preview merged in [PR #172](https://github.com/eyupio/reet.sh/pull/172).
  Read [its contract and continuation](docs/skill-content-preview.md). It observes and
  plans but never writes; a slot with no ownership receipt needs adoption.
- **S4 ownership receipts and adoption:** controller-attested receipts are the planner's
  baseline and an explicit, hash-bound adoption creates them (PRs #173 to
  [#176](https://github.com/eyupio/reet.sh/pull/176) merged). Read
  [the receipts contract](docs/skill-content-receipts.md). That slice was API only.
- **S4 review UI:** the Private skills dialog on Machines (pin, check, review the file
  breakdown, record and release ownership) merged through PRs #178 to #185, with its closing
  documentation and responsive matrix in one more pull request. Read
  [the dialog guide](docs/skill-content-review-ui.md). Nothing writes to a machine. The reviewed
  executor is the next unchecked S4 work, in an order the user chooses. S4 and S5 remain
  incomplete.
- **Add a machine in context:** a guided dialog, from the Machines page and from the Private skills
  dialog, that shows one block of commands for a new machine, watches for it to report in and carries
  on where the person started (PRs #188 to #190 and one closing pull request). Read
  [the guide](docs/add-machine-in-context.md). Nothing writes to a machine.
- **Later:** A1–A3 registered agent launch/activity/attention; W1–W3 projects,
  worktrees/diffs/structured tool review; L1–L4 AI software/MCP fleet maintenance
  and optional isolation. Existing machine inventory and setup snapshots are a
  baseline, not completion of those stages. Stage sizes differ; do not invent an
  overall completion percentage or claim full bidirectional file sync today.

### Reported terminal-stop incident — unresolved, separate from S2

The user reported that Stop/End with removal enabled interrupted independently
running tmux-backed work on the same machine. They resumed that work and declined
further incident diagnostics; do not disturb it or ask them to reproduce a stop.
Code inspection shows the removal flag changes only controller history metadata;
private End messages are identical with/without removal. Host End targets one
exact `reet-<hash>` session using an explicit enrollment-private socket. No
production end/archive path issues kill-server/pkill/killall.

An isolated local probe through the actual host Manager.End kept original PIDs
for a second managed session, unmanaged work on the same server, and work on a
separate server. This does **not** establish the cause on the user's machine or
prove the incident fixed. A separate probe showed inherited TMUX can put new
windows inside a parent session, but the user's independent-start clarification
means that explanation must not be asserted for this incident. If it recurs,
collect metadata about whether processes ended or merely paused, installed versus
running agent versions, target IDs/sockets and service events through normal SSH.
No broad cleanup, process killing or speculative lifecycle fix. Continue the
roadmap as the user requested, keeping any proven terminal fix in a separate PR.

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
- **T7: searchable command recall — merged through PR #149 after all five workflows passed.** The existing History
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
  organisation/public browser fixtures and the real two-host PTY fixture. All five PR workflows passed for `cad5466`; PR #149 merged as `d14b428`.
- **T8: visible output scrollback and theatre view — merged through PR #152 after all three workflows passed.** Latest user steering asks for earlier output to be scrollable and a wide theatre layout alongside Maximise. Tmux paints an alternate screen, so xterm's browser scrollback alone cannot expose machine-side history. `OutputScrollback.svelte` reads the existing bounded `/history?lines=2000` snapshot on demand, with native vertical/horizontal scrollbars, Page Up/Down, text selection and literal search/match navigation. It labels the snapshot as read-only, keeps old output marked after refresh failure, clears it on denied access and aborts requests on dismissal. Never persist it. `TerminalPane.svelte` retains the same live xterm DOM/connection behind an inert hidden screen; reading/selection sends no input. The Scrollback toolbar button or Shift+PageUp opens it; Back to live restores terminal focus. The command-recall drawer's output link opens the same reader. No wheel interception: full-screen applications keep their own mouse/scroll behavior.
  `TerminalWorkspace.svelte` owns a bound theatre flag: hide the workspace toolbar and session panel without changing saved panel preferences or replacing the terminal component. Site navigation remains visible, screen fitting resizes the existing PTY, and Exit theatre restores the workspace. Existing Maximise remains a separate viewport overlay. Synthetic tests cover real overflow, keyboard scrolling/search, failed-refresh preservation, no shell input/reattachment, theatre/maximise restoration and saved panel state. Local verification passed: frontend check/build (zero diagnostics), all five production UI fixtures, native mouse-wheel/keyboard scrolling and a real two-host PTY test. It confirms output outside the current screen, search, live return and theatre preserve original shell/background PIDs. All three PR workflows passed for `55f3624`; PR #152 merged as `51a2997`. After T5 merges green, return to S2; do not expand this terminal detour.
- **T5 implementation:** `feat/terminal-connection-setup`, originally saved as `d48ccca`, now integrated with merged main through PR #152 (including the separate sync/agent-update improvements from PR #150). The CLI/wizard, enrollment/install/upgrade offers, web readiness step and tmux diagnostics are implemented. `localTerminalAgentNative(config, version)` verifies the supervised process version through the private local socket; a stale version offers service refresh. Shared-label dev builds always offer refresh so an updated binary is actually loaded. Consent, enrollment and tmux jobs survive optional decline/failure. Review `docs/terminal-controller-commands.md#first-class-machine-connection-setup` for behavior and tests. Integrated frontend check/build, full vet/race, all five production UI suites and the real two-host PTY fixture passed. All five PR workflows passed for `9cd770c`; PR #153 merged as `442d3a7`. Preserve #146 AI scope, #147 fitting, #148 skill editing/import, #149 recall and #152 scrollback/theatre.

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

### T9 [x] Merged through PR #155 — quick terminal actions on workspace and control page

Merged as `b15bb5c` after all three workflows passed for `e1ab64e`. Frontend check/build, all five production UI fixtures, full vet/race and the real two-host PTY fixture passed. Preserve these controls when continuing the roadmap.
- `SessionSidebar.svelte`: visible + beside machine headings (including enrolled
  machines without sessions) and selected-machine picker; right-click/ellipsis
  machine menu. Disable new shells for unready/disabled/quota-limited machines.
- `TerminalWorkspace.svelte`: explicit quick-create uses one immutable request ID
  per machine, default Shell title and existing quota/retry logic. Switch directly
  to the new operation without first attaching another existing shell. A failed
  response retains its operation; retry cannot accidentally create a second shell.
- Clear Show sessions controls in the workspace header and terminal toolbar when
  hidden; the latter also exits theatre and restores the panel. Preserve saved width.
- Terminal right-click menu exposes saved commands, command history, output history
  and appearance alongside clipboard controls. Shift+right-click keeps native menus.
- `QuickTerminalButton.svelte` and `TerminalCardActions.svelte` add the same + and
  right-click/ellipsis actions to control-page machine and active-session cards.
  Continue/New/Commands/Manage target the exact card. Session cards also offer
  rename and reviewed stop with remove-after-confirmed-closure/account-default options.
  Commands on the control page can be maintained/copied; inserting still requires
  an attached terminal. Merely opening menus must never attach, create or send input.
- `ActiveSessions.svelte` fences observations across card mutations so an older poll
  cannot overwrite confirmed changes. Viewers/logged-out pages get no private actions.
- Browser tests verify explicit target IDs, empty/unready machines, ambiguous create
  retries, no intermediate attachments, panel reopening from collapse/theatre,
  landing rename/stop/default preservation and saved-command menus. Existing layout,
  clipboard, ownership and persistence tests stay enabled. Record final CI evidence.

S2 now continues on `feat/private-skill-revisions` with the original WIP integrated
with main through PR #155. The WIP implementation is no longer the continuation
baseline; use the S2 implementation map and publication/check status below.

### T5 [x] Merged through PR #153 — First-class terminal connection wizard during initial install, enrollment and upgrade

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

1. Reuse the existing guided host setup: check tmux 3.2+, explicit local shell
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
   Confirm the previous slice's state through its live PR/CI/merge record, then start
   from current main and preserve local edits.
3. Pick **one bounded next slice** from the continuation lists in the S4 records
   and docs/skill-content-receipts.md; the user chooses the order. T1–T9, S1–S3 and the
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

### S2 [x] Private immutable revision store and ownership

Merged in PR #156 as `ad1502adfd97e799a3e4947f03b70d62d5fb59c3`. All five workflows passed for `7e4aeec090a0946cc1021aec8ce3b7e29b55fa27`.
Read [the storage/API contract](docs/private-skill-revisions.md) before changing it.
- `skillbundle/contents.go` computes upload manifests with exact S1 filesystem parity;
  no byte/path normalization or local filesystem writes.
- `store_private_skills.go` adds owner-scoped blobs, immutable random revision IDs,
  separate manifest digests, immutable provenance, explicit expected-base head CAS,
  idempotent account request IDs and transactional account capacity limits.
- Retries return original receipts even after head advancement; the returned receipt
  head is historical. GET the bundle separately for current head. Do not rewind it.
- `api/private_skills.go` exposes bounded JSON GET/POST account routes under
  `/api/v2/skill-bundles`. Browser writes need same-origin Origin; personal API tokens
  use their own account. No default-owner fallback, public hosting mutation, remote
  dispatch or MCP allowlist expansion. Complete reads verify stored integrity.
- SKILL.md uses an isolated hosted-validation copy (2 MiB markdown maximum); all
  original bytes/modes are retained. Full revision limit is 8 MiB / 256 files;
  per-account limits are 128 bundles, 256 revisions, 64 MiB unique blob data.
- This stage is append-only and has no content UI/deletion/GC. Full controller DB
  backups include payloads; personal shelf JSON/personal backups exclude them.
  A separate account-content export contract is needed before UI backup claims.
- Targeted race tests pass for parsing/parity, cross-account/role isolation, stale
  concurrent edits, stable retries, injected SQL rollback, all capacity bounds,
  old-schema reopen, public/private separation, account deletion and full DB restore.
  Local verification passed: frontend check/build (zero diagnostics), full `go vet ./...`, full `go test -race ./...`, and all 16 MCP tests. Published PR #156 passed all five workflows for final head `7e4aeec` before merging.

After merging S2, take **S3 metadata observations**, then explicit content import.
Do not jump to S4 apply or the whole roadmap. Preserve all merged terminal controls.

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

### S3 [x] Machine content observations and explicit import

**Metadata slice merged in [PR #161](https://github.com/eyupio/reet.sh/pull/161), `dcef39cbd6cb172d0dc12a90b3730f78afc8852c`, after all five workflows passed for `39cb762931d5e0437acf59a1f906402bdf75d6ff`.** Read
[the protocol and continuation contract](docs/machine-content-observations.md).
- New authenticated prepare/report endpoints negotiate version 1 before file
  hashing. Old controllers (404/405) and future schemas skip content reads.
- Enrolled connectors hash only their configured local scopes after successful
  legacy scans. Reports contain digests/counts/completeness/times/issue codes,
  never file bytes or per-file paths. Limits: 64 scopes, 512 skills, 64 MiB reads
  per cycle, with existing S1 per-skill limits and cancellation before dispatch.
- Store rechecks owned active writer enrollment, pause state, expiring grant and
  inventory generation transactionally. New legacy scans invalidate old hashes,
  including after forget/recreate. Retries preserve original freshness; changed
  retries conflict. Revocation cascades; omitted scopes remain unknown.
- `SyncEnvironment.content` is separate from name/source desired-set identities.
  Machines and Sync show unknown/incomplete/stale/paused/observed labels and
  read-only fingerprint details; they do not claim contents are in sync.
- Local checks passed after integrating main through `0c39e79`: frontend check
  (zero errors/warnings) and build; `go vet ./...`; full `go test -race ./...`;
  all 16 MCP tests; all five production browser fixtures (including new content
  states/details and no writes on inspection); real browser/broker/tmux shell
  lifecycle preserving the original shell and job through refresh/reconnect and
  two-host lifecycle. Additional race tests cover byte-budget exhaustion,
  cancellation, legacy-schema reopen, expiry, role changes and rollback.
  No live user workloads or deployed connector installations were changed.
- **Explicit CLI transfer merged in [PR #164](https://github.com/eyupio/reet.sh/pull/164), `7253663505baa11187f87baa5bea093e87197442`, after all five workflows passed for `717681ac41338c28266ee67fb71ae52cfd1d4410`:** read
  [the complete import contract and granular continuation](docs/machine-skill-import.md).
  `reet agent skill import` previews one locally enrolled scope/folder; `--upload`
  plus typed IMPORT or explicit `--yes` sends selected files. Exact exclusions,
  conservative credential blocking, fresh post-confirmation source capture and
  selected digest verification precede transfer. No background upload or execution.
- `skillbundle.ReadContents` reuses bounded no-follow S1 reads, returns no payload
  on unsafe/changed/partial captures and checks exact captured byte/mode identity.
  `skillimport.Select` is shared by CLI/controller. `store_machine_skill_import.go`
  adds a guarded entry into the existing S2 transaction: active writer, enrollment,
  pause and reported-scope membership are rechecked before private writes/receipts.
  No new payload tables, backup promises or public/shelf mutations.
- GET/POST `/api/v2/sync/skill-import` negotiate before dispatch and require account
  plus owned enrollment. Strict bounded JSON, received digest recomputation,
  expected-base CAS and original request-ID receipts preserve S2 semantics.
  A printed request ID permits an identical retry after a lost acknowledgment.
- CLI import verification passed: frontend check/build (zero Svelte diagnostics),
  full Go vet/race suite, all 16 MCP tests; additional targeted race tests after
  integrating main through `ec1b6b5`. Two enrolled-machine fixtures create distinct
  same-name private revisions, preserve binary bytes/CRLF/modes, omit excluded
  credentials, recover identical retries and leave the public/shelf API unchanged.
  Tests also cover origin/account/enrollment/pause, bad digest, strict JSON/limits,
  old-controller negotiation, cancellation, changed review and SQL rollback.
  No production machines, services or terminal sessions were modified.
- **Central browser import merged in [PR #168](https://github.com/eyupio/reet.sh/pull/168) as `7e2c992`, after all five workflows passed for `56aefe0fbb76d9cf9ce85bfec2591d58cdcc3eee`:** read
  [the complete browser contract](docs/browser-skill-import.md). Machines → Private
  skill imports and per-skill Import privately open a review panel with source,
  destination/base, file metadata, exact exclusions, hazard blocking, explicit
  approval, cancellation, stable receipts and private bundle revision history.
- Imports have a separate versioned channel, never the legacy sync queue/terminal
  messages. Connector capability is recent for 90 seconds and tied to a per-process
  nonce; requests expire after ten minutes. Old/unsupported connectors do not read
  content. A process change, revoked/paused enrollment, role change, stale source,
  changed selection/destination or cancellation prevents a later commit.
- Preview API/browser history contain metadata only. Actual bytes are captured
  again against the full source digest and selected policy, with a final metadata
  authorization check before dispatch. Server recomputes selected byte/mode identity.
  S2 private writes and request completion/receipt commit in one transaction through
  before/after hooks. A completed-copy receipt remains visible if completion wins
  a cancellation race. Dismissing history never deletes private copies.
- Store/schema map: `store_remote_import.go`, `machine_import_capabilities` and
  `machine_import_requests`; pure `skillbundle.FromMetadata`, shared preview/hash/
  selection helpers; `api/remote_import.go`; `scan/remote_import.go`; browser
  `MachineSkillImports.svelte`. Protocol/client/store/UI tests cover their contracts.
- Limits are 128 retained requests/account plus unchanged S1/S2 bounds. Broader
  private-content authoring, deletion/GC and export UI remain later work, rather
  than implicit public editor/shelf mutations. The library currently browses heads
  and revision metadata. No PIN or new authentication preference is implemented.
- Local validation passed after integrating main through `144900b`: frontend
  check/build (zero diagnostics), full Go vet/race suite, all 16 MCP tests and six
  browser fixtures. The final phone layout keeps approval/cancellation visible.
  Real HTTP/SQLite + synthetic connector fixtures prove metadata-only review,
  two distinct same-name private copies, exact CRLF/binary/modes, explicit hazards/
  exclusions, changed-source rejection, unsupported servers, ownership, expiry,
  generation/pause/role/revoke/cancel fences, atomic receipt rollback and retries.
  Browser fixtures use synthetic API responses; no live user machines/services
  were modified and no physical cross-platform import rollout was performed.
- S3 is now complete through the recorded PR #168 merge. Continue with S4 in
  separate planner, negotiated preview and reviewed executor/receipt slices.
  Do not jump to bulk apply or reuse an import approval for machine writes.


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

Depends on S2/S3, which are merged. **Pure planner merged in
[PR #169](https://github.com/eyupio/reet.sh/pull/169) as `d494543`, after both workflows
passed for `7595bc9b50f9d920da30fc4803f7720dfdc5af2a`.** Read
[docs/skill-content-planner.md](docs/skill-content-planner.md) before continuing.

- `internal/skillplan` has no filesystem/DB/network operations. Build takes an owned
  pinned private revision, explicit recent observation, target/root/generation,
  verified managed baseline and additive/managed mode. It returns sorted actions,
  conflicts, preserved paths, installed/managed result manifests and a bound hash.
- Existing roots without a verified baseline require adoption; shared/unknown roots,
  local edits/deletions, unmanaged desired paths, stale/invalid observations and
  combined result collisions block. No partial executable plan is exposed.
- The pure planner slice did not add revision pins to storage, per-file connector
  preview, API/UI review, approval, transfer, apply, locking, rollback or durable receipts.
  S4 stays unchecked until real filesystem execution/recovery gates below pass.
- Optional revision pins/storage/API merged in PR #171 (below), and the separately
  negotiated metadata preview follows it (next subsection). Existing aggregate content
  observations cannot provide the required per-file tree or trustworthy absence, so the
  preview captures it explicitly. Keep execution separate.
- Validation: frontend check/build with zero Svelte diagnostics; full `go vet ./...`
  and `go test -race ./...`; all 16 MCP tests. Focused planner race tests have
  96.6% statement coverage. The first full run lacked tmux on PATH; after adding
  the installed tmux 3.5a, the complete suite passed without source/test weakening.
  No browser UI, live machine, service, session or remote filesystem was changed.

#### S4 desired revision pins — PR #171, pause checkpoint

Read [docs/skill-content-targets.md](docs/skill-content-targets.md).
[PR #171](https://github.com/eyupio/reet.sh/pull/171), branch `feat/private-content-target-pins`.

- `store_content_targets.go` adds optional `skill_content_targets`, migrated after
  private revisions/enrollments. Pins bind owned environment, enrollment, bundle,
  immutable revision, name derived from bundle, mode and independent CAS version.
  Maximum 1024 pins/account. Old reference desired sets and hashes stay unchanged.
- `api/content_targets.go` exposes private-account GET/POST collection and GET/DELETE
  item endpoints at `/api/v2/skill-content-targets`. Active writers only; same-origin
  browser writes or own personal tokens; strict 16 KiB JSON, no queries or MCP
  allowlist changes. Responses are metadata-only, always `not_verified`.
- Supported scopes: reported complete home claude/agents/cursor/opencode and
  absolute claude/agents project scopes within 14 days. Paused/revoked or wrong-host
  enrollments cannot save. Inspection/removal remain available while paused/offline.
- Private head changes never move a pin; explicit CAS can select an older revision.
  Identical current saves preserve version/time. Changed desired state creates a
  fresh version; stale updates/deletes conflict. After uncertain replies reconcile
  with GET; these are not durable execution receipts. A create replay with empty
  expected version can recreate a removed pin; clients must not blindly retry it.
- Removing pins affects only intent. Environment/enrollment/account cascades remove
  pins; replacement enrollments do not inherit them. Private revision bytes survive
  pin removal and enrollment revocation. Same-enrollment machine rename preserves
  pins and bumps versions; conflicting pins roll back the rename. Generic
  cross-scope report merging blocks if source pins exist. No new legacy job or
  terminal message exists.
- Focused SQLite/HTTP race tests pass: ownership/token/origin/role fences, pinned
  history/rollback, old modes/scopes, stale edits/deletes, pause/revoke, cascades,
  replacement enrollment, rollback injection, concurrent Store connections,
  capacity, old-schema migration/reopen and unchanged legacy desired/dispatch.
- Full local validation passed: frontend check/build with zero Svelte diagnostics,
  `go vet ./...`, full `go test -race ./...` with tmux 3.5a on PATH, and all 16 MCP
  tests. No browser UI, live machines, services or sessions were changed. The
  published workflows must pass for the final PR head before merge. The final
  pin implementation (including rename integration) is `43f083a`; the PR also
  integrates main through PR #170 (`d040518`). PR #171 remains the authoritative
  record of its final integration/documentation head, published checks and merge.
- Next (implemented in the preview subsection below): negotiated metadata-only capture bound to pin ID/version and current
  inventory/connection identities, local root identity/aliases, then trusted planner
  input and truthful review. A pin change away and back must invalidate old review
  even if the planner's content inputs match (bind the independent pin version).
  Full execution/journals/receipts and S5 remain later. Preserve the deferred PIN
  preference note and existing terminal jobs; do not ask users to stop sessions.

#### S4 negotiated metadata-only content preview — resumed after PR #171

Read [docs/skill-content-preview.md](docs/skill-content-preview.md). Branch
`claude/gallant-darwin-n9oqfj`, [PR #172](https://github.com/eyupio/reet.sh/pull/172).

- **Task and scope.** S4 negotiated per-file metadata preview only. The user resumed
  explicitly after PR #171 merged. No browser control, filesystem write, approval,
  staging, journal, rollback, durable receipt, baseline storage or adoption.
- **Behaviour added.**
  - `internal/skillpreview`: `Capture` (strict `Validate`, canonical `Hash`) and
    `Observe` (Linux/macOS). It reads only the connector's configured scope root with
    `os.Root` and S1's no-follow traversal and reports `present` (complete manifest of
    **every** file), `absent` (verified real root, no such entry) or `unknown` (any
    failed, partial or refused capture; issue codes and relative paths only). `rootId`
    hashes device+inode; every other configured scope root is compared, giving
    `exclusive` / `shared` / `unknown`. Shared, symlinked, unidentifiable or
    unprovable roots are reported and **never read**. A symlinked skill folder or any
    unsafe entry is `unknown`; a missing scope root is `unknown`, not absence. The
    non-POSIX stub never claims presence or absence.
  - `store_content_previews.go`: tables `skill_content_preview_capabilities` and
    `skill_content_previews`. Create names an owned pin ID and its exact version; the
    revision, bundle, name, mode and scope come from storage; the connector must have
    negotiated within 90 s and not be paused. Claim binds the inventory revision when
    capture begins (a legacy scan can finish between click and poll; none can run inside
    the claim-capture-report cycle). Report rechecks every fence in the writing
    transaction, builds `skillplan.Snapshot` from storage plus the validated capture
    (baseline **nil**, so any existing tree is `adoption_required`; enrolment generation
    is the connector ID) and stores the plan. `reviewHash` binds the pin version, so a
    pin that changes away and back invalidates the old review. Reads compute fences
    (`pin_changed`/`pin_removed`, `connector_paused`/`connector_removed`,
    `process_changed`, `inventory_changed`/`scope_removed`, expired, stale) without
    writing. 64 previews/account, one active per machine, request-key retries.
  - `api/content_previews.go`: browser routes under `/api/v2/skill-content-previews`
    and connector routes under `/api/v2/sync/skill-content-preview/{work,report,failure}`,
    behind the existing private-skill access wrapper, strict bounded JSON, no MCP change.
  - `scan/content_preview.go`: asking for work is the negotiation; 404/405/403 or a
    future schema leave files unread. The connector picks the root from its own
    configuration, refuses Codex, bad names and unenrolled projects, honours cancellation,
    and runs in `agentCycle` after import and before legacy polling.
  - Shared refactors, behaviour preserving: `reportedPinScope` (pin save and preview use
    one scope predicate), `skillScopeRoot` (import and preview) and `controllerPost`
    (one versioned-channel transport).
- **Compatibility / migration.** Additive `CREATE TABLE IF NOT EXISTS` tables, migrated
  after pins. Old connectors never negotiate; new connectors skip old controllers. No
  legacy desired set, hash, job, observation, import or terminal message changed. Full
  SQLite backups include the tables; shelf exports do not.
- **Checks actually run.** Final local run on the exact code (base `1eb92c0`; Go 1.27.1, Node 22.22.0, tmux 3.4 on PATH), in the required order: frontend `npm run check` (228 files, 0 errors, 0 warnings) and `npm run build` first; `go vet ./...` clean; `go test -race -count=1 ./...` with all 31 packages ok and 0 failures (internal/api 232 s, internal/store 128 s, internal/scan 23 s); MCP `npm test` 16/16; `git diff --check` clean. New code was written test-first (red for the right reason, then green). `internal/skillpreview` has race tests at 85.6% statements (the gaps are platform-defensive branches). Store, API and connector tests use real SQLite, a real HTTP controller and synthetic temporary homes: a new install into an absent skill; an existing tree whose bytes equal the library still `adoption_required`; an unmanaged local note preserved and never claimed; aliased, symlinked, linked-skill and missing roots reported but never read; project scope and unenrolled projects; the filesystem unchanged byte for byte; old, forbidden, paused and future-schema controllers triggering no file reads; redirect and cleartext refusal; cancellation; agent-cycle ordering; the ABA pin round trip; every fence before and during capture; corrupted private revisions never producing a plan; competing Store connections. Mutation checks (each break made the tests fail and the file was restored byte-identical): alias detection off, a truncated alias set trusted, a missing root treated as absent; pin version, process, inventory, pin removal, pause and staleness ignored; root ownership trusted blindly; the store's revision identity check removed; strict JSON off; connector identity skipped on report; cycle wiring removed. Cross-platform: `GOOS=darwin` build of `cmd/reet` and vet of the touched packages pass; the non-POSIX stub vets and test-compiles for Windows. The whole-binary Windows build fails identically on untouched `main` (`internal/scan/terminal_setup.go`), so it is pre-existing and not a CI target.
- **Not performed.** No live machine, production service, terminal session or physical
  cross-platform check. No browser UI was added or changed. Tests use synthetic homes,
  SQLite and HTTP. Nothing writes to a machine, so no recovery gate applies yet.
- **Known limitations and decisions.** "Exclusive" is among configured scopes only.
  Previews need a complete, recently reported inventory scope (the pin predicate).
  Inventory is bound at claim, not creation (reason above). Finished previews stay
  until dismissed, so a full history (64) needs dismissals via the API. A manual `reet`
  scan during capture supersedes the preview (fail-safe). There is no review UI, and
  pins have none either, so both are API-only. `adoption_required` had no resolution in this
  slice; the ownership receipts subsection below adds one.
- **Next unchecked work and first files.** (Written at the time; ownership receipts have since
  been delivered, see the next subsection.) The user chooses the order of: review UI with
  pin selection (`web/src/lib/api.ts`, `MachineSkillImports.svelte` for the pattern);
  ownership receipts and adoption design (`docs/skill-content-planner.md` items 3–4);
  reviewed executor with temporary-filesystem failure/crash/recovery gates.
- **Publication / deployment state.** Not inferred: the PR link is authoritative.

#### S4 ownership receipts and adoption — PRs #173 to #176

Read [docs/skill-content-receipts.md](docs/skill-content-receipts.md). Branch
`claude/gallant-darwin-n9oqfj`. The design is
`docs/superpowers/specs/2026-10-24-ownership-receipts-adoption-design.md` and the task plan is
`docs/superpowers/plans/2026-10-24-ownership-receipts-adoption.md`.

- **Task and scope.** S4 ownership receipts and explicit adoption: the baseline the planner
  has lacked, created only by a hash-bound decision. The user authorised stacking the work
  into several pull requests and merging each when green. API only. No browser control, no
  MCP tool, no filesystem write, no executor, no `applied` receipt, no ownership transfer
  between bundles, no claiming of locally edited files (S5) and no fleet apply.
- **Behaviour added.**
  - `internal/skillplan`: a baseline that does not fit now blocks instead of erroring,
    `baseline_target_mismatch` (root moved, adapter or schema changed) then
    `baseline_bundle_mismatch`, plan version still 1. Another owner's baseline, an invalid
    baseline revision ID and a non-canonical manifest stay errors. `ClaimSubset` (new
    `claim.go`) claims only files whose path, size, digest and executable bit equal a
    revision's, requires `SKILL.md` and returns counts; `IsSubset` compares a stored subset.
  - `store_content_receipts.go`: table `skill_content_receipts`, one row per account,
    environment and skill slot, with an independent random version, `provenance: adopted` and
    `verification: controller_attested`. List (metadata only; a row that no longer decodes is
    listed `damaged` and stays releasable), verified item read, version-checked release,
    the planner baseline lookup, `ReceiptCountsFor`, and adoption: eight checks in one write
    transaction, create-only, bound to the preview's review hash, consuming the preview,
    idempotent by request key. The managed subset is computed server-side; the caller supplies
    four values.
  - `store_content_previews.go`: the slot's receipt is the baseline; `receiptVersion` is in the
    review hash; fence `receipt_changed`; the informational `adoption` offer on single-preview
    reads only (captured, present, exclusive, only conflict `adoption_required`; at most 16
    candidates, best match then newest).
  - Lifecycle: a machine rename carries receipts (fresh version, destination collision rolls
    the rename back with 409); a generic scope merge is refused with 409
    `environment_has_receipts` (pins: `environment_has_pins`) and its dry run previews the same
    refusal; a non-forced environment delete refuses while receipts exist; typed sentinels and
    `PinCountsFor` back these.
  - `internal/api/content_receipts.go`: `GET`/`POST /api/v2/skill-content-receipts` and
    `GET`/`DELETE /api/v2/skill-content-receipts/{id}` behind the private-skill boundary, the
    status mapping, and audit events `skill-content-adopt` (logged once per decision) and
    `skill-content-release`; `web/src/lib/events.ts` Activity labels. MCP tool descriptions,
    `mcp/README.md` and `mcp_tools.json` name the new refusals.
- **Compatibility / migration.** Additive `CREATE TABLE IF NOT EXISTS`, migrated after the
  preview tables. No connector protocol change; old connectors are unaffected. No legacy desired
  set, hash, job, observation, import or terminal message changed. Previews gain an optional
  `receiptVersion`; earlier previews read as before. Planner version 1. Full SQLite backups
  include the table; shelf exports do not.
- **Checks actually run.**
  - Every task was written test-first, red for the right reason and then green, on real SQLite
    and HTTP with synthetic homes.
  - Every task had an independent reviewer. Tasks 1, 2, 4, 6 and 7a were approved without a
    fix round; Tasks 3, 5 and 7b each took one fix round (damaged-row listing, the adoption
    offer, the merge dry run) and were re-reviewed clean (Task 7b's fix round was read and
    accepted by the controller rather than re-reviewed by a separate reviewer).
  - Mutation checks per task, each break made a named test fail and the file was restored
    byte-identical: Task 1 had none of its own (red and green runs only; its baseline blocks
    were broken in the closing pass below); Task 2 two (claim equality, the `SKILL.md` rule); Task 3 four (the subset
    check, the identity rebuild, the release lock, the list guard); Task 4 one, plus the
    controller's review-hash binding; Task 5 fifteen, fourteen caught (the survivor, dropping
    the adoption step 5 pin check alone, is redundant defence because step 2 runs the same
    query); Task 6 four, plus the controller's `updated_at` bump; Task 7a twelve, eleven killed
    (the survivor, the `DELETE` nil check, is equivalent); Task 7b eighteen killed.
  - Broad review of PR #175's range (`d8af5e8..79d6c53`): safe to merge as the storage and
    adoption layer, no Critical finding. Its Minor findings were fixed with mutation checks
    (the offer's bundle limit, the offer never persisted); its Important findings, which were
    API-facing, were fixed in Task 7b.
  - PR #175 head `79d6c53`, local: `npm run check` 228 files with 0 errors and 0 warnings and
    `npm run build`; `go vet ./...` clean; `go test -race -count=1 ./...` 31 packages ok with no
    failures or races (tmux 3.4 on PATH); MCP `npm test` 16/16; `git diff --check` clean. Its
    published workflows were green on the head that merged (`be33e01`).
  - Final full validation for this pull request, local on `11ba84d` with tmux 3.4 on PATH and
    Go 1.27.1, in the required order: `npm run check` 228 files with 0 errors and 0 warnings,
    `npm run build`, `node src/lib/events.test.mjs`; `go vet ./...` and `go build ./...`
    clean; `go test -race -count=1 ./...` 31 packages ok with no failures or races; MCP
    `npm test` 16/16; `git diff --check` clean.
  - Closing mutation pass on `11ba84d`, in an isolated worktree with every file restored: both
    planner baseline blocks, claim equality (path only), the `SKILL.md` rule, create-only
    adoption, the review-hash binding, the `receipt_changed` fence and the receipt rename move
    were each broken in turn, and named tests failed every time (eight of eight).
  - Broad review of the PR #176 range (Tasks 7a, 7b, 8): no Critical finding; every new route is
    behind the shared boundary and the MCP path is refused; once-only audit, typed 409s and the
    receipt-aware delete confirmed. Its one Important finding, a documentation sentence
    claiming a mutation check that had not been run for the planner baseline blocks, is fixed
    (the check has now been run, and the sentence says when). Its Minor findings are fixed where
    they were wording and listed as follow-ups where they were code.
  - Published workflows on `11ba84d`: all five green (`test`, `verify`, `web` and `source`, the
    four release builds, the native service jobs and the tmux 3.2 minimum, the local relay jobs
    on macOS and Ubuntu, the terminal browser job, and CodeQL with its four analyses;
    `publish` is skipped on pull requests). The pull request is authoritative for the head
    that merges.
- **Not performed.** No live machine, production service, terminal session or physical
  cross-platform or fleet check. No browser UI. Nothing writes to a machine, so no recovery gate
  applies yet. Tests use synthetic homes, SQLite and HTTP. The whole-binary Windows build fails
  on untouched `main` in `internal/scan/terminal_setup.go`; that is pre-existing and not a CI
  target.
- **Known limitations and decisions.**
  - A receipt is the controller's attestation from a capture of at most five minutes old and the
    owner's say-so, never a check of the disk. `adopted` is the only provenance; no `applied`
    receipt exists, and initial installs record nothing until a future executor verifies.
  - Adoption claims only files equal to the chosen revision's, with `SKILL.md` among them. It is
    create-only, so re-adopting is release, preview again, adopt. Identical replay while the
    receipt exists returns the original (HTTP 200) and logs no second event; replay after a
    release conflicts; the same key with a new preview after a release is a new decision (keys
    end with their row; no tombstone).
  - The list shows digest and file count from the stored manifest without verifying it against
    its revision. The item read of a damaged row fails with the plain integrity error (HTTP 500
    "Could not access private skill storage."); the list flags it `damaged` and it can be
    released. Any error building a preview plan, a damaged receipt included, ends the request as
    `failed` / `plan_unavailable`; a transient database error is currently treated the same way.
  - Fences are live: only the preview used for adoption is persisted as consumed, so another
    preview of the slot that read `superseded` because a receipt existed can read `captured`
    again after release (bounded by the five-minute window and the inventory fence; it never
    allows two receipts for one slot). Fences compare versions only: a future executor must
    re-read the baseline through the store's validating lookup in its own transaction.
  - `POST /skill-content-previews` replaying a captured request returns no `adoption` block;
    `GET /{id}` does. A re-enrolled machine cannot reach `baseline_target_mismatch` (re-enrolment
    needs a revoke, which cascades receipts), so its next preview is `adoption_required`.
  - A forced environment delete, forgetting a machine or environment, revoking an enrolment and
    deleting an account still cascade receipts, by design; removing a pin does not remove the
    receipt. Not guarded: the automatic prune of empty server-side reports (practically
    unreachable) and the receipt count in a non-forced delete, read just before the transaction.
  - Limits: 1,024 receipts per account, 256 managed files per receipt, 16 KiB adopt body, 4 KiB
    release body.
  - Follow-ups: machine-merge 409s have no `code` field, so MCP and stdio clients lose the
    message; a receipts-aware guard for the empty-report prune; optionally persist `superseded`
    on every captured preview of the slot at adoption; an offer cost prefilter; transient
    database errors in plan building; a review UI and MCP exposure for receipts. From the last
    broad review: name the release audit event with a single-row lookup; record the receipts a
    forced environment delete cascades; a test with both skills and a receipt on one report to
    pin dry-run and delete ordering; the browser's per-report Forget drops receipts, which the
    review UI should surface; `AdoptSkillContentPreviewFor` now has no production caller.
- **Next unchecked work and first files.** The user chooses the order of: the review UI with pin
  selection and receipt review (`web/src/lib/api.ts`, `web/src/lib/MachineSkillImports.svelte`
  for the pattern, `docs/skill-content-receipts.md`); the reviewed executor with
  temporary-filesystem failure/crash/recovery gates (`docs/skill-content-planner.md` items 5 to
  7, `internal/skillplan/plan.go`, `skillContentReceiptBaseline` in
  `internal/store/store_content_receipts.go`, `skillContentReviewIdentity` in
  `internal/store/store_content_previews.go`). S4 cannot close, and S5 cannot start, before the
  executor and its gates pass.
- **Publication / deployment state.** PRs #173 (merge `f1fee4c`), #174 (`d8af5e8`), #175
  (`4fcd9fa`) and [#176](https://github.com/eyupio/reet.sh/pull/176) (`b486aa5`) are merged.
  Every check on #176's merged head `efbb2b1` was green: `test`, `verify`, `web` and `source`,
  the four release builds, the native service jobs and the tmux 3.2 minimum, the local relay
  jobs on macOS and Ubuntu, the terminal browser job, and CodeQL with its four analyses
  (`publish` is skipped on pull requests). Nothing is deployed, and a merge is not deployment.

#### S4 review UI, the Private skills dialog — PRs #178 to #185 and the closing pull request

Read [docs/skill-content-review-ui.md](docs/skill-content-review-ui.md). Branch
`claude/gallant-darwin-n9oqfj`. The design is
`docs/superpowers/specs/2026-10-25-review-ui-design.md` and the task plan is
`docs/superpowers/plans/2026-10-25-review-ui.md`.

- **Task and scope.** A browser flow over the three existing APIs: pin a library revision to a
  machine folder, check what the machine holds, review a file breakdown, and record or release
  ownership. The user authorised stacking the work into several pull requests, each merged when
  green. No filesystem write, no MCP tool, no apply, no executor, no `applied` receipt, and no
  change to terminals, the security/PIN preference or the legacy sync flows.
- **Merge history to know.** The user merged PRs #180 and #185 before their review fix rounds were pushed, so
  `main` held the Task 2 client logic (#180) and the Adopt and Release steps (#185) for a short while
  without those fixes; the fixes landed in #181 and in the closing pull request. The Adopt fix concerned
  two Important review findings (a refused adoption whose cause the screen cannot see said "Something
  changed" although the check still looked fine, and a lost reply with a failed records read left a
  recorded ownership unlisted). Neither allowed ownership to be recorded from a stale check.
- **Product direction (user, 25 October 2026).** The aim is a better multi-machine version of the website and
  the open-source app: agents, MCP servers and skills synced through reet.sh, with a professional UX and UI, and
  the ability to add a machine just in time so work with an AI agent can continue there. This slice is the
  groundwork for the skills part (per-machine pins, checks and controller-attested ownership). Candidate next
  slices, in the order I would suggest: add a machine in context from the dialog or Machines page; the reviewed
  executor that applies a pinned revision with rollback and verified receipts; a fleet overview with status
  per machine and per agent; MCP exposure of pins, checks and receipts. The order is the user's to choose.
  Do not name other products in the repository's docs or UI.
- **Follow-ups from the broad review (not done).** Split the ~1,500-line `skill-content.e2e.mjs` into pin,
  check and adopt/release files over a shared fixture module and consider running the 10-state layout matrix
  nightly; cache revision lists per bundle across reloads; a receipts-aware prune guard and what the dialog
  should say about Forget and merge dropping ownership records; MCP exposure and the reviewed executor.
- **Behaviour added.**
  - `GET /api/v2/skill-content-previews/{id}/adoption/{revision}` (PR #179): a metadata-only,
    read-only breakdown of which files adoption would claim, computed with the claim function
    adoption uses. Documented in the receipts contract.
  - `web/src/lib/contentReview.ts`: every label, state and decision rule, pure and unit tested;
    unknown codes read "Something changed. Check again." and never enable Adopt.
  - `MachineSkillContent.svelte` (dialog shell), `ContentPinStep.svelte`, `ContentCheckStep.svelte`,
    `ContentAdoptStep.svelte`, `ContentReceipts.svelte` and `checkWatcher.svelte.ts` (polling,
    request keys, busy and resume). Entry points are **Private skills** on each supported scope and
    **Pin a library revision** on each reported skill, in `MachineFleet.svelte` and
    `ContentObservation.svelte`. Accounts mode and a write role only.
  - Freshness is judged from when the browser received a response, never by comparing clocks: a
    captured read older than 30 seconds is "refreshing" until read again, and a captured check is
    read every ten seconds while shown. Adoption re-reads the check first and sends the review hash
    exactly as given. Request keys live in memory only; nothing is kept in browser storage.
  - Closing round: a size, theme, focus, Escape and reduced-motion matrix in
    `web/tests/skill-content.e2e.mjs`. Ten states at 1360, 768, 390 and 320px in light and dark; four
    walks checking focus on the opener, on the control that asked and on a heading after an action
    removes the control; Escape closing only the top dialog and writing nothing; and a reduced-motion
    walk with no running animation or transition in any dialog. No component needed changing.
- **Verification I ran for the closing pull request.** The browser fixture
  `web/tests/skill-content.e2e.mjs` passed four consecutive local runs against the production build
  (126, 120, 127 and 123 seconds) with `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/opt/pw-browsers/chromium`
  and `npm run preview -- --host 127.0.0.1 --port 4173`; the machine-import, control and
  public-workspace fixtures passed. An earlier run of the first version of the matrix failed once in
  an older check that moves the clock on while the tab is hidden (a read already under way was stamped
  fresh); the fixture now waits for that read first. An `axe-core` 4.10.2 scan run from a temporary
  directory outside the repository (no dependency added) covered the ten states at 375 and 1440px in
  light and dark, 40 scans, with no violations once transitions had finished; an unsettled first pass
  flagged button `color-contrast` mid-transition, which I took to be a timing artefact and did not
  confirm by hand. Details and limits are in [docs/ui-verification.md](docs/ui-verification.md). The
  full validation (frontend check and build, `go vet`, `go test -race` with tmux on `PATH`, the MCP
  tests, the mutation checks on the UI's key guards) and the published workflows are recorded in the
  pull requests, not repeated here.
- **Not performed.** No physical phone or tablet, screen reader, Safari or Firefox check; the
  fixtures use synthetic API responses in Chromium. No live machine, production service, terminal
  session or fleet. Nothing writes to a machine, so no recovery gate applies yet.
- **Known limitations and decisions.**
  - "Up to date" means the recorded files match the pinned revision, not that the machine is healthy.
    An ownership record is the controller's attestation from a recent check and is never described as
    verified; the sentence is "Recorded by reet from a recent check, not verified on the machine."
  - "Applying changes isn't available yet." is shown under any read-only change list.
  - The breakdown does not check the 1,024-record limit, so an account at the limit can see claims
    adoption then refuses; the screen words that refusal itself.
  - The dialog does not yet say that the machine page's per-report Forget drops ownership records
    with the report; that follow-up from the receipts record stands.
  - Follow-ups still open: the reviewed executor (the dialog will be reused for approval), MCP
    exposure for pins, checks and records, a fleet overview, bulk actions and a receipts-aware prune
    guard.
- **Next unchecked work and first files.** The reviewed executor with temporary-filesystem
  failure, crash and recovery gates (`docs/skill-content-planner.md` items 5 to 7,
  `internal/skillplan/plan.go`, `skillContentReceiptBaseline` in
  `internal/store/store_content_receipts.go`, `skillContentReviewIdentity` in
  `internal/store/store_content_previews.go`). S4 cannot close, and S5 cannot start, before the
  executor and its gates pass.
- **Publication / deployment state.** Recorded on the pull requests: #178 to #185 are merged, and
  the closing pull request carries the final head, the workflow results and the merge. Nothing is
  deployed, and a merge is not deployment.

#### Add a machine in context — PRs #188 to #190 and the closing pull request #191

Read [docs/add-machine-in-context.md](docs/add-machine-in-context.md). Branch
`claude/gallant-darwin-n9oqfj`. The design is
`docs/superpowers/specs/2026-10-25-add-machine-in-context-design.md` and the task plan is
`docs/superpowers/plans/2026-10-25-add-machine-in-context.md`.

- **Task and scope.** Bring a named machine in without leaving the screen you are on: one block of
  commands to paste there, a status line that watches the controller for it, and a landing back where
  the person started. Accounts mode and a write role only. It writes nothing to a machine and sends
  nothing but the existing token request and the page's own machine reads. No server code, terminal,
  security/PIN preference or legacy sync flow changed.
- **Pull requests.** The design and the plan were [#188](https://github.com/eyupio/reet.sh/pull/188) and
  [#189](https://github.com/eyupio/reet.sh/pull/189). The dialog, the Machines page entry and the join
  logic merged as [#190](https://github.com/eyupio/reet.sh/pull/190). Its review found three
  things, all fixed before the merge: a machine name is limited in bytes, as the server counts it, not
  in characters; a name saved as an alias after a rename or merge now resolves to its saved machine
  (the watch, the baseline and the Use button follow it); and landing a new machine on a Machines page
  filtered to another machine (`?machine=`) now clears the filter so the new card exists to be shown.
  This closing pull request adds the Private skills entry and landing, the layout matrix, three small
  hardening fixes and the documentation.
- **Behaviour added.**
  - `web/src/lib/machineJoin.ts` (pure, unit tested in `machineJoin.test.mjs`): the name rules, the one
    command block (the three existing builders, never a token), `joinState` (waiting, connected,
    first scan, ready, already here, unknown), `landingFor` (which folder of the new machine the
    Private skills dialog may open on), `landingNote`, the poll delay and the 15-minute watch cap.
  - `AddMachineDialog.svelte`: the three parts (name, key, run), a polite status line, polling that
    pauses while the tab is hidden, backs off on failure and stops on a lost sign-in, and *Use* as the
    single primary action. From the Private skills dialog it also pauses three seconds then carries on
    by itself, with a **Cancel**, and offers **Wait here** and **Close** when no folder qualifies.
  - Entry points: **Add a machine** on the Machines page (`EnvironmentSync.svelte`,
    `MachineFleet.svelte`) and **Not listed? Add a machine** in the Private skills dialog
    (`MachineSkillContent.svelte`), which passes the skill and agent it came with.
  - The landing: from Machines the card is scrolled to and marked "Just added"; from Private skills the
    dialog opens on the folder `landingFor` chose when Use was pressed, with a note saying which folder
    and how many others qualified. The machine list now judges that folder again at the instant it
    opens (a folder that no longer qualifies opens nothing and says so), focus falls back to the Add a
    machine button when the folder's own button is not drawn, and the note goes into a status region
    that is already on the page.
- **Tests.** `web/src/lib/machineJoin.test.mjs` (in `npm test`) and `web/tests/add-machine.e2e.mjs`
  (run in the terminal-browser workflow). The fixture covers every state, the Private skills landing,
  the hardening cases and a matrix: ten dialog states and the sign-in card at 1360, 768, 390 and
  320px in light and dark with no sideways scroll, the command block wrapping, the primary action
  reachable and uncovered, Escape closing without a write and returning focus, and a reduced-motion run.
- **Verification I ran for the closing pull request.** The fixture passed four consecutive local runs
  against the production build (53, 52, 56 and 52 seconds) with
  `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/opt/pw-browsers/chromium` and
  `npm run preview -- --host 127.0.0.1 --port 4173`; the skill-content, machine-import, control and
  public-workspace fixtures also passed. Each of the three behavioural fixes had a fixture assertion that failed before its fix, and breaking the
  command block's wrapping on purpose made the matrix fail. An `axe-core` 4.10.2 scan run from a
  temporary directory outside the repository (no dependency added) covered the eleven surfaces at 375
  and 1440px in light and dark, 44 scans, with no violations; `color-contrast` was reported as
  incomplete in 40 of them and I did not review those. Details and limits are in
  [docs/ui-verification.md](docs/ui-verification.md). Full validation results are recorded in the
  pull request.
- **Not performed.** No physical phone or tablet, screen reader, Safari or Firefox check; the fixtures
  use synthetic API responses in Chromium. No live machine, production service, terminal session or
  fleet. Whether a screen reader announces the landing note was not heard. The landing's second look
  was exercised by moving the browser clock past the 14-day window, not by pausing a connector at the
  same instant.
- **Known limitations and decisions.**
  - *Connected* and *Ready* say only that a connector with that name reported in and that a complete
    scan arrived. They do not say the commands worked, and the words "verified" and "healthy" are never
    used of a machine.
  - The bundle the Private skills dialog had chosen is carried in the dialog's context but not
    preselected; the landing finds the library skill by name.
  - An alias whose machine was forgotten is not visible to the page, so the dialog waits for the typed
    name until the watch cap.
  - An old machine and a new enrolment that share one name can make the old machine's newer scan read as
    Ready; use a distinct name.
  - When several folders qualify, one is opened and the note counts the rest; the person is not asked.
- **Follow-ups (not done).** Split the fixture (it now holds the entry, the join states, the landing and
  the matrix) over a shared fixture module and consider running the matrix nightly; preselect the
  bundle on the landing; take the alias map from the API instead of inferring it from the machine list;
  the reviewed executor that applies a pinned revision (the apply flow would reuse the landing); a
  fleet overview with status per machine and per agent; MCP exposure of pins, checks and receipts.
- **Direction.** The aim is unchanged: a better multi-machine version of the website and the
  open-source app, with agents, MCP servers and skills synced through reet.sh and the ability to add a
  machine just in time so work with an AI agent can continue there. This slice delivers the "add a
  machine just in time" part for skills; the order of the next slices is the user's to choose. Do not
  name other products in the repository's docs or UI.
- **Publication / deployment state.** Recorded on the pull requests: #188 to #190 are merged, and the
  closing pull request carries the final head, the workflow results and the merge. Nothing is
  deployed, and a merge is not deployment.

#### Remaining S4 integration and apply gates

The preview slice meets item 1 and the review-only part of item 2 below, and the ownership
receipts slice supplies a controller-attested baseline for item 3. Approval binding, verified
ownership, execution and everything after remain open.

1. Feed owned pinned desired revisions into negotiated review. Preserve legacy
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

> Continue reet.sh from handoff.md. Inspect branch, worktree, main and PR states without
> resetting edits. Terminal T1–T9 are merged through PR #155. S2 private revisions merged
> in PR #156; S3 is complete through PRs #161/#164/#168. S4's pure planner merged in
> PR #169, owned revision pins in PR #171 (`1eb92c0`), the negotiated metadata-only
> content preview in [PR #172](https://github.com/eyupio/reet.sh/pull/172) (`ebf58f4`), and
> ownership receipts with explicit adoption in PRs #173 to
> [#176](https://github.com/eyupio/reet.sh/pull/176) (`b486aa5`). PR #170's staged reet
> connector updates are integrated; they are not
> AI-agent software maintenance. Read docs/skill-content-receipts.md,
> docs/skill-content-preview.md, docs/skill-content-targets.md and
> docs/skill-content-planner.md. The review UI (the Private skills dialog, PRs #178 to #185
> and a closing documentation pull request) is described in docs/skill-content-review-ui.md.
> Nothing writes to a machine yet; the only baselines are
> controller-attested adoption receipts, and a tree with none needs adoption. The next slice
> is the reviewed executor, unless the user chooses another follow-up. Keep terminal sessions, the deferred user-controlled PIN/security preference,
> exact byte/mode manifests and stable retry receipts intact; do not jump to remote apply
> or the whole roadmap. Build the frontend before Go checks, record actual evidence,
> update this handoff and publish one reviewable PR per bounded stage.

## Start an agent session — protocol and connector (PRs #193, #194)

Added 25 October 2026. Design: `docs/superpowers/specs/2026-10-25-start-agent-session-design.md`.
Contract: [docs/terminal-launch.md](docs/terminal-launch.md).

**What exists.** A negotiated, optional launch request on the create-session call
(header token `launch-v1`, capability `launch` plus `tools`, no version bump). Four flat
string fields (`tool`, `model`, `dir`, `prompt`) keep `terminalproto.Create` comparable; the
whole `Create` is the idempotency key. The connector holds an allowlisted preset table for
`claude`, `codex`, `opencode` and `gemini`, offers a tool only if its binary is on `PATH`, and
starts it through tmux as separate arguments in a folder it resolves inside the enrolled home.
The API answers 422 `prompt_unsupported`, `tool_not_installed` and `folder_unavailable`, and
409 `launch_unsupported`; a launch writes the audit action `terminal-launch` (never the
prompt). There is no web dialog and no entry point yet.

**What was checked.** Only what the task reports record: preset flags confirmed from the
cited help text and documentation; argument vectors, folder policy and `#`/`;` escaping
exercised against a real tmux with recording fake CLIs; a real two-host PTY test that
asserts the recorded argv and working directory, no second start on retry, and that a plain
create still opens a shell; mutation checks on the prompt rule, the capability check, audit on
replay and the tool lookup. tmux 3.2 semantics were read from source, not run. Full
validation results are recorded in the pull request.

**Rulings made.**
- The controller sends a tool id, never a program or shell string; the connector's built-in
  table decides.
- Prompt storage: the opening prompt is kept in the controller's create-operation row and the
  machine's private registry (the idempotency record), never in audit, logs or browser storage,
  and is visible in the machine's process list.
- One-word prompts are refused for `claude` and `codex` (`prompt_unsupported`) because a bare
  subcommand name after `--` would run; a leading `-` is refused for every tool.
- Folder boundary is the enrolled home, symlinks followed; wider roots would be a deliberate
  later setting.
- A tool whose syntax cannot be confirmed is not offered (`pi` and `cursor` are not).
- The audit action is hyphenated, `terminal-launch`, like the other actions.

**What remains.**
- The web dialog and entry points (Terminals workspace and the machine card), with the
  refusal sentences mapped from the codes above.
- The Activity label for `terminal-launch` in `web/src/lib/events.ts` (add `tool`, `model`
  and `dir` to its `Detail` type).
- The layout matrix (320, 375, 768 and 1440px, light and dark) and the browser fixture.
- Projects.
- Agent status, resuming a conversation, worktrees, MCP exposure and wider folder roots remain
  out of scope.

**Product direction.** The owner calls this core to the new reet.sh. The core roadmap is Start
an agent session, then Projects: multi-repo named sets of repository folders across machines,
with a default agent and worktrees.

A started session means only that the connector started the program in tmux in that folder.
It does not show that the agent is signed in, understands the prompt or is making progress.

### Start an agent session — web side (PR #195)

Added 25 October 2026. Read [docs/start-agent-session.md](docs/start-agent-session.md) for what
people see. This extends the "What remains" list above: the web items there are now done.

**What exists now.**
- `web/src/lib/agentLaunch.ts` (pure, unit tested in `agentLaunch.test.mjs`) holds the tool names,
  model suggestions, the model, folder and prompt rules, folder choices, the sentence line,
  `refusalText`, the title and the Activity line.
- `web/src/lib/StartAgentDialog.svelte` is the dialog: one screen, one request id per attempt, a
  busy guard, and the existing 15-second abort.
- Entry points: **New agent session** in the Terminals workspace (sidebar, machine menu, empty
  state) and **Start an agent** on a machine's card (disabled, with the reason, while the machine
  isn't ready).
- Activity reads `terminal-launch` as "Started \<Agent\> [with \<model\>] in \<folder\> on
  \<machine\>".
- Browser fixture: `web/tests/start-agent.e2e.mjs`, run in the terminal-browser workflow.

#195 also adds:
- A status line that clears a validation message when the person edits a field.
- A fixture hardened to check every field is locked while a request is held or its outcome is
  unknown, Escape on a held first Start, a positive heading anchor for viewers and legacy sign-in,
  and IndexedDB opens and writes.
- The layout, focus and motion matrix.
- Three layout and accessibility fixes it found: the footer covered fields on narrow screens, the
  "Not installed" reason was faded below contrast, and the locked form could not be scrolled by
  keyboard.
- This documentation.

**Rulings recorded.**
- A 503 keeps the request id, like a lost reply: the machine may already have been asked. Retry
  sends the same id and body.
- Lost-reply caveat: closing the dialog after a lost reply forgets the request, because the prompt
  must not outlive the dialog. The dialog says plainly that a second agent may start if the person
  closes it and starts again, and to check their sessions first.
- One-word prompt rule: Claude Code and Codex need at least two words ("Write the opening prompt
  as at least two words."). It is checked in the page and refused by the server as
  `prompt_unsupported`.
- Session-limit wording: "The terminal session limit has been reached. End a session and try
  again.", because the server's 409 doesn't say whether the machine's limit or the account's limit
  was reached. The tool refusal is the generic "That agent isn't installed on that machine.",
  because the API answers with a code and has no table of names. The spec's Words table was
  amended to match `refusalText` row for row, and is kept that way.
- A create that answers 200 with a session the machine did not start (`lost` or `ended`, for
  example when local consent was switched off just before) is a definite outcome: the dialog says
  "That machine didn't start the agent. Check its terminal settings on that machine and try
  again." and the next Start is a new request. A 401 says "You've been signed out. Sign in again to
  continue." and, on the Machines page, hands over to the sign-in card.
- While a request is held or its outcome unknown, the tool, choices and sentence come from the
  request itself; a machine-list refresh cannot change them.

**What was checked.**
- Only what was run for these tasks, in Chromium against synthetic API responses: the fixture's
  states and refusals, the matrix (24 states at 1360, 768, 390 and 320px in light and dark),
  Escape at every size from both entry points, a reduced-motion run, and no browser storage.
- An `axe-core` scan from a temporary directory. Results and limits are in
  [docs/ui-verification.md](docs/ui-verification.md).
- No screen reader, Safari, Firefox or physical device.
- Full validation results are recorded in the pull request.

**What remains.**
- Projects.
- Agent status (working, waiting, idle).
- Resuming an earlier agent conversation.
- Wider folder roots as a deliberate setting.
- Worktrees and MCP exposure stay out of scope.
- The terminal pane's mobile key row is a scrolling region that can't be reached by keyboard. An
  axe scan flagged it on the Terminals page at 375px. It is outside this dialog and was not changed.

**Product direction.** The core roadmap is Start an agent session, which is now in place from the
browser, and then Projects: named sets of repository folders across machines, with a default agent
and worktrees, reusing the folder list and the launch request.

## Projects PR 1 — worktrees on the machine

Plan: [docs/superpowers/plans/2026-10-25-projects.md](docs/superpowers/plans/2026-10-25-projects.md).
Spec: [docs/superpowers/specs/2026-10-25-projects-design.md](docs/superpowers/specs/2026-10-25-projects-design.md)
(amended where the implementation found it overclaimed). What the code does:
[docs/terminal-worktrees.md](docs/terminal-worktrees.md). The spec and plan are on PR #201; the PR 1
implementation commits are on the working branch and had not been opened as a pull request when
this section was written.

**What is in place.**
- Protocol: `Create.worktree` (run token, lower-case hex 16–48) and `Create.project` (slug), feature
  `worktree-v1`, capability `worktree` (requires `launch` and git on `PATH`), and the
  `worktree_status`/`worktree_remove` requests answered by `worktree_state`/`worktree_result` with a
  reason and a plain detail.
- Connector: makes `~/.reet/worktrees/<project>/<repo>/<run>` on branch `reet/<project>-<run>` with
  one `git -c core.hooksPath=/dev/null worktree add`, refuses anything that is not the top of a
  normal repository or is mid-merge/rebase/cherry-pick/revert/bisect, adopts its own worktree on a
  retry, and cleans up only what a failed call made. Status reports changed, unpushed,
  unreferenced, hidden, nested and incomplete. Remove refuses unless the counts equal what was
  shown and nothing uncommitted, hidden, unreferenced or nested would be lost, then runs
  `git worktree remove` without `--force`. The branch is never deleted.
- Controller: the broker sends worktree fields and requests only to a connection whose stored
  capabilities report `worktree`, waits up to 180 seconds for them (raised from 100 in fix round 2), keeps the refusal reason, and
  drops late worktree replies; the control connection routes worktree replies (before this, any
  reply type other than result, reconcile and session change closed the connection). The API takes
  `worktree` and `project` on create, adds `POST /api/v2/terminal/worktrees/status` and
  `/remove`, maps every reason to a code and a sentence, and audits `worktree-remove` (machine,
  host, path, branch) from the machine's `ok` answer, which since the fix round names the folder and
  branch (a remove whose answer is lost is confirmed by one status call and audited with
  `confirmedBy: status`). `terminal-launch` gains `worktreeRun`, `project`,
  `branch` and, since the fix round, `worktree` (the folder): the create reply now carries
  `worktreePath` and `worktreeRepo`, which are stored on the session and returned in its JSON.

**What was run.** Linux only (git 2.43.0, tmux 3.4, Go 1.27.1): the package race tests, the API
terminal tests, and a real two-host test (`TestTerminalPrivateWorktreeStartsTheRecordedAgentInItsOwnWorktree`)
with real git, real tmux and a recording fake agent CLI: the agent started in the worktree, a retry
made nothing new, a machine without the capability got the update sentence while a plain shell and
a launch still worked there, a plain folder, a subfolder, a real rebase conflict and an existing
branch were refused with nothing left in tmux, status counted an edit, stale counts and shown
changes were refused, a commit was counted as unpushed and kept on the branch after removal, viewers
were refused, and the prompt was in no audit row. Mutations of the capability check, the late-reply
handling, the control routing, the feature header, the timeout, the audit placement and the
validation were each caught. Nothing was run on macOS outside CI; CI on macOS had not run on this
work when this was written.

**Rulings taken during PR 1.**
- Implementation commits stay local and are rebased on main after the spec and plan PR merges.
- One 60-second budget covers the whole create, status or remove on the machine (each git call is
  also capped at 60 seconds); cleanup has its own 30 seconds.
- `worktree list --porcelain` is read without `-z` so git 2.34 still works; no extra `-c` flags are
  added to the fixed commands.
- Listings (`worktree list`, `status`, `ls-files`) are capped at 4 MiB, not the 4 KiB in the plan:
  4 KiB breaks adoption and removal past about 17 worktrees. A listing that reaches the cap is not
  trusted.
- `has_changes` always blocks removal, even when shown; only unpushed commits can be shown and then
  removed with, because they stay on the branch.
- The run token is chosen by the caller, never generated by the controller.
- The spec's "cannot run repository code" and "three git calls" were corrected: hooks are disabled
  but repository configuration (filters, fsmonitor) still applies, and the read-only and
  failure-path commands are listed.

**Known limits.**
- An agent still running in the worktree is not checked; offer *Remove worktree* only for ended
  sessions.
- Ignored files (`.env`, `node_modules`) are deleted with the worktree; ignored nested repositories
  and worktrees with more than 200,000 entries are refused instead.
- A helper that daemonizes escapes the process-group kill on timeout.
- Fixed in the PR 1 fix round: the connector used to run every control message on its reader, so a
  long worktree request could leave health probes unanswered until the controller marked the
  machine unreachable and disconnected it. Probes are now answered on the reader, other messages go
  to one dispatcher, worktree status and remove run on their own goroutine, and a second concurrent
  worktree request is refused at once as `worktree_busy`. A worktree create still occupies the
  dispatcher while git runs: that machine's other creates and ends, worktree status and remove,
  and attachment renewals (every 5 seconds, given up after 10) wait or fail, so open terminals on
  that machine may detach during a slow create and during a remove that waits behind one.
- Fix round 2: a request that finds the connector's 64-message queue full is answered
  `unavailable` by the reader and the connection is kept (it used to end the session). A remove
  takes at most 60 seconds on the machine (30 for checks, 30 for `git worktree remove`); a late
  `ok` is still audited with `late: "true"`; if the connection is lost before the controller hears
  the result, the removal may not be audited. The controller refuses answers whose folder or
  branch is not the requested run's (`worktree_mismatch`).
- Status and remove must name the repository by the session's `worktreeRepo` (the name the machine
  used); a name recomputed from a symlinked member folder answers `not_found`. A create whose reply
  is lost entirely has no recorded folder.
- A create with a new request id and the same run token adopts the worktree and starts a second
  agent; the Projects run must reuse each member's derived request id for unknown outcomes.
- For PR 2 (recorded in the plan's Task 7): the worktree run token is derived per member,
  `derive(run, position)`, never one per run; members with the same machine and folder are refused
  as a duplicate repository at save and start (folders that only resolve to the same repository on
  the machine are not caught, but get separate worktrees); status and remove use the create reply's
  `worktreeRepo`.
- Mixed controller and connector versions are compatible: worktree fields and requests are only
  sent after `worktree-v1` is negotiated, and the new reply fields only answer those requests.

**PR 2 (the controller) is in this pull request.** It adds the Project store, the Projects API
and audit, and the run orchestration (`internal/projectrun`). The API shapes:
- `GET/POST /api/v2/projects`, `GET/PATCH/DELETE /api/v2/projects/{id}`;
- `GET /api/v2/projects/{id}/runs` (newest 20) and `GET /api/v2/project-runs/{runId}`;
- `POST /api/v2/projects/{id}/runs` with `{requestId, positions}`: 201 with the new run, 200 for a
  replay of the same request, 409 `request_changed` or `run_in_progress`, 422
  `request_id_required`;
- `POST /api/v2/project-runs/{runId}/members/{position}/retry` with `{}`.

Run members carry `state` (`pending`, `started`, `failed` with `code` and `sentence`, `removed`),
`sessionId`, `worktreePath`, `worktreeRepo`, `worktreeBranch`, `worktreeRun`, `worktreeProject`,
`sharesCheckout` and `attempt`. Audit rows: `project-create`, `project-update`, `project-delete`,
`project-run`, `project-run-retry`, and `terminal-launch` per started member; none carries the
prompt. Caveats recorded for PR 2:
- a single controller process (claims and per-machine slots are in memory);
- one create per machine at a time, so a 12-member run on one machine can take about 36 minutes in
  the worst case (12 worktree creates of up to 3 minutes each);
- a run without worktrees starts agents in the saved folder, which they share with anything else
  there (`sharesCheckout`);
- merging or revoking a machine leaves its members `unknown_machine` until the Project is edited;
- a database created from earlier commits of this branch must be recreated (the Project tables
  changed shape before release); a database from main upgrades cleanly (opened twice in a probe).
See [Worktrees](docs/terminal-worktrees.md), "The run orchestration".

**What remains.** PR 3: the web (Projects page, Project page, dialogs, browser fixture). PR 4: the
layout matrix, `docs/projects.md`, the UI verification section, README sentence and the closing
handoff. The landscape review runs after PR 2 merges.

### Roadmap ideas recorded

Recorded from the product owner's review; none is designed yet. Projects comes first; the chat
layer is the next slice and starts with a spike.
- A structured chat layer over every agent: a mode (build or plan), a model list and a thinking
  effort setting, alongside the terminal.
- Extra repositories attached to one session.
- Sandboxing for agent sessions.
- Session groups.
- Allowlisted per-agent flags in place of free-form arguments or a command override.
- Permission auto-approve is deliberately not offered; it would need a loud, audited, per-session
  opt-in.
- A landscape review workstream: compare comparable open projects against reet.sh's intended
  capabilities (see the plan), produce a private comparison and a ranked list, and record accepted
  items here in neutral wording.

## Projects — delivered (PRs #210, #211, #212 and Projects PR 4)

Added 26 October 2026. Read [docs/projects.md](docs/projects.md) for what people see, the routes and
JSON shapes, the limits, states, codes and sentences, and the known limits;
[docs/terminal-worktrees.md](docs/terminal-worktrees.md) for the worktree contract on the machine;
the "Projects" section of [docs/ui-verification.md](docs/ui-verification.md) for what the browser
checks and the accessibility scan found. Spec and plan:
`docs/superpowers/specs/2026-10-25-projects-design.md`, `docs/superpowers/plans/2026-10-25-projects.md`
(#201). This section supersedes the "What remains" list in "Projects PR 1" above.

**What exists.**
- #210 (merged as `801b4d5`): worktree create, status and remove on the connector, the `worktree-v1`
  feature and `worktree` capability, the broker and the `POST /api/v2/terminal/worktrees/status` and
  `/remove` routes.
- #211 (merged as `f32f39d`): the Project store (50 per account, 12 repositories each), the Projects
  API with `If-Match`/`ETag`, idempotent run start by `requestId`, the run orchestration
  (`internal/projectrun`: derived request ids and worktree tokens, the durable `sent` mark, two in
  flight per run, one create per machine, resume after restart) and the audit actions.
- #212 (merged as `cfde296`): the Projects list, the Project page, the create and change dialog, the
  Start all sheet, the run view, Remove worktree and Delete, the Activity labels, the browser fixture
  `web/tests/projects.e2e.mjs`, and on the server: DELETE and run start honour `If-Match`, `projects`
  is a reserved hosted username, and single sign-on returns to `/projects`.
- Projects PR 4 ([#213](https://github.com/eyupio/reet.sh/pull/213)): the layout, focus and motion matrix in the fixture (19 states at
  1360, 768, 390 and 320px in light and dark, Escape at every size, reduced motion), the axe scan and
  its one fix (the Remove worktree details are a focusable region), `docs/projects.md`, the UI
  verification section, the README sentence and this record. No Go change.

**Rulings taken during Projects.**
- Each member gets its own worktree token, `derive(run, position)`, never one per run and never per
  attempt; status and remove name the repository by the machine's reported `worktreeRepo`.
- A member is durably marked `sent` before its create goes to the broker. Retry reuses the same
  request id while the mark is set and takes a new attempt only when the create is known to have
  started nothing, because a new request id with the same token would adopt the worktree and start a
  second agent. The broker answers a reserved request id before checking capabilities.
- Routes: runs are read at `GET /api/v2/project-runs/{runId}` (not under `/projects/{id}`), retried at
  `POST /api/v2/project-runs/{runId}/members/{position}/retry`.
- Every change and the run start carry the version read (`If-Match`); a stale one is 409
  `project_changed` with nothing changed, created or audited. Absent `If-Match` means no
  precondition; the web always sends it.
- A run start needs a client `requestId` (one run per id, replay 200, changed positions 409
  `request_changed`), and a Project with pending members refuses a new run (`run_in_progress`).
- `has_changes` always blocks a remove; only unpushed commits can be shown and removed with. The page
  offers Remove only for a started member whose session it has read as ended, and reads the sessions
  again just before removing.
- One 60-second budget covers a worktree operation on the machine (30 more for cleanup); the
  controller waits up to 180 seconds; listings are capped at 4 MiB.
- Merging or revoking a machine rewrites no Project or run: its members fail `unknown_machine` until
  the Project is edited; an edit may keep a revoked machine's already-saved repository.
- The shared prompt is never in a list, save reply, run JSON, audit row, log or browser storage.
- Database: the Project tables changed shape on the working branch before release; a database made
  from those earlier branch commits must be recreated.
- The fixture's mock imitates the Go handlers, so If-Match is also guarded by explicit header
  assertions.

**What was checked.** Recorded in each pull request and in docs/ui-verification.md: Go race tests,
a real two-host test with real git and tmux on Linux (macOS only in CI), mutation checks on the
safety rules at every step, and the browser fixture in Chromium against synthetic answers. No screen
reader, Safari, Firefox or physical device. Contrast was not measured by hand: axe left
`color-contrast` undecided in 62 of 76 Projects scans.

**What remains.**
- A git remote as a second, optional repository identity, so a Project follows you to a newly added
  machine.
- Agent status (working, waiting, idle) and resuming an earlier agent conversation.
- One session across several repositories where an agent supports it, pull request creation, and
  MCP exposure of Projects and runs.
- The single-controller assumption: claims and per-machine slots are in memory.
- Remove worktree for a member whose create reply was lost (no recorded folder or repository name).
- Renewals and other session operations on a machine still wait behind a worktree create.
- The terminal pane's mobile key row is still a scrolling region that can't be reached by keyboard
  (outside Projects).
- A screen reader pass, Safari and Firefox, and a physical phone.

**Roadmap ideas (neutral; none designed yet).** The chat layer is the next slice and starts with a
spike.
- A structured chat layer over every agent: a mode (build or plan), a model list and a thinking
  effort setting, beside the terminal.
- Extra repositories attached to one session.
- Sandboxing for agent sessions.
- Session groups.
- Allowlisted per-agent flags in place of free-form arguments or a command override.
- Permission auto-approve is deliberately **not** offered. It would need a loud, audited,
  per-session opt-in, and reet.sh does not offer switches that skip an agent's own permission
  prompts.
- The landscape review of comparable open projects stays a separate workstream: its comparison is
  kept privately, and accepted items are recorded here in neutral wording without naming any
  product.

## Phone terminal — keyboard, size and repaint (26 October 2026)

**Problem.** Agent sessions were unusable on a phone: two rows with the keyboard open, about
41 columns so the agent's layout wrapped, chrome taking most of the screen, a blank band above the
keyboard, and a blank terminal on returning to the tab.

**What changed (web only; no Go, protocol or tmux change).**
- `web/src/lib/terminal/sizing.ts` (new, unit-tested in `web/tests/terminal-sizing.test.mjs`):
  phone font default, the 20×8 size floor, a resize sender (150 ms debounce, never resends a size
  the host has, a one-row repaint nudge), the usable viewport height and keyboard detection.
- `TerminalPane.svelte`: phones default to 11 px when no font is stored; one-line pane header and
  rows without vertical padding (toolbar and keys at least 40 × 40 px on touch screens); overflow fades on the toolbar and key
  row; the key row is focusable; the Alt+Shift+M advice is hidden on touch screens; the size sent
  is floored at 20×8 with the bottom rows kept in view; a reattach, or a return to the page that passes the
  gate in the fix round below, sends one repaint nudge (one row shorter, or taller at the floor,
  then the real size 300 ms later, using the existing resize message); the footer hides while the keyboard is open; a maximised pane follows the usable
  height and offset.
- `TerminalWorkspace.svelte`: the keyboard focus layout. On a narrow touch screen, with the focus
  in the terminal and the height at least 25% below the tallest seen, the site header and the
  workspace toolbar are hidden (`html[data-terminal-keyboard]`); Show sessions stays in the
  terminal toolbar.
- Viewport meta: `interactive-widget=resizes-content`, set by the terminal workspace while it is
  mounted (first in `web/index.html`; moved in the fix round). When the page itself shrinks for the
  keyboard by 15% or more, `innerHeight` is used instead of a lower `visualViewport.height`.
- `connection.ts`: `resize()` reports whether the frame was sent, so an undelivered size is not treated as sent
  (the next layout change or ticket carries it; there is no automatic retry).
- Fixture `web/tests/terminal-phone.e2e.mjs` (added to `terminal-browser.yml`); docs in
  `docs/terminal-workspace.md` (Phones) and `docs/ui-verification.md`.
- The extra-key row is now reachable by keyboard, which resolves the earlier
  `scrollable-region-focusable` note on `.mobile-keys`.

**Fix round after review.**
- The repaint nudge on a return to the page now needs a touch screen or 30 s hidden, and never runs
  twice within 5 s (`repaintOnReturn`). Reattach still always nudges. It runs on every screen size.
- At the 8-row floor the nudge goes one row taller (9, then 8) instead of below the floor.
- `interactive-widget=resizes-content` is no longer in `index.html`; the terminal workspace adds it
  while mounted and restores the previous meta when it unmounts.
- Sideways phones (touch, landscape, 500 px tall or less) get the phone rules, with the toolbar and
  extra keys in one row and no footer; while typing, the pane header also steps aside. Fixture
  numbers: 13 rows at 844 × 390 (15 at 915 × 412), 8 with the keyboard open, 9–10 maximised.
- Toolbar buttons and extra keys are at least 40 × 40 px on touch screens.
- `?viewport-debug=1` shows a live readout of the measured heights (below).

**Needs a real-phone check.** The blank band is explained by a hypothesis that the emulation can't
prove: an Android browser's visual viewport still subtracting a bottom toolbar it hides while the
keyboard is open. On the owner's phone:
1. Open a terminal and add `&viewport-debug=1` to the address (or `?viewport-debug=1` when the
   address has no `?`). A small dark readout appears in the top corner. It ignores touches and never
   takes focus.
2. Tap into the terminal so the keyboard opens. Note the readout: `visualViewport.height`,
   `offsetTop`, `innerHeight`, `screen.height`, the used height and `keyboard open/closed`.
3. Repeat with the keyboard closed, maximised and not, and held sideways. A screenshot of each is
   enough.

What the numbers mean:
- `innerHeight` drops with the keyboard: the browser honours the meta set after load, and the used
  height should match the visible area (no band).
- `innerHeight` does not drop and `visualViewport.height` is about 90 px short of the visible area:
  the band can remain. The next step would be a per-browser correction, which needs these numbers.
- Also check: rows with the keyboard open (expect at least 15 maximised at 390 px wide, about 8
  sideways), that the agent's screen repaints after switching apps and back, and that a long-idle
  tab that reloads shows the agent rather than a blank screen.
- Not covered yet: dialogs and other pages on sideways phones.
