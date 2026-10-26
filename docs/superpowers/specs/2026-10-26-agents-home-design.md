# Agents home (S8 design)

Date: 2026-10-26. Status: design for the product owner's review; no plan and no code yet.

The terminal page becomes reet.sh's home: a calm dark workspace with a left rail of sessions grouped
by project, a Projects list, and a large composer that starts an agent in a project folder on one of
your machines. A project stops being a named set on its own page and becomes one folder on one
machine, picked by browsing that machine's real folders. This design covers the whole Agents home
pass and how to ship it in pull-request-sized steps. Structured chat over the Agent Client Protocol
is designed here only as far as the home needs it; it gets its own spec.

"The reference designs" below means the screenshots the owner supplied: an agent dashboard (sessions
rail, centred composer) and a session manager (sidebar with a Projects list and an Add project dialog
with a folder browser). Nothing in this repository names them.

## Intent and success

**Outcome.** A signed-in owner with a write role opens reet.sh and lands on the Agents home. They add a
project by browsing a machine's folders, pick it in the composer, describe the work, choose an agent,
model and worktree, press send, and watch the session in the same page. Across all machines, a short
list says which sessions want them.

**Who it is for.** The account owner (and writers) on a desktop, a tablet or a phone, on machines that
are enrolled and consented to browser terminals.

**Success looks like.**

- Starting work is one composer and one press, from a project the person chose by looking at the
  machine's folders, not by typing a remembered path.
- The page works with a thumb on a 320 px phone, with the keyboard open, without the browser zooming
  out, and on a 768 px tablet and a 1360 px desktop.
- Nothing on screen claims more than happened. "Started" still means the connector started the
  program in tmux; attention states say where they come from and never guess.
- The controller still never sends a shell string, a binary, a flag or a git command. Everything new
  on the wire is a closed, validated field the machine re-checks.
- YOLO is never on unless an owner allowed it for the account, the machine did not refuse it, and the
  person switched it on for that one session after a warning.

## Decisions already taken with the product owner

These are recorded, not reopened.

| Question | Decision |
| --- | --- |
| Where do projects live? | On the terminal page, in the rail, as in the reference designs. There is **no separate Projects page**. |
| What is a project? | A folder on a machine, chosen by browsing the machine's folders at runtime, with automatic detection of whether it is a git repository. |
| When? | Soon. The main page should look and behave like the reference designs. |
| YOLO | Adopted, **off by default**, chosen per session at launch only, with a clear warning, a visible YOLO badge on that session everywhere, every use audited. An account setting **Allow YOLO**, off by default, that only owners and admins can switch on. A machine can refuse. In structured (ACP) sessions reet itself answers each permission request with `allow_once` (each one audited, never `allow_always`); terminal-only agents use their own allowlisted option. No free-form flags or command lines. |
| Chat transcripts | Stored **on the machine only**. The controller relays live events and fetches history on demand; history is unavailable while the machine is offline. |
| Agents | A built-in, connector-side catalogue (allowlisted, never command lines from the controller) covering every agent on the reference designs' lists plus Cline, each with its safest permission default (Cline's terminal mode auto-approves by default, so its preset passes `--auto-approve false`). New feature token `agents-v2`, because an older controller rejects a connection that lists an unknown tool id (`Capabilities.Validate`). |
| Structured chat | The Agent Client Protocol (stdio JSON-RPC) through a small runner inside tmux. For Claude Code and Codex, which need adapters, reet offers a **confirmed, audited install of a pinned adapter version** (owner or admin), with detection and the exact command shown; never silent. |
| Program order | Slice 1: catalogue + Agents home + YOLO. Slice 2: structured chat for Claude over ACP. Then more agents. |
| Attention | Working / waiting for input / needs sign-in / finished, with a cross-machine attention list, is part of this pass (the chat spike folds in). |

## Open question 1, stated first: what happens to today's Projects

Projects shipped in #210–#213: a named set of up to 12 machine-and-folder members, defaults, a shared
prompt, Start all, per-run worktrees, runs with retry, and a safe Remove worktree. The owner has not
yet said how that maps onto "a project is one folder". **This design assumes:**

- A project is **one folder on one machine**, added from the rail.
- **Start all and per-run worktrees survive** as an action on a multi-selection of projects in the rail
  ("Start on 3 projects"), using the same run orchestration.
