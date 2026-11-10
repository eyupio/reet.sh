---
title: "Set up Gaffer AI providers · reet.sh"
description: "Choose Gaffer's provider and model, fetch live model choices, test the connection and control which reet context a provider may receive."
layout: article
topic: GAFFER AI
category: selfhost
art: selfhost
art_icon: server
art_label: CHOICE OF BRAINS
card_title: "Choose who Gaffer talks to."
hero_title: "Pick the brains."
hero_accent: "Gaffer will put the kettle on."
related:
  - agent-models.md
  - backups.md
---

# Set up Gaffer AI providers

Open **Account → Gaffer AI** to configure Gaffer's own provider and model. They
belong to your account and are separate from the agent working in a session.
Use **Ask Gaffer** for a private companion conversation after choosing a provider.
Chat starts with typed messages only. You can preview and enable retained reet
context separately, or review a session's access and bounded goal together through
**Look after this session**. Saving a provider or opening the panel starts no
model request and sends no reet context.

## Talk to Gaffer

Use the circular **Ask Gaffer** mascot in the saved bottom corner on every page,
including session workspaces. Page actions and the session viewport leave room
for it. **Panel layout** offers Compact, Default and Expanded sizes, Full screen,
and left/right placement. The preset and corner are saved in this browser for
your account. Drag the desktop header or focus its mascot and use arrow keys
(Shift moves farther); the resize grip changes the panel temporarily. Choosing a
preset or reopening restores its saved corner. Full screen and the phone sheet
stay above the keyboard. The mascot follows your animation and reduced-motion
preferences. While typing into a worker with the phone keyboard open, the
launcher and its reserved space step aside in keyboard-sized views; closing the keyboard restores the
same corner. Wider workspaces, including landscape phones, reserve a narrow side strip beside the terminal
instead of consuming terminal rows.

Outside a session, use **General Gaffer chat**. Inside a supported structured chat,
**This session** has its own conversation and draft. **Conversation history** in
the header switches between general chat and this session, or opens a saved
companion in its session workspace. The current conversation is named in the
compact header. **Chat options** shows the working agent/model and Gaffer's
independent provider/model. A plain terminal explains why its session companion is unavailable and offers general chat.
Changing scope does not grant access to the working agent's history. Each
conversation has its own sharing choices, off until you enable them.

Write a message and choose **Send message**, or press Ctrl/Command+Enter. Replies
stream from the controller and are saved there. **Close Gaffer** keeps the current
answer running. Reopen to find it again; **Cancel answer** requests cancellation
of Gaffer's answer and leaves the working agent alone. The final saved outcome
shows completion, cancellation, interruption or failure. Provider usage is shown
when reported, otherwise marked unknown. No automatic follow-up starts here.

Use the compact **Model** control below the message box to choose Gaffer's
provider and optional model override. The picker opens separately, leaving the
conversation area at its full size. Cancel discards unsaved changes. Fetch the
provider's model list or enter an exact name. **Apply choice** saves it before your next message; that message shares this Gaffer
conversation's completed history with the chosen provider. Working-agent defaults
stay separate. Provider changes during an answer wait until it finishes or you
cancel it. Connection tests and model protocol limits below still apply. Replies
use readable paragraphs, lists and code; **Reply details** expands provider, usage
and retained-context receipts. In general chat, **Context** stays beside the model control. In a session,
**Look after this session** takes that place; detailed sharing choices remain in
**Chat options → Advanced session controls → Context sharing**.

If delivery is unconfirmed, choose **Check delivery**. Gaffer reads its saved
receipt rather than sending the message again. **Send same message safely**, when
available, uses the original receipt; do not retype the question as a new send
before checking. A paused reply connection can be reconnected without restarting
the model. After a controller restart, an unfinished answer stays interrupted
with its saved partial text and is never replayed automatically.

