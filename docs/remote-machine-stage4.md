# Remote machine stage 4: local consent and independent services

This implements stage 4 of the [handoff](../remote-machine-implementation-plan.md).
Terminal access is now an explicit local decision, with private paths and
separate per-user agent and tmux services. No shell is created and no terminal
is authorized by the controller in this increment; the host session manager and
broker remain later stages.

Subsequent behavior: [stage 5](remote-machine-stage5.md) adds an enrollment
namespace beneath `terminal/` and preserves a matching running tmux service
during refresh. Its migration notes supersede the unscoped layout and
restart-on-reinstall behavior recorded below. The stronger real-agent native
gate is recorded in the [foundation review](remote-machine-foundation-review.md).

## Local consent

Existing enrollment files have no `terminalEnabled` field and therefore remain
disabled. The web UI cannot switch it on.

```sh
reet agent terminal enable  --config ~/.config/reet/agent.json
reet agent terminal disable --config ~/.config/reet/agent.json
reet agent terminal status  --config ~/.config/reet/agent.json
reet agent terminal sessions
reet agent terminal end --session NAME
```

- `enable` requires the non-root host user that owns the enrollment. It checks
  the Linux/macOS host, a supported tmux (3.2 or newer), the private layout, and
  then writes consent atomically to the private enrollment file.
- `disable` only turns off remote terminal access. Running local tmux sessions
  continue and can be listed or ended locally.
- A running agent polls the consent file and sends a typed `hello` capability
  update over its existing control connection. The controller updates persisted
  readiness and the live health view without changing the connection
  generation. A local disable takes effect for new work; bridge teardown on an
  already attached terminal is the broker/lease stage.
- `sessions` and `end` are local administrative commands. They use explicit
  argv, never a shell, and accept only validated session names.

## Private layout

The default layout is beside the enrollment file:

```text
<config directory>/terminal/
  tmux.conf     reet-owned minimal configuration (0600)
  tmux.sock     private socket, created by tmux
  logs/         per-user service logs (0700)
```

- The directory and logs must be real directories owned by the current user with
  no group/world access. Symlinks, permissive modes, and non-socket entries fail
  closed; they are not silently repaired.
- The socket path must fit the shortest supported Unix limit. A too-long home
  path is rejected with guidance rather than falling back to an exposed socket.
- `tmux.conf` sets a bounded 5,000-line history and disables clipboard access,
  passthrough, focus events, and status output. It never sources a personal
  configuration or plugin.
- The tmux service runs `tmux -D -S <socket> -f <reet config>` in the
  foreground. No session command is appended.

## Services

```sh
reet agent service install   --config ~/.config/reet/agent.json
reet agent service status    --config ~/.config/reet/agent.json
reet agent service uninstall --config ~/.config/reet/agent.json
```

- Linux installs two independent user units: `reet-agent.service` and
  `reet-terminal.service`. The tmux unit is not `PartOf` the agent unit, so
  stopping or killing the agent cannot take tmux or its sessions with it.
- macOS installs two per-user LaunchAgents: `sh.reet.agent` and
  `sh.reet.terminal`.
- Both services use `env -i` with an explicit allowlist: `PATH`, `HOME`, `USER`,
  `LOGNAME`, `SHELL`, `TMPDIR`, `LANG`, `LC_ALL`, `LC_CTYPE`, and `TERM`.
  Agent tokens, controller tokens, backup keys, SSH agents, and other ambient
  credentials are not inherited by either service. The agent reads its
  enrollment file itself.
- Units and plists are private, restart on failure, and apply a `0077` umask.
- Linux installation now uses `enable` plus `restart` so refreshed unit
  definitions, binary paths, and environments are applied. Reinstalling an
  active tmux service therefore restarts it and ends local sessions; the
  command says so. Uninstalling stops both services and therefore ends
  local tmux sessions, while leaving consent and the private directory in place.
- Linux user services stop at logout unless lingering is enabled; `install`
  reports the exact `loginctl enable-linger` guidance. macOS per-user
  LaunchAgents stop at logout.

## Agent capability reporting

- `terminal_enabled` is reported only after local consent and validation.
- Readiness distinguishes `disabled_locally`, `tmux_unavailable`,
  `supervisor_unavailable`, and `ready`. `ready` requires a successful harmless
  tmux client probe, so a leftover socket from an unclean exit is not treated as
  a working supervisor.
- The controller accepts a capability update only for the current enrollment,
  instance, and connection generation. The update does not create, attach, or
  authorize a session.

## Native service gate

`TestNativeAgentKillKeepsTmuxSession` is opt-in with
`REET_NATIVE_SERVICE_TESTS=1`. On Linux and macOS it uses the real user service
manager to:

1. install the independent agent and tmux services;
2. start a real tmux session running a long-lived test process;
3. kill the agent service's main process;
4. wait for the service manager to restart it; and
5. verify tmux, the terminal unit, and the original process are still alive.

The `native services` workflow installs tmux, prepares the Linux user manager,
and runs that test on Ubuntu and macOS. It is intentionally separate from the
local-relay transport tests. If a hosted runner has no usable user service
manager, the job fails rather than claiming the gate from fixture-only evidence.
The gate passes on both hosted platforms: the agent service is killed, the user
manager restarts it, and the tmux unit plus the original test process survive.

Review follow-ups included in this increment:

- Readiness now runs a harmless tmux client probe (`show-options -gv
  exit-empty`), so a stale socket left by an unclean exit is not reported ready.
- tmux 3.2 is the minimum because session creation passes `-e` for `HOME`
  (added in 3.2).
  `allow-passthrough` only exists in 3.3+, so the private config writes it with
  `-q`; older tmux ignores the unknown option and has no passthrough anyway.
- Linux reinstall uses `enable` plus `restart`, so changed units, binary paths,
  and environments are applied; this restarts tmux and ends local sessions.
- macOS uninstall propagates a real `launchctl bootout` failure and ignores only
  the explicit not-loaded case.

## Limits

- No terminal creation, attachment, broker ticket, browser UI, or host session
  registry exists yet.
- Local consent and capability updates are implemented; broker-side teardown of
  an already attached bridge on disable is stage 6.
- The service definitions are generated and installed by the CLI; package
  managers and container images do not install host services.
- Cross-compilation and unit tests are not substitutes for the native
  service-manager gate.
