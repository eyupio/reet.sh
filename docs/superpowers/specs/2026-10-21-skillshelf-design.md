# Skillshelf design

Date: 2026-10-21

## Purpose

A self-hosted web app that lets one person like, bookmark and mark as "used" the skills listed on https://www.skills.sh. skills.sh has no accounts, likes or bookmarks, so this is a separate personal layer. Success: the user can see their liked, bookmarked and in-use skills in one place and add to them with minimal effort.

Stated by the user: self-hosted web app; skills added by URL plus a bookmarklet; import of installed skills.
Assumptions: single user, own homelab; "like" is a private favourite; "used" means installed or in regular use.

## Non-goals

Multi-user accounts, public sharing, cross-instance sync, any write-back to skills.sh.

## Architecture

One Node 22 service (Express) with a SQLite file (better-sqlite3), a static single-page UI, and a CLI scanner script. Ships as a Docker image with a `/data` volume.

### Data model

`skills` table, keyed by skills.sh id:

- `id` (`owner/repo/skill`, `domain/skill` for well-known sources, `local/name` for unresolved installs)
- `source`, `slug`, `source_type` (`github`, `well-known`, `local`), `url`
- `name`, `description`, `installs` (cached metadata, best effort)
- `liked`, `bookmarked`, `used` (0/1), `notes`
- `created_at`, `updated_at`

### Components

1. **Ref parser**: turns a skills.sh URL or slug into an id; rejects other hosts.
2. **Metadata fetcher**: optional official API (`SKILLS_SH_TOKEN`, which needs a Vercel OIDC token); otherwise scrapes the public page title and description; on any failure keeps the slug only. Never blocks saving.
3. **HTTP API** (`/api/*`): list with filter/search/sort, add, patch flags and notes, delete, refresh metadata, resolve a local entry to a real id, bulk import, JSON export and restore.
4. **Quick endpoint** (`/quick?action=like|save|used&url=`): used by the bookmarklets; returns a small self-closing confirmation page.
5. **UI**: add box, tabs (All, Liked, Bookmarked, Used), search, sort, per-card toggles and notes, import panel, bookmarklets, backup/restore.
6. **Scanner** (`scripts/scan-installed.mjs`): reads the skills CLI lock file if present and skill folders (`~/.claude/skills`, `~/.agents/skills`, and others), then posts them to `/api/import` as used. Supports `--dry-run`.

### Safety

Optional `APP_TOKEN` required for `/api/*` and `/quick`. UI renders scraped text with `textContent` only. Fetches have an 8 second timeout.

### Error handling

Unparseable input returns 400 with a message. Metadata failure is silent. Unknown lock file shapes fall back to folder names.

### Testing

Run the server locally and exercise add, toggle, filter, import, quick, export and restore with curl; run the scanner in dry-run mode against a fake home directory. Live skills.sh fetching can't be verified from the build sandbox, so the fallback path is what gets tested there.

## Known risks

- The CLI lock file format is unverified; folder names are the fallback.
- Folder-name-only skills have no known source until linked by hand.
- Page scraping may break if skills.sh changes its markup; the app still works with slugs.
