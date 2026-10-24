#!/bin/sh
# Download a checksummed reet release. No Go toolchain or elevated access needed.
set -eu

usage() {
  cat <<'EOF'
Usage: install.sh [--version VERSION] [--dir DIRECTORY]

Install reet for Linux or macOS (amd64 or arm64) from GitHub Releases.
  --version VERSION  Install a specific release, e.g. v1.2.3 or dev (default: REET_VERSION or latest)
  --dir DIRECTORY    Install directory (default: REET_INSTALL_DIR or ~/.local/bin)
  -h, --help         Show this help

Requires curl, tar, and sha256sum or shasum. Existing reet binaries are replaced
only after the downloaded archive's SHA-256 checksum has been verified.
EOF
}
fail() { printf 'reet: %s\n' "$*" >&2; exit 1; }
# Quote copyable shell commands without interpreting characters in a custom path.
quote() {
  remaining=$1
  printf "'"
  while :; do
    case "$remaining" in
      *"'"*) printf '%s' "${remaining%%\'*}"; printf '%s' "'\\''"; remaining=${remaining#*\'} ;;
      *) printf '%s' "$remaining"; break ;;
    esac
  done
  printf "'"
}

version=${REET_VERSION:-latest}
install_dir=${REET_INSTALL_DIR:-"${HOME:?HOME is not set}/.local/bin"}
while [ "$#" -gt 0 ]; do
  case "$1" in
    --version|--dir)
      [ "$#" -ge 2 ] && [ -n "$2" ] || fail "$1 requires a value"
      case "$1" in --version) version=$2 ;; --dir) install_dir=$2 ;; esac
      shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) fail "unknown option: $1 (use --help)" ;;
  esac
done

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) fail 'supported systems are Linux and macOS; see GitHub Releases for available binaries' ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) fail 'supported architectures are amd64 and arm64' ;;
esac
for dependency in curl tar awk install mktemp; do
  command -v "$dependency" >/dev/null 2>&1 || fail "$dependency is required"
done
if command -v sha256sum >/dev/null 2>&1; then
  checksum=sha256sum
elif command -v shasum >/dev/null 2>&1; then
  checksum=shasum
else
  fail 'sha256sum or shasum is required to verify the download'
fi

release_root=https://github.com/eyupio/reet.sh/releases
fetch() {
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
    --retry 3 --connect-timeout 10 --max-time 180 "$@"
}
if [ "$version" = latest ]; then
  latest_url=$(fetch --head --output /dev/null --write-out '%{url_effective}' "$release_root/latest") || fail 'could not find the latest release; check that a binary release has been published'
  version=${latest_url##*/}
fi
case "$version" in dev|v*) ;; *) version=v$version ;; esac
[ "$version" = dev ] || printf '%s\n' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$' || fail 'version must be a release tag such as v1.2.3'
asset=reet_${version}_${os}_${arch}.tar.gz
work_dir=$(mktemp -d "${TMPDIR:-/tmp}/reet-install.XXXXXX") || fail 'could not create a temporary directory'
staged_binary=
staged_previous=
cleanup() {
  rm -rf "$work_dir"
  if [ -n "$staged_binary" ]; then rm -f "$staged_binary"; fi
  if [ -n "$staged_previous" ]; then rm -f "$staged_previous"; fi
}
trap cleanup 0
trap 'exit 1' HUP INT TERM
manifest_asset=checksums.txt
build_sha=
if [ "$version" = dev ]; then
  if pointer_status=$(fetch --output "$work_dir/dev-build.txt" --write-out '%{http_code}' "$release_root/download/dev/dev-build.txt"); then
    build_sha=$(cat "$work_dir/dev-build.txt")
    printf '%s\n' "$build_sha" | grep -Eq '^[0-9a-f]{40}$' || fail 'invalid dev build pointer; existing installation was not changed'
    asset=reet_dev_${build_sha}_${os}_${arch}.tar.gz
    manifest_asset=checksums-${build_sha}.txt
  elif [ "$pointer_status" != 404 ]; then
    fail 'could not resolve the dev build; existing installation was not changed'
  fi
