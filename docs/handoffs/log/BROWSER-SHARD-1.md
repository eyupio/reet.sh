## BROWSER-SHARD-1: the browser suites run side by side and report every failure (24 November 2026)

The last two buildable parts of the roadmap's "Delivery speed" item that touch the browser suites.

- `scripts/run-browser-suites.sh` runs a list of `web/tests/NAME.e2e.mjs` suites and does not stop at the first one that
  fails: each suite's output is kept, a failure is raised as an error annotation with the end of its output repeated
  outside the folded group, and the run ends by naming every suite that failed (exit 1 if any did). Tried with five fake
  suites, two of them failing, in a scratch copy.
- `.github/workflows/terminal-browser.yml`: the terminal job was 58 scripts one after another, then the real-binary Go
  steps, in one job (45 minutes allowed). It is now `terminal-ui` with four shards that run at once: three of the script
  list in `web/tests/ci-terminal-ui.txt` (every third script, by position) and a fourth, `real`, for the real browser,
  broker and tmux shell step and the root shell step. `npm run check` runs once, on shard 1. A job named
  `terminal-browser` waits for all four and fails unless they all passed, so that name still means what it did. The phone
  matrix (`phone-dialogs`, `phone-chat`, `phone-gaffer`, `phone-sessions`) and `phone-zoom` use the same runner.
- A script added to `web/tests` that needs the preview server goes in `ci-terminal-ui.txt` (it lands in a shard by its
  place in the list) or in a phone group, as before.

Not done: `gaffer-settings.yml` still runs its 19 scripts one after another and stops at the first failure; the other
browser workflows are unchanged. Not checked: how long each terminal shard takes, because the workflow had not run on
the runners when this was written; read the times from the first run and rebalance the list if one shard is much slower.
The list is by position, not by measured time.
