// Account has a dozen sections. A row of that many links wraps in a narrow column, so the settings that belong
// together sit in two dropdowns and the rest stay one tap away as links.
export type AccountEntry = { key: string; label: string }
export type AccountNavLink = { label: string; href: string; current: boolean }
export type AccountNavItem = AccountNavLink & { children?: AccountNavLink[] }

const GROUPS: { label: string; keys: string[] }[] = [
  { label: 'Security', keys: ['security', 'sso', 'sessions', 'tokens', 'mcp'] },
  { label: 'Data & activity', keys: ['backups', 'maintenance', 'activity'] },
]

/** Machines and Sync live on the Machines page; every other section is an /account/<key> page. */
export function accountHref(key: string): string {
  return key === 'machines' ? '/machines' : key === 'sync' ? '/machines?view=compare' : `/account/${key}`
}

/**
 * The Account row: each present section as a link, with the members of a group folded into one dropdown placed where
 * its first member was. A group with a single present member stays a plain link. The group's `href` is its first
 * member's (a group has no page of its own) and it is current when any member is.
 */
export function accountNav(entries: AccountEntry[], current: string): AccountNavItem[] {
  const link = (entry: AccountEntry): AccountNavLink => ({ label: entry.label, href: accountHref(entry.key), current: entry.key === current })
  const items: AccountNavItem[] = []
  const placed = new Set<string>()
  for (const entry of entries) {
    const group = GROUPS.find((g) => g.keys.includes(entry.key))
    if (!group) { items.push(link(entry)); continue }
    if (placed.has(group.label)) continue
    placed.add(group.label)
    const members = entries.filter((e) => group.keys.includes(e.key)).map(link)
    items.push(members.length === 1 ? members[0] : { label: group.label, href: members[0].href, current: members.some((m) => m.current), children: members })
  }
  return items
}
