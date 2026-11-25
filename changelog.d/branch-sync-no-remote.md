- **A branch that was never pushed is reported as such**: after a worktree is removed, the automatic branch clean-up
  recorded "fetch failed" in Activity when the base branch was not on origin at all. It now records "no remote
  branch", and keeps "fetch failed" for a network or sign-in problem.
