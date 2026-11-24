# Changelog

Changes that matter to someone running a reet controller, newest first. Before
this file existed releases were described on their GitHub release pages.

## Unreleased

- **"Stopped waiting" has its own mark**: a chat reet stopped waiting on (its turn ended with work still running, and
  nothing said the work finished) now shows an amber "!" in place of its dot, in Sessions, Home, the session's tab and
  header, the workspace list, the resume list and the chat's status line, with its word in the same warning tone. On a
  phone, where a row shows no status word, it no longer looks like any other row. It still does not ring or count as
  waiting for you.
- **`reet install` keeps `REET_TRUSTED_PROXIES`**: a controller installed as a native service behind a reverse proxy
  no longer loses the setting.
- **Sign-in limits charge the right person**: a stranger who knows your email can no longer keep you out of password
  sign-in with five wrong passwords a minute from anywhere. A failed sign-in now counts against its address (60 a
  minute), the email from that address (5 a minute) and the account from anywhere (100 an hour); a correct one counts
  against none. IPv6 addresses are counted by their /64. A refused sign-in says "too many attempts, try again later".
- **`REET_TRUSTED_PROXIES` for controllers behind a reverse proxy**: until now every visitor behind a proxy shared the
  proxy's address, so one person's failures could block sign-in, single sign-on and password reset for everyone. List
  your proxy's own address (or a range) and reet takes the visitor's address from `X-Forwarded-For`. Empty by default,
  which keeps today's behaviour; a range that trusts every address is refused at start-up. List only the proxy itself:
  see the warnings in the README.
- **Answer a terminal agent's question from reet**: when Claude Code or Codex in a terminal session stops at a dialog
  with numbered options, the session's screen shows what it is asking (the command, file or plan heading, then the
  question) with the options as buttons, and Home's "Waiting for you" shows the same for a short dialog. Choosing one
  makes the machine read the screen again and press the keys only if the same dialog is still there; otherwise it
  says the question changed. An option that keeps applying after the answer, or whose text is cut short, takes a
  second tap. **This changes what leaves a machine**: for a dialog reet recognises, up to 500 characters of the
  dialog and up to nine options of 200 characters are sent to the controller, held in memory and never written to
  its database or logs; before, no screen text left the machine. Needs a machine and a controller on this version.
- **Skill suggestions stay out of the way in Chat**: **Skills for this task** now opens as one quiet line above the
  message box on every screen size, naming the task and the first suggestion with **See skills** to open the rest.
  **Pause** appears once the suggestions are open. A Factory stage's suggestions are unchanged and stay open beside
  its plan.
- **A desk can open in a tab of its own**: **Move to its own tab** in the desk switcher opens another desk in a new
  browser tab, leaving the desk you are on where it is; the desk already in this window is switched off. The same item
  is on each desk card's menu on the Desks list. Reet keeps one desk to a window, so each tab keeps its own address and
  its own chosen desk tab, and both come back as they were after a reload. Nothing new is stored.
- **An agent's slash commands in Chat**: when an agent announces its commands, the Skills sheet in Chat
  lists them first as "Commands from {agent}", each with its argument and description; picking one adds it to
  your message and sends nothing. If you type a `/name` the agent did not list, a line under the message says it may
  only work in the agent's terminal, with the way there; the message still sends, and the line then says it went as
  text. Needs a machine and a controller on this version; an older machine shows a line asking to update reet on it.
- **A turn that ends with work still running no longer reads as finished**: when an agent ends its turn while a
  subagent or background command it started is still open, the session says "In background" (in Chat: when the turn
  ended, how many tasks are running, and that you can send your next instruction) instead of "Finished", and it does
  not ring the finish alert. If the agent never says the work ended, reet stops waiting after 30 minutes and says
  "Stopped waiting", with a note that the work may still be running on the machine. In the Factory, a stage in this
  state says when it stops waiting, Gaffer does not review it as finished, and moving it on warns first that this
  stops the work. Needs a machine and a controller on this version; with an older one, sessions read as before.
- **Private Factory templates**: a finished outcome whose every pipeline finished, or had its delivery accepted, can
  be saved as a template with **Save as template…**. A template keeps the plan (each pipeline's prompt, stages and
  tiers, criteria, merge setting and the approved skill revisions) and never the machine, folder, agents, models or
  sign-ins, which you choose each time you use it. Every save is a new version and old versions stay readable, up to 20
  templates of 20 versions each. **Templates** on the Factory page lists them; **Use** opens the start step with one
  line per pipeline, what changed since the version you last used, which pipelines merge by themselves, and any skill
  that is no longer approved, which you must agree to start without. An outcome started from a template says which
  template and version it came from, and keeps its own inputs when the template later changes or is deleted.
  Templates are private to your account. Delete confirmations across reet now use the danger colour.
- **Factory reads are compressed, and the Factory page reads once**: JSON answers of 1 KiB or more are gzipped for
  browsers that accept it, so a Factory of 30 pipelines is read as about 92 KB, not 353 KB, for a few milliseconds
  more on the controller. On the Factory page the bell now uses the page's own reads, so outcomes are read 12 times a
  minute, not 16; there the bell can show a newly waiting pipeline a few seconds later than before.
- **A launch survives a lost connection**: when a machine's answer to a launch is lost, or the controller restarts
  while one is in flight, the pipeline now says it cannot tell whether the session started, and **Try again** takes up
  the session that did start and never starts a second one. A merge launch in flight at a restart is no longer
  forgotten. A stage that stopped with its plan low is run on its backup even when its old session could not be ended
  at first, and a session left behind by a backup switch or a send-back is ended once its machine can be reached.
- **A first session, guided**: after your first machine connects, Home offers **Your first session**. It lists the
  agents on that machine that run as a chat, each with its sign-in in the same words the Factory uses, and offers one
  read-only question to ask, without YOLO. It waits for the answer and links to it. An agent that is signed out is
  shown as waiting, not as an error, with a terminal on that machine one press away; the guide looks again by itself.
  Factory, Sessions and starting a session stay where they were. **Not now** puts the guide away, and **Show the
  first-session guide** brings it back until you have had a first answer. The quick start describes it, with the
  steps for using reet on your phone.
