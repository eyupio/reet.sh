# Remote machine foundation review

Reviewed stages 0–4 from freshly fetched main `8953545` (PRs #122–#126),
against the [approved handoff](../remote-machine-implementation-plan.md).
This review covers the delivered transport, protocol/store, private bootstrap,
monitoring, local consent, and service lifecycle. It is not a dependency security
audit or validation of the unimplemented browser terminal.

## Corrected findings

| Finding | Impact and correction |
| --- | --- |
| Agent used an exclusive-create `.lock` marker | SIGKILL left a file that blocked every real restart. Use an owned/private, no-symlink advisory lock; kernel releases it on death. Keep its inode persistent to prevent overlapping owners. Existing safe stale markers are reusable. |
| Native gate ran a placeholder shell loop | Proved service separation but missed the real agent lock failure. Build and run the actual reet agent in both OS jobs, verify it acquired its lock, kill it, and verify the replacement stays up while tmux and the original pane process survive. |
| Minute reset happened after adding the first event | First resource/RTT/traffic/missed-probe observation in a minute was discarded. Roll the bucket before mutation, with regression checks on values/counts. |
| Hub accepted delayed/duplicate connection generations | An older handshake finishing late could replace and close a newer live connection. Reject non-increasing live/retained generations before replacement. |
| Reconnect count reset to one and inferred drops from generations | Count observed replacements/recoveries across retained controller state; a persisted generation alone does not invent an observed drop. |
| SQLite writes ran in the control read loop under the connection mutex | A slow store could stall pong handling and UI status, suggesting false network loss. A cancellation-aware writer coalesces at most one pending cumulative snapshot per connection; status and probes do not wait for SQLite. |
| Reconnect did not join old reporting workers | Old collectors could overlap a new generation and race on its sequence. Cancel, close, and join the generation's three workers before reuse. Bound in-flight collection across reconnects and discard late results. |
| Consent errors preserved the last enabled state | Unreadable/replaced configuration could continue advertising terminal readiness. Read consent before every handshake, report disabled on errors, and fence updates to the same local enrollment. |
| Local tmux end accepted prefix matching | Asking to end an absent short name could kill another session. Use tmux's exact-target syntax and test a real prefix collision. |
| Enrollment reads followed symlinks and did not check file ownership | Apply the existing private regular-file/current-owner checks before loading enrollment data. |
| macOS plist lacked its documented umask | Set launchd's decimal `63` (`0077`), matching Linux and the documented file privacy policy. |
| Retry maximum defaulted to the minimum | Restore the intended 15-second default cap, retaining bounded exponential retries. |
| Readiness spawned tmux twice per poll | Reuse the first version-checked executable. Socket readiness also rejects wrong ownership/open permissions. |
| Linux I/O wait counted as CPU work | Treat I/O wait with idle time when deriving busy utilization. |

Collection timeout produces explicit missing readings; no stale reading is
promoted to a fresh sample. An OS call that cannot be interrupted may remain
outstanding until it completes, but there is at most one per agent collector,
and it cannot hold up cancellation, probes, or a reconnect. History writes are
best effort and bounded; a disconnect may cancel a queued summary. Live and
last-known status still include accepted observations and all missed probes.

## Reported live CLI behavior

The supplied production transcript shows upgrade success, followed by two
successful inventory uploads from `reet agent run`. This command intentionally
runs in the foreground, polling jobs and periodically rescanning. It is not
supposed to return to the shell after an upload. `--once` uploads once and exits;
continuous monitoring requires keeping the agent process or service running.

The CLI now explains foreground behavior, Ctrl+C, the single-run command, and
that it is waiting between cycles. Inventory success alone is not proof of an
authenticated private health connection. No production machine was modified
while investigating the supplied transcript.

## Validation and next stage

Regression tests use actual tmux for exact targets, a SIGKILLed subprocess for
lock recovery, controlled slow collectors/stores, fake controller clocks, and
WebSocket cancellation. The native gate additionally runs the built agent with
a local paused controller fixture through the real user service manager on
Linux and macOS. A fixture-only or cross-compiled run cannot satisfy that gate.
Exact completed checks are recorded in the corrective PR.

Stage 5 follows only after this corrective increment is merged green. Implement
the durable host registry and tmux session manager: persist intent/receipts
before acknowledging, attach only to an existing session, recover the same
process after agent restart, mark missing sessions lost, and keep output only
in bounded memory/tmux history. Use immutable enrollment/session IDs and the
crash-released lock helper. No public terminal route or browser attachment is
authorized merely by a host's readiness report; stages 6–8 still supply those.
