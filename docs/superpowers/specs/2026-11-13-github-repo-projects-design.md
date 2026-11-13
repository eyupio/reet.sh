# Projects from a GitHub repository: design

13 November 2026. Status: awaiting the owner's approval. Nothing here is built.

## Intent and why

Today a project is a folder that already exists on a machine. To work on a GitHub repository that is not
yet on any machine, a person has to open a terminal, clone it, then add the folder as a project. The owner
wants to skip that: paste `owner/repo`, choose a machine, and start sessions and Factory pipelines.

The owner's opening idea was a fresh clone per task, deleted after merge. Asked what mattered most, they
chose **no folder setup** over "nothing left on the machine" and "any machine can do it". That is why this
design keeps one clone per repository and gives each task a throwaway worktree, which reet already does
for every repository project.

## What exists and is reused unchanged

- An agent session started in a repository project gets its own worktree at
  `~/.reet/worktrees/<project>/<repo>/<run>` on branch `reet/<project>-<run>`
  (`internal/api/session_isolation.go`, `internal/terminalhost/worktree.go`). A plain terminal, a resumed
  conversation and a resumed chat are not isolated and run in the folder itself.
- Factory pipelines start their sessions through a folder project's machine and folder
  (`internal/gaffer/outcome_runs.go`), then run plan to merge with the merge gate.
- The GitHub panel reads pull requests with the machine's own `gh` sign-in. The controller holds no GitHub
  token.
