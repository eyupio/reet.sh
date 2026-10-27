# Projects

A Project is a named set of repository folders on your machines, with the agent and model each one
starts with. **Start all** starts one agent session per repository, each through the same request a
single [Start an agent session](start-agent-session.md) sends, and by default each in its own git
worktree and branch. The pages are `web/src/lib/ProjectsPage.svelte`, `ProjectPage.svelte` and their
dialogs; the controller code is `internal/api/projects.go`, `internal/api/project_runs.go`,
`internal/store/store_projects.go` and `internal/projectrun`. What a worktree is on the machine, and
the exact git commands, are in [Worktrees](terminal-worktrees.md). The design is in
[the Projects spec](superpowers/specs/2026-10-25-projects-design.md).

A started repository means only that the machine started the program in tmux in that folder. It does
not show that the agent is signed in, understands the prompt or is making progress, that its branch
holds anything, or that the repository's hooks, build or tests are safe to run.

## Who sees it

Only an account in accounts mode with a write role. The **Projects** entry sits beside Terminals in
the main navigation. Viewers, API and MCP tokens, single-token and open modes don't get the entry,
and the routes refuse them:

| Who | Status and code | Sentence |
| --- | --- | --- |
| An instance without accounts (single-token or open mode), or any request carrying a token | 403 `accounts_required` | "Projects need an account signed in through the browser." |
| Accounts mode, no signed-in session and no token | 401 `signed_out` | "Sign in to use Projects." |
| Viewer | 403 `read_only` | "This account is read-only, so it can't use Projects." |
| A change from another address | 403 `origin_refused` | "That request didn't come from this reet's own address." |

Opening `/projects` directly as a viewer or without an account shows the matching sentence and sends
no Projects request. Starting and retrying also need remote terminals switched on: otherwise 403
`terminals_disabled`, "Remote terminals are switched off on this server, so agents can't be started."
Every reply is `Cache-Control: no-store`.

`projects` is a reserved hosted username: `/projects/<id>` is the app's Project page, not a hosted
skill page. Anyone who had claimed `projects` as a publishing name can no longer reach their hosted
pages through it, and new claims of it are refused. Signing in through single sign-on from
`/projects` or a Project page returns there.

## Limits

| What | Limit |
| --- | --- |
| Projects per account | 50 (409 `project_limit`) |
| Repositories per Project | 12 (422 `member_limit`) |
| Name | 1 to 80 characters after trimming, no control, format, line or paragraph separator characters, unique per account ignoring case (409 `name_taken`) |
| Folder | an absolute path of at most 4,096 bytes, plain text; saved cleaned (`//`, `/./` and `..` collapsed). A machine and folder can be in a Project once (422 `duplicate_member`) |
| Agent | `claude`, `codex`, `opencode` or `gemini`, for the default and for each repository |
| Model | letters, digits and `. _ : / @ + -`, up to 100 characters, starting with a letter or digit; optional |
| Shared prompt | at most 2,000 bytes, valid UTF-8, no control characters except newline, not starting with `-`; when any repository starts Claude Code or Codex (or, for a Project with no repositories, when the default agent is one of them) it must contain a space or a line break, which is what "at least two words" means in the check (422 `prompt_unsupported`) |
| Runs listed | the newest 20 per Project |
| Body | 128 KiB (413 `too_large`); exactly one JSON object with no unknown fields (400 `invalid_request`) |
| Sessions | 8 open per machine and 32 per account, as for any terminal session |
| In flight | at most 2 repositories of one run, and 1 create per machine across all runs |

A repository's model is its own; otherwise the Project's default model, but only when the repository
uses the default agent (a default model belongs to the default agent); otherwise the agent's own
default.

## Routes and shapes

All routes are under `/api/v2`, need the access above and answer refusals as
`{error, code, field?, message}` with one plain sentence. A field refusal names the field
(`name`, `defaultModel`, `sharedPrompt`, `members.N.dir`, `members.N.host` and so on), and the dialog
moves focus to it.

### Projects

