import type { FolderProject, MachineHealth, TerminalSession, SessionState } from './terminal/api'
import type { SyncJob, SyncState } from './api'

// Pure logic for Home on today's data: the fleet strip's cards, the In flight groups and the Needs you list.
// No DOM, no fetch. Every figure is labelled with what it measures and comes from a record that exists today:
// the CPU reading from the health report, the session's own state or exit code, a finished sync check.
// Nothing here describes a step or a progress bar. The only worktree figure is the worktrees live sessions are working in
// now, labelled "in use": the listing has no record of which worktrees were removed, so it cannot count the rest.

export type Tone = 'good' | 'warn' | 'bad' | 'muted'
export type FleetCard = {
  /** 'paused' is a connected machine whose sync agent is paused: the word Control gives it (fleet.ts), never "online". */
  agentId: string; name: string; os: string; status: 'online' | 'paused' | 'offline' | 'reconnecting'
  /** Running and starting sessions of any kind on that machine: the listing cannot say which ones run an agent. A session
   *  whose end was asked for is not running; it is counted in sessionsEnding. */
  sessionsRunning: number
  /** Sessions on that machine whose end was asked for and is not yet confirmed. */
  sessionsEnding: number
  cpu: { label: 'CPU'; percent: number | null; text: string }
  /** Memory in use and space left on the home filesystem, from the same fresh report as the CPU; "No reading" when stale. */
  memory: { label: 'Memory'; percent: number | null; text: string }
  disk: { label: 'Disk'; percent: number | null; availableBytes: number | null }
  /** The distinct worktrees that live sessions on that machine are working in now ("in use"). The listing has no removal
   *  record, so a finished session's worktree is not counted: it may have been removed. */
  worktrees: number
  skills: { text: string; tone: 'good' | 'warn' | 'muted' }
}
export type SessionOwner = { id: string; name: string }
/** What the listing carries: the worktree fields only for a session started in its own worktree, and the folder project
 *  it was started in (`projectId`, null for one started outside a project). */
export type HomeSession = TerminalSession
export type GroupBy = 'project' | 'machine' | 'state'
export type Group = { key: string; label: string; count: number; sessions: HomeSession[] }
/** The Needs you card. Program 1's kinds arrive when a signal says a session wants the person; none does today. */
export type NeedCard = { kind: string; sessionId: string; at: number }

const MINUTE = 60_000, HOUR = 60 * MINUTE, DAY = 24 * HOUR
const LIVE: ReadonlySet<SessionState> = new Set(['creating', 'running', 'end_requested'])
/** A finished session stays in In flight this long after its last state change, then only Sessions lists it. */
export const FINISHED_KEPT_MS = DAY

const live = (s: Pick<TerminalSession, 'state'>): boolean => LIVE.has(s.state)

const reading = (percent: number | null, text: string) => percent == null ? { percent: null, text: 'No reading' } : { percent, text }

function memoryOf(m: MachineHealth): FleetCard['memory'] {
  const memory = freshReport(m)?.memory
  const total = memory?.totalBytes?.value, available = memory?.availableBytes?.value
  if (typeof total !== 'number' || typeof available !== 'number' || !(total > 0) || available < 0 || available > total) return { label: 'Memory', ...reading(null, 'No reading') }
  const used = (total - available) / total * 100
  return { label: 'Memory', ...reading(used, `${Math.round(used)}%`) }
}

function diskOf(m: MachineHealth): FleetCard['disk'] {
  const disk = freshReport(m)?.disk
  const total = disk?.totalBytes?.value, available = disk?.availableBytes?.value
  if (typeof available !== 'number') return { label: 'Disk', percent: null, availableBytes: null }
  const used = typeof total === 'number' && total > 0 && available >= 0 && available <= total ? (total - available) / total * 100 : null
  return { label: 'Disk', percent: used, availableBytes: available }
}

/** Whether the machine is reporting now, so its figures are current rather than last known. */
export const reporting = (card: Pick<FleetCard, 'status'>): boolean => card.status === 'online' || card.status === 'paused'

