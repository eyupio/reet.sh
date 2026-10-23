# Remote machine stage 6: broker, browser authorization, and private streams

Stage 5 is merged in [PR #128](https://github.com/eyupio/reet.sh/pull/128), including
the macOS nonblocking PTY correction. This increment connects the host manager
to the private runtime and adds browser APIs. It does not add the terminal UI;
that remains stage 7, followed by the stage 8 interaction and visual polish.

## Background live connection and CLI lifecycle

`reet agent run` now starts the live connection in the background and returns the
terminal prompt. `reet agent start` is its explicit equivalent. Run these on the
enrolled machine, adding `--config PATH` for a non-default enrollment:

```sh
reet agent start
reet agent status
reet agent restart
reet agent stop
```

Status is local and does not wait for the controller. It shows process, start
time, version, private-control connection state and last successful job/inventory
cycle. `reet agent status --remote` additionally requests controller state. A
running daemon alone is not proof of a connected private channel.

Installed native agent services are controlled through systemd/launchd, leaving
tmux untouched. Native definitions pass `--foreground --supervised` to prevent
nested daemonization. The native test exercises status/stop/start/restart with
the actual binary and verifies the original pane PID survives each action.

Without native units, a detached Linux/macOS child runs in a new process session
with disconnected stdin and a private log. Startup acknowledges only after its
exclusive enrollment lock and private control socket are ready; duplicate starts
do not spawn competing processes. This fallback survives closing the terminal,
but needs native service installation for reboot/crash supervision. Root agents
can use it for monitoring; remote shells still require a non-root host user.

Use `reet agent run --foreground` for debugging or an external supervisor.
`--once` still performs one cycle and exits. Lifecycle commands use an owner-
private Unix socket, never a public port or a possibly reused PID. Older running
binaries without local control must be stopped once through their original
terminal/service. Enrollment, credentials, consent and tmux work are retained.

Sync polling/result uploads honor shutdown cancellation. Stop waits for process
lock release, reporting a timeout instead of false success if other in-flight
work has not finished. Very long config paths fail the portable Unix socket
length check with guidance to move the config directory.

## Deployment and compatibility

Use accounts mode and configure `REET_PUBLIC_URL` to the exact browser origin,
for example `https://reet.sh`. Mutation and WebSocket requests require that exact
Origin, including scheme and port. A development deployment needs its own explicit
canonical origin. There is no wildcard development exception or forwarded-host
trust. Terminal endpoints refuse anonymous and single-token modes; single-token
interactive ownership is deliberately unsupported in this release.

Update the controller before agents. Updated agents advertise the additive
`terminalControl` capability. Older agents continue health reporting to the new
controller, but are never sent terminal operations. Health-only clients do not
receive reconciliation messages they cannot handle. An old controller's strict
decoder will not accept the new capability; rolling back it requires rolling
agents back too. No protocol version or encryption scheme changes are introduced.

The enrolled host must be Linux/macOS, run the agent as a non-root OS user, have
local terminal consent enabled, and have the independently supervised tmux
service installed. The manager reloads and verifies the locally bound enrollment
and consent. Browser/controller requests cannot turn consent on. Root agents can
still report health but cannot start this managed terminal backend.

## Browser contract

Every route requires a current non-viewer account browser login. API, MCP,
bootstrap and machine credentials are insufficient, including when supplied
alongside a browser cookie. An administrator can access only machines they
enrolled, using the same owner-scoped store methods as other accounts.

| Request | Payload / response |
| --- | --- |
| GET `/api/v2/terminal/machines` | Own machines, capabilities, and broker-derived `ready` |
| GET `/api/v2/terminal/sessions` | Own session metadata; active sessions first |
| POST `/api/v2/terminal/sessions` | `{agentId,requestId,title,cols,rows}`; returns the reserved logical session |
| PATCH `/api/v2/terminal/sessions/{id}` | `{title}`; controller display metadata only |
| POST `/api/v2/terminal/sessions/{id}/end` | `{requestId}`; returns current state, possibly `end_requested` while offline |
| POST `/api/v2/terminal/sessions/{id}/attachments` | `{cols,rows,takeover}`; returns `{ticket,expiresIn:30}` |
| GET `/api/v2/terminal/sessions/{id}/history?lines=200` | Plain-text recent snapshot, at most 2,000 lines / 256 KiB |
| GET `/api/v2/terminal/socket` | Cookie-authenticated same-origin WebSocket, followed by ticket authentication |

Use random unique request IDs (16–128 ASCII letters/digits/`_`/`-`) for creates
and ends. Retrying the same create key returns the same session; changing its
parameters conflicts. A timeout is an uncertain outcome: inspect that session
and reuse the key, never generate another create automatically.

There is no HTTP authentication bypass for the socket. Send `{"ticket":"…"}`
as its first text frame within five seconds. Never put a ticket, account token,
or private capability in the URL. Tickets are consumed atomically, expire after
30 seconds, and bind the exact browser login hash, account, session, attachment
operation, dimensions and explicit takeover choice. A replay or different login
cannot reuse them. Outstanding tickets are globally capped at 1,024 and expired
ones are reclaimed when issuing new tickets.

After the host attachment is acknowledged and its reverse stream matched, the
socket sends `{"type":"ready"}`. Subsequent terminal bytes are binary frames of
at most 32 KiB. Resize uses `{"type":"resize","cols":100,"rows":30}` text frames.
No other browser text operation is accepted. The initial ready notification
means the bridge is bound, not that the emulator has applied the redraw. Stage 7
must reset/resynchronise the emulator, apply the initial redraw, and only then
accept input. Never buffer or replay disconnected keystrokes. A policy closure
requires fresh authorization and resynchronisation, not input retransmission.

Normal second attachment requests conflict. An explicit `takeover:true` fences
and closes the old bridge before opening another disposable tmux client.
In-flight input at disconnection/transfer has an uncertain execution outcome.
The immutable logical session and shell process are preserved.

History uses its own authenticated reverse stream and never enters JSON control
messages. Responses are no-store/nosniff with `X-Reet-History-Snapshot: true`.
This is a bounded snapshot, not an atomic replay prepended to a live stream.
Oversized captures fail; request fewer lines. Render as text, never HTML. The
implemented limits supersede the larger earlier illustrative history numbers
in the original API sketch.

## Private binding, recovery and lifecycle

The agent's existing host-initiated Tailcat control connection carries only typed
operations and metadata. Terminal data uses the exact private-only route
`/private/v1/terminal/data`; no public host listener, SSH route or ordinary HTTP
fallback exists. Every reverse connection authenticates the enrollment again,
then sends a bounded `StreamHello` within five seconds. Its random secret, request
ID, enrollment/account, connection generation, session ID, attachment ID and
generation, and attach/history operation must exactly match an outstanding
expectation. The expectation is consumed once and expires after ten seconds.

Control RPCs have ten-second deadlines, at most 32 pending calls per connection,
and one response slot each. Each connection has at most 16 expected streams.
Binary output has an eight-frame / 256 KiB queue and five-second socket write
deadlines. Overflow or a slow consumer closes the attachment; it never silently
drops bytes or records an unbounded output buffer. Socket, PTY, wait and worker
cleanup is joined when a generation stops. The host retains its independent
shells. No terminal bytes, environments, stream secrets, tickets, or login hashes
are persisted or included in new logging.

On reconnect, the broker starts unready and requests a successful host registry /
actual-tmux reconciliation. The snapshot includes all live sessions (at most
eight) and recent final records, bounded to 32 records. Only already-owned
controller records are updated; unknown host records are not adopted. Missing
active records become lost. Uncertain creates are never replayed into new shells.
Pending durable ends are dispatched before marking the connection ready. A new
connection generation closes old bridges, rejects old reverse streams and needs
fresh browser tickets. Host state changes update controller metadata, including
natural exits and cleanup; out-of-order observations cannot regress durable
controller state. Controller-only renames are not overwritten by host titles.

Bridges lease host attachment authority for 15 seconds and attempt renewal every
five seconds. Each renewal rechecks the original browser session, account role,
enrollment, owner-bound session state, local capability, and exact current
connection generation. Revoked browser sessions cannot keep renewing. Known
sign-out and enrollment-revocation actions close bridges and invalidate tickets
immediately; other credential/role/account changes are caught by revalidation.
Control loss closes controller bridges; a partition also leaves the host's
independent finite lease to close its PTY client without killing the shell.
Local opt-out closes host clients through the manager loop.

## Verification and remaining work

Tests cover single-use/expired/login-bound tickets, cross-account/admin/viewer/
API/MCP/anonymous/single-token denial, canonical origins and URL credentials,
exact reverse-stream binding, replaced generations, uncertain-create recovery,
durable offline ends, slow consumers, control partitions and revoked-login
renewal. A real tmux integration test creates a shell through the browser API,
streams bytes, reads history, transfers control, restarts the agent connection
and proves the identical pane PID remains, then dispatches an offline end.

The native Linux/macOS workflow now runs those broker/API integration tests with
tmux installed, in addition to its real user-service restart gate. Exact checks
completed for this increment are recorded in its PR.

Stage 7 must implement the typed browser client and xterm.js workspace, the
health/terminal vertical slice, Vite WebSocket proxy, and a browser integration
test that proves emulator redraw and original-process continuity. Stage 8 adds
the full interaction, accessibility, mobile and theme matrix. Stage 9 still owns
the complete failure matrix, operational privacy assertions, feature-flag rollout
and pilot. A passing API test is not a completed or polished browser terminal.
