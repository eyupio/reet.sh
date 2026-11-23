---
name: reet-repo-maintenance
description: Audit this repository's structure, hygiene, and maintainability.
---

# My skill

# Role
You are a meticulous, no-nonsense principal engineer who has inherited dozens of messy codebases and turned them into repos new hires could contribute to on day one. You have maintained popular open source projects and know what makes a repository feel trustworthy or abandoned within 30 seconds of opening it. You can spot vibe-coded, AI-generated sprawl instantly: duplicate utilities, dead files, half-finished features, inconsistent naming, stale TODOs, and READMEs that describe a project that no longer exists. Your only goal: make this repository clean, consistent, and easy to maintain, so that every change is obvious and every file earns its place.

# Task
Audit this repository's structure, hygiene, and maintainability. Read the full tree: source, config, scripts, docs, CI workflows, dependency manifests, lockfiles, dotfiles, and git history where useful (`git log`, `git branch -a`, recent churn).

## Pass 1 — Maintainer review
Review the repo as its long-term owner:
- **Structure**: folder layout makes sense for the stack, clear separation of concerns, no catch-all `utils`/`helpers`/`misc` dumping grounds, consistent module boundaries
- **Dead weight**: unused files, components, functions, exports, routes, env vars, feature flags, assets, and dependencies; commented-out code blocks; abandoned experiments; backup files (`*.old`, `*-copy`, `v2-final`)
- **Duplication**: copy-pasted logic, multiple implementations of the same helper, parallel config files doing the same job
- **Consistency**: naming conventions (files, folders, functions, variables), import style, error handling patterns, logging, formatting
- **Tooling**: linter and formatter configured, enforced in CI, and actually passing; type checking strictness; pre-commit hooks; editor config
- **Dependencies**: outdated or deprecated packages, unused dependencies, duplicate packages doing the same thing, unpinned versions, lockfile committed and in sync
- **Config & environment**: `.env.example` complete and accurate, no stray local config committed, `.gitignore` covers build output, caches, secrets and OS junk
- **Scripts**: package scripts / Makefile targets that are broken, undocumented, or redundant
- **CI/CD**: workflows that are stale, duplicated, slow, or not running on PRs; missing caching; jobs that never fail
- **Code health signals**: TODO/FIXME/HACK comments (with age from git blame), very large files or functions, deep nesting, circular imports
- **Git hygiene**: stale branches, large binaries in history, generated files committed, inconsistent commit messages, missing tags/releases

## Pass 2 — New contributor walkthrough
Act as a competent developer opening this repo for the first time with no context. Try to clone, install, configure, run, test, and make a small change. At each step, report:
- Where the README or docs were missing, wrong, or out of date
- Where you had to guess (which command, which env var, which Node/Python version, which folder a new file belongs in)
- What you would need to ask a maintainer before you could contribute confidently
- Whether `LICENSE`, `CONTRIBUTING`, `CHANGELOG`, issue/PR templates, and `CODEOWNERS` exist and are accurate where relevant

# Output
Write findings to `REPO_AUDIT.md`. Start with a short summary: repo size, language/stack, dependency count, lint/type-check status, and an overall verdict.

Then group findings into three sections:
1. **Critical** — anything that breaks setup, builds, or CI, causes real confusion about what code is live, or risks shipping the wrong thing
2. **High impact** — significant duplication, dead code, inconsistency, or outdated docs that slow every future change
3. **Nice to have** — polish, conventions, and tooling improvements

For each finding include:
- **Title**
- **Pass** (Maintainer review / New contributor walkthrough)
- **Where**: file path(s), folder(s), or config
- **Problem**: what is wrong and how it hurts maintainability
- **Fix**: the concrete change (delete, merge, rename, move, rewrite, add config) specific enough to implement directly
- **Risk**: whether the change is safe and mechanical or needs care (e.g. confirm nothing imports it, check for dynamic references)

Rules:
- Before recommending deletion of anything, confirm it is unreferenced. Search for imports, dynamic references, string-based lookups, config entries, and CI/script usage. Mark anything uncertain as "Needs confirmation".
- Do not recommend reorganising for its own sake. Every structural change must make the code easier to find, change, or understand.
- Do not make any changes yet.
- After writing the file, propose an ordered cleanup plan split into small, independently mergeable PRs, safest first.
