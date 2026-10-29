# Projects

A **project** is one folder on one machine, kept in the project rail on Home. Choosing a card puts it
in Home's launch bar with its own defaults ([Start an agent session](start-agent-session.md#on-home-the-launch-bar));
a session started there is listed under the project in In flight. A project can carry an optional **starter prompt**,
a default agent, model and effort, a default base branch, whether each session gets its own worktree, and a
rail **colour** (blank, red, amber, green, blue or violet) that marks it in the rail and, by default,
the sessions started in it.
There is no separate Projects page: `/projects` and `/projects/<id>` go to Home.

The rail is `web/src/lib/home/ProjectRail.svelte`, the Add and Edit dialog `AddProjectDialog.svelte`
with `FolderBrowser.svelte` (how a machine lists its folders is in [Folder browse](folder-browse.md)),
the Worktrees list `ProjectWorktrees.svelte`. The controller code is `internal/api/folder_projects.go`,
`internal/store/store_folder_projects.go` and, for the move from saved Projects,
`internal/store/store_projects_migrate.go`. The design is in
[the agents home spec](superpowers/specs/2026-10-26-agents-home-design.md) ("Project registry",
"Migration"); the multi-repository Projects it replaces are in
the Projects spec (removed in the docs tidy-up; see git history before bd9e56c).

A started session means only that the machine started the program in tmux in that folder. It does
not show that the agent is signed in, understands the prompt or is making progress.

## Who sees it

Only an account in accounts mode with a write role sees the rail. Viewers, API and MCP tokens,
single-token and open modes get Home with the fleet strip alone, and the routes refuse them:

| Who | Status and code | Sentence |
| --- | --- | --- |
| An instance without accounts, or any request carrying a token | 403 `accounts_required` | "Using projects needs an account signed in through the browser." |
| Accounts mode, no signed-in session and no token | 401 `signed_out` | "Sign in to use projects." |
| Viewer | 403 `read_only` | "This account is read-only, so it can't use projects." |
| A change from another address | 403 `origin_refused` | "That request didn't come from this reet's own address." |

Every reply is `Cache-Control: no-store`. `projects` stays a reserved hosted username.

## Limits

| What | Limit |
| --- | --- |
| Projects per account | 100 (409 `project_limit`, "You have 100 projects, the most reet keeps. Remove one first.") |
| A machine and folder | once per account (409 `project_exists`, "studio already has a project for that folder.") |
| Name | 1 to 80 characters, plain text, unique per account ignoring case (409 `name_taken`); blank gives the folder's name, with ` (2)`, ` (3)`… on a clash |
| Folder | an absolute path of at most 4,096 bytes, saved cleaned |
| Base branch | blank or a valid git branch name of at most 255 bytes |
| Worktree for each session | `inherit` (the account default), `yes` or `no` |
| Colour | blank, or `red`, `amber`, `green`, `blue` or `violet` (422 `invalid_colour`) |
| Starter prompt | the opening-prompt rules: at most 2,000 bytes, valid text, no control characters but line breaks, not starting with `-`; for Claude Code and Codex at least two words |
| Writes | 30 a minute per account (429 `project_rate_limited`) |
| Body | 64 KiB (413 `too_large`); exactly one JSON object with no unknown fields (400 `invalid_request`) |

A saved starter prompt that today's rules refuse (one carried over from a saved Project) does not
block other edits: its shape is checked again only when it changes. The two-words rule depends on
the agent too, so it is checked again when the prompt **or** the default agent changes ("Write the
starter prompt as at least two words.").

## The rail

Desktop: the left column, 301 px (272 px cards inside 14 px padding and a 1 px rule). From 1100 to
1239 px wide it is a 56 px strip: **Add**, **Open the project rail** (marked with a dot, and named, when a
migration note or folders are waiting), then one button per project showing its initial, its base
branch in mono (cut short), the repository diamond, a running dot and the worktrees in use as a small count,
each named in full (the chosen one is marked current). A card's menu (its button, or a right-click on the
card) sets the project's colour; the colour is saved on the project and follows the person to every
browser. Open the project rail shows the full rail over the page as a 301 px panel starting where the strip is on screen, with every card menu, note and prompt and a
Close button; while it is open the toggle is named "Close the project rail", Tab stays inside the panel,
and Escape, Close, the toggle or a press outside closes it and gives focus back to the toggle. On a tablet or phone
it is on the Sessions screen, under the session groups (PR H).

