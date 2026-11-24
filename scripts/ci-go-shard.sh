#!/bin/sh
# Runs one shard of the Go suite under the race detector, so CI can run the shards on separate runners at once.
#
#   scripts/ci-go-shard.sh store|api-1|api-2|gaffer-1|gaffer-2|rest [--print]
#
# internal/api and internal/gaffer are the two slowest packages under -race (about 26 and 21 minutes), and
# internal/store the third (about 10), so each gets its own runner. api and gaffer are split in two: the -1 and -2
# shards take alternate tests of one `go test -list`, which stays balanced as tests are added or removed. rest is every
# other package. Together they run each test exactly once. --print shows what would run.
set -eu

shard=${1:?usage: ci-go-shard.sh store|api-1|api-2|gaffer-1|gaffer-2|rest [--print]}
print=${2:-}
run() {
	if [ "$print" = "--print" ]; then echo "go test $*"; else go test "$@"; fi
}

half() { # half <package path> <1|2>
	tests=$(go test -list '^Test' "$1" | grep '^Test' | awk -v n="$2" 'NR % 2 == n % 2' | paste -sd'|' -)
	[ -n "$tests" ] || { echo "no tests listed for $1" >&2; exit 1; }
	run -race -timeout=40m -run "^($tests)\$" "$1"
}

case "$shard" in
store)
	run -race -timeout=40m ./internal/store/...
	;;
api-1 | api-2)
	half ./internal/api "${shard#api-}"
	;;
gaffer-1 | gaffer-2)
	half ./internal/gaffer "${shard#gaffer-}"
	;;
rest)
	pkgs=$(go list ./... | grep -v -e '/internal/store$' -e '/internal/store/' -e '/internal/api$' -e '/internal/gaffer$')
	# shellcheck disable=SC2086
	run -race -timeout=30m $pkgs
	;;
*)
	echo "unknown shard: $shard" >&2
	exit 1
	;;
esac
