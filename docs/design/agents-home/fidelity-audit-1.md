# Fidelity audit 1: shipped Home (origin/main 9777bd7, PR #218) against the approved mock-up

Date: 27 Oct 2026. The ruling being applied: every PR must match the approved mock-up 100% at 1440, 834 and 390 px.

**How this was checked.** I built origin/main in an isolated worktree
(`<scratchpad>/wt-fidelity`, detached, `web/node_modules` symlinked) and served it with `vite preview`. Home was rendered with
the `openWorkspace` mock-API helper in dark theme, using fixtures shaped like the mock-up:
- machines Elitebook (linux, CPU 38%), Server (linux, 71%) and Desk (darwin, 6%);
- the five Projects reet, jiggered, zoomies-platform, wireguard.sh and dynamicweb;
- the five In flight sessions with their branches;
- skills checks giving "in sync / 1 out of date / in sync".

Viewports: 1440x900, 1239x900 (collapsed rail), 834x1112 (touch) and 390x844 (touch). I compared the renders with
`ref-home-desktop.png` and `ref-session-desktop.png`, and read the inline styles of every `.dc.html` as the spec. Computed
boxes and type were measured with `getBoundingClientRect` and `getComputedStyle`, and the PNGs were sampled pixel by pixel.

- Render script: `<scratchpad>/wt-fidelity/web/tests/fidelity-render.mjs` (it lives only in the worktree, not the repo).
- Screenshots: `<scratchpad>/fidelity/home-{desktop,1240,tablet,phone}[-full].png`, `terminals-{desktop,phone}.png`,
  `newproject-{desktop,phone}.png`, side-by-side crops `cmp-rail.png`, `cmp-fleet.png`, `crop-*-header-right.png`.
- Measurements: `<scratchpad>/fidelity/metrics.jsonl`.
- `<scratchpad>` = `/tmp/claude-0/-home-user-reet-sh/e3a2f026-c04b-5524-979f-bcb290f6d3eb/scratchpad`.

**Classes.**
- **A**: fixable now with today's data.
- **A\***: also fixable now, but the mock-up and the spec or plan disagree, so the owner should confirm first.
- **B**: needs data a later PR brings (B6, C, E, F, G, H or Program 2).
- **C**: conflicts with a ruling (R10, R12, or "verified"/"healthy"), so it must not be copied literally.

## 0. What already matches (no action)

- **Tokens.** Every dark token equals the mock-up hex exactly: `--bg #14110f`, `--card #1a1613`,
  `--card-raised #1d1815`, `--line-soft #2a241f`, `--line #342d27`, `--line-strong #4a4037`, `--ink #f1ebe4`,
  `--muted #a39a90`, `--accent #c2410c`, `--accent-text #ff8d63`, `--good #7fc79a`, `--warn #e9b44c`, `--info #7fb3d9`,
  `--repo #5fc2a8`.
- **Fonts.** Fraunces 600, IBM Plex Sans 400/500/600 and IBM Plex Mono 400 are self-hosted and applied where the mock-up
  uses them.
- **Desktop header.** 60 px plus a 1 px `#2a241f` rule. The logo is 30 px tall at x=24. The first tab starts at x=172,
  exactly as in the mock-up. The bell is 40x40 (38 + border), radius 9, with the right badge geometry.
- **Columns.** Needs you is 380 px with an 18 px gap. The main padding is 20px 24px 0 and the sections are 18 px apart.
- **Headings.** "In flight" and "Needs you" are Fraunces 20/600 (21 on tablet and phone).
- **Group-by control.** 13 px, padding 5px 10px, active fill `#2a241f`, border `#342d27`, radius 8.
- **Fleet card shell.** Padding 14px 16px, gap 10, radius 13, figure Fraunces 28 (26 on tablet), bar 6 px, radius 3,
  track `#2a241f`, 71% drawn in warn.
- **Row text.** In flight title 14/500, subtitle 12 muted, dot 9 px, age column 34 px wide, 12 px, right-aligned.
- **Launch shell.** Radius 16, border `#4a4037`, background `#1d1815`, margin-bottom 18 at desktop; on phone the margins
  are 0 12px 8px.
- **Bottom tab bar.** 64 px, Home/Sessions/Machines/More, 22 px icons with the same paths, 12 px labels, current tab in
  `#ff8d63`.
- **Tablet fleet cards.** 236 px wide with an 8 px gap. On phone the chips are 44 px pills with an edge fade mask.
- **Order on tablet and phone.** Needs you comes before In flight.

## 1. Cross-cutting deviations

| ID | Mock-up | Shipped | Class | Action |
| --- | --- | --- | --- | --- |
| X1 | `body` has no line-height, so `normal` (≈1.3 for Plex) | `body` is `14px/1.55` | A | This one setting is why rail cards are 69 px instead of 61, In flight rows 60 instead of 57, and fleet cards are taller. Set `line-height: 1.3` on `.home` and on the shell bar. Do not change it globally. |
| X2 | Colours the mock-up uses that have no token: `#110e0c` (bottom tabs, session tab strip, context panel, phone sheet footer), `#3d342d` (rail Add, machine tag, dashed tiles), `#8c837a` (placeholder / launch prompt), `#2b1a11` (avatar) | `--bg`, `--line-strong`, `--muted` and `--tint` stand in | A | Add `--bg-sunk #110e0c`, `--line-mid #3d342d`, `--faint #8c837a` (4.73:1 on `#1d1815`, 5.05:1 on `#14110f`) and `--accent-wash #2b1a11` (`#ff8d63` on it is 7.34:1). Light values: `--bg-sunk #f0eee8`, `--line-mid #d4cfc4`, `--faint #6f6b62` (5.23:1 on `#fffdf9`), `--accent-wash` = `#feefe3` (`--tint`). Extend `theme.test.mjs`. Colours for later PRs ship with those PRs: warn surface `#1f1a0f`/`#6a5420`/`#2a210d` (G, F, H), selected rail card `#201813`/`#6a3b22` (E), Approvals "on" `#2e2822` and slot underline `#6b5f54` (E), REPO badge `#2a5f52` and checkbox `#5a5047` (B5), diff colours (Program 2). |
| X3 | Root font size is 15px on tablet and phone | 14px at every width | A | Use `font-size: 15px` for the shell and Home at ≤1099 px. It drives the row titles, chips and tab labels in the tablet and phone mock-ups. |
| X4 | The reference render has no site footer and no page scroll at 1440x900 | Home grows to 987 px tall: the site footer ("reet.sh test · A REET GOOD HOME…", Sam/Admin/Sign out/Developed by) sits below the dock | A | Hide `<Footer>` on Home, as it already is in the terminal layout. Its links are already in the account menu. |

