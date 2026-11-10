// An outcome is an objective handed to Gaffer: Gaffer proposes the sessions, the person approves the plan, the browser
// starts each session through the ordinary launch, and each session's saved goal names the outcome it serves. That first
// line of the goal is what ties the sessions together on the factory floor.
export const MAX_OUTCOME_SESSIONS = 5
export const MAX_OBJECTIVE_BYTES = 4000
export const MAX_PLAN_PROMPT_BYTES = 8000
export const MAX_PLAN_TITLE = 80
const MAX_GOAL_BYTES = 2000
const OUTCOME_PREFIX = 'Outcome: '
const OUTCOME_LINE_BYTES = 600

export type PlanSession = { key: string; title: string; prompt: string }
export type OutcomeRow = { goal: { text: string; state: string }; attention: boolean }
export type OutcomeGroup<T> = { objective: string; rows: T[]; needsReview: number; following: number; finished: number }
export type OutcomeStep = 'waiting' | 'starting' | 'following' | 'unfollowed' | 'failed'
export type OutcomeResult = { key: string; title: string; step: OutcomeStep; note: string; sessionId?: string; agentId?: string }

const encoder = new TextEncoder()
const bytes = (text: string) => encoder.encode(text).length
const oneLine = (text: string) => text.replace(/\s+/g, ' ').trim()

/** Cuts on a character boundary so a multi-byte letter is never split. */
function clip(text: string, limit: number): string {
  if (bytes(text) <= limit) return text
  let out = ''
  for (const char of text) {
    if (bytes(out + char) > limit - 3) break
    out += char
  }
  return `${out.trimEnd()}…`
}

/** The outcome's name on a goal: the objective on one line, short enough to leave room for the rest of the goal. */
export function outcomeLine(objective: string): string {
  return clip(oneLine(objective), OUTCOME_LINE_BYTES)
}

/** The saved goal for one session of an outcome. It always fits the goal limit and always ends "Do not merge." */
export function outcomeGoal(objective: string, title: string): string {
  // The line and part are clipped first, so the whole goal is far inside the limit and its last sentence survives.
  const text = `${OUTCOME_PREFIX}${outcomeLine(objective)}\nThis session’s part: ${clip(oneLine(title), 240)}\nKeep this session moving on its part of the outcome and nothing beyond it. Make sure a pull request is raised and its description is accurate, fix failing checks and resolve straightforward conflicts. Bring scope changes, decisions and anything that belongs to another session back to me. Do not merge.`
  return clip(text, MAX_GOAL_BYTES)
}

/** The outcome a saved goal belongs to, or '' for an ordinary goal. */
export function outcomeOf(goalText: string): string {
  const first = goalText.split('\n', 1)[0] ?? ''
  return first.startsWith(OUTCOME_PREFIX) ? first.slice(OUTCOME_PREFIX.length).trim() : ''
}

/** Followed sessions grouped by the outcome their goal names, in first-seen order. Ordinary goals are left out. */
export function outcomeGroups<T extends OutcomeRow>(rows: T[]): OutcomeGroup<T>[] {
  const groups = new Map<string, OutcomeGroup<T>>()
  for (const row of rows) {
    const objective = outcomeOf(row.goal.text)
    if (!objective) continue
    let group = groups.get(objective)
    if (!group) { group = { objective, rows: [], needsReview: 0, following: 0, finished: 0 }; groups.set(objective, group) }
    group.rows.push(row)
    if (row.attention) group.needsReview++
    else if (row.goal.state === 'active') group.following++
    else if (['completed', 'stopped', 'limit'].includes(row.goal.state)) group.finished++
  }
  return [...groups.values()]
}

export function outcomeProgress(group: Pick<OutcomeGroup<unknown>, 'rows' | 'needsReview' | 'following' | 'finished'>): string {
  const total = group.rows.length
  const parts = [`${total} ${total === 1 ? 'session' : 'sessions'}`]
  if (group.needsReview) parts.push(`${group.needsReview} ${group.needsReview === 1 ? 'needs' : 'need'} review`)
  if (group.following) parts.push(`${group.following} following`)
  if (group.finished) parts.push(`${group.finished} finished`)
  const paused = total - group.needsReview - group.following - group.finished
  if (paused > 0) parts.push(`${paused} paused`)
  return parts.join(' · ')
}

/** Why the objective cannot be planned yet, or ''. */
export function objectiveBlocker(input: { objective: string; project: boolean; machineReady: boolean; agent: boolean; room: number }): string {
  if (!input.objective.trim()) return 'Write the outcome you want.'
  if (bytes(input.objective.trim()) > MAX_OBJECTIVE_BYTES) return `Keep the outcome within ${MAX_OBJECTIVE_BYTES.toLocaleString('en-GB')} bytes.`
  if (!input.project) return 'Choose the project the sessions work in.'
  if (!input.machineReady) return 'That project’s machine is not connected.'
  if (!input.agent) return 'That machine has no agent that can run a Gaffer chat.'
  if (input.room < 1) return 'Gaffer is already following ten sessions. Pause or stop one first.'
  return ''
}

/** Why the plan cannot be approved yet, or ''. */
export function planBlocker(sessions: PlanSession[], room: number): string {
  if (!sessions.length) return 'Add at least one session.'
  if (sessions.length > MAX_OUTCOME_SESSIONS) return `An outcome starts at most ${MAX_OUTCOME_SESSIONS} sessions.`
  if (sessions.length > room) return room ? `Gaffer has room to follow ${room} more ${room === 1 ? 'session' : 'sessions'}. Remove ${sessions.length - room}.` : 'Gaffer is already following ten sessions. Pause or stop one first.'
  const seen = new Set<string>()
  for (const [index, session] of sessions.entries()) {
    const title = session.title.trim(), n = index + 1
    if (!title) return `Give session ${n} a title.`
    if ([...title].length > MAX_PLAN_TITLE) return `Shorten the title of session ${n} to ${MAX_PLAN_TITLE} characters.`
    if (seen.has(title)) return `Two sessions are both called “${title}”. Give each its own title.`
    seen.add(title)
    if (!session.prompt.trim()) return `Write the prompt for “${title}”.`
    if (bytes(session.prompt.trim()) > MAX_PLAN_PROMPT_BYTES) return `The prompt for “${title}” is over ${MAX_PLAN_PROMPT_BYTES.toLocaleString('en-GB')} bytes.`
  }
  return ''
}

export function outcomeSummary(results: OutcomeResult[]): string {
  const following = results.filter(r => r.step === 'following').length
  const unfollowed = results.filter(r => r.step === 'unfollowed').length
  const failed = results.filter(r => r.step === 'failed').length
  const parts: string[] = []
  if (following) parts.push(`${following} started and followed`)
  if (unfollowed) parts.push(`${unfollowed} started but not followed yet`)
  if (failed) parts.push(`${failed} not started`)
  if (!parts.length) return 'Nothing was started.'
  return !unfollowed && !failed ? `Reet. Gaffer owns this outcome: ${parts[0]}.` : `${parts.join(', ')}.`
}
