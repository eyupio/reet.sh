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
- If you turn on single sign-on (`REET_OIDC_ISSUER`, see [docs/single-sign-on.md](docs/single-sign-on.md)), keep `REET_OIDC_LINK_BY_EMAIL` off unless your provider verifies every address it issues, keep `REET_OIDC_AUTO_CREATE` off unless anyone who can sign in there should get a reet.sh account, and put the client secret in `REET_OIDC_CLIENT_SECRET_FILE`. The owner always keeps password sign-in, so a provider outage never locks the instance.
- Back up `reet.db` and the exported JSON somewhere you control.
