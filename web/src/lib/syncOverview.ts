import type { Machine, SyncEnvironment, SyncJob, SyncRun, SyncState, SyncSkill, SyncTarget } from './api'

const DAY = 86_400_000
export const FRESH_WINDOW = 14 * DAY
export const SYNCABLE_AGENTS = ['agents', 'claude', 'cursor', 'opencode'] as const
const syncable = new Set<string>(SYNCABLE_AGENTS)
const ACTIVE_JOB = new Set(['queued', 'running', 'awaiting-approval'])
const ACTIVE_RUN = new Set(['running', 'review'])
const RANK: Record<string, number> = { agents: 0, claude: 1, cursor: 2, opencode: 3 }

export function foldHost(value: string): string {
  return (value ?? '').trim().toLowerCase()
}

export function syncableAgent(agent: string): boolean {
  return syncable.has(agent)
}

export function isFresh(env: Pick<SyncEnvironment, 'complete' | 'scannedAt'> | undefined, now = Date.now()): boolean {
  return !!env && env.complete && env.scannedAt > now - FRESH_WINDOW
}

export function agentFor(snapshot: SyncState, host: string) {
  return (snapshot.agents ?? []).find(agent => foldHost(agent.host) === foldHost(host))
}

export function targetsFor(snapshot: SyncState, host: string): SyncTarget[] {
  return (snapshot.targets ?? []).filter(target =>
    foldHost(target.target.host) === foldHost(host) ||
    (target.hostAliases ?? []).some(alias => foldHost(alias) === foldHost(host)))
}

export function jobsFor(snapshot: SyncState, host: string): SyncJob[] {
  const agent = agentFor(snapshot, host)
  if (!agent) return []
  return (snapshot.jobs ?? []).filter(job => job.kind !== 'update' && job.agentId === agent.id)
}

export function activeJobsFor(snapshot: SyncState, host: string): SyncJob[] {
  return jobsFor(snapshot, host).filter(job => ACTIVE_JOB.has(job.status))
}

export function activeRunFor(snapshot: SyncState, host: string): SyncRun | undefined {
  const agent = agentFor(snapshot, host)
  if (!agent) return
  return (snapshot.runs ?? []).find(run => run.agentId === agent.id && ACTIVE_RUN.has(run.status))
}

export function agoLabel(at: number, now = Date.now()): string {
  if (!at) return 'never'
  const ms = Math.max(0, now - at)
  if (ms < 60_000) return 'just now'
  const minutes = Math.floor(ms / 60_000)
  if (minutes < 60) return `${minutes} min ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} hr ago`
  const days = Math.floor(hours / 24)
  return `${days} ${days === 1 ? 'day' : 'days'} ago`
}

export type SyncTone = 'synced' | 'action' | 'working' | 'review' | 'attention' | 'setup'

export type MachineSyncOverview = {
  tone: SyncTone
  label: string
  detail: string
  canSync: boolean
  href: string
  targets: number
  missing: number
  extra: number
  lastSync: number
  run?: SyncRun
  job?: SyncJob
  progress?: { done: number; total: number }
}

function envLabel(env?: { host: string; agent: string; project?: string }): string {
  if (!env) return 'an agent scope'
  return `${env.host} · ${env.agent}${env.project ? ' · project' : ''}`
}

function scopeLabel(target?: SyncTarget): string {
  return envLabel(target?.target)
}

function jobLabel(job: SyncJob): string {
  const kind = job.kind === 'apply' ? 'Installing' : job.kind === 'scan' ? 'Scanning' : job.kind === 'preview' ? 'Previewing' : 'Syncing'
  const state: Record<string, string> = {
    queued: 'Waiting for machine',
    running: kind,
    'awaiting-approval': 'Review needed',
    succeeded: 'Done',
    failed: 'Failed',
    cancelled: 'Cancelled',
  }
  return state[job.status] ?? job.status.replaceAll('-', ' ')
}

