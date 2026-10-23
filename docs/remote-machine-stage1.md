# Remote machine stage 1: protocol and durable contracts

This implements stage 1 of the [handoff](../remote-machine-implementation-plan.md),
following the private transport foundation merged in PR #122. The application
still has no terminal endpoints, private bootstrap/runtime, resource collectors,
or host process manager. Existing enrollments remain without capabilities and
cannot reserve terminal sessions. No shell is started by this change.

## Protocol

Use `terminalproto.Encode` and `Decode`; do not marshal `Message` directly.
The envelope contains `version`, `type`, `requestId`, `generation`, `body`, and
the applicable `sessionId`, `attachmentId`, and `attachmentGeneration`.

- Connection generations are controller-assigned, persisted, and monotonically
  advanced by `BeginRemoteConnectionFor` after enrollment authentication. They
  are independent of terminal session IDs and agent process instance IDs.
- Attachment generations fence writers within a connection. The broker must
  assign/check these atomically in stage 6; field validation is not a lease or
  authorization check. Both generations must match before accepting data.
- All IDs are opaque ASCII letters/digits/underscore/hyphen, 16–128 bytes.
  Generate unguessable IDs; syntactic validation does not establish identity.
- Control frames are at most 16 KiB; binary frames at most 32 KiB. Output queues
  must be bounded at 256 KiB per hop in the later stream implementation.
- Titles are nonblank plain text of at most 80 Unicode code points; control and
  formatting characters are rejected. Columns are 2–500, rows 1–300.
- Unknown versions/operations/fields, duplicate fields including case aliases,
  invalid UTF-8, trailing values, excessive nesting, null bodies, nonfinite
  readings, and invalid operation bindings are rejected with a fixed error.
- Binary data carries terminal/history bytes. There is no command/environment
  field or free-form diagnostic/error buffer in the control protocol.

Alongside the handoff's operations, `resize` and `renew_attachment` have explicit
typed bodies. Resize carries dimensions and the current attachment binding.
Renewal carries a 1–60 second lease and may only follow authorization renewal;
it must not attach, create, or take over a writer. Attachment results can echo
the exact session/attachment/generations as well as the matching request ID.
History requests have a maximum of 2,000 lines; byte limits still apply to the
response stream. Complete reconciliation snapshots contain at most 32 unique
session records and must also fit the control frame limit. A request uses
`complete:false` and no records; only a complete response proves absence.

`health_report` includes schema version, process instance, sequence, host sample
time, and host/container scope. CPU uses normalized percent, capacities use
bytes, and uptime uses seconds. A missing reading has `value:null` and a fixed
reason (`initial_sample`, `unsupported`, `permission_denied`, or
`collection_failed`); a present reading cannot also have a missing reason.
Stage 3 must enforce instance/sequence/generation ordering and compute freshness
from controller receipt time. Wire validation alone does not detect replay or
prove a collector's reading is accurate.

## Persistence and caller obligations

The additive, transactional migration runs through `Store.Open` and creates:

| Table | Contract |
| --- | --- |
| `private_transport_identity` | Singleton versioned ciphertext, insert-if-absent; never silently replaced |
| `remote_agent_capabilities` | Immutable enrollment/owner binding, process instance, generation, opt-in/readiness, observation time |
| `terminal_sessions` | Owner/enrollment, title, lifecycle, timestamps, optional exit code |
| `terminal_operations` | Scoped request key, original typed create parameters, session, pending/final outcome |
| `remote_health_summaries` | One bounded controller-built summary per installation/minute, generation and revision |

`SavePrivateIdentityIfAbsent` accepts only an opaque sealed record; it cannot
rotate an existing identity. Stage 2 must first read `PrivateIdentity`, check its
format, and restore it with `privateconn.OpenIdentity` and the existing Sealer.
Create and seal a new identity only when no record exists. A stored record with
an unavailable key or damaged ciphertext is an error, not permission to replace
it. The store checks the ciphertext envelope's size/shape; the transport adapter
authenticates the full payload and its purpose. JSON and formatted records do
not expose the stored ciphertext.

`ReserveTerminalCreateFor` reserves a session and a pending operation in one
transaction. Its key is `(owner, enrollment, kind, requestId)`. Reusing a create
key with different initial parameters returns a conflict. Rename does not alter
the original parameters. Reusing it after restart, a generation change, a final
failure, or a lost acknowledgment returns the original session; it cannot
reserve another slot. The outcome remains available through
`TerminalOperationFor`.

