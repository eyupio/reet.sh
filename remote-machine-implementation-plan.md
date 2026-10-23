# Remote machine implementation plan

Status: implementation handoff; no runtime functionality is implemented by this document.

Decisions agreed with the product owner on 23 October 2026. Implement this plan in
small, reviewable stages. Read the current repository instructions before each
stage. This document consolidates the terminal, private connection, reconnection,
UI, and host monitoring requirements; it supersedes the earlier chat drafts.

## 1. Outcome and approved scope

A person signs in to reet.sh, opens Machines, and opens a terminal on a machine
they enrolled. Everything happens inside the reet.sh web UI. They do not install
Tailscale, Tailcat, an SSH client, or another connection application on their own
computer.

The installed reet binary maintains a private connection to the controller,
reports connection quality and basic host resources, and optionally provides
interactive terminals. Running commands survive browser disconnection and agent
or controller restart, provided the host and its independent tmux service remain
running.

| Decision | Approved v1 behaviour |
| --- | --- |
| Host platforms | Linux and macOS |
| Browser experience | Terminal embedded in the existing reet.sh web UI |
| Private transport | Embedded Tailcat, following the Zoomies private-agent pattern |
| Browser prerequisites | Ordinary supported browser; no local VPN or tunnel application |
| Terminal authorization | Only the account that enrolled the machine |
| Host consent | Terminal access explicitly enabled locally; disabled for existing enrollments |
| Disconnected sessions | Keep running until explicitly ended or the shell exits |
| Concurrent control | One controlling attachment per session; explicit transfer |
| Monitoring | Connection quality plus CPU, memory, host uptime, and free disk |
| Monitoring privileges | Read-only, without root; independent of terminal authorization |
| Recording | No persistent terminal content, command recording, or session replay analytics |

A terminal has the permissions of the OS user running the host agent. Starting
in an enrolled project directory is a convenience, not a sandbox. Do not introduce
automatic privilege escalation. The initial supported terminal setup should use
a non-root host account.

Do not expand v1 into shared operators, collaborative input, file transfer,
browser split panes, Windows/ConPTY, OS tuning, arbitrary port forwarding, or
AI/MCP access to terminals. Those require separate designs and product decisions.

## 2. Evidence and repository entry points

Source inspection used reet.sh commit
d12e627a03386802340da20a99c325c94a7fa304. Before this documentation PR, main advanced
to 957f60159675352454cc27e5767f64aa03dca993 with two additional commits changing
internal/scan/scan.go and its tests. Re-read current code before implementation;
do not assume line numbers or this inventory remain current.

The application already has Go, SQLite, an embedded Svelte 5 UI, account-scoped
machine enrollment, and typed sync jobs. The inspected code has no interactive
terminal or Tailcat integration.

| Existing area | Entry points and relevance |
| --- | --- |
| Server assembly | cmd/reet/main.go: API, public URL, backup sealer, shutdown |
| API authentication | internal/api/api.go: account/token/none modes and role guards |
| Agent APIs | internal/api/sync_control.go: enrollment and installation authentication |
| Installed agent | internal/scan/agent.go: local identity binding, credentials, polling |
| Persistent enrollment | internal/store/store_sync_control.go and internal/store/store.go |
| Machine identity | internal/store/store_machines.go: aliases and merge behaviour |
| Encryption helpers | internal/sealed/archive.go: existing SealSecret/OpenSecret |
| Service management | internal/lifecycle/service.go: existing server units, not agent supervision |
| Existing health check | cmd/reet/health.go: server /healthz and container check |
| Existing diagnostics | internal/lifecycle/doctor.go |
| Routing and UI | web/src/App.svelte, Machines.svelte, MachineFleet.svelte |
| Browser API helper | web/src/lib/api.ts |
| Design and copy | web/src/app.css and docs/voice-and-copy.md |
| Development proxy | web/vite.config.ts: add WebSocket proxy support |
| Build/release | go.mod, go.sum, Dockerfile, .github/workflows/release.yml |

Current agent last-seen reporting is not a latency measurement or terminal
readiness check. A long sync operation can delay a sequential polling loop.
The new private connection and monitoring loops must run independently.

The agent currently changes the process working directory during scans.
Background collectors and terminal work must use explicit absolute paths and
command-specific working directories. Remove the process-global dependency or
isolate scan execution before introducing unsafe concurrency.

## 3. Architecture and framework choice

Retain the existing Go binary, SQLite store, and Svelte application. Use mature
libraries for terminal emulation and WebSocket/PTY handling.

| Component | Selection | Responsibility |
| --- | --- | --- |
| Application framework | Existing Svelte 5 | Navigation, interaction, session management |
| Browser terminal | @xterm/xterm with fit and search addons | Terminal emulation and rendering |
| Browser/agent streaming | github.com/coder/websocket | WebSocket handling and cancellation |
| Private connection | github.com/tailscale/tailcat | Encrypted connectivity and relay fallback |
| Local attachment | github.com/creack/pty | PTY for a tmux attachment client |
| Durable process owner | Independent tmux service | Shells, programs, current screen, bounded history |
| Durable metadata | Existing SQLite | Ownership, operations, capabilities, health summaries |