export function machineSyncOverview(machine: Machine, snapshot: SyncState, now = Date.now()): MachineSyncOverview {
  const agent = agentFor(snapshot, machine.host)
  const targets = targetsFor(snapshot, machine.host).filter(target => syncableAgent(target.target.agent))
  const jobs = jobsFor(snapshot, machine.host)
  const run = activeRunFor(snapshot, machine.host)
  const active = jobs.filter(job => ['queued', 'running'].includes(job.status)).sort((a, b) => b.updatedAt - a.updatedAt)[0]
  const missing = targets.reduce((sum, target) => sum + (target.missing?.length ?? 0), 0)
  const extra = targets.reduce((sum, target) => sum + (target.extra?.length ?? 0), 0)
  const lastSync = Math.max(0,
    ...jobs.filter(job => job.status === 'succeeded').map(job => job.updatedAt),
    ...targets.filter(target => target.appliedRevision === target.revision).map(target => target.updatedAt))
  const href = `/sync?machine=${encodeURIComponent(machine.host)}`
  const base = { href, targets: targets.length, missing, extra, lastSync, run, job: active }
  const targetEnvs: Array<SyncEnvironment | undefined> = targets.map(target => (snapshot.environments ?? []).find(env => env.id === target.target.id))
  const fresh = targets.length > 0 && targetEnvs.every(env => isFresh(env, now))
  const detailFor = (lead: string) => {
    const scopes = `${targets.length} agent ${targets.length === 1 ? 'scope' : 'scopes'}`
    return lastSync ? `${lead}${lead ? ' · ' : ''}${scopes} · last checked ${agoLabel(lastSync, now)}` : `${lead}${lead ? ' · ' : ''}${scopes} · never synced`
  }

  if (!agent) {
    return {
      ...base,
      tone: 'setup',
      label: 'Sync not connected',
      detail: 'Install the reet agent once, then install skills from here in one click.',
      canSync: false,
      href: '/machines#connect-machine',
    }
  }
  if (agent.paused) {
    return { ...base, tone: 'attention', label: 'Sync paused', detail: 'Resume sync in Settings before changes can be applied.', canSync: false }
  }
  if (run) {
    return {
      ...base,
      tone: 'working',
      label: run.status === 'review' ? 'Review needed' : 'Syncing now',
      detail: `${run.cursor} of ${run.targets.length} ${run.targets.length === 1 ? 'scope' : 'scopes'} complete · live updates`,
      canSync: false,
      progress: { done: run.cursor, total: run.targets.length },
    }
  }
  if (active) {
    return { ...base, tone: 'working', label: jobLabel(active), detail: `Working on ${scopeLabel(targets.find(target => target.target.id === active.targetId))}`, canSync: false }
  }
  if (jobs.some(job => job.status === 'awaiting-approval')) {
    const count = jobs.filter(job => job.status === 'awaiting-approval').length
    return { ...base, tone: 'review', label: 'Review needed', detail: `${count} planned ${count === 1 ? 'change needs' : 'changes need'} your approval before applying.`, canSync: false }
  }
  if (!targets.length) {
    return { ...base, tone: 'setup', label: 'No skills set to sync', detail: 'Choose the agent scopes and skills this machine should have.', canSync: false }
  }
  if (!fresh) {
    return { ...base, tone: 'attention', label: 'Rescan needed', detail: 'A complete agent scan from the last 14 days is needed before syncing.', canSync: false }
  }
  if (missing > 0) {
    return { ...base, tone: 'action', label: `${missing} skill${missing === 1 ? '' : 's'} to install`, detail: detailFor('Ready to apply'), canSync: true }
  }
  if (extra > 0 && targets.some(target => (target.mode ?? 'mirror') !== 'additive')) {
    return { ...base, tone: 'review', label: `${extra} extra skill${extra === 1 ? '' : 's'} found`, detail: 'An exact-set sync will ask before removing anything.', canSync: true }
  }
  const offline = agent.lastSeen <= now - 90_000
  return {
    ...base,
    tone: 'synced',
    label: 'Everything in sync',
    detail: `${lastSync ? `Last checked ${agoLabel(lastSync, now)}` : 'No changes found'}${offline ? ' · machine offline' : ''}`,
    canSync: true,
  }
}

export type JobStageState = 'pending' | 'active' | 'done' | 'failed' | 'review'
export type JobStage = { key: string; label: string; state: JobStageState }

