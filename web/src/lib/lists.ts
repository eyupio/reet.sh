import type { SharedList } from './api'

/** The scopes a person can choose from above a list of lists, in the order they are offered. */
export type ListScope = 'all' | 'mine' | 'people' | 'team' | 'legacy'

export const LIST_SCOPES: { id: ListScope; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'mine', label: 'Mine' },
  { id: 'people', label: 'Shared with me' },
  { id: 'team', label: 'Teams' },
  { id: 'legacy', label: 'Legacy' },
]

export function matchesListScope(list: SharedList, scope: string, ownerId: number): boolean {
  if (scope === 'all') return true
  if (scope === 'mine') return list.ownerId === ownerId
  if (scope === 'team') return list.visibility === 'team'
  if (scope === 'people') return list.sharedWithMe ?? (list.ownerId !== ownerId && ['viewer', 'editor'].includes(list.role ?? '') && ['private', 'link'].includes(list.visibility))
  return list.visibility === 'members'
}

/** A search reads names, descriptions and owners. Every word must appear, in any order. */
export function matchesListSearch(list: SharedList, query: string): boolean {
  const term = query.trim().toLowerCase()
  if (!term) return true
  const haystack = `${list.name} ${list.description} ${list.ownerName ?? ''}`.toLowerCase()
  return term.split(/\s+/).every((word) => haystack.includes(word))
}

/** The lists a scope and a search leave, without changing the order the API sent. */
export function filterLists(lists: SharedList[], scope: string, ownerId: number, query = ''): SharedList[] {
  return lists.filter((list) => matchesListScope(list, scope, ownerId) && matchesListSearch(list, query))
}

/** How many lists each scope holds. Search first, so a chip never promises more than pressing it shows. */
export function listScopeCounts(lists: SharedList[], ownerId: number): Record<ListScope, number> {
  const counts: Record<ListScope, number> = { all: 0, mine: 0, people: 0, team: 0, legacy: 0 }
  for (const list of lists) {
    for (const scope of LIST_SCOPES) {
      if (matchesListScope(list, scope.id, ownerId)) counts[scope.id]++
    }
  }
  return counts
}

/** The one sentence that says whose list it is and what the person may do with it. */
export function listAccessLabel(list: SharedList, ownerId: number): string {
  const access = list.role === 'admin' ? 'Admin access' : list.role === 'editor' ? 'Can edit' : list.role === 'viewer' ? 'Can view' : ''
  const owner = list.ownerId === ownerId ? 'Owned by you' : list.ownerName ? `By ${list.ownerName}` : 'Shared with you'
  return [owner, access].filter(Boolean).join(' · ')
}

/** "5 lists", or "2 of 5 lists" while a filter narrows the set. */
export function listCountLabel(shown: number, total: number): string {
  if (total === 0) return 'No lists'
  if (shown === total) return `${total} ${total === 1 ? 'list' : 'lists'}`
  return `${shown} of ${total} lists`
}
