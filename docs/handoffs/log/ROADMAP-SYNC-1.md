## ROADMAP-SYNC-1: the roadmap and the unstable-test list brought up to date (24 November 2026)

Documents only. The roadmap's "Phone and web" and "Delivery speed" items now say what is built (drafts and the queue across a
reload, a typeable composer while reconnecting, a dead push subscription noticed and repaired, a tap that opens its session,
an optional tone; the Go suite in six shards, the terminal browser suites in four shards that name every failure, unstable
tests with an owner and a date, handoff entries and changelog fragments as one file each) and what is not (the
`gaffer-settings` job still runs its 19 scripts in sequence, and nothing checks that a pull request adds its fragment or log
entry). `docs/handoffs/flaky-tests.md` gains two entries: the `add-machine` browser suite's re-enrol step, which timed out
once on a pull request ("not stable", "detached from the DOM") and passed on re-run, and a `phone-dialogs` job whose
`npm ci && npm run build` setup failed once on main before any suite ran. Neither was reproduced.

Checked: the text against what merged that day. Not checked: that any of it holds on a real phone or push service. The owner
and the date (8 December 2026) on the new entries are my choice.