Drafts stay in memory while the page is open, separately for each scope; a reload
loses unsent drafts. Saved messages remain on the controller. Only receipt IDs,
not drafts or message text, enter browser session storage. **Chat options → Remove chat**
deletes its messages and stops its current answer; it leaves the working agent
running and cannot recover provider credits already used. Limits are 100 stored
conversations per account, 100 turns per conversation, 8,000 bytes per question
and 64 KiB per reply. The panel shows when a reply was cut short.

**Conversation history** in the header lists saved conversations with links to
available session workspaces. Unavailable sessions stay listed for removal,
including companions whose worker session has been archived. Archiving worker history does not silently delete
your Gaffer messages; the saved-chat cleanup remains available.

## Ask the working model to act

In a structured session, open **Chat options → Advanced session controls → Worker messages**. Allow Gaffer to
queue messages for this session, then **Save worker permission**. This is separate
from Context sharing and requires a provider allowing session data. Changing the
provider, model or provider policy requires reviewing the permission again.

Choose **Enable one worker message for next request**, then write what you want:
“Ask the working model to raise a PR, fix its checks and leave merging to me.” The
composer shows that this request can send one instruction; **Cancel** removes that
choice. The following ordinary message returns to read-only chat. Gaffer can
relay one instruction, then report its receipt; the existing session model does
the implementation, commands and PR work under its own permissions. Choosing a
permission or closing the widget does not create a background follow goal.

Expand **Worker instruction** in the reply or reopen **Worker messages** to read
the saved instruction and delivery state. **Queued** waits for the next worker
turn. **Accepted by worker** means the channel accepted it, not that the work is
finished. A blocked receipt means it was not sent, for example because the worker
needs a human approval or its channel is unavailable. Check the working conversation
before deciding what to do with an uncertain receipt; Gaffer never retries it
automatically. A failed Gaffer answer can still have a committed instruction, so
read its receipt before asking again.

For a queued instruction, **Stop worker and send** offers a separate confirmation.
It uses the worker's existing stop-and-send action. Only you can request that
interrupt in the browser; the model cannot. Worker approval cards still need your
decision. The session's ordinary queue can remove a managed instruction, but cannot
edit or retry it. Stop worker permission to remove instructions still queued;
already claimed or accepted work cannot be recalled. Cancelling Gaffer's answer
is separate from cancelling a committed worker instruction.

Messaging checks current ownership, account role, session state, controller worker
access and provider policy before delivery. Archived sessions and revoked
permissions block subsequent delivery. Instructions are limited to 8,000 UTF-8
bytes, with 500 retained receipts per account. MCP clients may have 100 enabled
session scopes each. Use a supported structured chat with a verified message channel.

## Let Gaffer look after a session

Choose **Look after this session** beside the composer, or in **Chat options**,
in an eligible structured session. One review shows Gaffer's selected provider
and model, the recent session conversation it may read (including code or logs
in that conversation), worker messaging, the agreed goal and its limits.
**Start looking after** saves those choices together. If needed, it enables the
provider's session/text permissions; other sessions still need their own
delegation. Closing this review makes no change. Detailed sharing, worker and
provider permissions remain optional advanced controls.

Write the agreed goal, or choose **Use a prompt** for a reet-written starting
point. **Save as prompt** keeps the current goal in this browser, alongside the
built-in prompts, for reuse in another session. Review the visible default of **12 Gaffer reviews over 24 hours**, or expand
**Change limits**, then choose **Start looking after**. You may choose 1–48 reviews over 1–168 hours, with at most ten
active session goals per account. Each review uses at most one worker instruction
and two Gaffer model requests. This budget covers Gaffer's reviews; the working
model keeps its existing usage and permission controls.

### Start from the session overview

In the session's **Overview**, each stage under **Progress** (Code, PR, Checks,
Review and Merge) opens a short menu when you click it, right-click it or press
Enter, Space or the Menu key on it. Each item is a ready-written goal, for
example to babysit the pull request (fix failing checks, address review comments
and keep conflicts resolved), to have Gaffer review it and send the working agent
its feedback as instructions, or to merge it when green.

