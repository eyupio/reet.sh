# Supporting page header and section row Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every signed-in supporting page one quiet header (a group label, the menu name as the title) and one shared section row, replacing the three current section-navigation patterns.

**Architecture:** A pure helper `pageGroup(more)` derives the current page's group and its permitted siblings from the `more` list `App.svelte` already builds. `App.svelte` shares it through Svelte context; `PageHeader` reads it and renders a new `SectionNav` (the generalised `NavigationMenu`). Machines, Account and Admin pass their own sections, which take priority. Web only: no server, API or protocol change.

**Tech Stack:** Svelte 5 + TypeScript (`web/src`), node unit tests (`*.test.mjs`), Playwright browser scripts (`web/tests/*.e2e.mjs`, listed in `.github/workflows/terminal-browser.yml`).

**Spec:** `docs/superpowers/specs/2026-11-08-supporting-page-navigation-design.md` (approved by the user).

## Global Constraints

- The top tabs, the More sheet and its three groups, every route and deep link, and who can see which page are unchanged. `more` is already permission-filtered; never build a sibling list from anything else.
- Titles are the menu names; the five slogan titles go ("Have a nosy.", "A proper toolkit. Together.", "Tell us what's not working.", "Your machines, connected.", "What's happened."). Descriptions (`lede`) are unchanged.
- No new colour or type tokens: use `--muted`, `--line-soft`, `--tint`, `--serif`, `--mono`. Title about 1.9rem (1.5rem on a phone); label 13px mono muted; 44px targets; visible focus; no horizontal overflow at 320px.
- Existing hooks other scripts depend on stay: `.page-heading` on the header element (it must keep the content's left edge and width, `loading-width.e2e.mjs`), `titleId` (`account-title`, `admin-title`), and the nav accessible names `Machines`, `Account` and `Admin` with their link names and hrefs. Software stays a button named exactly "Software" (`fleet-software.e2e.mjs`).
- One `h1` per page. The browser tab title (`pageTitle` in `App.svelte`) is not changed.
- Any new or changed browser script is listed in `.github/workflows/terminal-browser.yml` in the same change. Never loosen or delete an existing assertion; change quoted titles to the new ones.
- Spec deltas decided while planning (surface them in the PR description): (1) the helper is `pageGroup(more)` (the spec's `tabs` argument is unused) and returns the group even when only one page is visible, so the label still shows; the row shows only with two or more items. (2) The Skills page's label "Ey up. Your skill shelf." becomes "Your skill shelf", the one dialect eyebrow among the pages in scope.

## Review Focus

- **A hidden sibling must never show.** A viewer without write access has no "Write a skill"; no support desk means no "Support"; a non-admin has no "Instance administration". Tested in Task 1 and in the browser script.
- **A group with one visible page** (Workspace for a non-admin) shows the label only: no empty row, no empty dropdown.
- **Phone width:** the row folds into the dropdown showing the current page, and long names ("Teams & organisations", "Instance administration") wrap without overflow at 320px.
- **Detail pages** (a list, a ticket's list page, one machine) mark the right sibling current, keep one `h1`, and do not lose the list's visibility label (it moves into `meta`).
- **Machines, Account and Admin** keep their accessible nav names, hrefs, `aria-current`, grouped children and the Software dialog.

## Files

- Create: `web/src/lib/SectionNav.svelte` (renamed from `NavigationMenu.svelte` with `git mv`), `web/tests/supporting-nav.e2e.mjs`, `docs/handoffs/supporting-page-navigation.md`.
- Modify: `web/src/lib/shell/tabs.ts` + `tabs.test.mjs`, `web/src/lib/PageHeader.svelte`, `web/src/App.svelte`, `web/src/lib/{SetupProfiles,SharedLists,Hosting,Community,Support,Spaces,Notifications,ActivityPage,ListView,PublicList,NotFound,Machines,Account,Admin}.svelte`, `.github/workflows/terminal-browser.yml`, the scripts that quote "Your machines, connected." (`start-agent`, `first-run`, `loading`, `control`, `sessions`), `docs/handoffs/calm-workspace-redesign.md`.

---

### Task 1: `pageGroup` helper

**Files:** Modify `web/src/lib/shell/tabs.ts`, `web/src/lib/shell/tabs.test.mjs`

**Interfaces:**
- Consumes: `Tab = { href; label; current; group? }` (already in `tabs.ts`).
- Produces: `export type PageGroup = { group: string; items: { label: string; href: string; current: boolean }[] }` and `export function pageGroup(more: Tab[]): PageGroup | null`. It finds the entry with `current: true` that has a `group`; returns its group and every `more` entry with that group, in order; returns `null` when no such entry exists.

- [ ] **Step 1: Write failing tests in `tabs.test.mjs`** (compile `tabs.ts` as the file already does). Cases, with a `more` fixture shaped like `App.svelte`'s: Setups/Lists/Write a skill (Skills & distribution), Community/Support (Help & community), Teams/Instance administration (Workspace): current Lists gives group "Skills & distribution" and 3 items with only Lists current; a writer-less `more` (no Write a skill) gives 2 items; a `more` without Support gives Community alone (1 item); a non-admin Workspace gives 1 item; current Support (a ticket page) marks Support; no current entry, or a current entry with no group, returns `null`.
- [ ] **Step 2: Run `node web/src/lib/shell/tabs.test.mjs`.** Expected: FAIL (`pageGroup` is not exported).
- [ ] **Step 3: Implement `pageGroup`** in `tabs.ts` as specified above.
- [ ] **Step 4: Re-run the test.** Expected: PASS, with the pre-existing `jumpMatches` tests still green.
- [ ] **Step 5: Commit** "Derive a page's group and siblings from the More list".

### Task 2: `SectionNav` replaces `NavigationMenu`

**Files:** `git mv web/src/lib/NavigationMenu.svelte web/src/lib/SectionNav.svelte`; modify `Account.svelte`, `Admin.svelte` (imports and the one usage each).

**Interfaces:**
- Produces (exported from a `<script module lang="ts">` block so `PageHeader` can import the type): `export type SectionItem = { label: string; href: string; current: boolean; children?: { label: string; href: string; current: boolean }[] } | { label: string; action: () => void }`; props `{ label: string; items: SectionItem[] }`. Link items render `<a>` with `aria-current`; `children` items keep the existing grouped `<details>`; an action item renders a `<button type="button">` styled like a link. `<nav aria-label={label}>` and the folded `<details>` below 1000px behave exactly as `NavigationMenu` does today, including Escape and outside-click closing.

- [ ] **Step 1: Run the safety net first.** `node web/tests/navigation.e2e.mjs` and `node web/tests/account.e2e.mjs` against a local preview (commands under Verification). Expected: PASS before any change.
- [ ] **Step 2: `git mv` and extend the component** with the action item and the union type; keep every existing class and behaviour. Update the two imports and usages in Account and Admin only (same `label`, same item lists).
- [ ] **Step 3: Re-run the two scripts and `npm --prefix web run check`.** Expected: PASS, 0 errors / 0 warnings.
- [ ] **Step 4: Commit** "Make the section menu a shared component that can hold an action".

### Task 3: Quiet `PageHeader` and the context plumbing

**Files:** Create `web/src/lib/shell/pageNav.ts`; modify `PageHeader.svelte`, `App.svelte`.

**Interfaces:**
- Produces in `pageNav.ts`: `export const PAGE_GROUP = Symbol('page-group')` and `export type PageGroupContext = { readonly group: PageGroup | null }`.
- `PageHeader` props become `{ group?: string; groupHref?: string; title: string; lede?: string; titleId?: string; sections?: SectionItem[]; sectionsLabel?: string; meta?: Snippet; actions?: Snippet; eyebrow?: string }`. `eyebrow` is a temporary alias for `group` so every task builds on its own; Task 5 removes it. The label is `group ?? context.group?.group ?? ''`; when `groupHref` is set the label is a link. The row is `sections` when given (label `sectionsLabel ?? label`), otherwise the context's items when there are two or more, rendered with `SectionNav`.
- `App.svelte` calls `setContext(PAGE_GROUP, { get group() { return pageGroup(more) } })` at the top level of its script, so every routed page sees the current value.

- [ ] **Step 1: Start `web/tests/supporting-nav.e2e.mjs`** with its first section only (Lists, at 1440px, using `openWorkspace`): label "Skills & distribution", one `h1` "Lists", a row with Setups, Lists (`aria-current=page`), Write a skill. List it in `terminal-browser.yml` now. Run it against the unchanged build. Expected: FAIL.
- [ ] **Step 2: Implement the layout.** Keep `<header class="page-heading">` as the flex row (label, title, lede, meta; actions at the right); title `clamp`ed to about 1.9rem and 1.5rem at the existing 1000px breakpoint; the row sits under it inside one wrapper closed by a `--line-soft` hairline. Update the `eyebrow` callers that live in `App.svelte`: the Skills page gets `group="Your skill shelf"`; the three state headers ("Connection problem", "Your account", "Instance administration" access errors) keep their text as `group`.
- [ ] **Step 3: Run `npm --prefix web run check`, `navigation.e2e.mjs` and the new script.** Expected: check clean; navigation PASS; the Lists section still FAILS until Task 4 (it needs the title change), which is the intended red.
- [ ] **Step 4: Commit** "Add the quiet page header and share the page group through context".

### Task 4: More-group pages, Notifications, Activity, detail pages

**Files:** `SetupProfiles`, `SharedLists`, `Hosting`, `Community`, `Support`, `Spaces`, `Notifications`, `ActivityPage`, `ListView`, `PublicList`, `NotFound` (all `.svelte` in `web/src/lib/`).

- [ ] **Step 1: Titles and labels per the spec's table.** More-group pages drop `eyebrow` (the context supplies the group) and set `title` to the menu name: Setups, Lists, Write a skill, Community, Support, Teams & organisations. Notifications: `group="Account"`, title "Notifications". Activity keeps its scope label (`accounts ? 'Your account' : 'This workspace'`) as `group`. `NotFound` and `PublicList` pass their existing eyebrow text as `group`.
- [ ] **Step 2: Detail pages.** `ListView`: `group="Lists"` with `groupHref="/shared"`, and the old `visibilityLabel(detail.visibility)` text moves into a `meta` snippet. Keep `h1` single and the list name as title.
- [ ] **Step 3: Run `npm --prefix web run check`, then `supporting-nav.e2e.mjs` (Lists section now passes), `community.e2e.mjs`, `navigation.e2e.mjs`, `first-skills.e2e.mjs`.** Expected: PASS.
- [ ] **Step 4: Commit** "Use the menu name and the group label on the supporting pages".

### Task 5: Machines, Account, Admin onto `PageHeader` sections

**Files:** `Machines.svelte`, `Account.svelte`, `Admin.svelte`.

- [ ] **Step 1: Machines.** Remove the hand-written `<nav aria-label="Machines">`; pass `sections` with "Inventory & connection" (`/machines`, current when not compare), "Sync" (`/sync`, current when compare) and, when `accounts && canWrite`, an action "Software" that sets `softwareOpen = true`; `sectionsLabel="Machines"`. `group="Machines"` for the list and sync views; for a focused machine `group="Machines"` with `groupHref="/machines"` and the host as the title. Titles: list "Machines", compare "Sync".
- [ ] **Step 2: Account and Admin.** Remove their `NavigationMenu` usage; pass the same item lists as `sections` with `sectionsLabel` "Account" / "Admin". Account keeps title `who.name || who.email` with `group="Account"`; Admin keeps `sectionTitle` with `group="Instance administration"`. Keep `titleId`.
- [ ] **Step 3: Update the quoted titles.** "Your machines, connected." becomes "Machines" in `start-agent.e2e.mjs` (two places), `first-run.e2e.mjs`, `loading.e2e.mjs` (the regex), `control.e2e.mjs` and `sessions.e2e.mjs` (two places).
- [ ] **Step 4: Remove the temporary `eyebrow` alias from `PageHeader`.** `npm --prefix web run check` then proves no caller still uses it.
- [ ] **Step 5: Run `npm --prefix web run check`, then `navigation`, `account`, `admin-users`, `fleet-software`, `loading-width`, `loading`, `first-run`, `control`, `sessions`, `start-agent`.** Expected: PASS.
- [ ] **Step 6: Commit** "Put Machines, Account and Admin on the shared section row".

### Task 6: New browser script, CI listing, handoff

**Files:** Create `web/tests/supporting-nav.e2e.mjs`, `docs/handoffs/supporting-page-navigation.md`; modify `.github/workflows/terminal-browser.yml`, `docs/handoffs/calm-workspace-redesign.md`.

**Interfaces:** (the script file and its CI listing already exist from Task 3.) Consumes `openWorkspace(browser, { width, status, routes })` and `owner` from `web/tests/helpers/workspace.mjs`; `status` overrides `supportDesk` and `user.role` (writer, viewer, admin).

- [ ] **Step 1: Extend `supporting-nav.e2e.mjs`** (started in Task 3), one section per behaviour, run on a synthetic API (nothing started on a machine): for Setups, Lists, Write a skill, Community, Support, Teams, Instance administration, Machines (list, sync), Account, Admin, Notifications and Activity: exactly one `h1` whose text equals the menu name (or the person's name / open section where the spec says so), the group label, and the row with the right `aria-current`; at 390px the dropdown opens and closes and shows the current page; at 320px `scrollWidth <= clientWidth`; a viewer has no "Write a skill" link in the Skills & distribution row; `supportDesk: false` removes Support from Help & community (label only, no row); a non-admin Workspace shows the label and no row; deep links (`/community`, `/shared`, `/spaces`) load unchanged; Machines' Software button opens its dialog. Prove the first assertion non-vacuous against a build without the header change (throwaway extract, stated in the report).
- [ ] **Step 2: Write the handoff** (about 30 lines): what changed, the table of labels and titles, the spec deltas, the preserved hooks, what is unverified (deployment). Add one pointer line to the "Supporting pages: later slice" section of `calm-workspace-redesign.md`.
- [ ] **Step 3: Run the whole set** (Verification) plus `npm --prefix web test`. Expected: all PASS.
- [ ] **Step 4: Commit** "Test and document the supporting page navigation".

## Verification (every task)

```bash
npm --prefix web run check
npm --prefix web run build
(npm --prefix web run preview -- --host 127.0.0.1 --port 4173 > /tmp/preview.log 2>&1 &)
PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/usr/bin/google-chrome REET_UI_URL=http://127.0.0.1:4173 node web/tests/<script>.e2e.mjs
```

Pass means exit 0 and the script's `passed` line. Stop the preview afterwards. A script that passes on the old build and fails on the new one is a regression to fix, not to edit away.
