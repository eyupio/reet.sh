# Remote machines: operator runbook

## Enablement and network

Remote terminals ship disabled. Set `REET_AUTH=accounts`, a canonical HTTPS
`REET_PUBLIC_URL` (for example `https://reet.sh`), and
`REET_REMOTE_TERMINALS=on`, then restart the controller. The flag accepts exactly
`on`. HTTP is accepted only for localhost/loopback development. Health and sync
work independently of this flag. Configure and retain the existing private-
connection sealing key (`REET_BACKUP_KEY`, or the instance's protected
`backup.key`); see [stage 2](remote-machine-stage2.md) and
[backup keys](backups.md). Generate a key with `reet backup keygen` only when
initialising an instance; preserve the existing key during upgrades/recovery.

The browser needs no VPN client. It uses same-origin HTTPS/WebSocket to reet.sh.
The controller decrypts browser traffic and bridges it over the authenticated,
encrypted, host-initiated Tailcat connection. This is not browser-to-host end-to-end
encryption: the controller is trusted with terminal bytes in memory. Relays carry
encrypted Tailcat traffic. Do not add public SSH, terminal ports or host fallback
URLs. Outbound relay discovery/connectivity and the configured relay map must be
available. A relay outage pauses remote access; it does not end host tmux shells.

Use the same updated reet binary on controller and hosts. The UI identifies old
agents and missing tmux/supervision. New terminal control is negotiated separately
from health; older health-only agents keep monitoring without gaining shells.

## Host setup and root

Run enrollment and all lifecycle commands as the OS user that should own shells.
Root-owned enrollments are supported. Root requires the same explicit local
consent and private file ownership as any other account; the browser cannot
choose another OS user or escalate an existing enrollment. A project directory
is not a sandbox. Existing enrollments default to terminal access disabled.

Install tmux 3.3 or newer, then:

```sh
reet agent terminal enable
reet agent service install
reet agent status
```

Add `--config /absolute/path/agent.json` consistently for a non-default enrollment.
The agent and tmux run as separate native services; stopping/restarting the agent
must leave the terminal service alone. Non-root Linux services need a functioning
user manager; enable lingering for that OS user if work must survive logout.
Root enrollments use independent system services in `/etc/systemd/system` on
Linux and LaunchDaemons in `/Library/LaunchDaemons` on macOS. They need neither
a root user bus nor a root GUI login. Ordinary users retain their per-user units
and LaunchAgents; no command automatically elevates a non-root enrollment.
Verify active status instead of assuming installation proves connectivity.
Non-root macOS LaunchAgents depend on a logged-in user session. Host reboot and
OS logout that terminates tmux remain outside shell persistence guarantees.
For an earlier root user-service installation, stop the old agent locally and
preserve/finish its tmux work before migrating to system services. Installation
refuses to adopt or kill an existing unmanaged tmux server silently.

`reet agent run` / `start` returns after starting a background process. `status`
queries its private local control socket; `status --remote` also checks the
controller. `stop`, `start`, and `restart` manage the agent, preserving tmux.
Without installed native services the detached fallback survives terminal closure
but has no reboot/crash supervisor. `--foreground` is for explicit supervision
or diagnostics. Stop an older foreground agent once before switching to the new
lifecycle commands. Never infer a running process from a stale PID file.

## Sessions and recovery

Only the account that enrolled the machine can use its terminals. Viewers,
administrators of another account, API/MCP tokens and anonymous users cannot.
Sessions are created explicitly. A refresh, reconnect or restart never silently
creates a replacement shell. Closing the browser or detaching preserves work;
End session requires confirmation and stops the shell and its jobs.

There is one controlling browser per shell. Continue here explicitly transfers
control and fences the old attachment. Disconnected input is discarded. If a
connection fails during a command write, delivery is uncertain: inspect the
shell before repeating the command. Reconnection obtains fresh authorization
and redraws the current tmux screen; recent history is a separate bounded
snapshot rather than a recording/replay service.

Limits are eight active sessions per host and 32 per account. Creating and
pending-end sessions count. An offline end stays pending until acknowledged.
Retry uncertain creates/ends using their original request key; never manufacture
a replacement. Registry capacity errors require inspection, not automatic
deletion. The private host registry is capped at 16 MiB / 4,096 session records /
16,384 operation receipts; archived metadata is not silently pruned.

