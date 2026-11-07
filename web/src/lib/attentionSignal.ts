// A tiny signal so every surface that shows "needs you" refreshes at once when
// something is answered or marked seen, instead of waiting for its own poll.
// Each page keeps its own list and poll; this only tells them to reload now.
// Subscribers must never signal from inside their handler.
//
// A signal also schedules one more call SETTLE_MS later. On a real controller the
// answer's OK reply comes back before the server clears the pending item (that
// happens on a later event), so the immediate reload can still read a blocked
// list; the trailing one settles it.
export const SETTLE_MS = 1000

const handlers = new Set<() => void>()
let trailing: ReturnType<typeof setTimeout> | undefined

export function onAttentionChanged(handler: () => void): () => void {
  handlers.add(handler)
  return () => { handlers.delete(handler) }
}

function runHandlers(): void {
  for (const handler of [...handlers]) {
    try { handler() } catch { /* one failing subscriber must not stop the others */ }
  }
}

export function signalAttentionChanged(): void {
  runHandlers()
  // One pending trailing call at most: signals inside the window are covered by it.
  if (trailing !== undefined) return
  trailing = setTimeout(() => { trailing = undefined; runHandlers() }, SETTLE_MS)
  // A timer must not hold a Node process open (tests); browsers return a number and have no unref.
  ;(trailing as { unref?: () => void }).unref?.()
}
