// Which list sections are open, remembered per person in localStorage. Storage can be missing, full or blocked,
// so every read and write is guarded and falls back to the first-visit state.
export type OpenSections = Record<string, boolean>

/** The saved state merged over `first`, or `first` when nothing usable is saved. */
export function readSections(storage: Pick<Storage, 'getItem'> | undefined, key: string, first: OpenSections): OpenSections {
  try {
    const raw = storage?.getItem(key)
    if (!raw) return { ...first }
    const saved: unknown = JSON.parse(raw)
    if (!saved || typeof saved !== 'object' || Array.isArray(saved)) return { ...first }
    const merged: OpenSections = { ...first }
    for (const [name, open] of Object.entries(saved)) if (typeof open === 'boolean') merged[name] = open
    return merged
  } catch {
    return { ...first }
  }
}

/** Save the state; a storage that refuses is ignored. */
export function writeSections(storage: Pick<Storage, 'setItem'> | undefined, key: string, value: OpenSections): void {
  try {
    storage?.setItem(key, JSON.stringify(value))
  } catch {
    // Nothing to do: the sections simply start from their first state next time.
  }
}
