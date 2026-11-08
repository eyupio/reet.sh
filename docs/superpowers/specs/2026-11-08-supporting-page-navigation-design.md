# One quiet header and one section row for the supporting pages

Status: design for review. No code is written. Follows the handoffs that left "supporting-page navigation" as separate presentation work (`docs/handoffs/calm-workspace-redesign.md`, "Supporting pages: later slice"; `docs/handoffs/compact-work-items.md`). A private mock of the chosen direction (option C) was reviewed and approved by the user; it shows Lists, Community and Teams at desktop and phone width in both themes.

## Problem

The supporting pages (Lists, Setups, Write a skill, Community, Support, Teams & organisations, Instance administration, Machines, Account, Admin) are reached through the top tabs and the More sheet, but once there they do not say where they sit, and they do not look like one family. Verified in code:

1. **The eyebrow means something different on every page** (`PageHeader.svelte` takes a free-text `eyebrow`): a category ("Agent setups"), a tagline ("Good finds, kept together"), a label ("Public community") or the page name again ("Support", "Teams & organisations").
2. **The title is often a slogan, not the name in the menu**: "Have a nosy." (Community), "A proper toolkit. Together." (Teams & organisations), "Tell us what's not working." (Support), "Your machines, connected." (Machines), "What's happened." (Notifications). Someone who opened Community from More sees a heading that does not say Community.
3. **Three section-navigation patterns.** Account and Admin use `NavigationMenu.svelte` (a row that folds into a dropdown on a phone). Machines has its own `<nav aria-label="Machines">` of `btn quiet` links ("Inventory & connection", "Sync") plus a Software button that opens a dialog. Every other page has no sibling navigation at all, so moving between, say, Setups and Lists means reopening More.
4. **The header is loud.** The title is up to 3rem (`PageHeader.svelte`), the eyebrow is accent-coloured mono, and the header takes 32px of bottom margin, which pushes content down most of all on a phone.

## Decisions (agreed with the user)

- **Goal:** consistent, quieter page chrome (not a regrouping of More, and not new wayfinding between Home and supporting pages).
- **Direction C:** a compact header plus a row of the other pages in the same group.
- **Titles are the menu names.** The slogans go; the description paragraph under each title stays as written. The user approved this rule on four examples (Community, Teams, Support, Machines); Notifications ("What's happened.") turned up while writing this spec and follows the same rule, flagged for review below.
- **Scope:** all signed-in supporting pages, including Machines, Account and Admin, moved onto one shared row component, which retires the three patterns.

## Non-goals

- No change to the top tabs, the More sheet, its three groups, any route or deep link, or who can see which page (writers, support desk, administrators, accounts mode).
- No change to Home, the session workspace or Sessions page, the bell, Jump search or the account menu.
- No new colour or type tokens. No server, API or protocol change. Page-local workflows (software evidence, inventory, setup and sync diagnostics) stay on their pages.
- Server-rendered crawl fallbacks and `llms`-style text for public pages (`web/embed.go`, `web/seo_test.go`, `web/embed_test.go`) keep their own wording ("Public community"); they are not the on-page heading.

## Design

### 1. Components

- **`SectionNav.svelte`** replaces `NavigationMenu.svelte` (the same file may be renamed in place so Account and Admin keep working through the change). It is a row of links with the current page marked, and the same folded `<details>` dropdown below 1000px. Items are one of: a link, a link with `children` (the existing grouped form), or an **action** (a button with an `onclick`), so Machines' Software dialog stays an action rather than becoming a fake tab.
- **`PageHeader.svelte`** gets the quiet layout. `eyebrow` is replaced by `group`: a muted mono label (13px) naming the More group, or the section for a top-level page; when the page is a detail page it is a link back to its parent. `title` is the menu name, about 1.9rem (1.5rem on a phone). `lede` and `actions` stay. A hairline closes the header, and the section row sits inside it, above the hairline. No new tokens: `--muted`, `--line-soft`, `--tint`, `--serif`, `--mono`.

### 2. Where the row comes from

- **More-group pages** (Setups, Lists, Write a skill / Community, Support / Teams & organisations, Instance administration): a pure helper `groupRow(tabs, more)` in `web/src/lib/shell/tabs.ts` takes the `more` list `App.svelte` already builds (each entry carries `group` and `current`) and returns `{ group, items }` for the page in view, or `null`. `App.svelte` exposes it through Svelte context and `PageHeader` reads it, so page components change only their header props. Because `more` is already filtered by permission, a viewer never sees "Write a skill" or "Instance administration". A group with one visible page returns `null` for the row (the label still shows).
- **Detail pages** (a single list, a ticket, a machine focus): the quiet header with `group` linking back to the parent ("Lists", "Support", "Machines") and no row. `More` already marks Lists current for a list and Support current for a ticket, which `groupRow` reuses. Machines is the exception: a focused machine keeps the section row, because the Software action and the Sync link live there and moving a capability must not make it undiscoverable (`sections` outranks the `groupHref` suppression).
- **Machines, Account, Admin:** each passes its own `sections` to `PageHeader`, which takes priority over the context. Machines: "Inventory & connection" and "Sync" as links, Software as an action (shown only where `accounts && canWrite`, as today). Account and Admin: their existing section lists, including the grouped items `NavigationMenu` already supports. Account's existing links to Machines and Sync keep their targets. Account keeps showing the person's name or email as its title and Admin keeps showing the open section.

