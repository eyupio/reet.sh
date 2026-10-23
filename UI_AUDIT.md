# UI/UX Audit — reet.sh

**Audit date:** 2026-10-23
**Build inspected:** local source build (`npm run build` in `web/`; `go build ./cmd/reet`)
**Mode:** `REET_AUTH=accounts`
**Viewports:** 1440 x 1000 and 375 x 812 (plus 320 px overflow check), light and dark
**Method:** created a fresh owner account, added `anthropics/skills/pdf` and `anthropics/skills/frontend-design`, inspected the empty state, first-add state, list/board views, sign-in, setup, account, machines, community, desktop and mobile navigation, focus/hover/error states, and source-level state handling.

---

## Important context: there is no trial, paywall or upsell in this codebase

A search of `web/src` and `internal` found **no pricing, trial, subscription, billing, paywall, upgrade prompt, plan limit or nag modal**. This is an AGPL, self-hosted app where every feature is available to every account. The paywall checks in the brief therefore return negative: there are no premature upgrade modals, no simultaneous upsell touchpoints, and no paywall blocking value.

That does not make the conversion story healthy. The real funnel is:

> **land → create the owner account → empty shelf → add first skill → install/scan → share, host or sync**

The codebase is broader than a skills.sh tracker. It is a self-hosted home for AI skills, agents and setups, and also ships shared lists and teams, hosted skills, MCP access, portable agent setups, machine inventory and controller sync, and sealed backups. The copy and onboarding findings below treat those as first-class value props, not add-ons. The conversion event is **the first skill added and put to work** (installed, scanned, shared or hosted), not a trial signup.

### Onboarding data

There is no multi-step onboarding questionnaire. `Setup.svelte` collects name, email and password only; all three are used (name in lists/rollups, email as the login). Account preferences (theme, timezone, date order, list/board, collapsed/expanded cards) are saved and respected. There are **no collected-but-unused answers** because there are no questions. The problem is that there is no onboarding at all after account creation; the user is dropped on the shelf.

### First-time walkthrough, in one paragraph

Landing on a fresh instance shows "Set up reet.sh / Create the owner account for this instance" before the visitor has seen the product, a screenshot, or a mention of its real scope: a personal shelf, shared lists and teams, hosted skills, MCP, machine inventory, setups and backups. Creating the owner lands on an empty shelf with an input and three competing secondary paths. Adding a skill works, but the confirmation is a notice while the card itself is below the fold on both default desktop and mobile; on mobile the input's instruction is cut off mid-word. The skill card then presents five equal-weight actions, with "Save to test" visually primary and the actual value action (copy install / start using) secondary. There is no upgrade prompt because there is no upgrade path. The one choice the product does respect well is the list/board layout, which is saved to the account and persists across sessions.

---

# 1. Critical

Critical findings block conversion or break the experience.

## C1. There is no public landing page or trial path; `/` is an owner-setup form or a sign-in dead end

- **Pass:** First-time user
- **Where:** `web/src/App.svelte:645-710` (route branches), `web/src/lib/Setup.svelte:58`, `web/src/lib/AccountSignIn.svelte:41-50`; route `/`; 1440 and 375. `web/index.html` contains SEO/OG metadata but no marketing surface.
- **Problem:** A visitor on a new instance is asked to "Create the owner account for this instance" without a product explanation, screenshot, trust signal or mention of skills.sh. On a configured instance, `/` is only a sign-in card that tells them to ask an admin for an invitation. If reet.sh is offered as a hosted trial, there is no acquisition funnel at all. If it is self-hosted only, the setup page still does not sell the self-host path or explain what the visitor is installing.
- **Fix:** Add a public `/welcome` route (or render a marketing hero when unauthenticated, before the setup/sign-in card) with: H1 "Your shelf for everything AI", a value prop covering the full product (collect and test skills, sync agents and machines, share lists and teams, host skills, connect agents over MCP, keep portable setups and sealed backups), a product screenshot, a copyable self-host command, "Sign in"/"Create an account" CTAs, and a trust row (AGPL, self-hosted, no telemetry, one SQLite file, GitHub). Keep `/` as the app. If hosted, add pricing and a free-trial route and instrument landing → account → first skill; if self-host only, make the one-command install the primary CTA.

