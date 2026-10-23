import type { SharedList } from './api'

export function matchesListScope(list: SharedList, scope: string, ownerId: number): boolean {
  if (scope === 'all') return true
  if (scope === 'mine') return list.ownerId === ownerId
  if (scope === 'team') return list.visibility === 'team'
  if (scope === 'people') return list.sharedWithMe ?? (list.ownerId !== ownerId && ['viewer', 'editor'].includes(list.role ?? '') && ['private', 'link'].includes(list.visibility))
  return list.visibility === 'members'
}
