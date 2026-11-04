# SEO Audit — reet.sh

**Audit date:** 4 November 2026  
**Scope:** reet.sh source (`web/`, `internal/api/`, `docs/`, `README.md`, configuration) and the live site at `https://reet.sh`.  
**Method:** source inspection, live HTTP and header checks, no-JavaScript Playwright rendering, and a mobile-lab Playwright run. No application code was changed.

---

## Status update — 4 November 2026

The initial audit found the gaps below. The following work has since landed on `main`:

| PR | Change | Merge |
| --- | --- | --- |
| #479 | Real 404s, noindex for app/private routes, `llms.txt`/`ai.txt`, server-side legacy redirects | `e54cd9c` |
| #481 | Install-first landing CTAs and the real install command | `aed7d36` |
| #483 | Server-rendered `/community` and `/l/<slug>` with per-page metadata and sitemap entries | `187a5c7` |
| #487 | Hosted skill pages: canonical, OG, `TechArticle`/`BreadcrumbList`, sitemap entries | `7f713c2` |
| #489 | Documentation published at `/docs/` (Goldmark, metadata, images, sitemap) | `3d8d1bb` |
| #494 | `llms.txt`/`ai.txt` and public nav point at the docs | `5b4050a` |
| #496 | Guides: Claude Code on a VPS, skill sync, self-hosted MCP, skills.sh comparison | `f507b6f` |
| #497 | Crawl contract test for every sitemap URL | `b6b6ea4` |
| #502 | Readable public list addresses for new lists | `35c2182` |
| #505 | Open WebUI and LibreChat comparisons, no-VPN guide | `e95cb81` |
| #507 | Landing page links to the new guides | `701e1cd` |

The Go-served docs described in #489 were later replaced by a standalone MkDocs build in the same repository
(`mkdocs.yml`, `docs/requirements.txt`, `hooks/docs.py`, `overrides/main.html` and `.github/workflows/docs.yml`).
The application no longer serves `/docs`; the docs are deployed to GitHub Pages and `DOCS_URL` points at the public
docs origin. The remaining work is tracked in
[the SEO continuation handoff](docs/handoffs/seo-follow-up.md): existing opaque list links, external measurement
(Search Console, Bing, analytics), the marketing/app bundle split, remaining app metadata, and more content.

---


## Executive summary

reet.sh has a better technical foundation than most early-stage developer products: a real crawlable landing fallback, a valid `robots.txt`, a valid `sitemap.xml`, Open Graph tags, basic JSON-LD, self-hosted fonts, gzip, immutable asset caching, and an original, specific product voice.

The problem is not polish. It is **surface area**. Almost every valuable thing reet.sh already has is invisible to search:

- The 45 Markdown documents in `docs/` are not published at all. `/docs` serves the SPA with HTTP 200 and no documentation.
- The public community and public lists are client-rendered only. With JavaScript disabled, both return **zero words**. Their canonical URL is hard-coded to `https://reet.sh/`, so they cannot rank even when Google renders them.
- Every unknown URL returns **HTTP 200**. `/llms.txt`, `/ai.txt`, `/sitemap-index.xml` and `/feed.xml` all return `200 text/html` containing the app shell.
- `robots.txt` allows `/admin`, `/account`, `/lists`, `/sessions`, `/machines`, `/skills`, `/activity`, `/setup` and `/reset`; no `noindex` is emitted anywhere.
- Published skill pages are indexable but orphaned: no canonical, no Open Graph, no structured data, not in the sitemap.
- There are no docs pages, feature pages, integration guides, comparison pages, alternatives pages, blog, or changelog on the domain.
- There is no Search Console/Bing verification, no analytics, no sitemap submission, no IndexNow, and no SEO regression test beyond “the sitemap has exactly one URL”.

**Overall assessment:** technically tidy, commercially under-built. The highest-value work is not new copywriting; it is making existing assets (docs, public lists, hosted skills) crawlable, indexable and correctly canonicalised, then adding intent pages around the agents and workflows reet already supports.

### Top five highest-priority findings

1. **Publish the documentation as a real site at `/docs`.** It is the largest existing content asset and currently contributes nothing to organic search. `/docs` returns the app shell; `/docs/mcp` returns 404.
2. **Server-render (or ship crawlable fallbacks for) `/community`, `/l/<slug>` and hosted skill pages.** They currently return zero words without JavaScript and canonicalise to the homepage.
3. **Stop returning 200 for everything.** Unknown routes, missing text files and missing XML files must return 404 (or 410), with noindex where appropriate.
4. **Close the indexing leaks and open the intended doors.** Disallow/noindex private app routes; keep public lists and hosted skills indexable; generate a sitemap that lists them.
5. **Fix the anonymous conversion path.** On the live instance, signups are closed and the primary CTA is “Sign in”. A search visitor’s first meaningful action should be installing reet.sh or exploring public content, not signing in to an account they cannot create.

---

## Verified evidence

| Check | Observed result |
|---|---|
| `https://reet.sh/` with JavaScript disabled | 200, ~500 words of crawlable landing copy |
| `https://reet.sh/community` with JavaScript disabled | 200, **0 words** |
| `https://reet.sh/l/32d001e64e86227d` with JavaScript disabled | 200, **0 words** |
| `https://reet.sh/eyupio/ruthless-ux-ui-designer` with JavaScript disabled | 200, ~434 words, no canonical, no OG, no JSON-LD |
| `https://reet.sh/does-not-exist-xyz` | **200**, 0 words without JavaScript |
| `https://reet.sh/llms.txt` | **200 `text/html`**, app shell |
| `https://reet.sh/ai.txt` | **200 `text/html`**, app shell |
| `https://reet.sh/sitemap-index.xml` | **200 `text/html`**, app shell |
| `https://reet.sh/docs` | **200 `text/html`**, app shell |
| `https://reet.sh/docs/mcp` | **404** |
| Canonical on `/community` | `https://reet.sh/` |
| Canonical on `/l/32d001e64e86227d` | `https://reet.sh/` |
| Canonical on `/welcome` | `https://reet.sh/` |
| `robots.txt` disallows | only `/api/` and `/quick` |
| `noindex` / `X-Robots-Tag` in source | none |
| Sitemap URLs | exactly one: `https://reet.sh/` |
| Meta description length | 265 characters |
| Live auth status | `mode: accounts`, `signupEnabled: false`, `contactEmail: ""` |
| Mobile lab (this environment, not field data) | `/` LCP 548 ms, CLS 0, TBT 40 ms, ~598 KB transferred; initial JS 409 KB gzip, CSS 73 KB gzip |