```sh
reet agent terminal status
reet agent terminal sessions
reet agent terminal end --session EXACT_PHYSICAL_NAME
```

Local names are physical tmux identifiers from the listing, not editable web
titles. For manual recovery, the status command prints the private socket; use
`tmux -S SOCKET list-sessions` / `attach-session -t EXACT_NAME` as its owning OS
user. Keep the enrollment file, private socket/config directory and registry
permissions intact. Do not delete the registry, replace identities or run a
second untracked tmux server to repair an uncertain operation. Reconciliation
marks a genuinely missing shell lost; it does not recreate its processes.

## Health semantics and privacy

Host RTT is a controller-to-host application round trip over the private
connection, not browser ping. CPU is a 0–100 host-wide observation (or explicitly
container-environment observation); the first CPU sample can be unavailable. Memory used derives
from total and available bytes; disk available is on the agent home filesystem.
Uptime is observed host uptime, never extrapolated from agent uptime. Missing or
unsupported counters remain null with a reason. Resource freshness uses controller
receipt time and becomes stale after 90 seconds or disconnect; connection health
and resource freshness are separate. In a container these OS-visible counters
may reflect the host; they are not cgroup/quota accounting. Sync pause affects
jobs, not monitoring. Connection diagnostics expose RTT p50/p95 over recent valid
probes, consecutive missed probes and controller-observed reconnects; those
reconnect counts reset on controller restart. An unreported path stays unknown.

History retains 24 hours of minute aggregates. CPU chart points are valid-sample
means and RTT points are per-minute medians, never averaged percentiles. Gaps are
missing observations, not zero load. Traffic counts terminal payload bytes, not
an estimate of encrypted/framing/network throughput. A container scope is not a
claim about the whole host.

Terminal content is not recorded to the controller database, host registry,
analytics, browser storage or application logs. tmux and the active browser hold
bounded screen/scrollback in memory; explicitly requested history/clipboard and
user screenshots can contain secrets. The controller sees bytes while bridging.
Do not enable WebSocket/body capture in reverse proxies, session-replay analytics
or error-reporting products on these routes. Normal request logs contain path,
status and duration only, with no tickets in URLs. Back up configuration and
metadata according to the existing backup policy; retain the sealing key
separately and securely. SQLite backups do not checkpoint/restore running OS
processes or tmux scrollback. Losing the sealing key is not repaired by rotating
the identity while pretending enrolled hosts are unchanged.

Linux services use journal retention. Rotate macOS service logs and the detached
fallback's private agent log with the host's log management; restart the agent
after rotating a fallback log so its file descriptor reopens. Never rotate by
collecting terminal streams. For production use prefer native supervision.

## Disable, revoke, update and rollback

For immediate server rollback, remove/set off `REET_REMOTE_TERMINALS` and restart
the controller gracefully. Attachments close; no pending terminal commands are
dispatched while disabled. Owner-only metadata and health remain readable.
Surviving local tmux sessions and registries remain intact. Re-enable with the
same database, sealing key and enrollments, then resume existing sessions.

`reet agent terminal disable` removes local remote-terminal consent and closes
attachments; local shells remain. Revoking enrollment cuts private access and
prevents new authorization; it does not terminate independent local tmux work.
Sign-out closes that browser login's attachments. Forgetting an inventory report
is not credential revocation. End work locally if the controller is unavailable.

Update the binary and refresh services without restarting the terminal unit.
Service refresh preserves a matching running tmux server and refuses foreign or
unmanaged servers. New tmux configuration takes effect on an explicit local
terminal-service restart, which ends its shells. `reet agent service uninstall`
stops both services and therefore ends tmux sessions; it is not the rollback
procedure. Enrollment/consent and metadata remain for inspection.

## Pilot and release evidence

Start with a small disposable fleet. Verify the displayed OS user (including root
when intended), real resource readings, relay reachability and original shell/
job PIDs through browser refresh, agent kill, controller restart and rollback.
Check vim/top, phone keyboard/IME and real logout/reboot behavior on the actual
OS/service configuration before wider activation. Stop rollout for unexpected
shell replacement, persistent input, ownership failures or misleading health.

CI evidence and remaining boundaries are recorded in
[stage 9](remote-machine-stage9.md). Automated local-relay tests, component restart
tests and native manager tests complement each other; they are not a claim that
a production fleet has been deployed or piloted.
