# Add a machine in context (S5 design)

Date: 2026-10-25. Status: design for review; no code written.

Bring another machine in from where you are working, and carry on there. The aim, from the product
owner, is a better multi-machine reet.sh: agents, MCP servers and skills synced across machines, a
professional UX, and machines added just in time so an AI agent's work can continue on them. This
slice is the first step: a guided **Add a machine** dialog that can be opened from the Machines page
and from the Private skills dialog, that watches for the machine to appear, and that hands the new
machine back to whoever opened it.

## Intent and success

**Outcome.** A signed-in owner who finds the machine they want is not listed can add it without
leaving the screen they are on, and when it connects land back on that screen with the new machine
ready to use. For a person in the Private skills dialog that means the new machine's matching folder
opens in the Pin step with the skill they came with already chosen.

**Who it is for.** The account owner with a write role, on a desktop or a phone.

**Success looks like.**

- Nothing on screen claims more than the controller has been told. "Connected" means the machine has
  reported in; it never means a command succeeded on the machine, and it never means verified.
- The person never has to find a separate setup page, but the existing step-by-step setup is still
  one click away for anything the dialog does not cover.
- The same rules as the Review UI hold: nothing about a machine or a token is kept in browser
  storage, polling is fenced and backs off, and an unknown state falls back to "check again".

## Decisions already taken with the product owner

| Question | Decision |
| --- | --- |
| Where does the flow leave you? | Back where you started, with the new machine ready. |
| How is the machine brought in? | A single block of commands to paste on the machine, using today's enrolment. No pairing codes and no SSH from the controller. |
| Where is it offered in this slice? | The Machines page and the Private skills dialog. |
| How is it built? | A reusable dialog with a pure, tested logic module, reusing today's readiness logic and command builders. |

## Out of scope

Pairing codes, remote install over SSH, a "pending machine" record that can be pinned to before the
machine connects, a machine picker inside the Private skills dialog, the terminal workspace entry
point, moving a running agent session between machines, applying changes to a machine (the reviewed
executor), MCP exposure, and any change to terminals, the security/PIN preference or the legacy sync
flows.

## What the existing setup already does

`MachineSetup.svelte` (the "connect" mode of the Machines page) takes a machine name, shows the
install command, asks the owner to create a **machine token** (shown once, never stored in the
browser, pasted when the CLI prompts for it, deliberately not embedded in any command), shows the
configure command, then the enrol-and-run commands, and finally watches the controller for the
machine with `connectionStage`. `machineSetup.ts` holds the command builders (`installCommands`,
`configureCommand`, `enrollCommands`) and the readiness decision. This slice reuses all of that. It
does not change how enrolment, tokens or the agent work.

## The flow

Entry points. A **Add a machine** button on the Machines page beside the existing add action, and a
**Not listed? Add a machine** link in the Private skills dialog, shown where the dialog explains that
no eligible machine exists and under the pins list. Both open `AddMachineDialog.svelte` with a
*context*: nothing from Machines, or the skill name (and library bundle, when known) from the Private
skills dialog. Viewers and legacy auth modes do not see either entry, as for the Review UI.

The dialog is a native `<dialog>` with a fixed header and a scrolling body, in three short parts on
one screen, not a wizard.

1. **Name it.** One field, *Machine name*, prefilled with a suggestion when the owner has typed one
   before in this page's lifetime (in memory only), with the existing known-names list as hints. The
   name is how the machine appears in reet.sh and should stay the same between scans. Trimmed, not
   empty, no longer than the setup field's limit; the server remains the authority.
2. **Give it a key.** The existing machine-token creator (`tokenSetup`) is shown here, with the same
   words: the token is shown once, is not stored in the browser, and is typed or pasted when the CLI
   asks for it.
3. **Run it.** One block, with a Copy button, chaining the existing builders so there is one paste and
   one prompt for the token: install, save the connection, enrol and run. Beneath it, plain words:
   "Nothing happens on that machine until you run this there. reet.sh never logs in to it." The
   *Allow this machine to apply sync plans* option is not offered here (applying is not wired). An
   expander, *Prefer the step-by-step setup?*, closes this dialog and opens the existing setup section
   for the same name.

Below the block sits a live status line (a polite live region) with a text label as well as colour.

| State | Label | Means |
| --- | --- | --- |
| Waiting | Waiting for it | No connector for that name has reported in. |
| Connected | Connected | A connector for that name is enrolled but paused. The plain reason is given; the Private skills dialog carries any later "connector not ready" reason itself. |
| First scan | Waiting for its first scan | The connector is ready; no complete scan of an agent folder has arrived yet. |
| Ready | Ready | Connected, not paused, with a complete scan of at least one supported agent folder in the last 14 days. |
| Already here | A machine called \<name\> is already here. | The name belongs to a machine that was already reporting when the name was confirmed. *Use it* is offered at once, or the person chooses another name; it stays that way while that machine keeps reporting, and the normal states apply only if a new connector enrols under the name. |
| Unknown | Something changed. Check again. | Any unrecognised state. |

*Connected* is explicitly not *Ready*: both labels say what the controller has been told, and the
dialog says that a completed scan does not prove the machine is healthy.

