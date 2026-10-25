# Single sign-on

A team that runs reet.sh for other people, or for their own organisation, often wants everyone to
sign in through the identity provider they already have. reet.sh can be the relying party of any
OpenID Connect provider (Keycloak, Authentik, Authelia, Entra ID, Okta, Google and the like). It
never issues identities of its own, and the owner account keeps its password.

Nothing changes until `REET_OIDC_ISSUER` is set. With it set, the sign-in page offers a button as
well as the password form, and everything else (setup, the owner's password, personal API tokens,
backups) works as before.

## In the interface

- **Sign-in page.** A button, worded by `REET_OIDC_BUTTON_LABEL`, leads to the identity provider and
  brings the person back to the page they started from (a deep link, or the connect-an-agent consent
  page). Where password sign-in is on, the password form stays below it. With
  `REET_OIDC_PASSWORD_LOGIN=off` the form is folded under "Sign in with a password" with a note that
  only the owner can use it, so the owner always has a way in. With single sign-on off the page is
  exactly what it was.
- **When a sign-in cannot finish** the person lands on the sign-in page (or, if already signed in, on the
  page they were on) with one plain sentence for the code in the address, and the code is taken off the
  address so a reload does not say it again. A code the interface does not know gets a general sentence;
  anything that is not a plain code is ignored.
- **Account, Single sign-on.** Shown only when it is on. It says when the identity was linked and last used,
  links one (`Link your identity`, which comes back to the same page), and unlinks it with a confirmation.
  Unlinking is disabled, with the reason, while it would leave no way to sign in.
- **Admin, Users.** A read-only panel with the issuer, the redirect address, whether the provider can be
  reached right now, how new people and linking by email are handled, the group mappings and how many people
  are linked. It never shows, and the server never sends, the client secret.

## What you need from the provider

Register reet.sh there as a confidential client using the authorisation-code flow, and give it this
redirect address:

```
<REET_PUBLIC_URL>/api/v2/auth/oidc/callback
```

reet.sh sends PKCE (S256) and authenticates to the token endpoint with the client id and secret in
a basic authorization header (`client_secret_basic`). ID tokens must be signed RS256 or ES256.

For people to be signed out here when they sign out at the provider, register this as the client's
back-channel logout address (optional):

```
<REET_PUBLIC_URL>/api/v2/auth/oidc/backchannel-logout
```

## Settings

| Variable | Default | Purpose |
| --- | --- | --- |
| `REET_OIDC_ISSUER` | empty | The provider's issuer address, exactly as its discovery document names it. Setting it turns single sign-on on. |
| `REET_OIDC_CLIENT_ID` | empty | The client id registered at the provider. |
| `REET_OIDC_CLIENT_SECRET` or `_FILE` | empty | The client secret, or a file that holds it. Setting both is refused. |
| `REET_OIDC_REDIRECT_URL` | `REET_PUBLIC_URL` + `/api/v2/auth/oidc/callback` | The address the provider sends people back to. |
| `REET_OIDC_SCOPES` | `openid profile email` | Scopes asked for. `openid` is always included. |
| `REET_OIDC_NAME_CLAIM`, `_EMAIL_CLAIM`, `_GROUPS_CLAIM` | `name`, `email`, `groups` | Which claims carry the person's name, email address and groups. |
| `REET_OIDC_ADMIN_GROUPS`, `_MEMBER_GROUPS`, `_VIEWER_GROUPS` | empty | Group names (comma or space separated) that give the admin, member or viewer role. The highest match wins. |
| `REET_OIDC_DEFAULT_ROLE` | `viewer` | `member` or `viewer`: the role of a new person who matches no group. |
| `REET_OIDC_AUTO_CREATE` | `off` | `on` to create an account for a person reet.sh has not met. |
| `REET_OIDC_LINK_BY_EMAIL` | `off` | `on` to link a first sign-in to the existing account with the same, verified email. |
| `REET_OIDC_PASSWORD_LOGIN` | `on` | `off` to refuse password sign-in for everyone except the owner. |
| `REET_OIDC_BUTTON_LABEL` | `Sign in with single sign-on` | Text on the sign-in button. |
| `REET_OIDC` | empty | `off` switches single sign-on off without removing the rest of the settings. |