**Limitations:** no Search Console, Bing Webmaster Tools, analytics or CrUX access, so no impressions, rankings, click-through rates or field Core Web Vitals are claimed. Lab timings were measured once from a fast environment and are not a substitute for field data. No keyword volumes are invented; validate proposed terms in Search Console, Bing Keyword Research, or a paid keyword tool before committing to a content plan.

---

## 1. Critical

### C1. The documentation is not published, and `/docs` is a soft 200

- **Pass:** Documentation / Technical
- **Where:** `docs/*.md` (45 files), `web/src/lib/links.ts:10`, `web/embed.go`, `cmd/reet/main.go`
- **Evidence:** `DOCS_URL` is `https://github.com/eyupio/reet.sh#quick-start`. `https://reet.sh/docs` returns `200 text/html` with `data-boot="app"` and no documentation. `https://reet.sh/docs/mcp` returns 404 because `WithHostedPages` treats any two-segment path as a hosted-skill URL. The repository contains substantial guides: `binary-installation.md` (~3,166 words), `projects.md` (~5,721), `setup-discovery.md` (~5,629), `terminal-workspace.md` (~8,588), `start-agent-session.md` (~3,641), `mcp.md`, `controller-sync.md`, `backups.md`, `single-sign-on.md`, and more.
- **Problem:** Documentation is the single best organic acquisition asset for a developer tool, and it is currently a GitHub README anchor. Search engines cannot rank individual questions (“how to run Claude Code on a remote VPS”, “self-hosted MCP server”, “sync skills between machines”) because no URLs exist for them. The homepage’s “Installation guide” link sends visitors away from the product domain.
- **Fix:**
  1. Publish a curated public docs set at `/docs` (for example Astro Starlight, MkDocs Material, or a small Go Markdown renderer that reuses the existing binary). Keep the same Go binary and deployment; do not add a separate always-on service if avoidable.
  2. Add front matter to each public page: `title`, `description`, `slug`, `order`, `section`, `updated`.
  3. Reserve `/docs` in the router **before** `WithHostedPages`, and add `/docs/` and `/docs/{path}` handlers.
  4. Generate `/sitemap-docs.xml` (or include docs URLs in the main sitemap) with `lastmod`.
  5. Add canonical, Open Graph, breadcrumbs, previous/next links, an on-page table of contents, “Edit this page on GitHub”, and docs search.
  6. Curate: publish installation, configuration, CLI, MCP, sync, setups, machines, sessions, backups, security/SSO, support, and troubleshooting. Keep design notes, research, handoffs and verification logs (`docs/design`, `docs/research`, `docs/superpowers`, `docs/ui-verification.md`, `docs/work-orchestration-handoff.md`, `docs/structured-chat-plan.md`) out of the public site.
  7. Change `DOCS_URL` to the published docs root (for example `https://reet.sh/docs/`) and update every README/footer link.
- **Impact:** Creates dozens of indexable, high-intent URLs from content that already exists; gives the homepage and every hosted skill page a legitimate internal-linking hub; improves AI-search citation potential.
- **Effort:** Large
- **Confidence:** High

### C2. Public community, public lists and hosted skill pages are not reliably crawlable or correctly canonicalised

- **Pass:** Technical / Content
- **Where:** `web/embed.go:137-188` (`serveIndex`, `stripFallback`), `web/index.html:67` (canonical), `web/src/App.svelte:940-970`, `web/src/lib/PublicList.svelte:20-33`, `web/src/lib/Community.svelte`
- **Evidence:** With JavaScript disabled, `/community` and `/l/32d001e64e86227d` return **0 words**. Their served HTML has `<link rel="canonical" href="https://reet.sh/" />` and the homepage title, description and `og:url`. `PublicList.svelte` updates `document.title` on mount but never updates the description, canonical or OG tags. `/community` never updates any metadata. Hosted skill pages do render server-side, but have no canonical, no Open Graph, and no JSON-LD.
- **Problem:** Search engines that do not render JavaScript (and many AI crawlers) see an empty shell. Engines that do render JavaScript still see a canonical pointing at the homepage, which tells them not to index the public page. Public lists and the community therefore cannot acquire organic traffic despite being deliberately public.
- **Fix:**
  1. **Preferred:** server-render `/community`, `/l/{slug}` and `/{username}/{slug}` with the actual content, either in Go or with SSR/prerendering. Public data is already available from `GET /api/community` and `GET /api/v2/public/lists/{slug}`.
  2. If SSR is too large a first step, generate a route-specific crawlable fallback in `serveIndex` containing the list/community heading, description, skill names and links, and hide it only when JavaScript mounts.
  3. Emit per-route `<title>`, `<meta name="description">`, `<link rel="canonical">`, `og:title`, `og:description`, `og:url` and `og:image`.
  4. Keep the canonical self-referential on public pages; never point a public list or hosted skill at `/`.
  5. Add a visible “Public community” link from the landing page and the footer.
  6. Add public list and community URLs to the sitemap once they render content.
- **Impact:** Turns existing public surfaces into indexable pages; makes shared links preview correctly in Slack, X, LinkedIn and AI answer engines; removes a self-inflicted canonical block.
- **Effort:** Large
- **Confidence:** High

### C3. Every unknown URL returns HTTP 200, including missing text and XML files

