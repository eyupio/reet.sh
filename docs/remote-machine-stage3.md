# Remote machine stage 3: private monitoring core

This implements stage 3 of the [handoff](../remote-machine-implementation-plan.md).
The installed agent now bootstraps a private control connection, survives
reconnects, reports host resources, and answers controller probes. The
controller persists bounded health summaries and exposes an account-scoped
health endpoint. No shell, terminal session, tmux service, attachment ticket,
or terminal UI is enabled by this increment.

## Agent runtime

`reet agent run` starts the private monitoring loop in its own goroutine. A slow
scan or sync job cannot delay bootstrap, probes, or report processing. `--once`
does not start monitoring.

- The agent asks `POST /api/v2/sync/private` for the controller capability using
  the existing controller token and enrollment credentials. Redirects are
  rejected.
- It creates a fresh `privateconn` client per connection generation, opens
  `GET /private/v1/control`, and sends a typed handshake as the first text
  frame. The controller assigns the generation in the reply.
- The agent keeps its process-instance ID and health sequence across reconnects.
  A new process gets a new instance ID and sequence; the controller accepts the
  fresh instance without manufacturing history.
- Control probes are answered immediately by a dedicated read loop. Resource
  collection runs on a separate ticker with a bounded context, so a slow or
  failing collector cannot block pong processing.
- Reconnect uses exponential backoff with jitter, capped at 15 seconds. Each
  attempt bootstraps again and creates a new native Tailcat client.

## Collectors

`internal/agenthealth` uses `github.com/shirou/gopsutil/v4` (BSD-3-Clause,
pinned at v4.26.9) rather than parsing human-readable shell output. Readings are
read-only and do not require root.

| Reading | Unit and source |
| --- | --- |
| CPU | Host-wide busy percentage, 0–100, from the delta of aggregate CPU times across cores |
| Memory | Total and available bytes |
| Disk | Total and available bytes on the agent user's home filesystem (`Bavail` on Unix) |
| Host uptime | Seconds since OS boot |
| Sampled at | Host Unix milliseconds for diagnostics only |
| Scope | `host`, or `container` when a container marker is present |

The first CPU reading is `initial_sample`; it never guesses an instantaneous
value. Unsupported, permission-denied, failed, or timed-out readings are null
with a fixed reason. A failed collector cannot reuse the previous value as a new
observation.

## Controller hub

- Each connection generation has at most one live control connection. A newer
  handshake fences the previous connection and receives the next persisted
  generation.
- Health reports are accepted only for the current generation and process
  instance, in strictly increasing sequence order. Duplicate and out-of-order
  reports are ignored.
- Probes carry a sequence, nonce, and generation. Each matching result is
  accepted once. RTT uses the controller's monotonic receipt time; host
  timestamps are never subtracted.
- Probes run every 30 seconds with jitter, plus one immediately after the
  handshake. The initial probe timeout is 5 seconds. Three consecutive missed
  probes close the connection; a result resets the count.
- Freshness is the controller receipt age of the last resource report. More than
  90 seconds shows a stale report even while the connection is live.
- Rolling RTT samples are bounded. `rttP50Millis` and `rttP95Millis` are computed
  from retained samples; `rttVariabilityMillis` is the p95 minus p50. No RTT
  value is labelled raw network latency or browser end-to-end latency.
- The observed path is `unknown`; this increment does not infer direct/relayed
  transport from latency.
- Minute summaries keep valid/missing counts, min/max/sum per resource metric,
  up to 64 RTT samples, missed probes, reconnects, and terminal traffic counters.
  Percentiles are never averaged across minutes.
- A new connection generation replaces the current minute segment instead of
  adding counters from a dead process. Input/output counters are zero until
  terminal streams exist, so no negative restart rate can be inferred.
- Stored summaries are pruned after 24 hours, including a periodic prune while
  the controller is otherwise idle.

## Health API

`GET /api/v2/machines/health` returns the enrolling account's machines only. It
requires an authenticated instance; `REET_AUTH=none` refuses it.

- `serverNow` is controller time in Unix milliseconds; UI ageing must use it
  rather than the browser clock.
- Each machine contains its controller-assigned state (`never_connected`,
  `connected`, `unreachable`, or `unknown`), capabilities, observation time, and
  a `connection` object with the live or last-known status.
- `connection.report` is the latest resource report, with `reportFresh` computed
  from controller receipt time. `connection.rttMillis` is the latest accepted
  sample. Retained history is never proof of live connectivity.
- `?agent=<id>` adds bounded stored history for that machine, with per-minute
  p50/p95 RTT values derived from the retained samples.
- Cross-account and unknown machine requests do not expose other accounts'
  data.

## Working directory safety

The agent no longer changes the process working directory for scans, previews,
applies, or restore rescans. Scans take an explicit directory, sync uses its
explicit project directory for planning and rescanning, and `npx` receives the
directory through `cmd.Dir`. This removes the process-global hazard before
concurrent collectors and future terminal work.

## Dependency and validation

The added gopsutil module declares Go 1.26 and its transitive platform helpers
are already compatible with the Go 1.27.1 minimum. With the generated UI,
CGO disabled, `-trimpath`, `-buildvcs=false`, and `-ldflags='-s -w'`, the Linux
amd64 binary increased by about 254 KiB (39,284,896 to 39,538,848 bytes). The
module graph and licences were reviewed for this addition; this is not a
security audit.

Focused tests cover CPU interval normalisation and clamping, percentile
computation without reordering samples, bounded/minute aggregates, null
semantics, collector output, bootstrap headers and redirect refusal, capability
scope, duplicate/out-of-order reports, generation fencing, probe matching,
three consecutive missed probes, freshness with a fake clock, account isolation,
retention/pruning calls, working-directory preservation, and the full private
control-to-health-API path. The full repository web, MCP, vet, and race suites
pass; cross-compilation succeeds for Linux and macOS. Native macOS runtime
collection and service-manager gates remain later work.

Next: stage 4 adds local terminal consent, agent commands, and independent
agent/tmux service lifecycles. Terminal creation stays disabled until the host
session manager, broker, and browser authorization stages pass.
