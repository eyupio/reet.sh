# Email

reet sends transactional email, not marketing: password resets, password-change
notices, invitations, welcomes, and the notices a team's administrators choose
for their own team. There is nothing to unsubscribe from on the account
messages; team notices can be turned off by that team's owner or managers.

The code is `internal/mail`; its SQL is `internal/store/store_mail.go`.

## Two layers of setup

1. **The instance owner** sets the one mail server every message goes through,
   on **Admin → Email**. It applies to the next message without a restart.
2. **A team's owner or managers** choose what that team is emailed about, on
   **Teams & organisations → *team* → Notifications**. Every kind is on until
   they choose otherwise.

An ordinary member can read their team's page but cannot change its
notifications. A team notice goes to the owner and managers by default, or to
every member, and any extra addresses the team adds. Extra addresses receive
the email but are not given access to the team, its lists or anything else.

## How a message travels

Every message is a row in the outbox before anything tries to send it, so a
request never waits on a mail server and an outage costs a retry, not an email.

- **Once.** Each row has an idempotency key. A password reset is keyed on its
  token hash, an invitation on its address and expiry, a welcome on the
  account, and a team notice on the group, event, occurrence and recipient.
  A retried request returns the message already there.
- **Retries.** A failed send is tried again after a minute, doubling to at
  most two hours, with jitter; after ten tries it is `failed`. A 5xx reply to
  the recipient or the message is `bounced` at once. A 5xx to the greeting,
  sign-in or sender is a settings problem, so it is retried and the links are
  kept while the owner fixes it.
- **Crash-safe.** A row left `sending` by an instance that stopped mid-send is
  put back when reet starts, and sent again. The `Message-ID` is the row's, so
  a client folds a duplicate into one.
- **Sign-in links are not kept.** A reset or invitation carries a link. Its
  body is blanked once the message is delivered or given up on, and the admin
  page never shows it. Tokens are stored only as their SHA-256.
- **Nothing is kept for ever.** Delivered, failed and bounced messages are
  deleted 90 days after their last attempt, and expired reset links are
  removed. A queued, sending or held message is never deleted.

## Configuration

The mail server is set only on **Admin → Email**, and only by the owner.

| Field | Default | What it is |
| --- | --- | --- |
| Mail server | — | The SMTP host. |
| Port | 587, or 465 for implicit TLS | |
| Security | STARTTLS | STARTTLS, implicit TLS (from the first byte), or none. None is refused for any host but `localhost`, `127.0.0.1` or `::1`: it would put the password and every reset link on the wire. With STARTTLS, a server that does not offer it is refused before the password is sent. |
| Username | — | Signs in with AUTH PLAIN, or LOGIN where that is all the server offers. Empty sends without signing in. |
| Password | — | Write-only: sealed, never shown again. |
| From address | — | Bare (`hello@example.com`); the name has its own field, and `Name <addr>` is refused. Required with a server. |
| From name | `reet.sh` | |
| Reply-to | — | Where a reply goes. Empty sends replies to the from address. |
| Links start with | `REET_PUBLIC_URL`, then the page's own address | Only when email is to use an address other than the instance's own. Every link is absolute, so saving a mail server requires one. |
| DKIM selector | — | The selector the mail server signs with, used only by **Check DNS**. |

A saved password is kept only for the host it was entered for: pointing the
settings at another host asks for it again, so it is never handed to a server
nobody typed it for. The password is sealed with the instance's backup key
(`REET_BACKUP_KEY`, or the generated `backup.key`). An
instance with no usable key refuses to store one.

**Check connection** connects, secures the connection and signs in with what is
on the form, without sending anything. **Send test email** sends to the signed-in
owner through the settings in force and shows the server's own replies, line by
line. **Check DNS** looks up SPF and DMARC for the from domain, DKIM for the
selector named, and BIMI at `default._bimi.<from-domain>`.

Saves, checks, test sends and resends are written to the instance activity log.
The outbox on the same page lists each message's state, tries, next attempt and
the server's last word; a delivered or given-up message can be sent again, but
one whose sign-in link has been spent cannot.

### No mail server

With no server saved, reet still works and says so. In that state:

- an administrator's **Reset password** hands over a temporary password
  instead of emailing a link, and the sign-in page offers no self-service
  reset (it points at the admin or `REET_CONTACT_EMAIL`);
- an invitation shows its link to copy and send, and no invitation email is
  queued;
- welcome and password-change messages are **held** in the outbox and nobody
  receives them;
- team notification choices are saved, but team notices are not queued.

`REET_MAIL_OUTBOX_DIR` is for local work: each message is also written there as
`ID.eml` and `ID.html`, and nothing leaves the machine. It is not a way to
deliver email, and it should not be set on a public instance because it writes
every sign-in link to disk in the clear.

## What sends what

| Message | To | Sent when |
| --- | --- | --- |
| Password reset | the address asked for, if it has an enabled account | `/forgot` on the sign-in page, or an administrator's Reset password. The answer is the same whether or not the account exists. The link works once, for 30 minutes (24 hours when an administrator starts it); a newer link replaces an older one. |
| Password changed | the account | A reset link is used, or the password is changed on Account → Password. Every session is signed out. |
| Invitation | the invited address | An administrator creates an invitation on Admin → Users. It still shows the link to copy. The link works once, for seven days. |
| Welcome | the new account | A self-serve sign-up, or an invitation is accepted. |
| Team notice | the team's choice | A member joins, leaves or changes role; or a list is shared with the team. Each of those kinds can be turned off. |
| Support ticket | instance administrators, and a team's owner and managers | A ticket is opened or a reply is added. The person's notification choice decides email or centre-only; see [notifications](notifications.md). |

A password reset request is limited per address and per network address. A
reset link, a password change and an invitation are security messages and
carry no unsubscribe link. Team notices are controlled by the team, not by an
individual recipient.

## Deliverability

A receiver trusts mail from a domain that publishes:

- **SPF**, a TXT record on the domain naming the servers allowed to send for
  it, such as `v=spf1 include:<your provider> -all`;
- **DKIM**, a public key at `<selector>._domainkey.<domain>` that the server
  signs each message with;
- **DMARC**, a TXT record at `_dmarc.<domain>`, starting `v=DMARC1; p=none;
  rua=mailto:…` and tightened to `quarantine` or `reject` once its reports
  are clean.

The test email's headers show whether the receiver saw `spf=pass`,
`dkim=pass` and `dmarc=pass`. Without them, resets and invitations land in
spam.

Any provider that accepts SMTP submission works: Postmark, Amazon SES,
Resend, Mailgun, SendGrid, or a server of your own. There is no SDK; the
provider is configuration.

## API additions

- `GET`, `PATCH /api/v2/admin/mail`, and `POST …/check`, `…/test`, with
  `GET …/dns`. Reading the settings is for admins; changing and testing them
  is for the owner.
- `GET /api/v2/admin/mail/outbox` lists the outbox for admins;
  `POST /api/v2/admin/mail/outbox/resend` is the owner's.
- `POST /api/v2/auth/forgot` and `POST /api/v2/auth/reset` are public.
- `GET`, `PATCH /api/v2/groups/{id}/notifications` are for that group's owner
  or managers.
