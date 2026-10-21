# reet.sh — product audit and improvement plan

Date: 2026-10-21. Audited at commit `6e96850` by reading every source file, running the Go and Svelte test suites (all green), running the server locally and exercising the API and the UI (desktop, phone width, dark mode, first run with a token), and fetching live skills.sh pages to see what metadata is actually available. GitHub has no open issues or pull requests. There are no TODOs in the code and no roadmap beyond the original design spec and plan under `docs/superpowers/`.

Three asks from the product owner arrived during the audit and are treated as first-class requirements: **RBAC and multi-user**, **categories imported from skills.sh with user overrides**, **tracking where a skill is in use across environments**, and **the skills.sh-style install block (Command / Prompt, copy)**. Decisions taken with the owner during the audit: multi-user means **personal shelves plus shared lists**, signed in with **local accounts and per-user API tokens**.

## What the product is

reet.sh is a self-hosted, single-binary "shelf" for the agent skills listed on skills.sh. skills.sh is a public directory and install leaderboard with no accounts, so it cannot remember what you liked, what you meant to try, or what you actually installed. reet.sh is that missing personal layer: paste a link or click a bookmarklet to put a skill on the shelf, move it through **On the shelf → To test → In use**, mark favourites, keep notes, and run `reet scan` on a machine to import what is really installed in Claude Code, Cursor, Codex, OpenCode and the `skills` CLI. The core job to be done: *"When I come across a skill I might want, capture it in two seconds, and later know which skills I actually use so I can set up a new machine or recommend them to someone."* Today it is built for exactly one person on a homelab, with one shared access token.

## Feature inventory

| Area | Feature | Serves | Completeness | Notes |
| --- | --- | --- | --- | --- |
| Capture | Add by skills.sh URL or `owner/repo/skill` slug (`POST /api/skills`, `internal/ref`) | user | complete | Trailing slash, query, hash, `/security/socket` suffix all normalise. Non-skills.sh hosts rejected. |
| Capture | Bookmarklets Like / To test / In use (`/quick`, `web/src/lib/bookmarklet.ts`) | user | complete | Token is embedded in the `javascript:` URL and sent as `?t=`. |
| Capture | Paste-list import (`POST /api/import` text) | user | complete | Marks everything "in use"; keeps existing flags and notes. |
| Capture | `reet scan` CLI (`internal/scan`) | user | partial | Finds skills in 5 home folders, 2 project folders and 2 lock files. No record of *which* folder or machine found it. No prune. |
| Shelf | Three stages derived from `bookmarked`/`used` flags (`web/src/lib/shelf.ts`) | user | complete | Stage is derived, never stored; clean. |
| Shelf | Like (private favourite) | user | complete | |
| Shelf | Notes per skill | user | complete | Plain text, pre-wrap. |
| Shelf | Search (name, id, description, notes) | user | complete | Case-insensitive LIKE; no field or tag filters. |
| Shelf | Sort: Recent / Name / Installs | user | **rough** | "Installs" is always 0 without `SKILLS_SH_TOKEN`, which the README says cannot be obtained. Sort option is dead for every real user. |
| Shelf | Stage and Liked filter chips with counts | user | partial | "All" shows the current result count; "Liked" shows the global count. Inconsistent while searching. |
| Shelf | Remove skill | user | partial | `confirm()` dialog, no undo; notes are lost. A scanned skill comes back on the next scan. |
| Shelf | Link an unlinked local skill to its skills.sh id (`/api/skill/resolve`) | user | complete | Moves flags and notes; merges notes if target is empty. |
| Metadata | Background fetch of description from the public page (`internal/meta`) | user | **partial** | Only `description` is read from the page. Name stays the slug, installs stay 0, though the page carries both plus a weekly install sparkline. |
| Metadata | Official API path with `SKILLS_SH_TOKEN` | user | stub | Requires a Vercel OIDC token nobody can mint; effectively unreachable. |
| Metadata | Manual Refresh per card | user | partial | No scheduled refresh; "Refreshed" toast even when nothing changed. |
| Data out | Export JSON (`GET /api/export`), Restore JSON (`POST /api/restore`) | both | complete | Restore is insert-or-replace with no confirmation and no preview. |
| Data out | Any other view of your data (Markdown list, CSV, history, trends) | user | missing | |
| Auth | Single shared `REET_TOKEN` for UI, API and bookmarklets | both | partial | No identity, no sign-out, no rotation from the UI (only `init-env.sh --rotate`). First run shows a browser `prompt()` dialog. |
| Auth | CSRF guard: `Sec-Fetch-Site` and JSON content-type checks | both | complete | Good. |
| Admin | Container image, compose, `init-env.sh`, healthcheck | admin | complete | Polished operator onboarding. |
| Admin | Any in-app admin surface (users, stats, audit, logs, version) | admin | missing | Server logs one line at start-up and nothing else. |
| Notifications | Any reminder or digest | user | missing | Nothing nudges "To test" items that have sat for weeks. |
| Analytics | Any usage tracking of key features | both | missing | No way to know whether bookmarklets, scan or paste are used. |
| Brand | Logo set, OG image, PWA manifest, dark mode, reduced motion, skip link, ARIA | user | complete | Unusually polished for a day-old product. |

## Competitor comparison

Direct competitor: the skills.sh directory itself (what it lacks is the whole reason reet.sh exists) and the `skills` CLI (which already knows what is installed). Adjacent best-in-class: self-hosted bookmark managers, because reet.sh is structurally a bookmark manager with a fixed domain. Details for the adjacent products come from web sources checked during the audit and are marked "Needs confirmation" where only third-party listings were found.

