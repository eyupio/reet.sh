// When the session screen reconnects a lost chat by itself. Kept free of Svelte so node:test can run it.

/** Where an in-place reconnect of one chat stands in this page: none running, one in flight, or the last one failed. */
export type ReattachPhase = 'idle' | 'pending' | 'failed'

/** One automatic attempt per chat per page load, and only when nothing is in flight or has already failed: after a
 *  failure only the person retries. */
export function shouldAutoReattach(args: { canReattach: boolean; phase: ReattachPhase; tried: ReadonlySet<string>; sessionId: string }): boolean {
  return args.canReattach && args.phase === 'idle' && !args.tried.has(args.sessionId)
}

/** What the pane shows about one recovery of a chat: in flight, or why it failed. No entry is idle. */
export type RecoveryOutcome = { phase: 'pending' | 'failed'; reason?: string }

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
