# Factory delivery review: design and API contract

22 November 2026 · Stage 0 of [the delivery review plan](factory-delivery-review.md). This is the contract stages 1
(backend) and 2 (UI) build to.

## What was traced on main

- **Plan.** `gaffer.OutcomeInput` → `outcomePieces` → `store.GafferOutcome`, one JSON document per outcome in
  `gaffer_outcomes`, owned by one account. Every write goes through `UpdateGafferOutcomeFor`, a read-compare-write in
  one transaction on the outcome's `revision`.
- **Worktree and session.** Each stage is a session in the pipeline's `workDir`. The controller reads a folder's Git
  state from its machine with the bounded, read-only inspect request (`terminalproto.Inspect`).
- **Review.** Gaffer's goal review hands a stage over (`stageHandedOver`), the controller starts the next
  (`advancePipeline`).
- **Approval.** A pipeline stops at `ready_to_merge` until `ApproveMerge`; `SendPipelineBack` resets a stage and the
  later ones and tells the agent why.

Nothing on main recorded what the person agreed to accept, what commit they looked at, or their decision on it.

## Model

All of it is stored on the pipeline (`GafferOutcomeSession`), inside the outcome document. An outcome saved before this
change has none of these fields and behaves exactly as before.

| Field | Meaning |
| --- | --- |
| `criteria[]` `{id, text, revision}` | The agreed acceptance criteria. `id` is stable for the life of the criterion; `revision` is the criteria revision in which its text was last set. Up to 10, each up to 240 bytes. |
| `criteriaRevision` | 1 when the plan is approved with criteria, +1 on every edit. |
| `reviewRevision` | +1 on every change to the review: criteria, evidence, a decision, or the pipeline arriving at review. Every write names the one it saw. |
| `evidence[]` | `{id, criterionId, kind, verdict, text, url, name, state, commit, criteriaRevision, by, byName, at}`. `kind` is `note` (a reviewer's own note, labelled as such), `artefact` (a link inside the pipeline's GitHub repository) or `check` (a GitHub check, by name, read from the machine). Evidence is stored against the commit the machine reported when it was added. |
| `deliveries[]` | Recorded decisions, newest last: `{requestId, decision, commit, criteriaRevision, by, byName, at, notes, stage, statuses, failed[]}`. `decision` is `accept` or `send_back`. |
| `deliveryAccepted` | The delivery in hand was accepted. Cleared whenever the pipeline is sent back. |

## States and transitions

A pipeline with criteria gains one state, **`delivery_review`** (waiting for the person):

```
last stage before merge handed over ─┐
every stage handed over (no merge) ──┼─▶ delivery_review ──accept──▶ ready_to_merge | merge stage | done
retry at the merge stage ────────────┘          │
                                                └─send back──▶ advancing at the chosen stage (rework)
```

- **Accept delivery** records the decision and lets the pipeline carry on to what it would have done next. It never
  sets `mergeApproved`: a pipeline that waits for a merge approval still stops at `ready_to_merge`, and
  **Approve the merge** stays its own action. A pipeline set to merge by itself merges after acceptance.
- **Send back** records the decision and reuses the existing send-back: the chosen stage and every later one are done
  again in a fresh session in the same folder and branch. The agent is told the failed criteria, the criteria not
  verified and the reviewer's notes.
- Gaffer, given an outcome's decisions, never accepts a delivery, and there is no MCP tool for it.
- A pipeline without criteria never enters `delivery_review`.

## Criterion status

Computed on every read, never stored, for the commit the machine reports now:

- Evidence counts only when its `commit` is the current head and its `criteriaRevision` is not older than the
  criterion's `revision`. Anything else is returned with `historical: true` and never counts.
- **Needs work**: the reviewer's latest counting note or artefact says so, or a counting check has failed.
- **Unverified**: no counting evidence, or a counting check is still running, is missing from the pull request, or the
  pull request's head is not the reviewed commit, or GitHub could not be read.
- **Passed**: otherwise, with at least one counting passing note, artefact or check.

