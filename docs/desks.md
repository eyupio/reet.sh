# Desks

A **desk** is a saved set of terminals for one piece of work: tabs, each split into up to nine panes, each pane a shell
or an agent's terminal on any of your machines. A desk saves itself to your account as you change it and opens the same
in any browser. Each shell pane keeps the folder its shell is in and, on a machine with reet updated, its own command
history, so a shell lost to a restart comes back in the same place with the same history at one press.

A desk's terminals are ordinary sessions: they count towards the session limits and they keep running when the desk is
closed. Sessions lists one only when it needs you; otherwise the desk is where you see it. Opening a desk never starts a shell. A shell starts only from **New tab**, a split,
or a press on a pane (**Reopen**, **Start**, **Reopen all**, **Continue**).

## The Desks tab

**Desks** is a primary tab, after Factory and before Machines, for an account that can start sessions (accounts sign-in,
not a viewer): in the top bar on a desktop and a tablet, and on the phone's bottom bar (Home, Sessions, Factory, Desks,
Machines, More). It is marked as the current page on `/desks` and on an open desk. Jump lists desks too (not templates).

- `/desks` opens the desk you last opened in this browser, if it is still there; otherwise the list. The only thing a
  desk keeps in the browser is that one id (`reet.desks.last.<account>` in local storage).
- `/desks/<id>` opens a desk. `/desks/<id>#<pane>` opens it at that pane (the desk chip in Sessions uses this).
- **The list** shows each desk as a card: its name and colour, its tab and pane counts, the machines it spans, and pills
  for shells that need you, live shells and panes to reopen. **New desk** makes an empty desk named "Desk" (then
  "Desk 2", and so on) and opens it. **From a template ▾** lists your templates. Each card's menu has Open, Rename,
  Duplicate, Move to its own tab, Save as template, Delete and the colour swatches. With no desks the page says what a
  desk is and offers New desk. Your templates follow under a **Templates** heading, as cards of the same kind; a
  template card's name opens the template's own page and its menu has Open, New desk from this template, Rename and
  Delete.
- **A desk** has a header: the desk's name, which is the switcher (your other desks, All desks, New desk, and Move to
  its own tab), a line with its tab and machine counts and the save state, **Reopen all** when it applies, **Layout**,
  **Type in all panes** and the desk menu (Rename, Duplicate, Save as template, Keys, Delete, and the colour
  swatches). Below are the tab bar, one terminal toolbar for the focused pane, and the tab's panes.

**Move to its own tab** opens a desk in a browser tab of its own, leaving the window you are in as it is. Reet keeps one
desk to a window, so this is how two desks stay open side by side: each tab has its address and its own chosen tab, and
both come back as they were after a reload. From the switcher the desk already in this window is switched off, since it
already has this tab; from the list any card can be opened this way. The desk you last opened, which /desks reopens,
follows the newly opened tab.

Deleting a desk with no running shells asks "Its tabs and panes are removed. Nothing is running in it." A desk with
running shells asks whether to **End its shells** or **Leave them running in Sessions**, and says what the first means:
"Its shells are ended and leave Sessions, and their panes' command history is removed." When a shell would not end,
the page says so after the delete: "Sorted. “`<name>`” is deleted. One of its shells could not be ended. It is still in
Sessions." (or how many of them).

The Desks page reads your desks, sessions and machines every 15 seconds while the page is in view, as Sessions does.
If a desk cannot be read when you open it, the page says so and offers **Try again** and **All desks**.

## Tabs

The tab bar shows flat tabs with a colour marker and an underline in the tab's colour on the current one.

- Click to switch. Double-click or F2 renames in place. With a mouse, drag a tab to reorder; a sideways drag with a
  finger or pen pans the tab bar, and Move left or Move right in the tab's menu reorders on a touch screen.
- The tab shown is each window's own: switching tab saves nothing and moves no other window. The window remembers its tab
  for the desk until it is closed; a new window opens on the tab the desk was last worked in.
- The tab's menu (right-click, Shift+F10 or ⋮) has Rename, Move left, Move right, Duplicate tab, Close tab and the
  colour swatches.
- A tab you are not looking at shows a mark when one of its shells is waiting for you.
- **+** opens a menu: **New shell…** (a plain shell on a machine, in a folder you type or browse to, under the home rule;
  it is filled in with the focused pane's machine and the folder that pane's shell is in now, else the pane's last
  folder), **Start an agent…** and **Add a running session…**.
- New tab and a split check the session limits before anything is sent, and say so with the pane card's words
  ("`<machine>` already has 24 sessions running." or "You already have 64 sessions running."). A machine that does not
  answer gives "`<machine>` is not reporting. Try again once it is back." Pressing again after a start that timed out
  asks about the same start, so it never starts a second shell.
