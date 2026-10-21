#!/usr/bin/env bash
set -euo pipefail

: "${REET_TAG:?REET_TAG is required}"
[[ "$REET_TAG" = dev || "$REET_TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]] || { echo 'Expected a version tag such as v1.2.3' >&2; exit 1; }
cd "${1:-artifacts}"
files=()
for os in darwin linux; do
  for arch in amd64 arm64; do
    file="reet_${REET_TAG}_${os}_${arch}.tar.gz"
    test -f "$file" || { echo "Missing release archive: $file" >&2; exit 1; }
    files+=("$file")
  done
done
sha256sum "${files[@]}" > checksums.txt
files+=(checksums.txt)

# Only the explicitly rolling dev channel may replace published assets.
if [ "$REET_TAG" = dev ]; then
  : "${REET_SHA:?REET_SHA is required for dev releases}"
  [[ "$REET_SHA" =~ ^[0-9a-f]{40}$ ]] || { echo 'Expected a full source commit' >&2; exit 1; }
  repo=${GITHUB_REPOSITORY:-eyupio/reet.sh}
  if gh api "repos/$repo/git/ref/tags/dev" >/dev/null 2>&1; then
    gh api --method PATCH "repos/$repo/git/refs/tags/dev" -f sha="$REET_SHA" -F force=true >/dev/null
  else
    gh api --method POST "repos/$repo/git/refs" -f ref=refs/tags/dev -f sha="$REET_SHA" >/dev/null
  fi
  notes="Rolling development build from main at $REET_SHA. This prerelease is replaced after each merge; use a stable version for production."
  if gh release view dev >/dev/null 2>&1; then
    gh release upload dev "${files[@]}" --clobber
    gh release edit dev --prerelease --latest=false --title 'Development (main)' --notes "$notes"
  else
    gh release create dev "${files[@]}" --verify-tag --prerelease --latest=false --title 'Development (main)' --notes "$notes"
  fi
  exit 0
fi

scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
if gh release view "$REET_TAG" --json assets --jq '.assets[].name' > "$scratch/names"; then
  missing=()
  # Validate every existing asset before uploading anything. Never replace
  # published bytes or mix a new checksum manifest with different archives.
  for file in "${files[@]}"; do
    if grep -Fxq -- "$file" "$scratch/names"; then
      gh release download "$REET_TAG" --pattern "$file" --dir "$scratch"
      cmp -s "$file" "$scratch/$file" || { echo "Published asset differs: $file. Use a new version tag." >&2; exit 1; }
    else
      missing+=("$file")
    fi
  done
  if ((${#missing[@]})); then
    gh release upload "$REET_TAG" "${missing[@]}"
  fi
else
  flags=()
  case "${REET_TAG%%+*}" in *-*) flags+=(--prerelease) ;; esac
  gh release create "$REET_TAG" "${files[@]}" --verify-tag --generate-notes --title "$REET_TAG" "${flags[@]}"
fi