| Capability | reet.sh | skills.sh directory | `skills` CLI | linkding | Linkwarden | Karakeep (ex-Hoarder) | Raindrop.io |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Like / favourite | Yes | No | No | Yes (unread/shared flags) | Yes | Yes | Yes |
| Save for later with a pipeline (shelf → test → use) | **Yes, unique** | No | No | No | No | No | No |
| Knows what is actually installed | Yes via `reet scan` | No | Yes (`skills list`) | n/a | n/a | n/a | n/a |
| Knows *where* it is installed (machine, agent, project) | No | No | Per machine only | n/a | n/a | n/a | n/a |
| Categories / tags | No | 8 curated Topics (Agent workflows, Databases, Design & UI, Marketing, Mobile, Next.js, React, Testing) | No | Tags | Tags and collections | Tags, lists, AI tagging | Tags and nested collections |
| Install counts and trend | 0 unless unobtainable token | Yes, with weekly sparkline | No | n/a | n/a | n/a | n/a |
| One-click install command / agent prompt | No | Yes (Command / Prompt toggle, copy) | Yes | n/a | n/a | n/a | n/a |
| Update checking | No | No | Yes (`skills check`, `skills update`) | n/a | n/a | n/a | n/a |
| Multi-user | No | No | No | Yes, separate per user (Needs confirmation: SSO via OIDC/proxy) | Yes, share with users or guests | Yes, shared lists | Yes, collaborators per collection |
| Roles / permissions | No | No | No | Admin + user | Per-link sharing | Per-list | Per-collection view/edit |
| Public or read-only sharing | No | Everything is public | No | Shared bookmarks page | Public collections | Public lists (Needs confirmation) | Public collections |
| Bookmarklet / extension | Bookmarklet | n/a | n/a | Extension + bookmarklet | Extension | Extension | Extension |
| Import from other tools | Paste list, scan | n/a | n/a | Netscape HTML, Pocket | HTML, Wallabag | HTML, Pocket | Many |
| Export | JSON | No | n/a | HTML, JSON | HTML, JSON, CSV | JSON | HTML, CSV |
| Bulk actions | No | n/a | n/a | Yes | Yes | Yes | Yes |
| Admin UI / usage visibility | No | n/a | n/a | Django admin | Admin settings | Admin settings | n/a |
| Single static binary, SQLite, no CGO | **Yes** | n/a | n/a | Python + Docker | Node + Postgres | Node + Postgres/SQLite | SaaS |

**What competitors do well that fits reet.sh:** tags with filtering (every bookmark manager), per-collection sharing with roles (Raindrop, Linkwarden), bulk edit (linkding), and skills.sh's own install block and install trend. **What not to copy:** full-page archiving, AI auto-tagging, browser extensions for every browser, and nested folder hierarchies. reet.sh's domain is narrow, its items are canonical ids not arbitrary URLs, and a flat tag list plus the three-stage pipeline is enough. **Differentiators to double down on:** the stage pipeline (nobody else has "to test"), truth about what is really installed and where, and a one-file, one-binary deployment that a homelab user trusts.

## Personas

- **Priya, new solo user.** Full-stack dev who just found skills.sh, installed `reet` via compose yesterday. Wants to stop losing skills in browser tabs. Uses Claude Code on a laptop and Cursor on a desktop.
- **Sam, returning power user.** Six weeks in, 70 skills on the shelf, three machines, four agents. Reaches for reet.sh when setting up a new machine and when a colleague asks "what do you use for X?".
- **Alex, team lead and instance admin.** Runs one reet.sh for a six-person team on the company box. Wants a shared "team approved" list, wants to see who actually uses what, and wants to onboard a new hire without a wiki page that goes stale.
- **Jordan, operator.** Self-hosts twenty containers. Cares about upgrades, backups, one-line health answers and never opening a database shell.

## Verdict

**Strongest:** the capture and triage loop is genuinely good. Add-by-paste normalises every URL variant, the bookmarklet round-trips in a popup that closes itself, the three-stage shelf is a clearer mental model than any tag soup, re-import never clobbers flags or notes, cross-site write protection is in place, and the UI is accessible, responsive and branded to a standard most hobby projects never reach. The test suite covers the real edge cases named in the spec.

**Weakest:** the product stops the moment a skill is on the shelf. There is no grouping, no sense of where a skill lives, no install affordance, install counts are permanently zero, and the only data view is a JSON download. Operationally it is invisible: one log line, no version, no stats, no audit. And it is single-user by construction, with a token that leaks into bookmark URLs and a first-run experience that is a browser `prompt()`.

**Biggest opportunity:** turn the shelf from a personal notepad into the system of record for "which skills we use, where", for a person first and then a team. Three things deliver most of that: environments on each skill (from the scanner), tags seeded from skills.sh topics, and accounts with shared lists. Everything else on the list below either removes friction on the way there or makes the result visible.

---

## Tier 1 — Quick wins (under a day each)

### FEAT-001 — Read name, install count and weekly trend from the skills.sh page
- **Type:** Feature improvement · **Serves:** user (all personas) · **Pass:** Product review · **Where:** `internal/meta/meta.go` `fromPage`, `internal/store/store.go` `SetMeta`, `web/src/lib/SkillCard.svelte`
- **Problem:** Every card shows the slug as the name and no install count, and "Sort by installs" sorts a column of zeros. The README tells users the API token "cannot be generated", so this is the permanent state for everyone.
- **Evidence:** Live run: after adding `anthropics/skills/pptx` the row has `name: "pptx"`, `installs: 0`. The fetched page contains JSON-LD `"interactionStatistic": {"interactionType": ".../InstallAction", "userInteractionCount": 239629}`, `og:title` = `pptx — anthropics/skills`, and an SVG with `aria-label="Weekly installs: 4,298, 4,371, …"`.
- **Proposal:** In `fromPage`, parse the `application/ld+json` `SoftwareApplication` block for `name`, `description` and `interactionStatistic.userInteractionCount`; fall back to the `og:title` prefix before " — " for the name; parse the sparkline `aria-label` into an eight-week series. Store `installs`, `installs_week` (last value) and `trend` (JSON array) via `SetMeta`. Card shows "240K installs · 4.2K this week"; keep the current HTML meta path as the final fallback. Remove or demote the API path in docs.
- **Acceptance:** adding a GitHub-sourced skill shows the human name and a non-zero install count within three seconds; "Sort by installs" orders correctly; a page without JSON-LD still yields the description; existing `TestMetaAppliedInBackground` extended with a fixture containing JSON-LD.
- **Value:** makes a visible dead feature work and gives every card a credibility signal. Confidence high.
- **Effort & risk:** S. Scraping can break when skills.sh changes markup; keep the existing graceful fallback. Weekly fetch pressure is unchanged.
- **Dependencies:** none. Unlocks FEAT-014.

