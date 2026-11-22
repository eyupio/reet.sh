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

export type PlanSession = { key: string; title: string; prompt: string; stages?: string[]; /** The acceptance criteria the person agrees for this pipeline; empty rows are dropped when sent. */ criteria?: string[]; /** The tier of each stage that is not on its default, and Gaffer's reasons. */ tiers?: Record<string, 'heavy' | 'standard' | 'light'>; whys?: Record<string, string>; /** The parts a stage is split into, by stage kind, in the order they run. */ parts?: Record<string, StagePart[]> }
/** One part of a split stage: a short name, what it does, and how demanding it is. `why` is Gaffer's reason for the tier. */
export type StagePart = { title: string; brief: string; tier: 'heavy' | 'standard' | 'light'; why?: string }
/** How many parts a stage can be split into. */
export const MAX_STAGE_PARTS = 4
export type OutcomeRow = { goal: { text: string; state: string }; attention: boolean }
export type OutcomeGroup<T> = { objective: string; rows: T[]; needsReview: number; following: number; finished: number }
export type OutcomeStep = 'waiting' | 'starting' | 'following' | 'advancing' | 'delivery_review' | 'ready_to_merge' | 'done' | 'unfollowed' | 'failed' | 'cancelled'
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

/** Acceptance criteria a pipeline is reviewed against before it merges or finishes: how many, and how long each may be. */
export const MAX_CRITERIA = 10
export const MAX_CRITERION_BYTES = 240
/** The criteria as they are sent: each on one line, trimmed, with empty rows dropped. */
export const cleanCriteria = (list: readonly string[] | undefined): string[] => (list ?? []).map(oneLine).filter(Boolean)
/** The criteria the person typed for the pipeline that a revised plan's pipeline stands in for: the one of the same
 *  title, and only when no pipeline had that title, the one in the same place. A pipeline that had none gets none, so
 *  a plan that comes back in another order never hands one pipeline another's criteria. */
export function carryCriteria(typed: readonly { title: string; criteria?: readonly string[] }[], title: string, at: number): { criteria?: string[] } {
  const own = typed.find(old => old.title === title.trim()) ?? typed[at]
  return own?.criteria ? { criteria: [...own.criteria] } : {}
}
/** Why a list of criteria cannot be sent, or ''. Empty rows are dropped, so they are never a problem. */
export function criteriaBlocker(list: readonly string[] | undefined, who = ''): string {
  const own = cleanCriteria(list), where = who ? ` for ${who}` : ''
  if (own.length > MAX_CRITERIA) return `Keep the acceptance criteria${where} to ${MAX_CRITERIA} or fewer.`
  const long = own.findIndex(text => bytes(text) > MAX_CRITERION_BYTES)
  if (long >= 0) return `Shorten criterion ${long + 1}${where} to ${MAX_CRITERION_BYTES} bytes.`
  if (own.some(text => CONTROL.test(text))) return `A criterion${where} has a character that cannot be sent. Retype that part.`
  return ''
}

export const MAX_OUTCOME_REPLY_BYTES = 2000
// Tabs and newlines are ordinary typing; other controls are not sent to a provider.
const REPLY_CONTROL = /[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/
/** Why a reply to Gaffer cannot be sent, or ''. The provider checks the same limit again. */
export function replyBlocker(reply: string): string {
  const text = reply.trim()
  if (!text) return 'Write a reply to Gaffer first.'
  if (bytes(text) > MAX_OUTCOME_REPLY_BYTES) return `Keep the reply within ${MAX_OUTCOME_REPLY_BYTES.toLocaleString('en-GB')} bytes.`
  if (REPLY_CONTROL.test(text)) return 'The reply has a character that cannot be sent. Retype that part.'
  return ''
}

/** One message in the plan review: the person's reply, or the proposal Gaffer returned. */
export type PlanTurn = { from: 'you' | 'gaffer'; text: string }
export type PlanPrevious = { summary: string; sessions: { title: string; stages: string[] }[] }
/** The compact proposal a reply answers. A broken or half-edited title is replaced, never sent as an invalid one. */
export function previousPlan(summary: string, sessions: PlanSession[]): PlanPrevious {
  return {
    summary: summary.trim(),
    sessions: sessions.map((item, index) => {
      const title = item.title.trim()
      const keeps = title && [...title].length <= MAX_PLAN_TITLE && !/[\u0000-\u001F\u007F]/.test(title) && !/\p{Cf}/u.test(title)
      return { title: keeps ? title : `Session ${index + 1}`, stages: [...(item.stages ?? [])] }
    }),
  }
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
    const criteria = criteriaBlocker(session.criteria, `“${title}”`)
    if (criteria) return criteria
  }
  return ''
}

