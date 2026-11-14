# Terminal workspace

## Home and the tabs

Signing in lands on **Home** (`/`). Every figure on it comes from a record that exists today; anything without one is
left out or says so in words.

**The tabs.** A desktop (1100 px and wider) shows Home, Sessions, Machines and Skills in the top bar, with
**Jump to a session, project, skill or machine**, the bell and the account menu. Activity is a section of the account page at
`/account/activity`; the old `/activity` address opens it. The account menu holds **More**:
Setups, Sync, Lists, Community, Support, Teams & organisations and Host skills. An open instance has no
account menu, so it keeps More in the top bar. A tablet (651–1099 px) keeps
Home, Sessions and Machines in the top bar, with **More** beside them for the rest and a search button for Jump. A phone
has a slim top bar (logo, search, bell, account) and a 64 px bottom tab bar: Home, Sessions, Machines and More. While
the phone keyboard is up for Jump, the bottom bar steps aside, and Jump's results end above the keyboard. Sessions and
the bell are for a writer in accounts mode only. Jump lists sessions, folder projects, shelf skills and machines; a
skill row carries its saved sync intent, and choosing it brings that card into view on the shelf.

**What Home shows today.**

| Part | What it shows, and from where | Arrives with |
| --- | --- | --- |
| Fleet strip | One card per machine from the health report: the status in words (online, paused, reconnecting or offline) with a dot that also differs by shape, the live sessions on it of any kind ("sessions running"), CPU, memory in use and disk space free from a fresh report (or **No reading** for each, with a meter), the worktrees live sessions there are in ("3 worktrees in use"; no record says which other worktrees were removed), and the skills line from the last finished sync check for the current desired skills, or **Not checked**. A machine that is not reporting shows its last known count and says so. **Add a machine** opens the existing dialog on Machines. | OS version: PR C |
| In flight | Live sessions, and finished ones for a day, grouped by project, Machine or State. A session joins the folder project it was started in (its own project id); the subtitle is `project · machine · branch`. Each row shows the state word and where it came from ("from terminal activity", "from the session's state" or "from the exit code"). There is no step or progress bar. A row opens the session; a right-click (or Shift+F10 / the menu key) opens the same actions menu as the Sessions page: Rename, Open in theatre, Copy branch, End session or Remove from history (each confirmed), and Check and remove worktree. | |
| Needs you | Sessions that are waiting for you (a terminal bell, a question an agent has put on its terminal screen, or a pending Gaffer permission since you last opened or marked the session seen), finished cleanly ("Ready to review", with changed-file count when the session ran in a worktree) or exited with an error, newest first, with **Open** and **Mark seen** (`POST /api/v2/terminal/sessions/{id}/seen`). Empty state: "Nothing needs you." | Inline Allow once / Deny cards on Home: a later slice |
| Project rail | Your folder projects ([Projects](projects.md)), Home's own resizable left column on a desktop (301 px by default, 240–460 px, or a 56 px strip when folded; from 1100 to 1239 px it is always the strip). The primary tabs live in the top bar, not in this panel. Each compact card has two lines: its repository diamond or folder icon and name, then its machine, base branch, state in words and worktrees in use; choosing one sets the launch bar's project, while its **+** (and New session here in its menu) starts a session there at once; a menu, from the card's button or a right-click, with New session here, Edit, project colour, Worktrees, Move up and down, and Remove from reet. **Select projects** in the heading's menu (or a long press on touch) turns the cards into checkboxes for a start in 2 to 12 projects. **Add** opens the folder browser. On a tablet or phone it is on the **Sessions** screen instead (see [The Sessions page](#the-sessions-page)); Home there keeps the launch sentence's project slot and its Add a project. | |
| Launch bar | Opens in a modal from Home's Select and from an address that hands over projects: **Describe the job**, **Start** and the launch sentence ("Run Claude Code with default on Store rewrite at studio from main in a fresh worktree"), whose words are the choices. OpenCode, Cline or AGY on a machine offering its Gaffer integration reads "Run \<agent\> in Gaffer…" by default; the **Gaffer** word switches it to **\<agent\> TUI**. A project's **+** opens the Start an agent dialog, as it does on Machines and the Sessions panel. Details: [Start an agent session](start-agent-session.md#on-home-the-launch-bar). | **YOLO** needs the account setting, the machine's acceptance and a per-session confirmation. A live structured chat can switch Ask me ↔ YOLO from its header under the same gates. |
| Bell | The Needs you count across your machines, with a popover listing the same cards as Needs you. The page title prefixes `(N)` when `N > 0`, and a polite live region announces transitions into Waiting at most once per session per minute. | |

Viewers, single-token and open sign-ins get the fleet strip only. Those sign-ins cannot read sessions, so the cards
show no session count rather than a false 0. Home refreshes every 15 seconds while the tab is visible. If a refresh
fails, it keeps what it last read and says so. The only value it stores in the browser is the Group by choice, under
`reet.home.groupBy.<account>`.

On a tablet and a phone the page is one column: machines, then Needs you, then In flight; the project rail is on the
Sessions screen. Every target there is at least 44 px. In flight's rows open the session screen and say the state with
the words its header pill uses. The phone's launch bar has a one-line prompt, a round 48 px Start and one summary line with
**Change**; the tablet's has the shorter sentence with its own Change. Checks:
[Agents home shell and Home](https://github.com/eyupio/reet.sh/blob/main/docs/ui-verification.md#agents-home-shell-and-home--26-october-2026).

## The Sessions page

Control is folded into Home and Machines (R4): `/control` goes to `/`. The **Sessions** tab is `/sessions` (R17): every
session across machines, grouped by Project, Machine or State (Home's choice), with search (title, agent, model, branch,
project and machine), filters (state, agent, machine, colour, YOLO only, worktree only, and a sort that includes the sessions
panel's custom order; each row shows its colour, the panel's choice or its project's, as an edge strip and its name), and each row's actions menu: right-click, Shift+F10 (or the menu key) or the ⋮ button. It holds
Rename, Open in theatre, Copy branch (worktree sessions), End session or Remove from history (each confirmed), and Check
and remove worktree (a finished session's own worktree; offered only when its path and branch name the run). A machine
that is not reporting says "studio is offline" on its group and its sessions stay listed with what needs it disabled.
Each read shows "Checking…" over the last known list, on a line kept for it so the rows never move under a tap. A
refresh that fails keeps the list with "Couldn't refresh. Showing what we last saw." and **Try again**. **New** opens Home
with the launch prompt focused (`/#launch`); `/sessions/` is the same address as `/sessions`. Below the
groups, *Projects · N* lists the folder projects with **Add a project**: on a desktop as simple cards (each opens on
Home; a right-click or Shift+F10 offers Open on Home, New session here and Worktrees…), and on a tablet or phone as the
full project rail moved here from Home (each card's menu, its state in words and
**Select projects** or a long press). There a card, or New session here, opens Home's launch bar on that project
(`/?project=<id>`), and with two or more chosen in Select, **Start in these on Home** hands them to it in the address as
ids only (`/?projects=<id>,<id>`); nothing is stored. Skill sync and bulk
inventory actions remain on **More → Sync**. A row links to `/sessions?agent=…&session=…`, which opens that exact
connector and session ID in the workspace below; `/terminals` redirects to `/sessions` keeping its query. Opening a
session never creates a shell. Running means a shell lifecycle state, not AI-agent activity. Creating and
end-requested sessions retain their actual labels.

The Sessions page polls every 15 seconds while visible, times out requests
after 10 seconds, and labels observations older than 45 seconds as last known.
Transient errors preserve the last list; 401/403 clears it. Viewers get the Machines
page there and request no terminal data. Viewing the list does not attach to a shell.

## The session screen

`/sessions?agent=…&session=…` opens one session on the session screen, inside the workspace (the sessions panel stays
beside it). Its parts:

- **Tabs** in the workspace's toolbar row: the sessions opened in this browser tab since it loaded (R20), at most eight in
  the order opened (the ninth drops the oldest), each a link with its state dot and a close button. Choosing a tab switches
  in place; middle-click or the close button closes the tab only, never the session (closing the current tab shows its
  neighbour; closing the last goes back to the list). **+** goes to Home with the launch prompt focused (`/#launch`). Tabs
  are held in memory: nothing is stored, and the address carries only the session's ids.
- **Header**: the title, `agent · model · started …`, the state pill (Home's state words, with where the word came from in
  its tooltip and read out; while this browser's connection to a live session is down it says that instead, such as
  "Reconnecting" or "Disconnected", from the connection), a **YOLO** badge when on (or **Ask me | YOLO** for a live structured chat that can switch), **Gaffer | the agent TUI** — the agent TUI chosen for a shell or terminal
  session, Gaffer chosen for a structured chat (the switch to the agent's own TUI is OpenCode's, so Cline and AGY chats show it disabled), and
  Gaffer disabled with its reason for a shell. **Theatre view** is a direct header button that hides the panels without changing the session. Interface, approvals, **Terminal here** and (on larger screens) **Terminal beside** stay in a wrapping toolbar. The actions menu (More / ⋮, or Shift+F10 on it) contains Rename, Copy branch,
  End session or Remove from history, Check and remove worktree. Tab from the open menu goes on to the control after ⋮
  (Shift+Tab back to ⋮), and a reconnecting terminal never takes focus out of it. A terminal session shows no steps and no
  progress bar (R10). A tab's close button says the session keeps running, or for a finished one that it stays in its
  history.
- **The terminal pane**, unchanged. Its footer starts with **Approvals: Ask me** (or **Approvals: YOLO**). When the
  device asks for reduced motion the terminal's cursor does not blink.
- **A session's actions menu** has **Open a terminal here…** for any agent session. It starts a plain shell in the same
  folder as a separate session; a machine that negotiated `shell-dir-v1` runs it there, otherwise the controller says to
  update reet. This is the direct non-agent terminal entry point. **Open a terminal beside this session…** creates the
  same plain shell but shows it in the right-hand panel while Gaffer or agent TUI stays in the main column; Close
  ends that shell. The side panel replaces the context panel while it is open.
- **Gaffer pane** for a structured OpenCode, Cline or AGY session ([structured sessions](https://github.com/eyupio/reet.sh/blob/main/docs/structured-sessions.md)): the streamed
  conversation, tool calls, approval cards with **Allow once** and **Deny**, a composer (Enter sends, Shift+Enter adds a
  line), **Attach image** (pasted, dropped or chosen; the machine embeds it in the prompt when it negotiated
  `acp-image-v1`, otherwise the prompt names the uploaded path), **Stop**, and the watchdog's "still working" notice. A lost or crashed
  chat reconnects in place (OpenCode, and Cline on a connection that negotiated `acp-agent-reattach-v1`); a session a person ended offers **Continue in Gaffer with history…** in its menu,
  which opens a new Gaffer and loads the agent's stored conversation. Approval requests appear here; AGY uses its configured permission rules. There is no shell in this
  session. It has no terminal or session tabs on a phone; the usual slim header and End actions stay.
- **Super focus on a phone**: a Gaffer chat opens with the site chrome, workspace toolbar and secondary session controls
  out of the way, so the conversation and composer take the whole screen. The slim header keeps Back, the title, the
  state and **Exit focus**; the chat status and compact progress strip stay visible, and the composer keeps its message field,
  **Stop**/**Queue**, **Skills** and attach while effort and model wait behind **Exit focus**. The choice is remembered for
  this browser; **Focus** in the header or the actions menu turns it back on. The
  floating **Ask Gaffer** corner stays clear of the composer and steps aside while
  the worker has the phone keyboard open.
- **Context panel** (340 px): *Where it's working* (machine and whether it is reporting, project, branch, worktree: "own
  copy" or "the checkout"); *Changed files* for a session in its own worktree (the first 20 paths from the worktree status
  and the total, read when the panel opens on the session and again only from **Refresh**, R19); *Skills on this machine*
  (the fleet card's skills line; **Manage** opens that machine's skills against your list); *Decisions in this session*
  ("Asked in the terminal.", or the YOLO line with a link to Activity). With less than 1100 px of room for the screen (a
  narrower window or the sessions panel docked beside it) and in theatre view, the panel waits behind **Details** and
  opens over the terminal's edge, below the header. Focus moves into it (Close details first), Tab stays inside it, and
  Escape or Close details gives focus back to Details. Docked, the keyboard reaches it after the header and before the
  terminal.
- **Tablets** (651–1099 px): the context panel waits behind **Details** and opens as a 340 px side sheet along the
  terminal's right edge, from under the header to the foot of the screen; every target in the header, the toolbar, the
  tabs, the sheet, the docked sessions panel (its rows, menus and swatches; the separator has a 44 × 88 px grip that
  reaches into the panel's widened padding, covers no control and reaches no further over the terminal than its 24 px
  track; beside the mini rail there is no grip, only the 24 px track, which covers no rail button) and the pane's header and footer is at least 44 px on touch; so are the on-screen keys, and only the pane's terminal
  toolbar is 40 × 40 px.
- **Phones** keep the same session actions as desktop. Outside a Gaffer chat's super focus, the identity row shows the
  session and its state, with labelled **Focus**, **Theatre**, **Details** and **More** controls. Interface and approval
  choices stay visible below it, with 44 px touch targets; the toolbar wraps instead of moving them into the menu.
  **Terminal here** opens a terminal in the same folder. A Gaffer chat opens in super focus, where Back, the title, the
  state and **Exit focus** are the header and **Exit focus** restores the full controls. With the keyboard up, the site's
  top bar steps aside and the session controls remain reachable. In landscape, identity and actions share a compact row
  where there is room. Connection recovery buttons remain available in the pane.


The terminal route uses compact app navigation and the remaining usable viewport
(see [Phones](#phones) for how that height is chosen while a keyboard is open).
### The sessions panel

The panel opens with **Projects** above the session list: the same folder projects as Home's rail,
one row each with its colour, name and live count. A row's **+** starts an agent in that project's
folder and opens the new session; the row itself continues the project's live session (or its most
recent one). Right-click, Shift+F10 or the row's menu opens New session here, Show on Home and the
project's colour. The heading's **+** opens Add a project in place, so a project can be added
without leaving the terminal; the empty list says so too. Colours are saved on the project, so they
follow the person to every browser; a session's own colour, chosen from its row's menu, still wins
over its project's. A session row's hover or keyboard focus shows a card with the agent and model
the session was started with, the machine, project, branch, state, runtime and the machine's last
CPU, memory and disk readings, named last known when the report is stale.

The sessions panel scrolls independently and has three modes: **full**, a 64 px
**mini rail**, and **closed**. The sessions icon in the workspace header opens or
closes the full panel, the same simple control and place whether it is open or closed
(its tooltip names the action). The panel's own icon sits at the top of the panel —
the same top place, with the same icon, whether the panel is full, a drawer or the
mini rail — and cycles full → mini → closed (on a phone it only closes). The rules
live in `terminal/panelLayout.ts`, unit tested by
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
  rail lists every session, whatever search the full panel has.
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

**Your sessions** lists every session and its machine; machines also appear as group headings
when grouped by Machine, each with its own **+** and menu. Organising never creates or stops a
shell. Select a row to continue that exact session across machines. Sort by custom
order, name, recent metadata activity, creation or state; group by project (the default, shown as a tree with a "No project" group last), machine, state,
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

**New session** first asks what to start: an **Agent session** opens the Start an
agent dialog, a **Terminal session** opens the shell form (asking for a machine
first when none is selected), and, where the account has Gaffer, a **Factory
outcome** opens the Factory page with its outcome dialog ready (on the chosen
project's folder when the session was started from a project). An unconfirmed
creation skips the question and opens its own retry instead.

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
**Remove from history by default**, which starts as your saved default and says so
("This is your current default. Untick to change it."); toggling it and ending the session saves the
new default. Account → Profile also stores the default.
History stays visible during offline/pending termination unless you explicitly
choose **Remove from history** from the session's menu. That action hides the
entry without confirming termination: the stop remains queued for reconnection
and quota stays reserved until closure is confirmed. The immutable end
receipt stores the explicit choice; authenticated host closure inserts the
existing archive tombstone in the same transaction. A session that worked in a
worktree is cleaned up first: once closure is confirmed the controller checks the
worktree, removes it when nothing would be lost and then hides the session, so
ending and removing history is one action. Uncommitted changes are never
discarded automatically; they keep the session in history so the removal asks
you to confirm what would be deleted. Changing preferences later never changes an
already requested stop. Files and operation receipts remain.

## Skills in a session

**Skills** opens a picker for the session's machine and agent. It sits beside
Attach image in Gaffer composer and next to Commands in the agent TUI
toolbar, where it appears for agent sessions only. It lists three groups:

- skills on the machine for this session's agent, which you can insert (the
  shared `.agents/skills` folder counts for every agent);
- skills on the machine for other agents, shown dimmed and not insertable;
- reet skills that are not installed on the machine: a private draft or a
  published one, each with an **Install** button (see below).

Picking a skill inserts `Use the {name} skill.`. In Gaffer the text goes
into the message box for you to edit and send. In the agent TUI it is typed into
the terminal without Enter. Nothing is sent until you press Enter. An insertion
that would make Gaffer message longer than 2,000 bytes is refused with a
message and your draft is left as it was. A skill whose name has unusual
characters (a line break, a control character, or more than 80 characters) is
listed but cannot be inserted. Skills installed in a project folder are listed
only for sessions running in that project, or in a folder inside it. The folders are compared as written, so a session started through a symlinked path does not see skills from the real folder (or the other way round); start it from the path the machine scanned. In the reet
TUI the button is disabled, with the reason, while the agent is working, a
message is sending, the connection is not live or the session has ended. In the
agent TUI it is disabled only while the terminal is not live.

In Gaffer only, typing `/` as the first character of the composer opens the
picker too. The agent TUI has no `/` trigger because the agent owns that key.

The list is as fresh as the machine's last scan, and the picker says when that
was.

### Installing a written or published skill

A written reet skill that is not on the machine has an **Install** button in
the picker, whether it is a private draft or published. A draft's source line
says **Your skill · private draft** and its bytes come from the controller's
private revision, never a public URL. A skill whose name is already installed
on the machine for a different agent is listed once, under the other agents,
and has no Install button.

Pressing **Install** first asks you to confirm what will be added and where.
reet then asks the machine to preview the install and shows you the plan. Only
a plan that adds nothing but that one skill offers **Approve and install** in
the picker. Anything else, such as a replacement, a removal or another skill, is
handed to the Sync page. The machine writes the files after you approve; nothing
is written before that. The same steps work in Gaffer and the agent TUI;
in the agent TUI nothing is typed into the terminal.

The skill goes into the session agent's own skills folder in the home
directory (for Claude, `~/.claude/skills`), so it is available to every
project on that machine. It is not written into the session's project folder.
Only Claude, OpenCode, Cursor and `.agents` folders can be installed to. Codex
and Cline sessions get a message saying the skill can't be installed from reet
yet.

An install that does not finish leaves the machine's saved skill list as it
was: pressing **Cancel** in the picker, cancelling it from the Needs you list
or the Sync page, and an install that fails all take the skill back out (or
remove the saved list if the install created it). **Retry** after a failure
runs the install again. An install that has started can't be cancelled. If an
install fails after the machine started writing, some of the skill's files may
already be on disk even though the saved list no longer mentions the skill; the
next preview for that folder shows them.

While an install waits for you to review it, Keep synced holds off on that
machine, the Sync page's Run skips that folder, and the folder's saved list
can't be changed until you approve or cancel the install. Merging that folder's
environment into another cancels the waiting install (the merge preview counts
it), and is refused while an install is already running.

reet refuses the install, and says why, when:

- the machine is not connected to reet;
- reet has not scanned that skills folder recently;
- the machine's skills follow another machine;
- another install or sync job is waiting on that machine;
- the skill is already installed there;
- the skill is not published.

A stale saved entry with the same name, for a skill that is not on the machine, does not block an install: the chosen skill takes its place in the saved list, the plan is still shown for approval, and Cancel puts the old entry back.

The text a picker inserts is `Use the <name> skill: <its description>.`, with the description cut to one line of at most 280 characters, so the agent gets the skill's own context.

## Actions and keyboard

| Control | Effect |
| --- | --- |
| Disconnect | Close this browser attachment; leave the shell running |
| Reconnect | Attach to that existing shell; do not create another |
| New session | Choose an agent session or a shell, then create it; preserve retry IDs on uncertain responses |
| Manage → Rename | Change the title; preserve process identity |
| Manage → End session | Confirm and request termination of the exact shell and its work |
| History | Search saved Bash/Zsh commands; paste to edit or explicitly review and run |
| History → Terminal output history | Read a bounded output snapshot |
| Manage → Remove from history | Hide a closed or still-ending record; keep the queued stop, machine files and operation identity |

Intentional disconnect stays disconnected through online/visibility events.
Switching machines or sessions preserves the old shell. End requests on unavailable
machines remain pending until confirmed. Remove from history uses a persistent controller tombstone and rejects creating
or running sessions. Hiding a pending end does not confirm termination: the stop
stays queued for reconnection and quota remains reserved until closure is confirmed.
Confirmation captures the session ID/title. Failed removals stay
visible; success cannot detach a newer selection. It remains available when terminal
execution is disabled. See [the archive contract](https://github.com/eyupio/reet.sh/blob/main/docs/terminal-history-archive.md).

- Ctrl+C interrupts when there is no selected output. On Windows/Linux, selected
  output makes Ctrl+C copy. On Mac, Cmd+C copies and Ctrl+C interrupts. The visible
  **Interrupt** action always sends Ctrl+C, including when output is selected.
- Ctrl+V/Cmd+V and Ctrl+Shift+V use native paste events. The key handler must not
  also send clipboard text. Clipboard denial leaves keyboard paste available.
- Ctrl+X remains terminal input for a remote shell/editor. Native cut still works
  in editable app fields. Terminal output cannot be cut.
- Right-click opens the terminal Copy/Paste menu. Shift+right-click preserves the
  browser menu. Arrow keys, Home/End and Escape work in the local clipboard menu.
- Copy without dragging: **Copy screen** (the visible rows) is in the toolbar and
  the menu, beside **Copy last 50 lines**, **Copy last 200 lines** and **Copy all**.
  These read xterm's local buffer, never the network; trailing spaces are trimmed
  and blank lines kept. They are disabled while the Scrollback snapshot is open,
  because that view shows different text. An empty range says
  "There is nothing to copy yet."
- **Copy when I select text** in Appearance is off unless chosen. It is saved in
  this browser as `reet-terminal-copy-on-select` and copies a settled, non-empty
  selection once, saying "Selection copied." Nothing is copied while it is off.
- **Select text** in the toolbar and the menu opens a read-only bottom sheet filled
  from the local buffer (the last 2,000 lines, at most 256 KiB), so a phone's
  native long-press selection works without replacing xterm. **Copy selection**
  copies what you selected, **Copy all** copies the sheet's text, and closing
  discards it: the text is transient in the browser and is never saved. It is
  disabled while Scrollback is open, and closing returns focus to the terminal.
- Pasted text is stripped of bracketed-paste markers and every ESC byte. With
  bracketed paste enabled by the remote program, ordinary multi-line text (no
  control characters other than tab and newline, at most 64 KiB) goes in as one
  block without confirmation. Without bracketed paste, multiple lines, control
  characters and large pastes still require confirmation. A paste that is empty
  after sanitising sends nothing. A reconnect invalidates pending paste approval;
  disconnected input is not replayed.
- OSC 8 active hyperlinks remain disabled. An agent's OSC 52 clipboard write is
  accepted write-only, at most 1 MiB, and only after a recent pointer or key
  gesture in the pane or an explicit **Copy it** press; a read-back query is never
  answered and an empty write is ignored. Terminal text is not persisted to
  browser storage. Font size is a local preference.
- **Attach image** (or an image paste, the Paste button when the clipboard holds
  only an image, or a file dropped on the screen) uploads a PNG, JPEG, GIF or WebP
  image up to 5 MiB to a private per-session folder on the machine, outside the
  project. The browser inserts the returned path with a space either side and never
  presses Enter; the path is text, and the session and its tool decide whether that
  text becomes an attachment. Uploads are held in memory only and are never saved
  in browser storage. A machine that reports uploads off says "Image uploads are
  off on this machine."; an older machine says "Update reet on that machine to
  attach images." The machine keeps at most 20 images and 50 MiB per session,
  removes the folder when the session ends, and sweeps anything older than 24
  hours. The controller records one audit row with the machine, host, tool, image
  type and a size bucket only. Uploads can be turned off on the machine with
  `reet terminal uploads off`; an OSC 52 write can be turned off with
  `reet terminal clipboard off` (see [remote machine operations](remote-machine-operations.md)).

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
  grid clipping its final row. `keyboard.ts` classifies terminal-only shortcuts;
  `virtualKeys.ts` holds the on-screen key rows and how the modifiers that are on join the next
  key, covered by `web/tests/terminal-keys.test.mjs`. `connection.ts` retains lease,
  recovery, generation and input-replay boundaries.
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
`node web/tests/terminal-phone.e2e.mjs`, and `node web/tests/sessions-phone.e2e.mjs` for the session screen on phones.
The whole Agents home across every width, both themes, reduced motion and the phone keyboard is
`node web/tests/matrix/agents-home.matrix.mjs` (local only, several minutes; `REET_AXE=<axe.min.js>` adds axe-core;
results in [UI verification](https://github.com/eyupio/reet.sh/blob/main/docs/ui-verification.md#agents-home-layout-focus-and-motion-matrix--28-october-2026-pr-h-task-h4)).
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

Scroll directly in a terminal or agent TUI with a mouse wheel, trackpad, or a
vertical one-finger swipe on a phone or tablet. Tmux forwards scrolling to a
mouse-aware app so its own conversation history remains available when you
return to the session. For shells and apps without mouse support, tmux opens its
retained scrollback (up to 5,000 lines); scroll back to the bottom or press Escape
to leave tmux's copy mode.
Hold Shift while dragging to select terminal text when mouse reporting is active,
or use Select text for native selection on touch devices. Taps, horizontal drags
and two-finger pinch zoom are not converted into terminal scrolling. Scrolling
while disconnected sends nothing and is never replayed.

**Earlier output**, at the start of the toolbar (or Shift+PageUp while the terminal has focus), opens earlier output
inside the terminal panel. It reads the machine's most recent 2,000 lines / 256 KiB
using the existing bounded output-history API. Scroll vertically/horizontally,
select text, use Page Up/Down, or search and move between literal matches. It is a
read-only snapshot; Refresh obtains newer output, and Back to live returns input
focus to the same running shell. Its header's **Copy all** copies the snapshot it
is showing without saving it. Nothing is saved in browser storage. The live
xterm stays mounted and receives redraws behind the reader; no keystrokes from
reading/search are forwarded. An older connector without mouse reporting opens
this reader when you scroll upward, rather than sending cursor keys to the shell.
Full-screen apps can keep conversation history inside the app; this snapshot
contains only the output tmux retained. Use direct scrolling for the app's own
history. Tmux's alternate-screen rendering means a browser-only xterm scrollbar
cannot expose its machine-side history. Neither view reconstructs output the app
has discarded.

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
The pane header, terminal toolbar, extra-key row and footer lose their vertical padding. On a
touch screen every toolbar button and on-screen key is at least 40 × 40 px; on a touch phone so
are the workspace toolbar's buttons (back, +, Sessions, New session, Details & help), the pane
header's controls and the footer's Focus terminal (a touch tablet makes all but the terminal
toolbar 44 px). The screen frame padding is 4 px. The toolbar and the key rows
scroll sideways; the end that hides more controls fades. A key row is focusable, so
arrow keys scroll it; that tab stop also appears in mouse windows narrower than 650 px,
where the keys show too. The keys themselves are described under
[On-screen keys](#on-screen-keys). The Alt+Shift+M advice is hidden on touch screens
(no hover, coarse pointer); Restore size stays.

### On-screen keys

A phone or tablet keyboard has no Esc, Tab, Ctrl or arrow keys, so the terminal draws its own
under the screen. They show on any touch screen, whatever its width, and in a mouse window
650 px wide or less.

- **The row that stays** leads with **Esc**, **Tab**, **Ctrl**, **Alt** and **Shift**, then
  **^C** (Ctrl+C) and `/` `|` `-` `~`. That part scrolls sideways. The four arrows and the
  button for the second row sit at its end and do not scroll, so they are always in view. On
  a 390 px phone Esc, Tab and Ctrl are in view before any scrolling; on a tablet the whole row is.
- **Modifiers.** Tap Ctrl, Alt or Shift once and it is on for the next key only; tap it again
  and it is locked on, shown with a bar along its foot, until a third tap lets it go. Several
  can be on at once. They join a key as xterm sends it: Ctrl with Up is `CSI 1;5A`, Ctrl and
  Shift with Right is `CSI 1;6C`, Ctrl with `c` is `0x03`, Alt leads with Escape. They join
  what the device's own keyboard types too, so a locked Ctrl and the letter `l` clears the
  screen. Pasted text is never changed, and uses up a modifier that was on for one key.
- **Hold to repeat.** An arrow, Backspace, Delete, Page Up or Page Down sends once on a tap
  and, held, again every 70 ms after 400 ms, as a keyboard does. Letting go, sliding off the
  key or the row starting to scroll stops it. Keys that act once (Esc, Tab, Enter, Ctrl
  combinations, F keys) never repeat.
- **The second row** opens above the first from the keyboard button and takes one row's
  height from the screen; the first row does not move. It holds every other key, one group at
  a time: **Edit** (Enter, Backspace, Delete, Home, End, Page Up, Page Down, Insert), **#+=**
  (symbols a phone buries behind a symbol page), **^X** (common Ctrl combinations) and **Fn**
  (F1–F12). The group buttons stay in view while a group's keys scroll. Whether the row is
  open, and which group, is remembered in this browser.
- **Focus.** On a touch screen a key never moves the focus, so the device's keyboard stays as
  it was, open or closed: the arrows and Esc can drive an agent's interface with the keyboard
  down and the whole screen in view. With a mouse the terminal takes the focus, as before.
- Every key is a button with its own name for a screen reader ("Send Up arrow", "Ctrl locked").
  The toolbar's **Interrupt** is Ctrl+C exactly, whatever modifiers are on.

The keys and how modifiers join them are in `web/src/lib/terminal/virtualKeys.ts`
(`web/tests/terminal-keys.test.mjs`); the rows are in `TerminalPane.svelte`
(`web/tests/terminal-phone.e2e.mjs`).

**Sideways phones.** The toolbar and the extra keys share one row, each scrolling with
its own fade, and the pane footer is hidden (the state stays in the header). At 844 ×
390 the screen shows 13 rows with the keyboard closed.

**Keyboard focus layout.** While the on-screen keyboard is open and the focus is in the
terminal, the site header (through the shell's `keyboardChrome`, focus `session`), the
workspace toolbar and the pane footer are hidden; held sideways, the pane header is hidden
too (8 rows at 844 × 390 instead of none). Upright, the session screen's slim header stays
and the pane header steps aside instead.
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
Two more lines give the sideways reading: `innerWidth`, `visualViewport.width`, the page
scale and `offsetLeft`, then the document's `clientWidth` and `scrollWidth`, `screen.width`,
the pixel ratio and the viewport meta in force. Use them when the page looks narrower or
zoomed after focusing a field (a tablet, say): the widths and scale say which.
It never takes focus or touches, so the keyboard stays open while you read it; nothing
is stored or sent. See `handoff.md` for what to record.

## Session workspace details

The right-hand panel keeps the session's status and workspace alongside the conversation or terminal. At smaller widths, **Details** opens the same panel as a sheet; Escape closes it and returns focus. Its tabs support Left/Right arrow keys, Home and End, and the chosen tab is remembered in this browser: opening another session keeps **Changes**, **Files** or **GitHub** selected rather than resetting to **Overview**. **Session** returns focus to the conversation composer or terminal.

- **Overview** answers "where has this got to, and what is next" without opening the other tabs. Structured chats start with Gaffer’s work-update card, including his portrait, tool activity and handover/job sheet. It follows the account’s compact-card preference and can be expanded here. The bottom of the narrative keeps a separate compact status strip with an activity spinner, current action and reported tool results; it has no mascot. The separate **Ask Gaffer** companion floats over the workspace's corner without
  reserving a column beside it; while this panel is open its header carries **Ask
  Gaffer** and the floating circle steps aside. Its panel sizes, history and reviewed session
  delegation are described in [Gaffer AI](gaffer-ai.md). The Overview leads with the
  live status, then **Progress** and the **Pull request**, so the answer to "where has
  this got to, and what is next" is visible without scrolling. The sections, in order:
  - **A header** with the reported state and its source, a run clock line (`Started 14:02 · running 38m`, or `Ran 1h 12m` once the session has finished, or once a chat's latest turn has (the chat's agent keeps running between turns, and the clock starts again when the next turn begins); a finished session with no recorded end time shows only when it started) and one status line that carries a request waiting for you, a chat error, or otherwise the state and last activity. The line keeps its height whether or not something is waiting, so the sections below do not move when a request arrives. Gaffer chats reuse the existing conversation stream. The header carries no return summary or tool tally: Gaffer's card and **In progress** already say how the work is going. Terminal sessions report process/output activity rather than invented steps.
  - **Progress** is a tracker of five steps, **Code** (**Planning** while the agent is in a planning mode), **PR**, **Checks**, **Review** and **Merge**. Each step carries an icon and a word (Done, Now, Needs you, Not yet or Not read), so colour is never the only signal. The steps are built from facts the machine reported and nothing else: no reported checks is never shown as green ("None reported", Not read); review threads that were not read are never shown as zero, and an approved pull request whose threads were not read stays Now, not Done; a truncated thread list (100 threads) says "100+ threads, not all read"; and Merge is never claimed from green checks alone, only "Everything reported is done" once checks, review and mergeability all say so, with the words "GitHub decides whether it can merge" because the panel cannot know branch rules. A merged pull request collapses to one line with a link; a closed one says "Closed without merging". Where the machine could not read GitHub the later steps say "Not read" with the reason, and only a machine that answered "no pull request" shows them as Not yet. From the default 360 px panel up the five steps sit in one row, each an icon over its label and its state word; in a panel narrower than about 404 px a state word may take two lines ("Needs / you"), between about 306 and 353 px **Planning** reads **Plan**, and a panel narrower than about 306 px (the 300 px minimum) sets the steps three over two with every label whole. No label or state word is ever cut.
  - **Next** is a single sentence below the steps, chosen in order: a request waiting for you, merged or closed, conflicts, failing checks, requested changes, unresolved threads, running checks (named), a draft, a required review, and "Ready. GitHub decides whether it can merge." It may carry one link (Open pull request, Open failing check or Open review) or a **Review** button that opens the conversation. For conflicts, failing checks and unresolved threads it also offers a quieter **Ask the agent to fix the conflicts / the checks / address the threads** action. That action writes the request into the chat composer for you to read and send; it never sends it. The text names the base branch or up to three failing checks (cut to 60 characters, with control characters and links removed, because those names come from GitHub). If you have half-typed a message the request is added on a new line, and asking twice does not stack a second copy. A terminal session has no composer, and a chat whose composer is disabled, not on screen or too full to take the text without cutting it, would lose it: in those cases the button copies the message to the clipboard instead ("Copy message for the agent to fix the conflicts" in a terminal), and says plainly when the copy failed. The sentence takes two lines at most; its link or button and the fix action share one row beneath it, and what the last fix click did is said on the line under that. In a panel narrower than about 393 px for a terminal session (its **Copy message for the agent** is longer) the two take a row each, and in one narrower than about 338 px for every session, where the note then has two lines. Whichever it is, the merged row is as tall.
  - **Pull request** is a summary: the number, state and draft flag, the title on one line (whole in its tooltip), the verdict, one row with the failing, running and passed check counts (each with its own icon; the words are its name and tooltip), the review decision (or, with none, how many reviews the machine read) and **Conflicts** when there are any (on two lines in a panel narrower than about 388 px, the default included, so none of these words is cut), then **Open pull request** and **Details**. **Details** is a fold, closed until you open it and left open by refreshes and polls, that holds the rest: the check counts in words with a meter, the review summary, open threads, conflicts, added and removed lines, when the branch was last pushed, **Open failing check**, **Open review**, **Copy link** (named "Copy pull request link"; offered only for an `https` link) and **Full detail**, which opens the GitHub tab. A machine that cannot say who reviewed or which threads are open shows the review decision alone and "Review threads not read on this machine". While GitHub is being read, was not asked, or has no pull request for the branch, the same space holds one line saying which.
  - **You asked** shows the first two lines of the prompt the session was started with; **Read full prompt** opens it whole in a dialog, where it can be copied. The controller keeps the prompt with the session (at most 8,000 bytes) and clears it when the session is archived, so it is the opening prompt as sent and not a reading of the chat; see [Launch requests](terminal-launch.md#what-is-kept). The full text is fetched only when the dialog opens, and **Copy prompt** is withheld until it has arrived so a cut-off excerpt is never copied as the whole. A session started before reet kept prompts says so ("started before reet kept opening prompts") and, for a chat whose history is fully loaded, falls back to its first message.
  - **In progress**, **Latest agent update** and the workspace sections tell the current story after the prompt: what the agent is doing now, its latest message, the checkout and where it is working.
  - **Workspace changes** summarises the checkout and opens **Changes**, and **Where it's working** lists the repository (a link to GitHub when the machine reports an `owner/name` remote), machine and its state, project, branch (with the pull request's base beside it when known), whether the session has its own worktree or the shared checkout, the agent and when it started. The branch and the worktree path have copy buttons; a long path is shortened in the middle and shown whole in its tooltip. Skills and decisions sit in a fold at the end.

  The panel's footer times the workspace read, not GitHub's, so the tracker and the pull request card say for themselves when what they show may be out of date. When the machine is offline or paused, or the GitHub read is more than 180 seconds old (the GitHub tab's own threshold), the Next line starts with `Last known · checked 12m ago` and the card shows the same words beside its number. The Next sentence is the last known truth, not a live one. A read with no recorded time is not marked either way, and a request from the agent, which comes live from the chat, is never marked.

  The Overview never moves its sections when the agent moves on: the run line, the headline, the one-line status, the tracker, the pull request card, the prompt card, In progress, failures, the latest update, the changes card and the Where card each fill a slot of fixed height, so a new task, a changed pull request or a finished read changes the words inside a slot and nothing after it shifts. Long text is cut with an ellipsis or scrolls inside its slot, the changes and pull request cards hold their height while the first read is pending, and the folded pull request card is one height whatever the pull request says. Opening **Details** is the one thing that makes it taller, and its rows are fixed too while it is open. Every button and link is at least 44 px tall on a touch screen. The GitHub read for the pull request is the one the rest of the panel uses; the Overview does not make its own.
- **Changes** reads both ordinary checkouts and linked worktrees. Changed files show status, added/removed line counts with proportional bars, and merge conflicts. Select a file for a read-only diff against HEAD or a file preview. Untracked files have no diff counts; unavailable counts are left unknown. Upstream ahead/behind counts are local observations, without fetching.
- **Files** browses one folder at a time under the session's recorded workspace. Links, Git metadata and special-file previews are excluded. Previews and listings are bounded, with truncation labelled. File browsing respects the machine's local folder-browse consent.
- **GitHub** reads the branch's PR using `gh pr view` on the session's machine. It needs an explicit github.com origin, the GitHub CLI and an existing sign-in there. It leads with what needs attention — merge conflicts, failing checks, requested changes, running checks or a review — and names the checks still running or failing. A coloured check meter and counts summarise failing, running and passing checks; failed and pending checks appear first, with passing/skipped checks folded. Review decision, merge conflicts and change totals follow below. Check links open provider details. For an open pull request the machine makes one more read-only request, `gh api graphql`, for the latest review states, review requests, auto-merge, review threads (the first 100) and the last commit's date; it needs the machine's `gh` sign-in to allow GraphQL, and when it fails the rest of the panel still shows, with those figures left out rather than shown as zero. A merged or closed pull request makes no second request. The machine also reports the repository as `owner/name` for a link, even before a pull request exists. A machine that sends these extra fields to a controller from before this change has its reply refused, so update the controller before the machines that connect to it. Missing checks do not establish success, and the panel does not claim merge eligibility from green checks alone.

Local status refreshes every 15 seconds while working and every minute while quiet. GitHub is read automatically at most once every two minutes for a visible session while the Overview or GitHub tab is selected, because the Overview shows the pull request too. The panel and the navigation's background read share one store, so a session whose pull request was read moments ago is not read again when you open it, and a read already in flight is not repeated. Returning to a visible browser tab refreshes the current status and selected GitHub view. Hidden browser tabs pause inspection polling. **Live updates** pauses all automatic reads, including the Overview's GitHub read and the background one for sessions in the navigation; manual Refresh remains available and reads at once. The **GitHub** tab carries a small dot once the pull request has been read (red: needs attention, amber: in progress, green: checks passed or merged), repeated in the tab's accessible name; there is no dot while nothing has been read or when the pull request is closed. Green never means ready to merge. Files and previews refresh on navigation or manually. Each view displays its last check time; a failed refresh retains a clearly marked last-known result when available. Compact file/PR signals appear in the navigation for sessions inspected during this visit, with last-check tooltips. Each session on a machine that offers `session-inspect-v1` also has its branch's pull request read in the background (about every two minutes while live and every ten while quiet), so a PR created in a session shows its number without opening that session's GitHub tab first. These reads are in-memory only for this visit.

Workspace inspection requires a machine running reet with `session-inspect-v1`. Older machines retain manually refreshed worktree path listings and show an update message for new views. Inspection uses the existing account browser-session checks, controller-owned workspace paths, local terminal consent, bounded private-protocol replies and background workers. It never edits files, stages, commits, fetches, pushes or merges. Git hooks, fsmonitor and clean/process/diff helpers are disabled for the inspection commands. Partial clones are refused rather than allowing an implicit object fetch. GitHub uses the machine's configured credentials; no new token is requested or copied to the browser.