~~~mermaid
flowchart LR
    B["reet.sh browser UI<br/>Svelte + xterm.js"]
    C["reet.sh controller<br/>Authorization + broker + health"]
    A["Installed reet agent<br/>Private connection + monitoring"]
    T["Independent tmux service<br/>Durable shell sessions"]
    B <-->|HTTPS and browser WebSocket| C
    A <-->|Host-initiated encrypted Tailcat connections| C
    A <-->|Local PTY attachment| T
~~~

Use one persistent private control connection per installation and a separate
reverse data connection per active terminal attachment. The host initiates both.
Avoid writing a general-purpose byte multiplexer.

Monitoring uses the control connection. Terminal output has separate streams,
so large output cannot directly queue ahead of health messages.

Evaluate ttyd only as a reference during the compatibility prototype. Direct
xterm.js integration gives reet.sh control over its permissions, session UX,
and recovery without embedding another application's UI. Do not introduce a
second application framework or a separate identity platform.

### 3.1 Dependency gate

The inspected Zoomies source at b6b549d308b698a5452a403bb3578b6c0b8f7cc4 pins
Tailcat v0.7.0. That Tailcat tag declares Go 1.27.1; the inspected reet.sh checkout
declares Go 1.26.0. Verify the actual available release/toolchain before changing
dependencies. If using that pin, update go.mod, build images, CI references, and
development documentation together.

Record exact tested versions and commit lockfiles. Review Tailcat's indirect
dependency graph, supported architectures, binary-size change, licences, and
security advisories. Do not silently choose an older release to bypass a failed
build. Do not copy an entire Zoomies dependency graph into reet.sh.

Tailcat's API and wire format are not guaranteed stable. Keep it behind a small
adapter and test mixed agent/controller versions. Unsupported protocol versions
must fail with update guidance, not attempt an unsafe fallback.

### 3.2 Privacy and operational boundary

The browser's connection terminates at reet.sh. The controller can read terminal
input and output. Tailcat encrypts controller-to-host traffic; this design does
not provide browser-to-host encryption that hides content from the controller.

No public terminal port, TUN device, root network configuration, Tailscale account,
or user-device VPN is required by this design. Both controller and host still
need outbound connectivity to the required network infrastructure.

Treat Tailcat addresses as sensitive capabilities, not as application
authorization. Continue checking enrollment credentials and account ownership.

Public Tailcat relays are rate-limited and best effort. Make relay configuration
an adapter concern, document the selected deployment's dependencies, and test
with a local relay. Do not promise availability or throughput from a public
relay. Upstream's experimental threat model makes the private-handler allowlist,
application authentication, and isolation release requirements.

## 4. Identity, consent, and authentication

1. Bind every managed session to the immutable enrolled agent ID and owning
   reet account. Host display names, aliases, inventory IDs, and IPs are not
   authorization identities.
2. Add a local terminal opt-in to agent configuration. Existing configurations
   default to false. The web UI cannot switch it on.
3. Private health reporting is a separate capability. A person can have host
   monitoring without enabling remote shells.
4. Preserve local hostname/home binding and private configuration permissions.
   Do not copy credentials to another installation.
5. In accounts mode, interactive terminal operations require the enrolling
   account's browser session. An instance administrator has no implied right to
   another person's shell. Viewers, team/list membership, machine API tokens,
   bootstrap credentials, and MCP credentials grant no interactive access.
6. If authenticated single-token mode is supported, implement an explicit
   instance-owner path with dedicated tests. Never inherit the generic default
   owner from unauthenticated mode. Terminals are disabled with REET_AUTH=none.
7. Use existing enrollment authentication to bootstrap the agent's private
   address over HTTPS. Never return that address in browser machine responses.
8. Attribute private transport from the listener context, not an agent JSON
   field or a supplied proxy header.

Browser attachment flow:

- An authenticated POST authorizes one session and returns a random, single-use
  ticket, valid for at most 30 seconds.
- The browser upgrades a same-origin WebSocket and sends the ticket in its first
  message within 5 seconds. Send no ticket, account token, or capability in URLs.
- Consume the ticket atomically and recheck the principal before accepting data.
- Bind it to account, session, attachment, authorization source, and operation.
- Validate Origin against the configured canonical origin. Development origins
  must be explicit, not wildcard production exceptions.
- Any bypass of existing HTTP authentication is scoped to the exact socket
  handshake handler, which performs this authentication itself. Other API paths
  remain protected.

Use a dedicated agent handler over Tailcat. Accept only known private routes,
methods, and agent credentials. It must not expose the public UI, admin API,
SSH, Docker sockets, arbitrary targets, or the LAN.

## 5. Process lifetime, supervision, and recovery

The tmux server owns shells. A browser or agent attachment is a disposable tmux
client. Disconnecting, timing out, or cancelling an attachment must close only
that client and its PTY.

Run a dedicated tmux server per enrollment, with an installation-specific socket
in a private directory and a minimal reet-owned configuration. Verify ownership,
mode, symlinks, and socket-path length. Refuse an unsafe path; do not fall back to
an exposed socket. Do not load personal tmux plugins or configurations.

