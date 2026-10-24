# Terminal history archival contract

`POST /api/v2/terminal/sessions/{id}/archive` accepts `{}` and returns `{ "ok": true }`.
The caller must have an active account browser session with a write role and the
canonical Origin. API/MCP tokens, viewers, other accounts and disabled users fail.
This controller-only operation works when remote terminal execution is disabled.
Create, rename, end, attachments and socket authorization retain their current gates.

Only controller-confirmed ended/exited/lost rows can be archived. Creating, running
and end-requested rows return 409, including offline hosts. A transaction locks the
owned session with a write before reading its state; this serializes against
observations/end requests and across database handles. Missing/foreign rows retain
the existing indistinguishable 403 policy. Malformed IDs/bodies return 400.

A `terminal_session_archives` tombstone records session ID, owner, connector and
archive time, with a composite foreign key to the existing session identity.
Repeated requests preserve the original timestamp. Normal session lists exclude
tombstones. Direct broker/receipt/reconciliation reads remain unchanged so create
and end retries always resolve the original record, never a new shell. Operation
receipts, states, quota accounting and host files/processes are unchanged. No host
request is sent. Archival is persistent across controller restart and does not need
an online connector. There is no undo or bulk archive endpoint in this first stage.

The UI offers Remove from history only for closed sessions. Confirmation captures
the immutable ID/title and explains that machine files are kept. Failed requests
leave the entry visible. Successful responses remove only that entry; if the user
has selected a different session while waiting, keep the newer selection attached.
An archived deep link becomes unavailable without starting another shell.

The migration creates only the new child table. Existing sessions remain visible
until explicitly archived. Rolling back to an older binary can show closed records
again because it ignores tombstones; session/receipt identities remain intact.

## Remove after stopping

The existing end endpoint additionally accepts `removeHistory: true`. Omitting it
keeps history. The choice is part of the immutable end-operation parameters;
changing it on a retry with the same request ID returns 409. Pending termination
remains visible. Only an authenticated, generation-fenced ended/exited/lost
observation (or confirmed failed creation) adds the archive tombstone, in the
same transaction as closure. Controller restart preserves this decision. End
still requires the existing execution/account/Origin gates; no automatic end is
introduced by changing preferences.

`terminalRemoveOnClose` in the account profile defaults to false and sets the
checkbox's initial value for new stop confirmations. The user can override it per
session. Saving that profile preference never removes history retroactively or
changes pending end operations.