function statusOf(m: MachineHealth, paused: boolean): FleetCard['status'] {
  // A paused agent still reporting is paused, as Control says; one that stopped reporting is offline, the more pressing fact.
  if (m.state === 'connected') return paused ? 'paused' : 'online'
  // The controller has a connection record that is neither connected nor given up on: it is between attempts.
  if (m.state === 'unknown' && m.connection && !m.connection.connected) return 'reconnecting'
  return 'offline'
}

// One fresh report feeds all three: a stale one would draw load the machine may no longer have.
function freshReport(m: MachineHealth) { const c = m.connection; return c?.reportFresh ? c.report : undefined }

function cpuOf(m: MachineHealth): FleetCard['cpu'] {
  const c = m.connection
  // Only a fresh report is a reading; an old one would show load the machine may no longer have.
  const value = c?.reportFresh ? c.report?.cpuPercent?.value : null
  if (typeof value !== 'number' || !Number.isFinite(value)) return { label: 'CPU', percent: null, text: 'No reading' }
  return { label: 'CPU', percent: value, text: `${Math.round(value)}%` }
}

function ago(ms: number): string {
  if (ms < MINUTE) return 'just now'
  if (ms < HOUR) return `${Math.floor(ms / MINUTE)} min ago`
  if (ms < DAY) return `${Math.floor(ms / HOUR)} h ago`
  return `${Math.floor(ms / DAY)} d ago`
}

/** The differences a finished check found, or null when the job is not a finished check. */
function checkFound(job: SyncJob): number | null {
  const actions = Array.isArray(job.plan?.actions) ? job.plan.actions.length : null
  if (actions === null) return null
  // A preview (or an automatic run held for review) stops with its plan: the plan is what the check found.
  if ((job.kind === 'preview' || job.kind === 'auto') && job.status === 'awaiting-approval') return actions
  // An automatic run whose plan was empty finishes as an apply with nothing applied: it found no differences.
  // An apply that changed files is not a check, so it never reads as "in sync".
  if (job.kind === 'apply' && job.status === 'succeeded' && actions === 0) return 0
  return null
}

function skillsOf(agentId: string, sync: SyncState, now: number): FleetCard['skills'] {
  const notChecked = { text: 'Not checked', tone: 'muted' } as const
  const agent = (sync.agents ?? []).find(a => a.id === agentId)
  if (!agent) return notChecked
  const host = agent.host.toLowerCase()
  // Each target with the desired revision it is held to now. A job records the revision it was made for, so a check made
  // before the desired skills last changed says nothing about them and is not counted.
  const targets = new Map<string, string>()
  for (const t of sync.targets ?? []) if (t.target.host.toLowerCase() === host) targets.set(t.target.id, t.revision ?? '')
  if (targets.size === 0) return notChecked
  let differences = 0, oldest = Infinity, unchecked = false
  for (const [id, revision] of targets) {
    let latest: { found: number; at: number } | null = null
    for (const job of sync.jobs ?? []) {
      if (job.agentId !== agentId || job.targetId !== id) continue
      if (revision && job.revision !== revision) continue
      const found = checkFound(job)
      if (found !== null && (!latest || job.updatedAt > latest.at)) latest = { found, at: job.updatedAt }
    }
    if (!latest) { unchecked = true; continue }
    differences += latest.found
    oldest = Math.min(oldest, latest.at)
  }
  if (differences > 0) return { text: `${differences} skill${differences === 1 ? '' : 's'} out of date`, tone: 'warn' }
  if (unchecked) return notChecked
  return { text: `skills in sync · checked ${ago(Math.max(0, now - oldest))}`, tone: 'good' }
}

export function fleetCards(machines: MachineHealth[], sessions: TerminalSession[], sync: SyncState, now: number): FleetCard[] {
  return machines.map(m => ({
    agentId: m.agentId,
    name: m.host,
    os: m.capabilities?.os ?? '',
    status: statusOf(m, (sync.agents ?? []).some(a => a.id === m.agentId && a.paused)),
    sessionsRunning: sessions.filter(s => s.agentId === m.agentId && (s.state === 'running' || s.state === 'creating')).length,
    sessionsEnding: sessions.filter(s => s.agentId === m.agentId && s.state === 'end_requested').length,
    cpu: cpuOf(m),
    memory: memoryOf(m),
    disk: diskOf(m),
    worktrees: inUse(sessions.filter(s => s.agentId === m.agentId)),
    skills: skillsOf(m.agentId, sync, now),
  }))
}

