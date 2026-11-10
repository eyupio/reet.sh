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

export type PlanSession = { key: string; title: string; prompt: string; stages?: string[] }
export type OutcomeRow = { goal: { text: string; state: string }; attention: boolean }
export type OutcomeGroup<T> = { objective: string; rows: T[]; needsReview: number; following: number; finished: number }
export type OutcomeStep = 'waiting' | 'starting' | 'following' | 'advancing' | 'ready_to_merge' | 'done' | 'unfollowed' | 'failed' | 'cancelled'
export type OutcomeResult = { key: string; title: string; state: OutcomeStep; note: string; sessionId?: string }
export type OutcomeRecord = { id: string; objective: string; state: string; sessions: OutcomeResult[] }

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

/** The session's part of its outcome, from the goal's second line; '' for an ordinary goal. */
export function outcomePart(goalText: string): string {
  if (!outcomeOf(goalText)) return ''
  const second = goalText.split('\n', 2)[1] ?? ''
  const at = second.indexOf(': ')
  return at >= 0 && second.startsWith('This session') ? second.slice(at + 2).trim() : ''
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

const CONTROL = /[\u0000-\u0009\u000B-\u001F\u007F]/
/** A prompt as it is sent: tabs and Windows line ends become spaces and plain new lines, which the launch accepts. */
export function cleanPrompt(prompt: string): string {
  return prompt.replace(/\r\n?/g, '\n').replace(/\t/g, '    ').trim()
}
/** Whether a limit is a whole number in range; a field can hold 1.5 without the browser objecting. */
export function wholeBetween(value: unknown, low: number, high: number): boolean {
  return typeof value === 'number' && Number.isInteger(value) && value >= low && value <= high
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
    const prompt = cleanPrompt(session.prompt)
    if (bytes(prompt) > MAX_PLAN_PROMPT_BYTES) return `The prompt for “${title}” is over ${MAX_PLAN_PROMPT_BYTES.toLocaleString('en-GB')} bytes.`
    // The launch refuses these, so say so before anything starts: an agent would read a leading dash as a flag.
    if (prompt.startsWith('-')) return `Start the prompt for “${title}” with a word, not a dash.`
    if (CONTROL.test(prompt)) return `The prompt for “${title}” has a character that cannot be sent. Retype that part.`
    if (CONTROL.test(title) || /\p{Cf}/u.test(title)) return `The title of session ${n} has a character that cannot be used. Retype it.`
    if (session.stages && !session.stages.length) return `Choose at least one stage for “${title}”.`
  }
  return ''
}

export function outcomeSummary(results: Pick<OutcomeResult, 'state'>[]): string {
  const count = (state: OutcomeStep) => results.filter(r => r.state === state).length
  const following = count('following') + count('advancing') + count('ready_to_merge') + count('done'), unfollowed = count('unfollowed'), failed = count('failed') + count('cancelled')
  const parts: string[] = []
  if (following) parts.push(`${following} started and followed`)
  if (unfollowed) parts.push(`${unfollowed} started but not followed yet`)
  if (failed) parts.push(`${failed} not started`)
  if (!parts.length) return 'Nothing was started.'
  return !unfollowed && !failed ? `Reet. Gaffer owns this outcome: ${parts[0]}.` : `${parts.join(', ')}.`
}

/** How far the controller has got with an outcome it is still starting. */
export function startingProgress(sessions: Pick<OutcomeResult, 'state'>[]): string {
  const settled = sessions.filter(s => s.state !== 'waiting' && s.state !== 'starting').length
  return `Starting session ${Math.min(sessions.length, settled + 1)} of ${sessions.length}. The controller carries on if you close this.`
}

/**
 * The floor's outcomes: the controller's records, each with the followed goals of the sessions it started, then goals
 * that only name an outcome in their text (made before the controller kept records). A goal belongs to one group.
 */
