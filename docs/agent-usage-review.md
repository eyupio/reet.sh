---
title: "Agent usage review: inventory and assessment"
description: "Inventory of the instructions, handoffs, checks and scripts that drive coding agents in this repository, with severity-ordered findings and recommendations."
---

# Agent usage review: inventory and assessment

This is an analysis document. Nothing described here has been implemented, and no prompt, configuration, script or
application file was changed by the review.

## Scope

"Agent" in this review means the coding-agent setup around this repository: the instructions a person or an agent reads
before changing the code, the handoff and changelog workflow, and the scripts and CI checks that drive contribution. It
does not mean the AI agents inside the reet product (Gaffer, structured chats, Factory stages, agent accounts or the
built-in skills that reet ships to its users). That product surface is listed under "Inspected and irrelevant" so the
next reader does not re-open it.

If that reading is wrong, this document should be corrected before its recommendations are acted on.

## What already works

- `AGENTS.md` is short, aimed at traps rather than repeating the development guide, and its claims match the scripts:
  the six Go shards (`AGENTS.md:34-35` and `scripts/ci-go-shard.sh:8-43`), the `notes` check (`AGENTS.md:9` and
  `scripts/check-notes.sh:15-24`), and the unstable-test list (`AGENTS.md:36-37` and `docs/handoffs/flaky-tests.md`).
- Handoff entries and changelog fragments are new files rather than edits to a shared document, which removes the
  merge-conflict problem described in `docs/handoffs/log/NOTES-FRAGMENTS-1.md:3-10`. The handoff half is enforced by
  `.github/workflows/notes.yml:3-26`.
- CI path filters keep a documentation-only pull request off the 20-minute Go run (`image.yml:7-23`), and the docs
  build is strict and checks rendered links, metadata, search, the sitemap and crawler files
  (`.github/workflows/docs.yml:76-110`, `scripts/check-docs.py:96-227`).
- Unstable tests have an owner and a fix-by date instead of a "retry until green" habit (`docs/handoffs/flaky-tests.md`
  and `AGENTS.md:36-37`).
- Real-agent checks are honest about what has not been run, and name the exact commands and scripts
  (`docs/device-checks.md:1-10`, `docs/handoffs/real-agent-checks.md`).

## Inventory

Every path below was read in this tree. "State" distinguishes current guidance from historical records. The counts
are for this revision.

