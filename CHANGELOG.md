# Changelog

Changes that matter to someone running a reet controller, newest first. Before
this file existed releases were described on their GitHub release pages.

## Unreleased

- When a worktree is removed, reet now also brings the repository it was made
  from up to date with main, then prunes the merged local branches. It is on by
  default (owners and admins can turn it off in Settings › Admin › Repository
  maintenance). The folder is only ever fast-forwarded: a folder with
  uncommitted changes, on another branch, mid-merge or diverged from main is
  left exactly as it was. Each run is recorded in Activity as `branch-sync`.
  Needs the new `branch-sync-v1` on the machine; older machines are skipped.
- A staged import is checked again when the controller next starts. If the
  staged database changed, no longer passes SQLite's integrity check or is newer
  than the binary, it is moved to `pending-import.refused-<timestamp>/` with the
  reason, and the live database is left as it was.
- `GET /api/version` can carry a `features` list so a script can tell which
  parts of controller moving a build holds. It is left out when there are none.
- The README no longer says a restore ends browser sign-ins. It never did by
  default: sessions and personal tokens come across unless `--drop-sessions` or
  `--drop-tokens` is given.
