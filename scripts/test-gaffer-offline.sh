#!/usr/bin/env bash
# Isolated, fake-backed native/container acceptance test. Never runs a real model.
set -euo pipefail
gm6_mode="${1:-all}"
case "$gm6_mode" in native|docker|all);; *) echo 'Usage: scripts/test-gaffer-offline.sh [native|docker|all]' >&2; exit 2;; esac
gm6_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$gm6_root"
test -f web/dist/index.html || { echo 'Run npm --prefix web ci and npm --prefix web run build first.' >&2; exit 1; }
gm6_work="$(mktemp -d "${TMPDIR:-/tmp}/reet-gaffer-flow.XXXXXXXX")"
gm6_pid=''
gm6_container=''
gm6_image="reet-gaffer-acceptance:gm6-$$"
# Buildx writes client state even though the daemon's workspace is separate.
# Preserve configured registry credentials/proxies in an isolated writable copy.
mkdir -p "$gm6_work/docker-config"
if test -f "${DOCKER_CONFIG:-$HOME/.docker}/config.json"; then
  cp "${DOCKER_CONFIG:-$HOME/.docker}/config.json" "$gm6_work/docker-config/config.json"
fi
gm6_docker=(env -u DOCKER_HOST -u DOCKER_CONTEXT -u DOCKER_TLS -u DOCKER_TLS_VERIFY -u DOCKER_CERT_PATH docker --config "$gm6_work/docker-config" --host=unix:///var/run/docker.sock)
cleanup() {
  if test -n "$gm6_pid"; then kill "$gm6_pid" 2>/dev/null || true; wait "$gm6_pid" 2>/dev/null || true; cat "$gm6_work/native.log"; fi
  if test -n "$gm6_container"; then "${gm6_docker[@]}" logs "$gm6_container" || true; "${gm6_docker[@]}" rm -f "$gm6_container" >/dev/null || true; fi
  if test "$gm6_mode" != native; then "${gm6_docker[@]}" image rm "$gm6_image" >/dev/null 2>&1 || true; fi
  rm -rf "$gm6_work"
}
trap cleanup EXIT
run_flow() {
  local gm6_target="$1" gm6_port gm6_url gm6_ready=false
  gm6_port="$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')"
  gm6_url="http://127.0.0.1:$gm6_port"
  if test "$gm6_target" = native; then
    REET_GAFFER_BROWSER=1 REET_GAFFER_TEST_ADDR="127.0.0.1:$gm6_port" REET_GAFFER_TEST_PUBLIC_URL="$gm6_url" REET_GAFFER_TEST_WEB="$gm6_root/web/dist" \
      "$gm6_work/gaffer-native.test" -test.run '^TestGafferOfflineBrowserHarness$' -test.timeout=10m -test.v > "$gm6_work/native.log" 2>&1 &
    gm6_pid=$!
  else
    gm6_container="$("${gm6_docker[@]}" run -d --read-only --tmpfs /data:rw,uid=65532,gid=65532,mode=0700 --cap-drop ALL --security-opt no-new-privileges \
      -p "127.0.0.1:$gm6_port:8080" -e REET_GAFFER_BROWSER=1 -e REET_GAFFER_TEST_ADDR=0.0.0.0:8080 -e "REET_GAFFER_TEST_PUBLIC_URL=$gm6_url" "$gm6_image")"
  fi
  for _ in {1..60}; do
    if curl -fsS "$gm6_url/_gaffer_test/ready" >/dev/null 2>&1; then gm6_ready=true; break; fi
    if test "$gm6_target" = native && ! kill -0 "$gm6_pid" 2>/dev/null; then break; fi
    sleep 1
  done
  if test "$gm6_ready" != true; then
    if test "$gm6_target" = native; then cat "$gm6_work/native.log"; fi
    echo "$gm6_target fixture did not become ready" >&2; exit 1
  fi
  REET_UI_URL="$gm6_url" REET_GAFFER_TEST_MODE="$gm6_target" node web/tests/gaffer-offline-live.e2e.mjs
  if test "$gm6_target" = native; then
    wait "$gm6_pid" || { cat "$gm6_work/native.log"; exit 1; }; gm6_pid=''
    cat "$gm6_work/native.log"
  else
    local gm6_exit
    gm6_exit="$("${gm6_docker[@]}" wait "$gm6_container")"
    "${gm6_docker[@]}" logs "$gm6_container"
    test "$gm6_exit" = 0 || { echo "Container acceptance failed: $gm6_exit" >&2; exit 1; }
    "${gm6_docker[@]}" rm "$gm6_container" >/dev/null; gm6_container=''
  fi
}
if test "$gm6_mode" != docker; then
  go test -race -c -o "$gm6_work/gaffer-native.test" ./internal/api
  run_flow native
fi
if test "$gm6_mode" != native; then
  "${gm6_docker[@]}" info >/dev/null
  CGO_ENABLED=0 go test -c -o "$gm6_work/gaffer.test" ./internal/api
  cp -R web/dist "$gm6_work/web"
  cp scripts/gaffer-test.Dockerfile "$gm6_work/Dockerfile"
  # Use the production Dockerfile's actual runtime base and user, so this gate
  # follows changes to its runtime rather than silently testing a different OS.
  gm6_base="$(awk '/^FROM / { base=$2 } END { print base }' Dockerfile)"
  gm6_user="$(awk '/^USER / { user=$2 } END { print user }' Dockerfile)"
  test "$gm6_user" = 65532:65532 || { echo 'Review the fixture user after the production runtime user changes.' >&2; exit 1; }
  "${gm6_docker[@]}" build --build-arg "REET_RUNTIME_BASE=$gm6_base" -t "$gm6_image" "$gm6_work"
  run_flow docker
fi