- **Pass:** Technical
- **Where:** `web/embed.go:100-130` (fallback to `serveIndex` for any unmatched path), `web/src/App.svelte` (`NotFound` rendered client-side)
- **Evidence:** `/does-not-exist-xyz` → `200`, 0 words without JavaScript. `/llms.txt`, `/ai.txt`, `/sitemap-index.xml`, `/feed.xml` → `200 text/html` with the app shell. `/docs/mcp` → 404 only because it happens to match the hosted-skill route.
- **Problem:** Search engines treat these as soft 404s and waste crawl budget. Tools and agents requesting `llms.txt` or `ai.txt` receive HTML instead of plain text, which is worse than a clean 404. A missing resource should never claim success.
- **Fix:**
  1. Add a server-side allowlist of SPA routes (the routes already enumerated in `App.svelte`) and return `404` with the app shell for anything else. The client can still draw `NotFound`.
  2. Add explicit handlers for `/llms.txt`, `/llms-full.txt`, `/ai.txt` and any feed/sitemap-index file; return real `text/plain` or `application/xml` bodies, or 404 if intentionally absent.
  3. Ensure the 404 response carries `<meta name="robots" content="noindex">` and helpful links to Home, Docs and Community.
  4. Add tests: unknown path → 404; `/llms.txt` → `text/plain`; `/sitemap-index.xml` → `application/xml` or 404.
- **Impact:** Better crawl efficiency, correct agent behaviour, fewer misleading “successful” requests, cleaner analytics.
- **Effort:** Medium
- **Confidence:** High

### C4. Private, authenticated and empty app routes are indexable

- **Pass:** Technical
- **Where:** `web/embed.go:266-289` (`robotsTxt`), no `X-Robots-Tag` or `meta robots` in `serveIndex`, `App.svelte` route list
- **Evidence:** `robots.txt` only disallows `/api/` and `/quick`. `/signin`, `/signup`, `/admin/overview`, `/lists/1`, `/sessions`, `/machines`, `/skills`, `/activity`, `/setup` and `/reset` all return 200. No `noindex` directive exists in the source (excluding vendored types).
- **Problem:** Search engines can index empty app screens, sign-in forms and private-list shells. This creates thin/duplicate pages, leaks the shape of the private product into search results, and competes with the pages that should rank.
- **Fix:**
  1. In `robots.txt`, disallow `/admin/`, `/account/`, `/lists/`, `/sessions`, `/machines`, `/skills`, `/activity`, `/setup`, `/reset`, `/join`, `/notifications`, `/support/`, `/hosting` and `/quick`. Keep `/`, `/welcome`, `/community`, `/l/`, `/docs/` and hosted skill paths allowed.
  2. Emit `X-Robots-Tag: noindex, nofollow` for the app shell on private routes, and `<meta name="robots" content="noindex, nofollow">` in the served HTML.
  3. Do **not** noindex `/l/{slug}` or `/{username}/{slug}`; those are public content. Add a test that asserts the distinction.
  4. Consider a server-side `isPublicRoute(path)` helper shared by robots, metadata and sitemap generation so the rules cannot drift.
- **Impact:** Removes index bloat, protects crawl budget, avoids confusing search results for a self-hosted product where every instance has different accounts.
- **Effort:** Small
- **Confidence:** High

---

## 2. High impact

### H1. The sitemap advertises one URL

- **Pass:** Technical
- **Where:** `web/embed.go:297-308`, `web/embed_test.go:64-79`
- **Evidence:** Live sitemap contains exactly `https://reet.sh/`. The test enforces this. Public community, public lists, hosted skills and (future) docs are absent.
- **Problem:** A one-URL sitemap gives search engines almost no discovery value and makes the sitemap look stale. It also prevents fast indexing of newly published skills and lists.
- **Fix:** Generate the sitemap from the controller’s public data:
  - `/`, `/welcome` (if kept), `/community`
  - every public list `/l/{slug}`
  - every published hosted skill `/{username}/{slug}`
  - every public docs page `/docs/{slug}`
  - `lastmod` from `updatedAt`, and `changefreq`/`priority` only where meaningful.
  Split into a sitemap index if the URL count grows beyond a few thousand. Update the test to assert required URLs are present and every URL returns 200.
- **Impact:** Faster discovery of new public content; better indexing of long-tail skill and list pages.
- **Effort:** Medium
- **Confidence:** High

### H2. Published skill pages are indexable but orphaned and metadata-poor

- **Pass:** Content / Technical
- **Where:** `internal/api/hosted.go:291-312` (the `hostedPage` template), `internal/api/hosted.go:27-38` (`WithHostedPages`)
- **Evidence:** `https://reet.sh/eyupio/ruthless-ux-ui-designer` is server-rendered with ~434 words, but its `<head>` has only title and description. There is no canonical, no Open Graph, no Twitter card, no JSON-LD, no breadcrumb, no related links, no link back to the community or docs, and it is not in the sitemap. The page has two `<h2>` sections and dumps the full SKILL.md into a `<pre>`.
- **Problem:** Skill pages are the closest thing reet.sh has to programmatic long-tail content, but they cannot rank well or preview well, and they pass no authority to the rest of the site.
- **Fix:**
  1. Add self-canonical, `og:title`, `og:description`, `og:type=article`, `og:url`, `og:image` (generate or reuse the OG image), Twitter card and `meta name="robots" content="index,follow"`.
  2. Add JSON-LD `SoftwareSourceCode` or `CreativeWork` with `name`, `description`, `codeRepository`, `author`, `dateModified`, `programmingLanguage` (if known) and `isPartOf`.
  3. Add visible breadcrumbs: `reet.sh / Community / {username} / {skill}` with `BreadcrumbList` markup.
  4. Add “Install”, “Copy install command”, “View source”, “More skills by {username}”, “Browse the community”, “Documentation”.
  5. Render the skill description and headings before the raw `<pre>`; keep the raw content but consider syntax highlighting and a table of contents.
  6. Include published skills in the sitemap and link to them from `/community` and public lists.
- **Impact:** Makes every published skill a real landing page with rich-result eligibility and internal links; increases discovery of reet-hosted skills.
- **Effort:** Medium
- **Confidence:** High

### H3. One description and one canonical are reused for every SPA route

