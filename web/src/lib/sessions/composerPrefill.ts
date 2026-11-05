import { writable } from 'svelte/store'

// A request to put a message in a chat's composer. The Overview and the chat are far apart in the tree, so the
// request travels through a store rather than through every component between them. `answer` is how the chat
// says whether it took the message: a draft too full to take it, or a composer that is disabled, must not lose it.
export type ComposerPrefill = { sessionId: string; text: string; nonce: number; answer?: (applied: boolean) => void }
export const composerPrefill = writable<ComposerPrefill | null>(null)

let nonce = 0
// Nobody answers when no chat for that session is mounted; waiting for ever would leave the person with nothing.
const WAIT_MS = 400

/** Asks the chat for `sessionId` to put `text` in its composer. Resolves true only if it did. Never sends anything. */
export function offerPrefill(sessionId: string, text: string): Promise<boolean> {
  return new Promise(resolve => {
    let done = false
    const finish = (applied: boolean) => {
      if (done) return
      done = true
      clearTimeout(timer)
      // Cleared so a chat that mounts later does not act on an old request.
      composerPrefill.update(current => (current?.nonce === mine ? null : current))
      resolve(applied)
    }
    const mine = ++nonce
    const timer = setTimeout(() => finish(false), WAIT_MS)
    composerPrefill.set({ sessionId, text, nonce: mine, answer: finish })
  })
}
