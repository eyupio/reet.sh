# Changelog

Changes that matter to someone running a reet controller, newest first. Before
this file existed releases were described on their GitHub release pages.

## Unreleased

- You can add a project from a GitHub repository that isn't on any machine
  yet. **Add** in the project rail now offers **From GitHub** beside **Folder
  on a machine**: write `owner/name` or paste the github.com link, choose the
  machine, and reet clones it there into `~/reet/repos/owner/name` and adds it
  as a repository project, with the repository's default branch as its base.
  The machine clones with its own GitHub CLI sign-in, so it needs a current
  reet, git, `gh auth login` done there and terminal access on; a sign-in held
  only in a `GH_TOKEN` variable or a custom gh config folder isn't seen. No
  GitHub credential reaches the controller. A clone can take minutes: you can
  close the dialog, and the project appears in the rail when it's done. The
  project records which repository it came from, and a project reet cloned
  itself is not asked where a session starts (a clone of the same repository
  that was already there, and that reet didn't make, is used as it is and is
  still asked); what a session started from is recorded on the session and in
  Activity. Editing the project to another machine or folder clears where it
  came from. The clone is a full, normal one with its tags; Git LFS files stay
  pointer files in the clone's own folder, while other content filters in the
  machine's git settings still run. A machine clones one repository at a time,
  a dropped connection or a controller restart ends a clone in progress (adding
  the repository again picks up whatever finished, and the page says it lost
  track of the clone), a git setting that sends github.com over ssh stops the
  clone and says so, and an empty repository is added with a blank base.
  Removing the project leaves `~/reet/repos/owner/name` on the machine; the
  hidden folder a clone cut off part-way leaves is removed the next time that
  repository is cloned.
- A machine no longer loses its connection part-way through starting an agent
  session. The controller ended a machine's connection whenever the machine
  sent a kind of message missing from its list of accepted answers, and the
  stage notes of a starting session, the answer to a base check, and the
  answers to a branch sync, a branch prune and a software update were all
  missing. An answer that arrives after its request gave up is now dropped
  instead of ending the connection too.
- A machine waiting for its controller to finish moving keeps a readable log.
  The agent says once that the controller has moved, and again only if the
  address changes, where it printed four lines every five seconds; it also
  asks the old controller once per cycle, not four times.
- A starting session now reports its stages to the controller as it goes.
  The machine sent each stage note with the session's id attached, which the
  private protocol refuses, so every note was dropped before it left the
  machine and the stages a start had reached were never recorded.
- A session started in a saved repository now begins from the base branch as
  GitHub has it. The machine fetches that branch before it makes the session's
  worktree and starts from GitHub's commit, unless the branch on the machine has
  commits GitHub doesn't have, in which case it starts from the local branch;
  when GitHub can't be reached it starts from the last known commit and nothing
  in your checkout is touched. Agents no longer start work on a branch that was
  days behind. The fetch uses that machine's own git credential helpers and its
  `gh` sign-in, for github.com only: a private repository needs one of them to
  work there (for gh, `gh auth login` on that machine), and no credential ever
  reaches the controller. A repository whose origin is an ssh URL
  (`git@github.com:owner/repo.git`) is fetched over https with gh, and is not
  fetched at all on a machine with no gh — it then starts from its own branch,
  with nothing said about GitHub. A machine running an older reet is unchanged:
  it fetches nothing and starts its worktrees exactly where it always did.
- The branch is the project's own base branch. Your account's default base
  branch still applies where the browser sends it, as it always has; the
  controller does not fill it in, so a repository that calls its branch
  something else is never refused for a default meant for another one. With no
  base named at all, the machine starts from the branch that repository's remote
  calls its default. The browser asks its question about exactly the branch the
  session will start from.
- The sync that follows removing a session's worktree keeps working on a machine
  whose `gh` is signed out, or signed in as an account without access: the
  credential helpers your own git configuration names are asked as they always
  were, with gh offered alongside for github.com.
- Where a session started from is recorded in Activity for every start, not only
  the ones made in a browser: a Project run, a start in several projects and a
  session whose machine answered late all get the same line, worded like the
  question the browser asks.
