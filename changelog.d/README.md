# Changelog fragments

A change that matters to someone running a reet controller adds one file here instead of editing `CHANGELOG.md`, so two
pull requests never conflict on it (the rule is in [AGENTS.md](../AGENTS.md)). Name it for the change (`machine-lock.md`). It holds one bullet, written for the
person running reet (no internal names), starting with a bold lead:

```
- **Lock a machine from the machine**: `reet agent lock`, run on a machine, makes it refuse ...
```

Wrap lines at about 120 characters and indent continuation lines by two spaces, as `CHANGELOG.md` does.

When a release is cut, `python3 scripts/changelog_collect.py --write` moves every fragment (this file excepted) into
`CHANGELOG.md` under "Unreleased", oldest first at the bottom, and deletes the fragment files. Without `--write` it
prints what it would do. It stops and says which file when a fragment does not start with `- `.