## C2. There is no self-serve signup or password reset; the sign-in screen is a hard dead end

- **Pass:** First-time user
- **Where:** `web/src/lib/AccountSignIn.svelte:41-50`; route `/` when `setupRequired=false`; 1440 and 375. The invite flow exists at `/join?token=…` (`web/src/lib/Join.svelte`) but is not discoverable from this screen.
- **Problem:** "Need an account or a password reset? Ask your instance admin for an invitation." is the only path. A visitor cannot start an account, request access or recover a password. The invite mechanism is hidden behind an admin action, so even team onboarding leaks at the first screen. For a hosted product this is fatal; for a self-hosted team app it is unnecessary friction.
- **Fix:** Add a "Create an account" / "Request access" form on the sign-in card that emails the admin or enables signup behind a `REET_ALLOW_SIGNUP` flag. Add a "Forgot password" flow (an admin-generated reset link is acceptable if clearly labelled). Link to the invite docs from the sign-in card. If invite-only is intentional, replace the passive sentence with an explicit admin "Invite a teammate" action and a visible "Join with an invitation link" path.

## C3. The first skill is added below the fold, under two empty stages, and is never scrolled into view

- **Pass:** Designer + First-time user
- **Where:** `web/src/lib/shelf.ts:8-12` (`stages` is In use, To test, On the shelf); `web/src/App.svelte:133-136` (list uses `grouped`; board reverses it), `405-416` (success does not change `show`; `flash` only runs when `!s.created`); `web/src/lib/SkillCard.svelte`; default `shelfView` is `list` (`internal/store/store_backups.go:459`); 1440 x 1000 and 375 x 812.
- **Problem:** After "Add to shelf", the notice says "Reet, added X to the shelf", but the card starts at roughly y=950 on both a 1000 px desktop viewport and an 812 px phone viewport. The default list view renders "In use 0" and "To test 0" with two dead "Nowt…" paragraphs before the user's skill. The onboarding panel and the board say On the shelf → To test → In use; the default list shows the reverse. The user's first value moment looks like a failure.
- **Fix:** After a successful create, set `show = 'shelf'` or call `flash(s.id)`/`scrollIntoView` for created skills too, not just duplicates. Hide empty stage sections in list view (`g.items.length > 0 || !filtering` regardless of view, or `skills.length < 3`). Align list order with the onboarding/board pipeline, or default new accounts to Board. Add an end-to-end assertion that the newly added `[data-skill]` is inside the viewport after the first add at 375 and 1440.

## C4. The primary add input is clipped to "Paste an install commar" on mobile

- **Pass:** Designer
- **Where:** `web/src/App.svelte:713` (placeholder), `1099-1106` (`@media (max-width: 560px) .add input { width: 0 }` plus a fixed-width button); 375 x 812 and 320 px.
- **Problem:** The only visible instruction for the product's primary action is the placeholder, and at 375 px the input measures about 219 px wide, cutting the text mid-word. At 320 px it is worse. Sighted users never see that a skills.sh URL or `owner/repo/skill` is accepted. The first action reads as broken.
- **Fix:** On ≤560 px, stack the form: `.add { flex-wrap: wrap }`, `.add input { flex: 1 1 100%; width: 100% }`, `.add .btn { flex: 1 1 100%; width: 100% }` (or place the button below the input at full width). Shorten the placeholder to "Paste a skills.sh link" and add a visible helper/example below, e.g. "e.g. anthropics/skills/pdf". Keep the full format in the accessible label.

## C5. The empty shelf has no examples and no one-click add; every offered path is offsite, empty or a CLI install

- **Pass:** First-time user
- **Where:** `web/src/App.svelte:838-852`; route `/` empty state; 1440 and 375.
- **Problem:** A new user must already know a valid skill identifier. "Scan this machine" is a heavy CLI path, "Explore community" is empty on a fresh instance, and "Browse skills.sh" leaves the app. There is no sample skill, suggestion, "host your own" or "Add your first skill" control in the card. This is the blank-page problem at the exact activation moment, and it contradicts the input above, which is the actual fast path.
- **Fix:** Render three curated suggestions (for example `anthropics/skills/pdf`, `anthropics/skills/frontend-design`, and one non-Anthropic skill) as rows with source, one-line description and an "Add" button calling `addSkill`. Add an example placeholder and a "Try one of these" caption. Make the card's primary CTA focus/scroll to the input, and list the other real capture paths: host your own skill, connect a machine, import a backup. Demote "Scan this machine" to "Already have skills installed?" and "Explore community" to a secondary link.