### FEAT-002 — Say "already on the shelf" instead of "added"
- **Type:** UX tweak · **Serves:** user (Priya, Sam) · **Pass:** Walkthrough · **Where:** `internal/api/api.go` `add`, `web/src/App.svelte` `add()`
- **Problem:** Adding a skill that is already on the shelf returns the existing row unchanged and the UI says "Reet, added X to the shelf." The user cannot tell whether they just duplicated something or where it went.
- **Evidence:** Live run: second `POST /api/skills` for `vercel-labs/skills/find-skills` returned the same `createdAt`; UI copy is unconditional.
- **Proposal:** `add` returns `201` on insert and `200` on existing (store already knows: compare `createdAt == updatedAt` after upsert, or return a `created` boolean). UI shows "Already on the shelf, in To test" and scrolls to and briefly highlights the card (`data-skill` is already there). Same for `/quick`: "Already liked".
- **Acceptance:** duplicate add shows the distinct message and scrolls to the card; first add still shows the success toast; `/quick` on an already-liked skill says so.
- **Value:** removes a small but repeated confusion; bookmarklet users hit it constantly. Confidence high.
- **Effort & risk:** S. None.
- **Dependencies:** none.

### FEAT-003 — A sign-in screen and a sign-out, instead of `window.prompt`
- **Type:** UX tweak · **Serves:** both (Priya first run, Jordan support) · **Pass:** Walkthrough · **Where:** `web/src/lib/api.ts` `req()`, `web/src/App.svelte`
- **Problem:** With a token set, the first visit shows the full empty UI, a red "missing or wrong token" banner and a native `prompt()`. Dismiss it and you are stuck on an error page with no way to try again except reload. There is no way to sign out, switch token or see that you are signed in.
- **Evidence:** First-run screenshot during the audit; `req()` only prompts once per request and stores the token in `localStorage` for ever.
- **Proposal:** On a 401, render a small sign-in card in place of the shelf (logo, one password-type field, "Where do I find this?" pointing at `grep REET_TOKEN .env`, paste-friendly). Keep the token in `localStorage` as today. Add a "Sign out" link in the footer that clears it. This is the shell that FEAT-019 later swaps for email and password.
- **Acceptance:** fresh browser with a token-protected instance shows the sign-in card and no error banner; wrong token shows an inline error and keeps the field; sign out returns to the card; token-less instances never show it.
- **Value:** first impression and support load. Confidence high.
- **Effort & risk:** S. None.
- **Dependencies:** none; replaced by FEAT-019.

### FEAT-004 — Install block on every card: Command / Prompt toggle with copy
- **Type:** Core feature gap · **Serves:** user (Sam, Priya) · **Pass:** Competitor comparison, owner request · **Where:** `web/src/lib/SkillCard.svelte`, new `web/src/lib/install.ts`
- **Problem:** The shelf is where you decide to use a skill, then you have to open skills.sh to get the install line. "To test → Start using" is a click, but actually installing is a tab switch and a copy.
- **Evidence:** skills.sh skill pages show an "INSTALLATION" block with a Command / Prompt toggle and a copy button; reet.sh has nothing. Exact strings from the live page: command `npx skills add https://github.com/anthropics/skills --skill pptx`; prompt ``Run `npx skills use "https://github.com/anthropics/skills" --skill "pptx"` and follow the generated skill instructions now. Read its complete output, redirecting it to a temporary file first if necessary. Resolve relative paths from the supporting-files directory it provides.``
- **Proposal:** A collapsed "Install" row under the actions on every GitHub-sourced card (`sourceType === 'github'`): mono text, segmented control Command | Prompt, copy button that flips to "Copied" for 1.5 s, remembering the last choice in `localStorage`. Generate both strings from `source` and `slug` as above. For `well-known` sources show the same with `https://<domain>` as the source (Needs confirmation against the skills.sh page for a well-known skill); hide for `local`. Optional `-a <agent>` suffix later, once FEAT-012 knows the agent. Track a copy as an event (FEAT-016).
- **Acceptance:** both strings match skills.sh byte for byte for a GitHub skill; copy works with keyboard; local skills show no block; the chosen tab persists across reloads.
- **Value:** closes the loop from "decide" to "install" in the product; the single most-requested companion to "In use". Confidence high.
- **Effort & risk:** S. Strings drift if the `skills` CLI changes its flags; keep them in one file.
- **Dependencies:** none.