A foreground service invocation can use:

~~~text
tmux -D -S <private-socket> -f <reet-owned-config>
~~~

With -D, do not append a tmux command. Verify this against the supported tmux
version during the dependency gate.

Install the agent and tmux under separate service lifecycles:

- Linux: separate user systemd units; tmux must not be PartOf the agent or be
  killed as part of the agent's control group.
- macOS: separate per-user LaunchAgents; verify real launchd restart behaviour.
- Explain Linux lingering requirements and macOS logout limitations.
- Existing reet server installation commands are not the host agent service.
- Do not rely on daemonisation alone to escape supervisor cleanup.
- Uninstall/disable must explain what happens to already running local sessions.

Use explicit argv through os/exec. Session labels and paths must not become
shell command strings. Create sessions with validated opaque names, measured
dimensions, and an allowed initial working directory. Use the normal host
user's shell without injecting arbitrary startup commands.

The tmux service environment must not inherit agent tokens or controller
credentials. Preserve necessary user, locale, PATH, and terminal variables
through an explicit policy. This prevents accidental inheritance; it does not
make a same-user host shell a credential sandbox.

Set a bounded history limit. Detect dead panes, capture exit information where
available, persist the result, notify clients, and clean up finished managed
sessions. Do not let retained dead panes accumulate indefinitely.

| Event | Required result |
| --- | --- |
| Browser closes or refreshes | Shell remains available for later attachment |
| Browser network drops | Input disabled; reconnect to the same session |
| Controller restarts | Same private identity; reconcile with the agent |
| Agent restarts or is killed | Independently supervised tmux survives |
| Host unreachable | Retain last-known session metadata; show uncertainty |
| Host reboots or tmux dies | Mark missing sessions lost after reconciliation |
| Disconnect action | Detach only |
| End session action | End managed session; confirm only after host acknowledgment |
| Enrollment/access revoked | Close remote access; preserve local work by default |

Ending a terminal closes its managed shell/session. Do not claim it forcibly
terminates every possible disowned or independently supervised descendant.

## 6. Durable records and idempotency

Use the existing migration mechanism. Proposed records:

| Record | Minimum fields |
| --- | --- |
| Private transport identity | singleton ID, format version, encrypted identity |
| Agent capability | agent ID, supported protocol, local opt-in, OS, OS user, observation time |
| Session | ID, account ID, agent ID, title, state, timestamps, exit information if known |
| Operation | request ID, account/agent/session binding, kind, state, result |
| Health summary | account/agent ID, minute bucket, valid sample counts, bounded aggregates |

Use the existing sealing facility for the complete persistent Tailcat identity,
including private material and capability secrets. Give the payload a purpose
and version. Fail closed if its key is unavailable or decryption fails. Never
silently regenerate an identity and strand enrolled agents.

Session states:

~~~text
creating -> running -> end_requested -> ended
                   -> exited
                   -> lost
~~~

Connection state and terminal readiness are separate from session state.

- Generate a session ID and persist its operation before requesting host creation.
- Use an idempotency key scoped to owner, installation, and operation.
- Repeated creation requests, including after a lost acknowledgment, must return
  the same session rather than start another shell.
- The host persists its local registry atomically before acknowledging operations.
- Recover uncertain operations by checking that registry and actual tmux state.
- Reattach only to an existing session. Never use create-if-missing attachment.
- Reserve quota transactionally; creating and end_requested consume quota.
- Reject new creation while the host is unreachable. Do not queue a surprise
  shell for a machine that returns days later.
- End requests may be durable while offline, but remain visibly pending.
- Reconcile pending ends before authorizing later attachment to those sessions.

Initial limits: 8 live sessions per installation and 32 per account. Counts
include creating, running, and end_requested, including detached sessions.

Define enrollment revocation, account deletion, and foreign-key behaviour
explicitly. Revoked metadata may be retained as inaccessible history or removed,
but must never authorize access. Local tmux sessions may remain for local
inspection. A replacement enrollment never adopts them automatically.

Machine rename should preserve sessions through immutable IDs. Forgetting
inventory reports must not silently imply that enrollment or terminal access
was revoked. Update confirmation text accordingly.

Do not store terminal contents, typed input, command history, or full environments
in these records.

## 7. Wire protocol and broker

Create a versioned internal/terminalproto package with typed messages.

Control operations:

~~~text
hello
reconcile
create
attach
detach
end
history
probe
probe_result
health_report
result
session_changed
~~~

Every request has a protocol version and request ID. Session operations include
a session ID; attachments include a unique attachment ID and generation.
Health/probe messages include their own sequence and connection generation.

| Limit | Initial value |
| --- | --- |
| Control message | 16 KiB |
| Terminal binary data frame | 32 KiB |
| Pending output per attachment, per hop | 256 KiB |
| Session title | 80 characters, plain text |
| Columns | 2 to 500 |
| Rows | 1 to 300 |
| Attachment authentication deadline | 5 seconds |
| Unused browser ticket lifetime | At most 30 seconds |
| Attachment authorization lease | At most 60 seconds without renewal |

