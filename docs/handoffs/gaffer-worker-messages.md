# Gaffer worker messages — GM3B

6 November 2026. Implements the next step after merged #608 (`95eb8822`), based
on main after #609. Read alongside `gaffer-conversations.md` and the companion
roadmap. Gaffer relays instructions; the session's existing model does the work.

## Delivery contract

- Separate, off-by-default messaging grants for each account/conversation or MCP
  client and exact structured session. Shelf scopes and read grants imply no write
  authority. MCP also needs management access. Grants are revisioned; Gaffer pins
  provider ID/revision/resolved model and requires session data policy.
- Ordinary Gaffer sends remain read-only. A user may enable one worker message for
  the next Gaffer request. Only that request advertises the one narrowly scoped
  tool; its session is fixed by the conversation, never chosen by the model.
- Buffer the complete provider response and validate completion, tool name,
  arguments, bounds and access before executing. At most one instruction and one
  provider continuation per request. No automatic tool/model retries.
- Store the instruction receipt and existing queue item atomically before delivery.
  Bind stable request ID, subject/session, originating turn, text digest and grant
  revision. A fresh receipt cannot adopt an existing human queue entry. An altered
  duplicate conflicts; a matching retry only reads its receipt.
- Reuse the worker queue with `wait` intent. Recheck ownership, account role,
  session/archive, client switches/scope/expiry, grant and Gaffer provider policy
  at admission, queue claim and before handoff. Revocation removes unclaimed
  instructions; already claimed or delivered work cannot be recalled.
- Receipts distinguish queued, sending, sent, blocked, uncertain and removed.
  Sent means worker acceptance, not completed work or a fulfilled goal. Restart
  converts in-flight delivery to uncertain; never resubmit automatically. Reads
  reconcile receipts without starting a new Gaffer/model request.
- An explicit signed-in browser action can use the existing stop-and-send intent
  on a queued receipt. The model/MCP tool cannot interrupt, answer approval cards,
  change YOLO, broaden scope or run shell/GitHub actions. Managed queue items can
  be removed through the worker queue, but are immutable and cannot be silently
  edited/retried/promoted through generic queue controls.
- Stored instructions are bounded (8,000 UTF-8 bytes, 500 retained receipts per
  account; at most 100 enabled scopes per client). Removing a conversation/revoking a client or archiving a session makes
  obsolete receipts eligible for cleanup; retain live receipts/idempotency keys.
  Audit records contain IDs/state only, never instruction text or tool arguments.

## UI and MCP

Keep the narrative-led widget from #608. Worker permission, delivery receipts and
explicit interruption live in a secondary dialog; enabling the next request is
visible and cancellable before Send. Closing the panel does not create a goal or
cancel a committed worker message. Cancellation of a Gaffer answer is separate
from stopping the worker or revoking queued messages.

The shared authorised operation backs Gaffer and hosted/browser/scoped-stdio MCP.
Tools list granted worker scopes, send a next-turn instruction with a stable ID,
and read one receipt. Credentials cannot self-grant. Legacy API-token/anonymous
stdio and read-only MCP connections cannot send instructions. Browser WebMCP
confirms writes under its existing review contract.

## Implementation and verification

The store owns separate grants and atomic receipt/queue admission. The Gaffer
service owns the bounded model tool loop; API/MCP bind the authenticated subject.
The broker reads stored provenance rather than trusting a caller's queue
projection and rechecks controller policy plus stored authority before handoff.
The browser keeps all worker controls in a secondary dialog. Both the conversation
view and the worker-permission receipt list reconcile foreign-owner in-flight
queue states before returning them, even if the worker is offline or the user
stopped its grant. This is a passive read: no worker probe, wake or resubmission.

Fake-backed race tests cover default-off/per-request intent, identity/scope,
altered duplicates, grant revocation, provider/model/policy/role/archive changes,
waiting human input, unsupported/incomplete/multiple tools, failure after commit,
reopen uncertainty and explicit real-broker stop-and-send. Browser tests cover
permission, next-request intent, focus, escaped receipts, interrupt confirmation,
independent MCP/browser grants and responsive light/dark layouts. No new dependency
or real provider key is used. The main handoff and PR record final checks.

## Gate and next step

Use only fake providers and fake structured workers. Prove owner/client/session
separation, default-off and per-request intent, final grant checks, provider/model
changes, queue revocation, idempotency, malformed/incomplete/multiple tools,
partial provider failure after enqueue, uncertain delivery/reopen and explicit
interrupt. Verify generated tool parity, browser layout/focus and the existing
worker queue. Update final checks and PR state in the main handoff.

GM4 remains separate: durable per-session goals, bounded background follow-up,
multiple sessions and restart/offline supervision. No polling loop or autonomous
Gaffer follow-up is authorised merely by enabling messaging.
