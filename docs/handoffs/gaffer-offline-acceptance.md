# Gaffer offline acceptance — GM6

6 November 2026. Test slice following owner-merged runtime #620/overview #623 and
widget/setup #625. Only reet.sh changes. Every model and worker is synthetic;
real credentials, paid calls, deployment and agent merges remain out of scope.

## Actual integration boundary

`internal/api/gaffer_integration_test.go` assembles production `api.New`, scheduler,
SQLite store, sealed provider key, provider HTTP transport, authenticated private
WebSocket, real broker and persisted queue. It never replaces the broker with
`SetWorker`. The fake provider verifies the selected model/key and rejects
cross-session markers. The fake worker implements only reconcile, live controls,
prompt and probe operations; unexpected actions fail the test.

`web/tests/gaffer-offline-live.e2e.mjs` drives the built browser with real HTTP,
without API interception. It checks one reviewed session setup from default-off
provider permissions, visible default limits, cancellation without model calls,
scoped goals, queued handoff through browser closure/controller restart, approval
waits, preserved resume budgets, hard limits, explicit completion, queued-grant
revocation and uncertain delivery. Browser-free recovery performs no queue-read
request to wake dispatch. The final overview uses real retained receipts.

Restart closes the API and SQL store, reopens the same database and reloads its
sealed test key. It is an in-process controller reconstruction, rather than an OS
service restart. A separate broker test reconstructs its store/broker/peer and
proves queued instructions resume without browser reads while old sending work
becomes uncertain and is never retried.

## Native and container gate

`scripts/test-gaffer-offline.sh native|docker|all` builds a test binary, runs the
flow and cleans up only its own resources. The native binary enables the race
detector. Docker uses the production Dockerfile's actual runtime base and non-root
user, read-only root filesystem, temporary data, dropped capabilities and a local
published port. Build client configuration preserves existing registry/proxy
settings in a temporary writable copy. Fixture files belong to the runtime user,
including when the host's umask is restrictive.

This validates the API harness inside the production container runtime; it does
not claim full production CLI/image packaging or service-install coverage. The
control endpoints exist only in the compiled Go test fixture, require the owner
cookie and exact origin for writes, and never enter the production server.

The workflow runs native and Docker jobs independently and uploads synthetic UI
captures. No live PR/check monitoring is simulated as external evidence: the fake
worker reports named check/conflict progress. Gaffer still has no direct GitHub,
shell, approval, interrupt or merge authority. Unsupported providers/channels and
physical devices remain outside this gate.

## Verification record

The real private-wire HTTP/scheduler/broker integration and queue-recovery race
tests pass. Full browser flows pass in native with race detection (343 seconds)
and the production container runtime (325 seconds), including the revocation and
uncertain-delivery cases. Changed-package vet and shell syntax checks pass.
Inspected captures use synthetic data: native/light desktop review and overview,
Docker/dark phone overview. They are recorded in the UI verification page.
The owner merged [PR #626](https://github.com/eyupio/reet.sh/pull/626) as
`aa64c24f`, after widget/setup #625. Final head `db4f4ff7` passed all 22 applicable
checks, with four intentional skips and no unresolved inline review threads. The
automated reviewer was quota-blocked. See the [pause checkpoint](gaffer-resume.md)
for the continuation procedure and unverified areas.

The performance comparison now compiles both revisions before sampling and
interleaves eight one-second samples, reversing the starting revision each round.
It retains the existing benchmark set and statistically significant 15% gate.
This avoids measuring all base samples several minutes before all PR samples on
a shared runner. The identical-store-code local control uses all eight samples
per side; its original gate passes with the paired workflow.
