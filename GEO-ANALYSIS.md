# GEO / AI Search Analysis — reet.sh

**Target:** https://reet.sh
**Source reviewed:** `/root/reet` (Go server + Svelte SPA), HEAD `99c64ba` (fast-forwarded to `origin/main`, which matches the deployed site)
**Analysis date:** 2026-10-24
**Framework:** Google's AI optimization guide (2026-05-15, updated 2026-07-10). GEO findings below are framed as SEO fundamentals applied to AI-search surfaces, not a separate discipline.

> **Implementation status.** The two Critical items (real `robots.txt`/`sitemap.xml`, and crawler-readable landing content) are implemented in the working tree, plus canonical and JSON-LD. They are **not yet deployed** to https://reet.sh. See "Implementation log" at the end for files, tests and verification. Scores below show pre-fix and post-fix values; the live site is still at the pre-fix score until a deploy ships.

---

## 1. GEO Readiness Score: 33/100

| Criterion | Weight | Pre-fix | Post-fix | Post weighted |
|---|---:|---:|---:|---:|
| Citability | 25% | 25 | 45 | 11.3 |
| Structural readability | 20% | 55 | 65 | 13.0 |
| Multi-modal content | 15% | 45 | 45 | 6.8 |
| Authority & brand signals | 20% | 15 | 15 | 3.0 |
| Technical accessibility | 20% | 30 | 70 | 14.0 |
| **Total** | | **33 / 100** | **48 / 100** | |

Pre-fix, the score was dominated by one fact: **the public page had 7 words of body text in raw HTML and ~480 words after JavaScript ran.** AI crawlers that do not execute JavaScript saw a title and a meta description — nothing citable.

Post-fix (in repo, not deployed), raw HTML serves **314 words**, a real `robots.txt`, a valid `sitemap.xml`, a canonical URL, and Organization/WebSite/SoftwareApplication JSON-LD. Remaining drags are authority (no entity footprint), dates/sources (no citation-grade facts), multi-modal depth, and the still-unfixed soft-404/HTTP-redirect issues.

---

## 2. Platform Breakdown

No DataForSEO, SE Ranking, Profound, or Ahrefs MCP is configured in this session, so **no platform citation score was measured**. The following is qualitative readiness only.

| Surface | Readiness | Basis |
|---|---|---|
| Google AI Overviews | Low | Googlebot renders JS, so content is technically reachable — but `site:reet.sh` returned zero URLs, there is no schema, no sitemap, and no rankings to be selected from. |
| Google AI Mode | Low | Same index, broader pool; freshness and entity authority are missing (no dates, no entity footprint). |
| ChatGPT Search | Very low | `OAI-SearchBot` fetches raw HTML only; it receives a 2.6 KB app shell with an empty `<div id="app">`. Nothing to quote. |
| Perplexity | Very low | `PerplexityBot` fetches raw HTML only; same empty shell. |
| Claude search | Very low | `Claude-SearchBot` fetches raw HTML only; same empty shell. |

**Key asymmetry:** Google renders the SPA and can index the marketing copy; OpenAI, Anthropic, and Perplexity cannot. Every non-Google AI surface currently sees a blank page.

---

## 3. AI Crawler Access Status

**As found on the live site:** `/robots.txt` **is not a robots.txt file.** It was swallowed by the SPA catch-all (`web/embed.go:20-31`) and returned the 2,628-byte `index.html` with `HTTP 200 text/html`. Consequently there were **no robots.txt rules at all** — nothing was disallowed, but no policy was published either. The same catch-all served `/sitemap.xml` and `/llms.txt` as HTML.

**Fixed in the repo (not yet deployed):** the SPA handler now answers `/robots.txt` (`text/plain`, 494 bytes) and `/sitemap.xml` (`application/xml`) before the file server, so they can no longer be shadowed. The text is derived from the request origin (or `REET_PUBLIC_URL`), so self-hosted instances get their own sitemap URL. It explicitly allows `OAI-SearchBot`, `Claude-SearchBot`, `PerplexityBot`, and `Googlebot`, allows all other crawlers including training bots, and disallows only `/api/` and `/quick`.

Live fetch tests with each bot's user-agent all returned `HTTP 200 text/html 2,628b` (no Cloudflare block, no challenge). What each bot can *extract* differs. The robots status column describes the live site **before** the fix below:

| Bot | Governs | robots.txt (live, pre-fix) | Fetch result | Sees |
|---|---|---|---|---|
| `Googlebot` | Google Search inclusion, AI Overviews, AI Mode | No rules (file invalid) | 200 | Renders JS → ~480 words |
| `Google-Extended` | Gemini/Vertex training & grounding, and training of models behind Search gen-AI | No rules (file invalid) | n/a | Training control only — **not** an AI Overviews signal |
| `OAI-SearchBot` | **ChatGPT Search citability** | No rules | 200 | Raw shell → **no body text** |
| `GPTBot` | OpenAI model **training only** | No rules | 200 | Raw shell (irrelevant to ChatGPT Search) |
| `Claude-SearchBot` | **Claude search citability** | No rules | 200 | Raw shell → **no body text** |
| `ClaudeBot` | Anthropic model **training only** | No rules | 200 | Raw shell |
| `PerplexityBot` | Perplexity AI search | No rules | 200 | Raw shell → **no body text** |
| `Applebot` | Siri / Spotlight / Safari discovery | No rules | 200 | Raw shell |
| `Applebot-Extended` | Apple Intelligence training opt-out only | No rules | n/a | Training control only |
| `CCBot` | Common Crawl training data | No rules | 200 | Raw shell |

Interpretation, kept deliberately separate:

- **Search citability (measured):** `OAI-SearchBot`, `Claude-SearchBot`, `PerplexityBot`, and `Googlebot` are all permitted to fetch. Three of the four cannot read the content because it is client-rendered. The blocker is rendering, not permissions.
- **Training access (unmeasured, policy choice):** `GPTBot`, `ClaudeBot`, `CCBot`, `Google-Extended`, and `Applebot-Extended` have no rules. Whether to allow or block is a licensing decision, not a visibility lever.
- **Do not conflate the pairs.** The live site currently supports neither claim cleanly: blocking `GPTBot` would not affect ChatGPT Search, and allowing `Google-Extended` does nothing for AI Overviews.

**No `X-Robots-Tag`, no `Content-Language`, no `Link: rel=canonical` header** on the HTML response.

---

## 4. llms.txt Status: absent

- `https://reet.sh/llms.txt` → `HTTP 200 text/html` (the SPA shell). There is no llms.txt file.
- **This is not a defect for Google.** Google states llms.txt and AI-text files are not needed for Search and do not help or hurt visibility or rankings.
- Build one only for non-Google systems that may read it. Do not present it as a ranking or citation lever, and do not let it displace a real `robots.txt`/`sitemap.xml`.

**RSL 1.0:** no licensing metadata found.

---

## 5. Brand Mention Analysis

Measured (2026-10-24, session web-search provider):

| Signal | Finding |
|---|---|
| Exact-phrase brand query (`"reet.sh" AI fleet control room`) | No reet.sh-owned or third-party mentions in the top results — all results were unrelated "fleet control room" content. |
| `site:reet.sh` | **Zero indexed URLs returned.** |
| Wikipedia / Wikidata | No entity found. |
| Reddit / YouTube / LinkedIn | No mentions surfaced. |
| GitHub | `github.com/eyupio/reet.sh` exists and is linked from the page; `eyup.io` is credited as the maker. |

This is a proxy, not a citation measurement. It is consistent with one conclusion: the brand has essentially no entity footprint for AI systems to retrieve, which matters because brand mentions correlate with AI visibility far more strongly than backlinks (Ahrefs, 2025-12-12 — third-party study). For a dev-tool launch, GitHub and docs are usually the fastest path to retrievable entity signals.

---

## 6. Passage-Level Citability

Extracted rendered text: **1,718 characters (~480 words including nav)**, of which roughly 350 words are marketing copy.

**What works (one passage):**

> "reet.sh brings your machines, AI agents, skills and setups into one self-hosted workspace. Connect your fleet, use the agents already installed on each machine, and choose what gets synced where."

This is a clean "X is/does Y" definition in ~30 words, front-loaded in the rendered page. It is the single most quotable sentence on the site.

**What blocked citation (fixed in repo):**

- It sat behind JavaScript. The raw HTML contained **7 words**. Post-fix, the same landing copy is in `index.html` as 314 raw words, so a non-JS agent can quote it.

**Still weak after the fix:**