fi
printf 'Downloading reet %s for %s/%s…\n' "$version" "$os" "$arch"
fetch --output "$work_dir/$asset" "$release_root/download/$version/$asset" || fail "could not download $asset"
fetch --output "$work_dir/checksums.txt" "$release_root/download/$version/$manifest_asset" || fail 'could not download release checksums'
expected=$(awk -v name="$asset" '$2 == name { print $1; matches++ } END { if (matches != 1) exit 1 }' "$work_dir/checksums.txt") || fail 'release must contain exactly one checksum for this archive'
printf '%s\n' "$expected" | grep -Eq '^[0-9a-f]{64}$' || fail 'release checksum is not a SHA-256 digest'
if [ "$checksum" = sha256sum ]; then
  actual=$(sha256sum "$work_dir/$asset" | awk '{print $1}')
else
  actual=$(shasum -a 256 "$work_dir/$asset" | awk '{print $1}')
fi
[ "$actual" = "$expected" ] || fail 'checksum mismatch; the existing installation was not changed'
tar -xzf "$work_dir/$asset" -C "$work_dir" reet || fail 'could not extract the release binary'
[ -f "$work_dir/reet" ] && [ ! -L "$work_dir/reet" ] || fail 'archive does not contain a regular reet binary'
case "$install_dir" in /*) ;; *) install_dir=$PWD/$install_dir ;; esac
mkdir -p "$install_dir" || fail "cannot create $install_dir; choose a writable directory with --dir"
[ ! -d "$install_dir/reet" ] || fail "$install_dir/reet is a directory"
staged_binary=$(mktemp "$install_dir/.reet-install.XXXXXX") || fail "cannot write to $install_dir; choose a writable directory with --dir"
install -m 0755 "$work_dir/reet" "$staged_binary"
build=$("$staged_binary" --version) || fail 'verified release binary cannot run on this host; existing installation was not changed'
set -f
set -- $build
[ "$#" -eq 3 ] && [ "$1" = reet ] && [ "$2" = "$version" ] || fail 'release binary version does not match the requested tag; existing installation was not changed'
[ -z "$build_sha" ] || [ "$3" = "($build_sha)" ] || fail 'release binary commit does not match the dev build; existing installation was not changed'
if [ -e "$install_dir/reet" ] || [ -L "$install_dir/reet" ]; then
  [ -f "$install_dir/reet" ] && [ ! -L "$install_dir/reet" ] || fail 'existing binary must be a regular file; inspect its symlink before replacing it'
  [ ! -d "$install_dir/reet.previous" ] || fail 'reet.previous is a directory; existing installation was not changed'
  staged_previous=$(mktemp "$install_dir/.reet-previous.XXXXXX") || fail 'cannot stage previous binary'
  install -m 0755 "$install_dir/reet" "$staged_previous"
  mv -f "$staged_previous" "$install_dir/reet.previous"
  staged_previous=
fi
mv -f "$staged_binary" "$install_dir/reet"
staged_binary=
printf 'Reet, installed %s to %s/reet\n' "$version" "$install_dir"
case ":${PATH:-}:" in
  *":$install_dir:"*) ;;
  *) printf '\nRun this in your terminal, and add it to your shell profile for future sessions:\n'; printf 'export PATH='; quote "$install_dir"; printf ':"$PATH"\n' ;;
esac
resolved=$(command -v reet 2>/dev/null || true)
if [ -n "$resolved" ] && [ "$resolved" != "$install_dir/reet" ]; then
  printf '\nAnother reet is earlier on PATH: %s\nUse the absolute path below, or prepend the installation directory and run hash -r.\n' "$resolved"
fi
printf '\nVerified build: %s\nPrevious binary (when present): %s/reet.previous\n' "$build" "$install_dir"
printf '\nCheck your installation:\n'
quote "$install_dir/reet"
printf ' --version\n\nConnect to an existing site: open Machines on that site, then Connect a machine.\nHost your own site: run reet install --mode native (user service), or --mode compose (Docker).\nFor an existing managed server, use reet upgrade for future updates.\n'
printf '\nConnect a machine and set up its terminal with: reet agent terminal setup\nThe wizard checks tmux 3.2+, local shell consent and native services. Initial reet install and upgrade also offer connection setup in interactive sessions.\n'
