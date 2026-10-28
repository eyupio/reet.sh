# UI audit — `/sync`

**Scope:** `https://reet.sh/sync` (the Sync route in this worktree, `goths@9c91987`).
**Method:** Read `Machines.svelte`, `EnvironmentSync.svelte`, `SyncControl.svelte`, `SyncActivity.svelte`, `SyncProblem.svelte`, `FleetActions.svelte`, `ProjectsBar.svelte`, `PageHeader.svelte`, the shell navigation and `app.css`; then built the current web app and rendered `/sync` against fixture API responses in Chromium at **1440×1000**, **375×812** and **320×700**, light and dark. Interaction states were exercised with Playwright (selection, approval dialogs, failures, empty state). Production `/sync` is auth-gated and redirects to the signed-out landing page, so the rendered state used a writer account and fixture machines/jobs; no production data was read or changed.
**Passes:** Designer teardown and first-time-user walkthrough.
**Paywall/upgrade checks:** Not applicable. This is a self-hosted product with no trial, upgrade modal or paid tier in the codebase. “Conversion” here means reaching the first successful preview → approval → apply and trusting the result.

What already holds up: no horizontal overflow at 320/375/1440, failure cards name an exact fix and command, status is never colour-only, light/dark are consistent, and destructive “Stop tracking” has confirmation.

---

## Critical

### 1. “Save” writes an empty desired set
- **Pass:** First-time user
- **Where:** `web/src/lib/SyncControl.svelte:486` (button), `:123-149` (`save` / `reviewSave`); `/sync`, 1440×1000 and 375×812.
- **Problem:** The primary button is enabled with **0 skills selected**. A Playwright click sent `PUT /api/v2/sync/desired` with `{"skillIds":[],"mode":"additive"}`. Because the page also opens at 0 selected (finding 2), a distracted click silently replaces an existing saved set with nothing. In **Exact set** or **Bidirectional** this is the removal path, and the only guard is a confirmation about extras — not about saving zero skills.
- **Fix:** Add `selected.length === 0` to the button’s `disabled` expression and guard inside `save()`. Treat “save an empty set” as a deliberate action with its own label (“Clear this desired set”), and name the number of removals it will lead to. Put “0 of N skills selected” directly beside the button, not only above the list.

### 2. The compare panel ignores the selected destination’s saved set
- **Pass:** First-time user
- **Where:** `web/src/lib/SyncControl.svelte:307-327` (auto-select on mount), `:97-109` (`loadSource` clears selection), `:110-116` (`selectMachine`), `:491-515` (saved sets).
- **Problem:** With Elitebook/OpenCode saved as **4 desired / 2 to install**, the top panel auto-selects Elitebook/OpenCode but shows **0 selected**, **0 only on reet.sh**, **3 only on selected scopes**. It defaults the source to **My shelf** even when the saved set came from a list, and defaults mode to **Additive** even when the saved set is **mirror** or **bidirectional**. The same page then contradicts itself lower down (“4 desired”). Users conclude their set was lost, or save a new set that overwrites it.
- **Fix:** When auto-selecting a destination with an existing target, hydrate `source`, `listId`, `mode`, `selected` and `revision` from `snapshot.targets` (reuse the `edit()` path). If that is not wanted, do not auto-select a machine; show a per-scope “Edit saved set” empty state instead. Never present a blank form as the saved state. Add an “Editing saved set · revision r7” banner.

### 3. “Review & approve” hides the thing being approved
- **Pass:** First-time user
- **Where:** `web/src/lib/SyncControl.svelte:305` (confirm dialog), `:333-345` (job row), `:372-387` (plan disclosure); `/sync#sync-jobs`, 1440×1000.
- **Problem:** The primary action sits above a collapsed **“Plan · 3 changes”** disclosure. The confirmation says only “2 installs and 1 removals in /home/dev/.config/opencode/skills” and never names a skill. It also says “1 removals”. The user is asked to approve file-system changes they cannot see at the moment of approval. This is the trust centre of the product.
- **Fix:** When `entry.review`, render the plan actions inline above the button: install/remove + skill name + source. Pass that list into `ConfirmDialog` and fix the plural (`1 removal`). Label the confirm action **Apply changes**, not **Confirm**. Keep the disclosure for history, not for approval.

