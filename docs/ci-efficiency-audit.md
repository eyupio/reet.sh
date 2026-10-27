# GitHub Actions efficiency audit — 27 October 2026

Measured with the `gh` CLI against `eyupio/reet.sh` over the 24 hours from 2026-10-26T15:00Z to 2026-10-27T15:00Z: 83
runs across the five heavy workflows, every one of them a pull request, about 940 runner-minutes in total. The three
fixes below are applied on this branch; the impact figures are expected, not measured, because no run has happened
since.

## What the audit found

Runner-minutes actually spent in the sample window:

| Workflow | Runs | Minutes | Dominant job |
| --- | --- | --- | --- |
| image | 17 | 286 | `test`, median 19.0 min (mean 17.9, max 37.7) |
| terminal browser | 16 | 244 | `terminal-browser`, mean 9.8 min (max 25.3) |
| binary release | 17 | 207 | `verify`, 184.5 min on its own, median 14.8 min |
| native services | 17 | 142 | `native service (macos-latest)`, mean 4.9 min |
| private connection | 16 | 60 | `local relay (macos-latest)`, mean 2.3 min |

The waste is not in caches or concurrency: every Go job uses `actions/setup-go`'s module and build cache, every non-Go
job sets `cache: npm`, and all six pull-request workflows already cancel superseded runs with a per-PR concurrency
group. Three drivers account for the waste.