- **Launch readiness before you approve a Factory plan**: each stage of the plan, and each part of a split stage, is
  now one line saying the agent, model and effort it will run on and where its agent came from (your saved crew, your
  choice here, or the outcome's agent because no crew is saved or the saved agent is not on the machine). Its backups
  are folded under it, in order, with what each will do (balancing can move a stage onto it, or it takes over only
  after a usage limit). **Launch readiness on** the machine, at the head of the pipelines, counts the stages that are
  ready and says each cause once, with the crews and stages it affects; **Choose who does what** takes you to the
  agent to change. A model the machine does not list, an old list, no sign-in found, sign-ins the machine could not
  read and an effort the agent does not offer are each worded differently from a model or effort you left to the
  agent, which is shown as your choice. Old plan figures on a backup that can still take over are not something to
  look at. Where something was not checked, a note says what happens when the stage starts, and the stage's
  **Applied** line says what it is running on. The machine is read again when the plan opens. Only an agent that is
  not on the machine now, or a machine that is not connected, holds **Approve and start**, with the reason beside it;
  everything else is a warning.
- **A usage limit is explained, with both ways on, on Home and in the Factory**: a pipeline whose worker reached its
  plan's limit now says which agent ran out and which agent the backup is. **Continue with the backup** and **Choose
  another model…** both carry on in the same folder and branch. After another model has already taken over on the
  stage it says where the stage started, and whether a backup is saved or none is left. Home's **Waiting for you**
  and the Factory list show both as buttons, on a phone too, and the detail adds the plan left and when it resets.
- **Older machines are not asked to restart other agents**: reet only offers the terminal restart for Claude Code, Codex, Cline
  and Antigravity on a machine whose connector is new enough to open their terminal on a saved conversation
  (`session-mode-agents-v1`). Update reet on the machine to enable it; OpenCode is unchanged.
- **Codex YOLO now turns its sandbox off**: a Codex terminal started on YOLO runs with `--ask-for-approval never --sandbox
  danger-full-access`, so its commands can change any file and use the network on the machine. Before, it kept Codex's
  sandbox and refused whatever needed to leave it. The YOLO confirmation says so, and so does the confirmation when a Codex
  chat that is on YOLO is switched to its terminal, which now needs an explicit tick first. The account setting, the machine's own
  `reet terminal yolo` switch and the confirmation still gate it. A Gaffer chat on YOLO is unchanged: it still only answers
  the approvals Codex asks for.
- **An empty turn is no longer silent for Antigravity, Claude Code and Codex**: a turn that says and does nothing shows a
  notice that the agent ended it without a reply, and points at restarting it in its terminal (OpenCode and Cline keep their
  provider advice).
- **Restart any agent in its own terminal**: Session actions (and the **Terminal** tab, when the agent has no shared native terminal) can restart a Gaffer chat in the agent's terminal UI, and back, for
  Claude Code, Codex, Cline and Antigravity as well as OpenCode, on the same saved conversation. Cline's terminal resume
  (`--id`) has not been checked against a chat created over ACP; if it cannot open the conversation the chat is restored.
- **Delivery review for Factory pipelines**: when you approve a plan, or add pipelines, you can write acceptance
  criteria for each pipeline (up to ten, one line each). A pipeline with criteria stops in **Delivery review** before it
  merges or finishes. The review card on the Factory page shows the reviewed Git commit, each criterion as **Passed**,
  **Needs work** or **Unverified** with its evidence (your notes, artefact links and GitHub checks), and **Accept
  delivery** and **Send back** beside them. A decision names the commit and the review you saw; if a new commit arrived,
  or someone else changed or decided the review, it is refused and the card reads the review again and keeps what you
  typed. Accepting the delivery is not approving the merge: **Approve the merge** stays its own action. Home's
  **Waiting for you** shows the status and opens the exact review. Outcomes saved before this have no review.
- **Plan limits from more sign-ins**: a machine now also reads Claude Code's and Codex's macOS keychain items,
  Google Antigravity's Linux keyring item or macOS keychain item, and GitHub Copilot's own `gh` sign-ins, so their
  plan left shows where an agent keeps its sign-in outside its files. Where a file and a keychain item disagree the
  later token is used; reet still never renews a sign-in. The Usage page also draws every plan window even when a
  provider sends two of the same kind, and keeps figures from the last read dimmed after a failed period change.
- **Skill suggestions fold away on a phone**: the **Skills for this task** panel above a session's message box now
  starts folded to its header on a phone, so it no longer crowds the conversation and composer. **See skills** opens
  it; the desktop panel and a Factory stage's suggestions are unchanged.
- **Add pipelines stays on agents the machine has**: when the machine's report leaves out an agent, even the
  outcome's own, the **Add pipelines** dialog no longer offers it or picks it by default, and a pipeline that names
  an agent the machine does not offer cannot be added until another is chosen. The wording around stage efforts now
  matches what the controller does: an effort the machine does not report is dropped and the stage starts on the
  agent's default effort.
- **MCP connections no longer approve merges unless you allow it**: `reet_approve_pipeline_merge` is refused for a
  bearer-token or OAuth connection until **May approve Factory merges** is switched on for it under
  **Account → MCP access**. The switch is off for every connection, existing ones included. A Factory stage's agent
  can use the same connection as your own client on its machine, so turn it on only for a client you trust. Approving
  on the Factory page, through Gaffer's own tools and through Browser WebMCP is unchanged.
- **Use a published skill without installing it**: **Copy prompt** for a published skill is now the
  `npx skills use "<its reet.sh page>" --skill "<name>"` prompt, which fetches the skill for one session, so you, or
  anyone if it is public, can use it on a machine that does not have it. It is on the Skills page, in Write a skill, on
  Community and list entries for skills hosted on reet.sh (in your library or not), and shown on the skill's public
  page under **Use it without installing**. A skill that is not published is still used by name with its description,
  and the session skill pickers are unchanged.
- **Desk links open again**: a shared desk address (`/desks/<id>`) and the newer
  app pages (`/desks`, `/factory`, `/prompts`, `/usage`) now load the app instead
  of returning a 404. Signing in from one of them also returns you there.
- **Lists links go to Lists**: the bare `/lists` address and the stale link in a
  skill's Lists section now reach `/shared`, the Lists page, instead of a 404.
  `/factory/` likewise settles on `/factory`.
- **Gaffer keeps watching a stopped session**: a goal that went to **Needs you**
  no longer stays there after the working session has moved on. Gaffer keeps
  polling the session's retained progress and live status; when the agent
  carries on by itself (for example once you answer it in its own chat), the
  reason for the stop no longer holds and Gaffer follows again and reviews the
  new work. A review of Gaffer's own that failed still waits for you to resume
  it.
- **Factory stages the Factory has finished with leave the session lists**: Home, Sessions and the
  session workspace show the pipeline's stage in hand, marked with its stage, and leave the
  finished ones to the pipeline item on the Factory page. A Factory that cannot be read keeps
  every session listed, as before.
- **Latest code in Start an agent**: a repository project now offers the GitHub's-latest / this-machine's-copy choice
  even when its **Worktree for each session** setting is off. The controller isolates every agent start whose folder
  is a saved repository, so the dialog now asks for the worktree it gets and sends the choice with it; a folder that is
  behind GitHub can start from GitHub's latest commit.
- **Factory GitHub facts**: a pipeline's detail now shows the pull request number and link, every CI check with its
  state, how long CI has run, and how many of Gaffer's decisions put a stage right. The figures are read from the
  session in hand through the GitHub CLI signed in on its machine. A finished outcome's action bar now puts **Archive
  outcome** and **Delete outcome…** beside the pipeline, where the next step is read.
- **A message still goes after a finished turn**: a queued message no longer
  waits behind a chat that looks busy after the agent has finished. The
  connector now re-reads the native agent's status while it believes a turn is
  running, so a late or missed busy update cannot hold the queue. **Stop and
  send** also keeps the message when the stop is not confirmed at once and asks
  again, instead of turning it into a failure you have to retry.
- **Factory decisions**: a stage decision shown in its session now says who is taking it and why. When Gaffer is not
  going to send the choice itself it says so: this outcome is **You decide** (and how to hand it over), the stage has
  used Gaffer's three decisions, Gaffer left this one to you, or Gaffer is deciding now. **Gaffer's latest review**
  opens in the card instead of pointing at a review the session did not show. On a phone the card keeps a bounded
  height, scrolls and keeps its choices at the foot, so a long recommendation no longer hides the buttons. A stop whose
  request was lost when the controller restarted is asked again after two minutes without using another of the three
  decisions.
- **Gaffer planning behind a proxy**: asking Gaffer for a Factory plan no longer
  holds one request open while the model thinks. The controller starts the plan
  in the background and the browser reads it in short polls, so a reverse proxy
  or Cloudflare, which ends an origin response after about 100 seconds, no
  longer cuts a slow plan and shows a bare `502`. The provider call also keeps
  running if the page closes; a finished plan is readable for five minutes.
  Bad input still answers immediately, and the same per-account concurrency
  limit and provider error wording apply.
- **Skills follow one library**: Active skills go to every connected machine;
  Archived skills stay in reet and are removed from machines. Delete removes the
  library entry and its copies. Archive, Restore and Delete keep your place and
  offer Undo. In use and To test migrate to Active; Parked migrates to Archived.
- **Machine discoveries ask first**: New on a machine offers Keep or Archive,
  with the same review linked from Home. Scans do not create library items.
- **Skill content and recovery**: edit reet's saved copy, review Update from
  original, adopt a backed-up machine edit with Use this version, or put back an
  earlier version from History. Machine removals and replacements have dated
  backups retained for 30 days, listed with `reet agent restore --list`.
- **Simpler skill sync**: Sync reports fleet progress and offers Retry for a
  failed machine. The old Go and web plan/preview/approve engines are removed; Advanced sync links now open the current Sync page. Update
  connectors to skills protocol 2; local allow-apply consent and Pause sync still
  apply. Setups skip skill folders. `reet sync` and `reet agent library-import`
  explain their replacements and exit without changing files. MCP uses
  Active/Archived and archive/restore/delete in place of shelf stages and targets.

- **Usage**: a new page, **More → Usage**, lists every agent in one place: how
  much of each signed-in plan is left, and the tokens its chats and Factory
  stages reported today, over the last 7 days or over the last 30 days (in your
  own time zone), with the sessions behind the figures and Gaffer's own model
  requests. An agent with nothing to show says why in a sentence: no sign-in on
  a machine, an API key with no plan to read, an agent whose plan reet cannot
  read, a read that failed, or terminal access off. The **Plan limits** panel
  opened from the plan chips links to it with **See all usage**.
- **Plan limits are always on**: each machine now reads how much of each
  agent's plan is left for every account, for the agents reet can read a plan
  for. The switch in **Settings
  → Agents** is gone; in its place a short note under **Usage** says what is
  read and links to the page. Balancing a Factory stage to its backup now needs
  only that the machine can read the agent's plan.

  **Upgrading**: after this upgrade every machine with terminal access on asks
  the providers of the agents reet can read a plan for, from the machine and
  with the agent's own sign-in, how much of each plan is left, for every account, including accounts
  that had never turned plan limits on. The sign-in stays on the machine; the
  controller is sent the account's name, the plan and percentages, as before.
  Token totals on the Usage page start from the day of the upgrade: nothing
  earlier was dated. Counts older than 90 days are removed the next time the
  account reports usage; a report made while a machine is disconnected is not
  counted.

- **Plan limits**: a machine now also says whether OpenCode, Google Antigravity
  and GitHub Copilot are signed in there and how much is
  left of an OpenCode Go plan (session, week, month), an Antigravity plan
  (session and week, with **Other models** apart) and a Copilot plan (the
  month's premium requests, chats and completions). A sign-in with no such plan
  says so instead of showing figures. Update reet on each machine to get this;
  a machine with an older reet keeps showing Claude Code and Codex only. These
  are the providers' own internal endpoints and can change without notice; the
  Copilot request presents itself as an editor's Copilot Chat because that
  endpoint answers nothing else, and the Antigravity request names itself as
  that agent's own client (`User-Agent: antigravity`). Tokens are only read, only sent to their own
  provider and never refreshed. Sign-ins kept only in a keyring or keychain, or
  only by the GitHub CLI, are not read. See
  [agent sign-ins and plan limits](docs/agent-accounts.md).
- **Navigation**: the main tabs now read Home, Sessions, Factory, Desks,
  Machines, Skills. **Desks** is on a phone's bottom bar (Home, Sessions,
  Factory, Desks, Machines, More) and in a tablet's top bar, instead of under
  **More**.
- **Home and desks**: sessions on a desk no longer show in Home's **Continue
  working**, on its planning board, in its count of open sessions or in the
  Projects panel's running counts on Home; the desk is where you see them. A desk session that needs you (an approval, a question, a
  failure or work to review) still shows in **Waiting for you** and in the bell,
  counted as before, marked with its desk's name, and **Open** takes you to its
  pane on the desk. If your desks cannot be read, Home shows every session as
  before.
