// A pipeline that finishes while the app is open: noticed from the outcome reads the app already makes.
// Only `done` counts as finished; a failed, cancelled, unfollowed or ready-to-merge pipeline still wants the person.

export type FinishedItem = { id: string; outcomeId: string; key: string; title: string; objective: string; sessionId: string; at: number }

/** The parts of an outcome record this reads. A `GafferOutcome` is one. */
export type OutcomeRecord = {
  id: string; objective: string; archivedAt?: number
  sessions: { key: string; title: string; state: string; sessionId?: string; updatedAt: number }[]
}

type StorageLike = { getItem(key: string): string | null; setItem(key: string, value: string): void }

/** Every pipeline of an unarchived outcome whose state is exactly done. */
export function finishedItems(outcomes: readonly OutcomeRecord[]): FinishedItem[] {
  return outcomes.flatMap(outcome => outcome.archivedAt ? [] : outcome.sessions
    .filter(piece => piece.state === 'done')
    .map(piece => ({
      id: `${outcome.id}.${piece.key}`, outcomeId: outcome.id, key: piece.key, title: piece.title,
      objective: outcome.objective, sessionId: piece.sessionId ?? '', at: piece.updatedAt,
    })))
}

/** The toast for the pipelines that just finished: one names its pipeline and opens it; several say how many and open the Factory. */
export function finishedToast(items: readonly FinishedItem[]): { message: string; href: string } {
  if (items.length === 1) return { message: `“${items[0].title}” finished.`, href: `/factory?pipeline=${encodeURIComponent(items[0].id)}` }
  return { message: `${items.length} pipelines finished.`, href: '/factory' }
}

/** Remembers which finished pipelines the person has been told about. `storage` may be absent or refuse: then this visit only. */
export function createFinishedWatch(storage: StorageLike | undefined, key: string) {
  let previous = new Map<string, string>()
  let acknowledged = new Set<string>()
  let loaded = false
  let storageWorks = !!storage
  let savedText: string | null = null

  function read(): { present: boolean; ids: string[]; raw: string | null } {
    if (!storage) return { present: false, ids: [], raw: null }
    let raw: string | null
    try { raw = storage.getItem(key) } catch { storageWorks = false; return { present: false, ids: [], raw: null } }
    if (raw === null) return { present: false, ids: [], raw }
    try {
      const value: unknown = JSON.parse(raw)
      // A corrupt value is ignored like a missing one, so the next read that finishes nothing does not toast.
      if (!Array.isArray(value)) return { present: false, ids: [], raw }
      return { present: true, ids: value.filter((id): id is string => typeof id === 'string'), raw }
    } catch { return { present: false, ids: [], raw } }
  }

  function persist() {
    if (!storageWorks || !storage) return
    const text = JSON.stringify([...acknowledged])
    if (text === savedText) return
    try { storage.setItem(key, text); savedText = text } catch { storageWorks = false }
  }

  function states(outcomes: readonly OutcomeRecord[]): Map<string, string> {
    const map = new Map<string, string>()
    for (const outcome of outcomes) if (!outcome.archivedAt) for (const piece of outcome.sessions) map.set(`${outcome.id}.${piece.key}`, piece.state)
    return map
  }

  return {
    /** The pipelines that just moved into done (empty on this watch's first read), and how many finished pipelines are unacknowledged. */
    observe(outcomes: readonly OutcomeRecord[]): { unread: number; finished: FinishedItem[] } {
      const items = finishedItems(outcomes)
      const current = new Set(items.map(item => item.id))
      if (!loaded) {
        loaded = true
        const stored = read()
        // A brand-new (or unreadable) key seeds what is finished now as acknowledged, so a new install and a reload stay quiet.
        acknowledged = stored.present ? new Set(stored.ids.filter(id => current.has(id))) : new Set(current)
        if (stored.raw !== null && storageWorks) savedText = stored.raw
        previous = states(outcomes)
        persist()
        return { unread: items.filter(item => !acknowledged.has(item.id)).length, finished: [] }
      }
      const finished = items.filter(item => previous.get(item.id) !== 'done')
      previous = new Map([...previous, ...states(outcomes)])
      // An item that left done is no longer acknowledged: a genuine later finish notifies again.
      acknowledged = new Set([...acknowledged].filter(id => current.has(id)))
      persist()
      return { unread: items.filter(item => !acknowledged.has(item.id)).length, finished }
    },
    /** Marks every pipeline finished now as seen; one that leaves done loses its acknowledgement. */
    acknowledgeAll(outcomes: readonly OutcomeRecord[]) {
      const current = new Set(finishedItems(outcomes).map(item => item.id))
      const next = new Set([...acknowledged].filter(id => current.has(id)))
      for (const id of current) next.add(id)
      acknowledged = next
      persist()
    },
  }
}
