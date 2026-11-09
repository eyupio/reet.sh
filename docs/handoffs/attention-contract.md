# One attention contract for Home, the bell, the title and the sidebar

Design: [spec](../superpowers/specs/2026-11-07-attention-contract-design.md) · [plan](../superpowers/plans/2026-11-07-attention-contract.md). Web only; no API, protocol or server change.

## The rule (`web/src/lib/attention.ts`, `attentionRuleOf`)

- **Blocked:** a session with `attention === 'waiting'` counts while it holds an approval or question (`pendingPermission.permissionId` or `pendingQuestion.questionId`), seen or not. It leaves when answered, dismissed or ended.
- **Bell with nothing to answer** (a terminal bell, a chat rebuilt after a controller restart): counts only while unseen. Opening it still clears it.
- **Failed / finished:** only while unseen (`changedSinceSeen`) and updated within 24 hours (`ATTENTION_WINDOW_MS`).

## Identity, Gaffer fold and refresh

- One card per session: Home cards, the bell, the tab-title number and the workspace sidebar badge (`web/src/lib/terminal/panelState.ts` `needCount`/`needLabel`) all derive from the same rule, so they agree. A card carries ordered `reasons` (approval, question, failed/finished/review, gaffer); an empty `reasons` list is an unseen terminal bell, with nothing to resolve.
- On Home, `foldGafferGoals` folds a Gaffer goal that needs review into its session's card ("Gaffer follow-up also needs review") and leaves it out of the Gaffer button's count; the button stays visible when every goal is folded. The Gaffer reason line shows on the Home card only: the bell panel renders its own markup and ignores `reasons`.
- `web/src/lib/attentionSignal.ts`: a successful approval/question answer (Home card, bell, chat pane) or mark-seen (Home, the Sessions list, `App.svelte`, and opening an unseen session in the workspace) calls `signalAttentionChanged()`, and the bell's loader and the workspace list refresh at once. Each page keeps its own list and 15-second poll; a failed read keeps the last good count (`loadAttention` also drops a slow earlier read). `Home.svelte`'s optimistic mark-seen never hides a blocked card.
- The signal also schedules one trailing refresh about 1 s later (`SETTLE_MS`): on a real controller the pending item is cleared by a later `permission_resolved` event than the machine's OK reply, so the first refresh can still read it as blocked.
- Opening a session signals at most once per unseen stretch (`SessionScreen.svelte`, `signalledFor`), and only if the session was being counted. The session is still re-marked seen on each refresh, but a session the server keeps calling unseen (a bell that keeps ringing, a shell still printing) must not signal each time, or each refresh would trigger the next (a browser case pins the read count).

## Behaviour changes a person will see

- The sidebar badge also counts unseen failed and finished sessions, and reads "N need you" / "1 needs you".
- A blocked chat the person has opened stays counted until it is answered.
- A Gaffer goal and its worker's approval count once on Home.
- The bell and sidebar no longer lag up to 15 seconds after an answer. That is proven against a synthetic server only; the real-controller timing (the trailing refresh above) is unverified.

## Evidence

`web/tests/attention-contract.e2e.mjs` (Home, bell, title, Sessions list, another page and the sidebar agree; blocked-after-open, seen and opened bells, refresh without a poll, failed read). Attention counts were added to `session-end-gaffer.e2e.mjs` and `terminal-lifecycle.e2e.mjs` (confirmed-end dismissal, pending/uncertain/lost holds, history reopening, and a user-ended plain terminal raising no card: the machine reports `ended` with no exit code and the server gives an ended session no attention word). `terminal-organisation.e2e.mjs` follows the new sidebar rule.

## Non-goals and status

Not done here: server-owned attention items or an acknowledged state; the silent 100-session listing cap (`internal/store/store_terminal.go`); Gaffer's up-to-a-minute scheduler lag; row highlighting and `waitingProjects` stay blocked-only; the board's "Needs attention" filter stays a state label. Deployment of this change is a separate verification and is not verified by merging.

## Merge outcome

PR #651 was merged on 2026-11-08 as merge commit `81cdf32eaa4048d035deefa16f9e28afe39fd252` (head `bfb4ba38`, merge method: merge commit). Every pull-request check on that exact head passed, including both `offline` legs (the Docker leg now runs on `ubuntu-latest`, as in #652), and the Codex review had no findings. Of the push runs on `main` for the merge commit, `Gaffer offline acceptance`, `Gaffer settings browser` and `docs` passed; `image`, `release` and `terminal browser` were cancelled when the next merge (#653) landed, so they are **not verified here**. **Deployment is unverified**: controller and web rollout need their own check, and no connector upgrade is required. "No lag after answering" is proven against a synthetic server only.
