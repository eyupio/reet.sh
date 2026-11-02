// What the app is waiting on, for one indicator rather than one per page.
//
// Every request through api.ts begins and ends here, and the shell draws a thin bar across the top while something is
// outstanding. The bar waits before it shows, so a request that answers at once never flickers one on, and once shown it
// stays a moment, so a burst of short requests reads as one wait rather than a stutter. Both delays are decided here,
// against an injectable clock, so a test can see them without a browser.

export type Indicator = {
  /** Marks a request as outstanding and returns the call that ends it. Ending twice is harmless. */
  begin(): () => void
  /** The requests outstanding right now. */
  pending(): number
  /** Whether the bar should be on screen right now. */
  visible(): boolean
  /** Calls fn whenever visible() changes, and returns the call that stops it. */
  subscribe(fn: (visible: boolean) => void): () => void
}

export type Clock = {
  setTimeout(fn: () => void, ms: number): unknown
  clearTimeout(handle: unknown): void
}

export const SHOW_AFTER_MS = 200
export const SHOW_FOR_AT_LEAST_MS = 400

export function createIndicator(clock: Clock = globalThis): Indicator {
  let count = 0
  let shown = false
  let showTimer: unknown = null
  let hideTimer: unknown = null
  const listeners = new Set<(visible: boolean) => void>()

  function set(next: boolean) {
    if (next === shown) return
    shown = next
    for (const fn of listeners) fn(shown)
  }
  function settle() {
    if (count > 0) return
    if (showTimer !== null) {
      clock.clearTimeout(showTimer)
      showTimer = null
    }
    // The hide timer was started when the bar appeared; it hides the bar only once the minimum has passed and nothing
    // new has begun.
    if (shown && hideTimer === null) set(false)
  }
  return {
    begin() {
      count++
      if (!shown && showTimer === null) {
        showTimer = clock.setTimeout(() => {
          showTimer = null
          if (count === 0) return
          set(true)
          hideTimer = clock.setTimeout(() => {
            hideTimer = null
            settle()
          }, SHOW_FOR_AT_LEAST_MS)
        }, SHOW_AFTER_MS)
      }
      let ended = false
      return () => {
        if (ended) return
        ended = true
        count--
        settle()
      }
    },
    pending: () => count,
    visible: () => shown,
    subscribe(fn) {
      listeners.add(fn)
      return () => listeners.delete(fn)
    },
  }
}

/** The app's one indicator. api.ts reports to it; the shell's ProgressBar watches it. */
export const inflight = createIndicator()
