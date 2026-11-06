# Gaffer followed sessions — GM5

6 November 2026. [PR #623](https://github.com/eyupio/reet.sh/pull/623) follows the
offline-goal runtime in [#620](https://github.com/eyupio/reet.sh/pull/620). The owner
merged the GM4 plan in #617, runtime #620 and this overview #623.
Only reet.sh changes, using local fake providers and workers.

## Contract

- General and session chat expose **Chat options → Followed sessions**. The
  secondary job sheet preserves the narrative-led widget, header history button
  and bottom model/context controls. The Yorkshire voice stays occasional and
  useful; decisions and errors stay plain.
- `GET /api/v2/gaffer/goals` requires a fresh writer browser account. It lists at
  most 100 retained goals, owned through the same account's conversations. No MCP
  token or viewer access, provider generation, worker probe/enqueue, new grant or
  automatic goal activation. Public MCP tools/counts remain unchanged.
- Retained goal text, Gaffer reports and source/review timestamps remain separate
  from the overview read time. The latest report explains progress/remaining work;
  the UI does not invent task lists or independently verify PR/check claims.
  Delivery acknowledges handoff, and only the user marks completion.
- Passively reconcile previous-controller queue claims before showing a receipt.
  Uncertain/blocked/missing receipts, decisions, limits, ended workers and changed
  active-goal permissions need attention. Reading never retries an instruction.
- Decisions sort before active work, then paused/finished goals; actual review or
  creation times determine order within each group. Scheduler observation updates
  do not reorder the cards. Filters select All, Needs you, Following or Finished.
- **Open session** uses the existing workspace navigation event with an ordinary
  URL fallback. **Review goal** reuses existing fresh-pin controls/confirmations.
  Scope changes and closure discard stale reads, preserving scoped chat drafts.
- Archived/unavailable sessions retain only owned saved goal/report metadata with
  guidance to Conversation history cleanup. Their worker controls stay unavailable.
  A failed refresh labels the retained last read; it never refreshes source times.

## Validation

Fake-backed race tests cover account ownership, fresh demotion, browser-only HTTP
access, independent storage bounds, attention ordering, retained evidence after
revocation/archive, history deletion and passive restart receipt reconciliation.
They verify zero new provider calls, probes, enqueues or budget/revision changes.
The broader Gaffer/worker/goal race selection passes API (30 s), Gaffer (116 s)
and store (20 s). Final ended-worker reporting has its own targeted race check.

Web check/build and vet pass. Overview and existing goal/chat browser matrices
pass at desktop/tablet/phone widths in both themes, including escaping, nested
controls, stale refresh, empty state, drafts and focus when a row disappears.
Four synthetic captures were visually inspected and are recorded in
[UI verification](../ui-verification.md). Strict docs build, 51-page validation and
the 40-view docs browser smoke pass. Final-head CI/review results are recorded on
#623. No paid providers, deployment or merge.

## Next boundary

The MVP now has explicit bounded offline goals and an account job sheet. Further
work should be based on real user feedback and the broader orchestration handoff;
direct GitHub polling, automatic approval/merge and new worker backends remain
outside these slices. The working model retains implementation/PR authority.