Picking an item opens **Look after this session** with that goal already in the
goal field. Nothing starts: review or change the wording, the limits and the
permissions, then choose **Start looking after**. Closing it makes no change. The
working agent still does the work, since Gaffer makes no GitHub request of its
own. The merge items say exactly when to merge (every check passing, no open
review thread, no conflicts) and still bring decisions back to you; the others
say not to merge. A terminal session has no follow-up, so there the menu copies
the same words for you to give the agent.

Closing chat or the browser leaves an explicitly active goal running on the
controller, in both native and Docker deployments. Each selected session has its
own goal, conversation and budget. Gaffer waits for new retained worker progress
and an idle verified channel. Quiet, busy and unavailable sessions make no model
requests. The first review can use the session's existing retained progress.
Gaffer asks the working model to handle implementation, PR actions and any future
check monitoring; Gaffer itself makes no direct GitHub or shell request.

A review chooses one next step or brings a question back to you. A prose-only
review needs your attention too; it does not silently promise further progress.
Two identical instructions are allowed per goal; a third stops for your review.
Human input/approvals, a stopped or failed worker, interrupted Gaffer reviews,
ambiguous or blocked delivery, policy changes and exhausted limits also need you.
A quiet session or a model saying “ready” does not prove that the goal is complete.
Only **Mark goal complete** records your decision. It does not merge a PR.

The compact goal entry opens the saved goal, latest Gaffer review, observation
sequence/time, actual review finish time and reported token usage. Missing usage
is shown as unknown. Observation time is separate from a newer controller status
update. **Edit goal or start a new budget** opens the editor without crowding the
normal conversation. Automatic turns are labelled **Goal review**.

**Pause goal** or **Stop goal** removes that goal's instructions still queued,
cancels its current Gaffer review and leaves manual messages and the working model
alone. Already claimed or accepted work cannot be recalled. Cancelling one Gaffer
answer leaves its saved outcome and requires review before automatic follow-up
resumes. **Resume after review** asks you to check decisions and uncertain delivery;
it keeps the remaining budget and expiry and waits for fresh worker progress.
Starting a new goal/budget is a separate confirmed action.

An ordinary controller restart resumes unchanged active goals after reconciling
saved turns and receipts. In-flight interrupted reviews and uncertain deliveries
are never replayed automatically. Restoring an instance copy pauses its goals for
explicit review, because the worker may have progressed beyond the saved copy.
Changes to provider/model, account access, sharing or worker permission stop stale
follow-up. Resume reviews the current provider/model and session access together, then
preserves the existing budget and deadline.

This slice follows controller-retained worker events, with the same context bounds
and omissions described below. It does not poll unchanged evidence or observe
external PR/check changes on its own: the working model must monitor those and
report progress. Unsupported channels and providers remain visible limitations.

## The Factory

**Factory** is its own page, in the main navigation beside Home. It is where you
hand Gaffer an outcome and watch it run. Home has a **Factory** button, and a line
with Gaffer's face whenever something there is waiting; an empty Gaffer chat
offers **Give Gaffer an outcome** and **Open the Factory**. Opening the page starts
nothing and makes no model request.

The top of the page shows four numbers: pipelines that **need you**, are **ready to
merge**, are **running** and are **done**. Below, each outcome lists its pipelines.
A pipeline is one piece of the work, drawn as a production line:

- **Stages.** Plan, design, code, review, test, pull request, babysit and merge, or
  the ones you chose for it. Done stages are filled, the one in hand is ringed, the
  rest are to come.
- **What is happening.** One line from the controller, Gaffer's latest report, and a
  highlighted line when Gaffer has brought something back to you.
- **The numbers.** Which agent and model is doing this stage, reviews left of the
  stage's budget, time left, the last review, what Gaffer's own model has used on
  the pipeline so far, and how the merge is decided.
- **Actions.** Open session, Review goal, **Approve merge** when it is ready, and
  **Try again** for a pipeline that did not start or stopped part-way.

Each outcome also shows its own figures: pipelines, Gaffer's reviews and tokens
across all of them, and the budget each stage gets.

### Tokens and budgets

