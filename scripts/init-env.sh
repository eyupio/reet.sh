#!/bin/sh
# Creates or updates .env for docker compose: copies .env.example, generates REET_TOKEN,
# sets REET_UID/REET_GID to the current user and creates the data folder.
#
#   sh scripts/init-env.sh            keep any values already set
#   sh scripts/init-env.sh --rotate   generate a new REET_TOKEN (old bookmarklets stop working)
#   sh scripts/init-env.sh --set SKILLS_SH_TOKEN=xyz   set any KEY=value in .env
set -eu
cd "$(dirname "$0")/.."

rotate=0
sets=""
while [ $# -gt 0 ]; do
  case "$1" in
    --rotate) rotate=1 ;;
    --set) shift; [ $# -gt 0 ] || { echo "--set needs KEY=value" >&2; exit 2; }; sets="$sets
$1" ;;
    *) echo "usage: sh scripts/init-env.sh [--rotate] [--set KEY=value]..." >&2; exit 2 ;;
  esac
  shift
done

[ -f .env ] || { cp .env.example .env; echo "created .env from .env.example"; }

get() { sed -n "s/^$1=//p" .env | head -n 1; }

# Replace KEY=... in .env, or append it. Works with both GNU and BSD tools.
set_var() {
  tmp=$(mktemp)
  if grep -q "^$1=" .env; then
    awk -v k="$1" -v v="$2" 'BEGIN{FS=OFS="="} $1==k && !done {print k "=" v; done=1; next} {print}' .env > "$tmp"
  else
    cat .env > "$tmp"; printf '%s=%s\n' "$1" "$2" >> "$tmp"
  fi
  cat "$tmp" > .env; rm -f "$tmp"
}

new_token() {
  if command -v openssl >/dev/null 2>&1; then openssl rand -hex 32
  else head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'; fi
}

if [ "$rotate" = 1 ] || [ -z "$(get REET_TOKEN)" ]; then
  set_var REET_TOKEN "$(new_token)"
  echo "REET_TOKEN generated in .env"
else
  echo "REET_TOKEN already set (use --rotate to replace it)"
fi

if [ "$(id -u)" = 0 ]; then
  echo "warning: running as root, so the container will run as root too. Run this as your normal user (without sudo) instead." >&2
fi
set_var REET_UID "$(id -u)"
set_var REET_GID "$(id -g)"

IFS='
'
for kv in $sets; do
  [ -n "$kv" ] || continue
  set_var "${kv%%=*}" "${kv#*=}"
  echo "${kv%%=*} set in .env"
done
unset IFS

data=$(get REET_DATA_PATH); data=${data:-./data}
mkdir -p "$data"
echo "data folder: $data (owned by $(id -un))"
echo "done. Start it with: docker compose up -d"
