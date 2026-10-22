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
- **List or board.** Choose Board for On the shelf → To test → In use columns on desktop, stacked on smaller screens. Move skills with the same keyboard- and touch-accessible buttons; the layout choice is saved to your account (or this browser without accounts). Moving a card changes shelf status, not installed files.
- **Compact skill cards.** Cards start collapsed by default, with a link to their source, stage, available install/trend data, machine/tag/list counts and a personal-note indicator. Move stages, like, copy an install command or copy a ready-to-run prompt without opening the card. Expand an individual card for descriptions, tags, machine details, notes and history, or use Expand all / Collapse all. Expansion choices survive filtering, stage moves and layout changes during your visit.
- **Focused navigation.** Account and Admin dropdowns lead to individual pages with a section menu. Mobile/tablet navigation folds into Menu; team usage becomes compact cards with expandable report details. Server-only heartbeats stay in diagnostics, outside shelf filters and status badges.
- **Your defaults.** In Account → Profile, choose List/Board and collapsed/expanded cards across browsers. Individual expansion choices remain temporary for the current visit.
- **Environment badges.** Each card shows Installed (green), Missing (amber) or Not confirmed (neutral) for each known machine. Missing requires complete agent scans from the last 14 days; stale, incomplete and heartbeat-only reports cannot confirm absence. These are reported inventories, not live filesystem checks.
- **Capture anywhere.** Paste an `npx skills add` command, skills.sh URL or `owner/repo/skill`, or use the bookmarklets (Like, Save, In use) on any skill page.
- **Know what is installed.** `reet scan` reads the skills folders for your tools, per machine and per tool. New discoveries start In use; later scans preserve your existing shelf choices.
- **Copy feedback.** Copy buttons confirm success with a brief checkmark reveal; reduced-motion preferences use a gentle fade.
- **Machine maintenance.** Merge hostname/`--env` duplicates, rename or forget machines in Account → Machines. Old names remain aliases, preventing duplicate reports on later scans. `reet scan --prune --reconcile --env NAME` repairs the local hostname and refreshes inventories; fleet commands never install/remove skills.
- **Controller sync.** Compare selected reet.sh shelf/list/published skills with machine scopes. Enroll a scanner-only agent for scan, preview, approval, pause/resume and recovery controls. Additive mode preserves extras; exact and bidirectional modes require reviewed changes. [Setup and controls](docs/controller-sync.md).
- **Environment sync.** Account includes site-aware scanner setup, machine/server status and saved reference skill sets. Preview differences and run `reet sync --apply` locally to confirm installs/removals. [Setup and limits](docs/environment-sync.md).
- **Copy missing installs.** Pick an environment and copy the install commands for everything on your shelf that is not there yet.
- **Metadata that stays fresh.** Description, install count and a weekly sparkline, refreshed every six hours, politely.
- **Tags and notes.** Skills.sh topics are seeded as tags; add your own and filter by several at once.
- **Accounts and profiles.** Owner setup, argon2id passwords, HttpOnly SameSite sessions, rate-limited sign-in. Each person's Account has their profile, password, sessions, personal API tokens, activity and backups. Or token mode, or no auth on a trusted LAN.
- **Yours to take away.** Export JSON, Markdown or CSV, and restore from JSON. The database is one file.
- **Sealed backups, here and off-site.** The owner backs up the whole instance; every person can back up their own shelf. Copies are encrypted before they are written or shipped, verified after every take, pruned by a count you choose, and sent to any S3-compatible destination.
- **Yorkshire at heart.** A reet good toolkit, with a white-rose bookmark and a practical, quietly quirky voice. See [voice and copy](docs/voice-and-copy.md) when changing user-facing text.
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

### Shared lists

A **list** keeps a set of skills and shared list notes together. New lists default to **Personal**. Choose **Selected people**, a **Team or organisation** (with extra named people and individual exclusions), **Public community**, or an enabled **Unlisted link**. Each choice previews its audience before saving. Editors change skills and list notes; list owners manage audiences. Instance administrators retain administrative access, and read-only accounts stay read-only.

