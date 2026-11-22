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

Install and authenticate your chosen coding agent on the connected machine. Reet can start **Claude Code**, **Codex** or **OpenCode** when the machine reports it as available. Its own OpenCode copy can also be managed on [supported Linux machines](agent-software.md).

Add a project with **Add** in the project rail on Home: a folder on a machine, or **From GitHub**, a repository the machine clones for you into `~/reet/repos/owner/name`, chosen from the repositories its GitHub sign-in reaches or typed (that machine needs terminal access on, git and the GitHub CLI signed in with `gh auth login`; see [Projects](projects.md#adding-from-github)).

Choose **Start an agent**, pick a project and a machine, then an agent. Choose **No project — start in a folder** to pick a folder instead. Press **Start** and you arrive in the new session. Your coding tool’s provider access and approval rules still apply.

A started terminal means the program started; it does not prove the tool is signed in or making progress. If the session needs attention, use the reason shown in the UI and check the tool on the machine.

### Your first session

Once your first machine is connected and you have no sessions yet, Home shows **Your first session**. It is optional: Factory, Sessions, terminals and **Start an agent session** stay where they are, and **Not now** puts it away for good in that browser.

1. **Agents on the machine.** Each installed agent is one line: a ✓ and who it is signed in as on that machine, in the same words the Factory uses before it approves a run. A line says more only when something is wrong, such as no sign-in or a model list reet could not read.
2. **One question that changes nothing.** The guide suggests asking the agent to say which folder it is in and list what it can see, and to change, create or delete nothing. It starts a normal agent session without YOLO, in the machine’s home folder or in a project folder you pick from the ones you already have. The session is named **First look**.
3. **The reply.** When the agent has answered, the guide says so and gets out of the way. **Read the reply** opens the session. If the session ended first, it says so and you can try again.
4. **Signed out?** That is not an error. The guide shows the agent’s own line, such as “No Claude Code sign-in was found on studio”, and says to sign in to the agent on that machine. If another installed agent is ready, **Use … instead** switches to it. While a sign-in is missing, reet asks the machine again every 15 seconds, and **Check again** asks at once, so the guide moves on when the sign-in is there. With nothing missing it stops asking.
5. **Use reet on your phone.** Open this controller’s address in your phone’s browser, sign in, and use the browser’s **Add to Home Screen**. To be told when a session needs you, turn on notifications on the phone; see [notifications on a device](push-notifications.md), which needs accounts and https.

Someone who already has sessions never sees the guide.

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
