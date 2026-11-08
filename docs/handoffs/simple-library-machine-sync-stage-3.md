# Stage 3: selected-item automatic sync — in progress

Parent: [product contract](simple-library-machine-sync.md). Dependency: [Stage 2](simple-library-machine-sync-stage-2.md), PR #664, starting commit `19328605401cb667e4386de69c238410b3116d12`.

**Status: partial implementation, draft review only. Do not mark Stage 3 complete or advance to Stage 4.** No deployment or preview was created.

## Implemented experimental slice

- `DesiredSelection.selectedOnly=true` explicitly enables the prototype for an account. Existing clients retain their previous behaviour; no UI control advertises this incomplete policy.
- Only additive library selections can opt in.
- `library_selection_accounts` and `library_selections` retain explicit selected IDs independently of currently eligible desired content. Existing library targets are snapshotted on first opt-in. Other custom policies are not converted.
- Polling refreshes selected In use items to their latest available private source. It retains inactive IDs in the selection and does not subscribe to later library additions.
- New machines in an opted-in account get no implicit library selection.
- The existing durable jobs, installation checks and controller/connector transfer paths execute the resulting desired set; no second file writer was added.

## Evidence

`TestSelectedLibraryExplicitSelection` uses the real controller and separate connector processes. It verifies A/B receive the selected item, C receives nothing, a later central edit reaches A/B, and a later library addition is not implicitly selected. The focused race run passed, API package time 12.662 seconds. The existing scoped-library regression also passed in that run. The store package reported no matching tests for this particular regex; it is not a store-suite result.

An earlier selected-only contract run passed before introducing the explicit prototype gate. The ordinary tagged contract remains unchanged and is not claimed fixed for existing clients.

Scratch logs: `/workspace/work/stage3-selection.log`, `/workspace/work/stage3-selected-race.log`, `/workspace/work/stage3-focused.log`.

## Required before Stage 3 completion

1. Replace legacy skill-ID snapshots with canonical item/location selections and support registered-root content. Preserve identity through rename and source resolution.
2. Add authenticated HTTPS capability negotiation and safe per-destination cutover. Decide how to expose selection in the existing UI without requiring the prototype switch from normal users.
3. Accept intent for offline/stale-inventory destinations; remove the inherited whole-scope freshness prerequisite from intent persistence while preserving checks before filesystem writes.
4. Integrate background capture with local consent and source receipts. Propagate real A/B edits; avoid importing applied content as new edits.
5. Persist causal bases, source save time and stable edit IDs/counters. Test concurrent offline edits, older saves arriving last, clock uncertainty, deterministic resolution and retained losing revisions.
6. Fence pending work against newer intent and isolate failed items/jobs instead of inheriting machine-wide blocking and manual retry requirements.
7. Verify restart/offline catch-up, no repeated no-op jobs, all applicable adapters and retention. Run the full relevant regression/race/MCP/web checks.
8. Promote the selected-only/eligibility assertions only once normal supported operation satisfies them. Removal/restoration remains Stage 4.

## Activation and rollback

This is an explicitly gated development API prototype. Existing users are not migrated merely by opening the database. For a disposable preview account, stop polling before reverting the prototype; preserve selections and content for inspection. Remove its account opt-in marker only as an intentional rollback after reviewing saved targets. Do not reactivate a whole-library subscription as an automatic rollback of later production migrations.

The two new tables are additive. Stage 2's schema format and private-content protections remain unchanged. The current prototype still inherits legacy candidate name deduplication, legacy selection IDs, freshness restrictions and scheduler failure handling; these are remaining work, not accepted final semantics.
