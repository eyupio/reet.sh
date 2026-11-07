// A tiny signal so every surface that shows "needs you" refreshes at once when
// something is answered or marked seen, instead of waiting for its own poll.
// Each page keeps its own list and poll; this only tells them to reload now.
// Subscribers must never signal from inside their handler.
const handlers = new Set<() => void>()

export function onAttentionChanged(handler: () => void): () => void {
  handlers.add(handler)
  return () => { handlers.delete(handler) }
}

export function signalAttentionChanged(): void {
  for (const handler of [...handlers]) {
    try { handler() } catch { /* one failing subscriber must not stop the others */ }
  }
}
