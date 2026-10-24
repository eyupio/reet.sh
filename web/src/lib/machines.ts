import type { Environment, Machine, ReleaseInfo, SyncAgent, SyncEnvironment, SyncSkill } from './api'

export function groupLabel(env: Environment & { project?: string }): string {
  const names: Record<string, string> = { agents: 'Shared agents / Codex', claude: 'Claude Code', codex: 'Legacy Codex (scan only)', cursor: 'Cursor', opencode: 'OpenCode', reet: 'Scanner', 'reet-server': 'Server' }
  return `${names[env.agent] ?? env.agent}${env.project ? ` · ${env.project}` : ''}`
}
export function agentGroups(machine?: Machine): SyncEnvironment[] { return machine?.groups.filter(g => !g.agent.startsWith('reet')) ?? [] }
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
