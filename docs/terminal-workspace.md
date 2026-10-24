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
keep the same workspace behaviour.

## Actions and keyboard

| Control | Effect |
| --- | --- |
| Disconnect | Close this browser attachment; leave the shell running |
| Reconnect | Attach to that existing shell; do not create another |
| New session | Explicitly create a shell; preserve retry IDs on uncertain responses |
| Manage → Rename | Change the title; preserve process identity |
| Manage → End session | Confirm and request termination of the exact shell and its work |
| Recent history | Read a bounded snapshot; do not delete history |

Intentional disconnect stays disconnected through online/visibility events.
Switching machines or sessions preserves the old shell. End requests on unavailable
machines remain pending until confirmed. Remove from history requires the future
persistent archive contract; do not relabel End as Delete.

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

Run `npm --prefix web run check` and `npm --prefix web run build`. Finish the build
before Go checks because Go embeds its output. For production-bundle UI fixtures,
start `npm --prefix web run preview -- --host 127.0.0.1 --port 4173`, then run
`node web/tests/control.e2e.mjs` and `node web/tests/terminal-workspace.e2e.mjs`.
These fixtures cover synthetic two-machine continuation, permissions/errors,
viewport bounds, clipboard and intentional disconnect. They do not prove remote
shell identity. Pair them with the real browser/PTY fixture in
`.github/workflows/terminal-browser.yml`, which checks shell PID continuity,
controller/connector restarts, actual resizing, paste fences and rollout disable.

No production relay, physical phone, macOS or Windows runtime is established by
these local Linux/Chromium fixtures. Further lifecycle edge cases are tracked as
T2 in `handoff.md`; persistent history removal is T3.
