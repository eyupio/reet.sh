# Remote machine stage 5: durable host session manager

This increment follows the merged [foundation review](remote-machine-foundation-review.md)
and PR #127. It adds the host-side manager and its tmux/PTY/registry primitives.
It does not wire terminal operations into the private control reader, add a
browser endpoint, or authorize a browser terminal. Those remain stages 6–8 of
the [handoff](../remote-machine-implementation-plan.md).

## Host API and ownership

`NewManager` requires an immutable enrollment ID, verified private layout,
absolute enrolled home directory, supported tmux, non-root OS user, and a local
consent callback. The callback must reload the same local enrollment and opt-in;
it cannot come from a controller request. Creation accepts only a typed
`terminalproto.Create` (title/dimensions), request ID, and session ID. There is
no startup-command, arbitrary cwd, or environment field.

Physical tmux names derive from SHA-256 of enrollment ID plus session ID.
Titles are plain display metadata; rename never changes process identity or
becomes a shell command. Only registered sessions can be attached, captured,
ended, or reconciled. Unregistered tmux sessions are never adopted or cleaned up.

Use `NewEnrollmentLayout(configPath, enrollmentID)` for an installed agent.
The new layout is `<config directory>/terminal/<enrollment hash>/`, containing
the private tmux config/socket/logs and registry. A replacement enrollment gets
a different namespace. The registry also binds the complete enrollment ID, so
a path/hash collision cannot adopt another enrollment's metadata. The 100-byte
socket-path limit still fails with guidance instead of an exposed fallback.

The tmux service owns shells. Manager calls and PTY attachments only control
clients of the independently supervised server. `Close`, connection cancellation,
local disable, takeover, and lease expiry close clients without ending sessions.
`End` is the explicit operation that ends a registered session, using exact
tmux target syntax. It does not promise to kill disowned/supervised descendants.

## Registry and retries

`registry.json` is a versioned private file (0600) containing session metadata
and create/end receipts. Writes sync the temporary file, atomically rename it,
then sync the directory before acknowledging. Each transaction uses the
crash-released private file-lock helper and reloads the registry, so separate
processes cannot overwrite each other's quota/receipts using stale copies.
Never unlink the lock file while a process may use it.

- Persist creation intent and its original parameters before `new-session`.
- After creation, reconcile actual tmux state and persist the receipt before
  returning success. The same request ID with changed parameters conflicts.
- A retry after lost acknowledgment finds the same registered session/process.
  A pending intent with no actual session becomes lost; it is never recreated.
- Attaching uses `attach-session`, never `new-session -A` or create-if-missing.
- Creating/running/end-requested states reserve the eight-session host quota.
  Parallel managers share that limit through the file lock.
- End intent is saved first. If tmux is unavailable it stays pending and retains
  quota. Once a responsive server confirms absence, the end is complete.
- Reconciliation uses a responsive server before proving absence. Missing
  sessions become lost; an unavailable socket leaves last-known state intact.
- Invalid, foreign, oversized, symlinked, or permissive registry files fail
  closed. No empty replacement or local-shell termination is used as recovery.

The registry is bounded to 16 MiB, 4,096 session records and 16,384 operation
receipts. Exhaustion refuses additional metadata/actions; it does not erase
receipts and permit a late retry to create another shell. Retention/archival
policy is later rollout work. Local tmux inspection/end remains available.
Timestamps do not move backwards when the host clock is corrected.

`Run(ctx)` reconciles roughly once a second, closes clients on local opt-out,
and cleans up dead panes. Fatal registry errors close attachments and stop the
loop. The caller owns and must wait for this loop. `Changed` signals committed
metadata updates without retaining a queue of events/content: obtain it before
`List` and reconcile the snapshot when it closes. A later broker sends those
metadata observations to the controller.

tmux may expose a dead pane before its exit status is reaped. The manager allows
a later exited observation to fill an unknown code, retaining that pane briefly
before cleanup. A single known exit code is preserved; otherwise the code is
null. Retained dead panes are removed rather than accumulating indefinitely.

## Attachments and history

Attachments use the pinned `github.com/creack/pty` v1.1.24 already in the module
graph. The tmux client has a private PTY and cancellation context. Its descriptor
is nonblocking so closing it interrupts reads/writes and releases process-wait
resources. It receives only the explicit environment allowlist; the tmux server
is also started with the existing clean service environment. Client attachment
cannot copy SSH agents or other ambient variables through tmux's
`update-environment` mechanism, which is disabled for managed sessions.

- One controlling attachment per session, with a private kernel lock also
  preventing another manager process from attaching concurrently.
- A live local attachment needs explicit takeover, a new ID, and a strictly
  increasing attachment generation. It is closed before its replacement starts.
  Another process's attachment is refused rather than killed without authority.
- Host-side leases last at most 60 seconds. Renewal requires the current ID and
  generation and fresh local consent; expired leases cannot be revived.
- Input/output reads use bytes, with 32 KiB frame bounds. No input replay or
  output recording is introduced. The later broker must provide bounded queues,
  current browser authorization, connection-generation fencing and backpressure.
- Resize validates dimensions and affects only the disposable PTY client.
- History is a separate snapshot, at most 2,000 lines and 256 KiB, with empty
  screen padding removed. Oversized captures return an error so the caller can
  request fewer lines. It is never concatenated with streaming output as an
  allegedly atomic replay and is never written into the registry/database.

The initial cwd is the enrolled home, and the host user's normal shell behavior
applies. User shell startup files are separate from tmux configuration: reet
does not load personal tmux plugins or inject startup commands. Metadata paths,
shell input, snapshot bytes, and client diagnostics must not enter controller
logs, analytics, or persistence.

## Service refresh and migration from stage 4

Refreshing an already running, matching terminal service preserves its tmux
process and shells. Only the agent is restarted. tmux configuration changes take
effect on a later explicit local restart. Installation refuses a live service
on another/unavailable socket or an unmanaged live server instead of killing
or silently adopting it. Uninstall still explicitly stops both services and
ends local tmux work.

Stage 4 used an unscoped `<config directory>/terminal/tmux.sock`. The new namespace
does not automatically move/adopt work from it. If that older unit is still
active, installation refuses a silent replacement. Inspect its work locally
with `tmux -S <old socket> list-sessions`; finish/detach that work as appropriate
before explicitly stopping/migrating the old service and installing the new
layout. No old directory or session is automatically deleted.

## Verification and next stage

Real tmux tests cover lost acknowledgments, recovery after SIGKILL of a manager
subprocess with the same shell PID, missing-session non-recreation, concurrent
quota/receipt writes, local consent, corrupt/foreign registry refusal, exit status
and cleanup, binary PTY output, attachment transfer/stale input, lease expiry,
local disable, history bounds, credential filtering and absence of terminal
content from the registry. The native Linux/macOS gate also refreshes the real
services before killing/restarting the real reet agent and verifies the original
pane process survives both events. Exact completed checks are recorded in the PR.

Stage 6 must construct this manager from the validated local enrollment, start
and own its reconciliation loop, and dispatch authenticated/fenced private
operations. On controller reconnect, inspect receipts and actual state before
retrying uncertain work. Reconcile pending ends before attachment, authenticate
the exact reverse-data connection, recheck owner browser authorization before
lease renewal, discard disconnected input, and never give machine/API/MCP
credentials interactive browser access. Host lease primitives alone do not
implement those broker/authentication requirements.
