## AGENT-USAGE-FIXES-1: the agent usage review's mechanical recommendations are implemented (28 November 2026)

Implements the accepted recommendations from `docs/agent-usage-review.md`, leaving the heavy or behaviour-changing ones
for a maintainer decision.

What changed.

- **Broken headings.** `FACTORY-FINISHED-1`, `FOCUS-RING-DARK-1` and `REAL-AGENT-CHECKS-2` now start with
  `## <ID>: ...`, so lookup by entry ID works.
- **Dead references.** `README.md` and `docs/terminal-workspace.md` point at `docs/handoffs/log/` for current state;
  `docs/handoffs/seo-follow-up.md` points the historical `SEO-1` entry at `docs/handoffs/archive.md` and tells the
  reader to add a log entry instead of editing `handoff.md`; `docs/handoffs/gaffer-continuation.md`,
  `gaffer-resume.md` and `gaffer-factory-floor.md` point at the log or archive; `docs/control-centre.md` names the real
  `cd mcp && npm ci && npm test` command.
- **Checks and template.** `CONTRIBUTING.md` lists the web, MCP, fixture-build, vet and race steps that `image.yml`
  runs, the path-filter caveat, and docs/browser pointers; `.github/PULL_REQUEST_TEMPLATE.md` asks for the handoff entry,
  changelog fragment and MCP tests.
- **One rule, one place.** `AGENTS.md` keeps the two-file rule; `CONTRIBUTING.md`, `handoff.md`, `changelog.d/README.md`,
  `notes.yml` and `check-notes.sh` link to it and keep only their operational detail.
- **Probes and stages.** `CONTRIBUTING.md` gains short "Manual agent probes" and "Staged work" subsections.
  `handoff.md` indexes the four live continuation notes.
- **Review status.** `docs/agent-usage-review.md` gains a status update recording what was implemented and what was
  declined.

What was checked. `scripts/check-notes.sh` finds this entry, `mkdocs build --strict` passed,
`scripts/check-docs.py` passed 76 pages, and `git diff --check` reports no whitespace errors.

What was not checked. `scripts/check-docs-browser.py` was not run because Playwright is not installed in this worktree;
CI runs it in `docs.yml`. No application code changed, so the Go, web and MCP suites were not run. The declined
recommendations (archive consolidation, retiring plans, excluding this review, and a shared-file edit check) still need
a maintainer decision.
