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
This first slice provides settings and connection tests. The companion chat,
activity summaries and offline session follow-up are still being implemented.
Saving a provider does not start a conversation or send any reet context.

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

## Context and recovery

The three permissions separately allow reet status/activity, followed-session
conversations/tool results, and code/logs from those sessions. Code/logs require
session context to be enabled. They are stored policy for the forthcoming chat;
this settings slice sends none of that data.

Instance database backups include provider settings and encrypted keys. Keep the
instance key with your recovery material. If a restored key cannot be decrypted,
reet clears it, pauses the provider and asks you to enter it again. Personal shelf
exports do not include Gaffer providers or keys. See [Backups](backups.md).

Connection results record whether a response arrived, latency and whether usage
was reported. They do not certify tool support, the provider's internal model
identity, future availability or the working session's health.
