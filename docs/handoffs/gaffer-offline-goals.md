# Gaffer offline goals — GM4

6 November 2026. Follow-on to merged #612, aligned with main `1b10a978` after the owner merged the plan in #617.
Implementation is tracked in [PR #620](https://github.com/eyupio/reet.sh/pull/620).
Only reet.sh changes. Local fake providers/workers; no real credentials or merge.

The user selected 12 Gaffer reviews over 24 hours by default, with limits visible
and editable at activation. Reviews run only after new worker progress. Gaffer
communicates; the session's existing model performs implementation and PR work.

## Runtime contract

- One explicit saved goal per owned structured-session companion; several selected
  sessions may be followed independently. General chat and ordinary sends create
  no goal. Activation requires separately reviewed worker messaging and session
  history/text sharing, plus provider policy; no grant is enabled implicitly.
- Bind goal ID/revision, provider/revision/model, both grant revisions, expiry,
  budget, observed worker sequence and active review. Admission reserves the
  review and budget atomically with its existing durable Gaffer turn. At most one
  review per conversation; existing account/global concurrency limits apply.
- Controller supervision survives browser closure. It waits for new retained
  worker progress and an idle, available verified worker. No model request for
  unchanged evidence, a busy worker or a human approval. No direct GitHub polls,
  shell/PR execution, automatic interruption, YOLO change or approval answer.
- A review may relay one instruction through the existing queue, or request the
  user's attention. The current goal is the authority; worker/history/tool text
  is untrusted evidence. A model's readiness claim is a proposal for user review,
  never proof of goal completion or permission to merge. A prose-only review
  needs the user; it does not silently leave an idle goal following nothing.
  Two identical instructions are allowed per goal; a third needs review.
- Recheck goal, scope, account, provider and grant revisions before egress, during
  generation, at instruction admission, queue claim and handoff. Pausing, stopping
  or editing invalidates and removes unclaimed goal instructions, leaving manual
  messages and already claimed work alone. Resume rebinds explicitly reviewed
  permissions, preserves the budget and waits for fresh progress.
- Reconcile persisted reviews and queue receipts at restart. Resume unchanged
  active goals without replay. Interrupted/failed reviews, uncertain/blocked
  delivery and exhausted limits visibly need the user; no automatic retry.
- Restoring an instance copy pauses goals and removes their unclaimed instructions;
  a copied database cannot reauthorise old external effects. Normal controller
  restart still resumes an unchanged active goal after reconciliation.
- Reports retain observation time/sequence and actual provider usage, marked
  unknown when unavailable. Closing chat, cancelling one Gaffer answer, pausing
  supervision and stopping the worker remain distinct actions.

## UX and verification

Keep the narrative-led chat and current dock/Overview layout. Put goal setup and
controls in a secondary dialog, with a compact active/paused state entry. Show the
agreed goal, destination model, limits, latest review, receipts and what needs you.
Use fake-backed domain/store/HTTP and browser tests for consent, budget/expiry,
progress gating, multi-session fairness, revocation and queue fencing, shutdown,
actual database reopen, ambiguity and UI focus/phone/themes. Public docs and the
main handoff record the delivered scope and final PR/check state.

## Implementation record

The store owns the goal, source cursor, pins and atomic budget/review reservation.
The existing conversation service owns its bounded provider turn. The supervisor
uses a four-probe pool and oldest-due scanning while preserving the existing two-
per-account/four-global Gaffer turn limits. Queue delivery reads the saved goal
revision as well as worker grants. Browser-only goal controls cannot be supplied
through MCP credentials or ordinary chat-turn arguments. Model decisions remain
one next instruction or a user question; a prose-only review needs user attention.

Fake-backed race tests cover default-off grants, budget/expiry, changed permissions,
atomic claims across Store handles, busy/quiet/stopped workers, two sessions,
queued and in-flight pause, repeated instructions, saved user questions, browser-
free restart, interrupted review recovery and instance restore. The full local
race run passes API (404 s), Gaffer (103 s), store (174 s), broker (43 s), embedded
web and all other packages except the same five unchanged terminal-host environment
failures: three missing-tmux and two process-group cleanup tests. Later decision/
shutdown guards have targeted race checks. Runtime head `1ce63151` passes all
20 applicable CI checks, with four intentional skips and no unresolved review
threads; #620 remains open. The 16 MCP compatibility checks pass; generated tools remain unchanged.

The goal dialog and existing chat/context/worker/layout browser matrices pass on
main `b68c84e9`; web check/build, vet, strict docs, 51-page validation and the 40-view
docs browser smoke pass. Captures use only synthetic data. PR #620 records the
final immutable head, review and CI state. No paid provider, deployment or merge.