export function syncJobStages(job: SyncJob): JobStage[] {
  const status = job.status
  if (job.kind === 'scan') {
    return [
      { key: 'scan', label: 'Scan agent folders', state: status === 'queued' ? 'pending' : status === 'running' ? 'active' : status === 'succeeded' ? 'done' : status === 'failed' ? 'failed' : 'pending' },
      { key: 'report', label: 'Report inventory', state: status === 'succeeded' ? 'done' : status === 'failed' ? 'failed' : 'pending' },
    ]
  }
  const applying = job.kind === 'apply'
  const read = status === 'queued' ? 'pending' : status === 'cancelled' ? 'pending' : 'done'
  const plan = status === 'queued' ? 'pending' : status === 'cancelled' ? 'pending' : status === 'failed' && !applying ? 'failed' : status === 'running' && !applying ? 'active' : 'done'
  const apply = status === 'succeeded' ? 'done' : status === 'awaiting-approval' ? 'review' : status === 'failed' && applying ? 'failed' : status === 'running' && applying ? 'active' : status === 'cancelled' ? 'pending' : 'pending'
  return [
    { key: 'read', label: 'Read machine', state: read },
    { key: 'plan', label: 'Plan changes', state: plan },
    { key: 'apply', label: 'Apply changes', state: apply },
  ]
}

export function jobPlanCounts(job: SyncJob): { installs: number; removals: number; total: number } {
  const actions = job.plan?.actions ?? []
  const installs = actions.filter(action => action.add).length
  const removals = actions.length - installs
  return { installs, removals, total: actions.length }
}

export function activeJobKindLabel(job: SyncJob): string {
  return job.kind === 'apply' ? 'Installing' : job.kind === 'preview' ? 'Previewing' : job.kind === 'scan' ? 'Scanning' : 'Syncing'
}

export type InstallDestination = {
  targetId: string
  /** The value save-desired accepts: shelf, hosted or a scanned environment id. */
  sourceId: string
  /** Where the candidate skills for this destination are read from. */
  candidateKey: string
  listId: number
  mode: string
  revision: string
  skillIds: string[]
  agent: string
  label: string
}

function sourceInfo(raw: string): { sourceId: string; candidateKey: string; listId: number } {
  const match = /^list:(\d+)$/.exec(raw)
  if (match) return { sourceId: 'shelf', candidateKey: raw, listId: Number(match[1]) }
  const sourceId = raw || 'shelf'
  return { sourceId, candidateKey: sourceId, listId: 0 }
}

export function savedInstallTargets(machine: Machine, snapshot: SyncState, now = Date.now()): InstallDestination[] {
  return targetsFor(snapshot, machine.host)
    .filter(target => syncableAgent(target.target.agent))
    .filter(target => isFresh((snapshot.environments ?? []).find(env => env.id === target.target.id), now))
    .map(target => ({
      targetId: target.target.id,
      ...sourceInfo(target.sourceId),
      mode: target.mode || 'additive',
      revision: target.revision,
      skillIds: (target.desired ?? []).map(skill => skill.id),
      agent: target.target.agent,
      label: scopeLabel(target),
    }))
}

export function fallbackInstallTarget(machine: Machine, snapshot: SyncState, now = Date.now()): InstallDestination | null {
  const scopes = (machine.groups ?? [])
    .filter(group => syncableAgent(group.agent) && isFresh(group, now))
    .sort((a, b) => (RANK[a.agent] ?? 9) - (RANK[b.agent] ?? 9) || Number(!!a.project) - Number(!!b.project))
  const scope = scopes[0]
  if (!scope) return null
  return {
    targetId: scope.id,
    sourceId: 'shelf',
    candidateKey: 'shelf',
    listId: 0,
    mode: 'additive',
    revision: '',
    skillIds: [],
    agent: scope.agent,
    label: envLabel(scope),
  }
}

export function pickSyncCandidate(candidates: SyncSkill[], skill: Pick<SyncSkill, 'id' | 'slug'>): SyncSkill | null {
  const exact = candidates.filter(candidate => candidate.id === skill.id)
  const bySlug = candidates.filter(candidate => candidate.slug === skill.slug)
  return [...exact, ...bySlug].find(candidate => !!candidate.source && candidate.sourceType !== 'local') ?? null
}
