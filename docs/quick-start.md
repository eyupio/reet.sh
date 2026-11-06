---
title: "reet.sh quick start — install and connect your first machine"
description: "Install Reet on Linux or macOS, start your control room, connect a machine and run your first AI coding agent session."
label: GET STARTED
---

# A proper start.

One binary. Your own control room. Let’s get Reet running and connect the machine where your work happens.

!!! abstract "Before you start"
    You need **Linux or macOS**, on **amd64 or arm64**, and a terminal. The binary includes the web UI and local scanner: no Go or Node installation is needed to run Reet.

## 1. Install the binary

```sh
curl -fsSL https://raw.githubusercontent.com/eyupio/reet.sh/main/scripts/install.sh | sh
```

The installer verifies the download’s SHA-256 checksum and installs `reet` into `~/.local/bin`, without sudo. Add that directory to your shell’s `PATH` if it is not already there, then check:

```sh
reet --version
```

[Read the installer first](https://github.com/eyupio/reet.sh/blob/main/scripts/install.sh), or see [binary installation](binary-installation.md) for manual downloads, version selection and updates.

## 2. Open your control room

Choose a data directory and start the server:

```sh
REET_DATA_DIR="$HOME/.local/share/reet" reet serve
```

Open `http://localhost:8080` and create the owner account. Keep the data directory when you update Reet: it holds your SQLite database.

!!! tip "Putting it on the internet?"
    Serve the controller over HTTPS through your reverse proxy and set `REET_PUBLIC_URL` to its public origin. Keep the data directory private. The [installation guide](binary-installation.md) covers deployment options; [backups](backups.md) covers recovery.

## 3. Connect your first machine

Install the Reet binary on the machine where the work will run. This can be the controller’s own host or a different laptop, VPS or lab machine.

1. Open **Machines → Add a machine** in your instance.
2. Copy the enrolment command it gives you and run it on the machine you want to connect. Keep its personal token private.
3. Start its connector:

    ```sh
    reet agent run
    ```

The machine makes an outbound connection to the controller. Reet does not need an inbound port on that machine. For a persistent connection, run the connector under the machine’s normal process supervisor so it survives logout.

[Add a machine](add-machine-in-context.md) explains enrolment, project scopes and the machine’s local controls. Remote terminal access needs both server enablement and the machine’s consent; follow the readiness instructions shown by your instance.

## 4. Put an agent to work

Install and authenticate your chosen coding agent on the connected machine. Reet can start **Claude Code**, **Codex**, **OpenCode** or **Gemini CLI** when the machine reports it as available. Its own OpenCode copy can also be managed on [supported Linux machines](agent-software.md).

Choose **Start an agent**, pick a project and a machine, then an agent. Choose **No project — start in a folder** to pick a folder instead. Press **Start** and you arrive in the new session. Your coding tool’s provider access and approval rules still apply.

A started terminal means the program started; it does not prove the tool is signed in or making progress. If the session needs attention, use the reason shown in the UI and check the tool on the machine.

## Where to go next

<div class="grid cards" markdown>

- **Take it remote**

    [Run Claude Code on a VPS from your browser](run-claude-code-on-a-vps.md).

- **Bring some order**

    [Sync skills between machines](sync-skills-between-machines.md) with an exact plan to review before applying.

- **Keep the work separate**

    [Use worktrees for parallel coding agents](parallel-coding-agents-git-worktrees.md).

- **Look after your instance**

    [Run through the self-hosting checklist](self-hosted-ai-agents-checklist.md).

</div>
