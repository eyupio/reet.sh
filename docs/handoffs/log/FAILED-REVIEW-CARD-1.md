## FAILED-REVIEW-CARD-1: a failed Gaffer review says what to do next, and a waiting card links to its session (28 November 2026)

A person sent a tablet screenshot of a Factory card for a stage whose Gaffer review had failed. It read "Gaffer’s review
did not complete (failed): The provider returned no visible answer. Check the model or reasoning setting before trying
again. It will not be repeated automatically; resume the goal to try again." with **Try the review again** and **Details
and other actions** under it. Two things were unclear: what to do (the text said "resume the goal", a word the page
never uses, for what the filled button does; and "the model or reasoning setting" said no where), and there was no way
to open the stage's session from the card to see what its agent had been doing.

What changed.

- `needsYou` in `web/src/lib/gafferOutcome.ts` words a failed review for the card (`reviewFailedWhy`): "Gaffer’s review of
  this stage did not finish: <reason> It will not run again by itself." The controller's own note
  (`FinishGafferGoal`, `internal/store/store_gaffer_goals.go`) is unchanged, because a goal opened outside the Factory
  still shows it and there "resume" is its control. A note that does not have that shape (the older "interrupted,
  cancelled or failed" one) is shown as it is.
- The provider's reason for an empty answer (`internal/gaffer/worker_tools.go`) now says "Check the model and reasoning
  effort in your Gaffer settings before trying again", the page and the field names the person will find at
  `/account/gaffer`.
- A failed review's choices now end with **Open the session** (they had no way to it).
- `WaitingItem.svelte` takes `sessionHref` and, in its card layout, shows an **Open the session** link after the other
  buttons whenever the stage's choices include opening it as something other than the filled choice. It is a link, not
  a button, because it goes somewhere; its name is "Open the session for <title>". A row (wide Home) and the compact
  list item do not show it: the buttons column is full, and the detail has the link in its head. The ended-session card
  keeps its own label (**Read the ended session**). Factory and Home both pass the address.
- A second pass, after the person asked for the card's elements to be simplified, clarified and coloured (they chose
  all four options offered). `Need` has a `problem` flag, set by `needsYou` for a failed review. `factoryItems` gives
  such an item the chip **Review failed**, and a decision item carries `agent` (the stage's `agentState`,
  `agentActiveAt`, `agentHeldSince`). `WaitingItem.svelte` draws a problem as a full-bordered, `--bad`-tinted panel led
  by a ! mark (no side bar), tints the chip, and adds "Agent: working · last active 2 min ago" from `agentActivity`.
  **Details** and **Open the session** are `btn quiet` underlined text, so the filled button is the one next step. The
  visible label is **Details** at every width (the long and short spans are gone; the accessible name is still
  "Details and other actions for <title>"), and the objective line is clamped to one line, not two.
- `factory-pieces.e2e.mjs` has a case for the failed-review card at 390 px dark (link at least 44 px tall) and 1000 px
  light, checking the words, the link's address and no sideways scroll. `gafferOutcome.test.mjs` checks the words and
  the choices.

Checked: `npm test` in `web/` (134 files), `svelte-check` (0 errors), `go test ./internal/gaffer/`, and the browser
suites `factory-pieces`, `home-waiting`, `gaffer-factory`, `gaffer-factory-session` and `gaffer-outcome` against a built
UI, with screenshots of the card at 390 px dark and 1000 px light.

Not done. The card shows what the agent reports, not its latest output; the link opens the session for that. Only a
failed review gets the problem panel; a usage limit or an ended session could use it too. Reviews that failed before
this change keep their stored note, but the card words them the same way because it reads the note, not a new field.
At 390 px **Open the session** wraps to a second line under the filled button and **Details**.