- **Pass:** Technical / Content
- **Where:** `web/index.html:60-84`, `web/src/App.svelte:940-970`, `web/embed.go:137-188`
- **Evidence:** The meta description is 265 characters and describes the product, not the page. `/signin`, `/community`, `/welcome`, `/l/...` and unknown routes all serve it. Title is updated client-side for a few pages; description, canonical and OG are not. `svelte:head` only outputs `<title>`.
- **Problem:** Long descriptions are truncated in results; duplicated descriptions reduce click-through and can cause search engines to rewrite snippets. Duplicate canonicals suppress the wrong pages. Social previews for community/list/skill pages show the homepage card.
- **Fix:** Centralise route metadata in a small TypeScript module (`web/src/lib/seo.ts`) with `title`, `description`, `canonicalPath`, `ogImage`, `robots`. In `App.svelte`, set the full head from it. On the server, substitute the same values for public routes so non-JS crawlers see them. Keep descriptions to ~150–160 characters.
- **Impact:** Better CTR, fewer rewritten snippets, correct social previews, correct indexing.
- **Effort:** Medium
- **Confidence:** High

### H4. There are no keyword-targeted pages beyond the homepage

- **Pass:** Keywords / Content
- **Where:** `web/src/lib/Welcome.svelte` is the only marketing page; `docs/` is unpublished; there is no `blog/`, `guides/`, `features/`, `compare/` or `integrations/`
- **Evidence:** Source and live route checks show only `/`, `/welcome`, `/community`, `/l/{slug}`, `/{username}/{slug}` and app/auth routes.
- **Problem:** A single homepage cannot rank for the many distinct jobs reet performs. Searchers looking for “run Claude Code on a remote server”, “sync AI skills across machines”, “self-hosted MCP server”, “open source AI agent control plane” or “skills.sh alternative” land nowhere. This is the largest missed commercial opportunity.
- **Fix:** Build a small, high-quality page cluster. Each page needs a unique H1, 800–1,500 useful words, screenshots, a concrete walkthrough, internal links and a single CTA. Start with:
  - `/features/multi-machine-agents` — command Claude Code, Codex, OpenCode and Gemini CLI across machines.
  - `/features/skill-sync` — reviewed skill distribution with exact plans.
  - `/features/setups` — reusable AI setup profiles.
  - `/features/backups` — sealed, verified backups for skills and setups.
  - `/features/mcp` — self-hosted MCP access to your shelf.
  - `/guides/run-claude-code-on-a-vps` — browser terminal walkthrough.
  - `/guides/opencode-web-ui` — OpenCode in Gaffer / remote OpenCode.
  - `/guides/connect-a-fresh-vps` — one-command enrolment.
  - `/guides/sync-skills-between-machines` — scan, preview, approve, apply.
  - `/guides/self-host-mcp-server` — OAuth consent and client configuration.
  - `/compare/skills-sh` — directory vs control room.
  - `/compare/open-webui` — chat UI vs agent/machine control.
  - `/alternatives/tailscale-for-ai-agents` — outbound agent vs VPN mesh (framed carefully and fairly).
- **Impact:** Captures problem-aware and solution-aware searches that the homepage cannot; creates internal-link targets for docs and hosted skills.
- **Effort:** Large
- **Confidence:** High

### H5. The anonymous conversion path is backwards for a self-hosted product

- **Pass:** Conversion
- **Where:** `web/src/lib/Welcome.svelte:18-25`, `web/src/lib/links.ts:10`
- **Evidence:** Live status: `signupEnabled: false`, `contactEmail: ""`, `setupRequired: false`. The hero therefore renders **“Sign in →”** as the primary button, **“Install reet.sh”** as a secondary button, and the invite line “Need access? Ask this instance’s owner for an account.” The install command shown near the hero is `reet agent enroll --env fresh-vps`, which is a post-install action, not an installation command. The “Installation guide” link goes to the GitHub README anchor.
- **Problem:** A visitor arriving from search cannot create an account, cannot request an invite (no contact email), and is pushed toward signing in to an account they do not have. The viable action — self-hosting — is visually secondary and leads off-site to a README. This leaks the most valuable organic traffic.
- **Fix:**
  1. For anonymous visitors on the public instance, make **“Install reet.sh”** the primary CTA, linking to the published `/docs/install/` or `/docs/binary-installation/` page.
  2. Put the real installation command (`curl -fsSL … | sh` then `reet install`) in the hero, not the enrolment command. Keep `reet agent enroll` in the “after install” section.
  3. Add a secondary **“Explore the public community”** CTA for visitors who want to see value before installing.
  4. If the hosted instance is invite-only, say so explicitly and offer a waitlist or contact route; do not make “Sign in” the primary action for new visitors.
  5. Add a short “Try it in 5 minutes” section with three steps and a link to docs.
  6. Track clicks on each CTA once analytics is in place.
- **Impact:** Converts organic visitors into installs and community users instead of dead-ending them at a sign-in form.
- **Effort:** Small
- **Confidence:** High

### H6. Public list URLs are opaque and the public corpus is thin

- **Pass:** Content / Keywords
- **Where:** `internal/store` public list slug generation; `/l/32d001e64e86227d`
- **Evidence:** The only public list at audit time is `/l/32d001e64e86227d`, named “reet-jnnngs”, with an empty description, 3 skills, and no server-rendered content. The slug is a 16-hex-character token.
- **Problem:** The URL contains no keywords, no human-readable context, and the page has no description. It cannot rank and is not compelling when shared.
- **Fix:**
  1. Add a human-readable slug derived from the list name (for example `/l/reet-jnnngs-32d001e`), keep the existing opaque slug as a permanent redirect, and never reuse a slug.
  2. Require or strongly encourage a description when a list is made public; render it as the meta description and page lede.
  3. Show skill descriptions, source, install commands and a short “what this list is for” section.
  4. Consider curated editorial lists (“Best skills for code review”, “Claude Code starter skills”) as genuine content, not doorway pages.
- **Impact:** Makes public lists shareable and indexable; improves community quality and long-tail coverage.
- **Effort:** Medium
- **Confidence:** Medium

### H7. No measurement, verification or regression protection

