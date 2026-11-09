# Structured chats across agents

Status: implementation plan, last updated 9 November 2026. OpenCode, Cline and Google Antigravity
chats are offered to every account. Claude Code and Codex are built and wired but off by default:
each account turns them on for itself in Settings > Agents (a stored account setting, not an
environment variable) until the device and login checks in [device-checks.md](device-checks.md) are
recorded. Claude Code is feature-complete for a chat; see
[Claude Code in Gaffer](https://github.com/eyupio/reet.sh/blob/main/docs/structured-sessions.md#claude-code-in-gaffer).
The progress notes
below are in the order they were written; the current position and the order of what is left are
in the resume-here section of [handoff.md](../handoff.md).

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
controls where the wire contract matches. Pi is a later candidate,
not a scope addition to the first two integrations.

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

### SC4 progress: Codex Stop ends the chat when a command is running (1 November 2026)

Decided by the owner: Codex's `session/cancel` ends the turn but leaves a running command alive, and the commands are children of the engine the adapter starts, so reet cannot stop just
the command safely. `Spec.EndChatOnStopWhileToolRuns` (set for Codex only) makes Stop, when a tool is still running, say "Codex was running a command when you pressed Stop, so this chat
was ended to be sure nothing keeps running." and end the chat, retiring the process group. A Stop with nothing running only cancels the turn. Still dark behind
`REET_EXPERIMENTAL_CODEX_CHAT`; the browser does not offer Codex yet.

### SC4 progress: Codex mode switch in the session (1 November 2026)

First of three slices for the browser mode control (this one is the session core; then the wire, broker and API; then the browser). `Spec.ChoosableModes` lists the modes a person may
switch a live chat to (Codex: `read-only` and `agent`, never `agent-full-access`). `Session.SetMode` refuses any other mode, refuses while a turn works, and reports the switch as applied
only when the adapter's reply names the requested mode (`Session.Mode` is the last confirmed one). Nothing outside the session can call it yet, so it changes nothing for anyone.

### SC4 progress: Codex mode switch on the wire (1 November 2026)

Second of three slices for the browser mode control. New feature `acp-agent-mode-v1`, negotiated like the model switch: an agent's descriptor lists `modes` (the profile's `ChoosableModes`; Codex: `read-only`, `agent`) only on a
connection that negotiated it, and `Capabilities.ACPAgentMode` is set only when some available agent lists a mode. A new `acp_agent_mode` message moves a live chat; the connector hands it to `Session.SetMode`, which
confirms the mode with the agent. The broker (`SetAgentMode`) refuses a mode the descriptor does not list before anything is sent, and `POST /api/v2/terminal/sessions/{id}/chat/agent-mode` answers 200 with the mode only after
the agent confirmed it, 409 when the machine or agent does not offer it or the agent did not switch. Still dark: no shipped agent lists modes unless Codex is opted in.

Not in this slice, needed before the browser can show the mode: the current mode is not stored or sent to the browser (a reload would not know it), and the card wording. That is the third slice, which adds the stored mode on the
session and the plain-language control.

### SC4 progress: the chat's mode is stored on the session (2 November 2026)

Owner decision: the confirmed agent mode is stored on the session, like the model, so a reload shows it. `terminal_sessions` gains `agent_mode` (empty by default, so existing rows and agents that
offer no modes are unchanged) and the session JSON carries `agentMode`. A new chat records the first mode its descriptor lists, which the profile guarantees is the mode the machine opens it in
(a test pins the first `ChoosableModes` entry to `StartMode`). A confirmed switch records the new mode. A switch the machine did not confirm records "unknown" (empty) rather than the old mode,
because the agent may have switched. The browser control, which reads `agentMode` and shows the plain-language wording, is the last slice.

### SC4 progress: the approval card shows the agent's reason (2 November 2026)

Both of the card gaps the Codex probes left behind are closed, on the shared card, so the browser is ready the day Codex leaves its switch.

- **The reason the agent gave is on the card.** Codex puts the model's justification in `toolCall._meta.permission.description`; the machine now reads that
  (`permissionParams` in `internal/acpagent/session.go`) and sends it in the permission event's existing `text` field, which already carried Claude's `rawInput.description`.
  No new wire field, so a peer that decodes strictly still reads the event. The browser keeps it as `description` on the approval item (counted against the chat log's
  bound), and the card shows it above the command under a `Why {agent} asks` label. Both shapes are pinned by `TestPermissionCarriesTheAgentsReason` (a Codex escalation,
  a Claude write and a card with neither), and `chat.e2e.mjs` drives the card in a built browser.
