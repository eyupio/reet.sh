## CI-GUARDS-1: the Gaffer browser job in shards, and a check that a pull request leaves its notes (24 November 2026)

The two leftovers of the roadmap's "Delivery speed" item that were CI work.

- `gaffer-settings.yml`: the 19 browser scripts ran one after another in one job, about 11 minutes, the longest browser job.
  They are now listed in `web/tests/ci-gaffer-settings.txt` and run as three shards (`gaffer-settings (1)` to `(3)`) by
  `scripts/run-browser-suites.sh`, the runner the terminal job already uses, so every script runs and every failure is named. A job
  still called `gaffer-settings` waits for the three and fails unless all passed, so that name means what it did. 7, 6 and 6
  scripts by position, not by measured time; read the first run's times and rebalance if one shard is much slower.
- `.github/workflows/notes.yml` runs `scripts/check-notes.sh` on every pull request that a person opens: it fails if the pull
  request adds no `docs/handoffs/log/*.md`, and only notes (does not fail) a change under `web/src`, `internal/api` or `cmd`
  that adds no `changelog.d/*.md`, because only a person can say whether a change is visible to someone running reet. Bots are
  skipped. Tried in a scratch branch with and without an entry, and with a code change and no fragment.
- `AGENTS.md` and the roadmap say so.

Not checked: the first real run of either workflow (this pull request's own run is the first). The check is new, so a pull
request opened before it landed has no entry and will fail until it adds one. It does not look at what an entry says.
