# Remote machine stage 0: private connection foundation

This is the first implementation increment of
[the remote machine handoff](../remote-machine-implementation-plan.md).
It adds an internal transport adapter and compatibility fixtures. The public
server and installed agent do not call it yet: this change does not expose a
terminal endpoint, start a private listener on application startup, collect
host resources, or provide tmux session persistence.

## Implemented

- A bounded Tailcat relay-discovery and userspace listener adapter, admitting
  only private TCP port 80. The future caller must supply an authenticated,
  allowlisted agent handler; the adapter itself grants no account role.
- Versioned identity sealing through the existing instance Sealer. Wrong keys,
  damaged data, wrong purpose, and invalid identities fail without replacement.
- A private HTTP/WebSocket client that does not use an environment HTTP proxy,
  follow redirects, or fall back to ordinary HTTP networking.
- Listener shutdown that closes accepted streams as well as pending accepts.
- Suppressed upstream capability-bearing diagnostics and redacted formatting
  of identities, listeners, and clients.
- Local DERP fixtures for authenticated HTTP and binary WebSocket traffic,
  forbidden fixture routes, proxy/redirect isolation, controller identity
  restoration, deadline cancellation, and resource cleanup.
- A Linux/macOS CI matrix for the adapter tests. Native service supervision
  remains a later implementation stage, not something these tests certify.

## Pins and compatibility

| Component | Version |
| --- | --- |
| Minimum Go language/toolchain | 1.27.1, required by the selected Tailcat tag |
| Locally verified SDK and Docker build pin | Go 1.27.3 |
| github.com/tailscale/tailcat | v0.7.0 |
| tailscale.com | v1.103.0-pre.0.20260916030321-a2263542f260, Tailcat's requirement |
| github.com/coder/websocket | v1.8.15 |

The official Linux amd64 Go SDK was downloaded and checked against the SHA-256
published by go.dev. Build and CI configuration must continue to select a
compatible toolchain; no older Tailcat tag was substituted.

The direct added library licences inspected are BSD-3-Clause for Tailcat and
ISC for coder/websocket. Tailcat brings a substantial prerelease Tailscale and
gVisor dependency graph: explicit go.mod require entries increase from 23 to 68.
Module selection also upgrades golang.org/x/oauth2 from v0.35.0 to v0.36.0 and
golang.org/x/sync from v0.23.0 to v0.24.0. The full regression suite is therefore
required even though application routing is unchanged. This compatibility
exercise is not a claim that the dependency graph has passed a security audit.

For a like-for-like Linux amd64 build with the same generated UI, Go 1.27.3,
CGO_ENABLED=0, -trimpath, -buildvcs=false, and -ldflags='-s -w', the application
binary is 21,967,008 bytes both before and after the dependency update. The
adapter is not yet linked by cmd/reet, so this is not an estimate of the final
terminal feature's size. Measure again when the runtime is wired.

## Recovery findings that the next stages must preserve

Treat the native Tailcat client as a connection-generation resource. After the
control connection fails, close it, construct a new client with the saved
controller capability, and authenticate again. Keep logical machine and future
terminal session identities separate from this replacement.

Tailcat v0.7.0's Ping can observe an introduction acknowledgment cached by the
old client. Repeating it does not prove the restarted controller has replied,
and old WireGuard state can prevent timely recovery. The restart fixture
restores the sealed controller identity and establishes a fresh authenticated
connection; it does not pretend that a stale native client safely resumes.
Do not use this transport handshake as the UI's application RTT measurement.

The relay-only tests use TS_DEBUG_NEVER_DIRECT_UDP and assert the observed
transport path. TS_DEBUG_ALWAYS_USE_DERP installed dummy UDP sockets that hung
in upstream shutdown during the prototype; it is not used. No upstream code is
vendored or patched to conceal that fixture problem.

The integration tests use a local relay and embedded test region, disable
external netcheck/captive-portal work, and require only loopback sockets and
network-interface discovery. They do not require a Tailscale account, root,
TUN device, or hosted relay. Test-only TLS relaxation is confined to the local
DERP fixture; application TLS verification is not disabled.

## Validation and boundaries

The focused Linux amd64 race run passes all six top-level tests, including
controller restart, binary streaming, and shutdown, across three consecutive
runs. The final full Go vet and race suite pass too. The existing web check and
production build pass with zero Svelte diagnostics; all 16 MCP tests pass with
the verified Go compiler on PATH. The adapter also builds with CGO_ENABLED=0.

The new Linux/macOS CI matrix must run on this increment. Cross-compilation
alone is not native macOS runtime evidence.
No container image, release, production host upgrade, browser terminal flow,
or persistent host shell has been published or validated by this prototype.

Next: implement stage 1 protocol/persistence contracts, then authenticated
bootstrap/runtime wiring, followed by independent host health reporting. Keep
remote shells disabled until the later consent, supervision, and authorization
stages pass their acceptance gates.
