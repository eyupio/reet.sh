// What a stage of a plan will run on, and how sure that is, before the person approves it. Pure: the dialog hands in what
// it already holds (the machine, what it reported, the saved crew, the choices on screen) and draws what comes back.
// It words nothing a helper already words: connection (readiness), sign-in (signInLine, startHint), models (modelStatus)
// and plan figures (planLeft, limitsNote) keep their sentences. The post-launch "Applied" line (stageChoice) is the
// controller's read-back of the same choice; this says so, and does not replace it.
import { accountFor, limitsNote, planLeft, readAgo, signInLine, startHint, type AgentAccount, type MachineAccounts } from '../agentAccounts'
import { agentDisplayName } from '../catalogue'
import { modelStatus, formatAge, type MachineModels } from '../models'
import { readiness, type TerminalMachine } from '../terminal/api'

/** Five distinct outcomes. `default` is a choice (left to the agent); `missing` is nothing where something is needed;
 *  `stale` is an old answer; `unavailable` cannot be used or asked now; `ready` is reported by the machine as chosen. */
export type FactState = 'ready' | 'default' | 'missing' | 'stale' | 'unavailable'
export type Fact = { state: FactState; /** Short, for the line: "Agent’s default model". */ label: string; /** The sentence, with when it was read where that matters. */ text: string; /** Not read yet, so not yet a thing to look at. */ pending?: boolean }
/** Where the agent came from: the saved crew's tier, the outcome's own agent, the person's change in this dialog, or the
 *  outcome's agent because the saved crew's is not on this machine. */
export type AgentSource = 'saved' | 'outcome' | 'chosen' | 'fallback'
export type Choice = { tool: string; model: string; effort?: string }
export type SpareReadiness = {
  agent: string; model: string; effort: string
  /** On this machine and signed in: able to take over at all. */
  eligible: boolean
  /** Whether plan figures will balance the stage onto it: both its own and the first choice's are fresh. */
  balances: boolean
  /** Its own or the first choice's plan figures are old: they will not be used to balance. */
  stale: boolean
  /** One short reason when it does not, or when it cannot take over. */
  reason: string
}
export type StageReadiness = {
  machine: string; connected: Fact
  agent: Fact; model: Fact; effort: Fact; signIn: Fact
  source: AgentSource; sourceFact: Fact
  backups: SpareReadiness[]
  /** Model or effort was not checked against this machine: the stage's Applied line confirms it once the stage starts. */
  checkedAtStart: boolean
  /** blocked: Approve waits. look: something to read, the stage may still start. ready: nothing to say. */
  level: 'ready' | 'look' | 'blocked'
  /** The first reason for `level`, '' when ready. */
  issue: string
}
export type ReadinessInput = {
  machine: TerminalMachine | null | undefined
  /** The agents this machine runs for chats. */
  tools: readonly string[]
  /** The effort levels the machine reports for an agent; the first is always '' (the agent's default). */
  efforts: (tool: string) => string[]
  /** What the machine lists for an agent: undefined or null until read. */
  models: (tool: string) => MachineModels | null | undefined
  /** The machine's sign-ins: undefined while being read, null when it could not be. */
  accounts: MachineAccounts | null | undefined
  now: number
  /** The tier's choice as it will be asked for, and its backups in order. */
  member: Choice
  spares: readonly Choice[]
  /** The saved crew's entry for this tier: null when none is saved, undefined when that was not read. */
  saved: { tool?: string; model?: string; effort?: string } | null | undefined
  /** The person has set this tier in the dialog (or it started from the saved crew). */
  edited: boolean
}

/** The server balances only on figures read within this long (internal/gaffer/outcome_balance.go). */
export const BALANCE_FRESH_MS = 6 * 3_600_000
/** A model list read longer ago than this is said to be old. */
export const MODELS_STALE_MS = 24 * 3_600_000
export const CHECKED_AT_START = 'Checked when the stage starts: its Applied line says Confirmed or Not as chosen.'

const name = (tool: string) => tool ? agentDisplayName(tool) : 'No agent'
const fact = (state: FactState, label: string, text = label, pending = false): Fact => ({ state, label, text, ...(pending ? { pending } : {}) })
const capital = (word: string) => word ? word[0].toUpperCase() + word.slice(1) : ''

export function agentSource(input: Pick<ReadinessInput, 'saved' | 'edited' | 'member' | 'tools'>): AgentSource {
  const { saved, edited, member, tools } = input
  if (!edited) return saved?.tool && !tools.includes(saved.tool) ? 'fallback' : 'outcome'
  const same = saved?.tool === member.tool && (saved.model ?? '') === member.model.trim() && (saved.effort ?? '') === (member.effort ?? '')
  return same ? 'saved' : 'chosen'
}

