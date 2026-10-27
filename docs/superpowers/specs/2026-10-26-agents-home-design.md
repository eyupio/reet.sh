# Agents home (S8 design)

Date: 2026-10-26. Status: approved by the product owner, aligned with the approved clickable mock-up;
open questions settled as rulings below. Plan: `docs/superpowers/plans/2026-10-26-agents-home.md`.

reet.sh gets a new app shell and a new home. Top tabs (**Home, Sessions, Machines, Skills,
Activity**) with a bell carrying the attention count replace today's navigation for writers. **Home**
is a mission-control page: a fleet strip of machine cards, an **In flight** list of sessions, a **Needs
you** column, a project rail on the left, and a docked launch bar that reads as a sentence. A project
stops being a named set on its own page and becomes one folder on one machine, picked by browsing
that machine's real folders. **Sessions** holds the open sessions, each with a context panel that only
reet can fill (machine, project, branch, worktree, changed files, skills). This design covers the whole
pass and how to ship it in pull-request-sized steps. Structured chat over the Agent Client Protocol is
designed here only as far as the shell needs it; it is Program 2 with its own spec.

"The reference designs" means the screenshots the owner supplied earlier (an agent dashboard and a
session manager). "The mock-up" means the approved clickable design: nine boards (desktop Home,
desktop session, Add project, YOLO confirmation, tablet Home, and phone Home, Sessions, session with
the keyboard up, and Add project). The mock-up is the source of truth for layout, copy and tokens;
where it shows something this pass cannot honestly produce (step counts, command approvals and
sign-in detection for terminal agents), the [rulings](#rulings) say what ships instead and when the
rest arrives. Nothing in this repository names the products behind the reference designs.

## Intent and success

**Outcome.** A signed-in writer opens reet.sh and lands on Home. They see every machine at a glance,
what is in flight and what needs them. They add a project by browsing a machine's folders, fill the
launch sentence (agent, model, project, branch, worktree), describe the job and press **Start**, and
the session opens with its context beside it. The bell and Needs you say which sessions want them,
across all machines.

**Who it is for.** The account owner and writers, on a desktop, a tablet or a phone, with machines that
are enrolled and consented to browser terminals. Viewers and the single-token and open modes get a
read-only Home (see [rulings](#rulings), R16).

**Success looks like.**

- Starting work is one sentence and one press, from a project the person chose by looking at the
  machine's folders, not by typing a remembered path.
- The pages work with a thumb on a 320 px phone with the keyboard open and without the browser zooming
  out, on an 834 px tablet with 44 px targets, and on a 1360–1440 px desktop.
- The app follows the person's theme setting: the dark palette is the designed default, the light
  palette passes the same contrast and layout checks.
- Nothing on screen claims more than happened. "Started" still means the connector started the program
  in tmux; attention states say where they come from and never guess; no step count is shown for an
  agent that does not report steps.
- The controller still never sends a shell string, a binary, a flag or a git command. Everything new on
  the wire is a closed, validated field the machine re-checks.
- YOLO is never on unless an owner or admin allowed it for the account, the machine did not refuse it,
  and the person chose it for that one session after the confirmation dialog.

## Decisions already taken with the product owner

These are recorded, not reopened.

| Question | Decision |
| --- | --- |
| Where do projects live? | In the project rail on Home (and the Sessions screen on phones). There is **no separate Projects page**. |
| What is a project? | A folder on a machine, chosen by browsing the machine's folders at runtime, with automatic detection of whether it is a git repository. |
| Layout | The approved mock-up: top tabs with a bell, mission-control Home, session screen with a context panel, tablet and phone layouts as drawn. |
| YOLO | Adopted, **off by default**, chosen per session at launch only (Approvals: *Ask me* or *YOLO*), confirmed in a dialog, a visible YOLO badge on that session everywhere, every use audited. An account setting **Allow YOLO**, off by default, that only owners and admins can switch on. A machine can refuse. In structured (ACP) sessions reet itself answers each permission request with `allow_once` (each one audited, never `allow_always`); terminal-only agents use their own allowlisted option. No free-form flags or command lines. |
| Chat transcripts | Stored **on the machine only**. The controller relays live events and fetches history on demand; history is unavailable while the machine is offline. |
| Agents | A built-in, connector-side catalogue (allowlisted, never command lines from the controller), each preset with its safest permission default. New feature token `agents-v2`, because an older controller rejects a connection that lists an unknown tool id (`Capabilities.Validate`). |
| Folder browse and branches | Negotiated requests `browse-v1` (stat-only, folders only) and `branches-v1` (one read-only git command), specified below. |
| Structured chat | The Agent Client Protocol (stdio JSON-RPC) through a small runner inside tmux (Program 2). For Claude Code and Codex, which need adapters, reet offers a **confirmed, audited install of a pinned adapter version** (owner or admin), with detection and the exact command shown; never silent. |
| Program order | Program 1 (this design's plan): shell and Home first, then browse, projects, catalogue, launch bar, YOLO, attention, session screen (re-sliced at the owner's request on 2026-10-26: "do D sooner, and then each PR afterwards tweaks the redesign as required"; see [Delivery](#delivery-pull-request-slices-and-order)). Program 2: structured chat for Claude over ACP. Then more agents. |
| Attention | Working, quiet, waiting for input, needs sign-in, finished, failed, with a cross-machine Needs you list and the bell, are part of this pass (sign-in and exact states arrive with ACP). |

## Rulings

The owner delegated the open questions. Each ruling is recorded as *ruling — why — cost if wrong*.
R1–R9 were given; R10–R22 were made while aligning the spec with the mock-up.

**R1. Folder projects replace multi-repo sets.** Ruling: a project is one folder on one machine. Start
all and per-run worktrees become **Start in several projects**: a multi-select in the project rail
feeding the same run orchestration (`internal/projectrun`). `/projects` and `/projects/<id>` redirect
to Home. Saved Projects migrate: each member becomes a folder project, de-duplicated by machine and
folder; run records are kept read-only and listed in Activity, with their worktrees still checkable and
removable. — Why: the owner does not want a second page, and multi-select keeps every safety property of
runs. — Cost if wrong: named groups come back as a saved selection in the rail (one table, one menu);
nothing migrated is lost meanwhile.

**R2. Shared prompts become starter prompts.** Ruling: a folder project gains an optional **starter
prompt** (same rules as an opening prompt, at most 2,000 bytes). Migration copies each Project's shared
prompt into every folder project created from it. Where one folder belonged to several Projects with
different prompts, the oldest Project's prompt is kept and the folder project's notice lists the other
Projects' names so the person can find them in the read-only legacy record (kept one release). The
launch bar never fills a starter prompt by itself: an empty prompt box shows **Use the starter prompt**.
— Why: no loss, and a prompt is never sent without the person seeing it. — Cost if wrong: one optional
column and one button to remove.

**R3. First-cut catalogue.** Ruling: `claude`, `codex`, `opencode`, `gemini` (shipped) plus `pi`,
`cursor`, `cline`. Every other agent on the owner's list ships **disabled** until its flags are confirmed
from the real CLI's help text. Cline's preset passes `--auto-approve false`. — Why: each preset is a
security boundary and must be confirmed, not guessed. — Cost if wrong: the person sees a disabled
agent with "reet doesn't start this agent yet." until its preset lands.

**R4. Control folds into Home and Machines.** Ruling: the Control overview's session band is replaced by
In flight, its connection cards by the fleet strip and Machines. `/control` redirects to `/`. — Why: it
duplicated both. — Cost if wrong: Control's quick links return as a section on Home.

**R5. YOLO gates.** Ruling: a machine accepts YOLO sessions by default; `reet terminal yolo off` on the
machine refuses them. The account's **Allow YOLO** setting is **off** by default. — Why: the account
setting is the deliberate gate; the machine switch lets a machine's owner opt out locally. — Cost if
wrong: flip the machine default to refuse and add `reet terminal yolo on`; a one-line change and a
release note.

**R6. Theme.** Ruling: follow the person's theme setting (System, Light, Dark). Dark is the designed
default palette; the light palette is derived from reet's existing light tokens in `web/src/app.css`. —
Why: the setting already exists and people use it. — Cost if wrong: forcing dark is one attribute on
the shell.

**R7. Project limit.** Ruling: 100 projects per account (409 `project_limit`). — Why: generous and
bounded. — Cost if wrong: a constant and a test.

**R8. Enter on phones.** Ruling: Enter inserts a newline on phones and touch tablets; the **Start**
button sends. On a hardware keyboard Ctrl+Enter (Cmd+Enter on macOS) sends and Enter adds a line. —
Why: phone keyboards have no reliable modifier and a stray Enter must never start an agent. — Cost if
wrong: one key handler.

**R9. Notifications outside the page.** Ruling: out of scope for Program 1: the bell, Needs you and the
page title count only. — Why: exact states arrive with ACP; notifying on coarse terminal states would
cry wolf. — Cost if wrong: Program 2 adds them through `docs/notifications.md`.

**R10. No invented progress.** Ruling: the In flight row's step progress ("Step 4 of 7" and its bar)
appears only for structured sessions that report a plan (Program 2). Terminal sessions show the state
word and its source ("Working · terminal activity") with no bar. — Why: terminal output carries no
steps; a bar would claim progress nobody measured. — Cost if wrong: none; the row keeps its slot.

**R11. Needs you in Program 1.** Ruling: the cards that ship are **Waiting for you** (the program rang
the terminal bell), **Exited with an error**, and **Finished** (titled **Ready to review** for worktree
sessions, with the changed-file count from the existing worktree status, fetched once when the card
first appears). The **command approval** card (command, Allow once, Deny, Open) and **Needs you to sign
in** need structured sessions and arrive in Program 2; the column's layout already holds them. — Why: a
terminal agent's prompt cannot be answered safely from outside its terminal. — Cost if wrong: none;
the cards are additive.

**R12. Fleet card figures.** Ruling: the load bar is the machine's CPU percentage from the existing
health report, labelled "CPU", grey with "No reading" when absent; "agents running" counts live
sessions started with an agent; the worktree count is worktrees reet created on that machine and has
not removed; the skills line uses the existing sync state ("skills in sync" only when the last check
found no differences, with its age; "Not checked" otherwise). Home ships first (PR D) on today's API, so
the worktree figure appears only where an existing record says "created by reet and not removed"; without
one the card omits it (never a zero) until PR B records it. — Why: every figure already exists and is
labelled with what it measures. — Cost if wrong: a different load figure needs a connector field.

**R13. Navigation.** Ruling: desktop shows the five tabs; Setups, Sync, Lists, Community, Support, Teams
& organisations and Host skills move to the account menu under **More**, joined by Control and the
legacy Projects page until PR H and PR B retire their addresses. Tablet and phone show four
tabs, **Home, Sessions, Machines, More**, with Skills and Activity inside More. **Activity** is the
existing account activity list at `/activity`. — Why: the mock-up's tabs, without losing a page. — Cost
if wrong: a navigation array.

**R14. Wire gating.** Ruling: the new launch fields `Base`, `Effort` and `Yolo` all ride `agents-v2`
(not `branches-v1`), so a connector that negotiated an earlier token is never sent a field its strict
decoder would reject. YOLO support is a separate capability `yolo` (omitempty, requires `launch`),
absent when the machine refuses or before the YOLO release; the controller sends `Yolo` only to a peer
reporting it. Attention fields ride a new token, `attention-v1`. — Why: each token is introduced in the
pull request that teaches both ends to understand it. — Cost if wrong: none at runtime; only more
tokens.

**R15. Fonts.** Ruling: Fraunces (600) and IBM Plex Sans and Mono are served by reet itself as subset
WOFF2 files under their open font licences; no request goes to a third-party font host. — Why:
self-hosted installs must not leak page views. — Cost if wrong: about 150 KB of static files.

**R16. Read-only Home.** Ruling: viewers, single-token and open modes get Home with the fleet strip only
(no In flight, Needs you, project rail, launch bar or bell). — Why: they cannot open terminals. — Cost
if wrong: a role check.

**R17. Session routes.** Ruling: `/sessions` lists sessions; `/sessions?agent=…&session=…` opens one.
`/terminals` redirects there keeping its query. — Why: the mock-up's Sessions tab; every existing link
keeps working. — Cost if wrong: route names.

**R18. The machine follows the project.** Ruling: in the launch sentence, *at [machine]* is set by the
chosen project and is only choosable for **Home folder on…**. — Why: a project is one folder on one
machine. — Cost if wrong: none.

**R19. Changed files without line counts.** Ruling: the context panel lists changed paths from the
existing worktree status (at most 20, with the total). Added and removed line counts (+/−) arrive with
Program 2's tool events. — Why: line counts need a new git command on the machine. — Cost if wrong: a
`diff --numstat` read under a later token.

**R20. Session tabs.** Ruling: the strip of open-session tabs shows the sessions opened in this browser
tab since it loaded, at most 8 (oldest dropped), never stored. — Why: tabs are a view, not data. — Cost
if wrong: per-account remembered tabs in local storage.

**R21. Chat | Terminal.** Ruling: the switch ships in Program 1 with **Terminal** selected and **Chat**
disabled, explained "Chat arrives with structured sessions." — Why: the layout is stable before Program
2. — Cost if wrong: the switch is hidden until Program 2.

**R22. One release train for the Projects hand-over.** Ruling: PR B (which retires the Projects page and
Start all) to PR E (which brings back starting in several projects) merge in order with no release
tagged between them. Amended 2026-10-26 for the shell-first order: PR D now lands before PR B, so the
train holds only PR C and PR E (two reviews, down from three); the hold does not disappear, because
moving PR D earlier does not change when Start all leaves or returns. Every other merge, PR D first
among them, may be released on its own: PR D keeps the legacy Projects page, New project and Start an
agent dialogs reachable. — Why: the requested order still removes Start all before its replacement
lands. — Cost if wrong: a release holds for two reviews; if that is too long, PR B's retirement step
(B6) moves into PR E and the hold disappears.

### Still open

- **Which agents beyond the first cut, in what order?** Each needs its flags confirmed on a real
  machine; the owner orders the queue.
- **When to drop the legacy Projects tables.** Planned one release after migration; confirm before the
  drop pull request.

## Out of scope

Program 2 (the ACP runner, adapter install, the chat view, command approval and sign-in cards, plan
checklists, line counts, transcripts); resuming earlier conversations; pull requests, merging or
pushing; git-remote project identity; folders outside the enrolled home; Windows; notifications
outside the page (R9); MCP exposure of projects or sessions; agent settings screens; changing the
security/PIN preference or the legacy sync flows; any change to the shared dialog shell another change
is building (this design uses it).

## Visual identity

Warm ink, one cinnabar accent, a bookish heading face. Defined once as tokens in `web/src/app.css`;
components use tokens only, never literal colours.

| Token | Dark (designed default) | Light (derived from today's light tokens) | Use |
| --- | --- | --- | --- |
| `--bg` | `#14110f` | `#f7f6f2` | page, header |
| `--card` | `#1a1613` | `#fffdf9` | cards, rail items, panels |
| `--card-raised` | `#1d1815` | `#fffdf9` with `--shadow` | launch bar, dialogs |
| `--line-soft` | `#2a241f` | `#e8e4db` | row dividers, header rule |
| `--line` | `#342d27` | `#dedbd2` | card borders |
| `--line-strong` | `#4a4037` | `#c9c3b6` | buttons, launch bar border |
| `--ink` | `#f1ebe4` | `#1d1c1a` | text |
| `--muted` | `#a39a90` | `#5f5c54` | secondary text |
| `--accent` | `#c2410c` | `#c2410c` | filled buttons, badges, active tab rule |
| `--accent-text` | `#ff8d63` | `#c2410c` | accent text and links |
| `--accent-ink` | `#ffffff` | `#ffffff` | text on `--accent` |
| `--good` | `#7fc79a` | `#15703b` | working, in sync |
| `--warn` | `#e9b44c` | `#92400e` | waiting, YOLO, out of date |
| `--info` | `#7fb3d9` | `#1d5f8a` | needs sign-in, informational |
| `--repo` | `#5fc2a8` | `#0f766e` | repository icon and REPO badge |
| `--bad` | today's | today's | failed, refused |

Headings (page titles, section titles like *In flight*, fleet figures) use **Fraunces 600**; body and
controls **IBM Plex Sans** 400/500/600; paths, branches, commands and keyboard hints **IBM Plex Mono**
(self-hosted, R15). The header carries the real logo, `web/src/lib/logo.svg` (the Yorkshire rose in a
bookmark plus the wordmark), drawn with `--ink` and `--accent`. Every colour pair used for text passes
WCAG AA in both palettes; state is never colour alone (a word or icon always accompanies a dot).

**Voice.** `docs/voice-and-copy.md`: plain and exact everywhere; Yorkshire only in headings, ledes and
success lines (for example the empty launch box's heading "What shall we get stuck into?", "Sorted.
Project added."). Buttons, states, refusals and the YOLO dialog are plain.

## Information architecture and navigation

**Top bar** (60 px desktop): logo; tabs **Home, Sessions, Machines, Skills, Activity** (the current one
underlined in `--accent-text`); **Jump to a session, project or machine** (a search box, `/` focuses
it); the **bell** with the Needs you count; the account menu (initials) holding **More** (R13), the
theme setting and Sign out. Tablet and phone layouts are below.

**Routes.**

| Address | After this design |
| --- | --- |
| `/` | Home (writers in accounts mode); read-only Home for everyone else signed in (R16); the welcome page signed out. |
| `/sessions` | Session list: search, group by Project, Machine or State, filters. |
| `/sessions?agent=…&session=…` | The session screen for that session. |
| `/terminals` | Redirects to `/sessions` keeping `agent` and `session`. |
| `/?project=<id>` | Home with that project chosen in the launch bar (used by redirects and the rail). |
| `/projects`, `/projects/<id>` | Client-side redirect to `/` with a one-time note "Projects now live in the rail on Home." `/projects/<id>` maps to the first folder project migrated from it. `projects` stays a reserved hosted username. |
| `/control` | Redirects to `/` (R4). |
| `/activity` | The account activity list, including the read-only legacy runs (R1). |

## Screens

### Desktop Home (1100 px and wider)

```
+------------------------------------------------------------------------------------------------+
| [rose] reet.sh  Home  Sessions  Machines  Skills  Activity   [ Jump to a session... ]  (bell 3) PJ |
+--------------------+---------------------------------------------------------------------------+
| PROJECTS    [+ Add]| +--------------+ +--------------+ +--------------+ +- - - - - - -+        |
| +----------------+ | | o studio     | | o server     | | o desk       | | + Add a      |        |
| |<> reet [studio]| | | 3 agents run | | 1 agent run  | | 0 agents     | |   machine    |        |
| | main  2 running| | | CPU [===   ] | | CPU [=====  ]| | CPU [      ] | +- - - - - - -+        |
| |    3 worktrees | | | 4 worktrees  | | 2 worktrees  | | 0 worktrees  |                         |
| +----------------+ | | skills synced| | 1 skill old  | | skills synced|                         |
| |<> notes[studio]| | +--------------+ +--------------+ +--------------+                         |
| | main  waiting  | |                                                                           |
| +----------------+ | In flight     Group by [Project|Machine|State]    Needs you (3)            |
| | ...            | | +----------------------------------------+ +----------------------------+ |
| |                | | | o Refactor the store layer        12m  | | Waiting for you . 2m       | |
| |                | | |   reet . studio . reet/store-split     | | Add browse-v1 protocol     | |
| |                | | |   Working . terminal activity          | | reet . studio       [Open] | |
| |                | | | o Speed up the runner pool [YOLO] 34m  | +----------------------------+ |
| |                | | |   site . server . reet/pool            | | Ready to review . 1h       | |
| |                | | | o Docs pass   Finished            1h   | | Docs pass . 6 files changed| |
| |                | | +----------------------------------------+ +----------------------------+ |
| |- - - - - - - - | | +-----------------------------------------------------------------------+ |
| | Folders are    | | | Describe the job. Say what done looks like.    [Start  Ctrl+Enter]    | |
| | browsed live on| | |-----------------------------------------------------------------------| |
| | the machine    | | | Run Claude Code with sonnet on reet at studio from main in a fresh    | |
| +----------------+ | | worktree                              Approvals [Ask me | YOLO]       | |
+--------------------+-+-----------------------------------------------------------------------+-+
```

- **Project rail** (272 px): heading *Projects* with **Add**; one card per project: diamond icon in
  `--repo` for repositories (folder icon otherwise), name, machine tag, branch (mono), state in words
  (*2 running*, *waiting*, *idle*, *machine offline*), worktree count. Choosing a card sets the launch
  bar's project. **Select** (in the heading's menu, or long-press on touch) turns cards into checkboxes
  for **Start in several projects** (R1). Each card's menu: New session here, Edit, Worktrees, Remove
  from reet. The foot reads "Folders are browsed live on the machine". The rail scrolls on its own; it
  collapses to a 56 px strip under 1240 px wide.
- **Fleet strip**: one card per enrolled machine in the person's machine order, then **Add a machine**
  (opens the existing Add a machine dialog). A card: status dot and word in its accessible name
  (*online*, *offline*, *reconnecting*), name, OS, the agents-running figure in Fraunces, the CPU bar
  (R12), worktree count, skills line. Choosing a card opens that machine on Machines. More cards than
  fit scroll sideways with edge fades.
- **In flight**: heading, **Group by** *Project | Machine | State* (remembered per account in this
  browser). Each row: state dot plus word, title (one line, ellipsis, full text in the accessible
  name), `project · machine · branch` (mono branch), a **YOLO** badge when on, progress only per R10,
  age since the last state change. Groups are headed by the project, machine or state with a count.
  Finished sessions stay for 24 hours or until opened, then leave In flight (they remain on Sessions).
  Choosing a row opens the session screen.
- **Needs you** (380 px): count badge; cards newest first per R11. Each card has its kind and age on the
  first line in the kind's colour **and** words, the session title, `project · machine`, and actions
  (**Open**; for Finished, **Mark seen**). Empty: "Nothing needs you." under the heading.
- **Launch bar** (docked at the bottom of the main column): see [The launch bar](#the-launch-bar).

### Desktop session screen

```
+--------------------------------------------------------------------------------------------------+
| [rose] reet.sh   Home  Sessions  Machines  Skills  Activity                         (bell 3)  PJ  |
+--------------------------------------------------------------------------------------------------+
| (o Refactor the store..) (o Add browse-v1 protocol) (o Review code) (o Speed up the..)   [+]       |
+------------------------------------------------------------------+-------------------------------+
| Add browse-v1 protocol                    o Waiting for you      | WHERE IT'S WORKING            |
| Claude Code . sonnet . started 41 minutes ago   [Chat|Terminal]  | Machine   studio  o online    |
|------------------------------------------------------------------| Project   reet                |
|                                                                  | Branch    reet/browse-v1      |
|   live terminal pane (TerminalPane, unchanged behaviour)         | Worktree  own copy            |
|                                                                  |-------------------------------|
|                                                                  | CHANGED FILES            3    |
|                                                                  | internal/terminalproto/brow.. |
|                                                                  | internal/terminalhost/brow..  |
|                                                                  | docs/terminal-launch.md       |
|                                                                  |-------------------------------|
|                                                                  | SKILLS ON THIS MACHINE        |
|------------------------------------------------------------------| v 12 skills in sync  Manage   |
| toolbar and extra keys as today          Approvals: Ask me       |-------------------------------|
|                                                                  | DECISIONS IN THIS SESSION     |
|                                                                  | Asked in the terminal.        |
+------------------------------------------------------------------+-------------------------------+
```

- **Tab strip**: open sessions (R20), each with its state dot and title; the current one raised; **+**
  returns to Home with the launch bar focused. Middle-click or the tab's close button closes the tab
  (never the session).
- **Header**: title, `agent · model · started …`, the state pill with its source in the tooltip, a
  **YOLO** badge when on, **Chat | Terminal** (R21), and the existing actions menu (Rename, Theatre, End
  session, Copy branch, Check and remove worktree).
- **Conversation column**: in Program 1, the live terminal pane with today's toolbar and keys. The
  mock-up's conversation (user message, plan checklist, tool and edit cards with a diff, the permission
  decision card with **Allow once** / **Deny**, keyboard hints **A** / **D**, "Asking every time.
  Approvals are logged.") is Program 2's chat view; its tokens and spacing are defined now so it drops
  in without restyling. Under the pane the footer shows **Approvals: Ask me** or **Approvals: YOLO**.
- **Context panel** (320 px, collapsible, hidden under 1100 px behind a **Details** button):
  *Where it's working* (machine with online state, project, branch, worktree: "own copy" or "the
  checkout"); *Changed files* (R19; worktree sessions only, refreshed on open and by a button, never
  polled); *Skills on this machine* (the R12 skills line, **Manage** opens Skills filtered to that
  machine); *Decisions in this session* ("Asked in the terminal." for terminal agents; YOLO sessions
  say "YOLO: reet answers yes, this once, to each request." and link to the audit filter in Activity).

### Add project dialog

Built on the shared dialog shell (`shell` action from `web/src/lib/dialogShell.ts`). Desktop: centred,
up to 640 px wide. Phone: full-screen sheet (`sheet: 'full'`) with the chosen folder and buttons pinned
at the bottom.

```
+-------------------------------------------------------------+
| Projects                                                (x) |
| Add a project                                               |
| Machine   [ o studio . online                        v ]    |
| Folder    [ /root/reet                      ] [Hide browser]|
| +---------------------------------------------------------+ |
| | ~ / root                                    14 folders  | |
| | [ Type to filter...          ]  [ ] Show hidden folders | |
| |  [] archive                                             | |
| |  <> blog                                        REPO    | |
| |  <> reet                                        REPO  < | |
| |  <> site                                        REPO    | |
| |  [] scratch                                             | |
| | /root/reet is a git repository       [Use this folder]  | |
| +---------------------------------------------------------+ |
| Name (optional)            [ Defaults to the folder name ]  |
| Worktree for each session  [ Use the account default   v ] |
| Starter prompt (optional)  > (collapsed)                    |
| The machine decides whether it can open each folder.        |
|                                    [Cancel]  [Add project]  |
+-------------------------------------------------------------+
```

- **Machine** lists ready machines that report `browse`; others are disabled with their sentence.
- **Folder** can be typed; **Browse** (labelled **Hide browser** while open) opens the browser at the
  typed folder or the home. Clicking a folder enters it; the breadcrumb goes back (never above the
  home). **Use this folder** fills Folder and collapses the browser. Typing a path does not list it;
  leaving the field checks it with one browse request to show whether it exists and is a repository.
- Repository folders get the diamond icon in `--repo`, bold names and a **REPO** badge. The count reads
  "14 folders" ("200+ folders" with more pages; **Show more** loads the next page).
- A non-repository folder is allowed: "This folder isn't a git repository, so sessions there can't use
  worktrees." A folder inside a repository but not its top: "This folder is inside the repository at
  ~/work/reet. Use the top folder for worktrees." with a button to choose it.
- **Name** defaults to the folder's name. **Worktree for each session**: *Use the account default*,
  *Yes*, *No*. **Starter prompt** (collapsed): R2. Default base branch, agent and model sit under **More
  defaults** (collapsed).
- **Add project** is enabled when a machine and an absolute path are set. Refusals keep the dialog open
  and move focus to the field. Success: "Sorted. reet added." in the status line, the dialog closes,
  the new project is chosen in the launch bar.

### YOLO confirmation dialog

Choosing **YOLO** in Approvals opens an `alertdialog` (shared dialog shell) before the control moves:

```
+-------------------------------------------------------------+
| /!\ Start this session on YOLO?                             |
| The agent will not wait for you. reet says yes, this once,  |
| to every permission request it makes, including running     |
| commands and changing files on studio.                      |
|  - Every request it answers is written to the audit log,    |
|    one line each.                                           |
|  - The session wears a YOLO badge wherever it appears, for  |
|    everyone who can see it.                                 |
|  - It can't be switched on part-way through. To stop it,    |
|    end the session.                                         |
|  - Secrets in that folder are readable to the agent. Use a  |
|    throwaway worktree if unsure.                            |
| v Your account allows YOLO              set by an owner     |
| v studio accepts YOLO sessions       checked a moment ago   |
| v New worktree and branch for this session  reet/yolo-...   |
|                          [Keep asking me] [Start with YOLO] |
+-------------------------------------------------------------+
```

- The first sentence is per agent kind: for structured sessions (Program 2) as drawn; for terminal
  agents "The agent will not ask before running commands and changing files on studio. reet passes
  its own skip-the-prompts option and records the launch." The audit bullet becomes "The launch is
  written to the audit log." for terminal agents (reet cannot see their individual actions).
- The checklist states each gate with its source. A failed gate shows a cross and its reason, and
  **Start with YOLO** is disabled: "An owner or admin can allow YOLO for this account in Settings.",
  "studio doesn't accept YOLO sessions.", "This agent has no option reet can use for that.", "Update
  reet on that machine to use YOLO." The worktree line is a recommendation, not a gate: without a
  worktree it reads "Works in the checkout on main" with a warning icon.
- **Keep asking me** (default focus) closes it with Approvals on *Ask me*. **Start with YOLO** starts the
  session at once (it is the launch, not a toggle). Approvals returns to *Ask me* after every launch and
  is never remembered.

### Tablet (651–1099 px; drawn at 834 px)

```
+----------------------------------------------+
| [rose]  Home  Sessions  Machines  More (bell)|
+----------------------------------------------+
| (o studio 3) (o server 1) (o desk 0) -->     |  machines scroll sideways
| Needs you (3)                                |
| +------------------------------------------+ |
| | Waiting for you . 2m                     | |
| | Add browse-v1 protocol . reet . studio   | |
| |                                  [Open]  | |
| +------------------------------------------+ |
| In flight                                    |
| o Refactor the store layer   Working         |
| o Speed up the runner pool   Working [YOLO]  |
|                                              |
+----------------------------------------------+
| Describe the job. Say what done looks like.  |
|                                     [Start]  |
| Run Claude Code on reet at studio from       |
| main            Approvals [Ask me | YOLO]    |
+----------------------------------------------+
```

Needs you comes before In flight; the project rail is reached from **Sessions** (the phone Sessions
screen at tablet width) and from the launch sentence's project slot; the launch bar is docked; every
target is at least 44 px. The session screen hides the context panel behind **Details** (a side sheet).

### Phone (650 px and narrower, or sideways 500 px tall or less)

```
 Home                              Sessions                         Session, keyboard up
+---------------------------+    +---------------------------+    +---------------------------+
| [rose]            (bell 3)|    | [ Search sessions       ] |    | < Add browse-v1  o Waiting|
| (o studio 3)(o server)>   |    | [Project|Machine|State]   |    |   reet . studio           |
| Needs you (3)             |    | reet          studio      |    |                           |
| +-----------------------+ |    |  o Refactor the store 12m |    |   terminal pane           |
| | Waiting for you . 2m  | |    |  o Add browse-v1      2m  |    |   (today's phone          |
| | Add browse-v1 protocol| |    |  o Docs pass          1h  |    |    terminal)              |
| | reet . studio         | |    | notes      server         |    |                           |
| | [       Open        ] | |    |  o Review code        9m  |    | [extra keys row]          |
| +-----------------------+ |    | Projects . 11             |    +---------------------------+
| In flight                 |    | [   Add a project      ]  |    |        keyboard           |
| o Refactor the st. Working|    |                           |    |                           |
+---------------------------+    +---------------------------+    +---------------------------+
| Describe the job...  (^)  |    | Home Sessions Machines More|
| Claude Code on reet at    |    +---------------------------+
| studio . main . fresh     |
| worktree       [Change]   |
+---------------------------+
| Home Sessions Machines More|
+---------------------------+
```

- **Bottom tab bar**: Home, Sessions, Machines, More (64 px, safe-area padded); the bell sits in the
  slim top bar. (This said 56 px; the approved mock-up and the shipped bar are 64 px, so the text follows them.)
- **Home**: machine chips scroll sideways; Needs you first with thumb-size buttons (48 px, full width
  for the primary action); In flight below; the launch bar sits above the tab bar as a one-line prompt
  with a round Start button (48 × 48 px) and the sentence collapsed to one summary line with **Change**,
  which opens a bottom sheet holding the sentence's slots as large rows plus Approvals.
- **Sessions**: search, the Project | Machine | State switcher, groups with 44 px rows, then *Projects ·
  11* with **Add a project** and the project cards (multi-select via long-press).
- **Session**: a slim header (back, title, `project · machine`, state). With the keyboard up the site
  chrome (top bar and tab bar) steps aside and the terminal uses the usable-height rule of the phone
  terminal (`docs/terminal-workspace.md`, Phones). In Program 2 the decision card is pinned above the
  composer, as drawn in the mock-up.
- **Add project**: a full-screen sheet; machine, breadcrumb, filter, show hidden and the folder list
  scroll; the chosen folder ("/root/reet · git repository") and **Cancel** / **Add this project** stay
  pinned.
- Inputs use a 16 px font so focusing never zooms; nothing lays out wider than the device.

## The launch bar

The launch bar is a form that reads as a sentence above (desktop) or below (tablet, phone summary) a
prompt box:

> Run **[agent]** with **[model]** on **[project]** at **[machine]** from **[branch]** in **[a fresh
> worktree]** · thinking **[effort]**

Each bold slot is a button (dashed underline) that opens a listbox popover on desktop and a bottom
sheet on touch. Slots, each prefilled from the last launch so a second session is one press:

1. **Agent** from the catalogue: installed agents first; others disabled with the reason ("Not
   installed on studio", "Update reet on that machine", "reet doesn't start this agent yet.").
2. **Model**: the agent's suggestions plus *Other…* (the launch model rules); *default* sends nothing.
3. **Project**: registered projects grouped by machine with search; *Add a project…* opens the dialog;
   *Home folder on…* starts without a project. Unready machines are listed disabled with their
   sentence. With a multi-selection in the rail the slot reads "3 projects".
4. **Machine**: follows the project (R18).
5. **Branch**: with a worktree, the base branch, filled from the project's `baseBranch`, else the
   account default, else the repository's default from `branches-v1`, else the current branch; the
   picker lists the machine's branches with a filter. Without a worktree the slot is read-only:
   "Works in the checkout on main." reet never switches the branch of a checkout. A non-repository
   project hides the branch and worktree slots. With several projects: "each project's default".
6. **Worktree**: *a fresh worktree* or *the checkout*, defaulting from the project, then the account.
7. **Thinking** (effort): shown only for agents whose preset has a confirmed effort option (`low`,
   `medium`, `high`; *default* sends nothing).

**Prompt.** "Describe the job. Say what done looks like." Optional, at most 2,000 bytes, the existing
launch rules (no leading `-`, two words for agents that need it). R8 for Enter. "Secrets don't belong
here. The prompt is visible in the machine's process list." shows under the box when it has text. The
starter prompt chip per R2.

**Approvals** *Ask me | YOLO*: *Ask me* is selected and resets after every launch; choosing *YOLO* opens
the [confirmation dialog](#yolo-confirmation-dialog). When YOLO is unavailable the *YOLO* half is
disabled with the reason in its description.

**Start** (with the hint `Ctrl ↵`, `⌘ ↵` on macOS) is enabled once a project or home folder and an agent
are chosen. While the request is in flight the bar is locked and its status line reads "Starting Claude
Code on studio…"; on success the session opens on the session screen and the prompt clears. The
session title defaults to the prompt's first line (at most 80 characters), shown before sending as
"Titled “Fix the failing tests…”".

**Mapping onto the wire.** One `Create` per session through the existing idempotent create path:
`Tool`, `Model`, `Dir` (the project folder), `Prompt`, `Worktree` (a fresh run token) and `Project`
(the project's slug), as `launch-v1` and `worktree-v1` define them, plus three new flat fields under
`agents-v2` (R14):

- `Base`: the base branch name. The connector resolves it with `git rev-parse --verify --end-of-options
  refs/heads/<base>^{commit}` (then `refs/remotes/<base>^{commit}`) and uses that commit in place of
  `HEAD` in the one `worktree add`. Requires `Worktree`. Unknown → `worktree_refused` reason
  `base_missing`: "That branch isn't in the repository on studio."
- `Effort`: `low`, `medium` or `high`, mapped by the preset to its own confirmed option; refused for a
  preset without one.
- `Yolo`: a boolean, sent only to a peer reporting capability `yolo`; the connector maps it to that
  preset's allowlisted option and refuses if the machine has YOLO switched off or the preset has none.

The lost-reply rules of Start an agent apply unchanged: the request id and parameters are kept, the
retry button sends exactly the same request, nothing is kept in browser storage.

**Start in several projects.** With two or more projects selected in the rail, **Start** creates a
**run** through `internal/projectrun` unchanged in its rules: one member per project, at most 2 in
flight and 1 per machine, derived request ids, per-member retry, partial failure is normal. The run's
name is the projects' names joined; its progress shows in a status sheet ("2 of 3 started") and its
sessions appear in In flight. YOLO applies to every member, and the confirmation lists every machine
with its own gate line.

## Remote folder browser protocol (`browse-v1`)

A negotiated, optional control request, built like `worktree-v1`: the controller advertises
`browse-v1` in `X-Reet-Terminal-Features`; a connector that understands it records it and reports the
capability `browse` (omitempty, requires `terminalControl`) unless the machine refuses browsing
locally; a connector whose browse switch is off reports `browseOff` instead (omitempty, requires
`terminalControl`, never with `browse`, sent only on a connection that negotiated `browse-v1` because
an older controller's strict decoder would refuse it). The controller never sends a browse request to
a connection without `browse`: it answers 409 `browse_off` ("That machine has folder browsing switched
off.") to one that reports `browseOff`, and 409 `browse_unsupported` ("Update reet on that machine to
browse its folders.") to one that reports neither.

**Request** `Browse{Path, After, Filter string; Hidden bool}`: `Path` absolute (the existing `ValidDir`
rule) or empty for the enrolled home; `After` a continuation name (at most 255 bytes); `Filter` an
optional case-insensitive substring (at most 64 bytes, plain text); `Hidden` a boolean. Flat and
comparable.

**Reply** `BrowseResult{path, home, parent, repo, repoTop, entries, more, partial, scanned, omitted}`:

- `path` is the resolved folder; `home` the enrolled home; `parent` empty at the home.
- `repo` describes `path` itself: `root` (it holds a `.git` directory with a `HEAD` file), `inside` with
  `repoTop` the top folder's path (found by walking up with stat only, never above the home), or
  `none`.
- `entries`: at most 200 per page, sorted by name (byte order, case-folded), each `{name, dir,
  repoRoot, hidden}`. In `browse-v1` only directories are returned, so `dir` is always true: **file names
  never leave the machine.** The field is kept so a later version could list files without a wire
  change; the web ignores an entry with `dir` false. `repoRoot` is true when the entry holds a `.git`
  **directory** that contains a `HEAD` file (two `lstat` calls). A `.git` file (a linked worktree or
  submodule) is not a repository root here, matching what `worktree-v1` accepts. `hidden` is true for
  names starting with `.`; hidden entries are left out unless `Hidden` is set.
- `more` is true when another page exists (pass the last name as `After`); `partial` when the deadline
  cut `repoRoot` checks short; `scanned` and `omitted` count directory entries read and entries left
  out (files, links leaving the home, unreadable, names over 255 bytes).

**Machine-side rules.**

- **Containment.** `Path` is cleaned, resolved with symlinks followed, and must be the home or inside it,
  exactly as the launch folder policy (`ResolveDir`). A symlinked entry is listed only when its target
  resolves to a directory inside the home; it is listed under its own name and entering it resolves the
  target again. Nothing above the home is ever listed, including through `..` or a link.
- **No contents.** Only `readdir` and `lstat`/`stat`. No file is opened, no git command runs, no
  repository configuration is read.
- **Names.** Any byte sequence the filesystem allows is listed if it is valid UTF-8 without control
  characters; other names are omitted and counted (they cannot be shown or sent back safely).
- **Bounds.** At most 5,000 directory entries read per request, at most 200 returned, reply at most
  12 KiB so it fits the 16 KiB control frame. A 3-second deadline per request: a slow or network
  filesystem returns what it has with `more` set, and `repoRoot` checks that miss the deadline report
  `false` with `partial` set.
- **Concurrency.** One browse at a time per connection; a second is answered `conflict` at once.
- **Refusal.** `reet terminal browse off` on the machine (local, like terminal consent) removes the
  capability: the connector reports `browseOff` and answers `off` (a browse is also refused `off`
  while terminal access is off); `denied` stays for a folder the user cannot read. Default on, because
  every folder listed is one the enrolled user can already open in a terminal session.
- **Errors.** Typed reasons `not_found`, `not_dir`, `outside_home`, `denied` (permission denied),
  `unreadable`, `conflict` (another browse is running) and `off` (browsing switched off there), each
  mapped to one sentence ("That folder isn't on studio." for not found and not a directory, "That
  folder is outside the home folder reet was set up with.", "reet can't read that folder." for denied
  and unreadable, "That machine is already listing a folder. Try again in a moment." for conflict,
  "That machine has folder browsing switched off." for off).

**Controller rules.** Writers only, owner-scoped, canonical origin, the terminal rollout flag. Rate limit
20 browse requests per 10 seconds per account and machine (429 `browse_rate_limited`: "Slow down a
moment, then try again."). Controller deadline 6 seconds. An offline or reconnecting machine is
answered at once with the existing readiness sentence; the dialog keeps the typed path and lets the
person add the project without browsing (it is checked again at launch). A machine that disconnects
mid-browse fails only the request in flight, with the readiness sentence and **Try again**; the
breadcrumb and list already shown stay. **The controller stores nothing from a browse**: no cache, no
log of names. The audit trail gets one `folder-browse` event per opened browser (machine, starting
folder, number of pages), not one per click.

### Branch listing (`branches-v1`)

The branch slot and the dialog's base-branch suggestions need the machine's branches. That needs git, so
it is a separate negotiated request, `Branches{Dir, Filter string}`, under `branches-v1` and capability
`branches` (requires `worktree`). The connector checks `Dir` as the worktree create does (repository
root, inside the home) and runs, with the worktree environment and hooks rules and a 5-second timeout,
exactly: `git -C <dir> for-each-ref --count=201 --sort=-committerdate --format=%(refname)
refs/heads refs/remotes`, then `git -C <dir> symbolic-ref --quiet --short HEAD` and `git -C <dir>
symbolic-ref --quiet --short refs/remotes/origin/HEAD`. Each call also gets `GIT_DIR=<dir>/.git` and
`GIT_CEILING_DIRECTORIES=<parent of dir>`, so git reads that folder's own `.git` or fails, never a
repository around it. The reply `BranchesResult{current, default, branches, more}`: `current` the
checked-out branch (empty when detached), `default` the remote's default when
`refs/remotes/origin/HEAD` exists, at most 200 short names (each a valid ref name of at most 255
bytes), newest first, `Filter` applied as a case-insensitive substring. A refusal is a typed reason
instead: `not_repo_root` ("That folder isn't the top of a git repository."), `outside_home` ("That
folder is outside the home folder reet was set up with."), `git_failed` ("Git couldn't list the
branches there."; nothing of git's output is sent), `conflict` (another listing is running: "That
machine is already listing branches. Try again in a moment.") or `denied`, meaning the machine does
not offer branches now: terminal access is off there, or no git, or no worktrees ("That machine isn't
offering its branches right now. Check that terminal access is on there and git is installed."). A
connection without `branches` is answered 409 `branches_unsupported` ("Update reet on that machine to
list its branches.") or, when it reports `browse` or `browseOff`, 409 `branches_denied`. Rate limit
10 per 10 seconds per account and machine; nothing stored.

## Project registry

A **project** (per account): `id`, `host` (the machine's enrollment id), `dir` (absolute, cleaned,
`ValidDir`), `name` (1–80 characters, the Projects naming rules, unique per account ignoring case;
defaults to the folder name, with ` (2)` appended on a clash), `isRepo` (as last seen by browse or
launch, informational only), `baseBranch` (optional, a valid ref name, at most 255 bytes),
`worktreeDefault` (`inherit`, `yes`, `no`), `defaultTool` and `defaultModel` (optional, the launch
rules), `starterPrompt` (optional, R2), `position` (rail order), `createdAt`, `updatedAt`, `version`. A
machine and folder may be registered once per account (409 `project_exists`). At most 100 projects per
account (R7). Each person gains two profile settings: **default base branch** (blank = the repository's
own default) and **worktrees by default** (yes, initially).

**API** (under `/api/v2`, the Projects access rules, `Cache-Control: no-store`, one-sentence refusals
naming the field):

- `GET /folder-projects` → `{projects}`; `POST /folder-projects` → 201; `GET`, `PATCH`, `DELETE
  /folder-projects/{id}` with `ETag`/`If-Match` exactly as Projects (409 `project_changed` "Someone
  changed this project. Reload it, then try again." on a stale version; without the header the last
  write wins; the web always sends it).
- `POST /folder-projects/reorder` with `{ids, versions}`.
- `GET /machines/{agentId}/browse?path=&after=&filter=&hidden=` and `GET
  /machines/{agentId}/branches?dir=&filter=` for the two machine requests.
- Delete removes only the registry entry. Sessions and worktrees are untouched: "Project removed. Its
  sessions and any worktrees stay on studio."

The path `folder-projects` is internal; it becomes `projects` once the old routes are removed, with the
old shape's `GET /projects` answering 410 `projects_moved` for a release so stale tabs explain
themselves.

**Session metadata.** The controller already stores each create request. It gains `project_id`
(nullable), and the session listing returns `tool`, `model`, `effort`, `yolo`, `projectId`,
`worktreeBranch` and the attention fields. The prompt is never in the listing; only its first line, at
most 80 characters, as the default title.

## Sessions page

`/sessions` lists every session across machines from the existing account-wide inventory, joined with
projects by `projectId`. Sessions without one (plain shells, older sessions, home-folder agents) are
matched by exact machine and folder for display only, else grouped under **Other on \<machine\>**.

**Group by** Project | Machine | State (shared with Home's choice). **Search** matches titles, agent,
model, branch, project and machine names. **Filters**: state (needs you, running, finished, ended),
agent, machine, YOLO only, worktree only. Rows (44 px minimum on touch): state dot and word, title,
`agent · model · branch`, YOLO and worktree badges, age, and the existing actions menu (right-click,
Shift+F10 or the button): Rename, Open in theatre, Copy branch, End session (confirm), Remove history
(ended only, confirm), Check and remove worktree. The existing custom order and colours stay as
options under filters. Below the groups: *Projects · N* with **Add a project** and the project cards
(the phone's rail).

**States.** Loading shows the last known list with "Checking…" (no skeleton flash); an error keeps the
last list with "Couldn't refresh. Showing what we last saw." and **Try again**; an offline machine's
sessions stay listed with "studio is offline" on the group and actions that need it disabled; no
projects shows "Add a project to start an agent in one of your folders."; no machines shows the existing
add-a-machine prompt. Polling stays at 15 seconds while visible.

## Attention state

| State | Structured (ACP) sessions, Program 2 | Terminal sessions, this pass |
| --- | --- | --- |
| Starting | create accepted, agent not yet ready | create accepted, program not yet running |
| Working | a prompt turn is running | output within the last 30 s (tmux `window_activity`) |
| Quiet | not used | running with no output for 30 s or more |
| Waiting for input | a permission request or an ended turn awaiting a reply | the program rang the terminal bell since the person last looked (tmux `window_bell_flag` with `monitor-bell on`) |
| Needs sign-in | the adapter's authentication-required error | not detected |
| Finished | the turn ended | exited 0 (`remain-on-exit` status) |
| Failed | an error ended the session | exited non-zero: *Exited N* |

Terminal agents get **honest, coarse** states: *Working/Quiet* is output activity, not progress;
*Waiting* is only the bell. The row's tooltip and the session header say where the state came from
("From terminal activity"). Under `attention-v1` (R14) the connector reports per session `{activityAt,
bellAt, exitStatus}` on `SessionSnapshot`; the controller derives the state and keeps a per-person
`seenAt` per session, so "changed since you looked" is per person and survives reloads. Nothing in the
attention data contains terminal output.

**Needs you and the bell.** The queue holds Waiting, Failed and unseen Finished sessions across every
machine (plus Program 2's kinds), newest first, one card per session. The bell's count is the queue's
length; the bell opens a popover listing the same cards (on phones, Home's Needs you). The page title
gains the count, `(3) Home · reet.sh`. A polite live region announces only transitions into Waiting,
at most once per session per minute. Opening a session or **Mark seen** sets `seenAt`.

## Agent catalogue

One table in the connector, keyed by tool id: display name, binary, how a model, a prompt, an effort
and YOLO are passed, the safest permission default, and whether it speaks ACP (natively, through a
pinned adapter, or not). Each option is confirmed from the real CLI's help text on a machine that has
it (the help text is kept as test data), or the option ships disabled. Reported to the controller only
under `agents-v2`: a connector that did not negotiate it reports only `claude`, `codex`, `opencode`,
`gemini`, so an older controller never sees an unknown id. The web holds display names and model
suggestions only.

| Id | Agent | This pass |
| --- | --- | --- |
| `claude`, `codex`, `opencode`, `gemini` | Claude Code, Codex, OpenCode, Gemini CLI | Shipped presets; effort and YOLO options added where confirmed. Claude Code and Codex use adapters for ACP (Program 2). |
| `pi`, `cursor` | Pi, Cursor Agent | New presets, each option confirmed or disabled. |
| `cline` | Cline | New preset; always passes `--auto-approve false` so its default is to ask (its YOLO option, if confirmed, replaces that flag). |
| others | the rest of the owner's list | Listed disabled in the agent slot until confirmed (R3). |

## Migration

Runs once, in the pull request that retires the Projects page, inside one transaction, idempotent
(re-running creates nothing):

1. Every distinct machine and cleaned folder across all saved Projects becomes one folder project. The
   name is the folder's name (de-duplicated with ` (2)`); `defaultTool`/`defaultModel` come from the
   member, else the Project's defaults; `worktreeDefault` is `yes` or `no` from `useWorktrees`;
   `starterPrompt` from the shared prompt (R2); position follows Project then member order. A folder
   already registered is skipped (its prompt is not overwritten).
2. Each migrated Project writes one `projects-migrated` audit event naming the folder projects created
   (never the prompt). `/projects/<id>` redirects resolve through a kept `legacy_project_id` mapping.
3. Old **runs** are kept read-only and listed in Activity (*Runs from Projects*). Their recorded
   worktrees appear under each folder project's **Worktrees** menu with the existing Check and remove
   sheet, so no worktree loses its safe remove path.
4. The old `projects` and `project_members` tables are dropped one release later (still open).

## Security and privacy

- **Access.** Accounts mode, write role, canonical origin, rollout flag, owner scoping on every query, as
  Projects. Viewers, API and MCP tokens, single-token and open modes get no browse, branches, project
  registry or launch bar, and the routes refuse them with the Projects sentences.
- **Paths** are the owner's own and are stored only for registered projects and created sessions. Browse
  results are not stored. Only ids go in the address bar.
- **Prompts and starter prompts**: stored with the create request, the project and the machine's private
  registry, never in audit, logs, browser storage or the URL; visible in the process list while passed
  as an argument.
- **Secrets**: nothing in this design reads or sends file contents, environment values or git
  configuration from the machine; browse is stat-only.
- **YOLO**: the account setting (owner/admin; changes audited as `yolo-allowed` / `yolo-disallowed`);
  the machine refusal `reet terminal yolo off` removes the capability and is re-read on every create;
  the controller refuses a YOLO create without both (403 `yolo_not_allowed`, 409 `yolo_refused`); each
  YOLO launch writes `terminal-launch` with `yolo: true` plus a `yolo-launch` event; in ACP sessions
  (Program 2) each automatic `allow_once` is its own `yolo-permission` event.
- **Audit events added**: `project-add`, `project-update`, `project-remove`, `projects-migrated`,
  `folder-browse`, `yolo-allowed`, `yolo-disallowed`, `yolo-launch`, `yolo-permission` and
  `adapter-install` (Program 2). `terminal-launch` gains effort, base branch and YOLO.
- **Rate limits**: browse 20/10 s and branches 10/10 s per account and machine; project writes 30 per
  minute per account; session creation keeps 8 per machine and 32 per account.
- **Fonts**: self-hosted (R15); the content security policy is unchanged.

## Accessibility and responsive rules

- Breakpoints: phone is `PHONE_SCREEN` from `terminal/sizing.ts` (650 px wide, or sideways 500 px tall on
  touch); tablet 651–1099 px; desktop 1100 px and wider.
- Tabs are a `nav` of links with `aria-current="page"`, not an ARIA tablist (they change the page). The
  bell is a button named "Needs you: 3". Group by is a radio group. Session tabs are links with a close
  button each.
- The launch bar is a form: labelled prompt textarea; each slot a button naming its slot and value
  ("Agent: Claude Code") that opens a listbox popover (desktop) or bottom sheet dialog (touch); Escape
  closes and returns focus. Approvals is a radio group named "Approvals"; Start is a real button named
  "Start the session".
- Status in text and icon as well as colour; focus visible; reduced motion respected (sheets and the
  rail appear without sliding); touch targets at least 44 px (48 px for phone primary actions, 40 px
  for the terminal's own keys, as today).
- **Phone zoom contract** (`docs/ui-verification.md`, Phones: long names and page zoom): no screen may
  lay out wider than the device; long project names, paths and branch names wrap or truncate with their
  full text available; inputs use a 16 px font. Home, Sessions, the launch sheet and the add-project
  sheet with a long path join the `phone-zoom.e2e.mjs` matrix.
- **Keyboard-open layout**: the phone terminal's rules (`docs/terminal-workspace.md`, Phones) extend to
  focus in the launch prompt and the session screen: chrome hidden, the focused surface above the
  keyboard, same height rule.

## Testing strategy

- **Go.** Protocol round trips and strictness for `Browse`, `Branches`, `Base`, `Effort`, `Yolo` and the
  attention fields; negotiation and old-peer refusal for `browse-v1`, `branches-v1`, `agents-v2`,
  `attention-v1` (an old controller never receives a new tool id or field); connector browse against
  real temporary trees: symlink escape, a link to a folder inside home, `..`, hidden entries, names with
  spaces, quotes, newlines and invalid UTF-8, a `.git` file versus directory, a repository root without
  `HEAD`, unreadable folders, the 5,000-entry cap, paging, the deadline with a slowed filesystem fake,
  reply size under the frame limit, and a test that no file is opened; branches with the exact argument
  vectors; worktree create from a named base; store and API for projects (limits, uniqueness, If-Match,
  owner scoping, viewers blocked, rate limits); migration (dedupe, names, starter prompts, idempotent
  re-run, runs kept); YOLO gates (each refusal) and audit rows; attention derivation. A real two-host
  test extends the existing one with browse, branches and a YOLO launch against recording fake CLIs.
- **Web unit** (`npm test`): `folderBrowser.ts`, `launchBar.ts` (sentence, slot defaults precedence,
  summary, validity), `home.ts` (fleet figures, In flight grouping, Needs you queue), `catalogue.ts`,
  `theme` token contrast pairs.
- **Browser fixtures** with synthetic API answers: `home.e2e.mjs`, `add-project.e2e.mjs`,
  `launch.e2e.mjs`, `yolo.e2e.mjs`, `sessions.e2e.mjs`. Matrix: 1440, 1360, 1024 and 834 px, phone 412,
  390, 360 and 320 px with `isMobile` and `hasTouch`, sideways 844 × 390, light and dark, keyboard-open
  emulation. Axe at 390, 834 and 1440 px reported honestly (violations and the `incomplete` count).
- **Mutation checks**: browse containment, "no file names", the hidden filter, Approvals resetting after
  launch, YOLO refused without the account setting, never sending new fields to an old machine, the
  16 px input font, the bell count equal to the queue, no step bar on a terminal session.
- **CI budget**: the new fixtures add at most 120 seconds to `terminal-browser.yml`, split across the
  existing jobs; `phone-zoom` grows by the new screens and stays under 60 seconds.

## Docs to update

`docs/start-agent-session.md` ("No preset uses a flag that skips the agent's own permission prompts"),
`docs/terminal-launch.md` (the Presets paragraph that rules out approval-bypass and auto-approve flags,
and the test that pins the only flags), and the start-agent spec's "No permission-bypass flags" line
(amended, not rewritten): each to say YOLO exists, is off by default and how it is gated. Also
`docs/projects.md` (rewritten for folder projects), `docs/terminal-worktrees.md` (base branch, branch
listing), `docs/terminal-workspace.md` (Home, Sessions, the session screen, phones), a new
`docs/folder-browse.md`, `docs/control-centre.md` (folded into Home), `docs/roadmap.md`, and
`docs/ui-verification.md` (each new matrix).

## Delivery: pull-request slices and order

Each pull request merges when its checks are green, the broad review has passed and Codex is clean; each
leaves the product usable, and each release tag leaves it no less capable than before (see R22).

**Shell first.** At the owner's request the new app shell and Home (PR D) land first, right after the
shared dialog shell (#217), on today's API with nothing invented. Every later pull request swaps its
real piece into that shell and may tweak the layout where the real data needs it; each lists its swaps
and tweaks in the plan. The letters are identities, not the order. Execution order: **D, A, B, C, E, F,
G, H.**

| Order | PR | What it swaps into the shell |
| --- | --- | --- |
| 1 | D | The shell and Home on today's data. |
| 2 | A | Nothing visible (wire only); unblocks the folder browser. |
| 3 | B | Folder projects in the rail; the folder browser behind **Add**; `/projects` retired. |
| 4 | C | Nothing visible until PR E; the agent catalogue. |
| 5 | E | The launch sentence in place of the **Start an agent** button; Start in several projects. |
| 6 | F | YOLO in the launch bar; badges. |
| 7 | G | Needs you cards, the bell's count, attention state words. |
| 8 | H | `/sessions`, the session screen, the last redirects, the phone rail on Sessions. |

- **PR D — shell and Home on today's data (first).** Theme tokens (dark designed default, light derived,
  following the person's theme, R6), self-hosted fonts (R15), the real logo, top tabs, the bell,
  responsive navigation with the four-tab bar on tablets and phones (R13), `/activity`, and Home at `/`
  (read-only for viewers, R16) built only from existing endpoints:
  - *fleet strip* from the machines health and sync endpoints: CPU labelled "CPU" or "No reading", the
    skills line from existing sync state or "Not checked", the worktree count only where an existing
    record gives it (R12);
  - *In flight* from the existing sessions list, grouped by Project (existing Projects, by their runs'
    session ids, until PR B; sessions under no Project grouped by machine), Machine or State, with the
    state word and where it came from (R10);
  - *Needs you*: nothing today signals that a session needs the person, so the honest empty state
    "Nothing needs you." and a bell without a badge; cards arrive with PR G and decision cards with
    Program 2;
  - *project rail*: existing Projects, read-only, with a plain note that folder browsing comes later
    (PR A and PR B); **Add** opens the existing New project dialog;
  - *launch*: a **Start an agent** button opening the existing dialog until PR E.

  The legacy Projects page and Control stay reachable under **More**; `/terminals`, `/projects` and
  `/control` keep their pages (their redirects belong to PR H, PR B and PR H). The shell and Home are
  checked at 1360, 834, 390 and 320 px, light and dark, reduced motion, axe, the phone-zoom contract with
  long names, the keyboard-open phone layout and mutation checks.
- **PR A — browse and branches on the wire.** `browse-v1`, `branches-v1`, connector handlers, broker,
  API endpoints, rate limits, audit, Go and two-host tests, `docs/folder-browse.md`. No UI.
- **PR B — project registry, migration and the folder browser.** Store, API with If-Match, limits,
  audit, profile defaults, session `project_id`; the migration of saved Projects; the Add project dialog
  with the folder browser; the rail on folder projects; `/projects` retired with redirects and runs
  read-only in Activity. Because Start all goes here and Start in several projects returns in PR E,
  **PR B to PR E are one release train** (R22): no release is tagged between PR B and PR E merging.
- **PR C — catalogue and launch fields.** `agents-v2`, `pi`, `cursor`, `cline` presets with safe
  defaults, the disabled list, `Effort` and `Base` (and the `Yolo` field, refused until PR F).
- **PR E — launch bar.** The sentence, prompt and Start wired to `launch-v1`/`worktree-v1`/`agents-v2`,
  starter prompt chip, and Start in several projects through the run orchestration.
- **PR F — YOLO.** Account setting, machine refusal and capability, Approvals and the confirmation
  dialog, badges, audit, per-preset flag mapping, the docs that said auto-approve is not offered.
- **PR G — attention.** `attention-v1`, derived states, per-person seen, Needs you queue, bell, title
  count, live region.
- **PR H — session screen and small screens.** `/sessions` with the `/terminals` and `/control`
  redirects, tab strip, header, context panel, Chat | Terminal scaffolding, the session screen on
  tablets and phones, and the full fixture matrix and axe across every new screen.

**Program 2** (own spec and plan): the ACP runner in tmux, the Claude adapter's confirmed pinned install,
the chat view (messages, plan checklist, tool and edit cards, decision cards with A/D), command approval
and sign-in cards in Needs you, exact attention states, step progress, line counts, machine-only
transcripts, notifications outside the page.

PR D waits for the shared dialog shell to merge; PR A and PR C touch the protocol and can be reviewed in
parallel with PR D or each other.

## Risks

- **Large UI change on the most used pages.** Mitigated by landing the shell (PR D) first on today's
  data, keeping every legacy page and dialog reachable until its replacement lands, keeping
  `/terminals` links working, and the fixture matrix from the first pull request.
- **Shell first means layout is fixed before the real data.** Each later pull request lists the tweaks
  its data needs; the spec's drawings stay the reference, and PR H re-runs the full matrix.
- **Losing Projects' features in migration.** Mitigated by R1 and R2: folders, defaults, prompts, runs
  and worktree removal all survive.
- **Coarse attention looks wrong.** Terminal states are labelled with their source, never say "waiting"
  without a bell, and never show steps (R10).
- **The mock-up promises Program 2 features.** R10, R11, R19 and R21 state what ships now; the layout
  reserves their places so Program 2 adds, not rearranges.
- **YOLO harm.** Three independent gates, per session only, reset after launch, audited.
- **Unconfirmed CLI flags.** Every option ships disabled until confirmed from help text.
- **Phone keyboards differ.** The height rule is reused, not reinvented; physical devices remain
  unchecked (as stated in UI verification).
- **Concurrent dialog work.** PR D, the Add project dialog (PR B) and the YOLO dialog (PR F) depend on
  the shared dialog shell; PR D waits for it.

## What this does not establish

That an agent is signed in, understands the prompt or makes progress; that *Working* means useful work;
that a branch contains anything; that a repository's hooks, build or tests are safe; that a machine's
CPU figure reflects agent work. YOLO's confirmation is the only protection once the agent runs. No
physical phone, Safari or Firefox check is planned beyond the Chromium fixtures.

## Self-review

- **Placeholders:** none. Facts not yet known (exact CLI flags, agents beyond the first cut) are "ship
  disabled until confirmed" and listed under Still open.
- **Mock-up versus honesty:** every mock-up element that terminal agents cannot produce has a ruling
  (R10, R11, R19, R21) naming what ships and when the rest arrives.
- **Contradictions checked:** browse is stat-only while branches runs read-only git commands, so they
  are separate features; `Base` moved to `agents-v2` so no token carries a field its first release
  cannot decode; YOLO is per session and never remembered everywhere it appears; the 16 KiB frame limit
  is respected by paging browse replies; worktree accepts only a `.git` directory and browse marks only
  that as a repository root; "no separate Projects page" holds because runs move to Activity and
  worktrees to the project menu.
- **Scope:** eight pull requests, each reviewable on its own, run shell first (D, A, B, C, E, F, G, H);
  Program 2 is named and excluded.
