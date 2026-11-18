# Factory

The **Factory** is the page where you hand [Gaffer](gaffer-ai.md) an outcome and watch it run. You say what you want
done, Gaffer proposes the work as pipelines, you edit and approve the plan, and the controller runs each pipeline
stage by stage. Factory is a tab beside Home on a controller with accounts sign-in, for any account that can start
sessions; a read-only account does not have it. Before the first outcome you need a connected machine, a project and a
default Gaffer provider, and the page lists whichever is missing under **Three things and you're in business**.

Every stage of a pipeline is an ordinary agent session on the project's machine: it is listed in
[Sessions](session-board.md) marked **Factory · stage n of m**, and you can open it and type in it. In a git project
each pipeline works in its own [worktree](terminal-worktrees.md). Gaffer follows each stage as it follows any session
it looks after. It reviews and hands over; it cannot merge, run commands or answer approvals. Opening the Factory page
starts nothing and makes no model request.

## Outcomes and pipelines

An **outcome** is what you asked for, in up to 4,000 bytes of plain text, in one project. A **pipeline** is one piece
of that work, with its own title, brief and stages. Pipelines of an outcome run side by side and cannot see each other.

To start one, choose **Give Gaffer an outcome**:

1. Write the outcome and choose the **Project**, or **A repository from GitHub…** to have a connected machine clone
   one as a [temporary project](projects.md#temporary-projects). Choose the **Agent**, and a **Model** and **Effort**
   if you like, and how many pipelines Gaffer may propose under **Sessions** (up to five).
2. Choose **Ask Gaffer for a plan**. Gaffer's default provider is sent what you typed and the project's name, nothing
   else, and proposes the pipelines. **Write the plan myself** skips this.
3. Review the plan. Edit any title or brief, add or remove a pipeline, and tick the stages each runs through.
   **Reply to Gaffer** asks for a revised proposal.
4. Set the limits, **Approvals**, **Decisions** and the merge, described below, then choose **Approve and start**.

Nothing starts until you approve. After that the controller does the starting and the moving on, so you can close the
browser.

## The page

What needs you from the Factory is also in [Home](session-board.md)'s **Waiting for you** list, mixed with your
sessions, each with the same buttons; **Details** there opens the pipeline here. The bell counts the same things.

The page is a list of pipelines from every outcome, and the detail of one pipeline. The line under the title counts
what is **waiting for you**, **working** and **finished**. The list has four sections:

- **Waiting for you**: every pipeline that needs you, the longest waiting first.
- **Working**: pipelines that run by themselves or wait their turn.
- **Finished or stopped**: pipelines that finished or were stopped.
- **Other sessions**: sessions you started yourself, the ones Gaffer looks after and the running chats it does not
  yet.

A section opens and closes, and your browser remembers which are open. The first time only **Waiting for you** is
open. **Other sessions** also opens by itself when a session in it needs you, and then stays open for the visit
until you close it, so answering the last one does not close it under you. A closed section still says what is in
it: how long the oldest has waited, or the first titles.

A pipeline's status is **Decision**, **Approval**, **Ready to merge**, **Agent stopped** or **Failed** while it waits
for you, **Running** or **Waiting its turn** while it works, and **Finished** or **Stopped** once it ends. Each item in
the list shows its status, its title, the outcome it is for and its stages.

- On a wide screen, 1100 pixels and over, an item is one button with its stages as dots and a caption such as
  "Review · 4 of 8 · next Test". Choosing it shows the pipeline beside the list.
- On a tablet each stage is named. On a phone the stages are circles with a caption such as "Review in hand · Plan,
  Design, Code done · next Test".
- On a tablet or phone an item that waits for you also says why, or what Gaffer recommends and why. It has one filled
  button for what it most likely needs, such as **Have Gaffer look again**, **Approve the merge** or **Try again**, and
  an approval has **Allow once** and **Deny**. **Details and other actions** (**Details** on a phone) opens the
  pipeline.

On a wide screen the first pipeline waiting for you is open beside the list when the page opens, or else the first
one working. The detail has the outcome, the pipeline's title and status, **Open session** and the outcome's menu
(**⋯**). Under them are the pipeline's stages and, when it waits for you, what is waiting. Then come closed rows, each
showing its headline: **Gaffer's report**, **Usage and budget**, **What Gaffer decided**, **History** and **Settings
for this outcome**. One bar at the foot, **What do you want to do?**, holds everything you can do to the pipeline.

On a tablet or phone **Details** opens the pipeline as a screen of its own. **‹ Factory** goes back to the list, and so
does the browser's Back button. A pipeline opened from Waiting for you says where it is, such as "2 of 3 waiting for
you", with buttons for the previous and the next. The chosen pipeline is kept in the page's address, so a reload or a
shared link opens it again.

On a wide screen the **Projects** panel is down the left side, and each project says how many of its pipelines are
waiting and working. Choosing a project shows only its pipelines, and a new outcome then starts in that project;
**Show all projects** goes back. On a tablet or phone the **Projects** button opens the same choice.

The detail's rows hold the rest:

- **Gaffer's report**: **Ask Gaffer for a report** is one request to Gaffer's model for a report across the outcome's
  pipelines. A report may suggest pipelines to add; tick the ones you want and choose **Add 1 pipeline** (or **Add 2
  pipelines**, and so on). Nothing is added unless you approve it, and added pipelines never merge by themselves.
- **Usage and budget**: what this pipeline and the whole outcome have used. Reviews and time left in the stage in
  hand, Gaffer tokens, agent tokens, context left, the number of pipelines and the budget per stage. **Review goal**
  (**Goal and limits** while the pipeline waits for you) opens the stage's goal and limits.
- **What Gaffer decided**: the decisions Gaffer took for this outcome, described below.
- **History**: what happened to the outcome's pipelines, newest first, including what was done over MCP. It is read
  again each time you open it.
- **Settings for this outcome**: who decides when a stage stops, **Approvals**, this pipeline's **Merge**, and
  **Reports**.

The outcome's menu holds **Stop starting sessions** while it is starting and **Stop following** while it is
unfinished. A finished outcome has **Archive** and **Delete** there instead, and an archived one only **Restore**.

## Stages and what runs in each

A pipeline runs through the stages you ticked, in this order: **Plan**, **Design**, **Code**, **Review**, **Test**,
**PR**, **Babysit** and **Merge**.

- Every stage runs in an agent session of its own, in the pipeline's folder and branch. It starts from that stage's
  instruction and the handover of the stage before. The earlier stage's session is ended and its history kept.
- When the working agent's reports show the stage's deliverable is done, Gaffer hands it over with a summary and the
  controller starts the next stage you approved. Gaffer does not choose the next stage, its agent or its model.
- Every stage but merge is told not to merge.
- A stage can be **split into parts**, by Gaffer in its plan or by you with **Split into parts**. Each part has a name,
  a brief and a tier of its own, and the parts run one after another, each in its own session. **Do as one stage**
  undoes a split. The pull request, babysitting and merge are never split.
- Each stage has a budget of Gaffer's reviews and hours, set under **Change limits** when you approve: 12 reviews over
  24 hours unless you change it, from 1 to 48 reviews and 1 to 168 hours.

In the detail's stages a tick is a finished stage, a ring is the stage in hand, **!** is waiting for you, **×** has
stopped and **»** is a stage you moved on yourself. Pick a stage to see its facts. The stage in hand shows its agent,
how long it has run, what its agent is doing now, Gaffer's reviews and Gaffer's latest review. A finished stage shows
who did it, with its tier, agent, model and effort, how it ended and its handover. A stage to come says who will do it.

Once a stage's session is running, the controller asks the machine what it is on and shows it on the stage's
**Applied** line: **Confirmed**, **Not as chosen** (naming what differs) or **Not checked**. A finished stage keeps it.
Nothing is stopped by a difference.

An effort the stage's agent does not offer for its model does not stop the stage: it starts on the agent's default
effort instead, its facts say "agent's default effort (high is not offered)" and its **Applied** line is **Confirmed**
with the reason. A model the agent does not offer still stops it. **Try again** and **Send back** ask for the effort
again, except while an earlier start of that stage is still unanswered; so do Gaffer's own decision to run a stage again
and the restart of a stage on its backup.

## Your Factory crew

The crew says which agent does a stage. It has three tiers, named for how demanding a stage is:

- **Heavy**: careful judgement. By default plan, design and review.
- **Standard**: ordinary building and checking. By default code and test.
- **Light**: routine work. By default the pull request, babysitting and merge.

Under **Choose who does what** in the outcome dialog, give each tier an agent and, if you like, a model and an effort.
Left alone, one agent does every stage. When Gaffer plans an outcome it says how demanding each stage of this work is;
a stage it moved off its usual tier is marked **Gaffer's assessment** with its reason and a button that puts it back.
A tier you change is marked **Your choice**. Gaffer names no agent and no model: its rating picks a tier, and the
tier's agent is yours.

Each tier can have a **Backup**: a second agent, model and effort.

- A stage starts on its backup when its first choice's plan has under 20% left in its tightest window and the backup's
  has more, or the backup is signed in with a key. The figure is **Switch to a backup below**, from 5 to 90.
- A tier can have up to three backups, tried in the order you give them.
- A working agent is never interrupted. If a stage's agent stops, or its session ends, with its plan below your
  figure, the stage is run again on its backup in a new session in the same folder and branch, once for a stage.
- The stage's facts in the detail say **Balanced** with the reason. A finished stage keeps it, and so does History.
- Balancing needs the machine to be able to read the agent's [plan](agent-accounts.md), and figures no more than six hours old.
  Without them every stage stays on its first choice.

**Save as my crew** keeps the tiers, their backups and the figure for your account. **Your Factory crew** in Gaffer
settings holds the same, and every later outcome starts from it, including one started over MCP.

![Your Factory crew in Gaffer settings, with a backup on OpenCode and a saved model no machine lists](images/ui/crew-choices-1440-light.png)

In Gaffer settings the choices are those of every machine on your account, since no one machine is in view:

- **Agent**: every agent; one none of your machines can run says "(not on your machines)".
- **Model**: every model your machines list for the agent, with **Agent's default model** and **Other…**. A saved
  model none of them lists stays, with a line saying the stage stops if the agent does not offer it.
- **Effort**: every level your machines report for the agent, and **Default effort**. An agent a machine runs that has
  none, such as OpenCode, shows **No effort setting**. An agent none of your machines runs, or any agent when you have
  no machines, offers **Default effort**, low, medium and high. A saved level none of them offers stays, marked "(not
  offered by your machines)": the stage then starts on the agent's default effort.

