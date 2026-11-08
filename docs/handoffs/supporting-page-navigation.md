# One quiet header and a section row on the supporting pages

Design: [spec](../superpowers/specs/2026-11-08-supporting-page-navigation-design.md) · [plan](../superpowers/plans/2026-11-08-supporting-page-navigation.md). Web only; no API, protocol or server change.

## What changed

`PageHeader` is quiet: a muted group label, the menu name as the one `h1`, an optional lede, and a row of sibling pages above the closing hairline (`eyebrow` became `group`; the slogans are gone). `SectionNav.svelte` (a link, a link with children, or an action) replaces `NavigationMenu` and folds into one dropdown below 1000px. `pageGroup(more)` (`web/src/lib/shell/tabs.ts`) reads the More list `App.svelte` already builds and shares the current page's group through context, so a viewer has no "Write a skill" and a non-admin no "Instance administration". Machines, Account and Admin pass their own `sections`, which outrank the context row.

| Page | Group label | Title |
|---|---|---|
| Setups, Lists, Write a skill | Skills & distribution | the menu name |
| Community, Support | Help & community | the menu name |
| Teams & organisations | Workspace | Teams & organisations |
| Instance administration | Instance administration | the open section |
| Machines (list, sync, one machine) | Machines | Machines / Sync / the host name |
| Account, Notifications | Account | the person's name / Notifications |
| Activity (no accounts) | This workspace | Activity |
| Skills | Your skill shelf | Skills |

## Decisions and spec deltas

- `pageGroup` returns the group even for one page; the row shows for two or more items. A group of one (Help & community without the support desk, Workspace for a non-admin) shows its label and no row. Signed out, the context keeps only the current page, since the others need a sign-in.
- The Skills label "Your skill shelf" replaces "Ey up. Your skill shelf."; Notifications is titled "Notifications" under the Account label.
- Machines keeps its row in the focused-machine view although the spec gives detail pages none: Software and Sync live there, and `sections` outranks the context row and the `groupHref` suppression. A list's own page links its label back to Lists (a 44px target) and has no row.

## Preserved hooks

`.page-heading`, `class="group eyebrow"` on the label (`navigation.e2e.mjs` reads it), `titleId`, the navigation names Machines, Account and Admin, and the button "Software".

## Known trade-offs

- Below 1000px the row folds, so Software and the Machines links need one tap to open the menu (it closes before an action runs). The spec's fallback, Software in `actions`, was not taken; it is a product call.
- Account's 11-link row spans the full width under the avatar and title: one line at 1440px, wrapping where the column is narrower.
- Server-rendered crawl fallbacks keep "Public community"; they are not the on-page heading.

Evidence is `web/tests/supporting-nav.e2e.mjs` and `web/src/lib/shell/tabs.test.mjs`, on a synthetic API only. Deployment is a separate verification and is not established by merging.
