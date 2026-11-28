## MACHINES-COPY-1: the Machines page is one compact row per machine, and says what to do first (28 November 2026)

The page a person lands on after signing in said "Connect a machine with the reet CLI" while its button said "Add a
machine", and each machine was a 420 px card with five overlapping status signals, a loud reet agent panel ("Not
checked") and a second primary button, repeated for every machine.

What changed.

- `MachineFleet.svelte`: the cards are single-column rows (about 180 px). The head is a grid of name, one status and the
  actions (Open terminal, Start an agent, the options menu), stacking under 860 px. The status label is coloured only for
  attention, review and action tones; a healthy machine is plain. The Inventory badge shows only when the scan is not
  current, the "Machine inventory and install locations" subtitle, the install-location chips, the sync band box and
  its View sync status button (now a text link) are gone. Open terminal is no longer a primary button. The reet agent
  panel shows on the row when an update is available, queued, running, failed, rolled back or cancelled, and sits inside
  "Agents on this machine" when the machine is current or not checked. The search label is visually hidden.
- `MachineSetup.svelte`: the optional step-by-step card lost its "Machine setup" eyebrow and "Optional" badge and is a
  plain 56 px row, "Set up a machine by hand".
- `Machines.svelte`, `EnvironmentSync.svelte`: copy, as before (intro, empty state in three versions, advanced hints).
- `machines-fleet.e2e.mjs`: the locked card's status is read from `.status-line strong`, and the details are opened
  before the disabled Update button is asserted.

Kept on purpose: "Inventory & connection", "Refresh status", the maintenance summaries, the enrolment-code refusal text,
and every control and test hook the card had (`article.machine`, `.machine-head .badge`, `.dot`, `[data-agent-update]`).

Checked: web unit tests and `svelte-check`; the Machines browser suites and others that open a machine card; the page
by eye at 1360 and 390 px, light and dark. Not checked: a fleet of 50 machines, and a screen reader pass.
