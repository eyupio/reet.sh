# UI Audit — Machines page

**Scope:** `/machines` (Inventory & connection), plus the routes reached from it (`?machine=`, `#add-machine`, `#connect-machine`). The Sync view (`/machines?view=compare`, `/sync`) was inspected only where the Machines page links into it.
**Method:** Source review (`web/src/lib/Machines.svelte`, `MachineFleet.svelte`, `EnvironmentSync.svelte`, `MachineSetup.svelte`, `TerminalReadiness.svelte`, `AddMachineDialog.svelte`, `MachineSettings.svelte`, `syncOverview.ts`) plus a local build of the app rendered at **1440×812, 800×812, 375×812 and 320×812**, in dark and light themes, with the repo's fixture API (3 machines: one needing sync, one up to date with a failed update, one paused/offline).
**Conversion goal:** this is a self-hosted product with no pricing, trial or paywall in the code. The conversion event is **the first connected machine that produces inventory**. There is no paywall to audit; the failure modes below are activation friction, not payment friction.

---

## 1. Critical

### C1. Two onboarding systems are stacked on top of each other
- **Pass:** First-time user
- **Where:** `web/src/lib/Machines.svelte:48` renders `TerminalReadiness` (a 4-step "Get your first terminal running" checklist); `web/src/lib/EnvironmentSync.svelte:254` renders `MachineSetup` directly beneath it, auto-expanded when there are zero machines (`EnvironmentSync.svelte:129`). Empty state at `/machines`, both 1440px and 375px.
- **Problem:** A new user is given two competing tutorials at once: "Before your first terminal" (server terminal → connect machine → machine consent → session) and "Machine setup" (install → authenticate → connect terminal → inventory & sync). Both are numbered 1–4, both contain the same connect and terminal instructions, and both auto-open on first run. The page is asking the user to choose a curriculum before they have seen the product. On mobile the empty state is ~3,000px tall and contains no actual machine. This is the single biggest activation blocker.
- **Fix:** Ship one onboarding flow. Keep the `MachineSetup` wizard as the only numbered flow; delete the standalone `TerminalReadiness` card from the page and move its terminal/consent steps into `MachineSetup` step 3 (where terminal connection already lives). If the readiness checklist must survive, show it only after a machine exists and only for users who have opened Sessions, with an explicit "Dismiss" that persists.

### C2. Three differently-labelled CTAs for connecting a machine, doing two different things
- **Pass:** First-time user / Designer
- **Where:** `web/src/lib/MachineFleet.svelte:320` ("Add a machine" → `AddMachineDialog`; "Connect a machine" → `onadd` = step-by-step panel), `web/src/lib/EnvironmentSync.svelte:231` ("Add a machine"), `web/src/lib/Machines.svelte:47` ("Setups →"), `MachineSetup.svelte:309` ("Set up a machine"), `TerminalReadiness.svelte:47` ("Connect your first machine"). Desktop and mobile.
- **Problem:** "Add a machine" opens the good one-block guided dialog. "Connect a machine" — the more natural label — instead scrolls to a four-step technical wizard. "Set up a machine" is that wizard's collapsed title. Users cannot predict which control starts the two-minute path and which starts the ten-minute path, so they pick the wrong one and bounce. On mobile the three controls wrap into three rows, with the primary Connect action orphaned on its own row beneath an unstyled button (C3).
- **Fix:** One primary action: **Connect a machine** → `AddMachineDialog`. Move the wizard behind the dialog's existing "Prefer the step-by-step setup?" link (`AddMachineDialog.svelte:268`) and delete the duplicate fleet-level button. Rename the accordion to "Advanced: step-by-step machine setup" or make it a link inside the dialog. Remove "Setups →" from this page nav (Setups is already in the global nav).

