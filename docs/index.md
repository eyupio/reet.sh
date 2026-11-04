---
title: reet.sh documentation
description: Install reet.sh, connect machines, run AI agents, sync skills and setups, and operate a self-hosted fleet.
---

# reet.sh documentation

reet.sh is a self-hosted control room for the AI agents, skills and setups across your machines. It connects a laptop,
a VPS or a lab box through an outbound agent, gives you a browser terminal on each one, and keeps a central library of
skills and reusable setups that you can send to one machine or the whole fleet.

One Go binary. One SQLite file. No telemetry.

## Start here

<div class="grid cards" markdown>

-   **Install reet.sh**

    ---

    Put the controller on a machine you control, then create the owner account. The binary install is the quickest
    path for a single host.

    [Install the reet binary](binary-installation.md)

-   **Connect a machine**

    ---

    Run the outbound agent on the machine where the work happens. No inbound port is needed, and the machine reports
    the agents, skills and setups it already has.

    [Add a machine](add-machine-in-context.md)

-   **Run an agent**

    ---

    Open a terminal on any enrolled machine and start Claude Code, Codex, OpenCode or Gemini CLI in a folder. Pick the
    session up again later.

    [Start an agent session](start-agent-session.md)

-   **Sync skills**

    ---

    Keep a central library, choose a source and destinations, preview the exact plan, and approve it before anything
    changes.

    [Skills and enrolled machines](controller-sync.md)

</div>

## Guides

- [Run Claude Code on a VPS from your browser](run-claude-code-on-a-vps.md)
- [Sync skills between machines](sync-skills-between-machines.md)
- [Connect an AI client to your shelf with MCP](self-host-mcp-server.md)
- [Run agents on remote machines without a VPN](remote-agents-without-a-vpn.md)

## How it fits together

- **Machines** are enrolled hosts. Each one runs `reet agent run` and dials out to the controller.
- **Agents** are the tools already installed on those machines. reet starts them; it does not install them.
- **Skills** are the reusable instructions and files your agents use. They live in a central library and can be
  distributed with a reviewed plan.
- **Setups** are named AI configurations you can restore on a new machine.
- **Sessions** are the terminals and agent conversations you return to.

## Where to go next

- [Agent software](agent-software.md) — observe, install, update or roll back reet's own OpenCode copy.
- [Connect your shelf to an AI client](mcp.md) — the hosted MCP server, OAuth and permissions.
- [Backups](backups.md) — encrypted, verified copies of the instance and personal shelves.
- [Single sign-on](single-sign-on.md) — OIDC, roles and account policy.
- [Command reference](cli-reference.md) — every `reet` command.

## Source and licence

reet.sh is developed by [EyUp.io](https://eyup.io) and licensed under
[AGPL-3.0](https://github.com/eyupio/reet.sh/blob/main/LICENSE). The source, issues and releases are on
[GitHub](https://github.com/eyupio/reet.sh).