## C6. Discovery requires leaving reet.sh, and the bookmarklets that solve it are buried under "Shelf tools"

- **Pass:** First-time user
- **Where:** `web/src/App.svelte:847` ("Browse skills.sh", `target="_blank"`), `884-893` (Tools always rendered); `web/src/lib/Tools.svelte:186-190` (bookmarklets inside the collapsed tools panel); `/`; 1440 and 375.
- **Problem:** One of the core loops is "find a skill on skills.sh → capture it", but the UI sends users out to skills.sh with no way back except copy/paste and hides the Like/Save/In use bookmarklets in a collapsed advanced section below the shelf. First-time users will not discover them. The same discovery gap applies to the other capture paths the code already supports: hosting your own skill, scanning an existing machine, and importing a backup. skills.sh exposes a public search endpoint (I verified `https://skills.sh/api/search?q=pdf` returns results), so the outbound trip is self-imposed and the Go server could proxy it.
- **Fix:** Add an inline "Find skills" entry point that either searches skills.sh in-app or surfaces the bookmarklets above the fold with one-click "Add to browser" / "Copy bookmarklet" rows. Put hosting and machine-scan links in the same first-run discovery row, not only in the nav. Move bookmarklets into first-run onboarding, not "Shelf tools". Add a browser-extension callout for repeat use.

## C7. Setup/auth copy is vague, jargon-heavy and assumes self-hosting; mobile hides the only explanation

- **Pass:** Designer + First-time user
- **Where:** `web/src/lib/AuthCard.svelte:10,46-51`; `web/src/lib/Setup.svelte:58`; `web/src/lib/AccountSignIn.svelte:41,50`; routes `/` setup and sign-in; 1440 and 375.
- **Problem:** "A reet good home for your skills" and "A reet good toolkit" never say what the product is or how broad it is: a shelf, shared lists and teams, hosted skills, MCP, machine inventory, setups and backups. "Set up reet.sh", "Create the owner account for this instance" and "Everything already on the shelf stays where it is" are deployment language. "Create owner" is internal jargon. The mobile breakpoint hides `.intro-copy`, `.stages` (the only explanation of On the shelf / To test / In use) and `.local` ("Self-hosted. Yours to keep."), leaving a form on a slogan. Separately, `br { display: none }` removes the whitespace between "home" and "for", so every ≤560 px auth screen renders "A reet good homefor your skills."
- **Fix:** Use "Your shelf for everything AI" as the H1 and a lede that names the real scope: "Collect the AI skills, agents and setups you and your team use. Test them. Share lists. Sync every machine." Label the CTA "Create your account". Keep a compact one-line/three-chip explanation and the self-hosted trust line on mobile. Add "No telemetry · one SQLite file · AGPL" plus docs/GitHub links. Fix the whitespace by writing `A reet good home <br />for your skills.` (the space survives when the break is hidden by CSS).

## C8. Anchors styled as `.btn` render in the browser-default blue

- **Pass:** Designer
- **Where:** `web/src/app.css:40-42` (`.btn` sets background, border and radius but no `color`); `web/src/App.svelte:845-846` (`<a class="btn">Scan this machine</a>` and `<a class="btn">Explore community</a>`); empty shelf at 1440 and 375. The same defect is visible on the Machines sub-nav (`web/src/lib/Machines.svelte:16`), SetupProfiles (`web/src/lib/SetupProfiles.svelte:204,219`), Backups (`web/src/lib/Backups.svelte:728`) and PublicList (`web/src/lib/PublicList.svelte:58`) unless a scoped `a { color: … }` happens to override it.
- **Problem:** The first-run empty state shows two default-blue link/buttons next to an orange text link instead of the brand's ink/orange system. It looks like raw HTML/default styling and undercuts the crafted identity at the first decision point.
- **Fix:** Add `color: var(--ink)` to `.btn` and `color: var(--accent-ink)` to `.btn.primary`; keep `text-decoration: none`; ensure `a.btn:hover` matches `button.btn:hover`. Add a visual-regression snapshot for the empty shelf.

