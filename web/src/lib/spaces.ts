import type { Group } from './api'

export type GroupScope = 'all' | 'organisations' | 'teams' | 'managed'

export const GROUP_SCOPES: { id: GroupScope; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'organisations', label: 'Organisations' },
  { id: 'teams', label: 'Teams' },
  { id: 'managed', label: 'Managed by you' },
]

export function matchesGroupScope(group: Group, scope: string): boolean {
  if (scope === 'all') return true
  if (scope === 'organisations') return group.kind === 'organisation'
  if (scope === 'teams') return group.kind === 'team'
  if (scope === 'managed') return group.role === 'owner' || group.role === 'manager'
  return false
}

/** A group is found by its own name, its kind, or any member's name or address. */
export function matchesGroupSearch(group: Group, query: string): boolean {
  const term = query.trim().toLowerCase()
  if (!term) return true
  const members = (group.members ?? []).map((member) => `${member.name} ${member.email}`).join(' ')
  const haystack = `${group.name} ${group.kind === 'organisation' ? 'organisation' : 'team'} ${members}`.toLowerCase()
  return term.split(/\s+/).every((word) => haystack.includes(word))
}

export function filterGroups(groups: Group[], scope: string, query = ''): Group[] {
  return groups.filter((group) => matchesGroupScope(group, scope) && matchesGroupSearch(group, query))
}

export function groupScopeCounts(groups: Group[]): Record<GroupScope, number> {
  const counts: Record<GroupScope, number> = { all: 0, organisations: 0, teams: 0, managed: 0 }
  for (const group of groups) {
    for (const scope of GROUP_SCOPES) {
      if (matchesGroupScope(group, scope.id)) counts[scope.id]++
    }
  }
  return counts
}

export function groupCountLabel(shown: number, total: number): string {
  if (total === 0) return 'No groups'
  if (shown === total) return `${total} ${total === 1 ? 'group' : 'groups'}`
  return `${shown} of ${total} groups`
}

/** "Organisation", or "Team · Platform engineering" for a team inside an organisation. */
export function groupKindLabel(group: Group, groups: Group[]): string {
  const parent = group.parentId ? groups.find((candidate) => candidate.id === group.parentId)?.name : ''
  return `${group.kind === 'organisation' ? 'Organisation' : 'Team'}${parent ? ` · ${parent}` : ''}`
}

export function groupRoleLabel(role: string): string {
  if (role === 'owner') return 'You manage this'
  if (role === 'manager') return 'Manager'
  if (role === 'member') return 'Member'
  return role ? `${role.slice(0, 1).toUpperCase()}${role.slice(1)}` : 'Member'
}
