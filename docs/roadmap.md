# Roadmap

What is planned but not shipped yet. Everything else described on the landing page is available today.

These are directions, not commitments: there are no dates.

Next, in this order:

- **Reviewed batch agent maintenance.** reet installs, updates, rolls back and removes its own OpenCode copy on Linux from one verified 1.19.5 artifact, keeps it beside any system installation, recovers after a restart, and reviews each machine's plan side by side, applying one pilot machine at a time. Next: a reviewed batch across machines with a pilot order, and a controller-side artifact mirror for offline machines.

Later:

- **Richer run orchestration.** More control over agent runs than opening a terminal session and continuing it later.
- **Fleet-wide agent configuration.** Configure agents across the whole fleet from one place. Today skills and setups are synced and restored per machine, with a reviewed plan before anything changes.

Copy and paste (text and images), structured OpenCode chats, YOLO sessions, **Needs you** (`attention-v1`), inline
approvals on Home, **agent software** (OpenCode install/update/rollback/remove and restart recovery, with a fleet review
and pilot apply) and **model defaults** (Make default in every model picker, live model switching and clearer provider
errors) have shipped; see the merged pull requests.