### C3. The first control in the primary toolbar is an unstyled browser-default button
- **Pass:** Designer
- **Where:** `web/src/lib/MachineFleet.svelte:320` — `<button type="button" onclick={() => importSource = …}>Private skill imports</button>` has no `.btn` class. Rendered in the "Connected workspace" toolbar, 1440px and 375px.
- **Problem:** Computed style at 1440px is `background: rgb(107,107,107)`, `border: 2px solid white`, `padding: 1px 6px` — the Chromium default button — next to two `1px solid var(--line-strong)`, `9px 14px` `.btn`s. In dark mode it reads as a disabled/grey slab; in light mode as a white box. It is the first thing in the toolbar and it looks broken. Worse, its label collides with the per-scope "Private skills" button (`MachineFleet.svelte:307`), which opens a different dialog ("Private skills on {host}" vs "Bring a machine skill into reet").
- **Fix:** Add `class="btn"` and make it visually secondary to Connect (or move it into an overflow "More" menu). Rename it to **Import a machine skill into reet** so it is unambiguous against per-scope "Private skills". Never place it before the Connect action.

### C4. The machines themselves are below the fold at every target width
- **Pass:** Designer / First-time user
- **Where:** `Machines.svelte:45` (`ProjectsBar`), `:46` (`PageHeader` h1), `:47` (page nav), `EnvironmentSync.svelte:224–235` (second "Machines & sync / Inventory" header), `MachineFleet.svelte:313–321` (third "Connected workspace / N machines, one clear view" header), `EnvironmentSync.svelte:254` (`MachineSetup`). Measured: first `article.machine` top = **814px at 1440×812** and **1023px at 375×812**.
- **Problem:** The page has an H1, a second H2, an H3 inside the setup card, and a third H2 before the first machine. Add the unrelated Projects rail, the "Refresh status" line and the setup card, and the product's entire value — the machine cards — sits below the viewport on desktop and more than a screen down on mobile. The header copy repeats the same idea three times ("Your machines, connected.", "Inventory", "3 machines, one clear view."), which is marketing chrome, not hierarchy.
- **Fix:** Reduce to one heading. Remove `ProjectsBar` from `/machines` (it is a Home/Sessions concept; it appears even when the user has no machines). Merge the `EnvironmentSync` header and `MachineFleet` header into the `PageHeader` actions (`Connect a machine`, `Refresh status`), delete the "N machines, one clear view" block, and put the machine grid above `MachineSetup` whenever `machines.length > 0`. Target: first card within the first 500px at 1440px and 650px at 375px.

### C5. Status colour contradicts status text
- **Pass:** Designer / First-time user
- **Where:** `web/src/lib/MachineFleet.svelte:377` — `<span class="live"><span aria-hidden="true">●</span> {agentState(enrolled)}</span>`, with `.live { color: var(--good) }` at `:523`. The paused/offline `pi` card, 1440px and 375px.
- **Problem:** `agentState` returns "Sync paused" or "Sync agent offline", but the dot and label are always green (`--good`). The card simultaneously shows a red "Sync paused" band and a green "● Sync paused" meta line. A user scanning colour rather than reading will believe a paused/offline machine is healthy. In a fleet view, colour is the primary scanning channel; this actively undermines it.
- **Fix:** Derive the class from state: green only for online, `--warn` for paused, `--muted`/`--bad` for offline. Render the dot as a shared `<StatusDot state>` component so the card header, meta row, agent list and sync band cannot disagree.

---

## 2. High impact

### H1. Mobile page nav wraps into three ragged rows
- **Pass:** Designer
- **Where:** `Machines.svelte:47` (`nav[aria-label="Machines"]`), 375px and 320px.
- **Problem:** At 375px the nav renders "Inventory & connection" on row 1, "Sync  Sessions" on row 2, and "Setups →" alone on row 3. It consumes ~110px and looks like a broken toolbar rather than navigation.
- **Fix:** At `max-width: 560px`, use `display:grid; grid-template-columns:repeat(2,minmax(0,1fr));` for the four items (or a horizontal scroller with a fade mask). Remove "Setups →" from this nav entirely.

### H2. Long machine names break mid-word at display size
- **Pass:** Designer
- **Where:** `MachineFleet.svelte:515` `h3 { font: 500 1.35rem/1.15 var(--serif); overflow-wrap: anywhere; }`, rendered at 375px.
- **Problem:** A realistic FQDN (`build.server.eu.west.2a.very.long.hostname.without.breaks`) wraps into five 21.6px serif lines and squeezes the "Inventory current" badge into the top-right corner. The card identity looks broken; the machine name is hard to scan across a grid. Desktops hit the same wrap at two lines.
- **Fix:** Clamp the card title to two lines (`-webkit-line-clamp:2; display:-webkit-box; overflow:hidden`) and put the full name in `title`/an expandable heading; drop to 1.1rem on cards below 560px. Move the status badge to its own row under the name on mobile instead of competing for width.

