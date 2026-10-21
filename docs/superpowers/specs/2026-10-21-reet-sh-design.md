# reet.sh design

Date: 2026-10-21 (revised the same day: stack aligned with zoomies.sh)

## Purpose

A self-hosted app that lets one person like, bookmark and mark as "used" the skills listed on https://www.skills.sh. skills.sh has no accounts, likes or bookmarks, so this is a separate personal layer. Success: the user can see their liked, bookmarked and in-use skills in one place and add to them with minimal effort.

Stated by the user: self-hosted web app; skills added by URL plus a bookmarklet; import of installed skills; name reet.sh, repo eyupio/reet.sh; tech choices similar to zoomies.sh.
Assumptions: single user, own homelab; "like" is a private favourite; "used" means installed or in regular use.

## Non-goals

Multi-user accounts, public sharing, cross-instance sync, any write-back to skills.sh, an interactive installer (v1).

## Stack (mirrors zoomies.sh)

- Go backend on the stdlib `net/http` mux (Go 1.22+ pattern routing), no web framework.
- Svelte 5 SPA (Vite, TypeScript) embedded in the binary with `go:embed`.
- SQLite via `modernc.org/sqlite` (pure Go, no CGO) so the result is one static binary.
- Single binary `reet` with subcommands `reet serve` and `reet scan`.
- Docker image (distroless or scratch, non-root) with a `/data` volume, published to GHCR as `ghcr.io/eyupio/reet.sh` (tags `latest`, `vX.Y.Z`, `sha-<short>`; `linux/amd64` and `linux/arm64`) by a GitHub Actions workflow on pushes to `main` and version tags.
- `docker-compose.yml` references that image only: an `image:` line, no `build:` section. It sets the `/data` volume, port, `REET_TOKEN` and `SKILLS_SH_TOKEN` as environment variables, and a healthcheck against `/healthz`.

## Architecture

### Data model

`skills` table, keyed by skills.sh id:

- `id` (`owner/repo/skill`, `domain/skill` for well-known sources, `local/name` for unresolved installs)
- `source`, `slug`, `source_type` (`github`, `well-known`, `local`), `url`
- `name`, `description`, `installs` (cached metadata, best effort)
- `liked`, `bookmarked`, `used` (0/1), `notes`
- `created_at`, `updated_at`

### Go packages

1. `internal/ref`: parses a skills.sh URL or slug into an id; rejects other hosts.
2. `internal/meta`: optional official API (`SKILLS_SH_TOKEN`, which needs a Vercel OIDC token); otherwise reads the public page title and description; on any failure keeps the slug only. Never blocks saving.
3. `internal/store`: SQLite access (migrate, list with filter/search/sort, upsert, patch flags and notes, delete, export, restore).
4. `internal/api`: handlers under `/api/*`, the `/quick` endpoint for bookmarklets, optional `REET_TOKEN` auth middleware, `/healthz`, and serving the embedded SPA.
5. `internal/scan`: reads the skills CLI lock file if present and skill folders (`~/.claude/skills`, `~/.agents/skills`, and others) into `(name, source?)` pairs; `reet scan` posts them to `/api/import` as used, with `--dry-run`.
6. `web/`: Svelte 5 SPA with add box, tabs (All, Liked, Bookmarked, Used), search, sort, per-card toggles and notes, import panel, bookmarklets, backup/restore.

### API surface

`GET /api/skills`, `POST /api/skills`, `PATCH /api/skill`, `DELETE /api/skill`, `POST /api/skill/refresh`, `POST /api/skill/resolve`, `POST /api/import`, `GET /api/export`, `POST /api/restore`, `GET /quick?action=like|save|used&url=`.

### Safety

Optional `REET_TOKEN` required for `/api/*` and `/quick`. The SPA renders scraped text as text, never HTML. Outbound fetches have an 8 second timeout.

### Error handling

Unparseable input returns 400 with a message. Metadata failure is silent. Unknown lock file shapes fall back to folder names.

### Testing

`go test` for `ref`, `meta`, `store`, `api` (using `httptest`) and `scan` (using a temp fake home directory). Live skills.sh fetching can't be verified from the build sandbox, so the fallback path is what gets tested there. The SPA is type-checked with `svelte-check` and built with Vite.

## Known risks

- The CLI lock file format is unverified; folder names are the fallback.
- Folder-name-only skills have no known source until linked by hand.
- Page scraping may break if skills.sh changes its markup; the app still works with slugs.
