# Roadmap

What is planned but not shipped yet. Everything else described on the landing page is available today.

These are directions, not commitments: there are no dates.

Next, in this order:

- **Structured chats across agents.** OpenCode in Gaffer is available. Cline is too: a machine with Cline 3.0.74 or newer offers Gaffer for it, launched with `--auto-approve false`, asking before every read, edit and command, with act/plan modes and its stored history. Claude Code and Codex chats are built and wired but off by default: each account turns them on for itself in Settings > Agents, and nobody is offered them otherwise. On a machine that opts in, a Codex chat is offered in the browser with its approvals control (read-only, or let Codex decide, in words that say what each means) and an approval card that shows the model's reason and says that a reject ends the turn. Claude Code in Gaffer is feature-complete for a chat: history and reconnect, a chosen model and thinking level, images, YOLO behind the usual gates, a missing-adapter hint, and a terminal launch with resume, YOLO and a thinking level. It still waits, like Codex, on a signed-in run before it is on by default; its logged-out behaviour is recorded and the browser names that sign-in step. Typed agent questions, force-stop and late-callback handling are done. Codex login access is merged (#391), with its sign-in error and a label-only notice. Live model controls now follow each agent’s descriptor (#392, merged). Pending approval recovery through full reconnect backlogs is merged ([#396](https://github.com/eyupio/reet.sh/pull/396)). Pending typed-question recovery (#402), confirmed live model switches (#406) and explicit startup model confirmation (#409) are also merged. Connector-restart recovery (#414) now reopens recorded chats and restores the controller transcript. The bounded SC5 slice describes the other selectable options an adapter was measured to offer: effort now has controls (the same choices in both start dialogs, in saved backlog plans and in the live session, through `acp-start-options-v1` and the live controls), and model-config values are described but not yet surfaced. Next: finish and record the device and login checks, then make the chats on by default; the steps and the recording ledger are in [Device checks](device-checks.md). See the [implementation plan](https://github.com/eyupio/reet.sh/blob/main/docs/structured-chat-plan.md) and the resume-here section of [handoff.md](https://github.com/eyupio/reet.sh/blob/main/handoff.md).

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
for offline, expired, unsupported or oversized images) have shipped; see the merged pull requests.
