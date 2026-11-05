#!/usr/bin/env python3
"""Fail a pull request when a benchmark regresses beyond a threshold.

Reads two `go test -bench` outputs, asks benchstat for the A/B comparison, and
exits non-zero when a sec/op change is both larger than the threshold and
statistically significant (p < 0.05). Missing benchmarks on either side are a
skip, not a failure: a new benchmark has no base to compare with.
"""

import csv
import io
import re
import subprocess
import sys

PERCENT = re.compile(r"^([+-])([0-9]+(?:\.[0-9]+)?)%$")
P_VALUE = re.compile(r"p=([0-9]+(?:\.[0-9]+)?)")


def read_benchmarks(path):
    names = set()
    with open(path, encoding="utf-8", errors="replace") as handle:
        for line in handle:
            if line.startswith("Benchmark"):
                names.add(line.split()[0])
    return names


def comparison(base, new):
    result = subprocess.run(
        ["benchstat", "-format=csv", base, new],
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode != 0 and result.stderr.strip():
        print(result.stderr.strip(), file=sys.stderr)
    rows = list(csv.reader(io.StringIO(result.stdout)))
    in_sec = False
    out = []
    for row in rows:
        if len(row) >= 2 and row[1].strip() == "sec/op":
            in_sec = True
            continue
        if not in_sec:
            continue
        if not any(cell.strip() for cell in row):
            in_sec = False
            continue
        if len(row) < 7:
            continue
        out.append((row[0].strip(), row[5].strip(), row[6].strip()))
    return out


def main():
    if len(sys.argv) != 4:
        print("usage: bench-gate.py BASE.txt NEW.txt THRESHOLD_PERCENT", file=sys.stderr)
        return 2
    base, new = sys.argv[1], sys.argv[2]
    threshold = float(sys.argv[3])

    base_names = read_benchmarks(base)
    new_names = read_benchmarks(new)
    shared = base_names & new_names
    if not shared:
        print("bench-gate: no benchmark exists on both sides; skipping")
        return 0

    failures = []
    checked = 0
    for name, change, p in comparison(base, new):
        if name == "geomean" or change in ("", "~"):
            continue
        if name not in shared and "Benchmark"+name not in shared:
            continue
        match = PERCENT.match(change)
        p_match = P_VALUE.search(p)
        if not match or not p_match:
            continue
        percent = float(match.group(2))
        if match.group(1) == "-":
            percent = -percent
        checked += 1
        print(f"{name}: {change} ({p})")
        if percent > threshold and float(p_match.group(1)) < 0.05:
            failures.append(f"{name}: {change} ({p})")

    if failures:
        print("\nbench-gate: regressions over %.1f%%:" % threshold, file=sys.stderr)
        for failure in failures:
            print("  " + failure, file=sys.stderr)
        return 1
    print(f"\nbench-gate: {checked} shared benchmark(s), no regression over {threshold:.1f}%")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
