import { STAGES, defaultTier, stageLabel, type CrewTier } from '../gafferOutcome'
import type { GafferCrew, Pathway, PathwayAgent, PathwaySkill, PathwayStage } from '../api'

// A pathway is a person's saved line of stages. These are the rules the server holds it to (internal/gaffer/pathways.go),
// so the editor can say why a change is refused before the server is asked. Every edit returns a new pathway.
export type { Pathway, PathwayAgent, PathwaySkill, PathwayStage }

export const MAX_PATHWAY_STAGES = 12
export const MAX_STAGE_SKILLS = 2
export const MAX_STAGE_GUIDANCE_BYTES = 6000
export const MAX_STAGE_PROMPT_BYTES = 4000
export const MAX_STAGE_NAME = 40
export const MAX_PATHWAY_NAME = 60
export const MAX_PATHWAY_DESCRIPTION_BYTES = 300

const encoder = new TextEncoder()
export const byteLength = (text: string): number => encoder.encode(text).length
const chars = (text: string) => [...text].length
const bytesWord = (n: number) => n.toLocaleString('en-GB')
const CUSTOM = 'custom'
/** A copy of `list` with `del` items removed at `at` and `items` put there (the lib target has no toSpliced). */
const splice = <T>(list: T[], at: number, del: number, ...items: T[]): T[] => { const copy = [...list]; copy.splice(at, del, ...items); return copy }
const TOO_MANY = `A pathway can have up to ${MAX_PATHWAY_STAGES} stages.`

export const blankPathway = (): Pathway => ({ id: '', name: '', description: '', enabled: true, stages: [], createdAt: 0, updatedAt: 0 })
export const standardPathway = (): Pathway => ({
  ...blankPathway(), name: 'Standard',
  stages: ['code', 'review', 'test', 'pr', 'babysit', 'merge'].map(kind => ({ kind, tier: defaultTier(kind) })),
})

export function newStage(kind: string, taken: PathwayStage[]): PathwayStage {
  if (kind !== CUSTOM) return { kind, tier: defaultTier(kind) }
  const used = new Set(taken.map(s => s.name?.trim().toLowerCase()))
  let n = 1
  while (used.has(`stage ${n}`)) n++
  return { kind, name: `Stage ${n}`, prompt: '' }
}

/** The first broken order rule, with the stage it is about, or null. Custom stages sit anywhere. */
function orderBreak(kinds: string[]): { at: number; text: string } | null {
  const seen = new Set<string>()
  for (let at = 0; at < kinds.length; at++) {
    const kind = kinds[at]
    if (kind === CUSTOM) continue
    const label = stageLabel(kind)
    if (seen.has(kind)) return { at, text: `${label} is already in this pathway.` }
    seen.add(kind)
    if (kind === 'merge' && at !== kinds.length - 1) return { at, text: 'Merge is always the last stage.' }
    if ((kind === 'babysit' || kind === 'merge') && !kinds.slice(0, at).includes('pr')) return { at, text: `${label} needs a PR stage before it.` }
  }
  return null
}
const reason = (kinds: string[]) => orderBreak(kinds)?.text ?? ''

export function canInsert(stages: PathwayStage[], stage: PathwayStage, at: number): string {
  if (stages.length >= MAX_PATHWAY_STAGES) return TOO_MANY
  return reason(splice(stages.map(s => s.kind), at, 0, stage.kind))
}
export function canMove(stages: PathwayStage[], from: number, to: number): string {
  const kinds = stages.map(s => s.kind)
  return reason(splice(kinds.filter((_, i) => i !== from), to, 0, kinds[from]))
}

const withStages = (p: Pathway, stages: PathwayStage[]): Pathway => ({ ...p, stages })
export const insertStage = (p: Pathway, stage: PathwayStage, at: number): Pathway => withStages(p, splice(p.stages, at, 0, stage))
export const moveStage = (p: Pathway, from: number, to: number): Pathway => withStages(p, splice(p.stages.filter((_, i) => i !== from), to, 0, p.stages[from]))
export const removeStage = (p: Pathway, at: number): Pathway => withStages(p, splice(p.stages, at, 1))
/** A tier and an agent are alternatives: setting one clears the other. */
export function patchStage(p: Pathway, at: number, patch: Partial<PathwayStage>): Pathway {
  const stage: PathwayStage = { ...p.stages[at], ...patch }
  if (patch.tier) delete stage.agent
  else if (patch.agent) delete stage.tier
  return withStages(p, splice(p.stages, at, 1, stage))
}

