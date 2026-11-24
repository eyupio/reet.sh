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