## C9. "Add to shelf" has no disabled, busy or validation state; empty clicks do nothing and slow adds look dead

- **Pass:** Designer + First-time user
- **Where:** `web/src/App.svelte:405-419` (no busy flag; `if (!addInput.trim()) return`), `710-715` (button always enabled; no `aria-busy`; no `aria-invalid` on error); `/`; 1440 and 375.
- **Problem:** The primary CTA is active with an empty input; clicking it produces no message. While the request (and metadata fetch) is in flight the button still says "Add to shelf" and can be double-submitted. The error notice is a separate lowercase sentence ("expected a skills.sh URL, slug, or npx skills add command") not tied to the input, and the field is not marked invalid. At the first interaction this reads as unresponsive.
- **Fix:** Add `let adding = $state(false)`; disable when `!addInput.trim() || adding`; change the label to "Adding…" and set `aria-busy`; guard re-entry. On error set `aria-invalid="true"` and `aria-describedby`, focus/select the input, keep the typed text, and use friendly copy: "That doesn't look like a skill yet. Paste a skills.sh link like … or an `npx skills add` command." Render the error inline under the field rather than as a separate page notice.

## C10. "Scan this machine" is mislabelled and lands on a three-step CLI wall with no guided flow

- **Pass:** First-time user
- **Where:** `web/src/App.svelte:845`; `web/src/lib/EnvironmentSync.svelte:110-149` (`#connect-machine`, Install/Authenticate/Scan steps, long `<pre>` blocks); route `/account/machines#connect-machine` (renders `Machines.svelte`); 1440 and 375.
- **Problem:** The empty state says "Scan this machine", implying a one-click scan, but the destination is a dense install/auth/scan page. The install command is a wrapped monospace block, and the page exposes "controller site", "enrolled machines", "agent groups", OAuth, release channels and a "Host a server" tab before the user has a single skill. On mobile the command is barely legible and the step tabs are cramped. This is the second most likely first action and it feels like a different, advanced product.
- **Fix:** Open a focused dialog/sheet with three numbered steps and one primary action per step: "Copy install command" → "I have installed it" → "Copy scan command". Detect/report enrollment status and close the loop with "Scan received". Hide advanced sections behind an "Advanced" disclosure until step three. Rename the empty-state CTA to "Connect this machine" and set expectations ("about 2 minutes").

---

# 2. High impact

High-impact findings create significant friction or lost trust.

## H1. Navigation front-loads five nouns and a junk-drawer "More" before value

- **Pass:** Designer + First-time user
- **Where:** `web/src/App.svelte:115-129`; `web/src/lib/NavigationMenu.svelte`; both viewports.
- **Problem:** The primary nav is Skills, Lists, Setups, Machines, More. "Setups" and "Machines" are advanced concepts; "More" contains Community, Teams & organisations, Host skills, Account & preferences and Instance administration. A first-time user does not know what any of these mean and there is no primary action in the header. On mobile the Menu button hides the current section (`primary summary span { display: none }`), so users lose their place.
- **Fix:** Primary nav: Skills, Lists, Community. Move Machines/Setups into a "Workspace" menu or Account. Put Account/Admin behind an avatar menu. Use "Add skill" as the header primary action once the shelf has content. On mobile show "Menu · Skills" or similar.

## H2. "Filters & lists" is noisy on first run and has a plural bug

- **Pass:** Designer
- **Where:** `web/src/App.svelte:756-757`; empty and one-skill states; 375 and 1440.
- **Problem:** The summary always shows "0 tags · 0 environments" before there is anything to filter. After adding one tagged skill it says "1 tags". It sits between the add form and the onboarding card, making the empty state look like a configured dashboard that failed to load data.
- **Fix:** Render the details only when `tagCounts.length || shelfMachines.length || filtering || lists.length`. Label it "Filters" when there are no lists. Pluralize with a helper ("1 tag · 1 environment"). Move it below the start/empty card or into the toolbar.

## H3. "Shelf tools" advanced section is always on the page, including first run