| # | Path | What it defines or drives | Who or what consumes it | State |
| --- | --- | --- | --- | --- |
| 1 | `AGENTS.md` (37 lines) | The two-file rule, house style, and repository traps not visible in the code | Coding agents and contributors | Active |
| 2 | `CONTRIBUTING.md` (127 lines) | Development setup, local checks, style, pull-request flow, benchmark and Gaffer acceptance notes, docs-site build | Contributors and agents | Active |
| 3 | `handoff.md` (14 lines) | Says where new handoff entries go, the heading format, and the archived entries | Contributors writing entries | Active |
| 4 | `docs/handoffs/log/*.md` (51 files) | One record per change: what was built, checked and left undone | The next contributor or agent | Active, with some process entries |
| 5 | `docs/handoffs/archive.md` (4,905 lines) | Every handoff entry from before the one-file layout, newest first | Readers following an entry ID | Historical, excluded from the site |
| 6 | `handoff-history.md` (4,031 lines, 281,734 bytes, repository root) | Older delivery narrative, archived on 23 October 2026 | Linked from `docs/handoffs/archive.md` and research notes | Historical, outside `docs_dir` |
| 7 | `docs/handoffs/flaky-tests.md` | Unstable tests with evidence, owner and fix-by date | Contributors and CI reviewers | Active |
| 8 | `changelog.d/README.md` and `changelog.d/*.md` | One fragment per user-visible change; format and release collection rule | Contributors and release maintainer | Active |
| 9 | `scripts/changelog_collect.py` (54 lines) | Folds fragments into `CHANGELOG.md` under "Unreleased" | Release maintainer | Active |
| 10 | `.github/workflows/notes.yml` (26 lines) and `scripts/check-notes.sh` (26 lines) | Fails a pull request with no new handoff entry; advises a fragment for `web/src`, `internal/api` or `cmd` changes | CI on every pull request | Active |
| 11 | `.github/workflows/image.yml` (full suite) | Web check, MCP tests, `go vet`, and six Go shards under `-race`; path filters decide whether it runs | CI on code-changing pull requests and pushes | Active |
| 12 | `.github/workflows/docs.yml`, `scripts/check-docs.py` (228 lines), `scripts/check-docs-browser.py` (138 lines), `hooks/docs.py` (329 lines) | Builds and publishes the docs site; checks links, metadata, sitemap, search, crawler files and browser layouts | CI on docs changes; GitHub Pages | Active |
| 13 | `scripts/ci-go-shard.sh` (43 lines) | Splits the Go suite into `store`, `api-1`, `api-2`, `gaffer-1`, `gaffer-2` and `rest` | `image.yml:66-96` | Active |
| 14 | `scripts/run-browser-suites.sh` (55 lines) | Runs named browser suites and reports every failure | `terminal-browser`, `gaffer-settings` and sibling workflows | Active |
| 15 | `.github/PULL_REQUEST_TEMPLATE.md` (14 lines) | Pull-request prompts and a four-item checklist | Contributors opening a pull request | Active |
| 16 | `mkdocs.yml` (206 lines) | Site navigation, the public page set, and `exclude_docs` for design notes, research, handoffs and plans | Docs CI | Active |
| 17 | `docs/documentation.md` (9 lines) | Front page of the documentation hub | Site visitors | Active |
| 18 | `README.md` contribution pointers (`:19`, `:23`, `:505-509`, `:574`) | Points readers at `CONTRIBUTING.md`, instruction-file discovery and a handoff | Contributors and readers | Active |
| 19 | `docs/handoffs/*.md` outside `log/` and `archive.md` (36 files, 5,802 lines) | Feature continuation notes, acceptance contracts, QA lists and one-off delivery records | Contributors resuming that feature | Mixed; no status marker |
| 20 | `docs/handoff-golang-code-style.md` (149 lines) | Record of a one-off Go style migration | Excluded from the site; kept as history | Historical |
| 21 | `docs/superpowers/plans/*.md` (28 files) | Implementation plans for delivered work | Excluded from the site; used as a record | Historical |
| 22 | `docs/superpowers/specs/*.md` (42 files) | Design specifications for delivered work | Excluded from the site; used as a record | Historical |
| 23 | `docs/device-checks.md` and `scripts/probe-*.py` (10 scripts), `scripts/check-claude-credentials.py` | Manual checks and probes that start real agent CLIs against isolated homes and stubs | Maintainers closing real-agent gates, by hand; no CI | Active, manual |
| 24 | The other workflows (`terminal-browser.yml`, `gaffer-settings.yml`, `gaffer-offline.yml`, `structured-worktree.yml`, `native-services.yml`, `private-connection.yml`, `shared-native-view.yml`, `benchmarks.yml`, `release.yml`, `cleanup-*.yml`) | Product browser suites, acceptance runs, benchmarks, releases and housekeeping | CI on matching paths or schedules | Active |

Searched and not present in this revision: `CLAUDE.md`, `.claude/`, `.cursor/`, `.cursorrules`, `prompts/`, a
repository-level `skills/` directory, `.github/agents/`, `.opencode/`, and `.superpowers/` (the last is ignored by
`.gitignore:1`). `AGENTS.md` is the only root instruction file. The built-in skills under `internal/api/builtin_skills/`
are product content, not repository instructions.

## Findings

Severity: **S1** a red check or a dead reference that sends a reader to instructions that no longer exist; **S2**
guidance that conflicts with the real scripts or CI, or a stage with no guidance; **S3** duplicated or stale guidance
that can mislead; **S4** historical noise, or a setup heavier than the work needs.

### S1: dead references and a guaranteed red check

