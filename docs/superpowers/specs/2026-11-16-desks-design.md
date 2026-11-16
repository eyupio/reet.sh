# Desks: saved sets of terminal tabs with splits and full restore: design

16 November 2026. Status: awaiting the owner's approval. Nothing here is built.

## Intent and why

The owner runs several terminals at once for one job (for example a deploy: pull in one folder, logs in
another, a database shell on a second machine) and today does it in Ovis, outside reet. In reet each
terminal is a separate session, tabs are forgotten on reload, only one terminal is on screen at a time
(two with Terminal beside), and a shell lost to a reboot comes back, by hand, in the folder it was
started in with the machine's shared history.

The owner wants that working set to be a thing in reet: named, saved, reopened later on any browser, with
every tab back in its folder with its own command history, shown side by side when wanted. Ovis
(github.com/devswha/ovis, read at v0.5.4) is the reference, to be improved upon: it has no tab colours, no
per-tab history, and its reconnect replays a raw 128 KiB tail of output.

**The name is Desk.** "Project" is a folder project, "workspace" is the Sessions screen, "Workbench" is
taken. The owner chose Desk.

## What the owner decided

| Decision | Choice |
| --- | --- |
| Restore depth | Full: live folder and per-pane command history, restored after a machine reboot. Needs reet updated on each machine. |
| Name | Desk |
| What a pane can hold | A plain shell or an agent's own terminal (agent TUI). Not a Gaffer chat. |
| Navigation | Desks is its own top-bar tab, as Factory is. A desk's terminals are ordinary sessions and are also listed in Sessions. |
| Tabs and splits | A tab holds panes (my call after the owner's answer went to navigation; see Open points). |
| Closing a pane | Asks each time: End the shell, or Take off this desk and keep it running. |
| Extras in the first version | Type in all panes; a start-up command per pane; duplicate a desk and start one from a template. |
| Build approach | A, below. |
| Look (from the mock-ups in [docs/design/desks](../../design/desks/index.html)) | Underline tabs; a pane's header tinted in its colour; one shared terminal toolbar for the focused pane; Desks opens the desk last used, with the list behind the switcher and shown when there are no desks. |
| Session limits | Raised for everything: 24 live sessions a machine, 64 an account (from 8 and 32). A pane that still meets the limit says so and offers Try again. |
| A folder outside home | A desk pane may be reopened in any folder its machine user can enter, but only a folder that pane's shell was reported in. Every other launch keeps the home rule. |

## Goals

1. A desk holds tabs; a tab holds one to nine panes; a pane shows one terminal session on any of the
   person's machines. One tab may mix machines.
2. A desk saves itself as it is changed, to the account, and opens the same on any browser.
3. Tabs and panes can be renamed, coloured and reordered. Panes can be split right or down, resized,
   swapped, zoomed and arranged by presets (side by side, stacked, 2×2, 3×3, main and stack).
4. Each shell pane knows the folder its shell is in now, and keeps its own command history.
5. After a reboot of the machine, a pane offers a fresh shell in its last folder with its history loaded,
   one press for the pane or one for the whole desk.
6. It stays quick with nine panes on screen and stays correct through dropped connections, two browser
   tabs on the same desk, and a machine that is offline.

## Non-goals (first version)

- Gaffer chats in a pane. Sharing a desk with a team. Exporting a desk to a file (the owner did not pick it).
- Splits on a phone: a phone shows one pane at a time with a pane switcher.
- Per-pane history for shells other than bash and zsh. They get folder restore only, and the pane says so.
- Restoring a running process. A reboot ends processes; reet restores the place and the history, and
  offers the start-up command. reet's standing rule holds: shell persistence is not conversation recovery.
- Showing one session in two panes at once. A session keeps one controlling attachment.

## Anti-goals

- A command run without a press. A start-up command is offered, never run by reet on its own.
- Command history or terminal output stored on the controller or in the browser.
- A desk that silently shows a different shell from the one saved (a missing folder is said, not replaced
  by the home folder).
- Typing sent to panes the person cannot see.

## What exists and is reused unchanged

- A session is a tmux session on the machine (`internal/terminalhost`), surviving browser, controller and
  connector restarts. Attach is a disposable tmux client; a reattach repaints (`connection.ts`), with
  backoff 1 to 15 s, 32 KiB frames and a 256 KiB browser queue.
- `TerminalPane.svelte` (xterm 6, fit, search, serialised writes, debounced resize) and its toolbar,
  appearance, Earlier output and History drawers.
- `shell-dir-v1`: a plain shell created in a named folder.
- Session rename (`PATCH /api/v2/terminal/sessions/{id}`), end, archive, the attention fields, Sessions.
- The colour palette in `web/src/lib/projectColour.ts`.
- Optimistic versions on saved records (`folder_projects.version`, `expect`).
- Feature negotiation in `terminalproto` and `agreedControlFeatures`.

## Approaches

**A. Desk record on the controller, splits in the browser, one session per pane (chosen).** The desk is
one versioned record on the account. Each pane names an ordinary session. The browser draws the split
tree and attaches each visible pane with the existing attachment. Mixed machines in a tab work, Sessions
lists every pane for nothing, and the tested survival and reconnect paths are reused. Cost: one socket per
visible pane, bounded by nine panes a tab.

**B. tmux draws the splits on the machine.** One tmux window per tab with native panes and one
attachment. Fewer sockets, but a tab cannot span machines, panes stop being sessions (so they leave
Sessions, attention, rename, end), and names and colours would be forced into tmux. Against goals 1 and 3.

**C. Desks kept in the browser.** Quick, but a desk does not follow the person, and two browser tabs
overwrite each other. Against goal 2.

The owner chose A.

## Design

### The record

Table `desks`: `id, user_id, name, name_key, colour, position, template (0/1), doc (JSON text), schema,
version, created_at, updated_at`. At most 50 desks an account, 20 tabs a desk, 9 panes a tab. `doc`:

```ts
Doc  = { tabs: Tab[]; active: TabId }
Tab  = { id; name; colour; root: Node; zoomed: PaneId | ''; active: PaneId }
Node = { pane: PaneId } | { split: 'right' | 'down'; ratio: number /* 0.1–0.9 */; a: Node; b: Node }
Pane = { id; name; colour; host /* machine enrollment id */; session /* '' when none */;
         kind: 'shell' | 'agent'; dir /* last known folder */; dirAt; start /* start-up command, ≤ 2000 */ }
```

Panes are held in `Tab.panes: Record<PaneId, Pane>`; the tree holds only ids. Rules, enforced by the
controller on every write and unit tested in one pure package (`internal/desk`): every tree leaf names a
pane of the tab and every pane is a leaf exactly once; ratios are clamped; names are at most 80
characters; colours are from the palette; a session id appears in at most one pane across the account's
desks. `schema` is 1; a controller that reads a higher schema neither shows nor overwrites the desk.

Geometry is computed top-down from ratios only, never read back from terminal sizes (the feedback loop
Ovis hit). A preset is a function that builds a tree from the tab's panes in order; it is not a second
model. Closing a pane gives its space to its sibling.

A pane's name defaults to the session's title, and its folder's last element when the title is the
default. Renaming a pane renames the session (the existing PATCH), so Sessions and the desk agree. A
pane's colour is saved in the desk and wins over the project's on the desk; Sessions shows it as the
session's colour through the same `sessionColourOf` order, read from the desk list.

### Routes

All under the terminal access rule (accounts, cookie, same origin), writes with `expect` version, 409 with
the current record on a stale write:

- `GET /api/v2/desks` (summaries: id, name, colour, position, template, tab and pane counts, version,
  and the session ids each holds), `POST /api/v2/desks` (empty, `from` a desk or template, or
  `template: true`), `GET|PATCH|DELETE /api/v2/desks/{id}`, `POST /api/v2/desks/reorder`.
- `PATCH` takes the whole `doc` (it is small: under 64 KiB, refused above). The browser sends changes
  debounced 400 ms, serialised one at a time; on 409 it re-reads, replays its pending local change onto the
  newer doc when it still applies (rename, colour, ratio, add), otherwise drops it and says "This desk
  was changed in another window. Showing the latest." A `BroadcastChannel` tells other tabs of the same
  browser at once; other browsers pick it up on a 15 s poll while visible and on focus.
- `POST /api/v2/desks/{id}/panes/{pane}/reopen` creates a fresh session for a pane whose session is gone
  (below) and writes the new session id into the doc in the same transaction as the create's record.
- Deleting a desk asks: end its shells, or leave them running in Sessions. Templates hold no sessions.

Audit lines for create, delete and reopen, as other terminal writes have.

### Machine side: one new feature, `desk-shell-v1`

A machine that negotiates it does three things for a plain shell whose create carries `Desk: {Pane}`:

1. **Per-pane history.** The shell is started through a wrapper reet writes under
   `~/.reet/desks/shell/`: for bash an rc file given with `--rcfile` that sources the person's
   `~/.bashrc`, then sets `HISTFILE=~/.reet/desks/history/<pane>.bash`, `shopt -s histappend` and adds
   `history -a` to `PROMPT_COMMAND`; for zsh a `ZDOTDIR` whose `.zshenv`/`.zshrc` source the person's own,
   then set `HISTFILE` to `<pane>.zsh` with `INC_APPEND_HISTORY`. So each command is on disk as it is
   run, and a later shell for the same pane loads it. Files are 0600 in a 0700 folder, capped by the
   shell's own `HISTSIZE`. Another shell (`$SHELL` neither bash nor zsh) starts as today and the machine
   reports `history: false` for the pane. The pane id is validated (`terminalproto.ValidID`) before it
   is used in a path.
2. **Live folder.** Reconcile and `SessionChanged` carry `Dir` for a running session: tmux's
   `#{pane_current_path}` for the session's pane, read in the reconcile sweep that already lists panes
   (`manager_tmux.go:174`) and on activity, at most once every 2 s a session, sent only when it changed.
   The controller keeps it on the session record (`terminal_sessions.live_dir`, `live_dir_at`) and copies
   it into the desk pane's `dir` when the session ends or is lost, so the last folder outlives the session.
   This works for every shell and for agent terminals, and needs nothing in the person's shell config.
3. **Per-pane history read.** `command-history` for a session with a desk pane returns that pane's file
   first, then the shared files as today, each entry marked with its source, so the History drawer can
   show "This pane" above "This machine".

Deleting a pane for good (End the shell, or deleting the desk with its shells) sends a clean-up that
removes the pane's history file. Taking a pane off a desk keeps the file with the session.

A machine without `desk-shell-v1` can still be on a desk: its panes work, start in a folder where it has
`shell-dir-v1`, and show "Update reet on that machine to keep this pane's folder and history."

### Restore

A pane is in one of these states, drawn in the pane itself, never as a blank terminal:

| State | Shown | Action |
| --- | --- | --- |
| Live | the terminal | |
| Reconnecting, machine offline | the existing words, last screen kept | Retry |
| Open elsewhere (409) | "Open in another window." | Use it here (takeover) |
| Ended or lost, shell | "This shell ended." / "studio restarted, so this shell was lost." with the folder | **Reopen in ~/PROD_reet** |
| Ended or lost, agent | as above | **Continue** where the agent resumes (`canContinue`), else **Start a shell here** |
| Folder gone | "~/old no longer exists on studio." | Choose a folder, or Reopen in home |
| At the limit | "studio already has 24 sessions running." | Try again |
| Machine removed | "This machine was removed from reet." | Choose a machine, Close pane |
| Empty (template, or never started) | machine and folder | **Start** |

Reopen creates a plain shell with `Dir` = the pane's last folder and `Desk.Pane` = the same pane id, so
the history file is picked up. If the pane has a start-up command, the new terminal opens with a bar
above the prompt: the command, **Run** and **Dismiss**. Run types it and presses Enter through the
normal input path; nothing is run otherwise. The desk header has **Reopen all** when two or more panes
can be reopened; it reopens them at most four at a time and reports any that could not be.

**Outside home.** `ResolveDir` refuses a folder outside home. A reopen sends `Desk.Dir` instead of
`Dir`; the machine accepts it only on `desk-shell-v1`, only for a plain shell, only when the folder is
absolute, exists, is a directory and the machine's user can enter it, and the controller sends it only
when it equals the `dir` it last recorded for that pane from the machine's own report. A person cannot
type such a folder: New tab and Choose a folder keep the home rule.

**Limits.** `ErrTerminalCapacity`'s two numbers become 24 a machine and 64 an account, in the store and
in the web's copy of them (`TerminalWorkspace.svelte`), with the tests that name them.

Opening a desk never creates a shell on its own, the same rule as opening a session.

### The Desks screen

`/desks` (list) and `/desks/{id}`. The top bar gains **Desks** after Sessions for an account that can
start sessions; on a tablet and phone it is under More, as Factory's secondary tabs are.

- `/desks` opens the desk last used in this browser (the one stored id); with none, or when it is gone,
  the list. The desk's name is the switcher: the other desks, All desks, New desk.
- **List:** cards in the person's order: name, colour, tab and pane counts, the machines it spans, how
  many panes are live, anything waiting (the attention fields). **New desk** (empty, or from a
  template), and each card's menu: Open, Rename, Colour, Duplicate, Save as template, Delete. Empty
  state names what a desk is and offers New desk.
- **A desk:** a header (desk switcher, name, Reopen all, layout menu, Type in all panes, the menu), the
  tab bar, the split area. The whole width; no footer; theatre is the header collapsing to the tab bar.

```
Home  Factory  Sessions  [Desks]  Machines  Skills
┌ prod deploy ▾ ───────── Reopen all · ⊞ Layout · ⌨ All panes · ⋮ ┐
│ ● deploy │ ● logs │ db │ +                                       │
├───────────────────────────┬──────────────────────────────────────┤
│ ▍reet · studio ~/PROD_reet│ ▍platform · studio ~/PROD_platform   │
│ $ docker compose pull     │ $ tail -f app.log                    │
├───────────────────────────┴──────────────────────────────────────┤
│ ▍snicket · dev_snicket ~/app                                     │
│ $ _                                                              │
└──────────────────────────────────────────────────────────────────┘
```

- **Tabs** are flat with a colour marker and an underline in the tab's colour on the current one.
  Click to switch; double-click or F2 to rename in place; drag to reorder; a menu (right-click,
  Shift+F10, ⋮) with Rename, Colour, Duplicate tab, Close tab; a dot when a hidden tab has a bell or a
  waiting question; **+** opens "New tab": machine and folder (the folder browser), defaulting to the
  focused pane's machine and live folder.
- **Panes:** a one-line header washed in the pane's colour: name, machine, folder (middle-shortened, full in the
  tooltip), state, and a menu: Rename, Colour, Split right, Split down, Zoom, Swap with…, Move to tab…,
  Start-up command…, Open in Sessions, Close. The focused pane has a 2 px accent outline and its header
  in the stronger text colour; focus follows click, never hover. Dragging a pane header onto another
  pane's edge moves it there, with a drop preview. Separators are `role="separator"` with a 24 px hit
  area, pointer capture, arrow keys in 2 % steps and double-click to even out; a pane is never made
  smaller than 20×8 cells, and a split that would do so is refused with the reason.
