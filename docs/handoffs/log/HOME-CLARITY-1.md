## HOME-CLARITY-1: Home says plainly what it is showing when nothing is running (28 November 2026)

Why. Home's quiet states used words only the product's authors knew: "Nothing in flight.", an undefined "Gaffer" and
"Factory", and board stages that said "Make room for your next idea." or "Ready when you are." without saying the stage
was empty.

What changed. Mostly copy; no route, API or behaviour change.

- `StartWorkbench.svelte`: the heading is "No sessions running." and the Factory line reads "Or set Gaffer a goal and
  follow the work on the Factory page."
- `InFlight.svelte`: the empty line starts "No sessions running." to match.
- `SessionBoard.svelte`: an empty stage says what it is: "Nothing in your backlog.", "No active sessions." and "No
  finished sessions yet." Their explanations and actions are unchanged.
- `web/tests/workbench.e2e.mjs` and `web/tests/home-waiting.e2e.mjs` name the new heading.

- `InFlight.svelte`: its "Continue working" heading repeated the tab just above it, so it is now visually hidden (the
  region keeps its name) and Group by sits at the right.
- `WaitingItem.svelte`: a failed Factory item takes the danger tone (`--bad`), as a failed session already did.

Not changed. "Nothing is waiting for you.", the Planning board and Continue working labels, and the Yorkshire "Nowt
matches just yet." for a filtered board: many suites pin them and they already say what is true. The Continue working / Planning board toggle is
still two views: folding them into one would drop the backlog from Home and needs its own decision. The signed-out
landing page was not touched.