The first statement in mutation transactions acquires SQLite's write lock
before any quota/idempotency read. This also serializes separate Store handles
against the same WAL database. Quotas count `creating`, `running`, and
`end_requested`: eight per enrollment and 32 per account. There is no timeout
that releases an uncertain reservation.

The future broker must authenticate the enrolling browser principal, enforce
the instance feature/auth mode and current enrollment validity, verify a live
private control connection, then reserve and dispatch the returned logical ID.
Stored readiness does not prove connectivity. Never accept an offline create
and queue a surprise shell. Store ownership checks deny other accounts (including
administrators), disabled users, and viewer interactive operations; these checks
do not replace browser-session-only authentication in stage 6.

`UpdateRemoteCapabilitiesFor` refreshes readiness/consent within a live generation
only for its matching process instance. Local disable can therefore be reported
without a reconnect, and a delayed process cannot re-enable access. The runtime
must close existing bridges on disable; a metadata update alone cannot do that.

`RequestTerminalEndFor` works while disconnected or locally disabled. It keeps
quota reserved and the state visibly `end_requested` until a host observation
confirms `ended`, `exited`, or `lost`. Pending ends precede creates; a create for
an end-requested session is excluded from dispatch. A delayed running/create
acknowledgment cannot erase the pending end. An end request never manipulates
tmux from the controller.

`PendingTerminalOperationsFor` is an authenticated agent/reconciliation helper,
not a public API or permission to blindly replay creates. Look up the host's
durable registry and reconcile actual tmux state before dispatching uncertain
work. Missing sessions after reboot must be marked lost, never recreated.

`ObserveTerminalStateFor` requires the current connection generation and enforces
terminal lifecycle transitions. Final states cannot become running again.
Creating may become exited/lost without an observed running event, and an end
can be requested during creation. Confirmed absent/never-created work closes
the create receipt as failed; known running/exited work acknowledges creation.
`FailTerminalCreateFor` accepts a definitive authenticated host rejection and
releases its reservation without permitting a replacement under the same key.
A transport timeout must leave the operation pending for reconciliation.

## Health summaries and cleanup

Stage 3 builds cumulative controller-minute snapshots with min/max/sum and
valid/missing counts per metric. The schema bounds each count to 120, RTT samples
to 64 per minute, and RTT to the five-second probe deadline. RTT percentiles use
the retained samples, never an average of minute percentiles. Input/output are
terminal payload bytes, not synthetic speed tests. Collection, probe matching,
live status, and rate calculations are later runtime responsibilities.

Only the current controller minute can be written. Duplicate identical revisions
are harmless, changed/older revisions conflict, and old connection generations
cannot overwrite new ones. A new generation replaces its current minute segment
instead of adding counters from a dead process. Earlier minute segments remain
historical. This deliberately sacrifices an incomplete minute on reconnect to
bound storage independently of reconnect frequency. No negative restart rate
should be inferred from a replaced segment.

Writes prune summaries older than 24 hours; reads exclude expired records and
return at most 1,441 minute boundaries. The stage-3 scheduler must also call
`PruneRemoteHealth` while idle. There is no background pruning/collection loop
in this stage. Retained history is never proof of live connectivity or freshness.

Composite foreign keys bind enrollment ID and account together. Enrollment
revocation deletes its capabilities, sessions, operations, and health history;
account deletion cascades through its enrollments. A replacement enrollment,
even with the same hostname, starts empty. Inventory cleanup and machine rename
are separate existing operations. The singleton controller identity survives
enrollment/account deletion. No migration or cleanup sends a host command, so
local tmux work is untouched. Later runtime code must close active bridges on
revocation and fence those isolated by partitions with finite leases.

## Verification and next stage

Tests cover every typed operation, malformed/boundary controls, UTF-8 and Unicode
title limits, binary limits, null resource semantics, finite/count bounds, and
lifecycle transitions. The decoder includes a fuzz target with round-trip
invariants. Store tests cover parallel duplicate requests and quotas across
independent database handles, restart/retry, pending ends, stale generations,
definitive failures, account isolation, consent, migration/reopen, ciphertext
immutability/redaction, revocation, account deletion, and bounded health cleanup.

Record exact completed validation in the PR. Stage 2 must wire lazy identity
restoration, authenticated bootstrap, a strictly allowlisted private handler,
connection generations and cleanup. Keep terminal access disabled; host
collectors, services, tmux persistence, browser authentication, and the polished
terminal UI remain stages 3–8 with their existing acceptance gates.
