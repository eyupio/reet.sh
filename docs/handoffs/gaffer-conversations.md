# Gaffer conversation and session-access contract

5 November 2026. GM0 contract and GM2A implementation record. Read alongside
[the active companion roadmap](../work-orchestration-handoff.md#active-direction-gaffer-session-companion-mvp-5-november-2026).
This record uses the current broker, queue and account contracts; it does not
authorise new adapters or turn an existing shelf MCP credential into session access.

## Identities and authority

| Object | Binding and effective access |
| --- | --- |
| General Gaffer conversation | One per account, `sessionId` empty; private conversation history only |
| Session Gaffer conversation | One per account and immutable structured-session ID; independent of the worker transcript and other companions |
| Provider choice | Owned, enabled provider; first creation selects the explicit provider or current default and pins its ID; changing the account default does not redirect an existing conversation |
| Model choice | Optional conversation override; otherwise use that provider's current configured model; every turn records the actual requested model/provider name |
| Worker access | Same account, unarchived structured session and current account write access; attaching a Gaffer conversation grants no worker reads or writes |
| Future follow goal | Explicit user delegation, owner, immutable session, goal revision, provider/model choice, read/message grants and bounded follow-up policy |
| Future MCP connection | Separate explicit grants for session read and message operations, bound to owner and selected session; existing shelf scopes are unchanged |

The worker does coding, commands, PR creation, check fixes and conflict resolution.
Gaffer may eventually read permitted evidence and request the next action through
the existing queue. It cannot answer approval cards, enable YOLO, broaden scope,
run a shell or acquire a second GitHub-write path. Interrupting the worker remains
an explicit separate action. Generic chat, closing a panel and receiving a quiet
worker event do not create a follow goal or prove it is complete.

## Verified worker/message boundary for GM3 and GM4

The controller's existing source of truth is
`internal/terminalbroker/chat_controls.go`: `ChatControl` checks an owned
structured session, a live peer and matching connection generation, the reported
`ChatAgent(tool)` and negotiated `ACPLiveControls`, then reads actual controls.
`chat_queue.go` adds archive and running-state checks. A stored session's tool name
or an installed binary alone is not eligibility. Preserve adapter feature/version
and experiment gates; do not enable a connector or automate terminal keystrokes.

A follow operation must also require the controller's terminal access to be on,
a tool-capable Gaffer provider/model and an explicit goal/grant. Recheck effective
account, session, provider policy, grant and goal revision before context assembly
and immediately before enqueue. Revocation/pause invalidates pending proposals.
A missing/offline/stale machine, unconfirmed mode, waiting human approval, ended
session, changed ownership, revoked grant or unsupported backend becomes a visible
blocked state, with the existing access error retained as its reason.

Reuse `Broker.EnqueueChat` with its stable account/session/message ID and default
`wait` intent. Store a Gaffer instruction receipt mapping that queue ID to the
conversation, goal revision and originating Gaffer turn before enqueue. An
admitted queue item is not confirmed delivery or goal completion. The existing
queue provides `queued`, `sending`, `sent`, `failed`, `uncertain` and removal
outcomes; `RecoverTerminalQueueFor` turns foreign-owner in-flight delivery into
`uncertain` after restart. Reconcile the same receipt rather than submit a new
message. Never automatically retry `sending`/`uncertain`, and leave an actionable
review path for the user. If a future explicit interrupt is selected, use the
existing stop-and-send contract and its persisted stop intent.

## Context disclosure for the next integration slice

GM2A reads **no** worker events, projects, machine inventory, repository status or
GitHub data. Provider context switches are stored future policy. Explicitly typed
Gaffer messages go to the provider the user selected, even with those switches off.

For future tools, effective permission is the intersection of current account
access, selected-session grant, active goal/connection scope where applicable,
and current provider data policy. No cross-session/global transcript search.
Use bounded existing reads from `store_chat_events.go` and broker subscriptions;
retention is at most 1,000 events / 256 KiB, not the full native transcript. Record
source session, observed sequence/time and truncation. Missing/stale/trimmed data
must remain distinguishable from verified current evidence.

Status/activity needs `allowStatus`; worker conversations/results need
`allowSessions`; code, diff and raw log/tool payloads additionally need `allowCode`.
These labels cannot be implemented by blindly forwarding an event's whole JSON:
tool fields and prose can contain code, credentials or instructions. Specify
allowlisted projections and disclosure in the next context-read slice, and do
not expose that tool before its field-level tests. Treat all worker/tool text as
untrusted evidence, never as approval or an instruction to expand Gaffer's tools.

## GM2A: implemented durable conversation runtime

`internal/store/store_gaffer_chat.go`, `internal/gaffer/conversations.go` and
`internal/api/gaffer_chat.go` provide browser-account-only conversation operations.
They reuse canonical-origin checks for changes and exclude API/MCP credentials,
viewers and anonymous mode. Accounts own their own histories; general/session
bindings cannot be changed by configuration updates. An archived or unavailable
session cannot be read or used for another turn. An account-owned metadata index
still permits explicit cleanup of companions whose worker history is unavailable.
Gaffer history is separate: archiving worker history does not silently delete a
user's Gaffer messages. Removing a conversation or account cascades to its turns.

A turn records the user message, request ID/revision, requested provider/model,
partial/final answer, usage (including unknown), output-cut flag and outcome.
Persist admission before the first provider request. The same ID, original body
and original revision returns its existing outcome without another provider call;
a changed body/revision under that ID is refused. New messages require the viewed
conversation revision. Only one turn runs per conversation. Provider/model changes
are refused during a turn and invalidate stale revisions. Historical requested
model identity does not certify the provider's internal model implementation.

The controller owns the provider call, with a three-minute deadline, two concurrent
turns per account and four overall, and twelve admitted/attempted new turns per
account/minute. Provider key work shares the existing two-slot bound. Model text
is saved at most every 200 ms of received chunks or each 4 KiB; the final write
preserves partial output. Cancellation, shutdown, provider changes/deletion and
current account/session access are checked; a one-second watcher also handles a
silent provider. Revocation cannot recall context already sent upstream.

Closing an SSE observer does not cancel the answer. Reconnect to the same turn's
read endpoint; it starts with the latest persisted state, never another model
call. SSE rechecks browser sign-in/role and owned session access, has heartbeat
and write deadlines, and stops at a terminal outcome. Explicit cancellation is
separate and targets the exact turn. Cancellation finalises under the same lock
as completion. Controller shutdown cancels and joins admitted workers. Startup
marks persisted `running` turns `interrupted`, retains their saved partial answer
and never calls the provider. This assumes one controller owns a database.

Limits: 100 conversations/account, 100 turns/conversation, 8,000-byte user message,
64 KiB answer and 512 KiB conversation text. Admission reserves the maximum answer
space. Stop and require explicit removal/new conversation at the storage limit;
do not silently discard request receipts and make an old request eligible again.
Only completed message pairs plus the current prompt enter model context, capped
at 64 KiB. The system prompt discloses omitted/trimmed history and Gaffer's current
lack of tools/live evidence. Empty answers, unsolicited tool calls and streams
without a completion marker are failures, with partial text retained.

The UI is not connected to these routes yet. No tools, scoped MCP session writes,
background goals, autonomous polling or PR actions are enabled. There is no
automatic provider retry/fallback. No new dependency or paid provider was used.

## Browser API

All paths are under `/api/v2/gaffer/conversations` and require a browser account
with write access; mutating methods also require the canonical origin. Errors use
the existing JSON envelope. Message bodies and keys never enter Activity.

| Method/path | Contract |
| --- | --- |
| `GET /?sessionId=…` | The selected scope; omit sessionId for general; 404 until created |
| `GET /?all=true` | Owned conversation metadata, including unavailable worker scopes, for cleanup |
| `POST /` | `{sessionId?,providerId?,model?}`; create/find scope, no implicit turn or reconfiguration |
| `GET /{id}` | Conversation metadata and its bounded turns |
| `PATCH /{id}` | `{revision,sessionId,providerId,model?}`; retain original scope; explicit provider/model change applies to future calls with this conversation's history |
| `DELETE /{id}` | `{revision}`; remove conversation/turns and cancel its active call |
| `POST /{id}/turns` | `{id,revision,text}`; 32 lowercase hex ID; return 202 with durable receipt; never auto-retry with a new ID |
| `GET /{id}/turns/{turnId}/events` | SSE `turn` events containing the latest persisted turn; terminal event closes the stream |
| `POST /{id}/turns/{turnId}/cancel` | `{}`; acknowledge cancellation request; observe the receipt for its final outcome |

## Verification and next task

Fake-backed store/domain/HTTP tests cover durable IDs, altered-request rejection,
concurrent admission, retention, actual database reopen, partial output, shutdown,
explicit cancellation, provider pause/role change, history isolation, unsolicited
tools, incomplete streams, disconnect/reconnect and audit privacy. Record exact
final checks and PR state in the main companion handoff when publishing.

Next: add the floating general/session UI over these routes, including scope,
provider/model identity, partial/error/restart states, explicit cancellation and
removal. Then implement the bounded context projections and scoped MCP read/message
grants above. Durable follow goals remain GM4, after the queue/grant integration
is verified. Keep the existing worker composer and approval flow authoritative.
