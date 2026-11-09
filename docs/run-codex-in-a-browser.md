---
title: "Run Codex in a browser on your own machine · reet.sh"
description: "Run the Codex CLI on an enrolled laptop or server and reach its terminal from your browser. Choose the machine, folder and session in Reet."
layout: article
topic: REMOTE WORK
category: remote
art: remote
art_icon: monitor
art_label: FROM ANYWHERE
card_title: "Run Codex on your machine. Reach it from your browser."
hero_title: "Codex on your machine."
hero_accent: "Your browser, anywhere."
related:
  - run-claude-code-on-a-vps.md
  - parallel-coding-agents-git-worktrees.md
---

# Run Codex in a browser on your own machine

The Codex CLI can run on a machine you own while you use its terminal from another device. Reet supplies the machine connection, agent launcher and browser terminal. Codex supplies the coding agent and uses its own authentication and permissions.

This setup suits a project that already lives on a server, a laptop you want to reach from another device, or a development machine with tools you do not want to recreate elsewhere.

## What you need

- A running Reet controller, with an HTTPS address reachable from the machine doing the work.
- A supported Linux or macOS machine with the Reet binary installed.
- Codex installed and authenticated as the OS user that will run the session.
- A signed-in Reet browser account with a write role, and remote terminal access enabled on the server and machine.
- A project folder that the machine’s enrolled scopes allow you to use.

[Use the quick start](quick-start.md) if you have not installed the controller. Reet does not supply a Codex subscription or API credentials, and it does not sign in to Codex for you.

## 1. Check Codex on the machine

Before involving the browser, check that Codex works in a normal terminal under the account that will run Reet’s connector. Confirm that the account can open the repository and use the tools the project needs.

A tool installed for another OS user or a different shell environment may not be visible to the connector. Start with a working local command rather than debugging both installations at once. Reet’s agent picker shows what the machine reports as available.

## 2. Enrol the machine

In your Reet instance, choose **Machines → Add a machine**. Install the Reet binary on the target machine, then run the enrolment command from the dialog. Keep the token private and use a separate enrolment for each machine.

Start the connector after enrolling:

```sh
reet agent run
```

It connects outbound to the controller. You do not need an inbound port on the machine for Reet’s protocol. If the connection needs to survive logout, run it with the machine’s normal process supervisor.

Use the instance’s readiness instructions to enable browser terminals. Enrolment and terminal consent are separate controls; a machine that can report inventory may still need setup before it can start a terminal.

## 3. Launch a Codex session

1. Choose **Start an agent** from Home or the machine’s card.
2. Select the connected machine and **Codex**.
3. Choose your project folder. If entering a path, use an absolute path within an enrolled scope.
4. Add an opening prompt if useful. Codex prompts must contain at least two words.
5. Review the available launch settings, then press **Start**.

Reet opens a browser terminal attached to the new session. The browser names the tool and chosen settings; the machine resolves its installed program and supported flags. The normal launch path keeps the coding agent’s approval behavior. Explicitly changing a project’s approval mode is a separate decision with its own controls.

If Codex is unavailable in the picker, check the installation and machine inventory rather than selecting another tool and trying to turn it into a Codex command.

## 4. Review the result

The session workspace includes **Overview**, **Changes**, **Files** and **GitHub** views. Inspect the checkout or worktree diff in **Changes**, and run the checks the project requires before accepting an edit.

The **GitHub** view uses the `gh` CLI authenticated on the session’s machine. Starting Codex does not automatically establish that GitHub authentication.

A running session is not proof of useful progress. Read the terminal, review approvals, and check the result. A finished session also does not by itself mean the task succeeded.

## 5. Return to the session

Open **Sessions** to find work across your machines. Reopen a running session to return to its terminal.

For an exited Codex session, Continue starts `codex resume --last` in the same folder, which carries on the latest interactive Codex session there; this was measured with a stand-in provider, not with a signed-in account. Codex's own `codex resume` picker is still available from a terminal for any other conversation.

## Keep simultaneous work separate

If several agents will edit one repository, give each session its own worktree where supported. A browser tab does not isolate a checkout. Shared files can still be changed by another process on the same machine.

The [parallel agents guide](parallel-coding-agents-git-worktrees.md) explains the difference between a separate session and a separate working tree.

## Troubleshooting

| What you see | What to check |
| --- | --- |
| The machine is missing or reconnecting | Run `reet agent status` on the machine and check its connection to the controller. |
| Codex is disabled in the picker | Check the installation as the connector’s OS user, then refresh the reported inventory. |
| The folder is refused | Use an absolute path inside an enrolled home or project scope. |
| The terminal starts but Codex needs sign-in | Authenticate Codex on that machine; a started program does not prove provider access. |
| Pull request information is unavailable | Check `gh` authentication on the machine that owns the session. |

For launch details and capability limits, read [Start an agent session](start-agent-session.md). For the terminal itself, see [Terminal workspace](terminal-workspace.md).