### FEAT-005 — Undo for Remove, and a confirmation with counts for Restore
- **Type:** UX tweak · **Serves:** user (all) · **Pass:** Walkthrough · **Where:** `web/src/App.svelte` `remove()`, `web/src/lib/Tools.svelte` `restore()`
- **Problem:** Remove shows a native `confirm()` and then the notes are gone for good. Restore silently overwrites every matching row from whatever file was picked.
- **Evidence:** `store.Delete` is a hard delete; `Restore` is `INSERT OR REPLACE` in one transaction with no preview.
- **Proposal:** Remove: drop the `confirm()`, remove immediately, show a toast "Removed pptx · Undo" for 8 s; Undo calls `POST /api/restore` with the single cached skill object (already supported) which restores flags, notes and timestamps. Restore: parse the file client-side, show "Restore 42 skills? 17 already on the shelf will be overwritten" with Cancel. Add a "Replace everything" checkbox only if the user asks for it later; default stays merge.
- **Acceptance:** Undo brings the card back with its notes and stage; the toast is announced to screen readers; Restore shows the count dialog and can be cancelled.
- **Value:** fewer destructive accidents, no support "I lost my notes". Confidence high.
- **Effort & risk:** S. None.
- **Dependencies:** none.

### FEAT-006 — Operator visibility: request log, version, DB-backed health
- **Type:** Admin tooling · **Serves:** admin (Jordan) · **Pass:** Product review · **Where:** `cmd/reet/main.go`, `internal/api/api.go`, `web/src/lib/Footer.svelte`
- **Problem:** The server logs exactly one line, ever. There is no way to see what version is running, whether skills.sh is reachable, or whether a bookmarklet request arrived. `/healthz` returns "ok" even if the database file is unwritable.
- **Evidence:** Local run log: `reet listening on :8099` and nothing after twelve writes. No `-ldflags -X` version; the footer has no version.
- **Proposal:** One structured log line per non-GET request (method, path, status, duration, source: `ui`/`quick`/`scan`, never the token). Bake `version` and commit into the binary with `-ldflags -X` in the Dockerfile; expose `GET /api/version`; show it in the footer; print it at start-up. `/healthz` runs `SELECT 1` and reports `{"ok":true,"version":…,"skills":n}`. Log the result of each background metadata fetch at debug level, gated by `REET_LOG=debug`.
- **Acceptance:** `docker logs reet` shows one line per write; footer shows the version; a read-only data dir makes `/healthz` return 503.
- **Value:** halves the time of any support exchange. Confidence high.
- **Effort & risk:** S. Make sure tokens never appear in logs (strip `t` from logged query strings).
- **Dependencies:** none; FEAT-015 builds on it.

### FEAT-007 — First-run and empty states that lead somewhere
- **Type:** UX tweak · **Serves:** user (Priya) · **Pass:** Walkthrough · **Where:** `web/src/App.svelte` empty branch, `web/src/lib/Tools.svelte`
- **Problem:** An empty shelf says "Paste a skills.sh link above, or import what you have installed", but the import tools are collapsed at the bottom of the page under a link nobody reads. Each empty stage says "Nowt…" with no next action.
- **Evidence:** Screenshot of a fresh instance: three empty sections, tools panel closed.
- **Proposal:** When `counts.all === 0`, replace the three sections with a single getting-started card: three numbered steps (paste a link, drag the bookmarklets shown right there, run `reet scan` with the exact command pre-filled with this origin), and a "Browse skills.sh topics" link. Open the tools panel automatically in that state. Per-stage empties get one verb: "To test" empty → "Nowt waiting. Save something from the shelf below."
- **Acceptance:** fresh instance shows the getting-started card with bookmarklets and the scan command containing the real origin; it disappears after the first add; each stage empty state names the action that fills it.
- **Value:** activation; the first five minutes decide whether Priya comes back. Confidence medium-high.
- **Effort & risk:** S. None.
- **Dependencies:** none.

### FEAT-008 — Show age on cards, especially in "To test"
- **Type:** Usage & engagement · **Serves:** user (Sam) · **Pass:** Walkthrough · **Where:** `web/src/lib/SkillCard.svelte`
- **Problem:** `createdAt` and `updatedAt` are stored and never shown. Sam has 20 skills in "To test" and no idea which have been sitting there for two months.
- **Evidence:** Card markup renders no dates; store already bumps `updated_at` on stage moves.
- **Proposal:** In the card meta line show a relative time: "On the shelf 3 days", "To test for 6 weeks", "In use since 12 Aug" (from `updatedAt`, which changes on stage moves). Title attribute gives the absolute local time. Items in "To test" older than 30 days get a quiet "still to test?" label. Sort "Recent" is unchanged.
- **Acceptance:** every card shows a relative age; times render in the viewer's local timezone; the stale label appears only in To test after 30 days.
- **Value:** gives the To test stage a reason to be revisited; cheap retention lever. Confidence medium.
- **Effort & risk:** S. None.
- **Dependencies:** none.

### FEAT-009 — Consistent filter-chip counts
- **Type:** UX tweak · **Serves:** user · **Pass:** Product review · **Where:** `web/src/App.svelte` toolbar
- **Problem:** While searching, "All" and the stage chips show counts of the current results, but "♥ Liked" shows the global `counts.liked`. The numbers disagree.
- **Evidence:** `All<small>{skills.length}</small>` vs `Liked<small>{counts.liked}</small>`.
- **Proposal:** Either count all chips from the current result set (compute `liked` client-side from `skills`) or show global counts everywhere with the result count on the "N skills" line. Recommend result-set counts, since the chips act as filters on what is shown.
- **Acceptance:** with a search term, every chip count equals the number of matching items in that chip.
- **Value:** small trust fix. Confidence high.
- **Effort & risk:** S. None.
- **Dependencies:** none; revisit when tag chips arrive (FEAT-011).

## Tier 2 — Incremental improvements (one to five days each)

