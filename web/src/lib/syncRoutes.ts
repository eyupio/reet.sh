/**
 * Sync is two pages. `/sync` answers "am I in sync?" with one sentence, a row per machine and recent activity.
 * `/sync/advanced` holds the sync-plan controls: list-based or exact-set plans, plan revisions and manual scans.
 *
 * Links that carry something only the plan controls read must go to the advanced page, or they would land on a
 * page that ignores them. Every such link is built here, and an older address is sent on by `syncRedirect`.
 */
export const SYNC = '/sync'
export const ADVANCED_SYNC = '/sync/advanced'

/** Query parameters the plan controls read: the machine to plan for, a skill to prepare for removal, a list to start from. */
const PLAN_PARAMS = ['machine', 'remove', 'list']
/** The plan controls' own anchor. Recent activity (`#sync-jobs`) is on both pages, so it stays where it was asked for. */
const PLAN_HASH = '#sync-selection'

/** The advanced page with a machine chosen, and optionally a skill prepared for removal from it. */
export function advancedSyncHref(machine = '', remove = ''): string {
  const query = new URLSearchParams()
  if (machine) query.set('machine', machine)
  if (remove) query.set('remove', remove)
  const text = query.toString()
  return text ? `${ADVANCED_SYNC}?${text}` : ADVANCED_SYNC
}

/** The plan controls' selection, where a problem asks the person to review what a plan syncs. */
export const ADVANCED_SELECTION = `${ADVANCED_SYNC}${PLAN_HASH}`

/** True for an address that draws the Sync page: `/sync` and the two older addresses for it. */
function simpleSync(path: string, query: URLSearchParams): boolean {
  return path === SYNC || path === '/account/sync' || (path === '/machines' && query.get('view') === 'compare')
}

/**
 * Where an address should go instead, or null when it is right as it is. A Sync address that carries a plan
 * parameter or the plan anchor goes to the advanced page with everything it carried; `/sync/advanced/` loses its slash.
 */
export function syncRedirect(path: string, search = '', hash = ''): string | null {
  if (path === `${ADVANCED_SYNC}/`) return `${ADVANCED_SYNC}${search}${hash}`
  const query = new URLSearchParams(search)
  if (!simpleSync(path, query)) return null
  if (hash !== PLAN_HASH && !PLAN_PARAMS.some(name => query.has(name))) return null
  // `view=compare` chose the Sync page on /machines; the advanced page has its own address.
  query.delete('view')
  const text = query.toString()
  return `${ADVANCED_SYNC}${text ? `?${text}` : ''}${hash}`
}