- **Pass:** Measurement
- **Where:** repository-wide; no Search Console verification, Bing verification, analytics, IndexNow key, or SEO test beyond sitemap/robots presence
- **Evidence:** Search for `gtag`, `googletagmanager`, `plausible`, `umami`, `posthog`, `matomo`, `search-console`, `indexnow`, `google-site-verification`, `msvalidate` found nothing in application source or workflows.
- **Problem:** There is no way to know which pages are indexed, which queries bring traffic, whether canonicals are respected, or whether a change causes a regression. SEO work becomes guesswork.
- **Fix:**
  1. Verify `reet.sh` in Google Search Console and Bing Webmaster Tools (DNS verification; do not commit tokens). Submit `/sitemap.xml`.
  2. Add a privacy-respecting analytics option (Cloudflare Web Analytics, self-hosted Plausible/Umami, or a small server-side aggregate counter) with an environment flag, so forks are not forced into telemetry. reet’s “no telemetry” promise must remain true by default.
  3. Add IndexNow for published docs/skills if Bing/Copilot visibility matters.
  4. Add CI checks: `lychee` or equivalent link checker on built HTML and docs; sitemap XML validation; a test that every public route has a unique title, description and self-canonical; a test that private routes are noindex.
  5. Add a monthly “SEO health” job that checks status codes, canonical tags, sitemap URLs and missing metadata.
- **Impact:** Makes SEO measurable and prevents silent regressions; gives the team evidence for what to write next.
- **Effort:** Medium
- **Confidence:** High

### H8. Old URLs are rewritten only in the browser, not redirected on the server

- **Pass:** Technical
- **Where:** `web/src/App.svelte:139-148` (`/projects`, `/control`, `/terminals`, `/sessions/`)
- **Evidence:** The client calls `history.replaceState`; the server still returns 200 for the old URL. No 301/308 handlers exist in `web/embed.go` or `cmd/reet/main.go`.
- **Problem:** Search engines may index old URLs or treat them as duplicates; external links to old URLs do not pass signal cleanly. Users without JavaScript see nothing useful at the old URL.
- **Fix:** Add permanent redirects in Go before the SPA fallback:
  - `/control` → `/`
  - `/terminals` → `/sessions` (preserve query and fragment where possible)
  - `/projects` and `/projects/{id}` → `/?project={id}` or `/`
  - `/sessions/` → `/sessions`
  Use `301` for permanent moves and keep them tested.
- **Impact:** Consolidates link equity, removes duplicate URLs, improves non-JS behaviour.
- **Effort:** Small
- **Confidence:** High

### H9. AI-search discoverability is mostly absent

- **Pass:** AI Search
- **Where:** no `llms.txt`, no `llms-full.txt`, no FAQ/answer content, no docs site, client-only public pages; `web/embed.go:266-289`
- **Evidence:** `/llms.txt` and `/ai.txt` return `200 text/html` app shells. The landing fallback is the only non-JS content. `robots.txt` explicitly allows `OAI-SearchBot`, `Claude-SearchBot`, `PerplexityBot` and `Googlebot`, and leaves training crawlers like `GPTBot` unconstrained by default.
- **Problem:** AI answer engines cite crawlable, well-structured, factual pages. reet.sh currently gives them one landing page and empty shells for everything else. It also cannot control the difference between “cite me” crawlers and “train on me” crawlers.
- **Fix:**
  1. Publish real `/llms.txt` (short index of key pages) and `/llms-full.txt` (concise product/docs summary) as `text/plain`.
  2. Publish docs and comparison pages; AI engines need stable URLs and extractable answers.
  3. Add a concise “What is reet.sh?” definition near the top of the homepage and docs, using consistent wording across the site, README and GitHub description.
  4. Add FAQ sections to feature pages answering real questions (“Does reet.sh replace Claude Code?”, “Does it require a VPN?”, “Where is my data stored?”, “Is it free?”). Use `FAQPage` structured data only where the answers are genuinely on the page.
  5. Decide the crawler policy explicitly: allow `OAI-SearchBot`, `Claude-SearchBot`, `PerplexityBot`, `Googlebot`; consider disallowing `GPTBot`, `ClaudeBot`, `Google-Extended`, `CCBot` if training opt-out is desired. Document the choice in `robots.txt` comments.
  6. Do not claim that `llms.txt` or schema guarantees AI visibility; they improve machine readability, nothing more.
- **Impact:** Increases the chance of being cited by ChatGPT search, Perplexity, Copilot and AI Overviews for high-intent developer queries.
- **Effort:** Medium
- **Confidence:** Medium

---

## 3. Nice to have

### N1. Split the marketing page from the application bundle

- **Pass:** Technical
- **Where:** `web/vite.config.ts`, `web/src/App.svelte`, `web/index.html`
- **Evidence:** Initial JS is ~409 KB gzip and CSS ~73 KB gzip. The landing page content is rendered by the SPA after the bundle loads; the only non-JS copy is the hidden crawler fallback. Mobile lab LCP was 548 ms in this environment, so this is not currently a measured failure.
- **Problem:** On slower mobile networks and devices, the landing page’s meaningful content depends on a large application bundle. This is a CWV risk and forces crawlers to rely on JavaScript.
- **Fix:** Render the marketing page as static HTML and hydrate only the interactive pieces (theme picker, copy buttons, animated control-room preview), or code-split the app routes so `/` and `/welcome` load a small marketing bundle. Defer the terminal workspace and Gaffer chunks until needed (already partly split).
- **Impact:** Better LCP/INP on mobile, less JS for crawlers and ad blockers, lower bounce.
- **Effort:** Medium
- **Confidence:** Medium

### N2. Add baseline security headers

- **Pass:** Technical
- **Where:** `cmd/reet/main.go:366-367`, `web/embed.go`
- **Evidence:** Live HTML responses show `content-type` and `cache-control` only; no HSTS, CSP, `X-Content-Type-Options`, `Referrer-Policy`, `X-Frame-Options` or `Permissions-Policy` (Cloudflare did not add them).
- **Problem:** Not a ranking factor, but missing headers weaken trust and can affect how intermediaries and browsers treat the site. Some SEO tools flag them.
- **Fix:** Add a small middleware for HTML: `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin`, `X-Frame-Options: DENY` (or CSP `frame-ancestors 'none'`), `Permissions-Policy` limiting unused features, and HSTS only when served over HTTPS with a documented rollout.
- **Impact:** Trust and defence in depth; no direct ranking claim.
- **Effort:** Small
- **Confidence:** High

