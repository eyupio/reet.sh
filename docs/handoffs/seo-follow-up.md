# SEO continuation handoff

This handoff records the public-search work landed on 4 November 2026 and the remaining tasks. It is written to be
read from the GitHub repository alone; do not depend on a local worktree, an untracked file or a machine-specific path.

The full audit and its status update are in [SEO_AUDIT.md](../../SEO_AUDIT.md). The top-level current state is in
[handoff.md](../../handoff.md), section `SEO-1`.

## What is already on `main`

| PR | Scope | Merge |
| --- | --- | --- |
| #479 | Real 404s, noindex on app/private routes, `llms.txt`/`ai.txt`, server-side legacy redirects, `docs` namespace reserved | `e54cd9c` |
| #481 | Anonymous landing page leads with install and community | `aed7d36` |
| #483 | Server-rendered `/community` and `/l/<slug>` with self-canonicals, OG and sitemap entries | `187a5c7` |
| #487 | Hosted skill pages: canonical, OG, `SoftwareSourceCode`/`BreadcrumbList`, sitemap entries | `7f713c2` |
| #489 | Public documentation at `/docs/` rendered from `docs/*.md` with Goldmark | `3d8d1bb` |
| #494 | `llms.txt`, `llms-full.txt`, `ai.txt` and public nav link to `/docs/` | `5b4050a` |
| #496 | Guides: Claude Code on a VPS, skill sync, self-hosted MCP, skills.sh comparison | `f507b6f` |
| #497 | Crawl contract test: every sitemap URL is 200, indexable and self-canonical | `b6b6ea4` |
| #502 | New lists get readable public addresses (`name-<8 hex>`) | `35c2182` |
| #505 | Open WebUI and LibreChat comparisons, "remote agents without a VPN" guide | `e95cb81` |
| #507 | Landing page links to the new guides; crawl fallback updated | `701e1cd` |

Relevant code and content:

- `web/embed.go` — SPA routing, 404s, noindex, robots, sitemap, `llms.txt`/`ai.txt`.
- `web/docs.go` — `/docs/` pages, metadata, previous/next, docs images and 404.
- `docs/docs.go` — curated docs manifest, Goldmark rendering, link rewriting, sections.
- `cmd/reet/publicpages.go` — server-rendered community and public lists; sitemap URLs from the store.
- `internal/api/hosted.go` — hosted skill page metadata and JSON-LD.
- `internal/store/store_hosted.go` — `PublishedHostedSkills` for the sitemap.
- `internal/store/store_lists.go` — readable list slug generation.
- `web/seo_test.go` — crawl contract test.

## What remains

### 1. Existing public list links (H6, second part)

New lists get readable slugs, but existing lists keep their opaque 16-hex slugs. The next step is an explicit
"use a readable link" action that:

- generates a readable slug from the list name with a unique suffix;
- stores the old slug as an alias;
- returns a permanent redirect from the old address to the new one;
- updates the canonical and sitemap entry.

Read `internal/store/store_lists.go`, `internal/api/lists.go`, `internal/api/groups.go`,
`cmd/reet/publicpages.go` and `web/src/lib/ListView.svelte`/`NewList.svelte` before changing anything. Add a store
migration for the alias table and tests for aliases, redirects and collision handling.

### 2. Measurement (H7)

There is no Search Console or Bing verification, no analytics and no submitted sitemap. These need an owner or admin
and are not code-only changes:

- verify `reet.sh` in Google Search Console and Bing Webmaster Tools (DNS verification, no secret in the repo);
- submit `/sitemap.xml`;
- decide whether to add privacy-friendly analytics. Keep "no telemetry" as the default and make any analytics opt-in
  through configuration;
- review index coverage and queries after a few weeks.

The crawl contract test (#497) is the code-side guard. Do not add a third-party tracker without an explicit decision.

### 3. Marketing/app bundle split (N1)

The landing page is rendered by the SPA after the full application bundle loads. Split the marketing page from the
application so `/` and `/welcome` can be static-first, or code-split the app routes so the landing bundle is small.
Measure before and after; do not claim a field-data improvement without CrUX or Search Console data.

### 4. Remaining app metadata (H3)

Public pages now have per-page metadata. App and account screens are intentionally `noindex`; only add richer metadata
there if the product decides to expose more public help content. The private routes must stay out of the sitemap.

### 5. More content (H4)

The first content batch is live. Possible next pages, each tied to a real capability:

- run Codex on a remote machine;
- run OpenCode in the browser;
- reet.sh with an existing Tailscale network;
- a "day one" setup guide for a small team;
- a backup and restore walkthrough.

Keep comparisons fair and do not invent traffic, rankings or competitor weaknesses.

## Checks for a new session

From the repository root:

```sh
cd web && npm ci && npm run check && npm run build && cd ..
go vet ./...
go test -race ./...
```

For the narrow SEO surface:

```sh
go test ./docs ./web ./internal/store ./internal/api
```

To inspect the live behaviour:

```sh
curl -sI https://reet.sh/robots.txt
curl -s https://reet.sh/sitemap.xml
curl -s https://reet.sh/docs/ | head
curl -s https://reet.sh/llms.txt
```

## Continuation prompt

Copy this into a new session. It uses only the GitHub repository and public URLs.

```text
Continue the SEO work on the public GitHub repository eyupio/reet.sh.

Start by reading these files from the repository (use the GitHub repository as the source of truth, not a local
worktree or untracked file):
- handoff.md, section "SEO-1"
- docs/handoffs/seo-follow-up.md
- SEO_AUDIT.md, including the "Status update — 4 November 2026" section

The first implementation batch is merged. Confirm the current state with:
  gh pr list --repo eyupio/reet.sh --state merged --search "SEO OR documentation OR guides OR sitemap"
  git log --oneline origin/main -20

Then take the next unchecked item from docs/handoffs/seo-follow-up.md. Prefer this order unless the repository shows a
more urgent need:
1. Add a "use a readable link" action for existing public lists, with a permanent redirect from the old opaque slug.
2. Add a crawl/SEO regression check for any new public route, and keep web/seo_test.go passing.
3. Improve the landing-page/app bundle split only after measuring the current lab and, if available, field data.

Working rules:
- Branch from origin/main. One focused change per pull request. Do not include unrelated local changes.
- Read CONTRIBUTING.md and follow the repository's style, tests and PR template.
- For Go changes run go vet ./... and go test -race ./...; for web changes run npm run check and the relevant
  browser tests. For docs-only changes run go test ./docs ./web.
- Every public page must have a unique title and description, a self-referential canonical, and sensible
  indexability. Private, account and auth routes must stay noindex and out of sitemap.xml.
- Keep "no telemetry" as the default. Do not add analytics or a third-party service without an explicit product
  decision.
- Do not invent search volumes, rankings, traffic or competitor weaknesses. Verify claims against the repository or
  the public site.
- Update handoff.md and docs/handoffs/seo-follow-up.md in the same pull request when a task is completed.
- Open PRs against main, wait for green checks, and merge only when the PR is mergeable and green.

Deliver the smallest useful next PR, report the PR number and checks, and leave the handoff describing the next
unchecked task.
```

## Known limits

- The mobile lab run in the audit was a single fast-environment measurement, not field Core Web Vitals.
- No Search Console, Bing or analytics data exists in the repository; do not infer it.
- Existing lists keep their old links until the alias/redirect work is done.
- The docs set is curated; design notes, research, handoffs and verification logs remain internal.
