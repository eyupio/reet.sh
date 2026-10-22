# Install the reet binary

A release binary contains both the web UI and the local scanner. It runs on Linux and macOS, on amd64 (Intel/AMD) or arm64 (including Apple silicon), without a Go or Node installation.

## Install or update

```sh
curl -fsSL https://raw.githubusercontent.com/eyupio/reet.sh/main/scripts/install.sh | sh
```

The installer uses `--version`, then exported `REET_VERSION`, and otherwise resolves the latest stable GitHub release. It detects the operating system and CPU, downloads the matching archive and its SHA-256 manifest, and checks the archive before replacing `reet`. The default destination is `~/.local/bin`, with no sudo. Add that directory to your shell's PATH if needed:

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

## Connect an enrolled scanner

Use `reet agent enroll --server https://reet.sh --env NAME` with your personal `REET_TOKEN`, then `reet agent run`. Add `--allow-apply` during enrollment to enable plans approved in the web UI, and `--project /absolute/path` for tracked project scopes. See [controller sync](controller-sync.md) for private configuration, controls and recovery.

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

## Managed install, upgrade and doctor

Once the release binary is on PATH, it can install itself and manage a server:

```sh
reet install --mode binary                   # copy this binary to ~/.local/bin
reet install --mode native                   # Linux user systemd / macOS LaunchAgent
reet install --mode compose --version dev    # Docker + Compose plugin required
reet install --mode native --dry-run         # inspect without applying
```

Choose one mode per deployment. Installation shows the paths and asks you to type `yes`; `--yes` supports automation. Native mode installs the binary you are running. Compose resolves a published release and pins its image digest, checking that the image and checksummed binary are from the same build. The main image reports `dev`, matching the rolling binary channel.

Managed installs default to `127.0.0.1:8080`, `~/.local/share/reet` for server data and the platform user configuration directory (`~/.config/reet` on Linux) for `deployment.json`. Use `--dir`, `--data-dir`, `--addr` and `--state-dir` to choose other paths. Existing database directories are retained. Server settings such as `REET_AUTH`, `REET_TOKEN`, `REET_CONTROLLER_URL`, `REET_CONTROLLER_TOKEN` and `REET_ENV` are captured from your environment into the private deployment file, without printing their values. Set them before installing. Keep configuration files and backups private.

Linux uses `systemctl --user` and needs a working user service manager; use `loginctl enable-linger USER` if your administrator wants the service to survive logout. macOS uses a per-user LaunchAgent in the logged-in GUI session. `--no-start` writes the deployment without enabling or starting its server, for setup before the user service manager is available. Native service logs are available with `journalctl --user -u reet` on Linux and the configuration directory's `server.log` on macOS. Compose files are generated in the state directory with a unique project name and the current user's UID/GID. Run the CLI on the host, outside the container.

```sh
reet upgrade --check                 # look up the current published tag only
reet upgrade --dry-run               # download, checksum and inspect the candidate
reet upgrade                         # preserve dev; otherwise latest stable; confirmation required
reet upgrade --version dev           # explicitly select the rolling main build
reet upgrade --version latest        # explicitly switch to latest stable
reet upgrade --version v1.2.3         # explicitly select a version, including a downgrade
reet doctor                          # readable checks and advice
reet doctor --json                   # structured results, exit 1 if any check fails
reet healthcheck                     # saved target / REET_URL; prompt when missing
reet healthcheck --local             # local REET_ADDR (default port 8080)
reet healthcheck --url https://reet.sh # explicitly check this server at /healthz
```

`init` aliases `install`; `update` aliases `upgrade`. Each command accepts `--help`. Warnings alone leave doctor's exit status at zero. It checks the executable/PATH, deployment permissions, SQLite integrity, managed service status, local health, Node/npm availability for skill sync, and configured controller credentials. It never applies fixes or changes server configuration. A server heartbeat does not replace a full agent scan.

Upgrade preserves the previous executable as `reet.previous`. For a managed server it records whether the server is running, pulls/verifies releases before downtime, stops the service, makes a consistent SQLite backup under `STATE_DIR/backups`, and restarts only if it was running. A failed start/health check restores the previous executable, pinned Compose configuration and pre-upgrade database before restarting the old service. If stopping the failed service or restoring its files fails, the CLI stops recovery and prints an error rather than writing into a running database. Keep the backup for manual recovery. A previously stopped server remains stopped; its first subsequent start is not health-verified by the upgrade. Standalone binaries cannot restart a manually launched server: restart that process yourself.

Managed files belong to this CLI. Existing system-level services, custom Compose projects and manually launched servers are not automatically adopted. Stop them first, back up their database and explicitly point a new deployment at its absolute data path; do not run two servers against one database. Run lifecycle operations for a deployment with the same `--state-dir`; concurrent operations there are rejected. After an interrupted operation, check its process has exited before removing `lifecycle.lock`. No uninstall, OS tuning, Docker installation or privileged system service changes run automatically.

## Upgrade channel selection and recovery

An explicit `--version` takes precedence over `REET_VERSION`, which takes
precedence over saved defaults. Without any of those, plain `reet upgrade` keeps
a dev target on `dev`; other versions follow latest stable. For a binary/native deployment, when the running
CLI is the target executable, its actual build version takes precedence over a
stale deployment record left by a manual reinstall. A different target uses its
recorded version. Compose follows its recorded image version, independently of
the CLI build. The preview labels the running CLI build and recorded deployment
version separately; they may legitimately differ.