### N3. Polish social metadata

- **Pass:** Technical / Content
- **Where:** `web/index.html:70-84`, `internal/api/hosted.go:291`
- **Evidence:** OG and Twitter tags exist on the landing page. `twitter:site` is absent. Hosted skill pages have no OG tags at all. `og:image` is present and returns 63,828 bytes.
- **Fix:** Add `twitter:site` (if an account exists), `og:image:secure_url`, `og:locale`, and per-page images for docs and skills. Keep `og:image:alt` accurate.
- **Impact:** Better social previews and brand consistency.
- **Effort:** Small
- **Confidence:** High

### N4. Add breadcrumbs and richer structured data to public pages

- **Pass:** Technical
- **Where:** `web/index.html` JSON-LD, hosted page template, future docs
- **Evidence:** Only `Organization`, `WebSite` and `SoftwareApplication` exist, on every route that serves `index.html`.
- **Fix:** Add `BreadcrumbList` to docs, public lists and skill pages; `TechArticle`/`HowTo` where appropriate; `SoftwareSourceCode` for hosted skills. Remove `SoftwareApplication` from private app routes if those are noindex anyway.
- **Impact:** Richer result eligibility and clearer entity relationships.
- **Effort:** Small–Medium
- **Confidence:** Medium

### N5. Improve internal linking between public surfaces

- **Pass:** Content / Technical
- **Where:** `web/src/lib/Welcome.svelte`, `web/src/lib/Footer.svelte`, `Community.svelte`, `PublicList.svelte`
- **Evidence:** The landing page links to sign-in, install and GitHub; the footer’s public links appear only when signed out; hosted skill pages link only to `/` and `/hosting`; public lists link to the community and the visitor’s own shelf.
- **Fix:** Add a consistent public footer with Home, Community, Docs, Guides, GitHub, Licence and Status. Link from the homepage to Community, Docs and two or three guides. Link from docs to relevant feature pages and back. Link from skill pages to the author’s other skills and the community.
- **Impact:** Distributes authority, improves crawl depth, keeps visitors on-site.
- **Effort:** Small
- **Confidence:** High

### N6. Canonicalise the duplicate landing pages

- **Pass:** Technical
- **Where:** `web/embed.go:137-188`, `web/index.html:67`
- **Evidence:** `/` and `/welcome` both serve the full landing fallback and both canonicalise to `/`.
- **Problem:** This is currently correct for `/welcome` (if it is a deliberate alias), but should be explicit and tested; otherwise it becomes accidental duplication later.
- **Fix:** Keep `/welcome` as a 301 to `/` or keep the canonical to `/` and exclude `/welcome` from the sitemap. Add a test documenting the choice.
- **Impact:** Prevents duplicate-content drift.
- **Effort:** Small
- **Confidence:** High

### N7. Optimise unused or oversized public images

- **Pass:** Technical
- **Where:** `web/public/`, `web/dist/assets/`
- **Evidence:** `shelf-dark.png` is 304,988 bytes, `shelf-light.png` 300,081 bytes, and `gafferHead-ZxHEy_hX.png` is 673,526 bytes (gzip 516,868). These are not loaded on the landing page, but they are shipped in the build and may be used elsewhere.
- **Fix:** Convert screenshots to WebP/AVIF with fallbacks, serve responsive sizes, and keep only assets that are actually referenced. Do not lazy-load the LCP image; do lazy-load below-the-fold screenshots.
- **Impact:** Lower transfer for pages that use them; cleaner build.
- **Effort:** Small
- **Confidence:** Medium

---

## Keyword-to-page mapping

No search volumes are claimed. Validate every term in Search Console or a keyword tool before prioritising.

| Intent | Proposed keyword / query | Target page | Status |
|---|---|---|---|
| Product category | self-hosted AI agent control room | `/` | Exists; strengthen H1 and definition |
| Product category | open source AI agent control plane | `/` / `/features/multi-machine-agents` | Missing |
| Problem-aware | manage AI agents on multiple machines | `/features/multi-machine-agents` | Missing |
| Problem-aware | sync AI skills across machines | `/features/skill-sync` | Missing |
| Problem-aware | AI setup profiles / reusable AI environments | `/features/setups` | Missing |
| Integration | run Claude Code on a remote VPS | `/guides/run-claude-code-on-a-vps` | Missing |
| Integration | Claude Code browser terminal | `/guides/claude-code-web-terminal` | Missing |
| Integration | OpenCode web UI self-hosted | `/guides/opencode-web-ui` | Missing |
| Integration | run Codex CLI on a remote machine | `/guides/codex-cli-remote` | Missing |
| Integration | Gemini CLI remote session | `/guides/gemini-cli-remote` | Missing |
| Integration | self-hosted MCP server | `/features/mcp` / `/guides/self-host-mcp-server` | Missing |
| Comparison | skills.sh alternative | `/compare/skills-sh` | Missing |
| Comparison | Open WebUI alternative for agent control | `/compare/open-webui` | Missing |
| Comparison | LibreChat vs agent control room | `/compare/librechat` | Missing |
| Comparison | do I need Tailscale to run AI agents remotely | `/guides/remote-agents-without-vpn` | Missing |
| Documentation | reet install / reet CLI reference | `/docs/install/` / `/docs/cli-reference/` | Content exists, unpublished |
| Documentation | reet environment sync | `/docs/environment-sync/` | Content exists, unpublished |
| Documentation | reet backups | `/docs/backups/` | Content exists, unpublished |
| Documentation | reet SSO / OIDC | `/docs/single-sign-on/` | Content exists, unpublished |
| Directory | AI agent skills for code review | `/community` + curated lists | Exists but not crawlable |
| Long-tail | self-hosted alternative to skills.sh | `/compare/skills-sh` | Missing |

**Cannibalisation risk:** `/` and `/welcome` currently target the same intent; resolve as above. Future feature pages must each own one job and link to siblings rather than repeating the homepage’s full pitch.

---

## Competitor positioning opportunities

