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

## Alignment and polish pass (10 November 2026)

The user asked for the widget, the floor and the automation to be checked for
alignment and brought to a professional standard, as reet's main selling point.
An audit of every screen (desktop, phone, dark) found, and this pass fixes:

- **Cards were walls of text.** Every outcome session repeated the seven-line
  standing instruction, plus three separate evidence lines. A card now shows the
  title, state, an **Outcome** chip with the objective (or the goal clamped to
  three lines), a highlighted line when something waits on the person, the latest
  report clamped, and one budget line. The full goal and evidence times moved into
  **Review & evidence**. Nothing was removed from the record.
- **The floor was a 580 px column on a desktop.** `WorkspaceDialog` gained `wide`
  (820 px); the floor uses it. Phones are unchanged.
- **Loose structure.** The floor now has a hero action, then **Outcomes**,
  **Sessions** (filters on one scrolling row; bulk actions and Refresh in the
  heading row) and **Not followed**, each under a heading. An outcome's plan opens
  inside its own row instead of pushing the filters down.
- **The headline action was buried.** An empty general chat now leads with **Give
  Gaffer an outcome** (opens the floor with the dialog up, `startWithOutcome`) and
  keeps **Open the factory floor** beside it. Home's Gaffer line is a pill button
  with his face rather than an underlined sentence.
- **Red error before any input.** The outcome dialog showed "Write the outcome you
  want." in the error colour on opening. Missing input is now muted guidance;
  the error colour is kept for real failures. The dialog also shows its three
  steps (Outcome, Plan, Start).

Accessible names used by the suites are unchanged. `gaffer-outcome.e2e.mjs` adds
the direct widget entry, the steps, guidance styling and the compact card (the
standing instruction hidden until expanded). All Gaffer, Home and attention suites
pass against this build.

Not changed, and worth a decision: the vocabulary still mixes "look after",
"follow", "follow-up" and "goal" across the widget, floor and Home. Unifying it
touches many strings and tests, so it was left for an explicit choice of terms.

## The Factory page and staged pipelines (10 November 2026)

The user asked for the factory to be a first-class use case, a page rather than a
modal, showing work as production pipelines; for Gaffer's enablement to be a
global setting, on by default, with per-session overrides and setup guidance; and
named the typical flow: plan, design, code, review, test, PR, babysit, merge, with
the plan able to say which agent and model does which part.

### Decisions the user made

1. **Staged handoffs.** Each pipeline moves through stages, each with its own agent
   and model, and the controller starts the next stage on a hand-over. Not chosen:
   one session per pipeline with stages only displayed, or page first and engine
   later.
2. **Merge.** Auto-merge is a setting per pipeline, with a global default. (Asked
   as "who merges"; the user answered in their own words.) The global default ships
   **off**: that value is this agent's choice, not the user's stated one.
3. **Pre-approved, one click.** With the global setting on, asking Gaffer to look
   after a session starts at once with the usual goal and limits. Nothing is sent
   to the provider until the person asks. Not chosen: following every session
   automatically, or covering factory pipelines only.

### Contract

- **Stages** (`internal/gaffer/outcome_runs.go`): `StageKinds` is the fixed order
  plan, design, code, review, test, pr, babysit, merge. A planned session with
  `stages` is a pipeline; each stage carries the agent and model the person
  approved. Stages may be left out, never reordered or repeated. A session without
  stages behaves exactly as before.
- **Hand-over.** A review of a pipeline stage's goal is also offered `hand_over`
  (`worker_tools.go`). It takes a summary and nothing else. After the review is
  saved, `stageHandedOver` marks the stage done, closes that stage's goal as
  **stopped** ("handed over", never the person's "completed") and asks the
  controller to advance. An ordinary goal is never offered the tool.
- **Advance** (`advancePipeline`). The next stage keeps the session when the same
  agent and model do it, under a fresh goal and budget; otherwise the controller
  starts a new session in the first stage's working folder (`WorkDir`, the
  worktree in a git project) with the brief, the last handover and the stage's
  instruction. Later stages name no project, so they get no second worktree.
  Every launch still goes through `outcomeStarter`: no YOLO, audited.
- **Merge gate.** The merge stage starts only if the pipeline has `autoMerge` or
  `mergeApproved`. Otherwise the pipeline is `ready_to_merge` and nothing starts.
  `POST /api/v2/gaffer/outcomes/{id}/sessions/{key}/merge {revision, confirmed}`
  is the approval (writer browser, canonical origin; activity
  `gaffer-pipeline-merge-approved`). The merge goal says to merge only when every
  check passes, no review thread is open and there are no conflicts; every other
  stage's goal ends "Do not merge." The working agent merges under its own
  approvals: Gaffer still has no GitHub path.
- **Plans.** `propose_sessions` may list `stages` (kinds only). The parsed plan has
  no field for an agent, model or auto-merge, so the provider cannot set them; the
  crew and the merge tick come from the browser, from the person.