- The copy is a product landing page: no statistics, no benchmark, no unique data point, no dated claim, no primary-source links.
- No self-contained answer block of the 130–170 word kind commonly extracted by AI systems (this is a readability heuristic, not a Google requirement).
- The first 30% of rendered text is hero copy plus mocked terminal labels (`workstation`, `buildbox`, `developer@workstation`) — low information density where ~44% of citations are reported to originate (SE Ranking — third-party).
- The H1 is a slogan ("One control room. For your AI fleet."), not a definition or query match.
- No source attribution for claims such as "no telemetry" — verifiable claims without a link are weaker citations.

---

## 7. Server-Side Rendering Check: live fails, repo fixed

| Check | Live (pre-fix) | Repo (post-fix) |
|---|---|---|
| Raw HTML body | `<div id="app"></div>` — `web/index.html:42` | static landing copy inside `#app` |
| Raw body word count | 7 | **314** |
| Rendered body word count | ~480 | ~480 (fallback replaced on mount) |
| `is_spa` (headless renderer) | `true` | `true` |
| Render time | ~3.9 s | unchanged |
| Served by | Cloudflare → Go SPA handler (`web/embed.go:20-31`) | same handler, origin-aware |
| JavaScript required for body content | Yes | **No** |

`web/src/App.svelte` is a client-side dashboard that calls `/api` on mount; unauthenticated visitors get the public landing component. There is still no prerender/SSG step, so the static copy in `web/index.html` is a maintained duplicate of `Welcome.svelte` — it must be updated with it. `web/src/main.ts` clears the fallback before mounting, and the Go handler strips it entirely on non-landing routes (`/signin`, `/hosting`, …).

Verified end to end with a local build: `GET /` returns 8,519 bytes / 314 words with the fallback present; `GET /signin` returns zero occurrences of the marketing copy.

---

## 8. Top 5 Highest-Impact Changes

1. **Stop the SPA catch-all from serving `/robots.txt`, `/sitemap.xml`, and `/llms.txt`.**
   *First principle:* a crawler policy file must be a real text file; an HTML 200 is not a policy. *Dependency:* this unblocks every explicit crawler instruction and sitemap discovery. *Falsifier:* `curl -I /robots.txt` still returns `text/html`. *Leading indicator:* Search Console "robots.txt fetch" reports success.
   *Fix location:* `web/embed.go:20-31` — handle these paths before the `index.html` fallback.
   ```txt
   User-agent: *
   Allow: /

   Sitemap: https://reet.sh/sitemap.xml
   ```
   Keep training bots (`GPTBot`, `ClaudeBot`, `CCBot`, `Google-Extended`) a deliberate separate decision.
   **Status: done** — handled in `web/embed.go` (`writeRobots`, `writeSitemap`), with tests.

2. **Pre-render the public landing page.**
   *First principle:* AI search crawlers outside Google fetch HTML without executing JS. *Dependency:* prerequisite for #3–#5 having any effect on ChatGPT/Perplexity/Claude. *Falsifier:* `curl https://reet.sh/ | grep "self-hosted workspace"` returns nothing. *Leading indicator:* raw HTML word count rises from 7 to 300+.
   Minimal version: statically inject the hero, definition, feature H2s, install command, and Q&A into `index.html` (or add an SSG step). Full version: prerender `/` and `/welcome`.
   **Status: done** — static copy in `web/index.html`, removed on mount in `web/src/main.ts`, stripped on app routes by `serveIndex`.

3. **Add entity schema.**
   *First principle:* AI systems need disambiguated entities; the page currently declares none (0 JSON-LD blocks). *Dependency:* benefits from #2 only in the sense that script tags already work in raw HTML. *Falsifier:* Rich Results Test shows no Organization/SoftwareApplication. *Leading indicator:* brand-name query starts returning reet.sh URLs.
   ```json
   {"@context":"https://schema.org","@graph":[
     {"@type":"Organization","@id":"https://reet.sh/#org","name":"EyUp.io",
      "url":"https://eyup.io","sameAs":["https://github.com/eyupio/reet.sh"]},
     {"@type":"WebSite","@id":"https://reet.sh/#site","url":"https://reet.sh/",
      "name":"reet.sh","publisher":{"@id":"https://reet.sh/#org"}},
     {"@type":"SoftwareApplication","name":"reet.sh",
      "applicationCategory":"DeveloperApplication","operatingSystem":"Linux, macOS",
      "url":"https://reet.sh/","description":"Self-hosted control room for AI agents, machines, skills and reusable setups.",
      "offers":{"@type":"Offer","price":"0","priceCurrency":"USD"},
      "isAccessibleForFree":true,"publisher":{"@id":"https://reet.sh/#org"}}
   ]}
   ```
   Do not add `FAQPage` for SERP benefit (Google retired FAQ rich results for all sites on 2026-05-07); use `QAPage` only for genuine user Q&A.
   **Status: done** — `Organization`, `WebSite`, `SoftwareApplication` added to `web/index.html` with `{{ORIGIN}}` substitution. Not yet run through Rich Results Test.