### FEAT-010 — Environments: record where each skill is installed
- **Type:** Core feature gap · **Serves:** user (Sam, Priya), later admin (Alex) · **Pass:** Walkthrough, owner request · **Where:** `internal/scan/scan.go`, `internal/api/api.go` `importSkills`, `internal/store`, `web/src/lib/SkillCard.svelte`, new filter chips
- **Problem:** "In use" is a single bit. Sam has a laptop, a desktop and a CI box, each with Claude Code, Cursor and Codex. The scanner collapses all of that into one flag, so reet.sh cannot answer "is pptx on the desktop?", "what do I need to install on the new machine?", or "what is on my laptop that I have stopped using everywhere else?".
- **Evidence:** `scan.Collect` returns `(name, source)` pairs with the folder discarded; `importEntry` sets `used=true` and nothing else; `Found` has no host or agent. The owner asked for exactly this.
- **Proposal:**
  - Data: `environments(id, name, kind ∈ {machine, project}, agent ∈ {claude, cursor, codex, opencode, agents, other}, host, path, first_seen, last_seen)` and `skill_envs(skill_id, env_id, installed_at, last_seen, source_path)`. An environment is one (host, agent[, project]) pair, e.g. "sam-mbp · Claude Code" or "sam-mbp · Cursor · acme-web".
  - Scanner: keep the folder each skill was found in; derive the agent from the folder, the host from `os.Hostname()` overridable by `--env NAME`; send `{host, agent, project, path}` per item. Add `--prune` so skills no longer found in that environment lose that link (never their flags or notes). Print a diff at the end: "3 new, 1 gone on sam-mbp · Cursor".
  - Semantics: `used` becomes derived: true if any `skill_envs` row exists **or** the manual flag is set (keep the manual flag so paste-import and the bookmarklet still work).
  - UI: chips on the card ("mbp · Claude", "desktop · Cursor"), an "Environments" filter row that appears once any exist, a per-environment view with "Copy install commands for everything missing here" (uses FEAT-004 strings), and a "Only on one machine" smart filter.
  - Edge cases: renamed host → `--env` keeps continuity; an environment unseen for 90 days is greyed, not deleted; export/restore include environments.
- **Acceptance:** two scans from different hostnames yield two environments on the same skill; `--prune` removes a link and leaves the skill and notes intact; the environment filter shows only skills on that environment; export round-trips environments; `used` stays true for a paste-imported skill with no environment.
- **Value:** turns "In use" from a guess into a fact and makes new-machine setup a copy-paste. The strongest differentiator available. Confidence high.
- **Effort & risk:** M. Scanner protocol changes; keep the old `{name, source}` shape accepted for older binaries. Scan output must not include home paths beyond the skills folder (privacy on shared instances).
- **Dependencies:** FEAT-004 for the "install everything missing" action. Required by FEAT-021.

### FEAT-011 — Tags, seeded from skills.sh topics, with user overrides
- **Type:** Core feature gap · **Serves:** user (all), admin (shared tag list) · **Pass:** Competitor comparison, owner request · **Where:** `internal/store`, `internal/meta` (new `topics.go`), `internal/api`, `web/src/App.svelte` toolbar, `web/src/lib/SkillCard.svelte`
- **Problem:** Past about thirty skills the three stages are not enough: "the design ones", "the testing ones", "the ones for client work". Search by description is a poor substitute because descriptions are trigger-phrase lists, not categories.
- **Evidence:** No tag or category field anywhere. skills.sh exposes exactly eight curated Topics at `/topic/<slug>` (agent-workflows, databases, design, marketing, mobile, nextjs, react, testing); each topic page lists between 5 and 21 skills (live: design 16, testing 5, marketing 21). Individual skill pages do not name their topic; the mapping only exists on the topic pages. Coverage is therefore partial: most skills will be in no topic.
- **Proposal:**
  - Data: `tags(id, name, colour?)`, `skill_tags(skill_id, tag_id, source ∈ {user, topic}, hidden)`. Flat tags, no hierarchy, case-insensitive unique on name.
  - Seed: a background job on start-up and every 24 h fetches `/topic` and the eight topic pages (nine small requests, same 8 s timeout and size cap as today), builds `topic → [skill ids]`, and attaches `source=topic` tags to matching skills on the shelf and on every future add. Names come from the page ("Design & UI"). If a skill drops out of a topic, the tag is removed unless the user has also added it.
  - Overrides: user removes a topic tag → row kept with `hidden=1` so the seed does not re-add it; user adds any tag by typing, with autocomplete over existing tags; rename and merge tags from a small "Manage tags" dialog in the tools panel.
  - UI: tag chips on cards, a tag row in the toolbar (appears when any tag exists, with counts, multi-select AND), `tag:` syntax in search, and the publisher (`anthropics/skills`) clickable as an implicit filter without being a tag.
  - Under FEAT-019, tags are per shelf; shared lists may carry their own.
- **Acceptance:** adding `anthropics/skills/frontend-design` auto-tags it "Design & UI" within a day of the seed run (or immediately if the map is warm); removing that tag survives the next seed; a user tag filters the shelf; `tag:testing` works in search; export and restore round-trip tags; the seed failing leaves the app fully usable.
- **Value:** the main organisation primitive the product is missing; every bookmark competitor has it. Confidence high.
- **Effort & risk:** M. Nine extra outbound fetches per day to skills.sh (flag in docs; honour a `REET_TOPICS=off` switch). Topic markup can change; the job is best-effort like metadata.
- **Dependencies:** none. FEAT-016 should record tag usage.

### FEAT-012 — Bulk select and actions
- **Type:** Feature improvement · **Serves:** user (Sam), admin (Alex) · **Pass:** Walkthrough, competitor comparison · **Where:** `web/src/App.svelte`, `web/src/lib/SkillCard.svelte`, `internal/api` (batch endpoint)
- **Problem:** A single `reet scan` of a working machine drops thirty-plus skills into "In use" at once. Tagging, moving or removing them is one card at a time.
- **Evidence:** Every action is per card; `PATCH /api/skill` takes one id.
- **Proposal:** A "Select" toggle in the toolbar turns on checkboxes; a floating bar offers Tag, Move to stage, Like, Remove (with the FEAT-005 undo, batched). `PATCH /api/skills` accepts `{ids: [...], patch}`; `POST /api/skills/tags` accepts `{ids, add: [], remove: []}`. Shift-click range select.
- **Acceptance:** tagging 20 selected skills is one request; undo restores all removed items; keyboard-only selection works.
- **Value:** required for the shelf to stay tidy past the first scan. Confidence medium-high.
- **Effort & risk:** M. None notable.
- **Dependencies:** FEAT-011 for the Tag action.

