# UI/UX Audit — reet.sh (re-audit)

**Audit date:** 2026-10-25
**Supersedes:** the 2026-10-23 audit (still in git history). That audit predates the "AI fleet control room" redesign, so most of it described a different product. Section 4 tracks what happened to each of its findings.
**Build inspected:** `main` at `3bf999f` (merge of PR #194). `npm ci && npm run build` in `web/`, `go build ./cmd/reet`, served with `REET_AUTH=accounts` on a fresh data directory. Checked again with `REET_SIGNUP=open` plus `REET_CONTACT_EMAIL`, and with a second fresh instance to confirm the first-run behaviour.
**Viewports:** 1440 x 900 and 375 x 812, light mode. Welcome page also in dark mode at 375. Chromium via Playwright.
**Method:** read the routes, pages, components, styles and onboarding logic. Then walked landing, setup, sign-in, sign-up, first-run Control, Machines, the add-machine flows, Terminals, Skills, Sync, Lists, Setups, Host skills, Teams, Account, Notifications, Support and Admin at both widths. I measured overflow, panel geometry, input widths, contrast ratios and focus behaviour in the browser, and added a skill both from a suggestion and by typing.

**Not exercised:** anything needing an enrolled machine, so the live terminal workspace, a populated inventory, a real sync preview and the fleet cards with telemetry were not seen. Lists, teams and admin were viewed empty only. Safari, Firefox and a physical touch device were not used. Screenshots were captured for evidence but are not committed.

---

## Fix status

Updated 2026-10-25 after the fixes merged. Each fix was written test-first, with a browser test that failed on the old code, and is run in the `terminal-browser` workflow. The findings below are left as written; this table says what became of them.

| Finding | Status | Where |
| --- | --- | --- |
| C1 phone menu off-screen | Fixed | #197 |
| C2 setup lands on the shelf; no 404 | Fixed | #197 |
| C3 landing leads with the wrong action | Fixed | #200 |
| C4 terminals off by default, not said | Fixed | #203 |
| C5 three first-machine flows | Fixed | #205 |
| H1 phone auth: typo, form below the fold | Fixed | #197 |
| H2 autofocus on touch | **Mostly fixed.** The forms only take focus with a fine pointer on a wide screen. The double ring on a focused input (border plus outline, `app.css:31,34`) is unchanged. | #197 |
| H3 first-run Control has three filled buttons | Fixed | #203 |
| H4 Skills and Setups hidden in More; two-row header | Fixed | #202 |
| H5 landing hero is a small-text mock-up | **Partly.** Text is at least 11px and wraps. It is still an illustration, not a real screenshot. | #200 |
| H6 "See what's next" opens the handoff log | Fixed (opens `docs/roadmap.md`) | #199 |
| H7 first Skills page competes with itself | **Mostly fixed.** Sync waits for a skill, "Create a skill" is secondary and hosting explains itself first. Shelf tools and the bookmarklet disclosure still show on an empty shelf. | #205 |
| H8 Setups and Sync first screens | Fixed. Sync asks for a machine first. Setups keeps its CLI steps (setups are made from the CLI) and folds the storage note. | #205 |
| H9 phone filters hidden; prefix field 28px | Fixed | #199 |
| H10 skill card hierarchy and "Start using" | Fixed (label in #199, hierarchy in #205) | #199, #205 |
| H11 dark theme warning colour | Fixed | #197 |
| N1 terminology drift | Fixed | #202 |
| N2 public footer links | Fixed. Docs, GitHub and AGPL-3.0; no Support link. | #199 |
| N3 confirm-password field | Open | |
| N4 success banner and raw error text | Open | |
| N5 Notifications settings form | Open | |
| N6 system serif font | Open (needs a font to self-host) | |
| N7 theme toggle | Fixed | #205 |
| N8 bare shelf loading text | Open | |
| N9 small touch targets | **Partly.** The Show button fills its field. Machine-card buttons are still 34px high on touch. | #199 |

---

## Context: there is no trial, paywall or upsell in this product

I searched `web/src`, `internal`, `cmd` and `mcp/src` for pricing, billing, subscription, trial, paywall, premium, upgrade prompts and plan limits. The only hit is an unrelated "wait for install/upgrade to finish" message in `internal/lifecycle/doctor.go`. reet.sh is an AGPL, self-hosted app where every feature is free.

So the brief's "get every visitor to start a free trial" has to be translated into this product's real funnel:

> **land → install the server (or get invited) → create the account → connect the first machine → open a terminal or sync a skill**

The conversion event is **a connected machine with a working terminal session or a first reviewed sync**. A skill added to the shelf is a weaker proxy. Findings are ranked against that funnel.

**The brief's specific checks, answered:**

| Check | Result |
| --- | --- |
| Paywall or upgrade modals before value | None exist. |
| Multiple simultaneous upsell touchpoints | No upsells. The closest analogue is stacked, competing calls to action on first-run screens: C5, H3 and H7. |
| Broken or cramped mobile layouts | Yes: C1 (navigation off-screen), H1 (headline typo, form cut by the fold) and H9 (squashed input, hidden filters). |
| Onboarding answers collected but never used | There is no questionnaire. Account setup collects name, email and password and all three are used. Profile preferences (theme, layout, card density) are saved and applied. The real mismatch is that the landing and setup copy promise "then connect your first machine" and the app drops the owner somewhere else: C2. |

---

# 1. Critical

Critical findings block conversion or break the experience.

## C1. The mobile main navigation opens 74px off the left edge of the screen

- **Pass:** Designer
- **Where:** `web/src/lib/NavigationMenu.svelte:49` (`.mobile { position: relative }`) and `:57` (`.primary .links { position: absolute; right: 0; min-width: min(260px, calc(100vw - 32px)) }`). Header layout: `web/src/App.svelte:1115` (`.app-nav { margin-left: auto }`) and `:1276-1277`. Any signed-in page at 375 and 414 px.
- **Problem:** The panel is anchored to the right edge of the "Menu" button, but that button sits mid-screen (x 97-185 at 375px) and the panel is 260px wide. Measured: panel left edge at -74px at 375px and -35px at 414px, so the first characters of every link ("Control", "Machines", "Terminals", "More") are clipped. It is fine from 768px up. On a phone, the only way to reach Machines, Terminals, Skills, Lists or Setups is a menu you can't fully read. This is the single biggest mobile defect.
- **Fix:** In `NavigationMenu.svelte`, make the primary panel span the header instead of hanging off the button:
  ```css
  @media (max-width: 1000px) {
    .primary .mobile { position: static; }              /* was relative */
    .primary .links { position: absolute; left: 16px; right: 16px; min-width: 0; top: 100%; }
  }
  ```
  and add `position: relative` to `.header-inner` in `App.svelte:1114`. Add a Playwright assertion that every `nav.primary .links a` has `getBoundingClientRect().left >= 0` at 375 and 414.

## C2. After creating the owner account you land on the Skills shelf, on the URL `/setup`, not the control room you were sold

- **Pass:** First-time user
- **Where:** `web/src/App.svelte:358` (`['/signin','/signup','/welcome']` is the only list that redirects to `/`), `:791` (Setup branch), `:838` (the `{:else}` fallback renders the skill shelf), `:368` (`addEl?.focus()`); `web/src/lib/Welcome.svelte:10-11` and `web/src/lib/Setup.svelte:58` ("Create the owner account, then connect your first machine").
- **Problem:** Verified on a fresh instance at 375px. After submitting setup the URL stays `/setup`, the tab title is "Your shelf", the page is "Your central skill library", and keyboard focus jumps to the skill input (on a real phone that would raise the soft keyboard). Reloading stays on `/setup` and shows the shelf again. The landing page and setup form both say the next step is connecting a machine, so the first screen after commitment contradicts the stated journey and sends the new owner to the secondary feature. Related: any unknown URL (`/this-page-does-not-exist`) also renders the skill shelf, because the shelf is the catch-all. There is no 404.
- **Fix:** Add `'/setup'` to the list at `App.svelte:358` so setup ends at `/`, which is Control. Then, in the same function, skip `addEl?.focus()` unless the destination is the shelf and the pointer is fine (`matchMedia('(pointer: fine)')`). Give the shelf an explicit route (`isSkills` already exists) and replace the `{:else}` branch at `:838` with a small "Page not found" view with a link to Control. Add a test: setup on a clean instance ends at `location.pathname === '/'` with h1 "Your fleet. Ready for work."

## C3. The landing page's primary button is the returning-user action, and a new visitor has no obvious way in

- **Pass:** First-time user
- **Where:** `web/src/lib/Welcome.svelte:10-11` (CTA row and invite note), `web/src/App.svelte:773` (public nav); route `/` and `/welcome`; both viewports; checked with signup closed (the default) and open.
- **Problem:** With signup closed, the hero shows a filled "Open your workspace →" button and, below it, a 12px note: "Need access? Ask this instance's owner for an account." A visitor without an account clicks the primary button and lands on a sign-in form with no sign-up path. With signup open, "Open your workspace →" is still the filled button and "Create an account" is the outlined secondary, so the action a new visitor needs is demoted. "Host it yourself" is a quiet text button. The install command, the nearest thing to a trial start, is the last section of a 2,150px page. Before setup, the header still offers "Sign in →" although no account can exist, and its "Workspace" link points at the page you are already on.
- **Fix:** Choose the primary action by instance state in `Welcome.svelte`:
  - `setupRequired`: primary "Set up this instance →" (`/setup`), no "Sign in" in the header.
  - `signupEnabled`: primary "Create an account" (`/signup`), secondary "Sign in".
  - Invite-only: primary "Install reet.sh" (`#self-host`), secondary "Sign in", and "Request an invite" as a visible link when `contactEmail` is set.
  Put the copyable install one-liner and its `CopyButton` directly under the hero CTAs so the first action is on screen without scrolling. Extend the trust row to "Free · AGPL-3.0 · Self-hosted · No telemetry" at 0.8rem or larger. Drop the "Workspace" header link when it equals the current page.

## C4. Browser terminals are off by default, and the dashboard never says so

- **Pass:** First-time user
- **Where:** `web/src/lib/ControlCenter.svelte:30` ("Open terminals →"), `:32` and `web/src/lib/terminal/ActiveSessions.svelte:39` ("Open workspace →"), `:47` (first-machine card), `:51` (help dialog). Status is only shown in `web/src/lib/terminal/TerminalCommandReference.svelte:8-9, 20-22`. Server gate: `cmd/reet/main.go:203` (`REET_REMOTE_TERMINALS == "on"`, reached by `reet terminal start`). Route `/`; both viewports.
- **Problem:** The headline feature of the landing page is "Open a browser terminal to use installed agents". On a default install it does not work: the "Terminal setup guide" dialog reads **"Controller terminal access: Inactive. Remote access is stopped."** (and it also needs an HTTPS public URL, a signed-in account and tmux 3.2+ on each host). That state is visible only inside a help dialog that most users won't open. Meanwhile the Control screen offers "Open terminals", "Open workspace" and a first-machine card that says "then open its shell from your browser". A new owner can follow every on-screen instruction, connect a machine and still hit a dead terminal with no explanation on the page.
- **Fix:** In `ControlCenter.svelte`, call `terminalControllerStatus()` (already exported from `./terminal/api`) inside `refresh()` and render a "Get your first terminal running" checklist above `ActiveSessions` until it is complete:
  1. Controller terminal access: Active / Inactive / Blocked, with `status.reason` mapped to a human sentence (`https_required`, `accounts_required`) and a `reet terminal start` copy button.
  2. A machine is connected.
  3. Terminal consent granted on that machine (`reet agent terminal setup`).
  4. Open your first session.
  Until steps 1-2 pass, render "Open terminals" and "Open workspace" as secondary buttons labelled "Set up terminals", and keep the primary on the next unfinished step.

## C5. Connecting the first machine is three overlapping flows, and the main button is disabled with no reason given

- **Pass:** First-time user
- **Where:** `web/src/lib/MachineSetup.svelte:338, 354, 388` (`disabled={!host.trim()}` on "Continue to authentication →"); `web/src/lib/AddMachineDialog.svelte` ("Add a machine": name, token, run); `web/src/lib/ControlCenter.svelte:51` (terminal setup dialog); `web/src/lib/EnvironmentSync.svelte:121` (setup panel auto-opens when there are no machines); entry points: Control header, first-machine card, Terminals empty state, Machines page. Routes `/`, `/machines#connect-machine`, `/terminals`; both viewports.
- **Problem:** "Connect a machine" leads to a four-step panel (Install, Authenticate, Connect terminal, Inventory & sync). "Add a machine" on the same page opens a different three-step dialog (name, token, run). The Control page offers a third, terminal-only guide. They describe the same job with different steps and different wording. In the four-step panel the install command is already visible but the primary "Continue to authentication →" is greyed out at 50% opacity (white on that fill measures 2.3:1) until a machine name is typed in a field above it. Nothing says the name is required; the helper text beneath the command only says to keep it stable. Counting the actions from a fresh install to a first terminal: create owner, install the CLI, name the machine, create a token, authenticate, consent to the terminal on the host, then start controller access (C4). That is about seven steps across two computers.
- **Fix:** Make the "Add a machine" dialog the single path: Control, Terminals and the Machines empty state all open it (for example `/machines?add=1`) with the name field focused. Keep the four-step panel behind the existing "Prefer the step-by-step setup?" link. Remove the third guide from Control once C4's checklist exists. Replace `disabled={!host.trim()}` with an enabled button that, when the name is empty, focuses the Machine name field and shows "Name this machine to continue" (`aria-describedby`, `role="alert"`). Add a visible "Required" mark on the field.

---

# 2. High impact

High-impact findings create significant friction or lost trust.

## H1. Mobile auth screens: the headline has a typo and the form is below the fold

- **Pass:** Designer + First-time user
- **Where:** `web/src/lib/AuthCard.svelte:10` (`Your AI workspace.<br />A reet good place to work.`) and `:50` (`.headline br { display: none }`), `:46-58` (mobile layout); `/signin`, `/signup`, `/setup`, `/join` at 375.
- **Problem:** Hiding the `<br>` removes the only space between the sentences, so every mobile auth screen reads **"Your AI workspace.A reet good place to work."** This was flagged in the 2026-10-23 audit and is still there. Separately, the signed-out header is 147px tall and the intro block (eyebrow, headline, paragraph, three chips, licence line) comes before the form. Measured at 375x812: on sign-in the first field starts at y=659 and the Sign in button spans y=803-847, so the 812px fold cuts it; setup is 1,290px tall with its submit button at y≈1,080. A returning user has to scroll before they can sign in.
- **Fix:** Write `Your AI workspace. <br />A reet good place to work.` so the space survives. On ≤560px move the whole `.intro` below the form (CSS `order: 2`) or collapse it to the single headline line, so the first field and the button fit in the first viewport.

## H2. Autofocus fires on touch devices, and sign-up is the odd one out

- **Pass:** First-time user
- **Where:** `web/src/lib/AccountSignIn.svelte:31-33`, `web/src/lib/Setup.svelte:21-23`, `web/src/App.svelte:368, 383, 498, 517` (`addEl?.focus()`), `web/src/lib/SignUp.svelte` (no autofocus; the name field is only focused after an error). 375.
- **Problem:** On a phone, sign-in and setup raise the keyboard immediately and cover the page before the visitor has read anything (confirmed: `document.activeElement` is the email or name input on load, and the shelf's add input after setup). Sign-up does not autofocus at all, so the auth forms behave inconsistently. The focus ring on an autofocused field is a double orange ring (border and a 2px outline with offset, `app.css:31,34`) that reads like an error state in the screenshots.
- **Fix:** Wrap the focus calls in `if (matchMedia('(pointer: fine)').matches)`. Make all three forms consistent (focus on desktop only). In `app.css` use either the border colour or the outline on `:focus`, not both, and keep the full ring for `:focus-visible`.

## H3. First-run Control screen has three filled calls to action and a zero-state card that cannot work yet

- **Pass:** Designer + First-time user
- **Where:** `web/src/lib/ControlCenter.svelte:29-32, 34, 47`; `web/src/lib/terminal/ActiveSessions.svelte:39`; route `/`; both viewports.
- **Problem:** With no machines the page shows "Connect a machine" (filled, header), "Open workspace →" (filled, inside a "0 active sessions" card that sits above the machine list) and "Connect your first machine" (filled, centre), plus "Open terminals →", "Terminal setup guide", two "Refresh" buttons, "0 ready for terminals" and four shortcut cards. There are three primary-styled buttons for two destinations, and the most prominent card is a session counter that cannot be anything but zero. On mobile this is 1,650px tall.
- **Fix:** When `machines.length === 0`, render only the C4 checklist and one filled "Connect your first machine" button. Hide `ActiveSessions`, "Open terminals", the refresh buttons, the "0 ready" line and the shortcut grid until the first machine exists. Show them progressively as the checklist completes.

## H4. Navigation: Skills and Setups sit in "More", Notifications is shown twice, and the mobile header is two rows tall

- **Pass:** Designer + First-time user
- **Where:** `web/src/App.svelte:146-163` (nav model), `:757-760` (Notifications pill), `:1276-1277`; both viewports.
- **Problem:** The landing page sells four things: fleet, setups, skills and sync. The top-level nav exposes only Control, Machines and Terminals; Skills, Sync, Lists, Portable setups, Community, Support, Notifications, Teams & organisations and Host skills are nine items under "More". The Notifications link is also a permanent pill in the header (even with zero unread) *and* an item in "More". On mobile the header is two rows (logo, then Menu / Notifications / avatar) and takes about 138px, 17% of an 812px screen, before any content.
- **Fix:** Top level: Control, Machines, Terminals, Skills, Setups. "More": Lists, Community, Teams, Support, Host skills. Move Notifications into the account menu with an unread badge on the avatar. On mobile, put the logo, Menu and avatar on one row.

## H5. The landing page's hero is an HTML mock-up with 7-10px text, not the product

- **Pass:** Designer
- **Where:** `web/src/lib/Welcome.svelte:14-20` and the `.preview-*` CSS at `:40-41` (sizes `.47rem`, `.48rem`, `.49rem`, `.5rem`, `.52rem`, `.6rem`, so 7.5-9.6px); trust row `.67rem` (10.7px); `figcaption` "Illustration" at `.59rem`. 1440 and 375.
- **Problem:** The only proof of the product on the page is a hand-built illustration labelled "Workspace preview" and "Illustration". Its labels (Search, Copy, History, "Close your browser. Pick up here later.") are too small to read, and at 375 they shrink further (down to 6.4px). A visitor deciding whether to install sees a decorative drawing, not the product, and the page offers no other evidence: no real screenshot, no demo, no stars or licence in the hero.
- **Fix:** Replace the mock-up with a real screenshot of the populated Control page and one of a terminal session, using sample data (the `web/concepts/` explorer already has sample data). Keep all visible text at 12px or larger, give it real alt text, and drop the "Illustration" caption. Raise the trust row to 0.8rem.

## H6. Landing page "See what's next →" opens a 164KB internal handoff log

- **Pass:** First-time user
- **Where:** `web/src/lib/Welcome.svelte:35` (link to `https://github.com/eyupio/reet.sh/blob/main/handoff.md`); `handoff.md` is 164,202 bytes in the repo root.
- **Problem:** The page tells visitors that agent installation, updates and removal are "on the roadmap" and sends them to an engineering hand-off document rather than a roadmap. The strip also announces what is *not* shipped directly above the install block.
- **Fix:** Link to a short public `docs/roadmap.md` (five to ten bullets) or the GitHub issues/milestones view, or drop the link and keep the sentence. Move the strip below the install block.

## H7. First-run Skills page has about 13 controls competing at once, and the primary header button leads to a permanent commitment

- **Pass:** Designer + First-time user
- **Where:** `web/src/App.svelte:839-851` (header actions "Distribute skills" and "Create a skill"), `:977-1004` (start card), `:1037-1043` (Shelf tools); `web/src/lib/Hosting.svelte:344`; `/skills` at 1440 and 375.
- **Problem:** On an empty shelf: Distribute skills, Create a skill (filled), Add to shelf (filled but disabled), Add your first skill (filled), three "Add" buttons, Connect this machine, Explore community, "Find a skill on skills.sh", "Host your own skill", the bookmarklet disclosure and "Show bookmarklets and tools" are all visible, and "Ey up. Start here." is the second page to use that same eyebrow. "Create a skill" is the filled header button, and it opens a page whose first step is "This username is permanent" (`Claim username`). A user who is curious about creating a skill is asked for an irreversible choice before seeing what hosting is.
- **Fix:** On an empty shelf keep one filled button ("Add your first skill") plus the three suggestions. Demote the header buttons to links or hide "Distribute skills" until `counts.all > 0`. Hide "Shelf tools" until the shelf has content. Make "Create a skill" a secondary action. On `/hosting`, show what hosting gives you (an example install command and the URL) before the "Choose your publishing username" form, and say what the username is used for.

## H8. Landing promises setups and sync, but their first-run screens are CLI walls or full-power forms

- **Pass:** First-time user
- **Where:** `web/src/lib/SetupProfiles.svelte:212-224` ("No setups yet." with two code blocks and "the synced profile is metadata, not the encrypted snapshot"); `/sync` (`EnvironmentSync.svelte`, Compare & sync); `/profiles` and `/sync` at 1440 and 375.
- **Problem:** Welcome says to "Bring another machine into the fold with a known setup" and "Choose one machine or the fleet". The first visit to Setups shows terminal commands and a caveat about encrypted snapshots and S3. The first visit to Sync shows four dropdowns (Compare from, Skill collection, Destination, Sync mode), a disabled "Scan all" and a pale, disabled-looking "Sync all · preview" with "0 machines ready to scan". Neither page tells the user what to do first, or that a connected machine is a prerequisite.
- **Fix:** When there are no machines, replace both screens with the same empty state: one sentence, "Connect a machine first", and a single filled button that opens the add-machine dialog (C5). Keep the dropdown form and the CLI instructions behind the "has at least one machine" check, and move the S3/metadata caveat into a collapsed "How setups are stored".

## H9. Mobile layout breaks: filter row hides Liked and Select, and the hosted-prefix input is 28px wide

- **Pass:** Designer
- **Where:** `web/src/App.svelte:861-872` (markup) and `:1303` (`.filters { flex-wrap: nowrap; overflow-x: auto }`); `web/src/lib/Account.svelte:328` (Hosted skill name prefix); `/skills` and `/account` at 375.
- **Problem:** The stage filter row is 478px of content in a 343px container, with no fade or arrow. "Liked" starts at x=343 and "Select" at x=432, so they are cut or off-screen (only a "♥" fragment shows). Bulk selection is effectively hidden on a phone. On the Account profile, the custom-prefix text field next to its checkbox renders at **28px wide** (x 312-340), so the field can't be read or used.
- **Fix:** Let the filter row wrap on ≤560px, or move Liked and Select into an overflow menu. On the Account row, stack the checkbox label and the input (`.field { flex-direction: column }` below 560px) with the input at `width: 100%`.

## H10. Skill card: the primary action is "Save to test", and "Start using" does not start anything

- **Pass:** Designer + First-time user
- **Where:** `web/src/lib/shelf.ts:21-31` (`Save to test` primary; `Start using` sets only `used: true`), `web/src/lib/SkillCard.svelte:227-233`; `/skills` after adding a skill, both viewports.
- **Problem:** After adding a skill the card offers Save to test (outlined orange), Start using, Like, Copy install and Copy prompt at about equal weight. "Start using" suggests install but only moves the card to In use. The one action that changes the user's machines, "Copy install", is a grey chip. The page lede now says Sync controls installation, but the button label still contradicts it. This was H7 and H11 in the old audit and is unchanged.
- **Fix:** Rename "Start using" to "Mark in use". On a skill that is not yet installed anywhere, make "Copy install" the filled action and "Save to test" a secondary chip. Move "Copy prompt" behind Expand.

## H11. Explicit Dark theme leaves the warning colour at its light value (2.4:1)

- **Pass:** Designer
- **Where:** `web/src/app.css:16` defines `--warn` for system dark, but `:root[data-theme='dark']` at `:20-24` does not. Used in `SkillCard.svelte:446` ("missing" install badge), `MachineSetup.svelte:559-569`, `AddMachineDialog.svelte:339`, `Ticket.svelte:115`, `Support.svelte:144`, `AdminTickets.svelte:265-285`.
- **Problem:** A user who picks Dark in Account (which sets `data-theme='dark'`) keeps the light-theme `#92400e`. On the dark card that is 2.40:1, on the dark page 2.60:1, so warning and problem states are close to unreadable. The system-dark path is fine (`#fbbf24`, 11:1).
- **Fix:** Add `--warn: #fbbf24;` to the `:root[data-theme='dark']` block. Better, define the dark palette once and reuse it for both selectors so they can't drift.

---

# 3. Nice to have

## N1. Terminology drifts: control room / control centre / workspace; shelf / library; four names for setups

- **Pass:** Designer
- **Where:** "AI control room" (`Welcome.svelte:9`, `ControlCenter.svelte:29`) vs "Your control centre" (`Machines.svelte:22`) vs "workspace" (`AuthCard.svelte`, `AccountSignIn.svelte:76`); nav "Skills", tab title "Your shelf" (`App.svelte:744`), eyebrow "Your skill shelf.", H1 "Your central skill library."; "Portable setups" (`App.svelte:155`), "Setups" (page), "Reusable AI setups" (`ControlCenter.svelte:49`), "Agent setups"; "Sync", "Skills & sync", "Skills & fleet sync", "Compare & sync", "Distribute skills".
- **Problem:** Each pair is plausible alone, but a new user reads them as different things.
- **Fix:** Pick one term for each (suggest "Control", "Skills", "Setups", "Sync") and apply it to nav labels, H1s, tab titles and buttons. Keep the Yorkshire voice in the eyebrows only.

## N2. Public pages have no footer links

- **Where:** `web/src/lib/Footer.svelte:21-42`.
- **Problem:** The footer carries the tagline, version and developer credit. There are no links to docs, GitHub, licence or support on the landing, sign-in or community pages, so trust signals and help are only in the page body.
- **Fix:** Add Docs, GitHub, AGPL-3.0 and Support links when signed out.

## N3. Sign-up and setup ask for a confirm-password field

- **Where:** `web/src/lib/SignUp.svelte:58-59`, `web/src/lib/Setup.svelte:66-67`, `web/src/lib/Join.svelte`.
- **Problem:** Two password fields with a Show toggle add a failure mode and a longer mobile form, and the "At least 8 characters" rule is only a placeholder that disappears on typing.
- **Fix:** Drop the confirm field, keep Show, and show the rule as persistent helper text.

## N4. Success and error copy: full-width banner, raw server text

- **Where:** `web/src/App.svelte:856` (green full-width band: "Reet, added find-skills to the shelf."), `AccountSignIn.svelte:47` (server string "email or password is not correct").
- **Problem:** The success band pushes the toolbar down for six seconds and uses the slug rather than the skill's name (`find-skills`, `pdf`). The sign-in error is lowercase server text. The inline add-error (`That doesn't look like a skill yet…`) is good and is the standard to match.
- **Fix:** Use the existing `Toast` for add and move success. Capitalise the sign-in error and add a "Forgot password?" link to it.

## N5. Notifications settings are a seven-row staff form for a new owner

- **Where:** `web/src/lib/Notifications.svelte`; `/notifications` at 1440 and 375.
- **Problem:** The first visit shows five support-ticket rows, an accounts row and a security row, each with a delivery dropdown and "Your staff role's default" labels, next to an empty inbox.
- **Fix:** Collapse the preferences behind "Choose what you hear about" and show the inbox first.

## N6. Serif font still depends on the system

- **Where:** `web/src/app.css:4`.
- **Problem:** `'Iowan Old Style', 'Palatino Linotype', Palatino, Georgia` falls back to a different serif on Linux and Windows, and the display headings are the main brand element. (Unchanged since the previous audit.)
- **Fix:** Self-host one variable serif with `font-display: swap` and keep the current stack as fallback.

## N7. Theme can only be changed from Account

- **Where:** `web/src/lib/Account.svelte` (Profile); header and account menu.
- **Fix:** Add a light / dark / system toggle in the account menu.

## N8. Shelf loading state is bare text

- **Where:** `web/src/App.svelte:969` ("Loading…"); Control already has a skeleton (`ControlCenter.svelte:37`).
- **Fix:** Reuse the skeleton pattern for the shelf and auth checks.

## N9. Small touch targets

- **Where:** `web/src/lib/PasswordInput.svelte:22` (the Show button is inset 4px top and bottom, measured 56x39 at 375); `ControlCenter.svelte` machine cards (`.btn { min-height: 34px }`).
- **Fix:** Make the Show button fill the input height and use `min-height: 44px` under `@media (pointer: coarse)`.

---

# 4. Status of the 2026-10-23 audit

This table is as the re-audit found it, with the rows updated where they pointed at a finding that has since been fixed. *Fix status* above is the current position for the new findings.

| Old finding | Status now |
| --- | --- |
| C1 no landing page | **Fixed.** `Welcome.svelte` exists. Its calls to action are the new C3. |
| C2 no sign-up or password reset | **Fixed.** `REET_SIGNUP=open`, `SignUp.svelte`, forgot-password and a mailto invite request exist. Invite-only remains the default (C3). |
| C3 first skill below the fold | **Fixed.** Empty stages are hidden in list view. Verified: card at y=488 at 1440 and scrolled into view at 375 (card top 327 after the add). |
| C4 add input clipped on mobile | **Fixed.** The form stacks; the input is 343px at 375. |
| C5 empty shelf has no examples | **Fixed.** Three suggestions with one-click Add. |
| C6 bookmarklets buried | **Partly.** Now a first-run disclosure, still collapsed. |
| C7 vague auth copy | **Fixed.** Copy rewritten for the fleet product; the mobile `<br>` bug went in #197 (H1). |
| C8 anchors styled as `.btn` in default blue | **Fixed.** `.btn` sets `color: var(--ink)`. |
| C9 add button has no busy or validation state | **Fixed.** Disabled when empty, "Adding…" label, inline `role="alert"` error (verified). |
| C10 "Scan this machine" mislabelled | **Fixed in label** ("Connect this machine", guided steps); the flow itself is now C5. |
| H1 navigation front-loads advanced nouns | **Superseded** by H4. |
| H2 "Filters & lists" noisy on first run | **Fixed.** Only shown when there is something to filter. |
| H3 "Shelf tools" always shown | **Still open.** Collapsed, but visible on the empty shelf (H7). |
| H4 three equal CTAs in the empty state | **Mostly fixed** in #205 (H7). The filled buttons are the add form's and "Add your first skill"; Shelf tools still show on an empty shelf. |
| H5 autofocus on mobile | **Fixed** in #197 (H2). |
| H6 raw add-form error | **Fixed.** |
| H7 five equal-weight card actions | **Fixed** in #199 and #205 (H10). |
| H8 mobile filter row hidden | **Fixed** in #199 (H9). |
| H9 search zeroes tab counts | **Still open (source).** `App.svelte:862` counts from the filtered array. Not re-tested in the browser. |
| H10 H1 is a slogan | **Fixed.** "Your central skill library." |
| H11 "Start using" implies install | **Fixed** in #199 (H10): the button is "Mark in use". |
| N1 serif font | **Still open** (N6). |
| N2 pluralisation | **Partly.** Filter summary fixed; other screens not re-checked. |
| N3 skeletons | **Partly.** Control has one; the shelf does not (N8). |
| N4 success banner | **Still open** (N4). |
| N5 mobile auth logo and tagline | **Partly.** The tagline is gone in accounts mode; the logo size is unchanged. |
| N6 duplicate counts | **Still open.** With one skill, the "All 1" tab, the "1 skill" line, the "On the shelf 1" tab and its section heading all show the same count. |
| N7 theme toggle | **Fixed** in #205 (N7). |
| N8 confirm password | **Still open** (N3). |
| N9 double focus ring | **Still open** (H2). |
| N10 Yorkshire voice in errors | **Mostly fine.** Empty-search copy now pairs "Nowt matches just yet." with a literal sentence. Sign-in errors are plain. |

---

## Suggested fix order

1. **C1, C2** first. Both are small, local code changes (CSS positioning; one list entry plus a catch-all route) and they remove the two worst first-impression failures.
2. **C4, C5** next. They are the activation path: a working terminal after install. The checklist and a single add-machine flow also settle H3 and H8.
3. **C3, H5, H6.** Make the landing page convert by state, with a real screenshot and an install command in the hero.
4. **H1, H2, H9, H11.** Mobile and accessibility fixes: the form order and typo, touch-device focus, filter row, prefix input, dark `--warn`.
5. **H4, H7, H10, then N1-N9.** Navigation, first-run Skills page, card actions, naming and polish.
