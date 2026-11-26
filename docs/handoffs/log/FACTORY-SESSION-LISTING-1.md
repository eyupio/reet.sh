## FACTORY-SESSION-LISTING-1: archived or deleted Factory sessions leave Home and Sessions (26 November 2026)

A Factory stage session stayed on Home and Sessions after its outcome was archived or deleted. The archive only wrote
`terminal_session_archives` for a session a person archived directly; an outcome archive or delete never wrote one, so
the rows only disappeared while the client's own outcomes map hid them, and came back once that read failed, was slow,
or the outcome record was pruned.

What changed, in one place.

- **A read-side filter.** `TerminalSessionsForListing` (`internal/store/store_terminal.go`) is the same list query as
  `TerminalSessionsFor` with one predicate: hide a session when no unarchived outcome still names its id in
  `sessions[].sessionId` or `sessions[].stages[].sessionId` and either an archived outcome names it or it has a Gaffer
  goal whose text begins `Outcome: `. The predicate sits before `LIMIT 100`, so stale rows cannot crowd out live ones.
  No schema change, no migration, no write-path change, no backfill: archived, deleted and already-stale rows are
  covered, and restoring an outcome lists its sessions again.
- **The endpoint the app reads.** `terminalSessions` (`internal/api/terminal.go`) now calls
  `TerminalSessionsForListing`. That endpoint is read by Home, Sessions and the rest of the app (project running
  counts, the machine's live count, conversation history, activity), so a hidden session is left out of those too.
  `TerminalSessionsFor` is unchanged: the broker, cleanup and push paths need the unfiltered list to reconcile. A
  direct session URL (`TerminalSessionFor`) is unchanged.
- **Direct links still open.** The list query keeps one explicitly requested session (`?session=<id>`), and
  `TerminalWorkspace` passes the session from the address. So "Read the ended session" in an archived outcome's
  Factory detail, and a saved direct session URL, still open that history while the lists leave it out.
- **What is deliberately untouched.** `GafferGoalsFor` and Home's "Waiting for you" feed are unchanged, so a
  still-active goal of a deleted Factory session can still show there. Nothing is deleted; a desk that resolves a
  hidden session treats it as gone.
- **The known limit.** A deleted outcome leaves only its sessions' goals behind. Those goals are removed when their
  conversation is deleted, or pruned once the account passes `MaxGafferConversations`; the session is then listed
  again. A durable fix would write `terminal_session_archives` rows when an outcome is archived or deleted, and undo
  them on restore, which needs a decision.

Checked with `go test ./internal/store ./internal/api` (the new tests also pass under `-race`), plus `go test ./...`,
which passes except the pre-existing `internal/lifecycle` `TestDoctorIsReadOnlyAndRedactsCredentials` failure (it
fails at the base commit too). A full `-race ./...` run was not completed: the store suite timed out past ten minutes
at `TestSelectionRunLimits`, and the new tests were run under `-race` on their own. `go vet ./...` and `svelte-check` pass. There
is no browser test for the direct-link path. The store test pins the rule against archived, deleted, live,
restored, ordinary, non-Factory, stage-referenced and goal-pruned sessions, plus the `?session=` direct-link
exception; the API test asserts the ids on `GET /api/v2/terminal/sessions` with and without `?session=`.
