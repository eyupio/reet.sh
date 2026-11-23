# Reet central-control implementation handoff

## SKILL-BUDDY-FOLD-2: skill suggestions stay folded in Chat (23 November 2026)

Follow-up to **PHONE-SKILL-BUDDY-1**: the folded header was still a card above the composer, kept **Pause** beside it,
and a desktop opened the panel in full.

**What changed.** `web/src/lib/terminal/ChatPane.svelte` passes `compact` to `SkillBuddy` on every screen and no longer
tracks `PHONE_SCREEN`. `web/src/lib/SkillBuddy.svelte` gains a folded state: no card, no second row, a 14px muted
label, the first suggestion's name, and **See skills** beside it. **Pause** and the generic "Suggested from the task
and stage" line appear only when the panel is open or paused. The folded strip keeps the 14px floor that
`web/src/lib/typeScale.test.mjs` enforces.

**Unchanged.** A Factory stage's per-stage suggestions in `PipelineDetail.svelte` stay open; the native terminal's
**Suggested skills** dialog is unchanged; **See skills** gives the same panel as before.

**Tests.** `web/tests/calm-session.e2e.mjs`, `web/tests/chat.e2e.mjs` (which already covered the folded phone strip),
`web/tests/skill-buddy.e2e.mjs`, `npm test`, `npx svelte-check` and `npm run build` pass; screenshots at 1440 and 390
in both themes show the one-line strip.

**Docs.** `docs/skill-buddy.md` and `CHANGELOG.md` say the panel folds in Chat.

**PR.** Opened against `main` from `reet/fold-skill-suggestions`.

## DESK-OWN-TAB-1: a desk opens in a tab of its own (23 November 2026)

A small desktop-only affordance, no data-model or autosave change. Reet has no app-level tab that can hold a desk: the
top bar is fixed navigation, and a desk's own tabs hold panes. What reet does have is one desk per browser window, and
its tests already call the browser window "a tab for a desk" (`web/tests/desks.e2e.mjs`). So "its own tab" is the
desk's own browser tab: `/desks/<id>` opened with an ordinary link in a new tab, which leaves the current window's
desk and its unsaved edits alone.

**Where.** The switch-desk dropdown in `web/src/lib/desks/DeskScreen.svelte` gains **Move to its own tab** after All
desks and New desk. It opens a second list of the account's non-template desks; the desk already in this window is
disabled, every other desk is a `menuitem` link to `/desks/<id>` (`target="_blank"`, `rel="noopener noreferrer"`). The
Desks list's per-card menu in `DeskList.svelte` gains the same item. Both use `DeskMenu`, so first-item focus, arrows,
Home/End, Escape and focus return are the existing behaviour, and the links keep the menu's existing styling.

**Persistence.** Nothing new is stored. Each browser tab is already its own desk: its address is the desk, and its
chosen desk tab stays in `sessionStorage['reet-desk-tab:<deskId>']`. A reload restores both windows as they were. The
existing `localStorage['reet.desks.last.<userId>']` follows the newly opened desk, as it does for any desk open.

**Trade-offs.** The switcher is a switch list, not ownership, so the desk is not removed from it. "Already in its own
tab" can only be known for the current window; another browser tab on the same desk cannot be detected without a new
registry, so it is not attempted. Templates are left out, as in the switcher.

**Tests.** `web/tests/desks.e2e.mjs` covers the item in both menus, the keyboard path, the disabled current desk, the
link's address and new-tab attributes, and a real second window that opens the desk and keeps it across a reload.
`npm run check`, `npm run build`, `desks.e2e.mjs` and `desks-phone.e2e.mjs` pass.
## CHAT-COMMANDS-1: an agent's slash commands in Chat (23 November 2026)

The second "Agent behaviour" item of the roadmap's reliability workstream.

**Path.** ACP's `available_commands_update` used to be dropped. The connector now keeps the last announced list on the
chat session, in memory only (`internal/acpagent`); an empty or malformed announcement clears it. It is carried by a new
negotiated feature, `acp-commands-v1`, and a control action `"commands"` (`internal/terminalproto/acp_commands.go`),
kept out of the regular controls poll. `GET /api/v2/terminal/sessions/{id}/chat/commands` answers
`{"commands":[{name,description,hint}]}`; a machine without the feature answers 200 with an empty list, `code`
`chat_commands_unsupported` and a sentence. Nothing is stored: agents announce again on load.

**Limits on the agent's text.** At most 64 commands; names 1 to 64 characters of `[A-Za-z0-9:_-]`, first of two equal
names wins; description 300 bytes, hint 120; the encoded list fits 48 KiB. An invalid entry is dropped, not the list.
The connector trims and the decoder validates again. The web client drops control characters and the rows isolate
text direction; nothing is rendered as HTML.

**Web.** `web/src/lib/terminal/skills.ts` holds the rules (`groupCommands`, `withoutAnnounced`, `unknownCommand`,
`unlistedLine`, `nameClosed`); `SkillPicker.svelte` shows the group first, also while skills load or fail, and a skill
that is also a command appears once. `ChatPane.svelte` reads the list when the chat opens, when the sheet opens and
before judging a `/` message. The unlisted-command line shows while typing (after the name is closed by a space, or
600 ms) and after sending says "Sent to …" or "Queued for … as text"; it never blocks Send. The way to the terminal
is one decision, `terminalWay` in `sessions.ts`: open the native view, "Restart in terminal…", or no control with the
header switch's reason.

**Not done, and not known.**
- Nothing was run against a real agent: which commands each agent announces, and when, is not known. The rule assumes
  that a command an agent did not announce may need its terminal; reet has no list of terminal-only commands.
- Matching is case-sensitive, so `/Review` warns when `/review` was announced.
- Commands can be inserted only while the agent is idle, as with skills.
- At small heights the queue card still clips its Remove button, as before; the new line adds height there.

## HELD-WORK-1: a turn that ends with work still running is not a finish (23 November 2026)

The first "Agent behaviour" item of the roadmap's reliability workstream.

**What it is.** Over ACP an agent can end a turn while tool calls it started (a subagent, a background command) are
still open. Those sessions used to read as finished. The connector now keeps the open calls as a held set and puts
their count on the idle status (`internal/acpagent/session.go`); the feature is `acp-background-v1`
(`internal/terminalproto/acp_background.go`), and both the count and the give-up code are stripped for a peer without
it, so an older machine or controller behaves as before.

**States.** The broker stores chat status `background` (attention: working) while the count is above zero, with
`held_since` for when the turn ended. A held call that completes, fails or is cancelled leaves the set; at zero the
session finishes as usual, and a failed turn stays failed. A new message from the person clears the hold.

**Giving up.** Nothing is waited on for ever. After `BackgroundHoldLimit` (30 minutes) the connector sends an idle
status with the code `unwatched` and a transcript notice naming the agent, the count and the machine; the broker has
its own 30-minute bound for a machine that goes silent and writes its own line. Either ends as chat status
`unwatched`, never `finished`; it survives a restore, the push says "Stopped waiting for its background work; it may
still be running.", and it gets its own card kind in the bell and Waiting for you.

**Gaffer.** A stage in `background` is not reviewed as finished; its line says when waiting stops. A stored
`unwatched` newer than the last review or resume puts the goal in needs-you with a note to read the session, and no
review is spent. Both move-on confirmations open with the warning that moving on stops the work.

**Web.** `web/src/lib/sessionStatus.ts` holds the words (`heldWork`, `HELD_SENTENCE`, `statusLabel(status, held)`);
lists say "In background" with a static ring (`.status-ring`), sentences use the long form, and every surface reads
the full sentence once to a screen reader. `docs/ui-verification.md` and `docs/handoffs/attention-contract.md`
describe the state.

**Not done, and not known.**
- Nothing was run against a real agent or machine: which agents leave tool calls open at the end of a turn is not
  known, so the state may never appear for some of them, or appear for a call an agent simply forgot to close.
- There is no control to stop the background work; moving a stage on, or ending the session, is the only way.
- Permissions asked by a held tool after the turn ended are not handled specially.
- On a phone row "Stopped waiting" is cut short, as "Waiting for you" is.
- The connector's notice names the machine by hostname; the controller's own line uses the name shown in reet.
## FACTORY-TEMPLATES-6: private, versioned Factory templates (23 November 2026)

Stage 6 of [the delivery review plan](docs/handoffs/factory-delivery-review.md). The contract is
[docs/handoffs/factory-templates-design.md](docs/handoffs/factory-templates-design.md).

**Model.** `gaffer_templates` holds one row per version, keyed on `(id, version)`, with `UNIQUE(user_id, request_id)`;
a version is never updated. A version holds the name, description, objective, review limits and each pipeline's title,
prompt, merge setting, criteria `{id, text}` and stages (kind, part, brief, tier and its reason, skills with their
approved revision). The copy from an outcome is an allow-list, so no machine, project, agent, model, credential,
session id, image, note, evidence or decision can reach a template. Caps: 20 templates an account, 20 versions kept
(the oldest drops), 64 KiB a version. An outcome gains `templateId`, `templateVersion` and `templateName`; older
outcomes have none.

**Rules.** Only the signed-in browser account, through `featureAccess`; another account gets 404; read-only accounts,
API tokens and MCP are refused; there is no MCP tool. A plan can be saved when the outcome is not starting and every
pipeline is done, or accepted in delivery review and not failed, unfollowed or cancelled. A save or edit is version
`base + 1` only when `base` is the latest. A repeated request answers what it made; the same id with different content,
or reused across save, edit and launch, is 409 `template_request_reused`. Launch builds an ordinary `OutcomeInput` for
the project chosen then, through `CreateOutcome` and every plan check, with the crew from the launch body or the saved
crew. A skill is used only at exactly the stored approved revision; anything else is a gap the launch must list in
`skippedSkills`, or it is 409 `template_skills_changed` with the current gaps. The diff matches pipelines by title,
stages by kind and part, criteria by id, and names tier, brief and part changes; its default `from` is the version last
used, or the oldest kept when that one was dropped.

**UI.** `web/src/lib/factory/Templates.svelte` (list and a template's page), `TemplateSave.svelte`, `TemplatePlan.svelte`,
`TemplateChanges.svelte`, `TemplateLine.svelte`, `templates.ts`, and a template mode of `GafferOutcome.svelte` that
reuses the plan step's project, crew, readiness and launch controls. The start step shows one line per pipeline, opens
only those that changed or have a skill gap, marks what changed in the plan, and names beside Start the pipelines that
merge by themselves. `ConfirmDialog`'s danger button was being painted in the accent colour by a global rule; it now
uses the danger colour everywhere.

**Reviews.** An independent backend review (seven should-fix findings, all fixed) and four design passes on the
screens: audit 15/20, distill, polish, critique 26/40, whose findings the last round fixed.

**Known gaps.**
- Deleting one version is not built; versions leave only through the 20-version limit.
- Nothing from the last launch is remembered, so each use fills the start form again.
- Templates cannot be shared between accounts.
- The launch is tested end to end at the service layer; at the HTTP layer only as refused where remote sessions are off.
- Two launches at once with one request id can answer 500 for the second, as `CreateOutcome` already did.
- A plain start with no saved crew reads "0 of N stages ready, N to look at": the shared readiness rule, not templates.

**Tests.** `internal/store/store_gaffer_templates_test.go`, `internal/gaffer/templates_test.go`,
`internal/api/gaffer_templates_test.go`, `web/src/lib/factory/templates.test.mjs`, `web/tests/factory-templates.e2e.mjs`.

## FACTORY-RELIABILITY-4: measured Factory refresh, gzip, one poll, and launches that survive a lost connection (22 November 2026)

Stage 4 of [the delivery review plan](docs/handoffs/factory-delivery-review.md).

**Measured first.** Two kept measurements, neither a test nor in a workflow: `internal/api/gaffer_factory_baseline_test.go`
(`REET_FACTORY_BASELINE=1 go test ./internal/api/ -run TestFactoryRefreshBaseline -v -count=1`) and
`web/tests/factory-refresh-baseline.mjs`. For 6 outcomes × 5 pipelines × 8 stages one outcomes read was 353,372 bytes,
uncompressed, read 16 times a minute on the Factory page (the page's 5 s poll plus the shell's 15 s attention poll),
about 6.4 MB a minute in all. Half of each read is stage handover summaries, which only the open pipeline's detail
draws. Time to a visible decision is bounded by the 5 s poll (median about 1.7 s desktop).

**Performance.** `writeJSON` gzips a body of 1 KiB or more when `compressJSON` marked the response as accepting gzip
(not HEAD, not an upgrade; never 204, 304, 206 or an already encoded body) and sets `Vary: Accept-Encoding` on every
JSON body. The marker type overrides nothing that writes, so streams, flushing and hijacking are untouched. The
outcomes read is 92,490 bytes as sent, for about 6 ms more per read. On `/factory` the shell's poll uses the reads the
Factory page hands it (with the time each started) and reads outcomes and goals itself only on first load, after an
answer, or when the page's read is over 15 s old. The two changes were not measured together.

**Reliability.** `internal/gaffer/outcome_disconnect_test.go` exercises a lost machine and a controller restart at
launch, fallback, approval and handover. What changed in the controller:
- A launch's request is written to `UnsureRequest` in the same write that marks the pipeline starting and cleared when
  the answer is known. Whether it was sent, and what became of it, is read from the machine's create record in
  `terminal_operations` (`launchSent`): none or `failed` means not started; a running session is taken up; anything
  unreadable stays unknown. So a lost reply or a restart never starts a second session, and a merge launch in flight
  at a restart is not forgotten.
- Sending a pipeline back ends the session an unanswered launch started.
- `rebalanceStopped` marks a stage as moved to its backup in the retry's own write, counts an end as done only when
  the session no longer runs, and waits 5 minutes (in memory) before asking a machine again.
- `endLeftBehind` ends retired sessions of pipelines that are following, in delivery review, ready to merge or done.

**Known gaps.**
- A send-back that lands while a launch is in flight may have its new state overwritten by the old run's failure
  write; noticed in review, not tested or fixed.
- Deleting an outcome whose launch is unanswered leaves that session running. A session still being created when its
  pipeline is sent back is not ended later.
- No ETag, compact list read or change to poll intervals. The phone chat that stops following a streaming reply (see
  the long-chat baseline) is not addressed.
- Nothing ran against a real machine.
## FIRST-USEFUL-RUN-5: a guided first session on Home (22 November 2026)

Stage 5 of [the delivery review plan](docs/handoffs/factory-delivery-review.md). Web UI and docs only; no Go, no API.

**What it is.** `web/src/lib/home/FirstRun.svelte` (pure logic in `home/firstRun.ts`) shows **Your first session** on
Home once a machine is connected and every session so far is one the guide started. It lists the agents that run as a
chat on that machine (`structuredAgent`), each with its sign-in line from `launchReadiness`, the same sentences as the
Factory plan step; a model line shows only when the list could not be read. A ready agent that needs no folder trust
is preselected; one that needs it names the folder in its tick box. Agents that only run in a terminal get one line
pointing at **Start an agent session**. One read-only question is started through `createTerminal`, without YOLO, in a
chosen project folder or the home folder, with a reused request id so asking twice starts one session.

**States.** Asking (with a slower-than-usual line after a minute), answered ("It was asked to change nothing and ran
without YOLO": what is known, not a claim that nothing changed), ended before an answer (with the exit code when it is
not zero and the other agent offered), signed out (the machine's sign-ins are read again every 15 s only while one is
missing; **Open terminal** is the main action), no agent installed, no chat agent, machine not ready. **Not now** and
**Done** are remembered per person in the browser; after **Not now** a **Show the first-session guide** link stays on
Home. While the guide shows, the workbench's **Start an agent session** is drawn plain so there is one filled button.

**Design passes.** Audit 16/20, then distill, polish and a critique (26/40) whose findings are what the last round
fixed: only chat agents are offered, the success line says only what is known, every stuck state has a way to act.

**Known gaps.**
- The answer's text is not shown in the guide; it links to the session. An answer can take up to 15 s to register.
- A reply is read from session state; nothing ran against a real agent.
- The help link's address (`FIRST_SESSION_DOC_URL`) is derived from the docs site's address and was not loaded.
- **Open terminal** in the signed-out state starts a session, after which the guide no longer shows.
- The guide speaks plainly, not in the brand's voice.

**Tests.** `home/firstRun.test.mjs`; `web/tests/first-session.e2e.mjs` covers every state at desktop and phone widths.

## FACTORY-LAUNCH-READINESS-3: launch readiness before approval, and a usage limit explained (22 November 2026)

Stage 3 of [the delivery review plan](docs/handoffs/factory-delivery-review.md). It is in the web UI only: no Go, no
API and no stored field changed. Everything it says is worked out from what the pages already read.

**Before approval.** `web/src/lib/factory/launchReadiness.ts` is a pure resolver. From the machine, the agents, efforts
and model lists it reports, its sign-ins and plan figures, the saved crew and the choices on screen, it says for each
crew tier the agent, model and effort a stage will run on and where the agent came from (saved crew, the person's
choice, the outcome's agent, or the outcome's agent because the saved one is not on the machine). Each fact is one of
five states that are worded differently: ready, default (left to the agent, a choice), missing, stale and unavailable.
Backups are listed in the order tried with what each will do. A tier is **ready**, has something to **look at**, or is
**blocked**; only an agent that is not on the machine now, or a machine that is not connected, blocks.

The plan step of **New Factory outcome** (`GafferOutcome.svelte`, `factory/LaunchReadiness.svelte`,
`factory/StageParts.svelte`) draws it. Each stage, and each part of a split stage, is one line with its backups folded
under it. **Launch readiness on** the machine sits at the head of the pipelines: it counts the stages and says each
cause once, with the crews and stages it affects, and **Choose who does what** opens the crew editor on the first
affected tier's agent. The crew editor used to be on the first step only; it is now on the plan step too, so a blocked
plan can be fixed where it is read. An agent that has left the machine stays named in its box as missing. The reason
Approve is held is beside the button in one status line, which also says when it clears. The machine's sign-ins are
read once per machine when the plan opens (a busy or failed read is tried once more after 3 s), and the machine list is
read again when the plan opens and every 10 s while a stage is blocked.

**A usage limit.** `usageLimitWhy` in `web/src/lib/gafferOutcome.ts` says which agent and model ran out on which stage,
which agent the backup is, and that either choice carries on in the same folder and branch; after a model has already
taken over it says where the stage started, and whether no backup is saved or none is left. Home's **Waiting for you**
and the Factory list (`factory/WaitingItem.svelte`) show **Continue with the backup** and **Choose another model…** as
buttons; the Factory detail adds the plan left and when it resets.

**What it must match on the server, which is unchanged.** `balanceChoice` moves a stage to a backup only when the first
choice's figures are under six hours old and low, the backup is signed in and has more left. `nextUsageBackup` and
`recoverPipelineWorker` take the next backup after a usage limit without checking that its agent is on the machine or
signed in, so the plan says such a backup is still tried and is likely to be refused. The rows are built from
`withReportedEfforts(stagePlan(...))`, the same dedupe and effort strip as the payload, so a row never lists a backup
or an effort that is not sent.

**Reviews.** An independent review of the first build found sentences that said things the server does not do (a
same-agent backup that "can take over", a saved effort shown as the person's choice, pending reads counted as problems);
all were fixed. Four design passes followed: an audit (16/20), a distill, a polish and a critique (24/40). The critique's
two main findings, one cause repeated under every stage and a fix that could not be reached from the plan, are what the
panel and the button answer.

**Known gaps.**
- After a usage limit the server tries the next backup even when its agent is not on the machine or not signed in.
  The plan warns about it; making the server skip it is Go work.
- Home does not say when the plan resets; only the Factory detail reads plan figures. On a desktop Home row
  **Choose another model…** wraps onto two lines.
- "More left" compares each agent's tightest window, where the server also matches a window that names a model
  (a `shortcut:` comment marks it).
- The YOLO and folder-trust checks in the dialog still look only at first choices, as before.
- The figures are those read when the plan step opened, not at the moment of approval.
- Release B's checks were not made: all three agents on a real connector, a real exhausted plan and a lost connection,
  and a retry recording the chosen backup. Everything here ran against the suites' fixtures.

**Tests.** `launchReadiness.test.mjs` covers the five states, each source, the backups' rules, the summary and its
causes; `gafferOutcome.test.mjs` the usage-limit sentences. In the browser, `gaffer-outcome.e2e.mjs` covers the panel,
the button to the crew, the one-line stages and what holds Approve, at desktop and phone widths, light and dark;
`home-waiting.e2e.mjs` and `factory-decision.e2e.mjs` the usage-limit card and both ways on.

## CHAT-LONG-BASELINE-1: the long-chat baseline is measured (22 November 2026)

The delivery review plan's stage 0 asked for a long-chat baseline for stage 4 to compare against; it had not been
captured. This records it and keeps the script that measures it.

**What changed (a measurement and its record only).** `web/tests/chat-long-baseline.mjs` opens one chat on the
production bundle with the same fake API and event stream the chat suites use, replays a history through the stream,
streams a reply a piece at a time, and prints JSON: the history's bytes and time to draw, the time one streamed piece
takes to reach the screen (median, 95th percentile, worst), and how the scroll position behaves while following and
while reading. `REET_CHAT_EVENTS` and `REET_CHAT_UPDATES` set its size. It is not a test, asserts nothing and is in no
workflow. The figures are in
[docs/handoffs/factory-delivery-review-design.md](docs/handoffs/factory-delivery-review-design.md#long-chat).

**What it found, on main at `2dec0d1`.** A 2,000-event history is 313,161 bytes and draws in about a quarter of a
second. A streamed piece of 107 bytes reaches the screen in two or three frames (p95 50 ms) at both widths. A reader
scrolled back to the middle is not moved at all by 200 streamed pieces. Following is the weak spot: at phone width the
page stops following a streaming reply and ends about 4,600 px above the latest text, where desktop stays at the
bottom. The log follows only within 80 px of the bottom and scrolls smoothly, and on a narrow screen the smooth scroll
falls further behind than that. Nothing was changed about it here; it wants confirming on a real phone.

**Not captured.** Time to a visible decision on the Factory page, which belongs with stage 4's own changes.
## TEMPORARY-ORDER-1: temporary projects made in the same millisecond list in the order made (22 November 2026)

`TestTemporaryFolderProjectsListEveryAccountsOldestFirst` failed now and then. `TemporaryFolderProjects` ordered by
`created_at,id`: two projects made in the same millisecond tie on `created_at` and fell to `id`, which is random, so
"oldest first" listed them either way. The delivery review plan's stage 0 triaged it and left it for its own change.

**What changed.** `internal/store/store_folder_projects.go` orders by `created_at,rowid`, so a tie is settled by the
order the rows were made. The only caller is the temporary-project sweep, which looks at each project once and does
not depend on the order of a tie. The test is unchanged.

**Tests.** Before the change the test failed in each of three batches of 300 runs; after it, 2,000 runs pass
(`go test ./internal/store/ -run TestTemporaryFolderProjectsListEveryAccountsOldestFirst -count=2000`), as do the
other folder-project tests.

## FACTORY-DELIVERY-REVIEW-2: delivery review UI (22 November 2026)

Stage 2 of [the delivery review plan](docs/handoffs/factory-delivery-review.md), on top of
`FACTORY-DELIVERY-REVIEW-1` below, built to
[the contract](docs/handoffs/factory-delivery-review-design.md).

**What changed.**

- **Plan approval.** `web/src/lib/factory/CriteriaEditor.svelte`, used by `GafferOutcome.svelte` and
  `AddPipelines.svelte`: an editable list of acceptance criteria per pipeline, sent as `criteria` only when there are
  some. A reply to Gaffer keeps the criteria already typed.
- **The card.** `web/src/lib/factory/DeliveryReview.svelte` with pure helpers in `delivery.ts`, shown in
  `PipelineDetail` for a pipeline with criteria. It shows the reviewed commit, branch and pull request, each criterion
  as **Passed**, **Needs work** or **Unverified** with its evidence, **Add evidence**, **Edit criteria**, and
  **Accept delivery** and **Send back** beside the findings. A pipeline waiting for review, or one opened from Home,
  is read at once; any other shows **Show the delivery review**, so choosing a pipeline asks no machine anything.
- **Decisions.** Both send the criteria revision, review revision, commit and statuses on screen, with a request ID
  kept only for a retry of the same attempt. A 409 keeps what was typed, says what changed and reloads the review.
  What accepting leads to is worked out from the pipeline's own merge setting, and the copy says throughout that
  accepting is not approving the merge.
- **State.** `delivery_review` is in **Waiting for you** everywhere pipeline states are read (`gafferOutcome.ts`,
  `factoryList.ts`, `homeWaiting.ts`, `SessionDecision`), with **Review delivery** as its button. A pipeline that
  came back after acceptance says why and that the acceptance and merge approval were withdrawn.
- **Home.** The waiting card reads "Delivery review · 3 criteria" from the outcome list alone and links to
  `/factory?pipeline=<outcome>.<key>&review=1`, which selects the pipeline, brings the card into view and focuses its
  heading. Reload keeps it and Back returns to Home.

**Independent review.** A separate reviewer compared the UI with the finished backend and found one blocker (the
check picker threw when two CI jobs shared a name) and several smaller faults, fixed here: stale "what accepting leads
to", no reason shown for a returned review, two wrong conflict messages, a miscounted history line, criteria lost on
a reply to Gaffer, Edit criteria offered where the server refuses it, focus lost after a refusal, needless machine
re-reads, and browser fixtures that did not match what the server sends.

**Tests.** `web/tests/factory-delivery.e2e.mjs` runs at 1440 and 390 wide, with Accept and Send back by keyboard:
criteria into the create body, the card's states, both decisions and their bodies, the separate merge approval, the
409s, a retry reusing its request ID, a double click, duplicate check names, long text at phone width, an old
outcome with no card and no machine read, and Home's link. It is added to `gaffer-settings.yml`. `npm run check`
passes (118 test files, 0 Svelte errors, 0 warnings), and against `vite preview` the suites `factory-delivery`,
`gaffer-factory`, `factory-decision`, `gaffer-outcome`, `home-waiting`, `gaffer-factory-session`, `factory-pieces`,
`home-followups`, `phone-dialogs` and `phone-layout` pass.

**Not done.** The send-back stage is not changed by keyboard in the browser test (the default stage is kept), and the
reply-to-Gaffer carry-over has a unit test only.

## FACTORY-DELIVERY-REVIEW-1: delivery review backend (22 November 2026)

Stages 0 and 1 of [the delivery review plan](docs/handoffs/factory-delivery-review.md). The contract both stages build
to, with the states, the status rules, the conflict codes and the routes, is
[docs/handoffs/factory-delivery-review-design.md](docs/handoffs/factory-delivery-review-design.md).

**What changed.** A pipeline can be approved with acceptance criteria (`criteria` on each pipeline of the plan). Such a
pipeline stops in a new state, `delivery_review`, before its merge stage or before it is called done, and waits for the
person to **accept** the delivery or **send it back**. A pipeline without criteria, which is every outcome saved
before this, never enters the state.

- `internal/store/store_gaffer_outcomes.go`: `Criteria`, `CriteriaRevision`, `ReviewRevision`, `Evidence`,
  `Deliveries` and `DeliveryAccepted` on the pipeline, inside the outcome document. No migration: the fields are
  absent from old rows and omitted when empty.
- `internal/gaffer/outcome_delivery.go`: the review read, criteria edits, evidence and the decision.
  `outcome_runs.go` holds the gate on all three ways into the merge stage or the end, and tells every stage's agent the
  criteria. `outcome_manage.go` shares one send-back path, which now also clears an acceptance.
- `internal/api/gaffer_delivery.go`: four routes under `…/sessions/{key}/delivery`, the owning browser account's only.
- `internal/terminalproto`, `terminalhost`, `terminalbroker`: the inspect answer gains the folder's `head` commit and
  the pull request's `headOid`. Both are optional, so the inspect feature name is unchanged. A machine that predates
  them reports neither, and the review then says the commit is **unverified** and refuses a decision until it updates.

**Rules worth knowing.**

- A decision names the criteria revision, the review revision, the commit and the statuses the person was shown. The
  controller reads the head from the machine again, works the statuses out again, and writes only if the pipeline is
  still waiting on the same two revisions. Otherwise the answer is 409 with a code.
- A request ID makes a decision idempotent: the same request again answers the recorded decision, also after the
  pipeline moved on. Two decisions at once leave one.
- Accepting never sets `mergeApproved`. A pipeline whose merge waits for you still stops at `ready_to_merge`.
- What merges is what was accepted: the head is read again when the merge stage is about to start, and a commit that
  arrived since, or criteria reworded since, send the pipeline back to `delivery_review` with the acceptance and the
  merge approval cleared.
- A delivery is sent back to a work stage, never to the merge, and a pipeline in `delivery_review` leaves it only by
  a recorded decision: the plain send-back, on the page or over MCP, is refused there.
- A project reet knows is a repository is never decided without a commit, whatever its machine says of the folder.
- Evidence counts only for the current commit and the current wording of its criterion. A GitHub check counts only
  while GitHub's pull request is at the reviewed commit, and is read from the machine each time, never from the
  request or from what was saved.
- The head is read just before the write. A commit that lands between that read and the write is not seen; the
  decision records the commit that was read.
- Gaffer, given an outcome's decisions, cannot accept a delivery, and there is no MCP tool for it.
- The outcome reads the Factory page polls leave `evidence` out; only the review's own read carries it.

**Baseline (stage 0).** On main at `b3ad27c`, a synthetic Factory of 6 outcomes × 5 pipelines × 8 stages reads as
276,692 bytes per refresh, every 5 seconds. The long-chat baseline and time to a visible decision were not captured;
they need the browser harness and are left for stage 4.

**Ordering test triage (stage 0).** `TestTemporaryFolderProjectsListEveryAccountsOldestFirst` orders by
`created_at,id`. Two projects made in the same millisecond tie on `created_at` and fall to `id`, which is random, so
the test can list them either way. It did not interfere with this work and is unchanged here; it wants its own small
PR that either gives the fixture distinct times or settles the tie-break.

**Tests.** `internal/gaffer/outcome_delivery_test.go` covers the gate and the separate merge approval, a pipeline
without criteria, a repeated and an altered request, two reviewers at once, changed criteria, a new commit, an
unreachable or older machine, checks read from GitHub, artefact links, send-back into rework, another account, a
read-only and a disabled account, and another pipeline's criterion. `internal/store/store_gaffer_delivery_test.go`
opens and writes back an outcome in the old format. `internal/api/gaffer_delivery_test.go` holds the route refusals,
and the inspect tests cover the head commit on a real repository.

**Independent review.** A separate reviewer, given the diff and the contract and not this account of them, found one
blocker and several smaller faults, all fixed here with a test each: a send-back that lost a race answered 400 and
could stop the goal of a stage the winning decision had started; send-back to the merge stage redid nothing; an
encoded `..` passed the artefact allowlist; a commit added after acceptance could merge unreviewed; checks sharing a
name could hide a failure; long handovers could push criteria out of a stage's prompt; `not_repo` was trusted for a
repository; evidence at its cap could drop a failing item. Left as they are: the control routes other than the two
outcome reads still answer with evidence in the outcome, and Gaffer's chat context does not yet mention a review.

**Not done.** The UI is stage 2, in its own pull request on top of this one. Launch readiness, performance work and
templates are later stages.
## DECISION-TEST-RACE-1: the lost-decision test no longer races the supervisor (22 November 2026)

The `test` job for #876 failed once in `TestAStopWhoseAnswerNeverCameIsAskedAgain` with "That outcome changed. Reload
it before trying again." The test read the outcome's revision and then gave Gaffer the decisions through
`SetOutcomeAutomation`; the outcome supervisor can record other progress in that window, so the optimistic update lost
the revision check. The same hand-read/write pattern was in two neighbouring decision tests.

**Fix (tests only).** `internal/gaffer/outcome_decide_test.go` gains `setAutomation`, which retries on
`store.ErrGafferOutcomeBusy` with a fresh revision, as the person's card does after a reload. The three decision tests
use it instead of writing with a revision read by hand. No product behaviour changed: a stale revision is still
refused, and a real person still gets the reload sentence.

**Verified.** The three decision tests pass 20 times under `-race` (492 s), the whole `internal/gaffer` package
passes, and `go vet ./internal/gaffer` is clean.

**PR.** [#881](https://github.com/eyupio/reet.sh/pull/881) opened against `main`. **Merged** as `c827b800` on 22 November 2026, after the CodeQL, source, compare, test, web, build, offline and Gaffer offline checks passed.

## FACTORY-STAGES-TEST-1: finished Factory stages leave every list (22 November 2026)

`FACTORY-STAGES-LIST-1` left one gap it named: no browser suite proved a Factory-finished stage disappearing from Home,
Sessions and the workspace sidebar.

**What changed (tests only).** `web/tests/sessions.e2e.mjs` gains a `finished Factory stages` section. It answers one
outcome with two pipelines in the sessions fixture: an earlier plan stage whose session has ended and a finished
pipeline's session, both Factory-finished, beside the code stage in hand and a plain session. The Sessions page lists
the stage in hand and the plain session and omits the finished two; the workspace's **Terminal sessions** list agrees;
Home's **Continue working** and **Planning board** show the same two and omit the finished ones. `open()` in that file
now accepts `outcomes` for the route, empty by default.

**Tests.** `npm run check` passes (node tests and 0 Svelte diagnostics/warnings) and the new section passes against
`vite preview` with `REET_E2E_ONLY='finished Factory stages'`. No application or Go code changed.

**PR.** [#876](https://github.com/eyupio/reet.sh/pull/876) opened against `main`. **Merged** as `6073cff9` on 22 November 2026, after the CodeQL, source, test, web, build, image, offline, phone-chat, phone-dialogs, phone-gaffer, phone-sessions, phone-zoom, terminal-browser and Gaffer settings checks passed.

## USAGE-PLANS-2: plan limits read where each agent keeps its sign-in (22 November 2026)

The owner reported that the Usage page showed a plan for Codex only: Claude Code and Google Antigravity said the
sign-in had run out, OpenCode said no sign-in, and GitHub Copilot was not read, though the person was signed in to
them. A review of the plan readers against the reference implementation found that reet read only each agent's files
(and OpenCode's variable), while the agents also keep sign-ins in the platform keychain or keyring, and Copilot is
often signed in through the GitHub CLI alone.

**What changed.** `internal/agentaccounts` now reads each sign-in from more of the places its own agent keeps it, and
uses the later token where a file and a keychain item disagree (a CLI that cannot write the keychain refreshes the
file alone):

- **Claude Code**: the macOS login keychain's `Claude Code-credentials` item, as well as `.credentials.json`.
- **Codex**: the macOS keychain's `Codex Auth` item when the file holds none (or an older token).
- **Google Antigravity**: the Linux Secret Service item (`secret-tool lookup service gemini username antigravity`)
  and the macOS keychain's `gemini`/`antigravity` item, as well as `antigravity-oauth-token`.
- **GitHub Copilot**: the GitHub CLI's own sign-ins for `github.com` (`gh auth status --json hosts`, then
  `gh auth token --user`; an older CLI's `gh auth token` alone), tried in turn after the editor files.

Sign-ins are now found side by side and each keychain, keyring or CLI read is bounded at 2 s, so
`agentaccounts.ReadTakesAtMost` stays 11 s inside the connector's 12 s answer. The reader still never renews or
rewrites a sign-in.

**Usage page.** `limitLines` keys each plan window by its position, so two windows of the same kind (Codex's, when a
provider does not say each length) no longer collide in a keyed `each`. Stale figures stay dimmed after a failed
period change instead of only while loading.

**Verified.** A real read on this machine (states only): Codex `plus`, 2 windows; GitHub Copilot now `individual`,
1 window, through `gh`; Claude Code is signed in with an API key/prepaid (no plan to read); OpenCode holds only an
`ollama-cloud` key (no Go plan); Antigravity's file token expired on 12 November. `go test
./internal/agentaccounts/ ./internal/terminalagent/ ./internal/api/` passes, as do all 117 web test files, and the
`usage`, `plan-meters` and `chat-usage` browser suites pass against `vite preview`.

**Docs.** `docs/agent-accounts.md` names the new places; `CHANGELOG.md` records the change.

**PR.** [#877](https://github.com/eyupio/reet.sh/pull/877) opened against `main`. **Merged** as `6fbac352` on 22 November 2026, after the CodeQL, source, compare, test, web, build, image, offline, phone-chat, phone-dialogs, phone-gaffer, phone-sessions, phone-zoom, terminal-browser and Gaffer settings checks passed.

## PHONE-SKILL-BUDDY-1: skill suggestions fold to one row on a phone (22 November 2026)

The owner shared a phone screenshot of an active chat where the **Skills for this task** panel filled much of the screen
above the composer, pushing the conversation and message box away.

**What changed.** `web/src/lib/terminal/ChatPane.svelte` now watches `PHONE_SCREEN` from `sizing.ts` and passes the
existing `compact` prop to `SkillBuddy`, which until now was unused by the chat. On a phone the panel starts as its
header row — title, the first suggestion's name, **See skills** and **Pause** — and opens on demand. The desktop chat
panel, the native terminal's **Suggested skills** dialog and a Factory stage's per-stage suggestions are unchanged.

**Tests.** `web/tests/chat.e2e.mjs` gains a phone case: the panel shows **See skills**, its body is absent, and opening
it shows the suggestions. `npm run check` and `npm run build` pass, and the whole `chat.e2e.mjs` passes against
`vite preview` at phone and desktop widths.

**Docs.** `docs/skill-buddy.md` says the panel folds on a phone; `CHANGELOG.md` records it.

**PR.** [#874](https://github.com/eyupio/reet.sh/pull/874) opened against `main`. **Merged** as `8cf7455b` on 22 November 2026, after the CodeQL, source, test, web, build, image, phone-chat, phone-dialogs, phone-gaffer, phone-sessions, phone-zoom, terminal-browser, Gaffer offline and Gaffer settings checks passed.

## FACTORY-CAPABILITIES-1: Add pipelines stays within the machine's agents (21 November 2026)

A review of the Gaffer Factory's capability handling found three things. **Add pipelines** put the outcome's own agent
back into the agent list even when the machine's report left it out, so a pipeline could be added for an agent that was
uninstalled, gated off or unavailable on that machine, and every stage then failed at its start. Two comments still said
an effort the machine does not report fails the launch, which the controller stopped doing when it began dropping the
effort and starting the stage on the agent's default. And a hand-picked backup effort could stay selected after the
machine's report stopped listing it.

**What changed.** `web/src/lib/factory/AddPipelines.svelte` now lists only the agents the machine's known report offers;
the outcome's own agent is included only when reported, and a new pipeline defaults to an offered agent with a default
model and effort when it is not. The plan each added pipeline would send is derived once and reused for approval, and
the Add button is refused while it names an agent the machine does not offer, with a warning naming that agent; a machine
reporting no chat agent says so instead. The same effect that clears unoffered main and crew efforts now clears a
hand-picked backup effort. The comments in `web/src/lib/terminal/launchOptions.ts` and `web/src/lib/GafferOutcome.svelte`
now describe the controller's real behaviour: an unoffered effort is dropped, the stage starts on the agent's default,
and the record says why.

**Tests.** `web/tests/gaffer-factory.e2e.mjs` gains a case with a machine reporting only Codex for a Claude outcome: the
Agent list offers only Codex, Codex is the default, and the added pipeline is sent on Codex with the saved crew's Codex
backup. `npm run check` passes (117 test files, 0 Svelte diagnostics, 0 warnings), `npm run build` passes, and the whole
`gaffer-factory.e2e.mjs` passes against `vite preview`. No Go code changed.

**PR.** [#873](https://github.com/eyupio/reet.sh/pull/873) opened against `main`; its checks are this change's.

## SESSION-ACTIONS-REDESIGN-1: the Session actions sheet reads as grouped settings (20 November 2026)

The owner shared a screenshot of the **Session actions** dialog and asked for a significant UI and UX improvement. The
old sheet was a wrapping toolbar: the approval toggle, a long "legacy action" paragraph, the interface select, the
theatre button and every management button floated in one flow, so controls of different kinds and weights competed for
attention and the dialog grew taller than the viewport.

**How it works.** The sheet is now four labelled sections — **Approvals**, **Interface**, **Manage session** and a
separated **End session** danger zone. Each section has a short description, every control shares one height, radius and
focus ring, and management actions sit in a two-up grid of icon + label tiles that stacks to one column on narrow
screens. The session title and machine moved into the dialog header as a subtitle, the legacy paragraph became one
sentence about when a restart is needed, and the destructive action is a filled red button inside a tinted block. All
existing names, roles, disabled reasons, approvals gates and confirmations are preserved.

`WorkspaceDialog` also gained the optional `subtitle` prop and now uses a grid frame with `overflow: hidden`; only its
content region scrolls. Chromium's modal `<dialog>` UA style sets `overflow: auto`, so the previous auto-scrolling frame
and content could draw two scrollbars side by side.

**Checked.** `npm run check` (all node tests, 0 Svelte diagnostics, 0 warnings), `npm run build`, and against a local
`vite preview`: the full `sessions.e2e.mjs`, `phone-dialogs.e2e.mjs` (25 dialogs across six phone viewports) and
`terminal-organisation.e2e.mjs` all pass. Light and dark screenshots at 1360, 768 and 390 were reviewed.

**PR.** [#863](https://github.com/eyupio/reet.sh/pull/863) opened against `main` and **merged** as `424f4809` on 20 November 2026 after the source, test, web, build, image, phone, terminal-browser, Gaffer, offline and CodeQL checks passed on the merged head.

## LATEST-CODE-2: Start an agent offers latest code for a repository with worktrees off (20 November 2026)

The owner reported: "Start Agent modal doesnt allow the user to pull latest code from github if project folder is
behind." A repository project whose **Worktree for each session** setting was off (or whose account default was off)
hid the **Code for this worktree** choice in Start an agent, even though the controller's `isolateWorktree` gives every
agent start in a saved repository a worktree of its own whatever that setting says. The dialog also sent no worktree,
base or `pullLatest` in that case, so the choice could not have travelled with the start even if it had been shown.

**How it works.** `StartAgentDialog.svelte`'s `launchesInWorktree` now depends only on the chosen folder being a saved
repository on a machine that negotiated worktrees; the project's `worktreeDefault` and the account's
`worktreesByDefault` no longer gate it. The start's worktree block uses the same rule, so a repository project always
sends its worktree token, its base and the person's latest-code answer. A non-repository project, or a Gaffer chat,
still keeps its own folder. The controller needed no change: it already isolated these starts and applied
`pullLatestFor`.

**Tests.** `web/tests/start-agent.e2e.mjs` gains a case with a `worktreeDefault: 'no'` repository project: the choice is
visible, **GitHub's latest** is picked, and the create carries `worktree`, `base: 'main'` and `pullLatest: true`.
`svelte-check` is 0/0, `npm test` and `npm run build` pass, and the whole `start-agent.e2e.mjs` passes against
`vite preview`.

**Docs.** `docs/start-agent-session.md` (the dialog says the choice is offered whatever the worktree setting says) and
the changelog.

**PR.** [#864](https://github.com/eyupio/reet.sh/pull/864) opened against `main`; its checks are this change's.
**Merged** as `555be568` on 20 November 2026, after the build, gaffer-settings, offline, phone, source,
terminal-browser, test, web and CodeQL checks passed.

## GAFFER-GOAL-WATCH-1: a stopped goal keeps watching its session (20 November 2026)

The owner reported a Home **Waiting for you** Gaffer entry on a Factory pipeline that was running and needed nothing, and asked Gaffer to poll the sessions. The goal had gone `needs_user` (a review brought a question back) and was then never looked at again: `DueGafferGoals` only returned `needs_user` goals that were held because the working model itself was waiting, so any other stop was final until the person resumed it.

**Fix.** `DueGafferGoals` now returns every `needs_user` goal that is due. `reviewGoal` sends those to the new `reviewStoppedGoal`, which keeps watching the working session without a model request:

- A goal held for the working model still goes through `reviewHeldGoal` unchanged.
- A goal whose Gaffer review failed stays with the person (deferred, never released by progress), so its reason is not lost.
- Every other stop (a review's question, no next step, an idle agent, a stopped worker, an undelivered instruction) is released through `store.ReleaseGafferGoalWatch` once the session shows new retained progress after `LastSeq`, or is reported `working` with activity newer than the stop. Releasing clears `noStep`/`advice` from the old stop and marks the goal `active`, so the normal review sees the new progress.
- When nothing has moved, `store.DeferGafferGoal` sets `nextAt` to look again in 15 seconds (five minutes for a failed review or an ended session) rather than every tick. It changes only `nextAt`, not `updatedAt`, so the entry's age stays true.

`ReleaseGafferGoalWatch` keeps the account's ten-active-goals limit and leaves `LastSeq` alone. The held path and its release are untouched.

**Tests.** `TestAGoalThatStoppedForThePersonFollowsAgainWhenTheSessionMovesOn` proves an unchanged session stays stopped with no model request, new progress releases the goal, and the new progress is then reviewed. `TestAFailedReviewStaysWithThePersonWhenTheSessionMovesOn` proves a failed review is not auto-released, and `TestALiveWorkingSessionReleasesAGoalWithoutANewEvent` covers the live `working` report. `go test ./internal/gaffer ./internal/store` pass; the full local `-race` package run hit the pre-existing sqlite schema-init hang recorded below, so CI is the arbiter.

**PR.** [#861](https://github.com/eyupio/reet.sh/pull/861) opened against `main`; the checks above are this PR's. **Merged** as `89530cfd` on 20 November 2026, after the image, binary release, offline, benchmark, docs and CodeQL checks passed on the branch merged up to #860.

## FACTORY-STAGES-LIST-1: finished Factory stages off Home and the session lists (20 November 2026)

The owner asked: "Stop showing factory-finished stages on the home page and in sessions, as it's very confusing for
the user. We only need to be showing the umbrella factory item and not them within."

**What was built.** `offFinishedFactory` and `finishedFactorySessions` (`web/src/lib/gafferOutcome.ts`) take out of a
session list every session whose `FactoryMember.working` is false: an earlier stage's session, or any stage of a
pipeline the controller is no longer moving. The stage in hand stays, marked with its stage. Home's `listed` (Continue
working, the planning board, the open count and project running counts) applies it after `offDesks`; its `factory`
map is now built from every outcome, archived ones too, so a put-away outcome's stages leave with it while its items
still draw only from the unarchived list. `SessionsPage.svelte` reads `listGafferOutcomes` in its 15-second batch and
filters through the same helper; the workspace (`TerminalWorkspace.svelte`) reads the outcomes with its other inventory
(bounded to four seconds on its own, so a slow Factory cannot hold up the sessions), filters the sidebar, the resume
list, the panel count and the project running counts, and never selects a hidden session on first load. A failed or
absent Factory read keeps the last map and, with nothing read, every session shows exactly as before.

**Checked.** `gafferOutcome.test.mjs` gained the finished-stage case (`s1` earlier stage and `s3` finished pipeline
leave, `s2` in hand and a plain session stay; an empty map keeps everything). Full `npm test`, `svelte-check` and the
web build pass. `docs/session-board.md` and `docs/factory.md` now describe the stage in hand being listed and the
finished stages being left to the pipeline; `CHANGELOG.md` records the change.

**Not checked.** No browser suite was run against the changed lists; the existing `gaffer-goal.e2e.mjs` fixture keeps
its stage in hand, so its marker still shows, but a case for a finished stage disappearing from Home, Sessions and the
workspace sidebar is still to add. No real controller, archived outcome or desk-and-Factory session was exercised.

**PR.** [#860](https://github.com/eyupio/reet.sh/pull/860). **Merged** as `bbb92af8` on 20 November 2026.

## FACTORY-GITHUB-1: GitHub facts beside a Factory pipeline (20 November 2026)

The owner asked for more GitHub metadata on the Factory item detail: the pull request number and link, the stages of CI
and their status, whether Gaffer had to put any stage right and how many times, the CI run time, and a clear next
action such as ending the Factory sessions and archiving or removing the outcome.

**How it works.** The detail reads the GitHub observation the session surfaces already share (`ensurePullInsight` on the
session in hand, which every stage of a pipeline shares the folder and branch of). When a pull request is known, a
**GitHub** block shows its number, state and link, the CI counts, the run time and Gaffer's fixes. **CI stages** opens
the checks with each state and link. Gaffer's fixes count decisions for this pipeline whose choice was `again`, `tell`
or `restart`; a failed or left-to-the-person decision is not a fix. A finished outcome's action bar adds **Archive
outcome** and **Delete outcome…**, with Delete still asking first; the outcome menu keeps both.

**Data.** `terminalproto.InspectCheck` gains optional `startedAt` and `completedAt` (Unix milliseconds), read from
`gh pr view --json statusCheckRollup` and validated like the pull request times. No migration; older machines simply
report no run time. The browser's `InsightCheck` carries the same optional fields.

**Tests.** `web/src/lib/factory/githubFacts.test.mjs` holds the CI counts, run span and fix-count rules;
`internal/terminalhost` checks the times are read and sorted with the checks; `internal/terminalproto` bounds the new
fields and still fits forty checks in the frame. The factory browser test now answers an inspect read with a pull
request, CI checks and times, and checks the GitHub block and the finished bar. The shared test workspace answers an
unmocked session inspect with `no_pr`, so the automatic read does not count as an unexpected API call.

**Checked.** `npm run check` (all node tests, 0 Svelte diagnostics, 0 warnings), `go vet ./...`,
`go test ./internal/terminalproto ./internal/terminalhost` (also under `-race`), and against `vite preview`:
`gaffer-factory.e2e.mjs`, `factory-pieces.e2e.mjs`, `factory-decision.e2e.mjs` and `gaffer-layout.e2e.mjs`. The full
`go test -race ./...` did not finish cleanly in this environment: `internal/api` and `internal/gaffer` hit the ten-minute
package timeout under load, `internal/clientconfig` failed its pipe prompt test, and `internal/lifecycle` failed
`TestDoctorIsReadOnlyAndRedactsCredentials` ("token not sent"); all are outside this change and CI is the arbiter.

**Docs.** [Factory](docs/factory.md), the README's Factory bullet and the changelog.

**PR.** [#859](https://github.com/eyupio/reet.sh/pull/859) opened against `main`; the checks above are this PR's.
**Merged** as `92e38f50` on 20 November 2026; its facts and outcome-bar change are in `main` and this branch is merged up to it.

## FACTORY-OUTCOME-IMAGES-2: every attached image travels into a started agent (20 November 2026)

Every agent a Factory outcome starts now receives **all** of the outcome's images, not only the first. `gaffer.OutcomeLaunch` gained `Images []string`; `runOutcomeSession` fills it from the record (`o.Images`), so the same list reaches the first approval start, a retried pipeline, a later stage, and a pipeline added after approval. It is a copy, never the store's slice.

The launch path (`outcomeStarter.StartOutcomeSession`) builds the opening prompt with `outcomePromptImages`: it turns each raw machine path into a reference (`uploadReference`, so a path with spaces or quotes is one shell-safe word), then appends one `Image:` line or one `Images: a b c` line. An image the prompt already names is left alone, so an outcome saved by the previous part (whose session prompts already carry references) is not doubled, and an outcome with no images sends exactly the prompt it did before. Because the prompt line is built at the launch, new pipelines and MCP-added pipelines get the images too.

The browser no longer writes image references into the approved session prompts: `GafferOutcome.svelte` stores `upload.paths` on the outcome and sends the person's clean prompt. This leaves the outcome record as the single source of truth for what every started agent receives. The plan step now says "All N attached images travel with every pipeline this outcome starts", the result step still lists the stored paths, and AddPipelines says the outcome's images travel with each new pipeline.

**Ambiguity.** The previous handover said the approval prompt already carried references and a later start should read `Images`. I made the launch the only place that adds them, rather than trying to keep two writers in step; existing stored prompts that already carry references are detected and not duplicated.

**Checked.** `gofmt`, `go vet ./...`; `go test ./internal/gaffer ./internal/api ./internal/store ./internal/terminalproto`; targeted `-race` on the new tests; `go test ./...` (only the pre-existing terminal-pipe failures in `internal/clientconfig` and `internal/lifecycle` fail in this non-TTY shell); `npm run test`, `npm run build`, `svelte-check`; and `web/tests/gaffer-outcome.e2e.mjs` against a locally built preview (all desktop and phone cases passed, including eight attached images with one upload failing). A Go test starts an outcome holding three images through the controller and asserts both its launches carry all three and the machine receives one `Images:` line with every path; another covers zero images and an already-referenced prompt.

**Review (20 November 2026).** Re-read the two commits (`21e35143`, `e7a7dc07`) and confirmed: the only construction of a launch with prompts is `runOutcomeSession`, so the first start, a retry, a later stage and an added pipeline all take `o.Images` from the stored record; the browser sends a clean prompt and stores only paths; zero images return the prompt unchanged (tested by `TestOutcomePromptImagesLeavesAPromptWithNoImagesAlone`); the attach UI from IMAGES-1 was not reworked. Re-ran gofmt, vet, the Go packages above, `npm run test`, `svelte-check`, `npm run build`, and both `gaffer-outcome` and `gaffer-goal` e2e against a fresh preview: all pass. `go test ./...` still fails only in `internal/clientconfig` and `internal/lifecycle`, which this branch does not touch (terminal-pipe tests in a non-TTY shell).

Two fixes: the partial-upload result sentence counted the *failed* files instead of the attached ones, so one failure among seven said "the other image was attached" — it now counts `upload.paths` ("the other 7 images were attached"), and the e2e asserts the corrected sentence. The upload cache is now keyed by machine as well as file, so a path uploaded to one machine cannot be reused after the person switches project to another machine. Comments in `store.GafferOutcome`, `gaffer.OutcomeInput` and the web types now call `images` machine paths rather than references.

**Review thread fix (20 November 2026).** Codex's P2 on `internal/api/gaffer_outcomes.go` was right: `CreateSessionReserved` validates the prompt, so appending the image line after a prompt already at 8,000 bytes would refuse the launch. The line is now reserved where the prompt is built. `stagePrompt` leaves room for `outcomeImagesSuffix("x", o.Images)` before it clips the brief, so a staged prompt at the limit keeps both its boundary and every image. An unstaged brief and an added pipeline are checked with the same image line at approval (`CreateOutcome`, `addPipelinesOnce`) and refused there if the two cannot fit together. The line itself moved to `gaffer.OutcomePromptImages`; the launch path calls it, and `terminalproto.UploadReference` is the one place an upload path is quoted.

Open for the test/PR stages: no real machine was exercised. A later stage's launch is covered by the shared `runOutcomeSession` path but has no test of its own.

## FACTORY-OUTCOME-IMAGES-1: several images on a Factory outcome (20 November 2026)

`gaffer_outcomes`' JSON gains `images`: the raw machine upload paths the person attached, in order, at most eight (the ACP ceiling). Outcomes saved before this change simply have no field and read with none. `gaffer.OutcomeInput.Images` validates each as an absolute upload path and `store.GafferOutcome.Images` keeps them.

At approval the browser uploads every selected image through the ordinary machine launch upload and stores the successful paths on the outcome. (IMAGES-2 moved the writing of `Image:` / `Images:` references out of the browser and into the launch path, so this record stays the single source.) If one of several uploads fails, the others are attached and the result names the failed file; a single failed image still refuses approval as before.

`web/src/lib/imageAttachment.ts` now has `attachmentsFromPaste` and `MAX_ATTACHMENTS`. `web/src/lib/ImageAttachment.svelte` binds `files: File[]` with a `max` (1 in the goal, 8 in an outcome), allows multi-select, removes each image on its own, and drops only the image whose preview cannot be read. `GafferOutcome.svelte` owns the list and the uploads; `GafferGoal.svelte` uses the same picker for its one image.

For the part that carries images into later agent starts: `GafferOutcome.Images` are raw machine paths, not shell-quoted references and not bytes. A later start reads `Images` itself rather than parsing the prompt; IMAGES-2 carries them on `OutcomeLaunch.Images` and turns them into references at the launch.

## QUEUE-AFTER-FINISH-1: a queued message goes after a finished turn (20 November 2026)

The owner reported that a message typed after an agent finished stayed queued;
the only way out was **Stop and send**, which then seemed to time out and needed
**Retry**. The chat transcript and its `Finished` attention were already idle
while the machine's live controls still reported `working`.

**Cause.** OpenCode 1.19.5's HTTP event stream emits `session.status: busy` more
than once in a turn and can deliver a late busy after the idle that ended it.
`acpagent`'s `nativeBusy` mirrors those events, so `Controls().Working` stayed
true after the turn. The controller's queue waited behind it. A missed idle on
the stream had the same effect. **Stop and send** cleared it because
`session/cancel` provokes a fresh idle, but when its acknowledgement was not
confirmed within ten seconds the queue marked the message failed, which is why
the person then had to press **Retry**.

**Fix.** `native_view.go` now polls `/session/status` every five seconds while
it believes a native turn is running and reconciles `nativeBusy` from that
snapshot; the event stream stays the fast path and a failed snapshot never
invents an idle. `chat_queue.go` keeps a message queued when a stop's
acknowledgement is transiently lost or refused, keeps the `stop` intent so the
next pass asks again, and only fails it when the chat is confirmed closed. The
message therefore goes by itself once the turn is over, without Retry.

**Tests.** `TestNativeStaleBusyIsReconciledWhileConnected` proves a stuck busy
clears through the snapshot poll; `TestTransientStopFailureKeepsTheMessageAndSendsWhenTheTurnEnds`
proves a refused stop no longer becomes a failure. `go vet ./...`,
`go test -race ./internal/acpagent ./internal/terminalbroker` pass. No real
machine or paid provider was used for acceptance; the live symptom came from the
owner's own OpenCode 1.19.5 chat on this machine. Recorded in CHANGELOG.md and
docs/structured-sessions.md. Merged as [#856](https://github.com/eyupio/reet.sh/pull/856) (`ea7a0042`).

## FACTORY-DECISION-1: a lost decision is asked again, and the session says who decides (20 November 2026)

The owner sent a phone screenshot of a Factory stage's decision in its session: the recommendation and its instruction
filled the screen, the buttons sat under the fixed-height workspace where nothing could scroll them into view, and the
card only said "Gaffer needs your decision" on an outcome they believed Gaffer was deciding. They asked what had
happened and what they had to do.

**Why it was not sending itself.** Two defects, not a missing feature. A stop is claimed before Gaffer's model is asked
so a slow request is not asked twice; if the controller stopped between that claim and recording the choice, the claim
stayed and `autoDecisions` skipped the stop for ever while the session said "Gaffer is deciding". The stage now records
`decidingSince`, and a claim with no recorded decision is asked again after `decisionClaimTTL` (two minutes) without
spending another of the stage's three decisions. A decision that failed is still recorded, answers the claim and stays
with the person. The second: the Factory card said "Gaffer is deciding this" even after a failed or left-to-the-person
decision; both the card and the session now say it is yours and point at what Gaffer decided.

**Reading the card.** The session card for a goal stop now always says who decides: "You decide this outcome's
decisions. Pick one below, or turn on Gaffer decides on the Factory card…", the three-decision cap, that Gaffer left it
to you (with its reason on the Factory card), or that Gaffer is deciding now. Gaffer's latest review opens in place
under **Gaffer's latest review** instead of telling the person to read a review the session did not show. On a phone
the decision block keeps a bounded height and scrolls, and the choices stick to its foot, so a long recommendation
cannot bury its own buttons. A lost session or a merge carries no decisions note: those are the person's whatever the
setting.

**Data.** `GafferOutcomeStage.DecidingSince` (`decidingSince` in the outcome JSON; absent in older records, which are
treated as stale claims). No migration; older records are read as before.

**Tests.** `go test ./internal/gaffer` (including `TestAStopWhoseAnswerNeverCameIsAskedAgain` and
`TestAStopIsNotAskedAgainWhileClaimedOrOnceAnswered`), `go test ./internal/store`, `go vet ./...`,
`go test -race ./...`, `cd web && npm run check`, and against `vite preview`: `factory-decision.e2e.mjs` (including a
long recommendation at 390 whose last choice must scroll into view), `gaffer-factory.e2e.mjs`,
`gaffer-factory-session.e2e.mjs`, `factory-pieces.e2e.mjs` and `session-workspace.e2e.mjs`.

**Docs.** [Factory](docs/factory.md), [Gaffer AI](docs/gaffer-ai.md), [UI verification](docs/ui-verification.md) and
the changelog.

**Merged.** [#854](https://github.com/eyupio/reet.sh/pull/854) as `ec3a3343`. Every required check on the final head
passed.

## PLAN-JOB-1: a Gaffer plan survives a proxy that gives up waiting (20 November 2026)

**Merged.** [#851](https://github.com/eyupio/reet.sh/pull/851) as `da10b002`. Every required check on the final head passed.

The owner reported intermittent `502` errors when asking Gaffer for a Factory plan, and sometimes when starting an
agent. The screenshot showed a bare `502` in the outcome dialog. That number is not reet's: every reet 502 writes a
JSON body with a sentence, and `web/src/lib/api.ts` falls back to the raw status only when the body is not JSON. A
bare `502` therefore comes from the gateway in front of reet. The owner confirmed **Cloudflare**, which ends an origin
response after about 100 seconds.

Planning asked the provider on the browser's own request and could hold it for up to 90 seconds. A slow reasoning
model (the report used `deepseek-v4.1-flash:1017` on Ollama Cloud) pushed that close enough to the edge limit that
Cloudflare sometimes cut the connection; the browser then had only the status number to show.

**How it works.** Planning is now a background job. `POST /api/v2/gaffer/providers/{id}/plan` validates the ask
synchronously (so bad input still answers 400/422 before any provider call), registers a job, starts the provider
call detached from the request context, and answers `202 {"id","state":"running"}` at once. A new
`GET /api/v2/gaffer/providers/{id}/plan/{job}` answers `{"state":"running"}` while the model is working and the
finished plan afterwards; a provider or plan-shaping failure is mapped with the existing `gafferError` (so the
sentence is unchanged). `StartPlan`/`PlanResult` live in `internal/gaffer/outcomes.go`: jobs are per account and
provider, finished plans are kept for five minutes, and `s.begin` still applies the account's concurrency limit.
The browser (`planGafferOutcome` in `web/src/lib/api.ts`) starts the job, then polls once a second with the existing
abort signal; every request it makes is short, so no proxy can time it out.

**Tests.** `internal/api/gaffer_plan_test.go` starts a plan, polls it to the same plan and checks the provider was
asked once; `TestStartPlanRunsInTheBackgroundAndIsReadWithoutAskingTwice` in `internal/gaffer/outcomes_test.go`
covers the running state, another account's or provider's job answering not-found, and a second read returning the
same plan without another provider request. `web/tests/gaffer-outcome.e2e.mjs` mocks the 202 then the read.

**Checked.** `go vet ./internal/gaffer ./internal/api`; `go test ... -run 'TestPlanOutcome|TestGafferOutcomePlan|TestStartPlan|TestPlanOutcomeRatesEachStage'`;
`npx svelte-check` (0 errors, 0 warnings); `npm run build`; and the whole `web/tests/gaffer-outcome.e2e.mjs` passed
against `vite preview` on desktop and phone. Not checked against a real Cloudflare-fronted controller.

**Still open.** Starting an agent still holds its create request for up to 60 seconds (structured chat) or 180 seconds
(worktree) in `internal/terminalbroker/broker.go`. 60 seconds is under Cloudflare's limit; the worktree path is not,
so a slow worktree create can still be cut at the edge. The browser already gives up such a start after 60 seconds
and offers to resend the same request id, so the agent can still be running. A real fix would give the create the
same background-job treatment as the plan; it is not included here.

## SKILLS-DOCS-18: document the shared library (20 November 2026)

Continuation of merged PR #845 (`820631e7`), whose final head was `7f9b297a`.
The final head's checks passed, including benchmarks and both real-agent jobs;
the Archived published-skill picker finding is fixed and its thread resolved.

Task 18's documentation pass replaces the superseded user guides for plans,
stages, pins and adoption with Active/Archived/Delete, discovery review, saved
content, history, local consent and recovery. README and changelog now describe
the new model. SECURITY.md already documents outbound GitHub/site bundle fetches.
Verification and limits are recorded in docs/ui-verification.md.

**Task 17 continuation (PR #850).** Removed the old web engine, its exclusive
helpers, API wrappers and tests: Advanced sync, plan selection, private pins and
adoption, per-machine skill selection, manual skill sync and the job activity
dock. Connection, inventory, manual content import, connector updates, pause,
terminal access and local backup recovery remain. Old plan URLs redirect to
`/sync`, discarding plan parameters. Home and session details now read the
reconcile report instead of inferring skill state from preview jobs. Add a
machine waits for that report before claiming library sync; unavailable reports
remain unknown. Write a skill points to automatic library status.

Checks and captures are recorded in docs/ui-verification.md. The local computer
has no Go language toolchain (`/usr/bin/go` is an unrelated application), so Go
vet/race verification belongs to CI. Next: finish the exact-head CI and review
checks on #850; no live machine or paid provider has been used for acceptance.

## LATEST-CODE-1: fresh worktrees can start from GitHub's latest (19 November 2026)

The owner asked: "If we recognise that a machine folder doesn't have the latest code from GitHub, then we need a config
setting to always pull latest code into the worktree, but still allow the user to override when starting a new agent
session or factory outcome." Asked for the dynamic, interactive UI, the answer is a layered choice rather than a new git
operation: **start from GitHub's latest commit** or **this machine's copy**.

**How it works.** A fresh worktree that names its base can be started from GitHub's own branch. The controller does it
without a new wire field by naming `origin/<base>` to a machine that negotiated `base-fresh-v1` — the machine already
fetches that branch and starts from what it fetched. The machine's local branch is never pulled, moved or cleaned.
`isolation.go`'s `pullLatestFor` layers the answers: the launch's own (`Create.PullLatest`, controller-only and never
encoded), else the project's `pull_latest` (`inherit`/`yes`/`no`, default `inherit`), else the profile's
`pull_latest_by_default` (default off). Factory outcomes carry the person's answer as `OutcomeInput.PullLatest` /
`store.GafferOutcome.PullLatest` / `gaffer.OutcomeLaunch.PullLatest`, and inherit automatically when they do not.

**Data.** `folder_projects.pull_latest` (CHECKed, added by migration; older rows inherit) and
`user_profiles.pull_latest_by_default` (added by migration; older rows 0). Backups carry both. The folder-project view
and input gain `pullLatest`; the profile gains `pullLatestByDefault`.

**Web.** `launch/LatestCodeChoice.svelte` is the shared control: two buttons, and one sentence under them from the
read-only base check ("is in step", "is behind", "has commits GitHub doesn't have", GitHub unreachable), with a
**Check GitHub** / **Check again** action, a green/amber dot and a reduced-motion-safe pulse while it reads. Opening a
dialog does not fetch GitHub by itself: the person asks, or Start's own base check answers both (its result is handed
back to the control), so one start never makes the machine fetch the same branch twice. It is in Start an agent (for a
repository that will get a worktree), in the New Factory outcome dialog, and its account default is a select in Settings
→ Agents; Add a project gained "Latest code from GitHub" (inherit/always/never). `launchBar.ts` exposes
`startsFromLatest(project, profile)`, false when nothing says yes.

**Not done.** Home's launch bar and the desk composer have no per-start toggle: they inherit the project's and
account's answer, which the controller applies. No real machine was exercised end-to-end; the `origin/<base>` path is
the one the base check and its tests already cover.

reet.sh is a self-hosted AI fleet control room: machines, the AI agents installed on them, skills,
reviewed sync, browser terminals and, now, an agents home with folder projects. This file is the
current state and the standing rules; start here. Where things are: [README.md](README.md) and
CONTRIBUTING.md; [docs/](docs/) for shipped behaviour (for example
[control centre](docs/control-centre.md), [terminal workspace](docs/terminal-workspace.md),
[projects](docs/projects.md), [folder browse](docs/folder-browse.md),
[remote machine operations](docs/remote-machine-operations.md),
[controller terminal commands](docs/terminal-controller-commands.md),
[OpenCode in Gaffer](docs/structured-sessions.md) and
[voice and copy](docs/voice-and-copy.md)); the agents-home mock-up in
[docs/design/agents-home/](docs/design/agents-home/), its
[plan](docs/superpowers/plans/2026-10-26-agents-home.md) and
[spec](docs/superpowers/specs/2026-10-26-agents-home-design.md); the 2 November UI refresh contracts in
`docs/superpowers/specs/2026-11-02-*.md`; what each UI change was checked against in
[docs/ui-verification.md](docs/ui-verification.md); the research behind the queued work in
[docs/research/](docs/research/); and older narrative in
[handoff-history.md](handoff-history.md).

## FOLDER-BROWSE-1: start an agent by picking a folder (19 November 2026)

The Start an agent dialog's **Folder** field (under **No project — start in a folder**) only offered the
project folders a machine had reported plus **Type a path…**. A machine that reports folder browsing now shows
**Browse…** beside the select, opening the existing `FolderBrowser` inline
(`web/src/lib/StartAgentDialog.svelte`). A folder picked there fills the choice, including a folder the machine
never reported (it becomes its own option); picking the home sets the empty **Home folder** value. The browser
opens at the current choice, closes on a pick, and closes when a project, machine or agent change resets the
folder. Typed paths stay for machines that cannot browse (older reet, browsing switched off, offline); the
controller, broker and connector are unchanged. Browse requests are queued one at a time, as the Add project
dialog does, so a filter typed while a folder is still listing never draws the machine's "already listing"
refusal.

**Tests.** `web/tests/start-agent.e2e.mjs` gives the fixture machine `browse`, mocks
`GET /api/v2/machines/{id}/browse`, and checks the button, the live list, a pick filling the select and the
browser closing. `cd web && npm run check`; the full `start-agent.e2e.mjs` passed against `vite preview`.

**Docs.** [docs/start-agent-session.md](docs/start-agent-session.md) and README name the browser.

**Merged.** [#848](https://github.com/eyupio/reet.sh/pull/848) as `28c22e4b`.

## USAGE-1: one Usage page for every agent, and plan limits always on (18 November 2026)

The owner asked: "Add a single usage page for all agents, plus on by default." Asked in turn, they chose: the page
shows **plans plus tokens used** (per agent, the plan left for each account and window, then tokens used by its
sessions and Factory stages, today, 7 days and 30 days, with the sessions behind them; every agent appears, and one
that reports nothing says so); plan limits **"Always on, no switch"**; and placement as **an item in the More menu**
on every screen size, with **"See all usage"** in the plan chips' panel. The primary tabs did not change. The context:
their agent (OpenCode on a hosted model) has no plan reet can read, so Home showed nothing; the page must never be a
silent blank.

**Scope change during the work.** The owner then said the plan as described "isn't ideal, it should just work: if a
user has an agent on a machine, show usage stats", and chose to have machines also read OpenCode (Go/Zen), Antigravity
and Copilot plans; those readers come from a separate branch. So nothing here names a fixed pair of agents: a block
draws whatever accounts and windows any machine answers for any tool (scoped and month/day windows included), each
block leads with its token figure (for an agent with no plan, the only figures there are), and the no-figure sentence
comes from the answer's states (`planSentence`, `planNote` in `usage.ts`): no sign-in, API key or no plan allowance,
"<Agent>'s sign-in on <machine> has run out; it renews itself the next time <Agent> runs there", provider slowed down or unreachable (last figures dimmed), terminal access
off, no connected machine reporting sign-ins, and only where no machine answers for the agent at all, "reet can't read
this agent's plan". Settings → Agents says "the agents reet can read a plan for" and links to the sign-ins doc.

**Controller.** `AddTerminalSessionUsage` now also writes a dated row to `terminal_usage_events` (user, session,
machine id, tool, model, structured, at, input, output, cached; index on `(user_id, at)`) in the same transaction as
the running total, for every report with tokens. The session's latest title, machine name and project are kept in
`terminal_usage_sessions` (one row per session, updated on each report), so a row survives archiving, forgetting or
revoking the machine; while the session exists its own current title is shown. Both go with the account (trigger) and
are pruned on write after 90 days, at most 500 rows per write (as `remote_health_summaries` prunes on write). Archiving
no longer touches them. `UsageFor` sums per tool (sessions, Factory stages, first event), per session (bounded, 200,
`more`), and Gaffer's own `gaffer_turns` by provider and model. Factory marks come from the kept outcomes
(`gaffer_outcomes`, 30 kept): every stage's `sessionId` and a followed session's own. `GET /api/v2/usage?from&to`
(`internal/api/usage.go`): `terminalAccess(…, true)` + `sameOriginRead`, so it answers with remote terminals off and
**refuses viewers**, as the session list does; range must end after it starts and be at most 92 days. `agents` lists
`terminalproto.LaunchTools()` in order (the catalogue mirror, now an ordered list; a terminalhost test keeps it equal to
`Catalogue`, and `reports` = `chat` when `ReviewedChatTool`, which the same test ties to a preset's ACP adapter).

**Always on.** `internal/api/accounts.go` always asks with `Limits` (refresh with `Refresh`), never strips figures,
and re-reads a recent answer that says `not_asked`; the view keeps `planLimits:true`. The profile always answers
`planLimits:true`; `PATCH` accepts it as a no-op and does not audit it; the `plan_limits` column stays, unread and
unwritten. Balancing reads kept figures as before, so it now works for accounts that never opted in; the hints and
docs say it needs the machine to be able to read the agent's plan.

**Web.** `/usage`, More → Workspace → Usage for `sessionsAccess`. Period buttons with `aria-pressed`, remembered as
`reet.usage.period.<userId>`; ranges from the account's time zone (`usage.ts`, DST-safe); quiet refresh each minute
while visible; table above 700 px, cards below. Plan blocks reuse `planEntries`, `limitLines`, `limitsNote` and
`machinePlans`. Settings → Agents lost its fieldset; MachineSettings lost its off-state branches. `resetsIn` rounds to the minute
before splitting hours (it could say "2 h 60 min").

**Tests replaced.** Go: `TestMachineAccountsFirstReadAsksForSignInsOnly` → `…FirstReadAsksForTheAllowance`;
`…PlanLimitsFollowTheAccountSetting` → `…AnAnswerWithoutTheAllowanceIsReadAgain`; `TestMachineAccountsRefresh`
rewritten (always fresh figures, audit says limits true); `TestProfilePlanLimitsIsOffUntilTurnedOn` →
`TestProfilePlanLimitsIsAlwaysOn`; helper `planLimits` → `oldClientPlanLimits`. Browser: in `account.e2e.mjs` the
blocks "plan limits switch", "a refused plan limits switch puts the box back" and "plan limits switch is read-only for
a viewer" → "usage note replaces the plan limits switch" and "a viewer sees the usage note without a switch"; in
`agent-accounts.e2e.mjs` "sign-ins without plan limits" → "sign-ins before any plan is read" (no off hint), plus a
check that figures show whatever `planLimits` says. The More list grew Usage: `navigation.e2e.mjs`'s three More-list
assertions now include it, and `supporting-nav.e2e.mjs`'s "a writer has Teams alone in Workspace, no row" became "a
writer's Workspace row is Usage and Teams" plus "a viewer has Teams alone, no row".

**Data limits.** Totals start when this version is installed (nothing earlier is dated). Reports made while a machine
is disconnected are lost (never queued). Only chats report: plain terminals, Copilot and machines without
`acp-usage-v1` count nothing. Factory marks rely on the 30 outcomes kept. Gaffer's figures go when a Gaffer
conversation is deleted. Events older than 90 days are removed when the same account next reports usage.

**Not checked.** A real controller, machine or provider; a screen reader; a real tablet; the page under a very large
history (the session list is bounded at 200, the per-tool sums are not).

**Review fixes (PR #840).**
- Copilot reads only `github.com` and `github.com:<app>` keys; a look-alike host (`github.company.com:…`,
  `github.comcast.com`) is never read, so its token never reaches api.github.com.
- Balancing (`planLeft` in `internal/gaffer/outcome_balance.go`) now judges a plan whose windows are all labelled and
  none for the stage's model (Copilot's Premium, Chat, Completions) by its tightest window, instead of reading it as
  signed in with no figures. A plan with a plan-wide window is judged as before. A backup whose figures are over six
  hours old is chosen "whose plan figures are too old to go by", not "has no plan figures". With plan limits always on,
  a backup agent with a sign-in but no figures at all (a key, or a plan reet cannot read) can still be chosen, "which
  has no plan figures to run low": that is by design (the person chose it as the backup) but worth knowing.
- `readFile` looks at a sign-in path before opening it and opens without waiting, so a named pipe or device there is
  unreadable at once instead of blocking the read (`fifo_test.go`).
- Time: OpenCode's store wait is 1 s, providers 9 s, other sign-in files 1 s: `agentaccounts.ReadTakesAtMost` = 11 s,
  a second under the connector's 12 s `accountsAnswerAfter`; a terminalagent test keeps it so.
- Antigravity asks its second endpoint only on a failure (no answer, 5xx, 404); a 429 is reported rate limited and its
  Retry-After honoured. With no identity token, an Antigravity sign-in is told from the next by `keyMark` (moved to
  `accounts.go`) of its refresh token, so a new sign-in is not shown the last one's figures.
- The "renew" sentence on the Usage page now says the agent renews its own sign-in when it next runs. An agent reported
  `retired` with figures says reet no longer starts it. The headline's session count is the controller's
  `sessionCount` (each session once), not the sum of the agents' counts.
- The OpenCode percentage is read as *used* (`usage.*.percent`). This was not checked against the provider: if the page
  ever shows an OpenCode plan upside down (full when it is nearly spent), that is the place to look.
- An answer that an account has no plan with limits stands for five minutes (`freshFor`), like figures; a plan bought
  in those five minutes shows after them, or after **Check sign-ins** once 30 s have passed.

## DESK-SESSIONS-1: desk sessions off the Sessions page, unless they need you (19 November 2026)

The owner asked: "Desktop sessions shouldn't appear in sessions page." By the same rule as DESK-HOME-1, a session on a
non-template desk leaves the Sessions list; one that needs the person stays (an approval, a question, a failure or work
to review), still with its desk chip. A Factory pipeline's session is not special-cased here: it is on the desk, so the
desk holds it.

**What was built.** `SessionsPage.svelte` reads `listDesks` in its 15-second batch (bounded like the other reads) and
filters through `offDesks` (`web/src/lib/home/deskSessions.ts`) with the needing set it already computes
(`needsYou`/`needingIds`). The list, search, filters, agents, project running counts and worktrees all use the filtered
list; the desk chip and pane reads use it too, so only a needing session's desk is read. Add to a desk reads the desks
again before reading the pane, and a quiet desk session that starts needing the person has its desk read on that poll,
so its chip, pane and colour are there at once. A failed desk read keeps the last list and never adds a problem line,
and with no desks read every session shows as before. When every remaining session is on a desk the page says "Every session here is on a
desk. Open Desks to see them." instead of "No sessions match."

**Checked.** `web/tests/sessions.e2e.mjs` gained the "desk sessions" section: quiet and finished desk sessions leave the
list, a waiting one stays with its chip (`/desks/desk1#pa`), and a 503 desk read shows every session without a problem
line, and a quiet desk session that starts waiting enters with its chip on the next read. `web/tests/desks.e2e.mjs` was
updated: a session added to a desk leaves Sessions, the moved sessions wait so they
stay listed, and the chip block now holds one waiting session on `desk-prod` (quiet `desk-split` sessions are off the
list). Full `sessions.e2e.mjs`, full `desks.e2e.mjs`, `npm run check` and the web build pass.

**Not checked.** A real controller and desk; a session that is both a waiting desk session and a Factory pipeline's; the
page with more than 100 sessions and many desks. Docs updated: `README.md`, `docs/desks.md`, `docs/session-board.md`,
`docs/control-centre.md`, `CHANGELOG.md`.

## DESK-HOME-1: desk sessions off Home, unless they need you (18 November 2026)

The owner asked: "Do not show desk sessions on the home page." Asked whether a desk session that needs them should still
show, they chose: "Yes, only when it needs you".

**What was built.** Home reads the desk list with its other reads (`listDesks` in the same `Promise.allSettled` batch).
`offDesks` (`web/src/lib/home/deskSessions.ts`) takes a session on a non-template desk out of Continue working, the
Planning board and the head's "n sessions open", so a person whose only sessions are on desks sees **Nothing in
flight.** A Factory pipeline's session is kept (it is its Factory item). What needs the person is unchanged:
`whatWaits` takes the desks too and marks a waiting session (or a Gaffer goal on one) with `desk`, its link becoming
`/desks/<id>#<pane>`; `deskPaneReader` reads only the desks that hold a waiting session, once per desk version, and
without the pane the link is `/desks/<id>`. `SessionWaitingItem` and the bell add " · Desk <name>" to the place. The bell
off Home (`loadAttention` in `App.svelte`) reads the desks the same way (desk reads are plain fetches and never drive the
progress bar). A failed desk read keeps the last desks, never signs out and never adds a problem line; before any read
no session counts as a desk session. The Projects rail's running counts leave desk sessions out too (the owner's choice, so the
panel matches what Home lists). Left as they were, deliberately: the rail's worktrees,
the machine cards, owners, the waiting announcement and the launch bar's recent models still see every session.

**Checked.** Unit: `deskSessions.test.mjs`, four new cases in `whatWaits.test.mjs`. Browser: four new blocks in
`web/tests/home-waiting.e2e.mjs` (marked entry and link on Home and in the bell, count and title, Continue working and
board without desk sessions, phone card, only desk sessions, desk read failing with 503).

**Navigation, same branch.** The owner then asked: "Make desk menu more primary, after factory, before machines and
accessible", and on the order: "Home, sessions, factory, desks, ...". The tabs (`tabs` in `App.svelte`) now run Home,
Sessions, Factory, Desks, Machines, Skills (Activity after them without accounts), each on its old condition. Desks
joined the bar (`barTabs`): the phone's bottom bar is Home, Sessions, Factory, Desks, Machines, More, and a tablet's top
bar the same; More no longer lists Desks there. `BottomTabs.svelte` gives Desks a split-frame icon and sizes each tab to
its label (`flex: 1 1 auto`, 1 px padding, labels never wrap) so six fit at 320 px with 14 px labels. On a tablet
`TopTabs.svelte` sets the tabs 2 px apart with 10 px padding (six overflowed their box by 43 px at 834 px) and every
top tab is at least 44 px wide. Checked in
`navigation.e2e.mjs`: the order, the bar at 320, 360, 390 and 412 px (44 px targets, no overlap, no clipped label,
no sideways scroll), Desks current on `/desks` and `/desks/<id>`, Tab order with a focus ring, and the tabs clear of
Jump and the bell at 651, 834, 1100 and 1280 px. Not checked: a screen reader, and a 320 px phone with a larger
system font size.

**Not checked.** A real controller and desk (fixtures only); a desk session that is also a pipeline's, beyond the unit
case.

## CREW-CHOICES-1: the crew chosen from what the machines offer, and an effort that is not offered (17 November 2026)

The owner asked, with a picture of Gaffer settings → **Your Factory crew**: "The gaffer settings need a dynamically
generated dropdown with the actual values for the models and efforts, etc. Also, ensure the effort selection doesn't
cause any issues for agents/models that are not supportive." Built on `ccr-93f68fda-ippwiu` (draft PR #834), in two
parts: the controller first, then the form and these docs.

**What was built: the controller.** A stage whose effort its agent does not offer starts on the agent's default effort
instead of stopping, and says so (`internal/gaffer/outcome_effort.go`). Before a start, the machine's last capability
report is read (`effortNotReported`): if it describes the agent's options and lists no such thought level, the effort
is not asked for. If a start is refused for the effort all the same, the stage is asked for once more, under a fresh
request, without it (`dropStageEffort`). Either way the stage keeps the effort chosen in `EffortNotOffered` with
`Effort` empty, the activity log records `gaffer-stage-effort-not-offered`, the choice check reads **Confirmed** with
the reason, MCP's stage carries `effort_not_offered` (`internal/api/gaffer_factory_mcp.go`), and the Factory page's
stage facts say "agent's default effort (high is not offered)". A wrong model still stops the stage. Backups and
balancing treat a dropped effort as the one asked for. Trying a pipeline again or sending it back asks for the dropped
effort again, so a machine that offers it by then gets it, except while an earlier start of that stage is still
unanswered (`UnsureRequest`: that start was asked without it and is taken up as it is); Gaffer's own decision to run a
stage again and the restart on a backup ask again too. A machine whose reet takes no start options keeps its own
refusal, which says to update it. The activity log's `reason` is short ("OpenCode does not offer it for this model
here."), since the line already names the stage and the effort.

**How the refusal is recognised.** The machine refuses an unconfirmed effort with the generic code
`ACPUnavailable`, which also covers other refusals. The controller tells this one apart by the machine's exact
sentence, the constant `terminalproto.ACPEffortUnconfirmedDetail` (`terminalbroker.ACPUnavailableError.EffortNotConfirmed`).
The sentence is pinned on both sides: the connector's side by
`TestChosenEffortRefusalSaysTheSentenceTheControllerRecognises` (`internal/terminalagent/chat_test.go`), the
controller's by `TestAnEffortRefusalIsToldApartFromTheOthers` (`internal/terminalbroker/chat_effort_refusal_test.go`)
and `internal/api/gaffer_outcomes_test.go`. Changing the sentence on either side fails a test. The proper fix later is
a result code of its own from the machine, so nothing hangs on wording.

**What was built: the form.** `web/src/lib/GafferCrewSettings.svelte` no longer has boxes for model and effort.
- **Model** is `GafferModelSelect` fed one merged list per agent: GET `/api/v2/models?tool=` (`allModels`, the cached
  rows of every machine, no machine is asked), merged by `mergeMachineModels` (`web/src/lib/crewChoices.ts`: each id
  once, provider kept). The select gained `everywhere` (groups read "On your machines", a missing favourite "not on
  your machines") and `note`; a saved model no list has stays, under "Current", with "None of your machines lists this
  model. If the agent does not offer it, the stage stops when it starts." Favourites are read as the outcome dialog
  reads them.
- **Effort** is a select of `effortUnion` over every machine's `reportedEfforts` (low, medium, high, then the rest as
  reported), with **Default effort** first; `crewEffortChoices` gives **No effort setting**, disabled (described to a
  screen reader as "OpenCode has no effort setting"), only when a machine runs the agent and reports no level and none
  is saved, and keeps a saved level none offers as "High (not offered by your machines)" with the hint "If the agent
  does not offer this effort, the stage starts on the agent's default effort." When nothing is known for the agent (no
  machine runs it, the account has none, or the read failed) it offers Default, low, medium and high, the start levels
  of `launchOptions.ts`, with the same hint. The hint and the model's note are tied to their selects
  (`aria-describedby`); the failed-read line is a polite live region.
- **Agent** keeps the fixed list (a crew may be set before a machine connects); one no machine runs as a chat
  (`structuredAgent`) reads "(not on your machines)". Changing a row's agent clears its model, effort and Cline source.
- Reads: the crew, the machines and the favourites once on mount; each agent's models once, when it is first in the
  crew. No polling. A failed read is one line ("Your machines could not be read…"), the model select falls back to the
  agent's default, favourites, suggestions and **Other…**, the effort to the start levels, and saving is never blocked.
- The model note (`crewModelNote`) waits until the agent's list is read; when no machine has listed any model for the
  agent it says "Your machines have not listed this agent's models, so this name is not checked." **Other…** on a
  saved model no list has (the "Current" group) keeps it in the box, at most 100 bytes (`terminalproto.MaxModelBytes`);
  on a listed value it still starts from an empty box. Further backups are keyed by a per-row id, so removing one
  leaves the next as it was.
- The outcome dialog and **Add pipelines** now drop an unreported effort from every backup, further backups included
  (`withReportedEfforts` in `gafferOutcome.ts`); before, the dialog dropped only the first backup's and Add pipelines
  dropped none. When the machine or its capabilities are not known (not read yet, the read failed, or the outcome's
  machine is gone) nothing is dropped and the plan goes as chosen (`withReportedEfforts(plan, null)`): the controller
  drops an unoffered effort itself, and stripping on a guess only lost the person's choice. Add pipelines then offers
  the start levels; the outcome dialog cannot be approved without a machine, and clears nothing until it has one.

**Tests.** Unit: `crewChoices.test.mjs` (new, in `npm run test`), `models.test.mjs` (`everywhere`),
`gafferOutcome.test.mjs` (every backup's effort). Browser: `gaffer-settings.e2e.mjs` has the crew checks (options from
the machines' reports, OpenCode's **No effort setting**, the agent change that clears, a kept unlisted model and
unoffered effort, exactly what is saved, a failed read that still saves, 1440 light and dark, 390 light and dark);
`gaffer-factory.e2e.mjs`'s crew block now chooses from the selects (its two `fill` calls on the model and effort boxes
and the saved values they led to were replaced). `tests/helpers/workspace.mjs` answers `/api/v2/models` with no rows.

**Not covered.**
- Efforts are reported per agent, not per model, so the form (and the outcome dialog) may offer a level a particular
  model lacks; the stage then starts on the default effort.
- On a machine that reports nothing about its agents' options, every start of a stage with an unoffered effort, each
  **Try again** and **Send back** included, costs one refused start before the retry, and each refused start leaves one
  "lost" row in Sessions history.
- If the controller stops between the refusal and saving the retry, the stage reads as failed, and **Try again** goes
  the same way.

**Known and left.** `SendPipelineBack` (`internal/gaffer/outcome_manage.go`, around lines 647-670) rebuilds the stages
from the target on but does not clear the pipeline's `UnsureRequest`. Sending a pipeline back while a later stage's
start is still unanswered can therefore make the earlier stage take up, or ask again under, that old request. Found by
reading only; not changed here.

**The Factory's empty page.** With no pipelines the Factory is now one line, **No pipelines yet.**, and the **Give
Gaffer an outcome** button, on every screen size: no brief, example outcomes or job sheet. The owner asked for "just the
button" after seeing the old two-panel page squeezed side by side on a phone, a regression from the page's rebuild,
which removed the container it stacked on. Checked at 390, 834 and 1280 in `web/tests/gaffer-factory.e2e.mjs`
(one button, no text box or list, under 200 px tall, a 44 px button, no sideways scroll, focus back on the button
after the dialog closes). No docs picture showed the old empty page.

**Home with nothing in flight** was simplified the same way at the owner's request: "Nothing in flight.", three
buttons in a row (Give Gaffer an outcome, Start a session, Open terminal) and Plan a session under them. The
illustrated cards, their explanations and `LaunchPreview.svelte` are gone (`StartWorkbench.svelte`; `workbench.e2e.mjs`
checks it at 320, 390, 960 and 1440 in both themes).

**Tests added in the review fix wave.** `gafferOutcome.test.mjs` (efforts sent as chosen when unknown),
`crewChoices.test.mjs` (start levels for an unknown agent, the model note), `gaffer-factory.e2e.mjs` (Add pipelines with
the machines read failing keeps the crew's efforts, backups included), `gaffer-settings.e2e.mjs` (an agent no machine
runs and an account with no machines offer the start levels, the descriptions, Other… keeping a kept model at 100
bytes, removing a middle backup, the note waiting for the list, saving untouched sends what was loaded, and no
unmocked request on the page), `outcome_effort_test.go` (`TestAStageRefusedForAnotherReasonIsNotAskedForAgain`: an
untyped error with the same words starts once).

**Not checked.** No real machine or agent: every refusal, model list and capability report is a fixture or a unit
test. Widths are emulated. No screen reader. The captures are in [docs/ui-verification.md](docs/ui-verification.md).

## HOME-REWORK-1: Home rebuilt around one list of what waits (17 November 2026)

The owner asked for Home next, by the same method as the Factory page and in its own pull request, on the branch
`ccr-93f68fda-ippwiu` (restarted from main after #825 merged). The design is the Home section of
[2026-11-17-factory-home-rework-design.md](docs/superpowers/specs/2026-11-17-factory-home-rework-design.md), the plan
[2026-11-17-home-rework.md](docs/superpowers/plans/2026-11-17-home-rework.md). Web only: no controller, API, MCP or
stored-field change.

**What was built.** Home (for an account that can start sessions) opens on **Waiting for you**: one list that mixes
Factory pipelines and sessions, oldest first, each with its action. On a wide screen an entry is a row (chip, stages,
buttons each in a fixed column, so rows of every kind line up); on a tablet or phone it is a card. Under it are
**Factory, working without you** (one link per running pipeline) and **Your machines**, closed sections that say what
is inside when closed. Which sections are open is remembered per browser (`reet.home.sections.<user>`); the first time
only Waiting for you is open, and it opens by itself when it is empty. The view tabs (**Continue working**, **Planning
board**) and their remembered choice are kept. A finished session is not in the list; it is marked **not seen yet** in
Continue working and on the board. The head is the title, one line of counts, the plan meters, **Give Gaffer an
outcome** and **New session**. A narrow screen gets a **Projects** picker, as on the Factory page.

**One rule for what waits** (`web/src/lib/home/whatWaits.ts`, with `homeWaiting.ts`). Home's list, the bell and the page
title all take their entries from it, so the three count by the same rule on every page (with a project chosen on
Home, the section shows that project's entries while the bell and the title count everything). The bell now lists
Factory items too, with a link to each pipeline. A Gaffer goal that needs review on a session with no pipeline is an
entry too.

**Where things from the old Home went.**
- "Needs you" (`NeedsYou.svelte`, deleted): the session entries of Waiting for you (`SessionWaitingItem.svelte`).
- "Factory pipelines" (`FactoryStatus.svelte` and `factoryRows`, deleted): pipelines that need you are entries of
  Waiting for you; the ones that run by themselves are under Factory, working without you.
- The summary strip, the Gaffer pill and the Factory and Projects head buttons: gone; the counts are one line in the
  head, Projects is the rail (wide) or the picker (narrow), and the Factory is a tab.
- Your machines and the plan meters, New session, Give Gaffer an outcome, the Projects panel on a wide screen, both
  views and every dialog: kept. An account that cannot start sessions sees what it saw.

**Files.**
- `web/src/lib/home/`: `Home.svelte` (recomposed), `homeWaiting.ts` (the entries, `waitingSummary`, `workingSummary`),
  `whatWaits.ts` (the one rule), `SessionWaitingItem.svelte`; `SessionBoard.svelte` and `InFlight.svelte` show not seen
  yet.
- `web/src/lib/factory/`: `PipelineChoices.svelte` (new: carries out a pipeline's choice, the dialogs included, for the
  Factory page and for Home, so a choice does the same thing in both), `WaitingItem.svelte` (gained the `row` layout),
  `ListSection.svelte`, `factoryList.ts` (an item knows its `sessionId`).
- `web/src/lib/shell/Bell.svelte`, `web/src/App.svelte`, `AppShell.svelte`, `SessionsPage.svelte`: the bell and the
  counts take the one rule.
- `web/src/lib/floatingLauncher.ts`, `GafferChatWidget.svelte`: the Gaffer button, below.

**The Gaffer button.** It floats over Home and the Factory page and keeps off item buttons: an item's button row, an
approval's command, Allow once and Deny, and the detail's bar are marked `data-floating-clearance="avoid"`, and the
launcher climbs to the lowest spot that covers none of them (the sync dock ignores this value). The strip the Factory
page reserved for it, the `dock` case and the room in the detail's bar are gone. After archiving a pipeline from a
narrow detail, one press of Back leaves the page (the pushed history entry is taken back).

**Decisions the owner made.** Rework Home next, by the same method, in its own pull request. The Gaffer button floats
on both pages. Finished sessions are not in Waiting for you. Sections open and close, with only Waiting for you open
the first time, and are remembered. The Projects panel and both Home views are kept. Merge the pull request when it is
green. No deploy.

**Rulings made while building.**
- Task 3 wrote the home-waiting assertions but did not run them or add the suite to CI; Task 4 did, because nothing
  mounts the items before Home does.
- `PipelineChoices` also exports `release`, `manage` and `holdsRefresh`; its `onsignedout` is accepted and unused.
- In the row layout an approval sits in the text column for both kinds of entry, and the actions column holds Details
  or Open only (the mock had Allow and Deny in the actions column; the approval draws its own command and needs the
  room).
- The unrequested 720 to 899 px band, where the stages sit under the text, is kept.
- The bell lists Factory entries too and every page counts by the same rule as Home's Waiting for you, because the
  design says the bell and the page title count what is in that section.
- A Gaffer goal needing review on a session with no card is an entry in Waiting for you (it asks something of the
  person); running chats Gaffer does not follow are not shown on Home.
- Accepted as built: outcomes are read in the same batch as sessions; Factory rows use the short name "Details"; on a
  card an approval's command and buttons sit above the button row; the phone's section summary wraps onto its own line;
  the head style for an account that cannot start sessions.
- Every page but Home now reads outcomes and goals with sessions each 15 seconds, for the bell: two more reads per
  poll. Accepted.
- A Gaffer goal entry has no **Mark seen**; it leaves when the goal no longer needs review.

**What the whole-branch review found, and what changed.**
- A question a pipeline's agent asks keeps its session entry beside the pipeline's item unless the item is a decision
  whose choices include answering it (`homeWaiting.ts`); before, a Ready to merge pipeline hid the question.
- A failed read of the Factory or of Gaffer's goals (anything but 403 or 404) shows "The Factory could not be read;
  showing what was last read." and the list keeps what was last read; its empty line no longer says "Nothing is
  waiting for you." then. 403 and 404 (an account without Gaffer) stay silent.
- The Factory page and the bell refresh each other: every action in `PipelineChoices` and on the Factory page signals
  `signalAttentionChanged`, and the Factory page listens to it and reads its list and sessions again. Its own signal,
  sent while `busy`, is skipped, so it does not read twice (`attention-contract` counts the reads).
- After an entry leaves (Allow once, Deny, Mark seen, a confirmed choice), the focus goes to the next entry's first
  control, else the previous one's, else the section head (Home) or the bell button (`shell/keepFocus.ts`).
- A plain waiting session counts as "waiting" in the summary; the bell's and rows' links are named with the title
  ("Open <title> in the Factory", "Open <title>", "Mark <title> as seen"); the objective in a row or the bell is one
  short line (`short`, moved to `gafferOutcome.ts`) with the whole of it as a tooltip; the bell's background reads of
  outcomes and goals no longer drive the progress bar (`req(..., quiet)`); Home passes the time zone and date format
  to the Goal and limits dialog; the Sessions list's Needs you filter leaves out sessions that only finished
  (`needingIds`); a goal that names its session only in the goal folds into that session's card, and an archived
  pipeline's goals are never a stray Gaffer entry (`whatWaits.ts`, now unit-tested); each list section's head is an
  h2.
- From the re-check of those fixes: a session that holds both an approval and a question shows once, as its
  approval item (the approval is answered first; the item then offers the question); on the Factory page a signal
  that arrives during one of the page's own actions is acted on when the action ends, and a failed answer reads the
  sessions again; when the Factory cannot be read the first time, Home says it will try again every 15 seconds.
- After main was merged in (17 November), all 88 browser suites in CI were run locally on one build: all pass.

**Not done.**
- The bell panel is short: about one and a half entries are visible before it scrolls.
- On a phone the Gaffer button can cover card text (a long sentence on a card); it keeps off controls, not text. The
  captures were taken where it covers none.
- Small things found in review and left: no test pairs an empty-string item session with an empty-string card;
  `labelOf` is called twice per entry; an unknown card kind falls through to "waiting"; the moved dialogs now mount
  before the delete, add, send-back and YOLO dialogs; `WaitingItem` and `SessionWaitingItem` repeat some row CSS;
  `moveOn` posts with the record captured at the click (the same on the Factory page).
- Home's pictures in the docs (`docs/images/workspace/home-*`) are drawn from the docs script's example data, to which
  two sessions that need you were added so the list is not empty. The script's Skills check was stale (it looked for a
  "Shelf layout" group that no longer exists) and now only checks the page's heading; the Skills and Machines pictures
  were not retaken.

**Checked.** `npm run check` with 0 errors; on one production build `home-waiting` (new), `home`, `home-followups`,
`attention-contract`, `factory-pieces`, `factory-decision`, `gaffer-factory`, `gaffer-factory-session`,
`gaffer-outcome`, `shell-fidelity`, `phone-layout` and `navigation`. Before this, a full local run of the 88 suites CI
runs found two (add a machine, skill content) that forbid requests the bell now makes; both were taught the reads.
`home-waiting` runs at 1440 (light and dark), 1250, 1180, 834, 390 and 320. The captures are in
[docs/ui-verification.md](docs/ui-verification.md), and `docs/images/ui/factory-rework-*` were retaken.

**Not checked.** No real tablet or phone: widths are emulated. No real controller: every pipeline and session in the
tests is a fixture. No screen reader.

## FACTORY-REWORK-1: the Factory page rebuilt as a list and one detail (17 November 2026)

The owner found the Factory page overwhelming with several items on it, and its buttons and settings in too many
places. Spec [2026-11-17-factory-home-rework-design.md](docs/superpowers/specs/2026-11-17-factory-home-rework-design.md),
plan [2026-11-17-factory-page-rework.md](docs/superpowers/plans/2026-11-17-factory-page-rework.md), PR
[#825](https://github.com/eyupio/reet.sh/pull/825) on `ccr-93f68fda-ippwiu`. Web only: no controller, API, MCP or
stored-field change.

**What was built.** The page is a list of pipelines in sections plus one detail for the pipeline chosen. On a wide
screen (1100 px and over) the Projects panel, the list and the detail sit side by side, the list and the detail each
scrolling alone, and the first waiting pipeline is open on arrival. On a tablet or phone the list is the page and
**Details** opens the pipeline as a screen of its own with a way back and previous / next through what is waiting.

**Where things from the old page went.**
- The tiles and the Expand all / Collapse all buttons: one line of counts under the title.
- Each outcome card with every pipeline expanded: an item in a section (Waiting for you, Working, Finished or
  stopped), its stages drawn by name.
- The Decision cards and the buttons in the card head, the decisions block and the facts row: the item's own filled
  button and **Details**, then the detail's one bar at the foot (the recommended choice first and filled, then the
  stage's other choices, Tell the agent, Send back, Move to next stage, Review goal).
- Who decides, approvals, merge for this pipeline, and Switch to YOLO: the **Settings for this outcome** row. Reports
  is shown there as a fact, since nothing changes it after an outcome starts.
- Gaffer's report, What Gaffer decided, History, and the totals with Goal and limits: closed rows in the detail that
  show their headline without being opened (Goal and limits is inside Usage and budget).
- "Who did each stage": pick a stage in the detail and its facts show.
- Stop starting sessions, Stop following, Archive, Delete and Restore: one menu on the detail head, which closes on
  Escape or a press outside it. Add pipelines is reached as before, from the proposals in a report (the **Gaffer's
  report** row); the old page had no other way to it, so the menu has none.
- An archived outcome's pipelines offer nothing but **Restore** in that menu: no bar, a card with only Details, the
  report and the settings shown read-only.
- Other sessions Gaffer follows: a fourth section of the list, closed at first.
- The Projects panel: kept on a wide screen, each project saying how many pipelines wait and work. On a tablet or
  phone a **Projects** button opens a short picker that narrows the page.

**Files** (all under `web/src/lib/factory/`).
- `factoryList.ts`: turns outcomes, goals and sessions into items, puts each in exactly one section, orders them,
  and writes the stage caption (`stageCaption`).
- `StagePipeline.svelte`: the stages in three sizes (labelled, circles with a caption, dots with a caption).
- `ListSection.svelte` and `sectionMemory.ts`: a section that opens and closes, and the per-browser memory of which
  are open (`reet.factory.sections.<user>`).
- `WaitingItem.svelte`: one item, with its chip, its title as a heading, stages, sentence (Gaffer's recommendation and
  reason when it advised) and actions. A pipeline whose session holds a tool approval offers **Allow once** and
  **Deny** (the existing `Approval`, posting what Home posts) on the narrow card and at the head of the detail's bar on
  every width; a wide screen's list items carry no buttons.
- `PipelineDetail.svelte`: the detail, its rows, its action bar and its outcome menu.
- `ProjectPicker.svelte`: the Projects button and dialog on a narrow screen. The wide panel is `ProjectRail`, which
  gained one optional `detail` prop for the counts.
- `Factory.svelte`: loaders, handlers and dialogs as before; the template is replaced. The chosen pipeline is carried
  as `?pipeline=` in the address, so a reload and a shared link reopen it, and `/factory#<anchor>`, `?project=` and
  `?outcome=1` still work.

**Decisions the owner made.** A queue and a detail (not a table or a board). The stage names are shown, in today's
circles-and-line style. Sections open and close and are remembered, with only Waiting for you open the first time.
The Projects panel is kept. The page is replaced outright, with no classic view; going back is reverting the PR. The
filled button is always the choice the code already marks primary, so without Gaffer's advice it may be "Have Gaffer
look again" and not "Move to next stage". Failed pipelines count as waiting for you.

**On a narrow screen**, opening a pipeline adds a history entry, so the browser's Back button returns to the list.
Previous / next and a wide screen change the address without adding one.

**The floating Gaffer button.** The page keeps it clear of content with the app's own mechanism
(`floatingLauncher.ts`, `data-floating-clearance`), which learned one case: a region marked `"dock"` keeps a strip for
Gaffer's launcher, which rests centred on it instead of above it (only the launcher docks; the sync dock ignores such
regions). On a narrow detail the strip is the end of the action bar's first line, and the bar sits at the very foot of
the screen, on the tab bar of a phone, with no band beneath it. Over a narrow list the strip stays at the foot of the
screen and the list scrolls under it; on a wide page it is the last line of the page, under the list and the detail.
Without the launcher (no Gaffer, or the keyboard up) the strips take no room.

**Other changes in the final pass.** Named stages keep each word whole and, when they do not fit (eight on a phone),
scroll sideways inside their own box with the stage in hand in view; the compact item's dots run edge to edge; a
failed stage is not marked current; "Recommended" is a caption above the filled button, so the bar's four choices keep
one line at 1280 px and wider; when Gaffer advises opening the session, that is the bar's filled first choice (a link);
the detail's eyebrow drops the objective's closing full stop; History is read again at every opening; an address
naming an archived pipeline (`?pipeline=`) shows archived outcomes; the Projects button names a group chosen on the
wide panel; archiving or deleting from a narrow detail returns to the list, clears the address and focuses the list's
first item; Other sessions opens by itself while a goal in it needs the person, and its summary no longer counts such
a goal as followed too.

**Not done.**
- Home was the next piece: done, see HOME-REWORK-1 above.
- F5 in part: `gaffer-floor` (the "running chats not followed" link) and `session-end-gaffer` (the chat's Factory
  link) still open Other sessions by hand. Their fixtures hold no goal that needs the person, so by the rule asked for
  (open while something in it needs you) the section rightly stays closed there; opening it in those cases would be a
  different rule, which was not asked for. `home-followups` asserts it open on arrival.
- Small things found in review and left: `factoryList.ts` casts a read-only map for `waitingDecisions`, and its
  record types copy fields of the outcome types so they could drift; the finished-section summary, minutes and days
  in "waiting", and ordering by a goal's last change have no unit test; the circles caption calls every earlier stage
  "done" whatever its state; a section has no `aria-controls`; the compact item is a button holding block elements;
  the approval mapping is copied from `home.ts`; the counts line and the section count disagree while archived
  pipelines are shown; on a wide screen Back can move focus to a Details button though no detail closed; a section
  opened by a link is not saved; the floor's filter row scrolls sideways on a phone (its own old design).
- The floating button is kept clear where the captures are taken and wherever its strip is in view; a narrow detail
  with no action at all keeps an empty strip at its foot for it.

**Checked.** `npm run check` with 0 errors; on one production build `factory-pieces`, `factory-decision`,
`gaffer-factory-session`, `gaffer-factory`, `gaffer-outcome`, `gaffer-floor`, `gaffer-followed`, `home-followups`,
`session-end-gaffer`, `workbench`, `gaffer-goal` (unchanged), `gaffer-settings`, `home`, `sessions`,
`session-workspace` and `calm-session`. Widths and themes as run: `factory-pieces` at 1440 (light and dark), 1280
(light), 1000 and 834 (light) and 390 (dark for the list, the detail and the capture; light for the new checks of
touch sizes, eight stages, long part names and the floating button); `gaffer-factory` at 1440 (light and dark), 1280
(light) and 390 (light); `gaffer-factory-session` at 1440 (light) and 390 (dark). Restore is covered by
`gaffer-factory` (the archived pass), and the YOLO and merge-on-YOLO confirmations from the Settings row by
`factory-pieces`. The captures are in [docs/ui-verification.md](docs/ui-verification.md).

**After merging main.** Main added [docs/factory.md](docs/factory.md) and its two pictures (`factory-desktop-*` in
`docs/images/workspace/`) for the old page; both were brought up to date for this one (the pictures retaken with
`scripts/capture-docs-workspace.mjs --only=factory`, the old files removed), along with the Factory section of
[docs/gaffer-ai.md](docs/gaffer-ai.md).

**Not checked.** No real machine: every pipeline in the tests is a fixture, so a real run has not shown the page. No
real tablet or phone: widths are emulated, and the owner's Android tablet has not been tried. The keyboard is not
involved here.

## DESKS-KEYBOARD-1: a desk under the on-screen keyboard (17 November 2026)

The owner reported, from an Android tablet, that a desk's terminal did not adjust when the keyboard opened.

- **Cause:** the desk screen sets `#app` to `100dvh`. Android Chrome's default (and iOS) overlays the keyboard: the
  window keeps its height and only the visual viewport shrinks, so the bottom of the pane sat under the keyboard.
  The session workspace already measures the visual viewport; the desk screen did not.
- **Fix:** `fittedHeight` in `terminal/sizing.ts` gives the height above the keyboard (null when the page already
  fits, when zoomed, or with no visual viewport). `DeskScreen` sets it as `--desk-fit` on the root, which `#app`
  uses in place of `100dvh`, and scrolls back to the top if the browser panned the page.
- **Checked:** unit test in `terminal-sizing.test.mjs`; `desks.e2e.mjs` stands in a visual viewport, shrinks it to
  560 of 1000 and reads the desk's bottom edge; `desks-phone.e2e.mjs` still passes.
- **Bars step aside (the owner's choice: tabs only).** While the screen is fitted, the pointer is coarse and focus is
  inside `.area`, `typing` is true: the site header and `.dhead` are hidden (as in theatre) and `DeskToolbar` is not
  rendered. The tab bar, the phone's pane chips, the pane header and its keys bar stay. A keyboard opened by a
  field outside the panes (the desk's name, a dialog) moves nothing.
- **Not checked:** a real tablet or phone; the keyboard is a stand-in visual viewport.

## FACTORY-REALRUN-1: what the first real split run showed (17 November 2026)

Run over MCP in project reet on PROD_Snicket (outcome `7a1c3e5f…`, OpenCode, ordinary approvals): plan (heavy),
code in two parts (standard, light), review, test, pr. Stopped by hand during the first code part.

- **Seen working:** tiers and parts accepted over MCP and shown per stage; each stage's agent and model read back
  as applied; the owner's Gaffer-decides switch recorded (`gaffer-outcome-automation`); Gaffer closed the plan stage
  after one review; the first code part started in a session of its own and the plan session was ended.
- **Fixed here:** (1) the plan's handover described the plan instead of carrying it, so the code part was asked to
  fix numbered items it was never shown: `stageHandoverRule` in `worker_tools.go` and the `hand_over` description
  now say the handover is all the next stage gets. (2) `outcomeEvidence` clipped a brief at 600 bytes with a bare
  ellipsis, which Gaffer reported as a brief that "ends mid-sentence": `evidenceBrief` shows 2,400 and says in
  words when it shortened. (3) the evidence named a part's stage only by kind: it now gives the part's name and,
  until the part is done, its brief.
- **Also fixed, from the owner watching the run:** (4) nothing could put a started outcome on YOLO.
  `SetOutcomeYolo` (`POST /api/v2/gaffer/outcomes/{id}/yolo`, browser only, confirmed, needs Allow YOLO) sets the
  outcome's choice, switches each session in hand through `Broker.SetChatYolo`, clears every `YoloMergeOK`, and
  returns the pipelines whose session could not be switched. Not offered over MCP. (5) a session the controller
  ended for a pipeline (`endFinishedSession`, `rebalanceStopped`) showed as newly finished: `leftBehind` marks it
  seen, ten minutes ahead because the machine's ended report can land after the request returns.
- **Seen, not changed:** the ended chat's banner says "You ended this … chat" for a session the Factory ended. The
  web cannot tell who ended a session; the audit row `gaffer-session-ended` can.
- **Not exercised:** balancing (backups are fixed when an outcome starts, and the backup was saved after), a
  decision by Gaffer at a stopped stage, Gaffer answering a multiple-choice question, and any stage after the first
  code part. With ordinary approvals each new session stopped at once on a permission prompt, which only the person
  answers.
- **Not checked:** whether a real model now writes a fuller handover. The rule is wording; only a run shows it.

## DESKS-1: saved sets of terminal tabs with splits and restore (16 November 2026)

The owner runs several terminals at once for one job and did it outside reet. They asked for that working set to be a
thing in reet: named, saved, reopened in any browser, each tab back in its folder with its own command history, side by
side when wanted, and restored after a machine reboot. Spec
[2026-11-16-desks-design.md](docs/superpowers/specs/2026-11-16-desks-design.md), plan
[2026-11-16-desks.md](docs/superpowers/plans/2026-11-16-desks.md), mock-ups in [docs/design/desks/](docs/design/desks/).
What a person sees is in [docs/desks.md](docs/desks.md). PR [#813](https://github.com/eyupio/reet.sh/pull/813) on
`claude/youthful-cannon-53mko0`.

- **The record.** `internal/desk` (no dependencies): `Doc` (tabs, a split tree and panes), `Validate`, `Parse` and
  `ParseStored`, tree edits and presets; `web/src/lib/desks/tree.ts` is its twin and both read
  `web/src/lib/desks/fixtures/presets.json` (`TestPresetsMatchWeb`). `internal/store/store_desks.go`: tables `desks`,
  `desk_sessions` (one pane per session across an account, keyed by `(user_id, session_id)`; `rekeyDeskSessionsTx`
  rebuilds a development table keyed by session alone) and `desk_pane_dirs` (the folder a machine reported for a pane,
  the only trust source for a folder outside home). Every write takes `expect`. Forgetting a machine touches no desk:
  its enrolment and sessions stay. Revoking the enrolment (`ControlSyncAgent(..., "revoke")`) deletes its sessions by
  cascade, so that transaction empties every pane holding one and drops its index rows and reports
  (`clearDeskSessionsForAgentTx`). Archiving a session (or an end that asked for its removal) clears its pane with a
  version bump (`clearArchivedDeskSessionTx`, through `desk.Doc.ClearSession`). A merge needs nothing, since panes hold
  the enrolment id. A desk with a newer schema is never read or written; one whose stored document cannot be read is
  skipped and logged by the list and by a revoke.
- **Routes.** `internal/api/desks.go`: list, create (`from`, `template`), get, patch, delete (`endSessions`, answers
  `left`; on a `desk-shell-v1` machine a shell that had already ended is also sent `End{ForgetDesk}` with
  `removeHistory` false), reorder, and `POST …/panes/{pane}/reopen` (`deskReopenCreate`, `deskReopenError`). Audit `desk-created`,
  `desk-deleted`, `desk-pane-reopened`. `internal/api/terminal.go` gained `deskPane` on create
  (`terminalCreateDeskPane`) and `forgetDesk` on end.
- **The machine: `desk-shell-v1`.** `internal/terminalproto/desk_shell.go` (`DeskRef`, `FolderGone`); `SessionSnapshot.Dir`
  and `SessionChanged.Dir` carry the live folder only on a connection that negotiated it. `internal/terminalhost/live_dir.go`
  reads `pane_current_path` each sweep and before an end; `desk_shell.go` writes the bash and zsh wrappers under
  `~/.reet/desks/`, the pane's history file, `ForgetDeskSession`, and `sweepDeskHistory` (at manager start, files no
  registry session names and unchanged for 90 days; no symlink followed); `launch.go`'s `ResolveDeskDir` opens a trusted
  folder outside home; `command_history.go` reads the pane's file first as source `pane`. The broker records the folder
  (`recordLiveDir` → `RecordTerminalLiveDirFor`, which notes it on the pane in the same transaction), sends
  `End.ForgetDesk` (`EndDeskPane`, a mark held in memory), refuses a desk create to an older machine, and archives a
  reservation its reserved hook refused (a stale reopen), since nothing ran in it.
- **Limits.** The browser's session limits are 24 a machine and 64 an account (`SESSIONS_PER_MACHINE`,
  `SESSIONS_PER_ACCOUNT` in `web/src/lib/terminal/api.ts`). Task 4 found nothing on the controller or machine enforces a
  count, so these stay browser-side; the unused Go constants were removed.
- **The page.** `web/src/lib/desks/`: `DesksPage` (routes, the 15 s read, rename, duplicate, template, delete),
  `DeskList`, `DeskScreen` (the desk, reopen, Reopen all, Continue, Type in all panes, keys), `DeskTabs`, `SplitArea`
  (separators, drag to move), `DeskPane`, `DeskToolbar` (one toolbar for the focused pane), `PaneStateCard` with
  `paneState.ts` (`PANE_CARDS`, the copy), `ClosePaneDialog`, `NewTabDialog`, `StartupBar`, `AddSessionDialog`,
  `AddToDeskDialog`, `DeskMenu`; pure modules `autosave.ts`, `keys.ts`, `typeInAll.ts`, `addSession.ts`, `api.ts`.
  `TerminalPane.svelte` gained `chrome={false}`, `actions`, `oninput`, `type()`, `takeOver()` and `retryNow()`;
  `CommandHistory.svelte` the This pane group; `web/src/lib/sessions/` Add to a desk… and the desk chip; `App.svelte`
  and `shell/` the Desks tab and Jump rows.
- **Not done:** zsh per-pane history is verified on Linux zsh 5.9 only, not on macOS (whose `/etc/zshrc` sets
  `HISTFILE`); no real machine reboot test (the restore test ends and restarts the test's own tmux server); the broker's
  ForgetDesk mark is lost on a controller restart, and an end from Sessions after a pane was taken off a desk never asks,
  so those files wait for the 90-day sweep; downgrading reet on a machine that started desk shells can stop its
  terminals (the older registry reader refuses `Create.Desk` in a receipt); after a reboot a folder on a mount not yet
  present reads as Folder gone, and the last folder can be one sweep stale; the desk list has no reorder control; bash
  in a pane is interactive, not a login shell; fish and other shells get folder restore only; no sharing, export or
  splits on a phone. Parked after the final review: a zero-width joiner (some emoji sequences) is refused in names
  with a "control character" message; removing a still-ending session from Sessions empties its pane, which then
  offers Start; each tab's ⋮ and rename field sit inside the ARIA tablist; a `ZDOTDIR` exported before the shell
  starts or set in `/etc/zshenv` is not honoured; the current tab is in the saved document, so a tab switch moves
  another window on the same desk; End the shell on an agent's pane does not repeat Sessions' Factory-outcome and
  worktree warnings. The full list with what a person would notice is the last section of
  [docs/desks.md](docs/desks.md).
- **The open desk and the server.** An open desk re-reads itself when the page's 15 s desk-list read shows it at a
  newer version (`DeskScreen`'s effect calls `autosave.refresh()`; no second poll). `autosave.ts` stops retrying a save
  refused for good (any 4xx but 401, 408, 429; state `refused`, `onRefused`), refuses a change over 64 KiB locally
  (`MAX_DOC_BYTES`, `TOO_LARGE`), moves a pane between tabs as one change (`movePaneToTab`), and `retire()` keeps
  retrying a left desk's queue. End the shell follows the profile's `terminalRemoveOnClose` (read in `DesksPage`);
  closing a pane whose shell already ended sends `forgetDesk` with `removeHistory` false and no dialog.
- **How it was checked** (as recorded in `.superpowers/sdd/2026-11-16-desks/progress.md`): each task reviewed, with
  fix rounds; Go tests for the document, the store, the routes (`desks_integration_test.go`, `desk_history_test.go`), the
  wrappers against real bash (`desk_shell_test.go`, including a locally built bash 3.2.57) and the real-tmux launch test
  (`terminal_launch_integration_test.go`: a `cd` reported on a sweep, a reopen outside home, and
  `TestDeskPaneRestoredAfterTmuxServerEnds`: the private tmux server ended and restarted, the shell reported lost, and a
  reopen through the route in the last folder with the pane's history); the final fix wave added real-zsh tests for an
  XDG `ZDOTDIR` and real-bash tests (5.2 and the local 3.2.57) that a `PROMPT_COMMAND` sees the last exit status; the full
  Go suite passed
  in CI on 0a22ab1 and the macOS native job on 04cf6f2; web unit tests for the tree, autosave, keys, pane states and the
  Type in all gate; Playwright with synthetic answers in `web/tests/desks.e2e.mjs` and `desks-phone.e2e.mjs`, run in
  `.github/workflows/terminal-browser.yml`. CI on f776e4f (before the final fix wave): 31 checks, 27 passed, 4 skipped
  by design. On the final head the web check and build, thirteen browser files, `go vet`, `go build`, the desk Go
  packages under `-race` and focused store and api runs passed in the container, and screenshots at 1440, 820 and
  390 px in both themes were looked at; the record, with what was not checked, is the Desks section of
  [docs/ui-verification.md](docs/ui-verification.md).

## FACTORY-LIMITS-1: the limits FACTORY-PARTS-1 and FACTORY-BALANCE-1 left (16 November 2026)

The owner asked for the remaining limits on split stages and backups to be closed, and chose "switch when it
stops" for a stage whose plan runs low while it is running.

- **Split by hand:** `withNewPart`, `partsBlocker`, and the shared `factory/StageParts.svelte`, used by the outcome
  dialog and Add pipelines. MCP `reet_start_outcome` pipelines take `parts`.
- **Backups:** `More` (`moreBackups`) on a crew tier and a stage, after `Backup`; `store.Spares` lists them in order;
  `stageBackups` validates (three at most, no repeats). `balanceStage` takes the first that qualifies. A Cline
  backup carries `provider`.
- **Switch when it stops:** `rebalanceStopped` on the outcome supervisor. A following stage with a backup whose
  session has ended, or whose agent has stopped or failed with the goal needing the person, is run again through
  `RetryPipeline` when `balanceStage` now says a backup. `Rebalanced` makes it once for a stage. A working agent is
  never ended for this.
- **Add pipelines:** a tier's saved backups can be unticked for a pipeline; a stage can be split.
- **Afterwards:** Add pipelines has a backup choice for each tier (the crew's, none, or its own agent); the crew in
  settings chooses a Cline billing source from the shared select; `gaffer-factory.e2e.mjs` sends a split stage back.
- **Not done:** parts running at the same time (the owner chose one after another). Checked with fakes and
  synthetic plan figures only.

## FACTORY-PARTS-1: a stage split into parts, and what earlier slices left (16 November 2026)

The owner asked for several agents delivering one stage, the smaller gaps closed and the FACTORY-REVIEW-2 leftovers
fixed. Their choices: parts of different work (not several opinions), Gaffer proposes and the person approves, parts
run one after another, and MCP may never give Gaffer the decisions.

- **Parts are stage entries.** A split stage is two to four `GafferOutcomeStage` entries of one `Kind`, next to
  each other, each with `Part` and `Brief`. Everything a stage has (own session, tier, backup, balancing, review,
  hand-over, decisions) therefore holds for a part with no second code path. `outcomePieces` allows a repeated kind
  only as named parts; `stagePrompt`, the goal and the session title name the part. Not split: pr, babysit, merge.
- **Gaffer's plan** has `parts: [{stage, title, brief, tier, why}]`; `outcomeSessions` replaces the stage with them.
- **Web:** `planRatings` returns `parts`; `stagePlan` expands them; the plan review edits them; `track()` gives each
  part a node with a key of its own.
- **Send back** to a split kind now returns to its first part.
- **Crew in settings:** `GafferCrewSettings.svelte`; `GafferCrew.GafferDecides` only sets where the dialog starts.
- **MCP start:** a pipeline's `tiers` and the outcome's `balance_below`. Never automation, never a split.
- **Leftovers fixed:** `worktreeInPipeline` guards the sweep, its removal step and the manual remove;
  `GafferOutcomeSession.Applied` for a pipeline with no stages; `expirePendingTools` on the outcome supervisor.
- **Not done:** model names are still checked against the machine only for agents whose preset has a model list
  (OpenCode). Each other agent needs its own listing command confirmed on a real machine. Also not done: adding a
  part by hand in the plan review, parts over MCP, parts in the Add pipelines dialog, parts running at the same time.

## SYNC-INSTALL-NAME-1: installs that failed were called a changed plan (16 November 2026)

The owner reported skills sync still failing on Elitebook with **The plan changed before it ran**, after
f70eef5 had made stale plans repair themselves. The plan never changed. Read this before touching sync again.

- **Root cause, reproduced with the real CLI.** `npx skills@1.7.4 add <source> --skill <slug>` matches `--skill`
  against the name in the skill's SKILL.md, case-insensitively, not its folder. `rknall/claude-skills` names its
  skills "SVG Logo Designer" and "Web Design Builder"; reet passed `svg-logo-designer`, the CLI answered
  "No matching skills found" and exited 1. Those were the two installs on the failing card and the only two
  skills in the plan whose name is not their slug.
- **Why it read as a changed plan.** The apply failed as `sync stopped at <slug>: exit status 1; rescan and
  preview again`. `web/src/lib/syncProblems.ts` matched the bare words "preview again" as a stale plan. The
  stage strip was the tell: a stale plan stops at Plan, this stopped at Apply.
- **Fixes.** `skillsAddCommand` (`internal/scan/sync.go`) offers the slug and the skill's own name
  (`--skill` takes several values); a name that reads as a flag, holds `*` or control characters is left out.
  `installFailure` gives the installer's own error line and no longer says "preview again". The web has an
  `install` problem kind and its stale-plan matcher lists only messages that mean a stale plan.
- **Three names for one skill.** Catalogue slug (skills.sh `skillId`), installed folder (the CLI's
  `sanitizeName` of the skill name) and repository folder (lock `skillPath`) can all differ: "Python Backend
  Architecture Review" is `python-backend-architecture-review` installed and `python-architecture-review`
  upstream. `lockSkillNames` (`internal/scan/scan.go`) now maps a lock entry to the installed folder, with the
  repository folder as an alias; `internal/setup/skills.go` does the same for snapshots.
- **Catalogue slug against installed folder.** "UI/UX Design Review" is `uiux-design-review` in the catalogue
  and `ui-ux-design-review` on disk; the install worked and the plan never settled. The rule lives once, in
  `ref.InstalledFolder`. The agent's `knownByInstalledFolder` (`internal/scan/sync.go`) reports such a folder
  under the desired slug when its source matches, before planning. The controller's `importEntry` asks
  `SkillInstalledAsFor` when the shelf has no skill by the folder's name, so the scan is recorded as the
  catalogued skill and no second skill is made. An exact shelf entry under the folder's name still wins: a
  person who already has that ghost removes it. Run for real against the CLI, in mirror mode.
- **Copied commands.** `npx skills add` commands (`web/src/lib/install.ts`, `installCommand` in
  `internal/api/api.go`) give the slug and then the name in quotes; `ref.parseInstallCommand` reads past one
  quoted value after the slug so the command still pastes back into reet, and refuses anything unquoted as
  before. The `npx skills use` prompt takes one selector, so `useSelector` gives the name when it differs.
  Checked on skills 1.7.4 and 1.9.0: the name works on both; `svg-logo-designer` fails on 1.7.4 and
  `uiux-design-review` on both.
- **One CLI version.** `ref.SkillsCLI` (`skills@1.7.4`) is what sync runs and what every copied command names:
  the install command, the use prompt, the hosted skill page and the Markdown export. `SKILLS_CLI` in
  `web/src/lib/install.ts` holds the same value and `skillPrompt.test.mjs` fails if the two differ, so a
  version bump is one change in each file. `ref.parseInstallCommand` accepts `skills@<version>` pasted in.
  The pinned command for a hosted skill was not run against a live reet; it is the version sync already uses
  for hosted sources.
- **Checks that protect what is on the machine.** They compared slugs only. In `collect`
  (`internal/scan/sync.go`) the cross-app check, the unrecognised-entry check and the copy left out of an
  exact-set plan now also look at `installedFolder(skill)`, where the CLI will really write. In the scope being
  synced, `knownByInstalledFolder` reports that folder under the slug whatever its source, so `planSync` gives
  the usual answers: another source is a source conflict, no source is a replacement, and the backup moves the
  folder that is really there.
- **Auto fix.** `resolveSyncSourceFixesFor` (`internal/store/store_sync_source_fix.go`) also looks for the
  installed copy under `ref.InstalledFolder(name)`. It now makes the plan ask for the installed skill itself
  (id, name, slug and source). It used to change the source and keep the old id, so after the install the
  controller listed the old skill as missing and the installed one as an extra for ever; that was true of
  every Auto fix, not only this case. A plan that already held the installed skill keeps one entry.
- **Auto fix on a folder that follows its source.** `MaintainSync` compared the saved plan with the source on
  every poll and wrote the source's skill back, so the fix was undone, the machine refused the skill again and
  the two took turns, a job each time. `keepFollowedCopies` puts back each installed copy the saved plan
  already follows before the comparison, and again after a save made for another reason. It is stateless: a
  copy stands only while it is still on the machine and the one source of its name, so removing it returns
  the folder to its source. It never follows a copy the saved plan did not already follow. Library
  (canonical) folders are untouched, as Auto fix never applied to them.
- **MCP.** `mcpPage` gives up a record's largest list for a count when one record is over the response limit;
  `reet_list_sync_targets` failed for five of the owner's seven targets.
- **Seen, not changed.** 24 `reet-server` environments, one per controller container name; pruning is the
  `prune_server_environments_days` setting. `TestLibraryManagedRemovalExecutor/journal_refused` fails when the
  tests run as root (it relies on `chmod 0500`), with or without this change.

## A pull request appears in the stages as it is made (16 November 2026)

The owner created a pull request in a session and the right-hand panel still showed **PR — Not yet** with the other
stages to come. The machine read was fine (the same read returns the pull request now); the panel simply reads GitHub
at most once every two minutes, so a pull request created moments before the look was invisible, and **Next** said
"Nothing needs you right now" with unwarranted confidence.

- **Cadence.** The visible session's GitHub read now runs every 20 seconds while the session is working and every two
  minutes while it is quiet (`githubCadenceMs` in `pullFacts.ts`, used by `backgroundGitHub` in `ContextPanel.svelte`).
  It is the existing store-backed read, so the stages, the pull request card and the navigation all take it up; the
  navigation's own background clock for other sessions is unchanged. This covers terminal sessions and a panel opened
  after the pull request exists, which a command-triggered read did not.
- **Cost.** One visible session makes two `gh` requests a read, so about six a minute while working — well inside
  GitHub's limits; quiet sessions and other sessions in the navigation keep the slower clock.
- **Validation.** `pullFacts.test.mjs` holds the two clocks and their boundary (a stored answer is due at, not before,
  the cadence). `npm run check` passes.
- **CI.** Two Gaffer tests that raced after merging main are fixed: the audit test now waits for the background
  hand-over to end the previous stage's session, and the question test gives the rig's provider a reply because its
  supervisor may ask the model while the test only checks `pipelineStop` (b75045d4). Both pass repeatedly under
  `go test -race ./internal/gaffer`.
- **PR.** [#812](https://github.com/eyupio/reet.sh/pull/812) on `reet/reet-da349d2dcba62110d9c98f373d5bfbc3`.

## FACTORY-BALANCE-1: stages balanced across plans (16 November 2026)

The owner asked for stages to use one agent or model over another when it has more of its plan left.

- **A backup per tier.** `GafferCrewTier.Backup` and `GafferOutcomeStage.Backup` (`GafferStageAgent`). The outcome
  dialog's crew has a Backup row per tier; a stage sent with no agent takes its tier's backup from the saved crew.
- **The rule** (`outcome_balance.go`): when a stage is about to launch, `balanceStage` reads the machine's kept plan
  figures (`MachineAccountsFor`). First choice under 20% left in its tightest window, figures no older than six
  hours, backup signed in and with more left (or no figures at all): the stage runs on the backup. A window with a
  label counts only for a model whose name contains it.
- **The threshold is the person's.** `GafferCrew.BalanceBelow` and `GafferOutcome.BalanceBelow`, 5 to 90, zero for
  the default of 20. An outcome that names none takes the saved crew's. The dialog's crew has the field.
- **Said, not asked.** The stage's `Tool`, `Model`, `Effort` become the backup's, `First` keeps the first choice and
  `Balanced` the sentence. Audit `gaffer-stage-balanced`. A later start of the same stage looks again and can go back.
- **Not done:** backups in the Add pipelines dialog; a Cline source for a backup in the dialog; balancing between
  more than two choices; a re-check while a stage is running. Checked with synthetic figures only.
- **PR #808** (empty Home, Factory and Sessions) was test-merged against this branch with no conflict.

## FACTORY-DECIDE-1: decisions as choices, and Gaffer decides (16 November 2026)

The owner asked for every decision to be clickable options with the recommendation first, on the Factory card's
head and in Factory sessions, and for an option where Gaffer takes the decisions and reports what it took and why.
Read "Decisions as choices" in the [factory floor handoff](docs/handoffs/gaffer-factory-floor.md).

- **Recommendation.** A stage review's `ask_user` takes `recommend` (`again`, `tell`, `move`), `why` and
  `instruction`. It is kept as `GafferGoal.Advice` and cleared by the next review or control. The web puts that
  choice first; with no advice the usual first choice is marked.
- **Where.** `factory/Decision.svelte` is the one block. The outcome card's head lists every waiting decision
  (`waitingDecisions`), the merge included. `factory/SessionDecision.svelte` shows it under the session header for
  the session doing the stage in hand.
- **Gaffer decides.** `GafferOutcome.Automation`, set at approval or by `POST …/outcomes/{id}/automation`.
  `autoDecisions` (in the outcome supervisor) puts each stop to Gaffer's model once (`decide` tool), carries the
  choice out through the person's own paths and appends to `GafferOutcome.Decisions`. Three decisions a stage, one
  budget renewal, never a merge, never a goal held for the agent's own question.
- **An agent's multiple-choice question** is answered too (`outcome_answer.go`). `Broker.PendingQuestionChat` gives
  the question and whether it waits alone; `pipelineStop` offers `answer` for a held goal only then, and only when
  every field Gaffer must answer is a choice. The model gets `answer_question`, whose schema is the question's own
  options, and `leave_to_user`. The answer goes through `AnswerQuestionChat`, so the machine validates it. Counts
  toward the stage's three decisions.
- **Not done:** the switch over MCP, an account default, and a real run (checked with fakes and a stand-in model).

## FACTORY-CREW-1: a session per stage, and Gaffer advises the crew (16 November 2026)

The owner's direction: Gaffer orchestrates and advises; each Factory stage is a separate session on the agent and
model that suits how hard the stage is; several agents delivering one stage comes next. Two of three slices are done.
Read "A session per stage, and tiers" in the [factory floor handoff](docs/handoffs/gaffer-factory-floor.md).

- **A session per stage.** `advancePipeline` no longer carries a stage on in the previous stage's session when agent,
  model and effort match. Every stage launches its own session from the handover and the previous one is ended.
- **Tiers replace roles.** The crew is `light`, `standard`, `heavy` (`store.GafferCrew`, table `gaffer_crew`, one row
  per account, `GET/PUT /api/v2/gaffer/crew`). A stage carries `Tier` and `TierWhy`. A stage sent with no agent is
  resolved by the controller from its tier's saved crew member, else the outcome's agent; that is how an outcome
  started over MCP uses the crew.
- **Gaffer advises.** The plan tool takes `ratings: [{stage, tier, why}]`. Gaffer rates, names no agent or model. The
  dialog applies and marks a rating that differs from the kind's default, with the reason and a one-click way back.
- **Not done:** several agents per stage (slice 3, decisions still to be asked); a crew editor in Gaffer settings
  (the crew is saved from the outcome dialog only); tiers are not sent on the MCP start tool's input. Checked with
  fakes and a stand-in machine, not a real agent or provider, so how well a real model rates stages is unseen.

## FACTORY-REVIEW-2: the merge gate, and a stop that stands (16 November 2026)

The owner asked for a review of Gaffer, the Factory integration and what the handoffs still owed, then chose to have
every finding fixed in one PR. Read "Second review" in the
[factory floor handoff](docs/handoffs/gaffer-factory-floor.md): it lists what was found, the contract changes and
what was found and left.

- **The merge gate** held only on a hand-over. A pipeline at Ready to merge that was stopped and tried again, or one
  whose first stage is the merge, started its merge with no approval, from the page, over MCP and by Gaffer's tools.
  `runOutcomeSession` now holds the gate on every path.
- **A stop stands.** The controller writes its progress only while the pipeline is still its to move
  (`controllerHolds`), so a launch that answers after Stop following no longer puts the pipeline back in Gaffer's care.
  Archived outcomes are not set going again, and a pipeline that can be tried again keeps its worktree.
- **MCP and Gaffer's tools.** `reet_approve_pipeline_merge` needs `seen_updated_at` and approves only the pipeline as
  it was read; `applied` is false when nothing was queued or changed; `reet_start_outcome` refuses a machine that has
  not said what it runs; and once Gaffer has read handover text in a turn, a change asks first even on Allow.

- **Was the choice applied?** After a stage's session is up, the controller reads back from the machine which model and
  effort it is on, keeps that with the stage and shows it on the card (**Applied**: Confirmed, Not as chosen, Not
  checked), including for a stage that carries on in the session before it. A difference is logged and stops nothing.

- **A message that goes by itself.** A message the agent was too busy to take stays queued, and a failed message no
  longer holds back what was queued after it (`nextQueued` in `terminalbroker/chat_queue.go`). That is what left
  Gaffer's next instruction waiting for **Stop and send**.
- **Focus mode on a phone** shows less around a busy chat: one header row, one status line, the queue as one line that
  opens, and the composer's settings behind **Exit focus**. See [terminal workspace](docs/terminal-workspace.md).

- **Plan limits where the work is.** What is left of each agent's plan (AGENT-ACCOUNTS-1) shows on Home, the Sessions
  toolbar, a chat's status strip and a Factory card, as one control: `PlanMeters.svelte` (chips, a tip on hover or
  keyboard focus, a panel on click; both layers are HTML popovers, so nothing clips them), `FleetPlanMeters.svelte`
  for a page's machines, `AgentMark.svelte`, and `planEntries`/`planLeft` in `agentAccounts.ts`. Figures come from
  each machine's last read (`planLimits.ts`); no new endpoint and nothing asked of a machine.

The owner runs the Factory against real agents; this slice was checked with the test fakes only, so the Applied line on
a real stage is the first thing to look at. The roadmap's own next item is unchanged: finish and record the
[device and login checks](docs/device-checks.md), then turn Claude Code and Codex chats on by default.

## Factory outcome follows a stopped session (16 November 2026)

The owner reported that stopping a session in a Factory outcome left the outcome
waiting at **Needs you**; removing the session from history was then refused by
its worktree hold until the outcome was stopped and archived or deleted by hand.
The owner chose **stop following and delete**, with dialog toggles, a system
default and an account preference that overrides it.

- **Controller.** `POST /api/v2/terminal/sessions/{id}/end` takes
  `factoryOutcome` (`""`, `archive`, `delete`). `gaffer.EndSessionOutcome` stops
  following only the outcome whose pipeline works in that session now, closes
  its goal, then archives or deletes it. Earlier-stage sessions and finished
  pipelines are left alone. Failure is recorded as
  `factory-outcome-not-released` and does not undo the accepted end request.
- **Browser.** The end dialog in the session lists, Home and the terminal
  workspace gains **Stop following the outcome** and **Delete it for good,
  instead of archiving it**. Account → Profile has **When stopping a session in
  a Factory pipeline** (`delete` is the system default; `archive` and `leave`
  are the alternatives). The dialog's choice is per session and overrides the
  preference.
- **Stored preference.** `user_profiles.factory_session_end_mode`, default
  `delete`, normalised to delete for unknown values; returned as
  `factorySessionEndMode`.
- **Validation.** New Go tests in `internal/gaffer/outcome_orphans_test.go`,
  `internal/api/terminal_close_test.go` and `internal/store/profile_defaults_test.go`;
  browser helper tests in `web/src/lib/gafferOutcome.test.mjs`;
  `account-profile.e2e.mjs` updated for the extra preference. `go build ./...`,
  `go vet ./...`, the store, gaffer and api packages, `npm test`,
  `npm run build` and `svelte-check` pass. The browser e2e suite was not run in
  the container (no Chromium).
- **PR.** [#807](https://github.com/eyupio/reet.sh/pull/807) on
  `reet/reet-30e82926db6ff96af08cae36143495e8`. Open; waiting for the
  benchmarks and image checks before merging to `main`.

## MOBILE-3: what MOBILE-2 left undone (15 November 2026)

The owner asked for MOBILE-2's "Not done" list: three things.

- **Overlays against the insets.** Audited with `Emulation.setSafeAreaInsetsOverride` at 390 × 844 (47 px top, 34 px
  bottom) and 844 × 390 (47 px sides, 21 px bottom). An upright phone's sheets were already right: the shared shell's
  border takes the insets. Sideways, where a dialog is not a sheet, three things were under the notch or the home
  indicator and are fixed: a wide dialog (`max-width` now leaves the side insets), a short-screen dialog's foot
  (`dialog-shell.css` stops it above the bottom inset while the keyboard is down), and things pinned to an edge (the
  terminal's drawers, the Gaffer panel, the bell's panel, the project rail's drawer, a maximised pane, the save and
  sync docks). A bottom sheet tall enough to reach the top now stops below the top inset.
- **Server-side transcription** for browsers with no speech recognition: asked, and the owner chose to leave it out.
  A phone keyboard's own microphone already works in Compose and in the chat composer on every browser; the gap is
  Firefox on a computer.
- **A compose box for the chat pane**: its composer is one already. What it lacked was dictation, so the Dictate
  button moved out of Compose into `terminal/Dictate.svelte` and the chat composer has it too.

Seen on the way and left alone: inside `phone-dialogs.e2e.mjs`'s harness (which makes the page 3,000 px taller) an
open sheet carried `data-shell-kb`, the shell's keyboard-open mark, with no keyboard up; the cause was not traced. That
mark drops the sheet's bottom inset, so a first audit run through that harness reported footers under the home
indicator. The same sheet opened without the harness had its footer above the inset. The audit was redone
without it, and is now `overlays and insets` in `terminal-focus.e2e.mjs`.

**Not verified:** a real phone, notch or microphone, as before. Popover menus placed at the pointer (session,
project, command and clipboard menus) are clamped to the window, not to the safe area, and were not audited one by
one: their anchors sit inside the padded page, so they open inside it, but one opened at the very foot of a sideways
phone could overlap the home-indicator strip.

## MOBILE-2: what MOBILE-1 left undone (15 November 2026)

The owner asked for MOBILE-1's "Not done" list. Five things, each run at phone and tablet sizes before and after.

- **Compose** (`terminal/TerminalCompose.svelte`, rules in `terminal/compose.ts`): a message field for the terminal,
  sent as one paste with Enter (Send) or without (Insert). It sits in the terminal toolbar after Earlier output. It
  was first the key row's leading key; that pushed Ctrl half out of view at 390 px, so it moved. The text skips paste
  review: the person wrote it there to send it. Several lines in a terminal without bracketed paste are warned about
  in the sheet, since each line would run as it arrives.
- **Dictate**, inside Compose, uses the browser's `SpeechRecognition` where there is one. The browser sends the audio
  to its own vendor's speech service, so the sheet says so on the button and while it listens. Nothing is sent by
  speaking. There is no server-side transcription and no key to configure.
- **Attention grouping** (`attentionGroup` in `home.ts`, `GROUP_CHOICES`): a fourth Group by on Home's Continue
  working and the Sessions list. Home already had Needs you with inline answers, and the Sessions tab's running list
  from MOBILE-1 puts what needs you first, so this is the triage order for the full list, not a new screen.
- **A chat's finished turn** is now a moment a device is told about (`pushMoment` in `internal/api/push.go`): a
  structured session still running whose `chat_status` is finished or failed and whose `completed_at` is after the
  person last looked. It shares the device's second choice, now worded **A session has finished**.
- **The third "Waiting for your approval"** on a phone is gone: `ChatStatus`'s strip hides while it would say
  approval or question (`.conversation-status.attention`, 650 px and under). The pane's status line keeps the live
  region, so it is announced once.
- **Edge to edge**: `viewport-fit=cover` in `index.html`; side insets on `body`, top on `.site-header`, bottom
  subtracted from the workspace height through a probe element (`.safe-probe` in `TerminalWorkspace.svelte`) while
  the keyboard is down; the floating Gaffer launcher keeps clear of a side notch. Checked with Chromium's
  `Emulation.setSafeAreaInsetsOverride`, which makes `env(safe-area-inset-*)` real in tests.

Also: `ChatStatus` no longer pads its right end for a floating launcher on touch screens, where MOBILE-1 hid it.

**Checked:** see [docs/ui-verification.md](docs/ui-verification.md).

**Not verified:** a real phone, as before. Safe areas were emulated; a real notch, Dynamic Island or Android
gesture bar was not seen. Dictation was driven by a stand-in recogniser; no real microphone or speech service was
used. Safari and Firefox were not run.

**Not done:** dialogs and sheets other than the session workspace were not audited one by one against the insets
(the shared dialog shell already pads for them); server-side transcription for browsers without speech recognition
(Firefox); a compose box for the chat pane, which already is one. See MOBILE-3, above.

## MOBILE-1: sessions on phones and tablets, and notifications on a device (15 November 2026)

The owner asked for a usability assessment on phones and tablets, with most attention on terminal and agent sessions,
then chose the scope: every finding, plus push notifications.

The assessment ran the real UI against a synthetic fleet (a terminal socket that repaints for its size, a chat stream)
at 390 × 844, 360 × 640, 844 × 390, 820 × 1180 and 1180 × 820 with touch emulated. A real controller and machine were
started in the container, but the machine's private link needs a relay the container cannot reach, so no real
terminal was attached.

**What was wrong**, each seen on screen:

- **Focus** did nothing on a phone terminal: the button lit, but `focusOn` required a chat (`SessionScreen.svelte`).
- A phone terminal had 364 of 844 px (28 rows): six bars above it, a status footer below. Sideways, 9 rows.
- Ask Gaffer's circle sat on the agent's prompt line, lifted there by the footer's clearance, above a 68 px strip
  reserved for it and left empty. On a tablet it covered the terminal's last rows and a chat's Send.
- A tablet with its keyboard up kept every bar: 13 rows at 820 × 1180.
- The docked panel at 240 px wrapped a project's name ("Storefron/t") and floated its count; beside it the session's
  name was cut to "Refacto…" with room to spare.
- The phone drawer was 240 px with no scrim; names were cut while 140 px of the page showed beside it.
- `/sessions` with nothing chosen said "Nothing is open right now" with four sessions running: the list filtered by
  the chosen machine, and none was chosen.
- Typing in a chat on a phone left about 70 px of conversation.
- Start an agent showed "Choose a project…" in red before anything was touched.
- Home's board had 24 and 32 px controls on touch screens.
- Nothing told a phone that a session needed its person.

**What changed (browser):**

- `SessionScreen.svelte`: focus mode for any session on a phone, on by default; in focus a connected terminal's pane
  header and footer are hidden; `terminalShowing`; **Ask Gaffer** in the header; the name's own row on a narrow
  stage that is not a phone; the tablet keyboard rules; the header's second row hidden while typing in focus.
- `app.css`: no floating launcher and no reserve on a session screen at 767 px or less, or on any touch screen;
  `html[data-session-typing]` hides the top bar.
- `TerminalWorkspace.svelte`: `keyboard` also on a touch tablet (`TOUCH_TABLET` in `sizing.ts`), and
  `data-session-typing`; `resumeSessions` and the reordered "nothing open" screen; the drawer's scrim.
- `SessionSidebar.svelte`: one-line group headers (`.fold-title`), the drawer's width.
- `TerminalPane.svelte`: the 8-row floor's shift reads the host's height unrounded. `clientHeight` rounded 45.94 px
  to 46 and left the last row 1/16 px past the foot, which `terminal-workspace.e2e.mjs` caught at 390 × 420 once the
  reserve was gone.
- `ChatPane.svelte`, `StartAgentDialog.svelte`, `home/SessionBoard.svelte`: the typing layout, `.reason.hint`, 44 px
  targets.

**What changed (push):** `internal/webpush` (VAPID, an empty wake-up, the push service address list),
`internal/store/store_push.go` (`push_subscriptions`, `push_told`), `internal/api/push.go` (five routes and a worker
every five seconds), `web/public/sw.js`, `web/src/lib/push.ts`, `pushSupport.ts` and `DevicePush.svelte` on the
Notifications page. Design and limits: [docs/push-notifications.md](docs/push-notifications.md).

**Decisions made here, for the owner to overturn if wrong:**

- A phone terminal opens in focus, as a chat does. Existing suites about the full chrome start with focus off
  (`sessionFocus:false` in `helpers/workspace.mjs`, or the same storage key seeded).
- Wake-ups carry no payload, so nothing about a session reaches a push service and no subscription keys are stored.
  The cost is a generic line when the device is signed out or the moment has passed.
- The panel still docks by default from 768 px (the documented rule); only its layout at 240 px changed.
- Push is its own card, not a kind in `internal/notify`: it is per device, and that catalogue is per person.

**Checked:** see [docs/ui-verification.md](docs/ui-verification.md) for the commands and results.

**Not verified:** a real phone or tablet; Safari and Firefox; a real push through a push service; momentum scrolling,
pinch zoom and text selection in the terminal; the notch held sideways (`index.html` has no `viewport-fit=cover`, so
the safe-area insets the styles use are probably always zero).

**Not done:** a compose box that sends dictated or autocorrected text to a terminal as one paste; voice input; an
inbox that sorts every session by what needs you; a chat's "Waiting for your approval" still shows in two places
when the keyboard is down; a notification when a chat's turn completes while the session keeps running (only waiting
and ended sessions are told). All five were done in MOBILE-2, above.

## LINK-WRITE-1: a request that gives up no longer drops a machine's connection (14 November 2026)

The Gaffer offline acceptance test (`scripts/test-gaffer-offline.sh`, CI job `offline (native)` and once `offline
(docker)`) failed about one run in six for two days, on branches that changed nothing near it. Every failure was a wait
for a worker prompt or receipt timing out after a goal was resumed or set going. The owner asked for the cause.

**The cause is in the product.** Both ends of a machine's control connection wrote each message under the context of the
request it belonged to (`internal/api/private_control.go` `write`, `internal/agenthealth/agent.go` `send`).
`coder/websocket` closes the whole connection when a write's context ends, and does so even when the context had ended
before the write began: each of its two write locks is a `select` between "context done" and "lock free", Go picks at
random when both are ready, and once past them `context.AfterFunc` on the done context closes the connection at once.
So a request that had given up closed the connection about one time in four, and every terminal and chat on the machine
detached until it reconnected (see `docs/remote-machine-operations.md`).

**Who gave up.** An open chat's pane asks for its live controls every 1.5 s (`ChatPane.svelte`) and abandons the request
when the person leaves the chat. Under load the controller had often not yet sent it. With call paths recorded on a
loaded run, all 11 writes for a request that had given up were that one (`terminalChatControls` → `ChatControl` →
`callWithin` → `write`); none had waited for the write lock (about 1 µs), so they had given up before reaching it. On the
machine's side, a launch-stage note is sent with a one-second budget (`terminalagent/agent.go`); that end was not seen
failing, but it has the same shape.

**Why the test failed.** It opens and leaves chats as a person does. After a dropped connection, the goal loop's next
check of the worker failed ("terminal machine is reconnecting"), so it noted "The worker is unavailable" and waited a
minute (`internal/gaffer/goals.go`): exactly the test's 60 s. A drop at other moments refused Gaffer's instruction as
`blocked` or kept a turn open past a 10 s wait.

- **What changed:** `agenthealth.WriteControl` is how both ends write on the control connection. A caller that has given
  up sends nothing; a write that has begun runs under its own limit (`context.WithoutCancel`), five seconds as before. A
  write that cannot finish in that time still ends the connection, as it should for a half-open socket.
- **The fixture, which had two bugs of its own** (`internal/api/gaffer_integration_test.go`): its `progress` returned once
  the worker's events were sent, not recorded, so a resume straight after could treat a paused worker's progress as new
  and spend a review (seen locally once); it now waits until the session's progress has risen by both events. And after
  dialling again it sent its second event under the old link's generation, so the controller closed the new link at once
  (CI's #793 failure logged "dialling again" just before timing out); each message now carries the generation of the link
  it goes on. It also logs the moment the controller ends its link, which the `main` failure's log could not show, and
  the browser test's prompt waits say what the goal was doing when they time out.
- **Checked:** reproduced with logging in a scratch copy, the race-enabled binary CI uses, twelve flows at a time on four
  cores. Before: 5 failures in 16 runs, every one starting with the controller's read loop ending on `use of closed
  network connection` straight after a write for a request that had given up; no passing run had such a close. After:
  36 runs, no connection closed mid-run; 17 writes for requests that had given up were refused and the connection stayed;
  the 3 failures were the browser's 5 s waits for page elements while I ran Go tests on top, none at a goal-loop step.
  New tests: `TestWriteControlForACallerThatGaveUpLeavesTheLinkUp`, `TestWriteControlOutlivesTheCallersDeadline`
  (a 32 MiB write the peer reads after the caller's 50 ms deadline), and `TestACallThatGaveUpLeavesTheMachineLinkUp`
  (40 calls through the broker over a real link). Putting back the old write fails all three, the last with the
  controller resetting the link; keeping the early return but not detaching the write fails the second.
  `TestGafferWorkerLinkRedialsAfterItIsLost` now fails under the old fixture's stale generation. `go vet ./...`, and
  `go test -race` on `agenthealth`, `terminalagent` and `terminalbroker`, pass; on `api`, the control, agent, fence and
  Gaffer tests pass after merging main, and the full suite is left to CI.
- **Not verified:** macOS; a real machine and controller under real load; the connector-side drop was reasoned from the
  code, not seen.
- **Left as it is:** `ChatControl` holds a per-session lock (`controlMu`) with no deadline while it waits for the
  machine, so a busy session's callers queue behind each other; a stalled controller can still end a connection by the
  five-second write limit, which is the existing design.

## TERMINAL-KEYS-1: the on-screen terminal keys, rebuilt (14 November 2026)

The owner asked for the on-screen keys to be checked and made more like a phone SSH client's extra-key row, and chose
the scope: **the rebuilt bar**, without a settings panel to customise it and without swipe gestures.

What was wrong, each seen by running it at phone and tablet sizes:

- The keys were hidden on any touch screen wider than 650 px (a tablet), unless it was a sideways phone.
- At 390 px the row's 13 keys needed 616 px: everything after the Up arrow was off screen until scrolled.
- The pad's navigation keys were clipped to "Ho…", "E…", "Pg…", "Pg…", "I…", "D…".
- A modifier applied to one key only, one modifier at a time; nothing repeated when held.
- A modifier that was on drew its label in the fill's colour unless the pointer was over it (`.tools button[aria-pressed]`
  outranked `.key-modifier[aria-pressed]`).
- Every key called `terminal.focus()`, which opens the device's keyboard on a phone.

What changed (browser only; nothing in Go, the protocol or the machine):

- `web/src/lib/terminal/virtualKeys.ts`: the keys (`barKeys`, `arrowKeys`, `keyGroups`), the modifier state
  (`noModifiers`, `tapModifier`: off → next → locked → off; `afterKey`; `activeModifiers`), `withModifiers` for several
  modifiers at once (xterm's parameter is one plus Shift 1, Alt 2, Ctrl 4), and `repeats`.
- `TerminalPane.svelte`: one row that stays (a part that scrolls, then the arrows and the second row's button, which do
  not), a second row above it with four groups, `keycapAction` for tap and hold-to-repeat, the modifiers joining typed
  input through `term.onData`, no forced focus on touch, and the row shown under `(pointer:coarse)` at any width with
  44 px keys above phone size. The floating pad and its styles are gone.
- The second row takes its height from the screen, so opening it resizes the terminal once. That is the owner's choice
  over the pad that floated above the screen.
- Docs: [On-screen keys](docs/terminal-workspace.md#on-screen-keys).

Checked: `terminal-keys.test.mjs` (six cases) and `terminal-phone.e2e.mjs`, which now covers the fixed arrows, the
modifier cycle, two modifiers on one key, a locked modifier joining typed input, the label's colour, hold-to-repeat to
the millisecond with the page's clock held, sliding off a held key, a key that acts once, the second row's groups and
bytes, its height, clipped caps, what is remembered, and touch tablets at 820 × 1180 and 1180 × 820. The new test fails
on the old code, and five behaviours were each broken in turn and seen to fail (nothing repeats, the tap after a hold
sends one more, sliding off does not stop, the old label colour, hidden on tablets). The existing suites `sessions`,
`phone-layout`, `phone-zoom`, `terminal-workspace`, `terminal-lifecycle`, `terminal-clipboard`, `terminal-upload`,
`calm-session` and `sessions-phone` pass.

Not verified: a real phone or tablet. Touch is emulated in Chromium and the holds are mouse presses, so how a held key
behaves under a real finger (a long press, a scroll that starts on a key, the device's own keyboard staying down) was
not seen. Firefox and Safari were not run.

Not done: choosing or reordering the keys; swipe gestures for the arrows; hiding the row on a tablet that has a hardware
keyboard attached (it cannot be told apart); a locked modifier is let go when the connection changes state.

Also seen and left alone: [the phone docs](docs/terminal-workspace.md#phones) say a sideways 844 × 390 phone shows 13
rows; the test prints 9 on main before this change too.

## AGENT-ACCOUNTS-1: which agents are signed in, and plan limits (14 November 2026)

A machine now says which of its agents are signed in and, when the account turns plan limits on, how much of each plan's
allowance is left. The owner chose the gate: **an account setting, off by default, on machines with terminal access on;
no new switch on the machine.** Full contract: [agent sign-ins and plan limits](docs/agent-accounts.md).

- **Machine reader** (`internal/agentaccounts`): Claude Code (`.credentials.json`, `.claude.json`) and Codex
  (`auth.json`, identity-token claims) sign-ins from files; with limits, one GET each to the provider with the agent's
  own token. The rules, each with a test that fails when the rule is removed: the token is never in an answer; an
  expired token is never sent and never renewed; no redirect is followed; figures answer for five minutes; a provider is
  asked at most once in 30 s; a failure keeps the last figures; another account never inherits figures; limits are not
  read unless asked for.
- **Protocol** (`internal/terminalproto/accounts.go`): feature `agent-accounts-v1`, `Accounts{limits, refresh}` and
  `AccountsResult`, `Capabilities.AgentAccounts` (valid only with terminal control, terminal access and the feature).
- **Connector** (`internal/agenthealth`, `internal/terminalagent`): negotiates the feature, reports the capability
  only while terminal access is on, re-reads that switch per request, one read at a time per process, reader kept for
  the process so its memory outlives a reconnect.
- **Controller** (`internal/terminalbroker/accounts.go`, `internal/store/store_agent_accounts.go`,
  `internal/api/accounts.go`): asks only a machine that reported the capability; keeps the last answer per account and
  machine in `machine_agent_accounts` (replaced whole; goes with the enrollment); `GET /api/v2/machines/{id}/accounts`
  and `POST …/accounts/refresh`; `user_profiles.plan_limits` (default 0) through `PATCH /api/v2/profile {planLimits}`.
  With the setting off the machine is asked for sign-ins only and kept figures are not shown.
- **Browser** (`web/src/lib/agentAccounts.ts`, `MachineSettings.svelte`, `StartAgentDialog.svelte`,
  `AgentsSettings.svelte`): the machine dialog's Sign-ins row and per-agent lines; the Start dialog's hint when no
  sign-in was found; the Plan limits switch in Settings > Agents. The machine dialog asks the controller only for a
  machine that reports the capability; for the others it says why itself.
- **Compatibility.** An older controller never advertises the feature, so a new machine sends nothing new; an older
  machine never reports the capability, so a new controller never asks it and the page says to update reet there. A
  machine with terminal access off is told apart from an old one. One schema addition (a table and a profile column),
  both additive.
- **Checked:** unit and package tests for every layer above (`go test -race` on `internal/agentaccounts`,
  `terminalproto`, `agenthealth`, `terminalagent`, `terminalbroker`; `go test` on `internal/store`, `internal/backup`,
  `internal/api`, `cmd/reet`); `agentAccounts.test.mjs`; browser suites `agent-accounts` (new) and `account` (three new
  cases), and the existing `control`, `launch`, `launch-base`, `launch-options`, `add-machine`, `machines-search` and
  `phone-dialogs` (machine settings and start agent). The phone dialogs suite caught two things on the way: the dialog
  asking about machines that cannot answer, and a long host name widening it; both are fixed.
- **Not verified:** nothing was run against a real Claude or ChatGPT account, so the providers' present answers and
  the sign-in files' present shapes were not observed; they follow the agents' own clients. The endpoints are not
  published interfaces. macOS was not run (the keychain case is a unit test with the OS name set).
- **Not done:** GitHub Copilot, Google Antigravity, OpenCode and Cline; the macOS keychain and system keyrings;
  several accounts per agent; signing in from the browser; limits beside a session or in the sessions list; an MCP
  tool for the figures.
## COPILOT-CLI-1: GitHub Copilot in a terminal session (14 November 2026)

Branch `claude/copilot-agent`, four local commits, not pushed and no pull request when this was written.

- **Added.** Tool id `copilot`, display name **GitHub Copilot**: a launch preset in `internal/terminalhost/catalogue.go`
  (binary `copilot`), its row in the controller's option table (`terminalproto/agents.go`), its display name and greeting
  word for automatic titles, and its row in the browser catalogue (`web/src/lib/catalogue.ts`). It is a terminal session
  only. No ACP descriptor, chat adapter, model listing, sign-in or plan display, install or update was added, and the
  Gaffer and MCP agent lists were left alone because they name chat agents.
- **What the help proved** (GitHub Copilot CLI 1.0.112, npm package `@github/copilot`, bin `copilot`; captured in
  `internal/terminalhost/testdata/help/copilot.txt`, sources and integrity in `sources.json`). Only `--version`, `--help`,
  `help sessions` and `help permissions` ran, from a scratch directory with an empty home, a cleared environment and no
  credentials.
  - Opening prompt: `-i, --interactive <prompt>`, "Start interactive mode and automatically execute this prompt". The
    preset passes `--interactive <prompt>`. `-p, --prompt <text>` is "non-interactive mode (exits after completion)" and
    is never used; a test refuses it.
  - Model: `--model <model>`.
  - Effort: `--reasoning-effort <level>`, possible values none, minimal, low, medium, high, xhigh, max; the three a
    launch carries are mapped. The help adds "supported levels depend on the model".
  - YOLO: `--allow-all`, "Enable all permissions (equivalent to --allow-all-tools --allow-all-paths --allow-all-urls)".
    `--yolo` has the same description; `--allow-all` is the one sent.
  - Continue: **not mapped.** `--continue` is "Resume the most recent session", which `help sessions` qualifies as
    "preferring one from the current branch, repository or directory", so it can open another folder's conversation; and
    "A resumed session keeps its conversation, model, agent mode and allow-all state", so it could bring back approvals a
    machine with YOLO switched off refuses. The browser row has no `resume`, so **Continue session…** is not offered.
  - Session id: **not mapped.** `-r, --resume [<value>]` takes one, but the help's examples write only
    `--resume=<session-id>`, and the only path that supplies a session id is a Gaffer chat, which Copilot does not have.
- **Compatibility.** New token `copilot-cli-v1` keeps the id from a controller or connector that predates it, whose
  strict decoder would refuse it: the controller advertises it (`internal/api/private_control.go`); the connector
  claims it only with `agents-v2`, lists the tool only when it was negotiated and tells the dispatcher
  (`internal/agenthealth/agent.go`); a create for the id is refused as `launch_option_unsupported` on a connection
  without it (`internal/terminalagent/agent.go`); `Capabilities.ValidateFor` refuses the id without it; and a create is
  sent only to a machine that lists the tool, as for every agent. A model-list request also names a tool and is sent to
  any machine with `models-v1`, so `terminalbroker.models` answers `not_installed` itself for `copilot` unless the
  machine lists the tool: a connector that predates the id fails to decode such a request and ends its connection
  (`agenthealth` read loop). **The same request for `antigravity` is still sent unchecked**; that was read in the code,
  not reproduced, and is not changed here.
- **Tests seen failing with the piece removed** (each piece taken out in turn, the named test run, the piece put back):
  the capability check, the connector's tool filter and its `agents-v2` condition, the dispatcher's create refusal, the
  advertised token, the model-list gate, the prompt flag, the YOLO argument, the effort flag, resume arguments added, a
  flag the help lacks, the option-table row, the display name and the greeting word; in the browser, the catalogue row,
  the greeting word and a `resume` flag added.
- **Checks run and their results:** in `web/`, `npm ci --no-audit --no-fund && npm run check && npm run build` passed
  (unit tests, svelte-check 458 files with 0 errors and 0 warnings, build), before any Go build. `go vet ./...` clean.
  `go test -race -count=1 -timeout=60m` on `internal/terminalproto`, `terminalhost`, `agenthealth`, `terminalagent` and
  `terminalbroker`: all ok. `go test -count=1 -timeout=30m ./internal/api` without the race detector: ok in 147 s, on
  the second run; the first failed on one new assertion that expected the wrong refusal for a Copilot chat request (it is
  400, an invalid request), which was corrected. `go test -count=1 ./internal/scan`: only
  `TestLibraryManagedRemovalExecutor/journal_refused` fails, as it does whenever the suite runs as root. In Chromium
  against the dev server with mocked APIs: `start-agent.e2e.mjs` (changed here) and `launch.e2e.mjs` and
  `account.e2e.mjs` (unchanged; they draw the catalogue) passed. `git diff --check` clean. Not run: `go test -race ./...`
  as a whole, the `mcp` tests (nothing there was touched) and the documentation site build (mkdocs is not installed here).
- **Not verified.** Nothing was run signed in: no session was started, no model was reached, and the measured arguments
  were never given to the real interactive CLI. So unverified: that `--interactive` leaves the session open in practice;
  how the folder-trust question and an opening prompt interact; what the CLI does with an effort level the model lacks,
  or with a model name it does not know; any Copilot CLI older or newer than 1.0.112 (the preset has no version floor);
  macOS and Windows; an installation that is not on the connector's `PATH`. The help names an environment variable,
  `COPILOT_ALLOW_ALL`, that turns tool approvals off; reet does not set it, and whether a launch inherits it from the
  machine was not checked. A real older controller or connector was not connected; the compatibility rules are covered
  by unit and API tests only.
- **Next:** a device check of one signed-in Copilot terminal launch (plain, with a prompt, with YOLO) before calling the
  arguments verified; then decide resume from what that shows. First files: `internal/terminalhost/catalogue.go`,
  `internal/terminalhost/testdata/help/README.md`, `docs/terminal-launch.md`.
## TERMINAL-ASK-2: the screen tests wait for tmux, not the clock (14 November 2026)

TERMINAL-ASK-1's tests against a real tmux waited out the real gaps between two reads of a screen: about 21 s on
Linux in a package that took about 50 s, and more on macOS, where `native services` runs the package with a 3-minute
limit and was already close to it. The first run on `main` after the merge, and the next pull request's, both ended
`panic: test timed out after 3m0s` on macOS, each in a different worktree test.

- **What changed:** `Manager.promptEvery` and `Manager.promptAgain` replace `promptReadEvery` and `promptReadAgain`
  when set; nothing sets them outside tests, so the machine's behaviour is the same. The tests set a millisecond,
  draw a screen, wait until `tmux capture-pane` shows it, and reconcile once. The four tests now take about 1.5 s
  together; the one second left is tmux's whole-second activity, which the settled rule is about.
- **One more rule has a test:** a screen just read is not read again inside the gap, whatever is drawn meanwhile
  (`TestAScreenIsNotReadAgainInsideTheGap`). The gaps had no test before.
- **Checked:** the tests 15 times under `-race`; six rules each broken and a test seen to fail (recorded on every
  read, every session read, settled screens read again, settled at once, no gap, the test's gaps ignored).
  `go test -race ./internal/terminalhost`: 55 s here, 67 s before.
- **That was not all of it.** With the tests fast the package still took 154 s on macOS in a run that passed
  (`ok ... internal/terminalhost 154.269s`) and reached the 180 s limit again in the next run, on a branch that had
  this fix. The package had grown to the edge of its limit before TERMINAL-ASK-1; that change was what first pushed a
  run over. `native-services.yml` now gives the terminal host step and the broker and PTY step 5 minutes each (the
  second took 130 s of its 3 minutes in the same run). The job's 15 minutes still bounds a hang. Why macOS takes
  three times as long as Linux here was not looked into; the worktree and clone tests, which run `git` many times,
  are where both timeouts were caught.
- **Not verified:** macOS was not run here; the pull request's own `native service (macos-latest)` job is the check.
- **Left as it is:** the same run on `main` also failed `native service (ubuntu-latest)` once, in the root recovery
  test, with `could not run tmux -V`: the 3-second probe in `FindTmux` ran out straight after a 76-second compile.
  Nothing in TERMINAL-ASK-1 touches that probe, and it has passed since.

## TERMINAL-ASK-1: an agent's question on its terminal screen counts as a bell (14 November 2026)

A terminal session showed "needs you" only when the agent rang the bell, and Claude Code, Codex and OpenCode do not
always ring it when they stop to ask. `running` already meant the shell, not the agent (see "Terms that must not be
conflated"), so such a session looked like one still working.

- **What changed** (`internal/terminalhost/agent_prompt.go`, hooked into `reconcileLocked` in `manager.go`): for a
  running session that an agent with rules was started in, and that has no bell set, the host reads the visible screen
  (`tmux capture-pane -p`, 64 KiB at most) and matches the last 16 non-empty lines against that agent's patterns. A
  question that the look before did not find sets `BellAt`, exactly as a bell does. Nothing else changes: no protocol,
  controller, store or browser change, so the existing attention rule, Home, the bell and the sessions panel show it,
  and an older controller works with a new machine.
- **Read sparingly.** Only when the window's activity differs from the last read (a screen cannot change without
  activity), at most every 2 s while it keeps changing and never twice within 500 ms; a screen read after the second
  of its last activity is over is settled and not read again. A plain shell and an agent without rules are never read.
- **Privacy.** The text stays inside `askedOnScreen`: it is not stored, logged or sent. The registry and the snapshot
  carry a time. `docs/remote-machine-operations.md` says so beside the existing statement that terminal content is not
  recorded.
- **The patterns are narrow on purpose.** Each needs the marks of a question the agent drew (a numbered option with
  the selection mark on it, the keys that answer it, OpenCode's heading with its allow and reject choices), not just
  words an answer could contain, and only in the last lines. A screen no pattern knows is "not waiting".
- **Checked:** `agent_prompt_test.go` (each agent's question, working and idle screens, the words alone, a question
  that scrolled up, an agent without rules, a shell) and `agent_prompt_host_test.go` against a real tmux 3.4 (a
  question recorded once, a redrawn question read again and not recorded again, an answered one and a second one, a
  shell and an agent without rules never read, no screen text in the snapshot or the registry, a settled screen not
  read again). Three rules were each removed and a test seen to fail (recorded on every read, every session read,
  settled screens read again). `go test -race ./internal/terminalhost`: pass.
- **Not verified:** the patterns were not run against a signed-in Claude Code, Codex or OpenCode; the screens in the
  tests were written for them. [Device check 5](docs/device-checks.md) says how to confirm each and what to record
  when one misses. macOS was not run.
- **Not done:** Cline, Google Antigravity and any other agent; telling "working" from "idle" (the window's activity
  still decides that); a word for it distinct from a bell's, so opening the session clears it as it clears a bell; a
  switch to turn the read off.
## CHAT-USAGE-1: a chat shows what its agent reports using (14 November 2026)

The agent's tokens and context figures were already kept per session (`terminal_session_usage`, see "Agent tokens and
context left" in the [factory floor handoff](docs/handoffs/gaffer-factory-floor.md)) and shown only on the Factory page.
An ordinary chat now shows them too.

- **Controller.** `GET /api/v2/terminal/sessions/{id}/chat/usage` (`internal/api/terminal_chat_usage.go`) answers the
  stored `TerminalSessionUsage` for a structured session the account owns. It asks no machine, so an ended chat answers.
  No schema, protocol or machine change.
- **Browser.** `web/src/lib/chatUsage.ts` turns the record into the line's label and detail (nothing for an unreported
  chat); `ChatStatus.svelte` renders it at the end of the status line; `ChatPane.svelte` reads it when the chat opens,
  when a new status that is not a working one arrives, and every 30 s while working. The trigger is the status object,
  not `working`: a turn that starts and ends inside one batch of events never shows as working.
- **The floating Ask Gaffer circle** sits over the right-hand end of the status line on a desktop, so the line now keeps
  68 px clear there while the circle is shown. By the same geometry the tool counts at that end sat under the circle
  before; that was worked out from the layout, not looked at on its own.
- **Checked:** `internal/api/terminal_chat_usage_test.go` (seen failing with no route); `chatUsage.test.mjs`;
  `web/tests/chat-usage.e2e.mjs` (no figure for a quiet agent, the figure after a turn with no reload, a failed read
  keeping the last figures, the low mark, tokens with no window, 1360/390/320 px in both themes, nothing floating over
  the figure — that last check was seen failing with the clearance removed). The twelve existing chat and session
  browser suites pass. The usage field names were compared with the published protocol schema and the Claude adapter's
  source ([structured sessions](docs/structured-sessions.md#what-the-agent-has-used)).
- **Not done:** the sessions list and the hover card show no usage; no price; no plan or rate-limit allowance (that is
  separate work); nothing was run against a signed-in agent, so which agents actually send the figures is still unknown.
- **Later the same day:** `chat-usage.e2e.mjs` failed once in CI and about one run in twelve here. It measured the
  figure in the instant after the window shrank from 1360 to 390 px, before the page had changed to its phone
  arrangement (the status line was 58 px wide for that moment and right 300 ms later). The three measurements now
  wait for the arrangement the page settles in. 60 runs passed, and the check was again seen failing with the
  clearance removed. The moment itself, a wide arrangement squeezed into a narrow window until the page reacts, was
  not looked into.

## CI-FLAKE-1: two checks that failed on unchanged code (13 November 2026)

Both failed once on a pull request that touched neither, and passed on a re-run. Each had a cause.

- **`TestForceStopIsBoundToTheCancelledTurn`, "OpenCode reported no version".** The test failed in 0.00s, so the
  version probe had not run at all: `cmd.Start` returned "text file busy". A binary still open for writing cannot be
  run, and the fake agent is a script each test writes while other parallel tests fork, each fork holding the open
  file for an instant. `VersionError` words every refusal with a required version the same way, which hid the reason.
  `probeOnce` (`internal/acpagent/config.go`) now tries the start again while the kernel says busy, six tries over a
  little more than a second; the same moment exists for real after an install or update writes the agent.
  `probe_busy_test.go` holds the binary open and reproduces the exact CI message with the retry removed. Not changed:
  the session's own start (`session.go`, after the probe), and the error's wording.
- **The benchmark gate, `ListForCompact` +19.63% (p=0.035, n=6+7).** The code measured was identical on both sides
  (`internal/store` was untouched; allocations were equal to the byte). Two things made a false alarm likely. The
  runner's processor count changed mid-run (samples named `-2` and `-3`), so benchstat compared six against seven of
  eight; `bench-compare.sh` now fixes it with `-test.cpu` (`BENCH_CPU`, 2). And nine benchmarks at p < 0.05 on a
  shared runner will cross the gate by chance now and then; the job now measures again only what the first
  comparison named (`bench-gate.py --failed-to`) and fails on the second comparison. The threshold and significance
  are unchanged. Tests in `scripts/bench_gate_test.go`; the gate was also run against real benchstat with a
  synthetic 29% regression (caught, named) and the measure-again step was run by hand.

## MOVE-FOLLOW-2: nothing to run on a machine (14 November 2026)

The Move page still asked for `reet agent set-server` on each machine. Three gaps made that necessary, all closed:

- **No address at the fence.** `PATCH /api/v2/admin/fence` (owner, fenced only, 409 `not_fenced` otherwise) sets or
  changes `movedTo` keeping the date (`store.SetFenceAddress`); `SetFence` still never changes a raised fence.
- **The hint was only in memory.** The agent saves it as `movingTo` in the enrollment (`moveFollower.remember`) and
  tries it only when the old controller does not answer at all: a transport error or a 502/503/504
  (`controllerUnreachable`), never a local failure. Any other answer from the controller means the fence was lifted and
  clears it (`controllerAnswered`, `settled`), as does a fence that no longer names an address (`forget`).
  `set-server` and a completed follow clear it, as does a fence naming an address the agent refuses.
- The departure report is kept as `leftFrom`/`leftAt` and retried once a minute (`departureReport`) until the old
  controller answers at all, or for a week. The held report is said once per reason and address and again after a
  good cycle. A new fence address clears `address-refused` rows only (`SetFenceAddress`). With no address yet, the
  page says a turned-away machine waits for the address, not the new controller.
- **Nobody could see who had moved.** `fence_machines` (emptied on raise, lift and restore) records `told_at` when
  the fence gate refuses a request carrying a valid `X-Reet-Agent`/key, and `left_at`/`left_to`/`held` from
  `POST /api/v2/sync/moved` (open path, fence-exempt, the machine's own id and secret, 401 otherwise, 409 unfenced).
  The agent posts `{"to"}` after following and `{"held": "follow-off"|"address-refused"}` once when it will not.
  `GET /api/v2/admin/fence/machines` (admin) lists them; the page derives moved / held / waiting (told within two
  minutes) / quiet / unseen (`controllerMove.ts`). Features `fence-address` and `fence-machines`.
- `tellOldFn` is replaced in `internal/scan` tests (`agent_follow_pending_test.go`) so no test reaches the network.
  The `remember` step was seen failing a test when removed. See `docs/ui-verification.md` for the live run.

## MOVE-FOLLOW-1 and MOVE-OWNER-1: machines follow a move, and a restore can make an owner (13 November 2026)

- **Machines follow.** `internal/scan/agent_follow.go`: a running agent that is refused with 423 `controller_moved`
  and a `movedTo` runs the two `set-server` checks against that address (proof of the machine's record first, then
  the credentials), saves only the address into the enrollment read fresh from disk, and re-execs. The address is a
  hint: a place that cannot prove the record is sent no credential and is asked once a minute, which is the normal
  state while the new controller is being set up. `reet agent follow-moves on|off|status` is the local switch (on by
  default, `followOff` in the enrollment). `--once` carries on against the new address in the same run. Not followed
  on its own: a fence with no address, an address with a path, plain HTTP anywhere but this machine.
- **Shown for real:** two controllers built from this branch on one machine and a foreground agent. The old one was
  fenced with the new address, the agent said no credential was sent while the new one was down, the fenced copy was
  restored with `--public-url`, the new one started, and within a minute the agent pointed at it and was polling it,
  in the same process. While it waits the agent says the refusal once, not every cycle (`movedEcho`), and a cycle ends at the first
  moved refusal, so the fenced controller is asked once per cycle and the lines about work it could not do are gone.
- **An owner at restore.** `reet backup restore --make-owner ADDR --owner-password-file F` makes an account already in
  the copy its owner (`store.AdaptOptions.MakeOwner`, `makeOwner` in `store_restore.go`); the copy's owners become
  admins, the account's sessions and reset links end, its two-step sign-in is kept. Refused before anything changes
  for an address the copy lacks or holds disabled, a password under 8 characters, or `--owner-email` beside it. The
  command line only: the staged import does not take it.
- **Tests seen failing with the rule removed:** the proof skipped, the minute's pacing, the stale enrollment written
  back, the switch ignored, plain HTTP followed, any 423 followed; the disabled check, the sessions kept, the old
  owner not demoted, the short password and the check beside it. Unhooking the follower from the run loop does not
  compile. `TestLibraryManagedRemovalExecutor/journal_refused` fails when the suite runs as root (it relies on a
  read-only folder) and is not touched by this work.

## MOVE-UI-1: moving a controller from the Backups page (13 November 2026)

The move was complete on the command line and the API (fence, a copy with its own password, `import` with `publicUrl`
and `allowLiveSource`, `reet agent set-server`) and absent from the web UI. It is now in the **Move** section of **Admin → Backups**
(the section main called Import, renamed when the two were merged), owner only.

- **Old controller, Move this controller away** (`web/src/lib/MoveAway.svelte`): fence with an optional address and a
  confirm dialog; the final copy step is shut until the fence is up, takes a password for that one copy, and shows
  **taken fenced** from the copy's own header before offering the download; the checklist for the new controller and
  the copyable `reet agent set-server` command; **Lift the fence** throughout. A copy the fence did not hold for is
  said to be so and is not offered as final.
- **New controller, Move a controller in** (the import form in `Backups.svelte`, renamed): the address field
  (`publicUrl`, prefilled with `window.location.origin`), the live-source tick shown only after the controller's
  `copy_source_live` refusal, the refusal beside the form with the values kept, and a report that adds the address
  changes, what was held back, `notInCopy` and the `set-server` command.
- **Fenced state**: a banner on Backups, and `web/src/lib/shell/FenceStrip.svelte`, one line under the header on every
  other page. An admin's shell reads the state; anyone else gets the line from the first write refused with 423
  (`onRefusal` in `api.ts`). The line is left off the session workspace, which fills the viewport to the pixel, and
  off Backups, which says it in full.
- **Honest offers**: `moveFeatures` in `web/src/lib/controllerMove.ts` reads `GET /api/version`; with no `features`
  nothing of the move is offered and the fence is not even read.
- **API, all small**: `GET /api/v2/admin/fence` is readable by an admin (raise and lift stay owner only);
  `backup.Item` carries `fenced` and `protection`; the import answers carry `notInCopy` and the two refusals an owner
  can answer carry a `code`.

Checked as [docs/ui-verification.md](docs/ui-verification.md) records: unit and browser contracts, and two real
controllers driven through fence, final copy, download, lift and import in Chromium at 320 and 1280 px in both
themes. Not done: the session workspace shows no fenced line, and nothing was tried in Firefox, Safari or on a physical phone.
The refusal text for a live copy is the controller's own and still names the command-line flag.

## Current Gaffer pause point — 6 November 2026

The user has paused work. Read the [continuation checkpoint](docs/handoffs/gaffer-resume.md)
first: owner-merged #625 passed all 20 applicable checks and owner-merged #626
passed all 22. Both had four intentional skips and no unresolved inline review
threads. Automated code review was blocked by account quota. No next slice has
been selected; older “next task” statements are historical. The checkpoint records
completed UX/setup, offline acceptance, benchmark correction, limitations and
standing permissions. Resume from current main and agree the next bounded slice.

## Gaffer factory floor (9 November 2026)

Work resumed from the pause point above with the user's direction to make Gaffer a simpler, more capable controller of
an agent factory. They chose the **factory floor** as the next slice and **pick several, one review** as its start flow.
**Chat options → Factory floor** is the former Followed sessions job sheet plus every running structured chat not yet
followed; ticked sessions share one review and each is then started through the existing reviewed per-session setup.
Browser-only: no backend, schema or protocol change, and Gaffer's authority is unchanged. The
[factory floor handoff](docs/handoffs/gaffer-factory-floor.md) records the decisions, contract, validation and the
deferred options (commanding sessions from general chat, bulk pause/stop).

On 10 November the same branch gained a usability pass (the floor is one click from general chat and Home, the session
picker is folded into it) and **outcomes**: the user chose **plan, approve, then run**. `POST
/api/v2/gaffer/providers/{id}/plan` has Gaffer's provider propose sessions from the typed objective; the person edits and
approves; the browser then starts each session through the ordinary launch and sets up its goal, which names the outcome.
No schema change: outcomes group by the goal's first line. Read the handoff's outcome contract and limits before extending
it.

Later on 10 November, after #713 and #715 merged, a third PR made outcomes **controller-owned**: the approved plan is a
`gaffer_outcomes` record and the controller starts and follows the sessions without a browser, survives a restart and
stops on a restore. It also fixes six defects an independent review found in the browser-run start, adds one model and a
plugin-trust tick for an outcome, and bulk pause/stop on the floor. Read "Controller-owned outcomes" in the handoff; it
supersedes the browser-orchestration limits recorded for #715.

The fourth PR (#718) first aligned the widget, floor and outcome dialog, then made the factory first-class: a **Factory**
page and navigation tab that draws each outcome's work as pipelines; **staged pipelines** (plan, design, code, review,
test, PR, babysit, merge) where each stage has its own agent and model and the controller starts the next on Gaffer's
hand-over; a **merge gate** (per-pipeline auto-merge with an account default, otherwise Ready to merge and an explicit
approval); and two account settings, looking-after pre-approved by default and the auto-merge default. Read "The Factory
page and staged pipelines" in the handoff for the three decisions, the contract and what is still owed.

A fifth PR followed #719: one-click look-after inside a session's chat, later-stage sessions listed under their project,
**Try again** for a stopped pipeline, **usage** (Gaffer's reviews and tokens per stage, pipeline and outcome, with reviews
and time left; tokens only, no prices), a **report across an outcome** on request with **proposed pipelines** that wait for
approval, and one vocabulary (look after / goal / following / needs you). See "Owed items, smarter outcomes, usage and
wording" in the handoff.

On 13 November the owner asked for a review of the whole Factory. It found and fixed, each with a failing test first, a
pipeline that could not be recovered once its session was closed, a stopped outcome that restarted pipelines nobody asked
for, and MCP writes that skipped the fence and reached YOLO agents; the End dialogs now name the pipeline a session is
working for, the card of an ended session offers choices that work, and the controller ends sessions a pipeline has
finished with. Read "Review of the Factory, and sessions that end under a pipeline" in the handoff, including its list of
what was found and not fixed.

## OVERVIEW-4: the Overview leads with what matters (8 November 2026)

Follow-up to OVERVIEW-3 on the session workspace's right-hand panel.

- **Order.** The Overview now runs status header, **Progress**, **Pull request**, **You asked**, agent activity,
  **Workspace changes**, **Where it's working**, then the Skills fold. The tracker and pull request were the most useful
  facts and sat below the prompt and counters, so a 360 px panel made people scroll to reach them. The prompt moved below
  the tracker and PR.
- **Removed.** The "Since your last visit" block, its `newEvents` / `terminalAdvanced` derivations, the `sessionStorage`
  visit marker and the per-tab "returning" logic in `ContextPanel` are gone. The tool tally and its counts line went too:
  Gaffer's card chips and **In progress** already carry the same facts. This also closes OVERVIEW-3's deferred note that
  the return markers could move inside.
- **Status line.** The separate 64 px notice box became one fixed-height line under the headline. It shows a request
  waiting for you, a chat error, or the state and last activity; the Review button appears only when something waits. The
  panel no longer reserves a blank calm notice or a blank "nothing waiting" sentence.
- **No Gaffer column.** The 88 px right margin that reserved a column beside the terminal workspace is removed (`app.css`,
  `WorkspaceSkeleton.svelte`). The floating companion sits over the workspace's corner instead of widening the page and
  lifts clear of composer docks with `keepLauncherClear` (`GafferChatWidget`). The short-landscape phone still hides it.
- **Sticky view.** The selected tab is a browser preference (`panelTab.ts`, `reet.workspace.context-tab`), so switching to
  another session keeps **Changes**, **Files** or **GitHub** selected. A restored **Files** view reads its listing on mount;
  **GitHub** already refetches through the Overview cadence.
- **Validation.** `session-workspace.e2e.mjs` updated: the removed return-summary assertions are now absence checks and the
  away-text assertion stays. `npm run check` and the session workspace suite should run before merge; see
  `docs/ui-verification.md`.

## Gaffer AI session-companion MVP: agreed direction (5 November 2026)

The user selected the [Gaffer session-companion MVP](docs/work-orchestration-handoff.md#active-direction-gaffer-session-companion-mvp-5-november-2026)
as the initial AI-chat direction. Outside a session Gaffer is a general
Zoomies-style chat; each reet session has its own Gaffer conversation. Gaffer is
the user's quirky critical buddy: it reads permitted context and messages the
existing working model, which performs the coding and PR actions. Explicit goals
can continue for several selected sessions while the user is offline and recover
after controller restart. Initial follow-up supports verified structured chats.

The companion handoff records all choices, source evidence, provider/MCP
limitations and proposed GM0-GM6 slices alongside the broader KZ roadmap. The
plan [#600](https://github.com/eyupio/reet.sh/pull/600) and HTTP provider foundation
[#601](https://github.com/eyupio/reet.sh/pull/601) are merged. Tests use local fake
providers, with real credentials supplied later through Gaffer settings.

GM1B and GM2A shipped in [merged PR #603](https://github.com/eyupio/reet.sh/pull/603),
`feat/gaffer-provider-settings`. The owner merged conversation backend #605 into
that branch, then #603 merged into main as `b3b8e781`. It contains account-owned providers, encrypted keys,
live model selection and Account → Gaffer AI, plus separate durable general/session
conversations, versioned model choices, turn receipts, SSE, cancellation and
restart recovery without replay. That initial chat slice sent only typed companion messages; GM3A below adds explicit retained-context sharing.
The [conversation/access contract](docs/handoffs/gaffer-conversations.md) records
worker eligibility, grants and queue receipt/recovery boundaries for later slices.

GM2B shipped in [merged PR #606](https://github.com/eyupio/reet.sh/pull/606),
merged by the owner as `6f4abb45`. Floating general/session chat includes independent
model choices, durable receipt recovery, scoped drafts, cancellation and history
cleanup. Its final CI was green, including the launcher/composer/keyboard fixes.

GM3A shipped in [merged PR #608](https://github.com/eyupio/reet.sh/pull/608) on
`feat/gaffer-context-reads`, aligned with main `9ab70f83` after #607.
Gaffer's **Context** dialog previews explicit, off-by-default retained status,
event metadata and optional user/agent text under provider policy. Shared domain
reads also back hosted MCP, browser tools and scoped stdio: grants are separate
per conversation/client/scope, with CAS, provider/model binding, revocation and
archive handling. Snapshots are bounded and timestamped, omit raw tool payloads,
reasoning/approvals/files and make no live machine or GitHub requests. Durable turn
receipts record the source/category/digest; revoked context-derived history cannot
silently re-enter later provider prompts. See the updated
[conversation/access contract](docs/handoffs/gaffer-conversations.md).

The screenshot-led UX follow-up in #608 makes the narrative the main area: compact
header with Conversation history, separate model picker opened from below the
composer, Context beside it, and options/reply metadata on demand. History switches
general/current conversations or links to other available session workspaces while
preserving scoped drafts. The shared text parser renders paragraphs/lists/code as
escaped content. The browser matrix includes the supplied 800×1280 touch-tablet
shape, 560 px boundary, phones and desktop; normal layouts reserve over 60% for
messages. See the latest UI evidence and the PR for final verification.

GM3B shipped in [merged PR #612](https://github.com/eyupio/reet.sh/pull/612) as
`77892506`, with all 20 final-head checks passing. It used
`feat/gaffer-worker-messages`. It started from main `6c5638cf` after #608/#609 and
is aligned with main `6c40c7fb` after #610/#611. The concrete
[worker-message plan](docs/handoffs/gaffer-worker-messages.md) records separate
permissions, one explicitly enabled request, atomic queue receipts and delivery
checks. It preserves the working model and user approval boundary.
GM4’s plan was merged by the owner in [PR #617](https://github.com/eyupio/reet.sh/pull/617).
The implementation is tracked separately in [PR #620](https://github.com/eyupio/reet.sh/pull/620),
`feat/gaffer-offline-runtime`, aligned with main `1b10a978`. Its final head
`1ce63151` passed all 20 applicable CI checks (four intentionally skipped) and has
no unresolved review threads; the owner has merged it. The
[offline-goal contract](docs/handoffs/gaffer-offline-goals.md) records explicit
per-session goals, the user's default of 12 reviews over 24 hours, progress-only
reviews, atomic budget/turn admission, fresh goal/grant delivery checks, stop/
pause/resume, concurrent sessions and restart reconciliation. Instance restores
pause goals. The working model retains implementation and PR authority.
Gaffer has no coding/PR authority;
the working model performs those actions. No model-requested tools, worker writes,
background polling or real provider credentials are enabled in GM3A. Final checks
and PR state are recorded in the companion handoff and UI verification record.

GM5 shipped through owner-merged [PR #623](https://github.com/eyupio/reet.sh/pull/623),
a separate followed-session overview on
`feat/gaffer-followed-sessions`, based on runtime #620. It lists only the browser
account's selected goals, prioritises decisions, preserves source/review times and
separates model reports from retained delivery receipts and user completion.
**Chat options → Followed sessions** opens it from general or session chat without
crowding the narrative. Existing goal controls retain fresh permission checks;
listing never calls a provider, wakes a worker or starts a goal. See the
[overview handoff](docs/handoffs/gaffer-followed-sessions.md) for tests and PR state.

The owner merged the UX slice in [PR #625](https://github.com/eyupio/reet.sh/pull/625),
`feat/gaffer-companion-ux`. The user's selected **one reviewed session setup,
with advanced controls optional** replaces the multi-dialog activation sequence.
It atomically saves provider session/text maxima, only this conversation's grants
and a bounded goal. General chat and MCP retain their separate authority.
The single floating mascot has saved left/right placement, three reference-sized
presets, temporary drag/resize and modal full screen/phone layouts. Page docks and
session viewports reserve its corner. See the
[widget/setup handoff](docs/handoffs/gaffer-widget-setup.md).
GM6 shipped through owner-merged [PR #626](https://github.com/eyupio/reet.sh/pull/626).
Fake-backed real-controller/private-WebSocket browser acceptance passes in native
with race detection and the production container runtime/non-root user, including
restart, limits, approvals, revocation and uncertain delivery. Its
[acceptance contract](docs/handoffs/gaffer-offline-acceptance.md) records exact
coverage and validation limits. Final CI passed all 22 applicable checks; automated
review was quota-blocked. Neither
PR merging nor real-provider setup is authorised by this handoff.

The 5 November #603 review follow-up fixes first-address-only provider dialing:
try all validated literal addresses, with dual-stack fallback and bounded,
cancelled/joined racers. A localhost HTTP regression fails on the old transport
and passes with the fix. The recurring offline-browse CI failure now uses fake
time for the request while retaining its 100 ms limit, six offline/access cases
and no-send/no-audit assertions; an injected 101 ms wait still fails. No browse
production source changes. Focused repeated race checks, web check/build, vet,
16 MCP checks, strict docs and the provider browser matrix pass. The new full race
run passes API (409 s), Gaffer (40 s), store (197 s), broker (66 s) and every package
except the same five baseline terminal-host failures. Final-head CI remains the
merge gate; see the companion handoff's dated follow-up record for exact limits.

Keep that handoff current in every implementation batch. Changes remain confined
to reet.sh; reference repositories are read-only.

## OVERVIEW-3: the Overview in the browser (5 November 2026)

Part C of the session overview, on branch `claude/session-overview-ui`. The right-hand panel's Overview now
answers where a session has got to and what is next. It reads the prompt route from OVERVIEW-1 and the extra pull
request fields from OVERVIEW-2; contents and rules are in `docs/terminal-workspace.md` ("Session workspace details").

- **Shipped.** A run clock line (`runClock.ts`); a "You asked" prompt card with a full-prompt dialog (`PromptCard`,
  `PromptDialog`, `promptLoader.ts`, `promptSource.ts`); a five-step tracker with one Next line built from facts only
  (`overviewStages.ts`, `StageTracker`); a pull request card (`PullCard`, `pullFacts.ts`, `pullVerdict.ts` shared with the
  GitHub tab); a Where card with copy buttons and a repository link (`WhereCard`); a status dot on the GitHub tab; and one
  automatic GitHub read per cadence that respects Live updates (`githubReadDue`, `insightStore`). The Overview itself is its
  own component (`OverviewTab`), moved out of `ContextPanel` with no change to what it showed. The final review's fix wave
  added a `Last known · checked …` marker on the tracker's Next line and the pull request card (`githubStale`), a Copy pull
  request link button, a prompt dialog that stays open across the 15 s session refresh, and the phone sheet closing after
  a fix request fills the composer.
- **The fix shortcut never sends.** "Ask the agent to fix..." fills the chat composer through `composerPrefill.ts`
  and `ChatPane`; a terminal, or a composer that is nearly full, disabled or off screen, gets a copy to the clipboard
  instead (`fixMessages.ts`, `fixDelivery.ts`). A repeated request does not stack. Check and branch names are cleaned
  before they are quoted because GitHub's users write them.
- **Honesty rules.** No reported checks is never green; threads not read are never zero; Merge is never claimed from green
  checks alone. An older machine or controller degrades to "not read on this machine" and "started before reet kept opening
  prompts" instead of a guess.
- **Validation.** See "Session workspace: the Overview" in `docs/ui-verification.md` for the commands, the files that
  ran, the one that could not (`terminal.e2e.mjs`, through the Go fixture, fails on the emoji line as it does without this
  change) and the tests seen failing with their rule removed. This task added a stability block to
  `session-workspace.e2e.mjs` covering every Overview slot at 300, 340 and 520 px and the 340 px phone sheet, both themes, and
  restored `toHaveText` on the Branch assertion in `sessions.e2e.mjs`.
- **Known deferrals (from the task reports).**
  - Check detail reads "plus 2 mores" with more than two failing or running checks; the test records it. Fix the wording and
    the test together.
  - `OverviewTab`'s scoped `section` rules apply to every `section` in it; use `div` for nested cards.
  - `newEvents` and `terminalAdvanced` are still derived in `ContextPanel` and passed down; they could move inside.
  - The prompt dialog's switching guards cannot be told apart by the browser test, because the session screen remounts per
    session; they are held by `promptLoader.test.mjs`.
  - The Live updates gate on the mount effect cannot be exercised end to end.
  - The Overview is about 250 px longer than before the two new cards (their heights are fixed, so nothing moves).
  - The fake clock in the cadence test only moves `Date`, so the Live-off test waits for the panel's real 5 s tick.
  - `answer` on `composerPrefill` is an extra field beyond the original brief, so a chat can say whether it took the text.
  - Wording not covered rule by rule: the fix messages (checks and singular threads wording were chosen in Task 7).
- **Compact at 14px (Task 16).** At the default 360 px panel the five steps are one row, the Next buttons share a row
  above the fix note's line, and the pull request card is a summary with a native **Details** fold for the secondary facts
  and actions (closed by default, kept open across polls; its summary row is two lines below about 388 px); the 300 px
  panel sets the steps three over two. Measurements
  and thresholds are in `docs/ui-verification.md` and the components' comments.
- **Not done.** Firefox, Safari and physical devices; screenshots; `terminal.e2e.mjs` passing in this container.

## OVERVIEW-2: the machine reads the extra pull request fields (5 November 2026)

Part B of the session overview. `internal/terminalhost/inspect.go` now fills the optional fields added to the
inspect wire in `3580786`: base and head branch, created, merged and closed times, reviews, review requests,
auto-merge, review threads, the last commit date and `DetailKnown`, plus `Remote` (`owner/name`) on the status
and GitHub views, so the page can link the repository before any pull request exists.

- The first `gh pr view --json` call gains only scalar fields (`createdAt,mergedAt,closedAt,baseRefName,headRefName`).
  Review bodies and commit lists could overrun its 128 KiB output cap and break the whole read, so reviews, review
  requests, auto-merge, threads and the last commit date come from one fixed-query `gh api graphql` call (owner, name
  and number as flag variables), for open pull requests only. A merged or closed pull request makes one `gh` call.
- Absent means "not read": a failed or `null` GraphQL reply leaves reviews, threads, auto-merge and last push unset
  while `DetailKnown` stays true; a first reply without `createdAt` is not `DetailKnown`.
- Rollout: the controller decodes machine replies with `DisallowUnknownFields`, so a machine sending these fields to
  an older controller has its inspect reply refused. Update controllers before machines (documented in
  `docs/terminal-workspace.md`).
- The 12 KiB reply cap still trims checks first; the new fields are small and a test holds that.

## CONCURRENCY-1: background work is stopped and joined (5 November 2026)

A Go concurrency audit of the connector, controller and agent loops found work that could outlive its owner or
wait without a deadline. Three focused batches merged on `main`:

- [#559](https://github.com/eyupio/reet.sh/pull/559) (`63f55857`): `Server.Close` stops the API queue, topic and
  refresh workers; health probe loops are joined; the backup-import pipe reader closes on every path; long-lived
  scan and stray-chat waits use tickers; a prompt, model change or mode change refuses once the chat supervisor
  is closed.
- [#569](https://github.com/eyupio/reet.sh/pull/569) (`60de2b79`): the software gate is now a context-aware
  weighted semaphore, so a launch waiting behind an apply gives up with its request, and the apply's work has a
  21-minute deadline; the chat ring evicts its oldest subscriber, as its comment promised.
- [#575](https://github.com/eyupio/reet.sh/pull/575) (`84bad853`): chat resume is tracked and canceled by
  `Close`; the API workers, health-prune loop and private-connection retry goroutine are joined, and their
  network and sweep contexts follow the server lifetime; model and mode controls check `closed` under the same
  lock as their session lookup.

`golang.org/x/sync` was already required indirectly; #569 imports its `semaphore` directly. Checks: `go vet
./...`; `go test -race` for `internal/terminalagent`, `internal/terminalbroker`, `internal/agenthealth`,
`internal/privateconn` and `internal/api` (549–575 s for API); the full CI race suite, browser matrix and Codex
review were green on each final head. Known limit: an apply runs the host's install step without a context, so a
hung filesystem syscall can still hold its goroutine, though launches now stop waiting for it.

## GAFFER-TIMEOUT-1: quiet commands survive the silence deadline (5 November 2026)

The turn watchdog treated a tool that did not stream output as a silent provider, stopping it after ten minutes.
It now excludes outstanding tools from both the provider-retry notice and the silence deadline. The two-hour hard
cap still applies, and the silence deadline resumes after the final tool finishes. An output-only update retains
the tool's last known state so late output cannot reopen a completed tool and suppress timeout recovery.

Three regressions failed before the watchdog fix: quiet command completion and a subsequent prompt, silence after
command completion, and the hard deadline. Removing the sparse-state fix separately makes the completion/silence
regression fail. After rebasing onto current main (`c594d81f`), full
`go test -race ./internal/acpagent ./internal/terminalagent -count=1`, `go vet ./...` and
`npm --prefix web run check` pass. The web check reports zero errors and two existing unused CSS selector warnings
in `LaunchBar.svelte` and `SessionHoverCard.svelte`. No Svelte files changed.
[PR #577](https://github.com/eyupio/reet.sh/pull/577), branch `fix/gaffer-quiet-command-timeout`, carries the fix;
final-head CI and review remain the merge gate. No live connector has been updated or restarted.

Review batches: [process recovery #480](https://github.com/eyupio/reet.sh/pull/480),
[retained state #485](https://github.com/eyupio/reet.sh/pull/485), and
[transcript retention #484](https://github.com/eyupio/reet.sh/pull/484). Eight reproduced defects are addressed across the three
focused batches below. Each PR records its checks and merge state. All handoff entries are retained when integrating the stack.

## OVERVIEW-1: the controller keeps and serves a session's opening prompt (5 November 2026)

First part of the session Overview. The controller now keeps the opening prompt with the session, so the
Overview can show it after the dialog is gone. Nothing in the browser reads it yet.

- **Kept.** A `prompt` column on `terminal_sessions`, capped at `terminalproto.MaxPromptBytes` (8,000 bytes).
  The session list and create replies carry only `promptExcerpt` (300 characters) and `promptMore`; the full
  text is read with `Store.TerminalPromptFor` and served by `GET /api/v2/terminal/sessions/{id}/prompt`
  (`internal/api/terminal_prompt.go`) as `{"prompt": ...}` with `Cache-Control: no-store`. A session that is
  not the caller's is a 404, a malformed id a 400.
- **Cleared.** Both archive paths remove the prompt from the row and from the create operation's stored
  parameters. An archived session answers an empty prompt. A retry of an archived session's create that
  carried a prompt now answers a conflict instead of replaying; this is deliberate and pinned by a test.
- **Never logged.** `auditTerminalLaunch` records no prompt; `TestLaunchAuditNeverCarriesThePrompt` fails
  when the prompt is added to the detail.
- **Docs.** `docs/terminal-launch.md` ("What is kept") and `docs/start-agent-session.md`, whose
  "the prompt is never kept" sentence was no longer true.

## AUTOTITLE-1: automatic session names (5 November 2026)

A session started without a title now gets a name that says what it is, instead of the first 80 characters of
the prompt or the same `Claude Code · studio` for every session started without one. The name is offline and
deterministic: no model, no network, no schema change and no setting. Choosing between this, the agent's own
title after its first turn and a model-written title was the one real decision; the first was picked because
the controller makes no outbound calls today and the README promises none.

- **Rules.** `terminalproto.PromptTitle` and `AutoTitle` (`internal/terminalproto/autotitle.go`), mirrored by
  `promptTitle` and `titleFor` in `web/src/lib/agentLaunch.ts`. The first line with something to say; blank
  lines, code fences, the skills picker's `Use the {name} skill.` line and punctuation-only lines are skipped;
  heading and bullet markers and English filler (a greeting, "please", "can you", "I'd like you to", "help me",
  "let's") are removed; the name ends at the first sentence (`e.g.` does not end one, and nothing under 8
  characters does); an ASCII first letter is capitalised; over 60 characters it is cut at the last space (not
  before the 20th character) with an ellipsis, else at 60. With nothing usable it is `<Agent> · <folder>`, or
  `Terminal` and `Terminal · <folder>` for a plain shell. Stated in full in
  [Launch requests → Session titles](docs/terminal-launch.md#session-titles).
- **One list of cases for both languages.** `internal/terminalproto/testdata/autotitle.json` (38 prompt cases,
  14 whole-title cases) is read by `autotitle_test.go` and `web/src/lib/agentLaunch.test.mjs`, so the title the
  dialog suggests cannot drift from the one the controller would give. The expected values were worked out by
  hand from the rules before the code existed. Five deliberate breakages of the rules (the limit scan, the
  abbreviation guard, the minimum word, the minimum sentence, the skills line) each failed the fixture.
- **Where it applies.** `POST /api/v2/terminal/sessions` names a blank or whitespace-only title (an absent one
  used to be a 400); `POST`/`PUT /api/v2/session-backlog` names a draft that has a prompt or note but no title
  (nothing to go on is still a 422); `projectrun.SelectionTitle` calls `PromptTitle`. The plain-terminal dialog's
  **New session name** is optional (it was pre-filled with `Shell` and required) and its placeholder and hint
  follow the folder; Home's machine-card shell takes the same name. A title that is sent or typed is kept as
  typed, Rename is untouched and sessions that already exist are not renamed.
- **Visible changes.** An unnamed plain shell is now `Terminal` or `Terminal · <folder>`, not `Shell`, matching
  the `Terminal · <folder>` that "Open a terminal here…" already used; the `home`, `start-agent` and
  `terminal-organisation` e2e expectations moved with it and `terminal-start` gained the folder case. A long
  prompt now gives a 60-character name ending in an ellipsis, not a hard cut at 80, so `defaultTitle`'s unit
  expectations changed.
- **For the next author.** `terminalhost.TestPackageSourceHasNoInvisibleCharacters` also scans
  `internal/terminalproto/*.go`. Write zero-width, bidi and line-separator characters as `\u` escapes, and check
  a new file's bytes afterwards: tooling can turn an escape into the literal character, which is what that test
  caught here.
- **Not covered.** A session started with no prompt, or one whose prompt is typed into the terminal afterwards,
  can only be named from its agent and folder; a name from what the agent actually did needs the agent's own
  title or a model, both ruled out above. The filler list is English only. Go's `(?i)` folds a few non-ASCII
  letters that JavaScript's `/i` does not, so a prompt written with, say, a long s could be named differently by
  the two; no real prompt is affected and the fixture holds no such case.

## GAFFER-CLINE-1: Cline chats in Gaffer (5 November 2026)

Cline CLI 3.0.74 runs its ACP server with `cline --acp`, and reet now offers it as a Gaffer chat beside
OpenCode. The profile (`internal/acpagent/cline.go`) launches `cline --acp --auto-approve false`, requires
`act` mode, reads the boolean `auto_approve` back as `false` before any turn (`Spec.RequireOptions`), applies a
chosen model with a confirmed `session/set_config_option` because Cline ignores one at `session/new`
(`Spec.ConfirmModelBySwitch`), and routes the person's own `~/.cline` through `CLINE_DIR` while keeping the
private chat home. The reviewed tool set, the Cline handle shape (`<ms>_<5 base64url>_cli`), the descriptor
(history, images, model, approvals, act/plan modes) and the scan probe are wired through
`internal/terminalproto`, `internal/scan`, the browser's chat adapter table and both start dialogs. Cards now
show a read's file, an editor's `path`/`new_text` as a diff and a `commands` list as the command; a card with
nothing to review stays unapprovable. Opt-in `TestRealClineStart` passes against the installed binary with no
prompt: version floor, identity, act mode, a confirmed plan/act switch and a confirmed model switch. Two prompts
with every permission rejected recorded the cards, and a stored conversation replayed through `session/load`.
The evidence and limits are in [the Cline ACP probe](docs/research/cline-acp-probe.md). Not covered: images, MCP
tools or subagents in a turn. A provider failure Cline logs without an ACP frame is now shown as that turn's error
([PR #573](https://github.com/eyupio/reet.sh/pull/573)); in-place reconnect for a lost Cline chat is in review
([PR #567](https://github.com/eyupio/reet.sh/pull/567)).
Merged as [PR #554](https://github.com/eyupio/reet.sh/pull/554) (`3919deef`); all 18 checks passed, including the
browser suite that runs the new `chat-cline.e2e.mjs`.

### The model picker shows the agent's whole catalogue (5 November 2026)

Cline reports 307 models; a live control reply capped a model selector at 16 values inside 10 KiB, so most of the
catalogue was unreachable without typing an id. `acp-model-catalogue-v1` widens the model selector to 512 ids in a
snapshot bounded at 56 KiB — the worst case a valid snapshot can reach, and still one frame because `MaxControlBytes`
is 64 KiB — while the effort selector keeps its 16-value bound. The session keeps every value the agent offers;
`chatHost.forConnection` trims the model list to 16 for a connection that did not negotiate the feature, on a copy, so
an older controller never receives a frame its decoder rejects, the session snapshot stays whole, and a reconnect with
the feature gets the full list without asking the agent again. Tests: `TestModelCatalogueBoundsTheModelSelectorNotEffort`
(proto, both directions on the wire), `TestModelCatalogueKeepsEveryOfferedModel`, `TestModelCatalogueIsTrimmedPerConnection`
and a 307-entry case in `web/src/lib/models.test.mjs`. In review as
[PR #586](https://github.com/eyupio/reet.sh/pull/586).

## SEO-1: public search surface and documentation (4 November 2026)

The public-search audit and its current status are in [SEO_AUDIT.md](SEO_AUDIT.md). The continuation state, remaining
tasks and a copy-paste starting prompt are in [the SEO follow-up handoff](docs/handoffs/seo-follow-up.md). Read that
file before changing the public routes, metadata, sitemap or docs.

Merged on `main`: real 404s and private-route noindex (#479, `e54cd9c`); install-first landing CTAs (#481, `aed7d36`);
server-rendered community and public lists (#483, `187a5c7`); hosted skill page metadata and sitemap entries (#487,
`7f713c2`); documentation first published at `/docs/` (#489, `3d8d1bb`) and now a standalone MkDocs site built by
`.github/workflows/docs.yml`; AI-crawler files and public nav pointing at the docs (#494,
`5b4050a`); the first guides and skills.sh comparison (#496, `f507b6f`); the sitemap crawl contract test (#497,
`b6b6ea4`); readable public list addresses for new lists (#502, `35c2182`); Open WebUI and LibreChat comparisons plus
the no-VPN guide (#505, `e95cb81`); and landing-page links to the guides (#507, `701e1cd`).

The docs site is no longer embedded in the Go binary. `mkdocs.yml`, `docs/requirements.txt`, `hooks/docs.py`,
`overrides/main.html` and `.github/workflows/docs.yml` build it from the same `docs/*.md` files and publish it to GitHub
Pages. The docs own `https://reet.sh/`; `DOCS_URL` points there, and the repository variable `DOCS_SITE_URL` overrides
the canonical origin for another deployment. The app keeps only the public routes, sitemap and AI-crawler files.

Remaining, in order: a readable-link action with permanent redirects for existing public lists; external measurement
(Search Console, Bing, and an explicit analytics decision); the marketing/app bundle split; and more intent content.
Do not treat the lab timings in the audit as field data, and do not add a third-party tracker while "no telemetry"
remains the default.

## GAFFER-PREVIEWS-1: agent image previews and continuation (4 November 2026)

Start a new environment with [the current continuation handoff](docs/handoffs/gaffer-continuation.md). It records the
delivered Gaffer PRs, their final heads, validation results and merge shas using repository-relative paths. The image
preview implementation is in [PR #498](https://github.com/eyupio/reet.sh/pull/498) on `feat/chat-image-previews`, merged as
`0de8f7d`; its [feature handoff](docs/handoffs/chat-image-previews.md) covers the wire contract, private snapshot cache, UI,
recovery, limits and checked screenshots. [PR #490](https://github.com/eyupio/reet.sh/pull/490) (session actions) merged as
`99c7fd8` and [PR #493](https://github.com/eyupio/reet.sh/pull/493) (startup and backlog settings) merged as `918f452`.
The handoff refresh that records all three landed as #503 (`4539386`) and #504 (`80ae4cc`). No Gaffer pull request remains
open; follow the "Starting a fresh session" checklist in the continuation handoff before changing anything.

## GAFFER-START-1: launch and backlog model/effort selection (4 November 2026)

Implementation and verification are tracked in [the startup-options handoff](docs/handoffs/start-agent-options.md).
Both start dialogs and backlog creation/editing retain model and native effort choices. The new `acp-start-options-v1`
contract prevents older live-control connectors receiving unsupported create fields. Runtime confirmation precedes readiness
and the opening prompt. The local web, browser and focused race checks pass. [PR #493](https://github.com/eyupio/reet.sh/pull/493) merged as
`918f452`, after the toolbar PR #490 merged as `99c7fd8`; its final head passed every check and no review findings were open.

## GAFFER-LIVE-1: live settings and durable message delivery (4 November 2026)

Implementation and verification are tracked in [the live-controls handoff](docs/handoffs/live-chat-controls.md).
This covers negotiated runtime model/effort selectors, capability-aware steering, durable shared queues and recovery
from uncertain delivery. [PR #488](https://github.com/eyupio/reet.sh/pull/488) merged as `ac5938d` after final-head checks and review fixes passed.

## GAFFER-BOUNDS-1: keep history bounded around waiting cards (4 November 2026)

Final asynchronous review follow-up: the process waiter could mark the chat closed before buffered final ACP frames reached
a slow event sender. The connector now owns its stdout pipe, observes exit independently, retires descendants and gives the
reader up to two seconds to drain before ending the session. A permanently blocked sender still cannot hide an exited agent.
Tests reproduce loss of a flushed final chunk before the fix and cover bounded shutdown with a blocked callback. The fake
child disables the race runtime's exit sleep for this case so that sleep cannot hide the shutdown race. This resolves the
post-merge review on #485 as part of #484; independent exit detection and the inherited-stderr bound are preserved.

Branch `fix/gaffer-transcript-bounds`. The browser reducer skips individual pending approvals and questions when evicting
old history, so one unanswered card no longer pins every later item. Tool commands, paths and diffs now count towards the
256 KiB text budget. Pending cards remain answerable; if cards alone exceed the budget they are retained. This is a reducer
change, with no new UI controls, wire fields or dependency. `docs/structured-sessions.md` describes the retention rule.

`chatBounds.test.mjs` covers both failures and is included in `npm test`: a waiting approval and question followed by a
long stream retain the cards and newest output within the limits; tool-only command/path/diff payloads also trim. Both
tests failed against the unchanged reducer and pass with the fix. Validation and merge status are recorded in the PR.

CI follow-up: the parallel terminal-upload fixture used arrival-order filenames for identical files, then asserted selection
order, so reversed request arrival caused a false failure. Distinct file bytes now determine fixture paths; the second
response deliberately completes first and the test still requires insertion in selection order. No upload runtime change.
## GAFFER-STATE-1: retain live choices and actionable replay (4 November 2026)

Branch `fix/gaffer-recovery-state`. `chatHost.setModel` now copies the process's actual model read-back into the live
session and its existing restart record, even on a failed or unconfirmed switch. It does not guess from the requested model
or recreate a removed record. A plain refusal keeps the old read-back; an unknown model clears the old launch choice.

The controller's live chat ring evicts ordinary history before pending approval/question cards and the latest status,
matching the connector's detached queue. Count and byte limits remain hard; pending state alone can still fill them.
Resolved cards become evictable again. Live subscribers receive the newly published event even when that event itself
cannot be retained in a ring full of pending state. The stored transcript remains bounded and connector reconnect replay
continues to restore retained pending cards after a controller restart.

Regression tests first reproduced stale restart models (confirmed, unknown and different-model replies) and loss of a
waiting approval/status at both ring limits. Tests also cover explicit refusal, question cancellation and a ring filled
entirely with pending state. Validation and merge status are recorded in the PR. No wire or dependency changes.
## GAFFER-RECOVERY-1: retire unusable agent processes (4 November 2026)

Branch `fix/gaffer-process-recovery`, integrated by `fix/gaffer-recovery-state`. An EOF or oversized frame in the ACP stdout
reader retires the process group. Process exit is observed independently because children may inherit stdout. A broken output stream can no longer leave a chat
waiting on a live process until the turn timeout. Provider errors recovered from the local log also mark the turn as
cancelled for incoming requests and arm the existing forced-stop grace, while keeping the provider error visible.

Regression tests reproduce closed stdout, an oversized frame and a provider failure whose adapter ignores cancel.
The integration regression also covers a leader exiting while a child keeps stdout open; waiting for the reader first
delayed that exit, so the integration fix keeps the independent process waiter and kills the group when reading ends.
A one-second `exec.Cmd.WaitDelay` also bounds inherited stderr held by a descendant after leader exit; both stdout-only
and stdout-plus-stderr cases are covered. Without that bound, exec's stderr copier can hide the leader's exit indefinitely.
All three failed before the fix and pass under the race detector afterwards. The full ACP race suite passes. The orphan
cleanup test now checks live process-group retirement, matching production, because a container's PID 1 may retain exited
zombies; the old test also failed on the unchanged baseline here. Full validation and merge status are
recorded in the PR. Follow-on review batches cover retained chat state and browser transcript limits.

Recovery still requires the agent's saved conversation. A framing failure ends the process; it does not retry a prompt
automatically. No wire format, dependency or agent availability changes.

## AUTH-CARD-1: the account card agrees with the landing page (3 November 2026)

Branch `claude/fervent-wright-5omayg`. Copy only. The card on the sign-in, sign-up and first-run owner-setup pages
(`AuthCard.svelte`) now says "Your whole AI setup. Ready to control." instead of "...On every machine.", so it agrees with the
landing page's h1. The page title and meta description keep "on every machine" on purpose. `first-run.e2e.mjs` (`authfit`)
holds the words. Checks and screenshots in [UI verification](docs/ui-verification.md).

## LANDING-1: the control-room landing page (3 November 2026)

Branch `claude/fervent-wright-5omayg`. Copy and UI only; no API, database or server change. The signed-out page now says
what reet.sh is for: command agents on any machine, sync skills and setups to one machine or all, back them up, and bring
an empty machine up to speed with one command. The h1 second line is "Your whole AI setup shows up, ready to control."
(`copy.ts`, `index.html` h1, meta description and crawler copy, README, `docs/yorkshire-copy.md`, tests). New:
`ControlRoom.svelte` (hero diagram with a wide and a compact drawing, replay, terminal built from `enrollCommands` with env
`fresh-vps`; final state at once under reduced motion, no Replay button), a copy chip under the buttons, a four-tile strip,
three new steps, and `StartFresh.svelte` (before and after slider, native range input with a label and `aria-valuetext`).
The terminal palette lives in `landingTerminal.css` because `theme.test.mjs` forbids literal colours in components. The "On
the workbench" line now says "Agent software: OpenCode today, more agents to come." and the sync card says a machine applies
a plan only when its owner has opted in. The sign-in card headline was left alone at first; see AUTH-CARD-1 below.

Checks: `npm run check` (unit suites, svelte-check 0 errors, 0 warnings), `public-workspace.e2e.mjs` and `first-run.e2e.mjs`
against `vite preview`, `go test ./web/`. Screenshots and scope in [UI verification](docs/ui-verification.md). The copy chip
omits `--server` as approved: the command takes the server from `REET_URL`, so it works on a machine where that is set.
The boot-shell skeleton in `index.html` still draws the old hero shape; it is decoration and was not changed.

## CHAT-REATTACH-1: reconnect a lost chat in place (3 November 2026)

PRs [#451](https://github.com/eyupio/reet.sh/pull/451) (merged: design, plan, protocol, negotiation, store, machine and
controller) and [#459](https://github.com/eyupio/reet.sh/pull/459) (route, web, browser contract, docs), branch
`claude/tender-johnson-k3xmf1`. A lost or crashed (`exited`) structured OpenCode chat now reconnects **in place**: same reet
session row, URL and transcript, through `session/load`. Feature `chat-reattach-v1` (requires `acp-v1` and `acp-history-v1`),
wire body `chat_reattach` carrying the row's launch data, capability `chatReattach`. The machine drops the replay (the
controller holds the transcript), forwards the post-load idle status and interrupted-tool closes, adopts a live process
instead of restarting it, and refuses a conversation another session owns ("This conversation is open in another session.").

Controller `Broker.ReattachChat` is single-flight per session (concurrent callers join one machine call), bounded at 75 s,
leaves the row untouched on error and never creates a row. The store allows `lost|exited -> running` for structured chats
only (exit code cleared), and a continuation row whose create operation failed can no longer itself be continued, closing
the many-rows path in the legacy flow. Route `POST /api/v2/terminal/sessions/{id}/reattach` (204; codes
`chat_reattach_unsupported`, `chat_reattach_not_closed`, `chat_reattach_no_conversation`, `chat_reattach_conflict` and `chat_reattach_trust` (with a `detail`) answer 409, `chat_reattach_failed` 502; a timeout or offline machine answers 503 "Reconnecting took too long. Try again.").
Web: one automatic attempt per session per page load; "Reconnecting to your conversation…"; failure shows the reason with
**Try again** and **Start a new chat**; manual **Reconnect**; the header menu no longer offers the new-row "Continue in
Gaffer with history…" for a chat that can reconnect. `ended` chats and older connectors keep the new-row path, now with a
single-flight guard and an in-pane "Starting a new session…". Docs: `docs/structured-sessions.md`, spec and plan under
`docs/superpowers/`.

Checks. Full run, earlier in Task 10: `go vet ./...` exit 0 and `go test -race -timeout 40m ./...` exit 0, 40 packages `ok`, including `internal/api` 791.503s and `internal/store` 339.278s. Those two slow packages were not re-run afterwards. Re-run in the fix round (raw logs `.superpowers/sdd/2026-11-03-chat-reattach/task-10-go.log` and `task-10-web.log`, not committed): `go vet ./...` exit 0 and `go test -race -count=1 ./cmd/... ./internal/terminalproto/ ./internal/terminalagent/ ./internal/terminalbroker/` printed `ok .../cmd/reet 16.283s`, `ok .../internal/terminalproto 1.184s`, `ok .../internal/terminalagent 8.255s`, `ok .../internal/terminalbroker 63.251s`. In `web/`, `npm run check` ran its unit tests (53 `# fail 0` summaries, no non-zero `# fail`) and svelte-check printed `COMPLETED 360 FILES 0 ERRORS 0 WARNINGS 0 FILES_WITH_PROBLEMS`; `npm run build` printed `built in 9.58s`.

Known limitations: auto-reattach sends no folder-trust confirmation, so a folder with OpenCode plugins refuses with the
trust message (resolved by follow-up F3 below: the person can now reconnect in place with **Trust this folder and
reconnect**, beside Continue with history and Start a new chat); YOLO chats are
refused (not downgraded) if the account or the machine no longer allows YOLO; while the machine is offline the pane shows the idle ended note (not a waiting state) and a failure consumes the one
automatic attempt; Claude/Codex chats are not covered. Not exercised: a live OpenCode provider reload and production chats.

Final-review fix wave (commits `ae23404` Go, `c4cd2b0` web, and this docs commit). A person's End of a Gaffer chat is
answered by the machine with `exited` (code 0), so ended chats were stored `exited` and auto-reattached; the store now
records that close of an `end_requested` structured row as `ended`, and `ReattachChat` refuses any row with an end
operation (older rows stored `exited`/`lost`) with `chat_reattach_ended` ("You ended this chat. Continue it with history as
a new session."), which the browser treats quietly on an automatic attempt. Reattach checks the account's Allow-YOLO setting
first (`chat_reattach_yolo_not_allowed`), keeps the machine's YOLO refusal (`chat_reattach_yolo_refused`) and capacity
(`chat_reattach_capacity`) instead of the retryable failure, and a chat still starting on the machine (its conflict result
carries the detail `starting`, sent only to a controller that advertised `chat-reattach-detail-v1`, so either side can be
updated first; an older controller shows its old "open in another session" wording for that rare moment) answers
`chat_reattach_starting` ("Still reconnecting. Try again in a moment.") instead of "open in another session". A non-OpenCode row answers "This chat cannot be reconnected in place."; the generic failure
now reads "The machine did not bring this chat back. Try again, or start a new chat.". Web: Try again only for retryable
codes, the failure card is an alert, one live region while pending, "Reconnecting" for an exited row in the sidebar, and
(ruling R10) a folder-trust refusal shows the plugin files and re-offers Continue with history (new row, `trust: true`) in
the pane and the header menu, without Try again.

Checks for the fix wave (raw logs `.superpowers/sdd/2026-11-03-chat-reattach/final-fix-go.log`, `final-fix-go-full.log`
and `final-fix-web.log`, not committed): `go vet ./...` exit 0; `go test -race -count=1 ./internal/store
./internal/terminalbroker ./internal/terminalagent ./internal/terminalproto ./internal/api -run
'Reattach|Observe|End|Yolo|Resume'` all `ok`; full `go test -race -count=1 ./cmd/... ./internal/terminalproto/
./internal/terminalagent/ ./internal/terminalbroker/ ./internal/terminalhost/` all `ok` (terminalbroker 63.564s); full
`go test -race -count=1 -timeout 40m ./internal/api/ ./internal/store/` printed `ok .../internal/api 731.812s` and
`ok .../internal/store 308.621s`. Other Go packages were not re-run in this wave. In `web/`, `npm run check` (53 `# fail 0`
summaries, none non-zero; svelte-check `COMPLETED 360 FILES 0 ERRORS 0 WARNINGS 0 FILES_WITH_PROBLEMS`), `npm run build`
(`built in 12.38s`), and against a preview build `chat-recovery.e2e.mjs` and `sessions.e2e.mjs` passed. `chat.e2e.mjs`
failed twice in its Skills-install section while the full Go race run loaded the machine, then passed on a third run
after it finished; the same section also failed once on the unchanged baseline build in this session.

Follow-up F1: refuse an archived chat (local branch `followups/reattach`, commit `dd9a731`). `Broker.ReattachChat` refuses a
row with an entry in `terminal_session_archives` (new store helper `TerminalArchivedFor`, scoped by account) with
`ErrReattachArchived` before any machine call; the route answers 409 `chat_reattach_archived` "This chat was archived, so
its history is gone.", and the web treats it as final (no Try again, no Continue with history). In the final-review wave the
archived check moved above the ended checks, so a chat that was both ended and archived answers archived too. Checks
(`.superpowers/sdd/reattach-followups/f1-report.md`, not committed; all focused, none full): `go vet ./...` clean; `go test
-race -count=1` for `internal/store -run 'Archiv|Reattach|Observe'`, `internal/terminalbroker -run Reattach` and
`internal/api -run Reattach` all `ok` (timings not recorded); `reattachState.test.mjs` `pass 10 fail 0`; `npm run check` 0
errors 0 warnings.

Follow-up F2: negotiate the still-starting marker (commits `28f3e1c` code, `5e2555b` docs, nit `d3f0156`). New feature
`chat-reattach-detail-v1`: the controller advertises it, the connector echoes it, and the machine sends `conflict` with the
detail `starting` only to a controller that advertised it (any other gets a plain `conflict`), so the controller and the
machine can be updated in either order. Checks (`f2-report.md`, `f2-verify.log`, `f2-api-full.log`, not committed):
`go vet ./...` clean; full `internal/terminalproto` `ok 1.132s`, `internal/agenthealth` `ok 6.852s`, `internal/terminalagent`
`ok 7.673s`; focused `internal/terminalbroker -run 'Reattach|Conflict|Feature'` `ok 10.679s` and `internal/api -run
'PrivateControl|Reattach'` `ok 26.007s`; full `internal/api` `ok .../internal/api 714.878s` (an earlier run hit the default
10-minute test timeout and was re-run with `-timeout 40m`).

Full `internal/store` once on the final code before the final-review wave (`final-store-full.log`): `ok
.../internal/store 255.988s`; it was not re-run after that wave, which did not touch `internal/store`.

Follow-up F3: trusted in-place reconnect (local branch `followups/reattach`, commits `24c602b` Go, `b0b6fd9` web, and this
docs commit). `POST /api/v2/terminal/sessions/{id}/reattach` takes an optional body: none, `null` or `{}` mean no trust,
`{"trust": true}` passes trust for that request (the controller never stores it; each click is one request), anything
else is 400 before the machine is asked. The machine keeps the trust in its restart record exactly as for any chat started
with trust (including Continue with history), so its own restart reopens the chat with it (record kept up to 7 days or 3
failed reopens); a later in-place reconnect from the controller asks again.
`Broker.ReattachChat(ctx, uid, session, trust)` sends `Trust` on the wire, records `trust` in the `chat-reattach` audit
event, and keys its single-flight on session and trust, so a trusting call never joins a non-trusting one. The
`chat_reattach_trust` sentence is now "This folder has OpenCode plugins, which run as code on this machine, so reet did not
reconnect automatically. Reconnect only if you trust it.", and the card shows only the plugin-file list from the machine's
detail (not its "Start a chat only…" sentence). Web: after that refusal the card offers **Trust this folder and reconnect** (primary), **Continue
with history** and **Start a new chat**, still no Try again; only that click sends `{"trust":true}` (the automatic attempt
and every other request send the earlier `{}` body). Checks (raw logs `.superpowers/sdd/reattach-followups/f3-*.log`, not
committed): `go vet ./...` exit 0; full `go test -race -count=1 ./internal/terminalbroker/` printed `ok
.../internal/terminalbroker 72.247s` and full `./internal/api/` printed `ok .../internal/api 733.013s`; `internal/agenthealth`
and `internal/terminalproto` full `ok`; other Go packages were not re-run. In `web/`, `reattachState.test.mjs` `# pass 14`
`# fail 0`, `npm run check` (svelte-check `COMPLETED 360 FILES 0 ERRORS 0 WARNINGS 0 FILES_WITH_PROBLEMS`), `npm run build`
(`built in 9.64s`), and against a preview build `chat-recovery.e2e.mjs` and `sessions.e2e.mjs` passed.
F3 fix round (review): the copy says plugins run as code on the machine, the card shows only the plugin-file list, the docs
say what the machine keeps, and a terminalagent test pins the machine's trust gate on a reattach (refused without trust,
nothing left running; starts with trust). Checks: `go vet ./...` exit 0; full `go test -race -count=1
./internal/terminalagent/` `ok 7.914s`; `./internal/api/ -run 'Reattach|RefusalSentences'` `ok 35.018s`; `reattachState.test.mjs`
`# pass 15` `# fail 0`; `npm run check` 0 errors; `npm run build` `built in 10.56s`; `chat-recovery.e2e.mjs` and
`sessions.e2e.mjs` passed against a fresh preview of that build.

## CHAT-RECOVERY-1: continue a closed chat with saved history (3 November 2026)

PR [#445](https://github.com/eyupio/reet.sh/pull/445), branch `fix/chat-history-recovery`, implementation commit `e511a4a`,
based on main `e91afad`, adds **Continue with history** beside the ended-chat reason
when the OpenCode conversation id is saved and the machine advertises history support. It uses the existing exact-id
continuation operation, creates a new row with the same folder/model, and sends no prompt automatically. A missing process
no longer claims that native history could not be restored. The banner keeps the fresh-chat choice.

Failed connector-start restores retain the native id in their lifecycle reports. A successful manual continuation replaces
the old restart record. Manual and automatic loads share a supervisor gate that refuses a conversation already starting,
running, or held by a process whose termination is unconfirmed. No protocol or database migration and no dependency added.

Checks: web unit suites, svelte-check (0 errors/warnings), production build, the dedicated recovery browser contract,
existing OpenCode chat browser contracts (legacy and descriptor capabilities), Claude/Codex browser contracts and Go vet pass. The recovery contract covers
1360/390/320 px, both themes, lost/exited/ended states, unsupported history, offline machines, reload refusal and the same
operation id after an uncertain reply. New Go recovery tests pass under race; the retained-id and restart-record tests and
the web wording tests fail on unchanged main. Captures and scope are in [UI verification](docs/ui-verification.md).

The repository race run is not green: `TestCloseKillsOrphanedGroupMembers` and
`TestListModelsTimeoutKillsTheProcessGroup` also fail on unchanged main in this environment. The API suite exhausted its
ten-minute package budget while an unrelated authentication test was running; that test and
`TestChatResumeNeedsAClosedChatOwnedHere` pass together under race when targeted. The full run passes terminalagent,
terminalbroker and store. A separate affected-package run hit the timing-sensitive
`TestSwapToChatTerminatesALingeringTerminal`; it passes in isolation on both main and this branch.

CI on `c80bfce` passes the full browser matrix and native service jobs, including macOS. Its full race run exposed a
timing race in `TestSupervisorResumesPersistedChat`: the provisional Running row preceded process installation and the
registry write. The test now attaches before startup and waits for the real lifecycle receipt before ending the chat.
Before the fix it failed three times in 100 repetitions; the fixed recovery/supervisor group passes 100 repetitions under
race. Automated code review could not run because its usage limit was reached; the patch was reviewed directly.

Production chats, live provider reloads and preservation of a running reply across connector death were not exercised.
The owner authorised watching the PR, fixing failures and merging once CI is green. Nothing is deployed or merged. Next: watch
[#445](https://github.com/eyupio/reet.sh/pull/445) and CI, starting with `internal/terminalagent/chat_registry.go` and
`web/tests/chat-recovery.e2e.mjs`; restoring a lost conversation still depends on its native store being available.

## Gaffer block mascot and executive summaries (2 November 2026)

Draft [#418](https://github.com/eyupio/reet.sh/pull/418), branch `feat/gaffer-block-mascot`, based on main `1e626ff`, carries the owner-approved block portrait,
stepped facial animation and compact status card. Yorkshire quips rotate every eight seconds while
the agent works. Pause, hidden-tab handling and reduced motion cover the decoration, including idle
expressions. The final working and summary mocks are in `docs/images/ui/gaffer-*-desktop.png`.

The collapsible executive summary uses retained chat events: the latest turn while a session is
open, all retained activity when it ends. It shows the agent’s final handover, reported file and
command operations, failures, unfinished calls and a next step. Truncated history and missing
handovers are explicit. No provider request or stored report is introduced.

Web unit tests, svelte-check, the production build and the Gaffer browser matrix pass. The browser
matrix covers replay, phase changes, animation pause, summaries, failures, session end, keyboard
controls, both themes and 320/390/768 px widths. See [UI verification](docs/ui-verification.md) for
the approved mocks, phone captures and animation recording. The full chat browser fixture, Go vet,
race-enabled web/CLI packages and the API suite without race pass. The full race attempt exposed
existing process, model-listing and missing-tmux fixture failures, reproduced on unchanged `1e626ff`.
API uses the documented local non-race run; the completed UI build passes the web/CLI race rerun.
Main `4959472` (#416) was merged without rewriting history to bring in the startup-hook race fix;
both supervisor-resume regression tests now pass under race. CI results belong in the PR.
Next: review the draft PR and its checks; merge only on the owner’s instruction.

## UI refresh: directory, inbox, admin and machines (2 November 2026)

The owner asked for a page-by-page UI refresh with a UX focus. Each family was built test-first,
photographed at desktop and phone widths in light and dark, reviewed, and merged only after CI was
green. Contracts and rulings are in `docs/superpowers/specs/2026-11-02-directory-pages-refresh.md`,
`...-skills-machines-findability.md`, `...-inbox-pages.md`, `...-admin-users.md`,
`...-admin-team-usage.md`, `...-admin-tickets.md`, `...-admin-email.md` and
`...-admin-notifications.md`, `...-admin-backups.md`, `...-admin-settings.md`,
`...-admin-activity.md`, `...-account-sessions.md`, `...-account-tokens.md`,
`...-account-profile.md`, `...-account-security.md`, `...-account-mcp.md`,
`...-skill-card-machine-actions.md`, `...-machine-add-skill.md`,
`...-skill-remove-from-machine.md`, `...-skills-sync-vocabulary.md`,
`...-written-skill-record.md`, `...-written-skill-lifecycle.md` and
`...-session-skill-drafts.md`; checks are in [docs/ui-verification.md](docs/ui-verification.md).
The refresh merged through #457. Merge main into the next branch as later work lands.

**Merged, all with every required check green:**

| PR | Merge | Scope |
| --- | --- | --- |
| #395 | `5c7463b` | Lists, Community and Teams into one collection grammar: panel, count, search, honest scope chips, one row anatomy, folded Teams management, phone toolbar. |
| #400 | `01bb06f` | Jump finds shelf skills with saved sync intent; “Skills you create” rows say where each skill is; Machines search by host, alias and skill. |
| #403 | `26797e8` | Setups list in the same panel with a count, search, relative recency and honest revision wording. |
| #405 | `1235328` | Per-scope machine cleanup moved from the card disclosure into **Settings → Access** (audit H3). |
| #407 | `3211b32` | Notifications and Support in one panel grammar; locked security preference is an **Always on** fact; Support status chips and search. A Codex review found that the unread list must stay server-side; fixed and regression-tested. |
| #408 | `f051948` | `/sync` marks Machines current (desktop and phone); simple Sync source renamed **Everything on my shelf**. |
| #410 | `d61c6c0` | Admin → Users as one people panel with count, search, role chips, per-person rows and the existing Manage actions; invite is its own card. |
| #415 | `1e626ff` | Admin → Team usage as one report panel: count, search, List/Tag/Sort, one row per skill with metrics and Team disclosure; wide table and duplicate mobile cards removed. |
| #419 | `d374e00` | Admin → Support as one ticket queue panel: count, search, status chips whose figures follow the search, one row anatomy; detail and composer unchanged. |
| #420 | `4f18c10` | Admin → Email as one outbox panel: count, search, state chips whose figures follow the search, one row anatomy, retry; settings unchanged. |
| #421 | `90fa3a6` | Admin → Notifications as one grouped defaults panel: count, Save, locked facts, retry; no search. |
| #422 | `a08acb1` | Admin → Backups copies and runs in the collection grammar: counts, search, overlapping location chips, verify damage, retry; settings unchanged. |
| #423 | `6fc3197` | Admin → Settings as one instance panel: six grouped settings, Save, read-only view; immediate cards now only on Settings. |
| #424 | `ba3ce32` | Admin → Activity as one audit panel: newest 100, count, search, area chips, one row anatomy; no API change. |
| #425 | `85bde14` | Account → Sessions as one browser panel: count, search, this/other chips, sign-out actions; search reads friendly devices. |
| #426 | `415d2dd` | Account → API tokens as a create card plus one token panel: count, search, used/never chips, one-time secret, revoke. |
| #427 | `00840c7` | Account → Profile as one grouped preferences panel: count, Save, blank-date fallback; viewer name read-only. |
| #428 | `f920dd2` | Account → Password as one panel: three fields, head action, mismatch check, successful body and notice. |
| #429 | `f7e668c` | Account → MCP access clients in one panel: count, search, OAuth/bearer chips, connection actions; page sections unchanged. |
| #432 | `106c2fc` | Skill cards: a large fleet collapses to one installed/missing/unconfirmed summary; unconfirmed machines offer Add now that queues a scan. |
| #434 | `6d75e72` | Machines: Add a skill… picker on the card menu, shelf candidates with Installed tags, existing install intent; disabled without an agent. |
| #438 | `2a334ca` | Skill cards: Remove… opens Sync with an exact-set removal prepared; no write until save, preview and approval. |
| #441 | `e91afad` | Machines audit minors: profile-format timestamps, a single Commands close, shared icons, attention order and a large-fleet status filter; paused cards show an amber dot. |
| #446 | `d759055` | Skills and Sync vocabulary: one glossary, Sync only under Machines, **Write a skill** with a next-step link, and install locations and sync plans on cards and warnings. |
| #450 | `5ee31b8` | Written skills: **Your drafts** holds unpublished skills, a published one appears once as its shelf card with **Edit written skill**, and the authoring list names the machines. |
| #455 | `790a489` | Written-skill lifecycle: the shelf row is a hidden projection, so unpublish/delete hide it while tags, notes and list membership survive; rename carries the hidden state and all list readers exclude hidden rows. |
| #457 | `b0b087a` | Session picker: **Your skills** installs a private draft or a published skill through the same private-revision preview/approve path; a missing or unavailable skill has its own refusal. |

**What the refresh established.** The directory pages (Lists, Community, Teams, Setups) share one
bounded surface, a count, search, scope chips whose figures follow the search, one row anatomy and
panel-owned loading, empty and error states. Lists keeps Rows/Cards. Teams folds management per
group. Setups keeps its master-detail layout.

**Findability and safety.** Jump searches sessions, folder projects, shelf skills and machines; a
skill result carries `Skill · <saved intent>` and opens `/skills?skill=<id>`. A created skill shows
saved intent first, then observed state, then **Not on a machine yet**; published skills link to
their shelf card. Machines search reads host, saved alias and every observed skill. Per-scope Forget,
whole-machine Forget and Revoke now share **Settings → Access** with the usual confirmations.

**Inbox and admin.** Notifications loads its unread tab with the server-side predicate, so an old
unread beyond the newest page remains reachable; its chip and **Mark all read** use the server's
authoritative count. Support has status chips whose counts follow the search. Admin Users is the
proving page for Account & Admin: invite card, people panel, role chips, inline role change and the
existing Manage disclosure, with no wide table or horizontal scroller. Team usage is the second:
one report panel with its existing filters, one row per skill, and the drift and people views still
folded below. The Support queue is the third: one ticket panel with a live count, search, status
chips whose figures follow the search, and the conversation detail unchanged. The Email outbox is
the fourth: one message panel with a live count, search, a chip for every state, and rows that stack
on a phone; the mail settings stay above it. Notification defaults is the fifth: one grouped panel
with a count, Save, the existing locked security facts and rows that stack on a phone. Backups is
the sixth: copies and runs each have a count, search, chips, one row anatomy and a phone layout;
the schedule, destinations and import stay as they were. Settings is the seventh: six grouped
instance settings in one panel with Save, plus the immediate Approvals, Repository maintenance and
Software artifact mirror cards that now appear only there. Activity is the eighth: the newest 100
events with a count, search, area chips and one row anatomy. All seven Admin sections are now in the
collection grammar. Account Sessions is the first Account page in it: count, search, this/other
browsers and the existing sign-out actions. API tokens is the second: a create card, then a token
panel with count, search, used/never chips and the existing revoke confirmation. Profile is the
third: eight preferences in four groups with Save in the panel head and a blank date format shown as
the device's order. Password is the fourth: three fields in one panel with the mismatch check and
the existing successful-change notice. MCP access is the fifth: one Connected clients panel with a
count, search and OAuth/bearer chips; the connect card and other sections stay.

**Rulings recorded in the specs.** Scope chips keep no URL state; Rows and Cards both survive; the
directory pages use rows; ownership and role are each printed once; recency is relative with the
exact time in the title; Teams management stays in-page and folded; Jump reads the loaded shelf;
machine search matches observed reports, not saved intent; the notification unread query stays
server-side; the invite card stays above the people list; the role badge is hidden when the role
select is shown. No API, store, permission or dependency was added for the refresh.

**Not claimed.** No physical phone or tablet, no screen reader, and no Safari or Firefox for any of
the refreshed pages. Touch sizes were measured in an emulated coarse-pointer viewport. The Machines
search matches observed reports, not saved intent. All seven Admin sections are refreshed. All
Account list and form pages are refreshed: Sessions, API tokens, Profile, Password and MCP access;
Account Backups inherits the refreshed Backups panels, and Account Activity already had its own
count, filters and day groups. Skill cards now summarise a large fleet in one press, let an
unconfirmed machine be scanned before adding, and prepare a reviewed removal in Sync. A published
written skill appears once as its shelf card; **Your drafts** holds the unpublished ones, and the
shelf card's **Edit written skill** action returns to the editor. Unpublishing or deleting hides
that card while keeping its tags, notes and list membership for a later republish; the hosted
content and shelf row now share one lifecycle. The authoring list names the machines for every
skill. Machine timestamps follow the profile's zone and date order; a large
fleet sorts by attention and can be filtered by status. Skills and Sync use one glossary, Sync has
one place under Machines, and the authoring page is **Write a skill** with **Next: Put it on a
machine** after every save. The session picker also installs a private draft
through the same private-revision path, with no publish step.

**Next UI work:**
1. **Physical-device checks** for the refreshed pages: 320/390 touch, screen reader, Safari/Firefox.
   Do not claim these from the Chromium fixtures.

## Import into a running controller (4 November 2026)

Branch `backup-import-running`, on top of `backup-restore-checks`. `POST /api/v2/admin/backups/import` is now `multipart/form-data`
(options first, then `file`; header `X-Reet-Import: 1`), owner only, with the password in the body. The upload is streamed to a temp file and
bounded. `Service.StageImport` refuses a controller that holds data (`ErrNotEmpty`, `409`) unless `replace`, runs `prepare`, copies
the live database to `reet.db.before-import-<ts>` with `VacuumInto`, then stages it; `ApplyPendingImport` still swaps it in at the next
start. `reet backup import --server URL --token-file F FILE` is the matching command. Empty means the first owner and nothing else
(`holds` in `restore.go`; the topic tags and the server's own `reet-server` environment a new controller makes for itself are not counted). The old octet-stream plus `X-Copy-Password` form is gone.

## Careful whole-controller restore (4 November 2026)

Branch `backup-restore-checks`. `reet backup restore` checks everything before the swap, in order: integrity, schema version (the
refusal names both data formats), `--owner-email`, then prints the inventory (accounts by role, shelves, skills, lists, tags,
environments, connected machines, destinations). Without `--yes` it reports and changes nothing. Sessions and personal tokens now come
across by default; `--drop-sessions` and `--drop-tokens` end them, and `--keep-tokens` is an accepted no-op. Sealed secrets the
controller's key cannot open are cleared (a destination is also switched off) and listed with what to do. The choices are written to the
restored database's activity log as `backup-restored`. The staged import over the API shares `Service.prepare`, so it takes the same
checks and the query options `dropSessions`, `dropTokens` and `ownerEmail`. Next: an import command and a replace guard for a
controller that already holds data.

## Whole-controller restore and import (2 November 2026)

`reet backup restore` now adapts the copy before it replaces anything (`internal/backup/restore.go`, `internal/store/store_restore.go`):
browser sessions and reset links end, personal tokens and MCP connections are revoked unless `--keep-tokens`, the private-connection
identity is dropped, stored secrets the controller's key cannot open are named, and a copy stamped with a newer `PRAGMA user_version`
(`store.SchemaVersion`, stamped by `store.Open`) is refused. The owner can stage a copy on a running controller with
`POST /api/v2/admin/backups/import` (also `GET`, `DELETE`); `backup.ApplyPendingImport` swaps it in at the start of `serve`.
The import has an Admin → Backups control (`Backups.svelte`). Left for later: bumping `store.SchemaVersion`
when a release changes the schema incompatibly. See [docs/backups.md](docs/backups.md).

## Structured chats (SC1–SC5): resume here (2 November 2026)

**Read first:** [docs/structured-chat-plan.md](docs/structured-chat-plan.md) (the plan and every progress note),
[docs/device-checks.md](docs/device-checks.md) (what needs a real device or login) and
[docs/research/structured-adapters/probe-report.md](docs/research/structured-adapters/probe-report.md) (what each adapter was
measured to do). This section is the position and the order of what is left.

**Where it stands.** OpenCode, Cline and Google Antigravity chats are offered to every account. Claude Code and Codex are built end
to end but off by default: each account turns them on in Settings > Agents, a stored per-account setting
(`user_profiles.chat_agents`) that the controller applies when it reads a machine's capabilities (`internal/terminalproto/acp_agents.go`:
`ChatDefaultOn`, `WithoutChats`), and a connector reports them as available only to a controller that advertises `chat-gate-v1`. The
environment switches were removed. Claude Code is feature-complete for a chat (history, reconnect, model and thinking level,
images, a terminal launch with resume, YOLO and effort, a missing-adapter hint) and waits on the owner's signed-in Run B before it
is on by default. Descriptors are per agent
(`internal/terminalproto/acp_agents.go`); the controller binds each to its reviewed adapter; the browser offers a chat only from
a descriptor that says `available`.

**Main state (2 November 2026):** #409 merged as `c0b0c49`; #414 (connector-restart recovery) as `f64b407`; the UI refresh through
#415 as `1e626ff`. The earlier documentation-only #401 merged as `d5a4e80`. The active bounded SC5 slice is now the measured descriptor
option contract on branch `feat/descriptor-chat-options`; it adds no control, login gate or process behaviour. Older notes below describe
the slices' publication state at the time, not current `main`.

**On main (merged):**
- SC1–SC2: per-agent descriptors; probes of the Claude and Codex adapters (report above).
- SC3: typed agent questions, machine core to chat card; the form capability (`elicitation.form`) and Claude chats stay off until
  the touch and screen-reader checks (device-checks section 1) are recorded.
- SC4 Claude: `ClaudeCandidate` profile (`internal/acpagent/claude.go`), descriptor reports approvals and the model switch only, browser
  offers it from an available descriptor, credential-check script `scripts/check-claude-credentials.py`.
- SC5 audit fixes: force-stop of an agent that ignores cancel (`ForceStopGrace`), late permission and question callbacks refused
  (`turnAcceptsRequests`), neutral "this chat has ended" wording, tools left running when a turn stops.
- SC4 Codex: profile `CodexCandidate` (`codex.go`, floor 1.13.1, requires and starts in `read-only` via `Spec.StartMode`, only
  `read-only` and `agent` choosable via `Spec.ChoosableModes`, Stop ends the chat when a command is running via
  `EndChatOnStopWhileToolRuns`); `Session.SetMode` confirms a switch with the agent and never reports the old mode after an
  unanswered one (it clears the mode and tries once to restore); feature `acp-agent-mode-v1` (descriptor `modes`,
  `Capabilities.ACPAgentMode`, message `acp_agent_mode`, broker `SetAgentMode`, `POST /api/v2/terminal/sessions/{id}/chat/agent-mode`);
  the confirmed mode is stored on the session (`terminal_sessions.agent_mode`, JSON `agentMode`; empty means unknown), set to the
  first listed mode at create, to the new mode on a confirmed switch, and cleared on an unconfirmed or lost one.
- Probe scripts: `scripts/probe-codex-turn.py`, `scripts/probe-codex-start.py` (no model call; `--auth none|api-key|chat-gpt`,
  `--codex-home`) and `scripts/probe-agent-options.py` (initialize and `session/new` only; measured option values, no prompt).

**Merged on 2 November:** #375, "Offer Codex chats in the browser with an approvals-mode control that says what each mode means", and
#384, "Show the agent's reason on an approval card and say a Codex reject ends the turn". #375 makes the controller refuse a message
with a 409 (`ErrACPModeUnknown`, `PromptChat`) while an agent that offers modes is in an unknown mode, makes `codex` a reviewed chat
tool in the browser, and adds the Approvals control (`web/src/lib/terminal/AgentModeControl.svelte`, wired in `ChatPane.svelte`: the
wording for each mode, Send held while the mode is unknown, a YOLO-aware sentence), the Start and Home dialog copy, and
`web/tests/chat-codex.e2e.mjs`. #384 puts the model's justification (`_meta.permission.description`, carried in the permission event's
existing `text` field, so no wire change) on the approval card under a "Why {agent} asks" label, makes Codex's reject button read
"Deny and end the turn", and adds `TestChatHostSwitchesAgentMode` for the connector's mode switch. Both merged with every check green,
`phone-zoom` included.

**`phone-zoom` on main:** green again. #380 ("Let the skill card's sync line wrap long host names") merged first, then #375 took `main`
in and dropped its own copy of that rule, so `SkillCard.svelte` matches `main`. The failure had been the Skills page laying out 355 px
wide at 320 px since #370.

**Decisions already made (do not re-ask):**
- Codex starts in `read-only` (the only mode that can ask before a command leaves the sandbox); `agent` is chosen per chat, never the
  default; `agent-full-access` is never offered. The wording is in the plan and `AgentModeControl.svelte`.
- No "allow always" for Codex (its `accept_execpolicy_amendment` writes a persistent rule); allow once or reject only.
- Stop while a Codex command runs ends the whole chat (cancel leaves the command alive).
- A silent sandbox denial or an `agent`-mode refusal is not visible in the adapter's events; accepted, hence the labelled mode.
- Codex sign-in: reet never calls `authenticate` (`chat-gpt` opens a browser on the machine). The person signs in to Codex
  themselves; the Codex profile will set `CODEX_HOME` to their own Codex folder and pass nothing else from their real home; a failed
  start says "Sign in to Codex on that machine first"; the adapter's `_auth/status_update` label is shown in the chat, never the email.
- The current mode is stored on the session, not sent only as an event.

**Codex login access merged (2 November, [#391](https://github.com/eyupio/reet.sh/pull/391)):** the owner approved building Codex login access from source evidence and keeping section 3's signed-in Run B as
final verification. The profile now routes `CODEX_HOME` to the connector's own setting or original `HOME/.codex`, keeps replacement `HOME`, drops XDG
home directories for Codex, never calls `authenticate`, maps the authentication-required start error and forwards only the bounded status label as a notice.
Neither feature switch is retired. See the new progress note in the plan and source checkpoint in the probe report; test and publication results belong
in that PR. The earlier requirement to wait for section 3 before coding is superseded for this slice. Run B remains the next owner check; if it still
says "Authentication required", stop and ask the owner which way to go. No hardware or genuine-login check has been completed here.

**Owner checks, still unverified:** the owner deferred section 3 signed-in Run B ("skip this, assume it works for now"). This is permission to
continue independent SC5 work, not evidence of a real login. Do not re-ask while it remains deferred. When resumed, signed-in Run B runs first,
then Run A's follow-up; if Run B still reports "Authentication required", stop and ask which direction to take. Next are the signed-in runs
(section 2 Run B for Claude and section 3 Run B for Codex), then section 1's physical touch and screen-reader checks. Claude Code and Codex chats are not on by default, and
`elicitation.form` is not turned on, before the required evidence is recorded. Record each result in the ledger at the top of
[docs/device-checks.md](docs/device-checks.md#recorded-results) with its date and the PR that carries it. Reet never calls `authenticate`.

**Recorded 4 November 2026 (logged-out runs, pinned adapters):** Claude `claude-agent-acp@0.99.1` advertises no login methods, creates a session
without a login and refuses the first prompt with `-32000 Authentication required` (`authenticatedHint: false`); `TestRealClaudeStart` passes against
that adapter with no model call; Codex `codex-acp@1.13.1` refuses `session/new` before any authenticate and reports
`{kind: none, label: "Not logged in"}`. Because Claude answers at the prompt rather than at session/new, the prompt path now maps that refusal to
`acpagent.ErrClaudeLogin` ("Sign in to Claude on that machine first.") the way session/new already did for Codex, so a signed-out machine says what
to do instead of looking broken; the chat shows it as a sign-in step, not an error to diagnose (merged as `c600e114`, and the
status now carries `code: "login"` so the panel can say so). Signed-in runs for both agents and every phone/reader item are still open.

**Delivered SC5 slices:**
- #392: live model controls and lookups follow each available agent descriptor. Merged; bot review was blocked by account quota and its terminal CI
  failure was reproduced on main and left outside scope.
- #396 (`35225eb`): pending approvals/current status survive full/stalled reconnect replay to the surviving connector. All implementation CI passed
  on `ef2a418`; Linux native services used its single allowed retry. Codex review quota was exhausted: no submitted review or finding threads.
- #402 (`a78ffa6`): delivered typed-question cards recover with original tokens, schema and deadline; detached resolutions retire state, older peers
  receive no question frames and pending questions are cancelled. New questions without a negotiated recipient remain cancelled. All CI passed on
  `7dec2cc`; Codex review completed without findings. The native fixture now waits for foreground startup readiness before status polling, which
  otherwise can steal startup's enrollment lock; private failure logs are retained.
- #406 (`6973f08`): live model replies must confirm applied state. OpenCode alone permits omitted-list live acknowledgements; malformed/ambiguous or
  lost replies clear process/stored model and retained status detail. Explicit protocol refusals keep the prior model; later confirmation recovers.
  All five CI workflows passed on final head `59f896f`, without retries; Codex review completed without findings
  ([receipt](https://github.com/eyupio/reet.sh/pull/406#issuecomment-6585858018)). No genuine-login/device gate was closed.
- #409 (`c0b0c49`): explicit startup model confirmation before admission or the opening prompt. Its full validation is below.
- #414 (`f64b407`): structured chats recover after a connector restart or reboot from the persisted live-chat registry. Its startup resume race is fixed in #416.

**Merged SC5 slice — startup model confirmation ([#409](https://github.com/eyupio/reet.sh/pull/409), `c0b0c49`):** an explicitly chosen model must have exactly one valid matching read-back from `session/new`
or `session/load` before admission or the opening prompt. Startup has no omitted-list exception, even for OpenCode. Agent-default launches preserve
existing behaviour. Definite failed chat creates clear unconfirmed model metadata but retain the operation fingerprint/refusal for replay;
ordinary terminals and transport uncertainty are unchanged. A close callback before admission cannot announce a premature exited session; the admitted
process's Done watcher still observes an exit racing startup. No extra model-setting RPC, wire field, database column or adapter capability is added.

New fake-child/controller regressions failed before these fixes and pass under race. Real pinned OpenCode 1.19.5 and Claude adapter 0.99.1
(SDK 0.3.312) were probed on Linux x86-64 with isolated homes, without prompts or `authenticate`. OpenCode new/load reported `opencode/big-pickle`;
Claude new/load reported `default`. Requesting `anthropic/claude-sonnet-4-5` through OpenCode's private config still read back `opencode/big-pickle`:
config alone did not establish that selection. Enhanced opt-in Go tests passed for explicit matching OpenCode new/load and Claude new. These are
startup/profile checks, not real-login or model-turn evidence. Exact versions, reproduction and limits are in the probe report.

**#409 publication and validation:** merged as `c0b0c49`. The checks below describe implementation head `bd10520`; later heads have their own CI. All five CI workflows passed on that head, without retries:
[full tests/image](https://github.com/eyupio/reet.sh/actions/runs/41513691712) (full Go vet/race, web and MCP),
[terminal browser and both phone jobs](https://github.com/eyupio/reet.sh/actions/runs/41513691713),
[macOS/Linux native services and minimum tmux](https://github.com/eyupio/reet.sh/actions/runs/41513691764),
[private connection](https://github.com/eyupio/reet.sh/actions/runs/41513691722) and
[binary release](https://github.com/eyupio/reet.sh/actions/runs/41513691723). Local affected-package race checks, Go vet, API tests, web check/build
and all 16 MCP tests passed. The five known model-discovery/orphan container failures were reproduced on main `6973f08` before local exclusions;
CI passed the full suite without those exclusions.

Codex bot review did **not** complete before the merge: the account's code-review quota was exhausted
([bot response](https://github.com/eyupio/reet.sh/pull/409#issuecomment-6586917713)); there are no submitted reviews or inline finding threads.
The bot summary's finding flags do not establish findings when the review failed. This handoff-only follow-up records the completed implementation
checks; any new-head CI results belong in the PR. Do not represent the earlier head's green checks as checks of a later documentation commit.
The slice is merged; no deployment is claimed. Login/device checks remain deferred and unverified; both experimental switches and form capability stay off.

**Resume startup race fixed in #416 (this PR):** `newChatSupervisor` no longer starts persisted-chat adoption before a caller can install its
process hooks. `NewChatSupervisor` and the connection-scoped fallback call `startResume()` once; the two resume tests call it after their fakes.
The race in #415's `test` job reproduced on unchanged main and is fixed here. See the PR for the package race and vet results.

**Delivered SC5 slice — connector-restart recovery ([#414](https://github.com/eyupio/reet.sh/pull/414), merged `f64b407`):** a recorded chat
is reopened with `session/load` under the same reet session id after the connector restarts, the replay is dropped because the controller
already has the transcript, and the controller stores the same bounded transcript in SQLite. Archiving drops the record and transcript;
records older than seven days are dropped, and a failed reopen is attempted at most three times. Detached per-session workers and turn
generations across a native process restart remain future work; a turn in flight when the connector stopped is reported as stopped.

**Active SC5 slice — measured descriptor option contract (`feat/descriptor-chat-options`):** a new negotiated descriptor field `options`
describes the non-permission selectable options a pinned adapter was measured to offer. It is bounded, ordered, distinct and descriptive
only: no switch, startup option, UI control, database column or adapter capability is added beyond `acp-agent-options-v1`. The existing
`modes` list remains the permission-sensitive switch list and `model` the dynamic catalogue, so mode and model are not duplicated. Measured
values are Claude ACP 0.99.1 effort `default`/`low`/`medium`/`high`/`xhigh`/`max`, `fast` and `ultracode` (`on`/`off`); Codex ACP 1.13.1
`reasoning_effort` `low` through `ultra`, `fast-mode` (`off`/`on`) and `collaboration_mode` (`default`/`plan`). OpenCode 1.19.5 reported no
other option. Values can depend on the current model, so the contract says what may be offered, not what is valid now. The probe sent no
prompt, made no model call and used no real credential; Codex used a synthetic api-key placeholder for `session/new`.

**Next independent work after this slice:** a bounded effort control can use this contract, reading the live option list from the adapter and
confirming a switch before reporting it. Detached workers and turn generations across a native process restart remain future work.

**Working rules the owner set (keep following them):**
- One item at a time: build it test-first, open a **draft** PR, watch it, and **merge only when the owner says so** (they often mark
  PRs ready or merge them themselves). Then take the next item.
- Decisions for the owner: one question at a time, recommendation first. Do not re-ask decisions recorded here.
- Keep the requested commit trailer `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`. Use truthful Codex generation attribution;
  do not invent or request a Claude session URL. PR bodies use What and why / How I checked it / Checklist and end with
  `🤖 Generated with [Codex](https://openai.com/codex/)`. GitHub comments end with `---` and `_Generated by [Codex](https://openai.com/codex/)_`.
  Never name or link an external reference project in a commit or PR.
- Every ready PR gets a Codex bot review. Treat each finding as a claim to verify against the code: fix what is real with a test that
  fails without the fix, reply on the thread, resolve it. Merge `main` into a branch for conflicts or a fix that landed after the branch
  was cut; never rewrite history. A failing CI job is re-run at most once; a second failure is real, or is shown to be red on `main` too.
- The Codex review sometimes finds a second-order effect of an earlier fix (an unconfirmed mode, a lost reply, YOLO, a held message).
  The pattern that held: when something may have been applied but was not confirmed, record "unknown" and refuse to proceed, never
  claim the old state.

**Running things here:**
- In this persistent workspace activate the cached toolchain first: `. /workspace/.reet-tools/activate.sh` (Go 1.27.1).
  The system `go` is unrelated. Avoid reinstalling available tools; web/MCP dependencies are cached.
- `go test ./internal/api` takes about two minutes without `-race`; with `-race` it can exceed a ten-minute timeout in a small
  container (CI allows sixty). Run the other Go packages with `-race`.
- Browser tests: `npm --prefix web run build`, then `npm --prefix web run preview -- --host 127.0.0.1 --port 4173`, then
  `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/opt/pw-browsers/chromium REET_UI_URL=http://127.0.0.1:4173 node web/tests/<name>.e2e.mjs`.
  The agent chooser in the Start dialog is a dropdown (`getByRole('combobox', { name: 'Agent' }).selectOption`), not radios.
- The probe scripts are in `scripts/`, but the adapters they run are not in the repository: install the pinned versions with
  `npm install --ignore-scripts` as the probe report says.
- A fresh container may have no Go toolchain: `curl -sSL https://go.dev/dl/go1.27.1.linux-amd64.tar.gz | tar -xz -C ~/gotool` and put
  `~/gotool/go/bin` on `PATH`. The first `go build ./...` downloads modules and takes minutes. `TestCloseKillsOrphanedGroupMembers`
  (internal/acpagent) and the `TestModels*` family (internal/terminalagent) fail in this container on `main` too (no OpenCode binary,
  no process-group control), so do not read them as a regression; check the same test on `main` before chasing one.
- Creating a branch by pushing history containing workflow changes can fail with the token's missing `workflows` permission.
  Create the remote branch from current main through GitHub's branch tool first, then push updates. Use command-scoped
  `git -c 'credential.helper=!gh auth git-credential' fetch/push` if cached Git HTTPS auth fails. Never print credentials.
- Never use `pkill -f '<pattern>'` or `grep` for a pattern that also appears in your own command line: it ends the shell.

## SR1 — Self-healing automatic sync (2 November 2026)

**Scope:** the controller↔enrolled-agent sync job protocol; no new remote shell or arbitrary
command surface. Branch `fix/sync-self-heal`, based on main `f051948`.

**Problem:** a preview approved for apply could find that local files, the target revision or the
saved selection had changed. The machine refused correctly, but the controller failed the whole
automatic run, so later scopes waited behind a card the person had to press. The retry only
re-queued the one scope. That is the failure shown in Sync activity as **The plan changed before it
ran**.

**Fix:** the machine now answers the recovery direction, not just receives jobs. `internal/scan`
classifies the stale-plan refusals as recoverable and reports the new wire status `replan`; no
files were written. `internal/store` accepts `replan` only from preview/apply/automatic jobs,
records the attempt honestly as failed, and — when the job belongs to a running automatic run —
queues a fresh automatic job at the same `sync_run_jobs.position`, leaving the cursor untouched so
the run continues into later scopes. The controller-side "desired revision changed during apply"
check takes the same path. A scope gets at most three attempts per run (`syncRunReplanLimit`); after
that the run fails and the existing **Fix and try again** starts a fresh plan. Manual previews and
applies never retry themselves. Results stay idempotent through `sync_receipts`, and the agent's
durable `.result` file still resends rather than replaying filesystem work.

**Checks:** `gofmt`, `go vet ./internal/store ./internal/scan`, `go build ./...`, and
`go test ./internal/store ./internal/scan` (including
`TestSyncRunReplansStalePlanAndKeepsGoing`, `TestSyncRunStopsAfterReplanLimit`,
`TestSyncRefusesAChangedApprovedPlan`, `TestOrdinarySyncFailuresAreNotReplanned`).

**Limits:** this does not add a general controller-to-machine command executor; the existing
allowlisted typed jobs and local opt-ins still gate every write. A run interrupted by a disconnect
or timeout is still failed by the reaper rather than automatically re-planned, because partial
writes cannot be proven absent.

**Next bounded task:** decide whether a reaped automatic run should requeue a fresh preview (safe,
read-only) and then continue, with the same attempt budget; and add an end-to-end agent fixture that
drifts local files between preview and apply to prove the whole loop over HTTP.

## Structured chat recovery across a connector restart or reboot (2 November 2026)

**Scope:** the machine's structured-chat supervisor and the controller's chat transcript; no change
to ACP, permissions or the launch gates. Uncommitted working-tree changes on `ux/boot-shell`.

**Problem:** a machine agent upgrade, a crash or a reboot ended every Gaffer chat, and the browser was
told its live history was gone. OpenCode keeps the conversation on disk, but nothing reopened it.

**Fix:** the connector now records each live Gaffer chat's `ses_…` id, folder, model, approval mode
and folder trust in a private `structured-chats.json` beside its terminal state. On the next start it
reopens each one with `session/load` under the same reet session id and drops the replay, because the
controller already holds the transcript; the controller then adopts the row as running instead of
marking it lost. Ending, archiving or switching a chat to the OpenCode TUI removes the record, folder
trust and YOLO are re-checked at reopen, records older than seven days are dropped, and a failed
reopen is tried at most three times. The controller now also stores the same bounded transcript (1,000
events / 256 KiB per session) in SQLite, restored on subscribe, so its own restart or a reboot keeps
the conversation rather than only the row. A lost row may return to running only for a structured
chat; a plain shell is never resurrected. The stale "held in memory and is gone" copy is corrected.

**Checks:** `gofmt`, `go build ./...`, `go test ./internal/terminalagent ./internal/terminalbroker
./internal/store ./internal/terminalproto ./internal/api`, and the chat/agent-launch browser unit
tests. No real machine was rebooted; the reopen path is covered by a fake ACP child and a registry
fixture.

**Limits:** a turn that was in flight when the connector stopped is reported as stopped, not resumed.
Detached per-session workers, turn generations across a native restart, and a live-machine reboot
check remain future work. Records and transcripts are stored in the controller database and the
machine's private state directory; the agent-side record holds metadata only.

## Gaffer interface and Eli compatibility probe (1 November 2026)

Branch `feat/gaffer-interface`, based on main `9b8bc17`. The native browser
interface is branded Gaffer, with shared accessible HTML explanations; stored
`reet`/`chat` choices, ACP sessions and OpenCode TUI remain compatible. A bounded
Eli provider/conformance probe and licence/dependency review contribute to KZ5A;
no coordinator or new adapter is enabled. See [the companion update](docs/work-orchestration-handoff.md#gaffer-branding-and-eli-reuse-1-november-2026)
and [reuse decisions](docs/research/eli-reuse.md). KZ0 and KZ5A remain incomplete;
the exact remaining decisions and future stage gates stay in those documents.
Publication and validation are recorded in the PR; no merge or deployment is implied.

## Durable work, Eli and Kanzo follow-on handoff (1 November 2026)

The staged [work orchestration handoff](docs/work-orchestration-handoff.md) is a
companion plan for the user's durable-work direction: steps assigned across
agents, models and machines, with revision-bound independent reviews and an
Eli-style coordinator informed by Zoomies. It owns stages KZ0–KZ12, including
independent Kanzo-inspired documentation/resource/refresh improvements. This file
remains the entry point for standing rules and the active task. The core stages
fit richer run orchestration; the current structured-chat sequence retains its
recorded priority and capability gates until the active task is explicitly changed.
No KZ runtime implementation stage is complete; the branding follow-up records
a bounded KZ5A probe. The 5 November session-companion MVP now owns the active
first-release direction, with GM0 as its next design slice. The broader KZ
workstream retains its KZ0 reconciliation and later dependencies. Record the one
selected slice here and keep the companion's active section authoritative.

**Docs tidy-up (27 October 2026).** Finished plans, specs, stage documents and audits were removed
on 27 October 2026; they live in git history before commit `bd9e56c` (for example
`docs/superpowers/plans|specs/2026-10-2[145]*`, `docs/agent-setup-plan.md`, `remote-machine-*` and
`docs/remote-machine-stage*.md`, and the root files `FEATURES_AUDIT.md`, `GEO-ANALYSIS.md` and `UI_AUDIT.md`). Older per-feature delivery notes (single sign-on PR logs, terminal
sudo repair, roadmap checkpoints, terminal polish, terminal-first workspace requirements, the
implemented baseline, S/A/W/L task lists, start-agent-session protocol notes, Projects PR 1 and
delivered, phone terminal/zoom/dialog notes, the ready-to-use continuation brief) are in
[handoff-history.md](handoff-history.md), under "Moved from handoff.md on 27 October 2026".

## Loading: compressed assets, one progress bar, skeletons everywhere (2 November 2026)

**Scope:** how every page arrives and how it waits; no API, data or copy change. Branch
`claude/optimistic-volta-ehhex6`.

**Problem:** filmed on a slow connection (1.6 Mbit/s, 150 ms), a first load painted nothing for about five
seconds: the built bundle is 1.3 MB of JavaScript and 440 KB of CSS and the server sent both uncompressed,
with no cache header, so the boot shell could not appear until the render-blocking stylesheet had arrived
and every visit paid for it again. Once the app was up, each page showed a bare `Loading…` sentence (or
nothing at all) while its sections popped in one by one, and a request made from a page gave no sign it was
in flight.

**Fix:** `web/compress.go` gzips each built file once, on first request, and serves the compressed body to a
browser that takes it (q-values honoured, `Vary` said either way, fonts and images left alone); the hashed
files under `/assets/` are `immutable` for a year and `index.html` stays `no-cache` but goes compressed too.
The main chunk is now 383 KB on the wire and the page 6 KB, so the boot shell paints in the first half
second on the same link. In the app, `lib/inflight.ts` counts every request `api.ts` makes and
`shell/ProgressBar.svelte` draws one thin bar along the top edge while anything is outstanding, after a
200 ms grace and for at least 400 ms, so a fast answer shows nothing and a burst reads as one wait.
`lib/Skeleton.svelte` is the one loading affordance for content (lines, rows or cards, the shape of what is
coming, with a visually hidden label for a screen reader) and `lib/LoadingBoundary.svelte` renders the
waiting, failed, empty and content states in one place; every bare `Loading…` line (twenty-two of them,
including the machine list the UI audit's H9 named) now shows a skeleton, and the two ad hoc skeletons
(the shelf, the machine grid) use the same component. A button that is doing what it was asked carries
`aria-busy`, which `app.css` draws as a small turning ring beside its label; spinners live there and nowhere
else.

**Checks:** `go test ./web/` gained tests for the compressed and cached assets, the plain body for a client
that refuses gzip, the uncompressed fonts and the compressed page; `lib/inflight.test.mjs` checks the two
delays against a fake clock and that `api.ts` reports to it; `npm run check` (unit tests and svelte-check,
0 errors) and `npm run build` pass; `first-run`, `first-skills`, `navigation`, `home`, `empty-states` and
`first-machine` browser suites pass against the built preview. Filmed again on the same throttled
connection, every route paints within 400 ms and the machines page shows card skeletons inside its own
frame until the inventory lands.

## Boot shell: no crawler page, no bare loading screen (1 November 2026)

**Scope:** the first paint of every route, signed in and signed out; no API, data or copy change beyond the
crawler page. Uncommitted working-tree changes.

**Problem:** on a slow connection the browser painted the static crawler copy of the landing page (the older
"One control room. For your AI fleet." layout, no header, system fonts) for as long as the bundle took, then
snapped to the real Welcome page. App routes showed a bare `Loading…` line while the first auth answer was in
flight, and opening a session left an empty page with one sentence while the terminal chunk downloaded.

**Fix:** `index.html` now carries three small theme-aware boot shells (landing hero, entry card, app page).
The head script marks `data-js` before the body is parsed, so a browser hides the crawler copy at once and
shows the shell in the same frame; a crawler that runs no script still gets the readable copy, now updated to
the current Welcome words. `embed.go` serves only the shell for the route, names it on `<html data-boot>`, and
therefore handles a cookie-signed-in visitor on `/` without the browser having to know the session. A static
preview picks the same variant from the path. `App.svelte` sets `data-ready` after the first auth answer, so
the shell fades once the real page is behind it and `main.ts` removes it; public routes that need nothing from
the auth answer (community, shared lists, join, reset) dismiss it as soon as they draw, even if that answer
stalls. A late bundle shows a "taking longer" line with a reload link after ten seconds; the line is a live
region outside the aria-hidden shell, so a screen reader receives it. The pre-auth branch and the terminal
chunk's gap now use skeletons (`BootSkeleton.svelte`, and a workspace-shaped card in `Machines.svelte`) instead
of sentences. The two faces the first screen uses are preloaded; reduced motion gets no fade.

**Also fixed (found by CI on the merge with main):** the Gaffer help tooltip is a read-only explanation but its
popover was in the top layer and intercepted pointer events, so on a narrow layout it could cover the launch
bar's own listbox and block an option. It is now `pointer-events: none`; `launch.e2e.mjs` asserts that and that
the slot, list and options stay clickable.

**Checks:** `npm run build`, `npm run check` (unit tests and svelte-check, 0 errors/warnings), `go vet` and
`go test ./web/` (including the new route-to-shell test), and `first-run.e2e.mjs` gained a throttled-bundle
case: the crawler copy stays hidden, only the landing shell is visible, it is removed after auth, a
no-JavaScript context still reads the landing copy, the recovery line is a live region outside the hidden
shell, and `/reset` draws before a stalled auth answer. `public-workspace.e2e.mjs` passes at light/dark ×
1440/390/320. Screenshots at 1280 and 390 in both themes, and a delayed terminal chunk, confirmed the shells
match the pages they hand over to.

## Welcome page: one command on any machine (1 November 2026)

**Scope:** the signed-out Welcome page and the entry card beside sign-in, set-up and sign-up; no API or
signed-in change. Branch `welcome-one-command-any-machine`.

The hero now leads with the moment a newcomer can picture, one command on any machine, and the lede
says what reet.sh keeps on every machine (agents, skills and setups: synced, backed up, restorable).
The illustration is the product's own enrolment: a Machines list with three machines in sync over a
terminal running `enrollCommands()` from `machineSetup.ts` line for line, so it cannot drift from what
Add a machine gives. The three steps are enrol a machine, agents, skills and setups stay in sync, put
your agents to work. The entry card's headline is "Your whole AI setup. On every machine." The voice,
the rose and the palette are unchanged, and the page still says self-hosted, never a service.
`public-workspace.e2e.mjs` and `first-run.e2e.mjs` hold the words and the illustration's commands.
The public header also carries the colour choice (`ThemePicker.svelte`: three icons, light, dark, then
match this device, the chosen one ringed) between the links and Sign in, in the links row at every width
so a phone's set-up form still fits its first screen; it applies through `applyTheme`, is kept in this
browser, and a signed-in profile's saved choice still wins. The signed-in account menu is unchanged. The
sign-in, sign-up and set-up leads say machines are enrolled and agents, skills and setups stay in sync.

## SC2 — Claude lifecycle probes and ID/diff hardening (31 October 2026)

**Scope:** follow-up to PR #301; no adapter is enabled. Branch `ccr-4cc5980a-dt7hki`.

**Added:** `scripts/probe-claude-lifecycle.py` (trust, cancel, load, model, subagent);
adapter-aware native ID validation (`nativeIDMatches`: `ses_` for OpenCode, canonical
lower-case UUID for Claude/Codex, still gated by `ReviewedChatTool`); and `typedDiff` in
`internal/acpagent/session.go`, which turns Claude's `{type:"diff"}` content into the
existing bounded 10 KiB diff and reads `rawInput.file_path`. A missing or unusable diff
still leaves an edit card unapprovable. Results are in the
[probe report](docs/research/structured-adapters/probe-report.md).

**Findings that block enabling Claude:** a repository's own hooks run without an approval
card; leader exit did not retire the whole process group; `session/load` ignores `cwd`;
an unknown model value is silently reset; subagent calls arrive as kind `think`.

**Checks:** `go vet` and `go test` for `internal/acpagent` and `internal/terminalproto`,
gofmt, `py_compile`, and a real run of every scenario against the pinned adapter with
the loopback stub. The full race suite, web checks and browser fixtures were not re-run:
no web or protocol-shape change. No real provider, phone or other OS.

**Hooks gate (local, not pushed):** `Spec.SessionMeta` is sent as `_meta` on session/new and
session/load; `ClaudeSessionMeta` pins `settingSources` to `["user"]`, verified to stop a
repository hook while approvals still appear. No Claude profile is registered yet.

**Also local:** `retireGroup`/`GroupRetired` (group SIGKILL, confirmed via /proc, zombie-aware) and
the `think` subagent card (`subagentSummary`). Tests: group_test.go, session_test.go.

**Also local:** `Session.SetModel` reads the model option back from `session/set_config_option`
and returns `ErrModel` (recording the real value) when the agent reports a different one; a
response without an option list keeps the old behaviour. Test: `TestSetModelReportsAppliedState`.

**Subagents and Codex (local):** nested subagent calls raise their own approval with a
`parentToolUseId`; reet labels them "Subagent:". Codex probe (`scripts/probe-codex-turn.py`)
showed no per-command approval request in any mode and a provider error disguised as
`end_turn`; Codex stays disabled. See the probe report.

**Environment (local):** `acpagent.ClaudeCandidate` (unregistered) holds the measured launch profile:
adapter command, name/version floor, `default` mode required, credential-only allowlist, no
provider redirects, background traffic off, `ClaudeSessionMeta`. Not registered.

**Next unchecked task:** Claude missing/real-credential behaviour and registering the profile behind the
gates (descriptor + launch + history); for Codex, re-run with a working
sandbox to see whether any approval request exists, and cancel on a running command; then SC3.

## SC3 — Typed agent questions, machine core (31 October 2026)

**Scope:** `internal/acpagent` only; no wire, API or UI change and no agent enabled. See the SC3 progress
note in [the plan](docs/structured-chat-plan.md).

**Added:** a normalized `Question` model (text, single select, multi select), a strict `elicitation/create`
parser that refuses unsupported forms whole, an authoritative answer validator (`Question.Validate`,
`DecodeAnswers`), and session handling with single-use IDs, deadlines, a pending cap and cancellation on turn
cancel, close and exit. `Options.Questions` is off in every launch, `initialize` does not advertise
`elicitation.form`, and the connector drops `question*` events.

**Checks:** `go vet`, `gofmt`, `go build ./...` and `go test -race` for `acpagent`, `terminalagent` and
`terminalproto`; two deliberate mutations (any choice accepted, answered question not consumed) failed the
tests. No browser, phone, real provider or live adapter was used.

**Next unchecked task:** private-protocol messages and negotiation, the controller answer route with
account checks, then the chat card and Needs-you summary; request-scoped (pre-session) questions and
old-peer/concurrent-tab/phone fixtures. Do not advertise the form capability before those.

## SC3 — Typed agent questions, machine core (31 October 2026)

**Scope:** `internal/acpagent` only; no wire, API or UI change and no agent enabled. See the SC3 progress
note in [the plan](docs/structured-chat-plan.md).

**Added:** a normalized `Question` model (text, single select, multi select), a strict `elicitation/create`
parser that refuses unsupported forms whole, an authoritative answer validator (`Question.Validate`,
`DecodeAnswers`), and session handling with single-use IDs, deadlines, a pending cap and cancellation on turn
cancel, close and exit. `Options.Questions` is off in every launch, `initialize` does not advertise
`elicitation.form`, and the connector drops `question*` events.

**Checks:** `go vet`, `gofmt`, `go build ./...` and `go test -race` for `acpagent`, `terminalagent` and
`terminalproto`; two deliberate mutations (any choice accepted, answered question not consumed) failed the
tests. No browser, phone, real provider or live adapter was used.

**Wire stage:** `acp-question-v1`, the
question events and `acp_question_answer`, `question_invalid`, connector routing, `AnswerQuestionChat` and
the `/chat/questions/{id}` route. Still off by default; see the plan.

**Chat card:** the browser card for typed questions, with unit and browser tests; see `docs/ui-verification.md`.

**Next unchecked task:** SC3 code and fixtures are done (startup questions are cancelled with their own notice; holding them is
deferred). Left: the touch-device and screen-reader checks on a real device. Do not advertise the form capability before those.

## SC4 — Claude wired dark (1 November 2026)

Claude is registered but dark: `REET_EXPERIMENTAL_CLAUDE_CHAT=1` on a machine's connector makes it report Claude as available
(approvals and the model switch only; no history, images or questions), the controller binds each descriptor to its reviewed
adapter, and a descriptor-less peer is still OpenCode-only.

**Superseded:** the browser now offers Claude and Codex from an available descriptor, and the next tasks are listed in
"Structured chats (SC1–SC5): resume here" at the top of this file.

## SC2 — Structured adapter probe checkpoint (31 October 2026)

**Scope:** separate probe PR targeting main; no new runtime adapter is enabled.
Branch `feat/structured-adapter-probes`, following the merged
[SC1 PR #300](https://github.com/eyupio/reet.sh/pull/300). See
[the probe report](docs/research/structured-adapters/probe-report.md) and its
narrowed protocol fixtures. This is partial SC2 evidence, not completion of the
first additional-agent integration.

**Observed:** Claude adapter 0.99.1 / SDK 0.3.312 / native Claude Code 2.1.312,
and Codex adapter 1.13.1 / CLI 0.156.1, on Linux x86-64 with Node 22.23.4.
Adapter tarball SHA-512 values match recorded registry metadata. Both real
adapters initialized and created UUID-style sessions with fs/terminal delegation
disabled; Codex needed explicit synthetic-key ACP authentication. Claude Bash
and Write allow/deny controlled actual marker-file effects against a local stub;
form negotiation enabled AskUserQuestion and a typed answer completed the turn.
Write's typed content diff needs normalization before reet can review/allow it.

**Checks:** runnable standard-library Python probes, syntax/formatting and
recorded wire fixtures; actual candidate process versions and stub-backed
permission/question assertions. SC1's complete Go race suite, vet, web
check/build, MCP and browser checks passed separately. No live provider, real
credentials, physical phone or cross-OS verification was used here.

**Next unchecked task:** finish Claude's trust/override, cancellation/process
retirement, history/load, model and subagent probes; then add adapter-specific
UUID validation and bounded edit normalization. Keep unverified candidates and
form capability disabled. Codex tool/approval/provider/error paths need their own
stub probes. Read `scripts/probe-structured-adapters.py`,
`scripts/probe-claude-turn.py`, the report, `internal/acpagent/session.go` and
`internal/terminalproto/acp_agents.go`.

**Publication:** [PR #301](https://github.com/eyupio/reet.sh/pull/301) records this
checkpoint against main, following SC1's verified merge `e902b869`.
Its canonical PR record carries the current merge status; this checkpoint does
not enable an adapter or perform a release/deployment. Current main was merged
into the probe branch to resolve the handoff conflict after SC1 was squash-merged.
The resulting diff contains only the five SC2 report/fixture/helper/handoff files.
Syntax, fixture, link and whitespace checks passed after resolution; CI on the
resolved head must pass before merging. Preserve all remaining integration gates.

## SC1 — Per-agent structured chat capabilities (31 October 2026)

**Task ID and scope:** SC1, negotiated per-agent chat availability and feature
reporting, descriptor-aware launch/continuation, and tool-bound history ownership.
OpenCode remains the only reviewed structured integration; no new adapter runs.

**Branch / base / PR:** `feat/structured-agent-capabilities`, from main `c77f389`,
[PR #300](https://github.com/eyupio/reet.sh/pull/300), implementation commit `c9f5b52`.
Earlier uncommitted installation/research work remains in its original checkout;
this branch includes only the chat contract, its tests and related documentation.

**Behaviour added:** `acp-agents-v1` negotiates a bounded `acpAgents` list. Entries
report tool, adapter, minimum/observed installation version and
available/missing/unverified/incompatible states. Available features are masked
by negotiated history/images/model/approval support. The connector reads
installation metadata and resolves binaries without executing them. An available
entry still checks executable version and ACP identity/version at launch.

Explicit descriptors are authoritative, including an empty list. Older peers
omit them and retain OpenCode-only fallback. Controller and browser launch,
continuation and feature controls use the selected agent's descriptor. A
connector cannot enable an unreviewed tool by advertising it. Native session ID
validation dispatches by tool, preserving OpenCode's existing namespace, and the
resume query now binds the tool alongside account, machine, folder and closed
session state. There is no schema migration or controller transcript storage.

**Checks and limitations:** protocol/collector/profile, ownership, broker and API
regressions were added, plus launch/continuation unit cases and a descriptor mode
in `chat.e2e.mjs`. Web check/build and both legacy/descriptor chat browser modes
passed, including the launch dialog's desktop/tablet/phone light/dark matrix.
Full `go vet ./...`, all 16 MCP tests, and focused race regressions for launch
refusal, API feature gating and tool/account/machine/folder history binding
passed. The complete `go test -race -timeout=60m ./...` suite also passed;
API took 1,160.6 seconds and store 1,078.2 seconds on this host. The container's
non-reaping PID 1 needs a test-only subreaper for orphan-exit checks, and socket fixtures need
network permission. No real provider, physical phone or new adapter was run.

**Exact next unchecked task:** SC2's Claude Code and Codex ACP adapter probes;
read [the plan](docs/structured-chat-plan.md), `internal/acpagent/{spec,describe,session}.go`,
`internal/terminalproto/acp_agents.go`, `internal/scan/agent_terminal_control.go`
and `web/src/lib/StartAgentDialog.svelte`. Do not enable a candidate before its
callback, permission, trust, provider and history contracts are observed.

**Publication state:** [PR #300](https://github.com/eyupio/reet.sh/pull/300)
merged as `e902b869cecc4b2194a8bc778c8271ec2172c654`.
SC2 remains the separate probe checkpoint in PR #301 with its own handoff.
No release or deployment is performed by these continuation tasks.

## Terminal interface (30 October 2026)

The binary now has a terminal interface: `reet` in a terminal opens a menu over Doctor,
Healthcheck, Scan, Sync, Backups, Setups, Upgrade, Install, Connect machine, Version, Terminal
status, and `--tui` opens a screen directly. Setups also previews and applies a restore and rolls back the
newest completed restore for a profile, all through the same `setup` commands. Scan is a native `huh` form with dry-run, prune and reconcile; Sync
previews a target and applies a hash-checked plan; Backups lists, takes and restores copies through
the shared `backup.RestoreInstance`; Setups creates, pushes and pulls profiles through the shared
`setup.HTTPRemote`; Upgrade runs the same verified, backed-up, rollback-capable upgrade as the text
command behind an explicit confirmation. Install runs the same managed installer behind an explicit
confirmation, with a dry run. Connect machine writes the same enrollment as `reet agent enroll` behind a
confirmation. It is
built with Bubble Tea v2 under `internal/tui`, reads the same report values as the text commands,
and does not touch the store or the web app. v2 removes the v1 startup background query, so a
terminal that does not answer OSC 11 no longer delays every command; the palette comes from
`REET_TUI_BACKGROUND`, `COLORFGBG`, or a dark default. This deliberately moves the "full terminal TUI" note below
ahead of its earlier lower priority; it is scoped to the binary's existing commands, not a second
control room. The planned terminal-interface slices are complete; install, agent enrollment and the longer setup restore/rollback flows remain text commands.

## Live approval modes (31 October 2026)

- **A live structured chat can switch Ask me ↔ YOLO.** New `acp-approval-v1` (`ACPApproval` capability,
  `acp_mode` body) lets the controller change a running chat without restarting it. The connector sets its
  own auto-answer, answers cards already waiting, and keeps the local YOLO switch gate. The controller keeps
  the launch gates (account Allow YOLO, machine `yolo`, agent option), updates `terminal_sessions.yolo`, and
  audits `yolo-mode`; the session header and actions menu carry the switch, with the existing confirmation
  dialog in switching form. Docs: [structured sessions](docs/structured-sessions.md).
- **Ask me can allow a command or file for this session.** A card with a command or path adds
  **Allow this command/file for this session**. The controller remembers the exact tool kind plus command or
  path for that chat, answers repeats itself, audits each as `session-allow`, and forgets on chat end,
  archive or restart. It never sends the agent's `allow_always`. `PermissionChat` takes `forSession`; the
  route body adds `forSession`; the card shows the third button.
- **Checks:** `go test` on terminalproto, acpagent, terminalagent, terminalbroker, store, agenthealth and api;
  `npm --prefix web run check`; `npm --prefix web run build`.

## Agent software inventory and plan (31 October 2026)

**Task ID and scope:** L1, first slice — observe OpenCode on Linux and show a reviewed install, update or remove
plan; nothing is downloaded or executed.

**Branch / commit / PR:** `feat/agent-software-inventory`, commits `ec2a5d3`, `c4be69c`, `42bbded` and `7db0bec`,
[#282](https://github.com/eyupio/reet.sh/pull/282).

**Behaviour added:** a new `software-v1` private-control feature. A negotiating connector reports one
`terminalproto.SoftwareEntry` for OpenCode in its capabilities: state (`installed`, `missing`, `unknown`,
`unsupported`), exact version, package, manager/source evidence, executable path, platform/arch, the plan operations
the evidence supports and `checkedAt`. The connector reads a bounded `package.json` beside the resolved executable
through `setup.InspectInstallations`; it never runs OpenCode, npm or an installer. `GET
/api/v2/machines/{agentId}/software/plan?tool=opencode&action=install|update|remove` builds a plan from stored
evidence (current/target version, exact command, steps, running-session effects, always `executes: false`); the only
reviewed target is the pinned 1.19.5. Machines → Settings → **Software** shows the evidence and the plan inline, with
no Apply action. Read [docs/agent-software.md](docs/agent-software.md).

**Compatibility / migration behaviour:** an older controller does not advertise `software-v1`, so a new connector
sends no inventory; an older connector reports none. No schema migration — the inventory rides in the capabilities
JSON already stored per connection. The plan route reads stored evidence and session rows only and never contacts the
machine.

**Checks actually run and their results:** `npm --prefix web run check` (includes `software.test.mjs`) and
`npm --prefix web run build` passed; `go vet ./...` passed; `go test` passed for `terminalproto`, `terminalhost`,
`agenthealth`, `agentsoftware`, `scan` and the whole `api` package (176 s); `go test -race` passed for the first five
packages and for the software/private-control tests in `api`; `git diff --check` passed. The full `api` race package
timed out at 20 minutes on this shared host (other long Go suites were running), so CI is the gate for it. The
terminal-browser and phone-zoom jobs surfaced two pre-existing timing checks (`skill-content.e2e.mjs`'s records
section and `home.e2e.mjs`'s Start/Opening word), stabilised in `7db0bec` without changing product behaviour.

**Platform or live-environment checks not performed:** no physical machine, real npm run or live OpenCode install
was changed; macOS, Windows, other package managers, other CLIs and any live registry version lookup are out of scope.

**Known limitations or unresolved decisions:** inventory refreshes on reconnect or a local switch change, not on a
timer; 1.19.5 is the only reviewed version and a newer release is not assumed ACP-compatible; execution, rollback
and interruption recovery are not implemented.

**Exact next unchecked task and first files to read:** L2 — verified single-machine maintenance with rollback, still
plan-first. Read `internal/agentsoftware/plan.go`, `internal/terminalhost/software.go`, `internal/api/software.go`,
`internal/scan/agent.go`, `web/src/lib/MachineSettings.svelte` and [docs/agent-software.md](docs/agent-software.md).

**Publication / deployment state (verified, not inferred):** the branch is pushed and PR #282 is open; nothing is
merged, released or deployed.

## Agent software execution (L2, 31 October 2026)

**Owner decisions, 31 October 2026:** (1) use a **verified staged artifact** — download the pinned
`opencode-linux-x64@1.19.5` tarball, verify its recorded sha512 and binary sha256/size, and install from it, never from
an installer script; (2) a pre-existing OpenCode that reet did not install is **inspect-only** — reet installs its own
managed copy alongside and launches prefer it; (3) **refuse** install/update/remove while any live OpenCode session
runs on that machine; (4) the **connector downloads** the artifact from the fixed registry URL (no controller mirror
yet); (5) **any signed-in writer** may apply (the plan binding and audit are the safety boundary); (6) only **install
and update** are exposed in this slice, with automatic restore on a failed verification; (7) **one confirmation
dialog** with the machine, version, source and effects.

**L2a and L2b, on `feat/agent-software-apply` ([#289](https://github.com/eyupio/reet.sh/pull/289), commits `552ed51` and `2742911`, rebased onto `main` after #282 merged):** `internal/agentsoftware/artifact.go` holds the
one reviewed archive (URL, sha512, binary sha256 and size, the single `package/bin/opencode` member) and
`StageTarball`, which downloads it with a bounded response, verifies the archive checksum, extracts only the member
and verifies the binary's hash and size; it never executes anything. `managed.go` owns
`~/.reet/software/<tool>/versions/<version>/opencode` with a per-version receipt and a current receipt, an atomic
install from a staged file, a hash-verified rollback to the kept previous version, and a remove that deletes only the
managed tree (configuration and history live outside it). A system install has no receipt and is never updated or
removed. Tests use a fake tarball and a local HTTP server: checksum failure, wrong member, symlink member, traversal
entries, install/current/rollback/remove, broken binary, no previous version, and a user config file surviving removal.
Checks: `go test -race ./internal/agentsoftware`, `go vet ./internal/agentsoftware`, `go build ./...`.

**L2b, implemented on the same branch:** `software-apply-v1` is a separate negotiated capability, reported only while
terminal access is on. `terminalproto` adds `SoftwareApply`/`SoftwareApplyResult`, `SoftwareEntry.Managed`, the
`managed_broken` reason and `software-apply-v1`. The connector re-inspects the evidence, refuses when it changed,
downloads and verifies the artifact, installs it, verifies the current copy and restores the previous managed version
on failure (`terminalhost.ApplySoftware`); a process-wide slot serialises applies. The broker asks a ready connection
and checks the capability. `POST /api/v2/machines/{agentId}/software/apply` rebuilds the plan, refuses while a live
OpenCode session exists, sends only state/version/managed, merges the fresh entry into stored capabilities, audits
`software-apply`, and maps refusals/failures to codes. The UI shows **Apply** only when the machine reports the
capability, uses one confirmation dialog, and records the fresh entry. Launches, model listing and inventory now
prefer the managed binary (`launchArgv`, `ListModels`, `InspectSoftware`). A system install has no managed receipt and
is never updated or removed. Docs: [docs/agent-software.md](docs/agent-software.md).

**Checks for L2b:** `go test` passed for `terminalproto`, `agentsoftware`, `terminalhost`, `terminalagent`,
`terminalbroker`, `agenthealth`, `scan` and `api`; `go test -race` passed for the first seven and for the
software/private-control tests in `api`; `npm --prefix web run check` and `npm run build` passed; `go vet ./...` and
`git diff --check` passed. The full `api` race package was not run on this loaded host; CI is the gate.

**L2c, on `feat/agent-software-remove-rollback` ([#291](https://github.com/eyupio/reet.sh/pull/291), commit `e9657dd`, stacked on #289):** explicit **rollback** and **remove**, both only
for a reet-managed copy. `SoftwareEntry.Previous` carries the kept version; `SoftwareActionRollback` and
`SoftwareActionRemove` extend the apply request/result; `ApplySoftware` verifies the evidence, calls
`RollbackManaged`/`RemoveManaged`, verifies the result and reports the fresh entry. The controller's plan and apply
routes accept both; remove/rollback are refused while an OpenCode session runs; the UI lists the supported plans
(install/update/rollback/remove) and remove needs the machine's name typed in the confirmation dialog. A system copy is
never touched, and removing takes reet's kept previous versions with its own copy. Tests cover rollback plans,
remove effects, applied/refused results, the API actions and typed confirmation; docs and README are updated.

**L2d, on `feat/agent-software-remove-rollback` (with L2c):** `agentsoftware.RecoverManaged` runs once when the
connector starts. It removes staging files and unfinished version directories, verifies the current binary, restores
the kept previous version when the current is broken (clearing the broken rollback target), clears a previous target
that no longer verifies, and reports a broken current with no valid previous as `managed_broken` rather than hiding it
or falling back to a system copy. Tests cover leftovers, a broken current with a good previous, a broken current with
no previous, an invalid previous, and orphan versions without a current receipt. Docs: [docs/agent-software.md](docs/agent-software.md).

**L2e, on `feat/fleet-software-review` ([#292](https://github.com/eyupio/reet.sh/pull/292), commit `b06585d`, stacked on #291):** a **plan-only fleet review**. `GET
/api/v2/software/plans` builds one row per enrolled machine from stored evidence (version, managed/system/missing,
action, target, source, running sessions, whether the machine can apply, sentence) without contacting a machine.
Machines → **Software** opens the review dialog; rows can be selected locally, the footer says cross-machine apply is
not available yet, and each machine's Settings remains the place to apply. Tests cover the aggregate rows and the
label/warning helpers.

**L2f, on the same branch:** the fleet review can now **apply one machine at a time**. Each actionable row has
**Apply plan** when the machine reports `software-apply-v1` and no OpenCode session runs; it opens one confirmation
dialog and calls the existing per-machine apply endpoint. Other rows are untouched; there is no batch selection. The
row shows the outcome and the list refreshes from stored evidence; rollback/remove stay in Settings. Tests cover the
review label/warning and the new can-apply rule.

**Still not in this slice:** no batch apply or controller-side artifact mirror; the binary is verified at install and at
startup, not on every launch. Next: reviewed batch apply with a pilot order, and the mirror. Read
`internal/agentsoftware/{artifact,managed,plan}.go`, `internal/terminalhost/{software,software_apply}.go`,
`internal/terminalbroker/software.go`, `internal/api/software.go`, `web/src/lib/MachineSettings.svelte` and
`web/src/lib/FleetSoftware.svelte`.

**L3, reviewed batch apply (PR [#314](https://github.com/eyupio/reet.sh/pull/314)):** the fleet review now selects machines into an **ordered batch**. The
controller rebuilds each plan from stored evidence and freezes it; a machine that cannot apply (no plan, a live
OpenCode session, no `software-apply-v1`, already at the target, or held by another active batch) is excluded with its
reason shown. A batch applies one machine at a time in position order and stops at the first failure; a stopped batch
offers **Retry**, **Skip and continue** or **Abandon batch**. A refused plan whose machine already shows the target
version, and a member left running by a controller restart, are reconciled as applied. At most one batch is active per
signed-in person; a machine is in at most one. New durable tables `software_batches` / `software_batch_members`
(`internal/store/store_software.go`), a `softwarerun` orchestrator modelled on `projectrun`
(`internal/softwarerun/run.go`), and routes `POST/GET /api/v2/software/batches…` with retry/skip/abandon
(`internal/api/software.go`). Progress is polled; `FleetSoftware.svelte` carries selection, accessible up/down ordering,
one confirmation sheet and the live batch panel. Audit: `software-batch` at start and each terminal state, plus
`software-batch-retry`, `-skip` and `-abandon`. Tests cover the store guards and transitions, the orchestrator's order,
stop, retry, skip, offline code and reconciliation, and the API routes and audit. Checks run: `go vet ./...`; `go test`
for `store`, `softwarerun` and the whole `api` package (210 s); `npm --prefix web run check` (includes
`software.test.mjs`) and `npm --prefix web run build`. Not run: a live machine apply and the browser e2e suites.
**Still not in this slice:** batching rollback/remove. Read `internal/softwarerun/run.go`,
`internal/store/store_software.go`, `internal/api/software.go` and `web/src/lib/FleetSoftware.svelte`.

**L4, controller-side artifact mirror (same PR):** an opt-in admin setting (`software_mirror_enabled`) and an
Admin → **Software artifact mirror** section (owner/admin) with the toggle, cached size and **Cache the reviewed
artifact now**. `internal/softwaremirror/mirror.go` downloads the pinned archive once on demand (single-flight),
verifies its size, sha256 and sha512 and the extracted binary's sha256 and size with `agentsoftware.CheckTarball`,
then fsyncs and atomically renames it to a content-addressed file; a partial file is never served, and GC at
controller start keeps only the reviewed digest. The archive sha256 and size joined the pinned table
(`internal/agentsoftware/artifact.go`). Machines fetch it over the existing private connection at
`/private/v1/software/artifact/<archive sha256>` (exact-path allowlist, machine credentials, any enrolled machine in
the account, four concurrent serves, exact `Content-Length`); the route 404s when the mirror is off or the digest is
unknown. The connector derives the path from its own pinned table, tries the registry first and the mirror only when
that fails, verifies the same pinned artifact either way, and reports `registry`, `mirror` or `registry+mirror` in the
apply result. Timeouts: registry leg 5 minutes, mirror leg 15, connector apply 20, broker 21, orchestrator and
per-machine API 22. `FleetSoftware.svelte` carries the mirror status line. Tests cover the mirror's cache, single-flight,
corrupt/truncated files and GC; the connector's fallback and both-failed reporting; and the settings, cache action and
private route (enabled, disabled, unknown digest, wrong credential). Checks run: `go vet ./...`; `go test` for
`agentsoftware`, `softwaremirror`, `terminalhost`, `terminalagent`, `store`, `softwarerun` and the whole `api` package;
`npm --prefix web run check` and `npm --prefix web run build`. Not run: a live registry download into a real mirror, a live
machine fallback, or the browser e2e suites. **Publication state (verified, not inferred):** branch
`feat/software-batches-mirror` is pushed and PR #314 is open; nothing is merged, released or deployed. **Next:** the
roadmap's next directions (structured chats across agents, richer run orchestration); batching rollback/remove and
mirror resilience are later follow-ons. Read
`internal/softwaremirror/mirror.go`, `internal/api/software_mirror.go`, `internal/api/private.go`,
`internal/terminalhost/software_apply.go` and `internal/terminalagent/agent.go`.

## Agents home redesign (27–30 October 2026)

### Resume here (after Needs you and the approvals slice, 31 October 2026)

- **Chat images (in progress, branch `cline/6t9b2yrw`):** the reet TUI now passes a pasted, dropped or chosen image to
  OpenCode as embedded image content: the browser uploads it to the machine, `POST /chat/prompt` carries the upload path, and the
  connector embeds the bytes as an ACP image content block (`acp-image-v1`, `acpImages` capability). The agent sees the
  image itself, not a path it must open with a tool. At most eight images per prompt; older connectors keep the path-as-text
  fallback. Unit tests cover the protocol, ACP child content blocks, connector upload-root validation and the API; the mocked
  chat e2e at 1360x900 and 390x844 passes. Not checked: a live OpenCode model turn with an image, or Safari.

**#272 (inline approvals) merged as `362ffd9`; #269 (Needs you) as `52afc9d`;
#270 (reet TUI model choice and provider error recovery) as `1717b7a`;
#266 (YOLO audit follow-up) as `99b3fd0`; #264 (YOLO sessions) as `9cf82f4`;
#261 (reet TUI naming and terminal views) as `329f978`; #260 (Home reet TUI) as `2b35802`;
#258 (reet TUI history, continue, images) as `2ee12f6`; #256 (OpenCode in the reet TUI) as `7555da2`;
#255 (models UI) as `32866ad`; #252 as `3be0eaf`; #247 as `16da0bb`; #245 as `1371cb5`;
#242 as `961d530`; #231 as `7886511`.**

- **Merged PR:** [#269](https://github.com/eyupio/reet.sh/pull/269) implements PR G (`attention-v1`
  terminal activity and bell reporting, per-person `terminal_session_seen` store and `POST /api/v2/terminal/sessions/{id}/seen`,
  attention derivation for both `attention-v1` terminal sessions and structured reet TUI sessions, recovering completed-turn
  attention across controller restarts, gating chat snapshot attention fields behind `attention-v1` feature negotiation,
  coordinating workspace document titles with live attention counts via `titleWithAttention` in `App.svelte`, Home's **Needs you** queue,
  the bell popover, `(N)` document title prefix, polite live-region announcements, and Codex's YOLO preset updated to `--ask-for-approval never`).
  Its final head was green; merged under the owner's exception because the Codex quota was exhausted (recorded on the PR).
- **Merged PR:** [#272](https://github.com/eyupio/reet.sh/pull/272) implements Program 2, Slice 3
  (inline **Allow once** and **Deny** approvals on Home's **Needs you** cards and the header bell popover, backed by
  `TerminalSession.PendingPermission` queueing the earliest pending permission in `ApplyChatAttention`, shortening command
  previews to 240 bytes with `Truncated: true` to require opening chat for full commands, answer plumbing, unit tests, and Playwright e2e tests).
  It was retargeted from `ccr-attention` to `main` after #269 merged and merged under the same exception.
- **Merged PR:** [#270](https://github.com/eyupio/reet.sh/pull/270) added reet TUI model choice and provider-error
  recovery. OpenCode's `stream error` log is surfaced when a provider rejects a model, and the shared model picker lets a
  reet TUI session use a selected model while preserving OpenCode's configured default. F shipped: the account's Allow YOLO setting (`GET`/`PUT
  /api/v2/settings/yolo`, owners and admins change it), the machine's `reet terminal yolo
  on|off|status` switch and `yolo` capability, reviewed options for Codex/OpenCode/Cline, controller
  gates and audit, structured `allow_once` answers, the confirmation dialog and badges. #266 fixed the
  launch audit on a mismatched worktree and kept generic machine refusals from counting as started.
- **Next stage:** L2 install/update is in [#289](https://github.com/eyupio/reet.sh/pull/289); rollback/remove and
  restart recovery in [#291](https://github.com/eyupio/reet.sh/pull/291) (stacked on #289); the plan-only fleet review
  is in [#292](https://github.com/eyupio/reet.sh/pull/292) (stacked on #291), with a one-machine-at-a-time apply; the
  reviewed batch apply (L3) and the controller-side artifact mirror (L4) are built and awaiting review. Program 2,
  slice 4 (elicitation and rich input cards) still needs its own probe.
- **Do first:** for the next PR, check its final head with the REST forms
  (`gh api repos/eyupio/reet.sh/pulls/<n>` and `gh api repos/eyupio/reet.sh/commits/<head sha>/check-runs`;
  `gh pr view`/`gh pr checks` use GraphQL, refused with 403 here). Merge only when every check is green and Codex has
  reviewed it, or the owner gives an explicit exception. The full race job now has a 60-minute budget.
- **Release:** F can ship separately. R22 still lets the earlier B, C, E, copy/paste, models and reet TUI
  work go out together; the owner's call.
- The research these start from is in [docs/research/](docs/research/) (copied whole from the session's git-ignored
  notes). Those reports summarise third-party code and docs read as untrusted data: check a claim against its label
  (*read*, *doc*, *belief*/*memory*) before building on it.

### State

Home, the shell and the machine-folder projects were rebuilt to the mock-up in
[docs/design/agents-home/](docs/design/agents-home/) (reference renders at 1440, 834 and 390 px; each PR is checked against
them in `docs/ui-verification.md`). The plan is
[docs/superpowers/plans/2026-10-26-agents-home.md](docs/superpowers/plans/2026-10-26-agents-home.md); the **rulings R1–R22**
are binding and live in the spec's [Rulings](docs/superpowers/specs/2026-10-26-agents-home-design.md#rulings) section.
**Quicker plan, agreed 27 October 2026:** the fidelity polish (P) was folded into C and E, YOLO (F) and attention (G) were
left out of the first pass, and the Program 2 spike ran in parallel. F shipped in #264/#266; G is on `ccr-attention`.

| PR | What | State |
| --- | --- | --- |
| D | App shell (top and bottom tabs, bell, Jump, More sheet) and Home | merged, [#218](https://github.com/eyupio/reet.sh/pull/218) as `8c33c99` |
| A | Machine folder browse and branches (`agents`, `browse-v1`, `branches-v1`), start-agent opens the session, dockable sessions panel | merged, [#219](https://github.com/eyupio/reet.sh/pull/219) as `41066c5`; contract in [docs/folder-browse.md](docs/folder-browse.md) |
| B | Folder projects: store and API (`/api/v2/folder-projects`, reorder, `import-saved`), saved Projects moved over (old writes answer 410 `projects_moved`), the Add project dialog with the folder browser, the rail; `/projects` redirects to Home; read [docs/projects.md](docs/projects.md) | merged, [#221](https://github.com/eyupio/reet.sh/pull/221) as `cb89a26` (no Codex review of its final head; merged on green CI, no exception comment) |
| P | Fidelity polish | folded into C and E |
| C | Agent catalogue and launch fields (`agents-v2`, named worktree base, effort, tool on the listing, `osVersion`), Cline with safe arguments, chrome polish | merged, [#226](https://github.com/eyupio/reet.sh/pull/226) as `540d247` (no Codex review: quota exhausted; merged on green CI, no exception comment) |
| E | Launch bar on Home, Start in several projects, fixes from the #226 review; read [docs/start-agent-session.md](docs/start-agent-session.md#on-home-the-launch-bar) | merged, [#227](https://github.com/eyupio/reet.sh/pull/227) as `fe3e9b7` (owner exception, recorded on the PR) |
| F | YOLO (off by default, per session, audited, machines can refuse), with the account Allow YOLO setting, machine switch and capability, reviewed preset options, confirmation dialog and badges | merged, [#264](https://github.com/eyupio/reet.sh/pull/264) as `9cf82f4`; follow-up [#266](https://github.com/eyupio/reet.sh/pull/266) as `99b3fd0`; plan `docs/superpowers/plans/2026-10-26-agents-home.md` (Task F) |
| G | Attention (`attention-v1`), per-person seen state (`POST /api/v2/terminal/sessions/{id}/seen`), Home's Needs you queue, bell popover, title count and live-region announcements | merged, [#269](https://github.com/eyupio/reet.sh/pull/269) as `52afc9d` (owner exception: Codex quota, recorded on the PR); plan `docs/superpowers/plans/2026-10-26-agents-home.md` (Task G) |
| Program 2, slice 3 | Inline approvals on Home: answer pending reet TUI command and edit approvals (**Allow once** / **Deny**) directly from Home's **Needs you** column and bell popover | merged, [#272](https://github.com/eyupio/reet.sh/pull/272) as `362ffd9` (retargeted to `main`; owner exception); plan `docs/superpowers/plans/2026-10-26-agents-home.md` |
| H | `/sessions` (the Sessions page), the session screen, tablet and phone layouts, the last redirects, the phone rail on Sessions | merged, [#231](https://github.com/eyupio/reet.sh/pull/231) as `7886511` (owner's decision; no Codex review possible: quota exhausted). H1 `f0bffe5` Sessions page and Control's address retired (`/control` → `/`, `/terminals` → `/sessions`), fixes `048e431` (session colours, **Saved commands** button: owner ruling), `b85338a`; H2 `f74001f` session screen (tabs, header with the state pill, Chat \| Terminal with Chat disabled, context panel), fixes `f026cfb`, `c02fec6`; H3 `7581ad0` tablet and phone layouts (no rail on narrow Home, the rail on Sessions, slim phone header, `keyboardChrome('session')`), fixes `a984b5e` (44 px tablet targets, cases moved to `sessions-phone.e2e.mjs`), `3f1e89b` (separator grip); `521c406` CI fix (first-terminal checklist off `/sync`); merges of main `9c88a46` (#232, #233), `ae763c8` (#234) and `57d8f53` (#235, #236); H4 `1a5018d` (research copied into `docs/research/`), `c349a2b` (the matrix, phone target fixes, docs, this handoff), `1ff4230` (the research notes name the reference tool by role); final fix wave after the broad review `b53c7dd` (`/sessions/`, viewers at a session address), `adec499` (steady Sessions rows, menu focus, the connection in the phone header, tablet swatches and grip, context panel, tab tooltip, New → `/#launch`), `dd31068` (dead code), `8e6ce3e` (checks that went with Control, steadier phone tests), `6778211` (start-agent expects the Machines title) and the docs commit after them |
| Copy/paste | Terminal paste sanitiser and bracket-aware review, copy screen/last lines/all, Select text sheet, copy-on-select, gated OSC 52 (`clipboard-v1`), image attach/paste/drop with `upload-v1` and private per-session storage | merged, [#242](https://github.com/eyupio/reet.sh/pull/242) as `961d530` (owner's instruction; Codex quota exhausted). C1 `81b0e74`, C2 `dc93c39`, C3 `93e7e37`, C4 `7a75e8a`, C5 `f90e1e3`, U1 `64b2240`, U2 `497eda7`, U3 `cb606bc`, fence `1614b44`, U4 `4e9ac2f`, U5 `b209152`, review fixes `81b9e34`, `aa532b6`; the final CodeQL response/headers work and this follow-up are in #245 |
| Copy/paste follow-up | `Exited` cleanup, running 24 h sweep, two concurrent uploads and third refusal at the connector, lost-reply timeout, Scrollback-mid-upload wording, this handoff | merged, [#245](https://github.com/eyupio/reet.sh/pull/245) as `1371cb5` (owner approval; Codex re-review clean) |
| Models backend | `models-v1`, OpenCode listing, local switch, per-machine cache, read/refresh/list API, favourites, account defaults, project effort, Cline project default | merged, [#247](https://github.com/eyupio/reet.sh/pull/247) as `16da0bb` (owner exception; Codex quota). M0 `c1152ff`, M1 `f4bf81c`, M2 `11afb42`, M3 `9d7f234`, M4 `5490a6c`, docs `3cd34f0` (plan `2a54133`); review fixes `56f5f5b`, `39a61a8`, `a8c9310` |
| Models UI (M5–M7) | `models.ts`, model/favourite clients, picker star/status/Refresh/Use, launch bar defaults-first, Home account defaults, automatic machine read | **[#252](https://github.com/eyupio/reet.sh/pull/252), open**. Plan `d802ff0`, M5 `a7e0d7d`, picker `9202695`, bar `ca0f972`, review fixes `67feb99`, evidence `24df6cc` |
| Models UI (M8–M11) | Start an agent picker with project/account defaults and Cline thinking; Add project/Project settings defaults; Settings › Agents; Machines “Agents” cache state and guarded Refresh; Activity/hover-card model lines; docs and phone evidence | merged, [#255](https://github.com/eyupio/reet.sh/pull/255) as `32866ad` (owner's instruction; Codex reviewed an earlier head, the runner timed out on the final head and a rerun passed). Plan `docs/superpowers/plans/2026-10-29-agent-models-ui-2.md` |
| Program 2, slice 1 | OpenCode in the reet TUI: `acp-v1`, machine-side ACP supervisor with the seven probe conditions, controller relay and SSE, reet TUI, Allow once / Deny, docs | merged, [#256](https://github.com/eyupio/reet.sh/pull/256) as `7555da2` (owner's instruction; Codex findings fixed, all checks green) |
| Program 2, slice 2 | Reopen a closed reet session with `session/load` history, continue it, and attach images as machine paths | merged, [#258](https://github.com/eyupio/reet.sh/pull/258) as `2ee12f6` (Codex mixed-version finding fixed with `acp-history-v1`; all checks green) |
| Home OpenCode reet TUI | OpenCode on an `acp` machine starts in the reet TUI from Home's launch bar; the TUI word switches to the agent TUI | merged, [#261](https://github.com/eyupio/reet.sh/pull/261) as `329f978` (renamed reet TUI/OpenCode TUI, trust, Creating reconciliation, terminal views) |

Merged into main by the owner while this work was open: #222, #223 (`4791d89`), #224, #225, #228, #229, #230, #232, #233,
#234, #235 and #236 (each merged into the open branch before its next push; #235 and #236 in `57d8f53`), then #237–#244
and the Sync follow-ups while #242 was open (merged into it before each push). #242 then merged as `961d530`; #245 as
`1371cb5`; #246 and later main work landed while the models backend was open.

**Merge rule:** a PR in this work merges only when all checks are green on its final head **and** Codex has reviewed it
cleanly, or the owner gives an explicit exception. So far: #227's exception is recorded on the PR (a comment); #221 and
#226 merged on green CI without a Codex review and without an exception comment; #231 merged on the owner's decision
after green checks; #242 merged on the owner's 28 October instruction to create it and merge when green; #245 merged
on the owner's approval of the follow-up recommendation with a clean Codex re-review; #247 merged as `16da0bb` under the
owner's merge instruction; #252 and #255 merged on the owner's instruction to merge when green; #255's final head passed
after a terminal-browser rerun. CodeQL alert #20 on #242 was dismissed as a false positive (the
machine path is validated plain text, URL-escaped into response headers, and never in the JSON body; JSON plus nosniff).

**Rulings beyond R1–R22** (all recorded in the session notes and kept here so they survive):

- Owner, 27 Oct: the quicker plan above; Pi stays disabled; Cursor stays blocked until its installer host is allowed.
- Owner, 28 Oct: a **Saved commands** button on the Sessions page; copy and paste is its own PR after H, with pasted
  images in a private per-session folder (0700/0600, random names); Program 2 slice 1 is trusted workspaces only (OpenCode,
  Allow once / Deny only) behind the seven probe conditions; the order after H is copy/paste, models, Program 2; pause after
  PR H. Later that day: create the copy/paste PR and merge it when green. Copy/paste is #242 (merged as `961d530`);
  its image path is text only (per-agent attachment behaviour unverified). Then approve the follow-up #245 and start
  the models backend. 29 Oct: the models backend is #247; the web picker follows it as UI-1 (#252, M5–M7) and UI-2
  (M8–M11). The owner instructed: keep going, create PRs and merge when green.
- Controller defaults, 28 Oct (owner may revisit): models D2 a per-machine default is deferred, D3 a favourite can be the
  "default for this agent", D4 the model-name rule stays as it is.

### Limits and known gaps

After the copy/paste PR:

- Pasted images reach an agent as a **path**, not as an attachment reet creates. The UI and docs say so. Per-agent
  image-path behaviour at the pinned CLI versions has not been checked; do not claim one before checking it.
- Uploads are a new `upload-v1` feature and are on by default; `reet terminal uploads off` refuses them and the browser
  shows the sentence. An older controller never negotiates the token and never sends one. The machine stores at most 20
  images / 50 MiB per session, sweeps files older than 24 h (at start and about hourly), and removes the folder when the
  session ends. The controller audits machine, host, tool, MIME and a size bucket only.
- OSC 52 forwarding needs `clipboard-v1` and `reet terminal clipboard on`; `allow-passthrough` stays off. The browser
  still writes only after a gesture or **Copy it**, write-only, at most 1 MiB. Old browsers ignore the sequence.
- Image uploads write to the machine's private terminal directory, outside the project. They are derived state, not in
  the registry. A machine that ends a session mid-upload may make that one upload fail; nothing is retried.

After the models UI:

- Only OpenCode reports a machine-owned model list. Favourites, reet's suggestions and Other… are offered for every
  agent; they are suggestions, not proof that a model exists. A per-machine default stays deferred (D2); the account
  and project defaults are per account/project.
- **Refresh** reads the machine only when it is online and models-capable. Offline shows the last list with its age and
  disables Refresh; a failed refresh keeps the last good list and age. A cache read never contacts a machine.
- No real provider-backed OpenCode list was used; the tests use fake machines and synthetic API answers. A 500-model
  list was not load-tested, and no physical phone, Safari or live agent CLI run was part of this PR.
- The new terminal controls have no mock-up element. They join the existing toolbar, menu and banner styles; at 1440
  the toolbar scrolls as it did before. The Select text sheet, upload chip, copy-on-select checkbox and the uploads-off
  note are the additions listed in [docs/ui-verification.md](docs/ui-verification.md).
- `terminal-clipboard.e2e.mjs` and `terminal-upload.e2e.mjs` are in the `phone-dialogs` CI job. Nothing new was added to
  `terminal-browser`. The local matrix's session screen passes 40 states.
- Still true from PR H: the session screen keeps the sessions panel and workspace toolbar (the mock-up has neither); the
  tabs share the toolbar row and the pane keeps its own header. Old `/sessions` links on a rolled-back server still land
  on `/` or an unknown page. Needs you now shows real cards (#269); before that PR it was an honest empty state. The site footer and phone terminal chrome keep their
  documented 40 px targets. Nothing has been checked on a real phone or in Safari.

After the models backend:

- Only OpenCode has a machine-read list. Claude Code, Codex, Gemini CLI and Cline keep reet's static suggestions,
  favourites and Other…; the web picker is not in #247, so the browser does not show a machine list yet. The
  favourites/defaults API is likewise not wired to a page.
- A listed model is a suggestion, never proof it works, and it never blocks a launch. Env-only OpenCode providers
  are missing because the listing environment is an allowlist; say so when the picker lands.
- The cache keeps the last good list through a failed refresh and is deleted with the machine. A first read is
  synchronous only for an online machine that reports `models`; refresh is 6/min per account and machine and is the
  only audited path. A failure is visible as its own state, not as an empty successful list.
- The account defaults are for sessions outside a project; the browser resolves explicit > project > account >
  agent default. A per-machine default remains deferred (D2). A project default model still needs a project agent;
  an existing profile model without an agent stays readable and does not block unrelated edits.
- No real provider or paid model was used in the backend tests; the listing runner uses fake binaries, the API uses
  synthetic machines, and a physical phone/Safari was not checked.

After the models picker/launch bar:

- The launch bar now opens on the resolved defaults, not the last launch; the last models are a Recent group. The
  model slot has a star, filter, Use ‘x’ and a status/Refresh line; tablet/phone keep the model in the Change sheet.
- Only OpenCode has a machine list; other agents use favourites/suggestions/Other. The account defaults and
  favourite manager are under Account → Agents; the Start dialog, Add project/Project settings, Machines → Settings
  → Agents, Activity and the hover card are in #255. A per-machine default remains deferred.
- Home does not load favourites itself; the bar does (a recorded plan deviation). A model read failure never
  blocks Start. The real cached-offline shape is overlaid in the bar, not by the API.
- No physical phone/Safari; a 500-model list under a slow reader is not load-tested. The picker/star additions are
  not in the drawings; `docs/ui-verification.md` lists them.

After the reet TUI slices:

- **Home starts OpenCode in the reet TUI by default.** The launch sentence shows **in reet TUI** for OpenCode on an
  `acp` machine; the **TUI** word switches to **in OpenCode TUI**. The reet TUI runs in the project folder, not a
  worktree, and uses OpenCode's configured model. Home requires **I trust this folder** before Start, because `.opencode`
  plugins run when the reet TUI starts. A multi-project selection must switch to the agent TUI; it starts in one folder.
  A session's actions menu offers **Open a terminal here…**, a plain shell in the same folder as a separate session; it
  needs `shell-dir-v1` on the machine. **Open a terminal beside this session…** shows that plain shell in the right-hand
  panel, replacing the context panel while it is open; Close ends it. The agent TUI/reet TUI stays in the main column.
- **reet TUI history is bounded and kept.** The controller keeps at most 1,000 events / 256 KiB per reet session in SQLite,
  and a controller restart restores it; archiving drops it. A closed reet session can be reopened with Continue in reet TUI
  with history… as a new row that runs `session/load`; the old row stays. A control connection loss detaches chats without
  killing them; the next connection adopts the running process, reconciles it as running, and drains the bounded queue of
  events produced while detached. A connector restart or reboot reopens the recorded chats from OpenCode's store. Pending
  approvals are not restored; replayed tools are shown interrupted.
- **OpenCode 1.19.5 or newer.** The connector refuses an older version before starting a process; the ACP handshake must
  also name `OpenCode` and speak protocol 1. The seven conditions in
  [structured-sessions.md](docs/structured-sessions.md) are mandatory; a repository with `.opencode` plugins needs the
  Start dialog's trust confirmation. Always allow is never offered, and a diff or command too large for one control frame
  cannot be allowed.
- **No live model turn** was sent in the checks. The real 1.19.5 binary opened and then loaded a session in a throwaway
  HOME; the protocol tests use a fake ACP child. Modes, MCP servers and other agents are not in this slice. No physical
  phone, Safari or Firefox was checked.
- **Images are paths.** reet TUI attachments use the launch-upload folder and the prompt names the file; no image bytes are
  embedded in ACP. The 5 MiB / 20 files / 50 MiB limits and 24-hour sweep apply. The reet TUI's model is OpenCode's configured
  model; the picker is hidden. The process can still install `@opencode-ai/plugin` from npm in the background.

Before this PR:

- The session screen keeps the sessions panel and the workspace toolbar (the mock-up has neither); the tabs share the
  toolbar row, and the pane keeps its own header (some information is shown twice). A later PR may drop the panel there.
- Old `/sessions` links and sign-in resumes that reach an older server (a rollback) land on `/` or on an unknown page:
  PR H changed no Go protocol and nothing redirects `/sessions` there.
- `/sessions` with `session`, `agent` or `view=terminal` in the address is the workspace; bare `/sessions` is the list.
  The app has no client-side router: links are full page loads.
- Sessions outside a project are grouped "Other on <machine>" only (the listing has no folder to match a project by).
  The **Needs you** filter on `/sessions` matches sessions currently in `needsYou`. Check and remove worktree is offered only when the worktree path and
  branch name the run.
- `railState` puts "N running" ahead of "machine offline", so a project on a machine that is not reporting shows its
  last known count without saying so.
- Two sentences the spec does not word: **Start in these on Home** and "Projects now live on the Sessions screen."
- The site footer (account name, Admin, the developer credit) has targets under 44 px on touch; it predates the agents
  home and was left as it is.
- On a touch phone the terminal's chrome (workspace toolbar, pane header and footer, terminal keys) is 40 × 40 px, not
  44, so the terminal keeps its rows; documented in [docs/terminal-workspace.md](docs/terminal-workspace.md#phones). H4's
  40 px footer button costs one row with the keyboard down at 390 × 844 (39 → 38); keyboard-open and sideways rows are
  unchanged.
- `terminal-phone.e2e.mjs`'s 20 × 8 floor case used to read 13 rows in about one run in five: the workspace height was
  measured while the shell held the top bar aside and not measured again when it came back. Fixed and hardened in
  `5abe001` (the workspace measures again whenever the shell toggles its chrome; the test waits for the layout to
  settle instead of a fixed 450 ms); 10 runs in a row and one under CPU load pass, with the same figures.

Still true from earlier PRs:

- YOLO is live behind the account setting, the machine's acceptance, a reviewed agent option and a per-session
  confirmation. Claude Code and Gemini have no captured YOLO option, so they refuse it. The first line of a prompt
  becomes the stored session title. The Start an agent dialog now offers Cline on an agents-v2 machine; Pi and Cursor
  stay disabled.
- A base a member only inherits from the account's default is not sent to a machine without `agents-v2`; a project's
  own base is sent, and such a machine refuses it with "Update reet on that machine…". Rolling back to an older server
  while a start in several projects has members pending may fail those members or start them without base or effort.
- A replayed start is checked again against today's rules; a start in several projects checks "one start per folder" by
  machine and folder.
- The old Projects tables and their start/retry routes stay one release before they are dropped; a downgrade then
  re-upgrade leaves Projects saved while downgraded as "waiting folders"; a session's `projectId` is checked for account
  and machine, not folder; the import's answer still says "saved repositories".
- Worktree git calls share the lazy-fetch and fsmonitor hazard the folder browser already guards against.
- Nothing has been checked on a real phone or in Safari (browser emulation only), including the touch long press.
- The tablet shows its own **Change** button for model and worktree (approved deviation). Per-screen differences from
  the drawings are listed per PR in [docs/ui-verification.md](docs/ui-verification.md).

### Queued work (decided; next up)

**1. Verified agent maintenance (L2).** L1 inventory and plan-only maintenance for OpenCode on Linux is in
[#282](https://github.com/eyupio/reet.sh/pull/282) (see the section above). Next: execute a reviewed install, update
or remove with a staged/verified artifact or a specifically supported package-manager adapter, an explicit decision
about running sessions, a journal/rollback route and verification of the resulting version. No generic curl-to-shell
installer or arbitrary controller shell command.

**2. Program 2, slice 3: Inline approval cards in Home's Needs you column.** Merged in
[PR #272](https://github.com/eyupio/reet.sh/pull/272) as `362ffd9` (retargeted to `main`; owner exception because the
Codex quota was exhausted). It surfaces pending reet TUI permission
requests (`permission` events with `permissionId`, `title`, `command`, `diff`, `truncated`) on `GET /api/v2/terminal/sessions`
so Home's **Needs you** column and the bell popover render the command/edit approval cards with inline **Allow once** and
**Deny** actions (`POST /api/v2/terminal/sessions/{id}/chat/permissions/{permissionId}`) alongside **Open**. Slices 1 and
2 are merged in #256 and #258; their seven conditions and limits remain binding:
[docs/research/probe-report.md](docs/research/probe-report.md),
[reference-tool-frontend-research.md](docs/research/reference-tool-frontend-research.md) and
[reference-tool-agents-research.md](docs/research/reference-tool-agents-research.md). Owner decision: trusted workspaces only, OpenCode, Allow
once / Deny only, and only with all seven conditions:

1. A reet-owned ask-everywhere config in `OPENCODE_CONFIG_CONTENT` (edit, bash and external_directory ask, for every
   built-in agent; `default_agent: "build"`; `task: deny`; `webfetch: deny`), `OPENCODE_DISABLE_PROJECT_CONFIG=1`, and the environment
   scrubbed of `OPENCODE_PERMISSION`, `OPENCODE_CONFIG`, `OPENCODE_CONFIG_DIR` (and any `OPENCODE_*` reet does not set);
   check the mode is `build` after `session/new`.
2. A random `OPENCODE_SERVER_PASSWORD` per process: every `opencode acp` starts an unauthenticated local HTTP API on
   127.0.0.1:4096 that can answer permissions behind the client's back.
3. `task: deny` (subagent permissions never reach the protocol).
4. A permission timeout and a turn watchdog, with reet closing its own pending cards on cancel.
5. An environment allowlist and the opt-outs (models fetch, autoupdate, default plugins, LSP download and the rest).
6. A warning when the repository has `.opencode` plugins (repository plugins run at session start).
7. OpenCode pinned at exactly `opencode-ai@1.19.5` (integrity in the probe report). There is no `--standalone` or
   `--auto` on `acp` in 1.19.5.

**The probe's blocking findings** (full text: [probe-report.md](docs/research/probe-report.md), sections 4, 6 and 7):

- By default OpenCode asks nothing: writes, edits and shell commands ran without a permission request.
- A repository's config, the user's config or `OPENCODE_PERMISSION` can switch asking off.
- Every ACP process serves an unauthenticated control API on a predictable local port; an approval over it leaves the
  ACP card pending.
- Repository plugin code runs when a session opens, whatever the flags: no-go for untrusted repositories.
- No permission timeout; subagent permissions never reach the client, so a turn can hang; provider errors take ~74 s.
- "Always allow" stores a wildcard broader than the command shown; writes land in the user's `.git`.
- It contacts npmjs for `@opencode-ai/plugin` even with the opt-outs set.

**What not to copy from the other front end studied** (full lists: section 5 of each `reference-tool-*` report):

- Answering terminal prompts by sending keystrokes; regex status rules; writing hooks or trust into users' agent configs.
- Reading or writing agents' private stores (session databases, transcripts); use the protocol's load and list or nothing.
- YOLO that disables the agent's own sandbox, or set through an environment variable that `ps` cannot show.
- Forwarding `SSH_AUTH_SOCK`, `DISPLAY` or DBus to agents; `npm install -g` from a web button; `@latest` hints.
- Pasted images written into the worktree and cleaned by an in-memory timer.
- A `--help` probe on every launch as the main evidence (keep reet's captured help with integrity; add a `--version`
  check only).
- Breadth before depth (seventeen agents); unauthenticated modes and tokens in URLs.

### Pitfalls for the next session

- **CI** runs on the Zoomies runners (`zoomies-linux-x64`); tmux and Chromium are installed per job.
  - `terminal-browser` takes 17–25 of its 30 minutes: **add no fixtures or cases to that job**.
  - `phone-zoom` and `phone-dialogs` (10 minutes each) have headroom; new browser cases go there.
  - CI's Chromium (1248) has no DejaVu Sans Mono, so terminal rows differ from a local run: never assert an absolute row
    count; compare with a layout measured in the same browser.
  - The real-browser Go harness needs `LANG=C.UTF-8 LC_ALL=C.UTF-8` locally, or terminal.e2e's UTF-8 split check fails.
  - CI does not run workflows while a PR conflicts with main.
  - `home.e2e.mjs` opens the `details.home-detail` sections after each navigation; the old In flight tests were
    removed when #288 replaced the component with the SessionBoard, which `session-board.e2e.mjs` covers.
  - `mcp/test/oauth.test.mjs` needs its fixture built first in CI (the warm-up step in `image.yml`).
- **Merges:** the owner merges other PRs while one is open (#228–#241 during this work). Always `git fetch origin main`
  and merge it before pushing.
- **Git and processes:** never `git checkout -p`; commit with pathspecs; never `pkill -f` (it can match the session's own
  processes); never a shared tmux `kill-server`. The stop hook nags while subagents are still working; that is expected.
- **Commit trailers:** new commits follow the session's attribution reminder (`Co-Authored-By` and `Claude-Session`);
  every commit in this PR carries Claude Sonnet 5.5.
- **Upload wire**: the machine reads exactly the declared size and then returns one `UploadResult` frame; the controller
  waits for that frame before closing, because a websocket peer may not be able to write after a close. The host reserves
  quota under `uploadMu` and releases it while streaming; cleanup never takes `uploadMu`, or an ended session stalls the
  connector. The 24 h sweep runs at start and hourly while `Run` is alive.
- **New UI fixtures** go in `phone-dialogs`; `terminal-clipboard.e2e.mjs` and `terminal-upload.e2e.mjs` were added there.
  Run them plus `phone-dialogs`, `phone-zoom`, `sessions`, `sessions-phone` and the `terminal-browser` files in workflow
  order before the final push. The local matrix session screen is `REET_MATRIX_ONLY=session`.
- **Models listing**: only connector-owned argv, a fresh temp cwd, the launch allowlist minus `TERM`/`SHELL`, the
  fetch/autoupdate opt-outs, a 10 s process-group kill and 64 KiB stdout. Never run `--refresh` (it ignores the
  fetch opt-out) and never forward stderr. The controller caches the last good list; a failed refresh must not
  erase it. The first read is synchronous only for a machine that reports the capability.
- **Models UI**: the launch/e2e fixture must answer `/api/v2/profile/model-favourites` and
  `/api/v2/machines/<id>/models` (the bar reads both without blocking Start); `phone-zoom`/`phone-dialogs` use
  `helpers/longtext.mjs` and need the same routes. The picker's filter is pinned and the sheet body scrolls; keep it
  that way for 500 models. Star focus must be restored after the row moves groups.
- **Mixed versions:** `acpSessionId`, `resumeAcp` and the `acpHistory` capability are sent only after
  `acp-history-v1`. A slice-1 controller negotiated `acp-v1` but its strict decoder rejects those fields; never send them
  without the separate feature. `Continue in reet TUI` is offered only when the machine reports `acpHistory`.
- **reet TUI history:** a closed reet session is reopened as a **new** row with `resumeAcp`; the machine runs `session/load` and
  replays before its result. Store the `acpSessionId`, never a transcript. `CanResumeACPSessionFor` restricts the id to
  this account's closed structured sessions on that machine. Replayed pending tools are marked interrupted; pending
  approvals are not restored. The old row stays in history.
- **reet TUI:** the machine-side `internal/acpagent` starts `opencode acp` at or above `1.19.5` with
  `OPENCODE_CONFIG_CONTENT`, `OPENCODE_DISABLE_PROJECT_CONFIG=1`, a random `OPENCODE_SERVER_PASSWORD`, `task: deny` and
  the environment allowlist. It answers `fs/write_text_file` with success without writing (OpenCode already wrote the
  file), checks mode `build`, offers `once`/`reject` only, closes its own cards on cancel/timeout, and kills the process
  group on end. Do not forward raw ACP JSON to the browser; map it to `ACPEvent` and keep it inside the control frame.
- **reet TUI transport:** events are a bounded ring in `internal/terminalbroker/chat.go` and SSE at
  `/api/v2/terminal/sessions/{id}/chat`; prompts, cancel and approvals are POSTs. A browser reconnect resets and replays
  the ring. A structured session is an ordinary row with `structured=1`; it has no terminal, so do not attach to it.
- **Process used** (subagent-driven development): an implementer per task with a brief, a task review, fix rounds
  (at most five) with a scoped re-review, then a final broad review split by area and a single fix wave; mutation checks
  restore files byte-identical (`cp` before, `cmp` after); fidelity checks at 1440, 834 and 390 px recorded in
  `docs/ui-verification.md`. The full local matrix is `web/tests/matrix/agents-home.matrix.mjs` (not in CI).

**Rule for this work: every PR updates this block (state table, limits, queued work) before it merges.**

### Standing notes carried over from the moved sections

- Terminal security: a later, user-agreed security pass for privilege-capable browser terminals (a PIN
  or similar preference) is deferred; nothing of it is implemented. Read "Later: user-controlled
  terminal security preferences" in [handoff-history.md](handoff-history.md) before starting it.
- A reported terminal-stop incident (Stop/End with removal interrupting other tmux work) is unresolved;
  do not disturb the user's work or ask them to reproduce it. Never use a shared or default tmux
  `kill-server`. Details and what to collect if it recurs: "Reported terminal-stop incident" in
  [handoff-history.md](handoff-history.md).
- Shared dialog shell for phones: `web/src/lib/dialogShell.ts` and `web/src/dialog-shell.css`; how to
  adopt it is in "Phones: dialogs" in [handoff-history.md](handoff-history.md).
- Product acceptance and evidence boundaries: discovery is not install capability, shell persistence
  is not conversation recovery, configuration restore is not runtime health, and local checks are not
  a deployment; describe outcomes without competitor names (full text in the history).

## Code map: read before proposing new types or endpoints

| Area | Existing entry points |
| --- | --- |
| App/routes and the shell | `web/src/App.svelte`, `web/src/lib/shell/*` (AppShell, TopTabs, BottomTabs, MoreSheet, JumpSearch, Bell) |
| Home and projects | `web/src/lib/home/*` (Home, FleetStrip, InFlight, NeedsYou, ProjectRail, ProjectRailHost, AddProjectDialog, FolderBrowser), `web/src/lib/home.ts` |
| Launch bar | `web/src/lib/launch/LaunchBar.svelte`, `SlotPicker.svelte`, `web/src/lib/launchBar.ts`, `web/src/lib/catalogue.ts` |
| Sessions page and session screen | `web/src/lib/sessions/*` (SessionsPage, SessionScreen, SessionTabs, ContextPanel), `web/src/lib/sessions.ts` |
| Machines/onboarding | `Machines.svelte`, `MachineFleet.svelte`, `MachineSetup.svelte`, `machineSetup.ts`, `machines.ts` under `web/src/lib/` |
| Sync/fleet UI | `EnvironmentSync.svelte`, `SyncControl.svelte`, `FleetActions.svelte`, `fleet.ts`, `api.ts` under `web/src/lib/` |
| Live sync domain/execution | `internal/scan/sync.go`, `agent.go`, `restore.go`; adjacent tests |
| Desired sets/jobs/receipts | `internal/store/store_sync.go`, `store_sync_control.go`; `internal/api/sync.go`, `sync_control.go` |
| Store/migrations and route registration | `internal/store/store.go`, `internal/api/api.go`; inspect feature-local initialisation too |
| Hosted bundle validation/editor | `internal/store/store_hosted.go`, `internal/api/hosted.go`, `web/src/lib/Hosting.svelte` |
| Read-only content manifests (S1) | `internal/skillbundle`, [contract](docs/skill-bundle-manifests.md); independent of live sync/snapshot formats |
| Snapshot whole-skill collection/verification | `internal/setup/skills.go`, `skills_verify.go`, `snapshot.go`, `restore_apply.go`, `rollback.go` |
| Agent adapters/discovery/evidence | `internal/setup/adapters.go`, `discovery.go`, `installation.go`, `health.go`, config/policy files and tests |
| Setup profiles | `internal/setup/profile.go`, `internal/profilehistory`, `internal/store/store_setup_profiles.go`, `internal/api/setup_profiles.go` |
| Host shell registry and services | `internal/terminalhost/manager.go`, `manager_tmux.go`, `registry.go`, `service.go`, `readiness.go` |
| Typed private transport and broker | `internal/terminalproto`, `internal/terminalagent`, `internal/terminalbroker`, `internal/privateconn` |
| Terminal API and records | `internal/api/terminal.go`, `internal/store/store_terminal.go` |
| Terminal UI and connection state | `web/src/lib/terminal/TerminalWorkspace.svelte`, `TerminalPane.svelte`, `api.ts`, `connection.ts` |
| Desks: record and rules | `internal/desk` (`Doc`, `Tab`, `Node`, `Pane`, `Validate`, tree edits, presets); `internal/store/store_desks.go` (tables `desks`, `desk_sessions`, `desk_pane_dirs`; `Desk`, `CreateDeskFor`, `UpdateDeskFor`, `ReopenDeskPaneFor`, `NoteDeskPaneDirFor`, `DeskPaneReportedDirFor`); [docs/desks.md](docs/desks.md) |
| Desks: routes and machine side | `internal/api/desks.go` (`/api/v2/desks…`, reopen); `internal/terminalproto/desk_shell.go` (`desk-shell-v1`, `DeskRef`); `internal/terminalhost/desk_shell.go` (wrappers, pane history, `ForgetDeskSession`), `live_dir.go`, `launch.go` (`ResolveDeskDir`); `internal/terminalbroker` (`EndDeskPane`, `recordLiveDir`) |
| Desks: page | `web/src/lib/desks/*` (DesksPage, DeskList, DeskScreen, DeskTabs, SplitArea, DeskPane, DeskToolbar, PaneStateCard, `paneState.ts`, `autosave.ts`, `tree.ts`, `keys.ts`, `typeInAll.ts`, `addSession.ts`, `api.ts`); `web/tests/desks.e2e.mjs`, `desks-phone.e2e.mjs` |
| OpenCode in the reet TUI | `internal/acpagent` (agent spec, process, config, env, permissions), `internal/terminalagent/chat.go` (dispatch), `internal/terminalbroker/chat.go` (ring/commands), `internal/api/terminal_chat.go` (SSE and POSTs), `web/src/lib/chat.ts` and `terminal/ChatPane.svelte` |
| Controller enablement | `cmd/reet/terminal.go`, `internal/terminalcontrol`, `docs/terminal-controller-commands.md` |
| Machine health | `internal/agenthealth`, `internal/store/store_remote_health.go`, `internal/api/health.go` |
| Notifications | `internal/notify`, `internal/store/store_notify.go`, `internal/api/notifications.go`, `web/src/lib/Notifications.svelte` |
| Agents home fixtures | `web/tests/home.e2e.mjs`, `launch.e2e.mjs`, `add-project.e2e.mjs`, `sessions.e2e.mjs`, `sessions-phone.e2e.mjs`; the local matrix `web/tests/matrix/agents-home.matrix.mjs` |
| Existing regression fixtures | `internal/store/store_sync_control_test.go`, `web/src/lib/fleet.test.mjs`, `web/tests/control.e2e.mjs`, `web/tests/terminal.test.mjs`, `web/tests/terminal.e2e.mjs`, `internal/api/terminal_integration_test.go` |

Names such as `Skill`, `SyncSkill`, `SyncTarget`, `SyncPlan`, `SetupSkill`,
`SnapshotSkill`, `HostedSkill`, `TerminalSession` and `SyncAgent` already exist.
Read their actual contracts before adding a similarly named concept. Especially,
`SyncPlan.Hash()` hashes the planned actions; it is not an installed-content hash.

## Safety and conflict handling

- Restore verification uses format-specific limits: configurations 1 MiB, whole
  skill folders 8 MiB, supported memory databases 64 MiB. Capture/restore agree.
- An action is marked as written before verification, so a verification or journal
  persistence failure includes the current action in rollback. Write errors propagate.
- Codex memory restore/verification uses the destination's effective CODEX_HOME.
  SQLite sidecars are rechecked immediately before writes. Destination agents must
  be stopped; sidecar absence does not prove an application is inactive.
- Memory verification continues after a blocked database. Each skill produces one
  final result; an unsafe file cannot also make the skill count as verified.
- The restore wizard prints mapped paths and warnings, asks per-destination conflict
  choices and confirms the final plan. Incomplete file verification returns an error, including the standalone verify command; JSON reports are still emitted.
- Server profile writes require an identical history or an exact append, with an
  atomic payload comparison protecting concurrent writers. Stale/divergent pushes
  return 409. Web edits carry the viewed base revision; stale edits are rejected.
- Local pulls preserve divergent/newer local histories. Preserve a local copy with
  `reet setup profile rename NAME NAME-local`, then pull the server copy and review.
  Local saves/pulls/rename/remove share an exclusive `profiles/.write.lock`.
  Remove a stale lock only after confirming its owning process has stopped.
- A failed profile deletion can be retried without reloading the application.

## Installation and release changes

The shell installer verifies checksums, runs the staged candidate's version before
replacement, preserves a regular existing binary as `reet.previous`, and reports
PATH shadowing. It refuses an existing symlink rather than silently replacing it.
Existing data and managed deployment files remain separate.

Dev publication keeps legacy rolling names for existing clients, and also uploads
commit-addressed archives `reet_dev_SHA_OS_ARCH.tar.gz` with `checksums-SHA.txt`.
Those assets are checked on a rerun and never overwritten. Only after they are
available does the publisher update `dev-build.txt`. New shell installers and Go
upgrades resolve that pointer once and download the corresponding immutable pair.
Only a missing (404) pointer falls back to legacy releases; other errors fail closed.
No assets, tags, deployments or production hosts are published/upgraded by local tests.


## Accepted product direction and priority

The user wants reet.sh to be the central control unit for multiple machines:
maintain skills on reet, connect machines, import local skills, distribute to one,
selected, or all machines, review changes in both directions, and use installed
AI agents through the web terminal. Agent software install/update/remove and
cross-machine configuration management are future requirements. The user accepted
the central-control roadmap on 24 October 2026 and requested this detailed handoff.
This records the accepted direction; this turn authorises the handoff update,
not implementation of every milestone or production changes. Future sessions
should implement the task requested within this direction, and verify its gate.
Later steering prioritises the terminal workspace as the main everyday use case.

Priorities, in order (terminal UX moved first by the user on 24 October):

1. **T — Make terminals the primary usable workspace:** viewport layout, clear
   session navigation/controls and safe history maintenance (T1–T3). T1–T9 are
   merged (see handoff-history.md).
2. **S — Complete the skill foundation:** versioned bundles, content-level drift,
   reviewed local edits, safe one/many-machine transfer and verification.
3. **A — Make remote work convenient:** registered agent capabilities, project
   selection, explicit launch, semantic activity and an attention queue.
4. **W — Make parallel work reviewable:** managed worktrees, files/diffs and
   structured agent interaction where supported.
5. **L — Fleet lifecycle:** observed agent versions, reviewed install/update/remove,
   desired MCP configuration and optional isolated agent sessions.

Conversation resume/fork, granular machine/project permissions, read-only terminal
observation and OIDC login are valuable later extensions. A full terminal TUI,
themes, sound effects and a broad plugin marketplace are lower priority than the
outcomes above. Keep the warm Reet branding, Yorkshire rose and familiar
navigation; read [voice and copy](docs/voice-and-copy.md) for user-facing changes.

The older setup plan records an earlier delivery order (backup/restore, profiles,
maintenance). Preserve its data-safety and recovery requirements. This roadmap is
the newer ordering for the central-control product, with terminal UX now first;
do not attempt complete setup recovery before the bounded workspace/content tasks. Do not label partial file
recovery as a fully working recovered agent environment.

## Execution rules for the next session

1. Read this file, CONTRIBUTING.md, applicable AGENTS.md files, and the entry-point
   files for the single task selected below. Check the actual checkout; paths and
   branch state here are a dated baseline, not permission to reset local work.
2. Run `git status --short`, inspect the current branch/log and fetch the current
   base when network/authentication are available. Preserve existing edits.
   Confirm the previous slice's state through its live PR/CI/merge record, then start
   from current main and preserve local edits.
3. Pick **one bounded next slice**: the open agents-home work above, or from the
   continuation lists in the S4 records (now in handoff-history.md) and
   docs/skill-content-receipts.md; the user chooses the order. T1–T9, S1–S3 and the
   S4 planner, pins, preview, ownership receipts and review UI are done; do not restart them. Do not build an execution
   control before its prerequisite is present or turn the whole roadmap into one PR.
4. Before editing, state the task ID, files affected, expected behaviour and checks.
   Resolve routine implementation choices from existing conventions. If a domain
   decision genuinely remains open, record the question and continue independent
   work; never invent an unsafe default just to make a control appear functional.
5. Keep domain code separate from HTTP handlers, SQLite records and Svelte views.
   Reuse existing parsing, bounds, ownership, locks and recovery code where its
   contract fits. Do not copy a whole subsystem or introduce a dependency casually.
6. Add meaningful regression tests for new runtime behaviour and failures. Use
   synthetic homes, databases and fixtures; do not enroll or alter real machines
   or invoke a real paid AI provider as a test side effect.
7. Show observed/unknown/stale/incomplete/unsupported states distinctly. A request
   accepted by the controller is not an observed successful change on the host.
8. Update this file with completed task IDs, commit/PR, checks, limitations and the
   exact next task. Keep the stage-per-PR workflow and required CI. Do not merge,
   publish a release or change production merely because local tests pass.

### Terms that must not be conflated

| Term | Meaning |
| --- | --- |
| Reet connector / `reet agent` | Background machine enrollment, reporting and command transport; not Claude Code or Codex |
| AI agent application | A CLI such as Claude Code, Codex or OpenCode |
| Terminal session | A persistent shell; `running` does not mean an AI agent is working |
| Agent run | Proposed record binding a terminal/runtime to a registered AI agent, machine and project |
| Skill scope | A particular agent/global/project destination; several scopes may share physical files |
| Library membership | Personal shelf/list organisation; not install ownership or deletion approval |
| Desired set | Saved installation intent for a target; not a verified filesystem observation |
| Bundle revision | Proposed immutable skill contents and provenance; not a shelf row timestamp |
| Setup profile | Existing revisioned snapshot/exclusion workflow; not automatically a live agent launch profile |
| MCP endpoint | Reet's own API for clients; distinct from configuring MCP servers used by other agents |

## Validation commands and interpretation

For documentation-only changes, check Markdown links, whitespace and consistency
with the inspected code; do not rerun the entire application suite just to validate
prose. For implementation, run targeted regression tests while developing, then
required repository checks before PR handoff. Use tools from the current environment;
never assume a previous session's binaries or caches are present.

Prerequisites: versions from `go.mod` and `web/package.json` (at this baseline Go
1.27.1 and Node 22), dependency installation when missing, tmux 3.2+ for terminal
service/integration tests, and a supported Chromium for browser tests. Read the
managed environment runtime instructions before network/container work. Never
print environment secrets, commit credentials or change global system directories
just to reconstruct this previous session's environment.

From the repository root, run sequentially:

```sh
(cd web && npm run check && npm run build)
go vet ./...
go test -race ./...
(cd mcp && npm test)
git diff --check
```

**Finish the web build before starting Go builds/tests.** Go embeds `web/dist`;
concurrent Vite cleaning/rebuilding can produce missing embedded asset failures
that are test-environment races, not application regressions. Keep inputs stable
while checking. The full API race package can take several minutes on cold caches;
do not remove timeouts, skip checks or assume a silent process is hung.

Examples of narrow checks (add the relevant new package/tests for the chosen task):

```sh
go test ./internal/setup ./internal/scan ./internal/store
go test -race ./internal/setup ./internal/scan ./internal/store
```

For the existing synthetic Control browser contract, start the dev server in a
separate terminal and keep it running:

```sh
cd web
npm run dev -- --host 127.0.0.1 --port 4173
```

Then, from the repository root:

```sh
node web/tests/control.e2e.mjs
node web/tests/lists.e2e.mjs
node web/tests/community.e2e.mjs
node web/tests/spaces.e2e.mjs
node web/tests/setups.e2e.mjs
node web/tests/inbox.e2e.mjs
node web/tests/admin-users.e2e.mjs
node web/tests/admin-team.e2e.mjs
node web/tests/admin-tickets.e2e.mjs
node web/tests/admin-email.e2e.mjs
node web/tests/admin-notifications.e2e.mjs
node web/tests/admin-backups.e2e.mjs
node web/tests/admin-settings.e2e.mjs
node web/tests/admin-activity.e2e.mjs
node web/tests/account-sessions.e2e.mjs
node web/tests/account-tokens.e2e.mjs
node web/tests/account-profile.e2e.mjs
node web/tests/account-security.e2e.mjs
node web/tests/account-mcp.e2e.mjs
node web/tests/skill-card.e2e.mjs
node web/tests/machine-add-skill.e2e.mjs
node web/tests/sync-remove.e2e.mjs
node web/tests/machines-search.e2e.mjs
node web/tests/terminal-workspace.e2e.mjs
node web/tests/terminal-lifecycle.e2e.mjs
```

`REET_UI_URL` can select another dev URL. `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH`
selects a browser (the current fixture otherwise uses `/usr/bin/chromium`).
`REET_UI_SCREENSHOTS` is an optional output directory for synthetic screenshots.
This fixture mocks APIs; pair it with real domain/API/host tests for new effects.

For real terminal browser regression, follow `.github/workflows/terminal-browser.yml`
and the integration fixture, after building web and making tmux/browser available:

```sh
REET_BROWSER_TESTS=1 go test -race -count=1 -timeout=3m ./internal/api -run '^TestTerminalPrivateLifecyclePreservesActualShell$' -v
```

This exercises a real local shell/broker/PTY with test transports. It does not
establish a production relay deployment, cross-OS recovery, or a live fleet rollout.
Physical macOS service-manager and supported cross-OS checks remain separate gates.

### Record after every implementation task

Replace the relevant unchecked task with its completed state only after recording:

```text
Task ID and scope:
Branch / commit / PR:
Behaviour added:
Compatibility / migration behaviour:
Checks actually run and their results:
Platform or live-environment checks not performed:
Known limitations or unresolved decisions:
Exact next unchecked task and first files to read:
Publication / deployment state (verified, not inferred):
```

If a task cannot meet its acceptance gate, leave it incomplete and describe the
failing case and recoverable state. "Code compiles", "API returned 200", a mocked
success response or a green screenshot does not establish successful remote writes,
working agent authentication or complete environment recovery.
