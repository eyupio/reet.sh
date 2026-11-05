# Machine maintenance

Housekeeping for your machines, in **Account → Maintenance**. It removes only what you have already saved, and it keeps every branch. Today it has one task, cleaning up worktrees. The page is built so other tasks (log pruning, update warnings) can sit beside it with their own settings and last-run line.

## Worktrees

Each session can work in its own worktree, and they add up. **Scan for worktrees** asks your machines about the worktrees of finished sessions (archived ones included) that are old enough, and sorts what it finds:

| Group | Meaning | What you can do |
| --- | --- | --- |
| Safe to remove | No uncommitted changes, no commits on no branch, no hidden edits, no nested repository. | **Clean** removes the selected ones. Unpushed commits stay on the branch. |
| Needs your review | Uncommitted changes, or another check refuses it. | **Review…** opens the worktree's own dialog: it lists the files, says whether the branch has a pull request, and asks you to tick that the changes will be deleted. |
| Skipped | The machine was offline or couldn't be asked. | Scan again later. |

A clean checks each worktree again on its machine and removes it only if the counts still match what was read, so a change made after the scan is never lost. Bulk clean never removes unsaved work.

Only sessions reet has a record of are listed. A worktree the controller never recorded is not found.

## Scheduled clean-up

Off until an owner or admin turns it on. On a schedule (every 6 hours, day or week) it does a scan for every account and removes only the **safe** group, for sessions finished longer ago than the age you pick. The rest is left for you. Each removal is in Activity, marked automatic, and the page shows the last run's counts.

`REET_MAINTENANCE=off` stops the scheduler from starting at all.

## API

- `GET /api/v2/maintenance`: the settings and the last scheduled run.
- `PUT /api/v2/maintenance/worktrees` (admin): `{enabled, intervalHours, minAgeDays}`, each optional.
- `POST /api/v2/maintenance/worktrees/scan`: `{minAgeDays?}` returns `{items, more}`.
- `POST /api/v2/maintenance/worktrees/clean`: `{items: [{agentId, project, repo, run}]}` (1 to 50) returns each item's outcome.

Scan and clean need an account browser session, like the other terminal routes.