### H3. Destructive actions are 11.8px underlined links with 18px tap targets, styled like benign actions
- **Pass:** Designer / First-time user
- **Where:** `MachineFleet.svelte:432–438` ("Copy rescan command", "Revoke live access", "Forget {scope}") and `.link` at `:579–580` (`font-size: .74rem`), inside the expanded agent list, 1440px and 375px.
- **Problem:** "Revoke live access" and "Forget…" are irreversible-ish operations presented in the same tiny underlined style as "Copy rescan command", with a measured 18px row height and 11.84px text — well under a 44px touch target and below normal mobile reading size. They are also buried at the bottom of a disclosure, so they are hard to find when wanted and easy to hit by accident on a phone.
- **Fix:** Keep only "Copy rescan command" in the agent list. Move Revoke and Forget into the existing Settings dialog's Access section (`MachineSettings.svelte:104–122`), where they already live, and style them as `.btn.danger` with 44px targets. Do not place a destructive action adjacent to a clipboard action.

### H4. Every card action has the same visual weight; there is no primary next step
- **Pass:** Designer
- **Where:** `MachineFleet.svelte:396–401` — "Open terminal", "Start an agent", "Commands", "Settings" all `class="btn"`; the 2×2 grid at `max-width:560px` (`:581–586`).
- **Problem:** The page's own lede says "Choose a machine to sync its skills or continue working in its terminal", but on a mobile card four identical outlined buttons form a button wall, with Commands/Settings (administrative) as prominent as Open terminal (the product). The only filled button is "Sync now" in the band above, which is often the wrong action (see H5).
- **Fix:** Make "Open terminal" the card's primary filled button when terminal access is ready; keep "Start an agent" secondary. Move Commands and Settings into the existing "…" overflow menu. On mobile cap the card at two action rows.

### H5. "Everything in sync" still presents a primary "Sync now"
- **Pass:** First-time user
- **Where:** `web/src/lib/syncOverview.ts:165–172` returns `tone:'synced', canSync:true`; `MachineFleet.svelte:349–356` renders the primary `Sync now` for every `canSync` state. The up-to-date `build.*` card, 1440px and 375px.
- **Problem:** The band says "Everything in sync / No changes found" and directly beneath it the strongest button on the card says "Sync now". The user cannot tell whether the product knows the state or is just offering a no-op. It also wastes the one primary slot that should be reserved for machines with real pending work. On the fixture it makes two of three cards shout the same orange CTA for opposite reasons.
- **Fix:** In the `synced` branch return `canSync:false`. Render a quiet "Check again" (or "Run sync again") as a secondary action, and reserve the filled `Sync now` for `action`/`review` tones only. Keep a real primary only where `missing > 0` or approval is required.

### H6. The terminal-readiness card is permanent, undismissable, and shown to users who never asked for terminals
- **Pass:** First-time user
- **Where:** `Machines.svelte:29` `showSetup` is true whenever `controller` is unavailable or `setUpCount === 0`, regardless of whether the user cares about the terminal; `:48` renders it above everything. 1440px and 375px, empty and populated states.
- **Problem:** A user who connected a machine only to sync skills will keep seeing "Get your first terminal running" above their inventory forever, including a server-admin command (`reet terminal start`) they may not be able to run. It reads as an unskippable setup nag and pushes the actual machines down. This is the "multiple simultaneous upsell touchpoints" failure in a self-hosted form.
- **Fix:** Only show the readiness card when the user has no sessions and has not dismissed it; add a dismiss control persisted per user. Move terminal setup guidance into the Sessions empty state and the setup wizard's step 3.

### H7. Failed update action says "Try again…" with no object
- **Pass:** First-time user
- **Where:** `web/src/lib/machines.ts:206` returns `label:'Try again…'` for failed/rolled-back updates; rendered at `AgentUpdate.svelte:178`. The `build.*` card at both widths.
- **Problem:** "Try again…" is ambiguous out of context: try the sync, the update, the connection? The surrounding block says "Update failed", but the button itself should stand alone and say what will happen. The trailing ellipsis also implies a dialog rather than a direct retry.
- **Fix:** Label it with the target: `Retry update to v1.4.2`. For a channel switch use `Retry switch to dev`. Add the version from `shownTarget` so the action is self-describing.

