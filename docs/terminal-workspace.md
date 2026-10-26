# Terminal workspace

The signed-in **Control** page shows active shells and machine connection cards. Skill sync and bulk inventory actions remain on **More → Sync**. Continue
links select the exact connector and session ID on `/terminals`; they never create
a shell. Running means a shell lifecycle state, not AI-agent activity. Creating and
end-requested sessions retain their actual labels.

The account-scoped band polls every 15 seconds while visible, times out requests
after 10 seconds, and labels observations older than 45 seconds as last known.
Transient errors preserve the last list; 401/403 clears it. Viewers do not mount the
band or request terminal data. Viewing the band does not attach to a shell.

The terminal route uses compact app navigation and the remaining usable viewport
(see [Phones](#phones) for how that height is chosen while a keyboard is open).
The session sidebar scrolls independently and can be collapsed with **Sessions**
or its collapse button. Drag its edge on desktop, or focus the splitter and use
Left/Right/Home/End, to resize it between 220 and 480 pixels. Collapse and width
survive reloads in this browser, separately for each signed-in account. Phones
show an overlay sidebar and close it when a session is selected.

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
remains visible on phones; moving from desktop to phone closes the sidebar.

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

**Commands** opens a private account library, also reachable from Control. Search,
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
- `ControlCenter.svelte` mounts `terminal/ActiveSessions.svelte` only for terminal
  account/write access. `ActiveSessions` owns polling and stale/error presentation.
- `terminal/TerminalWorkspace.svelte` owns selection, sidebar, visual-viewport
  sizing, dialogs and existing create/rename/end APIs. Confirmations capture IDs
  and display names at open time. `WorkspaceDialog.svelte` provides native modals.
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
`node web/tests/control.e2e.mjs`, `node web/tests/terminal-workspace.e2e.mjs` and
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
terminal/PTY; neither disconnects, creates a shell or changes saved panel width.


### Quick terminal actions

The + beside a machine starts a new Shell explicitly. Empty machines remain in
the machine groups; unready/disabled or full machines disable creation. An
unconfirmed workspace creation reuses the same request when retried. The control
page has the same + on machine and active-session cards and opens the returned
session. Menus themselves never start or attach a shell.

Right-click a machine or use its ellipsis for terminal actions. Right-click the
terminal screen for saved commands, command history, earlier output, appearance
and clipboard controls; Shift+right-click keeps the browser menu. Control-page
active-session menus also rename or request a stop, with explicit confirmation
and the same remove-after-closure/account preference as the workspace.

When the sidebar is hidden, **Show sessions** is visible in both the workspace
header and the terminal toolbar. In theatre view the toolbar control exits theatre
and restores the sidebar. Its saved width and organisation remain intact.

## Phones

These rules apply to phones: a window 650 px wide or less, or a touch screen held
sideways that is 500 px tall or less (`PHONE_SCREEN` in `terminal/sizing.ts`). The
sideways rules cover the terminal only; dialogs and other pages are unchanged. Desktop
and tablet layouts are unchanged, and `web/tests/terminal-phone.e2e.mjs` checks them
at 1360 and 768 px.

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
