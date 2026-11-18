# Usage

**More → Usage** is one page for every agent: how much of each signed-in plan is left, and the tokens its sessions
and Factory stages reported today, over the last 7 days or over the last 30 days, with the sessions behind the figures
and Gaffer's own model requests. Every agent always has a block. One that has nothing to show says why in a sentence,
so the page is never blank.

It is for accounts that can start sessions (the same accounts that have the Sessions page). A viewer has neither.

## What the page shows

- **The period.** **Today**, **7 days** or **30 days**, counted in your own time zone (your account's profile): today
  starts at your local midnight, 7 days at local midnight six days before, 30 days likewise. A day on which the clocks
  change is as long as it really was. The browser remembers your choice. Under the title, one line gives the period's
  total, for example *1.6M tokens across 21 sessions* (a session whose agent changed during the period is one of
  them, though it is counted under each agent below); when the period starts before reet began counting it adds
  *counted since 18 Nov*. With nothing counted at all it says *Nothing has been counted yet. reet counts what your
  agents report from now on.*
- **One block per agent**, in the order reet lists its agents. Each starts with the tokens its sessions reported
  in the period, which for an agent with no plan to read (OpenCode on a hosted model, say) are the only figures there
  are.
  - **Plan left.** Each account signed in for the agent across your machines, with its plan and each window in
    words (*Session 81% left · resets in 2 h 10 min*, *Premium this month 70% left*) and a meter, and which machine read
    it and when. The same account on two machines is one line, from the machine that read it last.
  - **Where there is no figure**, one sentence chosen from what the machines answered for that agent: no connected
    machine reports sign-ins; *No sign-in found on studio*; signed in with an API key, or to an account with no plan
    allowance to read; *Claude Code's sign-in on studio has run out; it renews itself the next time Claude Code runs
    there* when the sign-in has expired (reet never renews it); the provider asked reet
    to slow down or could not be reached (the last figures stay, dimmed, under that line); terminal access is off on
    the machine; or reet on the machine needs updating. Only when no machine answers for the agent at all (its reet is
    too old to read that agent, or reet has no reader for it) does it say *reet can't read this agent's plan*. Which
    agents reet can read a plan for is listed in [agent sign-ins and plan limits](agent-accounts.md).
  - **Tokens.** *1.2M tokens*, then *14 sessions · 3 Factory stages*, with input, output and cached tokens on a
    third line.
    An agent that reported nothing in the period says *No tokens reported in this period.* GitHub Copilot says it runs
    only in a terminal here, which reports no token counts. An agent reet no longer starts appears, after the rest,
    only when it has figures in the period, and says *reet no longer starts* it and that the figures are from
    sessions it ran before.
- **Gaffer.** Gaffer's own requests to its model provider in the period, per provider and model, with the number of
  replies; or *Gaffer made no model requests in this period.* These are not added to any agent's figures.
- **Sessions in this period.** Each session that reported tokens, latest first: its title (a link to the session),
  agent, model, machine, tokens, how much context was left at its last report and when it was last active. A table on
  a wide screen, cards on a phone. A Factory stage's session is marked **Factory**; a session removed from the history
  keeps its title, without a link, and says so. At most 200 are listed, and the page says when there were more.

The page reads its figures when it opens and when you change the period, then quietly every minute while it is in
view. A read that fails says *Usage could not be read.* with **Try again**, and keeps the figures it had.

The **Plan limits** panel opened from the plan chips (on Home, Sessions, a chat and a pipeline) links here with
**See all usage**, and so does **Settings → Agents → Usage**.

## What the figures are, and their limits

The figures are the agents' own counts, passed on as numbers. reet attaches no price.

- **They start when this version is installed.** Nothing before it was dated, so earlier use is not in any period.
- **Only chats report.** Claude Code, Codex, OpenCode, Cline and Google Antigravity report tokens in a chat. Plain
  terminals, GitHub Copilot and machines whose reet does not report usage count nothing.
- **A report made while a machine is disconnected from the controller is lost.** Reports are not queued.
- **Factory marks rely on the pipelines the controller keeps** (the latest 30). A stage's session from an older
  pipeline is still counted, but is no longer marked Factory or counted as a Factory stage.
- **Gaffer's figures go when a Gaffer conversation is deleted.**
- **Counts are kept for at least 90 days.** Rows older than 90 days are removed the next time that account reports
  usage (a few hundred at a time), so an account that reports nothing keeps its older rows until then. The 30-day
  view is always complete once reet has counted for 30 days.
- Removing a session from the history, forgetting its machine or revoking the machine does not remove what it
  reported; deleting your account does.

## For administrators

The page reads `GET /api/v2/usage?from=<ms>&to=<ms>` (a browser account session, same origin; it answers while
remote terminals are off, as the session list does, and refuses viewers). The range must end after it starts and be
at most 92 days long. The answer is
`{from, to, since, sessionCount, agents, gaffer, sessions, more}`. `sessionCount` is the sessions that reported in
the range, each once. `agents` lists every agent the controller starts, zeros included, each with `reports`: `chat`
(it reports tokens in a chat) or `terminal_only` (it has no chat here); after them comes any agent the controller no
longer starts that has figures in the range, with `reports: retired`. Each token report is kept in the controller's
database (`terminal_usage_events`, with the session's latest title, machine name and project in
`terminal_usage_sessions`); events older than 90 days are removed, a bounded batch at a time, when the same account's
next report is written.