/** The distinct worktrees live sessions are working in. */
function inUse(sessions: TerminalSession[]): number {
  return new Set(sessions.filter(s => live(s) && s.worktreePath).map(s => `${s.agentId}\u0000${s.worktreePath}`)).size
}

/** "no worktrees in use", "1 worktree in use", "3 worktrees in use". */
export function worktreesInUse(n: number): string {
  return n === 0 ? 'no worktrees in use' : `${n} ${n === 1 ? 'worktree' : 'worktrees'} in use`
}

/** A session belongs to the folder project it was started in: its own projectId, when that project is still on the account. */
export function sessionOwners(sessions: TerminalSession[], projects: Pick<FolderProject, 'id' | 'name'>[]): Map<string, SessionOwner> {
  const byId = new Map(projects.map(p => [p.id, p]))
  const owners = new Map<string, SessionOwner>()
  for (const s of sessions) {
    const project = s.projectId ? byId.get(s.projectId) : undefined
    if (project) owners.set(s.id, { id: project.id, name: project.name })
  }
  return owners
}

const machineName = (agentId: string, machines: MachineHealth[]): string => machines.find(m => m.agentId === agentId)?.host || agentId

export function stateWord(session: Pick<TerminalSession, 'state' | 'exitCode'>): { word: string; source: string; tone: Tone } {
  const fromState = "from the session's state"
  switch (session.state) {
    case 'creating': return { word: 'Starting', source: fromState, tone: 'muted' }
    case 'running': return { word: 'Running', source: fromState, tone: 'good' }
    case 'end_requested': return { word: 'Ending', source: fromState, tone: 'muted' }
    case 'ended': return { word: 'Ended', source: fromState, tone: 'muted' }
    case 'exited':
      if (typeof session.exitCode === 'number') return { word: `Exited ${session.exitCode}`, source: 'from the exit code', tone: session.exitCode === 0 ? 'muted' : 'bad' }
      return { word: 'Exited', source: fromState, tone: 'muted' }
    case 'lost': return { word: 'Connection lost', source: fromState, tone: 'warn' }
    default: return { word: String(session.state), source: fromState, tone: 'muted' }
  }
}

const STATE_ORDER = ['Running', 'Starting', 'Ending', 'Connection lost', 'Exited', 'Ended']
const stateGroup = (s: HomeSession): string => stateWord({ state: s.state, exitCode: null }).word

/**
 * In flight: live sessions, and finished ones for a day after their last state change. Grouped by project (the folder
 * project each session was started in, by name, then the rest under their machine), by machine (the person's machine
 * order) or by state.
 */
export function inFlight(sessions: HomeSession[], owners: Map<string, SessionOwner>, machines: MachineHealth[], groupBy: GroupBy, now: number): Group[] {
  const shown = sessions.filter(s => live(s) || now - s.updatedAt <= FINISHED_KEPT_MS)
  const groups = new Map<string, Group>()
  const add = (key: string, label: string, s: HomeSession) => {
    let g = groups.get(key)
    if (!g) groups.set(key, g = { key, label, count: 0, sessions: [] })
    g.sessions.push(s)
    g.count++
  }
  for (const s of shown) {
    const owner = owners.get(s.id)
    if (groupBy === 'project' && owner) add(`project:${owner.id}`, owner.name, s)
    else if (groupBy === 'project') add(`machine:${s.agentId}`, `No project · ${machineName(s.agentId, machines)}`, s)
    else if (groupBy === 'machine') add(`machine:${s.agentId}`, machineName(s.agentId, machines), s)
    else { const word = stateGroup(s); add(`state:${word}`, word, s) }
  }
  const machineRank = (key: string): number => {
    const i = machines.findIndex(m => `machine:${m.agentId}` === key)
    return i < 0 ? machines.length : i
  }
  const rank = (g: Group): [number, number, string] => {
    if (g.key.startsWith('project:')) return [0, 0, g.label.toLowerCase()]
    if (g.key.startsWith('machine:')) return [1, machineRank(g.key), g.label.toLowerCase()]
    const i = STATE_ORDER.indexOf(g.label)
    return [2, i < 0 ? STATE_ORDER.length : i, g.label.toLowerCase()]
  }
  const out = [...groups.values()]
  for (const g of out) g.sessions.sort((a, b) => b.updatedAt - a.updatedAt || a.id.localeCompare(b.id))
  return out.sort((a, b) => {
    const [x1, x2, x3] = rank(a), [y1, y2, y3] = rank(b)
    return x1 - y1 || x2 - y2 || x3.localeCompare(y3)
  })
}

