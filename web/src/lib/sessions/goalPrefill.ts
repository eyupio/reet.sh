import { writable } from 'svelte/store'

// A request to open a session's follow-up editor with a goal already written. The Overview and the Gaffer panel are far apart in
// the tree, so the request travels through a store, as a composer prefill does. The panel answers at once to say whether it took
// responsibility (it is looking at that chat session); it opens the editor, which starts nothing, a moment later.
export type GoalPrefill = { sessionId: string; text: string; nonce: number; answer?: (taken: boolean) => void }
export const goalPrefill = writable<GoalPrefill | null>(null)

let nonce = 0
// Nobody answers when no panel is watching that session; waiting for ever would leave the person with nothing.
const WAIT_MS = 400

/** Asks the Gaffer panel for `sessionId` to open the follow-up editor with `text`. Resolves true only if it took the request. */
export function offerGoalPrefill(sessionId: string, text: string): Promise<boolean> {
  return new Promise(resolve => {
    let done = false
    const finish = (taken: boolean) => {
      if (done) return
      done = true
      clearTimeout(timer)
      // Cleared so a panel that mounts later does not act on an old request.
      goalPrefill.update(current => (current?.nonce === mine ? null : current))
      resolve(taken)
    }
    const mine = ++nonce
    const timer = setTimeout(() => finish(false), WAIT_MS)
    goalPrefill.set({ sessionId, text, nonce: mine, answer: finish })
  })
}
