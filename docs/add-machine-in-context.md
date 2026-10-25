# Add a machine from where you are working

When the machine you want isn't listed, you can bring it in without leaving the screen you are on.
The **Add a machine** dialog (`web/src/lib/AddMachineDialog.svelte`) shows one block of commands to
paste on that machine, watches the controller for it, and carries on where you started: back on the
Machines page with its card marked, or in the Private skills dialog on the new machine's matching
folder. Nothing on screen says more than the controller has been told. The design is in
[the add-a-machine spec](superpowers/specs/2026-10-25-add-machine-in-context-design.md).

## Purpose and non-goals

The dialog gets one named machine from "not listed" to "reporting in", using today's enrolment. It
reuses the existing command builders and the existing machine-token creator, and reads the same
machine list the page already shows.

It does not log in to the machine, install anything, run anything or write to it: the person pastes
the commands there. It has no pairing codes, no SSH from the controller and no "pending machine"
record. The *Allow this machine to apply sync plans* option is not offered, because applying is not
wired. It is shown only in accounts mode, to someone who can write. Terminal sessions and controls,
the deferred security/PIN preference and the legacy sync flows are untouched.

## The three parts

The dialog is one screen with a fixed header, a scrolling body and a footer.

1. **Name it.** One field, *Machine name*, with the names already known offered as hints. The name is
   trimmed. It is refused when empty, when longer than 200 bytes (accented letters and emoji take more than one), or when it holds a control
   character or a text-direction override (a pasted newline or tab, for instance), each with its own
   plain reason. A reason shows only after the field has been touched. The last name typed is
   offered again if the dialog is closed without using it and reopened on the same page, held in memory only.
   A name saved as an alias after a rename or merge is resolved to its saved machine: the status line
   then says "You typed \<alias\>; reet.sh saved it under \<machine\>." (or "The machine will appear as
   \<machine\>." while it waits), the watch follows the saved machine, and **Use** names that machine.
   The commands keep the name as typed, because the server does the resolving.
2. **Give it a key.** The page's existing machine-token creator, with the same words: the token is
   shown once, is never stored in the browser, and is pasted when the CLI asks for it. Where the
   instance has no token creator, the same plain sentence as the step-by-step setup is shown. A
   failed or missing token never hides the commands; the dialog says the machine will ask for a
   token.
3. **Run it.** Once the machine list has been read, one block with a *Copy commands* button, made of
   the three existing builders in order (install, save the connection, enrol and run), separated by
   a blank line, so there is one paste and one prompt for the token. The name is quoted by the
   builders. The block is never given the token, and the button copies only the block. Beneath it:
   "Nothing happens on that machine until you run this there. reet.sh never logs in to it." The
   release channel follows the server build, as the step-by-step setup does. *Prefer the
   step-by-step setup?* closes the dialog, opens the existing setup section and fills its machine
   name with the same name.

## The status line

A polite live region under the three parts. Every state has a text label as well as a colour. The
logic is `web/src/lib/machineJoin.ts`.

| State | Label | Means |
| --- | --- | --- |
| Not ready to judge | Choose a name to start watching. | No valid name yet. |
| Loading | Machines are still loading. | The machine list has not been read once, so nothing is claimed about the name and *Use* is not offered. The line adds that the step-by-step setup still works if the list never loads. |
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

What was already there (the connector ids and the newest complete supported scan) is captured from
loaded data when the confirmed name changes, and not on every refresh. If the machine list has not
loaded, nothing is captured and no state is shown. The state is asked against the clock as it is at
that moment, never a time held from earlier. The 14-day window is a coarse eligibility rule, not a
freshness claim.

## Polling, the watch cap and sign-out

While the dialog is open and the tab is visible it asks the page to read the machines again, first
after three seconds, then at the same pace, backing off to six, twelve and at most twenty seconds
while reads keep failing and returning to three once one works. It pauses while the tab is hidden
and reads at once on return, and when the browser comes back online. After fifteen minutes of
waiting for one name (a new name starts the wait again), counted while the tab is open or hidden,
with one read when you come back, it stops and shows **Still waiting. Check again.** with a
*Check again* button. The same single read happens when the browser comes back online. A lost
sign-in stops everything at once, says "You've been signed out. Sign in again to continue.", and
hands over to the page's sign-in card. Closing the dialog (the button or Escape) stops everything.
Reads are fenced by the dialog's lifetime and a generation counter, so one that finishes late is
ignored. Nothing about a machine name, token or state goes in browser storage.

## Entry points

- **Machines page.** An **Add a machine** button beside **Connect a machine** in the machine list.
  Before any machine is listed, or while the first read is still under way, it sits in the section
  header instead. It carries no context.
- **Private skills dialog.** A **Not listed? Add a machine** link, under the dialog's opening line
  in the Pin step (so it is in view without scrolling, including at 320px). It is not shown in the
  Check step. Pressing it closes the Private skills dialog and opens this one with a context: the
  skill the dialog was opened for (if it was opened from a skill's row), the library skill the Pin
  step had chosen, and the agent of that folder. It is disabled while a save or removal is under
  way.

Viewers and legacy sign-in modes see neither: the page gives the dialog to a signed-in writer on the
Machines view only, and the Private skills dialog checks that again before showing its link.

## The landing

Choosing **Use \<name\>** closes the dialog and carries on where the person started.

**From the Machines page** it scrolls to that machine's card, marks it with a border highlight that
fades over six seconds and the text "Just added", and gives focus back to the button that opened the
dialog. With reduced motion the page scrolls at once and the highlight is a still ring that does not
fade. The highlight clears after eight seconds. If the Machines page is filtered to a different
machine through `?machine=`, the filter is cleared first (other query parameters and the hash stay)
so that the new card exists to be shown; if the page is already filtered to the machine just added,
the filter stays.

**From the Private skills dialog** the folder is chosen by `landingFor` at the moment **Use** is
pressed (or the automatic continue fires), from the machine and connectors as the page holds them
then, never from an earlier look. A folder qualifies only if the Private skills dialog would let a
pin be saved on it: a supported agent, a complete scan in the last 14 days, a running connector, and
a signed-in writer. The folder of the same agent as the one the person came from is preferred;
otherwise the first supported one. The Private skills dialog then opens on that folder with the
skill chosen (by name; the library bundle is not carried across), and a note in its status line:

- "Opened on Claude Code on rack."
- "Opened on Claude Code on rack. 1 other folder also qualifies; open it from the Machines page."
- "Opened on Shared agents / Codex on rack. 2 other folders also qualify; open them from the
  Machines page."

The person is not asked to choose. Focus goes to the new dialog's heading. The note is put into the
dialog's status region a moment after the region is on the page, so that a screen reader has a
region to announce it from. The page hands the folder to the machine list once and then clears it,
so redrawing the list does not open the dialog again.

The machine list judges the folder once more at the instant it opens the dialog, against the
machines and the clock as they are then. If the folder no longer qualifies (a scan has aged past 14
days, or the connector has paused, between **Use** and the page drawing the landing) nothing opens
and the page says "Private skills can't open on that folder right now." When the dialog is closed,
focus returns to the Private skills button of the chosen folder, whose list is opened so the button
is on screen. If that button is not drawn (the Machines page is filtered to another machine, so the
new machine's card is not shown), focus returns to the **Add a machine** button instead, never to
the page.

**When there is no folder to open** (the connector is paused, the only scan is older than 14 days or
incomplete, the agent folders are scan-only, or nothing is listed for the machine yet) the person
stays in this dialog. It shows "Private skills can't open on \<name\> yet." with the reason, and two
buttons: **Wait here** (the dialog keeps watching, as it always does, and says so) and **Close**.
The panel appears on its own once a connector has reported in but nothing qualifies, and when
**Use** is pressed and nothing qualifies. The dialog never opens the Pin step on a folder that
fails those rules.

**The automatic continue.** Only when the dialog was opened from a Private skills dialog that named
a skill, and only while the state is *Ready* and a folder qualifies, the dialog says "Opening the
Private skills dialog for \<skill\> in 3 seconds." with a **Cancel** button, and then does what
**Use** does (so the folder is chosen again at that moment). One timer serves it. It is cleared when
the dialog closes, when Cancel is pressed, when the name changes, and when the state leaves *Ready*.
After Cancel it stays cancelled for that name and **Use** carries on when pressed. From the Machines
page nothing continues by itself.

**Closing without a landing.** Focus returns to the control that opened the dialog: the Add a
machine button, or the Private skills button or skill row that opened the Private skills dialog.

## Known gaps

- If an old machine and a new enrolment share one name, the old machine's newer scan can make the new
  one look Ready; choose a distinct name.
- An alias whose machine has been forgotten is not visible to the page (see *Name it*), so the dialog
  waits for the typed name until the fifteen-minute cap.
- The library bundle the Private skills dialog had chosen is carried in the dialog's context but is
  not used: the landing finds the library skill by the skill's name and does not preselect the bundle.
- When several folders of the new machine qualify, one is opened and the note counts the others; the
  person is not offered a choice.
- Landing from the Private skills dialog on a Machines page filtered to another machine
  (`?machine=`) leaves the new card hidden; clear the filter to see it. Focus is kept safe.
- The pause before the automatic continue is three seconds and is not adjustable.

## What this does not establish

That the commands worked on the machine, that the machine is in good shape, or that its agents are
configured. *Connected* and *Ready* say only that a connector with that name reported in and that a
complete recent scan arrived. The landing note says only which folder was chosen and how many others
qualified. The words "verified" and "healthy" are never used of a machine. No request is made to the
machine itself. No physical device, screen reader, Safari or Firefox check was made beyond the
Chromium fixture (`web/tests/add-machine.e2e.mjs`, run with the other fixtures in the
terminal-browser workflow). That fixture also holds a layout matrix for the dialog and the landing
note; what it checks, and what an accessibility scan found, is in
[the UI verification record](ui-verification.md#add-a-machine-in-context-layout-focus-and-motion-matrix--25-october-2026).

## Continuation

Candidates after this slice are the reviewed executor, a fleet overview, MCP exposure of pins, and
an entry point from the terminal workspace; none is started here.