1. **The review itself cannot pass the `notes` check as a one-file change.** `scripts/check-notes.sh:13-19` fails any
   pull request that does not add a new `docs/handoffs/log/*.md`, and `.github/workflows/notes.yml:7` runs on every pull
   request. Adding only `docs/agent-usage-review.md` therefore leaves `notes` red. The options are to add a second new
   file (which contradicts the one-document deliverable), exempt a docs-only analysis in `check-notes.sh`, or accept the
   red check with a maintainer's decision. This is a process conflict, not a defect in the review text.

2. **`docs/control-centre.md:96-97` points at an MCP suite in `CONTRIBUTING.md` that is not there.** `CONTRIBUTING.md`
   contains no MCP command or section; the suite lives in `image.yml:58-62` as `cd mcp && npm ci && npm test`. A reader
   following the validation paragraph cannot find the step it names.

3. **`docs/handoffs/seo-follow-up.md` still gives the pre-24-November instructions.** `:7` and `:145` send the reader to
   `handoff.md` section `SEO-1`, which no longer exists: `handoff.md` is 14 lines and contains no sections. `:170` says
   to update `handoff.md` in the same pull request, directly contradicting `AGENTS.md:9`, `CONTRIBUTING.md:48` and
   `handoff.md:5-8`. An agent resuming the SEO work would edit a shared file it is told not to edit and then fail the
   `notes` check.

### S2: instructions that do not match the real checks, or missing stages

4. **"Run what CI runs" is not what CI runs.** `CONTRIBUTING.md:31-39` lists only `cd web && npm run check`, `go vet
   ./...` and `go test -race ./...`. The real suite in `image.yml:54-64` also runs `npm ci` and `npm run check` in
   `web`, `npm ci`, a fixture build and `npm test` in `mcp`, and then `go vet`; `image.yml:11-23` restricts
   pull-request runs by path, so a documentation-only pull request runs none of the Go suite at all. The docs checks in
   `docs.yml:61-78` and the browser suites in `terminal-browser.yml` are also absent. The PR template repeats the same
   short list.

5. **The pull-request template omits the two things CI enforces.** `.github/PULL_REQUEST_TEMPLATE.md:11-14` has no
   handoff-entry item (required by `notes.yml`) and no changelog-fragment item (advised by `check-notes.sh:21-25`). It
   also has no MCP test item, although `image.yml:58-62` runs that suite for every code pull request. A contributor who
   ticks every box can still fail `notes`.

6. **The staged workflow has no home in this repository.** `CONTRIBUTING.md:89-95` describes a five-step human pull
   request flow only. It does not define plan, code, review, pull request, babysit or merge stages, what a review must
   cover, when a fix is required, or who may merge. The only text in the tree that defines babysitting and review is
   `internal/api/builtin_skills/reet-babysit/SKILL.md:1-120`, which is a skill reet ships to its users, and historical
   plans such as `docs/superpowers/plans/2026-10-26-agents-home.md:78`, which require a model review and a clean Codex
   review but are not current instructions. This is missing guidance, not a conflict in existing text.

### S3: duplicated or stale guidance

7. **The two-file rule is written out six times.** `AGENTS.md:5-9`, `CONTRIBUTING.md:48`, `handoff.md:5-14`,
   `changelog.d/README.md:3-5`, `.github/workflows/notes.yml:3-5` and `scripts/check-notes.sh:2-9` all restate it in
   their own words. They agree today, so the risk is not a current contradiction but six edits for the next change and
   drift between them.

8. **`handoff.md` is cited 99 times across 42 Markdown files, and most citations no longer resolve.** The file is now
   14 lines. Examples with line or section references: `docs/research/paste-research.md:108-110`
   (`handoff.md:70`, `:62-64, 101`, `:104-106`), `docs/research/reference-tool-frontend-research.md:9, 94, 175`
   (`handoff.md:39-50`, `:50`), `docs/structured-chat-plan.md:11`, `docs/roadmap.md:9`,
   `docs/browser-skill-import.md:143`, `docs/private-skill-revisions.md:205`,
   `docs/machine-content-observations.md:91`, and `docs/ui-verification.md:5, 142, 162, 262, 277, 306, 347, 1923`.
   Public pages are the most damaging: `README.md:509` offers `handoff.md` as the place for setup-discovery limits, and
   `docs/terminal-workspace.md:562` and `:813` point readers at it for current priorities and "what to record". Active
   handoffs repeat the pattern: `docs/handoffs/seo-follow-up.md:7, 145, 170`,
   `docs/handoffs/gaffer-continuation.md:23, 136`, `docs/handoffs/gaffer-resume.md:70` and
   `docs/handoffs/gaffer-factory-floor.md:1549`. Historical citations in excluded pages are lower risk but inflate the
   count.

