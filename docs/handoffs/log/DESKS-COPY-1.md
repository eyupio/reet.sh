## DESKS-COPY-1: the Desks list says what a desk is and what each count means (28 November 2026)

Copy only, in `web/src/lib/desks/DeskList.svelte`; no behaviour changes.

- The sub-line now defines a desk ("A desk is a saved set of terminals. Open one to carry on where you left it.").
- The empty list adds the next step (start one, then add a tab for each shell).
- The New desk card said "Start empty, or from “<template>”" but only ever started an empty desk. It now says
  "Starts empty. Add a tab for each shell."; From a template stays the button in the header.
- The Templates group has a one-line explanation.
- Card counts name the thing counted: "2 shells live", "1 shell to reopen", "1 shell needs you".
- `desks.e2e.mjs` follows the sub-line and the counts (three assertions). The whole suite passes: 105 checks, run against a built preview with the cloud Chromium.

Left alone on purpose: "Move to its own tab" (card menu and desk switcher) can be read as a desk tab rather than a
browser tab, but the same label is in both places and in the changelog, so it wants its own change.
