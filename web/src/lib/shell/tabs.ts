// The app shell's navigation types. A tab is a link to a page; `current` marks the page in view (aria-current="page").
export type Tab = { href: string; label: string; current: boolean; group?: string }

// One row of the Jump search: a session, an existing Project or a machine the workspace already knows about.
export type JumpItem = { kind: 'session' | 'project' | 'machine'; label: string; detail: string; href: string }

const KIND_ORDER: JumpItem['kind'][] = ['session', 'project', 'machine']

/** The rows whose name contains the query (case-insensitive), sessions first, then Projects, then machines; at most `limit`. */
export function jumpMatches(items: JumpItem[], query: string, limit = 8): JumpItem[] {
  const q = query.trim().toLowerCase()
  if (!q) return []
  return items
    .filter(item => item.label.toLowerCase().includes(q))
    .sort((a, b) => KIND_ORDER.indexOf(a.kind) - KIND_ORDER.indexOf(b.kind) || a.label.localeCompare(b.label))
    .slice(0, limit)
}
