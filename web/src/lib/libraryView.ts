import type { LibraryItem, LibraryReceipt } from './api'

/**
 * What the Library page shows about one logical item. Everything here comes
 * from the canonical library and the saved machine selections: nothing is
 * inferred about what is installed, which the Machines page reports.
 */
export const STATUSES = [
  { id: 'in_use', label: 'In use' },
  { id: 'to_test', label: 'To test' },
  { id: 'on_shelf', label: 'Parked' },
  { id: 'disabled', label: 'Disabled' },
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
  const counts = { all: items.length, in_use: 0, to_test: 0, on_shelf: 0, disabled: 0 }
  for (const item of items) if (item.status in counts) counts[item.status as keyof typeof counts]++
  return counts
}

/** Machines that have saved a selection of each item, one entry per machine. */
export function machinesByItem(selections: { host: string; itemIds: string[] }[]): Map<string, string[]> {
  const sets = new Map<string, Set<string>>()
  for (const selection of selections) {
    for (const id of selection.itemIds) {
      if (!sets.has(id)) sets.set(id, new Set())
      sets.get(id)!.add(selection.host)
    }
  }
  return new Map([...sets].map(([id, hosts]) => [id, [...hosts].sort((a, b) => a.localeCompare(b))]))
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

/** One saved version in an item's history. A concurrent edit that lost is kept, not lost. */
export function receiptLine(receipt: LibraryReceipt, timeZone = 'UTC'): string {
  const when = new Date(receipt.savedAt)
  // An unset zone (the page has not read the profile yet) uses the browser's own.
  const zone = timeZone || undefined
  const date = when.toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric', timeZone: zone })
  const time = when.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', timeZone: zone })
  return `${date} · ${time} · ${receipt.won ? 'became the current version' : 'kept as another version'}${receipt.clockFallback ? ' (time estimated)' : ''}`
}

const size = (bytes = 0): string => bytes < 1024 ? `${bytes} B` : bytes < 1024 * 1024 ? `${(bytes / 1024).toFixed(1)} KB` : `${(bytes / (1024 * 1024)).toFixed(1)} MB`

/**
 * What one differing file means, in words: where it exists and how big each copy is. The comparison knows names, sizes
 * and fingerprints only, so it says a file changed, never what changed inside it.
 */
export function differenceFileLine(file: { path: string; change: string; reetBytes?: number; machineBytes?: number }, host: string): string {
  if (file.change === 'machine') return `Only on ${host} (${size(file.machineBytes)})`
  if (file.change === 'reet') return `Only in reet’s copy (${size(file.reetBytes)})`
  if (file.reetBytes === file.machineBytes) return `Changed (same size, ${size(file.reetBytes)})`
  return `Changed (${size(file.reetBytes)} in reet, ${size(file.machineBytes)} on ${host})`
}

/** "2 files differ" and what kinds, for the line above the list. */
export function differenceSummary(files: { change: string }[], host: string): string {
  if (!files.length) return `${host}’s files now match reet’s copy. This notice clears after its next scan.`
  const count = (change: string) => files.filter(file => file.change === change).length
  const parts = [
    count('changed') ? `${count('changed')} changed` : '',
    count('machine') ? `${count('machine')} only on ${host}` : '',
    count('reet') ? `${count('reet')} only in reet’s copy` : '',
  ].filter(Boolean)
  return `${files.length} ${files.length === 1 ? 'file differs' : 'files differ'}: ${parts.join(', ')}.`
}
