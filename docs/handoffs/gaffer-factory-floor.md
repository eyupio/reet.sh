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

## Outcomes: Gaffer plans, the person approves, sessions start (10 November 2026)

The user asked for Gaffer to **own an outcome and start and manage sessions to
meet it**, and chose **plan, approve, then run** over full autonomy within a
budget, grouping only, or deferring. This deliberately crosses the earlier "no new
worker launches" line in [work-orchestration-handoff.md](../work-orchestration-handoff.md),
on the user's instruction, in the narrowest way that does it.

### Contract

- **Planning** is `POST /api/v2/gaffer/providers/{id}/plan` with
  `{revision, objective, project, sessions}` (`internal/gaffer/outcomes.go`,
  `internal/api/gaffer_settings.go`). Writer browser account only, same origin
  and rate/concurrency gates as a connection test. The provider receives the
  typed objective and project name and one tool, `propose_sessions`. No reet
  context is read, so no context grant or provider session permission is needed
  or changed. Nothing is saved: no conversation, goal, grant or session. The
  activity log records `gaffer-outcome-planned` with the provider and count.
- The reply must be exactly one complete `propose_sessions` call with 1 to the
  asked number of sessions (at most 5), unique valid titles, and prompts that pass
  the launch prompt rules (at most 8,000 bytes). Prose, another tool, extra
  fields, too many sessions, a cut-off answer or malformed JSON are refused with a
  message that offers writing the plan by hand. Model text only ever becomes
  editable titles and prompts.
- **Approval** happens in the browser (`GafferOutcome.svelte`). Until **Approve
  and start**, the plan request is the only write. On approval the browser starts
  each session with the ordinary `POST /api/v2/terminal/sessions` (structured
  chat, the chosen project and agent, the edited prompt, no YOLO, a fresh request
  ID each). Ownership, capability, project and worktree isolation checks and the
  launch audit are the existing ones; Gaffer's model chooses no machine, folder,
  agent or flag.
- **Following** reuses the reviewed per-session setup: create the session's
  conversation, wait up to about 30 seconds for the new chat's message channel,
  then `goal/setup` with `confirmed: true`. The goal text is
  `Outcome: <objective on one line>`, `This session's part: <title>`, then a fixed
  instruction ending "Do not merge." (`outcomeGoal` in `gafferOutcome.ts`).
- **Grouping** is derived, not stored: goals whose first line is `Outcome: …`
  group on the floor by that line. There is no outcome record, schema, migration
  or backup change. Editing a goal's first line moves it out of the group.
- Each failure is per session and reported: not started (launch refused), or
  started but not followed (it then sits under Not followed). Nothing is retried.
- Unchanged: Gaffer cannot merge, run commands, answer approvals, enable YOLO or
  message a session outside its own goal. The ten-active-goal limit applies.

### Validation

`internal/gaffer/outcomes_test.go` (fake OpenAI-compatible provider): the plan
from the typed objective only, no conversation or provider change after planning,
six input refusals and a stale or foreign provider all before any provider
request, and ten malformed or partial replies refused.
`internal/api/gaffer_plan_test.go`: foreign origin, signed-out and API-token
refusals, strict body, and a successful plan that saves nothing.
`web/src/lib/gafferOutcome.test.mjs` covers goal text, grouping, blockers and
summaries. `web/tests/gaffer-outcome.e2e.mjs` (in the Gaffer browser workflow)
drives the whole flow on desktop and phone: gates, the exact plan and launch
requests, no write before approval, waiting for a fresh chat, a refused launch, a
started-but-unfollowed session, grouping on the floor, a model that cannot plan
and a hand-written plan. All evidence is synthetic: no real provider, machine,
agent or paid call was used, and a real launch-then-follow has not been run.

### Limits to know before building on this

- The browser is the orchestrator for the start: closing it mid-run leaves later
  sessions unstarted. A controller-owned outcome record would fix that and is the
  natural next slice, together with cross-session awareness.
- Gaffer reviews each session against its own part; it does not see sibling
  sessions, re-plan, or add or retire sessions after approval.
- Agents that need plugin trust at launch are refused with the controller's
  reason. Model and effort are the agent's defaults.
- Outcome identity is the goal's first line, so two outcomes with the same
  objective text merge into one group.

## Controller-owned outcomes, review fixes and floor controls (10 November 2026)

After #713 and #715 merged, the user asked for all three offered follow-ups: an
independent review, controller-owned outcomes, and the smaller floor improvements.
This section supersedes the "browser orchestrates the start" parts of the outcome
contract above.

### Independent review

A fresh-context review of `5f973bf..4d68d6b` found no problem in the plan
endpoint's gates, model-text handling, HTML escaping or the no-YOLO/approval
path. It confirmed these defects in the browser-run start, all fixed here:

1. A create the machine refused (state `lost`/`ended`) was shown as started. The
   controller's launch adapter now reports it as not started.
