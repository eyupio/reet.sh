# Terminal interface

Status: read-only screens, Scan and Sync shipped; Backup, Setup and Upgrade follow.

The `reet` binary has a terminal interface built with
[Bubble Tea](https://github.com/charmbracelet/bubbletea). It is a face on the
commands the binary already has, not a second control room: the browser app keeps
Home, Sessions, Machines, Skills and Sync, and the terminal interface reads the
local deployment and the controllers those commands already use.

## Entry points

| Invocation | What opens |
| --- | --- |
| `reet` in a terminal | The menu |
| `reet doctor --tui` | Doctor |
| `reet healthcheck --tui [--url URL \| --local]` | Healthcheck |
| `reet scan --tui [--server URL] [--env NAME] [--dry-run]` | Scan |
| `reet sync --tui [--server URL] [--target ID] [--apply]` | Sync |
| `reet version --tui` | Version |
| `reet terminal status --tui [--data-dir PATH]` | Terminal status |
| `reet agent status --tui [--config PATH]` | Agent status |

`--tui` is removed before the existing command flags are parsed, so the text
commands keep their parser and their output. `REET_NO_TUI=1` keeps a bare `reet`
on its old path (`serve`). Redirected input or output, `TERM=dumb`, containers
and services never start the interface; they run exactly as before. `serve`,
`install` and `upgrade` are not changed by this slice.

## Screens

- **Doctor.** Every check from `reet doctor`, grouped by status, with each
  check's advice. The report is the same value the text command renders.
- **Healthcheck.** The endpoint, HTTP answer, latency and time, or the request
  error. `--url` and `--local` behave as they do on the command.
- **Scan.** A form for environment, controller, token and extra folders, with
  dry-run, prune and reconcile switches. Submitting collects the installed
  skills and, unless it is a dry run, reports them; the result shows every
  finding and the controller's counts, skips and errors. `r` runs the same form
  again, `e` edits it and `esc` returns to the menu.
- **Sync.** A form for target, controller, token, environment and project
  directory. It previews first and shows every install, replace and remove;
  `a` applies exactly that plan (the hash is checked again before any file
  changes), `r` previews again and `e` edits the form. `--apply` goes straight
  to the apply step after the preview. Apply still requires this machine to be
  enrolled, as the text command does.
- **Version.** Binary version, source commit, Go version and platform.
- **Terminal status.** Whether terminal access is enabled, available and
  running, the saved public URL and authentication mode, and why it is blocked.
- **Agent status.** The private enrollment path and, when enrolled, the local
  daemon's PID, service mode, private connection, start time, version and last
  inventory cycle.

Keys: `↑`/`↓` or `j`/`k` move, `enter` opens, `esc` returns, `r` reloads,
`q` quits. A viewport scrolls long reports; the layout follows the terminal size.

## Shape

- `internal/tui` holds the model, views and theme. The model is pure: it returns
  commands for work and receives results as messages, so transitions and views
  are tested without a terminal.
- Data comes from the packages the commands already use:
  `lifecycle.DoctorReport`, `health.Check`, `terminalcontrol.Command` and
  `scan.ReadAgentStatus`. No screen parses command output and no screen writes to
  the store.
- The text commands and the terminal screens share report values. Doctor's
  `Report`/`Diagnostic` and the agent's `AgentStatus` are exported for that
  reason; their text renderers remain in the command packages.
- Forms use `huh` with the same model, so a form is a screen like any other:
  `esc` cancels back to the menu and `ctrl+c` quits.
- Colours use the reet orange with adaptive light and dark values. `NO_COLOR`
  and `TERM=dumb` use plain styles. `REET_NO_ANIMATION` is respected by the text
  commands' spinner; the terminal interface's spinner is part of its loading
  state.

## Dependencies

Bubble Tea, Bubbles and Lip Gloss are MIT-licensed, pure Go and already the
smallest well-tested way to get keyboard handling, a viewport and adaptive
colour. A stripped hello-world grows from 1.51 MB to 3.57 MB with these three
packages on Go 1.27. `huh` supplies the forms. The project keeps
"one Go binary, one SQLite file"; this is a deliberate exception, not a new
runtime.

## Packaging

The interface is compiled into the same single binary as the server and the CLI.
It has no separate executable, asset directory or runtime to install. Release and
container builds already set `CGO_ENABLED=0`, so the binary is statically linked
and carries every terminal-interface dependency; verify with
`CGO_ENABLED=0 go build ./cmd/reet && file reet` (statically linked) and `ldd reet`
(not a dynamic executable). The embedded web UI remains in the same binary for
`reet serve`; the terminal interface does not load it.

## Limits

Bubble Tea owns one program's screen and input. It cannot host OpenCode, Codex or
Claude inside a pane without a PTY and terminal-emulator stack. Live agent
terminals stay in the browser and tmux; the terminal interface is the control
plane and, in later slices, a launcher.

## Next slices

1. Backup: take, verify and restore with passphrase handling.
2. Setup: create, push and pull a setup profile.
3. Upgrade: check, confirm, back up, verify and roll back on a failed restart.