Older upgrade implementations always defaulted to latest stable. Running one from
a dev binary could therefore replace it with an older stable release lacking
`upgrade`, `doctor` and `setup`. If those commands have disappeared, reinstall the
dev binary once and verify the actual executable:

```sh
curl -fsSL https://raw.githubusercontent.com/eyupio/reet.sh/main/scripts/install.sh | sh -s -- --version dev
hash -r
~/.local/bin/reet --version
~/.local/bin/reet upgrade --version dev
```

The latest fixed dev binary preserves its channel on subsequent plain upgrades.
The shell installer uses explicit `--version`, then exported `REET_VERSION`, and
otherwise latest stable. It does not read the CLI defaults file.
Reinstalling a CLI does not restart a manually launched server.

`healthcheck --url` accepts an HTTP(S) server base URL or its `/healthz` endpoint,
preserving a base path such as `/reet`. It rejects unknown flags and positional
arguments. Older binaries ignored those arguments and always contacted the local
port; connection reset/EOF from `127.0.0.1` does not establish remote-site health.
Use `curl -fsS https://your-site.example/healthz` until the CLI is updated.

## Saved CLI defaults

```sh
reet configure
# Or supply both without prompting:
reet configure --version dev --url https://reet.sh

# Exported values override saved defaults for this shell:
export REET_VERSION=dev
export REET_URL=https://reet.sh
reet upgrade --check
reet healthcheck
```

`configure` previews current values in its interactive prompts; Enter keeps a
suggested value. Valid answers are saved together after both values pass validation.
The file is `reet/client.env` under the platform user configuration directory:
`$XDG_CONFIG_HOME/reet/client.env` or `~/.config/reet/client.env` on Linux, and
`~/Library/Application Support/reet/client.env` on macOS. Files are written with
private permissions and atomically replaced. The file contains only `REET_VERSION`
and `REET_URL`, quoted as env assignments; the CLI parses it as data and never
executes it. Tokens and credentials must not be put in this file. Concurrent writes
are locked; if a process dies during a write, confirm it has stopped before removing
`client.env.lock` beside the defaults file.

Precedence is **explicit command flag → exported environment → saved default →
installed version where relevant**. Supported CLI versions are `latest`, `dev` or
release tags such as `v1.2.3`. The server is an HTTP(S) base URL without embedded
credentials, query or fragment. `--url` selects it for healthcheck, while scanner,
sync, enrollment and doctor use their existing `--server` flag. An enrolled sync
configuration remains a fallback when no server default was supplied.

Commands ask only for missing values they need. An interactive scan, sync with a
target, enrollment, doctor in text mode, or healthcheck can ask for an absent URL
and save it for next time. Upgrade infers its channel from the installed version;
only an unknown build without another channel needs a prompt. Prompts do not read
from redirected stdin. Noninteractive commands requiring a missing value fail
with setup instructions; doctor JSON retains its diagnostic report instead of
prompting. Local `setup scan` and `scan --dry-run` do not ask for a server.

`healthcheck` uses `REET_URL`/saved defaults first, and an explicit `REET_ADDR` as
a local fallback. `--local` ignores remote defaults and checks local `REET_ADDR`
(or port 8080). Container health checks use this explicit local mode. `serve`,
`version`, help and local setup discovery do not load client defaults.

These settings apply to future reet CLI processes; they cannot export variables
into the parent shell. A project `.env` is not loaded or changed automatically.
Docker Compose still reads its project `.env` using its own behaviour. For the
shell download installer, export `REET_VERSION` before the pipeline, or pass
`--version` to `sh`; assigning it only to `curl` does not configure the shell on
the other side of the pipe.

## Terminal presentation

Interactive help, configuration and lifecycle commands show a compact reet.sh
banner using the existing burnt-orange brand palette. Checking a release,
downloading/verifying a binary and remote health checks animate only while work
is running. Status output goes to the terminal on stderr and is cleared before
subsequent command output; no sleeps are added to make work appear longer.

`REET_NO_ANIMATION=1` disables motion. `NO_COLOR` (even empty) or `TERM=dumb`
disables colour and motion. Redirected output gets no escape sequences or banner,
and JSON output remains machine-readable. Version output retains the exact
`reet VERSION (COMMIT)` contract used to verify release binaries.

## Scanner authentication

`reet scan` uses `--token`, then exported `REET_TOKEN`, then the token in an
explicitly selected file:

```sh
reet scan --env-file .env
# Or, with no token configured, enter it at the hidden terminal prompt:
reet scan
```

Use a personal API token from **Account** on the target server. The file option
reads only `REET_TOKEN`; the target still comes from `--server`, `REET_URL`, or
saved CLI defaults. It accepts literal unquoted, single-quoted or double-quoted
values and optional `export`. It never executes the file or expands variables.
Duplicate token entries and malformed values are rejected. A project `.env` is
not loaded implicitly. Keep files containing tokens private (mode 0600).

The prompt does not echo or save the token. Enter skips authentication for servers
configured without it. Noninteractive commands never prompt and retain support
for unauthenticated servers; a 401 includes configuration guidance. `--dry-run`
neither reads the token file nor prompts. Tokens are not written to `client.env`.
