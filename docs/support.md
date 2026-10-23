# Support tickets

Support is a conversation between the person who opened a ticket and the
people who can answer it. A ticket is either personal or about a team.

- **The requester** opens it from **Support**, sees their own tickets, replies
  and closes one.
- **A team's owner and managers** can read and answer tickets opened for that
  team. Other members of the team cannot see them.
- **Instance owners and administrators** see every ticket in **Admin →
  Support**, can reply, add internal notes, assign a ticket to an
  administrator, and change its status. Internal notes never reach the
  requester.

A personal ticket is visible to the person who opened it and to instance
administrators. A team ticket is visible to its requester, that team's owner
and managers, and instance administrators. Another person's ticket and one
that never existed answer alike, so the API cannot be used to discover which
tickets exist.

## Statuses

| Status | Means |
| --- | --- |
| **Open** | A requester is waiting for an answer. A requester's reply always puts it here. |
| **Waiting** | Somebody has answered, and the ticket is waiting on the requester. A team or instance reply puts it here. |
| **Closed** | Done. A reply reopens it. Administrators can reopen or close it from the queue. |

A reply from a team manager counts as the team answering, so the ticket waits
on the requester. The same reply from an instance administrator also waits on
the requester. Internal notes change neither the status nor the time the
requester sees.

## What a ticket is

A subject of up to 200 characters and messages of up to 20,000 characters.
There are no attachments: paste the relevant text, or describe where to find
it. A support message is prose, not a file upload.

## Notifications

Opening a ticket tells the instance administrators and the team's owner and
managers (except whoever opened it). A requester reply tells the same people.
A team manager's reply tells the requester. An administrator's reply tells the
requester and the team's managers. Internal notes are never emailed.

Every one of those notices is also a row in the notification centre. Each
person chooses whether they get the email now, keep it in the centre only, or
turn that kind off, under **Notifications**. An administrator's assignment of
a ticket is a staff notification too. The message carries a link to the
ticket, not the conversation itself. With no mail server, tickets still work
and the centre still fills; nobody is emailed.

## API additions

- `GET`, `POST /api/v2/tickets`
- `GET /api/v2/tickets/{id}`
- `POST /api/v2/tickets/{id}/reply`
- `POST /api/v2/tickets/{id}/close`
- `GET /api/v2/admin/tickets`, `GET /api/v2/admin/tickets/{id}`
- `POST /api/v2/admin/tickets/{id}/reply`, `…/note`, `…/status`, `…/assign`

Reading and writing tickets is available to read-only accounts too: support
is not a change to a shelf or a shared list.