- The **Projects page, its named sets and its `/projects` routes are retired**. Existing saved Projects
  are migrated into folder projects (see [Migration](#migration)); old run records are kept read-only so
  their worktrees can still be checked and removed safely.

Alternatives the owner may prefer:

- **A. Keep sets as "groups".** A saved multi-selection with a name, shown as a collapsible group in the
  rail's Projects section, with Start all on the group. Costs one more concept and table; keeps the
  shared prompt and named runs.
- **B. Drop history.** Migrate folders, delete sets and run records. Simplest, but worktrees recorded by
  old runs lose their safe Remove path (the person would remove them by hand).

**Recommendation:** the assumption above. It keeps every safety property of Projects, removes the
second page the owner does not want, and leaves "groups" as an easy later addition if multi-selection
proves too fiddly.

## Out of scope

The ACP chat view itself and the adapter install (slice 2, own spec); resuming earlier conversations;
pull requests, merging or pushing; git-remote project identity; folders outside the enrolled home;
Windows; push or email notifications for attention (the attention list is in-page only); MCP exposure
of projects or sessions; agent settings screens; changing the security/PIN preference or the legacy
sync flows; any change to the shared dialog shell another change is building (this design uses it).

## Information architecture and navigation

**Landing.** For accounts mode with a write role, `/` becomes the Agents home. Everyone else (viewers,
single-token and open modes, signed-out visitors) keeps today's landing: the welcome page signed out,
the Control overview signed in. Viewers cannot open terminals, so the home would be empty for them.

**Routes.**

| Address | After this design |
| --- | --- |
| `/` | Agents home (writers in accounts mode); unchanged for everyone else. |
| `/terminals` | The same Agents home component. Kept so every existing link works. `?agent=…&session=…` opens that session in the home. |
| `/terminals?project=<id>` | The home with that project chosen in the composer (new, used by redirects and the rail). |
| `/projects`, `/projects/<id>` | Permanent client-side redirect to `/` (writers) with a one-time note "Projects now live in the rail." `/projects/<id>` maps to the first folder project migrated from it. `projects` stays a reserved hosted username. |
| `/control` | The Control overview (machine connection cards and quick links) stays reachable for every role. |

**Main navigation** for writers in accounts mode: **Agents** (the home), **Machines**, **Skills**,
**Setups**, **More** (Sync, Lists, Community, Support, Teams & organisations, Host skills, Control).
The separate **Terminals** and **Projects** entries go; Agents replaces both. Viewers and the other
modes keep Control as their first entry. On the Agents home the app header is the existing compact
terminal header so the rail and the composer get the height.

**The Control page.** Its active-session band duplicates the rail, and its connection cards duplicate
Machines. For writers it moves under More and stops being the landing; its content is unchanged in
this pass. Whether to fold it into Machines entirely is open question 4.

## Screens

The look follows the reference designs: a near-black canvas, one quiet accent, generous spacing,
small-caps section labels in the rail, monospace for paths and branches, and a centred composer with a
round send button. It uses reet's existing tokens and themes: dark is the design target, light must
pass the same checks (open question 6 asks whether the home should always be dark). Copy follows
`docs/voice-and-copy.md`: plain and exact, at most one light Yorkshire phrase in a heading or lede.

### Desktop (1100 px and wider): empty state

```
+------------------+-----------------------------------------------------------------+
| reet.sh   Agents Machines Skills Setups More                       [PJ]           |
+------------------+-----------------------------------------------------------------+
| SESSIONS  (s)(f) |                                                                 |
| [+ New session ] |                                                                 |
|                  |                   What shall we work on?                        |
| NEEDS YOU (2)    |                                                                 |
| ! reet  Waiting  |      [ [] reet - studio  v ]  [ <> main  v ]                    |
| ! site  Sign in  |      +-----------------------------------------------------+    |
|                  |      | Describe what you want...                            |    |
| v reet  (3)      |      |                                                      |    |
|   Fix sizing  2m |      |                                                 (^)  |    |
|   Tidy docs  1h  |      +-----------------------------------------------------+    |
|   [YOLO] Lint 3h |      | Claude Code v | opus v | Thinking: High v |           |    |
| > site  (1)      |      | Worktree: new  v |                     YOLO [ o  ] (i)|    |
| > Other on mini  |      +-----------------------------------------------------+    |
|                  |       Starts Claude Code in ~/work/reet on studio, in a new     |
| PROJECTS (4)  +  |       worktree from main. Nothing runs until you press send.    |
|  reet            |                                                                 |
|  ~/work/reet     |                                                                 |
|  studio          |                                                                 |
|  site            |                                                                 |
|  ~/work/site     |                                                                 |
+------------------+-----------------------------------------------------------------+
  (s) search  (f) filters  (^) send  (i) what YOLO does
```

### Desktop: a session open

```
+------------------+-----------------------------------------------------------------+
| SESSIONS  (s)(f) | Fix sizing  . Claude Code . opus . reet/reet-3f9a . studio  [...]|
| [+ New session ] | Working - last output 4 s ago                     [Theatre][End]|
| ...              +-----------------------------------------------------------------+
| v reet  (3)      |                                                                 |
| > Fix sizing  2m |   live terminal pane (TerminalPane, unchanged behaviour)        |
|   Tidy docs  1h  |                                                                 |
| ...              |                                                                 |
|                  +-----------------------------------------------------------------+
|                  | toolbar and extra keys as today                                 |
+------------------+-----------------------------------------------------------------+
```

The header line is the session's title, agent, model, branch (when a worktree), machine, attention
state with its source, a YOLO badge when on, and the existing actions. **New session** (or `n` when
focus is not in a terminal or field) returns to the empty-state composer without ending anything.

### Tablet (651–1099 px)

Landscape (1024 px and wider) keeps the rail docked at its stored width (220–480 px). Portrait and
narrower windows show the rail as an overlay opened by **Sessions** in the top bar, closed by
choosing a session, Escape or the scrim. The composer is centred with 24 px side margins and the
picker row wraps onto two lines.

```
+----------------------------------------------+
| [=] Sessions (2!)      reet.sh         [PJ]  |
+----------------------------------------------+
|          [ reet - studio v ] [ main v ]      |
|  +----------------------------------------+  |
|  | Describe what you want...          (^) |  |
|  +----------------------------------------+  |
|  | Claude Code v | opus v | High v        |  |
|  | Worktree: new v          YOLO [ o ](i) |  |
|  +----------------------------------------+  |
+----------------------------------------------+
```

### Phone (650 px and narrower, or sideways 500 px tall or less)

The rail is a full-height drawer from the left (the existing overlay sidebar, restyled), opened by
**Sessions** in the top bar, which shows the attention count. The composer docks to the bottom where
thumbs are, with pickers as a sideways-scrolling chip row that fades at the end with more chips.
Each picker opens a bottom sheet with large rows. The textarea uses a 16 px font so focusing it never
zooms the page.

```
 Empty state                       Drawer open                    Picker sheet
+---------------------------+    +---------------------+----+    +---------------------------+
| [=] 2!   Agents     [PJ]  |    | SESSIONS   (s)(f)(x)|////|    |                           |
|                           |    | [+ New session    ] |////|    |                           |
|                           |    | NEEDS YOU (2)       |////|    +---------------------------+
|  What shall we work on?   |    | ! reet  Waiting     |////|    | Choose an agent        (x)|
|                           |    | ! site  Sign in     |////|    | (o) Claude Code    opus   |
|                           |    | v reet (3)          |////|    | ( ) Codex                 |
|                           |    |   Fix sizing    2m  |////|    | ( ) Gemini CLI            |
+---------------------------+    |   Tidy docs     1h  |////|    | ( ) Cline  Not installed  |
| [reet v][main v][Claude v]>|   | PROJECTS (4)     +  |////|    |   on this machine         |
| +-----------------------+ |    |  reet  ~/work/reet  |////|    | ( ) Pi  Update reet on    |
| | Describe what you     | |    |  site  ~/work/site  |////|    |   this machine            |
| | want...           (^) | |    +---------------------+----+    +---------------------------+
| +-----------------------+ |
+---------------------------+
```

With the keyboard open in the composer, the site header hides and the composer sits directly above
the keyboard, using the same usable-height rule as the phone terminal (`docs/terminal-workspace.md`,
Phones): the page height when it shrank by 15% or more, otherwise the visual viewport height and
offset. The chip row stays visible above the textarea; the send button stays at least 48 × 48 px.
A session open on a phone is today's phone terminal, unchanged, with **Sessions** in its toolbar.

### Session row anatomy

```
 [state] Title (first line of prompt, or the title)           age  [...]
         Agent . model . branch-or-folder      [YOLO] [worktree]
```

- **State**: an icon plus a word, never colour alone: *Working*, *Waiting*, *Sign in*, *Finished*,
  *Exited 1*, *Quiet*, *Starting*, *Lost*. A filled dot marks "changed since you last looked".
- **Title**: the session title (defaults to the prompt's first line, at most 80 characters, else
  `<Agent> · <folder>` as today). Long titles wrap to two lines then truncate with the full text in
  the row's accessible name.
- **Second line**: agent, model (or "default"), the worktree branch or the folder, and the machine when
  the grouping does not already say it. Badges: **YOLO** (always shown when on), **worktree**.
- **Age**: since the last state change, as `2m`, `1h`, `3d`.
- **Actions** (the existing menu: right-click, Shift+F10 or the button): Rename; Open in theatre;
  Copy branch; End session (confirm); Remove history (ended only, confirm); Check and remove worktree
  (worktree sessions; the existing status-then-remove sheet). Rows are at least 44 px tall on touch.

### Add project dialog with the folder browser

Built on the shared dialog shell. Desktop is a centred dialog up to 640 px wide; phones get a
full-screen sheet. The browser expands inside the dialog, below the Path field.

```
+-------------------------------------------------------------+
| Add project                                             (x) |
| Machine   [ studio                                    v ]   |
| Path      [ /home/you/work/reet                ] [Browse]   |
| +---------------------------------------------------------+ |
| | ~ / work /                                   [Up]       | |
| | [ type to filter                         ]              | |
| | [ ] Show hidden folders                                 | |
| |  [] archive                                             | |
| |  [] notes                                               | |
| |  [] reet                         (git) REPO             | |
| |  [] site                         (git) REPO             | |
| |  [] scratch                                             | |
| |  Showing 5 folders.                                     | |
| | Chosen: /home/you/work/reet          [Use this folder]  | |
| +---------------------------------------------------------+ |
| This folder is the top of a git repository.                 |
| Name (optional)          [ defaults to reet            ]    |
| Default base branch (optional)                              |
|                          [ blank = global default, then   ] |
|                          [ the repository's own default   ] |
| Worktree by default      [ Use global default (yes)    v ]  |
| More defaults >  (agent, model)                             |
|                                       [Cancel]  [Add]       |
+-------------------------------------------------------------+
```

- **Machine** lists ready machines that report `browse`; others are disabled with their sentence.
- **Path** can be typed; **Browse** opens the browser at the typed folder or the home. Clicking a folder
  enters it; the breadcrumb and **Up** go back (never above the home). **Use this folder** fills Path
  and collapses the browser. Typing a path does not list it; leaving the field checks it with one
  browse request to show whether it exists and is a repository.
- A non-repository folder is allowed, with "This folder isn't a git repository, so sessions there
  can't use worktrees." A folder inside a repository but not its top says "This folder is inside the
  repository at ~/work/reet. Use the top folder for worktrees." with a button to choose it.
- **Name** defaults to the folder's name; **Default base branch** blank inherits the account default
  then the repository's detected default; **Worktree by default** is *Use global default*, *Yes*, *No*.
  **More defaults** (collapsed) holds default agent and model.
- **Add** is enabled when a machine and an absolute path are set. Refusals keep the dialog open and move
  focus to the field.

### Attention list

On desktop, **Needs you** is the first rail section, present only when non-empty. On phones and
tablets the top bar's **Sessions** button carries the count, and the drawer opens on the list. It
spans every machine, newest first:

```
NEEDS YOU (3)
 ? Waiting   Fix sizing      reet . studio     2m
 ! Sign in   Draft release   site . mini       9m
 v Finished  Tidy docs       reet . studio     1h   [Mark seen]
```

Choosing an entry opens the session. *Finished* entries leave the list once opened or marked seen.
The page title gains a count, `(3) Agents · reet.sh`, and a polite live region announces only
transitions into *Waiting* or *Sign in*, at most once per session per minute.

### YOLO warning

Switching YOLO on opens a confirmation before the switch moves:

```
+-------------------------------------------------------------+
| Let Claude Code act without asking?                         |
| For this one session the agent will run commands, edit and  |
| delete files and use the network on studio without asking   |
| you first. Anything your user on studio can do, it can do.  |
| reet records every use. This can't be changed once the      |
| session starts.                                             |
|                                     [Keep asking] [Turn on] |
+-------------------------------------------------------------+
```

The switch then reads **YOLO on** with a warning colour **and** an icon and the word. The summary line
under the composer adds "Without permission prompts." When unavailable the switch is disabled with the
reason underneath: "An owner or admin can allow YOLO for this account in Settings.", "studio doesn't
allow YOLO.", "This agent has no option reet can use for that.", or "Update reet on that machine to
use YOLO." YOLO resets to off after every launch and is never remembered.

## Remote folder browser protocol (`browse-v1`)

A negotiated, optional control request, built like `worktree-v1`: the controller advertises
`browse-v1` in `X-Reet-Terminal-Features`; a connector that understands it records it and reports the
capability `browse` (omitempty, requires `terminalControl`) unless the machine refuses browsing
locally. The controller never sends a browse request to a connection without it and answers 409
`browse_unsupported`: "Update reet on that machine to browse its folders."

**Request** `Browse{Path, After, Filter, Hidden}`: `Path` absolute (the existing `ValidDir` rule) or
empty for the enrolled home; `After` a continuation name (at most 255 bytes); `Filter` an optional
case-insensitive substring (at most 64 bytes, plain text); `Hidden` a boolean. Flat and comparable.

**Reply** `{path, home, parent, repo, entries, more, scanned, omitted}`:

- `path` is the resolved folder; `home` the enrolled home; `parent` empty at the home.
- `repo` describes `path` itself: `root` (it holds a `.git` directory), `inside` with the top folder's
  path (found by walking up with stat only, never above the home), or `none`.
- `entries`: at most 200 per page, sorted by name (byte order, case-folded), each
  `{name, dir, repoRoot, hidden}`. In `browse-v1` only directories are returned, so `dir` is always
  true: **file names never leave the machine.** The field is kept so a later version could list files
  without a wire change; the web ignores an entry with `dir` false. `repoRoot` is true when the entry
  holds a `.git` **directory** that contains a `HEAD` file (two `lstat` calls). A `.git` file (a linked
  worktree or submodule) is not a repository root here, matching what `worktree-v1` accepts.
  `hidden` is true for names starting with `.`; hidden entries are left out unless `Hidden` is set.
- `more` is true when another page exists (pass the last name as `After`); `scanned` and `omitted`
  count directory entries read and entries left out (files, links leaving the home, unreadable).

**Machine-side rules.**

- **Containment.** `Path` is cleaned, resolved with symlinks followed, and must be the home or inside
  it, exactly as the launch folder policy. A symlinked entry is listed only when its target resolves
  to a directory inside the home; it is listed under its own name and entering it resolves the target
  again. Nothing above the home is ever listed, including through `..` or a link.
- **No contents.** Only `readdir` and `lstat`/`stat`. No file is opened, no git command runs, no
  repository configuration is read.
- **Bounds.** At most 5,000 directory entries read per request, at most 200 returned, names at most
  255 bytes (longer names are omitted and counted), reply at most 12 KiB so it fits the 16 KiB control
  frame. A 3-second deadline per request: a slow or network filesystem returns what it has with
  `more` set, and per-entry `repoRoot` checks that miss the deadline report `false` with the page
  marked `partial`.
- **Concurrency.** One browse at a time per connection; a second is answered `conflict` at once.
- **Refusal.** `reet terminal browse off` on the machine (local, like terminal consent) removes the
  capability and makes the connector answer `denied`. Default on, because every folder listed is one
  the enrolled user can already open in a terminal session.
- **Errors.** Not found, not a directory, outside the home, permission denied: each a typed reason
  mapped to one sentence ("That folder isn't on studio.", "That folder is outside the home folder reet
  was set up with.", "reet can't read that folder.").

**Controller rules.** Writers only, owner-scoped, canonical origin, the terminal rollout flag. Rate
limit 20 browse requests per 10 seconds per account and machine (429 `browse_rate_limited`: "Slow down
a moment, then try again."). Controller deadline 6 seconds. An offline or reconnecting machine is
answered at once with the existing readiness sentence; the dialog keeps the typed path and lets the
person add the project without browsing (it is checked again at launch). **The controller stores
nothing from a browse**: no cache, no log of names. The audit trail gets one `folder-browse` event per
opened browser (machine, starting folder, number of pages), not one per click.

### Branch listing (`branches-v1`)

The composer's branch picker and the dialog's base-branch suggestions need the machine's branches.
That needs git, so it is a separate negotiated request, `Branches{Dir, Filter}`, under `branches-v1`
and capability `branches` (requires `worktree`). The connector checks `Dir` as the worktree create
does (repository root, inside the home) and runs exactly one read-only command already allowed by the
worktree rules: `git -C <dir> for-each-ref --count=201 --sort=-committerdate
--format=%(refname) refs/heads refs/remotes`, with the worktree environment and hooks rules and a
5-second timeout. The reply is `{current, default, branches, more}`: `current` the checked-out branch
(from `symbolic-ref`, or empty when detached), `default` the remote's default if `refs/remotes/origin/HEAD`
exists, at most 200 short names (each a valid ref name of at most 255 bytes), newest first. Rate limit
10 per 10 seconds per account and machine; nothing stored.

## Project registry

A **project** (per account): `id`, `host` (the machine's enrollment id), `dir` (absolute, cleaned,
`ValidDir`), `name` (1–80 characters, the Projects naming rules, unique per account ignoring case;
defaults to the folder name, with ` (2)` appended on a clash), `isRepo` (as last seen by browse or
launch, informational only), `baseBranch` (optional, a valid ref name, at most 255 bytes),
`worktreeDefault` (`inherit`, `yes`, `no`), `defaultTool` and `defaultModel` (optional, the launch
rules), `position` (rail order), `createdAt`, `updatedAt`, `version`. A machine and folder may be
registered once per account (409 `project_exists`). At most 100 projects per account (409
`project_limit`). The account gains two settings: **default base branch** (blank = the repository's
own default) and **worktrees by default** (yes, initially).

**API** (under `/api/v2`, the Projects access rules, `Cache-Control: no-store`, one-sentence refusals
naming the field):

- `GET /folder-projects` → `{projects}`; `POST /folder-projects` → 201; `GET`, `PATCH`, `DELETE
  /folder-projects/{id}` with `ETag`/`If-Match` exactly as Projects (409 `project_changed` on a stale
  version; without the header the last write wins; the web always sends it).
- `POST /folder-projects/reorder` with `{ids, versions}`.
- `GET /machines/{agentId}/browse?path=&after=&filter=&hidden=` and
  `GET /machines/{agentId}/branches?dir=&filter=` for the two machine requests.
- Delete removes only the registry entry. Sessions and worktrees are untouched; the answer says
  "Project removed. Its sessions and any worktrees stay on studio."

The path `folder-projects` is internal; it becomes `projects` once the old routes are removed (step 5
of the delivery), keeping the old shape's `GET /projects` answering 410 `projects_moved` for a
release so stale tabs explain themselves.

**Session metadata.** The controller already stores each create request. It gains `project_id`
(nullable), and the session listing returns, for the rail, `tool`, `model`, `effort`, `yolo`,
`projectId`, `worktreeBranch` and the attention fields. The prompt is never in the listing; only its
first line, at most 80 characters, as the default title, which the composer shows the person before
sending ("The session will be titled …").

## The New session composer

**Order of choices**, each prefilled from the last one so a second session is one press:

1. **Project** (the folder chip): registered projects, grouped by machine, with search; *Add a project…*
   opens the dialog; *Home folder on…* starts without a project. Unready machines are listed disabled
   with their sentence.
2. **Branch.** With a worktree, the base branch, filled from the project's `baseBranch`, else the
   account default, else the repository's default from `branches-v1`, else the current branch; the
   picker lists the machine's branches with a filter. Without a worktree the chip is read-only and shows
   the current branch: "Works in the checkout on main." reet never switches the branch of a checkout.
   A non-repository project hides the chip and the worktree choice.
3. **Prompt.** Optional, at most 2,000 bytes, the existing launch rules (no leading `-`, two words for
   agents that need it). Enter inserts a newline on touch screens; on a hardware keyboard Enter sends
   and Shift+Enter adds a line. The note "Secrets don't belong here. The prompt is visible in the
   machine's process list." shows under the box when it has text (for ACP sessions in slice 2 the prompt
   travels over stdin instead, and the note changes).
4. **Agent, model, thinking.** Agent from the catalogue (installed agents first; others disabled with
   the reason); model suggestions plus *Other…*; **Thinking** shown only for agents whose preset has a
   confirmed effort option (`low`, `medium`, `high`; *Default* sends nothing).
5. **Worktree**: *New worktree* or *This checkout*, defaulting from the project, then the account.
6. **YOLO**: off; see [YOLO warning](#yolo-warning).

A summary line states exactly what will happen. Send is enabled once a project or home folder and an
agent are chosen. While the request is in flight the composer is locked and the line reads "Starting
Claude Code on studio…"; on success the session opens in place and the composer clears its prompt.

**Mapping onto the wire.** One `Create` per session through the existing idempotent create path: `Tool`,
`Model`, `Dir` (the project folder), `Prompt`, `Worktree` (a fresh run token) and the project slug, as
`launch-v1` and `worktree-v1` define them, plus three new flat fields:

- `Base` (`branches-v1`): the base branch name. The connector resolves it with `rev-parse --verify
  --end-of-options refs/heads/<base>^{commit}` (then `refs/remotes/<base>`), and uses that commit in
  place of `HEAD` in the one `worktree add`. Unknown → `worktree_refused` reason `base_missing`: "That
  branch isn't in the repository on studio."
- `Effort` (`agents-v2`): `low`, `medium` or `high`, mapped by the preset to its own confirmed option.
- `Yolo` (`agents-v2`): a boolean; the connector maps it to that preset's allowlisted option and
  refuses if the machine has YOLO switched off or the preset has none.

The lost-reply rules of Start an agent apply unchanged: the request id and parameters are kept, the
retry button sends exactly the same request, nothing is kept in browser storage.

**Several projects at once.** In the rail, **Select** (or long-press on touch) adds checkboxes to
project rows. With two or more selected, the composer's project chip reads "3 projects" and the
branch chip reads "Each project's default". Send creates a **run** through `internal/projectrun`
unchanged in its rules: one member per project, at most 2 in flight and 1 per machine, derived request
ids, per-member retry, partial failure is normal. The run's `projectName` becomes the list of names;
runs are listed in a **Runs** item in the rail's Projects menu (the newest 20). YOLO applies to every
member and the warning names every machine.

## The sessions rail

**Model.** Sessions from the existing account-wide inventory (all machines), joined with projects by
`projectId`. Sessions without one (plain shells, older sessions, home-folder agents) are matched by
exact machine and folder for display only, else grouped under **Other on \<machine\>**.

**Order of sections:** search and filter bar; **New session**; **Needs you**; one group per project in
the person's project order, each with its session count; **Other on \<machine\>** per machine; the
**Projects** section (each project's name, path and machine, `+` to add, a menu per project: New
session here, Edit, Worktrees, Runs, Remove from reet). Within a group, sessions sort by last state
change, newest first; the existing custom order, colours and group-by choices stay as options under
filters. Groups collapse; collapse and width are remembered per account in this browser, as today.

**Search** (`/` or the icon) matches titles, agent, model, branch, project and machine names.
**Filters**: state (needs you, running, finished, ended), agent, machine, YOLO only, worktree only.

**States.** Loading shows the last known list with "Checking…" (no skeleton flash); an error keeps the
last list with "Couldn't refresh. Showing what we last saw." and a retry; an offline machine's sessions
stay listed with "studio is offline" on the group and actions that need it disabled; no projects shows
"Add a project to start an agent in one of your folders." with the add button; no machines shows the
existing add-a-machine prompt. Polling stays at 15 seconds while visible.

## Attention state

| State | Structured (ACP) sessions, slice 2 | Terminal sessions, this pass |
| --- | --- | --- |
| Starting | create accepted, agent not yet ready | create accepted, program not yet running |
| Working | a prompt turn is running | output within the last 30 s (tmux `window_activity`) shows *Working*; otherwise *Quiet* |
| Waiting for input | a permission request or an ended turn awaiting a reply | the program rang the terminal bell since you last looked (tmux `monitor-bell`) |
| Needs sign-in | the adapter's authentication-required error | not detected |
| Finished | the turn ended; for terminal programs, exited with status 0 | exited 0 (`remain-on-exit` status) |
| Failed | an error ended the session | exited non-zero: *Exited N* |

Terminal agents get **honest, coarse** states: *Working/Quiet* is output activity, not progress;
*Waiting* is only the bell. The row's tooltip and the session header say where the state came from
("From terminal activity"). Exact states arrive with ACP. The connector reports per session
`{activityAt, bellAt, exitStatus}` in the existing inventory reply under `agents-v2`; the controller
derives the state and keeps a per-user `seenAt` per session (so "changed since you looked" is
per-person and survives reloads). Nothing in the attention data contains terminal output.

## Agent catalogue

One table in the connector, keyed by tool id: display name, binary, how a model, a prompt, an effort
and YOLO are passed, the safest permission default, and whether it speaks ACP (natively, through a
pinned adapter, or not). Each option is confirmed from the real CLI's help text on a machine that has
it, or ships disabled. Reported to the controller only under `agents-v2`: a connector that did not
negotiate it reports only `claude`, `codex`, `opencode`, `gemini`, so an older controller never sees an
unknown id. The web holds display names and model suggestions only.

| Id | Agent | Notes for this pass |
| --- | --- | --- |
| `claude`, `codex`, `opencode`, `gemini` | Claude Code, Codex, OpenCode, Gemini CLI | Shipped presets; add effort and YOLO options once confirmed. Claude Code and Codex use adapters for ACP. |
| `pi`, `cursor` | Pi, Cursor Agent | Ids exist; launch syntax still to confirm. |
| `cline` | Cline | Terminal preset passes `--auto-approve false` so its default is to ask. |
| others | every remaining agent on the reference designs' lists | Each added only with confirmed syntax; the final list is open question 3. |

## Migration

Runs once when the Projects page is removed (delivery step 5), inside one transaction, idempotent:

1. Every distinct machine and folder across all saved Projects becomes one folder project. The name is
   the folder's name (de-duplicated with ` (2)`); `defaultTool`/`defaultModel` come from the member,
   else the Project's defaults; `worktreeDefault` is `yes` or `no` from `useWorktrees`; position follows
   Project then member order. A folder already registered is skipped.
2. **Shared prompts are not carried over** (a project has no prompt). Before migrating, the old page
   shows a notice for one release: "Projects are moving to the Agents home. Shared prompts won't move;
   copy any you need." The migration writes one audit event per migrated Project naming the folder
   projects created (never the prompt), and then deletes the shared prompt text.
3. Old **runs** are kept read-only. Their recorded worktrees appear under each folder project's
   **Worktrees** menu with the existing Check and remove sheet, so no worktree loses its safe remove
   path. Old run pages are not reachable by URL any more.
4. The old `projects` and `project_members` tables are dropped one release later.

## Security and privacy

- **Access.** Accounts mode, write role, canonical origin, rollout flag, owner scoping on every query,
  as Projects. Viewers, API and MCP tokens, single-token and open modes get no Agents home, browse,
  branches, project registry or composer, and the routes refuse them with the Projects sentences.
- **Paths** are the owner's own and are stored only for registered projects and created sessions.
  Browse results are not stored. Paths never go in the address bar except a project id.
- **Prompts**: unchanged from Start an agent: stored with the create request and in the machine's
  private registry, never in audit, logs, browser storage or the URL; visible in the process list while
  passed as an argument.
- **Secrets**: nothing in this design reads or sends file contents, environment values or git
  configuration from the machine; browse is stat-only.
- **YOLO**: the account setting (owner/admin; changes audited as `yolo-allowed` / `yolo-disallowed`);
  the machine refusal `reet terminal yolo off` removes the capability and is re-read on every create;
  the controller refuses a YOLO create without both (403 `yolo_not_allowed`, 409 `yolo_refused`); each
  YOLO launch writes `terminal-launch` with `yolo: true` plus a `yolo-launch` event; in ACP sessions
  each automatic `allow_once` is its own `yolo-permission` event (session, tool kind, title truncated to
  120 characters, never file contents).
- **Audit events added**: `project-add`, `project-update`, `project-remove`, `projects-migrated`,
  `folder-browse`, `yolo-allowed`, `yolo-disallowed`, `yolo-launch`, `yolo-permission` (slice 2),
  `adapter-install` (slice 2). `terminal-launch` gains effort, base branch and YOLO.
- **Rate limits**: browse 20/10 s and branches 10/10 s per account and machine; project writes 30 per
  minute per account; session creation keeps 8 per machine and 32 per account.

## Accessibility and responsive rules

- Breakpoints: phone is `PHONE_SCREEN` from `terminal/sizing.ts` (650 px wide, or sideways 500 px tall
  on touch); tablet 651–1099 px; desktop 1100 px and wider.
- The rail is a `nav` with labelled sections and disclosure buttons, not an ARIA tree. Every row is a
  link-like button with an accessible name that includes title, state, agent, machine and YOLO.
- The composer is a form: labelled textarea, each picker a button that opens a listbox popover
  (desktop) or a bottom sheet dialog (phone and tablet portrait), Escape closes and returns focus.
  Send is a real button named "Start session". The YOLO switch is `role="switch"` named "YOLO: skip
  the agent's permission prompts".
- Status in text and icon as well as colour; focus visible; reduced motion respected (the drawer and
  sheets appear without sliding); touch targets at least 44 px (40 px for the terminal's own keys, as
  today).
- **Phone zoom contract** (`docs/ui-verification.md`, Phones: long names and page zoom): no screen may
  lay out wider than the device; long project names, paths and branch names wrap or truncate with
  their full text available; inputs use a 16 px font. The home, the drawer, the composer sheets and the
  add-project dialog with a long path join the `phone-zoom.e2e.mjs` matrix.
- **Keyboard-open layout**: the phone terminal's rules (`docs/terminal-workspace.md`, Phones) are
  extended to focus in the composer: header hidden, composer above the keyboard, same height rule.

## Testing strategy

- **Go.** Protocol round trips and strictness for `Browse`, `Branches`, `Base`, `Effort`, `Yolo`;
  feature negotiation and old-peer refusal for `browse-v1`, `branches-v1`, `agents-v2` (an old
  controller never receives a new tool id); connector browse against real temporary trees: symlink
  escape, a link to a folder inside home, `..`, hidden entries, a `.git` file versus directory, a
  repository root without `HEAD`, unreadable folders, 5,000-entry cap, paging, the deadline with a
  slowed filesystem fake, reply size under the frame limit, and a test that no file is opened (an
  unreadable file in the folder must not change the reply); branches with the exact argument vector;
  worktree create from a named base; store and API for projects (limits, uniqueness, If-Match, owner
  scoping, viewers blocked, rate limits); migration (dedupe, names, idempotent re-run, prompts deleted,
  runs kept); YOLO gates (each of the three refusals) and audit rows; attention derivation from
  activity, bell and exit. A real two-host test extends the existing one with browse, branches and a
  YOLO launch against recording fake CLIs.
- **Web unit** (`npm test`): `folderBrowser.ts` (breadcrumbs, filter, repository messages),
  `agentsHome.ts` (rail grouping and ordering, search, filters, attention list, summary sentence,
  composer defaults precedence for base branch and worktree), `catalogue.ts`.
- **Browser fixtures** with synthetic API answers: `agents-home.e2e.mjs` (empty state, composer flow,
  multi-select run, rail states including offline and error, attention list, YOLO warning and every
  disabled reason, lost-reply retry, nothing in storage, viewer and legacy modes see nothing, redirects
  from `/projects`), `add-project.e2e.mjs` (browse, paging, hidden, filter, not-a-repository, inside a
  repository, offline machine, rate limit). Matrix: 1360, 1024 and 768 px, phone 412, 390, 360 and 320 px
  with `isMobile` and `hasTouch`, sideways 844 × 390, light and dark, keyboard-open emulation. Axe at
  390 and 1360 px reported honestly (violations and the `incomplete` count).
- **Mutation checks**: browse containment, "no file names", the hidden filter, YOLO resetting after
  launch, YOLO refused without the account setting, never sending new fields to an old machine, the
  16 px input font, drawer closing on selection, the attention dedupe.
- **CI budget**: the new fixtures add at most 90 seconds to `terminal-browser.yml`, split across the
  existing jobs; `phone-zoom` grows by the new screens and stays under 60 seconds.

## Docs to update

`docs/start-agent-session.md` (line 12, "No preset uses a flag that skips the agent's own permission
prompts"), `docs/terminal-launch.md` (the Presets paragraph that rules out approval-bypass and
auto-approve flags, and the test that pins the only flags), and the start-agent spec's "No
permission-bypass flags" line (amended, not rewritten): each to say YOLO exists, is off by default and
how it is gated. Also `docs/projects.md` (rewritten for folder projects), `docs/terminal-worktrees.md`
(base branch, branch listing), `docs/terminal-workspace.md` (the home, rail and phones), a new
`docs/folder-browse.md`, `docs/control-centre.md` (landing change), `docs/roadmap.md`, and
`docs/ui-verification.md` (each new matrix).

## Delivery: pull-request slices and order

Each step merges when its checks are green and the broad review has passed; each leaves the product
usable.

1. **Folder browse and branches on the wire.** `browse-v1`, `branches-v1`, `Base` on worktree create,
   connector, controller endpoints, rate limits, audit, Go and two-host tests, `docs/folder-browse.md`.
   No UI. Green: old peers unaffected.
2. **Project registry.** Store, API with If-Match, account defaults, `project_id` and metadata on the
   session listing. No UI change. Old Projects untouched.
3. **Add project and the rail's Projects section** on today's terminal page, with the folder browser,
   on the shared dialog shell. Fixture and phone-zoom screens. Old Projects page still there.
4. **The Agents home shell.** Restyled rail with grouping by project, search, filters, New session;
   the empty-state composer with the four shipped agents, models, base branch and worktree; the session
   header; `/` as landing for writers; nav changes; phones drawer and docked composer with the keyboard
   layout. Start an agent's dialog stays for the Machines page.
5. **Multi-select runs, migration and retiring Projects.** Run from a selection, Worktrees and Runs in
   the project menu, the one-release notice, the migration, redirects, nav entry removed, docs.
6. **Catalogue and YOLO.** `agents-v2`, new presets as confirmed (Cline first with
   `--auto-approve false`), effort, the account setting, machine refusal, warning, badges, audit, docs
   that said auto-approve is not offered.
7. **Attention.** Activity, bell and exit reporting, derived states, per-user seen, Needs you list,
   title count.

Slice 2 (own spec): the ACP runner in tmux, the Claude adapter's confirmed pinned install, the chat
view, exact attention states, machine-only transcripts. Steps 6 and 7 can swap if the owner prefers
YOLO later; steps 1–3 can be reviewed in parallel with 4's design review.

## Risks

- **Large UI change on the most used page.** Mitigated by steps 3 and 4 landing on today's terminal page
  behind the existing fixtures, and keeping `/terminals` links working.
- **Losing Projects' features in migration.** Mitigated by keeping runs and worktree removal; shared
  prompts are the one real loss (open question 1, alternative A keeps them).
- **Coarse attention looks wrong.** Terminal states are labelled with their source and never say
  "waiting" without a bell.
- **YOLO harm.** Three independent gates, per-session only, reset after launch, audited per use.
- **Unconfirmed CLI flags.** Every option ships disabled until confirmed from help text.
- **Phone keyboards differ.** The height rule is reused, not reinvented; physical devices remain
  unchecked (as stated in UI verification).
- **Concurrent dialog work.** The add-project dialog depends on the shared dialog shell; step 3 waits
  for it.

## Open questions for the owner (ranked, with recommendations)

1. **What happens to today's Projects?** (see the top) — *Recommend:* one folder per project,
   multi-select keeps Start all, retire the page, migrate folders, keep runs read-only.
2. **Should shared prompts be kept somewhere?** — *Recommend:* no; show the one-release notice. If kept,
   add optional per-project "prompt snippets" later.
3. **Which agents exactly make the catalogue's first cut?** The reference designs' lists were seen as
   screenshots, not text. — *Recommend:* confirm the list; ship Cline plus the shipped four first, then
   each other agent as its syntax is confirmed.
4. **Fold Control into Machines?** — *Recommend:* yes, in a later pass; for now Control moves under More
   for writers and stays the landing for viewers.
5. **Default for a machine's YOLO refusal.** — *Recommend:* allowed unless the machine says
   `reet terminal yolo off`; the account setting is the main gate. The stricter choice is off until the
   machine opts in.
6. **Always dark, or follow the theme?** — *Recommend:* follow the person's theme, with dark as the
   design target and light checked to the same standard.
7. **Project limit of 100 per account.** — *Recommend:* 100; raise if anyone reaches it.
8. **Should Enter send on phones?** — *Recommend:* no; Enter adds a line on touch screens, the round
   button sends.
9. **Attention notifications outside the page (push, email).** — *Recommend:* later, reusing
   `docs/notifications.md`, once ACP gives exact states.

## What this does not establish

That an agent is signed in, understands the prompt or makes progress; that *Working* means useful work;
that a branch contains anything; that a repository's hooks, build or tests are safe. YOLO's warning is
the only protection once the agent runs. No physical phone, Safari or Firefox check is planned beyond
the Chromium fixtures.

## Self-review

- **Placeholders:** none left as "TBD". Items that depend on facts not yet known (exact CLI flags, the
  reference lists) are stated as "ship disabled until confirmed" and open question 3.
- **Contradictions checked:** browse is stat-only while branches runs one read-only git command, so
  they are separate features; "no separate Projects page" holds because runs and worktrees move into
  the rail's project menu; YOLO is per session and never remembered everywhere it appears; the 16 KiB
  frame limit is respected by paging browse replies; worktree accepts only a `.git` directory and browse
  marks only that as a repository root.
- **Scope:** ACP chat, adapter install and transcripts are recorded decisions but specified in slice 2;
  notifications, git-remote identity and wider folder roots stay out. The seven steps are each a
  reviewable pull request; step 4 is the largest and may split into rail and composer if review asks.
