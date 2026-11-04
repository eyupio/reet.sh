---
title: "What is a self-hosted AI agent control room? · reet.sh"
description: "Understand the difference between an AI coding agent, a model chat interface and a self-hosted control room for machines, sessions and skills."
layout: article
topic: SELF-HOSTING
category: selfhost
art: selfhost
art_icon: server
art_label: ON YOUR TERMS
card_title: "A control room for the agents you already use."
hero_title: "Your agents have tools."
hero_accent: "Give the work a home."
related:
  - quick-start.md
  - self-hosted-ai-agents-checklist.md
---

# What is a self-hosted AI agent control room?

An AI coding agent works inside an environment: a repository, a shell, installed tools, credentials and project instructions. As the work spreads across a laptop, a VPS and a lab machine, finding the right environment can become a task of its own.

A control room provides one place to reach those environments and return to their sessions. Reet supplies that layer while the installed coding agent continues to do the coding.

## Three layers that are easy to mix up

| Layer | Main job | In a Reet setup |
| --- | --- | --- |
| Model and provider | Supply inference and provider access | Chosen and configured for your agent. Reet does not include model credits. |
| Coding agent | Read the project, propose changes and run its tools | Claude Code, Codex, OpenCode or Gemini CLI on the machine where the work runs. |
| Control room | Connect machines, start sessions and organise reusable skills | Your self-hosted Reet controller and its machine connectors. |

A browser chat interface is another useful layer. If your primary need is conversations with models and shared knowledge, read the [Open WebUI](compare-open-webui.md) or [LibreChat](compare-librechat.md) comparison before deciding that you need a machine control room.

## What Reet brings together

**Machines.** An enrolled machine connects outbound to the controller and reports what it can see. The UI distinguishes a ready machine from one that is reconnecting, needs setup or only has last-known information.

**Sessions.** Start an installed agent in a chosen folder, reach it through a browser terminal, and return to running work from the Sessions view. The session board separates saved backlog prompts, active sessions and finished sessions; finished does not mean successful.

**Projects.** Keep a machine-and-folder combination with useful launch defaults. Use supported worktree launches when concurrent agents need separate checkouts.

**Skills.** Keep a private library of reusable instructions, choose destinations and inspect an exact plan before approving a sync. A bookmark, a saved desired selection and an installation are different steps.

**Setups and operations.** Discover existing AI configurations, manage the instance’s access, and plan backups. The details belong in the relevant documentation rather than in a single launch command.

## Where the work runs

The coding agent runs on the machine you chose, under that machine’s OS account. Reet does not move your entire development environment into the browser. The browser is how you reach the session and its workspace.

The controller holds its own database, account information and machine/session records. Host it on infrastructure you control, and treat it as part of the environment you need to maintain and recover.

One Go binary and SQLite keep the deployment small. They do not eliminate the need for HTTPS, updates, access management or backups.

## How machines connect

Each connector dials out to the controller. A machine does not need an inbound port for Reet’s protocol, which can be useful for a laptop or home lab behind a router.

This is not a general-purpose network tunnel. You may still use a VPN for SSH, private databases or other services. The [remote agents guide](remote-agents-without-a-vpn.md) explains that boundary and the connection requirements.

## How skill distribution fits

A public directory helps you discover skills. A private control room helps you decide which skills belong on which machines and see the plan before applying it.

Reet can accept a skills.sh reference into the library, then use a reviewed sync flow for enrolled destinations. It does not need to replace the directory or turn private machine inventories into public pages. Read [skills.sh and Reet](compare-skills-sh.md) for a task-based comparison.

## When this is a good fit

Start with Reet if you already use coding agents on machines you control and want a common place for sessions, inventory and reviewed skill distribution. A second machine, recurring remote work or a skill library you maintain are concrete reasons to try it.

If all your work is in one local terminal, that terminal may already cover the immediate need. If you only need a model chat UI or a public skill catalogue, start with a tool built around that job.

## Try the smallest useful setup

Install the controller, enrol one machine and start one agent in a project you know. Return to that running session, review its changes, then try a skill-sync preview before applying anything.

The [quick start](quick-start.md) takes you through the first connection. The [self-hosting checklist](self-hosted-ai-agents-checklist.md) covers the decisions that make the controller maintainable once you start depending on it.
