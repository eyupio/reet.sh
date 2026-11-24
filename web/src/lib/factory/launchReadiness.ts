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
export type Fact = {
  state: FactState; /** Short, for the line: "Agent’s default model". */ label: string; /** The sentence, with when it was read where that matters. */ text: string
  /** Being read, so neither a problem nor ready yet. */ pending?: boolean
  /** Not checked against this machine: the sentence saying what happens once the stage starts. */ later?: string
  /** Worth saying, never a thing to look at: it does not count against the stage and is never the summary's first item. */ quiet?: boolean
}
/** Where the agent came from: the saved crew's tier, the outcome's own agent, the person's change in this dialog, or the
 *  outcome's agent because the saved crew's is not on this machine. */
export type AgentSource = 'saved' | 'outcome' | 'chosen' | 'fallback'
export type Choice = { tool: string; model: string; effort?: string }
export type SpareReadiness = {
  agent: string; model: string; effort: string
  /** Balancing can use it: on this machine and signed in. After a usage limit the next backup is tried whatever this says. */
  eligible: boolean
  /** Whether balancing can move a stage onto it: the first choice's figures are fresh and it is signed in. */
  balances: boolean
  /** Its sign-ins or plan figures are still being read. */
  pending: boolean
  /** What it will do, in a few words. */
  verdict: string
  /** One short reason. */
  reason: string
}
export type StageReadiness = {
  machine: string; connected: Fact
  agent: Fact; model: Fact; effort: Fact; signIn: Fact
  source: AgentSource; sourceFact: Fact
  backups: SpareReadiness[]
  /** Model or effort was not checked against this machine: the sentences say what happens once the stage starts. */
  checkedAtStart: boolean
  /** What the stage runs on in one statement: "Claude Code · opus · high effort". Defaults are said here once, as a choice. */
  line: string
  /** Something is still being read, so the stage is neither ready nor a problem yet. */
  pending: boolean
  /** blocked: Approve waits. look: something to read, the stage may still start. ready: nothing to say. */
  level: 'ready' | 'look' | 'blocked'
  /** The first reason for `level`, '' when ready. */
  issue: string
  /** The machine's owner locked it: the stage is waiting to be unlocked there, not failing. */
  locked: boolean
}
/** What the machine lists for an agent: 'reading' while it is being read, null when it could not be. */
export type ModelsRead = MachineModels | null | 'reading'
export type ReadinessInput = {
  machine: TerminalMachine | null | undefined
  /** The agents this machine runs for chats. */
  tools: readonly string[]
  /** The effort levels the machine reports for an agent; the first is always '' (the agent's default). */
  efforts: (tool: string) => string[]
  models: (tool: string) => ModelsRead
  /** The machine's sign-ins: undefined while being read, null when they could not be. An answer that is not "read" still
   *  carries what the controller kept, and balancing goes by that. */
  accounts: MachineAccounts | null | undefined
  now: number
  /** The tier's choice as it will be asked for, with the effort that was asked even if the machine does not offer it, and
   *  its backups in order, exactly as they are sent. */
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
/** The sentence GafferModelSelect already uses for a model the machine's list does not confirm. */
export const MODEL_LATER = 'Not checked against this machine. If the agent does not offer this model, the stage stops when it starts; otherwise its Applied line says what it is running on.'
export const EFFORT_LATER = 'Not checked against this machine. If the agent does not offer this effort, the stage starts on the agent’s default effort; its Applied line says what it is running on.'

const name = (tool: string) => tool ? agentDisplayName(tool) : 'No agent'
const fact = (state: FactState, label: string, text = label, flags: Pick<Fact, 'pending' | 'later' | 'quiet'> = {}): Fact => ({ state, label, text, ...flags })
const capital = (word: string) => word ? word[0].toUpperCase() + word.slice(1) : ''
/** The accounts the controller holds for the machine, whether or not the latest read succeeded. */
const kept = (accounts: MachineAccounts | null | undefined): AgentAccount[] => accounts?.accounts ?? []

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

/** Plan figures of one agent, as the server judges them: read over six hours ago, they decide nothing. */
function figures(account: AgentAccount | undefined, now: number): 'fresh' | 'stale' | 'absent' {
  if (!account || account.state !== 'signed_in' || !planLeft(account, now)) return 'absent'
  return !account.limitsAt || now - account.limitsAt > BALANCE_FRESH_MS ? 'stale' : 'fresh'
}

function signInFact(input: ReadinessInput): Fact {
  const { accounts, member, machine, now } = input, agent = name(member.tool), host = machine?.host || 'this machine'
  if (accounts === undefined) return fact('unavailable', 'Sign-in', 'Reading the machine’s sign-ins…', { pending: true })
  const account = accountFor({ accounts: kept(accounts) }, member.tool)
  if (accounts === null || (!account && accounts.state !== 'read' && !kept(accounts).length)) return fact('unavailable', 'Sign-in not read', accounts?.sentence || 'reet could not read the sign-ins on this machine.')
  // The machine's read covers only the agents it has a reader for (not Cline). Nothing to look at, nothing to fix.
  if (!account) return fact('unavailable', 'Sign-in not reported', `${host} does not report ${agent}’s sign-in, so reet cannot tell. It is not needed to start.`, { quiet: true })
  if (account.state === 'none') return fact('missing', 'No sign-in found', startHint(account, agent, host))
  if (account.state === 'unreadable') return fact('unavailable', 'Sign-in unreadable', signInLine(account))
  const line = [signInLine(account), limitsNote(account, agent, now)].filter(Boolean).join('. ')
  // Only the read itself is not current: what the controller kept is still what balancing goes by.
  if (accounts.state !== 'read') return fact('stale', 'Sign-in not current', `${line}. ${accounts.checkedAt ? `Last read ${readAgo(accounts.checkedAt, now)}. ` : ''}${accounts.sentence}`)
  return fact('ready', 'Signed in', line)
}

function modelFact(input: ReadinessInput): Fact {
  const { member, machine, now } = input, model = member.model.trim(), host = machine?.host || 'this machine'
  if (!model) return fact('default', 'Agent’s default model', 'The agent’s default model, as you chose.')
  const list = input.models(member.tool)
  if (list === 'reading') return fact('unavailable', 'Reading models', `Reading ${name(member.tool)}’s models from ${host}…`, { pending: true })
  if (!list) return fact('unavailable', 'Model not checked', `reet could not read ${member.tool ? name(member.tool) : 'the agent'}’s models from ${host}.`, { later: MODEL_LATER })
  if (list.state !== 'listed') return fact('unavailable', 'Model not checked', modelStatus(list, machine?.host ?? '').sentence, { later: MODEL_LATER })
  const read = list.fetchedAt ? `read ${formatAge(now, list.fetchedAt)}` : ''
  if (!list.models.some(entry => entry.id === model)) {
    // A list that says it is partial cannot prove a model is missing.
    return list.more ? fact('unavailable', 'Model not checked', `${model} is not in the part of ${host}’s list that reet has.`, { later: MODEL_LATER })
      : fact('missing', 'Not in the list', `${host}’s list${read ? `, ${read},` : ''} has no ${model}.`, { later: MODEL_LATER })
  }
  if (list.fetchedAt && now - list.fetchedAt > MODELS_STALE_MS) return fact('stale', 'List is old', `${model} is in ${host}’s list, but that list was ${read}.`)
  return fact('ready', 'Listed', `${model} is in ${host}’s list${read ? `, ${read}` : ''}.`)
}

function effortFact(input: ReadinessInput): Fact {
  const { member, machine } = input, effort = member.effort ?? ''
  if (!effort) {
    // An agent with no effort setting has nothing to choose: that is not the person's choice to repeat back to them.
    return input.efforts(member.tool).length < 2 && machine?.capabilities ? fact('default', 'No effort setting', 'The agent has no effort setting here, so it uses its own.', { quiet: true }) : fact('default', 'Agent’s default effort', 'The agent’s default effort, as you chose.')
  }
  if (!machine?.capabilities?.acpStartOptions) return fact('unavailable', 'Effort not checked', 'This machine has not reported effort levels.', { later: EFFORT_LATER })
  if (!input.efforts(member.tool).includes(effort)) return fact('unavailable', 'Effort not offered', `${capital(effort)} effort is not offered by ${name(member.tool)} here, so the stage starts on the agent’s default effort.`)
  return fact('ready', `${capital(effort)} effort`, `${capital(effort)} effort is one the machine reports for ${name(member.tool)}.`)
}

/** One backup, judged as the controller does (internal/gaffer/outcome_balance.go, outcome_runs.go). Balancing moves a stage
 *  to a backup only when the first choice's figures are fresh and low, the backup is signed in and, if it has figures, has
 *  more left. After a usage limit the next configured backup is tried without any of that, and may be refused when it starts.
 *  shortcut: "more left" compares the tightest windows, where the server also matches a window that names a model. */
function spareReadiness(spare: Choice, first: AgentAccount | undefined, input: ReadinessInput): SpareReadiness {
  const { machine, accounts, now } = input, agent = name(spare.tool), host = machine?.host || 'this machine', firstName = name(input.member.tool)
  const out = { agent, model: spare.model.trim(), effort: spare.effort ?? '' }
  const after = 'after a usage limit it is still the next one tried'
  const no = (reason: string): SpareReadiness => ({ ...out, eligible: false, balances: false, pending: false, verdict: 'balancing will not use it', reason: `${reason} It is still tried after a usage limit, and is likely to be refused there.` })
  const yes = (balances: boolean, reason: string, verdict = balances ? 'can take over' : 'takes over only after a usage limit'): SpareReadiness => ({ ...out, eligible: true, balances, pending: false, verdict, reason })
  if (!input.tools.includes(spare.tool)) return no(`${agent} is not on ${host}.`)
  if (accounts === undefined) return { ...yes(false, 'Reading plan figures…'), pending: true }
  const account = accountFor({ accounts: kept(accounts) }, spare.tool)
  if (account && account.state !== 'signed_in') return no(`${signInLine(account)}.`)
  const stays = 'so stages stay on the first choice until it stops at its limit.'
  if (!kept(accounts).length) return yes(false, `Plan figures are not available, ${stays}`)
  if (!account) return yes(false, `${host} does not report ${agent}’s sign-in or plan, so balancing will not move a stage onto it; ${after}.`)
  if (spare.tool === input.member.tool) return yes(false, `It shares ${agent}’s plan with the first choice, so balancing moves a stage onto it only when a plan window for its model has more left; otherwise it takes over only after a usage limit.`)
  const one = figures(first, now), own = figures(account, now)
  const old = (who: string, at: AgentAccount | undefined) => `${who}’s plan figures were read ${readAgo(at?.limitsAt, now)}`
  if (one !== 'fresh') return yes(false, one === 'stale' ? `${old(firstName, first)}, ${stays}` : `${firstName} has no plan figures, ${stays}`)
  if (own === 'stale') return yes(true, `${old(agent, account)}, so it takes over without a comparison when ${firstName}’s plan runs low.`)
  if (own === 'absent') return yes(true, `${agent} has no plan figures, so it takes over without a comparison when ${firstName}’s plan runs low.`)
  const a = planLeft(first, now)!.left, b = planLeft(account, now)!.left
  return b > a
    ? yes(true, `${agent} has ${b}% of its plan left against ${firstName}’s ${a}%, so balancing can move a stage onto it when ${firstName}’s runs low.`)
    : yes(false, `${agent} has ${b}% of its plan left, no more than ${firstName}’s ${a}%, so balancing will not move a stage onto it; it takes over only after a usage limit.`)
}

/** What one crew tier will run on this machine, and how sure that is. */
export function launchReadiness(input: ReadinessInput): StageReadiness {
  const { machine, member } = input
  // The server's own verdict (`ready`) decides; readiness() says why it is not.
  const locked = !!(machine?.locked || machine?.capabilities?.locked)
  const gone = machine?.ready && !locked ? '' : readiness(machine ?? undefined) || 'The machine is not connected.'
  const connected = gone ? fact('unavailable', locked ? 'Waiting to be unlocked' : 'Machine not connected', gone) : fact('ready', 'Connected', `${machine?.host} is connected.`)
  const agent = !member.tool ? fact('missing', 'No agent', 'No agent is chosen for this tier.')
    : input.tools.includes(member.tool) ? fact('ready', name(member.tool), `${name(member.tool)} is on ${machine?.host || 'this machine'}.`)
    : fact('unavailable', 'Not on this machine', `${name(member.tool)} is not on ${machine?.host || 'this machine'} now.`)
  const model = modelFact(input), effort = effortFact(input), signIn = signInFact(input)
  const source = agentSource(input), sourceF = sourceFact(source, input)
  const first = accountFor({ accounts: kept(input.accounts) }, member.tool)
  const backups = input.spares.filter(spare => spare.tool).map(spare => spareReadiness(spare, first, input))
  const blocked = connected.state !== 'ready' ? connected : agent.state !== 'ready' ? agent : undefined
  // Things to read, in the order a person would fix them. A default is a choice, a source that is simply the outcome's is
  // not news, and something still being read or never reported is not a problem.
  const notes = [signIn, model, effort, sourceF].filter(item => !item.pending && !item.quiet && (item.state === 'missing' || item.state === 'stale' || item.state === 'unavailable'))
  const spareNote = backups.find(item => !item.pending && !item.eligible)
  const issue = blocked?.text ?? notes[0]?.text ?? spareNote?.reason ?? ''
  const wholeDefault = !member.model.trim() && effort.state === 'default' && !effort.quiet
  const asked = effort.label === 'Effort not offered' || !member.effort ? '' : `${member.effort} effort`
  const line = [name(member.tool), ...(wholeDefault ? ['Agent’s default model and effort'] : [member.model.trim() || 'Agent’s default model', ...(effort.state === 'default' && !effort.quiet ? ['Agent’s default effort'] : asked ? [asked] : [])])].join(' · ')
  return {
    machine: machine?.host ?? '', connected, agent, model, effort, signIn, source, sourceFact: sourceF, backups, line,
    checkedAtStart: [model, effort].some(item => item.later),
    pending: [signIn, model, effort].some(item => item.pending) || backups.some(item => item.pending),
    level: blocked ? 'blocked' : issue ? 'look' : 'ready', issue, locked,
  }
}

export type ReadinessItem = { level: 'blocked' | 'look' | 'note'; text: string; /** The machine, not a tier: it holds for every stage. */ machine?: true; /** The machine is locked: waiting, not failed. */ waiting?: true }

/** Everything one tier has to say. A machine that is not connected, or an agent that is not on it, is the whole story:
 *  the tier’s model, effort, sign-in and backups do not matter until that is fixed. */
export function readinessItems(ready: StageReadiness): ReadinessItem[] {
  if (ready.connected.state !== 'ready') return [{ level: 'blocked', text: ready.connected.text, machine: true, ...(ready.locked ? { waiting: true as const } : {}) }]
  if (ready.agent.state !== 'ready') return [{ level: 'blocked', text: ready.agent.text }]
  const facts = [ready.signIn, ready.model, ready.effort, ready.sourceFact].filter(item => !item.pending && item.state !== 'ready' && item.state !== 'default')
  const later = [...new Set([ready.model.later, ready.effort.later].filter((text): text is string => !!text))]
  // Backups that share an agent and a reason are one cause, and a backup on the first choice's own agent adds nothing
  // once that agent's sign-in is already said.
  const spares = new Map<string, string[]>()
  ready.backups.forEach((item, at) => {
    if (item.pending || item.eligible || (facts.includes(ready.signIn) && item.agent === ready.agent.label)) return
    const key = `${item.agent}: ${item.reason}`
    spares.set(key, [...(spares.get(key) ?? []), String(at + 1)])
  })
  return [
    ...facts.map((item): ReadinessItem => ({ level: item.quiet ? 'note' : 'look', text: item.text })),
    ...later.map((text): ReadinessItem => ({ level: 'note', text })),
    ...[...spares].map(([text, at]): ReadinessItem => ({ level: 'look', text: `${at.length > 1 ? 'Backups' : 'Backup'} ${inWords(at)}, ${text}` })),
  ]
}

export type LaunchCause = Omit<ReadinessItem, 'machine'> & { machine: boolean; /** Tier labels, Heavy first. */ crews: string[]; /** Stage labels in plan order. */ stages: string[] }
const CREW_ORDER = ['Heavy', 'Standard', 'Light'], LEVEL_ORDER = ['blocked', 'look', 'note']
/** "Heavy and Standard", "Code, Review and Test". */
export const inWords = (items: readonly string[]) => new Intl.ListFormat('en-GB', { type: 'conjunction' }).format(items)

/** The summary at the head of the pipelines: how many stages are ready, and each cause said once with who it affects. */
export function launchSummary(stages: readonly { /** "Code"; a part of a split stage uses the stage’s label. */ stage: string; /** The tier’s label: "Standard". */ crew: string; readiness: StageReadiness }[]): { ready: number; look: number; blocked: number; /** Of `blocked`, the stages on a locked machine. */ waiting: number; reading: number; text: string; blocker: string; causes: LaunchCause[] } {
  const count = (level: StageReadiness['level']) => stages.filter(item => item.readiness.level === level).length
  const waiting = stages.filter(item => item.readiness.level === 'blocked' && item.readiness.locked).length
  const reading = stages.filter(item => item.readiness.level === 'ready' && item.readiness.pending).length
  const ready = count('ready') - reading, look = count('look'), blocked = count('blocked')
  const found = new Map<string, LaunchCause>()
  for (const item of stages) for (const one of readinessItems(item.readiness)) {
    const cause = found.get(one.text) ?? { ...one, machine: !!one.machine, crews: [], stages: [] }
    found.set(one.text, cause)
    if (!cause.crews.includes(item.crew)) cause.crews.push(item.crew)
    if (!cause.stages.includes(item.stage)) cause.stages.push(item.stage)
  }
  // Array.sort is stable, so causes of one level stay in the order they were first seen.
  const causes = [...found.values()].map(cause => ({ ...cause, crews: cause.crews.sort((a, b) => CREW_ORDER.indexOf(a) - CREW_ORDER.indexOf(b)) })).sort((a, b) => LEVEL_ORDER.indexOf(a.level) - LEVEL_ORDER.indexOf(b.level))
  const first = causes.find(cause => cause.level === 'blocked')
  const parts = [`${ready} of ${stages.length} ${stages.length === 1 ? 'stage' : 'stages'} ready`, look ? `${look} to look at` : '', reading ? `${reading} still being read` : '', blocked - waiting ? `${blocked - waiting} cannot start` : '', waiting ? `${waiting} waiting to be unlocked` : ''].filter(Boolean)
  const blocker = first ? `${first.text}${first.machine ? '' : ` Change the agent of the ${inWords(first.crews)} ${first.crews.length === 1 ? 'crew' : 'crews'} under “Choose who does what”.`}` : ''
  return { ready, look, blocked, waiting, reading, text: `${parts.join(', ')}.`, blocker, causes }
}