When the state reaches *Ready* the dialog announces it and offers a single primary action, **Use
\<name\>**. It also continues on its own after a short pause in the one case where the person is
waiting at the Private skills dialog, so they are not stranded; they can cancel the pause. Choosing
it closes the dialog and calls `onready({ host, context })`.

### Landing

- From **Machines**: the new machine's card is scrolled to and highlighted, and the dialog's focus
  returns to the button that opened it.
- From the **Private skills dialog**: the shell closes the Pin step for the old machine and opens the
  dialog for the new machine's matching folder. The matching folder is the first supported agent
  folder with a complete recent scan, preferring the same agent as the folder the person came from,
  and the skill and bundle from the context are preselected. If the machine is *Connected* but has no
  complete scan yet, the person can choose **Wait here** or **Close**; the dialog never opens an
  empty Pin step on a machine that is not eligible. If more than one folder qualifies the person is
  not asked: the preferred one is chosen and the dialog says which, with the other folders reachable
  from the Machines page.

## Words

Plain, exact, one light Yorkshire phrase at most in a heading, lede or success line, as the voice
guide allows. The state labels, token wording and "nothing happens until you run this" stay plain. The
word "verified" never describes a machine or an ownership record. Errors from the token creator and
the controller are passed through only when they are the server's own words; other errors read as the
generic sentence.

## Browser structure

- `web/src/lib/machineJoin.ts` (+ `machineJoin.test.mjs`, added to `npm test`): pure functions.
  `joinCommands(site, name, channel)` returns the chained block from the existing builders;
  `validateMachineName(name)`; `joinState(name, snapshot, machines, now)` returns the table above
  with its reason text; `landingFor(context, machine)` decides the folder to open, or that none is
  eligible yet; `pollDelay` reuses `retryDelay`.
- `web/src/lib/AddMachineDialog.svelte`: the dialog, mounted by `EnvironmentSync` (Machines) and by
  `MachineSkillContent` (Private skills). Props: `{ site, machines, snapshot, accounts, canWrite,
  token, tokenSetup, context, onready, onopenstepbystep, onclose }`.
- `EnvironmentSync.svelte` and `MachineFleet.svelte`: the Machines entry point and the landing
  highlight. `MachineSkillContent.svelte`: the *Not listed? Add a machine* entry and the landing that
  switches the shell to the new folder.
- The snapshot and machine list already refresh on a timer in `EnvironmentSync`; the dialog watches
  those props and also requests a refresh through an `onrefresh` callback at its own back-off pace
  while open and the tab is visible. Reads are fenced by the dialog's lifetime and a generation
  counter, as in the Review UI.

## Safety, accessibility and layout

- Accounts mode only, hidden for legacy auth, hidden for viewers.
- No machine name, token or state in `localStorage`, `sessionStorage` or IndexedDB. The token is
  rendered only by the existing creator and is never put in a command, a URL or the log.
- The dialog does nothing to any machine and sends nothing to the controller except the existing
  token-create request and the refresh reads.
- Native `<dialog>` with focus returned to its opener, a labelled form, a polite live region for the
  status line, keyboard-only operation, reduced motion respected, status shown in text as well as
  colour, and no horizontal page scroll at 320, 375, 768 and 1440px in light and dark.
- Copy buttons announce that the command is on the clipboard.

## Testing

- Node unit tests for `machineJoin.ts`: every state and its reason, name validation edge cases (empty,
  spaces, very long, unicode), the chained command (quoting a name with an apostrophe or spaces, the
  channel, no token inside it), host matching (case-insensitive on ASCII only, as the server), the
  landing decision (same-agent preference, none eligible, more than one eligible), and the back-off.
- A Playwright fixture `web/tests/add-machine.e2e.mjs` run beside the others and added to the
  terminal-browser workflow: opens from both entry points; the three parts; the command block is
  exactly the builders' output and contains no token; the token creator is the existing one; the
  status line walks Waiting → Connected → First scan → Ready as the synthetic snapshot changes; Ready
  hands over and the Private skills dialog opens on the new folder with the skill chosen; no
  eligible folder yet keeps the person in place; a paused connector shows its reason; polling stops on
  close and while the tab is hidden; viewer and legacy auth see nothing; nothing in browser storage;
  layout and theme matrix. Mutation checks cover the landing choice, the "never claim Ready without a
  complete scan" rule and the token-never-in-command rule.

## Delivery

Two reviewable pull requests, each merged when its checks are green, with a broad review before the
last merges. The user has merged early before, so each PR description states plainly what is still
outstanding and the work is not pushed until its review fixes are in.

1. `machineJoin.ts` and its tests, `AddMachineDialog.svelte`, the Machines entry point and its
   fixture, with the docs for the dialog.
2. The Private skills entry point and landing, its fixture cases, and the handoff.

## What this does not establish

That the command worked on the machine, that the machine is healthy, or that its agents are
configured: only that a connector with that name reported in and that a complete recent scan arrived.
Nothing is installed or written by reet.sh. No physical device, screen reader, Safari or Firefox
check is planned beyond the existing Chromium fixtures. The "pin before the machine exists" case, the
in-dialog machine picker and the terminal entry point are future slices.

## Continuation

Candidates after this slice, in the order the product owner chooses: the reviewed executor (stage,
lock, revalidate, journal, rollback, verified `applied` receipts), a fleet overview with status per
machine and per agent, MCP exposure of pins, checks and receipts, and an entry point from the
terminal workspace.
