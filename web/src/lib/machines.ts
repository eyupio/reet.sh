import type { Environment, Machine, ReleaseInfo, SyncAgent, SyncEnvironment, SyncJob, SyncSkill } from './api'

export function groupLabel(env: Environment & { project?: string }): string {
  const names: Record<string, string> = { agents: 'Shared agents / Codex', claude: 'Claude Code', codex: 'Legacy Codex (scan only)', cursor: 'Cursor', opencode: 'OpenCode', reet: 'Scanner', 'reet-server': 'Server' }
  return `${names[env.agent] ?? env.agent}${env.project ? ` · ${env.project}` : ''}`
}
export function agentGroups(machine?: Machine): SyncEnvironment[] { return machine?.groups.filter(g => !g.agent.startsWith('reet')) ?? [] }
// The same window the fleet uses to call an agent online. A job's fix cannot
// run here while the machine is away, so the UI hides the command and retry.
export const AGENT_ONLINE_WINDOW = 90_000
export function agentOnline(agent: Pick<SyncAgent, 'lastSeen'> | undefined, now = Date.now()): boolean {
  return !!agent && agent.lastSeen > now - AGENT_ONLINE_WINDOW
}
// Heartbeats identify running servers; only agent inventories belong in shelf filters.
export function inventoryMachines(machines: Machine[]): Machine[] {
  return machines.filter(machine => agentGroups(machine).length > 0)
}
export function inventoryGroups(machine?: Machine, group = ''): SyncEnvironment[] { return agentGroups(machine).filter(g => !group || g.id === group) }
export function inventory(groups: SyncEnvironment[]): SyncSkill[] {
  return [...new Map(groups.flatMap(g => g.skills ?? []).map(s => [s.id, s])).values()].sort((a, b) => a.name.localeCompare(b.name))
}
export function recentComplete(groups: SyncEnvironment[], now: number): boolean {
  return groups.length > 0 && groups.every(g => g.complete && g.scannedAt > now - 14 * 86400000)
}

export type InstallationReport = { host: string; installed: Set<string>; complete: boolean; groupCount: number }
export type InstallationStatus = 'installed' | 'missing' | 'unknown'

// Index each scan once for the whole shelf, rather than searching every inventory for every card.
export function installationReports(machines: Machine[], now: number): InstallationReport[] {
  return machines.map(machine => {
    const groups = agentGroups(machine)
    const confirmedGroups = groups.filter(group => recentComplete([group], now))
    return {
      host: machine.host,
      installed: new Set(confirmedGroups.flatMap(group => (group.skills ?? []).map(skill => skill.id))),
      complete: recentComplete(groups, now),
      groupCount: groups.length,
    }
  })
}

export function installationStatus(report: InstallationReport, skillId: string): InstallationStatus {
  if (report.installed.has(skillId)) return 'installed'
  return report.complete ? 'missing' : 'unknown'
}
export function compareInventories(left: SyncEnvironment[], right: SyncEnvironment[], now: number) {
  const a = inventory(left), b = inventory(right)
  const aIDs = new Set(a.map(s => s.id)), bIDs = new Set(b.map(s => s.id))
  return {
    common: a.filter(s => bIDs.has(s.id)),
    leftOnly: a.filter(s => !bIDs.has(s.id)),
    rightOnly: b.filter(s => !aIDs.has(s.id)),
    confirmed: recentComplete(left, now) && recentComplete(right, now),
    unknownSources: [...a, ...b].some(s => s.sourceType === 'local' || !s.source),
  }
}


export function shortCommit(commit?: string): string {
  const value = (commit ?? '').trim()
  return value && value.toLowerCase() !== 'unknown' ? value.slice(0, 12) : ''
}

export function agentBuild(agent: Pick<SyncAgent, 'version' | 'commit'>): string {
  const commit = shortCommit(agent.commit)
  if (agent.version === 'dev') return commit ? `dev · ${commit}` : 'dev · commit unknown'
  return commit ? `${agent.version} · ${commit}` : agent.version
}

export function agentUpdateTarget(agent: Pick<SyncAgent, 'version'>): 'dev' | 'latest' {
  return agent.version === 'dev' ? 'dev' : 'latest'
}

export type LatestRelease = { channel: 'dev' | 'latest'; version: string; commit?: string; label: string; error?: string }

export function latestRelease(agent: Pick<SyncAgent, 'version'>, releases?: ReleaseInfo): LatestRelease | null {
  if (!releases) return null
  if (agent.version === 'dev') {
    const channel = releases.dev
    if (!channel || (!channel.commit && !channel.error)) return null
    const commit = shortCommit(channel.commit)
    return { channel: 'dev', version: 'dev', commit: channel.commit, label: commit ? `dev · ${commit}` : 'dev', error: channel.error }
  }
  const channel = releases.latest
  if (!channel || (!channel.version && !channel.error)) return null
  return { channel: 'latest', version: channel.version ?? '', label: channel.version ?? 'latest', error: channel.error }
}

export function updateAvailable(agent: Pick<SyncAgent, 'version' | 'commit'>, releases?: ReleaseInfo): boolean {
  const latest = latestRelease(agent, releases)
  if (!latest || latest.error) return false
  if (latest.channel === 'dev') {
    const current = (agent.commit ?? '').trim().toLowerCase()
    return !current || current === 'unknown' || !latest.commit || current !== latest.commit.toLowerCase()
  }
  return versionLess(agent.version, latest.version)
}

type ParsedVersion = { major: number; minor: number; patch: number; pre: string }

