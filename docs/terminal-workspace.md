# Terminal workspace

The signed-in **Control** page shows active shells before fleet inventory. Continue
links select the exact connector and session ID on `/terminals`; they never create
a shell. Running means a shell lifecycle state, not AI-agent activity. Creating and
end-requested sessions retain their actual labels.

The account-scoped band polls every 15 seconds while visible, times out requests
after 10 seconds, and labels observations older than 45 seconds as last known.
Transient errors preserve the last list; 401/403 clears it. Viewers do not mount the
band or request terminal data. Viewing the band does not attach to a shell.

The terminal route uses compact app navigation and the remaining visual viewport.
The desktop session sidebar scrolls independently; phones can open it with
**Sessions**. The screen fits when the viewport/sidebar/font changes. **Details &
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

## Actions and keyboard

| Control | Effect |
| --- | --- |
| Disconnect | Close this browser attachment; leave the shell running |
| Reconnect | Attach to that existing shell; do not create another |
| New session | Explicitly create a shell; preserve retry IDs on uncertain responses |
| Manage → Rename | Change the title; preserve process identity |
| Manage → End session | Confirm and request termination of the exact shell and its work |
| Recent history | Read a bounded output snapshot |
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
- `terminal/TerminalPane.svelte` owns xterm fitting, attachment controls, clipboard,
  focus and extra keys. `keyboard.ts` classifies terminal-only shortcuts.
  `connection.ts` retains lease, recovery, generation and input-replay boundaries.
  The pane requests a full xterm refresh after each parsed batch and fit, keeping
  accessibility rows current through full redraws and resize. xterm coalesces these
  requests in its rendering frame. Terminal modes and screen-reader announcements
  remain enabled; no DOM-only copy of remote output is maintained.

Run `npm --prefix web run check` and `npm --prefix web run build`. Finish the build
before Go checks because Go embeds its output. For production-bundle UI fixtures,
start `npm --prefix web run preview -- --host 127.0.0.1 --port 4173`, then run
`node web/tests/control.e2e.mjs`, `node web/tests/terminal-workspace.e2e.mjs` and
`node web/tests/terminal-lifecycle.e2e.mjs`.
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
these local Linux/Chromium fixtures. Modern terminal presentation, themes, account-synced commands and first-machine/
upgrade terminal setup are the next priorities recorded in `handoff.md`.