- When the two sides differ, every start you make in the browser says so and
  asks before anything starts: "main on studio has commits GitHub doesn't have.
  This session starts from the local main (abc1234)." That is Home's launch bar,
  the Start an agent dialog (the project rail's +, the machine cards, Machines
  and the Sessions screen) and Start on a backlog plan. Cancel starts nothing. A
  base that matches GitHub, or that the machine has no branch of its own for, is
  not worth a question, so nothing is shown, and a
  machine that can't answer the question — an older reet, a busy one, or no
  answer within ten seconds — never holds the start up. The sentence is kept with
  the session and shown on its Overview, under Where it's working.
- A restore can give the copy an owner you can sign in as. `reet backup
  restore --make-owner ADDR --owner-password-file F` makes an account already
  in the copy its owner with the password in the file, for a copy from a
  controller whose sign-in the new machine does not have or whose owner
  belonged to whoever operated it before. The owners the copy had become
  admins, and an address the copy lacks or holds disabled is refused before
  anything changes.
- Machines follow a controller that has moved. When the old controller is
  fenced with the new address (`reet backup fence --moved-to NEW`), each
  running agent learns the address from the refusal, has the new controller
  prove it holds the machine's record, checks its credentials are accepted
  there, and only then saves the address and restarts. A place that cannot
  prove the record is sent no credential. `reet agent follow-moves off` keeps
  a machine where it is until `reet agent set-server` is run on it.
