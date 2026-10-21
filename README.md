# reet.sh

A self-hosted shelf for the skills on [skills.sh](https://www.skills.sh). skills.sh has no accounts, likes or bookmarks, so reet.sh keeps your own list: like skills, save them for later, and track which ones you actually use.

It is a single Go binary with an embedded web UI and a SQLite file. Nothing is sent to skills.sh except read-only page fetches for names and descriptions.

## Quick start

```sh
git clone https://github.com/eyupio/reet.sh && cd reet.sh   # or just download docker-compose.yml, .env.example and scripts/init-env.sh
sh scripts/init-env.sh          # creates .env, generates REET_TOKEN, sets your UID/GID, creates ./data
docker compose up -d
```

Open http://localhost:8080 and sign in with the token from `.env` (`grep REET_TOKEN .env`).

The compose file uses the prebuilt image `ghcr.io/eyupio/reet.sh` (no local build). The database lives in a host folder, `./data` by default, bind-mounted at `/data`, so you can back it up or move it like any other folder. The container runs as `REET_UID:REET_GID` (your user, set by the script), so that folder stays owned by you.

### Settings (`.env`)

`docker compose` reads `.env` automatically. `.env.example` lists every setting with comments.

| Variable | Default | Purpose |
| --- | --- | --- |
| `REET_DATA_PATH` | `./data` | Host folder for the database. Must exist and be writable by `REET_UID:REET_GID`. |
| `REET_UID`, `REET_GID` | `1000` | User the container runs as. `init-env.sh` sets these to `id -u` / `id -g`. |
| `REET_PORT` | `8080` | Host port for the UI. |
| `REET_VERSION` | `latest` | Image tag (`latest`, `v1.0.0`, `sha-<short>`). |
| `REET_TOKEN` | empty | Access token for the UI, API and bookmarklets. Empty means no auth; only do that on a network you fully trust. |
| `SKILLS_SH_TOKEN` | empty | Optional skills.sh API token (needs a Vercel OIDC token, so it can't be generated). Without it, reet reads the public skills.sh pages. |
| `REET_LOG` | empty | Set to `debug` for one line per metadata fetch; writes are always logged without tokens. |

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
- **Bookmarklets.** Open "Show bookmarklets, import and backup" in the UI and drag the Like, Save and In use links to your bookmarks bar. On any skills.sh skill page, click one to send that skill to reet.sh. If you set `REET_TOKEN`, the links contain it, so keep them private.
- **Import what you have installed.** Run the scanner on the machine that has your skills:

  ```sh
  reet scan --server http://your-host:8080 --token "$REET_TOKEN"
  ```

  It reads the skills folders for your tools (`~/.claude/skills`, `~/.agents/skills`, `~/.cursor/skills`, `~/.codex/skills`, `~/.config/opencode/skills`, plus `.claude/skills` and `.agents/skills` in the current directory) and the skills CLI lock file at `~/.agents/.skill-lock.json` if present, then marks each skill as "in use". Existing likes, saves and notes are kept. Add `--dir PATH` for other folders and `--dry-run` to preview. The scanner needs to see your skills folders, so run it on the machine that has them, not inside the container. Build it with `go install github.com/eyupio/reet.sh/cmd/reet@latest` (the scanner does not need the web UI).

  Skills found only by folder name show as "unlinked". Use **Link** on the card to attach the skills.sh URL.

## Backup

Use **Download JSON** and **Restore from JSON** in the UI, or call `GET /api/export` and `POST /api/restore`. The database is a single file, `reet.db`, in your data folder.

## Development

```sh
cd web && npm ci && npm run build   # builds the UI into web/dist, which the Go binary embeds
cd .. && go test ./... && go run ./cmd/reet serve
```

For UI work, run `npm run dev` in `web/` (it proxies the API to `localhost:8080`).

The `Dockerfile` builds the UI and the binary and cross-compiles for `linux/amd64` and `linux/arm64`. The GitHub Actions workflow tests every push and publishes `ghcr.io/eyupio/reet.sh` (`latest`, `vX.Y.Z` and `sha-<short>` tags) from `main` and version tags.

---

<div align="center">

<img src="brand/logo/tile-ink.svg" alt="" width="28" height="28">

**reet.sh** · Your shelf for AI skills<br>
Developed by [EyUp.io](https://eyup.io)

</div>
