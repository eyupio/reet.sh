/** Retired plan addresses lead to the current library status, without plan parameters. */
export function syncRedirect(path: string, search = '', hash = ''): string | null {
  const query = new URLSearchParams(search)
  if (path === '/sync/advanced' || path === '/sync/advanced/' || path === '/account/sync' || path === '/machines' && query.get('view') === 'compare') return '/sync'
  if (path === '/sync' && (['machine', 'remove', 'list'].some(key => query.has(key)) || ['#sync-selection', '#sync-jobs'].includes(hash))) return '/sync'
  return null
}