- **Sessions and desks**: the Sessions page leaves a session on a desk out too,
  unless it needs you; then it stays, still marked with its desk and opening at
  its pane. When every session left is on a desk, the page says so and points to
  **Desks**. If your desks cannot be read, every session shows as before.
- **Factory crew**: **Your Factory crew** in Gaffer settings now offers real
  choices instead of boxes to type in. **Model** lists the models your machines
  have listed for the chosen agent, with **Agent's default model** and
  **Other…** for a name of your own; **Effort** lists the levels your machines
  report for that agent, and shows **No effort setting** for an agent a machine
  runs that has none, such as OpenCode. An agent none of your machines can run
  is marked "(not on your machines)" but can still be chosen, with low, medium
  and high offered for it, so a crew can be set before a machine connects. A saved model or effort that none of your machines offers is
  kept and marked, with a line saying what happens at the start. Changing a
  row's agent clears its model and effort. If your machines cannot be read, the
  form says so and still saves.
- **Factory**: a stage whose chosen effort its agent does not offer for the
  model no longer stops: it starts on the agent's default effort and says so on
  the stage ("agent's default effort (high is not offered)"), on its **Applied**
  line and in the activity log. A model the agent does not offer still stops the
  stage. **Try again** and **Send back** ask for the chosen effort again, so a
  machine that has since been updated gets it, except while an earlier start of
  that stage is still unanswered; so do Gaffer's own decision to run a stage
  again and the restart of a stage on its backup. A machine whose reet is too
  old to set an effort at all is still refused, with the line telling you to
  update it. Every backup of a stage, not only the first, now drops an effort
  its machine does not report before it is started; when what the machine
  offers could not be read, every effort is sent as chosen.