Use **Lists** to browse My lists, Shared with me and Teams & organisations. **Community** is a separate, anonymous directory of deliberately published lists and their skills. Personal shelf notes, people, memberships, exclusions and machine reports are never published with a list. Existing signed-in-only lists keep their previous access under **Legacy instance audience**; they do not become public.

**Teams** manages organisations, teams and their members. Sharing with a group follows its membership automatically; exclusions override group and individual access for ordinary accounts. Existing unlisted links remain controlled by the owner's link-sharing switch. See [sharing and audience rules](docs/sharing.md) for permissions, publication, migration and limitations.

### Accounts

The default `REET_AUTH=accounts` creates one owner through the setup page on first visit, then signs people in with email and password. Each account has its own shelf. Roles are owner, admin, member and viewer; viewers are read-only. Shared lists are where people work together without sharing a shelf. Sessions are HttpOnly, SameSite=Lax cookies; passwords are stored with argon2id; sign-in is rate-limited.

Use the navbar’s **Account** menu (inside **Menu** on smaller screens). Its sections are **Profile** (name, theme, time zone and date order), **Password** (which ends every other session), **Sessions**, **Machines** (connect and inspect agent inventories), **Compare & sync** (comparison and reference snapshots), **API tokens** (shown once), **Backups** for your own shelf, and **Activity**. Admins get **Admin → Users** to invite people by link, reset passwords, disable accounts, change roles, revoke tokens and transfer ownership, and **Admin → Backups** for the instance. If `REET_TOKEN` is set it is required once on the setup page and works for `reet scan` only until a personal API token exists.

- **Solo or trusted LAN:** `REET_AUTH=none` removes sign-in and maps everything to an implicit owner. Use it only on a network you trust.
- **Legacy single token:** `REET_AUTH=token` keeps the single `REET_TOKEN` login card and requires it for the UI, API and bookmarklets.
- **Upgrading:** an existing shelf is kept and becomes the owner's shelf on first setup; flags, notes, tags and environments are carried over. Nothing is deleted.

