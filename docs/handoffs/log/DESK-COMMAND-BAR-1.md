## DESK-COMMAND-BAR-1: a desk has a command bar with next actions and completion (25 November 2026)

The ask was autocomplete for commands in a desk's terminal, and a way to see the next action (cd into a folder). A desk
pane is a real shell, so reet cannot know what is on its prompt. The owner chose a bar that owns its own field over a
popup at the terminal's cursor (which would have to guess the prompt) and over suggestions with no typing.

What changed.

- `web/src/lib/desks/CommandBar.svelte`: one bar under the panes, for the focused pane, as the toolbar is one for the
  focused pane (so no pane resizes when focus moves). A combobox field and a row of suggestions. Arrow keys choose,
  Tab or a press fills the field, Enter or Run sends the line and Enter through the pane's own `actions.send`, Esc
  returns to the terminal. It is off, and says why, for an agent's pane, a pane with no shell, a terminal that is not
  connected and while earlier output is open. A line with a hidden character is not sent.
- `web/src/lib/desks/commandBar.ts` (tested by `commandBar.test.mjs`): what is suggested. Empty: `cd` into up to six
  folders, `cd ..`, then recent and saved commands. Typed: folders that complete the last word, then commands that start
  with the line. Twelve at most.
- No new server code. Folders come from the machine's browse route for the session's `liveDir` (or the folder the path
  being typed names), recent commands from the session's command history, saved commands from the account's library.
  A folder listing is kept for 30 seconds and read 200 ms after the last change, well inside the browse limit (20 in
  10 seconds for a machine).
- `DeskScreen.svelte` draws it, not for a template. On a touch screen with the keyboard open in a terminal it is not
  shown (as the toolbar is not); with the keyboard open in the bar it stays and the desk header folds away.
- `keys.ts`: Alt+Shift+K moves to the bar. `docs/desks.md` says all of this.

Checked: `npm run check` in `web/` (129 test files, svelte-check clean); `desks.e2e.mjs` (a new part: next actions,
Alt+Shift+K, Tab, a folder's own folders, Enter sends once, a press only fills, Esc, the off state) and
`desks-phone.e2e.mjs` against a built UI; the bar looked at in light and dark at 1360 px and at 390 px.

Not done, not checked.

- Only the first page of a folder is read, and only folders are completed, not files (the browse route lists folders).
- A line with a quote or backslash, or a word with `$` or a glob, gets no folder completion, so there is no drilling
  into a folder whose name has a space.
- Command words complete from history and saved commands only, not from the programs on the machine's PATH.
- The bar's Run is not passed on by Type in all panes; it goes to the focused pane only.
- Not tried on a real phone with its keyboard open, nor against a real machine: the suites use synthetic answers.
- The Sessions screen's single terminal has no bar.
