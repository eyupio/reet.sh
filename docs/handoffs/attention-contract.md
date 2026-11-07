# One attention contract for Home, the bell, the title and the sidebar

Design: [spec](../superpowers/specs/2026-11-07-attention-contract-design.md) · [plan](../superpowers/plans/2026-11-07-attention-contract.md). Web only; no API, protocol or server change.

## The rule (`web/src/lib/attention.ts`, `attentionRuleOf`)

- **Blocked:** a session with `attention === 'waiting'` counts while it holds an approval or question (`pendingPermission.permissionId` or `pendingQuestion.questionId`), seen or not. It leaves when answered, dismissed or ended.
- **Bell with nothing to answer** (a terminal bell, a chat rebuilt after a controller restart): counts only while unseen. Opening it still clears it.
- **Failed / finished:** only while unseen (`changedSinceSeen`) and updated within 24 hours (`ATTENTION_WINDOW_MS`).

## Identity, Gaffer fold and refresh

- One card per session: Home cards, the bell, the tab-title number and the workspace sidebar badge (`panelState.ts` `needCount`/`needLabel`) all derive from the same rule, so they agree. A card carries ordered `reasons` (approval, question, failed/finished/review, gaffer).
- On Home, `foldGafferGoals` folds a Gaffer goal that needs review into its session's card ("Gaffer follow-up also needs review") and leaves it out of the Gaffer button's count; the button stays visible when every goal is folded.
- `attentionSignal.ts`: a successful approval/question answer (Home card, bell, chat pane) or mark-seen calls `signalAttentionChanged()`, and the bell's loader and the workspace list refresh at once. Each page keeps its own list and 15-second poll; a failed read keeps the last good count. `Home.svelte`'s optimistic mark-seen never hides a blocked card.

## Behaviour changes a person will see

- The sidebar badge also counts unseen failed and finished sessions, and reads "N need you" / "1 needs you".
- A blocked chat the person has opened stays counted until it is answered.
- A Gaffer goal and its worker's approval count once on Home.
- The bell and sidebar no longer lag up to 15 seconds after an answer.

## Evidence

`web/tests/attention-contract.e2e.mjs` (Home, bell, title, Sessions list, another page and the sidebar agree; blocked-after-open, seen and opened bells, refresh without a poll, failed read). Attention counts were added to `session-end-gaffer.e2e.mjs` and `terminal-lifecycle.e2e.mjs` (confirmed-end dismissal, pending/uncertain/lost holds, history reopening, and a user-ended plain terminal raising no card: the machine reports `ended` with no exit code and the server gives an ended session no attention word). `terminal-organisation.e2e.mjs` follows the new sidebar rule.

## Non-goals and status

Not done here: server-owned attention items or an acknowledged state; the silent 100-session listing cap (`internal/store/store_terminal.go`); Gaffer's up-to-a-minute scheduler lag; row highlighting and `waitingProjects` stay blocked-only; the board's "Needs attention" filter stays a state label. Deployment of this change is a separate verification and is not verified by merging.
