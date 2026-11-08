# Start an agent session

Home starts sessions from its launch bar ([below](#on-home-the-launch-bar)). The **Start an agent**
dialog stays for Machines and the Sessions panel: pick a project and a machine, then an agent and, if
you like, a model and an opening prompt. Choose **No project — start in a folder** to pick a folder
instead. Press **Start**
and you land in a new browser terminal session with that agent already running, or in Gaffer when you
choose it. The dialog is
`web/src/lib/StartAgentDialog.svelte`. The launch request it sends and how a machine handles it are
in [Launch requests](terminal-launch.md). The design spec was removed in the docs tidy-up
(see git history before bd9e56c).

The browser never sends a command line. It names a tool id (`claude`, `codex`, `opencode`,
`gemini`, `cline` or `antigravity`) and the values you chose. The machine looks the tool up in a table built into its own
connector and decides the program and its flags itself. Approval-bypass flags are sent only
when you choose YOLO and the account and machine allow it.

**Google Antigravity (AGY)** uses the official `agy` CLI on the machine's PATH; the
Antigravity desktop app alone does not count as installed. It supports an optional model,
an interactive opening prompt (including one word), and low/medium/high thinking. Update
the machine's reet connector if AGY is installed but not offered. Its model defaults and
sign-in remain AGY's own; reet does not install it or sign in for you.

Paste an image into the **Opening prompt**, or use **Attach images**, and each file is uploaded to the
machine before the session starts; its absolute path joins the opening prompt, so the agent starts
with it in view. At most 20 images and 50 MiB at once. A machine with uploads off, or an older reet,
says so and Start waits.

## Gaffer or the agent TUI

Google Antigravity (AGY) offers **Gaffer** on an updated machine that reports its
native ACP integration. Choose **Google Antigravity (AGY) TUI** for its terminal
instead. Gaffer uses AGY's configured permission rules, displays its approval
requests, and supports model/effort choices, images and continuing closed chats
with history. Workspace hooks and customizations may run, so start only in a
folder you trust. Sign in with `agy` on that machine first. See
[AGY in Gaffer](https://github.com/eyupio/reet.sh/blob/main/docs/structured-sessions.md#google-antigravity-agy-in-gaffer).

For OpenCode (1.19.5 or newer) or Cline (3.0.74 or newer) on a machine that advertises `acp`, the dialog's **Session**
choice is **Gaffer** by default; choose the agent's own TUI for the terminal interface instead. A chat sends
`structured: true`, and OpenCode also sends the person's trust confirmation. The **Model** control chooses the model;
**Agent default** uses the agent's configured model, and Thinking stays the agent's own. The opening prompt is the first
message. The new session opens on its Gaffer, where the agent asks before it edits a file or runs a command with
**Allow once** / **Deny**. On a machine that negotiated `session-mode-v1`, OpenCode's session header **OpenCode TUI**
button switches the running session to OpenCode's own terminal UI and **Gaffer** switches it back, once the agent is idle;
a session started in the OpenCode TUI cannot move to Gaffer, and a Cline chat stays in Gaffer. Terminal scrollback is not
kept across a switch, and returning to Gaffer asks you to trust a folder with OpenCode plugins. **Attach image** (or
paste/drop) sends a file through the launch-upload path and puts its path in the message. A Gaffer chat that was lost or
crashed reconnects in place (same session, same transcript) on a machine with `chat-reattach-v1`, for OpenCode and, on a
connection that negotiated `acp-agent-reattach-v1`, for Cline; a chat you
ended, or one on an older machine, offers **Continue in Gaffer with history…**, which loads the agent's stored
conversation into a new session row. The conditions, deadlines and limits are in
[structured sessions](https://github.com/eyupio/reet.sh/blob/main/docs/structured-sessions.md).

For a plain shell, **New session**, the workspace’s **✦ Start a new Terminal session**, and the
sidebar machine menu’s **Terminal** open the terminal settings dialog. Choose its machine, enter an
absolute folder path (or leave it empty for that machine’s home folder), and, if you like, name the
session before pressing **New terminal**. Left empty, the name is Terminal, followed by the folder’s last
part when you chose one (Terminal · api); the field shows it as you type the folder. Changing draft settings or closing the dialog leaves the current session
in place. An unconfirmed creation preserves its original machine, folder, name and request ID for retry.

## Who sees it

Only an account in accounts mode with a write role sees the entry points, just as for browser
shells. Viewers, API and MCP tokens and legacy sign-in modes don't see them. The server refuses them
too. The usual terminal rules still apply: the rollout flag, the canonical origin, the machine's
local consent, and the session limits (8 per machine, 32 per account).

## Where to start it

- **Sessions → New agent session.** It appears under **Your sessions** in the sessions sidebar,
  in a machine's menu next to **New terminal**, and as **✦ Start a new agent session** in the empty
  or ended workspace beside **✦ Start a new Terminal session**. Each opens its settings dialog.
  The machine you were on is preselected; the project is still a deliberate choice. On success the new
  session opens in the workspace.
- **Machines → Start an agent** on every enrolled machine's card. While the machine isn't ready the
  button is disabled and the reason is written underneath; a machine the terminal list doesn't know
  yet says "This machine hasn't connected for terminals yet." If its readiness couldn't be read, it says
  "Couldn't check this machine just now. Try again in a moment." On success you are taken to
  `/sessions?agent=…&session=…`.

**Close** and Escape both close the dialog without sending anything and return focus to the button
you pressed, except while a request is in flight (see below).

Home starts an agent from the rail card's **+** (and **New session here** in the card's menu): the
same **Start an agent** dialog that Machines, the Sessions panel and a project list open, with the
project, its machine, folder and defaults already chosen. Nothing starts until **Start** is pressed.

Every entry point asks for the same two required choices. A project or machine action fills in what it
already knows: a project action fills the project and its machine, and a machine action fills the
machine. Nothing else is chosen for you, so opening the dialog from elsewhere starts on **Choose a
project** and **Choose a machine**. The **Folder** and **Folder path** fields appear only under **No
project — start in a folder**; a project supplies its own folder.

## On Home: the launch bar

Home's launch bar (`web/src/lib/launch/LaunchBar.svelte`; its rules are in
`web/src/lib/launchBar.ts`) opens in a modal (`web/src/lib/home/LaunchDialog.svelte`) for a start in
several projects and for an address that hands Home projects (`#launch`, `/?project=<id>`,
`/?projects=<id>,<id>`). It is a **Describe the job** box, **Start**, and the launch sentence, whose
bold words are the choices. It no longer sits docked at the foot of the page.

For OpenCode, Cline or AGY on a machine that offers its Gaffer integration, the sentence says **in Gaffer** by default and Start
opens Gaffer instead of the agent terminal UI. OpenCode and AGY ask you to confirm **I trust this folder** first:
OpenCode plugins in `.opencode`, or AGY workspace hooks and customizations, may run. Cline has no folder plugin warning. The **Gaffer**
word switches it to **in the agent TUI**; the worktree choices come back for that terminal. AGY also offers its supported effort choices in Gaffer. The Model word
chooses the chat's model; leave it at **default** to use the agent's configured model. Gaffer runs in the chosen project
folder (not a worktree) and can attach images through Home's existing pasted-image path. It starts in one folder at a time. A session's actions menu has **Open a terminal here…**, which starts a plain shell in
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
- **The rail.** Choosing a rail card narrows the board to that project. Its **+** and **New session
  here** open the Start an agent dialog with that project's machine, folder and defaults already
  chosen; `/?project=<id>` opens the launch modal on it once. **Select projects** in the heading's
  menu turns the cards into checkboxes; **Start in these projects** opens the launch bar's modal on
  two or more of them.
- **Machines on Home.** Each machine card's **+** and its menu's **New session…** ask what to start
  there: one of that machine's folder projects, which opens Start an agent with it chosen, or
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
- **Title.** The session is titled by the work the prompt asks for, shown as Titled "…" before sending (and
  stored as the session title). That is the first line with something to say, without a greeting or
  "please" ("Hi, can you please fix the login redirect loop? It started after the deploy." becomes
  "Fix the login redirect loop"), up to the end of its first sentence and at most 60 characters, cut at a
  word with an ellipsis. The `Use the {name} skill.` line the skills picker adds, code fences and lines of
  punctuation are skipped. Without a prompt, or one with nothing to name it from, it is
  \<Agent\> · \<folder name\>. A title you type is kept as typed. [Session titles](terminal-launch.md#session-titles)
  says how the controller names a session whose request carries none.
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
| **Project** (required) | The folder projects this account can start in. A project on a machine that isn't ready is listed but can't be chosen. **No project — start in a folder** is an explicit choice which reveals the folder fields. |
| **Machine** (required) | Every terminal machine. A machine that isn't ready is listed but can't be chosen. Only a machine an action named is preselected; otherwise nothing is chosen. A machine change leaves a project that belongs to another machine. **Not listed? Add a machine** opens [Add a machine](add-machine-in-context.md). |
| **Agent** | Claude Code, Codex, OpenCode, Gemini CLI and, on a machine with `agents-v2`, Cline and Google Antigravity (AGY). Only an agent the machine reports as installed can be chosen. Any other is listed but disabled, with the reason "Not installed on <machine>." |
| **Model** (optional) | The shared picker: **Agent default**, the account's favourites for this agent, the machine's list by provider, reet's suggestions and **Other…** for any model name made of letters, digits and `. _ : / @ + -`, up to 100 characters. A star adds or removes a favourite. A list is only a hint; if a model name doesn't exist, the agent says so itself. Nothing is sent for Agent default. |
| **Thinking** | Only for an agent that takes an effort level (Cline and Google Antigravity): **Agent default**, low, medium or high. Nothing is sent for Agent default. |
| **Source** | Only for a Cline Gaffer chat on a machine that negotiated `acp-provider-v1`: **Cline's own choice**, **Usage-Billing** or **ClinePass**. It opens on your saved default source; a saved source this machine does not offer falls back to Cline's own choice with a note and never blocks Start. The star saves or clears the default. Nothing is sent for Cline's own choice. If Cline cannot use the chosen source the chat is not started and the dialog says so. The default model is offered only while the chosen source is your default source. |
| **Folder** (only under **No project**) | **Home folder**, the project folders that machine has reported, and **Type a path…**. A typed path must be absolute. The machine decides whether it may start there: the folder must exist and be inside the home folder it was enrolled with. |
| **Opening prompt** (optional) | Up to 8,000 bytes, with a byte counter. It can't start with a dash. For Claude Code, Codex and Cline it must be at least two words, because a single word could be read as one of their subcommands. "Secrets don't belong here." |

Opening the dialog applies a model marked as the default for the chosen agent first; otherwise the
project's default model applies (when the dialog has a project), then the account's. A project also
supplies the default agent and thinking. Changing the machine or the agent resets the model and
folder to their defaults. Changing the project to one on another machine moves the machine with it;
changing the machine leaves a project that belongs to another machine unset, so it is chosen again.
The model list's status line and Refresh are honest about a cache read; a failed read never disables
Start.

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
twice. If you close the dialog instead, the request is forgotten with it: the dialog does not keep the prompt
(a session that did start keeps its own copy; see Safety and privacy). A new dialog sends a new request, so check your sessions first. If the first
attempt did start, it appears in your sessions list on the next refresh.

## Safety and privacy

- **Command line.** The page never sends a program name or a command line. The model and the prompt
  each reach the agent as a single argument, never through a shell.
- **Where the prompt is kept.** It is stored with the session and with the create request in the
  controller's database, capped at 8,000 bytes, and in the machine's private registry file, which only
  its owner can read. Only the session's owner can read it back: the sessions list carries a
  300-character excerpt and the full text comes from a request of its own. Archiving the session clears
  it, including the copy kept with the create request. It is never written to the audit trail, to logs,
  to browser storage or to the address bar.
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
- Source: a provider id Cline reports, from the same character set as a model name. It is sent as `provider` only for a structured Cline create, only to a machine that negotiated `acp-provider-v1`, and never stored with the session. Not yet checked against a signed-in Cline (see the Cline probe notes).
- Model: 1 to 100 characters from the set above. Prompt: 8,000 bytes. Typed folder: 4,096 bytes.
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
[UI verification](https://github.com/eyupio/reet.sh/blob/main/docs/ui-verification.md#start-an-agent-session-layout-focus-and-motion-matrix--25-october-2026).