- **Duplicate tab** copies the tab's layout, machines, folders, names, colours and start-up commands. Its panes start
  empty, because a session can be in only one pane.

Only the tab on screen has its terminals attached; a hidden tab holds no terminal and no connection, and tmux keeps its
shells running. When a tab opens, its panes attach at most three at a time, the focused pane first.

## Panes, splits and layouts

Each pane has a one-line header washed in its colour (or the tab's): its name, its machine, its folder (shortened in the
middle, in full in the tooltip), its state in a word, and a menu. The focused pane has an accent outline. Focus follows a
click, never the pointer passing over.

The pane menu has Rename, Split right, Split down, Zoom (or Show all panes), Swap with…, Move to tab…, Start-up
command…, Open in Sessions (when the pane has a session) and Close, with the colour swatches.

- **Split right** and **Split down** start a new plain shell on the focused pane's machine, in the folder its shell is
  in now, beside it. A tab holds at most nine panes. A split that would leave either half smaller than 20 columns by 8
  rows is refused: "There isn't room to split this pane."
- **Separators** sit between panes. Drag one, or focus it and use the arrow keys (2% a press), Home and End (to 10% and
  90%); a double-click puts it back at half. A drag stops at the last place where every pane still has 20 × 8 cells. The
  terminals keep their size while you drag and are resized once, when you let go.
- **Combining tabs.** Layout also brings the panes of other tabs into the current one. **Combine tabs…** opens a
  dialog listing the other tabs, all that fit ticked; untick the ones to leave out, choose **Side by side**, **Stacked**
  or **Grid**, and **Combine**. On a tab of one pane, which has nothing to arrange, the menu offers **Show all tabs
  side by side**, **stacked** and **as a grid** instead of the layouts. A tab is taken whole, in tab order, and only
  while the result stays within nine panes; one that does not fit stays as it is and the desk says so. Nothing is
  restarted, the emptied tabs go, and **Move to tab…** in a pane's menu moves a pane back out. Layout is disabled only
  when there is one pane and no other tab that fits.
- **Layout** offers **Side by side**, **Stacked**, **Grid** (as many columns as the square root of the pane count,
  rounded up), **Main and stack** (the first pane on the left at 60%, the others stacked beside it) and **Even out**
  (the same shape, every pane an equal share). A layout rearranges the panes you have; it starts and ends nothing.