Each card shows, in words: a repository diamond (or a folder icon for a plain folder), the name, the
machine, the base branch (mono) when one is set, the state and the worktrees in use. The state is
"N running" (its own running and starting sessions, by each session's project id), "machine offline"
when its machine is known not to be reporting, else "idle". Worktrees in use are the distinct
worktrees its live sessions are working in; the session listing has no record of removed worktrees,
so a finished session's worktree is never counted, and nothing shows at 0. The fleet cards show the
same figure per machine ("no worktrees in use" included). The foot reads "Folders are browsed live on
the machine".

Choosing a card, or **New session here** in its menu, sets the launch bar's project: its default agent
and model, its base branch and its worktree choice go into the sentence, and focus moves to the prompt.
The start carries the project's id, so the session is listed under it. While the bar has a start out
or kept for a retry, a card changes nothing and the bar's status line says why. Each card's menu (its button shows on hover or
keyboard focus with a mouse, always on touch; arrow keys move, Escape closes): **New session here**,
**Edit**, **Worktrees**, **Move up**, **Move down** and **Remove from reet**.

- **Add** opens the folder browser on the first machine that can browse; a machine that cannot (it is
  offline, runs an older reet, or has browsing switched off) still takes a typed path, checked again
  at launch. Success: "Sorted. reet added." in Home's status line.
- **Edit** reads the project again with its starter prompt and sends only what changed, naming the
  version it read. Success: "Sorted. reet saved." When someone else saved first, the dialog says
  "Someone changed this project. Reload it, then try again.", locks the fields and offers **Load the
  latest version**: the latest version fills the fields, the person's own changes are listed under
  *Your changes*, and **Use my changes again** puts them back on top of it. Nothing is saved until
  **Save changes**, which names the latest version. The stale version is never sent again.
- **Worktrees** lists the worktrees reet has a record of for that folder: the ones its live sessions
  are in (with **Open the session**), and the ones old Project runs made in the same folder on the
  same machine, each with **Check and remove** (below). Nothing else is listed: reet never guesses.
- **Move up** and **Move down** send the whole order with the version of each project as read; a
  stale list is 409 `project_changed`, said as sent, and the list is read again.
- **Remove from reet** asks first ("Remove reet from reet?" — "It leaves the rail. Its sessions and
  any worktrees stay on studio, and the folder isn't touched."), then answers "Project removed. Its
  sessions and any worktrees stay on studio." Only the registry entry goes.

**Select projects** in the rail heading's menu, or a long press on a card on touch, turns the cards
into checkboxes for **Start in several projects** (2 to 12 projects; the 56 px strip shows how many
are chosen). The launch bar then starts them as one run, each project from its own base branch (else
the account's default), with a status sheet that counts what started and offers Retry per member.
How it works is in [Start an agent session](start-agent-session.md#start-in-several-projects).

`/?project=<id>` marks that project's card and puts it in the launch bar. Jump finds
projects by name and opens that address.

## Routes and shapes

All under `/api/v2`, with the access above; refusals are `{error, code, field?, message}` with one
plain sentence, and a field refusal names the field so the dialog can move focus there.

- `GET /folder-projects` → `{projects: [FolderProject], savedWaiting}` in rail order. No starter
  prompt, only `hasStarterPrompt`.
- `GET /folder-projects?legacy=<saved Project id>` → `{id}`, the folder project made from that saved
  Project's first folder still on the account. Unknown ids, another account's and removed ones are the
  same 404 `project_not_found`.
- `POST /folder-projects` with `{host, dir, isRepo?, name?, baseBranch?, worktreeDefault?,
  defaultTool?, defaultModel?, defaultEffort?, starterPrompt?}` → 201 and an `ETag`. `host` is the machine's
  enrollment id. `defaultTool` accepts any agent reet can start (including Cline); a default model needs
  a default agent, and a default effort needs an agent whose preset takes one.
- `GET /folder-projects/{id}` → the project **with** `starterPrompt`, for the edit form, and its
  `ETag` (the version, quoted).
- `PATCH /folder-projects/{id}` with `If-Match` and any of the create fields: absent keeps, `""`
  clears; `{"note": ""}` clears the migration note (any other note is 422 `invalid_note`). A stale
  version is 409 `project_changed`.
- `DELETE /folder-projects/{id}` with `If-Match` → `{ok, message}`.
- `POST /folder-projects/reorder` with `{ids, versions}` naming every project once → the list.
- `POST /folder-projects/import-saved` with `{}` → the list plus `{created, skipped, message}`.

A FolderProject is `{id, host, machine, dir, name, isRepo, baseBranch, worktreeDefault, defaultTool,
defaultModel, defaultEffort, hasStarterPrompt, starterPrompt?, note, position, createdAt, updatedAt, version}`.
`machine` is the machine's current name, `""` once it is no longer enrolled (the card says "machine
not enrolled"). A terminal create may carry `projectId`; the controller checks it belongs to the
account and to that machine (404 `project_not_found`, 422 `project_machine_mismatch`) and never
sends it to the machine.

## Session and profile fields

The session listing (`GET /api/v2/terminal/sessions`) now carries, for every session, `tool`, `model`,
`effort` (always `""` for now), `yolo` (always `false` for now) and `projectId` (the folder project it
was started in, `null` otherwise), and never the prompt. The profile (`GET`/`PATCH /api/v2/profile`)
gains `defaultBaseBranch` (blank, the repository's own default, or a valid branch name),
`worktreesByDefault` (true unless changed): the account default a project's `inherit` means, and
`defaultTool`, `defaultModel`, `defaultEffort` for a session started outside a project. Model
favourites are per account under `/api/v2/profile/model-favourites`; see
[agent models](agent-models.md). No page sets these yet: they are set through the API, and the
launch bar (the next PR) reads them.

## Starter prompts

A starter prompt lives in the project record only: never in the audit, logs, browser storage or an
address. The list never carries it. The launch bar offers it as **Use the starter prompt** while
the prompt box is empty; reet never puts it in the box, or sends it, by itself.

## Moving saved Projects

Saved multi-repository Projects became folder projects when the controller first started with this
release, once and idempotently: one transaction per controller start, with a savepoint per account so
one account's failure is rolled back alone and tried again at the next start (the rail's import below
is its own transaction):

- Every distinct machine and cleaned folder across an account's saved Projects became one folder
  project, oldest Project first. The name is the folder's (a Project with one repository lends its own
  name when it is free); the member's own agent and model, else the Project's defaults (a member that
names its own agent but no model gets that agent's own default, an empty model, not the Project's
default model); `useWorktrees`
  became `yes` or `no`; the shared prompt became the starter prompt.
- A folder the person had already registered was left as it was. Where one folder belonged to several
  Projects with different prompts, the oldest Project's prompt was kept and the card shows a one-time
  note: "Moved from your saved Projects. The starter prompts of A, B weren't kept here; their old runs
  are in Activity." **Dismiss** clears it for good. "Shown once" means exactly this: the note stays on its card until
  the person dismisses it, and once cleared on the server it never comes back.
- Each Project wrote one `projects-migrated` event naming the folder projects made, never the prompt.
- An account with the most projects reet keeps was left with the rest waiting. The rail then says
  "N folders from your saved Projects aren't in the rail yet." with **Add the waiting folders (N)**;
  nothing comes in by itself, not even after a remove. (The wording holds whether or not there is room
  now: see the downgrade limit below.) N counts folders (machine and
  cleaned folder), not Project members: several Projects naming one folder count once, and a folder
  that is already a project, or one reet can't use, is not counted. The answer is shown as sent:
  "Added N saved repositories as projects.", "N saved repositories are waiting for room. Remove a
  project, then try again." or "Nothing is waiting to be added: every saved folder is already in reet,
  or can't be used as a project."
- The old `projects`, `project_members` and run tables are kept, unchanged, for one release. Their
  create, change and delete routes answer 410 `projects_moved`, "Projects now live in the rail on
  Home."
- `/projects` and `/projects/<id>` are client-side redirects to `/`, and `/projects/<id>` to
  `/?project=<folder project id>` when one was made from it. Home then says "Projects now live in the
  rail on Home." once, for that page load (put into the status line after the page is drawn, so it is
  announced); nothing is stored to say it again. Viewers and other sign-ins without a rail are told
  "Projects are on Home for accounts that can start sessions." instead. On a tablet or phone, where the
  rail is below In flight, the chosen card is scrolled into view and named ("Showing reet in Projects
  below.").

## Runs from Projects

The runs of saved Projects are kept to read. Activity (in your account at `/account/activity`) lists them for a writer, newest first, under
*Runs from Projects*; each opens its run (read-only: no Retry). A worktree a run recorded can still be
checked and removed, there or from the folder project's **Worktrees** menu. Viewers see no runs.

The rest of this section, and the run limits under Known limits, describe how those runs were started
and what their records mean. The confirmation sheet, Start all, Retry and Delete they mention are no
longer on any page.

### Run routes

- `GET /projects/{id}/runs` → `{runs: [Run]}`, the newest 20, newest first.
- `GET /project-runs/{runId}` → the Run. A run stays readable after its Project is deleted.
- `POST /projects/{id}/runs` with `{requestId, positions}` and `If-Match` → 201 with the new Run,
  every member `pending`. The members are started in the background; closing the page does not stop
  them.
- `POST /project-runs/{runId}/members/{position}/retry` with `{}` → 200 with the Run.

A Run is `{id, projectId, projectName, state, startedAt, useWorktrees, worktreeProject, members}` and
never has a prompt field. `state` is `running` while any member is pending, `complete` when every
member started, `failed` when none did and `partial` otherwise (a removed member counts as started).
A member is `{position, host, dir, tool, model, state, code, sentence, sessionId, worktreePath,
worktreeRepo, worktreeBranch, worktreeRun, worktreeProject, sharesCheckout, attempt}`.

Starting a run:

| Answer | When |
| --- | --- |
| 201 | a new run |
| 200 with the same run | the same `requestId` and positions again, while the Project still exists; with a stale `If-Match` too (the replay is looked up before the version is compared) |
| 400 `invalid_request` | not exactly one JSON object with only `requestId` and `positions` |
| 400 `invalid_version` | a malformed `If-Match` |
| 403 `terminals_disabled` | remote terminals are switched off on the server |
| 404 `not_found` | the Project isn't on this account, including a replay after the Project was deleted (the run still exists and goes on) |
| 409 `project_changed` | `If-Match` names a version that is no longer saved; nothing is created or sent |
| 409 `request_changed` | the same `requestId` with different positions: "That request changed. Start it again." |
| 409 `run_in_progress` | an earlier run of this Project still has pending members: "This Project is still starting. Wait for it to finish, then start again." |
| 413 `too_large` | a body over 128 KiB |
| 422 `request_id_required` | no request id, or not 16 to 128 letters, digits, hyphens or underscores |
| 422 `no_members` / `unknown_position` | no positions, or one that isn't in the Project or is named twice. Without `If-Match`, or with the current version, a replay whose positions have since been removed from the Project also gets `unknown_position` |

No page of this release sends a start or a retry. The routes stay for one release, so a browser tab
opened before the upgrade can still start and retry its saved Projects' runs; those sessions carry no
project id and are not listed under a folder project. The routes go with the old tables.

Retrying a member:

| Answer | When |
| --- | --- |
| 200 with the run | the member is pending again (same attempt while its create may have been sent, the next one otherwise) |
| 400 `invalid_request` | a body other than `{}` |
| 403 `terminals_disabled` | remote terminals are switched off on the server |
| 404 `run_not_found` | the run isn't on this account: "That run isn't on this account." |
| 409 `not_failed` | the member isn't failed: "Only a repository that failed can be tried again." |
| 413 `too_large` | a body over 128 KiB |
| 422 `unknown_position` | the position isn't a whole number in the run |

### Worktrees of a run

*Remove worktree* uses the existing routes `POST /terminal/worktrees/status` and `/remove` (see
[Worktrees](terminal-worktrees.md#the-api)) with `agentId` = the member's `host`,
`project` = `worktreeProject`, `repo` = `worktreeRepo` (the name the machine reported, never one
recomputed from the saved folder) and `run` = `worktreeRun`. A successful remove moves the member to
`removed`.

### Member states and codes

| State | Words |
| --- | --- |
| `pending` | "Waiting to start." |
| `started` | "Started." |
| `failed` | the sentence for its code, below |
| `removed` | "Started. Its worktree has been removed." |

| Code | Sentence |
| --- | --- |
| `unknown_machine` | "That machine isn't enrolled on this account." |
| `machine_unavailable` | "That machine is reconnecting. Try again in a moment." |
| `launch_unsupported` | "Update reet on that machine to start agents." |
| `worktree_unsupported` | "Update reet on that machine to start agents with worktrees." |
| `tool_not_installed` | "That agent isn't installed on that machine." |
| `folder_unavailable` | "That folder isn't available on that machine." |
| `prompt_unsupported` | "Write the opening prompt as at least two words." |
| `session_limit` | "The terminal session limit has been reached. End a session and try again." |
| `not_repo_root` | "That folder isn't the top of a git repository." |
| `repo_busy` | "That repository is in the middle of a merge, rebase or similar. Finish or abort it first." |
| `worktree_exists` | "A worktree or branch with this run's name is already on that machine, so nothing was started." |
| `outside_home` | the `~/.reet/worktrees` sentence in [Worktrees](terminal-worktrees.md#refusals-and-sentences) |
| `git_failed` | "Git couldn't make a worktree there." |
| `worktree_refused` | "That machine refused to make a worktree for this request. Start it again to see why." |
| `worktree_mismatch` | "That machine's answer did not match the request. Check again." The machine reported a worktree folder that isn't this run's. The agent **may be running** (its session is listed without a worktree folder); look at your sessions. A run member has no Check again, only **Retry**, which sends the same request id and starts nothing new while the create may have been sent. |
| `unknown_outcome` | "We didn't hear back, so the agent may have started. Check your sessions before retrying." |
| `session_lost` | "That machine lost the session before it started. Start it again." |
| `machine_refused` | "That machine refused to start the agent. Check the machine, then try again." |
| `terminals_disabled` | "Remote terminals are switched off on this server, so agents can't be started." |
| `invalid_launch` | "reet couldn't send that start request. Check the Project's settings and try again." |
| `internal_error` | "Something went wrong on our side. Check your sessions, then retry." |

Any other code reads "Something changed. Check again."

### Derivations

- **Request id** of a member's create: the first 48 hex characters of SHA-256 of
  `run|position|attempt`.
- **Worktree token** (the run token on the machine): the first 24 hex characters of SHA-256 of
  `worktree|run|position`. It is per member, never one per run, so two members on one machine never
  share a worktree; and never per attempt, so a retry adopts the worktree an earlier attempt made.
- **Project slug**: from the Project's name *when the run started* (a later rename doesn't change a
  run's worktrees): ASCII letters lower-cased, digits kept, anything else one hyphen, hyphens trimmed;
  longer than 40 bytes keeps its start plus 8 hex characters of SHA-256 of the name; a name with
  nothing usable becomes `project-` plus those 8 characters.
- **Folder and branch** on the machine: `~/.reet/worktrees/<project-slug>/<repo-slug>/<token>` on the
  new branch `reet/<project-slug>-<token>`, made at the repository's current `HEAD`. `<repo-slug>` is
  made by the machine from the resolved repository folder's name and reported back as
  `worktreeRepo`.
- **Session title**: the Project's name and the folder's last part, "Site · api", at most 80
  characters.

### The `sent` rule: one agent per repository

Just before a member's create is handed to the broker, the member is durably marked *sent* in the
database. The mark is cleared only when that request id is known to have started nothing. **Retry**
reuses the same attempt, and so the same request id, while the mark is set, whatever the last failure
said; it takes the next attempt only when the mark is clear. The store decides this in one
transaction.

This is what keeps a repository to one agent. The machine adopts an existing worktree for a run token
regardless of the request id, so a new request id with the same token would start a second agent in
the same worktree. The same request id is answered by the broker from its reservation, before it
looks at the machine's capabilities, and by the machine with the session it already made.

After a controller restart, members still pending are started again with their unchanged request ids.
A member that was in flight and never reached the machine comes back `session_lost`; one **Retry**
starts it.

### Waits and timeouts

| Where | Wait |
| --- | --- |
| Every browser request | given up after 15 seconds |
| Run view | read every 2 seconds while a member is pending and the tab is visible |
| Machines and sessions (the retired Project page) | read again every 15 seconds while the tab was visible |
| A machine that isn't ready when its member's turn comes | 30 seconds, or 2 minutes after a controller restart, without holding a slot; then `machine_unavailable` |
| A worktree create, status or remove through the controller | up to 180 seconds |
| On the machine | 60 seconds for a worktree create (plus 30 for cleanup) or a status; a remove takes at most 60 seconds in all (30 for its checks, 30 for `git worktree remove`) |

One create per machine at a time means a 12-repository run on one machine can take about 36 minutes
(12 worktree creates of up to 3 minutes each) even when nothing else is going on. That is a lower
bound of the worst case, not a ceiling: the per-machine create slot is shared with every other run on
the account, and waits for a machine that isn't ready come on top. A remove can take longer on the
machine than the browser waits: the dialog then says "We didn't hear back, so the worktree may have
been removed. Check again.", and **Check again** reports what is there.

### Where the prompt lives

The shared prompt is kept in the controller's database with the Project, copied onto each run when
it starts (so the run keeps the prompt it used after the Project changes or is deleted), and sent to
each machine as that repository's opening prompt. From there it is where a single launch's prompt
is: the controller's create-operation row, the machine's private registry, and the machine's process
list, where other users of that machine can see it.

It is **never** in the Projects list, a save reply, a run, an audit row, a log line, an error, browser
storage or an address. Only the single-Project read returns it, for its owner's edit form. An edit
sends it only when it changed; a save reply leaves it out.

### Shared checkouts

A Project with worktrees switched off starts each agent in the saved folder itself. Any other run or
session in that folder shares the same checkout and files. The run view says so for each member
(`sharesCheckout`).

### Machines that are merged or revoked

A member names its machine by enrollment id. Merging or revoking a machine rewrites no saved Project
or run. Members of the old enrollment are listed as "A machine no longer on this account", are
unticked on the sheet with **Add a machine**, and if started fail `unknown_machine` with nothing sent.
A saved Project can no longer be changed (410 `projects_moved`); its folder project is changed instead:
Edit, then choose the remaining machine (PATCH `host`). A folder project already on a revoked machine
can be kept unchanged in an edit; moving it to a new folder on that machine is refused (422
`unknown_machine`).

### Check and remove a run's worktree

reet never removes a worktree by itself, apart from undoing its own failed create: when a create
fails part-way, the machine removes the worktree and deletes the branch that same call had just made,
and nothing else (see [Worktrees](terminal-worktrees.md)). *Remove worktree* is offered only for a member that started
with a worktree and whose session the page has read as ended:

- while the sessions are not read yet: "Checking whether its session has ended…";
- when they couldn't be read: "Couldn't check whether its session has ended, so its worktree can't be
  removed from here just now. Reload the page to try again.";
- while it runs: "End its session before removing its worktree."

The dialog reads the worktree's status first and shows the folder, branch, saved folder and counts.
**Remove** is offered only when nothing blocks it; otherwise only **Check again**:

| Blocks removal | Sentence |
| --- | --- |
| Uncommitted changes (always; up to 20 paths shown, then "and N more not listed.") | "This worktree has changes you haven't saved. Commit, stash or discard them in the worktree, then check again." |
| Commits on no branch | "Some commits in this worktree are not on any branch. Put them on a branch first, then check again." |
| Edits git is told not to check | "Git is not tracking some edits in this worktree. Review them before removing." |
| Another repository inside | "This worktree contains another git repository. Move or remove it first." |
| The check was incomplete (large trees) | "reet couldn't check all of this worktree. Remove it yourself." |

When nothing blocks, the dialog states the counts, that unpushed commits stay on the branch, that
ignored files such as `.env` and `node_modules` are deleted with the folder, and that the branch is
kept. The button names the unpushed count ("Remove with 1 unpushed commit"). Just before removing,
the dialog reads the sessions again and refuses if the session runs again ("Its session is still
running, so nothing was removed. End its session first.") or can't be read. The remove carries the
counts that were shown; if the machine now sees different ones it refuses with "That worktree changed
since you looked. Check again." Other refusals are the worktree sentences: "Git doesn't list that
folder as this run's worktree, so reet won't touch it.", "That worktree isn't on that machine. It may
have been removed already.", "Git couldn't check that worktree." or "Git couldn't remove that
worktree." with git's first line shown as "Git said: …". A second status or remove on a machine while
one runs is 409 `worktree_busy`. Success says "Sorted. The worktree at … is removed. The branch … is
kept." *Remove worktree* never deletes the branch.

## Audit

`project-add`, `project-update` and `project-remove` record the project's id, machine (id and name),
folder and name; `projects-migrated` the saved Project's name and the folder projects made from it;
`folder-browse` one row per opened browser (machine, starting folder, pages). None carries a starter
prompt. Activity reads them as, for example, "Added project reet (/home/me/reet on studio)", "Removed
project reet; its folder on studio is untouched", "Moved saved Project Site into the rail as 2
projects" and "Browsed folders from ~ (3 pages)". The old runs' `project-run`, `project-run-retry`,
`terminal-launch` and `worktree-remove` rows are unchanged.

## Known limits

- **Worktrees in use, not worktrees made.** No record says which worktrees reet made and has not
  removed, so the rail and fleet cards count only those live sessions are in. A worktree left by an
  ended session (outside an old run) is not listed anywhere.
- **YOLO not yet.** The launch bar shows YOLO disabled and the controller refuses it (409
  `yolo_refused`), for one project and for several, until a later release.
- **Old runs listed: the newest 20 per saved Project** (what the run list answers); older runs and their
  worktrees are not listed.
- **Rail menu reorder only.** Moving is one place at a time from the menu; there is no drag.
- **Waiting needs the person.** Saved folders left waiting come in only from the rail's prompt.
- **Downgrade, then upgrade again.** `reet upgrade --version` can go back to a release without folder
  projects. Saved Projects created or changed while downgraded are not migrated at the next upgrade
  (the migration has already run); their new folders show as waiting in the rail, even when there is
  room, and come in with **Add the waiting folders**. Changes to folders already moved are not carried
  over.
- **A session's project is checked for account and machine, not folder.** A start may name any of the
  account's projects on that machine, whatever folder it starts in; the rail then counts that session
  under the project. The launch bar sends a project's id only with that project's own folder.
- **No running state without sessions.** When the session list could not be read, cards say nothing
  about running, idle or worktrees in use ("machine offline" still shows from the health report).

- **One controller process.** Member claims and per-machine slots are held in memory. Two
  controllers on one database could start a member twice.
- **Branch-era databases.** The Project tables changed shape before release. A database made from
  earlier commits of the Projects branch must be recreated. A database from main before Projects
  gains the new tables when it is opened.
- **Same repository, two folders.** Two saved folders that resolve to one repository on the machine
  are not caught; they get separate worktrees and branches.
- **A lost create reply.** If a worktree create's reply never arrives, the session is found by
  reconciling, which doesn't carry the worktree's folder or repository name. The run then can't name
  that worktree, so *Remove worktree* isn't offered for it; remove it on the machine by hand. The same
  holds for one whose create was answered `worktree_mismatch`: only worktrees a run recorded are
  listed.
- **An agent still running in the worktree** is not checked by the machine; the page offers removal
  only after the session has ended, from the sessions it read.
- **A slow remove** can outlast the browser's 15 seconds; Check again reports the outcome.
- **A slow status can't finish from the page.** The dialog gives up on a status read after 15
  seconds, but the machine allows it 60 and keeps going. Until it ends, **Check again** gets "That
  machine is already working on a worktree. Try again in a minute." (409 `worktree_busy`). A worktree
  whose status always takes longer than 15 seconds (a very large tree, a slow disk) never shows its
  counts, so *Remove worktree* never offers Remove for it. This fails safe: nothing is removed.
  Remove such a worktree on the machine by hand.
- **Busy machines.** While a worktree create runs, that machine's other session operations wait, and
  open terminals on it may detach for a moment.
- **Not checked.** No physical phone or tablet, screen reader, Safari or Firefox; nothing on macOS
  outside CI; Windows isn't supported. See [UI verification](ui-verification.md).
- **Out of scope.** A git remote as a second repository identity, one session spanning several
  repositories, agent status, resuming a conversation, merging, pushing, pull requests, deleting
  branches and MCP exposure of Projects.
