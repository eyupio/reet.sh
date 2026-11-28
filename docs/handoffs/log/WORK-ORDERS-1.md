## WORK-ORDERS-1: An outside agent can hand the Factory a work order, and get a work report back (2026-11-28)

Built from `docs/superpowers/specs/2026-11-28-factory-work-orders-design.md` by the plan beside it.

**What was built**

- `internal/gaffer/workorder.go`: `ParseWorkOrder` reads the Markdown work order (`reet: work-order/1`) into the
  `OutcomeSession` values the plan review and `CreateOutcome` already take. It is strict: an unknown key or section, a
  near-miss setting line (`Stages:`), an indented heading, an unclosed code fence or a paste over 64 KiB is a problem
  with its line, never something quietly different. Session rules are not written twice: it calls `outcomePieces`.
- `internal/gaffer/workreport.go`: `WorkReport` writes an outcome as `reet: work-report/1`. Handover text is quoted so
  it cannot forge structure. A criterion's status comes from `criterionView`, and a recorded `passed` is shown only
  while the pipeline waits with no work started since.
- `POST /api/v2/gaffer/work-orders/check` and `GET /api/v2/gaffer/outcomes/{id}/work-report`.
- The built-in skill `factory-contract` (`internal/api/builtin_skills/factory-contract/`): an entry page and six topic
  files. `builtinSkill` now serves a skill's other files, and `hooks/docs.py` publishes them on the docs site.
- MCP: `reet_check_work_order`, `reet_get_work_report`, and `work_order` on `reet_start_outcome` and
  `reet_add_pipelines`. Tool counts went from 50 to 52 (18 to 20 for a read-only connection).
- Web: New Factory outcome recognises a pasted work order and opens the existing plan review; the crew suggestion is
  marked **Suggested by the work order**; **Copy prompt for an outside agent**; a work order that continues an outcome
  opens the review as **Add to an outcome**; **Copy for your agent** in an outcome's menu.

**Why this shape.** One parser and one writer sit behind both routes, and everything a work order can say already had
a place in the plan review. No store tables, no new dependency, no new setting.

**What a work order can never do.** Switch on YOLO, folder trust, a merge by itself, Gaffer's decisions or the review
limits; choose an agent over MCP; start anything without the person's approval (paste) or `confirm: true` (MCP).

**Checked**

- `go vet ./...` and `go test ./...` passed on the branch before the last fixes (50 packages); after them
  `go test ./internal/gaffer ./internal/api` passed again (117 s and 244 s).
- `cd mcp && npm test`: 18 of 18. `cd web && npm run check`: 135 test files, svelte-check 0 errors.
- `web/tests/work-order.e2e.mjs`: 12 cases at 390 px and desktop, light and dark.
- Hooks tests and `python3 scripts/check-docs.py` pass.
- A built controller serves `/eyupio/factory-contract`, its index with the six topics, and the topic files.
- Each task had a review against the spec, and the whole branch had one more; their findings are fixed in the commits.

**Not checked**

- No live end-to-end run: a real paste into a controller with accounts, a connected machine and a Gaffer provider was
  not made. The check route was exercised by its tests and the dialog by the browser suite against a mocked API.
- `npx skills use` was not run against a controller, so how the skills CLI lays out the topic files was not seen.
- The docs site was not built with `mkdocs build --strict` after the hook change; its tests pass.

**Known limits**

- A fenced paste is refused when a later line could close its wrapper again (a bare fence of the wrapper's own
  characters in a brief, or a code block in chatter after the order). The author wraps the work order in a longer
  fence. This refuses some pastes that would have been harmless, on purpose: a brief is never cut short quietly.
- A commit pushed by hand while a pipeline waits in delivery review leaves a recorded `passed` in the work report. The
  report says its statuses are as last recorded.
- Archived outcomes do not offer **Copy for your agent**.
- `TestMCPAddsAWorkOrderAndKeepsItsRefusals` failed 3 runs in 30 before it was changed to send the revision the store
  returns, because the outcome supervisor moves the revision. The cause was the test's stale revision, not the code.
