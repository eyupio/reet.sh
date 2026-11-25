## STUCK-STAGE-1: a failed turn no longer wedges a Factory stage, and the session's decision leaves room for the chat (25 November 2026)

A person reported, with screenshots from a tablet (about 714 px wide), a Factory pipeline whose plan stage could not go
on: the agent's first turn failed, Gaffer's request for the plan "came back as queued in a blocked state", the session
page showed the decision but the composer was off the bottom of the screen and nothing scrolled, and the stage had run
on a model they thought they had undone. Four causes, each fixed where it was.

What was wrong, and what changed.

- **The wedge.** `stepChatQueue` (`internal/terminalbroker/chat_queue.go`) fails a queued message with intent `wait`
  when the chat's last turn failed, so that a person's own queue does not drain into a broken agent until they press
  Retry. A companion message (Gaffer's instruction, **Tell the agent…**, a connected tool) took the same path, and
  `ChangeTerminalQueueFor` refuses every action but Remove on a companion message, so nobody could retry it: after one
  failed turn the stage's agent could not be reached again. A companion message is now not paused by a failed turn. It
  is the retry. The person's own queue behaves as before (`TestQueuePausesAfterAgentFailureUntilExplicitRetry`).
- **No way out.** A stage that waited for the person with its session still open offered review again, tell and move
  on, never a new session (that was offered only once the session had ended). `needsYou` in `gafferOutcome.ts` now adds
  **Restart stage…** to those three lists, after Move on and never as the recommendation. In the session it is one of
  the decision's buttons. On the Factory detail it is a link in the **Waiting for you** card with a sentence saying what
  it does, not a fifth button: five do not fit the bar on one line at 1280 or 1440 px even with a short label, and
  `factory-pieces` holds the bar to one line there.
  `RestartPipelineStage` (`internal/gaffer/outcome_manage.go`) does it: it is refused unless the pipeline is following
  the named session and its goal is `needs_user`, not waiting on the worker and with no review running, so a working
  agent is never ended by it. It then goes through `retryPipeline` (which now takes the session being left and treats it
  as ended), stops the old goal and ends the old session. The retry route takes it as `endSession` in the body. The
  page asks first, in `PipelineChoices.svelte` and `SessionDecision.svelte`, with the same words (`RESTART_BODY`).
  Gaffer was not given this choice for outcomes whose decisions it takes: its tell now lands, and a restart stays the
  person's.
- **The session page.** The decision sits in `.below-head` above the conversation. With a long instruction it took most
  of the height, and the chat's rigid parts (status, queue, composer) were clipped by `.stage{overflow:hidden}`: at
  714 × 1023 the composer was at y 1201 to 1245, at 390 × 844 at 1145, at 1280 × 900 at 1056. Now, when a decision is
  shown, the grid gives the conversation at least `min(26rem, 40dvh)` and the decision the rest, scrolling itself, with
  its choices sticky where they sit in a row (above 560 px). `Decision.svelte` has **Minimise** / **Show decision** and
  clamps the instruction to three lines with **Read all of it**. In `ChatPane.svelte` the queue (and only the queue)
  may shrink to 3.5rem and scroll when the pane is too short, instead of pushing the composer out. The phone rule that
  capped the block at 55dvh is gone, since the grid does it at every width.
- **The model.** In a new outcome, **Use Standard** under a stage moves that stage to the Standard tier; it was read as
  undoing the model picked for Heavy, which stayed and ran Plan. The form was checked in a browser and sends what it
  shows. The button now reads **Move back to Standard tier**, and a tier changed in **Choose who does what** has
  **Back to your saved crew** (or **Back to the outcome's agent**), which runs the same start-up code for that one tier
  (`startTier` in `GafferOutcome.svelte`). Backups are left as they are.
- On the Factory page's detail, **Waiting for you** now comes before the skill suggestions, which on a phone pushed it
  off the screen.

Checked.

- Go: `TestCompanionInstructionIsDeliveredAfterAFailedTurn` (failed before the fix with the receipt `blocked` and the
  note "The previous turn failed…", exactly as reported) and `TestRestartingAStuckStageEndsItsSessionAndStartsANewOne`.
- Web unit tests and `svelte-check`.
- Browser, against a built UI: `factory-decision` (minimise at 1440 and 390; a long instruction beside a queued
  message at 714 × 1023, 390 × 844 and 1280 × 900 with the composer wholly in view; the restart's confirmation, Cancel
  sending nothing, and the body sent), `gaffer-outcome` (the tier link appears only on a changed tier and puts it
  back), `gaffer-factory` and `factory-pieces` (the card's link and its confirmation, four choices on one line), and
  these, which pass unchanged: `gaffer-factory-session`, `factory-delivery`, `factory-templates`, `home-waiting`,
  `home-followups`, `gaffer-goal`, `stage-follow`, `gaffer-followed`, `gaffer-floor`, `gaffer-worker`, `chat`,
  `chat-controls`, `phone-drafts`, `terminal-focus`, `terminal-question`, `question-card`, `sessions-phone`,
  `phone-layout`, `calm-session`, `session-workspace`. Light and dark were both used; the other browser suites were
  not run locally.

Not checked, and left out.

- No real agent or machine was used: the failed turn, the restart and the ending of the old session were exercised
  with the Go fakes only.
- The Add pipelines dialog has its own crew rows and got no reset link.
- On a phone with a queued message and the decision open, the queue shrinks to its smallest and the log to a sliver;
  Minimise is the way to read the conversation there.