Changing a row's agent clears its model and effort. If your machines cannot be read the form says so and still saves.

## Decisions that wait for you

A pipeline that needs you is in **Waiting for you**. Its detail says why in one sentence and asks **What do you want
to do?**, with a button for each choice. The one to take unless you know better comes first, marked **Recommended**.
When Gaffer's review brought the stage back, the detail says **Gaffer recommends** with its reason, and so does the
item on a tablet or phone. Nothing happens until you choose.

- **Have Gaffer look again** has Gaffer review the work that is there now.
- **Tell the agent…** sends your own words to the working agent for its next turn.
- **Move to next stage** (or **Finish this stage**) closes the stage as it stands.
- **Open the session to answer** takes you to the agent while it waits for your answer or approval. When the session
  holds an approval, the pipeline is marked **Approval** and you can answer it on the page with **Allow once** or
  **Deny**: on the item on a tablet or phone, and in the detail at every width. Gaffer carries on by itself once you
  have answered.
- **Try the review again** appears when Gaffer's own review failed.
- **Give it a new budget** appears when the stage used its reviews or its time.
- **Run this stage again in a new session** appears when the stage's session was ended.

The same decisions are in the pipeline's session, under its header.

### Gaffer decides

Each outcome is set to **You decide** or **Gaffer decides**, under **Who decides when a stage stops** in **Settings for
this outcome**, and under **Decisions** when you approve the plan. It is yours unless you choose otherwise, and you can
switch at any time.

