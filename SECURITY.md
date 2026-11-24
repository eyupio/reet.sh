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

In scope: authentication and sessions, CSRF, injection, access to another account's data, secrets leaking into logs or backups, the container image, and the `reet scan` client.

Out of scope: running with `REET_AUTH=none` on an untrusted network (it is documented as open), findings that need a compromised host or browser, and denial of service from unrealistic request volumes.

## Hardening tips for operators

- Keep the default `REET_AUTH=accounts`, and put the instance behind HTTPS if it is reachable beyond your LAN.
- Treat `REET_TOKEN` like a password and rotate it with `sh scripts/init-env.sh --rotate`.
- Enrol machines with an enrolment code (**Machines → Add a machine**, single use, 15 minutes), typed at the hidden prompt or set in `REET_ENROL_CODE` rather than passed as `--code`, which other users of the machine can see. A machine enrolled by code, or from a signed-in browser, holds only its own id and secret; they open only the routes the machine agent uses, its inventory report only for its own name, and removing the machine revokes them. A machine enrolled with a personal token (every machine enrolled before codes) needs that token on every request, so revoking the token cuts it off; the Machines page marks it, and `reet agent enroll --again` with a code replaces its secret, drops the token, keeps every local setting (remote apply included) and clears the mark. Then revoke the token.
- To stop a controller acting on a machine, run `reet agent lock` on that machine (`reet agent unlock` lifts it, `reet agent lock status` shows it). While locked the machine refuses everything its controller starts: new sessions and chats, attaching to a session, typing into one, answering an agent's question or permission, reading terminal history, browsing folders, branches and clones, uploads, model and sign-in listings, software installs, remote skill imports, skill installs (even with remote apply on) and `reet` updates. The refusals are an allow-list on the machine, so a kind of request added later is refused too. Attachments are cut within a second and chat output stops reaching the controller. Only a command on the machine changes the lock; nothing the controller sends can lift it, and it survives restarts, updates, a controller move and enrolling again. What it does not stop: inventory scans and health reports keep running, so the controller still sees the machine; a controller move is still followed (only after the new controller proves it holds the machine's record); ending a session, detaching and cancelling a chat still work; running sessions and chats keep running and nothing is deleted. It is not a network firewall: the machine stays connected to its controller.
- Personal API tokens are listed, made and revoked only from a signed-in browser. A leaked token therefore cannot mint or revoke tokens. It can still do what its account can through the API: for example enrol a machine, which works only while that token does, and, for an admin, invite people. Revoke a leaked token from the browser.

## Known properties

- A machine can read any private skill bundle of its own account by id (it installs them); it cannot read another account's.
- Failed machine-secret attempts are not rate-limited. Each secret is 192 random bits, so guessing one is not practical.
- If you turn on single sign-on (`REET_OIDC_ISSUER`, see [docs/single-sign-on.md](docs/single-sign-on.md)), keep `REET_OIDC_LINK_BY_EMAIL` off unless your provider verifies every address it issues, keep `REET_OIDC_AUTO_CREATE` off unless anyone who can sign in there should get a reet.sh account, and put the client secret in `REET_OIDC_CLIENT_SECRET_FILE`. The owner always keeps password sign-in, so a provider outage never locks the instance.
- Back up `reet.db` and the exported JSON somewhere you control.
- The controller makes outbound requests to keep its own copy of each skill added by link: GitHub archives from `codeload.github.com` (up to 128 MiB each) and `https://<host>/.well-known/agent-skills/` for a skill from a site's index. If your network limits egress, allow those hosts or expect such skills to show "Couldn't fetch the files".
