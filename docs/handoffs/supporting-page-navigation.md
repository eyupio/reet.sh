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
| Ticket (one support ticket) | Support, linking back to `/support` | the ticket subject; no row |

## Decisions and spec deltas

- `pageGroup` returns the group even for one page; the row shows for two or more items. A group of one (Help & community without the support desk, Workspace for a non-admin) shows its label and no row. Signed out, the context keeps only the current page, since the others need a sign-in.
- The Skills label "Your skill shelf" replaces "Ey up. Your skill shelf."; Notifications is titled "Notifications" under the Account label.
- A viewer at `/hosting` keeps the label "Skills & distribution", although the page is not in their More list.
- Machines keeps its row in the focused-machine view although the spec gives detail pages none: the Sync link lives there, and `sections` outranks the context row and the `groupHref` suppression. A list's own page links its label back to Lists (a 44px target) and has no row.
- Software is a page action beside the Machines title (the spec's own fallback, taken after the first merge), not an item of the row: one tap at every width and a 44px target. The row holds Inventory & connection and Sync. `SectionNav` still accepts action items; no page passes one now.
- Account's row is seven items so it stays one line: Profile, Agents, Gaffer AI, Security (Password, Single sign-on, Sessions, API tokens, MCP access), Data & activity (Backups, Maintenance, Activity), Machines and Compare & sync. `accountNav` (`web/src/lib/accountNav.ts`) builds it, keeps the original order, leaves a group with one present section as a plain link, and the folded menu names the open dropdown's current child.

## Preserved hooks

`.page-heading`, `class="group eyebrow"` on the label (`navigation.e2e.mjs` reads it), `titleId`, the navigation names Machines, Account and Admin, and the button "Software".

## Known trade-offs

- Below 1000px the row folds, so the Machines links and Account's sections need one tap to open the menu. Account's grouped sections (Password, Sessions, Backups and the rest) sit one click deeper than the old flat row, on desktop as well.
- Server-rendered crawl fallbacks keep "Public community"; they are not the on-page heading.

Evidence is `web/tests/supporting-nav.e2e.mjs`, `web/src/lib/shell/tabs.test.mjs` and `web/src/lib/accountNav.test.mjs`, on a synthetic API only. Deployment is a separate verification and is not established by merging.

## Merge outcome

PR #663 was merged on 2026-11-08 as merge commit `b00422ed7ef8c2f6e2d0e0513d5c628d3f67dda5` (head `f817eb9`, merge method: merge commit). Every pull-request check on that exact head passed, including both `offline` legs; `offline (native)` passed on one re-run after a loopback broken-pipe in the Go fixture, which is not in this change. The Codex review had no findings.

Push runs on `main` for the merge commit: `docs`, `image`, `binary release`, `Gaffer settings browser` and `Gaffer offline acceptance` passed. `terminal browser` did **not** pass and is **not verified here**. Its first attempt failed in `terminal-workspace.e2e.mjs:275`, a wall-clock check on a loaded runner, and `phone-zoom` failed once and then passed on re-run, neither reproducible locally. On attempt 2, `phone-zoom` passed; `phone-dialogs` was cancelled at its 20-minute job limit on the `zoomies-linux-x64` pool (it also ran out of time on attempt 1; on pull requests it runs on `ubuntu-latest`); `terminal-browser` was cancelled at 18:21 UTC, the minute the merge of #662 started a newer `main` run (the workflow cancels in-progress runs per branch). Nothing shows #663 as the cause, but the push-run browser scripts, `supporting-nav.e2e.mjs` among them, have no green run on the merge commit. They passed on the pull-request head.

**Deployment is unverified**: web rollout needs its own check, and no connector or API change is involved.

## Follow-up: the two trade-offs and two flaky checks

Software and Account's row were changed as described under Decisions; `supporting-nav.e2e.mjs`, `navigation.e2e.mjs` (Activity now sits in the Data & activity dropdown) and `accountNav.test.mjs` cover them.

- `terminal-workspace.e2e.mjs`: the 2 s "Synthetic continuation" budget failed at 2675 ms on the `zoomies-linux-x64` pool (526 ms locally on an idle machine). Its window began at the click and included the script's own steps (the click, the resynchronising state, a double frame wait and a row count). It now begins when the fixture releases the host redraw, so it measures the app: 57 ms idle and 64 ms with all four cores busy, where the old window read 2055 ms under the same load. The one-ticket, one-socket assertions that pin a single attachment are unchanged. The budget is still 2 s.
- `gaffer-offline-live.e2e.mjs` (native leg): the fixture's fake worker wrote `progress` on its control link after the link had closed (`write: broken pipe`, run `42492037569` attempt 1). A real connector dials again when the controller ends its link, so the fixture now does too, resends with the new generation, and logs why the link ended. `TestGafferWorkerLinkRedialsAfterItIsLost` fails without this and passes with it. **The reason the controller ended the link that time is not established**: the failure did not reproduce locally in two full runs (both passed, with no redial), and the controller returns from that handler without logging the cause. A repeat will show the reason in the test log.
