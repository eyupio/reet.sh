# Roadmap

What is planned but not shipped yet. Everything else described on the landing page is available today.

These are directions, not commitments: there are no dates.

Next, in this order:

- **Richer run orchestration.** More control over agent runs than opening a terminal session and continuing it later.

Later:

- **Fleet-wide agent configuration.** Configure agents across the whole fleet from one place. Today skills and setups are synced and restored per machine, with a reviewed plan before anything changes.
- **Batch maintenance for every action.** Extend the reviewed batch beyond install and update to rollback and remove, keeping the destructive actions behind typed confirmation.
- **Mirror resilience.** Resume a partial download and let an account prefer the controller's artifact mirror over the registry; today the registry is tried first and a download is not resumed.

Copy and paste (text and images), structured OpenCode chats, YOLO sessions, **Needs you** (`attention-v1`), inline
approvals on Home, **agent software** (OpenCode install/update/rollback/remove and restart recovery, with a fleet review,
a reviewed batch apply with a pilot order, one-machine-at-a-time apply, and a controller-side artifact mirror for
machines without registry access) and **model defaults** (Make default in every
model picker, live model switching and clearer provider errors) have shipped; see the merged pull requests.