Reject malformed IDs, unknown operations/versions, invalid lengths, and oversized
messages. Use binary data frames for terminal bytes. Feed output bytes to
xterm.js without independently decoding each frame: UTF-8 sequences can span
frames. Handle xterm's binary-input events as bytes where required.

The control connection asks the agent to open an attachment. The agent initiates
a separate authenticated private data connection, which the broker matches to
the exact expected agent, session, request, and attachment generation. A claimed
ID alone is not authentication.

Only one attachment controls a session. A second browser sees a conflict and can
explicitly select Continue here. Transfer atomically fences the previous writer.
Old sockets and delayed messages must not become valid again after a transfer.

Use bounded queues, cancellation, and write deadlines. A slow consumer should
lose its attachment with a resynchronisation explanation rather than silently
lose output or block host processes indefinitely. Ensure all socket, PTY,
process-wait, observer, and goroutine resources are released.

Keep long-lived streams out of the existing 30-second agent HTTP client timeout
and bounded sync-job execution contexts.

## 8. Connection recovery and authorization lifetime

Browser connection states:

~~~text
connecting -> resynchronising -> live
live -> reconnecting -> resynchronising -> live
live -> detached
live -> taken_over
any -> ended / lost / access_denied
~~~

- Retry transient failures with exponential backoff and jitter, capped at about
  15 seconds. Retry promptly on online/visibility recovery.
- Disable and discard new terminal input while disconnected. Never queue,
  replay, or automatically retransmit keystrokes after a drop.
- Preserve the visible terminal during interruption.
- Obtain fresh attachment authorization after reconnect.
- Reconcile the same session and reset/redraw the emulator before re-enabling
  input. Do not create a replacement shell when reconciliation fails.
- Input in flight during failure has an uncertain execution outcome; never
  promise exactly-once command execution.
- Expired credentials or revoked access stop automatic attachment retries until
  authorization is restored.
- Renew attachment leases only after current authorization succeeds. Close
  browser and host bridges when a lease expires, even if a revocation event
  could not cross a network partition.
- Recheck account/session/token/enrollment validity periodically and on known
  revocation events. Local disable immediately closes host attachments.
- A stopped bridge detaches from tmux; it does not kill the shell.

Model recovery after controller restart explicitly: its in-memory tickets and
connection registry are gone, while durable operations and host sessions remain.
Reconcile before advertising readiness.

Follow the inspected Zoomies fresh peer-introduction/reannouncement pattern
when appropriate to the pinned Tailcat API. Do not assume a reused idle virtual
connection detects a restarted listener. Bound dialing and retry failures.

The stage-0 prototype found that Tailcat v0.7.0 can retain an old introduction
acknowledgment and WireGuard peer state after the controller restarts. When the
control connection is lost, close that native client and create a fresh client
for the next connection generation, using the same saved controller capability.
Authenticate again and reattach the existing logical session. A repeated Ping
on the old native client is not proof that the new controller generation is ready.

Ordinary HTTP is not a fallback for terminal contents. Custom private dials
must not follow redirects or use an environment proxy to reroute credentials
outside the tunnel. Relay discovery itself must respect supported deployment
network policy.

## 9. Host and connection health specification

The installed reet binary is the probe and collector. No inbound health listener,
privileged helper, extra monitoring application, or shell session is required.

Health reporting is independent of terminal opt-in and of scan/sync execution.
If scans are paused, show Sync paused while private monitoring continues. Do not
silently reinterpret Pause as disabling all remote access: provide clear,
separate connection/terminal controls and accurate labels.

### 9.1 Measurements

| Measurement | Definition and source |
| --- | --- |
| Agent contact | Controller time of last authenticated valid contact |
| Host RTT | Controller-to-agent-to-controller application probe over Tailcat |
| RTT trend | Median and p95 over a rolling valid-sample window |
| Variability | Clearly labelled RTT variability, with documented calculation |
| Missed probes | Unanswered application probes, not IP packet loss |
| Reconnects | Observed private connection interruptions and recoveries |
| Terminal traffic | Actual terminal payload bytes to/from the host |
| Path | Direct, relayed, or unknown, only if the pinned transport exposes reliable observations |
| CPU | Host-wide utilisation over the sampling interval, normalised 0 to 100 percent |
| Memory | Total bytes and available bytes; used percentage derived consistently |
| Disk | Total and space available to the agent's user on its home filesystem |
| Host uptime | Time since OS boot, separate from agent uptime |
| Terminal readiness | Opt-in, version, tmux service, and session capacity |

Do not infer a direct/relayed path from latency. Do not label application RTT as
raw network RTT or browser end-to-end latency. Browser-to-controller latency,
if measured, is a separately labelled value.

The CPU's first sample is unavailable until a valid interval exists. Define
CPU normalisation across cores and validate it on Linux and macOS. Do not mix
an instantaneous load average with CPU utilisation.

A native report describes the machine; a container-scoped observation must be
marked as such. Missing, unsupported, permission-denied, or failed readings are
null with a reason, never zero or an old value presented as new.

