# Changelog

Changes that matter to someone running a reet controller, newest first. Before
this file existed releases were described on their GitHub release pages.

## Unreleased

- The Gaffer factory floor is easier to read and to reach. Followed-session cards
  are short, with the full goal and evidence under **Review & evidence**; the
  floor is wider on desktop and laid out as Outcomes, Sessions and Not followed;
  an empty Gaffer chat leads with **Give Gaffer an outcome**; Gaffer's entry on
  Home is a button; and the outcome dialog shows its steps. Browser-only.
- Gaffer outcomes are now started by the controller. After you approve a plan the
  sessions start and are followed whether or not the browser stays open, a
  restart carries on without starting a session twice, and **Stop starting
  sessions** leaves the rest unstarted. You can choose one model for the outcome
  and, for OpenCode and Google Antigravity, confirm you trust the folder's
  plugins. The floor gains **Pause all following** and **Stop all goals** for the
  sessions shown. Fixes from review: a launch the machine refused is no longer
  reported as started, limits must be whole numbers, and prompts an agent cannot
  be started with are caught before anything starts. Adds
  `GET/POST /api/v2/gaffer/outcomes`, `GET /api/v2/gaffer/outcomes/{id}` and
  `POST /api/v2/gaffer/outcomes/{id}/stop` (browser account only) and a
  `gaffer_outcomes` table, created on start. Restoring a copy stops any outcome
  that was still starting.
- **Give Gaffer an outcome** (factory floor). Write what you want done and choose
  a project and agent; Gaffer proposes the sessions and their prompts, you edit
  and approve the plan, and the sessions are then started as ordinary chats and
  followed, each with a goal naming the outcome. The floor groups them under
  **Outcomes**. Nothing starts before you approve, sessions never start in YOLO,
  and Gaffer still cannot merge. Adds `POST /api/v2/gaffer/providers/{id}/plan`
  (browser account only), which sends the provider only the typed objective and
  project name and saves nothing. Needs a controller update; no database or
  connector change.
- Gaffer's **Followed sessions** view is now the **Factory floor** (Ask Gaffer →
  Chat options). Beside the sessions Gaffer already follows it lists every
  running structured chat that is not followed. Tick several, review one goal
  and one set of limits, and each session is started through its own reviewed
  setup, with a per-session result. Each keeps its own access, goal and budget.
  The floor opens in one click from general Gaffer chat and from Home, which
  now also says when running chats are not followed. The separate "Choose a
  session for Gaffer" picker is replaced by **Open** on each row.
  Browser-only: no server, database or connector change.
- A portable copy can be taken with a password given for that copy alone.
  `POST /api/v2/admin/backups/run` accepts an optional body `{"password": "..."}`
  and `reet backup run` takes `--password-file F` (a file or `-`, never an
  argument). The copy is sealed with that password in the same format as one
  sealed with the stored copy password, and the stored setting is not read,
  changed or cleared, so the schedule and every other copy are unaffected. The
  password needs at least 12 characters; a blank or shorter one is refused with
  a 400 before anything is taken. It is never stored, logged or returned, and the
  activity log records that a copy was taken with a password given on the
  request, by whom and when. `GET /api/version` lists `"copy-password-request"`
  under `features`.
- The checklist at the end of a restore report now names
  `REET_LIBRARY_MANAGED_REMOVALS`, a setting the server reads that the checklist
  had missed. The test that compares the checklist with what the server reads
  caught it, and was failing on `main` until it was added.
- When a worktree is removed, reet now also brings the repository it was made
  from up to date with main, then prunes the merged local branches. It is on by
  default (owners and admins can turn it off in Settings › Admin › Repository
  maintenance). The folder is only ever fast-forwarded: a folder with
  uncommitted changes, on another branch, mid-merge or diverged from main is
  left exactly as it was. Each run is recorded in Activity as `branch-sync`.
  Needs the new `branch-sync-v1` on the machine; older machines are skipped.
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
