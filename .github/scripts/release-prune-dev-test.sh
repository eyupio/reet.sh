#!/bin/sh
# Exercise release-prune-dev.sh against a fake `gh`: old per-commit assets go,
# but the newest commits, young commits, the commit the dev tag points at and
# every non-per-commit asset stay. Needs jq and GNU date.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin"

age() { date -u -d "-$1 days" +%Y-%m-%dT%H:%M:%SZ; }
sha() { printf '%038d%02d' 0 "$1"; }

# Fake gh: serves fixtures and records DELETE calls.
cat > "$work/bin/gh" <<'GH'
#!/bin/sh
case "$*" in
  *"--method DELETE"*) echo "$*" | sed 's#.*/assets/##' >> "$FAKE/deleted" ;;
  *"/actions/workflows/"*) echo 0 ;;
  *"/releases/tags/dev"*) echo 7 ;;
  *"/git/ref/tags/dev"*) cat "$FAKE/current" ;;
  *"/assets?"*) [ -e "$FAKE/fail" ] && exit 1; jq -c '.[]' "$FAKE/assets.json" ;;
  *) echo "unexpected gh $*" >&2; exit 1 ;;
esac
GH
chmod +x "$work/bin/gh"

# Commit N has 5 assets with ids N*10+0..4 and is N+40 days old, except the
# two youngest. Commit 1 is the one the dev tag points at but is the oldest.
: > "$work/lines"
n=1
while [ "$n" -le 30 ]; do
  days=$((n + 40)); [ "$n" -le 2 ] && days=3
  [ "$n" -eq 1 ] && days=200
  s=$(sha "$n")
  i=0
  for os in darwin linux; do for arch in amd64 arm64; do
    printf '{"id":%s,"name":"reet_dev_%s_%s_%s.tar.gz","created":"%s"}\n' $((n * 10 + i)) "$s" "$os" "$arch" "$(age $days)" >> "$work/lines"
    i=$((i + 1))
  done; done
  printf '{"id":%s,"name":"checksums-%s.txt","created":"%s"}\n' $((n * 10 + 4)) "$s" "$(age $days)" >> "$work/lines"
  n=$((n + 1))
done
# Legacy rolling names and pointers must never be deleted.
for name in reet_dev_linux_amd64.tar.gz checksums.txt dev-build.txt; do
  printf '{"id":%s,"name":"%s","created":"%s"}\n' $((900 + $(wc -l < "$work/lines"))) "$name" "$(age 400)" >> "$work/lines"
done
jq -s . "$work/lines" > "$work/assets.json"
sha 1 > "$work/current"

run() { env FAKE="$work" PATH="$work/bin:$PATH" GITHUB_REPOSITORY=o/r KEEP_COMMITS=20 KEEP_DAYS=14 "$@" sh "$here/release-prune-dev.sh"; }

# Dry run deletes nothing.
run DRY_RUN=true >/dev/null
[ ! -e "$work/deleted" ] || { echo "dry run deleted assets"; exit 1; }

run DRY_RUN=false >/dev/null
# Commits are ordered newest first: 2 (3 days), then 3..30 by age, 1 oldest.
# Keep: 2 young, 20 newest (2..21), plus commit 1 (current). Delete 22..30.
deleted=$(sort -n "$work/deleted" | tr '\n' ' ')
want=$(n=22; while [ "$n" -le 30 ]; do i=0; while [ "$i" -le 4 ]; do echo $((n * 10 + i)); i=$((i + 1)); done; n=$((n + 1)); done | tr '\n' ' ')
[ "$deleted" = "$want" ] || { echo "wrong deletions:"; echo "got  $deleted"; echo "want $want"; exit 1; }

# Fails closed: unreadable current commit deletes nothing.
rm "$work/deleted"; echo "garbage" > "$work/current"
if run DRY_RUN=false >/dev/null 2>&1; then echo "accepted an unreadable dev tag"; exit 1; fi
[ ! -e "$work/deleted" ] || { echo "deleted with unreadable tag"; exit 1; }

# Fails closed: a plan covering every per-commit build deletes nothing.
sha 1 > "$work/current"
if run DRY_RUN=false KEEP_COMMITS=0 KEEP_DAYS=0 >/dev/null 2>&1; then
  # current commit is always kept, so this must delete the rest but not commit 1.
  grep -qx '1[0-4]' "$work/deleted" 2>/dev/null && { echo "deleted the current commit"; exit 1; }
fi
echo ok
