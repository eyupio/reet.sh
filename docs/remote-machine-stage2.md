# Remote machine stage 2: private bootstrap and agent hello

This implements stage 2 of the [handoff](../remote-machine-implementation-plan.md),
following the private transport foundation and durable contracts. The controller
can now give an authenticated enrollment a private controller capability and
accept a fenced handshake over Tailcat. The installed agent still does not
connect, report health, manage tmux, or offer a terminal; those remain later
stages.

## Controller flow

1. An enrolled agent posts to `POST /api/v2/sync/private` over ordinary HTTPS.
   The existing account authentication and enrollment headers are both required.
   The response is `{protocolVersion, address}` with `no-store` and `nosniff`.
2. The controller lazily restores or creates its sealed Tailcat identity, starts
   its private listener when first requested, and returns the listener's
   capability. The address is an opaque capability, not an account role.
3. The agent opens a private HTTP connection and posts a typed handshake to
   `POST /private/v1/hello`. Only that exact path and method exist on the private
   handler. Query strings, redirects, unknown routes, and unexpected methods are
   rejected.
4. The controller authenticates the enrollment from its agent ID and secret,
   validates the handshake, advances the persisted connection generation, and
   returns `{protocolVersion, requestId, generation}` with `no-store`.

The private handler is not mounted on the public mux. A request reaches it only
through the Tailcat listener, which marks the request context. A header or agent
message cannot mark a request as private. Browser machine responses never include
the capability, and bootstrap response bodies are not logged.

## Identity and recovery

- The complete Tailcat identity is sealed with the existing instance backup key
  and stored as the singleton `private_transport_identity` record.
- A stored identity is always restored and authenticated. A missing sealing key,
  wrong key, damaged ciphertext, or unsupported format fails closed; the runtime
  never silently generates a replacement.
- If two controller processes attempt the first insert, the stored winner is
  used only after it opens with the configured key. An unpersisted identity is
  never served.
- The first bootstrap performs the relay-discovery work. Later bootstraps reuse
  the restored identity, so the capability survives controller restarts.
- Listener loss is retried with exponential backoff, jitter, and a 15-second cap.
  Retries reuse the saved identity and therefore the same capability. The retry
  loop stops on shutdown.

## Authentication and feature boundaries

- Accounts mode requires a personal token or session plus a valid enrollment for
  bootstrap. Token mode requires a configured non-empty `REET_TOKEN`. `REET_AUTH=none`
  has no authenticated principal and refuses both bootstrap and handshake.
- Private handshakes accept enrollment credentials only; they do not read a
  browser session, personal token, MCP credential, bootstrap token, or proxy
  header to establish identity.
- Health-only capability is independent of terminal opt-in: an agent may report
  `terminalEnabled:false`, `healthEnabled:true`, and `readiness:"disabled_locally"`
  and still establish a connection generation. Session authorization remains
  absent in this stage.
- A revoked enrollment or disabled account cannot bootstrap or handshake. The
  refusal is the same for unknown IDs and wrong secrets.
- Capacities, terminal consent, session dispatch, attachment tickets, probes,
  and durable end requests are not implemented by this increment. No shell can
  be created.

## Configuration

`REET_RELAY_MAP_URL` optionally selects an HTTPS Tailcat relay map. Empty uses
the Tailcat default. The value is never returned to a browser; relay discovery
respects the deployment's outbound network policy.

## Verification

Focused tests cover:

- lazy start, saved identity restoration with the same capability, listener
  restart, bounded retry after discovery failure, shutdown, and concurrent start;
- wrong-key and damaged-identity failure without replacement or discovery;
- bootstrap authentication, cross-account denial, disabled auth modes, missing
  configuration, relay failure, and no capability in browser machine state;
- exact private paths/methods, unmarked and header-spoofed requests, query-string
  rejection, wrong content types, oversized and malformed handshakes;
- health-only capabilities, generation fencing across reconnects, revoked
  enrollments, and disabled accounts.

The tests use the existing local relay fixtures and do not contact hosted relays.
Cross-compilation is not claimed as native macOS or service-manager validation.
No agent service, tmux session, terminal UI, or health collector is enabled.

Next: stage 3 must add the agent-side bootstrap/connection loop, independent
resource collection, controller-receipt freshness, probe matching, and bounded
health persistence. Keep terminal creation disabled until the later consent,
supervision, broker, and browser-authorization stages pass their gates.
