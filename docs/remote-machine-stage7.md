# Remote machine stage 7: browser terminal and health vertical slice

Stage 6 is merged in [PR #129](https://github.com/eyupio/reet.sh/pull/129), including
the background agent lifecycle and the inert history-response correction.
This increment adds the actual terminal inside the reet.sh browser UI.

## Entry points and framework

In Machines, choose **Terminals**, or **Open terminal** on an enrolled machine.
The route is `/machines?view=terminal&agent=<immutable enrollment ID>` with an
optional logical `session` ID. These are metadata IDs, never authentication
tickets or private capabilities. The workspace uses the account browser login;
the generic API helper's persisted API token is deliberately not forwarded.

The terminal uses pinned xterm.js 6.0.0 and fit addon 0.11.0. Search addon 0.16.0
is pinned for the next UX increment. No custom terminal emulator or encryption
is introduced. The workspace and emulator load only on the terminal route;
ordinary application pages keep their previous bundle size. Vite proxies the
WebSocket through the same `/api` target, preserving the browser Origin.

The backend/controller must use accounts mode with the exact `REET_PUBLIC_URL`.
The host must use an updated agent, a non-root terminal user, explicit local
terminal opt-in and the independent tmux service. Root monitoring agents remain
monitoring-only. Eligibility/setup notices explain missing opt-in, old agents,
unavailable services and reconnecting machines without inventing shell readiness.

## Creation, streaming and recovery

Creation is an explicit **New terminal** action. Its request ID is retained
after an uncertain response so retry inspects the same durable operation. Existing
sessions can be selected without creating another shell. The browser preserves
the logical IDs in its route so refresh returns to the original session.

Each attachment obtains a fresh cookie-authorized ticket, opens a same-origin
WebSocket with no URL credentials, and sends the ticket in its first frame.
Only the server's ready message and binary frames are accepted. The browser
resets/redraws the emulator before enabling input. Output remains bytes through
xterm.js, including UTF-8 sequences split between frames.

The display queue is bounded to 256 KiB and frames to 32 KiB. A slow renderer or
oversized frame closes/reconnects for resynchronisation; output is never silently
dropped as though it were complete. Input is accepted only in the live state,
bounded before sending, and never queued/replayed across disconnection. A new
connection epoch fences stale socket events and old renderer callbacks.

Reconnect uses jittered exponential backoff capped at 15 seconds and retries
promptly on browser online/visibility recovery. It reauthorizes the same logical
session; access denial stops retrying. Conflicts require explicit **Continue
here**. **Detach** closes only the attachment and leaves the host shell running.
Final/lost states do not silently create a replacement. In-flight input remains
uncertain, as documented in the protocol.

The component owns and disposes WebSockets, fetch cancellation, timers, terminal
subscriptions, resize/theme observers and browser listeners. Theme changes are
reflected without reconnecting. OSC clipboard writes and active OSC hyperlinks
are disabled. Terminal bytes are not persisted or sent to analytics.

## Health and traffic

The workspace shows controller-to-host application RTT, CPU, memory used,
available home-filesystem disk and host uptime. Reports retain missing readings,
controller receipt time, container scope and explicit last-known/stale semantics.
Missing values are shown as unavailable, never manufactured zeros. Host uptime
is the last observed value rather than an invented clock through disconnects.

Actual binary payload bytes forwarded to/from the host now update the health
hub's terminal traffic counters. Control JSON, tickets and WebSocket framing are
excluded. The hub's bounded summary worker handles persistence; stream readers
do not perform database writes. History captures count only their payload bytes.

## Verification

Run `npm ci`, `npm run check` and `npm run build` in `web`. Node tests cover
redraw-before-input, no disconnected input replay, stale-socket fencing, split
UTF-8 bytes, bounded output and missing/stale resource semantics.

For the real-browser gate, install tmux and Playwright Chromium, build the web
assets, then run as a non-root user:

```sh
REET_BROWSER_TESTS=1 go test -race -count=1 -timeout=3m ./internal/api \
  -run '^TestTerminalPrivateLifecyclePreservesActualShell$' -v
```

`PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` can select an existing Chromium executable.
`REET_BROWSER_ARTIFACT_DIR` enables screenshots. An explicitly requested browser
gate fails instead of skipping when tmux/root constraints are not met.

The test runs the actual web UI, owner authentication, broker, reverse streams,
PTY and tmux. It starts a long-running job, refreshes the browser, restarts the
agent connection, and verifies the exact original shell PID, job and single
logical session survive. It also verifies UTF-8 display, live health, measured
traffic and absence of document overflow. Only the private network transport is
replaced with a local fixture; the independent DERP/native tests exercise real
encrypted Tailcat transport. Fixture-only control routes are never registered
in production. A dedicated CI workflow runs Chromium and saves screenshots.

## Next increments

Stage 8 adds session tabs/management, rename/end confirmation and pending offline
states, bounded history/search/copy/paste/font/maximise controls, mobile modifiers,
broader keyboard/accessibility/theme coverage, and fleet/expanded health views.
Stage 9 owns the full resilience/privacy matrix, operator runbook, release flag,
rollback and pilot. This vertical slice proves durability before that polish.
