#!/usr/bin/env bash
# Delete Actions artifacts that nothing still needs.
#
# Usage: artifacts-prune.sh
#    env: GH_TOKEN (actions:write), GITHUB_REPOSITORY, DRY_RUN (default true),
#         KEEP_DAYS (default 14), KEEP_NEWEST (default 5),
#         DEFAULT_BRANCH (default main)
#
# An artifact is deleted only when all of these hold:
#   - it is older than KEEP_DAYS;
#   - it is not among the newest KEEP_NEWEST artifacts with its name;
#   - its workflow run is known and completed; and
#   - its run is not the latest successful run on the default branch.
# Anything unknown is kept: a run that cannot be read, or an artifact with no
# run. A run that is still going may download its artifact in a later job, and
# the newest artifacts are the ones somebody is likely still looking at.
set -euo pipefail

DRY_RUN=${DRY_RUN:-true}
KEEP_DAYS=${KEEP_DAYS:-14}
KEEP_NEWEST=${KEEP_NEWEST:-5}
DEFAULT_BRANCH=${DEFAULT_BRANCH:-main}

list_artifacts() { # prints a JSON array of {id,name,created_at,workflow_run}
  gh api --paginate "/repos/$GITHUB_REPOSITORY/actions/artifacts?per_page=100" \
    --jq '.artifacts[]' | jq -s .
}
fetch_run() { # $1 = run id; prints the run JSON
  gh api "/repos/$GITHUB_REPOSITORY/actions/runs/$1"
}
latest_success_run() { # prints the newest successful default-branch run id
  gh api "/repos/$GITHUB_REPOSITORY/actions/runs?branch=$DEFAULT_BRANCH&status=success&per_page=1" \
    --jq '.workflow_runs[0].id // empty'
}
delete_artifact() { gh api -X DELETE "/repos/$GITHUB_REPOSITORY/actions/artifacts/$1" >/dev/null; }

prune() {
  local artifacts cutoff keep latest count=0 id name created run_id run status
  artifacts=$(list_artifacts)
  cutoff=$(date -u -d "-${KEEP_DAYS} days" +%Y-%m-%dT%H:%M:%SZ)
  latest=$(latest_success_run)

  # Newest of each name stay, so the artifact a recent failure left behind is
  # still there to download.
  keep=$(jq -r --argjson n "$KEEP_NEWEST" '
    group_by(.name)[]
    | sort_by(.created_at) | reverse | .[:$n] | .[].id' <<<"$artifacts")

  while IFS=$'\t' read -r id name created run_id; do
    if grep -qxF "$id" <<<"$keep"; then
      echo "kept $id $name: among the newest $KEEP_NEWEST of its name"
      continue
    fi
    if [ -z "$run_id" ]; then
      echo "kept $id $name: no workflow run to read"
      continue
    fi
    if ! run=$(fetch_run "$run_id"); then
      echo "kept $id $name: run $run_id could not be read"
      continue
    fi
    status=$(jq -r '.status // empty' <<<"$run")
    if [ "$status" != completed ]; then
      echo "kept $id $name: run $run_id is ${status:-unknown}"
      continue
    fi
    if [ "$run_id" = "$latest" ]; then
      echo "kept $id $name: run $run_id is the latest successful $DEFAULT_BRANCH run"
      continue
    fi
    if [ "$DRY_RUN" = true ]; then
      echo "would delete $id $name (created $created, run $run_id)"
    else
      delete_artifact "$id"
      echo "deleted $id $name (created $created, run $run_id)"
    fi
    count=$((count + 1))
  done < <(jq -r --arg c "$cutoff" '
    .[] | select(.expired != true and .created_at < $c)
    | [.id, .name, .created_at, (.workflow_run.id // "")] | @tsv' <<<"$artifacts")

  echo "$count artifact(s) $([ "$DRY_RUN" = true ] && echo 'would be ')deleted"
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  GITHUB_REPOSITORY=${GITHUB_REPOSITORY:?}
  prune
fi
