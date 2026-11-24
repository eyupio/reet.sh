# Security policy

reet.sh can hold your sign-in, session cookies and personal tokens, so we take security reports seriously.

## Supported versions

Security fixes go into the latest release and the `latest` image. Please upgrade before reporting if you are behind.

## Reporting a vulnerability

**Please do not open a public issue.**

Use GitHub's private reporting: go to the repository's **Security** tab and choose **Report a vulnerability** (<https://github.com/eyupio/reet.sh/security/advisories/new>). Include:

- what the problem is and the impact you expect,
- steps or a proof of concept to reproduce it,
- the version and how you run reet.sh (`REET_AUTH` mode, behind a proxy or not).

You can expect an acknowledgement within a few days, and we will keep you informed while we fix it. We are happy to credit you in the release notes unless you prefer not to be named.

## Scope

In scope: authentication and sessions, CSRF, injection, access to another account's data, secrets leaking into logs or backups, the container image, the `reet scan` client, and the machine agent with its enrolment and its connection to the controller (see the trust model below).

Out of scope: running with `REET_AUTH=none` on an untrusted network (it is documented as open), findings that need a compromised host or browser, denial of service from unrealistic request volumes, and a compromised controller acting on the machines enrolled with it (it is trusted with them, as set out below).

## Trust model for the remote surfaces

This is what reet.sh trusts, what it can do to your machines, and what each switch controls. It describes the code as shipped. Nothing here has been checked against a relay or network other than the one the tests use.

**The controller is trusted with every machine enrolled with it.** A machine you enrol lets the controller, on your account's behalf, start sessions and chats as the user the agent runs as, read what they print, type into them, browse the folders you share, and (only where you allow it) install skills. Someone who controls the controller, its database or an admin account can do all of that to every machine that has terminal access on. The local switches below, and the lock, are how a machine's owner limits it; they are enforced by the agent on the machine, not by the controller.

**The machine agent.**
- It is one process per enrolled operating-system user, running as that user (as root only if you enrolled as root). Its service unit does not set `NoNewPrivileges`, so shells it starts can use `sudo` the way that user can.
- Connections go from the machine to the controller. The agent signs in over HTTPS (plain HTTP only for `localhost`), then holds a control connection and a terminal connection to the controller's private address. The controller never connects in to a machine.
- What it will do is set on the machine, read fresh from its private config each time. Terminal access is off until `reet agent terminal enable`. Browsing, clipboard, uploads, model listings and YOLO are each on once terminal access is on, and can be switched off one by one. Writing skills happens only with remote apply (`--allow-apply`), which is off unless you set it. Installing OpenCode uses a pinned SHA-256 artifact and needs terminal access on.
- Updating the `reet` binary is allowed whenever the agent runs as a service and the machine is not locked, whether or not remote apply is on. The binary comes from the project's GitHub release and is checked against that release's `checksums.txt`; the checksum file is not signed. If you do not want a controller to cause updates, lock the machine, or run the agent without the service.
- The lock (`reet agent lock`, below) makes the agent refuse everything the controller starts. If its config cannot be read the agent treats itself as locked.

**Enrolment.**
- A machine holds its id and its own secret (192 random bits) in `agent.json` in its user config directory: the directory is mode 0700, the file 0600, and the agent refuses a file that is group or world readable, a symlink, or owned by someone else. The controller stores only a SHA-256 hash of the secret.
- An enrolment code is single use, lasts 15 minutes and is stored hashed. A machine enrolled by code, or from a signed-in browser, can use its id and secret on a fixed list of the machine agent's routes only (identity and update checks, polling for work and posting a job's result, the private-connection address, skill reports and imports, reading a private skill's files, and its inventory scan), and nothing else.
- Removing a machine on the Machines page deletes its record and secret, ends its connection and drops the account's attach tickets.
- A machine enrolled with a personal API token is still served on that token (see the hardening tips for what revoking it does and does not cut off).

**The path between the browser, the controller and the machine.**
- The browser connects to the controller with a same-origin WebSocket and a signed-in browser session. API tokens and credentials in the URL are refused, a role above viewer is needed, and the origin is checked. Terminals need accounts mode and an `https` (or loopback) public origin.
- The controller reaches the machine over an encrypted connection the machine opens to it (Tailcat, which uses WireGuard). This is not end-to-end encryption between your browser and the machine: the controller decrypts the traffic and passes it on, so it sees terminal bytes, chat text and what you type.
- What the controller keeps: chat transcripts (up to 1000 events and 256 KiB per chat), queued chat text and session details, in its SQLite database, which is not encrypted at rest. Anyone with the database file, a backup of it, or the host can read them. Terminal scrollback is read from the machine when asked and is not stored. The text of an agent's on-screen question is held in memory only, never in SQLite or logs.
- Sessions are keyed to the account that started them, and every terminal request is checked against the signed-in account.

**Approval-bypass (YOLO) modes.**
- YOLO starts the agent with its own approval prompts off: `--dangerously-skip-permissions` for Claude and Antigravity, `--ask-for-approval never --sandbox danger-full-access` for Codex, `--auto` for OpenCode, `--yolo` for Cline and `--allow-all` for Copilot. In a structured chat on YOLO, each permission card is answered "Allow once", and switching a running chat to YOLO approves the cards already waiting. There is no standing "always allow" in chat.
- It takes two things to be on. An admin turns YOLO on for the whole instance, one switch, not per account (the message says "this account", which is wrong), and the change is logged; and the machine has not opted out. The machine's own `YoloOff` setting is the per-machine control and is checked again on the machine. Each YOLO launch is logged. The start dialog asks for a confirmation that is never remembered.
- A Factory outcome can run every one of its sessions on YOLO. That is the owner's choice for the outcome and is shown on it.

**What a relay can see.** The relay is Tailscale's DERP relay, used through Tailcat; `REET_RELAY_MAP_URL` sets the relay map (it must be `https`, and Tailcat's default is used when it is empty). It carries WireGuard traffic that ends at the controller, so it can see IP addresses, timing, packet sizes and node keys, but not terminal bytes, chat text or credentials. That follows from how Tailcat works and was not checked in its source. The controller's private address is a long-lived capability that includes a pre-shared WireGuard key; it is sealed at rest and handed to any machine that signs in. A relay outage pauses remote access and does not end the machine's own tmux sessions. More: [docs/remote-machine-operations.md](docs/remote-machine-operations.md), [docs/structured-sessions.md](docs/structured-sessions.md), [docs/controller-sync.md](docs/controller-sync.md), [docs/factory.md](docs/factory.md).