- When a stage stops, Gaffer chooses one of that stage's own choices: review again, tell the agent, move on, a new
  budget or a new session. The controller then does it as your button would.
- Gaffer also answers an agent's multiple-choice question, choosing among the options the agent offered, or leaves it
  to you with its reason.
- It takes at most three decisions on a stage. After that the stage is yours and the detail says so.
- It never decides a merge, never answers an approval, never answers a question that needs your own words and never
  stops an outcome.
- Each decision is one request to Gaffer's model. The detail's **What Gaffer decided** row lists each one with what it
  did and why.
- You can always choose first: the buttons stay.

## YOLO

**Approvals** is **Ask me** unless you choose **YOLO**.

- On **Ask me**, agents ask before running commands or changing files. A pipeline waits for you while one is asking,
  and carries on once you answer, on the page or in the session. Gaffer never answers an approval for you.
- On **YOLO**, every session of the outcome starts without approvals, and the agents also act on what Gaffer asks them
  without waiting for you.
- YOLO is offered only when your account allows it, the machine accepts it and every agent in the crew has a YOLO
  option, and only after a confirmation. It is never remembered, and no plan and no model can ask for it. The
  controller checks again at every start and records each one.
- A started outcome can be switched on the **Approvals** line of **Settings for this outcome**: **Switch to YOLO…**
  after a confirmation, or **Make them ask me**. The session each pipeline has in hand is switched and every later
  stage starts that way.
- An outcome started over MCP never runs on YOLO.

## Merging

A pipeline with a merge stage either waits for you or merges by itself. The **Merge** line in **Settings for this
outcome** says which.

- **Waits for you** is the default. The pipeline stops at **Ready to merge**. **Approve the merge** asks you to confirm,
  and **Approve merge** then tells its agent to merge the pull request once every check passes, no review thread is open
  and there are no conflicts. If any of those is not true the agent fixes that first or brings it back to you.
