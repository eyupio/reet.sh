<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="brand/logo/horizontal-color-dark.svg">
  <img src="brand/logo/horizontal-color-light.svg" alt="reet.sh" width="340">
</picture>

### Your own shelf for the AI skills on [skills.sh](https://www.skills.sh)

Like them, save them for later, and keep track of the ones you actually use.<br>
One Go binary. One SQLite file. No telemetry.

[![CI](https://github.com/eyupio/reet.sh/actions/workflows/image.yml/badge.svg)](https://github.com/eyupio/reet.sh/actions/workflows/image.yml)
[![License: AGPL v3](https://img.shields.io/badge/license-AGPL--3.0-c2410c.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26-1d1c1a.svg)](go.mod)
[![Container](https://img.shields.io/badge/ghcr.io-eyupio%2Freet.sh-1d1c1a.svg)](https://github.com/eyupio/reet.sh/pkgs/container/reet.sh)
[![PRs welcome](https://img.shields.io/badge/PRs-welcome-c2410c.svg)](CONTRIBUTING.md)

[Quick start](#quick-start) · [Features](#features) · [Adding skills](#adding-skills) · [Configuration](#settings-env) · [Contributing](CONTRIBUTING.md)

<br>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/shelf-dark.png">
  <img src="docs/images/shelf-light.png" alt="The reet.sh shelf: skills grouped by In use, To test and On the shelf, with install counts and weekly trend" width="860">
</picture>

</div>

## Why

[skills.sh](https://www.skills.sh) is a great directory of agent skills, but it has no accounts, likes or bookmarks. You find a skill, close the tab, and a month later cannot remember which one it was or whether you ever installed it.

reet.sh is the missing personal layer. Capture a skill in two seconds, move it through **On the shelf → To test → In use**, and run a scanner on any machine to see what is really installed in Claude Code, Cursor, Codex, OpenCode and the `skills` CLI. Setting up a new laptop becomes one click: **Copy missing installs**.

Nothing is sent to skills.sh except read-only page fetches for names and descriptions.

## Features

- **Three-stage shelf.** On the shelf, To test, In use. Stage is derived from your flags, never stored out of step.
- **Capture anywhere.** Paste a skills.sh URL or `owner/repo/skill`, or use the bookmarklets (Like, Save, In use) on any skill page.
- **Know what is installed.** `reet scan` reads the skills folders for your tools and marks each skill "in use", per machine and per tool.
- **Copy missing installs.** Pick an environment and copy the install commands for everything on your shelf that is not there yet.
- **Metadata that stays fresh.** Description, install count and a weekly sparkline, refreshed every six hours, politely.
- **Tags and notes.** Skills.sh topics are seeded as tags; add your own and filter by several at once.
- **Accounts.** Owner setup, argon2id passwords, HttpOnly SameSite sessions, rate-limited sign-in. Or token mode, or no auth on a trusted LAN.
- **Yours to take away.** Export JSON, Markdown or CSV, and restore from JSON. The database is one file.
- **Pleasant to use.** Light and dark themes, works on a phone, keyboard friendly, respects reduced motion.

<details>
<summary>Dark mode and phone</summary>

<p align="center">
  <img src="docs/images/shelf-dark.png" alt="reet.sh in dark mode" width="560">
  <img src="docs/images/shelf-phone.png" alt="reet.sh on a phone" width="190">
</p>

</details>

## Quick start

```sh
git clone https://github.com/eyupio/reet.sh && cd reet.sh   # or just download docker-compose.yml, .env.example and scripts/init-env.sh
sh scripts/init-env.sh          # creates .env, generates REET_TOKEN, sets your UID/GID, creates ./data
docker compose up -d
```

Open http://localhost:8080. On a new instance, create the owner account; if `REET_TOKEN` is set, the setup page asks for it as the bootstrap token (`grep REET_TOKEN .env`). Later visits sign in with email and password.

The compose file uses the prebuilt image `ghcr.io/eyupio/reet.sh` (no local build). The database lives in a host folder, `./data` by default, bind-mounted at `/data`, so you can back it up or move it like any other folder. The container runs as `REET_UID:REET_GID` (your user, set by the script), so that folder stays owned by you.

### Settings (`.env`) <a id="settings-env"></a>

`docker compose` reads `.env` automatically. `.env.example` lists every setting with comments.

### Accounts

The default `REET_AUTH=accounts` creates one owner through the setup page on first visit, then signs people in with email and password. Sessions are HttpOnly, SameSite=Lax cookies; passwords are stored with argon2id; sign-in is rate-limited. If `REET_TOKEN` is set it is required once on the setup page and keeps working for `reet scan` until personal tokens arrive.

- **Solo or trusted LAN:** `REET_AUTH=none` removes sign-in and maps everything to an implicit owner. Use it only on a network you trust.
- **Legacy single token:** `REET_AUTH=token` keeps the single `REET_TOKEN` login card and requires it for the UI, API and bookmarklets.
- **Upgrading:** an existing shelf is kept; the first visit asks for the owner account. Nothing is moved or deleted.

| Variable | Default | Purpose |
| --- | --- | --- |
| `REET_DATA_PATH` | `./data` | Host folder for the database. Must exist and be writable by `REET_UID:REET_GID`. |
| `REET_UID`, `REET_GID` | `1000` | User the container runs as. `init-env.sh` sets these to `id -u` / `id -g`. |
| `REET_PORT` | `8080` | Host port for the UI. |
| `REET_VERSION` | `latest` | Image tag (`latest`, `v1.0.0`, `sha-<short>`). |
| `REET_AUTH` | `accounts` | `accounts` (owner setup and sessions), `token` (the single `REET_TOKEN`) or `none` (trusted LAN, no sign-in). |
| `REET_TOKEN` | empty | Accounts mode: setup bootstrap and scanner token. Token mode: the only credential. Empty means no token. |
| `SKILLS_SH_TOKEN` | empty | Optional skills.sh API token (needs a Vercel OIDC token, so it can't be generated). Without it, reet reads the public skills.sh pages. |
| `REET_LOG` | empty | Set to `debug` for one line per metadata fetch; writes are always logged without tokens. |
| `REET_TOPICS` | empty | Set to `off` to stop the daily skills.sh topic seed (nine read-only page fetches a day). |
| `REET_REFRESH` | empty | Set to `off` to stop the six-hourly metadata refresh (the 50 oldest skills, two seconds apart). |

### Commands for keys and settings

All of these edit the same `.env`, then `docker compose up -d` applies them:

```sh
sh scripts/init-env.sh                                  # first-time setup; keeps values already set
sh scripts/init-env.sh --rotate                         # new REET_TOKEN (old bookmarklets stop working)
sh scripts/init-env.sh --set SKILLS_SH_TOKEN=your-token # set or change any KEY=value
sh scripts/init-env.sh --set REET_PORT=9090
docker compose up -d                                    # apply changes
```

Without the script, the equivalent manual steps are:

```sh
cp .env.example .env
openssl rand -hex 32            # paste the output after REET_TOKEN= in .env
id -u; id -g                    # paste after REET_UID= and REET_GID=
mkdir -p data
```

If the container exits with "unable to open database file", the data folder isn't writable by `REET_UID:REET_GID`; fix it with `chown -R "$(id -u):$(id -g)" data` and restart.

(The `reet` server itself reads `REET_ADDR` (default `:8080`), `REET_DATA_DIR`, `REET_TOKEN` and `SKILLS_SH_TOKEN`; compose maps the settings above onto them.)

## Adding skills

- **Paste a link.** Paste a skills.sh URL or `owner/repo/skill` into the box at the top.
- **Bookmarklets.** Open "Show bookmarklets and tools" in the UI and drag the Like, Save and In use links to your bookmarks bar. On any skills.sh skill page, click one to send that skill to reet.sh. In accounts mode the links use your session cookie and carry no secret; in token mode they contain `REET_TOKEN`, so keep them private.
- **Tags.** The daily skills.sh topic seed adds tags like Design & UI or Testing to matching skills. Add your own from a card, then filter the shelf by one or more tags. Rename or merge tags under Show bookmarklets and tools → Manage tags.
- **Import what you have installed.** Run the scanner on the machine that has your skills:

  ```sh
  reet scan --server http://your-host:8080 --token "$REET_TOKEN" --env sam-mbp
  ```

  It reads the skills folders for your tools (`~/.claude/skills`, `~/.agents/skills`, `~/.cursor/skills`, `~/.codex/skills`, `~/.config/opencode/skills`, plus `.claude/skills` and `.agents/skills` in the current directory) and the skills CLI lock file at `~/.agents/.skill-lock.json` if present, then marks each skill as "in use". Existing likes, saves and notes are kept.

  In accounts mode, `--token` is the bootstrap `REET_TOKEN`; personal API tokens arrive with the account work. `--env NAME` names this machine (the hostname is the default) and turns each folder into an environment on the card, such as `sam-mbp · Claude Code`. `--prune` removes the environment link for skills that are no longer in a scanned folder; the skill and its notes stay. Add `--dir PATH` for other folders and `--dry-run` to preview. The scanner needs to see your skills folders, so run it on the machine that has them, not inside the container. Build it with `go install github.com/eyupio/reet.sh/cmd/reet@latest` (the scanner does not need the web UI).

  Skills found only by folder name show as "unlinked". Use **Link** on the card to attach the skills.sh URL.

  Once environments exist, the shelf gets an environment filter and a **Copy missing installs** action that copies the install commands for everything on the shelf that is not on that machine yet.

## Backup

Use **Download JSON** and **Restore from JSON** in the UI, or call `GET /api/export` and `POST /api/restore`. The backup includes skills, notes, tags, environments and the ignored list; tick **Include local history** to add the event log. **Markdown** and **CSV** downloads follow the current search, stage, tag and environment filters, so you can hand someone exactly what is on screen. The database is a single file, `reet.db`, in your data folder.

## Shelf tools and admin

- **Select** in the toolbar turns cards into checkboxes. Shift-click selects a range; the floating bar can tag, move, like or remove the selection, and a bulk remove gets the same Undo toast as a single remove.
- Metadata for the 50 oldest skills is refreshed every six hours (two seconds apart) and the card shows the weekly install sparkline and change. **Refresh** on a card checks immediately and says when nothing changed.
- **Admin** in the footer opens `/admin`: version, database size, skills.sh reachability, topic seed status, counts, environments, the refresh/topic switches, instance name and recent activity. The admin surface is available to any signed-in account until per-role access arrives.

## Development

```sh
cd web && npm ci && npm run build   # builds the UI into web/dist, which the Go binary embeds
cd .. && go test ./... && go run ./cmd/reet serve
```

For UI work, run `npm run dev` in `web/` (it proxies the API to `localhost:8080`).

The `Dockerfile` builds the UI and the binary and cross-compiles for `linux/amd64` and `linux/arm64`. The GitHub Actions workflow tests every push and publishes `ghcr.io/eyupio/reet.sh` (`latest`, `vX.Y.Z` and `sha-<short>` tags) from `main` and version tags.


## Project layout

| Path | What is there |
| --- | --- |
| `cmd/reet` | The one binary: `serve` runs the server, `scan` imports installed skills. |
| `internal/api` | HTTP API and the embedded UI handler. |
| `internal/auth` | Accounts, sessions, passwords and tokens. |
| `internal/store` | SQLite storage and migrations. |
| `internal/meta` | Fetches skills.sh metadata. |
| `internal/ref` | Parses skills.sh URLs and slugs. |
| `internal/scan` | Finds skills installed on a machine. |
| `web/` | The Svelte UI, built into `web/dist` and embedded in the binary. |
| `brand/` | Logo set, colours and usage rules. |

## Contributing

Bug reports, ideas and pull requests are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) first; it covers the dev loop, tests and what makes a good change. Please follow the [Code of Conduct](CODE_OF_CONDUCT.md). Found a vulnerability? See [SECURITY.md](SECURITY.md) and do not open a public issue.

## Licence

reet.sh is free software under the [GNU Affero General Public License v3.0](LICENSE). In short: use it, change it, run it for yourself or your team; if you offer a modified version to others over a network, share your changes under the same licence.

reet.sh is an independent project and is not affiliated with or endorsed by skills.sh or Vercel.

---

<div align="center">

<img src="brand/logo/tile-ink.svg" alt="" width="28" height="28">

**reet.sh** · Your shelf for AI skills<br>
Developed by [EyUp.io](https://eyup.io)

</div>
