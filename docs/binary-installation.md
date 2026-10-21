# Install the reet binary

A release binary contains both the web UI and the local scanner. It runs on Linux and macOS, on amd64 (Intel/AMD) or arm64 (including Apple silicon), without a Go or Node installation.

## Install or update

```sh
curl -fsSL https://raw.githubusercontent.com/eyupio/reet.sh/main/scripts/install.sh | sh
```

The installer resolves the latest stable GitHub release, detects the operating system and CPU, downloads the matching archive and its SHA-256 manifest, and checks the archive before replacing `reet`. The default destination is `~/.local/bin`, with no sudo. Add that directory to your shell's PATH if needed:

```sh
export PATH="$HOME/.local/bin:$PATH"
reet --version
```

Running the installer again updates the binary. Existing shelves and settings are stored separately and are not touched by installation.

To review the script, choose a version, or change the destination, download it first:

```sh
curl -fsSL https://raw.githubusercontent.com/eyupio/reet.sh/main/scripts/install.sh -o install-reet.sh
sh install-reet.sh --version v1.2.3 --dir "$HOME/bin"
```

Replace `v1.2.3` with an existing release tag. The `v` prefix is optional. `REET_INSTALL_DIR` also sets the destination. Linux and macOS require `curl`, `tar`, `awk`, `install`, `mktemp`, and either `sha256sum` or `shasum`.

## Scan your machine

Create a personal API token under **Account** on your instance, then run the scanner on the machine containing your installed skills:

```sh
reet scan --server https://your-reet-instance.example --token "$REET_TOKEN" --env my-laptop
```

`REET_TOKEN` in this command is your personal scanner token. Use `reet scan --help` for the supported folders, environment and pruning options. Add `--dry-run` to inspect the scan before sending it.

## Run the web UI

The same binary includes the built Library UI:

```sh
REET_DATA_DIR="$HOME/.local/share/reet" reet serve
```

Open `http://localhost:8080` and create the owner account. The data directory holds the SQLite database; keep it when updating the binary. Existing `REET_AUTH`, `REET_ADDR`, and other server settings work as documented in the main README.

## Download and install manually

Each GitHub release contains:

- `reet_VERSION_linux_amd64.tar.gz`
- `reet_VERSION_linux_arm64.tar.gz`
- `reet_VERSION_darwin_amd64.tar.gz`
- `reet_VERSION_darwin_arm64.tar.gz`
- `checksums.txt`

Here `VERSION` includes the release tag's `v` prefix. Each archive contains `reet`, `LICENSE`, and `README.md`. Compare the archive's SHA-256 digest with `checksums.txt`, extract it, then put `reet` on your PATH.

## Rolling development builds

Each merge/push to main publishes a prerelease named `dev`, with four checksummed binaries, and a Docker `:dev` tag. To install it:

```sh
sh scripts/install.sh --version dev
```

The `dev` tag and assets move to the tested main commit on each successful build. It is explicitly excluded from GitHub's latest stable release. Only this rolling channel may replace assets; stable `v*` releases retain their existing verification and no-overwrite rules. During a dev update, a checksum mismatch means retry after publication finishes; the installer keeps the old binary on any verification failure. Every build job uses one resolved source commit, including manual `dev` runs.

## Publishing a binary release

The `binary release` workflow tests and cross-compiles all four targets on pull requests. On a version tag such as `v1.2.3`, it also publishes the archives and checksums to GitHub Releases. Tags containing a prerelease suffix are marked as prereleases; the installer's default `latest` follows GitHub's latest stable release.

The workflow builds the Svelte app once and embeds that output into every binary. It includes the version tag and source commit in `reet --version`. If a release already exists, it uploads missing binary assets and checksums after verifying that any matching published files are identical.

Stable publishing never overwrites published assets; use a new version tag when files differ. To recover an existing empty release, open **Actions → binary release → Run workflow**, select the default branch, and enter the existing tag (for example `v1.0.0`). This builds and tests that tag, then fills its release assets without changing its notes. Re-running an older failed job still uses its original workflow; use the manual run for this recovery.

Archive metadata uses the source commit timestamp so repeated builds do not differ only because of packaging time. Docker images continue through the existing image workflow. Newer runs cancel older runs of the same workflow for the same branch, pull request, or release tag; package cleanup also cancels its older active run.

The installer is ready when this change is merged, and downloads become available after the first version tag runs the new release workflow. No release is published by opening or merging the pull request alone.