9. **Three log files break the heading contract in `handoff.md:6-7`.** `docs/handoffs/log/FACTORY-FINISHED-1.md:1`
   starts with the literal text `## ID:` instead of its entry name, and `docs/handoffs/log/FOCUS-RING-DARK-1.md:1` and
   `docs/handoffs/log/REAL-AGENT-CHECKS-2.md:1` use one `#` instead of `##`. A reader or script that finds an entry by
   `## <ID>:` will miss all three. No current script parses them, so this is mechanical rather than red.

10. **There are two historical archives with overlapping scopes and different conventions.**
    `docs/handoffs/archive.md:1-3` holds every entry from before the one-file layout, up to 24 November 2026.
    `handoff-history.md:1-7` is a second archive from 23 October 2026, 281,734 bytes at the repository root. It is
    outside `docs_dir: docs` (`mkdocs.yml:26`), so it is not published, but it is linked from
    `docs/handoffs/archive.md:1238, 3792, 4624, 4628, 4630, 4727, 4759` and from research notes. It tells readers to
    consult `handoff.md` for current state (`:6`) and references files removed in the docs tidy-up (`:3`, `:17`,
    `:1524`).

11. **Two adjacent process entries contradict each other in the log.** `docs/handoffs/log/NOTES-FRAGMENTS-1.md` ends
    with "Not done: nothing checks that a pull request ... added a fragment or a log entry";
    `docs/handoffs/log/CI-GUARDS-1.md` says `notes.yml` now does, with `scripts/check-notes.sh`. Both are dated
    24 November 2026. The later entry supersedes the earlier one, but nothing in the earlier file says so.

12. **`docs/roadmap.md:47-52` reads as both pending and built.** The delivery-speed item says "keep `handoff.md` to a
    bounded length with a short `AGENTS.md`" and then, in the same item, "Built: ... handoff entries and changelog
    fragments are one file each". The page is excluded from the site, so this is record-keeping noise rather than
    live guidance.

13. **`docs/ui-verification.md` and `docs/handoffs/*.md` mix current and historical instructions without a marker.**
    The 36 feature files outside `log/` and `archive.md` share one directory, from live continuation notes (`real-agent-checks.md`,
    `seo-follow-up.md`, `gaffer-continuation.md`, `gaffer-resume.md`) to closed design records. There is no index or
    status line, so a reader cannot tell which file still binds them. `docs/ui-verification.md` compounds this with
    nine references to sections of `handoff.md` that no longer exist.

14. **The real-agent probe scripts are not discoverable from the contribution guide.** Ten `scripts/probe-*.py` and
    `scripts/check-claude-credentials.py` start real agent CLIs (`cline --acp`, Codex adapters, Claude adapters) against
    isolated homes and stubs. They install nothing, and most make no model call, but
    `scripts/check-claude-credentials.py` can make a real, billable call when run with `prompt --yes-spend`
    (`docs/device-checks.md:134`). Their only instructions are `docs/device-checks.md:134, 159-171` and
    `docs/research/structured-adapters/probe-report.md`; `CONTRIBUTING.md` never mentions them. Someone reading only
    the contribution guide can miss both the scripts and the spending guard.

### S4: heavier than the work needs, or purely historical

