## FACTORY-REVIEW-1: the nineteen findings of a review of the factory and Gaffer are fixed (25 November 2026)

A review read the factory (outcomes, pipelines, the MCP tools) and Gaffer (chat, goals, worker tools) in four parts, about
24,000 lines without tests, and its main claims were checked in the code before anything was changed. This entry records
what changed for each finding, the places where the fix differs from what the review proposed, and what was not checked.
The load assumed was one controller, a few dozen people, ten active goals each and one SQLite connection.

### What changed

Store (`internal/store`).

- **A full account reuses a place** (`CreateGafferConversationFor`). Each pipeline stage takes a Gaffer conversation and
  only Remove gave one back, so a busy account hit 100 and its next stage started unfollowed. At 100, the oldest
  conversation whose goal is stopped or completed and that nothing has touched for a day is deleted (its goal and
  grants go with it by the existing cascade and triggers), and only then does the account count as full. A chat with no
  goal, a goal that can still go on, and anything touched in the last day are never removed.
- **The ten-goal limit has its own error and one check.** `ErrGafferGoalLimit` and `MaxActiveGafferGoals` replace four
  copies of `count >= 10` that returned the chat-busy error ("already answering this conversation"), and
  `ActiveGafferGoalsFor` counts what an account follows.
- **Pause and complete refuse a goal that is over** (stopped or completed), so a stale tab cannot turn it into one that
  resume would revive.
- **An unchanged idle observation moves only the next look.** The supervisor saved "Waiting for new worker progress"
  every 15 seconds and bumped the goal's updated time each time. It still costs one small write per check, so the
  write rate of idle goals is the same; only the churn of the updated time is gone.
- **The six outcome lists share one reader** (`gafferOutcomesWhere`). The automated and backed lists skip outcomes with
  no followed pipeline (their loops skipped those anyway), and the report list skips archived outcomes, which could be
  asked for a report after being put away.
- **A reused request ID is an answer or a conflict.** The ID is the outcome's primary key. Asked for twice at once by one
  account it now returns that account's outcome, and one another account used is a conflict, where both were a raw
  `UNIQUE constraint failed` and a 500.

Gaffer service (`internal/gaffer`).

- **The ten-goal room is checked where plans start.** `CreateOutcome` and adding pipelines refuse when
  `active + new > 10` (a retry of a create that worked still returns its outcome), so the MCP start tool is held to the
  limit the browser dialog already showed. A pipeline that has no room waits, with a note, and starts when a goal is
  finished, stopped or paused, instead of launching an agent nobody follows. The wait polls every 2 seconds inside the
  outcome's own runner; returning false would have made the loop wake itself without end.
- **A chat message sent again inside a pipeline stage gets its saved turn.** `pipelineChat` is worked out before the
  replay check, and the "relay" comparison is skipped for it. The stored turn carries the grant revision for a pipeline
  chat even when the send was not a relay, so every resend read as a conflict.
- **A busy provider gate spends nothing.** A decision refused with `ErrBusy` is released (the claim and the stage's
  count) rather than recorded as "Gaffer could not take this decision", and an automatic report's mark is cleared.
  While the gate stays full, the claim and release write the outcome each 10-second tick until it frees.
- **The browser can send `requestId`** (`OutcomeInput.RequestID` is `json:"requestId"`), the replay path already existed.
- **Credential masking** takes `[a-z0-9_]*` around its names and an underscore-joined suffix, skips a `bearer|basic|token`
  word before the value, matches any `scheme://user:pass@` and `sk_live_`, `rk_test_` style keys. "tokens", "passwords"
  and "secretary" are still left alone. It is still best effort, as `docs/gaffer-ai.md` says.
- **Slots.** Chat is allowed 3 turns per account and 16 for the controller, goal reviews 2 and 12, so reviews cannot
  crowd out typing to Gaffer.