const guidance = (ids: string[], shelf: PathwaySkill[]) => ids.reduce((sum, id) => sum + (shelf.find(s => s.id === id)?.bytes ?? 0), 0)
const OVER = `These skills together add more than ${bytesWord(MAX_STAGE_GUIDANCE_BYTES)} bytes of guidance. Choose shorter ones.`
export function canAddSkill(stage: PathwayStage, skill: PathwaySkill, shelf: PathwaySkill[]): string {
  const have = stage.skills ?? []
  if (have.includes(skill.id)) return 'That skill is already on this stage.'
  if (have.length >= MAX_STAGE_SKILLS) return `A stage can have up to ${MAX_STAGE_SKILLS} skills.`
  return guidance(have, shelf) + skill.bytes > MAX_STAGE_GUIDANCE_BYTES ? OVER : ''
}
const withSkills = (p: Pathway, at: number, skills: string[]): Pathway => {
  const stage = { ...p.stages[at], skills }
  if (!skills.length) delete (stage as Partial<PathwayStage>).skills
  return withStages(p, splice(p.stages, at, 1, stage))
}
export const addSkill = (p: Pathway, at: number, id: string): Pathway => withSkills(p, at, [...(p.stages[at].skills ?? []), id])
export const removeSkill = (p: Pathway, at: number, id: string): Pathway => withSkills(p, at, (p.stages[at].skills ?? []).filter(s => s !== id))

export type StageProblem = { at: number; field: 'name' | 'prompt' | 'skills' | 'order' | 'who'; text: string }

/** Every rule the server would refuse the pathway for, as a sentence each. A null shelf is one not loaded yet: skills are not checked against it. */
export function pathwayProblems(p: Pathway, shelf: PathwaySkill[] | null): { pathway: string[]; stages: StageProblem[] } {
  const pathway: string[] = []
  const stages: StageProblem[] = []
  const name = p.name.trim()
  if (!name) pathway.push('Give the pathway a name.')
  else if (chars(name) > MAX_PATHWAY_NAME) pathway.push(`Keep the name to ${MAX_PATHWAY_NAME} characters.`)
  if (byteLength(p.description.trim()) > MAX_PATHWAY_DESCRIPTION_BYTES) pathway.push(`Keep the description to ${MAX_PATHWAY_DESCRIPTION_BYTES} bytes.`)
  if (!p.stages.length) pathway.push('Add at least one stage.')
  else if (p.stages.length > MAX_PATHWAY_STAGES) pathway.push(TOO_MANY)

  const taken: string[] = []
  p.stages.forEach((stage, at) => {
    const add = (field: StageProblem['field'], text: string) => stages.push({ at, field, text })
    const prompt = stage.prompt?.trim() ?? ''
    if (stage.kind === CUSTOM) {
      const own = stage.name?.trim() ?? ''
      const lower = own.toLowerCase()
      if (!own) add('name', 'Give this stage a name.')
      else if (chars(own) > MAX_STAGE_NAME) add('name', `Keep the stage name to ${MAX_STAGE_NAME} characters.`)
      else if (lower === CUSTOM || STAGES.some(s => s.kind === lower)) add('name', `${own} is a built-in stage's name. Choose another.`)
      else if (taken.includes(lower)) add('name', `Another stage is already named ${own}. Give each its own name.`)
      if (own) taken.push(lower)
      if (!prompt) add('prompt', `${own || 'This stage'} needs a prompt.`)
    }
    if (byteLength(prompt) > MAX_STAGE_PROMPT_BYTES) add('prompt', `Keep the prompt to ${bytesWord(MAX_STAGE_PROMPT_BYTES)} bytes.`)
    if (stage.agent && !stage.agent.tool.trim()) add('who', 'Choose an agent for this stage.')
    const skills = stage.skills ?? []
    if (skills.length > MAX_STAGE_SKILLS) add('skills', `A stage can have up to ${MAX_STAGE_SKILLS} skills.`)
    else if (new Set(skills).size < skills.length) add('skills', 'Each skill can be on a stage once.')
    else if (shelf) {
      for (const id of skills) if (!shelf.some(s => s.id === id)) add('skills', `The skill ${id} is no longer available. Remove it.`)
      if (guidance(skills, shelf) > MAX_STAGE_GUIDANCE_BYTES) add('skills', OVER)
    }
  })
  const broken = orderBreak(p.stages.map(s => s.kind))
  if (broken) stages.push({ at: broken.at, field: 'order', text: broken.text })
  return { pathway, stages }
}