15. **The planning record is larger than the live instructions.** `docs/superpowers/` holds 28 plans and 42 specs
    (about 1.2 MB), all excluded from the site. Thirty-two references in fifteen of them still instruct the reader to
    edit `handoff.md` (for example `docs/superpowers/plans/2026-10-26-agents-home.md:16, 76, 574`,
    `docs/superpowers/plans/2026-11-05-session-overview.md:24, 66, 109, 311` and
    `docs/superpowers/specs/2026-11-05-session-overview-design.md:243-245`). `docs/handoffs/archive.md:3786` records an
    earlier precedent: finished plans and specs were removed and left to git history.

16. **One one-off style record sits outside the handoff directories.** `docs/handoff-golang-code-style.md` is a
    149-line delivery record, excluded from the site by `mkdocs.yml:104`. It would fit with the other historical
    records, or not need to be a separate page at all.

## Improvements

Nothing here is implemented. "Mechanical" means wording or path corrections that do not change what an agent is
expected to do; "behaviour" means it would change how contributors or agents work and needs a maintainer's decision.

| # | Recommendation | Files it would touch | Benefit | Risk | Effort | Type |
| --- | --- | --- | --- | --- | --- | --- |
| A | Decide how a docs-only analysis passes `notes`: exempt it, or add an entry and accept a second new file | `scripts/check-notes.sh`, `.github/workflows/notes.yml`, or a new `docs/handoffs/log/*.md` | Unblocks this deliverable without silently breaking the rule | A check exemption weakens the "every change" rule; an entry adds a second file | Light | Behaviour |
| B | Correct the three broken log headings to `## <ID>: ...` | `docs/handoffs/log/FACTORY-FINISHED-1.md:1`, `FOCUS-RING-DARK-1.md:1`, `REAL-AGENT-CHECKS-2.md:1` | The documented lookup by entry ID works again | None | Light | Mechanical |
| C | Repoint the public and active dead references | `README.md:509`, `docs/terminal-workspace.md:562, 813`, `docs/control-centre.md:96-97`, `docs/handoffs/seo-follow-up.md:7, 145, 170`, `docs/handoffs/gaffer-continuation.md:23, 136`, `docs/handoffs/gaffer-resume.md:70`, `docs/handoffs/gaffer-factory-floor.md:1549` | Readers reach the current record instead of a 14-line index | A wrong target is still misleading; each link needs a current destination | Light | Mechanical |
| D | Align the contributor checks and PR template with `image.yml` and `docs.yml` | `CONTRIBUTING.md:31-39, 89-95`, `.github/PULL_REQUEST_TEMPLATE.md:11-14` | Local checks match CI; the template states the handoff and fragment rules CI enforces | A longer checklist may slow small changes; keep the docs-only exception explicit | Light | Mechanical |
| E | Keep the two-file rule in `AGENTS.md` only and replace the repeats with links | `CONTRIBUTING.md:48`, `handoff.md:5-14`, `changelog.d/README.md:3-5`, `notes.yml:3-5`, `check-notes.sh:2-9` | One place to change; no drift | A reader landing on a secondary file must follow one link | Light | Mechanical |
| F | Add a short "Manual agent probes" subsection to the contribution guide | `CONTRIBUTING.md` | The probe scripts and the `--yes-spend` guard become discoverable | None if it links rather than duplicates | Light | Mechanical |
| G | Write down the staged workflow: plan, code, review, PR, babysit, merge, with the review standard and merge rule | `AGENTS.md` or a new `docs/agent-workflow.md`; link from `CONTRIBUTING.md:89-95` | Agents stop inferring a workflow from a product skill and old plans | It would define behaviour; maintainers must agree the stages and the merge rule | Standard | Behaviour |
| H | Mark each handoff outside `log/` as active or historical, or index the live ones in `handoff.md` | `handoff.md` plus front matter or a status line in `docs/handoffs/*.md` | A reader can tell binding instructions from records | Touches many files; status can go stale | Standard | Behaviour |
| I | Consolidate the two archives, or move `handoff-history.md` under `docs/handoffs/` and exclude it | `handoff-history.md`, `docs/handoffs/archive.md`, research links | One archive, one convention, and the 281 KB root file stops looking current | Rewriting links is error-prone; the history is cited in many places | Heavy | Behaviour |
| J | Retire finished plans and specs to git history, as the 27 October tidy-up did | `docs/superpowers/plans/*`, `docs/superpowers/specs/*`, `mkdocs.yml:99-116` | Removes about 1.2 MB of instructions that no longer apply | Loses an in-tree record; needs a link audit first | Heavy | Behaviour |
| K | If this review should not be public, add it to the docs exclusions | `mkdocs.yml:99-116` | Keeps an internal process review off `reet.sh` and out of the sitemap | It would no longer be readable on the public docs site | Light | Behaviour |
| L | Consider a check that a pull request does not edit `handoff.md` or `CHANGELOG.md` | `scripts/check-notes.sh` | Makes the "do not edit shared files" rule enforceable rather than advisory | A legitimate maintainer edit would need an override | Standard | Behaviour |

