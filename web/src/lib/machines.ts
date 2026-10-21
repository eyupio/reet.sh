import type { Environment, Machine, SyncEnvironment, SyncSkill } from './api'

export function groupLabel(env: Environment & { project?: string }): string {
  const names: Record<string, string> = { agents: 'Shared agents / Codex', claude: 'Claude Code', codex: 'Legacy Codex (scan only)', cursor: 'Cursor', opencode: 'OpenCode', reet: 'Scanner', 'reet-server': 'Server' }
  return `${names[env.agent] ?? env.agent}${env.project ? ` · ${env.project}` : ''}`
}
export function agentGroups(machine?: Machine): SyncEnvironment[] { return machine?.groups.filter(g => !g.agent.startsWith('reet')) ?? [] }
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