The reviewed commit itself has a state: `verified` (the machine reported the folder's `HEAD`), `none` (the folder is
not a Git repository; the decision is recorded with no commit), `unverified` (the machine is too old to report it) or
`unreachable`. A decision is refused in the last two.

## Server rules

- Every route is account-scoped by the signed-in browser account (`featureAccess`, then the owner-keyed outcome read):
  another account gets 404. The pipeline key, criterion IDs and evidence are resolved inside that outcome only.
  Read-only accounts, API tokens and MCP connections are refused.
- A decision names `criteriaRevision`, `reviewRevision`, `commit` and the `statuses` the reviewer was shown. The server
  reads the head from the machine again, recomputes the statuses, and applies the decision in one conditional write
  that re-checks the pipeline state and both revisions. Any difference is **409** with a `code`:
  `delivery_closed`, `delivery_changed`, `delivery_criteria_changed`, `delivery_head_changed`,
  `delivery_head_unverified`, `delivery_request_reused`.
- `requestId` (32 lowercase hex) makes a decision idempotent: the same request again returns the recorded decision,
  also after the pipeline has moved on; the same ID with different content is refused. Two reviewers deciding at once
  produce one decision and one `delivery_closed`.
- The head is read from the machine immediately before the write. A commit that lands on the machine between that read
  and the write cannot be seen; the decision records the commit that was read.

## API

All under `/api/v2/gaffer/outcomes/{id}/sessions/{key}/delivery`, JSON, strict bodies, `Cache-Control: no-store`.

| Route | Body | Answer |
| --- | --- | --- |
| `GET …/delivery` | — | `DeliveryReview`. Reads the machine (up to about 15 s). |
| `PUT …/delivery/criteria` | `{criteriaRevision, criteria: [{id?, text}]}` | `DeliveryReview`. An item with a known `id` keeps it; one without gets a new one. |
| `POST …/delivery/evidence` | `{requestId, reviewRevision, criterionId, kind, verdict?, text?, url?, name?}` | `DeliveryReview` |
| `POST …/delivery/decision` | `{requestId, decision, criteriaRevision, reviewRevision, commit, statuses, notes?, stage?, confirmed}` | `{outcome, review, decision}` |

`DeliveryReview`:

```json
{
  "outcomeId": "…", "key": "a", "title": "Export API", "state": "delivery_review",
  "open": true,
  "criteriaRevision": 1, "reviewRevision": 3,
  "commit": "9f2c…", "commitState": "verified", "commitNote": "",
  "branch": "reet/export-api", "remote": "acme/billing",
  "pull": {"number": 12, "url": "https://github.com/…", "state": "OPEN", "headCommit": "9f2c…", "atHead": true},
  "checks": [{"name": "test", "state": "SUCCESS", "tone": "good", "url": "https://…"}],
  "criteria": [{"id": "…", "text": "The export downloads as CSV.", "status": "passed",
    "evidence": [{"id": "…", "kind": "check", "name": "test", "state": "SUCCESS", "verdict": "passed",
      "commit": "9f2c…", "byName": "Paul", "at": 1795000000000, "historical": false, "unverified": false}]}],
  "decisions": [],
  "accepted": false, "acceptedCommit": "", "acceptedCurrent": false,
  "stages": ["code", "review", "test"], "defaultStage": "code",
  "next": "ready_to_merge"
}
```

`status` is `passed`, `needs_work` or `unverified`. `next` says what accepting leads to: `ready_to_merge`, `merge` or
`done`. Plan approval takes `criteria: string[]` on each pipeline of `POST /api/v2/gaffer/outcomes` and of
`POST …/sessions` (added pipelines).

## Baseline

Captured on main at `b3ad27c` with the synthetic Factory fixture of `web/tests/gaffer-factory.e2e.mjs`, for stage 4 to
compare against; nothing here changes transport. See the numbers recorded in `handoff.md` under
`FACTORY-DELIVERY-REVIEW-1`.
