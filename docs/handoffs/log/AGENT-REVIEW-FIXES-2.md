## AGENT-REVIEW-FIXES-2: Gaffer's session history counts a streamed message once, and log-reading API tests no longer race (26 November 2026)

Two things AGENT-REVIEW-FIXES-1 left open.

- **The history list.** A stored `agent` event is one chunk of a message, and `ContextEventsFor`
  (`internal/store/store_context.go`) read the newest 40 rows, so with an agent that streams small chunks Gaffer's
  history held the last few words of one reply and nothing before it. It now reads back until it has 40 records, where
  consecutive agent rows with one `messageId` are one record (text joined, capped at 4,096 characters, the newest seq
  kept), and returns whether older rows were left; `contextSnapshot` uses that for `Truncated` in place of
  `count > 40`. `TestContextHistoryCountsAStreamedMessageAsOneEvent` fails without it.
- **The test race.** Most API tests never close the `Server` they start, so its setup-batch purge goroutine lives on
  and logs "database is closed" ten minutes later. Six tests point the process log at a plain `bytes.Buffer` and read
  it, which raced with that write when the package ran for longer than ten minutes under `-race` on a loaded machine
  (`TestProjectRunStartsAtOnceAndEachMemberStartsInItsOwnWorktree`, `TestSelectionRunAuditHasNoPrompt`). They now use
  one locked `captureLog` helper in `api_test.go`. Production code is unchanged.

What was checked: `go vet` and `go test` for `internal/store` and `internal/gaffer`, and the six log-capturing tests
under `-race`.

What was not: the servers the API tests leave open are still open; closing them is 57 call sites and was not needed
to end the race. The race was not reproduced after the fix, since it needs a run of more than ten minutes.