## Hardening tips for operators

- Keep the default `REET_AUTH=accounts`, and put the instance behind HTTPS if it is reachable beyond your LAN.
- Treat `REET_TOKEN` like a password and rotate it with `sh scripts/init-env.sh --rotate`.
- Enrol machines with an enrolment code (**Machines → Add a machine**, single use, 15 minutes), typed at the hidden prompt or set in `REET_ENROL_CODE` rather than passed as `--code`, which other users of the machine can see. A machine enrolled by code, or from a signed-in browser, holds only its own id and secret; they open only the routes the machine agent uses, its inventory report only for its own name, and removing the machine revokes them. A machine enrolled with a personal token (every machine enrolled before codes) needs that token on the routes it uses over the controller's public address, so revoking the token cuts it off there. Its private control and terminal connection checks only the machine's own id and secret, so revoking the token alone does not end that connection: remove the machine from the Machines page, or run `reet agent enroll --again` with a code, which replaces its secret, drops the token, keeps every local setting (remote apply included) and clears the mark. The Machines page marks a machine still on a token. Then revoke the token.
- To stop a controller acting on a machine, run `reet agent lock` on that machine (`reet agent unlock` lifts it, `reet agent lock status` shows it and how many sessions are running). While locked the machine refuses everything its controller starts: new sessions and chats, attaching to a session, typing into one, answering an agent's question or permission, reading terminal history, browsing folders, branches and clones, uploads, model and sign-in listings, software installs, remote skill imports, skill installs (even with remote apply on) and `reet` updates. The refusals are an allow-list on the machine, so a kind of request added later is refused too. Within about a second attachments are cut and an upload in progress stops; chat output and the text of an agent's on-screen question stop reaching the controller, and the controller is told no question is waiting. Only a command on the machine changes the lock; nothing the controller sends can lift it, and it survives restarts, updates, a controller move and enrolling again. **The lock stops the controller from starting or reaching anything new; it does not contain processes already running on the machine.** A session or chat started before the lock runs as the same user and can run `reet agent unlock` itself (a shell loop, or an agent in the middle of a turn). `reet agent lock` says how many are still running; `reet agent lock --end-sessions` locks and then ends every session and chat reet runs on the machine. Work the machine had already started when the lock was taken (a software install, a git branch operation, a worktree removal) is finished, not cut part-way. What the lock does not stop: inventory scans and health reports keep running, so the controller still sees the machine; a controller move is still followed (only after the new controller proves it holds the machine's record); ending a session, detaching and cancelling a chat still work, and sessions left running keep running. Nothing is deleted for the controller while locked: ending a desk shell keeps its history file. It is not a network firewall: the machine stays connected to its controller.
- Personal API tokens are listed, made and revoked only from a signed-in browser. Revoke a leaked token from the browser. What a personal API token can and cannot do, with accounts on:
  - It can do what its account does day to day through the API and the CLI: read and change its own shelf, lists and setups, scan and sync, and enrol a machine (which works only while that token does).
  - An owner's or admin's token can still read the admin listings a script might watch, none of which holds a secret: users, status and activity, the fence and which machines have moved, mail and single sign-on settings (passwords and secrets are never returned), the outbox, and the backup copies and destinations. It can also rename and merge tags, act on support tickets, and verify a backup copy (at most 6 verifies an hour per account, because each may download and open a whole copy and a wrong password is a guess).
  - It cannot mint, list or revoke API tokens, make enrolment codes or manage MCP access.
  - It cannot lock its person out or keep itself alive: listing and ending sessions, changing the password, unlinking single sign-on and adding a notification device are done only from a signed-in browser, so a leaked token cannot end the sessions its person would revoke it from.
  - It cannot do the owner's or an admin's changes: run, schedule, download, delete or prune an instance backup; stage, read or cancel an import that replaces the database at the next start; add, change, test or remove a backup destination; transfer ownership; invite people; change a person's role, disable, enable, delete them, reset their password or revoke their tokens; change instance, mail, single sign-on, notification, YOLO, maintenance, branch-pruning or software-mirror settings; raise, lift or readdress the fence; fetch the software mirror's download. These answer `403` with "do this from a signed-in browser".
  - With `REET_AUTH=token` the one instance token is also the browser's credential, so it can do everything.

## Known properties

- A machine can read any private skill bundle of its own account by id (it installs them); it cannot read another account's.
- Failed machine-secret attempts are not rate-limited. Each secret is 192 random bits, so guessing one is not practical.
- If you turn on single sign-on (`REET_OIDC_ISSUER`, see [docs/single-sign-on.md](docs/single-sign-on.md)), keep `REET_OIDC_LINK_BY_EMAIL` off unless your provider verifies every address it issues, keep `REET_OIDC_AUTO_CREATE` off unless anyone who can sign in there should get a reet.sh account, and put the client secret in `REET_OIDC_CLIENT_SECRET_FILE`. The owner always keeps password sign-in, so a provider outage never locks the instance.
- Back up `reet.db` and the exported JSON somewhere you control.
- The controller makes outbound requests to keep its own copy of each skill added by link: GitHub archives from `codeload.github.com` (up to 128 MiB each) and `https://<host>/.well-known/agent-skills/` for a skill from a site's index. If your network limits egress, allow those hosts or expect such skills to show "Couldn't fetch the files".