4. **Eliminate soft 404s and add a canonical.**
   *First principle:* an infinite space of URLs returning `200` wastes crawl budget and creates duplicates. *Dependency:* independent. *Falsifier:* `/nonexistent-abc123` still returns 200. *Leading indicator:* "Soft 404" and "Duplicate, Google chose different canonical" drop to zero in Search Console.
   - Return `404` for unknown paths; allowlist `/`, `/welcome`, `/signin`, `/api/*`.
   - Add `<link rel="canonical" href="https://reet.sh/">` to the shell (currently absent).
   - `/welcome` serves the same landing copy as `/` — canonicalize it or make it distinct.
   **Status: partially done** — canonical added (`{{ORIGIN}}/`). Soft 404s and the HTTP→HTTPS redirect are still open.

5. **Build one indexable content surface.**
   *First principle:* AI answers cite pages that answer a question; a single marketing page has almost no surface area. *Dependency:* requires #2 so the content is readable without JS. *Falsifier:* 90 days out, `site:reet.sh` still returns 0. *Leading indicator:* branded and "self-hosted AI agent dashboard" queries begin returning reet.sh.
   Priority pages, each with a real question answered in the first 60 words:
   - "What is a self-hosted AI agent control room?"
   - "How to sync Claude Code / Codex skills across machines"
   - "reet.sh vs cloud agent dashboards" (comparison table, not a landing-page rewrite)
   - Install and requirements reference (static, crawlable, dated)
   **Status: not started** — no new content routes exist yet.

**Also fix:** `http://reet.sh/` returns `200` with no redirect to HTTPS and no HSTS — duplicate scheme, avoidable security gap. `www.reet.sh` does not resolve; either add it with a 301 or keep it unlinked.

---

## 9. Schema Recommendations

| Type | Priority | Why |
|---|---|---|
| `Organization` | High | Binds the brand to `eyup.io` and GitHub; gives AI systems a resolvable entity. |
| `WebSite` | High | Declares the site entity and canonical URL. |
| `SoftwareApplication` | High | Describes the product itself: category, OS, price, free tier. This is the type AI answers need to describe what reet.sh *is*. |
| `Person` (founder/author) | Medium | Only if a named human is willing to be the public expert; anonymous authorship is a weak authority signal. |
| `BreadcrumbList` | Low | Site is too shallow today. |
| `FAQPage` | **Do not add for SERP** | No rich result since 2026-05-07. Visible Q&A copy is still useful. |
| `HowTo` | **Never** | Deprecated Sept 2023. |

Add a visible "Last updated" date and a link to the source/docs for the "no telemetry" claim; undated, unsourced claims are weaker citations (recency is a reported 3x citation factor — SE Ranking, third-party).

---

## 10. Content Reformatting Suggestions

**Hero (first 60 words).** Replace the slogan-first H1 with a definition-first opening that keeps the slogan as a subline:

> **What is reet.sh?**
> reet.sh is a self-hosted control room for AI agents. It connects your laptops and servers into one fleet, uses the agents already installed on each machine, and syncs skills and reusable setup profiles between them. One Go binary, one SQLite file, no telemetry.

**Add a question-shaped H2 set** (matches how people query AI):
- What is a self-hosted AI control room?
- How does reet.sh sync skills between machines?
- Does reet.sh send data to a third party?
- Can I run it on my own server?

Give each a 2–4 sentence, self-contained answer, with the direct answer in the first sentence.

**Add one comparison table** (currently zero tables on the page): reet.sh vs. cloud-hosted agent dashboards across hosting, data ownership, multi-machine sync, and price. Tables are disproportionately quoted.

**Fix the H1 text concatenation.** Rendered H1 reads `One control room.For your AI fleet.` — insert a space or a `<span>` with a space; text extractors and screen readers otherwise see one malformed string.