### FEAT-013 — Ignore list for the scanner
- **Type:** Feature improvement · **Serves:** user (Sam) · **Pass:** Walkthrough · **Where:** `internal/store`, `internal/api` `importEntry`, `web/src/App.svelte` remove flow
- **Problem:** Remove a scanned skill (a half-finished local experiment, a vendor skill you never use) and the next scan puts it straight back as "In use".
- **Evidence:** `importEntry` upserts unconditionally; nothing records a deletion.
- **Proposal:** `ignored(id, reason, at)` table. Removing a skill that has environments (or `sourceType=local`) offers "Remove and ignore in future scans". Scan and paste import skip ignored ids and report them ("skipped 2 ignored"). "Manage ignored" list in the tools panel with un-ignore.
- **Acceptance:** an ignored skill does not reappear after a scan; the scan output names it; un-ignore lets the next scan re-add it.
- **Value:** stops a recurring annoyance that makes "In use" lie. Confidence high.
- **Effort & risk:** S-M. None.
- **Dependencies:** FEAT-010 for the environment-aware prompt (works without it).

### FEAT-014 — Scheduled metadata refresh and install trend on the card
- **Type:** Feature improvement · **Serves:** user · **Pass:** Product review · **Where:** `internal/api` worker, `internal/store`, `web/src/lib/SkillCard.svelte`
- **Problem:** Metadata is fetched once at add time and only refreshed by hand. Install counts go stale immediately and the eight-week trend skills.sh shows is never seen.
- **Evidence:** `enqueue` only on add, resolve and import; `refresh` is manual; the page sparkline is parsed by nothing.
- **Proposal:** Background job refreshes the oldest-fetched 50 skills every 6 h (`fetched_at` column), one request every two seconds so a big shelf never bursts. Card shows a tiny eight-bar sparkline and "↑ 12% this week" from the parsed series. Manual Refresh stays and now reports "No change" when nothing differs.
- **Acceptance:** `fetched_at` advances for every skill within a week; sparkline renders only when a series exists; the job backs off for an hour after three consecutive failures.
- **Value:** keeps the shelf alive without user effort; trend is a reason to open the app. Confidence medium.
- **Effort & risk:** M. Polite traffic to skills.sh must stay low; document it and offer `REET_REFRESH=off`.
- **Dependencies:** FEAT-001.

### FEAT-015 — Admin and settings page
- **Type:** Admin tooling · **Serves:** admin (Jordan, Alex) · **Pass:** Product review · **Where:** new `web/src/lib/Admin.svelte`, `GET /api/admin/status`, `cmd/reet/main.go`
- **Problem:** Everything an operator wants to know needs `docker logs`, `sqlite3` or `.env`. Rotating the token means a shell and a container restart; nobody can see whether the scanner has ever run.
- **Evidence:** Only `init-env.sh --rotate` and `--set` exist; no settings surface in the app.
- **Proposal:** A `/admin` route (owner/admin only once FEAT-019 lands; token holders before that) showing: version and update available (compare against GHCR tags, optional), DB path and size, counts per stage and per environment, last scan per environment with the scanner version, skills.sh reachability and last successful fetch, the topic seed status, the last 100 audit rows from FEAT-016, and switches for the refresh and topic jobs. Settings that are safe to change at runtime (job toggles, instance name) live in a `settings` table; env-only settings are shown read-only with their source.
- **Acceptance:** an admin can answer "which version, how many skills, when did sam-mbp last scan, is skills.sh reachable" without leaving the browser; non-admins get 403.
- **Value:** support burden and operator confidence. Confidence high.
- **Effort & risk:** M. Do not expose tokens or env values that are secrets.
- **Dependencies:** FEAT-006, FEAT-016; gated by FEAT-019 roles when present.

### FEAT-016 — Minimal local usage events and audit log
- **Type:** Usage & engagement · **Serves:** both · **Pass:** Product review · **Where:** `internal/store` (new `events` table), every write handler in `internal/api`
- **Problem:** There is no way to tell whether people use the bookmarklets, the scanner or paste; whether "To test" items ever graduate; or who changed what on a shared instance. Without this, none of the recommendations here can be judged after shipping.
- **Evidence:** No analytics, no audit rows, no counters anywhere.
- **Proposal:** `events(id, at, actor, source ∈ {ui, quick, scan, import, api}, action, skill_id, detail JSON)`. Record add, stage move, like, note edit, tag change, remove, scan run (with counts), export, restore, install-copy (from FEAT-004), sign-in. Everything stays in the SQLite file; nothing leaves the instance. Surface: "Recently" strip at the top of the shelf (last 10 actions), per-skill history in an expandable "History" line, admin page counts (FEAT-015), and three derived numbers on the admin page: weekly adds, To test → In use conversion, scans per environment. Retention setting, default 365 days.
- **Acceptance:** every write produces one event with the actor; the recently strip updates after each action; events are included in export and can be excluded with a checkbox; no event is sent anywhere off-box.
- **Value:** makes every later product decision measurable, and gives the shared instance an audit trail. Confidence high.
- **Effort & risk:** M. Privacy: events contain who did what on a team instance; show this in the docs and keep the table owner-exportable and deletable. Small DB growth.
- **Dependencies:** none. Feeds FEAT-015, FEAT-021.

