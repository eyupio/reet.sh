import { get, writable } from 'svelte/store'
import type { SessionInsight } from '../sessionInsights'
import { inspectSession } from '../terminal/api'
// Last observations for inspected sessions in this visit; never persist paths or PR content.
type SessionInsightEntry = { status?: SessionInsight; github?: SessionInsight }
export const sessionInsights = writable<Record<string, SessionInsightEntry>>({})
/** The session panel's Live updates choice: automatic reads, including background PR reads, wait while it is off. */
export const insightsAuto = writable(true)
export function rememberInsight(id: string, view: 'status' | 'github', value: SessionInsight) {
  sessionInsights.update(all => Object.fromEntries(Object.entries({ ...all, [id]: { ...all[id], [view]: value } }).slice(-64)))
}

// The navigation shows the pull request number for every session, not only the one whose GitHub tab was opened.
// One background read per session is shared by everything watching it and repeated on a slow clock, so a person
// never has to inspect a session first, and a list of sessions cannot start a burst of its own.
const PULL_ACTIVE_MS = 2 * 60_000
const PULL_QUIET_MS = 10 * 60_000
const PULL_CONCURRENCY = 3
const pullReadAt = new Map<string, number>()
const pullReads = new Map<string, Promise<void>>()
const pullQueue: (() => void)[] = []
let pullRunning = 0
// A long session list asks for many reads at once, one per machine; a few run together and the rest wait their turn.
function pullTurn(task: () => Promise<SessionInsight>): Promise<SessionInsight> {
  return new Promise((resolve, reject) => {
    const run = () => {
      pullRunning++
      task().then(resolve, reject).finally(() => {
        pullRunning--
        pullQueue.shift()?.()
      })
    }
    if (pullRunning < PULL_CONCURRENCY) run()
    else pullQueue.push(run)
  })
}
export function ensurePullInsight(sessionId: string, active: boolean): Promise<void> {
  if (!get(insightsAuto)) return Promise.resolve()
  const maxAge = active ? PULL_ACTIVE_MS : PULL_QUIET_MS
  if (Date.now() - (pullReadAt.get(sessionId) ?? 0) < maxAge) return Promise.resolve()
  const pending = pullReads.get(sessionId)
  if (pending) return pending
  pullReadAt.set(sessionId, Date.now())
  const read = pullTurn(() => inspectSession(sessionId, 'github', '', AbortSignal.timeout(30_000)))
    .then(answer => { if (!answer.reason) rememberInsight(sessionId, 'github', answer) })
    .catch(() => { /* A failed read is tried again on the next pass. */ })
    .finally(() => { pullReads.delete(sessionId) })
  pullReads.set(sessionId, read)
  return read
}
