#!/bin/sh
# Runs browser suites from web/tests and reports every failure, not only the first.
#
#   scripts/run-browser-suites.sh NAME...                          run web/tests/NAME.e2e.mjs for each NAME
#   scripts/run-browser-suites.sh --list FILE --shard N --of M     run shard N of M of the names in FILE (one a line)
#
# A suite that fails does not stop the rest. Its output is printed again after its group so it is not folded away, and
# the run ends by naming every suite that failed. The exit status is 1 if any did. The preview server and
# REET_UI_URL are the caller's to set up, as they are for running a suite by hand.
set -u

list="" shard="" of=""
while [ $# -gt 0 ]; do
	case "$1" in
	--list) list=$2; shift 2 ;;
	--shard) shard=$2; shift 2 ;;
	--of) of=$2; shift 2 ;;
	--) shift; break ;;
	*) break ;;
	esac
done

if [ -n "$list" ]; then
	[ -n "$shard" ] && [ -n "$of" ] || { echo "--list needs --shard and --of" >&2; exit 2; }
	names=$(grep -v '^[[:space:]]*$' "$list" | awk -v n="$shard" -v m="$of" '(NR - 1) % m == n - 1')
else
	names="$*"
fi
[ -n "$names" ] || { echo "no suites to run" >&2; exit 2; }

out=$(mktemp)
trap 'rm -f "$out"' EXIT
failed=""
total=0
for name in $names; do
	total=$((total + 1))
	echo "::group::$name"
	if node "web/tests/$name.e2e.mjs" >"$out" 2>&1; then
		cat "$out"
		echo "::endgroup::"
	else
		cat "$out"
		echo "::endgroup::"
		echo "::error title=$name failed::web/tests/$name.e2e.mjs failed; the end of its output follows."
		tail -n 40 "$out"
		failed="$failed $name"
	fi
done

if [ -n "$failed" ]; then
	echo "Failed:$failed" >&2
	echo "$(echo "$failed" | wc -w | tr -d ' ') of $total suites failed." >&2
	exit 1
fi
echo "All $total suites passed."
