# Start an agent session

Pick a machine, an agent, a folder and, if you like, a model and an opening prompt. Press **Start**
and you land in a new browser terminal session with that agent already running. The dialog is
`web/src/lib/StartAgentDialog.svelte`. The launch request it sends and how a machine handles it are
in [Launch requests](terminal-launch.md). The design spec was removed in the docs tidy-up
(see git history before bd9e56c).

The browser never sends a command line. It names a tool id (`claude`, `codex`, `opencode` or
`gemini`) and the values you chose. The machine looks the tool up in a table built into its own
connector and decides the program and its flags itself. No preset uses a flag that skips the
agent's own permission prompts.

## Who sees it

Only an account in accounts mode with a write role sees the entry points, just as for browser
shells. Viewers, API and MCP tokens and legacy sign-in modes don't see them. The server refuses them
too. The usual terminal rules still apply: the rollout flag, the canonical origin, the machine's
local consent, and the session limits (8 per machine, 32 per account).

## Where to start it

- **Terminals → New agent session.** It appears under the machine picker in the sessions sidebar,
  in a machine's menu next to **New terminal**, and in the empty workspace next to **New terminal**.
  The machine you were on is preselected. On success the new session opens in the workspace.
- **Machines → Start an agent** on every enrolled machine's card. While the machine isn't ready the
  button is disabled and the reason is written underneath; a machine the terminal list doesn't know
  yet says "This machine hasn't connected for terminals yet." If its readiness couldn't be read, it says
  "Couldn't check this machine just now. Try again in a moment." On success you are taken to
  `/terminals?agent=…&session=…`.

**Close** and Escape both close the dialog without sending anything and return focus to the button
you pressed, except while a request is in flight (see below).

## The dialog

It is one screen:

| Field | What it offers |
| --- | --- |
| **Machine** | Every terminal machine. A machine that isn't ready is listed but can't be chosen. If the one you came from isn't ready and another is, the other is chosen and the dialog says "\<from\> isn't ready, so \<other\> is selected." **Not listed? Add a machine** opens [Add a machine](add-machine-in-context.md). |
| **Agent** | Claude Code, Codex, OpenCode and Gemini CLI. Only an agent the machine reports as installed can be chosen. Any other is listed but disabled, with the reason "Not installed on this machine." |
| **Model** (optional) | **Agent default**, a few suggestions for the chosen agent, and **Other…** for any model name made of letters, digits and `. _ : / @ + -`, up to 100 characters. Suggestions are only hints. If a model name doesn't exist, the agent says so itself. Nothing is sent for Agent default. |
| **Folder** | **Home folder**, the project folders that machine has reported, and **Type a path…**. A typed path must be absolute. The machine decides whether it may start there: the folder must exist and be inside the home folder it was enrolled with. |
| **Opening prompt** (optional) | Up to 2,000 bytes, with a byte counter. It can't start with a dash. For Claude Code and Codex it must be at least two words, because a single word could be read as one of their subcommands. "Secrets don't belong here." |

Changing the machine or the agent resets the model and folder to their defaults.

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
- **Activity.** A start writes one audit event, which Activity shows as, for example, "Started
  Claude Code with opus in /home/you/work/site on studio". It never includes the prompt.

## Limits

- Agents: `claude`, `codex`, `opencode` and `gemini`. A machine offers only the ones whose program it
  finds on its `PATH`. `pi` and `cursor` aren't offered until their launch syntax is confirmed.
- Folder: inside the enrolled home, symlinks followed. Wider roots would be a deliberate later
  setting.
- Model: 1 to 100 characters from the set above. Prompt: 2,000 bytes. Typed folder: 4,096 bytes.
- The opening prompt can't start with `-`. For Claude Code and Codex it must be at least two words.
- No agent status (working, waiting or idle), no resuming an earlier conversation, no choosing
  several folders at once, and no Windows machines.

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

The matrix results and their limits are in
[UI verification](ui-verification.md#start-an-agent-session-layout-focus-and-motion-matrix--25-october-2026).
