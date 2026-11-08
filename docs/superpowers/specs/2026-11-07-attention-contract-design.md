# One attention contract for Home, the bell, the title and the sidebar

Status: design for review. No code is written. Follows the handoffs that left "broader Home/bell/session attention unification" as separate work (`docs/handoffs/home-gaffer-followups.md`, `docs/handoffs/calm-workspace-redesign.md`, `docs/handoffs/compact-work-items.md`).

## Problem

"Needs you" means different things on different surfaces, and the same underlying thing can be counted twice or hidden while still blocked. Verified in code:

1. **Blocked but hidden.** A chat waiting on an approval or question has `changedSinceSeen = bell > seen` (`internal/terminalbroker/chat.go:547`). Once the person opens it, it drops out of Home "Needs you", the bell, the tab title and the sidebar, while Continue rows and the board still say "Waiting".
2. **Double counted.** A Gaffer goal in `needs_user` (`internal/gaffer/goals.go:339`, set when the worker has a pending approval) and the worker session's own waiting card are separate counts with different identities (goal/conversation id vs session id). Home shows both numbers.
3. **Four definitions.** Home, bell and title count waiting, failed and finished within 24 hours (`web/src/lib/home.ts`, `needsYou`). The workspace sidebar counts waiting only (`web/src/lib/terminal/panelState.ts`, `needsInput`/`needCount`). The board's "Needs attention" filter counts waiting, failed, lost or non-zero exit with no seen check and no window (`web/src/lib/sessionBoard.ts:57`). The Gaffer dialog reuses the label "Needs you".
4. **Stale after answering.** Answering an approval in the chat pane triggers no attention refresh; the bell waits for its 15-second poll. Three loaders feed the bell and the sidebar (`App.loadAttention`, the Home push, the Sessions list push, plus the sidebar's own poll in `TerminalWorkspace.svelte`).

## Decisions (agreed with the user)

- **Scope:** a shared client rule first. No API or protocol change. A server-owned version can adopt the same identity later.
- **Blocked items count until resolved,** even after the person opens them.
- **Count once per session:** a Gaffer goal for a session that already has a Needs-you card is folded into it as an extra reason.
- **One rule for all counts and badges** (Home, bell, tab title, workspace sidebar). The board's "Needs attention" filter and the "Waiting for you" words on rows stay descriptive state labels, not counts.

## Non-goals

- Server-owned attention items or an "acknowledged" state; the silent 100-session listing cap (`internal/store/store_terminal.go:456`); Gaffer's up-to-a-minute scheduler lag; any new UI, layout change or mock.
- Changing what Gaffer follow-ups are, who sees them (account writers only), or the Followed sessions entry and dialog merged in #643. No implicit grants or model writes.

## Design

### 1. Identity

An attention item is keyed `(kind, sessionId, itemId)`:

| kind | itemId |
|---|---|
| `approval` | permission id |
| `question` | question id |
| `failed` / `finished` | the session's completion time |
| `gaffer` | the goal id (carried with the session id from the goal overview) |

The count stays **one card per session**, so the existing guarantee holds: bell count = Home card count = tab-title number. A card carries `reasons`, an ordered list of items, so one session with an approval and a Gaffer goal is one card with two reasons. Today's ordering stands: approval before question, then failed/finished by recency.

### 2. One derivation

Extend `needsYou()` in `web/src/lib/home.ts` so it returns cards with `reasons`, and derive every count from it: Home header and section summary, the bell, `titleWithAttention`, and the sidebar badge. `needCount` in `web/src/lib/terminal/panelState.ts` becomes a thin read of the same cards instead of a second rule. No surface keeps its own definition of "needs you". The Gaffer dialog's duplicate "Needs you" label is renamed to a distinct one.

### 3. The rule

- **Blocked:** a session with `attention === 'waiting'` counts when it has a pending approval or question (`pendingPermission.permissionId` or `pendingQuestion.questionId`), regardless of seen-ness: it leaves the count when answered, dismissed, or the session ends. A waiting session with **no pending item** (a terminal bell, or a chat rebuilt after a controller restart) has nothing to resolve, so it keeps today's rule and counts only while unseen (`changedSinceSeen`). Opening a bell still clears it.
- **Finished / failed:** unchanged: unseen (`changedSinceSeen`) and updated within 24 hours.
- Opening a card still marks it seen; for a blocked card that no longer removes it. Home's optimistic local rewrite of `attention` from waiting to working on mark-seen (`Home.svelte` near line 101) must not hide a blocked card.
- A waiting chat with no pending detail (rebuilt after a controller restart by `restoreChat`) still counts but has no Allow/Deny until the machine replays. Unchanged.

### 4. Gaffer overlap (Home only)

On Home, for each Gaffer goal whose `attention` is true and whose linked session already has a Needs-you card, add a `gaffer` reason to that card and exclude the goal from the Gaffer button's count. Goals with no blocked session keep their own count. The bell and title on other pages never included Gaffer and do not change.

### 5. One source, no staleness

Keep each page's own list and 15-second poll (the workspace sidebar's list carries the stale-poll fencing and #642 holds, so it is not replaced). Make them converge by sharing the rule (section 2) and a small refresh signal: answering an approval or question from any surface (Home card, bell, in-chat card) or marking a session seen signals every mounted loader (the bell's `loadAttention`, the workspace list) to refresh immediately. A failed read keeps the last good value, as today. The signal also schedules one trailing refresh about a second later: on a real controller the pending item is cleared by a later event than the machine's OK reply, so the immediate refresh can still read the session as blocked. A single shared store is deferred to the server-owned version.

## Behaviour changes a person will see

- The sidebar badge now also counts unseen failed and finished sessions, not only waiting ones.
- A blocked chat the person has opened stays counted until resolved.
- A Gaffer goal and its worker's approval are counted once on Home.
- The bell no longer lags after answering in the chat.

## Testing

- **Unit** (`web/src/lib/home.test.mjs`, `panelState.test.mjs`, `sessions.test.mjs`): blocked-and-seen still counts; finished/failed seen or older than 24 hours do not; approval before question; Gaffer fold gives one card with two reasons and a reduced Gaffer count; sidebar count equals bell count for the same list; a failed list read keeps the previous value.
- **Preserve** (must stay green): `home.e2e` bell = card count = title prefix and approval Allow/Deny on card and bell; `compact-work-items.e2e` 12 sessions with bell 12 and five shown; `navigation.e2e` bell and title on other pages; `home-followups.e2e` and `gaffer-followed.e2e` Gaffer counts (updated only for the fold); `terminal-organisation.e2e` sidebar badge (expectation updated for the new rule).
- **New browser assertions:** `session-end-gaffer.e2e` and `terminal-lifecycle.e2e` do not check attention today; add attention counts across #642's confirmed-end dismissal, pending/uncertain/lost holds and explicit history reopening. This also settles a suspected case: whether a user-ended non-chat terminal can raise a failed or finished card after dismissal. Verify it first and fix only if real.
- Any new or changed browser script must be listed in `.github/workflows/terminal-browser.yml` in the same change.

## Risks and open points

- **Preserved contracts:** #642 confirmed-end dismissal, pending/uncertain/lost holds, newer selection and explicit history reopening; #643 direct Followed sessions entry without implicit grants; same-process Chat/Terminal switching untouched.
- **Seen semantics:** archive removes unseen items from counts without marking them seen. Left as is; revisit with the server-owned version.
- **Latency:** the server's `ApplyChatAttention` overlay runs only in the list handler, so counts are as fresh as the last list read (15 s, or immediate after an answer).
- **Unknown:** whether viewer or read-only Home pushes `onattention` (the effect runs regardless); confirm in the plan and do not change visible behaviour for viewers.
- **Not covered by the client rule:** attention beyond the first 100 listed sessions stays uncounted until a server-owned version.
