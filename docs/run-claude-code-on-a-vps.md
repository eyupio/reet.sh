# Run Claude Code on a VPS from your browser

Claude Code is happiest in a terminal. If the machine you want it on is a VPS, a lab box or a desktop you are not sitting at, reet.sh gives you that terminal in a browser and keeps the session with the rest of your fleet.

This guide uses Claude Code as the example. The same flow starts Codex, OpenCode and Gemini CLI when the machine reports them as installed.

## What you need

- A machine to run the reet.sh controller: a small VPS, a home server, or a laptop. Docker Compose is the quickest path.
- A second machine for Claude Code. It needs a supported Linux or macOS install, a shell, and Claude Code already installed and signed in.
- A folder you are allowed to work in on that machine.

reet.sh starts the agents that are already on the machine. It does not install Claude Code, and it does not send your prompts or files anywhere except the controller you host.

## 1. Put reet.sh on the controller

On the controller machine:

```sh
git clone https://github.com/eyupio/reet.sh && cd reet.sh
sh scripts/init-env.sh
docker compose up -d
```

Open `http://localhost:8080` (or the address of your controller) and create the owner account. If `REET_TOKEN` was generated, the setup page asks for it. Keep the `data/` folder: it holds the SQLite database and the files the instance needs.

If the controller is on the public internet, put it behind HTTPS before you use it from other machines. Set `REET_PUBLIC_URL` to the public origin, for example `https://reet.example.com`.

## 2. Install the reet agent on the VPS

On the VPS, install the `reet` binary:

```sh
curl -fsSL https://raw.githubusercontent.com/eyupio/reet.sh/main/scripts/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"
reet --version
```

The installer downloads a published release, verifies its SHA-256 checksum and puts the binary in `~/.local/bin`. See [Install the reet binary](binary-installation.md) for pinned versions and manual installs.

## 3. Enrol the VPS

In the controller, open **Machines → Add a machine**. The dialog gives you the exact enrolment command for this instance. It looks like this:

```sh
export REET_URL='https://reet.example.com'
# Set REET_TOKEN to this machine's personal API token privately.
reet agent enroll --env hetzner-vps --allow-apply
reet agent run
```

`--env` is the name you will see in the UI. Use a name you recognise, such as `hetzner-vps` or `studio`.

`--allow-apply` is optional and off by default. It lets this machine execute sync plans that a person approves in the web UI. Without it, scans and previews still work, but remote apply reports a failure without changing files. You can change it later with `reet agent allow-apply on` or `off` on the machine. Only the machine itself can turn it on.

Keep `reet agent run` under your usual process supervisor if it should survive logout. `reet agent status` and `reet doctor` help when it does not report in.

The machine dials out to the controller, so you do not need to open an inbound port on the VPS.

## 4. Start Claude Code

When the machine shows as ready:

1. Open **Home** and choose **Start an agent**, or use **New agent session** on the machine's card.
2. Choose the VPS, then **Claude Code**.
3. Pick the folder, or type an absolute path inside the machine's enrolled home folder.
4. Add an opening prompt if you want one. For Claude Code, the prompt must be at least two words.
5. Press **Start**.

You land in a terminal attached to the new Claude Code session, in the browser. The machine decides how the agent starts; reet does not invent a command for an agent it cannot see.

Claude Code's own permissions still apply. Use a dedicated user on the VPS, and review what the agent is allowed to run.

## 5. Continue later

**Sessions** lists every session across your machines. Open one to return to its terminal, or use the machine's terminals directly.

When a session has exited, **Manage → Continue session…** starts a new terminal in the same folder and asks the agent to resume its last conversation there. OpenCode supports that resume path today; other agents wait for a reviewed resume command. For Claude Code, start a new session in the same folder and use Claude Code's own conversation history.

## 6. Review changes and CI

The session workspace beside the terminal has **Overview**, **Changes**, **Files** and **GitHub** views. Use **Changes** to inspect the checkout or worktree diff, and **GitHub** to see pull request reviews, conflicts and CI checks. GitHub status uses the `gh` CLI signed in on the session's machine; it is not a separate GitHub App.

## Troubleshooting

- **The machine is not listed.** Check `reet agent status` on the VPS and that `REET_URL` is reachable from it.
- **Claude Code is disabled in the agent picker.** The machine does not report it as installed. Install and sign in to Claude Code on that machine, then run **Scan all**.
- **The folder cannot be chosen.** A typed path must be absolute and inside the home folder the agent was enrolled with. Add another project with `--project /absolute/path` when you enrol.
- **The start is refused.** Read the sentence in the dialog. Common causes are an old agent build, a machine that is reconnecting, or an opening prompt that is too short for Claude Code.

## Limits

- reet.sh does not install, update or authenticate Claude Code. It starts the agent your machine already has.
- A browser terminal keeps the OS account's normal permissions, including `sudo` if that account has it.
- The controller sees the machine's inventory and session metadata. Keep the controller and its `data/` folder as private as the machines themselves.
