# Controller terminal commands

`reet terminal start|stop|restart|status [--data-dir PATH]` replaces manual changes
to `REET_REMOTE_TERMINALS`. It controls server-wide browser terminal availability,
not host agent processes or durable shells. Run as the controller's OS user in
its filesystem namespace (inside the container for Docker deployments).

Terminal commands discover `.env` automatically (or `REET_ENV_FILE` /
`--env-file PATH`). `REET_PUBLIC_URL` or `REET_URL` and `REET_DATA_DIR` are reused;
exported variables and explicit flags win. Saved client URL defaults are a fallback.
The API token is never sent to the local control socket or printed. If a running
accounts controller has no public origin, start/restart applies and persists the
existing configured URL without restarting that process. If no URL is available,
an interactive terminal prompts for it and validates the answer. If accounts
authentication is missing, the guided flow asks for explicit confirmation before
saving it. Cancel leaves setup unchanged; noninteractive invocations fail with a
specific missing-setting message. Saved answers and live controller settings are
reused without repeated prompts. Each local request gets a fresh timeout after
the user answers, so a slow response at the prompt does not expire setup.

The confirmed accounts choice is persisted for the next controller start. An
existing process's authentication mode is not switched underneath live requests;
the command reports "setup saved; controller restart required" rather than
claiming access is active. Stop/status never prompt or downgrade authentication.

`reet serve` also loads `.env` settings, retaining exported/managed overrides.
After upgrading a controller that predates this loader/protocol, restart its
running server process once so the new binary is actually in use; terminal
start/restart thereafter repairs a missing public URL live.

The Machines terminal section includes a copyable command reference and controller
status independent of selected host/readiness or session attachment. It reports
Active, Inactive, Blocked, and unavailable/stale observations honestly, refreshes
every 15 seconds while visible, and offers a manual status refresh. It remains
available with an empty fleet. Host-local consent and live connection health are
separate from controller availability.

The controller owns a crash-released lifetime lock and private Unix socket below
its data directory. There is no public settings endpoint and no browser/API token
is required. The state file is strict, bounded JSON, checked for ownership,
permissions and symlinks, and written by fsync/atomic rename before runtime
changes. Saved state overrides the legacy environment default. Offline start/stop
configure the next process; offline restart fails instead of claiming availability.

Live stop immediately disables admission, clears outstanding tickets and closes
existing broker/control generations. Host tmux sessions are not ended. Live start
reconnects private peers and reconciles existing sessions with fresh authorization.
Restart performs the same fence then enables access. Repeated start/stop is
idempotent. Health/private connections may briefly reconnect; durable host jobs
continue. Accounts mode, HTTPS outside loopback and local terminal consent remain
mandatory; status distinguishes configured-on from blocked availability.

Validation covers live and offline configuration, persistence across controller
instances, idempotency/restart transitions, unsafe state/symlink/socket paths,
failed persistence leaving runtime unchanged, ticket fencing without shell state
changes, CLI parsing/output, and real browser/PTY/tmux stop/start through the Unix
control socket with original shell/job preservation. Web check/build and Go race
checks remain required. See the updated [operator runbook](remote-machine-operations.md).