function sourceFact(source: AgentSource, input: ReadinessInput): Fact {
  const agent = name(input.member.tool)
  if (source === 'saved') return fact('ready', 'Saved crew', 'From your saved crew for this tier.')
  if (source === 'chosen') return fact('ready', 'Your choice', 'Chosen for this outcome.')
  if (source === 'fallback') return fact('unavailable', 'Outcome’s agent', `${name(input.saved?.tool ?? '')} from your saved crew is not on ${input.machine?.host || 'this machine'}, so this tier uses the outcome’s agent, ${agent}.`)
  return input.saved === null
    ? fact('missing', 'Outcome’s agent', `No crew is saved for this tier, so it uses the outcome’s agent, ${agent}.`)
    : fact('ready', 'Outcome’s agent', `This tier uses the outcome’s agent, ${agent}.`)
}

/** Sign-in and plan figures of one agent. `plan` is judged as the server judges it: figures read over six hours ago decide nothing. */
function figures(account: AgentAccount | undefined, now: number): 'fresh' | 'stale' | 'absent' {
  if (!account || account.state !== 'signed_in' || !planLeft(account, now)) return 'absent'
  return !account.limitsAt || now - account.limitsAt > BALANCE_FRESH_MS ? 'stale' : 'fresh'
}

function signInFact(input: ReadinessInput): Fact {
  const { accounts, member, machine, now } = input, agent = name(member.tool), host = machine?.host || 'this machine'
  if (accounts === undefined) return fact('unavailable', 'Sign-in', 'Reading the machine’s sign-ins…', true)
  if (accounts === null) return fact('unavailable', 'Sign-in not read', 'reet could not read the sign-ins on this machine.')
  if (accounts.state !== 'read') return fact('unavailable', 'Sign-in not read', accounts.sentence || `${host} cannot read its sign-ins just now.`)
  const account = accountFor(accounts, member.tool)
  if (!account) return fact('unavailable', 'Sign-in unknown', `${host} said nothing about ${agent}’s sign-in, so reet cannot tell.`)
  if (account.state === 'none') return fact('missing', 'No sign-in found', startHint(account, agent, host))
  if (account.state === 'unreadable') return fact('unavailable', 'Sign-in unreadable', signInLine(account))
  return fact('ready', 'Signed in', [signInLine(account), limitsNote(account, agent, now)].filter(Boolean).join('. '))
}

function modelFact(input: ReadinessInput): Fact {
  const { member, machine, now } = input, model = member.model.trim()
  if (!model) return fact('default', 'Agent’s default model', 'The agent’s default model, as you chose.')
  const list = input.models(member.tool)
  if (!list) return fact('unavailable', 'Model not checked', `reet has not read ${member.tool ? name(member.tool) : 'the agent'}’s models from ${machine?.host || 'this machine'}. ${CHECKED_AT_START}`)
  if (list.state !== 'listed') return fact('unavailable', 'Model not checked', `${modelStatus(list, machine?.host ?? '').sentence} ${CHECKED_AT_START}`)
  const read = list.fetchedAt ? `read ${formatAge(now, list.fetchedAt)}` : ''
  if (!list.models.some(entry => entry.id === model)) {
    // A list that says it is partial cannot prove a model is missing.
    return list.more ? fact('unavailable', 'Model not checked', `${model} is not in the part of ${machine?.host || 'the machine'}’s list that reet has. ${CHECKED_AT_START}`)
      : fact('missing', 'Not in the list', `${machine?.host || 'This machine'}’s list${read ? `, ${read},` : ''} has no ${model}. ${CHECKED_AT_START}`)
  }
  if (list.fetchedAt && now - list.fetchedAt > MODELS_STALE_MS) return fact('stale', 'List is old', `${model} is in ${machine?.host || 'the machine'}’s list, but that list was ${read}.`)
  return fact('ready', 'Listed', `${model} is in ${machine?.host || 'the machine'}’s list${read ? `, ${read}` : ''}.`)
}

function effortFact(input: ReadinessInput): Fact {
  const { member, machine } = input, effort = member.effort ?? ''
  if (!effort) return fact('default', 'Agent’s default effort', input.efforts(member.tool).length < 2 && machine?.capabilities ? 'The agent has no effort setting here, so it uses its own.' : 'The agent’s default effort, as you chose.')
  if (!machine?.capabilities?.acpStartOptions) return fact('unavailable', 'Effort not checked', `This machine has not reported effort levels. ${CHECKED_AT_START}`)
  if (!input.efforts(member.tool).includes(effort)) return fact('unavailable', 'Effort not offered', `${capital(effort)} effort is not offered by ${name(member.tool)} here, so the stage starts on the agent’s default effort.`)
  return fact('ready', `${capital(effort)} effort`, `${capital(effort)} effort is one the machine reports for ${name(member.tool)}.`)
}

