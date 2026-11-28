## AGENT-USAGE-REVIEW-1: add an analysis of how coding agents are instructed in this repository (28 November 2026)

Adds `docs/agent-usage-review.md`, an analysis of the instructions, handoffs, checks and scripts that drive coding
agents here. It is documentation only: no prompt, configuration, script, test or application file was changed, and none
of its recommendations were implemented.

What changed.

- `docs/agent-usage-review.md`: an inventory of every place agent usage is defined or driven, with a path and what it
  does; findings ordered S1–S4; recommendations A–L with files touched, benefit, risk, effort and type; and the
  inspected-but-irrelevant list plus method and limits. "Agent" means the coding-agent setup around this repository, not
  the AI agents inside the reet product; that reading is stated in the Scope section so a human can correct it.
- `docs/handoffs/log/AGENT-USAGE-REVIEW-1.md`: this entry, added so the `notes` check passes. It is the second new file
  the review's finding 1 and recommendation A warned about; the alternative was to exempt docs-only analyses in
  `check-notes.sh` or accept a red check, which needs a maintainer's decision.

Why. The task was to review how agents are currently used in this project and propose improvements, without implementing
any of them.

What was checked. `mkdocs build --strict` passed (the new page is an INFO-level unlisted page, like `docs/ux-review.md`);
`scripts/check-docs.py` passed; the `hooks` unit tests passed. Every citation was re-read at this revision and line
numbers come from `nl -ba` on the file named.

What was not checked. `scripts/check-docs-browser.py` was not run locally because Playwright/Chromium is not installed
in this worktree; CI runs it in `docs.yml`. The Go, web and MCP suites were not run because no application code changed.
Organisation-level rulesets and branch protection were not inspected. The review's own counts include this entry, so
they move by one whenever another handoff log file is added.
