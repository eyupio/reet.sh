// Reads the whole opening prompt for one session. Plain TypeScript with no Svelte so the races are testable: the answer
// of a call that was aborted, superseded or made for another session must never change what the person sees.
export type PromptOutcome =
  | { kind: 'idle' }
  | { kind: 'loading' }
  | { kind: 'ready'; text: string }
  | { kind: 'none' }
  | { kind: 'failed' }

export type PromptFetch = (id: string, signal: AbortSignal) => Promise<string>

export function createPromptLoader(fetchPrompt: PromptFetch, getId: () => string, onchange: (outcome: PromptOutcome) => void, isMissing: (error: unknown) => boolean = () => false) {
  let controller: AbortController | undefined
  let outcome: PromptOutcome = { kind: 'idle' }
  const set = (next: PromptOutcome) => { outcome = next; onchange(next) }

  async function load(): Promise<void> {
    // A second load replaces the first: the first one's answer is dropped by its own, aborted, controller.
    controller?.abort()
    const mine = new AbortController(), id = getId()
    controller = mine
    set({ kind: 'loading' })
    // An answer counts only while this call is the current one, still for the same session.
    const stale = () => mine.signal.aborted || id !== getId()
    try {
      const text = await fetchPrompt(id, mine.signal)
      if (stale()) return
      set(text === '' ? { kind: 'none' } : { kind: 'ready', text })
    } catch (error) {
      if (stale()) return
      set(isMissing(error) ? { kind: 'none' } : { kind: 'failed' })
    }
  }

  // Closing or leaving the session: nothing in flight may land, and a read cut short is not a failure.
  function abort(): void {
    controller?.abort()
    controller = undefined
    if (outcome.kind === 'loading') set({ kind: 'idle' })
  }

  function reset(): void {
    abort()
    set({ kind: 'idle' })
  }

  return { load, abort, reset, get outcome() { return outcome } }
}
