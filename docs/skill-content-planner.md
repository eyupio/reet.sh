# The skill-content planner

This page described an earlier stage of skill sync. Its per-machine pins, planner, preview and ownership-adoption workflow was retired with the old engine in PR #845. It is not a supported setup path for the current controller.

Use the shared library instead: Active skills reach every connected machine; Archived skills stay in reet and are removed from machines. The connector installs saved bundles, verifies their digests, and backs up replacements and removals. Unknown machine skills wait for Keep or Archive.

- [Current workflow and upgrade behaviour](sync-skills-between-machines.md)
- [Connection, local consent and recovery](controller-sync.md)
- [Saved content and version history](skills-single-source.md)

The earlier implementation history remains in git and the dated design records under `docs/superpowers/`.