1. **The Go suite runs twice on every code pull request.** `image.yml`'s `test` job runs `npm run check`, the MCP tests,
   `go vet ./...` and `go test -race -timeout=20m ./...`; `release.yml`'s `verify` runs exactly `go vet ./... && go test
   -race -timeout=20m ./...` on the same commit, and its pull-request paths (`internal/**`, `web/**`, `cmd/**`, `go.mod`)
   match nearly every code change. Measured: 184.5 of 207 release minutes in the window went to `verify`
   (`gh run view 40452581946`: `verify` 9m16s; `gh run view 40466790371`: `verify` 21m30s while `image` ran the same
   suite for 22m30s).
2. **`image.yml` has no pull-request path filter.** Its `test` job is the repository's full suite, so it runs for
   documentation-only and workflow-only pull requests. 5 of the last 33 pull requests (190–222) touched nothing that is
   compiled. Pull request #207 changed one file, `UI_AUDIT.md`, and still ran `image` for 7m25s plus `binary release`
   (`web` 27s, `verify` 4m31s, four build legs ≈ 6.5 min). Pull request #220 changed only workflow YAML and ran the
   suite twice. The skill's own rule applies: a workflow-only change that still runs the full suite means the gating

## Fixes

The exact edits are in [`ci-efficiency-audit.patch`](./ci-efficiency-audit.patch); apply them with
`git apply docs/ci-efficiency-audit.patch`. They are a patch rather than a commit because the automation token that
opened this branch is not allowed to write `.github/workflows/**`. Each fix below was checked with `actionlint` and
parses as YAML once applied.

**1. Stop duplicating the Go suite (`release.yml`).** `verify` now carries `if: github.event_name != 'pull_request'`, so
pull requests validate the packaging path (`web` → `build`) and rely on `image.yml` for the suite, while pushes, tags
and manual dispatches keep the full gate. `publish` now needs `[source, build, verify]` and requires
`needs.verify.result == 'success'`, so nothing publishes on an untested commit. No validation is removed from the
release path: it is only the pull-request duplicate of `image.yml` that goes.

**2. Scope the `image.yml` pull-request trigger.** `pull_request` gained a `paths` list covering what the job actually
compiles: `cmd/**`, `internal/**`, `web/**`, `mcp/**`, `scripts/**` (the installer and publish tests are Go tests in
that package), `go.mod`, `go.sum`, `Dockerfile`, `.dockerignore`, `docker-compose.yml` and its own workflow file.
`push` and `workflow_dispatch` stay unfiltered, so every published image has passed the suite. Documentation, brand
artwork and the other workflows are now free.

**3. Package once on pull requests, every target on release.** `build` no longer waits for `verify` (it only needs
`web`; the Go sources come from the checkout), so releases run the suite and the archives in parallel instead of one
after the other. Its matrix is `fromJSON`-driven: pull requests get `linux/amd64` only, release events still get all
four targets, and the upload step is skipped on pull requests because `publish` is the only consumer. macOS is not
unchecked at pull request time — `native services` and `private connection` already compile and test the same packages
on `macos-latest` (4.9 and 2.3 min medians).

Guardrails checked: no release, schema, migration or shared-library check is dropped; none of the three reduces needed
parallelism (the phone suites and the Go tests still run concurrently); matrix legs with a real platform commitment
(`macos-latest` in `native services` and `private connection`) are untouched; no formatter or write-back job exists to
opt in; and everything here is repository YAML — the org-level CodeQL workflow is out of scope and unchanged.

## Validation

Proven with live data before the change: the job timings and the skipped `publish` above come from the runs API, not
from reading YAML. `go test -race ./...` was 30m44s of a 37m40s `image` job on run 40520072189, so the suite, not setup,
is the cost.

Checked locally after the change: `actionlint` 1.7.12 (with `zoomies-linux-x64` declared as a self-hosted label) reports
no findings on any workflow, which covers the expressions, matrix shape and `needs` references added here. The matrix
expressions were evaluated by hand for both branches: pull request → `["linux"] × ["amd64"]`, everything else →
`["linux","darwin"] × ["amd64","arm64"]`.

Not done: live validation. No workflow has run against these files, so no claim here rests on a post-change run. The
four checks that would close it, on a non-protected branch:

- Open a pull request whose only commit touches `docs/**`; `image` and `binary release` should not appear at all.
- Push two commits to an open pull request in quick succession; the first `image` run should be cancelled by the
  concurrency group (unchanged by this work, unverified in this session).
- `gh workflow run release.yml -f tag=dev`, then confirm `verify` and the four `build` legs run concurrently, `publish`
  waits for both, and four `binary-*` artifacts are attached.
- Merge a code pull request and confirm the main-branch release still builds four targets and publishes.

Remaining risk: the branch ruleset `Protect Main` is disabled and only carries deletion and non-fast-forward rules
(checked through the API), so no required status check is lost today — but if `image / test` is ever made a required
check, path-gated pull requests leave it pending on documentation-only changes, and the ruleset will need the same

## Impact

Expected, from the measured medians — not measured, because there is no post-change run:

- `verify` (median 14.8 min) leaves every code pull request: about 184 runner-minutes a day at the sample's rate.
- Documentation and workflow-only pull requests (about 15% of recent ones) stop paying for the `image` suite: roughly
  43 minutes a day, plus the duplicate `verify` already counted above.
- Pull-request packaging drops from four legs to one: about 4.5 minutes per release-triggered pull request, and three
  fewer artifacts per run in storage.
- Release pull request wall clock falls from about 11 minutes (`verify` 9m16s → `build` 1m31s) to about 3 minutes
  (`source` → `web` → `build`). Release-event wall clock drops by the ≈1m40s that `build` used to spend queued behind
  `verify`.

Total runner minutes should fall by roughly a quarter against the 940-minute day. Wall clock on the fastest feedback
path barely moves — `image`'s `go test -race ./...` still sets the floor at about 20 minutes for a code change. Treat
all of this as an estimate: the sample window covers a runner migration to `zoomies-linux-x64`, and 12 of 17 `image`
runs were cancelled, which drags the means below what a completed run costs.

I can report the mechanisms that changed, but I cannot honestly claim exact minutes saved without comparing
before/after GitHub Actions runs.

## Next, not done here

- `phone-zoom` and `phone-dialogs` each repeat `npm ci && npm run build` and Chromium preparation for about 2 minutes of
  testing. Merging them saves ~1.5 minutes a run and leaves the 25-minute critical path untouched, at the cost of
  serialising two 2-minute suites.
- `image`'s 20-minute suite is the feedback floor. Splitting `go test -race ./...` across package groups would cut pull
  request wall clock to roughly 8 minutes, at slightly higher runner spend.

filter or a separate always-on check. Pull requests also no longer cross-compile for darwin/arm64; that compile coverage
moves to the macOS legs of `native services` and `private connection`, and to the release build that gates `publish`.

   model is too broad.
3. **The release workflow packages four times on pull requests and throws the result away.** `build` has no `if` guard,
   so a passing pull request runs all four cross-compile legs and uploads four archives, while `publish` is
   `if: github.event_name != 'pull_request'` and skips. `gh run view 40444983241` and `40452581946` show
   `verify` → four `build` legs at 1m25s–1m40s each → `publish` skipped. Those archives have no consumer and are left
   for `cleanup-artifacts.yml` to prune.