- **The reject button says what it does.** Codex ends the whole turn when an approval is rejected, not just the one tool, so its button reads "Deny and end the turn"
  (`rejectLabel` in `web/src/lib/chat.ts`); every other reviewed agent keeps "Deny", because their rejection closes the tool the card is about. Pinned by
  `chat.test.mjs`.
- **The connector's mode switch has its own test.** `TestChatHostSwitchesAgentMode` covers what `setAgentMode` refuses (an unknown chat, a chat that is busy or no
  longer running, and a process that offers no switch) and that the mode reaches the process unchanged and an agent's refusal comes back as it was, so nothing claims a
  mode the adapter did not confirm.

Codex stays dark behind `REET_EXPERIMENTAL_CODEX_CHAT` until the sign-in run in section 3 of the device checks is recorded.

### SC4 progress: the Codex chat in the browser (2 November 2026)

Last of the mode-control slices. `codex` is now a reviewed chat tool in the browser (`CHAT_ADAPTERS`, adapter `codex-acp`), so the Start dialog offers Gaffer for Codex only on a machine whose descriptor reports it available, which still
needs `REET_EXPERIMENTAL_CODEX_CHAT=1` there. Codex takes no OpenCode plugin trust. The Start dialog and the Home dialog say what Codex does (it runs commands inside its sandbox and asks before one leaves it; in the other mode it may
not ask). The chat screen shows an **Approvals** control, built from the descriptor's `modes`, with the two wordings agreed earlier: "Codex runs commands inside its sandbox and asks before one leaves it." and "Codex decides for itself what
needs your approval. You may not be asked." A switch is shown only after the machine confirms it; a switch it does not confirm shows no active mode and says "Codex's mode isn't confirmed. Choose one before you send anything else." The empty
chat screen explains the sandbox in the same terms. `web/tests/chat-codex.e2e.mjs` covers all of it and runs in the browser workflow.

Still open before Codex can be offered to anyone: the real-login check (`docs/device-checks.md`, section 3) and the connector's way of letting the adapter see the person's own Codex login. Until then a Codex chat on a signed-in machine may
fail to start ("Sign in to Codex on that machine first" is not built yet).

Decided by the owner for that login: the Codex profile sets `CODEX_HOME` to the person's own Codex folder (`~/.codex`, or the `CODEX_HOME` the connector itself runs with) and passes nothing else from their real home, so the chat uses
the same login they use in the terminal and the adapter can refresh it there. The alternatives (copying `auth.json` into a private folder each start, or passing the real `HOME`) were rejected: the copy goes stale when a token is refreshed in the chat,
and the real `HOME` exposes everything the connector keeps from every other agent. It is built only after the signed-in run in `docs/device-checks.md` section 3 records how the adapter behaves with a real login.

### SC4 progress: Codex login access (2 November 2026, draft)

The owner approved implementing from source evidence with the real signed-in run kept as final verification, rather than a prerequisite to coding.
The Codex profile routes `CODEX_HOME` to the connector's own setting, or to `.codex` under its original `HOME`. The adapter keeps the connector-owned
replacement `HOME`; XDG home directories and other agents' credentials are not forwarded for Codex. The folder is used directly, so Codex can refresh
its login there; reet neither copies credentials nor calls `authenticate`. An invalid or relative login path is refused before launch.

A `session/new` error with code `-32000` and message "Authentication required" now reaches the browser as "Sign in to Codex on that machine first."
Only Codex gets that mapping. `_auth/status_update` becomes a bounded notice in the existing chat stream: "Codex sign-in: {label}". Only the known
status kind and public label are decoded; account/email and gateway detail are discarded, and labels containing an email or control characters are refused.
There is no new private-protocol field or message. Other agents ignore this extension.

The pinned adapter source confirms that `checkAuthorization` asks `account/read` with `refreshToken: false` and refuses only when authentication is
required and no account was returned. Its status shapes are `account` (for example "ChatGPT Pro", with email separately under `account.email`),
`api_key`, `none`, `gateway` and `external`. This is source evidence, not a successful real ChatGPT login run.