**Keep `noindex` off `/signin` only if it is useful to crawl; otherwise noindex it** — it currently returns the generic shell with the same title as the homepage and adds no citation value.

---

## Priority Summary

| Priority | Action | Status |
|---|---|---|
| Critical | Real `robots.txt` + `sitemap.xml` (SPA fallback shadowing them today) | Done in repo, pending deploy |
| Critical | Pre-render the landing page for non-JS AI crawlers | Done in repo (static fallback), pending deploy |
| High | Organization + WebSite + SoftwareApplication JSON-LD, canonical tag | Done in repo, pending deploy |
| High | Kill soft 404s (`/nonexistent` currently 200), redirect HTTP→HTTPS | Open |
| High | One content surface answering real questions | Open |
| Medium | Definition-first hero, question H2s, comparison table, update dates | Open |
| Medium | Entity footprint: GitHub README depth, docs, Reddit/YouTube presence | Open |
| Low | `/llms.txt` for non-Google crawlers (explicitly not a Google lever), RSL licensing | Open (intentionally not added) |

---

## Implementation Log (2026-10-24)

**Source base:** fast-forwarded `main` from `6976f2b` to `origin/main` `99c64ba` before editing; `origin/main` matches the deployed title and landing copy. The earlier checkout was 335 commits behind and did not contain the fleet-control-room site.

**Files changed**

| File | Change |
|---|---|
| `web/embed.go` | Serves `/robots.txt` (`text/plain`) and `/sitemap.xml` (`application/xml`) before the SPA fallback; serves `index.html` through `serveIndex` with `{{ORIGIN}}` substitution; strips the static fallback on non-landing routes; `Handler(baseURL)` derives the origin from `REET_PUBLIC_URL` or the request (including `X-Forwarded-Proto`). |
| `web/index.html` | Added `rel=canonical`, `{{ORIGIN}}`-based OG/Twitter URLs, Organization/WebSite/SoftwareApplication JSON-LD, and a semantic static copy of the landing page between `seo-fallback` markers. |
| `web/src/main.ts` | Clears the static fallback and its style block before mounting the SPA. |
| `cmd/reet/main.go` | Passes `publicURL` into `web.Handler`. |
| `web/embed_test.go` | New tests: robots is text not SPA, sitemap is valid XML, origin derivation, fallback present on `/` and `/welcome`, absent on `/signin`, static assets still served. |
| `web/dist/**` | Rebuilt with `npm run build` (embedded assets changed). |

**Verification**

```
go test ./web/...        ok
go test ./...            all packages ok
npm run check            0 errors, 0 warnings
npm run build            built in 3.05s
```

End-to-end against a local binary (`REET_PUBLIC_URL=https://reet.sh`):

| Request | Result |
|---|---|
| `GET /robots.txt` | `200 text/plain`, 494 bytes, `Sitemap: https://reet.sh/sitemap.xml` |
| `GET /sitemap.xml` | `200 application/xml`, valid `<urlset>` with `https://reet.sh/` |
| `GET /` | 8,519 bytes, 314 raw words, canonical + JSON-LD present, landing copy present |
| `GET /signin` | 0 occurrences of the landing copy (stripped) |

**What this does and does not fix**

- Fixes: non-JS AI crawlers now receive a real title, a definition, product sections, an install command, internal links, entity schema and a canonical URL. `robots.txt`/`sitemap.xml` can no longer be shadowed. Self-hosted instances get correct absolute URLs via the request origin.
- Does not fix: soft 404s (`/nonexistent` still returns 200), HTTP→HTTPS redirect/HSTS, thin content (314 words is still a landing page, not a citation surface), missing dates/sources, zero brand/entity footprint, no content routes.
- Deployment is required before any of this affects https://reet.sh. After deploy, verify with `curl -sI https://reet.sh/robots.txt`, `curl -s https://reet.sh/sitemap.xml`, and a raw-HTML fetch (`curl -s https://reet.sh/ | grep "self-hosted workspace"`).

**Maintenance note:** the fallback in `web/index.html` duplicates copy from `web/src/lib/Welcome.svelte`. Change both together; the tests do not compare them.

**Method note:** the numeric heuristics cited (130–170 words, 44% from first 30%, 3x recency, 92% top-10) are third-party measurements, not Google requirements. Google's guidance says you do not need to chunk content for AI, and that optimizing for generative search is still SEO.