### 4. “Sync now” changes files while the page says nothing changes without approval
- **Pass:** First-time user
- **Where:** `web/src/lib/SyncControl.svelte:77` (“Nothing changes until you approve a plan.”), `:193` and `:508` (“Automatic sync started… additive plans apply on their own”), `docs/controller-sync.md:60`; saved machines at `/sync`, 1440×1000.
- **Problem:** Automatic runs auto-approve additive installs. On the same screen the activity panel promises no changes without approval, and **Sync all now** / **Sync now** are the strongest primaries. A user expecting a preview gets installs. This is the largest trust break on the page and directly contradicts the safety story used elsewhere (“review every change before it is applied”).
- **Fix:** Pick one model per surface. Either make “Sync now” a preview-and-approve flow, or relabel it **“Sync now (installs automatically)”** and change the activity copy to “Automatic runs install additive changes; removals wait for your approval.” Add a confirmation naming the count before an automatic run. Do not show both promises on the same page.

### 5. The empty-state primary CTA is hidden behind the phone tab bar
- **Pass:** First-time user / Designer
- **Where:** `web/src/lib/EnvironmentSync.svelte:247-251`; `web/src/lib/shell/BottomTabs.svelte:16-28`; `/sync` empty state at 375×812.
- **Problem:** Measured CTA: top **778px**, bottom **822px**. Bottom tab bar: top **748px**, bottom **812px**. `elementFromPoint` at the CTA centre hits the tab bar. The only next step on the empty page is invisible at first paint, and the page is 1092px tall for one empty card. A new user sees “Connect a machine to start syncing” with no visible action.
- **Fix:** Fit the empty state inside `100dvh - top bar - bottom chrome` (shorten the paragraph, reduce padding), move the CTA immediately under the H3, and/or place it above the bottom chrome. Add a hit-test regression at 320/375/390 to prove the CTA is not covered. Do not rely on the user scrolling past a fixed bar to discover the only button.

---

## High impact

### 6. Activity comes first; the task starts below the fold
- **Pass:** Designer / First-time user
- **Where:** `web/src/lib/SyncControl.svelte:401-439` (activity), `:442-443` (fleet controls + selection); `/sync` 1440×1000 with four jobs.
- **Problem:** The activity card is **906px tall and starts at y=549**; the selection card starts at **y=1657**. At 1440×1000 the entire first viewport is page chrome and sync activity; none of the compare controls are visible. On 375×812 the page is **6073px** tall and the selection task begins around y=1900.
- **Fix:** Put only unresolved failures/reviews in a slim “Needs you” strip at the top (max two rows). Move resolved activity below the selection/saved sets or into a side rail, and keep “Earlier activity” collapsed. The full activity panel should be a tab, not the default first block.

### 7. The primary action is three to five screens away from the selection
- **Pass:** Designer / First-time user
- **Where:** `web/src/lib/SyncControl.svelte:474-486`; measured Save top **2850px** at 1440 and **3998px** at 375 (page 6073px).
- **Problem:** After choosing skills inside a 320px inner scroller, the user must pass seven count chips and two disclosures to reach Save. There is no sticky action. On a phone the button is roughly two-thirds down the document.
- **Fix:** Add a sticky action bar at the bottom of the selection card (above the phone’s bottom chrome): “N skills · 3 scopes · Additive” plus **Save desired set**. Keep a second Save at the top when editing an existing set. The label should say “Save desired set (no files change yet)”, not “Save to 3 agent scopes”, which sounds like apply.

### 8. “Compare” shows aggregates, not differences
- **Pass:** Designer / First-time user
- **Where:** `web/src/lib/SyncControl.svelte:480-484` (counts, diff disclosure), `:35-38` (derivations), `:474-476` (skill rows).
- **Problem:** Seven chips (“0 only on reet.sh (selected)”, “3 only on selected scopes”, “0 found in at least one selected scope”) are hard to parse, the per-destination diff is collapsed, and skill rows show only name/source. There is no per-skill installed/missing/extra state. The page promises review before change but hides the review.
- **Fix:** Put a status badge on every skill row (On all selected scopes / Missing on 2 / Extra on 1). Make **Differences by destination** a visible table or per-scope tabs above Save. Reduce the chips to three plain-language totals: to install, to remove, conflicts. Drop “found in at least one selected scope” entirely.