### FEAT-017 — "Copy as Markdown" and shelf views you can hand to someone
- **Type:** Feature improvement · **Serves:** user (Sam), admin (Alex onboarding) · **Pass:** Walkthrough · **Where:** `web/src/lib/Tools.svelte`, `GET /api/export?format=md`
- **Problem:** The only way to show a colleague "the skills I use" is a JSON dump. Sam currently retypes a list into Slack.
- **Evidence:** `export` only emits JSON; no sharing of any kind.
- **Proposal:** Export menu: JSON (as now), Markdown (one section per stage or per tag, each line `- [name](skills.sh url) — description` plus the install command in a fenced block), CSV. Respect the current filters so "In use, tag:testing" exports exactly what is on screen. Later, shared lists (FEAT-020) replace most of this with a link.
- **Acceptance:** Markdown export of the current filter pastes into GitHub cleanly; CSV opens in a spreadsheet with one row per skill including tags and environments.
- **Value:** cheap word-of-mouth lever: the exported list advertises the product. Confidence medium.
- **Effort & risk:** S-M. None.
- **Dependencies:** FEAT-004 strings; FEAT-010 and FEAT-011 columns when present.

## Tier 3 — Larger bets

The escalation rule is met: the owner asked for multi-user and RBAC, the competitor table shows every adjacent product has them, and the Alex persona cannot use the product at all today.

### FEAT-018 — Accounts, roles and per-user API tokens
- **Type:** New feature · **Serves:** both (Alex, Jordan; every user signs in) · **Pass:** Owner request, competitor comparison · **Where:** new `internal/auth`, `internal/store` (users, sessions, tokens, per-user shelf rows), `internal/api` middleware replacing `auth()`, new `web/src/lib/SignIn.svelte`, `Account.svelte`, `Admin.svelte` users tab, `scan.go` token handling, docs
- **Problem:** One shared token means one shelf, no identity, no revocation per device, and a token that sits in every bookmarklet URL, browser history and proxy log. A team cannot use it; a solo user cannot safely share a bookmarklet.
- **Evidence:** `api.auth` compares a single string; `bookmarklet.ts` embeds it in `javascript:` URLs; `/quick` accepts it as `?t=`.
- **Proposal (decided with the owner: local accounts plus per-user API tokens; personal shelves plus shared lists):**
  - **Roles** (one per user, instance-wide):

    | Capability | Owner | Admin | Member | Viewer |
    | --- | --- | --- | --- | --- |
    | Own shelf: add, stage, like, notes, tags, environments, scan, bookmarklets | ✔ | ✔ | ✔ | ✖ |
    | Own API tokens (create, name, revoke) | ✔ | ✔ | ✔ | ✖ |
    | Export own shelf | ✔ | ✔ | ✔ | ✖ |
    | Create shared lists (FEAT-019) | ✔ | ✔ | ✔ | ✖ |
    | Read shared lists they are given | ✔ | ✔ | ✔ | ✔ |
    | Invite users, disable, reset password, revoke anyone's tokens | ✔ | ✔ | ✖ | ✖ |
    | See admin page, audit log, team rollup | ✔ | ✔ | ✖ | ✖ |
    | Promote or demote admins, transfer ownership | ✔ | ✖ | ✖ | ✖ |
    | Delete any user's data, full instance export, instance settings | ✔ | ✖ | ✖ | ✖ |

  - **Data:** `users(id, email, name, role, password_hash, created_at, disabled_at)`, `sessions(id, user_id, expires_at, ua)`, `api_tokens(id, user_id, name, hash, last_used_at, revoked_at)`, `invites(token_hash, email, role, expires_at)`. The `skills` table becomes the shared catalogue (identity and metadata only). Per-person state moves to `shelf(user_id, skill_id, liked, bookmarked, used, notes, created_at, updated_at)`; tags, environments, ignores and events gain `user_id`. A skill with no shelf rows and no list membership is garbage-collected weekly.
  - **Auth mechanics:** argon2id password hashing, session cookie `HttpOnly; Secure (when TLS); SameSite=Lax`, keep the existing `Sec-Fetch-Site` and JSON content-type guards, rate-limit sign-in (5 per minute per IP and per account), tokens shown once and stored hashed, `Authorization: Bearer` or `X-Reet-Token` for API tokens, and **`/quick` uses the session cookie** (the popup is same-origin), so bookmarklets carry no secret at all. Password reset is admin-driven (no email dependency); a self-service email reset is optional later via `REET_SMTP_*`.
  - **Migration and solo mode:** on first start after upgrade, if users are empty: a setup page creates the owner; all existing shelf rows migrate to that owner; `REET_TOKEN` keeps working only as a bootstrap credential for the setup page and for `reet scan` until the owner creates a real token, then is ignored with a log warning. A solo homelab user therefore sees one setup page once and nothing else changes. Keep "no auth at all" possible with `REET_AUTH=none` for trusted LANs, mapping everything to a single implicit owner.
  - **UI:** sign-in page (replaces FEAT-003), account menu (name, role, tokens, sign out), admin "Users" tab (invite link, role, disable, last active), bookmarklets regenerated without the token.
  - **Scanner:** `reet scan --token` takes a personal token; the scan runs as that user; `scan` prints which user and environment it reported to.
- **Acceptance:** two members see different shelves for the same skill id; a viewer gets 403 on every write; a revoked token stops a scan within one request; a bookmarklet URL contains no token and still works while signed in; upgrading an existing instance preserves every flag and note under the owner; `go test -race` covers role checks on every handler.
- **Value:** unlocks teams, fixes the token leak, and gives every later feature an actor. Confidence high that it is needed; medium on adoption until FEAT-019 ships.
- **Effort & risk:** L (two to three weeks). Security-sensitive: password storage, session fixation, rate limiting, migration correctness. Adds ongoing cost: account support, password resets. Breaking change to the API shape; version the API (`/api/v2`) and keep v1 read endpoints for one release.
- **Dependencies:** FEAT-006 (logging), FEAT-016 (audit). Supersedes FEAT-003.

