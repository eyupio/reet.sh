#!/bin/sh
# Runs one shard of the Go suite under the race detector, so CI can run the shards on separate runners at once.
#
#   scripts/ci-go-shard.sh store|api-1|api-2|rest [--print]
#
# store is internal/store alone, the slowest package after api. api-1 and api-2 split internal/api's tests between
# them by their position in `go test -list`, which is stable until a test is added or removed (then the halves
# rebalance themselves). rest is every other package. Together they run each test exactly once: api-1 and api-2 take
# alternate entries of one list, and rest is everything that is neither store nor api. --print shows what would run.
set -eu

shard=${1:?usage: ci-go-shard.sh store|api-1|api-2|rest [--print]}
print=${2:-}
run() {
	if [ "$print" = "--print" ]; then echo "go test $*"; else go test "$@"; fi
}

case "$shard" in
store)
	run -race -timeout=40m ./internal/store/...
	;;
api-1 | api-2)
	n=${shard#api-}
	tests=$(go test -list '^Test' ./internal/api | grep '^Test' | awk -v n="$n" 'NR % 2 == n % 2' | paste -sd'|' -)
	[ -n "$tests" ] || { echo "no tests listed for $shard" >&2; exit 1; }
	run -race -timeout=40m -run "^($tests)\$" ./internal/api
	;;
rest)
	pkgs=$(go list ./... | grep -v -e '/internal/store$' -e '/internal/store/' -e '/internal/api$')
	# shellcheck disable=SC2086
	run -race -timeout=30m $pkgs
	;;
*)
	echo "unknown shard: $shard" >&2
	exit 1
	;;
esac
