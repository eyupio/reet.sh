# reet.sh

A self-hosted shelf for the skills on [skills.sh](https://www.skills.sh). skills.sh has no accounts, likes or bookmarks, so reet.sh keeps your own list: like skills, save them for later, and track which ones you actually use.

It is a single Go binary with an embedded web UI and a SQLite file. Nothing is sent to skills.sh except read-only page fetches for names and descriptions.

## Quick start

```sh
curl -O https://raw.githubusercontent.com/eyupio/reet.sh/main/docker-compose.yml
docker compose up -d
```

Open http://localhost:8080. The compose file uses the prebuilt image `ghcr.io/eyupio/reet.sh` (no local build) and stores data in the `reet-data` volume.

### Settings

| Variable | Default | Purpose |
| --- | --- | --- |
| `REET_TOKEN` | empty | If set, the API and bookmarklets need this token. The UI asks for it once and remembers it. Set it if the app is reachable by anyone else. |
| `SKILLS_SH_TOKEN` | empty | Optional token for the official skills.sh API (it needs a Vercel OIDC token). Without it, reet reads the public skill pages instead. |
| `REET_ADDR` | `:8080` | Listen address. |
| `REET_DATA_DIR` | `./data` (`/data` in the image) | Where the database lives. |

```sh
REET_TOKEN=choose-something docker compose up -d
```

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

Use **Download JSON** and **Restore from JSON** in the UI, or call `GET /api/export` and `POST /api/restore`. The database is also a single file in the data volume.

## Development

```sh
cd web && npm ci && npm run build   # builds the UI into web/dist, which the Go binary embeds
cd .. && go test ./... && go run ./cmd/reet serve
```

For UI work, run `npm run dev` in `web/` (it proxies the API to `localhost:8080`).

The `Dockerfile` builds the UI and the binary and cross-compiles for `linux/amd64` and `linux/arm64`. The GitHub Actions workflow tests every push and publishes `ghcr.io/eyupio/reet.sh` (`latest`, `vX.Y.Z` and `sha-<short>` tags) from `main` and version tags.