- **Profile.** `gaffer_standing` (default 1) and `gaffer_auto_merge` (default 0)
  on `user_profiles`. `gafferStanding` only changes whether the browser shows the
  review before starting; the controller enforces nothing new from it. Per-session
  override is the existing review, pause and stop on that session.
- **Page.** `/factory` (`web/src/lib/factory/Factory.svelte`), a tab after Home.
  It reads outcomes and goals every five seconds and starts nothing. The floor's
  session sections render inline there (`GafferFollowedSessions inline`); the
  dialog form remains only behind **Chat options → Factory floor**. Home's Gaffer
  line and the chat's entries are links to the page; `?outcome=1` opens the
  outcome dialog.

### Validation

`outcome_runs_test.go`: a pipeline through code (new model → new session in the
worktree, handover in the prompt), review to test (same model → same session,
fresh goal), the merge gate with stale and foreign approvals refused, merge goal
wording, done; auto-merge per outcome and overridden per pipeline; stage order and
agent validation; and a real goal review whose provider calls `hand_over`.
`gaffer_outcomes_test.go` adds the merge route's refusals. Browser:
`gaffer-factory.e2e.mjs` (tab, tiles, tracks, merge approval, setup guidance,
settings), `gaffer-outcome.e2e.mjs` (crew, stages, auto-merge, exact approved
body), `gaffer-floor.e2e.mjs` and `home-followups.e2e.mjs` moved to the page,
`navigation` and `shell-fidelity` updated for the fifth tab.

### Not done, and known limits

- **No real run.** Everything is synthetic. The hand-over depends on Gaffer's
  model judging a stage done from the worker's reports; how well that works with
  real models is untested. A model that never hands over leaves a pipeline at its
  stage until its review budget runs out.
- **The session chat is not one-click yet.** "Look after this session" inside a
  session's Gaffer chat still opens the review; only the Factory list starts at
  once. The user chose one-click for ordinary sessions, so this is owed.
- **Later-stage sessions are not tied to the project** in the session list,
  because they launch in the worktree folder without a project ID.
- A stage that fails to start leaves the pipeline stopped; there is no retry or
  re-plan, and Gaffer does not see sibling pipelines.
- Per-stage budgets are the outcome's one budget, renewed for each stage.
- No token prices: the crew is the person's choice of agent and model; reet does
  not know or compare prices.
- The floor dialog and the page list duplicate some wording; the vocabulary
  ("look after", "follow", "goal") is still mixed.

## Owed items, smarter outcomes, usage and wording (10 November 2026)

After #719 merged the user chose, one question at a time: finish what was owed,
make pipelines smarter, and show cost; **tokens only, no money**, including
remaining budgets; and unify the wording to **look after / goal / following /
needs you**. Auto-merge stays off by default.

### What changed

- **One-click in a session's chat.** With looking-after pre-approved, the
  composer's **Look after** control starts a session that has no goal at once
  with the usual goal and limits (`GafferGoal quick`). **Chat options → Look
  after this session** still opens the review.
- **Later stages listed under the project.** `LinkTerminalSessionProjectFor`
  sets a later stage's session's project when its folder is the project's folder
  or one of its `.reet/worktrees`, on the project's machine. Best effort, after
  the launch; it is not a project launch and makes no worktree.
- **Try again.** `POST …/outcomes/{id}/sessions/{key}/retry {revision}` for a
  pipeline that is failed, unfollowed or cancelled. A step that never started is
  asked for under a new request ID; a session that did start is kept and followed.
- **Usage.** At hand-over a stage keeps Gaffer's reviews and tokens for it
  (`Reviews`, `InputTokens`, `OutputTokens`, `UsageReported`). The page sums
  finished stages, the goal in hand and any report (`pipelineUsage`,
  `outcomeUsage`), shows "not reported" where a count is missing, and shows
  reviews and time left for the stage in hand. No prices and no spend budget.
  **Agent tokens are not known to reet at all**; only Gaffer's.
- **Report across an outcome.** `POST …/outcomes/{id}/review {revision}`
  (`ReviewOutcome`): one request to the outcome's own provider, refused unless
  that provider is enabled and still allowed session text. The evidence is the
  objective, each pipeline's state, handovers and Gaffer's saved reviews, capped
  at 16 KiB; no session is read. Tool `report_outcome{summary, proposals}`;
  proposals have a title, brief, reason and stage kinds only, at most three, and
  one that repeats an existing pipeline is dropped. The report is saved on the
  outcome.
- **Adding pipelines.** `POST …/outcomes/{id}/sessions {revision, confirmed,
  sessions}` (`AddPipelines`): 1 to 5 at a time, ten per outcome, same validation
  as a new outcome. Added pipelines take the outcome's agent where a stage names
  none and never auto-merge. Approving clears the report's proposals.
- **Wording.** State and filter "Needs review" → "Needs you"; "Not followed" →
  "Not looked after"; "Start follow-up" → "Start looking after"; "Pause/Resume
  follow-up" → "Pause/Resume goal"; Home's line reads "Gaffer: N goal(s) need(s)
  you". Internal names (`followups`, file names) are unchanged.

