## FACTORY-ORPHAN-GOALS-1: deleting a Factory outcome removes its ended sessions' goals (27 November 2026)

Reported: the Factory page's Other sessions listed dozens of Finished rows ("Fix refresh showing a pre-loaded screen…
· merge", Stopped) for outcomes that had been deleted.

Cause. `DeleteGafferOutcomeFor` removed only the `gaffer_outcomes` row. Each stage session's Gaffer goal (text
`Outcome: …`) and saved chat stayed, and with no pipeline drawing them `Factory.svelte` listed them as Other sessions.
The goal is also what `listingHideFactorySessions` uses to keep the session off Home and Sessions once its outcome is
gone (the old `shortcut:` note there), so the goals could not simply be dropped.

What changed (`internal/store/store_gaffer_orphans.go`).

- `pruneFactoryGoals` finds goals that start `Outcome: `, are completed, stopped or at their limit, and that no
  outcome names (by session, conversation, retired conversation or stage). It removes the goal's conversation (the goal
  and turns cascade) only when its session is gone, already archived, or ended with no worktree still to release; the
  last is archived first, with the same steps as removing it from history (`archiveTerminalTx`, split out of
  `ArchiveTerminalFor`). A running session, or one whose worktree is not recorded as removed, keeps its goal and stays
  hidden as before.
- It runs in the same transaction as `DeleteGafferOutcomeFor` and after `CreateGafferOutcomeFor`'s retention prune
  (which orphaned goals the same way), and once at start (`pruneFactoryGoalsAtStart`) for rows left by earlier deletes.

Test: `TestDeletingAnOutcomeRemovesTheGoalsOfItsEndedSessions` (ended, running, worktree and other-outcome sessions, and
the start-up sweep). `go test ./internal/store ./internal/gaffer` and `./internal/api -run Outcome|Listing|Factory|Gaffer`
pass.

Not checked: the web page against real data (no browser run; `web/node_modules` absent here), the full Go suite, and
an upgrade on a large database (the sweep runs on every start; its query reads every Factory goal once). Goals of
sessions with an unremoved worktree, or still running, stay in Other sessions only if their outcome is not deleted;
once deleted they stay hidden but are not removed until the session ends and its worktree is released.
