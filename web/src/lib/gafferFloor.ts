// The factory floor: every running chat beside the goals Gaffer already follows, so several sessions can be put in
// Gaffer's care with one review. This file is the arithmetic; the dialog does the reading and the asking.
export const MAX_ACTIVE_GOALS = 10
export const MAX_GOAL_BYTES = 2000
export const DEFAULT_REVIEWS = 12
export const DEFAULT_HOURS = 24

export type FloorSession = { id: string; agentId: string; title: string; state: string; structured?: boolean; tool?: string; dir?: string }
export type FloorGoal = { sessionId: string; state: string }
export type FloorDefaults = { text: string; maxReviews: number; hours: number }
export type FloorOutcome = { id: string; title: string; ok: boolean; note: string }

const encoder = new TextEncoder()
const whole = (value: unknown, low: number, high: number, fallback: number) =>
  typeof value === 'number' && Number.isFinite(value) ? Math.min(high, Math.max(low, Math.round(value))) : fallback

/** Running structured chats with no saved goal of any kind. A session that has one, even a finished one, is managed
 *  from its own card, where starting a new budget keeps its separate confirmation. */
export function floorCandidates<T extends FloorSession>(sessions: T[], goals: FloorGoal[]): T[] {
  const followed = new Set(goals.map(goal => goal.sessionId))
  return sessions.filter(session => session.structured === true && session.state === 'running' && !followed.has(session.id))
}

/** How many more goals the account may have active at once. */
export function floorRoom(goals: FloorGoal[]): number {
  return Math.max(0, MAX_ACTIVE_GOALS - goals.filter(goal => goal.state === 'active').length)
}

/** The ticked sessions that are still on the floor, in the floor's order. */
export function floorChosen<T extends FloorSession>(candidates: T[], ticked: ReadonlySet<string>): T[] {
  return candidates.filter(session => ticked.has(session.id))
}

/** Why the combined review cannot start, or '' when it can. */
export function floorBlocker(input: { chosen: number; room: number; text: string; provider: boolean }): string {
  if (!input.provider) return 'Choose a default Gaffer provider in Gaffer settings first.'
  if (!input.chosen) return 'Tick at least one session.'
  if (input.chosen > input.room) {
    return input.room
      ? `Gaffer can follow ${MAX_ACTIVE_GOALS} sessions at once and has room for ${input.room} more. Untick ${input.chosen - input.room}.`
      : `Gaffer is already following ${MAX_ACTIVE_GOALS} sessions. Pause or stop one first.`
  }
  if (!input.text.trim()) return 'Write the goal these sessions share.'
  if (encoder.encode(input.text).length > MAX_GOAL_BYTES) return `The goal is over ${MAX_GOAL_BYTES} bytes.`
  return ''
}

export function floorSummary(outcomes: FloorOutcome[]): string {
  const started = outcomes.filter(outcome => outcome.ok).length
  const failed = outcomes.length - started
  if (!failed) return started === 1 ? 'Reet. Gaffer is looking after 1 session.' : `Reet. Gaffer is looking after ${started} sessions.`
  if (!started) return failed === 1 ? 'That session was not started. Nothing was changed for it.' : `None of the ${failed} sessions were started. Nothing was changed for them.`
  return `${started} started, ${failed} not started. The ones not started were left unchanged.`
}

export function floorDefaultsKey(userId: number): string {
  return `reet.gaffer.floorDefaults.${userId || 'anon'}`
}

export function parseFloorDefaults(raw: string | null, fallbackText: string): FloorDefaults {
  const fallback = { text: fallbackText, maxReviews: DEFAULT_REVIEWS, hours: DEFAULT_HOURS }
  if (!raw) return fallback
  let stored: unknown
  try {
    stored = JSON.parse(raw)
  } catch {
    return fallback
  }
  if (!stored || typeof stored !== 'object') return fallback
  const { text, maxReviews, hours } = stored as Partial<FloorDefaults>
  const kept = typeof text === 'string' && text.trim() && encoder.encode(text.trim()).length <= MAX_GOAL_BYTES ? text.trim() : fallbackText
  return { text: kept, maxReviews: whole(maxReviews, 1, 48, DEFAULT_REVIEWS), hours: whole(hours, 1, 168, DEFAULT_HOURS) }
}

function storage(fallback?: Storage): Storage | null {
  try {
    return fallback ?? globalThis.localStorage ?? null
  } catch {
    return null
  }
}

export function readFloorDefaults(userId: number, fallbackText: string, area?: Storage): FloorDefaults {
  const store = storage(area)
  try {
    return parseFloorDefaults(store ? store.getItem(floorDefaultsKey(userId)) : null, fallbackText)
  } catch {
    return parseFloorDefaults(null, fallbackText)
  }
}

export function writeFloorDefaults(userId: number, defaults: FloorDefaults, area?: Storage): void {
  const store = storage(area)
  if (!store) return
  try {
    store.setItem(floorDefaultsKey(userId), JSON.stringify({ version: 1, text: defaults.text.trim(), maxReviews: defaults.maxReviews, hours: defaults.hours }))
  } catch {
    /* full or blocked: the review still works for this visit */
  }
}
