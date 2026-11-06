#!/usr/bin/env bash
# Keep each revision's samples close together on a shared CI runner.
set -euo pipefail
if test "$#" != 3; then
  echo 'Usage: scripts/bench-compare.sh BASE_DIR PR_DIR OUTPUT_DIR' >&2
  exit 2
fi
bench_base="$(cd "$1" && pwd)"
bench_pr="$(cd "$2" && pwd)"
mkdir -p "$3"
bench_out="$(cd "$3" && pwd)"
bench_work="$(mktemp -d "${TMPDIR:-/tmp}/reet-bench.XXXXXXXX")"
trap 'rm -rf "$bench_work"' EXIT
bench_regex="${BENCH_REGEX:-^Benchmark(Community|ListFor|ListForCompact|ListSkillsAPI(/.*)?|TerminalSessionsFor|AppendTerminalChatEventFor|AppendTerminalChatEventsFor32|PublishChatEvent)$}"
bench_packages=(api store terminalbroker)

# Finish all compilation before measuring either revision. Each binary runs in
# its package directory, matching go test's working-directory contract.
for bench_side in base pr; do
  bench_tree="$bench_base"
  if test "$bench_side" = pr; then bench_tree="$bench_pr"; fi
  for bench_package in "${bench_packages[@]}"; do
    (cd "$bench_tree" && go test -c -o "$bench_work/$bench_side-$bench_package.test" "./internal/$bench_package")
  done
done
: > "$bench_out/base.txt"
: > "$bench_out/new.txt"
for bench_round in {1..8}; do
  bench_order=(base pr)
  if (( bench_round % 2 == 0 )); then bench_order=(pr base); fi
  for bench_package in "${bench_packages[@]}"; do
    for bench_side in "${bench_order[@]}"; do
      bench_tree="$bench_base"
      bench_file="$bench_out/base.txt"
      if test "$bench_side" = pr; then bench_tree="$bench_pr"; bench_file="$bench_out/new.txt"; fi
      printf '%s: %s, sample %s/8\n' "$bench_side" "$bench_package" "$bench_round"
      (cd "$bench_tree/internal/$bench_package" && "$bench_work/$bench_side-$bench_package.test" \
        -test.run '^$' -test.bench "$bench_regex" -test.benchmem -test.count=1 -test.benchtime=1s) \
        | tee -a "$bench_file"
    done
  done
done