export function floorOutcomes<T extends OutcomeRow & { goal: { sessionId: string } }>(records: OutcomeRecord[], rows: T[]): (OutcomeGroup<T> & { id: string; record?: OutcomeRecord })[] {
  const claimed = new Set<T>()
  const out: (OutcomeGroup<T> & { id: string; record?: OutcomeRecord })[] = []
  for (const record of records) {
    const ids = new Set(record.sessions.map(s => s.sessionId).filter(Boolean))
    const mine = rows.filter(row => ids.has(row.goal.sessionId) && !claimed.has(row))
    mine.forEach(row => claimed.add(row))
    out.push({ id: record.id, record, objective: record.objective, rows: mine, ...tally(mine) })
  }
  for (const group of outcomeGroups(rows.filter(row => !claimed.has(row)))) out.push({ id: `text:${group.objective}`, ...group })
  return out
}

function tally(rows: OutcomeRow[]) {
  let needsReview = 0, following = 0, finished = 0
  for (const row of rows) {
    if (row.attention) needsReview++
    else if (row.goal.state === 'active') following++
    else if (['completed', 'stopped', 'limit'].includes(row.goal.state)) finished++
  }
  return { needsReview, following, finished }
}

/** One line for an outcome on the floor: what the controller is still doing, then how its followed sessions stand. */
export function floorOutcomeLine(group: OutcomeGroup<unknown> & { record?: OutcomeRecord }): string {
  const record = group.record
  if (!record) return outcomeProgress(group)
  const parts: string[] = []
  const total = record.sessions.length
  parts.push(`${total} ${total === 1 ? 'session' : 'sessions'} planned`)
  if (record.state === 'starting') parts.push('starting')
  if (group.needsReview) parts.push(`${group.needsReview} ${group.needsReview === 1 ? 'needs' : 'need'} review`)
  if (group.following) parts.push(`${group.following} following`)
  const ready = record.sessions.filter(s => s.state === 'ready_to_merge').length
  const done = record.sessions.filter(s => s.state === 'done').length
  if (ready) parts.push(`${ready} ready to merge`)
  if (done) parts.push(`${done} done`)
  else if (group.finished) parts.push(`${group.finished} finished`)
  const unfollowed = record.sessions.filter(s => s.state === 'unfollowed').length
  const failed = record.sessions.filter(s => s.state === 'failed' || s.state === 'cancelled').length
  if (unfollowed) parts.push(`${unfollowed} not followed`)
  if (failed) parts.push(`${failed} not started`)
  return parts.join(' · ')
}

// Pipelines. A planned session with stages is a pipeline: it runs through the stages below in order, each done by the
// agent and model the person chose for it. The controller moves it on when Gaffer's review hands a stage over.
export const STAGES = [
  { kind: 'plan', label: 'Plan', role: 'think' },
  { kind: 'design', label: 'Design', role: 'think' },
  { kind: 'code', label: 'Code', role: 'build' },
  { kind: 'review', label: 'Review', role: 'think' },
  { kind: 'test', label: 'Test', role: 'build' },
  { kind: 'pr', label: 'PR', role: 'ship' },
  { kind: 'babysit', label: 'Babysit', role: 'ship' },
  { kind: 'merge', label: 'Merge', role: 'ship' },
] as const
export type StageKind = typeof STAGES[number]['kind']
export type CrewRole = 'think' | 'build' | 'ship'
export type Crew = Record<CrewRole, { tool: string; model: string }>
export const CREW_ROLES: { role: CrewRole; label: string; does: string }[] = [
  { role: 'think', label: 'Thinking', does: 'Plan, design and review. Worth a stronger model.' },
  { role: 'build', label: 'Building', does: 'Code and test. Most of the tokens go here.' },
  { role: 'ship', label: 'Shipping', does: 'Pull request, babysitting and merge. A cheaper model does fine.' },
]
/** The stages a hand-written session gets, and the fallback when Gaffer's plan names none. */
export const DEFAULT_STAGES: StageKind[] = ['code', 'review', 'test', 'pr', 'babysit', 'merge']
export const stageLabel = (kind: string): string => STAGES.find(stage => stage.kind === kind)?.label ?? kind

