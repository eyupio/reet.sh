# Gaffer and the Eli reuse boundary

Inspected 1 November 2026 against reet main `9b8bc1743282fa69fc086f1015720a31a355eab6`
and Zoomies `b4c67a1413c8274334d18225df4053959861fbc0`, the handoff's pinned source.
This is a bounded contribution to KZ5A. KZ0 and KZ5A remain incomplete. At the
original inspection, full access, MCP discovery measurements and coordinator
configuration decisions were outstanding; the update below reconciles the decisions.

Updated direction, 5 November 2026: the
[active session-companion handoff](../work-orchestration-handoff.md#active-direction-gaffer-session-companion-mvp-5-november-2026)
records the user's now-agreed placement, provider selection, context and offline
coordination choices, plus a newer local source comparison. The dated inspection
below remains evidence for the original probe. GM0 resolves the remaining access,
delivery, retention and recovery contract; no production provider runtime is
enabled by this documentation update.

Implementation follow-up: the active handoff's GM1A entry now records HTTP
adapters and local protocol tests under `internal/elicore/provider/`, adapted
from the newer pinned source with provenance in `internal/elicore/NOTICE.md`.
They have no production caller yet. Provider ownership/settings, allowed network
and context access, conversation persistence and offline coordination still need
their recorded implementation gates.

Gaffer is reet's native browser agent control interface. Eli AI Chat provides the
reusable provider foundation. OpenCode TUI remains OpenCode's original terminal
interface. The binary's CLI TUI retains its existing name and behaviour.

## Reuse decisions

| Inspected Zoomies seam | Decision for reet | Reason and remaining work |
| --- | --- | --- |
| `internal/assistant/provider.go` | Adapt as `internal/elicore/provider.go` | Standard-library-only provider, stream, tool-call, usage and cancellation types. No runtime caller yet. |
| `internal/assistant/contract.go`, `fake.go` | Adapt into test-only conformance probe | Retain upstream contract assertions; remove Zoomies demo fleet assumptions. Fake reads require no credentials or provider spend. |
| `internal/controller/assistant_chat.go`, `assistant_tools.go` | Adapt orchestration patterns later | Existing owner/provider selection, redaction, bounded rounds and explicit tool allowlist must bind to reet's access model. No direct import of the Zoomies controller or toolbox. |
| `internal/api/handlers_assistant_conversations.go`, `internal/store/queries_assistant_conversations.go` | Keep account/store wiring product-local | Adapt ownership, bounds and redaction for GM session-bound conversations and goals; later work records retain KZ5/KZ6's gates. Read existing retained worker events separately rather than importing ACP transcripts through the conversation-save route. |
| `web/src/lib/assistant/conversation.svelte.ts`, `EliWidget.svelte`, Markdown helpers | Adapt for the general and per-session Gaffer companion | Widget/state imports assume Zoomies routes, provider settings, contexts and design tokens. Retain reet's working ChatPane and reducer alongside the separate companion conversation. Offline continuity needs server-owned conversation and goal records. |
| `internal/controller/eli_repairs.go` | Compare durable-attempt patterns | Repair opt-in, budgets and deduplication inform KZ5. They do not replace reet's request IDs, session supervisor, trust gates or work acceptance. |

Both repositories carry GNU AGPL version 3. The small adapted files retain
provenance and modification details in [the source notice](../../internal/elicore/NOTICE.md).
No provider SDK or new dependency is added. Future copied files need the same
source/notice review; this review does not establish licences for unrelated assets.

## Coordinator and worker boundary

The optional future coordinator uses controller-owned API-provider credentials,
with explicit per-provider evidence access. Its stream can propose work; it does
not grant execution authority. Machine workers continue through `internal/acpagent`,
`internal/terminalagent/chat.go`, the broker, existing API/SSE and the browser
chat reducer. Worker credentials stay on the machine. Existing account ownership,
ACP capability negotiation, approval cards, model selection, trust, YOLO and
agent shutdown contracts continue to apply. Provider cancellation ends a model
request; worker cancellation follows the existing acknowledged lifecycle.

The spike runs the reused conformance suite against a disposable fake. It also
round-trips one explicitly offered synthetic read through tool-call and tool-result
messages, then cancels a stream. This proves the local contract can be reused;
it does not prove a real provider, authenticated read route or dispatcher works.
The fake is test-only and no Gaffer session is routed to it.

## Remaining design work after the 5 November decisions

- Placement is agreed: general chat outside sessions and a separate Gaffer
  conversation for each session, with explicit offline goals. The Gaffer default
  provider/model has a per-session override independent of the working model.
- GM0 must specify account-owned provider secrets, per-provider permitted context,
  scoped MCP session access, bounded retention and usage, backup/export and the
  distinction between foreground cancellation and active offline goals.
- Map stable Gaffer instruction IDs to existing durable queue receipts and
  preserve uncertain outcomes, worker approvals and verified capability gates.
- Retain Zoomies' actual provider limitations: subscription CLI backends have no
  fleet toolbox, and model discovery/protocol support varies by backend/model.
- Later KZ work still needs the versioned plan proposal schema, work identity and
  approved execution transitions. KZ attempt dispatch retains KZ7/KZ9A's gates;
  the MVP messages existing sessions through their existing verified controls.
- Narrow adaptation versus a jointly versioned shared core after real adapters
  and both products demonstrate a stable common contract.

KZ6 through KZ12 retain their original dependencies and acceptance criteria.
No adapter is enabled, work migration introduced, automatic routing started or
unfinished handoff stage marked complete by the branding or this probe.