### H8. Focused-machine mode mixes global and filtered counts
- **Pass:** First-time user
- **Where:** `?machine=…` (`Machines.svelte:46` H1 "pi details"; `EnvironmentSync.svelte:62` filters `visibleMachines`; `MachineSetup` still receives the full `machines` list at `:254`). 1440px.
- **Problem:** The H1 says "pi details", the fleet header says "1 machine, one clear view", and the setup card directly above says "3 machines connected". Three different scopes on one screen, and "pi details" is machine-generated rather than human. A user arriving from Jump search cannot tell whether they are looking at one machine or a filtered list.
- **Fix:** Use a human H1 ("Machine details" or the host with "Machine" eyebrow), add a visible breadcrumb above the H1 (not below the section heading), and pass the same filtered set (or an explicit "of 3") to every count. Hide the setup card's global count in focus mode.

### H9. No loading state for the machine list
- **Pass:** First-time user
- **Where:** `EnvironmentSync.svelte:255–257` renders the fleet only when `visibleMachines.length`; `loading` only changes the "Refresh status" label (`:232`). On a slow `/api/v2/sync`, the inventory section is blank.
- **Problem:** On first load the page shows a fully formed header and setup card with no indication that machines are still being read. A user can reasonably conclude their machines disappeared, or that the page is broken. The same blank state appears after a failed read (`error` banner is elsewhere and easy to miss).
- **Fix:** Render an explicit `<p role="status">Reading your machines…</p>` and 1–3 skeleton cards in the grid while `loading && !loaded`. On failure, show the error inline in the grid with a Retry that reloads only this section.

### H10. Inconsistent icon language
- **Pass:** Designer
- **Where:** `MachineSetup.svelte:319` (`↻`), `:323` (`＋` fullwidth), `:313` (`›`), `MachineFleet.svelte:565` (`⌄`), `Machines.svelte:47` (`→`), dialog close buttons (`×`), plus SVG dots in the sync menu (`MachineFleet.svelte:358`).
- **Problem:** Six glyph families for four concepts (expand, add, navigate, close). `↻` for "connect an existing machine" reads as refresh; the fullwidth `＋` has different metrics from the rest; the chevron rotates from `›` to a non-chevron at 90°. It makes an otherwise distinctive design feel assembled rather than crafted.
- **Fix:** Introduce one 16px SVG icon set (`chevron`, `plus`, `refresh`, `arrow-right`, `x`, `more`) and use it everywhere, including the machine card overflow menu.

### H11. Add-a-machine dialog's footer has no action until the machine is ready
- **Pass:** First-time user
- **Where:** `AddMachineDialog.svelte:302–307`; `canUse` requires `status.canUse` (`:75`).
- **Problem:** Before the connector reports in, the footer contains only "Nothing here changes a machine." and an empty action area. The user who has just run the command has no "I've done it — check again" affordance, and the only exit is the close cross. The eventual primary is also vague: `Use dev-laptop`.
- **Fix:** Add a persistent secondary **"I've run it — check again"** next to the primary (or in the status block), and rename the primary to **"Done — show dev-laptop"**. Keep the footer layout stable so the dialog does not jump when `Use` appears.

---

## 3. Nice to have

### N1. Three date/time formats on one screen
- **Pass:** Designer
- **Where:** `EnvironmentSync.svelte:80–84,237` (`toLocaleString`), `MachineFleet.svelte:169–172,415` (`toLocaleString`), `syncOverview.ts:49–59` (relative "just now"/"2 days ago").
- **Problem:** "Last report 10/28/2026, 8:33:08 PM", "Status refreshed 10/28/2026, 8:33:09 PM" and "last checked just now" coexist; the profile's `dateFormat`/`timezone` (`api.ts` profile) are ignored.
- **Fix:** Use relative time for per-card recency, and the user's profile format for the absolute tooltip/title. One helper, used everywhere.

