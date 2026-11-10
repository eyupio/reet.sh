# Changelog

Changes that matter to someone running a reet controller, newest first. Before
this file existed releases were described on their GitHub release pages.

## Unreleased

- Factory: Gaffer can report across an outcome by itself when a pipeline
  finishes or stops (your choice when approving, bounded, proposals only).
  Effort choices are now the agent's own levels as its machine reports them, so
  OpenCode can be given one. An added pipeline can have a crew, and adding
  pipelines is no longer refused because another pipeline moved. Needs a
  controller update.
- Factory: when you approve pipelines Gaffer proposed, each one can be given its
  own agent, model and effort. Left alone it uses the outcome's.
- The Library now tells you when a connected machine holds a different version
  of a skill than the copy reet has saved, and lets you choose. **Use that
  machine's version** saves its files as the skill's next version, in the same
  entry, so every machine the skill is selected for receives it. **Keep reet's
  copy** leaves reet's copy as it is and stops flagging that machine until its
  files change again. Nothing changes until you choose. A machine that still
  has an older version reet saved is not flagged: it is only behind. Adds
  `differences` to `GET /api/v2/library` and
  `POST /api/v2/library/{item}/difference`. Needs a controller update.

- Factory: a pipeline no longer stops for good when its agent asks for an
  approval. It shows **Needs you** with the reason and carries on by itself once
  you answer. An outcome can be approved to run on **YOLO** (off by default,
  confirmed each time, only where the account, machine and agents allow it). The
  agent lists now offer the machine's real **models**, and an **effort** for the
  outcome or per crew role. Home marks a session that
  is part of a pipeline (**Factory · stage n of m**). Needs a controller update.
- The skills you own are now called your library everywhere you read about
  them. The same collection used to be called the shelf, your skill shelf, My
  shelf, your personal shelf and your private toolkit, depending on the page;
  buttons now read "Add to library" and "Remove from library", and the tools
  and layout switch are "Library tools" and "Library layout". The status "On
  the shelf" is now "Parked", including in the Markdown and CSV exports.
  Choosing which skills a machine has is one button with one name, "Skills on
  this machine", on the machine card, in Home's machine menu and on Sync; the
  second menu entry that opened the same dialog is gone. The dialog explains
  that each AI tool keeps its skills in its own folder and opens on the first
  folder instead of an empty choice. "Sync to machines…" on a skill card, which
  led to a page that said everything was in sync, is now "Choose machines" and
  goes to Machines. Words only: nothing stored on the controller, no setting
  and no API field or value changes, and exported file names stay the same.

- reet now saves a copy of every skill a connected machine shares, by itself.
  When a machine checks in, the controller asks it for the files of each skill
  the library lists without content, and the copy fills that same library entry:
  one entry per name, with its status kept. Before, such skills waited on
  "Waiting for complete content" until someone imported them by hand, and that
  import added a second entry of the same name. Nothing is installed anywhere
  as a result; choosing machines stays your decision. A copy with files that
  need a look still stops for review, and reet tries a skill three times before
  leaving it to a manual import. Needs a controller update.

- A skill you selected for a machine in the library is no longer described as
  "Not set to sync" on the Skills page, and a machine is no longer described as
  "Everything in sync" while a selected skill cannot be installed. The Skills
  card, the machine card and Sync now read the library selection: they say the
  skill is selected, which machine it is for, and why it is waiting (reet has no
  saved copy yet, it is not In use, or two selected skills share a name). Sync
  state gains `held` on each target. Needs a controller update.

- `reet agent status` and the terminal connection wizard now say why a machine
  is not syncing or connected: the controller's refusal or the network error,
  what to do next, and where the log is. Before, the wizard only said the
  private connection had not become ready. In the `reet` terminal home, commands
  that prompt (Upgrade, Install, Connect machine, Sync and others) now run on
  reet's own screen and return home, without dropping into shell scrollback, and
  **Connect machine** on a machine that is already connected says so. Machines
  need the new build to report reasons; no controller update is needed.
- Factory: each outcome and pipeline now shows what Gaffer's own model has used
  (reviews and tokens) and what is left of the stage's budget. Tokens only; reet
  attaches no prices and does not see the working agents' usage. **Ask Gaffer for
  a report** gives one account across an outcome's pipelines and may propose
  pipelines, which are added only when you approve them. A pipeline that did not
  start can be **tried again**. **Look after** beside a session's Gaffer chat box
  starts at once when pre-approved. Wording is unified: look after, goal,
  following, needs you. Adds `POST /api/v2/gaffer/outcomes/{id}/review`,
  `POST /api/v2/gaffer/outcomes/{id}/sessions` and
  `POST /api/v2/gaffer/outcomes/{id}/sessions/{key}/retry`. Needs a controller
  update.
- **Factory** is now a page of its own, in the main navigation. Each outcome you
  give Gaffer is drawn as pipelines that move through plan, design, code, review,
  test, pull request, babysit and merge, with the agent and model doing each stage,
  reviews used, and what is waiting on you. When you approve a plan you choose the
  crew (which agent and model does the thinking, building and shipping stages) and
  the stages each pipeline runs. The controller starts the next stage when Gaffer
  hands one over. A pipeline stops at **Ready to merge** until you approve it,
  unless you allowed that outcome to merge by itself. Account → Gaffer AI gains
  **How Gaffer works**: looking after sessions is pre-approved by default, and
  auto-merge for new pipelines is off by default. Adds
  `POST /api/v2/gaffer/outcomes/{id}/sessions/{key}/merge` and two profile
  columns, created on start. Needs a controller update.
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
