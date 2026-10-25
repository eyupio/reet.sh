# Add a machine from the Machines page

When the machine you want isn't listed, you can bring it in without leaving the page. The
**Add a machine** dialog (`web/src/lib/AddMachineDialog.svelte`) shows one block of commands to paste on
that machine, watches the controller for it, and takes you back to the Machines page with its card
marked. Nothing on screen says more than the controller has been told. The design is in
[the add-a-machine spec](superpowers/specs/2026-10-25-add-machine-in-context-design.md).

## Purpose and non-goals

The dialog gets one named machine from "not listed" to "reporting in", using today's enrolment. It
reuses the existing command builders and the existing machine-token creator, and reads the same
machine list the page already shows.

It does not log in to the machine, install anything, run anything or write to it: the person pastes
the commands there. It has no pairing codes, no SSH from the controller and no "pending machine"
record. The *Allow this machine to apply sync plans* option is not offered, because applying is not
wired. It is shown only in accounts mode, to someone who can write. Terminal sessions and controls, the
deferred security/PIN preference and the legacy sync flows are untouched. This slice opens the dialog
from the Machines page only; the Private skills entry and landing are a later slice.

## The three parts

The dialog is one screen with a fixed header, a scrolling body and a footer.

1. **Name it.** One field, *Machine name*, with the names already known offered as hints. The name is
   trimmed. It is refused when empty, when longer than 200 characters, or when it holds a control
   character or a text-direction override (a pasted newline or tab, for instance), each with its own
   plain reason. A reason shows only after the field has been touched. The last name typed is
   offered again if the dialog is closed without using it and reopened on the same page, held in memory only.
2. **Give it a key.** The page's existing machine-token creator, with the same words: the token is
   shown once, is never stored in the browser, and is pasted when the CLI asks for it. Where the
   instance has no token creator, the same plain sentence as the step-by-step setup is shown. A failed
   or missing token never hides the commands; the dialog says the machine will ask for a token.
3. **Run it.** Once the machine list has been read, one block with a *Copy commands* button, made of the three existing builders in order
   (install, save the connection, enrol and run), separated by a blank line, so there is one paste and
   one prompt for the token. The name is quoted by the builders. The block is never given the token,
   and the button copies only the block. Beneath it: "Nothing happens on that machine until you run
   this there. reet.sh never logs in to it." The release channel follows the server build, as the
   step-by-step setup does. *Prefer the step-by-step setup?* closes the dialog, opens the existing
   setup section and fills its machine name with the same name.

## The status line

A polite live region under the three parts. Every state has a text label as well as a colour. The logic
is `web/src/lib/machineJoin.ts`.

| State | Label | Means |
| --- | --- | --- |
| Not ready to judge | Choose a name to start watching. | No valid name yet. |
| Loading | Machines are still loading. | The machine list has not been read once, so nothing is claimed about the name and *Use* is not offered. |
| Waiting | Waiting for it | No connector with that name has reported in. |
| Connected | Connected | A connector with that name is enrolled but paused; the reason is given. |
| First scan | Waiting for its first scan | A connector is running, but no complete scan of a supported agent folder, newer than any the name already had, has arrived in the last 14 days. |
| Ready | Ready | A connector is running and a complete scan of a supported agent folder, newer than any the name already had, has arrived in the last 14 days. *Use \<name\>* is offered. |
| Already here | A machine called \<name\> is already here. | The name belonged to a machine that was already reporting when the name was confirmed. *Use \<name\>* is offered if that machine is running with a recent scan; otherwise the reason is given. |
| Unknown | Something changed. Check again. | Anything the logic does not recognise. |

*Already here* is sticky: a healthy machine's last-seen time moves with every heartbeat, so a moving
time never turns it into *Ready*. It ends only when the person picks another name, or a connector
that was not there when the name was confirmed appears under it (a new enrolment), after which the
normal states apply.

What was already there (the connector ids and the newest complete supported scan) is captured from loaded
data when the confirmed name changes, and not on every refresh. If the machine list has not loaded, nothing
is captured and no state is shown. The state is asked against the clock as it is at that moment, never
a time held from earlier. The 14-day window is a coarse eligibility rule, not a freshness claim.

## Polling, the watch cap and sign-out

While the dialog is open and the tab is visible it asks the page to read the machines again, first
after three seconds, then at the same pace, backing off to six, twelve and at most twenty seconds
while reads keep failing and returning to three once one works. It pauses while the tab is hidden and
reads at once on return, and on coming back online. It stops after fifteen minutes of waiting
for one name (a new name starts the wait again), counted while the tab is open or hidden, with one read when you come back, and shows **Still waiting. Check again.** with a
*Check again* button. A lost sign-in stops everything at once, says "You've been signed out. Sign in
again to continue.", and hands over to the page's sign-in card. Closing the dialog (the button or
Escape) stops everything. Reads are fenced by the dialog's lifetime and a generation counter, so one
that finishes late is ignored. Nothing about a machine name, token or state goes in browser storage.

## The landing

Choosing **Use \<name\>** closes the dialog, then on the Machines page scrolls to that machine's
card, marks it with a border highlight that fades over a few seconds (a still border with reduced
motion) and the text "Just added", and gives focus back to the button that opened the dialog. The
highlight clears after eight seconds. If the Machines page is showing a single machine through its
`?machine=` filter, there is no other card to show and nothing scrolls.

## Entry point

An **Add a machine** button beside **Connect a machine** in the machine list. Before any machine is
listed, or while the first read is still under way, it sits in the section header instead. Viewers and
legacy sign-in modes do not see it.

## Known limitation

If an old machine and a new enrolment share one name, the old machine's newer scan can make the new one look Ready; choose a distinct name.

## What this does not establish

That the commands worked on the machine, that the machine is in good shape, or that its agents are
configured. *Connected* and *Ready* say only that a connector with that name reported in and that a
complete recent scan arrived. The words "verified" and "healthy" are never used of a machine. No
physical device, screen reader, Safari or Firefox check was made beyond the Chromium fixture
(`web/tests/add-machine.e2e.mjs`, run with the other fixtures in the terminal-browser workflow).

## Continuation

The Private skills dialog's *Not listed? Add a machine* entry and its landing on the new machine's
matching folder come next; the dialog's `context` and `onready` already carry what that needs.
