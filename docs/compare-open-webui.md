---
title: 'Open WebUI and Reet: chat platform or agent control room?'
description: Use a chat interface for model conversations and a control room for coding sessions, machines and skills.
card_title: Open WebUI & Reet
---

# Open WebUI and reet.sh: chat platform or agent control room?

Open WebUI describes itself as a **self-hosted AI platform**: run AI on your own terms, connect models, and extend it with code. reet.sh is a different layer. It is the control room for the agents, skills and setups across your machines.

The two can sit side by side. Open WebUI is where a person talks to a model. reet.sh is where an agent runs on a machine, a skill is distributed to a fleet, and a session is picked up later.

## What each is for

**Open WebUI is a chat and model interface.** Its centre is the conversation: models, prompts, knowledge, users and a browser UI. It is a good fit when the main question is "how do I give people a private interface to these models?"

**reet.sh is an operations layer for agents and machines.** Its centre is the machine: what is installed, which session is running where, which skills are on which host, what changed, and whether the last sync plan was approved. It starts the agents already installed on your machines and gives you terminals, session history, reviewed skill distribution and backups.

## Choose by the job

| You want to... | Start with |
| --- | --- |
| Give a team a private chat UI for several models | Open WebUI |
| Manage prompts, knowledge collections and model access for chat users | Open WebUI |
| Run Claude Code, Codex or OpenCode on a remote machine | reet.sh |
| Keep a private skill library and send a reviewed set to several machines | reet.sh |
| See which machine has which skill and whether its scan is stale | reet.sh |
| Back up skills, setups and session records before changing a machine | reet.sh |

## They are complementary

A useful split is:

- Open WebUI for conversations with models and shared knowledge.
- reet.sh for the machines where coding agents run, the skills those agents use, and the reviewable changes between them.

reet.sh does not try to be a general chat front end for every model. Open WebUI does not try to be a fleet controller for terminal agents. Judging either by the other's job produces a misleading comparison.

## Where reet.sh is different

- **Machine-first, not chat-first.** The inventory, sessions, sync plans and backups are the product.
- **Outbound agents.** A connected machine dials the controller; there is no inbound port or VPN mesh to open for reet itself.
- **Review before change.** A sync plan is shown before files are written, and the machine checks the plan again before applying it.
- **Self-hosted and small.** One Go binary, one SQLite file, no telemetry by default. There is no reet-hosted control plane for your fleet.

## When Open WebUI is the better answer

- Your users mainly want to chat with models through a browser.
- You need model access control, prompt libraries and knowledge collections for conversations.
- You do not need terminals, agent sessions or multi-machine skill distribution.

## Try reet.sh

- [Install reet.sh](binary-installation.md)
- [Run Claude Code on a VPS from your browser](run-claude-code-on-a-vps.md)
- [Sync skills between machines](sync-skills-between-machines.md)

If the job is a private chat interface, start with Open WebUI. If the job is "run these agents on these machines, with these skills, and show me the plan first", that is reet.sh.
