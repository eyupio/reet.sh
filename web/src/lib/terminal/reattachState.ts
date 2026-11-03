// When the session screen reconnects a lost chat by itself. Kept free of Svelte so node:test can run it.

/** Where an in-place reconnect of one chat stands in this page: none running, one in flight, or the last one failed. */
export type ReattachPhase = 'idle' | 'pending' | 'failed'

/** One automatic attempt per chat per page load, and only when nothing is in flight or has already failed: after a
 *  failure only the person retries. */
export function shouldAutoReattach(args: { canReattach: boolean; phase: ReattachPhase; tried: ReadonlySet<string>; sessionId: string }): boolean {
  return args.canReattach && args.phase === 'idle' && !args.tried.has(args.sessionId)
}

/** What the pane shows about one recovery of a chat: in flight, or why it failed. No entry is idle. */
export type RecoveryOutcome = { phase: 'pending' | 'failed'; reason?: string; code?: string; detail?: string }

/** A chat has one recovery at a time: an in-place reconnect and a Continue with history (a new row) of the same
 *  conversation never run together, whichever started first. */
export function recoveryInFlight(sessionId: string, reattach: Readonly<Record<string, RecoveryOutcome>>, continuing: Readonly<Record<string, RecoveryOutcome>>): boolean {
  return reattach[sessionId]?.phase === 'pending' || continuing[sessionId]?.phase === 'pending'
}

/** Whether the chat pane will show a failed Continue with history: only a chat on screen whose pane is not offering
 *  Reconnect instead. Anywhere else the failure goes to the workspace's own error line, so it is never invisible. */
export function continuationFailureInPane(args: { structured: boolean; selected: boolean; offersReconnect: boolean }): boolean {
  return args.structured && args.selected && !args.offersReconnect
}

/** Refusals of an in-place reconnect that trying again cannot fix: the account or the machine refuses YOLO, the machine
 *  is full, the folder has plugins (Continue with history is offered instead), the person ended the chat, or the row
 *  cannot be reconnected at all. Anything else (no code, a failure, a chat still starting, another owner) may succeed. */
const FINAL_REATTACH_REFUSALS = new Set(['chat_reattach_yolo_not_allowed', 'chat_reattach_yolo_refused', 'chat_reattach_capacity', 'chat_reattach_trust', 'chat_reattach_ended', 'chat_reattach_unsupported', 'chat_reattach_no_conversation'])

/** Whether a failed reconnect offers Try again, from the server's refusal code. */
export function reattachRetryable(code?: string): boolean {
  return !code || !FINAL_REATTACH_REFUSALS.has(code)
}

/** A reconnect refused because the folder has OpenCode plugins, or because the person ended the chat (a manual attempt
 *  is told so), offers the explicit Continue with history (a new row) again, in the pane and the header menu (ruling R10). */
export function offersContinueAfterReattach(outcome: { phase: 'pending' | 'failed'; code?: string } | undefined): boolean {
  return outcome?.phase === 'failed' && (outcome.code === 'chat_reattach_trust' || outcome.code === 'chat_reattach_ended')
}

/** A refusal that is not a failure to show, for an AUTOMATIC attempt only: the person ended the chat ('ended': it is no
 *  longer offered a reconnect and shows the ended note with Continue with history), or the chat is still running
 *  ('drop': forget the attempt and read the listing again). A reconnect the person asked for always gets its answer
 *  (null shows the failure card). */
export function quietReattachRefusal(code: string | undefined, auto: boolean): 'ended' | 'drop' | null {
  if (auto && code === 'chat_reattach_ended') return 'ended'
  if (auto && code === 'chat_reattach_not_closed') return 'drop'
  return null
}