| Variable | Default | Purpose |
| --- | --- | --- |
| `REET_DATA_PATH` | `./data` | Host folder for the database. Must exist and be writable by `REET_UID:REET_GID`. |
| `REET_UID`, `REET_GID` | `1000` | User the container runs as. `init-env.sh` sets these to `id -u` / `id -g`. |
| `REET_PORT` | `8080` | Host port for the UI. |
| `REET_VERSION` | `latest` | Image tag (`latest`, `dev`, `v1.0.0`, `sha-<short>`). |
| `REET_AUTH` | `accounts` | `accounts` (owner setup and sessions), `token` (the single `REET_TOKEN`) or `none` (trusted LAN, no sign-in). |
| `REET_TOKEN` | empty | Accounts mode: setup bootstrap and fallback scan token until a personal token exists. Token mode: the only credential. Empty means no token. |
| `SKILLS_SH_TOKEN` | empty | Optional skills.sh API token (needs a Vercel OIDC token, so it can't be generated). Without it, reet reads the public skills.sh pages. |
| `REET_LOG` | empty | Set to `debug` for one line per metadata fetch; writes are always logged without tokens. |
| `REET_TOPICS` | empty | Set to `off` to stop the daily skills.sh topic seed (nine read-only page fetches a day). |
| `REET_ENV` | hostname | Stable name for this machine in scan/sync/server reports. |
| `REET_CONTROLLER_URL` | empty | Optional remote controller to receive a server process's scans every minute. |
| `REET_CONTROLLER_TOKEN` | empty | Personal token from the remote controller account; separate from this instance's bootstrap token. |
| `REET_REFRESH` | empty | Set to `off` to stop the six-hourly metadata refresh (the 50 oldest skills, two seconds apart). |
| `REET_BACKUP_KEY` | generated | Passphrase that seals every backup and every stored destination secret. Use `reet backup keygen`, and keep a copy away from this machine. Empty generates `data/backup.key` on first start. |
| `REET_BACKUP_DIR` | `<data>/backups` | Where this machine keeps copies. |
| `REET_BACKUP_EVERY`, `REET_BACKUP_KEEP` | `24h`, `48` | Instance backup schedule and the count kept here. `0` turns the schedule off; `0` kept means every copy. |
| `REET_BACKUP_USER_EVERY`, `REET_BACKUP_USER_KEEP` | `0`, `14` | Defaults for a person's own backups; they can change them under Account → Backups. |
| `REET_BACKUP_DESTINATION` | `both` | Where copies go by default: `local` (this machine only), `remote` (an enabled destination only, kept here when none took it) or `both`. Each owner can change it on the Backups page. |
| `REET_BACKUP_COMPRESSION` | `gzip` | Compress copies before sealing by default: `gzip` or `none`. Each owner can change it, and give copies a password, on the Backups page. |
| `REET_BACKUP_S3_*` | empty | One destination from the environment (`ENDPOINT`, `REGION`, `BUCKET`, `PREFIX`, `ACCESS_KEY_ID`, `SECRET_ACCESS_KEY`, `PATH_STYLE`, `NAME`), readable by a machine rebuilt from nothing. More destinations are added in the UI. |
| `REET_BACKUP_ALLOW_LOCAL` | empty | Set `1` to allow an `http://` destination on another machine. Backups are sealed, but the access key id would travel in the clear. |

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

## Install the binary

Install `reet` on a Linux or macOS machine (amd64 or arm64) without Go or Node:

```sh
curl -fsSL https://raw.githubusercontent.com/eyupio/reet.sh/main/scripts/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"
reet --version
```

The installer downloads a GitHub release, verifies its SHA-256 checksum, and installs to `~/.local/bin`. The binary includes the Library web UI (`reet serve`) and the scanner (`reet scan`). See [binary installation](docs/binary-installation.md) for pinned versions, custom destinations, manual installation, and publishing releases. Binaries become available after the first version tag runs the release workflow.

## Hosting your own skills

Open **Host skills** in the navigation and claim a permanent publishing username. Create a skill using the editor or upload an existing `SKILL.md`. Its YAML frontmatter must contain a `name` matching the skill's URL name and a `description` (up to 1024 bytes). Add instructions below the frontmatter. Supporting files are optional; paths can be edited to preserve folders such as `scripts/` and `references/`. Each bundle can contain up to 100 supporting files and 2 MiB total.

Skills are private drafts until you select **Publish publicly** and save. A published skill has a public page at `https://reet.sh/username/skill-name`, with this install command:

```sh
npx skills add https://reet.sh/username/skill-name
```

Use `npx skills add` before the URL; `npx https://…` alone is not a skills CLI command. Self-hosted instances use their own hostname. Scoped Agent Skills discovery endpoints below the public URL let the CLI download `SKILL.md` and all supporting files. You can also paste the URL or its install command into the shelf.

Uncheck **Publish publicly** and save to unpublish, or delete the skill to remove its files. Both stop future downloads immediately; existing installations remain on users' machines. Disabled author accounts stop serving public skills. Publishing usernames cannot change or be reused after account deletion, so old links cannot be claimed by another account. Skill hosting follows the instance's normal write permissions; viewers cannot publish. Bundles live in `reet.db` and are covered by whole-instance database backups. Shelf JSON exports and personal shelf backups contain skill references, not authored bundles.

## Adding skills

### MCP integration

Open **Account → MCP access** and copy the hosted MCP URL into Claude or another
compatible client. OAuth sign-in and consent are the default; manual bearer
tokens and opt-in browser WebMCP are also available. Each person manages their
own connection permissions, expiry, pause, rotation and revocation. The existing
Go server hosts `/mcp`; no sidecar is needed. See [MCP setup and access](docs/mcp.md).

### Capture and scan

- **Paste a command or link.** Paste an install command such as `npx skills add https://github.com/anthropics/skills --skill mcp-builder`, a skills.sh URL or `owner/repo/skill` into the box at the top. reet extracts the repository and skill name and fetches the details from skills.sh without running the command.
- **Bookmarklets.** Open "Show bookmarklets and tools" in the UI and drag the Like, Save and In use links to your bookmarks bar. On any skills.sh skill page, click one to send that skill to reet.sh. In accounts mode the links use your session cookie and carry no secret; in token mode they contain `REET_TOKEN`, so keep them private.
- **Tags.** The daily skills.sh topic seed adds tags like Design & UI or Testing to matching skills. Use **Edit tags** on a skill to add tags or remove them with ×; changes save immediately. Removed tags stay removed through topic refreshes until you explicitly add them again. Outside edit mode, click a tag to filter the shelf, or combine several tag filters. Rename or merge tags under Show bookmarklets and tools → Manage tags.
- **Import what you have installed.** Run the scanner on the machine that has your skills:

  ```sh
  reet scan --server http://your-host:8080 --token "$REET_TOKEN" --env sam-mbp --prune
  ```

  It reads the skills folders for your tools (`~/.claude/skills`, `~/.agents/skills`, `~/.cursor/skills`, `~/.codex/skills`, `~/.config/opencode/skills`, plus `.claude/skills` and `.agents/skills` in the current directory) and the skills CLI lock file at `~/.agents/.skill-lock.json` if present, then marks each skill as "in use". Existing likes, saves and notes are kept.

  In accounts mode, `--token` is a personal API token from Account; the bootstrap `REET_TOKEN` works only until the first personal token is created. `--env NAME` names this machine (the hostname is the default) and turns each folder into an environment on the card, such as `sam-mbp · Claude Code`. `--prune` removes the environment link for skills that are no longer in a scanned folder; the skill and its notes stay. Add `--dir PATH` for other folders and `--dry-run` to preview. The scanner needs to see your skills folders, so run it on the machine that has them, not inside the container. Build it with `go install github.com/eyupio/reet.sh/cmd/reet@latest` (the scanner does not need the web UI).

  Skills found only by folder name show as "unlinked". Use **Link** on the card to attach the skills.sh URL.

  Once environments exist, the shelf gets an environment filter and a **Copy missing installs** action that copies the install commands for everything on the shelf that is not on that machine yet.

Manage the binary or a server with `reet install --mode binary|native|compose`, inspect updates with `reet upgrade --check`, apply a verified update with `reet upgrade` (or `--version dev`), and diagnose problems with `reet doctor [--json]`. Managed server upgrades preserve configuration, back up SQLite and restore the previous deployment on a failed restart. Commands preview their intent and require confirmation before applying; see [managed installation and upgrades](docs/binary-installation.md#managed-install-upgrade-and-doctor).

**Account → Machines** groups agents under machines; **Account → Compare & sync** offers a two-machine comparison with shared/missing skills and explicit stale/incomplete scan warnings. Library filters follow the same machine → agent hierarchy.

See [environment sync](docs/environment-sync.md) for account setup, confirmed installs/removals, server reporting and freshness advice. The rolling `dev` binary release and Docker `:dev` tag update from main after every merge; stable binary releases remain immutable.

## Backup

### Local AI agent setup discovery

`reet setup scan` provides a standalone, read-only inventory of known configuration
locations and executables for Claude Code, Codex, OpenCode, Pi, Cursor and Gemini
CLI. It does not require a reet account or server:

```sh
reet setup scan
reet setup scan --json
reet setup scan --project /path/to/project
```

This first step towards complete setup backup/restore checks path metadata and
executable availability only. It does not read configuration contents, run agents,
collect credentials, upload reports or change files. It reports setup backup and
restore as **not implemented**. Existing instance and shelf backups below remain
separate. See [setup discovery and limits](docs/setup-discovery.md), the
[implementation plan](docs/agent-setup-plan.md) and [handoff](handoff.md).

### Instance and personal shelf backups

There are two kinds: a one-off export, and sealed copies taken on a schedule.

**Personal shelf export and restore.** Use **JSON (personal shelf backup) → Download** and **Restore from JSON** in Shelf tools, or call `GET /api/export` and `POST /api/restore`. The export includes personal skills, flags, notes, tags, attached environment reports and the ignored list; tick **Include local history** to add the event log. It does not include accounts, shared lists, teams, sync references or standalone machine heartbeats; use an instance backup for those. **Markdown** and **CSV** downloads follow the current search, stage, tag and environment filters.

**Sealed, scheduled backups.** The instance owner configures the schedule, retention and off-site destinations under **Admin → Backups**; each person can back up their own shelf under **Account → Backups**, to their own destination. Every copy is verified after it is taken, then kept on this machine, copied to each enabled S3-compatible destination, or both, as the owner chooses. Copies are gzip-compressed by default; an owner can protect them with a password instead of the instance key and open a downloaded copy with `reet backup open`. **Download** hands over the sealed `.reetbackup` file; **Export JSON** is the unsealed snapshot. The key is separate from the database: keep a copy of `data/backup.key` (or `REET_BACKUP_KEY`) somewhere else, or an off-site copy cannot be opened after the machine is lost.

```sh
reet backup keygen                     # print a key for REET_BACKUP_KEY
reet backup list                      # instance copies on this machine
reet backup run                       # take one now, and ship it
reet backup open --password-file p f  # open a downloaded copy to stdout
reet backup escrow --out f --passphrase-file p   # key and destinations, sealed
reet backup restore --yes <id>        # with the server stopped
```

An instance restore replaces the live database, so the admin page tells you the command rather than doing it behind a running server; the database in place is moved aside, not deleted. A person's restore runs from their own page and only ever touches their own shelf. See [backups](docs/backups.md) for the full runbook, including a machine rebuilt from nothing.

## Shelf tools and admin

- **Select** in the toolbar turns cards into checkboxes. Shift-click selects a range; the floating bar can tag, move, like or remove the selection, and a bulk remove gets the same Undo toast as a single remove.
- Metadata for the 50 oldest skills is refreshed every six hours (two seconds apart) and the card shows the weekly install sparkline and change. **Refresh** on a card checks immediately and says when nothing changed.
- **Admin** in the header opens `/admin` for the owner and admins: version, database size, skills.sh reachability, topic seed status, counts, environments, users and invites, the team rollup, the refresh/topic/link-sharing switches, instance name and recent activity. Only the owner can change instance settings or transfer ownership.
- **Team rollup** on `/admin` shows adoption by skill across active accounts, the people behind each count, environment chips, and drift: skills in use outside any list and list skills nobody uses. Filter by list or tag, sort by adoption, and download exactly the visible rows as CSV. The owner can turn off per-person usage and leave counts only.

## Development

```sh
cd web && npm ci && npm run build   # builds the UI into web/dist, which the Go binary embeds
cd .. && go test ./... && go run ./cmd/reet serve
```

For UI work, run `npm run dev` in `web/` (it proxies the API to `localhost:8080`).

The [whole UI design studies](docs/ui-redesign.md) compare Library, Workbench and Focus across the signed-out pages, shelf, lists, account and admin. Open `web/concepts/controller.html` directly, or visit `/concepts/controller.html` on the Vite dev server. The app implements Library throughout: shared account forms, page headings, bordered shelf cards, list and account panels, grouped tools, and spacious admin tables. The other two directions remain concepts for reference. See [Library implementation](docs/library-implementation.md) for screenshots and validation.

Version tags also publish checksummed Linux and macOS binaries through the [binary release workflow](.github/workflows/release.yml).

The `Dockerfile` builds the UI and the binary and cross-compiles for `linux/amd64` and `linux/arm64`. The GitHub Actions workflow tests every push and publishes `ghcr.io/eyupio/reet.sh` (`latest`, `vX.Y.Z` and `sha-<short>` tags) from `main` and version tags.


## Project layout

| Path | What is there |
| --- | --- |
| `cmd/reet` | The one binary: `serve` runs the server, `scan` imports installed skills, `backup` takes and restores copies. |
| `internal/api` | HTTP API and the embedded UI handler. |
| `internal/auth` | Accounts, sessions, passwords and tokens. |
| `internal/backup` | Sealed copies, S3 destinations, schedules, retention and restores. |
| `internal/store` | SQLite storage and migrations. |
| `internal/meta` | Fetches skills.sh metadata. |
| `internal/ref` | Parses skill install commands, skills.sh URLs and slugs. |
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
