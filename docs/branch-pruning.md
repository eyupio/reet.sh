# Pruning merged local branches

Agents leave a branch behind for every piece of work: a worktree's branch, a
task branch that was squash-merged, or one made by hand. reet can list the
local branches whose work is already in the base and delete exactly those,
from the web UI, from `reet branches`, or automatically after a worktree is
removed.

Nothing about a worktree's removal changes: reet still never deletes a
worktree's branch when it removes the worktree. Pruning is a separate,
explicit action.

## What is safe to delete

A branch is offered only when its work is already in the base:

- its commits are contained in the base (`git rev-list --count base..branch`
  is 0), or
- every commit it has in front of the base is already applied there with the
  same patch (`git cherry base branch` lists no `+` line).

Anything that cannot be proven that way is kept, with a reason. These are
never deleted:

- the branch checked out in the folder (current),
- the repository's default branch, and the base itself,
- a branch checked out in any worktree,
- a branch with commits its remote does not have (unpushed),
- a branch that is not proven merged.

Deletion is `git update-ref -d refs/heads/<name> <oid>`: only a branch still
pointing at the tip the plan showed is removed, and the working tree is never
touched. Remote branches are never pruned.

The proof is local, because a machine may not have forge credentials. A squash
merge that does not preserve patch-ids is therefore shown as not merged and
kept; that is the conservative answer, not a promise.

## In the web UI

On **Machines**, a repository project's menu has **Prune merged branches…**.
It reads the plan first and shows:

- a warning with how many local branches will go and the safety rules,
- **Will be deleted**, and
- **Kept**, each with its reason.

Nothing is deleted until **Delete N branches** is pressed. The answer says
what was deleted and lists anything skipped because it moved or git refused.

The controller route is:

- `GET /api/v2/machines/{agentId}/branches/prune?dir=&base=` — the plan.
- `POST /api/v2/machines/{agentId}/branches/prune` — delete the branches in
  the plan.

Both need a writer's browser session on the same origin; the prune is rate
limited (5 per 10 seconds per account and machine) and writes one
`branches-prune` audit row (machine, host, folder, deleted names, skipped
count).

## From the command line

On the machine that holds the repository:

```sh
reet branches status --dir /home/dev/site
reet branches prune --dir /home/dev/site --dry-run
reet branches prune --dir /home/dev/site --yes
```

- `--dir` is the repository's top folder (default: the current folder).
- `--base` picks the branch merged work is measured against (default: the
  repository's default, then `main`, then `master`).
- `--dry-run` prints the plan and changes nothing.
- `--yes` deletes without asking; without a terminal, a prune does nothing
  until it is given.
- `--json` prints the plan or result as JSON.

Exit codes: `0` nothing to do or done, `1` refused or failed, `2` usage, and
`3` when `--dry-run` (or a run without `--yes`) found branches to prune.

The command runs on the machine with the same rules as the controller's
machine answer. It is not sent to the controller, so it does not appear in the
controller's Activity.

## Automation

An owner or admin can turn on **Settings › Admin › Repository maintenance**:
after a worktree is removed, reet plans and prunes the merged local branches in
the repository that worktree came from. It is off by default, uses the same
proof and deletion rules, and writes the same `branches-prune` audit row with
`automatic` set. Anything that fails is left for the person to see.

## Protocol

The machine answers two messages after negotiating `branch-prune-v1`:

- `branch_prune_plan {dir, base?}` → `branch_prune_plan_result`, each branch
  with its tip and a skip reason or none.
- `branch_prune {dir, base, branches[{name, oid}]}` → `branch_prune_result`,
  the deleted names and anything skipped.

The folder must be the top of a git repository inside the user's home, git
runs as an argument vector with the hardened environment, and the work is
bounded to 20 seconds. An older machine without the capability is refused
with “Update reet on that machine to prune its branches.”
