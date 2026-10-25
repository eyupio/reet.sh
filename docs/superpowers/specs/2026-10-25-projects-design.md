# Projects (S7 design)

Date: 2026-10-25. Status: design for review; no code written.

Choose several repositories, press one button, and have an agent working in each. The product owner
calls Projects, with agent and model selection, core to the new reet.sh. "Start an agent session"
(#193, #194, #195) is the single-repository building block; this slice composes it.

## Intent and success

**Outcome.** A signed-in owner with a write role can save a named set of repository folders (on one
or many of their machines) with a default agent and model, start work on all of them in one action,
and keep parallel agents from sharing a checkout by giving each run its own git worktree.

**Who it is for.** The account owner on a desktop or a phone, on machines already enrolled and
consented to browser terminals.

**Success looks like.**

- One confirmation sheet shows exactly what will start; one press starts it; every repository
  succeeds or fails on its own and says so in plain words.
- Retrying a lost request never starts a second agent or a second worktree.
- Nothing is removed from a machine without an explicit press that first shows uncommitted and
  unpushed work.
- Nothing on screen claims more than happened: "started" means the connector started the program in
  tmux in that folder, not that the agent is signed in, understands the prompt or is making progress.

## Decisions already taken with the product owner

| Question | Decision |
| --- | --- |
| What does starting a Project do? | One agent session per repository, each through the existing Start an agent request. |
| How is a repository identified? | A machine plus an absolute folder, picked from folders the machine reports. A git remote may be added later as a second, optional identity without migrating Projects. |
| What is in the first slice? | Projects with defaults and Start all, **and** git worktrees. |
| How do worktrees work? | One new worktree and branch per run, under reet's own folder inside the home boundary; reet never removes one by itself. |

## Out of scope

Git-remote repository identity, one session spanning several repositories, agent status (working,
waiting, idle), resuming a past conversation, merging, pushing or pull requests, a worktree outside
`~/.reet/worktrees`, deleting branches, reet.sh running git for anything but the calls listed in
[Worktrees](../../terminal-worktrees.md) (amended: the one `worktree add`, read-only `rev-parse`,
`for-each-ref`, `worktree list`, `status`, `symbolic-ref`, `rev-list` and `ls-files`, the one
`worktree remove` without `--force`, and on a failed create only `worktree unlock`,
`worktree remove --force` and `branch -D` of what that same call created),
Windows, MCP exposure of Projects, and any change to the security/PIN preference or sync flows.

## The model

A **Project** (per account): `name` (1–80 characters, plain text, unique per account, compared
case-insensitively), `default_tool`, optional `default_model`, optional `shared_prompt` (the same
rules as the opening prompt: at most 2,000 bytes, valid UTF-8, no control characters except newline,
not starting with `-`), `use_worktrees` (default true), up to 12 **members**, created and updated
times. At most 50 Projects per account.

A **member**: `host` (the enrolled machine), `dir` (absolute folder, validated exactly as the Start
an agent folder is), optional `tool` and `model` overriding the defaults, a position. A machine plus
folder may appear in a Project once.

A **run**: created when Start all is confirmed. It records the Project name at that time, who started
it, the time, and for each member: the machine, the folder, the tool and model used, the session id,
the worktree path and branch (when used) and a state (`pending`, `started`, `failed` with a code,
`removed`). A run is a historical record, so renaming or deleting a Project never rewrites it.

The shared prompt is kept in the controller's database and is sent to the machine as each repository's
opening prompt; the existing statements about where a prompt is stored and that it is visible in the
machine's process list apply to every repository of a run and are repeated on the confirmation sheet.

## Starting a run

1. **Confirm.** The sheet lists every member that will start: machine, folder, agent, model, and
   whether a worktree will be made. A member whose machine is not ready, lacks the launch feature, does
   not have the tool installed, or would exceed the per-machine (8) or per-account (32) session
   limits is shown with a plain reason and is unchecked; the person can start the rest. A machine that
   is not enrolled offers *Add a machine*. Nothing runs until the person presses *Start*.
2. **Start.** One API call creates the run and returns at once. The controller then starts the members
   one after another, never more than two in flight, each through the existing create path
   (`Create{Tool, Model, Dir, Prompt}` plus the worktree request below). The browser polls the run
   and shows each member's state; closing the page does not stop the run.
3. **Idempotency.** The request id for each member is derived from the run id and the member position,
   so a retried call (the run, or a single member) maps to the same create request. A changed
   parameter is the existing 409. *Retry* on a failed member reuses its derived id only when the failure
   left the outcome unknown; after a definite refusal it uses a new derived id (attempt number).
4. **Partial failure is normal.** The run is `complete` when every member is `started`,
   `partial` when some failed, `failed` when none started. Each failed member shows the same sentence the
   single-repository dialog would show (`refusalText`), plus the worktree sentences below.
5. **Landing.** The result page lists each started member with *Open*, which goes to the existing
   terminal session. With one started member the page offers to open it.

## Worktrees on the machine

A negotiated, optional extension of the create-session call, built the way launch was: a flat string
field on `terminalproto.Create` (`worktree`, a run token: lower-case hex, the same shape as an id),
the feature token `worktree-v1` in `X-Reet-Terminal-Features`, and a capability flag `worktree`
(omitempty, requires `launch`). A peer without it is never sent the field and the controller answers
"Update reet on that machine to start agents with worktrees."

When `worktree` is set the connector, before it starts the agent:

1. Resolves `Dir` as it already does (absolute, symlinks followed, exists, a directory, inside the
   enrolled home) and requires it to be the **root of a git working tree**: a `.git` directory (not a
   file pointing elsewhere, not a bare repository), `HEAD` readable and attached to a branch or at a
   commit, no merge, rebase, cherry-pick, revert or bisect in progress.
2. Computes the destination `~/.reet/worktrees/<project-slug>/<repo-name>/<run-token>` and the branch
   `reet/<project-slug>-<run-token>`. Slugs are lower-case letters, digits and hyphens made from the
   names; they never contain a dot-segment or a path separator. It refuses if the destination or
   branch already exists (a retried request that already created them adopts them: the existing
   worktree must be registered with git, on that branch and at the expected path, else it refuses).
3. Runs exactly `git -C <repo> -c core.hooksPath=/dev/null worktree add -b <branch> <destination> HEAD`
   as an argument vector (never a shell), with a minimal environment (`HOME`, `PATH`, `LANG`;
   every `GIT_*` variable removed; no identity is needed because no commit is made), a 60-second
   timeout, and output limited to 4 KiB. It is the only git command that changes anything.
4. Starts the agent in the new worktree exactly as a launch does (the working directory, the
   `#`/`;` escaping, the argument allowlist). A failure at any step leaves nothing behind: a
   half-created worktree is removed with `git worktree remove --force` limited to the destination the
   connector itself just created, and a failed branch is deleted only if the connector created it in
   this call.

The repository's own hooks are disabled for the call and no commit is made. *Amended after
implementation:* this does not mean creating the worktree runs no repository-configured program.
The repository's configuration still applies, so a smudge or clean filter (git-lfs, for example) or
`core.fsmonitor` can run a program during the add's checkout and during status. That configuration
is the user's own local setup, not cloned content, and the fixed command lines rule out overriding
it. The agent that is started afterwards can run anything, as it always could.

**Status and remove** are two further negotiated requests under the same feature
(`WorktreeStatus{Run, Project, Repo}` and `WorktreeRemove{Run, Project, Repo, Changed, Unpushed}`
as amended: both carry the Project slug the folder depends on; strict, comparable, replies are small
typed results):

- *Status* runs `git -C <worktree> status --porcelain=v1 -z` and counts commits, and returns the
  counts and the first 20 paths (each at most 255 bytes, control characters replaced), never file
  contents. *Amended:* **unpushed** is the commits reachable from the worktree's `HEAD` that are on
  no remote-tracking branch and on no local branch other than the run's branch and the branch
  checked out there (they stay on the run's branch after a remove); **unreferenced** is the commits
  reachable from `HEAD` or from the worktree's own `refs/worktree/*` and `refs/bisect/*` that no
  branch, tag or remote-tracking branch holds, with what `refs/stash` holds protected (they would
  become unreachable, so they block removal).
- *Remove* first repeats the status and refuses unless the request carries the counts it was shown
  (so changes made after the person looked cannot be thrown away), then runs
  `git worktree remove <path>` **without `--force`**. The branch is never deleted. Both refuse any
  path that is not registered by `git worktree list` and under `~/.reet/worktrees/`.

*Amended after implementation* (see [Worktrees](../../terminal-worktrees.md) for the exact
commands): status also runs `--untracked-files=all --ignore-submodules=none`, counts commits only
`HEAD` holds (`unreferenced`), files git is told not to check (`hidden`), and walks the folder for a
nested repository (at most 200,000 entries). Remove refuses, whatever was shown, while anything is
uncommitted (`has_changes` always blocks: the sheet shows the paths and tells the person to commit,
stash or discard in the worktree and Check again), while there are hidden edits, unreferenced
commits or a nested repository, or when the check was incomplete. Only unpushed commits can be
shown and then removed with, because they stay on the branch. Ignored files (`.env`,
`node_modules`) are deleted with the folder. Listings are capped at 4 MiB, not 4 KiB.

Codes (*amended*): a refused worktree create is `worktree_refused` with a reason, and the API maps
each reason to its own code and sentence (`not_repo_root`, `repo_busy`, `worktree_exists`,
`git_failed`, `outside_home`; for status and remove also `not_registered`, `has_changes`,
`unreferenced_commits`, `nested_repository`, `hidden_changes`, `check_incomplete`, 404 `not_found`);
`launch_failed` keeps its folder and tool meaning. `worktree_changed` (409) for a remove whose counts
no longer match. A machine runs one worktree status or remove at a time: a second is answered at
once with `conflict`, which the API reports as 409 `worktree_busy`, and probes are still answered
while git runs. The reply to a worktree create carries `worktreePath` and `worktreeRepo` (the
folder and the repository name the machine used), which the controller stores on the session; a
successful remove's answer names the folder it deleted and the branch it kept.

## Words

Plain and exact. One light Yorkshire phrase at most in a heading, lede or success line. "Verified" and
"healthy" never describe a machine, a session or a worktree.

| Cause | Sentence |
| --- | --- |
| Not a git working tree root | "That folder isn't the top of a git repository." |
| Repository in the middle of something | "That repository is in the middle of a merge, rebase or similar. Finish or abort it first." |
| Worktree could not be made | "Git couldn't make a worktree there." (detail: first line of git's message, plain text, 200 bytes) |
| Machine too old for worktrees | "Update reet on that machine to start agents with worktrees." |
| Destination or branch already there and not this run's | "A worktree or branch with this run's name is already on that machine, so nothing was started." |
| reet's worktree folder is a link, another user's or writable by others | "The folder ~/.reet/worktrees on that machine is a link, belongs to another user or can be changed by others. Fix it, then try again." |
| Remove blocked by changes (always blocks) | "This worktree has changes you haven't saved. Commit, stash or discard them in the worktree, then check again." |
| Remove after the counts changed | "That worktree changed since you looked. Check again." |
| Commits only on a detached HEAD | "Some commits in this worktree are not on any branch. Put them on a branch first, then check again." |
| Another repository inside the worktree | "This worktree contains another git repository. Move or remove it first." |
| Files git is told not to check | "Git is not tracking some edits in this worktree. Review them before removing." |
| Worktree could not be checked completely | "reet couldn't check all of this worktree. Remove it yourself." |
| Something at the path git does not list | "Git doesn't list that folder as this run's worktree, so reet won't touch it." |
| Nothing at the path | "That worktree isn't on that machine. It may have been removed already." |
| Git failed during status or remove | "Git couldn't check that worktree." / "Git couldn't remove that worktree." (detail: git's first line) |
| Branch kept | "The branch reet/… is kept." |
| Everything else | the existing `refusalText` sentences |

## Web

- A **Projects** page (a new navigation entry for accounts mode, writers only; viewers and legacy
  sign-in do not see it): the list, *New project*, and for each Project its member count and last run.
- **Project page**: name, defaults (agent, model, shared prompt, worktrees switch), members (machine,
  folder via the same folder choices as Start an agent, per-member overrides), *Start all*, and the
  **Runs** list (newest first, 20 shown) with each member's state, *Open*, and *Remove worktree*.
- Dialogs follow the existing patterns: native `<dialog>`, labelled fields, a polite status line, one
  immutable operation per attempt, focus returned to the opener, Escape and Close disabled while a call
  is in flight, 15-second abort, nothing about Projects in browser storage or URLs beyond the Project and
  run ids.
- Pure logic in `web/src/lib/projects.ts` (+ tests): member and name validation (reusing `agentLaunch`
  rules), the confirmation-sheet plan (`planRun(project, machines, snapshot)` returning start-able
  members with reasons for the rest), the run summary sentence, and `worktreeRefusalText`.

## Safety, accessibility and layout

- Accounts mode, write role, canonical origin and the rollout flag, as for terminals. Owner-only data;
  every query is scoped by the account.
- The controller sends only validated fields; the connector re-validates everything and resolves the
  paths itself. The page can never name a git command, a branch, a destination or a path outside the
  Project's members.
- Audit events (never the prompt): `project-create`, `project-update`, `project-delete`,
  `project-run` (Project, run, member count), `terminal-launch` (existing) per started member with the
  worktree path and branch added to its details, `worktree-remove` (machine, path, branch).
- The 12-member and 50-Project limits and the 2-in-flight rule keep a run bounded.
- Native `<dialog>`, keyboard operation, reduced motion respected, status in text as well as colour, no
  horizontal scroll at 320, 375, 768 and 1440px in light and dark, honest axe reporting (violations and the
  `incomplete` count; contrast is not claimed to pass).

## Testing

- Go: store (migrations, limits, uniqueness, scoping by account, run records immutable apart from state
  transitions), API (validation, owner scoping, viewers blocked, idempotent start, partial failure,
  derived request ids, retry rules), orchestration (two in flight, per-member failure, controller restart
  mid-run resumes pending members), protocol (`worktree` field validation, comparability, feature
  negotiation, old-peer refusal), connector (argument vectors exactly; repo-state refusals — bare repo,
  `.git` file, detached HEAD, rebase in progress, dirty tree is allowed; destination/branch exist;
  symlink escapes; hooks disabled proven by a repository whose `post-checkout` hook writes a marker;
  cleanup on failure leaves nothing; adoption on retry; status counts; remove refuses on changed counts
  and unregistered paths and never passes `--force`), and a real two-host test with recording fake agent
  CLIs and real git and tmux, run on Linux and macOS in CI (resolve `/tmp` symlinks; do not assume the
  shell name).
- Web: unit tests for `projects.ts`; a Playwright fixture `web/tests/projects.e2e.mjs` with synthetic API
  responses: create/edit/delete, the confirmation sheet with each unchecked reason, a run with all
  states, partial failure and per-member retry, a lost reply resumed with the same derived ids, remove with
  unsaved changes shown then confirmed, viewers and legacy sign-in seeing nothing, no storage, the layout
  and theme matrix. Mutation checks on the key rules.

## Delivery

Four pull requests, each merged when its checks are green and the broad review has passed:

1. Worktree create, status and remove on the connector, with the protocol feature, capability and tests.
2. The controller: store, API, run orchestration, audit.
3. The web: Projects page, dialogs, fixture.
4. The layout matrix, docs (`docs/projects.md`, `docs/terminal-worktrees.md`), handoff.

## What this does not establish

That an agent started in a worktree is signed in, understands the prompt, or makes progress; that the
branch contains anything; or that a repository's hooks, build or tests are safe to run. Removing a
worktree does not delete its branch. Physical device, screen reader, Safari and Firefox checks are not
planned beyond the existing Chromium fixtures.

## Continuation

Git-remote identity so a Project follows you to a newly added machine; agent status; resuming earlier
conversations; one session across several repositories where an agent supports it; pull request
creation; MCP exposure of Projects and runs; wider folder roots as a deliberate setting.