### 9. Offline machines look syncable
- **Pass:** First-time user
- **Where:** `web/src/lib/SyncControl.svelte:496-499`, `:508`; `web/src/lib/machines.ts:11`; saved sets at `/sync`, Studio last seen four days ago.
- **Problem:** “Sync all now” and “Sync now” are enabled with no offline badge or last-seen line. Clicking shows “Automatic sync started”, but the run cannot execute until the machine returns. The user concludes sync is broken rather than the machine being offline.
- **Fix:** Derive online with `agentOnline`; show “Offline · last seen 4 days ago” in the machine/scope header; disable or relabel actions with the reason and a **Reconnect** link/command. Only claim “started” after acknowledgement, or say “Queued — waiting for Studio”.

### 10. Two primaries and five equal actions per saved machine
- **Pass:** Designer
- **Where:** `web/src/lib/SyncControl.svelte:496-509`.
- **Problem:** The machine header has primary **Sync all now** plus **Preview all on machine**; every scope row has **Edit selection**, **Review local & web changes**, primary **Sync now**, **Preview** and **Stop tracking**. At 1440 there is no single next action; on mobile all five stack full-width and read as equally important. “Sync all now” and “Sync now” are ambiguous siblings.
- **Fix:** One primary per scope (**Sync now**), one secondary (**Preview**), and an overflow for **Stop tracking**. Move machine-level actions into the header overflow; use the documented **Sync saved skills** wording for the machine-level action and explain machine-vs-scope once.

### 11. Fleet controls look ready but are dead
- **Pass:** First-time user
- **Where:** `web/src/lib/FleetActions.svelte:20-23`; `/sync` with no runnable scopes.
- **Problem:** With 0 machines ready and 0 scopes ready, **Sync all · preview** is still the solid orange primary (at 50% opacity). The reason is buried in the collapsed **Skipped operations**. The user clicks the loudest button and nothing happens.
- **Fix:** Inline the reasons: “Elitebook: job pending · Studio: offline (last seen 4d)”. Use a neutral disabled style for primaries. Move the fleet controls into a toolbar near the activity headline instead of a full-width card that pushes the selection down.

### 12. Touch targets below WCAG minimum
- **Pass:** Designer
- **Where:** `web/src/lib/SyncControl.svelte:454` (All/None), `:378`, `:390`, `:485` (summaries), `:467` (“Choose destinations”); 375×812 touch.
- **Problem:** Measured **All 15×19**, **None 30×19**, disclosure summaries **19–22px high**, “Choose destinations” **15px high**. WCAG 2.2 AA 2.5.8 requires 24×24; the project already uses 44px for coarse pointers elsewhere. On a phone, All/None is the only bulk-select for scopes and is nearly impossible to hit.
- **Fix:** Give `.link` and summary a minimum 24×24 hit area (44×44 on `pointer: coarse`) with padding; replace All/None text links with 44px buttons. Enlarge inline links such as “Choose destinations”. Re-run the existing target-size audit.

### 13. Four names and an invalid heading order
- **Pass:** Designer
- **Where:** `web/src/lib/Machines.svelte:46` (eyebrow “Machines”, H1 “Sync, on your terms.”), `web/src/lib/EnvironmentSync.svelte:227-235` (H2 “Compare & sync”, eyebrow “Machines & sync”), `web/src/lib/SyncControl.svelte:444-450` (“Compare from”, “Skill collection”), `web/src/lib/ProjectsBar.svelte:105` (H2 before H1).
- **Problem:** The page is called Sync in navigation, Machines in the eyebrow, “Sync, on your terms.” in the H1 and “Compare & sync” in the H2. The source picker is called “Compare from”; the list picker is “Skill collection”. “Projects” is an H2 that precedes the page H1 on every signed-in page, so heading order is invalid and screen readers announce a Projects section before the page title.
- **Fix:** H1 **Sync**, eyebrow **Machines**, delete the duplicate H2 (or make it the task heading “Choose what to sync”). Rename **Compare from → Source** and **Skill collection → List**. Render Projects after the H1, or use a non-heading label / `aria-labelledby` so it does not break the outline.