Do not include process lists, command lines, usernames of other users, file
inventories, environment variables, or terminal contents.

### 9.2 Sampling, freshness, and bounds

- Application probe: every 15 seconds while a terminal is attached, every
  30 seconds otherwise, with jitter. Use a nonce, sequence, and connection
  generation; accept each matching response once.
- Measure elapsed time with the controller's monotonic clock. Do not subtract
  host timestamps to compute RTT.
- Initial probe timeout: 5 seconds. A timeout records a missed probe and
  degraded evidence; three consecutive missed probes mark the private
  connection unreachable. A closed socket may establish loss sooner.
- Resource sample: every 30 seconds, with jitter and bounded collection time.
  A failed collector must not delay pong processing or the next scan.
- At more than 90 seconds without a fresh resource report, show Report stale.
  Agent connectivity remains an independent observation.
- Use controller receipt time for freshness. Include host sampling time for
  diagnostics, plus schema version, sequence, and agent-instance ID.
- Reject duplicate/out-of-order reports within an instance. Fence old connection
  generations so an old report cannot overwrite the new instance.
- On reconnect, accept fresh reports without manufacturing historical samples.
- Keep recent readings and rolling connection samples in bounded memory.
- Persist minute-level summaries for 24 hours and prune automatically.
- Store units, valid/missing sample counts, and aggregate definitions. Do not
  average percentiles to manufacture an overall percentile. Persist suitable
  bounded buckets or samples if a combined distribution is needed.
- Reset or segment traffic counters on instance/generation change. Never show a
  negative rate after a restart.
- Do not run background bandwidth/speed tests.

Initial terminal-readiness results should distinguish disabled locally,
unsupported agent/protocol, tmux unavailable, supervisor unavailable, capacity
reached, reconnecting, and ready. An authenticated heartbeat alone is not proof
of a usable terminal.

### 9.3 Health UI

Machine cards display a compact summary of availability, host RTT, resource
readings, and terminal readiness. Expanded details show a small latency trend,
resource bars, observation timestamps, recent interruptions, and actionable
diagnostics. The terminal header stays focused on connection state and readiness.

Example only; these are not observed measurements:

~~~text
Connected · Host RTT 42 ms · Terminal ready
CPU 18% · Memory 6.2 / 16 GB · Disk available 84 GB
~~~

When unreachable, label retained measurements Last known. A connected agent with
an old resource report shows Report stale. Unknown observations are neutral,
not green. Never imply an unseen job is still running.

Fetch browser health updates through an authenticated account-scoped health
endpoint, initially at most once every 5 seconds while visible; pause polling
when hidden and refresh on return. Return controller-derived status and
serverNow so UI ageing does not depend on a correctly set browser clock. A
later shared event stream can replace polling without changing metric semantics.

No public /healthz expansion is required. Preserve the existing lightweight
container health-check contract.

## 10. Browser APIs and UI contract

Suggested authenticated browser API:

| Method | Path | Purpose |
| --- | --- | --- |
| GET | /api/v2/terminal/machines | Eligibility and readiness |
| GET | /api/v2/terminal/sessions | Current account's sessions |
| POST | /api/v2/terminal/sessions | Idempotent creation |
| PATCH | /api/v2/terminal/sessions/{id} | Rename, plain text only |
| POST | /api/v2/terminal/sessions/{id}/attachments | Attach or explicit transfer |
| POST | /api/v2/terminal/sessions/{id}/end | Durable end request |
| GET | /api/v2/terminal/sessions/{id}/history | Bounded recent-history snapshot |
| GET | /api/v2/terminal/socket | Narrow ticket-authenticated stream upgrade |
| GET | /api/v2/machines/health | Owner-scoped current health and bounded history |

The private bootstrap, control, and reverse-stream endpoints belong to the
agent-specific handler. Reuse established agent authentication without
reusing the entire public/admin router.

A history request authorizes the same session owner and asks the agent for a
bounded plain-text tmux snapshot through a separate reverse data stream, marked
as history in the broker's expected-stream registry. Return it as text with
no-store and nosniff; render it through textContent or equivalent. Do not put a
large history payload into the 16 KiB control channel. Limit it to 10,000 lines
and 1 MiB, with explicit truncation metadata. This is recent pane history, not
a complete recording.

### 10.1 Workspace

Use a dedicated route such as:

~~~text
/machines/terminal?machine=<agent-id>&session=<session-id>
~~~

- GET navigation never creates a shell. Create only after an explicit action.
- Machine cards offer Open terminal based on actual capability, not inventory
  freshness. Show setup/update guidance where unavailable.
- Show existing sessions and a prominent New terminal action.
- Header: machine, OS user, connection/readiness, session actions.
- Desktop: compact session sidebar or selector, tabs, terminal filling the
  remaining viewport, restrained status line.
- Controls: search, copy, paste, font size, maximise, Disconnect, End session.
- Mobile: contained session tabs and Ctrl/Alt/Esc/Tab/arrow modifier toolbar.
- Keep reet's existing brand, typography, tokens, voice, and light/dark support.
  Do not add a second UI framework.
