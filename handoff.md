# Reet central-control implementation handoff

Each change leaves a note here for whoever works on it next: what was built, why, what was checked, and what was not.

**New entries are one file each** in [docs/handoffs/log/](docs/handoffs/log/), named for the entry's ID
(`CLONE-TEST-1.md`), so two pull requests never edit the same file and never conflict. Start the file with
`## ID: the sentence that says what changed (date)`, as the older entries do. Newest entries are the newest files by
date in their heading; `git log --diff-filter=A -- docs/handoffs/log` lists them in the order they landed.

**Entries written before this layout** are in [docs/handoffs/archive.md](docs/handoffs/archive.md), newest first. Other
pages cite them by ID (CREW-CHOICES-1, DESKS-1, MOBILE-2 and so on); search that file for the ID.

User-facing changes also leave a changelog fragment in [changelog.d/](changelog.d/), for the same reason. See
[CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md).
