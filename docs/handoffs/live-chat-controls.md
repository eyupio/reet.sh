# Live Gaffer controls handoff

Updated 4 November 2026. Feature branch: `feat/live-chat-controls`.

## Implementation

- Negotiate `acp-live-controls-v1`. Older connectors keep their existing wire format, idle model change and browser queue.
- Read model and `thought_level` selectors from the active session; exclude permission controls. Apply model and effort while
  a turn runs without holding the turn lock during RPC. Cancel remains responsive. Require actual read-back and clear unknown
  settings on ambiguous acknowledgements. Persist confirmed model/effort together in the controller and connector restart registry.
- Use explicitly advertised native steering with the idle `promptRequired` opt-in. Distinguish injected, unconsumed, rejected
  and uncertain outcomes. Do not automatically resend an uncertain mutation or steering request.
- Store at most ten active queue items per account/session. Stable client IDs survive lost enqueue replies; completed IDs retain
  payload-free receipts. Revision claims prevent duplicate delivery and stale-tab edits. Archiving clears queued payloads.
- A controller worker sends without a browser. Ordinary delivery waits for the machine's actual turn, approvals and compaction.
  Send now injects when supported. Stop and send records cancellation intent and waits for actual idle; an early cancelled SSE
  event cannot start the next turn. A controller restart during a claimed delivery marks it uncertain, requiring explicit review.
- Poll runtime selectors and shared queue in Gaffer. Show effort only when advertised; allow live model switches, inline queue
  edits, removal and explicit recovery. Poll requests are retired when the pane closes or capability changes.

## Validation

Focused race-enabled tests pass for runtime control transport, live setting acknowledgement, responsive cancellation, saved effort
startup and restart options, selector invalidation, compaction/approval steering refusal, shared queue claims, idempotency,
ownership, bounded capacity, stale edits, uncertain-delivery restart recovery, stop timing and all four steering outcomes.
API tests cover negotiated controls, queue operations and stale revisions. Web type checking passes.

Remaining release checks: final web build/check, desktop/phone light/dark browser tests, full Go vet and race suite, PR review,
current-main conflict resolution and final-head CI. Keep this section current before merging.

## Recovery and limits

Check the conversation before retrying an uncertain message; the Retry button requires acknowledging possible duplication.
Failures remain editable or removable. If Stop ends the session, reconnect first and retry the retained message. Expired images
must be uploaded again. Queue workers assume the existing single-controller ownership model; this adds no multi-controller lease.
The connector must be upgraded to expose the new controls. Native steering is available only when its running adapter explicitly
advertises support. These are user-directed settings; no autonomous model-routing policy is enabled.

Rollback can hide `acpLiveControls` or revert the feature. The additive queue table is harmless to older binaries, but their UI
does not process saved messages. Retain the database until any queued or uncertain messages have been reconciled.