- **Home**: with nothing in flight, Home says **Nothing in flight.** and has
  three buttons in a row, **Give Gaffer an outcome**, **Start a session** and
  **Open terminal**, with **Plan a session** under them. The illustrated cards
  and their explanations are gone.
- **Factory**: with no pipelines yet, the page is one line, **No pipelines
  yet.**, and the **Give Gaffer an outcome** button, on every screen size. The
  brief to fill in, the example outcomes and the job sheet are gone (on a phone
  they had been squeezed into two thin columns side by side); the outcome
  dialog asks for everything.
- **Home**: Home now opens on **Waiting for you**, one list of everything that
  needs you, Factory pipelines and sessions together, the one that has waited
  longest at the top. Each entry says what it needs (a decision, an approval,
  a merge, a question, a failure or work to review) and has its button on it,
  so most things can be dealt with without opening anything: **Move to next
  stage**, **Approve the merge**, **Allow once** and **Deny**, **Answer in
  chat**. On a wide screen an entry is a row; on a tablet or phone it is a
  card. A pipeline shows its stages and, when Gaffer advised, what it
  recommends and why. **Details** opens the pipeline on the Factory page. A
  session that has finished is no longer in the list; it is marked **not seen
  yet** in Continue working and on the board until you have looked at it. A
  Gaffer goal that needs your review on a session with no pipeline is in the
  list too. **Factory, working without you** and **Your machines** sit under
  it and start closed; each says what is inside when closed. Which sections are
  open is remembered in your browser, and only **Waiting for you** is open the
  first time. Continue working and the Planning board are as they were. On a
  tablet or phone a **Projects** button narrows the list, as it does on the
  Factory page. The bell now lists the Factory's items too, and its number and
  the page title count by the same rule as the list on every page (with a
  project chosen on Home, the list shows that project's entries while the bell
  and the title count everything). A question a pipeline's agent asks stays in
  the list beside a merge or a decision that does not answer it. When the
  Factory or Gaffer's goals cannot be read, Home says so and keeps what it last
  read instead of saying nothing is waiting. After an entry is answered, marked
  seen or dealt with, the focus moves to the next entry rather than being lost.
  The bell and the Factory page refresh each other at once after an action on
  either. The Sessions list's **Needs you** filter now leaves out sessions that
  only finished, as the bell does. Gaffer's floating button now
  floats over Home and the Factory page and keeps off the buttons on an item.
  The Factory page no longer reserves a strip at the foot of the page for it,
  and after archiving a pipeline from a narrow screen's detail, one press of
  Back leaves the page.

- **Single sign-on**: the switches about your people are now settings the owner
  changes on **Admin, Users**, not environment variables you restart for: new
  people created automatically, linking by email, the role for a new person,
  password sign-in, the button words, the claim names, the scopes and the
  admin, member and viewer groups. A saved change applies to the next
  sign-in, with no restart. Only the owner can save them; administrators see
  them read-only. Linking by email still never links an owner or an
  administrator. The provider's address, client id, secret and redirect
  address, and `REET_OIDC` itself, stay in the environment.
  **Upgrading:** the first start after this reads the `REET_OIDC_*` switches
  once and stores them; from then on the stored values are used. If a variable
  you left set differs from what is stored, the log says `single sign-on:
  REET_OIDC_… is set and differs from the stored setting; the stored setting
  is used` (naming the variable, never its value). `PATCH /api/v2/admin/sso`
  (owner only) takes the whole set, and `/api/version` lists `sso-settings`
  under `features`. See [single sign-on](docs/single-sign-on.md).

- **Factory**: **Other sessions**, once it has opened by itself because a
  session needs you, stays open for the visit. Stopping or answering the last
  such goal no longer closes the section and takes the dialog you were in
  with it.

- **Desks**: the terminal toolbar no longer shifts or grows a scroll bar when
  the focused pane has a long name. The name has a fixed-width column, Attach
  and Skills keep their place (switched off where they do not apply), and the
  labels follow the toolbar's width.

- **Factory**: the page now opens on what is waiting for you, in a list beside
  one pipeline's detail. On a wide screen each item in the list shows its
  stages as dots with a caption, and its action is in the detail beside it. On
  a tablet each item names its stages and carries the action it needs (for
  example **Move to next stage**, **Approve the merge** or **Try again**); on a
  phone the stages are circles with a caption and the action is on the item
  too, so most things can be dealt with without opening anything. When Gaffer
  advised, the item says what it recommends and why. A tool approval is
  answered with **Allow once** or **Deny** on the item (tablet and phone) and
  in the detail (every width). **Details** opens one pipeline with one bar of
  actions at the bottom and one **Settings for this outcome** row for who
  decides, approvals and merge. An archived outcome's pipelines can be read
  and restored, and offer nothing else. **Working**, **Finished or stopped**
  and **Other sessions** open and close, say what is inside when closed, and
  are remembered in your browser; only **Waiting for you** is open the first
  time, and **Other sessions** also opens by itself while a goal in it needs
  you. On a tablet or phone a pipeline opens as its own screen and
  the browser's Back button returns to the list. Projects stay in the left
  column on a wide screen, with how many of each project's pipelines are
  waiting and working; on a tablet or phone a **Projects** button opens the
  same choice.