- A worktree is removed when its session is removed from history, and by the scheduled clean-up, which is
  on by default with a 1-day wait ([machine maintenance](../../machine-maintenance.md), changed in
  eyupio/reet.sh#776). After a removal, unless an admin turned it off, reet fetches the base,
  fast-forwards the folder and prunes merged local branches ([branch pruning](../../branch-pruning.md)).
  That fetch runs in the hardened git environment, so today it answers `fetch_failed` for a private
  HTTPS remote.

## Goals

1. Add a project by naming a GitHub repository and a machine. No folder has to exist first.
2. Once added, it behaves as any repository project: launch bar, worktree per session, Factory, GitHub
   panel, worktree clean-up.
3. A task's worktree starts from the base branch as it is on GitHub at that moment, not from whenever the
   clone was last touched.
4. Removing the project can remove the clone reet made, so nothing reet created is left behind.

## Non-goals

- Choosing a machine automatically, or moving a project between machines.
- A saved setup command, copied `.env` files or stored secrets. The owner chose to leave setup to the
  agent, guided by the project's starter prompt.
- Removing a worktree at the moment a pull request merges. The owner chose session removal plus the
  scheduled clean-up as the two triggers.
- Hosts other than github.com, and GitHub Enterprise.
- Listing or searching the person's repositories in the dialog.

## Anti-goals

- The controller must never hold, receive or forward a GitHub credential.
- reet must never delete a folder it did not clone, and never delete a clone that holds work that exists
  nowhere else.
- The existing safety of git on the machine (hooks off, argument vectors, bounded output and time) must
  not be weakened for the clone or the fetch.

## Constraints

- **Full, normal clone.** `PrepareWorktree` requires `.git` to be a real directory in a normal working
  tree, so a bare or mirror clone will not do. The Changes panel refuses partial clones, so no
  `--filter`. A shallow clone is out too: the pruning and unpushed counts need history.
- **Inside the machine user's home.** `ResolveDir` refuses a folder outside home.
- **Credentials come from the machine's `gh` sign-in.** The hardened git environment keeps only `HOME`,
  `PATH` and `LANG`, and `SyncBranch` already notes that a remote needing a credential helper outside
  that environment answers `fetch_failed`. The clone, the fetch before a task and the existing
  post-removal `SyncBranch` fetch must all work for a private repository with only `gh auth login` done
  on the machine.
- **Time.** A worktree create is bounded at 60 seconds on the machine. The fetch before a task gets its
  own bound inside that (writer's call: 30 seconds); a fetch that runs out counts as a failed fetch.
- **Mixed versions.** A new machine request needs a negotiated feature and a capability, as
  `worktree-v1` and `branch-prune-v1` have. A machine without it is never sent the request and the
  dialog does not offer it there.
- **Strict decoding.** New wire fields go only to connections that negotiated the feature.

## Approaches

Three were weighed with the owner, who picked B.

| | How | Trade-off |
| --- | --- | --- |
| A. Fresh clone per task | Clone, work, merge, delete everything | Nothing kept on the machine, but every task re-clones and reinstalls dependencies, and it duplicates the worktree machinery |
| **B. Kept clone, worktree per task** | One clone per repository per machine; each task gets a worktree | Fast starts and reuses today's code; a clone stays on disk for reet to manage |
| C. Controller-side GitHub App | The controller holds credentials and hands out clones | Any machine could work, but it breaks "no GitHub token on the controller" |

Within B there were two ways to model it; this is the writer's call, not the owner's:

- **B1 (chosen): clone when the project is added, then save an ordinary folder project** that points at
  the clone and records where it came from. Every later code path sees a normal repository project.
- B2: a new project kind with no folder until the first task. Rejected: every reader of a project's
  folder (launch, Factory, worktrees list, pruning, inspection) would need a "not cloned yet" case.

## The design

### Adding

The Add project dialog gains a second way in beside the folder browser: **From GitHub**. The person
types `owner/repo` or pastes a `https://github.com/owner/repo` link, and picks a machine from those that
report the new capability. reet shows the folder it will create before anything happens.

The controller validates the name (`owner` and `repo` by GitHub's own character rules; nothing else is
passed to the machine) and sends one new request. The machine:

1. Refuses unless `git` and `gh` are found at absolute paths and `gh` reports a github.com sign-in.
2. Clones `https://github.com/<owner>/<repo>.git` into `~/reet/repos/<owner>/<repo>`, using the `gh`
   sign-in for credentials, hooks disabled. Writer's call on the location: visible rather than under
   `~/.reet`, so the folder browser and a person's own shell find it, and so it never sits beside
   `~/.reet/worktrees`.
3. If the destination already exists, adopts it only when it is a normal clone whose `origin` is that
   same repository; anything else is refused and left untouched. A retry of the same add therefore
   succeeds.
4. Writes a marker that says reet made this clone, so removal can tell it from a folder a person made.
5. Answers with the folder and the remote's default branch.

The controller then saves a folder project exactly as today, with `isRepo` true, the base branch set to
the default branch, the name defaulting to the repository's name, and two new stored facts: the origin
`owner/repo` and "cloned by reet". The existing limits apply (100 projects, one project per machine and
folder, name rules).

A clone can take minutes. The request is bounded (writer's call: 10 minutes) and the dialog shows that
it is working and can be left; a clone that outlives the dialog still ends in a project or a stated
failure, never a half-made folder. A failed clone removes only what that call created.

Refusals each get one plain sentence, in the house voice: not signed in to GitHub on that machine, no
access to that repository or it does not exist, the folder exists and is something else, the machine
needs a newer reet, the machine is offline, the clone timed out.

### Starting a task

Before a worktree is created for a session in a repository project that has a github.com `origin`, the
machine fetches the base branch from `origin` and then chooses the starting commit.

**Which branch is the base.** The launch's own base when it names one, otherwise the project's saved
base branch, otherwise the remote's default branch. Factory launches and forced isolation name no base
today and so start from whatever the folder's `HEAD` is; with this change they use the project's saved
base. (Writer's call.)

**Which commit** (the owner's decision):

| Local base compared with GitHub's | The task starts from | The person is told |
| --- | --- | --- |
| The same | That commit | Nothing |
| Local is behind, or there is no local branch | GitHub's commit | Nothing for a project reet cloned |
| Local has commits GitHub lacks (ahead or diverged) | The local commit | That the task starts from local work GitHub does not have |
| The fetch failed or timed out | The last known base, local first | That GitHub could not be reached and the base may be old |

Today's lookup tries `refs/heads/<base>` then `refs/remotes/<base>`, which never finds `origin/<base>`;
a clone with no local base branch answers `base_missing`. The new lookup uses `refs/remotes/origin/<base>`.

**Folders a person added themselves** (the owner's addition). The same rule applies, with a notice and a
confirmation: when the local base and GitHub's differ, a start from the browser shows which commit the
task will start from and why, and waits for the person to confirm before the worktree is made. Writer's
calls on what the owner left open:

- The confirmation appears only when the two differ. When they are the same the start is not interrupted.
- A start with nobody at the browser (a Factory pipeline's later stages, a controller-owned outcome)
  cannot ask. It follows the table and records the notice on the session and in Activity. The Factory's
  own plan review is the confirmation for the outcome as a whole.
- A project reet cloned never asks: nobody works in its checkout by design, so the table's notices are
  enough.

The clone's own checkout is meant to stay on the base branch, and the existing post-removal sync keeps
it fast-forwarded. A plain terminal or a resumed session can still be opened in it and leave it dirty or
off the base; the sync then leaves it alone, and tasks are unaffected because they start from the
fetched commit, not from the checkout.

### Removing

**Remove from reet** on a project that reet cloned asks one more thing: also delete the clone. It is
offered only when the machine confirms, at that moment, all of:

- the marker is present and `origin` still matches,
- no worktree of the clone remains,
- the checkout has no uncommitted changes, no stash, and no local branch with commits that are not on
  the remote.

Otherwise the project is removed from reet and the folder is kept, with a sentence saying why.
Writer's calls: while a finished session's worktree is still waiting for the clean-up, deleting the
clone is simply not offered, and the sentence says the worktrees must go first; Remove does not clear
them for the person. A `reet/...` branch that is fully pushed does not count as work that would be lost,
whether or not it has a pull request.

If the project is removed and the clone kept, adding the same repository on the same machine adopts the
folder and, because the marker is there, it is again "cloned by reet". A folder without the marker is
adopted as a person's own folder and never becomes deletable. Deleting
uses the same shown-then-confirmed pattern as worktree removal: the machine re-checks what the person
was shown and refuses if it changed. The removal is audited.

A project that points at a folder a person made is removed as today and its folder is never touched.

### What the person sees

The Add dialog's new path, the base notice and confirmation on a start, and the extra line on Remove
are new. A project from GitHub
shows in the rail as any repository project. Copy follows [voice and copy](../../voice-and-copy.md).

## Left to the builder

- The wire shape: message names, the feature token and capability name, result codes.
- How the `gh` sign-in is handed to git for the clone and the fetch while keeping the hardened
  environment (for example a per-call credential helper that runs `gh auth git-credential`). This is the
  main technical risk: prove it against a private repository on a real machine before building the rest.
- Whether the add is one long request or a started job that the dialog polls; the behaviour above is
  what matters.
- The marker's form and place, and the schema change for the two new stored facts.
- Whether the pre-task fetch is part of the worktree create or its own request before it.
- Slices and their order. A sensible first slice is handing the `gh` sign-in to git and the
  fetch-before-worktree change, which are useful alone and independent of cloning.
- Tests, docs (`docs/projects.md`, `docs/terminal-worktrees.md`, README, CHANGELOG) and UI verification
  per the repository's rules.

## Decisions and who made them

| Decision | Who |
| --- | --- |
| Kept clone plus a worktree per task (B) | Owner |
| No folder setup is the main goal | Owner |
| Worktrees go on session removal and the scheduled clean-up, on by default, 1-day wait | Owner |
| Setup is left to the agent; no saved command or secrets | Owner |
| Clone at add time and save an ordinary folder project (B1) | Writer |
| One machine per project, chosen when adding | Writer |
| Clone lives at `~/reet/repos/<owner>/<repo>` | Writer |
| A task starts from GitHub's base unless the local base is ahead, for every repository project | Owner |
| A person's own folder gets a notice and a confirmation | Owner |
| The confirmation shows only when local and GitHub differ; unattended starts record a notice instead | Writer |
| With no base named, a launch uses the project's saved base branch | Writer |
| A failed or slow fetch does not block the task | Writer |
| Deleting the clone waits for its worktrees to go; pushed branches do not block it | Writer |
| Remove can delete the clone, only when nothing would be lost | Writer |
| github.com only | Writer |