2. A fractional limit (1.5 reviews) passed the client check, started every
   session, then failed every goal setup. Limits must now be whole numbers in the
   browser (both reviews), and the create body is strictly decoded as integers.
3. A 60-second browser timeout was reported as "Not started" with no idempotent
   retry. The controller now owns the launch and retries only under the saved
   request ID.
4. The browser accepted prompts the launch refuses (leading dash, tabs, control
   characters). Tabs and Windows line ends are normalised; the rest block approval;
   the controller validates the whole plan before anything starts; model plans are
   normalised the same way.
5. A misleading "Review Worker messages and Context sharing" reason was shown for
   a chat that was not ready. Both paths now state the real cause.
6. Closing the dialog mid-run silently abandoned later sessions. The controller
   now carries on.

### Contract

- `POST /api/v2/gaffer/outcomes` saves an approved plan: `{objective, projectId,
  tool, model, trust, sessions:[{title,prompt}], maxReviews, hours, confirmed}`.
  Writer browser account, canonical origin, and remote sessions enabled. The whole
  plan is validated first (project owned, reviewed chat tool, 1–5 unique valid
  titles, launch-valid prompts, whole-number limits, an enabled default provider);
  a refused plan saves and starts nothing. `GET /api/v2/gaffer/outcomes`,
  `GET …/{id}` and `POST …/{id}/stop {revision}` read and stop. Activity log:
  `gaffer-outcome-approved`, `gaffer-outcome-stopped`.
- Record: table `gaffer_outcomes(id, user_id, created_at, data JSON)`
  (`internal/store/store_gaffer_outcomes.go`). Outcome state `starting`, `settled`
  or `stopped`; session state `waiting`, `starting`, `following`, `unfollowed`,
  `failed`, `cancelled`. Changes are revision-checked, so the person's stop and
  the runner never overwrite each other. Thirty kept per account; three may be
  starting at once.
- Runner (`internal/gaffer/outcome_runs.go`): a supervisor beside the goal
  supervisor, under its context and joined by `Close`. It starts nothing while the
  store is fenced, the worker channel is disabled or no launch path is installed.
  Per session it re-reads the account (a person who lost write access starts
  nothing more), launches through `SessionStarter`, creates the session's
  conversation, waits up to 60 seconds for setup to be permitted, then calls the
  existing `SetupGoal` with the person's approval as its confirmation. The goal
  text is built by `OutcomeGoal` and still starts `Outcome: …`.
- Launch (`internal/api/gaffer_outcomes.go`, `outcomeStarter`): the same project
  check, worktree isolation, broker rules and audit as a browser launch, with the
  machine, folder and agent taken from the approved outcome. `Yolo` is never set.
  `Trust` is only the person's own tick. Refusals carry the launch handler's own
  words.
- Shutdown and restart: a launch in flight at shutdown is left `starting` and
  retried under the same request ID, which the broker answers with the session it
  already made. A session that was started but not yet followed is followed, not
  started again.
- Restore: `store_restore.go` marks any `starting` outcome `stopped`, so a restored
  copy starts nothing.
- The floor reads the records (`floorOutcomes`): planned sessions, what is still
  starting, and the goals of the sessions the record started. Goals that only name
  an outcome in their text (made by #715) still group by that line.
- Bulk **Pause all following** / **Stop all goals** act on the sessions shown, one
  goal at a time against a fresh read, with stop confirmed. No bulk resume: resume
  needs the person's per-session check.

### Validation

`internal/gaffer/outcome_runs_test.go`: two approved sessions started and followed
with no browser and no provider call; a refusal reported while the next carries
on; fourteen refused plans and a viewer start nothing; stop leaves the unstarted
unstarted; restart retries the in-flight launch under its own request ID; a
stopped copy starts nothing; the goal fits the limit. `internal/api/
gaffer_outcomes_test.go`: origin, signed-out, API-token, strict-body (fractional
limit, `yolo` at either level) and remote-sessions-off refusals.
`gaffer-outcome.e2e.mjs` and `gaffer-floor.e2e.mjs` cover the browser side,
including closing while it starts, stopping, trust, model, and bulk pause/stop
with a partial failure. Go race runs for `internal/gaffer`, and the Gaffer and
restore selections of `internal/api` and `internal/store`, pass locally.

Still synthetic: the launch adapter itself (`outcomeStarter`) has no test against
a real broker and machine here, so a real start-and-follow remains the first thing
to try after deploying.

## Not done, for the next slice

- No bulk resume (deliberate), and no per-session goal text in one review.
- The review uses the default provider's own model; a per-session override is
  still made in that session's chat.
- General chat still cannot instruct workers.
- Gaffer still reviews each session on its own: no cross-session report, and no
  re-planning (proposing an added or replacement session for approval).
- No per-session model or effort; one model applies to the whole outcome.
- A failed or unfollowed session of an outcome cannot be retried from the outcome;
  start it again or look after it from the floor.