### FEAT-019 — Shared lists with per-list roles
- **Type:** New feature · **Serves:** both (Alex, Sam, new hires) · **Pass:** Owner request, competitor comparison (Raindrop collaborators, Karakeep shared lists) · **Where:** `internal/store` (lists), `internal/api` (`/api/lists*`), new `web/src/lib/Lists.svelte`, list view route
- **Problem:** A team's real question is "what do *we* use?", and a new hire's is "what should I install?". Personal shelves cannot answer either.
- **Evidence:** Decision with the owner: personal shelves plus shared lists. No list concept exists today.
- **Proposal:**
  - Data: `lists(id, owner_id, name, description, visibility ∈ {private, members, link})`, `list_members(list_id, user_id, role ∈ {editor, viewer})`, `list_skills(list_id, skill_id, note, added_by, position)`.
  - Behaviour: any member creates a list; editors add from their shelf or by paste; each entry can carry a list-level note ("use this for client decks"); `visibility=members` makes it readable by every signed-in user, `link` creates a read-only URL with a random slug for people without accounts (off by default, owner can disable instance-wide). Lists appear as chips in the toolbar next to stages; a skill card shows which lists it is in. "Add all to my shelf as To test" on a list is the new-hire action; "Copy install commands" (FEAT-004) on a list is the new-machine action. Team "In use" rollup: on a list, show how many members have each skill in use (from FEAT-010 environments), so the list earns a "7 of 9 use this" badge.
  - Admin: admins can see and edit every list; owner can transfer a list.
- **Acceptance:** an editor adds a skill with a note and a viewer sees it but cannot change it; a link-visible list renders without a session and exposes no personal notes; "Add all to my shelf" creates To test rows without touching existing ones; the rollup badge counts distinct users.
- **Value:** the team use case and the strongest recommend-to-a-colleague moment the product can have. Confidence medium-high.
- **Effort & risk:** L (about two weeks). Public links are a data-exposure surface: no personal notes, no environments, no emails on them.
- **Dependencies:** FEAT-018; FEAT-010 for the rollup; FEAT-004 for install commands.

### FEAT-020 — Team rollup: who uses what, where
- **Type:** Admin tooling · **Serves:** admin (Alex) · **Pass:** Walkthrough · **Where:** `Admin.svelte` team tab, `GET /api/admin/rollup`
- **Problem:** Alex wants "which skills are in use on more than half the team", "who has nothing installed", "which approved skill is nobody using", "what is installed that is on nobody's list".
- **Evidence:** Nothing today; needs FEAT-010 and FEAT-018 to exist.
- **Proposal:** A table: skill × members (with environment chips), sortable by adoption, filters by list membership and tag, CSV export (FEAT-017). Includes a "drift" view: skills in use by a member but not in any shared list, and list skills nobody has installed.
- **Acceptance:** the rollup matches the sum of environment links; CSV export matches the table; viewers and members cannot open it.
- **Value:** the admin's weekly reason to open reet.sh. Confidence medium.
- **Effort & risk:** M once dependencies exist. Privacy: shows per-person usage; document it, and let the owner disable the per-person breakdown (show counts only).
- **Dependencies:** FEAT-010, FEAT-016, FEAT-018, FEAT-019.

---

## Items considered and left out

- Browser extension: the bookmarklet already works on every browser; an extension is maintenance with no new capability until FEAT-018 cookies make it unnecessary anyway.
- Full-text archiving of SKILL.md: skills.sh and GitHub are the source of truth; cache nothing you cannot keep fresh.
- Pagination: `List` loads everything, but a shelf of a few thousand rows with four `LIKE`s on SQLite is still milliseconds. Revisit after FEAT-019 if shared instances grow.
- Dark mode, keyboard shortcuts, PWA: already done or not evidenced.

## Top 10 by value relative to effort, in ship order

1. **FEAT-001** page metadata: name, installs, trend (S, fixes a dead feature for everyone)
2. **FEAT-004** install block with Command / Prompt and copy (S, closes decide → install)
3. **FEAT-003** sign-in screen and sign-out (S, first impression)
4. **FEAT-002** "already on the shelf" (S)
5. **FEAT-005** undo remove, confirm restore (S)
6. **FEAT-006** logging, version, real healthcheck (S, operator trust)
7. **FEAT-010** environments from the scanner (M, the differentiator)
8. **FEAT-011** tags seeded from skills.sh topics (M, the organisation primitive)
9. **FEAT-016** local events and audit (M, makes everything after measurable)
10. **FEAT-018** accounts, roles, per-user tokens (L, unlocks the team)

### Suggested small releases

- **v1.1 "Make it true"** (one week): FEAT-001, FEAT-002, FEAT-003, FEAT-004, FEAT-005, FEAT-006, FEAT-007, FEAT-008, FEAT-009. No data model change beyond three metadata columns. Ship as one minor release.
- **v1.2 "Where and what"** (two weeks): FEAT-010 environments, FEAT-011 tags, FEAT-013 ignore list, FEAT-016 events. Scanner gets `--env` and `--prune`; old scanners keep working.
- **v1.3 "Tidy and share"** (one to two weeks): FEAT-012 bulk actions, FEAT-014 scheduled refresh, FEAT-017 Markdown/CSV export, FEAT-015 admin page.
- **v2.0 "Together"** (four to six weeks): FEAT-018 accounts and RBAC, FEAT-019 shared lists, FEAT-020 team rollup. Setup page migrates existing data to the owner; API moves to `/api/v2`.
