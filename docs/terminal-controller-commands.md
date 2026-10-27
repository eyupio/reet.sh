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

On an interactive start/restart, an available controller is followed by guided
local-host setup. It inspects the existing enrollment and prompts for missing
enrollment, explicit shell consent (including root), tmux installation and native
service repair. Use `--agent-config PATH` for a non-default local enrollment.
Declining a host step keeps controller access enabled and states which host work
was deferred. Existing ready installations are verified without repeated consent
questions. API tokens are reused or requested with hidden input; package-manager
children do not inherit REET credentials.

Service installation preflights existing tmux ownership, serializes lifecycle
changes, stops an old standalone agent through its private control socket, and
verifies a native agent and independent terminal supervisor before success. It
never kills tmux as part of handoff. Host setup also validates manager state and
checks private connectivity before claiming completion. A missing/unavailable
service manager or unsafe/unmanaged tmux state remains an explicit error rather
than silently adopting or deleting existing work.

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

## First-class machine connection setup

Run `reet agent terminal setup` in the machine's own interactive OS-user shell.
Use `--config PATH` for a non-default enrollment. This resumable wizard handles
missing enrollment, explicit local shell consent, tmux 3.2+, native service
installation/repair and the private connection. Existing enrollment credentials
and consent are reused. No root shell is enabled without explicit local consent.

Interactive `reet agent enroll` offers this wizard after saving enrollment.
Interactive managed `reet install` and successful `reet upgrade` offer it too,
including an already-current upgrade. Declining or failing an optional step keeps
the completed enrollment/deployment and prints a resume command. Noninteractive
operation, `--yes`, `--skip-terminal-setup`, checks and dry runs never launch new
consent/package-manager prompts. The shell download installer prints the resume
command; it cannot conduct a wizard through piped stdin.

For native/Compose controllers, setup first runs `reet terminal start` in the
controller's namespace with `--skip-host-setup`; then it separately offers the
host's `reet agent terminal setup`. A container is not treated as the host.
A stopped controller remains stopped and gets follow-up instructions instead.
The standalone `--skip-host-setup` flag skips only the host offer after a
controller start/restart; it does not bypass account/public-URL requirements.

A native agent still running a different version triggers service repair/refresh.
Development builds share the `dev` label, so they always offer refresh to load
the installed binary. Refresh asks first and preserves tmux sessions. The wizard
checks local service/manager health and private connectivity; the web machine's
live readiness remains the authority for opening a terminal.

Add machine now has four primary steps: Install, Authenticate (or Restore setup),
Connect terminal, and optional Inventory & sync. The connection step provides
enrollment/setup commands and checks terminal readiness separately from inventory.
A completed scan is never presented as proof that a terminal can connect.

If tmux is missing, check `tmux -V` and the service PATH. Ubuntu/Debian operators
can install it with `sudo apt install tmux`; macOS operators can use
`brew install tmux`. Versions before 3.2 remain unsupported. The wizard offers a
local package-manager install only after confirmation, then verifies the result.
Readiness is not claimed for an unavailable service manager or disconnected peer.

Validation includes real TTY install/upgrade/enrollment prompts and decline/skip
paths, preservation after optional failure, stopped-controller behavior, Compose
namespace separation, actionable tmux diagnostics, local daemon-version checks,
and web readiness independent of scan inventory. Native Linux/macOS CI validates
services; local Linux checks alone do not establish macOS package-manager behavior.

## Bring a local skill into reet privately

Inside the machine's normal shell or reet terminal, use
`reet agent skill import --agent agents --name NAME` for a local file preview.
Repeat with `--upload`, then type `IMPORT` to copy the reviewed files into your
account's private revision store. Use `--exclude RELATIVE_FILE` to omit supporting
files; SKILL.md remains required. This does not publish, install or execute the skill.
The CLI and controller must support this feature and the enrollment must use an
active account personal token. See [machine skill import](machine-skill-import.md)
for project scopes, credential blocking, explicit revision bases and stable retries.

## Sudo in a machine terminal

A machine terminal runs as the enrolled OS user. `sudo` follows that account's
host permissions and password policy; reet does not grant sudo access or bypass
authentication. The native agent and tmux service templates use
`NoNewPrivileges=false` so their shells retain those normal OS capabilities.
Both units need this: agent recovery can also start a tmux server.

Older unit templates set `NoNewPrivileges=true`, causing sudo to report that the
“no new privileges” flag is set. It is inherited and cannot be cleared in an
already running tmux server or its shells. Updating a unit and opening another
session in the same server is insufficient. Service install/status detects a
restricted server on Linux and explains the pending restart.

After installing a build containing this fix, refresh units from a normal
SSH/login shell as the enrolling user:

```sh
reet agent service install
reet agent service status
```

Refresh preserves running tmux work. Finish or safely save that work before a
planned terminal-host restart. For the default **user** service, perform this
from the separate normal SSH/login shell:

```sh
systemctl --user restart reet-terminal.service
```

That restart ends the reet terminal service's sessions and jobs. Use the actual
unit name for a custom service prefix; root enrollments use their system service
instead. Reopen a new terminal afterwards. Keep normal SSH available for package
maintenance until migration is complete. Never kill the machine's shared/default
tmux server to repair this enrollment. A container or parent service that itself
enforces no-new-privileges also needs its own host configuration addressed; a
child unit cannot undo an inherited kernel flag.
