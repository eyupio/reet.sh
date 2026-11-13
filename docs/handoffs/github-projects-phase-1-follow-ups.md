# GitHub projects, phase 1: what was left open

Phase 1 of [the plan](../superpowers/plans/2026-11-13-github-repo-projects.md) merged on 13 November 2026
(eyupio/reet.sh#777). Its reviews raised the points below and none of them blocked the merge. Line numbers
are as the reviewers gave them and will have moved. Items the final fix round closed are not listed.

## Owed by the owner

- **The `gh` check on a real machine** ([device checks](../device-checks.md), gate 4). Nobody has yet seen
  hardened git use the `gh` sign-in against a private repository. Run it from the connector's own service
  environment, so it also shows `gh` is on the service's PATH.
- **A real session start.** Launch stage notes reach the controller for the first time, so the browser's
  start progress shows stages it never received before. It was not exercised against a live machine.
- **A decision:** a start in several projects from the launch bar does not ask the base question; it
  records the notice instead.

## Fix before phase 2 builds on it

The two `GitHubRemote` items (a port read as the owner, and the host matched case-sensitively) were fixed
on the phase 2 branch: a port is refused, the host is matched in any case and `https://user@github.com/…`
is recognised (`internal/terminalhost/gh_credentials.go`).

- `internal/terminalhost/inspect.go` (`inspectGitHubRepo`) parses github.com remotes a second way. Phase 2
  Task 7 moves the owner and repo patterns; fold this one in then.

## Behaviour worth a second look

- **The default branch is the cached `origin/HEAD`** and is never refreshed. After a default-branch rename
  on GitHub, an unnamed base resolves the old name, finds it absent, and starts from the stale local
  branch as `ahead`.
- **A repository too large to fetch in 30 seconds never catches up:** each start discards the partial
  fetch and pays the 30 seconds again. An offline machine pays about 10 seconds in the browser and 30 in
  the create on every start; nothing remembers "unreachable".
- **The 30-second fetch budget starts before the wait for the per-repository lock**, so a start queued
  behind a hung fetch can answer "couldn't reach GitHub" without trying.
- **A saved project base that no longer exists now refuses controller-made launches** with `base_missing`,
  where they used to start from HEAD.
- **A confirmed start fetches twice** (the check, then the create), so the commit named in the dialog can
  differ from the one the session starts at. The stored notice is the true one.
- **No launch stage is reported during the fetch**, so a slow fetch shows up to 30 seconds of the previous
  stage.
- `githubOrigin` treats any `git config` failure as "not a GitHub origin", which silently skips the fetch.
- `checkWorktreeRepo` runs with git pinned in `PlanBase` and unpinned in `prepareWorktree`. No repository
  was found where the two disagree.
- `OnSessionBase`'s callback reads and writes the database on the peer's observer goroutine.
- The browser's confirmation key (`baseKey`) leaves out the project's saved base, so a base changed while
  the dialog is open is not asked about again.
- An invalid `SessionChanged.Base` makes the whole reply fail strict decoding; the "ignored, never fails
  the create" rule holds only at the store.

## A record that is lost

- **A late branch prune or software apply answer leaves no audit row.** The machine does the work, but the
  `branches-prune` and `software-apply` audit rows, and the stored software entry, are written only by the
  call that was waiting. If that call has given up (a closed tab, a timeout), the answer is counted and
  dropped by the broker (`internal/terminalbroker/broker.go`, `Peer.Receive`). Before the connection fix
  it ended the machine's connection instead, and the record was lost then too. Recording a late answer
  with the account, the machine and the outcome is still owed.

## Small code and test gaps

- `base_missing`'s sentence reads "…on ." when the machine's name cannot be read
  (`internal/api/terminal_browse.go`); the same `BranchesReason` switch is written three times there.
- `RepoProjectForDir` runs before the folder is validated in `internal/api/terminal_base_check.go`.
- The `worktree-base` event's detail includes the host, so its once-only key changes if a machine is
  renamed between a create and its resend. No test covers a resent create writing one event.
- `StartAgentDialog.svelte`: `checking` is not in a `try`/`finally`, and Start stays enabled and un-busy
  while the check runs. `baseGate.ts` swallows a signed-out error with every other failure.
- No test for: the base check's rate limit; the connector and agenthealth feature gates for `base_check`;
  a github.com origin with no `gh` on PATH in the `SyncBranch` test; a `git_failed` or offline answer in
  the browser gate; an empty base with a successful fetch and HEAD on another branch through
  `PrepareWorktree`.
- `TestBaseFetchIsBoundedAt30Seconds` answers at once instead of blocking until its context ends.
- Over-long lines were left in `docs/projects.md`, `docs/session-board.md`, `docs/terminal-worktrees.md`
  and `.github/workflows/terminal-browser.yml`.

## CI jobs that fail now and then, on `main` too

Neither was traced to a root cause.

- **`offline (native)`** (Gaffer offline acceptance): `web/tests/gaffer-offline-live.e2e.mjs:138` expects a
  new goal's instruction to be `queued` and sometimes finds `sent`. It failed twice on the phase 1 branch
  and passed nine times; the same job failed on `main` at `5429695` at a different wait in the same flow.
- **`terminal-browser`**: `web/tests/terminal-phone.e2e.mjs:154` sometimes sees a resize to fewer rows
  after the first attach. The job was red on `main` for three of five pushes that day, with a different
  symptom in the one that was read ("browser terminal integration timed out").
- **`native service (ubuntu-latest)`** failed once: the root run of `TestNativeAgentKillKeepsTmuxSession`
  could not run `tmux -V`, while the same test passed as the normal user in the same job.