- **Pass:** Designer
- **Where:** `web/src/App.svelte:884-893`; `web/src/lib/Tools.svelte:186-190`; both viewports.
- **Problem:** Import/export, bookmarklets and ignored items take a 48 px margin, a heading and a button before the footer on every state, including the empty shelf. It asks users to configure tools before they have a single skill and competes with the real next step.
- **Fix:** Hide until `counts.all > 0`. Move the full tools panel to Account → Tools for established users. Keep only a small "Import your installed skills" link under the empty-state card. Once shown, keep it collapsed.

## H4. Three equal-weight CTAs and duplicated instructions in the empty state

- **Pass:** Designer + First-time user
- **Where:** `web/src/App.svelte:842-848`; empty state; both viewports.
- **Problem:** The paragraph repeats the input placeholder almost verbatim. Then two bordered links and one orange text link offer three paths with no hierarchy. The fast path (paste a link above) has no button inside the card. This creates hesitation instead of action.
- **Fix:** One primary "Add your first skill" button that focuses the input, one secondary "Connect a machine", and a quiet "Browse skills.sh" link. Put the format explanation in a single helper line under the input, not in the card. Remove the duplicated sentence.

## H5. Auth inputs autofocus on load, forcing the mobile keyboard up and hiding the page

- **Pass:** Designer + First-time user
- **Where:** `web/src/lib/Setup.svelte:21-23`; `web/src/lib/AccountSignIn.svelte:15-17`; `web/src/App.svelte` after-auth `addEl?.focus()`; 375.
- **Problem:** On a phone, autofocus opens the soft keyboard immediately, covering the hero/form and causing a layout jump before the user has read the value prop. On the setup page this happens before the user chooses to type. The heavy focus ring also reads as a validation error in screenshots.
- **Fix:** Autofocus only when `matchMedia('(pointer: fine)')` or viewport ≥700 px. Never autofocus on touch. After setup, scroll the add input into view without focusing on mobile. Soften the focus ring with a 2 px outline at reduced opacity, or remove the simultaneous `border-color: accent` on focus.

## H6. The add-form error state is raw and disconnected from the field

- **Pass:** Designer
- **Where:** `web/src/App.svelte:712-720`; invalid paste at 1440 and 375.
- **Problem:** The error appears as a separate notice below the form; the input keeps a neutral border, receives no `aria-invalid`, and focus stays on the button. The copy is lowercase and developer-ish ("expected a skills.sh URL, slug, or npx skills add command"). Users who paste the wrong thing may not connect the message to the field.
- **Fix:** Render an inline error under the input with `role="alert"` and `aria-invalid="true"`, focus/select the input, and use friendly copy with an example. Consider parsing the input and suggesting the likely fix before submit.

## H7. The skill card shows five equal-weight actions, and the value action is visually secondary

- **Pass:** Designer
- **Where:** `web/src/lib/SkillCard.svelte:224-233`; newly added skill at 1440 and 375.
- **Problem:** On a new skill "Save to test" is an orange outlined chip (primary), while "Start using", "Copy install", "Copy prompt" and "Like" are neutral. The user's actual goal is to install/use the skill; the card asks them to re-file it first. Five choices create decision friction and make the card feel like a control panel rather than a next step.
- **Fix:** On a newly created skill make "Copy install" or "Start using" the primary action. Move "Save to test" to secondary/overflow. Show one explicit "Next step" affordance. Put "Copy prompt" behind the expanded details. Keep Like as an icon-only action with an accessible label.

## H8. The mobile filter row scrolls horizontally with no affordance; Liked and Select disappear off-screen

- **Pass:** Designer
- **Where:** `web/src/App.svelte:1124-1125` (`.filters { flex-wrap: nowrap; overflow-x: auto }`); 375.
- **Problem:** The stage filters plus Liked and Select live in one scrollable row. At 375 px "Liked" and "Select" are off-screen; there is no fade, arrow or "more" cue. Users may never find bulk selection or liked filtering. A horizontal swipe inside the toolbar can also fight page scrolling.
- **Fix:** Let the stage filters wrap on mobile, or add a scroll shadow/snap. Move Liked into the stage row and "Select" into an overflow menu. Alternatively use one "Filter" button that opens the existing Filters & lists panel.

## H9. Search collapses every tab count to zero, making a one-skill shelf look empty