function parseVersion(raw: string): ParsedVersion | null {
  let value = (raw ?? '').trim().replace(/^v/i, '')
  const plus = value.indexOf('+')
  if (plus >= 0) value = value.slice(0, plus)
  let pre = ''
  const dash = value.indexOf('-')
  if (dash >= 0) {
    pre = value.slice(dash + 1)
    value = value.slice(0, dash)
  }
  const parts = value.split('.')
  if (parts.length !== 3) return null
  const numbers = parts.map(part => /^(0|[1-9][0-9]*)$/.test(part) ? Number(part) : NaN)
  if (numbers.some(Number.isNaN)) return null
  return { major: numbers[0], minor: numbers[1], patch: numbers[2], pre }
}

function comparePrerelease(a: string, b: string): number {
  if (a === b) return 0
  if (!a) return 1
  if (!b) return -1
  const left = a.split('.'), right = b.split('.')
  for (let i = 0; i < left.length && i < right.length; i++) {
    const x = Number(left[i]), y = Number(right[i])
    const xNumeric = /^[0-9]+$/.test(left[i]), yNumeric = /^[0-9]+$/.test(right[i])
    if (xNumeric && yNumeric) {
      if (x !== y) return x < y ? -1 : 1
    } else if (xNumeric) {
      return -1
    } else if (yNumeric) {
      return 1
    } else if (left[i] !== right[i]) {
      return left[i] < right[i] ? -1 : 1
    }
  }
  return left.length < right.length ? -1 : left.length > right.length ? 1 : 0
}

export function versionLess(a: string, b: string): boolean {
  const left = parseVersion(a), right = parseVersion(b)
  if (!left || !right) return false
  if (left.major !== right.major) return left.major < right.major
  if (left.minor !== right.minor) return left.minor < right.minor
  if (left.patch !== right.patch) return left.patch < right.patch
  return comparePrerelease(left.pre, right.pre) < 0
}

export function updateJobFor(jobs: SyncJob[] | undefined, agentId: string): SyncJob | undefined {
  return (jobs ?? []).filter(job => job.agentId === agentId && job.kind === 'update').sort((a, b) => b.createdAt - a.createdAt)[0]
}

export function updateProgress(job?: SyncJob): { done: number; total: number; value: number } {
  const stages = job?.stages ?? []
  const done = stages.filter(stage => stage.status === 'done' || stage.status === 'skipped').length
  return { done, total: stages.length, value: stages.length ? (job?.status === 'running' ? Math.max(0.04, done / stages.length) : done / stages.length) : 0 }
}

export type AgentUpdateState = 'current' | 'available' | 'queued' | 'running' | 'failed' | 'rolled_back' | 'cancelled' | 'unknown'

export function agentUpdateState(agent: Pick<SyncAgent, 'version' | 'commit'>, releases: ReleaseInfo | undefined, job?: SyncJob): AgentUpdateState {
  if (job) {
    if (job.status === 'queued') return 'queued'
    if (job.status === 'running') return 'running'
    if (job.status === 'rolled_back') return 'rolled_back'
    if (job.status === 'cancelled') return 'cancelled'
    if (job.status === 'failed') return 'failed'
    if (job.status === 'succeeded') return updateAvailable(agent, releases) ? 'available' : 'current'
  }
  if (!releases) return 'unknown'
  if (updateAvailable(agent, releases)) return 'available'
  const latest = latestRelease(agent, releases)
  if (!latest || latest.error) return 'unknown'
  return 'current'
}

export function updateStateLabel(state: AgentUpdateState): string {
  return ({
    current: 'Up to date',
    available: 'Update available',
    queued: 'Update queued',
    running: 'Updating',
    failed: 'Update failed',
    rolled_back: 'Rolled back',
    cancelled: 'Update cancelled',
    unknown: 'Not checked',
  })[state]
}

// A running update is read by its steps; an ended one by its outcome. The steps of a
// failure stay reachable because the step it stopped at is where diagnosis starts.
export function updateStepsShown(job?: SyncJob): 'live' | 'folded' | 'hidden' {
  if (!job?.stages?.length) return 'hidden'
  if (job.status === 'queued' || job.status === 'running') return 'live'
  if (job.status === 'failed' || job.status === 'rolled_back') return 'folded'
  return 'hidden'
}

export type UpdateAction = { intent: 'review' | 'cancel'; emphasis: 'primary' | 'quiet'; label: string }

// The card carries a button only when a person has something to do. Automatic
// updates make the install the controller's job, so the offer steps back to quiet.
export function updateAction(phase: AgentUpdateState, policy: { autoUpdate: boolean; updateWaiting: boolean }): UpdateAction | undefined {
  if (phase === 'queued') return { intent: 'cancel', emphasis: 'quiet', label: 'Cancel update' }
  if (phase === 'running' || phase === 'current') return undefined
  if (phase === 'failed' || phase === 'rolled_back') return { intent: 'review', emphasis: 'primary', label: 'Try again…' }
  if (phase === 'unknown') return { intent: 'review', emphasis: 'quiet', label: 'Update reet…' }
  if (!policy.updateWaiting) return undefined
  return policy.autoUpdate
    ? { intent: 'review', emphasis: 'quiet', label: 'Update now…' }
    : { intent: 'review', emphasis: 'primary', label: 'Review update…' }
}

// The build a channel points at, as the UI names it everywhere: the card, the
// review and the machine's settings all describe the same move with the same words.
export function channelBuild(releases: ReleaseInfo | undefined, channel: 'dev' | 'latest'): string {
  if (channel === 'latest') return releases?.latest?.version ?? ''
  if (releases?.dev?.commit) return `dev · ${shortCommit(releases.dev.commit)}`
  return releases?.dev?.version === 'dev' ? 'dev' : ''
}