- Use user-chosen titles. Escape-sequence window titles from a remote program
  must not become trusted machine identities or HTML.
- No recording, collaboration, split-pane, or AI feature should appear as
  unfinished controls.

### 10.2 Interaction states

| State | Copy/action |
| --- | --- |
| Disabled locally | Enable terminal access on this machine; setup instructions |
| Incompatible agent | This agent version does not support terminals; update instructions |
| Connecting | Connecting to MACHINE; Cancel |
| Reconnecting | Connection lost. Reconnecting; Retry or Disconnect |
| Host unreachable | Machine unreachable; last confirmed contact |
| Already controlled | This session is open in another window; Continue here |
| Shell exited | Session exited; New terminal |
| Lost session | The host no longer has this session; New terminal |
| End requested offline | End requested. Waiting for the machine |
| Connected, stale health | Report stale; last report age and explanation |

Do not hide the terminal behind a large loader on reconnection. Retain visible
output and status. Distinguish Disconnect from End session everywhere, including
tab closing. Confirm ending with the machine and session name; do not announce
success before acknowledgment.

A new attachment resets the emulator and receives tmux's current screen redraw.
Exact historical xterm scrollback restoration is not a v1 guarantee; recent
history is available separately. Do not concatenate a captured history dump
with a live stream and call that an atomic replay.

Use ResizeObserver and debouncing, ignoring zero dimensions for hidden tabs.
Theme changes update terminal options without recreating a session. Lazy-load
xterm and dispose terminals, addons, sockets, and observers on unmount.

Preserve normal terminal key handling. Copy shortcuts must respect selection
and platform conventions. Show a paste preview for multiline pastes before
sending bytes; never automatically append Enter. Disable automatic terminal
escape-sequence clipboard access and require explicit user clipboard actions.

Support keyboard traversal, focus restoration, screen-reader connection status,
reduced motion, and at least 44px touch targets. Do not announce every output
line twice through an additional live region. Provide screen-reader mode.
Test at 375, 768, and 1440 CSS pixels in both themes. No document-level
horizontal overflow; terminal content may scroll within its own region.

## 11. Privacy, retention, and audit

Audit operational metadata only: session created, attached, transferred,
detached, ended, lost, access denied, and connection recovered. Use structured
errors with no raw buffers attached.

Never log or send to analytics/error-reporting services:

- Terminal input, output, passwords, or command history.
- Browser stream tickets or agent credentials.
- Tailcat capability addresses or raw upstream diagnostics containing them.
- Full environments or copied shell command contents.

Do not write terminal contents to browser localStorage, IndexedDB, service-worker
caches, controller SQLite, backups, or session-replay tools. Browser preferences
and noncredential session IDs may be stored separately from secrets and content.

A same-page script can access terminal input. Keep third-party analytics and
session replay off the terminal page. Render output through the terminal library,
never HTML. Allow only appropriate link schemes and explicit user activation.

Resource and connection metrics are visible only to the enrolling account.
Bound labels and reject nonfinite/out-of-range values at ingestion. A collector
must not be able to overwrite another installation's records.

Database backups retain operational metadata and encrypted transport identity.
Document that the matching sealing key is required and that restoring the
database does not restore running shells. Account/enrollment deletion and
24-hour metric pruning must have deterministic cleanup behaviour.

## 12. Implementation stages and task cards

Every stage is a small buildable PR. Implement only the current stage, preserve
the approved contracts, and record exact validation. Do not mark later work as
implemented. Proposed new files are responsibilities, not permission to create
duplicate abstractions if the current tree already has suitable ones.

### Stage 0 — Compatibility and private transport spike

Files: go.mod/go.sum, build toolchain references if needed, isolated integration
fixtures under internal/privateconn.

- Verify dependency pin, toolchain availability, static builds, architectures,
  and library compatibility.
- Prove authenticated WebSocket traffic over Tailcat using a local relay.
- Prove forced relay operation and recovery after restarting the listener with
  the same identity.
- Bound startup/discovery failures and record binary-size/dependency impact.
- Record exact versions and recovery observations. Do not publish binaries.

Exit: repeatable local integration test passes and the dependency gate is
reviewable. Stop and report evidence if the required toolchain is unavailable;
do not invent a successful build or silently downgrade.

### Stage 1 — Protocol and persistence contracts

Implementation contract: see [the stage 1 notes](docs/remote-machine-stage1.md)
for the concrete envelope, store caller obligations, lifecycle edge cases,
generation fencing, and deletion/retention choices. No runtime endpoint or
terminal is enabled by that increment.

