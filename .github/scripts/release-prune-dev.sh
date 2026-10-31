#!/bin/sh
# Delete old per-commit assets from the rolling `dev` GitHub release.
#
# usage: release-prune-dev.sh
# env:   GH_TOKEN (contents:write, actions:read), GITHUB_REPOSITORY,
#        KEEP_DAYS (default 14), KEEP_COMMITS (default 20), DRY_RUN (default
#        true), PUBLISHERS, PUBLISH_WAIT_TRIES, PUBLISH_WAIT_SECONDS
#
# scripts/publish-release.sh adds reet_dev_<sha>_<os>_<arch>.tar.gz and
# checksums-<sha>.txt for every merge and never removes any. The release's API
# listing grows with them, and an agent that has to read it stops working once
# it passes its size cap. A commit's assets are deleted here only when
#   - they are all per-commit assets (the legacy rolling names, checksums.txt
#     and dev-build.txt are never touched);
#   - the newest of them is older than KEEP_DAYS;
#   - the commit is not among the KEEP_COMMITS newest; and
#   - the commit is not the one the `dev` tag points at now.
# It fails closed: an unreadable release, tag or asset list deletes nothing,
# and so does a plan that would delete every per-commit asset.
set -eu

repo=${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}
keep_days=${KEEP_DAYS:-14}
keep_commits=${KEEP_COMMITS:-20}
dry_run=${DRY_RUN:-true}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# Do not delete while a dev build is being published.
if [ "$dry_run" != "true" ]; then
  tries=${PUBLISH_WAIT_TRIES:-10}
  while :; do
    busy=0
    for wf in ${PUBLISHERS:-release.yml}; do
      for status in queued in_progress; do
        n=$(gh api "/repos/$repo/actions/workflows/$wf/runs?status=$status&per_page=1" --jq '.total_count')
        busy=$((busy + n))
      done
    done
    [ "$busy" -eq 0 ] && break
    tries=$((tries - 1))
    if [ "$tries" -le 0 ]; then
      echo "::warning::release workflows are still running; deleting nothing this time"
      exit 0
    fi
    echo "$busy release run(s) in progress; waiting"
    sleep "${PUBLISH_WAIT_SECONDS:-60}"
  done
fi

release_id=$(gh api "/repos/$repo/releases/tags/dev" --jq .id)
current=$(gh api "/repos/$repo/git/ref/tags/dev" --jq .object.sha)
case $current in
  [0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]*) ;;
  *) echo "::error::cannot read the commit the dev tag points at; deleting nothing"; exit 1 ;;
esac

gh api --paginate "/repos/$repo/releases/$release_id/assets?per_page=100" \
  --jq '.[] | {id, name, created: .created_at}' | jq -s . > "$work/assets.json"

cutoff=$(date -u -d "-$keep_days days" +%Y-%m-%dT%H:%M:%SZ)

# Per-commit assets only, grouped by commit, newest commit first.
jq --arg current "$current" --arg cutoff "$cutoff" --argjson keep "$keep_commits" '
  [ .[] | . as $a
    | ($a.name | capture("^(reet_dev_(?<sha>[0-9a-f]{40})_.+\\.tar\\.gz|checksums-(?<sha2>[0-9a-f]{40})\\.txt)$")? ) as $m
    | select($m != null)
    | $a + {sha: ($m.sha // $m.sha2)} ]
  | group_by(.sha)
  | map({sha: .[0].sha, newest: (map(.created) | max), ids: map(.id), names: map(.name)})
  | sort_by(.newest) | reverse
  | to_entries
  | map(.value + {keep: (.key < $keep or .value.newest >= $cutoff or .value.sha == $current)})
' "$work/assets.json" > "$work/plan.json"

total=$(jq length "$work/plan.json")
doomed=$(jq '[.[] | select(.keep | not)] | length' "$work/plan.json")
echo "dev release: $(jq length "$work/assets.json") assets, $total per-commit builds, $doomed to delete"

if [ "$doomed" -eq 0 ]; then
  exit 0
fi
if [ "$doomed" -ge "$total" ]; then
  echo "::error::the plan would delete every per-commit build; deleting nothing"
  exit 1
fi

jq -r '.[] | select(.keep | not) | .ids[] | tostring' "$work/plan.json" > "$work/ids"
jq -r '.[] | select(.keep | not) | .names[]' "$work/plan.json" | while read -r name; do
  if [ "$dry_run" = "true" ]; then echo "would delete $name"; else echo "deleting $name"; fi
done
[ "$dry_run" = "true" ] && exit 0

while read -r id; do
  gh api --method DELETE "/repos/$repo/releases/assets/$id" >/dev/null
done < "$work/ids"