## 2. Desktop Home (1440x900)

### Shell and top bar
| ID | Mock-up | Shipped | Class | Action |
| --- | --- | --- | --- | --- |
| S1 | `header` gap 28px: search 958–1284, bell 1312–1352, avatar 1380–1416 | gap 12px: Jump 1008–1308, bell 1320–1360, account 1372–1416 | A | At ≥1100 use `.shell-bar { gap: 28px }` and `.top-tabs { margin-left: 0 }`. The logo-to-tab distance stays 28 px. |
| S2 | Search box is 326 px outside (300 content + 12px padding each side + border), 36 px tall, radius 9, gap 8 | 300 px border-box, padding 0 10 | A | `.jump { width: 326px; padding: 0 12px }` at ≥1100. Keep the clamp below 1300 px so it does not crowd the tabs. (At 1239 the placeholder clips to "…machir". There is no mock-up at that width.) |
| S3 | Tabs are 14px; inactive weight 400 in `#a39a90`, current 600 in `#fff`-ink | 14.4px (0.9rem); inactive weight 500 | A | `font: 400 14px var(--sans)`; current tab 600. |
| S4 | Current tab underline: 2px `#ff8d63` across the full tab width (x 172–237), square | Inset 10 px each side (x 182–231), radius 2 | A | `left: 0; right: 0; border-radius: 0`. |
| S5 | Avatar: 36 px solid circle `#2b1a11`, initials `#ff8d63` 13px/600, no ring | 44 px ring (1px `--line` border, `--card` fill, 6 px padding) around a 30 px `--tint` circle; initials 11.2px/700 | A | In `AccountMenu.svelte` at ≥1100: summary 36x36, no border, no padding; avatar fills it with `--accent-wash`, 13px/600. Keep a 44 px hit area on coarse pointers with an invisible outset. |
| S6 | Bell icon in warn colour with a "3" badge | Muted icon with no badge, because nothing needs you yet | B (G) | This is the honest form. The warn colour and badge already switch on when count > 0. |

### Fleet strip
| ID | Mock-up | Shipped | Class | Action |
| --- | --- | --- | --- | --- |
| F1 | Head row: dot, name 600, OS right-aligned. No visible state word | The word "online" is shown after the name in green | A | Make `.status` visually hidden at every width; it is already in the card's accessible name. Offline, reconnecting and paused have their own dot shapes (ring, half, square), so the state is not shown by colour alone. |
| F2 | OS "Ubuntu 24.04", "Debian 12", "macOS 15" | Raw GOOS "linux", "darwin" | A for the name; B for the version, which no PR in the plan provides | Map now: linux → "Linux", darwin → "macOS", windows → "Windows". A distro and version need a connector field (for example `osName`/`osVersion` in capabilities), which no PR in the plan carries. Add it to the plan or accept "Linux". |
| F3 | "3 agents running" | "2 sessions running" | B (B2: `tool` in the session listing; R12 counts sessions started with an agent) | Keep "sessions running" until B2. Switch the label when the listing says which sessions have an agent. |
| F4 | No CPU label: the bar sits alone under the figure | A separate row "CPU ……… 38%" above the bar | C (R12: the bar must be labelled "CPU", grey with "No reading" when there is none) | Do not drop the label. Remove the extra row and put `CPU 38%` (or `CPU No reading`) right-aligned on the figure row's baseline: 12 px, label in `--muted`, value in `--ink`. The card then has the mock-up's 4 rows: head, figure, bar, footer. |
| F5 | Footer: "skills in sync" | "skills in sync · checked 5 min ago" | C (R12: the age is required) | Keep the age, on the footer row after the worktree slot. |
| F6 | Footer starts "4 worktrees" | No worktree figure | B (B2 records it, B6 shows it) | The honest form is to omit it, never show 0 (R12). The footer then holds only the skills line, which is what ships now. |
| F7 | Card is 127 px tall (ref y 81–208) | 160 px | A (F4 + X1) | Expect about 127 px after F4 and line-height 1.3. |
| F8 | Card ≈299 px wide (3 cards plus a 152 px tile in the wider main column) | 309 px | A (follows R1) | Fixed by R1. |
| F9 | Desk is idle (0 agents): grey dot `#a39a90` and grey bar at 6% | Green "online" dot and green bar | A\* | The mock-up colours the dot and bar by activity; the spec colours the dot by connection. Owner call. Proposal: keep the dot for connection and use `--muted` for the bar when `sessionsRunning === 0`. |
| F10 | Add a machine tile: 150px content-box (152 outside), dashed `#3d342d`, 20 px plus icon, 14 px muted | 150 px, dashed `--line-strong #4a4037` | A | `flex-basis: 152px`; border `--line-mid`. |

