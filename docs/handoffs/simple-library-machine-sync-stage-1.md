# Simple library and machine sync: Stage 1

8 November 2026 · implementation handover

Parent: [Simple library and machine sync](simple-library-machine-sync.md). Baseline: [Stage 0](simple-library-machine-sync-stage-0.md). Pull request: [#662](https://github.com/eyupio/reet.sh/pull/662).

Continuation: [Stage 2 foundation and handover](simple-library-machine-sync-stage-2.md). The results below are the Stage 1 record; use the parent ledger and continuation for the current implementation checkpoint.

**Status:** Stage 1 complete. All 44 Go test packages have passing race results across the broad run and complete corrected package reruns below. Web checks/build and `go vet` pass. The handover records remaining field-testing limits, actual CI observations and browser-testing instructions; no preview or production deployment was created.

## Starting point

- Branch: `feat/simple-library-sync-recovery`.
- Stage 0 was committed as `31fa681e9409633560eb664f24edddc3a5a0d579` after rebasing onto main `c40f67e118d8d1f8c7fe1de6cf9170a0d516c940`.
- The Stage 0 restart probe still failed on that rebased baseline before the fix. Its local log is `/workspace/work/stage1/baseline-import-restart.log`.
- Scope is Stage 1: persistent enrollment, recoverable transfers, independent sync/private monitoring, restart and credential boundaries. Library status/selection changes and the new UI remain later stages.

## Runtime change

### Why an import stopped

A machine-only skill's private capture is bound to its connector process instance. When that process restarts, the old preview/upload must be rejected. Previously, `ClaimRemoteImportFor` correctly marked it `expired` / `preview_expired`, but its library idempotency key remained occupied. `libraryCaptureProblems` then kept the item excluded from reconciliation, so ordinary reconnection could not recover it.

### Recovery now

`internal/store/store_remote_import.go` adds `releaseExpiredLibraryImport` and considers previously expired library captures during authenticated source claims.

1. Keep the old capture expired, with its original process, preview and history.
2. For an automatic following-library capture only, move its current server-generated `library_<skill hash>` request key to `retried_<old request id>`.
3. Existing policy reconciliation can now request a new capture through `RequestMachineSkillSync` / `CreateRemoteImportFor`.
4. That new request gets a new ID and current process binding, and must produce a fresh preview before any upload.
5. Existing source/destination ownership, pause/role checks, content validation, expected-base checks, capacity limits and atomic receipts still apply.

This also recovers captures already left expired by an older controller. It does not merely fix restarts occurring after the upgrade.

### Boundaries retained

- Old request IDs, approvals and process-bound uploads remain invalid.
- Old previews, exclusion lists and approvals are not transplanted into the new request.
- Manual imports and non-library request keys retain their previous review/retry behaviour.
- Cancelled requests, permanent capture failures and completed receipts are not automatically revived.
- Paused/revoked sources and read-only accounts cannot retire a request key through an unauthorised claim.
- The helper only releases a retry key; it does not itself authorise a transfer or activate a destination. Current policy must schedule the replacement through the existing path.
- No database migration, wire capability, credential scope, public endpoint or dependency was added.
- Existing bounded import-history limits remain in force; old attempts remain inspectable.

The runtime patch is confined to import recovery. Existing HTTPS polling, private-monitoring backoff and stable enrollment already provide the separation needed by the new integration checks; those paths were verified rather than replaced with another transport.

## New and promoted verification

### Store-level recovery and authorization

`internal/store/store_remote_import_recovery_test.go` exercises:

- Restart during a preview, expiry with the same process, and a record already expired by an older controller.
- A fresh replacement request with no inherited preview, approval or exclusions.
- Rejection of old approvals and uploads.
- Exactly one canonical bundle/revision after a successful replacement and an identical receipt retry.
- Cancelled, permanently failed and completed requests staying terminal.
- Standalone manual imports retaining their consent/retry boundary.
- Paused/revoked source enrollment and viewer-role claims failing before changing the old retry key.

The pre-existing remote-import exclusion, cancellation, owner and atomic receipt tests also passed in the focused run.

### Real controller and connector processes

`internal/api/sync_recovery_test.go` contains the promoted restart regression and new recovery tests. They use the production controller routes, a real temporary SQLite database and actual files in separate connector homes.

| Scenario | Verified outcome |
| --- | --- |
| Restart source between preview and upload | A fresh capture completes; the destination receives exact skill/supporting-file bytes; two enrollments remain. |
| Lose a controller-accepted result reply, reopen the store through production `store.Open` and restart the HTTP listener/connector | The saved result is reconciled, stored enrollment IDs and target selections remain, and new canonical content reaches both machines without re-enrollment. Private config bytes remain unchanged. |
| Revoke one machine's individual account token | That machine loses access while its local config, server enrollment and selections remain. The other machine receives subsequent edits using its own credential. |
| Run actual foreground connector with private bootstrap returning HTTP 503 | The production timed poll loop continues syncing, while local status correctly reports private monitoring disconnected and a successful sync cycle. |
| Interrupt all foreground connector HTTP traffic | Work waits; restoring ordinary HTTP reachability delivers the queued revision using the same enrollment. |
| Stop foreground connector | Interrupt-driven shutdown joins its workers and releases the local lifetime lock. |

The Stage 0 harness now gives each machine its own disposable credential. It also supports controller reopening on the same origin, short private control-socket paths and a foreground worker with captured fixture logs. Test workers stop cleanly and their exit status is checked, including when race detection is enabled.

The import-restart test is now in the ordinary suite, retaining its name `TestSimpleSyncContractImportRecoversAfterConnectorRestart`. The other four approved-behaviour probes remain opt-in under `sync_contract` because their implementation belongs to Stages 2–4.

## Validation

Local environment: Debian 13.6 amd64, Go 1.27.1 and Node 22.23.4.

| Check | Result |
| --- | --- |
| New import recovery store tests and existing consent/receipt tests | Passed. |
| Promoted import restart plus controller/connector restart, token isolation and foreground recovery | Passed; focused API package time 27.225 seconds. |
| Strengthened controller-restart scenario with durable uncertain acknowledgement and production store reopening | Passed under race detection; package time 15.730 seconds. |
| Final recovery tests under race detection, repeated three times | Passed; package time 154.625 seconds. Includes the corrected outage observation and strengthened controller restart. |
| `go vet ./...` | Passed. |
| `npm run check` from `web/` | Passed; zero Svelte errors or warnings. |
| `npm run build` from `web/`; `go test -race ./web -count=1` | Passed; fresh assets replace a stale local build. Web race package time 0.509 seconds. |
| Full repository `go test -race -p 2 ./... -count=1 -timeout=15m` | Initial run: 41 packages passed, 3 required the correction/setup reruns below. Initial exit 1 is retained in the log; all 44 packages now have passing race results across these runs. |
| Corrected complete API race package, `-count=1 -timeout=60m` | Passed; package time 700.658 seconds. No race reported. |
| Complete terminal-host race package under a child subreaper | Passed; package time 47.170 seconds. Both process-group cleanup checks pass without changing their assertions. |
| Remaining tagged future-stage probes | Four expected failures reproduced (selection, inactive authored skills, deletion/notification, inactive removal); promoted restart test passed. Package time 3.406 seconds; exit 1 is intentional for the four unimplemented contracts. |

The full race run uses `GORACE=atexit_sleep_ms=0` to avoid an exit delay for helper processes. This does not disable race detection or its failure exit code.

The first full run exposed a test-observation bug: the foreground outage assertion counted only failed polls, while a pending result was correctly retried before polling. The fixture now counts failed file-sync requests separately from private bootstrap. Three repeated focused race runs and the corrected complete API race suite pass. The initial run itself was not green; final coverage combines its 41 passing packages with complete API, terminal-host and web package reruns.

The same run found stale local `web/dist` assets and two terminal-host process-group checks that observed already-killed children as zombies (`State: Z`, parent PID 1). Rebuilding the web assets resolves its package failures. This container's PID 1 does not reap those orphaned children; the complete terminal-host race package passes under a per-command Linux child subreaper. This runner change is scoped to test execution and changes no product code or test expectations. Use a proper reaping init on future container hosts.

The container initially lacked tmux. Debian tmux 3.5a-3 was extracted into `/workspace/.reet-tools/tmux` for test execution, with archive SHA-256 checked against the HTTPS-fetched Debian package index. No system package or actual agent application was installed.

Commands, from the repository root unless stated otherwise:

```sh
# First run npm run build in web/ so Go embeds current assets.

go test ./internal/store \
  -run 'TestExpiredLibraryImport|TestLibraryRecovery|TestRemoteImportCancellationExpiryGenerationAndOwners|TestRemoteImportReviewExclusionsAndAtomicReceipt' \
  -count=1 -v

go test ./internal/api \
  -run '^TestSimpleSync(Recovery|ContractImportRecovers)' \
  -count=1 -v

go vet ./...

PATH="/workspace/.reet-tools/tmux/usr/bin:$PATH" \
GORACE=atexit_sleep_ms=0 \
REET_SYNC_CONTRACT_EVIDENCE_DIR=/workspace/work/stage1/evidence \
go test -race -p 2 ./... -count=1 -timeout=15m

GORACE=atexit_sleep_ms=0 go test -race ./internal/api \
  -run '^TestSimpleSync(Recovery|ContractImportRecovers)' -count=3 -timeout=5m

PATH="/workspace/.reet-tools/tmux/usr/bin:$PATH" \
GORACE=atexit_sleep_ms=0 \
go test -race ./internal/api -count=1 -timeout=60m

PATH="/workspace/.reet-tools/tmux/usr/bin:$PATH" \
GORACE=atexit_sleep_ms=0 \
python3 /workspace/work/stage1/run_with_reaper.py \
  go test -race ./internal/terminalhost -count=1 -timeout=5m

GORACE=atexit_sleep_ms=0 go test -race ./web -count=1

go test -tags=sync_contract ./internal/api \
  -run '^TestSimpleSyncContract' -count=1 -v
```

Run `npm run check` from `web/`. In this workspace, `/workspace/.reet-tools/activate.sh` selects the cached Go/Node tools; that path is local setup, not a repository prerequisite.

Scratch evidence is under `/workspace/work/stage1`: `store-recovery.log`, `api-recovery.log`, `pending-controller-restart-race.log`, `vet.log`, `web-check.log`, `web-build.log`, `full-race.log`, `final-recovery-race.log`, `final-api-race.log`, `final-terminalhost-race.log`, `final-web-race.log`, `future-contract.log` and optional fixture JSON. Future models can reproduce the checks from the committed tests without these scratch files; the local subreaper wrapper is environment setup, not a repository prerequisite.

## Runtime evidence limits

The foreground check exercises the real timed connector and local control socket. It deliberately rejects private bootstrap and therefore does not itself establish a healthy or recovering Tailcat data connection.

The full race run passed the actual local encrypted Tailcat/relay and sealed-identity restart tests under `internal/privateconn` (43.110 seconds), private-worker cleanup tests under `internal/agenthealth` (5.829 seconds), and daemon checks under `internal/scan` (14.708 seconds). The complete terminal-host race rerun passed in 47.170 seconds with the container-reaping setup described above.

This container does not provide a normal logged-in systemd user session or a physical reboot/logout test. Keep the documented Linux lingering and macOS LaunchAgent boundaries. GitHub's existing native-services workflow exercises supported service hosts separately; report its observed state rather than claiming local service installation.

For the Stage 1 implementation commit `df57df560a2aa4002c747a63467760f1cde5091e`, GitHub's [private connection](https://github.com/eyupio/reet.sh/actions/runs/42490748161) workflow passed. The [Linux native-service job](https://github.com/eyupio/reet.sh/actions/runs/42490748173/job/128762355530) passed, including native-agent kill/restart and root system-service recovery; the tmux 3.2 job also passed. The macOS native-service job and full image workflow are still running at this update. Recheck the final PR head; workflow completion is separate from production deployment.

The reported production Tailcat outage still has no captured production connector/controller logs. Stage 0's connected reet read calls returned internal MCP errors. No production tokens, inventories or machines were changed, and this handoff does not claim that specific field incident has been diagnosed.

## Web UI test checkpoint

**Availability:** PR #662 contains the implementation. No preview URL or production deployment has been created by this work. These are manual test instructions, not a claim that the user has run them.

Once the PR controller build is running in a preview, use a disposable account and two Linux test connectors, each enrolled against that controller with its own config/token:

1. In the existing Machines/Skills pages, connect both machines and create an In use skill with supporting content. Confirm the expected files reach both machines.
2. Restart one connector using its saved config. Confirm it returns under the same machine entry and sync continues. The automated suite additionally covers restart between private preview and upload.
3. Take one connector offline, edit the skill centrally and confirm the other machine updates. Bring the first connector back and verify it catches up without enrolling again. Refresh the UI to confirm both machine entries remain.

The Stage 1 change is recovery underneath the current UI. Stage 3 is the first selected-item automatic-sync checkpoint; Stage 4 adds removal/restoration notifications; Stage 5 delivers the new minimal interface. The parent handoff lists these checkpoints explicitly so later stages remain testable from the browser.

## Stage 2 entry

1. Read the parent product contract and Stage 0 authority/migration map, then rebaseline the current PR/checkout.
2. Implement canonical item identity, complete typed content and lifecycle projection while preserving authored skills, likes, lists, bookmarks, notes/tags and visibility.
3. Keep inactive item status distinct from machine selection. Deployment/removal semantics still need their later-stage implementation.
4. Preserve this recovery's authorization and request-generation fences when centralising the library pipeline. Any change to library capture keys or scheduling must retain recovery for existing expired attempts.
5. Keep the four outstanding contract probes explicit; do not change their expectations to match current whole-library/additive behaviour.

## Stage 1 handover

- **Status:** complete, with field-only checks and production diagnosis limits retained above. Local race coverage, web checks/build and vet pass; broader GitHub CI is still running for the implementation commit.
- **Starting commit:** `31fa681e9409633560eb664f24edddc3a5a0d579`.
- **Implementation commit:** `df57df560a2aa4002c747a63467760f1cde5091e`; subsequent documentation records final validation and browser-testing checkpoints.
- **PR/branch:** [#662](https://github.com/eyupio/reet.sh/pull/662), `feat/simple-library-sync-recovery`.
- **Runtime file:** `internal/store/store_remote_import.go`.
- **Tests:** `internal/store/store_remote_import_recovery_test.go`, `internal/api/sync_recovery_test.go`, updated shared harness and tagged future-stage probes.
- **Compatibility:** no schema/API change. Existing imports and receipts stay readable; only expired automatic library retry-key handling changes.
- **Activation:** ordinary authenticated source polling can recover eligible captures after upgrade; current policy schedules each fresh attempt.
- **Rollback:** revert the Stage 1 runtime patch to restore old retry behaviour. Archived expired attempts remain ordinary retained history; no user content is deleted.
- **Not part of this stage:** new Library/Machines UI, status-based distribution/removal, general latest-save conflict ordering, four-app provisioning, independent backup rollout or production deployment.
- **Next:** Stage 2. Rebaseline the PR/checkout, read the authority/migration map and begin canonical item identity/content/status while preserving this recovery's request-generation fences.


### CI supervision follow-up

The owner authorized checking all open PRs and merging completed green work. The shared-native and structured-worktree jobs were cancelled by their ten-minute job budgets while Go was still compiling packages; real ACP tests in the same log had passed. Their job budgets are now twenty minutes to include cold race compilation. The existing per-test three-minute limits, pinned real-agent checksums and test assertions are unchanged. This is a runner-budget correction, not a claim that the cancelled checks passed. Fresh GitHub checks remain required before merge.