Observed live titles and descriptions on 4 November 2026 (no traffic, ranking or backlink data is claimed):

| Competitor | Observed positioning | reet.sh opportunity |
|---|---|---|
| **skills.sh** | “The Agent Skills Directory” — “Discover and install skills for AI agents.” | skills.sh is a public directory. reet.sh is the control room around a private library: review, sync, pin, back up and apply skills across machines. Own “self-hosted skill library”, “skill sync”, “skills.sh alternative for teams”. |
| **Smithery** | “MCP servers, now part of Arcade” — discovery of MCP servers. | Smithery is a hosted catalogue. reet.sh hosts an MCP server over your own shelf with OAuth consent. Own “self-hosted MCP server”, “MCP access to your own tools”, “MCP without a third-party catalogue”. |
| **Open WebUI** | “Self-Hosted AI Platform” — run AI on your own terms. | Open WebUI is a chat interface. reet.sh commands agents on machines. A fair comparison page should say they are complementary: “Open WebUI is where you chat; reet.sh is where your agents and machines are controlled.” |
| **LibreChat** | “The Open-Source AI Platform” — unified AI conversations. | Same as Open WebUI: chat-centric vs fleet/agent-centric. Avoid attacking; position by job-to-be-done. |
| **Tailscale** | “Secure Connectivity for AI, IoT & Multi-Cloud.” | reet.sh’s agent dials out, so there is no inbound port or VPN mesh to configure. Own “run AI agents on remote machines without opening ports” and “remote agent access without a VPN”. |
| **Cursor** | “AI Coding Agent for Building Ambitious Software.” | Cursor is an editor/agent. reet.sh is the remote control room around terminal agents. Own “run coding agents on your own machines”, “continue agent sessions from the browser”. |
| **Letta** | “Machines that learn” — stateful agents. | Different category (agent memory/runtime). Do not force a comparison; link to it only if reet integrates. |
| **Anthropic Claude Code / OpenAI Codex / OpenCode / Gemini CLI** | Agent products, not fleet controllers. | Highest-value integration content: install, authenticate, run remotely, resume, review diffs and CI. These are the searches reet’s users actually make. |

**Positioning sentence to reuse consistently:** “reet.sh is a self-hosted control room for the AI agents, skills and setups across all your machines. One Go binary, one SQLite file, no telemetry.”

---

## Documentation improvements

1. **Publish a curated docs site** at `/docs` with stable, keyword-bearing URLs (`/docs/install/`, `/docs/cli-reference/`, `/docs/mcp/`, `/docs/sync/`, `/docs/backups/`, `/docs/sso/`, `/docs/troubleshooting/`).
2. **Front matter for every public page:** title, description, slug, section, order, last-updated. Generate the sidebar and sitemap from it.
3. **Keep internal material out:** design notes, research, superpowers, handoffs, verification logs and implementation slices should not be public unless rewritten for users.
4. **Fix the `/docs/mcp` routing collision:** reserve `/docs/` before hosted-skill routing. Today `/docs/mcp` is intercepted as `/{username}/{slug}` and returns 404.
5. **Information architecture:** Getting started → Install → Connect a machine → First agent session → Skills → Sync → Setups → Backups → Security/SSO → CLI reference → Troubleshooting.
6. **Every page needs:** one H1, a one-sentence answer at the top, a table of contents, copyable commands, previous/next links, “Edit this page”, and links to related feature/guide pages.
7. **Troubleshooting content:** turn real error messages and `reet doctor` output into searchable pages. These are high-intent, low-competition queries.
8. **Screenshots:** use descriptive alt text and optimise the existing PNGs in `docs/images/`; add captions that explain the workflow, not just the UI.
9. **Versioning:** state the version the docs apply to and keep `cli-reference.md` generated (it already is).
10. **Link docs from the homepage, README and app footer.** The current `DOCS_URL` points to a GitHub anchor; make it point to the published docs.

---

## AI search discoverability recommendations

- Publish real `llms.txt` and `llms-full.txt` as `text/plain`; link the most important docs and feature pages. Do not overstate their effect.
- Publish stable, factual pages with extractable answers: “What is reet.sh?”, “How does reet.sh connect to a machine?”, “Does reet.sh send telemetry?”, “Can I self-host it?”, “Which agents does it support?”.
- Use one consistent product description everywhere (site, README, GitHub, docs, package metadata).
- Add `BreadcrumbList` and `SoftwareSourceCode`/`TechArticle` where accurate. Do not add ratings, prices or claims that are not true.
- Keep `OAI-SearchBot`, `Claude-SearchBot`, `PerplexityBot` and `Googlebot` allowed; make the training-crawler policy explicit (`GPTBot`, `ClaudeBot`, `Google-Extended`, `CCBot`).
- Make public pages readable without JavaScript. AI crawlers are less consistent than Googlebot about rendering.
- Earn legitimate external references through docs, GitHub, skill pages and integrations; do not buy links or mass-generate pages.
- Measure AI referrals separately in analytics (referrer buckets for ChatGPT, Perplexity, Copilot, Gemini) once analytics exists.

---

## Prioritised implementation roadmap (for reet-babysit)

Each step should be one small PR, test-first where possible, with CI gating and no visual regression on the landing page.

### Phase 0 — Baseline and guardrails (1–2 days)

1. Add Google Search Console and Bing Webmaster Tools verification via DNS, outside the repo. Submit the current sitemap and record the baseline.
2. Add a `docs/seo-baseline.md` note with the verified evidence from this audit and the date.
3. Add CI checks:
   - every public route has a unique title and description;
   - every public route has a self-canonical;
   - every private route has `noindex`;
   - unknown routes return 404;
   - sitemap XML is valid and every URL returns 200;
   - built HTML has no broken internal links.
4. Add a lightweight, opt-in analytics hook (default off) only if the owner approves it; preserve “no telemetry” as the default.

**Acceptance:** CI fails on a missing canonical or a 200 soft 404; GSC/Bing properties verified; baseline recorded.

### Phase 1 — Fix indexing bugs (2–4 days)

