---
title: "Self-hosting AI coding agents: a practical Reet checklist"
description: "Plan your Reet controller, machine accounts, HTTPS, agent access, skill-sync review and backups before depending on a self-hosted agent workspace."
layout: article
topic: SELF-HOSTING
category: selfhost
art: checklist
art_icon: shield
art_label: START PROPERLY
card_title: "Self-host your agents. Keep the setup maintainable."
hero_title: "A little preparation."
hero_accent: "A better place to work."
related:
  - quick-start.md
  - backups.md
---

# Self-hosting AI coding agents: a practical checklist

The first agent session is only part of a useful setup. Once Reet becomes the place you return to every day, you also need a clear controller address, machine accounts you understand, and a way to recover the instance.

Use this checklist before expanding beyond one machine. It records the decisions and checks that matter without requiring you to build an elaborate platform first.

## Choose the controller’s home

- [ ] Choose a host you can maintain and recover.
- [ ] Set a persistent data directory and record where it lives.
- [ ] Check how you will restart Reet after a reboot.
- [ ] Keep a record of the binary version and your deployment configuration.

The controller stores its SQLite database in its data directory. Reinstalling the binary and restoring an instance are different jobs. See [binary installation](binary-installation.md) and [backups](backups.md) before treating the controller as disposable.

## Give it a stable HTTPS address

- [ ] Serve a remote controller over HTTPS.
- [ ] Set `REET_PUBLIC_URL` to the public origin you intend to use.
- [ ] Preserve the intended host and forwarding headers in your reverse proxy.
- [ ] Check that each machine can connect outbound to that address.

The address is part of browser, token and connection behavior. A machine that reports inventory but repeatedly reconnects may have a proxy or reachability problem. Start with the status shown by the instance and the machine’s `reet doctor` output.

Reet’s outbound connector avoids an inbound Reet port on each machine. It does not provide general access to every private service on that host.

## Review the OS account doing the work

- [ ] Run the connector as the OS user intended to run coding agents.
- [ ] Check that user’s access to project folders and credentials.
- [ ] Review whether the account can use `sudo` or reach other sensitive resources.
- [ ] Enrol only the home and project scopes you intend to expose.

A browser terminal keeps the OS account’s normal permissions. A worktree separates files in a checkout; it does not isolate the agent from the machine’s other resources.

Use a dedicated account where that gives you a clearer boundary. Keep machine enrolments and their tokens separate rather than copying one connector configuration between hosts.

## Check agent and provider access

- [ ] Install and authenticate the chosen coding agent on its machine.
- [ ] Confirm it works in a normal terminal as the connector’s OS user.
- [ ] Check which models and launch settings the machine actually reports.
- [ ] Verify a small task and its required approvals before starting broader work.

Reet can start the installed agents the machine advertises. It can also manage its own OpenCode copy on supported Linux machines through a reviewed plan. It does not supply the other tools’ authentication or include their model usage.

A process starting is not proof of sign-in or useful progress. Read the terminal and inspect the result.

## Keep changes reviewable

- [ ] Start with ordinary agent approvals.
- [ ] Use separate worktrees for concurrent terminal tasks where supported.
- [ ] Review diffs and run the project’s checks.
- [ ] Inspect skill-sync previews before approving applies.

For an explicit skill preview, use **Advanced sync**. The simpler **Sync now** action can install and replace skills automatically, and **Keep synced** maintains the selection on later polls. Enable those flows only where you want that behavior. The destination machine returns a concrete plan, and local remote-apply consent is a separate gate. Keep the distinction between your library, desired selections and installed files clear.

[Sync skills between machines](sync-skills-between-machines.md) is a good first trial of that flow.

## Plan access and recovery

- [ ] Use browser accounts and roles appropriate to the people using the instance.
- [ ] Review [single sign-on](single-sign-on.md) if your team uses an identity provider.
- [ ] Create and verify a backup using the [documented backup workflow](backups.md).
- [ ] Keep recovery material private and available when the original host is unavailable.
- [ ] Record how to revoke an old machine enrolment or client connection.

Do a recovery check before the only copy of the instance is the one you are trying to recover. Follow the backup guide for the exact archive and encryption requirements; copying an arbitrary database file is not a substitute for that workflow.

## Expand from working evidence

Once the first host can start an agent, preserve its data and restore a verified backup, add the next machine. Check readiness and capabilities on that machine rather than assuming it behaves identically.

A small setup you understand is a useful starting point. Add more machines, sync scopes and launch defaults as you have a concrete job for them.