- **Desks**: on a tablet or phone whose on-screen keyboard overlays the page,
  an open desk now ends above the keyboard, so the terminal's last rows and
  its keys bar stay in view while you type. Before, the desk kept the
  window's full height and the keyboard covered the bottom of the pane.
  While you type in a pane, the site bar, the desk bar and the tool row step
  aside so the terminal gets the room; the tab bar stays, and everything
  comes back when the keyboard closes.

- **Desks**: **Layout** now works on a tab with one pane. It combines tabs:
  all of them at once, or the ones you tick in **Combine tabs…**, laid out
  side by side, stacked or as a grid. Nothing is restarted.
- Factory, from the first real run of a split pipeline: a stage's handover
  now carries the plan, design or findings themselves, since the next stage
  starts in a session of its own that cannot read the last one. Gaffer's
  report and decisions are shown a pipeline's whole brief (up to 2,400
  characters, and told in words when it is longer) and each part's name and
  brief, instead of calling a clipped brief unfinished and a split stage
  unexplained.
  A started outcome can now be put on YOLO, or taken off, from its card
  (**Switch to YOLO…** beside Approvals, after a confirmation): the session
  each pipeline has in hand is switched and every later stage starts that
  way; a pipeline set to merge by itself waits for you again until you
  confirm that for YOLO. A session the Factory ended because its pipeline
  moved on no longer shows as newly finished with something for you to do.
