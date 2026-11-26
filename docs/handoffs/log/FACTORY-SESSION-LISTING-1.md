## FACTORY-SESSION-LISTING-1: archived or deleted Factory sessions leave Home and Sessions (26 November 2026)

A Factory stage session stayed on Home and Sessions after its outcome was archived or deleted. The archive only wrote
`terminal_session_archives` for a session a person archived directly; an outcome archive or delete never wrote one, so
the rows only disappeared while the client's own outcomes map hid them, and came back once that read failed, was slow,
or the outcome record was pruned.

What changed, in one place.

- **A read-side filter.** `TerminalSessionsForListing` (`internal/store/store_terminal.go`) is the same list query as
  `TerminalSessionsFor` with one predicate: hide a session when it has a Gaffer goal whose text begins `Outcome: ` and
  no unarchived outcome references its id in `sessions[].sessionId` or `sessions[].stages[].sessionId`. The predicate
  sits before `LIMIT 100`, so stale rows cannot crowd out live ones. No schema change, no migration, no write-path
  change, no backfill: archived, deleted and already-stale rows are covered, and restoring an outcome lists its
  sessions again.
- **The endpoint both pages read.** `terminalSessions` (`internal/api/terminal.go`) now calls
  `TerminalSessionsForListing`. `TerminalSessionsFor` is unchanged: the broker, cleanup and push paths need the
  unfiltered list to reconcile. A direct session URL (`TerminalSessionFor`) is unchanged.
- **What is deliberately untouched.** `GafferGoalsFor` and Home's "Waiting for you" feed are unchanged, so a
  still-active goal of a deleted Factory session can still show there. Nothing is deleted; a desk that resolves a
  hidden session treats it as gone. A stage session may be hidden for the moment before its outcome record references
  it.

Checked with `go test -race ./internal/store ./internal/api`, `go test -race ./...` and `cd web && npm run check`.
The store test pins the rule against archived, deleted, live, restored, ordinary, non-Factory and stage-referenced
sessions; the API test asserts the ids on `GET /api/v2/terminal/sessions`. No browser test: no UI change.