Order of work: A unblocks the current deliverable. B, C, D, E and F are mechanical and can be one small pull request
each. G is the largest gap in live guidance and should be agreed before H, I or J, because those change where the
workflow record lives. K is a decision about this document, not about the workflow.

## Inspected and irrelevant

These were opened to check whether they drive coding-agent work in this repository. They do not, and the next reader
can skip them:

- Product documentation about reet's own agents: `docs/agent-accounts.md`, `docs/agent-models.md`,
  `docs/agent-software.md`, `docs/start-agent-session.md`, `docs/structured-sessions.md`,
  `docs/structured-chat-plan.md`, `docs/factory.md`, `docs/gaffer-ai.md`,
  `docs/parallel-coding-agents-git-worktrees.md` and `docs/device-checks.md` as a product page.
- `docs/handoffs/real-agent-checks.md`, which is a manual QA list for the product, not a coding workflow.
- User guides and comparison pages: `docs/guides/**`, `docs/compare/**`, `docs/self-hosted-ai-agents-checklist.md`,
  `docs/self-hosted-ai-agent-control-room.md`.
- Product audits and reviews: `PRODUCT.md`, `SEO_AUDIT.md`, `UI_AUDIT.md`, `docs/ux-review.md`.
- The built-in skills under `internal/api/builtin_skills/` (14 skills). They are shipped product content. One of them,
  `reet-babysit`, describes a pull-request workflow, but it is not maintained as this repository's workflow and its
  `:42` preference for `handoff.md` only applies when repository conventions do not say otherwise.
- The remaining workflows named in inventory item 24, which run product tests, benchmarks, releases or cleanup and do
  not start a coding agent.
- `.github/ISSUE_TEMPLATE/*`, `.github/dependabot.yml` and `.github/scripts/*`, which handle contributor intake,
  dependency updates and repository housekeeping.
- `docs/handoffs/*.md` feature records other than the four named as live in finding 13. They are product history.

## Method and limits

- Every citation was read at this revision, and line numbers come from `nl -ba` on the file named.
- Counts: 51 files in `docs/handoffs/log/`; 36 feature files in `docs/handoffs/` plus `archive.md`; 28 plans and 42
  specs; 99 references
  to `handoff.md` in 42 Markdown files; 10 `scripts/probe-*.py` scripts plus `scripts/check-claude-credentials.py`.
- `handoff-history.md` was confirmed absent from the built site because it sits outside `docs_dir: docs`
  (`mkdocs.yml:26`), not because of an exclusion rule.
- Not verified: organisation-level rulesets and branch protection, so the claim that a conflicted pull request runs
  "only CodeQL" (`AGENTS.md:32-33`) cannot be checked from this repository; and whether maintainers still consider the
  real-agent checks and probe scripts current after the latest adapter changes.
- Checks run for this document: `mkdocs build --strict` (passed; the new page is an INFO-level unlisted page, as
  `docs/ux-review.md` already is), `scripts/check-docs.py` (76 pages passed), `scripts/check-docs-browser.py` (passed)
  and the `hooks` unit tests with the docs virtual environment (31 passed). The Go, web and MCP suites were not run
  because no application code changed.
- The `notes` check fails on this branch by design; finding 1 and recommendation A record that rather than hiding it.
