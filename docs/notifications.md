# Notifications

The notification centre tells a person what happened without making email the
only record. Every notification is a row in the centre; whether an email also
goes is that person's choice, within the instance owner's defaults.

There are two audiences:

- **People** hear about replies to tickets they opened or teams they manage,
  and about their own account security.
- **Staff** — the owner and instance administrators — hear about new support
  tickets, requester replies, tickets assigned to them, and new accounts.

The catalogue is in `internal/notify`; the rows and preferences are in
`internal/store/store_notify.go`.

## Who chooses what

1. **The instance owner** sets the platform defaults under **Admin →
   Notifications**: whether a kind is on, whether its email goes now or it
   waits in the centre, and — for staff kinds — whether the owner and
   administrators receive it by default.
2. **Each person** chooses their own delivery under **Notifications →
   What you hear about**. Their choice wins over the platform default.

A security notice (`Password changed`) is always on and always immediate; it
cannot be turned off.

Each kind is either:

- **Email me now** — a row in the centre and an email through the instance's
  mail server.
- **Keep it in the centre only** — a row with no email.
- **Off** — no row and no email.

There is no digest in this build. A kind that is on sends immediately; the
centre keeps it either way. Team membership and list-sharing notices keep
their own choices under **Teams & organisations → Notifications**.

## What is recorded

| Kind | Who | What |
| --- | --- | --- |
| New support ticket | Staff | A person or team opened a ticket. |
| Requester replied | Staff | A requester added to a ticket. |
| Ticket assigned to you | Staff | An administrator was given a ticket. |
| New account | Staff | A sign-up or an accepted invitation created an account. |
| New team ticket | People | A member opened a ticket for a team you manage. |
| Support ticket reply | People | Somebody replied to a ticket you opened or manage. |
| Password changed | People | Your password was changed or reset. Always on. |

A notification is keyed on the event: reporting the same occurrence twice
tells nobody twice. An email is a keyed row in the outbox, so a mail server
outage costs a retry, not the notification.

## The centre

The account menu shows an unread count on your avatar. **Notifications**, in
that menu, lists them newest first, marks one read when it is opened, and has
**Mark all read**. A staff ticket notification opens the ticket in the admin queue; a person's
ticket notification opens their ticket page. Read rows are removed after 90
days; unread rows are kept.

## API additions

- `GET /api/v2/notifications` and `GET /api/v2/notifications/count`
- `POST /api/v2/notifications/read`
- `GET`, `PATCH /api/v2/notifications/preferences`
- `GET`, `PATCH /api/v2/admin/notifications/defaults` (owner)
