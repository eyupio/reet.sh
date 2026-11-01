# Structured chats across agents

Status: implementation plan, 31 October 2026. No additional agent adapter is
enabled by this document. OpenCode remains the only structured chat integration.

The goal is one reet conversation view for supported agents, with streamed
messages, tool details, reviewed approvals, agent questions and honest recovery
states. This is the browser's Gaffer, separate from the binary's terminal menu.
Terminal sessions remain available for agents without a verified chat adapter.

## Reuse the existing stack

Keep `internal/acpagent` as the ACP client, `internal/terminalagent/chat.go` as
the connector supervisor, `internal/terminalbroker/chat.go` as the relay and
bounded memory ring, and the existing API/SSE routes and browser reducer/view.
Agent launch and compatibility belong in profiles, not separate chat services.
There is no prerequisite to replace SSE, add a new service or persist controller
transcripts. Existing session/account ownership checks remain mandatory.

The current OpenCode integration and its seven conditions are documented in
[structured sessions](structured-sessions.md). Preserve those conditions when
generalising the client; additional agents need their own measured contract.

## SC1 — Per-agent capability and identity contract

First implementation slice. Add negotiated descriptors for structured chat
availability per tool, adapter identity/version and supported history/inputs.
Distinguish absent, unverified, incompatible and usable adapters. A machine-wide
`acp` boolean must not imply that every installed terminal agent supports chat.

Keep older peers OpenCode-only. New fields and message kinds must be negotiated
before sending them to existing strict decoders; missing descriptors must not
enable new tools. Descriptors report observation, not proof that a paid model
turn has succeeded.

Audit all start, prompt, resume and list gates, not just the launch dialog.
Replace the OpenCode-specific native session ID assumption with adapter-aware,
bounded opaque identifiers. A native ID never authorises access on its own:
continuation must retain the account, machine, tool, folder and closed-session
binding. Changing agents creates a distinct conversation; do not implicitly send
one agent's transcript to another.

Primary files: `internal/acpagent/spec.go`, `session.go`,
`internal/terminalproto/acp.go`, `protocol.go`,
`internal/terminalagent/chat.go`, `internal/terminalbroker/chat.go`,
`internal/api/terminal_chat.go`, `web/src/lib/StartAgentDialog.svelte`,
`web/src/lib/chat.ts` and `terminal/ChatPane.svelte`. Discover the other launch
and persistence gates before editing; this list is not exhaustive.

Acceptance: protocol round trips and mixed-version fixtures; old machines retain
OpenCode behaviour; unsupported tools cannot start via crafted requests; native
IDs and history cannot cross tool/account/machine/folder boundaries. Existing
OpenCode launch, approval and history regression tests continue to pass.

## SC2 — Verify and integrate the first additional adapter

Candidate: Claude Code through a separately installed ACP adapter. Also probe
Codex through its ACP adapter before fixing the integration order; choose the
one whose observed callback and permission contract fits the client first.
Do not assume the terminal CLI itself accepts ACP arguments.

For each candidate, record an exact adapter package/version and integrity,
underlying CLI requirements, executable resolution and protocol identity. Use
temporary homes/projects and a local stub provider where feasible. Capture:

- `initialize`, protocol/name/version, advertised features and auth requirements;
- new/load IDs, replay ordering and explicit history support/refusal;
- tool event and command/diff shapes, allow-once and reject behaviour;
- question requests, delegated filesystem/terminal callbacks and subagent behaviour;
- repository startup hooks/plugins, inherited environment and credential needs;
- model selection, silent-provider errors, cancellation, timeouts and child exit.

Do not advertise delegated filesystem, terminal or elicitation support until
the corresponding handlers exist and are tested. If a required callback cannot
be implemented in this slice, keep that adapter unavailable with a clear reason.
Do not answer a required write callback with an OpenCode-specific mirror no-op.

Add the measured launch profile, identity/version checks, narrowly required
environment, trust checks and error handling. Keep launches connector-owned with
fixed argv. Use already-installed compatible binaries; missing adapters receive
an actionable setup requirement, without implicit downloads or `npx` startup.
The installation-maintenance roadmap is not a prerequisite for this slice.

Acceptance: fake-child protocol regressions plus a captured real-binary protocol
probe, including an actual approval/denial and stopped-tool observation where
possible with the stub. Record unavailable provider-backed checks explicitly;
do not label a fake success or a handshake as a working end-to-end integration.

