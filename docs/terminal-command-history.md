# Shell command recall

`GET /api/v2/terminal/sessions/{id}/command-history` reads a bounded snapshot from
the selected machine, on explicit user request. It requires the existing account
cookie/write-role/terminal-availability gates, an owned running session and a
ready authenticated peer. It accepts no query, path, shell command or OS-user
parameters. API/MCP tokens, viewers and other accounts cannot read it. The API
rechecks the browser account and session ownership after transfer. Responses use
JSON and `Cache-Control: no-store`.

The snapshot contains `entries: [{command, source, timestamp?}]`,
`sources: [{source, available, reason?}]`, `truncated` and `skipped`. Sources are
`bash` or `zsh`; timestamps are optional Unix milliseconds from saved history.
No timestamp is invented for an unmarked entry. The reader preserves command text
and does not evaluate it, alter history settings, inject `history` into a running
shell, scrape output, or record browser keystrokes.

Only default `.bash_history` and `.zsh_history` in the locally bound enrollment
home are read. Files must be regular, owned by that OS user, single-linked and
not writable by other users. Opens are nonblocking and do not follow symlinks.
Missing or unsupported files return source availability; arbitrary HISTFILE and
ZDOTDIR paths and other shell formats are not followed. Open shells control when
they save history, so current-session entries may not appear immediately.

Bounds: read the last 256 KiB per file; return at most 500 entries, each at most
16 KiB, with a 256 KiB encoded snapshot. Invalid UTF-8, terminal control codes
(except line breaks/tabs), empty and oversized entries are skipped. Bash timestamp
markers and Zsh extended-history prefixes/continuations are understood. History
is a snapshot of the shell's saved format, not a verified execution log.

## Compatible private extension

After authenticating an agent, the HTTP websocket upgrade advertises
`X-Reet-Terminal-Features: command-history-v1`. New agents send optional
`commandHistory:true` only when that exact extension is advertised and they have
the terminal dispatcher. Without it, the JSON field is omitted, preserving the
old strict V1 handshake. Old agents can connect to the new server unchanged.
Consent updates retain the negotiated flag.

The broker checks the current generation and capability before sending the new
`command_history` request. Unsupported peers return an update-required error;
they never receive an unknown operation. The request has only a stream token and
the existing bound session/attachment/generation envelope. The existing bounded
read-only history data stream carries strict JSON separately from live PTY bytes.
There is no version bump, arbitrary exec operation, decoder relaxation, path
parameter or new browser credential mechanism.

## UI and validation

History search/filtering and snapshot contents remain in memory. Paste adds no
extra Enter and keeps existing review for multiline/control text. Run always
requires confirmation showing the command and selected terminal. A generation
change invalidates confirmation; no disconnected input is replayed. A running
foreground application receives terminal input, so users must be at a shell
prompt to run a recalled shell command. Cancelling Run preserves the drawer's
search. Output snapshots remain available even with an older agent.

Tests cover owned-file parsing and unchanged bytes, symlinks/hardlinks/foreign
ownership/permissions, limits and invalid text; strict codecs and old capability
JSON; no new dispatch to legacy peers; account/token/viewer/foreign/offline gates;
search, paste, reviewed rerun, cancellation and output fallback in the browser.
The real two-host fixture reads seeded files, verifies actual command output and
reconnect fencing, and preserves original shell PIDs and background work. Local
Linux/Chromium tests do not establish physical phone or macOS runtime behavior.