- **Merge by itself**, ticked when you approve the plan, lets the agent merge under the same three conditions without
  stopping. The tick starts from **Let new pipelines merge by themselves** in Gaffer settings, which is off by default.
- **Let it merge by itself** and **Make it wait for me** on the Merge line change one pipeline, until it is ready to
  merge.
- On a YOLO outcome, merging by itself takes a separate confirmation for each pipeline. Until you give it the Merge
  line reads **Waits for you (YOLO)** and the pipeline stops at Ready to merge, whatever was ticked at approval.
- Added pipelines and pipelines started over MCP always stop at Ready to merge.
- A merge cannot be undone from reet.

## Stopping, retrying and sending back

- **Move to next stage** closes the stage in hand as it stands and starts the next, with Gaffer's last review as the
  handover. On the last stage it reads **Finish this stage**.
- **Send back…** has the stage in hand, or an earlier one, done again. You choose the stage and say why; that stage
  and every later one start over in a fresh agent session in the same folder and branch, and the agent is told your
  reason. Nothing already committed is undone.
- **Try again** is offered on a pipeline that did not start or has stopped, in its detail and, for one that did not
  start, on its item on a tablet or phone. A session that did start is followed again; otherwise the stage starts in a
  new session. A pipeline that was waiting at Ready to merge waits there again. Nothing is retried on its own.
- **Stop starting sessions**, in the outcome's menu while it is still starting, leaves what has not started unstarted.
  Sessions already started keep running.
- **Stop following**, in the outcome's menu, ends every unfinished pipeline of an outcome: Gaffer and the controller
  stop looking after them and start nothing more. Sessions that still exist keep running, and nothing is merged or
  closed. A stopped pipeline can be tried again, one at a time.
- Ending a session that a pipeline is working in says so first, and offers to stop following its outcome and delete or
  archive it. If the session is ended and the outcome kept, the pipeline waits for you marked **Agent stopped**, and
  offers to run the stage again in a new session or move on. Stop following is in the outcome's menu.
- A finished outcome's menu has **Archive**, which hides it and keeps its record, report and history, and **Delete**,
  which erases the record for good. Your account's default comes first. **Show archived** at the top of the page lists
  archived outcomes under Finished or stopped, and their menu has **Restore**. Neither touches the sessions or pull
  requests the outcome started. An archived outcome has to be restored before a pipeline in it is tried again, sent back
  or added to.
- The controller ends the sessions a pipeline has finished with and keeps their history. Worktrees are not touched,
  and a worktree a pipeline still works in is kept by every kind of cleaning.

## From an AI client

A connected AI client can read and steer the Factory through [MCP](mcp.md#read-and-steer-the-factory). The three
reads need a writer account; the rest also need a management connection.

- `reet_list_factory` lists the outcomes with their pipelines, stages and states.
- `reet_get_factory_outcome` reads one outcome; a management connection also receives handovers and Gaffer's report.
- `reet_list_factory_projects` lists the projects an outcome can run in.
- `reet_start_outcome` starts an outcome in a project, with ordinary approvals.
- `reet_add_pipelines` adds up to five pipelines to an outcome.
- `reet_move_pipeline_on` closes the stage in hand and starts the next.
- `reet_tell_pipeline_agent` queues your instruction for the working agent of the stage in hand.
- `reet_send_pipeline_back` has the stage in hand, or an earlier one, done again with your reason.
- `reet_retry_pipeline` tries a pipeline that did not start, or stopped, again.
- `reet_set_pipeline_merge` makes a pipeline wait for your approval before merging. It cannot do the opposite.
- `reet_approve_pipeline_merge` records your approval for a pipeline that is Ready to merge.
- `reet_report_outcome` asks Gaffer's model for one report.
- `reet_stop_outcome` stops an outcome. Sessions already running keep running.

An outcome that runs on YOLO can only be read, made to wait before merging, asked for a report and stopped over MCP.
No tool answers an approval, turns on YOLO, gives Gaffer an outcome's decisions or merges anything itself.

## Limits

- An outcome starts with up to five pipelines and holds at most ten.
- A stage has at most four parts, and a pipeline at most sixteen stages and parts in all.
- A tier has at most three backups.
- Gaffer takes at most three decisions on a stage.
- Gaffer follows at most ten sessions at once, so a plan with more pipelines than it has room for is not approved.
- With **Have Gaffer report across this outcome by itself** ticked, Gaffer reports at most six times for an outcome
  and never within ten minutes of the last.
- An account keeps its thirty newest outcomes. An older one is dropped only when nothing in it is still moving.
- The Factory counts tokens and attaches no price to them.

For how Gaffer reviews a stage, what it is sent, token counts and the activity log, see
[Gaffer](gaffer-ai.md#the-factory).