- `GET /projects` → `{projects: [Project]}`. Each has `lastRun: {id, state, startedAt}` when it has
  run. **No shared prompt**, only `hasSharedPrompt`.
- `POST /projects` with `{name, defaultTool, defaultModel?, sharedPrompt?, useWorktrees?, members?}`
  → 201 with the Project. `useWorktrees` defaults to true. Each member is `{host, dir, tool, model}`;
  its position is its place in the list.
- `GET /projects/{id}` → the Project **with** `sharedPrompt`, for the owner's edit form, and an
  `ETag` header: the version, quoted (`"4"`).
- `PATCH /projects/{id}` with any of the create fields. Absent or `null` keeps the saved value, `""`
  clears `defaultModel` or `sharedPrompt`, and `members`, when present, replaces the whole list
  (`[]` removes every repository). Answers the Project (no prompt) and its new `ETag`.
- `DELETE /projects/{id}` → `{ok: true, mayHaveWorktrees, message}`. Runs are kept. When any run of
  the Project, however old, recorded a worktree, the message is "Project deleted. Its runs are kept.
  Worktrees those runs made, if still there, stay on their machines." Only worktrees a run *recorded*
  count: one whose create reply was lost, or answered `worktree_mismatch`, has no recorded folder,
  and Delete doesn't mention it.

A Project is `{id, name, defaultTool, defaultModel, useWorktrees, hasSharedPrompt, sharedPrompt?,
members: [{host, dir, tool, model, position}], createdAt, updatedAt, version, lastRun?}`. `host` is
the machine's enrollment id (its agent id), never its display name.

**If-Match.** `PATCH`, `DELETE` and starting a run take an optional `If-Match` with the version that
was read, bare or quoted. With it, the store compares versions inside the same transaction and a
different version answers 409 `project_changed`, "Someone changed this Project. Reload it and try
again.", changes nothing and writes no audit row. A malformed value is 400 `invalid_version`.
Without the header the change goes ahead (the last write wins). The web always sends it.

### Runs

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

The page makes the request id once per confirmation (`crypto.randomUUID()`) and resends it unchanged,
with the same positions and version, after a lost reply: "We didn't hear back, so the run may have
started. Try the same request again: it can't start a second run." A Project changed since the sheet
was made closes the sheet, reads the Project again and says "This Project changed since that sheet
was made, so nothing was started. Look it over, then press Start all again."

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

## How a run starts