- **Pass:** First-time user
- **Where:** `web/src/App.svelte:723-729` (tab counts derive from the filtered `skills` array); search state at both viewports.
- **Problem:** Typing a non-matching query turns "All 1 / On the shelf 1" into "All 0 / On the shelf 0" while the user still has a skill. It is technically the result count, but it reads as data loss, and the empty state is below the toolbar, so the tabs reinforce the wrong impression.
- **Fix:** Keep total counts in the stage tabs (use the API's `counts`) and show "0 results" separately, or hide the stage tabs when `q` is non-empty and show a result count plus a "Clear search" chip.

## H10. The page H1 is a brand slogan, not a product title

- **Pass:** Designer
- **Where:** `web/src/App.svelte:708` (`<PageHeader eyebrow="Ey up. Your skill shelf." title="A reet good toolkit." …>`); both viewports.
- **Problem:** "A reet good toolkit" tells a new user nothing about what the page is or what to do. The actual page purpose is relegated to a tiny uppercase eyebrow. The largest text on the most important screen is brand voice, not navigation or value.
- **Fix:** Make the H1 "Your skills" or "Your shelf". Use the brand line as a small eyebrow or remove it. Add a one-line subhead: "Saved from skills.sh. Test them, keep the good ones, track what you use."

## H11. "Start using" implies an install but only moves the card between shelf stages

- **Pass:** First-time user
- **Where:** `web/src/lib/shelf.ts:31-32` (`{ label: 'Start using', flags: { used: true } }`), `web/src/lib/SkillCard.svelte:226-232`; README explicitly says "Moving a card changes shelf status, not installed files"; newly added skill at 1440 and 375.
- **Problem:** A first-time user reads "Start using" as "install and use this skill". It only sets the `used` flag and moves the card to In use; nothing is installed. The actual install action is "Copy install", which sits next to it as a neutral chip. The UI never explains the difference, so the user can believe they have started using a skill that is not on their machine.
- **Fix:** Rename the stage action to "Mark as in use" or "I use this" and make "Copy install" the visually primary action for a skill on the shelf. Add a persistent one-line clarification on the card or in the empty-state onboarding: "Moving a card changes your shelf, not your machine. Use Copy install to install the skill." Keep the README's explanation consistent with the label.

---

# 3. Nice to have

## N1. Bundle the serif font

- **Pass:** Designer
- **Where:** `web/src/app.css:5` (`--serif: 'Iowan Old Style', 'Palatino Linotype', Palatino, Georgia, serif`); all headings; Linux/Windows.
- **Problem:** The editorial identity depends on Iowan/Palatino being installed. On Linux the screenshots fall back to a Times-like serif, which makes the product look generic and mismatched with the rounded wordmark.
- **Fix:** Self-host one variable serif (for example Fraunces or Source Serif 4) with `font-display: swap` and set `--serif` to it, keeping the existing stack as fallback. Check the weight/letter-spacing at 1.4-3 rem.

## N2. Fix pluralisation across counts

- **Pass:** Designer
- **Where:** `web/src/App.svelte:757` ("1 tags"), `web/src/lib/SharedLists.svelte:31-36`, `web/src/lib/Community.svelte:24`, `web/src/lib/EnvironmentSync.svelte:213`, `web/src/lib/ListView.svelte:516`, `web/src/lib/SetupProfiles.svelte:192,293,335`.
- **Problem:** Counts are hard-coded plural. One-skill lists and one-tag shelves show "1 skills", "1 tags", "1 components" and "1 memory databases".
- **Fix:** Add a shared `plural(n, 'skill')` helper and use it everywhere a count is rendered. Add a test for 0, 1 and 2.

## N3. Add loading skeletons instead of bare "Loading…" text

- **Pass:** Designer
- **Where:** `web/src/App.svelte:831`, `Account.svelte`, `SharedLists.svelte`, `Community.svelte`; slow first load.
- **Problem:** The app flashes a centred italic "Loading…" then the whole layout. On a slow self-hosted box this feels unfinished and causes layout shift.
- **Fix:** Render a shelf skeleton (one header row plus two card shapes) and an auth-card skeleton at the same dimensions as the final content. Keep `role="status"` with visually hidden text.

## N4. Consolidate success feedback

- **Pass:** Designer
- **Where:** `web/src/App.svelte:718-721` (full-width green `.notice` for add/moves), `web/src/lib/Toast.svelte` (used for bulk/undo).
- **Problem:** Add success uses a full-width green band that pushes the toolbar down for six seconds; other actions use a toast. The two patterns compete and the band looks like an alert, not confirmation.
- **Fix:** Use the existing Toast for add/move/refresh success, or make the notice a small inline status in the input row. Reserve the full-width band for errors and blocking notices.

## N5. Fix the mobile auth header and logo scale

- **Pass:** Designer
- **Where:** `web/src/App.svelte:1101-1103` (`.tag` wraps at 18ch), `web/src/lib/Logo.svelte:8` (160 px); 375 auth pages.
- **Problem:** The 160 px logo plus a two-line uppercase tagline makes a 70 px header that competes with the form. The tagline wraps awkwardly ("YOUR SHELF FOR / AI SKILLS").
- **Fix:** On ≤560 px, reduce the logo to 132-140 px and hide the tagline or shorten it to "AI skill shelf". Keep the header one line.

## N6. Remove duplicate counts and empty-stage filler

- **Pass:** Designer
- **Where:** `web/src/App.svelte:726-728` ("All 1") and `823` ("1 skill") and section counts; empty list view; both viewports.
- **Problem:** The same total is shown three times (All tab, shelf summary, section header) and empty stages add two italic paragraphs before the first card. The screen is busy without adding information.
- **Fix:** Show the total once in the summary; keep stage counts in the filter tabs only. Hide empty stages in list view until the shelf has multiple skills. Shorten the remaining empty copy to one line.

## N7. Make the theme toggle discoverable

- **Pass:** Designer
- **Where:** `web/src/lib/Account.svelte` profile section; header; all pages.
- **Problem:** Theme lives only under Account → Profile. Users who prefer dark mode (or need high contrast) must leave the shelf and find a settings page.
- **Fix:** Add a small light/dark/system toggle in the header or account menu, and keep the Account setting as the persistent default. Ensure the toggle respects `prefers-color-scheme` when set to system.

## N8. Reduce signup friction: drop the confirm-password field or explain why it exists

- **Pass:** Designer + First-time user
- **Where:** `web/src/lib/Setup.svelte:75-80`, `web/src/lib/Join.svelte`; setup at both viewports.
- **Problem:** The setup form asks for name, email, password and confirm password. The confirm field adds a failure mode without adding security, especially with the existing Show control. The password requirement is only revealed after an error.
- **Fix:** Keep the Show toggle and drop the confirm field on first setup (or keep it only for high-risk admin accounts). Show "At least 8 characters" as persistent helper text under the password field. Add a lightweight strength hint.

## N9. Fix the add-input focus ring

- **Pass:** Designer
- **Where:** `web/src/app.css:31,34` (focus border plus a 2 px outline with 2 px offset); setup and shelf add input.
- **Problem:** The autofocused input renders a bright double orange ring in the screenshots. On first load it reads like an error state rather than focus.
- **Fix:** Use either the border colour or the outline on focus, not both; reduce the outline to 2 px with `outline-offset: 1px` and a 60% accent mix, and keep the high-contrast focus style for keyboard users only.

## N10. Keep the Yorkshire voice out of error and recovery copy

- **Pass:** Designer + First-time user
- **Where:** `web/src/App.svelte:855` ("Nowt matches just yet."), `web/src/lib/Community.svelte` empty states, `web/src/lib/SetupProfiles.svelte`; error paths.
- **Problem:** The voice is a genuine differentiator, but "nowt" and "reet" in empty/error states can be opaque to international users and slow down recovery. The brand line is charming; the error state should be plain.
- **Fix:** Keep the voice in marketing and success microcopy; use literal copy for errors ("No matches. Try another search or clear the filters.") and keep the playful line as a secondary sentence.

---

## Suggested fix order

1. C3 + C9 first: make the first add visible and the add button honest. These are the cheapest changes with the largest activation effect.
2. C4 + C5: fix the mobile input and give users three one-click examples. This removes the blank-page problem.
3. C7 + C8 + C10: make the first screens explain the product, fix the blue anchors, and turn "Scan this machine" into a guided two-minute flow.
4. C1 + C2: build the public landing/self-serve path once the app's first-run experience can actually convert.
5. H1-H11: information architecture, error states, mobile filters and card action hierarchy.
6. N1-N10: typography, pluralisation, skeletons and feedback polish.
