# Gaffer conversation and session-access contract

6 November 2026. GM0 contract and GM2A/GM3A/GM3B implementation record. Read alongside
[the active companion roadmap](../work-orchestration-handoff.md#active-direction-gaffer-session-companion-mvp-5-november-2026).
This record uses the current broker, queue and account contracts; it does not
authorise new adapters or turn an existing shelf MCP credential into session access.

## Identities and authority

| Object | Binding and effective access |
| --- | --- |
| General Gaffer conversation | One per account, `sessionId` empty; private conversation history plus explicitly granted retained account counts |
| Session Gaffer conversation | One per account and immutable structured-session ID; independent of the worker transcript and other companions |
| Provider choice | Owned, enabled provider; first creation selects the explicit provider or current default and pins its ID; changing the account default does not redirect an existing conversation |
| Model choice | Optional conversation override; otherwise use that provider's current configured model; every turn records the actual requested model/provider name |
| Worker access | Same account, unarchived structured session and current account write access; attaching a Gaffer conversation grants no worker reads or writes |
| Future follow goal | Explicit user delegation, owner, immutable session, goal revision, provider/model choice, read/message grants and bounded follow-up policy |
| MCP connection | Explicit read grants per owner/client/scope; browser tools have a separate subject; existing shelf scopes are unchanged. Separate GM3B message grants require management access. |

The worker does coding, commands, PR creation, check fixes and conflict resolution.
Gaffer reads explicitly permitted evidence and can relay one requested instruction through
the existing queue. It cannot answer approval cards, enable YOLO, broaden scope,
run a shell or acquire a second GitHub-write path. Interrupting the worker remains
an explicit separate action. Generic chat, closing a panel and receiving a quiet
worker event do not create a follow goal or prove it is complete.

## Reviewed browser session setup

The [widget/setup contract](gaffer-widget-setup.md) supersedes the old multi-dialog
activation instructions. One explicitly reviewed browser operation enables the
selected provider's session/text maxima if needed, binds read/message grants to
only this conversation/session and starts or resumes its bounded goal atomically.
It requires confirmation and current conversation/provider/model/grant/goal
revisions. Failure rolls back every access change. Other subjects, ordinary chat,
MCP access, worker approval/interrupt rules and delivery fences retain their
existing contracts. Advanced separate-grant controls remain available.

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

## GM3A: implemented context disclosure and scoped MCP reads

`internal/store/store_context.go`, `internal/gaffer/context.go` and
`internal/api/gaffer_context.go` implement the shared authorised read operation.
The Gaffer runtime injects a passive JSON snapshot on explicit sends; hosted MCP,
browser WebMCP and scoped stdio forwarding expose the same bounded projection.
GM3A alone enables no provider-requested tool loop, worker message or goal.
GM3B adds the explicitly enabled, bounded instruction loop below.

Grants are durable, off by default and bound to owner, subject kind/ID and immutable
scope (empty for account counts, exact structured-session ID otherwise). Subjects
are a Gaffer conversation, MCP connection or browser tools, never caller-supplied
identity. Shelf OAuth `mcp:read`/`mcp:write` does not imply context permission.
Legacy API-token/anonymous stdio cannot read context. At most 100 active scopes per
subject; edits use grant revision CAS. Removing a conversation removes its grants;
archiving a session clears all of its grants and increments their revisions.

Gaffer additionally binds approval to provider ID, provider revision and resolved
model. Changes require review. Effective access intersects current account role,
owned unarchived structured session, grant and provider policy. Browser management
requires canonical-origin write access; clients cannot self-grant. MCP reads check
active client/master/browser switches, expiry, role, scope and revision before and
after assembly. A read-only shelf connection can read explicitly granted context
for a writer account; viewers cannot read or grant context. Subject visibility is
checked before scope lookup and ungranted session IDs reveal no session existence.

| Category | Projection | Gaffer provider maximum |
| --- | --- | --- |
| `status` | Account machine/unarchived-session counts and last report time, or selected-session canonical state, worker type and update time; no names/paths/text | `allowStatus` |
| `history` | Recent event sequence/time/role, canonical tool kind/state and status; no prose | `allowSessions` |
| `text` | User/agent text, selected-session title and opening-goal excerpt; requires `history` | `allowSessions` and `allowCode` |

SQL projects allowlisted fields before loading events. Reasoning, approval prompts,
raw tool arguments/output, files and diffs are never forwarded. User/agent prose
may contain code or credentials, so it needs explicit code/log permission. Best-
effort masking covers recognised keys, assignments, URL credentials and PEM blocks;
it does not promise secret detection. Text is UTF-8 clipped and rendered escaped.

A versioned snapshot includes `source: controller_retained_not_live`, scope/read
and source-update timestamps, retention bounds, unavailable and truncation markers.
Limits are 16 KiB JSON, 20 projected events from the latest 40 retained rows, 2 KiB
per user/agent event. Existing retention remains at most 1,000 events / 256 KiB;
this is not the native transcript. No machine/GitHub request is made. Omitted or
old evidence never means checks passed or the goal is complete. MCP reads have a
separate 60/account/minute limiter.

The browser requires a preview before enabling categories and explains continuing
sharing. Changing Gaffer sharing bumps its conversation revision, cancels its
current answer and leaves the worker alone. The runtime checks account/session,
provider policy and grant revision at assembly, just before provider egress, while
streaming and at finalisation. Already sent context cannot be recalled. A turn
stores only a versioned category/revision/time/sequence/digest receipt, not a second
raw context copy. The context consumes the existing 64 KiB model-history budget.
Completed message pairs derived from categories no longer granted (or unknown
context versions) are excluded from later model history but remain in saved UI.

Existing database backups include grants and receipts. A restored unavailable
provider/key remains blocked under existing recovery rules; revoked restored MCP
connections cannot use their old grants. Restart interrupts in-flight answers
without replay, as before. No grants start polling or background provider calls.

Browser routes: GET/PUT `.../context` and POST `.../context/preview` under
`/api/v2/gaffer/conversations/{id}`, `/api/v2/mcp/connections/{id}` and
`/api/v2/mcp/browser`. Gaffer scope comes from the conversation; MCP browser
management uses `?sessionId=...`. MCP tools are `reet_list_context_access` and
`reet_get_context`; a caller cannot choose a different subject. Context is
read-only and does not broaden the existing worker approval/queue authority.

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
lack of actions/live evidence; GM3A adds explicitly granted passive snapshots. Empty answers, unsolicited tool calls and streams
without a completion marker are failures, with partial text retained.

GM2B connects the floating general/session UI to these routes; see the newer
record in the orchestration handoff. No model-requested tools, scoped MCP session writes,
background goals, autonomous polling or PR actions are enabled. There is no
automatic provider retry/fallback. No new dependency or paid provider was used.

## GM3B: explicit worker messages

The [worker-message contract](gaffer-worker-messages.md) records the implemented
operation, separate grants and atomic queue receipts. Ordinary chat remains
read-only. Only a user-enabled request (`relay: true`) advertises `message_worker`,
with one complete validated instruction and at most one provider continuation.
The fixed conversation supplies the destination; the model cannot choose a session
or interrupt. A continuation failure retains the committed receipt.

Browser grants bind provider/revision/resolved model and the exact owned structured
session; MCP and browser tools have independent client/session grants and require
management access. Current authority is rechecked at admission, queue claim and
before handoff. Revocation removes queued items. A controller restart reconciles
in-flight delivery to uncertain without replay. Acceptance does not prove task or
goal completion. Explicit browser-only stop-and-send uses a confirmed queued
receipt; managed queue items cannot use generic edit/retry/promote controls.

Worker browser settings use `GET/PUT /{id}/worker`; the explicit confirmed
interrupt is `POST /{id}/worker/messages/{messageId}/interrupt` with
`{revision,confirm:true}`. MCP management uses equivalent paths under
`/api/v2/mcp/connections/{id}` and `/api/v2/mcp/browser`, with a selected `sessionId`.
Tools are `reet_list_worker_access`, `reet_send_worker_message` and
`reet_get_worker_message`. Legacy stdio cannot use them. Receipt reads do not wake
the queue or request a provider answer. No durable follow goal is created.

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
| `POST /{id}/turns` | `{id,revision,text,relay?}`; relay defaults false; 32 lowercase hex ID; return 202 with durable receipt; never auto-retry with a new ID |
| `GET /{id}/turns/{turnId}/events` | SSE `turn` events containing the latest persisted turn; terminal event closes the stream |
| `POST /{id}/turns/{turnId}/cancel` | `{}`; acknowledge cancellation request; observe the receipt for its final outcome |

## Verification and next task

Fake-backed store/domain/HTTP tests cover durable IDs, altered-request rejection,
concurrent admission, retention, actual database reopen, partial output, shutdown,
explicit cancellation, provider pause/role change, history isolation, unsolicited
tools, incomplete streams, disconnect/reconnect and audit privacy. Record exact
final checks and PR state in the main companion handoff when publishing.

GM2B now supplies the floating UI with scope, independent model identity,
partial/error/restart states, explicit cancellation/removal and receipt recovery.
GM3A supplies the bounded context projections and scoped MCP reads above.
GM3B supplies explicit worker-message grants and durable queue receipts. Durable
follow goals remain GM4, after the queue/grant integration is verified. Keep the existing worker composer and approval flow authoritative.