1. **Confirm.** *Start all* reads the machines and the account's sessions again, and opens the sheet
   only when both answered ("Couldn't check your machines and sessions just now, so Start all can't
   open. Try again in a moment."). The sheet lists every repository: the ones that can start are
   ticked and say their agent, model and "new worktree and branch" or "in the saved folder"; the
   others are unticked, disabled and say "Won't start:" and the reason below. Nothing runs until
   **Start** is pressed.
2. **Start.** One request creates the run and answers at once. The controller starts the members in
   order, at most two at a time and one create per machine, each through the create path a single
   launch uses, with the Project's shared prompt as the opening prompt.
3. **Watch.** The page reads the run every 2 seconds while a member is pending and the tab is
   visible, and stops when none is. Each failed member shows its sentence and **Retry**.
4. **Open.** Each started member has **Open**, which goes to its terminal session. When a run ends
   with exactly one started member, the page offers to open it.

| Why a repository won't start (sheet) | Sentence |
| --- | --- |
| The machine isn't enrolled (offers **Add a machine**) | "That machine isn't enrolled on this account." |
| The machine isn't ready | its readiness sentence, for example "The machine is reconnecting. Existing shells will be kept." |
| No launch requests | "Update reet on that machine to start agents." |
| Agent not installed | "That agent isn't installed on that machine." |
| No worktrees on that machine (Project uses them) | "Update reet on that machine to start agents with worktrees." |
| 8 sessions on that machine, counting the ones this run plans | "That machine already has 8 sessions, the most it can have. End one, then start again." |
| 32 sessions on the account, counting the ones this run plans | "This account already has 32 sessions, the most it can have. End one, then start again." |

The sheet's sentence says what will start and, when the Project has a shared prompt, "The prompt is
kept with the session record and is visible to other users of that machine in its process list." A
Project without worktrees adds: "This Project makes no worktrees. Each agent works in the saved
folder itself, so any other session there shares that checkout and its files."

## Member states and codes

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

## Derivations

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

## The `sent` rule: one agent per repository

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

## Waits and timeouts

| Where | Wait |
| --- | --- |
| Every browser request | given up after 15 seconds |
| Run view | read every 2 seconds while a member is pending and the tab is visible |
| Machines and sessions on the Project page | read again every 15 seconds while the tab is visible |
| A machine that isn't ready when its member's turn comes | 30 seconds, or 2 minutes after a controller restart, without holding a slot; then `machine_unavailable` |
| A worktree create, status or remove through the controller | up to 180 seconds |
| On the machine | 60 seconds for a worktree create (plus 30 for cleanup) or a status; a remove takes at most 60 seconds in all (30 for its checks, 30 for `git worktree remove`) |

One create per machine at a time means a 12-repository run on one machine can take about 36 minutes
(12 worktree creates of up to 3 minutes each) even when nothing else is going on. That is a lower
bound of the worst case, not a ceiling: the per-machine create slot is shared with every other run on
the account, and waits for a machine that isn't ready come on top. A remove can take longer on the
machine than the browser waits: the dialog then says "We didn't hear back, so the worktree may have
been removed. Check again.", and **Check again** reports what is there.

## Where the prompt lives

The shared prompt is kept in the controller's database with the Project, copied onto each run when
it starts (so the run keeps the prompt it used after the Project changes or is deleted), and sent to
each machine as that repository's opening prompt. From there it is where a single launch's prompt
is: the controller's create-operation row, the machine's private registry, and the machine's process
list, where other users of that machine can see it.

It is **never** in the Projects list, a save reply, a run, an audit row, a log line, an error, browser
storage or an address. Only the single-Project read returns it, for its owner's edit form. An edit
sends it only when it changed; a save reply leaves it out.

## Shared checkouts

A Project with worktrees switched off starts each agent in the saved folder itself. Any other run or
session in that folder shares the same checkout and files. The sheet, the run view (each member's
`sharesCheckout`) and the Project page say so.

## Machines that are merged or revoked

A member names its machine by enrollment id. Merging or revoking a machine rewrites no saved Project
or run. Members of the old enrollment are listed as "A machine no longer on this account", are
unticked on the sheet with **Add a machine**, and if started fail `unknown_machine` with nothing sent.
Change the Project to name the remaining machine. A repository already saved on a revoked machine
can be kept unchanged in an edit; a new folder on it is refused (422 `unknown_machine`).

## Remove worktree

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

`project-create`, `project-update` (with `previousName` after a rename) and `project-delete` record
the Project's id, name and repository count. `project-run` records the Project, run and repository
count; `project-run-retry` the Project, run, position and attempt. Each started member writes the
existing `terminal-launch` row with the worktree token, Project slug, branch and folder, and a remove
writes `worktree-remove`. None carries the prompt. Activity reads them as, for example, "Created
project Site with 3 repositories" and "Tried repository 2 of a Project run again (attempt 2)".

## Known limits

- **One controller process.** Member claims and per-machine slots are held in memory. Two
  controllers on one database could start a member twice.
- **Branch-era databases.** The Project tables changed shape before release. A database made from
  earlier commits of the Projects branch must be recreated. A database from main before Projects
  gains the new tables when it is opened.
- **Same repository, two folders.** Two saved folders that resolve to one repository on the machine
  are not caught; they get separate worktrees and branches.
- **A lost create reply.** If a worktree create's reply never arrives, the session is found by
  reconciling, which doesn't carry the worktree's folder or repository name. The run then can't name
  that worktree, so *Remove worktree* isn't offered for it; remove it on the machine by hand. Deleting
  the Project doesn't mention such a worktree either, nor one whose create was answered
  `worktree_mismatch`: Delete's warning covers only worktrees a run recorded.
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
