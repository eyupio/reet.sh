import type { UserMeta } from './api'

export type AdminUserScope = 'all' | 'owner' | 'admin' | 'member' | 'viewer'

export const ADMIN_USER_SCOPES: { id: AdminUserScope; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'owner', label: 'Owners' },
  { id: 'admin', label: 'Admins' },
  { id: 'member', label: 'Members' },
  { id: 'viewer', label: 'Viewers' },
]

/** A person is found by name, address or role, in any word order. */
export function adminUserMatchesSearch(user: UserMeta, query: string): boolean {
  const term = query.trim().toLowerCase()
  if (!term) return true
  const haystack = `${user.name} ${user.email} ${user.role}`.toLowerCase()
  return term.split(/\s+/).every((word) => haystack.includes(word))
}

export function filterAdminUsers(users: UserMeta[], scope: string, query = ''): UserMeta[] {
  return users.filter((user) => (scope === 'all' || user.role === scope) && adminUserMatchesSearch(user, query))
}

/** Role counts are taken from the people the current search leaves, so a chip never promises more than pressing it shows. */
export function adminUserRoleCounts(users: UserMeta[]): Record<AdminUserScope, number> {
  const counts: Record<AdminUserScope, number> = { all: users.length, owner: 0, admin: 0, member: 0, viewer: 0 }
  for (const user of users) {
    if (user.role in counts && user.role !== 'all') counts[user.role as Exclude<AdminUserScope, 'all'>]++
  }
  return counts
}

export function adminUserCountLabel(shown: number, total: number): string {
  if (total === 0) return 'No people'
  if (shown === total) return `${total} ${total === 1 ? 'person' : 'people'}`
  return `${shown} of ${total} people`
}
