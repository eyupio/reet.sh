#!/usr/bin/env bash
set -euo pipefail

: "${REET_TAG:?REET_TAG is required}"
[[ "$REET_TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]] || { echo 'Expected a version tag such as v1.2.3' >&2; exit 1; }
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