### In flight
| ID | Mock-up | Shipped | Class | Action |
| --- | --- | --- | --- | --- |
| I1 | One flat list of 5 rows, with no group headings, although Group by is set to "Project" | Group headings inside the list ("jiggered 1", 12px/600, .04em, padding 8 16 4) and dividers between groups | A\* | The spec says "Groups are headed by the project, machine or state with a count", but the PNG shows none. Owner call. Proposal: keep the headings but style them as the mock-up's section label (11px/600, `letter-spacing:.09em`, uppercase, `--muted`, padding 10px 16px 2px) with no extra divider. |
| I2 | A 110 px progress slot: "Step 4 of 7" (11px muted, 3 px gap) over a 4 px coloured bar | Right-aligned block up to 40% wide: word (13px/500, coloured) over source (11px muted) | C (R10: no step count or bar for terminal sessions; show the state word and its source) | Do not copy the step or bar. Use the mock-up's slot: a fixed **118 px** column (the mock-up's 110 plus 8, because "from the session's state" measures 117 px at 11 px), left-aligned. Line 1: the word, 11px/500 in its tone. Line 2: the source, 11px `--muted`, in place of the bar. The slot stays put, so G and Program 2 only swap the content. |
| I3 | Row padding 11px 16px, gap 14, height 57 | Padding 10px 16px, height 60 | A | Padding 11px 16px plus X1. |
| I4 | Subtitle is all sans: "reet · Elitebook · reet/store-split" | The branch is in IBM Plex Mono 12 | A\* | The spec says "(mono branch)"; the PNG shows sans. Owner call. The plan's B6 tweak assumes mono. |
| I5 | Amber dot and "Waiting for you"; blue dot and "Needs sign-in" | Only Running (green), Exited 0 (grey), Exited n (red) and Connection lost (warn) | B (G: Waiting for you, from the bell; Program 2: sign-in) | Present states already use the mock-up's tones. |
| I6 | YOLO badge: 10px/600, .08em, `#e9b44c` on a `#6a5420` border, radius 4, padding 2px 6px | None | B (F4) | |
| I7 | Row hover `filter: brightness(1.12)` | Hover fills `--card-raised` | A (minor) | Optional: `.row:hover { filter: brightness(1.12); background: none }`. |

### Needs you
| ID | Mock-up | Shipped | Class | Action |
| --- | --- | --- | --- | --- |
| N1 | Count pill right-aligned (`h2 { flex-grow: 1 }`) at x 1391–1416 | Pill directly after the heading (x 1143) | A | `.needs h2 { flex-grow: 1 }` at ≥1100 only. The tablet and phone mock-ups keep it next to the heading, as shipped. |
| N2 | Solid accent pill "3" (12px/600, padding 2px 9px) | Outlined muted "0" | B (G) | Honest at 0; the solid style already applies when count > 0. |
| N3 | Three cards: "Wants to run a command" (warn card `#1f1a0f`/`#6a5420`, mono command block, Allow once/Deny/Open 36 px), "Needs you to sign in" (Open the terminal, 36 px full width) and "Ready to review · 1h" (title 500, "6 files changed · reet · Elitebook") | Empty card "Nothing needs you." | B: approval and sign-in cards are Program 2 (R11); Waiting, Exited with an error and Ready to review are G | The empty form is honest. For G, cards use padding 13px 15px, radius 13, gap 6, kind line 12px/600 in the kind's tone with "· age", title 14/500, meta 12 muted. Buttons are 36 px, radius 8; the primary is `--accent`, the secondary has a 1px `--line-strong` border. G's tweak should give the empty card the same 13/15 padding. |