## SC3 — Typed agent questions

Introduce a bounded internal question model independent of raw adapter JSON.
Treat choice-shaped permission options and typed `elicitation/create` forms as
different mechanisms. Preserve original option IDs and distinguish a question
from permission to run a tool. Never map Allow once to an Always allow option.

Start with only the wire shapes the adapter probe establishes. A practical
first subset is bounded text, single select and multi-select; add boolean or
numeric fields only with their validation and tests. Unsupported schema modes,
constraints or oversized forms must return an explicit cancellation/refusal,
not a guessed answer. Keep accept, decline and cancel distinct.

The machine retains the normalized pending schema and validates submitted
fields, types, allowed values, required answers and bounds. The browser's
validation is feedback only. Reject unknown keys, duplicate/unknown options,
stale tokens and answers for a different session or generation. Define limits
for field/option count, label/schema size, answer length and pending requests
before enabling form capability negotiation.

Pending questions use single-use connector-generated tokens, deadlines and
cancellation on turn/session retirement. Invalid input should leave a valid
pending question answerable; it must not accidentally consume its token.
Request-scoped questions during startup bind to the admitted launch/request,
even when no native session ID exists yet. Session-scoped questions bind to the
admitted native session. Browser reconnect never recreates answer authority.

Negotiate new private-protocol fields/messages separately from existing ACP
support. Render accessible cards in the chat and a pending-question summary in
Needs you, with the account/session access checks used by approvals. Keep pending
cards reachable within transcript bounds. If answering from Home would omit
required context, send the person to the complete card instead.

Acceptance: schema/answer validation; fabricated, stale and duplicate answers;
concurrent tabs; pre-session requests; timeout/cancel/disconnect; old-peer
negotiation; replayed resolved cards; keyboard and phone-width browser fixtures.
Do not advertise elicitation.form in a partial integration before these gates.

### SC3 progress: machine core (31 October 2026, local)

Done in `internal/acpagent`: a normalized `Question` model (text, single select, multi
select), a strict `elicitation/create` parser that refuses the whole form for anything
outside that subset (URL mode, numbers, booleans, patterns, formats, defaults, unknown
keywords, oversized or non-plain text), an authoritative answer validator (`Question.Validate`,
`DecodeAnswers` rejecting duplicate keys), and session handling with single-use
connector-generated IDs, deadlines, a pending cap, and cancellation on turn cancel, close and
exit. Invalid input leaves the question waiting. Accept, decline and cancel are distinct.
`Options.Questions` is off in every launch, `initialize` still does not advertise
`elicitation.form`, and the connector drops `question*` events, so nothing user-visible
changes. Not done: private-protocol messages and negotiation, the controller answer route with
account checks, the chat card and Needs-you summary, request-scoped (pre-session) questions,
and old-peer/concurrent-tab/phone fixtures. Do not advertise the form capability before those.

### SC3 progress: wire, connector and controller route (31 October 2026, local)

Done: the private-protocol feature `acp-question-v1` (negotiated per connection; a
machine's `acpQuestions` capability and a per-agent `questions` descriptor flag, both
false today); `question`, `question_resolved` and `question_cancelled` events with a
bounded, validated `ACPQuestion` payload that must fit the control frame (the machine
refuses a form that does not); an `acp_question_answer` message (accept, decline or cancel;
answers are one JSON object with no duplicate keys, at most 8 KiB); a `question_invalid`
result with a bounded detail; connector routing to `Session.AnswerQuestion`, with question
events sent only on a connection that negotiated the feature (otherwise the question is
cancelled, never buffered); controller `AnswerQuestionChat` scoped to the caller's own
chat, requiring the machine and agent to offer questions, with only the action audited; the
chat log counting a waiting question like an approval; and
`POST /api/v2/terminal/sessions/{id}/chat/questions/{questionId}` (422 for a rejected answer,
409 when the question is gone or not offered). Still off: `Options.Questions` is false in
every launch, `initialize` does not advertise `elicitation.form`, and no agent descriptor
sets `questions`. Not done: the chat card, the Needs-you summary (the broker keeps no queue
entry for questions yet), request-scoped (pre-session) questions, a dispatcher-level test of
the feature gate, and the concurrent-tab, replay and phone-width browser fixtures.