A half-set configuration stops the server from starting and says what to fix, rather than starting
up looking like single sign-on is off. Single sign-on needs `REET_AUTH=accounts`.

## How a person is recognised

1. The provider's subject (`sub`) under this issuer is the identity. A person who has signed in
   before, or who linked their account, is found by it.
2. A person reet.sh has not met is matched by email only if `REET_OIDC_LINK_BY_EMAIL=on` **and** the
   provider says the address is verified (`email_verified`). The owner and admin accounts are never
   linked by email: a matching address must not hand over the keys.
3. Otherwise, with `REET_OIDC_AUTO_CREATE=on` a member account is created, again only for a verified
   address. It has no password. Without it the person is told no account is available.
4. A person who is signed in can link their own identity (`GET /api/v2/auth/oidc/start?intent=link`;
   the Account page offers it), and unlink it again
   (`DELETE /api/v2/auth/oidc/identity`) as long as that leaves them a way in: a password, and
   password sign-in not turned off.

The owner is never created or linked by single sign-on. Ownership comes only from setup and from
transfer. Before setup is done, single sign-on signs nobody in.

## Roles

When any of the group settings is used, the provider decides the role at each sign-in: the highest of
admin, member and viewer that a group matches, and `REET_OIDC_DEFAULT_ROLE` when groups are sent but
none matches (a typo gives read-only, never more). A token with no groups claim at all changes nobody's
role. The owner's role is never changed by the provider. With no group settings, roles are set by an
admin as usual, and a new person gets the default role.

## Disabling a person

An admin disables an account under **Admin, Users**, or through the same API
(`PATCH /api/v2/admin/users` with `{"id": 7, "disabled": true}`). That ends the person's sessions and
personal tokens and closes their open terminals, and the provider can no longer sign them in. It
works the same for accounts made by single sign-on and by password.

## Errors

A sign-in that cannot finish sends the browser to `/?sso_error=CODE`, with only a fixed code and never
anything the provider said:

| Code | Meaning |
| --- | --- |
| `denied` | The person cancelled, or the provider refused. |
| `expired` | The sign-in took too long, or was already used. Start again. |
| `mismatch` | The sign-in was finished in a different browser from the one that began it. |
| `failed` | The provider's answer could not be trusted. The server log says why. |
| `unavailable` | The provider cannot be reached. |
| `not_ready` | The instance has no owner yet. Set it up first. |
| `no_account` | No account here, and accounts are not created automatically. |
| `exists` | An account with that email exists and is not linked. Ask an admin, or link from Account. |
| `not_linkable` | That email belongs to an owner or admin, which single sign-on will not take over. |
| `email_missing`, `email_unverified` | The provider sent no email, or does not vouch for it. |
| `disabled` | The account is disabled. |
| `already_linked`, `linked_elsewhere` | The account, or the identity, is already linked. |
| `signin_required` | Linking needs the person to stay signed in here. |
| `rate_limited`, `busy` | Too many sign-ins are starting. Try again shortly. |

## Behind the scenes

- The sign-in is bound to the browser that began it by a short-lived cookie, and to the provider's
  answer by `state`, `nonce` and PKCE. An attempt works once and lasts ten minutes; at most 1,000 can
  be waiting.
- After sign-in the browser goes to the `next` path it began from, if that is a path on this site, so
  a sign-in that interrupts the connect-an-agent consent page resumes there.
- The provider's discovery document and key set are fetched with a timeout, a size cap and no
  redirects, and never from a link-local address.
- `GET /api/v2/auth/status` gains `sso: {enabled, label, passwordLogin}`. `GET /api/v2/admin/sso`
  (admins) shows how it is set up and whether the provider can be reached. Neither shows the secret.