/** The chosen stages in pipeline order, whatever order they were ticked in. Unknown kinds are dropped. */
export function orderStages(kinds: Iterable<string>): StageKind[] {
  const chosen = new Set(kinds)
  return STAGES.filter(stage => chosen.has(stage.kind)).map(stage => stage.kind)
}

/** What is sent for one pipeline: each chosen stage with the agent and model of the crew role that does it. */
export function stagePlan(kinds: Iterable<string>, crew: Crew): { kind: string; tool: string; model: string }[] {
  return orderStages(kinds).map(kind => {
    const role = STAGES.find(stage => stage.kind === kind)!.role
    return { kind, tool: crew[role].tool, model: crew[role].model.trim() }
  })
}

/** How many separate agent sessions a pipeline will use: a new one each time the agent or model changes. */
export function sessionCount(plan: { tool: string; model: string }[]): number {
  return plan.reduce((count, stage, index) => count + (index === 0 || stage.tool !== plan[index - 1].tool || stage.model !== plan[index - 1].model ? 1 : 0), 0)
}

export type TrackNode = { kind: string; label: string; state: 'done' | 'running' | 'waiting' | 'pending' | 'failed'; tool: string; model: string; summary: string }
type PipelinePiece = { state: string; stage?: number; stages?: { kind: string; tool: string; model?: string; state: string; summary?: string }[] }
/** A pipeline's stages as the page draws them. The stage in hand shows the pipeline's own state: running, waiting on the person, or failed. */
export function track(piece: PipelinePiece): TrackNode[] {
  const at = piece.stage ?? 0
  return (piece.stages ?? []).map((stage, index) => {
    let state: TrackNode['state'] = stage.state === 'done' ? 'done' : 'pending'
    if (index === at && stage.state !== 'done') {
      if (piece.state === 'ready_to_merge') state = 'waiting'
      else if (['failed', 'unfollowed', 'cancelled'].includes(piece.state)) state = 'failed'
      else if (['following', 'starting', 'advancing'].includes(piece.state)) state = 'running'
    }
    return { kind: stage.kind, label: stageLabel(stage.kind), state, tool: stage.tool, model: stage.model ?? '', summary: stage.summary ?? '' }
  })
}

/** One word for where a pipeline stands, for its badge. */
export function pipelineStatus(piece: { state: string }, attention: boolean): { label: string; tone: 'good' | 'busy' | 'you' | 'bad' | 'idle' } {
  if (piece.state === 'ready_to_merge') return { label: 'Ready to merge', tone: 'you' }
  if (piece.state === 'done') return { label: 'Done', tone: 'good' }
  if (piece.state === 'failed' || piece.state === 'cancelled') return { label: 'Not started', tone: 'bad' }
  if (piece.state === 'unfollowed') return { label: 'Stopped', tone: 'bad' }
  if (attention) return { label: 'Needs you', tone: 'you' }
  if (piece.state === 'waiting') return { label: 'Queued', tone: 'idle' }
  return { label: 'Running', tone: 'busy' }
}

/** The page's headline numbers across every outcome. */
export function factoryTally(records: { sessions: { state: string; conversationId?: string }[] }[], attention: ReadonlySet<string>): { running: number; needYou: number; ready: number; done: number } {
  const out = { running: 0, needYou: 0, ready: 0, done: 0 }
  for (const record of records) for (const piece of record.sessions) {
    if (piece.state === 'ready_to_merge') out.ready++
    else if (piece.state === 'done') out.done++
    else if (['following', 'starting', 'advancing', 'waiting'].includes(piece.state)) {
      if (piece.conversationId && attention.has(piece.conversationId)) out.needYou++
      else out.running++
    }
  }
  return out
}