Files: internal/terminalproto/*, internal/store/store_terminal.go and tests,
health store migrations.

- Implement typed/versioned envelopes, validation, limits, and state transitions.
- Add encrypted identity, capabilities, sessions, operations, and health summary
  records.
- Implement transactional quota and idempotency; specify revoke/delete effects.
- Test malformed input, boundaries, duplicate requests, and concurrent creation.

Exit: one logical create produces one record and one quota reservation across
retries; migrations preserve existing data.

### Stage 2 — Private connection adapter and bootstrap

Implementation contract: see [the stage 2 notes](docs/remote-machine-stage2.md)
for the lazy listener, sealed identity restoration, exact private route allowlist,
connection generations, and disabled-auth boundaries. No agent loop, shell,
collector, or terminal is enabled by that increment.

Files: internal/privateconn/*, dedicated internal/api private handlers, server
assembly options in cmd/reet/main.go.

- Start lazily, persist/recover identity with existing sealing facilities.
- Authenticate exact agent paths; mark observed transport in trusted context.
- Implement bounded dial/reannouncement/reconnect and listener shutdown.
- Keep streams off public HTTP fallback, redirects, and inappropriate proxies.
- Add health-only enrollment capability independent of terminal opt-in.

Exit: restart, wrong-key, revoked-agent, forbidden-route, relay failure, and
disabled-auth tests pass.

### Stage 3 — Monitoring core before terminals

Files: internal/agenthealth/*, agent runtime loop, health persistence/API, tests.

- Implement cross-platform read-only collectors behind an interface; reuse
  tested library/OS adapters rather than parsing human-readable shell output.
- Run collection/pong processing independently of long scans and sync jobs.
- Resolve process-global working-directory hazards from section 2 first.
- Implement report sequences, freshness, counters, retention, and probe timing.
- Add fake-clock tests; preserve /healthz behaviour.

Exit: Linux and macOS samples obey units and null semantics; slow/failed
collection cannot block connection probes or existing agent operations.

### Stage 4 — Local consent and independent services

Files: agent configuration/CLI, internal/terminalhost service support,
agent-service lifecycle fixtures.

- Add enable/disable/status/sessions/local-end commands with private config.
- Validate intended OS user, tmux, directories, paths, and local enrollment.
- Install separate Linux and macOS agent/tmux services.
- Filter shell environment and document logout/linger/uninstall behaviour.

Exit: actual native service-manager tests prove agent restart/kill leaves tmux
and a running test process alive on both OSes. Fixture tests alone do not pass
this gate.

### Stage 5 — Host session manager

Files: internal/terminalhost/{manager,tmux,attachment,registry}.go and tests.

- Implement creation, attach-only, rename metadata, end, bounded history, and
  dead-pane/exit cleanup using explicit argv.
- Persist local operation receipts before acknowledgment.
- Reconcile registry and tmux on startup; mark missing sessions lost.
- Never bind session identity to a display name.

Exit: lost create acknowledgments do not duplicate shells; agent restart finds
the original session and process; missing sessions are not recreated.

### Stage 6 — Broker, browser authentication, and leases

Files: internal/terminalbroker/*, internal/api/terminal*.go and tests.

- Implement control/data binding, ticket authentication, origin checks, bounded
  queues, and exactly one controlling attachment.
- Implement explicit transfer with stale-writer fencing.
- Revalidate authorization and enforce finite leases.
- Implement durable end requests and revocation cleanup.

Exit: cross-account/admin/MCP/API-token denial tests and slow-consumer,
partition, transfer, replayed-ticket, and lease-expiry tests pass.

### Stage 7 — Terminal and health vertical slice

Files: typed browser API, minimal TerminalWorkspace/TerminalPane,
MachineFleet integration, Vite WebSocket proxy.

- Create one terminal through an explicit UI action.
- Stream input/output through Tailcat, never through a public host connection.
- Display live RTT and CPU/memory/disk/uptime observations.
- Disconnect, restart the agent, reconnect, and resume the same shell.
- Verify the original long-running process remains, not merely a new prompt.

Exit: browser integration test proves the complete vertical slice. Do not spend
the stage on broad styling before durability works.

### Stage 8 — Complete UX, session management, and history

Files: web/src/lib/terminal/*, shared machine health components, app routing.

- Add tabs/selector, explicit transfer, end confirmation, pending-offline state,
  bounded history, search/copy/paste/font/maximise, and actionable setup errors.
- Add compact health summaries and expanded charts with correct stale semantics.
- Finish light/dark, mobile modifiers, accessibility, focus, and cleanup.

Exit: keyboard/mobile/theme/state matrix passes without document overflow,
resource leaks, hidden input replay, or misleading health claims.

### Stage 9 — Resilience, documentation, and gated rollout

Files: integration/browser/native-service tests, operator docs, feature flags.

- Run the failure matrix in section 13.
- Add privacy assertions for logs/storage/error reporting.
- Document setup, updates, relay dependencies, recovery, local session handling,
  backups, metric semantics, and disabling/uninstalling.
- Release behind a server flag with local terminal opt-in still required.
- Pilot with a small fleet. Rollback disables remote access without killing
  surviving local tmux sessions or deleting data.

Exit: required checks and both native OS gates pass; results and remaining
operational limits are recorded accurately.

### Instruction template for an implementing model

> Implement stage N only. Read current repository instructions and prerequisite
> stage contracts first. Preserve ownership checks, privacy boundaries,
> idempotency, health semantics, and the distinction between session lifetime and
> connection lifetime. Use the selected libraries. Do not create a custom
> terminal emulator, encryption scheme, or general remote-command API.
> Add meaningful acceptance tests, run affected checks, and report changed files,
> results, and limitations. Stop if an agreed protocol or permission contract
> must change. Do not publish, merge, or deploy as part of implementation unless
> separately authorized.

## 13. Acceptance matrix and release checks

| Scenario | Required evidence |
| --- | --- |
| Duplicate/concurrent creation | One session and one quota reservation |
| Lost create acknowledgment | Retry identifies the existing shell |
| Browser close/refresh | Original shell can resume |
| Long command during disconnect | Same process continues |
| Controller restart | Same private identity; fresh authorization; same shell |
| Agent restart/kill | Independent tmux and original process survive |
| Direct path blocked | Local relay path carries terminal and probes |
| Relay unavailable/recovered | Bounded retries, honest UI, recovery without input replay |
| Browser or private link partition | Input disabled; leases expire; tmux survives |
| Second browser transfers control | Old writer is fenced |
| Account/enrollment/token revoked | Remote attachments close and cannot reauthorize |
| Local terminal disable | Remote input stops; health may remain available |
| Host offline during end | Pending until acknowledgment, never false success |
| Host reboot/tmux failure | Missing session becomes lost, never silently recreated |
| vim/top and resize/reconnect | Current screen and modes recover |
| Split Unicode across frames | Correct rendering without replacement corruption |
| Excess output/slow browser | Bounded memory and responsive UI |
| Failed/slow collector | Probes and scan/sync remain independent |
| CPU first sample/multicore | Correct unavailable state and 0–100 normalisation |
| Unsupported counters/permissions | Null plus reason, never invented zero |
| Container observation | Correct scope label, no claim of full host health |
| Stale resources, live connection | Report stale alongside connected status |
| Clock skew and duplicate samples | Controller-based freshness and fenced sequences |
| Restart/reset counters | No negative rates or old-generation overwrite |
| 24-hour retention | Old metric buckets removed without terminal-state loss |
| Cross-account health request | No other account's metrics exposed |
| Plain-text history | Bounded, explicitly truncated, never executed as HTML |
| Light/dark and phone widths | Accessible controls, focus, no document overflow |
| Logging/storage inspection | No terminal bytes, tokens, or capabilities retained |
| Machine rename/forget | Identity preserved; wording accurately describes access |
| Old agent / auth disabled | Clear unsupported/disabled state, no unsafe fallback |

Use disposable accounts, temporary homes, deterministic clocks, a local relay,
and test processes. Do not use production hosts or hosted public relays for
automated stress tests. Cross-compilation is not proof that macOS supervision
or resource collection works.

After affected checks, run the repository's required full checks for the final
runtime implementation:

~~~sh
cd web
npm run check
npm run build
cd ..
go vet ./...
go test -race ./...
cd mcp
npm test
~~~

Add browser tests and real Linux/macOS service-manager gates. Record exactly what
ran, which platform it ran on, and which checks remain unverified. Do not claim
deployment, release publication, remote upgrade, or process recovery without
observing the resulting state.

This documentation-only handoff does not require running application suites to
claim runtime validation: no runtime changes or validations are delivered by
this file.

## 14. First milestone and operator runbook

The first product milestone is:

1. Open a terminal inside reet.sh through the private Tailcat connection.
2. Observe live connection quality and host resource readings.
3. Start a long-running process and record its identity.
4. Disconnect the browser and restart the host agent.
5. Resume the same shell and verify the same process.
6. Confirm monitoring reconnects with a new agent instance and honest freshness.

The runbook must cover install/enable, non-root user selection, compatible
versions, tmux service supervision, resource units and scope, relay outages,
credential revocation, controller-key recovery, local session list/end,
host reboot limits, backup limitations, pending end requests, and safe
disable/uninstall/rollback.

## 15. Primary references

These are design references, not instructions to copy upstream code wholesale.
Reverify APIs and dependency requirements against the selected pinned versions.

- [Zoomies private hosts](https://zoomies.sh/private-hosts/)
- [Zoomies host-health reporting](https://zoomies.sh/host-health/)
- [Inspected Zoomies Tailcat client recovery source](https://github.com/eyupio/zoomies/blob/b6b549d308b698a5452a403bb3578b6c0b8f7cc4/internal/agent/tailcat.go)
- [Inspected Zoomies private controller listener](https://github.com/eyupio/zoomies/blob/b6b549d308b698a5452a403bb3578b6c0b8f7cc4/internal/api/tailcat.go)
- [Tailcat](https://github.com/tailscale/tailcat)
- [Tailcat v0.7.0 module requirements](https://github.com/tailscale/tailcat/blob/v0.7.0/go.mod)
- [Tailcat security model](https://github.com/tailscale/tailcat/blob/main/SECURITY.md)
- [xterm.js](https://xtermjs.org/)
- [xterm.js security guidance](https://xtermjs.org/docs/guides/security/)
- [Go WebSocket library](https://github.com/coder/websocket)
- [Go PTY library](https://github.com/creack/pty)
- [tmux manual](https://man.openbsd.org/tmux.1)
