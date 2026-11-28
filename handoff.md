# Reet central-control implementation handoff

Each change leaves a note here for whoever works on it next: what was built, why, what was checked, and what was not.

**New entries are one file each** in [docs/handoffs/log/](docs/handoffs/log/), named for the entry's ID
(`CLONE-TEST-1.md`), so two pull requests never edit the same file and never conflict. Start the file with
`## ID: the sentence that says what changed (date)`, as the older entries do. Newest entries are the newest files by
date in their heading; `git log --diff-filter=A -- docs/handoffs/log` lists them in the order they landed.

**Entries written before this layout** are in [docs/handoffs/archive.md](docs/handoffs/archive.md), newest first. Other
pages cite them by ID (CREW-CHOICES-1, DESKS-1, MOBILE-2 and so on); search that file for the ID.

The live continuation notes are [real-agent-checks.md](docs/handoffs/real-agent-checks.md),
[seo-follow-up.md](docs/handoffs/seo-follow-up.md), [gaffer-continuation.md](docs/handoffs/gaffer-continuation.md) and
[gaffer-resume.md](docs/handoffs/gaffer-resume.md). The rest of `docs/handoffs/` outside `log/` and `archive.md` is
historical.

The two-file rule is in [AGENTS.md](AGENTS.md); changelog fragments are in [changelog.d/](changelog.d/).
