## MACHINES-COPY-1: the Machines page says what to do first, in plainer words (28 November 2026)

The page a person lands on after signing in with no machines said "Connect a machine with the reet CLI" while its
button said "Add a machine", and a viewer saw the same advice twice.

What changed (copy only, no behaviour).

- `Machines.svelte`: the intro no longer tells someone with machines to connect them; it says what the page is.
- `EnvironmentSync.svelte`: the empty state has three texts. With accounts and write access it points at Add a machine
  and its three steps; a writer without accounts keeps the CLI route; a viewer is told machines an owner or editor
  connects appear here (the viewer note above already says who to ask, so the second hint went). The advanced
  maintenance hints drop "heartbeat", "inventory" and "host IDs".

Kept on purpose: "Inventory & connection", "Refresh status", the maintenance summaries and the enrolment-code refusal
text, because tests and `docs/environment-sync.md` name them.

Checked: unit tests for the web client. Not checked: the browser suites, and a look at the page at 390 px and desktop
in light and dark.
