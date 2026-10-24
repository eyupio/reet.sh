#!/usr/bin/env bash
# Exercise artifacts-prune.sh with fake GitHub responses: old artifacts in
# completed runs go, but recent ones, the newest of each name, a running run,
# the latest successful default-branch run and an unreadable run stay.
set -euo pipefail
# shellcheck source-path=SCRIPTDIR source=artifacts-prune.sh
source "$(dirname "$0")/artifacts-prune.sh"

old=2020-01-01T00:00:00Z
new=$(date -u +%Y-%m-%dT%H:%M:%SZ)

fixture=$(cat <<JSON
[
 {"id":10,"name":"terminal-browser","created_at":"$old","workflow_run":{"id":100}},
 {"id":11,"name":"terminal-browser","created_at":"$old","workflow_run":{"id":101}},
 {"id":12,"name":"terminal-browser","created_at":"$old","workflow_run":{"id":102}},
 {"id":13,"name":"terminal-browser","created_at":"$old","workflow_run":{"id":103}},
 {"id":14,"name":"terminal-browser","created_at":"$old","workflow_run":{"id":104}},
 {"id":30,"name":"terminal-browser","created_at":"$new","workflow_run":{"id":300}},
 {"id":20,"name":"release-web","created_at":"$old","workflow_run":{"id":200}},
 {"id":21,"name":"release-web","created_at":"$old","workflow_run":{"id":201}},
 {"id":22,"name":"release-web","created_at":"$old","workflow_run":{"id":202}},
 {"id":23,"name":"release-web","created_at":"$old","workflow_run":{"id":203}},
 {"id":24,"name":"release-web","created_at":"$old","workflow_run":{"id":204}},
 {"id":25,"name":"release-web","created_at":"$old","workflow_run":{"id":205}},
 {"id":40,"name":"logs","created_at":"$old","workflow_run":{"id":500}},
 {"id":50,"name":"benchmark","created_at":"$old","workflow_run":{"id":900}},
 {"id":60,"name":"debug","created_at":"$old","workflow_run":{"id":600}},
 {"id":80,"name":"no-run","created_at":"$old","workflow_run":null}
]
JSON
)

list_artifacts() { printf "%s\n" "$fixture"; }
latest_success_run() { echo 900; }
fetch_run() {
  case $1 in
    500) echo '{"status":"in_progress"}' ;;
    600) return 1 ;;
    *) echo '{"status":"completed","conclusion":"success"}' ;;
  esac
}
deleted=()
delete_artifact() { deleted+=("$1"); }

DRY_RUN=false KEEP_DAYS=14 KEEP_NEWEST=5 DEFAULT_BRANCH=main
prune >/dev/null
got=$(printf '%s\n' "${deleted[@]}" | sort -n | tr '\n' ' ')
want="10 20 "
if [ "$got" != "$want" ]; then
  echo "FAIL: deleted [$got], want [$want]"
  exit 1
fi

deleted=()
DRY_RUN=true
prune >/dev/null
if [ ${#deleted[@]} -ne 0 ]; then
  echo "FAIL: a dry run deleted ${deleted[*]}"
  exit 1
fi
echo "artifacts-prune: ok"
