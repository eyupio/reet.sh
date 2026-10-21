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