- **Local models** get 2 minutes for response headers instead of 15 seconds (hosted providers keep 15).
- **Pipeline details are shared only with status.** `contextSnapshot` adds a pipeline's stages only when the grant allows
  status. A pipeline's stage conversation gets its grant in `SetupGafferGoal` with `status = sharing.Status &&
  p.AllowStatus`, so a pipeline chat or review is now told where its session sits only when the person allows status
  sharing. `ContextPipeline`'s own comment said that all along.
- **The goal text of a long part** clips the part brief to 400 bytes (it was 600), so an 80-character title in a script
  with wide characters no longer builds more than the 2,000 bytes a goal may hold. Worst case is 1,853 bytes.
- **Wording.** A turn that runs past its three minutes says so, and a tool call left "approved" with no recorded result
  says it may have run.
- **Less code.** One `strictJSON`, one `validCallID`, one `newGoal`, one `pipelineText` replacer, `workerControls` used by
  both review functions, `slices.Sorted(maps.Keys(...))` for the tool list, and `itoa` (broken from 10 pipelines on) is
  gone. `goalQuestion` and `workerInstruction` keep their own decoding: they give different messages for a decode error
  and trailing data, and merging them would change the text of a tool error.

API and MCP (`internal/api`, `mcp/src/server.ts`, `internal/api/mcp_tools.json`).

- `gafferError` answers the new limit error as 409. `reet_move_pipeline_on` requires `stage` (the handler, the generated
  schema and the HTTP route all say so).
- **MCP replies stay under the 25,000-character limit.** `reet_list_factory` returns short rows under `items` (it was
  `outcomes`), pages with `limit` and `offset`, and leaves out archived outcomes unless `include_archived` is true.
  `reet_get_factory_outcome` clips each stage's handover to 600 characters and reads one pipeline with `pipeline`: clipping
  alone still left a 5-pipeline outcome at 31,800 characters. Every control (start, add, send back, move on, retry,
  approve, ...) answers with the outcome in short plus `applied`, where an approval used to be applied and the client
  then saw "the response exceeds 25,000 characters". Measured before and after with the real functions on a heavy
  stand-in: get of 3 pipelines 80,628 to 19,452, an approval of 5 pipelines 133,819 to 953, a list page of 6 outcomes
  3,769.
- `mcp_tools.json` is generated from `mcp/src/server.ts`. A worker hand-edited the JSON; the source was then edited to
  match, and `npm run export-tools` gives the committed JSON exactly (`npm test` in `mcp/` passes).
- The handler boilerplate that repeated in `gaffer_outcomes.go` is one `outcomeRevision` helper, two comments that sat
  on the wrong functions moved, a dead assignment was deleted.

Web (`web/src/lib`).

- The room offered to a new outcome comes from `floorRoom()` in the Factory and on Home instead of a literal 10.
- Approving a hand-planned outcome sends a request ID (kept per attempt, cleared on success or when the failure shows the
  request cannot have arrived), so a lost reply followed by a second click starts it once.
- The session page's decisions are `PipelineChoices`: `SessionDecision` mounts it and the 50 duplicated lines are gone.
  The bell and title now update after every answer from the session page. The wording of three messages on that page
  became PipelineChoices' ("Your instruction is with the agent of ... for its next turn.", "Gaffer is reviewing ... again.",
  "Move ... on to review?").
- The two bulk selects of Gaffer's tools no longer offer Allow. Allow stays per tool, where the warning is.
- The outcome dialog's poll no longer lets an older reply overwrite a newer one.
- Dead code in `gafferOutcome.ts` (`clip`, `outcomeLine`, `outcomeGoal`, `factoryTally`, two constants) is deleted.

### Where the fix differs from the review

- The conversation reuse excludes goals in the `limit` state: they are waiting for the person to renew them. Pause and
  complete are refused for stopped and completed goals only, for the same reason.
- The review said an idle goal's write rate would fall. Moving only `nextAt` does not remove the write, only the updated
  time's churn, so that finding is partly fixed.
- One review finding was wrong and was dropped: that the goal list is cut to 100 random rows and pipelines vanish from
  "Waiting for you". Goals belong to conversations and those are capped at 100, so it never cuts. The real cause of that
  symptom is the conversation cap fixed above.
- Bulk Allow was rated lower than the review had it. The select is disabled while it saves, so a double arrow-key press
  is unlikely, but one choice still turned on every tool with no confirmation, which is what is removed.

### Things to know

- **MCP clients that read `data.outcomes` from `reet_list_factory` must read `items`**, and the controls no longer return a
  whole outcome. `reet_move_pipeline_on` without `stage` is refused.
- Still unbounded in a read: `gaffer_decisions` (up to 60 entries with `why`) and the report text, so an outcome of ten
  pipelines of sixteen stages can still pass 25,000 characters. `pipeline` narrows the pipelines, not these.
- `gafferToolReturnsSessionText` still marks every Factory control as returning session text, though they now return none,
  so a Gaffer change after one still waits for the person. That is the cautious side, and its test expects it.
- `stop()` in the outcome dialog can still be overwritten by a slow poll already in flight.

### Checked

On the merged tree (this branch with `main` merged in).

- `go vet ./...` is clean. `go test -race -count=1` passes for `internal/store` (631 s), `internal/gaffer` (1,601 s) and
  `internal/api` (2,092 s). Before the merge, store and Gaffer also passed without the race detector (81 s and 122 s), and
  the API tests for MCP, Gaffer, Factory, outcomes, templates and tools did (29 s).
- `cd web && npm run check`: 127 test files pass and svelte-check reports 0 errors and 0 warnings. Browser specs against
  the rebuilt UI: `gaffer-outcome`, `factory-decision`, `gaffer-tools`, `gaffer-factory-session` and `factory-pieces`.
  Before the merge the web change was also run through `gaffer-floor`, `attention-contract`, `gaffer-factory`,
  `home-followups` and `home-waiting`; those were not repeated after it.
- `cd mcp && npm test`: 18 of 18 pass, including the check that `mcp_tools.json` equals what `server.ts` generates.

New tests, each guarding one behaviour.

- Store, `store_gaffer_limits_test.go`: a full account reuses the oldest finished place and keeps chats, goals that can go
  on and recent ones; the goal limit at start, held and watched; an unchanged observation; pause and complete on a
  stopped goal; what each outcome list holds; a reused request ID.
- Gaffer: a plan with too many pipelines is refused and one that fits is not; a pipeline with no room waits and launches
  nothing; a pipeline chat resent gets its saved turn; a busy provider leaves a stop undecided and a report unspent; the
  same request ID starts one outcome; the common credential forms are masked; goal reviews do not crowd out chat; local
  providers wait longer for headers; the longest part goal still saves; a turn that runs out of time says so; an approved
  call with no result may have run; pipeline details need status.
- API: the list is short rows that page and leave out archived, a control answers in short, a read clips handovers and
  reads one pipeline, move-on needs `stage` (handler, schema and route), and the goal limit is a 409.
- Web: approving a hand-written plan resends the same ID after a 502 and takes a new one after a 400, the bulk selects
  offer no Allow, and the session page's decision expectations follow the shared component.
- For the store and Gaffer tests, a test was shown to fail with its fix switched off (the outcome lists test by removing
  the followed-pipeline filter; the Gaffer worker did the same for its own). The API and web tests were checked by
  reading, not by reverting.

### Not checked

- No real agent, machine or phone: the pipelines, the retry of a lost reply and the wait for room were exercised with the
  test rigs only. The wait for room has one fenced scenario and no test of two accounts at once.
- The full `go test -race ./...` was not run on this machine (CI shards it); three packages were, above.
- Dark mode and the 390 px layout were looked at only by the e2e specs, and only `gaffer-tools` and `factory-decision`
  ran at 390 px dark.
- The timing of response headers from Ollama or LM Studio, and the cost of the five outcome scans in Go on a large
  history, were reasoned about and measured on stand-ins, not observed.
