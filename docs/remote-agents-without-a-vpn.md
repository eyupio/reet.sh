---
title: Run remote AI coding agents without a VPN · reet.sh
description: Connect remote coding machines to Reet through outbound HTTPS. Understand enrolment, local consent
  and where a VPN still helps.
topic: REMOTE WORK
category: remote
art: selfhost
art_icon: server
art_label: WITHIN REACH
card_title: Remote coding agents, without setting up a VPN.
hero_title: Your remote machines.
hero_accent: Within reach.
related:
- add-machine-in-context.md
- run-claude-code-on-a-vps.md
layout: article
---

# Run agents on remote machines without a VPN

A common first question about reet.sh is whether it needs a VPN or an inbound port on every machine. It does not. The agent on each machine dials out to the controller, and the controller uses that connection to send work.

This guide explains the model, what it protects, and when a VPN is still the right tool.

## The connection model

- The controller is the only address that needs to be reachable by the machines.
- Each machine runs `reet agent run`, which connects out to `REET_URL` over HTTPS.
- The controller never opens a listening socket on the machine.
- The machine keeps its own identity: an installation credential plus a personal token. Revoking either stops access.
- The connection is used for inventory, terminal sessions and reviewed sync jobs.

That is why a laptop behind a home router, a VPS with no public inbound rules and a lab box on a private network can all be enrolled the same way. If the machine can reach the controller, it can report in.

## What you need

- A controller with a stable address. Use HTTPS, preferably with `REET_PUBLIC_URL` set to the public origin.
- Outbound HTTPS from each machine to that address.
- The `reet` binary on the machine.
- A personal token from **Account → Machines**, or the bootstrap token while you are still setting up the instance.

## Enrol a machine

From **Machines → Add a machine**, copy the command the instance gives you. It looks like:

```sh
export REET_URL='https://reet.example.com'
# Set REET_TOKEN to this machine's personal API token privately.
reet agent enroll --env home-lab --allow-apply
reet agent run
```

`--allow-apply` is optional. It lets plans approved in the web UI run on this machine. Without it, the machine can still scan and preview, but applying is refused locally.

Run `reet agent run` under the machine's normal process supervisor if it should survive logout. `reet agent status` shows whether it is reporting.

## What this does not replace

A VPN or mesh network such as Tailscale is still useful for other traffic:

- reaching a database or admin port on the machine;
- SSH access when the agent is not running;
- exposing a private web service to a few people;
- network-level access control between machines that is independent of any application.

reet.sh is not a general network tunnel. It carries the controller's own protocol, not arbitrary TCP traffic. If you already have a VPN, keep it for the jobs it does well.

## Security notes

- Use HTTPS for the controller. A controller on plain HTTP exposes tokens and session data.
- Give each machine its own enrolment and personal token. Do not copy `~/.config/reet/agent.json` between machines.
- Turn on `--allow-apply` only where you want remote plans to write files.
- Revoke an old enrolment before re-enrolling the same machine with a replacement configuration.
- Keep the controller's `data/` folder as private as the machines. It holds the SQLite database and the instance's files.
- A browser terminal keeps the OS account's normal permissions, including `sudo` where that account has it. Use a dedicated account for agent work.

## Troubleshooting

- **The machine never appears.** Check that it can reach `REET_URL` and that `reet agent status` says it is running.
- **TLS errors.** Use the same hostname in `REET_URL` that the controller's certificate covers. Behind a proxy, preserve the public host and forwarded headers.
- **The machine reconnects repeatedly.** Check the controller logs and the machine's `reet doctor` output. A proxy timeout or a changing host is more likely than a fault in reet itself.
- **A remote apply fails.** Remote apply is off on that machine. Run `reet agent allow-apply on` there, or re-enrol with `--allow-apply`.

## Next steps

- [Run Claude Code on a VPS from your browser](run-claude-code-on-a-vps.md)
- [Sync skills between machines](sync-skills-between-machines.md)
- [Skills and enrolled machines](controller-sync.md)