### Launch (docked)
| ID | Mock-up | Shipped | Class | Action |
| --- | --- | --- | --- | --- |
| L1 | Two-row docked sentence bar, 117 px tall. Row 1: "Describe the job. Say what done looks like." 16px `#8c837a`, plus **Start** with a `Ctrl ↵` hint (11px mono, .8 opacity). Row 2 (border-top `#2a241f`, 14px): "Run **Claude Code** with **sonnet** on **reet** at **Elitebook** from **main** in **a fresh worktree**", slots 600 ink with a dashed `#6b5f54` underline, then "Approvals" and an Ask me / YOLO segment (on `#2e2822`) | One row, 66 px: the muted sentence "Start an agent in a new terminal session on one of your machines." and a **Start an agent** button | B (E1/E2, catalogue from C, project slot from B, the YOLO half from F) | Keep the button until E. Note for E: the bar is 51 px taller, so In flight and Needs you lose that space at 1440x900 (see the plan's E tweak). |
| L2 | Start button radius 10, height 40, padding 0 18. Shadow `0 -8px 30px rgba(0,0,0,.25)`. Padding 14px 18px 8px | Radius 8 (`.btn`). Shadow `0 -8px 30px -12px color-mix(--bg 80%)`. Padding 12px 12px 12px 18px | A | `.start { border-radius: 10px }`. Add a shadow token `--shadow-dock: 0 -8px 30px rgb(0 0 0 / .25)` (light: `0 -8px 30px rgb(29 28 26 / .08)`); components may not use literal colours. |

## 3. Desktop rail (and its collapsed form)

| ID | Mock-up | Shipped | Class | Action |
| --- | --- | --- | --- | --- |
| R1 | `aside width:272px` is content-box, so the render is **301 px** wide (272 + 14×2 padding + 1 border). Ref: border at x=300, cards x 14–285 (272 wide), main starts at 325 | Grid column 272 px in total, cards 243 px, border at x=271, main at 296 | A | `.home.writer { grid-template-columns: 301px minmax(0,1fr) }`. The PNG decides under the ruling; note in the spec that "272 px" means card width. Everything to the right moves 29 px and the fleet cards narrow to ≈299. |
| R2 | Card row 1: diamond 14, name 600 (`flex-grow:1`), machine tag pill on the right (11px, `#a39a90`, 1px `#3d342d`, radius 99, padding 1px 8px) | The tag is on row 2, on the left | A | Move the tags into `.top` after the name. The name gets `flex-grow:1` with an ellipsis. |
| R3 | Row 2: `main` (12 mono muted), state in its colour (gap 10), "3 worktrees" right-aligned | Row 2 is tag(s), then the state right-aligned | A for the layout; B for branch and worktrees (B6: `baseBranch`, worktree records from B2) | Now: row 2 = state only, left-aligned (`margin-left:0`), 12px. The branch and worktree slots render nothing until B6, never a placeholder figure. |
| R4 | Gap between cards 12px | 8px (`ul { gap: 8px }`) | A | `gap: 12px`. |
| R5 | Card 61 px tall | 69 px | A (R2 + X1) | |
| R6 | Add button and tag border `#3d342d` | `--line-strong #4a4037` | A | Use `--line-mid` (X2). |
| R7 | The chosen card ("reet") has border `#6a3b22` and fill `#201813` | No chosen state | B (E: a card sets the launch bar's project) | |
| R8 | State words "needs sign-in" (info) and "waiting" | "N running", "run in progress", "idle" | B (G / Program 2) | |
| R9 | Foot: 16 px monitor icon, gap 10, padding 10px 12px, dashed `#3d342d`, radius 11, 13 px muted, one line "Folders are browsed live on the machine" | No icon, three lines: "These are your saved Projects. Browsing a machine's folders comes in a later release." | A for the shape; B (B6) for the copy | Add the icon and flex row now and keep the honest copy. B6 swaps in the mock-up sentence. |
| R10 | Diamond = repository (`--repo`); folders get a folder icon | Diamond on every legacy Project, including multi-repo ones | B (B6) | |
| R11 | Card choice sets the launch bar's project | Card opens `/projects/<id>` (legacy) | B (B6 / E) | |
| R12 | Rail border runs the full height | At 1100–1239 px (56 px strip) the right border stops where the content ends (y≈377), because `.rail { align-self: start }` and the strip has no min-height | A (bug) | `.strip { min-height: calc(100dvh - 61px) }`, or let the rail stretch and make the inner part sticky. |

## 4. Tablet Home (834x1112)

| ID | Mock-up | Shipped | Class | Action |
| --- | --- | --- | --- | --- |
| T1 | Header: logo, 4 tabs, spacer, **bell only** (44x44, border `#342d27`, radius 10) | Adds a bordered 44 px search button and the 44 px account avatar | A\* | Owner call. The mock-up has no Jump and no account on tablet. Proposal: move "Jump to…" and the account entries (profile, theme, sign out) into the More sheet at ≤1099, keeping `/` as the shortcut. |
| T2 | Header gap 20, tab padding 0 14, tabs 15 px | Gap 12, tab padding 0 12, 14.4 px | A | `gap: 20px`; `a { padding: 0 14px; font-size: 15px }` at 651–1099. |
| T3 | Logo 30 px tall (≈120 px wide) at every width | 104 px wide (≈26 px tall) at ≤1099 | A | `.home :global(.logo) { width: 120px }` at every width. |
| T4 | Fleet card: no OS, unit "agents running" 13 px | Unit 14 px, word "online" shown | A (F1) / B (F3) | `.unit { font-size: 13px }` at ≤1099. |
| T5 | Exactly the three machine cards, no Add tile, no fade | The Add a machine tile is cut off at the right edge (x 768–918 in an 834 viewport) | A | Hide `.add` at 651–1099 to match the mock-up (Add a machine stays on Machines), or add the phone edge fade. I recommend hiding it. |
| T6 | "In flight" heading only, with **no Group by** | 44 px Group by segments | A\* | Owner call. The mock-up moves grouping to the Sessions screen (H). Proposal: hide `.group-by` at ≤1099, keeping the stored choice. |
| T7 | Row: min-height 60, padding 8px 16px. Right side is one 12 px muted string ("Step 4 of 7", "Waiting for you"). No age | Two-line word and source plus age | A for age and size; C for the source (R10) | At ≤1099: one line `word · source`, 12px; the word in its tone, the source muted; no age column. |
| T8 | Subtitle is "reet · Elitebook" with no branch | Branch shown in mono | A\* | Hide `.branch` at ≤1099. This matches both mock-ups and the phone drawing in the spec. |
| T9 | Group headings: none | Present | A\* | Same as I1. |
| T10 | Launch bar: prompt row (16 px, min-height 44, Start 44, padding 0 22, radius 10), then a sentence row (14 px, min-height 44, Approvals 44 px tall); margin 0 24px 20px | One-row dock, Start 44, radius 8, bottom margin 18 | B (E) for the content; A for the radius (10) and bottom margin (20) | |

## 5. Phone Home (390x844)

| ID | Mock-up | Shipped | Class | Action |
| --- | --- | --- | --- | --- |
| P1 | Header 56: logo (30 px tall) with `flex-grow`, then the **bell only**: 44x44, **no border**, 22 px icon, badge `top:4px; right:4px` | Logo 104 wide, bordered search button, bordered bell with an 18 px icon, 44 px avatar | A\* (search and avatar, as T1); A (bell style) | Bell at ≤650: `border: 0`, `svg 22px`, badge `top:4px; right:4px`. |
| P2 | Chips: name 500, then "3 running" (13 px muted) | Name 600, then "2 sessions running" | A | Chip copy "N running" (honest: no "agents"); name weight 500. |
| P3 | Only machine chips | Also an Add a machine chip at the end of the scroller | A | Hide on phone, as T5. |
| P4 | `h2 "In flight"` with margin-top 4, no Group by | Group by segments | A\* | As T6. |
| P5 | Row min-height 56, padding 8px 14px, gap 12. Right side 12 px muted ("4 of 7", "Waiting") | Word 13px/500 in its tone; source visually hidden (still read out); no age | A for size; C for the step text | Word 12px. R10 also says the source is shown. The visually hidden source predates this audit; consider `word · source` on one line if it fits at 390, otherwise accept and record it in `docs/ui-verification.md`. |
| P6 | Launch: one-line prompt "Describe the job…" 16 px `#8c837a`, a round 44 px Start (up arrow; the spec and plan say 48x48), and a summary line "Claude Code on **reet** at Elitebook · main · fresh worktree" with **Change** (`#cfc6bc`, 500) | Full-width 48 px "Start an agent" | B (E) | Margins already match. |
| P7 | Bottom tab bar background `#110e0c`; inactive labels weight 400; current tab = colour plus 600 weight | `--bg #14110f`; inactive 500; extra 28x3 px accent bar along the top of the current tab | A for background and weight; A\* for the bar | `background: var(--bg-sunk)`; `font-weight: 400`. The bar was added so state is not colour alone, but the weight already does that job. Owner call on dropping it. |
| P8 | No Needs you cards; "Needs you to sign in" row with `›` | Empty card | B (G / Program 2) | |
| P9 | Rail not on Home: reached from the Sessions tab (PhoneRail) | Rail is `display:none` at ≤1099 and not reachable from Home at all; Projects only through More › Projects | B (B6: rail under In flight until H; H1/H3: PhoneRail on Sessions) | |

## 6. Phone rail (PhoneRail.dc.html), which PR H builds

The current phone Sessions tab is `/terminals`, the terminal workspace with its drawer (`terminals-phone.png`). Every
item below is **B (H1/H3)**, with folder-project data from B6:
- **Header.** 56 px: back 44 (`#cfc6bc`, chevron 22) with margin-left -10; "Sessions" in Fraunces 21/600 with `flex-grow`; **New**
  44 px primary (padding 0 14, radius 10).
- **Search.** 44 px, radius 10, `--card`, 16 px glass, "Search sessions".
- **Switcher.** Project / Machine / State, radius 10, 14 px; each segment padding 12px 0 (about 44 px), the active one
  `#2a241f` at 600.
- **Groups.**
  - Group header: diamond 14 in `--repo`, name 600 with `flex-grow`, machine pill 12 px (`#3d342d`).
  - Group card: radius 13, `--card`.
  - Rows: min-height 52, padding 6px 14px, gap 12, dot 9, title 15 px with ellipsis, age 12 muted.
- **Foot.** "PROJECTS · 11" label (11px/600, .09em, uppercase), then **Add a project**: 44 px, bordered `#4a4037`, radius
  10, 500.
- **Tab bar.** Sessions is current.
- **R-H check.** `/sessions` replaces `/terminals` (R17), and the switcher reuses `home.ts` grouping.

## 7. Session screen (Session.dc.html / PhoneSession.dc.html) against today's Terminals page, for PR H

Today's `/terminals` at 1440 (`terminals-desktop.png`) shows:
- the compact 56 px app bar;
- a framed "Terminals" panel with a back arrow, Find, Sessions 4, New session, and Details & help;
- a 280 px left sidebar: machine filter, New agent session, search, Sort/Group selects, sessions grouped by machine;
- a session row with title, `dev@Elitebook`, status dot and "Reconnecting", Retry now and Manage;
- a toolbar (Search, Copy, Paste, Interrupt, Commands, Scrollback, Appearance, History…);
- the pane, and a footer reading "Input paused…".

What the mock-up has that the page lacks:

| ID | Mock-up element (exact values) | Today | Class / note |
| --- | --- | --- | --- |
| H1 | The full **60 px** app bar with Sessions current | Compact 56 px bar | B (H2): the session screen uses the standard header at ≥1100. |
| H2 | **Tab strip**: 46 px, `background #110e0c`, border-bottom `#2a241f`, padding 0 24, `align-items:flex-end`, gap 6. Each tab is 36 px, padding 0 14, radius 9 9 0 0, 8 px dot, nowrap. The current tab has fill `#14110f`, a `#2a241f` border with none at the bottom, and `#f1ebe4` text; the others are transparent and `#a39a90`. A **+** button, 40x36 | None (the sidebar list does this job) | B (H2, R20): at most 8, held in memory; close the tab, never the session; + goes to Home with the launch bar focused. |
| H3 | **Header**: 62 px, padding 0 28, gap 14. Title Fraunces 21/600. Subline 13 muted "Claude Code · sonnet · started 41 minutes ago". State pill: 30 px, padding 0 12, radius 99, warn fill `#2a210d`, border `#6a5420`, `#e9b44c` 13/500, 8 px dot. **Chat \| Terminal** segment: border `#342d27`, radius 9, padding 7px 16px, active `#2a241f` at 500 | Title 600 sans + `dev@host` + status dot word + Retry/Manage | B (H2). The agent and model need the B2 listing; "started …" can use `createdAt` today. The pill vocabulary comes from G (Waiting) and from session state now. Chat is disabled with "Chat arrives with structured sessions." (R21); Terminal is selected; the YOLO badge comes from F. |
| H4 | **Conversation**: a 740 px centred column, line-height 1.6. User bubble: max 540, `#241f1b`, radius 14 14 4 14. Plan checklist with 20 px numbered/✓ discs. Edit card with diff (`#12281b`/`#a8e0bc`, +84/−0). Decision card (warn surface, "Run a command in your worktree", "Runs code on Elitebook" pill, mono command block 13 px, **Allow once** with key `A` and **Deny** with key `D` at 42 px, "Asking every time. Approvals are logged.") | Terminal pane | Program 2. In Program 1 the pane stays (spec). H should hold the 740 px column only for chat; the terminal fills the column. |
| H5 | **Composer**: 740 wide, border `#4a4037`, radius 14, `#1d1815`. "Reply to the agent, or interrupt it…" 15 px `#8c837a`. Footer row (border-top `#2a241f`, 13 px muted): "Approvals: Ask me" and a 34 px round send button (`#2a241f`/`#756c63`) | Footer "Input paused · …" and Focus terminal | Program 2 for the composer; **B (H2)** for the "Approvals: Ask me / YOLO" footer text under the pane (spec). |
| H6 | **Context panel**: **340 px** (the spec says 320; the PNG is the truth, so use 340), `background #110e0c`, border-left `#2a241f`, padding 20, section gap 18. Labels 11/600, .09em, uppercase, muted, 8 px below. Cards radius 11, `--card`, padding 12px 14px, 13 px | None (the right side is all terminal; Details & help is a dialog) | B (H2), section by section below. |
| H6a | *Where it's working*: Machine "Elitebook ● online" (good), Project, Branch (mono), Worktree "own copy" (mono, muted); label/value rows with `space-between` | Data exists today for machine, branch and worktree path; project needs B2 | B (H2) |
| H6b | *Changed files* with a count on the right; rows padding 9px 14px, 12.5 px mono with ellipsis, **+84/−0** in `#7fc79a`/`#e5867a` | Worktree status exists (paths) | B (H2). **C for the +/− counts (R19)**: list paths only, at most 20, with the total; no line counts until Program 2. |
| H6c | *Skills on this machine*: "✓ 12 skills in sync with your list", **Manage** muted | `fleetCards` skills line | B (H2). **C (R12)**: use the R12 skills line with its age or "Not checked", never a bare "in sync". |
| H6d | *Decisions in this session*: rows "Allowed git status 38m", "Waiting go test … now" | None | Program 2 for the rows. **C (R11/R21)**: Program 1 says "Asked in the terminal.", or the YOLO line from F. |
| H7 | **Panel collapse**: hidden behind **Details** under 1100 px (side sheet on tablet) | Not applicable | B (H2/H3) |
| H8 | **Phone session**: 52 px header, padding 0 8. Back 44 (`#cfc6bc`). Title 600 with ellipsis over "reet · Elitebook" 12 muted. State pill 28 px, padding 0 10, 12/500, 7 px dot, "Waiting". Overflow ⋯ 44 px. Decision card pinned above the composer, with 48 px **Allow once** (flex) and 96 px **Deny**. Composer: radius 16, min-height 48, padding 6 6 6 14, 16 px input, 44 px round send. With the keyboard up, all site chrome is hidden | Compact 54 px bar with search/bell/⋯/avatar, then a Show sessions / New / Help bar, a status row, a toolbar and extra keys | B (H3) for the slim header, state pill and hidden chrome (`keyboardChrome('session')`); Program 2 for the decision card and chat composer. Program 1 keeps the terminal pane and its extra-keys row. |
| H9 | No left sessions sidebar on the session screen: sessions are reached through the tab strip and `/sessions` | 280 px sidebar (A6 docking) | B (H1/H2). Decide in H whether A6's docked panel survives on the session screen. The mock-up has none. |

## 8. Add project (AddProject.dc.html / PhoneAdd.dc.html) against the legacy New project dialog, for PR B5

The legacy dialog (`newproject-desktop.png` and `newproject-phone.png`) has:
- the "PROJECTS" eyebrow, "New project" in Fraunces and a two-line lede;
- a text button "Close ×";
- Name, Default agent and Default model, Shared prompt, the "Give each run its own git worktree" checkbox card,
  Repositories 0 of 12 with Add a repository, and **Save Project**.

Every difference below is **B (B5)**.

- **Dialog.** 640 wide, top 28, height 844 (it fills the viewport). Fill `#1a1613`, border `#342d27`, radius 14, shadow
  `0 24px 60px rgba(0,0,0,.5)`. Overlay `rgba(0,0,0,.62)`. It is built on the shared shell; do not edit the shell.
- **Head.** Padding 20px 24px 12px.
  - Eyebrow "Projects": 11/600, .08em, uppercase, `#ff8d63`.
  - Title "Add a project": Fraunces 24/600, margin-top 2.
  - **Close**: a 44x44 icon button with an 18 px X, 1px `#342d27`, radius 8. Today it is a text button.
  - No lede paragraph.
- **Body.** Padding 0 24, gap 14. Each label is 13 px muted, 6 px above its control.
  - **Machine**: 42 px select row, fill `#14110f`, radius 8, an 8 px state dot, "Elitebook · online" (the
    "· online" part muted) and a 14 px chevron.
  - **Folder**: a 42 px input (mono 13, placeholder `/path/to/repo` muted) plus a **Hide browser** / **Browse** button,
    42 px, padding 0 16, 500.
  - **Browser panel**: border `#342d27`, radius 10, fill `#14110f`.
    - Breadcrumb row: padding 10 14, 13 px `#cfc6bc`; `~` muted; `/` in `#5d656d`; "14 folders" 12 muted on the
      right; border-bottom.
    - Filter row: a 36 px filter field (mono 13, "Type to filter…") and "Show hidden folders" with a 16 px checkbox
      (`#5a5047`, radius 3).
    - List: **300 px** tall, rows 44 px, padding 0 14, gap 12. Icons 16 px: a folder path in muted, or the diamond in
      `#5fc2a8` for a repository. Names mono 13, 600 for a repository. **REPO** badge: 10/600, .08em, `#5fc2a8`,
      border `#2a5f52`, radius 4, padding 2px 6px. The selected row is filled `#2a241f`.
    - Footer: "/root/reet **is a git repository**" (mono 13, `#cfc6bc`; the phrase in `--repo`) and **Use this
      folder**: 36 px, 1px `#ff8d63`, text `#ff8d63`, 500.
  - Then two columns: **Name** "optional" (placeholder "Defaults to the folder name") and **Worktree for each session**
    ("Use the account default"). The spec adds a collapsed Starter prompt and More defaults; the mock-up omits them, so
    B5 must decide whether they sit collapsed under this row (the spec wins on content, the mock-up on shape).
- **Footer.** Padding 16px 24px, border-top `#2a241f`. Note "The machine decides whether it can open each folder."
  (13 muted, `flex-grow`), **Cancel** (44, padding 0 20, border `#342d27`, 500) and **Add project** (44, padding 0 24,
  primary).
- **Phone sheet.**
  - Header: 60 px, padding 0 8 0 16. Eyebrow 10 px; h1 Fraunces 21; close 44 with a 20 px icon and no border.
  - Machine row 48 px, radius 10. Breadcrumb 14 px, min 36. Filter 48 px (mono 14). Checkbox 22 px, radius 5, row
    min 44.
  - List rows min 52, 18 px icons, mono 14.
  - **Pinned footer**: `#110e0c`, padding 12 16 16. Path (mono 13, `#e2dad1`) and "git repository" (`--repo`).
    **Cancel** 110x48 and **Add this project** flex at 48 px.
- **Copy conflict.** The phone says "Add this project" and the desktop and spec say "Add project". Owner call.
- **Not to copy.** The AddProject mock-up's background shows an older shell: a 288 px aside with "New session" and
  "Projects (11)". That conflicts with Main's top-tab shell, so it is not a target. The dialog opens over Home.
- **Removed fields (spec R2, B4).** Default agent, model and shared prompt move to "More defaults" or the starter
  prompt. The repositories list goes away, because a project is one folder.

## 9. Counts

Home screens and rail only (sections 1–5 and 3; P9 counted once):

| Class | Count | IDs |
| --- | --- | --- |
| A | 37 | X1–X4, S1–S5, F1, F2 (name), F7, F8, F10, I3, I7, N1, L2, R1, R2, R3 (layout), R4, R5, R6, R9 (shape), R12, T2, T3, T4, T5, T7 (age/size), T10 (radius/margin), P1 (bell), P2, P3, P5 (size), P7 (bg/weight) |
| A\* (owner call before fixing) | 8 | F9, I1/T9, I4, T1/P1 (search + avatar), T6/P4, T8, P7 (marker bar), Add project button copy (§8) |
| B | 20 on Home and rail | S6, F2 (version: no PR in the plan), F3 (B2), F6 (B2/B6), I5 (G/P2), I6 (F), N2 (G), N3 (G/P2), L1 (E/C/B/F), R3 data (B6), R7 (E), R8 (G/P2), R9 copy (B6), R10 (B6), R11 (B6/E), T4 unit (B2), T10 content (E), P6 (E), P8 (G/P2), P9 (B6/H). Also the phone rail screen (§6, H) as one item, 9 session-screen items (§7, H/Program 2) and the Add project dialog (§8, B5) |
| C | 4 on Home, 3 on the session screen | Home: F4 (CPU label kept, R12), F5 (skills age kept, R12), I2 (no step or bar, R10), T7/P5 (source shown, R10). Session screen: H6b +/− counts (R19), H6c bare "in sync" (R12), H6d decision rows (R11/R21) |

No copy says "verified" or "healthy" (`home.e2e.mjs` asserts this); nothing to fix there.

## 10. Top 10 visible gaps (by how much they change the picture at first glance)

1. **Launch bar.** A one-row "Start an agent" dock instead of the two-row launch sentence (B, E). It is the largest
   visual difference at every width.
2. **Rail width.** 272 instead of 301 px; cards are 243 wide instead of 272, and the main column sits 29 px left (A, R1).
3. **Fleet cards.** 160 px tall with an extra "CPU ··· %" row and a visible "online" word, against 127 px and 4 rows
   (A + C: F1, F4, F7).
4. **In flight rows.** Group headings and right-aligned two-line coloured state words instead of the flat list with the
   110 px step slot (A\*/C: I1, I2).
5. **Header right side.** 12 px gaps and a ringed 44 px avatar instead of 28 px gaps and a solid 36 px `#2b1a11` disc;
   the Jump box is 300 instead of 326 (A: S1, S2, S5).
6. **Rail cards.** The machine tag is on the second row, cards are 8 apart and 69 px tall, and there is no branch or
   worktree line; the mock-up has the tag on row 1, 12 px gaps and 61 px cards (A + B: R2–R5).
7. **Tablet and phone chrome.** Extra search button and avatar, a bordered phone bell, a 104 px logo, and a Group-by
   control the mock-up does not show (A/A\*: T1, T3, T6, P1, P4).
8. **Site footer under Home.** The page scrolls to 987 px at 1440x900 (A: X4).
9. **Tablet fleet strip.** The Add a machine tile is cut off at the right edge (A: T5). On phone, the chips read
   "2 sessions running" in 600 instead of "3 running" in 500 (A: P2, P3).
10. **Line-height and missing surface tokens.** Body 1.55 against the mock-up's `normal` (≈1.3) inflates every card and
    row. The tab bar, tab strip and panels lack the `#110e0c` "sunk" surface (A: X1, X2, P7).

## 11. Proposed "fidelity polish" slice (all class A; one small PR right after PR B, or folded into B6)

Owner-call (A\*) items are left out unless the owner approves them. Each task lists exact targets. Verify by re-running
`tests/fidelity-render.mjs` against the reference PNG.

**Task P-1: tokens and line-height.**
- Add these tokens to `app.css`, `theme.ts` (TOKENS) and `theme.test.mjs` (AA pairs): `--bg-sunk` #110e0c / #f0eee8,
  `--line-mid` #3d342d / #d4cfc4, `--faint` #8c837a / #6f6b62, `--accent-wash` #2b1a11 / #feefe3, and
  `--shadow-dock: 0 -8px 30px rgb(0 0 0 / .25)` (light `rgb(29 28 26 / .08)`).
- Line-height and size:
  - `.home, .shell-bar { line-height: 1.3 }`, keeping multi-line prose at 1.55.
  - At ≤1099 the shell and Home use a base `font-size: 15px`.
- Hide `<Footer>` on `/` (the Home route).

**Task P-2: top bar (desktop, tablet, phone).**
- Desktop (≥1100):
  - `.shell-bar { gap: 28px }` and `.top-tabs { margin-left: 0 }`.
  - Jump: `width: 326px`, `padding: 0 12px`; keep the clamp under 1300 px.
  - Tabs `font: 400 14px var(--sans)`, current 600.
  - Underline 2px `--accent-text`, `left: 0; right: 0; radius 0`.
  - Avatar: 36x36 disc, fill `--accent-wash`, `--accent-text`, 13px/600, no ring or padding. Keep a 44 px hit area on
    coarse pointers.
- Tablet (651–1099): `gap: 20px`; tabs `padding: 0 14px; font-size: 15px`.
- All widths: logo `width: 120px` (30 px tall).
- Phone (≤650): bell `border: 0`, icon 22 px, badge `top: 4px; right: 4px`. Bottom tabs: `background: var(--bg-sunk)`
  and inactive label weight 400.

**Task P-3: rail.**
- `.home.writer { grid-template-columns: 301px minmax(0,1fr) }`, so cards are 272.
- `ul { gap: 12px }`.
- Card row 1: diamond, name (`flex-grow:1`, ellipsis), machine tag pill(s) on the right. Tag: 11 px, border
  `--line-mid`, padding 1px 8px.
- Row 2: the state word, left-aligned, 12 px (`--good` when running); leave room for B6's mono branch and its
  right-aligned worktree count.
- Add button border `--line-mid`.
- Foot: flex row, gap 10, padding 10px 12px, 16 px monitor icon (`rect x3 y5 w18 h12 rx2` + `M8 21h8M12 17v4`, stroke
  1.8), dashed `--line-mid`, radius 11, 13 px muted. Keep today's honest copy; B6 swaps in "Folders are browsed live on
  the machine".
- Strip (1100–1239): `min-height: calc(100dvh - 61px)`, so the border runs the full height.
- Target: card 61 px tall, main column starting at x=325 at 1440.

**Task P-4: fleet strip.**
- `.status` becomes visually hidden at every width (the accessible name keeps the word; the dot shapes carry
  offline/reconnecting/paused).
- OS display names: linux → "Linux", darwin → "macOS", windows → "Windows".
- R12-compliant CPU label: remove the `.cpu` row and render `CPU 38%` / `CPU No reading` right-aligned on the figure
  row's baseline (12 px, label `--muted`, value `--ink`). The card is then head, figure, bar, footer with gap 10,
  ≈127 px tall at 1440.
- Add a machine tile `flex: 0 0 152px`, dashed `--line-mid`.
- Tablet: hide the Add tile; `.unit { font-size: 13px }`.
- Phone: hide the Add chip; chip text "N running" with the name at weight 500.
- Update `home.e2e.mjs` expectations: the `.cpu` locator moves, the accessible name is unchanged, and `/agents?
  running/` stays absent.

**Task P-5: In flight, Needs you and the dock.**
- Desktop rows: `padding: 11px 16px` (57 px with P-1).
- State block in the mock-up's progress slot (R10-safe): `flex: none; width: 118px; text-align: left`. Line 1: the word
  at 11px/500 in its tone. Line 2: the source at 11px `--muted`. It sits between the title column and the 34 px age.
- At ≤1099:
  - one line `word · source` at 12 px;
  - no `.age`;
  - `.branch` hidden;
  - phone word at 12 px.
- Needs you: `h2 { flex-grow: 1 }` at ≥1100, so the count pill sits at the right edge.
- Dock:
  - `.start { border-radius: 10px }`;
  - `box-shadow: var(--shadow-dock)`;
  - bottom margin 20 px on tablet.
- If the owner approves the A\* items: restyle the group headings as section labels (11/600, .09em, uppercase,
  padding 10px 16px 2px, no extra divider), and hide `.group-by` at ≤1099.

Not in the slice:
- Every B item: the launch sentence (E), the rail's branch, worktrees and menu (B6), the cards (G), and the session
  screen and phone rail (H).
- Every C item. The C resolutions are already part of P-4 and P-5.
- The F2 OS version: it needs a connector field that no PR in the plan owns. Either add it to the plan (a small
  `osName`/`osVersion` capability) or accept "Linux" / "macOS".