export const stageTitle = (stage: PathwayStage): string => stage.kind === CUSTOM ? stage.name ?? '' : stageLabel(stage.kind)
/** Who does the stage: its crew tier, or the model and effort it is pinned to. */
export function stageWho(stage: PathwayStage): string {
  if (stage.agent) return [stage.agent.model || stage.agent.tool, stage.agent.effort].filter(Boolean).join(' · ')
  return `${(stage.tier ?? defaultTier(stage.kind)).toLowerCase()} · crew`
}

/** Why a stage runs on the agent it does: named by the stage, a tier's saved crew, the outcome's agent, or not known. */
export type PathwayRunOrigin = 'pathway' | 'saved-crew' | 'outcome' | 'unknown-crew'
export type PathwayRunMember = { tool: string; model: string; effort: string }
export type PathwayRun = {
  stage: PathwayStage
  /** null when the stage names its own agent. */
  tier: CrewTier | null
  origin: PathwayRunOrigin
  member: PathwayRunMember
  /** Backups of the saved crew member, in tried order. A stage own agent has none. */
  spares: PathwayRunMember[]
}
const memberOf = (one: { tool?: string; model?: string; effort?: string }): PathwayRunMember =>
  ({ tool: one.tool ?? '', model: one.model ?? '', effort: one.effort ?? '' })

/** What each stage runs on as the controller resolves it (outcomePieces): the stage's own agent, else its tier's saved
 *  crew member with that member's backups, else the outcome's agent. A crew that was not read is not guessed. */
export function resolvePathwayRuns(
  pathway: Pathway, savedCrew: GafferCrew | null | undefined,
  outcome: { tool: string; model: string; effort?: string }, crewKnown = true,
): PathwayRun[] {
  return pathway.stages.map(stage => {
    if (stage.agent) return { stage, tier: null, origin: 'pathway' as const, member: memberOf(stage.agent), spares: [] }
    const tier = stage.tier ?? defaultTier(stage.kind)
    if (!crewKnown) return { stage, tier, origin: 'unknown-crew' as const, member: memberOf({}), spares: [] }
    const member = savedCrew?.[tier]
    if (!member?.tool) return { stage, tier, origin: 'outcome' as const, member: memberOf(outcome), spares: [] }
    const spares = [member.backup, ...(member.moreBackups ?? [])].flatMap(one => one?.tool ? [memberOf(one)] : [])
    return { stage, tier, origin: 'saved-crew' as const, member: memberOf(member), spares }
  })
}

const stable = (_: string, v: unknown) => v && typeof v === 'object' && !Array.isArray(v) ? Object.fromEntries(Object.entries(v).sort(([a], [b]) => a < b ? -1 : 1)) : v
/** The unsaved-changes test: the same pathway as far as a person can edit it. */
export const sameEdit = (a: Pathway, b: Pathway): boolean => {
  const edit = ({ updatedAt: _t, problems: _p, ...rest }: Pathway) => JSON.stringify(rest, stable)
  return edit(a) === edit(b)
}
/** The same stages, in the same order, with the same settings. */
export const sameStages = (a: PathwayStage[], b: PathwayStage[]): boolean => JSON.stringify(a, stable) === JSON.stringify(b, stable)