The Factory counts what **Gaffer's own model** used: every finished stage keeps
its reviews and tokens, the stage in hand reads its live goal, and a report you
ask for is added to the outcome's total. Where a provider reports no token count
the page says so instead of showing a number. reet does not see what the working
agents use, and it attaches no price to any of it.

Budgets are per stage: each stage gets the outcome's review budget and hours
afresh, and the page shows what is left of the stage in hand. A stage that uses
its budget stops and waits for you.

### Ask Gaffer about the whole outcome

**Ask Gaffer for a report** on an outcome is one model request to that outcome's
own provider. It is sent the outcome, each pipeline's state, the handovers and
Gaffer's latest saved reviews, nothing newer, and only while that provider is
still allowed session text. The report says what is done, in hand, stuck or
waiting on you, and whether the pipelines together will meet the outcome.

Gaffer may also **propose pipelines** to add: work nothing covers, or a
replacement for one that stopped. Each shows why and the stages it would run.
Tick the ones you want and choose **Add pipelines**; they use the outcome's agent
and never merge by themselves. Nothing is added or started unless you approve it,
and an outcome holds at most ten pipelines.

A new installation sees **Set up your factory** instead: connect a machine, add a
project, choose Gaffer's model. Each step links to where it is done and ticks off.

### Give Gaffer an outcome

1. Write the outcome and choose the project. Only agents whose chat is available on
   that project's machine are offered, and the machine must be connected.
2. Choose the **crew** under **Choose who does what**. Thinking covers plan, design
   and review; Building covers code and test; Shipping covers the pull request,
   babysitting and merge. Give each its agent and, if you like, a model: a stronger
   one for thinking, a cheaper one for building or shipping. Left alone, one agent
   does every stage.
3. Choose **Ask Gaffer for a plan**. Gaffer's default provider is sent what you
   typed and the project's name, nothing else, and proposes the pipelines: a title,
   a brief and the stages each needs. It cannot choose an agent, a model or whether
   anything merges. **Write the plan myself** skips this.
4. Review the plan. Edit any title or brief, add or remove a pipeline, and tick the
   stages each one runs through. The plan says how many agent sessions a pipeline
   will use: a new session starts whenever the agent or model changes from one
   stage to the next, and otherwise the same session carries on.
5. Decide the merge. **Merge by itself** lets each pipeline's agent merge once every
   check passes, no review thread is open and there are no conflicts. Left off, a
   pipeline stops at **Ready to merge** and waits for **Approve merge** on the
   Factory page. The tick starts from your default in Gaffer settings.
6. For OpenCode and Google Antigravity, tick **I trust this folder** if the folder's
   plugins may run.
7. Choose **Approve and start**. The controller starts each pipeline's first stage as
   an ordinary chat in the project, never in YOLO, in its own worktree in a git
   project.

How a pipeline moves on: Gaffer follows the stage in hand like any session it looks
after. When the working agent's own reports show the stage's deliverable is done,
Gaffer hands it over with a short summary. The controller then starts the next
stage you approved, in the first stage's working folder, with that handover. Gaffer
does not choose the next stage, its agent or its model. Every stage but merge is
told not to merge.

The controller does the starting and the moving on, so you can close the browser. A
restart carries on without starting a session twice. **Stop starting sessions**
leaves what has not started unstarted. Restoring a controller from a copy stops any
outcome that was still starting. Nothing is retried on its own: a stage that could
not start says why, and its session is left for you to look after.

### Settings for the whole account

**Account → Gaffer AI → How Gaffer works** has two choices:

- **Look after sessions without asking each time** (on by default). When you tell
  Gaffer to look after sessions, from the Factory or with **Look after** beside a
  session's own Gaffer chat box, it starts at once with your usual goal and limits.
  **Chat options → Look after this session** still opens the review first. Nothing is read or sent until you ask. **Review first** still
  shows the goal and limits beforehand, and any one session can be reviewed, paused
  or stopped. Turned off, you are shown the review every time.
- **Let new pipelines merge by themselves** (off by default). The starting value of
  the merge tick on each new outcome; you can change it per outcome.