### SC3 progress: the chat card (31 October 2026)

Done in the browser: the `question` card in the chat view (reducer, draft checks, answer call, accessible controls,
decided summary), covered by `chat.test.mjs` and `chat.e2e.mjs`. Also done: the Needs-you summary (the broker keeps a bounded
queue of open questions and publishes the first, truncated, with a count of the rest; Home and the bell show it with
"Answer in chat"). A connector test refuses a question answer unless the connection negotiated both `acp-v1` and
`acp-question-v1`. A browser fixture covers a concurrent tab answering first (the late answer is refused as gone and the
stream's resolution closes the card) and a replayed resolved card coming back decided. Old-peer fixtures cover an old
peer's capabilities carrying no questions, and a machine or agent offering only half being refused before anything is
sent or stored. A question asked before the chat is ready is cancelled with its own notice and never held or attached
(holding startup questions is deferred). Still not done: the
touch-device and screen-reader checks. Nothing sends a question yet.

## SC4 — Integrate the second adapter

Use SC1's capability contract and the existing view with another independently
verified profile. Do not copy the first adapter's permission, ID, environment,
trust, model or history assumptions. Reuse SC2's probe matrix and SC3's question
controls where the wire contract matches. Gemini and Pi are later candidates,
not scope additions to the first two integrations.

### SC4 progress: Codex gates re-run (1 November 2026)

The three open Codex gates were re-probed (see "Codex gates, re-run" in the probe report). Codex's sandbox is active; approvals appear only in `read-only` mode and only when the
model asks to leave the sandbox, so commands inside the sandbox, including project writes, run without a prompt. `agent` mode never asks, a reject ends the whole turn, an
"allow always" option writes a persistent rule, and `session/cancel` does not stop a running command. Codex stays unregistered: it needs the product decisions listed there
before any wiring, and the earlier note that no approval exists in any mode is corrected.

### SC4 decision: Codex starts in `read-only` and may be switched to `agent` (1 November 2026)

Decided by the owner, against the recommendation to offer `read-only` only. What it commits the Codex integration to, because the probes showed `agent` never asks and a refused
escalation is silent:

- **Default is `read-only`.** It is the only mode that can ask before a command leaves the sandbox. `agent` is something the person chooses for a chat, never the default.
  `agent-full-access` is not offered.
- **The mode is stated in the chat, in words that say what it means.** `read-only`: "Codex runs commands inside its sandbox and asks before one leaves it." `agent`: "Codex decides
  for itself what needs your approval. You may not be asked." The mode is visible without opening a menu, and it is shown again if it changes.
- **A mode switch is reported only after the adapter acknowledges it** (the existing rule for the model), and the choice is only offered when the machine's descriptor lists the mode.
- **No "allow always".** The adapter's `accept_execpolicy_amendment` option writes a persistent rule outside the chat; it is not offered. Allow once and reject only.
- **A reject ends the turn,** so the card says so before it is pressed.
- **Stop retires the command,** not only the turn, because `session/cancel` leaves a running command alive. This reuses the process-group retirement already in `Close`.
- **A silent sandbox denial or an `agent`-mode refusal cannot be seen in the events the adapter sends today.** That gap is accepted for `agent`, and is why the mode is labelled as above. If
  a later adapter version reports them, they should be shown.

Still to build, in this order, each behind the same opt-in as Claude until its checks are recorded: a Codex profile and descriptor (dark), the mode control and wording in the browser, the
reject and Stop behaviour, then a real-login check. None of it is built yet.

### SC4 progress: Codex registered, dark (1 November 2026)

`acpagent.CodexCandidate` exists and is reachable only where a machine sets `REET_EXPERIMENTAL_CODEX_CHAT=1`. It follows the decision above: `Spec.StartMode` moves a session
that starts in `agent` (Codex's default) to `read-only`, using `session/set_config_option`, and the chat is refused unless the adapter's reply reports the new mode. The
descriptor reports approvals only. The browser does not offer Codex yet (`CHAT_ADAPTERS` is unchanged), so nothing is visible to anyone.

Probed afterwards (`scripts/probe-codex-start.py`, probe report "Codex start, probed"): the agent name is right and the `set_config_option` mode switch works and confirms
`read-only`. Authentication is not settled: `session/new` answers "Authentication required" until `authenticate` is called, and the connector replaces `HOME`, so a login under the
real home is not visible. A Codex chat cannot start until reet has a decided way to authenticate. That is the next decision, before the browser mode control.

Decided by the owner: use the person's own ChatGPT login, and never sign anyone in from a chat. reet does not call `authenticate`. The adapter is expected to find the login the
person already made with Codex on that machine, so the Codex profile has to let it see that login (the real home, or the person's `CODEX_HOME`) instead of the connector's replaced
`HOME`. To start, reet runs `session/new`; if it fails with "Authentication required" the chat is refused with "Sign in to Codex on that machine first." The adapter's
`_auth/status_update` label ("Not logged in", or the account or key in use) is shown in the chat so the person can see which identity answers, without the account email. Whether a
signed-in adapter's `session/new` succeeds with no `authenticate` call, and what status it sends, is unobserved and needs the real-login run in `docs/device-checks.md` before this is wired.

### Device and credential checks (1 November 2026)

What cannot be closed in CI is written up in `docs/device-checks.md`: the touch and screen-reader checks for typed questions, run on a fixture page served to a real
device (`npm run fixture:questions`), and the logged-out and logged-in Claude runs. `question-card.e2e.mjs` automates what Chromium can show: the accessibility
tree, a keyboard answer, and an emulated 390 px touch phone with every control on screen and at least 40 px tall. The form capability and Claude chats stay off until the
recorded results are in.

### SC4 progress: Claude wired dark (1 November 2026)

Claude is registered behind the descriptor and launch gates but dark. The controller accepts a Claude descriptor only for the
reviewed adapter and only when the machine reports it available; a machine reports it available only when it opts in with
`REET_EXPERIMENTAL_CLAUDE_CHAT=1`. The descriptor reports approvals and the model switch only. Native-session replay, images and
questions stay off. The browser offers a Claude chat only where a machine reports the adapter available (below). Next: run the credential
and missing-credential check on a real machine (script and checklist in the probe report).

### SC4 progress: the browser offers Claude chats (1 November 2026)

The browser reads the reviewed adapter from each machine's descriptor instead of assuming OpenCode: a chat is offered for Claude
only when the machine reports `claude-agent-acp` as available, so it stays hidden everywhere the machine has not opted in. Claude
chats show Claude Code's name, skip OpenCode's plugin trust prompt (Claude ignores a folder's own settings), and send no trust.
Covered by `launchBar.test.mjs` and the existing launch, start-agent and chat browser tests; `chat-claude.e2e.mjs` covers the Claude
start dialog, create body and session screen. Native-session continue, images and typed questions stay off for Claude.

## SC5 — Recovery and capability-driven controls

Differentiate browser reconnect, connector transport reconnect, native process
exit and connector process restart. Adopt a surviving native session without
sending new/load again; load history only into a new process when supported.
Preserve explicit context-loss states rather than presenting a fresh chat as a
successful continuation.

Add graceful cancellation followed by bounded explicit force-stop recovery.
Session/turn generations must prevent stale callbacks from touching a newer
turn. Pending cards retire with their owning generation. Confirm process-group
retirement before starting a replacement. Do not infer successful termination
from elapsed time or UI state alone.

Drive model, effort and mode controls from verified descriptors. An absent
option list and an empty replacement differ; report applied state only after
the agent acknowledges it. Do not offer arbitrary mode changes that weaken the
adapter's permission contract.

Test replay/sequence deduplication, stale connection generations, delayed replies,
cancel ignored by an agent, output before completion, active approvals during
reconnect and native process failure. Preserve the bounded SSE/ring contract.
Durable controller history and detached per-session workers need their own
retention, lifecycle and recovery design; they are not implied by this stage.

### SC5 audit (1 November 2026)

Read against the SC5 list; no behaviour changed. **Already covered, with tests:** process-group retirement on `Close` (confirmed, zombie-aware);
cards (permissions and questions) retire when a turn is cancelled or the session closes; the silence watchdog is joined before the next turn so a late
recovery cannot report against it; the browser drops events at or below its last sequence number; a connection's messages carry its link generation; and
`SetModel` reports the applied model only after the agent acknowledges it.

**Gaps found:**

1. **A cancel the agent ignores leaves the chat busy for about 10 minutes.** `Cancel` shows "cancelled" immediately and sends `session/cancel`, but
   `Working()` stays true until `session/prompt` returns. If the agent never answers, that is the 10 minute turn timeout plus the 30 second request
   allowance; the timeout path sends another cancel and nothing stronger. Meanwhile the person sees "Stopped" and a new message is refused as busy.
   Reproduced with a fake agent that ignores `session/cancel` (not committed: it needs a 30 second wait). There is no force-stop.
2. **"Cancelled" is shown before the agent acknowledges it.** Not harmful on its own, but it is what makes gap 1 confusing. To verify: late output
   from the cancelled turn after the status flips.
3. **Late callbacks from a retired turn.** The turn sequence guards the silence recovery only. To verify: a permission or question request that arrives after
   a cancel or turn end; the agent does not withdraw outstanding requests, so reet closes them, but a request that arrives afterwards may still raise a card.
4. **A connector restart loses every chat.** Chats live in the connector process, so a restart ends them and nothing adopts a surviving native session.
   The browser says the history is not kept; it does not say why. A connector transport reconnect (same process) is covered by the link generation.
5. **Controls are model-only.** There is no effort or mode descriptor, which is intended while the mode is pinned, but the plan's "absent list versus
   empty replacement" rule only applies to the model today.

**Fixed (gap 1):** `Cancel` now arms a force-stop bound to the cancelled turn. If the same turn is still running after `CancelGrace` (15 seconds by
default), the person is told the agent did not stop and the chat is ended, with the process group confirmed gone. A cancel the agent honours, or a later
turn, is never touched. Covered by `TestCancelIgnoredByTheAgentEndsTheChatAfterTheGracePeriod` and `TestForceStopIsBoundToTheCancelledTurn`.

**Fixed (gap 3):** a permission or question request that arrives with no running turn, or after the running turn was cancelled, belongs to no turn the
person is waiting on. It used to raise an approval card and show an idle chat as "waiting". It is now answered as cancelled and raises nothing
(`turnAcceptsRequests`). Covered for permissions after a turn ended, permissions during a cancel, and questions after a turn ended. **Gap 2** is resolved on the browser side: showing
"cancelled" at once is deliberate feedback, and the force-stop bounds the worst case. What the review of output after the status flips found: text and tool reports that arrive late are still shown and a late report of how a tool ended still replaces what the row said, but a tool left pending or in progress when the turn stopped kept a busy "Running" state on a chat that said it stopped. Those rows are now shown as stopped when the turn stops (cancelled, finished, errored or ended).

**Fixed (gap 4, wording only):** the app already marks a chat the machine can no longer find as lost, and the sessions list says "Connection lost". The chat itself now says
why it closed: a lost chat explains that reet lost contact (for example when the connection closed or the reet service on the machine restarted) and that the
history, held only in memory, is gone, so a new chat is needed; a chat that merely exited says only that it ended, because the state cannot tell a chat that stopped on its own from one the person ended. Adopting a surviving native session after a
connector restart is not implemented and not claimed.

**Proposed order:** (1) bounded force-stop for an ignored cancel: after a grace period without an acknowledgement, end the chat explicitly with a notice
and confirm the process group is gone, rather than leaving it busy; (2) tests for gaps 2 and 3 first, fixing only what they show; (3) say why a chat ended
when the connector restarted; (4) descriptor-driven controls when a second agent needs them.

## Delivery and completion evidence

Keep one bounded stage per PR. Reuse protocol fixtures, fake children and browser
fixtures rather than duplicating a test stack for each agent. Review actual
wire captures to prevent fixtures from agreeing on the same invented contract.
Record exactly which adapter version/platform and real-binary/provider/phone
checks ran. A registered agent or a successful initialize is not completion.

Current status: SC1's negotiated descriptor, launch gating and tool-bound
history contract is implemented in this branch. OpenCode remains the only
reviewed integration; a descriptor reports local installation evidence and
launch still checks executable/handshake identity and version. The native ID
validator now dispatches by tool while retaining OpenCode's existing namespace.
No additional agent namespace is admitted before its adapter probe.

Next: SC2's remaining integration gates (see the probe report); Claude's lifecycle probes, UUID validation and typed edit-diff normalization are done. Live OpenCode model/approval switching,
embedded images and managed software maintenance have already shipped on main;
SC3–SC5 must extend those contracts rather than restart their implementations.
SC5's broader process recovery and generic controls remain future work.
