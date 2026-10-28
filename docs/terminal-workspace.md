# Terminal workspace

## Home and the tabs

Signing in lands on **Home** (`/`). Every figure on it comes from a record that exists today; anything without one is
left out or says so in words.

**The tabs.** A desktop (1100 px and wider) shows Home, Sessions, Machines, Skills and Activity in the top bar, with
**Jump to a session, project or machine**, the bell and the account menu. The account menu holds **More**:
Setups, Sync, Lists, Community, Support, Teams & organisations and Host skills. An open instance has no
account menu, so it keeps More in the top bar. A tablet (651–1099 px) keeps
Home, Sessions and Machines in the top bar, with **More** beside them for the rest and a search button for Jump. A phone
has a slim top bar (logo, search, bell, account) and a 64 px bottom tab bar: Home, Sessions, Machines and More. While
the phone keyboard is up for Jump, the bottom bar steps aside, and Jump's results end above the keyboard. Sessions and
the bell are for a writer in accounts mode only.

**What Home shows today.**

| Part | What it shows, and from where | Arrives with |
| --- | --- | --- |
| Fleet strip | One card per machine from the health report: the status in words (online, paused, reconnecting or offline) with a dot that also differs by shape, the live sessions on it of any kind ("sessions running"), CPU, memory in use and disk space free from a fresh report (or **No reading** for each, with a meter), the worktrees live sessions there are in ("3 worktrees in use"; no record says which other worktrees were removed), and the skills line from the last finished sync check for the current desired skills, or **Not checked**. A machine that is not reporting shows its last known count and says so. **Add a machine** opens the existing dialog on Machines. | OS version: PR C |
| In flight | Live sessions, and finished ones for a day, grouped by project, Machine or State. A session joins the folder project it was started in (its own project id); the subtitle is `project · machine · branch`. Each row shows the state word and where it came from ("from the session's state" or "from the exit code"). There is no step or progress bar. | Attention states: PR G |
| Needs you | "Nothing needs you." with a count of 0. Nothing signals today that a session needs the person. | Cards: PR G. Decision cards: a later programme |
| Project rail | Your folder projects ([Projects](projects.md)): each card's colour, machine, base branch, state in words and worktrees in use; choosing one (or New session here in its menu) sets the launch bar's project; a menu, from the card's button or a right-click, with New session here, Edit, project colour, Worktrees, Move up and down, and Remove from reet. **Select projects** in the heading's menu (or a long press on touch) turns the cards into checkboxes for a start in 2 to 12 projects. **Add** opens the folder browser. Under 1240 px it is a 56 px strip that shows how many are chosen; on a tablet or phone it sits under In flight. | The phone rail on Sessions: PR H |
| Launch bar | Docked at the foot: **Describe the job**, **Start** and the launch sentence ("Run Claude Code with default on Store rewrite at studio from main in a fresh worktree"), whose words are the choices. Home has no Start an agent button; the dialog stays for Machines and the Sessions panel. Details: [Start an agent session](start-agent-session.md#on-home-the-launch-bar). | YOLO: a later release |
| Bell | The Needs you count, which is 0 today and shows no badge. | PR G |

Viewers, single-token and open sign-ins get the fleet strip only. Those sign-ins cannot read sessions, so the cards
show no session count rather than a false 0. Home refreshes every 15 seconds while the tab is visible. If a refresh
fails, it keeps what it last read and says so. The only value it stores in the browser is the Group by choice, under
`reet.home.groupBy.<account>`.

On a tablet and a phone the page is one column: machines, then Needs you, then In flight. Every target there is at
least 44 px. The phone's launch bar has a one-line prompt, a round 48 px Start and one summary line with
**Change**; the tablet's has the shorter sentence with its own Change. Checks:
[Agents home shell and Home](ui-verification.md#agents-home-shell-and-home--26-october-2026).

## The Sessions page

Control is folded into Home and Machines (R4): `/control` goes to `/`. The **Sessions** tab is `/sessions` (R17): every
session across machines, grouped by Project, Machine or State (Home's choice), with search (title, agent, model, branch,
project and machine), filters (state, agent, machine, colour, YOLO only, worktree only, and a sort that includes the sessions
panel's custom order; each row shows its colour, the panel's choice or its project's, as an edge strip and its name), and each row's actions menu: right-click, Shift+F10 (or the menu key) or the ⋮ button. It holds
Rename, Open in theatre, Copy branch (worktree sessions), End session or Remove from history (each confirmed), and Check
and remove worktree (a finished session's own worktree; offered only when its path and branch name the run). A machine
that is not reporting says "studio is offline" on its group and its sessions stay listed with what needs it disabled.
A refresh that fails keeps the list with "Couldn't refresh. Showing what we last saw." and **Try again**. Below the
groups, *Projects · N* lists the folder projects (each opens on Home) with **Add a project**. Skill sync and bulk
inventory actions remain on **More → Sync**. A row links to `/sessions?agent=…&session=…`, which opens that exact
connector and session ID in the workspace below; `/terminals` redirects to `/sessions` keeping its query. Opening a
session never creates a shell. Running means a shell lifecycle state, not AI-agent activity. Creating and
end-requested sessions retain their actual labels.

The account-scoped band polls every 15 seconds while visible, times out requests
after 10 seconds, and labels observations older than 45 seconds as last known.
Transient errors preserve the last list; 401/403 clears it. Viewers do not mount the
band or request terminal data. Viewing the band does not attach to a shell.

The terminal route uses compact app navigation and the remaining usable viewport
(see [Phones](#phones) for how that height is chosen while a keyboard is open).
### The sessions panel

The panel opens with **Projects** above the session list: the same folder projects as Home's rail,
one row each with its colour, name and live count. A row's **+** starts an agent in that project's
folder and opens the new session; the row itself continues the project's live session (or its most
recent one). Right-click, Shift+F10 or the row's menu opens New session here, Show on Home and the
project's colour. Colours are saved on the project, so they follow the person to every browser;
a session's own colour, chosen from its row's menu, still wins over its project's. A session row's
hover or keyboard focus shows a card with the machine, project, branch, state, runtime and the
machine's last CPU, memory and disk readings, named last known when the report is stale.

The sessions panel scrolls independently and has three modes: **full**, a 64 px
**mini rail**, and **closed**. **Sessions** in the workspace header opens or closes
the full panel. The panel's chevron cycles full → mini → closed (on a phone it only
closes). The rules live in `terminal/panelLayout.ts`, unit tested by
`terminal/panelLayout.test.mjs`.

- **Docked by default from 768 px, never over the terminal.** On a tablet or
  desktop window 768 px or wider (on a device that is not a phone) the full panel is
  a grid column beside the terminal; from 600 to 767 px it docks once Keep open is
  on. The terminal's column is `minmax(0, 1fr)` of what
  remains, so the whole terminal stays visible with no horizontal scroll, and it
  refits to its new width. A first visit under 960 px starts the panel at 240 px
  (280 px from 960 px), so a tablet's terminal keeps the most columns.
- **Keep open.** Between 600 and 959 px the panel header has **Keep open**. Until
  the person chooses, it is on from 768 px and off from 600 to 767 px, so a window in
  that narrower band keeps the drawer by default. Off, the panel is the drawer: it
  opens over the terminal (Keep open is in the drawer to dock it) and closes when a
  session is chosen. The person's choice is saved and then applies at any width. From 960 px the panel always docks, so the control is not shown.
  Expanding the mini rail (its Expand or Find, the panel's Sessions button, the
  separator's Right arrow or double-click, or dragging the rail out) turns Keep open
  back on, so the panel opens where the rail was instead of as a drawer, and a drag
  keeps its separator under the pointer.
- **Resizing.** Between the docked panel and the terminal is a separator
  (`role="separator"`, vertical, with `aria-valuenow`, `aria-valuemin` 64 and
  `aria-valuemax`). Its hit area is 24 px wide and it captures the pointer, so a drag
  that leaves the handle keeps going; it works with a mouse, pen or touch. The panel
  is 240 to 480 px and never more than 40 % of the workspace, so the terminal keeps at
  least 60 %. A drag below 160 px snaps to the mini rail, and dragging the rail out
  past 160 px opens the docked panel again. Keyboard: Left/Right move 16 px (Left at
  240 px becomes the rail, Right from the rail opens the panel at 240 px), Home is the
  rail, End the widest. Double-click resets it to 280 px.
- **One size to the host per drag.** The grid column follows the drag and the
  terminal refits locally (at most one xterm fit per animation frame), but the size
  sent to the host is held for the whole drag, however long its pauses, and sent once
  150 ms after the drag ends. The layout is saved when the drag ends.
- **Focus.** When the panel changes mode, focus moves to the control that undoes it:
  shrinking to the rail focuses the rail's Expand, expanding focuses the panel's
  chevron, Find focuses the panel's search, and closing focuses **Sessions** in the
  workspace header.
- **Mini rail.** One 44 px button per session, in the panel's order: a state dot
  (running a filled circle, starting a ring, ending a half circle, lost a diamond,
  finished a hollow square) and the title's initial. Its accessible name is
  "Title on machine, state". Hover or keyboard focus shows the session card (machine,
  project, branch, state and the machine's last readings). The rail also has **Expand
  sessions**, **New session**, **Find sessions** and a chevron that closes it. The
  rail lists every session, whatever machine filter or search the full panel has.
- **Saved layout.** Mode, width and Keep open are saved in this browser under the
  existing per-account key (`reet-terminal-layout:<user>`), as
  `{open, mode, width, pinned}`; `open` keeps older pages reading it. A layout saved
  before the rail (`{open, width}`, width 220 to 480) still loads: `open` becomes full
  or closed, the width is brought to 240 to 480, and Keep open stays the screen's
  default (on from 768 px). `pinned` is only saved once the person sets Keep open.
  Unreadable values fall back to the defaults. Only a person's change on a tablet or
  desktop is written: nothing is written on load, the untouched defaults are never
  written, and opening or closing the drawer on a phone writes nothing, so a phone
  visit leaves the wider screen's layout as it was.
- **Phones keep the drawer.** A window narrower than 600 px, or a device whose
  screen's shorter side is under 600 px (a phone, upright or held sideways), gets
  the overlay drawer, closed on arrival, whatever was saved: no rail, no separator,
  no Keep open. A small tablet held sideways with tall browser chrome (an 834 × 480
  page) is still a tablet and docks. A docked panel that would become a drawer
  because the window narrowed closes instead of covering the terminal. The drawer
  slides in briefly unless the system asks for reduced motion. The phone keyboard
  focus layout is unchanged.

**Your sessions → Machines** defaults to all machines, with individual selection
and multi-machine checkboxes. Filtering or organising never creates or stops a
shell. Select a row to continue that exact session across machines. Sort by custom
order, name, recent metadata activity, creation or state; group by machine, state,
colour or none. Drag the row's action handle within its group in Custom order;
keyboard users can choose Move up/down from the same menu. Colours mark sessions
separately from running-state dots. Organisation choices stay in this browser.

Right-click a row, press Shift+F10, or use its action button to rename, disconnect
the selected browser attachment, stop the shell or remove confirmed closed history.
Actions capture the clicked session without selecting or attaching to it. Cross-machine
responses cannot retarget the active shell. Stop and history removal require
confirmation; pending or running shells cannot be removed directly. The screen fits when the viewport/sidebar/font changes. **Details &
help** contains health, setup, quotas and command reference. Opening it does not
replace the attachment. Legacy `/machines?view=terminal` and account route links
keep the same workspace behaviour. **Find** searches host and shell-user names,
including enrolled hosts that need connection/setup. Each machine remembers its
last selected session while this workspace is mounted. The selected user/host
remains visible on phones; moving from desktop to phone closes the panel.

Each unconfirmed creation keeps its immutable request ID and original title per
machine while the workspace is mounted. Switching machines cannot discard or
retarget that retry. **Review creation** or **New session** opens the original retry;
its title is locked until the controller confirms the result. These pending
operations are in memory, not terminal output saved in browser storage. After
reloading, use the reported session inventory to continue existing work; the app
never automatically creates a replacement shell.

Status fetches have a 10-second deadline; creation requests stop waiting after 15
seconds and retain the retry operation. A successful mutation invalidates older
in-flight observations and schedules a fresh list, so stale polling cannot erase a
confirmed new session or undo rename/end presentation. Selection stays independent
of mutation responses. Metadata access denial clears the private inventory and
closes the browser attachment, with an access explanation; it does not end shells.

## Appearance, commands and close defaults

**Appearance** offers Midnight, Graphite, Ember and Paper palettes, follow-app
mode, font size and cursor shape. These browser preferences leave the shell and
attachment identity intact. The compact toolbar keeps clipboard, interrupt,
search, history and maximise controls within reach.

**Commands** opens a private account library, also reachable from **Saved commands** in the Sessions page header (no
session needed; Insert is offered only from a pane). Search,
create, edit, remove, copy or explicitly insert command text. The controller stores
up to 200 commands, with 1–80-character names, 16 KiB of exact UTF-8 command text
and optional notes. Optimistic revisions reject stale edits/deletes. Another
browser sees the same library. It is metadata, usable without an available host;
viewers and API tokens cannot read or write it. Saving never dispatches input.
Insertion binds to the chosen live attachment generation and adds no Enter.
Multiline/tab/control/large input retains explicit paste review. No shell output
or credential interpolation is used to build commands.

Stopping a session includes **Remove from history when this session ends** and
**Use this as my account default**. Account → Profile also stores the default.
History stays visible during offline/pending termination. The immutable end
receipt stores the explicit choice; authenticated host closure inserts the
existing archive tombstone in the same transaction. Changing preferences later
never changes an already requested stop. Files and operation receipts remain.

## Actions and keyboard

| Control | Effect |
| --- | --- |
| Disconnect | Close this browser attachment; leave the shell running |
| Reconnect | Attach to that existing shell; do not create another |
| New session | Explicitly create a shell; preserve retry IDs on uncertain responses |
| Manage → Rename | Change the title; preserve process identity |
| Manage → End session | Confirm and request termination of the exact shell and its work |
| History | Search saved Bash/Zsh commands; paste to edit or explicitly review and run |
| History → Terminal output history | Read a bounded output snapshot |
| Manage → Remove from history | Hide an ended/exited/lost record; keep machine files and operation identity |

Intentional disconnect stays disconnected through online/visibility events.
Switching machines or sessions preserves the old shell. End requests on unavailable
machines remain pending until confirmed. Remove from history uses a persistent controller tombstone and rejects active or
pending sessions. Confirmation captures the session ID/title. Failed removals stay
visible; success cannot detach a newer selection. It remains available when terminal
execution is disabled. See [the archive contract](terminal-history-archive.md).

- Ctrl+C interrupts when there is no selected output. On Windows/Linux, selected
  output makes Ctrl+C copy. On Mac, Cmd+C copies and Ctrl+C interrupts. The visible
  **Interrupt** action always sends Ctrl+C, including when output is selected.
- Ctrl+V/Cmd+V and Ctrl+Shift+V use native paste events. The key handler must not
  also send clipboard text. Clipboard denial leaves keyboard paste available.
- Ctrl+X remains terminal input for a remote shell/editor. Native cut still works
  in editable app fields. Terminal output cannot be cut.
- Right-click opens the terminal Copy/Paste menu. Shift+right-click preserves the
  browser menu. Arrow keys, Home/End and Escape work in the local clipboard menu.
- Multiple lines, control characters and large pastes require confirmation. A
  reconnect invalidates pending paste approval; disconnected input is not replayed.
- OSC 52 clipboard writes and OSC 8 active hyperlinks remain disabled. Terminal
  text is not persisted to browser storage. Font size is a local preference.

These interactions use the existing private terminal transport; they do not add a
new direct SSH adapter. Phone viewport emulation is not a physical keyboard test.

## Implementation map and verification

- `App.svelte` scopes compact chrome to authorized terminal routes;
  `Machines.svelte` suppresses secondary inventory navigation there.
- `sessions/SessionsPage.svelte` is the Sessions list for a writer in accounts mode; `sessions.ts` holds its search,
  filters and groups (Home's `groupSessions`). It owns polling and stale/error presentation.
- `terminal/TerminalWorkspace.svelte` owns selection, sidebar, visual-viewport
  sizing, dialogs and existing create/rename/end APIs. Confirmations capture IDs
  and display names at open time. `WorkspaceDialog.svelte` provides native modals.
- `terminal/panelLayout.ts` holds the sessions panel's rules (modes, docking,
  width clamp, separator keys, the saved layout); `terminal/panelLayout.test.mjs`
  covers them.
- `terminal/sizing.ts` holds the browser-free sizing rules: the phone font default,
  the 20×8 floor, the debounced resize sender with its repaint nudge, the usable
  viewport height and keyboard detection. `web/tests/terminal-sizing.test.mjs`
  covers them.
- `terminal/TerminalPane.svelte` owns xterm fitting, attachment controls, clipboard,
  focus and extra keys. `screen-frame` owns visual padding; the immediate xterm
  fitting parent `screen` has no padding. FitAddon reads that parent's dimensions
  without subtracting parent padding. Keep this split to prevent an oversized
  grid clipping its final row. `keyboard.ts` classifies terminal-only shortcuts.
  `connection.ts` retains lease, recovery, generation and input-replay boundaries.
  The pane requests a full xterm refresh after each parsed batch and fit, keeping
  accessibility rows current through full redraws and resize. xterm coalesces these
  requests in its rendering frame. Terminal modes and screen-reader announcements
  remain enabled; no DOM-only copy of remote output is maintained.

Run `npm --prefix web run check` and `npm --prefix web run build`. Finish the build
before Go checks because Go embeds its output. For production-bundle UI fixtures,
start `npm --prefix web run preview -- --host 127.0.0.1 --port 4173`, then run
`node web/tests/control.e2e.mjs`, `node web/tests/sessions.e2e.mjs`, `node web/tests/terminal-workspace.e2e.mjs` and
`node web/tests/terminal-lifecycle.e2e.mjs`,
`node web/tests/terminal-organisation.e2e.mjs` and
`node web/tests/terminal-phone.e2e.mjs`.
These fixtures cover synthetic two-machine continuation, permissions/errors,
viewport bounds, clipboard, intentional disconnect, delayed observations, per-host
retries, pending end, quota and explicit takeover. They do not prove remote
shell identity. Pair them with the real browser/PTY fixture in
`.github/workflows/terminal-browser.yml`, which checks shell PID continuity,
controller/connector restarts, actual resizing, paste fences and rollout disable.
Its two local enrolled hosts have independent tmux servers and registries; the
browser proves creation-response retry, context switching, explicit lease takeover,
rename and offline end without replacing either original shell.

No production relay, physical phone, macOS or Windows runtime is established by
these local Linux/Chromium fixtures. Initial-install/enrollment/upgrade terminal
setup is the next priority recorded in `handoff.md`.

## Account command metadata API

| Method/path | Body/result |
| --- | --- |
| GET `/api/v2/terminal/commands` | `{commands:[...]}` for the signed-in account |
| POST `/api/v2/terminal/commands` | `{name,command,description,revision:0}` → saved record with generated ID/revision 1 |
| PATCH `/api/v2/terminal/commands/{id}` | Same fields plus current revision → incremented revision |
| DELETE `/api/v2/terminal/commands/{id}` | `{revision}` → `{ok:true}` |

Records also expose `createdAt` and `updatedAt`. Unknown body fields and invalid
text return 400; stale revisions and the 200-item limit return 409. Missing and
foreign records share the existing 403 policy. These metadata routes keep account,
write-role, cookie-only and canonical write-Origin checks while allowing command
management when terminal execution is disabled. Commands are never interpolated
or sent to the broker by these handlers. The profile default is UI-only: an end
API request must explicitly set `removeHistory:true` to request archival.

The fitting regression checks the actual renderer rectangle against its clipping
area, including the final row, rather than only asserting that text exists in the
accessibility tree. The real fixture also exercises `top` while resizing and a
full-screen last-row marker before any input is sent to release it. ANSI colour
checks read rendered foreground colours after real host SGR output and palette
changes. Plain application output remains plain; `top` uses `z` for its colour mode.

## Searchable shell command history

History opens a drawer for the selected machine's saved Bash/Zsh commands. Search
and source filters stay in memory. **Paste to edit** inserts without an extra
Enter, retaining multiline/control paste review. **Run…** always shows the exact
command and target before adding Enter. Be at a shell prompt: a foreground editor
or full-screen program receives terminal input instead. Cancelling Run returns
to the same search results. A reconnect invalidates pending confirmation; commands
are never automatically replayed. Terminal output history is a separate action.

Saved history comes from the enrolling OS user's default `.bash_history` and
`.zsh_history`. Custom HISTFILE/ZDOTDIR locations and other shell formats are not
read. Open shells may not have saved their latest commands; Ctrl+R at a Bash/Zsh
prompt also searches current-session history. The account command library remains
separate and manually curated.

See [the command history contract](terminal-command-history.md) for the private
extension, bounds, compatibility and verification. Existing agents keep their
terminal connection and display an update prompt until upgraded and restarted.


### Scrollback and viewing layouts

**Scrollback** (or Shift+PageUp while the terminal has focus) opens earlier output
inside the terminal panel. It reads the machine's most recent 2,000 lines / 256 KiB
using the existing bounded output-history API. Scroll vertically/horizontally,
select text, use Page Up/Down, or search and move between literal matches. It is a
read-only snapshot; Refresh obtains newer output, and Back to live returns input
focus to the same running shell. Nothing is saved in browser storage. The live
xterm stays mounted and receives redraws behind the reader; no keystrokes from
reading/search are forwarded. Scrolling an interactive app such as `top` remains
under that app's control. Tmux's alternate-screen rendering means a browser-only
xterm scrollbar cannot expose its machine-side history.

**Theatre view** hides the session panel and workspace toolbar while retaining the
site navigation. Exit theatre restores the previous panel layout. **Maximise**
remains the separate viewport-filling view (Alt+Shift+M). Both fit the existing
terminal/PTY; neither disconnects, creates a shell or changes the saved panel layout
(mode, width, Keep open).


### Quick terminal actions

The + beside a machine starts a new Shell explicitly. Empty machines remain in
the machine groups; unready/disabled or full machines disable creation. An
unconfirmed workspace creation reuses the same request when retried. The control
page has the same + on machine and active-session cards and opens the returned
session. Menus themselves never start or attach a shell.

Right-click a machine or use its ellipsis for terminal actions. Right-click the
terminal screen for saved commands, command history, earlier output, appearance
and clipboard controls; Shift+right-click keeps the browser menu. The Sessions
page's menus also rename or request a stop, with explicit confirmation and the
same remove-after-closure/account preference as the workspace.

When the panel is closed, **Show sessions** is visible in both the workspace
header and the terminal toolbar (the mini rail has its own Expand instead). In theatre
view the toolbar control exits theatre and restores the panel as it was. Its saved
width and organisation remain intact.

## Phones

These rules apply to phones: a window 650 px wide or less, or a touch screen held
sideways that is 500 px tall or less (`PHONE_SCREEN` in `terminal/sizing.ts`). The
sideways rules cover the terminal only; dialogs and other pages are unchanged. The
sessions panel decides "phone" its own way (a window under 600 px or a screen whose
shorter side is under 600 px, see [The sessions panel](#the-sessions-panel)), so a
window between 600 and 650 px gets the compact phone chrome with a panel that docks
once Keep open is on (the drawer by default),
and a short tablet held sideways gets the compact terminal chrome with a docked panel.
Desktop and tablet terminal chrome is unchanged, and
`web/tests/terminal-phone.e2e.mjs` checks it at 1360 and 768 px.

**Size sent to the host.** The browser resizes the real session: the host sets the
attachment's PTY size and tmux resizes the window, so the app gets SIGWINCH.
- The size sent is never below 20 columns by 8 rows. If the screen has room for fewer
  rows, xterm keeps the 8-row grid and shows its bottom rows, where prompts and
  agent input live.
- Sizes are sent 150 ms after the last layout change (a keyboard animating in sends
  one size, not one per frame), and a size the host already has is never sent. The
  attachment ticket counts as sent. A size the connection could not send is not
  treated as sent; the next layout change or the next ticket carries it.
- A repaint nudge sends the height one row shorter (one row taller at the 8-row
  floor), then 300 ms later the real size. tmux passes a real size change to the app,
  which repaints in full. It uses the existing resize message; no tmux option or
  protocol message changed, and other tmux clients are unaffected beyond tmux's
  normal latest-client sizing.
- The nudge runs on every screen, not only phones: after each reattach, and when the
  page becomes visible again if the screen is a touch screen (phones freeze background
  tabs) or the page was hidden for 30 s or more. Never twice within 5 s; a reattach
  counts. The first attach is not nudged because tmux repaints its own grid then.

**Font.** With no stored size, phones with a coarse pointer start at 11 px, about 54
columns at 390 px instead of 41. A stored size (10–22) always wins.

**Chrome.** The pane header is one line, with the user/host still shown and truncated.
The pane header, terminal toolbar and extra-key row lose their vertical padding. On a
touch screen every toolbar button and extra key is at least 40 × 40 px, and the header
controls are 40 px tall. The screen frame padding is 4 px. The toolbar and key row
scroll sideways; the end that hides more controls fades. The key row is focusable, so
arrow keys scroll it; that tab stop also appears in mouse windows narrower than 650 px,
where the key row shows too. The Alt+Shift+M advice is hidden on touch screens (no
hover, coarse pointer); Restore size stays.

**Sideways phones.** The toolbar and the extra keys share one row, each scrolling with
its own fade, and the pane footer is hidden (the state stays in the header). At 844 ×
390 the screen shows 13 rows with the keyboard closed.

**Keyboard focus layout.** While the on-screen keyboard is open and the focus is in the
terminal, the site header, the workspace toolbar and the pane footer are hidden; held
sideways, the pane header is hidden too (8 rows at 844 × 390 instead of none).
**Show sessions** stays in the terminal toolbar. Everything returns when the keyboard
closes or the focus leaves the terminal. Detection needs a coarse pointer, a phone
screen, an unzoomed page, focus in the terminal and a height at least 25% below the
tallest seen at that width. Theatre view still keeps the site header.
- The tallest height is kept per width for the page's life. A same-width split screen
  that halves the window while the focus is in the terminal therefore also gets this
  layout, which suits it. A page first loaded with the keyboard already open, in a
  browser that overlays the keyboard, gets the layout only after the keyboard has
  closed once.

**Usable height.** While the terminal workspace is mounted it adds
`interactive-widget=resizes-content` to the viewport meta and restores the previous
value when it unmounts; other pages keep the default, so their fixed bars and `100dvh`
dialogs are unchanged. Android browsers that honour it shrink the page itself for the
keyboard. Whether they honour a meta changed after load must be checked on the
device; the height rule below covers both answers.
- When the page is at least 15% shorter than the tallest seen at that width, its
  height (`innerHeight`) is used even if `visualViewport.height` is lower. One Android
  browser reports a visual viewport that still subtracts a bottom toolbar it hides
  while typing, which left a blank band about 90 px tall above the keyboard.
- Otherwise (iOS, which ignores the meta, and browsers that overlay the keyboard) the
  visual viewport height and its `offsetTop` are used. A maximised pane follows the
  same height and offset.

**Checking a phone.** Open the terminal with `?viewport-debug=1` added to its address.
A small readout in the top corner shows, live, `visualViewport.height` and `offsetTop`,
`innerHeight`, `screen.height`, the height used and whether the keyboard layout is on.
It never takes focus or touches, so the keyboard stays open while you read it; nothing
is stored or sent. See `handoff.md` for what to record.