- **The pane's terminal toolbar** (Compose, Search, Copy, History and the rest) is drawn once, above the
  split area, for the focused pane, so nine panes do not carry nine toolbars.
- **Keys** (all rebindable later; none shadows a shell key): Alt+Shift+D split right, Alt+Shift+E split
  down, Alt+Shift+arrows move focus, Alt+Shift+Z zoom, Alt+Shift+W close pane, Alt+Shift+T new tab,
  Alt+Shift+[ and ] previous and next tab, Alt+Shift+1…9 tab by number. A **Keys** entry in the menu
  lists them.
- **Type in all panes:** a toggle per tab, never saved, off on every load and on every tab switch. While
  on, the tab has an amber stripe and each receiving pane's header says "Receiving"; input, paste and
  Compose go to every live pane of the tab that is on screen and connected, and the count is shown ("Typing in 3
  panes"). A pane that is zoomed away or not live does not receive, and the count says so.
- **Closing a pane** with a live shell opens a dialog: **End the shell** (default, Enter) or **Take off
  this desk** ("It keeps running in Sessions."), Cancel. An ended pane closes at once. Closing a tab
  asks once for all its live panes.
- **Adding what exists:** a session's menu in Sessions and Home gains **Add to a desk…**; the desk's
  **+** menu has **Add a running session…**. A session already on a desk is offered as "Move here".
- **Phone:** one pane at a time, the tab bar scrolls, a pane switcher (the pane names as chips) under
  it; splits, presets and Type in all panes are not offered; the saved tree is kept untouched.
- Sessions marks a session that is on a desk with the desk's name, which opens the desk at that pane.

Copy follows `docs/voice-and-copy.md`: familiar labels, plain destructive confirmations, British spelling.

### Performance and resilience

- Only the active tab's panes are mounted and attached. A hidden tab holds no xterm and no socket; tmux
  keeps the shell and repaints on return. Switching tabs unmounts as today.
- A zoomed pane keeps its hidden siblings attached but not rendered (xterm paused through
  `IntersectionObserver`; no fit on a pane with no size), so unzoom is instant and no output is missed.
- xterm keeps the DOM renderer. WebGL has a per-page context limit near sixteen and reet sets
  `screenReaderMode`; a WebGL pass is a separate change with its own measurements.
- Fit runs once a frame per pane from its ResizeObserver; a separator drag holds sizes (`holdSize`) and
  sends one resize on release, so nine shells are not resized sixty times a second.
- Attaches for a tab are started at most three at a time, each with the existing jittered backoff, so a
  controller restart does not bring nine simultaneous tickets from every open desk.
- Each pane fails alone: an offline machine, a 409 or a lost shell is that pane's state; the rest of
  the tab is untouched.
- The desk doc is read before any pane mounts; a failed read keeps the last doc held in memory and says
  so, as Sessions does. Nothing of a desk is kept in localStorage except the last opened desk id.
- A write that fails is retried with backoff and the header says "Not saved yet"; leaving the page with
  an unsaved change sends it with `keepalive`.
- A session ended or archived elsewhere (Sessions, the machine) is noticed from the session list the
  desk already polls; the pane moves to Ended with its last folder. Session records are never deleted
  except when a machine's enrolment is revoked (they go by cascade); removing a session from history
  archives it. The controller clears a pane's `session`, with a version bump, in the same transaction as
  either: the archive (`ArchiveTerminalFor`, and an end that asked for the session's removal), after
  which the pane reads Not started and offers Start, and the revoke. So a doc never names a session that
  no longer exists or that Sessions no longer shows.

## Calls made after the builder's read (mine, for the owner to check)

- **Login shell kept.** Plain shells are login shells today (tmux's default shell, no command). The
  wrapper keeps that: for bash, the rc file sources `/etc/profile` and the first of `~/.bash_profile`,
  `~/.bash_login`, `~/.profile` when the shell would have been a login shell, then `~/.bashrc` if those
  did not, then applies the history settings last so the person's files cannot undo them; for zsh the
  `ZDOTDIR` has `.zshenv`, `.zprofile`, `.zshrc` and `.zlogin`, each sourcing the person's own, and zsh is
  started with `-l`. The shell is the one a plain create starts today (the validated `SHELL` the service
  passes), not a new lookup. A real-shell test checks that a `PATH` set in a profile file is present.
- **Agent panes.** A desk's **+** menu has New shell, **Start an agent…** (the existing dialog, with the
  focused pane's machine and folder; the session it makes is put in the new pane) and Add a running
  session. `Desk.Pane` is sent only for plain shells. Agent panes get the live folder (it is reported
  for every running session) and no per-pane history. **Continue** makes a new session through the
  existing path and that session takes the pane.
- **Reading the live folder.** One extra tmux call a sweep (`list-panes -a -F` with the session name
  first and the path last), not a change to `allPanes`; a path with a newline is skipped. It is read on
  the sweep, on attach and when the session ends; there is no per-activity hook, and none is added.
- **A machine forgotten, merged or revoked.** A merge keeps the enrolment id panes name, so they stay
  valid. Forgetting a machine (`ForgetMachineFor`) removes only what it listed: its enrolment and
  sessions stay, so its panes keep their shells and its reported folders stay. Revoking its enrolment
  (`ControlSyncAgent(..., "revoke")`) deletes its sessions by cascade, so that transaction clears every
  pane holding one of them, with a version bump, and removes the machine's `desk_sessions` and
  `desk_pane_dirs` rows; a pane on a machine that is gone shows "This machine was removed from reet."
  offering Choose a machine and Close pane.
- **Type in all panes turns itself off** when the set of receiving panes changes for any reason other
  than the person's own action (a takeover elsewhere, a lost connection, a shell ending), and says
  "Stopped typing in all panes: snicket is no longer connected."
- **Colour in Sessions.** The person's own choice in Sessions (kept in that browser) wins there, then
  the desk pane's colour, then the project's. On the desk the pane's colour is always used.

## Order of work (one draft PR, in slices that each build and pass)

1. `internal/desk` (doc rules, tree operations, presets) and its twin in `web/src/lib/desks/tree.ts`,
   with tests; the store table and routes with integration tests.
2. The Desks tab: list, a desk with tabs of one pane, rename, colour, reorder, add and close, autosave
   and the stale-write path.
3. Splits: the tree view, separators, presets, zoom, swap, move, drag, keys, the shared toolbar.
4. `desk-shell-v1` live folder: protocol, machine, controller, pane header, Reopen and Reopen all.
5. `desk-shell-v1` history: the wrappers, the per-pane read, the History drawer's two groups, clean-up.
6. Type in all panes; the start-up command; duplicate and templates; Add to a desk from Sessions.
7. Docs (`docs/desks.md`, terminal-workspace, command-history, roadmap, handoff), Playwright checks
   (`web/tests/desks.e2e.mjs`, phone variant), `docs/ui-verification.md`, light and dark, phone width.

## Testing

- Go: table tests for doc validation and every tree operation; store tests for versions, limits and the
  one-pane-per-session rule; API integration tests for each route and the stale write; protocol tests
  that an older controller or machine never sees the new fields; machine tests for the bash and zsh
  wrappers against real shells (a command run in one shell is in the next shell's history; the person's
  rc is sourced; an unknown shell is left alone); the real-PTY workflow extended with a folder change
  and a reopen.
- Web: `node:test` for the tree and autosave merge; Playwright with stubbed routes for the list, tabs,
  splits by pointer and by key, each pane state, the close dialog, Type in all panes, and two pages on
  one desk.
- Not checkable here: a real machine reboot. It is checked by killing the tmux server in the real-PTY
  workflow's private socket, never a shared one.

## Left to the builder

File and component boundaries inside `web/src/lib/desks/`; the exact SQL; the debounce and concurrency
numbers above where measurement says otherwise; icon choices; the wording of tooltips within the copy
rules.

## Open points for the owner

1. **Tabs hold panes** was my recommendation; the owner's reply to that question was about navigation.
   The mock-ups, which the owner chose looks from, draw it this way. The alternative is one terminal a
   tab with a layout that tiles tabs.
2. bash and zsh only for per-pane history in the first version.
3. Nine panes a tab, twenty tabs a desk, fifty desks an account.
4. The calls listed under "Calls made after the builder's read".
