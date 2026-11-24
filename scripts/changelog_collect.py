#!/usr/bin/env python3
"""Move changelog.d/*.md fragments into CHANGELOG.md under "Unreleased".

    python3 scripts/changelog_collect.py            show what would change
    python3 scripts/changelog_collect.py --write    do it and delete the fragments
"""
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
FRAGMENTS = ROOT / "changelog.d"
CHANGELOG = ROOT / "CHANGELOG.md"
HEADING = "## Unreleased\n"


def added_order(path):
    """When the fragment was first committed, so the newest change sits on top; falls back to the name."""
    out = subprocess.run(
        ["git", "log", "--diff-filter=A", "--format=%ct", "--", str(path)],
        cwd=ROOT, capture_output=True, text=True,
    ).stdout.split()
    return (int(out[-1]) if out else 0, path.name)


def collect():
    files = sorted((f for f in FRAGMENTS.glob("*.md") if f.name != "README.md"), key=added_order, reverse=True)
    bullets = []
    for f in files:
        text = f.read_text().strip("\n")
        if not text.startswith("- "):
            sys.exit(f"{f.relative_to(ROOT)}: a fragment is one bullet and must start with '- '")
        bullets.append(text)
    return files, bullets


def main():
    write = "--write" in sys.argv[1:]
    files, bullets = collect()
    if not files:
        print("no fragments")
        return
    changelog = CHANGELOG.read_text()
    if HEADING not in changelog:
        sys.exit(f"CHANGELOG.md has no '{HEADING.strip()}' heading")
    head, rest = changelog.split(HEADING, 1)
    updated = head + HEADING + "\n" + "\n".join(bullets) + "\n" + ("\n" + rest.lstrip("\n") if rest.strip() else "")
    for f in files:
        print(("moved " if write else "would move ") + str(f.relative_to(ROOT)))
    if write:
        CHANGELOG.write_text(updated)
        for f in files:
            f.unlink()


if __name__ == "__main__":
    main()
