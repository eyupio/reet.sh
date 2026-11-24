# For agents (and people in a hurry)

Only what the code does not show. How to build, test and open a pull request is in [CONTRIBUTING.md](CONTRIBUTING.md).

## Every change leaves two small files, not edits to shared ones

- A handoff entry: `docs/handoffs/log/<ID>.md` (see [handoff.md](handoff.md)).
- A changelog fragment for anything a person running reet would notice: `changelog.d/<name>.md` (see
  [changelog.d/README.md](changelog.d/README.md)). Do not edit `CHANGELOG.md` or `handoff.md` in a pull request.

## House style

- British spelling, plain words, no em dashes in UI copy. Commit subjects are an imperative sentence with no prefix.
- UI takes its colours from the design tokens in `web/src/app.css` (a literal colour fails `web/src/lib/theme.test.mjs`); there are no spacing or size tokens, so sizes are written in px as the surrounding CSS does. It uses the product's own marks (✓ ! ✕ · …), and 44 px touch targets
  on coarse pointers. Check light and dark, 390 px and desktop.
- Do not name other products reet was compared with in a commit, pull request, comment, code or doc.

## Traps

- **Worktrees have no `web/node_modules`.** `npm ci` in `web/` first, or link an existing one. Several Go tests
  transpile the web client's TypeScript and need it too.
- **Tests that transpile `web/src/lib/api.ts` or `web/src/lib/terminal/api.ts` allow one import only (`./inflight`).**
  A new import there fails those tests; put the helper in the file instead.
- **Every `REET_*` setting the server reads must be in the backup checklist** (`internal/backup/checklist.go`) and
  `serverEnv` in `internal/lifecycle/config.go`, or `TestTheChecklistNamesEverySettingTheServerReads` fails.
- **A feature added to the terminal protocol is stripped for older peers** by its negotiated feature name. Add the name
  to the feature lists in `internal/api/private_control.go` and the worktree test that lists them.
- **Browser suites** run against a built UI: `cd web && npm run build`, then
  `npx vite preview --host 127.0.0.1 --port N --strictPort` and
  `REET_UI_URL=http://127.0.0.1:N node tests/<name>.e2e.mjs`. Stop a preview by its PID; never `pkill`, which kills
  other people's runs.
- **`git fetch` before you merge `main`.** A stale local `origin/main` leaves the pull request conflicted, and a
  conflicted pull request does not start the main test workflow at all (only CodeQL runs).
- **The Go suite runs in six shards in CI** (`scripts/ci-go-shard.sh`). A new slow package should get its own shard.
  Locally, `go test ./...` is still the whole suite.
- **Do not skip, retry or lengthen a timeout to get green.** Find the cause. Known unstable tests, with an owner and a
  date, are in [docs/handoffs/flaky-tests.md](docs/handoffs/flaky-tests.md).
