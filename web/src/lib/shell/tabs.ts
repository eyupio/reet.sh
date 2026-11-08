// The app shell's navigation types. A tab is a link to a page; `current` marks the page in view (aria-current="page").
export type Tab = { href: string; label: string; current: boolean; group?: string }

// One row of the Jump search: a session, an existing Project, a shelf skill or a machine the workspace already knows about.
export type JumpItem = { kind: 'session' | 'project' | 'skill' | 'machine'; label: string; detail: string; href: string }

const KIND_ORDER: JumpItem['kind'][] = ['session', 'project', 'skill', 'machine']

/** The rows whose name contains the query (case-insensitive), sessions, Projects and skills first, then machines; at most `limit`. */
export function jumpMatches(items: JumpItem[], query: string, limit = 8): JumpItem[] {
  const q = query.trim().toLowerCase()
  if (!q) return []
  return items
    .filter(item => item.label.toLowerCase().includes(q))
    .sort((a, b) => KIND_ORDER.indexOf(a.kind) - KIND_ORDER.indexOf(b.kind) || a.label.localeCompare(b.label))
    .slice(0, limit)
}

/** The section a supporting page belongs to: its name and every entry of that section, in order. */
export type PageGroup = { group: string; items: { label: string; href: string; current: boolean }[] }

/** The group of the `more` entry in view, with all of its siblings (the caller shows the row only when there are two or more); null when the page is not in a group. */
export function pageGroup(more: Tab[]): PageGroup | null {
  const group = more.find(tab => tab.current && tab.group)?.group
  if (!group) return null
  return {
    group,
    items: more.filter(tab => tab.group === group).map(({ label, href, current }) => ({ label, href, current })),
  }
}