5. Add `isPublicRoute(path)` and route allowlisting in `web/embed.go`; return 404 for unknown paths.
6. Add `/llms.txt`, `/llms-full.txt` and `/ai.txt` handlers (real text), or 404 them if not wanted.
7. Add private-route `Disallow` rules and `X-Robots-Tag: noindex` for app/auth routes.
8. Add permanent redirects for `/control`, `/terminals`, `/projects`, `/sessions/`.
9. Add regression tests for robots, sitemap, canonicals and status codes.

**Acceptance:** `/does-not-exist` → 404; `/llms.txt` → `text/plain`; private routes → `noindex`; old routes → 301; tests pass.

### Phase 2 — Publish docs (1–2 weeks)

10. Choose the docs renderer and reserve `/docs/` in the router.
11. Add front matter and a curated public manifest; exclude internal design/research docs.
12. Build the docs index, sidebar, search, breadcrumbs, previous/next and “Edit this page”.
13. Generate docs metadata, canonical, OG and JSON-LD; add docs URLs to the sitemap.
14. Rewrite `DOCS_URL`, README links and footer links to `/docs/`.
15. Add a docs smoke test: every sitemap docs URL returns 200 and has one H1, a title, a description and a canonical.

**Acceptance:** `/docs/` and `/docs/mcp/` return real HTML; `/docs` is in the sitemap; no internal design docs are public; all docs links resolve.

### Phase 3 — Make public surfaces crawlable (1–2 weeks)

16. Server-render or add crawlable fallbacks for `/community`, `/l/{slug}` and hosted skill pages.
17. Add per-route metadata and self-canonicals for those routes.
18. Add hosted-skill OG, JSON-LD, breadcrumbs and related links.
19. Add public lists and published skills to the sitemap with `lastmod`.
20. Add human-readable list slugs with permanent redirects from old opaque slugs.
21. Add internal links from the homepage/footer/community to docs, lists and skills.

**Acceptance:** No-JS rendering of `/community` and a public list returns at least 150–200 meaningful words; canonical is self; each page has a unique title/description; sitemap includes them.

### Phase 4 — Build intent pages (2–4 weeks)

22. Ship the first four feature pages: multi-machine agents, skill sync, setups, MCP.
23. Ship the first four guides: Claude Code on a VPS, OpenCode web UI, connect a fresh VPS, sync skills between machines.
24. Ship two fair comparison pages: skills.sh and Open WebUI.
25. Add FAQ sections and internal links from docs and the homepage.
26. Add analytics events for CTA clicks and docs scroll depth.

**Acceptance:** 8–12 new pages, each with a unique title/description/H1, 800+ useful words, a screenshot or diagram, at least three internal links, and a single primary CTA.

### Phase 5 — Conversion and polish (1 week)

27. Reorder the anonymous hero CTAs: install first, community second, sign in third.
28. Replace the hero enrolment command with the real install command; move enrolment below.
29. Add a “Try it in 5 minutes” block and a public-community link.
30. Add security headers and social metadata polish.
31. Split the marketing bundle from the app bundle and keep the landing static-first.

**Acceptance:** Anonymous visitor can reach install docs in one click and community in one click; lab LCP remains < 2.5 s; no regression in existing UI tests.

---

## Measurable acceptance criteria

Use these as the definition of done for the roadmap:

- `curl -s -o /dev/null -w '%{http_code}' https://reet.sh/does-not-exist` returns **404**.
- `curl -sI https://reet.sh/llms.txt` returns `content-type: text/plain` (or a deliberate 404, never `text/html`).
- `curl -s https://reet.sh/community` contains at least **150 words** of public content without JavaScript.
- `curl -s https://reet.sh/l/<slug>` contains at least **150 words** and a self-referential canonical without JavaScript.
- Every public page has exactly one `<title>`, one `<meta name="description">` of 120–160 characters, and one self-canonical.
- Every private/app route returns `X-Robots-Tag: noindex` and is absent from the sitemap.
- `/sitemap.xml` contains `/`, `/community`, every public list, every published hosted skill, and every public docs page; every URL returns 200 with no redirect.
- `/docs/` and `/docs/mcp/` return documentation HTML with unique metadata; `/docs/mcp` is not intercepted by hosted-skill routing.
- Google Search Console and Bing Webmaster Tools are verified; the sitemap is submitted; index coverage is reviewed weekly.
- No-JS rendering of `/`, `/community`, a public list, a hosted skill and a docs page each returns meaningful content (no zero-word pages).
- Mobile lab Lighthouse (performance, SEO, accessibility, best practices) is ≥ 90 on the landing page; LCP < 2.5 s, CLS < 0.1. Treat lab results as directional; track field data once available.
- CI fails on: missing canonical, duplicate title, 200 soft 404, sitemap URL that does not return 200, broken internal link, or public page with no description.
- At least eight new intent pages are live, each with a unique title/description/H1 and a single primary CTA.
- Anonymous landing-page CTA click-through to install docs and community is measured and improves against the baseline.

---

## Appendix — Key file references

| Concern | File / location |
|---|---|
| Global head metadata, JSON-LD, crawler fallback | `web/index.html:1-120`, `web/index.html:331-386` |
| SPA fallback and metadata substitution | `web/embed.go:100-188` |
| robots.txt and sitemap.xml | `web/embed.go:266-308` |
| SPA routing and page titles | `web/src/App.svelte:139-198`, `web/src/App.svelte:940-970` |
| Landing page content and CTAs | `web/src/lib/Welcome.svelte:6-25`, `:31-53` |
| Public list page | `web/src/lib/PublicList.svelte:20-33` |
| Community page | `web/src/lib/Community.svelte` |
| Hosted skill page template | `internal/api/hosted.go:291-312` |
| Hosted route interception | `internal/api/hosted.go:27-38` |
| Server route assembly | `cmd/reet/main.go:361-367` |
| Docs links and README anchor | `web/src/lib/links.ts:10` |
| Public documentation source | `docs/*.md` |
| Sitemap/robots tests | `web/embed_test.go:36-79` |
| Vite build configuration | `web/vite.config.ts` |
| Manifest | `web/public/site.webmanifest` |
| Live OG image | `web/public/og-image.png` |