### Other sessions

Under the pipelines the Factory lists the sessions you started yourself: the ones
Gaffer looks after, and the running chats it does not yet. With two or more
followed sessions in view, **Pause all following** and **Stop all goals** act on
every one shown, one goal at a time; stopping asks first, and the working models
keep running either way. Resuming is done one session at a time.

From a session's own Gaffer chat, **Chat options → Factory floor** shows the same
list in a dialog, so you can look without leaving the session.

### Put several sessions in Gaffer's care

Tick the running chats Gaffer should look after, or **Select all**, then choose
**Look after selected**. With looking-after pre-approved (the default) that starts
them at once with your usual goal and limits; **Review first** opens the review
described below instead. Ticking shares nothing and starts nothing. **Open**
beside a chat shows that session with its own Gaffer conversation instead, which
also enables nothing. One review
lists the chosen sessions, Gaffer's default provider and model, what starting
shares, one goal and one set of limits. Write the goal or pick one with **Use a
prompt**; expand **Change limits** for the review budget and hours.

**Start looking after** then sets each session up in turn through the same reviewed
setup a single session uses. Each session gets its own Gaffer conversation,
access, goal and budget, so pausing or changing one leaves the others alone. A
session that already has a Gaffer chat keeps the provider and model chosen there.
The results name every session as **Following** or **Not started** with the
reason; a session that could not start is left unchanged and stays ticked, so you
can try it again or open it. The goal and limits you used are kept in this browser
as the starting point for next time.

Only running structured chats without a saved goal are offered. A plain terminal
has no follow-up, and a session that already has a goal, finished or not, is
managed from its card. Gaffer follows at most ten sessions at once: the floor
says how much room is left before the review opens. The working model in each
session still does the coding and PR work under its own approvals; Gaffer gains
no shell, GitHub or merge path here.

### Review followed sessions

The floor reads top to bottom: **Give Gaffer an outcome**, then **Outcomes**,
then **Sessions** with its filters and the actions for the sessions shown, then
**Not looked after**. Each card is short on purpose. It shows the session, its state,
the outcome it serves (or the start of its own goal), anything waiting on you in
a highlighted line, the latest Gaffer report, and its review budget and deadline.
Expand **Review & evidence** for the full agreed goal, the full saved report, when
worker evidence was observed and the review finished, reported usage and the last
goal instruction. Those times stay separate from when the floor was read. **Handed to worker** confirms
handoff, not completion. Uncertain or blocked delivery needs your review; a model's
PR/check report still needs verification in the working conversation.

**Open session** returns to that workspace and its companion, preserving drafts
when switching within the current workspace. **Review goal** opens the existing
controls with fresh permission checks and the same resume/completion confirmations.
Archived or unavailable sessions retain your saved goal for review; use
**Conversation history** to remove their saved chats. Their unavailable worker
channels cannot be resumed from the floor.

The view refreshes retained records while open. A failed refresh keeps the last
successful read visibly labelled; source times are never replaced by refresh time.
If the running sessions cannot be read, the followed work stays and the floor
says what is missing.

## Add a provider

1. Choose **Add provider**, then a preset: OpenAI, Anthropic, Ollama Cloud,
   OpenCode Zen, OpenCode Go, Local Ollama or another compatible server.
2. Give it a name, check the API address and enter its key if required.
3. Choose **Fetch models**. Search the live results and select a model, or choose
   **Enter a model name…** when discovery is unavailable or the list is empty.
4. Choose the context permissions you want this provider to have. All three
   start off. A future conversation will still require access to the selected
   session; these settings alone do not grant session access.
5. Save, then optionally choose **Test connection**. Testing sends a short request
   to that model, can use provider credits, and includes no reet/session context.

A model appearing in a provider's list does not prove that it supports the chosen
API protocol, reasoning setting or tools. Test the selection. OpenCode and other
compatible presets use the OpenAI **chat-completions** protocol; models requiring
Responses or Messages need a backend supporting that protocol. Anthropic uses
its Messages API. Subscription CLI backends are not available in this slice.
An OpenAI API key uses API billing, separately from a ChatGPT subscription.

