---
title: 'GOAT and Reet: an agent cockpit or a fleet control room?'
description: Steer coding agents from a desktop, browser or phone, or keep the machines, skills and setups around them in good order. See where GOAT and Reet differ.
card_title: GOAT & Reet
---

# GOAT and reet.sh: agent cockpit or fleet control room?

GOAT describes itself as a way to run the coding agents you already use on the machines you own and steer them from one place: a desktop app, a browser or a phone. reet.sh is a self-hosted control room for the machines, skills and setups those agents depend on.

Both start agents on machines you control, so they overlap. They put the centre in different places. GOAT is built around the agent conversation, with machines underneath. reet.sh is built around the machine and what it holds, with agent sessions on top.

*Compared against GOAT's public repository and documentation on 17 November 2026. GOAT changes quickly, so check its own docs for anything you are relying on.*

## What each is for

**GOAT is an agent cockpit.** Every machine runs a GOAT server, and its desktop, web and mobile clients connect to as many of them as you like. Agents run in a native chat view with their tool calls, diffs and approvals, across a long list of providers, with cloud agents alongside the local ones. A standing assistant (GOAT Bot) can start work in the right project on the right machine and report back.

**reet.sh is a fleet control room.** One controller you host holds the inventory of your machines, the skill library, saved setups and the history of sessions. Each machine connects outbound, so the controller can open a terminal, start an agent installed there, scan what is on the machine and send it a reviewed plan.

## Choose by the job

| You want to... | Start with |
| --- | --- |
| Steer agents from a native desktop or mobile app | GOAT |
| Chat with many different agent providers in one structured view | GOAT |
| Let agents operate desktop apps, or watch an iOS or Android emulator beside a thread | GOAT |
| Keep memory that follows you across devices and conversations | GOAT |
| Keep one private skill library and see which machine has which skill | reet.sh |
| Send skills to one machine or all of them, and review the plan first | reet.sh |
| Back up setups, skills and session records, off-site if you like | reet.sh |
| Give a team accounts, roles and single sign-on over one fleet | reet.sh |
| Reach machines that have no inbound port and no VPN | reet.sh |
| Run agents on your machines with a one-binary, no-telemetry controller | reet.sh |

## How they connect machines

**GOAT:** each machine is its own environment. Clients reach it directly on your network, over Tailscale, or over SSH, where a host receives its own GOAT runtime on first use. A phone pairs with one machine acting as the hub. GOAT's managed tunnel is not available yet.

**reet.sh:** each machine runs `reet agent run` and dials out to the controller over HTTPS. The controller never opens a listening socket on the machine, so no inbound port or VPN is needed for reet itself. The machine keeps its own identity, and revoking it stops access. See [remote agents without a VPN](remote-agents-without-a-vpn.md) for the model and where a VPN still helps.

## Where reet.sh is different

- **Skills and setups are managed state.** The library, the shelf, lists and per-machine inventories are the product. GOAT's documentation describes a skills folder on each machine, shared across that machine's providers, and no fleet-wide library or sync plan.
- **Change is reviewed.** A sync plan is shown first, and the machine checks it again before writing.
- **Built for a team.** Accounts, roles, invitations, OpenID Connect sign-in, notifications and a support queue are part of the binary. GOAT's documentation describes an owner who pairs devices and clients with scoped access, plus guest links in beta, rather than team accounts.
- **Backups are built in.** Sealed, verified copies of the instance or a person's shelf, sent to any S3-compatible store.
- **Small and private.** One Go binary and one SQLite file, with no telemetry. GOAT's documentation describes product usage events that are sent by default and can be switched off.
- **Licence.** reet.sh is AGPL-3.0. GOAT is MIT.

## Where GOAT is different

- **Native clients.** A desktop app and a mobile app, where reet.sh is a web app with push notifications.
- **More agent providers in a structured view.** reet.sh's structured chat covers OpenCode, Cline and Antigravity today, with Claude Code and Codex built but off by default. Its terminal view runs the agents already installed on a machine.
- **Orchestration across threads.** Agents can start, watch and message other threads and machines through MCP tools, and a fleet goal keeps workers running until its conditions hold. reet.sh offers Factory, which runs an outcome through stages, and an MCP server for the shelf and Factory.
- **Computer use, device emulators, memory and secrets.** reet.sh has none of these.

## They can share a fleet

Nothing stops you running both. Use GOAT where you want a native cockpit on one or two machines, and reet.sh to keep the skills, setups and backups across all of them. Give each its own data and its own backup plan.

## When GOAT is the better answer

- You mainly want a native desktop or phone app to drive agents.
- You want one structured view across many providers, or cloud agents beside local ones.
- You want agents that can use the computer or an emulator, or that coordinate other agents.
- You do not need a shared skill library, reviewed sync, team accounts or backups.

## Try reet.sh

- [Install reet.sh](binary-installation.md)
- [Run Claude Code on a VPS from your browser](run-claude-code-on-a-vps.md)
- [Sync skills between machines](sync-skills-between-machines.md)

If the job is driving agents from a native app, start with GOAT. If the job is "keep these machines, skills and setups in order, and show me the plan first", that is reet.sh.
