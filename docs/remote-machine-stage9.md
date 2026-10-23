# Stage 9: resilient rollout and acceptance evidence

Remote terminals now require `REET_REMOTE_TERMINALS=on` plus accounts mode and a
canonical HTTPS public origin (loopback HTTP is allowed for development). The
default is off. Both browser terminal work and private terminal data/control
dispatch are gated. Health is independent. Owner metadata remains readable while
off, supporting rollback inspection without deleting or ending local shells.
The UI reports the server-disabled state explicitly. Host-local consent is still
mandatory, including root-owned enrollments as agreed during stage 8.

## Evidence map

| Requirement | Automated evidence |
| --- | --- |
| Real encrypted relay, restored identity, unavailable relay/deadlines | `internal/privateconn` local-DERP HTTP/WebSocket/runtime tests, Linux/macOS gate |
| Duplicate creates, uncertain receipts, quota, durable ends, generation fencing | Store, host-manager and broker tests; real API lifecycle test |
| Browser refresh, long command, agent reconnect | Real Chromium + API + broker + PTY + tmux; original shell and job PIDs checked |
| Controller component restart, flag rollback/re-enable | Browser fixture replaces controller/broker over the same store; original shell/job remain and fresh authorization reconnects |
| Actual agent kill/restart, lifecycle start/stop/status | Native Linux/macOS service gate, independent tmux PID/job preserved |
| Explicit transfer, partitions, revocation, finite leases, slow consumer | Host attachment, broker bridge/ticket and real API integration tests |
| Local disable and tmux loss | Attachment consent test; manager reconciliation marks missing shells lost without recreation |
| Full-screen terminal restoration | Real browser enters alternate screen, refreshes/re-attaches, checks contents, then restores main screen |
| Unicode, input pause, bounded output/history | Browser Unicode check; client queue/stale-socket tests; broker and host bounds tests |
| UI, clipboard and history | Browser search/rename/cancelled end and dangerous paste/history/font/maximise; dark 390px layout and overflow checks |
| Root shells | Root-run real host manager retry/restart and local consent gate on Linux |
| Health accuracy/isolation/retention | Collector/hub/store/API tests for nulls, CPU normalisation, controller receipt time, generations, 24h pruning, account isolation |
| Content persistence | Browser local/session storage assertions; scan SQLite/WAL and private host metadata for output sentinels; host registry/ambient credential test |
| Safe rollback/secure origin | All terminal mutation/history/socket routes reject disabled/insecure origins; metadata and health stay readable |

The browser fixture marks a loopback transport as private for deterministic API
testing; encryption/identity/recovery are independently exercised through real
local DERP. It does not claim one production browser-to-DERP fleet stress test.
Alternate-screen tests do not replace manual vim/top/IME testing on pilot hosts.
Log paths are reviewed for no terminal payload recording; storage assertions do
not prove the configuration of external proxies, analytics or crash reporters.

## Operator handoff

See [the operations runbook](remote-machine-operations.md) for opt-in, host users
and root, background/native lifecycle, relay dependencies, private identity keys,
metrics, pending requests, local recovery, log rotation, updates, backups and safe
rollback versus destructive terminal-service uninstall.

Required validation: web checks/build, full Go vet/race suite, MCP suite, actual
browser/PTY/tmux test, root shell check, native Linux/macOS services and local relay
gates. PR checks record exact platform results. No production deployment, host
upgrade or small-fleet pilot is asserted by this implementation. Keep the flag
off until the operator completes the documented pilot on intended hosts.

The product owner's requested final cross-implementation review follows this
stage and records additional fixes and UI improvements in a separate increment.