- **Zoom** fills the tab with one pane (from a pane's menu, that pane, which also takes the focus). The others stay attached, hidden, so nothing they print is missed; Show all
  brings them back.
- **Swap with…** exchanges two panes' places. **Move to tab…** moves the pane, with its shell, beside the focused pane
  of a tab that has room, in one saved step, so another window never sees it on neither tab. You can also drag a pane by its header onto an edge of another pane; a preview shows the half
  it will take.
- **Theatre view** (the last toolbar button) folds the top bar and the desk header away, leaving the tab bar, the toolbar
  and the panes. It is not saved.
- **The command bar** inside the focused pane's terminal area runs a command in its shell. Empty, it offers what you might do
  next: `cd` into each folder of the folder the shell is in, then the commands you ran lately and the ones you saved.
  As you type it completes the command, and the last word to a folder (`cd w` offers `web/`, and `cd web/` then offers
  that folder's own folders). The arrow keys choose a suggestion and Tab or a press takes it; that only fills the field.
  Enter or **Run** types the line into the shell and presses Enter. Esc goes back to the terminal. It is for shell
  panes: an agent's pane has Compose. A line with a quote or a backslash is not completed.

The layout of every tab, its zoomed pane and its focused pane are saved with the desk.

## Closing a pane or a tab

A pane or tab with nothing running closes at once. Closing the last pane of a tab closes the tab. A pane whose shell has
already ended or was lost is gone for good when it closes: on a machine with `desk-shell-v1` its own command history is
removed, and the session stays in Sessions as it was. One with a running shell asks "Close “`<name>`”? Its shell is still
running on `<machine>`." (a tab asks once for all its running shells) and offers two choices:

- **End the shell** (the default; Enter chooses it): "Stops what is running in it. The session stays in Sessions as
  ended, and this pane's command history is removed." Whether the session then stays in Sessions follows your
  **Terminal history default** (Account → Profile), as Sessions' own End does: with **Remove from history after it
  ends** the line says "The session leaves Sessions, and this pane's command history is removed." instead.
- **Take off this desk**: "It keeps running, and stays in Sessions."

The arrow keys move between the two, and Cancel leaves everything as it was, as does leaving the desk while the dialog
is open. If a shell will not end, the desk says so; it is still in Sessions.

## Names and colours

A desk, a tab and a pane each have a name of 1 to 80 characters (a tab or pane may be left unnamed) and one of the
project colours: none, red, amber, green, blue or violet. Desk names are unique on an account, ignoring case. A desk, tab
or pane name with a control character, a format character (a zero-width space or a direction-changing character, for
example) or a line separator is refused by the controller; it is the rule a session title follows, so a pane's name can
always be its shell's title. A stored name written before this rule has those characters removed when it is read.

Renaming a pane that holds a session renames the session too, so Sessions and the desk agree. If the session's rename
fails, the desk says so and the pane goes back to the session's name.

A pane's colour (or else its tab's) is what Sessions shows for that session, after any colour you chose in Sessions
itself in that browser and before the project's.

## Keys

Every desk key is Alt+Shift and one more key, read by its place on the keyboard so the layout does not change it. They
work anywhere on a desk, in a terminal too, but not in a text field (a rename, a search), and not while a dialog is
open. None of them is a key a shell uses by default. Held down, only the focus arrows repeat.

| Keys | Does |
| --- | --- |
| Alt+Shift+D | Split right |
| Alt+Shift+E | Split down |
| Alt+Shift+arrows | Move focus to the next pane |
| Alt+Shift+Z | Zoom the pane, or show them all |
| Alt+Shift+W | Close the pane |
| Alt+Shift+T | New tab |
| Alt+Shift+[ and ] | Previous and next tab |
| Alt+Shift+1…9 | Tab by number |
| Alt+Shift+K | Command bar |

**Keys** in the desk menu lists them. Alt+Shift+M, the Sessions terminal's maximise, is not a desk key: a desk pane zooms
instead.

## The shared toolbar

One terminal toolbar sits above the panes and acts on the focused pane, so nine panes do not carry nine toolbars: Earlier
output, Compose, Search, Copy, Screen, Select text, Paste, Attach (where the machine takes images), Interrupt, Commands,
Prompts, Skills (for an agent's pane), History and Appearance, then Split right, Split down, Zoom and Theatre view. The
toolbar names the pane it acts on. A pane with no live terminal leaves the terminal's buttons off.

The toolbar holds still as the focus moves between panes. The pane's name has a column of a fixed width and is cut short
with an ellipsis when it is longer (the whole name is in its tooltip and on the pane's own header). Attach and Skills
keep their place on a pane they do not apply to, switched off, with the reason in their tooltip. How much of each
button's label shows follows the toolbar's width alone: every label on a very wide window, the main ones on a wide one,
icons only below that, where the buttons wrap onto a second line if they must. On a desktop it never scrolls sideways.

## Saving

Every change shows at once and is saved after a short pause (400 ms), one save at a time. The header says **Saving…**,
**Not saved yet** (a save failed; it is tried again, waiting 1 s at first and up to 15 s) or **saved**. Anything not
yet saved is sent as the page goes away or is hidden, or when you switch to another desk.

- **Another window.** Every save names the version it was made on. A save that lost to a newer copy (another window or
  browser saved first) is made again on the newer copy: each waiting change is applied to it, and a change that no
  longer applies (its tab or pane is gone) is dropped. When anything was dropped the desk says "This desk was changed in
  another window. Showing the latest." Other windows of the same browser hear of a save at once and read the desk
  again. A change made in another browser shows within about 15 seconds: when the page's regular read of the desk list
  finds the open desk at a newer version, the desk is read again, and any change still waiting here is applied on top
  of it. It also shows when this page is shown again (switching back to its browser tab).
- **A refused save.** A save the controller refuses for good (any refusal but a sign-in, a timeout, a rate limit or a
  newer copy) is not tried again: the waiting changes are dropped, the desk is shown as it is saved, the header says
  **last change not saved** and the desk says "reet couldn't save your last change: `<the controller's reason>`.
  Showing the desk as it is saved."
- **Too large.** A change that would take the desk over 64 KiB is refused before it is made: "This desk is as large as
  it can be. Remove a tab or shorten a start-up command."
- **Leaving with a save waiting.** A change still waiting when you switch desk or go to another part of reet keeps being tried, as
  above, until it is saved or refused.
- **A lost answer.** A save whose answer never came back may still have been stored; when the newer copy turns out to be
  that save, its changes are not applied a second time.

A desk's name and colour are saved straight away rather than through this queue; a stale one is tried once more on the
newer copy.

## What a pane says

A pane is never a blank terminal. Every state but a live shell is drawn in the pane as a card: a tag, a heading, a
sentence and its buttons, the main one first. Over a terminal that is still there (machine offline, open elsewhere) the
card is a band across the top, so the last screen stays in view. `<machine>` is the machine's name and `<folder>` the
pane's folder, `~` for home.

| Tag | Heading | Sentence | Buttons |
| --- | --- | --- | --- |
| Ended | This shell ended. | It was in `<folder>`. A new shell opens there with this pane's command history. | Reopen in `<folder>`, Close pane |
| Lost | `<machine>` restarted, so this shell was lost. | It was in `<folder>`. A new shell opens there with this pane's command history. | Reopen in `<folder>`, Close pane |
| Agent ended | `<agent>` finished in this pane. | Continue the same conversation, or start a plain shell in `<folder>`. | Continue, Start a shell here |
| Lost (an agent's pane) | `<machine>` restarted, so this shell was lost. | as Agent ended | Continue, Start a shell here |
| Folder gone | `<folder>` no longer exists on `<machine>`. | reet won't quietly open somewhere else. | Choose a folder, Reopen in home |
| Folder unavailable | reet couldn't open a shell in `<folder>` on `<machine>`. | reet won't quietly open somewhere else. | Choose a folder, Reopen in home |
| At the limit | `<machine>` already has 24 sessions running. (or: You already have 64 sessions running.) | | Try again |
| Machine removed | This machine was removed from reet. | | Choose a machine, Close pane |
| Machine offline | `<machine>` is not reporting. | The shell may still be running there. reet keeps trying; nothing you typed is resent. | Retry now |
| Open elsewhere | This terminal is open in another window. | A terminal has one keyboard at a time. | Use it here |
| Not continued | reet couldn't read your projects just now, so this conversation was not continued. | | Try again, Start a shell here |
| Not started | `<machine>` · `<folder>` | | Start |

Where the agent's own resume is not on offer, an agent's pane says only "Start a plain shell in `<folder>`." and offers
Start a shell here. A pane on a machine without `desk-shell-v1` shows its terminal with one line under the header:
"Update reet on that machine to keep this pane's folder and history." A template's panes keep only the buttons that do
not start a shell. The table is `PANE_CARDS` in `web/src/lib/desks/paneState.ts`; the component and its unit test both
read it.

**Choose a folder** opens the folder browser on the pane's machine and reopens there; **Choose a machine** lets you pick
a machine and a folder. A folder chosen this way keeps the home rule (below). **Start** opens a pane that has never had a
shell in its folder, or in home when it has none.

## Reopen and Reopen all

**Reopen** starts a fresh plain shell for a pane whose shell ended or was lost, in the pane's last folder, and puts it in
the pane. On a machine with `desk-shell-v1` the new shell loads that pane's own command history. An agent's pane reopens
as a plain shell. Nothing that was running in the old shell is restarted: a restart ends processes, and reet brings back
the place and the history.

Before anything is sent the browser checks the session limits; at a limit the pane says so and offers Try again. If
another window reopened the pane first, nothing is started and the pane shows that shell.

**Reopen all (N)** appears in the desk header when two or more panes, on any tab, are ended or lost shells on a machine
that is still enrolled. It reopens them one at a time and then says "Reopened N." or which could not be reopened and
why. A pane that a try has already found a reason not to reopen (folder gone, at the limit) is left out until you try it
again yourself.

## The live folder

A machine with `desk-shell-v1` reads the folder each running session's shell is in (tmux's `pane_current_path`) on its
regular sweep, about once a second, and once more just before it ends a shell, and reports it when it changes. The
controller keeps it on the session record and on the desk pane that holds the session, so the pane's folder outlives the
shell. The pane's header shows it. This works for any shell and for an agent's terminal; it needs nothing in your shell
set-up.

A folder change never makes your own unsaved desk edits stale: it does not move the desk's version on.

## Reopening outside your home folder

Shells that reet starts in a folder keep the **home rule**: the folder must be inside the machine user's home. A desk
pane is the one exception. It may reopen in a folder outside home, any folder the machine's user can enter, but only
when that folder is the one **the machine itself last reported for that pane's live shell**.

The controller keeps those reports apart from the desk, in `desk_pane_dirs`, because anyone who can write the desk could
write any folder into it. Only a report made while that pane's own shell was live (creating, running or ending), by the
machine the pane names, is kept there; a session that had already ended when it was put on the pane moves no trust. On a reopen the controller sends that folder as the desk pane's own folder only when it
matches the report; every other folder (one you chose, one in a copied desk, one written by an older browser) is sent
under the home rule. The machine then checks the folder exists, is a folder and can be entered; if not, the pane says
**Folder gone**. A folder refused under the home rule says **Folder unavailable**. A folder is never swapped for another
one quietly.

Forgetting a machine (removing it from the machines list) keeps its enrolment, so its shells, their desk panes and what
it reported for them stay. Revoking a machine's enrolment removes its sessions, so in the same step every desk pane
holding one is emptied and what that machine reported for desk panes is removed.

## Per-pane command history

On a machine with `desk-shell-v1`, a plain shell started for a desk pane (New tab, a split, Reopen, Start) keeps its own
history, beside your usual one.

- **Where.** On the machine only, under the machine user's home: `~/.reet/desks/history/<pane>.bash` or
  `<pane>.zsh` (files 0600, folders 0700). The small start-up wrapper reet writes is under `~/.reet/desks/shell/`,
  rewritten each time such a shell starts. Neither the controller nor the browser keeps history.
- **bash.** The shell is started with reet's rc file, which loads your files as a login shell would (`/etc/profile`,
  then the first of `~/.bash_profile`, `~/.bash_login` and `~/.profile`), each guarded so one that fails does not stop
  the shell, then points `HISTFILE` at the pane's file, turns on `histappend` and puts a small function in front of your
  `PROMPT_COMMAND` that runs `history -a` and hands the last command's exit status on, so your own prompt command still
  sees it. Each command is on disk as soon as it has run, so a crash or a restart keeps it.
- **zsh.** The shell is started as a login shell with reet's `ZDOTDIR`, whose `.zshenv`, `.zprofile`, `.zshrc` and
  `.zlogin` load yours from your own `ZDOTDIR` (or home). While each of your files runs, `ZDOTDIR` is yours (unset when
  you have none), so a `~/.zshenv` that sets a default such as `export ZDOTDIR="${ZDOTDIR:-$HOME/.config/zsh}"` is
  followed for the files after it. They then point `HISTFILE` at the pane's file, set `SAVEHIST`
  when yours is not set (to your `HISTSIZE` or 1000, whichever is larger) and turn on `INC_APPEND_HISTORY`. `ZDOTDIR` is
  handed back to yours once start-up is done.
- **Other shells** start exactly as a plain shell does today, with no wrapper. They get the live folder and Reopen, but
  no history of their own.
- **History drawer.** For a desk pane's shell, History shows two groups: **This pane** first, then **This machine**
  (your usual `.bash_history` and `.zsh_history`). The pane's entries take at most half of the drawer's 500 entries and
  of its size, so the machine's history is never crowded out. See [Shell command recall](terminal-command-history.md).
- **What removes it.** **End the shell** (closing a pane with a running shell) removes the pane's history file (and the
  lock and new copy zsh keeps beside it), and again three seconds later in case the shell saved one last command on its
  way out, unless a new shell for that pane has started by then. Closing a pane whose shell had already ended or was
  lost removes it too, and so does deleting a desk with **End its shells**, for its running shells and for those that
  had already ended. **Take off this desk** keeps the file with the shell, which keeps running in Sessions.
- **The sweep.** When reet starts on the machine it removes history files under `~/.reet/desks/history/` that have not
  changed for 90 days and that no session in its registry was started for. It removes only regular files named as a
  pane's history, never follows or removes a symlink, and never stops reet starting.

The history is your shell's own: its size is your `HISTSIZE` and `HISTFILESIZE` (or `SAVEHIST`), and what it saves is
up to your shell.

## The start-up command

A pane can hold a start-up command of up to 2,000 characters (**Start-up command…** in the pane menu). reet never runs
it on its own. After this window reopens or starts the pane's shell, a band above the terminal shows the whole command
with **Run** and **Dismiss**. Run waits until the terminal is connected, then types the command and Enter once. Opening a
desk, or a shell started any other way, offers nothing.

A command with a hidden character (a line break, a control character, a format character such as a zero-width space, or
a direction-changing one) is refused when you save it, and if one is already stored the band says "This command has
hidden characters, so reet won't run it. Edit it first." and Run stays off. The controller also refuses control, format
and line-separator characters in a start-up command or a tab or pane name.

## Type in all panes

**Type in all panes** in the desk header sends what you type in the focused pane to every other pane of the tab that is
live, on screen and connected. It needs two or more such panes; a pane hidden by zoom or reading Earlier output does not
receive.

- **What it sends:** keys you type (and the on-screen keys), your own pastes (keyboard, Paste, the menu) and text sent
  from Compose. Only input made in the focused pane goes to the others.
- **What it never sends:** Run (the start-up command), Commands, Prompts, Skills, Attach, Interrupt and the History
  drawer's Run and Paste to edit stay with the focused pane. Answers the terminal makes to its program are never copied.
- **The first time** in a page's life it asks: "Type in all panes? Everything you type, paste or send from Compose goes
  to all N live panes on this tab until you turn it off or change tab." with Cancel and **Type in all N**. After that the
  button turns it on at once. Nothing about it is saved; it is off whenever a desk opens.
- **While on**, a stripe says "Typing in N panes on this tab. Turns off when you change tab." and every pane the input goes
  to, the one you are typing in included, says **Receiving** in its header (N counts them all).

Before each input goes on, every receiving pane is checked as it is at that moment; if anything is wrong, nothing is
sent to any other pane and the mode turns itself off with a reason:

- you change tab or desk, or the screen becomes a phone (it turns off quietly);
- the panes on the tab change: a pane is split, closed, moved, swapped, zoomed or laid out, here or in another window,
  or a receiving pane opens Earlier output ("Stopped typing in all panes: the panes on this tab changed.");
- a receiving pane drops its connection, its shell ends or it is opened in another window ("… `<pane>` is no longer
  connected.");
- a paste another pane would hold for review, for example several lines into a program that does not take a paste as
  one block ("… `<pane>` would run this paste line by line."), or one it would hold for another reason ("… `<pane>`
  would need to review this paste.");
- a send fails partway ("… only `<panes>` received the last input.", naming the ones that did).

Dragging a separator does not turn it off. It is not offered on a phone or on a template.

## Duplicate and templates

- **Duplicate** (desk menu or card menu) makes "`<name>` copy" with every tab, layout, machine, folder, name, colour and
  start-up command, and no sessions: every pane starts empty, with new pane ids, so it has no history of its own yet.
- **Save as template** makes "`<name>` template" the same way, marked as a template. A template holds no sessions and
  starts nothing. It appears under **From a template ▾**, which makes a new desk from it (named after the template
  without " template"), and as a card in a **Templates** group below your desks, whose menu has Open, New desk from this
  template, Rename and Delete.
- A template opened by its address says "This is a template. Make a desk from it to start shells." with **New desk from
  this template**; its panes offer no Start, Reopen or split.

Templates count towards the 50 desks.

## Adding what is already running

- **Add to a desk…** is in a running session's actions menu on Sessions and on Home (not for a chat). Choose a desk and
  a tab of it, or **A new tab**; the session goes in a new pane beside that tab's focused pane. A session that is
  already on a desk is offered as **Move here**: "It is on “`<desk>`” now. Moving it closes its pane on that desk; a
  tab’s only pane stays, empty." The old pane's neighbours take its space. If the move takes it off the old desk but cannot add it to the new one, the dialog says it is on
  neither desk and still running in Sessions.
- **Add a running session…** in a desk's **+** menu does the same from the desk's side, into a new tab, listing the
  running sessions that are not already on this desk.
- **The desk chip.** Sessions shows the desk a session that needs you is on as a chip after the row's words; it opens
  the desk at that pane.
- **Home and Sessions leave desk sessions out.** A session on a desk is not in Home's Continue working, its planning
  board, its count of open sessions, or Sessions' list; the desk is where you see it. When it needs you (an approval, a
  question, a failure, work to review) it is still in **Waiting for you** and the bell, marked "Desk" and the desk's
  name, and its **Open** opens the desk at its pane; Sessions also lists it, with the desk chip. A template's sessions
  do not count, and a Factory pipeline's session shows as its pipeline. If the desks cannot be read, Home and Sessions
  show every session as they would without desks.

A session can be in only one pane across all your desks.

## Start an agent from a desk

**Start an agent…** in the **+** menu opens the usual Start an agent dialog on the focused pane's machine, with its
folder project when the pane's folder is one. The session it starts goes into a new tab as an agent's pane. A Gaffer chat
cannot be a pane: one started from here is left in Sessions and the desk says so. An agent's pane has no history of its
own. When the agent ends, **Continue** (where the agent's resume is on offer) starts the continued session in the same
pane, with the session's project; if your projects cannot be read just then, a session in a project is not continued
and the pane says **Not continued**.

## On a phone

A phone shows one pane at a time, edge to edge, with the tab bar (it scrolls) and the pane names as chips under it to
switch panes. Splits, separators, Layout, zoom, swap, move and Type in all panes are not offered, and the saved layout is
left as it is for a larger screen. Alt+Shift+arrows step through the panes in order. Every target is at least 44 px.

## Limits

| Limit | Value | Where it is applied |
| --- | --- | --- |
| Desks an account (templates included) | 50 | controller |
| Tabs a desk | 20 | controller and browser |
| Panes a tab | 9 | controller and browser |
| A desk, tab or pane name | 80 characters | controller and browser |
| A start-up command | 2,000 characters | controller and browser |
| A desk's document | 64 KiB | controller |
| A split | 10% to 90% | controller and browser |
| A pane's size | 20 columns by 8 rows | browser |
| Live sessions a machine | 24 | browser only |
| Live sessions an account | 64 | browser only |

The session limits count creating, running and ending sessions, desk panes or not. They are checked by the browser
(here and in Sessions); the controller and machines do not count them.

## Machines without the update

The machine side of desks is the `desk-shell-v1` feature, negotiated on the machine's connection. A controller never
sends a desk field to a machine that did not negotiate it, and such a machine never sends one.

A machine without it can still be on a desk: its panes show their terminals, New tab and splits start plain shells in a
folder where the machine has `shell-dir-v1`, and Reopen opens a shell in the pane's folder under the home rule. What it
does not do: report the live folder (the pane keeps the folder it was opened in), keep per-pane history (History has no
This pane group), open a folder outside home, or remove a pane's history. A live shell pane on it says "Update reet on
that machine to keep this pane's folder and history." A machine too old even for `shell-dir-v1` answers a reopen in a
folder with the same words.

## Where the code is

For anyone changing desks: the page is `web/src/lib/desks/`; the record and its rules are `internal/desk` and
`internal/store/store_desks.go`; the routes are `internal/api/desks.go`; what a machine does for a desk pane is
`internal/terminalhost/desk_shell.go` and `live_dir.go`, under the `desk-shell-v1` feature.

## Routes

Every route needs an account browser session (not a token or a viewer) and, for a write, the same origin; these are the
terminal routes' rules. Every write names the version it read as `expect` and a stale one is answered 409 with the
current desk. All but Reopen still work while remote terminals are turned off on the controller.

| Route | Does |
| --- | --- |
| `GET /api/v2/desks` | Summaries: id, name, colour, template, position, version, tab and pane counts, the session ids and machines each holds. A desk saved by a newer reet is left out. |
| `POST /api/v2/desks` | Makes a desk: `name`, `colour`, `template`, and `from` (a desk to copy without its sessions). |
| `GET /api/v2/desks/{id}` | One desk with its document. |
| `PATCH /api/v2/desks/{id}` | `expect`, and any of `name`, `colour` and the whole `doc`. |
| `DELETE /api/v2/desks/{id}` | `expect` and `endSessions`. With `endSessions`, its running shells are ended and removed from Sessions, each pane's history with them; a shell that had already ended, on a `desk-shell-v1` machine, is sent an end that only removes its pane's history. Answers `left`: the running shells that would not end. |
| `POST /api/v2/desks/reorder` | `ids`: every desk once, in the new order. The list has no control for this yet. |
| `POST /api/v2/desks/{id}/panes/{pane}/reopen` | `expect`, and `home` to reopen in home. Answers the desk and the new session. |

The terminal routes take two desk fields: a create (`POST /api/v2/terminal/sessions`) takes `deskPane`, the pane a plain
shell is for, and an end takes `forgetDesk`, which also removes that pane's history on a `desk-shell-v1` machine, for a
shell that has already ended or was lost as well. The end's own `removeHistory` says, as always, whether the session
leaves Sessions; `forgetDesk` never sets it. An older machine is sent a plain end, which for an ended shell does
nothing.

| Code | Status | Means |
| --- | --- | --- |
| `desk_invalid` | 400 | The request or document breaks a rule (the message says which), a template was asked to reopen, or `deskPane` is not a valid id. |
| `desk_missing` | 404 | That desk isn't on this account. |
| `pane_missing` | 404 | That pane isn't on this desk. |
| `desk_stale` | 409 | Someone changed this desk; the body carries `desk`, as it is now. |
| `desk_full` | 409 | The account has 50 desks. |
| `desk_name_taken` | 409 | Another desk already has that name. |
| `desk_session_taken` | 409 | A session can be in only one pane. |
| `pane_live` | 409 | This pane's shell is still running. |
| `machine_gone` | 409 | This machine was removed from reet. |
| `machine_offline` | 409 | The machine is not reporting. |
| `folder_gone` | 409 | The machine says the folder no longer exists or cannot be entered. |
| `folder_unavailable` | 409 | The machine couldn't open a shell in that folder (for example outside home without its own report). |
| `shell_dir_unsupported` | 409 | Update reet on that machine to keep this pane's folder and history. |

Other refusals from a reopen are the terminal create's. The audit records `desk-created`, `desk-deleted` and
`desk-pane-reopened` (with the folder and whether it was outside home).

The tables are `desks` (one row a desk, its document as JSON), `desk_sessions` (which desk and pane hold each of an
account's sessions, keyed by account and session, so a session is in one pane only) and `desk_pane_dirs` (the folder a
machine last reported for a pane). A desk with a schema newer than this reet's is neither listed nor overwritten, and one
whose stored document cannot be read is left out of the list and logged rather than failing it.

A session removed from Sessions' history (archived there, or ended with its removal asked for) is taken off the desk pane
that held it in the same step, and the desk's version moves on, so a pane never names a session Sessions no longer
shows; the pane then reads **Not started** and offers Start. A reopen that finds the desk changed in another window
starts nothing, and the session it had reserved is removed from history at once.

## Not done and not verified

- **zsh per-pane history is verified on Linux only.** The wrapper's real-shell tests (both wrapper tests, and a
  `~/.zshenv` that defaults `ZDOTDIR` to `~/.config/zsh`) ran on Linux with zsh 5.9. They have not been run on macOS,
  whose `/etc/zshrc` sets `HISTFILE` itself; the wrapper sets the pane's file after it, but that is unchecked there.
- **A `ZDOTDIR` set before the shell starts is not followed.** One exported in the environment the shell starts in, or
  defaulted system-wide in `/etc/zshenv`, is not honoured by reet's wrapper; only one set in your own `~/.zshenv` is.
- **A real machine reboot has not been tested.** Restore after the tmux server ends is tested, on a private tmux
  socket of the test's own with a real bash: the server is ended with the pane's shell in it and started again, the
  shell is reported lost, and a reopen through the route opens a new shell in the pane's last folder with the pane's
  command in its history. A shell killed outright, and one ended and reopened outside home, are tested too. A whole
  machine restarting, with reet's service starting again, is not.
- **After a reboot, a folder on a mount that is not there yet** (a network share, an encrypted volume not yet unlocked)
  reads as **Folder gone**. Reopen again once it is mounted.
- **The last folder can be one sweep stale.** The machine reads it about once a second and once more before it ends a
  shell; a shell lost to a crash or a reboot keeps the folder from the last read, so a `cd` in the second before is
  missed.
- **Downgrading reet on a machine** that has started desk pane shells can stop its terminals: an older reet reads the
  machine's session registry strictly and refuses the desk field a `desk-shell-v1` create left in it, so it cannot open
  the registry. Update reet on that machine again to bring them back.
- **bash 3.2 (macOS).** A brand-new pane's shell shows a placeholder line, `: reet desk history`, on up-arrow or in
  `history` until its first command is saved. Run by mistake it does nothing.
- **bash is interactive, not a login shell.** It loads your login files, but `logout` refuses and `~/.bash_logout` is
  not read. zsh is a login shell.
- **Some history files wait for the sweep.** A controller restart while a pane is being ended forgets that the end
  should remove the pane's history, and a reet restart on the machine within three seconds of the end skips the second
  removal; ending a shell from Sessions after it was taken off a desk never asks. Those files stay until the 90-day sweep
  removes them, and only once the machine's registry no longer holds the session started for that pane.
- **A newline in a folder name** is printed by current tmux as `_`, so the folder is recorded under the wrong name. A
  reopen then opens only if a folder of that name exists, and otherwise says Folder gone; it never opens elsewhere.
- **Shells other than bash and zsh** get the live folder and Reopen only; fish history is not kept per pane.
- **A pane's history follows the pane its shell was started for.** A session added from Sessions has none of its own
  until the pane is reopened; a session moved to another desk keeps writing to its first pane's file.
- **Gaffer chats cannot be panes.**
- **No sharing and no export.** A desk belongs to one account.
- **No splits on a phone.**
- **The desk list cannot be reordered.** Desk order can be set through `POST /api/v2/desks/reorder` only; the list has
  no way to move a card.
- **Reopen all** gives up waiting for a reopened pane after about half a minute and counts it as reopened; the 15-second
  read shows it when it arrives.
- **The session limits** are the browser's; another browser or an older page can start more.
- **Some joined emoji are refused in names.** A desk, tab or pane name (or a start-up command) holding a zero-width
  joiner, as some emoji sequences do (a family, a flag with a modifier), is refused with a message about a control
  character, and such characters are removed from a stored desk when it is read.
- **Removing a session that is still ending** from Sessions empties its pane at once, while its shell may still be
  stopping; the pane then offers **Start**.
- **The tab bar for screen readers.** The tab list holds only the tabs: it claims them with `aria-owns`, and each tab's
  ⋮ button and the rename field sit beside it. This was checked in Chromium's accessibility tree only, not with a screen
  reader.
- **The floating Gaffer button is not shown on an open desk**, so nothing covers a terminal. It is on every other page.
- **Ending an agent from a desk** (End the shell on an agent's pane) does not repeat the warnings Sessions' own End
  dialog gives about a Factory outcome or a worktree.
