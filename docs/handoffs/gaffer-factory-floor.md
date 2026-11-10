# Gaffer factory floor

9 November 2026. First slice of the user's direction to make Gaffer simpler and
more capable as a controller of an AI agent factory. It resumes from the
[6 November pause checkpoint](gaffer-resume.md). Only reet.sh changes; all
evidence is synthetic.

## Decisions the user made

Asked one at a time with a recommendation, as the checkpoint requires:

1. **Next slice: the factory floor.** One Gaffer view of every running session,
   followed or not. The alternatives offered and not chosen for this slice were
   commanding several sessions from general chat (needs a multi-session grant and
   tool contract), Gaffer launching new workers (crosses the current "no new
   worker launches" boundary) and simplifying per-session setup only.
2. **Start flow: pick several, one review.** Tick any number of running chats,
   review once, confirm once. Not chosen: one pre-filled review per session, and a
   standing consent that skips the review (it needs an account-level permission
   and weakens per-session consent).

## Contract

- **Chat options → Factory floor** is the former Followed sessions job sheet, at
  the same entry points (general chat, session chat and Home's follow-up entry).
  Goal cards, filters, evidence times and nested goal controls are unchanged.
- The floor also reads `GET /api/v2/terminal/sessions` and lists, under **Not
  followed**, each running structured chat that has no saved goal in any state.
  Plain terminals and ended chats are not offered. A session with a goal, even a
  finished one, is managed from its card, which keeps the separate "start a new
  budget" confirmation. A failed session read leaves the followed work in place
  and says so.
- Ticking, **Select all** and opening **Look after these sessions** make no write
  and no provider or worker request. The review names every chosen session, the
  default provider/model, the access that starting grants, one goal and one set of
  limits.
- **Start follow-up** runs the existing reviewed setup once per session, in
  order: read or create that session's conversation with the default provider,
  read its goal pins, then `POST …/goal/setup` with `confirmed: true`,
  `action: "start"` and those exact revisions. Grants, goals and budgets stay per
  conversation. No backend, schema, protocol, MCP or connector change.
- One session failing does not stop the others and is reported with the
  controller's reason. Nothing is retried automatically. A failed session stays
  ticked. An existing session conversation keeps its own provider and model.
- The ten-active-goal limit is counted from the goals read and stated before the
  review; the controller still enforces it at setup.
- The goal and limits last used are kept per account in this browser
  (`reet.gaffer.floorDefaults.<user>`), clamped to 1–48 reviews, 1–168 hours and
  2,000 bytes. The first starting point is the built-in "Ship the pull request"
  prompt, which ends "Do not merge."
- Unchanged boundaries: the working model does the coding, commands and PR work
  under its own approvals; Gaffer has no shell, GitHub-write or merge path, does
  not answer approval cards, enable YOLO or launch sessions.

## Code

- `web/src/lib/gafferFloor.ts`: candidates, room, blockers, result summary and
  saved defaults, with `gafferFloor.test.mjs` in `npm run test`.
- `web/src/lib/GafferFloorReview.svelte`: the combined review and per-session
  start.
- `web/src/lib/GafferFollowedSessions.svelte`: the floor (file name kept).
- `web/tests/gaffer-floor.e2e.mjs`, added to `.github/workflows/gaffer-settings.yml`.

## Validation

`npm --prefix web run check` (unit tests, zero Svelte diagnostics) and the
production build pass. `gaffer-floor` passes four theme/width views plus the two
limit cases; `gaffer-followed`, `home-followups`, `gaffer-goal`,
`gaffer-widget-ux`, `session-end-gaffer` and `stage-follow` pass against the same
build. `gaffer-offline-live.e2e.mjs` was renamed to the new dialog title but needs
the real controller harness and was not run locally; CI runs it. No Go code
changed, so the backend suites were not rerun. Final-head CI and review remain the
publication gate. No real provider, paid call, device or deployment was used.

## Usability pass (10 November 2026)

The user asked for a simpler, more usable UX. Changes, all browser-only:

- **One click to the floor.** General chat's header link under Gaffer's name is
  now **Factory floor** (it was **Follow a session**), and an empty general chat
  offers **Open the factory floor**. Chat options keeps its entry.
- **One way to choose a session.** The separate "Choose a session for Gaffer"
  picker is removed (`GafferSessionPicker.svelte` deleted). Each Not-followed row
  has **Open**, which shows that session with its Gaffer conversation and enables
  nothing, so the general-to-session path is kept on the floor.
- **Less chrome.** The counts line is gone; the filter chips carry the counts
  (shown only when above zero).
- **Home entry without a goal.** Home's Gaffer line also appears when running
  structured chats are not followed ("Gaffer: N running chats not followed"),
  after any review/following/unavailable wording. It reads no extra endpoint:
  Home already holds the sessions and goals. It is an entry, not an attention
  count; the bell, title and Needs you counts are unchanged.

`gaffer-floor.e2e.mjs` covers the header and welcome entries, the Open link, the
counted filters and the Home entry with focus return; `session-end-gaffer`
follows the floor instead of the picker; `gaffer-followed`, `home-followups`,
`gaffer-chat`, `gaffer-goal` and `gaffer-widget-ux` pass against the same build.

## Not done, for the next slice

- No bulk pause, stop or resume, and no per-session goal text in one review.
- The review uses the default provider's own model; a per-session override is
  still made in that session's chat.
- General chat still cannot instruct workers, and Gaffer cannot start sessions.
  On 9 November the user asked for Gaffer to **own an outcome and start and manage
  sessions to meet it**. That crosses the current "no new worker launches"
  boundary and needs its contract agreed (autonomy, limits, where sessions run)
  before code; nothing for it is implemented here.