Delivery record:
- **Task / scope:** SC4 Codex login access, actionable start refusal and label-only notice.
- **Branch / implementation / PR:** `fix/codex-login-access`, `d593b86`, [draft #391](https://github.com/eyupio/reet.sh/pull/391). Main `99828de` was merged with `b0389dd`; history was not rewritten.
- **Compatibility:** existing notice and refusal messages, no protocol or database change; the login-folder route is set only on the Codex profile.
- **Checks run:** the new tests failed before implementation; Codex and connector race regressions passed, as did the web checks (zero Svelte errors/warnings), production build, Codex browser fixture including notice replay, and `go vet ./...`. Final broader check and CI results are recorded in the PR.
- **Existing container failures:** a detached `99828de` checkout reproduces `TestCloseKillsOrphanedGroupMembers` and `TestModelsOneAtATime`, `TestModelsFloorReturnsTheLastResult`, `TestModelsReplyShapeAndOperation`, `TestModelsLateRunAnswersTimedOut`; they are outside this slice.
- **Not run / limitation:** real ChatGPT login, physical-device and screen-reader checks. Source inspection and fake children do not close those gates.
- **Next unchecked task:** section 3 Run B in `docs/device-checks.md`, then its signed-out follow-up; read that section and this progress note first. If the signed-in start still fails, bring the decision to the owner.
- **Publication:** draft PR only; no merge or deployment.

Codex remains behind `REET_EXPERIMENTAL_CODEX_CHAT`; section 3 Run B in the device checks is still
required before enabling it. If that run still returns "Authentication required", stop and ask the owner which way to go. Claude and typed-question
checks, retiring either switch, generic controls and recovery remain separate work.

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

### SC5 progress: live model controls follow the descriptor (2 November 2026, draft)

The owner asked to continue independent work while the login checks are pending. This is one bounded SC5 slice, based on main independently of
Codex login PR #391. No login or device gate is bypassed and neither experimental switch is retired.

**Problem:** Codex's available descriptor offers approvals and modes, but no verified live model switch. The chat nevertheless rendered the Model field,
Change model and Make default, requested model options and favourites, and told people to choose another model after provider errors. The controller
already refused unsupported switches; the browser was offering an action its descriptor did not support.

**Behaviour:** `TerminalWorkspace` passes `structuredAgent(...).model` to `ChatPane`. Missing, false or unavailable support hides the model controls and
skips their lookups. Explicit descriptors remain authoritative over global flags; older OpenCode peers use `acpModel`. The pane reacts when support
appears or disappears without remounting. Removal clears options and aborts requests; stale replies are ignored. An unchanged capability refresh does
not refetch. The current confirmed model and mode-switch acknowledgement rules remain as before. Error guidance names only actions the pane offers.

Delivery record:
- **Task / scope:** SC5 descriptor-driven live model controls; no effort or new mode support.
- **Branch / implementation / PR:** `fix/descriptor-chat-model-control`, `5d0c82c`, [#392](https://github.com/eyupio/reet.sh/pull/392), opened as a draft.
- **Compatibility:** browser only; existing descriptor/legacy flags and API routes, no new wire field, Go change, database migration or dependency.
- **Checks:** the Codex browser regression failed before the fix because a Model textbox was present. Web checks passed with zero Svelte errors/warnings;
  production build and Claude/OpenCode browser fixtures passed (OpenCode through both legacy flags and explicit descriptors). The Codex fixture covers
  capability changes, a delayed response after revocation, unchanged refreshes, four provider-error cases, and light/dark at 1360 and 390 px. Final CI
  and check results are recorded in the PR.
- **Not performed:** physical-device, screen-reader or genuine-login checks; no new adapter capability is enabled.
- **Next unchecked work:** owner login/device checks remain pending. SC5 still needs broader descriptor-driven mode/effort option contracts and process
  recovery; read this note and the audit below before choosing that next bounded slice.
- **Publication:** open as a draft, owner controls ready/merge; no merge or deployment claimed.

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

### SC5 progress: connector restart and controller transcript recovery (2 November 2026)

Gap 4 is closed without a new process model. A structured chat's OpenCode id is recorded on the machine beside the
terminal state; when the connector process starts again after an upgrade, a crash or a reboot, it reopens each recorded
chat with `session/load` under the same reet session id, drops the replay (the controller already has the transcript),
and reports the row running so the controller adopts it instead of marking it lost. Ending a chat, archiving it or
switching it to the OpenCode TUI removes its record. Folder trust is re-checked, YOLO is re-checked against the account
and the machine switch, records older than seven days are dropped, and a failed reopen is attempted at most three times.
The controller now stores the same bounded transcript (1,000 events / 256 KiB per session) in SQLite, so its own restart
or a reboot restores the conversation rather than just the row; archiving still drops it. A lost row can return to
running only for a structured chat, so a plain shell is never resurrected. Detached per-session workers and turn
generations across a native process restart remain future work; a turn in flight when the connector stopped is reported
as stopped, not resumed.

**Proposed order:** (1) bounded force-stop for an ignored cancel: after a grace period without an acknowledgement, end the chat explicitly with a notice
and confirm the process group is gone, rather than leaving it busy; (2) tests for gaps 2 and 3 first, fixing only what they show; (3) say why a chat ended
when the connector restarted; (4) descriptor-driven controls when a second agent needs them.

### SC5 progress: pending approvals after a full detached backlog (2 November 2026, merged #396)

Independent work while the owner login/device checks are pending; initially based on main `9905e8b`, then main `0e6a3b2` (including #391 and #392)
merged without rewriting history. A replacement control connection used to rebuild its
outbox with the last status and retained pending approval cards **before** the buffered history. At either queue limit, replaying that history evicted
the recovery records again. A running agent could therefore remain waiting while the replacement controller received no approval card. The current
idle status after a cancellation could be lost the same way.

`seedRecoveredChatEvents` now replays history first, removes buffered copies of currently pending approvals, then appends their current snapshots
and the latest status. Historical resolution receipts remain ordinary bounded history so they cannot monopolize the queue and displace new answers. Overflow discards old history before current pending approvals and the latest buffered status, so a stalled
replacement transport cannot let fresh output evict the recovery records again. Requeueing a failed send applies the same trimming policy. If an approval resolves during replay and settles the retained waiting state, its new status is buffered too.
The original token and complete card travel unchanged. Resolved approvals retain their historical events and are not recreated
from the pending set. The outbox still uses its existing 1,000-event and 256 KiB limits. No process is replaced and no new/load or approval call is
introduced by preparing recovery.

Delivery record:
- **Task / scope:** SC5 retained approval/status recovery over a replacement control connection to the surviving connector process.
- **Branch:** `fix/recover-pending-chat-approvals`; implementation commits `5699548`, `40f67cd` and `ef2a418`; [#396](https://github.com/eyupio/reet.sh/pull/396).
- **Compatibility:** existing events, tokens, generation and memory limits; no protocol field, database, dependency, adapter capability or UI control added.
- **Checks:** the event-limit and byte-limit regressions failed before the fix, as did recovery of the resolved idle status. They pass under `-race`
  with existing reconnect/startup tests. Tests also check complete command retention, answering the original process, the replacement generation,
  replay budgets, and a buffered approval appearing once alongside resolved history. A blocked-transport test failed under continued output;
  a failed-send variant also reproduced a 1,001-event outbox. Both now pass under the race detector. A cancellation during blocked replay also reproduced a stale waiting status; buffering the newly settled state fixes it. Control-state-only overflow also remains
  bounded by the existing ceilings. A separate regression reproduced old receipts displacing new answers; ordinary bounded receipt history fixes it. Vet and race tests pass for terminalagent, terminalbroker and terminalproto; the four
  known container `TestModels*` failures were excluded from that local package run (already confirmed on main; the model-discovery code is unchanged).
  The production web build passed. Full implementation CI passed on `ef2a418`, including web/MCP checks, full Go vet and race tests without the local exclusions, release/private-connection checks, terminal-browser, both phone jobs, macOS/Linux native services and minimum tmux. Linux native services passed its single allowed retry after its standalone fixture failed to start at `native_test.go:154`; main `0e6a3b2` also passed that check ([run](https://github.com/eyupio/reet.sh/actions/runs/41483989031)).
- **Not performed:** real-provider, physical-device, screen-reader and genuine-login checks. Existing fake-process/control-link fixtures exercise this slice.
- **Next unchecked work:** owner checks still start with section 3 Run B in `docs/device-checks.md`. Broader native-process recovery and descriptor
  contracts remain future SC5 work. Pending typed-question recovery is outside this approval slice; the form capability remains off.
- **Publication / review:** [#396](https://github.com/eyupio/reet.sh/pull/396) was opened as a draft and marked ready after implementation CI passed. The automatic Codex bot review failed because the account’s code-review quota is exhausted ([bot response](https://github.com/eyupio/reet.sh/pull/396#issuecomment-6583179410)); submitted reviews and inline threads were empty. The owner subsequently merged #396 as `35225eb`; the bot review did not complete before that merge. No deployment or real-login/device completion is claimed.


### SC5 progress: delivered typed questions across transport reconnects (2 November 2026, draft)

This bounded follow-up to #396 retains complete, already delivered typed-question cards on the surviving connector process. Reconnect replay
places their original tokens and schemas after buffered history, once, alongside pending approvals and current status. Old output is evicted
before pending cards; question IDs and serialized schemas count against the unchanged 1,000-event / 256 KiB queue limits. The native process
still owns deadlines, single-use tokens and answer validation. No new/load, new token or new adapter behaviour is introduced.

Question resolution and timeout callbacks now retire local state while detached or replaying. Waiting settles only when neither approvals nor
questions remain. A replacement peer without `acp-question-v1` receives no question frames, including historical frames from exited chats;
pending questions are cancelled through the original process. Questions first asked without a live negotiated recipient are still cancelled.

Delivery record:
- **Task / scope:** SC5 pending typed-question recovery over a replacement control connection; no connector-process restart adoption.
- **Branch / PR:** `fix/recover-pending-chat-questions`, implementation `89fc305`, draft [#402](https://github.com/eyupio/reet.sh/pull/402). Based on main `9961b11`;
  main `d5a4e80` (the independent documentation-only #401) merged without rewriting history.
- **Compatibility:** existing negotiated events, tokens, generation and queue limits; no protocol field, dependency or database change.
- **Checks:** final targeted question/approval recovery and negotiation tests pass under `-race`, including stalled/failed replay and a late old-link
  failure requeueing a frame during older-peer replay. Regressions fail against main `9961b11`: missing cards even without a backlog, stale waiting
  after a detached timeout, no cancellation for an older peer, omitted schema bytes, mixed-card waiting loss, and the late-frame decoder hazard.
  Web tests/check/build, all 16 MCP tests, full Go vet and whitespace checks pass. Full race/CI and bot review results are recorded in #402.
  The four known `TestModels*` failures and `TestCloseKillsOrphanedGroupMembers` were reproduced on current main in this container.
- **Not performed:** genuine login, physical-device, screen-reader or real-provider checks. The owner deferred signed-in Codex Run B in this session;
  it remains unverified, rather than recorded as a pass. Both experimental switches and the disabled form capability remain in place.
- **Next unchecked work:** owner checks in `device-checks.md` section 3 Run B, then its signed-out follow-up, section 2 and section 1. Broader
  descriptor contracts and native-session adoption after connector restart remain independent SC5 work.
- **Publication:** #402 is open as a draft; Codex bot review requested. No merge or deployment authorized.

### SC5 progress: confirmed live model switches (2 November 2026, draft)

This independent slice distinguishes an omitted `configOptions` reply from an explicit replacement that cannot establish the model.
Only the OpenCode profile permits its existing omitted-list acknowledgement. Claude requires the model's read-back `currentValue`, as measured
in the SC2 probe. An explicit empty/null/malformed list, missing/empty/invalid model value, duplicate model entries, lost reply or internal RPC
error cannot confirm the requested model. The machine clears its model and retained status detail, returns an error and emits a notice instead of reporting success.
A confirmed different model is still recorded on the process and returned as a rejected choice, preserving the measured silent-reset handling.
An explicit invalid-parameters or unsupported-method refusal retains the process's last confirmed model. A later confirmed switch recovers normally.

The controller clears its stored model after an unconfirmed response or a reply lost after sending. Failure before sending, busy and missing-chat
responses retain the stored value. An empty stored model makes no claim about the agent's current choice; it does not block sending or change
approvals. Existing API errors provide feedback. No new wire field, message, negotiation feature, database column or adapter capability is added.

Delivery record:
- **Task / scope:** SC5 live model acknowledgement and unknown-state handling. Startup model confirmation, effort/mode option contracts and native-session
  adoption remain separate work.
- **Branch / implementation / PR:** `fix/confirm-chat-model-switch`, `9054732`, [draft #406](https://github.com/eyupio/reet.sh/pull/406), independently based on main `7e390b2`. The owner explicitly authorized this slice while #402 was open.
  The owner later merged #402 as `a78ffa6`; current main is merged into this branch without rewriting history. Its changes are not duplicated in this PR.
- **Checks:** new regressions failed before the fix for incomplete replies, strict-profile omitted replies, lost acknowledgements and stale stored models.
  A separate late internal-error regression failed before its fix too. Targeted core/controller race tests, web check/build and all 16 MCP tests pass;
  the API package also passes without race (73 s). A final full ACP package run passes under race with only the baseline orphan test excluded.
  The initial broader local run compiled before the late internal-error fix and is not reported as a final pass; it also hit one Stop timeout.
  The final ACP package and the same Stop test on current main both pass. Merged-tree ACP/connector/broker/protocol race checks pass with only the five baseline-confirmed container tests excluded; vet and API tests pass.
  A further regression reproduced the old model surviving in retained idle-status detail; the follow-up clears that detail for reconnect replay too.
  Full broader results, bot review and CI are recorded in #406.
- **Baseline:** the known model, orphan/process-group and missing-tmux tests were reproduced on current main `7e390b2` in this container.
- **Evidence / limits:** Claude read-back follows the recorded real-adapter probe; OpenCode's pre-existing omitted-list compatibility is preserved.
  New edge cases are fake-child/control-link regressions, not claims that an adapter was observed sending malformed replies. No genuine-login,
  real-provider, physical-device or screen-reader check ran. The owner previously deferred Codex Run B; it remains unverified. Both experimental
  switches and disabled form capability remain unchanged.
- **Next:** owner login/device checks remain in their recorded order. SC5 still has broader option contracts and connector-restart adoption.
- **Publication:** draft PR only; no merge or deployment authorized.


### SC5 progress: startup model confirmation (2 November 2026)

Status reconciliation: the owner merged #402 as `a78ffa6` (all CI passed on `7dec2cc`, bot review completed without findings) and #406 as
`6973f08` (all five CI workflows passed on `59f896f`, no retry, bot review completed without findings). Their earlier draft notes above describe
scope/history, not current publication state. #401's independent documentation update is already merged as `d5a4e80`.

This bounded next slice requires exactly one valid matching model read-back from `session/new` and `session/load` when the person selected a model.
It reuses the live-model acknowledgement parser, with omission disallowed at startup even for OpenCode. Missing, empty, duplicate and mismatched
model reports cannot admit a usable chat or send its opening prompt. No-model/Agent-default launches keep existing behaviour. No extra startup
model-setting RPC is introduced. A definite refused chat create clears unconfirmed model metadata while retaining the operation fingerprint and
refusal receipt; replay does not start another process. Plain terminals and transport uncertainty retain their existing treatment.

Closing a rejected ACP startup emits an ended callback before Start returns. The supervisor now waits for admission before publishing that lifecycle
transition; a post-admission Done watcher still catches an exit racing successful startup. Regressions cover both sides of this ordering.

Delivery record:
- **Branch / implementation:** `fix/confirm-startup-chat-model`, initial implementation `a032345`, based on `1235328`; main `6973f08` and `f051948` merged
  without rewriting history. The owner authorized this slice and requested an up-to-date handoff in its draft PR.
- **Checks:** new fake-child startup tests failed before the fix for incomplete/ambiguous read-back on both new/load and OpenCode/Claude profiles.
  Controller regressions reproduced the stale stored model and verify refusal replay stays idempotent; supervisor regressions reproduced premature exit publication
  and an exit overwritten during admission. These pass under race. Web check/build and Go vet pass; final package/CI/review receipts belong in the PR.
- **Real evidence:** isolated Linux x86-64 probes used OpenCode 1.19.5 and Claude ACP 0.99.1 / SDK 0.3.312, Node 24.19.0. Only initialize/new/load
  were called. OpenCode returned `opencode/big-pickle` on new/load; Claude returned `default`. OpenCode still reported `opencode/big-pickle` when
  private config requested `anthropic/claude-sonnet-4-5`; config alone did not establish selection. Opt-in Go profile tests passed for explicitly
  matching OpenCode new/load and Claude new. See the probe report for exact reproduction and package locks.
- **Limits / next:** no prompts, genuine credentials, model turns, physical devices or screen readers were verified. Codex Run B remains deferred
  at the owner's request, not passed. Both experimental switches and form capability remain off. Broader mode/effort option contracts and native-session
  adoption after connector restart remain separate SC5 work. No merge or deployment is authorized; publication results belong in the draft PR.


### SC5 progress: measured descriptor option contract (2 November 2026)

This bounded slice adds a descriptive contract for the selectable options a reviewed adapter was measured to offer, without adding a
control or changing any launch. The descriptor already covers the model catalogue (`model`) and the permission-sensitive modes a person
may switch (`modes`). It now also carries `options`, a bounded list of `{id, category, values}` for the other measured option categories:
`thought_level`, `model_config` and `collaboration_mode`. A mode or model option is not duplicated there.

The contract is negotiated by `acp-agent-options-v1`, like the mode contract: an older controller never receives the field, and a machine
does not claim the capability without an available descriptor that has options. Values must be lower-case tokens, distinct within an option,
and bounded (at most eight options and sixteen values each); unknown categories and malformed or duplicate values are refused. A descriptor
for an agent that is missing, unverified or incompatible carries no options. The profile copies the measured values into each descriptor, so
a caller cannot mutate the profile through it.

The values were measured before implementation from the pinned adapters: Claude ACP 0.99.1 reports effort `default`, `low`, `medium`, `high`,
`xhigh`, `max`, plus `fast` and `ultracode` (`on`/`off`); Codex ACP 1.13.1 reports `reasoning_effort` (`low` through `ultra`),
`fast-mode` (`off`/`on`) and `collaboration_mode` (`default`/`plan`). OpenCode 1.19.5 reported only model and mode. The values can depend on
the current model (and Codex also embeds effort in some model IDs), so the contract says what may be offered, not that every value is valid
for every model. `scripts/probe-agent-options.py` reproduces the measurement with initialize and session/new only; Codex used a synthetic
api-key placeholder, and no prompt, model call, real credential or network request was made.

Delivery record:
- **Task / scope:** SC5 descriptor option contract for measured non-permission options; no live switch, startup option, UI control, database
  column or adapter capability beyond the negotiated description.
- **Branch:** `feat/descriptor-chat-options`, based on main after #409 and #414/#415. Publication and CI results belong in the draft PR.
- **Checks:** the new descriptor validation, negotiation, profile-copy and feature tests failed before implementation. They pass with the
  existing descriptor and feature-list tests; the real probes above sent no prompt and used no real credential.
- **Compatibility:** a new JSON field on `ACPAgent` behind a new feature; old peers and old controllers are unaffected. No model/mode switch
  or permission behaviour changes.
- **Not performed:** no genuine login, model turn, physical device, screen reader or provider-billing check. The contract does not close any
  of those gates, and both experimental switches and the form capability remain off.
- **Next:** a later bounded slice can use this contract to offer an effort control, reading the live option list from the adapter and
  confirming a switch before reporting it. Native-session adoption after connector restart is already on main (#414); detached workers and
  turn generations across a process restart remain future work.
- **Publication:** draft PR only; no merge or deployment claimed.

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

## Progress note: logged-out runs recorded (4 November 2026)

The pinned adapters were run on a machine signed in to neither agent, which records the logged-out half of device-check sections 2
and 3 without touching a switch:

- `claude-agent-acp@0.99.1` advertises no authentication methods, creates a session with `session/new`, and refuses the first
  prompt with `-32000 Authentication required` (`replyIsOk: false`, `authenticatedHint: false`, no tools requested). A missing
  Claude login is therefore visible only at the prompt, which settles section 2's "does the browser need a message?" question:
  it does, and it has to be at the prompt.
- `TestRealClaudeStart` passes against that adapter with no model call: version floor, adapter identity, the required `default`
  mode and process retirement.
- `codex-acp@1.13.1` with `@openai/codex@0.156.1` advertises `api-key` and `chat-gpt`, refuses `session/new` before any
  authenticate, and reports `{kind: none, label: "Not logged in"}`.

The prompt path now names the sign-in step for both reviewed chat agents (`ErrClaudeLogin`, `ErrCodexLogin`) instead of showing
the adapter's raw words, and the connector reports either as unavailable with that sentence. Signed-in runs for both agents and
every phone and reader item remain open, so both switches and `elicitation.form` stay off.