- **Desks**: the tab shown is now each window's own, so switching tab saves
  nothing and no longer moves another window on the same desk. **Move here**
  closes the pane the session left (a tab's only pane stays, empty). The
  floating Gaffer button is not shown on an open desk, and the tab list is
  cleaner for screen readers.
- **Desks**, a new tab: a saved set of terminal tabs, each split into up to
  nine panes on any of your machines, saved to your account as you change it
  and opened the same in any browser. Panes can be split, resized, zoomed,
  swapped, moved and laid out, named and coloured, with Alt+Shift keys for
  each. A pane that lost its shell says why and offers **Reopen** in its last
  folder (or **Reopen all**); opening a desk never starts a shell, and a
  pane's start-up command runs only when you press Run. **Type in all panes**
  sends what you type to every live pane of a tab. A running session can be
  added to a desk from Sessions or Home. Templates are listed under their own
  heading on the Desks page. Ending a pane's shell follows your Terminal
  history default. See `docs/desks.md`.
- Machines: updating reet on a machine adds `desk-shell-v1`. A desk pane's
  shell there reports the folder it is in, keeps its own bash or zsh command
  history under `~/.reet/desks/` on that machine (the History drawer shows
  **This pane** above **This machine**), and may reopen outside the home
  folder, but only in the folder that machine itself last reported for that
  pane. An older machine still works on a desk without these. When reet
  starts on such a machine it removes desk pane history files that no
  session there names and that have not changed for 90 days. Downgrading
  reet on a machine that has started desk pane shells can stop its terminals
  (the older version refuses the desk field in its session registry); update
  it again to bring them back.
- The browser's session limits are now 24 live sessions a machine and 64 an
  account (from 8 and 32).
- Factory: in Add pipelines each tier's backup can be your crew's, none, or
  one of that pipeline's own. The crew in Gaffer settings takes a billing
  source for a Cline tier or backup.

- Factory: a stage can be split into parts by hand, and a part added, in the
  outcome dialog and in Add pipelines; an MCP client can split a stage too.
- Factory: a tier can have up to three backups, tried in order, and a Cline
  backup its own billing source. A stage whose agent stops with its plan
  below your figure is run again on its backup, once; a working agent is
  never interrupted. Add pipelines can leave a tier's backups off.

- Factory: Gaffer can split a stage into parts of different work, each with
  its own brief and tier, so each part runs on the agent that suits it. Parts
  run one after another, each in a session of its own. You see and edit the
  split before anything starts.
- Gaffer settings has **Your Factory crew**: the tiers, their backups, when a
  backup takes over, and whether the outcome dialog starts on Gaffer decides.
  Added pipelines take each tier's backup from it.
- MCP: `reet_start_outcome` takes a stage's tier and `balance_below`. It
  cannot give Gaffer an outcome's decisions.
- A worktree a Factory pipeline still works in is kept by every kind of
  cleaning. A pipeline with no stages now says whether the outcome's agent
  and model were applied. A tool request Gaffer left waiting for you is
  closed after a day whether or not anyone answers it.

- Skills sync: a skill whose name is not its folder name ("SVG Logo
  Designer", installed as `svg-logo-designer`) now installs. Reet asked the
  `skills` CLI for it by folder name only, the CLI matches on the skill's own
  name, and every install of such a skill failed. Machines need the new
  agent build; they update themselves where auto-update is on.
- Skills sync: a failed install is reported as that, with the skill's name
  and what the installer said. It was shown as **The plan changed before it
  ran**, which sent you to preview again for a plan that had not changed.
- Skills sync: a skill the `skills` CLI installs under a different folder
  than it has in its repository keeps its source in a scan, instead of
  showing as an unlinked local copy beside a second, phantom entry.
- Skills sync: a skill the catalogue names differently from the folder the
  `skills` CLI installs it into ("UI/UX Design Review": `uiux-design-review`
  in the catalogue, `ui-ux-design-review` on disk) now settles. The install
  worked and the skill stayed **missing**, with a second unlinked skill beside
  it. The machine and the controller both read that folder as the skill you
  selected when it came from the same source.
- Skills sync: a copy another app holds, an entry that is not a skill, or
  your own unlinked copy is now seen when it sits under the folder the
  `skills` CLI installs into and not only under the catalogue's name. Such
  a copy is left alone and reported, or backed up before it is replaced,
  as one under the catalogue's name already was. It would have been
  overwritten.
- Skills sync: **Auto fix** now makes the plan ask for the copy already
  installed, as the skill it is. It changed only the source, so after the
  install the controller still listed the old skill as missing and the
  installed one as an extra. It also finds a copy held under the folder the
  `skills` CLI installs into when that differs from the catalogue's name.
- Skills sync: an **Auto fix** on a folder that follows a source now holds.
  The next poll used to write the source's own skill back, the machine
  refused it again, and the two took turns, queueing a job each time. The
  folder keeps following the copy while it is on the machine and returns to
  its source when that copy is removed.
- Skills: the **Copy prompt** text (`npx skills use …`) names a skill by its
  own name when that is not its folder name, as the copied install command
  does. The folder name found nothing for such a skill.
- Skills: every command reet copies out (the install command, the **Copy
  prompt** text, a published skill's page and the export) names the `skills`
  CLI version that sync runs on a machine, `skills@1.7.4`, instead of
  whatever is newest. A pasted command now behaves like the one reet ran.
  Reet still accepts an unpinned or differently pinned command pasted in.
- MCP: `reet_list_sync_targets` returns a target that is too large for one
  response with its largest lists replaced by counts (`desired_count`,
  `lists_counted`), instead of failing with "one record exceeds the response
  limit".

- A session's right-hand **Progress** tracker now shows a pull request created
  in the session within about 20 seconds while the agent is working, instead
  of up to two minutes later: the visible session's GitHub read runs every 20
  seconds while it works and every two minutes while it is quiet.

- Sessions: an agent's answer is now shown as the markdown it wrote. Tables
  are tables, lists nest and keep their task boxes, and italics,
  strikethrough, rules and every heading level are drawn rather than left as
  raw symbols. The same goes for Gaffer's reviews and reports. HTML in an
  answer is still shown as text, and only http and https links are links.

- Factory: on an outcome set to **Gaffer decides**, Gaffer now answers a
  working agent's multiple-choice question, choosing among the options the
  agent offered, or leaves it to you with its reason. It never answers an
  approval or a question that needs your own words.

- Factory: each crew tier can have a **Backup** agent, model and effort. A
  stage starts on its backup when its first choice's plan has under 20% left
  (a figure you can change, from 5 to 90) and the backup's has more, so work is spread across the plans you have.
  The card says **Balanced** with the reason. It needs plan limits to be on.

- Factory: every decision a pipeline is waiting on you for is a set of
  buttons with the recommended choice first, at the head of the outcome's
  card and in the pipeline's session as well as on the pipeline. When
  Gaffer's review brought a stage back, the recommendation is Gaffer's,
  with its reason.
- Factory: an outcome can be set to **Gaffer decides**. When a stage stops,
  Gaffer then chooses what happens next (review again, tell the agent, move
  on, a new budget or a new session) and the card lists what it decided and
  why. It is off unless you choose it, takes at most three decisions on a
  stage, and never covers a merge or an agent's own question or approval.

- Factory: every stage of a pipeline now runs in an agent session of its
  own. Two stages with the same agent, model and effort used to share one
  session; the later stage now starts fresh from the earlier one's handover,
  and the earlier session is ended.
- Factory: the crew is now three tiers, **Heavy**, **Standard** and
  **Light**, in place of Thinking, Building and Shipping. When Gaffer plans
  an outcome it says how demanding each stage is, with a reason where a
  stage is harder or easier than usual; the plan arrives with that applied
  and marked, and one click puts a stage back. Gaffer still names no agent
  or model. **Save as my crew** keeps your tiers for the account, and later
  outcomes, including ones started over MCP, begin from them.

- Factory: a pipeline that was waiting at **Ready to merge** when you stopped
  it waits there again when you try it again. It used to start its merge
  stage without your approval; so did a pipeline whose first stage was the
  merge. Stopping an outcome while a session is still starting now stands:
  the session is listed, and nothing follows it. An archived outcome has to
  be brought back before a pipeline in it is tried again, sent back or added.
- Factory: each stage now says whether the agent, model and effort you chose
  were **applied**. Once the stage's session is running the controller asks
  the machine what it is on and shows it on the card: Confirmed (naming the
  model and effort, including one you left to the agent), Not as chosen
  (naming what differs) or Not checked. A later change made in the session's
  own chat shows too. "Who did each stage" now lists effort.
- With plan limits on, what is left of each agent's plan now shows where the
  work is, not only in a machine's settings: on **Home**, the **Sessions**
  toolbar, a chat's status strip and a Factory pipeline's card. Each account
  is a chip with the agent's mark and the share left of its tightest window;
  a mouse over it lists every window, and a click opens **Plan limits** with
  each window's reset, the account, and which machine read it and when.
- Focus mode on a phone shows less around a chat, so the conversation has the
  screen while an agent works and messages are queued: one header row, the
  agent's state said once, the queue as one line that opens, and the
  composer's Prompts, Skills, image, dictation, effort and model behind
  **Exit focus**. Upright, the conversation goes from about a third of the
  screen to about two thirds; sideways, from one line to most of it.
- A message waiting for an agent to finish its turn no longer gets stuck
  behind an earlier message that failed, and an agent that turned out to be
  busy no longer turns the message into a failure you have to retry: it goes
  when the turn ends. This is what made Gaffer's next instruction wait until
  you pressed **Stop and send**.
- MCP: on an outcome that runs on YOLO, trying a pipeline again, moving it on
  and approving its merge are refused, as telling its agent, adding pipelines
  and sending back already were.
- Factory: a session that started but did not answer in time is taken up when
  you try the pipeline again, instead of a second agent being started in the
  same folder.
- MCP: `reet_approve_pipeline_merge` now needs `seen_updated_at`, the
  pipeline's `updated_at` from the read the approval is made on, and approves
  nothing if the pipeline has changed since. `reet_tell_pipeline_agent` and
  `reet_stop_outcome` answer `applied: false` when nothing was queued or
  changed, and `reet_start_outcome` is refused for a machine that has not
  reported that it can run the agent. It takes an optional `request_id`, so a
  retried call starts the outcome once.
- Gaffer's tools: after Gaffer reads one outcome in full in a chat turn, a
  tool that changes something asks you first for the rest of that turn, even
  on Allow.

- **Ending a session that a Factory pipeline is working in now puts the
  outcome away with it.** The end dialog defaults to stopping following the
  outcome and deleting it, with toggles to archive it instead or leave it
  alone. Account → Profile sets the default for every session; the dialog
  still lets you choose for one. Deleting erases the outcome's report and
  totals, not the sessions or pull requests it started.
- **Dictate** in a Gaffer chat's composer, beside Attach, where the browser
  has speech recognition. As in a terminal's Compose, nothing is sent by
  speaking, and the browser sends the audio to its own speech service.
- On a phone with a notch held sideways, dialogs, the terminal's drawers,
  the Gaffer panel and the save and sync docks no longer run under the
  notch, and a dialog stops above the home indicator.
- Factory: a stage instruction or Gaffer message longer than 2,000 bytes now
  reaches the agent. It used to stop on the controller and show **Delivery
  was not confirmed**, which left the goal waiting for you at every stage that
  carried on in the same session. Longer messages need this build on the
  machine as well; an older machine's message now fails with that reason. A
  message that never left the controller is shown as failed, not uncertain.
- **Compose** in a terminal: write or dictate a message in an ordinary field,
  where a phone's autocorrect and dictation work, then **Send** it as one
  paste with Enter, or **Insert** it without. It is in the terminal toolbar.
  **Dictate** uses the browser's speech recognition where it has one; the
  browser sends that audio to its own speech service, not to reet.
- Home's Continue working and the Sessions list can be grouped by
  **Attention**: Needs you first, then what stopped with a problem, what
  finished, and what is still going.
- A device with notifications on now hears when a chat finishes its turn and
  waits for your next message, not only when a session ends. The choice is
  now called **A session has finished**.
- On a phone, an open approval or question is no longer said three times in
  a chat; the strip above the composer waits until it is answered.
- reet runs edge to edge on phones with a notch or a home indicator
  (`viewport-fit=cover`), keeping everything you read or press clear of both.
- **Notifications on a device.** A phone, tablet or computer can be told when a
  session needs you or has ended, while reet is closed: **Notifications → On
  this device → Turn on notifications**, then choose what that device hears
  about. A tap opens the session. Nothing about a session passes through the
  browser's push service: the device is woken with an empty request and asks
  your controller what to show. It needs accounts, https, and outbound https
  from the controller to the browsers' push services; on an iPhone or iPad,
  add reet to the Home Screen first. See
  [Notifications on a device](docs/push-notifications.md).
- **Sessions on a phone open full screen.** A terminal now opens in focus
  mode, as a chat already did: the site header, the workspace toolbar and the
  pane's own header and footer step aside, and **Exit focus** brings them
  back. At 390 × 844 a terminal has 49 rows where it had 28, and 21 held
  sideways where it had 9. **Focus** did nothing on a terminal before. With
  the keyboard up the header is one row, and a chat's model and effort
  settings wait until the keyboard goes.
- **Ask Gaffer** no longer floats over a session on a phone or a touch tablet,
  where it covered the agent's prompt line above an empty strip kept for it.
  It is in the session's header (a terminal) or status row (a chat).
- **A touch tablet with its keyboard up** gives the terminal the room a phone
  does: 30 rows at 820 × 1180 where it had 13.
- **The Sessions tab** with nothing open lists every machine's running
  sessions, what needs you first, each with its machine and state. It said
  "Nothing is open right now" while sessions were running on a phone.
- The sessions drawer on a phone is wide enough to read a session's name and
  closes with a tap beside it. In a docked panel a project's header is one
  line, and beside it a session's name is no longer cut short while there is
  room. On Home, the board's add, density, sort, search and filter controls
  are 44 px on a touch screen. **Start an agent** no longer shows a choice not
  made yet in red.
- Home's launch bar offers **A repository from GitHub…** in its Project
  list, as Start an agent does: pick a machine and a repository, and the bar
  names it (cloned as a temporary project, or the project that already is
  it). Press Start when the prompt is written.

- A repository added as a folder now shows the GitHub repository it pushes
  to on its card, as one added From GitHub does. reet asks the machine for
  the folder's origin when the project is added or moved, and once a day.
  Picking that repository in Start an agent or Factory starts in your folder
  instead of cloning it again.

- **Start an agent** and Factory's **Give Gaffer an outcome** can start in a
  GitHub repository without adding a project first: choose **A repository
  from GitHub…** in the Project field, then the repository (from the
  machine's list, or typed). reet clones it on that machine and saves it as a
  project as you start (or uses the project already cloned from it), then
  starts there as on any project, worktree and base branch included. Such a
  project is temporary: once its sessions or outcome are done reet removes it
  and deletes the clone, but only when the machine finds nothing that would be
  lost (uncommitted files, a stash, unpushed commits, a worktree still there).
  Otherwise its card says why it stays. **Keep this project** in its menu keeps
  it for good.
- **From GitHub** now lists the repositories the chosen machine's GitHub
  sign-in reaches (its own, its organizations' and those shared with it, the
  200 most recently pushed) under the Repository field. Type to filter, then
  choose with the arrows and Enter or a tap; anything typed as owner/name or a
  github.com link still works. When nothing in the list matches what is typed,
  the field says so and still accepts the typed name. The machine asks GitHub with its own `gh`, so
  the list is what that machine can clone. Both the controller and `reet agent`
  need this release; a machine on an older reet keeps the plain field.
- A project added from GitHub now shows where it came from (`owner/name`) on
  its card in the project rail, in the Sessions screen's project list and in
  Manage projects. It is still called a project, like any other folder on a
  machine; a session's **Where it's working** already named its repository.
- **Add a project** on the Sessions screen now offers **From GitHub** at every
  width. Its desktop project list opened a copy of the dialog without it.

- Leaving a chat no longer risks dropping its machine's connection. An open
  chat asks for its live controls every 1.5 seconds and abandons the request
  when you leave it; on a busy controller the request was often still waiting
  to be sent, and sending it could close the connection that all of the
  machine's terminals and chats share. Everything on the machine detached until
  it reconnected, and Gaffer's goal follow-up, finding the machine
  reconnecting, waited a minute before looking again. Any request that has
  given up now sends nothing, and a message already being written finishes
  under its own five-second limit. The controller and `reet agent` each
  protect their own end of the connection.
- A machine no longer loses its connection when it answers which of its
  agents are signed in. The controller ended the connection on that answer,
  so opening the sign-in list could drop the machine and its sessions for a
  moment.
- Removing a project reet cloned from GitHub can now delete the clone too.
  **Remove from reet** asks the machine what the clone holds and, when nothing
  would be lost, offers a tick, off by default: "Also delete
  ~/reet/repos/owner/name from studio". Files git ignores there, such as .env
  or build output, are deleted with it. The tick is not offered, and one
  sentence says why, when a session still has a worktree from the clone, when
  a session is still running in the clone's own folder, when
  it has uncommitted files, a stash or commits that aren't on GitHub, when it
  holds something reet can't check (a submodule or another repository inside
  it, a mounted folder, a rebase under way), when reet can't be sure it is
  still a plain clone of that repository (the machine's git settings can
  cause this), when reet didn't make the folder, or when the machine can't be
  asked. A `reet/…` branch with commits that were never pushed keeps the
  clone after its session is gone.
  The machine looks again before it deletes: if the clone changed since the
  dialog, nothing is removed and the project stays. If the machine's answer is
  lost, or a proxy in front of reet gives up waiting, reet says it couldn't
  confirm, keeps the project, and records in Activity that the machine was
  asked and the delete could not be confirmed. reet then says only what it
  knows: "reet finds no clone it can delete at ~/reet/repos/owner/name on
  studio." Removing without the tick works as before and never touches the
  folder, and reet never deletes a folder it did not clone. Activity records
  the deletion with the folder, and for a project reet cloned that is removed
  without the tick it says that removal did not delete the clone, not that
  the folder is untouched. The machine needs a current reet for this; an
  older one keeps the clone.
- Adding from GitHub: if starting the clone gets no answer in 15 seconds the
  dialog now says "reet didn't hear back about starting the clone. Check the
  project list in a minute." instead of saying the request never arrived;
  pressing Escape repeatedly while the start is on its way no longer hides the
  dialog; and when reet loses track of a clone that had finished, the project
  simply appears, without the "lost track" message. A machine that refuses a
  clone now says "studio refused to clone: terminal access is off there, or git
  or the GitHub CLI is missing." instead of naming terminal access alone.
- Machines now follow a moved controller without anyone touching them, and
  the Move page shows them do it. The `reet agent set-server` command is gone
  from the page. A fence raised before the new address was known can be given
  it later, or corrected, without lifting the fence (**Change the address**,
  or `PATCH /api/v2/admin/fence`). A machine keeps the address the fence named
  in its enrollment (`movingTo`), so it still gets there after its agent
  restarts or after the old controller is switched off. Once it has moved it
  tells the old controller, and **Machines follow on their own** lists every
  machine as moved, waiting for the new controller, not asking right now, or
  needing a hand (following turned off, or an `http://` address it will not
  follow). The page warns about an `http://` address before you fence. The
  safety rule is unchanged: no credential goes to an address until it proves
  it holds the machine's record.
- The terminal's on-screen keys are rebuilt, and tablets get them. They were
  hidden on any touch screen wider than 650 px, so a tablet had no Esc, Tab,
  Ctrl or arrow keys; they now show on every touch screen. The row leads with
  Esc, Tab, Ctrl, Alt and Shift in plain key names, and the four arrows stay
  in view at its end instead of scrolling away. Tap a modifier for the next
  key, tap it again to lock it, and a third time to let go; several can be on
  at once, and they join what the device's own keyboard types as well. Hold an
  arrow, Backspace, Delete, Page Up or Page Down and it repeats. The other
  keys (Enter, Home, End, symbols, Ctrl combinations, F1–F12) are a second row
  that opens above the first, in place of the panel that floated over the
  screen and cut its key names short ("Ho…", "Pg…"). On a touch screen a key
  no longer opens the device's keyboard, so the arrows work with it down. A
  modifier that was on also drew its name in the colour of its fill until the
  pointer was over it; it can now be read.
- A machine now says which of its agents are signed in. Its Settings →
  **Agents** shows, for Claude Code and Codex, whether an account is signed in
  there, whose it is and which plan it is on, read from the agents' own files
  on the machine; **Check sign-ins** reads them again. Starting an agent on a
  machine where no sign-in was found says so first, where before the first
  message was simply refused. A sign-in kept in the macOS keychain or a system
  keyring is not opened, and is shown as not readable or not found, never as
  signed out.
- Plan limits, off until you turn them on. With **Settings → Agents → Plan
  limits** on, each of your machines with terminal access on also reads how
  much of each Claude or ChatGPT plan is left (the session window, the week,
  and a model's own weekly window) and when it starts again. The machine asks
  the agent's provider itself, with the agent's own sign-in; that sign-in
  never leaves the machine and is never renewed by reet, and the controller
  keeps only the account's name, the plan and percentages. A token that has
  run out is reported as needing renewal: start the agent on that machine
  once. The providers' endpoints are the ones the agents' own clients use and
  are not published interfaces, so a changed answer shows as a failed read
  with the last figures kept. Both machines and the controller need updating.
- You can start GitHub Copilot in a terminal session. Install the Copilot
  CLI's `copilot` command on a machine, sign in there with `copilot login` and
  update reet on that machine; the agent picker then offers **GitHub Copilot**
  with an optional model, an opening prompt and a thinking level. Approvals
  stay on unless you choose YOLO, which needs the same account setting, machine
  switch and confirmation as for every other agent. An opening prompt goes
  through the CLI's interactive option, not the one that answers once and
  exits. Not there yet: a Gaffer chat for it, **Continue session…** on a closed
  session, and a model list read from the machine (type the model name under
  **Other…**). reet doesn't install the CLI or sign in for you. Both the
  controller and the machine need this version: an older one is never sent the
  new agent's name, so Copilot just isn't offered there until it is updated.
  The arguments reet starts it with were taken from the help of Copilot CLI
  1.0.112; they have not been run with a signed-in account.
- An agent in a terminal that stops to ask now shows as needing you. Claude
  Code, Codex and OpenCode do not always ring the terminal bell when they ask
  before running a command or changing a file, so such a session looked the
  same as one still working. The machine now reads the visible screen of an
  agent's terminal session and, when the agent's question is on it, records
  that as it records a bell: the session counts under **Needs you** on Home,
  in the bell and in the sessions panel until you open it. The screen's text
  is matched on the machine and dropped; nothing of it is stored or sent. A
  plain shell is never read. Only the machine needs updating. The patterns
  were written from how these agents draw their questions and have not been
  checked against a signed-in run of each, so a question in a shape they do
  not know is missed, as every question was before.
- A chat now shows what its agent reports using. The status line above the
  message box ends with how much of the agent's context window is left, or
  the chat's tokens when the agent never says how large its window is, and
  marks the figure when 15% or less is left. The numbers were already kept
  for the Factory page; an ordinary chat showed none of them. An agent that
  reports nothing shows nothing.
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
  Removing the project leaves `~/reet/repos/owner/name` on the machine unless
  you ask for it to be deleted (below); the
  hidden folder a clone cut off part-way leaves is removed the next time that
  repository is cloned, but only when reet can tell it is that repository's.
- An agent whose binary has only just been written, by an install or an
  update, is no longer refused as having "no version". Starting it could fail
  for an instant with "text file busy"; the version check now waits that out.
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
