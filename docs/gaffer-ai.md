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
Chat starts with typed messages only. You can separately preview and enable
retained reet or selected-session context as described below. Worker instructions
and offline follow-up are still being implemented. Saving a provider or
opening the panel does not start a model request or send any reet context.

## Talk to Gaffer

Choose **Ask Gaffer** at the edge of your workspace. On a desktop the panel leaves
the workspace usable; on a phone it fills the screen and stays above the keyboard.
The portrait follows your Gaffer animation and reduced-motion preferences.

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
and retained-context receipts. **Context** stays beside the model control, with
a visible off/on/review state.

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
There are no worker messages, tool execution or background follow-up in this slice.

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
