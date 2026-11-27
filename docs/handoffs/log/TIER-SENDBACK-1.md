## TIER-SENDBACK-1: a stage sent back moves up one tier (27 November 2026)

Why. The last item from the look at how agents choose effort (EFFORT-LEVELS-1): a stage sent back for its work was
done again on the same tier. The owner chose to raise it on send-back only, not on a retry after a failed start.

What changed.

- `internal/gaffer/outcome_manage.go`: `sendPipelineBack` reads the account's crew and calls `raiseTier` on the stage
  it goes back to (not the ones after it). `raiseTier` moves the stage to the next tier's crew member (agent, model,
  effort, source and backups), sets `TierFrom` and a `TierWhy` ("Moved up from standard because it was sent back."),
  and clears `First` and `Balanced`. It does nothing when the stage was moved before (`TierFrom`), is heavy, has no
  tier, when its own or the next tier has no crew agent, when its first choice is not its tier's current crew member
  (picked by hand, or chosen by an outcome's own crew, which the outcome does not keep), or when the next tier's
  member is the same worker. Both the person's send-back and the delivery review's go through it.
- `store.GafferOutcomeStage.TierFrom` is new and kept through later send-backs, so a stage moves once.
- `docs/gaffer-ai.md` says so. The page shows the new tier and its reason through the existing tier fields.

Tests. `raiseTier` on its own (moved once, kept for hand-picked, heavy, untiered, unset tiers, a stage on its backup
judged by its first choice) and a send-back end to end: the stage sent back starts on the heavier model, the one after
it does not move. Full `go test ./internal/gaffer` and the related api tests pass. No browser suite was run.

Not done. A stage whose agent came from an outcome's own crew does not move, because the outcome does not keep its
crew; keeping it would let those move too.
