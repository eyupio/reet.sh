# Roadmap

What is planned but not shipped yet. Everything else described on the landing page is available today.

These are directions, not commitments: there are no dates.

Next, in this order:

- **Structured chats across agents.** OpenCode in Gaffer is available. Cline is too: a machine with Cline 3.0.74 or newer offers Gaffer for it, launched with `--auto-approve false`, asking before every read, edit and command, with act/plan modes and its stored history. Claude Code and Codex chats are built and wired but off by default: each account turns them on for itself in Settings > Agents, and nobody is offered them otherwise. On a machine that opts in, a Codex chat is offered in the browser with its approvals control (read-only, or let Codex decide, in words that say what each means) and an approval card that shows the model's reason and says that a reject ends the turn. Claude Code in Gaffer is feature-complete for a chat: history and reconnect, a chosen model and thinking level, images, YOLO behind the usual gates, a missing-adapter hint, and a terminal launch with resume, YOLO and a thinking level. It still waits, like Codex, on a signed-in run before it is on by default; its logged-out behaviour is recorded and the browser names that sign-in step. Typed agent questions, force-stop and late-callback handling are done. Codex login access is merged (#391), with its sign-in error and a label-only notice. Live model controls now follow each agent’s descriptor (#392, merged). Pending approval recovery through full reconnect backlogs is merged ([#396](https://github.com/eyupio/reet.sh/pull/396)). Pending typed-question recovery (#402), confirmed live model switches (#406) and explicit startup model confirmation (#409) are also merged. Connector-restart recovery (#414) now reopens recorded chats and restores the controller transcript. The bounded SC5 slice describes the other selectable options an adapter was measured to offer: effort now has controls (the same choices in both start dialogs, in saved backlog plans and in the live session, through `acp-start-options-v1` and the live controls), and model-config values are described but not yet surfaced. Next: finish and record the device and login checks, then make the chats on by default; the steps and the recording ledger are in [Device checks](device-checks.md). See the [implementation plan](https://github.com/eyupio/reet.sh/blob/main/docs/structured-chat-plan.md) and the resume-here section of [handoff.md](https://github.com/eyupio/reet.sh/blob/main/handoff.md).

Reliability, hardening and speed (a workstream that runs alongside the Factory delivery review; items are proposals, in
priority order within each group):

- **Security and access.**
  - Harden sign-in rate limiting for controllers behind a reverse proxy, so one visitor cannot throttle another and a failed
    attempt is charged to the right key. Built: `REET_TRUSTED_PROXIES` names the proxies whose `X-Forwarded-For` is
    believed, IPv6 addresses are counted by their /64, and a failed password sign-in is charged to its address, to the
    email from that address and to the email from anywhere (hourly), a right one to none of them.
  - Give machine enrolment its own narrow, short-lived credential instead of a personal API token, and require a browser
    session to create or revoke tokens.
  - Extend `SECURITY.md` with a scope and trust model for the remote surfaces: the machine agent, enrolment, the
    controller-to-machine terminal path, YOLO and approval-bypass modes, and what a relay can see.
  - A machine-local lock that refuses every controller-initiated launch and attach until the owner lifts it.
- **Agent behaviour.**
  - Terminal "waiting" detection for Cline, Antigravity and Copilot, with the question and its options, not only a yes/no.
  - A turn that ends while a subagent or background command is still running should not read as finished. Built for
    ACP chats; not yet checked against real agents.
  - Show an agent's slash commands in Chat (`available_commands_update`), and say when a command needs the terminal. Built;
    not yet checked against real agents.
  - Show the full Claude Code plan at its approval dialog, and make the Codex queue visible, with answer keys re-checked
    against the current attachment before they are written. The question card and the re-checked
    answer are built for Claude Code and Codex, not yet checked against real agents; the full plan and the Codex queue
    are not.
  - An Antigravity chat read from the conversation ledger the agent writes, so a turn cannot go silent. Staged; the plan is in
    `docs/superpowers/plans/2026-11-22-agy-transcript-chat.md`.
- **Phone and web.** Keep unsent drafts and queued messages across a reload or a discarded tab, keep the composer typeable
  through a short reconnect, notice and repair a dead push subscription, open a notification on the exact session and view,
  and add an optional audible alert.
- **Delivery speed.** Shard the Go suite across CI jobs (`internal/store` on its own shard), run the web tests in parallel and
  report every failure, give flaky tests an owner and an end date, move changelog entries to one fragment per PR, and keep
  `handoff.md` to a bounded length with a short `AGENTS.md` for the traps the code does not show.

Overlap with the Factory delivery review, so the two do not collide: its stage 4 (delivery reliability) and the recovery states
of stage 3 should own anything about a lost connection or a launch that is retried, and the "turn ended with background work"
item above should reuse the states they define. Its stage 5 (first useful run) should carry the phone setup step. Its stage 7
(release verification) and the CI items above should be planned together so the test lanes are changed once.

Later:

- **Fleet-wide agent configuration.** Configure agents across the whole fleet from one place. Today skills and setups are synced and restored per machine, with a reviewed plan before anything changes.
- **Batch maintenance for every action.** Extend the reviewed batch beyond install and update to rollback and remove, keeping the destructive actions behind typed confirmation.
- **Mirror resilience.** Resume a partial download and let an account prefer the controller's artifact mirror over the registry; today the registry is tried first and a download is not resumed.

Copy and paste (text and images), structured OpenCode chats, the session workspace (Overview, Changes, Files and GitHub
views), YOLO sessions, **Needs you** (`attention-v1`), inline
approvals on Home, **agent software** (OpenCode install/update/rollback/remove and restart recovery, with a fleet review,
a reviewed batch apply with a pilot order, one-machine-at-a-time apply, and a controller-side artifact mirror for
machines without registry access), **model defaults** (Make default in every
model picker, live model switching and clearer provider errors), **per-session model and effort** (the same choices in both
start dialogs, in saved backlog plans and in the live session, with agent default always available) and **agent image
previews** (inline previews of images an agent reads, writes or references, a full-size viewer, and labelled placeholders
for offline, expired, unsupported or oversized images) and **desks** (saved sets of terminal tabs split into panes on any
machine, with each shell's live folder, a command history per pane and Reopen after a restart; see [Desks](desks.md))
have shipped; see the merged pull requests.