## Native service and Docker

Hosted API backends work from either deployment when the controller can reach
their HTTPS endpoints. **Local network only** is available to owners and
administrators, for loopback, private-network and Tailnet destinations. Local
requests go directly to that network; hosted requests can use the controller's
configured HTTPS proxy and trust store. Redirects are refused.

**Localhost means the controller.** With a native service, it means the service's
host. With Docker, it means the controller container. For a model running outside
that container, enter a private address reachable from it. Reet does not install
Ollama or expose a model server automatically. Link-local/cloud-metadata endpoints
and mixed public/private DNS answers are refused.

Settings require a signed-in browser account with write access and a configured
`REET_PUBLIC_URL` matching the browser origin. Viewers, API/MCP credentials and
`REET_AUTH=none` cannot configure or test providers.

## Defaults, edits and keys

You can keep up to twelve named providers. Choose **Make default** on an enabled
provider; Gaffer's default does not alter any working agent's defaults. Pausing
retains the explicit default, and deleting it leaves no default until you select
one. Reet never silently switches providers.

Keys are encrypted in the controller database under its existing instance key.
Responses and audit entries do not include keys or ciphertext. Editing leaves
the password field blank: leave it blank to keep the saved key at the same API
address. Changing the address or backend clears the old key unless you provide a
replacement. Model discovery will ask for a replacement or explicit key removal
before using a changed address. **Remove** deletes the provider and its saved key.

A stale form is rejected if another tab changed the provider. Model discovery
can be cancelled by closing the form; changing its address or key discards older
results. **Cancel test** stops the waiting request, although the provider may
already have billed it.

## Choose what Gaffer can read

Provider permissions set the maximum data that provider may receive; they do not
start sharing. In a conversation, choose **Context**, select the categories and
**Preview shared data**, then **Enable selected sharing**. The dialog names the
destination provider/model and shows the actual bounded data shape. Opening it or
previewing sends no model request. A later read may contain newer permitted events.

- **Last-reported status:** general chat gets counts of your machines and
  unarchived sessions. A session companion gets only that session's stored state,
  worker type and update time. Requires the provider's status permission.
- **Conversation event summaries:** recent roles, timestamps and tool kinds/states
  for that selected session, without message text. Requires session permission.
- **Message text and code:** bounded user/agent text, session title and opening-goal
  excerpt. Requires both session and code/logs permissions, plus event summaries.
  Text can contain code or secrets; automatic masking recognises some credentials
  and cannot guarantee their removal. Check what you are comfortable sharing.

The controller reads a fresh snapshot when you explicitly send a message. It
sends at most 16 KiB of context and up to 20 projected events from the last 40
retained records. The snapshot carries read/update times, retention bounds and
omission markers. The answer shows when retained context was read. This is not
live machine health, current PR checks or a full transcript: reasoning, approval
prompts, raw tool arguments/output, files and diffs are excluded.

Sharing continues for future messages until **Stop sharing**. Provider/model or
data-policy changes require a fresh review. Changing sharing stops the current
Gaffer answer and invalidates stale sends; the working agent keeps running.
Already shared data cannot be recalled. Completed Gaffer replies derived from
categories you revoke are excluded from later model history; they remain visible
in your saved conversation. Archiving a worker session revokes its context grants.
Context sharing alone grants no worker messaging. Enable an instruction separately
as described above, or activate a separately bounded session goal.

[MCP context access](mcp.md#share-retained-reet-context) is granted separately per
connection or browser tools. Giving Gaffer context does not grant it to MCP clients.

## Recovery

Instance database backups include provider settings and encrypted keys. Keep the
instance key with your recovery material. If a restored key cannot be decrypted,
reet clears it, pauses the provider and asks you to enter it again. Personal shelf
exports do not include Gaffer providers or keys. See [Backups](backups.md).

Connection results record whether a response arrived, latency and whether usage
was reported. They do not certify tool support, the provider's internal model
identity, future availability or the working session's health.