- **A controller can be moved from the Backups page.** In the **Move** section of
  **Admin → Backups** the owner now has **Move this controller away**: fence the controller (with an
  optional new address, and a dialog that says what stops and that it can be
  undone), take the final copy sealed with a password typed for that one copy,
  download it, copy the `reet agent set-server` command for the machines, and
  lift the fence at any time. The final copy is only offered once the fence is
  up, and the page says whether the copy was really taken fenced. **Import a copy
  from another controller** is now **Move a controller in**: it takes the address
  the controller answers at (filled in from the page's own address), asks for
  **The old controller is stopped or gone** only after the controller has refused
  a copy for that reason, shows a refusal beside the form with what was typed
  kept, and its report now says what was done about the address, what was held
  back and what is not in the copy. A fenced controller says so in a banner on
  Backups and in one line under the header on every other page. Each piece is
  offered only when `GET /api/version` lists its feature.
- For the page above: an admin may now read `GET /api/v2/admin/fence` (raising
  and lifting stay the owner's); a copy in the backup list and the answer of a
  run carry `fenced` and `protection` from the copy's header; an import's answer
  and the pending import carry `notInCopy`; and an import refused because its
  source was live, or because the controller already holds data, carries a
  `code` (`copy_source_live`, `controller_not_empty`).
- The **+** beside the session tabs now asks what to start, the same choice as
  **New session** (an agent session, a terminal, or a Factory outcome), instead
  of assuming an agent session and sending you to the launch bar.
- **Ending a session that a Factory pipeline is working in now says so first.**
  The End and Remove from history dialogs name the pipeline and stage and what
  ending it costs. Afterwards the pipeline's card says its session was ended
  and offers what actually works: run the stage again in a new session in the
  same folder and branch, move to the next stage, or stop following. It no
  longer offers to tell an agent that is not there.
- **Try again now recovers a pipeline whose session is gone.** It used to try
  to follow the dead session again and fail, or refuse because the closed goal
  was still there. It starts the stage in a new session, or follows the session
  again if it is still running.
- **The controller ends sessions a pipeline has finished with**, keeping their
  history: the session a stage leaves when the next stage gets one of its own,
  and the last session when every stage is done. Worktrees are not touched.
- **A stopped outcome restarts only what you ask for.** A pipeline shown as not
  started could not be tried again, and trying another one again, adding a
  pipeline or approving a merge started every pipeline that had been waiting.
- Moving a pipeline on is refused if it has already left the stage you were
  looking at. A Cline stage that is sent back keeps its billing source. An
  outcome that names Google Antigravity without folder trust is refused when it
  is approved, instead of failing at that stage.
- An outcome whose pipelines are still running is no longer dropped when the
  account passes thirty outcomes.
- **MCP and Gaffer's own tools:** Factory changes are refused on a fenced
  controller, as they are in the browser. `reet_tell_pipeline_agent` needs
  `confirm: true`. Telling an agent, adding pipelines and sending work back are
  refused for an outcome that runs on YOLO and stay Factory-page actions.
  `reet_stop_outcome` now stops following the outcome instead of reporting
  success when nothing was being started; `reet_move_pipeline_on` takes the
  `stage` it is moving on from. Update the stdio MCP package with the
  controller.
- Factory page: what an action did, or why it was refused, stays in view and
  is announced as a problem when it is one; a refusal of Tell the agent, Send
  back or Add pipelines shows inside its dialog; a failed first read is no
  longer shown as an empty factory; proposals in a report start ticked; a long
  pipeline on a phone names the stage in hand instead of overlapping eight
  names; a session's Factory badge opens its pipeline.
- **An approval nobody answers no longer leaves a goal looping.** A request
  expires after five minutes and stops the agent's turn; the goal now says so
  instead of "You answered", and resuming it has Gaffer review where the agent
  stopped and set it going again.
- Outcomes are carried on side by side: one slow launch no longer holds up
  every other outcome on the controller.
- A session whose worktree a Factory pipeline still works in is kept in
  history, with the reason, until the pipeline finishes or its outcome is
  stopped and archived.
- A call Gaffer asked your approval for is not run if it has waited more than
  a day. Gaffer's own Factory read returns handover notes and reports only to
  a provider allowed session text. An outcome's History includes what was done
  to it over MCP. `reet_start_outcome` takes a `model` and refuses an agent
  the project's machine cannot run.
- Factory card: **Who did each stage** lists each stage's agent, model, who
  closed it and its handover. The outcome dialog says Pipeline, not Session.
- The Factory page has the project list down its left side, as Home does.
  Choosing a project (or a group) shows only its outcomes, and **Give Gaffer an
  outcome** then opens on that project; a project's **+** does both in one
  press. **Other sessions** follows the chosen project too. On a tablet or
  phone the page is unchanged.
- **New session** on Home, a machine card's **New session…** and a project's
  **+** now ask what to start, as the Sessions page does: an agent session, a
  terminal session (in the project's folder when one is named), or a **Factory
  outcome**. The Sessions page's chooser offers the Factory outcome too.
- The scheduled worktree clean-up is now on unless an admin turns it off, so
  the clean worktrees of finished sessions are no longer left behind. It still
  removes only worktrees with nothing unsaved, and its default wait after a
  session finishes is now 1 day, down from 7. A controller where it was
  switched off stays off, and a saved wait is kept; one that never saved these
  settings starts sweeping, with the 1-day wait, after this update.
- **Apply retention now** on Admin, Backups no longer fails with "Cannot read
  properties of null (reading 'length')" when there is nothing to remove. The
  controller now always answers with empty lists, and the page copes with the
  old answer too. The page is also split into Overview, Copies, Destinations,
  Schedule, Runs and Import sections, so it no longer runs to several screens.
- A Factory outcome can no longer get stuck on a session that is gone. When a
  followed session is removed (for example ended with its history removed), the
  controller now notices within seconds, stops following that pipeline and
  closes its goal, so the outcome can be archived or deleted and the pipeline
  retried. An outcome that is still moving also has **Stop following**, which
  ends every unfinished pipeline without touching sessions that still exist,
  so any stuck outcome can be put away.
- The Factory can now put finished work away. A finished outcome (nothing
  moving) has **Archive**, which hides it from the page and the headline
  numbers but keeps the record, its report and its history, and **Delete**,
  which asks first and erases the record for good. **Show archived** brings
  archived outcomes back, with **Restore**. Which of the two stands out is
  your choice under Account → Profile; it defaults to archive. Neither touches
  the sessions or pull requests an outcome started.
- A finished outcome is one line on the Factory until opened, and its long
  report is clamped with **Read all of it**, so old work no longer fills the
  screen. Outcomes still moving, or with a pipeline to try again, stay open.
- Every outcome on the Factory can now be folded and reopened with **Hide
  details** / **Show details**, including one still moving, and the folded line
  says how many pipelines it has and whether they are in progress, finished or
  all done. **Expand all** and **Collapse all** now cover outcomes as well as
  their pipelines.
- **Open session** on a Factory pipeline, and in the outcome dialog, now opens
  the session itself rather than opening Gaffer's panel beside it.
- A person-pressed model refresh asks an up-to-date machine for a fresh
  listing instead of accepting its short cache, so the list a person sees after
  Refresh is the list the agent reports now.
- An agent software install started from the Software page keeps running if
  the controller's private connection drops; a later reconcile observes the
  version it left.
- Sync no longer stops a whole machine folder over one skill. When another app
  on the machine already has a skill of the same name from a different source,
  reet now leaves that one skill alone, says so with the reason, and syncs
  everything else. It used to fail the folder with "… is shared with agents
  from a different source; resolve it before syncing", and **Auto fix** then
  answered "No installed source conflict left to fix" for any skill reet holds
  as its own copy, so nothing could clear it. A machine needs the new reet
  build for this; until it has it, the failure is described in plain words, and
  **Auto fix** is offered where the failure is shown: the Sync activity panel,
  the machine's row on Sync and the machine card. When Auto fix cannot settle
  it, it now says why and what to do instead. A finished sync lists any skill
  it left alone, and the machine shows "1 skill left alone" in place of
  "Everything in sync".
- Gaffer settings: the list of tools Gaffer can use is quicker to set. **All
  tools** sets every tool at once, each group has its own set-all, and each
  tool has a small three-way control (Allow, Ask me, Off) instead of a
  dropdown. A group that is mixed reads "Custom". Descriptions open from the
  tool's name, and a group can be folded away.
- Factory: a pipeline that **needs you** now says why in one sentence, shows
  Gaffer's review, and asks "What do you want to do?" with buttons for the
  things you can do (have Gaffer look again, tell the agent, move on, open the
  session, try a failed review again, give it a new budget). Gaffer's review is
  shown as formatted text, not raw markdown. The numbers at the top that want
  you are buttons to the first such pipeline, and the usage and budget figures
  are folded away under the card.
- Factory: each pipeline is now one line (title, stage, status and why) that
  opens to its full card; pipelines that need you, are ready to merge or have
  stopped start open, and **Expand all** opens the rest. Stage and
  status colours say more: a green tick is done, a pulsing green ring is
  running now, amber with "!" is waiting for you, red has stopped. The motion
  is switched off for anyone who asks their system to reduce it.
- Home: **Factory pipelines** lists every pipeline with the outcome it belongs
  to, its stage and why it stands where it does, the ones that need you first.
  A line opens that pipeline's card on the Factory page.
- Factory fixes: a stage that carries on in the same session is now given its
  instruction by the controller (it was left to Gaffer's first review, so an
  agent could sit idle at a stage nobody had told it to start); **Tell the
  agent…** was always refused with a misleading "provider changed" error and
  now works; and a review in which Gaffer chose no next step says so instead
  of "Gaffer needs your decision". Needs a controller update.
- Gaffer can use reet's own tools, the same ones an MCP client gets, in your
  chats with it. **Gaffer settings → Tools Gaffer can use** lists every tool
  with three choices: **Off** (the default for all of them), **Ask me** (Gaffer
  shows the exact call in the chat and it runs only when you approve) and
  **Allow** (it runs straight away). This includes reading the Factory and the
  activity log, moving, retrying, sending back and stopping pipelines, starting
  an outcome, approving a merge, and the shelf and machine tools. Switching a
  tool to Allow lets Gaffer do that on your word with no further click; goal
  reviews never get these tools, and in a session's chat anything that changes
  something always asks first. Every use is in the activity log. Needs a
  controller update.
- Gaffer: a stage review now reads the working agent's latest reply at length
  (up to 9 KiB), not a 2 KiB fragment, so it can see a plan, design or final
  report and hand the stage over by itself. Needs a controller update.
- Factory: each pipeline card shows how long the stage in hand has run and
  what its agent last reported doing and when; a stage you moved on yourself is
  marked differently from one Gaffer handed over; **History** on an outcome
  lists what happened to its pipelines and why; and a Gaffer review that fails
  says why and offers **Try the review again**. Needs a controller update.
- Factory: **a YOLO pipeline no longer merges by itself unless you confirm
  exactly that.** A pipeline whose agent runs on YOLO stops at Ready to merge
  even when set to merge by itself, until you turn the switch on from its card
  and confirm. Pipelines approved with both before this update stop too.
- Activity log: a scan that found nothing new is recorded once for the same
  counts and again after six hours, not every minute. The events read (and
  MCP's `reet_list_events`) can leave sources out with `exclude`.
- MCP: `reet_list_factory` returns the controller's build and each machine's,
  and each stage says when it started, who closed it and what its agent last
  reported.
- When the Library says a machine has a different version of a skill, **See
  what differs** now lists the files: which are changed, which exist only on
  that machine and which only in reet's copy, with their sizes. The machine
  reports names, sizes and fingerprints only, so no file contents leave it for
  a comparison, and it cannot show the changed lines inside a file. Choosing
  **Use that machine's version** afterwards takes the files it just listed. An
  answer arrives a few seconds after you ask, while the machine is online.
  Needs a controller update.

- Gaffer: a goal no longer looks active when nothing can happen without you.
  If Gaffer's instruction is still undelivered after ten minutes and the
  working agent is not busy, or the agent has sat idle for thirty minutes after
  Gaffer's last review with nothing queued for it, the goal goes to **Needs
  you** and says which. A busy agent is still waited for, and an agent waiting
  for your approval is held as before. Needs a controller update.
- Activity log: what Gaffer and the controller do to a Factory pipeline by
  themselves is now recorded, under the source `gaffer`: a stage handed over
  (and whether Gaffer's review, your button or your word in the chat did it), a
  stage started, a pipeline waiting for your merge decision, done or stopped,
  and a goal that needs you, is held while the agent waits for your answer, is
  released, or runs out of budget. Each carries the outcome, pipeline, stage
  and the controller's reason, so there is a history of why a pipeline moved or
  stopped. Reasons are the controller's own wording; handover notes stay on the
  stage. MCP's `reet_list_events` can filter `source=gaffer` and
  `source=machine`. Needs a controller update.
- Activity log: YOLO approvals no longer write a row each. The approvals of one
  turn of a session share one `yolo-allow` row with a `count`, the first and
  last request and the time span (a turn longer than ten minutes starts another
  row). Every approval is still counted and saved as it happens. Older rows are
  unchanged and count as one. Needs a controller update.
- Factory: a goal whose session is archived or no longer available can now be
  removed from the page. The "Conversation history" button shown there did
  nothing on the Factory page.
- Factory: **Agent tokens** shows what the working agents themselves report
  using, per pipeline and per outcome with a split by agent, and **Context
  left** shows how much of the stage in hand's context window remains. Agents
  that report nothing read "Not reported". Needs both a controller and a
  machine update.
- Factory: **Tell the agent…** sends your instruction to a pipeline's working
  agent, and **Send back…** has a stage done again with your reason. MCP can now
  also start an outcome, add pipelines, tell a pipeline's agent and send it back
  (management scope, `confirm: true`; never YOLO). Needs a controller update.
- MCP: eight Factory tools. Any writer's connection can read outcomes and
  pipelines (`reet_list_factory`, `reet_get_factory_outcome`); a management
  connection can also move a pipeline on, retry it, set or approve its merge,
  stop an outcome and ask Gaffer for a report. Needs a controller update.
- Factory: you can move a pipeline on yourself with **Move to next stage**, or
  by telling Gaffer "continue" in that session's chat, where Gaffer can now also
  pass your instruction to the working agent. Each pipeline has a switch for
  whether its merge waits for you. A stage's agent is told its stage and where
  to stop before the whole brief, so a plan stage no longer does the coding.
  Needs a controller update.
- The **Sync** page is simpler. It now shows one status sentence, a row for
  each machine and recent activity, and nothing else competes with that
  sentence. A plan waiting for your review, a problem on one machine or an
  update that just finished is said on that machine's row. The sync-plan
  controls (plans that follow a list or keep an exact set, plan revisions,
  **Scan all**) have moved, unchanged, to their own page, **Advanced sync**
  (`/sync/advanced`), linked once from Sync. Existing plans keep working and
  older links such as `/sync?machine=…` are sent on to the new page. The second
  copy of the skills picker has gone from Sync: a machine's skills are chosen
  in its **Skills on this machine** dialog. A connected machine nobody has
  chosen skills for now says "No skills chosen for … yet" and where to choose
  them; it used to say "Syncing your machines…" for as long as that lasted.
  Needs a controller update for a reload of `/sync/advanced` to work.
- Factory: Gaffer can report across an outcome by itself when a pipeline
  finishes or stops (your choice when approving, bounded, proposals only).
  Effort choices are now the agent's own levels as its machine reports them, so
  OpenCode can be given one. An added pipeline can have a crew, and adding
  pipelines is no longer refused because another pipeline moved. Needs a
  controller update.
- Factory: when you approve pipelines Gaffer proposed, each one can be given its
  own agent, model and effort. Left alone it uses the outcome's.
- The Library now tells you when a connected machine holds a different version
  of a skill than the copy reet has saved, and lets you choose. **Use that
  machine's version** saves its files as the skill's next version, in the same
  entry, so every machine the skill is selected for receives it. **Keep reet's
  copy** leaves reet's copy as it is and stops flagging that machine until its
  files change again. Nothing changes until you choose. A machine that still
  has an older version reet saved is not flagged: it is only behind. Adds
  `differences` to `GET /api/v2/library` and
  `POST /api/v2/library/{item}/difference`. Needs a controller update.

- Factory: a pipeline no longer stops for good when its agent asks for an
  approval. It shows **Needs you** with the reason and carries on by itself once
  you answer. An outcome can be approved to run on **YOLO** (off by default,
  confirmed each time, only where the account, machine and agents allow it). The
  agent lists now offer the machine's real **models**, and an **effort** for the
  outcome or per crew role. Home marks a session that
  is part of a pipeline (**Factory · stage n of m**). Needs a controller update.
- The skills you own are now called your library everywhere you read about
  them. The same collection used to be called the shelf, your skill shelf, My
  shelf, your personal shelf and your private toolkit, depending on the page;
  buttons now read "Add to library" and "Remove from library", and the tools
  and layout switch are "Library tools" and "Library layout". The status "On
  the shelf" is now "Parked", including in the Markdown and CSV exports.
  Choosing which skills a machine has is one button with one name, "Skills on
  this machine", on the machine card, in Home's machine menu and on Sync; the
  second menu entry that opened the same dialog is gone. The dialog explains
  that each AI tool keeps its skills in its own folder and opens on the first
  folder instead of an empty choice. "Sync to machines…" on a skill card, which
  led to a page that said everything was in sync, is now "Choose machines" and
  goes to Machines. Words only: nothing stored on the controller, no setting
  and no API field or value changes, and exported file names stay the same.

- reet now saves a copy of every skill a connected machine shares, by itself.
  When a machine checks in, the controller asks it for the files of each skill
  the library lists without content, and the copy fills that same library entry:
  one entry per name, with its status kept. Before, such skills waited on
  "Waiting for complete content" until someone imported them by hand, and that
  import added a second entry of the same name. Nothing is installed anywhere
  as a result; choosing machines stays your decision. A copy with files that
  need a look still stops for review, and reet tries a skill three times before
  leaving it to a manual import. Needs a controller update.

- A skill you selected for a machine in the library is no longer described as
  "Not set to sync" on the Skills page, and a machine is no longer described as
  "Everything in sync" while a selected skill cannot be installed. The Skills
  card, the machine card and Sync now read the library selection: they say the
  skill is selected, which machine it is for, and why it is waiting (reet has no
  saved copy yet, it is not In use, or two selected skills share a name). Sync
  state gains `held` on each target. Needs a controller update.

- `reet agent status` and the terminal connection wizard now say why a machine
  is not syncing or connected: the controller's refusal or the network error,
  what to do next, and where the log is. Before, the wizard only said the
  private connection had not become ready. In the `reet` terminal home, commands
  that prompt (Upgrade, Install, Connect machine, Sync and others) now run on
  reet's own screen and return home, without dropping into shell scrollback, and
  **Connect machine** on a machine that is already connected says so. Machines
  need the new build to report reasons; no controller update is needed.
- Factory: each outcome and pipeline now shows what Gaffer's own model has used
  (reviews and tokens) and what is left of the stage's budget. Tokens only; reet
  attaches no prices and does not see the working agents' usage. **Ask Gaffer for
  a report** gives one account across an outcome's pipelines and may propose
  pipelines, which are added only when you approve them. A pipeline that did not
  start can be **tried again**. **Look after** beside a session's Gaffer chat box
  starts at once when pre-approved. Wording is unified: look after, goal,
  following, needs you. Adds `POST /api/v2/gaffer/outcomes/{id}/review`,
  `POST /api/v2/gaffer/outcomes/{id}/sessions` and
  `POST /api/v2/gaffer/outcomes/{id}/sessions/{key}/retry`. Needs a controller
  update.
- **Factory** is now a page of its own, in the main navigation. Each outcome you
  give Gaffer is drawn as pipelines that move through plan, design, code, review,
  test, pull request, babysit and merge, with the agent and model doing each stage,
  reviews used, and what is waiting on you. When you approve a plan you choose the
  crew (which agent and model does the thinking, building and shipping stages) and
  the stages each pipeline runs. The controller starts the next stage when Gaffer
  hands one over. A pipeline stops at **Ready to merge** until you approve it,
  unless you allowed that outcome to merge by itself. Account → Gaffer AI gains
  **How Gaffer works**: looking after sessions is pre-approved by default, and
  auto-merge for new pipelines is off by default. Adds
  `POST /api/v2/gaffer/outcomes/{id}/sessions/{key}/merge` and two profile
  columns, created on start. Needs a controller update.
- The Gaffer factory floor is easier to read and to reach. Followed-session cards
  are short, with the full goal and evidence under **Review & evidence**; the
  floor is wider on desktop and laid out as Outcomes, Sessions and Not followed;
  an empty Gaffer chat leads with **Give Gaffer an outcome**; Gaffer's entry on
  Home is a button; and the outcome dialog shows its steps. Browser-only.
- Gaffer outcomes are now started by the controller. After you approve a plan the
  sessions start and are followed whether or not the browser stays open, a
  restart carries on without starting a session twice, and **Stop starting
  sessions** leaves the rest unstarted. You can choose one model for the outcome
  and, for OpenCode and Google Antigravity, confirm you trust the folder's
  plugins. The floor gains **Pause all following** and **Stop all goals** for the
  sessions shown. Fixes from review: a launch the machine refused is no longer
  reported as started, limits must be whole numbers, and prompts an agent cannot
  be started with are caught before anything starts. Adds
  `GET/POST /api/v2/gaffer/outcomes`, `GET /api/v2/gaffer/outcomes/{id}` and
  `POST /api/v2/gaffer/outcomes/{id}/stop` (browser account only) and a
  `gaffer_outcomes` table, created on start. Restoring a copy stops any outcome
  that was still starting.
- **Give Gaffer an outcome** (factory floor). Write what you want done and choose
  a project and agent; Gaffer proposes the sessions and their prompts, you edit
  and approve the plan, and the sessions are then started as ordinary chats and
  followed, each with a goal naming the outcome. The floor groups them under
  **Outcomes**. Nothing starts before you approve, sessions never start in YOLO,
  and Gaffer still cannot merge. Adds `POST /api/v2/gaffer/providers/{id}/plan`
  (browser account only), which sends the provider only the typed objective and
  project name and saves nothing. Needs a controller update; no database or
  connector change.
- Gaffer's **Followed sessions** view is now the **Factory floor** (Ask Gaffer →
  Chat options). Beside the sessions Gaffer already follows it lists every
  running structured chat that is not followed. Tick several, review one goal
  and one set of limits, and each session is started through its own reviewed
  setup, with a per-session result. Each keeps its own access, goal and budget.
  The floor opens in one click from general Gaffer chat and from Home, which
  now also says when running chats are not followed. The separate "Choose a
  session for Gaffer" picker is replaced by **Open** on each row.
  Browser-only: no server, database or connector change.
- A portable copy can be taken with a password given for that copy alone.
  `POST /api/v2/admin/backups/run` accepts an optional body `{"password": "..."}`
  and `reet backup run` takes `--password-file F` (a file or `-`, never an
  argument). The copy is sealed with that password in the same format as one
  sealed with the stored copy password, and the stored setting is not read,
  changed or cleared, so the schedule and every other copy are unaffected. The
  password needs at least 12 characters; a blank or shorter one is refused with
  a 400 before anything is taken. It is never stored, logged or returned, and the
  activity log records that a copy was taken with a password given on the
  request, by whom and when. `GET /api/version` lists `"copy-password-request"`
  under `features`.
- The checklist at the end of a restore report now names
  `REET_LIBRARY_MANAGED_REMOVALS`, a setting the server reads that the checklist
  had missed. The test that compares the checklist with what the server reads
  caught it, and was failing on `main` until it was added.
- When a worktree is removed, reet now also brings the repository it was made
  from up to date with main, then prunes the merged local branches. It is on by
  default (owners and admins can turn it off in Settings › Admin › Repository
  maintenance). The folder is only ever fast-forwarded: a folder with
  uncommitted changes, on another branch, mid-merge or diverged from main is
  left exactly as it was. Each run is recorded in Activity as `branch-sync`.
  Needs the new `branch-sync-v1` on the machine; older machines are skipped.
- `reet agent set-server URL` points a machine's enrollment at a controller that
  holds the same machine record, for following a controller that has moved. The
  machine first has the controller prove it holds its record (a fresh nonce
  answered with an HMAC keyed by the stored secret hash, over the new
  unauthenticated `POST /api/v2/sync/agent-proof`) and only then sends its
  credentials once to confirm them. Any failed check changes nothing and names
  the check. On success only the address changes and the agent restarts. Machines
  do not yet learn a fence's `--moved-to` address on their own.
- `reet backup restore` and `reet backup import` take `--public-url ADDRESS` (form
  field `publicUrl`) to rewrite the saved mail base URL and sync library origin to
  the controller's new address and to remove MCP grants issued for the old one. An
  address that is not a plain http or https origin is refused before anything
  changes. `GET /api/version` lists `"restore-public-url"` under `features`.
- A restore or import now holds back project run members, software batches and
  queued mail, as it already did Gaffer goals, so the restored controller does not
  repeat them. Each is resumed with the existing retry, and is listed in the
  report.
- The restore report ends with a checklist of what lives outside the database and
  must be set by hand on the new machine.
- **A controller can be fenced as moved.** `reet backup fence [--moved-to URL]
  [--lift]` and `GET`, `POST` and `DELETE /api/v2/admin/fence` (owner only) raise,
  read and lift a persisted state that refuses every write with 423 (Locked), with
  a message saying the controller has moved, when and to where. Reads work. The
  owner's backup settings, run, download and verify, the fence's own routes, and
  signing in and out still work. Scheduled backups, the mail queue, the notifier,
  project runs, software batches, Gaffer goals, queued chat messages and the
  scheduled sweeps stand still, and carry on after the lift. The state survives a
  restart. A copy taken while fenced is marked fenced. `GET /api/version` lists
  `"fence"` under `features`.
- **A copy of a controller that was not stopped now needs `--allow-live-source`.**
  `reet backup restore` and `reet backup import` (and the `allowLiveSource` field
  of `POST /api/v2/admin/backups/import`) refuse a whole-controller copy that
  says its source was still running, or that says nothing because an older reet
  took it, unless the flag is given. The refusal changes nothing. Copies now
  record `source` (the controller's own identifier, stable across a change of
  address) and `fenced`. The choice to allow a live source is written to the
  activity log. `GET /api/version` lists `"copy-origin"` under `features`.
- A staged import is checked again when the controller next starts. If the
  staged database changed, no longer passes SQLite's integrity check or is newer
  than the binary, it is moved to `pending-import.refused-<timestamp>/` with the
  reason, and the live database is left as it was.
- `GET /api/version` can carry a `features` list so a script can tell which
  parts of controller moving a build holds. It is left out when there are none.
- The README no longer says a restore ends browser sign-ins. It never did by
  default: sessions and personal tokens come across unless `--drop-sessions` or
  `--drop-tokens` is given.
