#!/bin/sh
# Fails a pull request that leaves no handoff entry, and says when a change that people running reet would notice leaves no
# changelog fragment. Both are new files, so two pull requests never conflict on them (see AGENTS.md).
#
#   scripts/check-notes.sh [BASE]     BASE is the branch to compare with (default origin/main)
#
# A handoff entry is a new docs/handoffs/log/*.md. A changelog fragment is a new changelog.d/*.md other than README.md. The
# fragment is only advised, because only a person can say whether a change is visible to someone running reet; the entry is
# required for every change.
set -eu

base=${1:-origin/main}
added=$(git diff --diff-filter=A --name-only "$base"...HEAD)

if ! echo "$added" | grep -q '^docs/handoffs/log/[^/]*\.md$'; then
	echo "::error title=No handoff entry::Add docs/handoffs/log/<ID>.md saying what changed, why, what was checked and what was not (see AGENTS.md)."
	echo "This pull request adds no handoff entry. Add docs/handoffs/log/<ID>.md (see handoff.md and AGENTS.md)." >&2
	exit 1
fi

changed=$(git diff --name-only "$base"...HEAD)
fragments=$(echo "$added" | grep -E '^changelog\.d/[^/]*\.md$' | grep -vc 'README\.md$' || true)
if [ "$fragments" -eq 0 ] && echo "$changed" | grep -Eq '^(web/src/|internal/api/|cmd/)'; then
	echo "::notice title=No changelog fragment::If a person running reet would notice this change, add changelog.d/<name>.md (see changelog.d/README.md)."
fi
echo "Handoff entry found."
