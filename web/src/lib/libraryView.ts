import type { LibraryItem } from './api'

/**
 * What the Library page shows about one logical item. Everything here comes
 * from the canonical library: nothing is inferred about what is installed,
 * which the Machines page reports.
 */
export const STATUSES = [
  { id: 'active', label: 'Active' },
  { id: 'archived', label: 'Archived' },
] as const

export type StatusFilter = 'all' | (typeof STATUSES)[number]['id']

export function statusLabel(status: string): string {
  return STATUSES.find(entry => entry.id === status)?.label ?? status.replaceAll('_', ' ')
}

/** Search and status combine; every inactive item stays findable. */
export function filterLibrary(items: LibraryItem[], query: string, status: StatusFilter): LibraryItem[] {
  const needle = query.trim().toLowerCase()
  return items.filter(item => (status === 'all' || item.status === status) && (!needle || item.name.toLowerCase().includes(needle)))
}

/** Per-status totals over the whole library, whatever is searched. */
export function statusCounts(items: LibraryItem[]): Record<'all' | (typeof STATUSES)[number]['id'], number> {
  const counts = { all: items.length, active: 0, archived: 0 }
  for (const item of items) if (item.status in counts) counts[item.status as keyof typeof counts]++
  return counts
}

/** The shelf details migration preserves: where it came from, likes, lists and tags. */
export function itemNotes(item: LibraryItem): string[] {
  const notes: string[] = []
  if (item.aliases?.some(alias => alias.kind === 'hosted')) notes.push(`Written here · ${item.published ? 'public' : 'private'}`)
  if (item.metadata?.liked) notes.push('Liked')
  const lists = item.metadata?.lists ?? []
  if (lists.length) notes.push(`On list ${lists.map(list => list.name).join(', ')}`)
  const tags = item.metadata?.tags ?? []
  if (tags.length) notes.push(`Tagged ${tags.map(tag => tag.name).join(', ')}`)
  return notes
}
