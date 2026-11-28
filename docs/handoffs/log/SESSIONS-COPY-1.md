## SESSIONS-COPY-1: a clearer Sessions page (28 November 2026)

`web/src/lib/sessions/SessionsPage.svelte`: wording first, then four UI changes the owner picked from a list.

Wording.

- First machine: "Enroll a machine and enable terminal access locally..." is now "Connect a machine, then turn on
  terminal access on that machine. Its sessions are listed here." (the heading and button are unchanged).
- No sessions yet says the new session will be listed on the page. The all-on-desks message says a session needing the
  person still shows here. The Needs you filter with no match reads "No session needs you right now."
- Left as they were because tests pin them: "No sessions match.", "No sessions match these filters.", "Couldn't read your
  sessions just now.", "Couldn't refresh. Showing what we last saw.", the Repair link, the Saved commands name, and the
  terminal access refused message (`terminal-workspace.e2e.mjs` matches part of it).

UI.

- **Summary line** under the title: "2 need you · 3 running" in the status colours, or "Nothing is running or waiting."
  It counts every session the page lists. Not a live region (the page reads every 15 seconds).
- **State chips** above Group by: All, Needs you, Running, Finished, Ended, each with a count that follows the search and
  the filters in the panel. Needs you is always shown; the others only while they hold something or are chosen. The
  State select is gone from the panel, and the Filters & sort count no longer includes the state. Chips scroll sideways
  on a phone.
- **More filters**: the panel keeps Agent, Machine and Sort; Colour, YOLO only and Worktree only are in a native
  `<details>`, with a count beside it.
- **Projects fold** (desktop only; the phone rail is unchanged): the heading is a button, `aria-expanded`, closed on each
  visit and not stored (the page keeps only the Group by choice in browser storage, and a test says so). With no session
  to list the projects are open and the heading is plain text.

The summary line and the chips' counts overlap when everything running is also the whole list; the line earns its place
when something needs the person. Drop one if it reads as noise.

`terminal-workspace.e2e.mjs` asserted the first Sessions row at y < 400 at 1360 x 900; the summary line and chips put it at
415, so the bound is 500 (still above the fold). CI found this; the local run of the page's own suites did not.

Tests: `sessions.e2e.mjs` follows (chips, More filters, the fold, the panel count of 3 where it was 4, and a new
section "summary line and state chips"). Ran against a built UI: sessions, add-project, add-project-edit, projects,
first-machine, device-push, sessions-phone, phone-zoom, phone-dialogs, supporting-nav, navigation, loading-width;
`npm run test` and `svelte-check` pass. Seen at 1440, 834, 390 and 320 px in light and dark.
