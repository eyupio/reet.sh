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
**Start follow-up** saves those choices together. If needed, it enables the
provider's session/text permissions; other sessions still need their own
delegation. Closing this review makes no change. Detailed sharing, worker and
provider permissions remain optional advanced controls.

Write the agreed goal, or choose **Use a prompt** for a reet-written starting
point. **Save as prompt** keeps the current goal in this browser, alongside the
built-in prompts, for reuse in another session. Review the visible default of **12 Gaffer reviews over 24 hours**, or expand
**Change limits**, then choose **Start follow-up**. You may choose 1–48 reviews over 1–168 hours, with at most ten
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
permissions, then choose **Start follow-up**. Closing it makes no change. The
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

**Pause follow-up** or **Stop goal** removes that goal's instructions still queued,
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

## Run the factory floor

The factory floor is one click away: **Factory floor** under Gaffer's name in
general chat, **Open the factory floor** in an empty general chat, or Home's
**Gaffer:** line, which appears whenever a goal needs review, is being followed,
or running chats are not followed yet. **Chat options → Factory floor** opens it
from a session chat. It is one job sheet for the whole floor: the sessions you
asked Gaffer to look after, with decisions first, and under **Not followed**
every running structured chat you have not. The filters **Needs review**,
**Following**, **Not followed** and **Finished** carry their counts and narrow
the view. Paused goals remain under **All**. Opening or refreshing this view
starts no goal, model request or worker instruction.

### Give Gaffer an outcome

Choose **Give Gaffer an outcome** at the top of the floor when you want a result
and would sooner not start each session yourself.

1. Write the outcome, choose the project its sessions work in, the agent, and how
   many sessions Gaffer may propose (up to five). Only agents whose chat is
   available on that project's machine are offered, and the machine must be
   connected.
2. Choose **Ask Gaffer for a plan**. Gaffer's default provider is sent what you
   typed and the project's name, nothing else, and proposes the sessions: a title
   and a full prompt for each. This is one model request and may use provider
   credits. It starts nothing.
3. Review the plan. Every title and prompt is editable; remove a session or **Add
   a session** of your own. **Write the plan myself** skips Gaffer's proposal,
   for when the model cannot plan or you already know the split.
4. Choose **Approve and start**. Each session is then started in turn as an
   ordinary chat in that project, with the prompt shown and ordinary approvals,
   never YOLO. In a git project each session gets its own worktree, so they do
   not edit the same files. As each chat comes up it is put in Gaffer's care with
   a goal that names the outcome and that session's part, and ends "Do not merge."

The results name every session as **Following**, **Started, not followed** (with
the reason; it then appears under **Not followed**) or **Not started**. Keep the
dialog open until they settle: your browser starts the sessions, so closing it
stops the ones still waiting. Sessions already started keep running.

On the floor, **Outcomes** lists each outcome with its sessions and how many need
review, are being followed or have finished. Choose one to see only its sessions.
Each session keeps its own goal, budget and controls; only you mark a goal
complete, and no outcome is ever marked done for you.

What this does not do yet: Gaffer does not add, replace or retire sessions after
you approve, sessions do not see each other's work, and nothing merges. An agent
that asks to trust a folder's plugins first (OpenCode, Google Antigravity) is
refused here with that reason; start it from the launch bar, then look after it
from the floor.

### Put several sessions in Gaffer's care

Tick the running chats Gaffer should look after, or **Select all**, then choose
**Look after selected**. Ticking shares nothing and starts nothing. **Open**
beside a chat shows that session with its own Gaffer conversation instead, which
also enables nothing. One review
lists the chosen sessions, Gaffer's default provider and model, what starting
shares, one goal and one set of limits. Write the goal or pick one with **Use a
prompt**; expand **Change limits** for the review budget and hours.

**Start follow-up** then sets each session up in turn through the same reviewed
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

Each card shows the agreed work, latest Gaffer report, review budget and deadline.
Worker evidence observation time and the actual review finish time stay separate
from the overview read time. Expand **Review & evidence** for the full saved
report, reported usage and last goal instruction. **Handed to worker** confirms
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