export function outcomeSummary(results: Pick<OutcomeResult, 'state'>[]): string {
  const count = (state: OutcomeStep) => results.filter(r => r.state === state).length
  const following = count('following') + count('advancing') + count('delivery_review') + count('ready_to_merge') + count('done'), unfollowed = count('unfollowed'), failed = count('failed') + count('cancelled')
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
  const review = record.sessions.filter(s => s.state === 'delivery_review').length
  const ready = record.sessions.filter(s => s.state === 'ready_to_merge').length
  const done = record.sessions.filter(s => s.state === 'done').length
  if (review) parts.push(`${review} in delivery review`)
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
// Each stage has a tier: how demanding it is, which is what chooses its agent and model from the crew. `tier` below is
// the default for the stage's kind; Gaffer's plan, or the person, can put a stage on another.
export const STAGES = [
  { kind: 'plan', label: 'Plan', tier: 'heavy' },
  { kind: 'design', label: 'Design', tier: 'heavy' },
  { kind: 'code', label: 'Code', tier: 'standard' },
  { kind: 'review', label: 'Review', tier: 'heavy' },
  { kind: 'test', label: 'Test', tier: 'standard' },
  { kind: 'pr', label: 'PR', tier: 'light' },
  { kind: 'babysit', label: 'Babysit', tier: 'light' },
  { kind: 'merge', label: 'Merge', tier: 'light' },
] as const
export type StageKind = typeof STAGES[number]['kind']
export type CrewTier = 'heavy' | 'standard' | 'light'
export type Crew = Record<CrewTier, { tool: string; model: string; effort?: string; source?: string; /** The tier's second choice, used when the first choice's plan is running low. An empty tool is no backup. */ backup?: CrewSpare; /** Further backups, tried in order after the first. */ more?: CrewSpare[] }>
/** One backup of a tier. `source` is the billing source a Cline backup starts on. */
export type CrewSpare = { tool: string; model: string; effort?: string; source?: string }
/** How many backups a tier can have. */
export const MAX_BACKUPS = 3
/** How much of a plan must be left, in its tightest window, for a stage to stay on its first choice. */
export const BALANCE_LOW_PERCENT = 20
/** The least and most a person may choose for that share. */
export const BALANCE_MIN_PERCENT = 5, BALANCE_MAX_PERCENT = 90
/** A typed share as one a person may choose: a whole number within the bounds, the default for anything that is not a number. */
export function balancePercent(value: unknown): number {
  const n = Math.round(Number(value))
  return Number.isFinite(n) && n > 0 ? Math.min(BALANCE_MAX_PERCENT, Math.max(BALANCE_MIN_PERCENT, n)) : BALANCE_LOW_PERCENT
}
/** The crew, heaviest first: which agent, model and effort does a stage of each tier. */
export const CREW_TIERS: { tier: CrewTier; label: string; does: string }[] = [
  { tier: 'heavy', label: 'Heavy', does: 'Careful judgement: by default plan, design and review. Worth a stronger model.' },
  { tier: 'standard', label: 'Standard', does: 'Ordinary building and checking: by default code and test. Most of the tokens go here.' },
  { tier: 'light', label: 'Light', does: 'Routine work: by default pull request, babysitting and merge. A cheaper model does fine.' },
]
export const tierLabel = (tier: string): string => CREW_TIERS.find(item => item.tier === tier)?.label ?? tier
/** The crew saved in Gaffer settings as the outcome dialog shows it at the time it opens: one row per tier that names
 *  an agent, heaviest first, with the model saved beside it. Empty when no crew has been saved. */
export function savedCrewRows(crew: Partial<Record<CrewTier, { tool?: string; model?: string }>> | null | undefined): { tier: CrewTier; label: string; tool: string; model: string }[] {
  return CREW_TIERS.flatMap(item => {
    const member = crew?.[item.tier]
    const tool = (member?.tool ?? '').trim()
    return tool ? [{ tier: item.tier, label: item.label, tool, model: (member?.model ?? '').trim() }] : []
  })
}
const isTier = (tier: unknown): tier is CrewTier => tier === 'heavy' || tier === 'standard' || tier === 'light'
/** The tier a stage has until Gaffer or the person says otherwise. */
export const defaultTier = (kind: string): CrewTier => STAGES.find(stage => stage.kind === kind)?.tier ?? 'standard'
/** The tiers a pipeline's stages are on where that is not their default, and Gaffer's reasons. A stage Gaffer rated
 *  has an entry in `whys`, empty when it gave no reason, so its tier can be told from one the person chose. */
export type StageRatings = { tiers?: Record<string, CrewTier>; whys?: Record<string, string>; parts?: Record<string, StagePart[]> }
/** Gaffer's assessment as a plan carries it. Only a tier that is one is kept. */
export function planRatings(stages: { kind: string; tier?: string; tierWhy?: string; part?: string; brief?: string }[] | undefined): StageRatings {
  const tiers: Record<string, CrewTier> = {}, whys: Record<string, string> = {}, parts: Record<string, StagePart[]> = {}
  for (const stage of stages ?? []) {
    // A split stage arrives as one entry for each part; the rest are one entry with, perhaps, a rating.
    if (stage.part?.trim()) (parts[stage.kind] ??= []).push({ title: stage.part.trim(), brief: stage.brief?.trim() ?? '', tier: isTier(stage.tier) ? stage.tier : defaultTier(stage.kind), ...(stage.tierWhy?.trim() ? { why: stage.tierWhy.trim() } : {}) })
    else if (isTier(stage.tier)) { tiers[stage.kind] = stage.tier; whys[stage.kind] = stage.tierWhy?.trim() ?? '' }
  }
  return { tiers, whys, parts }
}
/** The parts of a stage, when it is split. */
export const stageParts = (ratings: StageRatings | undefined, kind: string): StagePart[] => ratings?.parts?.[kind] ?? []
/** The parts after one is changed, or removed when `next` is null. A stage left with one part is done as one again. */
export function withPart(ratings: StageRatings | undefined, kind: string, index: number, next: Partial<StagePart> | null): Record<string, StagePart[]> {
  const parts = { ...(ratings?.parts ?? {}) }
  const list = stageParts(ratings, kind).flatMap((part, at) => at !== index ? [part] : next === null ? [] : [{ ...part, ...next, ...(next.tier && next.tier !== part.tier ? { why: undefined } : {}) }])
  if (list.length > 1) parts[kind] = list; else delete parts[kind]
  return parts
}
/** Whether a kind of stage can be split: the pull request, its babysitting and the merge are each one thing. */
export const stageTakesParts = (kind: string): boolean => ['plan', 'design', 'code', 'review', 'test'].includes(kind)
/** The parts after the person splits a stage by hand, or adds a part to one already split. A stage starts as two
 *  parts on the tier the stage was on, named and briefed by the person; a split holds at most MAX_STAGE_PARTS. */
export function withNewPart(ratings: StageRatings | undefined, kind: string): Record<string, StagePart[]> {
  const parts = { ...(ratings?.parts ?? {}) }, have = stageParts(ratings, kind), tier = stageTier(kind, ratings).tier
  if (!stageTakesParts(kind) || have.length >= MAX_STAGE_PARTS) return parts
  const blank = (n: number): StagePart => ({ title: `Part ${n}`, brief: '', tier })
  parts[kind] = have.length ? [...have, blank(have.length + 1)] : [blank(1), blank(2)]
  return parts
}
/** Why a plan's parts cannot be sent yet: every part needs a name of its own and a brief. Empty when they can. */
export function partsBlocker(sessions: { title: string; stages?: string[]; parts?: Record<string, StagePart[]> }[]): string {
  for (const item of sessions) for (const kind of item.stages ?? []) {
    const parts = stageParts(item, kind)
    if (parts.length < 2) continue
    const names = parts.map(part => part.title.trim())
    if (names.some(name => !name) || parts.some(part => !part.brief.trim())) return `Give every part of the ${stageLabel(kind).toLowerCase()} stage of “${item.title.trim() || 'a pipeline'}” a name and a brief saying what it does.`
    if (new Set(names).size !== names.length) return `Two parts of the ${stageLabel(kind).toLowerCase()} stage of “${item.title.trim() || 'a pipeline'}” have the same name.`
  }
  return ''
}
/** The parts with a stage done as one again. */
export function withoutParts(ratings: StageRatings | undefined, kind: string): Record<string, StagePart[]> {
  const parts = { ...(ratings?.parts ?? {}) }
  delete parts[kind]
  return parts
}
/** One stage's tier as the dialog shows it: the tier in force, the default for its kind, whether they differ, who
 *  set it (Gaffer's assessment, the person, or nobody) and Gaffer's reason. */
export function stageTier(kind: string, ratings: StageRatings | undefined): { tier: CrewTier; standard: CrewTier; changed: boolean; by: 'gaffer' | 'you' | 'default'; why: string } {
  const standard = defaultTier(kind), tier = ratings?.tiers?.[kind] ?? standard, gaffer = ratings?.whys !== undefined && kind in ratings.whys
  return { tier, standard, changed: tier !== standard, by: gaffer ? 'gaffer' : tier !== standard ? 'you' : 'default', why: gaffer ? ratings!.whys![kind] : '' }
}
/** The ratings after the person puts a stage on a tier. Their choice replaces Gaffer's, so its reason goes; choosing
 *  the default clears the entry. */
export function withTier(ratings: StageRatings | undefined, kind: string, tier: string): Required<StageRatings> {
  const tiers = { ...(ratings?.tiers ?? {}) }, whys = { ...(ratings?.whys ?? {}) }
  delete whys[kind]
  if (isTier(tier) && tier !== defaultTier(kind)) tiers[kind] = tier; else delete tiers[kind]
  return { tiers, whys, parts: ratings?.parts ?? {} }
}
/** The stages a hand-written session gets, and the fallback when Gaffer's plan names none. */
export const DEFAULT_STAGES: StageKind[] = ['code', 'review', 'test', 'pr', 'babysit', 'merge']
export const stageLabel = (kind: string): string => STAGES.find(stage => stage.kind === kind)?.label ?? kind

/** The chosen stages in pipeline order, whatever order they were ticked in. Unknown kinds are dropped. */
export function orderStages(kinds: Iterable<string>): StageKind[] {
  const chosen = new Set(kinds)
  return STAGES.filter(stage => chosen.has(stage.kind)).map(stage => stage.kind)
}

/** A backup as it is sent with a stage. */
export type SentSpare = { tool: string; model?: string; effort?: string; provider?: string }
/** What is sent for one pipeline: each chosen stage with its tier and the agent and model of the crew member for that
 *  tier. Gaffer's reason goes with a tier that is Gaffer's. A Cline stage carries its chosen billing source when the
 *  machine takes one; empty is Cline's own choice and is left out. */
export function stagePlan(kinds: Iterable<string>, crew: Crew, sourceSupported = true, ratings?: StageRatings): { kind: string; tool: string; model: string; effort?: string; provider?: string; tier: CrewTier; tierWhy?: string; backup?: SentSpare; moreBackups?: SentSpare[]; part?: string; brief?: string }[] {
  // A split stage is sent as one entry for each part, in order, each on its own tier's agent.
  const entries = orderStages(kinds).flatMap(kind => { const parts = stageParts(ratings, kind); return parts.length > 1 ? parts.map(part => ({ kind, part })) : [{ kind, part: undefined as StagePart | undefined }] })
  return entries.map(({ kind, part }) => {
    const { tier, why } = part ? { tier: part.tier, why: part.why ?? '' } : stageTier(kind, ratings), member = crew[tier]
    const source = sourceSupported && member.tool === 'cline' ? (member.source ?? '').trim() : ''
    // A backup that is the first choice again, or one given twice, is no backup. They are sent in the order they are tried.
    const spares: SentSpare[] = []
    for (const one of [member.backup, ...(member.more ?? [])]) {
      if (!one?.tool || spares.length >= MAX_BACKUPS) continue
      const cline = sourceSupported && one.tool === 'cline' ? (one.source ?? '').trim() : ''
      const sent: SentSpare = { tool: one.tool, ...(one.model.trim() ? { model: one.model.trim() } : {}), ...(one.effort ? { effort: one.effort } : {}), ...(cline ? { provider: cline } : {}) }
      const same = (a: SentSpare) => a.tool === sent.tool && (a.model ?? '') === (sent.model ?? '') && (a.effort ?? '') === (sent.effort ?? '') && (a.provider ?? '') === (sent.provider ?? '')
      if (same({ tool: member.tool, ...(member.model.trim() ? { model: member.model.trim() } : {}), ...(member.effort ? { effort: member.effort } : {}), ...(source ? { provider: source } : {}) }) || spares.some(same)) continue
      spares.push(sent)
    }
    return { kind, tool: member.tool, model: member.model.trim(), ...(member.effort ? { effort: member.effort } : {}), ...(source ? { provider: source } : {}), tier, ...(why ? { tierWhy: why } : {}), ...(part ? { part: part.title.trim(), brief: part.brief.trim() } : {}), ...(spares.length ? { backup: spares[0] } : {}), ...(spares.length > 1 ? { moreBackups: spares.slice(1) } : {}) }
  })
}

/** A plan with every effort the machine does not report left to the agent's default, on each stage and on each of its
 *  backups alike. `efforts` is null when what the machine offers is not known (it was not read, the read failed or the
 *  machine is gone): the plan is then sent as chosen, since the controller starts a stage on the agent's default effort
 *  by itself when its effort is not offered, and stripping on a guess would only lose the person's choice. */
export function withReportedEfforts<T extends { tool: string; effort?: string; backup?: SentSpare; moreBackups?: SentSpare[] }>(plan: T[], efforts: ((tool: string) => string[]) | null): T[] {
  if (!efforts) return plan
  const keeps = (one: { tool: string; effort?: string }) => !one.effort || efforts(one.tool).includes(one.effort)
  const strip = <S extends { tool: string; effort?: string }>(one: S): S => {
    if (keeps(one)) return one
    const copy = { ...one }
    delete copy.effort
    return copy
  }
  return plan.map(stage => {
    if (keeps(stage) && (!stage.backup || keeps(stage.backup)) && (stage.moreBackups ?? []).every(keeps)) return stage
    const copy = strip(stage)
    return { ...copy, ...(stage.backup ? { backup: strip(stage.backup) } : {}), ...(stage.moreBackups ? { moreBackups: stage.moreBackups.map(strip) } : {}) }
  })
}

/** How many separate agent sessions a pipeline will use: one for every stage, whatever agent and model do it. */
export function sessionCount(plan: unknown[]): number {
  return plan.length
}

export type TrackNode = { /** Unique in the pipeline: a split stage has one node for each part. */ key: string; kind: string; /** The part's name, for one part of a split stage. */ part: string; label: string; state: 'done' | 'running' | 'waiting' | 'pending' | 'failed'; tool: string; model: string; effort: string; source?: string; summary: string; /** Why the stage ran on its backup instead of its first choice; empty when it did not. */ balanced: string; /** The effort chosen for the stage when its agent did not offer it, so it started on the agent's default; empty when nothing was dropped. */ effortNotOffered: string; moved: boolean; /** Gaffer closed the stage itself, on an outcome whose decisions it was given. */ auto: boolean; choice: StageChoice | null; /** The tier's name, empty for a stage with none, and Gaffer's reason where the tier was its assessment. */ tier: string; tierWhy: string }
type ChosenStage = { tool: string; model?: string; effort?: string; effortNotOffered?: string; tier?: string; tierWhy?: string; balanced?: string; appliedModel?: string; appliedEffort?: string; choiceCheck?: string; choiceNote?: string }
type PipelinePiece = { state: string; stage?: number; stages?: (ChosenStage & { kind: string; part?: string; provider?: string; state: string; summary?: string; closedBy?: string })[] }

/** Whether a stage's agent, model and effort were applied, as the controller read it back from the machine. */
export type StageChoice = { tone: 'good' | 'bad' | 'idle'; label: string; text: string }
/** What the page says about a stage's choice. The stage's own agent, model and effort are what was asked for; this is
 *  what its session was found to be on. Nothing is said for a stage that has not been checked: a stage yet to start,
 *  or one that ran before the controller made the check. */
export function stageChoice(stage: ChosenStage | undefined): StageChoice | null {
  if (!stage?.choiceCheck) return null
  if (stage.choiceCheck === 'mismatch') return { tone: 'bad', label: 'Not as chosen', text: stage.choiceNote || 'The session is not on what was chosen for this stage.' }
  if (stage.choiceCheck === 'unchecked') return { tone: 'idle', label: 'Not checked', text: stage.choiceNote || 'reet could not read back what this session is running.' }
  const on = [stage.appliedModel || 'model not reported', stage.appliedEffort ? `${stage.appliedEffort} effort` : ''].filter(Boolean).join(' · ')
  // The effort chosen was not offered by the agent, so the stage started on the agent's default: that is as asked, and says why.
  if (stage.effortNotOffered && !stage.effort) {
    const why = stage.choiceNote || `You chose the effort ${stage.effortNotOffered}; the agent does not offer it for this model here, so the session was started on the agent's default effort.`
    const reported = [stage.appliedModel, stage.appliedEffort ? `${stage.appliedEffort} effort` : ''].filter(Boolean).join(' · ')
    return { tone: 'good', label: 'Confirmed', text: [why, reported && `The session reports ${reported}.`, !stage.model && 'The model was left to the agent.'].filter(Boolean).join(' ') }
  }
  // A model or effort left to the agent has nothing to be compared with, so what it turned out to be is the news.
  const left = [!stage.model && 'model', !stage.effort && stage.appliedEffort && 'effort'].filter(Boolean).join(' and ')
  return { tone: 'good', label: 'Confirmed', text: left ? `The session is on ${on}. The ${left} ${left.includes(' and ') ? 'were' : 'was'} left to the agent.` : `The session is on ${on}, as chosen.` }
}
/** A pipeline's stages as the page draws them. The stage in hand shows the pipeline's own state: running, waiting on the person, or failed. */
export function track(piece: PipelinePiece): TrackNode[] {
  const at = piece.stage ?? 0
  return (piece.stages ?? []).map((stage, index) => {
    let state: TrackNode['state'] = stage.state === 'done' ? 'done' : 'pending'
    if (index === at && stage.state !== 'done') {
      if (piece.state === 'ready_to_merge' || piece.state === 'delivery_review') state = 'waiting'
      else if (['failed', 'unfollowed', 'cancelled'].includes(piece.state)) state = 'failed'
      else if (['following', 'starting', 'advancing'].includes(piece.state)) state = 'running'
    }
    // A stage the person moved on was closed on their word, not on Gaffer's review of it.
    return { key: `${stage.kind}:${index}`, kind: stage.kind, part: stage.part ?? '', label: stage.part ? `${stageLabel(stage.kind)}: ${stage.part}` : stageLabel(stage.kind), state, tool: stage.tool, model: stage.model ?? '', effort: stage.effort ?? '', choice: stageChoice(stage), tier: stage.tier ? tierLabel(stage.tier) : '', tierWhy: stage.tierWhy ?? '', source: stage.provider, summary: stage.summary ?? '', balanced: stage.balanced ?? '', effortNotOffered: stage.effortNotOffered ?? '', moved: stage.state === 'done' && (stage.closedBy === 'person' || stage.closedBy === 'chat'), auto: stage.state === 'done' && stage.closedBy === 'auto' }
  })
}

/** One word for where a pipeline stands, for its badge. */
export function pipelineStatus(piece: { state: string; sessionState?: string }, attention: boolean): { label: string; tone: 'good' | 'busy' | 'you' | 'bad' | 'idle' } {
  if (sessionLost(piece)) return { label: 'Needs you', tone: 'you' }
  if (piece.state === 'delivery_review') return { label: 'Delivery review', tone: 'you' }
  if (piece.state === 'ready_to_merge') return { label: 'Ready to merge', tone: 'you' }
  if (piece.state === 'done') return { label: 'Done', tone: 'good' }
  if (piece.state === 'failed' || piece.state === 'cancelled') return { label: 'Not started', tone: 'bad' }
  if (piece.state === 'unfollowed') return { label: 'Stopped', tone: 'bad' }
  if (attention) return { label: 'Needs you', tone: 'you' }
  if (piece.state === 'waiting') return { label: 'Queued', tone: 'idle' }
  return { label: 'Running', tone: 'busy' }
}

/** A pipeline that is still followed but whose session was ended or removed: nothing can move it but the person. */
export const sessionLost = (piece: { state: string; sessionState?: string }) => piece.state === 'following' && (piece.sessionState === 'ended' || piece.sessionState === 'gone')
export const SESSION_ENDED_WHY = 'This stage’s session was ended, so nothing is working on it. What it committed is still in the pipeline’s folder and branch.'

/** An outcome is finished when it is not starting sessions and no pipeline is moving. Only a finished one can be put away. */
export function outcomeFinished(record: { state: string; sessions: { state: string }[] }): boolean {
  return record.state !== 'starting' && !record.sessions.some(piece => ['following', 'starting', 'advancing', 'waiting', 'delivery_review', 'ready_to_merge'].includes(piece.state))
}

/** The page's headline numbers across every outcome. */
export function factoryTally(records: { sessions: { state: string; conversationId?: string; sessionState?: string }[] }[], attention: ReadonlySet<string>): { running: number; needYou: number; ready: number; done: number } {
  const out = { running: 0, needYou: 0, ready: 0, done: 0 }
  for (const record of records) for (const piece of record.sessions) {
    if (piece.state === 'delivery_review') out.needYou++
    else if (piece.state === 'ready_to_merge') out.ready++
    else if (piece.state === 'done') out.done++
    else if (['following', 'starting', 'advancing', 'waiting'].includes(piece.state)) {
      if (sessionLost(piece) || piece.conversationId && attention.has(piece.conversationId)) out.needYou++
      else out.running++
    }
  }
  return out
}

// Usage. reet knows what Gaffer's own model used: each finished stage keeps its count, and the stage in hand reads its
// live goal. It does not know what the working agents used, and it attaches no price to any of it.
export type Usage = { tokens: number; reviews: number; reported: boolean }
type UsageStage = { reviews?: number; inputTokens?: number; outputTokens?: number; usageReported?: boolean; state?: string }
type UsageGoal = { usedReviews: number; inputTokens: number; outputTokens: number; usageReported: boolean }
/** What Gaffer has used on one pipeline: its finished stages plus the goal in hand. `reported` is false when any part gave no token count. */
export function pipelineUsage(piece: { stages?: UsageStage[] }, goal?: UsageGoal): Usage {
  const out: Usage = { tokens: 0, reviews: 0, reported: true }
  let counted = false
  for (const stage of piece.stages ?? []) {
    if (stage.state !== 'done' || !stage.reviews) continue
    counted = true
    out.reviews += stage.reviews
    out.tokens += (stage.inputTokens ?? 0) + (stage.outputTokens ?? 0)
    if (!stage.usageReported) out.reported = false
  }
  if (goal && goal.usedReviews > 0) {
    counted = true
    out.reviews += goal.usedReviews
    out.tokens += goal.inputTokens + goal.outputTokens
    if (!goal.usageReported) out.reported = false
  }
  if (!counted) out.reported = false
  return out
}

/** The same across an outcome, with the report Gaffer was asked for, if any. */
export function outcomeUsage(pieces: Usage[], report?: { inputTokens: number; outputTokens: number; usageReported: boolean }): Usage {
  const out: Usage = { tokens: 0, reviews: 0, reported: pieces.some(piece => piece.reviews > 0) || !!report }
  for (const piece of pieces) {
    out.tokens += piece.tokens; out.reviews += piece.reviews
    if (piece.reviews > 0 && !piece.reported) out.reported = false
  }
  if (report) { out.tokens += report.inputTokens + report.outputTokens; if (!report.usageReported) out.reported = false }
  return out
}

/** Tokens as the page shows them: a count when the provider reported one for everything counted, otherwise said plainly. */
export function tokensText(usage: Usage): string {
  if (!usage.reviews && !usage.tokens) return 'None yet'
  return usage.reported ? usage.tokens.toLocaleString('en-GB') : usage.tokens ? `${usage.tokens.toLocaleString('en-GB')}+ (not all reported)` : 'Not reported'
}

/** How long a goal still follows for, in words. */
/** How long ago something happened, in the page's short form. Empty when the time is not known. */
export function ago(at: number | undefined, now: number): string {
  if (!at) return ''
  const minutes = Math.floor((now - at) / 60000)
  if (minutes < 1) return 'under a minute'
  if (minutes < 60) return `${minutes} min`
  const hours = Math.floor(minutes / 60)
  return hours < 48 ? `${hours} h ${minutes % 60} min` : `${Math.floor(hours / 24)} days`
}

/** What the stage in hand's agent last reported doing, and for how long it has been quiet. `quiet` marks an agent that
 *  says it is working but has shown nothing for a quarter of an hour, or that is idle. */
export function agentActivity(stage: { agentState?: string; agentActiveAt?: number } | undefined, now: number): { text: string; quiet: boolean } {
  if (!stage?.agentState && !stage?.agentActiveAt) return { text: '', quiet: false }
  const since = ago(stage.agentActiveAt, now), minutes = stage.agentActiveAt ? Math.floor((now - stage.agentActiveAt) / 60000) : 0
  const names: Record<string, string> = { working: 'Working', waiting: 'Waiting for you', idle: 'Idle', finished: 'Finished', failed: 'Stopped', cancelled: 'Cancelled' }
  const name = names[stage.agentState ?? ''] ?? 'Not reported'
  return { text: since ? `${name} · last active ${since} ago` : name, quiet: stage.agentState === 'idle' || (stage.agentState === 'working' && minutes >= 15) }
}

export function timeLeft(expiresAt: number, now: number): string {
  const minutes = Math.floor((expiresAt - now) / 60000)
  if (minutes <= 0) return 'Ended'
  if (minutes < 60) return `${minutes} min`
  const hours = Math.floor(minutes / 60)
  return hours < 48 ? `${hours} h ${minutes % 60} min` : `${Math.floor(hours / 24)} days`
}

// Which sessions belong to a Factory pipeline, so Home and the session lists can say so. Every stage's session is
// included: a finished stage's session is still part of the pipeline it worked on.
export type FactoryMember = { outcomeId: string; objective: string; pipeline: string; stage: string; step: number; steps: number; current: boolean; /** The pipeline is still being worked on in this session: ending it stops the pipeline. */ working: boolean; key: string }
type MemberStage = { kind: string; sessionId?: string }
type MemberPiece = { key?: string; state?: string; title: string; sessionId?: string; stage?: number; stages?: MemberStage[] }
export function factoryMembers(records: { id: string; objective: string; sessions: MemberPiece[] }[]): Map<string, FactoryMember> {
  const out = new Map<string, FactoryMember>()
  const label = (kind: string) => STAGES.find(item => item.kind === kind)?.label ?? kind
  for (const record of records) for (const piece of record.sessions) {
    const stages = piece.stages ?? []
    const moving = ['following', 'starting', 'advancing', 'delivery_review', 'ready_to_merge'].includes(piece.state ?? 'following')
    const base = { outcomeId: record.id, objective: record.objective, pipeline: piece.title, steps: stages.length, key: piece.key ?? '' }
    // Later stages that keep one session overwrite earlier ones, so a session shows the latest stage it is doing.
    stages.forEach((stage, index) => { if (stage.sessionId && index <= (piece.stage ?? 0)) out.set(stage.sessionId, { ...base, stage: label(stage.kind), step: index + 1, current: index === (piece.stage ?? 0), working: moving && stage.sessionId === piece.sessionId }) })
    if (piece.sessionId && !out.has(piece.sessionId)) out.set(piece.sessionId, { ...base, stage: '', step: 0, current: true, working: moving })
  }
  return out
}
/** The sessions of stages the Factory has finished with: an earlier stage's, or any stage of a pipeline that is no longer
 *  being worked on. Their pipeline's own item says what became of them, so Home and the session lists leave them out. */
export function finishedFactorySessions(members: ReadonlyMap<string, FactoryMember>): Set<string> {
  const out = new Set<string>()
  for (const [id, member] of members) if (!member.working) out.add(id)
  return out
}
/** The sessions with the Factory-finished ones taken out. With nothing known of the Factory, every session stays. */
export function offFinishedFactory<T extends { id: string }>(sessions: readonly T[], members: ReadonlyMap<string, FactoryMember>): T[] {
  const finished = finishedFactorySessions(members)
  return finished.size ? sessions.filter(session => !finished.has(session.id)) : [...sessions]
}
/** What stopping a session does to a Factory outcome whose pipeline still works in it. */
export type FactorySessionEndMode = 'leave' | 'archive' | 'delete'
/** The system default: stopping a pipeline's session stops following the outcome and deletes it. */
export const FACTORY_SESSION_END_DEFAULT: FactorySessionEndMode = 'delete'
/** A saved preference, or the system default when it is not one of the known choices. */
export function factorySessionEndModeOf(value: unknown): FactorySessionEndMode {
  return value === 'leave' || value === 'archive' || value === 'delete' ? value : FACTORY_SESSION_END_DEFAULT
}
/** The end request's `factoryOutcome`: '' when the outcome is left alone. */
export function factorySessionEndRequest(mode: FactorySessionEndMode): string {
  return mode === 'leave' ? '' : mode
}
/** What ending, or removing, a session costs a pipeline that is still working in it. '' when it costs nothing. */
export function factoryEndWarning(member: FactoryMember | undefined, removing = false): string {
  if (!member?.working) return ''
  const what = member.stage ? `the ${member.stage.toLowerCase()} stage of the Factory pipeline “${member.pipeline}”` : `the Factory pipeline “${member.pipeline}”`
  return removing
    ? `This session is doing ${what}. Removing it leaves that pipeline with nothing working on it. The session stays in history while the pipeline still works in its worktree.`
    : `This session is doing ${what}. Ending it leaves that pipeline with nothing working on it, so choose below what happens to the outcome.`
}
/** "Factory · Plan 1 of 8", or "Factory" for a session with no stages. */
export function factoryBadge(member: FactoryMember): string {
  return member.step ? `Factory · ${member.stage} ${member.step} of ${member.steps}` : 'Factory'
}

// What the working agents report using. An agent says how many tokens a turn took and how full its context window is;
// reet adds the turns up per stage and attaches no price. An agent that reports nothing is said to, never shown as zero.
type AgentStage = { tool: string; state?: string; agentInputTokens?: number; agentOutputTokens?: number; agentCachedTokens?: number; agentUsageReported?: boolean; contextUsed?: number; contextSize?: number }
export type AgentUsage = { tokens: number; cached: number; reported: boolean }
const stageTokens = (stage: AgentStage) => (stage.agentInputTokens ?? 0) + (stage.agentOutputTokens ?? 0)
/** Input and output tokens the agents of one pipeline reported, with cached tokens counted apart. */
export function pipelineAgentUsage(piece: { stages?: AgentStage[] }): AgentUsage {
  const out: AgentUsage = { tokens: 0, cached: 0, reported: false }
  for (const stage of piece.stages ?? []) {
    if (!stage.agentUsageReported) continue
    out.reported = true; out.tokens += stageTokens(stage); out.cached += stage.agentCachedTokens ?? 0
  }
  return out
}
/** The same across an outcome, split by agent, largest first. */
export function outcomeAgentUsage(pieces: { stages?: AgentStage[] }[]): { total: AgentUsage; byAgent: { tool: string; tokens: number }[] } {
  const total: AgentUsage = { tokens: 0, cached: 0, reported: false }, by = new Map<string, number>()
  for (const piece of pieces) for (const stage of piece.stages ?? []) {
    if (!stage.agentUsageReported) continue
    total.reported = true; total.tokens += stageTokens(stage); total.cached += stage.agentCachedTokens ?? 0
    by.set(stage.tool, (by.get(stage.tool) ?? 0) + stageTokens(stage))
  }
  return { total, byAgent: [...by].map(([tool, tokens]) => ({ tool, tokens })).sort((a, b) => b.tokens - a.tokens || a.tool.localeCompare(b.tool)) }
}
/** A token count in a few characters: 1,234, 12k, 1.2M. */
export const compactTokens = (n: number): string => n >= 999_500 ? `${(n / 1_000_000).toFixed(n >= 10_000_000 ? 0 : 1)}M` : n >= 10_000 ? `${Math.round(n / 1000)}k` : n.toLocaleString('en-GB')
const compact = compactTokens
export function agentTokensText(usage: AgentUsage): string {
  if (!usage.reported) return 'Not reported'
  return usage.cached ? `${compact(usage.tokens)} · ${compact(usage.cached)} cached` : compact(usage.tokens)
}
/** How much of the agent's context window is left, for the stage in hand. '' when the agent has not said. */
export function contextLeft(stage: AgentStage | undefined): { text: string; low: boolean } {
  const size = stage?.contextSize ?? 0
  if (!stage || size <= 0) return { text: '', low: false }
  const left = Math.max(0, size - Math.min(size, stage.contextUsed ?? 0)), percent = Math.round(left / size * 100)
  return { text: `${percent}% · ${compact(left)} of ${compact(size)}`, low: percent <= 15 }
}

/** An objective can run to paragraphs; a title, a notice or a list row names it in a line. */
export const short = (text: string): string => { const line = text.replace(/\s+/g, ' ').trim(); return line.length > 80 ? `${line.slice(0, 79)}…` : line }

/** The id of a pipeline's card on the Factory page, so Home and other pages can link straight to it. */
export const pipelineAnchor = (outcomeId: string, key: string): string => `pipeline-${outcomeId}-${key}`

/** The id of a pipeline's delivery review card on the Factory page. */
export const deliveryAnchor = (outcomeId: string, key: string): string => `delivery-${outcomeId}-${key}`
/** The Factory page opened on one pipeline with its delivery review in view and focused. */
export const deliveryHref = (outcomeId: string, key: string): string => `/factory?pipeline=${encodeURIComponent(`${outcomeId}.${key}`)}&review=1`

export type NeedChoice = { id: 'review' | 'answer' | 'retry' | 'again' | 'tell' | 'move' | 'budget' | 'open' | 'backup' | 'replace-worker' | 'restart' | 'stop' | 'merge'; label: string; primary?: boolean; /** The choice to take unless the person knows better: Gaffer's, when its review advised, else the usual one. */ recommended?: boolean }
/** Who recommends the first choice and why. `text` is the instruction Gaffer would give the agent. */
export type NeedRecommend = { by: 'gaffer' | 'default'; label: string; why: string; text?: string }
export type Need = { why: string; choices: NeedChoice[]; recommend?: NeedRecommend }
/** Gaffer's advice on a stage its review brought back: one of the person's choices, and why. */
export type GoalAdvice = { choice: string; why: string; text?: string }
/** How many decisions Gaffer may take on one stage of an outcome whose decisions it was given. */
export const MAX_STAGE_DECISIONS = 3
/** Puts the recommended choice first and marks it. With one choice there is nothing to recommend. */
function recommend(why: string, choices: NeedChoice[], advice?: GoalAdvice): Need {
  if (choices.length < 2) return { why, choices }
  const advised = advice?.why ? choices.find(choice => choice.id === advice.choice) : undefined
  const first = advised ?? choices[0]
  const ordered = [first, ...choices.filter(choice => choice !== first)].map((choice, index) => ({ id: choice.id, label: choice.label, ...(index === 0 ? { primary: true, recommended: true } : {}) }))
  return { why, choices: ordered, recommend: advised ? { by: 'gaffer', label: first.label, why: advice!.why, ...(advice!.text ? { text: advice!.text } : {}) } : { by: 'default', label: first.label, why: '' } }
}
/** What the person can do about a pipeline whose session was ended or removed under it. `gone` is a session that was
 *  removed from history, which cannot be opened. */
export function sessionEndedChoices(last: boolean, gone: boolean): Need {
  const choices: NeedChoice[] = [{ id: 'restart', label: 'Run this stage again in a new session', primary: true }, { id: 'move', label: last ? 'Finish this stage' : 'Move to next stage' }, { id: 'stop', label: 'Stop following this outcome' }]
  if (!gone) choices.push({ id: 'open', label: 'Read the ended session' })
  return recommend(SESSION_ENDED_WHY, choices)
}
type NeedGoal = { state: string; note: string; waitingOnWorker?: boolean; reviewFailed?: boolean; noStep?: boolean; advice?: GoalAdvice }
/** What a pipeline that needs the person is waiting for, in one sentence, and the things they can do about it as
 *  buttons, the most likely first. `last` is whether the stage in hand is the pipeline's last. */
export function needsYou(goal: NeedGoal, reason: string | undefined, last: boolean): Need {
  const move: NeedChoice = { id: 'move', label: last ? 'Finish this stage' : 'Move to next stage' }
  if (goal.waitingOnWorker) return { why: 'The agent is waiting for your answer or approval. Gaffer carries on by itself once you have answered.', choices: [{ id: 'answer', label: 'Open the session to answer', primary: true }] }
  if (goal.reviewFailed) return recommend(goal.note, [{ id: 'retry', label: 'Try the review again', primary: true }, { id: 'tell', label: 'Tell the agent…' }, move])
  if (goal.state === 'limit') return recommend(goal.note || 'This stage used its review budget or its time.', [{ id: 'budget', label: 'Give it a new budget', primary: true }, move])
  if (goal.noStep) return recommend('Gaffer reviewed this and chose no next step, so it stopped. Nothing in particular is being asked of you.', [{ id: 'again', label: 'Have Gaffer look again', primary: true }, { id: 'tell', label: 'Tell the agent…' }, move, { id: 'open', label: 'Open the session' }])
  return recommend(reason || goal.note || 'Gaffer brought this back to you.', [{ id: 'again', label: 'Have Gaffer look again', primary: true }, { id: 'tell', label: 'Tell the agent…' }, move, { id: 'open', label: 'Open the session' }], goal.advice)
}
/** The pipeline's own note when it came back to review after it had been accepted: the work changed under the acceptance. */
export const CHANGED_AFTER_ACCEPT = 'Delivery review. The work changed after you accepted it'
export const changedAfterAccept = (note: string | undefined): boolean => (note ?? '').startsWith(CHANGED_AFTER_ACCEPT)
/** What a pipeline in delivery review says about itself from the outcome list alone, with no read of the machine:
 *  "Delivery review · 3 criteria", and how often it has already been sent back. */
export function deliveryWhy(piece: { criteria?: readonly unknown[]; deliveries?: readonly { decision: string }[]; note?: string }): string {
  const count = piece.criteria?.length ?? 0, back = (piece.deliveries ?? []).filter(d => d.decision === 'send_back').length
  return ['Delivery review', changedAfterAccept(piece.note) ? 'changed after you accepted it' : '', `${count} ${count === 1 ? 'criterion' : 'criteria'}`, back ? `sent back ${back === 1 ? 'once' : back === 2 ? 'twice' : `${back} times`} before` : ''].filter(Boolean).join(' · ')
}
type WaitingPiece = { key: string; title: string; state: string; note?: string; criteria?: readonly unknown[]; deliveries?: readonly { decision: string }[]; conversationId?: string; sessionId?: string; sessionState?: string; stage?: number; stages?: { kind: string; tool?: string; model?: string; first?: { tool: string; model?: string }; usageFailover?: boolean; backup?: { tool: string }; moreBackups?: { tool: string }[]; nextUsageBackup?: string }[] }
const TOOL_WORDS: Record<string, string> = { claude: 'Claude Code', codex: 'Codex', opencode: 'OpenCode', cline: 'Cline', antigravity: 'Google Antigravity (AGY)', copilot: 'GitHub Copilot' }
/** An agent and its model as the controller words a backup: "OpenCode (paid/backup)". */
const agentWords = (agent: { tool?: string; model?: string } | undefined): string => agent?.tool ? `${TOOL_WORDS[agent.tool] ?? agent.tool}${agent.model ? ` (${agent.model})` : ''}` : ''
/** What happened when a stage's worker reached its usage limit, from the stage's own record, and what each choice does. */
export function usageLimitWhy(stage: NonNullable<WaitingPiece['stages']>[number]): string {
  const ran = agentWords(stage), first = agentWords(stage.first), where = `on the ${stageLabel(stage.kind).toLowerCase()} stage`
  const none = `${stage.backup?.tool || stage.moreBackups?.length ? 'No backup is left for this stage' : 'No backup is saved for this stage'}; choose another model to carry on in the same folder and branch.`
  // The buttons say what each choice does; this says what happened. usageFailover is also set when the person chose the
  // other model, and there can be a third stop: say only what is always true.
  return [
    ran ? `${ran} reached its usage limit ${where}.` : `The worker reached its usage limit ${where}.`,
    stage.usageFailover && `Another model already took over on this stage after a usage limit${first ? `; it started on ${first}` : ''}.`,
    stage.nextUsageBackup ? `The backup is ${stage.nextUsageBackup}. Either choice carries on in the same folder and branch.` : none,
  ].filter(Boolean).join(' ')
}
/** One decision an outcome is waiting on the person for: which pipeline and stage, why, and the choices, recommended first. */
export type WaitingDecision<P> = { piece: P; stage: string; need: Need; kind: 'ended' | 'goal' | 'merge' | 'delivery' | 'usage_limit' }
/** Every decision an outcome is waiting on the person for, in pipeline order: a stage Gaffer brought back or that used
 *  its budget, a stage whose session ended, and a merge that waits for approval. */
export function waitingDecisions<P extends WaitingPiece>(record: { sessions: P[] }, goals: Map<string, { attention: boolean; reason?: string; goal: NeedGoal; workerUsageLimited?: boolean }>): WaitingDecision<P>[] {
  const out: WaitingDecision<P>[] = []
  for (const piece of record.sessions) {
    const stages = piece.stages ?? [], at = Math.min(piece.stage ?? 0, Math.max(stages.length - 1, 0))
    const stage = stages.length ? stageLabel(stages[at].kind) : '', last = (piece.stage ?? 0) >= stages.length - 1
    const goal = piece.conversationId ? goals.get(piece.conversationId) : undefined
    if (sessionLost(piece)) out.push({ piece, stage, kind: 'ended', need: sessionEndedChoices(last, piece.sessionState === 'gone') })
    else if (piece.state === 'delivery_review') out.push({ piece, stage, kind: 'delivery', need: recommend(deliveryWhy(piece), [{ id: 'review', label: 'Review delivery' }, { id: 'open', label: 'Open the session' }]) })
    else if (piece.state === 'ready_to_merge') out.push({ piece, stage, kind: 'merge', need: recommend('Ready to merge. Nothing merges until you approve it.', [{ id: 'merge', label: 'Approve the merge…' }, { id: 'open', label: 'Open the session' }]) })
    else if (goal && pipelineStatus(piece, goal.attention).label === 'Needs you') {
      if (goal.workerUsageLimited) {
        const backup = stages[at]?.nextUsageBackup
        const replace: NeedChoice = { id: 'replace-worker', label: 'Choose another model…', primary: !backup, recommended: !backup }
        const why = stages[at] ? usageLimitWhy(stages[at]) : ''
        const need = !stages[at]
          ? { why: goal.reason || goal.goal.note, choices: [{ id: 'open', label: 'Open worker session', primary: true } satisfies NeedChoice] }
          : backup
            ? recommend(why, [{ id: 'backup', label: 'Continue with the backup' }, replace])
            : { why, choices: [replace] }
        out.push({ piece, stage, kind: 'usage_limit', need })
      } else out.push({ piece, stage, kind: 'goal', need: needsYou(goal.goal, goal.reason, last) })
    }
  }
  return out
}
const DECISION_ABOUT: Record<string, string> = { question: 'Gaffer’s review brought the stage back', no_step: 'Gaffer’s review chose no next step', review_failed: 'Gaffer’s review did not complete', limit: 'The stage had used its budget', session_ended: 'The stage’s session had ended', agent_question: 'The agent asked a multiple-choice question' }
const DECISION_DID: Record<string, string> = { again: 'Reviewed the stage again', tell: 'Told the agent what to do', move: 'Moved to the next stage', budget: 'Gave the stage a new budget', restart: 'Ran the stage again in a new session', answer: 'Answered it' }
/** One of Gaffer's decisions in words: what had stopped the stage, and what Gaffer did about it. */
export function decisionText(d: { about: string; choice?: string; failed?: boolean }): { about: string; did: string } {
  return { about: DECISION_ABOUT[d.about] ?? 'The stage stopped', did: d.failed || !d.choice ? 'Left to you' : DECISION_DID[d.choice] ?? d.choice }
}