function spareReadiness(spare: Choice, first: AgentAccount | undefined, input: ReadinessInput): SpareReadiness {
  const { machine, accounts, now } = input, agent = name(spare.tool), host = machine?.host || 'this machine'
  const out = { agent, model: spare.model.trim(), effort: spare.effort ?? '' }
  if (!input.tools.includes(spare.tool)) return { ...out, eligible: false, balances: false, stale: false, reason: `${agent} is not on ${host}, so it cannot take over.` }
  const account = accounts?.state === 'read' ? accountFor(accounts, spare.tool) : undefined
  if (account?.state === 'none') return { ...out, eligible: false, balances: false, stale: false, reason: `${signInLine(account)}, so it cannot take over.` }
  const own = figures(account, now), firstFigures = figures(first, now)
  const old = (a: AgentAccount | undefined, who: string) => `${who}’s plan figures were read ${readAgo(a?.limitsAt, now)}; figures over 6 hours old are not used to balance.`
  if (accounts?.state !== 'read') return { ...out, eligible: true, balances: false, stale: false, reason: 'Plan figures are not available, so it will not be used to balance.' }
  if (firstFigures !== 'fresh') return { ...out, eligible: true, balances: false, stale: firstFigures === 'stale', reason: firstFigures === 'stale' ? old(first, name(input.member.tool)) : `${name(input.member.tool)} has no plan figures, so it will not be used to balance.` }
  if (own !== 'fresh') return { ...out, eligible: true, balances: false, stale: own === 'stale', reason: own === 'stale' ? old(account, agent) : `${agent} has no plan figures, so it will not be used to balance.` }
  return { ...out, eligible: true, balances: true, stale: false, reason: '' }
}

/** What one crew tier will run on this machine, and how sure that is. */
export function launchReadiness(input: ReadinessInput): StageReadiness {
  const { machine, member } = input
  const gone = readiness(machine ?? undefined)
  const connected = gone ? fact('unavailable', 'Machine not connected', gone) : fact('ready', 'Connected', `${machine?.host} is connected.`)
  const agent = !member.tool ? fact('missing', 'No agent', 'No agent is chosen for this tier.')
    : input.tools.includes(member.tool) ? fact('ready', name(member.tool), `${name(member.tool)} is on ${machine?.host || 'this machine'}.`)
    : fact('unavailable', 'Not on this machine', `${name(member.tool)} is not on ${machine?.host || 'this machine'} now. Choose another agent for the stage.`)
  const model = modelFact(input), effort = effortFact(input), signIn = signInFact(input)
  const source = agentSource(input), sourceF = sourceFact(source, input)
  const first = input.accounts?.state === 'read' ? accountFor(input.accounts, member.tool) : undefined
  const backups = input.spares.filter(spare => spare.tool).map(spare => spareReadiness(spare, first, input))
  const blocked = connected.state !== 'ready' ? connected : agent.state !== 'ready' ? agent : undefined
  // Things to read, in the order a person would fix them. A default is a choice and a source that is simply the outcome's is not news.
  const notes = [signIn, model, effort, sourceF].filter(item => !item.pending && (item.state === 'missing' || item.state === 'stale' || item.state === 'unavailable'))
  const spareNote = backups.find(item => !item.eligible || item.stale)
  const issue = blocked?.text ?? notes[0]?.text ?? spareNote?.reason ?? ''
  return {
    machine: machine?.host ?? '', connected, agent, model, effort, signIn, source, sourceFact: sourceF, backups,
    checkedAtStart: [model, effort].some(item => item.text.includes(CHECKED_AT_START)),
    level: blocked ? 'blocked' : issue ? 'look' : 'ready', issue,
  }
}

/** The line above Approve: how many stages are ready, how many have something to read, and the first thing to fix. */
export function launchSummary(stages: readonly { label: string; where?: string; readiness: StageReadiness }[]): { ready: number; look: number; blocked: number; text: string; blocker: string } {
  const count = (level: StageReadiness['level']) => stages.filter(stage => stage.readiness.level === level).length
  const ready = count('ready'), look = count('look'), blocked = count('blocked')
  const first = stages.find(stage => stage.readiness.level === 'blocked') ?? stages.find(stage => stage.readiness.level === 'look')
  const where = first ? `${first.label}${first.where ? ` in ${first.where}` : ''}: ${first.readiness.issue}` : ''
  const parts = [`${ready} of ${stages.length} ${stages.length === 1 ? 'stage' : 'stages'} ready`, look ? `${look} to look at` : '', blocked ? `${blocked} cannot start` : ''].filter(Boolean)
  return { ready, look, blocked, text: `${parts.join(', ')}.${where ? ` First: ${where}` : ''}`, blocker: blocked ? `${first!.label}${first!.where ? ` in ${first!.where}` : ''}: ${first!.readiness.issue}` : '' }
}
