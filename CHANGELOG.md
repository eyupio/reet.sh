# Changelog

Changes that matter to someone running a reet controller, newest first. Before
this file existed releases were described on their GitHub release pages.

## Unreleased

- `reet agent set-server URL` points a machine's enrollment at a controller that
  holds the same machine record, for following a controller that has moved. The
  machine first has the controller prove it holds its record (a fresh nonce
  answered with an HMAC keyed by the stored secret hash, over the new
  unauthenticated `POST /api/v2/sync/agent-proof`) and only then sends its
  credentials once to confirm them. Any failed check changes nothing and names
  the check. On success only the address changes and the agent restarts. Machines
  do not yet learn a fence's `--moved-to` address on their own.
- `reet backup restore` and `reet backup import` take `--public-url ADDRESS` (form
  field `publicUrl`) to rewrite the saved mail base URL and sync library origin to
  the controller's new address and to remove MCP grants issued for the old one. An
  address that is not a plain http or https origin is refused before anything
  changes. `GET /api/version` lists `"restore-public-url"` under `features`.
- A restore or import now holds back project run members, software batches and
  queued mail, as it already did Gaffer goals, so the restored controller does not
  repeat them. Each is resumed with the existing retry, and is listed in the
  report.
- The restore report ends with a checklist of what lives outside the database and
  must be set by hand on the new machine.
- **A controller can be fenced as moved.** `reet backup fence [--moved-to URL]
  [--lift]` and `GET`, `POST` and `DELETE /api/v2/admin/fence` (owner only) raise,
  read and lift a persisted state that refuses every write with 423 (Locked), with
  a message saying the controller has moved, when and to where. Reads work. The
  owner's backup settings, run, download and verify, the fence's own routes, and
  signing in and out still work. Scheduled backups, the mail queue, the notifier,
  project runs, software batches, Gaffer goals, queued chat messages and the
  scheduled sweeps stand still, and carry on after the lift. The state survives a
  restart. A copy taken while fenced is marked fenced. `GET /api/version` lists
  `"fence"` under `features`.
- **A copy of a controller that was not stopped now needs `--allow-live-source`.**
  `reet backup restore` and `reet backup import` (and the `allowLiveSource` field
  of `POST /api/v2/admin/backups/import`) refuse a whole-controller copy that
  says its source was still running, or that says nothing because an older reet
  took it, unless the flag is given. The refusal changes nothing. Copies now
  record `source` (the controller's own identifier, stable across a change of
  address) and `fenced`. The choice to allow a live source is written to the
  activity log. `GET /api/version` lists `"copy-origin"` under `features`.
- A staged import is checked again when the controller next starts. If the
  staged database changed, no longer passes SQLite's integrity check or is newer
  than the binary, it is moved to `pending-import.refused-<timestamp>/` with the
  reason, and the live database is left as it was.
- `GET /api/version` can carry a `features` list so a script can tell which
  parts of controller moving a build holds. It is left out when there are none.
- The README no longer says a restore ends browser sign-ins. It never did by
  default: sessions and personal tokens come across unless `--drop-sessions` or
  `--drop-tokens` is given.