### N2. "Private skills" and "Private skill imports" are near-homonyms
- **Pass:** First-time user
- **Where:** `MachineFleet.svelte:307` vs `:320`; dialogs titled "Private skills on {host}" (`MachineSkillContent.svelte:446`) and "Bring a machine skill into reet" (`MachineSkillImports.svelte`).
- **Problem:** Two controls with almost the same name open opposite-direction flows (manage private skills on a machine vs import a machine skill into reet). Even after C3 is fixed, the labels will still be confused.
- **Fix:** Name the import flow by its outcome ("Import a machine skill into reet") and keep "Private skills" for the per-scope management. Consider moving the import entry into the scope row next to "Private skills" where the source machine is obvious.

### N3. Duplicate "Sessions" entry
- **Pass:** Designer
- **Where:** `Machines.svelte:47` sub-nav and the global top/bottom nav (`App.svelte:195`).
- **Problem:** Sessions appears twice in the same viewport, once as a page sub-nav item and once as a global destination.
- **Fix:** Keep it in the global nav only; use the sub-nav for Machines-specific views.

### N4. Machine card "…" menu is not keyboard-complete and does not close on outside click
- **Pass:** Designer
- **Where:** `MachineFleet.svelte:357–365` (native `<details>`), `:220–222` (`closeMenu` only on item activation).
- **Problem:** `role="menu"`/`menuitem` implies arrow-key navigation and Escape-to-close; the implementation has neither, and clicking anywhere else leaves the menu open.
- **Fix:** Use a small menu component with roving tabindex, Escape handling, focus return to the summary, and a document click listener.

### N5. "Advanced machine maintenance" duplicates destructive paths
- **Pass:** First-time user
- **Where:** `EnvironmentSync.svelte:260–295`; `MachineSettings.svelte:104–122`.
- **Problem:** Forget/merge/report cleanup exists both under Advanced and in per-machine Settings, with different copy and confirmation flows.
- **Fix:** Keep identity cleanup (merge/rename) in Advanced and per-machine Forget in Settings, and cross-link them instead of duplicating.

### N6. No way to sort or filter the fleet
- **Pass:** Designer
- **Where:** `MachineFleet.svelte:323–443` renders machines in API order.
- **Problem:** With three machines the grid is fine; with ten it is a wall of equal cards with mixed states. "One clear view" stops being true.
- **Fix:** Sort by attention (action > review > working > paused/offline > synced), then host; add a compact status filter once the count exceeds ~6.

### N7. Commands dialog has two competing exits and an unrelated CTA
- **Pass:** First-time user
- **Where:** `MachineCommands.svelte:92–96` ("Open compare & sync", "Set up from scratch", "Done").
- **Problem:** "Set up from scratch" in a command-reference dialog is a context switch; "Done" and the header close cross duplicate each other.
- **Fix:** Keep the primary "Open compare & sync", make "Done" the only close affordance (or rely on the header close), and drop "Set up from scratch".

### N8. `aria-current` misuse on the machine article
- **Pass:** Designer
- **Where:** `MachineFleet.svelte:331` — `aria-current={highlight === machine.host ? 'true' : undefined}` on `<article>`.
- **Problem:** `aria-current` is meant for navigation/selection items; using it to mean "just added" can be announced oddly and duplicates the visible "Just added" note.
- **Fix:** Remove `aria-current`; the visible "Just added" text and the temporary border/animation already communicate it. If announcement is needed, use a visually hidden live region.

---

## Top 5 critical fixes, in order

1. **C1 — Delete the duplicated onboarding.** One flow (`MachineSetup`), with terminal readiness folded into step 3. The empty page currently gives a new user two competing 4-step tutorials.
2. **C2 — One connect CTA.** "Add a machine" (guided dialog) and "Connect a machine" (technical wizard) are adjacent and inverted; make the guided dialog the only primary and demote the wizard to a link inside it.
3. **C3 — Style and rename the toolbar button.** "Private skill imports" is a raw Chromium-default button (`rgb(107,107,107)`, 2px white border) sitting first in the primary toolbar, and its name collides with per-scope "Private skills".
4. **C4 — Put the machines above the fold.** Drop the Projects rail from `/machines`, collapse the three stacked headings into one, and move the grid above the setup card. Measured first card: 814px at 1440×812, 1023px at 375×812.
5. **C5 — Stop showing paused/offline in green.** `MachineFleet.svelte:377` colours every agent state with `--good`, so the paused card shows a green "● Sync paused" beside a red "Sync paused" band.
