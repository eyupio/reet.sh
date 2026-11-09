# Stage 4: managed removal, restoration and notifications

Parent: [product contract and acceptance scenarios](simple-library-machine-sync.md). Dependency: [Stage 3](simple-library-machine-sync-stage-3.md), delivered to main through #664 (`d85699b4`). Starting main: `9a93f6c48b604e6c7c5eba60e89c5e8099e734ba`.

**Status: in progress, draft controller-contract checkpoint. Stage 4 is not complete or enabled on connectors.** The owner explicitly requested this stage. Do not mark the parent ledger complete or start Stage 5 from this checkpoint. No deployment, production Tailcat fix, provisioning or real-machine acceptance is claimed.

## Implemented first checkpoint

`POST /api/v2/library/deployment-intent` is an authenticated connector request using the existing private-content account/role, personal-token/origin and enrollment-key guards. Its flat JSON request contains `targetId`, `bindingId`, `itemId`, `revisionId` and optional `generation`.

The store checks one transactional snapshot of the active enrollment, target ownership and canonical selection, exact existing revision pin, canonical status, content and deletion state. It returns `generation`, `action`, `reason`, `name`, `revisionId`, `digest`:

- `maintain` / `selected_in_use`: selected In use skill with usable canonical contents.
- `remove` / `inactive`, `deselected` or `library_deleted`: explicit central control intent, never inferred from a missing local file.
- `blocked` / `content_unavailable`: retain local bytes while a selected item has unavailable/unsupported contents or another content problem; do not turn incomplete metadata into deletion permission.

The generation hashes the account, enrollment, target, binding, pinned base, durable selection version and item version/status/deletion/content fields. Re-submit the observed generation immediately before work and when acknowledging a result. A mismatch returns 409, including status/selection ABA changes back to the same visible value. Repeat reads without control changes return the same token. Foreign enrollment/target or unpinned bindings are refused. No new schema or database-format bump is introduced by this read contract.

**This is a controller intent snapshot, not a filesystem ownership grant or completed deployment receipt.** The existing pin protects recoverable revision bytes; it does not prove the connector owns an arbitrary local path. The connector must independently prove a managed local binding, containment, local consent and stable contents before any destructive action. No connector calls this endpoint yet, so this checkpoint changes no installed files or notification behavior.

## Required continuation within Stage 4

1. Extend the durable installed-binding journal with target identity and ownership/operation receipts. Migrate existing bindings only from verified installed/applied bases, preserving registered source folders and foreign installations. Negotiate the managed-removal/receipt capability explicitly; older connectors must retain their existing additive behavior. Keep the existing single target executor and backup path.
2. On intentional status/deselection/library removal, capture unseen local changes durably before moving only a proven managed entry into recoverable backup. Ambiguous/unreadable/permission-denied items must remain local while healthy deployments proceed. Inactive status retains selections/history; reactivation resumes the latest revision.
3. Debounce disappearance across stable observations. Missing bundle files are accidental deletion, not central manifest edits. Preserve surviving local changes before restoring verified canonical bytes. Atomic editor saves must not cause restore loops or notification spam.
4. Fence preview, pre-write, post-write and acknowledgement against current intent and enrollment. A read token alone cannot make controller and filesystem atomic; a superseded result must not acknowledge success or leave a resurrected copy. Exercise delayed installs/restores/removals and status/selection ABA transitions using the real controller/connector harness.
5. Add durable deduplicated factual restore receipts and informational notification-centre events only after filesystem verification and a current-intent check. Retry/restart replay must emit one notification per actual deletion episode. Intentional removals must emit no accidental-deletion notification.
6. Prove A05–A09, A11, A13–A16, A19–A20 from the parent acceptance matrix, including offline removal/reactivation, deselect-B-only, partial missing bundles, unrelated files, paused/revoked/wrong-account enrollments, capture failures and restart/replay. Promote the remaining Stage 4 contract probes only when normal negotiated operation passes.

Do not expose Stage 5's new UX or claim managed deletion/restoration is already implemented. Do not weaken legacy job/ownership, revision/base retention, backup, consent or content checks to make this stage pass.

## Validation at this checkpoint

Store tests exercise active/inactive/deselected/deleted intent, stable repeat reads, stale status and selection generations including ABA, exact base binding, account/machine boundaries and unavailable-content preservation. API tests exercise valid connector access, wrong enrollment key, strict decoding, paused-machine refusal and a stale-generation refusal after a status change. Focused store race tests pass (6.300s), final API race checks pass (4.744s), store/API vet pass, and strict documentation/rendered checks pass for 51 pages. Current-head GitHub CI remains required. No clean latest repository-wide race sweep or filesystem acceptance is claimed.

One initial store-test run failed because the new fixture had not claimed a publishing username; the corrected fixture passes. Initial race-build attempts hit a full local Go cache or exhausted temporary build space before the API fixture could start. Old generated documentation outputs, an unused clean detached worktree, and six old regenerable Go-cache artifacts were reclaimed; committed source and the unpublished alternative query experiment were retained. Use short `TMPDIR=/workspace/t` for Unix socket tests and `GOTMPDIR=/workspace/work/go-build` in this constrained workspace.
