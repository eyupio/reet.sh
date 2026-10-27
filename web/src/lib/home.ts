import type { MachineHealth, TerminalSession, SessionState } from './terminal/api'
import type { SyncJob, SyncState } from './api'
import type { Project, ProjectRun } from './projects'

// Pure logic for Home on today's data: the fleet strip's cards, the In flight groups and the Needs you list.
// No DOM, no fetch. Every figure is labelled with what it measures and comes from a record that exists today:
// the CPU reading from the health report, the session's own state or exit code, a finished sync check.
// Nothing here describes a step, a progress bar or a worktree count without a record that says so.

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
  /** Absent in this release: the session listing cannot say "created by reet on that machine and not removed". */
  worktrees?: number
  skills: { text: string; tone: 'good' | 'warn' | 'muted' }
}
export type SessionOwner = { id: string; name: string }
/** What the listing carries today; the worktree branch is in the JSON for a session started in its own worktree. */
export type HomeSession = TerminalSession & { worktreeBranch?: string }
export type GroupBy = 'project' | 'machine' | 'state'
export type Group = { key: string; label: string; count: number; sessions: HomeSession[] }
/** The Needs you card. Program 1's kinds arrive when a signal says a session wants the person; none does today. */
export type NeedCard = { kind: string; sessionId: string; at: number }

const MINUTE = 60_000, HOUR = 60 * MINUTE, DAY = 24 * HOUR
const LIVE: ReadonlySet<SessionState> = new Set(['creating', 'running', 'end_requested'])
/** A finished session stays in In flight this long after its last state change, then only Sessions lists it. */
export const FINISHED_KEPT_MS = DAY

const live = (s: Pick<TerminalSession, 'state'>): boolean => LIVE.has(s.state)

/** Whether the machine is reporting now, so its figures are current rather than last known. */
export const reporting = (card: Pick<FleetCard, 'status'>): boolean => card.status === 'online' || card.status === 'paused'

function statusOf(m: MachineHealth, paused: boolean): FleetCard['status'] {
  // A paused agent still reporting is paused, as Control says; one that stopped reporting is offline, the more pressing fact.
  if (m.state === 'connected') return paused ? 'paused' : 'online'
  // The controller has a connection record that is neither connected nor given up on: it is between attempts.
  if (m.state === 'unknown' && m.connection && !m.connection.connected) return 'reconnecting'
  return 'offline'
}

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
    skills: skillsOf(m.agentId, sync, now),
  }))
}

/** A session belongs to a Project only when a member of one of that Project's runs carries its session id. */
export function sessionProjects(projects: Project[], runs: ProjectRun[]): Map<string, SessionOwner> {
  const byId = new Map(projects.map(p => [p.id, p]))
  const owners = new Map<string, SessionOwner>()
  for (const run of runs) {
    const project = byId.get(run.projectId)
    if (!project) continue
    for (const m of run.members) if (m.sessionId) owners.set(m.sessionId, { id: project.id, name: project.name })
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
 * In flight: live sessions, and finished ones for a day after their last state change. Grouped by Project (Projects
 * first, by name, then the rest under their machine), by machine (the person's machine order) or by state.
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

/** `project · machine`, with the branch only when the listing carries one. */
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

/**
 * The runs Home reads: each Project's last run when it started in the last 24 hours, or when the Projects list says it is
 * still running however long ago it started. Older finished runs' sessions sit under their machine.
 */
export function recentRunIds(projects: Project[], now: number): string[] {
  return projects.filter(p => p.lastRun && (p.lastRun.state === 'running' || now - p.lastRun.startedAt <= DAY)).map(p => p.lastRun!.id)
}

/** The rail's state words: the live sessions tied to the Project, else what its last run record says, else idle. */
export function projectState(project: Pick<Project, 'lastRun'>, running: number): string {
  if (running > 0) return `${running} running`
  if (project.lastRun?.state === 'running') return 'run in progress'
  return 'idle'
}

/** Live sessions per Project, from the same ownership In flight groups by. */
export function projectRunning(sessions: TerminalSession[], owners: Map<string, SessionOwner>): Map<string, number> {
  const counts = new Map<string, number>()
  for (const s of sessions) {
    const owner = owners.get(s.id)
    if (owner && live(s)) counts.set(owner.id, (counts.get(owner.id) ?? 0) + 1)
  }
  return counts
}

/** Age since the last state change, short: "now", "12m", "3h", "2d". */
export function shortAge(ms: number): string {
  if (ms < MINUTE) return 'now'
  if (ms < HOUR) return `${Math.floor(ms / MINUTE)}m`
  if (ms < DAY) return `${Math.floor(ms / HOUR)}h`
  return `${Math.floor(ms / DAY)}d`
}