### 14. Policy copy and repeated status noise sit between the user and the task
- **Pass:** Designer
- **Where:** `web/src/lib/SyncControl.svelte:445`, `:466-471`, `:462`; `web/src/lib/ContentObservation.svelte:7`.
- **Problem:** Two long 12.8px paragraphs (“Every selected scope receives the same desired set…”, “Bookmarks and ‘In use’ are personal shelf choices…”) sit between the pickers and the skill list. Every scope repeats **“Content not reported”**. The user must read policy before choosing skills; the control is pushed down and the page feels like documentation.
- **Fix:** Put one line under the mode select (“Additive keeps local extras; Exact set removes them after approval”) and move the rest into a “How sync decides” dialog. Collapse content observation to a single badge per scope and only when a report exists; hide “Content not reported” unless it changes a decision.

### 15. Unknown-source skills are selectable but cannot be fixed inline
- **Pass:** First-time user
- **Where:** `web/src/lib/SyncControl.svelte:475`, `:38`, `:486`.
- **Problem:** `scratch-notes` has a checkbox. Selecting it silently disables Save via `unknown.length > 0`; the only clues are a small “Unknown source — link before syncing” row and a count chip “1 unknown” after the list. There is no link action.
- **Fix:** Disable the row’s checkbox and replace it with **Link source**, or open the existing link flow from the row. Put an inline banner beside Save naming the blocked rows instead of only a count.

### 16. A failed projects read becomes a page-level error on Sync
- **Pass:** First-time user
- **Where:** `web/src/lib/Machines.svelte:45`; `web/src/lib/ProjectsBar.svelte:104-107`; `/sync`.
- **Problem:** When the folder-projects read fails, Sync shows a card above the H1: “PROJECTS — Couldn’t read your projects just now. Add or manage on Home.” It is unrelated to sync, competes with sync errors, and makes the page look broken before the user reaches the task.
- **Fix:** On `/sync`, omit the projects strip or reduce a failure to a quiet one-line aside below the task. Never render it above the H1. Label it as non-blocking and keep the retry with the thing that failed.

---

## Nice to have

- **Disabled primaries still look active.** `.btn:disabled { opacity: .5 }` leaves the orange primary dominant. Use a neutral fill for disabled and keep **Preview** secondary. `web/src/app.css`.
- **Timestamp is machine-speak.** “10/28/2026, 8:31:11 PM” should be “Updated just now” with the exact time in a `title`. `web/src/lib/EnvironmentSync.svelte:80`, `:232`, `:237`.
- **Queued job card wastes a screen.** “Waiting for machine” is followed by a large blank area and a lone centred Cancel. Collapse it to one line with an inline Cancel. `web/src/lib/SyncControl.svelte:347-376`.
- **Activity is not announced.** The inline activity has no live region; review/failure/completion changes are visual only. Add a polite `aria-live` on the headline, not the whole panel. `web/src/lib/SyncControl.svelte:401-439`.
- **“All machines” dumps every scope.** It renders a flat checkbox grid with no per-machine grouping or search. Group by machine with All/None per group, and add search above ~10 scopes. `web/src/lib/SyncControl.svelte:449-465`.
- **Accent overload.** Eyebrows, links, badges and primaries all use the same orange, so the CTA does not stand out. Reserve solid accent for primary actions and use `--accent-text` for labels/badges. `web/src/app.css`, `web/src/lib/theme.ts`.
- **“Sync is up to date” is job-derived.** It comes from `activity.done`, not from the current per-target diff. A partial or ignored state can still read “up to date”. Derive it from all saved targets’ `missing`/`extra` and scan freshness. `web/src/lib/SyncControl.svelte:59-66`.
- **Empty state repeats headings and offers a dead refresh.** Three headings stack before the CTA, and **Refresh status** is present with nothing to refresh. Use one heading and one action. `web/src/lib/Machines.svelte:46`, `web/src/lib/EnvironmentSync.svelte:225-233`, `:247-251`.
- **Mobile activity badge wraps.** The “4 jobs” chip drops under the subtext at 375px, visually disconnecting it from the headline. Keep it on the eyebrow row or hide it. `web/src/lib/SyncControl.svelte:403-410`, `:554-559`.