### 3. Copy

| Page | Group label (was) | Title (was) |
|---|---|---|
| Setups | Skills & distribution ("Agent setups") | Setups (unchanged) |
| Lists | Skills & distribution ("Good finds, kept together") | Lists (unchanged) |
| Write a skill | Skills & distribution ("Write & publish") | Write a skill (unchanged) |
| Community | Help & community ("Public community") | Community ("Have a nosy.") |
| Support | Help & community ("Support") | Support ("Tell us what's not working.") |
| Teams & organisations | Workspace ("Teams & organisations") | Teams & organisations ("A proper toolkit. Together.") |
| Instance administration | Instance administration (unchanged; it already names the section) | the current section's name (unchanged) |
| Machines (list / sync / one machine) | Machines ("Machines" / "Machine") | Machines / Sync / the machine's name ("Your machines, connected." on the list) |
| Account | Account ("Your account") | the person's name or email (unchanged) |
| Activity | Your account / This workspace (unchanged; it names the scope) | Activity (unchanged) |
| Skills | Your skill shelf ("Ey up. Your skill shelf.") | Skills (unchanged) |
| Notifications | Account ("Notifications") | Notifications ("What's happened.") |

The rule is that the label names a group or section and the title is the menu name, except where the title is already a specific thing (Account shows the person, Admin shows the open section). The Skills page is the one place an eyebrow carried dialect ("Ey up."); its label becomes plain, for the same reason as the slogans (found while planning; confirm when reviewing). The plan confirms the label for any other page that uses `PageHeader` today. Descriptions are unchanged.

### 4. Behaviour and accessibility

- One `h1` per page, the `PageHeader` title. The row is a `<nav>` labelled with the group (or section), the current page has `aria-current="page"`, and on a phone the folded dropdown keeps its summary showing the current page.
- Targets stay 44px, focus is visible, Escape and an outside click close the dropdown, and following a link does not move focus.
- The row and header must not overflow horizontally at 320px; long group or page names wrap.
- The browser tab title is derived in `App.svelte` (`pageTitle`) and is not changed by this work.

### 5. Voice

`docs/yorkshire-copy.md` keeps navigation in standard text and puts warmth in companion lines. The four slogans are hard-coded strings outside `copy.ts`, so removing them aligns titles with "navigation stays standard". If the product wants some voice back on these pages, the description is where the policy puts it; that is a separate copy decision and is **not** part of this change.

## Testing

- **Unit** (`web/src/lib/shell/tabs.test.mjs`): `groupRow` for each group; a viewer without write access gets no "Write a skill"; no support desk gets no "Support"; a non-admin gets one Workspace page and so no row; a list detail marks Lists current; a ticket marks Support current; an unknown path returns `null`.
- **Browser** (a new `web/tests/supporting-nav.e2e.mjs`, listed in `.github/workflows/terminal-browser.yml` in the same change): for each supporting page, one `h1` equal to the menu name, the group label, the row with the current page marked, the phone dropdown opening and closing, no horizontal overflow at 320px, deep links unchanged, and a read-only viewer seeing the permitted pages only. Account, Admin and Machines each keep their section links working, including the Software action.
- **Existing scripts** that quote old titles must change to the new ones, never be loosened: `start-agent.e2e.mjs`, `first-run.e2e.mjs`, `loading.e2e.mjs`, `control.e2e.mjs` and `sessions.e2e.mjs` assert "Your machines, connected."; the plan greps the rest (Community, Teams, Support and Setups headings, eyebrows).
- Both themes, desktop and phone, checked by screenshot, as the mock was.

## Risks and open points

- **Old titles are quoted in tests and docs** (listed above, plus `docs/superpowers/specs/2026-11-02-directory-pages-refresh.md`); each is updated with the change.
- **Public pages share `PageHeader`** (`PublicList`, `Community` when signed out). They get the same quiet header; the row appears only when the context supplies siblings, so signed-out views show the label only. The plan confirms this per page.
- **Notifications and Activity are not in `more`**, so they get a label and no row. The Notifications title change ("What's happened." to "Notifications") is the one slogan removal not named in the approved examples; confirm it when reviewing this spec.
- **Machines' Software button** is an action, not a page; if keeping it inside the row proves awkward, it moves into the header's `actions` with no loss of function.
- **Preserved contracts:** permissions, deep links, the More sheet and top tabs, same-process Chat/Terminal switching and the attention contract are untouched. Deployment is a separate verification and is not established by merging.
