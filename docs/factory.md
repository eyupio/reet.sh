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

The top of the page counts the pipelines that **Need you**, are **Ready to merge**, are **Running** and are **Done**;
the first two jump to the first such pipeline. On a desktop the project list is down the left side: choosing a project
shows only its outcomes, and a new outcome then starts in that project.

Each outcome's card has:

- **Ask Gaffer for a report**: one request to Gaffer's model for a report across the pipelines. A report may suggest
  pipelines to add; tick the ones you want and choose **Add pipelines**. Nothing is added unless you approve it, and
  added pipelines never merge by themselves.
- **History**: what happened to its pipelines, newest first, including what was done over MCP.
- **Hide details** / **Show details**, and its figures: pipelines, Gaffer reviews, Gaffer tokens, agent tokens, the
  budget per stage, **Approvals** and **Reports**.

A pipeline's status is one of **Queued**, **Running**, **Needs you**, **Ready to merge**, **Done**, **Stopped** or
**Not started**.

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

On the stage track a tick is a finished stage, a ring is the stage running now, **!** is waiting for you, **×** has
stopped and **»** is a stage you moved on yourself. **Who did each stage** lists each stage's tier, agent, model and
effort, who closed it and its handover.

Once a stage's session is running, the controller asks the machine what it is on and shows it on the **Applied** line:
**Confirmed**, **Not as chosen** (naming what differs) or **Not checked**. Nothing is stopped by a difference.

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
- The pipeline's card says **Balanced** with the reason, and **Who did each stage** and the History keep it.
- Balancing needs [plan limits](agent-accounts.md) to be on for the machine, and figures no more than six hours old.
  Without them every stage stays on its first choice.

**Save as my crew** keeps the tiers, their backups and the figure for your account. **Your Factory crew** in Gaffer
settings holds the same, and every later outcome starts from it, including one started over MCP.

## Decisions that wait for you

A pipeline marked **Needs you** says why in one sentence and asks **What do you want to do?**, with a button for each
choice. The one to take unless you know better comes first, marked **Recommended**; when Gaffer's review brought the
stage back the card says **Gaffer recommends** with its reason. Nothing happens until you choose.

- **Have Gaffer look again** has Gaffer review the work that is there now.
- **Tell the agent…** sends your own words to the working agent for its next turn.
- **Move to next stage** (or **Finish this stage**) closes the stage as it stands.
- **Open the session to answer** is the only choice while the agent waits for your answer or approval. Gaffer carries
  on by itself once you have answered.
- **Try the review again** appears when Gaffer's own review failed.
- **Give it a new budget** appears when the stage used its reviews or its time.
- **Run this stage again in a new session** appears when the stage's session was ended.

The same decisions are at the head of the outcome's card, under **N decisions waiting for you**, and in the pipeline's
session under its header.

### Gaffer decides

Each outcome is set to **You decide** or **Gaffer decides**, at the head of its card and under **Decisions** when you
approve the plan. It is yours unless you choose otherwise, and you can switch at any time.

- When a stage stops, Gaffer chooses one of that stage's own choices: review again, tell the agent, move on, a new
  budget or a new session. The controller then does it as your button would.
- Gaffer also answers an agent's multiple-choice question, choosing among the options the agent offered, or leaves it
  to you with its reason.
- It takes at most three decisions on a stage. After that the stage is yours and the card says so.
- It never decides a merge, never answers an approval, never answers a question that needs your own words and never
  stops an outcome.
- Each decision is one request to Gaffer's model. **What Gaffer decided** lists each one with what it did and why.
- You can always choose first: the buttons stay.

## YOLO

**Approvals** is **Ask me** unless you choose **YOLO**.

- On **Ask me**, agents ask before running commands or changing files. A pipeline shows **Needs you** while one is
  asking and carries on once you answer in the session. Gaffer never answers an approval for you.
- On **YOLO**, every session of the outcome starts without approvals, and the agents also act on what Gaffer asks them
  without waiting for you.
- YOLO is offered only when your account allows it, the machine accepts it and every agent in the crew has a YOLO
  option, and only after a confirmation. It is never remembered, and no plan and no model can ask for it. The
  controller checks again at every start and records each one.
- A started outcome can be switched from its card: **Switch to YOLO…** after a confirmation, or **Make them ask me**.
  The session each pipeline has in hand is switched and every later stage starts that way.
- An outcome started over MCP never runs on YOLO.

## Merging

A pipeline with a merge stage either waits for you or merges by itself. The **Merge** line on its card says which.

- **Waits for you** is the default. The pipeline stops at **Ready to merge**. **Approve merge** tells its agent to
  merge the pull request once every check passes, no review thread is open and there are no conflicts. If any of
  those is not true the agent fixes that first or brings it back to you.
- **Merge by itself**, ticked when you approve the plan, lets the agent merge under the same three conditions without
  stopping. The tick starts from **Let new pipelines merge by themselves** in Gaffer settings, which is off by default.
- **Let it merge by itself** and **Make it wait for me** on the card change one pipeline, until it is ready to merge.
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
- **Try again** is offered on a pipeline that did not start or has stopped. A session that did start is followed
  again; otherwise the stage starts in a new session. A pipeline that was waiting at Ready to merge waits there again.
  Nothing is retried on its own.
- **Stop starting sessions**, while an outcome is still starting, leaves what has not started unstarted. Sessions
  already started keep running.
- **Stop following** ends every unfinished pipeline of an outcome: Gaffer and the controller stop looking after them
  and start nothing more. Sessions that still exist keep running, and nothing is merged or closed. A stopped pipeline
  can be tried again, one at a time.
- Ending a session that a pipeline is working in says so first, and offers to stop following its outcome and delete or
  archive it. If the session is ended and the outcome kept, the pipeline's card offers to run the stage again in a new
  session, move on, or stop following.
- A finished outcome has **Archive**, which hides it and keeps its record, report and history, and **Delete**, which
  erases the record for good. **Show archived** brings archived outcomes back with **Restore**. Neither touches the
  sessions or pull requests the outcome started. An archived outcome has to be restored before a pipeline in it is
  tried again, sent back or added to.
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
