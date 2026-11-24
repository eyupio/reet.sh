## NOTES-FRAGMENTS-1: handoff entries and changelog fragments are one file each (24 November 2026)

The rest of the roadmap's "Delivery speed" item that ends merge conflicts. Every pull request I opened that day
conflicted on `handoff.md` or `CHANGELOG.md`, because each added an entry at the top, and a conflicted pull request does
not start the main test workflow at all.

- `handoff.md` was 4,900 lines and 124 entries. All of them are now in `docs/handoffs/archive.md`, unchanged, and
  `handoff.md` is a short page that says where new entries go: `docs/handoffs/log/<ID>.md`, one file each.
- `CHANGELOG.md` keeps its text. New entries go in `changelog.d/<name>.md`, one bullet each, and
  `python3 scripts/changelog_collect.py --write` folds them into "Unreleased" when a release is cut (it stops on a file
  that is not a bullet). Tried in a scratch copy with two fragments and a bad one.
- `AGENTS.md` is new and short: the traps the code does not show (worktrees without `web/node_modules`, the one-import
  rule for the web API files, the backup checklist, feature names, previews and `pkill`, fetching before merging,
  shards). `CONTRIBUTING.md` points at it.
- `docs/handoffs/flaky-tests.md` lists the unstable tests I know of with an owner and a date. The owner and the date
  (8 December 2026) are my choice, so change them to who should answer for each.
- One wording fix found on the way: the changelog said revoking a personal token "still cuts off any machine that has
  not moved"; it does not end the machine's private connection. That is corrected, and a fragment says so for the next
  release.

Not done: nothing checks that a pull request that changes behaviour added a fragment or a log entry. Moving the archive one folder down broke 124 repository-relative links in it (a review found them); they
are rewritten and a check finds none broken. Not checked: that every cross-reference to `handoff.md` still reads well
(they cite entry IDs, which are all in the archive).