/** `project · machine · branch`: the project only when the session was started in one, the branch (its worktree's)
 *  only when the listing carries one. */
export function rowSubtitle(session: HomeSession, owner: SessionOwner | undefined, machine: MachineHealth | undefined): string {
  return [owner?.name, machine?.host || session.agentId, session.worktreeBranch].filter(Boolean).join(' · ')
}

/**
 * Needs you is empty in this release: no existing signal says a session wants the person (an exit already shows in
 * In flight), and without a per-person "seen" record a card could not be dismissed.
 */
export function needsYou(_sessions: TerminalSession[], _now: number): NeedCard[] {
  return []
}

export function attentionCount(cards: NeedCard[]): number {
  return cards.length
}

/** Running and starting sessions per folder project, from each session's own projectId (an ending one is not running,
 *  as on the fleet card). */
export function projectRunning(sessions: TerminalSession[]): Map<string, number> {
  const counts = new Map<string, number>()
  for (const s of sessions) if (s.projectId && (s.state === 'running' || s.state === 'creating')) counts.set(s.projectId, (counts.get(s.projectId) ?? 0) + 1)
  return counts
}

/** Worktrees in use per folder project: the distinct worktrees its live sessions are working in. */
export function projectWorktrees(sessions: TerminalSession[]): Map<string, number> {
  const by = new Map<string, TerminalSession[]>()
  for (const s of sessions) if (s.projectId) by.set(s.projectId, [...(by.get(s.projectId) ?? []), s])
  const counts = new Map<string, number>()
  for (const [id, list] of by) { const n = inUse(list); if (n > 0) counts.set(id, n) }
  return counts
}

/**
 * The rail's state words for a project: its live sessions, else "machine offline" when its machine is known not to be
 * reporting, else "idle"; nothing when the sessions were not read. (The spec's "waiting" needs the attention signal, which
 * a later release brings.)
 */
export function railState(running: number | null, reporting: boolean | undefined): { text: string; tone: Tone } | null {
  if (running !== null && running > 0) return { text: `${running} running`, tone: 'good' }
  if (reporting === false) return { text: 'machine offline', tone: 'warn' }
  // `running` is null when the sessions were not read: nothing is known, so nothing is said (never a false "idle").
  return running === null ? null : { text: 'idle', tone: 'muted' }
}

/**
 * The Sessions tab's title: the session the person is in, then its project (or its machine when the session has no
 * project), then reet. Two reet tabs are told apart by the work they hold. With no session it is the page's name.
 */
export function sessionTabTitle(session: Pick<TerminalSession, 'title' | 'projectId'> | undefined, projectName?: string, machineName?: string): string {
  if (!session) return 'Sessions · reet.sh'
  return [session.title, projectName || machineName].filter(Boolean).join(' · ') + ' · reet.sh'
}

/** Age since the last state change, short: "now", "12m", "3h", "2d". */
export function shortAge(ms: number): string {
  if (ms < MINUTE) return 'now'
  if (ms < HOUR) return `${Math.floor(ms / MINUTE)}m`
  if (ms < DAY) return `${Math.floor(ms / HOUR)}h`
  return `${Math.floor(ms / DAY)}d`
}
