# LibreChat and reet.sh: conversations or machine control?

LibreChat describes itself as an **open-source AI platform** that brings AI conversations into one unified, customisable interface. reet.sh is not a chat interface. It is a self-hosted control room for the agents, skills and setups across your machines.

The distinction matters when you are choosing what to run. A chat platform and a machine controller solve different problems, even when both mention AI agents.

## What each is for

**LibreChat is a conversation platform.** It focuses on a shared interface for models and conversations, with the accounts and configuration a team needs to use them.

**reet.sh is an operations layer.** It connects machines through an outbound agent, opens terminals, starts the agents already installed there, keeps sessions, and distributes skills and reusable setups with a reviewed plan.

## Choose by the job

| You want to... | Start with |
| --- | --- |
| A unified chat interface for several models | LibreChat |
| Shared conversations, accounts and model configuration for a team | LibreChat |
| A browser terminal on a remote machine | reet.sh |
| Start Claude Code, Codex, OpenCode or Gemini CLI in a folder on that machine | reet.sh |
| Keep a private skill library and review what goes to each machine | reet.sh |
| Back up setups and restore them on a fresh machine | reet.sh |

## They can share a fleet

A common arrangement is LibreChat for conversations and reet.sh for the machines where terminal agents do the work. The two do not need to know about each other. They can also be hosted on the same hardware, provided each has its own database, configuration and backup plan.

reet.sh does not provide a general chat UI, model marketplace or conversation sharing for a team. LibreChat does not provide a machine inventory, terminal sessions or reviewed skill distribution.

## Where reet.sh is different

- **The machine is the unit.** reet.sh shows what a host has, what is running there and what the last scan or sync did.
- **The agent stays where it is installed.** reet.sh starts the local agent; it does not proxy every prompt through the controller.
- **Change is reviewed.** Skill distribution returns a concrete plan, and the machine checks it again before writing.
- **Small and self-hosted.** One Go binary and one SQLite file, with no telemetry by default.

## When LibreChat is the better answer

- The team mainly needs a shared conversation interface.
- You want model configuration and accounts centred on chat, not on machines.
- You do not need remote terminals, agent sessions or skill distribution.

## Try reet.sh

- [Install reet.sh](binary-installation.md)
- [Connect your shelf to an AI client with MCP](self-host-mcp-server.md)
- [Run Claude Code on a VPS from your browser](run-claude-code-on-a-vps.md)

Choose LibreChat for conversations. Choose reet.sh when the question is where the agent runs and how its skills got there.