### Validation

Go: retry (new request, stale and non-stopped refused), added pipelines
(unapproved, repeated title and over-limit refused; outcome's agent inherited),
the review (evidence content, saved report, dropped duplicate proposal, nothing
started, refused with session text revoked and no request sent), stage usage at
hand-over, the project link, and the three routes' refusals. Browser:
`gaffer-factory.e2e.mjs` covers usage figures, Try again, the report, proposals
and approval; `gaffer-goal.e2e.mjs` the one-click start; every suite carrying the
old wording was updated. All evidence is synthetic.

### Still not done

- No real run; the hand-over and the report both depend on model judgement.
- Gaffer does not review an outcome on its own schedule: the report is on request.
- Added pipelines cannot choose a different crew in the approval dialog.
- No agent-token figures and no prices, by the user's choice of tokens only.

## A stuck pipeline, YOLO, real models and the Home marker (10 November 2026, after #720)

The owner ran a real outcome on dev.reet.sh and reported four things.

### The pipeline stopped after its first stage (bug)

Evidence from the instance's audit log: the plan session asked for approvals and
was switched to YOLO by hand; the goal showed Needs you with zero reviews. Cause:
`reviewGoal` found `Controls.Waiting`, set the goal to `needs_user`, and nothing
ever looked at it again, so answering the approval did not resume it. With
ordinary approvals almost every pipeline would hit this.

Fix: such a goal is **held**. `GafferGoal.WaitingOnWorker` is set with
`needs_user` (`HoldGafferGoalForWorker`); `DueGafferGoals` also returns held
goals; `reviewHeldGoal` probes the worker without any model request and, once it
is no longer waiting, `ReleaseGafferGoalHold` makes the goal active again within
the ten-active limit, **without moving `LastSeq`**, so the finished work is
reviewed. An expired goal or an ended session ends the hold
(`EndGafferGoalHold`); a pause, stop or resume by the person clears it. Access is
checked by the normal review after the release. The goal's revision does not
change. Also: resuming a **pipeline stage** goal by hand sets `LastSeq` one
behind the worker's progress so Gaffer reviews what is there (ordinary goals
still wait for new progress), and the Factory page now shows the goal's own note
when a pipeline needs you.

The owner's already-stuck pipeline predates the flag: after updating, use
**Review goal → Resume goal** on it.

### YOLO for an outcome (the owner's decision; reverses "never YOLO")

`OutcomeInput.Yolo` → `GafferOutcome.Yolo` → `OutcomeLaunch.Yolo` →
`terminalproto.Create.Yolo`. It is a field of the outcome only: a session or
stage carrying `yolo` is still an unknown field, and neither the plan tool nor
the report tool can set it. The create route refuses it without the account
setting; the broker checks the account, the agent and the machine at every
start and audits each. Added pipelines inherit it. The dialog offers it on the
plan step behind the existing `YoloDialog` with three gates (account, machine,
every crew agent). **Gaffer's instructions to a YOLO agent are acted on without
an approval**; the dialog and the guide say so. Gaffer still has no shell, GitHub
or merge path and never answers approvals.

### Real models, and the Home marker

`GafferModelSelect.svelte`: favourites, the machine's own list
(`GET /api/v2/machines/{id}/models`), reet's suggestions and Other…, for the main
agent and each crew role. The main model applies to roles on the main agent until
a role is given its own. `factoryMembers`/`factoryBadge` in `gafferOutcome.ts`
mark a pipeline's sessions on Home's Continue working rows and planning-board
cards ("Factory · Plan 1 of 8", linking to `/factory`); a failed outcomes read
only omits the marker.

### Validation

Go: a held goal stays held while the worker waits, makes no model request,
resumes and is reviewed once the wait ends, and a pause ends the hold; YOLO off
by default and present on every stage launch when chosen; the route refuses YOLO
without the account setting. Browser: `gaffer-outcome.e2e.mjs` (model lists,
Other…, the YOLO confirmation and what is sent), `gaffer-factory.e2e.mjs`
(Approvals fact), `home-followups.e2e.mjs` (the marker), plus the unit test for
membership. Synthetic only; the hold was not re-run against the owner's
instance.

### Not done

- The Sessions page carries the marker too (added in the same PR): its list rows and the
  session header, from the same `factoryMembers` read in `TerminalWorkspace`.
- A manual Gaffer chat in a pipeline's session is not told about the pipeline's
  stages, so it cannot answer "where are we" (the owner saw this).
- Effort per crew role was added in the same PR at the owner's request:
  `OutcomeStageInput.Effort` → `GafferOutcomeStage.Effort` → `OutcomeLaunch.Effort`
  → `terminalproto.Create.Effort`, validated with `ValidEffort` and
  `ToolTakesEffort`; a stage keeps the previous stage's session only when agent,
  model **and effort** match. The dialog offers it for the main agent and each
  role, from the web catalogue's effort flag, so OpenCode (no launch effort in
  `agentOptions`) shows none even though its live chat has an effort control.

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
