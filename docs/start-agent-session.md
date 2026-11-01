# Start an agent session

Home starts sessions from its launch bar ([below](#on-home-the-launch-bar)). The **Start an agent**
dialog stays for Machines and the Sessions panel: pick a machine, an agent, a folder and, if you like,
a model and an opening prompt. Press **Start**
and you land in a new browser terminal session with that agent already running, or in the reet TUI when you
choose it. The dialog is
`web/src/lib/StartAgentDialog.svelte`. The launch request it sends and how a machine handles it are
in [Launch requests](terminal-launch.md). The design spec was removed in the docs tidy-up
(see git history before bd9e56c).

The browser never sends a command line. It names a tool id (`claude`, `codex`, `opencode` or
`gemini`) and the values you chose. The machine looks the tool up in a table built into its own
connector and decides the program and its flags itself. No preset uses a flag that skips the
agent's own permission prompts.

## The reet TUI or the agent TUI

For OpenCode on a machine that advertises `acp`, the dialog's **Session** choice is **reet TUI** by default; choose
**OpenCode TUI** for the terminal UI instead. It sends `structured: true` and
the person's trust confirmation. The **Model** control chooses the model; **Agent default** uses OpenCode's configured
model, and Thinking stays OpenCode's own. The opening prompt is the first message. The new session opens on its reet TUI, where OpenCode asks before it edits a file
or runs a command with **Allow once** / **Deny**. On a machine that negotiated `session-mode-v1`, the session header's
**OpenCode TUI** button switches the running session to OpenCode's own terminal UI and **reet TUI** switches it back,
once the agent is idle; a session started in the OpenCode TUI cannot move to the reet TUI. Terminal scrollback is not kept
across a switch, and returning to the reet TUI asks you to trust a folder with OpenCode plugins. **Attach image** (or paste/drop)
sends a file through the launch-upload path and puts its path in the message. A closed reet TUI session offers **Continue in reet TUI with
history…**, which loads OpenCode's stored conversation into a new session row. The conditions, deadlines and limits are in
[OpenCode in the reet TUI](structured-sessions.md).

## Who sees it

Only an account in accounts mode with a write role sees the entry points, just as for browser
shells. Viewers, API and MCP tokens and legacy sign-in modes don't see them. The server refuses them
too. The usual terminal rules still apply: the rollout flag, the canonical origin, the machine's
local consent, and the session limits (8 per machine, 32 per account).

## Where to start it

- **Sessions → New agent session.** It appears under **Your sessions** in the sessions sidebar,
  in a machine's menu next to **New terminal**, and in the empty workspace next to **New terminal**.
  The machine you were on is preselected. On success the new session opens in the workspace.
- **Machines → Start an agent** on every enrolled machine's card. While the machine isn't ready the
  button is disabled and the reason is written underneath; a machine the terminal list doesn't know
  yet says "This machine hasn't connected for terminals yet." If its readiness couldn't be read, it says
  "Couldn't check this machine just now. Try again in a moment." On success you are taken to
  `/sessions?agent=…&session=…`.

**Close** and Escape both close the dialog without sending anything and return focus to the button
you pressed, except while a request is in flight (see below).

Home has no Start an agent button. Its rail card's **+**, and **New session here** in the card's
menu, start a session in that project at once with the project's default agent, model, base branch
and worktree choice (no dialog, no prompt). Choosing the card itself still puts the project in the
launch bar's sentence and moves focus to the prompt.

## On Home: the launch bar

The bar is docked at the foot of Home (`web/src/lib/launch/LaunchBar.svelte`; its rules are in
`web/src/lib/launchBar.ts`). It is a **Describe the job** box, **Start**, and the launch sentence,
whose bold words are the choices.

For OpenCode on a machine that advertises `acp`, the sentence says **in reet TUI** by default and Start
opens the reet TUI instead of the agent terminal UI. Home asks you to confirm **I trust this folder** first, because
OpenCode runs any plugins in `.opencode` when the reet TUI starts. The **TUI** word switches it to **in OpenCode TUI**;
the worktree and thinking choices come back for that terminal. The Model word chooses the chat's model; leave it at
**default** to use OpenCode's configured model. The reet TUI runs in the chosen project folder (not a worktree) and can
attach images through Home's existing pasted-image path. It starts in one folder at a time. A session's actions menu has **Open a terminal here…**, which starts a plain shell in
the same folder as a separate session.

> Run **Claude Code** with **default** on **Store rewrite** at **studio** from **main** in **a fresh worktree** · thinking **high**

Each bold word is a button named for its slot and value ("Agent: Claude Code") that opens a list:
Agent, Model, Project, Branch, Worktree and, for an agent that takes it, Thinking. The machine
follows the project and is named, not chosen, except for **Home folder on** a machine. A project on a
machine that isn't ready is listed, disabled, with that machine's readiness sentence.

- **What it opens on.** The last launch, read from the sessions list (never from browser storage),
  with the prompt empty and Approvals on the project's own default: **Ask me**, or **YOLO** when the
  project was saved that way. Choosing **YOLO** in the bar opens the confirmation dialog; confirming
  turns it on for this launch and does **not** start anything, so Start stays the person's decision.
  A project default is confirmed the same way when Start is pressed. YOLO is available only when an
  owner or admin has allowed it for the account, the machine accepts it and the agent has a reviewed
  option. The controller refuses it with 403 `yolo_not_allowed` or 409 `yolo_refused` otherwise.
- **The rail.** Choosing a rail card puts that project in the sentence with its own default agent,
  model, base branch and worktree choice, then moves focus to the prompt. `/?project=<id>` does the
  same once. The card's **+** and **New session here** start at once instead, sending the bar's Start
  with that project; a refusal appears on the bar, with the project left in the sentence so it can be
  fixed. While a start is out or kept for a retry, any of these changes nothing and the status line
  says "Finish or discard the current start first." ("Wait for the current start to finish." while the
  request is out), after what the bar already said.
- **Machines on Home.** Each machine card's **+** and its menu's **New session…** ask what to start
  there: one of that machine's folder projects, which starts at once with the project's defaults, or
  **Terminal session**, a plain shell in the machine's home folder. Its menu also offers **New
  terminal**, **Open on Machines** and **Skills & sync**. The machine's pills report what its last
  scan says is installed: agents, skills and MCP (which no agent reports yet, so it reads "MCP —").
- **Base branch.** The Branch slot is the base of the fresh worktree. `base` is sent only when it is
  chosen: by the person, by the project's base branch or by the account's default base branch.
  Otherwise nothing is sent, the worktree starts from the checkout's current commit, and the sentence
  names the checkout's current branch as the machine reported it. A branch only suggested from the
  machine's list is shown, never sent unless picked. In the checkout there is no Branch slot; a line
  under the sentence says "Works in the checkout on main."
- **Branch listing.** The Branch list comes from the machine (`GET /api/v2/machines/{agentId}/branches`,
  see [Folder browse and branch listing](folder-browse.md)): read once per folder, at most 200 names,
  with a filter that asks the machine again. A machine that can't list branches says "Update reet on
  that machine to list its branches."
- **Starter prompt.** A project's starter prompt is offered as **Use the starter prompt** while the
  box is empty. It is never put in the box, or sent, by itself.
- **Title.** The prompt's first line is the session's title, shown as Titled "…" before sending (and
  stored as the session title); without a prompt it is \<Agent\> · \<folder name\>.
- **Keys.** Enter sends from a hardware keyboard; Shift+Enter adds a line. Ctrl+Enter and Cmd+Enter
  also send. On touch no key sends, Enter included: only **Start** does.
- **Images.** Paste (or drop) a PNG, JPEG, GIF or WebP into the prompt to attach it. Each image is
  uploaded to the chosen machine before the session starts, and its absolute path is added to the
  opening prompt. At most 20 images and 50 MiB at once, and one project at a time. A machine with
  uploads off, or an older reet, says so and Start waits.
- **Starting, refusals and lost replies** work as in the dialog: the bar is locked while the request
  is out; a definite refusal unlocks it with its sentence; a lost reply keeps the same request (request
  id, run token and every field) for **Try the same request again**, and **Discard this request**
  drops it.
- **Phone and tablet.** A phone (and a phone held sideways) folds the sentence into one summary line
  with **Change**, which opens the **Change the launch** sheet with the slots as large rows and
  Approvals. A tablet shows the shorter sentence (no model or worktree) with its own **Change** for
  the rest. Only one Change shows at a time.

### Start in several projects

**Select projects** in the rail heading's menu, or a long press on a card on touch, turns the cards
into checkboxes (44 px targets; the 56 px strip shows how many are chosen). With 2 to 12 projects
chosen the sentence reads "on 3 projects at studio from each project's default", and Start sends one
request, never a session create:

`POST /api/v2/folder-projects/runs` with `{requestId, projectIds, tool, model, effort, prompt, worktree}`

Each project starts from its own base: its base branch, else the account's default, else the
checkout's current commit; the one-project Branch choice is never used. The controller starts the
members, at most two at a time and one per machine. The **Start in several projects** sheet counts
only what the controller says started ("2 of 3 started"), opens each started session, and offers
**Retry** for a member that failed (`POST …/runs/{runId}/members/{position}/retry`). Closed with a
member waiting or failed, the bar keeps **Show run status** until every member has started or a later
start in one project is made. **Done** leaves Select.

A lost reply keeps the same run request for **Try the same request again**, and so does a refusal for
speed (429 `project_rate_limited`). Other refusals are in this start's own words, for example "One of
those projects is still starting. Wait for it to finish, then start again." (409 `run_in_progress`)
and "Choose 2 to 12 different projects." YOLO applies to every member, after the same account,
machine and agent gates.

## The dialog

It is one screen:

| Field | What it offers |
| --- | --- |
| **Machine** | Every terminal machine. A machine that isn't ready is listed but can't be chosen. If the one you came from isn't ready and another is, the other is chosen and the dialog says "\<from\> isn't ready, so \<other\> is selected." **Not listed? Add a machine** opens [Add a machine](add-machine-in-context.md). |
| **Agent** | Claude Code, Codex, OpenCode, Gemini CLI and, on a machine with `agents-v2`, Cline. Only an agent the machine reports as installed can be chosen. Any other is listed but disabled, with the reason "Not installed on <machine>." |
| **Model** (optional) | The shared picker: **Agent default**, the account's favourites for this agent, the machine's list by provider, reet's suggestions and **Other…** for any model name made of letters, digits and `. _ : / @ + -`, up to 100 characters. A star adds or removes a favourite. A list is only a hint; if a model name doesn't exist, the agent says so itself. Nothing is sent for Agent default. |
| **Thinking** | Only for an agent that takes an effort level (Cline): **Agent default**, low, medium or high. Nothing is sent for Agent default. |
| **Folder** | **Home folder**, the project folders that machine has reported, and **Type a path…**. A typed path must be absolute. The machine decides whether it may start there: the folder must exist and be inside the home folder it was enrolled with. |
| **Opening prompt** (optional) | Up to 2,000 bytes, with a byte counter. It can't start with a dash. For Claude Code, Codex and Cline it must be at least two words, because a single word could be read as one of their subcommands. "Secrets don't belong here." |

Opening the dialog with a project applies that project's default agent, model and thinking;
otherwise the account defaults apply, then a favourite marked for the agent. Changing the machine or
the agent resets the model and folder to their defaults. The model list's status line and Refresh are
honest about a cache read; a failed read never disables Start.

Below the fields, one line says exactly what will happen, for example "Starts Claude Code in
/home/you/work/site on studio. Nothing runs until you press Start." When there is a prompt, it adds
"The prompt is kept with the session record and is visible to other users of that machine in its
process list."

The new session is titled \<Agent\> · \<folder name\>, for example "Claude Code · site". With the
home folder it is just the agent's name. You can rename it afterwards.

## What each state says

The status line at the foot of the dialog is a polite live region. Each message is plain text, not
only a colour.

| State | What you see |
| --- | --- |
| Starting | **Starting…** on the button and "Starting \<Agent\> on \<machine\>… this can take a few seconds." Every field, **Close** and Escape wait until the request finishes. A request gives up after 15 seconds. |
| A field needs fixing | The field's own message, and the same message in the status line, with focus on that field. The status line clears as soon as you edit a field. |
| Old machine | "Update reet on this machine to start agents." Every agent is disabled and Start is off. |
| Machine not ready | The existing readiness sentence, for example "The machine is reconnecting. Existing shells will be kept." Start is off. |
| No agent installed | "None of the agents reet can start is installed on this machine." |
| Started | The dialog closes and you land in the new session. |

When the server refuses a start for certain, the dialog shows its sentence, unlocks the fields and
moves focus to the field concerned. Your next Start is a new request:

| Refusal | Sentence |
| --- | --- |
| Folder refused by the machine (422 `folder_unavailable`) | "That folder isn't available on that machine." |
| Agent not installed (422 `tool_not_installed`) | "That agent isn't installed on that machine." |
| One-word prompt for Claude Code or Codex (422 `prompt_unsupported`) | "Write the opening prompt as at least two words." |
| Machine too old for launch requests (409 `launch_unsupported`) | "Update reet on that machine to start agents." |
| Session limit (409) | "The terminal session limit has been reached. End a session and try again." The server doesn't say whether the per-machine or the per-account limit was reached. |
| The same request with different details (409) | "That request changed. Start it again." |
| Browser terminals switched off on the server (403) | "Browser terminals are switched off on this server." |
| Signed out (401) | "You've been signed out. Sign in again to continue." On the Machines page the sign-in card then takes over. |
| Answered, but the machine didn't start it (a 200 whose session is `lost` or `ended`, for example when terminal access was switched off on the machine just before) | "That machine didn't start the agent. Check its terminal settings on that machine and try again." Nothing is running, so you can start again. |
| Anything else (another 400, 403, 404 or 422) | "Something changed. Check again." |

## When the reply is lost

If no answer comes back (the network drops or the 15 seconds run out), or the server gives any
reply other than a success or one of the definite refusals above (400, 401, 403, 404, 409 or 422),
the dialog can't know whether the agent started. It says:

> We didn't hear back, so the agent may have started. Check your sessions before starting another.
> If you close this and start again, a second agent may start.

A reconnecting machine (503) says "That machine is reconnecting. Try again in a moment." followed by
the same last two sentences.

The fields stay locked and the button reads **Try the same request again**. It sends the same
request id with exactly the same details. A start that already happened is returned, not started
twice. If you close the dialog instead, the request is forgotten with it: the prompt is never kept
past the dialog. A new dialog sends a new request, so check your sessions first. If the first
attempt did start, it appears in your sessions list on the next refresh.

## Safety and privacy

- **Command line.** The page never sends a program name or a command line. The model and the prompt
  each reach the agent as a single argument, never through a shell.
- **Where the prompt is kept.** It is stored with the create request in the controller's database
  and in the machine's private registry file, which only its owner can read. It is never written to
  the audit trail, to logs, to browser storage or to the address bar.
- **Process list.** The prompt is on the agent's command line, so other users of that machine can
  see it in the process list. Secrets don't belong in a prompt.
- **Browser storage.** Nothing about the session, folder, model or prompt is written there. The
  prompt lives only in the open dialog and is cleared when it closes.
- **Activity.** A start writes one audit event, which Activity (in your account) shows as, for example, "Started
  Claude Code with opus in /home/you/work/site on studio". It never includes the prompt.

## Limits

- Agents: `claude`, `codex`, `opencode`, `gemini` and, with `agents-v2`, `cline`. A machine offers only
  the ones whose program it finds on its `PATH`. `pi` and `cursor` aren't offered until their launch
  syntax is confirmed.
- Folder: inside the enrolled home, symlinks followed. Wider roots would be a deliberate later
  setting.
- Model: 1 to 100 characters from the set above. Prompt: 2,000 bytes. Typed folder: 4,096 bytes.
- The opening prompt can't start with `-`. For Claude Code, Codex and Cline it must be at least two words.
- No agent status (working, waiting or idle), no resuming an earlier conversation and no Windows
  machines. Several folders start together only from Home's Select (2 to 12 folder projects).
- YOLO is off unless an owner or admin allows it for the account, the machine accepts it and the
  session is confirmed. A machine can refuse it locally with `reet terminal yolo off`.
- YOLO is recorded per launch; a structured session records each permission reet answers.

## What this does not establish

A started session means only that the machine's connector started the program in tmux in that
folder. It does not show that the agent is signed in, has network access, understands the prompt or
is making progress, and it does not show that a model name exists. Nothing on screen calls the
session more than started. After the start, anyone in the session can type anything into it.

## Checks

`web/src/lib/agentLaunch.test.mjs` (in `npm test`) covers the rules and sentences.
`web/tests/start-agent.e2e.mjs` checks the dialog in Chromium against synthetic API responses:

- both entry points
- every disabled state and every refusal
- the lost-reply retry with the same request id and body
- that nothing is written to storage
- a layout, focus and motion matrix

`web/src/lib/launchBar.test.mjs` covers the launch bar's rules, and `web/tests/launch.e2e.mjs` the bar
on Home: the sentence and each slot, the rail card and New session here, the base sent only when
chosen, Select and the status sheet with Retry, lost replies and refusals, the keys on touch and with
a keyboard, the phone and tablet sheets, and that nothing reaches browser storage.

The dialog's matrix results and their limits are in
[UI verification](ui-verification.md#start-an-agent-session-layout-focus-and-motion-matrix--25-october-2026).
