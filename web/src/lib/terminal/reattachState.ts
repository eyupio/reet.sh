// When the session screen reconnects a lost chat by itself. Kept free of Svelte so node:test can run it.

/** Where an in-place reconnect of one chat stands in this page: none running, one in flight, or the last one failed. */
export type ReattachPhase = 'idle' | 'pending' | 'failed'

/** One automatic attempt per chat per page load, and only when nothing is in flight or has already failed: after a
 *  failure only the person retries. */
export function shouldAutoReattach(args: { canReattach: boolean; phase: ReattachPhase; tried: ReadonlySet<string>; sessionId: string }): boolean {
  return args.canReattach && args.phase === 'idle' && !args.tried.has(args.sessionId)
}
