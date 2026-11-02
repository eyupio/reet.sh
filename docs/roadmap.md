# Roadmap

What is planned but not shipped yet. Everything else described on the landing page is available today.

These are directions, not commitments: there are no dates.

Next, in this order:

- **Structured chats across agents.** OpenCode in Gaffer is available. Claude Code and Codex chats are built and wired but dark: a machine reports them only when its connector runs with `REET_EXPERIMENTAL_CLAUDE_CHAT=1` or `REET_EXPERIMENTAL_CODEX_CHAT=1`, and nobody is offered them otherwise. On a machine that opts in, a Codex chat is offered in the browser with its approvals control (read-only, or let Codex decide, in words that say what each means) and an approval card that shows the model's reason and says that a reject ends the turn. Claude waits on the real-login checks; Codex waits on the signed-in run and on letting its adapter see the person's own login. Typed agent questions, force-stop and late-callback handling are done. Next: the device and login checks, then the Codex login access, then retire the switches. See the [implementation plan](structured-chat-plan.md) and the resume-here section of [handoff.md](../handoff.md).

Later:

- **Fleet-wide agent configuration.** Configure agents across the whole fleet from one place. Today skills and setups are synced and restored per machine, with a reviewed plan before anything changes.
- **Batch maintenance for every action.** Extend the reviewed batch beyond install and update to rollback and remove, keeping the destructive actions behind typed confirmation.
- **Mirror resilience.** Resume a partial download and let an account prefer the controller's artifact mirror over the registry; today the registry is tried first and a download is not resumed.

Copy and paste (text and images), structured OpenCode chats, the session workspace (Overview, Changes, Files and GitHub
views), YOLO sessions, **Needs you** (`attention-v1`), inline
approvals on Home, **agent software** (OpenCode install/update/rollback/remove and restart recovery, with a fleet review,
a reviewed batch apply with a pilot order, one-machine-at-a-time apply, and a controller-side artifact mirror for
machines without registry access) and **model defaults** (Make default in every
model picker, live model switching and clearer provider errors) have shipped; see the merged pull requests.
